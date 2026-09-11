package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type privateCASRecoveryJournalTestFixtureV1 struct {
	journal   *privateCASRecoveryJournalV1
	lease     *CompositeLease
	namespace string
	request   privatecasport.RecoveryPreparationRequestV1
}

type privateCASRecoveryJournalLifecycleV1 struct {
	preparation domainprivatecas.RecoveryJournalPreparationV1
	chunks      []domainprivatecas.RecoveryTargetChunkV1
	manifest    domainprivatecas.RecoveryJournalManifestV1
	witness     domainprivatecas.RecoveryJournalCommitWitnessV1
	completion  domainprivatecas.RecoveryJournalCompletionReceiptV1
}

func TestPrivateCASRecoveryJournalConstructorAndLoadCreateNoState(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	ctx := context.Background()

	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() error = %v, want ErrJournalAbsent", err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.authorityKeyPath())
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.activePath())
	if _, err := fixture.journal.KeyID(ctx); err == nil {
		t.Fatal("KeyID() created or accepted an absent installation key")
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.authorityKeyPath())
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.activePath())
}

func TestPrivateCASRecoveryJournalBeginUsesSharedKeyAndExactReplay(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	ctx := context.Background()

	shared, err := openOrCreateStartupJournalAuthority(
		fixture.journal.journalAuthority, fixture.journal.authorityAnchorLocked(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() with only shared key error = %v", err)
	}
	preparation, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if preparation.AuthorityKeyID != shared.keyID || preparation.RootBindingDigest != rootBindingDigest(fixture.journal.roots) {
		t.Fatalf("preparation authority = %#v", preparation)
	}
	if _, err := os.Stat(filepath.Join(fixture.activePath(), privateCASRecoveryTargetsDirectoryV1)); err != nil {
		t.Fatalf("targets directory missing: %v", err)
	}
	message, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(preparation)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Verify(ctx, preparation.AuthorityKeyID, message, preparation.AuthoritySignature); err != nil {
		t.Fatalf("Verify(preparation) error = %v", err)
	}
	replayed, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request)
	if err != nil || replayed.PreparationDigest != preparation.PreparationDigest || replayed.AuthoritySignature != preparation.AuthoritySignature {
		t.Fatalf("exact Begin replay = %#v, %v", replayed, err)
	}
	conflict := fixture.request
	conflict.PreparedAt = conflict.PreparedAt.Add(time.Nanosecond)
	if _, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, conflict); err == nil {
		t.Fatal("conflicting Begin replay was accepted")
	}
	session, err := fixture.journal.Load(ctx)
	if err != nil || session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 ||
		session.Preparation.PreparationDigest != preparation.PreparationDigest {
		t.Fatalf("Load() = %#v, %v", session, err)
	}
}

func TestPrivateCASRecoveryJournalSignedLifecycleAndRetirement(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	lifecycle := fixture.appendCompletedLifecycle(t)
	ctx := context.Background()

	if err := fixture.journal.PutPreparationIfAbsent(ctx, lifecycle.preparation); err != nil {
		t.Fatalf("exact preparation replay: %v", err)
	}
	if err := fixture.journal.PutTargetChunkIfAbsent(ctx, lifecycle.chunks[0]); err != nil {
		t.Fatalf("exact chunk replay: %v", err)
	}
	if err := fixture.journal.PutManifestIfAbsent(ctx, lifecycle.manifest); err != nil {
		t.Fatalf("exact manifest replay: %v", err)
	}
	if err := fixture.journal.PutCommitWitnessIfAbsent(ctx, lifecycle.witness); err != nil {
		t.Fatalf("exact witness replay: %v", err)
	}
	if err := fixture.journal.PutCompletionReceiptIfAbsent(ctx, lifecycle.completion); err != nil {
		t.Fatalf("exact completion replay: %v", err)
	}
	session, err := fixture.journal.Load(ctx)
	if err != nil || session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 ||
		session.CompletionReceipt == nil || session.CompletionReceipt.ReceiptDigest != lifecycle.completion.ReceiptDigest {
		t.Fatalf("completed Load() = %#v, %v", session, err)
	}
	if err := fixture.journal.Retire(ctx, privatecasport.RecoveryRetirementRequestV1{
		TransactionID:           lifecycle.completion.ReceiptDigest,
		CompletionReceiptDigest: lifecycle.manifest.TransactionID,
	}); err == nil {
		t.Fatal("Retire() accepted interchanged transaction and completion selectors")
	}
	if session, err := fixture.journal.Load(ctx); err != nil ||
		session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 {
		t.Fatalf("rejected retirement changed completed journal: %#v, %v", session, err)
	}

	if err := fixture.journal.Retire(ctx, privatecasport.RecoveryRetirementRequestV1{
		TransactionID: lifecycle.manifest.TransactionID, CompletionReceiptDigest: lifecycle.completion.ReceiptDigest,
	}); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() after retirement error = %v", err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.activePath())
	if _, err := os.Stat(fixture.authorityKeyPath()); err != nil {
		t.Fatalf("shared installation key was retired with the journal: %v", err)
	}
	entries, err := os.ReadDir(fixture.namespace)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), privateCASRecoveryRetiredPrefixV1) {
			t.Fatalf("retired directory remained after successful retirement: %s", entry.Name())
		}
	}
}

