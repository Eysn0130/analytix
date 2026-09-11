package jobs

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestClaimChildRunStartRenewsExactDurableLeaseAndRejectsReplay(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := startClaimFixture(t, manager, string(domainjob.StatusRunning))
	claimed, err := manager.ClaimChildRunStart(record)
	if err != nil {
		t.Fatalf("claim running child: %v", err)
	}
	if claimed.Status != string(domainjob.StatusRunning) || claimed.LeaseOwner != record.LeaseOwner ||
		!startClaimTime(t, claimed.LastHeartbeatAt).After(startClaimTime(t, record.LastHeartbeatAt)) ||
		!startClaimTime(t, claimed.LeaseExpiresAt).After(startClaimTime(t, record.LeaseExpiresAt)) ||
		!startClaimTime(t, claimed.UpdatedAt).After(startClaimTime(t, record.UpdatedAt)) {
		t.Fatalf("claim did not durably renew exact runtime lease: before=%#v after=%#v", record, claimed)
	}
	latest, err := manager.ClaimChildRunStart(record)
	if err == nil || latest.LeaseExpiresAt != claimed.LeaseExpiresAt || !strings.Contains(err.Error(), "lease was replaced") {
		t.Fatalf("stale claim replay was not rejected: latest=%#v err=%v", latest, err)
	}
	reloaded, err := NewManager(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reloaded.LoadChildRun(record.ID)
	if err != nil || persisted.LeaseExpiresAt != claimed.LeaseExpiresAt || persisted.UpdatedAt != claimed.UpdatedAt {
		t.Fatalf("start claim was not durable: persisted=%#v err=%v", persisted, err)
	}
}

func TestClaimChildRunStartRejectsStoppedPausedTerminalDeadLetterAndReplacedRecords(t *testing.T) {
	for _, status := range []string{
		string(domainjob.StatusKilled),
		string(domainjob.StatusPauseRequested),
		string(domainjob.StatusPaused),
		string(domainjob.StatusResumeRequested),
		string(domainjob.StatusResuming),
		string(domainjob.StatusCompleted),
		string(domainjob.StatusFailed),
		string(domainjob.StatusAborted),
		string(domainjob.StatusInterrupted),
	} {
		t.Run(status, func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record := startClaimFixture(t, manager, status)
			if latest, err := manager.ClaimChildRunStart(record); err == nil || latest.Status != status {
				t.Fatalf("non-startable status was claimed: latest=%#v err=%v", latest, err)
			}
		})
	}

	t.Run("dead_letter", func(t *testing.T) {
		manager, err := NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		record := startClaimFixture(t, manager, string(domainjob.StatusRunning))
		dead, err := manager.UpdateChildRun(record.ID, UpdateRequest{
			CompletionDeliveryStatus: "dead_letter",
			CompletionDeliveryReason: "parent authority missing",
		})
		if err != nil {
			t.Fatal(err)
		}
		if latest, err := manager.ClaimChildRunStart(dead); err == nil || latest.DeadLetterReason == "" {
			t.Fatalf("dead-lettered child was claimed: latest=%#v err=%v", latest, err)
		}
	})

	t.Run("replaced", func(t *testing.T) {
		manager, err := NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		stale := startClaimFixture(t, manager, string(domainjob.StatusRunning))
		replaced, err := manager.UpdateChildRun(stale.ID, UpdateRequest{Output: "new owner state"})
		if err != nil {
			t.Fatal(err)
		}
		latest, err := manager.ClaimChildRunStart(stale)
		if err == nil || latest.UpdatedAt != replaced.UpdatedAt || !strings.Contains(err.Error(), "lease was replaced") {
			t.Fatalf("replaced child record was claimed: latest=%#v err=%v", latest, err)
		}
	})
}

func TestValidateChildRunStartIsReadOnlyAndRejectsChangedAuthority(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := startClaimFixture(t, manager, string(domainjob.StatusRunning))
	validated, err := manager.ValidateChildRunStart(record)
	if err != nil {
		t.Fatalf("validate exact child start: %v", err)
	}
	if validated.UpdatedAt != record.UpdatedAt || validated.LastHeartbeatAt != record.LastHeartbeatAt || validated.LeaseExpiresAt != record.LeaseExpiresAt {
		t.Fatalf("read-only start validation mutated the runtime lease: before=%#v after=%#v", record, validated)
	}
	changed, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: string(domainjob.StatusKilled), Error: "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	if latest, err := manager.ValidateChildRunStart(record); err == nil || latest.Status != changed.Status {
		t.Fatalf("changed child authority passed frozen validation: latest=%#v err=%v", latest, err)
	}
}

