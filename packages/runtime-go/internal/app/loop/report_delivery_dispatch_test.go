package loop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type reportDeliveryHostStubV1 struct {
	calls      int
	projection domaintoolresult.PublicToolResultProjectionV1
	terminal   ReportTerminalCapability
	err        error
}

func (stub *reportDeliveryHostStubV1) ExecuteReportWithinHeldContextEffectV1(
	context.Context,
	appmodel.PendingToolCall,
) (
	domaintoolresult.PublicToolResultProjectionV1,
	ReportTerminalCapability,
	error,
) {
	stub.calls++
	return stub.projection, stub.terminal, stub.err
}

type reportTerminalCapabilityStubV1 struct {
	context domainsecurity.TurnSecurityContext
	grant   domainsecurity.ExecutionGrant
	callID  string
	digest  string
}

func (stub reportTerminalCapabilityStubV1) VerifyReportTerminal(
	context domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	callID string,
) (string, bool) {
	return stub.digest,
		context == stub.context && grant == stub.grant && callID == stub.callID
}

func TestReportDeliveryDispatchRequiresApprovalAndEndsOnlyWithPrivateCapability(t *testing.T) {
	pending := reportDeliveryPendingV1(t)
	capability := reportTerminalCapabilityStubV1{
		context: pending.SecurityContext,
		grant:   pending.ExecutionGrant,
		callID:  pending.Call.ID,
		digest:  domainsecurity.SHA256Hex([]byte("projected-report-outcome")),
	}
	host := &reportDeliveryHostStubV1{
		projection: domaintoolresult.WithheldProjectionV1(
			"completed", "tool_output_private",
		),
		terminal: capability,
	}
	output, isError := DispatchReportDeliveryWithinHeldContextEffectV1(
		context.Background(), host, pending, nil,
		func(appmodel.PendingToolCall) error { return nil },
	)
	if isError || host.calls != 1 {
		t.Fatalf("approved report dispatch did not reach the host exactly once: calls=%d isError=%v", host.calls, isError)
	}
	settled, handled, err := SettleReportDeliveryDispatchV1(pending, output)
	if err != nil || !handled || settled.IsError ||
		settled.ReportTerminal == nil ||
		settled.Output.(map[string]any)["projectionKind"] != "withheld" {
		t.Fatalf("report dispatch did not settle as metadata-only success: settled=%#v handled=%v err=%v", settled, handled, err)
	}
	terminal := EvaluateReportTerminalV1(
		pending.SecurityContext, pending.ExecutionGrant, pending.Call, settled,
	)
	if !terminal.Completed || terminal.Invalid || terminal.Boundary == "" {
		t.Fatalf("private terminal capability did not close the report loop: %#v", terminal)
	}

	withoutApproval := pending
	withoutApproval.ApprovalTransition = nil
	output, _ = DispatchReportDeliveryWithinHeldContextEffectV1(
		context.Background(), host, withoutApproval, nil,
		func(appmodel.PendingToolCall) error { return nil },
	)
	_, handled, err = SettleReportDeliveryDispatchV1(withoutApproval, output)
	var failure TurnFailureError
	if !handled || !errors.As(err, &failure) ||
		failure.Code != "report_publication_fallback" || host.calls != 1 {
		t.Fatalf("missing approval crossed report authority: calls=%d handled=%v err=%v", host.calls, handled, err)
	}
}

func reportDeliveryPendingV1(t *testing.T) appmodel.PendingToolCall {
	t.Helper()
	issuedAt := time.Now().UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(
		domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-report-dispatch", TurnID: "turn-report-dispatch",
			WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: issuedAt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{}`)
	call := domainmodel.ToolCall{
		ID:   loopTestHostToolCallID("report-dispatch"),
		Name: toolcatalogapp.ReportDeliveryToolName, Arguments: arguments,
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash:   domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("report-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("report-scope")),
		ReadOnly:   false, ApprovalState: "approved",
		IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Hour),
	})
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: securityContext.WorkspaceRealPath, Prompt: "请生成案件资金分析报告",
		ApprovalPolicy: "on-request", Call: call, SecurityContext: securityContext,
		ExecutionGrant:     grant,
		ApprovalTransition: &domainsecurity.ApprovalGrantTransitionV1{},
	}
}
