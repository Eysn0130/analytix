package caseentity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainprivacyprojection "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

var _ KeyedPayloadDigester = (*pendingworkapp.Service)(nil)

func TestReferenceV1IsStableAcrossTurnThreadAndSnapshotEvolution(t *testing.T) {
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-a")}
	service := NewService(digester)
	bindingHash := strings.Repeat("a", 64)
	firstContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-first", TurnID: "turn-first", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: bindingHash, SnapshotSeed: "snapshot-a", ContextEpoch: 7,
	})
	secondContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-after-restart", TurnID: "turn-after-restart", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: bindingHash, SnapshotSeed: "snapshot-b", ContextEpoch: 8,
	})

	first, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
		firstContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222-0212 3456-7890",
	))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
		secondContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same case entity drifted across turn/snapshot evolution: first=%q second=%q", first, second)
	}
	if err := domaincaseentity.ValidateReferenceV1(string(first)); err != nil {
		t.Fatal(err)
	}

	if len(digester.calls) != 2 {
		t.Fatalf("unexpected keyed digester call count: %d", len(digester.calls))
	}
	last := digester.calls[len(digester.calls)-1]
	if last.purpose != referenceDigestPurposeV1 {
		t.Fatalf("reference used the wrong keyed purpose: %q", last.purpose)
	}
	wantPayload := `{"canonicalValue":"6222021234567890","canonicalizer":"analytix.bank-account-text/v2","caseBindingHash":"` +
		bindingHash + `","caseId":"case-a","entityType":"bank_account_number","tenantId":"tenant-a","userId":"user-a"}`
	if string(last.payload) != wantPayload {
		t.Fatal("reference canonical payload did not match the closed identity fields")
	}
	for _, excluded := range []string{
		secondContext.ThreadID,
		secondContext.TurnID,
		secondContext.DatasetSnapshotID,
		secondContext.SourceManifestHash,
		secondContext.ContextDigest,
		secondContext.IssuedAt,
	} {
		if strings.Contains(string(last.payload), excluded) {
			t.Fatalf("reference payload included snapshot/turn authority field %q", excluded)
		}
	}
	if strings.Contains(string(last.payload), `"contextEpoch"`) {
		t.Fatal("reference payload included context epoch")
	}
}

func TestReferenceV1SeparatesCaseBindingTenantUserTypeAndInstallationKey(t *testing.T) {
	baseInput := caseEntityTestContextInputV1{
		ThreadID: "thread-base", TurnID: "turn-base", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	}
	baseContext := caseEntityTestContextV1(t, baseInput)
	serviceA := NewService(&recordingKeyedDigesterV1{key: []byte("installation-key-a")})
	base := deriveCaseEntityReferenceTestV1(
		t, serviceA, baseContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
	)

	tests := map[string]struct {
		context     domainsecurity.TurnSecurityContext
		accountType string
		service     *Service
	}{
		"case ID": {
			context: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-case-b", TurnID: "turn-case-b", TenantID: "tenant-a", UserID: "user-a",
				CaseID: "case-b", CaseBindingHash: baseInput.CaseBindingHash, SnapshotSeed: "snapshot-a", ContextEpoch: 1,
			}),
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			service:     serviceA,
		},
		"case binding": {
			context: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-binding-b", TurnID: "turn-binding-b", TenantID: "tenant-a", UserID: "user-a",
				CaseID: "case-a", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
			}),
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			service:     serviceA,
		},
		"tenant": {
			context: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-tenant-b", TurnID: "turn-tenant-b", TenantID: "tenant-b", UserID: "user-a",
				CaseID: "case-a", CaseBindingHash: baseInput.CaseBindingHash, SnapshotSeed: "snapshot-a", ContextEpoch: 1,
			}),
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			service:     serviceA,
		},
		"user": {
			context: caseEntityTestContextV1(t, caseEntityTestContextInputV1{
				ThreadID: "thread-user-b", TurnID: "turn-user-b", TenantID: "tenant-a", UserID: "user-b",
				CaseID: "case-a", CaseBindingHash: baseInput.CaseBindingHash, SnapshotSeed: "snapshot-a", ContextEpoch: 1,
			}),
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			service:     serviceA,
		},
		"entity type": {
			context:     baseContext,
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
			service:     serviceA,
		},
		"installation key": {
			context:     baseContext,
			accountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			service:     NewService(&recordingKeyedDigesterV1{key: []byte("installation-key-b")}),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := deriveCaseEntityReferenceTestV1(t, test.service, test.context, test.accountType)
			if got == base {
				t.Fatalf("%s did not domain-separate the stable reference: %q", name, got)
			}
		})
	}
}

func TestReferenceV1RejectsInvalidExactValuesWithoutEcho(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-invalid", TurnID: "turn-invalid", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	service := NewService(&recordingKeyedDigesterV1{key: []byte("installation-key-a")})
	for name, sourceExactValue := range map[string]string{
		"full-width":       "６２２２０２１２３４５６７８９０",
		"non-breaking":     "6222\u00a0021234567890",
		"double separator": "6222--021234567890",
		"leading":          " 6222021234567890",
		"scientific":       "6.22202123456789e15",
	} {
		t.Run(name, func(t *testing.T) {
			reference, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
				securityContext,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				sourceExactValue,
			))
			if !errors.Is(err, ErrInvalidReferenceInput) || reference != "" {
				t.Fatalf("invalid exact value minted a reference: reference=%q err=%v", reference, err)
			}
			if strings.Contains(err.Error(), sourceExactValue) {
				t.Fatal("reference error exposed the source-exact value")
			}
		})
	}

	generalContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general", TurnID: "turn-general", WorkspaceRealPath: "/workspace/general",
		TenantID: "tenant-a", UserID: "user-a", ContextEpoch: 1, IssuedAt: time.Unix(1_700_000_100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reference, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
		generalContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890",
	)); !errors.Is(err, ErrInvalidReferenceInput) || reference != "" {
		t.Fatalf("general context minted a case reference: reference=%q err=%v", reference, err)
	}
}

func TestPrivateApplicationInputsRejectOrdinarySerializationAndFormatting(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-a", TurnID: "turn-a", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	const exact = "6222021234567890"
	reference, err := NewService(&recordingKeyedDigesterV1{key: []byte("installation-key-a")}).DeriveReferenceV1(
		context.Background(),
		NewDeriveReferenceInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			exact,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{
		NewDeriveReferenceInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			exact,
		),
		NewResolveReferenceInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			reference,
			[]string{exact},
		),
		newPersistIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"分析 "+exact,
			"分析 "+string(reference),
			nil,
		),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"分析 "+exact,
			nil,
		),
		newProviderIngressProjectionV1("分析 acct:1"),
	}
	for _, value := range values {
		body, marshalErr := json.Marshal(value)
		if marshalErr == nil || strings.Contains(string(body), exact) ||
			strings.Contains(marshalErr.Error(), exact) {
			t.Fatalf("ordinary JSON exposed private application input: body=%q err=%v", body, marshalErr)
		}
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, exact) || !strings.Contains(formatted, "[REDACTED]") {
				t.Fatalf("ordinary format %q exposed private application input: %q", format, formatted)
			}
		}
	}
	type deriveInputDefinedV1 DeriveReferenceInputV1
	type resolveInputDefinedV1 ResolveReferenceInputV1
	type ingressInputDefinedV1 PersistIngressInputV1
	type compileIngressInputDefinedV1 CompileAccountIngressInputV1
	type providerProjectionDefinedV1 ProviderIngressProjectionV1
	for name, value := range map[string]any{
		"derive":              deriveInputDefinedV1(values[0].(DeriveReferenceInputV1)),
		"resolve":             resolveInputDefinedV1(values[1].(ResolveReferenceInputV1)),
		"ingress":             ingressInputDefinedV1(values[2].(PersistIngressInputV1)),
		"compile ingress":     compileIngressInputDefinedV1(values[3].(CompileAccountIngressInputV1)),
		"provider projection": providerProjectionDefinedV1(values[4].(ProviderIngressProjectionV1)),
	} {
		body, marshalErr := json.Marshal(value)
		formatted := fmt.Sprintf("%#v", value)
		if marshalErr != nil || strings.Contains(string(body), exact) || strings.Contains(formatted, exact) {
			t.Fatalf("defined-type %s exposed private application input: body=%q format=%q err=%v", name, body, formatted, marshalErr)
		}
	}
}

func TestProviderIngressProjectionMapsCallbackErrorsWithoutReferenceEcho(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	privateText := "核查账号 acct:1"
	projection := newProviderIngressProjectionV1(privateText)
	err = projection.UseExactV1(func(current string) error {
		return errors.New("untrusted callback echoed " + current)
	})
	if !errors.Is(err, ErrPrivateStateUnavailable) ||
		strings.Contains(err.Error(), string(reference)) ||
		strings.Contains(err.Error(), privateText) {
		t.Fatalf("provider projection callback error crossed the private boundary: %v", err)
	}
	if secondErr := projection.UseExactV1(func(string) error { return nil }); !errors.Is(secondErr, ErrPrivateStateIntegrity) {
		t.Fatalf("provider projection carrier was reusable: %v", secondErr)
	}
}

func TestProviderIngressProjectionDescriptorsAreOneUseCallbackScopedAndErrorClosed(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	descriptor := providerIngressEntityDescriptorV1{
		reference:            reference,
		alias:                domaincaseentity.ModelEntityAliasV1("acct:1"),
		entityType:           domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		financialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		bankInstitution:      "中国银行",
		accountType:          "储蓄账户",
	}
	projection := newProviderIngressProjectionV1("核查账号 acct:1", descriptor)
	var retained ProviderIngressDescriptorsUseV1
	descriptorCalls := 0
	err = projection.UseExactWithDescriptorsV1(func(
		text string,
		count uint32,
		useDescriptors ProviderIngressDescriptorsUseV1,
	) error {
		if text != "核查账号 acct:1" || count != 1 {
			return errors.New("provider descriptor envelope drifted")
		}
		retained = useDescriptors
		return useDescriptors(func(
			ordinal uint32,
			currentAlias domaincaseentity.ModelEntityAliasV1,
			entityType string,
			financialAccountType string,
			bankInstitution string,
			accountType string,
		) error {
			descriptorCalls++
			if ordinal != 0 || currentAlias != descriptor.alias ||
				entityType != descriptor.entityType ||
				financialAccountType != descriptor.financialAccountType ||
				bankInstitution != descriptor.bankInstitution || accountType != descriptor.accountType {
				return errors.New("provider descriptor fields drifted")
			}
			return nil
		})
	})
	if err != nil || descriptorCalls != 1 {
		t.Fatalf("provider descriptor exact use failed: calls=%d err=%v", descriptorCalls, err)
	}
	if retained == nil {
		t.Fatal("provider descriptor use callback was not observed")
	}
	if retainedErr := retained(func(
		uint32,
		domaincaseentity.ModelEntityAliasV1,
		string,
		string,
		string,
		string,
	) error {
		return nil
	}); !errors.Is(retainedErr, ErrPrivateStateUnavailable) {
		t.Fatalf("retained provider descriptor callback survived its boundary: %v", retainedErr)
	}
	if secondErr := projection.UseExactV1(func(string) error { return nil }); !errors.Is(secondErr, ErrPrivateStateIntegrity) {
		t.Fatalf("descriptor-aware provider projection was reusable: %v", secondErr)
	}

	errorProjection := newProviderIngressProjectionV1("核查账号 acct:1", descriptor)
	err = errorProjection.UseExactWithDescriptorsV1(func(
		_ string,
		_ uint32,
		useDescriptors ProviderIngressDescriptorsUseV1,
	) error {
		return useDescriptors(func(
			uint32,
			domaincaseentity.ModelEntityAliasV1,
			string,
			string,
			string,
			string,
		) error {
			return errors.New("untrusted callback echoed " + string(reference))
		})
	})
	if !errors.Is(err, ErrPrivateStateUnavailable) || strings.Contains(err.Error(), string(reference)) {
		t.Fatalf("provider descriptor callback error crossed the private boundary: %v", err)
	}

	ignoredProjection := newProviderIngressProjectionV1("核查账号 acct:1", descriptor)
	err = ignoredProjection.UseExactWithDescriptorsV1(func(
		string,
		uint32,
		ProviderIngressDescriptorsUseV1,
	) error {
		return nil
	})
	if !errors.Is(err, ErrPrivateStateUnavailable) {
		t.Fatalf("descriptor-aware projection accepted an ignored descriptor set: %v", err)
	}

	incompleteProjection := newProviderIngressProjectionV1(
		"比较账号 acct:1 与 "+string(reference),
		descriptor,
	)
	err = incompleteProjection.UseExactWithDescriptorsV1(func(
		_ string,
		_ uint32,
		useDescriptors ProviderIngressDescriptorsUseV1,
	) error {
		return useDescriptors(func(
			uint32,
			domaincaseentity.ModelEntityAliasV1,
			string,
			string,
			string,
			string,
		) error {
			return nil
		})
	})
	if !errors.Is(err, ErrPrivateStateIntegrity) {
		t.Fatalf("provider projection accepted a referenced entity without a descriptor: %v", err)
	}
}

