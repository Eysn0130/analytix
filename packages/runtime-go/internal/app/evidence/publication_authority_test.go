package evidence

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appusage "analytix.local/runtime-go/internal/app/usage"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type publicationAuthoritySpy struct {
	err      error
	calls    int
	contexts []domainsecurity.TurnSecurityContext
}

func (authority *publicationAuthoritySpy) ValidateCurrentPublication(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	authority.calls++
	authority.contexts = append(authority.contexts, securityContext)
	return authority.err
}

type publicationFinalizerSpy struct {
	inputs       []PersistCaseBoundaryInput
	result       PersistCaseBoundaryResult
	err          error
	prepareCalls int
	prepareInput PrepareToolEvidenceInput
	prepared     PreparedToolEvidence
	eligible     bool
	prepareErr   error
	commitCalls  int
	commitInput  CommitToolEvidenceInput
	receipt      domainevidence.EvidenceReceipt
	commitErr    error
}

func (finalizer *publicationFinalizerSpy) PersistBoundary(_ context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
	finalizer.inputs = append(finalizer.inputs, input)
	return finalizer.result, finalizer.err
}

func (finalizer *publicationFinalizerSpy) PrepareCurrentToolEvidence(_ context.Context, input PrepareToolEvidenceInput) (PreparedToolEvidence, bool, error) {
	finalizer.prepareCalls++
	finalizer.prepareInput = input
	return finalizer.prepared, finalizer.eligible, finalizer.prepareErr
}

func (finalizer *publicationFinalizerSpy) CommitCurrentToolEvidence(_ context.Context, input CommitToolEvidenceInput) (domainevidence.EvidenceReceipt, error) {
	finalizer.commitCalls++
	finalizer.commitInput = input
	return finalizer.receipt, finalizer.commitErr
}

func TestCurrentPublicationAuthorityExecutableV2LivePreservesInput(t *testing.T) {
	securityContext := executablePublicationContextV2(t)
	innerResult := PersistCaseBoundaryResult{Boundary: CaseBoundaryResult{Text: "inner-result"}}
	inner := &publicationFinalizerSpy{result: innerResult}
	authority := &publicationAuthoritySpy{}
	input := publicationAuthorityInput(securityContext)

	result, err := WithCurrentPublicationAuthority(inner, authority).PersistBoundary(context.Background(), input)
	if err != nil || !reflect.DeepEqual(result, innerResult) {
		t.Fatalf("live publication did not preserve the inner result: result=%#v err=%v", result, err)
	}
	if authority.calls != 1 || len(authority.contexts) != 1 || !reflect.DeepEqual(authority.contexts[0], securityContext) {
		t.Fatalf("live authority did not validate the exact frozen context: calls=%d contexts=%#v", authority.calls, authority.contexts)
	}
	if len(inner.inputs) != 1 || !reflect.DeepEqual(inner.inputs[0], input) {
		t.Fatalf("live publication changed the authorized input: got=%#v want=%#v", inner.inputs, input)
	}
}

