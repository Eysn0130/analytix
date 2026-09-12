//go:build darwin || linux

package securegeneration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"

	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"

	"golang.org/x/sys/unix"
)

func TestSecureGenerationFirstInstallAndUpdateAreExact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	firstResult, err := store.Publish(nil, first)
	if err != nil || firstResult.State != Committed || firstResult.Receipt != first.Receipt() {
		t.Fatalf("first install failed: result=%#v err=%v", firstResult, err)
	}
	assertGenerationPayload(t, filepath.Join(root, "public"), "first")
	assertRootNames(t, root, "public")

	second := generationFixture(t, "second")
	secondResult, err := store.Publish(nil, second)
	if err != nil || secondResult.State != Committed || secondResult.Receipt != second.Receipt() {
		t.Fatalf("update failed: result=%#v err=%v", secondResult, err)
	}
	assertGenerationPayload(t, filepath.Join(root, "public"), "second")
	assertGenerationPayload(t, filepath.Join(root, ".public.previous"), "first")
	assertRootNames(t, root, ".public.previous", "public")

	observation, err := store.Observe(nil)
	if err != nil || !observation.Installed || observation.Current != second.Receipt() || observation.Previous == nil || *observation.Previous != first.Receipt() {
		t.Fatalf("exact observation failed: observation=%#v err=%v", observation, err)
	}

	third := generationFixture(t, "third")
	thirdResult, err := store.Publish(nil, third)
	if err != nil || thirdResult.State != Committed {
		t.Fatalf("third generation failed after predecessor rotation: result=%#v err=%v", thirdResult, err)
	}
	assertGenerationPayload(t, filepath.Join(root, "public"), "third")
	assertGenerationPayload(t, filepath.Join(root, ".public.previous"), "second")
	assertRootNames(t, root, ".public.previous", "public")
}

func TestSecureGenerationPublishExpectedChecksCASBeforeFilesystemEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	second := generationFixture(t, "second")
	third := generationFixture(t, "third")

	if result, err := store.PublishExpected(nil, first, ExpectedCurrent{Kind: ExpectedCurrentAbsent}); err != nil || result.State != Committed {
		t.Fatalf("first CAS publish: result=%#v err=%v", result, err)
	}
	before, err := os.Stat(filepath.Join(root, "public"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		expected ExpectedCurrent
	}{
		{name: "stale absent", expected: ExpectedCurrent{Kind: ExpectedCurrentAbsent}},
		{name: "wrong generation", expected: ExpectedCurrent{Kind: ExpectedCurrentGeneration, GenerationID: third.Receipt().GenerationID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := store.PublishExpected(nil, second, test.expected)
			if result.State != NotCommitted || !errors.Is(err, ErrCurrentGeneration) {
				t.Fatalf("stale CAS result=%#v err=%v", result, err)
			}
			after, statErr := os.Stat(filepath.Join(root, "public"))
			if statErr != nil || !os.SameFile(before, after) {
				t.Fatalf("stale CAS changed current identity: before=%v after=%v err=%v", before, after, statErr)
			}
			assertGenerationPayload(t, filepath.Join(root, "public"), "first")
			assertRootNames(t, root, "public")
		})
	}

	if result, err := store.PublishExpected(nil, second, ExpectedCurrent{
		Kind: ExpectedCurrentGeneration, GenerationID: first.Receipt().GenerationID,
	}); err != nil || result.State != Committed {
		t.Fatalf("matching CAS publish: result=%#v err=%v", result, err)
	}
	if result, err := store.PublishExpected(nil, second, ExpectedCurrent{Kind: ExpectedCurrentAbsent}); err != nil || result.State != Committed {
		t.Fatalf("idempotent CAS publish: result=%#v err=%v", result, err)
	}
}

func TestSecureGenerationPublishExpectedRejectsInvalidExpectation(t *testing.T) {
	store := newGenerationTestStore(t, filepath.Join(t.TempDir(), "generations"))
	prepared := generationFixture(t, "first")
	for _, expected := range []ExpectedCurrent{
		{},
		{Kind: ExpectedCurrentAbsent, GenerationID: prepared.Receipt().GenerationID},
		{Kind: ExpectedCurrentGeneration},
		{Kind: ExpectedCurrentGeneration, GenerationID: "not-a-digest"},
	} {
		if result, err := store.PublishExpected(nil, prepared, expected); result.State != NotCommitted || !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid expectation accepted: %#v result=%#v err=%v", expected, result, err)
		}
	}
}

func TestSecureGenerationOpenExistingNeverCreatesMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if store, err := OpenExisting(root, "public", Limits{}); err == nil || store != nil {
		t.Fatalf("missing existing store opened: store=%#v err=%v", store, err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenExisting created or changed missing root: %v", err)
	}
}

