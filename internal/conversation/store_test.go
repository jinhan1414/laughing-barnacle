package conversation

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
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

func TestStoreAppendAssistantUsage_PersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.Append("user", "hello")
	store.AppendAssistant("reply", &TokenUsage{
		PromptTokens:     120,
		CompletionTokens: 30,
		TotalTokens:      150,
		CachedTokens:     90,
	})

	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].Usage == nil {
		t.Fatalf("expected assistant usage")
	}
	if messages[1].Usage.TotalTokens != 150 {
		t.Fatalf("unexpected usage total: %+v", messages[1].Usage)
	}
	if messages[1].Usage.CachedTokens != 90 {
		t.Fatalf("unexpected usage cached: %+v", messages[1].Usage)
	}

	_ = store.Close()
	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })

	_, reloadedMessages := reloaded.Snapshot()
	if len(reloadedMessages) != 2 {
		t.Fatalf("expected 2 messages after reload, got %d", len(reloadedMessages))
	}
	if reloadedMessages[1].Usage == nil || reloadedMessages[1].Usage.TotalTokens != 150 {
		t.Fatalf("unexpected reloaded usage: %+v", reloadedMessages[1].Usage)
	}
	if reloadedMessages[1].Usage.CachedTokens != 90 {
		t.Fatalf("unexpected reloaded cached usage: %+v", reloadedMessages[1].Usage)
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
	if err := store.SaveAsyncTaskState([]byte(`[{"ID":"async_20260228_120000_1","TaskType":"generic","Status":"submitted"}]`)); err != nil {
		t.Fatalf("SaveAsyncTaskState error: %v", err)
	}

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	summary, messages, events := store.SnapshotWithEvents()
	if summary != "" || len(messages) != 0 || len(events) != 0 {
		t.Fatalf("expected empty store after reset")
	}
	raw, err := store.LoadAsyncTaskState()
	if err != nil {
		t.Fatalf("LoadAsyncTaskState error: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected async task state cleared after reset, got %q", string(raw))
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
	raw, err = reloaded.LoadAsyncTaskState()
	if err != nil {
		t.Fatalf("reloaded LoadAsyncTaskState error: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected persisted async task state cleared after reset, got %q", string(raw))
	}
}

func TestSetSummaryAndTrim_CreatesArchiveAndSectionLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

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

func TestReadArchiveSection_PreservesFullReplayContentForNewArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	longUserMessage := strings.Repeat("A", 320) + " 完整历史尾巴"
	store.Append("user", longUserMessage)
	store.Append("assistant", "收到，我先记录完整上下文")
	store.Append("user", "继续推进")
	store.Append("assistant", "好的")
	store.SetSummaryAndTrim("压缩摘要", 0)

	summary, _, _ := store.SnapshotWithEvents()
	_, refs := splitSummaryArchiveRefs(summary)
	if len(refs) == 0 {
		t.Fatalf("expected archive refs in summary, got %q", summary)
	}

	section, err := store.ReadArchiveSection(refs[0].ID, "S1")
	if err != nil {
		t.Fatalf("ReadArchiveSection error: %v", err)
	}
	if section.LegacyIncomplete {
		t.Fatalf("new archive section should not be marked legacy incomplete")
	}
	if len(section.Messages) == 0 {
		t.Fatalf("expected replay messages in archive section")
	}
	if section.Messages[0].Content != longUserMessage {
		t.Fatalf("expected full replay message, got %q", section.Messages[0].Content)
	}
	if !strings.Contains(section.Content, longUserMessage) {
		t.Fatalf("expected rendered content contains full message, got %q", section.Content)
	}
}

func TestReadArchiveSection_LegacyArchiveMarkedIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	record := archiveRecord{
		ID:                "arc-legacy-001",
		CreatedAt:         time.Now().UTC(),
		TrimmedMessageCnt: 2,
		KeySummary:        []string{"[S1] 旧摘要"},
		Sections: []archiveSectionItem{
			{
				ID:      "S1",
				Title:   "旧归档",
				Digest:  "旧摘要",
				Content: "1. [user] 旧格式内容",
			},
		},
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal legacy archive error: %v", err)
	}
	if err := store.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketArchives))
		return b.Put([]byte(record.ID), encoded)
	}); err != nil {
		t.Fatalf("write legacy archive error: %v", err)
	}

	section, err := store.ReadArchiveSection(record.ID, "S1")
	if err != nil {
		t.Fatalf("ReadArchiveSection error: %v", err)
	}
	if !section.LegacyIncomplete {
		t.Fatalf("legacy archive section should be marked incomplete")
	}
	if len(section.Messages) != 0 {
		t.Fatalf("legacy archive should not have replay messages, got %d", len(section.Messages))
	}
	if section.Content != "1. [user] 旧格式内容" {
		t.Fatalf("unexpected legacy content: %q", section.Content)
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
