//go:build !windows

package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/unix"
)

func TestCaseBindingObserverClassifiesUnsafeFilesystemAuthority(t *testing.T) {
	t.Run("file symlink", func(t *testing.T) {
		workspace := t.TempDir()
		metadata := filepath.Join(workspace, workspaceHostMetadataDir)
		if err := os.Mkdir(metadata, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(metadata, "target.json")
		writeCaseBindingDocumentAt(t, target, workspace, "case_symlink")
		if err := os.Symlink(target, filepath.Join(metadata, caseBindingFileName)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		observation, err := (CaseBindingReader{}).Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
	})

	t.Run("metadata symlink", func(t *testing.T) {
		workspace := t.TempDir()
		target := t.TempDir()
		writeCaseBindingDocumentAt(t, filepath.Join(target, caseBindingFileName), workspace, "case_symlink")
		if err := os.Symlink(target, filepath.Join(workspace, workspaceHostMetadataDir)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		observation, err := (CaseBindingReader{}).Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
	})

	t.Run("hardlink", func(t *testing.T) {
		workspace := t.TempDir()
		writeCaseBindingFixture(t, workspace, map[string]any{
			"version": 1, "workspaceRoot": workspace, "caseId": "case_hardlink", "source": "analytix-data-analysis",
		})
		metadata := filepath.Join(workspace, workspaceHostMetadataDir)
		if err := os.Link(filepath.Join(metadata, caseBindingFileName), filepath.Join(metadata, "alias.json")); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		observation, err := (CaseBindingReader{}).Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
	})

	for _, target := range []string{"metadata", "file"} {
		t.Run("group writable "+target, func(t *testing.T) {
			workspace := t.TempDir()
			writeCaseBindingFixture(t, workspace, map[string]any{
				"version": 1, "workspaceRoot": workspace, "caseId": "case_mode", "source": "analytix-data-analysis",
			})
			path := filepath.Join(workspace, workspaceHostMetadataDir)
			if target == "file" {
				path = filepath.Join(path, caseBindingFileName)
			}
			if err := os.Chmod(path, 0o775); err != nil {
				t.Fatal(err)
			}
			observation, err := (CaseBindingReader{}).Observe(workspace)
			assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
		})
	}

	t.Run("unsafe owner predicate", func(t *testing.T) {
		stat := unix.Stat_t{Mode: unix.S_IFREG | 0o600, Uid: uint32(os.Geteuid()) + 1, Nlink: 1}
		if caseBindingUnixPrivateOwner(stat) {
			t.Fatal("foreign-owned case binding authority was accepted")
		}
	})

	t.Run("non regular file", func(t *testing.T) {
		workspace := t.TempDir()
		path := filepath.Join(workspace, workspaceHostMetadataDir, caseBindingFileName)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		observation, err := (CaseBindingReader{}).Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
	})
}

func TestCaseBindingObserverClassifiesTOCTOUAsUnstable(t *testing.T) {
	workspace := t.TempDir()
	document := map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_swap", "source": "analytix-data-analysis",
	}
	writeCaseBindingFixture(t, workspace, document)
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	swapped := false
	observation, err := observeCaseBinding(workspace, func(stage string) {
		if stage != "after_initial_read" || swapped {
			return
		}
		swapped = true
		if renameErr := os.Rename(metadata, metadata+".old"); renameErr != nil {
			t.Fatalf("rename metadata: %v", renameErr)
		}
		writeCaseBindingFixture(t, workspace, document)
	})
	if !swapped {
		t.Fatal("TOCTOU hook did not run")
	}
	assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateUnstable)
}

func TestReadAnalytixCaseBindingRejectsMetadataRenameSwap(t *testing.T) {
	workspace := t.TempDir()
	document := map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_swap", "source": "analytix-data-analysis",
	}
	writeCaseBindingFixture(t, workspace, document)
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	swapped := false
	_, err := readAnalytixCaseBinding(workspace, func(stage string) {
		if stage != "after_initial_read" || swapped {
			return
		}
		swapped = true
		if renameErr := os.Rename(metadata, metadata+".old"); renameErr != nil {
			t.Fatalf("rename metadata: %v", renameErr)
		}
		writeCaseBindingFixture(t, workspace, document)
	})
	if !swapped || err == nil || !strings.Contains(err.Error(), "changed during read") {
		t.Fatalf("metadata rename swap was accepted: swapped=%v err=%v", swapped, err)
	}
}

func TestCaseBindingUnixTypedPermissionClassification(t *testing.T) {
	err := classifyCaseBindingUnixOpenError(unix.EACCES, true)
	if !errors.Is(err, ErrCaseBindingUnreadable) || errors.Is(err, ErrCaseBindingMissing) {
		t.Fatalf("permission error classification is not typed and exclusive: %v", err)
	}
}

func TestCaseBindingObserverClassifiesUnreadableFile(t *testing.T) {
	workspace := t.TempDir()
	writeCaseBindingFixture(t, workspace, map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_unreadable", "source": "analytix-data-analysis",
	})
	path := filepath.Join(workspace, workspaceHostMetadataDir, caseBindingFileName)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)
	observation, err := (CaseBindingReader{}).Observe(workspace)
	assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateUnreadable)
}
