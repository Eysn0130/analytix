package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestReportStageLeasePersistsExactContextGrantAndRevalidatesBeforeSideEffect(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant),
		StageInputHash:  domainsecurity.SHA256Hex([]byte("verified-claim-ledger+snapshot+pii-projection")),
		IssuedAt:        fixture.now.Add(2 * time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.ReadReceipt(context.Background(), lease.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != domainpendingwork.KindReportStage || receipt.Context.ContextDigest != fixture.securityContext.ContextDigest ||
		len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != grant.GrantID || receipt.RouteHash != reportStageRouteHash() {
		t.Fatalf("report stage receipt lost exact private authority: %#v", receipt)
	}
	if _, err := fixture.service.BeginReportStage(context.Background(), request); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("open report stage minted another write lease: %v", err)
	}
	if err := fixture.service.VerifyReportStageRequest(context.Background(), lease, request, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatalf("exact report stage request failed pre-side-effect verification: %v", err)
	}
	changed := request
	changed.StageInputHash = domainsecurity.SHA256Hex([]byte("different-claim-ledger"))
	if err := fixture.service.VerifyReportStageRequest(context.Background(), lease, changed, fixture.now.Add(3*time.Second)); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed report stage semantic input reused a lease: %v", err)
	}
	if _, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(4*time.Second),
	); err == nil {
		t.Fatal("report stage completed before its exact grant had a durable result")
	}
	fixture.addResult(grant, false, "staged bytes retained privately", fixture.now.Add(4*time.Second))
	disposition, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(5*time.Second),
	)
	if err != nil || disposition.Status != domainpendingwork.StatusCompleted || disposition.ReceiptID != receipt.ReceiptID {
		t.Fatalf("settled report stage did not close against its exact receipt: disposition=%#v err=%v", disposition, err)
	}
	repeated, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(6*time.Second),
	)
	if err != nil || repeated.DispositionID != disposition.DispositionID || len(fixture.store.dispositions) != 1 {
		t.Fatalf("report stage terminal disposition was not idempotent: first=%#v repeated=%#v err=%v", disposition, repeated, err)
	}
}

func TestHistoricalReportStageInputBindsOriginalKeyedPayload(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("original canonical stage")), IssuedAt: fixture.now.Add(time.Second)}
	ctx := context.Background()
	lease, err := fixture.service.BeginReportStage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.ReadReceipt(ctx, lease.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := fixture.service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	// No store or current grant reader is available to this historical verifier.
	verifier := NewService(fixture.service.authority, nil, nil)
	if err := verifier.VerifyHistoricalReportStageInputV1(ctx, receipt, thread, grant.ToolCallID, request.StageInputHash); err != nil {
		t.Fatal(err)
	}
	changed := domainsecurity.SHA256Hex([]byte("different signed attempt stage input"))
	if err := verifier.VerifyHistoricalReportStageInputV1(ctx, receipt, thread, grant.ToolCallID, changed); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("changed stage input retained the original keyed payload: %v", err)
	}
	if err := verifier.VerifyHistoricalReportStageInputV1(ctx, receipt, thread, "different-call", request.StageInputHash); !errors.Is(err, ErrGrantAuthority) {
		t.Fatalf("different actual call retained stage authority: %v", err)
	}
	if len(fixture.store.dispositions) != 0 || len(fixture.store.receipts) != 1 {
		t.Fatal("historical verification wrote pending state")
	}
}

