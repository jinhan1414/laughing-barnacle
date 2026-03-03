package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListProjectIndexLines_FallsBackToRootScan(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "alpha-app"), 0o755); err != nil {
		t.Fatalf("mkdir alpha-app: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "beta_app"), 0o755); err != nil {
		t.Fatalf("mkdir beta_app: %v", err)
	}
	agentSvc := &Agent{
		projectCfg: &mockProjectRoot{rootDir: rootDir},
	}

	lines := agentSvc.listProjectIndexLines(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 root-scan lines, got %#v", lines)
	}
	if !strings.Contains(lines[0], "summary=root-scan") {
		t.Fatalf("expected root-scan summary, got %q", lines[0])
	}
}
