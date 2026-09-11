package privatecastopology

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

type recoveryJournalFixtureV1 struct {
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	preparation RecoveryJournalPreparationV1
	chunks      []RecoveryTargetChunkV1
	manifest    RecoveryJournalManifestV1
	witness     RecoveryJournalCommitWitnessV1
	completion  RecoveryJournalCompletionReceiptV1
}

func TestRecoveryJournalCanonicalSignedLifecycle(t *testing.T) {
	fixture := newRecoveryJournalFixtureV1(t)

	if err := ValidateRecoveryJournalSessionV1(RecoveryJournalSessionV1{
		State: RecoveryJournalSessionCompletedV1, Preparation: fixture.preparation,
		Chunks: fixture.chunks, Manifest: &fixture.manifest, CommitWitness: &fixture.witness,
		CompletionReceipt: &fixture.completion,
	}); err != nil {
		t.Fatalf("ValidateRecoveryJournalSessionV1() error = %v", err)
	}

	preparationBody, err := RecoveryJournalPreparationV1Bytes(fixture.preparation)
	if err != nil {
		t.Fatal(err)
	}
	parsedPreparation, err := ParseRecoveryJournalPreparationV1(preparationBody)
	if err != nil || parsedPreparation != fixture.preparation {
		t.Fatalf("preparation round trip = %#v, %v", parsedPreparation, err)
	}

	manifestBody, err := RecoveryJournalManifestV1Bytes(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsedManifest, err := ParseRecoveryJournalManifestV1(manifestBody)
	if err != nil || parsedManifest.ManifestDigest != fixture.manifest.ManifestDigest ||
		parsedManifest.TransactionID != fixture.manifest.TransactionID {
		t.Fatalf("manifest round trip = %#v, %v", parsedManifest, err)
	}

	witnessBody, err := RecoveryJournalCommitWitnessV1Bytes(fixture.witness)
	if err != nil {
		t.Fatal(err)
	}
	parsedWitness, err := ParseRecoveryJournalCommitWitnessV1(witnessBody)
	if err != nil || parsedWitness != fixture.witness {
		t.Fatalf("witness round trip = %#v, %v", parsedWitness, err)
	}

	completionBody, err := RecoveryJournalCompletionReceiptV1Bytes(fixture.completion)
	if err != nil {
		t.Fatal(err)
	}
	parsedCompletion, err := ParseRecoveryJournalCompletionReceiptV1(completionBody)
	if err != nil || parsedCompletion != fixture.completion {
		t.Fatalf("completion round trip = %#v, %v", parsedCompletion, err)
	}

	for name, material := range map[string]struct {
		bytes     []byte
		signature string
	}{
		"preparation": {mustPreparationSigningBytesV1(t, fixture.preparation), fixture.preparation.AuthoritySignature},
		"manifest":    {mustManifestSigningBytesV1(t, fixture.manifest), fixture.manifest.AuthoritySignature},
		"witness":     {mustWitnessSigningBytesV1(t, fixture.witness), fixture.witness.AuthoritySignature},
		"completion":  {mustCompletionSigningBytesV1(t, fixture.completion), fixture.completion.AuthoritySignature},
	} {
		signature, err := base64.RawURLEncoding.DecodeString(material.signature)
		if err != nil || !ed25519.Verify(fixture.publicKey, material.bytes, signature) {
			t.Fatalf("%s signature did not verify", name)
		}
	}
}

func TestRecoveryJournalTransactionAndTargetSetAreDeterministic(t *testing.T) {
	fixture := newRecoveryJournalFixtureV1(t)
	root, count, err := RecoveryTargetSetRootV1(fixture.chunks, fixture.preparation.PlanCount)
	if err != nil || root != fixture.manifest.TargetSetRoot || count != fixture.manifest.TargetCount {
		t.Fatalf("target set = %q, %d, %v", root, count, err)
	}

	reordered := []RecoveryTargetEntryV1{
		fixture.chunks[0].Entries[1], fixture.chunks[0].Entries[0],
	}
	rebuilt, err := BuildRecoveryTargetChunksV1(reordered)
	if err != nil || len(rebuilt) != 1 || rebuilt[0].ChunkDigest != fixture.chunks[0].ChunkDigest {
		t.Fatalf("rebuilt chunks = %#v, %v", rebuilt, err)
	}

	draft, err := NewRecoveryJournalManifestDraftV1(RecoveryJournalManifestInputV1{
		Preparation: fixture.preparation, Chunks: fixture.chunks,
		ManifestedAt: mustRecoveryTimeV1(t, fixture.manifest.ManifestedAt),
	})
	if err != nil || draft.TransactionID != fixture.manifest.TransactionID {
		t.Fatalf("deterministic transaction = %q, %v", draft.TransactionID, err)
	}

	otherSignature := base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	resigned, err := SealRecoveryJournalManifestV1(draft, otherSignature)
	if err != nil || resigned.TransactionID != fixture.manifest.TransactionID ||
		resigned.ManifestDigest == fixture.manifest.ManifestDigest {
		t.Fatalf("signature-independent transaction = %#v, %v", resigned, err)
	}
}

func TestRecoveryJournalRejectsUnsafeOrNonCanonicalTargets(t *testing.T) {
	fixture := newRecoveryJournalFixtureV1(t)
	base := fixture.chunks[0].Entries[0]

	for name, mutate := range map[string]func(*RecoveryTargetEntryV1){
		"absolute root": func(entry *RecoveryTargetEntryV1) { entry.RootID = "/tmp/private" },
		"unknown root":  func(entry *RecoveryTargetEntryV1) { entry.RootID = "unknown/records" },
		"root traversal": func(entry *RecoveryTargetEntryV1) {
			entry.RootID = "accepted-finals/../records"
		},
		"uppercase authority digest": func(entry *RecoveryTargetEntryV1) {
			entry.PlanAuthorityDigest = strings.ToUpper(entry.PlanAuthorityDigest)
		},
		"wrong shard": func(entry *RecoveryTargetEntryV1) { entry.ShardName = "ff" },
		"recovery-stage name": func(entry *RecoveryTargetEntryV1) {
			entry.OriginalName = RecoveryStagePrefixV1 + strings.Repeat("a", 64) + "-bad"
		},
		"oversize body": func(entry *RecoveryTargetEntryV1) { entry.ByteLength = MaxRecoveryTargetBodyBytesV1 + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			entry := base
			mutate(&entry)
			entry.TargetID = recoveryTargetIDV1(entry)
			if err := ValidateRecoveryTargetEntryV1(entry); err == nil {
				t.Fatal("invalid recovery target was accepted")
			}
		})
	}

	duplicate := base
	duplicate.Linked = !duplicate.Linked
	duplicate.TargetID = recoveryTargetIDV1(duplicate)
	if _, err := BuildRecoveryTargetChunksV1([]RecoveryTargetEntryV1{base, duplicate}); err == nil {
		t.Fatal("two target ids for one physical residue were accepted")
	}

	gap := fixture.chunks[0].Entries[1]
	gap.PlanIndex = 2
	gap.TargetID = recoveryTargetIDV1(gap)
	gapChunks, err := BuildRecoveryTargetChunksV1([]RecoveryTargetEntryV1{base, gap})
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := RecoveryTargetSetRootV1(gapChunks, 3); err != nil || count != 2 {
		t.Fatalf("sparse target-bearing plans were rejected: count=%d err=%v", count, err)
	}
	if _, _, err := RecoveryTargetSetRootV1(gapChunks, 2); err == nil {
		t.Fatal("out-of-range target plan index was accepted")
	}

	badChunk := fixture.chunks[0]
	badChunk.EntryCount++
	if err := ValidateRecoveryTargetChunkV1(badChunk); err == nil {
		t.Fatal("mismatched target chunk count was accepted")
	}
}

