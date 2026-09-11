package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type continuationCapabilityStub struct {
	parent  domainsecurity.TurnSecurityContext
	callID  string
	grantID string
	digest  string
	allow   bool
}

func (stub continuationCapabilityStub) VerifyParentContinuation(parent domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, callID string) (string, bool) {
	return stub.digest, stub.allow && parent == stub.parent && callID == stub.callID && (stub.grantID == "" || grant.GrantID == stub.grantID)
}

func TestCaseChildContinuationDefaultsBlockedWithoutTypedAuthority(t *testing.T) {
	context := childContinuationCaseContext(t)
	call := domainmodel.ToolCall{ID: loopTestHostToolCallID("child-continuation-default"), Name: "task", Arguments: []byte(`{"prompt":"work"}`)}
	grant := childContinuationGrant(context, call)
	for _, output := range []any{
		map[string]any{"status": "completed"},
		map[string]any{"outputWithheld": true, "canContinueParent": false},
		map[string]any{"outputWithheld": false, "canContinueParent": true, "childCompletionReceiptDigest": domainsecurity.SHA256Hex([]byte("forged"))},
	} {
		decision := EvaluateProviderContinuation(context, grant, call, SettledToolExecution{Output: output})
		if !decision.Blocked || decision.Boundary != childCompletionReceiptBoundary {
			t.Fatalf("public child output bypassed typed authority: output=%#v decision=%#v", output, decision)
		}
	}
	foreign := domainmodel.ToolCall{ID: "call-foreign", Name: "mcp__foreign__read", Arguments: []byte(`{}`)}
	if decision := EvaluateProviderContinuation(context, domainsecurity.ExecutionGrant{}, foreign, SettledToolExecution{Output: map[string]any{"outputWithheld": true}}); decision.Blocked {
		t.Fatalf("foreign output marker affected a non-child tool: %#v", decision)
	}
}

func TestCaseChildContinuationRequiresExactUniqueCapabilities(t *testing.T) {
	context := childContinuationCaseContext(t)
	taskCall := domainmodel.ToolCall{ID: loopTestHostToolCallID("child-continuation-task"), Name: "task", Arguments: []byte(`{"prompt":"work"}`)}
	taskGrant := childContinuationGrant(context, taskCall)
	valid := continuationCapabilityStub{parent: context, callID: taskCall.ID, grantID: taskGrant.GrantID, digest: domainsecurity.SHA256Hex([]byte("receipt-1")), allow: true}
	settled := SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{valid})}
	if decision := EvaluateProviderContinuation(context, taskGrant, taskCall, settled); decision.Blocked {
		t.Fatalf("exact typed capability was rejected: %#v", decision)
	}
	wrongGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: taskGrant.Provider, ServerIdentity: taskGrant.ServerIdentity, ToolName: taskGrant.ToolName, ToolCallID: taskGrant.ToolCallID,
		ArgsHash: taskGrant.ArgsHash, SchemaHash: taskGrant.SchemaHash, ScopeHash: domainsecurity.SHA256Hex([]byte("wrong-scope")),
		ReadOnly: taskGrant.ReadOnly, ApprovalState: taskGrant.ApprovalState, IssuedAt: time.Now().UTC(),
	})
	if decision := EvaluateProviderContinuation(context, wrongGrant, taskCall, settled); !decision.Blocked {
		t.Fatal("receipt for another execution grant authorized parent continuation")
	}
	argsDrift := taskCall
	argsDrift.Arguments = []byte(`{"prompt":"different work"}`)
	if decision := EvaluateProviderContinuation(context, taskGrant, argsDrift, settled); !decision.Blocked {
		t.Fatal("child call arguments drifted after grant issuance")
	}
	nameDrift := taskCall
	nameDrift.Name = "delegate_task"
	if decision := EvaluateProviderContinuation(context, taskGrant, nameDrift, settled); !decision.Blocked {
		t.Fatal("child call name drifted after grant issuance")
	}

	wrongCall := valid
	wrongCall.callID = "call-other"
	if decision := EvaluateProviderContinuation(context, taskGrant, taskCall, SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{wrongCall})}); !decision.Blocked {
		t.Fatal("wrong parent tool call capability was accepted")
	}
	notContinuable := valid
	notContinuable.allow = false
	if decision := EvaluateProviderContinuation(context, taskGrant, taskCall, SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{notContinuable})}); !decision.Blocked {
		t.Fatal("non-continuable child receipt was accepted")
	}
	if decision := EvaluateProviderContinuation(context, taskGrant, taskCall, SettledToolExecution{IsError: true, ParentContinuation: settled.ParentContinuation}); !decision.Blocked {
		t.Fatal("failed child execution authorized continuation")
	}

	parallelCall := domainmodel.ToolCall{ID: loopTestHostToolCallID("child-continuation-parallel"), Name: "parallel_tasks", Arguments: []byte(`{"tasks":[{"id":"a","prompt":"a"},{"id":"b","prompt":"b"}]}`)}
	parallelGrant := childContinuationGrant(context, parallelCall)
	first := continuationCapabilityStub{parent: context, callID: parallelCall.ID, grantID: parallelGrant.GrantID, digest: domainsecurity.SHA256Hex([]byte("parallel-1")), allow: true}
	second := continuationCapabilityStub{parent: context, callID: parallelCall.ID, grantID: parallelGrant.GrantID, digest: domainsecurity.SHA256Hex([]byte("parallel-2")), allow: true}
	if decision := EvaluateProviderContinuation(context, parallelGrant, parallelCall, SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(2, []ParentContinuationCapability{first, second})}); decision.Blocked {
		t.Fatalf("complete parallel capability set was rejected: %#v", decision)
	}
	if decision := EvaluateProviderContinuation(context, parallelGrant, parallelCall, SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(2, []ParentContinuationCapability{first})}); !decision.Blocked {
		t.Fatal("partial parallel capability set was accepted")
	}
	if decision := EvaluateProviderContinuation(context, parallelGrant, parallelCall, SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(2, []ParentContinuationCapability{first, first})}); !decision.Blocked {
		t.Fatal("duplicate parallel receipt was accepted")
	}
}

