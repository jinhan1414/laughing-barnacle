#!/usr/bin/env python3
from __future__ import annotations

import argparse
import asyncio
import logging
import shutil

from pathlib import Path

import uvicorn

from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.apps.jsonrpc import A2AFastAPIApplication
from a2a.server.events import EventQueue
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.tasks import TaskUpdater
from a2a.types import AgentCapabilities, AgentCard, AgentSkill, Part, TextPart
from a2a.utils import new_agent_text_message

from codex_exec_runtime import build_effective_prompt, resolve_task_workdir
from codex_a2a_observability import register_observability_routes
from persistent_task_store import PersistentJSONTaskStore
from xiaohongshu_codex_runner import CodexStageRequest, CodexStageRunner
from xiaohongshu_workflow import (
    WORKFLOW_NAME,
    WorkflowExecutionError,
    XiaohongshuWorkflowEngine,
    should_run_xiaohongshu_workflow,
)


DEFAULT_PROTOCOL_VERSION = "0.3.0"
DEFAULT_AGENT_VERSION = "1.1.0"


class CodexAgentExecutor(AgentExecutor):
    def __init__(self, workdir: Path, codex_bin: str, output_dir: Path):
        self._workdir = workdir
        self._codex_bin = codex_bin
        self._output_dir = output_dir
        self._last_error = ""
        self._logger = logging.getLogger("codex-a2a.executor")
        self._runner = CodexStageRunner(codex_bin=codex_bin, output_dir=output_dir, logger=self._logger)
        self._workflow = XiaohongshuWorkflowEngine(runner=self._runner, output_dir=output_dir, logger=self._logger)

    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        task_id, context_id = require_context_ids(context)
        updater = TaskUpdater(event_queue=event_queue, task_id=task_id, context_id=context_id)
        user_prompt = context.get_user_input().strip()
        if not user_prompt:
            await updater.failed(self._status_message(task_id, context_id, "empty message text"))
            return
        try:
            task_workdir = resolve_task_workdir(self._workdir, context.metadata)
        except ValueError as exc:
            await updater.failed(self._status_message(task_id, context_id, str(exc)))
            return

        await updater.submit(self._status_message(task_id, context_id, "submitted"))
        await updater.start_work(self._status_message(task_id, context_id, "working"))
        try:
            if should_run_xiaohongshu_workflow(user_prompt, context.metadata):
                await self._run_xiaohongshu_workflow(updater, task_id, context_id, task_workdir, user_prompt)
                return
            await self._run_single_codex_stage(updater, task_id, context_id, task_workdir, user_prompt)
        except WorkflowExecutionError as exc:
            self._last_error = str(exc)
            await updater.failed(self._status_message(task_id, context_id, str(exc)))
        except Exception as exc:
            self._last_error = str(exc)
            self._logger.exception("execute failed task_id=%s", task_id)
            await updater.failed(self._status_message(task_id, context_id, str(exc)))

    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        task_id, context_id = require_context_ids(context)
        updater = TaskUpdater(event_queue=event_queue, task_id=task_id, context_id=context_id)
        await self._runner.cancel(task_id)
        await updater.cancel(self._status_message(task_id, context_id, "canceled"))

    async def _run_single_codex_stage(
        self,
        updater: TaskUpdater,
        task_id: str,
        context_id: str,
        task_workdir: Path,
        user_prompt: str,
    ) -> None:
        request = CodexStageRequest(
            task_id=task_id,
            stage_id="generic",
            prompt=build_effective_prompt(user_prompt),
            workdir=task_workdir,
        )
        output = await self._runner.run(request)
        await updater.add_artifact([Part(root=TextPart(text=output.message))], name="codex-output")
        await updater.add_artifact([Part(root=TextPart(text=output.evidence))], name="codex-evidence")
        await updater.complete(self._status_message(task_id, context_id, "completed"))

    async def _run_xiaohongshu_workflow(
        self,
        updater: TaskUpdater,
        task_id: str,
        context_id: str,
        task_workdir: Path,
        user_prompt: str,
    ) -> None:
        result = await self._workflow.run(
            task_id=task_id,
            workdir=task_workdir,
            user_prompt=user_prompt,
            updater=updater,
            message_builder=lambda text: self._status_message(task_id, context_id, text),
        )
        await updater.add_artifact([Part(root=TextPart(text=result.final_report))], name="xiaohongshu-final-report")
        await updater.add_artifact(
            [Part(root=TextPart(text=f"state_file={result.workflow_state_file}"))],
            name="xiaohongshu-workflow-state",
        )
        await updater.complete(self._status_message(task_id, context_id, f"completed ({WORKFLOW_NAME})"))

    def _status_message(self, task_id: str, context_id: str, text: str):
        return new_agent_text_message(text=text, task_id=task_id, context_id=context_id)

    async def runtime_snapshot(self) -> dict[str, object]:
        runner = await self._runner.snapshot()
        return {
            "workdir": str(self._workdir),
            "output_dir": str(self._output_dir),
            "codex_bin": self._codex_bin,
            "codex_available": Path(self._codex_bin).exists(),
            "last_error": self._last_error,
            **runner,
        }


