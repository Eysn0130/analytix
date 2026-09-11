package model

import (
	"encoding/json"
	"testing"
)

func TestCanonicalJSONSchemaOnlyNormalizesSchemaNodes(t *testing.T) {
	got := CanonicalJSONSchema(json.RawMessage(`{
		"type":"object",
		"required":["literal","required"],
		"properties":{
			"required":{"type":"string"},
			"literal":{
				"type":"object",
				"const":{"required":["z","a"],"type":"object"},
				"enum":[{"required":["b","a"]}]
			}
		}
	}`))
	want := `{"properties":{"literal":{"const":{"required":["z","a"],"type":"object"},"enum":[{"required":["b","a"]}],"type":"object"},"required":{"type":"string"}},"required":["literal","required"],"type":"object"}`
	if got != want {
		t.Fatalf("canonical schema rewrote instance data or property names:\n got: %s\nwant: %s", got, want)
	}
}

func TestCanonicalJSONSchemaNormalizesNestedSchemaSets(t *testing.T) {
	got := CanonicalJSONSchema(json.RawMessage(`{
		"type":"object",
		"properties":{"scope":{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"string"}},"required":["b","a","b"]}}
	}`))
	want := `{"properties":{"scope":{"properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["a","b"],"type":"object"}},"type":"object"}`
	if got != want {
		t.Fatalf("nested schema sets were not canonical: got=%s want=%s", got, want)
	}
}

func TestCanonicalJSONSchemaNormalizesDraft7Dependencies(t *testing.T) {
	left := CanonicalJSONSchema(json.RawMessage(`{
		"$schema":"http://json-schema.org/draft-07/schema#",
		"type":"object",
		"properties":{"mode":{"type":"string"},"amount":{"type":"number"},"currency":{"type":"string"},"scope":{"type":"object"}},
		"dependencies":{
			"mode":["currency","amount","currency"],
			"scope":{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"string"}},"required":["b","a"]}
		}
	}`))
	right := CanonicalJSONSchema(json.RawMessage(`{
		"dependencies":{
			"scope":{"required":["a","b"],"properties":{"a":{"type":"string"},"b":{"type":"string"}},"type":"object"},
			"mode":["amount","currency"]
		},
		"properties":{"scope":{"type":"object"},"currency":{"type":"string"},"amount":{"type":"number"},"mode":{"type":"string"}},
		"type":"object",
		"$schema":"http://json-schema.org/draft-07/schema#"
	}`))
	if left != right {
		t.Fatalf("semantically equal draft-07 dependencies were unstable:\nleft:  %s\nright: %s", left, right)
	}
	changed := CanonicalJSONSchema(json.RawMessage(`{
		"$schema":"http://json-schema.org/draft-07/schema#","type":"object",
		"properties":{"mode":{"type":"string"},"amount":{"type":"number"},"currency":{"type":"string"},"scope":{"type":"object"}},
		"dependencies":{"mode":["amount"],"scope":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["a","b"]}}
	}`))
	if left == changed {
		t.Fatal("a changed draft-07 dependency constraint was erased")
	}
}

func TestCanonicalJSONSchemaNeverCollapsesAmbiguousOrMalformedInput(t *testing.T) {
	valid := CanonicalJSONSchema(json.RawMessage(`{"type":"object","properties":{}}`))
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"type":"object","properties":{}} {}`),
		json.RawMessage(`{"type":"object","type":"string","properties":{}}`),
		json.RawMessage(`{"type":"object","required":["id",3],"properties":{"id":{"type":"string"}}}`),
		json.RawMessage(`{"type":"object","dependentRequired":{"id":["kind",false]},"properties":{}}`),
	} {
		if canonical := CanonicalJSONSchema(raw); canonical == valid {
			t.Fatalf("malformed schema collapsed into valid schema: raw=%s canonical=%s", raw, canonical)
		}
	}
}
