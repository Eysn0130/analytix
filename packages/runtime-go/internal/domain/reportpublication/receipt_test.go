package reportpublication

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPublicationReceiptBindsExactReportClaimSnapshotPIIAndTarget(t *testing.T) {
	fixture := publicationReceiptFixture(t, PIIProjectionOrdinaryMasked)
	if err := ValidatePublicationReceiptForInstallationV1(
		fixture.receipt, fixture.installationID, fixture.enrollmentID, fixture.keyID, fixture.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicationReceiptMaterialsV1(fixture.receipt, fixture.ledger, fixture.projection, fixture.inspection); err != nil {
		t.Fatal(err)
	}
	index, err := NewPublicationIndexV1(PublicationIndexInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID, Generation: 1,
		PreviousIndexDigest: PublicationIndexGenesisDigestV1(), MutationID: domainsecurity.SHA256Hex([]byte("publication-index-mutation")),
		ReceiptID: fixture.receipt.ReceiptID, ReceiptRecordDigest: fixture.receipt.RecordDigest,
		TargetIdentityDigest: fixture.receipt.TargetIdentityDigest, AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicationIndexReceiptV1(index, fixture.receipt); err != nil {
		t.Fatal(err)
	}

	mutatedInspection := fixture.inspection
	mutatedInspection.ReportSHA256 = domainsecurity.SHA256Hex([]byte("different-report"))
	mutatedInspection.InspectionDigest = renderInspectionDigestV1(mutatedInspection)
	if err := ValidatePublicationReceiptMaterialsV1(fixture.receipt, fixture.ledger, fixture.projection, mutatedInspection); err == nil {
		t.Fatal("publication receipt accepted a different inspected report")
	}
	mutatedLedger := fixture.ledger
	mutatedLedger.EvidenceReceiptIDs = []string{"evr_" + domainsecurity.SHA256Hex([]byte("other-evidence"))}
	mutatedLedger.LedgerDigest = claimLedgerDigestV1(mutatedLedger)
	if err := ValidatePublicationReceiptMaterialsV1(fixture.receipt, mutatedLedger, fixture.projection, fixture.inspection); err == nil {
		t.Fatal("publication receipt accepted a different evidence receipt set")
	}
}

func TestPublicationReceiptContainsNoRawBankAccountAndControlledProjectionRequiresAudit(t *testing.T) {
	ordinary := publicationReceiptFixture(t, PIIProjectionOrdinaryMasked)
	body, err := PublicationReceiptV1Bytes(ordinary.receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), publicationTestBankAccount) {
		t.Fatalf("publication receipt leaked a raw bank account: %s", body)
	}
	if _, err := NewPIIProjectionV1(PIIProjectionInputV1{
		ProjectionClass: PIIProjectionOrdinaryMasked, RulesetHash: domainsecurity.SHA256Hex([]byte("rules")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("content")), PreservedControlledFieldCount: 1,
		AuthorizationAuditDigest: domainsecurity.SHA256Hex([]byte("unauthorized")),
	}); err == nil {
		t.Fatal("ordinary projection preserved a controlled account")
	}
	controlled := publicationReceiptFixture(t, PIIProjectionControlledFull)
	if controlled.receipt.AuthorizationAuditDigest == "" || controlled.receipt.PIIProjectionClass != PIIProjectionControlledFull {
		t.Fatalf("controlled publication lost authorization authority: %#v", controlled.receipt)
	}
	controlledBody, err := PublicationReceiptV1Bytes(controlled.receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(controlledBody), publicationTestBankAccount) {
		t.Fatalf("controlled receipt leaked raw account instead of binding the controlled artifact by hash: %s", controlledBody)
	}
}

func TestVerifiedNoHitPublicationRejectsPositiveClaimAndFailedInspection(t *testing.T) {
	fixture := publicationReceiptFixture(t, PIIProjectionOrdinaryMasked)
	input := fixture.input
	input.ReportVariant = VerifiedNoHitReport
	if _, err := NewPublicationReceiptV1(input, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.privateKey, message), nil }); err == nil {
		t.Fatal("verified no-hit publication accepted a positive claim")
	}
	failed, err := NewRenderInspectionV1(RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: fixture.receipt.ReportSHA256,
		ReportByteLength: fixture.receipt.ReportByteLength, MediaType: fixture.receipt.MediaType,
		Passed: false, IssueCodes: []string{"render_mismatch"}, InspectedAt: time.Date(2026, 7, 13, 6, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	input = fixture.input
	input.RenderInspection = failed
	if _, err := NewPublicationReceiptV1(input, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.privateKey, message), nil }); err == nil {
		t.Fatal("formal publication accepted a failed render inspection")
	}
}

