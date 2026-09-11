package evidence

import (
	"context"
	"encoding/json"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestClaimVerifierAcceptsOnlyExactCurrentEvidence(t *testing.T) {
	payload := amountClaimPayload()
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, payload, "transactions", domainevidence.PaginationComplete)
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	proposal.EvidenceIDs = []string{receipt.ReceiptID}
	verifier := claimVerifierForTest(issuer)
	record, err := verifier.Verify(context.Background(), input.Context, proposal)
	if err != nil || record.SupportState != domainevidence.ClaimVerified || len(record.EvidenceIDs) != 1 ||
		record.EvidenceIDs[0] != receipt.ReceiptID || record.VerifierReceiptID == "" || record.SupportedScope == nil {
		t.Fatalf("exact current evidence was not verified: record=%#v err=%v", record, err)
	}
}

func TestFakeCitationRejected(t *testing.T) {
	payload := amountClaimPayload()
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, payload, "transactions", domainevidence.PaginationComplete)
	proposal.EvidenceIDs = []string{"provider-forged-receipt"}
	record, err := claimVerifierForTest(issuer).Verify(context.Background(), input.Context, proposal)
	if err != nil || record.SupportState != domainevidence.ClaimUnresolved || len(record.EvidenceIDs) != 0 || record.VerifierReceiptID != "" {
		t.Fatalf("fake citation was not rejected mechanically: record=%#v err=%v", record, err)
	}
}

func TestMismatchedCitationRejected(t *testing.T) {
	payload := amountClaimPayload()
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAmount, payload, "transactions", domainevidence.PaginationComplete)
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	cases := map[string]func(*domainevidence.NormalizedClaimPayload){
		"subject":   func(value *domainevidence.NormalizedClaimPayload) { value.SubjectID = "entity-b" },
		"amount":    func(value *domainevidence.NormalizedClaimPayload) { value.AmountMinor = "4200001" },
		"account":   func(value *domainevidence.NormalizedClaimPayload) { value.AccountID = "00999999999999999999" },
		"direction": func(value *domainevidence.NormalizedClaimPayload) { value.Direction = "in" },
		"date":      func(value *domainevidence.NormalizedClaimPayload) { value.StartAt = "2026-01-02T00:00:00Z" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := proposal
			candidate.EvidenceIDs = []string{receipt.ReceiptID}
			mutate(&candidate.NormalizedPayload)
			record, err := claimVerifierForTest(issuer).Verify(context.Background(), input.Context, candidate)
			if err != nil {
				if record.SchemaVersion != 0 || record.ClaimID != "" || record.SupportState != "" {
					t.Fatalf("invalid mismatched claim returned a usable record: record=%#v err=%v", record, err)
				}
				return
			}
			if record.SupportState != domainevidence.ClaimUnresolved || len(record.EvidenceIDs) != 0 {
				t.Fatalf("mismatched citation was accepted: record=%#v err=%v", record, err)
			}
		})
	}
}

func TestClaimVerifierEnforcesSourceCapabilityAndPartialScope(t *testing.T) {
	ownershipPayload := domainevidence.NormalizedClaimPayload{
		SubjectID: "entity-a", EntityID: "entity-a", AttributeName: "shareholder", AttributeValue: "person-a", Granularity: "record",
	}
	issuer, input, _ := claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete)
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-ownership-from-transaction",
		ClaimType: domainevidence.ClaimOwnership, NormalizedPayload: ownershipPayload,
		EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{},
	}
	record, err := claimVerifierForTest(issuer).Verify(context.Background(), input.Context, proposal)
	if err != nil || record.SupportState != domainevidence.ClaimUnresolved || len(record.EvidenceIDs) != 0 ||
		record.VerificationReason != "citation_missing_mismatched_or_source_unauthorized" ||
		domainevidence.SourceTypeSupportsClaim("transactions", domainevidence.ClaimOwnership) {
		t.Fatalf("transaction source supported an ownership claim: record=%#v err=%v", record, err)
	}

	issuer, input, proposal = claimEvidenceFixture(t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationPartial)
	receipt = seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	proposal.EvidenceIDs = []string{receipt.ReceiptID}
	record, err = claimVerifierForTest(issuer).Verify(context.Background(), input.Context, proposal)
	if err != nil || record.SupportState != domainevidence.ClaimPartial || len(record.AllowedWording) != 1 || record.AllowedWording[0] != "within_checked_scope_only" {
		t.Fatalf("partial receipt became whole verified claim: record=%#v err=%v", record, err)
	}
}

