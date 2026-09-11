package persistencefs

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestPrivateCASRecoveryJournalExactEmptyBootstrapRestarts(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	ctx := context.Background()
	if _, err := openOrCreateStartupJournalAuthority(
		fixture.journal.journalAuthority, fixture.journal.authorityAnchorLocked(),
	); err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupCreateDirectory(
		fixture.journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() exact empty bootstrap error = %v", err)
	}
	if _, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request); err != nil {
		t.Fatalf("BeginPreparationAfterValidatedPreflight() after empty bootstrap: %v", err)
	}
}

func TestPrivateCASRecoveryJournalRecoversAuthenticatedPreparationResidue(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	ctx := context.Background()
	preparation, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainprivatecas.RecoveryJournalPreparationV1Bytes(preparation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.activePath(), privateCASRecoveryPreparationFileV1)); err != nil {
		t.Fatal(err)
	}
	tempPath := stagePrivateCASRecoveryAuthenticatedResidueV1(
		t, fixture, false, privateCASRecoveryPreparationFileV1, body,
		domainprivatecas.MaxRecoveryJournalRecordBytesV1,
	)
	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() after preparation crash cut error = %v", err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, tempPath)
	replayed, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request)
	if err != nil || replayed.PreparationDigest != preparation.PreparationDigest {
		t.Fatalf("restarted preparation = %#v, %v", replayed, err)
	}
}

