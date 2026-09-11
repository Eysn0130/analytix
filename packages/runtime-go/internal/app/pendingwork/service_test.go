package pendingwork

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func pendingWorkTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.pending-work-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func TestToolBatchIssueVerifyAndCompleteRequiresEveryMemberSettled(t *testing.T) {
	fixture := newServiceFixture(t)
	first := fixture.addGrant(t, "read_a", true, "not_required", fixture.now)
	second := fixture.addGrant(t, "read_b", true, "not_required", fixture.now.Add(time.Second))
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch,
		[]GrantReference{{GrantID: second.GrantID}, {GrantID: first.GrantID}},
		"batch-a",
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.GrantMembers) != 2 || receipt.GrantMembers[0].GrantID != first.GrantID || receipt.GrantMembers[1].GrantID != second.GrantID {
		t.Fatalf("tool batch members were not canonically ordered: %#v", receipt.GrantMembers)
	}
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(2*time.Second)); err != nil {
		t.Fatalf("active read-only batch did not verify before execution: %v", err)
	}
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("partially unsettled tool batch was completed")
	}
	fixture.addResult(first, false, "first-result", fixture.now.Add(3*time.Second))
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second)); err == nil {
		t.Fatal("tool batch with one unsettled member was completed")
	}
	fixture.addResult(second, true, "second-failed-result", fixture.now.Add(4*time.Second))
	disposition, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("fully settled batch was not closable: %v", err)
	}
	if disposition.Status != domainpendingwork.StatusCompleted {
		t.Fatalf("unexpected batch disposition: %#v", disposition)
	}
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(6*time.Second)); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("completed receipt remained open: %v", err)
	}
}

func TestIssueAndCompleteToolBatchHighLevelAPIBindsExactCalls(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	pending := fixture.pendingCall(grant)
	receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != domainpendingwork.KindToolBatch || receipt.RouteHash != domainsecurity.SHA256Hex([]byte(toolBatchRouteIdentity)) {
		t.Fatalf("high-level batch identity is incomplete: %#v", receipt)
	}
	changed := pending
	changed.Call.Arguments = json.RawMessage(`{"member":"another"}`)
	if _, err := fixture.service.CompleteToolBatch(context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{changed}, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("different call content matched the issued tool batch")
	}
	fixture.addResult(grant, false, "done", fixture.now.Add(3*time.Second))
	if _, err := fixture.service.CompleteToolBatch(context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second)); err != nil {
		t.Fatalf("exact high-level batch did not complete: %v", err)
	}
}

func TestAuditOnlyV1PendingWorkRejectsBeforeLeaseSignerStoreOrEffect(t *testing.T) {
	legacyContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pending", TurnID: "turn-pending", WorkspaceRealPath: "/workspace/case-a",
		CaseID: "case-private", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-a-binding")), DatasetSnapshotID: "snapshot-v1",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-a")), ContextEpoch: 7, IssuedAt: time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC),
	})

	t.Run("direct issue", func(t *testing.T) {
		fixture := newServiceFixture(t)
		input := fixture.issueInput(domainpendingwork.KindToolBatch, nil, "legacy")
		input.SecurityContext = legacyContext
		if _, err := fixture.service.Issue(context.Background(), input); err == nil {
			t.Fatal("audit-only V1 context issued pending work")
		}
		assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	})

	t.Run("tool batch", func(t *testing.T) {
		fixture := newServiceFixture(t)
		pending := legacyPendingToolCall(fixture.pendingCall(fixture.addGrant(t, "read", true, "not_required", fixture.now)), legacyContext)
		gate := &countingPendingWorkGate{}
		executeCount := 0
		if _, err := fixture.service.WithOpenToolBatch(context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			executeCount++
			return nil
		}); err == nil {
			t.Fatal("audit-only V1 tool batch reached its effect boundary")
		}
		if gate.effectAcquires != 0 || gate.transitionAcquires != 0 || executeCount != 0 {
			t.Fatalf("V1 tool batch crossed a host lease/effect boundary: gate=%#v execute=%d", gate, executeCount)
		}
		assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	})

	t.Run("provider continuation", func(t *testing.T) {
		fixture := newServiceFixture(t)
		input := ProviderContinuationIssueInput{
			SecurityContext: legacyContext, CanonicalPayload: []byte(`{"schemaVersion":1}`),
			RouteHash: domainsecurity.SHA256Hex([]byte("legacy-route")), IssuedAt: fixture.now, ExpiresAt: fixture.now.Add(time.Minute),
		}
		if _, err := fixture.service.IssueProviderContinuation(context.Background(), input); err == nil {
			t.Fatal("audit-only V1 provider continuation reached its signer")
		}
		if _, err := fixture.service.BeginProviderContinuation(context.Background(), ProviderContinuationRequest{SecurityContext: legacyContext}); err == nil {
			t.Fatal("audit-only V1 provider continuation reached high-level issuance")
		}
		assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	})

	t.Run("report stage", func(t *testing.T) {
		fixture := newServiceFixture(t)
		pending := legacyPendingToolCall(fixture.pendingCall(fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)), legacyContext)
		if _, err := fixture.service.BeginReportStage(context.Background(), ReportStageRequest{
			PendingToolCall: pending, StageInputHash: domainsecurity.SHA256Hex([]byte("stage-input")), IssuedAt: fixture.now,
		}); err == nil {
			t.Fatal("audit-only V1 report stage reached its signer")
		}
		assertNoPendingWorkAuthorityEffect(t, fixture, 0)
	})
}

func TestWitnessedBoundaryOrdinaryToolBatchRunsAndCompletes(t *testing.T) {
	fixture := newServiceFixture(t)
	boundary := mustPendingWorkWitnessedBoundaryContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		ContextEpoch:      fixture.securityContext.ContextEpoch, IssuedAt: fixture.now,
	})
	fixture.installContext(boundary)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	pending := fixture.pendingCall(grant)
	gate := effectgateapp.New()
	executed := 0
	receipt, err := fixture.service.WithOpenToolBatch(
		context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			executed++
			return nil
		},
	)
	if err != nil || executed != 1 || receipt.WorkID == "" {
		t.Fatalf("ordinary boundary batch did not execute: receipt=%#v executed=%d err=%v", receipt, executed, err)
	}
	fixture.addResult(grant, false, "done", fixture.now.Add(time.Second))
	if _, err := fixture.service.CompleteToolBatchWithGate(
		context.Background(), gate, receipt.WorkID, boundary, []appmodel.PendingToolCall{pending},
		domainpendingwork.StatusCompleted, "batch_completed",
	); err != nil {
		t.Fatalf("ordinary boundary batch did not complete: %v", err)
	}
}

