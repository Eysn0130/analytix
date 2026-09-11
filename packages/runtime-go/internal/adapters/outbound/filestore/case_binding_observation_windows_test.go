//go:build windows

package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/windows"
)

func TestCaseBindingWindowsRejectsUnsafeComponentAliases(t *testing.T) {
	for _, component := range []string{
		"",
		".",
		"..",
		"case-project.json:hidden",
		"case-project.json.",
		"case-project.json ",
		"NUL",
		`nested\name`,
	} {
		if caseBindingWindowsSafeComponent(component) {
			t.Fatalf("unsafe Windows component was accepted: %q", component)
		}
	}
	if !caseBindingWindowsSafeComponent(caseBindingFileName) {
		t.Fatal("canonical case-binding file name was rejected")
	}
}

func TestCaseBindingWindowsRejectsAlternateDataStream(t *testing.T) {
	workspace := t.TempDir()
	writeCaseBindingFixture(t, workspace, map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_windows_ads", "source": "analytix-data-analysis",
	})
	bindingPath := filepath.Join(workspace, workspaceHostMetadataDir, caseBindingFileName)
	if err := os.WriteFile(bindingPath+":hidden", []byte("untrusted"), 0o600); err != nil {
		t.Skipf("alternate data streams unavailable: %v", err)
	}
	observation, err := (CaseBindingReader{}).Observe(workspace)
	assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
}

func TestCaseBindingWindowsRejectsCaseAliasedBindingName(t *testing.T) {
	workspace := t.TempDir()
	writeCaseBindingDocumentAt(
		t,
		filepath.Join(workspace, workspaceHostMetadataDir, "Case-Project.json"),
		workspace,
		"case_windows_alias",
	)
	observation, err := (CaseBindingReader{}).Observe(workspace)
	assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
}

func TestCaseBindingWindowsRejectsWorldWritableDACL(t *testing.T) {
	for _, target := range []string{"metadata", "file"} {
		t.Run(target, func(t *testing.T) {
			workspace := t.TempDir()
			writeCaseBindingFixture(t, workspace, map[string]any{
				"version": 1, "workspaceRoot": workspace, "caseId": "case_windows_dacl", "source": "analytix-data-analysis",
			})
			path := filepath.Join(workspace, workspaceHostMetadataDir)
			directory := true
			if target == "file" {
				path = filepath.Join(path, caseBindingFileName)
				directory = false
			}
			setCaseBindingWindowsWorldWritableDACL(t, path, directory)
			observation, err := (CaseBindingReader{}).Observe(workspace)
			assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
		})
	}
}

func TestCaseBindingWindowsRetainedHandlesPreventOrDetectRenameSwap(t *testing.T) {
	workspace := t.TempDir()
	document := map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_windows_swap", "source": "analytix-data-analysis",
	}
	writeCaseBindingFixture(t, workspace, document)
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	var renameErr error
	attempted := false
	binding, err := readAnalytixCaseBinding(workspace, func(stage string) {
		if stage != "after_initial_read" || attempted {
			return
		}
		attempted = true
		renameErr = os.Rename(metadata, metadata+".old")
		if renameErr == nil {
			writeCaseBindingFixture(t, workspace, document)
		}
	})
	if !attempted {
		t.Fatal("Windows TOCTOU hook did not run")
	}
	if renameErr != nil {
		if err != nil || binding.CaseID != "case_windows_swap" {
			t.Fatalf("blocked Windows rename corrupted a stable observation: binding=%#v err=%v rename=%v", binding, err, renameErr)
		}
		if !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) &&
			!errors.Is(renameErr, windows.ERROR_LOCK_VIOLATION) &&
			!strings.Contains(strings.ToLower(renameErr.Error()), "being used") {
			t.Fatalf("Windows metadata rename failed for an unexpected reason: %v", renameErr)
		}
		return
	}
	if err == nil || !errors.Is(err, ErrCaseBindingUnstable) {
		t.Fatalf("successful Windows metadata rename swap was not rejected: binding=%#v err=%v", binding, err)
	}
}

func setCaseBindingWindowsWorldWritableDACL(t *testing.T, path string, directory bool) {
	t.Helper()
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatal("current Windows SID is unavailable")
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;GW;;;WD)",
	)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.WRITE_DAC|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
}
