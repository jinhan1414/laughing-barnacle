from __future__ import annotations

import json
import logging

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Awaitable, Callable, Mapping

from a2a.types import Part, TaskState, TextPart

from xiaohongshu_codex_runner import CodexStageRequest, CodexStageRunner


WORKFLOW_SKILL_ID = "xiaohongshu_ops"
WORKFLOW_NAME = "xiaohongshu-ops-closed-loop"
STAGE_COUNT = 6

PromptBuilder = Callable[[str, Mapping[str, dict[str, Any]]], str]


@dataclass(frozen=True)
class StageSpec:
    id: str
    title: str
    required_keys: tuple[str, ...]
    prompt_builder: PromptBuilder


@dataclass(frozen=True)
class StageResult:
    stage_id: str
    title: str
    payload: dict[str, Any]
    evidence: str


@dataclass(frozen=True)
class WorkflowResult:
    final_report: str
    workflow_state_file: Path
    stages: tuple[StageResult, ...]


class WorkflowExecutionError(RuntimeError):
    def __init__(self, stage_id: str, reason: str):
        super().__init__(f"stage={stage_id} failed: {reason}")
        self.stage_id = stage_id
        self.reason = reason


class XiaohongshuWorkflowEngine:
    def __init__(self, runner: CodexStageRunner, output_dir: Path, logger: logging.Logger):
        self._runner = runner
        self._output_dir = output_dir
        self._logger = logger

    async def run(
        self,
        task_id: str,
        workdir: Path,
        user_prompt: str,
        updater: Any,
        message_builder: Callable[[str], Any],
    ) -> WorkflowResult:
        context: dict[str, dict[str, Any]] = {}
        stage_results: list[StageResult] = []
        for index, stage in enumerate(get_workflow_stages(), start=1):
            stage_result = await self._run_stage(
                task_id=task_id,
                stage=stage,
                stage_index=index,
                user_prompt=user_prompt,
                workdir=workdir,
                context=context,
                updater=updater,
                message_builder=message_builder,
            )
            context[stage.id] = stage_result.payload
            stage_results.append(stage_result)
        report = build_final_report(user_prompt, context)
        state_file = await self._write_workflow_state(task_id, user_prompt, stage_results, report)
        return WorkflowResult(final_report=report, workflow_state_file=state_file, stages=tuple(stage_results))

    async def _run_stage(
        self,
        task_id: str,
        stage: StageSpec,
        stage_index: int,
        user_prompt: str,
        workdir: Path,
        context: dict[str, dict[str, Any]],
        updater: Any,
        message_builder: Callable[[str], Any],
    ) -> StageResult:
        metadata = build_stage_status_metadata(stage.id, stage.title, stage_index)
        await updater.update_status(
            TaskState.working,
            message_builder(f"{WORKFLOW_NAME} stage {stage_index}/{STAGE_COUNT}: {stage.title}"),
            metadata=metadata,
        )
        prompt = stage.prompt_builder(user_prompt, context)
        request = CodexStageRequest(task_id=task_id, stage_id=stage.id, prompt=prompt, workdir=workdir)
        self._logger.info("workflow stage start task_id=%s stage=%s", task_id, stage.id)
        try:
            output = await self._runner.run(request)
            payload = parse_stage_payload(output.message, stage.required_keys)
        except Exception as exc:
            self._logger.error("workflow stage failed task_id=%s stage=%s error=%s", task_id, stage.id, exc)
            raise WorkflowExecutionError(stage.id, str(exc)) from exc
        artifact_json = json.dumps(payload, ensure_ascii=False)
        await updater.add_artifact([Part(root=TextPart(text=artifact_json))], name=f"xiaohongshu-{stage.id}")
        await updater.add_artifact([Part(root=TextPart(text=output.evidence))], name=f"xiaohongshu-{stage.id}-evidence")
        self._logger.info("workflow stage done task_id=%s stage=%s", task_id, stage.id)
        return StageResult(stage_id=stage.id, title=stage.title, payload=payload, evidence=output.evidence)

    async def _write_workflow_state(
        self,
        task_id: str,
        user_prompt: str,
        stage_results: list[StageResult],
        report: str,
    ) -> Path:
        output = {
            "workflow": WORKFLOW_NAME,
            "task_id": task_id,
            "user_prompt": user_prompt,
            "stage_count": len(stage_results),
            "stages": [
                {"stage_id": item.stage_id, "title": item.title, "payload": item.payload, "evidence": item.evidence}
                for item in stage_results
            ],
            "final_report": report,
        }
        state_file = self._output_dir / f"{task_id}.xiaohongshu.workflow.json"
        await _write_json(state_file, output)
        return state_file