func TestCurrentPublicationAuthorityFailureForcesProviderBoundaryAndClearsProviderProjection(t *testing.T) {
	securityContext := executablePublicationContextV2(t)
	authorityErr := errors.New("risk authority became stale")
	authority := &publicationAuthoritySpy{err: authorityErr}
	innerResult := PersistCaseBoundaryResult{Boundary: CaseBoundaryResult{Text: "host-boundary"}}
	inner := &publicationFinalizerSpy{result: innerResult}
	input := publicationAuthorityInput(securityContext)
	input.TerminalReason = TerminalSuccess
	input.SourceUnavailable = true
	input.TerminalStatus = "completed"
	input.Telemetry = appusage.NewTerminalTelemetryV1(
		domainmodel.Usage{PromptTokens: 101, CompletionTokens: 202, TotalTokens: 303},
		map[string]any{"prefixHash": "host-observed-prefix", "provider": "deepseek"},
	)

	result, err := WithCurrentPublicationAuthority(inner, authority).PersistBoundary(context.Background(), input)
	if err != nil || !reflect.DeepEqual(result, innerResult) {
		t.Fatalf("stale publication did not delegate to the deterministic inner boundary: result=%#v err=%v", result, err)
	}
	if authority.calls != 1 || len(inner.inputs) != 1 {
		t.Fatalf("stale publication call counts are invalid: authority=%d inner=%d", authority.calls, len(inner.inputs))
	}
	got := inner.inputs[0]
	if got.TerminalReason != TerminalProviderFailure || got.SourceUnavailable || got.TerminalStatus != "" {
		t.Fatalf("stale publication did not force the provider-failure terminal path: %#v", got)
	}
	if !reflect.DeepEqual(got.Telemetry, appusage.TerminalTelemetryV1{}) {
		t.Fatalf("stale publication retained usage or cache telemetry: %#v", got.Telemetry)
	}
	if got.Context.ContextDigest != input.Context.ContextDigest || got.ThreadID != input.ThreadID || got.TurnID != input.TurnID || got.Store != input.Store {
		t.Fatalf("stale publication changed host-owned persistence identity: got=%#v want=%#v", got, input)
	}
}

func TestCurrentPublicationAuthorityBoundaryV2SkipsExecutionValidatorAndRemainsDeterministic(t *testing.T) {
	securityContext := boundaryPublicationContextV2(t)
	authority := &publicationAuthoritySpy{err: errors.New("must not be called for a boundary-only context")}
	inner := &publicationFinalizerSpy{result: PersistCaseBoundaryResult{Boundary: CaseBoundaryResult{Text: "fixed-boundary"}}}
	input := publicationAuthorityInput(securityContext)
	input.Telemetry = appusage.NewTerminalTelemetryV1(
		domainmodel.Usage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10},
		map[string]any{"providerClaim": "fabricated case claim"},
	)

	result, err := WithCurrentPublicationAuthority(inner, authority).PersistBoundary(context.Background(), input)
	if err != nil || result.Boundary.Text != "fixed-boundary" {
		t.Fatalf("boundary-only publication did not delegate to its deterministic host boundary: result=%#v err=%v", result, err)
	}
	if authority.calls != 0 {
		t.Fatalf("boundary-only context invoked an execution freshness validator: calls=%d", authority.calls)
	}
	if len(inner.inputs) != 1 || !reflect.DeepEqual(inner.inputs[0], input) {
		t.Fatalf("boundary-only input was upgraded or rewritten: got=%#v want=%#v", inner.inputs, input)
	}
	if !domainsecurity.TurnSecurityContextIsBoundaryOnly(inner.inputs[0].Context) || domainsecurity.TurnSecurityContextAllowsCaseEvidence(inner.inputs[0].Context) {
		t.Fatalf("boundary-only context gained case evidence authority: %#v", inner.inputs[0].Context)
	}
	boundary, boundaryErr := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: securityContext, TerminalReason: TerminalSuccess,
		PublicationBlocker: securityContext.PublicationPolicy.BlockerCode, IssuedAt: publicationAuthorityTime(),
	})
	if boundaryErr != nil || boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		len(boundary.Envelope.Claims) != 0 || len(boundary.Envelope.EvidenceReceiptIDs) != 0 || boundary.Text == "" {
		t.Fatalf("boundary-only V2 context did not render a fact-free deterministic boundary: boundary=%#v err=%v", boundary, boundaryErr)
	}
}