func TestWitnessedBoundaryAllowsOrdinaryTaskContinuationButNotProtectedCall(t *testing.T) {
	securityContext := childContinuationWitnessedBoundaryContext(t)
	taskCall := domainmodel.ToolCall{
		ID: loopTestHostToolCallID("boundary-child-task"), Name: "task", Arguments: []byte(`{"prompt":"ordinary work"}`),
	}
	taskGrant := childContinuationGrant(securityContext, taskCall)
	taskCapability := continuationCapabilityStub{
		parent: securityContext, callID: taskCall.ID, grantID: taskGrant.GrantID,
		digest: domainsecurity.SHA256Hex([]byte("boundary-task-receipt")), allow: true,
	}
	if decision := EvaluateProviderContinuation(
		securityContext,
		taskGrant,
		taskCall,
		SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{taskCapability})},
	); decision.Blocked {
		t.Fatalf("ordinary child continuation was blocked by unavailable case authority: %#v", decision)
	}

	fundsCall := domainmodel.ToolCall{
		ID: loopTestHostToolCallID("boundary-child-funds"), Name: "mcp__analytix_funds__analyze_account_flows", Arguments: []byte(`{}`),
	}
	fundsGrant := childContinuationGrant(securityContext, fundsCall)
	fundsCapability := continuationCapabilityStub{
		parent: securityContext, callID: fundsCall.ID, grantID: fundsGrant.GrantID,
		digest: domainsecurity.SHA256Hex([]byte("boundary-funds-receipt")), allow: true,
	}
	if decision := EvaluateProviderContinuation(
		securityContext,
		fundsGrant,
		fundsCall,
		SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{fundsCapability})},
	); !decision.Blocked {
		t.Fatal("protected funds continuation escaped boundary-only authority")
	}
}

func TestOrdinaryTaskContinuationRejectsQuarantinedAndAuditOnlyContexts(t *testing.T) {
	issuedAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-quarantine", TurnID: "turn-child-quarantine",
		WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-v1", TurnID: "turn-child-v1", WorkspaceRealPath: t.TempDir(),
		CaseID: "case-v1", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-v1")),
		DatasetSnapshotID: "snapshot-v1", SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-v1")),
		ContextEpoch: 1, IssuedAt: issuedAt,
	})
	for name, securityContext := range map[string]domainsecurity.TurnSecurityContext{
		"quarantined": quarantined,
		"audit-only":  legacy,
	} {
		t.Run(name, func(t *testing.T) {
			call := domainmodel.ToolCall{ID: loopTestHostToolCallID("child-" + name), Name: "task", Arguments: []byte(`{"prompt":"work"}`)}
			grant := childContinuationGrant(securityContext, call)
			capability := continuationCapabilityStub{
				parent: securityContext, callID: call.ID, grantID: grant.GrantID,
				digest: domainsecurity.SHA256Hex([]byte("child-" + name)), allow: true,
			}
			if decision := EvaluateProviderContinuation(
				securityContext,
				grant,
				call,
				SettledToolExecution{ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{capability})},
			); !decision.Blocked {
				t.Fatalf("%s context authorized parent continuation", name)
			}
		})
	}
}