func TestWitnessedBoundaryToolBatchRejectsProtectedAndMixedEffectsBeforeLease(t *testing.T) {
	issuedAt := time.Date(2026, 7, 27, 12, 30, 0, 0, time.UTC)
	boundary := mustPendingWorkWitnessedBoundaryContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pending-boundary", TurnID: "turn-pending-boundary",
		WorkspaceRealPath: "/workspace/pending-boundary", ContextEpoch: 1, IssuedAt: issuedAt,
	})
	protected := pendingWorkCallForContext(t, boundary, "stage_case_report", true, "not_required", issuedAt)
	gate := &countingPendingWorkGate{}
	if _, err := (&Service{}).WithOpenToolBatch(
		context.Background(), gate, []appmodel.PendingToolCall{protected}, func(context.Context) error { return nil },
	); !errors.Is(err, ErrGrantAuthority) || gate.effectAcquires != 0 || gate.ordinaryEffectAcquires != 0 {
		t.Fatalf("protected boundary batch reached a lease: gate=%#v err=%v", gate, err)
	}

	fixture := newServiceFixture(t)
	ordinary := fixture.pendingCall(fixture.addGrant(t, "read", true, "not_required", fixture.now))
	protected = fixture.pendingCall(fixture.addGrant(t, "stage_case_report", true, "not_required", fixture.now.Add(time.Second)))
	for _, calls := range [][]appmodel.PendingToolCall{{ordinary, protected}, {protected, ordinary}} {
		gate = &countingPendingWorkGate{}
		if _, err := fixture.service.WithOpenToolBatch(
			context.Background(), gate, calls, func(context.Context) error { return nil },
		); !errors.Is(err, ErrGrantAuthority) || gate.effectAcquires != 0 || gate.ordinaryEffectAcquires != 0 {
			t.Fatalf("mixed-effect batch reached a lease: gate=%#v err=%v", gate, err)
		}
	}
}

func legacyPendingToolCall(pending appmodel.PendingToolCall, legacy domainsecurity.TurnSecurityContext) appmodel.PendingToolCall {
	pending.ThreadID = legacy.ThreadID
	pending.TurnID = legacy.TurnID
	pending.SecurityContext = legacy
	pending.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: legacy, Provider: pending.ExecutionGrant.Provider, ServerIdentity: pending.ExecutionGrant.ServerIdentity,
		ToolName: pending.Call.Name, ToolCallID: pending.Call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(pending.Call.Arguments),
		SchemaHash: pending.ExecutionGrant.SchemaHash, ScopeHash: pending.ExecutionGrant.ScopeHash,
		ReadOnly: pending.ExecutionGrant.ReadOnly, ApprovalState: pending.ExecutionGrant.ApprovalState,
		IssuedAt: time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 7, 12, 4, 20, 0, 0, time.UTC),
	})
	return pending
}

type countingPendingWorkGate struct {
	effectAcquires         int
	ordinaryEffectAcquires int
	transitionAcquires     int
}

func (gate *countingPendingWorkGate) AcquireEffect(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	gate.effectAcquires++
	return ctx, func() {}, nil
}

func (gate *countingPendingWorkGate) AcquireOrdinaryEffect(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	gate.ordinaryEffectAcquires++
	return ctx, func() {}, nil
}

func (gate *countingPendingWorkGate) AcquireTransition(context.Context, domainsecurity.TurnSecurityContext) (func(), error) {
	gate.transitionAcquires++
	return func() {}, nil
}

func assertNoPendingWorkAuthorityEffect(t *testing.T, fixture *serviceFixture, wantSigns int) {
	t.Helper()
	if signs := fixture.authority.SignCount(); signs != wantSigns {
		t.Fatalf("unexpected pending-work signer calls: got=%d want=%d", signs, wantSigns)
	}
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	if len(fixture.store.receipts) != 0 || len(fixture.store.dispositions) != 0 {
		t.Fatalf("rejected V1 pending work mutated durable state: receipts=%d dispositions=%d", len(fixture.store.receipts), len(fixture.store.dispositions))
	}
}

func TestVerifyToolBatchAtStartRejectsClosedAndStaleReceipt(t *testing.T) {
	t.Run("closed receipt", func(t *testing.T) {
		fixture := newServiceFixture(t)
		grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
		pending := fixture.pendingCall(grant)
		receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		fixture.addResult(grant, false, "done", fixture.now.Add(3*time.Second))
		if _, err := fixture.service.CompleteToolBatch(context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.service.VerifyToolBatchAtStart(context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, fixture.now.Add(5*time.Second)); !errors.Is(err, ErrWorkClosed) {
			t.Fatalf("closed batch receipt reached its effect boundary: %v", err)
		}
	})

	t.Run("stale context receipt", func(t *testing.T) {
		fixture := newServiceFixture(t)
		grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
		pending := fixture.pendingCall(grant)
		receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		newContext := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: fixture.securityContext.ThreadID, TurnID: "turn-new", WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
			CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-b-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-b")), ContextEpoch: fixture.securityContext.ContextEpoch + 1,
			IssuedAt: fixture.now.Add(time.Minute),
		})
		fixture.setCurrentContext(newContext)
		if _, err := fixture.service.VerifyToolBatchAtStart(context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Minute)); !errors.Is(err, ErrCurrentContext) {
			t.Fatalf("stale batch receipt reached its effect boundary: %v", err)
		}
	})
}

