package memory

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreProjectIndexAndResolveWorkdir(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.UpsertProject("laughing-barnacle", "laughing-barnacle", []string{"barnacle"}, "repo"); err != nil {
		t.Fatalf("UpsertProject error: %v", err)
	}
	lines := store.ListProjectIndexLines(10, `E:\projects\ai`)
	if len(lines) != 1 || !strings.Contains(lines[0], `working_dir=E:\projects\ai`) {
		t.Fatalf("unexpected project lines: %#v", lines)
	}
	workdir, found, err := store.ResolveProjectWorkingDir(`E:\projects\ai`, "laughing-barnacle")
	if err != nil {
		t.Fatalf("ResolveProjectWorkingDir error: %v", err)
	}
	if !found || !strings.Contains(workdir, `laughing-barnacle`) {
		t.Fatalf("unexpected resolve result found=%v workdir=%q", found, workdir)
	}
}
