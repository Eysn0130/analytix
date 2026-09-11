package toolcatalog

import (
	"encoding/json"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type advertisementSourceStub struct {
	items []MCPToolAdvertisementV1
}

func (stub advertisementSourceStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []MCPToolAdvertisementV1 {
	return stub.items
}

func TestMCPAdvertisementSnapshotKeepsOrdinaryWritesAndDetachesSchemas(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-advertisement", TurnID: "turn-advertisement", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-advertisement", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source")), ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC),
	})
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("advertisement-instance")), 7,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	source := advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__write", InputSchema: closed, OutputSchema: closed, ReadOnly: false, ConnectionEpoch: 7, ServerIdentity: identity},
		{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 7, ServerIdentity: identity},
	}}
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(source, context)
	if !ok || len(advertisements) != 2 ||
		advertisements[0].Name != "mcp__docs__read" ||
		advertisements[1].Name != "mcp__docs__write" {
		t.Fatalf("case context replaced ordinary MCP tools: ok=%v items=%#v", ok, advertisements)
	}
	source.items[1].InputSchema[0] = '['
	if advertisements[0].InputSchema[0] != '{' {
		t.Fatalf("frozen advertisement shared mutable schema bytes: %s", advertisements[0].InputSchema)
	}
	if epochs := MCPConnectionEpochsFromAdvertisementsV1(advertisements); epochs[advertisements[0].Name] != 7 {
		t.Fatalf("frozen connection epoch was lost: %#v", epochs)
	}
	if policies := MCPReadOnlyPoliciesFromAdvertisementsV1(advertisements); !policies[advertisements[0].Name] || policies[advertisements[1].Name] {
		t.Fatalf("frozen host read-only policy was lost: %#v", policies)
	}
}

func TestMCPAdvertisementSnapshotQuarantinesMixedOrForgedServerPartition(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-invalid-advertisement", TurnID: "turn-invalid-advertisement", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-invalid-advertisement", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-invalid")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-invalid"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-invalid")), ContextEpoch: 2,
		IssuedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
	})
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	identityEpochOne, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("advertisement-instance-one")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	identityEpochTwo, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("advertisement-instance-two")), 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	identitySameEpochOtherInstance, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("advertisement-instance-three")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	otherServerIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"notes", "notes-server", "1.0.0", domainsecurity.SHA256Hex([]byte("advertisement-other-server")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, items := range map[string][]MCPToolAdvertisementV1{
		"forged identity": {{
			Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true,
			ConnectionEpoch: 1, ServerIdentity: "not-host-issued",
		}},
		"identity belongs to another server": {{
			Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true,
			ConnectionEpoch: 1, ServerIdentity: otherServerIdentity,
		}},
		"identity epoch differs from advertisement": {{
			Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true,
			ConnectionEpoch: 1, ServerIdentity: identityEpochTwo,
		}},
		"zero connection epoch": {{
			Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true,
			ConnectionEpoch: 0, ServerIdentity: identityEpochOne,
		}},
		"duplicate identity": {
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
		},
		"mixed epoch for one server": {
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
			{Name: "mcp__docs__search", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 2, ServerIdentity: identityEpochTwo},
		},
		"mixed identity at the same epoch": {
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
			{Name: "mcp__docs__search", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identitySameEpochOtherInstance},
		},
		"invalid schema": {
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
			{Name: "mcp__docs__search", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
		},
		"invalid output schema": {
			{Name: "mcp__docs__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
			{Name: "mcp__docs__search", InputSchema: closed, OutputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true, ConnectionEpoch: 1, ServerIdentity: identityEpochOne},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: items}, context); !ok || len(advertisements) != 0 {
				t.Fatalf("invalid server partition was not quarantined: ok=%v items=%#v", ok, advertisements)
			}
		})
	}
}

func TestMCPAdvertisementSnapshotKeepsValidDocsWhenFundsPartitionIsForged(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-partitioned-advertisement", TurnID: "turn-partitioned-advertisement", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-partitioned-advertisement", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-partitioned")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-partitioned"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-partitioned")), ContextEpoch: 4,
		IssuedAt: time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC),
	})
	docsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("partitioned-docs-instance")), 6,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 6, ServerIdentity: docsIdentity},
		{Name: "mcp__analytix_funds__analyze_account_flows", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 8, ServerIdentity: "not-host-issued"},
	}}, context)
	if !ok || len(advertisements) != 1 || advertisements[0].Name != "mcp__docs__lookup" {
		t.Fatalf("forged funds partition removed independent docs tools: ok=%v advertisements=%#v", ok, advertisements)
	}
}

func TestMCPAdvertisementSnapshotAddsAuthorizedReadOnlyFundsToOrdinaryMCP(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-authorized-funds", TurnID: "turn-authorized-funds", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-authorized-funds", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-authorized-funds")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-authorized-funds"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-authorized-funds")), ContextEpoch: 6,
		IssuedAt: time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC),
	})
	docsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("authorized-funds-docs-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	fundsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("authorized-funds-instance")), 7,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: docsIdentity},
		{Name: "mcp__analytix_funds__count_case_rows", InputSchema: json.RawMessage(`{"type":"array"}`), OutputSchema: nil, ReadOnly: false, ConnectionEpoch: 999, ServerIdentity: "canary-not-provider-authority"},
		{Name: "mcp__analytix_funds__analyze_account_flows", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 7, ServerIdentity: fundsIdentity},
	}}, context)
	if !ok || len(advertisements) != 2 ||
		advertisements[0].Name != "mcp__analytix_funds__analyze_account_flows" ||
		advertisements[1].Name != "mcp__docs__lookup" {
		t.Fatalf("authorized funds tool was not added to ordinary MCP: ok=%v advertisements=%#v", ok, advertisements)
	}
}