func TestSecureGenerationPublishExpectedUnderPinsParentDescriptorAcrossPathReplacement(t *testing.T) {
	outer := t.TempDir()
	parentPath := filepath.Join(outer, "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	movedPath := filepath.Join(outer, "snapshot-parent-pinned")
	if err := os.Rename(parentPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}

	prepared := generationFixture(t, "pinned")
	result, err := PublishExpectedUnder(
		nil,
		parent,
		"source",
		"current",
		prepared,
		ExpectedCurrent{Kind: ExpectedCurrentAbsent},
		Limits{},
	)
	if err != nil || result.State != Committed || result.Receipt != prepared.Receipt() {
		t.Fatalf("descriptor publication result=%#v err=%v", result, err)
	}
	assertGenerationPayload(t, filepath.Join(movedPath, "source", "current"), "pinned")
	assertRootNames(t, parentPath)

	observed, err := ObserveUnder(nil, parent, "source", "current", Limits{})
	if err != nil || !observed.Installed || observed.Current != prepared.Receipt() {
		t.Fatalf("descriptor observation=%#v err=%v", observed, err)
	}
}

func TestPrivateDirectoryLeaseCreatesPinsAndRemovesExactEmptyDirectory(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	lease, err := CreatePrivateDirectoryLeaseUnder(context.Background(), parent, "native-session-a")
	if err != nil {
		t.Fatalf("create private lease: %v", err)
	}
	duplicate, err := lease.Duplicate()
	if err != nil {
		t.Fatalf("duplicate private lease: %v", err)
	}
	var held unix.Stat_t
	if err := unix.Fstat(int(duplicate.Fd()), &held); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	visible, err := os.Stat(filepath.Join(parentPath, "native-session-a"))
	if err != nil || uint64(held.Ino) != visible.Sys().(*syscall.Stat_t).Ino {
		t.Fatalf("visible lease identity mismatch: stat=%v err=%v", visible, err)
	}
	already, err := lease.RemoveEmpty(context.Background())
	if err != nil || already {
		t.Fatalf("remove exact lease already=%v err=%v", already, err)
	}
	if _, err := os.Lstat(filepath.Join(parentPath, "native-session-a")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed lease still visible: %v", err)
	}
	already, err = lease.RemoveEmpty(context.Background())
	if err != nil || !already {
		t.Fatalf("idempotent lease removal already=%v err=%v", already, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close clean lease: %v", err)
	}
}

func TestPrivateDirectoryLeaseNeverDeletesSameNameReplacement(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	lease, err := CreatePrivateDirectoryLeaseUnder(context.Background(), parent, "native-session-b")
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(parentPath, "native-session-b")
	moved := filepath.Join(parentPath, "native-session-b-moved")
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.RemoveEmpty(context.Background()); !errors.Is(err, ErrCleanupIndeterminate) {
		t.Fatalf("replacement removal error = %v, want indeterminate", err)
	}
	if stat, err := os.Stat(original); err != nil || !stat.IsDir() {
		t.Fatalf("same-name replacement was removed: stat=%v err=%v", stat, err)
	}
	if err := lease.Close(); !errors.Is(err, ErrCleanupIndeterminate) {
		t.Fatalf("close quarantined lease error = %v", err)
	}
}

func TestPrivateDirectoryLeaseRecognizesAlreadyDetachedExactDirectory(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	lease, err := CreatePrivateDirectoryLeaseUnder(context.Background(), parent, "native-session-c")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(parentPath, "native-session-c")); err != nil {
		t.Fatal(err)
	}
	already, err := lease.RemoveEmpty(context.Background())
	if err != nil || !already {
		t.Fatalf("detached lease already=%v err=%v", already, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close detached lease: %v", err)
	}
}

func TestPrivateDirectoryLeaseDetachedCleanupPreservesReplacementContents(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	lease, err := CreatePrivateDirectoryLeaseUnder(context.Background(), parent, "native-session-replaced")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close() })
	original := filepath.Join(parentPath, "native-session-replaced")
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(original, "replacement-payload")
	if err := os.WriteFile(residue, []byte("synthetic replacement must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	var detachedStat unix.Stat_t
	if err := unix.Fstat(int(lease.directory.Fd()), &detachedStat); err != nil {
		t.Fatal(err)
	}
	already, removeErr := lease.RemoveEmpty(context.Background())
	if runtime.GOOS == "linux" {
		if detachedStat.Nlink != 0 || removeErr != nil || !already {
			t.Fatalf("Linux detached cleanup links=%d already=%v err=%v", detachedStat.Nlink, already, removeErr)
		}
	} else {
		// APFS can retain the old directory's link count and F_GETPATH name
		// after unlink. If that name now belongs to a replacement, the existing
		// Darwin proof is indeterminate and must not claim successful cleanup.
		if !errors.Is(removeErr, ErrCleanupIndeterminate) || already {
			t.Fatalf("Darwin ambiguous cleanup did not fail closed: already=%v err=%v", already, removeErr)
		}
	}
	after, err := os.Stat(original)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("replacement directory identity changed: %v", err)
	}
	if body, err := os.ReadFile(residue); err != nil || string(body) != "synthetic replacement must survive" {
		t.Fatalf("replacement contents changed: %v", err)
	}
	closeErr := lease.Close()
	if runtime.GOOS == "linux" && closeErr != nil ||
		runtime.GOOS == "darwin" && !errors.Is(closeErr, ErrCleanupIndeterminate) {
		t.Fatalf("cleanup disposition was not retained by Close: %v", closeErr)
	}
}

func TestPrivateDirectoryLeaseRejectsResidueWithoutMutation(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	lease, err := CreatePrivateDirectoryLeaseUnder(context.Background(), parent, "native-session-d")
	if err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(parentPath, "native-session-d", "unexpected")
	if err := os.WriteFile(residue, []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.RemoveEmpty(context.Background()); !errors.Is(err, ErrResidue) || !errors.Is(err, ErrCleanupIndeterminate) {
		t.Fatalf("residue removal error = %v", err)
	}
	if body, err := os.ReadFile(residue); err != nil || string(body) != "residue" {
		t.Fatalf("residue was mutated: body=%q err=%v", body, err)
	}
	if err := lease.Close(); !errors.Is(err, ErrCleanupIndeterminate) {
		t.Fatalf("close residue lease error = %v", err)
	}
}

func TestSecureGenerationPublishExpectedUnderRejectsCallerPathSyntaxWithoutEffects(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	prepared := generationFixture(t, "blocked")
	for _, rootName := range []string{"", ".hidden", "../escape", "nested/source", domainartifact.InventoryFileNameV1} {
		result, err := PublishExpectedUnder(
			nil, parent, rootName, "current", prepared,
			ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
		)
		if result.State != NotCommitted || !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("rootName=%q result=%#v err=%v", rootName, result, err)
		}
	}
	assertRootNames(t, parentPath)
}

func TestSecureGenerationPublishExpectedUnderValidatesBeforeCreatingRoot(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	valid := generationFixture(t, "valid")
	invalid := domainartifact.PreparedV1{}

	for _, test := range []struct {
		name     string
		prepared domainartifact.PreparedV1
		expected ExpectedCurrent
		limits   Limits
	}{
		{name: "invalid prepared", prepared: invalid, expected: ExpectedCurrent{Kind: ExpectedCurrentAbsent}},
		{name: "invalid expected", prepared: valid, expected: ExpectedCurrent{}},
		{name: "over limit", prepared: valid, expected: ExpectedCurrent{Kind: ExpectedCurrentAbsent}, limits: Limits{MaxFiles: 1, MaxFileBytes: 1, MaxTotalBytes: 1, MaxDepth: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := PublishExpectedUnder(nil, parent, "source", "current", test.prepared, test.expected, test.limits)
			if result.State != NotCommitted || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			assertRootNames(t, parentPath)
		})
	}
}

func TestSecureGenerationPublishExpectedUnderCanceledContextNeverCreatesRoot(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := PublishExpectedUnder(
		ctx, parent, "source", "current", generationFixture(t, "canceled"),
		ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
	)
	if result.State != NotCommitted || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled publication result=%#v err=%v", result, err)
	}
	assertRootNames(t, parentPath)
}

