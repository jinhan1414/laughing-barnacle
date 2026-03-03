from __future__ import annotations

import json
import sys
import unittest

from pathlib import Path
from tempfile import TemporaryDirectory

sys.path.insert(0, str(Path(__file__).resolve().parent))

from xiaohongshu_codex_runner import CodexStageOutput
from xiaohongshu_workflow import (
    STAGE_COUNT,
    XiaohongshuWorkflowEngine,
    parse_stage_payload,
    should_run_xiaohongshu_workflow,
)


class FakeRunner:
    def __init__(self) -> None:
        self.calls: list[str] = []

    async def run(self, req):
        self.calls.append(req.stage_id)
        payload = build_payload_for_stage(req.stage_id)
        return CodexStageOutput(
            message=json.dumps(payload, ensure_ascii=False),
            evidence=f"evidence-{req.stage_id}",
            events_file=req.workdir / f"{req.task_id}.{req.stage_id}.events.jsonl",
        )


class FakeUpdater:
    def __init__(self) -> None:
        self.status_calls: list[tuple[str, dict]] = []
        self.artifacts: list[str] = []

    async def update_status(self, state, message=None, final=False, timestamp=None, metadata=None):
        self.status_calls.append((str(state), metadata or {}))

    async def add_artifact(self, parts, name: str):
        self.artifacts.append(name)


class XiaohongshuWorkflowTests(unittest.IsolatedAsyncioTestCase):
    async def test_engine_runs_six_stage_closed_loop(self) -> None:
        with TemporaryDirectory() as tmp:
            output_dir = Path(tmp).resolve()
            engine = XiaohongshuWorkflowEngine(runner=FakeRunner(), output_dir=output_dir, logger=build_logger())
            updater = FakeUpdater()
            result = await engine.run(
                task_id="task-1",
                workdir=output_dir,
                user_prompt="为新中式咖啡做小红书运营闭环",
                updater=updater,
                message_builder=lambda text: text,
            )
            self.assertEqual(STAGE_COUNT, len(result.stages))
            self.assertTrue(result.workflow_state_file.exists())
            self.assertIn("## 6) 复盘", result.final_report)
            self.assertEqual(STAGE_COUNT * 2, len(updater.artifacts))
            self.assertEqual(STAGE_COUNT, len(updater.status_calls))
            self.assertTrue(all(item[0].endswith("working") for item in updater.status_calls))

    def test_parse_stage_payload_rejects_missing_required_key(self) -> None:
        with self.assertRaisesRegex(ValueError, "missing required key"):
            parse_stage_payload('{"a":1}', ("b",))

    def test_should_run_xiaohongshu_workflow_by_prompt_or_metadata(self) -> None:
        self.assertTrue(should_run_xiaohongshu_workflow("请做一份小红书计划", None))
        self.assertTrue(should_run_xiaohongshu_workflow("plain", {"skill_id": "xiaohongshu_ops"}))
        self.assertFalse(should_run_xiaohongshu_workflow("plain", None))


def build_payload_for_stage(stage_id: str) -> dict:
    if stage_id == "topic_selection":
        return {
            "topic_candidates": [{"title": "主题A", "angle": "角度A", "reason": "理由A"}],
            "selected_topic": {
                "title": "主题A",
                "angle": "角度A",
                "target_audience": "新手",
                "value_proposition": "低成本可执行",
            },
        }
    if stage_id == "copywriting":
        return {
            "title": "标题",
            "content_markdown": "正文",
            "hashtags": ["#小红书"],
            "cover_text": "封面",
        }
    if stage_id == "review":
        return {
            "risk_level": "low",
            "issues": [],
            "approved": True,
            "revised_copy": {
                "title": "标题",
                "content_markdown": "正文",
                "hashtags": ["#小红书"],
                "cover_text": "封面",
            },
        }
    if stage_id == "publish_orchestration":
        return {
            "schedule": {"publish_at": "2026-03-03T10:00:00+08:00", "channel": "xiaohongshu", "timezone": "Asia/Shanghai"},
            "execution": {
                "mode": "simulated",
                "result": "queued",
                "external_publish_id": "sim-001",
                "boundary": "无真实发布权限，已执行模拟发布",
            },
            "checklist": ["内容已审"],
        }
    if stage_id == "data_collection":
        return {
            "data_source": "simulated",
            "window": "24h",
            "metrics": {"impressions": 1000, "likes": 110, "comments": 15, "collects": 9, "follows": 4, "ctr": 0.08},
            "notes": "模拟采样",
        }
    if stage_id == "retrospective":
        return {
            "summary": "复盘总结",
            "insights": ["洞察1"],
            "next_actions": ["动作1"],
            "next_topic_hypothesis": "主题B",
        }
    raise ValueError(f"unexpected stage: {stage_id}")


def build_logger():
    import logging

    return logging.getLogger("test-xiaohongshu-workflow")


if __name__ == "__main__":
    unittest.main()
