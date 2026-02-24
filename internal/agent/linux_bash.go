package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"laughing-barnacle/internal/llm"
)

type linuxBashRequest struct {
	Command    string
	WorkDir    string
	TimeoutSec int
}

func linuxBashToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinLinuxBashToolName,
			Description: "Run one Linux shell command (prefer bash, fallback sh) and return stdout/stderr/exit_code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Bash command string to execute.",
					},
					"working_dir": map[string]any{
						"type":        "string",
						"description": "Optional working directory.",
					},
					"timeout_sec": map[string]any{
						"type":        "integer",
						"description": "Optional timeout in seconds, default 20, max 180.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
	}
}

func parseLinuxBashArguments(raw string) (linuxBashRequest, error) {
	args, err := readToolArguments(raw)
	if err != nil {
		return linuxBashRequest{}, err
	}

	commandRaw, ok := args["command"]
	if !ok {
		return linuxBashRequest{}, fmt.Errorf("tool argument %q is required", "command")
	}
	command, ok := commandRaw.(string)
	if !ok || strings.TrimSpace(command) == "" {
		return linuxBashRequest{}, fmt.Errorf("tool argument %q must be non-empty string", "command")
	}

	req := linuxBashRequest{
		Command:    strings.TrimSpace(command),
		TimeoutSec: defaultBashTimeoutSeconds,
	}
	if v, ok := readOptionalStringArgument(args, "working_dir"); ok {
		req.WorkDir = v
	}
	if rawTimeout, exists := args["timeout_sec"]; exists {
		timeout, ok := parsePositiveInt(rawTimeout)
		if !ok {
			return linuxBashRequest{}, fmt.Errorf("tool argument %q must be positive integer", "timeout_sec")
		}
		req.TimeoutSec = timeout
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = defaultBashTimeoutSeconds
	}
	if req.TimeoutSec > maxBashTimeoutSeconds {
		req.TimeoutSec = maxBashTimeoutSeconds
	}
	return req, nil
}

func runLinuxBash(ctx context.Context, req linuxBashRequest) (string, error) {
	timeout := time.Duration(req.TimeoutSec) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellName, err := buildShellCommand(runCtx, req.Command)
	if err != nil {
		return "", err
	}
	if wd := strings.TrimSpace(req.WorkDir); wd != "" {
		if abs, err := filepath.Abs(wd); err == nil {
			cmd.Dir = abs
		} else {
			cmd.Dir = wd
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			exitCode = 124
		} else {
			return "", fmt.Errorf("run bash command: %w", runErr)
		}
	}
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	if timedOut && exitCode == 0 {
		exitCode = 124
	}

	stdoutText := trimRunes(stdout.String(), maxBashStdoutRunes)
	stderrText := trimRunes(stderr.String(), maxBashStderrRunes)
	if shouldHintCmdCurlQuoteFix(shellName, exitCode, req.Command, stderrText) {
		stderrText = "curl 在 Windows cmd 下可能因引号转义失败；重试请使用 curl -sS \"http://127.0.0.1:9080/...\"（不要写 \\\"）。"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("exit_code: %d\n", exitCode))
	b.WriteString("shell: " + shellName + "\n")
	if cmd.Dir != "" {
		b.WriteString("working_dir: " + cmd.Dir + "\n")
	}
	if timedOut {
		b.WriteString("timed_out: true\n")
	}
	b.WriteString("stdout:\n")
	b.WriteString(safeOrEmpty(stdoutText))
	b.WriteString("\n")
	b.WriteString("stderr:\n")
	b.WriteString(safeOrEmpty(stderrText))
	return strings.TrimSpace(b.String()), nil
}

func buildShellCommand(ctx context.Context, command string) (*exec.Cmd, string, error) {
	shellName := preferredShellName()
	command = normalizeCommandForShell(shellName, command)
	switch shellName {
	case "bash":
		bashPath, _ := exec.LookPath("bash")
		return exec.CommandContext(ctx, bashPath, "-lc", command), "bash", nil
	case "sh":
		shPath, _ := exec.LookPath("sh")
		return exec.CommandContext(ctx, shPath, "-c", command), "sh", nil
	case "cmd":
		cmdPath, _ := exec.LookPath("cmd")
		return exec.CommandContext(ctx, cmdPath, "/C", command), "cmd", nil
	default:
		return nil, "", fmt.Errorf("run shell command: no bash/sh available in current environment")
	}
}

func normalizeCommandForShell(shellName, command string) string {
	command = strings.TrimSpace(command)
	if shellName != "cmd" {
		return command
	}
	command = strings.ReplaceAll(command, `\"`, `"`)
	command = strings.ReplaceAll(command, `\'`, `'`)
	return command
}

func shouldHintCmdCurlQuoteFix(shellName string, exitCode int, command, stderr string) bool {
	if shellName != "cmd" || exitCode != 3 {
		return false
	}
	if strings.TrimSpace(stderr) != "" {
		return false
	}
	return strings.Contains(strings.ToLower(command), "curl")
}

func preferredShellName() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("cmd"); err == nil {
			return "cmd"
		}
	}
	return ""
}

func readToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("tool arguments are required")
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	if args == nil {
		return nil, fmt.Errorf("tool arguments are required")
	}
	return args, nil
}

func readOptionalStringArgument(args map[string]any, key string) (string, bool) {
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func parsePositiveInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		if t <= 0 || t != float64(int(t)) {
			return 0, false
		}
		return int(t), true
	case int:
		if t <= 0 {
			return 0, false
		}
		return t, true
	default:
		return 0, false
	}
}
