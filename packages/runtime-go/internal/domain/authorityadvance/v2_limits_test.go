package authorityadvance

import (
	"strconv"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestMonotonicAdvanceV2RangeAndJournalBoundsAgree(t *testing.T) {
	fixture := newAuthorityAdvanceTestFixture("v2-size-bound")
	enrollment := newAuthorityAdvanceEnrollmentCheckpoint(
		t, fixture, domainsecurity.ThreadRiskAuthorityNamespaceV1, "size-bound",
	)
	first := newAuthorityAdvanceRiskIndex(t, fixture, 1, "", "size-bound-candidate", "policy", "")
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
	receipt := newAuthorityAdvanceReceipt(t, fixture, enrollment, request, "size-bound-observed")
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
			Namespace:      domainsecurity.ThreadRiskAuthorityNamespaceV1,
			ChallengeNonce: authorityAdvanceTestDigest("v2-size-bound-challenge"),
			AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPublic,
		},
		fixture.authoritySign,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		observeRequest, receipt.Checkpoint, fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	references := make([]MonotonicAdvanceRangeReferenceV2, MaxMonotonicAdvanceRangeStepsV2)
	for index := range references {
		prefix := "v2-size-reference:" + strconv.Itoa(index) + ":"
		references[index] = MonotonicAdvanceRangeReferenceV2{
			IntentSchemaVersion:      MonotonicAdvanceIntentSchemaVersionV2,
			SettlementSchemaVersion:  MonotonicAdvanceSettlementSchemaVersionV2,
			Root:                     AdvanceRootThreadRiskV2,
			MutationID:               authorityAdvanceTestDigest(prefix + "mutation"),
			IntentDigest:             authorityAdvanceTestDigest(prefix + "intent"),
			RequestDigest:            authorityAdvanceTestDigest(prefix + "request"),
			ReceiptDigest:            authorityAdvanceTestDigest(prefix + "receipt"),
			SettlementDigest:         authorityAdvanceTestDigest(prefix + "settlement"),
			PreviousCheckpointDigest: authorityAdvanceTestDigest(prefix + "previous"),
			NextCheckpointDigest:     authorityAdvanceTestDigest(prefix + "next"),
		}
	}
	settlement := newMonotonicAdvanceSettlementV2(intent, MonotonicAdvanceSettlementSupersededV2)
	settlement.Superseded = &MonotonicAdvanceSupersededV2{
		ObserveRequest: observeRequest, Observation: observation, RangeReferences: references,
	}
	if err := signMonotonicAdvanceSettlementV2(&settlement, fixture.authoritySign); err != nil {
		t.Fatalf("maximum V2 range must remain inside the shared journal bound: %v", err)
	}
	body, err := MonotonicAdvanceSettlementV2Bytes(settlement)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > MaxMonotonicAdvanceJournalRecordBytesV2 {
		t.Fatalf("domain-valid V2 settlement is not persistable: %d > %d", len(body), MaxMonotonicAdvanceJournalRecordBytesV2)
	}

	tooMany := settlement
	tooMany.AuthoritySignature = ""
	tooMany.RecordDigest = ""
	tooMany.Superseded = &MonotonicAdvanceSupersededV2{
		ObserveRequest: observeRequest,
		Observation:    observation,
		RangeReferences: append(
			append([]MonotonicAdvanceRangeReferenceV2(nil), references...), references[0],
		),
	}
	if err := signMonotonicAdvanceSettlementV2(&tooMany, fixture.authoritySign); err == nil {
		t.Fatal("V2 settlement accepted a range above the shared deterministic step bound")
	}
}
