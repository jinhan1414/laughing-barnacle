package agent

import (
	"net/url"
	"regexp"
	"strings"
)

func rewriteCmdCurlSettingsPost(command string) string {
	tokens := splitShellTokens(command)
	if len(tokens) < 2 || !strings.EqualFold(tokens[0], "curl") {
		return command
	}

	targetURL := ""
	formFields := make([]string, 0, 8)
	for i := 1; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			targetURL = token
		}
		if (lower == "--data-urlencode" || lower == "--data" || lower == "-d" || token == "-F") && i+1 < len(tokens) {
			i++
			formFields = append(formFields, strings.TrimSpace(tokens[i]))
			continue
		}
		if strings.HasPrefix(lower, "--data-urlencode=") {
			formFields = append(formFields, strings.TrimSpace(token[len("--data-urlencode="):]))
			continue
		}
		if strings.HasPrefix(lower, "--data=") {
			formFields = append(formFields, strings.TrimSpace(token[len("--data="):]))
			continue
		}
		if strings.HasPrefix(lower, "-d=") {
			formFields = append(formFields, strings.TrimSpace(token[len("-d="):]))
			continue
		}
		if strings.HasPrefix(token, "-F=") {
			formFields = append(formFields, strings.TrimSpace(token[len("-F="):]))
			continue
		}
	}

	if !isSettingsSaveURL(targetURL) || len(formFields) == 0 {
		return command
	}

	encoded := encodeFormFields(targetURL, formFields)
	if encoded == "" {
		return command
	}
	return "curl -sS -X POST " + targetURL + " -d \"" + encoded + "\" -D -"
}

func isSettingsSaveURL(raw string) bool {
	u := strings.ToLower(strings.TrimSpace(raw))
	if u == "" {
		return false
	}
	return strings.Contains(u, "/settings/skills/save") || strings.Contains(u, "/settings/schedules/save")
}

type formField struct {
	key   string
	value string
}

var skillIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func encodeFormFields(targetURL string, fields []string) string {
	pairs := parseFormFields(fields)
	pairs = normalizeSettingsSkillSaveFields(targetURL, pairs)
	if len(pairs) == 0 {
		return ""
	}
	encoded := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		key := strings.TrimSpace(pair.key)
		if key == "" {
			continue
		}
		value := strings.TrimSpace(pair.value)
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		encoded = append(encoded, url.QueryEscape(key)+"="+url.QueryEscape(value))
	}
	return strings.Join(encoded, "&")
}

func parseFormFields(fields []string) []formField {
	pairs := make([]formField, 0, len(fields))
	for _, raw := range fields {
		field := strings.Trim(strings.TrimSpace(raw), "\"")
		if field == "" {
			continue
		}
		for _, seg := range strings.Split(field, "&") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			key, value := splitKeyValue(seg)
			if key == "" {
				continue
			}
			pairs = append(pairs, formField{key: key, value: value})
		}
	}
	return pairs
}

func normalizeSettingsSkillSaveFields(targetURL string, fields []formField) []formField {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(targetURL)), "/settings/skills/save") {
		return fields
	}

	idValue := ""
	nameValue := ""
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field.key))
		switch key {
		case "id":
			idValue = strings.TrimSpace(field.value)
		case "name":
			nameValue = strings.TrimSpace(field.value)
		}
	}

	useIDAsName := skillIDPattern.MatchString(idValue) && (nameValue == "" || !skillIDPattern.MatchString(nameValue))
	out := make([]formField, 0, len(fields))
	nameUpdated := false
	for _, field := range fields {
		keyLower := strings.ToLower(strings.TrimSpace(field.key))
		if keyLower == "id" {
			continue
		}
		if keyLower == "name" && useIDAsName {
			field.value = idValue
			nameUpdated = true
		}
		out = append(out, field)
	}
	if useIDAsName && !nameUpdated {
		out = append(out, formField{key: "name", value: idValue})
	}
	return out
}

func splitKeyValue(raw string) (string, string) {
	idx := strings.Index(raw, "=")
	if idx < 0 {
		return "", ""
	}
	key := strings.TrimSpace(raw[:idx])
	value := ""
	if idx+1 < len(raw) {
		value = strings.TrimSpace(raw[idx+1:])
	}
	return key, value
}

func splitShellTokens(command string) []string {
	out := make([]string, 0, 16)
	var b strings.Builder
	inQuotes := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch ch {
		case '"':
			inQuotes = !inQuotes
		case ' ', '\t', '\r', '\n':
			if inQuotes {
				b.WriteByte(ch)
				continue
			}
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(ch)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
