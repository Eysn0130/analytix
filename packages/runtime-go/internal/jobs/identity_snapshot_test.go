package jobs

import (
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChildRunIdentitySnapshotPreservesRawRecords(t *testing.T) {
	root := t.TempDir()
	body := []byte(`{"id":"job-900","kind":"subagent","status":"completed","parentThreadId":"thr_durable_1","parentTurnId":"turn_17","childThreadId":"thr_durable_fork_800","childTurnId":"turn_950","autoContinueTurnId":"turn_990"}`)
	path := filepath.Join(root, "job-900.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Records) != 1 || snapshot.Records[0].ChildTurnID != "turn_950" || snapshot.Records[0].AutoContinueTurnID != "turn_990" || snapshot.Records[0].SecurityBinding != nil {
		t.Fatal("raw identity snapshot normalized or dropped committed identity")
	}
	if err := ValidateChildRunInventoryV1(root, snapshot.Inventory); err != nil {
		t.Fatal(err)
	}
	readback, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(body, readback) {
		t.Fatal("identity read changed source bytes")
	}
	if err := os.WriteFile(filepath.Join(root, "job-901.json"), []byte(`{"id":"job-901"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChildRunInventoryV1(root, snapshot.Inventory); err == nil {
		t.Fatal("full identity manifest ignored a new job")
	}
}

func TestChildIdentityFloorsReachJobConstructorBeforeAllocation(t *testing.T) {
	for _, semantic := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "child-runs")
		manager, err := newManager(root, root, nil, nil, semantic, domainpendingwork.ChildIdentityFloorsV1{JobSequence: 900})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("floor initialization created missing jobs")
		}
		record, err := manager.Start("goal-synthetic", "thr_durable_1", "shell")
		if err != nil || record.ID != "job-901" {
			t.Fatalf("first job reused signed identity: %s %v", record.ID, err)
		}
	}
}

func TestChildRunIdentitySnapshotDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "child-runs")
	snapshot, err := ReadChildRunIdentitySnapshotV1(context.Background(), root)
	if err != nil || snapshot.Inventory.RootExists || len(snapshot.Records) != 0 {
		t.Fatalf("missing inventory: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("identity snapshot created missing root")
	}
}
