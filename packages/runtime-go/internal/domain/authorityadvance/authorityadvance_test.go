package authorityadvance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type authorityAdvanceTestFixture struct {
	installationID   string
	enrollmentID     string
	authorityKeyID   string
	authorityPublic  ed25519.PublicKey
	authorityPrivate ed25519.PrivateKey
	witnessKeyID     string
	witnessPublic    ed25519.PublicKey
	witnessPrivate   ed25519.PrivateKey
}

func newAuthorityAdvanceTestFixture(label string) authorityAdvanceTestFixture {
	authoritySeed := sha256.Sum256([]byte("authority-advance-authority:" + label))
	authorityPrivate := ed25519.NewKeyFromSeed(authoritySeed[:])
	authorityPublic := append(ed25519.PublicKey(nil), authorityPrivate.Public().(ed25519.PublicKey)...)
	witnessSeed := sha256.Sum256([]byte("authority-advance-witness:" + label))
	witnessPrivate := ed25519.NewKeyFromSeed(witnessSeed[:])
	witnessPublic := append(ed25519.PublicKey(nil), witnessPrivate.Public().(ed25519.PublicKey)...)
	return authorityAdvanceTestFixture{
		installationID:   authorityAdvanceTestDigest("installation:" + label),
		enrollmentID:     authorityAdvanceTestDigest("enrollment:" + label),
		authorityKeyID:   domainsecurity.SHA256Hex(authorityPublic),
		authorityPublic:  authorityPublic,
		authorityPrivate: authorityPrivate,
		witnessKeyID:     domainsecurity.SHA256Hex(witnessPublic),
		witnessPublic:    witnessPublic,
		witnessPrivate:   witnessPrivate,
	}
}

func (fixture authorityAdvanceTestFixture) authoritySign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.authorityPrivate, message), nil
}

func (fixture authorityAdvanceTestFixture) witnessSign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.witnessPrivate, message), nil
}

func authorityAdvanceTestDigest(label string) string {
	return domainsecurity.SHA256Hex([]byte(label))
}

