package memory

import (
	"fmt"
	"laughing-barnacle/internal/conversation"
	"sort"
	"strings"
	"time"
)

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(strings.ToLower(path))
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("path must start with /")
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	if path == "/" {
		return path, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts {
		if !pathSegPattern.MatchString(part) {
			return "", fmt.Errorf("invalid path segment: %s", part)
		}
	}
	return path, nil
}

func parentPath(path string) string {
	if path == "/" {
		return ""
	}
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}

func pathTitle(path string) string {
	if path == "/" {
		return "root"
	}
	seg := path[strings.LastIndex(path, "/")+1:]
	if seg == "" {
		return "root"
	}
	return strings.ReplaceAll(seg, "-", " ")
}

func buildNodeID(path string, now time.Time) string {
	return fmt.Sprintf("mem-%s-%d", strings.TrimPrefix(strings.ReplaceAll(path, "/", "-"), "-"), now.UnixNano())
}

func buildSegmentID(now time.Time) string {
	return fmt.Sprintf("seg-%s-%d", now.UTC().Format("20060102-150405"), now.UTC().UnixNano())
}

func normalizeStringList(in []string, limit int, maxRunes int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := trimRunes(raw, maxRunes)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeSections(in []Section) []Section {
	if len(in) == 0 {
		return nil
	}
	out := make([]Section, 0, len(in))
	for i, sec := range in {
		id := strings.TrimSpace(sec.ID)
		if id == "" {
			id = fmt.Sprintf("s%d", i+1)
		}
		title := trimRunes(sec.Title, 80)
		if title == "" {
			title = fmt.Sprintf("section %d", i+1)
		}
		digest := trimRunes(sec.Digest, 120)
		content := trimRunes(sec.Content, 2000)
		if content == "" {
			continue
		}
		if digest == "" {
			digest = trimRunes(content, 80)
		}
		out = append(out, Section{ID: id, Title: title, Digest: digest, Content: content})
	}
	return out
}

func normalizeRefs(in []Ref) []Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]Ref, 0, len(in))
	for _, ref := range in {
		kind := trimRunes(ref.Kind, 40)
		value := trimRunes(ref.Value, 200)
		if kind == "" || value == "" {
			continue
		}
		out = append(out, Ref{Kind: kind, Value: value})
	}
	return out
}

func normalizePathList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, path := range in {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeToolCalls(in []conversation.ToolCall) []conversation.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]conversation.ToolCall, 0, len(in))
	for _, call := range in {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		call.Name = name
		call.Arguments = trimRunes(call.Arguments, 400)
		call.Result = trimRunes(call.Result, 400)
		call.Error = trimRunes(call.Error, 200)
		out = append(out, call)
	}
	return out
}

func trimRunes(input string, max int) string {
	input = strings.TrimSpace(input)
	if max <= 0 || input == "" {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func clamp(value, minV, maxV float64) float64 {
	if value < minV {
		return minV
	}
	if value > maxV {
		return maxV
	}
	return value
}

func encodePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "-")
	if path == "" {
		return "root"
	}
	return path
}
