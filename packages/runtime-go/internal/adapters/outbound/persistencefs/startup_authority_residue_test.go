package persistencefs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupAuthorityResidueRequiresCanonicalSelfConsistentBody(t *testing.T) {
	for _, test := range []struct {
		name      string
		nameBody  func(*testing.T) []byte
		writeBody func(*testing.T) []byte
	}{
		{
			name:      "partial JSON",
			nameBody:  func(*testing.T) []byte { return []byte(`{"partial":`) },
			writeBody: func(*testing.T) []byte { return []byte(`{"partial":`) },
		},
		{
			name:     "body digest mismatch",
			nameBody: newStartupAuthorityBodyForTest,
			writeBody: func(t *testing.T) []byte {
				return newStartupAuthorityBodyForTest(t)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			nameBody := test.nameBody(t)
			writeBody := test.writeBody(t)
			if test.name == "body digest mismatch" {
				for string(writeBody) == string(nameBody) {
					writeBody = newStartupAuthorityBodyForTest(t)
				}
			}
			targetName := filepath.Base(startupJournalAuthorityPath(journalRoot))
			residueName, err := startupAuthorityCreateTempName(
				targetName, nameBody, "0123456789abcdef01234567",
			)
			if err != nil {
				t.Fatal(err)
			}
			residuePath := filepath.Join(filepath.Dir(journalRoot), residueName)
			if err := os.WriteFile(residuePath, writeBody, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := NewSemanticPlanBuilder(roots).Recover(
				context.Background(), semanticConfigurationDigestForTest(),
			); err == nil {
				t.Fatalf("Recover() accepted unsafe %s authority residue", test.name)
			}
			if _, err := os.Lstat(residuePath); err != nil {
				t.Fatalf("unsafe authority residue was mutated: %v", err)
			}
		})
	}
}

func TestStartupAuthorityResidueWithCommittedTargetRequiresExactBytes(t *testing.T) {
	for _, test := range []struct {
		name     string
		conflict bool
	}{
		{name: "exact duplicate"},
		{name: "conflict", conflict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			targetName := filepath.Base(startupJournalAuthorityPath(journalRoot))
			targetPath := startupJournalAuthorityPath(journalRoot)
			targetBody := newStartupAuthorityBodyForTest(t)
			if err := os.WriteFile(targetPath, targetBody, 0o600); err != nil {
				t.Fatal(err)
			}
			residueBody := targetBody
			if test.conflict {
				residueBody = newStartupAuthorityBodyForTest(t)
			}
			residueName, err := startupAuthorityCreateTempName(
				targetName, residueBody, "0123456789abcdef01234567",
			)
			if err != nil {
				t.Fatal(err)
			}
			residuePath := filepath.Join(filepath.Dir(journalRoot), residueName)
			if err := os.WriteFile(residuePath, residueBody, 0o600); err != nil {
				t.Fatal(err)
			}
			recoverErr := NewSemanticPlanBuilder(roots).Recover(
				context.Background(), semanticConfigurationDigestForTest(),
			)
			if test.conflict {
				if recoverErr == nil {
					t.Fatal("Recover() accepted conflicting authority target and residue")
				}
				if _, err := os.Lstat(residuePath); err != nil {
					t.Fatalf("conflicting authority residue was mutated: %v", err)
				}
			} else {
				if recoverErr != nil {
					t.Fatalf("Recover() exact duplicate authority residue: %v", recoverErr)
				}
				if _, err := os.Lstat(residuePath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("exact duplicate authority residue survived: %v", err)
				}
			}
			committed, err := os.ReadFile(targetPath)
			if err != nil || string(committed) != string(targetBody) {
				t.Fatalf("committed authority target changed: %v", err)
			}
		})
	}
}

func TestStartupAuthorityBootstrapPreservesResidueBesideAuthorityDependentState(t *testing.T) {
	for _, dependentName := range []string{
		"journal",
		privateCASRecoveryJournalDirectoryV1,
		".retired-journal-0123456789abcdef0123456789abcdef",
		privateCASRecoveryRetiredPrefixV1 + "0123456789abcdef0123456789abcdef",
	} {
		t.Run(dependentName, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			targetName := filepath.Base(startupJournalAuthorityPath(journalRoot))
			body := newStartupAuthorityBodyForTest(t)
			residueName, err := startupAuthorityCreateTempName(
				targetName, body, "0123456789abcdef01234567",
			)
			if err != nil {
				t.Fatal(err)
			}
			residuePath := filepath.Join(namespace.path(), residueName)
			if err := os.WriteFile(residuePath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(namespace.path(), dependentName), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := openOrCreateStartupJournalAuthority(namespace, journalRoot); err == nil {
				t.Fatal("authority bootstrap accepted pre-existing authority-dependent state")
			}
			if _, err := os.Lstat(residuePath); err != nil {
				t.Fatalf("uncommitted authority residue was mutated beside dependent state: %v", err)
			}
			if _, err := os.Lstat(startupJournalAuthorityPath(journalRoot)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("authority key was created beside dependent state: %v", err)
			}
		})
	}
}

func TestStartupAuthorityBodyRejectsNonCanonicalBase64(t *testing.T) {
	body := newStartupAuthorityBodyForTest(t)
	record, err := parseStartupJournalAuthorityBody(body)
	if err != nil {
		t.Fatal(err)
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := len(record.PublicKey) - 1
	index := 0
	for index < len(alphabet) && alphabet[index] != record.PublicKey[last] {
		index++
	}
	if index == len(alphabet) {
		t.Fatal("generated authority public key is not raw base64url")
	}
	alternative := (index &^ 3) | ((index + 1) & 3)
	if alternative == index {
		alternative = (index &^ 3) | ((index + 2) & 3)
	}
	record.PublicKey = record.PublicKey[:last] + string(alphabet[alternative])
	nonCanonical, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, decodeErr := base64.RawURLEncoding.DecodeString(record.PublicKey); decodeErr == nil &&
		base64.RawURLEncoding.EncodeToString(decoded) == record.PublicKey {
		t.Fatal("test fixture unexpectedly remained canonical")
	}
	if _, err := parseStartupJournalAuthorityBody(nonCanonical); err == nil {
		t.Fatal("non-canonical base64 authority key was accepted")
	}
}

func newStartupAuthorityBodyForTest(t *testing.T) []byte {
	t.Helper()
	body, err := newStartupJournalAuthorityBody()
	if err != nil {
		t.Fatal(err)
	}
	return body
}