func TestSecureGenerationCreatedRootRollbackRequiresExactEmptyAuthority(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()

	store, err := openStoreUnder(parent, "source", "current", Limits{}, true)
	if err != nil || !rootAuthorityCreatedByCall(store.root) {
		t.Fatalf("created root authority store=%#v err=%v", store, err)
	}
	if err := rollbackCreatedRootAuthority(store.root); err != nil {
		t.Fatalf("rollback exact empty root: %v", err)
	}
	if err := closeRootAuthority(store.root); err != nil {
		t.Fatal(err)
	}
	assertRootNames(t, parentPath)

	store, err = openStoreUnder(parent, "source", "current", Limits{}, true)
	if err != nil || !rootAuthorityCreatedByCall(store.root) {
		t.Fatalf("recreated root authority store=%#v err=%v", store, err)
	}
	if err := os.WriteFile(filepath.Join(parentPath, "source", "untrusted"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rollbackCreatedRootAuthority(store.root); !errors.Is(err, ErrCleanupIndeterminate) ||
		!errors.Is(err, ErrResidue) {
		t.Fatalf("populated root rollback err=%v", err)
	}
	if err := closeRootAuthority(store.root); err != nil {
		t.Fatal(err)
	}
	assertRootNames(t, parentPath, "source")
}

func TestSecureGenerationDiscardExpectedUnderRemovesOnlyExactDisposableGeneration(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	prepared := generationFixture(t, "disposable")
	result, err := PublishExpectedUnder(
		nil, parent, "source", "current", prepared,
		ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
	)
	if err != nil || result.State != Committed || result.Receipt != prepared.Receipt() {
		t.Fatalf("publish disposable result=%#v err=%v", result, err)
	}
	wrong := generationFixture(t, "other").Receipt()
	if err := DiscardExpectedUnder(nil, parent, "source", "current", wrong, Limits{}); !errors.Is(err, ErrCurrentGeneration) {
		t.Fatalf("wrong receipt discard err=%v", err)
	}
	assertGenerationPayload(t, filepath.Join(parentPath, "source", "current"), "disposable")
	if err := DiscardExpectedUnder(nil, parent, "source", "current", prepared.Receipt(), Limits{}); err != nil {
		t.Fatalf("exact discard err=%v", err)
	}
	assertRootNames(t, parentPath)
}

func TestSecureGenerationDiscardExpectedUnderRejectsPredecessorAndCanceledContext(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	first := generationFixture(t, "first")
	firstResult, err := PublishExpectedUnder(
		nil, parent, "source", "current", first,
		ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
	)
	if err != nil || firstResult.State != Committed {
		t.Fatalf("first result=%#v err=%v", firstResult, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := DiscardExpectedUnder(ctx, parent, "source", "current", first.Receipt(), Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled discard err=%v", err)
	}
	assertGenerationPayload(t, filepath.Join(parentPath, "source", "current"), "first")
	second := generationFixture(t, "second")
	secondResult, err := PublishExpectedUnder(
		nil, parent, "source", "current", second,
		ExpectedCurrent{Kind: ExpectedCurrentGeneration, GenerationID: first.Receipt().GenerationID}, Limits{},
	)
	if err != nil || secondResult.State != Committed {
		t.Fatalf("second result=%#v err=%v", secondResult, err)
	}
	if err := DiscardExpectedUnder(nil, parent, "source", "current", second.Receipt(), Limits{}); !errors.Is(err, ErrResidue) {
		t.Fatalf("predecessor discard err=%v", err)
	}
	assertGenerationPayload(t, filepath.Join(parentPath, "source", "current"), "second")
	assertGenerationPayload(t, filepath.Join(parentPath, "source", ".current.previous"), "first")
}

func TestSecureGenerationDiscardCurrentUnderRemovesOnlyCleanCurrentOrEmptyRoot(t *testing.T) {
	t.Run("clean-current", func(t *testing.T) {
		parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
		if err := os.Mkdir(parentPath, 0o700); err != nil {
			t.Fatal(err)
		}
		parent, err := os.Open(parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer parent.Close()
		prepared := generationFixture(t, "reconcile")
		published, err := PublishExpectedUnder(
			nil, parent, "source", "current", prepared,
			ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
		)
		if err != nil || published.State != Committed {
			t.Fatalf("publish result=%#v err=%v", published, err)
		}
		result, err := DiscardCurrentUnder(nil, parent, "source", "current", Limits{})
		if err != nil || !result.Installed || result.Current != prepared.Receipt() ||
			result.ReceiptDigest != domainartifact.DigestBytesV1(prepared.ReceiptBytes()) {
			t.Fatalf("discard result=%#v err=%v", result, err)
		}
		if _, err := os.Lstat(filepath.Join(parentPath, "source")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("discarded source root survived: %v", err)
		}
	})

	t.Run("empty-root", func(t *testing.T) {
		parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
		if err := os.MkdirAll(filepath.Join(parentPath, "source"), 0o700); err != nil {
			t.Fatal(err)
		}
		parent, err := os.Open(parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer parent.Close()
		result, err := DiscardCurrentUnder(nil, parent, "source", "current", Limits{})
		if err != nil || result != (DiscardCurrentResult{}) {
			t.Fatalf("empty discard result=%#v err=%v", result, err)
		}
		if _, err := os.Lstat(filepath.Join(parentPath, "source")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("empty source root survived: %v", err)
		}
	})
}

func TestSecureGenerationDiscardCurrentUnderRejectsResidueWithoutMutation(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	first := generationFixture(t, "first")
	second := generationFixture(t, "second")
	firstResult, err := PublishExpectedUnder(
		nil, parent, "source", "current", first,
		ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
	)
	if err != nil || firstResult.State != Committed {
		t.Fatalf("first publish result=%#v err=%v", firstResult, err)
	}
	secondResult, err := PublishExpectedUnder(
		nil, parent, "source", "current", second,
		ExpectedCurrent{Kind: ExpectedCurrentGeneration, GenerationID: first.Receipt().GenerationID}, Limits{},
	)
	if err != nil || secondResult.State != Committed {
		t.Fatalf("second publish result=%#v err=%v", secondResult, err)
	}
	before := rootEntryNames(t, filepath.Join(parentPath, "source"))
	result, err := DiscardCurrentUnder(nil, parent, "source", "current", Limits{})
	if result != (DiscardCurrentResult{}) || !errors.Is(err, ErrResidue) {
		t.Fatalf("predecessor discard result=%#v err=%v", result, err)
	}
	after := rootEntryNames(t, filepath.Join(parentPath, "source"))
	if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
		t.Fatalf("residue discard mutated root: before=%v after=%v", before, after)
	}
	assertGenerationPayload(t, filepath.Join(parentPath, "source", "current"), "second")
	assertGenerationPayload(t, filepath.Join(parentPath, "source", ".current.previous"), "first")
}

func TestSecureGenerationPublishExpectedUnderGenerationCASNeverCreatesMissingRoot(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	prepared := generationFixture(t, "blocked")
	result, err := PublishExpectedUnder(
		nil,
		parent,
		"source",
		"current",
		prepared,
		ExpectedCurrent{Kind: ExpectedCurrentGeneration, GenerationID: prepared.Receipt().GenerationID},
		Limits{},
	)
	if result.State != NotCommitted || err == nil {
		t.Fatalf("missing generation CAS result=%#v err=%v", result, err)
	}
	assertRootNames(t, parentPath)
}

func TestSecureGenerationFailedPublicationNeverDeletesExistingEmptyRoot(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	rootPath := filepath.Join(parentPath, "source")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	prepared := generationFixture(t, "blocked")
	result, err := PublishExpectedUnder(
		nil,
		parent,
		"source",
		"current",
		prepared,
		ExpectedCurrent{Kind: ExpectedCurrentGeneration, GenerationID: prepared.Receipt().GenerationID},
		Limits{},
	)
	if result.State != NotCommitted || err == nil {
		t.Fatalf("existing empty root publication result=%#v err=%v", result, err)
	}
	assertRootNames(t, parentPath, "source")
	assertRootNames(t, rootPath)
}

func TestSecureGenerationPublishExpectedUnderRejectsUnsafeExistingRoot(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, parentPath string)
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, parentPath string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target")
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(parentPath, "source")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong mode",
			setup: func(t *testing.T, parentPath string) {
				t.Helper()
				root := filepath.Join(parentPath, "source")
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "regular file",
			setup: func(t *testing.T, parentPath string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(parentPath, "source"), []byte("not-a-directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
			if err := os.Mkdir(parentPath, 0o700); err != nil {
				t.Fatal(err)
			}
			test.setup(t, parentPath)
			parent, err := os.Open(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			result, err := PublishExpectedUnder(
				nil, parent, "source", "current", generationFixture(t, "blocked"),
				ExpectedCurrent{Kind: ExpectedCurrentAbsent}, Limits{},
			)
			if result.State != NotCommitted || !errors.Is(err, ErrUnsafeRoot) {
				t.Fatalf("unsafe root result=%#v err=%v", result, err)
			}
		})
	}
}

func TestSecureGenerationPublishesEmptyPayloadFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	prepared, err := domainartifact.PrepareV1([]domainartifact.SourceFileV1{{
		Path: "empty.txt", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Publish(nil, prepared)
	if err != nil || result.State != Committed {
		t.Fatalf("empty payload publish result=%#v err=%v", result, err)
	}
	body, err := os.ReadFile(filepath.Join(root, "public", "empty.txt"))
	if err != nil || len(body) != 0 {
		t.Fatalf("empty payload mismatch body=%q err=%v", body, err)
	}
}

func TestSecureGenerationIdenticalPublishIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	prepared := generationFixture(t, "same")
	if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
		t.Fatalf("first publish result=%#v err=%v", result, err)
	}
	before, err := os.Stat(filepath.Join(root, "public"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Publish(nil, prepared)
	if err != nil || result.State != Committed || result.Receipt != prepared.Receipt() {
		t.Fatalf("idempotent publish result=%#v err=%v", result, err)
	}
	after, err := os.Stat(filepath.Join(root, "public"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("idempotent publish replaced the exact installed generation")
	}
	assertRootNames(t, root, "public")
}

func TestSecureGenerationCanRepublishRetainedPredecessor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	second := generationFixture(t, "second")
	for _, prepared := range []domainartifact.PreparedV1{first, second, first} {
		if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
			t.Fatalf("publish result=%#v err=%v", result, err)
		}
	}
	observation, err := store.Observe(nil)
	if err != nil || observation.Current != first.Receipt() || observation.Previous == nil || *observation.Previous != second.Receipt() {
		t.Fatalf("republished predecessor observation=%#v err=%v", observation, err)
	}
	assertGenerationPayload(t, filepath.Join(root, "public"), "first")
	assertGenerationPayload(t, filepath.Join(root, ".public.previous"), "second")
}

func TestSecureGenerationPostVisibilityFailuresRollbackOldGeneration(t *testing.T) {
	for _, phase := range []string{
		"after_old_to_previous",
		"after_old_parent_sync",
		"after_stage_to_public",
		"after_public_parent_sync",
		"after_visibility_readback",
		"after_final_parent_sync",
		"before_commit_stability_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			first := generationFixture(t, "first")
			if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
				t.Fatalf("setup publish failed: result=%#v err=%v", result, err)
			}
			fault := errors.New("injected post-visibility failure")
			store.faults = &faultPlan{cut: func(current string) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "second"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("failure did not report a verified rollback: result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, observeErr := store.Observe(nil)
			if observeErr != nil || !observation.Installed || observation.Current != first.Receipt() || observation.Previous != nil {
				t.Fatalf("old generation was not restored exactly: observation=%#v err=%v", observation, observeErr)
			}
			assertGenerationPayload(t, filepath.Join(root, "public"), "first")
			assertRootNames(t, root, "public")
		})
	}
}

func TestSecureGenerationPostVisibilityFsyncFailuresRollbackOldGeneration(t *testing.T) {
	for _, phase := range []string{"old_parent", "public_parent", "final_parent"} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			first := generationFixture(t, "first")
			if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
				t.Fatalf("setup publish failed: result=%#v err=%v", result, err)
			}
			fault := errors.New("injected publication fsync failure")
			store.faults = &faultPlan{fsync: func(current string, _ int) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "second"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("fsync failure did not report a verified rollback: result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, observeErr := store.Observe(nil)
			if observeErr != nil || !observation.Installed || observation.Current != first.Receipt() || observation.Previous != nil {
				t.Fatalf("old generation was not restored after fsync failure: observation=%#v err=%v", observation, observeErr)
			}
			assertGenerationPayload(t, filepath.Join(root, "public"), "first")
			assertRootNames(t, root, "public")
		})
	}
}

func TestSecureGenerationFirstInstallFsyncFailuresRestoreAbsence(t *testing.T) {
	for _, phase := range []string{"public_parent", "final_parent"} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			fault := errors.New("injected first-install fsync failure")
			store.faults = &faultPlan{fsync: func(current string, _ int) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "first"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("first-install fsync failure result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, observeErr := store.Observe(nil)
			if observeErr != nil || observation.Installed {
				t.Fatalf("first-install fsync failure did not restore absence: observation=%#v err=%v", observation, observeErr)
			}
			assertRootNames(t, root)
		})
	}
}

func TestSecureGenerationFirstInstallPostVisibilityFailuresRestoreAbsence(t *testing.T) {
	for _, phase := range []string{
		"after_stage_to_public",
		"after_public_parent_sync",
		"after_visibility_readback",
		"after_final_parent_sync",
		"before_commit_stability_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			fault := errors.New("first-install visibility failure")
			store.faults = &faultPlan{cut: func(current string) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "first"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("first-install failure did not roll back: result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, observeErr := store.Observe(nil)
			if observeErr != nil || observation.Installed {
				t.Fatalf("failed first install left a visible generation: observation=%#v err=%v", observation, observeErr)
			}
			assertRootNames(t, root)
		})
	}
}

func TestSecureGenerationCrashCutsRecoverExistingGenerationWithoutGuessing(t *testing.T) {
	for _, phase := range []string{
		"after_journal_sync",
		"after_old_to_previous",
		"after_old_parent_sync",
		"after_stage_to_public",
		"after_public_parent_sync",
		"after_visibility_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			first := generationFixture(t, "first")
			if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
				t.Fatalf("setup publish failed: result=%#v err=%v", result, err)
			}
			store.faults = crashAt(phase)
			result, err := store.Publish(nil, generationFixture(t, "second"))
			if result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("crash cut was not retained as indeterminate: result=%#v err=%v", result, err)
			}
			store.faults = nil
			if _, err := store.Observe(nil); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("journaled crash was not quarantined before recovery: %v", err)
			}
			observation, err := store.Recover(nil)
			if err != nil || !observation.Installed || observation.Current != first.Receipt() || observation.Previous != nil {
				t.Fatalf("recovery did not deterministically restore old generation: observation=%#v err=%v", observation, err)
			}
			assertGenerationPayload(t, filepath.Join(root, "public"), "first")
			assertRootNames(t, root, "public")
		})
	}
}