func TestToolBatchEffectLeaseSerializesExecutionAndDisposition(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	pending := fixture.pendingCall(grant)
	gate := effectgateapp.New()
	issued, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	releaseExecutor := make(chan struct{})
	type executionResult struct {
		receipt domainpendingwork.PendingWorkReceiptV1
		err     error
	}
	executed := make(chan executionResult, 1)
	fixture.setBoundaryTime(fixture.now.Add(time.Second))
	go func() {
		receipt, err := fixture.service.WithOpenToolBatch(context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			fixture.addResult(grant, false, "done", fixture.now.Add(3*time.Second))
			close(started)
			<-releaseExecutor
			return nil
		})
		executed <- executionResult{receipt: receipt, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("batch effect did not acquire its lease")
	}
	closed := make(chan error, 1)
	fixture.setBoundaryTime(fixture.now.Add(4 * time.Second))
	go func() {
		_, err := fixture.service.CompleteToolBatchWithGate(context.Background(), gate, issued.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, domainpendingwork.StatusCompleted, "batch_completed")
		closed <- err
	}()
	select {
	case err := <-closed:
		t.Fatalf("batch disposition crossed the in-flight effect: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseExecutor)
	select {
	case result := <-executed:
		if result.err != nil || result.receipt.WorkID != issued.WorkID {
			t.Fatalf("batch effect failed after release: receipt=%#v err=%v", result.receipt, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("batch effect did not return after release")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("batch disposition failed after effect release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("batch disposition did not resume after effect release")
	}
}

func TestToolBatchClosedOrStaleWinnerHasZeroEffect(t *testing.T) {
	t.Run("closed wins", func(t *testing.T) {
		fixture := newServiceFixture(t)
		grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
		pending := fixture.pendingCall(grant)
		gate := effectgateapp.New()
		receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		fixture.setBoundaryTime(fixture.now.Add(3 * time.Second))
		if _, err := fixture.service.CompleteToolBatchWithGate(context.Background(), gate, receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending}, domainpendingwork.StatusFailed, "batch_execution_failed"); err != nil {
			t.Fatal(err)
		}
		executeCount := 0
		fixture.setBoundaryTime(fixture.now.Add(4 * time.Second))
		if _, err := fixture.service.WithOpenToolBatch(context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			executeCount++
			return nil
		}); !errors.Is(err, ErrWorkClosed) {
			t.Fatalf("closed winner did not reject effect start: %v", err)
		}
		if executeCount != 0 {
			t.Fatal("closed batch still executed")
		}
	})

	t.Run("context transition wins", func(t *testing.T) {
		fixture := newServiceFixture(t)
		grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
		pending := fixture.pendingCall(grant)
		gate := effectgateapp.New()
		newContext := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: fixture.securityContext.ThreadID, TurnID: "turn-new", WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
			CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-b")), ContextEpoch: fixture.securityContext.ContextEpoch + 1, IssuedAt: fixture.now.Add(time.Minute),
		})
		release, err := gate.AcquireTransition(context.Background(), newContext)
		if err != nil {
			t.Fatal(err)
		}
		fixture.setCurrentContext(newContext)
		release()
		executeCount := 0
		fixture.setBoundaryTime(fixture.now.Add(2 * time.Minute))
		if _, err := fixture.service.WithOpenToolBatch(context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			executeCount++
			return nil
		}); !errors.Is(err, ErrCurrentContext) {
			t.Fatalf("stale context winner did not reject effect start: %v", err)
		}
		if executeCount != 0 {
			t.Fatal("stale-context batch still executed")
		}
	})
}

func TestToolBatchSamplesExpiryAfterEffectLeaseAcquisition(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	pending := fixture.pendingCall(grant)
	gate := newPendingWorkBoundaryGate(true, false)
	fixture.setBoundaryTime(fixture.now.Add(time.Second))
	executeCount := 0
	result := make(chan error, 1)
	go func() {
		_, err := fixture.service.WithOpenToolBatch(context.Background(), gate, []appmodel.PendingToolCall{pending}, func(context.Context) error {
			executeCount++
			return nil
		})
		result <- err
	}()
	<-gate.effectEntered
	fixture.setBoundaryTime(fixture.now.Add(21 * time.Minute))
	close(gate.effectRelease)
	select {
	case err := <-result:
		if !errors.Is(err, ErrGrantAuthority) {
			t.Fatalf("batch start used its pre-wait timestamp after grant expiry: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expired batch start did not return")
	}
	if executeCount != 0 {
		t.Fatal("expired batch executed after waiting for its effect lease")
	}
}

func TestToolBatchSamplesExpiryAfterDispositionLeaseAcquisition(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	pending := fixture.pendingCall(grant)
	receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "done", fixture.now.Add(3*time.Second))
	gate := newPendingWorkBoundaryGate(false, true)
	fixture.setBoundaryTime(fixture.now.Add(4 * time.Second))
	result := make(chan error, 1)
	go func() {
		_, err := fixture.service.CompleteToolBatchWithGate(
			context.Background(), gate, receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending},
			domainpendingwork.StatusCompleted, "batch_completed",
		)
		result <- err
	}()
	<-gate.transitionEntered
	fixture.setBoundaryTime(fixture.now.Add(21 * time.Minute))
	close(gate.transitionRelease)
	select {
	case err := <-result:
		if !errors.Is(err, ErrWorkExpired) {
			t.Fatalf("batch disposition used its pre-wait timestamp after receipt expiry: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expired batch disposition did not return")
	}
	if _, err := fixture.store.ReadDisposition(context.Background(), receipt.WorkID); !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		t.Fatalf("expired batch disposition was persisted: %v", err)
	}
}

func TestVerifyOpenForRejectsWrongOperationIdentity(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "identity",
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []struct{ kind, payloadHash, routeHash string }{
		{domainpendingwork.KindReportStage, receipt.PayloadHash, receipt.RouteHash},
		{receipt.Kind, domainsecurity.SHA256Hex([]byte("wrong-payload")), receipt.RouteHash},
		{receipt.Kind, receipt.PayloadHash, domainsecurity.SHA256Hex([]byte("wrong-route"))},
	} {
		if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, mismatch.kind, mismatch.payloadHash, mismatch.routeHash, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
			t.Fatalf("wrong expected operation was not rejected: mismatch=%#v err=%v", mismatch, err)
		}
	}
}

func TestCompletedBatchReplaysOriginalActiveMemberDigest(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "active-digest",
	))
	if err != nil {
		t.Fatal(err)
	}
	turn := fixture.threads.threads[fixture.securityContext.ThreadID]["turns"].([]any)[0].(map[string]any)
	callItem := turn["items"].([]any)[0].(map[string]any)
	callItem["createdAt"] = fixture.now.Add(time.Second).Format(time.RFC3339Nano)
	fixture.addResult(grant, false, "done", fixture.now.Add(3*time.Second))
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second)); err == nil {
		t.Fatal("mutated active registry entry digest was not detected at completion")
	}
}

