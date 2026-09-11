package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestToolSchemaHashIgnoresToolOrderAndJSONSetOrder(t *testing.T) {
	left := []domainmodel.ToolSchema{
		{Name: "write", Description: "Write", Parameters: json.RawMessage(`{"type":"object","required":["path","content","path"],"properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)},
		{Name: "read", Description: "Read", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)},
	}
	right := []domainmodel.ToolSchema{
		{Name: "read", Description: "Read", Parameters: json.RawMessage(`{"required":["path"],"properties":{"path":{"type":"string"}},"type":"object"}`)},
		{Name: "write", Description: "Write", Parameters: json.RawMessage(`{"properties":{"content":{"type":"string"},"path":{"type":"string"}},"required":["content","path"],"type":"object"}`)},
	}
	if ToolSchemaHash(left) != ToolSchemaHash(right) {
		t.Fatalf("schema hash should ignore order and required duplicates: left=%s right=%s", ToolSchemaHash(left), ToolSchemaHash(right))
	}
	changed := append([]domainmodel.ToolSchema(nil), right...)
	changed[0].Description = "Read changed"
	if ToolSchemaHash(left) == ToolSchemaHash(changed) {
		t.Fatalf("schema hash should change when materialized schema changes")
	}
}

func TestToolSchemaNameSetHashIgnoresOrderWhitespaceAndDuplicates(t *testing.T) {
	left := []domainmodel.ToolSchema{{Name: " write "}, {Name: "read"}, {Name: "read"}, {Name: ""}}
	right := []domainmodel.ToolSchema{{Name: "read"}, {Name: "write"}}
	if ToolSchemaNameSetHash(left) != ToolSchemaNameSetHash(right) {
		t.Fatalf("name-set hash should bind the sorted unique normalized names: left=%s right=%s", ToolSchemaNameSetHash(left), ToolSchemaNameSetHash(right))
	}
	changed := []domainmodel.ToolSchema{{Name: "read"}, {Name: "edit"}}
	if ToolSchemaNameSetHash(left) == ToolSchemaNameSetHash(changed) {
		t.Fatal("name-set hash did not change with the advertised name set")
	}
}

func TestToolSchemaHashBindsHostOnlyMCPOutputSchema(t *testing.T) {
	base := []domainmodel.ToolSchema{{
		Name: "mcp__docs__lookup", Description: "Lookup",
		Parameters:   json.RawMessage(`{"type":"object","additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Source:       "mcp",
	}}
	reordered := append([]domainmodel.ToolSchema(nil), base...)
	reordered[0].OutputSchema = json.RawMessage(`{"required":["text"],"properties":{"text":{"type":"string"}},"additionalProperties":false,"type":"object"}`)
	if ToolSchemaHash(base) != ToolSchemaHash(reordered) {
		t.Fatal("canonical output schema key order changed the grant hash")
	}
	changed := append([]domainmodel.ToolSchema(nil), base...)
	changed[0].OutputSchema = json.RawMessage(`{"type":"object","properties":{"text":{"type":"number"}},"required":["text"],"additionalProperties":false}`)
	if ToolSchemaHash(base) == ToolSchemaHash(changed) {
		t.Fatal("output schema change did not invalidate the grant hash")
	}
	body, err := json.Marshal(base[0])
	if err != nil || strings.Contains(string(body), "outputSchema") {
		t.Fatalf("host output schema leaked through provider-facing JSON: body=%s err=%v", body, err)
	}
}

func TestToolSchemaHashBindsHostOnlyMCPTaskSupport(t *testing.T) {
	base := []domainmodel.ToolSchema{{
		Name: "mcp__docs__lookup", Description: "Lookup",
		Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Source:     "mcp", TaskSupport: "forbidden",
	}}
	defaulted := append([]domainmodel.ToolSchema(nil), base...)
	defaulted[0].TaskSupport = ""
	if ToolSchemaHash(base) != ToolSchemaHash(defaulted) {
		t.Fatal("the MCP default taskSupport=forbidden changed the grant hash")
	}
	optional := append([]domainmodel.ToolSchema(nil), base...)
	optional[0].TaskSupport = "optional"
	if ToolSchemaHash(base) == ToolSchemaHash(optional) {
		t.Fatal("taskSupport change did not invalidate the execution grant hash")
	}
	body, err := json.Marshal(optional[0])
	if err != nil || strings.Contains(string(body), "taskSupport") {
		t.Fatalf("host-only taskSupport leaked through provider-facing JSON: body=%s err=%v", body, err)
	}
}

func TestToolSchemaHashPreservesExactLargeNumbers(t *testing.T) {
	base := []domainmodel.ToolSchema{{
		Name:         "mcp__funds__lookup",
		Parameters:   json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer","const":9007199254740992}},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer","const":9007199254740992}},"additionalProperties":false}`),
	}}
	inputChanged := append([]domainmodel.ToolSchema(nil), base...)
	inputChanged[0].Parameters = json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer","const":9007199254740993}},"additionalProperties":false}`)
	if ToolSchemaHash(base) == ToolSchemaHash(inputChanged) {
		t.Fatal("input schema hash folded distinct >2^53 constants")
	}
	outputChanged := append([]domainmodel.ToolSchema(nil), base...)
	outputChanged[0].OutputSchema = json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer","const":9007199254740993}},"additionalProperties":false}`)
	if ToolSchemaHash(base) == ToolSchemaHash(outputChanged) {
		t.Fatal("output schema hash folded distinct >2^53 constants")
	}
}

