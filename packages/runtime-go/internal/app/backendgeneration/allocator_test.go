package backendgeneration

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	domainbackend "analytix.local/runtime-go/internal/domain/backendgeneration"
	backendport "analytix.local/runtime-go/internal/ports/backendgeneration"
)

func TestAllocatorV1ConsumesCanonicalContinuousChainAndBurnsLostResponse(t *testing.T) {
	store := newMemoryStoreV1()
	allocator, err := NewAllocatorV1(store, bytes.NewReader(bytes.Repeat([]byte{0x41}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	first, err := allocator.Consume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.Generation != 1 || first.Record.PreviousRecordDigest != "" || first.RecordDigest == "" {
		t.Fatalf("unexpected first allocation: %#v", first)
	}
	// Simulate a committed response being lost by discarding it. The next call
	// must consume a successor rather than replaying generation one.
	second, err := allocator.Consume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Record.Generation != 2 || second.Record.PreviousRecordDigest != first.RecordDigest {
		t.Fatalf("lost response did not burn generation one: first=%#v second=%#v", first, second)
	}
	third, err := allocator.Consume(context.Background())
	if err != nil || third.Record.Generation != 3 || third.Record.PreviousRecordDigest != second.RecordDigest {
		t.Fatalf("third allocation did not extend the exact chain: %#v err=%v", third, err)
	}
}

func TestAllocatorV1ConcurrentConsumersNeverReturnSameGeneration(t *testing.T) {
	store := newMemoryStoreV1()
	allocator, err := NewAllocatorV1(store, repeatingReaderV1{})
	if err != nil {
		t.Fatal(err)
	}
	const count = 128
	results := make(chan ConsumedAllocationV1, count)
	errorsSeen := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			result, consumeErr := allocator.Consume(context.Background())
			if consumeErr != nil {
				errorsSeen <- consumeErr
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errorsSeen)
	for consumeErr := range errorsSeen {
		t.Fatal(consumeErr)
	}
	generations := make([]int, 0, count)
	for result := range results {
		generations = append(generations, int(result.Record.Generation))
	}
	sort.Ints(generations)
	if len(generations) != count {
		t.Fatalf("allocation count = %d, want %d", len(generations), count)
	}
	for index, generation := range generations {
		if generation != index+1 {
			t.Fatalf("concurrent allocation fork or gap at %d: %#v", index, generations)
		}
	}
}

func TestAllocatorV1RejectsForkGapOrphanAndBodyDigestMismatch(t *testing.T) {
	validStore := newMemoryStoreV1()
	allocator, _ := NewAllocatorV1(validStore, repeatingReaderV1{})
	first, err := allocator.Consume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*memoryStoreV1)
	}{
		{name: "gap", mutate: func(store *memoryStoreV1) {
			record := mustAllocationRecordV1(t, 3, first.RecordDigest, 0x33)
			store.addRecord(t, record)
		}},
		{name: "orphan", mutate: func(store *memoryStoreV1) {
			record := mustAllocationRecordV1(t, 2, fmt.Sprintf("%064x", 7), 0x34)
			store.addRecord(t, record)
		}},
		{name: "digest-mismatch", mutate: func(store *memoryStoreV1) {
			for digest, body := range store.records {
				delete(store.records, digest)
				store.records[fmt.Sprintf("%064x", 9)] = body
				break
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := validStore.clone()
			test.mutate(store)
			candidate, _ := NewAllocatorV1(store, repeatingReaderV1{})
			if _, err := candidate.Consume(context.Background()); !errors.Is(err, backendport.ErrCorrupt) {
				t.Fatalf("corrupt allocation chain was accepted: %v", err)
			}
		})
	}

	store := validStore.clone()
	fork := mustAllocationRecordV1(t, 1, "", 0x55)
	store.addRecord(t, fork)
	candidate, _ := NewAllocatorV1(store, repeatingReaderV1{})
	if _, err := candidate.Consume(context.Background()); !errors.Is(err, backendport.ErrCorrupt) {
		t.Fatalf("same-generation fork was accepted: %v", err)
	}
}

func TestAllocatorV1PostCommitErrorFailsClosedAndBurnsObservedRecord(t *testing.T) {
	store := newMemoryStoreV1()
	store.postCommitError = errors.New("simulated response loss inside transaction")
	allocator, _ := NewAllocatorV1(store, repeatingReaderV1{})
	if result, err := allocator.Consume(context.Background()); err == nil || result.RecordDigest != "" {
		t.Fatalf("post-commit error was reported as a successful allocation: %#v err=%v", result, err)
	}
	store.postCommitError = nil
	result, err := allocator.Consume(context.Background())
	if err != nil || result.Record.Generation != 2 {
		t.Fatalf("record observed after response loss was reused: %#v err=%v", result, err)
	}
}

type repeatingReaderV1 struct{}

func (repeatingReaderV1) Read(target []byte) (int, error) {
	for index := range target {
		target[index] = byte(index + 1)
	}
	return len(target), nil
}

type memoryStoreV1 struct {
	mu              sync.Mutex
	records         map[string][]byte
	postCommitError error
}

func newMemoryStoreV1() *memoryStoreV1 {
	return &memoryStoreV1{records: make(map[string][]byte)}
}

func (store *memoryStoreV1) WithExclusive(
	ctx context.Context,
	use func(backendport.TransactionV1) error,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(memoryTransactionV1{store: store})
}

func (store *memoryStoreV1) clone() *memoryStoreV1 {
	store.mu.Lock()
	defer store.mu.Unlock()
	clone := newMemoryStoreV1()
	for digest, body := range store.records {
		clone.records[digest] = append([]byte(nil), body...)
	}
	return clone
}

func (store *memoryStoreV1) addRecord(t *testing.T, record domainbackend.AllocationRecordV1) {
	t.Helper()
	body, err := domainbackend.AllocationRecordBytesV1(record)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := domainbackend.AllocationRecordDigestV1(record)
	if err != nil {
		t.Fatal(err)
	}
	store.records[digest] = body
}

type memoryTransactionV1 struct {
	store *memoryStoreV1
}

func (transaction memoryTransactionV1) Visit(
	ctx context.Context,
	visit func(backendport.StoredAllocationV1) error,
) error {
	for digest, body := range transaction.store.records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(backendport.StoredAllocationV1{Digest: digest, Body: append([]byte(nil), body...)}); err != nil {
			return err
		}
	}
	return nil
}

func (transaction memoryTransactionV1) PutIfAbsent(
	ctx context.Context,
	digest string,
	body []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if existing, ok := transaction.store.records[digest]; ok {
		if bytes.Equal(existing, body) {
			return backendport.ErrConflict
		}
		return backendport.ErrCorrupt
	}
	transaction.store.records[digest] = append([]byte(nil), body...)
	return transaction.store.postCommitError
}

func (transaction memoryTransactionV1) Resolve(
	ctx context.Context,
	digest string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, ok := transaction.store.records[digest]
	if !ok {
		return nil, backendport.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func mustAllocationRecordV1(
	t *testing.T,
	generation uint64,
	previous string,
	nonceByte byte,
) domainbackend.AllocationRecordV1 {
	t.Helper()
	record, err := domainbackend.NewAllocationRecordV1(
		generation,
		previous,
		base64NonceV1(nonceByte),
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func base64NonceV1(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}