const publicationTestBankAccount = "6222020202020202020"

type publicationReceiptTestFixture struct {
	input          PublicationReceiptInputV1
	receipt        PublicationReceiptV1
	ledger         ClaimLedgerV1
	projection     PIIProjectionV1
	inspection     RenderInspectionV1
	installationID string
	enrollmentID   string
	keyID          string
	publicKey      ed25519.PublicKey
	privateKey     ed25519.PrivateKey
}

func publicationReceiptFixture(t *testing.T, projectionClass string) publicationReceiptTestFixture {
	t.Helper()
	now := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication", TurnID: "turn-publication", WorkspaceRealPath: "/workspace/publication",
		CaseID: "case-publication", CaseBindingHash: domainsecurity.SHA256Hex([]byte("publication-binding")), ContextEpoch: 5, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("publication-evidence"))
	claimID := "claim-account-exact"
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", AccountID: publicationTestBankAccount}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-account-exact",
		ClaimType: domainevidence.ClaimAccount, NormalizedPayload: payload, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
	}
	scope := &domainevidence.EvidenceQueryRange{
		EntityIDs: []string{"entity-a"}, AccountIDs: []string{publicationTestBankAccount}, Directions: []string{}, SourceIDs: []string{"bank-flow"},
		FiltersHash: domainsecurity.SHA256Hex([]byte("publication-claim-scope")),
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: proposal, SupportState: domainevidence.ClaimVerified, EvidenceIDs: []string{evidenceID},
		CounterEvidenceIDs: []string{}, SupportedScope: scope, AllowedWording: []string{"the exact account is verified within the authorized evidence scope"},
		ProhibitedUpgrades: []string{"do not infer account ownership"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest(claimID, domainevidence.ClaimAccount, payload, []string{evidenceID}, []string{}, domainevidence.ClaimVerified),
		VerificationReason: "exact account field match", VerifiedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := NewClaimLedgerV1(ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	report := []byte("deterministic publication report")
	reportSHA := domainsecurity.SHA256Hex(report)
	projectionInput := PIIProjectionInputV1{
		ProjectionClass: projectionClass, RulesetHash: domainsecurity.SHA256Hex([]byte("pii-ruleset-v1")),
		ProjectedContentSHA256: reportSHA, RestrictedFieldCount: 1,
	}
	if projectionClass == PIIProjectionControlledFull {
		projectionInput.PreservedControlledFieldCount = 1
		projectionInput.AuthorizationAuditDigest = domainsecurity.SHA256Hex([]byte("controlled-artifact-authorization"))
	}
	projection, err := NewPIIProjectionV1(projectionInput)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewRenderInspectionV1(RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA,
		ReportByteLength: uint64(len(report)), MediaType: "application/pdf", Passed: true, IssueCodes: []string{},
		InspectedAt: now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x71}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	input := PublicationReceiptInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("publication-installation")),
		EnrollmentID:   domainsecurity.SHA256Hex([]byte("publication-enrollment")), Context: securityContext,
		ReportVariant: EvidenceBackedReport, EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("evidence-authority-bundle")),
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("evidence-registry-index")), EvidenceRegistryCount: 3,
		EvidenceRegistrySequence: 1, EvidenceRegistryStateDigest: domainsecurity.SHA256Hex([]byte("evidence-registry-state")),
		ClaimLedger: ledger, ReportSHA256: reportSHA, ReportByteLength: uint64(len(report)), MediaType: "application/pdf",
		PIIProjection: projection, RenderInspection: inspection, Publisher: "analytix-host", PublisherVersion: "1.0.0",
		TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("publication-target")), IssuedAt: now.Add(4 * time.Minute),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	receipt, err := NewPublicationReceiptV1(input, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return publicationReceiptTestFixture{
		input: input, receipt: receipt, ledger: ledger, projection: projection, inspection: inspection,
		installationID: input.InstallationID, enrollmentID: input.EnrollmentID, keyID: keyID, publicKey: publicKey, privateKey: privateKey,
	}
}