func TestCompileAccountIngressPersistsPrivateRawAndReturnsStableProviderProjection(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-ingress", TurnID: "turn-ingress", WorkspaceRealPath: "/cases/ingress",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash:   strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-ingress"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("manifest:snapshot-ingress"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_010, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newCaseEntityMemoryStoreV1()
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
		harness,
		harness,
		harness.ValidateCurrent,
	)
	const (
		firstExact  = "6222-0212 3456-7890"
		secondExact = "6222021234567890"
		thirdExact  = "6217-0098 7654-3210"
	)
	rawText := "核查以下对象：\n账号：" + firstExact + "\n账户：" + secondExact + "\n另一账号：" + thirdExact
	input := NewCompileAccountIngressInputV1(
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		rawText,
		accountIngressBatchResolverFromCandidateV1(func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			canonical string,
		) (AccountIngressCandidateResolutionV1, error) {
			resolution := AccountIngressCandidateResolutionV1{
				Disposition:          AccountIngressResolutionResolvedAccountV1,
				FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}
			switch canonical {
			case secondExact:
				resolution.BankInstitution = "中国银行"
				resolution.AccountType = "储蓄账户"
			case "6217009876543210":
				resolution.BankInstitution = "中国工商银行"
				resolution.AccountType = "结算账户"
			default:
				return AccountIngressCandidateResolutionV1{}, errors.New("unexpected account candidate")
			}
			return resolution, nil
		}),
	)
	compiled, err := service.CompileAccountIngressV1(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	compiledProviderText, descriptors := useAccountIngressProviderProjectionWithDescriptorsV1(t, compiled)
	if !compiled.IsPersisted() || compiled.IsNoop() ||
		compiled.SpanCount != 3 || compiled.UniqueReferenceCount != 2 ||
		compiledProviderText == "" || compiled.PublicText == "" || len(descriptors) != 2 {
		t.Fatalf("compiled ingress metadata mismatch: %#v", compiled)
	}
	privateReference := useAccountIngressPrivateReferenceV1(t, compiled)
	if !domainsecurity.IsSHA256Hex(privateReference.RecordID) ||
		!domainsecurity.IsSHA256Hex(privateReference.RecordDigest) {
		t.Fatal("compiled ingress private handle is invalid")
	}
	if secondUseErr := compiled.UsePrivateRecordReferenceV1(func(PrivateRecordReferenceV1) error { return nil }); !errors.Is(secondUseErr, ErrPrivateStateUnavailable) {
		t.Fatalf("private handle was reusable: %v", secondUseErr)
	}
	compiledJSON, compiledJSONErr := json.Marshal(compiled)
	referenceJSON, referenceJSONErr := json.Marshal(privateReference)
	if compiledJSONErr == nil || referenceJSONErr == nil || len(compiledJSON) != 0 || len(referenceJSON) != 0 ||
		strings.Contains(compiledJSONErr.Error(), privateReference.RecordDigest) ||
		strings.Contains(referenceJSONErr.Error(), privateReference.RecordDigest) {
		t.Fatal("private ingress identity supported ordinary JSON or exposed its digest")
	}
	for _, forbidden := range []string{firstExact, secondExact, thirdExact, "[ACCOUNT]"} {
		if strings.Contains(compiledProviderText, forbidden) {
			t.Fatalf("provider projection retained forbidden account material %q", forbidden)
		}
	}
	if strings.Contains(compiledProviderText, "cer1_") ||
		strings.Count(compiledProviderText, "acct:1") != 2 ||
		strings.Count(compiledProviderText, "acct:2") != 1 ||
		strings.Contains(compiled.PublicText, "cer1_") ||
		strings.Count(compiled.PublicText, "[ACCOUNT]") != 3 ||
		strings.Contains(compiled.PublicText, "中国银行") ||
		strings.Contains(compiled.PublicText, "中国工商银行") ||
		strings.Contains(compiled.PublicText, "储蓄账户") ||
		strings.Contains(compiled.PublicText, "结算账户") {
		t.Fatalf("provider/public projection split is invalid")
	}
	if len(store.bindings) != 2 || len(store.ingress) != 1 {
		t.Fatalf("private persistence mismatch: bindings=%d ingress=%d", len(store.bindings), len(store.ingress))
	}

	record, found := store.ingress[privateReference.RecordID]
	if !found || record.RecordDigest != privateReference.RecordDigest || len(record.Spans) != 3 {
		t.Fatalf("persisted ingress mismatch: found=%t record=%#v", found, record)
	}
	if record.Spans[0].Reference != record.Spans[1].Reference ||
		record.Spans[0].Reference == record.Spans[2].Reference {
		t.Fatal("canonical duplicate or distinct-account reference identity drifted")
	}
	if descriptors[0].Ordinal != 0 || descriptors[1].Ordinal != 1 ||
		descriptors[0].Alias != domaincaseentity.ModelEntityAliasV1("acct:1") ||
		descriptors[1].Alias != domaincaseentity.ModelEntityAliasV1("acct:2") ||
		descriptors[0].EntityType != domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		descriptors[0].FinancialAccountType != descriptors[0].EntityType ||
		descriptors[0].BankInstitution != "中国银行" || descriptors[0].AccountType != "储蓄账户" ||
		descriptors[1].EntityType != domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		descriptors[1].FinancialAccountType != descriptors[1].EntityType ||
		descriptors[1].BankInstitution != "中国工商银行" || descriptors[1].AccountType != "结算账户" {
		t.Fatalf("provider-safe ingress descriptors drifted: %#v", descriptors)
	}
	for _, descriptor := range descriptors {
		for _, forbidden := range []string{firstExact, secondExact, thirdExact, "6217009876543210"} {
			if strings.Contains(descriptor.EntityType, forbidden) ||
				strings.Contains(descriptor.FinancialAccountType, forbidden) ||
				strings.Contains(descriptor.BankInstitution, forbidden) ||
				strings.Contains(descriptor.AccountType, forbidden) {
				t.Fatalf("provider descriptor retained source-exact account material %q: %#v", forbidden, descriptor)
			}
		}
	}
	for _, span := range record.Spans {
		if span.EntityType != domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
			record.ProjectedText[span.ProjectedStartByte:span.ProjectedEndByte] != string(span.Reference) {
			t.Fatalf("persisted projected offsets are not exact: %#v", span)
		}
	}
	var restoredRaw string
	if err := service.UseIngressPrivateV1(
		ctx,
		securityContext,
		privateReference.RecordID,
		func(restored domaincaseentity.CaseIngressRecord, raw string) error {
			restoredRaw = raw
			if restored.RecordDigest != privateReference.RecordDigest {
				t.Fatal("private restoration returned a different record")
			}
			return nil
		},
	); err != nil || restoredRaw != rawText {
		t.Fatalf("private ingress restoration mismatch: err=%v", err)
	}
	providerUseCalls := 0
	if err := service.UseIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		func(projection ProviderIngressProjectionV1) error {
			return projection.UseExactV1(func(providerText string) error {
				providerUseCalls++
				if providerText != compiledProviderText {
					t.Fatalf("provider-only ingress restoration drifted: got=%q want=%q", providerText, compiledProviderText)
				}
				return nil
			})
		},
	); err != nil || providerUseCalls != 1 {
		t.Fatalf("provider-only ingress restoration mismatch: calls=%d err=%v", providerUseCalls, err)
	}
	harness.SetRevoked(true)
	revokedUseCalls := 0
	revokedErr := service.UseIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		func(ProviderIngressProjectionV1) error {
			revokedUseCalls++
			return nil
		},
	)
	if revokedErr == nil || revokedUseCalls != 0 {
		t.Fatalf("revoked dataset authority reached provider projection: calls=%d err=%v", revokedUseCalls, revokedErr)
	}
	harness.SetRevoked(false)
	restoredUseCalls := 0
	if err := service.UseIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		func(projection ProviderIngressProjectionV1) error {
			return projection.UseExactV1(func(string) error {
				restoredUseCalls++
				return nil
			})
		},
	); err != nil || restoredUseCalls != 1 {
		t.Fatalf("reauthorized dataset authority did not restore provider projection: calls=%d err=%v", restoredUseCalls, err)
	}
	newIngressUseContext := func(
		turnID string,
		snapshotSeed string,
		manifestSeed string,
		epoch uint64,
	) domainsecurity.TurnSecurityContext {
		t.Helper()
		candidate, contextErr := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-ingress", TurnID: turnID, WorkspaceRealPath: "/cases/ingress",
			TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
			CaseBindingHash:   strings.Repeat("a", 64),
			DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(snapshotSeed),
			SourceManifestHash: domainsecurity.SHA256Hex(
				[]byte("manifest:" + manifestSeed),
			),
			ContextEpoch: epoch,
			IssuedAt:     time.Unix(1_700_000_010, 0).UTC(),
		})
		if contextErr != nil {
			t.Fatal(contextErr)
		}
		return candidate
	}
	for _, test := range []struct {
		name            string
		securityContext domainsecurity.TurnSecurityContext
	}{
		{
			name: "new turn",
			securityContext: newIngressUseContext(
				"turn-ingress-next", "snapshot-ingress", "snapshot-ingress", 1,
			),
		},
		{
			name: "new context digest",
			securityContext: newIngressUseContext(
				"turn-ingress", "snapshot-ingress", "manifest-ingress-next", 1,
			),
		},
		{
			name: "new snapshot",
			securityContext: newIngressUseContext(
				"turn-ingress", "snapshot-ingress-next", "snapshot-ingress", 1,
			),
		},
		{
			name: "new epoch",
			securityContext: newIngressUseContext(
				"turn-ingress", "snapshot-ingress", "snapshot-ingress", 2,
			),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			useCalls := 0
			err := service.UseIngressPrivateV1(
				ctx,
				test.securityContext,
				privateReference.RecordID,
				func(domaincaseentity.CaseIngressRecord, string) error {
					useCalls++
					return nil
				},
			)
			if !errors.Is(err, ErrPrivateStateIntegrity) || useCalls != 0 {
				t.Fatalf("non-exact ingress context reached private callback: calls=%d err=%v", useCalls, err)
			}
			providerUseCalls := 0
			providerErr := service.UseIngressProviderProjectionV1(
				ctx,
				test.securityContext,
				domaincaseentity.CaseIngressKindTurnV1,
				0,
				func(ProviderIngressProjectionV1) error {
					providerUseCalls++
					return nil
				},
			)
			if providerErr == nil || providerUseCalls != 0 {
				t.Fatalf("non-exact provider ingress context reached callback: calls=%d err=%v", providerUseCalls, providerErr)
			}
		})
	}

	assertAccountIngressValuesDoNotLeakV1(
		t,
		[]string{firstExact, secondExact, thirdExact},
		compiled,
		record,
	)
	body, marshalErr := json.Marshal(input)
	if marshalErr == nil || len(body) != 0 {
		t.Fatalf("private compiler input supported ordinary JSON: body=%q err=%v", body, marshalErr)
	}
	assertTextDoesNotContainAnyV1(t, marshalErr.Error(), firstExact, secondExact, thirdExact)
}

func TestUseRecompiledIngressProviderProjectionRestoresDescriptorsWithoutNestedLeaseAndExpires(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-recompile", TurnID: "turn-recompile", WorkspaceRealPath: "/cases/recompile",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash:   strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-recompile"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("manifest:snapshot-recompile"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_020, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newCaseEntityMemoryStoreV1()
	trackedAuthority := &trackingCurrentDatasetAuthorityV1{delegate: harness}
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
		trackedAuthority,
		harness,
		harness.ValidateCurrent,
	)
	const (
		firstExact  = "6222021234567890123"
		secondExact = "6217009876543210987"
	)
	rawText := "分析账号 " + firstExact + "，再比较账户 " + secondExact + " 与 " + firstExact
	var resolverCalls atomic.Uint32
	resolver := accountIngressBatchResolverFromCandidateV1(func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
		canonical string,
	) (AccountIngressCandidateResolutionV1, error) {
		if trackedAuthority.active.Load() {
			return AccountIngressCandidateResolutionV1{}, errors.New("resolver entered under private-state lease")
		}
		resolverCalls.Add(1)
		resolution := AccountIngressCandidateResolutionV1{
			Disposition:          AccountIngressResolutionResolvedAccountV1,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		}
		switch canonical {
		case firstExact:
			resolution.BankInstitution = "中国银行"
			resolution.AccountType = "结算账户"
		case secondExact:
			resolution.BankInstitution = "中国工商银行"
			resolution.AccountType = "储蓄账户"
		default:
			return AccountIngressCandidateResolutionV1{}, errors.New("unexpected replay candidate")
		}
		return resolution, nil
	})
	compiled, err := service.CompileAccountIngressV1(ctx, NewCompileAccountIngressInputV1(
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		4,
		rawText,
		resolver,
	))
	if err != nil || !compiled.IsPersisted() {
		t.Fatalf("initial ingress compile failed: result=%s err=%v", compiled, err)
	}
	initialProviderText, initialDescriptors := useAccountIngressProviderProjectionWithDescriptorsV1(t, compiled)
	initialPrivateReference := useAccountIngressPrivateReferenceV1(t, compiled)

	var retainedProjection ProviderIngressProjectionV1
	var retainedDescriptors ProviderIngressDescriptorsUseV1
	consume := func(
		projection ProviderIngressProjectionV1,
	) (string, []providerIngressDescriptorTestV1, error) {
		retainedProjection = projection
		if textOnlyErr := projection.UseExactV1(func(string) error { return nil }); !errors.Is(textOnlyErr, ErrPrivateStateUnavailable) {
			return "", nil, errors.New("recompiled projection allowed text-only consumption")
		}
		var providerText string
		descriptors := []providerIngressDescriptorTestV1{}
		err := projection.UseExactWithDescriptorsV1(func(
			text string,
			descriptorCount uint32,
			useDescriptors ProviderIngressDescriptorsUseV1,
		) error {
			providerText = text
			retainedDescriptors = useDescriptors
			if descriptorCount != 2 {
				return errors.New("recompiled descriptor count drifted")
			}
			return useDescriptors(func(
				ordinal uint32,
				alias domaincaseentity.ModelEntityAliasV1,
				entityType string,
				financialAccountType string,
				bankInstitution string,
				accountType string,
			) error {
				descriptors = append(descriptors, providerIngressDescriptorTestV1{
					Ordinal: ordinal, Alias: alias, EntityType: entityType,
					FinancialAccountType: financialAccountType,
					BankInstitution:      bankInstitution,
					AccountType:          accountType,
				})
				return nil
			})
		})
		return providerText, descriptors, err
	}

	var firstReplayText string
	var firstReplayDescriptors []providerIngressDescriptorTestV1
	err = service.UseRecompiledIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		4,
		resolver,
		func(projection ProviderIngressProjectionV1) error {
			var consumeErr error
			firstReplayText, firstReplayDescriptors, consumeErr = consume(projection)
			return consumeErr
		},
	)
	if err != nil || firstReplayText != initialProviderText ||
		len(firstReplayDescriptors) != len(initialDescriptors) {
		t.Fatalf("recompiled projection mismatch: text=%q descriptors=%#v err=%v", firstReplayText, firstReplayDescriptors, err)
	}
	for index := range initialDescriptors {
		if firstReplayDescriptors[index] != initialDescriptors[index] {
			t.Fatalf("recompiled descriptor %d drifted: got=%#v want=%#v", index, firstReplayDescriptors[index], initialDescriptors[index])
		}
	}
	if retainedErr := retainedProjection.UseExactWithDescriptorsV1(func(
		string,
		uint32,
		ProviderIngressDescriptorsUseV1,
	) error {
		return nil
	}); !errors.Is(retainedErr, ErrPrivateStateUnavailable) {
		t.Fatalf("recompiled projection survived its callback: %v", retainedErr)
	}
	if retainedDescriptors == nil {
		t.Fatal("recompiled descriptor callback was not observed")
	}
	if retainedErr := retainedDescriptors(func(
		uint32,
		domaincaseentity.ModelEntityAliasV1,
		string,
		string,
		string,
		string,
	) error {
		return nil
	}); !errors.Is(retainedErr, ErrPrivateStateUnavailable) {
		t.Fatalf("recompiled descriptor callback survived its boundary: %v", retainedErr)
	}

	var replayText string
	var replayDescriptors []providerIngressDescriptorTestV1
	err = service.UseRecompiledIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		4,
		resolver,
		func(projection ProviderIngressProjectionV1) error {
			var consumeErr error
			replayText, replayDescriptors, consumeErr = consume(projection)
			return consumeErr
		},
	)
	if err != nil || replayText != firstReplayText || len(replayDescriptors) != 2 ||
		replayDescriptors[0] != firstReplayDescriptors[0] ||
		replayDescriptors[1] != firstReplayDescriptors[1] {
		t.Fatalf("exact replay was not stable: text=%q descriptors=%#v err=%v", replayText, replayDescriptors, err)
	}
	if len(store.bindings) != 2 || len(store.ingress) != 1 ||
		store.ingress[initialPrivateReference.RecordID].RecordDigest != initialPrivateReference.RecordDigest {
		t.Fatalf("recompile changed immutable private state: bindings=%d ingress=%d", len(store.bindings), len(store.ingress))
	}

	leakErr := service.UseRecompiledIngressProviderProjectionV1(
		ctx,
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		4,
		resolver,
		func(projection ProviderIngressProjectionV1) error {
			return projection.UseExactWithDescriptorsV1(func(
				_ string,
				_ uint32,
				useDescriptors ProviderIngressDescriptorsUseV1,
			) error {
				return useDescriptors(func(
					uint32,
					domaincaseentity.ModelEntityAliasV1,
					string,
					string,
					string,
					string,
				) error {
					return errors.New("untrusted callback echoed " + rawText)
				})
			})
		},
	)
	if !errors.Is(leakErr, ErrPrivateStateUnavailable) ||
		strings.Contains(leakErr.Error(), firstExact) || strings.Contains(leakErr.Error(), secondExact) ||
		strings.Contains(leakErr.Error(), rawText) {
		t.Fatalf("recompiled projection error leaked private ingress: %v", leakErr)
	}
	if resolverCalls.Load() != 8 || trackedAuthority.calls.Load() != 10 {
		t.Fatalf("unexpected replay phase calls: resolver=%d authority=%d", resolverCalls.Load(), trackedAuthority.calls.Load())
	}
}

