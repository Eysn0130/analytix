package executiongrant

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestHostGeneralOnlyRiskKeepsOrdinaryGrantsAndRejectsCaseEffects(t *testing.T) {
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-grant", TurnID: "turn-general-grant", WorkspaceRealPath: "/workspace/general-grant",
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	ordinary := []struct {
		name          string
		readOnly      bool
		approvalState string
		mcpEpoch      uint64
		mcpIdentity   string
	}{
		{name: "get_goal", readOnly: true, approvalState: "not_required"},
		{name: "read", readOnly: true, approvalState: "not_required"},
		{name: "read_file", readOnly: true, approvalState: "not_required"},
		{name: "grep", readOnly: true, approvalState: "not_required"},
		{name: "glob", readOnly: true, approvalState: "not_required"},
		{name: "find", readOnly: true, approvalState: "not_required"},
		{name: "code_index", readOnly: true, approvalState: "not_required"},
		{name: "ls", readOnly: true, approvalState: "not_required"},
		{name: "web_fetch", readOnly: true, approvalState: "not_required"},
		{name: "write", readOnly: false, approvalState: "pending"},
		{name: "write_file", readOnly: false, approvalState: "pending"},
		{name: "bash", readOnly: false, approvalState: "pending"},
		{name: "run_skill", readOnly: false, approvalState: "pending"},
		{name: "task", readOnly: false, approvalState: "pending"},
		{
			name: "mcp__docs__lookup", readOnly: true, approvalState: "not_required", mcpEpoch: 7,
			mcpIdentity: executionGrantTestMCPIdentity(t, "docs", "docs", "1.0.0", 7),
		},
	}
	for _, test := range ordinary {
		t.Run("ordinary/"+test.name, func(t *testing.T) {
			call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("general-" + test.name), Name: test.name, Arguments: json.RawMessage(`{}`)}
			grant, err := IssueProvider(
				securityContext, "provider", call,
				[]domainmodel.ToolSchema{{Name: test.name, Parameters: closed}}, []string{test.name},
				test.readOnly, test.approvalState, test.mcpEpoch, test.mcpIdentity, now,
			)
			if err != nil {
				t.Fatalf("ordinary tool grant was rejected by unavailable case authority: %v", err)
			}
			if grant.ReadOnly != test.readOnly || grant.ApprovalState != test.approvalState {
				t.Fatalf("ordinary tool bypassed its normal grant policy: %#v", grant)
			}
		})
	}

	fundsName := "mcp__analytix_funds__count_case_rows"
	fundsCall := domainmodel.ToolCall{
		ID: executionGrantTestToolCallID("general-funds"), Name: fundsName, Arguments: json.RawMessage(`{}`),
	}
	_, err = IssueProvider(
		securityContext, "provider", fundsCall,
		[]domainmodel.ToolSchema{{Name: fundsName, Parameters: closed}}, []string{fundsName}, true, "not_required", 9,
		executionGrantTestMCPIdentity(t, "analytix_funds", "analytix_funds", "1.0.0", 9), now,
	)
	if err == nil || err.Error() != "execution_grant_case_authority_required" {
		t.Fatalf("funds tool received a grant without case authority: %v", err)
	}

	reportCall := domainmodel.ToolCall{
		ID: executionGrantTestToolCallID("general-report"), Name: toolcatalogapp.ReportDeliveryToolName, Arguments: json.RawMessage(`{}`),
	}
	_, err = IssueProvider(
		securityContext, "provider", reportCall,
		[]domainmodel.ToolSchema{{Name: reportCall.Name, Parameters: closed}}, []string{reportCall.Name}, false, "pending", 0, "", now,
	)
	if err == nil || err.Error() != "execution_grant_case_authority_required" {
		t.Fatalf("case report received a grant without case authority: %v", err)
	}
}

func TestWitnessedBoundaryIssuesOrdinaryGrantsAndRejectsProtectedCaseCalls(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantBoundaryFixture(
		t, "thread-boundary-grant", "turn-boundary-grant", "/workspace/boundary-grant", "case-boundary-grant", now,
	)
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	ordinary := []struct {
		name          string
		readOnly      bool
		approvalState string
		mcpEpoch      uint64
		mcpIdentity   string
	}{
		{name: "read", readOnly: true, approvalState: "not_required"},
		{name: "write_file", readOnly: false, approvalState: "pending"},
		{name: "bash", readOnly: false, approvalState: "pending"},
		{name: "todo_write", readOnly: false, approvalState: "pending"},
		{name: "task", readOnly: false, approvalState: "pending"},
		{
			name: "mcp__docs__lookup", readOnly: true, approvalState: "not_required", mcpEpoch: 7,
			mcpIdentity: executionGrantTestMCPIdentity(t, "docs", "docs", "1.0.0", 7),
		},
	}
	for _, test := range ordinary {
		t.Run("ordinary/"+test.name, func(t *testing.T) {
			call := domainmodel.ToolCall{
				ID: executionGrantTestToolCallID("boundary-" + test.name), Name: test.name, Arguments: json.RawMessage(`{}`),
			}
			grant, err := IssueProvider(
				fixture.Context, "provider", call,
				[]domainmodel.ToolSchema{{Name: test.name, Parameters: closed}}, []string{test.name},
				test.readOnly, test.approvalState, test.mcpEpoch, test.mcpIdentity, now,
			)
			if err != nil {
				t.Fatalf("ordinary boundary grant was rejected: %v", err)
			}
			authorization := AuthorizationInput{
				OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
				Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: grant,
				Provider: "provider", Call: call, ToolScope: []string{test.name},
				ExpectedApprovalState: test.approvalState, Now: now.Add(time.Minute),
			}
			if test.mcpEpoch > 0 {
				authorization.LiveMCPTools = map[string]bool{test.name: true}
				authorization.MCPServerIdentities = map[string]string{test.name: test.mcpIdentity}
			}
			if err := Authorize(authorization); err != nil {
				t.Fatalf("current ordinary boundary grant was rejected: %v", err)
			}
		})
	}

	protected := []struct {
		name         string
		mcpEpoch     uint64
		mcpIdentity  string
		expectedCode string
	}{
		{
			name: "mcp__analytix_funds__count_case_rows", mcpEpoch: 8,
			mcpIdentity:  executionGrantTestMCPIdentity(t, "analytix_funds", "analytix_funds", "1.0.0", 8),
			expectedCode: "execution_grant_context_invalid",
		},
		{name: toolcatalogapp.ReportDeliveryToolName, expectedCode: "execution_grant_context_invalid"},
		{
			name: "mcp__docs__run_full_case_analysis", mcpEpoch: 9,
			mcpIdentity:  executionGrantTestMCPIdentity(t, "docs", "docs", "1.0.0", 9),
			expectedCode: "execution_grant_context_invalid",
		},
	}
	for _, test := range protected {
		t.Run("protected/"+test.name, func(t *testing.T) {
			call := domainmodel.ToolCall{
				ID: executionGrantTestToolCallID("boundary-protected-" + test.name), Name: test.name, Arguments: json.RawMessage(`{}`),
			}
			_, err := IssueProvider(
				fixture.Context, "provider", call,
				[]domainmodel.ToolSchema{{Name: test.name, Parameters: closed}}, []string{test.name},
				true, "not_required", test.mcpEpoch, test.mcpIdentity, now,
			)
			if err == nil || err.Error() != test.expectedCode {
				t.Fatalf("protected call used caller readOnly to forge ordinary classification: %v", err)
			}
		})
	}
}

