package mcp

import (
	"net/http"
	"slices"
	"sort"
	"strings"
)

const authorizationHeaderName = "Authorization"

var protectedHTTPHeaderNames = map[string]struct{}{
	"accept":               {},
	"authorization":        {},
	"content-type":         {},
	"mcp-protocol-version": {},
	"mcp-session-id":       {},
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func normalizeServiceEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		nextKey := strings.TrimSpace(key)
		nextValue := strings.TrimSpace(value)
		if nextKey == "" || nextValue == "" {
			continue
		}
		out[nextKey] = nextValue
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeServiceHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		nextKey := http.CanonicalHeaderKey(strings.TrimSpace(key))
		nextValue := strings.TrimSpace(value)
		if nextKey == "" || nextValue == "" {
			continue
		}
		out[nextKey] = nextValue
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedStringMapKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeConfiguredEnv(base []string, env map[string]string) []string {
	if len(env) == 0 {
		return slices.Clone(base)
	}
	out := slices.Clone(base)
	for _, key := range sortedStringMapKeys(env) {
		out = append(out, key+"="+env[key])
	}
	return out
}

func applyConfiguredHeaders(header http.Header, configured map[string]string) {
	for _, key := range sortedStringMapKeys(configured) {
		if isProtectedHTTPHeader(key) {
			continue
		}
		header.Set(key, configured[key])
	}
}

func isProtectedHTTPHeader(key string) bool {
	_, found := protectedHTTPHeaderNames[strings.ToLower(strings.TrimSpace(key))]
	return found
}
