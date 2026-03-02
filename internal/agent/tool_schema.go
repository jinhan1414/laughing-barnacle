package agent

func strictToolParameters(
	properties map[string]any,
	required []string,
	nullableOptional []string,
) map[string]any {
	normalized := cloneSchemaMap(properties)
	requiredFields := make([]string, 0, len(required)+len(nullableOptional))
	requiredFields = append(requiredFields, required...)
	for _, name := range nullableOptional {
		prop, ok := normalized[name].(map[string]any)
		if !ok {
			continue
		}
		normalized[name] = nullableToolProperty(prop)
		requiredFields = append(requiredFields, name)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           normalized,
		"required":             requiredFields,
		"additionalProperties": false,
	}
}

func nullableToolProperty(schema map[string]any) map[string]any {
	return map[string]any{
		"anyOf": []any{
			cloneSchemaMap(schema),
			map[string]any{"type": "null"},
		},
	}
}

func cloneSchemaMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		nested, ok := value.(map[string]any)
		if !ok {
			out[key] = value
			continue
		}
		out[key] = cloneSchemaMap(nested)
	}
	return out
}
