#!/usr/bin/env python3
from __future__ import annotations

import argparse
import asyncio
import shutil

from pathlib import Path

import uvicorn

from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.apps.jsonrpc import A2AFastAPIApplication
from a2a.server.events import EventQueue
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import InMemoryTaskStore, TaskUpdater
from a2a.types import (
    AgentCapabilities,
    AgentCard,
    AgentSkill,
    Part,
    TextPart,
)
from a2a.utils import new_agent_text_message


DEFAULT_PROTOCOL_VERSION = "0.3.0"
DEFAULT_AGENT_VERSION = "1.0.0"
TERMINATE_TIMEOUT_SECONDS = 3
MAX_ERROR_TEXT_LEN = 2000


class CodexAgentExecutor(AgentExecutor):
    def __init__(self, workdir: Path, codex_bin: str, output_dir: Path):
        self.workdir = str(workdir)
        self.codex_bin = codex_bin
        self.output_dir = output_dir
        self._processes: dict[str, asyncio.subprocess.Process] = {}
        self._lock = asyncio.Lock()

    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        task_id, context_id = require_context_ids(context)
        updater = TaskUpdater(event_queue=event_queue, task_id=task_id, context_id=context_id)
        prompt = context.get_user_input().strip()
        if not prompt:
            await updater.failed(self._status_message(task_id, context_id, "empty message text"))
            return

        await updater.submit(self._status_message(task_id, context_id, "submitted"))
        await updater.start_work(self._status_message(task_id, context_id, "working"))
        await self._run_codex_task(updater, task_id, context_id, prompt)

    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        task_id, context_id = require_context_ids(context)
        updater = TaskUpdater(event_queue=event_queue, task_id=task_id, context_id=context_id)
        proc = await self._take_process(task_id)
        if proc is not None:
            await terminate_process(proc)
        await updater.cancel(self._status_message(task_id, context_id, "canceled"))

    async def _run_codex_task(self, updater: TaskUpdater, task_id: str, context_id: str, prompt: str) -> None:
        output_file = self.output_dir / f"{task_id}.txt"
        cmd = [self.codex_bin, "exec", "-C", self.workdir, "-o", str(output_file), prompt]
        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
        except FileNotFoundError:
            await updater.failed(self._status_message(task_id, context_id, f"codex cli not found: {self.codex_bin}"))
            return

        await self._set_process(task_id, proc)
        try:
            stdout, stderr = await proc.communicate()
        except asyncio.CancelledError:
            await terminate_process(proc)
            raise
        finally:
            await self._clear_process(task_id)

        if proc.returncode != 0:
            err_text = build_process_error(stderr, stdout, proc.returncode)
            await updater.failed(self._status_message(task_id, context_id, err_text))
            return

        output_text = await read_output_text(output_file)
        artifact_text = output_text or "(empty output)"
        await updater.add_artifact([Part(root=TextPart(text=artifact_text))], name="codex-output")
        await updater.complete(self._status_message(task_id, context_id, "completed"))

    async def _set_process(self, task_id: str, proc: asyncio.subprocess.Process) -> None:
        async with self._lock:
            self._processes[task_id] = proc

    async def _take_process(self, task_id: str) -> asyncio.subprocess.Process | None:
        async with self._lock:
            return self._processes.pop(task_id, None)

    async def _clear_process(self, task_id: str) -> None:
        async with self._lock:
            self._processes.pop(task_id, None)

    def _status_message(self, task_id: str, context_id: str, text: str):
        return new_agent_text_message(text=text, task_id=task_id, context_id=context_id)


def require_context_ids(context: RequestContext) -> tuple[str, str]:
    task_id = (context.task_id or "").strip()
    context_id = (context.context_id or "").strip()
    if not task_id or not context_id:
        raise RuntimeError("request context missing task_id/context_id")
    return task_id, context_id


def build_process_error(stderr: bytes | None, stdout: bytes | None, return_code: int | None) -> str:
    stderr_text = (stderr or b"").decode("utf-8", errors="replace").strip()
    stdout_text = (stdout or b"").decode("utf-8", errors="replace").strip()
    if stderr_text:
        return trim_text(stderr_text, MAX_ERROR_TEXT_LEN)
    if stdout_text:
        return trim_text(stdout_text, MAX_ERROR_TEXT_LEN)
    if return_code is None:
        return "codex terminated unexpectedly"
    return f"codex exit code {return_code}"