func TestToolBatchRequiresActiveReadOnlyGrantAndAllowsRegistryAppend(t *testing.T) {
	fixture := newServiceFixture(t)
	readGrant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: readGrant.GrantID}}, "prefix",
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.addGrant(t, "later_read", true, "not_required", fixture.now.Add(time.Second))
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(2*time.Second)); err != nil {
		t.Fatalf("unrelated append invalidated the sealed issue prefix: %v", err)
	}
	writable := fixture.addGrant(t, "write", false, "approved", fixture.now.Add(2*time.Second))
	if _, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: writable.GrantID}}, "writable-batch",
	)); err == nil {
		t.Fatal("writable grant entered a tool batch")
	}
	fixture.addResult(readGrant, false, "done", fixture.now.Add(3*time.Second))
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(4*time.Second)); err == nil {
		t.Fatal("settled grant remained executable through its active batch receipt")
	}
}

func TestProviderContinuationBindsExactDurableFailedResult(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	result := fixture.addResult(grant, true, "tool failed safely", fixture.now.Add(time.Second))
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindProviderContinuation,
		[]GrantReference{{GrantID: grant.GrantID, ResultItemID: mapString(result, "id")}},
		"continuation",
	))
	if err != nil {
		t.Fatalf("durable failed tool result was not eligible as continuation data: %v", err)
	}
	if len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].ResultItemDigest == "" {
		t.Fatalf("provider continuation omitted durable result authority: %#v", receipt.GrantMembers)
	}
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(2*time.Second)); err != nil {
		t.Fatalf("settled provider continuation did not verify: %v", err)
	}
	result["output"] = "tampered after issuance"
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("changed durable result bytes retained continuation authority")
	}
}

func TestProviderContinuationHighLevelAPIBindsExpectedPayloadRouteAndReferences(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	result := fixture.addResult(grant, true, "failed result for provider", fixture.now.Add(time.Second))
	input := ProviderContinuationIssueInput{
		SecurityContext:  fixture.securityContext,
		GrantReferences:  []GrantReference{{GrantID: grant.GrantID, ResultItemID: mapString(result, "id")}},
		CanonicalPayload: []byte(`{"messages":[{"content":"ACCOUNT_6222020202020202020","role":"tool"}]}`),
		RouteHash:        domainsecurity.SHA256Hex([]byte("provider-route")), IssuedAt: fixture.now.Add(2 * time.Second),
		ExpiresAt: fixture.now.Add(10 * time.Minute),
	}
	receipt, err := fixture.service.IssueProviderContinuation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.VerifyProviderContinuation(context.Background(), receipt.WorkID, input, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("exact provider continuation did not verify: %v", err)
	}
	changedPayload := input
	changedPayload.CanonicalPayload = []byte(`{"messages":[{"content":"ACCOUNT_6222020202020202021","role":"tool"}]}`)
	if _, err := fixture.service.VerifyProviderContinuation(context.Background(), receipt.WorkID, changedPayload, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("different provider request content matched work id: %v", err)
	}
	changedReference := input
	changedReference.GrantReferences = []GrantReference{{GrantID: grant.GrantID, ResultItemID: "another-result"}}
	if _, err := fixture.service.VerifyProviderContinuation(context.Background(), receipt.WorkID, changedReference, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("different provider result reference matched work id: %v", err)
	}
	body, _ := json.Marshal(receipt)
	if strings.Contains(string(body), "6222020202020202020") || strings.Contains(string(body), "failed result for provider") {
		t.Fatalf("provider continuation receipt persisted private payload bytes: %s", body)
	}
}

func TestProviderContinuationRejectsMissingSyntheticOrCrossContextResult(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	if _, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindProviderContinuation, nil, "empty",
	)); err == nil {
		t.Fatal("zero-member provider continuation was issued")
	}
	if _, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindProviderContinuation,
		[]GrantReference{{GrantID: grant.GrantID, ResultItemID: "synthetic_pairing_result"}},
		"synthetic",
	)); err == nil {
		t.Fatal("synthetic provider-pairing result became durable authority")
	}
	result := fixture.addResult(grant, false, "ok", fixture.now.Add(time.Second))
	if _, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindProviderContinuation,
		[]GrantReference{{GrantID: grant.GrantID, ResultItemID: "wrong_item"}},
		"wrong-item",
	)); err == nil {
		t.Fatal("wrong durable result item was accepted")
	}
	result["contextDigest"] = domainsecurity.SHA256Hex([]byte("another-context"))
	if _, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindProviderContinuation,
		[]GrantReference{{GrantID: grant.GrantID, ResultItemID: mapString(result, "id")}},
		"cross-context",
	)); err == nil {
		t.Fatal("cross-context durable result was accepted")
	}
}

func TestProviderContinuationLeaseBindsExactPrivateSemanticRequest(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	result := fixture.addResult(grant, true, "PRIVATE_TOOL_RESULT_6222020202020202020", fixture.now.Add(time.Second))
	arguments, _ := json.Marshal(fixture.grantArguments[grant.GrantID])
	providerConfig := domainmodel.TurnConfig{ProviderID: grant.Provider, Family: "deepseek", EndpointFormat: "chat-completions", BaseURL: "https://example.invalid", Model: "model-a"}
	tools := []domainmodel.ToolSchema{{Name: "read", Description: "Read", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}
	request := ProviderContinuationRequest{
		SecurityContext: fixture.securityContext,
		References:      []domainsecurity.SettledToolReference{{GrantID: grant.GrantID, ResultItemID: mapString(result, "id")}},
		Request: domainmodel.Request{
			ProviderID: providerConfig.ProviderID, Family: providerConfig.Family, EndpointFormat: providerConfig.EndpointFormat,
			BaseURL: providerConfig.BaseURL, APIKey: "credential-rotates", Model: providerConfig.Model, Route: "agent",
			SystemPrompt: "PRIVATE_SYSTEM_PROMPT", Tools: tools,
			Messages: []domainmodel.Message{
				{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: grant.ToolCallID, Name: grant.ToolName, Arguments: arguments}}},
				{Role: "tool", ToolCallID: grant.ToolCallID, Name: grant.ToolName, Content: "PRIVATE_TOOL_RESULT_6222020202020202020"},
			},
		},
		ProviderConfig: providerConfig,
		PromptRoute:    "agent", ToolManifestHash: toolcatalogapp.ToolSchemaHash(tools), Sequence: 1, IssuedAt: fixture.now.Add(2 * time.Second),
	}
	lease, err := fixture.service.BeginProviderContinuation(context.Background(), request)
	if err != nil {
		t.Fatalf("begin provider continuation: %v", err)
	}
	if err := fixture.service.VerifyProviderContinuationLease(context.Background(), lease, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("verify provider continuation: %v", err)
	}
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, request, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("verify exact provider continuation request: %v", err)
	}
	changedRequest := request
	changedRequest.Request.Messages = append([]domainmodel.Message(nil), request.Request.Messages...)
	changedRequest.Request.Messages[1].Content = "different private tool result"
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, changedRequest, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed semantic request reused provider lease: %v", err)
	}
	changedRequest = request
	changedRequest.PromptRoute = "recovery"
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, changedRequest, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed provider route reused provider lease: %v", err)
	}
	changedRequest = request
	changedRequest.Request.SystemPrompt = "different system prompt"
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, changedRequest, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed system prompt reused provider lease: %v", err)
	}
	changedRequest = request
	changedRequest.Request.Model = "different-model"
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, changedRequest, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed provider model reused provider lease: %v", err)
	}
	changedRequest = request
	changedRequest.Request.APIKey = "rotated-credential"
	if err := fixture.service.VerifyProviderContinuationRequest(context.Background(), lease, changedRequest, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("credential rotation should not invalidate semantic provider lease: %v", err)
	}
	receiptBody, _ := json.Marshal(fixture.store.receipts[lease.WorkID()])
	if bytes.Contains(receiptBody, []byte("PRIVATE_TOOL_RESULT")) || bytes.Contains(receiptBody, []byte("6222020202020202020")) {
		t.Fatalf("provider continuation receipt leaked private request bytes: %s", receiptBody)
	}
	if _, err := fixture.service.CloseProviderContinuationLease(context.Background(), lease, domainpendingwork.StatusCompleted, "provider_completed", fixture.now.Add(4*time.Second)); err != nil {
		t.Fatalf("close provider continuation: %v", err)
	}
}

