package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

type nativeSourceProbeClient struct {
	tools                []ToolSpec
	response             any
	transportCalls       int
	listCalls            int
	nativeCalls          int
	toolCalls            int
	callStarted          chan struct{}
	callRelease          chan struct{}
	callErr              error
	listErr              error
	catalogErr           error
	catalogIssues        []mcpprotocol.ToolContractIssue
	nativeErr            error
	losslessErr          error
	closeCalls           int
	closeStarted         chan struct{}
	closeRelease         chan struct{}
	observedMethod       string
	observedParams       map[string]any
	observedMethods      []string
	observedParamHistory []map[string]any
	nativeResponse       func(string, map[string]any) (any, error)
	losslessResponse     func(string, map[string]any) (domainmcp.LosslessToolResult, error)
	losslessStarted      chan struct{}
	losslessRelease      chan struct{}
}

func TestLegacyReadyProbeCannotAdvertiseExecuteOrMintGrant(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("legacy-quarantine-binding"))
	snapshotID := domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("legacy-quarantine-snapshot"))
	spec := pinnedFundsSourceSpec(t, ServerSpec{
		ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.16",
		IdentitySource: "installed-plugin-manifest", ReadOnlyToolNames: map[string]bool{"count_case_rows": true},
	})
	client := &nativeSourceProbeClient{
		tools: []ToolSpec{{
			Name: "count_case_rows", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
		response: map[string]any{
			"version": float64(1), "serverName": "analytix_funds", "serverVersion": "0.16.16",
			"caseId": "case-legacy-quarantine", "caseBindingHash": bindingHash, "datasetSnapshotId": snapshotID,
			"ready": true, "readOnly": true, "blocker": "", "checkedAt": now.Format(time.RFC3339Nano),
		},
	}
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.clients[spec.ID] = client
	manager.connectionEpochs[spec.ID] = 11
	manager.serverIdentities[spec.ID] = mcpTestVerifiedIdentity(t, spec.ID, spec.ExpectedServerName, spec.ExpectedServerVersion, 11)
	manager.specFingerprints[spec.ID] = SpecFingerprint(spec)
	if err := manager.registerToolsNoLock(spec, client.tools); err != nil {
		t.Fatal(err)
	}
	manager.refreshCatalogStateNoLock()
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-legacy-quarantine", TurnID: "turn-legacy-quarantine", WorkspaceRealPath: "/workspace",
		CaseID: "case-legacy-quarantine", CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotID,
		SourceManifestHash: domainsecurity.SourceManifestHash(manager.ServerDiagnostics()), ContextEpoch: 5, IssuedAt: now,
	})
	input := sourceprobeport.Input{
		ServerID: spec.ID, WorkspaceRealPath: securityContext.WorkspaceRealPath, ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
		DatasetSnapshotID: snapshotID, ContextEpoch: securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
	}
	if probe, err := manager.ProbeCaseSource(context.Background(), input); err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable || probe.ProbeDigest != "" {
		t.Fatalf("legacy ready self-report escaped quarantine: probe=%#v err=%v", probe, err)
	}
	if client.listCalls != 0 || client.nativeCalls != 0 || client.toolCalls != 0 {
		t.Fatalf("legacy snapshot reached MCP I/O: list=%d native=%d tool=%d", client.listCalls, client.nativeCalls, client.toolCalls)
	}
	if tools := manager.LiveToolsForSecurityContext(securityContext); len(tools) != 0 {
		t.Fatalf("legacy snapshot exposed provider tools: %#v", tools)
	}
	if advertisements := manager.MCPToolAdvertisementSnapshotV1(securityContext); len(advertisements) != 0 {
		t.Fatalf("legacy snapshot exposed grant-minting advertisements: %#v", advertisements)
	}
	toolName := CanonicalToolName(spec.ID, "count_case_rows")
	if result := manager.CallToolContext(context.Background(), toolName, true, map[string]any{}); result["executed"] != false || client.toolCalls != 0 {
		t.Fatalf("legacy snapshot reached factual tool execution: result=%#v calls=%d", result, client.toolCalls)
	}

	bareV2 := input
	bareV2.DatasetSnapshotID = domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("bare-v2"))
	if _, err := manager.ProbeCaseSource(context.Background(), bareV2); err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("bare DSV2 self-report escaped registry gate: %v", err)
	}
	if client.listCalls != 0 || client.nativeCalls != 0 || client.toolCalls != 0 {
		t.Fatalf("bare DSV2 reached MCP I/O: list=%d native=%d tool=%d", client.listCalls, client.nativeCalls, client.toolCalls)
	}
}

