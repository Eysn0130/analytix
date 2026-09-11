package pendingwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

func TestCompletedReportSnapshotAuditsHeldHistoryWithoutLiveAuthority(t *testing.T) {
	ctx := context.Background()
	fixture, workID := completedReportSnapshotFixtureV1(t)
	other := reportRestartSiblingFixture(t, fixture, fixture.securityContext.ThreadID, "turn-later-report")
	beginReportRestartFixture(t, other)
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	thread["turns"] = append(thread["turns"].([]any), other.threads.threads[other.securityContext.ThreadID]["turns"].([]any)...)
	thread["securityState"] = mapRecord(other.securityContext)
	reader := reportRestartPrimaryFixture{threads: fixture.threads}
	scope, err := fixture.service.PlanReportRestartPreservationV1(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.PreserveReportRestartScopeV1(ctx, scope); err != nil {
		t.Fatal(err)
	}
	receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	verification := &reportSnapshotVerificationOnlyV1{Authority: fixture.authority}
	prepared, err := PrepareCompletedReportSnapshotV1(ctx, receipts, dispositions, verification, reader)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := prepared.ResolveTrustedCompletedReportStageV1(ctx, workID)
	if err != nil || terminal.ResultItemID == "" || terminal.ResultItemDigest == "" || terminal.Receipt.WorkID != workID {
		t.Fatalf("Original completed history did not resolve: %v", err)
	}
	terminal.Receipt.GrantMembers[0].GrantID = "caller mutation"
	terminal.ResultItem["toolName"] = "caller mutation"
	for i := range receipts {
		receipts[i] = clonePendingWorkReceipt(receipts[i])
		receipts[i].GrantMembers[0].GrantID = "caller input mutation"
	}
	again, err := prepared.ResolveTrustedCompletedReportStageV1(ctx, workID)
	if err != nil || again.Grant.ToolName != ReportStageToolName || again.ResultItem["toolName"] != ReportStageToolName {
		t.Fatalf("caller mutated sealed historical Core: %v", err)
	}
	if _, err := fixture.service.ResolveTrustedCompletedReportStageV1(ctx, workID); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("historical resolution bypassed live hold: %v", err)
	}
	if verification.signs != 0 {
		t.Fatal("historical snapshot attempted signing")
	}
}

func TestCompletedReportSnapshotRejectsIncompleteInventoryAndPrimaryDrift(t *testing.T) {
	for _, fault := range []string{"missing-result", "signature", "orphan-disposition", "primary-drift", "read-error", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture, workID := completedReportSnapshotFixtureV1(t)
			receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
			if err != nil {
				t.Fatal(err)
			}
			physical := errors.New("synthetic strict primary read failed")
			reader := &completedReportPrimaryReaderV1{PrimaryThreadReaderV1: reportRestartPrimaryFixture{threads: fixture.threads}}
			switch fault {
			case "missing-result":
				for _, raw := range fixture.threads.threads[fixture.securityContext.ThreadID]["turns"].([]any) {
					raw.(map[string]any)["items"] = []any{}
				}
			case "signature":
				dispositions[0].AuthoritySignature = "invalid"
			case "orphan-disposition":
				receipts = nil
			case "read-error":
				reader.err = physical
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			prepared, err := PrepareCompletedReportSnapshotV1(ctx, receipts, dispositions, fixture.authority, reader)
			if fault == "primary-drift" {
				if err != nil {
					t.Fatal(err)
				}
				fixture.threads.threads[fixture.securityContext.ThreadID]["title"] = "changed Original primary"
				_, err = prepared.ResolveTrustedCompletedReportStageV1(ctx, workID)
			}
			if err == nil {
				t.Fatal("invalid Original Core produced trusted history")
			}
			if fault == "read-error" && !errors.Is(err, physical) || fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("Original observation lost its cause: %v", err)
			}
		})
	}
}

func TestCompletedReportSnapshotRevalidatesAfterReadAndRetainsCause(t *testing.T) {
	fixture, workID := completedReportSnapshotFixtureV1(t)
	receipts, dispositions, err := fixture.store.SnapshotInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reader := &completedReportPrimaryReaderV1{PrimaryThreadReaderV1: reportRestartPrimaryFixture{threads: fixture.threads}}
	prepared, err := PrepareCompletedReportSnapshotV1(context.Background(), receipts, dispositions, fixture.authority, reader)
	if err != nil {
		t.Fatal(err)
	}
	physical := errors.New("synthetic post-read primary failure")
	reads := 0
	reader.afterRead = func() {
		reads++
		if reads == 1 {
			reader.err = physical
		}
	}
	if _, err := prepared.ResolveTrustedCompletedReportStageV1(context.Background(), workID); !errors.Is(err, physical) {
		t.Fatalf("post-read physical failure was accepted or lost: %v", err)
	}
}

