//go:build darwin || linux

package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	"golang.org/x/sys/unix"
)

func TestApplyTextWriteRejectsBeforeDriftWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("prepared"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareTextWrite(workspace, "target.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("drifted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyTextWrite(plan, "replacement"); err == nil || !strings.Contains(err.Error(), ErrAtomicTextBeforeDrift.Error()) {
		t.Fatalf("expected before-drift rejection, got %v", err)
	}
	assertAtomicTextContent(t, target, "drifted")
}

func TestApplyExistingTextMutationRejectsSymlinkSwap(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("prepared"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{Workspace: workspace, Path: "target.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyExistingTextMutation(plan, "replacement"); err == nil || !strings.Contains(err.Error(), ErrAtomicTextUnsafePath.Error()) {
		t.Fatalf("expected no-follow rejection, got %v", err)
	}
	assertAtomicTextContent(t, outside, "outside")
}

func TestAtomicTextReplaceRejectsSymlinkAncestor(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "target.txt")
	if err := os.WriteFile(outsideTarget, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(workspace, "linked")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceText(atomicTextReplaceRequest{
		Path: filepath.Join(linkedParent, "target.txt"), Content: []byte("replacement"),
		ExpectedExists: true, ExpectedHash: digestAtomicText([]byte("outside")),
		PreserveMode: true, DefaultMode: 0o644,
	})
	if err == nil || !strings.Contains(err.Error(), ErrAtomicTextUnsafePath.Error()) {
		t.Fatalf("expected symlink-ancestor rejection, got %v", err)
	}
	assertAtomicTextContent(t, outsideTarget, "outside")
}

func TestAtomicTextReplaceRevalidatesSymlinkSwapInsideMutation(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, Content: []byte("replacement"), ExpectedExists: true,
		ExpectedHash: digestAtomicText([]byte("before")), PreserveMode: true, DefaultMode: 0o644,
	}, &atomicTextTestHooks{AfterInitialValidation: func() {
		if removeErr := os.Remove(target); removeErr != nil {
			t.Fatalf("remove target in hook: %v", removeErr)
		}
		if symlinkErr := os.Symlink(outside, target); symlinkErr != nil {
			t.Fatalf("replace target with symlink in hook: %v", symlinkErr)
		}
	}})
	if err == nil || !strings.Contains(err.Error(), ErrAtomicTextUnsafePath.Error()) {
		t.Fatalf("expected swap rejection, got %v", err)
	}
	assertAtomicTextContent(t, outside, "outside")
	assertNoAtomicTextTemps(t, workspace, "target.txt")
}

func TestAtomicTextReplaceRollsBackLastMomentSymlinkSwap(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, Content: []byte("replacement"), ExpectedExists: true,
		ExpectedHash: digestAtomicText([]byte("before")), PreserveMode: true, DefaultMode: 0o644,
	}, &atomicTextTestHooks{BeforeReplace: func() error {
		if removeErr := os.Remove(target); removeErr != nil {
			t.Fatalf("remove target in replace hook: %v", removeErr)
		}
		if symlinkErr := os.Symlink(outside, target); symlinkErr != nil {
			t.Fatalf("replace target with symlink in replace hook: %v", symlinkErr)
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), ErrAtomicTextBeforeDrift.Error()) {
		t.Fatalf("expected last-moment swap rejection, got %v", err)
	}
	linked, err := os.Lstat(target)
	if err != nil || linked.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original swapped path was not restored: info=%v err=%v", linked, err)
	}
	assertAtomicTextContent(t, outside, "outside")
	assertNoAtomicTextTemps(t, workspace, "target.txt")
}

func TestAtomicTextReplacePreservesExistingMode(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareTextWrite(workspace, "target.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyTextWrite(plan, "after"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	assertAtomicTextContent(t, target, "after")
}

func TestAtomicTextReplaceWriteFailureLeavesNoPartialTarget(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, Content: []byte("replacement"), ExpectedExists: true,
		ExpectedHash: digestAtomicText([]byte("before")), PreserveMode: true, DefaultMode: 0o644,
	}, &atomicTextTestHooks{FailWriteAfter: 3})
	if err == nil || !strings.Contains(err.Error(), "injected atomic text write failure") {
		t.Fatalf("expected injected write failure, got %v", err)
	}
	assertAtomicTextContent(t, target, "before")
	assertNoAtomicTextTemps(t, workspace, "target.txt")
}

