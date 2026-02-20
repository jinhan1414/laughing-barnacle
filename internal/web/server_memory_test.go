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
