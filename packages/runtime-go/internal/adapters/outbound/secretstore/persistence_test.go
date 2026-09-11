package secretstore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestPersistenceRejectsPlaintextUnknownAndMalformedDocuments(t *testing.T) {
	t.Parallel()

	ref := "cred_" + strings.Repeat("A", 43)
	nonce := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 12))
	ciphertext := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	for _, testCase := range []struct {
		name    string
		content string
	}{
		{name: "plaintext fallback", content: "synthetic-plaintext-marker"},
		{name: "malformed JSON", content: `{"version":1`},
		{name: "trailing document", content: `{"version":1,"records":{}} {}`},
		{name: "unknown top-level field", content: `{"version":1,"records":{},"plaintext":"synthetic-plaintext-marker"}`},
		{name: "unknown version", content: `{"version":2,"records":{}}`},
		{name: "null records", content: `{"version":1,"records":null}`},
		{
			name:    "plaintext record field",
			content: `{"version":1,"records":{"` + ref + `":{"purpose":"provider-api-key","lifecycle":"active","plaintext":"synthetic-plaintext-marker","envelope":{"version":1,"nonce":"` + nonce + `","ciphertext":"` + ciphertext + `"}}}}`,
		},
		{
			name:    "unknown envelope version",
			content: `{"version":1,"records":{"` + ref + `":{"purpose":"provider-api-key","lifecycle":"active","envelope":{"version":2,"nonce":"` + nonce + `","ciphertext":"` + ciphertext + `"}}}}`,
		},
		{
			name:    "truncated ciphertext",
			content: `{"version":1,"records":{"` + ref + `":{"purpose":"provider-api-key","lifecycle":"active","envelope":{"version":1,"nonce":"` + nonce + `","ciphertext":"AA"}}}}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.enc.json")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			_, err := (filePersistence{}).Load(path)
			if !errors.Is(err, portsecretstore.ErrPersistence) {
				t.Fatalf("Load() error = %v, want persistence failure", err)
			}
			assertRedactedError(t, err, "synthetic-plaintext-marker", ref, path)
		})
	}
}

func TestPersistenceCommitsPrivateAtomicEncryptedDocument(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "credentials.enc.json")
	document := validSyntheticPersistentDocument()
	if err := (filePersistence{}).Commit(path, document); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("committed store is not a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("committed mode = %o, want 600", info.Mode().Perm())
	}
	temporary, err := filepath.Glob(filepath.Join(directory, ".credentials.enc.json.tmp-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(temporary) != 0 {
		t.Fatalf("atomic commit retained temporary files: %d", len(temporary))
	}
	loaded, err := (filePersistence{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Version != document.Version || len(loaded.Records) != len(document.Records) {
		t.Fatal("loaded document does not match the committed structure")
	}
	content := mustReadFile(t, path)
	if bytes.Contains(content, []byte("synthetic-plaintext-marker")) {
		t.Fatal("committed store contains a synthetic plaintext marker")
	}
}

func TestPersistencePreReplaceFailurePreservesPriorBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix private-mode failure seam")
	}

	directory := t.TempDir()
	path := filepath.Join(directory, "credentials.enc.json")
	document := validSyntheticPersistentDocument()
	if err := (filePersistence{}).Commit(path, document); err != nil {
		t.Fatalf("initial Commit() error = %v", err)
	}
	before := mustReadFile(t, path)
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatalf("Chmod(0500) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(directory, 0o700)
	})
	document.Records["cred_"+strings.Repeat("B", 43)] = document.Records["cred_"+strings.Repeat("A", 43)]
	err := (filePersistence{}).Commit(path, document)
	if !errors.Is(err, portsecretstore.ErrPersistence) {
		t.Fatalf("Commit() error = %v, want persistence failure", err)
	}
	after := mustReadFile(t, path)
	if !bytes.Equal(before, after) {
		t.Fatal("pre-replace failure changed prior committed bytes")
	}
}

func TestPersistenceAtomicFailuresPreservePriorRecoverableBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory-sync fault seam")
	}

	for _, testCase := range []struct {
		name  string
		hooks *atomicCommitHooks
	}{
		{
			name: "pre replace",
			hooks: &atomicCommitHooks{beforeReplace: func() error {
				return errors.New("synthetic pre-replace failure")
			}},
		},
		{
			name: "post rename directory sync",
			hooks: &atomicCommitHooks{afterReplaceBeforeDirectorySync: func() error {
				return errors.New("synthetic post-rename directory-sync failure")
			}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "credentials.enc.json")
			document := validSyntheticPersistentDocument()
			if err := (filePersistence{}).Commit(path, document); err != nil {
				t.Fatalf("initial Commit() error = %v", err)
			}
			before := mustReadFile(t, path)
			document.Records["cred_"+strings.Repeat("B", 43)] = document.Records["cred_"+strings.Repeat("A", 43)]
			err := (filePersistence{atomicHooks: testCase.hooks}).Commit(path, document)
			if !errors.Is(err, portsecretstore.ErrPersistence) {
				t.Fatalf("Commit() error = %v, want persistence failure", err)
			}
			after := mustReadFile(t, path)
			if !bytes.Equal(before, after) {
				t.Fatal("returned atomic failure did not leave the prior committed bytes at the target")
			}
			recovered, loadErr := (filePersistence{}).Load(path)
			if loadErr != nil {
				t.Fatalf("Load(after failed commit) error = %v", loadErr)
			}
			if len(recovered.Records) != 1 {
				t.Fatal("failed commit did not preserve the last committed document")
			}
			temporary, globErr := filepath.Glob(filepath.Join(directory, ".credentials.enc.json.tmp-*"))
			if globErr != nil {
				t.Fatalf("Glob() error = %v", globErr)
			}
			if len(temporary) != 0 {
				t.Fatalf("returned atomic failure retained task temporary files: %d", len(temporary))
			}
			for _, suffix := range []string{".commit-journal", ".previous", ".committed", ".rollback-required"} {
				if _, statErr := os.Lstat(filepath.Join(directory, ".credentials.enc.json"+suffix)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("returned atomic failure retained recovery artifact %s", suffix)
				}
			}
		})
	}
}

func TestPersistenceCommitMarkerSyncFailureRestoresPriorState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix commit-marker directory-sync fault seam")
	}

	for _, priorExists := range []bool{true, false} {
		name := "prior absent"
		if priorExists {
			name = "prior present"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "credentials.enc.json")
			before := []byte(nil)
			if priorExists {
				if err := (filePersistence{}).Commit(path, validSyntheticPersistentDocument()); err != nil {
					t.Fatalf("initial Commit() error = %v", err)
				}
				before = mustReadFile(t, path)
			}
			candidate := validSyntheticPersistentDocument()
			candidate.Records["cred_"+strings.Repeat("B", 43)] = candidate.Records["cred_"+strings.Repeat("A", 43)]
			err := (filePersistence{atomicHooks: &atomicCommitHooks{
				afterCommitMarkerLink: func() error {
					return errors.New("synthetic commit-marker directory-sync failure")
				},
			}}).Commit(path, candidate)
			if !errors.Is(err, portsecretstore.ErrPersistence) {
				t.Fatalf("Commit() error = %v, want persistence failure", err)
			}
			if priorExists {
				after := mustReadFile(t, path)
				if !bytes.Equal(before, after) {
					t.Fatal("commit-marker sync failure did not restore the exact prior target")
				}
			} else if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("commit-marker sync failure retained a target for prior-absent state")
			}
			recovered, loadErr := (filePersistence{}).Load(path)
			if loadErr != nil {
				t.Fatalf("Load(after failed marker sync) error = %v", loadErr)
			}
			wantRecords := 0
			if priorExists {
				wantRecords = 1
			}
			if len(recovered.Records) != wantRecords {
				t.Fatal("restart recovery silently finalized the failed candidate")
			}
			for _, suffix := range []string{".commit-journal", ".previous", ".committed", ".rollback-required"} {
				if _, statErr := os.Lstat(filepath.Join(directory, ".credentials.enc.json"+suffix)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("marker-sync failure retained recovery artifact %s", suffix)
				}
			}
			pending, globErr := filepath.Glob(filepath.Join(directory, "..credentials.enc.json.committed.pending-*"))
			if globErr != nil {
				t.Fatalf("Glob() error = %v", globErr)
			}
			if len(pending) != 0 {
				t.Fatalf("marker-sync failure retained pending markers: %d", len(pending))
			}
		})
	}
}

func TestPersistenceRollbackEvidenceReestablishmentFailureDoesNotFinalizeCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix rollback-evidence fault seam")
	}

	for _, priorExists := range []bool{true, false} {
		name := "prior absent"
		if priorExists {
			name = "prior present"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "credentials.enc.json")
			before := []byte(nil)
			if priorExists {
				if err := (filePersistence{}).Commit(path, validSyntheticPersistentDocument()); err != nil {
					t.Fatalf("initial Commit() error = %v", err)
				}
				before = mustReadFile(t, path)
			}
			candidate := validSyntheticPersistentDocument()
			candidate.Records["cred_"+strings.Repeat("B", 43)] = candidate.Records["cred_"+strings.Repeat("A", 43)]
			err := (filePersistence{atomicHooks: &atomicCommitHooks{
				afterRollbackRequiredRemoval: func() error {
					return errors.New("synthetic confirmation directory-sync failure")
				},
				beforeRollbackRequiredReestablishment: func() error {
					return errors.New("synthetic rollback evidence re-establishment failure")
				},
			}}).Commit(path, candidate)
			if !errors.Is(err, portsecretstore.ErrPersistence) {
				t.Fatalf("Commit() error = %v, want persistence failure", err)
			}
			if priorExists {
				after := mustReadFile(t, path)
				if !bytes.Equal(before, after) {
					t.Fatal("combined confirmation failure did not restore the exact prior target")
				}
			} else if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("combined confirmation failure retained a target for prior-absent state")
			}
			recovered, loadErr := (filePersistence{}).Load(path)
			if loadErr != nil {
				t.Fatalf("Load(after combined confirmation failure) error = %v", loadErr)
			}
			wantRecords := 0
			if priorExists {
				wantRecords = 1
			}
			if len(recovered.Records) != wantRecords {
				t.Fatal("combined confirmation failure silently finalized the candidate")
			}
			assertNoAtomicRecoveryArtifacts(t, directory)
		})
	}
}

func assertNoAtomicRecoveryArtifacts(t *testing.T, directory string) {
	t.Helper()
	for _, suffix := range []string{".commit-journal", ".previous", ".committed", ".rollback-required"} {
		if _, statErr := os.Lstat(filepath.Join(directory, ".credentials.enc.json"+suffix)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("atomic recovery artifact retained: %s", suffix)
		}
	}
}

func validSyntheticPersistentDocument() persistentDocument {
	return persistentDocument{
		Version: persistentFormatVersion,
		Records: map[string]persistentRecord{
			"cred_" + strings.Repeat("A", 43): {
				Purpose:   "provider-api-key",
				Lifecycle: lifecycleActive,
				Envelope: encryptedEnvelope{
					Version:    envelopeVersion,
					Nonce:      base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 12)),
					Ciphertext: base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)),
				},
			},
		},
	}
}
