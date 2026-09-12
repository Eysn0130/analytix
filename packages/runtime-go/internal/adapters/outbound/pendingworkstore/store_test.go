package pendingworkstore

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestStorePersistsContentAddressedReceiptAndExactDispositionAcrossRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	store, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if has, err := store.HasRecords(context.Background()); err != nil || has {
		t.Fatalf("new pending work store is not empty: has=%v err=%v", has, err)
	}
	receipt, privateKey, input := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatalf("idempotent pending work receipt write failed: %v", err)
	}
	expectedReceiptPath := filepath.Join(root, "receipts", receipt.WorkID[:2], receipt.WorkID+".json")
	if _, err := os.Stat(expectedReceiptPath); err != nil {
		t.Fatalf("content-addressed receipt path is missing: %v", err)
	}
	read, err := store.ReadReceipt(context.Background(), receipt.WorkID)
	if err != nil || read.ReceiptID != receipt.ReceiptID {
		t.Fatalf("pending work receipt read mismatch: read=%#v err=%v", read, err)
	}

	publicKey := privateKey.Public().(ed25519.PublicKey)
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		receipt, domainpendingwork.StatusCompleted, "tool_batch_completed", input.IssuedAt.Add(time.Minute),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatalf("idempotent pending work disposition write failed: %v", err)
	}

	reopened, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	receipts, receiptErr := reopened.ListReceipts(context.Background())
	dispositions, dispositionErr := reopened.ListDispositions(context.Background())
	if receiptErr != nil || dispositionErr != nil || len(receipts) != 1 || len(dispositions) != 1 ||
		receipts[0].WorkID != receipt.WorkID || dispositions[0].DispositionID != disposition.DispositionID {
		t.Fatalf("restart inventory mismatch: receipts=%#v dispositions=%#v receiptErr=%v dispositionErr=%v", receipts, dispositions, receiptErr, dispositionErr)
	}
	if has, err := reopened.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("persisted pending work store appears empty: has=%v err=%v", has, err)
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{root, filepath.Dir(expectedReceiptPath), expectedReceiptPath} {
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			want := os.FileMode(0o700)
			if !info.IsDir() {
				want = 0o600
			}
			if info.Mode().Perm() != want {
				t.Fatalf("private pending work mode for %s = %o, want %o", path, info.Mode().Perm(), want)
			}
		}
	}
}

func TestStoreRecoversRecognizedExclusiveWriteCrashResidue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	store, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	shard := filepath.Join(store.receipts, receipt.WorkID[:2])
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(shard, "."+receipt.WorkID+".json-crash.tmp")
	if err := os.WriteFile(partial, []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recoverTestPendingWorkStore(t, root)
	reopened, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatalf("recover pre-link temp: %v", err)
	}
	if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-link temp survived recovery: %v", err)
	}
	if _, err := os.Stat(shard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty crash shard survived recovery: %v", err)
	}
	if err := reopened.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	finalPath := reopened.receiptPath(receipt.WorkID)
	linkedTemp := filepath.Join(filepath.Dir(finalPath), "."+receipt.WorkID+".json-linked.tmp")
	if err := os.Link(finalPath, linkedTemp); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	recoverTestPendingWorkStore(t, root)
	reopened, err = newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatalf("recover post-link temp: %v", err)
	}
	if _, err := os.Stat(linkedTemp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-link temp survived recovery: %v", err)
	}
	read, err := reopened.ReadReceipt(context.Background(), receipt.WorkID)
	if err != nil || read.ReceiptID != receipt.ReceiptID {
		t.Fatalf("committed receipt was lost while removing crash link: read=%#v err=%v", read, err)
	}
}

