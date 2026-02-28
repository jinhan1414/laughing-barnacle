package agent

import "strings"

var a2aDispatchPrefixes = []string{
	"调用 ",
	"请调用 ",
	"再次调用 ",
	"重新调用 ",
	"继续调用 ",
	"再次重新调用 ",
	"call ",
	"invoke ",
	"use ",
}

var a2aTurnWordPrefixes = []string{
	"再次",
	"重新",
	"继续",
	"再",
}

func normalizeA2ASubmitText(agentID, text string) string {
	trimmed := strings.TrimSpace(text)
	id := strings.TrimSpace(agentID)
	if trimmed == "" || id == "" {
		return trimmed
	}

	out := strings.ReplaceAll(trimmed, "`"+id+"`", id)
	for _, prefix := range a2aDispatchPrefixes {
		out = strings.ReplaceAll(out, prefix+id, "")
	}
	out = collapseWhitespaces(out)
	out = trimLeadingPunctuation(out)
	out = trimLeadingTurnWords(out)
	return strings.TrimSpace(out)
}

func normalizeA2ARequestAndInput(agentID, request, agentInput string) (string, string) {
	normalizedRequest := normalizeA2ASubmitText(agentID, request)
	if isMeaninglessA2AText(normalizedRequest) {
		normalizedRequest = strings.TrimSpace(request)
	}
	normalizedInput := normalizeA2ASubmitText(agentID, agentInput)
	if isMeaninglessA2AText(normalizedInput) {
		normalizedInput = strings.TrimSpace(agentInput)
	}
	return normalizedRequest, normalizedInput
}

func collapseWhitespaces(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func trimLeadingPunctuation(text string) string {
	out := strings.TrimSpace(text)
	for {
		trimmed := strings.TrimLeft(out, " ,，。:：;；-—")
		if trimmed == out {
			return out
		}
		out = strings.TrimSpace(trimmed)
	}
}

func trimLeadingTurnWords(text string) string {
	out := strings.TrimSpace(text)
	for {
		changed := false
		for _, prefix := range a2aTurnWordPrefixes {
			if strings.HasPrefix(out, prefix) {
				out = strings.TrimSpace(strings.TrimPrefix(out, prefix))
				changed = true
			}
		}
		if !changed {
			return out
		}
		out = trimLeadingPunctuation(out)
	}
}

func isMeaninglessA2AText(text string) bool {
	trimmed := strings.TrimSpace(trimLeadingPunctuation(text))
	if trimmed == "" {
		return true
	}
	switch trimmed {
	case "请", "请你", "请帮我":
		return true
	default:
		return false
	}
}
