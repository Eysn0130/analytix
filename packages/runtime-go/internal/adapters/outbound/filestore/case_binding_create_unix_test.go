//go:build darwin || linux

package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/unix"
)

func newMissingImportWorkspace(t *testing.T) (string, domainsecurity.CaseBindingObservationV1, string) {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	observation, identity, err := (CaseBindingReader{}).ObserveMissingImportWorkspace(workspace)
	if err != nil || observation.State != domainsecurity.CaseBindingStateMissing || !domainsecurity.IsSHA256Hex(identity) {
		t.Fatal("missing workspace observation failed")
	}
	return workspace, observation, identity
}

func TestCreateCaseBindingForMainSelectedImport(t *testing.T) {
	workspace, expected, identity := newMissingImportWorkspace(t)
	reader := CaseBindingReader{}
	created, err := reader.CreateForMainSelectedImport(context.Background(), workspace, expected, identity, "case_import_001", time.Now())
	if err != nil || created.State != domainsecurity.CaseBindingStateValid || created.CaseID != "case_import_001" {
		t.Fatalf("explicit create failed: %v", err)
	}
	reopened, err := (CaseBindingReader{}).Observe(workspace)
	if err != nil || reopened != created {
		t.Fatal("new reader did not recover the exact created binding")
	}
	for name, mode := range map[string]os.FileMode{workspaceHostMetadataDir: 0o700, filepath.Join(workspaceHostMetadataDir, caseBindingFileName): 0o600} {
		info, err := os.Lstat(filepath.Join(workspace, name))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("created metadata is not owner-only")
		}
	}
	if _, err := reader.CreateForMainSelectedImport(context.Background(), workspace, expected, identity, "case_replacement", time.Now()); err == nil {
		t.Fatal("existing binding was overwritten")
	}
	if current, err := reader.Observe(workspace); err != nil || current != created {
		t.Fatal("rejected replacement changed the existing binding")
	}
	if _, _, err := reader.ObserveMissingImportWorkspace(workspace); err == nil {
		t.Fatal("existing binding was offered for creation")
	}
}