func TestPrivateCASRecoveryJournalRejectsTargetOutsidePreparedPlanBeforeWrite(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	ctx := context.Background()
	if _, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request); err != nil {
		t.Fatal(err)
	}
	target, err := domainprivatecas.NewRecoveryTargetEntryV1(domainprivatecas.RecoveryTargetEntryInputV1{
		PlanIndex: 1, RootID: "accepted-finals/records",
		PlanAuthorityDigest: privateCASRecoveryTestDigestV1("wrong-plan-authority"),
		ShardName:           "bb", ShardIdentityDigest: privateCASRecoveryTestDigestV1("wrong-plan-shard"),
		OriginalName:         privateCASRecoveryTempNameV1("b", "wrong-plan"),
		ObjectIdentityDigest: privateCASRecoveryTestDigestV1("wrong-plan-object"),
		BodySHA256:           privateCASRecoveryTestDigestV1("wrong-plan-body"), ByteLength: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := domainprivatecas.BuildRecoveryTargetChunksV1([]domainprivatecas.RecoveryTargetEntryV1{target})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.PutTargetChunkIfAbsent(ctx, chunks[0]); err == nil {
		t.Fatal("target outside the prepared plan range was appended")
	}
	session, err := fixture.journal.Load(ctx)
	if err != nil || len(session.Chunks) != 0 || session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 {
		t.Fatalf("journal changed after rejected target: %#v, %v", session, err)
	}
}

func TestPrivateCASRecoveryJournalRejectsTamperUnknownInventoryAndMissingKey(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, privateCASRecoveryJournalTestFixtureV1)
	}{
		{
			name: "unknown active entry",
			mutate: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(fixture.activePath(), "unknown.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "tampered preparation",
			mutate: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1) {
				t.Helper()
				path := filepath.Join(fixture.activePath(), privateCASRecoveryPreparationFileV1)
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				body[len(body)/2] ^= 1
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing installation key",
			mutate: func(t *testing.T, fixture privateCASRecoveryJournalTestFixtureV1) {
				t.Helper()
				if err := os.Remove(fixture.authorityKeyPath()); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
			if _, err := fixture.journal.BeginPreparationAfterValidatedPreflight(context.Background(), fixture.request); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, fixture)
			if _, err := fixture.journal.Load(context.Background()); err == nil || errors.Is(err, privatecasport.ErrJournalAbsent) {
				t.Fatalf("Load() did not fail closed: %v", err)
			}
		})
	}
}

