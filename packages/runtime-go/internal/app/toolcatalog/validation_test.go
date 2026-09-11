package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestValidateToolCallArgumentsRejectsMissingRequiredFields(t *testing.T) {
	output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		ID:        "call_1",
		Name:      "task",
		Arguments: json.RawMessage(`{}`),
	}, []domainmodel.ToolSchema{{
		Name:       "task",
		Parameters: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`),
	}})
	if !blocked {
		t.Fatal("missing required prompt should be blocked before tool execution")
	}
	if output["code"] != "validation_error" || !strings.Contains(output["error"].(string), "prompt is required") {
		t.Fatalf("unexpected validation output: %#v", output)
	}
}

func TestValidateToolCallArgumentsRejectsInvalidJSON(t *testing.T) {
	output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		ID:        "call_1",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":`),
	}, []domainmodel.ToolSchema{{
		Name:       "bash",
		Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
	}})
	if !blocked {
		t.Fatal("invalid JSON arguments should be blocked before tool execution")
	}
	if output["code"] != "validation_error" || !strings.Contains(output["error"].(string), "not valid JSON") {
		t.Fatalf("unexpected invalid JSON output: %#v", output)
	}
}

func TestValidateToolCallArgumentsRejectsDuplicateKeys(t *testing.T) {
	output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name: "lookup", Arguments: json.RawMessage(`{"caseId":"case-a","caseId":"case-b"}`),
	}, []domainmodel.ToolSchema{{
		Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"caseId":{"type":"string"}},"required":["caseId"],"additionalProperties":false}`),
	}})
	if !blocked || output["code"] != "validation_error" {
		t.Fatalf("duplicate provider argument reached execution: output=%#v blocked=%v", output, blocked)
	}
}

func TestValidateToolCallArgumentsAllowsZeroArgTools(t *testing.T) {
	schemas := []domainmodel.ToolSchema{{
		Name:       "status",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		ID:        "call_1",
		Name:      "status",
		Arguments: json.RawMessage(`{}`),
	}, schemas); blocked {
		t.Fatalf("zero-argument tools should be allowed: %#v", output)
	}
	if !ToolRequiresExactEmptyObjectArguments("status", schemas) {
		t.Fatal("strict zero-argument schema was not recognized")
	}
}

func TestExactEmptyObjectArgumentDetectionNeverWidensSchemas(t *testing.T) {
	if !ToolRequiresExactEmptyObjectArguments("closed_without_properties", []domainmodel.ToolSchema{{
		Name:       "closed_without_properties",
		Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}) {
		t.Fatal("closed object without a properties keyword was not recognized as exact empty arguments")
	}
	for name, parameters := range map[string]json.RawMessage{
		"optional_property": json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}},"additionalProperties":false}`),
		"open_object":       json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`),
		"required_property": json.RawMessage(
			`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		),
		"composed_object": json.RawMessage(
			`{"type":"object","properties":{},"additionalProperties":false,"allOf":[{"type":"object"}]}`,
		),
	} {
		if ToolRequiresExactEmptyObjectArguments(name, []domainmodel.ToolSchema{{Name: name, Parameters: parameters}}) {
			t.Fatalf("schema %q was incorrectly narrowed to exact empty arguments", name)
		}
	}
	if ToolRequiresExactEmptyObjectArguments("missing", nil) {
		t.Fatal("missing schema was treated as an exact empty-object contract")
	}
}

func TestValidateToolCallArgumentsRejectsUnknownSchema(t *testing.T) {
	output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		ID:        "call_1",
		Name:      "unadvertised_tool",
		Arguments: json.RawMessage(`{}`),
	}, nil)
	if !blocked || output["code"] != "tool_schema_missing" {
		t.Fatalf("unknown schemas must fail closed: %#v blocked=%v", output, blocked)
	}
}

