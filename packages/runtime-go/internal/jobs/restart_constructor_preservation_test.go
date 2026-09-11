package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
)

func newPreservedSemanticManagerForTestV1(t *testing.T, root string, held Record) (*Manager, error) {
	t.Helper()
	snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
	if err != nil {
		return nil, err
	}
	for _, record := range snapshot.Records {
		if record.ID == held.ID {
			held = record
		}
	}
	return NewManagerForSemanticStartupWithRestartPreservationV1(context.Background(), root, root, nil, nil, &RestartPreservationInputV1{
		ThreadIDs: []string{held.ParentThreadID, held.ChildThreadID}, JobIDs: []string{held.ID}, OriginalRecords: []Record{held},
		OriginalInventory: snapshot.Inventory,
	})
}

func TestRestartPreservationPrecedesSemanticConstructorEffects(t *testing.T) {
	root := t.TempDir()
	initial, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	held := startClaimFixture(t, initial, "running")
	ordinary, err := initial.StartChildRun(StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, held.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "\t"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(indented.Bytes(), '\n', '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{held.ID, ordinary.ID} {
		if err := os.WriteFile(filepath.Join(root, id+".log"), []byte("synthetic artifact"), 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := createChildRunTemporaryFileV1(root, id+".json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("synthetic temporary"); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	before, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newPreservedSemanticManagerForTestV1(t, root, held)
	if err != nil {
		t.Fatal(err)
	}
	after, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]ChildRunInventoryEntryV1{}
	for _, entry := range after.Entries {
		entries[entry.Name] = entry
		if entry.JobID == ordinary.ID && entry.Kind != ChildRunInventoryRecordV1 {
			t.Error("independent constructor cleanup did not finish")
		}
	}
	for _, entry := range before.Entries {
		if entry.JobID == held.ID && !reflect.DeepEqual(entry, entries[entry.Name]) {
			t.Errorf("constructor changed held %s before scope installation", entry.Kind)
		}
	}
	if _, err := manager.UpdateChildRun(ordinary.ID, UpdateRequest{Output: "independent progress"}); err != nil {
		t.Fatalf("independent live update failed: %v", err)
	}
	loaded, err := manager.LoadChildRun(held.ID)
	if err != nil {
		t.Fatal(err)
	}
	if held, err := manager.RestartPreservesChildRunV1(loaded); err != nil || !held {
		t.Fatalf("constructor did not install the live hold: held=%t err=%v", held, err)
	}
	if _, err := manager.UpdateChildRun(loaded.ID, UpdateRequest{Status: "interrupted"}); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("held record accepted a live effect: %v", err)
	}
}

func TestRestartConstructorRejectsInvalidOriginalBeforeIndependentEffects(t *testing.T) {
	for _, fault := range []string{"missing_record", "changed_record", "missing_job", "duplicate_job", "spaced_job", "duplicate_thread", "unrelated_record", "held_status", "held_sequence", "held_unprojected", "independent_status", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			initial, err := NewManager(root)
			if err != nil {
				t.Fatal(err)
			}
			held := startClaimFixture(t, initial, "running")
			ordinary, err := initial.StartChildRun(StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running"})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			held, ordinary = snapshot.Records[0], snapshot.Records[1]
			switch fault {
			case "held_status":
				held.Status = "unknown"
			case "held_sequence":
				held.ChildSeq = 0
			case "held_unprojected":
				held.Name = "  synthetic display  "
			case "independent_status":
				ordinary.Status = "unknown"
			}
			for _, record := range []Record{held, ordinary} {
				body, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, record.ID+".json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ordinary.ID+".log"), []byte("synthetic residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			input := RestartPreservationInputV1{ThreadIDs: []string{held.ParentThreadID, held.ChildThreadID}, JobIDs: []string{held.ID}, OriginalRecords: []Record{held}}
			ctx := context.Background()
			switch fault {
			case "missing_record":
				input.OriginalRecords = nil
			case "changed_record":
				input.OriginalRecords[0].Status = "interrupted"
			case "missing_job":
				input.JobIDs = nil
			case "duplicate_job":
				input.JobIDs = append(input.JobIDs, held.ID)
			case "spaced_job":
				input.JobIDs = append(input.JobIDs, " job-901")
			case "duplicate_thread":
				input.ThreadIDs = append(input.ThreadIDs, held.ParentThreadID)
			case "unrelated_record":
				input.JobIDs = append(input.JobIDs, ordinary.ID)
				input.OriginalRecords = append(input.OriginalRecords, ordinary)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before, err := BuildChildRunInventoryV1(root)
			if err != nil {
				t.Fatal(err)
			}
			input.OriginalInventory = before
			manager, err := NewManagerForSemanticStartupWithRestartPreservationV1(ctx, root, root, nil, nil, &input)
			if err == nil || manager != nil {
				t.Fatal("invalid original admitted a constructor")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Errorf("constructor lost cancellation: %v", err)
			}
			if err := ValidateChildRunInventoryV1(root, before); err != nil {
				t.Fatalf("rejected constructor changed original inventory: %v", err)
			}
		})
	}
}

func TestRestartConstructorRejectsPhysicalDriftSinceOriginalObservation(t *testing.T) {
	for _, fault := range []string{"record_format", "artifact_removed", "temporary_added", "absent_reserved_added"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			initial, err := NewManager(root)
			if err != nil {
				t.Fatal(err)
			}
			held := startClaimFixture(t, initial, "running")
			if err := os.WriteFile(filepath.Join(root, held.ID+".log"), []byte("synthetic original artifact"), 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			input := &RestartPreservationInputV1{ThreadIDs: []string{held.ParentThreadID, held.ChildThreadID}, JobIDs: []string{held.ID, "job-901"}, OriginalRecords: snapshot.Records, OriginalInventory: snapshot.Inventory}
			switch fault {
			case "record_format":
				path := filepath.Join(root, held.ID+".json")
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(path, append(body, '\n'), 0o600)
			case "artifact_removed":
				err = os.Remove(filepath.Join(root, held.ID+".log"))
			case "temporary_added":
				var file *os.File
				file, err = createChildRunTemporaryFileV1(root, held.ID+".json")
				if err == nil {
					err = file.Close()
				}
			case "absent_reserved_added":
				err = os.WriteFile(filepath.Join(root, "job-901.log"), []byte("synthetic unexpected reserved artifact"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := BuildChildRunInventoryV1(root)
			if err != nil {
				t.Fatal(err)
			}
			if manager, err := NewManagerForSemanticStartupWithRestartPreservationV1(context.Background(), root, root, nil, nil, input); err == nil || manager != nil {
				t.Error("constructor promoted changed physical inventory into original authority")
			}
			if err := ValidateChildRunInventoryV1(root, before); err != nil {
				t.Fatalf("rejected constructor wrote changed input: %v", err)
			}
		})
	}
}

func TestRestartConstructorPreservesAbsentReservedJobs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-900.log"), []byte("synthetic reserved artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := &RestartPreservationInputV1{ThreadIDs: []string{"thread-held"}, JobIDs: []string{"job-900", "job-901"}}
	var err error
	input.OriginalInventory, err = BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManagerForSemanticStartupWithRestartPreservationV1(context.Background(), root, root, nil, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running"})
	if err != nil || ordinary.ID != "job-902" {
		t.Fatalf("original reservation floor was reused: id=%s err=%v", ordinary.ID, err)
	}
	manager, err = NewManagerWithRestartPreservationV1(context.Background(), root, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-held", ParentThreadID: "thread-held", Kind: "subagent", Status: "running"}); !errors.Is(err, ErrRestartPreserved) {
		t.Fatalf("held absent parent admitted a new child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "job-901.log"), []byte("synthetic unexpected held addition"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CleanupStaleRunningRecords(); err == nil {
		t.Fatal("new held physical artifact escaped original absence")
	}
	if err := ValidateChildRunInventoryV1(root, before); err != nil {
		t.Fatalf("stale scope cleanup wrote inventory: %v", err)
	}
}

type constructorCompletionFailureV1 struct{ err error }

func (verifier constructorCompletionFailureV1) VerifyStoredChildCompletion(context.Context, Record) error {
	return verifier.err
}

func TestRestartConstructorPreservesCompletionVerificationErrors(t *testing.T) {
	parent, child, binding, receipt := jobCompletionReceiptFixture(t)
	root := t.TempDir()
	initial, err := NewManagerWithChildCompletionVerifier(root, completionVerifierStub{digest: receipt.ReceiptDigest})
	if err != nil {
		t.Fatal(err)
	}
	request := StartRequest{ParentGoalID: "goal-1", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID, ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID, Kind: "subagent", Status: "running"}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
	record, err := initial.StartChildRun(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initial.UpdateChildRun(record.ID, UpdateRequest{Status: "completed", ChildTurnID: child.TurnID, ChildCompletionReceipt: &receipt}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	input := &RestartPreservationInputV1{ThreadIDs: []string{parent.ThreadID, child.ThreadID}, JobIDs: []string{record.ID}, OriginalRecords: snapshot.Records, OriginalInventory: snapshot.Inventory}
	for _, sentinel := range []error{errors.New("synthetic completion verification I/O failure"), context.Canceled} {
		manager, err := NewManagerForSemanticStartupWithRestartPreservationV1(context.Background(), root, root, constructorCompletionFailureV1{err: sentinel}, nil, input)
		if manager != nil || !errors.Is(err, sentinel) {
			t.Errorf("constructor lost completion verification failure: %v", err)
		}
		if err := ValidateChildRunInventoryV1(root, snapshot.Inventory); err != nil {
			t.Fatalf("failed completion observation wrote inventory: %v", err)
		}
	}
	if manager, err := NewManagerForSemanticStartupWithRestartPreservationV1(context.Background(), root, root, nil, nil, input); err == nil || manager != nil {
		t.Fatal("constructor bypassed missing trusted completion verifier")
	}
}
