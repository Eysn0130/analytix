package evidence

import (
	"context"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestVerifyClaimLedgerForPublicationRequiresExactCurrentRegistryMembership(t *testing.T) {
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", EntityID: "entity-a", AccountID: "6222020202020202020"}
	issuer, input, proposal := claimEvidenceFixture(t, domainevidence.ClaimAccount, payload, "transactions", domainevidence.PaginationComplete)
	input.Material.PIIClassification = domainevidence.PIIControlled
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	proposal.EvidenceIDs = []string{receipt.ReceiptID}
	claim := forgedVerifiedClaim(t, proposal, receipt.ReceiptID, input.Material.QueryRange)
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: input.Context, EvidenceReceiptIDs: []string{receipt.ReceiptID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := issuer.Registry.(*memoryEvidenceRegistry).current(input.Context)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaimLedgerForPublication(context.Background(), registry, input.Context, ledger, true); err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaimLedgerForPublication(context.Background(), registry, input.Context, ledger, false); err == nil {
		t.Fatal("controlled evidence entered an ordinary publication ledger")
	}

	revoked, err := domainevidence.RevokeEvidenceReceipt(registry, receipt.ReceiptID, "test_revoked", evidenceIssuerTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaimLedgerForPublication(context.Background(), revoked, input.Context, ledger, true); err == nil {
		t.Fatal("revoked evidence retained publication authority")
	}
	other, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "other-thread", TurnID: "other-turn", WorkspaceRealPath: "/workspace/other", CaseID: "other-case",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")), ContextEpoch: 1, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaimLedgerForPublication(context.Background(), registry, other, ledger, true); err == nil {
		t.Fatal("claim ledger crossed its frozen case context")
	}
}
