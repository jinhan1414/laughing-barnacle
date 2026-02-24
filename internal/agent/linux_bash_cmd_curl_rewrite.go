package agent

import (
	"net/url"
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

	encoded := encodeFormFields(formFields)
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

func encodeFormFields(fields []string) string {
	pairs := make([]string, 0, len(fields))
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
			pairs = append(pairs, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}
	return strings.Join(pairs, "&")
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