func (client *nativeSourceProbeClient) ListTools() ([]ToolSpec, error) {
	client.transportCalls++
	return client.tools, nil
}
func (client *nativeSourceProbeClient) ListPrompts() ([]PromptSpec, error) {
	client.transportCalls++
	return nil, nil
}
func (client *nativeSourceProbeClient) ListResources() ([]ResourceSpec, error) {
	client.transportCalls++
	return nil, nil
}
func (client *nativeSourceProbeClient) CallTool(string, map[string]any) (any, error) {
	client.toolCalls++
	if client.callStarted != nil {
		client.callStarted <- struct{}{}
	}
	if client.callRelease != nil {
		<-client.callRelease
	}
	if client.callErr != nil {
		return nil, client.callErr
	}
	return mcpprotocol.ExtractLosslessToolResult(json.RawMessage(`{"content":[],"structuredContent":{}}`)), nil
}
func (client *nativeSourceProbeClient) CallToolWithHostContext(_ context.Context, name string, arguments map[string]any, envelope domainmcp.HostContextEnvelope) (any, error) {
	if err := domainmcp.ValidateHostContextEnvelope(envelope); err != nil {
		return nil, err
	}
	return client.CallTool(name, arguments)
}
func (client *nativeSourceProbeClient) Close() {
	client.closeCalls++
	if client.closeStarted != nil {
		client.closeStarted <- struct{}{}
	}
	if client.closeRelease != nil {
		<-client.closeRelease
	}
}
func (client *nativeSourceProbeClient) ListToolsContext(context.Context) ([]ToolSpec, error) {
	client.listCalls++
	if client.listErr != nil {
		return nil, client.listErr
	}
	return client.tools, nil
}
func (client *nativeSourceProbeClient) ListToolCatalogContext(context.Context) (mcpprotocol.ToolCatalog, error) {
	client.listCalls++
	if client.catalogErr != nil {
		return mcpprotocol.ToolCatalog{}, client.catalogErr
	}
	if client.listErr != nil {
		return mcpprotocol.ToolCatalog{}, client.listErr
	}
	return mcpprotocol.ToolCatalog{Tools: client.tools, Quarantined: client.catalogIssues}, nil
}
func (client *nativeSourceProbeClient) CallNativeContext(_ context.Context, method string, params map[string]any) (any, error) {
	client.nativeCalls++
	client.observedMethod = method
	client.observedParams = params
	client.observedMethods = append(client.observedMethods, method)
	client.observedParamHistory = append(client.observedParamHistory, params)
	if client.nativeErr != nil {
		return nil, client.nativeErr
	}
	if client.nativeResponse != nil {
		return client.nativeResponse(method, params)
	}
	return client.response, nil
}
func (client *nativeSourceProbeClient) CallNativeLosslessContext(ctx context.Context, method string, params map[string]any) (domainmcp.LosslessToolResult, error) {
	client.nativeCalls++
	client.observedMethod = method
	client.observedParams = params
	client.observedMethods = append(client.observedMethods, method)
	client.observedParamHistory = append(client.observedParamHistory, params)
	if client.losslessStarted != nil {
		client.losslessStarted <- struct{}{}
	}
	if client.losslessRelease != nil {
		select {
		case <-client.losslessRelease:
		case <-ctx.Done():
			return domainmcp.LosslessToolResult{}, ctx.Err()
		}
	}
	if client.losslessErr != nil {
		return domainmcp.LosslessToolResult{}, client.losslessErr
	}
	if client.losslessResponse != nil {
		return client.losslessResponse(method, params)
	}
	body, err := json.Marshal(client.response)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	return mcpprotocol.DecodeLosslessJSONResult(body), nil
}

