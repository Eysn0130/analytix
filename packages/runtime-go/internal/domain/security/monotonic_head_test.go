package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
)

type monotonicHeadTestFixture struct {
	authority      threadRiskAuthorityTestKeys
	witnessPrivate ed25519.PrivateKey
	witnessPublic  ed25519.PublicKey
	witnessKeyID   string
}

func newMonotonicHeadTestFixture(t *testing.T) monotonicHeadTestFixture {
	t.Helper()
	witnessPublic, witnessPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return monotonicHeadTestFixture{
		authority: newThreadRiskAuthorityTestKeys(t), witnessPrivate: witnessPrivate,
		witnessPublic: witnessPublic, witnessKeyID: SHA256Hex(witnessPublic),
	}
}

func (fixture monotonicHeadTestFixture) witnessSign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.witnessPrivate, message), nil
}

func newTestMonotonicCheckpoint(
	t *testing.T,
	fixture monotonicHeadTestFixture,
	generation uint64,
	currentState, previousState, previousCheckpoint, fenceNonce, mutationID string,
) MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := NewMonotonicHeadCheckpointV1(MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: ThreadRiskAuthorityNamespaceV1, Generation: generation,
		CurrentStateDigest: currentState, PreviousStateDigest: previousState,
		PreviousCheckpointDigest: previousCheckpoint, FenceNonce: fenceNonce, MutationID: mutationID,
		WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func newTestMonotonicRequest(t *testing.T, fixture monotonicHeadTestFixture, previous MonotonicHeadCheckpointV1, nextState, mutationID string) MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := NewMonotonicHeadAdvanceRequestV1(MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace:          ThreadRiskAuthorityNamespaceV1,
		ExpectedGeneration: previous.Generation, ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest: previous.CurrentStateDigest, NextGeneration: previous.Generation + 1,
		NextStateDigest: nextState, ExpectedFenceNonce: previous.FenceNonce, MutationID: mutationID,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, fixture.authority.sign)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newTestMonotonicObserveRequest(t *testing.T, fixture monotonicHeadTestFixture, namespace, challenge string) MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := NewMonotonicHeadObserveRequestV1(MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: namespace, ChallengeNonce: challenge,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, fixture.authority.sign)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newTestMonotonicReceipt(
	t *testing.T,
	fixture monotonicHeadTestFixture,
	previous MonotonicHeadCheckpointV1,
	request MonotonicHeadAdvanceRequestV1,
	nextFence string,
) MonotonicHeadAdvanceReceiptV1 {
	t.Helper()
	next := newTestMonotonicCheckpoint(
		t, fixture, request.NextGeneration, request.NextStateDigest, previous.CurrentStateDigest,
		previous.CheckpointDigest, nextFence, request.MutationID,
	)
	receipt, err := NewMonotonicHeadAdvanceReceiptV1(request, next, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestMonotonicHeadObservationBindsFreshChallengeWithoutMutatingStableHead(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	checkpoint := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("enrollment-state")), "", "", SHA256Hex([]byte("fence-0")), "",
	)
	challengeA := SHA256Hex([]byte("client-challenge-a"))
	requestA := newTestMonotonicObserveRequest(t, fixture, ThreadRiskAuthorityNamespaceV1, challengeA)
	tamperedRequest := requestA
	tamperedRequest.ChallengeNonce = SHA256Hex([]byte("tampered-challenge"))
	if ValidateMonotonicHeadObserveRequestV1(tamperedRequest) == nil {
		t.Fatal("tampered signed observe request was accepted")
	}
	observationA, err := NewMonotonicHeadObservationV1(requestA, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicHeadObservationForRequestV1(
		observationA, requestA, fixture.authority.installationID, fixture.authority.keyID, fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID, fixture.witnessPublic,
	); err != nil {
		t.Fatal(err)
	}
	challengeB := SHA256Hex([]byte("client-challenge-b"))
	requestB := newTestMonotonicObserveRequest(t, fixture, ThreadRiskAuthorityNamespaceV1, challengeB)
	if ValidateMonotonicHeadObservationForRequestV1(
		observationA, requestB, fixture.authority.installationID, fixture.authority.keyID, fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID, fixture.witnessPublic,
	) == nil {
		t.Fatal("old signed observation replay passed a fresh challenge")
	}
	observationB, err := NewMonotonicHeadObservationV1(requestB, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if observationA.ObservationDigest == observationB.ObservationDigest {
		t.Fatal("distinct observation challenges produced the same observation")
	}
	if observationA.Checkpoint.CheckpointDigest != observationB.Checkpoint.CheckpointDigest ||
		observationA.Checkpoint.Generation != observationB.Checkpoint.Generation ||
		observationA.Checkpoint.FenceNonce != observationB.Checkpoint.FenceNonce {
		t.Fatal("observe mutated the stable checkpoint")
	}

	attacker := newMonotonicHeadTestFixture(t)
	attackerCheckpoint := newTestMonotonicCheckpoint(
		t, attacker, 0, checkpoint.CurrentStateDigest, "", "", SHA256Hex([]byte("attacker-fence")), "",
	)
	attackerRequest := newTestMonotonicObserveRequest(t, attacker, ThreadRiskAuthorityNamespaceV1, challengeA)
	attackerObservation, err := NewMonotonicHeadObservationV1(attackerRequest, attackerCheckpoint, attacker.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadObservationV1(attackerObservation) != nil {
		t.Fatal("attacker observation should be authentic under its own witness key")
	}
	if ValidateMonotonicHeadObservationForRequestV1(
		attackerObservation, attackerRequest, fixture.authority.installationID, fixture.authority.keyID, fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID, fixture.witnessPublic,
	) == nil {
		t.Fatal("self-signed attacker observation passed enrolled witness validation")
	}
}

func TestMonotonicHeadProtocolAllowsOnlyVersionedAuthorityNamespaces(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	checkpoint, err := NewMonotonicHeadCheckpointV1(MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: EvidenceRegistryAuthorityNamespaceV1, Generation: 0,
		CurrentStateDigest: SHA256Hex([]byte("evidence-state-0")), FenceNonce: SHA256Hex([]byte("evidence-fence-0")),
		WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	observeRequest := newTestMonotonicObserveRequest(
		t, fixture, EvidenceRegistryAuthorityNamespaceV1, SHA256Hex([]byte("evidence-challenge")),
	)
	observation, err := NewMonotonicHeadObservationV1(observeRequest, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicHeadObservationForRequestV1(
		observation, observeRequest, fixture.authority.installationID, fixture.authority.keyID, fixture.authority.publicKey,
		fixture.authority.enrollmentID, fixture.witnessKeyID, fixture.witnessPublic,
	); err != nil {
		t.Fatal(err)
	}
	advanceRequest, err := NewMonotonicHeadAdvanceRequestV1(MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: EvidenceRegistryAuthorityNamespaceV1, ExpectedGeneration: 0,
		ExpectedCheckpointDigest: checkpoint.CheckpointDigest, ExpectedStateDigest: checkpoint.CurrentStateDigest,
		NextGeneration: 1, NextStateDigest: SHA256Hex([]byte("evidence-state-1")),
		ExpectedFenceNonce: checkpoint.FenceNonce, MutationID: SHA256Hex([]byte("evidence-mutation-1")),
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, fixture.authority.sign)
	if err != nil {
		t.Fatal(err)
	}
	nextCheckpoint, err := NewMonotonicHeadCheckpointV1(MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: EvidenceRegistryAuthorityNamespaceV1, Generation: 1,
		CurrentStateDigest: advanceRequest.NextStateDigest, PreviousStateDigest: checkpoint.CurrentStateDigest,
		PreviousCheckpointDigest: checkpoint.CheckpointDigest, FenceNonce: SHA256Hex([]byte("evidence-fence-1")),
		MutationID: advanceRequest.MutationID, WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPublic,
	}, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := NewMonotonicHeadAdvanceReceiptV1(advanceRequest, nextCheckpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicHeadAdvanceV1(checkpoint, advanceRequest, receipt); err != nil {
		t.Fatal(err)
	}

	unknownNamespace := "analytix.unregistered-authority/v1"
	badCheckpoint := MonotonicHeadCheckpointInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: unknownNamespace, CurrentStateDigest: SHA256Hex([]byte("state")), FenceNonce: SHA256Hex([]byte("fence")),
		WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPublic,
	}
	if _, err := NewMonotonicHeadCheckpointV1(badCheckpoint, fixture.witnessSign); err == nil {
		t.Fatal("unregistered checkpoint namespace was accepted")
	}
	if _, err := NewMonotonicHeadObserveRequestV1(MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: unknownNamespace, ChallengeNonce: SHA256Hex([]byte("challenge")),
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, fixture.authority.sign); err == nil {
		t.Fatal("unregistered observe namespace was accepted")
	}
}

func TestThreadRiskAuthorityFirstAndSubsequentWitnessAdvance(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	entryV1 := testThreadRiskAuthorityEntry("thread-a", "/workspace/a", RiskClassGeneral, SHA256Hex([]byte("policy-1")), "")
	firstMutation := SHA256Hex([]byte("mutation-1"))
	firstIndex := newTestThreadRiskAuthorityIndex(t, fixture.authority, 1, "", firstMutation, []ThreadRiskAuthorityEntryV1{entryV1})
	enrollment := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("enrollment-state")), "", "", SHA256Hex([]byte("fence-0")), "",
	)
	firstRequest := newTestMonotonicRequest(t, fixture, enrollment, firstIndex.IndexDigest, firstMutation)
	firstReceipt := newTestMonotonicReceipt(t, fixture, enrollment, firstRequest, SHA256Hex([]byte("fence-1")))
	if err := ValidateThreadRiskAuthorityFirstWitnessAdvanceV1(firstIndex, enrollment, firstRequest, firstReceipt); err != nil {
		t.Fatal(err)
	}
	attackerAuthority := newMonotonicHeadTestFixture(t)
	attackerAuthority.authority.installationID = fixture.authority.installationID
	attackerAuthority.authority.enrollmentID = fixture.authority.enrollmentID
	attackerRequest := newTestMonotonicRequest(t, attackerAuthority, enrollment, firstIndex.IndexDigest, firstMutation)
	attackerReceipt := newTestMonotonicReceipt(t, fixture, enrollment, attackerRequest, SHA256Hex([]byte("attacker-request-fence")))
	if ValidateThreadRiskAuthorityFirstWitnessAdvanceV1(firstIndex, enrollment, attackerRequest, attackerReceipt) == nil {
		t.Fatal("index advance signed by a different installation key was accepted")
	}
	if err := ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		enrollment, firstRequest, firstReceipt,
		fixture.authority.installationID, fixture.authority.keyID, fixture.authority.publicKey,
		fixture.authority.enrollmentID, fixture.witnessKeyID, fixture.witnessPublic,
	); err != nil {
		t.Fatal(err)
	}

	entryV2 := testThreadRiskAuthorityEntry(
		"thread-a", "/workspace/a", RiskClassCase, SHA256Hex([]byte("policy-2")), entryV1.CurrentPolicyDigest,
	)
	secondMutation := SHA256Hex([]byte("mutation-2"))
	secondIndex := newTestThreadRiskAuthorityIndex(
		t, fixture.authority, 2, firstIndex.IndexDigest, secondMutation, []ThreadRiskAuthorityEntryV1{entryV2},
	)
	secondRequest := newTestMonotonicRequest(t, fixture, firstReceipt.Checkpoint, secondIndex.IndexDigest, secondMutation)
	secondReceipt := newTestMonotonicReceipt(t, fixture, firstReceipt.Checkpoint, secondRequest, SHA256Hex([]byte("fence-2")))
	if err := ValidateThreadRiskAuthorityWitnessAdvanceV1(
		firstIndex, secondIndex, firstReceipt.Checkpoint, secondRequest, secondReceipt,
	); err != nil {
		t.Fatal(err)
	}
	wrongMutationCheckpoint := newTestMonotonicCheckpoint(
		t, fixture, secondIndex.Generation, secondIndex.IndexDigest, firstIndex.IndexDigest,
		firstReceipt.Checkpoint.CheckpointDigest, SHA256Hex([]byte("wrong-mutation-fence")), SHA256Hex([]byte("wrong-mutation")),
	)
	if ValidateThreadRiskAuthorityIndexCheckpointV1(secondIndex, wrongMutationCheckpoint) == nil {
		t.Fatal("thread risk index accepted a checkpoint for a different mutation")
	}
}

func TestMonotonicHeadAdvanceRejectsStaleSkippedAndRotatedHeads(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("state-0")), "", "", SHA256Hex([]byte("fence-0")), "",
	)
	request := newTestMonotonicRequest(t, fixture, previous, SHA256Hex([]byte("state-1")), SHA256Hex([]byte("mutation-1")))
	receipt := newTestMonotonicReceipt(t, fixture, previous, request, SHA256Hex([]byte("fence-1")))
	if err := ValidateMonotonicHeadAdvanceV1(previous, request, receipt); err != nil {
		t.Fatal(err)
	}
	reusedMutationRequest := newTestMonotonicRequest(
		t, fixture, receipt.Checkpoint, SHA256Hex([]byte("state-2")), request.MutationID,
	)
	reusedMutationReceipt := newTestMonotonicReceipt(
		t, fixture, receipt.Checkpoint, reusedMutationRequest, SHA256Hex([]byte("fence-2")),
	)
	if ValidateMonotonicHeadAdvanceV1(receipt.Checkpoint, reusedMutationRequest, reusedMutationReceipt) == nil {
		t.Fatal("current head mutation ID was reused for a new advance")
	}

	staleRequestInput := MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.authority.installationID, EnrollmentID: fixture.authority.enrollmentID,
		Namespace: ThreadRiskAuthorityNamespaceV1, ExpectedGeneration: previous.Generation,
		ExpectedCheckpointDigest: SHA256Hex([]byte("old-checkpoint")), ExpectedStateDigest: previous.CurrentStateDigest,
		NextGeneration: 1, NextStateDigest: request.NextStateDigest, ExpectedFenceNonce: previous.FenceNonce,
		MutationID: SHA256Hex([]byte("stale-mutation")), AuthorityKeyID: fixture.authority.keyID,
		AuthorityPublicKey: fixture.authority.publicKey,
	}
	staleRequest, err := NewMonotonicHeadAdvanceRequestV1(staleRequestInput, fixture.authority.sign)
	if err != nil {
		t.Fatal(err)
	}
	staleReceipt := newTestMonotonicReceipt(t, fixture, previous, staleRequest, SHA256Hex([]byte("stale-fence")))
	if ValidateMonotonicHeadAdvanceV1(previous, staleRequest, staleReceipt) == nil {
		t.Fatal("stale expected checkpoint was accepted")
	}

	wrongFenceInput := staleRequestInput
	wrongFenceInput.ExpectedCheckpointDigest = previous.CheckpointDigest
	wrongFenceInput.ExpectedFenceNonce = SHA256Hex([]byte("wrong-fence"))
	wrongFenceInput.MutationID = SHA256Hex([]byte("wrong-fence-mutation"))
	wrongFenceRequest, err := NewMonotonicHeadAdvanceRequestV1(wrongFenceInput, fixture.authority.sign)
	if err != nil {
		t.Fatal(err)
	}
	wrongFenceReceipt := newTestMonotonicReceipt(t, fixture, previous, wrongFenceRequest, SHA256Hex([]byte("new-fence")))
	if ValidateMonotonicHeadAdvanceV1(previous, wrongFenceRequest, wrongFenceReceipt) == nil {
		t.Fatal("wrong CAS fence was accepted")
	}

	skippedCheckpoint := newTestMonotonicCheckpoint(
		t, fixture, request.NextGeneration, request.NextStateDigest, previous.CurrentStateDigest,
		SHA256Hex([]byte("unrelated-checkpoint")), SHA256Hex([]byte("fence-skip")), request.MutationID,
	)
	skippedReceipt, err := NewMonotonicHeadAdvanceReceiptV1(request, skippedCheckpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadAdvanceV1(previous, request, skippedReceipt) == nil {
		t.Fatal("checkpoint that skipped the old head was accepted")
	}

	rotated := newMonotonicHeadTestFixture(t)
	rotated.authority = fixture.authority
	rotatedCheckpoint := newTestMonotonicCheckpoint(
		t, rotated, request.NextGeneration, request.NextStateDigest, previous.CurrentStateDigest,
		previous.CheckpointDigest, SHA256Hex([]byte("rotated-fence")), request.MutationID,
	)
	rotatedReceipt, err := NewMonotonicHeadAdvanceReceiptV1(request, rotatedCheckpoint, rotated.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadAdvanceV1(previous, request, rotatedReceipt) == nil {
		t.Fatal("silent witness key rotation was accepted")
	}

	overflowInput := staleRequestInput
	overflowInput.ExpectedGeneration = ^uint64(0)
	overflowInput.NextGeneration = 0
	if _, err := NewMonotonicHeadAdvanceRequestV1(overflowInput, fixture.authority.sign); err == nil {
		t.Fatal("generation overflow request was accepted")
	}
}

func TestMonotonicHeadCheckpointDirectSuccessorRejectsMutationReuse(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	firstMutation := SHA256Hex([]byte("direct-successor-mutation-1"))
	previous := newTestMonotonicCheckpoint(
		t, fixture, 1, SHA256Hex([]byte("direct-successor-state-1")), SHA256Hex([]byte("direct-successor-state-0")),
		SHA256Hex([]byte("direct-successor-checkpoint-0")), SHA256Hex([]byte("direct-successor-fence-1")), firstMutation,
	)
	reused := newTestMonotonicCheckpoint(
		t, fixture, 2, SHA256Hex([]byte("direct-successor-state-2")), previous.CurrentStateDigest,
		previous.CheckpointDigest, SHA256Hex([]byte("direct-successor-fence-2")), firstMutation,
	)
	if ValidateMonotonicHeadCheckpointDirectSuccessorV1(previous, reused) == nil {
		t.Fatal("direct checkpoint successor reused the previous mutation ID")
	}
	valid := newTestMonotonicCheckpoint(
		t, fixture, 2, reused.CurrentStateDigest, previous.CurrentStateDigest,
		previous.CheckpointDigest, reused.FenceNonce, SHA256Hex([]byte("direct-successor-mutation-2")),
	)
	if err := ValidateMonotonicHeadCheckpointDirectSuccessorV1(previous, valid); err != nil {
		t.Fatalf("valid direct checkpoint successor was rejected: %v", err)
	}
}

func TestMonotonicHeadIdempotencyIsExactByMutationRequestAndReceipt(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("state-0")), "", "", SHA256Hex([]byte("fence-0")), "",
	)
	mutation := SHA256Hex([]byte("mutation-1"))
	request := newTestMonotonicRequest(t, fixture, previous, SHA256Hex([]byte("state-1")), mutation)
	receipt := newTestMonotonicReceipt(t, fixture, previous, request, SHA256Hex([]byte("fence-1")))
	if err := ValidateMonotonicHeadIdempotentReplayV1(request, receipt, request, receipt); err != nil {
		t.Fatal(err)
	}

	collision := newTestMonotonicRequest(t, fixture, previous, SHA256Hex([]byte("different-state")), mutation)
	if ValidateMonotonicHeadIdempotentReplayV1(request, receipt, collision, receipt) == nil {
		t.Fatal("same mutation ID with different request was accepted")
	}
	alternateReceipt := newTestMonotonicReceipt(t, fixture, previous, request, SHA256Hex([]byte("alternate-fence")))
	if ValidateMonotonicHeadAdvanceV1(previous, request, alternateReceipt) != nil {
		t.Fatal("alternate receipt fixture must remain a valid CAS result")
	}
	if ValidateMonotonicHeadIdempotentReplayV1(request, receipt, request, alternateReceipt) == nil {
		t.Fatal("same request replay returned a different valid receipt")
	}
}