func TestPrivateCASRecoveryJournalRecoversAuthenticatedChunkResidue(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	lifecycle := fixture.appendCompletedLifecycle(t)
	removePrivateCASRecoveryRecordsV1(t, fixture,
		privateCASRecoveryCompletionFileV1,
		privateCASRecoveryWitnessFileV1,
		privateCASRecoveryManifestFileV1,
		filepath.Join(privateCASRecoveryTargetsDirectoryV1, privateCASRecoveryChunkNameV1(0)),
	)
	body, err := domainprivatecas.RecoveryTargetChunkV1Bytes(lifecycle.chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	tempPath := stagePrivateCASRecoveryAuthenticatedResidueV1(
		t, fixture, true, privateCASRecoveryChunkNameV1(0), body, domainprivatecas.MaxRecoveryTargetChunkBytesV1,
	)
	session, err := fixture.journal.Load(context.Background())
	if err != nil || session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 || len(session.Chunks) != 0 {
		t.Fatalf("Load() after chunk crash cut = %#v, %v", session, err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, tempPath)
	if err := fixture.journal.PutTargetChunkIfAbsent(context.Background(), lifecycle.chunks[0]); err != nil {
		t.Fatalf("PutTargetChunkIfAbsent() after residue cleanup: %v", err)
	}
}

func TestPrivateCASRecoveryJournalRecoversAuthenticatedSignedFrontierResidues(t *testing.T) {
	for _, test := range []struct {
		name          string
		target        string
		remove        []string
		expectedState domainprivatecas.RecoveryJournalSessionStateV1
		body          func(privateCASRecoveryJournalLifecycleV1) ([]byte, error)
		reappend      func(context.Context, *privateCASRecoveryJournalV1, privateCASRecoveryJournalLifecycleV1) error
	}{
		{
			name: "manifest", target: privateCASRecoveryManifestFileV1,
			remove:        []string{privateCASRecoveryCompletionFileV1, privateCASRecoveryWitnessFileV1, privateCASRecoveryManifestFileV1},
			expectedState: domainprivatecas.RecoveryJournalSessionPreparedV1,
			body: func(value privateCASRecoveryJournalLifecycleV1) ([]byte, error) {
				return domainprivatecas.RecoveryJournalManifestV1Bytes(value.manifest)
			},
			reappend: func(ctx context.Context, journal *privateCASRecoveryJournalV1, value privateCASRecoveryJournalLifecycleV1) error {
				return journal.PutManifestIfAbsent(ctx, value.manifest)
			},
		},
		{
			name: "commit witness", target: privateCASRecoveryWitnessFileV1,
			remove:        []string{privateCASRecoveryCompletionFileV1, privateCASRecoveryWitnessFileV1},
			expectedState: domainprivatecas.RecoveryJournalSessionManifestedV1,
			body: func(value privateCASRecoveryJournalLifecycleV1) ([]byte, error) {
				return domainprivatecas.RecoveryJournalCommitWitnessV1Bytes(value.witness)
			},
			reappend: func(ctx context.Context, journal *privateCASRecoveryJournalV1, value privateCASRecoveryJournalLifecycleV1) error {
				return journal.PutCommitWitnessIfAbsent(ctx, value.witness)
			},
		},
		{
			name: "completion receipt", target: privateCASRecoveryCompletionFileV1,
			remove:        []string{privateCASRecoveryCompletionFileV1},
			expectedState: domainprivatecas.RecoveryJournalSessionCommitWitnessedV1,
			body: func(value privateCASRecoveryJournalLifecycleV1) ([]byte, error) {
				return domainprivatecas.RecoveryJournalCompletionReceiptV1Bytes(value.completion)
			},
			reappend: func(ctx context.Context, journal *privateCASRecoveryJournalV1, value privateCASRecoveryJournalLifecycleV1) error {
				return journal.PutCompletionReceiptIfAbsent(ctx, value.completion)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
			lifecycle := fixture.appendCompletedLifecycle(t)
			removePrivateCASRecoveryRecordsV1(t, fixture, test.remove...)
			body, err := test.body(lifecycle)
			if err != nil {
				t.Fatal(err)
			}
			tempPath := stagePrivateCASRecoveryAuthenticatedResidueV1(
				t, fixture, false, test.target, body, domainprivatecas.MaxRecoveryJournalRecordBytesV1,
			)
			session, err := fixture.journal.Load(context.Background())
			if err != nil || session.State != test.expectedState {
				t.Fatalf("Load() after %s crash cut = %#v, %v", test.name, session, err)
			}
			assertPrivateCASRecoveryJournalPathAbsentV1(t, tempPath)
			if err := test.reappend(context.Background(), fixture.journal, lifecycle); err != nil {
				t.Fatalf("reappend %s: %v", test.name, err)
			}
		})
	}
}

func TestPrivateCASRecoveryJournalRecoversAuthenticatedRetirementResidue(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	lifecycle := fixture.appendCompletedLifecycle(t)
	body := privateCASRecoveryRetirementBodyV1(t, fixture, false)
	tempPath := stagePrivateCASRecoveryAuthenticatedResidueV1(
		t, fixture, false, privateCASRecoveryRetirementFileV1, body, maxPrivateCASRecoveryRetirementBytesV1,
	)
	session, err := fixture.journal.Load(context.Background())
	if err != nil || session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 {
		t.Fatalf("Load() after retirement crash cut = %#v, %v", session, err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, tempPath)
	if err := fixture.journal.Retire(context.Background(), privatecasport.RecoveryRetirementRequestV1{
		TransactionID: lifecycle.manifest.TransactionID, CompletionReceiptDigest: lifecycle.completion.ReceiptDigest,
	}); err != nil {
		t.Fatalf("Retire() after residue cleanup: %v", err)
	}
}

func TestPrivateCASRecoveryJournalRejectsUnauthenticatedTamperedMultipleAndUnknownResidues(t *testing.T) {
	for _, test := range []struct {
		name  string
		stage func(*testing.T, privateCASRecoveryJournalTestFixtureV1, privateCASRecoveryJournalLifecycleV1) []string
	}{
		{
			name: "legacy unsigned name",
			stage: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1, lifecycle privateCASRecoveryJournalLifecycleV1) []string {
				body, err := domainprivatecas.RecoveryJournalManifestV1Bytes(lifecycle.manifest)
				if err != nil {
					t.Fatal(err)
				}
				name := "." + privateCASRecoveryManifestFileV1 + "-" + strings.Repeat("a", 32) + ".tmp"
				path := filepath.Join(fixture.activePath(), name)
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				return []string{path}
			},
		},
		{
			name: "tampered authenticated body",
			stage: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1, lifecycle privateCASRecoveryJournalLifecycleV1) []string {
				body, err := domainprivatecas.RecoveryJournalManifestV1Bytes(lifecycle.manifest)
				if err != nil {
					t.Fatal(err)
				}
				path := stagePrivateCASRecoveryAuthenticatedResidueV1(
					t, fixture, false, privateCASRecoveryManifestFileV1, body, domainprivatecas.MaxRecoveryJournalRecordBytesV1,
				)
				body[len(body)/2] ^= 1
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				return []string{path}
			},
		},
		{
			name: "multiple authenticated residues",
			stage: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1, lifecycle privateCASRecoveryJournalLifecycleV1) []string {
				manifestBody, err := domainprivatecas.RecoveryJournalManifestV1Bytes(lifecycle.manifest)
				if err != nil {
					t.Fatal(err)
				}
				chunkBody, err := domainprivatecas.RecoveryTargetChunkV1Bytes(lifecycle.chunks[0])
				if err != nil {
					t.Fatal(err)
				}
				return []string{
					stagePrivateCASRecoveryAuthenticatedResidueV1(t, fixture, false, privateCASRecoveryManifestFileV1, manifestBody, domainprivatecas.MaxRecoveryJournalRecordBytesV1),
					stagePrivateCASRecoveryAuthenticatedResidueV1(t, fixture, true, privateCASRecoveryChunkNameV1(0), chunkBody, domainprivatecas.MaxRecoveryTargetChunkBytesV1),
				}
			},
		},
		{
			name: "unknown entry beside authenticated residue",
			stage: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1, lifecycle privateCASRecoveryJournalLifecycleV1) []string {
				body, err := domainprivatecas.RecoveryJournalManifestV1Bytes(lifecycle.manifest)
				if err != nil {
					t.Fatal(err)
				}
				path := stagePrivateCASRecoveryAuthenticatedResidueV1(
					t, fixture, false, privateCASRecoveryManifestFileV1, body, domainprivatecas.MaxRecoveryJournalRecordBytesV1,
				)
				unknown := filepath.Join(fixture.activePath(), "unknown.json")
				if err := os.WriteFile(unknown, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				return []string{path, unknown}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
			lifecycle := fixture.appendCompletedLifecycle(t)
			paths := test.stage(t, fixture, lifecycle)
			if _, err := fixture.journal.Load(context.Background()); err == nil || errors.Is(err, privatecasport.ErrJournalAbsent) {
				t.Fatalf("Load() accepted unsafe residue inventory: %v", err)
			}
			for _, path := range paths {
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf("unsafe residue %q was mutated: %v", path, err)
				}
			}
		})
	}
}

