from __future__ import annotations

import asyncio
import json

from datetime import datetime, timezone
from pathlib import Path

from a2a.server.tasks import TaskStore
from a2a.types import Task, TaskState, TaskStatus
from a2a.utils import new_agent_text_message


ORPHANED_REASON = "service_restarted_orphaned_task"
ORPHANED_MESSAGE = "codex-a2a 服务已重启，原任务执行进程丢失，当前任务已标记失败，请重新提交。"
TERMINAL_TASK_STATES = {
    TaskState.completed,
    TaskState.canceled,
    TaskState.failed,
    TaskState.rejected,
}


class PersistentJSONTaskStore(TaskStore):
    def __init__(self, root_dir: Path) -> None:
        self._root_dir = root_dir.resolve()
        self._root_dir.mkdir(parents=True, exist_ok=True)
        self._lock = asyncio.Lock()

    async def save(self, task: Task, context=None) -> None:
        payload = task.model_dump(mode="json", by_alias=True, exclude_none=True)
        async with self._lock:
            await asyncio.to_thread(self._write_payload, self._task_file(task.id), payload)

    async def get(self, task_id: str, context=None) -> Task | None:
        path = self._task_file(task_id)
        if not path.exists():
            return None
        async with self._lock:
            payload = await asyncio.to_thread(self._read_payload, path)
        return Task.model_validate(payload)

    async def delete(self, task_id: str, context=None) -> None:
        path = self._task_file(task_id)
        async with self._lock:
            await asyncio.to_thread(self._delete_file, path)

    async def list_tasks(self) -> list[Task]:
        async with self._lock:
            files = sorted(self._root_dir.glob("*.json"))
            payloads = [await asyncio.to_thread(self._read_payload, path) for path in files]
        tasks = [Task.model_validate(payload) for payload in payloads]
        tasks.sort(key=_task_sort_key, reverse=True)
        return tasks

    async def fail_incomplete_tasks(self) -> list[str]:
        repaired: list[str] = []
        for task in await self.list_tasks():
            if task.status.state in TERMINAL_TASK_STATES:
                continue
            metadata = dict(task.metadata or {})
            metadata.update(
                {
                    "orphaned": True,
                    "orphaned_reason": ORPHANED_REASON,
                    "last_known_state": task.status.state.value,
                    "orphaned_at": datetime.now(timezone.utc).isoformat(),
                }
            )
            task.metadata = metadata
            task.status = TaskStatus(
                state=TaskState.failed,
                message=new_agent_text_message(
                    text=ORPHANED_MESSAGE,
                    task_id=task.id,
                    context_id=task.context_id,
                ),
                timestamp=datetime.now(timezone.utc).isoformat(),
            )
            await self.save(task)
            repaired.append(task.id)
        return repaired

    def _task_file(self, task_id: str) -> Path:
        safe_id = task_id.strip().replace("/", "_").replace("\\", "_")
        return self._root_dir / f"{safe_id}.json"

    @staticmethod
    def _write_payload(path: Path, payload: dict) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        temp_path = path.with_suffix(path.suffix + ".tmp")
        temp_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
        temp_path.replace(path)

    @staticmethod
    def _read_payload(path: Path) -> dict:
        return json.loads(path.read_text(encoding="utf-8"))

    @staticmethod
    def _delete_file(path: Path) -> None:
        if path.exists():
            path.unlink()


def _task_sort_key(task: Task) -> tuple[str, str]:
    timestamp = task.status.timestamp or ""
    return (timestamp, task.id)
