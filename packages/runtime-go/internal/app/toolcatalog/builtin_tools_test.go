package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBuiltinToolSchemasPreserveSourceAndBackgroundBashBoundary(t *testing.T) {
	tools := BuiltinToolSchemas(BuiltinToolSchemaInput{AllowBackgroundBash: true, WebFetch: true, NativeSelections: true})
	if len(tools) != 21 {
		t.Fatalf("builtin schema count drifted: %d", len(tools))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Source != "builtin" {
			t.Fatalf("tool %s missing builtin source: %#v", tool.Name, tool)
		}
		seen[tool.Name] = true
	}
	for _, name := range []string{"native_selection_read", "native_selection_propose", "read_task_history", "read", "bash", "write", "edit", "grep", "find", "glob", "code_index", "ls", "read_file", "write_file", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol", "web_fetch"} {
		if !seen[name] {
			t.Fatalf("missing builtin tool schema %s", name)
		}
	}
	if !schemaHasProperty(t, schemaByName(t, tools, "bash"), "run_in_background") {
		t.Fatalf("foreground bash schema should allow background when configured")
	}
	foregroundOnly := BuiltinToolSchemas(BuiltinToolSchemaInput{AllowBackgroundBash: false, WebFetch: false, NativeSelections: true})
	if len(foregroundOnly) != 20 {
		t.Fatalf("foreground-only schema count drifted: %d", len(foregroundOnly))
	}
	if schemaHasProperty(t, schemaByName(t, foregroundOnly, "bash"), "run_in_background") {
		t.Fatalf("subagent/disabled bash schema must not expose background execution")
	}
	if description := schemaByName(t, tools, "bash").Description; !strings.Contains(description, "exact command") ||
		!strings.Contains(description, "copy it unchanged") || !strings.Contains(description, "Do not add timeout") ||
		!strings.Contains(description, "Windows uses PowerShell") || !strings.Contains(description, "cmd.exe") {
		t.Fatalf("bash description should steer Windows shell syntax: %q", description)
	}
	for _, schemas := range [][]domainmodel.ToolSchema{tools, foregroundOnly} {
		timeoutDescription := schemaPropertyDescription(t, schemaByName(t, schemas, "bash"), "timeout")
		if !strings.Contains(timeoutDescription, "default and maximum are 120 seconds") ||
			!strings.Contains(timeoutDescription, "Omit unless the user explicitly requests") {
			t.Fatalf("bash timeout description must preserve exact user arguments: %q", timeoutDescription)
		}
	}
	for _, property := range []string{"run_in_background", "runInBackground"} {
		description := schemaPropertyDescription(t, schemaByName(t, tools, "bash"), property)
		if !strings.Contains(description, "Omit unless the user explicitly requests") {
			t.Fatalf("bash background description must preserve exact user arguments: %q", description)
		}
	}
	if schemaByNameOptional(foregroundOnly, "web_fetch") != nil {
		t.Fatalf("web_fetch should only be advertised when web access is enabled")
	}
}

func TestBuiltinReadToolDescriptionsMentionConfiguredReadRoots(t *testing.T) {
	tools := BuiltinToolSchemas(BuiltinToolSchemaInput{AllowBackgroundBash: true, WebFetch: true})
	for _, name := range []string{"read", "grep", "find", "glob", "code_index", "ls", "read_file"} {
		description := schemaByName(t, tools, name).Description
		if !strings.Contains(description, "configured read root") {
			t.Fatalf("%s description should mention configured read roots: %q", name, description)
		}
	}
}

func TestFilterToolSchemasKeepsRequestedOrder(t *testing.T) {
	tools := BuiltinToolSchemas(BuiltinToolSchemaInput{AllowBackgroundBash: true, WebFetch: true})
	filtered := FilterToolSchemas(tools, []string{"grep", "read"})
	if len(filtered) != 2 || filtered[0].Name != "read" || filtered[1].Name != "grep" {
		t.Fatalf("filter should preserve advertised tool order: %#v", filtered)
	}
}

func schemaByName(t *testing.T, tools []domainmodel.ToolSchema, name string) domainmodel.ToolSchema {
	t.Helper()
	if tool := schemaByNameOptional(tools, name); tool != nil {
		return *tool
	}
	t.Fatalf("missing schema %s", name)
	return domainmodel.ToolSchema{}
}

