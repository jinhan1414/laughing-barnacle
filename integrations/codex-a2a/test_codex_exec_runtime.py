from __future__ import annotations

import json
import unittest

from pathlib import Path
from tempfile import TemporaryDirectory

from codex_exec_runtime import (
    CodexEventSummary,
    build_codex_exec_command,
    build_effective_prompt,
    build_event_summary_evidence,
    parse_codex_event_stream,
    resolve_task_workdir,
)


class CodexExecRuntimeTests(unittest.TestCase):
    def test_build_effective_prompt_contains_prefix_and_user_task(self) -> None:
        prompt = build_effective_prompt("请分析仓库")
        self.assertIn("执行要求：", prompt)
        self.assertIn("用户任务：", prompt)
        self.assertIn("请分析仓库", prompt)

    def test_build_codex_exec_command_uses_high_privilege_json_mode(self) -> None:
        command = build_codex_exec_command("codex", Path("E:/repo"), "do it")
        self.assertIn("--dangerously-bypass-approvals-and-sandbox", command)
        self.assertIn("--json", command)
        self.assertIn("-C", command)
        self.assertEqual("do it", command[-1])

    def test_resolve_task_workdir_uses_default_when_metadata_missing(self) -> None:
        with TemporaryDirectory() as tmp:
            default_workdir = Path(tmp).resolve()
            resolved = resolve_task_workdir(default_workdir, {})
            self.assertEqual(default_workdir, resolved)

    def test_resolve_task_workdir_uses_valid_metadata(self) -> None:
        with TemporaryDirectory() as tmp:
            default_workdir = Path(tmp).resolve()
            child = default_workdir / "child"
            child.mkdir(parents=True, exist_ok=True)
            resolved = resolve_task_workdir(default_workdir, {"working_dir": str(child)})
            self.assertEqual(child.resolve(), resolved)

    def test_resolve_task_workdir_rejects_invalid_metadata(self) -> None:
        with TemporaryDirectory() as tmp:
            default_workdir = Path(tmp).resolve()
            with self.assertRaisesRegex(ValueError, "metadata\\.working_dir"):
                resolve_task_workdir(default_workdir, {"working_dir": "   "})
            with self.assertRaisesRegex(ValueError, "metadata\\.working_dir"):
                resolve_task_workdir(default_workdir, {"working_dir": str(default_workdir / "missing")})

    def test_parse_codex_event_stream_extracts_terminal_message(self) -> None:
        raw = "\n".join(
            [
                '{"type":"turn.started"}',
                '{"type":"item.completed","item":{"type":"agent_message","text":"中间信息"}}',
                '{"type":"item.completed","item":{"type":"agent_message","text":"最终答案"}}',
                '{"type":"turn.completed"}',
            ]
        )
        summary = parse_codex_event_stream(raw)
        self.assertTrue(summary.turn_completed)
        self.assertEqual("最终答案", summary.final_message)
        self.assertEqual(4, summary.event_count)
        self.assertEqual(0, len(summary.parse_warnings))

    def test_parse_codex_event_stream_collects_parse_warnings(self) -> None:
        raw = "\n".join(
            [
                "not-json",
                '{"type":"item.completed","item":{"type":"agent_message","text":"ok"}}',
            ]
        )
        summary = parse_codex_event_stream(raw)
        self.assertFalse(summary.turn_completed)
        self.assertEqual("ok", summary.final_message)
        self.assertGreaterEqual(len(summary.parse_warnings), 1)

    def test_build_event_summary_evidence_is_json(self) -> None:
        summary = CodexEventSummary(
            final_message="ok",
            turn_completed=True,
            event_count=8,
            parse_warnings=tuple(),
        )
        with TemporaryDirectory() as tmp:
            evidence = build_event_summary_evidence(
                summary=summary,
                workdir=Path(tmp).resolve(),
                events_file=Path(tmp).resolve() / "task.events.jsonl",
            )
        payload = json.loads(evidence)
        self.assertTrue(payload["turn_completed"])
        self.assertEqual(8, payload["event_count"])


if __name__ == "__main__":
    unittest.main()
