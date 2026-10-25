package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func testProtectedOutputReferenceV1(value string) *domainjob.ProtectedOutputSnapshotV1 {
	return &domainjob.ProtectedOutputSnapshotV1{Binding: domaintoolresult.ProtectedSnapshotBindingV1{
		Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1,
		SnapshotDigest: domainsecurity.SHA256Hex([]byte(value)), BodyDigest: domainsecurity.SHA256Hex(nil),
	}}
}

func TestProtectedJobOutputReferenceCommitsWithTerminalAndSurvivesColdLoadV1(t *testing.T) {
	root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(filepath.Base(path), ".json")
	ref := testProtectedOutputReferenceV1("original private CAS snapshot")
	if _, err := manager.UpdateChildRun(id, UpdateRequest{ProtectedOutputSnapshot: ref}); err == nil {
		t.Fatal("running Job acquired a fabricated terminal output reference")
	}
	completed, err := manager.UpdateChildRun(id, UpdateRequest{Status: "completed", ProtectedOutputSnapshot: ref})
	if err != nil || completed.Output != "" || completed.ProtectedOutputSnapshot == nil {
		t.Fatal("terminal capture not committed", err)
	}
	ref.Binding.SnapshotDigest = domainsecurity.SHA256Hex([]byte("mutated pointer"))
	loaded, err := manager.LoadChildRun(id)
	if err != nil || *loaded.ProtectedOutputSnapshot != *completed.ProtectedOutputSnapshot {
		t.Fatal("caller changed private reference", err)
	}
	if _, err := manager.UpdateChildRun(id, UpdateRequest{ProtectedOutputSnapshot: ref}); err == nil {
		t.Fatal("terminal reference changed")
	}
	reopened, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := reopened.LoadChildRun(id)
	if err != nil || cold.ProtectedOutputSnapshot == nil || *cold.ProtectedOutputSnapshot != *completed.ProtectedOutputSnapshot || cold.Output != "" {
		t.Fatal("cold Job lost private immutable reference", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(body), "original private CAS snapshot") {
		t.Fatal("producer text entered ordinary Job JSON", err)
	}
}

func TestProtectedJobOutputCannotBeBackfilledOrReplaceSuppressedCompletionV1(t *testing.T) {
	for _, status := range []string{"completed", "killed"} {
		t.Run(status, func(t *testing.T) {
			root, path := newBoundBackgroundRecordForSemanticMigrationTest(t)
			manager, err := NewManager(root)
			if err != nil {
				t.Fatal(err)
			}
			id := strings.TrimSuffix(filepath.Base(path), ".json")
			if _, err := manager.UpdateChildRun(id, UpdateRequest{Status: status}); err != nil {
				t.Fatal(err)
			}
			late, err := manager.UpdateChildRun(id, UpdateRequest{Status: "completed", ProtectedOutputSnapshot: testProtectedOutputReferenceV1("late")})
			if status == "completed" && err == nil {
				t.Fatal("missing historical output was reconstructed")
			}
			if status == "killed" && (err != nil || late.Status != "killed" || !late.LateCompletionSuppressed || late.ProtectedOutputSnapshot != nil) {
				t.Fatal("late capture changed original stopped Job or suppression facts", err)
			}
		})
	}
}
