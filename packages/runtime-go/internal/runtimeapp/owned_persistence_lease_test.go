package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestOwnedPersistenceLeaseShutdownRetainsAuthorityUntilRetryDrainsLifecycle(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	blocker := errors.New("background jobs remain after shutdown deadline")
	lifecycle := &retryShutdownLifecycle{results: []error{blocker, nil}}
	handler := &ownedPersistenceLeaseHandler{Handler: lifecycle, lease: lease}

	if err := handler.Shutdown(context.Background()); !errors.Is(err, blocker) {
		t.Fatalf("first shutdown did not return the lifecycle blocker: %v", err)
	}
	contender, err := AcquireRuntimePersistenceLease(config)
	if err == nil {
		_ = contender.Close()
		t.Fatal("failed lifecycle shutdown released persistence authority")
	}
	if !errors.Is(err, persistencefs.ErrPersistenceInUse) {
		t.Fatalf("failed lifecycle shutdown returned an unexpected lease result: %v", err)
	}

	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry did not drain lifecycle and release authority: %v", err)
	}
	contender, err = AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatalf("successful retry did not release persistence authority: %v", err)
	}
	defer contender.Close()
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("completed shutdown was not idempotent: %v", err)
	}
	if calls := lifecycle.Calls(); calls != 2 {
		t.Fatalf("completed shutdown re-entered lifecycle: calls=%d", calls)
	}
}

type retryShutdownLifecycle struct {
	mu      sync.Mutex
	results []error
	calls   int
}

func (lifecycle *retryShutdownLifecycle) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (lifecycle *retryShutdownLifecycle) Shutdown(context.Context) error {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls++
	if len(lifecycle.results) == 0 {
		return nil
	}
	result := lifecycle.results[0]
	lifecycle.results = lifecycle.results[1:]
	return result
}

func (lifecycle *retryShutdownLifecycle) Calls() int {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return lifecycle.calls
}