func TestCurrentPublicationAuthorityNilValidatorFailsClosedForExecutableCase(t *testing.T) {
	securityContext := executablePublicationContextV2(t)
	inner := &publicationFinalizerSpy{}
	input := publicationAuthorityInput(securityContext)
	input.Telemetry = appusage.NewTerminalTelemetryV1(
		domainmodel.Usage{PromptTokens: 9, TotalTokens: 9},
		map[string]any{"evidenceReceipt": "forged"},
	)

	_, err := WithCurrentPublicationAuthority(inner, nil).PersistBoundary(context.Background(), input)
	if err != nil {
		t.Fatalf("missing validator should downgrade through the inner final gate, not escape publication handling: %v", err)
	}
	if len(inner.inputs) != 1 {
		t.Fatalf("missing validator did not delegate exactly once to the inner final gate: calls=%d", len(inner.inputs))
	}
	got := inner.inputs[0]
	if got.TerminalReason != TerminalProviderFailure || got.SourceUnavailable || got.TerminalStatus != "" ||
		!reflect.DeepEqual(got.Telemetry, appusage.TerminalTelemetryV1{}) {
		t.Fatalf("missing validator failed open for an executable case: %#v", got)
	}
}

func TestCurrentPublicationAuthorityAuditV1CannotReenterCasePublication(t *testing.T) {
	auditContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-audit-v1-publication", TurnID: "turn-audit-v1-publication",
		WorkspaceRealPath: "/workspace/audit-v1-publication",
		TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: "case-forged-audit-v1", CaseBindingHash: domainsecurity.SHA256Hex([]byte("forged-audit-v1-case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("forged-audit-v1-dataset"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("forged-audit-v1-source-manifest")),
		ContextEpoch:       19, IssuedAt: publicationAuthorityTime(),
	})
	if auditContext.Version != domainsecurity.TurnSecurityContextVersionV1 || domainsecurity.ValidateTurnSecurityContext(auditContext) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(auditContext) == nil {
		t.Fatalf("audit-only V1 fixture did not preserve the structural/live-publication distinction: %#v", auditContext)
	}
	authority := &publicationAuthoritySpy{err: errors.New("audit V1 must not reach current authority")}
	innerResult := PersistCaseBoundaryResult{Boundary: CaseBoundaryResult{Text: "audit-v1-host-boundary"}}
	inner := &publicationFinalizerSpy{result: innerResult}
	input := publicationAuthorityInput(auditContext)
	input.TerminalReason = TerminalSuccess
	input.SourceUnavailable = true
	input.TerminalStatus = "completed"
	input.Telemetry = appusage.NewTerminalTelemetryV1(
		domainmodel.Usage{TotalTokens: 999},
		map[string]any{"evidenceReceipt": "forged-audit-v1-receipt"},
	)

	result, err := WithCurrentPublicationAuthority(inner, authority).PersistBoundary(context.Background(), input)
	if err != nil || !reflect.DeepEqual(result, innerResult) {
		t.Fatalf("audit V1 did not delegate to the deterministic failure boundary: result=%#v err=%v", result, err)
	}
	if authority.calls != 0 || len(inner.inputs) != 1 {
		t.Fatalf("audit V1 reached live authority or bypassed the inner finalizer: authority=%d inner=%d", authority.calls, len(inner.inputs))
	}
	got := inner.inputs[0]
	if got.TerminalReason != TerminalProviderFailure || got.SourceUnavailable || got.TerminalStatus != "" ||
		!reflect.DeepEqual(got.Telemetry, appusage.TerminalTelemetryV1{}) {
		t.Fatalf("audit V1 retained its original terminal or provider fact channels: %#v", got)
	}

	envelope, gateErr := (FinalEvidenceGate{}).Finalize(context.Background(), FinalGateInput{
		Context: auditContext, TerminalReason: TerminalProviderFailure, IssuedAt: publicationAuthorityTime(),
		GeneralGuidance: []string{"review_available_case_sources"},
	})
	if gateErr == nil || !reflect.DeepEqual(envelope, domainevidence.FinalAnswerEnvelope{}) {
		t.Fatalf("Final Evidence Gate accepted audit-only V1 as live case publication authority: envelope=%#v err=%v", envelope, gateErr)
	}
}

