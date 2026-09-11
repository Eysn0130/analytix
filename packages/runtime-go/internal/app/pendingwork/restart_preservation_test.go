package pendingwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type reportRestartPrimaryFixture struct {
	threads    *memoryPendingWorkThreads
	beforeRead func()
}

func (reader reportRestartPrimaryFixture) ReadPrimaryThreadSnapshotV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if reader.beforeRead != nil {
		reader.beforeRead()
	}
	thread, err := reader.threads.GetThread(threadID)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	body, err := json.Marshal(thread)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	var original map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&original); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	return recoveryport.PrimaryThreadSnapshotV1{ThreadID: threadID, ThreadFileSHA256: domainsecurity.SHA256Hex(body), Thread: original}, nil
}

func (reportRestartPrimaryFixture) ReadCommittedEventLogSHA256V1(context.Context, string) (string, error) {
	return "", errors.New("pending preservation cannot infer an ordinary publication")
}

func beginReportRestartFixture(t *testing.T, fixture *serviceFixture) (ReportStageLease, ReportStageRequest) {
	t.Helper()
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("preserved stage")), IssuedAt: fixture.now.Add(time.Second)}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return lease, request
}

func TestReportRestartPreservationRetainsUnknownAndRejectsEffects(t *testing.T) {
	for _, persistedUnknown := range []bool{false, true} {
		name := "open original"
		if persistedUnknown {
			name = "persisted unknown"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newServiceFixture(t)
			lease, request := beginReportRestartFixture(t, fixture)
			if persistedUnknown {
				if _, err := fixture.service.CloseAllOpenOnRestart(ctx, fixture.now.Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(map[string]any{"receipts": fixture.store.receipts, "dispositions": fixture.store.dispositions, "threads": fixture.threads.threads})
			restarted := NewService(fixture.authority, fixture.store, fixture.threads)
			scope, err := restarted.PlanReportRestartPreservationV1(ctx, reportRestartPrimaryFixture{threads: fixture.threads})
			if err != nil {
				t.Fatal(err)
			}
			if !scope.OwnsThread(fixture.securityContext.ThreadID) || scope.OwnsThread("unrelated") || len(scope.Contexts()) != 1 || scope.Contexts()[0] != fixture.securityContext {
				t.Fatal("preservation scope lost exact Core context")
			}
			if err := restarted.PreserveReportRestartScopeV1(ctx, scope); err != nil {
				t.Fatal(err)
			}
			if dispositions, err := restarted.CloseAllOpenOnRestart(ctx, fixture.now.Add(2*time.Minute)); err != nil || len(dispositions) != 0 {
				t.Fatalf("preservation entered generic disposal: %v", err)
			}
			if err := restarted.VerifyReportStageRequest(ctx, lease, request, fixture.now.Add(3*time.Minute)); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("preserved lease verified for effect: %v", err)
			}
			if _, err := restarted.BeginReportStage(ctx, request); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("preserved report reminted a lease: %v", err)
			}
			if _, err := restarted.ResolveReportStageLease(ctx, lease, request); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("preserved report resolved an executable lease: %v", err)
			}
			receipt := fixture.store.receipts[lease.WorkID()]
			if _, err := restarted.dispose(ctx, receipt, domainpendingwork.StatusFailed, "report_stage_failed", fixture.now.Add(3*time.Minute)); !errors.Is(err, ErrRestartPreserved) {
				t.Fatalf("lower disposition boundary changed held work: %v", err)
			}
			if err := restarted.PreserveReportRestartScopeV1(ctx, ReportRestartScopeV1{}); err == nil {
				t.Fatal("zero scope released an installed hold")
			}
			after, _ := json.Marshal(map[string]any{"receipts": fixture.store.receipts, "dispositions": fixture.store.dispositions, "threads": fixture.threads.threads})
			if string(before) != string(after) {
				t.Fatal("preservation changed original unknown bytes or Core graph")
			}
		})
	}
}

