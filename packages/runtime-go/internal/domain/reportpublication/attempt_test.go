package reportpublication

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPublicationAttemptFixesOneReportStagePlanBeforeSideEffects(t *testing.T) {
	fixture := publicationAttemptFixture(t)
	attempt := fixture.attempt
	if err := ValidatePublicationAttemptForInstallationV1(
		attempt, fixture.receiptFixture.installationID, fixture.receiptFixture.enrollmentID,
		fixture.receiptFixture.keyID, fixture.receiptFixture.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicationAttemptGraphV1(attempt, fixture.stageReceipt, fixture.receiptFixture.receipt, fixture.index); err != nil {
		t.Fatal(err)
	}
	if attempt.PublicationIndexMutationID != fixture.index.MutationID ||
		attempt.PublicationIndexMutationID != PublicationIndexMutationIDForAttemptV1(attempt.AttemptID) ||
		attempt.AuthorityAdvanceMutationID != PublicationAuthorityMutationIDForAttemptV1(attempt.AttemptID) {
		t.Fatalf("publication attempt did not fix both mutation identities: %#v", attempt)
	}
	body, err := PublicationAttemptV1Bytes(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), publicationTestBankAccount) {
		t.Fatalf("publication attempt leaked raw controlled data: %s", body)
	}
	parsed, err := ParsePublicationAttemptV1(body)
	if err != nil || parsed != attempt {
		t.Fatalf("publication attempt did not round trip canonically: parsed=%#v err=%v", parsed, err)
	}
}

func TestPublicationAttemptRejectsProviderRawToolCallIdentity(t *testing.T) {
	fixture := publicationAttemptFixture(t)
	fixture.input.ToolCallID = "provider_call_6222020202020202020"
	if _, err := NewPublicationAttemptV1(fixture.input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.receiptFixture.privateKey, message), nil
	}); err == nil || strings.Contains(err.Error(), "6222020202020202020") {
		t.Fatalf("publication attempt accepted or reflected provider identity: %v", err)
	}
}

func TestPublicationAttemptSameStageCannotChangePlanWithoutConflictIdentity(t *testing.T) {
	fixture := publicationAttemptFixture(t)
	changed := fixture.input
	changed.StageInputHash = domainsecurity.SHA256Hex([]byte("changed-stage-input"))
	second, err := NewPublicationAttemptV1(changed, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.receiptFixture.privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.AttemptID != fixture.attempt.AttemptID || second.RecordDigest == fixture.attempt.RecordDigest {
		t.Fatalf("same report-stage work did not map changed plans to one exclusive identity: first=%#v second=%#v", fixture.attempt, second)
	}
	if second.PublicationIndexMutationID != fixture.attempt.PublicationIndexMutationID ||
		second.AuthorityAdvanceMutationID != fixture.attempt.AuthorityAdvanceMutationID {
		t.Fatal("changed plan minted different retry mutation identities")
	}
}

func TestPublicationAttemptRejectsUnknownFieldsAndForeignStageReceipt(t *testing.T) {
	fixture := publicationAttemptFixture(t)
	body, err := PublicationAttemptV1Bytes(fixture.attempt)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParsePublicationAttemptV1(unknown); err == nil {
		t.Fatal("publication attempt accepted an unknown property")
	}

	foreignKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x7f}, ed25519.SeedSize))
	foreignPublic := foreignKey.Public().(ed25519.PublicKey)
	foreignReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: fixture.receiptFixture.input.Context,
		GrantRegistrySequence: 1, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant")), RegistrySequence: 1,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload")), RouteHash: domainsecurity.SHA256Hex([]byte("route")),
		IssuedAt: time.Date(2026, 7, 13, 6, 3, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 7, 13, 7, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(foreignPublic), AuthorityPublicKey: foreignPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(foreignKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	foreignInput := fixture.input
	foreignInput.ReportStageReceipt = foreignReceipt
	if _, err := NewPublicationAttemptV1(foreignInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.receiptFixture.privateKey, message), nil
	}); err == nil {
		t.Fatal("publication attempt accepted a report-stage receipt from a foreign authority")
	}
}

type publicationAttemptTestFixture struct {
	receiptFixture publicationReceiptTestFixture
	stageReceipt   domainpendingwork.PendingWorkReceiptV1
	index          PublicationIndexV1
	input          PublicationAttemptInputV1
	attempt        PublicationAttemptV1
}

func publicationAttemptFixture(t *testing.T) publicationAttemptTestFixture {
	t.Helper()
	receiptFixture := publicationReceiptFixture(t, PIIProjectionOrdinaryMasked)
	issuedAt := time.Date(2026, 7, 13, 6, 3, 0, 0, time.UTC)
	stageReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: receiptFixture.input.Context,
		GrantRegistrySequence: 7, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("publication-grant-registry")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("publication-grant")), RegistrySequence: 7,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("publication-grant-entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("publication-stage-payload")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("publication-stage-route")),
		IssuedAt:    issuedAt, ExpiresAt: issuedAt.Add(time.Hour),
		AuthorityKeyID: receiptFixture.keyID, AuthorityPublicKey: receiptFixture.publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(receiptFixture.privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	attemptID := PublicationAttemptIDV1(receiptFixture.installationID, receiptFixture.enrollmentID, stageReceipt.WorkID)
	index, err := NewPublicationIndexV1(PublicationIndexInputV1{
		InstallationID: receiptFixture.installationID, EnrollmentID: receiptFixture.enrollmentID,
		Generation: 1, PreviousIndexDigest: PublicationIndexGenesisDigestV1(),
		MutationID: PublicationIndexMutationIDForAttemptV1(attemptID), ReceiptID: receiptFixture.receipt.ReceiptID,
		ReceiptRecordDigest: receiptFixture.receipt.RecordDigest, TargetIdentityDigest: receiptFixture.receipt.TargetIdentityDigest,
		AuthorityKeyID: receiptFixture.keyID, AuthorityPublicKey: receiptFixture.publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(receiptFixture.privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	toolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x73}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	input := PublicationAttemptInputV1{
		InstallationID: receiptFixture.installationID, EnrollmentID: receiptFixture.enrollmentID,
		ReportStageReceipt: stageReceipt, ToolCallID: toolCallID,
		StageInputHash: domainsecurity.SHA256Hex([]byte("publication-stage-input")),
		Candidate:      receiptFixture.receipt, Index: index,
		ExpectedEvidenceBundleDigest:   receiptFixture.receipt.EvidenceAuthorityBundleDigest,
		ExpectedPublicationIndexDigest: PublicationIndexGenesisDigestV1(), ExpectedPublicationCount: 0,
		NextEvidenceBundleDigest:     domainsecurity.SHA256Hex([]byte("publication-next-evidence-bundle")),
		AuthorityAdvanceIntentDigest: domainsecurity.SHA256Hex([]byte("publication-authority-advance-intent")),
		AuthorityKeyID:               receiptFixture.keyID, AuthorityPublicKey: receiptFixture.publicKey,
	}
	attempt, err := NewPublicationAttemptV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(receiptFixture.privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return publicationAttemptTestFixture{
		receiptFixture: receiptFixture, stageReceipt: stageReceipt, index: index, input: input, attempt: attempt,
	}
}
