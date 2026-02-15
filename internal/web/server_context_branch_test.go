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

func TestHandleAPIContextBranches_ReturnsCurrentAndBranches(t *testing.T) {
	convPath := filepath.Join(t.TempDir(), "conversation.db")
	convStore, err := conversation.NewStoreWithFile(convPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	if err := convStore.SwitchBranch("task/api"); err != nil {
		t.Fatalf("SwitchBranch error: %v", err)
	}
	if err := convStore.SwitchBranch("main"); err != nil {
		t.Fatalf("SwitchBranch main error: %v", err)
	}

	s := &Server{convStore: convStore}
	req := httptest.NewRequest(http.MethodGet, "/api/context/branches", nil)
	rec := httptest.NewRecorder()
	s.handleAPIContextBranches(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload struct {
		CurrentBranch string   `json:"current_branch"`
		Branches      []string `json:"branches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if payload.CurrentBranch != "main" {
		t.Fatalf("expected current_branch=main, got %q", payload.CurrentBranch)
	}
	if !containsWebString(payload.Branches, "task/api") {
		t.Fatalf("expected task/api in branches, got %v", payload.Branches)
	}
}

func TestHandleAPIContextBranchSwitch_AndMerge(t *testing.T) {
	convPath := filepath.Join(t.TempDir(), "conversation.db")
	convStore, err := conversation.NewStoreWithFile(convPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	convStore.Append("user", "main-base")
	if err := convStore.SwitchBranch("task/merge-api"); err != nil {
		t.Fatalf("SwitchBranch error: %v", err)
	}
	convStore.Append("assistant", "task-output")
	if err := convStore.SwitchBranch("main"); err != nil {
		t.Fatalf("SwitchBranch main error: %v", err)
	}

	s := &Server{convStore: convStore}

	switchReq := httptest.NewRequest(http.MethodPost, "/api/context/branch/switch", strings.NewReader("branch=task%2Fmerge-api"))
	switchReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	switchRec := httptest.NewRecorder()
	s.handleAPIContextBranchSwitch(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("expected switch status 200, got %d", switchRec.Code)
	}
	if current := convStore.CurrentBranch(); current != "task/merge-api" {
		t.Fatalf("expected switched branch task/merge-api, got %q", current)
	}

	backReq := httptest.NewRequest(http.MethodPost, "/api/context/branch/switch", strings.NewReader("branch=main"))
	backReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	backRec := httptest.NewRecorder()
	s.handleAPIContextBranchSwitch(backRec, backReq)
	if backRec.Code != http.StatusOK {
		t.Fatalf("expected switch-main status 200, got %d", backRec.Code)
	}

	mergeReq := httptest.NewRequest(http.MethodPost, "/api/context/branch/merge", strings.NewReader("source_branch=task%2Fmerge-api"))
	mergeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mergeRec := httptest.NewRecorder()
	s.handleAPIContextBranchMerge(mergeRec, mergeReq)
	if mergeRec.Code != http.StatusOK {
		t.Fatalf("expected merge status 200, got %d body=%s", mergeRec.Code, mergeRec.Body.String())
	}

	_, messages, _ := convStore.SnapshotWithEvents()
	found := false
	for _, msg := range messages {
		if strings.TrimSpace(msg.Content) == "task-output" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected merged task-output message in main branch")
	}
}

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

func containsWebString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