func TestGeneralTurnDoesNotRequireCaseChildContinuationAuthority(t *testing.T) {
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general", TurnID: "turn-general", WorkspaceRealPath: t.TempDir(),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("general-manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: "call-task", Name: "task", Arguments: []byte(`{"prompt":"work"}`)}
	if decision := EvaluateProviderContinuation(context, domainsecurity.ExecutionGrant{}, call, SettledToolExecution{}); decision.Blocked {
		t.Fatalf("general turn inherited the case-only continuation gate: %#v", decision)
	}
}

func TestParentContinuationCapabilitySidecarNeverSerializes(t *testing.T) {
	context := childContinuationCaseContext(t)
	digest := domainsecurity.SHA256Hex([]byte("private-capability"))
	capability := continuationCapabilityStub{parent: context, callID: "call-task", digest: digest, allow: true}
	body, err := json.Marshal(SettledToolExecution{
		Output:             map[string]any{"status": "completed"},
		ParentContinuation: NewParentContinuationAuthority(1, []ParentContinuationCapability{capability}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), digest) || strings.Contains(string(body), "ParentContinuation") {
		t.Fatalf("in-process continuation authority serialized: %s", body)
	}
}

func TestRunToolStepUsesTypedCaseChildContinuationDecision(t *testing.T) {
	securityContext := newLoopCaseContextV2(t, "thread-step-child", "turn-step-child", t.TempDir(), "case-a")
	call := domainmodel.ToolCall{ID: loopTestHostToolCallID("call-task"), Name: "task", Arguments: []byte(`{}`)}
	base := ToolStepInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider-a", Workspace: securityContext.WorkspaceRealPath,
		ApprovalPolicy: "auto", SandboxMode: "danger-full-access", EffectiveMaxModelSteps: 2,
		ToolCalls: []domainmodel.ToolCall{call}, ToolSchemas: zeroArgumentToolSchemas("task"), AdvertisedTools: advertisedToolNames("task"),
		SecurityContext: securityContext,
	}
	blockedDriver := &toolStepDriverStub{}
	base.Driver = blockedDriver
	blocked, err := RunToolStep(context.Background(), base)
	if err != nil || !blocked.ProviderContinuationBlocked || len(blockedDriver.executed) != 1 {
		t.Fatalf("case child without typed authority was not stopped after settlement: result=%#v err=%v", blocked, err)
	}
	capability := continuationCapabilityStub{
		parent: securityContext, callID: call.ID, digest: domainsecurity.SHA256Hex([]byte("step-receipt")), allow: true,
	}
	allowedDriver := &toolStepDriverStub{continuations: map[string]ParentContinuationAuthority{
		"task": NewParentContinuationAuthority(1, []ParentContinuationCapability{capability}),
	}}
	base.Driver = allowedDriver
	allowed, err := RunToolStep(context.Background(), base)
	if err != nil || allowed.ProviderContinuationBlocked || len(allowedDriver.executed) != 1 {
		t.Fatalf("exact typed child authority did not reach the normal loop boundary: result=%#v err=%v", allowed, err)
	}
}

func childContinuationCaseContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	context, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case", TurnID: "turn-case", WorkspaceRealPath: t.TempDir(), CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("case-manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func childContinuationWitnessedBoundaryContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	issuedAt := time.Date(2026, 7, 27, 11, 45, 0, 0, time.UTC)
	workspace := t.TempDir()
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-boundary", TurnID: "turn-child-boundary",
		WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		quarantined.ThreadID,
		quarantined.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		quarantined.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: quarantined.ThreadID, TurnID: quarantined.TurnID, WorkspaceRealPath: quarantined.WorkspaceRealPath,
		TenantID: quarantined.TenantID, UserID: quarantined.UserID, CaseID: quarantined.CaseID,
		CaseBindingHash: quarantined.CaseBindingHash, DatasetSnapshotID: quarantined.DatasetSnapshotID,
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func childContinuationGrant(context domainsecurity.TurnSecurityContext, call domainmodel.ToolCall) domainsecurity.ExecutionGrant {
	return domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.SHA256Hex(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
}
