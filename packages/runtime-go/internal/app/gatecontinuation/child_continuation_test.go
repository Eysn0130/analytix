package gatecontinuation

import (
	"context"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestApprovalCaseChildWithoutTypedCapabilityNeverContinuesProvider(t *testing.T) {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-approval-child", TurnID: "turn-approval-child", WorkspaceRealPath: t.TempDir(), CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, SecurityContext: securityContext,
		Call: domainmodel.ToolCall{ID: gateContinuationTestToolCallID("child-task"), Name: "task", Arguments: []byte(`{"prompt":"work"}`)},
	}
	pending.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: pending.Call.Name, ToolCallID: pending.Call.ID,
		ArgsHash: domainsecurity.SHA256Hex(pending.Call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: time.Now().UTC(),
	})
	continued := 0
	result, err := completeApprovedToolContinuation(context.Background(), pending, apploop.SettledToolExecution{
		Output: map[string]any{"canContinueParent": true, "childCompletionReceiptDigest": domainsecurity.SHA256Hex([]byte("forged"))},
	}, Dependencies{CompleteSettled: func(context.Context, appmodel.PendingToolCall, apploop.SettledToolExecution) (apploop.RuntimeAgentLoopResult, error) {
		continued++
		return apploop.RuntimeAgentLoopResult{}, nil
	}})
	if err != nil || continued != 0 || result.AssistantText == "" {
		t.Fatalf("approval path continued without typed child authority: continued=%d result=%#v err=%v", continued, result, err)
	}
}