def require_context_ids(context: RequestContext) -> tuple[str, str]:
    task_id = (context.task_id or "").strip()
    context_id = (context.context_id or "").strip()
    if not task_id or not context_id:
        raise RuntimeError("request context missing task_id/context_id")
    return task_id, context_id


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
            ),
            AgentSkill(
                id="xiaohongshu_ops",
                name="小红书运营闭环",
                description="Run six-stage Xiaohongshu operation loop: topic, copywriting, review, publish orchestration, data collection, retrospective.",
                tags=["xiaohongshu", "operations", "workflow"],
                examples=["为护肤新品执行小红书运营闭环，输出全链路结果"],
            ),
        ],
    )


def build_app(
    executor: CodexAgentExecutor,
    base_url: str,
    task_store: PersistentJSONTaskStore,
    output_dir: Path,
):
    card = build_agent_card(base_url)
    request_handler = DefaultRequestHandler(agent_executor=executor, task_store=task_store)
    app = A2AFastAPIApplication(agent_card=card, http_handler=request_handler).build(
        agent_card_url="/.well-known/agent-card.json",
        rpc_url="/a2a/rpc",
        title="codex-a2a",
    )
    register_observability_routes(app, task_store, executor.runtime_snapshot, output_dir)
    return app


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run local Codex as an A2A agent using official a2a-python SDK.")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=9091)
    parser.add_argument("--base-url", default="")
    parser.add_argument("--workdir", required=True)
    parser.add_argument("--output-dir", default=str(Path(__file__).parent / "state" / "output"))
    parser.add_argument("--task-store-dir", default="")
    parser.add_argument("--codex-bin", default="")
    return parser.parse_args()


def resolve_task_store_dir(explicit_dir: str, output_dir: Path) -> Path:
    value = explicit_dir.strip()
    if value:
        return Path(value).resolve()
    return output_dir.parent / "tasks"


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
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
    task_store = PersistentJSONTaskStore(resolve_task_store_dir(args.task_store_dir, output_dir))
    orphaned_task_ids = asyncio.run(task_store.fail_incomplete_tasks())
    if orphaned_task_ids:
        logging.getLogger("codex-a2a.task-store").warning(
            "marked orphaned tasks failed: %s",
            ", ".join(orphaned_task_ids),
        )
    executor = CodexAgentExecutor(workdir=workdir, codex_bin=codex_bin, output_dir=output_dir)
    app = build_app(executor, base_url, task_store, output_dir)
    print(f"codex-a2a listening on http://{args.host}:{args.port}")
    print(f"agent_card={base_url}/.well-known/agent-card.json")
    print(f"rpc={base_url}/a2a/rpc")
    print(f"codex={codex_bin}")
    print(f"task_store={resolve_task_store_dir(args.task_store_dir, output_dir)}")
    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