func TestSecureGenerationCrashCutsRecoverFirstInstallToAbsence(t *testing.T) {
	for _, phase := range []string{
		"after_journal_sync",
		"after_stage_to_public",
		"after_public_parent_sync",
		"after_visibility_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			store.faults = crashAt(phase)
			result, err := store.Publish(nil, generationFixture(t, "first"))
			if result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("first-install crash was not indeterminate: result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, err := store.Recover(nil)
			if err != nil || observation.Installed {
				t.Fatalf("first-install recovery guessed a winner: observation=%#v err=%v", observation, err)
			}
			assertRootNames(t, root)
		})
	}
}

func TestSecureGenerationUnjournaledStageCrashIsQuarantinedWithoutGuessing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	store.faults = crashAt("after_stage_sync")
	result, err := store.Publish(nil, generationFixture(t, "first"))
	if result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
		t.Fatalf("unjournaled stage crash result=%#v err=%v", result, err)
	}
	store.faults = nil
	before := rootEntryNames(t, root)
	if len(before) != 1 || !strings.HasPrefix(before[0], ".public.stage-") {
		t.Fatalf("unexpected pre-journal crash topology: %v", before)
	}
	if _, err := store.Recover(nil); !errors.Is(err, ErrResidue) {
		t.Fatalf("recovery guessed how to dispose an unjournaled stage: %v", err)
	}
	if after := rootEntryNames(t, root); strings.Join(before, "\x00") != strings.Join(after, "\x00") {
		t.Fatalf("failed recovery mutated unjournaled stage: before=%v after=%v", before, after)
	}
}

