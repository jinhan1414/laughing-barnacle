package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type llmGatewayEnv struct {
	DefaultProvider         string
	DefaultModel            string
	RequestTimeout          time.Duration
	CerberBaseURL           string
	CerberAPIKey            string
	CerberMaxRetries        int
	CerberRetryBaseDelay    time.Duration
	CerberRetryMaxDelay     time.Duration
	OpenAICodexBaseURL      string
	OpenAICodexAPIToken     string
	OpenAICodexAuthFilePath string
	OpenAICodexTransport    string
	OpenAICodexMaxRetries   int
	Warnings                []string
}

func loadLLMGatewayEnv() llmGatewayEnv {
	warnings := make([]string, 0, 8)
	defaultProvider := envStringWithLegacy("LLM_GATEWAY_DEFAULT_PROVIDER", "", "cerber", &warnings)
	defaultModel := envStringWithLegacy("LLM_GATEWAY_DEFAULT_MODEL", "CERBER_MODEL", "gpt-4o-mini", &warnings)

	return llmGatewayEnv{
		DefaultProvider: strings.ToLower(strings.TrimSpace(defaultProvider)),
		DefaultModel:    strings.TrimSpace(defaultModel),
		RequestTimeout:  envDurationWithLegacy("LLM_GATEWAY_REQUEST_TIMEOUT", "CERBER_TIMEOUT", 45*time.Second, &warnings),

		CerberBaseURL:           envStringWithLegacy("LLM_GATEWAY_CERBER_BASE_URL", "CERBER_BASE_URL", "https://api.cerber.ai", &warnings),
		CerberAPIKey:            envStringWithLegacy("LLM_GATEWAY_CERBER_API_KEY", "CERBER_API_KEY", "", &warnings),
		CerberMaxRetries:        envIntWithLegacy("LLM_GATEWAY_CERBER_MAX_RETRIES", "CERBER_MAX_RETRIES", 2, &warnings),
		CerberRetryBaseDelay:    envDurationWithLegacy("LLM_GATEWAY_CERBER_RETRY_BASE_DELAY", "CERBER_RETRY_BASE_DELAY", 700*time.Millisecond, &warnings),
		CerberRetryMaxDelay:     envDurationWithLegacy("LLM_GATEWAY_CERBER_RETRY_MAX_DELAY", "CERBER_RETRY_MAX_DELAY", 8*time.Second, &warnings),
		OpenAICodexBaseURL:      envStringWithLegacy("LLM_GATEWAY_OPENAI_CODEX_BASE_URL", "", "https://api.openai.com", &warnings),
		OpenAICodexAPIToken:     envStringWithLegacy("LLM_GATEWAY_OPENAI_CODEX_API_KEY", "", "", &warnings),
		OpenAICodexAuthFilePath: envStringWithLegacy("LLM_GATEWAY_OPENAI_CODEX_AUTH_FILE_PATH", "", "", &warnings),
		OpenAICodexTransport:    envStringWithLegacy("LLM_GATEWAY_OPENAI_CODEX_TRANSPORT", "", "auto", &warnings),
		OpenAICodexMaxRetries:   envIntWithLegacy("LLM_GATEWAY_OPENAI_CODEX_MAX_RETRIES", "", 1, &warnings),
		Warnings:                warnings,
	}
}

func envStringWithLegacy(primary, legacy, fallback string, warnings *[]string) string {
	primaryValue, primarySet := readNonEmptyEnv(primary)
	legacyValue, legacySet := readNonEmptyEnv(legacy)
	if primarySet {
		appendLegacyIgnoredWarning(legacy, primary, legacySet, warnings)
		return primaryValue
	}
	if legacySet {
		appendLegacyMappedWarning(legacy, primary, warnings)
		return legacyValue
	}
	return fallback
}

func envIntWithLegacy(primary, legacy string, fallback int, warnings *[]string) int {
	primaryValue, primarySet := readEnv(primary)
	legacyValue, legacySet := readEnv(legacy)
	if primarySet {
		appendLegacyIgnoredWarning(legacy, primary, legacySet && strings.TrimSpace(legacyValue) != "", warnings)
		if parsed, err := strconv.Atoi(strings.TrimSpace(primaryValue)); err == nil {
			return parsed
		}
		return fallback
	}
	if legacySet && strings.TrimSpace(legacyValue) != "" {
		appendLegacyMappedWarning(legacy, primary, warnings)
		if parsed, err := strconv.Atoi(strings.TrimSpace(legacyValue)); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDurationWithLegacy(primary, legacy string, fallback time.Duration, warnings *[]string) time.Duration {
	primaryValue, primarySet := readEnv(primary)
	legacyValue, legacySet := readEnv(legacy)
	if primarySet {
		appendLegacyIgnoredWarning(legacy, primary, legacySet && strings.TrimSpace(legacyValue) != "", warnings)
		if parsed, err := time.ParseDuration(strings.TrimSpace(primaryValue)); err == nil {
			return parsed
		}
		return fallback
	}
	if legacySet && strings.TrimSpace(legacyValue) != "" {
		appendLegacyMappedWarning(legacy, primary, warnings)
		if parsed, err := time.ParseDuration(strings.TrimSpace(legacyValue)); err == nil {
			return parsed
		}
	}
	return fallback
}

func appendLegacyMappedWarning(legacy, primary string, warnings *[]string) {
	if legacy == "" || primary == "" {
		return
	}
	*warnings = append(*warnings, fmt.Sprintf("legacy env %s is deprecated; mapped to %s", legacy, primary))
}

func appendLegacyIgnoredWarning(legacy, primary string, legacySet bool, warnings *[]string) {
	if legacy == "" || primary == "" || !legacySet {
		return
	}
	*warnings = append(*warnings, fmt.Sprintf("legacy env %s is ignored because %s is set", legacy, primary))
}

func readNonEmptyEnv(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	raw, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func readEnv(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	raw, ok := os.LookupEnv(key)
	return raw, ok
}
