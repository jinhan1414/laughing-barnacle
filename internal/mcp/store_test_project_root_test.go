package mcp

import (
	"path/filepath"
	"testing"
)

func TestStoreProjectRootDir_Persisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.SetProjectRootDir(`E:\projects\ai`); err != nil {
		t.Fatalf("SetProjectRootDir error: %v", err)
	}
	if got := store.GetProjectRootDir(); got != `E:\projects\ai` {
		t.Fatalf("unexpected root dir: %q", got)
	}
	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if got := reloaded.GetProjectRootDir(); got != `E:\projects\ai` {
		t.Fatalf("unexpected reloaded root dir: %q", got)
	}
}