func TestValidateToolCallArgumentsRejectsMalformedSchema(t *testing.T) {
	output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		ID:        "call_1",
		Name:      "lookup",
		Arguments: json.RawMessage(`{}`),
	}, []domainmodel.ToolSchema{{
		Name:       "lookup",
		Parameters: json.RawMessage(`{"type":`),
	}})
	if !blocked || output["code"] != "tool_schema_invalid" {
		t.Fatalf("malformed schemas must fail closed: %#v blocked=%v", output, blocked)
	}
}

func TestValidateToolCallArgumentsEnforcesTypesEnumsNestedAndAdditionalProperties(t *testing.T) {
	schema := []domainmodel.ToolSchema{{
		Name: "lookup",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"mode":{"type":"string","enum":["safe"]},
				"scope":{"type":"object","properties":{"limit":{"type":"integer","minimum":1}},"required":["limit"],"additionalProperties":false}
			},
			"required":["mode","scope"],
			"additionalProperties":false
		}`),
	}}
	for _, args := range []string{
		`{"mode":"unsafe","scope":{"limit":1}}`,
		`{"mode":"safe","scope":{"limit":"1"}}`,
		`{"mode":"safe","scope":{"limit":0}}`,
		`{"mode":"safe","scope":{"limit":1,"extra":true}}`,
		`{"mode":"safe","scope":{"limit":1},"extra":true}`,
	} {
		if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "lookup", Arguments: json.RawMessage(args)}, schema); !blocked {
			t.Fatalf("invalid arguments must be blocked: args=%s output=%#v", args, output)
		}
	}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name:      "lookup",
		Arguments: json.RawMessage(`{"mode":"safe","scope":{"limit":2}}`),
	}, schema); blocked {
		t.Fatalf("valid nested arguments should pass: %#v", output)
	}
}

func TestValidateJSONSchemaRawValueUsesExactOutputSemantics(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{"rows":{"type":"array","items":{"type":"object","properties":{"amount":{"type":"integer"}},"required":["amount"],"additionalProperties":false}}},
		"required":["rows"],
		"additionalProperties":false
	}`)
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"rows":[]}`), schema, "structuredContent"); err != nil {
		t.Fatalf("present empty array is valid JSON Schema output: %v", err)
	}
	for _, value := range []string{
		`{}`,
		`{"rows":"none"}`,
		`{"rows":[{"amount":"1"}]}`,
		`{"rows":[],"unexpected":true}`,
		`{"rows":[],"rows":[{"amount":1}]}`,
	} {
		if err := ValidateJSONSchemaRawValue(json.RawMessage(value), schema, "structuredContent"); err == nil {
			t.Fatalf("invalid output was accepted: %s", value)
		}
	}
}

func TestValidateToolCallArgumentsEnforcesConstAndRejectsUnsupportedSchemaKeywords(t *testing.T) {
	constSchema := []domainmodel.ToolSchema{{
		Name:       "count_case_rows",
		Parameters: json.RawMessage(`{"type":"object","properties":{"table":{"type":"string","const":"analysis_txn_detail_idx"}},"required":["table"],"additionalProperties":false}`),
	}}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name: "count_case_rows", Arguments: json.RawMessage(`{"table":"other_table"}`),
	}, constSchema); !blocked || output["code"] != "validation_error" {
		t.Fatalf("const mismatch reached execution: output=%#v blocked=%v", output, blocked)
	}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name: "count_case_rows", Arguments: json.RawMessage(`{"table":"analysis_txn_detail_idx"}`),
	}, constSchema); blocked {
		t.Fatalf("matching const was rejected: %#v", output)
	}
	unknownSchema := []domainmodel.ToolSchema{{
		Name:       "lookup",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false,"x-unsupported":false}`),
	}}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "lookup", Arguments: json.RawMessage(`{}`)}, unknownSchema); !blocked || output["code"] != "tool_schema_invalid" {
		t.Fatalf("unsupported schema keyword did not fail closed: output=%#v blocked=%v", output, blocked)
	}
}