func TestCallUsesCaseDataAuthoritySharesTheGrantClassifier(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "read", want: false},
		{name: "write_file", want: false},
		{name: "bash", want: false},
		{name: "todo_write", want: false},
		{name: "task", want: false},
		{name: "mcp__docs__lookup", want: false},
		{name: "mcp__analytix_funds__analyze_account_flows", want: true},
		{name: toolcatalogapp.ReportDeliveryToolName, want: true},
		{name: "mcp__docs__run_full_case_analysis", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := domainmodel.ToolCall{Name: test.name, Arguments: json.RawMessage(`{}`)}
			if got := CallUsesCaseDataAuthority(call); got != test.want {
				t.Fatalf("host call classifier drifted for %q: got=%t want=%t", test.name, got, test.want)
			}
		})
	}
}

func TestWitnessedBoundaryOrdinaryAuthorizationRejectsAuthorityDrift(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)
	newInput := func(t *testing.T, suffix string) (*executionGrantSecurityFixture, AuthorizationInput) {
		t.Helper()
		fixture := newExecutionGrantBoundaryFixture(
			t, "thread-boundary-"+suffix, "turn-boundary-"+suffix,
			"/workspace/boundary-"+suffix, "case-boundary-"+suffix, now,
		)
		call := domainmodel.ToolCall{
			ID: executionGrantTestToolCallID("boundary-drift-" + suffix), Name: "read", Arguments: json.RawMessage(`{}`),
		}
		schemas := []domainmodel.ToolSchema{{
			Name: call.Name, Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}}
		grant, err := IssueProvider(
			fixture.Context, "provider", call, schemas, []string{call.Name}, true, "not_required", 0, "", now,
		)
		if err != nil {
			t.Fatal(err)
		}
		return fixture, AuthorizationInput{
			OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
			Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: grant,
			Provider: "provider", Call: call, ToolScope: []string{call.Name},
			ExpectedApprovalState: "not_required", Now: now.Add(time.Minute),
		}
	}

	t.Run("risk", func(t *testing.T) {
		fixture, input := newInput(t, "risk")
		fixture.Risk.markStale()
		if err := Authorize(input); err == nil || err.Error() != "turn_security_risk_policy_mismatch" {
			t.Fatalf("risk drift was not rejected: %v", err)
		}
	})
	t.Run("identity", func(t *testing.T) {
		_, input := newInput(t, "identity")
		input.SecurityAuthority.Identity = nil
		if err := Authorize(input); err == nil || err.Error() != "execution_grant_context_invalid" {
			t.Fatalf("identity drift was not rejected: %v", err)
		}
	})
	t.Run("workspace", func(t *testing.T) {
		fixture, input := newInput(t, "workspace")
		fixture.Observer.set(executionGrantCaseObservation(
			t, fixture.Context.WorkspaceRealPath+"-moved", "case-boundary-workspace", "workspace-moved",
		))
		if err := Authorize(input); err == nil || err.Error() != "turn_security_workspace_mismatch" {
			t.Fatalf("workspace drift was not rejected: %v", err)
		}
	})
	t.Run("binding", func(t *testing.T) {
		fixture, input := newInput(t, "binding")
		fixture.Observer.set(executionGrantCaseObservation(
			t, fixture.Context.WorkspaceRealPath, "case-boundary-rebound", "binding-rebound",
		))
		if err := Authorize(input); err == nil || err.Error() != "turn_security_case_binding_mismatch" {
			t.Fatalf("case-binding drift was not rejected: %v", err)
		}
	})
	t.Run("epoch", func(t *testing.T) {
		fixture, input := newInput(t, "epoch")
		issuedAt, err := time.Parse(time.RFC3339Nano, fixture.Context.IssuedAt)
		if err != nil {
			t.Fatal(err)
		}
		drifted, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: fixture.Context.ThreadID, TurnID: fixture.Context.TurnID,
			WorkspaceRealPath: fixture.Context.WorkspaceRealPath,
			TenantID:          fixture.Context.TenantID, UserID: fixture.Context.UserID,
			CaseID: fixture.Context.CaseID, CaseBindingHash: fixture.Context.CaseBindingHash,
			DatasetSnapshotID: fixture.Context.DatasetSnapshotID, SourceManifestHash: fixture.Context.SourceManifestHash,
			ContextEpoch: fixture.Context.ContextEpoch + 1, IssuedAt: issuedAt.Add(time.Second),
			PublicationPolicy: fixture.Context.PublicationPolicy, RiskAuthorityBinding: fixture.Context.RiskAuthorityBinding,
		})
		if err != nil {
			t.Fatal(err)
		}
		input.Context = drifted
		if err := Authorize(input); err == nil || err.Error() != "execution_grant_context_mismatch" {
			t.Fatalf("epoch/context binding drift was not rejected: %v", err)
		}
	})
}

func executionGrantTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.execution-grant-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func executionGrantTestSchemas(call domainmodel.ToolCall) []domainmodel.ToolSchema {
	return []domainmodel.ToolSchema{{
		Name:       call.Name,
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"q":{"type":"string"}},"additionalProperties":false}`),
	}}
}

func TestExecutionGrantDirectCallerCannotBypassClosedCatalogAndExactScope(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread-catalog", "turn-catalog", "/workspace/catalog", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-catalog"), Name: "read", Arguments: json.RawMessage(`{}`)}
	closed := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	tests := []struct {
		name    string
		call    domainmodel.ToolCall
		schemas []domainmodel.ToolSchema
		scope   []string
		code    string
	}{
		{
			name:    "open schema",
			call:    call,
			schemas: []domainmodel.ToolSchema{{Name: "read", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
			scope:   []string{"read"},
			code:    "execution_grant_schema_invalid",
		},
		{
			name:    "duplicate schema",
			call:    call,
			schemas: []domainmodel.ToolSchema{{Name: "read", Parameters: closed}, {Name: "read", Parameters: closed}},
			scope:   []string{"read"},
			code:    "execution_grant_schema_invalid",
		},
		{
			name:    "scope omits advertised tool",
			call:    call,
			schemas: []domainmodel.ToolSchema{{Name: "read", Parameters: closed}, {Name: "grep", Parameters: closed}},
			scope:   []string{"read"},
			code:    "execution_grant_scope_mismatch",
		},
		{
			name:    "scope adds unadvertised tool",
			call:    call,
			schemas: []domainmodel.ToolSchema{{Name: "read", Parameters: closed}},
			scope:   []string{"read", "write"},
			code:    "execution_grant_scope_mismatch",
		},
		{
			name:    "arguments include unknown field",
			call:    domainmodel.ToolCall{ID: call.ID, Name: "read", Arguments: json.RawMessage(`{"forgedAuthority":true}`)},
			schemas: []domainmodel.ToolSchema{{Name: "read", Parameters: closed}},
			scope:   []string{"read"},
			code:    "execution_grant_arguments_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := IssueProvider(securityContext, "provider", test.call, test.schemas, test.scope, true, "not_required", 0, "", now)
			if err == nil || err.Error() != test.code {
				t.Fatalf("direct grant caller bypass result=%v want=%s", err, test.code)
			}
		})
	}
}

func TestPendingCatalogRejectsSchemaThatBecameOpen(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 30, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread-pending-catalog", "turn-pending-catalog", "/workspace/catalog", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-pending-catalog"), Name: "read", Arguments: json.RawMessage(`{}`)}
	closedSchemas := []domainmodel.ToolSchema{{Name: "read", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}}
	grant, err := IssueProvider(securityContext, "provider", call, closedSchemas, []string{"read"}, true, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{Call: call, ToolScope: []string{"read"}, ExecutionGrant: grant}
	openSchemas := []domainmodel.ToolSchema{{Name: "read", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}
	if err := ValidatePendingCatalog(pending, openSchemas, true); err == nil || err.Error() != "execution_grant_schema_unavailable" {
		t.Fatalf("open resumed catalog was accepted: %v", err)
	}
}

func TestExecutionGrantRejectsUnknownExpiredAndMismatchedAuthority(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantCaseFixture(t, "thread_1", "turn_1", "/workspace/case-a", "case_a", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "mcp__analytix_funds__count_case_rows", Arguments: json.RawMessage(`{}`)}
	schemas := []domainmodel.ToolSchema{{
		Name:       call.Name,
		Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}
	verifiedIdentity := executionGrantTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 1)
	grant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{call.Name}, true, "not_required", 1, verifiedIdentity, now)
	if err != nil {
		t.Fatalf("issue execution grant: %v", err)
	}
	expected := Expectation{
		ContextDigest:   securityContext.ContextDigest,
		TurnID:          securityContext.TurnID,
		Provider:        "provider_1",
		ServerIdentity:  verifiedIdentity,
		ToolName:        call.Name,
		ToolCallID:      call.ID,
		ConnectionEpoch: 1,
		ArgsHash:        ArgumentsHash(call.Arguments),
		SchemaHash:      SchemaHash(call.Name, schemas),
		ScopeHash:       ScopeHash([]string{call.Name}),
		ApprovalState:   "not_required",
		ReadOnly:        true,
		Now:             now.Add(time.Minute),
	}
	if err := Validate(grant, expected); err != nil {
		t.Fatalf("valid grant rejected: %v", err)
	}

	tests := map[string]func(*Expectation){
		"wrong context":          func(value *Expectation) { value.ContextDigest = "context-b" },
		"wrong server":           func(value *Expectation) { value.ServerIdentity = "mcp:spoofed_funds" },
		"wrong connection epoch": func(value *Expectation) { value.ConnectionEpoch = 2 },
		"wrong schema":           func(value *Expectation) { value.SchemaHash = "schema-b" },
		"wrong call":             func(value *Expectation) { value.ToolCallID = "call_forged" },
		"future issued":          func(value *Expectation) { value.Now = now.Add(-10 * time.Second) },
		"expired":                func(value *Expectation) { value.Now = now.Add(DefaultTTL) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := expected
			mutate(&changed)
			if err := Validate(grant, changed); err == nil {
				t.Fatal("invalid grant must fail closed")
			}
		})
	}

	tampered := grant
	tampered.ReadOnly = false
	if err := Validate(tampered, expected); err == nil {
		t.Fatal("tampered grant integrity must fail closed")
	}
}

func TestIssueRejectsAmbiguousMCPNameAsBuiltin(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread-ambiguous", "turn-ambiguous", "/workspace", now).Context
	for _, name := range []string{"mcp__foo__bad name", "MCP__foo__lookup", " mcp__foo__lookup "} {
		call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-ambiguous"), Name: name, Arguments: json.RawMessage(`{}`)}
		schemas := executionGrantTestSchemas(call)
		_, err := IssueProvider(securityContext, "provider", call, schemas, []string{call.Name}, true, "not_required", 0, "", now)
		var typed ValidationError
		if !AsValidationError(err, &typed) || typed.Code != "execution_grant_tool_identity_invalid" {
			t.Fatalf("ambiguous MCP identity %q was not rejected before builtin fallback: %#v", name, err)
		}
		if identity := BoundServerIdentity(call.Name, securityContext, 0); identity != "" {
			t.Fatalf("ambiguous MCP identity %q became %q", name, identity)
		}
	}
}

func TestCurrentExecutionGrantRequiresHostToolCallIdentity(t *testing.T) {
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread-raw-id", "turn-raw-id", "/workspace", now).Context
	const raw = "provider_call_6222020202020202020"
	call := domainmodel.ToolCall{ID: raw, Name: "read", Arguments: json.RawMessage(`{}`)}
	schemas := executionGrantTestSchemas(call)
	if _, err := IssueProvider(securityContext, "provider", call, schemas, []string{call.Name}, true, "not_required", 0, "", now); err == nil || err.Error() != "execution_grant_identity_invalid" || strings.Contains(err.Error(), raw) {
		t.Fatalf("provider raw call identity was accepted or reflected: %v", err)
	}
	legacy := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: raw,
		ArgsHash: ArgumentsHash(call.Arguments), SchemaHash: SchemaHash(call.Name, schemas), ScopeHash: ScopeHash([]string{call.Name}),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	if err := Authorize(AuthorizationInput{Call: call, Grant: legacy}); err == nil || err.Error() != "execution_grant_identity_invalid" || strings.Contains(err.Error(), raw) {
		t.Fatalf("legacy raw call identity was authorized or reflected: %v", err)
	}
}

type executionGrantWorkspaceStub struct {
	binding  domainsecurity.CaseBinding
	realPath string
}

func (stub executionGrantWorkspaceStub) ReadOptional(string) (domainsecurity.CaseBinding, bool, error) {
	return stub.binding, stub.binding.CaseID != "", nil
}

func (stub executionGrantWorkspaceStub) WorkspaceRealPath(string) (string, error) {
	if stub.realPath != "" {
		return stub.realPath, nil
	}
	return "/workspace/case-a", nil
}

type executionGrantThreadStoreStub struct{ thread map[string]any }

func (stub executionGrantThreadStoreStub) GetThread(string) (map[string]any, error) {
	return stub.thread, nil
}

func activeExecutionGrantThread(context domainsecurity.TurnSecurityContext, workspace string, call domainmodel.ToolCall, grant domainsecurity.ExecutionGrant) map[string]any {
	record := turnsecurityapp.PublicRecord(context)
	issuedAt, _ := time.Parse(time.RFC3339Nano, context.IssuedAt)
	epochState, _ := contextepochapp.BootstrapState(
		context.ThreadID, context.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(context)}, issuedAt,
	)
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	arguments := map[string]any{}
	_ = json.Unmarshal(call.Arguments, &arguments)
	return map[string]any{
		"id":                context.ThreadID,
		"workspace":         workspace,
		"securityState":     record,
		"contextEpochState": contextepochapp.PublicState(epochState),
		"turns": []any{map[string]any{
			"id":              context.TurnID,
			"status":          "running",
			"securityContext": record,
			"items": []any{map[string]any{
				"id": "item_tool", "kind": "tool_call", "threadId": context.ThreadID, "turnId": context.TurnID,
				"toolName": call.Name, "callId": call.ID, "arguments": arguments, "createdAt": grant.IssuedAt,
				"contextDigest": context.ContextDigest, "contextEpoch": float64(context.ContextEpoch),
				"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
			}},
		}},
	}
}

type executionGrantSourceStub struct{}

func (executionGrantSourceStub) ServerDiagnostics() []any { return nil }
func (executionGrantSourceStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []toolcatalogapp.MCPToolAdvertisementV1 {
	return nil
}

type executionGrantMCPSourceStub struct {
	toolName     string
	identity     string
	epoch        uint64
	inputSchema  json.RawMessage
	outputSchema json.RawMessage
	readOnly     bool
}

func (stub executionGrantMCPSourceStub) ServerDiagnostics() []any { return nil }
func (stub executionGrantMCPSourceStub) LiveToolsForSecurityContext(domainsecurity.TurnSecurityContext) []string {
	return []string{stub.toolName}
}
func (stub executionGrantMCPSourceStub) ToolConnectionEpoch(toolName string) (uint64, bool) {
	return stub.epoch, toolName == stub.toolName && stub.epoch > 0
}
func (stub executionGrantMCPSourceStub) ToolServerIdentity(toolName string, _ domainsecurity.TurnSecurityContext) (string, bool) {
	return stub.identity, toolName == stub.toolName && stub.identity != ""
}
func (stub executionGrantMCPSourceStub) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	return stub.inputSchema, toolName == stub.toolName && len(stub.inputSchema) > 0
}
func (stub executionGrantMCPSourceStub) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	return stub.outputSchema, toolName == stub.toolName && len(stub.outputSchema) > 0
}
func (stub executionGrantMCPSourceStub) ToolDescription(string) (string, bool) { return "lookup", true }
func (stub executionGrantMCPSourceStub) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []toolcatalogapp.MCPToolAdvertisementV1 {
	return []toolcatalogapp.MCPToolAdvertisementV1{{
		Name: stub.toolName, Description: "lookup", InputSchema: stub.inputSchema, OutputSchema: stub.outputSchema,
		ReadOnly: stub.readOnly, ConnectionEpoch: stub.epoch, ServerIdentity: stub.identity,
	}}
}

func TestStaleApprovalGrantRejected(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread_1", "turn_1", "/workspace/case-a", "case_a", now)
	securityContext := fixture.Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_approval"), Name: "write", Arguments: json.RawMessage(`{"path":"report.md"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath, ProviderID: "provider_1", Call: call, ToolScope: []string{"write"}, SecurityContext: securityContext, ExecutionGrant: grant}
	store := executionGrantThreadStoreStub{thread: activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, grant)}
	registry, err := AuthorizePendingRegistry(context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "pending", now.Add(time.Minute))
	if err != nil || domainsecurity.VerifyExecutionGrantMembership(registry, securityContext.ThreadID, securityContext.TurnID, grant, domainsecurity.GrantRegistryPending) != nil {
		t.Fatalf("pending authorization did not return its exact durable registry snapshot: registry=%#v err=%v", registry, err)
	}
	if approved, err := AuthorizePendingApproval(context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute)); err != nil || approved.ApprovalState != "approved" {
		t.Fatalf("registered same-context approval must validate before transition: grant=%#v err=%v", approved, err)
	}
	advancedEpoch, err := contextepochapp.BootstrapState(
		securityContext.ThreadID, securityContext.ContextEpoch+1,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	currentEpochState := store.thread["contextEpochState"]
	store.thread["contextEpochState"] = contextepochapp.PublicState(advancedEpoch)
	if _, err := AuthorizePendingApproval(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute),
	); err == nil || err.Error() != "execution_grant_context_epoch_mismatch" {
		t.Fatalf("stale accepted context epoch resumed a pending approval: %v", err)
	}
	store.thread["contextEpochState"] = currentEpochState
	caseB := executionGrantCaseObservation(t, securityContext.WorkspaceRealPath, "case_b", "binding:case_b")
	fixture.Observer.set(caseB)
	if _, err := AuthorizePendingApproval(context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute)); err == nil {
		t.Fatal("approval must fail after the active case binding changes")
	}
	fixture.Observer.set(executionGrantCaseObservation(t, securityContext.WorkspaceRealPath, securityContext.CaseID, "binding:"+securityContext.CaseID))
	if _, err := AuthorizePendingApproval(context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(DefaultTTL)); err == nil {
		t.Fatal("expired pending grant must not resume after approval")
	}
	store.thread["workspace"] = "/workspace/case-b"
	movedReader := fixture.Reader
	movedReader.realPath = "/workspace/case-b"
	if _, err := AuthorizePendingApproval(context.Background(), fixture.Authority, movedReader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute)); err == nil {
		t.Fatal("approval must fail after the thread workspace changes")
	}
}