func TestProviderContinuationTerminalFinalizerClosesExpiredStaleAndCancelledOnce(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*serviceFixture)
		disposedAt func(*serviceFixture) time.Time
		wantStatus string
	}{
		{name: "cancelled", disposedAt: func(fixture *serviceFixture) time.Time { return fixture.now.Add(3 * time.Second) }, wantStatus: domainpendingwork.StatusCancelled},
		{name: "expired", disposedAt: func(fixture *serviceFixture) time.Time { return fixture.now.Add(11 * time.Minute) }, wantStatus: domainpendingwork.StatusExpired},
		{
			name: "stale context",
			mutate: func(fixture *serviceFixture) {
				fixture.setCurrentContext(mustPendingWorkCaseContextV2(fixture.t, domainsecurity.TurnSecurityContextInput{
					ThreadID: fixture.securityContext.ThreadID, TurnID: "turn-next", WorkspaceRealPath: "/workspace/case-b",
					CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-b-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
					SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-b")), ContextEpoch: fixture.securityContext.ContextEpoch + 1,
					IssuedAt: fixture.now.Add(3 * time.Second),
				}))
			},
			disposedAt: func(fixture *serviceFixture) time.Time { return fixture.now.Add(4 * time.Second) },
			wantStatus: domainpendingwork.StatusStaleContext,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			grant := fixture.addGrant(t, "read_terminal", true, "not_required", fixture.now)
			result := fixture.addResult(grant, false, "settled", fixture.now.Add(time.Second))
			_, registry, err := fixture.service.currentAuthority(fixture.securityContext)
			if err != nil {
				t.Fatal(err)
			}
			input := ProviderContinuationIssueInput{
				SecurityContext: fixture.securityContext,
				GrantReferences: []GrantReference{{GrantID: grant.GrantID, ResultItemID: mapString(result, "id")}},
				RegistryDigest:  registry.StateDigest, CanonicalPayload: []byte(`{"request":"terminal"}`),
				RouteHash: domainsecurity.SHA256Hex([]byte("terminal-route")), IssuedAt: fixture.now.Add(2 * time.Second),
				ExpiresAt: fixture.now.Add(10 * time.Minute),
			}
			receipt, err := fixture.service.IssueProviderContinuation(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			lease := ProviderContinuationLease{workID: receipt.WorkID, input: input}
			if test.mutate != nil {
				test.mutate(fixture)
			}
			disposition, err := fixture.service.CloseProviderContinuationLease(
				context.Background(), lease, domainpendingwork.StatusCancelled, "provider_cancelled", test.disposedAt(fixture),
			)
			if err != nil || disposition.Status != test.wantStatus {
				t.Fatalf("terminal disposition mismatch: disposition=%#v err=%v", disposition, err)
			}
			repeated, err := fixture.service.CloseProviderContinuationLease(
				context.Background(), lease, domainpendingwork.StatusCancelled, "provider_cancelled", test.disposedAt(fixture).Add(time.Second),
			)
			if err != nil || repeated.DispositionID != disposition.DispositionID || len(fixture.store.dispositions) != 1 {
				t.Fatalf("terminal finalizer was not idempotent: first=%#v repeated=%#v err=%v", disposition, repeated, err)
			}
		})
	}
}

func TestReportStageRequiresApprovedWritableGrantAndDurableCompletion(t *testing.T) {
	fixture := newServiceFixture(t)
	approved := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindReportStage, []GrantReference{{GrantID: approved.GrantID}}, "report",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("approved writable report stage did not verify before execution: %v", err)
	}
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "report_stage_completed", fixture.now.Add(4*time.Second)); err == nil {
		t.Fatal("report stage completed without a durable result")
	}
	fixture.addResult(approved, false, "publication receipt committed", fixture.now.Add(4*time.Second))
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "report_stage_completed", fixture.now.Add(5*time.Second)); err != nil {
		t.Fatalf("settled approved report stage did not close: %v", err)
	}

	readonlyFixture := newServiceFixture(t)
	readonly := readonlyFixture.addGrant(t, ReportStageToolName, true, "approved", readonlyFixture.now)
	if _, err := readonlyFixture.service.Issue(context.Background(), readonlyFixture.issueInput(
		domainpendingwork.KindReportStage, []GrantReference{{GrantID: readonly.GrantID}}, "readonly-report",
	)); err == nil {
		t.Fatal("read-only grant authorized report staging")
	}
	unapprovedFixture := newServiceFixture(t)
	unapproved := unapprovedFixture.addGrant(t, ReportStageToolName, false, "not_required", unapprovedFixture.now.Add(time.Second))
	if _, err := unapprovedFixture.service.Issue(context.Background(), unapprovedFixture.issueInput(
		domainpendingwork.KindReportStage, []GrantReference{{GrantID: unapproved.GrantID}}, "unapproved-report",
	)); err == nil {
		t.Fatal("unapproved writable grant authorized report staging")
	}
	wrongToolFixture := newServiceFixture(t)
	wrongTool := wrongToolFixture.addGrant(t, "bash", false, "approved", wrongToolFixture.now)
	if _, err := wrongToolFixture.service.Issue(context.Background(), wrongToolFixture.issueInput(
		domainpendingwork.KindReportStage, []GrantReference{{GrantID: wrongTool.GrantID}}, "wrong-tool-report",
	)); err == nil {
		t.Fatal("non-report writable grant authorized report staging")
	}
}

