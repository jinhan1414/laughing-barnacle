package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestHandleAPIContextArchiveIndexAndSection(t *testing.T) {
	convPath := filepath.Join(t.TempDir(), "conversation.db")
	convStore, err := conversation.NewStoreWithFile(convPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	convStore.Append("user", "排查支付回调签名失败")
	convStore.Append("assistant", "先确认网关时间偏差和密钥版本")
	convStore.Append("user", "发现部分实例密钥版本滞后")
	convStore.SetSummaryAndTrim("支付回调异常处理中", 1)

	summary, _, _ := convStore.SnapshotWithEvents()
	archiveID := extractArchiveIDFromSummary(summary)
	if strings.TrimSpace(archiveID) == "" {
		t.Fatalf("expected archive refs in summary, got %q", summary)
	}

	s := &Server{convStore: convStore}
	indexReq := httptest.NewRequest(http.MethodGet, "/api/context/archive/index?archive_id="+archiveID, nil)
	indexRec := httptest.NewRecorder()
	s.handleAPIContextArchiveIndex(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("expected index status 200, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}

	var indexPayload struct {
		ArchiveID string `json:"archive_id"`
		Sections  []struct {
			ID string `json:"id"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(indexRec.Body.Bytes(), &indexPayload); err != nil {
		t.Fatalf("decode index response error: %v", err)
	}
	if strings.TrimSpace(indexPayload.ArchiveID) == "" || len(indexPayload.Sections) == 0 {
		t.Fatalf("unexpected index payload: %+v", indexPayload)
	}

	sectionReq := httptest.NewRequest(
		http.MethodGet,
		"/api/context/archive/section?archive_id="+indexPayload.ArchiveID+"&section_id="+indexPayload.Sections[0].ID,
		nil,
	)
	sectionRec := httptest.NewRecorder()
	s.handleAPIContextArchiveSection(sectionRec, sectionReq)
	if sectionRec.Code != http.StatusOK {
		t.Fatalf("expected section status 200, got %d body=%s", sectionRec.Code, sectionRec.Body.String())
	}
	var sectionPayload struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(sectionRec.Body.Bytes(), &sectionPayload); err != nil {
		t.Fatalf("decode section response error: %v", err)
	}
	if strings.TrimSpace(sectionPayload.ID) == "" || strings.TrimSpace(sectionPayload.Content) == "" {
		t.Fatalf("unexpected section payload: %+v", sectionPayload)
	}
}

func extractArchiveIDFromSummary(summary string) string {
	marker := "archive_id="
	idx := strings.Index(summary, marker)
	if idx < 0 {
		return ""
	}
	rest := summary[idx+len(marker):]
	end := strings.IndexAny(rest, " |\n\r\t")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}