func TestAuthorizeUnregisteredPendingRequiresCurrentTurnWithoutPrematureRegistryMembership(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantGeneralFixture(t, "thread-admission", "turn-admission", "/workspace/general", now)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-admission"), Name: "read", Arguments: json.RawMessage(`{}`)}
	schemas := []domainmodel.ToolSchema{{
		Name: call.Name, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}
	grant, err := IssueProvider(fixture.Context, "provider-admission", call, schemas, []string{call.Name}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: fixture.Context.ThreadID, TurnID: fixture.Context.TurnID, Workspace: fixture.Context.WorkspaceRealPath,
		ProviderID: "provider-admission", Call: call, ToolScope: []string{call.Name}, SecurityContext: fixture.Context, ExecutionGrant: grant,
	}
	thread := activeExecutionGrantThread(fixture.Context, fixture.Context.WorkspaceRealPath, call, grant)
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["items"] = []any{}
	store := executionGrantThreadStoreStub{thread: thread}
	if err := AuthorizeUnregisteredPending(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "not_required", now.Add(time.Minute),
	); err != nil {
		t.Fatalf("current unregistered grant was rejected before ready persistence: %v", err)
	}
	if err := AuthorizePending(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "not_required", now.Add(time.Minute),
	); err == nil || err.Error() != "execution_grant_registry_membership_invalid" {
		t.Fatalf("unregistered grant became executable before ready persistence: %v", err)
	}

	stale := newExecutionGrantGeneralFixture(t, fixture.Context.ThreadID, "turn-new", fixture.Context.WorkspaceRealPath, now.Add(time.Minute)).Context
	thread["securityState"] = turnsecurityapp.PublicRecord(stale)
	if err := AuthorizeUnregisteredPending(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "not_required", now.Add(2*time.Minute),
	); err == nil || err.Error() != "execution_grant_context_mismatch" {
		t.Fatalf("durably superseded context admitted a ready grant: %v", err)
	}
}

func TestAuthorizeRequiresOperationContextAndObserverAuthority(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantGeneralFixture(t, "thread-authority", "turn-authority", "/workspace/general", now)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-authority"), Name: "calculate", Arguments: json.RawMessage(`{}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(fixture.Context, "provider", call, schemas, []string{call.Name}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	input := AuthorizationInput{
		OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
		Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: grant,
		Provider: "provider", Call: call, ToolScope: []string{call.Name}, Now: now.Add(time.Minute),
	}
	input.OperationContext = nil
	if err := Authorize(input); err == nil || err.Error() != "execution_grant_authority_unavailable" {
		t.Fatalf("nil operation context did not fail closed: %v", err)
	}
	input.OperationContext = context.Background()
	input.SecurityAuthority.Observer = nil
	if err := Authorize(input); err == nil || err.Error() != "execution_grant_authority_unavailable" {
		t.Fatalf("missing observer did not fail closed: %v", err)
	}
}