func TestProductionManagerLocksExactFundsEvidenceNativeRead(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	arguments := json.RawMessage(`{"table_name":"analysis_txn_detail_idx"}`)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-legacy-quarantine",
		ServerIdentity: fixture.manager.serverIdentities[fixture.spec.ID],
		ToolName:       fundsCountEvidenceToolName, ToolCallID: toolidentity.MustHostToolCallIDV1("source-probe-count"), ConnectionEpoch: fixture.connectionEpoch,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("legacy-count-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("legacy-count-scope")), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	callbackCalled := false
	err := fixture.manager.WithCurrentEvidenceRead(context.Background(), sourceprobeport.EvidenceReadInput{
		Context: fixture.securityContext, Grant: grant, Arguments: arguments,
	}, func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error {
		callbackCalled = true
		return nil
	})
	if err == nil || callbackCalled {
		t.Fatalf("legacy snapshot reached native evidence read: called=%v err=%v", callbackCalled, err)
	}
	publicationCalled := false
	err = fixture.manager.WithFreshPublicationSnapshot(context.Background(), sourceprobeport.PublicationInput{
		Context: fixture.securityContext,
		Requirements: []sourceprobeport.PublicationSourceRequirement{{
			ReceiptID: "evr_legacy_quarantine", ServerID: fixture.spec.ID,
			ServerIdentity: fixture.manager.serverIdentities[fixture.spec.ID],
			ServerVersion:  hostFundsTestPackageVersionV1, ConnectionEpoch: fixture.connectionEpoch,
			ToolName: fundsCountEvidenceToolName, DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID,
		}},
	}, func([]domainsecurity.VerifiedSourceProbe) error {
		publicationCalled = true
		return nil
	})
	if err == nil || publicationCalled {
		t.Fatalf("legacy snapshot reached publication freshness callback: called=%v err=%v", publicationCalled, err)
	}
	_, err = fixture.manager.ProbeCaseSource(context.Background(), sourceprobeport.Input{
		ServerID: fixture.spec.ID, WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		CaseID: fixture.securityContext.CaseID, CaseBindingHash: fixture.securityContext.CaseBindingHash,
		DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID, ContextEpoch: fixture.securityContext.ContextEpoch,
		ContextDigest: fixture.securityContext.ContextDigest,
	})
	if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("legacy snapshot did not return the fixed probe blocker: %v", err)
	}
	if fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 || fixture.client.toolCalls != 0 ||
		len(fixture.manager.sourceProbes) != 0 || len(fixture.manager.LiveToolsForSecurityContext(fixture.securityContext)) != 0 {
		t.Fatalf("legacy snapshot crossed MCP fact boundaries: client=%#v probes=%#v", fixture.client, fixture.manager.sourceProbes)
	}
}
func TestProductionManagerCurrentRunCaseSourceProbe(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	input := sourceprobeport.Input{
		ServerID: fixture.spec.ID, WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		CaseID: fixture.securityContext.CaseID, CaseBindingHash: fixture.securityContext.CaseBindingHash,
		DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID, ContextEpoch: fixture.securityContext.ContextEpoch,
		ContextDigest: fixture.securityContext.ContextDigest,
	}
	for _, invalidSnapshotID := range []string{
		"", "snapshot_1", domainsecurity.NoDatasetSnapshotID,
		domainsecurity.UnresolvedSnapshotMark + fixture.securityContext.CaseBindingHash,
		domainsecurity.DatasetSnapshotIDPrefixV2 + strings.ToUpper(domainsecurity.SHA256Hex([]byte("uppercase"))),
		domainsecurity.DatasetSnapshotIDPrefixV2 + strings.Repeat("a", 63),
		fixture.securityContext.DatasetSnapshotID,
	} {
		candidate := input
		candidate.DatasetSnapshotID = invalidSnapshotID
		if _, err := fixture.manager.ProbeCaseSource(context.Background(), candidate); err == nil {
			t.Fatalf("non-authoritative host snapshot reached current-run probe: %q", invalidSnapshotID)
		}
	}
	if fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 || fixture.client.toolCalls != 0 ||
		len(fixture.manager.sourceProbes) != 0 {
		t.Fatalf("legacy current-run probes crossed MCP I/O or retained authority: client=%#v probes=%#v", fixture.client, fixture.manager.sourceProbes)
	}
	if tools := fixture.manager.LiveToolsForSecurityContext(fixture.securityContext); len(tools) != 0 {
		t.Fatalf("legacy source probe advertised factual tools: %#v", tools)
	}
	diagnostics := fixture.manager.ServerDiagnosticsForSecurityContext(fixture.securityContext)[0].(map[string]any)
	if diagnostics["sourceReady"] != false || diagnostics["sourceProbeDigest"] != "" ||
		diagnostics["datasetSnapshotId"] != "" || diagnostics["sourceCaseId"] != "" {
		t.Fatalf("legacy source authority leaked into contextual diagnostics: %#v", diagnostics)
	}
	callbackCalled := false
	if err := fixture.manager.WithCurrentProbe(context.Background(), sourceprobeport.CurrentInput{
		ServerID: fixture.spec.ID, Context: fixture.securityContext, ConnectionEpoch: fixture.connectionEpoch,
	}, func(domainsecurity.VerifiedSourceProbe) error {
		callbackCalled = true
		return nil
	}); err == nil || callbackCalled {
		t.Fatalf("legacy source probe reached current authority callback: called=%v err=%v", callbackCalled, err)
	}
}
func TestProductionManagerValidateCurrentProbeRequiresExactCurrentAuthorityWithoutIO(t *testing.T) {
	t.Run("exact current authority", func(t *testing.T) {
		fixture := newCurrentProbeValidationFixture(t)
		before := sourceProbeClientCallCountsForTest(fixture.client)
		if err := fixture.manager.ValidateCurrentProbe(context.Background(), fixture.spec.ID, fixture.securityContext); err == nil {
			t.Fatal("structurally exact DSV1 probe gained current fact authority")
		}
		assertSourceProbeValidationDidNotCallClient(t, fixture.client, before)
	})

	tests := []struct {
		name   string
		mutate func(*testing.T, *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext)
	}{
		{
			name: "disconnect",
			mutate: func(_ *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				fixture.manager.Disconnect()
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "client nil",
			mutate: func(_ *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				fixture.manager.mu.Lock()
				fixture.manager.clients[fixture.spec.ID] = nil
				fixture.manager.mu.Unlock()
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "connection epoch increment",
			mutate: func(_ *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				fixture.manager.mu.Lock()
				fixture.manager.connectionEpochs[fixture.spec.ID]++
				fixture.manager.mu.Unlock()
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "server identity replacement",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				replacement, err := domainsecurity.NewVerifiedMCPServerIdentity(
					fixture.spec.ID,
					fixture.spec.ExpectedServerName,
					fixture.spec.ExpectedServerVersion,
					domainsecurity.SHA256Hex([]byte("replacement-current-probe-runtime")),
					fixture.connectionEpoch,
				)
				if err != nil {
					t.Fatal(err)
				}
				fixture.manager.mu.Lock()
				fixture.manager.serverIdentities[fixture.spec.ID] = replacement
				fixture.manager.mu.Unlock()
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "configured provenance replacement",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				replacement := pinnedFundsSourceSpec(t, ServerSpec{
					ID: fixture.spec.ID, ExpectedServerName: fixture.spec.ExpectedServerName,
					ExpectedServerVersion: fixture.spec.ExpectedServerVersion,
				})
				fixture.manager.mu.Lock()
				fixture.manager.specs = []ServerSpec{replacement}
				fixture.manager.specFingerprints[fixture.spec.ID] = SpecFingerprint(replacement)
				fixture.manager.mu.Unlock()
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "configured source mutation",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				if err := os.WriteFile(fixture.spec.EntrypointPath, []byte("export const server = 'mutated'\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return context.Background(), fixture.securityContext
			},
		},
		{
			name: "wrong thread",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.ThreadID = "thread-current-probe-other"
				})
			},
		},
		{
			name: "wrong turn",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.TurnID = "turn-current-probe-other"
				})
			},
		},
		{
			name: "wrong case",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.CaseID = "case-current-probe-other"
				})
			},
		},
		{
			name: "wrong binding",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("current-probe-other-binding"))
				})
			},
		},
		{
			name: "wrong context epoch",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.ContextEpoch++
				})
			},
		},
		{
			name: "wrong context digest",
			mutate: func(_ *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				changed := fixture.securityContext
				changed.ContextDigest = domainsecurity.SHA256Hex([]byte("current-probe-other-context"))
				return context.Background(), changed
			},
		},
		{
			name: "wrong dataset snapshot",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
					input.DatasetSnapshotID = domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("current-probe-other-snapshot"))
				})
			},
		},
		{
			name: "audit V1 context",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				base := fixture.securityContext
				issuedAt, err := time.Parse(time.RFC3339Nano, base.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				return context.Background(), domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
					ThreadID: base.ThreadID, TurnID: base.TurnID, WorkspaceRealPath: base.WorkspaceRealPath,
					TenantID: base.TenantID, UserID: base.UserID, CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash,
					DatasetSnapshotID: base.DatasetSnapshotID, SourceManifestHash: base.SourceManifestHash,
					ContextEpoch: base.ContextEpoch, IssuedAt: issuedAt,
				})
			},
		},
		{
			name: "boundary V2 context",
			mutate: func(t *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return context.Background(), currentProbeBoundaryContextForTest(t, fixture.securityContext)
			},
		},
		{
			name: "nil operation context",
			mutate: func(_ *testing.T, fixture *currentProbeValidationFixture) (context.Context, domainsecurity.TurnSecurityContext) {
				return nil, fixture.securityContext
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCurrentProbeValidationFixture(t)
			operationContext, securityContext := test.mutate(t, fixture)
			before := sourceProbeClientCallCountsForTest(fixture.client)
			if err := fixture.manager.ValidateCurrentProbe(operationContext, fixture.spec.ID, securityContext); err == nil {
				t.Fatal("mismatched or unavailable current source authority was accepted")
			}
			assertSourceProbeValidationDidNotCallClient(t, fixture.client, before)
		})
	}

	t.Run("nil manager", func(t *testing.T) {
		fixture := newCurrentProbeValidationFixture(t)
		before := sourceProbeClientCallCountsForTest(fixture.client)
		var manager *ProductionManager
		if err := manager.ValidateCurrentProbe(context.Background(), fixture.spec.ID, fixture.securityContext); err == nil {
			t.Fatal("nil production manager accepted current source authority")
		}
		assertSourceProbeValidationDidNotCallClient(t, fixture.client, before)
	})
}

