// Package workspacetest supplies ordinary workspace inputs for tests that
// publish thread summaries. It does not grant protected-source authority.
package workspacetest

import (
	"crypto/rand"
	"encoding/base32"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// New creates a real, private directory under the existing test temp root.
// Unlike testing.TempDir, its basename cannot join numeric suffixes across
// separators into a phone/account-shaped public workspace identifier.
func New(t testing.TB) string {
	t.Helper()
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		t.Fatal("generate ordinary workspace identity")
	}
	name := base32.NewEncoding("abcdefghijklmnopqrstuvwxyzABCDEF").WithPadding(base32.NoPadding).EncodeToString(identity[:])
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal("resolve ordinary workspace temp root")
	}
	workspace := filepath.Join(parent, "analytix-workspace-"+name)
	if domainsecurity.ContainsProtectedCaseData(workspace) {
		t.Fatal("ordinary workspace temp root contains a restricted identifier")
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal("create ordinary workspace")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(workspace); err != nil {
			t.Error("remove ordinary workspace")
		}
	})
	return workspace
}