async def read_output_text(path: Path) -> str:
    if not path.exists():
        return ""
    return (await asyncio.to_thread(path.read_text, encoding="utf-8", errors="ignore")).strip()


async def terminate_process(proc: asyncio.subprocess.Process) -> None:
    if proc.returncode is not None:
        return
    proc.terminate()
    try:
        await asyncio.wait_for(proc.wait(), timeout=TERMINATE_TIMEOUT_SECONDS)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()


def trim_text(raw: str, max_len: int) -> str:
    text = raw.strip()
    if max_len <= 0 or len(text) <= max_len:
        return text
    return text[:max_len].strip()


def resolve_codex_bin(explicit_path: str) -> str:
    candidates: list[str] = []
    explicit = explicit_path.strip()
    if explicit:
        candidates.append(explicit)
    for name in ("codex", "codex.cmd", "codex.exe"):
        found = shutil.which(name)
        if found:
            candidates.append(found)
    seen: set[str] = set()
    for candidate in candidates:
        normalized = str(Path(candidate).expanduser())
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        if Path(normalized).exists():
            return normalized
    return ""


def resolve_base_url(host: str, port: int, explicit_base_url: str) -> str:
    value = explicit_base_url.strip()
    if value:
        return value.rstrip("/")
    safe_host = host.strip() or "127.0.0.1"
    if safe_host == "0.0.0.0":
        safe_host = "127.0.0.1"
    return f"http://{safe_host}:{port}"


def build_agent_card(base_url: str) -> AgentCard:
    return AgentCard(
        name="codex-local",
        description="Local Codex CLI agent powered by official a2a-python SDK",
        url=f"{base_url}/a2a/rpc",
        version=DEFAULT_AGENT_VERSION,
        protocol_version=DEFAULT_PROTOCOL_VERSION,
        preferred_transport="JSONRPC",
        default_input_modes=["text/plain"],
        default_output_modes=["text/plain"],
        capabilities=AgentCapabilities(
            streaming=False,
            push_notifications=False,
            state_transition_history=True,
        ),
        skills=[
            AgentSkill(
                id="codex_exec",
                name="Codex Exec",
                description="Execute Codex CLI prompts and return generated output",
                tags=["code", "codex"],
                examples=["请修复这个 bug 并解释修改内容"],
            )
        ],
    )


def build_app(executor: CodexAgentExecutor, base_url: str):
    card = build_agent_card(base_url)
    task_store = InMemoryTaskStore()
    request_handler = DefaultRequestHandler(agent_executor=executor, task_store=task_store)
    return A2AFastAPIApplication(agent_card=card, http_handler=request_handler).build(
        agent_card_url="/.well-known/agent-card.json",
        rpc_url="/a2a/rpc",
        title="codex-a2a",
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run local Codex as an A2A agent using official a2a-python SDK.")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=9091)
    parser.add_argument("--base-url", default="")
    parser.add_argument("--workdir", required=True)
    parser.add_argument("--output-dir", default=str(Path(__file__).parent / "state" / "output"))
    parser.add_argument("--codex-bin", default="")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    workdir = Path(args.workdir).resolve()
    if not workdir.exists():
        raise SystemExit(f"workdir not found: {workdir}")

    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    codex_bin = resolve_codex_bin(args.codex_bin)
    if not codex_bin:
        raise SystemExit("codex cli not found. Set --codex-bin or fix PATH.")

    base_url = resolve_base_url(args.host, args.port, args.base_url)
    executor = CodexAgentExecutor(workdir=workdir, codex_bin=codex_bin, output_dir=output_dir)
    app = build_app(executor, base_url)

    print(f"codex-a2a listening on http://{args.host}:{args.port}")
    print(f"agent_card={base_url}/.well-known/agent-card.json")
    print(f"rpc={base_url}/a2a/rpc")
    print(f"codex={codex_bin}")
    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
