package authorityadvance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestMonotonicAdvanceV1GrammarIsFrozen(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(AdvanceTransitionBindingV1{}),
		reflect.TypeOf(ThreadRiskTransitionBindingV1{}),
		reflect.TypeOf(EvidenceBundleTransitionBindingV1{}),
		reflect.TypeOf(MonotonicAdvanceIntentV1{}),
		reflect.TypeOf(MonotonicAdvanceCommittedV1{}),
		reflect.TypeOf(MonotonicAdvanceSupersededV1{}),
		reflect.TypeOf(MonotonicAdvanceSettlementV1{}),
		reflect.TypeOf(MonotonicAdvanceRangeReferenceV1{}),
	}
	fingerprint := authorityAdvanceWireGrammarFingerprint(types, []string{
		string(AdvanceRootThreadRisk), string(AdvanceRootDatasetSnapshot),
		string(AdvanceRootEvidenceRegistry), string(AdvanceRootPublication),
	})
	const expected = "6d2090cd3df971ba7b0ad00b1e3f0526f78a6d9c72c1dc16b45e3a2795c702b3"
	if fingerprint != expected {
		t.Fatalf("V1 grammar changed without an explicit migration: %s", fingerprint)
	}
	if ValidateAdvanceRootV1(AdvanceRootV1("evidence_authority_genesis")) == nil {
		t.Fatal("frozen V1 root enum accepted V2 evidence genesis")
	}

	fixture := newAuthorityAdvanceTestFixture("v1-grammar-closed")
	previous := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "v1-grammar-previous", "policy-1", "")
	checkpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previous)
	next := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previous.IndexDigest, "v1-grammar-next", "policy-2", authorityAdvanceTestDigest("policy-1"),
	)
	intent := newAuthorityAdvanceRiskIntent(t, fixture, previous, next, checkpoint)
	body, err := MonotonicAdvanceIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"threadRiskGenesis", "evidenceGenesis"} {
		injected := bytes.Replace(body, []byte(`"transition":{`), []byte(`"transition":{"`+field+`":null,`), 1)
		if _, err := ParseMonotonicAdvanceIntentV1(injected); err == nil {
			t.Fatalf("frozen V1 parser accepted %s", field)
		}
	}
}

func TestMonotonicAdvanceV1EvidenceAndSupersededWireGolden(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("v1-evidence-wire-golden")
	dataset0 := authorityAdvanceTestDigest("v1-golden-dataset-0")
	registry0 := authorityAdvanceTestDigest("v1-golden-registry-0")
	publication0 := authorityAdvanceTestDigest("v1-golden-publication-0")
	previousBundle := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 1, "", "v1-golden-bundle-1", dataset0, 0, registry0, 0, publication0, 0,
	)
	nextBundle := newAuthorityAdvanceEvidenceBundle(
		t, fixture, 2, previousBundle.RecordDigest, "v1-golden-bundle-2",
		authorityAdvanceTestDigest("v1-golden-dataset-1"), 1, registry0, 0, publication0, 0,
	)
	root, binding, err := NewEvidenceTransitionBindingV1(previousBundle, nextBundle)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := newAuthorityAdvanceCheckpointForEvidenceBundle(t, fixture, previousBundle)
	request := newAuthorityAdvanceRequest(t, fixture, checkpoint, nextBundle.RecordDigest, nextBundle.MutationID)
	intent, err := NewMonotonicAdvanceIntentV1(MonotonicAdvanceIntentInputV1{
		Root: root, PreviousCheckpoint: checkpoint, AdvanceRequest: request, Transition: binding,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
	}, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	receipt := newAuthorityAdvanceReceipt(t, fixture, checkpoint, request, "v1-evidence-golden-fence")
	settlement, err := NewCommittedMonotonicAdvanceSettlementV1(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	intentBody, _ := MonotonicAdvanceIntentV1Bytes(intent)
	settlementBody, _ := MonotonicAdvanceSettlementV1Bytes(settlement)

	previousRisk := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "v1-loser-base", "risk-1", "")
	riskCheckpoint := newAuthorityAdvanceCheckpointForRiskIndex(t, fixture, previousRisk)
	winner := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previousRisk.IndexDigest, "v1-winner", "risk-winner", authorityAdvanceTestDigest("risk-1"),
	)
	winnerIntent := newAuthorityAdvanceRiskIntent(t, fixture, previousRisk, winner, riskCheckpoint)
	winnerStep := newAuthorityAdvanceCommittedStep(t, fixture, winnerIntent, "v1-winner-fence")
	loser := newAuthorityAdvanceRiskIndex(
		t, fixture, 2, previousRisk.IndexDigest, "v1-loser", "risk-loser", authorityAdvanceTestDigest("risk-1"),
	)
	loserIntent := newAuthorityAdvanceRiskIntent(t, fixture, previousRisk, loser, riskCheckpoint)
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
			Namespace:      domainsecurity.ThreadRiskAuthorityNamespaceV1,
			ChallengeNonce: authorityAdvanceTestDigest("v1-superseded-golden-challenge"),
			AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
		},
		fixture.authoritySign,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		observeRequest, winnerStep.Settlement.Committed.Receipt.Checkpoint, fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	superseded, err := NewSupersededMonotonicAdvanceSettlementV1(
		loserIntent, observeRequest, observation, []MonotonicAdvanceCommittedStepV1{winnerStep}, fixture.authoritySign,
	)
	if err != nil {
		t.Fatal(err)
	}
	supersededBody, _ := MonotonicAdvanceSettlementV1Bytes(superseded)
	referenceBody, _ := json.Marshal(superseded.Superseded.RangeReferences[0])
	got := []string{
		domainsecurity.SHA256Hex(intentBody), intent.RecordDigest, intent.AuthoritySignature,
		domainsecurity.SHA256Hex(settlementBody), settlement.RecordDigest, settlement.AuthoritySignature,
		domainsecurity.SHA256Hex(supersededBody), superseded.RecordDigest, superseded.AuthoritySignature,
		domainsecurity.SHA256Hex(referenceBody),
	}
	want := []string{
		"2af115f6dbb5be3010d8d97091767f24df5e7c0e5e61153a381aae6014456c9b",
		"4cf713d5f7c509db783c229b6d91bc93e1c421cd2f4fbaa2ad8e5774d0cb9fbc",
		"DCue2oA_SZedJDYheyITP-v69BGuAl70yAY7tdNq9qEUhXggSiofjJp0D0DF5kpIn7TLxSkw-fNCdxSihenTBw",
		"6fb820ec0b2d8e5265cb71e96d14b8c01a81c71f72e83ced61b829e3cae7a79f",
		"922d5a06bf8807309118c3bbc0fb94c04262330276a0a39e671433bbed70d991",
		"bgF94i6IcT7v3Wfb8HMSEv-7O5BNRGFXjqB1pmD2RRUO-aRzrD47GnOc3jqwnqKZzYzXGmcafF2GcreOZ6DpBA",
		"b509b58e148456517fcfe27d2815dffb07ff58777865dc852702237ff499687e",
		"28a8eb50b3c32a05d4a3915f9cbcdebeab6548cc916b7b9e2f4403748c5a0174",
		"wu-168rnOOsXUzBjUD1kRvBMBmUOnVlerbJufTQ35hsN86XWsYlBQSOIiOB3mZJSBCQZmANmMGEQ6KOYDZ8kBw",
		"01c451be565aa2e80c28a63bf5db807f1a260ce7b74d60ad63825a1aee49523c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V1 evidence/superseded wire golden mismatch: %#v", got)
	}
}