func TestSecureGenerationCrashAfterJournalRemovalIsAnExactCommittedGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
		t.Fatal(err)
	}
	second := generationFixture(t, "second")
	store.faults = crashAt("after_journal_unlink")
	result, err := store.Publish(nil, second)
	if result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
		t.Fatalf("journal-removal crash result=%#v err=%v", result, err)
	}
	store.faults = nil
	// The new generation and predecessor were both fsynced before journal
	// removal. With no journal, startup verifies this exact committed topology;
	// it does not infer from a partial residue.
	observation, err := store.Recover(nil)
	if err != nil || observation.Current != second.Receipt() || observation.Previous == nil || *observation.Previous != first.Receipt() {
		t.Fatalf("durable clean topology was not verified exactly: observation=%#v err=%v", observation, err)
	}
}

func TestSecureGenerationRetainedPredecessorPruneCrashRecoversDeterministically(t *testing.T) {
	for _, phase := range []string{"before_retained_previous_prune", "after_retained_previous_prune"} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			first := generationFixture(t, "first")
			second := generationFixture(t, "second")
			if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
				t.Fatal(err)
			}
			if result, err := store.Publish(nil, second); err != nil || result.State != Committed {
				t.Fatal(err)
			}
			store.faults = crashAt(phase)
			if result, err := store.Publish(nil, generationFixture(t, "third")); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("retained-prune crash result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, err := store.Recover(nil)
			if err != nil || observation.Current != second.Receipt() || observation.Previous == nil || *observation.Previous != first.Receipt() {
				t.Fatalf("retained-prune recovery did not restore current exactly: observation=%#v err=%v", observation, err)
			}
			assertRootNames(t, root, ".public.previous", "public")
		})
	}
}

func TestSecureGenerationCommittedRetainedGCPartialDeletionResumesFromInventory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	second := generationFixture(t, "second")
	if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
		t.Fatal(err)
	}
	if result, err := store.Publish(nil, second); err != nil || result.State != Committed {
		t.Fatal(err)
	}
	store.faults = crashAt("after_commit_marker_sync")
	if result, err := store.Publish(nil, generationFixture(t, "third")); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
		t.Fatalf("failed to create retained-prune journal: result=%#v err=%v", result, err)
	}
	store.faults = nil
	retainedPath := onlyRetainedPath(t, root)
	// Model a process death after commit while GC deleted one payload but not
	// the journal-bound retained generation metadata.
	if err := os.Remove(filepath.Join(retainedPath, "payload.txt")); err != nil {
		t.Fatal(err)
	}
	observation, err := store.Recover(nil)
	third := generationFixture(t, "third")
	if err != nil || observation.Current != third.Receipt() || observation.Previous == nil || *observation.Previous != second.Receipt() {
		t.Fatalf("partial retained-prune recovery failed: observation=%#v err=%v", observation, err)
	}
	assertRootNames(t, root, ".public.previous", "public")
}

func TestSecureGenerationRecoveryRejectsUnboundRetainedDirectoryWithoutDeletingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	for _, value := range []string{"first", "second"} {
		if result, err := store.Publish(nil, generationFixture(t, value)); err != nil || result.State != Committed {
			t.Fatalf("setup publish result=%#v err=%v", result, err)
		}
	}
	store.faults = crashAt("after_commit_marker_sync")
	if result, err := store.Publish(nil, generationFixture(t, "third")); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
		t.Fatalf("failed to create retained recovery fixture: result=%#v err=%v", result, err)
	}
	store.faults = nil
	unbound := filepath.Join(onlyRetainedPath(t, root), "unbound")
	if err := os.Mkdir(unbound, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Recover(nil); !errors.Is(err, ErrResidue) {
		t.Fatalf("unbound retained directory was not quarantined: %v", err)
	}
	if info, err := os.Stat(unbound); err != nil || !info.IsDir() {
		t.Fatalf("failed recovery deleted unbound residue: info=%v err=%v", info, err)
	}
}

func TestSecureGenerationFinalStabilityRejectsSameInodeSameLengthMutation(t *testing.T) {
	t.Run("pre-commit-restores-full-third-generation-prestate", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		first := generationFixture(t, "first")
		second := generationFixture(t, "second")
		for _, prepared := range []domainartifact.PreparedV1{first, second} {
			if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
				t.Fatalf("setup publish result=%#v err=%v", result, err)
			}
		}
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase == "before_commit_stability_readback" {
				mutateSameInodeSameLength(t, filepath.Join(root, "public", "payload.txt"), "mutate")
			}
			return nil
		}}
		result, err := store.Publish(nil, generationFixture(t, "third!"))
		if result.State == Committed || !errors.Is(err, ErrCommitIndeterminate) {
			t.Fatalf("same-inode mutation was reported committed: result=%#v err=%v", result, err)
		}
		assertGenerationPayload(t, filepath.Join(root, "public"), "second")
		assertGenerationPayload(t, filepath.Join(root, ".public.previous"), "first")
	})

	t.Run("last-return-fence-never-reports-committed-mismatch", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase == "after_last_readback_before_return" {
				mutateSameInodeSameLength(t, filepath.Join(root, "public", "payload.txt"), "mutate")
			}
			return nil
		}}
		result, err := store.Publish(nil, generationFixture(t, "first!"))
		if result.State == Committed || !errors.Is(err, ErrCommitIndeterminate) {
			t.Fatalf("last-fence mutation was reported committed: result=%#v err=%v", result, err)
		}
		store.faults = nil
		if _, observeErr := store.Observe(nil); observeErr == nil {
			t.Fatal("receipt-mismatched committed path was observable as valid")
		}
	})

	t.Run("idempotent-return-uses-the-same-final-fence", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		first := generationFixture(t, "first!")
		if result, err := store.Publish(nil, first); err != nil || result.State != Committed {
			t.Fatalf("setup publish result=%#v err=%v", result, err)
		}
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase == "after_last_readback_before_return" {
				mutateSameInodeSameLength(t, filepath.Join(root, "public", "payload.txt"), "mutate")
			}
			return nil
		}}
		result, err := store.Publish(nil, first)
		if result.State == Committed || !errors.Is(err, ErrCommitIndeterminate) {
			t.Fatalf("idempotent final-fence mutation was reported committed: result=%#v err=%v", result, err)
		}
	})

	t.Run("post-commit-cleanup-error-still-uses-the-final-fence", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		fault := errors.New("post-commit cleanup fault")
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase == "after_commit_marker_sync" {
				mutateSameInodeSameLength(t, filepath.Join(root, "public", "payload.txt"), "mutate")
				return fault
			}
			return nil
		}}
		result, err := store.Publish(nil, generationFixture(t, "first!"))
		if result.State == Committed || !errors.Is(err, ErrCommitIndeterminate) || !errors.Is(err, fault) {
			t.Fatalf("post-commit mismatch was reported committed: result=%#v err=%v", result, err)
		}
	})
}

func TestSecureGenerationThirdPublishOrdinaryFailuresRestoreFullPrestate(t *testing.T) {
	for _, phase := range []string{
		"after_journal_sync", "after_retained_to_third", "after_retained_previous_prune",
		"after_old_to_previous", "after_old_parent_sync", "after_stage_to_public",
		"after_public_parent_sync", "after_visibility_readback", "after_final_parent_sync",
		"before_commit_stability_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root, store, first, second := thirdPublishPrestate(t)
			fault := errors.New("third-generation ordinary failure")
			store.faults = &faultPlan{cut: func(current string) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "third"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("third failure result=%#v err=%v", result, err)
			}
			store.faults = nil
			assertFullPrestate(t, root, store, second, first)
		})
	}
}