func TestPrivateCASRecoveryJournalRejectsStaleCommittedRetirementBeforeResidueCleanup(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	fixture.appendCompletedLifecycle(t)
	retirementBody := privateCASRecoveryRetirementBodyV1(t, fixture, true)
	preparationPath := filepath.Join(fixture.activePath(), privateCASRecoveryPreparationFileV1)
	preparationBody, err := os.ReadFile(preparationPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(preparationPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparationPath, preparationBody, 0o600); err != nil {
		t.Fatal(err)
	}
	tempPath := stagePrivateCASRecoveryAuthenticatedResidueV1(
		t, fixture, false, privateCASRecoveryRetirementFileV1, retirementBody, maxPrivateCASRecoveryRetirementBytesV1,
	)
	if _, err := fixture.journal.Load(context.Background()); err == nil || errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() accepted stale committed retirement inventory: %v", err)
	}
	if _, err := os.Lstat(tempPath); err != nil {
		t.Fatalf("retirement residue was deleted before inventory validation: %v", err)
	}
}

func TestPrivateCASRecoveryAuthenticatedTempRequiresCanonicalBase64(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	fixture.appendCompletedLifecycle(t)
	body, err := os.ReadFile(filepath.Join(fixture.activePath(), privateCASRecoveryManifestFileV1))
	if err != nil {
		t.Fatal(err)
	}
	directory, authority := openPrivateCASRecoveryResidueTestDirectoryV1(t, fixture, false)
	defer directory.Close()
	name, err := newPrivateCASRecoveryAuthenticatedTempNameV1(directory, authority, privateCASRecoveryManifestFileV1, body)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "." + privateCASRecoveryManifestFileV1 + "-"
	remainder := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".tmp")
	signature := remainder[33:]
	decoded, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		t.Fatal(err)
	}
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, signature[len(signature)-1])
	alternative := last ^ 1
	if alternative>>4 != last>>4 {
		alternative = last ^ 2
	}
	nonCanonicalSignature := signature[:len(signature)-1] + string(alphabet[alternative])
	if _, err := base64.RawURLEncoding.DecodeString(nonCanonicalSignature); err != nil ||
		!strings.EqualFold(base64.RawURLEncoding.EncodeToString(decoded), signature) {
		t.Skip("Go base64 decoder rejected the non-canonical pad-bit fixture")
	}
	nonCanonicalName := prefix + remainder[:33] + nonCanonicalSignature + ".tmp"
	if err := validatePrivateCASRecoveryAuthenticatedTempV1(
		directory, authority, privateCASRecoveryManifestFileV1, nonCanonicalName, body,
	); err == nil {
		t.Fatal("non-canonical base64 temp signature was accepted")
	}
}

