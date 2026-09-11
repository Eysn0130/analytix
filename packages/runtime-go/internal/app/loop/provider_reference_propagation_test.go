package loop

import (
	"context"
	"strings"
	"testing"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestProviderStreamKeepsHostSelectionPrivateAndMasksEveryAuthorityReference(t *testing.T) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-provider-selection", 7)
	reference := domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("e", 64))
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
		securityContext,
		caseentityapp.PrivateRecordReferenceV1{
			RecordID:     domainsecurity.SHA256Hex([]byte("provider-selection-record")),
			RecordDigest: domainsecurity.SHA256Hex([]byte("provider-selection-record-digest")),
		},
		[]domaincaseentity.ReferenceV1{reference},
	)
	if err != nil {
		t.Fatal(err)
	}

	provider := &providerStreamStub{responses: []providerStreamResponse{{}}}
	_, err = StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:            provider,
		SecurityContext:     securityContext,
		HostEntitySelection: selection,
		Request: domainmodel.Request{Messages: []domainmodel.Message{{
			Role: "user", Content: "analyze " + string(reference),
		}}},
	})
	if err != nil || len(provider.requests) != 1 {
		t.Fatalf("host-selected case request did not reach provider: calls=%d err=%v", len(provider.requests), err)
	}
	if sent := provider.requests[0]; strings.Contains(sent.Messages[0].Content, string(reference)) ||
		domaincaseentity.ContainsReferenceCandidateV1(sent.Messages[0].Content) {
		t.Fatalf("host-private authority reference reached provider: %#v", sent.Messages)
	}

	otherContext := hostCaseEntitySelectionContextV1(t, "case-provider-selection-other", 8)
	provider = &providerStreamStub{responses: []providerStreamResponse{{}}}
	_, err = StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:            provider,
		SecurityContext:     otherContext,
		HostEntitySelection: selection,
		Request: domainmodel.Request{Messages: []domainmodel.Message{{
			Role: "user", Content: "analyze " + string(reference),
		}}},
	})
	if err != nil || len(provider.requests) != 1 ||
		strings.Contains(provider.requests[0].Messages[0].Content, string(reference)) {
		t.Fatalf("cross-context host selection influenced provider projection: request=%#v err=%v", provider.requests, err)
	}

	provider = &providerStreamStub{responses: []providerStreamResponse{{}}}
	_, err = StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:            provider,
		SecurityContext:     securityContext,
		HostEntitySelection: selection,
		OrdinaryEffect:      true,
		Request: domainmodel.Request{Messages: []domainmodel.Message{{
			Role: "user", Content: "ordinary work " + string(reference),
		}}},
	})
	if err != nil || len(provider.requests) != 1 ||
		strings.Contains(provider.requests[0].Messages[0].Content, string(reference)) {
		t.Fatalf("ordinary effect retained case reference: request=%#v err=%v", provider.requests, err)
	}
}

func TestTaskRelevantFinancialEntityTypesRequireExactEnglishTokens(t *testing.T) {
	if got := taskRelevantFinancialEntityTypesV1("explain accountability and cardinality"); len(got) != 0 {
		t.Fatalf("substring-only prose selected case identities: %#v", got)
	}
	if got := taskRelevantFinancialEntityTypesV1("analyze this account and card"); len(got) != 2 {
		t.Fatalf("exact task entity tokens were not selected: %#v", got)
	}
}

func TestPreparedProviderStepCannotReplaceHostEntitySelection(t *testing.T) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-provider-step", 3)
	newSelection := func(seed, referenceRune string) HostCaseEntitySelectionV1 {
		selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
			securityContext,
			caseentityapp.PrivateRecordReferenceV1{
				RecordID:     domainsecurity.SHA256Hex([]byte(seed + "-record")),
				RecordDigest: domainsecurity.SHA256Hex([]byte(seed + "-digest")),
			},
			[]domaincaseentity.ReferenceV1{
				domaincaseentity.ReferenceV1("cer1_" + strings.Repeat(referenceRune, 64)),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return selection
	}
	current := RuntimeProviderStep{
		Prompt: "analyze", LogicalEffect: domainsecurity.LogicalEffectFundsData,
		HostEntitySelection: newSelection("current", "f"),
	}
	if err := validatePreparedProviderStep(current, current); err != nil {
		t.Fatalf("exact provider step was rejected: %v", err)
	}
	changed := current
	changed.HostEntitySelection = newSelection("changed", "g")
	if err := validatePreparedProviderStep(current, changed); err == nil {
		t.Fatal("provider-step preparation replaced host entity provenance")
	}
}

func TestPreparedProviderStepDropsHostEntitySelectionOnlyForAdditiveOrdinaryFallback(t *testing.T) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-provider-fallback", 4)
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
		securityContext,
		caseentityapp.PrivateRecordReferenceV1{
			RecordID:     domainsecurity.SHA256Hex([]byte("fallback-record")),
			RecordDigest: domainsecurity.SHA256Hex([]byte("fallback-digest")),
		},
		[]domaincaseentity.ReferenceV1{
			domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("h", 64)),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	current := RuntimeProviderStep{
		Prompt: "analyze and update code", LogicalEffect: domainsecurity.LogicalEffectFundsData,
		OrdinaryWork: true, HostEntitySelection: selection,
	}
	fallback := RuntimeProviderStep{
		Prompt: current.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork: true, CaseSourceUnavailable: true,
	}
	if err := validatePreparedProviderStep(current, fallback); err != nil {
		t.Fatalf("additive ordinary fallback was rejected: %v", err)
	}
	fallback.HostEntitySelection = selection
	if err := validatePreparedProviderStep(current, fallback); err == nil {
		t.Fatal("ordinary fallback retained protected host entity provenance")
	}
}