func newAuthorityAdvanceRiskIndex(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	generation uint64,
	previousIndexDigest, mutationLabel, policyLabel, previousPolicyDigest string,
) domainsecurity.ThreadRiskAuthorityIndexV1 {
	t.Helper()
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: fixture.installationID,
		EnrollmentID:   fixture.enrollmentID,
		Namespace:      domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:     generation,
		Entries: []domainsecurity.ThreadRiskAuthorityEntryV1{{
			ThreadID: "thread-a", WorkspaceRealPath: "/workspace/a", RiskClass: domainsecurity.RiskClassGeneral,
			CurrentPolicyDigest: authorityAdvanceTestDigest(policyLabel), PreviousPolicyDigest: previousPolicyDigest,
		}},
		PreviousIndexDigest: previousIndexDigest,
		MutationID:          authorityAdvanceTestDigest(mutationLabel),
		AuthorityKeyID:      fixture.authorityKeyID,
		AuthorityPublicKey:  fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func newAuthorityAdvanceCheckpointForRiskIndex(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	index domainsecurity.ThreadRiskAuthorityIndexV1,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:               index.Generation,
		CurrentStateDigest:       index.IndexDigest,
		PreviousStateDigest:      authorityAdvanceTestDigest("prior-state:" + index.MutationID),
		PreviousCheckpointDigest: authorityAdvanceTestDigest("prior-checkpoint:" + index.MutationID),
		FenceNonce:               authorityAdvanceTestDigest("fence:" + index.MutationID),
		MutationID:               index.MutationID,
		WitnessKeyID:             fixture.witnessKeyID,
		WitnessPublicKey:         fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func newAuthorityAdvanceCheckpointForEvidenceBundle(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	bundle domainevidence.EvidenceAuthorityBundleV1,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                domainsecurity.EvidenceRegistryAuthorityNamespaceV1,
		Generation:               bundle.Generation,
		CurrentStateDigest:       bundle.RecordDigest,
		PreviousStateDigest:      authorityAdvanceTestDigest("prior-evidence-state:" + bundle.MutationID),
		PreviousCheckpointDigest: authorityAdvanceTestDigest("prior-evidence-checkpoint:" + bundle.MutationID),
		FenceNonce:               authorityAdvanceTestDigest("evidence-fence:" + bundle.MutationID),
		MutationID:               bundle.MutationID,
		WitnessKeyID:             fixture.witnessKeyID,
		WitnessPublicKey:         fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func newAuthorityAdvanceEnrollmentCheckpoint(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	namespace string,
	label string,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     fixture.installationID,
		EnrollmentID:       fixture.enrollmentID,
		Namespace:          namespace,
		Generation:         0,
		CurrentStateDigest: authorityAdvanceTestDigest("enrollment-state:" + label),
		FenceNonce:         authorityAdvanceTestDigest("enrollment-fence:" + label),
		WitnessKeyID:       fixture.witnessKeyID,
		WitnessPublicKey:   fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func newAuthorityAdvanceRequest(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	nextStateDigest, mutationID string,
) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                previous.Namespace,
		ExpectedGeneration:       previous.Generation,
		ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest:      previous.CurrentStateDigest,
		NextGeneration:           previous.Generation + 1,
		NextStateDigest:          nextStateDigest,
		ExpectedFenceNonce:       previous.FenceNonce,
		MutationID:               mutationID,
		AuthorityKeyID:           fixture.authorityKeyID,
		AuthorityPublicKey:       fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newAuthorityAdvanceReceipt(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	fenceLabel string,
) domainsecurity.MonotonicHeadAdvanceReceiptV1 {
	t.Helper()
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           fixture.installationID,
		EnrollmentID:             fixture.enrollmentID,
		Namespace:                previous.Namespace,
		Generation:               request.NextGeneration,
		CurrentStateDigest:       request.NextStateDigest,
		PreviousStateDigest:      previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce:               authorityAdvanceTestDigest(fenceLabel),
		MutationID:               request.MutationID,
		WitnessKeyID:             fixture.witnessKeyID,
		WitnessPublicKey:         fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func newAuthorityAdvanceRiskIntent(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	previousIndex, nextIndex domainsecurity.ThreadRiskAuthorityIndexV1,
	previousCheckpoint domainsecurity.MonotonicHeadCheckpointV1,
) MonotonicAdvanceIntentV1 {
	t.Helper()
	binding, err := NewThreadRiskTransitionBindingV1(previousIndex, nextIndex)
	if err != nil {
		t.Fatal(err)
	}
	request := newAuthorityAdvanceRequest(
		t, fixture, previousCheckpoint, nextIndex.IndexDigest, nextIndex.MutationID,
	)
	intent, err := NewMonotonicAdvanceIntentV1(MonotonicAdvanceIntentInputV1{
		Root:               AdvanceRootThreadRisk,
		PreviousCheckpoint: previousCheckpoint,
		AdvanceRequest:     request,
		Transition:         binding,
		AuthorityKeyID:     fixture.authorityKeyID,
		AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func newAuthorityAdvanceCommittedStep(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	intent MonotonicAdvanceIntentV1,
	fenceLabel string,
) MonotonicAdvanceCommittedStepV1 {
	t.Helper()
	receipt := newAuthorityAdvanceReceipt(t, fixture, intent.PreviousCheckpoint, intent.AdvanceRequest, fenceLabel)
	settlement, err := NewCommittedMonotonicAdvanceSettlementV1(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return MonotonicAdvanceCommittedStepV1{Intent: intent, Settlement: settlement}
}

func newAuthorityAdvanceEvidenceBundle(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	generation uint64,
	previousDigest, mutationLabel string,
	datasetDigest string,
	datasetCount uint64,
	registryDigest string,
	registryCount uint64,
	publicationDigest string,
	publicationCount uint64,
) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              fixture.installationID,
		EnrollmentID:                fixture.enrollmentID,
		Generation:                  generation,
		PreviousBundleDigest:        previousDigest,
		MutationID:                  authorityAdvanceTestDigest(mutationLabel),
		DatasetSnapshotIndexDigest:  datasetDigest,
		DatasetSnapshotCount:        datasetCount,
		EvidenceRegistryIndexDigest: registryDigest,
		EvidenceRegistryCount:       registryCount,
		PublicationIndexDigest:      publicationDigest,
		PublicationCount:            publicationCount,
		AuthorityKeyID:              fixture.authorityKeyID,
		AuthorityPublicKey:          fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func resignAuthorityAdvanceSettlement(
	t *testing.T,
	fixture authorityAdvanceTestFixture,
	settlement MonotonicAdvanceSettlementV1,
) MonotonicAdvanceSettlementV1 {
	t.Helper()
	settlement.AuthoritySignature = ""
	settlement.RecordDigest = ""
	settlement.AuthoritySignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(fixture.authorityPrivate, MonotonicAdvanceSettlementSigningBytesV1(settlement)),
	)
	settlement.RecordDigest = monotonicAdvanceSettlementDigestV1(settlement)
	return settlement
}

func TestAdvanceRootV1IsClosed(t *testing.T) {
	roots := []AdvanceRootV1{
		AdvanceRootThreadRisk, AdvanceRootDatasetSnapshot, AdvanceRootEvidenceRegistry, AdvanceRootPublication,
	}
	for _, root := range roots {
		if err := ValidateAdvanceRootV1(root); err != nil {
			t.Fatalf("known root %q was rejected: %v", root, err)
		}
	}
	for _, root := range []AdvanceRootV1{"", "active", "evidence_bootstrap", "unknown"} {
		if err := ValidateAdvanceRootV1(root); err == nil {
			t.Fatalf("unknown root %q was accepted", root)
		}
	}
}

func TestAdvanceRootV2IsClosed(t *testing.T) {
	roots := []AdvanceRootV2{
		AdvanceRootThreadRiskV2, AdvanceRootEvidenceGenesisV2, AdvanceRootDatasetSnapshotV2,
		AdvanceRootEvidenceRegistryV2, AdvanceRootPublicationV2,
	}
	for _, root := range roots {
		if err := ValidateAdvanceRootV2(root); err != nil {
			t.Fatalf("expected V2 root %q to be valid: %v", root, err)
		}
	}
	for _, root := range []AdvanceRootV2{"", "active", "evidence_bootstrap", "unknown"} {
		if err := ValidateAdvanceRootV2(root); err == nil {
			t.Fatalf("expected V2 root %q to fail closed", root)
		}
	}
}

func TestGenesisThreadRiskAdvanceIsJournaled(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("thread-risk-genesis")
	enrollment := newAuthorityAdvanceEnrollmentCheckpoint(
		t, fixture, domainsecurity.ThreadRiskAuthorityNamespaceV1, "thread-risk",
	)
	first := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "first-risk-mutation", "first-policy", "")
	binding, err := NewThreadRiskGenesisTransitionBindingV2(enrollment, first)
	if err != nil {
		t.Fatal(err)
	}
	request := newAuthorityAdvanceRequest(t, fixture, enrollment, first.IndexDigest, first.MutationID)
	intent, err := NewMonotonicAdvanceIntentV2(MonotonicAdvanceIntentInputV2{
		Root: AdvanceRootThreadRiskV2, PreviousCheckpoint: enrollment, AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	receipt := newAuthorityAdvanceReceipt(t, fixture, enrollment, request, "first-risk-fence")
	settlement, err := NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	intentBody, err := MonotonicAdvanceIntentV2Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	parsedIntent, err := ParseMonotonicAdvanceIntentV2(intentBody)
	if err != nil || parsedIntent.RecordDigest != intent.RecordDigest {
		t.Fatalf("genesis intent canonical round trip failed: %#v err=%v", parsedIntent, err)
	}
	if _, err := ParseMonotonicAdvanceIntentV1(intentBody); err == nil {
		t.Fatal("frozen V1 parser accepted a V2 genesis intent")
	}
	settlementBody, err := MonotonicAdvanceSettlementV2Bytes(settlement)
	if err != nil {
		t.Fatal(err)
	}
	parsedSettlement, err := ParseMonotonicAdvanceSettlementV2(settlementBody)
	if err != nil || parsedSettlement.RecordDigest != settlement.RecordDigest {
		t.Fatalf("genesis settlement canonical round trip failed: %#v err=%v", parsedSettlement, err)
	}
	if _, err := ParseMonotonicAdvanceSettlementV1(settlementBody); err == nil {
		t.Fatal("frozen V1 parser accepted a V2 genesis settlement")
	}
	step := MonotonicAdvanceCommittedStepV2{Intent: intent, Settlement: settlement}
	if err := ValidateMonotonicAdvanceCommittedRangeV2(enrollment, []MonotonicAdvanceCommittedStepV2{step}, receipt.Checkpoint); err != nil {
		t.Fatal(err)
	}

	tampered := binding
	tampered.ThreadRiskGenesis = &ThreadRiskGenesisBindingV2{
		EnrollmentCheckpoint: enrollment,
		FirstIndex:           first,
	}
	tampered.ThreadRiskGenesis.EnrollmentCheckpoint.CurrentStateDigest = authorityAdvanceTestDigest("wrong-enrollment-state")
	if ValidateAdvanceTransitionBindingV2(AdvanceRootThreadRiskV2, tampered) == nil {
		t.Fatal("thread risk genesis accepted a different enrollment checkpoint")
	}
}

func TestGenesisEvidenceAuthorityAdvanceRequiresCanonicalRoots(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("evidence-genesis")
	enrollment := newAuthorityAdvanceEnrollmentCheckpoint(
		t, fixture, domainsecurity.EvidenceRegistryAuthorityNamespaceV1, "evidence",
	)
	first := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 1, "", "first-evidence-mutation",
		domainsecurity.DatasetSnapshotIndexGenesisDigestV1(), 0,
		domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(), 0,
		domainpublication.PublicationIndexGenesisDigestV1(), 0,
	)
	binding, err := NewEvidenceAuthorityGenesisTransitionBindingV2(enrollment, first)
	if err != nil {
		t.Fatal(err)
	}
	request := newAuthorityAdvanceRequest(t, fixture, enrollment, first.RecordDigest, first.MutationID)
	intent, err := NewMonotonicAdvanceIntentV2(MonotonicAdvanceIntentInputV2{
		Root: AdvanceRootEvidenceGenesisV2, PreviousCheckpoint: enrollment, AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	receipt := newAuthorityAdvanceReceipt(t, fixture, enrollment, request, "first-evidence-fence")
	settlement, err := NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicAdvanceCommittedRangeV2(
		enrollment,
		[]MonotonicAdvanceCommittedStepV2{{Intent: intent, Settlement: settlement}},
		receipt.Checkpoint,
	); err != nil {
		t.Fatal(err)
	}

	wrong := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 1, "", "wrong-evidence-mutation",
		authorityAdvanceTestDigest("caller-selected-dataset-root"), 0,
		domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(), 0,
		domainpublication.PublicationIndexGenesisDigestV1(), 0,
	)
	if _, err := NewEvidenceAuthorityGenesisTransitionBindingV2(enrollment, wrong); err == nil {
		t.Fatal("evidence authority genesis accepted a caller-selected child root")
	}
	if ValidateAdvanceTransitionBindingV2(AdvanceRootDatasetSnapshotV2, binding) == nil {
		t.Fatal("evidence authority genesis was relabeled as a child-root advance")
	}
}

func TestEvidenceTransitionBindingDerivesExactChangedRoot(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("evidence-roots")
	dataset0 := authorityAdvanceTestDigest("dataset-0")
	registry0 := authorityAdvanceTestDigest("registry-0")
	publication0 := authorityAdvanceTestDigest("publication-0")
	previous := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 1, "", "bundle-1", dataset0, 0, registry0, 0, publication0, 0,
	)
	tests := []struct {
		name             string
		root             AdvanceRootV1
		dataset          string
		datasetCount     uint64
		registry         string
		registryCount    uint64
		publication      string
		publicationCount uint64
	}{
		{"dataset", AdvanceRootDatasetSnapshot, authorityAdvanceTestDigest("dataset-1"), 1, registry0, 0, publication0, 0},
		{"registry", AdvanceRootEvidenceRegistry, dataset0, 0, authorityAdvanceTestDigest("registry-1"), 1, publication0, 0},
		{"publication", AdvanceRootPublication, dataset0, 0, registry0, 0, authorityAdvanceTestDigest("publication-1"), 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := newAuthorityAdvanceEvidenceBundle(
				t, fixture, 2, previous.RecordDigest, "bundle-2:"+test.name,
				test.dataset, test.datasetCount, test.registry, test.registryCount,
				test.publication, test.publicationCount,
			)
			root, binding, err := NewEvidenceTransitionBindingV1(previous, next)
			if err != nil {
				t.Fatal(err)
			}
			if root != test.root {
				t.Fatalf("derived root=%q want=%q", root, test.root)
			}
			if err := ValidateAdvanceTransitionBindingV1(root, binding); err != nil {
				t.Fatal(err)
			}
			wrongRoot := AdvanceRootDatasetSnapshot
			if wrongRoot == root {
				wrongRoot = AdvanceRootPublication
			}
			if ValidateAdvanceTransitionBindingV1(wrongRoot, binding) == nil {
				t.Fatal("same bundle transition was accepted under the wrong logical root")
			}
			if root == AdvanceRootDatasetSnapshot {
				checkpoint := newAuthorityAdvanceCheckpointForEvidenceBundle(t, fixture, previous)
				request := newAuthorityAdvanceRequest(t, fixture, checkpoint, next.RecordDigest, next.MutationID)
				intent, err := NewMonotonicAdvanceIntentV1(MonotonicAdvanceIntentInputV1{
					Root: root, PreviousCheckpoint: checkpoint, AdvanceRequest: request, Transition: binding,
					AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
				}, fixture.authoritySign)
				if err != nil {
					t.Fatal(err)
				}
				if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
					t.Fatal(err)
				}
				if _, err := NewMonotonicAdvanceIntentV1(MonotonicAdvanceIntentInputV1{
					Root: wrongRoot, PreviousCheckpoint: checkpoint, AdvanceRequest: request, Transition: binding,
					AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
				}, fixture.authoritySign); err == nil {
					t.Fatal("evidence bundle transition produced an intent under the wrong root")
				}
			}
			tampered := binding
			tampered.NextRootDigest = authorityAdvanceTestDigest("moved-root")
			if ValidateAdvanceTransitionBindingV1(root, tampered) == nil {
				t.Fatal("moved child root summary was accepted")
			}
		})
	}

	batch := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 2, previous.RecordDigest, "bundle-batch",
		authorityAdvanceTestDigest("dataset-batch"), 1,
		authorityAdvanceTestDigest("registry-batch"), 1,
		publication0, 0,
	)
	if _, _, err := NewEvidenceTransitionBindingV1(previous, batch); err == nil {
		t.Fatal("two-child evidence bundle advance was accepted")
	}
}

func TestEvidenceTransitionBindingV2RejectsInvalidSignedBundleLineage(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("evidence-v2-invalid-lineage")
	dataset0 := authorityAdvanceTestDigest("v2-dataset-0")
	registry0 := authorityAdvanceTestDigest("v2-registry-0")
	publication0 := authorityAdvanceTestDigest("v2-publication-0")
	previous := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 1, "", "v2-bundle-1", dataset0, 0, registry0, 0, publication0, 0,
	)
	next := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 2, previous.RecordDigest, "v2-bundle-2",
		authorityAdvanceTestDigest("v2-dataset-1"), 1, registry0, 0, publication0, 0,
	)
	root, binding, err := NewEvidenceTransitionBindingV2(previous, next)
	if err != nil {
		t.Fatal(err)
	}
	tamperedNext := next
	tamperedNext.AuthoritySignature = previous.AuthoritySignature
	malicious := binding
	malicious.EvidenceBundle = &EvidenceBundleTransitionBindingV1{
		PreviousBundle: previous,
		NextBundle:     tamperedNext,
	}
	if ValidateAdvanceTransitionBindingV2(root, malicious) == nil {
		t.Fatal("V2 binding accepted a next bundle with an invalid authority signature")
	}
	checkpoint := newAuthorityAdvanceCheckpointForEvidenceBundle(t, fixture, previous)
	request := newAuthorityAdvanceRequest(t, fixture, checkpoint, tamperedNext.RecordDigest, tamperedNext.MutationID)
	if _, err := NewMonotonicAdvanceIntentV2(MonotonicAdvanceIntentInputV2{
		Root: root, PreviousCheckpoint: checkpoint, AdvanceRequest: request, Transition: malicious,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign); err == nil {
		t.Fatal("authentically signed V2 request produced an intent over invalid bundle lineage")
	}
}

func TestMonotonicAdvanceIntentCanonicalClosedAndExact(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("intent")
	policy1 := authorityAdvanceTestDigest("policy-1")
	previousIndex := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "mutation-1", "policy-1", "")
	nextIndex := newAuthorityAdvanceRiskIndex(t, fixture, 2, previousIndex.IndexDigest, "mutation-2", "policy-2", policy1)
	previousCheckpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previousIndex)
	intent := newAuthorityAdvanceRiskIntent(t, fixture, previousIndex, nextIndex, previousCheckpoint)
	body, err := MonotonicAdvanceIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMonotonicAdvanceIntentV1(body)
	if err != nil || parsed.RecordDigest != intent.RecordDigest {
		t.Fatalf("canonical intent round trip failed: %v", err)
	}
	if err := ValidateMonotonicAdvanceIntentForPreviousCheckpointV1(intent, previousCheckpoint); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicAdvanceIntentExactReplayV1(intent, intent.AdvanceRequest); err != nil {
		t.Fatal(err)
	}

	unknownTop := bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"attacker":true`), 1)
	if _, err := ParseMonotonicAdvanceIntentV1(unknownTop); err == nil {
		t.Fatal("unknown top-level intent property was accepted")
	}
	unknownNested := bytes.Replace(body, []byte(`"transition":{`), []byte(`"transition":{"attacker":true,`), 1)
	if _, err := ParseMonotonicAdvanceIntentV1(unknownNested); err == nil {
		t.Fatal("unknown nested intent property was accepted")
	}
	duplicateNested := bytes.Replace(
		body,
		[]byte(`"transition":{`),
		[]byte(`"transition":{"previousStateDigest":"`+authorityAdvanceTestDigest("duplicate")+`",`),
		1,
	)
	if _, err := ParseMonotonicAdvanceIntentV1(duplicateNested); err == nil {
		t.Fatal("duplicate nested intent property was accepted")
	}
	if _, err := ParseMonotonicAdvanceIntentV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical intent whitespace was accepted")
	}

	tampered := intent
	tampered.MutationID = authorityAdvanceTestDigest("tampered-mutation")
	if ValidateMonotonicAdvanceIntentV1(tampered) == nil {
		t.Fatal("tampered intent mutation was accepted")
	}
	tampered = intent
	tampered.Root = AdvanceRootPublication
	if ValidateMonotonicAdvanceIntentV1(tampered) == nil {
		t.Fatal("tampered intent root was accepted")
	}

	differentRequest := newAuthorityAdvanceRequest(
		t, fixture, previousCheckpoint, authorityAdvanceTestDigest("different-state"), intent.MutationID,
	)
	if ValidateMonotonicAdvanceIntentExactReplayV1(intent, differentRequest) == nil {
		t.Fatal("same mutation ID with different request bytes was accepted")
	}
}

func TestCommittedSettlementRequiresExactRequestAndReceipt(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("committed")
	policy1 := authorityAdvanceTestDigest("policy-1")
	previousIndex := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "mutation-1", "policy-1", "")
	nextIndex := newAuthorityAdvanceRiskIndex(t, fixture, 2, previousIndex.IndexDigest, "mutation-2", "policy-2", policy1)
	previousCheckpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previousIndex)
	intent := newAuthorityAdvanceRiskIntent(t, fixture, previousIndex, nextIndex, previousCheckpoint)
	step := newAuthorityAdvanceCommittedStep(t, fixture, intent, "fence-2")
	body, err := MonotonicAdvanceSettlementV1Bytes(step.Settlement)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMonotonicAdvanceSettlementV1(body)
	if err != nil || parsed.RecordDigest != step.Settlement.RecordDigest {
		t.Fatalf("canonical committed settlement round trip failed: %v", err)
	}
	if err := ValidateCommittedMonotonicAdvanceExactReplayV1(
		intent, step.Settlement, intent.AdvanceRequest, step.Settlement.Committed.Receipt,
	); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(MonotonicAdvanceIntentSigningBytesV1(intent), monotonicAdvanceIntentSignatureDomainV1) ||
		!bytes.HasPrefix(MonotonicAdvanceSettlementSigningBytesV1(step.Settlement), monotonicAdvanceSettlementSignatureDomainV1) ||
		bytes.Equal(monotonicAdvanceIntentSignatureDomainV1, monotonicAdvanceSettlementSignatureDomainV1) {
		t.Fatal("intent and settlement signatures are not domain separated")
	}

	differentReceipt := newAuthorityAdvanceReceipt(t, fixture, previousCheckpoint, intent.AdvanceRequest, "alternate-fence-2")
	if ValidateCommittedMonotonicAdvanceExactReplayV1(
		intent, step.Settlement, intent.AdvanceRequest, differentReceipt,
	) == nil {
		t.Fatal("different valid receipt was accepted as exact replay")
	}

	bothBranches := step.Settlement
	bothBranches.Superseded = &MonotonicAdvanceSupersededV1{}
	bothBranches = resignAuthorityAdvanceSettlement(t, fixture, bothBranches)
	if ValidateMonotonicAdvanceSettlementV1(bothBranches) == nil {
		t.Fatal("authentically signed settlement with both branches was accepted")
	}

	unknownNested := bytes.Replace(body, []byte(`"committed":{`), []byte(`"committed":{"attacker":true,`), 1)
	if _, err := ParseMonotonicAdvanceSettlementV1(unknownNested); err == nil {
		t.Fatal("unknown nested settlement property was accepted")
	}
	duplicateNested := bytes.Replace(body, []byte(`"committed":{`), []byte(`"committed":{"receipt":{},`), 1)
	if _, err := ParseMonotonicAdvanceSettlementV1(duplicateNested); err == nil {
		t.Fatal("duplicate nested settlement property was accepted")
	}
}

func TestSupersededSettlementRequiresFreshObservationAndExactCommittedRange(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("superseded")
	policy1 := authorityAdvanceTestDigest("policy-1")
	previousIndex := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "mutation-1", "policy-1", "")
	previousCheckpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previousIndex)

	winnerIndex1 := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previousIndex.IndexDigest, "winner-mutation-2", "winner-policy-2", policy1,
	)
	winnerIntent1 := newAuthorityAdvanceRiskIntent(t, fixture, previousIndex, winnerIndex1, previousCheckpoint)
	winnerStep1 := newAuthorityAdvanceCommittedStep(t, fixture, winnerIntent1, "winner-fence-2")

	winnerPolicy2 := authorityAdvanceTestDigest("winner-policy-2")
	winnerIndex2 := newAuthorityAdvanceRiskIndex(
		t, fixture, 3, winnerIndex1.IndexDigest, "winner-mutation-3", "winner-policy-3", winnerPolicy2,
	)
	winnerIntent2 := newAuthorityAdvanceRiskIntent(
		t, fixture, winnerIndex1, winnerIndex2, winnerStep1.Settlement.Committed.Receipt.Checkpoint,
	)
	winnerStep2 := newAuthorityAdvanceCommittedStep(t, fixture, winnerIntent2, "winner-fence-3")
	committedRange := []MonotonicAdvanceCommittedStepV1{winnerStep1, winnerStep2}
	tail := winnerStep2.Settlement.Committed.Receipt.Checkpoint
	if err := ValidateMonotonicAdvanceCommittedRangeV1(previousCheckpoint, committedRange, tail); err != nil {
		t.Fatal(err)
	}

	losingIndex := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previousIndex.IndexDigest, "losing-mutation-2", "losing-policy-2", policy1,
	)
	losingIntent := newAuthorityAdvanceRiskIntent(t, fixture, previousIndex, losingIndex, previousCheckpoint)
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     fixture.installationID,
		EnrollmentID:       fixture.enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		ChallengeNonce:     authorityAdvanceTestDigest("fresh-observe-challenge"),
		AuthorityKeyID:     fixture.authorityKeyID,
		AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(observeRequest, tail, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := NewSupersededMonotonicAdvanceSettlementV1(
		losingIntent, observeRequest, observation, committedRange, fixture.authoritySign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicAdvanceSettlementForIntentV1(settlement, losingIntent, committedRange); err != nil {
		t.Fatal(err)
	}
	body, err := MonotonicAdvanceSettlementV1Bytes(settlement)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMonotonicAdvanceSettlementV1(body); err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicAdvanceSettlementForIntentV1(settlement, losingIntent, nil) == nil {
		t.Fatal("superseded settlement was accepted from references without committed range records")
	}

	tamperedReference := settlement
	tamperedReference.Superseded.RangeReferences = append(
		[]MonotonicAdvanceRangeReferenceV1(nil), settlement.Superseded.RangeReferences...,
	)
	tamperedReference.Superseded.RangeReferences[0].MutationID = authorityAdvanceTestDigest("tampered-reference")
	tamperedReference = resignAuthorityAdvanceSettlement(t, fixture, tamperedReference)
	if err := ValidateMonotonicAdvanceSettlementV1(tamperedReference); err != nil {
		t.Fatalf("self-consistent reference record should require live-range validation, got: %v", err)
	}
	if ValidateMonotonicAdvanceSettlementForIntentV1(tamperedReference, losingIntent, committedRange) == nil {
		t.Fatal("tampered signed range reference matched the committed range")
	}

	reordered := []MonotonicAdvanceCommittedStepV1{winnerStep2, winnerStep1}
	if ValidateMonotonicAdvanceCommittedRangeV1(previousCheckpoint, reordered, tail) == nil {
		t.Fatal("reordered committed range was accepted")
	}
	if ValidateMonotonicAdvanceCommittedRangeV1(previousCheckpoint, committedRange[:1], tail) == nil {
		t.Fatal("truncated committed range was accepted against descendant tail")
	}
	if _, err := NewSupersededMonotonicAdvanceSettlementV1(
		winnerIntent1, observeRequest, observation, committedRange, fixture.authoritySign,
	); err == nil {
		t.Fatal("range containing the candidate state produced a superseded settlement")
	}

	staleObservation, err := domainsecurity.NewMonotonicHeadObservationV1(
		observeRequest, winnerStep1.Settlement.Committed.Receipt.Checkpoint, fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSupersededMonotonicAdvanceSettlementV1(
		losingIntent, observeRequest, staleObservation, committedRange, fixture.authoritySign,
	); err == nil {
		t.Fatal("observation that did not match the committed range tail was accepted")
	}
}

func TestMonotonicAdvanceV1WireGolden(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("v1-wire-golden")
	previous := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "v1-golden-previous", "v1-policy-1", "")
	checkpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previous)
	next := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previous.IndexDigest, "v1-golden-next", "v1-policy-2", authorityAdvanceTestDigest("v1-policy-1"),
	)
	intent := newAuthorityAdvanceRiskIntent(t, fixture, previous, next, checkpoint)
	step := newAuthorityAdvanceCommittedStep(t, fixture, intent, "v1-golden-fence")
	intentBody, err := MonotonicAdvanceIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	settlementBody, err := MonotonicAdvanceSettlementV1Bytes(step.Settlement)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{
		domainsecurity.SHA256Hex(intentBody),
		intent.RecordDigest,
		intent.AuthoritySignature,
		domainsecurity.SHA256Hex(settlementBody),
		step.Settlement.RecordDigest,
		step.Settlement.AuthoritySignature,
	}
	want := []string{
		"993bdc18771019a30236f49c4bfcc8d541cd1e367dcce90e76d9fb40c5a12ed6",
		"bf3f1354d954566878c3967a81918ecd9583040b78c7c66ea3667d12120653c9",
		"u_VvGalrmDb9spGI07KgLj60fDBwwPo1w6_A6b4UvnSDTf099q0z0e3mP2IBJeHi5nVBtsjI5cwTfcgBuBVlCg",
		"7fdb212fec1273e34f46b3aea118d1e66aa253cef3430f3711b65078e949b4fb",
		"61187f15cc17d0218a719bac04190cf68586c5105af966f52e9ec29db5290e71",
		"pjDKeNFq1KEfqU6F134azWLWdDkmFlth9g8HnoAcUvb_IHFRBBZa_ASCaTFVxYNKzXgQxz5xmZjnoBzk5uguCQ",
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("V1 wire golden mismatch; replace fixture only through an explicit migration: %#v", got)
		}
	}
}
