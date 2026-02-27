package openaicodex

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llmgateway"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultAuthDirName  = ".codex"
	defaultAuthFileName = "auth.json"
)

func (a *Adapter) resolveAuthToken() (string, error) {
	explicitPath := strings.TrimSpace(a.authFilePath)
	if explicitPath != "" {
		return readTokenFromFile(explicitPath, true)
	}
	if token := strings.TrimSpace(a.apiToken); token != "" {
		return token, nil
	}
	defaultPath, err := defaultAuthFilePath()
	if err != nil {
		return "", authPathError("", err)
	}
	return readTokenFromFile(defaultPath, false)
}

func defaultAuthFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, defaultAuthDirName, defaultAuthFileName), nil
}

func readTokenFromFile(path string, explicit bool) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", authPathError(path, err)
	}
	token, err := parseToken(content)
	if err != nil {
		return "", authPathError(path, err)
	}
	if token != "" {
		return token, nil
	}
	if explicit {
		return "", authPathError(path, fmt.Errorf("token not found"))
	}
	return "", authPathError(path, fmt.Errorf("token not found in default auth file"))
}

func parseToken(raw []byte) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", fmt.Errorf("file is empty")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "\"") {
		var decoded any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
			return "", fmt.Errorf("decode auth json: %w", err)
		}
		return findToken(decoded), nil
	}
	return trimmed, nil
}

func findToken(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"api_key", "access_token", "token", "OPENAI_API_KEY", "openai_api_key"} {
			if token := findToken(v[key]); token != "" {
				return token
			}
		}
		for _, item := range v {
			switch item.(type) {
			case map[string]any, []any:
				if token := findToken(item); token != "" {
					return token
				}
			}
		}
	case []any:
		for _, item := range v {
			if token := findToken(item); token != "" {
				return token
			}
		}
	}
	return ""
}

func authPathError(path string, err error) error {
	message := "codex auth configuration is invalid"
	if p := strings.TrimSpace(path); p != "" {
		message = fmt.Sprintf("codex auth file is invalid: %s", p)
	}
	return &llmgateway.Error{
		Code:     llmgateway.ErrorCodeAuthConfigInvalid,
		Provider: providerName,
		Message:  message,
		Cause:    err,
	}
}