func TestToolSchemaHashDoesNotCollapseMalformedSchemaIntoValidGrant(t *testing.T) {
	valid := []domainmodel.ToolSchema{{Name: "mcp__docs__lookup", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}}
	for _, malformed := range []json.RawMessage{
		json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false} {}`),
		json.RawMessage(`{"type":"object","type":"string","properties":{},"additionalProperties":false}`),
		json.RawMessage(`{"type":"object","required":["id",3],"properties":{},"additionalProperties":false}`),
	} {
		changed := []domainmodel.ToolSchema{{Name: valid[0].Name, Parameters: malformed}}
		if ToolSchemaHash(valid) == ToolSchemaHash(changed) {
			t.Fatalf("malformed schema minted valid schema hash: %s", malformed)
		}
	}
}

func TestToolSchemaHashBindsCanonicalDraft7Dependencies(t *testing.T) {
	base := []domainmodel.ToolSchema{{
		Name: "mcp__funds__lookup", Description: "Lookup case funds", Source: "mcp",
		Parameters: json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"mode":{"type":"string"},"amount":{"type":"number"},"currency":{"type":"string"}},"dependencies":{"mode":["currency","amount"]},"additionalProperties":false}`),
	}}
	reordered := append([]domainmodel.ToolSchema(nil), base...)
	reordered[0].Parameters = json.RawMessage(`{"additionalProperties":false,"dependencies":{"mode":["amount","currency","amount"]},"properties":{"currency":{"type":"string"},"amount":{"type":"number"},"mode":{"type":"string"}},"type":"object","$schema":"http://json-schema.org/draft-07/schema#"}`)
	if ToolSchemaHash(base) != ToolSchemaHash(reordered) {
		t.Fatal("equivalent draft-07 dependency order changed the execution schema hash")
	}
	changed := append([]domainmodel.ToolSchema(nil), base...)
	changed[0].Parameters = json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"mode":{"type":"string"},"amount":{"type":"number"},"currency":{"type":"string"}},"dependencies":{"mode":["amount"]},"additionalProperties":false}`)
	if ToolSchemaHash(base) == ToolSchemaHash(changed) {
		t.Fatal("changed draft-07 dependency constraint did not invalidate the execution schema hash")
	}
}