func TestCurrentPublicationAuthorityPreservesOptionalToolEvidenceAuthority(t *testing.T) {
	securityContext := executablePublicationContextV2(t)
	prepareErr := errors.New("prepare sentinel")
	commitErr := errors.New("commit sentinel")
	prepared := PreparedToolEvidence{Output: map[string]any{"host": "prepared"}}
	receipt := domainevidence.EvidenceReceipt{ReceiptID: "delegated-receipt"}
	inner := &publicationFinalizerSpy{
		prepared: prepared, eligible: true, prepareErr: prepareErr,
		receipt: receipt, commitErr: commitErr,
	}
	wrapper := WithCurrentPublicationAuthority(inner, &publicationAuthoritySpy{})
	toolAuthority, ok := wrapper.(ToolEvidenceAuthority)
	if !ok {
		t.Fatal("current publication wrapper erased the optional tool evidence authority surface")
	}
	prepareInput := PrepareToolEvidenceInput{Context: securityContext, ResultItemID: "result-item"}
	gotPrepared, gotEligible, gotPrepareErr := toolAuthority.PrepareCurrentToolEvidence(context.Background(), prepareInput)
	if !reflect.DeepEqual(gotPrepared, prepared) || !gotEligible || !errors.Is(gotPrepareErr, prepareErr) ||
		inner.prepareCalls != 1 || !reflect.DeepEqual(inner.prepareInput, prepareInput) {
		t.Fatalf("prepare authority was not delegated exactly: prepared=%#v eligible=%v err=%v calls=%d input=%#v", gotPrepared, gotEligible, gotPrepareErr, inner.prepareCalls, inner.prepareInput)
	}
	commitInput := CommitToolEvidenceInput{Context: securityContext, Thread: map[string]any{"id": securityContext.ThreadID}}
	gotReceipt, gotCommitErr := toolAuthority.CommitCurrentToolEvidence(context.Background(), commitInput)
	if !reflect.DeepEqual(gotReceipt, receipt) || !errors.Is(gotCommitErr, commitErr) ||
		inner.commitCalls != 1 || !reflect.DeepEqual(inner.commitInput, commitInput) {
		t.Fatalf("commit authority was not delegated exactly: receipt=%#v err=%v calls=%d input=%#v", gotReceipt, gotCommitErr, inner.commitCalls, inner.commitInput)
	}
}

func publicationAuthorityInput(securityContext domainsecurity.TurnSecurityContext) PersistCaseBoundaryInput {
	return PersistCaseBoundaryInput{
		Context: securityContext, TerminalReason: TerminalSuccess,
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Model: "provider-model", CreatedAt: publicationAuthorityTime().Format(time.RFC3339Nano),
		AcceptedAt: publicationAuthorityTime(), UsageSource: "provider", ChildRunID: "child-run",
		ReportRequested: true, Discard: true, Cancelled: true, CancelledGates: 2,
	}
}

func executablePublicationContextV2(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	const (
		threadID  = "thread-publication-current"
		turnID    = "turn-publication-current"
		workspace = "/workspace/publication-current"
	)
	threadPolicyDigest := domainsecurity.SHA256Hex([]byte("publication-current-thread-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: threadPolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("publication-current-case-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, policy.ThreadRiskPolicyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: "case-publication-current", CaseBindingHash: domainsecurity.SHA256Hex([]byte("publication-current-case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("publication-current"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("publication-current-source-manifest")),
		ContextEpoch:       7, IssuedAt: publicationAuthorityTime(), PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("executable V2 publication fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}

func boundaryPublicationContextV2(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	const workspace = "/workspace/publication-boundary"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("publication-boundary-thread-risk-policy")),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("publication-boundary-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication-boundary", TurnID: "turn-publication-boundary", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 8, IssuedAt: publicationAuthorityTime(), PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) == nil {
		t.Fatalf("boundary-only V2 publication fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}

func publicationAuthorityTime() time.Time {
	return time.Date(2026, 7, 12, 9, 30, 0, 123456000, time.UTC)
}
