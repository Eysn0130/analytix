//go:build darwin || linux || windows

package finalauthority

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingFileVerificationIsReadOnlyAndBoundToKeyIdentity(t *testing.T) {
	for _, change := range []string{"unchanged", "same_key_replacement", "missing_key", "wrong_key"} {
		t.Run(change, func(t *testing.T) {
			fixture := newExistingFileAuthorityAnchorFixture(t)
			body, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			verification, err := OpenExistingFileVerificationV1(fixture.rootAuthority)
			if err != nil {
				t.Fatal(err)
			}
			message := []byte("synthetic verification")
			signature, err := fixture.authority.Sign(context.Background(), message)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := verification.Sign(context.Background(), message); err == nil {
				t.Fatal("read-only key view minted authority")
			}
			switch change {
			case "same_key_replacement":
				if err := os.Rename(fixture.path, filepath.Join(t.TempDir(), "original-key.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture.path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing_key":
				if err := os.Remove(fixture.path); err != nil {
					t.Fatal(err)
				}
			case "wrong_key":
				otherPath := filepath.Join(t.TempDir(), "other-key.json")
				if _, err := OpenOrCreateFileAuthority(otherPath, false); err != nil {
					t.Fatal(err)
				}
				other, err := os.ReadFile(otherPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture.path, other, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err = verification.VerifyTrusted(context.Background(), verification.KeyID(), verification.PublicKey(), message, signature)
			if (change == "unchanged") != (err == nil) {
				t.Fatalf("existing key drift classification mismatch: %v", err)
			}
			if change == "missing_key" {
				if _, err := OpenExistingFileVerificationV1(fixture.rootAuthority); err == nil {
					t.Fatal("missing key was recreated")
				}
				if _, err := os.Lstat(fixture.path); !os.IsNotExist(err) {
					t.Fatal("verification created missing key")
				}
			}
			if change == "unchanged" {
				current, err := os.ReadFile(fixture.path)
				if err != nil || !bytes.Equal(body, current) {
					t.Fatal("verification changed key bytes")
				}
			}
		})
	}
}
