package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"laughing-barnacle/internal/llm"
)

type linuxBashRequest struct {
	Command string
}

func linuxBashToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinLinuxBashToolName,
			Description: "Run one shell command (prefer bash/sh; on Windows prefer PowerShell) and return stdout/stderr/exit_code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command string to execute.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
	}
}

func parseLinuxBashArguments(raw string) (linuxBashRequest, error) {
	command, err := parseCommandToolArgument(raw)
	if err != nil {
		return linuxBashRequest{}, err
	}
	return linuxBashRequest{Command: command}, nil
}

func runLinuxBash(ctx context.Context, req linuxBashRequest) (string, error) {
	if isLocalAPIShellCommand(req.Command) {
		return "", fmt.Errorf("local api access via linux__bash is forbidden; use context__read or maintenance__write")
	}

	timeout := time.Duration(defaultBashTimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellName, err := buildShellCommand(runCtx, req.Command)
	if err != nil {
		return "", err
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
	if aliasHint := localAPIEndpointAliasHint(req.Command); strings.TrimSpace(aliasHint) != "" {
		if strings.TrimSpace(stderrText) == "" {
			stderrText = aliasHint
		} else {
			stderrText += "\n" + aliasHint
		}
	}
	if redirectErr := extractRedirectErrorFromCurlHeaders(stdoutText); strings.TrimSpace(redirectErr) != "" {
		if strings.TrimSpace(stderrText) == "" {
			stderrText = redirectErr
		} else {
			stderrText += "\n" + redirectErr
		}
	}
	if shouldHintCmdCurlQuoteFix(shellName, exitCode, req.Command, stderrText) {
		stderrText = "curl 在 Windows cmd 下可能因引号转义失败；重试请使用 curl -sS \"http://127.0.0.1:9080/...\"（不要写 \\\"）。"
	}
	if shouldHintScheduleSaveReadback(shellName, exitCode, req.Command, stdoutText) {
		hint := "注意：/settings/schedules/save 在该环境通常返回重定向空响应；必须立即回读 GET /api/schedules 以确认是否真正写入成功。"
		if strings.TrimSpace(stderrText) == "" {
			stderrText = hint
		} else {
			stderrText += "\n" + hint
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("exit_code: %d\n", exitCode))
	b.WriteString("shell: " + shellName + "\n")
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
	case "pwsh":
		pwshPath, _ := exec.LookPath("pwsh")
		return exec.CommandContext(ctx, pwshPath, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command), "pwsh", nil
	case "powershell":
		psPath, _ := exec.LookPath("powershell")
		return exec.CommandContext(ctx, psPath, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command), "powershell", nil
	case "cmd":
		cmdPath, _ := exec.LookPath("cmd")
		return exec.CommandContext(ctx, cmdPath, "/C", command), "cmd", nil
	default:
		return nil, "", fmt.Errorf("run shell command: no available shell in current environment")
	}
}

func normalizeCommandForShell(shellName, command string) string {
	command = strings.TrimSpace(command)
	command = normalizeLocalAPIEndpointAliases(command)
	switch shellName {
	case "cmd":
		command = normalizeEscapedCmdQuotes(command)
		command = rewriteCmdCurlSettingsPost(command)
		command = appendCurlHeadersForSettings(command)
		return command
	case "powershell", "pwsh":
		return normalizePowerShellCommand(command)
	default:
		return command
	}
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

func shouldHintScheduleSaveReadback(shellName string, exitCode int, command, stdout string) bool {
	if shellName != "cmd" || exitCode != 0 {
		return false
	}
	if strings.TrimSpace(stdout) != "" {
		return false
	}
	command = strings.ToLower(strings.TrimSpace(command))
	if !strings.Contains(command, "curl") {
		return false
	}
	return strings.Contains(command, "/settings/schedules/save")
}

func preferredShellName() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	if runtime.GOOS == "windows" {
		return preferredWindowsShellName()
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

func parseCommandStringArgument(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("tool arguments must be non-empty command string")
	}

	var command string
	if err := json.Unmarshal([]byte(trimmed), &command); err == nil {
		command = strings.TrimSpace(command)
		if command == "" {
			return "", fmt.Errorf("tool arguments must be non-empty command string")
		}
		return command, nil
	}

	var jsonValue any
	if err := json.Unmarshal([]byte(trimmed), &jsonValue); err == nil {
		return "", fmt.Errorf("tool arguments must be JSON string command")
	}
	return trimmed, nil
}

func parseCommandToolArgument(raw string) (string, error) {
	if command, err := parseCommandStringArgument(raw); err == nil {
		return command, nil
	}

	args, err := readToolArguments(raw)
	if err != nil {
		return "", fmt.Errorf("tool argument %q is required", "command")
	}
	commandRaw, ok := args["command"]
	if !ok {
		return "", fmt.Errorf("tool argument %q is required", "command")
	}
	command, ok := commandRaw.(string)
	if !ok || strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("tool argument %q must be non-empty string", "command")
	}
	return strings.TrimSpace(command), nil
}

func isLocalAPIShellCommand(command string) bool {
	text := strings.ToLower(strings.TrimSpace(command))
	if text == "" {
		return false
	}
	if !strings.Contains(text, "/api/") {
		return false
	}
	if !strings.Contains(text, "curl") && !strings.Contains(text, "invoke-restmethod") {
		return false
	}
	if !strings.Contains(text, "127.0.0.1") &&
		!strings.Contains(text, "localhost") &&
		!strings.Contains(text, "0.0.0.0") {
		return false
	}

	blockedPrefixes := []string{
		"/api/mcp/services",
		"/api/skills",
		"/api/schedules",
		"/api/a2a/agents",
		"/api/memory/index",
		"/api/memory/read",
		"/api/memory/section",
	}
	for _, prefix := range blockedPrefixes {
		if strings.Contains(text, prefix) {
			return true
		}
	}
	return false
}
