package piiauthorization

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestControlledArtifactAccessReceiptAndDispositionBindExactProtectedPublicationWithoutRawPII(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV1(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil || ValidateControlledArtifactAccessReceiptForGrantV1(receipt, grant) != nil {
		t.Fatalf("access receipt did not remain within its exact PII grant: grant=%#v err=%v", grant, err)
	}
	body, err := ControlledArtifactAccessReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(piiGrantTestAccount)) {
		t.Fatalf("access authority leaked complete account data: %s", body)
	}
	parsed, err := ParseControlledArtifactAccessReceiptV1(body)
	if err != nil || parsed.AccessID != receipt.AccessID || parsed.RecordDigest != receipt.RecordDigest {
		t.Fatalf("access receipt strict roundtrip failed: parsed=%#v err=%v", parsed, err)
	}
	keyID, publicKey, signature, err := ControlledArtifactAccessReceiptAuthorityMaterialV1(parsed)
	if err != nil || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, ControlledArtifactAccessReceiptSigningBytesV1(parsed), signature) {
		t.Fatalf("access receipt authority material is invalid: key=%s err=%v", keyID, err)
	}
	disposition, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionHostReleaseCommittedV1, ControlledArtifactAccessReasonHostReleaseCommittedV1,
		receipt.ArtifactByteLength,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := ControlledArtifactAccessDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dispositionBody, []byte(piiGrantTestAccount)) {
		t.Fatalf("access disposition leaked complete account data: %s", dispositionBody)
	}
	parsedDisposition, err := ParseControlledArtifactAccessDispositionV1(dispositionBody)
	if err != nil || ValidateControlledArtifactAccessDispositionForReceiptV1(parsedDisposition, parsed) != nil {
		t.Fatalf("access disposition did not close the exact receipt: disposition=%#v err=%v", parsedDisposition, err)
	}
}