func TestReportStageLeaseRejectsStaleContextBeforeExecution(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant),
		StageInputHash:  domainsecurity.SHA256Hex([]byte("stage-input")),
		IssuedAt:        fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	newContext := mustPendingWorkCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: "turn-report-new", WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		TenantID: fixture.securityContext.TenantID, UserID: fixture.securityContext.UserID, CaseID: "case-report-new",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-report-new")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-report-new"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-report-new")), ContextEpoch: fixture.securityContext.ContextEpoch + 1,
		IssuedAt: fixture.now.Add(time.Minute),
	})
	fixture.setCurrentContext(newContext)
	if err := fixture.service.VerifyReportStageRequest(context.Background(), lease, request, fixture.now.Add(2*time.Minute)); !errors.Is(err, ErrCurrentContext) {
		t.Fatalf("stale report stage survived a case/context switch: %v", err)
	}
	disposition, err := fixture.service.CloseStaleReportStageLease(
		context.Background(), lease, request, newContext, fixture.now.Add(2*time.Minute),
	)
	if err != nil || disposition.Status != domainpendingwork.StatusStaleContext || disposition.ReasonCode != "report_stage_stale_context" {
		t.Fatalf("stale report stage did not receive a durable fixed downgrade: disposition=%#v err=%v", disposition, err)
	}
}

func TestOpenReportStageBecomesOutcomeUnknownAndNeverResumable(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant),
		StageInputHash:  domainsecurity.SHA256Hex([]byte("stage-input")),
		IssuedAt:        fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(2*time.Minute))
	if err != nil || len(dispositions) != 1 || dispositions[0].WorkID != lease.WorkID() ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown || dispositions[0].ReasonCode != "report_stage_outcome_unknown_after_restart" {
		t.Fatalf("open report stage did not fail closed on restart: dispositions=%#v err=%v", dispositions, err)
	}
	if err := restarted.VerifyReportStageRequest(context.Background(), lease, request, fixture.now.Add(3*time.Minute)); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("restarted report stage remained executable: %v", err)
	}
	if _, err := restarted.BeginReportStage(context.Background(), request); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("restarted report stage minted a replacement lease: %v", err)
	}
	inventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := RestartOutcomeUnknownGrantsV1(inventory)
	if err != nil || len(unknown) != 1 || unknown[0].GrantID != grant.GrantID {
		t.Fatalf("report stage outcome-unknown authority was not replayable: unknown=%#v err=%v", unknown, err)
	}
}

func TestReportStageGrantCannotMintAnotherEffectForChangedStageInput(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("first-stage-input")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.StageInputHash = domainsecurity.SHA256Hex([]byte("different-stage-input"))
	if _, err := fixture.service.BeginReportStage(context.Background(), changed); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("same report grant minted another effect for changed content: %v", err)
	}
	receipts, err := fixture.store.ListReceipts(context.Background())
	if err != nil || len(receipts) != 1 || receipts[0].WorkID != lease.WorkID() {
		t.Fatalf("changed report content created another durable effect: receipts=%#v err=%v", receipts, err)
	}
}

func TestRestartPreservesKnownDurableReportStageResult(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("known-stage-input")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	_, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "known durable report result", fixture.now.Add(2*time.Second))
	restarted := NewService(fixture.authority, fixture.store, fixture.threads)
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(3*time.Second))
	if err != nil || len(dispositions) != 1 || dispositions[0].Status != domainpendingwork.StatusCompleted ||
		dispositions[0].ReasonCode != "report_stage_completed" {
		t.Fatalf("known report result was downgraded on restart: dispositions=%#v err=%v", dispositions, err)
	}
}

func TestResolveTrustedCompletedReportStageRequiresSignedDurableTerminal(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("trusted-completed-stage")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID()); !errors.Is(err, ErrWorkAlreadyOpen) {
		t.Fatalf("open report stage acquired completed authority: %v", err)
	}
	fixture.addResult(grant, false, "durable staged artifact result", fixture.now.Add(2*time.Second))
	wantDisposition, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID())
	if err != nil || terminal.Receipt.WorkID != lease.WorkID() || terminal.Disposition.DispositionID != wantDisposition.DispositionID ||
		terminal.ResultItemID == "" || terminal.ResultItemDigest == "" {
		t.Fatalf("trusted completed report stage was not resolved: terminal=%#v err=%v", terminal, err)
	}
	terminal.Receipt.GrantMembers[0].GrantID = "mutated-caller-copy"
	terminal.ResultItem["toolName"] = "mutated-caller-copy"
	if output, ok := terminal.ResultItem["output"].(map[string]any); ok {
		output["status"] = "mutated-caller-copy"
	}
	again, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID())
	againOutput, _ := again.ResultItem["output"].(map[string]any)
	if err != nil || again.Receipt.GrantMembers[0].GrantID != grant.GrantID || again.ResultItem["toolName"] != ReportStageToolName ||
		againOutput["status"] == "mutated-caller-copy" {
		t.Fatalf("caller mutated store-owned report-stage authority: again=%#v err=%v", again, err)
	}

	fixture.store.mu.Lock()
	tampered := fixture.store.dispositions[lease.WorkID()]
	tampered.AuthoritySignature = "tampered"
	fixture.store.dispositions[lease.WorkID()] = tampered
	fixture.store.mu.Unlock()
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID()); !errors.Is(err, ErrGrantAuthority) {
		t.Fatalf("untrusted report-stage disposition acquired completion authority: %v", err)
	}
}