func schemaByNameOptional(tools []domainmodel.ToolSchema, name string) *domainmodel.ToolSchema {
	for index := range tools {
		if tools[index].Name == name {
			return &tools[index]
		}
	}
	return nil
}

func schemaHasProperty(t *testing.T, tool domainmodel.ToolSchema, property string) bool {
	t.Helper()
	var decoded struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters, &decoded); err != nil {
		t.Fatalf("decode schema for %s: %v", tool.Name, err)
	}
	return decoded.Properties[property] != nil
}

func schemaPropertyDescription(t *testing.T, tool domainmodel.ToolSchema, property string) string {
	t.Helper()
	var decoded struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters, &decoded); err != nil {
		t.Fatalf("decode schema for %s: %v", tool.Name, err)
	}
	return decoded.Properties[property].Description
}

func TestNativeSelectionSchemasExposeOnlyScopedProposalAuthority(t *testing.T) {
	tools := BuiltinToolSchemas(BuiltinToolSchemaInput{NativeSelections: true})
	for _, name := range []string{"native_selection_read", "native_selection_propose"} {
		tool := schemaByName(t, tools, name)
		if !schemaHasProperty(t, tool, "scopeId") {
			t.Fatal("missing opaque scope")
		}
		for _, field := range []string{"path", "sessionId", "threadId", "selectionToken", "command", "bytes", "replacement", "provider"} {
			if schemaHasProperty(t, tool, field) {
				t.Fatalf("model controls %s", field)
			}
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["additionalProperties"] != false {
			t.Fatal("open input schema")
		}
		if HostAuthorizesReadOnly(name, false, false) != (name == "native_selection_read") {
			t.Fatal("incorrect effect classification")
		}
		if CanRunInParallel(name, false, false) {
			t.Fatal("native scopes must remain serialized")
		}
	}
	proposal := schemaByName(t, tools, "native_selection_propose")
	if !schemaHasProperty(t, proposal, "parts") || !schemaHasProperty(t, proposal, "operationId") {
		t.Fatal("proposal identity/typed parts missing")
	}
}

func TestNativePresentationToolSchemaHasOnlyFiniteExclusivePatches(t *testing.T) {
	schemas := BuiltinToolSchemas(BuiltinToolSchemaInput{NativeSelections: true})
	prefix := `{"scopeId":"` + strings.Repeat("a", 48) + `","operationId":"presentation_propose_01",`
	for _, payload := range []string{`"parts":[{"kind":"literal","text":"replacement"}]}`, `"workbook":{"kind":"number","value":1}}`, `"canvas":[{"kind":"set-edge-route","id":"canvas_` + strings.Repeat("a", 48) + `","points":[]}]}`, `"presentation":{"kind":"shape-fill","rgb":"#123456"}}`, `"presentation":{"kind":"shape-geometry","x100thMm":0,"y100thMm":0,"width100thMm":1,"height100thMm":1}}`} {
		if result, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "native_selection_propose", Arguments: json.RawMessage(prefix + payload)}, schemas); blocked {
			t.Fatal("finite patch rejected", result)
		}
	}
	for _, payload := range []string{`"canvas":[{"kind":"set-edge-route","id":"canvas_` + strings.Repeat("a", 48) + `","points":[{"x":0,"y":0}]}]}`, `"parts":[],"workbook":{"kind":"number","value":1}}`, `"presentation":{"kind":"chart-data"}}`, `"presentation":{"kind":"shape-fill","rgb":"#123456","shapeIndex":0}}`, `"presentation":{"kind":"shape-fill","rgb":"#123456"},"parts":[]}`, `"presentation":null}`, `"image":{"kind":"region"}}`} {
		if _, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "native_selection_propose", Arguments: json.RawMessage(prefix + payload)}, schemas); !blocked {
			t.Fatal("unsupported or mixed payload admitted", payload)
		}
	}
	description := schemaByName(t, schemas, "native_selection_read").Description
	for _, expected := range []string{"image-region", "noteParts", "imageObservation is unavailable", "no image pixels"} {
		if !strings.Contains(description, expected) {
			t.Fatal("image observation boundary missing", expected)
		}
	}
}