func TestMonotonicAdvanceV2WireGoldenAndDomainSeparation(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("v2-wire-golden")
	enrollment := newAuthorityAdvanceEnrollmentCheckpoint(
		t, fixture, domainsecurity.ThreadRiskAuthorityNamespaceV1, "v2-wire",
	)
	first := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "v2-wire-first", "policy", "")
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
	receipt := newAuthorityAdvanceReceipt(t, fixture, enrollment, request, "v2-wire-fence")
	settlement, err := NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, fixture.authoritySign)
	if err != nil {
		t.Fatal(err)
	}
	intentBody, _ := MonotonicAdvanceIntentV2Bytes(intent)
	settlementBody, _ := MonotonicAdvanceSettlementV2Bytes(settlement)
	got := []string{
		domainsecurity.SHA256Hex(intentBody), intent.RecordDigest, intent.AuthoritySignature,
		domainsecurity.SHA256Hex(settlementBody), settlement.RecordDigest, settlement.AuthoritySignature,
	}
	want := []string{
		"c989f9ee738df510c60563738bea56f15e6a8ba27afa84f15f9554cf0cd5dbeb",
		"588bac01062340a6a913418d368dfed36bc0703bd6db9e9c422059ef3cf0b90d",
		"ZtijCqkb4G0_vWmOcFSK9M-MI2MVVN10SduV2-xAu8PByAlcYYO13QsaB_gAD0FY9Di5zM8SqIx7TWWs7CcZCQ",
		"d3853619df3c5e6b54d04da049d4e535f633846150bd69a09ca7ea0843973ba3",
		"3292d60a0c7d79c0591ea665988309c3039a42c93e47c78851bdd587d8adb995",
		"OxYayKGDwoWLetsLCHvt49UKj4uHxX-Yf2XVKxxG1ABr5F-_xSiPvuvvJbiY7-j_uFQsegjB2uQTfgEveKH6CQ",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V2 wire golden mismatch: %#v", got)
	}
	if bytes.Equal(monotonicAdvanceIntentSignatureDomainV1, monotonicAdvanceIntentSignatureDomainV2) ||
		bytes.Equal(monotonicAdvanceIntentDigestDomainV1, monotonicAdvanceIntentDigestDomainV2) ||
		bytes.Equal(monotonicAdvanceSettlementSignatureDomainV1, monotonicAdvanceSettlementSignatureDomainV2) ||
		bytes.Equal(monotonicAdvanceSettlementDigestDomainV1, monotonicAdvanceSettlementDigestDomainV2) {
		t.Fatal("V1 and V2 signature/digest domains are not separated")
	}
}

func authorityAdvanceWireGrammarFingerprint(types []reflect.Type, roots []string) string {
	var builder strings.Builder
	for _, current := range types {
		builder.WriteString(current.PkgPath())
		builder.WriteByte('.')
		builder.WriteString(current.Name())
		builder.WriteByte('\n')
		for index := 0; index < current.NumField(); index++ {
			field := current.Field(index)
			builder.WriteString(field.Name)
			builder.WriteByte('|')
			builder.WriteString(field.Type.String())
			builder.WriteByte('|')
			builder.WriteString(string(field.Tag))
			builder.WriteByte('\n')
		}
	}
	for _, root := range roots {
		builder.WriteString("root|")
		builder.WriteString(root)
		builder.WriteByte('\n')
	}
	return domainsecurity.SHA256Hex([]byte(builder.String()))
}
