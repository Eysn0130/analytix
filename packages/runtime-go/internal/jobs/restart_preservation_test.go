package jobs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func preserveRestartJobsForTest(t *testing.T, manager *Manager, threads []string, records []Record) {
	t.Helper()
	if err := manager.PreserveRestartScopeV1(context.Background(), threads, records); err != nil {
		t.Fatal(err)
	}
}

func TestRestartPreservationCannotInstallAfterEffectAdmission(t *testing.T) {
	for _, operation := range []string{"recovery_query", "validate_start", "claim_start", "runtime_lock", "cleanup", "write"} {
		t.Run(operation, func(t *testing.T) {
			initial, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record := startClaimFixture(t, initial, "running")
			manager, err := NewManager(initial.root)
			if err != nil {
				t.Fatal(err)
			}
			record, err = manager.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "recovery_query":
				var held bool
				held, err = manager.RestartPreservesChildRunV1(record)
				if held {
					t.Fatal("uninstalled scope claimed a hold")
				}
			case "validate_start":
				_, err = manager.ValidateChildRunStart(record)
			case "claim_start":
				record, err = manager.ClaimChildRunStart(record)
			case "runtime_lock":
				var unlock func()
				unlock, err = manager.LockChildRun(record.ID)
				if err == nil {
					defer unlock()
				}
			case "cleanup":
				_, err = manager.CleanupStaleRunningRecords()
				if err == nil {
					record, err = manager.LoadChildRun(record.ID)
				}
			case "write":
				record, err = manager.UpdateChildRun(record.ID, UpdateRequest{Output: "ordinary progress"})
			}
			if err != nil {
				t.Fatalf("ordinary admission failed: %v", err)
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.PreserveRestartScopeV1(context.Background(), []string{record.ParentThreadID}, []Record{record}); err == nil {
				t.Error("scope installed after a consumer entered its effect window")
			}
			if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
				t.Fatalf("late installation wrote inventory: %v", err)
			}
		})
	}
}

func TestRestartPreservedJobRejectsExistingStartLeaseAndLock(t *testing.T) {
	for _, operation := range []string{"validate_start", "claim_start", "runtime_lock"} {
		t.Run(operation, func(t *testing.T) {
			initial, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record := startClaimFixture(t, initial, "running")
			manager, err := NewManager(initial.root)
			if err != nil {
				t.Fatal(err)
			}
			record, err = manager.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			preserveRestartJobsForTest(t, manager, []string{record.ParentThreadID}, []Record{record})
			if held, err := manager.RestartPreservesChildRunV1(record); err != nil || !held {
				t.Fatalf("exact public record observation was rejected: held=%t err=%v", held, err)
			}
			changed := record
			changed.SteerState.CanAcceptSteer = !changed.SteerState.CanAcceptSteer
			if held, err := manager.RestartPreservesChildRunV1(changed); err == nil || held {
				t.Fatal("changed public observation was normalized into an accepted hold")
			}
			switch operation {
			case "validate_start":
				_, err = manager.ValidateChildRunStart(record)
			case "claim_start":
				_, err = manager.ClaimChildRunStart(record)
			case "runtime_lock":
				var unlock func()
				unlock, err = manager.LockChildRun(record.ID)
				if err == nil {
					defer unlock()
				}
			}
			if !errors.Is(err, ErrRestartPreserved) {
				t.Errorf("held existing lease admitted an effect: %v", err)
			}
			if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
				t.Fatalf("held admission changed original inventory: %v", err)
			}
		})
	}
}

func TestRestartPreservedJobsRejectWritesAndKeepIndependentCleanup(t *testing.T) {
	for _, operation := range []string{"cleanup", "update", "new_related_job"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			manager, err := NewManager(root)
			if err != nil {
				t.Fatal(err)
			}
			held, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-held", ParentThreadID: "thread-held", Kind: "subagent", Status: "running", Background: true})
			if err != nil {
				t.Fatal(err)
			}
			ordinary, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-ordinary", ParentThreadID: "thread-ordinary", Kind: "subagent", Status: "running", Background: true})
			if err != nil {
				t.Fatal(err)
			}
			manager, err = NewManager(root)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, held.ID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			preserveRestartJobsForTest(t, manager, []string{held.ParentThreadID}, []Record{held})
			switch operation {
			case "cleanup":
				cleaned, err := manager.CleanupStaleRunningRecords()
				if err != nil || len(cleaned) != 1 || cleaned[0].ID != ordinary.ID || cleaned[0].Status != "interrupted" {
					t.Errorf("cleanup did not preserve exact scope: count=%d err=%v", len(cleaned), err)
				}
			case "update":
				if _, err := manager.UpdateChildRun(held.ID, UpdateRequest{Status: "interrupted"}); err == nil {
					t.Error("held job accepted lifecycle rewrite")
				}
			case "new_related_job":
				if _, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-held", ParentThreadID: held.ParentThreadID, Kind: "subagent", Status: "running"}); err == nil {
					t.Error("held parent acquired a new child job")
				}
			}
			after, err := os.ReadFile(filepath.Join(root, held.ID+".json"))
			if err != nil || !bytes.Equal(before, after) {
				t.Error("held job original bytes changed")
			}
		})
	}
}

func TestRestartPreservedJobsRejectStaleIncompleteAndReplacedScopes(t *testing.T) {
	for _, fault := range []string{"missing_record", "changed_record", "cancelled", "duplicate_thread"} {
		t.Run(fault, func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-held", ParentThreadID: "thread-held", Kind: "subagent", Status: "running"})
			if err != nil {
				t.Fatal(err)
			}
			threads, records := []string{record.ParentThreadID}, []Record{record}
			manager, err = NewManager(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch fault {
			case "missing_record":
				records = nil
			case "changed_record":
				records[0].Status = "interrupted"
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "duplicate_thread":
				threads = append(threads, threads[0])
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.PreserveRestartScopeV1(ctx, threads, records); err == nil || manager.restartPreserved != nil {
				t.Fatal("invalid scope installed a partial preservation")
			}
			if err := ValidateChildRunInventoryV1(manager.root, before); err != nil {
				t.Fatalf("invalid scope changed original inventory: %v", err)
			}
		})
	}
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-held", ParentThreadID: "thread-held", Kind: "subagent", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err = NewManager(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	preserveRestartJobsForTest(t, manager, []string{record.ParentThreadID}, []Record{record})
	if err := manager.PreserveRestartScopeV1(context.Background(), nil, nil); err == nil {
		t.Fatal("empty replacement released installed hold")
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "interrupted"}); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("hold lost after failed replacement: %v", err)
	}
	path := filepath.Join(manager.root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if held, err := manager.RestartPreservesChildRunV1(record); err == nil || held {
		t.Fatal("changed original job bytes were hidden behind a successful hold")
	}
	if _, err := manager.CleanupStaleRunningRecords(); err == nil {
		t.Fatal("cleanup ignored changed preserved authority")
	}
}