func TestCurrentFullSecurityContextIsRequiredAndStaleCloseIsMonotonic(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "stale",
	))
	if err != nil {
		t.Fatal(err)
	}
	newContext := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: "turn-new", WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		CaseID: "case-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-b-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-b")), ContextEpoch: fixture.securityContext.ContextEpoch + 1,
		IssuedAt: fixture.now.Add(time.Minute),
	})
	fixture.setCurrentContext(newContext)
	if _, err := fixture.service.VerifyOpenFor(context.Background(), receipt.WorkID, fixture.securityContext, receipt.Kind, receipt.PayloadHash, receipt.RouteHash, fixture.now.Add(2*time.Minute)); !errors.Is(err, ErrCurrentContext) {
		t.Fatalf("old full context survived case switch: %v", err)
	}
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, newContext, domainpendingwork.StatusCompleted, "completed", fixture.now.Add(2*time.Minute)); err == nil {
		t.Fatal("stale work was completed in the new context")
	}
	if _, err := fixture.service.Close(context.Background(), receipt.WorkID, newContext, domainpendingwork.StatusStaleContext, "case_context_changed", fixture.now.Add(2*time.Minute)); err != nil {
		t.Fatalf("stale work could not be monotonically closed: %v", err)
	}
}

func TestCloseAllOpenOnRestartPreflightsInventoryAndIsIdempotent(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	first, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "restart-a",
	))
	if err != nil {
		t.Fatal(err)
	}
	secondInput := fixture.issueInput(domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "restart-b")
	secondInput.PayloadHash = domainsecurity.SHA256Hex([]byte("different-payload"))
	second, err := fixture.service.Issue(context.Background(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Close(context.Background(), first.WorkID, fixture.securityContext, domainpendingwork.StatusCancelled, "cancelled_before_restart", fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	closed, err := fixture.service.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(4*time.Second))
	if err != nil || len(closed) != 1 || closed[0].WorkID != second.WorkID || closed[0].Status != domainpendingwork.StatusRestartInvalid {
		t.Fatalf("restart did not close exactly the open inventory: closed=%#v err=%v", closed, err)
	}
	closed, err = fixture.service.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(5*time.Second))
	if err != nil || len(closed) != 0 {
		t.Fatalf("restart close was not idempotent: closed=%#v err=%v", closed, err)
	}

	corruptFixture := newServiceFixture(t)
	corruptGrant := corruptFixture.addGrant(t, "read", true, "not_required", corruptFixture.now)
	valid, err := corruptFixture.service.Issue(context.Background(), corruptFixture.issueInput(
		domainpendingwork.KindToolBatch, []GrantReference{{GrantID: corruptGrant.GrantID}}, "corrupt",
	))
	if err != nil {
		t.Fatal(err)
	}
	corrupt := corruptFixture.store.receipts[valid.WorkID]
	corrupt.AuthoritySignature = strings.Repeat("A", len(corrupt.AuthoritySignature))
	corruptFixture.store.receipts[valid.WorkID] = corrupt
	before := len(corruptFixture.store.dispositions)
	if _, err := corruptFixture.service.CloseAllOpenOnRestart(context.Background(), corruptFixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("corrupt restart inventory was accepted")
	}
	if len(corruptFixture.store.dispositions) != before {
		t.Fatal("restart inventory mutated before full preflight succeeded")
	}
}

func TestPendingWorkReceiptPersistsOnlySafeBindingsAndHashes(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, "read", true, "not_required", fixture.now)
	privatePayload := []byte(`{"account":"6222020202020202020","reasoning":"PRIVATE_REASONING_SENTINEL"}`)
	payloadHash, err := fixture.service.KeyedPayloadHash(context.Background(), "tool_batch.semantic_request", privatePayload)
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.issueInput(domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "privacy")
	input.PayloadHash = payloadHash
	receipt, err := fixture.service.Issue(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(receipt)
	for _, forbidden := range []string{
		fixture.securityContext.WorkspaceRealPath, fixture.securityContext.CaseID, fixture.securityContext.TenantID,
		fixture.securityContext.UserID, "prompt", "reasoning", "thinking", "6222020202020202020",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("pending-work receipt leaked forbidden content %q: %s", forbidden, body)
		}
	}
}

func TestKeyedPayloadHashBindsExactCanonicalContentWithoutPublicDigest(t *testing.T) {
	fixture := newServiceFixture(t)
	left := []byte(`{"account":"6222020202020202020","reasoning":"PRIVATE_REASONING_SENTINEL"}`)
	right := []byte(`{"account":"6222020202020202021","reasoning":"PRIVATE_REASONING_SENTINEL"}`)
	leftHash, err := fixture.service.KeyedPayloadHash(context.Background(), "provider_continuation.semantic_request", left)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := fixture.service.KeyedPayloadHash(context.Background(), "provider_continuation.semantic_request", left)
	if err != nil || repeated != leftHash {
		t.Fatalf("same canonical semantic request did not produce a stable MAC: first=%q repeated=%q err=%v", leftHash, repeated, err)
	}
	rightHash, err := fixture.service.KeyedPayloadHash(context.Background(), "provider_continuation.semantic_request", right)
	if err != nil || rightHash == leftHash {
		t.Fatalf("different exact content did not change the keyed MAC: left=%q right=%q err=%v", leftHash, rightHash, err)
	}
	if leftHash == domainsecurity.SHA256Hex(left) {
		t.Fatal("payload authority used a publicly guessable unkeyed digest")
	}
	if strings.Contains(leftHash, "622202") || strings.Contains(leftHash, "REASONING") {
		t.Fatalf("keyed payload digest leaked raw semantic bytes: %q", leftHash)
	}
	if _, err := fixture.service.KeyedPayloadHash(context.Background(), "provider_continuation.semantic_request", []byte(`{"b":2, "a":1}`)); err == nil {
		t.Fatal("non-canonical JSON spelling was accepted")
	}
	if _, err := fixture.service.KeyedPayloadHash(context.Background(), "provider_continuation.semantic_request", []byte(`{"a":1,"a":2}`)); err == nil {
		t.Fatal("ambiguous duplicate-key JSON was accepted")
	}
}

