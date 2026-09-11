//go:build darwin || linux

package persistencefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredTreeLaterPageSpecialFileCausesZeroCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "retired")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < startupPrivateEntryPage+1; index++ {
		name := filepath.Join(root, fileNameForPrivateTreeTest(index))
		if err := os.WriteFile(name, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "zz-unsafe-link")); err != nil {
		t.Fatal(err)
	}
	root, err := canonicalPathWithoutCreate(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := platformDirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureRemovePrivateTree(root, identity); err == nil {
		t.Fatal("private tree with a later special entry was removed")
	}
	for index := 0; index < startupPrivateEntryPage+1; index++ {
		if _, err := os.Lstat(filepath.Join(root, fileNameForPrivateTreeTest(index))); err != nil {
			t.Fatalf("private tree preflight partially deleted file %d: %v", index, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "zz-unsafe-link")); err != nil {
		t.Fatalf("private tree preflight mutated unsafe residue: %v", err)
	}
}

func TestRetiredTreeDepth65RejectedBeforeMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "retired")
	current := root
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	for depth := 0; depth <= maxPrivateTreeDepth; depth++ {
		current = filepath.Join(current, "d")
		if err := os.Mkdir(current, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	leaf := filepath.Join(current, "leaf.bin")
	if err := os.WriteFile(leaf, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := canonicalPathWithoutCreate(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err = canonicalPathWithoutCreate(current)
	if err != nil {
		t.Fatal(err)
	}
	leaf = filepath.Join(current, "leaf.bin")
	identity, err := platformDirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	var limit StartupResourceLimitError
	if err := secureRemovePrivateTree(root, identity); !errors.As(err, &limit) || limit.Code != "private_tree_depth" {
		t.Fatalf("private tree depth error = %v", err)
	}
	if _, err := os.Lstat(leaf); err != nil {
		t.Fatalf("private tree depth preflight partially deleted the tree: %v", err)
	}
}

func fileNameForPrivateTreeTest(index int) string {
	const digits = "0123456789abcdef"
	name := make([]byte, 8)
	for position := len(name) - 1; position >= 0; position-- {
		name[position] = digits[index&15]
		index >>= 4
	}
	return string(name) + ".bin"
}
