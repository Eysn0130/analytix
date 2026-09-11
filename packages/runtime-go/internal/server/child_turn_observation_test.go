package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRuntimeChildFirstTurnObservationRequiresStrictPrimaryAndSignedAssociation(t *testing.T) {
	for _, mode := range []string{"reserved", "committed", "missing_primary", "malformed_primary", "duplicate_turns", "missing_frozen", "wrong_reserved_turn", "missing_inventory", "empty_inventory"} {
		t.Run(mode, func(t *testing.T) {
			h, pending, ctx, slot, providerClient, _ := runtimePreparedChildExecutionFixtureV1(t, true)
			binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
			if err != nil {
				t.Fatal(err)
			}
			record := domainjob.Record{ID: slot.target.JobID, ParentThreadID: pending.ThreadID, ParentTurnID: pending.TurnID,
				ChildThreadID: slot.target.ChildThreadID, ChildTurnID: slot.target.ChildTurnID, SecurityBinding: binding}
			primary := map[string]any{"id": record.ChildThreadID, "turns": []any{}}
			if mode == "committed" {
				frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
					ThreadID: record.ChildThreadID, TurnID: record.ChildTurnID, WorkspaceRealPath: pending.SecurityContext.WorkspaceRealPath,
					ContextEpoch: pending.SecurityContext.ContextEpoch, IssuedAt: time.Now().UTC(), TenantID: pending.SecurityContext.TenantID, UserID: pending.SecurityContext.UserID,
				})
				primary["turns"] = []any{map[string]any{"id": record.ChildTurnID, "threadId": record.ChildThreadID, "securityContext": turnsecurityapp.PublicRecord(frozen)}}
			}
			if mode == "missing_frozen" {
				primary["turns"] = []any{map[string]any{"id": record.ChildTurnID}}
			}
			if mode == "duplicate_turns" {
				primary["turns"] = []any{map[string]any{"id": "turn_900"}, map[string]any{"id": "turn_900"}}
			}
			body, err := json.Marshal(primary)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "malformed_primary" {
				body = []byte(`{"id":`)
			}
			if mode != "missing_primary" {
				if err := os.MkdirAll(filepath.Dir(h.store.threadPath(record.ChildThreadID)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(h.store.threadPath(record.ChildThreadID), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "wrong_reserved_turn" {
				record.ChildTurnID = "turn_999"
			}
			if mode == "missing_inventory" {
				h.pendingWork = nil
			}
			if mode == "empty_inventory" {
				empty, _ := runtimeChildProducerPendingFixtureV1(t, "task", json.RawMessage(`{"prompt":"synthetic"}`))
				h.pendingWork = empty.pendingWork
			}
			before := runtimeRestoreFileDigestsV1(t, h.store.root)
			frozen, committed, err := h.observeRuntimeChildFirstTurnV1(ctx, record)
			switch mode {
			case "reserved":
				if err != nil || committed || frozen.ContextDigest != "" {
					t.Fatalf("signed absent reservation was not observed: %v", err)
				}
			case "committed":
				if err != nil || !committed || frozen.TurnID != record.ChildTurnID {
					t.Fatalf("exact committed first turn was not observed: %v", err)
				}
			default:
				if err == nil || committed || frozen.ContextDigest != "" {
					t.Fatal("unavailable primary or producer became first-turn authority")
				}
			}
			if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, h.store.root)) || len(providerClient.Requests()) != 0 {
				t.Fatal("first-turn observation recovered or wrote primary state")
			}
		})
	}
}

func TestRuntimeReservedChildWithoutLiveRegistrationCannotQueueSteer(t *testing.T) {
	h, pending, ctx, slot, _, _ := runtimePreparedChildExecutionFixtureV1(t, true)
	preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, slot.request)
	if err != nil {
		t.Fatal(err)
	}
	defer preparation.ReleaseSourceLock()
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := subagentapp.StartPreparedTaskRun(subagentapp.PreparedTaskRunStartInput{Preparation: preparation, Pending: pending, Security: binding, Settings: h.subagents, StartChildRun: func(start domainjob.StartRequest) (domainjob.Record, error) {
		return h.startPreparedReservedChildV1(ctx, pending, slot, start)
	}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"id": record.ChildThreadID, "turns": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(h.store.threadPath(record.ChildThreadID)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.store.threadPath(record.ChildThreadID), body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := runtimeRestoreFileDigestsV1(t, h.store.root)
	authorization, blocker, err := h.beginRuntimeTaskJobSteerAuthority(context.Background(), record, domainjob.SteerMessage{Text: "synthetic pending guidance"})
	if authorization.Release != nil {
		authorization.Release()
	}
	if err == nil || blocker != "" || authorization.PendingUntilChildTurn {
		t.Fatal("signed inventory alone recreated a live start barrier")
	}
	current, loadErr := h.jobs.LoadChildRun(record.ID)
	if loadErr != nil || !reflect.DeepEqual(current, record) || !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, h.store.root)) {
		t.Fatal("missing live registration changed durable state")
	}
}
