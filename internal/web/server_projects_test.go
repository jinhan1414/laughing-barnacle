package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/project"
)

func TestHandleAPIProjects_ListReadAndUpsert(t *testing.T) {
	store, err := project.NewStoreWithFile(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	s := &Server{projectStore: store}

	form := url.Values{}
	form.Set("name", "支付重构")
	form.Set("goal", "统一支付回调链路")
	form.Set("status", "进行中")
	form.Set("todos", "补齐回归\n推进灰度")
	upsertReq := httptest.NewRequest(http.MethodPost, "/api/projects/upsert", strings.NewReader(form.Encode()))
	upsertReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	upsertRec := httptest.NewRecorder()
	s.handleAPIProjectUpsert(upsertRec, upsertReq)
	if upsertRec.Code != http.StatusOK {
		t.Fatalf("expected upsert status 200, got %d body=%s", upsertRec.Code, upsertRec.Body.String())
	}

	var upsertPayload struct {
		Project struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Todos []string `json:"todos"`
		} `json:"project"`
	}
	if err := json.Unmarshal(upsertRec.Body.Bytes(), &upsertPayload); err != nil {
		t.Fatalf("decode upsert response error: %v", err)
	}
	if strings.TrimSpace(upsertPayload.Project.ID) == "" {
		t.Fatalf("expected generated project id")
	}
	if upsertPayload.Project.Name != "支付重构" {
		t.Fatalf("unexpected project name: %q", upsertPayload.Project.Name)
	}
	if len(upsertPayload.Project.Todos) != 2 {
		t.Fatalf("expected two todos, got %d", len(upsertPayload.Project.Todos))
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	listRec := httptest.NewRecorder()
	s.handleAPIProjects(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listPayload struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response error: %v", err)
	}
	if len(listPayload.Projects) != 1 {
		t.Fatalf("expected one project, got %d", len(listPayload.Projects))
	}
	if strings.Contains(listRec.Body.String(), `"todos"`) {
		t.Fatalf("expected index response without full details, got %s", listRec.Body.String())
	}

	indexReq := httptest.NewRequest(http.MethodGet, "/api/projects/index", nil)
	indexRec := httptest.NewRecorder()
	s.handleAPIProjects(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("expected index status 200, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/projects/read?id="+upsertPayload.Project.ID, nil)
	readRec := httptest.NewRecorder()
	s.handleAPIProjectRead(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}
	var readPayload struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(readRec.Body.Bytes(), &readPayload); err != nil {
		t.Fatalf("decode read response error: %v", err)
	}
	if readPayload.ID != upsertPayload.Project.ID {
		t.Fatalf("unexpected read id: %q", readPayload.ID)
	}
	if readPayload.Status != "进行中" {
		t.Fatalf("unexpected read status: %q", readPayload.Status)
	}
}
