package pendingwork

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

func childFloorInventoryFixtureV1(t *testing.T, closed bool) (TrustedInventoryV1, domainjob.Record) {
	t.Helper()
	fixture, request := childProducerRequestFixture(t, "parallel_tasks")
	targets := childProducerTargetsForTest(2)
	targets[1].JobID, targets[1].ChildThreadID, targets[1].ChildTurnID = "job-900", "thr_durable_fork_81", "turn_900"
	plan, err := NewChildProducerPlanV1(request.Pending, request.SemanticIdentity, targets, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	request.ChildProducer = &plan
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if closed {
		if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, request.IssuedAt.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		fixture.addResult(request.Pending.ExecutionGrant, false, "synthetic settlement", request.IssuedAt.Add(2*time.Second))
		if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(context.Background(), lease, request, request.IssuedAt.Add(3*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := fixture.service.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainjob.NewSecurityBinding(request.Pending.SecurityContext, request.Pending.ExecutionGrant, request.Pending.Call.ID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{ID: targets[0].JobID, Kind: "subagent", Status: "completed", ParentThreadID: binding.ParentThreadID, ParentTurnID: binding.ParentTurnID,
		ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: targets[0].ChildThreadID, ChildTurnID: targets[0].ChildTurnID}
	return inventory, record
}

func TestChildIdentityFloorsIncludeClosedExpiredMissingJobsAndActualRecords(t *testing.T) {
	for _, closed := range []bool{false, true} {
		inventory, child := childFloorInventoryFixtureV1(t, closed)
		before := clonePendingWorkReceipt(inventory.Receipts[0])
		floors, err := DeriveChildIdentityFloorsV1(inventory, nil, nil)
		if err != nil || floors.JobSequence != 900 || floors.ThreadSequence != 21 || floors.ForkSequence != 81 || floors.TurnSequence != 900 {
			t.Fatalf("missing jobs lost signed allocator floors: %v", err)
		}
		jobs := []domainjob.Record{child, {ID: "job-888", Kind: "background-shell", Status: "failed", ChildThreadID: "thr_durable_resume_66", AutoContinueTurnID: "turn_1301"}}
		threads := []map[string]any{{"id": "thr_durable_301", "turns": []any{map[string]any{"id": "turn_1201"}}}, {"id": child.ChildThreadID, "turns": []any{map[string]any{"id": child.ChildTurnID}}}}
		floors, err = DeriveChildIdentityFloorsV1(inventory, jobs, threads)
		want := domainpendingwork.ChildIdentityFloorsV1{JobSequence: 900, ThreadSequence: 301, ForkSequence: 81, ResumeSequence: 66, TurnSequence: 1301}
		if err != nil || floors != want {
			t.Fatalf("actual primary/job denominator was omitted: got=%+v err=%v", floors, err)
		}
		if !reflect.DeepEqual(before, inventory.Receipts[0]) || len(jobs) != 2 {
			t.Fatal("floor derivation rewrote or completed missing child inventory")
		}
	}
}

func TestChildIdentityFloorsRejectIncompleteOrCrossBoundCoreInventory(t *testing.T) {
	for _, mode := range []string{"legacy_missing_producer", "legacy_kind_alias", "duplicate_signed_job", "wrong_job_turn", "wrong_job_binding", "wrong_ordinal", "wrong_primary_thread", "duplicate_primary_turn", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			inventory, record := childFloorInventoryFixtureV1(t, false)
			threads := []map[string]any{}
			switch mode {
			case "legacy_missing_producer":
				inventory.Receipts[0].ChildProducer = nil
			case "legacy_kind_alias":
				inventory.Receipts[0].ChildProducer = nil
				record.Kind = " subagent "
			case "duplicate_signed_job":
				inventory.Receipts = append(inventory.Receipts, clonePendingWorkReceipt(inventory.Receipts[0]))
			case "wrong_job_turn":
				record.ChildTurnID = "turn_999"
			case "wrong_job_binding":
				record.SecurityBinding.ParentContextEpoch++
			case "wrong_ordinal":
				record.ParallelIndex = 2
			case "wrong_primary_thread":
				threads = []map[string]any{{"id": "thr_durable_999", "turns": []any{map[string]any{"id": record.ChildTurnID, "securityContext": map[string]any{}}}}}
			case "duplicate_primary_turn":
				threads = []map[string]any{{"id": record.ChildThreadID, "turns": []any{map[string]any{"id": record.ChildTurnID}, map[string]any{"id": record.ChildTurnID}}}}
			case "overflow":
				record.AutoContinueTurnID = "turn_" + strings.Repeat("9", 40)
			}
			floors, err := DeriveChildIdentityFloorsV1(inventory, []domainjob.Record{record}, threads)
			if err == nil || floors != (domainpendingwork.ChildIdentityFloorsV1{}) {
				t.Fatal("invalid Core inventory returned usable partial allocator floors")
			}
		})
	}
}

func TestChildIdentityFloorsIncludeNonChildParentTurn(t *testing.T) {
	floors, err := DeriveChildIdentityFloorsV1(TrustedInventoryV1{}, []domainjob.Record{{ID: "job-3", Kind: "background-shell", ParentThreadID: "thr_durable_2", ParentTurnID: "turn_1500"}}, nil)
	if err != nil || floors.TurnSequence != 1500 {
		t.Fatalf("actual parent turn was omitted: %+v %v", floors, err)
	}
}

func TestChildIdentityFloorsAllowStrippedForkResumeHistory(t *testing.T) {
	inventory, record := childFloorInventoryFixtureV1(t, true)
	const target = "thr_durable_resume_700"
	// CloneTurnForThread preserves turn.id and strips securityContext from
	// fork/resume history. Use its resulting public shape without an app cycle.
	cloned := map[string]any{"id": record.ChildTurnID, "threadId": target, "status": "completed", "items": []any{}}
	floors, err := DeriveChildIdentityFloorsV1(inventory, []domainjob.Record{record}, []map[string]any{{"id": target, "turns": []any{cloned}}})
	if err != nil || floors.ResumeSequence != 700 || floors.TurnSequence != 900 {
		t.Fatalf("historical copy was mistaken for child execution authority: %+v %v", floors, err)
	}
}

func TestChildIdentityFloorsRejectLegacyTurnCounterOverflow(t *testing.T) {
	floors, err := DeriveChildIdentityFloorsV1(TrustedInventoryV1{}, nil, []map[string]any{{"id": "thr_durable_1", "turns": []any{map[string]any{"id": "turn_d0242_" + strings.Repeat("9", 40)}}}})
	if err == nil || floors != (domainpendingwork.ChildIdentityFloorsV1{}) {
		t.Fatal("legacy turn overflow returned usable floors")
	}
}
