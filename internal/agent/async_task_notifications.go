package agent

import (
	"context"
	"encoding/json"
	"strings"

	"laughing-barnacle/internal/llm"
)

const (
	maxNotificationSummaryItems = 2
	maxNotificationSummaryRunes = 180
)

var taskResultMetaPrefixes = []string{
	"agent_id:", "task_id:", "status:", "raw_status:",
	"sdk_provider:", "sdk_method:",
}

func (a *Agent) onAsyncTaskStatusChanged(task AsyncTask) {
	if a == nil || a.store == nil {
		return
	}
	content := "task_id=" + safeOrEmpty(task.ID) +
		" | type=" + safeOrEmpty(task.TaskType) +
		" | status=" + safeOrEmpty(task.Status)
	if text := strings.TrimSpace(task.RemoteTaskID); text != "" {
		content += " | remote_task_id=" + text
	}
	a.store.AppendEvent("async_task_status", content)
}

func (a *Agent) onAsyncTaskCompleted(ctx context.Context, task AsyncTask) {
	if a == nil || a.store == nil {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	resp, text, err := a.generateAsyncTaskNotification(ctx, task)
	if err != nil || text == "" {
		text = fallbackAsyncTaskNotification(task)
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	text = mergeNotificationSummary(text, buildAsyncTaskResultSummary(task))
	a.store.AppendAssistant(text, toConversationUsage(resp.Usage))
	go a.resumeAutonomousRunsForTask(ctx, task)
}

func fallbackAsyncTaskNotification(task AsyncTask) string {
	if strings.EqualFold(strings.TrimSpace(task.Status), asyncTaskStatusSucceeded) {
		return "后台任务 " + safeOrEmpty(task.ID) + " 已完成。"
	}
	if strings.EqualFold(strings.TrimSpace(task.Status), asyncTaskStatusCanceled) {
		return "后台任务 " + safeOrEmpty(task.ID) + " 已取消。"
	}
	return "后台任务 " + safeOrEmpty(task.ID) + " 执行失败：" + safeOrEmpty(trimRunes(task.Error, 120))
}

func (a *Agent) generateAsyncTaskNotification(ctx context.Context, task AsyncTask) (llm.ChatResponse, string, error) {
	if a == nil || a.llm == nil {
		return llm.ChatResponse{}, "", nil
	}
	systemPrompt := "你是数字分身。请将后台任务终态生成 1-2 句中文通知，简洁明确，不要 markdown。"
	userPrompt := "task_id: " + safeOrEmpty(task.ID) +
		"\nstatus: " + safeOrEmpty(task.Status) +
		"\ntype: " + safeOrEmpty(task.TaskType) +
		"\nrequest: " + safeOrEmpty(trimRunes(task.Request, 160)) +
		"\nresult: " + safeOrEmpty(trimRunes(task.Result, 360)) +
		"\nerror: " + safeOrEmpty(trimRunes(task.Error, 220))
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "async_task_notify",
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	})
	return resp, strings.TrimSpace(resp.Content), err
}

func buildAsyncTaskResultSummary(task AsyncTask) string {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	if status == asyncTaskStatusSucceeded {
		summary := summarizeSucceededTaskResult(task.Result)
		if summary == "" {
			return ""
		}
		return "结果摘要：" + summary
	}
	if status == asyncTaskStatusFailed {
		errText := strings.TrimSpace(task.Error)
		if errText == "" {
			return ""
		}
		return "失败原因：" + trimRunes(errText, 120)
	}
	return ""
}

func summarizeSucceededTaskResult(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	blocks := collectTaskResultBlocks(raw)
	artifacts := collectTaskResultBlocksByKind(blocks, "artifact")
	if len(artifacts) > 0 {
		return joinTaskSummaryItems(artifacts)
	}
	messages := collectTaskResultBlocksByKind(blocks, "message")
	if len(messages) > 0 {
		return joinTaskSummaryItems(messages)
	}
	fallback := collectGeneralTaskResultLines(raw)
	return joinTaskSummaryItems(fallback)
}

type taskResultBlock struct {
	kind    string
	content string
}

func collectTaskResultBlocks(raw string) []taskResultBlock {
	out := make([]taskResultBlock, 0, maxNotificationSummaryItems)
	currentKind := ""
	currentLines := make([]string, 0, 4)
	flush := func() {
		text := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if currentKind == "" || text == "" {
			currentKind = ""
			currentLines = currentLines[:0]
			return
		}
		out = append(out, taskResultBlock{kind: currentKind, content: text})
		currentKind = ""
		currentLines = currentLines[:0]
	}
	for _, line := range strings.Split(raw, "\n") {
		text := strings.TrimSpace(line)
		kind, value := parseTaskResultLine(text)
		if kind != "" {
			flush()
			currentKind = kind
			if value != "" {
				currentLines = append(currentLines, value)
			}
			continue
		}
		if currentKind == "" || text == "" || isTaskResultMetaLine(text) {
			continue
		}
		currentLines = append(currentLines, text)
	}
	flush()
	return out
}

func parseTaskResultLine(text string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(lower, "artifact:"):
		return "artifact", strings.TrimSpace(text[len("artifact:"):])
	case strings.HasPrefix(lower, "message:"):
		return "message", strings.TrimSpace(text[len("message:"):])
	default:
		return "", ""
	}
}

func collectTaskResultBlocksByKind(blocks []taskResultBlock, kind string) []string {
	out := make([]string, 0, maxNotificationSummaryItems)
	for _, block := range blocks {
		if block.kind != kind {
			continue
		}
		text := strings.TrimSpace(block.content)
		if text == "" || isTaskResultEvidenceBlock(text) {
			continue
		}
		out = append(out, text)
		if len(out) >= maxNotificationSummaryItems {
			break
		}
	}
	return out
}

func collectGeneralTaskResultLines(raw string) []string {
	out := make([]string, 0, maxNotificationSummaryItems)
	for _, line := range strings.Split(raw, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if isTaskResultMetaLine(text) || isTaskResultEvidenceBlock(text) {
			continue
		}
		out = append(out, text)
		if len(out) >= maxNotificationSummaryItems {
			break
		}
	}
	return out
}

func isTaskResultMetaLine(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range taskResultMetaPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isTaskResultEvidenceBlock(text string) bool {
	payload := make(map[string]any)
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return false
	}
	_, hasWorkdir := payload["working_dir"]
	_, hasEventsFile := payload["events_file"]
	_, hasEventCount := payload["event_count"]
	_, hasTurnCompleted := payload["turn_completed"]
	return hasWorkdir && hasEventsFile && hasEventCount && hasTurnCompleted
}

func joinTaskSummaryItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		out = append(out, trimRunes(text, maxNotificationSummaryRunes))
	}
	if len(out) == 0 {
		return ""
	}
	return trimRunes(strings.Join(out, "；"), maxNotificationSummaryRunes)
}

func mergeNotificationSummary(base, summary string) string {
	base = strings.TrimSpace(base)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return base
	}
	if base == "" {
		return summary
	}
	if strings.Contains(base, summary) {
		return base
	}
	return base + "\n" + summary
}