func TestValidateJSONSchemaNumbersRemainExactBeyondFloat64(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"amount":{"type":"integer","minimum":9007199254740993},
			"ratio":{"type":"number","const":1}
		},
		"required":["amount","ratio"],
		"additionalProperties":false
	}`)
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"amount":9007199254740992,"ratio":1}`), schema, "structuredContent"); err == nil {
		t.Fatal("value below an exact >2^53 minimum was accepted")
	}
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"amount":9007199254740993,"ratio":1.0}`), schema, "structuredContent"); err != nil {
		t.Fatalf("exact boundary or numerically-equal const was rejected: %v", err)
	}
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"amount":9007199254740993,"ratio":1e0}`), schema, "structuredContent"); err != nil {
		t.Fatalf("exact exponent representation was rejected: %v", err)
	}
}

func TestStandardsCompliantMCPJSONSchemaKeywordsAreEnforcedOffline(t *testing.T) {
	schema := json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"$defs":{"account":{"type":"string","format":"uuid"}},
		"properties":{
			"account":{"$ref":"#/$defs/account"},
			"amounts":{"type":"array","minItems":1,"uniqueItems":true,"items":{"type":"integer","exclusiveMinimum":0}}
		},
		"required":["account","amounts"],
		"minProperties":2,
		"additionalProperties":false,
		"unevaluatedProperties":false
	}`)
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"account":"550e8400-e29b-41d4-a716-446655440000","amounts":[1,2]}`), schema, "structuredContent"); err != nil {
		t.Fatalf("standards-compliant MCP schema rejected valid output: %v", err)
	}
	for _, value := range []json.RawMessage{
		json.RawMessage(`{"account":"not-a-uuid","amounts":[1,2]}`),
		json.RawMessage(`{"account":"550e8400-e29b-41d4-a716-446655440000","amounts":[1,1]}`),
		json.RawMessage(`{"account":"550e8400-e29b-41d4-a716-446655440000","amounts":[0]}`),
	} {
		if err := ValidateJSONSchemaRawValue(value, schema, "structuredContent"); err == nil {
			t.Fatalf("standards-compliant MCP constraint was ignored: %s", value)
		}
	}
	external := json.RawMessage(`{"type":"object","properties":{"value":{"$ref":"https://example.invalid/schema.json"}},"additionalProperties":false}`)
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"value":"x"}`), external, "structuredContent"); err == nil {
		t.Fatal("MCP schema validation performed an external reference load")
	}
}

func TestBooleanChildSchemasCannotCrashInputOrOutputValidation(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{"value":{"anyOf":[false,{"type":"string"}]}},
		"required":["value"],
		"additionalProperties":false
	}`)
	advertised := []domainmodel.ToolSchema{{Name: "boolean_schema", Parameters: schema}}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name: "boolean_schema", Arguments: json.RawMessage(`{"value":7}`),
	}, advertised); !blocked || output["code"] != "validation_error" {
		t.Fatalf("boolean child schema did not fail closed for invalid input: output=%#v blocked=%v", output, blocked)
	}
	if output, blocked := ValidateToolCallArguments(domainmodel.ToolCall{
		Name: "boolean_schema", Arguments: json.RawMessage(`{"value":"ok"}`),
	}, advertised); blocked {
		t.Fatalf("boolean child schema rejected valid input: %#v", output)
	}
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"value":7}`), schema, "structuredContent"); err == nil {
		t.Fatal("boolean child schema accepted invalid structured output")
	}
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"value":"ok"}`), schema, "structuredContent"); err != nil {
		t.Fatalf("boolean child schema rejected valid structured output: %v", err)
	}
}

func TestSchemaRequiredPropertyNamesAreByteExact(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{"account":{"type":"string"}," account":{"type":"string"},"\u00a0account":{"type":"string"}},
		"required":[" account","\u00a0account"],
		"additionalProperties":false
	}`)
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{"account":"x"}`), schema, "structuredContent"); err == nil {
		t.Fatal("trimmed property name incorrectly satisfied exact required fields")
	}
	if err := ValidateJSONSchemaRawValue(json.RawMessage(`{" account":"x","\u00a0account":"y"}`), schema, "structuredContent"); err != nil {
		t.Fatalf("byte-exact whitespace property names were rejected: %v", err)
	}
}