func TestRecoveryJournalRejectsCrossRecordAndSessionMismatch(t *testing.T) {
	fixture := newRecoveryJournalFixtureV1(t)

	otherPreparation := fixture.preparation
	otherPreparation.TargetsDirectoryIdentity = recoveryTestDigestV1("other-target-directory")
	otherPreparation.AuthoritySignature = ""
	otherPreparation.PreparationDigest = ""
	otherPreparation = mustSealPreparationV1(t, otherPreparation, fixture.privateKey)
	if err := ValidateRecoveryJournalManifestForPreparationV1(fixture.manifest, otherPreparation); err == nil {
		t.Fatal("manifest accepted another targets directory identity")
	}

	otherManifest := fixture.manifest
	otherManifest.TargetSetRoot = recoveryTestDigestV1("another-target-root")
	otherManifest.TransactionID = recoveryManifestTransactionIDV1(otherManifest)
	otherManifest.AuthoritySignature = ""
	otherManifest.ManifestDigest = ""
	otherManifest = mustSealManifestV1(t, otherManifest, fixture.privateKey)
	if err := ValidateRecoveryJournalManifestForChunksV1(otherManifest, fixture.chunks); err == nil {
		t.Fatal("manifest accepted mismatched target chunks")
	}

	otherWitness := fixture.witness
	otherWitness.ManifestDigest = recoveryTestDigestV1("other-manifest")
	otherWitness.AuthoritySignature = ""
	otherWitness.WitnessDigest = ""
	otherWitness = mustSealWitnessV1(t, otherWitness, fixture.privateKey)
	if err := ValidateRecoveryJournalCommitWitnessForManifestV1(otherWitness, fixture.manifest); err == nil {
		t.Fatal("commit witness accepted another manifest")
	}

	otherCompletion := fixture.completion
	otherCompletion.CommitWitnessDigest = recoveryTestDigestV1("other-witness")
	otherCompletion.AuthoritySignature = ""
	otherCompletion.ReceiptDigest = ""
	otherCompletion = mustSealCompletionV1(t, otherCompletion, fixture.privateKey)
	if err := ValidateRecoveryJournalCompletionForWitnessV1(otherCompletion, fixture.witness); err == nil {
		t.Fatal("completion accepted another commit witness")
	}

	if err := ValidateRecoveryJournalSessionV1(RecoveryJournalSessionV1{
		State: RecoveryJournalSessionManifestedV1, Preparation: fixture.preparation,
		Chunks: fixture.chunks, Manifest: &fixture.manifest, CommitWitness: &fixture.witness,
	}); err == nil {
		t.Fatal("manifested state accepted a commit witness")
	}
}

