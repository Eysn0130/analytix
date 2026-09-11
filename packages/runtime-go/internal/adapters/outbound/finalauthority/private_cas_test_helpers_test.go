package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func openTestSecurePrivateCAS(t *testing.T, root string, maxBytes int) (*SecurePrivateCAS, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, maxBytes, access)
	if err == nil {
		t.Cleanup(func() { _ = store.Close() })
	}
	return store, err
}

func TestSecurePrivateCASAtRestPostureRemainsPlaintextIntegrityOnly(t *testing.T) {
	if SecurePrivateCASAtRestEncryptionEnabled || SecurePrivateCASAtRestProtectionV1 != "plaintext_integrity_only" {
		t.Fatal("private CAS at-rest posture was misclassified as encrypted")
	}
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1,"plaintextPostureCanary":"synthetic-only"}`)
	digest := domainsecurity.SHA256Hex([]byte("plaintext-at-rest-posture"))
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, digest[:2], digest+".json"))
	if err != nil || !bytes.Equal(raw, body) {
		t.Fatalf("private CAS no longer matches the declared plaintext posture: equal=%t err=%v", bytes.Equal(raw, body), err)
	}
}

func recoverTestSecurePrivateCAS(t *testing.T, root string, maxBytes int) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, maxBytes, access)
	if err != nil {
		t.Fatalf("prepare private CAS recovery: %v", err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate private CAS recovery: %v", err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("apply private CAS recovery: %v", err)
	}
}

type changingPrivateCASAccessAuthority struct {
	inner SecurePrivateCASAccessAuthority
	mu    sync.Mutex
	calls int
}

func (authority *changingPrivateCASAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	return authority.inner.WithPrivateCASAccess(ctx, requestedRoot, func(binding privatecasport.RootBinding) error {
		authority.mu.Lock()
		authority.calls++
		call := authority.calls
		authority.mu.Unlock()
		if call > 1 {
			switch binding.RootIdentity.Kind {
			case privatecasport.DirectoryIdentityUnix:
				binding.RootIdentity.Inode++
			case privatecasport.DirectoryIdentityWindows:
				binding.RootIdentity.FileID[0]++
			}
		}
		return access(binding)
	})
}

func TestPrivateCASEveryAccessRequiresFreshUnchangedBinding(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	inner, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	authority := &changingPrivateCASAccessAuthority{inner: inner}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1}`)
	digest := domainsecurity.SHA256Hex(body)
	if err := store.PutIfAbsent(context.Background(), digest, body); err == nil {
		t.Fatal("private CAS accepted a changed fresh root binding")
	}
	if _, err := os.Lstat(filepath.Join(root, digest[:2], digest+".json")); !os.IsNotExist(err) {
		t.Fatalf("changed binding reached private CAS storage: %v", err)
	}
}

func TestSecurePrivateCASCloseReleasesSharedGenerationAfterLastSibling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("close-shared-generation"))
	body := []byte(`{"record":"close"}`)
	if err := first.PutIfAbsent(context.Background(), digest, body); err != nil {
		_ = sibling.Close()
		_ = first.Close()
		t.Fatal(err)
	}
	generation := first.generation
	key := generation.key
	if sibling.generation != generation {
		t.Fatal("preopened sibling did not share the root generation")
	}
	if err := first.Close(); err != nil {
		_ = sibling.Close()
		t.Fatalf("close first sibling: %v", err)
	}
	if got, err := sibling.Read(context.Background(), digest); err != nil || !equalPrivateCASBytes(got, body) {
		_ = sibling.Close()
		t.Fatalf("remaining sibling lost live generation: body=%q err=%v", got, err)
	}
	if err := sibling.Close(); err != nil {
		t.Fatalf("close last sibling: %v", err)
	}
	privateCASRootGenerations.Lock()
	registered := privateCASRootGenerations.byRoot[key]
	privateCASRootGenerations.Unlock()
	if registered != nil {
		t.Fatal("last sibling close retained the root generation registry entry")
	}
	generation.mu.Lock()
	revoked, refs, shardAnchors := generation.revoked, generation.refs, len(generation.shardAnchors)
	rootAnchorErr := privateCASValidateRootAnchor(generation.rootAnchor, generation.root)
	generation.mu.Unlock()
	if !revoked || refs != 0 || shardAnchors != 0 || rootAnchorErr == nil {
		t.Fatalf(
			"closed generation retained authority: revoked=%v refs=%d shards=%d rootAnchorErr=%v",
			revoked, refs, shardAnchors, rootAnchorErr,
		)
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.generation == generation {
		t.Fatal("reopen reused the closed root generation")
	}
	if got, err := reopened.Read(context.Background(), digest); err != nil || !equalPrivateCASBytes(got, body) {
		t.Fatalf("reopened generation lost committed inventory: body=%q err=%v", got, err)
	}
}

