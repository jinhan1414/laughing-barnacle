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
	defaultAuthDirName      = ".codex"
	defaultAuthFileName     = "auth.json"
	authModeChatGPT         = "chatgpt"
	authProviderOpenAICodex = "openai-codex"
)

type authContext struct {
	Token     string
	AccountID string
	AuthMode  string
}

func (a *Adapter) resolveAuthContext() (authContext, error) {
	explicitPath := strings.TrimSpace(a.authFilePath)
	if explicitPath != "" {
		return readAuthContextFromFile(explicitPath)
	}
	if token := strings.TrimSpace(a.apiToken); token != "" {
		return authContext{Token: token}, nil
	}
	defaultPath, err := defaultAuthFilePath()
	if err != nil {
		return authContext{}, authPathError("", err)
	}
	return readAuthContextFromFile(defaultPath)
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

func readAuthContextFromFile(path string) (authContext, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return authContext{}, authPathError(path, err)
	}
	cred, err := parseAuthContext(content)
	if err != nil {
		return authContext{}, authPathError(path, err)
	}
	if strings.TrimSpace(cred.Token) == "" {
		return authContext{}, authPathError(path, fmt.Errorf("token not found"))
	}
	return cred, nil
}

func parseAuthContext(raw []byte) (authContext, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return authContext{}, fmt.Errorf("file is empty")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return authContext{Token: trimmed}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return authContext{}, fmt.Errorf("decode auth json: %w", err)
	}
	chatgpt := parseChatGPTAuth(decoded)
	if strings.TrimSpace(chatgpt.Token) != "" {
		return chatgpt, nil
	}
	token := parseAPIKeyAuth(decoded)
	if token != "" {
		return authContext{Token: token}, nil
	}
	return authContext{}, fmt.Errorf("no usable token fields found")
}

func parseChatGPTAuth(decoded map[string]any) authContext {
	authMode := strings.ToLower(strings.TrimSpace(stringValue(decoded["auth_mode"])))
	if authMode != authModeChatGPT {
		return authContext{}
	}
	tokens, ok := decoded["tokens"].(map[string]any)
	if !ok {
		return authContext{}
	}
	return authContext{
		Token:     strings.TrimSpace(stringValue(tokens["access_token"])),
		AccountID: strings.TrimSpace(stringValue(tokens["account_id"])),
		AuthMode:  authModeChatGPT,
	}
}

func parseAPIKeyAuth(decoded map[string]any) string {
	candidates := []any{
		decoded["OPENAI_API_KEY"],
		decoded["openai_api_key"],
		decoded["api_key"],
		decoded["token"],
		decoded["access_token"],
	}
	for _, candidate := range candidates {
		if token := strings.TrimSpace(stringValue(candidate)); token != "" {
			return token
		}
	}
	if tokens, ok := decoded["tokens"].(map[string]any); ok {
		if token := strings.TrimSpace(stringValue(tokens["access_token"])); token != "" {
			return token
		}
	}
	return ""
}

func stringValue(value any) string {
	if str, ok := value.(string); ok {
		return str
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
		Provider: authProviderOpenAICodex,
		Message:  message,
		Cause:    err,
	}
}
