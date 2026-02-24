package agent

import (
	"strings"
)

func pruneSummaryOverlap(summary string, references ...string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	refSegments := buildReferenceSegments(references...)
	if len(refSegments) == 0 {
		return summary
	}

	lines := strings.Split(summary, "\n")
	filtered := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			filtered = append(filtered, "")
			continue
		}
		if isRedundantSummaryLine(trimmed, refSegments) {
			continue
		}
		filtered = append(filtered, line)
	}

	collapsed := make([]string, 0, len(filtered))
	prevBlank := true
	for _, line := range filtered {
		if strings.TrimSpace(line) == "" {
			if prevBlank {
				continue
			}
			collapsed = append(collapsed, "")
			prevBlank = true
			continue
		}
		collapsed = append(collapsed, line)
		prevBlank = false
	}
	return strings.TrimSpace(strings.Join(collapsed, "\n"))
}

func buildReferenceSegments(references ...string) []string {
	seen := make(map[string]struct{}, 64)
	out := make([]string, 0, 64)
	add := func(raw string) {
		normalized := normalizeComparableText(raw)
		if len([]rune(normalized)) < 6 {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	for _, text := range references {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		add(text)
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			add(line)
			if value, ok := extractComparableValue(line); ok {
				add(value)
			}
		}
	}
	return out
}

func isRedundantSummaryLine(line string, refSegments []string) bool {
	lineNorm := normalizeComparableText(line)
	if lineNorm == "" {
		return false
	}

	if isIdentityHeadingLine(lineNorm) {
		return true
	}

	candidates := []string{lineNorm}
	if value, ok := extractComparableValue(line); ok {
		if valueNorm := normalizeComparableText(value); len([]rune(valueNorm)) >= 6 && valueNorm != lineNorm {
			candidates = append(candidates, valueNorm)
		}
	}

	for _, candidate := range candidates {
		if len([]rune(candidate)) < 6 {
			continue
		}
		for _, ref := range refSegments {
			if candidate == ref || strings.Contains(ref, candidate) {
				return true
			}
		}
	}
	return false
}

func isIdentityHeadingLine(normalized string) bool {
	switch normalized {
	case "人格与沟通偏好", "核心身份与风格", "身份与风格", "沟通偏好", "人格设定":
		return true
	default:
		return false
	}
}

func extractComparableValue(line string) (string, bool) {
	cleaned := strings.TrimSpace(line)
	cleaned = strings.TrimLeft(cleaned, "-*• \t")
	cleaned = numberedListPrefixPattern.ReplaceAllString(cleaned, "")
	parts := strings.SplitN(cleaned, "：", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(cleaned, ":", 2)
		if len(parts) != 2 {
			return "", false
		}
	}

	label := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if label == "" || value == "" {
		return "", false
	}
	if len([]rune(label)) > 12 {
		return "", false
	}
	return value, true
}

func normalizeComparableText(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "-*• \t")
	text = numberedListPrefixPattern.ReplaceAllString(text, "")
	text = comparableStripPattern.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}