func TestUseRecompiledIngressProviderProjectionFailsClosedBeforeDelivery(t *testing.T) {
	type fixtureV1 struct {
		ctx             context.Context
		harness         *currentdatasettest.Harness
		securityContext domainsecurity.TurnSecurityContext
		store           *caseEntityMemoryStoreV1
		service         *Service
		rawText         string
		resolver        ResolveAccountIngressCandidatesV1
	}
	newFixture := func(t *testing.T) fixtureV1 {
		t.Helper()
		ctx := context.Background()
		harness := currentdatasettest.NewHarness()
		securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-recompile-closed", TurnID: "turn-recompile-closed",
			WorkspaceRealPath: "/cases/recompile-closed", TenantID: "tenant-a", UserID: "user-a",
			CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
			DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-recompile-closed"),
			SourceManifestHash: domainsecurity.SHA256Hex(
				[]byte("manifest:snapshot-recompile-closed"),
			),
			ContextEpoch: 1,
			IssuedAt:     time.Unix(1_700_000_021, 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		store := newCaseEntityMemoryStoreV1()
		service := NewPersistentService(
			&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
			store,
			harness,
			harness,
			harness.ValidateCurrent,
		)
		const exact = "6222021234567890123"
		rawText := "分析账号 " + exact
		resolver := accountIngressResolverV1(map[string]string{
			exact: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		})
		compiled, compileErr := service.CompileAccountIngressV1(ctx, NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			rawText,
			resolver,
		))
		if compileErr != nil || !compiled.IsPersisted() {
			t.Fatalf("fixture compile failed: result=%s err=%v", compiled, compileErr)
		}
		return fixtureV1{
			ctx: ctx, harness: harness, securityContext: securityContext,
			store: store, service: service, rawText: rawText, resolver: resolver,
		}
	}

	t.Run("missing noop coordinate", func(t *testing.T) {
		fixture := newFixture(t)
		resolverCalls := 0
		callbackCalls := 0
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			99,
			func(
				context.Context,
				domainsecurity.TurnSecurityContext,
				AccountIngressCandidateBatchV1,
			) (AccountIngressResolutionBatchV1, error) {
				resolverCalls++
				return AccountIngressResolutionBatchV1{}, nil
			},
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrPrivateStateNotFound) || resolverCalls != 0 || callbackCalls != 0 ||
			strings.Contains(err.Error(), fixture.rawText) {
			t.Fatalf("missing coordinate was not closed before effects: resolver=%d callback=%d err=%v", resolverCalls, callbackCalls, err)
		}
	})

	t.Run("source blocked", func(t *testing.T) {
		fixture := newFixture(t)
		callbackCalls := 0
		resolver := accountIngressResolverV1(map[string]string{})
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			resolver,
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrAccountIngressResolution) || callbackCalls != 0 ||
			strings.Contains(err.Error(), fixture.rawText) {
			t.Fatalf("source-negative replay reached provider delivery: callback=%d err=%v", callbackCalls, err)
		}
	})

	t.Run("resolver error is redacted", func(t *testing.T) {
		fixture := newFixture(t)
		callbackCalls := 0
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			func(
				_ context.Context,
				_ domainsecurity.TurnSecurityContext,
				candidates AccountIngressCandidateBatchV1,
			) (AccountIngressResolutionBatchV1, error) {
				_ = candidates.UseExactV1(func([]string) error { return nil })
				return AccountIngressResolutionBatchV1{}, errors.New("resolver echoed " + fixture.rawText)
			},
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrAccountIngressResolution) || callbackCalls != 0 ||
			strings.Contains(err.Error(), fixture.rawText) ||
			strings.Contains(err.Error(), "6222021234567890123") {
			t.Fatalf("resolver failure leaked private ingress: callback=%d err=%v", callbackCalls, err)
		}
	})

	t.Run("noop consumer", func(t *testing.T) {
		fixture := newFixture(t)
		callbackCalls := 0
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			fixture.resolver,
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrPrivateStateUnavailable) || callbackCalls != 1 ||
			strings.Contains(err.Error(), fixture.rawText) {
			t.Fatalf("noop projection consumer was accepted: callback=%d err=%v", callbackCalls, err)
		}
	})

	t.Run("entity type mismatch", func(t *testing.T) {
		fixture := newFixture(t)
		bindingsBefore := len(fixture.store.bindings)
		ingressBefore := len(fixture.store.ingress)
		callbackCalls := 0
		resolver := accountIngressResolverV1(map[string]string{
			"6222021234567890123": domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		})
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			resolver,
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrAccountIngressResolution) || callbackCalls != 0 ||
			len(fixture.store.bindings) != bindingsBefore || len(fixture.store.ingress) != ingressBefore {
			t.Fatalf("entity mismatch changed private state or reached delivery: bindings=%d ingress=%d callback=%d err=%v", len(fixture.store.bindings), len(fixture.store.ingress), callbackCalls, err)
		}
	})

	t.Run("authority stale after source resolution", func(t *testing.T) {
		fixture := newFixture(t)
		callbackCalls := 0
		resolver := func(
			ctx context.Context,
			securityContext domainsecurity.TurnSecurityContext,
			candidates AccountIngressCandidateBatchV1,
		) (AccountIngressResolutionBatchV1, error) {
			resolved, resolveErr := fixture.resolver(ctx, securityContext, candidates)
			fixture.harness.SetRevoked(true)
			return resolved, resolveErr
		}
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			resolver,
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrPrivateStateUnavailable) || callbackCalls != 0 ||
			strings.Contains(err.Error(), fixture.rawText) {
			t.Fatalf("stale authority reached provider delivery: callback=%d err=%v", callbackCalls, err)
		}
	})

	t.Run("ingress CAS conflict", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.store.putIngressErr = caseentityport.ErrConflict
		callbackCalls := 0
		err := fixture.service.UseRecompiledIngressProviderProjectionV1(
			fixture.ctx,
			fixture.securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			fixture.resolver,
			func(ProviderIngressProjectionV1) error {
				callbackCalls++
				return nil
			},
		)
		if !errors.Is(err, ErrPrivateStateConflict) || callbackCalls != 0 ||
			strings.Contains(err.Error(), fixture.rawText) {
			t.Fatalf("CAS conflict reached provider delivery: callback=%d err=%v", callbackCalls, err)
		}
	})
}

func TestAccountIngressDescriptorMatchesSecondExactLiveSelection(t *testing.T) {
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-ingress-provenance", TurnID: "turn-ingress-provenance",
		WorkspaceRealPath: "/cases/ingress-provenance", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("provisional-ingress-provenance"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("provisional-ingress-provenance"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_010, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		newCaseEntityMemoryStoreV1(),
		harness,
		harness,
		harness.ValidateCurrent,
	)
	descriptor, err := currentdatasettest.AccountIngressDescriptorV1(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	err = service.withCurrentPrivateSelectionV1(
		context.Background(),
		securityContext,
		func(_ context.Context, selection datasetsnapshotport.CurrentSelectionV2) error {
			if !service.accountIngressDescriptorMatchesCurrentSelectionV1(
				descriptor,
				securityContext,
				selection,
			) {
				return errors.New("first descriptor did not match second exact live selection")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompileAccountIngressRejectsFirstSourceRecordDriftBeforePrivateWrite(t *testing.T) {
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-ingress-record-drift", TurnID: "turn-ingress-record-drift",
		WorkspaceRealPath: "/cases/ingress-record-drift", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("provisional-ingress-record-drift"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("provisional-ingress-record-drift"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_010, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newCaseEntityMemoryStoreV1()
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
		harness,
		harness,
		harness.ValidateCurrent,
	)
	const exact = "6222021234567890123"
	compiled, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"核查账号 "+exact,
			func(
				ctx context.Context,
				current domainsecurity.TurnSecurityContext,
				candidates AccountIngressCandidateBatchV1,
			) (AccountIngressResolutionBatchV1, error) {
				batch, resolveErr := accountIngressBatchResolverFromCandidateV1(func(
					_ context.Context,
					_ domainsecurity.TurnSecurityContext,
					_ string,
				) (AccountIngressCandidateResolutionV1, error) {
					return AccountIngressCandidateResolutionV1{
						Disposition: AccountIngressResolutionResolvedAccountV1,
						FinancialAccountType: domaincontrolledaccount.
							ControlledAccountFinancialFieldBankAccountNumberV1,
					}, nil
				})(ctx, current, candidates)
				if resolveErr != nil {
					return AccountIngressResolutionBatchV1{}, resolveErr
				}
				input := accountIngressDescriptorInputV1ForTest(batch.sourceDescriptor)
				input.SnapshotRecordDigest = strings.Repeat("f", 64)
				drifted, descriptorErr := domainfundsquerysource.NewDescriptorV1(input)
				if descriptorErr != nil {
					return AccountIngressResolutionBatchV1{}, descriptorErr
				}
				batch.sourceDescriptor = drifted
				return batch, nil
			},
		),
	)
	if err != nil || !compiled.IsBlocked() || len(store.bindings) != 0 || len(store.ingress) != 0 {
		t.Fatalf(
			"first-source record drift crossed the second live selection: result=%s bindings=%d ingress=%d err=%v",
			compiled,
			len(store.bindings),
			len(store.ingress),
			err,
		)
	}
}

func TestCompileAccountIngressResolvesBatchBeforePrivateStateLease(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-batch-order", TurnID: "turn-batch-order", WorkspaceRealPath: "/cases/batch-order",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash:   strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-batch-order"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("manifest:snapshot-batch-order"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_011, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tracker := &trackingCurrentDatasetAuthorityV1{delegate: harness}
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		newCaseEntityMemoryStoreV1(),
		tracker,
		harness,
		harness.ValidateCurrent,
	)
	const exact = "6222021234567890123"
	resolverCalls := 0
	compiled, err := service.CompileAccountIngressV1(ctx, NewCompileAccountIngressInputV1(
		securityContext,
		domaincaseentity.CaseIngressKindTurnV1,
		0,
		"核查账号 "+exact,
		func(
			_ context.Context,
			currentContext domainsecurity.TurnSecurityContext,
			candidates AccountIngressCandidateBatchV1,
		) (AccountIngressResolutionBatchV1, error) {
			resolverCalls++
			if tracker.active.Load() {
				return AccountIngressResolutionBatchV1{}, errors.New("batch resolver entered a nested private-state lease")
			}
			body, marshalErr := json.Marshal(candidates)
			formatted := fmt.Sprintf("%#v", candidates)
			if marshalErr == nil || len(body) != 0 || strings.Contains(formatted, exact) ||
				!strings.Contains(formatted, "[REDACTED]") || candidates.CandidateCountV1() != 1 {
				return AccountIngressResolutionBatchV1{}, errors.New("private candidate carrier escaped")
			}
			var values []string
			if useErr := candidates.UseExactV1(func(current []string) error {
				values = append([]string(nil), current...)
				return nil
			}); useErr != nil || len(values) != 1 || values[0] != exact {
				return AccountIngressResolutionBatchV1{}, errors.New("private candidate batch changed")
			}
			if secondUseErr := candidates.UseExactV1(func([]string) error { return nil }); secondUseErr == nil {
				return AccountIngressResolutionBatchV1{}, errors.New("private candidate batch was reusable")
			}
			return newAccountIngressResolutionBatchV1ForTest(currentContext, []AccountIngressCandidateResolutionV1{{
				Ordinal:              0,
				Disposition:          AccountIngressResolutionResolvedAccountV1,
				FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				BankInstitution:      "中国银行",
				AccountType:          "储蓄账户",
			}})
		},
	))
	if err != nil || !compiled.IsPersisted() || resolverCalls != 1 ||
		tracker.calls.Load() != 1 || tracker.active.Load() {
		t.Fatalf("sequential ingress authority order failed: result=%s resolverCalls=%d leaseCalls=%d active=%t err=%v", compiled, resolverCalls, tracker.calls.Load(), tracker.active.Load(), err)
	}
}

func TestCompileAccountIngressRejectsResolverThatIgnoresFailedCandidateUse(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-failed-candidate-use", TurnID: "turn-failed-candidate-use",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-failed-candidate-use", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const exact = "6222021234567890123"
	compiled, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"核查账号 "+exact,
			func(
				_ context.Context,
				currentContext domainsecurity.TurnSecurityContext,
				candidates AccountIngressCandidateBatchV1,
			) (AccountIngressResolutionBatchV1, error) {
				_ = candidates.UseExactV1(func([]string) error {
					return errors.New("candidate consumer rejected private input")
				})
				return newAccountIngressResolutionBatchV1ForTest(
					currentContext,
					[]AccountIngressCandidateResolutionV1{{
						Ordinal:              0,
						Disposition:          AccountIngressResolutionResolvedAccountV1,
						FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					}},
				)
			},
		),
	)
	if err != nil || !compiled.IsBlocked() ||
		strings.Contains(compiled.PublicText, exact) ||
		len(store.bindings) != 0 || len(store.ingress) != 0 {
		t.Fatalf(
			"failed candidate use was treated as a verified resolution: result=%s bindings=%d ingress=%d err=%v",
			compiled, len(store.bindings), len(store.ingress), err,
		)
	}
}

func TestCompileAccountIngressIsStableWithinCaseAndSeparatedAcrossCases(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	store := newCaseEntityMemoryStoreV1()
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
		harness,
		harness,
		harness.ValidateCurrent,
	)
	newContext := func(threadID, turnID, caseID, binding, snapshot string, epoch uint64) domainsecurity.TurnSecurityContext {
		t.Helper()
		securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/current",
			TenantID: "tenant-a", UserID: "user-a", CaseID: caseID,
			CaseBindingHash:   binding,
			DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(snapshot),
			SourceManifestHash: domainsecurity.SHA256Hex(
				[]byte("manifest:" + snapshot),
			),
			ContextEpoch: epoch,
			IssuedAt:     time.Unix(1_700_000_020+int64(epoch), 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return securityContext
	}
	const exact = "6222021234567890123"
	firstContext := newContext("thread-a", "turn-a", "case-a", strings.Repeat("a", 64), "snapshot-a", 1)
	evolvedContext := newContext("thread-after-restart", "turn-b", "case-a", strings.Repeat("a", 64), "snapshot-b", 2)
	otherCaseContext := newContext("thread-case-b", "turn-c", "case-b", strings.Repeat("b", 64), "snapshot-c", 1)

	compile := func(securityContext domainsecurity.TurnSecurityContext, ordinal uint32) domaincaseentity.ReferenceV1 {
		t.Helper()
		compiled, err := service.CompileAccountIngressV1(
			ctx,
			NewCompileAccountIngressInputV1(
				securityContext,
				domaincaseentity.CaseIngressKindTurnV1,
				ordinal,
				"账号："+exact,
				accountIngressResolverV1(map[string]string{
					exact: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				}),
			),
		)
		if err != nil {
			t.Fatal(err)
		}
		privateReference := useAccountIngressPrivateReferenceV1(t, compiled)
		record := store.ingress[privateReference.RecordID]
		if len(record.Spans) != 1 {
			t.Fatalf("compiled ingress span count = %d", len(record.Spans))
		}
		return record.Spans[0].Reference
	}
	first := compile(firstContext, 0)
	evolved := compile(evolvedContext, 0)
	otherCase := compile(otherCaseContext, 0)
	if first != evolved {
		t.Fatalf("same-case account reference drifted: first=%q evolved=%q", first, evolved)
	}
	if first == otherCase {
		t.Fatalf("account reference crossed case boundary: %q", first)
	}
}

func TestCompileAccountIngressNoopAndUnsupportedPIIFailClosed(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-compile-validation", TurnID: "turn-compile-validation",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const safeText = "请分析本月的流入和流出"
	noop, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			safeText,
			nil,
		),
	)
	if err != nil || !noop.IsNoop() || noop.PublicText != safeText ||
		noop.SpanCount != 0 || noop.UniqueReferenceCount != 0 ||
		len(store.bindings) != 0 || len(store.ingress) != 0 {
		t.Fatalf("safe no-op compilation mismatch: result=%#v err=%v", noop, err)
	}
	if useErr := noop.UsePrivateRecordReferenceV1(func(PrivateRecordReferenceV1) error { return nil }); !errors.Is(useErr, ErrPrivateStateUnavailable) {
		t.Fatalf("no-op exposed a private handle: %v", useErr)
	}
	const ordinaryCodeText = `请修改 cer1_parser 和 cer1_callback，并解释 ReferencePrefixV1 = "cer1_"`
	ordinaryCode, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			1,
			ordinaryCodeText,
			nil,
		),
	)
	if err != nil || !ordinaryCode.IsNoop() || ordinaryCode.PublicText != ordinaryCodeText {
		t.Fatalf("ordinary code identifier was treated as a forged case reference: result=%s err=%v", ordinaryCode, err)
	}

	tests := map[string]struct {
		rawText   string
		forbidden []string
	}{
		"mixed phone": {
			rawText:   "账号：6222021234567890；手机号：13800138000",
			forbidden: []string{"6222021234567890", "13800138000"},
		},
		"identity number": {
			rawText:   "身份证号：11010519491231002X",
			forbidden: []string{"11010519491231002X"},
		},
		"non-canonical card": {
			rawText:   "卡号：６２２２０２１２３４５６７８９０",
			forbidden: []string{"６２２２０２１２３４５６７８９０"},
		},
		"opaque account": {
			rawText:   "account number: opaque-value",
			forbidden: []string{"opaque-value"},
		},
		"partial account": {
			rawText:   "账号：1234",
			forbidden: []string{"1234"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := service.CompileAccountIngressV1(
				context.Background(),
				NewCompileAccountIngressInputV1(
					securityContext,
					domaincaseentity.CaseIngressKindTurnV1,
					1,
					test.rawText,
					nil,
				),
			)
			if err != nil || !result.IsBlocked() || result.PublicText == "" {
				t.Fatalf("unsupported PII did not fail closed: result=%#v err=%v", result, err)
			}
			assertTextDoesNotContainAnyV1(t, result.PublicText, test.forbidden...)
		})
	}
	if len(store.bindings) != 0 || len(store.ingress) != 0 {
		t.Fatalf("validation failure wrote private state: bindings=%d ingress=%d", len(store.bindings), len(store.ingress))
	}

	const exact = "6222021234567890"
	invalid, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			"unknown",
			2,
			"账号："+exact,
			nil,
		),
	)
	if !errors.Is(err, ErrInvalidAccountIngress) || invalid.Status != "" ||
		invalid.PublicText != "" {
		t.Fatalf("invalid ingress slot did not fail closed: result=%#v err=%v", invalid, err)
	}
	assertTextDoesNotContainAnyV1(t, err.Error(), exact)
}