func TestProductionManagerValidateCurrentProbeSerializesConcurrentDisconnect(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	fixture.client.closeStarted = make(chan struct{})
	fixture.client.closeRelease = make(chan struct{})

	disconnectDone := make(chan struct{})
	go func() {
		fixture.manager.Disconnect()
		close(disconnectDone)
	}()
	<-fixture.client.closeStarted

	const validators = 64
	validationErrors := make(chan error, validators)
	for index := 0; index < validators; index++ {
		go func() {
			validationErrors <- fixture.manager.ValidateCurrentProbe(context.Background(), fixture.spec.ID, fixture.securityContext)
		}()
	}
	select {
	case err := <-validationErrors:
		close(fixture.client.closeRelease)
		<-disconnectDone
		t.Fatalf("current validation crossed the disconnect lease: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(fixture.client.closeRelease)
	<-disconnectDone
	for index := 0; index < validators; index++ {
		if err := <-validationErrors; err == nil {
			t.Fatal("concurrent validation accepted a probe after disconnect")
		}
	}
	for index := 0; index < validators; index++ {
		if err := fixture.manager.ValidateCurrentProbe(context.Background(), fixture.spec.ID, fixture.securityContext); err == nil {
			t.Fatal("post-disconnect validation accepted stale source authority")
		}
	}
	if fixture.client.transportCalls != 0 || fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 || fixture.client.toolCalls != 0 || fixture.client.closeCalls != 1 {
		t.Fatalf("pure current validation invoked MCP I/O: client=%#v", fixture.client)
	}
}

type currentProbeValidationFixture struct {
	manager         *ProductionManager
	client          *nativeSourceProbeClient
	spec            ServerSpec
	securityContext domainsecurity.TurnSecurityContext
	connectionEpoch uint64
}

func TestSourceProbeCatalogContractDamageRevokesLiveAuthority(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	_, contractErr := mcpprotocol.CollectToolCatalogPages(context.Background(), func(context.Context, map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"tools":[{"name":"bad name","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`), nil
	})
	if contractErr == nil || !mcpprotocol.IsToolCatalogContractError(contractErr) {
		t.Fatalf("test did not construct catalog contract damage: %v", contractErr)
	}
	fixture.client.catalogErr = contractErr
	securityContext := fixture.securityContext
	_, err := fixture.manager.ProbeCaseSource(context.Background(), sourceprobeport.Input{
		ServerID: fixture.spec.ID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest,
	})
	if err == nil {
		t.Fatal("legacy snapshot reached structurally damaged current-run catalog")
	}
	if err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable ||
		fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 || fixture.client.closeCalls != 0 ||
		fixture.manager.clients[fixture.spec.ID] != fixture.client || fixture.manager.serverIdentities[fixture.spec.ID] == "" ||
		fixture.manager.connectionEpochs[fixture.spec.ID] != fixture.connectionEpoch ||
		len(fixture.manager.LiveToolsForSecurityContext(securityContext)) != 0 {
		t.Fatalf("legacy snapshot crossed catalog quarantine: err=%v client=%#v epochs=%#v identities=%#v tools=%#v",
			err, fixture.client, fixture.manager.connectionEpochs, fixture.manager.serverIdentities, fixture.manager.Tools())
	}
	if err := fixture.manager.ValidateCurrentProbe(context.Background(), fixture.spec.ID, securityContext); err == nil {
		t.Fatal("revoked catalog retained a current source probe")
	}
}

func newCurrentProbeValidationFixture(t *testing.T) *currentProbeValidationFixture {
	t.Helper()
	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("current-probe-binding"))
	snapshotID := domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("current-probe-snapshot"))
	spec := pinnedFundsSourceSpec(t, ServerSpec{
		ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.15",
	})
	client := &nativeSourceProbeClient{
		tools: []ToolSpec{{
			Name: "get_case_status", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), ReadOnlyHint: true,
		}},
		response: map[string]any{
			"version": float64(1), "serverName": spec.ExpectedServerName, "serverVersion": spec.ExpectedServerVersion,
			"caseId": "case-current-probe", "caseBindingHash": bindingHash, "datasetSnapshotId": snapshotID,
			"ready": true, "readOnly": true, "blocker": "", "checkedAt": now.Format(time.RFC3339Nano),
		},
	}
	const connectionEpoch = uint64(23)
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.clients[spec.ID] = client
	manager.connectionEpochs[spec.ID] = connectionEpoch
	manager.serverIdentities[spec.ID] = mcpTestVerifiedIdentity(t, spec.ID, spec.ExpectedServerName, spec.ExpectedServerVersion, connectionEpoch)
	manager.specFingerprints[spec.ID] = SpecFingerprint(spec)
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-current-probe", TurnID: "turn-current-probe", WorkspaceRealPath: "/workspace/current-probe",
		CaseID: "case-current-probe", CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotID,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("current-probe-source-manifest")), ContextEpoch: 7, IssuedAt: now,
	})
	if err := manager.registerToolsNoLock(spec, client.tools); err != nil {
		t.Fatal(err)
	}
	manager.refreshCatalogStateNoLock()
	catalogFingerprint := sourceProbeCatalogFingerprintWithIssues(client.tools, nil)
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: spec.ID, ServerIdentity: manager.serverIdentities[spec.ID], ConnectionEpoch: connectionEpoch,
		CatalogFingerprint: catalogFingerprint, SpecFingerprint: SpecFingerprint(spec),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: now,
		Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: spec.ExpectedServerName, ServerVersion: spec.ExpectedServerVersion,
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
			DatasetSnapshotID: securityContext.DatasetSnapshotID, Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.ServerIdentity != manager.serverIdentities[spec.ID] || probe.ConnectionEpoch != connectionEpoch ||
		probe.SpecFingerprint != SpecFingerprint(spec) || probe.ProbeContextDigest != securityContext.ContextDigest {
		t.Fatalf("fixture did not establish an audit-readable legacy probe: probe=%#v", probe)
	}
	manager.sourceCatalogs[spec.ID] = catalogFingerprint
	manager.sourceProbes[sourceProbeRegistryKey(spec.ID, securityContext.ThreadID, securityContext.TurnID)] = probe
	return &currentProbeValidationFixture{
		manager: manager, client: client, spec: spec, securityContext: securityContext, connectionEpoch: connectionEpoch,
	}
}