func TestSecureGenerationThirdPublishFsyncFailuresRestoreFullPrestate(t *testing.T) {
	for _, phase := range []string{"retained_parent", "old_parent", "public_parent", "final_parent"} {
		t.Run(phase, func(t *testing.T) {
			root, store, first, second := thirdPublishPrestate(t)
			fault := errors.New("third-generation fsync failure")
			store.faults = &faultPlan{fsync: func(current string, _ int) error {
				if current == phase {
					return fault
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "third"))
			if result.State != NotCommitted || !errors.Is(err, fault) {
				t.Fatalf("third fsync failure result=%#v err=%v", result, err)
			}
			store.faults = nil
			assertFullPrestate(t, root, store, second, first)
		})
	}
}

func TestSecureGenerationThirdPublishCrashRecoveryRestoresFullPrestate(t *testing.T) {
	for _, phase := range []string{
		"after_journal_sync", "after_retained_to_third", "after_retained_previous_prune",
		"after_old_to_previous", "after_old_parent_sync", "after_stage_to_public",
		"after_public_parent_sync", "after_visibility_readback", "after_final_parent_sync",
		"before_commit_stability_readback",
	} {
		t.Run(phase, func(t *testing.T) {
			root, store, first, second := thirdPublishPrestate(t)
			store.faults = crashAt(phase)
			if result, err := store.Publish(nil, generationFixture(t, "third")); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("third crash result=%#v err=%v", result, err)
			}
			store.faults = nil
			if _, err := store.Recover(nil); err != nil {
				t.Fatalf("third crash recovery failed: %v", err)
			}
			assertFullPrestate(t, root, store, second, first)
		})
	}
}

func TestSecureGenerationDurableCommitMarkerFinishesThirdGenerationAfterCrash(t *testing.T) {
	for _, phase := range []string{"after_commit_marker_sync", "after_journal_unlink", "after_commit_marker_unlink"} {
		t.Run(phase, func(t *testing.T) {
			root, store, _, second := thirdPublishPrestate(t)
			third := generationFixture(t, "third")
			store.faults = crashAt(phase)
			if result, err := store.Publish(nil, third); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
				t.Fatalf("post-commit crash result=%#v err=%v", result, err)
			}
			store.faults = nil
			observation, err := store.Recover(nil)
			if err != nil || observation.Current != third.Receipt() || observation.Previous == nil || *observation.Previous != second.Receipt() {
				t.Fatalf("durable commit recovery observation=%#v err=%v", observation, err)
			}
			assertRootNames(t, root, ".public.previous", "public")
		})
	}
}

func TestSecureGenerationPublishesExactContractModesAndNativeExecutable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generations")
	priorUmask := unix.Umask(0o777)
	t.Cleanup(func() { unix.Umask(priorUmask) })
	store := newGenerationTestStore(t, root)
	prepared, err := domainartifact.PrepareV1([]domainartifact.SourceFileV1{
		{Path: "data.txt", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte("data")},
		{Path: "bin/native", Type: domainartifact.FileTypeNativeExecutableV1, Mode: domainartifact.NativeExecutableModeV1, Body: []byte("native")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
		t.Fatalf("mode publish result=%#v err=%v", result, err)
	}
	assertMode(t, root, domainartifact.DirectoryModeV1)
	assertMode(t, filepath.Join(root, "public"), domainartifact.DirectoryModeV1)
	assertMode(t, filepath.Join(root, "public", "bin"), domainartifact.DirectoryModeV1)
	assertMode(t, filepath.Join(root, "public", "data.txt"), domainartifact.RegularFileModeV1)
	assertMode(t, filepath.Join(root, "public", "bin", "native"), domainartifact.NativeExecutableModeV1)
	assertMode(t, filepath.Join(root, "public", domainartifact.InventoryFileNameV1), domainartifact.RegularFileModeV1)
	assertMode(t, filepath.Join(root, "public", domainartifact.ReceiptFileNameV1), domainartifact.RegularFileModeV1)
}

func TestSecureGenerationRejectsEverySpecialPermissionBit(t *testing.T) {
	specialBits := []struct {
		name string
		mode uint32
	}{
		{name: "setuid", mode: 0o4000},
		{name: "setgid", mode: 0o2000},
		{name: "sticky", mode: 0o1000},
	}
	surfaces := []struct {
		name string
		path func(string) string
		mode uint32
	}{
		{name: "store-root", path: func(root string) string { return root }, mode: domainartifact.DirectoryModeV1},
		{name: "current-directory", path: func(root string) string { return filepath.Join(root, "public") }, mode: domainartifact.DirectoryModeV1},
		{name: "nested-directory", path: func(root string) string { return filepath.Join(root, "public", "nested") }, mode: domainartifact.DirectoryModeV1},
		{name: "regular-payload", path: func(root string) string { return filepath.Join(root, "public", "payload.txt") }, mode: domainartifact.RegularFileModeV1},
		{name: "inventory-metadata", path: func(root string) string {
			return filepath.Join(root, "public", domainartifact.InventoryFileNameV1)
		}, mode: domainartifact.RegularFileModeV1},
		{name: "receipt-metadata", path: func(root string) string {
			return filepath.Join(root, "public", domainartifact.ReceiptFileNameV1)
		}, mode: domainartifact.RegularFileModeV1},
		{name: "native-executable", path: func(root string) string {
			return filepath.Join(root, "public", "bin", "native")
		}, mode: domainartifact.NativeExecutableModeV1},
	}

	for _, surface := range surfaces {
		for _, special := range specialBits {
			t.Run(surface.name+"-"+special.name, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "generations")
				store := newGenerationTestStore(t, root)
				prepared := generationWithNativeFixture(t)
				if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
					t.Fatalf("setup publish result=%#v err=%v", result, err)
				}
				before := rootEntryNames(t, root)
				path := surface.path(root)
				setExactUnixMode(t, path, surface.mode|special.mode, surface.mode)
				if _, err := store.Observe(nil); err == nil {
					t.Fatalf("%s on %s was accepted", special.name, surface.name)
				}
				after := rootEntryNames(t, root)
				if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
					t.Fatalf("read-only special-mode rejection mutated root: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func TestRemoveDirectoryContentsRejectsEverySpecialPermissionBit(t *testing.T) {
	specialBits := []struct {
		name string
		mode uint32
	}{
		{name: "setuid", mode: 0o4000},
		{name: "setgid", mode: 0o2000},
		{name: "sticky", mode: 0o1000},
	}
	targets := []struct {
		name   string
		create func(*testing.T, string) string
		mode   uint32
	}{
		{name: "cleanup-root", create: func(t *testing.T, root string) string {
			if err := os.WriteFile(filepath.Join(root, "payload.txt"), []byte("payload"), os.FileMode(domainartifact.RegularFileModeV1)); err != nil {
				t.Fatal(err)
			}
			return root
		}, mode: domainartifact.DirectoryModeV1},
		{name: "cleanup-regular-file", create: func(t *testing.T, root string) string {
			path := filepath.Join(root, "payload.txt")
			if err := os.WriteFile(path, []byte("payload"), os.FileMode(domainartifact.RegularFileModeV1)); err != nil {
				t.Fatal(err)
			}
			return path
		}, mode: domainartifact.RegularFileModeV1},
		{name: "cleanup-native-executable", create: func(t *testing.T, root string) string {
			path := filepath.Join(root, "native")
			if err := os.WriteFile(path, []byte("native"), os.FileMode(domainartifact.NativeExecutableModeV1)); err != nil {
				t.Fatal(err)
			}
			return path
		}, mode: domainartifact.NativeExecutableModeV1},
		{name: "cleanup-nested-directory", create: func(t *testing.T, root string) string {
			path := filepath.Join(root, "nested")
			if err := os.Mkdir(path, os.FileMode(domainartifact.DirectoryModeV1)); err != nil {
				t.Fatal(err)
			}
			return path
		}, mode: domainartifact.DirectoryModeV1},
	}

	for _, target := range targets {
		for _, special := range specialBits {
			t.Run(target.name+"-"+special.name, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "cleanup")
				if err := os.Mkdir(root, os.FileMode(domainartifact.DirectoryModeV1)); err != nil {
					t.Fatal(err)
				}
				path := target.create(t, root)
				setExactUnixMode(t, path, target.mode|special.mode, target.mode)
				before := rootEntryNames(t, root)
				directory, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
				if err != nil {
					t.Fatal(err)
				}
				removeErr := removeDirectoryContents(directory)
				closeErr := unix.Close(directory)
				if removeErr == nil {
					t.Fatalf("%s on %s was accepted during cleanup", special.name, target.name)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf("rejected cleanup mutated hostile target: %v", err)
				}
				after := rootEntryNames(t, root)
				if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
					t.Fatalf("rejected cleanup mutated directory: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func TestSecureGenerationRejectsSymlinkHardlinkAndExtraEntries(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(t *testing.T, root string)
	}{
		{name: "payload-symlink", apply: func(t *testing.T, root string) {
			outside := filepath.Join(filepath.Dir(root), "outside")
			if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "public", "payload.txt")
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "payload-hardlink", apply: func(t *testing.T, root string) {
			target := filepath.Join(root, "public", "payload.txt")
			alias := filepath.Join(filepath.Dir(root), "payload-alias")
			if err := os.Link(target, alias); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "extra-entry", apply: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "public", "extra.txt"), []byte("extra"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "extra-empty-directory", apply: func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "public", "unbound"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "payload-wrong-mode", apply: func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "public", "payload.txt"), 0o400); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "metadata-wrong-mode", apply: func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "public", domainartifact.InventoryFileNameV1), 0o400); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory-wrong-mode", apply: func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "public", "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			if result, err := store.Publish(nil, generationFixture(t, "first")); err != nil || result.State != Committed {
				t.Fatal(err)
			}
			mutate.apply(t, root)
			before := rootEntryNames(t, root)
			if _, err := store.Observe(nil); err == nil {
				t.Fatal("unsafe generation was accepted")
			}
			after := rootEntryNames(t, root)
			if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
				t.Fatalf("read-only rejection mutated root: before=%v after=%v", before, after)
			}
		})
	}
}

