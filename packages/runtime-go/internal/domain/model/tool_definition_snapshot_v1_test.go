package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolDefinitionSnapshotRetainsOnlyActualModelVisibleDefinitionV1(t *testing.T) {
	schema := ToolSchema{Name: "read", Description: "ACTUAL_REQUEST", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`), OutputSchema: json.RawMessage(`{"PRIVATE_HOST":"output"}`), Source: "PRIVATE_HOST_SOURCE", TaskSupport: "PRIVATE_HOST_TASK"}
	definition := FreezeModelVisibleToolDefinitionsV1([]ToolSchema{schema})[schema.Name]
	want, _ := json.Marshal(schema)
	if definition == nil || definition.Truncated || definition.SchemaJSON != string(want) || ValidateToolDefinitionSnapshotV1(definition, schema.Name) != nil {
		t.Fatal("definition differs from the actual request")
	}
	if strings.Contains(definition.SchemaJSON, "PRIVATE_HOST") {
		t.Fatal("execution metadata entered the model-visible definition")
	}
	copy_ := CloneToolDefinitionSnapshotV1(definition)
	definition.SchemaJSON = "changed"
	if copy_.SchemaJSON != string(want) {
		t.Fatal("snapshot clone retained a mutable producer pointer")
	}
	if FreezeModelVisibleToolDefinitionsV1([]ToolSchema{schema, schema})[schema.Name] != nil {
		t.Fatal("ambiguous duplicate definition became historical evidence")
	}
	schema.Description = strings.Repeat("x", ToolDefinitionSnapshotLimitV1)
	oversized := FreezeModelVisibleToolDefinitionsV1([]ToolSchema{schema})[schema.Name]
	if oversized == nil || !oversized.Truncated || oversized.SchemaJSON != "" || ValidateToolDefinitionSnapshotV1(oversized, schema.Name) != nil {
		t.Fatal("oversized definition retained partial JSON or lost truncation")
	}
}

func TestToolDefinitionSnapshotRejectsHostFieldsAmbiguousAndPartialValuesV1(t *testing.T) {
	for _, raw := range []string{
		`{"name":"read","parameters":{}}`,
		`{"name":"read","description":null,"parameters":{}}`,
		`{"name":"read","description":"a","parameters":null}`,
		`{"name":"read","description":"a","parameters":[]}`,
		`{"name":"read","description":"a","parameters":{},"outputSchema":{}}`,
		`{"name":"read","description":"a","parameters":{},"ExecutionGrant":{}}`,
		`{"name":"read","name":"read","description":"a","parameters":{}}`,
		`{"name":"other","description":"a","parameters":{}}`,
		`{"name":"read","description":"a","parameters":`,
	} {
		if ValidateToolDefinitionSnapshotV1(&ToolDefinitionSnapshotV1{SchemaJSON: raw}, "read") == nil {
			t.Fatal("invalid model-visible definition accepted")
		}
	}
	if ValidateToolDefinitionSnapshotV1(nil, "read") != nil || ValidateToolDefinitionSnapshotV1(&ToolDefinitionSnapshotV1{SchemaJSON: `{"name":"read","description":"","parameters":{}}`}, "read") != nil {
		t.Fatal("unavailable or valid empty definition was rejected")
	}
	if ValidateToolDefinitionSnapshotV1(&ToolDefinitionSnapshotV1{SchemaJSON: `{}`, Truncated: true}, "read") == nil {
		t.Fatal("truncated definition retained bytes")
	}
}