func TestCompileAccountIngressSourceAwareDiscoveryDoesNotLeakUncuedOrUnresolvedNumbers(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-source-aware", TurnID: "turn-source-aware",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-source-aware", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)

	const uncued = "12345678"
	resolved, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"核查 "+uncued,
			accountIngressResolverV1(map[string]string{
				uncued: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}),
		),
	)
	resolvedProviderText := useAccountIngressProviderProjectionV1(t, resolved)
	if err != nil || !resolved.IsPersisted() || strings.Contains(resolvedProviderText, uncued) ||
		strings.Contains(resolvedProviderText, "cer1_") ||
		strings.Count(resolvedProviderText, "acct:1") != 1 ||
		strings.Contains(resolved.PublicText, "cer1_") || !strings.Contains(resolved.PublicText, "[ACCOUNT]") {
		t.Fatalf("uncued current-snapshot account was not safely tokenized: result=%s err=%v", resolved, err)
	}
	const invalidAccountSpelling = "6217/00/9876543210"
	invalidCanonical := accountIngressASCIIDigitsV1(invalidAccountSpelling)
	invalidSpellingResult, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			1,
			"核查 "+invalidAccountSpelling,
			accountIngressResolverV1(map[string]string{
				invalidCanonical: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}),
		),
	)
	if err != nil || !invalidSpellingResult.IsBlocked() ||
		strings.Contains(invalidSpellingResult.PublicText, invalidAccountSpelling) ||
		len(store.bindings) != 1 || len(store.ingress) != 1 {
		t.Fatalf("invalid resolved account spelling crossed the pre-write boundary: result=%s bindings=%d ingress=%d err=%v", invalidSpellingResult, len(store.bindings), len(store.ingress), err)
	}
	const currentAccount = "6222021234567890123"
	accountListResult, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			"核查账号 "+currentAccount+" 和 2026-07-28",
			accountIngressResolverV1(map[string]string{
				currentAccount: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}),
		),
	)
	if err != nil || !accountListResult.IsBlocked() ||
		strings.Contains(accountListResult.PublicText, currentAccount) ||
		strings.Contains(accountListResult.PublicText, "2026-07-28") ||
		len(store.bindings) != 1 || len(store.ingress) != 1 {
		t.Fatalf("ambiguous account list crossed the all-candidates pre-write boundary: result=%s bindings=%d ingress=%d err=%v", accountListResult, len(store.bindings), len(store.ingress), err)
	}

	for name, test := range map[string]struct {
		raw       string
		forbidden string
	}{
		"account-labeled date shape":       {raw: "核查账号 2026-07-28", forbidden: "2026-07-28"},
		"account symbol joiner":            {raw: "核查账号№2026-07-28", forbidden: "2026-07-28"},
		"account parenthetic joiner":       {raw: "核查账号（待查）2026-07-28", forbidden: "2026-07-28"},
		"account no joiner":                {raw: "check account no. 2026-07-28", forbidden: "2026-07-28"},
		"account format joiner":            {raw: "核查账号\u200b2026-07-28", forbidden: "2026-07-28"},
		"account cue format split":         {raw: "核查账\u200b号 2026-07-28", forbidden: "2026-07-28"},
		"account cue mark split":           {raw: "核查账\u034f号 金额 6217009876543210 CNY", forbidden: "6217009876543210"},
		"account ASCII mark split":         {raw: "acc\u034fount amount 6217009876543210 USD", forbidden: "6217009876543210"},
		"account cue selector split":       {raw: "核查账\ufe0f号 日期 2026-07-28", forbidden: "2026-07-28"},
		"account cue punctuation split":    {raw: "核查账-号 金额 6217009876543210 CNY", forbidden: "6217009876543210"},
		"account cue fullwidth split":      {raw: "核查账－号 金额 6217009876543210 CNY", forbidden: "6217009876543210"},
		"account ASCII punctuation split":  {raw: "a.c.c.o.u.n.t amount 6217009876543210 USD", forbidden: "6217009876543210"},
		"account mixed visual split":       {raw: "Ａ．c\u034fＣＯＵＮＴ amount 6217009876543210 USD", forbidden: "6217009876543210"},
		"account underscore label":         {raw: "account_id=6217009876543210 CNY", forbidden: "6217009876543210"},
		"card underscore label":            {raw: "card_id=6217009876543210 CNY", forbidden: "6217009876543210"},
		"account camel label":              {raw: "sourceAccountId=6217009876543210 CNY", forbidden: "6217009876543210"},
		"acct camel label":                 {raw: "acctNo=6217009876543210 CNY", forbidden: "6217009876543210"},
		"embedded lowercase account label": {raw: "sourceaccountid=6217009876543210 CNY", forbidden: "6217009876543210"},
		"joined bank account label":        {raw: "bankaccountnumber=6217009876543210 CNY", forbidden: "6217009876543210"},
		"uppercase account label":          {raw: "ACCOUNTID=6217009876543210 CNY", forbidden: "6217009876543210"},
		"account amount cue poison":        {raw: "核查账号 金额 6217009876543210 CNY", forbidden: "6217009876543210"},
		"account date cue poison":          {raw: "核查账号 日期 2026-07-28", forbidden: "2026-07-28"},
		"account embedded temporal":        {raw: "核查账号存在 2026-07-28", forbidden: "2026-07-28"},
		"non-UTC timestamp":                {raw: "截至 2026-07-28T15:30:45+08:00", forbidden: "2026-07-28"},
		"timestamp token suffix":           {raw: "截至 2026-07-28T15:30:45Zextra", forbidden: "2026-07-28"},
		"timestamp underscore suffix":      {raw: "截至 2026-07-28T15:30:45Z_extra", forbidden: "2026-07-28"},
		"date word suffix":                 {raw: "核查 2026-07-28extra", forbidden: "2026-07-28"},
		"date underscore suffix":           {raw: "核查 2026-07-28_account", forbidden: "2026-07-28"},
		"date postfix account":             {raw: "核查 2026-07-28账号", forbidden: "2026-07-28"},
		"date postfix account phrase":      {raw: "核查 2026-07-28 是账号", forbidden: "2026-07-28"},
		"date postfix next sentence":       {raw: "核查 2026-07-28。它是账号", forbidden: "2026-07-28"},
		"currency postfix account":         {raw: "6217009876543210 CNY 为账号", forbidden: "6217009876543210"},
		"currency prefix spoof":            {raw: "notcny 6217009876543210", forbidden: "6217009876543210"},
		"currency prefix word":             {raw: "非人民币 6217009876543210", forbidden: "6217009876543210"},
		"currency suffix spoof":            {raw: "6217009876543210 cny_account", forbidden: "6217009876543210"},
		"currency suffix word":             {raw: "6217009876543210 元件", forbidden: "6217009876543210"},
		"compact date":                     {raw: "比较日期 20260728 的流入", forbidden: "20260728"},
		"compact timestamp":                {raw: "比较时间 20260728153045 的流入", forbidden: "20260728153045"},
		"epoch":                            {raw: "比较时间戳 1700000000000 的流出", forbidden: "1700000000000"},
		"bare amount":                      {raw: "比较金额 328000000 的流出", forbidden: "328000000"},
		"bare count":                       {raw: "交易笔数 12600000", forbidden: "12600000"},
		"invalid compact date":             {raw: "比较 20261340 的流入", forbidden: "20261340"},
		"unknown":                          {raw: "核查账号 6217009876543210", forbidden: "6217009876543210"},
		"uncued short":                     {raw: "核查 12345678", forbidden: "12345678"},
		"full-width short":                 {raw: "核查 １２３４５６７８", forbidden: "１２３４５６７８"},
		"superscript number":               {raw: "核查 " + strings.Repeat("⁶", 8), forbidden: strings.Repeat("⁶", 8)},
		"circled number":                   {raw: "核查 ①②③④⑤⑥⑦⑧", forbidden: "①②③④⑤⑥⑦⑧"},
		"expanding circled number":         {raw: "核查 ⑩⑩⑩⑩", forbidden: "⑩⑩⑩⑩"},
		"combining mark account":           {raw: "核查 6222021\u034f2345678\u034f90123", forbidden: "6222021"},
		"variation selector account":       {raw: "核查 6222021\ufe0f2345678\ufe0f90123", forbidden: "6222021"},
		"overlong visual separator account": {
			raw: "核查 6222021" + strings.Repeat("\u034f", 300) + "2345678" +
				strings.Repeat("\ufe0f", 300) + "90",
			forbidden: "6222021",
		},
	} {
		t.Run(name, func(t *testing.T) {
			blocked, compileErr := service.CompileAccountIngressV1(
				context.Background(),
				NewCompileAccountIngressInputV1(
					securityContext,
					domaincaseentity.CaseIngressKindSteerV1,
					1,
					test.raw,
					accountIngressResolverV1(nil),
				),
			)
			if compileErr != nil || !blocked.IsBlocked() || blocked.PublicText == "" ||
				strings.Contains(blocked.PublicText, test.forbidden) ||
				domaincaseentity.ContainsReferenceV1(blocked.PublicText) {
				t.Fatalf("unresolved numeric candidate was not withheld: result=%s err=%v", blocked, compileErr)
			}
			if useErr := blocked.UsePrivateRecordReferenceV1(func(PrivateRecordReferenceV1) error { return nil }); !errors.Is(useErr, ErrPrivateStateUnavailable) {
				t.Fatalf("blocked result exposed private state: %v", useErr)
			}
		})
	}
	for name, resolver := range map[string]accountIngressTestCandidateResolverV1{
		"invalid disposition": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{}, nil
		},
		"invalid nonzero disposition": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{
				Disposition: AccountIngressResolutionDispositionV1(255),
			}, nil
		},
		"invalid type": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{
				Disposition:          AccountIngressResolutionResolvedAccountV1,
				FinancialAccountType: "unknown",
			}, nil
		},
		"complete identifier in semantic metadata": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{
				Disposition:          AccountIngressResolutionResolvedAccountV1,
				FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				BankInstitution:      uncued,
				AccountType:          "储蓄账户",
			}, nil
		},
		"verified negative with type": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{
				Disposition:          AccountIngressResolutionVerifiedNoCurrentAccountMatchV1,
				FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}, nil
		},
		"private resolver failure": func(
			_ context.Context,
			_ domainsecurity.TurnSecurityContext,
			_ string,
		) (AccountIngressCandidateResolutionV1, error) {
			return AccountIngressCandidateResolutionV1{}, errors.New("private source failure carried " + uncued)
		},
	} {
		t.Run(name, func(t *testing.T) {
			blocked, compileErr := service.CompileAccountIngressV1(
				context.Background(),
				NewCompileAccountIngressInputV1(
					securityContext,
					domaincaseentity.CaseIngressKindSteerV1,
					2,
					"核查 "+uncued,
					accountIngressBatchResolverFromCandidateV1(resolver),
				),
			)
			if compileErr != nil || !blocked.IsBlocked() ||
				strings.Contains(blocked.PublicText, uncued) ||
				domaincaseentity.ContainsReferenceV1(blocked.PublicText) {
				t.Fatalf("invalid source resolution did not fail closed: result=%s err=%v", blocked, compileErr)
			}
		})
	}
	if len(store.ingress) != 1 {
		t.Fatalf("blocked candidates wrote private ingress: count=%d", len(store.ingress))
	}
	if len(store.bindings) != 1 {
		t.Fatalf("blocked candidates wrote private bindings: count=%d", len(store.bindings))
	}
}

func TestCompileAccountIngressBlocksSourceRejectedStructuredNumbers(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-structured-numbers", TurnID: "turn-structured-numbers",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-structured-numbers", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	for name, rawText := range map[string]string{
		"ISO date":              "查询 2026-07-01 至 2026-07-28 的流入流出",
		"UTC timestamp":         "截至 2026-07-28T15:30:45Z 的结果",
		"funds timestamp":       "截至2026-07-28T15:30:45.000000Z的结果",
		"currency suffix":       "金额 328000000 CNY",
		"currency prefix":       "金额 ¥328,000,000.25",
		"currency symbol tight": "金额¥328000000",
		"currency word tight":   "金额人民币328000000",
		"currency unit":         "金额 328000000 元",
	} {
		t.Run(name, func(t *testing.T) {
			resolverCalls := 0
			result, err := service.CompileAccountIngressV1(
				context.Background(),
				NewCompileAccountIngressInputV1(
					securityContext,
					domaincaseentity.CaseIngressKindTurnV1,
					0,
					rawText,
					accountIngressBatchResolverFromCandidateV1(func(
						_ context.Context,
						_ domainsecurity.TurnSecurityContext,
						_ string,
					) (AccountIngressCandidateResolutionV1, error) {
						resolverCalls++
						return AccountIngressCandidateResolutionV1{
							Disposition: AccountIngressResolutionVerifiedNoCurrentAccountMatchV1,
						}, nil
					}),
				),
			)
			if err != nil || !result.IsBlocked() || resolverCalls == 0 {
				t.Fatalf("source-rejected structured number did not fail closed: status=%q public=%q calls=%d err=%v", result.Status, result.PublicText, resolverCalls, err)
			}
			spans, spanErr := accountIngressRawSpansV1(rawText)
			if spanErr != nil || len(spans) == 0 {
				t.Fatalf("structured number discovery: spans=%#v err=%v", spans, spanErr)
			}
			for _, span := range spans {
				rawValue := rawText[span.StartByte:span.EndByte]
				if strings.Contains(result.PublicText, rawValue) {
					t.Fatalf("source-rejected structured number leaked: raw=%q public=%q", rawValue, result.PublicText)
				}
			}
		})
	}
	if len(store.ingress) != 0 || len(store.bindings) != 0 {
		t.Fatalf("source-rejected structured numbers wrote private state: ingress=%d bindings=%d", len(store.ingress), len(store.bindings))
	}
}

func TestCompileAccountIngressBlocksMixedResolvedAccountAndSourceRejectedNumbers(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-mixed-structured", TurnID: "turn-mixed-structured",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-mixed-structured", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const account = "6222021234567890123"
	rawText := "查询账号 " + account + " 在 2026-07-01 至 2026-07-28 的流入，金额 328000000 元"
	compiled, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			rawText,
			accountIngressResolverV1(map[string]string{
				account: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}),
		),
	)
	if err != nil || !compiled.IsBlocked() {
		t.Fatalf("mixed source resolution did not fail closed: status=%q public=%q err=%v", compiled.Status, compiled.PublicText, err)
	}
	for _, rawValue := range []string{account, "2026-07-01", "2026-07-28", "328000000"} {
		if strings.Contains(compiled.PublicText, rawValue) {
			t.Fatalf("mixed source-rejected ingress leaked %q: public=%q", rawValue, compiled.PublicText)
		}
	}
	if len(store.ingress) != 0 || len(store.bindings) != 0 {
		t.Fatalf("mixed source resolution partially wrote private state: ingress=%d bindings=%d", len(store.ingress), len(store.bindings))
	}
}

func TestCompileAccountIngressResolvesISOLikeAccountsDeduplicatesAndRejectsForgedReferences(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-source-aware-iso", TurnID: "turn-source-aware-iso",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-source-aware-iso", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const (
		isoSpelling = "2026-07-28"
		canonical   = "20260728"
	)
	resolverCalls := 0
	resolver := func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
		candidate string,
	) (AccountIngressCandidateResolutionV1, error) {
		resolverCalls++
		disposition := AccountIngressResolutionVerifiedNoCurrentAccountMatchV1
		accountType := ""
		if candidate == canonical {
			disposition = AccountIngressResolutionResolvedAccountV1
			accountType = domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
		}
		return AccountIngressCandidateResolutionV1{
			Disposition:          disposition,
			FinancialAccountType: accountType,
		}, nil
	}
	batchResolver := accountIngressBatchResolverFromCandidateV1(resolver)
	compiled, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			"核查 "+isoSpelling+" 与 "+isoSpelling,
			batchResolver,
		),
	)
	compiledProviderText := useAccountIngressProviderProjectionV1(t, compiled)
	if err != nil || !compiled.IsPersisted() || compiled.SpanCount != 2 ||
		compiled.UniqueReferenceCount != 1 || resolverCalls != 1 ||
		strings.Contains(compiledProviderText, isoSpelling) ||
		strings.Contains(compiledProviderText, "cer1_") ||
		strings.Count(compiledProviderText, "acct:1") != 2 ||
		strings.Contains(compiled.PublicText, isoSpelling) ||
		strings.Contains(compiled.PublicText, "cer1_") {
		t.Fatalf("ISO-shaped exact account was not resolved once and tokenized: result=%s calls=%d err=%v", compiled, resolverCalls, err)
	}

	forged, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			1,
			"继续分析 "+string(forged),
			batchResolver,
		),
	)
	if err != nil || !blocked.IsBlocked() || resolverCalls != 1 ||
		domaincaseentity.ContainsReferenceV1(blocked.PublicText) ||
		strings.Contains(blocked.PublicText, string(forged)) ||
		!strings.Contains(blocked.PublicText, "[ACCOUNT]") {
		t.Fatalf("forged internal reference was not withheld before source resolution: result=%s calls=%d err=%v", blocked, resolverCalls, err)
	}
	const malformedReference = "cer1_deadbeef"
	ordinaryIdentifier, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindSteerV1,
			2,
			"继续分析 "+malformedReference,
			batchResolver,
		),
	)
	if err != nil || !ordinaryIdentifier.IsNoop() || resolverCalls != 1 ||
		ordinaryIdentifier.PublicText != "继续分析 "+malformedReference {
		t.Fatalf("ordinary short code identifier was blocked as internal identity: result=%s calls=%d err=%v", ordinaryIdentifier, resolverCalls, err)
	}
}