func TestStoreRejectsReceiptReplacementAndSecondDisposition(t *testing.T) {
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, privateKey, input := storeReceiptFixture(t, domainpendingwork.KindReportStage)
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	input.IssuedAt = input.IssuedAt.Add(time.Second)
	input.ExpiresAt = input.ExpiresAt.Add(time.Second)
	conflictingReceipt := mustStoreReceipt(t, input, privateKey)
	if conflictingReceipt.WorkID != receipt.WorkID || conflictingReceipt.ReceiptID == receipt.ReceiptID {
		t.Fatal("receipt replacement fixture does not share the logical content address")
	}
	if err := store.PutReceiptIfAbsent(context.Background(), conflictingReceipt); err == nil {
		t.Fatal("store replaced an existing pending work receipt authority")
	}

	first := mustStoreDisposition(t, receipt, privateKey, domainpendingwork.StatusCompleted, "report_stage_completed", input.IssuedAt.Add(time.Minute))
	if err := store.PutDispositionIfAbsent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := mustStoreDisposition(t, receipt, privateKey, domainpendingwork.StatusFailed, "report_stage_failed", input.IssuedAt.Add(2*time.Minute))
	if err := store.PutDispositionIfAbsent(context.Background(), second); err == nil {
		t.Fatal("store accepted a second pending work disposition")
	}
	read, err := store.ReadDisposition(context.Background(), receipt.WorkID)
	if err != nil || read.DispositionID != first.DispositionID {
		t.Fatalf("first disposition was not preserved: read=%#v err=%v", read, err)
	}
}

func TestStoreRejectsDispositionWithoutExactReceipt(t *testing.T) {
	receipt, privateKey, input := storeReceiptFixture(t, domainpendingwork.KindProviderContinuation)
	disposition := mustStoreDisposition(t, receipt, privateKey, domainpendingwork.StatusFailed, "provider_failed", input.IssuedAt.Add(time.Minute))
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err == nil {
		t.Fatal("store accepted a disposition without its exact receipt")
	}
	if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		t.Fatalf("missing receipt did not return ErrNotFound: %v", err)
	}
	if _, err := store.ReadDisposition(context.Background(), receipt.WorkID); !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		t.Fatalf("missing disposition did not return ErrNotFound: %v", err)
	}
}

