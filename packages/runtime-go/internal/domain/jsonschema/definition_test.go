package jsonschema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDefinitionRejectsUnknownOpenAndIncompleteExternalSchemas(t *testing.T) {
	valid := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"rows": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"rows"},
	}
	options := DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}
	if err := ValidateDefinition(valid, options); err != nil {
		t.Fatalf("valid external schema was rejected: %v", err)
	}
	for name, schema := range map[string]map[string]any{
		"unknown": {"type": "object", "additionalProperties": false, "x-unsupported": false},
		"open":    {"type": "object", "additionalProperties": true},
		"array without items": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"rows": map[string]any{"type": "array"}},
		},
		"unknown required": {
			"type": "object", "additionalProperties": false, "properties": map[string]any{}, "required": []any{"missing"},
		},
		"unknown dialect": {"$schema": "https://example.invalid/schema", "type": "object", "additionalProperties": false},
		"implicit nested object": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"scope": map[string]any{"properties": map[string]any{}}},
		},
		"untyped numeric constraint": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"limit": map[string]any{"minimum": float64(1)}},
		},
		"empty child schema": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"payload": map[string]any{}},
		},
		"annotation-only child schema": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"payload": map[string]any{"description": "unconstrained"}},
		},
		"true child schema": {
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"payload": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDefinition(schema, options); err == nil {
				t.Fatal("unsafe schema was accepted")
			}
		})
	}
}

func TestValidateDefinitionAcceptsStandardDraftKeywordsAndLocalRefs(t *testing.T) {
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"$defs": map[string]any{
			"account": map[string]any{"type": "string", "format": "uuid"},
		},
		"properties": map[string]any{
			"account": map[string]any{"$ref": "#/$defs/account"},
			"rows": map[string]any{
				"type": "array", "uniqueItems": true, "minItems": float64(1),
				"items": map[string]any{"type": "integer", "exclusiveMinimum": float64(0)},
			},
		},
		"required": []any{"account", "rows"}, "minProperties": float64(2), "unevaluatedProperties": false,
	}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}); err != nil {
		t.Fatalf("standards-compliant closed schema was rejected: %v", err)
	}
	if err := ValidateStandardValue(schema, map[string]any{
		"account": "550e8400-e29b-41d4-a716-446655440000", "rows": []any{json.Number("1"), json.Number("2")},
	}); err != nil {
		t.Fatalf("standards-compliant value was rejected: %v", err)
	}
	if err := ValidateStandardValue(schema, map[string]any{
		"account": "not-a-uuid", "rows": []any{json.Number("1"), json.Number("1")},
	}); err == nil {
		t.Fatal("format/uniqueItems constraints were ignored")
	}
	external := map[string]any{
		"type": "object", "additionalProperties": false, "properties": map[string]any{
			"value": map[string]any{"$ref": "https://example.invalid/external.json"},
		},
	}
	if err := ValidateDefinition(external, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err == nil {
		t.Fatal("untrusted external schema reference was loaded")
	}
}

func TestValidateDefinitionKeepsRequiredPropertyNamesExact(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"account":  map[string]any{"type": "string"},
			" account": map[string]any{"type": "string"},
		},
		"required": []any{" account"},
	}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err != nil {
		t.Fatalf("exact whitespace property name was normalized: %v", err)
	}
	schema["required"] = []any{"  missing"}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err == nil {
		t.Fatal("unknown whitespace property was trimmed into a declared property")
	}
}

func TestValidateDefinitionRejectsExponentialLocalReferenceFanout(t *testing.T) {
	definitions := map[string]any{}
	const depth = 18
	for index := depth; index >= 0; index-- {
		name := fmt.Sprintf("level_%02d", index)
		if index == depth {
			definitions[name] = map[string]any{"type": "string"}
			continue
		}
		next := "#/$defs/" + fmt.Sprintf("level_%02d", index+1)
		definitions[name] = map[string]any{
			"anyOf": []any{map[string]any{"$ref": next}, map[string]any{"$ref": next}},
		}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"$defs": definitions,
		"properties": map[string]any{
			"value": map[string]any{"$ref": "#/$defs/level_00"},
		},
		"required": []any{"value"},
	}
	err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true})
	if err == nil || !strings.Contains(err.Error(), "evaluation budget") {
		t.Fatalf("exponential local-reference schema was not rejected by a deterministic budget: %v", err)
	}
}

func TestValidateDefinitionRejectsNonPointerAndDynamicReferences(t *testing.T) {
	for name, child := range map[string]map[string]any{
		"anchor ref":  {"$ref": "#named"},
		"dynamic ref": {"$dynamicRef": "#named"},
	} {
		t.Run(name, func(t *testing.T) {
			schema := map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"value": child},
			}
			if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err == nil {
				t.Fatal("non-deterministic schema reference was accepted")
			}
		})
	}
}

func TestRecursiveLocalRefFanoutBudgetRejected(t *testing.T) {
	node := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"next": map[string]any{"anyOf": []any{
				map[string]any{"$ref": "#/$defs/node"},
				map[string]any{"$ref": "#/$defs/node"},
			}},
		},
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"$defs":      map[string]any{"node": node},
		"properties": map[string]any{"value": map[string]any{"$ref": "#/$defs/node"}},
	}
	err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true})
	if err == nil || !strings.Contains(err.Error(), "recursive schema") {
		t.Fatalf("recursive local-ref fanout was accepted: %v", err)
	}
}

