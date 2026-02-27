package agent

import (
	"context"
	"strings"

	"laughing-barnacle/internal/llm"
)

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
	if a == nil || a.store == nil || a.llm == nil {
		return
	}
	systemPrompt := "你是数字分身。请将后台任务终态生成 1-2 句中文通知，简洁明确，不要 markdown。"
	userPrompt := "task_id: " + safeOrEmpty(task.ID) +
		"\nstatus: " + safeOrEmpty(task.Status) +
		"\ntype: " + safeOrEmpty(task.TaskType) +
		"\nrequest: " + safeOrEmpty(trimRunes(task.Request, 160)) +
		"\nresult: " + safeOrEmpty(trimRunes(task.Result, 220)) +
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
	text := strings.TrimSpace(resp.Content)
	if err != nil || text == "" {
		text = fallbackAsyncTaskNotification(task)
	}
	a.store.AppendAssistant(text, toConversationUsage(resp.Usage))
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
