package agent

import "testing"

func TestContextReadToolDefinition_UsesNullableOptionalFields(t *testing.T) {
	params := contextReadToolDefinition().Function.Parameters
	required := params["required"].([]string)
	if len(required) != 9 {
		t.Fatalf("expected all context__read fields required for strict schema, got %v", required)
	}
	properties := params["properties"].(map[string]any)
	if _, ok := properties["resource"].(map[string]any)["anyOf"]; ok {
		t.Fatalf("resource should remain non-nullable")
	}
	pathSchema, ok := properties["path"].(map[string]any)
	if !ok {
		t.Fatalf("path schema missing")
	}
	anyOf, ok := pathSchema["anyOf"].([]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("expected nullable anyOf for path, got %#v", pathSchema)
	}
}

func TestAsyncTaskToolDefinitions_UseNullableOptionalFields(t *testing.T) {
	submitParams := asyncTaskSubmitToolDefinition().Function.Parameters
	submitRequired := submitParams["required"].([]string)
	if len(submitRequired) != 8 {
		t.Fatalf("expected strict submit required list, got %v", submitRequired)
	}
	submitProps := submitParams["properties"].(map[string]any)
	if _, ok := submitProps["agent_id"].(map[string]any)["anyOf"]; !ok {
		t.Fatalf("agent_id should be nullable for strict schema")
	}
	metadataSchemaAnyOf, ok := submitProps["metadata"].(map[string]any)["anyOf"].([]any)
	if !ok || len(metadataSchemaAnyOf) != 2 {
		t.Fatalf("metadata should be nullable for strict schema, got %#v", submitProps["metadata"])
	}
	metadataSchema, ok := metadataSchemaAnyOf[0].(map[string]any)
	if !ok {
		t.Fatalf("metadata schema missing object definition")
	}
	metadataProps, ok := metadataSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("metadata properties missing")
	}
	if _, ok := metadataProps["working_dir"]; !ok {
		t.Fatalf("metadata should expose working_dir")
	}
	if _, ok := metadataProps["project_id"]; !ok {
		t.Fatalf("metadata should expose project_id")
	}

	cancelParams := asyncTaskCancelToolDefinition().Function.Parameters
	cancelProps := cancelParams["properties"].(map[string]any)
	if _, ok := cancelProps["reason"].(map[string]any)["anyOf"]; !ok {
		t.Fatalf("reason should be nullable for strict schema")
	}
}
