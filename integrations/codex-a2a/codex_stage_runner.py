from __future__ import annotations

import asyncio
import logging

from dataclasses import dataclass
from pathlib import Path

from codex_exec_runtime import (
    build_codex_exec_command,
    build_event_summary_evidence,
    build_terminal_evidence_error,
    parse_codex_event_stream,
)


MAX_ERROR_TEXT_LEN = 2000
TERMINATE_TIMEOUT_SECONDS = 3


@dataclass(frozen=True)
class CodexStageRequest:
    task_id: str
    stage_id: str
    prompt: str
    workdir: Path


@dataclass(frozen=True)
class CodexStageOutput:
    message: str
    evidence: str
    events_file: Path


class CodexStageRunner:
    def __init__(self, codex_bin: str, output_dir: Path, logger: logging.Logger):
        self._codex_bin = codex_bin
        self._output_dir = output_dir
        self._logger = logger
        self._lock = asyncio.Lock()
        self._processes: dict[str, asyncio.subprocess.Process] = {}

    async def run(self, req: CodexStageRequest) -> CodexStageOutput:
        events_file = self._output_dir / f"{req.task_id}.{req.stage_id}.events.jsonl"
        cmd = build_codex_exec_command(self._codex_bin, req.workdir)
        process = await self._spawn_process(cmd, req)
        await self._set_process(req.task_id, process)
        try:
            stdout, stderr = await process.communicate(input=req.prompt.encode("utf-8"))
        finally:
            await self._clear_process(req.task_id)

        stdout_text = (stdout or b"").decode("utf-8", errors="replace").strip()
        await asyncio.to_thread(events_file.write_text, stdout_text, encoding="utf-8")
        if process.returncode != 0:
            raise RuntimeError(build_process_error(stderr, stdout, process.returncode))

        summary = parse_codex_event_stream(stdout_text)
        terminal_error = build_terminal_evidence_error(summary)
        if terminal_error:
            raise RuntimeError(f"{terminal_error} | events_file={events_file}")

        evidence = build_event_summary_evidence(summary, req.workdir, events_file)
        self._logger.info("codex stage completed task_id=%s stage=%s", req.task_id, req.stage_id)
        return CodexStageOutput(message=summary.final_message, evidence=evidence, events_file=events_file)

    async def cancel(self, task_id: str) -> None:
        process = await self._take_process(task_id)
        if process is None:
            return
        if process.returncode is not None:
            return
        process.terminate()
        try:
            await asyncio.wait_for(process.wait(), timeout=TERMINATE_TIMEOUT_SECONDS)
        except asyncio.TimeoutError:
            process.kill()
            await process.wait()

    async def _spawn_process(self, cmd: list[str], req: CodexStageRequest) -> asyncio.subprocess.Process:
        try:
            return await asyncio.create_subprocess_exec(
                *cmd,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
        except FileNotFoundError as exc:
            raise RuntimeError(f"codex cli not found: {self._codex_bin}") from exc
        except OSError as exc:
            raise RuntimeError(f"failed to start codex for stage={req.stage_id}: {exc}") from exc

    async def _set_process(self, task_id: str, process: asyncio.subprocess.Process) -> None:
        async with self._lock:
            self._processes[task_id] = process

    async def _take_process(self, task_id: str) -> asyncio.subprocess.Process | None:
        async with self._lock:
            return self._processes.pop(task_id, None)

    async def _clear_process(self, task_id: str) -> None:
        async with self._lock:
            self._processes.pop(task_id, None)

    async def snapshot(self) -> dict[str, object]:
        async with self._lock:
            active_task_ids = sorted(self._processes.keys())
        return {
            "active_task_ids": active_task_ids,
            "active_process_count": len(active_task_ids),
        }


def build_process_error(stderr: bytes | None, stdout: bytes | None, return_code: int | None) -> str:
    stderr_text = (stderr or b"").decode("utf-8", errors="replace").strip()
    stdout_text = (stdout or b"").decode("utf-8", errors="replace").strip()
    if stderr_text:
        return trim_text(stderr_text)
    if stdout_text:
        return trim_text(stdout_text)
    if return_code is None:
        return "codex terminated unexpectedly"
    return f"codex exit code {return_code}"


def trim_text(raw: str) -> str:
    text = raw.strip()
    if len(text) <= MAX_ERROR_TEXT_LEN:
        return text
    return text[:MAX_ERROR_TEXT_LEN].strip()