func TestReportRestartPreservationDoesNotClaimInvalidCoreGraph(t *testing.T) {
	for _, fault := range []string{"receipt signature", "missing grant", "non-object turn", "non-object item", "missing sibling context", "wrong sibling context", "primary changed", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newServiceFixture(t)
			lease, _ := beginReportRestartFixture(t, fixture)
			thread := fixture.threads.threads[fixture.securityContext.ThreadID]
			switch fault {
			case "receipt signature":
				receipt := fixture.store.receipts[lease.WorkID()]
				receipt.PayloadHash = domainsecurity.SHA256Hex([]byte("different"))
				fixture.store.receipts[lease.WorkID()] = receipt
			case "missing grant":
				thread["turns"].([]any)[0].(map[string]any)["items"] = []any{}
			case "non-object turn":
				thread["turns"] = append(thread["turns"].([]any), nil)
			case "non-object item":
				turn := thread["turns"].([]any)[0].(map[string]any)
				turn["items"] = append(turn["items"].([]any), nil)
			case "missing sibling context", "wrong sibling context":
				sibling := map[string]any{"id": "turn-older", "threadId": fixture.securityContext.ThreadID, "items": []any{}}
				if fault == "wrong sibling context" {
					sibling["securityContext"] = mapRecord(fixture.securityContext)
				}
				thread["turns"] = append(thread["turns"].([]any), sibling)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := reportRestartPrimaryFixture{threads: fixture.threads}
			if fault == "primary changed" {
				reads := 0
				reader.beforeRead = func() {
					reads++
					if reads == 2 {
						thread["updatedAt"] = "2026-09-07T00:00:00Z"
					}
				}
			}
			if fault == "cancelled" {
				cancel()
			}
			if scope, err := fixture.service.PlanReportRestartPreservationV1(ctx, reader); err == nil || len(scope.ThreadIDs()) != 0 {
				t.Fatal("invalid Core graph supplied a preservation scope")
			}
		})
	}
}

func TestReportRestartPreservationRejectsStaleScopeBeforeInstallation(t *testing.T) {
	for _, fault := range []string{"primary changed", "pending changed"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newServiceFixture(t)
			lease, _ := beginReportRestartFixture(t, fixture)
			scope, err := fixture.service.PlanReportRestartPreservationV1(context.Background(), reportRestartPrimaryFixture{threads: fixture.threads})
			if err != nil {
				t.Fatal(err)
			}
			if fault == "primary changed" {
				fixture.threads.threads[fixture.securityContext.ThreadID]["updatedAt"] = "2026-09-07T00:00:00Z"
			} else {
				receipt := fixture.store.receipts[lease.WorkID()]
				if _, err := fixture.service.dispose(context.Background(), receipt, domainpendingwork.StatusFailed, "report_stage_failed", fixture.now.Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			restarted := NewService(fixture.authority, fixture.store, fixture.threads)
			if err := restarted.PreserveReportRestartScopeV1(context.Background(), scope); err == nil || restarted.restartOwnsThread(fixture.securityContext.ThreadID) {
				t.Fatal("stale scope was installed")
			}
		})
	}
}

func TestReportRestartPreservationKeepsIndependentPendingWorkUsable(t *testing.T) {
	ctx := context.Background()
	fixture := newServiceFixture(t)
	lease, _ := beginReportRestartFixture(t, fixture)
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	scope, err := restarted.PlanReportRestartPreservationV1(ctx, reportRestartPrimaryFixture{threads: fixture.threads})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.PreserveReportRestartScopeV1(ctx, scope); err != nil {
		t.Fatal(err)
	}
	ordinary := newServiceFixture(t)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-independent", TurnID: "turn-independent", WorkspaceRealPath: "/synthetic/independent", ContextEpoch: 1, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ordinary.installContext(securityContext)
	grant := ordinary.addGrant(t, "read", true, "not_required", ordinary.now)
	fixture.threads.threads[securityContext.ThreadID] = ordinary.threads.threads[securityContext.ThreadID]
	receipt, err := restarted.Issue(ctx, ordinary.issueInput(domainpendingwork.KindToolBatch, []GrantReference{{GrantID: grant.GrantID}}, "independent"))
	if err != nil {
		t.Fatalf("independent ordinary work was blocked: %v", err)
	}
	dispositions, err := restarted.CloseAllOpenOnRestart(ctx, fixture.now.Add(time.Minute))
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != receipt.WorkID {
		t.Fatalf("independent restart work was not handled: %v", err)
	}
	if _, found := fixture.store.dispositions[lease.WorkID()]; found {
		t.Fatal("unrelated work closed the held report")
	}
}