func TestClaimProposalStrictSchemaAndExactNumericStrings(t *testing.T) {
	if _, err := domainevidence.ParseClaimProposal(json.RawMessage(`{
		"schemaVersion":1,"proposalId":"p","claimType":"amount",
		"normalizedPayload":{"subjectId":"entity-a","entityId":"","accountId":"","counterpartyId":"","amountMinor":"4200000","count":"","currency":"CNY","direction":"","startAt":"","endAt":"","relationshipType":"","quote":"","deviceKind":"","deviceIdentifier":"","attributeName":"","attributeValue":"","legalCharacterization":"","granularity":"transaction"},
		"evidenceIds":[],"counterEvidenceIds":[],"unexpected":true
	}`)); err == nil {
		t.Fatal("unknown claim proposal property was accepted")
	}
	if _, err := domainevidence.ParseClaimProposal(json.RawMessage(`{
		"schemaVersion":1,"proposalId":"p","claimType":"amount",
		"normalizedPayload":{"subjectId":"entity-a","entityId":"","accountId":"","counterpartyId":"","amountMinor":4200000,"count":"","currency":"CNY","direction":"","startAt":"","endAt":"","relationshipType":"","quote":"","deviceKind":"","deviceIdentifier":"","attributeName":"","attributeValue":"","legalCharacterization":"","granularity":"transaction"},
		"evidenceIds":[],"counterEvidenceIds":[]
	}`)); err == nil {
		t.Fatal("numeric amount bypassed exact string contract")
	}
}

func claimEvidenceFixture(t *testing.T, claimType domainevidence.ClaimType, payload domainevidence.NormalizedClaimPayload, sourceType string, pagination domainevidence.PaginationCompleteness) (Issuer, IssueEvidenceInput, domainevidence.ClaimProposal) {
	t.Helper()
	issuer, input := evidenceIssuerFixture(t)
	fact := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts:         []domainevidence.CanonicalEvidenceFact{{FactID: "fact-a", ClaimType: claimType, NormalizedPayload: payload}},
	}
	canonical, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	input.Material.CanonicalEvidence = canonical
	input.Material.SourceType = sourceType
	input.Material.PaginationCompleteness = pagination
	alignEvidenceOutcomeWithPagination(&input, pagination)
	input.Material.QueryRange.EntityIDs = []string{"entity-a"}
	if payload.AccountID != "" {
		input.Material.QueryRange.AccountIDs = []string{payload.AccountID}
	}
	if payload.Direction != "" {
		input.Material.QueryRange.Directions = []string{payload.Direction}
	}
	if payload.StartAt != "" {
		input.Material.QueryRange.StartAt = payload.StartAt
		input.Material.QueryRange.EndAt = payload.EndAt
	}
	input.Material.TransformationLineage[0].OutputHash = domainsecurity.CanonicalJSONHash(canonical)
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-a", ClaimType: claimType,
		NormalizedPayload: payload, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	return issuer, input, proposal
}

func claimVerifierForTest(issuer Issuer) ClaimVerifier {
	return ClaimVerifier{
		Registry: issuer.Registry, IDGenerator: func() (string, error) { return "claim-host-a", nil }, Now: evidenceIssuerTime,
	}
}

func amountClaimPayload() domainevidence.NormalizedClaimPayload {
	return domainevidence.NormalizedClaimPayload{
		SubjectID: "entity-a", EntityID: "entity-a", AccountID: "00123456789012345678",
		AmountMinor: "4200000", Currency: "CNY", Direction: "out",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
	}
}