func TestMonotonicHeadContractsRejectAmbiguousOrOpenJSON(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	checkpoint := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("state-0")), "", "", SHA256Hex([]byte("fence-0")), "",
	)
	observeRequest := newTestMonotonicObserveRequest(t, fixture, ThreadRiskAuthorityNamespaceV1, SHA256Hex([]byte("challenge")))
	observation, err := NewMonotonicHeadObservationV1(observeRequest, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	request := newTestMonotonicRequest(t, fixture, checkpoint, SHA256Hex([]byte("state-1")), SHA256Hex([]byte("mutation")))
	receipt := newTestMonotonicReceipt(t, fixture, checkpoint, request, SHA256Hex([]byte("fence-1")))

	tests := []struct {
		name  string
		value any
		parse func([]byte) error
	}{
		{"checkpoint", checkpoint, func(body []byte) error { _, err := ParseMonotonicHeadCheckpointV1(body); return err }},
		{"observe request", observeRequest, func(body []byte) error { _, err := ParseMonotonicHeadObserveRequestV1(body); return err }},
		{"observation", observation, func(body []byte) error { _, err := ParseMonotonicHeadObservationV1(body); return err }},
		{"request", request, func(body []byte) error { _, err := ParseMonotonicHeadAdvanceRequestV1(body); return err }},
		{"receipt", receipt, func(body []byte) error { _, err := ParseMonotonicHeadAdvanceReceiptV1(body); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]any
			if err := json.Unmarshal(body, &object); err != nil {
				t.Fatal(err)
			}
			object["unknown"] = true
			unknown, _ := json.Marshal(object)
			if test.parse(unknown) == nil {
				t.Fatal("unknown property was accepted")
			}
			duplicate := bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":2`), 1)
			if test.parse(duplicate) == nil {
				t.Fatal("duplicate property was accepted")
			}
			wrongCase := bytes.Replace(body, []byte(`"schemaVersion"`), []byte(`"SchemaVersion"`), 1)
			if test.parse(wrongCase) == nil {
				t.Fatal("non-canonical property casing was accepted")
			}
			if test.parse(append(body, []byte(` {}`)...)) == nil {
				t.Fatal("trailing JSON was accepted")
			}
		})
	}
}