func (fixture *currentProbeValidationFixture) contextWith(t *testing.T, mutate func(*domainsecurity.TurnSecurityContextInput)) domainsecurity.TurnSecurityContext {
	t.Helper()
	base := fixture.securityContext
	issuedAt, err := time.Parse(time.RFC3339Nano, base.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{
		ThreadID: base.ThreadID, TurnID: base.TurnID, WorkspaceRealPath: base.WorkspaceRealPath,
		TenantID: base.TenantID, UserID: base.UserID, CaseID: base.CaseID, CaseBindingHash: base.CaseBindingHash,
		DatasetSnapshotID: base.DatasetSnapshotID, SourceManifestHash: base.SourceManifestHash,
		ContextEpoch: base.ContextEpoch, IssuedAt: issuedAt,
	}
	mutate(&input)
	return newExecutableCaseSecurityContext(t, input)
}

type sourceProbeClientCallCounts struct {
	transport int
	list      int
	native    int
	tool      int
	close     int
}

func sourceProbeClientCallCountsForTest(client *nativeSourceProbeClient) sourceProbeClientCallCounts {
	return sourceProbeClientCallCounts{
		transport: client.transportCalls, list: client.listCalls, native: client.nativeCalls, tool: client.toolCalls, close: client.closeCalls,
	}
}

func assertSourceProbeValidationDidNotCallClient(t *testing.T, client *nativeSourceProbeClient, before sourceProbeClientCallCounts) {
	t.Helper()
	if after := sourceProbeClientCallCountsForTest(client); after != before {
		t.Fatalf("pure current validation invoked MCP transport/native/lifecycle I/O: before=%#v after=%#v", before, after)
	}
}

func currentProbeBoundaryContextForTest(t *testing.T, base domainsecurity.TurnSecurityContext) domainsecurity.TurnSecurityContext {
	t.Helper()
	issuedAt, err := time.Parse(time.RFC3339Nano, base.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("current-probe-boundary-policy")),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateInvalid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("current-probe-boundary-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingInvalid,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: base.ThreadID, TurnID: base.TurnID, WorkspaceRealPath: base.WorkspaceRealPath,
		TenantID: base.TenantID, UserID: base.UserID, CaseID: domainsecurity.UnboundCaseID,
		CaseBindingHash:   domainsecurity.UnboundCaseBindingHash(base.WorkspaceRealPath),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: base.ContextEpoch, IssuedAt: issuedAt, PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func TestProductionManagerRejectsPluginSelectedDatasetSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("plugin-selected-snapshot-binding"))
	expectedSnapshotID := domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("host-frozen-snapshot"))
	otherSnapshotID := domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("plugin-selected-snapshot"))
	spec := pinnedFundsSourceSpec(t, ServerSpec{
		ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.15",
	})
	client := &nativeSourceProbeClient{
		tools: []ToolSpec{{
			Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
		response: map[string]any{
			"version": float64(1), "serverName": "analytix_funds", "serverVersion": "0.16.15", "caseId": "case-plugin-selected",
			"caseBindingHash": bindingHash, "datasetSnapshotId": otherSnapshotID, "ready": true, "readOnly": true,
			"blocker": "", "checkedAt": now.Format(time.RFC3339Nano),
		},
	}
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.clients[spec.ID] = client
	manager.connectionEpochs[spec.ID] = 5
	manager.serverIdentities[spec.ID] = mcpTestVerifiedIdentity(t, spec.ID, spec.ExpectedServerName, spec.ExpectedServerVersion, 5)
	manager.specFingerprints[spec.ID] = SpecFingerprint(spec)
	_, err := manager.ProbeCaseSource(context.Background(), sourceprobeport.Input{
		ServerID: spec.ID, WorkspaceRealPath: "/workspace", ThreadID: "thread-plugin-selected", TurnID: "turn-plugin-selected",
		CaseID: "case-plugin-selected", CaseBindingHash: bindingHash, DatasetSnapshotID: expectedSnapshotID,
		ContextEpoch: 2, ContextDigest: domainsecurity.SHA256Hex([]byte("plugin-selected-context")),
	})
	if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable ||
		client.listCalls != 0 || client.nativeCalls != 0 || len(manager.sourceProbes) != 0 {
		t.Fatalf("legacy host snapshot reached plugin-selected snapshot response: err=%v client=%#v probes=%#v", err, client, manager.sourceProbes)
	}
}

func TestFatalTransportFailureRevokesServerIdentityAndSourceProbe(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	toolName := CanonicalToolName(fixture.spec.ID, "get_case_status")
	arguments := map[string]any{}
	body, _ := json.Marshal(arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-fatal-quarantine",
		ServerIdentity: fixture.manager.serverIdentities[fixture.spec.ID],
		ToolName:       toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("source-probe-fatal-quarantine"), ConnectionEpoch: fixture.connectionEpoch,
		ArgsHash:   domainsecurity.CanonicalJSONHash(body),
		SchemaHash: managedToolSchemaHash(toolName, fixture.manager.tools[toolName]),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("fatal-quarantine-scope")),
		ReadOnly:   false, ApprovalState: "approved", IssuedAt: time.Now().UTC(),
	})
	envelope, err := domainmcp.NewHostContextEnvelope(fixture.securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	fixture.client.callErr = fatalManagerTransportError{}
	result := fixture.manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, arguments)
	if result["executed"] != false || result["code"] != "mcp_host_context_invalid" ||
		result["result"] != nil || fixture.client.toolCalls != 0 || fixture.client.closeCalls != 0 {
		t.Fatalf("legacy snapshot reached fatal transport or factual fallback: result=%#v client=%#v", result, fixture.client)
	}
	if live := fixture.manager.LiveToolsForSecurityContext(fixture.securityContext); len(live) != 0 {
		t.Fatalf("legacy fatal fixture retained factual advertisement: %#v", live)
	}
	if fixture.manager.clients[fixture.spec.ID] != fixture.client ||
		fixture.manager.serverIdentities[fixture.spec.ID] == "" ||
		fixture.manager.connectionEpochs[fixture.spec.ID] != fixture.connectionEpoch {
		t.Fatalf("unreached fatal transport mutated connection authority: client=%#v identities=%#v epochs=%#v",
			fixture.manager.clients, fixture.manager.serverIdentities, fixture.manager.connectionEpochs)
	}
}
func TestNativeProbeFatalFailureRevokesIdentityAndAllProbes(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	fixture.client.listErr = fatalManagerTransportError{}
	_, err := fixture.manager.ProbeCaseSource(context.Background(), sourceprobeport.Input{
		ServerID: fixture.spec.ID, WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		CaseID: fixture.securityContext.CaseID, CaseBindingHash: fixture.securityContext.CaseBindingHash,
		DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID, ContextEpoch: fixture.securityContext.ContextEpoch,
		ContextDigest: fixture.securityContext.ContextDigest,
	})
	if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("legacy snapshot reached fatal native probe injection: %v", err)
	}
	if fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 || fixture.client.closeCalls != 0 ||
		len(fixture.manager.sourceProbes) != 0 {
		t.Fatalf("legacy snapshot performed native probe I/O or retained probe authority: client=%#v probes=%#v",
			fixture.client, fixture.manager.sourceProbes)
	}
	if fixture.manager.clients[fixture.spec.ID] != fixture.client ||
		fixture.manager.serverIdentities[fixture.spec.ID] == "" ||
		fixture.manager.connectionEpochs[fixture.spec.ID] != fixture.connectionEpoch {
		t.Fatalf("unreached fatal probe mutated connection authority: clients=%#v identities=%#v epochs=%#v",
			fixture.manager.clients, fixture.manager.serverIdentities, fixture.manager.connectionEpochs)
	}
}
func TestNativeEvidenceFatalFailureRevokesIdentityAndAllProbes(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	arguments := json.RawMessage(`{"table_name":"analysis_txn_detail_idx"}`)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-native-evidence-quarantine",
		ServerIdentity: fixture.manager.serverIdentities[fixture.spec.ID],
		ToolName:       fundsCountEvidenceToolName, ToolCallID: toolidentity.MustHostToolCallIDV1("source-probe-native-evidence-quarantine"),
		ConnectionEpoch: fixture.connectionEpoch, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("native-evidence-quarantine-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("native-evidence-quarantine-scope")),
		ReadOnly:   true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	fixture.client.losslessErr = fatalManagerTransportError{}
	callbackCalled := false
	err := fixture.manager.WithCurrentEvidenceRead(context.Background(), sourceprobeport.EvidenceReadInput{
		Context: fixture.securityContext, Grant: grant, Arguments: arguments,
	}, func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error {
		callbackCalled = true
		return nil
	})
	if err == nil || callbackCalled {
		t.Fatalf("legacy snapshot reached fatal native evidence injection: called=%v err=%v", callbackCalled, err)
	}
	if fixture.client.nativeCalls != 0 || fixture.client.closeCalls != 0 || fixture.client.toolCalls != 0 ||
		len(fixture.manager.LiveToolsForSecurityContext(fixture.securityContext)) != 0 {
		t.Fatalf("legacy evidence failure crossed source boundary: client=%#v", fixture.client)
	}
	if fixture.manager.clients[fixture.spec.ID] != fixture.client ||
		fixture.manager.serverIdentities[fixture.spec.ID] == "" ||
		fixture.manager.connectionEpochs[fixture.spec.ID] != fixture.connectionEpoch {
		t.Fatalf("unreached fatal evidence read mutated connection authority: clients=%#v identities=%#v epochs=%#v",
			fixture.manager.clients, fixture.manager.serverIdentities, fixture.manager.connectionEpochs)
	}
}
func TestProductionManagerRejectsCachedOrSpoofedSourceProbe(t *testing.T) {
	bindingHash := domainsecurity.SHA256Hex([]byte("binding"))
	snapshotID := domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("snapshot-1"))
	spec := ServerSpec{ID: "analytix_funds", ExpectedServerName: "analytix_funds", ExpectedServerVersion: "0.16.15", IdentitySource: "manifest"}
	input := sourceprobeport.Input{
		ServerID: spec.ID, WorkspaceRealPath: "/workspace", ThreadID: "thread_1", TurnID: "turn_1", CaseID: "case_1234", CaseBindingHash: bindingHash,
		DatasetSnapshotID: snapshotID, ContextEpoch: 2, ContextDigest: domainsecurity.SHA256Hex([]byte("context")),
	}
	manager := NewProductionManager(nil)
	manager.specs = []ServerSpec{spec}
	manager.cachedSchemas[spec.ID] = true
	manager.specFingerprints[spec.ID] = SpecFingerprint(spec)
	if _, err := manager.ProbeCaseSource(context.Background(), input); err == nil {
		t.Fatal("cached catalog without connected native transport satisfied source readiness")
	}
	client := &nativeSourceProbeClient{
		tools: []ToolSpec{{Name: "get_case_status", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}},
		response: map[string]any{
			"version": float64(1), "serverName": "spoofed_funds", "serverVersion": "0.16.15", "caseId": "case_1234",
			"caseBindingHash": bindingHash, "datasetSnapshotId": snapshotID, "ready": true, "readOnly": true,
			"blocker": "", "checkedAt": "2026-07-10T12:00:00Z",
		},
	}
	manager.clients[spec.ID] = client
	manager.connectionEpochs[spec.ID] = 1
	manager.serverIdentities[spec.ID] = mcpTestVerifiedIdentity(t, spec.ID, spec.ExpectedServerName, spec.ExpectedServerVersion, 1)
	if _, err := manager.ProbeCaseSource(context.Background(), input); err == nil {
		t.Fatal("spoofed server identity satisfied case source readiness")
	}
	if diagnostics := manager.ServerDiagnostics()[0].(map[string]any); diagnostics["sourceReady"] != false {
		t.Fatalf("failed/spoofed probe remained ready: %#v", diagnostics)
	}
}