func TestPrivateCASRecoveryJournalRecoversAuthenticatedRetiredDirectory(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	lifecycle := fixture.appendCompletedLifecycle(t)
	ctx := context.Background()

	fixture.journal.mu.Lock()
	authority, err := fixture.journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		fixture.journal.mu.Unlock()
		t.Fatal(err)
	}
	directory, err := openPrivateCASRecoveryDirectoryV1(
		fixture.journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1,
		lifecycle.preparation.JournalDirectoryIdentity,
	)
	if err != nil {
		fixture.journal.mu.Unlock()
		t.Fatal(err)
	}
	session, _, err := fixture.journal.readSessionFromDirectoryLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		fixture.journal.mu.Unlock()
		t.Fatal(err)
	}
	retirement, err := fixture.journal.newRetirementLocked(ctx, directory, authority, session)
	if err != nil {
		_ = directory.Close()
		fixture.journal.mu.Unlock()
		t.Fatal(err)
	}
	body, err := privateCASRecoveryRetirementBytesV1(*retirement)
	if err == nil {
		err = writePrivateCASRecoveryRecordExactV1(
			directory, authority, privateCASRecoveryRetirementFileV1, body, maxPrivateCASRecoveryRetirementBytesV1,
		)
	}
	identity := directory.Identity()
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		fixture.journal.mu.Unlock()
		t.Fatal(errors.Join(err, closeErr))
	}
	retiredName, retired, err := secureStartupRetireDirectory(
		fixture.journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, identity, privateCASRecoveryRetiredPrefixV1,
	)
	if retired != nil {
		_ = retired.Close()
	}
	fixture.journal.mu.Unlock()
	if err != nil || retiredName == "" {
		t.Fatalf("stage retired directory: %q, %v", retiredName, err)
	}

	if _, err := fixture.journal.Load(ctx); !errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() after retired crash cut error = %v", err)
	}
	assertPrivateCASRecoveryJournalPathAbsentV1(t, filepath.Join(fixture.namespace, retiredName))
	assertPrivateCASRecoveryJournalPathAbsentV1(t, fixture.activePath())
}

