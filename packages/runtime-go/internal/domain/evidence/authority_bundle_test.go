package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type evidenceAuthorityBundleTestKeys struct {
	installationID   string
	enrollmentID     string
	authorityKeyID   string
	authorityPublic  ed25519.PublicKey
	authorityPrivate ed25519.PrivateKey
	witnessKeyID     string
	witnessPublic    ed25519.PublicKey
	witnessPrivate   ed25519.PrivateKey
}

func newEvidenceAuthorityBundleTestKeys(label string) evidenceAuthorityBundleTestKeys {
	authoritySeed := sha256.Sum256([]byte("evidence-authority-bundle-authority:" + label))
	authorityPrivate := ed25519.NewKeyFromSeed(authoritySeed[:])
	authorityPublic := append(ed25519.PublicKey(nil), authorityPrivate.Public().(ed25519.PublicKey)...)
	witnessSeed := sha256.Sum256([]byte("evidence-authority-bundle-witness:" + label))
	witnessPrivate := ed25519.NewKeyFromSeed(witnessSeed[:])
	witnessPublic := append(ed25519.PublicKey(nil), witnessPrivate.Public().(ed25519.PublicKey)...)
	return evidenceAuthorityBundleTestKeys{
		installationID:  domainsecurity.SHA256Hex([]byte("installation:" + label)),
		enrollmentID:    domainsecurity.SHA256Hex([]byte("enrollment:" + label)),
		authorityKeyID:  domainsecurity.SHA256Hex(authorityPublic),
		authorityPublic: authorityPublic, authorityPrivate: authorityPrivate,
		witnessKeyID:  domainsecurity.SHA256Hex(witnessPublic),
		witnessPublic: witnessPublic, witnessPrivate: witnessPrivate,
	}
}

func (keys evidenceAuthorityBundleTestKeys) authoritySign(message []byte) ([]byte, error) {
	return ed25519.Sign(keys.authorityPrivate, message), nil
}

func (keys evidenceAuthorityBundleTestKeys) witnessSign(message []byte) ([]byte, error) {
	return ed25519.Sign(keys.witnessPrivate, message), nil
}

func evidenceAuthorityTestDigest(label string) string {
	return domainsecurity.SHA256Hex([]byte(label))
}

func evidenceAuthorityTestInput(keys evidenceAuthorityBundleTestKeys) EvidenceAuthorityBundleInputV1 {
	return EvidenceAuthorityBundleInputV1{
		InstallationID: keys.installationID, EnrollmentID: keys.enrollmentID, Generation: 1,
		MutationID:                 evidenceAuthorityTestDigest("mutation-1"),
		DatasetSnapshotIndexDigest: evidenceAuthorityTestDigest("dataset-index-0"), DatasetSnapshotCount: 0,
		EvidenceRegistryIndexDigest: evidenceAuthorityTestDigest("registry-index-0"), EvidenceRegistryCount: 0,
		PublicationIndexDigest: evidenceAuthorityTestDigest("publication-index-0"), PublicationCount: 0,
		AuthorityKeyID: keys.authorityKeyID, AuthorityPublicKey: keys.authorityPublic,
	}
}

func newEvidenceAuthorityTestBundle(t *testing.T, keys evidenceAuthorityBundleTestKeys, input EvidenceAuthorityBundleInputV1) EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle, err := NewEvidenceAuthorityBundleV1(input, keys.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func nextEvidenceAuthorityTestBundle(t *testing.T, keys evidenceAuthorityBundleTestKeys, previous EvidenceAuthorityBundleV1, child, mutation string) EvidenceAuthorityBundleV1 {
	t.Helper()
	input := EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                 evidenceAuthorityTestDigest(mutation),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: keys.authorityKeyID, AuthorityPublicKey: keys.authorityPublic,
	}
	switch child {
	case "dataset":
		input.DatasetSnapshotIndexDigest = evidenceAuthorityTestDigest("dataset:" + mutation)
		input.DatasetSnapshotCount++
	case "registry":
		input.EvidenceRegistryIndexDigest = evidenceAuthorityTestDigest("registry:" + mutation)
		input.EvidenceRegistryCount++
	case "publication":
		input.PublicationIndexDigest = evidenceAuthorityTestDigest("publication:" + mutation)
		input.PublicationCount++
	case "none":
	default:
		t.Fatalf("unknown child %q", child)
	}
	return newEvidenceAuthorityTestBundle(t, keys, input)
}

