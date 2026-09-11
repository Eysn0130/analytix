package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceHasAnalytixCaseBinding(t *testing.T) {
	workspace := t.TempDir()
	if WorkspaceHasAnalytixCaseBinding(workspace) {
		t.Fatal("empty workspace should not be bound to a case")
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatalf("create analytix metadata dir: %v", err)
	}
	writeCaseBindingFixture(t, workspace, map[string]any{
		"version":       1,
		"workspaceRoot": workspace,
		"caseId":        "case_1234",
		"source":        "analytix-data-analysis",
		"updatedAt":     "2026-07-10T00:00:00Z",
	})
	binding, err := ReadAnalytixCaseBinding(workspace)
	if err != nil {
		t.Fatalf("read case binding: %v", err)
	}
	if binding.CaseID != "case_1234" || binding.WorkspaceRealPath == "" || len(binding.BindingSHA256) != 64 || len(binding.CaseBindingHash) != 64 {
		t.Fatalf("unexpected case binding: %#v", binding)
	}
	if !WorkspaceHasAnalytixCaseBinding(workspace) {
		t.Fatal("workspace with a valid case-project binding should be bound")
	}
}

func TestReadAnalytixCaseBindingFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	otherWorkspace := t.TempDir()
	tests := map[string]map[string]any{
		"missing case": {
			"version": 1, "workspaceRoot": workspace, "source": "analytix-data-analysis",
		},
		"wrong source": {
			"version": 1, "workspaceRoot": workspace, "caseId": "case_1234", "source": "caller-reported",
		},
		"wrong workspace": {
			"version": 1, "workspaceRoot": otherWorkspace, "caseId": "case_1234", "source": "analytix-data-analysis",
		},
		"unknown field": {
			"version": 1, "workspaceRoot": workspace, "caseId": "case_1234", "source": "analytix-data-analysis", "safeToAnswer": true,
		},
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			writeCaseBindingFixture(t, workspace, document)
			if binding, err := ReadAnalytixCaseBinding(workspace); err == nil {
				t.Fatalf("invalid binding must fail closed: %#v", binding)
			}
		})
	}
}

func TestCaseBindingReaderDistinguishesMissingFromInvalid(t *testing.T) {
	workspace := t.TempDir()
	reader := CaseBindingReader{}
	if binding, present, err := reader.ReadOptional(workspace); err != nil || present || binding.CaseID != "" {
		t.Fatalf("missing binding should be an explicit unbound workspace: binding=%#v present=%v err=%v", binding, present, err)
	}
	writeCaseBindingFixture(t, workspace, map[string]any{"version": 1, "workspaceRoot": workspace, "caseId": "case_1234", "source": "caller-reported"})
	if binding, present, err := reader.ReadOptional(workspace); err == nil || !present || binding.CaseID != "" {
		t.Fatalf("present invalid binding must fail closed: binding=%#v present=%v err=%v", binding, present, err)
	}
}

func TestCaseBindingReaderCurrentBindingNeverTreatsMissingAsUnbound(t *testing.T) {
	workspace := t.TempDir()
	reader := CaseBindingReader{}
	if binding, err := reader.ReadCurrentBinding(workspace); err == nil || binding.CaseID != "" {
		t.Fatalf("authority-known current binding must fail closed when missing: binding=%#v err=%v", binding, err)
	}
	writeCaseBindingFixture(t, workspace, map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_1234", "source": "analytix-data-analysis",
	})
	workspaceRealPath, err := reader.WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := reader.ReadCurrentBinding(workspace)
	if err != nil || binding.CaseID != "case_1234" || binding.WorkspaceRealPath != workspaceRealPath {
		t.Fatalf("current binding reader did not return host-resolved authority: binding=%#v err=%v", binding, err)
	}
}

func TestCaseBindingReaderRejectsEmptyWorkspaceInsteadOfUsingProcessCWD(t *testing.T) {
	reader := CaseBindingReader{}
	if path, err := reader.WorkspaceRealPath("  "); err == nil || path != "" {
		t.Fatalf("empty workspace must fail closed, path=%q err=%v", path, err)
	}
	if binding, present, err := reader.ReadOptional(""); err == nil || present || binding.CaseID != "" {
		t.Fatalf("empty workspace must not inherit process cwd authority: binding=%#v present=%v err=%v", binding, present, err)
	}
}

func TestReadAnalytixCaseBindingRejectsSymlinkAndHardlinkAuthority(t *testing.T) {
	t.Run("file symlink", func(t *testing.T) {
		workspace := t.TempDir()
		metadata := filepath.Join(workspace, workspaceHostMetadataDir)
		if err := os.MkdirAll(metadata, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(metadata, "target.json")
		writeCaseBindingDocumentAt(t, target, workspace, "case_symlink")
		if err := os.Symlink(target, filepath.Join(metadata, caseBindingFileName)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := ReadAnalytixCaseBinding(workspace); err == nil {
			t.Fatal("symlinked case binding was accepted")
		}
	})
	t.Run("metadata symlink", func(t *testing.T) {
		workspace := t.TempDir()
		target := t.TempDir()
		writeCaseBindingDocumentAt(t, filepath.Join(target, caseBindingFileName), workspace, "case_dirlink")
		if err := os.Symlink(target, filepath.Join(workspace, workspaceHostMetadataDir)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := ReadAnalytixCaseBinding(workspace); err == nil {
			t.Fatal("symlinked metadata directory was accepted")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		workspace := t.TempDir()
		writeCaseBindingFixture(t, workspace, map[string]any{
			"version": 1, "workspaceRoot": workspace, "caseId": "case_hardlink", "source": "analytix-data-analysis",
		})
		bindingPath := filepath.Join(workspace, workspaceHostMetadataDir, caseBindingFileName)
		if err := os.Link(bindingPath, filepath.Join(workspace, workspaceHostMetadataDir, "binding-alias.json")); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		if _, err := ReadAnalytixCaseBinding(workspace); err == nil || !strings.Contains(err.Error(), "single-link") {
			t.Fatalf("hardlinked case binding was accepted: %v", err)
		}
	})
}

func writeCaseBindingFixture(t *testing.T, workspace string, document map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o755); err != nil {
		t.Fatalf("create analytix metadata dir: %v", err)
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode case binding: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".analytix", "case-project.json"), body, 0o644); err != nil {
		t.Fatalf("write case binding: %v", err)
	}
}

func writeCaseBindingDocumentAt(t *testing.T, path string, workspace string, caseID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": caseID, "source": "analytix-data-analysis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
