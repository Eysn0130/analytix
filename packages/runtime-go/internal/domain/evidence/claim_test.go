package evidence

import "testing"

func TestLegalCharacterizationCannotBeSmuggledThroughAttributeClaim(t *testing.T) {
	for _, proposal := range []ClaimProposal{
		{
			SchemaVersion: ClaimProposalVersion, ProposalID: "p-1", ClaimType: ClaimBidEditMetadata,
			NormalizedPayload: NormalizedClaimPayload{
				SubjectID: "entity-a", EntityID: "entity-a", AttributeName: "document_author",
				AttributeValue: "person-a", LegalCharacterization: "已构成串通投标", Granularity: "record",
			}, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
		},
		{
			SchemaVersion: ClaimProposalVersion, ProposalID: "p-2", ClaimType: ClaimBidEditMetadata,
			NormalizedPayload: NormalizedClaimPayload{
				SubjectID: "entity-a", EntityID: "entity-a", AttributeName: "legal_characterization",
				AttributeValue: "具备立案条件", Granularity: "record",
			}, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
		},
	} {
		if _, err := NormalizeClaimProposal(proposal); err == nil {
			t.Fatalf("legal characterization passed through attribute variant: %#v", proposal)
		}
	}
}

func TestRelationshipTypeCannotCarryLegalConclusion(t *testing.T) {
	proposal := ClaimProposal{
		SchemaVersion: ClaimProposalVersion, ProposalID: "p-1", ClaimType: ClaimRelationship,
		NormalizedPayload: NormalizedClaimPayload{
			SubjectID: "person-a", CounterpartyID: "person-b", RelationshipType: "行贿和利益输送", Granularity: "record",
		}, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	if _, err := NormalizeClaimProposal(proposal); err == nil {
		t.Fatal("legal conclusion passed through relationship type")
	}
}

func TestClaimPayloadVariantsRejectUnrelatedFields(t *testing.T) {
	proposal := ClaimProposal{
		SchemaVersion: ClaimProposalVersion, ProposalID: "p-1", ClaimType: ClaimAccount,
		NormalizedPayload: NormalizedClaimPayload{
			SubjectID: "entity-a", AccountID: "00123456789012345678", Quote: "hidden second claim",
		}, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	if _, err := NormalizeClaimProposal(proposal); err == nil {
		t.Fatal("claim payload accepted a field from another discriminated variant")
	}
}

func TestRenderedSubjectCannotDifferFromEvidenceEntity(t *testing.T) {
	period := NormalizedClaimPayload{StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z"}
	cases := map[ClaimType]NormalizedClaimPayload{
		ClaimAmount: {
			SubjectID: "entity-a", EntityID: "entity-b", AccountID: "00123456", AmountMinor: "100", Currency: "CNY",
			Direction: "out", StartAt: period.StartAt, EndAt: period.EndAt, Granularity: "transaction",
		},
		ClaimCount: {
			SubjectID: "entity-a", EntityID: "entity-b", Count: "1", StartAt: period.StartAt, EndAt: period.EndAt,
			Granularity: "transaction",
		},
		ClaimAccount:          {SubjectID: "entity-a", EntityID: "entity-b", AccountID: "00123456"},
		ClaimDirection:        {SubjectID: "entity-a", EntityID: "entity-b", Direction: "out"},
		ClaimDateRange:        {SubjectID: "entity-a", EntityID: "entity-b", StartAt: period.StartAt, EndAt: period.EndAt},
		ClaimRelationship:     {SubjectID: "entity-a", EntityID: "entity-b", CounterpartyID: "entity-c", RelationshipType: "ownership"},
		ClaimQuote:            {SubjectID: "entity-a", EntityID: "entity-b", Quote: "quoted evidence"},
		ClaimDeviceIdentifier: {SubjectID: "entity-a", EntityID: "entity-b", DeviceKind: "ip", DeviceIdentifier: "192.0.2.1"},
		ClaimLegalCharacterization: {
			SubjectID: "entity-a", EntityID: "entity-b", LegalCharacterization: "requires human review",
		},
	}
	for claimType, payload := range cases {
		t.Run(string(claimType), func(t *testing.T) {
			proposal := ClaimProposal{
				SchemaVersion: ClaimProposalVersion, ProposalID: "mismatched-rendered-subject", ClaimType: claimType,
				NormalizedPayload: payload, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
			}
			if _, err := NormalizeClaimProposal(proposal); err == nil {
				t.Fatalf("%s allowed rendered subject A to use evidence entity B", claimType)
			}
		})
	}
}