func TestChildRunStartClaimRejectsChangedReservedTurn(t *testing.T) {
	for _, renew := range []bool{false, true} {
		t.Run(fmt.Sprintf("renew=%t", renew), func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record := startClaimFixture(t, manager, string(domainjob.StatusRunning))
			record, err = manager.UpdateChildRun(record.ID, UpdateRequest{ChildTurnID: "turn_31"})
			if err != nil {
				t.Fatal(err)
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			wrong := cloneRecord(record)
			wrong.ChildTurnID = "turn_99"
			if renew {
				_, err = manager.ClaimChildRunStart(wrong)
			} else {
				_, err = manager.ValidateChildRunStart(wrong)
			}
			if err == nil {
				t.Error("different reserved child turn retained start authority")
			}
			after, readErr := BuildChildRunInventoryV1(manager.root)
			if readErr != nil || !reflect.DeepEqual(before, after) {
				t.Error("wrong child turn crossed the durable start-claim writer")
			}
		})
	}
}

func TestChildRunStartClaimBindsDelegatedToolManifest(t *testing.T) {
	schemaHash := domainsecurity.SHA256Hex([]byte("delegated-schema"))
	manifest, err := domainjob.NewDelegatedToolManifestV1(
		[]string{"read"}, schemaHash, domainsecurity.SHA256Hex([]byte("delegated-mcp")),
	)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{ToolScope: []string{"read"}, ToolSchemaHash: schemaHash, DelegatedToolManifest: manifest}
	expected := cloneRecord(record)
	if !sameChildRunStartIdentity(record, expected) {
		t.Fatal("exact delegated manifest was rejected")
	}
	expected.DelegatedToolManifest.MCPAuthorityHash = domainsecurity.SHA256Hex([]byte("replaced-mcp"))
	if sameChildRunStartIdentity(record, expected) {
		t.Fatal("changed delegated MCP authority survived the durable start CAS")
	}
	expected = cloneRecord(record)
	expected.ToolScope = []string{"write"}
	if sameChildRunStartIdentity(record, expected) {
		t.Fatal("changed delegated tool scope survived the durable start CAS")
	}
}

func startClaimFixture(t *testing.T, manager *Manager, status string) Record {
	t.Helper()
	startStatus := status
	switch status {
	case string(domainjob.StatusPauseRequested), string(domainjob.StatusPaused),
		string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming):
		startStatus = string(domainjob.StatusRunning)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:     "goal_start_claim",
		ParentThreadID:   "thread_start_claim",
		ParentTurnID:     "turn_start_claim",
		ParentToolCallID: "call_start_claim",
		ChildThreadID:    "thread_child",
		Kind:             "subagent",
		Status:           startStatus,
		Background:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if startStatus == status {
		return record
	}
	now := time.Now().UTC()
	record, pauseRequest, err := manager.AddPauseRequest(record.ID, domainjob.PauseRequest{
		ID:             "pause_" + record.ID,
		ParentThreadID: record.ParentThreadID,
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Status:         "requested",
		RequestedAt:    now.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("request pause: %v", err)
	}
	if status == string(domainjob.StatusPauseRequested) {
		return record
	}
	pausedAt := now.Add(time.Second)
	record, pauseRequest, err = manager.MarkPauseRequestPaused(record.ID, pauseRequest.ID, pausedAt.Format(time.RFC3339Nano), domainjob.ResumeToken{
		ResumeToken:    "resume_" + record.ID,
		IssuedAt:       pausedAt.Format(time.RFC3339Nano),
		ExpiresAt:      pausedAt.Add(time.Hour).Format(time.RFC3339Nano),
		ChildRunID:     record.ID,
		ParentThreadID: record.ParentThreadID,
	})
	if err != nil {
		t.Fatalf("settle pause: %v", err)
	}
	if status == string(domainjob.StatusPaused) {
		return record
	}
	record, _, err = manager.MarkPauseResumeRequested(record.ID, pauseRequest.ID)
	if err != nil {
		t.Fatalf("mark resume requested: %v", err)
	}
	if status == string(domainjob.StatusResumeRequested) {
		return record
	}
	record, _, err = manager.MarkPauseRequestResumed(record.ID, pauseRequest.ID, pausedAt.Add(time.Second).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("request resume: %v", err)
	}
	return record
}

func startClaimTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse job timestamp %q: %v", value, err)
	}
	return parsed
}

