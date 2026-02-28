from __future__ import annotations

import json

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping


DEFAULT_EXECUTION_PREFIX = (
    "执行要求：\n"
    "1. 直接执行用户任务，不要只输出计划后结束。\n"
    "2. 需要时主动读取文件并运行命令，直到形成可交付结果。\n"
    "3. 失败时必须显式返回失败原因与关键证据。\n"
    "4. 最终输出给出结论与支撑依据，避免空泛描述。"
)
MAX_PARSE_WARNINGS = 3


@dataclass(frozen=True)
class CodexEventSummary:
    final_message: str
    turn_completed: bool
    event_count: int
    parse_warnings: tuple[str, ...]


def build_effective_prompt(user_prompt: str) -> str:
    prompt = user_prompt.strip()
    if not prompt:
        return DEFAULT_EXECUTION_PREFIX
    return DEFAULT_EXECUTION_PREFIX + "\n\n用户任务：\n" + prompt


def build_codex_exec_command(codex_bin: str, workdir: Path, prompt: str) -> list[str]:
    return [
        codex_bin,
        "exec",
        "--dangerously-bypass-approvals-and-sandbox",
        "--skip-git-repo-check",
        "--json",
        "-C",
        str(workdir),
        prompt,
    ]


def resolve_task_workdir(default_workdir: Path, metadata: Mapping[str, Any] | None) -> Path:
    if not metadata or "working_dir" not in metadata:
        return default_workdir
    raw = metadata.get("working_dir")
    if not isinstance(raw, str):
        raise ValueError("metadata.working_dir must be a string")
    text = raw.strip()
    if not text:
        raise ValueError("metadata.working_dir must not be empty")
    candidate = Path(text).expanduser()
    if not candidate.is_absolute():
        candidate = (default_workdir / candidate).resolve()
    else:
        candidate = candidate.resolve()
    if not candidate.exists():
        raise ValueError(f"metadata.working_dir not found: {candidate}")
    if not candidate.is_dir():
        raise ValueError(f"metadata.working_dir is not a directory: {candidate}")
    try:
        next(candidate.iterdir(), None)
    except OSError as exc:
        raise ValueError(f"metadata.working_dir is not accessible: {candidate}") from exc
    return candidate


def parse_codex_event_stream(raw: str) -> CodexEventSummary:
    final_message = ""
    turn_completed = False
    event_count = 0
    parse_warnings: list[str] = []
    for index, line in enumerate(raw.splitlines(), start=1):
        text = line.strip()
        if not text:
            continue
        event_count += 1
        payload = parse_event_line(text, index, parse_warnings)
        if payload is None:
            continue
        if payload.get("type") == "turn.completed":
            turn_completed = True
        agent_message = extract_agent_message(payload)
        if agent_message:
            final_message = agent_message
    return CodexEventSummary(
        final_message=final_message,
        turn_completed=turn_completed,
        event_count=event_count,
        parse_warnings=tuple(parse_warnings),
    )


def build_event_summary_evidence(summary: CodexEventSummary, workdir: Path, events_file: Path) -> str:
    payload = {
        "working_dir": str(workdir),
        "events_file": str(events_file),
        "event_count": summary.event_count,
        "turn_completed": summary.turn_completed,
    }
    if summary.parse_warnings:
        payload["parse_warnings"] = list(summary.parse_warnings)
    return json.dumps(payload, ensure_ascii=False)


def build_terminal_evidence_error(summary: CodexEventSummary) -> str:
    if not summary.turn_completed:
        return "codex output missing turn.completed event"
    if summary.final_message == "":
        return "codex output missing final agent_message"
    return ""


def parse_event_line(line: str, line_no: int, warnings: list[str]) -> dict[str, Any] | None:
    try:
        payload = json.loads(line)
    except json.JSONDecodeError as exc:
        append_parse_warning(warnings, f"line {line_no}: invalid json ({exc.msg})")
        return None
    if not isinstance(payload, dict):
        append_parse_warning(warnings, f"line {line_no}: event is not object")
        return None
    return payload


def append_parse_warning(warnings: list[str], text: str) -> None:
    if len(warnings) >= MAX_PARSE_WARNINGS:
        return
    warnings.append(text.strip())


def extract_agent_message(event: dict[str, Any]) -> str:
    if event.get("type") != "item.completed":
        return ""
    item = event.get("item")
    if not isinstance(item, dict):
        return ""
    if item.get("type") != "agent_message":
        return ""
    text = item.get("text")
    if isinstance(text, str):
        return text.strip()
    content = item.get("content")
    if isinstance(content, str):
        return content.strip()
    return ""
