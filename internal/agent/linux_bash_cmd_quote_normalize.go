package agent

import (
	"regexp"
	"strings"
)

var (
	escapedCmdURLQuotePattern      = regexp.MustCompile(`\\\"(https?://[^\\"]+)\\\"`)
	escapedCmdHeaderQuotePattern   = regexp.MustCompile(`\\\"([A-Za-z0-9_-]+:[^\\"]+)\\\"`)
	escapedCmdFormDataQuotePattern = regexp.MustCompile(`\\\"([A-Za-z0-9_.%-]+=[^\\\"{}]*)\\\"`)
)

func normalizeEscapedCmdQuotes(command string) string {
	command = strings.ReplaceAll(command, `\'`, `'`)
	command = escapedCmdURLQuotePattern.ReplaceAllString(command, `"$1"`)
	command = escapedCmdHeaderQuotePattern.ReplaceAllString(command, `"$1"`)
	return escapedCmdFormDataQuotePattern.ReplaceAllString(command, `"$1"`)
}