func TestMCPAdvertisementSnapshotInvalidDocsGroupDoesNotLeakPartialTools(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-invalid-docs-group", TurnID: "turn-invalid-docs-group", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-invalid-docs-group", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-invalid-docs-group")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-invalid-docs-group"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-invalid-docs-group")), ContextEpoch: 5,
		IssuedAt: time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
	})
	docsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("invalid-docs-instance")), 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	notesIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"notes", "notes-server", "1.0.0", domainsecurity.SHA256Hex([]byte("valid-notes-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 2, ServerIdentity: docsIdentity},
		{Name: "mcp__docs__search", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 2, ServerIdentity: docsIdentity},
		{Name: "mcp__notes__read", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: notesIdentity},
	}}, context)
	if !ok || len(advertisements) != 1 || advertisements[0].Name != "mcp__notes__read" {
		t.Fatalf("invalid docs group leaked partial tools or removed independent notes: ok=%v advertisements=%#v", ok, advertisements)
	}
}

func TestMCPAdvertisementSnapshotInvalidSecurityContextFailsGlobally(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true},
	}}, domainsecurity.TurnSecurityContext{})
	if ok || advertisements != nil {
		t.Fatalf("invalid global security context produced an advertisement snapshot: ok=%v advertisements=%#v", ok, advertisements)
	}
}

func TestValidMCPAdvertisementsFailClosedAndSelectRequestedNames(t *testing.T) {
	context := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-selection", TurnID: "turn-selection", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-selection", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-selection")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-selection"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-selection")), ContextEpoch: 3,
		IssuedAt: time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC),
	})
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("selection-instance")), 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	source := advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__first", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 4, ServerIdentity: identity},
		{Name: "mcp__docs__second", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 4, ServerIdentity: identity},
	}}
	advertisements := ValidMCPToolAdvertisementsForSecurityContextV1(source, context)
	selected := MCPToolAdvertisementsByNameV1(advertisements, []string{"mcp__docs__second", "missing", "mcp__docs__first"})
	if len(selected) != 2 || selected[0].Name != "mcp__docs__second" || selected[1].Name != "mcp__docs__first" {
		t.Fatalf("requested advertisement order was not preserved: %#v", selected)
	}
	if invalid := ValidMCPToolAdvertisementsForSecurityContextV1(source, domainsecurity.TurnSecurityContext{}); invalid != nil {
		t.Fatalf("invalid context returned usable advertisements: %#v", invalid)
	}
}

func TestGeneralContextKeepsOrdinaryMCPAndOmitsCaseMCP(t *testing.T) {
	context, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-mcp", TurnID: "turn-general-mcp", WorkspaceRealPath: "/workspace/general-mcp",
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("general-mcp-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	fundsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("general-funds-instance")), 5,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: identity},
		{Name: "mcp__docs__write", InputSchema: closed, OutputSchema: closed, ReadOnly: false, ConnectionEpoch: 3, ServerIdentity: identity},
		{Name: "mcp__analytix_funds__count_case_rows", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 5, ServerIdentity: fundsIdentity},
	}}, context)
	if !ok || len(advertisements) != 2 ||
		advertisements[0].Name != "mcp__docs__lookup" ||
		advertisements[1].Name != "mcp__docs__write" {
		t.Fatalf("general context replaced ordinary MCP or exposed case MCP: ok=%v advertisements=%#v", ok, advertisements)
	}
}

func TestWitnessedBoundaryKeepsOrdinaryMCPAndOmitsCaseMCP(t *testing.T) {
	securityContext := toolCatalogWitnessedBoundaryContext(t, "mcp")
	docsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("boundary-docs-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	fundsIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("boundary-funds-instance")), 5,
	)
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(advertisementSourceStub{items: []MCPToolAdvertisementV1{
		{Name: "mcp__docs__lookup", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 3, ServerIdentity: docsIdentity},
		{Name: "mcp__docs__write", InputSchema: closed, OutputSchema: closed, ReadOnly: false, ConnectionEpoch: 3, ServerIdentity: docsIdentity},
		{Name: "mcp__analytix_funds__count_case_rows", InputSchema: closed, OutputSchema: closed, ReadOnly: true, ConnectionEpoch: 5, ServerIdentity: fundsIdentity},
	}}, securityContext)
	if !ok || len(advertisements) != 2 ||
		advertisements[0].Name != "mcp__docs__lookup" ||
		advertisements[1].Name != "mcp__docs__write" {
		t.Fatalf("witnessed case boundary replaced ordinary MCP or exposed case MCP: ok=%v advertisements=%#v", ok, advertisements)
	}
}
