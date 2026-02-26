package agent

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const windowsShellEnvKey = "AGENT_WINDOWS_SHELL"

var powerShellCurlCommandPattern = regexp.MustCompile(`(?i)(^|[;&|]\s*)curl(\s|$)`)

func preferredWindowsShellName() string {
	preferred := strings.ToLower(strings.TrimSpace(os.Getenv(windowsShellEnvKey)))
	if preferred == "" || preferred == "auto" {
		return firstAvailableShellName("pwsh", "powershell", "cmd")
	}
	if !isSupportedWindowsShell(preferred) {
		return firstAvailableShellName("pwsh", "powershell", "cmd")
	}
	if _, err := exec.LookPath(preferred); err == nil {
		return preferred
	}
	return firstAvailableShellName("pwsh", "powershell", "cmd")
}

func isSupportedWindowsShell(shell string) bool {
	return shell == "pwsh" || shell == "powershell" || shell == "cmd"
}

func firstAvailableShellName(candidates ...string) string {
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func normalizePowerShellCommand(command string) string {
	return powerShellCurlCommandPattern.ReplaceAllString(command, "${1}curl.exe${2}")
}