func TestResolveTrustedSettledReportStageRequiresDecisionAdmissionBeforeDisposition(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("decision-bound-stage")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	decisionID := domainsecurity.SHA256Hex([]byte("report-decision"))
	decisionDigest := domainsecurity.SHA256Hex([]byte("report-decision-record"))
	result := fixture.addResult(grant, false, "private report result", fixture.now.Add(2*time.Second))
	result["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(
		domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
	)
	result["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(
		domaintoolresult.NewHostReportAdmissionV1(decisionID, decisionDigest),
	)
	settled, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(
		context.Background(), lease.WorkID(), decisionID, decisionDigest,
	)
	if err != nil || settled.Receipt.WorkID != lease.WorkID() || settled.Grant != grant ||
		settled.ResultItemID != result["id"] || settled.ResultItemDigest == "" || settled.SettledAt.IsZero() {
		t.Fatalf("decision-bound settled report stage was not resolved: settled=%#v err=%v", settled, err)
	}
	if _, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(
		context.Background(), lease.WorkID(), domainsecurity.SHA256Hex([]byte("other-decision")), decisionDigest,
	); !errors.Is(err, ErrGrantAuthority) {
		t.Fatalf("wrong report decision acquired settlement authority: %v", err)
	}
	if _, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(
		context.Background(), lease.WorkID(), decisionID, decisionDigest,
	); !errors.Is(err, ErrWorkClosed) {
		t.Fatalf("pre-disposition settlement authority survived stage closure: %v", err)
	}
}

func TestReportStageDispositionCannotRaceAheadOfSettlementReservation(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("settlement-reservation")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	decisionID := domainsecurity.SHA256Hex([]byte("reserved-report-decision"))
	decisionDigest := domainsecurity.SHA256Hex([]byte("reserved-report-decision-record"))
	result := fixture.addResult(grant, false, "private report result", fixture.now.Add(2*time.Second))
	result["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(
		domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
	)
	result["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(
		domaintoolresult.NewHostReportAdmissionV1(decisionID, decisionDigest),
	)
	reservationEntered := make(chan struct{})
	releaseReservation := make(chan struct{})
	settlementDone := make(chan error, 1)
	go func() {
		settlementDone <- fixture.service.WithTrustedSettledReportStageForDecisionV1(
			context.Background(), lease.WorkID(), decisionID, decisionDigest,
			func(TrustedSettledReportStageV1) error {
				close(reservationEntered)
				<-releaseReservation
				return nil
			},
		)
	}()
	<-reservationEntered
	closeDone := make(chan error, 1)
	go func() {
		_, closeErr := fixture.service.CloseReportStageLease(
			context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second),
		)
		closeDone <- closeErr
	}()
	select {
	case closeErr := <-closeDone:
		t.Fatalf("terminal disposition raced ahead of settlement reservation: %v", closeErr)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseReservation)
	if err := <-settlementDone; err != nil {
		t.Fatalf("settlement reservation failed: %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("completed disposition did not follow settlement reservation: %v", err)
	}
}

