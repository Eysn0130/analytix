package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestPresentFileMetadataPolicyAndNoMutationV1(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	file := filepath.Join(workspace, "actual.bin")
	outside := filepath.Join(external, "external.pdf")
	for _, path := range []string{file, outside} {
		if err := os.WriteFile(path, []byte{0, 1, 255, 0}, 0600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(workspace, "linked.bin")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	aliasLink := filepath.Join(external, "linked.pdf")
	if err := os.Symlink(outside, aliasLink); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.Mkdir(metadata, 0700); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(metadata, "private.json")
	if err := os.WriteFile(private, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := ExternalReadRootAlias(external)
	for _, test := range []struct {
		name, path, mode, wantCode string
		roots, protected           []string
	}{
		{name: "relative", path: "actual.bin", mode: "workspace-write"},
		{name: "absolute", path: file, mode: "workspace-write"},
		{name: "readonly", path: "actual.bin", mode: "read-only"},
		{name: "external_root", path: outside, mode: "workspace-write", roots: []string{external}},
		{name: "external_alias", path: alias + "/external.pdf", mode: "workspace-write", roots: []string{external}},
		{name: "external_escape", path: outside, mode: "workspace-write", wantCode: "workspace_escape"},
		{name: "alias_escape", path: alias + "/../actual.bin", mode: "workspace-write", roots: []string{external}, wantCode: "workspace_escape"},
		{name: "full_external", path: outside, mode: "danger-full-access"},
		{name: "missing", path: "missing.pdf", mode: "workspace-write", wantCode: "not_found"},
		{name: "directory", path: ".", mode: "workspace-write", wantCode: "validation_error"},
		{name: "symlink", path: link, mode: "workspace-write", wantCode: "validation_error"},
		{name: "alias_symlink", path: alias + "/linked.pdf", mode: "workspace-write", roots: []string{external}, wantCode: "validation_error"},
		{name: "host_metadata", path: private, mode: "danger-full-access", wantCode: "host_metadata_protected"},
		{name: "ordinary_protected", path: file, mode: "workspace-write", protected: []string{workspace}, wantCode: "protected_dir"},
		{name: "full_ordinary", path: file, mode: "danger-full-access", protected: []string{workspace}},
		{name: "full_mandatory", path: file, mode: "danger-full-access", protected: []string{MandatoryProtectedRoot(workspace)}, wantCode: "protected_dir"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := PresentFilesInputV1{Context: context.Background(), Workspace: workspace, SandboxMode: test.mode, ReadRoots: test.roots, ProtectedReadDirs: test.protected,
				Receipt: domaintoolresult.ProtectedPresentReceiptV1{Files: []domaintoolresult.ProtectedPresentedFileV1{{Index: 0, Path: test.path}}}}
			targets, blocked := ValidatePresentFilesV1(input)
			if test.wantCode != "" {
				if targets != nil || blocked == nil || blocked["code"] != test.wantCode {
					t.Fatal("path policy failed", blocked)
				}
			} else if blocked != nil || len(targets) != 1 || targets[0].Index != 0 || !filepath.IsAbs(targets[0].Path) {
				t.Fatal("existing regular file was not observed", blocked)
			}
		})
	}
	bytes, err := os.ReadFile(file)
	if err != nil || string(bytes) != string([]byte{0, 1, 255, 0}) {
		t.Fatal("present changed the source file")
	}
}

func TestPresentAllFilesValidationAndCancellationV1(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "first.pdf"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	input := PresentFilesInputV1{Context: context.Background(), Workspace: workspace, SandboxMode: "workspace-write",
		Receipt: domaintoolresult.ProtectedPresentReceiptV1{Files: []domaintoolresult.ProtectedPresentedFileV1{{Index: 0, Path: "first.pdf"}, {Index: 1, Path: "missing.pdf"}}}}
	if targets, blocked := ValidatePresentFilesV1(input); targets != nil || blocked == nil {
		t.Fatal("partial declaration succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input.Context = ctx
	if targets, blocked := ValidatePresentFilesV1(input); targets != nil || blocked["code"] != "tool_cancelled" {
		t.Fatal("canceled declaration succeeded")
	}
}
