package caseentity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
)

// This owner matrix uses synthetic source admission, not a native calculation.
// The separate delivery vector exercises actual DuckDB results and Provider IO.
func TestCompiledAccountIngressPreservesLongitudinalCurrentness(t *testing.T) {
	for _, name := range []string{"cold", "current", "updated", "corrected source", "revoked", "cross case", "hypothesis", "integrity failure"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := newCaseEntityMemoryStoreV1()
			indexedStore := &compiledIngressIndexFailureStoreV1{caseEntityMemoryStoreV1: store}
			harness := currentdatasettest.NewHarness()
			service := NewPersistentService(&recordingKeyedDigesterV1{key: []byte("synthetic-longitudinal")},
				indexedStore, harness, harness, harness.ValidateCurrent)
			input := caseEntityTestContextInputV1{ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a",
				UserID: "user-a", CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1}
			a := caseEntityTestContextV1(t, input)
			const account = "6222021234567890"
			const evidenceRef = "evidence-synthetic-a"
			claimDigest := domainsecurity.SHA256Hex([]byte("synthetic-claim-a"))
			if name != "cold" {
				ref, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(a,
					domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, account))
				if err != nil {
					t.Fatal(err)
				}
				if err := service.AppendCaseLongitudinalIngressV1(ctx, AppendCaseLongitudinalIngressInputV1{
					SecurityContext: a, References: []domaincaseentity.ReferenceV1{ref},
				}); err != nil {
					t.Fatal(err)
				}
				state := domaincaseentity.InvestigationConfirmedV1
				if name == "hypothesis" {
					state = domaincaseentity.InvestigationOpenV1
				}
				if err := service.AppendCaseLongitudinalOwnerStateV1(ctx, AppendCaseLongitudinalOwnerStateInputV1{
					SecurityContext: a,
					Evidence:        []CaseLongitudinalEvidenceDigestV1{{EvidenceReference: evidenceRef, EvidenceDigest: domainsecurity.SHA256Hex([]byte(evidenceRef))}},
					Claims: []CaseLongitudinalClaimDigestV1{{ClaimReference: "claim-synthetic-a", ClaimDigest: claimDigest,
						ClaimType: "amount", InvestigationState: state, EvidenceReferences: []string{evidenceRef}, CounterEvidenceReferences: []string{}}},
				}); err != nil {
					t.Fatal(err)
				}
			}
			input.ThreadID, input.TurnID = "thread-b", "turn-b"
			if name == "updated" || name == "corrected source" {
				input.SnapshotSeed, input.ContextEpoch = "snapshot-b", 2
			}
			if name == "cross case" {
				input.CaseID, input.CaseBindingHash, input.SnapshotSeed = "case-b", strings.Repeat("b", 64), "snapshot-other-case"
			}
			b := caseEntityTestContextV1(t, input)
			if name == "revoked" {
				harness.SetRevoked(true)
			}
			entityType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
			if name == "corrected source" {
				entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
			}
			if name == "integrity failure" {
				store.putIngressHook = func() { indexedStore.indexError = caseentityport.ErrIntegrity }
			}
			recordCount := len(store.threadContext)
			resolver := accountIngressBatchResolverFromCandidateV1(func(context.Context, domainsecurity.TurnSecurityContext, string) (AccountIngressCandidateResolutionV1, error) {
				return AccountIngressCandidateResolutionV1{Disposition: AccountIngressResolutionResolvedAccountV1,
					FinancialAccountType: entityType, BankInstitution: "中国银行", AccountType: "储蓄账户"}, nil
			})
			compiled, err := service.CompileAccountIngressV1(ctx, NewCompileAccountIngressInputV1(b,
				domaincaseentity.CaseIngressKindTurnV1, 0, "继续分析账号 "+account, resolver))
			if name == "integrity failure" {
				if !errors.Is(err, ErrPrivateStateIntegrity) || compiled.IsPersisted() || len(store.ingress) != 1 {
					t.Fatalf("index integrity failure was downgraded after private persistence: persisted=%t err=%v", compiled.IsPersisted(), err)
				}
				return
			}
			if name == "revoked" {
				if err == nil && compiled.IsPersisted() {
					t.Fatal("revoked current authority produced a consumable projection")
				}
				return
			}
			if err != nil || !compiled.IsPersisted() {
				t.Fatalf("compile: persisted=%t err=%v", compiled.IsPersisted(), err)
			}
			var selected ProviderIngressLongitudinalStateV1
			err = compiled.UseProviderProjectionV1(func(projection ProviderIngressProjectionV1) error {
				return projection.useExactV1(true, func(_ string, descriptors []providerIngressEntityDescriptorV1, state ProviderIngressLongitudinalStateV1) error {
					selected = state
					if len(descriptors) != 1 || descriptors[0].entityType != entityType ||
						descriptors[0].bankInstitution != "中国银行" || descriptors[0].accountType != "储蓄账户" {
						t.Error("current source resolution lost its typed descriptor")
					}
					return nil
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			if name == "cold" || name == "cross case" {
				if len(selected.Items) != 0 || len(store.threadContext) != recordCount {
					t.Fatal("missing case index borrowed state or created a new index")
				}
				return
			}
			found := false
			for _, item := range selected.Items {
				if item.Digest != claimDigest {
					continue
				}
				found = true
				want := ProviderIngressLongitudinalCurrentVerifiedFactV1
				if name == "updated" || name == "corrected source" {
					want = ProviderIngressLongitudinalHistoricalComparisonFactV1
					if item.Currentness == domaincaseentity.SnapshotCurrentV1 {
						t.Error("old snapshot claim remained current")
					}
				}
				if name == "hypothesis" {
					want = ProviderIngressLongitudinalEvidenceClaimReferenceV1
				}
				if item.Kind != want || item.EvidenceReferenceCount != 1 {
					t.Errorf("claim kind=%s want=%s evidence count=%d", item.Kind, want, item.EvidenceReferenceCount)
				}
			}
			if !found {
				t.Fatal("compiled raw ingress discarded A's selected claim metadata")
			}
			body, err := json.Marshal(selected)
			if err != nil || strings.Contains(string(body), account) || strings.Contains(string(body), evidenceRef) {
				t.Fatal("selection reflected private source or raw evidence handle")
			}
		})
	}
}

type compiledIngressIndexFailureStoreV1 struct {
	*caseEntityMemoryStoreV1
	indexError error
}

func (store *compiledIngressIndexFailureStoreV1) ResolveLatestCaseLongitudinalContext(ctx context.Context, scope domainsecurity.TurnSecurityContext) (domaincaseentity.ThreadCaseContextRecord, error) {
	if store.indexError != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, store.indexError
	}
	return store.caseEntityMemoryStoreV1.ResolveLatestCaseLongitudinalContext(ctx, scope)
}
