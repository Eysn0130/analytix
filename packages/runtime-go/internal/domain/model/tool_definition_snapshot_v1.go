package model

import (
	"encoding/json"
	"errors"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

// New private display limit, aligned with the existing protected Web JSON
// budget. Oversized definitions are unavailable with Truncated=true; the
// provider request itself is unchanged and no spill storage is created.
const ToolDefinitionSnapshotLimitV1 = 64 << 10

// ToolDefinitionSnapshotV1 is a private observation of the definition sent in
// one model request. It is display evidence, never an execution grant schema.
// Nil means unavailable; a truncated observation never retains partial JSON.
type ToolDefinitionSnapshotV1 struct {
	SchemaJSON string `json:"schemaJSON,omitempty"`
	Truncated  bool   `json:"truncated"`
}

func ValidateToolDefinitionSnapshotV1(value *ToolDefinitionSnapshotV1, toolName string) error {
	if value == nil {
		return nil
	}
	if value.Truncated {
		if value.SchemaJSON != "" {
			return errors.New("truncated tool definition contains bytes")
		}
		return nil
	}
	schema, err := domainjsonstrict.DecodeObject([]byte(value.SchemaJSON), domainjsonstrict.Options{RequireObject: true, MaxBytes: ToolDefinitionSnapshotLimitV1, MaxDepth: 64, MaxTokens: 200_000, MaxStringBytes: ToolDefinitionSnapshotLimitV1})
	if err != nil || len(schema) != 3 {
		return errors.New("model-visible tool definition is invalid")
	}
	name, named := schema["name"].(string)
	_, described := schema["description"].(string)
	_, parameterized := schema["parameters"].(map[string]any)
	if !named || name != toolName || !described || !parameterized {
		return errors.New("model-visible tool definition identity is invalid")
	}
	return nil
}

func CloneToolDefinitionSnapshotV1(value *ToolDefinitionSnapshotV1) *ToolDefinitionSnapshotV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	return &copy_
}

// Freeze before dispatch: marshaling ToolSchema retains only its model-visible
// name, description and parameters, excluding all Host execution metadata.
func FreezeModelVisibleToolDefinitionsV1(schemas []ToolSchema) map[string]*ToolDefinitionSnapshotV1 {
	definitions := make(map[string]*ToolDefinitionSnapshotV1, len(schemas))
	seen := map[string]bool{}
	for _, schema := range schemas {
		if seen[schema.Name] {
			definitions[schema.Name] = nil
			continue
		}
		seen[schema.Name] = true
		body, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		value := &ToolDefinitionSnapshotV1{SchemaJSON: string(body)}
		if len(body) > ToolDefinitionSnapshotLimitV1 {
			value.SchemaJSON, value.Truncated = "", true
		}
		if ValidateToolDefinitionSnapshotV1(value, schema.Name) == nil {
			definitions[schema.Name] = value
		}
	}
	return definitions
}