func TestSecureGenerationRejectsRootSymlinkAndPostOpenRootSwap(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, "public", Limits{}); err == nil {
		t.Fatal("symlink root was accepted")
	}
	ancestorTarget := filepath.Join(parent, "ancestor-target")
	if err := os.Mkdir(ancestorTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(parent, "ancestor-link")
	if err := os.Symlink(ancestorTarget, ancestorLink); err != nil {
		t.Fatal(err)
	}
	redirectedRoot := filepath.Join(ancestorLink, "redirected-generation")
	if _, err := Open(redirectedRoot, "public", Limits{}); !errors.Is(err, ErrUnsafeRoot) {
		t.Fatalf("symlink ancestor was accepted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ancestorTarget, "redirected-generation")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink ancestor rejection created redirected root: %v", err)
	}

	root := filepath.Join(parent, "generations")
	store := newGenerationTestStore(t, root)
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Observe(nil); err == nil {
		t.Fatal("replacement root reused a captured authority")
	}

	modeRoot := filepath.Join(parent, "mode-root")
	modeStore := newGenerationTestStore(t, modeRoot)
	if err := os.Chmod(modeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := modeStore.Observe(nil); err == nil {
		t.Fatal("non-exact root mode was accepted")
	}
}

func TestSecureGenerationMidPublishRootReplacementFailsClosed(t *testing.T) {
	t.Run("before-first-visibility", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "generations")
		moved := filepath.Join(parent, "detached")
		store := newGenerationTestStore(t, root)
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase != "before_stage_to_public" {
				return nil
			}
			if err := os.Rename(root, moved); err != nil {
				t.Fatal(err)
			}
			return os.Mkdir(root, 0o700)
		}}
		result, err := store.Publish(nil, generationFixture(t, "first"))
		if result.State != NotCommitted || !errors.Is(err, ErrUnsafeRoot) {
			t.Fatalf("root replacement result=%#v err=%v", result, err)
		}
		assertRootNames(t, root)
		assertRootNames(t, moved)
	})

	t.Run("after-new-visibility", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "generations")
		moved := filepath.Join(parent, "detached")
		store := newGenerationTestStore(t, root)
		if result, err := store.Publish(nil, generationFixture(t, "first")); err != nil || result.State != Committed {
			t.Fatalf("setup publish result=%#v err=%v", result, err)
		}
		store.faults = &faultPlan{cut: func(phase string) error {
			if phase != "after_public_parent_sync" {
				return nil
			}
			if err := os.Rename(root, moved); err != nil {
				t.Fatal(err)
			}
			return os.Mkdir(root, 0o700)
		}}
		result, err := store.Publish(nil, generationFixture(t, "second"))
		if result.State != NotCommitted || !errors.Is(err, ErrUnsafeRoot) {
			t.Fatalf("post-visibility root replacement result=%#v err=%v", result, err)
		}
		assertRootNames(t, root)
		assertGenerationPayload(t, filepath.Join(moved, "public"), "first")
		assertRootNames(t, moved, "public")
	})
}

func TestSecureGenerationRejectsNonComponentPublicName(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "generations"), "nested/public", Limits{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("multi-component public name was accepted: %v", err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "generations"), " public ", Limits{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-canonical public name was accepted: %v", err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "generations")+" ", "public", Limits{}); err == nil {
		t.Fatal("non-canonical root path was silently rewritten")
	}
}

func TestSecureGenerationRejectsUnknownMultipleAndMalformedJournalResidue(t *testing.T) {
	t.Run("multiple-stage", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		for _, suffix := range []string{strings.Repeat("a", 24), strings.Repeat("b", 24)} {
			if err := os.Mkdir(filepath.Join(root, ".public.stage-"+suffix), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		before := rootEntryNames(t, root)
		if _, err := store.Recover(nil); !errors.Is(err, ErrResidue) {
			t.Fatalf("multiple residues were not rejected: %v", err)
		}
		if after := rootEntryNames(t, root); strings.Join(before, "\x00") != strings.Join(after, "\x00") {
			t.Fatalf("ambiguous recovery mutated residues: before=%v after=%v", before, after)
		}
	})

	t.Run("malformed-journal", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "generations")
		store := newGenerationTestStore(t, root)
		store.faults = crashAt("after_journal_sync")
		if result, err := store.Publish(nil, generationFixture(t, "first")); result.State != Indeterminate || !errors.Is(err, errSimulatedCrash) {
			t.Fatalf("failed to create crash fixture: result=%#v err=%v", result, err)
		}
		store.faults = nil
		journalPath := filepath.Join(root, ".public.journal.v1.json")
		body, err := os.ReadFile(journalPath)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body[:len(body)-1], []byte(`,"unknown":true}`)...)
		if err := os.WriteFile(journalPath, body, 0o600); err != nil {
			t.Fatal(err)
		}
		before := rootEntryNames(t, root)
		if _, err := store.Recover(nil); !errors.Is(err, ErrResidue) {
			t.Fatalf("malformed journal was not rejected: %v", err)
		}
		if after := rootEntryNames(t, root); strings.Join(before, "\x00") != strings.Join(after, "\x00") {
			t.Fatalf("malformed recovery mutated root: before=%v after=%v", before, after)
		}
	})
}