async def _write_json(path: Path, payload: dict[str, Any]) -> None:
    text = json.dumps(payload, ensure_ascii=False, indent=2)
    await __import__("asyncio").to_thread(path.write_text, text, encoding="utf-8")


def should_run_xiaohongshu_workflow(user_prompt: str, metadata: Mapping[str, Any] | None) -> bool:
    if metadata:
        fields = (
            str(metadata.get("workflow", "")).strip().lower(),
            str(metadata.get("skill_id", "")).strip().lower(),
            str(metadata.get("skill", "")).strip().lower(),
        )
        if WORKFLOW_SKILL_ID in fields or WORKFLOW_NAME in fields:
            return True
    prompt = user_prompt.strip().lower()
    if "小红书" in prompt:
        return True
    return False


def build_stage_status_metadata(stage_id: str, title: str, index: int) -> dict[str, Any]:
    return {
        "workflow": WORKFLOW_NAME,
        "skill_id": WORKFLOW_SKILL_ID,
        "stage_id": stage_id,
        "stage_title": title,
        "stage_index": index,
        "stage_total": STAGE_COUNT,
    }


def parse_stage_payload(raw: str, required_keys: tuple[str, ...]) -> dict[str, Any]:
    payload_text = extract_json_object(raw)
    try:
        payload = json.loads(payload_text)
    except json.JSONDecodeError as exc:
        raise ValueError(f"stage output is not valid json: {exc.msg}") from exc
    if not isinstance(payload, dict):
        raise ValueError("stage output root must be json object")
    for key in required_keys:
        if key not in payload:
            raise ValueError(f"stage output missing required key: {key}")
    return payload


def extract_json_object(raw: str) -> str:
    text = raw.strip()
    if text.startswith("```"):
        lines = text.splitlines()
        body = [line for line in lines if not line.strip().startswith("```")]
        text = "\n".join(body).strip()
    start = text.find("{")
    end = text.rfind("}")
    if start < 0 or end < 0 or end <= start:
        raise ValueError("stage output does not contain json object")
    return text[start : end + 1]


def get_workflow_stages() -> tuple[StageSpec, ...]:
    return (
        StageSpec("topic_selection", "选题", ("topic_candidates", "selected_topic"), build_topic_prompt),
        StageSpec("copywriting", "文案生成", ("title", "content_markdown", "hashtags", "cover_text"), build_copy_prompt),
        StageSpec("review", "审核", ("risk_level", "issues", "approved", "revised_copy"), build_review_prompt),
        StageSpec("publish_orchestration", "发布编排", ("schedule", "execution", "checklist"), build_publish_prompt),
        StageSpec("data_collection", "数据回收", ("data_source", "window", "metrics", "notes"), build_data_prompt),
        StageSpec("retrospective", "复盘", ("summary", "insights", "next_actions", "next_topic_hypothesis"), build_retrospective_prompt),
    )


def build_topic_prompt(user_prompt: str, _: Mapping[str, dict[str, Any]]) -> str:
    return (
        "你是小红书运营选题专家。基于用户需求输出选题决策。\n"
        f"用户需求：{user_prompt}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"topic_candidates":[{"title":"","angle":"","reason":""}],'
        '"selected_topic":{"title":"","angle":"","target_audience":"","value_proposition":""}}'
    )


