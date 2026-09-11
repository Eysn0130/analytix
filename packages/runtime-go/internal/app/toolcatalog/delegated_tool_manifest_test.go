package toolcatalog

import (
	"encoding/json"
	"testing"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSubagentScopedManifestIgnoresUnrelatedMCPToolAddition(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	identity := delegatedManifestIdentity(t, "docs", 3, "instance-a")
	lookup := MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		TaskSupport: domainmcp.ToolTaskSupportForbidden, ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: identity,
	}
	schemas := MCPToolSchemasFromAdvertisementsV1([]MCPToolAdvertisementV1{lookup})
	providerSchemas := []domainmodel.ToolSchema{{
		Name: schemas[0].Name, Description: schemas[0].Description, Parameters: schemas[0].Parameters,
		OutputSchema: schemas[0].OutputSchema, Source: "mcp", TaskSupport: string(schemas[0].TaskSupport),
	}}
	original, err := BuildDelegatedToolManifestV1([]string{lookup.Name}, providerSchemas, []MCPToolAdvertisementV1{lookup})
	if err != nil {
		t.Fatal(err)
	}
	unrelated := lookup
	unrelated.Name = "mcp__docs__unrelated"
	withUnrelated, err := BuildDelegatedToolManifestV1([]string{lookup.Name}, providerSchemas, []MCPToolAdvertisementV1{lookup, unrelated})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch := DelegatedToolManifestMismatchV1(original, withUnrelated); mismatch != "" {
		t.Fatalf("unrelated MCP addition changed the scoped manifest: %s", mismatch)
	}
}

func TestSubagentMCPIdentityEpochSwapWithSameSchemaRejected(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	originalAdvertisement := MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: delegatedManifestIdentity(t, "docs", 3, "instance-a"),
	}
	providerSchemas := []domainmodel.ToolSchema{{
		Name: originalAdvertisement.Name, Description: originalAdvertisement.Description,
		Parameters: closed, OutputSchema: closed, Source: "mcp",
	}}
	original, err := BuildDelegatedToolManifestV1([]string{originalAdvertisement.Name}, providerSchemas, []MCPToolAdvertisementV1{originalAdvertisement})
	if err != nil {
		t.Fatal(err)
	}
	changed := originalAdvertisement
	changed.ConnectionEpoch = 4
	changed.ServerIdentity = delegatedManifestIdentity(t, "docs", 4, "instance-b")
	actual, err := BuildDelegatedToolManifestV1([]string{changed.Name}, providerSchemas, []MCPToolAdvertisementV1{changed})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch := DelegatedToolManifestMismatchV1(original, actual); mismatch != "subagent_mcp_authority_mismatch" {
		t.Fatalf("same-schema identity/epoch swap mismatch=%q", mismatch)
	}
}

func TestSubagentMCPReadOnlyPolicySwapWithSameSchemaRejected(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisement := MCPToolAdvertisementV1{
		Name: "mcp__docs__lookup", Description: "Lookup", InputSchema: closed, OutputSchema: closed,
		ReadOnly: true, ConnectionEpoch: 2, ServerIdentity: delegatedManifestIdentity(t, "docs", 2, "instance"),
	}
	schemas := []domainmodel.ToolSchema{{Name: advertisement.Name, Description: advertisement.Description, Parameters: closed, OutputSchema: closed, Source: "mcp"}}
	original, err := BuildDelegatedToolManifestV1([]string{advertisement.Name}, schemas, []MCPToolAdvertisementV1{advertisement})
	if err != nil {
		t.Fatal(err)
	}
	advertisement.ReadOnly = false
	actual, err := BuildDelegatedToolManifestV1([]string{advertisement.Name}, schemas, []MCPToolAdvertisementV1{advertisement})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch := DelegatedToolManifestMismatchV1(original, actual); mismatch != "subagent_mcp_authority_mismatch" {
		t.Fatalf("same-schema host policy swap mismatch=%q", mismatch)
	}
}

func TestSubagentSchemaSwapBeforeFirstProviderDispatchRejected(t *testing.T) {
	stringSchema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`)
	integerSchema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"integer"}},"required":["q"],"additionalProperties":false}`)
	output := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	identity := delegatedManifestIdentity(t, "docs", 8, "instance")
	originalAdvertisement := MCPToolAdvertisementV1{Name: "mcp__docs__lookup", InputSchema: stringSchema, OutputSchema: output, ReadOnly: true, ConnectionEpoch: 8, ServerIdentity: identity}
	changedAdvertisement := originalAdvertisement
	changedAdvertisement.InputSchema = integerSchema
	originalSchemas := []domainmodel.ToolSchema{{Name: originalAdvertisement.Name, Parameters: stringSchema, OutputSchema: output, Source: "mcp"}}
	changedSchemas := []domainmodel.ToolSchema{{Name: changedAdvertisement.Name, Parameters: integerSchema, OutputSchema: output, Source: "mcp"}}
	expected, err := BuildDelegatedToolManifestV1([]string{originalAdvertisement.Name}, originalSchemas, []MCPToolAdvertisementV1{originalAdvertisement})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := BuildDelegatedToolManifestV1([]string{changedAdvertisement.Name}, changedSchemas, []MCPToolAdvertisementV1{changedAdvertisement})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch := DelegatedToolManifestMismatchV1(expected, actual); mismatch != "subagent_tool_schema_mismatch" {
		t.Fatalf("same-name schema swap mismatch=%q", mismatch)
	}
	if err := ValidateDelegatedToolManifestForDispatchV1(expected, []string{changedAdvertisement.Name}, changedSchemas, []MCPToolAdvertisementV1{changedAdvertisement}); err == nil || err.Error() != "subagent_tool_schema_mismatch" {
		t.Fatalf("dispatch accepted the same-name schema swap: %v", err)
	}
	if err := ValidateDelegatedToolManifestForDispatchV1(nil, nil, nil, nil); err != nil {
		t.Fatalf("top-level dispatch unexpectedly required a delegated manifest: %v", err)
	}
}

func delegatedManifestIdentity(t *testing.T, serverID string, epoch uint64, instance string) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		serverID, serverID+"-server", "1.0.0", domainsecurity.SHA256Hex([]byte(instance)), epoch,
	)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
