package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type journalAuthorityPreflightTestOwnerV1 struct {
	mu                sync.Mutex
	digest            string
	validationCount   int
	failValidationAt  int
	blockValidationAt int
	validationEntered chan struct{}
	validationRelease <-chan struct{}
}

func (owner *journalAuthorityPreflightTestOwnerV1) ObserveJournalAuthorityPreflightV1(
	context.Context,
) (string, error) {
	if owner == nil {
		return "", errors.New("test preflight owner is unavailable")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.digest == "" {
		return "", errors.New("test preflight owner is unavailable")
	}
	return owner.digest, nil
}

func (owner *journalAuthorityPreflightTestOwnerV1) ValidateJournalAuthorityPreflightV1(
	_ context.Context,
	expected string,
) error {
	if owner == nil {
		return errors.New("test preflight owner changed")
	}
	owner.mu.Lock()
	owner.validationCount++
	count := owner.validationCount
	digest := owner.digest
	fail := owner.failValidationAt == count
	block := owner.blockValidationAt == count
	entered := owner.validationEntered
	release := owner.validationRelease
	owner.mu.Unlock()
	if block {
		if entered != nil {
			close(entered)
		}
		if release != nil {
			<-release
		}
	}
	if fail || digest == "" || digest != expected {
		return errors.New("test preflight owner changed")
	}
	return nil
}

func (owner *journalAuthorityPreflightTestOwnerV1) setDigest(value string) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.digest = value
}

func TestCompositeOwnerLeaseCreatesNoJournalAuthorityBeforePreparedPreflight(t *testing.T) {
	base, roots, ownerRoot, namespace := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("composite owner lease implicitly created journal authority")
	}
	assertJournalAuthorityPathAbsentV1(t, namespace)

	owner := &journalAuthorityPreflightTestOwnerV1{digest: journalAuthorityPreflightDigestV1("stable")}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	assertJournalAuthorityPathAbsentV1(t, namespace)
	if _, present, err := prepared.BindExistingV1(context.Background()); err != nil || present {
		t.Fatalf("missing authority discovery = present=%v err=%v", present, err)
	}

	homeB := filepath.Join(base, "home-b")
	configB := filepath.Join(base, "config-b")
	for _, path := range []string{homeB, configB} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", homeB)
	t.Setenv("XDG_CONFIG_HOME", configB)
	authority, err := prepared.CreateV1(context.Background())
	if err != nil || authority == nil || authority.path() != namespace {
		t.Fatalf("prepared authority bootstrap = path=%v err=%v", authority, err)
	}
	if _, err := os.Lstat(namespace); err != nil {
		t.Fatalf("frozen authority namespace was not created: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(configB, "analytix")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment retarget received authority state: %v", err)
	}
}

func TestJournalAuthorityBootstrapRejectsChangedOwnerWithoutNamespace(t *testing.T) {
	_, roots, ownerRoot, namespace := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	owner := &journalAuthorityPreflightTestOwnerV1{digest: journalAuthorityPreflightDigestV1("before")}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	owner.setDigest(journalAuthorityPreflightDigestV1("after"))
	if _, err := prepared.CreateV1(context.Background()); err == nil {
		t.Fatal("changed owner authorized journal authority bootstrap")
	}
	assertJournalAuthorityPathAbsentV1(t, namespace)
}

func TestJournalAuthorityCreatePostValidationFailureNeverPublishesCandidate(t *testing.T) {
	_, roots, ownerRoot, namespace := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	owner := &journalAuthorityPreflightTestOwnerV1{
		digest: journalAuthorityPreflightDigestV1("stable"), failValidationAt: 3,
	}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.CreateV1(context.Background()); err == nil {
		t.Fatal("post-create owner validation failure authorized a capability")
	}
	if _, err := os.Lstat(namespace); err != nil {
		t.Fatalf("expected only an unauthoritative empty create residue: %v", err)
	}
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("failed post-create validation published its provisional authority")
	}
}

func TestJournalAuthorityBindPostValidationFailureNeverPublishesCandidate(t *testing.T) {
	_, roots, ownerRoot, _ := journalAuthorityPreflightFixtureV1(t)
	if _, err := FreezeJournalNamespaceAuthorityForRoots(roots); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	owner := &journalAuthorityPreflightTestOwnerV1{
		digest: journalAuthorityPreflightDigestV1("stable"), failValidationAt: 3,
	}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, present, err := prepared.BindExistingV1(context.Background()); err == nil || present {
		t.Fatalf("post-bind owner validation failure = present=%v err=%v", present, err)
	}
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("failed post-bind validation published its provisional authority")
	}
}

func TestJournalAuthorityCandidateIsInvisibleDuringPostValidation(t *testing.T) {
	_, roots, ownerRoot, _ := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	owner := &journalAuthorityPreflightTestOwnerV1{
		digest: journalAuthorityPreflightDigestV1("stable"), blockValidationAt: 3,
		validationEntered: entered, validationRelease: release,
	}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	created := make(chan error, 1)
	go func() {
		_, err := prepared.CreateV1(context.Background())
		created <- err
	}()
	<-entered
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("concurrent reader observed a provisional authority")
	}
	close(release)
	if err := <-created; err != nil {
		t.Fatal(err)
	}
	if _, held := lease.FrozenJournalAuthority(); !held {
		t.Fatal("validated authority was not published")
	}
}

func TestJournalAuthorityCommitRejectsSameRootsFromRetargetedNamespace(t *testing.T) {
	base, roots, ownerRoot, namespaceA := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	homeB := filepath.Join(base, "home-b-retarget")
	configB := filepath.Join(base, "config-b")
	for _, path := range []string{homeB, configB} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", homeB)
	t.Setenv("XDG_CONFIG_HOME", configB)
	authorityB, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalPathKey(authorityB.path()) == canonicalPathKey(namespaceA) {
		t.Fatal("test did not retarget the authority namespace")
	}
	if _, err := lease.commitJournalAuthorityCandidateV1(authorityB, nil); err == nil {
		t.Fatal("lease accepted a same-roots authority from a different namespace")
	}
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("retargeted namespace became the lease authority")
	}
}

func TestJournalAuthorityBootstrapNeverAdoptsAppearedNamespace(t *testing.T) {
	_, roots, ownerRoot, namespace := journalAuthorityPreflightFixtureV1(t)
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	owner := &journalAuthorityPreflightTestOwnerV1{digest: journalAuthorityPreflightDigestV1("stable")}
	prepared, err := PrepareJournalAuthorityBootstrapV1(context.Background(), lease, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(namespace, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(namespace, "attacker-sentinel")
	if err := os.WriteFile(sentinel, []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.CreateV1(context.Background()); err == nil {
		t.Fatal("appeared namespace was adopted after preflight")
	}
	body, err := os.ReadFile(sentinel)
	if err != nil || string(body) != "untrusted" {
		t.Fatalf("failed bootstrap changed appeared namespace: body=%q err=%v", body, err)
	}
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("appeared namespace became a frozen journal authority")
	}
}

func journalAuthorityPreflightFixtureV1(t *testing.T) (string, RootSet, string, string) {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config-a")
	ownerRoot := filepath.Join(base, "electron")
	for _, path := range []string{home, config, ownerRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := persistentStartupNamespacePath(roots, false)
	if err != nil {
		t.Fatal(err)
	}
	return base, roots, ownerRoot, namespace
}

func journalAuthorityPreflightDigestV1(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func assertJournalAuthorityPathAbsentV1(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected journal authority path %s: %v", path, err)
	}
}