func TestAtomicTextReplaceRenameFailureLeavesNoPartialTarget(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, Content: []byte("replacement"), ExpectedExists: true,
		ExpectedHash: digestAtomicText([]byte("before")), PreserveMode: true, DefaultMode: 0o644,
	}, &atomicTextTestHooks{BeforeReplace: func() error { return errors.New("injected rename failure") }})
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("expected injected rename failure, got %v", err)
	}
	assertAtomicTextContent(t, target, "before")
	assertNoAtomicTextTemps(t, workspace, "target.txt")
}

func TestCheckpointRestoreRejectsPostPreflightDrift(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("checkpoint-after"), 0o600); err != nil {
		t.Fatal(err)
	}
	preflight := checkpointapp.ApplyFilePreflight{
		Action: "restore_previous_version", Status: "apply", AbsolutePath: target,
		CurrentHash: checkpointapp.Hash("checkpoint-after"), AfterHash: checkpointapp.Hash("checkpoint-after"),
		BeforeHash: checkpointapp.Hash("checkpoint-before"), TargetContent: "checkpoint-before",
	}
	if err := os.WriteFile(target, []byte("post-preflight-drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyCheckpointFileMutation(preflight); err == nil || !strings.Contains(err.Error(), ErrAtomicTextBeforeDrift.Error()) {
		t.Fatalf("expected checkpoint drift rejection, got %v", err)
	}
	assertAtomicTextContent(t, target, "post-preflight-drift")
}

func TestConditionalDeleteCommitsToPrivateQuarantineAndRecoversIdempotently(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	journalRoot := filepath.Join(root, "private", "file-mutation-quarantine-v1")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "target.txt")
	content := []byte("delete with exact authority")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	request := atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content),
		Delete: true, MutationAuthority: mustConditionalMutationAuthority(t, journalRoot),
	}
	if err := atomicReplaceText(request); err != nil {
		t.Fatalf("conditional delete: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted target still exists: %v", err)
	}
	assertConditionalQuarantineContains(t, journalRoot, content, ".settled-v1-")
	if err := atomicReplaceText(request); err != nil {
		t.Fatalf("committed delete recovery must be idempotent: %v", err)
	}
}

func TestConditionalDeleteRetryCannotDeleteReappearedSameContentTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	content := []byte("same bytes but a distinct replacement inode")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	request := atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content), Delete: true,
		MutationAuthority: mustConditionalMutationAuthority(t, filepath.Join(root, "journal")),
	}
	if err := atomicReplaceText(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplaceText(request); !errors.Is(err, ErrConditionalMutationResidue) {
		t.Fatalf("reappeared same-content target was not rejected: %v", err)
	}
	assertAtomicTextContent(t, target, string(content))
}

func TestConditionalDeleteRejectsUnknownTargetJournalEntryBeforeMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	content := []byte("must remain")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustConditionalMutationAuthority(t, filepath.Join(root, "journal"))
	journal, err := openConditionalUnixJournalDirectory(authority, target)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := unix.Openat(journal, "unknown-entry", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
	if err != nil {
		_ = unix.Close(journal)
		t.Fatal(err)
	}
	_ = unix.Close(unknown)
	if err := unix.Fsync(journal); err != nil {
		_ = unix.Close(journal)
		t.Fatal(err)
	}
	_ = unix.Close(journal)

	err = conditionalDeleteExact(target, digestAtomicText(content), authority, nil)
	if !errors.Is(err, ErrConditionalMutationResidue) {
		t.Fatalf("unknown journal entry was not rejected: %v", err)
	}
	assertAtomicTextContent(t, target, string(content))
}

func TestConditionalDeleteRejectsMultipleSettledEntries(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	journalRoot := filepath.Join(root, "journal")
	content := []byte("single authorized delete")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	request := atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content), Delete: true,
		MutationAuthority: mustConditionalMutationAuthority(t, journalRoot),
	}
	if err := atomicReplaceText(request); err != nil {
		t.Fatal(err)
	}
	settled := findConditionalQuarantineFile(t, journalRoot, ".settled-v1-")
	name := filepath.Base(settled)
	nonce := strings.Repeat("0", 32)
	if strings.HasSuffix(name, nonce) {
		nonce = strings.Repeat("f", 32)
	}
	duplicate := filepath.Join(filepath.Dir(settled), name[:len(name)-32]+nonce)
	if err := os.Link(settled, duplicate); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplaceText(request); !errors.Is(err, ErrConditionalMutationResidue) {
		t.Fatalf("multiple settled records were not rejected: %v", err)
	}
}

func TestConditionalDeleteRollsBackInjectedFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	content := []byte("must survive")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content), Delete: true,
		MutationAuthority: mustConditionalMutationAuthority(t, filepath.Join(root, "private-journal")),
	}, &atomicTextTestHooks{AfterDeleteQuarantine: func() error { return errors.New("injected delete cut") }})
	if err == nil || !strings.Contains(err.Error(), "injected delete cut") {
		t.Fatalf("expected injected delete failure, got %v", err)
	}
	assertAtomicTextContent(t, target, string(content))
}

func TestConditionalDeleteLastMomentExchangeCannotDeleteReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	original := filepath.Join(root, "original.txt")
	content := []byte("authorized original")
	attacker := []byte("attacker replacement")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content), Delete: true,
		MutationAuthority: mustConditionalMutationAuthority(t, filepath.Join(root, "private-journal")),
	}, &atomicTextTestHooks{BeforeReplace: func() error {
		if err := os.Rename(target, original); err != nil {
			t.Fatal(err)
		}
		return os.WriteFile(target, attacker, 0o600)
	}})
	if err == nil || !strings.Contains(err.Error(), ErrAtomicTextBeforeDrift.Error()) {
		t.Fatalf("expected exchange rejection, got %v", err)
	}
	assertAtomicTextContent(t, target, string(attacker))
	assertAtomicTextContent(t, original, string(content))
}

func TestConditionalDeleteRecoversDurablePendingJournal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "workspace", "target.txt")
	journalRoot := filepath.Join(root, "private-journal")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("crash cut content")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	parent, base, missing, err := openAtomicUnixParent(target, false)
	if err != nil || missing {
		t.Fatalf("open target parent: missing=%v err=%v", missing, err)
	}
	defer unix.Close(parent)
	observation, err := observeConditionalUnixAt(parent, base)
	if err != nil {
		t.Fatal(err)
	}
	authority := mustConditionalMutationAuthority(t, journalRoot)
	journal, err := openConditionalUnixJournalDirectory(authority, target)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(journal)
	pending, err := conditionalUnixJournalName(".pending-v1-", observation)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameConditionalUnixNoReplace(parent, base, journal, pending); err != nil {
		t.Fatal(err)
	}
	if err := syncConditionalUnixParents(parent, journal); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplaceText(atomicTextReplaceRequest{
		Path: target, ExpectedExists: true, ExpectedHash: digestAtomicText(content), Delete: true, MutationAuthority: authority,
	}); err != nil {
		t.Fatalf("recover pending journal: %v", err)
	}
	assertConditionalQuarantineContains(t, journalRoot, content, ".settled-v1-")
}

func TestConditionalDeleteRejectsInvalidHashBeforeFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := conditionalDeleteExact(target, strings.Repeat("z", 64), mustConditionalMutationAuthority(t, filepath.Join(root, "journal")), nil)
	if err == nil || !strings.Contains(err.Error(), "hash is invalid") {
		t.Fatalf("invalid hash was accepted: %v", err)
	}
	assertAtomicTextContent(t, target, "unchanged")
}