type serviceFixture struct {
	t               *testing.T
	now             time.Time
	clockMu         sync.RWMutex
	clockNow        time.Time
	securityContext domainsecurity.TurnSecurityContext
	authority       *memoryPendingWorkAuthority
	store           *memoryPendingWorkStore
	threads         *memoryPendingWorkThreads
	service         *Service
	grantArguments  map[string]map[string]any
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	now := time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC)
	securityContext := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pending", TurnID: "turn-pending", WorkspaceRealPath: "/workspace/case-a",
		TenantID: "tenant-private", UserID: "user-private", CaseID: "case-private",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-a-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-a")), ContextEpoch: 7, IssuedAt: now,
	})
	contextRecord := mapRecord(securityContext)
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": contextRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "running",
			"securityContext": contextRecord, "items": []any{},
		}},
	}
	authority := newMemoryPendingWorkAuthority(t)
	store := newMemoryPendingWorkStore()
	threads := &memoryPendingWorkThreads{threads: map[string]map[string]any{securityContext.ThreadID: thread}}
	fixture := &serviceFixture{
		t: t, now: now, securityContext: securityContext, authority: authority, store: store, threads: threads,
		clockNow: now, service: NewService(authority, store, threads), grantArguments: map[string]map[string]any{},
	}
	fixture.service.now = fixture.boundaryTime
	return fixture
}

func (fixture *serviceFixture) installContext(securityContext domainsecurity.TurnSecurityContext) {
	fixture.securityContext = securityContext
	contextRecord := mapRecord(securityContext)
	fixture.threads.threads = map[string]map[string]any{
		securityContext.ThreadID: {
			"id": securityContext.ThreadID, "securityState": contextRecord,
			"turns": []any{map[string]any{
				"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "running",
				"securityContext": contextRecord, "items": []any{},
			}},
		},
	}
	fixture.grantArguments = map[string]map[string]any{}
}

func mustPendingWorkCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	context, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func mustPendingWorkWitnessedBoundaryContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	quarantined, err := securitycontexttest.BoundaryOnlyContextV2(input)
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
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: input.IssuedAt,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func pendingWorkCallForContext(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	toolName string,
	readOnly bool,
	approvalState string,
	issuedAt time.Time,
) appmodel.PendingToolCall {
	t.Helper()
	call := domainmodel.ToolCall{
		ID: pendingWorkTestToolCallID(securityContext.ThreadID + "-" + toolName), Name: toolName,
		Arguments: json.RawMessage(`{"member":"test"}`),
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-test", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema:" + call.Name)), ScopeHash: domainsecurity.SHA256Hex([]byte("scope:" + call.Name)),
		ReadOnly: readOnly, ApprovalState: approvalState, IssuedAt: issuedAt,
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: grant.Provider,
		Call: call, SecurityContext: securityContext, ExecutionGrant: grant,
	}
}

func (fixture *serviceFixture) boundaryTime() time.Time {
	fixture.clockMu.RLock()
	defer fixture.clockMu.RUnlock()
	return fixture.clockNow
}

func (fixture *serviceFixture) setBoundaryTime(value time.Time) {
	fixture.clockMu.Lock()
	fixture.clockNow = value.UTC()
	fixture.clockMu.Unlock()
}

type pendingWorkBoundaryGate struct {
	effectEntered     chan struct{}
	effectRelease     chan struct{}
	transitionEntered chan struct{}
	transitionRelease chan struct{}
	blockEffect       bool
	blockTransition   bool
	effectEnteredOnce sync.Once
	transitionOnce    sync.Once
}

func newPendingWorkBoundaryGate(blockEffect, blockTransition bool) *pendingWorkBoundaryGate {
	return &pendingWorkBoundaryGate{
		effectEntered: make(chan struct{}), effectRelease: make(chan struct{}), transitionEntered: make(chan struct{}),
		transitionRelease: make(chan struct{}), blockEffect: blockEffect, blockTransition: blockTransition,
	}
}

func (gate *pendingWorkBoundaryGate) AcquireEffect(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	return gate.acquireEffect(ctx)
}

func (gate *pendingWorkBoundaryGate) AcquireOrdinaryEffect(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	return gate.acquireEffect(ctx)
}

func (gate *pendingWorkBoundaryGate) acquireEffect(ctx context.Context) (context.Context, func(), error) {
	gate.effectEnteredOnce.Do(func() { close(gate.effectEntered) })
	if gate.blockEffect {
		select {
		case <-gate.effectRelease:
		case <-ctx.Done():
			return ctx, nil, ctx.Err()
		}
	}
	return ctx, func() {}, nil
}

