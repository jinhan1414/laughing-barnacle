from __future__ import annotations

import sys
import unittest

from pathlib import Path
from tempfile import TemporaryDirectory

from fastapi import FastAPI
from fastapi.testclient import TestClient

from a2a.types import Task, TaskState, TaskStatus
from a2a.utils import new_agent_text_message

sys.path.insert(0, str(Path(__file__).resolve().parent))

from codex_a2a_observability import register_observability_routes
from persistent_task_store import (
    ORPHANED_REASON,
    PersistentJSONTaskStore,
)


class PersistentTaskStoreTests(unittest.IsolatedAsyncioTestCase):
    async def test_fail_incomplete_tasks_marks_orphaned_failed(self) -> None:
        with TemporaryDirectory() as tmp:
            store = PersistentJSONTaskStore(Path(tmp))
            task = build_task("task-1", TaskState.working)
            await store.save(task)

            repaired = await store.fail_incomplete_tasks()
            self.assertEqual(["task-1"], repaired)

            saved = await store.get("task-1")
            self.assertIsNotNone(saved)
            assert saved is not None
            self.assertEqual(TaskState.failed, saved.status.state)
            self.assertEqual(ORPHANED_REASON, saved.metadata["orphaned_reason"])
            self.assertIn("服务已重启", saved.status.message.parts[0].root.text)


class ObservabilityRouteTests(unittest.IsolatedAsyncioTestCase):
    async def test_health_and_debug_routes_expose_stage_and_output_files(self) -> None:
        with TemporaryDirectory() as tmp:
            root = Path(tmp)
            output_dir = root / "output"
            output_dir.mkdir()
            (output_dir / "task-2.workflow.json").write_text("{}", encoding="utf-8")

            store = PersistentJSONTaskStore(root / "tasks")
            task = build_task("task-2", TaskState.working)
            task.metadata = {
                "workflow": "codex-generic",
                "stage_id": "generic",
                "stage_title": "执行中",
                "stage_index": 1,
                "stage_total": 1,
            }
            await store.save(task)

            app = FastAPI()

            async def runtime_snapshot() -> dict[str, object]:
                return {
                    "codex_available": True,
                    "active_task_ids": ["task-2"],
                    "active_process_count": 1,
                }

            register_observability_routes(app, store, runtime_snapshot, output_dir)

            with TestClient(app) as client:
                health = client.get("/healthz")
                self.assertEqual(200, health.status_code)
                self.assertEqual("ok", health.json()["status"])
                self.assertEqual(1, health.json()["task_counts"]["working"])

                detail = client.get("/debug/tasks/task-2")
                self.assertEqual(200, detail.status_code)
                body = detail.json()
                self.assertEqual("generic", body["stage"]["stage_id"])
                self.assertTrue(any(path.endswith("task-2.workflow.json") for path in body["outputFiles"]))


def build_task(task_id: str, state: TaskState) -> Task:
    return Task(
        id=task_id,
        context_id=f"{task_id}-context",
        status=TaskStatus(
            state=state,
            message=new_agent_text_message(
                text=f"status={state.value}",
                task_id=task_id,
                context_id=f"{task_id}-context",
            ),
        ),
    )


if __name__ == "__main__":
    unittest.main()
