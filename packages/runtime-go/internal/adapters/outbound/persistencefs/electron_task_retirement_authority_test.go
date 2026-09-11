package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestElectronTaskRetirementAuthorityUsesPurposeSeparatedInstallationSignaturesAcrossRestart(t *testing.T) {
	roots, ownerRoot, lease, authority, keyPath := newElectronTaskRetirementAuthorityFixture(t, true)

	if _, err := os.Lstat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authority constructor created a key: %v", err)
	}
	session, err := authority.PrepareNewTransaction(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := session.Identity(context.Background())
	if err != nil || identity.AuthorityAlgorithm != electronTaskRetirementAlgorithm ||
		identity.AuthorityKeyID == "" || identity.RootBindingDigest != rootBindingDigest(roots) {
		t.Fatalf("authority identity = %#v, %v", identity, err)
	}
	message := []byte("canonical plan payload")
	signature, err := session.SignPlanV2(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.VerifyPlanV2(context.Background(), identity.AuthorityKeyID, message, signature); err != nil {
		t.Fatalf("verify plan signature: %v", err)
	}
	if err := session.VerifyProtectionV2(
		context.Background(), identity.AuthorityKeyID, message, signature,
	); err == nil {
		t.Fatal("plan signature was accepted in the protection domain")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}

	restartedLease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedLease.Close() })
	if _, present, err := restartedLease.OpenExistingJournalAuthorityV1(); err != nil || !present {
		t.Fatalf("restarted journal authority = %v, present=%v", err, present)
	}
	restartedAuthority, err := IssueElectronTaskRetirementJournalAuthorityV2(restartedLease, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := restartedAuthority.OpenExisting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restartedIdentity, err := restarted.Identity(context.Background())
	if err != nil || restartedIdentity != identity {
		t.Fatalf("restarted authority identity = %#v, %v; want %#v", restartedIdentity, err, identity)
	}
	if err := restarted.VerifyPlanV2(
		context.Background(), identity.AuthorityKeyID, message, signature,
	); err != nil {
		t.Fatalf("restart did not verify the installation signature: %v", err)
	}
}

func TestElectronTaskRetirementAuthorityNeverBootstrapsWithoutSourceOrFromJournal(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		_, _, lease, authority, keyPath := newElectronTaskRetirementAuthorityFixture(t, false)
		t.Cleanup(func() { _ = lease.Close() })
		if _, err := authority.PrepareNewTransaction(context.Background()); err == nil {
			t.Fatal("missing source bootstrapped a signing key")
		}
		if _, err := os.Lstat(keyPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing source created authority state: %v", err)
		}
	})

	t.Run("existing journal with missing key", func(t *testing.T) {
		_, ownerRoot, lease, authority, keyPath := newElectronTaskRetirementAuthorityFixture(t, true)
		t.Cleanup(func() { _ = lease.Close() })
		if err := os.Mkdir(filepath.Join(ownerRoot, electronTaskRetirementJournalDirectoryV1), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := authority.OpenExisting(context.Background()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing recovery authority = %v", err)
		}
		if _, err := authority.PrepareNewTransaction(context.Background()); err == nil {
			t.Fatal("existing journal bootstrapped a replacement key")
		}
		if _, err := os.Lstat(keyPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("existing journal created authority state: %v", err)
		}
	})
}

func TestElectronTaskRetirementSigningSessionDetectsKeyBodyReplacement(t *testing.T) {
	_, _, lease, authority, keyPath := newElectronTaskRetirementAuthorityFixture(t, true)
	t.Cleanup(func() { _ = lease.Close() })
	session, err := authority.PrepareNewTransaction(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(keyPath)
	if err != nil || len(body) < 2 {
		t.Fatal(err)
	}
	body[len(body)-2] ^= 1
	if err := os.WriteFile(keyPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := session.SignCommitV2(context.Background(), []byte("commit")); err == nil {
		t.Fatal("key body replacement kept a signing session live")
	}
}

func newElectronTaskRetirementAuthorityFixture(
	t *testing.T,
	withTarget bool,
) (
	RootSet,
	string,
	*CompositeLease,
	*ElectronTaskRetirementJournalAuthorityV2,
	string,
) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	managed := t.TempDir()
	dataRoot := filepath.Join(managed, "data")
	durableRoot := filepath.Join(managed, "durable")
	ownerRoot := filepath.Join(managed, "owner")
	for _, path := range []string{dataRoot, durableRoot, ownerRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if withTarget {
		if err := os.WriteFile(
			filepath.Join(ownerRoot, electronTaskRetirementTargetV1), []byte("opaque"), 0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(dataRoot, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.createJournalAuthorityAfterPreflightV1(); err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	authority, err := IssueElectronTaskRetirementJournalAuthorityV2(lease, ownerRoot)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	namespace, ok := lease.FrozenJournalAuthority()
	if !ok {
		_ = lease.Close()
		t.Fatal("journal namespace authority is unavailable")
	}
	keyPath := startupJournalAuthorityPath(filepath.Join(namespace.path(), "journal"))
	return roots, ownerRoot, lease, authority, keyPath
}