func TestResolveTrustedSettledReportStageRejectsGenericSuccessfulResult(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("generic-stage")),
		IssuedAt: fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "model says report completed", fixture.now.Add(2*time.Second))
	if _, err := fixture.service.ResolveTrustedSettledReportStageForDecisionV1(
		context.Background(), lease.WorkID(), domainsecurity.SHA256Hex([]byte("report-decision")),
		domainsecurity.SHA256Hex([]byte("report-decision-record")),
	); !errors.Is(err, ErrGrantAuthority) {
		t.Fatalf("generic successful result acquired report settlement authority: %v", err)
	}
}

func TestResolveTrustedCompletedReportStageRejectsMultiGrantReceiptBeforeDisposition(t *testing.T) {
	fixture := newServiceFixture(t)
	first := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	second := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now.Add(time.Second))
	receipt, err := fixture.service.Issue(context.Background(), fixture.issueInput(
		domainpendingwork.KindReportStage,
		[]GrantReference{{GrantID: first.GrantID}, {GrantID: second.GrantID}},
		"multi-grant-report-stage",
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.GrantMembers) != 2 {
		t.Fatalf("test fixture did not construct a signed multi-grant report stage: %#v", receipt.GrantMembers)
	}
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), receipt.WorkID); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("multi-grant report stage crossed the keyed trusted authority: %v", err)
	}
}

func TestReportStageReceiptNeverPersistsRawStageData(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant),
		StageInputHash:  domainsecurity.SHA256Hex([]byte("account=6222020202020202020;PRIVATE_REASONING_SENTINEL")),
		IssuedAt:        fixture.now.Add(time.Second),
	}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.ReadReceipt(context.Background(), lease.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "staged bytes retained privately", fixture.now.Add(2*time.Second))
	if _, err := fixture.service.CloseReportStageLease(
		context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	disposition, err := fixture.store.ReadDisposition(context.Background(), lease.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(struct {
		Receipt     domainpendingwork.PendingWorkReceiptV1     `json:"receipt"`
		Disposition domainpendingwork.PendingWorkDispositionV1 `json:"disposition"`
	}{receipt, disposition})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"6222020202020202020", "PRIVATE_REASONING_SENTINEL"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("private report stage receipt leaked raw semantic data %q: %s", forbidden, body)
		}
	}
}

func TestReportStageFailureSettlementCannotBecomeCompleted(t *testing.T) {
	for _, test := range []struct {
		name       string
		isError    bool
		itemStatus string
	}{
		{name: "is-error", isError: true, itemStatus: "completed"},
		{name: "failed-status", itemStatus: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
			request := ReportStageRequest{
				PendingToolCall: fixture.pendingCall(grant),
				StageInputHash:  domainsecurity.SHA256Hex([]byte("stage-input")),
				IssuedAt:        fixture.now.Add(time.Second),
			}
			lease, err := fixture.service.BeginReportStage(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			result := fixture.addResult(grant, test.isError, "stage failed", fixture.now.Add(2*time.Second))
			result["status"] = test.itemStatus
			if _, err := fixture.service.CloseReportStageLease(
				context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second),
			); err == nil {
				t.Fatal("failed report stage settlement was upgraded to completed")
			}
			disposition, err := fixture.service.CloseReportStageLease(
				context.Background(), lease, request, domainpendingwork.StatusFailed, fixture.now.Add(4*time.Second),
			)
			if err != nil || disposition.Status != domainpendingwork.StatusFailed || disposition.ReasonCode != "report_stage_failed" {
				t.Fatalf("failed report stage did not receive a fixed downgrade: disposition=%#v err=%v", disposition, err)
			}
		})
	}
}

func TestReportStageRequiresDeterministicIssueTime(t *testing.T) {
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	_, err := fixture.service.BeginReportStage(context.Background(), ReportStageRequest{
		PendingToolCall: fixture.pendingCall(grant),
		StageInputHash:  domainsecurity.SHA256Hex([]byte("stage-input")),
	})
	if !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("report stage without a deterministic issue time was accepted: %v", err)
	}
}