func (fixture privateCASRecoveryJournalTestFixtureV1) appendCompletedLifecycle(
	t *testing.T,
) privateCASRecoveryJournalLifecycleV1 {
	t.Helper()
	ctx := context.Background()
	preparation, err := fixture.journal.BeginPreparationAfterValidatedPreflight(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	target, err := domainprivatecas.NewRecoveryTargetEntryV1(domainprivatecas.RecoveryTargetEntryInputV1{
		PlanIndex: 0, RootID: "accepted-finals/records",
		PlanAuthorityDigest: privateCASRecoveryTestDigestV1("plan-authority"),
		ShardName:           "aa", ShardIdentityDigest: privateCASRecoveryTestDigestV1("shard-identity"),
		OriginalName:         privateCASRecoveryTempNameV1("a", "target"),
		ObjectIdentityDigest: privateCASRecoveryTestDigestV1("object-identity"),
		BodySHA256:           privateCASRecoveryTestDigestV1("body"), ByteLength: 17,
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := domainprivatecas.BuildRecoveryTargetChunksV1([]domainprivatecas.RecoveryTargetEntryV1{target})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.PutTargetChunkIfAbsent(ctx, chunks[0]); err != nil {
		t.Fatal(err)
	}
	manifestDraft, err := domainprivatecas.NewRecoveryJournalManifestDraftV1(
		domainprivatecas.RecoveryJournalManifestInputV1{
			Preparation: preparation, Chunks: chunks, ManifestedAt: fixture.request.PreparedAt.Add(time.Second),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest := sealPrivateCASRecoveryManifestV1(t, fixture.journal, manifestDraft)
	if err := fixture.journal.PutManifestIfAbsent(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	witnessDraft, err := domainprivatecas.NewRecoveryJournalCommitWitnessDraftV1(
		domainprivatecas.RecoveryJournalCommitWitnessInputV1{
			Manifest: manifest, Chunks: chunks, CommitTargetID: target.TargetID,
			WitnessedAt: fixture.request.PreparedAt.Add(2 * time.Second),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	witness := sealPrivateCASRecoveryWitnessV1(t, fixture.journal, witnessDraft)
	if err := fixture.journal.PutCommitWitnessIfAbsent(ctx, witness); err != nil {
		t.Fatal(err)
	}
	completionDraft, err := domainprivatecas.NewRecoveryJournalCompletionDraftV1(
		domainprivatecas.RecoveryJournalCompletionInputV1{
			Manifest: manifest, CommitWitness: witness,
			FinalInventoryDigest: privateCASRecoveryTestDigestV1("final-inventory"),
			CompletedAt:          fixture.request.PreparedAt.Add(3 * time.Second),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion := sealPrivateCASRecoveryCompletionV1(t, fixture.journal, completionDraft)
	if err := fixture.journal.PutCompletionReceiptIfAbsent(ctx, completion); err != nil {
		t.Fatal(err)
	}
	return privateCASRecoveryJournalLifecycleV1{
		preparation: preparation, chunks: chunks, manifest: manifest, witness: witness, completion: completion,
	}
}

func newPrivateCASRecoveryJournalTestFixtureV1(t *testing.T) privateCASRecoveryJournalTestFixtureV1 {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(config, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireCompositeLeaseAt(roots, filepath.Join(base, "leases"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Errorf("close lease: %v", err)
		}
	})
	if _, err := lease.createJournalAuthorityAfterPreflightV1(); err != nil {
		t.Fatal(err)
	}
	journalPort, err := NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		t.Fatal(err)
	}
	journal, ok := journalPort.(*privateCASRecoveryJournalV1)
	if !ok {
		t.Fatalf("journal type = %T", journalPort)
	}
	return privateCASRecoveryJournalTestFixtureV1{
		journal: journal, lease: lease, namespace: journal.journalAuthority.path(),
		request: privatecasport.RecoveryPreparationRequestV1{
			AuthoritySetDigest: privateCASRecoveryTestDigestV1("authority-set"),
			ParticipantCount:   1, PlanCount: 1, TopologyCount: 1,
			PreparedAt: time.Date(2026, 7, 18, 8, 0, 0, 123, time.UTC),
		},
	}
}

func (fixture privateCASRecoveryJournalTestFixtureV1) activePath() string {
	return filepath.Join(fixture.namespace, privateCASRecoveryJournalDirectoryV1)
}

func (fixture privateCASRecoveryJournalTestFixtureV1) authorityKeyPath() string {
	return startupJournalAuthorityPath(fixture.journal.authorityAnchorLocked())
}

func sealPrivateCASRecoveryManifestV1(
	t *testing.T,
	journal *privateCASRecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalManifestV1,
) domainprivatecas.RecoveryJournalManifestV1 {
	t.Helper()
	message, err := domainprivatecas.RecoveryJournalManifestSigningBytesV1(draft)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := journal.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := domainprivatecas.SealRecoveryJournalManifestV1(draft, signature)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sealPrivateCASRecoveryWitnessV1(
	t *testing.T,
	journal *privateCASRecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalCommitWitnessV1,
) domainprivatecas.RecoveryJournalCommitWitnessV1 {
	t.Helper()
	message, err := domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1(draft)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := journal.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := domainprivatecas.SealRecoveryJournalCommitWitnessV1(draft, signature)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sealPrivateCASRecoveryCompletionV1(
	t *testing.T,
	journal *privateCASRecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalCompletionReceiptV1,
) domainprivatecas.RecoveryJournalCompletionReceiptV1 {
	t.Helper()
	message, err := domainprivatecas.RecoveryJournalCompletionSigningBytesV1(draft)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := journal.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := domainprivatecas.SealRecoveryJournalCompletionV1(draft, signature)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func privateCASRecoveryTempNameV1(digestPrefix, nonce string) string {
	digest := strings.Repeat(digestPrefix, 64)
	nonceDigest := sha256.Sum256([]byte(nonce))
	return "." + digest + ".json-" + base64.RawURLEncoding.EncodeToString(nonceDigest[:18]) + ".tmp"
}

func privateCASRecoveryTestDigestV1(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmtDigestV1(digest[:])
}

func fmtDigestV1(value []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, current := range value {
		result[index*2] = alphabet[current>>4]
		result[index*2+1] = alphabet[current&0x0f]
	}
	return string(result)
}

func assertPrivateCASRecoveryJournalPathAbsentV1(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q should be absent, got %v", path, err)
	}
}