func TestSecureGenerationSwapBackAndStageExtrasNeverBecomeVisible(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(t *testing.T, stage string)
	}{
		{name: "swap-back-mutated-object", apply: func(t *testing.T, stage string) {
			original := filepath.Join(stage, "payload.txt")
			outside := filepath.Join(filepath.Dir(stage), "held-payload")
			if err := os.Rename(original, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(original, []byte("substitute"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(original); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outside, []byte("mutated-original"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(outside, original); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "extra-entry", apply: func(t *testing.T, stage string) {
			if err := os.WriteFile(filepath.Join(stage, "unexpected.txt"), []byte("unexpected"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "generations")
			store := newGenerationTestStore(t, root)
			store.faults = &faultPlan{cut: func(phase string) error {
				if phase == "before_stage_to_public" {
					stage := onlyStagePath(t, root)
					mutate.apply(t, stage)
				}
				return nil
			}}
			result, err := store.Publish(nil, generationFixture(t, "first"))
			if result.State != Indeterminate || !errors.Is(err, ErrCommitIndeterminate) {
				t.Fatalf("tampered exact object did not fail closed: result=%#v err=%v", result, err)
			}
			if _, statErr := os.Lstat(filepath.Join(root, "public")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("tampered stage became public: %v", statErr)
			}
			store.faults = nil
			before := rootEntryNames(t, root)
			if _, recoveryErr := store.Recover(nil); !errors.Is(recoveryErr, ErrResidue) {
				t.Fatalf("tampered crash state was guessed during recovery: %v", recoveryErr)
			}
			if after := rootEntryNames(t, root); strings.Join(before, "\x00") != strings.Join(after, "\x00") {
				t.Fatalf("failed recovery mutated tampered residue: before=%v after=%v", before, after)
			}
		})
	}
}

func newGenerationTestStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := Open(root, "public", Limits{MaxFiles: 32, MaxFileBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxDepth: 8})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func generationFixture(t *testing.T, value string) domainartifact.PreparedV1 {
	t.Helper()
	prepared, err := domainartifact.PrepareV1([]domainartifact.SourceFileV1{
		{Path: "payload.txt", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte(value)},
		{Path: "nested/detail.json", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte(`{"value":"` + value + `"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func generationWithNativeFixture(t *testing.T) domainartifact.PreparedV1 {
	t.Helper()
	prepared, err := domainartifact.PrepareV1([]domainartifact.SourceFileV1{
		{Path: "payload.txt", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte("payload")},
		{Path: "nested/detail.json", Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: []byte(`{"value":"fixture"}`)},
		{Path: "bin/native", Type: domainartifact.FileTypeNativeExecutableV1, Mode: domainartifact.NativeExecutableModeV1, Body: []byte("native")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func setExactUnixMode(t *testing.T, path string, hostile, restore uint32) {
	t.Helper()
	if err := unix.Chmod(path, hostile); err != nil {
		t.Fatalf("chmod %s to %#o: %v", path, hostile, err)
	}
	t.Cleanup(func() { _ = unix.Chmod(path, restore) })
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		t.Fatal(err)
	}
	if actual := uint32(stat.Mode & 0o7777); actual != hostile {
		t.Fatalf("hostile mode fixture %s=%#o, want %#o", path, actual, hostile)
	}
}

func crashAt(phase string) *faultPlan {
	return &faultPlan{cut: func(current string) error {
		if current == phase {
			return errors.Join(errSimulatedCrash, errors.New("crash at "+phase))
		}
		return nil
	}}
}

func thirdPublishPrestate(t *testing.T) (string, *Store, domainartifact.PreparedV1, domainartifact.PreparedV1) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "generations")
	store := newGenerationTestStore(t, root)
	first := generationFixture(t, "first")
	second := generationFixture(t, "second")
	for _, prepared := range []domainartifact.PreparedV1{first, second} {
		if result, err := store.Publish(nil, prepared); err != nil || result.State != Committed {
			t.Fatalf("setup publish result=%#v err=%v", result, err)
		}
	}
	return root, store, first, second
}

func assertFullPrestate(t *testing.T, root string, store *Store, current, previous domainartifact.PreparedV1) {
	t.Helper()
	observation, err := store.Observe(nil)
	if err != nil || observation.Current != current.Receipt() || observation.Previous == nil || *observation.Previous != previous.Receipt() {
		t.Fatalf("full prestate mismatch: observation=%#v err=%v", observation, err)
	}
	assertGenerationPayload(t, filepath.Join(root, "public"), payloadBody(t, current))
	assertGenerationPayload(t, filepath.Join(root, ".public.previous"), payloadBody(t, previous))
	assertRootNames(t, root, ".public.previous", "public")
}

func payloadBody(t *testing.T, prepared domainartifact.PreparedV1) string {
	t.Helper()
	for _, file := range prepared.Files() {
		if file.Path == "payload.txt" {
			return string(file.Body)
		}
	}
	t.Fatal("payload.txt missing from fixture")
	return ""
}

func mutateSameInodeSameLength(t *testing.T, path, replacement string) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(replacement)) != before.Size() {
		t.Fatalf("replacement length=%d, want %d", len(replacement), before.Size())
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte(replacement), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("same-inode mutation write=%v close=%v", writeErr, closeErr)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		t.Fatalf("mutation replaced identity or length: before=%v after=%v err=%v", before, after, err)
	}
}

func assertMode(t *testing.T, path string, expected uint32) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := uint32(info.Mode().Perm()); actual != expected {
		t.Fatalf("%s mode=%#o, want %#o", path, actual, expected)
	}
}

func assertGenerationPayload(t *testing.T, generation, expected string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(generation, "payload.txt"))
	if err != nil || string(body) != expected {
		t.Fatalf("generation payload=%q err=%v, want %q", body, err, expected)
	}
}

func assertRootNames(t *testing.T, root string, expected ...string) {
	t.Helper()
	actual := rootEntryNames(t, root)
	sort.Strings(expected)
	if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("root entries=%v, want %v", actual, expected)
	}
}

func rootEntryNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for index := range entries {
		names[index] = entries[index].Name()
	}
	sort.Strings(names)
	return names
}

func onlyStagePath(t *testing.T, root string) string {
	t.Helper()
	var stage string
	for _, name := range rootEntryNames(t, root) {
		if strings.HasPrefix(name, ".public.stage-") {
			if stage != "" {
				t.Fatal("multiple stage directories")
			}
			stage = filepath.Join(root, name)
		}
	}
	if stage == "" {
		t.Fatal("stage directory is missing")
	}
	return stage
}

func onlyRetainedPath(t *testing.T, root string) string {
	t.Helper()
	var retained string
	for _, name := range rootEntryNames(t, root) {
		if strings.HasPrefix(name, ".public.third-") {
			if retained != "" {
				t.Fatal("multiple retained generation directories")
			}
			retained = filepath.Join(root, name)
		}
	}
	if retained == "" {
		t.Fatal("retained generation directory is missing")
	}
	return retained
}