func TestAuthorizeRejectsStaleRiskWitness(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-risk-stale", "turn-risk-stale", "/workspace/case-risk", "case-risk", now)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-risk-stale"), Name: "calculate", Arguments: json.RawMessage(`{}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(fixture.Context, "provider", call, schemas, []string{call.Name}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	authorize := func() error {
		return Authorize(AuthorizationInput{
			OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
			Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: grant,
			Provider: "provider", Call: call, ToolScope: []string{call.Name}, Now: now.Add(time.Minute),
		})
	}
	if err := authorize(); err != nil {
		t.Fatalf("fresh witnessed risk authority was rejected: %v", err)
	}
	fixture.Risk.markStale()
	if err := authorize(); err == nil || err.Error() != "turn_security_risk_policy_mismatch" {
		t.Fatalf("stale witnessed risk authority did not fail closed: %v", err)
	}
}

func TestAuthorizeDatasetSnapshotMismatchBlocksOnlyCaseDataEffect(t *testing.T) {
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-snapshot", "turn-snapshot", "/workspace/case-snapshot", "case-snapshot", now)
	ordinaryCall := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-snapshot-ordinary"), Name: "calculate", Arguments: json.RawMessage(`{}`)}
	ordinarySchemas := executionGrantTestSchemas(ordinaryCall)
	ordinaryGrant, err := IssueProvider(fixture.Context, "provider", ordinaryCall, ordinarySchemas, []string{ordinaryCall.Name}, true, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	authorizeOrdinary := func() error {
		return Authorize(AuthorizationInput{
			OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
			Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: ordinaryGrant,
			Provider: "provider", Call: ordinaryCall, ToolScope: []string{ordinaryCall.Name}, Now: now.Add(time.Minute),
		})
	}
	if err := authorizeOrdinary(); err != nil {
		t.Fatalf("ordinary effect was rejected before snapshot drift: %v", err)
	}
	observation, err := fixture.Observer.Observe(fixture.Context.WorkspaceRealPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Snapshot.replace(t, observation, "replacement", now.Add(time.Second))
	if err := authorizeOrdinary(); err != nil {
		t.Fatalf("case snapshot drift disabled an unrelated ordinary effect: %v", err)
	}

	fundsName := "mcp__analytix_funds__count_case_rows"
	fundsCall := domainmodel.ToolCall{
		ID: executionGrantTestToolCallID("call-snapshot-funds"), Name: fundsName, Arguments: json.RawMessage(`{}`),
	}
	fundsSchemas := []domainmodel.ToolSchema{{
		Name: fundsName, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}
	fundsIdentity := executionGrantTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 4)
	fundsGrant, err := IssueProvider(
		fixture.Context, "provider", fundsCall, fundsSchemas, []string{fundsName}, true, "not_required", 4, fundsIdentity, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	err = Authorize(AuthorizationInput{
		OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
		Reader: fixture.Reader, Workspace: fixture.Context.WorkspaceRealPath, Context: fixture.Context, Grant: fundsGrant,
		Provider: "provider", Call: fundsCall, ToolScope: []string{fundsName}, Now: now.Add(time.Minute),
		LiveMCPTools: map[string]bool{fundsName: true}, MCPServerIdentities: map[string]string{fundsName: fundsIdentity},
	})
	if err == nil || err.Error() != "turn_security_dataset_snapshot_mismatch" {
		t.Fatalf("changed snapshot did not block the protected funds effect: %v", err)
	}
}

func TestApprovalResumeRevalidatesExactCurrentAuthority(t *testing.T) {
	now := time.Date(2026, 7, 12, 11, 30, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-resume", "turn-resume", "/workspace/case-resume", "case-resume", now)
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-resume"), Name: "write", Arguments: json.RawMessage(`{"path":"report.md"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(fixture.Context, "provider", call, schemas, []string{call.Name}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: fixture.Context.ThreadID, TurnID: fixture.Context.TurnID, Workspace: fixture.Context.WorkspaceRealPath,
		ProviderID: "provider", Call: call, ToolScope: []string{call.Name}, SecurityContext: fixture.Context, ExecutionGrant: grant,
	}
	store := executionGrantThreadStoreStub{thread: activeExecutionGrantThread(fixture.Context, fixture.Context.WorkspaceRealPath, call, grant)}
	approved, err := AuthorizePendingApproval(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute),
	)
	if err != nil || approved.ApprovalState != "approved" {
		t.Fatalf("exact current authority did not resume approval: grant=%#v err=%v", approved, err)
	}
	fixture.Risk.markStale()
	if _, err := AuthorizePendingApproval(
		context.Background(), fixture.Authority, fixture.Reader, store, executionGrantSourceStub{}, pending, "allow", now.Add(time.Minute),
	); err == nil || err.Error() != "turn_security_risk_policy_mismatch" {
		t.Fatalf("approval resume reused stale risk authority: %v", err)
	}
}

