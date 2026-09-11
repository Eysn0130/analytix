package security

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestMonotonicHeadMutationResolutionRecoversOnlyExactCommittedReceipt(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("recovery-state-0")), "", "", SHA256Hex([]byte("recovery-fence-0")), "",
	)
	advance := newTestMonotonicRequest(
		t, fixture, previous, SHA256Hex([]byte("recovery-state-1")), SHA256Hex([]byte("recovery-mutation-1")),
	)
	receipt := newTestMonotonicReceipt(t, fixture, previous, advance, SHA256Hex([]byte("recovery-fence-1")))
	request, err := NewMonotonicHeadMutationResolveRequestV1(
		advance,
		SHA256Hex([]byte("recovery-challenge-1")),
		fixture.authority.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := NewMonotonicHeadMutationResolutionV1(
		request,
		receipt.Checkpoint,
		&MonotonicHeadCommittedMutationV1{AdvanceRequest: advance, AdvanceReceipt: receipt},
		fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMonotonicHeadMutationResolutionForRequestV1(
		resolution,
		request,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	); err != nil {
		t.Fatal(err)
	}
	requestBody, err := MonotonicHeadMutationResolveRequestV1Bytes(request)
	if err != nil {
		t.Fatal(err)
	}
	parsedRequest, err := ParseMonotonicHeadMutationResolveRequestV1(requestBody)
	if err != nil || parsedRequest != request {
		t.Fatalf("canonical resolve request round trip failed: parsed=%#v err=%v", parsedRequest, err)
	}
	resolutionBody, err := MonotonicHeadMutationResolutionV1Bytes(resolution)
	if err != nil {
		t.Fatal(err)
	}
	parsedResolution, err := ParseMonotonicHeadMutationResolutionV1(resolutionBody)
	if err != nil || parsedResolution.Committed == nil || *parsedResolution.Committed != *resolution.Committed ||
		parsedResolution.ResolutionDigest != resolution.ResolutionDigest {
		t.Fatalf("canonical mutation resolution round trip failed: parsed=%#v err=%v", parsedResolution, err)
	}

	replayRequest, err := NewMonotonicHeadMutationResolveRequestV1(
		advance,
		SHA256Hex([]byte("recovery-challenge-2")),
		fixture.authority.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadMutationResolutionForRequestV1(
		resolution,
		replayRequest,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	) == nil {
		t.Fatal("old committed resolution passed a fresh recovery challenge")
	}

	otherAdvance := newTestMonotonicRequest(
		t, fixture, previous, SHA256Hex([]byte("other-state-1")), SHA256Hex([]byte("other-mutation-1")),
	)
	otherReceipt := newTestMonotonicReceipt(t, fixture, previous, otherAdvance, SHA256Hex([]byte("other-fence-1")))
	mismatched, err := NewMonotonicHeadMutationResolutionV1(
		request,
		otherReceipt.Checkpoint,
		&MonotonicHeadCommittedMutationV1{AdvanceRequest: otherAdvance, AdvanceReceipt: otherReceipt},
		fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadMutationResolutionV1(mismatched) != nil {
		t.Fatal("mismatched response should remain authentic under the witness for binding-test coverage")
	}
	if ValidateMonotonicHeadMutationResolutionForRequestV1(
		mismatched,
		request,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	) == nil {
		t.Fatal("authentic response for another mutation recovered the local intent")
	}
}

func TestMonotonicHeadAbsentMutationResolutionNeverCarriesCommittedData(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("absent-state-0")), "", "", SHA256Hex([]byte("absent-fence-0")), "",
	)
	advance := newTestMonotonicRequest(
		t, fixture, previous, SHA256Hex([]byte("absent-state-1")), SHA256Hex([]byte("absent-mutation-1")),
	)
	request, err := NewMonotonicHeadMutationResolveRequestV1(
		advance,
		SHA256Hex([]byte("absent-challenge")),
		fixture.authority.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := NewMonotonicHeadMutationResolutionV1(request, previous, nil, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != MonotonicHeadMutationResolutionAbsentV1 || resolution.Committed != nil {
		t.Fatalf("absent resolution is not a closed union: %#v", resolution)
	}
	if err := ValidateMonotonicHeadMutationResolutionForRequestV1(
		resolution,
		request,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	); err != nil {
		t.Fatal(err)
	}
	body, err := MonotonicHeadMutationResolutionV1Bytes(resolution)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object["committed"] != nil {
		t.Fatalf("absent canonical wire unexpectedly exposed committed: %s", body)
	}

	invalid := resolution
	invalid.Committed = &MonotonicHeadCommittedMutationV1{}
	resignMutationResolution(t, fixture, &invalid)
	if ValidateMonotonicHeadMutationResolutionV1(invalid) == nil {
		t.Fatal("signed absent resolution carrying committed data was accepted")
	}
	unknown := resolution
	unknown.Status = "unknown"
	resignMutationResolution(t, fixture, &unknown)
	if ValidateMonotonicHeadMutationResolutionV1(unknown) == nil {
		t.Fatal("signed unknown mutation resolution status was accepted")
	}
}

func TestMonotonicHeadRecoveryParserRejectsAmbiguousOrNonCanonicalJSON(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("parse-state-0")), "", "", SHA256Hex([]byte("parse-fence-0")), "",
	)
	advance := newTestMonotonicRequest(
		t, fixture, previous, SHA256Hex([]byte("parse-state-1")), SHA256Hex([]byte("parse-mutation-1")),
	)
	request, err := NewMonotonicHeadMutationResolveRequestV1(
		advance,
		SHA256Hex([]byte("parse-challenge")),
		fixture.authority.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := MonotonicHeadMutationResolveRequestV1Bytes(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMonotonicHeadMutationResolveRequestV1(append(append([]byte(nil), body...), '\n')); err == nil {
		t.Fatal("non-canonical recovery request whitespace was accepted")
	}
	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := ParseMonotonicHeadMutationResolveRequestV1(unknown); err == nil {
		t.Fatal("unknown recovery request property was accepted")
	}
	duplicate := append([]byte(nil), body[:len(body)-1]...)
	duplicate = append(duplicate, []byte(`,"purpose":"analytix.monotonic-head-mutation-resolve-request/v1"}`)...)
	if _, err := ParseMonotonicHeadMutationResolveRequestV1(duplicate); err == nil {
		t.Fatal("duplicate recovery request property was accepted")
	}

	tampered := request
	tampered.ChallengeNonce = SHA256Hex([]byte("tampered-recovery-challenge"))
	if ValidateMonotonicHeadMutationResolveRequestV1(tampered) == nil {
		t.Fatal("tampered recovery challenge retained request authority")
	}
	attacker := newMonotonicHeadTestFixture(t)
	if ValidateMonotonicHeadMutationResolveRequestForInstallationV1(
		request,
		attacker.authority.installationID,
		attacker.authority.keyID,
		attacker.authority.publicKey,
	) == nil {
		t.Fatal("recovery request passed a different installation authority")
	}
}

func resignMutationResolution(t *testing.T, fixture monotonicHeadTestFixture, resolution *MonotonicHeadMutationResolutionV1) {
	t.Helper()
	resolution.WitnessSignature = ""
	resolution.ResolutionDigest = ""
	signature := ed25519.Sign(fixture.witnessPrivate, MonotonicHeadMutationResolutionSigningBytesV1(*resolution))
	resolution.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	resolution.ResolutionDigest = monotonicHeadMutationResolutionDigestV1(*resolution)
}

func TestMonotonicHeadMutationResolutionRejectsLowerOrEquivocatingCurrentHead(t *testing.T) {
	fixture := newMonotonicHeadTestFixture(t)
	previous := newTestMonotonicCheckpoint(
		t, fixture, 0, SHA256Hex([]byte("head-state-0")), "", "", SHA256Hex([]byte("head-fence-0")), "",
	)
	advance := newTestMonotonicRequest(
		t, fixture, previous, SHA256Hex([]byte("head-state-1")), SHA256Hex([]byte("head-mutation-1")),
	)
	receipt := newTestMonotonicReceipt(t, fixture, previous, advance, SHA256Hex([]byte("head-fence-1")))
	request, err := NewMonotonicHeadMutationResolveRequestV1(
		advance,
		SHA256Hex([]byte("head-challenge")),
		fixture.authority.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	lower, err := NewMonotonicHeadMutationResolutionV1(
		request,
		previous,
		&MonotonicHeadCommittedMutationV1{AdvanceRequest: advance, AdvanceReceipt: receipt},
		fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadMutationResolutionForRequestV1(
		lower,
		request,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	) == nil {
		t.Fatal("committed receipt was accepted while the signed current head was lower")
	}

	equivocatingCheckpoint := newTestMonotonicCheckpoint(
		t,
		fixture,
		receipt.Checkpoint.Generation,
		SHA256Hex([]byte("equivocating-state")),
		previous.CurrentStateDigest,
		previous.CheckpointDigest,
		SHA256Hex([]byte("equivocating-fence")),
		SHA256Hex([]byte("equivocating-mutation")),
	)
	equivocating, err := NewMonotonicHeadMutationResolutionV1(
		request,
		equivocatingCheckpoint,
		&MonotonicHeadCommittedMutationV1{AdvanceRequest: advance, AdvanceReceipt: receipt},
		fixture.witnessSign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateMonotonicHeadMutationResolutionForRequestV1(
		equivocating,
		request,
		fixture.authority.installationID,
		fixture.authority.keyID,
		fixture.authority.publicKey,
		fixture.authority.enrollmentID,
		fixture.witnessKeyID,
		fixture.witnessPublic,
	) == nil {
		t.Fatal("witness equivocation at the committed generation was accepted")
	}

	left, _ := json.Marshal(receipt.Checkpoint)
	right, _ := json.Marshal(equivocatingCheckpoint)
	if bytes.Equal(left, right) {
		t.Fatal("equivocation fixture did not produce a distinct checkpoint")
	}
}