func TestConditionalDeleteRejectsJournalRootIdentitySwap(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	journalRoot := filepath.Join(root, "journal")
	content := []byte("identity-bound")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustConditionalMutationAuthority(t, journalRoot)
	if err := os.Rename(journalRoot, journalRoot+".displaced"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journalRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	err := conditionalDeleteExact(target, digestAtomicText(content), authority, nil)
	if err == nil || !strings.Contains(err.Error(), "authority identity changed") {
		t.Fatalf("journal root identity swap was accepted: %v", err)
	}
	assertAtomicTextContent(t, target, string(content))
}

func TestConditionalMoveLastMomentExchangeRollsBackWrongSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	original := filepath.Join(root, "original.txt")
	if err := os.WriteFile(source, []byte("authorized source"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination,
		MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations")),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{BeforeRename: func() {
		if err := os.Rename(source, original); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("attacker source"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}
	if err := ApplyMoveRegularFile(plan); err == nil {
		t.Fatal("last-moment source exchange was accepted")
	}
	assertAtomicTextContent(t, source, "attacker source")
	assertAtomicTextContent(t, original, "authorized source")
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong source survived at destination: %v", err)
	}
}

func TestConditionalMoveInjectedPostInstallFailureKeepsOnlyAuthorizedDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("move rollback"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination,
		MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations")),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{AfterRename: func() error { return errors.New("injected move cut") }}
	if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), "injected move cut") {
		t.Fatalf("expected injected move failure, got %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authorized source unexpectedly reappeared: %v", err)
	}
	assertAtomicTextContent(t, destination, "move rollback")
	if err := ApplyMoveRegularFile(plan); err != nil {
		t.Fatalf("completed move retry must be idempotent: %v", err)
	}
}

func TestConditionalMoveWrongInodeNeverEntersPublicDestinationWhenSourceIsOccupied(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	original := filepath.Join(root, "original.txt")
	if err := os.WriteFile(source, []byte("authorized inode"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations"))
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{
		BeforeRename: func() {
			if err := os.Rename(source, original); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("wrong inode"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		AfterSourceRenameBeforeVerify: func() {
			if err := os.WriteFile(source, []byte("source blocker"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	if err := ApplyMoveRegularFile(plan); err == nil {
		t.Fatal("wrong source inode was accepted")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong inode reached public destination: %v", err)
	}
	assertAtomicTextContent(t, source, "source blocker")
	assertAtomicTextContent(t, original, "authorized inode")
}

func TestConditionalMoveResumesExactPrivatePendingAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("resume exact move"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations"))
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	sourceParent, sourceBase, missing, err := openAtomicUnixParent(source, false)
	if err != nil || missing {
		t.Fatalf("open source: missing=%v err=%v", missing, err)
	}
	defer unix.Close(sourceParent)
	journal := openMoveJournalForTest(t, authority, plan)
	defer unix.Close(journal)
	pending, err := conditionalUnixMovePendingName(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pending); err != nil {
		t.Fatal(err)
	}
	if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMoveRegularFile(plan); err != nil {
		t.Fatalf("resume private pending: %v", err)
	}
	if err := ApplyMoveRegularFile(plan); err != nil {
		t.Fatalf("idempotent completed retry: %v", err)
	}
	assertAtomicTextContent(t, destination, "resume exact move")
}

func TestConditionalMoveDestinationRaceLeavesAuthorizedInodePrivate(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	privateRoot := filepath.Join(root, ".private-mutations")
	if err := os.WriteFile(source, []byte("authorized source"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustMoveMutationAuthority(t, privateRoot)
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{BeforeDestinationInstall: func() {
		if err := os.WriteFile(destination, []byte("attacker destination"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}
	if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrConditionalMutationResidue.Error()) {
		t.Fatalf("destination race was accepted: %v", err)
	}
	assertAtomicTextContent(t, destination, "attacker destination")
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authorized source was not isolated privately: %v", err)
	}
	assertConditionalQuarantineContains(t, privateRoot, []byte("authorized source"), ".move-pending-v1-")
}

func TestConditionalMoveSourceReappearanceQuarantinesCreatedDestinationParents(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destinationRoot := filepath.Join(root, "generated")
	destination := filepath.Join(destinationRoot, "nested", "destination.txt")
	privateRoot := filepath.Join(root, ".private-mutations")
	if err := os.WriteFile(source, []byte("authorized source"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority := mustMoveMutationAuthority(t, privateRoot)
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{BeforeDestinationInstall: func() {
		if err := os.WriteFile(source, []byte("source blocker"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}
	if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrConditionalMutationResidue.Error()) {
		t.Fatalf("source reappearance was accepted: %v", err)
	}
	assertAtomicTextContent(t, source, "source blocker")
	if _, err := os.Stat(destinationRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed move left a public destination directory: %v", err)
	}
	assertConditionalQuarantineContains(t, privateRoot, []byte("authorized source"), "destination.txt")
}

func TestConditionalMoveRejectsCorruptMultipleAndABAJournalStates(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "source.txt")
		destination := filepath.Join(root, "destination.txt")
		if err := os.WriteFile(source, []byte("unknown journal"), 0o600); err != nil {
			t.Fatal(err)
		}
		authority := mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations"))
		plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority})
		if err != nil {
			t.Fatal(err)
		}
		plan = bindMovePlanForTest(t, plan)
		journal := openMoveJournalForTest(t, authority, plan)
		fd, err := unix.Openat(journal, "unknown", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_ = unix.Close(fd)
		_ = unix.Close(journal)
		if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrConditionalMutationResidue.Error()) {
			t.Fatalf("unknown journal passed: %v", err)
		}
		assertAtomicTextContent(t, source, "unknown journal")
	})

	t.Run("multiple", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "source.txt")
		destination := filepath.Join(root, "destination.txt")
		if err := os.WriteFile(source, []byte("same bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		authority := mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations"))
		plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority})
		if err != nil {
			t.Fatal(err)
		}
		plan = bindMovePlanForTest(t, plan)
		sourceParent, sourceBase, missing, err := openAtomicUnixParent(source, false)
		if err != nil || missing {
			t.Fatal(err)
		}
		defer unix.Close(sourceParent)
		journal := openMoveJournalForTest(t, authority, plan)
		defer unix.Close(journal)
		first, _ := conditionalUnixMovePendingName(plan)
		second, _ := conditionalUnixMovePendingName(plan)
		if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, first); err != nil {
			t.Fatal(err)
		}
		if err := unix.Linkat(journal, first, journal, second, 0); err != nil {
			t.Fatal(err)
		}
		if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrConditionalMutationResidue.Error()) {
			t.Fatalf("multiple journal entries passed: %v", err)
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("multiple journal state reached destination: %v", err)
		}
	})

	t.Run("same-path-aba", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "source.txt")
		destination := filepath.Join(root, "destination.txt")
		if err := os.WriteFile(source, []byte("same bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		authority := mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations"))
		plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{Workspace: root, SourcePath: source, DestinationPath: destination, MutationAuthority: authority})
		if err != nil {
			t.Fatal(err)
		}
		plan = bindMovePlanForTest(t, plan)
		sourceParent, sourceBase, missing, err := openAtomicUnixParent(source, false)
		if err != nil || missing {
			t.Fatal(err)
		}
		defer unix.Close(sourceParent)
		journal := openMoveJournalForTest(t, authority, plan)
		defer unix.Close(journal)
		pending, _ := conditionalUnixMovePendingName(plan)
		if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pending); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("same bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrConditionalMutationResidue.Error()) {
			t.Fatalf("same-path ABA state passed: %v", err)
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("same-path ABA state reached destination: %v", err)
		}
	})
}