func TestPrivateCASLiveRecoveryBarrierIsContextCancellableAndExclusive(t *testing.T) {
	barrier := newPrivateCASLiveRecoveryBarrier()
	if err := barrier.acquireLive(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := barrier.acquireLive(context.Background()); err != nil {
		barrier.releaseLive()
		t.Fatal(err)
	}
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err := barrier.acquireRecovery(recoveryContext); !errors.Is(err, context.DeadlineExceeded) {
		cancelRecovery()
		barrier.releaseLive()
		barrier.releaseLive()
		t.Fatalf("recovery entered while live operations remained: %v", err)
	}
	cancelRecovery()
	barrier.releaseLive()
	barrier.releaseLive()
	if err := barrier.acquireRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	liveContext, cancelLive := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err := barrier.acquireLive(liveContext); !errors.Is(err, context.DeadlineExceeded) {
		cancelLive()
		barrier.releaseRecovery()
		t.Fatalf("live operation entered an active recovery: %v", err)
	}
	cancelLive()
	barrier.releaseRecovery()
	if err := barrier.acquireLive(context.Background()); err != nil {
		t.Fatal(err)
	}
	barrier.releaseLive()
}

func TestPrivateCASAccessHelperFailsClosedWithoutAuthority(t *testing.T) {
	called := false
	err := withPrivateCASAccess(context.Background(), nil, t.TempDir(), func(privatecasport.RootBinding) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("nil private CAS access authority failed open: called=%v err=%v", called, err)
	}
}

func TestSecurePrivateCASRecoveryPreflightDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := PreflightSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority); err != nil {
		t.Fatalf("preflight missing private CAS: %v", err)
	}
	if err := RecoverSecurePrivateCASIfPresent(context.Background(), root, 4096, authority); err != nil {
		t.Fatalf("recover missing private CAS: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing private CAS recovery created its root: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := PreflightSecurePrivateCASRecoveryIfPresent(cancelled, root, 4096, authority); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled private CAS preflight classification = %v", err)
	}
}

func TestOpenExistingSecurePrivateCASDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, present, err := OpenExistingSecurePrivateCASWithAccessAuthorityContext(
		context.Background(), root, 4096, authority,
	)
	if err != nil || present || store != nil {
		t.Fatalf("missing private CAS open result: store=%v present=%v err=%v", store, present, err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("opening a missing private CAS created its root: %v", err)
	}
}

func TestSecurePrivateCASDirectoryPresenceDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "private-authority")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	present, err := SecurePrivateCASDirectoryPresentWithAccessAuthorityContext(context.Background(), root, authority)
	if err != nil || present {
		t.Fatalf("missing private authority presence result: present=%v err=%v", present, err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private authority presence check created its root: %v", err)
	}
}

type blockingPrivateCASAccessAuthority struct {
	lease   *persistencefs.CompositeLease
	entered chan struct{}
	release chan struct{}
}

func (authority *blockingPrivateCASAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	canonicalRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	return authority.lease.WithPrivateCASAccess(ctx, canonicalRoot, func(binding privatecasport.RootBinding) error {
		if authority.entered != nil {
			close(authority.entered)
			<-authority.release
		}
		return access(binding)
	})
}

func TestPrivateCASListHoldsLeaseUntilEnumerationCompletes(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	roots, ok := lease.FrozenRoots()
	if !ok {
		_ = lease.Close()
		t.Fatal("persistence lease did not expose frozen roots")
	}
	authority := &blockingPrivateCASAccessAuthority{lease: lease}
	store, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(roots.DataDir, "private", "authority"), 4096, authority,
	)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	authority.entered = make(chan struct{})
	authority.release = make(chan struct{})
	listDone := make(chan error, 1)
	go func() {
		_, listErr := store.List(context.Background())
		listDone <- listErr
	}()
	select {
	case <-authority.entered:
	case err := <-listDone:
		t.Fatalf("CAS enumeration returned before entering its lease: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("CAS enumeration did not enter its lease")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- lease.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("lease closed before CAS enumeration completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(authority.release)
	if err := <-listDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("closed persistence lease authorized a CAS enumeration")
	}
	takeover, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatalf("lease was not released after the enumeration drained: %v", err)
	}
	if err := takeover.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateCASVisitHoldsLeaseThroughCallbackAndFinalRevalidation(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	roots, ok := lease.FrozenRoots()
	if !ok {
		_ = lease.Close()
		t.Fatal("persistence lease did not expose frozen roots")
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(roots.DataDir, "private", "authority"), 4096, lease,
	)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1}`)
	digest := domainsecurity.SHA256Hex(body)
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	visitDone := make(chan error, 1)
	go func() {
		visitDone <- store.Visit(context.Background(), func(SecurePrivateCASFile) error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case err := <-visitDone:
		t.Fatalf("CAS visit returned before its callback: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("CAS visit did not reach its callback")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- lease.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("lease closed before CAS visit completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-visitDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := store.Visit(context.Background(), func(SecurePrivateCASFile) error { return nil }); err == nil {
		t.Fatal("closed persistence lease authorized a CAS visit")
	}
}