def build_copy_prompt(_: str, context: Mapping[str, dict[str, Any]]) -> str:
    selected = json.dumps(context["topic_selection"]["selected_topic"], ensure_ascii=False)
    return (
        "你是小红书爆文文案助手。请根据选题生成可发布文案。\n"
        f"选题信息：{selected}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"title":"","content_markdown":"","hashtags":["#"],"cover_text":""}'
    )


def build_review_prompt(_: str, context: Mapping[str, dict[str, Any]]) -> str:
    copy_payload = json.dumps(context["copywriting"], ensure_ascii=False)
    return (
        "你是小红书审核员，请从合规、真实性、表达清晰度进行审核并改写。\n"
        f"待审文案：{copy_payload}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"risk_level":"low|medium|high","issues":[""],"approved":true,'
        '"revised_copy":{"title":"","content_markdown":"","hashtags":["#"],"cover_text":""}}'
    )


def build_publish_prompt(_: str, context: Mapping[str, dict[str, Any]]) -> str:
    revised = json.dumps(context["review"]["revised_copy"], ensure_ascii=False)
    return (
        "你是发布编排器。因为无真实平台权限，执行方式必须是模拟发布，但流程字段必须完整。\n"
        f"已审核文案：{revised}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"schedule":{"publish_at":"","channel":"xiaohongshu","timezone":"Asia/Shanghai"},'
        '"execution":{"mode":"simulated","result":"queued|published","external_publish_id":"sim-*",'
        '"boundary":"无真实发布权限，已执行模拟发布"},"checklist":[""]}'
    )


def build_data_prompt(_: str, context: Mapping[str, dict[str, Any]]) -> str:
    publish_data = json.dumps(context["publish_orchestration"], ensure_ascii=False)
    return (
        "你是数据回收分析器。基于发布编排输出 24h 数据回收结果。\n"
        f"发布结果：{publish_data}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"data_source":"simulated","window":"24h",'
        '"metrics":{"impressions":0,"likes":0,"comments":0,"collects":0,"follows":0,"ctr":0},"notes":""}'
    )


def build_retrospective_prompt(user_prompt: str, context: Mapping[str, dict[str, Any]]) -> str:
    metrics = json.dumps(context["data_collection"], ensure_ascii=False)
    topic = json.dumps(context["topic_selection"]["selected_topic"], ensure_ascii=False)
    return (
        "你是运营复盘顾问。基于选题与数据回收输出可执行复盘结论。\n"
        f"用户需求：{user_prompt}\n"
        f"选题：{topic}\n"
        f"数据：{metrics}\n"
        "请只输出 JSON，不要任何额外文字。\n"
        "JSON 结构："
        '{"summary":"","insights":[""],"next_actions":[""],"next_topic_hypothesis":""}'
    )


def build_final_report(user_prompt: str, context: Mapping[str, dict[str, Any]]) -> str:
    sections = [
        "# 小红书运营闭环结果",
        "## 0) 任务输入",
        user_prompt,
        "## 1) 选题",
        json.dumps(context["topic_selection"], ensure_ascii=False, indent=2),
        "## 2) 文案生成",
        json.dumps(context["copywriting"], ensure_ascii=False, indent=2),
        "## 3) 审核",
        json.dumps(context["review"], ensure_ascii=False, indent=2),
        "## 4) 发布编排",
        json.dumps(context["publish_orchestration"], ensure_ascii=False, indent=2),
        "## 5) 数据回收",
        json.dumps(context["data_collection"], ensure_ascii=False, indent=2),
        "## 6) 复盘",
        json.dumps(context["retrospective"], ensure_ascii=False, indent=2),
    ]
    return "\n\n".join(sections)