func stagePrivateCASRecoveryAuthenticatedResidueV1(
	t *testing.T,
	fixture privateCASRecoveryJournalTestFixtureV1,
	targetsDirectory bool,
	targetName string,
	body []byte,
	maxBytes int64,
) string {
	t.Helper()
	directory, authority := openPrivateCASRecoveryResidueTestDirectoryV1(t, fixture, targetsDirectory)
	defer directory.Close()
	tempName, err := newPrivateCASRecoveryAuthenticatedTempNameV1(directory, authority, targetName, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.WriteExclusive(tempName, body, maxBytes); err != nil {
		t.Fatal(err)
	}
	path := fixture.activePath()
	if targetsDirectory {
		path = filepath.Join(path, privateCASRecoveryTargetsDirectoryV1)
	}
	return filepath.Join(path, tempName)
}

func openPrivateCASRecoveryResidueTestDirectoryV1(
	t *testing.T,
	fixture privateCASRecoveryJournalTestFixtureV1,
	targetsDirectory bool,
) (*startupPrivateDirectory, *startupJournalAuthority) {
	t.Helper()
	fixture.journal.mu.Lock()
	authority, err := fixture.journal.loadAuthorityLocked(context.Background(), false)
	fixture.journal.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupOpenDirectory(
		fixture.journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !targetsDirectory {
		return directory, authority
	}
	targets, err := directory.OpenDirectory(privateCASRecoveryTargetsDirectoryV1, "")
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		if targets != nil {
			_ = targets.Close()
		}
		t.Fatal(errors.Join(err, closeErr))
	}
	return targets, authority
}

func removePrivateCASRecoveryRecordsV1(
	t *testing.T,
	fixture privateCASRecoveryJournalTestFixtureV1,
	names ...string,
) {
	t.Helper()
	for _, name := range names {
		if err := os.Remove(filepath.Join(fixture.activePath(), name)); err != nil {
			t.Fatal(err)
		}
	}
}

func privateCASRecoveryRetirementBodyV1(
	t *testing.T,
	fixture privateCASRecoveryJournalTestFixtureV1,
	commit bool,
) []byte {
	t.Helper()
	ctx := context.Background()
	fixture.journal.mu.Lock()
	defer fixture.journal.mu.Unlock()
	authority, err := fixture.journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupOpenDirectory(
		fixture.journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fixture.journal.readSessionFromDirectoryLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	retirement, err := fixture.journal.newRetirementLocked(ctx, directory, authority, session)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	body, err := privateCASRecoveryRetirementBytesV1(*retirement)
	if err == nil && commit {
		err = writePrivateCASRecoveryRecordExactV1(
			directory, authority, privateCASRecoveryRetirementFileV1, body, maxPrivateCASRecoveryRetirementBytesV1,
		)
	}
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	return body
}