func pendingSteerStartFixtureV1(t *testing.T) (*Manager, Record) {
	t.Helper()
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := childReservationRequestForTest(t)
	request.Status = "running"
	request.Background = true
	request.ChildThreadID = "thr_durable_21"
	request.ChildTurnID = "turn_31"
	record, err := manager.StartChildRun(request)
	if err != nil {
		t.Fatal(err)
	}
	return manager, record
}

func queuePendingStartSteerV1(t *testing.T, manager *Manager, record Record, id string) Record {
	t.Helper()
	authority, err := domainjob.NewSteerQueueAuthorityV1(record, "", true)
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := manager.QueueSteerMessage(record.ID, authority, domainjob.SteerMessage{ID: id, ParentThreadID: record.ParentThreadID, Text: "synthetic guidance", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true})
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func TestClaimChildRunStartAcceptsOnlySuccessfulPendingSteerAdvances(t *testing.T) {
	manager, original := pendingSteerStartFixtureV1(t)
	middle := queuePendingStartSteerV1(t, manager, original, "steer-first")
	latest := queuePendingStartSteerV1(t, manager, middle, "steer-second")
	claimed, err := manager.ClaimChildRunStart(original)
	if err != nil || len(claimed.Steers) != 2 || claimed.ChildTurnID != original.ChildTurnID {
		t.Fatalf("exact pending queue chain rejected at first-turn claim: %v", err)
	}
	if claimed.UpdatedAt == latest.UpdatedAt {
		t.Fatal("claim did not consume and renew the exact queued record")
	}
	if _, err := manager.ClaimChildRunStart(middle); err == nil {
		t.Fatal("copied pre-claim snapshot replayed consumed queue chain")
	}
}

func TestClaimChildRunStartPendingSteerDoesNotHideOtherWritesOrRestart(t *testing.T) {
	for _, change := range []string{"same_record_write", "status_aba", "reopen", "failed_write_then_queue", "bound_queue"} {
		t.Run(change, func(t *testing.T) {
			manager, original := pendingSteerStartFixtureV1(t)
			latest := queuePendingStartSteerV1(t, manager, original, "steer-first")
			switch change {
			case "failed_write_then_queue":
				invalid := cloneRecord(latest)
				invalid.FailureCode = "not_a_failure_code"
				manager.mu.Lock()
				err := manager.writeRecordNoLock(&invalid)
				manager.mu.Unlock()
				if err == nil {
					t.Fatal("invalid fixture write unexpectedly succeeded")
				}
				latest = queuePendingStartSteerV1(t, manager, latest, "steer-after-failure")
			case "bound_queue":
				authority, err := domainjob.NewSteerQueueAuthorityV1(latest, strings.Repeat("e", 64), false)
				if err != nil {
					t.Fatal(err)
				}
				latest, _, err = manager.QueueSteerMessage(latest.ID, authority, domainjob.SteerMessage{ID: "steer-bound", ParentThreadID: latest.ParentThreadID, Text: "synthetic bound guidance", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true})
				if err != nil {
					t.Fatal(err)
				}
			case "same_record_write":
				manager.mu.Lock()
				err := manager.writeRecordNoLock(&latest)
				manager.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			case "status_aba":
				for _, status := range []string{"queued", "running"} {
					var err error
					latest, err = manager.UpdateChildRun(original.ID, UpdateRequest{Status: status, LastHeartbeatAt: original.LastHeartbeatAt, LeaseExpiresAt: original.LeaseExpiresAt})
					if err != nil {
						t.Fatal(err)
					}
				}
			case "reopen":
				var err error
				manager, err = NewManager(manager.root)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.ClaimChildRunStart(original); err == nil {
				t.Fatal("unrelated write or restart retained pending queue advancement")
			}
			after, err := BuildChildRunInventoryV1(manager.root)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected queue advancement rewrote durable job")
			}
		})
	}
}

func TestPendingSteerStartChainIsIsolatedFromOtherJobs(t *testing.T) {
	manager, original := pendingSteerStartFixtureV1(t)
	queuePendingStartSteerV1(t, manager, original, "steer-first")
	if _, err := manager.Start("goal-independent", "thread-independent", "shell"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ClaimChildRunStart(original); err != nil {
		t.Fatalf("independent job invalidated pending start chain: %v", err)
	}
}
