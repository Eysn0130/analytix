//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestSemanticStageManagedReaderAllowsOrdinaryManagedModes(t *testing.T) {
	root, pinned := semanticStageManagedRootForTest(t)
	defer pinned.Close()
	directory := filepath.Join(root, "ordinary")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	setSemanticStageFixtureMode(t, directory, 0o755)
	path := filepath.Join(directory, "records.jsonl")
	body := []byte("one\ntwo\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	setSemanticStageFixtureMode(t, path, 0o644)
	expected := semanticStageManagedFileStateForTest(t, path)
	read, err := readSemanticStageManagedFile(context.Background(), pinned, filepath.Join("ordinary", "records.jsonl"), expected)
	if err != nil {
		t.Fatalf("ordinary managed modes rejected: %v", err)
	}
	if !bytes.Equal(read, body) {
		t.Fatalf("managed stage read = %q", read)
	}
}

func TestSemanticStageManagedReaderAllowsEmptyOrdinaryFile(t *testing.T) {
	root, pinned := semanticStageManagedRootForTest(t)
	defer pinned.Close()
	path := filepath.Join(root, "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	setSemanticStageFixtureMode(t, path, 0o644)
	read, err := readSemanticStageManagedFile(
		context.Background(), pinned, "empty.jsonl", semanticStageManagedFileStateForTest(t, path),
	)
	if err != nil || len(read) != 0 {
		t.Fatalf("empty managed file read=%q err=%v", read, err)
	}
}

func TestSemanticStageManagedReaderRejectsSymlinkAndHardlink(t *testing.T) {
	t.Run("directory symlink", func(t *testing.T) {
		root, pinned := semanticStageManagedRootForTest(t)
		defer pinned.Close()
		external := filepath.Join(t.TempDir(), "external")
		if err := os.Mkdir(external, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(external, "records.jsonl")
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := readSemanticStageManagedFile(
			context.Background(), pinned, filepath.Join("linked", "records.jsonl"), semanticStageManagedFileStateForTest(t, target),
		); err == nil {
			t.Fatal("directory symlink passed managed stage reader")
		}
	})
	t.Run("file hardlink", func(t *testing.T) {
		root, pinned := semanticStageManagedRootForTest(t)
		defer pinned.Close()
		path := filepath.Join(root, "records.jsonl")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(root, "alias.jsonl")); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		if _, err := readSemanticStageManagedFile(
			context.Background(), pinned, "records.jsonl", semanticStageManagedFileStateForTest(t, path),
		); err == nil {
			t.Fatal("hardlinked file passed managed stage reader")
		}
	})
}

func TestSemanticStageManagedReaderRejectsReplacementAfterOpen(t *testing.T) {
	root, pinned := semanticStageManagedRootForTest(t)
	defer pinned.Close()
	directory := filepath.Join(root, "ordinary")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "records.jsonl")
	body := []byte("same bytes")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	expected := semanticStageManagedFileStateForTest(t, path)
	replaced := false
	_, err := readSemanticStageManagedFileWithHook(
		context.Background(), pinned, filepath.Join("ordinary", "records.jsonl"), expected,
		func(point string) error {
			if point != "before_final_name_check" || replaced {
				return nil
			}
			replaced = true
			retired := filepath.Join(directory, "retired.jsonl")
			if err := os.Rename(path, retired); err != nil {
				return err
			}
			return os.WriteFile(path, body, 0o644)
		},
	)
	if err == nil || !replaced {
		t.Fatalf("leaf replacement result: replaced=%t err=%v", replaced, err)
	}
}

func TestSemanticStageManagedReaderRejectsMismatchAndCancellation(t *testing.T) {
	root, pinned := semanticStageManagedRootForTest(t)
	defer pinned.Close()
	path := filepath.Join(root, "records.jsonl")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	setSemanticStageFixtureMode(t, path, 0o644)
	expected := semanticStageManagedFileStateForTest(t, path)
	expected.Mode = uint32(os.FileMode(0o600))
	if _, err := readSemanticStageManagedFile(context.Background(), pinned, "records.jsonl", expected); err == nil {
		t.Fatal("mode mismatch passed managed stage reader")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readSemanticStageManagedFile(
		cancelled, pinned, "records.jsonl", semanticStageManagedFileStateForTest(t, path),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
}

func TestStartupAuthorityReaderStillRejectsSharedManagedMode(t *testing.T) {
	root, pinned := semanticStageManagedRootForTest(t)
	defer pinned.Close()
	path := filepath.Join(root, "ordinary.jsonl")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	setSemanticStageFixtureMode(t, path, 0o644)
	if _, _, err := pinned.ReadFile("ordinary.jsonl", 1, false); err == nil {
		t.Fatal("private authority reader accepted an ordinary managed mode")
	}
}

func setSemanticStageFixtureMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func semanticStageManagedRootForTest(t *testing.T) (string, *startupPrivateDirectory) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "stage-data")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := canonicalPathWithoutCreate(root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := captureStartupAuthorityRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := secureStartupOpenRootDirectory(authority)
	if err != nil {
		t.Fatal(err)
	}
	return root, pinned
}

func semanticStageManagedFileStateForTest(t *testing.T, path string) domainstartup.SemanticEntryStateV1 {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return domainstartup.SemanticEntryStateV1{
		Type: domainstartup.ManagedEntryTypeFile, Mode: uint32(info.Mode()), Size: info.Size(),
		SHA256: domainsecurity.SHA256Hex(body),
	}
}