func openMoveJournalForTest(t *testing.T, authority ConditionalMutationAuthority, plan MoveRegularFilePlan) int {
	t.Helper()
	root, err := openConditionalUnixBoundAuthorityRoot(authority)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(root)
	journal, err := openConditionalUnixMoveJournal(root, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ensureConditionalUnixMoveIntent(journal, plan); err != nil {
		_ = unix.Close(journal)
		t.Fatal(err)
	}
	return journal
}

func assertConditionalQuarantineContains(t *testing.T, journalRoot string, want []byte, namePrefix string) {
	t.Helper()
	found := 0
	err := filepath.WalkDir(journalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), namePrefix) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(body) == string(want) {
			found++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("exact quarantine count=%d, want 1", found)
	}
}

func findConditionalQuarantineFile(t *testing.T, journalRoot, namePrefix string) string {
	t.Helper()
	found := ""
	err := filepath.WalkDir(journalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), namePrefix) {
			return nil
		}
		if found != "" {
			return errors.New("multiple matching quarantine files")
		}
		found = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("matching quarantine file not found")
	}
	return found
}

func mustConditionalMutationAuthority(t *testing.T, root string) ConditionalMutationAuthority {
	t.Helper()
	authority, err := OpenConditionalMutationAuthority(root)
	if err != nil {
		t.Fatalf("open conditional mutation authority: %v", err)
	}
	return authority
}

func assertAtomicTextContent(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("content = %q, want %q", body, want)
	}
}

func assertNoAtomicTextTemps(t *testing.T, directory, base string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, "."+base+".analytix-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files leaked: %v", matches)
	}
}
