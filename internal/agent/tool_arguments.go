package agent

import "strings"

func readStringArgument(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func requireStringArgument(args map[string]any, key string) string {
	return readStringArgument(args, key)
}

func readObjectArgument(args map[string]any, key string) map[string]any {
	value, ok := args[key]
	if !ok {
		return nil
	}
	objectValue, ok := value.(map[string]any)
	if !ok || len(objectValue) == 0 {
		return nil
	}
	return objectValue
}
