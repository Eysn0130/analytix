package caseentity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	apploop "analytix.local/runtime-go/internal/app/loop"
	"analytix.local/runtime-go/internal/app/privacyprojection"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// The golden binds final model context bytes from the pre-Top32 full-sort
// implementation, exercised through the same public consumer/privacy seam.
// The independent full-sort owner oracle separately checks broad input shapes.
func TestLongitudinalFinalModelJSONMatchesFullSortV1(t *testing.T) {
	ctx := context.Background()
	store, service := openPersistentCaseEntityServiceV1(t, t.TempDir(), &recordingKeyedDigesterV1{key: []byte("top32-final-model-fixture")})
	defer store.Close()
	scope := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-top32", TurnID: "turn-top32", TenantID: "tenant-top32", UserID: "user-top32", CaseID: "case-top32",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "top32-model", ContextEpoch: 1,
	})
	account := "6222021234567890"
	ref, err := service.BindReferenceV1(ctx, caseentityapp.NewDeriveReferenceInputV1(scope, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, account))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.AppendCaseLongitudinalIngressV1(ctx, caseentityapp.AppendCaseLongitudinalIngressInputV1{SecurityContext: scope, References: []domaincaseentity.ReferenceV1{ref}}); err != nil {
		t.Fatal(err)
	}
	input := caseentityapp.AppendCaseLongitudinalOwnerStateInputV1{SecurityContext: scope, ContinuationDigest: domainsecurity.SHA256Hex([]byte("top32-continuation"))}
	for i := 0; i < 40; i++ {
		input.Evidence = append(input.Evidence, caseentityapp.CaseLongitudinalEvidenceDigestV1{
			EvidenceReference: "evr_" + domainsecurity.SHA256Hex([]byte(fmt.Sprintf("top32-reference-%d", i))), EvidenceDigest: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("top32-evidence-%d", i))),
		})
	}
	for i, kind := range []string{"entity", "relationship", "entity", "amount"} {
		claim := caseentityapp.CaseLongitudinalClaimDigestV1{ClaimReference: fmt.Sprintf("claim_top32_%d", i), ClaimDigest: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("top32-claim-%d", i))), ClaimType: kind,
			InvestigationState: domaincaseentity.InvestigationOpenV1, EvidenceReferences: []string{}, CounterEvidenceReferences: []string{}}
		if i == 0 {
			claim.InvestigationState = domaincaseentity.InvestigationConfirmedV1
			claim.EvidenceReferences = []string{input.Evidence[0].EvidenceReference}
		}
		if i == 2 {
			claim.InvestigationState = domaincaseentity.InvestigationRejectedV1
			claim.CounterEvidenceReferences = []string{input.Evidence[1].EvidenceReference}
		}
		input.Claims = append(input.Claims, claim)
	}
	if err = service.AppendCaseLongitudinalOwnerStateV1(ctx, input); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = service.UseCaseLongitudinalAliasesV1(ctx, caseentityapp.UseCaseLongitudinalAliasesInputV1{
		SecurityContext: scope, Aliases: []domaincaseentity.ModelEntityAliasV1{"acct:1"}, ProviderText: "分析账号 acct:1 的流水",
	}, func(selection caseentityapp.CaseLongitudinalAliasSelectionV1) error {
		calls++
		consumed, err := apploop.ConsumeInitialProviderIngressWithAliasesV1(&selection.ProviderProjection, "")
		if err != nil {
			return err
		}
		message := domainmodel.Message{Role: "user", Content: consumed.Text}
		if err = privacyprojection.BindProviderCaseAliasesToMessageV1(scope, consumed.Aliases, &message); err != nil {
			return err
		}
		projected, err := privacyprojection.ProjectProviderRequestForEffect(scope, domainmodel.Request{Messages: []domainmodel.Message{message}}, false)
		if err != nil {
			return err
		}
		content := projected.Messages[0].Content
		const start = "<analytix_host_verified_case_entity_semantics>\n"
		const end = "\n</analytix_host_verified_case_entity_semantics>"
		if strings.Contains(content, account) || strings.Contains(content, "evr_") {
			t.Fatal("final model context contains private identity")
		}
		_, body, ok := strings.Cut(content, start)
		if !ok {
			t.Fatal("final semantic envelope missing")
		}
		raw, _, ok := strings.Cut(body, end)
		if !ok {
			t.Fatal("final semantic envelope incomplete")
		}
		var envelope struct {
			Longitudinal struct {
				Items        []json.RawMessage `json:"items"`
				OmittedTotal uint32            `json:"omittedTotal"`
			} `json:"longitudinal"`
		}
		if err = json.Unmarshal([]byte(raw), &envelope); err != nil || len(envelope.Longitudinal.Items) != 32 || envelope.Longitudinal.OmittedTotal != 13 {
			t.Fatal("final context lost exact selection coverage", err)
		}
		const fullSortJSONSHA256 = "0198dd82efa46ee0f60ffc879f554d5cce11f44a93e5867accab4a6220d13ce0"
		if digest := domainsecurity.SHA256Hex([]byte(raw)); digest != fullSortJSONSHA256 {
			t.Fatalf("final model JSON differs from frozen full-sort baseline: sha256=%s", digest)
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("public semantic/privacy consumer failed: calls=%d err=%v", calls, err)
	}
}