func TestCreateCaseBindingRejectsUnsafeExistingTargets(t *testing.T) {
	for _, kind := range []string{"invalid", "symlink", "hardlink", "fifo", "directory", "parent symlink", "parent file", "parent permissions"} {
		t.Run(kind, func(t *testing.T) {
			workspace, expected, identity := newMissingImportWorkspace(t)
			metadata := filepath.Join(workspace, workspaceHostMetadataDir)
			target := filepath.Join(metadata, caseBindingFileName)
			if strings.HasPrefix(kind, "parent") {
				switch kind {
				case "parent symlink":
					if err := os.Symlink(t.TempDir(), metadata); err != nil {
						t.Fatal(err)
					}
				case "parent file":
					if err := os.WriteFile(metadata, []byte("retained"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "parent permissions":
					if err := os.Mkdir(metadata, 0o755); err != nil {
						t.Fatal(err)
					}
					// The task shell may have umask 077; establish the hostile
					// mode explicitly instead of accidentally testing a safe dir.
					if err := os.Chmod(metadata, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				target = metadata
			} else {
				if err := os.Mkdir(metadata, 0o700); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "invalid":
					if err := os.WriteFile(target, []byte("invalid-retained"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), target); err != nil {
						t.Fatal(err)
					}
				case "hardlink":
					other := filepath.Join(metadata, "retained")
					if err := os.WriteFile(other, []byte("retained"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Link(other, target); err != nil {
						t.Fatal(err)
					}
				case "fifo":
					if err := unix.Mkfifo(target, 0o600); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(target, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			_, err = (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace, expected, identity, "case_new", time.Now())
			if err == nil || strings.Contains(err.Error(), workspace) {
				t.Fatal("unsafe target was accepted or raw path leaked")
			}
			after, err := os.Lstat(target)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
				t.Fatal("rejected unsafe target was changed")
			}
		})
	}
}

func TestCreateCaseBindingRejectsStaleWorkspaceConfirmation(t *testing.T) {
	workspace, expected, identity := newMissingImportWorkspace(t)
	if err := os.Rename(workspace, workspace+"-retained"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(workspace+"-retained", workspace) })
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(workspace) })
	current, err := (CaseBindingReader{}).Observe(workspace)
	if err != nil || current != expected {
		t.Fatal("test did not reproduce indistinguishable missing observations")
	}
	if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace, expected, identity, "case_stale", time.Now()); err == nil {
		t.Fatal("replaced workspace was accepted across confirmation window")
	}
	if _, err := os.Lstat(filepath.Join(workspace, workspaceHostMetadataDir)); !os.IsNotExist(err) {
		t.Fatal("stale confirmation wrote to replacement workspace")
	}
}

func TestCreateCaseBindingRejectsRacesAndCancellation(t *testing.T) {
	for _, kind := range []string{"cancel", "metadata replacement", "workspace replacement", "competing file", "post-commit cancellation"} {
		t.Run(kind, func(t *testing.T) {
			workspace, expected, identity := newMissingImportWorkspace(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			metadata := filepath.Join(workspace, workspaceHostMetadataDir)
			called := false
			hook := func(stage string) {
				wanted := "before_commit"
				if kind == "post-commit cancellation" {
					wanted = "after_commit"
				}
				if stage != wanted {
					return
				}
				called = true
				switch kind {
				case "cancel", "post-commit cancellation":
					cancel()
				case "metadata replacement":
					if err := os.Rename(metadata, metadata+"-retained"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(metadata, 0o700); err != nil {
						t.Fatal(err)
					}
				case "workspace replacement":
					if err := os.Rename(workspace, workspace+"-retained"); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Rename(workspace+"-retained", workspace) })
					if err := os.Mkdir(workspace, 0o700); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Remove(workspace) })
				case "competing file":
					if err := os.WriteFile(filepath.Join(metadata, caseBindingFileName), []byte("competing-retained"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			created, err := createCaseBindingForMainSelectedImport(ctx, workspace, expected, identity, "case_race", time.Now(), hook)
			if !called || err == nil || created != (domainsecurity.CaseBindingObservationV1{}) {
				t.Fatal("race returned binding authority")
			}
			if kind == "cancel" || kind == "post-commit cancellation" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation was not preserved")
				}
			}
			if kind == "competing file" {
				body, err := os.ReadFile(filepath.Join(metadata, caseBindingFileName))
				if err != nil || string(body) != "competing-retained" {
					t.Fatal("competing binding was overwritten")
				}
			} else if kind == "post-commit cancellation" {
				current, err := (CaseBindingReader{}).Observe(workspace)
				if err != nil || current.State != domainsecurity.CaseBindingStateValid {
					t.Fatal("committed identity was silently erased")
				}
			} else if _, err := os.Lstat(filepath.Join(metadata, caseBindingFileName)); !os.IsNotExist(err) {
				t.Fatal("pre-commit rejection created a binding")
			}
		})
	}
}

func TestCreateCaseBindingExclusiveConcurrentCommit(t *testing.T) {
	workspace, expected, identity := newMissingImportWorkspace(t)
	if err := os.Mkdir(filepath.Join(workspace, workspaceHostMetadataDir), 0o700); err != nil {
		t.Fatal(err)
	}
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, caseID := range []string{"case_first", "case_second"} {
		go func(caseID string) {
			_, err := createCaseBindingForMainSelectedImport(context.Background(), workspace, expected, identity, caseID, time.Now(), func(stage string) {
				if stage == "before_commit" {
					ready.Done()
					<-start
				}
			})
			results <- err
		}(caseID)
	}
	ready.Wait()
	close(start)
	wins := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("exclusive create winners = %d", wins)
	}
	current, err := (CaseBindingReader{}).Observe(workspace)
	if err != nil || current.State != domainsecurity.CaseBindingStateValid {
		t.Fatal("winning binding is unavailable")
	}
}

func TestCreateCaseBindingRejectsInvalidInputWithoutWriting(t *testing.T) {
	workspace, expected, identity := newMissingImportWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(ctx, workspace, expected, identity, "case_valid", time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request accepted")
	}
	for _, caseID := range []string{"", "abc", "../escape", "case with spaces", domainsecurity.UnboundCaseID} {
		if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace, expected, identity, caseID, time.Now()); err == nil {
			t.Fatal("invalid case ID accepted")
		}
	}
	for _, wrongIdentity := range []string{"", strings.Repeat("0", 64)} {
		if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace, expected, wrongIdentity, "case_valid", time.Now()); err == nil {
			t.Fatal("missing or wrong workspace identity accepted")
		}
	}
	tampered := expected
	tampered.ObservationDigest = strings.Repeat("0", 64)
	if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace, tampered, identity, "case_valid", time.Now()); err == nil {
		t.Fatal("tampered missing observation accepted")
	}
	if _, err := (CaseBindingReader{}).CreateForMainSelectedImport(context.Background(), workspace+string(filepath.Separator)+".", expected, identity, "case_valid", time.Now()); err == nil {
		t.Fatal("noncanonical workspace accepted")
	}
	if _, err := os.Lstat(filepath.Join(workspace, workspaceHostMetadataDir)); !os.IsNotExist(err) {
		t.Fatal("invalid request mutated workspace")
	}
}
