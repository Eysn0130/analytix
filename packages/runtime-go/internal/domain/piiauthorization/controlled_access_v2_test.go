package piiauthorization

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestControlledAccessV2RequiresExactProjectedDeliveryOutcome(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV2(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV2(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil || ValidateControlledArtifactAccessReceiptForGrantV2(receipt, grant) != nil {
		t.Fatalf("V2 receipt exceeded its exact PII grant: grant=%#v err=%v", grant, err)
	}
	if receipt.DeliveryID != input.DeliveryID || receipt.DeliveryOutcomeRecordDigest != input.DeliveryOutcomeRecordDigest ||
		receipt.PublicationCommitDigest != input.PublicationCommitDigest ||
		receipt.ReleaseTargetIdentityDigest != input.ReleaseTargetIdentityDigest ||
		receipt.ReleaseTargetIdentityDigest == receipt.TargetIdentityDigest {
		t.Fatalf("V2 receipt lost projected-delivery binding: %#v", receipt)
	}
	body, err := ControlledArtifactAccessReceiptV2Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(piiGrantTestAccount)) {
		t.Fatalf("V2 access authority leaked complete account data: %s", body)
	}
	parsed, err := ParseControlledArtifactAccessReceiptV2(body)
	if err != nil || parsed != receipt {
		t.Fatalf("V2 receipt strict roundtrip failed: parsed=%#v err=%v", parsed, err)
	}
	keyID, publicKey, signature, err := ControlledArtifactAccessReceiptAuthorityMaterialV2(parsed)
	if err != nil || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, ControlledArtifactAccessReceiptSigningBytesV2(parsed), signature) {
		t.Fatalf("V2 receipt authority material is invalid: key=%s err=%v", keyID, err)
	}
	disposition, err := NewControlledArtifactAccessDispositionV2(
		receipt, ControlledArtifactAccessDispositionHostReleaseCommittedV1,
		ControlledArtifactAccessReasonHostReleaseCommittedV1, receipt.ArtifactByteLength,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	)
	if err != nil || disposition.DeliveryID != receipt.DeliveryID ||
		disposition.DeliveryOutcomeRecordDigest != receipt.DeliveryOutcomeRecordDigest ||
		disposition.ReleaseTargetIdentityDigest != receipt.ReleaseTargetIdentityDigest ||
		ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		t.Fatalf("V2 disposition lost exact projected winner: disposition=%#v err=%v", disposition, err)
	}
}