func TestCompileAccountIngressWithholdsVisuallyObfuscatedReferenceCandidatesBeforeEffects(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-obfuscated-reference", TurnID: "turn-obfuscated-reference",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-obfuscated-reference", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	resolverCalls := 0
	resolver := func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
		_ string,
	) (AccountIngressCandidateResolutionV1, error) {
		resolverCalls++
		return AccountIngressCandidateResolutionV1{}, errors.New("resolver must not be reached")
	}
	batchResolver := accountIngressBatchResolverFromCandidateV1(resolver)
	for ordinal, candidate := range []string{
		"cer1_" + strings.Repeat("aaaa\u200b", 16),
		"cer1_" + strings.Repeat("dead-beef-", 8),
		"ＣＥＲ１＿" + strings.Repeat("Ａ", 64),
		"𝐜𝐞𝐫𝟏_" + strings.Repeat("𝐚", 64),
		"ⓒⓔⓡ①＿" + strings.Repeat("ⓐ", 64),
		"c\u034fe\u200br1_" + strings.Repeat("aaaa-aaaa-", 8),
		"C.E.R.1＿" + strings.Repeat("AAAA-AAAA-", 8),
		"cer1_aaaa cer1_" + strings.Repeat("deadbeef", 8),
	} {
		result, err := service.CompileAccountIngressV1(
			context.Background(),
			NewCompileAccountIngressInputV1(
				securityContext,
				domaincaseentity.CaseIngressKindSteerV1,
				uint32(ordinal),
				"继续分析 "+candidate,
				batchResolver,
			),
		)
		if err != nil || !result.IsBlocked() || resolverCalls != 0 ||
			len(store.bindings) != 0 || len(store.ingress) != 0 ||
			strings.Contains(result.PublicText, candidate) ||
			domaincaseentity.ContainsReferenceCandidateV1(result.PublicText) ||
			!strings.Contains(result.PublicText, "[ACCOUNT]") {
			t.Fatalf("obfuscated reference candidate crossed preflight: candidate=%q result=%s calls=%d bindings=%d ingress=%d err=%v", candidate, result, resolverCalls, len(store.bindings), len(store.ingress), err)
		}
	}
}

func TestAccountIngressFindingValidationRejectsOverlapAndInvalidBoundaries(t *testing.T) {
	rawText := "账号：6222021234567890"
	start := strings.Index(rawText, "6222021234567890")
	valid := domainprivacyprojection.Finding{
		Kind: domainprivacyprojection.KindAccount, StartByte: start, EndByte: len(rawText),
	}
	if _, _, err := accountIngressDiscoveryTextV1(
		rawText,
		[]domainprivacyprojection.Finding{
			valid,
			{Kind: domainprivacyprojection.KindAccount, StartByte: start + 1, EndByte: len(rawText)},
		},
	); !errors.Is(err, ErrAccountIngressProjection) {
		t.Fatalf("overlapping finding was accepted: %v", err)
	}
	unicodeRaw := "账号：中6222021234567890"
	unicodeStart := strings.Index(unicodeRaw, "中")
	if validAccountIngressFindingV1(unicodeRaw, domainprivacyprojection.Finding{
		Kind: domainprivacyprojection.KindAccount, StartByte: unicodeStart + 1, EndByte: len(unicodeRaw),
	}) {
		t.Fatal("non-UTF-8-boundary finding was accepted")
	}
}

func TestCompileAccountIngressPreservesOriginalOffsetsAcrossPrivacyFixedPoint(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-fixed-point", TurnID: "turn-fixed-point",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-fixed-point", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const (
		firstExact  = "6222020000000000000"
		secondExact = "12345678"
	)
	rawText := "账户 " + firstExact + "xxxxxxxxxx " + secondExact
	privacy := domainprivacyprojection.ProjectText(rawText)
	if len(privacy.Findings) != 1 || strings.Count(privacy.Text, "[ACCOUNT]") != 2 {
		t.Fatalf("test input did not exercise fixed-point offset discovery: %#v", privacy)
	}
	compiled, err := service.CompileAccountIngressV1(
		context.Background(),
		NewCompileAccountIngressInputV1(
			securityContext,
			domaincaseentity.CaseIngressKindTurnV1,
			0,
			rawText,
			accountIngressResolverV1(map[string]string{
				firstExact:  domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				secondExact: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			}),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	privateReference := useAccountIngressPrivateReferenceV1(t, compiled)
	record := store.ingress[privateReference.RecordID]
	if len(record.Spans) != 2 || compiled.SpanCount != 2 || compiled.UniqueReferenceCount != 2 {
		t.Fatalf("fixed-point compilation lost an account: result=%#v record=%#v", compiled, record)
	}
	for index, exact := range []string{firstExact, secondExact} {
		span := record.Spans[index]
		if rawText[span.RawStartByte:span.RawEndByte] != exact ||
			record.ProjectedText[span.ProjectedStartByte:span.ProjectedEndByte] != string(span.Reference) {
			t.Fatalf("fixed-point span %d lost original offsets: %#v", index, span)
		}
	}
	assertAccountIngressValuesDoNotLeakV1(t, []string{firstExact, secondExact}, compiled, record)
}

func TestCompileAccountIngressSuppressesPersistenceFailureAndAuthorityDrift(t *testing.T) {
	ctx := context.Background()
	const exact = "6222021234567890123"
	newFixture := func() (*currentdatasettest.Harness, domainsecurity.TurnSecurityContext, *caseEntityMemoryStoreV1, *Service) {
		t.Helper()
		harness := currentdatasettest.NewHarness()
		securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-failure", TurnID: "turn-failure", WorkspaceRealPath: "/cases/failure",
			TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
			CaseBindingHash:   strings.Repeat("a", 64),
			DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-failure"),
			SourceManifestHash: domainsecurity.SHA256Hex(
				[]byte("manifest:snapshot-failure"),
			),
			ContextEpoch: 1,
			IssuedAt:     time.Unix(1_700_000_030, 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		store := newCaseEntityMemoryStoreV1()
		service := NewPersistentService(
			&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
			store,
			harness,
			harness,
			harness.ValidateCurrent,
		)
		return harness, securityContext, store, service
	}
	compile := func(service *Service, securityContext domainsecurity.TurnSecurityContext) (AccountIngressCompilationV1, error) {
		return service.CompileAccountIngressV1(
			ctx,
			NewCompileAccountIngressInputV1(
				securityContext,
				domaincaseentity.CaseIngressKindSteerV1,
				1,
				"账号："+exact,
				accountIngressResolverV1(map[string]string{
					exact: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				}),
			),
		)
	}

	_, persistenceContext, persistenceStore, persistenceService := newFixture()
	persistenceStore.putIngressErr = errors.New("adapter detail carried " + exact)
	result, err := compile(persistenceService, persistenceContext)
	if !errors.Is(err, ErrPrivateStateIntegrity) || result.Status != "" ||
		len(persistenceStore.ingress) != 0 || len(persistenceStore.bindings) != 1 {
		t.Fatalf("persistence failure did not fail closed: result=%#v err=%v", result, err)
	}
	assertTextDoesNotContainAnyV1(t, err.Error(), exact, "adapter detail")
	persistenceStore.putIngressErr = nil
	result, err = compile(persistenceService, persistenceContext)
	if err != nil || !result.IsPersisted() || len(persistenceStore.bindings) != 1 ||
		len(persistenceStore.ingress) != 1 {
		t.Fatalf("retry did not reuse the idempotent private binding: result=%#v err=%v", result, err)
	}

	_, partialContext, partialStore, partialService := newFixture()
	const secondExact = "6217009876543210987"
	putCalls := 0
	partialStore.putBindingHook = func(domaincaseentity.CaseEntityBindingRecord) {
		putCalls++
		if putCalls == 2 {
			partialStore.putBindingErr = errors.New("adapter detail carried " + secondExact)
		}
	}
	compilePair := func() (AccountIngressCompilationV1, error) {
		return partialService.CompileAccountIngressV1(
			ctx,
			NewCompileAccountIngressInputV1(
				partialContext,
				domaincaseentity.CaseIngressKindSteerV1,
				2,
				"账号："+exact+"；另一账号："+secondExact,
				accountIngressResolverV1(map[string]string{
					exact:       domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					secondExact: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				}),
			),
		)
	}
	result, err = compilePair()
	if !errors.Is(err, ErrPrivateStateIntegrity) || result.Status != "" ||
		len(partialStore.bindings) != 1 || len(partialStore.ingress) != 0 {
		t.Fatalf("partial binding failure escaped or lost its bounded residue: result=%#v err=%v", result, err)
	}
	assertTextDoesNotContainAnyV1(t, err.Error(), exact, secondExact, "adapter detail")
	partialStore.putBindingErr = nil
	partialStore.putBindingHook = nil
	result, err = compilePair()
	if err != nil || !result.IsPersisted() || len(partialStore.bindings) != 2 ||
		len(partialStore.ingress) != 1 {
		t.Fatalf("retry did not close partial idempotent bindings: result=%#v err=%v", result, err)
	}

	driftHarness, driftContext, driftStore, driftService := newFixture()
	driftStore.putIngressHook = func() { driftHarness.SetRevoked(true) }
	result, err = compile(driftService, driftContext)
	if !errors.Is(err, ErrPrivateStateUnavailable) || result.Status != "" {
		t.Fatalf("authority drift returned a provider projection: result=%#v err=%v", result, err)
	}
	assertTextDoesNotContainAnyV1(t, err.Error(), exact)
	if len(driftStore.ingress) != 1 {
		t.Fatal("authority drift test did not reach the private persistence boundary")
	}

	driftHarness.SetRevoked(false)
	driftStore.putIngressHook = nil
	result, err = compile(driftService, driftContext)
	if err != nil || !result.IsPersisted() {
		t.Fatalf("reauthorized additive service did not resume: result=%#v err=%v", result, err)
	}
}

func assertAccountIngressValuesDoNotLeakV1(t *testing.T, exactValues []string, values ...any) {
	t.Helper()
	for _, value := range values {
		body, err := json.Marshal(value)
		formatted := fmt.Sprintf("%#v", value)
		for _, exact := range exactValues {
			if strings.Contains(string(body), exact) || strings.Contains(formatted, exact) ||
				(err != nil && strings.Contains(err.Error(), exact)) {
				t.Fatalf("ordinary value exposed source-exact account: body=%q format=%q err=%v", body, formatted, err)
			}
		}
	}
}

func assertTextDoesNotContainAnyV1(t *testing.T, text string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && strings.Contains(text, value) {
			t.Fatalf("text exposed forbidden private value %q", value)
		}
	}
}

func accountIngressResolverV1(
	verified map[string]string,
) ResolveAccountIngressCandidatesV1 {
	copy := make(map[string]string, len(verified))
	for canonical, entityType := range verified {
		copy[canonical] = entityType
	}
	return accountIngressBatchResolverFromCandidateV1(func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
		canonical string,
	) (AccountIngressCandidateResolutionV1, error) {
		entityType, found := copy[canonical]
		disposition := AccountIngressResolutionVerifiedNoCurrentAccountMatchV1
		if found {
			disposition = AccountIngressResolutionResolvedAccountV1
		}
		return AccountIngressCandidateResolutionV1{
			Disposition: disposition, FinancialAccountType: entityType,
		}, nil
	})
}

type accountIngressTestCandidateResolverV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	string,
) (AccountIngressCandidateResolutionV1, error)

func accountIngressBatchResolverFromCandidateV1(
	resolve accountIngressTestCandidateResolverV1,
) ResolveAccountIngressCandidatesV1 {
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		candidates AccountIngressCandidateBatchV1,
	) (AccountIngressResolutionBatchV1, error) {
		var values []string
		if err := candidates.UseExactV1(func(current []string) error {
			values = append([]string(nil), current...)
			return nil
		}); err != nil {
			return AccountIngressResolutionBatchV1{}, err
		}
		resolutions := make([]AccountIngressCandidateResolutionV1, len(values))
		for index, canonical := range values {
			resolution, err := resolve(ctx, securityContext, canonical)
			if err != nil {
				return AccountIngressResolutionBatchV1{}, err
			}
			resolution.Ordinal = uint32(index)
			resolutions[index] = resolution
		}
		return newAccountIngressResolutionBatchV1ForTest(securityContext, resolutions)
	}
}

func newAccountIngressResolutionBatchV1ForTest(
	securityContext domainsecurity.TurnSecurityContext,
	resolutions []AccountIngressCandidateResolutionV1,
) (AccountIngressResolutionBatchV1, error) {
	descriptor, err := currentdatasettest.AccountIngressDescriptorV1(securityContext)
	if err != nil {
		return AccountIngressResolutionBatchV1{}, err
	}
	nativeResolutions := make([]domainnative.AccountIngressResolutionV1, len(resolutions))
	for index, resolution := range resolutions {
		nativeResolution := domainnative.AccountIngressResolutionV1{Ordinal: uint32(index)}
		switch resolution.Disposition {
		case AccountIngressResolutionResolvedAccountV1:
			nativeResolution.Disposition = domainnative.AccountIngressResolutionDispositionResolvedV1
			nativeResolution.EntityType = resolution.FinancialAccountType
			nativeResolution.BankInstitution = resolution.BankInstitution
			nativeResolution.AccountType = resolution.AccountType
		case AccountIngressResolutionVerifiedNoCurrentAccountMatchV1:
			nativeResolution.Disposition = domainnative.AccountIngressResolutionDispositionNotFoundV1
		default:
			nativeResolution.Disposition = "invalid"
		}
		nativeResolutions[index] = nativeResolution
	}
	return NewAccountIngressResolutionBatchV1(
		securityContext,
		descriptor,
		domainnative.ResolveAccountIngressResultV1{
			SchemaVersion: 1,
			Contract:      domainnative.AccountIngressResolutionResultContractV1,
			Resolutions:   nativeResolutions,
			Provenance: domainnative.AccountIngressResolutionProvenanceV1{
				DatasetSnapshotID:              securityContext.DatasetSnapshotID,
				ContextEpoch:                   securityContext.ContextEpoch,
				ContextDigest:                  securityContext.ContextDigest,
				CaseBindingHash:                securityContext.CaseBindingHash,
				ExpectedProducerContentID:      descriptor.FundsProducerContentID,
				ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
				DuckDBContentSnapshotDigest:    descriptor.DuckDBContentSnapshotDigest,
				DuckDBSnapshotManifestSHA256:   descriptor.DuckDBSnapshotManifestSHA256,
				MaterializationIdentity:        descriptor.MaterializationIdentity,
				SourceSignature: strings.TrimPrefix(
					descriptor.MaterializationIdentity,
					domainsecurity.FundsMaterializationIdentityPrefixV1,
				),
				ResultSignature:        domainsecurity.SHA256Hex([]byte("test-account-ingress-result:\x00" + descriptor.DescriptorDigest)),
				ProducerContentID:      descriptor.FundsProducerContentID,
				ProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
				QueryContract:          domainnative.AccountIngressResolutionQueryContractV1,
				QuerySQLHash:           domainnative.AccountIngressResolutionQuerySQLHashV1,
			},
		},
	)
}

func accountIngressDescriptorInputV1ForTest(
	descriptor domainfundsquerysource.DescriptorV1,
) domainfundsquerysource.DescriptorInputV1 {
	return domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:     descriptor.SnapshotRecordDigest,
		DatasetSnapshotID:        descriptor.DatasetSnapshotID,
		SourceManifestHash:       descriptor.SourceManifestHash,
		CaseID:                   descriptor.CaseID,
		CaseBindingHash:          descriptor.CaseBindingHash,
		DatasetBindingDigest:     descriptor.DatasetBindingDigest,
		BindingObservationDigest: descriptor.BindingObservationDigest,

		FundsProducerContentID:                 descriptor.FundsProducerContentID,
		FundsProducerContentManifestSHA256:     descriptor.FundsProducerContentManifestSHA256,
		FundsProducerContentManifestByteLength: descriptor.FundsProducerContentManifestByteLength,

		DuckDBSHA256:                 descriptor.DuckDBSHA256,
		DuckDBByteLength:             descriptor.DuckDBByteLength,
		DuckDBContentSnapshotDigest:  descriptor.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: descriptor.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      descriptor.MaterializationIdentity,
		SchemaDigest:                 descriptor.SchemaDigest,
		DatasetUTCOffsetMinutes:      descriptor.DatasetUTCOffsetMinutes,
		ExpectedCurrency:             descriptor.ExpectedCurrency,
		MinorUnitScale:               descriptor.MinorUnitScale,
		QueryProfileDigest:           descriptor.QueryProfileDigest,
	}
}

