from __future__ import annotations

from collections import Counter
from pathlib import Path
from typing import Any, Awaitable, Callable

from fastapi import FastAPI, HTTPException

from a2a.types import Message, Task

from persistent_task_store import PersistentJSONTaskStore


RuntimeSnapshot = Callable[[], Awaitable[dict[str, Any]]]


def register_observability_routes(
    app: FastAPI,
    task_store: PersistentJSONTaskStore,
    runtime_snapshot: RuntimeSnapshot,
    output_dir: Path,
) -> None:
    @app.get("/healthz")
    async def healthz() -> dict[str, Any]:
        tasks = await task_store.list_tasks()
        counts = Counter(task.status.state.value for task in tasks)
        runtime = await runtime_snapshot()
        codex_available = bool(runtime.get("codex_available"))
        status = "ok" if codex_available else "degraded"
        return {
            "status": status,
            "task_counts": dict(counts),
            "runtime": runtime,
        }

    @app.get("/debug/tasks")
    async def debug_tasks() -> dict[str, Any]:
        tasks = await task_store.list_tasks()
        return {
            "tasks": [
                {
                    "task_id": task.id,
                    "state": task.status.state.value,
                    "status_message": extract_message_text(task.status.message),
                    "context_id": task.context_id,
                    "metadata": dict(task.metadata or {}),
                }
                for task in tasks
            ]
        }

    @app.get("/debug/tasks/{task_id}")
    async def debug_task(task_id: str) -> dict[str, Any]:
        task = await task_store.get(task_id)
        if task is None:
            raise HTTPException(status_code=404, detail=f"task not found: {task_id}")
        return build_task_debug_payload(task, output_dir)


def build_task_debug_payload(task: Task, output_dir: Path) -> dict[str, Any]:
    payload = task.model_dump(mode="json", by_alias=True, exclude_none=True)
    metadata = dict(task.metadata or {})
    payload["statusMessageText"] = extract_message_text(task.status.message)
    payload["stage"] = {
        "workflow": metadata.get("workflow", ""),
        "stage_id": metadata.get("stage_id", ""),
        "stage_title": metadata.get("stage_title", ""),
        "stage_index": metadata.get("stage_index", ""),
        "stage_total": metadata.get("stage_total", ""),
    }
    payload["outputFiles"] = [str(path.resolve()) for path in sorted(output_dir.glob(f"{task.id}*"))]
    return payload


def extract_message_text(message: Message | None) -> str:
    if message is None:
        return ""
    parts: list[str] = []
    for part in message.parts:
        root = part.root
        text = getattr(root, "text", "")
        if isinstance(text, str) and text.strip():
            parts.append(text.strip())
    return " ".join(parts).strip()