func TestStoreFailsClosedOnPartialUnknownAndUnsafeInventory(t *testing.T) {
	tests := []struct {
		name   string
		poison func(*testing.T, *Store, domainpendingwork.PendingWorkReceiptV1)
	}{
		{
			name: "partial content address",
			poison: func(t *testing.T, store *Store, receipt domainpendingwork.PendingWorkReceiptV1) {
				path := store.receiptPath(receipt.WorkID)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"schemaVersion":`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := store.PutReceiptIfAbsent(context.Background(), receipt); err == nil {
					t.Fatal("store replaced a partial preexisting content-addressed target")
				}
				body, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(body, []byte(`{"schemaVersion":`)) {
					t.Fatalf("partial authority residue was changed: body=%q err=%v", body, err)
				}
			},
		},
		{
			name: "unknown file",
			poison: func(t *testing.T, store *Store, _ domainpendingwork.PendingWorkReceiptV1) {
				if err := os.WriteFile(filepath.Join(store.receipts, ".DS_Store"), []byte("untrusted"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "empty shard",
			poison: func(t *testing.T, store *Store, _ domainpendingwork.PendingWorkReceiptV1) {
				if err := os.Mkdir(filepath.Join(store.receipts, "ff"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests, struct {
			name   string
			poison func(*testing.T, *Store, domainpendingwork.PendingWorkReceiptV1)
		}{
			name: "symlink",
			poison: func(t *testing.T, store *Store, _ domainpendingwork.PendingWorkReceiptV1) {
				if err := os.Symlink(filepath.Join(store.root, "outside"), filepath.Join(store.receipts, "unsafe")); err != nil {
					t.Fatal(err)
				}
			},
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
			if err != nil {
				t.Fatal(err)
			}
			receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
			test.poison(t, store, receipt)
			if _, err := store.ListReceipts(context.Background()); err == nil {
				t.Fatal("ListReceipts accepted poisoned pending work inventory")
			}
			if has, err := store.HasRecords(context.Background()); err == nil || has {
				t.Fatalf("HasRecords did not fail closed: has=%v err=%v", has, err)
			}
		})
	}
}

func TestStoreConcurrentPutIsIdempotentAndHonorsCancellation(t *testing.T) {
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	const writers = 24
	errorsByWriter := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByWriter <- store.PutReceiptIfAbsent(context.Background(), receipt)
		}()
	}
	wait.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("concurrent idempotent writer failed: %v", err)
		}
	}
	receipts, err := store.ListReceipts(context.Background())
	if err != nil || len(receipts) != 1 {
		t.Fatalf("concurrent receipt inventory mismatch: receipts=%#v err=%v", receipts, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.PutReceiptIfAbsent(cancelled, receipt); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled pending work write returned %v", err)
	}
	if _, err := store.ListReceipts(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled pending work list returned %v", err)
	}
}

func TestStoreAllowsPreopenedSiblingExactReplayAfterExactlyOneCommit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	first, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	start := make(chan struct{})
	errorsByStore := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		go func(candidate *Store) {
			<-start
			errorsByStore <- candidate.PutReceiptIfAbsent(context.Background(), receipt)
		}(store)
	}
	close(start)
	succeeded := 0
	rejected := 0
	for range 2 {
		if err := <-errorsByStore; err != nil {
			rejected++
		} else {
			succeeded++
		}
	}
	if succeeded != 2 || rejected != 0 {
		t.Fatalf("preopened sibling replay result: succeeded=%d rejected=%d", succeeded, rejected)
	}
	reopened, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := reopened.ListReceipts(context.Background())
	if err != nil || len(receipts) != 1 || receipts[0].ReceiptID != receipt.ReceiptID {
		t.Fatalf("no-replace contender inventory mismatch: receipts=%#v err=%v", receipts, err)
	}
}

func TestStoreExclusiveReceiptCreateHasExactlyOneCreatorAcrossPreopenedSiblings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	first, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindApprovedToolDispatch)
	start := make(chan struct{})
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 2)
	for _, candidate := range []*Store{first, second} {
		go func(store *Store) {
			<-start
			created, err := store.CreateReceiptExclusive(context.Background(), receipt)
			results <- result{created: created, err: err}
		}(candidate)
	}
	close(start)
	createdCount := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("exclusive receipt contender failed: %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("exclusive receipt creators = %d, want exactly one", createdCount)
	}
	if created, err := first.CreateReceiptExclusive(context.Background(), receipt); err != nil || created {
		t.Fatalf("exact replay regained creator authority: created=%v err=%v", created, err)
	}
}

func TestStoreSnapshotInventoryBlocksSiblingMutationAcrossBothRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	reader, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	first, privateKey, input := storeReceiptFixture(t, domainpendingwork.KindApprovedToolDispatch)
	if created, err := reader.CreateReceiptExclusive(context.Background(), first); err != nil || !created {
		t.Fatalf("seed receipt create: created=%v err=%v", created, err)
	}
	input.GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("grant-sibling"))
	input.GrantMembers[0].RegistryEntryDigest = domainsecurity.SHA256Hex([]byte("entry-sibling"))
	input.PayloadHash = domainsecurity.SHA256Hex([]byte("payload-sibling"))
	second := mustStoreReceipt(t, input, privateKey)
	if second.WorkID == first.WorkID {
		t.Fatal("sibling snapshot fixture did not create a distinct work identity")
	}

	snapshotCut := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	reader.afterReceiptSnapshot = func() {
		close(snapshotCut)
		<-releaseSnapshot
	}
	mutationAtGate := make(chan struct{})
	writer.beforeMutationGate = func() { close(mutationAtGate) }
	type snapshotResult struct {
		receipts     []domainpendingwork.PendingWorkReceiptV1
		dispositions []domainpendingwork.PendingWorkDispositionV1
		err          error
	}
	snapshotDone := make(chan snapshotResult, 1)
	go func() {
		receipts, dispositions, err := reader.SnapshotInventory(context.Background())
		snapshotDone <- snapshotResult{receipts: receipts, dispositions: dispositions, err: err}
	}()
	<-snapshotCut
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.CreateReceiptExclusive(context.Background(), second)
		writeDone <- err
	}()
	<-mutationAtGate
	select {
	case err := <-writeDone:
		t.Fatalf("sibling mutation crossed an in-progress two-root snapshot: %v", err)
	default:
	}
	close(releaseSnapshot)
	snapshot := <-snapshotDone
	if snapshot.err != nil || len(snapshot.receipts) != 1 || snapshot.receipts[0].WorkID != first.WorkID || len(snapshot.dispositions) != 0 {
		t.Fatalf("snapshot was not one immutable inventory point: %#v", snapshot)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("sibling mutation did not resume after snapshot: %v", err)
	}
	receipts, err := reader.ListReceipts(context.Background())
	if err != nil || len(receipts) != 2 {
		t.Fatalf("post-snapshot inventory lost the serialized sibling mutation: receipts=%#v err=%v", receipts, err)
	}
}

func TestStoreSnapshotInventoryFailsClosedWhenAppendOnlyInventoryChanges(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	reader, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	bypass, err := newTestPendingWorkStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	first, privateKey, input := storeReceiptFixture(t, domainpendingwork.KindApprovedToolDispatch)
	if created, err := reader.CreateReceiptExclusive(context.Background(), first); err != nil || !created {
		t.Fatalf("seed receipt create: created=%v err=%v", created, err)
	}
	input.GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("grant-bypass"))
	input.GrantMembers[0].RegistryEntryDigest = domainsecurity.SHA256Hex([]byte("entry-bypass"))
	input.PayloadHash = domainsecurity.SHA256Hex([]byte("payload-bypass"))
	second := mustStoreReceipt(t, input, privateKey)
	body, err := domainpendingwork.PendingWorkReceiptV1Bytes(second)
	if err != nil {
		t.Fatal(err)
	}
	reader.afterReceiptSnapshot = func() {
		reader.afterReceiptSnapshot = nil
		if err := bypass.receiptCAS.PutIfAbsent(context.Background(), second.WorkID, body); err != nil {
			t.Errorf("install bypass receipt at deterministic snapshot cut: %v", err)
		}
	}
	if _, _, err := reader.SnapshotInventory(context.Background()); !errors.Is(err, ErrInventoryChanged) {
		t.Fatalf("changing append-only inventory returned a mixed snapshot: %v", err)
	}
}

type receiptInventoryAccessCounter struct {
	*privatecastest.AccessAuthority
	receiptAccesses int
}

func (counter *receiptInventoryAccessCounter) WithPrivateCASAccess(ctx context.Context, root string, access func(privatecasport.RootBinding) error) error {
	if filepath.Base(root) == "receipts" {
		counter.receiptAccesses++
	}
	return counter.AccessAuthority.WithPrivateCASAccess(ctx, root, access)
}

func TestStoreSnapshotInventoryReceiptScansDoNotGrowWithDispositions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pending-work")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	counter := &receiptInventoryAccessCounter{AccessAuthority: access}
	store, err := NewStore(root, counter)
	if err != nil {
		t.Fatal(err)
	}
	_, key, input := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	receipts := make([]domainpendingwork.PendingWorkReceiptV1, 4)
	for index := range receipts {
		input.PayloadHash = domainsecurity.SHA256Hex([]byte{byte(index)})
		receipts[index] = mustStoreReceipt(t, input, key)
		if err := store.PutReceiptIfAbsent(context.Background(), receipts[index]); err != nil {
			t.Fatal(err)
		}
	}
	baseline := 0
	for count := 0; count <= len(receipts); count++ {
		if count > 0 {
			disposition := mustStoreDisposition(t, receipts[count-1], key, domainpendingwork.StatusCompleted, "tool_batch_completed", input.IssuedAt.Add(time.Minute))
			if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
				t.Fatal(err)
			}
		}
		counter.receiptAccesses = 0
		gotReceipts, gotDispositions, err := store.SnapshotInventory(context.Background())
		if err != nil || len(gotReceipts) != len(receipts) || len(gotDispositions) != count {
			t.Fatalf("snapshot inventory is incomplete: receipts=%d dispositions=%d err=%v", len(gotReceipts), len(gotDispositions), err)
		}
		if count == 0 {
			baseline = counter.receiptAccesses
			if baseline < 2 {
				t.Fatal("snapshot omitted its two fresh receipt CAS observations")
			}
		} else if counter.receiptAccesses != baseline {
			t.Fatalf("receipt CAS scans grew with dispositions: dispositions=%d accesses=%d baseline=%d", count, counter.receiptAccesses, baseline)
		}
	}
}

func TestStoreSnapshotInventoryRejectsDispositionCrossBinding(t *testing.T) {
	for _, fixture := range []string{"missing-receipt", "different-receipt"} {
		t.Run(fixture, func(t *testing.T) {
			store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
			if err != nil {
				t.Fatal(err)
			}
			receipt, key, input := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
			if fixture == "different-receipt" {
				if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
					t.Fatal(err)
				}
				input.IssuedAt = input.IssuedAt.Add(time.Second)
				other := mustStoreReceipt(t, input, key)
				if other.WorkID != receipt.WorkID || other.ReceiptID == receipt.ReceiptID {
					t.Fatal("fixture did not preserve work identity with a different signed receipt")
				}
				receipt = other
			}
			disposition := mustStoreDisposition(t, receipt, key, domainpendingwork.StatusCompleted, "tool_batch_completed", input.IssuedAt.Add(time.Minute))
			body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
			if err != nil {
				t.Fatal(err)
			}
			// Admit a canonical CAS record while bypassing only the cross-owner
			// write API, so the read must independently reject the hostile link.
			if err := store.dispositionCAS.PutIfAbsent(context.Background(), disposition.WorkID, body); err != nil {
				t.Fatal(err)
			}
			if receipts, dispositions, err := store.SnapshotInventory(context.Background()); err == nil || receipts != nil || dispositions != nil {
				t.Fatal("snapshot accepted a disposition without its exact receipt")
			}
		})
	}
}

func TestStoreSnapshotInventoryRejectsReceiptTamperingBetweenObservations(t *testing.T) {
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, key, input := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	disposition := mustStoreDisposition(t, receipt, key, domainpendingwork.StatusCompleted, "tool_batch_completed", input.IssuedAt.Add(time.Minute))
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	store.afterReceiptSnapshot = func() {
		if err := os.Remove(store.receiptPath(receipt.WorkID)); err != nil {
			t.Fatal(err)
		}
	}
	if receipts, dispositions, err := store.SnapshotInventory(context.Background()); err == nil || receipts != nil || dispositions != nil {
		t.Fatal("same-pass receipt reuse concealed deletion before the confirmation observation")
	}
}

func TestStoreSnapshotInventoryRejectsDuplicateReceiptIdentities(t *testing.T) {
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, key, input := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	input.IssuedAt = input.IssuedAt.Add(time.Second)
	other := mustStoreReceipt(t, input, key)
	if other.WorkID != receipt.WorkID || other.ReceiptID == receipt.ReceiptID {
		t.Fatal("duplicate fixture has no conflicting receipt for the same work")
	}
	// This is the narrow snapshot join boundary: even two otherwise valid
	// signed receipts cannot be silently collapsed by its work-ID index.
	for _, duplicate := range []domainpendingwork.PendingWorkReceiptV1{receipt, other} {
		if _, err := store.listDispositionsForReceiptSnapshot(context.Background(), []domainpendingwork.PendingWorkReceiptV1{receipt, duplicate}); err == nil {
			t.Fatal("receipt snapshot silently collapsed a duplicate identity")
		}
	}
}

func TestStoreRejectsLiveRootAncestorAndShardReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("live rename attacks are exercised on Unix; Windows builds use FileID and OBJ_DONT_REPARSE")
	}
	t.Run("root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "pending-work")
		store, err := newTestPendingWorkStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
		if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		moved := root + ".moved"
		if err := os.Rename(root, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "receipts"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); err == nil {
			t.Fatal("replacement pending-work root retained receipt authority")
		}
	})

	t.Run("ancestor", func(t *testing.T) {
		base := t.TempDir()
		ancestor := filepath.Join(base, "authority")
		root := filepath.Join(ancestor, "pending-work")
		store, err := newTestPendingWorkStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
		if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(ancestor, ancestor+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "receipts"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); err == nil {
			t.Fatal("replacement pending-work ancestor retained receipt authority")
		}
	})

	t.Run("shard with copied record", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "pending-work")
		store, err := newTestPendingWorkStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
		if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		path := store.receiptPath(receipt.WorkID)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		shard := filepath.Dir(path)
		if err := os.Rename(shard, shard+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); err == nil {
			t.Fatal("replacement shard with byte-identical receipt retained authority")
		}
	})
}

func TestStoreRejectsMultiLinkRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink topology test is exercised on Unix")
	}
	store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
	if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	path := store.receiptPath(receipt.WorkID)
	if err := os.Link(path, path+".outside-link"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); err == nil {
		t.Fatal("multi-link receipt retained authority")
	}
}

func TestStoreRejectsBroadenedRootShardAndRecordPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode-bit enforcement does not apply on Windows")
	}
	tests := []struct {
		name string
		path func(*Store, domainpendingwork.PendingWorkReceiptV1) string
		mode os.FileMode
	}{
		{name: "root", path: func(store *Store, _ domainpendingwork.PendingWorkReceiptV1) string { return store.receipts }, mode: 0o755},
		{name: "shard", path: func(store *Store, receipt domainpendingwork.PendingWorkReceiptV1) string {
			return filepath.Dir(store.receiptPath(receipt.WorkID))
		}, mode: 0o755},
		{name: "record", path: func(store *Store, receipt domainpendingwork.PendingWorkReceiptV1) string {
			return store.receiptPath(receipt.WorkID)
		}, mode: 0o644},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := newTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
			if err != nil {
				t.Fatal(err)
			}
			receipt, _, _ := storeReceiptFixture(t, domainpendingwork.KindToolBatch)
			if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(test.path(store, receipt), test.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ReadReceipt(context.Background(), receipt.WorkID); err == nil {
				t.Fatal("broadened private authority permissions were accepted")
			}
		})
	}
}

func storeReceiptFixture(t *testing.T, kind string) (domainpendingwork.PendingWorkReceiptV1, ed25519.PrivateKey, domainpendingwork.ReceiptInputV1) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_710_000_000, 0).UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-store", TurnID: "turn-store", WorkspaceRealPath: "/cases/private", CaseID: "case-private",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-store"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	members := []domainpendingwork.GrantMemberV1{{
		Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant")), RegistrySequence: 1,
		RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry")),
	}}
	if kind == domainpendingwork.KindProviderContinuation {
		members[0].ResultItemID = "item-result"
		members[0].ResultItemDigest = domainsecurity.SHA256Hex([]byte("exact durable result"))
	}
	input := domainpendingwork.ReceiptInputV1{
		Kind: kind, SecurityContext: securityContext, GrantRegistrySequence: 1,
		GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry")), GrantMembers: members,
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload")), RouteHash: domainsecurity.SHA256Hex([]byte("route")),
		IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}
	return mustStoreReceipt(t, input, privateKey), privateKey, input
}

func mustStoreReceipt(t *testing.T, input domainpendingwork.ReceiptInputV1, privateKey ed25519.PrivateKey) domainpendingwork.PendingWorkReceiptV1 {
	t.Helper()
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func mustStoreDisposition(t *testing.T, receipt domainpendingwork.PendingWorkReceiptV1, privateKey ed25519.PrivateKey, status, reason string, disposedAt time.Time) domainpendingwork.PendingWorkDispositionV1 {
	t.Helper()
	publicKey := privateKey.Public().(ed25519.PublicKey)
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		receipt, status, reason, disposedAt, domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return disposition
}

func newTestPendingWorkStore(t *testing.T, root string) (*Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewStore(root, mutation)
}

func recoverTestPendingWorkStore(t *testing.T, root string) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatalf("prepare pending work recovery: %v", err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatalf("validate pending work recovery: %v", err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate pending work recovery: %v", err)
	}
	if err := privatecasrecoverytest.ApplyV4(context.Background(), "pending-work-test", prepared); err != nil {
		t.Fatalf("apply pending work recovery: %v", err)
	}
}