func resignEvidenceAuthorityTestBundle(t *testing.T, keys evidenceAuthorityBundleTestKeys, bundle EvidenceAuthorityBundleV1) EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle = signEvidenceAuthorityTestBundleUnchecked(t, keys, bundle)
	if err := ValidateEvidenceAuthorityBundleV1(bundle); err != nil {
		t.Fatalf("resigned bundle is not individually valid: %v", err)
	}
	return bundle
}

func signEvidenceAuthorityTestBundleUnchecked(t *testing.T, keys evidenceAuthorityBundleTestKeys, bundle EvidenceAuthorityBundleV1) EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle.AuthoritySignature = ""
	bundle.RecordDigest = ""
	signature, err := keys.authoritySign(EvidenceAuthorityBundleSigningBytesV1(bundle))
	if err != nil {
		t.Fatal(err)
	}
	bundle.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	bundle.RecordDigest = evidenceAuthorityBundleDigestV1(bundle)
	return bundle
}

func TestEvidenceAuthorityBundleCanonicalClosedRoundTrip(t *testing.T) {
	keys := newEvidenceAuthorityBundleTestKeys("canonical")
	bundle := newEvidenceAuthorityTestBundle(t, keys, evidenceAuthorityTestInput(keys))
	body, err := EvidenceAuthorityBundleV1Bytes(bundle)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEvidenceAuthorityBundleV1(body)
	if err != nil || parsed != bundle {
		t.Fatalf("canonical round trip failed: parsed=%+v err=%v", parsed, err)
	}
	reencoded, err := EvidenceAuthorityBundleV1Bytes(parsed)
	if err != nil || !bytes.Equal(body, reencoded) {
		t.Fatal("canonical bundle bytes were not stable")
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["attackerField"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseEvidenceAuthorityBundleV1(unknown); err == nil {
		t.Fatal("unknown JSON field was accepted")
	}
	if _, err := ParseEvidenceAuthorityBundleV1(append(append([]byte(nil), body...), []byte(`{}`)...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	if _, err := ParseEvidenceAuthorityBundleV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical leading whitespace was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"purpose":`), []byte(`"purpose":"attacker","purpose":`), 1)
	if _, err := ParseEvidenceAuthorityBundleV1(duplicate); err == nil {
		t.Fatal("duplicate JSON field was accepted")
	}
	overflow := bytes.Replace(body, []byte(`"datasetSnapshotCount":0`), []byte(`"datasetSnapshotCount":18446744073709551616`), 1)
	if _, err := ParseEvidenceAuthorityBundleV1(overflow); err == nil {
		t.Fatal("overflowing JSON child count was accepted")
	}

	nonCanonicalDigest := bundle
	nonCanonicalDigest.MutationID = " " + nonCanonicalDigest.MutationID
	nonCanonicalDigest = signEvidenceAuthorityTestBundleUnchecked(t, keys, nonCanonicalDigest)
	if err := ValidateEvidenceAuthorityBundleV1(nonCanonicalDigest); err == nil {
		t.Fatal("whitespace-padded signed digest field was accepted")
	}
	uppercaseDigest := bundle
	uppercaseDigest.DatasetSnapshotIndexDigest = strings.ToUpper(uppercaseDigest.DatasetSnapshotIndexDigest)
	uppercaseDigest = signEvidenceAuthorityTestBundleUnchecked(t, keys, uppercaseDigest)
	if err := ValidateEvidenceAuthorityBundleV1(uppercaseDigest); err == nil {
		t.Fatal("uppercase signed digest field was accepted")
	}
}

func TestEvidenceAuthorityBundleAuthenticityIsSeparateFromInstallationAnchor(t *testing.T) {
	trusted := newEvidenceAuthorityBundleTestKeys("trusted")
	attacker := newEvidenceAuthorityBundleTestKeys("attacker")
	trustedBundle := newEvidenceAuthorityTestBundle(t, trusted, evidenceAuthorityTestInput(trusted))
	if err := ValidateEvidenceAuthorityBundleForInstallationV1(
		trustedBundle, trusted.installationID, trusted.enrollmentID, trusted.authorityKeyID, trusted.authorityPublic,
	); err != nil {
		t.Fatal(err)
	}

	attackerInput := evidenceAuthorityTestInput(attacker)
	attackerInput.InstallationID = trusted.installationID
	attackerInput.EnrollmentID = trusted.enrollmentID
	attackerBundle := newEvidenceAuthorityTestBundle(t, attacker, attackerInput)
	if err := ValidateEvidenceAuthorityBundleV1(attackerBundle); err != nil {
		t.Fatal("attacker bundle should remain self-authentic under its own key")
	}
	if err := ValidateEvidenceAuthorityBundleForInstallationV1(
		attackerBundle, trusted.installationID, trusted.enrollmentID, trusted.authorityKeyID, trusted.authorityPublic,
	); err == nil {
		t.Fatal("attacker authority passed the trusted installation anchor")
	}
	if err := ValidateEvidenceAuthorityBundleForInstallationV1(
		trustedBundle, trusted.installationID, attacker.enrollmentID, trusted.authorityKeyID, trusted.authorityPublic,
	); err == nil {
		t.Fatal("wrong enrollment anchor was accepted")
	}
	if err := ValidateEvidenceAuthorityBundleForInstallationV1(
		trustedBundle, attacker.installationID, trusted.enrollmentID, trusted.authorityKeyID, trusted.authorityPublic,
	); err == nil {
		t.Fatal("wrong installation anchor was accepted")
	}

	tampered := trustedBundle
	tampered.AuthorityPublicKey = attackerBundle.AuthorityPublicKey
	if err := ValidateEvidenceAuthorityBundleV1(tampered); err == nil {
		t.Fatal("attacker public key substitution was accepted")
	}
	tampered = trustedBundle
	tampered.DatasetSnapshotIndexDigest = evidenceAuthorityTestDigest("unsigned-tamper")
	if err := ValidateEvidenceAuthorityBundleV1(tampered); err == nil {
		t.Fatal("unsigned child digest tamper was accepted")
	}
	tampered = trustedBundle
	tampered.RecordDigest = evidenceAuthorityTestDigest("wrong-record-digest")
	if err := ValidateEvidenceAuthorityBundleV1(tampered); err == nil {
		t.Fatal("wrong complete record digest was accepted")
	}
}

func TestEvidenceAuthorityBundleRejectsZeroGenerationAndAllowsExplicitEmptyChildIndices(t *testing.T) {
	keys := newEvidenceAuthorityBundleTestKeys("zero")
	input := evidenceAuthorityTestInput(keys)
	input.Generation = 0
	if _, err := NewEvidenceAuthorityBundleV1(input, keys.authoritySign); err == nil {
		t.Fatal("zero bundle generation was accepted")
	}
	input = evidenceAuthorityTestInput(keys)
	input.MutationID = ""
	if _, err := NewEvidenceAuthorityBundleV1(input, keys.authoritySign); err == nil {
		t.Fatal("empty mutation ID was accepted")
	}
	input = evidenceAuthorityTestInput(keys)
	bundle := newEvidenceAuthorityTestBundle(t, keys, input)
	if bundle.DatasetSnapshotCount != 0 || bundle.EvidenceRegistryCount != 0 || bundle.PublicationCount != 0 {
		t.Fatal("explicit empty child-index counts were changed")
	}
}

func TestEvidenceAuthorityBundleTransitionAdvancesExactlyOneChildPair(t *testing.T) {
	keys := newEvidenceAuthorityBundleTestKeys("transition")
	previous := newEvidenceAuthorityTestBundle(t, keys, evidenceAuthorityTestInput(keys))
	for _, child := range []string{"dataset", "registry", "publication"} {
		next := nextEvidenceAuthorityTestBundle(t, keys, previous, child, "valid-"+child)
		if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err != nil {
			t.Fatalf("valid %s transition failed: %v", child, err)
		}
	}

	noChange := nextEvidenceAuthorityTestBundle(t, keys, previous, "none", "no-child-change")
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, noChange); err == nil {
		t.Fatal("zero-child transition was accepted")
	}
	twoChanges := nextEvidenceAuthorityTestBundle(t, keys, previous, "dataset", "two-changes")
	twoChanges.EvidenceRegistryIndexDigest = evidenceAuthorityTestDigest("registry:two-changes")
	twoChanges.EvidenceRegistryCount++
	twoChanges = resignEvidenceAuthorityTestBundle(t, keys, twoChanges)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, twoChanges); err == nil {
		t.Fatal("two-child transition was accepted")
	}
}

func TestEvidenceAuthorityBundleTransitionRejectsCountDigestMismatchAndOverflow(t *testing.T) {
	keys := newEvidenceAuthorityBundleTestKeys("count")
	previous := newEvidenceAuthorityTestBundle(t, keys, evidenceAuthorityTestInput(keys))
	valid := nextEvidenceAuthorityTestBundle(t, keys, previous, "dataset", "count-valid")

	digestOnly := valid
	digestOnly.DatasetSnapshotCount = previous.DatasetSnapshotCount
	digestOnly = resignEvidenceAuthorityTestBundle(t, keys, digestOnly)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, digestOnly); err == nil {
		t.Fatal("digest-only child change was accepted")
	}
	countOnly := valid
	countOnly.DatasetSnapshotIndexDigest = previous.DatasetSnapshotIndexDigest
	countOnly = resignEvidenceAuthorityTestBundle(t, keys, countOnly)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, countOnly); err == nil {
		t.Fatal("count-only child change was accepted")
	}
	batchCount := valid
	batchCount.DatasetSnapshotCount = previous.DatasetSnapshotCount + 2
	batchCount = resignEvidenceAuthorityTestBundle(t, keys, batchCount)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, batchCount); err == nil {
		t.Fatal("batch child count change was accepted")
	}

	overflowInput := evidenceAuthorityTestInput(keys)
	overflowInput.DatasetSnapshotCount = math.MaxUint64
	overflowPrevious := newEvidenceAuthorityTestBundle(t, keys, overflowInput)
	overflowNext := nextEvidenceAuthorityTestBundle(t, keys, overflowPrevious, "none", "overflow")
	overflowNext.DatasetSnapshotIndexDigest = evidenceAuthorityTestDigest("dataset:overflow")
	overflowNext.DatasetSnapshotCount = 0
	overflowNext = resignEvidenceAuthorityTestBundle(t, keys, overflowNext)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(overflowPrevious, overflowNext); err == nil {
		t.Fatal("overflowing child count was accepted")
	}

	maxGenerationInput := evidenceAuthorityTestInput(keys)
	maxGenerationInput.Generation = math.MaxUint64
	maxGenerationInput.PreviousBundleDigest = evidenceAuthorityTestDigest("prior-to-max")
	maxGeneration := newEvidenceAuthorityTestBundle(t, keys, maxGenerationInput)
	wrapped := maxGeneration
	wrapped.Generation = 1
	wrapped.PreviousBundleDigest = ""
	wrapped.MutationID = evidenceAuthorityTestDigest("wrapped-generation")
	wrapped.DatasetSnapshotIndexDigest = evidenceAuthorityTestDigest("wrapped-dataset")
	wrapped.DatasetSnapshotCount++
	wrapped = resignEvidenceAuthorityTestBundle(t, keys, wrapped)
	if err := ValidateEvidenceAuthorityBundleTransitionV1(maxGeneration, wrapped); err == nil {
		t.Fatal("overflowing bundle generation was accepted")
	}
}

func TestEvidenceAuthorityBundleTransitionRejectsWrongLineageAndMutation(t *testing.T) {
	keys := newEvidenceAuthorityBundleTestKeys("lineage")
	previous := newEvidenceAuthorityTestBundle(t, keys, evidenceAuthorityTestInput(keys))
	valid := nextEvidenceAuthorityTestBundle(t, keys, previous, "registry", "lineage-valid")

	tests := []struct {
		name   string
		mutate func(*EvidenceAuthorityBundleV1)
	}{
		{name: "predecessor", mutate: func(next *EvidenceAuthorityBundleV1) {
			next.PreviousBundleDigest = evidenceAuthorityTestDigest("wrong-predecessor")
		}},
		{name: "generation", mutate: func(next *EvidenceAuthorityBundleV1) { next.Generation++ }},
		{name: "mutation replay", mutate: func(next *EvidenceAuthorityBundleV1) { next.MutationID = previous.MutationID }},
		{name: "installation", mutate: func(next *EvidenceAuthorityBundleV1) {
			next.InstallationID = evidenceAuthorityTestDigest("wrong-installation")
		}},
		{name: "enrollment", mutate: func(next *EvidenceAuthorityBundleV1) {
			next.EnrollmentID = evidenceAuthorityTestDigest("wrong-enrollment")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := valid
			test.mutate(&next)
			next = resignEvidenceAuthorityTestBundle(t, keys, next)
			if err := ValidateEvidenceAuthorityBundleTransitionV1(previous, next); err == nil {
				t.Fatal("invalid lineage transition was accepted")
			}
		})
	}
}

type evidenceAuthorityWitnessFixture struct {
	keys       evidenceAuthorityBundleTestKeys
	enrollment domainsecurity.MonotonicHeadCheckpointV1
}

func newEvidenceAuthorityWitnessFixture(t *testing.T, label string) evidenceAuthorityWitnessFixture {
	t.Helper()
	keys := newEvidenceAuthorityBundleTestKeys(label)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: keys.installationID, EnrollmentID: keys.enrollmentID,
		Namespace: EvidenceAuthorityBundleWitnessNamespaceV1, Generation: 0,
		CurrentStateDigest: evidenceAuthorityTestDigest("enrollment-state:" + label),
		FenceNonce:         evidenceAuthorityTestDigest("enrollment-fence:" + label),
		WitnessKeyID:       keys.witnessKeyID, WitnessPublicKey: keys.witnessPublic,
	}, keys.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return evidenceAuthorityWitnessFixture{keys: keys, enrollment: checkpoint}
}

func evidenceAuthorityWitnessAdvance(
	t *testing.T,
	fixture evidenceAuthorityWitnessFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	next EvidenceAuthorityBundleV1,
	fenceLabel string,
) (domainsecurity.MonotonicHeadAdvanceRequestV1, domainsecurity.MonotonicHeadAdvanceReceiptV1) {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.keys.enrollmentID,
		Namespace:          EvidenceAuthorityBundleWitnessNamespaceV1,
		ExpectedGeneration: previous.Generation, ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest: previous.CurrentStateDigest, NextGeneration: next.Generation, NextStateDigest: next.RecordDigest,
		ExpectedFenceNonce: previous.FenceNonce, MutationID: next.MutationID,
		AuthorityKeyID: fixture.keys.authorityKeyID, AuthorityPublicKey: fixture.keys.authorityPublic,
	}, fixture.keys.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.keys.enrollmentID,
		Namespace: EvidenceAuthorityBundleWitnessNamespaceV1, Generation: request.NextGeneration,
		CurrentStateDigest: request.NextStateDigest, PreviousStateDigest: previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest, FenceNonce: evidenceAuthorityTestDigest(fenceLabel),
		MutationID: request.MutationID, WitnessKeyID: fixture.keys.witnessKeyID, WitnessPublicKey: fixture.keys.witnessPublic,
	}, fixture.keys.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, checkpoint, fixture.keys.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return request, receipt
}

func TestEvidenceAuthorityBundleCheckpointAndFirstNextWitnessAdvance(t *testing.T) {
	fixture := newEvidenceAuthorityWitnessFixture(t, "witness")
	first := newEvidenceAuthorityTestBundle(t, fixture.keys, evidenceAuthorityTestInput(fixture.keys))
	firstRequest, firstReceipt := evidenceAuthorityWitnessAdvance(t, fixture, fixture.enrollment, first, "fence-1")
	if err := ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(first, fixture.enrollment, firstRequest, firstReceipt); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(first, firstReceipt.Checkpoint); err != nil {
		t.Fatal(err)
	}

	second := nextEvidenceAuthorityTestBundle(t, fixture.keys, first, "dataset", "mutation-2")
	secondRequest, secondReceipt := evidenceAuthorityWitnessAdvance(t, fixture, firstReceipt.Checkpoint, second, "fence-2")
	if err := ValidateEvidenceAuthorityBundleWitnessAdvanceV1(first, second, firstReceipt.Checkpoint, secondRequest, secondReceipt); err != nil {
		t.Fatal(err)
	}

	wrongStateCheckpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.keys.enrollmentID,
		Namespace: EvidenceAuthorityBundleWitnessNamespaceV1, Generation: second.Generation,
		CurrentStateDigest: evidenceAuthorityTestDigest("wrong-state"), PreviousStateDigest: first.RecordDigest,
		PreviousCheckpointDigest: firstReceipt.Checkpoint.CheckpointDigest, FenceNonce: evidenceAuthorityTestDigest("wrong-fence"),
		MutationID: second.MutationID, WitnessKeyID: fixture.keys.witnessKeyID, WitnessPublicKey: fixture.keys.witnessPublic,
	}, fixture.keys.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(second, wrongStateCheckpoint); err == nil {
		t.Fatal("checkpoint for a different bundle state was accepted")
	}

	wrongFirstRequest := firstRequest
	wrongFirstRequest.NextStateDigest = evidenceAuthorityTestDigest("wrong-first-state")
	if err := ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(first, fixture.enrollment, wrongFirstRequest, firstReceipt); err == nil {
		t.Fatal("tampered first witness request was accepted")
	}
	wrongNext := nextEvidenceAuthorityTestBundle(t, fixture.keys, first, "publication", "mutation-wrong-next")
	if err := ValidateEvidenceAuthorityBundleWitnessAdvanceV1(first, wrongNext, firstReceipt.Checkpoint, secondRequest, secondReceipt); err == nil {
		t.Fatal("witness receipt was reused for a different next bundle")
	}
}

func TestEvidenceAuthorityBundleIdempotentReplayMustBeByteExact(t *testing.T) {
	fixture := newEvidenceAuthorityWitnessFixture(t, "replay")
	first := newEvidenceAuthorityTestBundle(t, fixture.keys, evidenceAuthorityTestInput(fixture.keys))
	request, receipt := evidenceAuthorityWitnessAdvance(t, fixture, fixture.enrollment, first, "replay-fence-1")
	if err := ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(first, fixture.enrollment, request, receipt); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceAuthorityBundleIdempotentReplayV1(first, request, receipt, first, request, receipt); err != nil {
		t.Fatal(err)
	}

	differentBundle := first
	differentBundle.DatasetSnapshotIndexDigest = evidenceAuthorityTestDigest("replay-different-dataset")
	differentBundle.DatasetSnapshotCount++
	differentBundle = resignEvidenceAuthorityTestBundle(t, fixture.keys, differentBundle)
	if err := ValidateEvidenceAuthorityBundleIdempotentReplayV1(first, request, receipt, differentBundle, request, receipt); err == nil {
		t.Fatal("same witness replay was accepted with different bundle bytes")
	}

	differentRequest, differentReceipt := evidenceAuthorityWitnessAdvance(t, fixture, fixture.enrollment, differentBundle, "replay-fence-2")
	if differentRequest.MutationID != request.MutationID {
		t.Fatal("replay test did not reuse the same mutation ID")
	}
	if err := ValidateEvidenceAuthorityBundleIdempotentReplayV1(first, request, receipt, differentBundle, differentRequest, differentReceipt); err == nil {
		t.Fatal("mutation ID reuse with different request bytes was accepted")
	}
}
