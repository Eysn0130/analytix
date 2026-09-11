//go:build darwin || linux

package finalauthority

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateNamedRecoveryRemovesMoreThanOneBoundedBatch(t *testing.T) {
	root := privateNamedRecoveryRoot(t)
	authority, err := capturePrivateRootAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	const target = "authority.json"
	for index := 0; index < privateCASScanPageEntries+17; index++ {
		writePrivateNamedRecoveryTemp(t, root, target, index, 0o600)
	}
	if err := secureRecoverPrivateNamedRoot(authority, target, maxAuthorityKeyBytes); err != nil {
		t.Fatalf("recover multiple bounded batches: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("bounded recovery left residue: entries=%v err=%v", entries, err)
	}
}

func TestPrivateNamedRecoveryPreflightsAllPagesBeforeCleanup(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, root, target string)
	}{
		{
			name: "unknown residue",
			setup: func(t *testing.T, root, _ string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, "unknown.bin"), []byte("unknown"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unsafe temp",
			setup: func(t *testing.T, root, target string) {
				t.Helper()
				writePrivateNamedRecoveryTemp(t, root, target, privateCASScanPageEntries+99, 0o644)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := privateNamedRecoveryRoot(t)
			authority, err := capturePrivateRootAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			const target = "authority.json"
			for index := 0; index < privateCASScanPageEntries+17; index++ {
				writePrivateNamedRecoveryTemp(t, root, target, index, 0o600)
			}
			test.setup(t, root, target)
			before, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := secureRecoverPrivateNamedRoot(authority, target, maxAuthorityKeyBytes); err == nil {
				t.Fatal("unsafe later-page state was recovered instead of rejected")
			}
			after, err := os.ReadDir(root)
			if err != nil || len(after) != len(before) {
				t.Fatalf("failed preflight partially cleaned state: before=%d after=%d err=%v", len(before), len(after), err)
			}
		})
	}
}

func TestExistingPrivateAuthorityRejectsLargeInventoryWithoutCleanup(t *testing.T) {
	path := existingEnrolledAuthorityPath(t)
	root := filepath.Dir(path)
	for index := 0; index < privateCASScanPageEntries*2; index++ {
		name := fmt.Sprintf("residue-%04d", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("residue"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	assertExistingAuthorityRejected(t, path)
	after, err := os.ReadDir(root)
	if err != nil || len(after) != len(before) {
		t.Fatalf("bounded exact-one read changed inventory: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

func privateNamedRecoveryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func writePrivateNamedRecoveryTemp(t *testing.T, root, target string, index int, mode os.FileMode) {
	t.Helper()
	name := fmt.Sprintf(".%s-%024x.tmp", target, index+1)
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("residue"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
