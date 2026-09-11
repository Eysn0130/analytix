package filestore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSamePathNormalizesEquivalentPaths(t *testing.T) {
	dir := t.TempDir()
	if !SamePath(dir, filepath.Join(dir, ".")) {
		t.Fatalf("expected equivalent paths to match")
	}
	if SamePath(dir, filepath.Join(dir, "child")) {
		t.Fatalf("different paths should not match")
	}
}

func TestIsInsideTempDir(t *testing.T) {
	dir := t.TempDir()
	if !IsInsideTempDir(filepath.Join(dir, "child")) {
		t.Fatalf("temp child should be recognized")
	}
	if IsInsideTempDir(filepath.Dir(os.TempDir())) {
		t.Fatalf("temp parent should not be recognized as inside temp dir")
	}
}

func TestIsDirectoryNotEmptyError(t *testing.T) {
	if !IsDirectoryNotEmptyError(syscall.ENOTEMPTY) {
		t.Fatal("ENOTEMPTY should be recognized")
	}
	if !IsDirectoryNotEmptyError(errors.New("directory not empty")) {
		t.Fatal("portable directory-not-empty text should be recognized")
	}
	if IsDirectoryNotEmptyError(nil) || IsDirectoryNotEmptyError(errors.New("permission denied")) {
		t.Fatal("unrelated errors should not be recognized")
	}
}

func TestCopyMissingDirectoryPreservesExistingAndAllowsReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "keep.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "nested", "keep.txt"), []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyMissingDirectory(source, target, nil); err != nil {
		t.Fatalf("copy missing directory: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "nested", "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "target" {
		t.Fatalf("existing non-empty target should be preserved: %q", data)
	}

	if err := CopyMissingDirectory(source, target, func(relativePath string, sourcePath string, targetPath string) bool {
		return relativePath == filepath.Join("nested", "keep.txt")
	}); err != nil {
		t.Fatalf("replace existing file: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(target, "nested", "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "source" {
		t.Fatalf("existing target should be replaced when callback allows it: %q", data)
	}
}

func TestCopyMissingDirectoryLosslessRejectsDivergentConflictWithoutMutation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "record.json"), []byte(`{"value":"source"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(target, "record.json")
	targetBody := []byte(`{"value":"target"}`)
	if err := os.WriteFile(targetPath, targetBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyMissingDirectoryLossless(source, target, nil); err == nil {
		t.Fatal("divergent legacy merge was accepted")
	}
	after, err := os.ReadFile(targetPath)
	if err != nil || !bytes.Equal(after, targetBody) {
		t.Fatalf("rejected merge mutated target: body=%q err=%v", after, err)
	}
	if _, err := os.Stat(filepath.Join(source, "record.json")); err != nil {
		t.Fatalf("rejected merge removed source: %v", err)
	}
}

func TestPrepareAndApplyMoveRegularFile(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source.txt")
	if err := os.WriteFile(source, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace:         workspace,
		SourcePath:        "source.txt",
		DestinationPath:   filepath.Join("nested", "destination.txt"),
		MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(workspace, ".private-mutations")),
	})
	if err != nil {
		t.Fatalf("prepare move: %v", err)
	}
	plan = bindMovePlanForTest(t, plan)
	if plan.SourceRelativePath != "source.txt" || plan.DestinationRelativePath != filepath.Join("nested", "destination.txt") || plan.BytesMoved != 7 {
		t.Fatalf("plan mismatch: %#v", plan)
	}
	if err := ApplyMoveRegularFile(plan); err != nil {
		t.Fatalf("apply move: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "nested", "destination.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("destination content mismatch: %q", data)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source should be removed, err=%v", err)
	}
}

func TestPrepareMoveRegularFileNoopAndDestinationExists(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source.txt")
	destination := filepath.Join(workspace, "destination.txt")
	if err := os.WriteFile(source, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace:       workspace,
		SourcePath:      "source.txt",
		DestinationPath: "source.txt",
	})
	if err != nil {
		t.Fatalf("prepare noop: %v", err)
	}
	if !plan.Noop {
		t.Fatalf("expected noop plan: %#v", plan)
	}
	_, err = PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace:       workspace,
		SourcePath:      "source.txt",
		DestinationPath: "destination.txt",
	})
	var operationErr OperationError
	if !errors.As(err, &operationErr) || operationErr.Code != "destination_exists" {
		t.Fatalf("expected destination_exists, got %T %v", err, err)
	}
}

func TestMoveRegularFileFailsClosedAcrossDevicesWithoutResidue(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "nested", "destination.txt")
	if err := os.WriteFile(source, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, SandboxMode: "danger-full-access",
		MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(root, ".private-mutations")),
	})
	if err != nil {
		t.Fatalf("prepare cross-filesystem move: %v", err)
	}
	plan = bindMovePlanForTest(t, plan)
	plan.testHooks = &conditionalMoveTestHooks{ForceCrossDevice: true}
	if err := ApplyMoveRegularFile(plan); err == nil || !strings.Contains(err.Error(), ErrAtomicTextUnsupportedPlatform.Error()) {
		t.Fatalf("cross-filesystem move must fail closed: %v", err)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "content" {
		t.Fatalf("failed-closed move changed source: body=%q err=%v", data, err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed-closed move created destination or residue: %v", err)
	}
}

func TestMoveRegularFileMissingAuthorityFailsWithoutCreatingDestinationParent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destinationParent := filepath.Join(root, "missing", "nested")
	destination := filepath.Join(destinationParent, "destination.txt")
	if err := os.WriteFile(source, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: root, SourcePath: source, DestinationPath: destination, SandboxMode: "danger-full-access",
	})
	if err != nil {
		t.Fatalf("prepare missing-parent move: %v", err)
	}
	plan = bindMovePlanForTest(t, plan)
	if err := ApplyMoveRegularFile(plan); err == nil {
		t.Fatal("missing-parent move must fail closed")
	}
	body, readErr := os.ReadFile(source)
	if readErr != nil || string(body) != "content" {
		t.Fatalf("failed-closed move changed source: body=%q err=%v", body, readErr)
	}
	if _, err := os.Stat(destinationParent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed move created destination parent residue: %v", err)
	}
}

func mustMoveMutationAuthority(t *testing.T, root string) ConditionalMutationAuthority {
	t.Helper()
	authority, err := OpenConditionalMutationAuthority(root)
	if err != nil {
		t.Fatalf("open move mutation authority: %v", err)
	}
	if !authority.Available() {
		t.Skip("conditional move authority is unavailable on this platform")
	}
	return authority
}

func bindMovePlanForTest(t *testing.T, plan MoveRegularFilePlan) MoveRegularFilePlan {
	t.Helper()
	body, err := os.ReadFile(plan.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	plan.operationGroupID = digestAtomicText([]byte("test-operation\x00" + plan.SourcePath + "\x00" + plan.DestinationPath))
	plan.operationDigest = digestAtomicText([]byte("test-intent\x00" + plan.operationGroupID))
	plan.sourceAuthority = digestAtomicText([]byte("test-source-authority\x00" + plan.SourcePath))
	plan.sourceRelative = filepath.ToSlash(plan.SourceRelativePath)
	plan.destinationAuthority = digestAtomicText([]byte("test-destination-authority\x00" + plan.DestinationPath))
	plan.destinationRelative = filepath.ToSlash(plan.DestinationRelativePath)
	plan.sourceEncoding = "utf8"
	plan.sourceBytesBase64 = base64.StdEncoding.EncodeToString(body)
	plan.operationBound = true
	return plan
}