func TestLocalRefCannotTargetInstanceDataKeyword(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"default": map[string]any{"hidden": map[string]any{}},
		"properties": map[string]any{
			"payload": map[string]any{"$ref": "#/default/hidden"},
		},
	}
	err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true})
	if err == nil || !strings.Contains(err.Error(), "reviewed schema node") {
		t.Fatalf("local ref into unreviewed instance data was accepted: %v", err)
	}
}

func TestNestedIDCannotRedirectLocalRefAroundEvaluationBudget(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"payload": map[string]any{
				"$id": "child.json", "type": "object", "additionalProperties": false,
				"properties": map[string]any{},
			},
		},
	}
	err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true})
	if err == nil || !strings.Contains(err.Error(), "$id") {
		t.Fatalf("nested $id resource scope was accepted: %v", err)
	}
}

func TestPrefixItemsWithoutTailSchemaIsIncomplete(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"tuple": map[string]any{
				"type": "array", "prefixItems": []any{map[string]any{"type": "string"}},
			},
		},
	}
	options := DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}
	if err := ValidateDefinition(schema, options); err == nil {
		t.Fatal("prefixItems without an explicit tail schema was accepted as complete")
	}
	tuple := schema["properties"].(map[string]any)["tuple"].(map[string]any)
	tuple["items"] = false
	if err := ValidateDefinition(schema, options); err != nil {
		t.Fatalf("fixed-length tuple with items:false was rejected: %v", err)
	}
}

func TestSchemaDiagnosticsAreByteStableAcrossRuns(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"alpha": map[string]any{"type": "integer"},
			"beta":  map[string]any{"type": "integer"},
			"gamma": map[string]any{"type": "integer"},
		},
		"required": []any{"alpha", "beta", "gamma"},
	}
	value := map[string]any{"alpha": "x", "beta": "x", "gamma": "x"}
	want := ""
	for attempt := 0; attempt < 500; attempt++ {
		err := ValidateStandardValue(schema, value)
		if err == nil {
			t.Fatal("invalid value was accepted")
		}
		if want == "" {
			want = err.Error()
		}
		if err.Error() != want {
			t.Fatalf("schema diagnostic changed across identical runs: want=%q got=%q", want, err.Error())
		}
	}
}

func TestSchemaInstanceProductCannotExhaustRuntime(t *testing.T) {
	branches := make([]any, 0, 100)
	for value := 0; value < 100; value++ {
		branches = append(branches, map[string]any{"const": float64(value)})
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"rows": map[string]any{"type": "array", "items": map[string]any{"oneOf": branches}},
		},
		"required": []any{"rows"},
	}
	options := DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}
	if err := ValidateDefinition(schema, options); err != nil {
		t.Fatalf("bounded composition fixture was rejected before instance preflight: %v", err)
	}
	rows := make([]any, 20_000)
	for index := range rows {
		rows[index] = float64(index % 100)
	}
	err := ValidateStandardValue(schema, map[string]any{"rows": rows})
	if err == nil || !strings.Contains(err.Error(), "work budget") {
		t.Fatalf("schema-instance product bypassed deterministic work preflight: %v", err)
	}
	tooManyBranches := append(append([]any(nil), branches...), branches[:29]...)
	schema["properties"].(map[string]any)["rows"].(map[string]any)["items"].(map[string]any)["oneOf"] = tooManyBranches
	if err := ValidateDefinition(schema, options); err == nil || !strings.Contains(err.Error(), "branch budget") {
		t.Fatalf("oversized composition branch set was accepted: %v", err)
	}
}

func TestContainsIsDisabledForUntrustedRuntimeSchemas(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"rows": map[string]any{
				"type": "array", "items": map[string]any{"type": "integer"}, "contains": map[string]any{"type": "string"},
			},
		},
	}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}); err == nil {
		t.Fatal("contains error accumulation was enabled for an untrusted runtime schema")
	}
}

func TestEnumCardinalityParticipatesInSchemaInstanceBudget(t *testing.T) {
	values := make([]any, 2000)
	for index := range values {
		values[index] = fmt.Sprintf("value-%04d", index)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"rows": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": values}},
		},
		"required": []any{"rows"},
	}
	options := DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true}
	if err := ValidateDefinition(schema, options); err != nil {
		t.Fatalf("bounded enum schema was rejected before instance work preflight: %v", err)
	}
	rows := make([]any, 1000)
	for index := range rows {
		rows[index] = values[index%len(values)]
	}
	err := ValidateStandardValue(schema, map[string]any{"rows": rows})
	if err == nil || !strings.Contains(err.Error(), "work budget") {
		t.Fatalf("enum cardinality was omitted from schema-instance work: %v", err)
	}
}

func TestContentSchemaCannotBypassInstanceBudget(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"payload": map[string]any{
				"type": "string", "contentMediaType": "application/json",
				"contentSchema": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			},
		},
	}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err == nil {
		t.Fatal("nested content decoding bypass was enabled for an untrusted runtime schema")
	}
}

func TestPatternPropertiesKeyWorkCannotBypassBudget(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"patternProperties": map[string]any{"^account_[0-9]+$": map[string]any{"type": "string"}},
	}
	if err := ValidateDefinition(schema, DefinitionOptions{RequireObjectRoot: true, RequireClosedObjects: true}); err == nil {
		t.Fatal("patternProperties key-work multiplier was enabled for an untrusted runtime schema")
	}
}
