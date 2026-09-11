package toolcatalog

import (
	"encoding/json"
	"testing"
)

func TestCreatePlanToolSchemaPreservesPlanContract(t *testing.T) {
	schema := CreatePlanToolSchema()
	if schema.Name != ToolCreatePlanName || schema.Source != "plan" {
		t.Fatalf("plan tool identity drifted: %#v", schema)
	}
	var decoded struct {
		Required []string       `json:"required"`
		Props    map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &decoded); err != nil {
		t.Fatalf("decode plan schema: %v", err)
	}
	if len(decoded.Required) != 2 || decoded.Required[0] != "markdown" || decoded.Required[1] != "operation" {
		t.Fatalf("plan schema required fields drifted: %#v", decoded.Required)
	}
	if decoded.Props["plan_relative_path"] == nil || decoded.Props["plan_id"] == nil {
		t.Fatalf("plan schema lost GUI-owned path/id guards: %#v", decoded.Props)
	}
}