type trackingCurrentDatasetAuthorityV1 struct {
	delegate *currentdatasettest.Harness
	active   atomic.Bool
	calls    atomic.Uint32
}

func (authority *trackingCurrentDatasetAuthorityV1) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if authority == nil || authority.delegate == nil || use == nil ||
		!authority.active.CompareAndSwap(false, true) {
		return errors.New("test dataset authority nested lease")
	}
	authority.calls.Add(1)
	defer authority.active.Store(false)
	return authority.delegate.WithCurrentSelectionV2(ctx, input, securityContext, use)
}

type retainedBindingAuthorityStubV1 struct {
	current       *currentdatasettest.Harness
	selection     datasetsnapshotport.RetainedSelectionV2
	callbackCalls atomic.Uint32
}

func (authority *retainedBindingAuthorityStubV1) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	return authority.current.WithCurrentSelectionV2(ctx, input, securityContext, use)
}

func (authority *retainedBindingAuthorityStubV1) WithRetainedSelectionV2(
	ctx context.Context,
	_ datasetsnapshotport.RetainedSelectionInputV2,
	use func(context.Context, datasetsnapshotport.RetainedSelectionV2) error,
) error {
	if authority == nil || authority.current == nil || use == nil {
		return ErrPrivateStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	authority.callbackCalls.Add(1)
	return use(ctx, authority.selection)
}

func useAccountIngressPrivateReferenceV1(
	t *testing.T,
	compiled AccountIngressCompilationV1,
) PrivateRecordReferenceV1 {
	t.Helper()
	var reference PrivateRecordReferenceV1
	if err := compiled.UsePrivateRecordReferenceV1(func(current PrivateRecordReferenceV1) error {
		reference = current
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return reference
}

func useAccountIngressProviderProjectionV1(
	t *testing.T,
	compiled AccountIngressCompilationV1,
) string {
	t.Helper()
	var providerText string
	if err := compiled.useProviderProjectionV1(func(current string) error {
		providerText = current
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return providerText
}

type providerIngressDescriptorTestV1 struct {
	Ordinal              uint32
	Alias                domaincaseentity.ModelEntityAliasV1
	EntityType           string
	FinancialAccountType string
	BankInstitution      string
	AccountType          string
}

func useAccountIngressProviderProjectionWithDescriptorsV1(
	t *testing.T,
	compiled AccountIngressCompilationV1,
) (string, []providerIngressDescriptorTestV1) {
	t.Helper()
	var providerText string
	descriptors := []providerIngressDescriptorTestV1{}
	err := compiled.UseProviderProjectionV1(func(projection ProviderIngressProjectionV1) error {
		return projection.UseExactWithDescriptorsV1(func(
			currentText string,
			descriptorCount uint32,
			useDescriptors ProviderIngressDescriptorsUseV1,
		) error {
			providerText = currentText
			if descriptorCount == 0 {
				return errors.New("compiled provider projection omitted entity descriptors")
			}
			return useDescriptors(func(
				ordinal uint32,
				alias domaincaseentity.ModelEntityAliasV1,
				entityType string,
				financialAccountType string,
				bankInstitution string,
				accountType string,
			) error {
				descriptors = append(descriptors, providerIngressDescriptorTestV1{
					Ordinal: ordinal, Alias: alias, EntityType: entityType,
					FinancialAccountType: financialAccountType,
					BankInstitution:      bankInstitution,
					AccountType:          accountType,
				})
				return nil
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return providerText, descriptors
}

func TestVerifiedBindingUseRequiresPersistentHMACRevalidationAndStaysCallbackScoped(t *testing.T) {
	ctx := context.Background()
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-bind", TurnID: "turn-bind", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-a")}
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(digester, store)
	const exact = "6222021234567890123"
	reference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		exact,
	))
	if err != nil {
		t.Fatal(err)
	}
	useCalls := 0
	var resolvedExact string
	var resolvedDigest string
	err = service.UseVerifiedBindingByReferenceV1(
		ctx,
		ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: securityContext,
			Reference:       reference,
		},
		func(canonicalValue string, recordDigest string) error {
			useCalls++
			resolvedExact = canonicalValue
			resolvedDigest = recordDigest
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if useCalls != 1 || resolvedExact != exact || !domainsecurity.IsSHA256Hex(resolvedDigest) {
		t.Fatalf("verified private use mismatch: calls=%d exact=%q digest=%q", useCalls, resolvedExact, resolvedDigest)
	}

	// Stable entity identity is case-scoped rather than snapshot, thread, turn,
	// or epoch scoped. A newly authorized context for the same case binding can
	// re-resolve the private record without changing the entity reference.
	evolvedContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-after-restart", TurnID: "turn-new-snapshot", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 2,
	})
	evolvedCalls := 0
	err = service.UseVerifiedBindingByReferenceV1(
		ctx,
		ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: evolvedContext,
			Reference:       reference,
		},
		func(value string, digest string) error {
			evolvedCalls++
			if value != exact || digest != resolvedDigest {
				t.Fatal("same-case resolution changed private identity material")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("same-case snapshot evolution lost the stable entity: %v", err)
	}
	if evolvedCalls != 1 {
		t.Fatalf("same-case evolved private use failed: calls=%d", evolvedCalls)
	}

	otherCase := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-other", TurnID: "turn-other", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-b", CaseBindingHash: strings.Repeat("b", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 1,
	})
	crossCaseCalls := 0
	if err := service.UseVerifiedBindingByReferenceV1(
		ctx,
		ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: otherCase,
			Reference:       reference,
		},
		func(string, string) error {
			crossCaseCalls++
			return nil
		},
	); !errors.Is(err, ErrReferenceNotFound) || crossCaseCalls != 0 {
		t.Fatalf("cross-case private use survived: calls=%d err=%v", crossCaseCalls, err)
	}

	// A structurally valid domain record can carry an arbitrary canonical value
	// under the same reference. Persistence plus installation-keyed revalidation
	// must reject that forgery before private bytes reach the callback.
	bindingKey, err := domaincaseentity.CaseEntityBindingLookupKeyV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
	)
	if err != nil {
		t.Fatal(err)
	}
	original := store.bindings[bindingKey]
	forged, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(domaincaseentity.NewCaseEntityBindingRecordInputV1(
			securityContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			reference,
			"6217009876543210987",
		), original.StableOrdinal),
	)
	if err != nil {
		t.Fatal(err)
	}
	store.bindings[forged.BindingKey] = forged
	forgedCalls := 0
	if err := service.UseVerifiedBindingByReferenceV1(
		ctx,
		ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: securityContext,
			Reference:       reference,
		},
		func(string, string) error {
			forgedCalls++
			return nil
		},
	); !errors.Is(err, ErrPrivateStateIntegrity) || forgedCalls != 0 {
		t.Fatalf("structurally valid forged binding reached callback: calls=%d err=%v", forgedCalls, err)
	}
	store.bindings[forged.BindingKey] = original
	wrongKeyCalls := 0
	if err := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("different-installation-key")},
		store,
	).UseVerifiedBindingByReferenceV1(
		ctx,
		ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: securityContext,
			Reference:       reference,
		},
		func(string, string) error {
			wrongKeyCalls++
			return nil
		},
	); !errors.Is(err, ErrPrivateStateIntegrity) || wrongKeyCalls != 0 {
		t.Fatalf("wrong installation key reached callback: calls=%d err=%v", wrongKeyCalls, err)
	}
}

func TestRetainedBindingUseReadsHistoricalValueWithCurrentSnapshotAuthority(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	workspace := "/cases/retained-binding"
	bindingHash := strings.Repeat("a", 64)
	active, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-retained", TurnID: "turn-current", WorkspaceRealPath: workspace,
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-retained", CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("retained-current"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("retained-current-manifest")),
		ContextEpoch:       2, IssuedAt: time.Unix(1_700_000_002, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	historical, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-retained", TurnID: "turn-historical", WorkspaceRealPath: workspace,
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-retained", CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("retained-historical"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("retained-historical-manifest")),
		ContextEpoch:       1, IssuedAt: time.Unix(1_700_000_001, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-retained")}
	store := newCaseEntityMemoryStoreV1()
	authority := &retainedBindingAuthorityStubV1{current: harness}
	service := NewPersistentService(digester, store, authority, harness, harness.ValidateCurrent)
	const exact = "6222021234567890123"
	reference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		historical,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		exact,
	))
	if err != nil {
		t.Fatal(err)
	}
	authority.selection = datasetsnapshotv2fixture.RetainedSelectionForContextsV2(active, historical)
	var resolved string
	err = service.UseRetainedBindingByReferenceV1(
		ctx,
		UseRetainedBindingByReferenceInputV1{
			ActiveSecurityContext: active, HistoricalSecurityContext: historical, Reference: reference,
		},
		func(value, _ string) error {
			resolved = value
			return nil
		},
	)
	if err != nil || resolved != exact || authority.callbackCalls.Load() != 1 {
		t.Fatalf("retained binding did not read historical value: value=%q callbacks=%d err=%v", resolved, authority.callbackCalls.Load(), err)
	}

	wrongCase, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-retained", TurnID: "turn-other-case", WorkspaceRealPath: workspace,
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-other", CaseBindingHash: strings.Repeat("b", 64),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("retained-other-case"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("retained-other-case-manifest")),
		ContextEpoch:       3, IssuedAt: time.Unix(1_700_000_003, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	authority.callbackCalls.Store(0)
	if err := service.UseRetainedBindingByReferenceV1(
		ctx,
		UseRetainedBindingByReferenceInputV1{
			ActiveSecurityContext: wrongCase, HistoricalSecurityContext: historical, Reference: reference,
		},
		func(string, string) error { t.Fatal("scope-mismatched retained binding reached callback"); return nil },
	); !errors.Is(err, ErrPrivateStateUnavailable) || authority.callbackCalls.Load() != 0 {
		t.Fatalf("scope-mismatched retained binding was not rejected: callbacks=%d err=%v", authority.callbackCalls.Load(), err)
	}
}

func TestCaseAcceptedDisplayBindingPersistsAcrossRestartResumeForkAndAutomaticCompactionTransition(t *testing.T) {
	ctx := context.Background()
	const exactSentinel = "6222021234567890123"
	caseBindingHash := strings.Repeat("c", 64)
	origin := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-display-origin", TurnID: "turn-display-origin",
		TenantID: "tenant-display", UserID: "user-display", CaseID: "case-display",
		CaseBindingHash: caseBindingHash, SnapshotSeed: "display-snapshot", ContextEpoch: 1,
	})
	newContext := func(threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
		t.Helper()
		return caseEntityTestContextV1(t, caseEntityTestContextInputV1{
			ThreadID: threadID, TurnID: turnID,
			TenantID: origin.TenantID, UserID: origin.UserID, CaseID: origin.CaseID,
			CaseBindingHash: caseBindingHash, SnapshotSeed: "display-snapshot", ContextEpoch: epoch,
		})
	}
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-display-continuity")}, store,
	)
	reference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		origin,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		exactSentinel,
	))
	if err != nil {
		t.Fatal(err)
	}
	entityBinding, err := service.resolveBindingRecordByReferenceV1(ctx, origin, reference)
	if err != nil {
		t.Fatal(err)
	}
	evidenceReference := "evr_" + domainsecurity.SHA256Hex([]byte("display-continuity-evidence-reference"))
	evidenceState := domaincaseentity.CaseEvidenceStateV1{
		EvidenceReference: evidenceReference,
		EvidenceDigest:    domainsecurity.SHA256Hex([]byte("display-continuity-evidence")),
		DatasetSnapshotID: origin.DatasetSnapshotID,
		Currentness:       domaincaseentity.SnapshotCurrentV1,
	}
	claimState := domaincaseentity.CaseClaimStateV1{
		ClaimReference: "claim_display_continuity", ClaimDigest: domainsecurity.SHA256Hex([]byte("display-continuity-claim")),
		DatasetSnapshotID: origin.DatasetSnapshotID, Currentness: domaincaseentity.SnapshotCurrentV1,
		InvestigationState: domaincaseentity.InvestigationConfirmedV1,
		EvidenceReferences: []string{evidenceReference}, CounterEvidenceReferences: []string{},
	}
	displayBinding, err := domaincaseentity.NewCaseAcceptedDisplayBindingV1(
		domaincaseentity.CaseAcceptedDisplayBindingInputV1{
			CaseBindingHash:  origin.CaseBindingHash,
			OriginalThreadID: origin.ThreadID, OriginalTurnID: origin.TurnID,
			AcceptedFinalDigest: domainsecurity.SHA256Hex([]byte("display-continuity-final")),
			DispositionDigest:   domainsecurity.SHA256Hex([]byte("display-continuity-disposition")),
			FinalGateVersion:    domainevidence.FinalEvidenceGateVersion,
			ContextDigest:       origin.ContextDigest,
			DatasetSnapshotID:   origin.DatasetSnapshotID, ContextEpoch: origin.ContextEpoch,
			EntityReference: reference, EntityBindingDigest: entityBinding.RecordDigest,
			SlotID: "account-slot-1",
			ClaimBindings: []domaincaseentity.CaseAcceptedDisplayClaimBindingV1{{
				ClaimReference: claimState.ClaimReference, ClaimDigest: claimState.ClaimDigest,
			}},
			EvidenceReceiptBindings: []domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1{{
				EvidenceReference: evidenceState.EvidenceReference, EvidenceDigest: evidenceState.EvidenceDigest,
			}},
			Currentness: domaincaseentity.SnapshotCurrentV1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	indexInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: origin, Generation: 1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{
			Reference:     reference,
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: entityBinding.StableOrdinal,
		}},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: origin.DatasetSnapshotID, ContextEpoch: origin.ContextEpoch,
			Currentness: domaincaseentity.SnapshotCurrentV1,
		}},
		Claims: []domaincaseentity.CaseClaimStateV1{claimState}, Evidence: []domaincaseentity.CaseEvidenceStateV1{evidenceState},
		DisplayBindings:        []domaincaseentity.CaseAcceptedDisplayBindingV1{displayBinding},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(indexInput)
	if err != nil || store.PutThreadContextIfAbsent(ctx, index) != nil {
		t.Fatal("persist initial display-binding index")
	}
	providerState, err := providerIngressLongitudinalStateFromIndexV1(index)
	providerBody, providerBodyErr := json.Marshal(providerState)
	if err != nil || providerBodyErr != nil {
		t.Fatalf("project display-binding index for provider: stateErr=%v bodyErr=%v", err, providerBodyErr)
	}
	for _, private := range []string{
		displayBinding.BindingDigest, displayBinding.AcceptedFinalDigest, displayBinding.DispositionDigest,
		displayBinding.OriginalThreadID, displayBinding.OriginalTurnID, string(displayBinding.EntityReference),
		displayBinding.ClaimBindings[0].ClaimReference,
		displayBinding.EvidenceReceiptBindings[0].EvidenceReference,
		exactSentinel, "authorityEntityRef", "sourceFileId", "sourceRowNumber",
	} {
		if bytes.Contains(providerBody, []byte(private)) {
			t.Fatalf("provider longitudinal projection reflected private display binding %q: %s", private, providerBody)
		}
	}

	assertInherited := func(name string, record domaincaseentity.ThreadCaseContextRecord, wantGeneration uint64) {
		t.Helper()
		if record.Generation != wantGeneration || len(record.DisplayBindings) != 1 ||
			!reflect.DeepEqual(record.DisplayBindings[0], displayBinding) {
			t.Fatalf("%s did not inherit exact display binding: %#v", name, record)
		}
		body, bodyErr := domaincaseentity.ThreadCaseContextRecordV1Bytes(record)
		if bodyErr != nil || strings.Contains(string(body), exactSentinel) || strings.Contains(string(body), "sourceFileId") ||
			strings.Contains(string(body), "sourceRowNumber") || strings.Contains(string(body), "authorityEntityRef") {
			t.Fatalf("%s serialized source-exact/private locator material: body=%s err=%v", name, body, bodyErr)
		}
	}

	created, err := service.persistCurrentThreadContinuityExactV1(
		ctx, origin, []domaincaseentity.ReferenceV1{reference}, index, CaseContinuityIndependentV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInherited("initial thread", created, 1)
	restartTransition, current, err := service.classifyCaseContinuityTransitionExactV1(
		ctx, origin, "", "", []domaincaseentity.ReferenceV1{reference},
	)
	if err != nil || restartTransition != CaseContinuityRestartV1 || current.RecordDigest != created.RecordDigest {
		t.Fatalf("restart classification changed: transition=%s current=%#v err=%v", restartTransition, current, err)
	}
	restarted, err := service.persistCurrentThreadContinuityExactV1(
		ctx, origin, []domaincaseentity.ReferenceV1{reference}, index, restartTransition,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInherited("restart", restarted, 1)

	for _, inherited := range []struct {
		name, threadID, relation string
		transition               CaseContinuityTransitionV1
	}{
		{name: "resume", threadID: "thread-display-resume", relation: "primary", transition: CaseContinuityResumeV1},
		{name: "authorized fork", threadID: "thread-display-fork", relation: "fork", transition: CaseContinuityForkV1},
	} {
		t.Run(inherited.name, func(t *testing.T) {
			active := newContext(inherited.threadID, "turn-"+inherited.threadID, origin.ContextEpoch)
			transition, current, classifyErr := service.classifyCaseContinuityTransitionExactV1(
				ctx, active, inherited.relation, origin.ThreadID, []domaincaseentity.ReferenceV1{reference},
			)
			if classifyErr != nil || transition != inherited.transition || current.Generation != 0 {
				t.Fatalf("classification changed: transition=%s current=%#v err=%v", transition, current, classifyErr)
			}
			record, persistErr := service.persistCurrentThreadContinuityExactV1(
				ctx, active, []domaincaseentity.ReferenceV1{reference}, index, transition,
			)
			if persistErr != nil {
				t.Fatal(persistErr)
			}
			assertInherited(inherited.name, record, 1)
		})
	}

	compactedContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: origin.ThreadID, TurnID: "turn-display-compacted", WorkspaceRealPath: origin.WorkspaceRealPath,
		TenantID: origin.TenantID, UserID: origin.UserID, CaseID: origin.CaseID, CaseBindingHash: origin.CaseBindingHash,
		DatasetSnapshotID: origin.DatasetSnapshotID, SourceManifestHash: origin.SourceManifestHash,
		ContextEpoch: origin.ContextEpoch + 1, IssuedAt: time.Unix(1_700_000_100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Production automatic compaction advances this same thread-owned context
	// epoch before the next case ingress reaches the longitudinal owner.
	snapshots, claims, evidence, continuations, displayBindings, err := evolveCaseLongitudinalCurrentnessV1(index, compactedContext)
	if err != nil {
		t.Fatal(err)
	}
	compactedIndex, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(
		domaincaseentity.ThreadCaseContextRecordInputV1{
			SecurityContext: compactedContext, Generation: 2, PreviousRecordDigest: index.RecordDigest,
			EntityReferences: index.EntityReferences, EntityIdentities: index.EntityIdentities,
			Snapshots: snapshots, Claims: claims, Evidence: evidence, Continuations: continuations,
			DisplayBindings:        displayBindings,
			OpenQuestionReferences: index.OpenQuestionReferences, DataGapReferences: index.DataGapReferences,
		},
	)
	if err != nil || store.PutThreadContextIfAbsent(ctx, compactedIndex) != nil {
		t.Fatal("persist compacted display-binding index")
	}
	compactionTransition, current, err := service.classifyCaseContinuityTransitionExactV1(
		ctx, compactedContext, "", "", []domaincaseentity.ReferenceV1{reference},
	)
	if err != nil || compactionTransition != CaseContinuityCompactionV1 || current.RecordDigest != restarted.RecordDigest {
		t.Fatalf("compaction classification changed: transition=%s current=%#v err=%v", compactionTransition, current, err)
	}
	compacted, err := service.persistCurrentThreadContinuityExactV1(
		ctx, compactedContext, []domaincaseentity.ReferenceV1{reference}, compactedIndex, compactionTransition,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInherited("automatic compaction transition", compacted, 2)
}

func TestLegacyLongitudinalRestartDoesNotBackfillAbsentDisplayBindings(t *testing.T) {
	ctx := context.Background()
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-legacy-display-absent", TurnID: "turn-legacy-display-absent",
		TenantID: "tenant-legacy", UserID: "user-legacy", CaseID: "case-legacy",
		CaseBindingHash: strings.Repeat("d", 64), SnapshotSeed: "legacy-display-absent", ContextEpoch: 1,
	})
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	input := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: 1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{
			Reference:     reference,
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1,
		}},
		Snapshots: []domaincaseentity.CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID, ContextEpoch: securityContext.ContextEpoch,
			Currentness: domaincaseentity.SnapshotCurrentV1,
		}},
		Claims: []domaincaseentity.CaseClaimStateV1{}, Evidence: []domaincaseentity.CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(input)
	if err != nil {
		t.Fatal(err)
	}
	threadInput := input
	threadInput.EntityIdentities = nil
	thread, err := domaincaseentity.NewThreadCaseContextRecordV1(threadInput)
	if err != nil || index.DisplayBindings != nil || thread.DisplayBindings != nil {
		t.Fatalf("legacy owner acquired an implicit display-binding child: index=%#v thread=%#v err=%v", index, thread, err)
	}
	store := newCaseEntityMemoryStoreV1()
	if err := store.PutThreadContextIfAbsent(ctx, index); err != nil {
		t.Fatal(err)
	}
	if err := store.PutThreadContextIfAbsent(ctx, thread); err != nil {
		t.Fatal(err)
	}
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("legacy-display-absent-key")}, store,
	)
	restarted, err := service.persistCurrentThreadContinuityExactV1(
		ctx, securityContext, []domaincaseentity.ReferenceV1{reference}, index, CaseContinuityRestartV1,
	)
	if err != nil || restarted.RecordDigest != thread.RecordDigest || restarted.Generation != 1 ||
		restarted.DisplayBindings != nil {
		t.Fatalf("legacy restart rewrote or backfilled display bindings: restarted=%#v err=%v", restarted, err)
	}
}