func TestControlledArtifactAccessFailsClosedOnUnknownFieldsTamperAndInvalidDeliveryState(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV1(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ControlledArtifactAccessReceiptV1Bytes(receipt)
	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"safeToAnswer":true}`)...)
	if _, err := ParseControlledArtifactAccessReceiptV1(unknown); err == nil {
		t.Fatal("access receipt accepted a model-controlled unknown field")
	}
	tampered := receipt
	tampered.TargetIdentityDigest = domainsecurity.SHA256Hex([]byte("attacker-target"))
	tampered.AccessID = controlledArtifactAccessIDV1(tampered)
	tampered.RecordDigest = controlledArtifactAccessReceiptRecordDigestV1(tampered)
	if ValidateControlledArtifactAccessReceiptV1(tampered) == nil {
		t.Fatal("access receipt accepted a target mutation without a host signature")
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionHostReleaseCommittedV1, ControlledArtifactAccessReasonHostReleaseCommittedV1,
		receipt.ArtifactByteLength-1,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("delivered disposition accepted a short write")
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionFailedV1, ControlledArtifactAccessReasonHostReleaseFailedV1,
		receipt.ArtifactByteLength,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("failed disposition claimed a complete delivery")
	}
}

func TestControlledArtifactAccessReceiptRejectsCrossWindowAndMismatchedDisposition(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV1(fixture)
	input.AuthorizedUntil = input.RequestedAt.Add(PIIProjectionGrantMaxTTL + time.Nanosecond)
	if _, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign); err == nil {
		t.Fatal("access receipt accepted an overlong authorization window")
	}
	input = controlledArtifactAccessInputV1(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionRejectedV1, ControlledArtifactAccessReasonAccessRejectedV1, 0,
		fixture.now.Add(3*time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	other := receipt
	other.UseSlotDigest = domainsecurity.SHA256Hex([]byte("other-access-slot"))
	other.AccessID = controlledArtifactAccessIDV1(other)
	other.AuthoritySignature = ""
	other.RecordDigest = ""
	other, err = NewControlledArtifactAccessReceiptV1(ControlledArtifactAccessReceiptInputV1{
		SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
		ControlledHandleDigest: input.ControlledHandleDigest, UseSlotDigest: other.UseSlotDigest,
		RendererPrincipalDigest: input.RendererPrincipalDigest, RendererGeneration: input.RendererGeneration,
		BackendGeneration: input.BackendGeneration, AccessPolicyDigest: input.AccessPolicyDigest,
		RetentionPolicyDigest:   input.RetentionPolicyDigest,
		PublicationCommitDigest: input.PublicationCommitDigest, PublicationReceiptDigest: input.PublicationReceiptDigest,
		PIIProjectionDigest: input.PIIProjectionDigest, PIIAuthorizationDigest: input.PIIAuthorizationDigest,
		ClaimLedgerDigest: input.ClaimLedgerDigest, TargetIdentityDigest: input.TargetIdentityDigest,
		ArtifactSHA256: input.ArtifactSHA256, ArtifactByteLength: input.ArtifactByteLength, MediaType: input.MediaType,
		RequestedAt: input.RequestedAt.Add(time.Nanosecond), AuthorizedUntil: input.AuthorizedUntil,
		AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, other) == nil {
		t.Fatal("access disposition closed a different logical access")
	}
}

func TestControlledArtifactAccessIDMakesUseSlotSingleUse(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	firstInput := controlledArtifactAccessInputV1(fixture)
	first, err := NewControlledArtifactAccessReceiptV1(firstInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := firstInput
	secondInput.RequestedAt = secondInput.RequestedAt.Add(time.Nanosecond)
	second, err := NewControlledArtifactAccessReceiptV1(secondInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessID != second.AccessID || first.RecordDigest == second.RecordDigest {
		t.Fatalf(
			"one use slot did not map to one reservation key: first=%s second=%s firstRecord=%s secondRecord=%s",
			first.AccessID, second.AccessID, first.RecordDigest, second.RecordDigest,
		)
	}
}

func TestControlledArtifactAccessReceiptCannotExceedGrantRightsOrExpiry(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	limitedInput := fixture.input
	limitedInput.AllowedAccessActions = []string{ControlledArtifactAccessActionDisplayV1}
	grant, err := NewPIIProjectionGrantV1(limitedInput, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	input := controlledArtifactAccessInputV1(fixture)
	input.PIIAuthorizationDigest = grant.RecordDigest
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateControlledArtifactAccessReceiptForGrantV1(receipt, grant) == nil {
		t.Fatal("export access exceeded a display-only PII grant")
	}

	fullGrant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	input = controlledArtifactAccessInputV1(fixture)
	input.PIIAuthorizationDigest = fullGrant.RecordDigest
	input.AuthorizedUntil = fixture.input.ExpiresAt.Add(time.Nanosecond)
	receipt, err = NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateControlledArtifactAccessReceiptForGrantV1(receipt, fullGrant) == nil {
		t.Fatal("access receipt outlived its PII grant")
	}
}

func TestControlledArtifactAccessDispositionEnforcesTimeAndIndeterminateReleaseSemantics(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV1(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionHostReleaseCommittedV1, ControlledArtifactAccessReasonHostReleaseCommittedV1,
		receipt.ArtifactByteLength, input.RequestedAt.Add(-time.Nanosecond), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("access disposition accepted a release before its request")
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionHostReleaseCommittedV1, ControlledArtifactAccessReasonHostReleaseCommittedV1,
		receipt.ArtifactByteLength, input.AuthorizedUntil, input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("access disposition accepted a committed release at grant expiry")
	}
	indeterminate, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionReleaseIndeterminateV1, ControlledArtifactAccessReasonReleaseIndeterminateV1,
		receipt.ArtifactByteLength, input.AuthorizedUntil.Add(time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	)
	if err != nil || ValidateControlledArtifactAccessDispositionForReceiptV1(indeterminate, receipt) != nil {
		t.Fatalf("full but indeterminate host release was not representable: disposition=%#v err=%v", indeterminate, err)
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionRejectedV1, ControlledArtifactAccessReasonAccessRejectedV1,
		1, input.RequestedAt.Add(time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("rejected access claimed released bytes")
	}
}

func TestControlledArtifactAccessMetadataCannotCarryRawPIIOrPaddedAuthorityAliases(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	input := controlledArtifactAccessInputV1(fixture)
	input.MediaType = piiGrantTestAccount
	if _, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign); err == nil {
		t.Fatal("access receipt accepted a PII-bearing media type")
	}
	input = controlledArtifactAccessInputV1(fixture)
	input.ArtifactByteLength = MaxControlledPIIArtifactBytesV1 + 1
	if _, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign); err == nil {
		t.Fatal("access receipt accepted an oversized controlled artifact")
	}
	input = controlledArtifactAccessInputV1(fixture)
	receipt, err := NewControlledArtifactAccessReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	padded := receipt
	padded.UseSlotDigest = " " + receipt.UseSlotDigest
	padded.AccessID = controlledArtifactAccessIDV1(padded)
	padded.AuthoritySignature = ""
	padded.RecordDigest = ""
	if ValidateControlledArtifactAccessReceiptV1(padded) == nil {
		t.Fatal("access receipt accepted a padded use-slot digest")
	}
	if _, err := NewControlledArtifactAccessDispositionV1(
		receipt, ControlledArtifactAccessDispositionFailedV1, piiGrantTestAccount, 0,
		input.RequestedAt.Add(time.Minute), input.AuthorityKeyID, input.AuthorityPublicKey, fixture.sign,
	); err == nil {
		t.Fatal("access disposition accepted a PII-bearing reason code")
	}
}

func controlledArtifactAccessInputV1(fixture piiGrantFixture) ControlledArtifactAccessReceiptInputV1 {
	grant, _ := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	return ControlledArtifactAccessReceiptInputV1{
		SecurityContext: fixture.input.SecurityContext, AccessAction: ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest:   domainsecurity.SHA256Hex([]byte("controlled-artifact-handle")),
		UseSlotDigest:            domainsecurity.SHA256Hex([]byte("controlled-access-use-slot")),
		RendererPrincipalDigest:  domainsecurity.SHA256Hex([]byte("renderer-principal")),
		RendererGeneration:       7,
		BackendGeneration:        11,
		AccessPolicyDigest:       fixture.input.AccessPolicyDigest,
		RetentionPolicyDigest:    fixture.input.RetentionPolicyDigest,
		PublicationCommitDigest:  domainsecurity.SHA256Hex([]byte("controlled-publication-commit")),
		PublicationReceiptDigest: domainsecurity.SHA256Hex([]byte("controlled-publication-receipt")),
		PIIProjectionDigest:      domainsecurity.SHA256Hex([]byte("controlled-pii-projection")),
		PIIAuthorizationDigest:   grant.RecordDigest,
		ClaimLedgerDigest:        fixture.input.ClaimLedgerDigest, TargetIdentityDigest: fixture.input.TargetIdentityDigest,
		ArtifactSHA256: fixture.input.ProjectedContentSHA256, ArtifactByteLength: 4096,
		MediaType: ControlledPIIArtifactMediaTypeV1, RequestedAt: fixture.now.Add(2 * time.Minute),
		AuthorizedUntil: fixture.input.ExpiresAt, AuthorityKeyID: fixture.input.AuthorityKeyID,
		AuthorityPublicKey: fixture.input.AuthorityPublicKey,
	}
}
