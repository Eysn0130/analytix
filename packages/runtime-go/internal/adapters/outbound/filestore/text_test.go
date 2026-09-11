package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

func TestReadTextFileDecodesTextAndEncoding(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "utf16.txt")
	if err := os.WriteFile(path, filetoolsapp.EncodeTextBytes("hello", filetoolsapp.TextEncodingUTF16LE), 0o600); err != nil {
		t.Fatalf("write text: %v", err)
	}
	result, err := ReadTextFile(path)
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	if result.Content != "hello" || result.Encoding != filetoolsapp.TextEncodingUTF16LE {
		t.Fatalf("result = %#v", result)
	}
	if len(result.RawBytes) == 0 {
		t.Fatalf("expected raw bytes")
	}
}

func TestReadTextFileRejectsDirectoriesAndBinary(t *testing.T) {
	root := t.TempDir()
	if _, err := ReadTextFile(root); !errors.Is(err, ErrTextFileIsDirectory) {
		t.Fatalf("directory error = %v", err)
	}
	binaryPath := filepath.Join(root, "binary.bin")
	if err := os.WriteFile(binaryPath, []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if _, err := ReadTextFile(binaryPath); !errors.Is(err, ErrTextFileBinary) {
		t.Fatalf("binary error = %v", err)
	}
}

func TestPrepareAndApplyTextWrite(t *testing.T) {
	workspace := t.TempDir()
	plan, err := PrepareTextWrite(workspace, "nested/a.txt", nil)
	if err != nil {
		t.Fatalf("prepare create: %v", err)
	}
	if plan.RelativePath != "nested/a.txt" || plan.DiffKind != filetoolsapp.DiffCreate || !plan.IncludeDiff {
		t.Fatalf("create plan mismatch: %#v", plan)
	}
	encoded, err := ApplyTextWrite(plan, "hello\n")
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	if string(encoded) != "hello\n" {
		t.Fatalf("encoded content mismatch: %q", encoded)
	}
	plan, err = PrepareTextWrite(workspace, "nested/a.txt", nil)
	if err != nil {
		t.Fatalf("prepare modify: %v", err)
	}
	if plan.Before != "hello\n" || plan.DiffKind != filetoolsapp.DiffModify {
		t.Fatalf("modify plan mismatch: %#v", plan)
	}
	if _, err := PrepareTextWrite(workspace, filepath.Join(t.TempDir(), "escape.txt"), nil); err == nil {
		t.Fatal("absolute path outside workspace should fail")
	}
}

func TestPrepareAndApplyExistingTextMutation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, filetoolsapp.EncodeTextBytes("package main\n", filetoolsapp.TextEncodingUTF8BOM), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:         workspace,
		Path:              "src/main.go",
		RequiredExtension: ".go",
	})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if plan.RelativePath != "src/main.go" || plan.Before != "package main\n" || plan.Encoding != filetoolsapp.TextEncodingUTF8BOM {
		t.Fatalf("plan mismatch: %#v", plan)
	}
	encoded, err := ApplyExistingTextMutation(plan, "package main\n\n")
	if err != nil {
		t.Fatalf("apply mutation: %v", err)
	}
	if string(encoded[:3]) != string([]byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("expected encoding to be preserved, got %v", encoded[:3])
	}
}

func TestPrepareExistingTextMutationReportsStructuredErrors(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	_, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:                 workspace,
		Path:                      "notes.txt",
		RequiredExtension:         ".go",
		UnsupportedExtensionError: "expected go file",
	})
	var operationErr OperationError
	if !errors.As(err, &operationErr) {
		t.Fatalf("expected operation error, got %T %v", err, err)
	}
	if operationErr.Code != "unsupported_file_type" || operationErr.RelativePath != "notes.txt" {
		t.Fatalf("operation error mismatch: %#v", operationErr)
	}
}