func TestDisplayLabelUseKeepsStableCaseIdentityAndOnlyReturnsNaturalSafeText(t *testing.T) {
	ctx := context.Background()
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-display", TurnID: "turn-display", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	store := newCaseEntityMemoryStoreV1()
	service := newPersistentCaseEntityServiceV1(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		store,
	)
	const firstExact = "6222021234567890123"
	firstReference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		firstExact,
	))
	if err != nil {
		t.Fatal(err)
	}
	secondReference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
		"6217009876543210123",
	))
	if err != nil {
		t.Fatal(err)
	}

	useLabel := func(reference domaincaseentity.ReferenceV1, semantic DisplayLabelSemanticV1) domaincaseentity.DisplayLabelV1 {
		t.Helper()
		var label domaincaseentity.DisplayLabelV1
		calls := 0
		err := service.UseDisplayLabelV1(ctx, UseDisplayLabelInputV1{
			SecurityContext: securityContext,
			Reference:       reference,
			Semantic:        semantic,
		}, func(current domaincaseentity.DisplayLabelV1) error {
			calls++
			label = current
			return nil
		})
		if err != nil || calls != 1 || domaincaseentity.ValidateDisplayLabelV1(label) != nil {
			t.Fatalf("display label callback mismatch: label=%#v calls=%d err=%v", label, calls, err)
		}
		if strings.Contains(label.Text, firstExact) || strings.Contains(label.Text, string(reference)) ||
			domaincaseentity.ContainsReferenceV1(label.Text) {
			t.Fatal("display label exposed source-exact or internal identity")
		}
		return label
	}

	first := useLabel(firstReference, DisplayLabelSemanticV1{
		Institution: "中国银行",
		AccountType: "储蓄账户",
	})
	second := useLabel(secondReference, DisplayLabelSemanticV1{
		Institution: "中国银行",
		AccountType: "借记卡",
	})
	if first.StableOrdinal != 1 || first.SafeSuffix != "0123" ||
		first.Text != "第1个银行账户（中国银行；储蓄账户；尾号0123）" {
		t.Fatalf("first natural label mismatch: %#v", first)
	}
	if second.StableOrdinal != 2 || second.SafeSuffix != "0123" ||
		second.Text != "第2个银行卡（中国银行；借记卡；尾号0123）" || second.Text == first.Text {
		t.Fatalf("same-suffix identities collapsed: first=%#v second=%#v", first, second)
	}

	evolvedContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-after-restart", TurnID: "turn-new-snapshot", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-b", ContextEpoch: 2,
	})
	var evolved domaincaseentity.DisplayLabelV1
	err = service.UseDisplayLabelV1(ctx, UseDisplayLabelInputV1{
		SecurityContext: evolvedContext,
		Reference:       firstReference,
		Semantic: DisplayLabelSemanticV1{
			Institution: "中国银行",
			AccountType: "已销户账户",
		},
	}, func(label domaincaseentity.DisplayLabelV1) error {
		evolved = label
		return nil
	})
	if err != nil || evolved.StableOrdinal != first.StableOrdinal ||
		evolved.SafeSuffix != first.SafeSuffix || evolved.AccountType != "已销户账户" {
		t.Fatalf("snapshot evolution changed identity instead of semantics: first=%#v evolved=%#v err=%v", first, evolved, err)
	}

	invalidCalls := 0
	err = service.UseDisplayLabelV1(ctx, UseDisplayLabelInputV1{
		SecurityContext: securityContext,
		Reference:       firstReference,
		Semantic:        DisplayLabelSemanticV1{Institution: firstExact},
	}, func(domaincaseentity.DisplayLabelV1) error {
		invalidCalls++
		return nil
	})
	if !errors.Is(err, ErrPrivateStateIntegrity) || invalidCalls != 0 || strings.Contains(err.Error(), firstExact) {
		t.Fatalf("unsafe display semantic reached callback: calls=%d err=%v", invalidCalls, err)
	}
}

