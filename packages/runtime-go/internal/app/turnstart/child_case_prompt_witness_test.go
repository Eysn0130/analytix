package turnstart

import (
	"context"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCopiedHostChildCasePromptWitnessCannotRepeatConsumer(t *testing.T) {
	fixture := newChildAuthorityFixture(t, domainsecurity.RiskClassCase)
	record := fixture.record
	delegation, err := domainjob.NewCaseDelegationContextV1(record.SecurityBinding, domainjob.CaseDelegationSemanticContextV1{
		TaskKind: domainjob.CaseDelegationTaskKindV1, Currentness: domaincaseentity.SnapshotCurrentV1,
		Entities: []domainjob.CaseDelegatedEntitySemanticV1{{Alias: "acct:1", EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, ResolutionDigest: strings.Repeat("a", 64)}},
		Claims:   []domainjob.CaseDelegatedClaimReferenceV1{}, Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{}, Continuations: []domainjob.CaseDelegatedContinuationReferenceV1{},
	})
	if err != nil {
		t.Fatal(err)
	}
	record.CaseDelegation = delegation
	record.Name, record.Label = domainjob.CaseDelegationNameV1, domainjob.CaseDelegationLabelV1
	record.Prompt, err = domainjob.CaseDelegationProviderPromptV1(delegation, record.SecurityBinding)
	if err != nil {
		t.Fatal(err)
	}
	authority := resolveChildAuthorityForTest(t, record, childAuthorityNoBlocker)
	store := &childAuthorityStoreStub{loaded: record, validated: record}
	if _, witness, err := NewHostChildCasePromptFrozenWitnessV1(authority, fixture.frozen, fixture.frozen.TurnID, record.Prompt); err == nil || witness != nil {
		t.Fatal("unvalidated record acquired frozen witness")
	}
	if err := ValidateChildTransitionFrozen(context.Background(), authority, store, childAuthorityNoBlocker, fixture.frozen); err != nil {
		t.Fatal(err)
	}
	copyAuthority := *authority
	_, witness, err := NewHostChildCasePromptFrozenWitnessV1(authority, fixture.frozen, fixture.frozen.TurnID, record.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, again, err := NewHostChildCasePromptFrozenWitnessV1(&copyAuthority, fixture.frozen, fixture.frozen.TurnID, record.Prompt); err == nil || again != nil {
		t.Fatal("copied frozen authority reissued witness")
	}
	if err := ValidateChildTransitionFrozen(context.Background(), authority, store, childAuthorityNoBlocker, fixture.frozen); err == nil {
		t.Fatal("revalidation reset consumed frozen authority")
	}
	copied := *witness
	uses := 0
	use := func(string, domainsecurity.TurnSecurityContext, domainjob.Record, string, string, string, *domainjob.SecurityBinding, *domainjob.CaseDelegationContextV1) error {
		uses++
		return nil
	}
	if err := witness.UseCurrentAttemptV1(use); err != nil {
		t.Fatal(err)
	}
	if err := copied.UseCurrentAttemptV1(use); err == nil || uses != 1 {
		t.Fatal("copied private witness repeated the same provider-attempt consumer")
	}
}
