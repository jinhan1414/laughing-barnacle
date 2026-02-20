package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"laughing-barnacle/internal/memory"
)

func TestHandleAPIMemoryIndexReadAndSection(t *testing.T) {
	store, err := memory.NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.UpsertNode(memory.UpsertRequest{
		Mode:          "replace",
		Path:          "/projects/pay-refactor/overview",
		Type:          memory.NodeTypeFile,
		Title:         "项目概览",
		SchemaKind:    "project_overview",
		SchemaVersion: 1,
		Summary:       "支付重构灰度中",
		Sections: []memory.Section{
			{ID: "s1", Title: "状态", Digest: "稳定", Content: "当前灰度30%，成功率99.98%"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertNode error: %v", err)
	}

	s := &Server{memoryStore: store}

	indexReq := httptest.NewRequest(http.MethodGet, "/api/memory/index?path=/projects/pay-refactor", nil)
	indexRec := httptest.NewRecorder()
	s.handleAPIMemoryIndex(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/memory/read?path=/projects/pay-refactor/overview", nil)
	readRec := httptest.NewRecorder()
	s.handleAPIMemoryRead(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	sectionReq := httptest.NewRequest(http.MethodGet, "/api/memory/section?path=/projects/pay-refactor/overview&section_id=s1", nil)
	sectionRec := httptest.NewRecorder()
	s.handleAPIMemorySection(sectionRec, sectionReq)
	if sectionRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", sectionRec.Code, sectionRec.Body.String())
	}
	var payload struct {
		Section struct {
			ID string `json:"id"`
		} `json:"section"`
	}
	if err := json.Unmarshal(sectionRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if payload.Section.ID != "s1" {
		t.Fatalf("unexpected section id: %+v", payload)
	}
}

func TestHandleAPIMemoryUpsertMoveDelete(t *testing.T) {
	store, err := memory.NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	s := &Server{memoryStore: store}

	upsertBody := bytes.NewBufferString(`{"mode":"replace","path":"/goals/2026-q1","type":"file","title":"季度目标","schema_kind":"goal","schema_version":1,"summary":"Q1 目标"}`)
	upsertReq := httptest.NewRequest(http.MethodPost, "/api/memory/upsert", upsertBody)
	upsertRec := httptest.NewRecorder()
	s.handleAPIMemoryUpsert(upsertRec, upsertReq)
	if upsertRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", upsertRec.Code, upsertRec.Body.String())
	}

	moveBody := bytes.NewBufferString(`{"from_path":"/goals/2026-q1","to_path":"/goals/2026-q1-main"}`)
	moveReq := httptest.NewRequest(http.MethodPost, "/api/memory/move", moveBody)
	moveRec := httptest.NewRecorder()
	s.handleAPIMemoryMove(moveRec, moveReq)
	if moveRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", moveRec.Code, moveRec.Body.String())
	}

	deleteBody := bytes.NewBufferString(`{"path":"/goals/2026-q1-main","mode":"soft"}`)
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/memory/delete", deleteBody)
	deleteRec := httptest.NewRecorder()
	s.handleAPIMemoryDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestHandleAPIMemoryInboxReviewMaintenanceAndAudit(t *testing.T) {
	store, err := memory.NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.UpsertNode(memory.UpsertRequest{
		Mode:          "replace",
		Path:          "/inbox/pending/seg-demo-goals",
		Type:          memory.NodeTypeFile,
		Title:         "待审核 目标",
		SchemaKind:    "memory_candidate",
		SchemaVersion: 1,
		Summary:       "本段用户目标候选",
		Refs: []memory.Ref{
			{Kind: "target_path", Value: "/goals/active/seg-demo"},
			{Kind: "target_title", Value: "目标沉淀 seg-demo"},
			{Kind: "target_schema_kind", Value: "goal"},
			{Kind: "target_confidence", Value: "0.70"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert pending candidate error: %v", err)
	}

	s := &Server{memoryStore: store}

	inboxReq := httptest.NewRequest(http.MethodGet, "/api/memory/inbox?limit=10", nil)
	inboxRec := httptest.NewRecorder()
	s.handleAPIMemoryInbox(inboxRec, inboxReq)
	if inboxRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", inboxRec.Code, inboxRec.Body.String())
	}

	reviewBody := bytes.NewBufferString(`{"path":"/inbox/pending/seg-demo-goals","action":"confirm"}`)
	reviewReq := httptest.NewRequest(http.MethodPost, "/api/memory/inbox/review", reviewBody)
	reviewRec := httptest.NewRecorder()
	s.handleAPIMemoryInboxReview(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", reviewRec.Code, reviewRec.Body.String())
	}

	maintenanceReq := httptest.NewRequest(http.MethodPost, "/api/memory/maintenance/run", bytes.NewBuffer(nil))
	maintenanceRec := httptest.NewRecorder()
	s.handleAPIMemoryMaintenanceRun(maintenanceRec, maintenanceReq)
	if maintenanceRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", maintenanceRec.Code, maintenanceRec.Body.String())
	}

	updateBody := bytes.NewBufferString(`{"mode":"patch","path":"/goals/active/seg-demo","summary":"patched summary for rollback"}`)
	updateReq := httptest.NewRequest(http.MethodPost, "/api/memory/upsert", updateBody)
	updateRec := httptest.NewRecorder()
	s.handleAPIMemoryUpsert(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/memory/audit?limit=10", nil)
	auditRec := httptest.NewRecorder()
	s.handleAPIMemoryAudit(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/api/memory/metrics", nil)
	metricsRec := httptest.NewRecorder()
	s.handleAPIMemoryMetrics(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", metricsRec.Code, metricsRec.Body.String())
	}

	rollbackBody := bytes.NewBufferString(`{"path":"/goals/active/seg-demo"}`)
	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/memory/rollback", rollbackBody)
	rollbackRec := httptest.NewRecorder()
	s.handleAPIMemoryRollback(rollbackRec, rollbackReq)
	if rollbackRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rollbackRec.Code, rollbackRec.Body.String())
	}
}
