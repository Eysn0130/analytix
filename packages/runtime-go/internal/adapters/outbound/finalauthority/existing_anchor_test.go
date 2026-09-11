//go:build darwin || linux || windows

package finalauthority

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

var _ finalauthorityport.Authority = (*AnchoredFileAuthority)(nil)

func TestExistingFileAuthorityAnchorOpensExactFrozenIdentity(t *testing.T) {
	fixture := newExistingFileAuthorityAnchorFixture(t)
	authority, err := fixture.anchor.Open(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if authority.KeyID() != fixture.authority.KeyID() || !bytes.Equal(authority.PublicKey(), fixture.authority.PublicKey()) {
		t.Fatal("anchored authority identity differs from the independent enrollment")
	}
	message := []byte("anchored-existing-authority")
	signature, err := authority.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.VerifyTrusted(context.Background(), authority.KeyID(), authority.PublicKey(), message, signature); err != nil {
		t.Fatalf("anchored authority rejected its enrolled signature: %v", err)
	}
}

func TestExistingFileAuthorityAnchorRejectsMissingOrMismatchedIndependentAnchor(t *testing.T) {
	fixture := newExistingFileAuthorityAnchorFixture(t)
	if _, err := NewExistingFileAuthorityAnchor(nil, fixture.authority.KeyID(), fixture.authority.PublicKey()); err == nil {
		t.Fatal("nil startup root authority was accepted")
	}
	if _, err := NewExistingFileAuthorityAnchor(fixture.rootAuthority, fixture.authority.KeyID(), bytes.Repeat([]byte{0x41}, 32)); err == nil {
		t.Fatal("key id and public key mismatch was accepted")
	}

	otherPath := filepath.Join(t.TempDir(), "authority", "other.json")
	other, err := OpenOrCreateFileAuthority(otherPath, false)
	if err != nil {
		t.Fatal(err)
	}
	wrongAnchor, err := NewExistingFileAuthorityAnchor(fixture.rootAuthority, other.KeyID(), other.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongAnchor.Open(fixture.path); err == nil {
		t.Fatal("self-consistent key passed an independently mismatched anchor")
	}
	if _, err := fixture.anchor.Open(otherPath); err == nil {
		t.Fatal("authority path outside the startup-frozen data root was accepted")
	}
}

func TestExistingFileAuthorityAnchorRejectsKeyReplacement(t *testing.T) {
	fixture := newExistingFileAuthorityAnchorFixture(t)
	replacementPath := filepath.Join(t.TempDir(), "authority", "replacement.json")
	replacement, err := OpenOrCreateFileAuthority(replacementPath, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.anchor.Open(fixture.path); err == nil {
		t.Fatalf("replacement key %q passed expected key %q", replacement.KeyID(), fixture.authority.KeyID())
	}
}

func TestAnchoredFileAuthorityRejectsAuthorityPathIdentityReplacement(t *testing.T) {
	fixture := newExistingFileAuthorityAnchorFixture(t)
	anchored, err := fixture.anchor.Open(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := filepath.Dir(fixture.path)
	if err := os.Rename(authorityRoot, authorityRoot+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := anchored.Sign(context.Background(), []byte("must-fail")); err == nil {
		t.Fatal("anchored authority signed after its directory/file identity was replaced with identical key bytes")
	}
}

func TestExistingFileAuthorityAnchorRejectsFrozenRootIdentityReplacement(t *testing.T) {
	fixture := newExistingFileAuthorityAnchorFixture(t)
	anchored, err := fixture.anchor.Open(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fixture.dataRoot, fixture.dataRoot+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fixture.dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.anchor.Open(fixture.path); err == nil {
		t.Fatal("replacement data-root inode/FileID passed the startup-frozen authority")
	}
	if _, err := anchored.Sign(context.Background(), []byte("must-fail")); err == nil {
		t.Fatal("loaded authority signed after startup-frozen root replacement")
	}
}

type existingFileAuthorityAnchorFixture struct {
	dataRoot      string
	path          string
	authority     *FileAuthority
	rootAuthority *persistencefs.RootAuthority
	anchor        *ExistingFileAuthorityAnchor
}

func newExistingFileAuthorityAnchorFixture(t *testing.T) existingFileAuthorityAnchorFixture {
	t.Helper()
	rawDataRoot := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(rawDataRoot, "private", "authority", "final-answer-ed25519-v1.json")
	authority, err := OpenOrCreateFileAuthority(path, false)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(rawDataRoot, rawDataRoot)
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := roots.DataDir
	path = filepath.Join(dataRoot, "private", "authority", "final-answer-ed25519-v1.json")
	rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := NewExistingFileAuthorityAnchor(rootAuthority, authority.KeyID(), authority.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return existingFileAuthorityAnchorFixture{
		dataRoot: dataRoot, path: path, authority: authority, rootAuthority: rootAuthority, anchor: anchor,
	}
}