func (gate *pendingWorkBoundaryGate) AcquireTransition(ctx context.Context, _ domainsecurity.TurnSecurityContext) (func(), error) {
	gate.transitionOnce.Do(func() { close(gate.transitionEntered) })
	if gate.blockTransition {
		select {
		case <-gate.transitionRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return func() {}, nil
}

func (fixture *serviceFixture) issueInput(kind string, references []GrantReference, label string) IssueInput {
	return IssueInput{
		Kind: kind, SecurityContext: fixture.securityContext, GrantReferences: references,
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload:" + label)), RouteHash: domainsecurity.SHA256Hex([]byte("route:" + label)),
		IssuedAt: fixture.now.Add(2 * time.Second), ExpiresAt: fixture.now.Add(10 * time.Minute),
	}
}

func (fixture *serviceFixture) addGrant(t *testing.T, toolName string, readOnly bool, approvalState string, issuedAt time.Time) domainsecurity.ExecutionGrant {
	t.Helper()
	arguments := map[string]any{"member": toolName}
	argumentBytes, _ := json.Marshal(arguments)
	toolCallID := pendingWorkTestToolCallID(toolName)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-test", ServerIdentity: "host:builtin", ToolName: toolName,
		ToolCallID: toolCallID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBytes),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema:" + toolName)), ScopeHash: domainsecurity.SHA256Hex([]byte("scope:" + toolName)),
		ReadOnly: readOnly, ApprovalState: approvalState, IssuedAt: issuedAt, ExpiresAt: fixture.now.Add(20 * time.Minute),
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(fixture.securityContext.TurnID, toolCallID), "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": grant.IssuedAt,
		"contextDigest": fixture.securityContext.ContextDigest, "contextEpoch": float64(fixture.securityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": mapRecord(grant),
	}
	fixture.appendItem(item)
	fixture.grantArguments[grant.GrantID] = arguments
	return grant
}

func (fixture *serviceFixture) addResult(grant domainsecurity.ExecutionGrant, isError bool, output string, finishedAt time.Time) map[string]any {
	stamp := finishedAt.UTC().Format(time.RFC3339Nano)
	item := map[string]any{
		"id": domaintoolresult.ToolResultItemIDV1(fixture.securityContext.TurnID, grant.ToolCallID), "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "output": output, "isError": isError,
		"createdAt": stamp, "finishedAt": stamp, "contextDigest": fixture.securityContext.ContextDigest,
		"contextEpoch": float64(fixture.securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
	}
	fixture.appendItem(item)
	return item
}

func (fixture *serviceFixture) pendingCall(grant domainsecurity.ExecutionGrant) appmodel.PendingToolCall {
	arguments, _ := json.Marshal(fixture.grantArguments[grant.GrantID])
	return appmodel.PendingToolCall{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		ProviderID: grant.Provider, Call: domainmodel.ToolCall{ID: grant.ToolCallID, Name: grant.ToolName, Arguments: arguments},
		SecurityContext: fixture.securityContext, ExecutionGrant: grant,
	}
}

func (fixture *serviceFixture) appendItem(item map[string]any) {
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	turns := thread["turns"].([]any)
	turn := turns[0].(map[string]any)
	turn["items"] = append(turn["items"].([]any), item)
}

func (fixture *serviceFixture) setCurrentContext(current domainsecurity.TurnSecurityContext) {
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	thread["securityState"] = mapRecord(current)
	thread["turns"] = append(thread["turns"].([]any), map[string]any{
		"id": current.TurnID, "threadId": current.ThreadID, "status": "running", "securityContext": mapRecord(current), "items": []any{},
	})
}

type memoryPendingWorkThreads struct {
	threads map[string]map[string]any
}

func (reader *memoryPendingWorkThreads) GetThread(threadID string) (map[string]any, error) {
	thread, found := reader.threads[threadID]
	if !found {
		return nil, errors.New("thread not found")
	}
	return thread, nil
}

type memoryPendingWorkAuthority struct {
	mu         sync.Mutex
	signCount  int
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newMemoryPendingWorkAuthority(t *testing.T) *memoryPendingWorkAuthority {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &memoryPendingWorkAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *memoryPendingWorkAuthority) KeyID() string { return authority.keyID }
func (authority *memoryPendingWorkAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *memoryPendingWorkAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	authority.mu.Lock()
	authority.signCount++
	authority.mu.Unlock()
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *memoryPendingWorkAuthority) SignCount() int {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.signCount
}
func (authority *memoryPendingWorkAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("untrusted signature")
	}
	return nil
}

type memoryPendingWorkStore struct {
	mu           sync.Mutex
	receipts     map[string]domainpendingwork.PendingWorkReceiptV1
	dispositions map[string]domainpendingwork.PendingWorkDispositionV1
}

func newMemoryPendingWorkStore() *memoryPendingWorkStore {
	return &memoryPendingWorkStore{
		receipts: map[string]domainpendingwork.PendingWorkReceiptV1{}, dispositions: map[string]domainpendingwork.PendingWorkDispositionV1{},
	}
}

func (store *memoryPendingWorkStore) PutReceiptIfAbsent(_ context.Context, receipt domainpendingwork.PendingWorkReceiptV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, found := store.receipts[receipt.WorkID]; found {
		return errors.New("receipt exists")
	}
	store.receipts[receipt.WorkID] = receipt
	return nil
}

func (store *memoryPendingWorkStore) CreateReceiptExclusive(_ context.Context, receipt domainpendingwork.PendingWorkReceiptV1) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, found := store.receipts[receipt.WorkID]; found {
		if existing.ReceiptID == receipt.ReceiptID {
			return false, nil
		}
		return false, errors.New("receipt conflicts")
	}
	store.receipts[receipt.WorkID] = receipt
	return true, nil
}

func (store *memoryPendingWorkStore) ReadReceipt(_ context.Context, workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	receipt, found := store.receipts[workID]
	if !found {
		return domainpendingwork.PendingWorkReceiptV1{}, pendingworkstoreport.ErrNotFound
	}
	return receipt, nil
}

func (store *memoryPendingWorkStore) ListReceipts(context.Context) ([]domainpendingwork.PendingWorkReceiptV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]domainpendingwork.PendingWorkReceiptV1, 0, len(store.receipts))
	for _, receipt := range store.receipts {
		result = append(result, receipt)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WorkID < result[j].WorkID })
	return result, nil
}

func (store *memoryPendingWorkStore) PutDispositionIfAbsent(_ context.Context, disposition domainpendingwork.PendingWorkDispositionV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, found := store.dispositions[disposition.WorkID]; found {
		return errors.New("disposition exists")
	}
	store.dispositions[disposition.WorkID] = disposition
	return nil
}

func (store *memoryPendingWorkStore) ReadDisposition(_ context.Context, workID string) (domainpendingwork.PendingWorkDispositionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	disposition, found := store.dispositions[workID]
	if !found {
		return domainpendingwork.PendingWorkDispositionV1{}, pendingworkstoreport.ErrNotFound
	}
	return disposition, nil
}

func (store *memoryPendingWorkStore) ListDispositions(context.Context) ([]domainpendingwork.PendingWorkDispositionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(store.dispositions))
	for _, disposition := range store.dispositions {
		result = append(result, disposition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WorkID < result[j].WorkID })
	return result, nil
}

func (store *memoryPendingWorkStore) SnapshotInventory(context.Context) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts := make([]domainpendingwork.PendingWorkReceiptV1, 0, len(store.receipts))
	for _, receipt := range store.receipts {
		receipts = append(receipts, receipt)
	}
	dispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(store.dispositions))
	for _, disposition := range store.dispositions {
		dispositions = append(dispositions, disposition)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].WorkID < receipts[j].WorkID })
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].WorkID < dispositions[j].WorkID })
	return receipts, dispositions, nil
}

func (store *memoryPendingWorkStore) HasRecords(context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.receipts) > 0 || len(store.dispositions) > 0, nil
}

func mapRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
