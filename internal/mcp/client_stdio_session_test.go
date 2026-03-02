package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPClient_StdioReusesSessionAcrossCalls(t *testing.T) {
	if isWindows() {
		t.Skip("stdio mock script test is POSIX-only")
	}

	countFile := filepath.Join(t.TempDir(), "count.txt")
	script := writeStdioSessionScript(t, `#!/bin/sh
count_file="$COUNT_FILE"
count=0
if [ -f "$count_file" ]; then
  count=$(cat "$count_file")
fi
count=$((count + 1))
printf "%s" "$count" > "$count_file"

extract_id() {
  printf "%s" "$1" | sed -n 's/.*"id":[ ]*\([0-9][0-9]*\).*/\1/p'
}
while IFS= read -r line; do
  case "$line" in
    *\"method\":\"initialize\"*)
      id=$(extract_id "$line")
      [ -n "$id" ] || id=1
      echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":\"2025-06-18\"}}"
      ;;
    *\"method\":\"tools/list\"*)
      id=$(extract_id "$line")
      [ -n "$id" ] || id=2
      echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"tools\":[{\"name\":\"echo\",\"description\":\"echo\",\"inputSchema\":{\"type\":\"object\"}}]}}"
      ;;
    *\"method\":\"tools/call\"*)
      id=$(extract_id "$line")
      [ -n "$id" ] || id=3
      echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"ok\"}]}}"
      ;;
  esac
done
`)

	client := NewHTTPClient(3*time.Second, "")
	service := Service{
		ID:        "stdio_demo",
		Name:      "stdio_demo",
		Transport: "stdio",
		Command:   script,
		Env:       map[string]string{"COUNT_FILE": countFile},
		Enabled:   true,
	}

	tools, err := client.ListTools(context.Background(), service)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	result, err := client.CallTool(context.Background(), service, "echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}

	rawCount, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatalf("read count file: %v", err)
	}
	if got := strings.TrimSpace(string(rawCount)); got != "1" {
		t.Fatalf("expected one stdio process, got %q", got)
	}
}

func TestHTTPClient_StdioRecreatesSessionAfterTimeout(t *testing.T) {
	if isWindows() {
		t.Skip("stdio mock script test is POSIX-only")
	}

	tempDir := t.TempDir()
	countFile := filepath.Join(tempDir, "count.txt")
	onceFile := filepath.Join(tempDir, "once.txt")
	script := writeStdioSessionScript(t, `#!/bin/sh
count_file="$COUNT_FILE"
once_file="$ONCE_FILE"
count=0
if [ -f "$count_file" ]; then
  count=$(cat "$count_file")
fi
count=$((count + 1))
printf "%s" "$count" > "$count_file"

extract_id() {
  printf "%s" "$1" | sed -n 's/.*"id":[ ]*\([0-9][0-9]*\).*/\1/p'
}
while IFS= read -r line; do
  case "$line" in
    *\"method\":\"initialize\"*)
      id=$(extract_id "$line")
      [ -n "$id" ] || id=1
      echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"protocolVersion\":\"2025-06-18\"}}"
      ;;
    *\"method\":\"tools/call\"*)
      id=$(extract_id "$line")
      [ -n "$id" ] || id=2
      if [ ! -f "$once_file" ]; then
        touch "$once_file"
        sleep 1
      fi
      echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"ok\"}]}}"
      ;;
  esac
done
`)

	client := NewHTTPClient(3*time.Second, "")
	service := Service{
		ID:        "stdio_timeout_demo",
		Name:      "stdio_timeout_demo",
		Transport: "stdio",
		Command:   script,
		Env: map[string]string{
			"COUNT_FILE": countFile,
			"ONCE_FILE":  onceFile,
		},
		Enabled: true,
	}

	firstCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := client.CallTool(firstCtx, service, "echo", map[string]any{"text": "slow"})
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}

	result, err := client.CallTool(context.Background(), service, "echo", map[string]any{"text": "fast"})
	if err != nil {
		t.Fatalf("CallTool after timeout error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("unexpected result after rebuild: %+v", result)
	}

	rawCount, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatalf("read count file: %v", err)
	}
	if got := strings.TrimSpace(string(rawCount)); got != "2" {
		t.Fatalf("expected two stdio process starts after timeout rebuild, got %q", got)
	}
}

func writeStdioSessionScript(t *testing.T, content string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fake-mcp.sh")
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake stdio script: %v", err)
	}
	return script
}
