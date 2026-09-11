package pendingworkstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func pendingPreparedTreeV1(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = domainsecurity.SHA256Hex(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPreparedPendingInventoryPreservesClosedExpiredChildVectorsWithoutLiveOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	store, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	_, key, input := storeReceiptFixture(t, domainpendingwork.KindSideEffectIntent)
	input.ChildProducer = &domainpendingwork.ChildProducerV1{ParentBindingDigest: strings.Repeat("a", 64), Children: []domainpendingwork.ChildProducerTargetV1{{Ordinal: 1, JobID: "job-71", ChildThreadID: "thr_durable_fork_81", ChildTurnID: "turn_91"}}}
	receipt := mustStoreReceipt(t, input, key)
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	disposition := mustStoreDisposition(t, receipt, key, domainpendingwork.StatusCancelled, "tool_call_cancelled", input.IssuedAt.Add(time.Minute))
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.receiptCAS.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.dispositionCAS.Close(); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	before := pendingPreparedTreeV1(t, root)
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	receipts, dispositions, err := prepared.SnapshotInventory(context.Background())
	if err != nil || len(receipts) != 1 || len(dispositions) != 1 || !reflect.DeepEqual(receipts[0], receipt) || !reflect.DeepEqual(dispositions[0], disposition) {
		t.Fatalf("prepared inventory omitted or rewrote closed expired child allocation: %v", err)
	}
	receipts[0].ChildProducer.Children[0].ChildTurnID = "turn_999"
	current, _, err := prepared.SnapshotInventory(context.Background())
	if err != nil || !reflect.DeepEqual(current[0], receipt) {
		t.Fatal("caller changed prepared child allocation")
	}
	if !reflect.DeepEqual(before, pendingPreparedTreeV1(t, root)) {
		t.Fatal("prepared inventory wrote or recovered private CAS")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if values, _, err := prepared.SnapshotInventory(ctx); err == nil || values != nil {
		t.Fatal("cancelled prepared inventory returned a partial denominator")
	}
}

func TestPreparedPendingAbsentInventoryDoesNotCreateAReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	receipts, dispositions, err := prepared.SnapshotInventory(context.Background())
	if err != nil || len(receipts) != 0 || len(dispositions) != 0 {
		t.Fatalf("absent inventory failed: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("observing absent pending owner created a replacement")
	}
}