func reportRestartSiblingFixture(t *testing.T, original *serviceFixture, threadID, turnID string) *serviceFixture {
	t.Helper()
	fixture := newServiceFixture(t)
	frozen := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/case-a",
		TenantID: "tenant-private", UserID: "user-private", CaseID: "case-private",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-a-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-a")), ContextEpoch: 7, IssuedAt: original.now,
	})
	fixture.installContext(frozen)
	fixture.authority, fixture.store = original.authority, original.store
	fixture.service = NewService(fixture.authority, fixture.store, fixture.threads)
	fixture.service.now = fixture.boundaryTime
	return fixture
}

func TestReportRestartPreservationRejectsNewUnownedReportAtInstall(t *testing.T) {
	for _, initialReport := range []bool{false, true} {
		name := "empty_scope"
		if initialReport {
			name = "existing_other_thread"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			if initialReport {
				beginReportRestartFixture(t, fixture)
			}
			scope, err := fixture.service.PlanReportRestartPreservationV1(context.Background(), reportRestartPrimaryFixture{threads: fixture.threads})
			if err != nil {
				t.Fatal(err)
			}
			other := reportRestartSiblingFixture(t, fixture, "thread-new-report", "turn-new-report")
			beginReportRestartFixture(t, other)
			fixture.threads.threads[other.securityContext.ThreadID] = other.threads.threads[other.securityContext.ThreadID]
			if err := fixture.service.PreserveReportRestartScopeV1(context.Background(), scope); err == nil {
				t.Fatal("stale scope installed despite newly unresolved report outside its original threads")
			}
		})
	}
}

func TestReportRestartPreservationRejectsSettledDecisionAuthorization(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("settled open report")), IssuedAt: fixture.now.Add(time.Second)}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	decisionID := domainsecurity.SHA256Hex([]byte("preserved-decision"))
	decisionDigest := domainsecurity.SHA256Hex([]byte("preserved-decision-record"))
	result := fixture.addResult(grant, false, "private report", fixture.now.Add(2*time.Second))
	result["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"))
	result["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(domaintoolresult.NewHostReportAdmissionV1(decisionID, decisionDigest))
	if _, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(context.Background(), lease.WorkID(), decisionID, decisionDigest); err != nil {
		t.Fatalf("healthy settled control failed: %v", err)
	}
	scope, err := fixture.service.PlanReportRestartPreservationV1(context.Background(), reportRestartPrimaryFixture{threads: fixture.threads})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.PreserveReportRestartScopeV1(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(context.Background(), lease.WorkID(), decisionID, decisionDigest); !errors.Is(err, ErrRestartPreserved) {
		t.Errorf("held result became settlement authority: %v", err)
	}
	called := false
	err = fixture.service.WithTrustedSettledReportStageForDecisionV1(context.Background(), lease.WorkID(), decisionID, decisionDigest, func(TrustedSettledReportStageV1) error { called = true; return nil })
	if !errors.Is(err, ErrRestartPreserved) || called {
		t.Fatalf("held result invoked decision callback: called=%t err=%v", called, err)
	}
}

func TestReportRestartPreservationRejectsEarlierCompletedAuthorizationOnHeldThread(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("earlier completed report")), IssuedAt: fixture.now.Add(time.Second)}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "completed report", fixture.now.Add(2*time.Second))
	if _, err := fixture.service.CloseReportStageLease(context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	other := reportRestartSiblingFixture(t, fixture, fixture.securityContext.ThreadID, "turn-later-report")
	beginReportRestartFixture(t, other)
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	thread["turns"] = append(thread["turns"].([]any), other.threads.threads[other.securityContext.ThreadID]["turns"].([]any)...)
	thread["securityState"] = mapRecord(other.securityContext)
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID()); err != nil {
		t.Fatalf("healthy earlier completion control failed: %v", err)
	}
	scope, err := fixture.service.PlanReportRestartPreservationV1(context.Background(), reportRestartPrimaryFixture{threads: fixture.threads})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.PreserveReportRestartScopeV1(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID()); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("earlier held completion remained delivery authority: %v", err)
	}
	if _, err := fixture.service.TrustedInventoryV1(context.Background()); err != nil {
		t.Fatalf("hold prevented full trusted inventory validation: %v", err)
	}
}