func TestAccountFlowCounterpartyUseIsStableCaseScopedAndDoesNotAcquireNestedDatasetAuthority(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	newContext := func(threadID, turnID, caseID, bindingHash, snapshotSeed string, epoch uint64) domainsecurity.TurnSecurityContext {
		t.Helper()
		securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/current",
			TenantID: "tenant-a", UserID: "user-a", CaseID: caseID, CaseBindingHash: bindingHash,
			DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(snapshotSeed),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + snapshotSeed)),
			ContextEpoch:       epoch, IssuedAt: time.Unix(1_700_100_000+int64(epoch), 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return securityContext
	}
	firstContext := newContext(
		"thread-counterparty", "turn-counterparty-a", "case-a", strings.Repeat("a", 64), "snapshot-a", 1,
	)
	evolvedContext := newContext(
		"thread-counterparty", "turn-counterparty-b", "case-a", strings.Repeat("a", 64), "snapshot-b", 2,
	)
	otherCaseContext := newContext(
		"thread-counterparty-other", "turn-counterparty-other", "case-b", strings.Repeat("b", 64), "snapshot-b", 1,
	)
	nestedAuthority := &accountFlowCounterpartyNoNestedDatasetAuthorityV1{}
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		newCaseEntityMemoryStoreV1(),
		nestedAuthority,
		harness,
		harness.ValidateCurrent,
	)
	const exact = "6217009876543210987"
	use := func(securityContext domainsecurity.TurnSecurityContext) (domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) {
		t.Helper()
		descriptor, err := currentdatasettest.AccountIngressDescriptorV1(securityContext)
		if err != nil {
			t.Fatal(err)
		}
		var reference domaincaseentity.ReferenceV1
		var display domaincaseentity.DisplayLabelV1
		calls := 0
		err = service.UseAccountFlowCounterpartyV1(
			ctx,
			NewUseAccountFlowCounterpartyInputV1(
				securityContext,
				descriptor,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				exact,
				DisplayLabelSemanticV1{
					Institution: "Analytix Test Bank",
					AccountType: domainnative.AccountFlowCounterpartyAccountTypeV1,
				},
				harness.ValidateCurrent,
			),
			func(currentReference domaincaseentity.ReferenceV1, currentDisplay domaincaseentity.DisplayLabelV1) error {
				calls++
				reference = currentReference
				display = currentDisplay
				return nil
			},
		)
		if err != nil || calls != 1 {
			t.Fatalf("counterparty use failed: calls=%d err=%v", calls, err)
		}
		return reference, display
	}

	firstReference, firstDisplay := use(firstContext)
	evolvedReference, evolvedDisplay := use(evolvedContext)
	otherReference, otherDisplay := use(otherCaseContext)
	if firstReference != evolvedReference || firstDisplay != evolvedDisplay ||
		firstDisplay.StableOrdinal != 1 || firstDisplay.SafeSuffix != "0987" ||
		firstDisplay.Institution != "Analytix Test Bank" ||
		firstDisplay.AccountType != domainnative.AccountFlowCounterpartyAccountTypeV1 ||
		strings.Contains(firstDisplay.Text, exact) || strings.Contains(firstDisplay.Text, string(firstReference)) {
		t.Fatalf("same-case snapshot evolution drifted counterparty identity: first=%q/%#v evolved=%q/%#v", firstReference, firstDisplay, evolvedReference, evolvedDisplay)
	}
	if otherReference == firstReference || otherDisplay.StableOrdinal != 1 {
		t.Fatalf("cross-case counterparty identity was linked: first=%q other=%q/%#v", firstReference, otherReference, otherDisplay)
	}
	if nestedAuthority.calls.Load() != 0 {
		t.Fatalf("counterparty path acquired nested DSV2 authority: calls=%d", nestedAuthority.calls.Load())
	}

	descriptor, err := currentdatasettest.AccountIngressDescriptorV1(firstContext)
	if err != nil {
		t.Fatal(err)
	}
	unsafeCalls := 0
	err = service.UseAccountFlowCounterpartyV1(
		ctx,
		NewUseAccountFlowCounterpartyInputV1(
			firstContext,
			descriptor,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			exact,
			DisplayLabelSemanticV1{
				Institution: exact,
				AccountType: domainnative.AccountFlowCounterpartyAccountTypeV1,
			},
			harness.ValidateCurrent,
		),
		func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error {
			unsafeCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrInvalidReferenceInput) || unsafeCalls != 0 {
		t.Fatalf("unsafe counterparty semantic reached callback: calls=%d err=%v", unsafeCalls, err)
	}
	mismatchedDescriptor, err := currentdatasettest.AccountIngressDescriptorV1(evolvedContext)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedCalls := 0
	err = service.UseAccountFlowCounterpartyV1(
		ctx,
		NewUseAccountFlowCounterpartyInputV1(
			firstContext,
			mismatchedDescriptor,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			exact,
			DisplayLabelSemanticV1{
				Institution: "Analytix Test Bank",
				AccountType: domainnative.AccountFlowCounterpartyAccountTypeV1,
			},
			harness.ValidateCurrent,
		),
		func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error {
			mismatchedCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrPrivateStateUnavailable) || mismatchedCalls != 0 {
		t.Fatalf("mismatched source descriptor reached callback: calls=%d err=%v", mismatchedCalls, err)
	}
}

func TestAccountFlowSubjectUseBindsCurrentDescriptorWithoutNestedDatasetAuthority(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-subject", TurnID: "turn-subject", WorkspaceRealPath: "/cases/subject",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-subject"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:snapshot-subject")),
		ContextEpoch:       1, IssuedAt: time.Unix(1_700_200_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-subject")}
	store := newCaseEntityMemoryStoreV1()
	setup := NewPersistentService(digester, store, harness, harness, harness.ValidateCurrent)
	const exact = "6222021234567890123"
	wantReference, err := setup.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		exact,
	))
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := currentdatasettest.AccountIngressDescriptorV1(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	nestedAuthority := &accountFlowCounterpartyNoNestedDatasetAuthorityV1{}
	service := NewPersistentService(digester, store, nestedAuthority, harness, harness.ValidateCurrent)

	var reference domaincaseentity.ReferenceV1
	var canonicalValue, recordDigest string
	calls := 0
	err = service.UseAccountFlowSubjectV1(
		ctx,
		NewUseAccountFlowSubjectInputV1(securityContext, descriptor, "acct:1", harness.ValidateCurrent),
		func(currentReference domaincaseentity.ReferenceV1, currentValue, currentDigest string) error {
			calls++
			reference, canonicalValue, recordDigest = currentReference, currentValue, currentDigest
			return nil
		},
	)
	if err != nil || calls != 1 || reference != wantReference || canonicalValue != exact || recordDigest == "" {
		t.Fatalf(
			"subject use failed: calls=%d reference=%q canonical=%q digest=%q err=%v",
			calls, reference, canonicalValue, recordDigest, err,
		)
	}
	if nestedAuthority.calls.Load() != 0 {
		t.Fatalf("subject path acquired nested DSV2 authority: calls=%d", nestedAuthority.calls.Load())
	}

	mismatchedContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-subject", TurnID: "turn-subject-mismatch", WorkspaceRealPath: "/cases/subject",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-subject-mismatch"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:snapshot-subject-mismatch")),
		ContextEpoch:       2, IssuedAt: time.Unix(1_700_200_001, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	mismatchedDescriptor, err := currentdatasettest.AccountIngressDescriptorV1(mismatchedContext)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedCalls := 0
	err = service.UseAccountFlowSubjectV1(
		ctx,
		NewUseAccountFlowSubjectInputV1(
			securityContext,
			mismatchedDescriptor,
			"acct:1",
			harness.ValidateCurrent,
		),
		func(domaincaseentity.ReferenceV1, string, string) error {
			mismatchedCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrPrivateStateUnavailable) || mismatchedCalls != 0 || nestedAuthority.calls.Load() != 0 {
		t.Fatalf(
			"mismatched subject descriptor reached callback: calls=%d nested=%d err=%v",
			mismatchedCalls, nestedAuthority.calls.Load(), err,
		)
	}
}

func TestAccountFlowSubjectUseFailsClosedOnPreAndPostCurrentnessRevocation(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-subject-revoked", TurnID: "turn-subject-revoked", WorkspaceRealPath: "/cases/subject-revoked",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-subject-revoked"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:snapshot-subject-revoked")),
		ContextEpoch:       1, IssuedAt: time.Unix(1_700_300_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	digester := &recordingKeyedDigesterV1{key: []byte("installation-key-subject-revoked")}
	store := newCaseEntityMemoryStoreV1()
	setup := NewPersistentService(digester, store, harness, harness, harness.ValidateCurrent)
	if _, err := setup.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		"6222021234567890123",
	)); err != nil {
		t.Fatal(err)
	}
	descriptor, err := currentdatasettest.AccountIngressDescriptorV1(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	nestedAuthority := &accountFlowCounterpartyNoNestedDatasetAuthorityV1{}

	harness.SetRevoked(true)
	preCalls := 0
	service := NewPersistentService(digester, store, nestedAuthority, harness, harness.ValidateCurrent)
	err = service.UseAccountFlowSubjectV1(
		ctx,
		NewUseAccountFlowSubjectInputV1(securityContext, descriptor, "acct:1", harness.ValidateCurrent),
		func(domaincaseentity.ReferenceV1, string, string) error {
			preCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrPrivateStateUnavailable) || preCalls != 0 || nestedAuthority.calls.Load() != 0 {
		t.Fatalf("pre-revoked subject reached callback: calls=%d nested=%d err=%v", preCalls, nestedAuthority.calls.Load(), err)
	}

	harness.SetRevoked(false)
	var validationCalls atomic.Uint32
	postValidator := func(currentCtx context.Context, current domainsecurity.TurnSecurityContext) error {
		if validationCalls.Add(1) == 2 {
			harness.SetRevoked(true)
		}
		return harness.ValidateCurrent(currentCtx, current)
	}
	service = NewPersistentService(digester, store, nestedAuthority, harness, postValidator)
	postCalls := 0
	err = service.UseAccountFlowSubjectV1(
		ctx,
		NewUseAccountFlowSubjectInputV1(securityContext, descriptor, "acct:1", postValidator),
		func(domaincaseentity.ReferenceV1, string, string) error {
			postCalls++
			return nil
		},
	)
	if !errors.Is(err, ErrPrivateStateUnavailable) || postCalls != 1 || validationCalls.Load() != 2 ||
		nestedAuthority.calls.Load() != 0 {
		t.Fatalf(
			"post-revoked subject was accepted: calls=%d validations=%d nested=%d err=%v",
			postCalls, validationCalls.Load(), nestedAuthority.calls.Load(), err,
		)
	}
}

type accountFlowCounterpartyNoNestedDatasetAuthorityV1 struct {
	calls atomic.Uint32
}

func (authority *accountFlowCounterpartyNoNestedDatasetAuthorityV1) WithCurrentSelectionV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
	domainsecurity.TurnSecurityContext,
	func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	authority.calls.Add(1)
	return errors.New("nested dataset authority must not be acquired")
}

func TestVerifiedBindingUseFailsClosedAcrossCurrentAuthorityRevocation(t *testing.T) {
	ctx := context.Background()
	harness := currentdatasettest.NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-revocation", TurnID: "turn-revocation", WorkspaceRealPath: "/cases/revocation",
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash:   strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-revocation"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("manifest:snapshot-revocation"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_001, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewPersistentService(
		&recordingKeyedDigesterV1{key: []byte("installation-key-a")},
		newCaseEntityMemoryStoreV1(),
		harness,
		harness,
		harness.ValidateCurrent,
	)
	const exact = "6222021234567890123"
	reference, err := service.BindReferenceV1(ctx, NewDeriveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		exact,
	))
	if err != nil {
		t.Fatal(err)
	}
	input := ResolveVerifiedBindingByReferenceInputV1{
		SecurityContext: securityContext,
		Reference:       reference,
	}

	harness.SetRevoked(true)
	preUseCalls := 0
	if err := service.UseVerifiedBindingByReferenceV1(
		ctx,
		input,
		func(string, string) error {
			preUseCalls++
			return nil
		},
	); !errors.Is(err, ErrPrivateStateUnavailable) || preUseCalls != 0 {
		t.Fatalf("pre-use revocation reached private callback: calls=%d err=%v", preUseCalls, err)
	}

	harness.SetRevoked(false)
	postUseCalls := 0
	if err := service.UseVerifiedBindingByReferenceV1(
		ctx,
		input,
		func(value string, digest string) error {
			postUseCalls++
			if value != exact || !domainsecurity.IsSHA256Hex(digest) {
				t.Fatal("private callback received invalid verified binding")
			}
			harness.SetRevoked(true)
			return nil
		},
	); !errors.Is(err, ErrPrivateStateUnavailable) || postUseCalls != 1 {
		t.Fatalf("post-use revocation completed successfully: calls=%d err=%v", postUseCalls, err)
	}

	// Reauthorization resumes the same additive service; it does not require a
	// different Agent, thread, or private-state implementation.
	harness.SetRevoked(false)
	reauthorizedCalls := 0
	if err := service.UseVerifiedBindingByReferenceV1(
		ctx,
		input,
		func(value string, digest string) error {
			reauthorizedCalls++
			if value != exact || !domainsecurity.IsSHA256Hex(digest) {
				t.Fatal("reauthorized private callback received invalid binding")
			}
			return nil
		},
	); err != nil || reauthorizedCalls != 1 {
		t.Fatalf("reauthorization did not resume private use: calls=%d err=%v", reauthorizedCalls, err)
	}
}

func TestReferenceV1SuppressesDigesterFailuresAndInvalidDigests(t *testing.T) {
	const sourceExactValue = "6222021234567890"
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-digester", TurnID: "turn-digester", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	tests := map[string]*recordingKeyedDigesterV1{
		"failure":        {err: errors.New("private digester detail")},
		"invalid digest": {forcedDigest: strings.Repeat("A", 64)},
	}
	for name, digester := range tests {
		t.Run(name, func(t *testing.T) {
			reference, err := NewService(digester).DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
				securityContext,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				sourceExactValue,
			))
			if !errors.Is(err, ErrReferenceDerivation) || reference != "" {
				t.Fatalf("digester failure minted a reference: reference=%q err=%v", reference, err)
			}
			if strings.Contains(err.Error(), sourceExactValue) ||
				strings.Contains(err.Error(), "private digester detail") {
				t.Fatal("reference derivation error exposed private input or adapter detail")
			}
		})
	}
}

func TestResolveReferenceExactV1RequiresExactlyOneDistinctCanonicalPreimage(t *testing.T) {
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-resolve", TurnID: "turn-resolve", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	constantDigester := &recordingKeyedDigesterV1{forcedDigest: strings.Repeat("0", 64)}
	service := NewService(constantDigester)
	reference := deriveCaseEntityReferenceTestV1(
		t, service, securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
	)

	resolved, err := service.resolveReferenceExactV1(context.Background(), NewResolveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		[]string{
			"6222-0212 3456-7890",
			"6222021234567890",
		},
	))
	if err != nil || resolved != "6222021234567890" {
		t.Fatalf("equivalent source spellings were not one canonical preimage: err=%v", err)
	}

	resolved, err = service.resolveReferenceExactV1(context.Background(), NewResolveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		[]string{
			"6222021234567890",
			"6217009876543210",
		},
	))
	if !errors.Is(err, ErrReferenceAmbiguous) || resolved != "" {
		t.Fatalf("keyed digest collision did not fail closed: err=%v", err)
	}
	for _, sourceExactValue := range []string{"6222021234567890", "6217009876543210"} {
		if strings.Contains(err.Error(), sourceExactValue) {
			t.Fatal("ambiguous resolution error exposed a source-exact value")
		}
	}

	keyedService := NewService(&recordingKeyedDigesterV1{key: []byte("installation-key-a")})
	keyedReference := deriveCaseEntityReferenceTestV1(
		t, keyedService, securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
	)
	if resolved, err := keyedService.resolveReferenceExactV1(context.Background(), NewResolveReferenceInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		keyedReference,
		[]string{"6217009876543210"},
	)); !errors.Is(err, ErrReferenceNotFound) || resolved != "" {
		t.Fatalf("missing canonical preimage did not fail closed: err=%v", err)
	}
}

func deriveCaseEntityReferenceTestV1(
	t *testing.T,
	service *Service,
	securityContext domainsecurity.TurnSecurityContext,
	accountType string,
) domaincaseentity.ReferenceV1 {
	t.Helper()
	reference, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(
		securityContext,
		accountType,
		"6222-0212 3456-7890",
	))
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

type caseEntityTestContextInputV1 struct {
	ThreadID        string
	TurnID          string
	TenantID        string
	UserID          string
	CaseID          string
	CaseBindingHash string
	SnapshotSeed    string
	ContextEpoch    uint64
}

func caseEntityTestContextV1(
	t *testing.T,
	input caseEntityTestContextInputV1,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := currentdatasettest.NewHarness().NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, WorkspaceRealPath: "/cases/current",
		TenantID: input.TenantID, UserID: input.UserID, CaseID: input.CaseID, CaseBindingHash: input.CaseBindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(input.SnapshotSeed),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + input.SnapshotSeed)),
		ContextEpoch:       input.ContextEpoch, IssuedAt: time.Unix(1_700_000_000+int64(input.ContextEpoch), 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func newPersistentCaseEntityServiceV1(
	digester KeyedPayloadDigester,
	store caseentityport.Store,
) *Service {
	harness := currentdatasettest.NewHarness()
	return NewPersistentService(digester, store, harness, harness, harness.ValidateCurrent)
}

type keyedDigestCallV1 struct {
	purpose string
	payload []byte
}

type recordingKeyedDigesterV1 struct {
	key          []byte
	forcedDigest string
	err          error
	calls        []keyedDigestCallV1
}

type caseEntityMemoryStoreV1 struct {
	bindings       map[string]domaincaseentity.CaseEntityBindingRecord
	ingress        map[string]domaincaseentity.CaseIngressRecord
	threadContext  map[string]domaincaseentity.ThreadCaseContextRecord
	putBindingErr  error
	putBindingHook func(domaincaseentity.CaseEntityBindingRecord)
	putIngressErr  error
	putIngressHook func()
}

func newCaseEntityMemoryStoreV1() *caseEntityMemoryStoreV1 {
	return &caseEntityMemoryStoreV1{
		bindings:      make(map[string]domaincaseentity.CaseEntityBindingRecord),
		ingress:       make(map[string]domaincaseentity.CaseIngressRecord),
		threadContext: make(map[string]domaincaseentity.ThreadCaseContextRecord),
	}
}

func (store *caseEntityMemoryStoreV1) EnsureBinding(
	_ context.Context,
	input domaincaseentity.CaseEntityBindingRecordInputV1,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	key, err := domaincaseentity.CaseEntityBindingLookupKeyV1(input.SecurityContext, input.EntityType, input.Reference)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	if current, found := store.bindings[key]; found {
		expected, expectedErr := domaincaseentity.NewCaseEntityBindingRecordV1(
			domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, current.StableOrdinal),
		)
		if expectedErr == nil && current.RecordDigest == expected.RecordDigest {
			return current, nil
		}
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrConflict
	}
	var maxOrdinal uint32
	for _, current := range store.bindings {
		if current.TenantID == input.SecurityContext.TenantID && current.UserID == input.SecurityContext.UserID &&
			current.CaseID == input.SecurityContext.CaseID && current.CaseBindingHash == input.SecurityContext.CaseBindingHash &&
			current.StableOrdinal > maxOrdinal {
			maxOrdinal = current.StableOrdinal
		}
	}
	record, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, maxOrdinal+1),
	)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	if store.putBindingHook != nil {
		store.putBindingHook(record)
	}
	if store.putBindingErr != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, store.putBindingErr
	}
	store.bindings[record.BindingKey] = record
	return record, nil
}

func (store *caseEntityMemoryStoreV1) ResolveBinding(
	_ context.Context,
	key string,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	record, found := store.bindings[key]
	if !found {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *caseEntityMemoryStoreV1) ResolveBindingByStableOrdinal(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	stableOrdinal uint32,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	var resolved domaincaseentity.CaseEntityBindingRecord
	for _, record := range store.bindings {
		if record.TenantID != securityContext.TenantID || record.UserID != securityContext.UserID ||
			record.CaseID != securityContext.CaseID || record.CaseBindingHash != securityContext.CaseBindingHash ||
			record.EntityType != entityType || record.StableOrdinal != stableOrdinal {
			continue
		}
		if resolved.Reference != "" {
			return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
		}
		resolved = record
	}
	if resolved.Reference == "" {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return resolved, nil
}

func (store *caseEntityMemoryStoreV1) PutIngressIfAbsent(
	_ context.Context,
	record domaincaseentity.CaseIngressRecord,
) error {
	if store.putIngressHook != nil {
		store.putIngressHook()
	}
	if store.putIngressErr != nil {
		return store.putIngressErr
	}
	if current, found := store.ingress[record.IngressID]; found {
		if current.RecordDigest == record.RecordDigest {
			return nil
		}
		return caseentityport.ErrConflict
	}
	store.ingress[record.IngressID] = record
	return nil
}

func (store *caseEntityMemoryStoreV1) ResolveIngress(
	_ context.Context,
	key string,
) (domaincaseentity.CaseIngressRecord, error) {
	record, found := store.ingress[key]
	if !found {
		return domaincaseentity.CaseIngressRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *caseEntityMemoryStoreV1) PutThreadContextIfAbsent(
	_ context.Context,
	record domaincaseentity.ThreadCaseContextRecord,
) error {
	if current, found := store.threadContext[record.StorageKey]; found {
		if current.RecordDigest == record.RecordDigest {
			return nil
		}
		return caseentityport.ErrConflict
	}
	store.threadContext[record.StorageKey] = record
	return nil
}

func (store *caseEntityMemoryStoreV1) ResolveThreadContext(
	_ context.Context,
	key string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	record, found := store.threadContext[key]
	if !found {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

func (store *caseEntityMemoryStoreV1) ResolveLatestCaseLongitudinalContext(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.latestThreadContextForScopeV1(securityContext, "", true)
}

func (store *caseEntityMemoryStoreV1) ResolveLatestThreadContextForScope(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.latestThreadContextForScopeV1(securityContext, threadID, false)
}

func (store *caseEntityMemoryStoreV1) latestThreadContextForScopeV1(
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
	caseLongitudinal bool,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	var latest domaincaseentity.ThreadCaseContextRecord
	for _, record := range store.threadContext {
		if record.TenantID != securityContext.TenantID || record.UserID != securityContext.UserID ||
			record.CaseID != securityContext.CaseID || record.CaseBindingHash != securityContext.CaseBindingHash ||
			domaincaseentity.IsCaseLongitudinalIndexRecordV1(record) != caseLongitudinal ||
			(!caseLongitudinal && record.ThreadID != threadID) {
			continue
		}
		if record.Generation > latest.Generation {
			latest = record
		}
	}
	if latest.Generation == 0 {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrNotFound
	}
	return latest, nil
}

func (digester *recordingKeyedDigesterV1) KeyedPayloadHash(
	_ context.Context,
	purpose string,
	payload []byte,
) (string, error) {
	digester.calls = append(digester.calls, keyedDigestCallV1{
		purpose: purpose,
		payload: append([]byte(nil), payload...),
	})
	if digester.err != nil {
		return "", digester.err
	}
	if digester.forcedDigest != "" {
		return digester.forcedDigest, nil
	}
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