func TestRestartedManagerRejectsPersistedMCPGrantEvenWhenEpochVersionAndSchemaMatch(t *testing.T) {
	now := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-restart", "turn-restart", "/workspace/case-a", "case-a", now)
	securityContext := fixture.Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-restart"), Name: "mcp__docs__lookup", Arguments: json.RawMessage(`{"q":"x"}`)}
	inputSchema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`)
	outputSchema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	schemas := []domainmodel.ToolSchema{{Name: call.Name, Description: "lookup", Parameters: inputSchema, OutputSchema: outputSchema, Source: "mcp"}}
	beforeRestart, err := domainsecurity.NewVerifiedMCPServerIdentity("docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("runtime-before-restart")), 1)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := domainsecurity.NewVerifiedMCPServerIdentity("docs", "docs-server", "1.0.0", domainsecurity.SHA256Hex([]byte("runtime-after-restart")), 1)
	if err != nil {
		t.Fatal(err)
	}
	pendingGrant, err := IssueProvider(securityContext, "provider-1", call, schemas, []string{call.Name}, true, "pending", 1, beforeRestart, now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath, ProviderID: "provider-1",
		Call: call, ToolScope: []string{call.Name}, SecurityContext: securityContext, ExecutionGrant: pendingGrant,
	}
	store := executionGrantThreadStoreStub{thread: activeExecutionGrantThread(securityContext, securityContext.WorkspaceRealPath, call, pendingGrant)}
	source := executionGrantMCPSourceStub{
		toolName: call.Name, identity: afterRestart, epoch: 1, inputSchema: inputSchema, outputSchema: outputSchema, readOnly: true,
	}
	err = AuthorizePending(context.Background(), fixture.Authority, fixture.Reader, store, source, pending, "pending", now.Add(time.Minute))
	var validation ValidationError
	if !AsValidationError(err, &validation) || validation.Code != "execution_grant_server_mismatch" {
		t.Fatalf("same numeric epoch from a restarted manager accepted a persisted grant: %T %v", err, err)
	}
}

func TestSameNameSchemaSwapRejected(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-schema-swap", "turn-schema-swap", "/workspace/case-a", "case-a", now)
	toolName := "mcp__docs__lookup"
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-schema-swap"), Name: toolName, Arguments: json.RawMessage(`{"q":"x"}`)}
	originalInput := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`)
	changedInput := json.RawMessage(`{"type":"object","properties":{"q":{"type":"integer"}},"required":["q"],"additionalProperties":false}`)
	outputSchema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	advertisedSchemas := []domainmodel.ToolSchema{{
		Name: toolName, Description: "lookup", Parameters: originalInput, OutputSchema: outputSchema, Source: "mcp",
	}}
	identity := executionGrantTestMCPIdentity(t, "docs", "docs", "1.0.0", 7)
	grant, err := IssueProvider(fixture.Context, "provider-schema-swap", call, advertisedSchemas, []string{toolName}, true, "pending", 7, identity, now)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: fixture.Context.ThreadID, TurnID: fixture.Context.TurnID, Workspace: fixture.Context.WorkspaceRealPath,
		ProviderID: "provider-schema-swap", Call: call, ToolScope: []string{toolName}, SecurityContext: fixture.Context, ExecutionGrant: grant,
	}
	store := executionGrantThreadStoreStub{thread: activeExecutionGrantThread(fixture.Context, fixture.Context.WorkspaceRealPath, call, grant)}
	currentSource := executionGrantMCPSourceStub{
		toolName: toolName, identity: identity, epoch: 7, inputSchema: changedInput, outputSchema: outputSchema, readOnly: true,
	}
	err = AuthorizePending(context.Background(), fixture.Authority, fixture.Reader, store, currentSource, pending, "pending", now.Add(time.Minute))
	var validation ValidationError
	if !AsValidationError(err, &validation) || validation.Code != "execution_grant_schema_mismatch" {
		t.Fatalf("same-name schema replacement accepted a grant from the provider-request snapshot: %T %v", err, err)
	}
}

