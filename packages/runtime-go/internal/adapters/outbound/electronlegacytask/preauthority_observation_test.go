package electronlegacytask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestPreAuthorityObservationBindsFullTargetAndRejectsUnknownJournalWithoutNamespace(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config")
	ownerRoot := filepath.Join(base, "electron")
	for _, path := range []string{home, config, ownerRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	roots, err := persistencefs.ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(ownerRoot, BackgroundTaskFileV1)
	if err := os.WriteFile(target, []byte("opaque-account-6222020202020202020"), 0o600); err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	before, err := ObserveBeforeJournalAuthorityV2(context.Background(), lease, ownerRoot)
	if err != nil || !before.TargetPresent() || before.JournalPresent() || before.Digest() == "" {
		t.Fatalf("pre-authority observation = %#v, %v", before, err)
	}
	if _, held := lease.FrozenJournalAuthority(); held {
		t.Fatal("pre-authority observation created a journal namespace")
	}
	if err := os.WriteFile(target, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBeforeJournalAuthorityV2(context.Background(), lease, ownerRoot, before); err == nil {
		t.Fatal("target byte change preserved the pre-authority fixed point")
	}

	journal := filepath.Join(ownerRoot, journalDirectoryV1)
	if err := os.Mkdir(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, "unknown.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ObserveBeforeJournalAuthorityV2(context.Background(), lease, ownerRoot); err == nil {
		t.Fatal("unknown Electron journal object passed pre-authority observation")
	}
	if _, err := os.Lstat(filepath.Join(config, "analytix", "startup-authority-v1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed pre-authority observation created namespace state: %v", err)
	}
}