func TestRecoveryJournalStrictParsingRejectsUnknownDuplicateAndNonCanonicalJSON(t *testing.T) {
	fixture := newRecoveryJournalFixtureV1(t)
	body, err := RecoveryJournalManifestV1Bytes(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}

	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := ParseRecoveryJournalManifestV1(unknown); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}

	duplicate := strings.Replace(string(body), `{"schemaVersion":1`, `{"schemaVersion":1,"schemaVersion":1`, 1)
	if _, err := ParseRecoveryJournalManifestV1([]byte(duplicate)); err == nil {
		t.Fatal("duplicate manifest field was accepted")
	}
	if _, err := ParseRecoveryJournalManifestV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical whitespace was accepted")
	}
	if _, err := ParseRecoveryJournalManifestV1(append(body, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func newRecoveryJournalFixtureV1(t *testing.T) recoveryJournalFixtureV1 {
	t.Helper()
	seed := sha256.Sum256([]byte("private-cas-recovery-journal-test-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := recoveryTestDigestBytesV1(publicKey)
	baseTime := time.Date(2026, 7, 18, 8, 0, 0, 123, time.UTC)

	entries := []RecoveryTargetEntryV1{
		mustRecoveryTargetV1(t, RecoveryTargetEntryInputV1{
			PlanIndex: 1, RootID: "pending-work/receipts", PlanAuthorityDigest: recoveryTestDigestV1("plan-1"),
			ShardName: "bb", ShardIdentityDigest: recoveryTestDigestV1("shard-1"),
			OriginalName: privateRecoveryTempNameV1("b", "first"), ObjectIdentityDigest: recoveryTestDigestV1("object-1"),
			BodySHA256: recoveryTestDigestV1("body-1"), ByteLength: 42,
		}),
		mustRecoveryTargetV1(t, RecoveryTargetEntryInputV1{
			PlanIndex: 0, RootID: "accepted-finals/records", PlanAuthorityDigest: recoveryTestDigestV1("plan-0"),
			ShardName: "aa", ShardIdentityDigest: recoveryTestDigestV1("shard-0"),
			OriginalName: privateRecoveryTempNameV1("a", "second"), ObjectIdentityDigest: recoveryTestDigestV1("object-0"),
			BodySHA256: recoveryTestDigestV1("body-0"), ByteLength: 17,
		}),
	}
	chunks, err := BuildRecoveryTargetChunksV1(entries)
	if err != nil {
		t.Fatal(err)
	}
	preparationDraft, err := NewRecoveryJournalPreparationDraftV1(RecoveryJournalPreparationInputV1{
		RootBindingDigest: recoveryTestDigestV1("root-binding"), AuthoritySetDigest: recoveryTestDigestV1("authority-set"),
		ParticipantCount: 1, PlanCount: 2, TopologyCount: 1,
		JournalDirectoryIdentity: recoveryTestDigestV1("journal-directory"),
		TargetsDirectoryIdentity: recoveryTestDigestV1("targets-directory"),
		PreparedAt:               baseTime, AuthorityKeyID: keyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	preparation := mustSealPreparationV1(t, preparationDraft, privateKey)
	manifestDraft, err := NewRecoveryJournalManifestDraftV1(RecoveryJournalManifestInputV1{
		Preparation: preparation, Chunks: chunks, ManifestedAt: baseTime.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := mustSealManifestV1(t, manifestDraft, privateKey)
	witnessDraft, err := NewRecoveryJournalCommitWitnessDraftV1(RecoveryJournalCommitWitnessInputV1{
		Manifest: manifest, Chunks: chunks, CommitTargetID: chunks[0].Entries[0].TargetID,
		WitnessedAt: baseTime.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	witness := mustSealWitnessV1(t, witnessDraft, privateKey)
	completionDraft, err := NewRecoveryJournalCompletionDraftV1(RecoveryJournalCompletionInputV1{
		Manifest: manifest, CommitWitness: witness, FinalInventoryDigest: recoveryTestDigestV1("final-inventory"),
		CompletedAt: baseTime.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	completion := mustSealCompletionV1(t, completionDraft, privateKey)
	return recoveryJournalFixtureV1{
		privateKey: privateKey, publicKey: publicKey, preparation: preparation,
		chunks: chunks, manifest: manifest, witness: witness, completion: completion,
	}
}

func mustRecoveryTargetV1(t *testing.T, input RecoveryTargetEntryInputV1) RecoveryTargetEntryV1 {
	t.Helper()
	entry, err := NewRecoveryTargetEntryV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func mustSealPreparationV1(t *testing.T, draft RecoveryJournalPreparationV1, key ed25519.PrivateKey) RecoveryJournalPreparationV1 {
	t.Helper()
	bytes := mustPreparationSigningBytesV1(t, draft)
	sealed, err := SealRecoveryJournalPreparationV1(draft, recoverySignatureV1(key, bytes))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mustSealManifestV1(t *testing.T, draft RecoveryJournalManifestV1, key ed25519.PrivateKey) RecoveryJournalManifestV1 {
	t.Helper()
	bytes := mustManifestSigningBytesV1(t, draft)
	sealed, err := SealRecoveryJournalManifestV1(draft, recoverySignatureV1(key, bytes))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mustSealWitnessV1(t *testing.T, draft RecoveryJournalCommitWitnessV1, key ed25519.PrivateKey) RecoveryJournalCommitWitnessV1 {
	t.Helper()
	bytes := mustWitnessSigningBytesV1(t, draft)
	sealed, err := SealRecoveryJournalCommitWitnessV1(draft, recoverySignatureV1(key, bytes))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mustSealCompletionV1(t *testing.T, draft RecoveryJournalCompletionReceiptV1, key ed25519.PrivateKey) RecoveryJournalCompletionReceiptV1 {
	t.Helper()
	bytes := mustCompletionSigningBytesV1(t, draft)
	sealed, err := SealRecoveryJournalCompletionV1(draft, recoverySignatureV1(key, bytes))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mustPreparationSigningBytesV1(t *testing.T, value RecoveryJournalPreparationV1) []byte {
	t.Helper()
	bytes, err := RecoveryJournalPreparationSigningBytesV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func mustManifestSigningBytesV1(t *testing.T, value RecoveryJournalManifestV1) []byte {
	t.Helper()
	bytes, err := RecoveryJournalManifestSigningBytesV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func mustWitnessSigningBytesV1(t *testing.T, value RecoveryJournalCommitWitnessV1) []byte {
	t.Helper()
	bytes, err := RecoveryJournalCommitWitnessSigningBytesV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func mustCompletionSigningBytesV1(t *testing.T, value RecoveryJournalCompletionReceiptV1) []byte {
	t.Helper()
	bytes, err := RecoveryJournalCompletionSigningBytesV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func recoverySignatureV1(key ed25519.PrivateKey, body []byte) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, body))
}

func privateRecoveryTempNameV1(digestPrefix, nonce string) string {
	digest := strings.Repeat(digestPrefix, 64)
	nonceDigest := sha256.Sum256([]byte(nonce))
	return "." + digest + ".json-" + base64.RawURLEncoding.EncodeToString(nonceDigest[:18]) + ".tmp"
}

func recoveryTestDigestV1(value string) string {
	return recoveryTestDigestBytesV1([]byte(value))
}

func recoveryTestDigestBytesV1(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func mustRecoveryTimeV1(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
