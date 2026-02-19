package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreUpsertReadAndList(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first, err := store.UpsertProject(Project{
		Name:    "支付重构",
		Goal:    "统一支付网关并提升回调稳定性",
		Status:  "进行中",
		Summary: "已完成网关改造，正在灰度回调签名链路。",
		KeyFacts: []string{
			"Q1 完成主链路迁移",
			"Q1 完成主链路迁移",
		},
		Todos: []string{"补齐回归测试", "补齐回归测试"},
	})
	if err != nil {
		t.Fatalf("UpsertProject error: %v", err)
	}
	if strings.TrimSpace(first.ID) == "" {
		t.Fatalf("expected generated project id")
	}
	if len(first.KeyFacts) != 1 {
		t.Fatalf("expected deduped key facts, got %d", len(first.KeyFacts))
	}
	if len(first.Todos) != 1 {
		t.Fatalf("expected deduped todos, got %d", len(first.Todos))
	}

	reloaded, ok := store.ReadProject(first.ID)
	if !ok {
		t.Fatalf("expected project readable by id")
	}
	if reloaded.Name != "支付重构" {
		t.Fatalf("unexpected project name: %q", reloaded.Name)
	}

	updated, err := store.UpsertProject(Project{
		Name:       "支付重构",
		Status:     "已灰度",
		Milestones: []string{"M1: 网关接入完成"},
	})
	if err != nil {
		t.Fatalf("second UpsertProject error: %v", err)
	}
	if updated.ID != first.ID {
		t.Fatalf("expected same id by name, got %q != %q", updated.ID, first.ID)
	}
	if updated.Status != "已灰度" {
		t.Fatalf("unexpected status: %q", updated.Status)
	}
	if len(updated.Milestones) != 1 {
		t.Fatalf("expected milestones written")
	}

	items := store.ListProjects()
	if len(items) != 1 {
		t.Fatalf("expected one project, got %d", len(items))
	}
	if items[0].ID != first.ID {
		t.Fatalf("unexpected listed id: %q", items[0].ID)
	}
}

func TestStorePersistAcrossReload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "projects.db")
	store, err := NewStoreWithFile(dbPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	created, err := store.UpsertProject(Project{
		Name:   "客服机器人升级",
		Status: "待开始",
	})
	if err != nil {
		t.Fatalf("UpsertProject error: %v", err)
	}
	_ = store.Close()

	reloaded, err := NewStoreWithFile(dbPath)
	if err != nil {
		t.Fatalf("reload NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })

	project, ok := reloaded.ReadProject(created.ID)
	if !ok {
		t.Fatalf("expected project after reload")
	}
	if project.Name != "客服机器人升级" {
		t.Fatalf("unexpected project name: %q", project.Name)
	}
}

func TestStoreRejectsMissingProjectName(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.UpsertProject(Project{Status: "进行中"}); err == nil {
		t.Fatalf("expected validation error for missing name")
	}
}
