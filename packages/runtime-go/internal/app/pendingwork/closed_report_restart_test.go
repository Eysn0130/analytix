package pendingwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type closedReportRestartThreadsV1 struct {
	*memoryPendingWorkThreads
	appends int
}

func (store *closedReportRestartThreadsV1) AppendItemToTurn(threadID, turnID string, item map[string]any) error {
	thread, err := store.GetThread(threadID)
	if err != nil {
		return err
	}
	turn, found := appmodel.TurnByID(thread, turnID)
	if !found {
		return errors.New("closed report fixture turn missing")
	}
	turn["items"] = append(turn["items"].([]any), clonePendingWorkRecordV1(item))
	store.appends++
	return nil
}

func TestClosedReportRestartRetainsRealFailureBeforeResult(t *testing.T) {
	for _, status := range []string{domainpending.StatusFailed, domainpending.StatusCancelled} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			fixture := newServiceFixture(t)
			lease, request := beginReportRestartFixture(t, fixture)
			disposition, err := fixture.service.CloseReportStageLease(ctx, lease, request, status, fixture.now.Add(2*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := fixture.service.TrustedInventoryV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.service.BindClosedReportRestartV1(ctx, inventory, []string{lease.WorkID()}); err != nil {
				t.Fatal(err)
			}
			if fixture.service.OwnsRestartTurnV1(fixture.securityContext.ThreadID, fixture.securityContext.TurnID) {
				t.Fatal("closed report binding became a thread hold")
			}
			if _, err := fixture.service.BeginReportStage(ctx, request); !errors.Is(err, ErrWorkClosed) {
				t.Fatalf("closed WorkID reopened: %v", err)
			}
			store := &closedReportRestartThreadsV1{memoryPendingWorkThreads: fixture.threads}
			for restart := 0; restart < 2; restart++ {
				if closed, err := fixture.service.CloseAllOpenOnRestart(ctx, fixture.now.Add(3*time.Second)); err != nil || len(closed) != 0 {
					t.Fatalf("closed report acquired another disposition: %v", err)
				}
				fresh, err := fixture.service.TrustedInventoryV1(ctx)
				if err != nil || !reflect.DeepEqual(fresh, inventory) {
					t.Fatalf("closed signed pending changed: %v", err)
				}
				unknown, err := RestartOutcomeUnknownGrantsV1(fresh)
				if err != nil || len(unknown) != 0 {
					t.Fatal("closed failure was reclassified UNKNOWN")
				}
				outcomes := map[string]executiongrantapp.RestartGrantOutcomesV1{}
				if err := fixture.service.AddClosedReportRestartOutcomesV1(ctx, fresh, outcomes); err != nil {
					t.Fatal(err)
				}
				turnKey := fixture.securityContext.ThreadID + "\x00" + fixture.securityContext.TurnID
				count, err := executiongrantapp.ReconcileOpenTurnGrantsOnRestart(store, fixture.securityContext.ThreadID, fixture.securityContext.TurnID, fixture.now.Add(4*time.Second), outcomes[turnKey])
				if err != nil || count != 1-restart || store.appends != 1 {
					t.Fatalf("closed failure reconciliation count=%d appends=%d err=%v", count, store.appends, err)
				}
			}
			thread, _ := store.GetThread(fixture.securityContext.ThreadID)
			turn, _ := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
			items := turn["items"].([]any)
			if !executiongrantapp.ClosedReportRestartResultMatchesV1(items[len(items)-1].(map[string]any), inventory.Receipts[0], disposition, true) {
				t.Fatal("closed failure did not produce the exact failure-only result")
			}
			for _, fault := range []string{"private-content", "wrong-role", "successful-projection", "before-disposition"} {
				result := clonePendingWorkRecordV1(items[len(items)-1].(map[string]any))
				switch fault {
				case "private-content":
					result["content"] = "synthetic-private-body"
				case "wrong-role":
					result["role"] = "assistant"
				case "successful-projection":
					result["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"))
				case "before-disposition":
					result["createdAt"], result["finishedAt"] = fixture.now.Format(time.RFC3339Nano), fixture.now.Format(time.RFC3339Nano)
				}
				if executiongrantapp.ClosedReportRestartResultMatchesV1(result, inventory.Receipts[0], disposition, true) {
					t.Fatalf("non-producer restart result accepted: %s", fault)
				}
				if fault == "successful-projection" && executiongrantapp.ClosedReportRestartResultMatchesV1(result, inventory.Receipts[0], disposition, false) {
					t.Fatal("inconsistent successful projection accepted as original failure")
				}
			}
		})
	}
}

func TestClosedReportRestartSelectionAndFreshInventoryFailClosed(t *testing.T) {
	for _, fault := range []string{"not-selected", "duplicate", "unknown", "missing-fresh", "changed-fresh", "conflict"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture := newServiceFixture(t)
			lease, request := beginReportRestartFixture(t, fixture)
			status := domainpending.StatusFailed
			if fault == "unknown" {
				if _, err := fixture.service.CloseAllOpenOnRestart(ctx, fixture.now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
			} else if _, err := fixture.service.CloseReportStageLease(ctx, lease, request, status, fixture.now.Add(2*time.Second)); err != nil {
				t.Fatal(err)
			}
			inventory, err := fixture.service.TrustedInventoryV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{lease.WorkID()}
			if fault == "not-selected" {
				ids = nil
			}
			if fault == "duplicate" {
				ids = append(ids, ids[0])
			}
			err = fixture.service.BindClosedReportRestartV1(ctx, inventory, ids)
			if fault == "duplicate" || fault == "unknown" {
				if err == nil {
					t.Fatal("invalid closed authority bound")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fault == "missing-fresh" {
				inventory.Receipts = nil
			}
			if fault == "changed-fresh" {
				inventory.Receipts[0].GrantMembers[0].GrantID = "changed"
			}
			outcomes := map[string]executiongrantapp.RestartGrantOutcomesV1{}
			if fault == "conflict" {
				key := request.PendingToolCall.ThreadID + "\x00" + request.PendingToolCall.TurnID
				outcomes[key] = executiongrantapp.RestartGrantOutcomesV1{request.PendingToolCall.ExecutionGrant.GrantID: {}}
			}
			err = fixture.service.AddClosedReportRestartOutcomesV1(ctx, inventory, outcomes)
			if fault == "not-selected" {
				if err != nil || len(outcomes) != 0 {
					t.Fatal("unselected closed work acquired recovery authority")
				}
			} else if err == nil {
				t.Fatal("changed or conflicting closed work acquired recovery authority")
			}
		})
	}
}
