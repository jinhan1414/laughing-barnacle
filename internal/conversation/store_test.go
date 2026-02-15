package conversation

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreWithFile_PersistsSummaryMessagesAndToolCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.Append("user", "今天北京天气")
	if err := store.SetLatestUserToolCalls([]ToolCall{
		{
			ID:        "call_1",
			Name:      "weather__query",
			Arguments: `{"city":"beijing"}`,
			Result:    `{"temp":18}`,
		},
	}); err != nil {
		t.Fatalf("SetLatestUserToolCalls error: %v", err)
	}
	store.Append("assistant", "18 度")
	store.SetSummaryAndTrim("用户询问天气", 10)
	store.AppendEvent("context_compression", "【上下文压缩】\n用户询问天气")

	_ = store.Close()

	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })

	summary, messages, events := reloaded.SnapshotWithEvents()
	if summary != "用户询问天气" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call on first user message, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != "weather__query" {
		t.Fatalf("unexpected tool call name: %q", messages[0].ToolCalls[0].Name)
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("unexpected second role: %s", messages[1].Role)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestSetLatestUserToolCalls_RequiresPendingUserMessage(t *testing.T) {
	store := NewStore()
	store.Append("assistant", "ready")
	if err := store.SetLatestUserToolCalls([]ToolCall{{Name: "any"}}); err == nil {
		t.Fatalf("expected error without pending user message")
	}
}

func TestStoreReset_ClearsAndPersistsAllState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.Append("user", "u1")
	store.Append("assistant", "a1")
	store.SetSummaryAndTrim("summary", 10)
	store.AppendEvent("context_compression", "compressed")

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	summary, messages, events := store.SnapshotWithEvents()
	if summary != "" || len(messages) != 0 || len(events) != 0 {
		t.Fatalf("expected empty store after reset")
	}
	_ = store.Close()

	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	summary, messages, events = reloaded.SnapshotWithEvents()
	if summary != "" || len(messages) != 0 || len(events) != 0 {
		t.Fatalf("expected persisted empty state after reset")
	}
}

func TestStoreSwitchBranch_IsolatesBranchChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}

	store.Append("user", "main-only")
	if err := store.SwitchBranch("task/long-job"); err != nil {
		t.Fatalf("SwitchBranch error: %v", err)
	}
	store.Append("assistant", "branch-only")
	if err := store.SwitchBranch("main"); err != nil {
		t.Fatalf("SwitchBranch main error: %v", err)
	}

	_, mainMsgs, _ := store.SnapshotWithEvents()
	if !containsMessage(mainMsgs, "main-only") {
		t.Fatalf("expected main message after switching back")
	}
	if containsMessage(mainMsgs, "branch-only") {
		t.Fatalf("unexpected branch-only message on main")
	}
}

func TestStoreListBranchesAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}

	store.Append("user", "main-seed")
	if err := store.SwitchBranch("task/merge"); err != nil {
		t.Fatalf("SwitchBranch task/merge error: %v", err)
	}
	store.Append("assistant", "task-result")
	if err := store.SwitchBranch("main"); err != nil {
		t.Fatalf("SwitchBranch main error: %v", err)
	}

	branches := store.ListBranches()
	if !containsString(branches, "main") || !containsString(branches, "task/merge") {
		t.Fatalf("unexpected branch list: %v", branches)
	}

	if err := store.MergeBranch("task/merge"); err != nil {
		t.Fatalf("MergeBranch error: %v", err)
	}
	_, msgs, _ := store.SnapshotWithEvents()
	if !containsMessage(msgs, "task-result") {
		t.Fatalf("expected merged message from task branch")
	}
}

func TestSetSummaryAndTrim_CreatesArchiveAndSectionLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}

	store.Append("user", "排查登录接口超时，先看网关日志")
	store.Append("assistant", "先确认 502 还是 504，并记录时间窗口")
	store.Append("user", "主要是 504，集中在 10:00-10:30")
	store.Append("assistant", "收到，我继续给修复步骤")

	store.SetSummaryAndTrim("当前摘要：登录接口超时处理中", 1)
	summary, messages, _ := store.SnapshotWithEvents()
	if len(messages) != 1 {
		t.Fatalf("expected 1 kept message after trim, got %d", len(messages))
	}
	if !strings.Contains(summary, archiveIndexBeginTag) {
		t.Fatalf("expected archive index block in summary, got %q", summary)
	}

	_, refs := splitSummaryArchiveRefs(summary)
	if len(refs) == 0 {
		t.Fatalf("expected at least one archive ref in summary")
	}
	index, err := store.ReadArchiveIndex(refs[0].ID)
	if err != nil {
		t.Fatalf("ReadArchiveIndex error: %v", err)
	}
	if len(index.Sections) == 0 {
		t.Fatalf("expected archive sections in index")
	}

	section, err := store.ReadArchiveSection(refs[0].ID, index.Sections[0].ID)
	if err != nil {
		t.Fatalf("ReadArchiveSection error: %v", err)
	}
	if strings.TrimSpace(section.Content) == "" {
		t.Fatalf("expected non-empty section content")
	}
}

func containsMessage(messages []Message, target string) bool {
	target = strings.TrimSpace(target)
	for _, msg := range messages {
		if strings.TrimSpace(msg.Content) == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
