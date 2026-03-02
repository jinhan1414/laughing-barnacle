package web

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func parseJSONStringMap(raw string, fieldLabel string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s 必须是 JSON 字符串对象，例如 {\"KEY\":\"VALUE\"}", fieldLabel)
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

func sortedMapKeys(values map[string]string) []string {
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

func joinSortedMapKeys(values map[string]string) string {
	return strings.Join(sortedMapKeys(values), ", ")
}