func TestCompletedReportSnapshotRejectsSignedForeignRegistryPrefix(t *testing.T) {
	for _, fault := range []string{"digest", "sequence", "settled-prefix"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture, workID := completedReportSnapshotFixtureV1(t)
			receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
			if err != nil || len(receipts) != 1 || len(dispositions) != 1 {
				t.Fatalf("completed fixture inventory: %v", err)
			}
			receipt, disposition := receipts[0], dispositions[0]
			issued, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
			if err != nil {
				t.Fatal(err)
			}
			expires, err := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
			if err != nil {
				t.Fatal(err)
			}
			disposed, err := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
			if err != nil {
				t.Fatal(err)
			}
			input := domainpendingwork.ReceiptInputV1{
				Kind: receipt.Kind, SecurityContext: fixture.securityContext,
				GrantRegistrySequence: receipt.GrantRegistrySequence, GrantRegistryDigest: receipt.GrantRegistryDigest,
				GrantMembers: receipt.GrantMembers, PayloadHash: receipt.PayloadHash, RouteHash: receipt.RouteHash,
				IssuedAt: issued, ExpiresAt: expires, AuthorityKeyID: fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
			}
			switch fault {
			case "digest":
				input.GrantRegistryDigest = domainsecurity.SHA256Hex([]byte("another signed registry prefix"))
			case "sequence":
				input.GrantRegistrySequence++
			case "settled-prefix":
				registry, err := executiongrantapp.RegistryFromThread(fixture.securityContext.ThreadID, fixture.threads.threads[fixture.securityContext.ThreadID], fixture.securityContext.TurnID)
				if err != nil {
					t.Fatal(err)
				}
				input.GrantRegistrySequence, input.GrantRegistryDigest = registry.Sequence, registry.StateDigest
			}
			sign := func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) }
			receipt, err = domainpendingwork.NewPendingWorkReceiptV1(input, sign)
			if err != nil || receipt.WorkID != workID {
				t.Fatalf("same-work signed counterexample: %v", err)
			}
			disposition, err = domainpendingwork.NewPendingWorkDispositionV1(receipt, disposition.Status, disposition.ReasonCode, disposed, fixture.authority.KeyID(), fixture.authority.PublicKey(), sign)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareCompletedReportSnapshotV1(ctx, []domainpendingwork.PendingWorkReceiptV1{receipt}, []domainpendingwork.PendingWorkDispositionV1{disposition}, fixture.authority, reportRestartPrimaryFixture{threads: fixture.threads})
			if err == nil {
				_, err = prepared.ResolveTrustedCompletedReportStageV1(ctx, workID)
			}
			if err == nil {
				t.Fatal("signed report receipt outside its actual issued active registry prefix acquired completed authority")
			}
		})
	}
}

func completedReportSnapshotFixtureV1(t *testing.T) (*serviceFixture, string) {
	t.Helper()
	fixture := newServiceFixture(t)
	grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
	request := ReportStageRequest{PendingToolCall: fixture.pendingCall(grant), StageInputHash: domainsecurity.SHA256Hex([]byte("Original complete report")), IssuedAt: fixture.now.Add(time.Second)}
	lease, err := fixture.service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(grant, false, "completed Original report", fixture.now.Add(2*time.Second))
	if _, err := fixture.service.CloseReportStageLease(context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	want, err := fixture.service.ResolveTrustedCompletedReportStageV1(context.Background(), lease.WorkID())
	if err != nil || reflect.DeepEqual(want, TrustedCompletedReportStageV1{}) {
		t.Fatalf("actual Core fixture is incomplete: %v", err)
	}
	return fixture, lease.WorkID()
}

type completedReportPrimaryReaderV1 struct {
	recoveryport.PrimaryThreadReaderV1
	err       error
	afterRead func()
}

func (reader *completedReportPrimaryReaderV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if reader.err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, reader.err
	}
	snapshot, err := reader.PrimaryThreadReaderV1.ReadPrimaryThreadSnapshotV1(ctx, id)
	if reader.afterRead != nil {
		reader.afterRead()
	}
	return snapshot, err
}
