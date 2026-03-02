package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestHandleAPIChatArchiveIndexAndSection(t *testing.T) {
	store := newArchiveBackedStore(t)
	s := &Server{convStore: store}

	indexReq := httptest.NewRequest(http.MethodGet, "/api/chat/archive/index", nil)
	indexRec := httptest.NewRecorder()
	s.handleAPIChatArchiveIndex(indexRec, indexReq)

	if indexRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for archive index, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}
	var indexPayload struct {
		Archives []apiChatArchiveIndexItem `json:"archives"`
	}
	if err := json.Unmarshal(indexRec.Body.Bytes(), &indexPayload); err != nil {
		t.Fatalf("decode archive index error: %v body=%s", err, indexRec.Body.String())
	}
	if len(indexPayload.Archives) == 0 {
		t.Fatalf("expected at least one archive in index")
	}
	first := indexPayload.Archives[0]
	if len(first.Sections) == 0 {
		t.Fatalf("expected archive sections in index, got %+v", first)
	}

	sectionURL := "/api/chat/archive/section?archive_id=" + first.ArchiveID + "&section_id=" + first.Sections[0].ID
	sectionReq := httptest.NewRequest(http.MethodGet, sectionURL, nil)
	sectionRec := httptest.NewRecorder()
	s.handleAPIChatArchiveSection(sectionRec, sectionReq)
	if sectionRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for archive section, got %d body=%s", sectionRec.Code, sectionRec.Body.String())
	}

	var sectionPayload apiChatArchiveSectionResponse
	if err := json.Unmarshal(sectionRec.Body.Bytes(), &sectionPayload); err != nil {
		t.Fatalf("decode archive section error: %v body=%s", err, sectionRec.Body.String())
	}
	if sectionPayload.LegacyIncomplete {
		t.Fatalf("new archive section should not be legacy incomplete")
	}
	if !strings.Contains(sectionPayload.Content, "排查登录接口超时") {
		t.Fatalf("expected section content to include full message, got %q", sectionPayload.Content)
	}
}

func TestHandleAPIChatArchiveSection_InvalidAndNotFound(t *testing.T) {
	store := newArchiveBackedStore(t)
	s := &Server{convStore: store}

	badReq := httptest.NewRequest(http.MethodGet, "/api/chat/archive/section?archive_id=abc", nil)
	badRec := httptest.NewRecorder()
	s.handleAPIChatArchiveSection(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid params, got %d body=%s", badRec.Code, badRec.Body.String())
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, "/api/chat/archive/section?archive_id=missing&section_id=S1", nil)
	notFoundRec := httptest.NewRecorder()
	s.handleAPIChatArchiveSection(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing archive, got %d body=%s", notFoundRec.Code, notFoundRec.Body.String())
	}
}

func TestHandleChatPage_RendersArchiveAsChatStreamScript(t *testing.T) {
	s := &Server{
		convStore: conversation.NewStore(),
		tmpl:      template.Must(template.ParseFS(embeddedTemplates, "templates/*.html")),
	}
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()
	s.handleChatPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `id="chat-archive-panel"`) {
		t.Fatalf("did not expect archive panel container in chat page")
	}
	if !strings.Contains(body, `archiveContainer.id = "chat-archive-stream"`) {
		t.Fatalf("expected archive chat stream script in chat page")
	}
	if !strings.Contains(body, `new EventSource("/api/chat/stream?since_us="`) {
		t.Fatalf("expected chat page to open sse stream")
	}
	if !strings.Contains(body, `id="chat-runtime-status"`) {
		t.Fatalf("expected runtime status container in chat page")
	}
}

func newArchiveBackedStore(t *testing.T) *conversation.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conversation.db")
	store, err := conversation.NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.Append("user", strings.Repeat("排查登录接口超时 ", 20))
	store.Append("assistant", "先看网关和上游日志")
	store.Append("user", "时间范围是 10:00-10:30")
	store.Append("assistant", "收到，继续排查")
	store.SetSummaryAndTrim("压缩摘要", 1)
	return store
}