func executionGrantTestMCPIdentity(t *testing.T, serverID, observedName, observedVersion string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, domainsecurity.SHA256Hex([]byte("execution-grant-test-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestExecutionGrantRequiresKnownSchemaAndApprovalTransition(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread_1", "turn_1", "/workspace", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"x"}`)}
	if _, err := IssueProvider(securityContext, "provider_1", call, nil, nil, false, "pending", 0, "", now); err == nil {
		t.Fatal("unknown schema must not receive an execution grant")
	}
	schemas := executionGrantTestSchemas(call)
	pending, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := Approve(pending, now.Add(time.Minute))
	if err != nil || approved.ApprovalState != "approved" || approved.GrantID == pending.GrantID {
		t.Fatalf("approval must issue a distinct approved grant: %#v err=%v", approved, err)
	}
	if _, err := Approve(pending, now.Add(DefaultTTL)); err == nil {
		t.Fatal("expired pending grant must not be approved")
	}
}

func TestCaseContextKeepsOrdinaryWriteGrantUnderNormalApprovalPolicy(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantCaseFixture(t, "thread-write", "turn-write", "/workspace/case-a", "case_a", now)
	securityContext := fixture.Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call-write"), Name: "write_file", Arguments: json.RawMessage(`{"path":"src/result.txt","content":"ordinary workspace output"}`)}
	schemas := executionGrantTestSchemas(call)
	authorize := func(grant domainsecurity.ExecutionGrant, expectedApprovalState string) error {
		return Authorize(AuthorizationInput{
			OperationContext: context.Background(), SecurityAuthority: fixture.Authority,
			Reader: fixture.Reader, Workspace: securityContext.WorkspaceRealPath, Context: securityContext, Grant: grant,
			Provider: "provider_1", Call: call, ToolScope: []string{call.Name}, ExpectedApprovalState: expectedApprovalState,
			Now: now.Add(time.Minute),
		})
	}
	autoGrant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{call.Name}, false, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize(autoGrant, ""); err != nil {
		t.Fatalf("case context replaced an ordinary auto-approved workspace write: %v", err)
	}
	pendingGrant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{call.Name}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize(pendingGrant, "pending"); err != nil {
		t.Fatalf("pending grant must remain valid for a deny decision without executing: %v", err)
	}
	approvedGrant, err := Approve(pendingGrant, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize(approvedGrant, "approved"); err != nil {
		t.Fatalf("approved ordinary workspace write was mistaken for case publication: %v", err)
	}

	unboundFixture := newExecutionGrantGeneralFixture(t, "thread-unbound", "turn-unbound", "/workspace/unbound", now)
	unboundContext := unboundFixture.Context
	unboundGrant, err := IssueProvider(unboundContext, "provider_1", call, schemas, []string{call.Name}, false, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := Authorize(AuthorizationInput{
		OperationContext: context.Background(), SecurityAuthority: unboundFixture.Authority,
		Reader: unboundFixture.Reader, Workspace: unboundContext.WorkspaceRealPath, Context: unboundContext, Grant: unboundGrant,
		Provider: "provider_1", Call: call, ToolScope: []string{call.Name}, Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ordinary workspace write was overblocked: %v", err)
	}
}

func TestCaseReportGrantStillRequiresCaseAuthority(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	fixture := newExecutionGrantGeneralFixture(t, "thread-report-unbound", "turn-report-unbound", "/workspace/general", now)
	call := domainmodel.ToolCall{
		ID: executionGrantTestToolCallID("call-case-report"), Name: toolcatalogapp.ReportDeliveryToolName, Arguments: json.RawMessage(`{}`),
	}
	schemas := []domainmodel.ToolSchema{{
		Name: call.Name, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}
	if _, err := IssueProvider(
		fixture.Context, "provider", call, schemas, []string{call.Name}, false, "pending", 0, "", now,
	); err == nil || err.Error() != "execution_grant_case_authority_required" {
		t.Fatalf("case report received a grant without case authority: %v", err)
	}
}

func TestExecutionGrantIssueRejectsV1TurnSecurityContext(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	legacyContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: now,
	})
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"x"}`)}
	schemas := executionGrantTestSchemas(call)
	if legacyContext.Version != domainsecurity.TurnSecurityContextVersionV1 {
		t.Fatalf("test fixture unexpectedly stopped representing V1: %#v", legacyContext)
	}
	if _, err := IssueProvider(legacyContext, "provider_1", call, schemas, []string{"write"}, false, "not_required", 0, "", now); err == nil || err.Error() != "execution_grant_context_invalid" {
		t.Fatalf("audit-only V1 authority must not mint an execution grant: %v", err)
	}
}