func TestControlledAccessV2SlotCannotRebindAcrossDeliveryOutcomes(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	firstInput := controlledArtifactAccessInputV2(fixture)
	first, err := NewControlledArtifactAccessReceiptV2(firstInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := firstInput
	secondInput.DeliveryOutcomeRecordDigest = domainsecurity.SHA256Hex([]byte("different projected winner"))
	secondInput.RequestedAt = secondInput.RequestedAt.Add(time.Nanosecond)
	second, err := NewControlledArtifactAccessReceiptV2(secondInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessID != second.AccessID || first.RecordDigest == second.RecordDigest {
		t.Fatalf("same use slot did not force a no-replace conflict across outcomes: first=%#v second=%#v", first, second)
	}
	disposition, err := NewControlledArtifactAccessDispositionV2(
		first, ControlledArtifactAccessDispositionRejectedV1, ControlledArtifactAccessReasonAccessRejectedV1, 0,
		fixture.now.Add(3*time.Minute), firstInput.AuthorityKeyID, firstInput.AuthorityPublicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, second) == nil {
		t.Fatal("V2 disposition closed the same slot under a different projected outcome")
	}
}

func TestControlledAccessV1AndV2CannotCrossParseOrAcceptUnknownFields(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	v1, err := NewControlledArtifactAccessReceiptV1(controlledArtifactAccessInputV1(fixture), fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	v1Body, _ := ControlledArtifactAccessReceiptV1Bytes(v1)
	if _, err := ParseControlledArtifactAccessReceiptV2(v1Body); err == nil {
		t.Fatal("legacy V1 receipt parsed as V2 authority")
	}
	v2, err := NewControlledArtifactAccessReceiptV2(controlledArtifactAccessInputV2(fixture), fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	v2Body, _ := ControlledArtifactAccessReceiptV2Bytes(v2)
	if _, err := ParseControlledArtifactAccessReceiptV1(v2Body); err == nil {
		t.Fatal("V2 receipt parsed as legacy V1 authority")
	}
	unknown := append([]byte(nil), v2Body[:len(v2Body)-1]...)
	unknown = append(unknown, []byte(`,"safeToAnswer":true}`)...)
	if _, err := ParseControlledArtifactAccessReceiptV2(unknown); err == nil {
		t.Fatal("V2 receipt accepted a model-controlled unknown field")
	}
}

func TestControlledAccessV2RejectsMissingOrTamperedDeliveryBinding(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV2(fixture)
	input.DeliveryID = ""
	if _, err := NewControlledArtifactAccessReceiptV2(input, fixture.sign); err == nil {
		t.Fatal("V2 receipt accepted a missing delivery ID")
	}
	input = controlledArtifactAccessInputV2(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV2(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	tampered := receipt
	tampered.DeliveryOutcomeRecordDigest = domainsecurity.SHA256Hex([]byte("tampered projected outcome"))
	tampered.RecordDigest = controlledArtifactAccessReceiptRecordDigestV2(tampered)
	if ValidateControlledArtifactAccessReceiptV2(tampered) == nil {
		t.Fatal("V2 receipt accepted a projected-outcome mutation without a host signature")
	}
	tampered = receipt
	tampered.ReleaseTargetIdentityDigest = domainsecurity.SHA256Hex([]byte("other trusted sink target"))
	tampered.RecordDigest = controlledArtifactAccessReceiptRecordDigestV2(tampered)
	if ValidateControlledArtifactAccessReceiptV2(tampered) == nil {
		t.Fatal("V2 receipt accepted a trusted-sink mutation without a host signature")
	}
	missingTarget := input
	missingTarget.ReleaseTargetIdentityDigest = ""
	if _, err := NewControlledArtifactAccessReceiptV2(missingTarget, fixture.sign); err == nil {
		t.Fatal("V2 receipt accepted a missing trusted-sink target binding")
	}
	aliasedTarget := input
	aliasedTarget.ReleaseTargetIdentityDigest = aliasedTarget.TargetIdentityDigest
	if _, err := NewControlledArtifactAccessReceiptV2(aliasedTarget, fixture.sign); err == nil {
		t.Fatal("V2 receipt accepted the private artifact CAS identity as its trusted-sink identity")
	}
}

func TestControlledAccessV2UsesConservativeTerminalByteSemantics(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV2(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV2(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewControlledArtifactAccessDispositionV2(
		receipt, ControlledArtifactAccessDispositionReleaseIndeterminateV1,
		ControlledArtifactAccessReasonReleaseIndeterminateV1, receipt.ArtifactByteLength-1,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("V2 indeterminate release understated the conservative exposure bound")
	}
	if _, err := NewControlledArtifactAccessDispositionV2(
		receipt, ControlledArtifactAccessDispositionFailedV1,
		ControlledArtifactAccessReasonAuthorityUnavailableV2, 1,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("V2 pre-sink authority failure claimed released bytes")
	}
	failed, err := NewControlledArtifactAccessDispositionV2(
		receipt, ControlledArtifactAccessDispositionFailedV1,
		ControlledArtifactAccessReasonAuthorityIntegrityV2, 0,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	)
	if err != nil || ValidateControlledArtifactAccessDispositionForReceiptV2(failed, receipt) != nil {
		t.Fatalf("V2 could not represent a zero-byte authority failure: disposition=%#v err=%v", failed, err)
	}
}

func controlledArtifactAccessInputV2(fixture piiGrantFixture) ControlledArtifactAccessReceiptInputV2 {
	v1 := controlledArtifactAccessInputV1(fixture)
	handleDigest, _ := ControlledAccessHandleDigestV2("controlled-artifact-handle-token-v2")
	useSlotDigest, _ := ControlledAccessUseSlotDigestV2("controlled-access-use-slot-token-v2")
	principalDigest, _ := ControlledAccessRendererPrincipalDigestV2("controlled-renderer-principal-token-v2")
	return ControlledArtifactAccessReceiptInputV2{
		SecurityContext: v1.SecurityContext, AccessAction: v1.AccessAction,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest,
		RendererPrincipalDigest: principalDigest, RendererGeneration: v1.RendererGeneration,
		BackendGeneration: v1.BackendGeneration, AccessPolicyDigest: v1.AccessPolicyDigest,
		RetentionPolicyDigest:       v1.RetentionPolicyDigest,
		DeliveryID:                  domainsecurity.SHA256Hex([]byte("controlled projected delivery")),
		DeliveryOutcomeRecordDigest: domainsecurity.SHA256Hex([]byte("controlled projected outcome record")),
		PublicationCommitDigest:     v1.PublicationCommitDigest, PublicationReceiptDigest: v1.PublicationReceiptDigest,
		PIIProjectionDigest: v1.PIIProjectionDigest, PIIAuthorizationDigest: v1.PIIAuthorizationDigest,
		ClaimLedgerDigest: v1.ClaimLedgerDigest, TargetIdentityDigest: v1.TargetIdentityDigest,
		ReleaseTargetIdentityDigest: domainsecurity.SHA256Hex([]byte("controlled trusted sink target v2")),
		ArtifactSHA256:              v1.ArtifactSHA256, ArtifactByteLength: v1.ArtifactByteLength, MediaType: v1.MediaType,
		RequestedAt: v1.RequestedAt, AuthorizedUntil: v1.AuthorizedUntil,
		AuthorityKeyID: v1.AuthorityKeyID, AuthorityPublicKey: v1.AuthorityPublicKey,
	}
}