func TestFundsFinalDispatchSerializesProbeSwitchBeforeTransport(t *testing.T) {
	fixture := newCurrentProbeValidationFixture(t)
	toolName := CanonicalToolName(fixture.spec.ID, "get_case_status")
	arguments := map[string]any{}
	body, _ := json.Marshal(arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-dispatch-quarantine",
		ServerIdentity: fixture.manager.serverIdentities[fixture.spec.ID],
		ToolName:       toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("source-probe-dispatch-quarantine"), ConnectionEpoch: fixture.connectionEpoch,
		ArgsHash:   domainsecurity.CanonicalJSONHash(body),
		SchemaHash: managedToolSchemaHash(toolName, fixture.manager.tools[toolName]),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("dispatch-quarantine-scope")),
		ReadOnly:   false, ApprovalState: "approved", IssuedAt: time.Now().UTC(),
	})
	envelope, err := domainmcp.NewHostContextEnvelope(fixture.securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	callDone := make(chan map[string]any, 1)
	go func() {
		callDone <- fixture.manager.CallToolSecurityBoundContext(context.Background(), toolName, true, envelope, arguments)
	}()

	contextB := fixture.contextWith(t, func(input *domainsecurity.TurnSecurityContextInput) {
		input.ThreadID = "thread-dispatch-b"
		input.TurnID = "turn-dispatch-b"
		input.CaseID = "case-dispatch-b"
		input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("dispatch-binding-b"))
		input.DatasetSnapshotID = domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("dispatch-snapshot-b"))
	})
	probeDone := make(chan error, 1)
	go func() {
		_, err := fixture.manager.ProbeCaseSource(context.Background(), sourceprobeport.Input{
			ServerID: fixture.spec.ID, WorkspaceRealPath: contextB.WorkspaceRealPath,
			ThreadID: contextB.ThreadID, TurnID: contextB.TurnID, CaseID: contextB.CaseID,
			CaseBindingHash: contextB.CaseBindingHash, DatasetSnapshotID: contextB.DatasetSnapshotID,
			ContextEpoch: contextB.ContextEpoch, ContextDigest: contextB.ContextDigest,
		})
		probeDone <- err
	}()

	select {
	case result := <-callDone:
		if result["executed"] != false || result["result"] != nil {
			t.Fatalf("legacy case call produced a factual fallback: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy case call did not fail closed promptly")
	}
	select {
	case err := <-probeDone:
		if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
			t.Fatalf("legacy case-B probe did not return fixed blocker: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy case-B probe did not fail closed promptly")
	}
	if fixture.client.toolCalls != 0 || fixture.client.listCalls != 0 || fixture.client.nativeCalls != 0 ||
		len(fixture.manager.LiveToolsForSecurityContext(fixture.securityContext)) != 0 ||
		len(fixture.manager.LiveToolsForSecurityContext(contextB)) != 0 {
		t.Fatalf("legacy concurrent dispatch crossed transport or case isolation: client=%#v", fixture.client)
	}
}
func newExecutableCaseSecurityContext(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	if input.SourceManifestHash == "" {
		input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	}
	threadPolicyDigest := domainsecurity.SHA256Hex([]byte("source-probe-thread-risk:" + input.ThreadID))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: threadPolicyDigest,
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:       domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"source-probe-binding-observation:" + input.CaseID + ":" + input.CaseBindingHash,
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = domainsecurity.RiskAuthorityBindingV1{
		SchemaVersion: domainsecurity.RiskAuthorityBindingSchemaVersion,
		Purpose:       domainsecurity.RiskAuthorityBindingPurpose,
		State:         domainsecurity.RiskAuthorityBindingStateWitnessed,
		IndexDigest:   domainsecurity.SHA256Hex([]byte("source-probe-risk-index:" + input.ThreadID)),
		Generation:    1,
		CheckpointDigest: domainsecurity.SHA256Hex([]byte(
			"source-probe-risk-checkpoint:" + input.ThreadID,
		)),
		ObservationDigest: domainsecurity.SHA256Hex([]byte(
			"source-probe-risk-observation:" + input.ThreadID,
		)),
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func pinnedFundsSourceSpec(t *testing.T, spec ServerSpec) ServerSpec {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(root, "server.mjs")
	body := []byte("export const server = 'analytix_funds'\n")
	if err := os.WriteFile(entrypoint, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestBody, err := json.Marshal(map[string]string{
		"name":    "analytix-fund-analysis",
		"version": spec.ExpectedServerVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	if err := os.MkdirAll(filepath.Join(root, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex-plugin", "plugin.json"), manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	treeDigest, err := mcpidentity.ComputeSourceTreeSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	spec.Transport = "stdio"
	spec.Command = entrypoint
	spec.IdentitySource = "installed-plugin-manifest"
	spec.ManifestSHA256 = domainsecurity.SHA256Hex(manifestBody)
	spec.EntrypointPath = entrypoint
	spec.EntrypointSHA256 = domainsecurity.SHA256Hex(body)
	spec.PluginRootPath = root
	spec.SourceTreeSHA256 = treeDigest
	return spec
}

func sourceProbeHostContext(t *testing.T, manager *ProductionManager, securityContext domainsecurity.TurnSecurityContext, toolName string, connectionEpoch uint64, arguments map[string]any) domainmcp.HostContextEnvelope {
	return sourceProbeHostContextAt(t, manager, securityContext, toolName, connectionEpoch, arguments, time.Now().UTC())
}

func sourceProbeHostContextAt(t *testing.T, manager *ProductionManager, securityContext domainsecurity.TurnSecurityContext, toolName string, connectionEpoch uint64, arguments map[string]any, issuedAt time.Time) domainmcp.HostContextEnvelope {
	t.Helper()
	body, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	serverID, rawName, parsed := domainmcpname.Parse(toolName)
	if !parsed {
		t.Fatalf("invalid canonical MCP tool name %q", toolName)
	}
	manager.mu.Lock()
	serverIdentity := manager.serverIdentities[serverID]
	spec, specOK := manager.specByIDNoLock(serverID)
	tool, toolOK := manager.tools[toolName]
	hostReadOnly := specOK && spec.ReadOnlyToolNames[rawName]
	manager.mu.Unlock()
	if serverIdentity == "" {
		t.Fatalf("missing verified server identity for %s", serverID)
	}
	if !toolOK {
		t.Fatalf("missing managed tool %s", toolName)
	}
	approvalState := "approved"
	if hostReadOnly {
		approvalState = "not_required"
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-test",
		ServerIdentity: serverIdentity,
		ToolName:       toolName, ToolCallID: toolidentity.MustHostToolCallIDV1("source-probe-test:" + securityContext.ContextDigest), ConnectionEpoch: connectionEpoch,
		ArgsHash: domainsecurity.CanonicalJSONHash(body), SchemaHash: managedToolSchemaHash(toolName, tool),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: hostReadOnly, ApprovalState: approvalState, IssuedAt: issuedAt,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}