func TestExecutionGrantRejectsMutatedArgumentsAndUnsafeSettlement(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	securityContext := newExecutionGrantGeneralFixture(t, "thread_1", "turn_1", "/workspace", now).Context
	call := domainmodel.ToolCall{ID: executionGrantTestToolCallID("call_1"), Name: "write", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	schemas := executionGrantTestSchemas(call)
	grant, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	mutated := call
	mutated.Arguments = json.RawMessage(`{"path":"b.txt"}`)
	if err := Validate(grant, Expectation{
		ContextDigest: securityContext.ContextDigest, TurnID: securityContext.TurnID, Provider: "provider_1",
		ServerIdentity: ServerIdentity(call.Name), ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: ArgumentsHash(mutated.Arguments), SchemaHash: SchemaHash(call.Name, schemas), ScopeHash: ScopeHash([]string{"write"}),
		ApprovalState: "not_required", ReadOnly: false, Now: now.Add(time.Minute),
	}); err == nil {
		t.Fatal("changing arguments after grant issuance must fail closed")
	}
	if output, isError := SettlementOutput(map[string]any{"ok": true}, false, grant, call.Name, "", nil); !isError || output.(map[string]any)["code"] != "execution_grant_rejected" {
		t.Fatalf("nil settlement authority must replace success with a fixed error: %#v error=%v", output, isError)
	}
	pending, err := IssueProvider(securityContext, "provider_1", call, schemas, []string{"write"}, false, "pending", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if output, isError := SettlementOutput(map[string]any{"ok": true}, false, pending, call.Name, "", func() error { return nil }); !isError || output.(map[string]any)["code"] != "execution_grant_approval_mismatch" {
		t.Fatalf("pending settlement must replace success with a fixed error: %#v error=%v", output, isError)
	}
	denied := map[string]any{"code": "approval_denied", "error": "tool execution denied by user"}
	if output, isError := SettlementOutput(denied, true, pending, call.Name, "pending", func() error { return nil }); !isError || output.(map[string]any)["code"] != "approval_denied" {
		t.Fatalf("host-fixed denial should survive pending-grant revalidation: %#v error=%v", output, isError)
	}
}

func TestExecutionGrantPublicFailureProjectionIsClosed(t *testing.T) {
	tests := []struct {
		internal string
		public   string
	}{
		{internal: "publication_receipt_required", public: "publication_receipt_required"},
		{internal: "execution_grant_source_unavailable", public: "execution_grant_source_unavailable"},
		{internal: "execution_grant_server_mismatch", public: "execution_grant_server_identity_invalid"},
		{internal: "execution_grant_connection_epoch_invalid", public: "execution_grant_connection_epoch_mismatch"},
		{internal: "execution_grant_schema_mismatch", public: "execution_grant_schema_unavailable"},
		{internal: "execution_grant_time_invalid", public: "execution_grant_expired"},
		{internal: "execution_grant_approval_pending", public: "execution_grant_approval_mismatch"},
		{internal: "execution_grant_workspace_mismatch", public: "execution_grant_context_invalid"},
		{internal: "PRIVATE tool=/private/case.csv account=6222021234567890 SELECT secret", public: "execution_grant_rejected"},
	}
	for _, test := range tests {
		t.Run(test.public+"/"+test.internal, func(t *testing.T) {
			details := ErrorDetails(ValidationError{Code: test.internal}, "PRIVATE_TOOL")
			if details["code"] != test.public || details["error"] != "tool execution authorization was rejected by current host authority" {
				t.Fatalf("unexpected public failure projection: %#v", details)
			}
			encoded, err := json.Marshal(details)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), "6222021234567890") || strings.Contains(string(encoded), "/private/") {
				t.Fatalf("public failure projection leaked authority detail: %s", encoded)
			}
		})
	}
}
