package loop

import (
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPrepareInitialProviderMessagesCarriesOnlyTypedContinuationIntoCaseRequest(t *testing.T) {
	continuation := appmodel.TypedOrdinaryCaseContinuationPrefixV1 + `{"schemaVersion":1,"stateDigest":"` + strings.Repeat("a", 64) + `"}`
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true,
		Policy: CaseFundAnalysisPolicy{
			Active: true, ToolScope: []string{"mcp__analytix_funds__analyze_account_flows"},
			SystemInstruction: "CASE_STEP_ONLY",
		},
		History: []domainmodel.Message{{Role: "user", Content: "CURRENT_POST_CUT_CASE_HISTORY"}},
		OrdinaryHistory: []domainmodel.Message{{
			Role: "assistant", Content: "OLD_TYPED_ORDINARY_RESULT_MUST_NOT_JOIN_CASE_REQUEST",
		}},
		CaseTaskContinuationHistory: []domainmodel.Message{{Role: "user", Content: continuation}},
		UserPrompt:                  "analyze the current case selection",
	})
	body := ""
	for _, message := range prepared.Messages {
		body += "\n" + message.Content
	}
	for _, required := range []string{continuation, "CURRENT_POST_CUT_CASE_HISTORY", "analyze the current case selection"} {
		if !strings.Contains(body, required) {
			t.Fatalf("case request omitted %q: %#v", required, prepared.Messages)
		}
	}
	if strings.Contains(body, "OLD_TYPED_ORDINARY_RESULT_MUST_NOT_JOIN_CASE_REQUEST") {
		t.Fatalf("case request replayed an ordinary result beside the typed continuation: %#v", prepared.Messages)
	}

	duplicate := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true, Policy: preparedCaseFundPolicyForContinuationTestV1(),
		CaseTaskContinuationHistory: []domainmodel.Message{{Role: "user", Content: continuation}, {Role: "user", Content: continuation}},
		UserPrompt:                  "analyze the current case selection",
	})
	for _, message := range duplicate.Messages {
		if strings.Contains(message.Content, "Analytix task continuation snapshot") {
			t.Fatal("duplicate typed continuation entered a case provider request")
		}
	}
}

func preparedCaseFundPolicyForContinuationTestV1() CaseFundAnalysisPolicy {
	return CaseFundAnalysisPolicy{
		Active: true, ToolScope: []string{"mcp__analytix_funds__analyze_account_flows"},
		SystemInstruction: "CASE_STEP_ONLY",
	}
}

func TestPrepareInitialProviderMessagesKeepsImmutableBaseAcrossEffectPrompts(t *testing.T) {
	policy := CaseFundAnalysisPolicy{
		Active: true, ToolScope: []string{"mcp__analytix_funds__analyze_account_flows"},
		SystemInstruction: "FUNDS_STEP_ONLY",
	}
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "BASE_SYSTEM", Policy: policy, UserPrompt: "analyze current account flows",
		ContextEpoch: domaincontextepoch.ProviderContext{StablePrefix: []domaincontextepoch.ProviderFragment{{
			Boundary: domaincontextepoch.BoundaryStablePrefix, Content: "stable host context",
		}}},
	})
	if prepared.BaseSystemPrompt == "" || !strings.Contains(prepared.BaseSystemPrompt, "BASE_SYSTEM") ||
		!strings.Contains(prepared.BaseSystemPrompt, "stable host context") ||
		strings.Contains(prepared.BaseSystemPrompt, "FUNDS_STEP_ONLY") {
		t.Fatalf("effect-neutral base prompt is invalid: %q", prepared.BaseSystemPrompt)
	}
	if prepared.SystemPrompt != RuntimeProviderSystemPromptV1(prepared.BaseSystemPrompt, policy) ||
		!strings.HasSuffix(prepared.SystemPrompt, "FUNDS_STEP_ONLY") || len(prepared.Messages) == 0 ||
		prepared.Messages[0].Role != "system" || prepared.Messages[0].Content != prepared.SystemPrompt {
		t.Fatalf("initial funds prompt was not compiled from the immutable base: %#v", prepared)
	}
	ordinary := RuntimeProviderSystemPromptV1(prepared.BaseSystemPrompt, CaseFundAnalysisPolicy{})
	if ordinary != prepared.BaseSystemPrompt || strings.Contains(ordinary, "FUNDS_STEP_ONLY") {
		t.Fatalf("ordinary step retained a prior funds instruction: %q", ordinary)
	}
	nextFunds := RuntimeProviderSystemPromptV1(prepared.BaseSystemPrompt, CaseFundAnalysisPolicy{
		Active: true, SystemInstruction: "NEXT_FUNDS_STEP",
	})
	if strings.Contains(nextFunds, "FUNDS_STEP_ONLY") || !strings.HasSuffix(nextFunds, "NEXT_FUNDS_STEP") {
		t.Fatalf("next effect was compiled from a prior effect instead of the base: %q", nextFunds)
	}
}

func TestPrepareInitialProviderMessagesCaseThreadUsesOnlyCurrentOrdinaryInput(t *testing.T) {
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true,
		History: []domainmodel.Message{
			{Role: "user", Content: "OLD_CASE_PROMPT_SENTINEL"},
			{Role: "assistant", Content: "OLD_UNTYPED_CASE_RESULT_SENTINEL"},
		},
		OrdinaryHistory: []domainmodel.Message{{Role: "assistant", Content: "TRUSTED_TYPED_ORDINARY_RESULT"}},
		Background:      []domainmodel.Message{{Role: "user", Content: "OLD_BACKGROUND_SENTINEL"}},
		UserPrompt:      "Update the ordinary source file.",
	})
	if !prepared.OrdinaryResultInputIsolated || len(prepared.Messages) != 3 ||
		prepared.Messages[0].Role != "system" || prepared.Messages[1].Role != "assistant" ||
		prepared.Messages[1].Content != "TRUSTED_TYPED_ORDINARY_RESULT" ||
		prepared.Messages[2].Role != "user" || prepared.Messages[2].Content != "Update the ordinary source file." {
		t.Fatalf("case-thread ordinary input was not isolated: %#v", prepared)
	}
	for _, sentinel := range []string{"OLD_CASE_PROMPT_SENTINEL", "OLD_UNTYPED_CASE_RESULT_SENTINEL", "OLD_BACKGROUND_SENTINEL"} {
		for _, message := range prepared.Messages {
			if strings.Contains(message.Content, sentinel) {
				t.Fatalf("untyped case history reached ordinary provider input: %q in %#v", sentinel, prepared.Messages)
			}
		}
	}
}

func TestPrepareInitialProviderMessagesKeepsRawCaseTranscriptOutOfOrdinaryLane(t *testing.T) {
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true,
		Policy: CaseFundAnalysisPolicy{
			Active: true, OrdinaryWorkRequested: true,
		},
		History: []domainmodel.Message{
			{Role: "user", Content: "RAW_CASE_HISTORY_SENTINEL"},
			{Role: "assistant", Content: "RAW_CASE_RESULT_SENTINEL"},
		},
		OrdinaryHistory: []domainmodel.Message{{Role: "assistant", Content: "TRUSTED_TYPED_ORDINARY_RESULT"}},
		Background:      []domainmodel.Message{{Role: "user", Content: "RAW_CASE_BACKGROUND_SENTINEL"}},
		UserPrompt:      "MIXED_CURRENT_CASE_PROMPT_SENTINEL",
	})
	if len(prepared.Messages) < 4 || len(prepared.OrdinaryLaneMessages) != 2 ||
		prepared.OrdinaryLaneMessages[0].Role != "system" ||
		prepared.OrdinaryLaneMessages[1].Role != "assistant" ||
		prepared.OrdinaryLaneMessages[1].Content != "TRUSTED_TYPED_ORDINARY_RESULT" {
		t.Fatalf("typed ordinary lane was not composed independently: %#v", prepared)
	}
	for _, sentinel := range []string{
		"RAW_CASE_HISTORY_SENTINEL", "RAW_CASE_RESULT_SENTINEL",
		"RAW_CASE_BACKGROUND_SENTINEL", "MIXED_CURRENT_CASE_PROMPT_SENTINEL",
	} {
		for _, message := range prepared.OrdinaryLaneMessages {
			if strings.Contains(message.Content, sentinel) {
				t.Fatalf("raw case transcript reached ordinary lane: %q in %#v", sentinel, prepared.OrdinaryLaneMessages)
			}
		}
	}
}

func TestPrepareInitialProviderMessagesMixedSourceUnavailableUsesExactOrdinarySubrequest(t *testing.T) {
	rawAccount := "6222020000000000000"
	prompt := "Update the code and run tests; query bank account " + rawAccount + " in the current case."
	policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
	if !policy.Active || !policy.SourceUnavailable || !policy.OrdinaryWorkRequested ||
		policy.OrdinaryPrompt != "Update the code；run tests" {
		t.Fatalf("mixed prompt was not partitioned before provider preparation: %#v", policy)
	}
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true,
		Policy:          policy,
		History:         []domainmodel.Message{{Role: "user", Content: "OLD_CASE_PROMPT_SENTINEL"}},
		OrdinaryHistory: []domainmodel.Message{{Role: "assistant", Content: "TRUSTED_ORDINARY_RESULT"}},
		Background:      []domainmodel.Message{{Role: "user", Content: "OLD_BACKGROUND_SENTINEL"}},
		UserPrompt:      prompt,
	})
	if !prepared.OrdinaryResultInputIsolated || len(prepared.Messages) != 3 ||
		prepared.Messages[1].Content != "TRUSTED_ORDINARY_RESULT" ||
		prepared.Messages[2].Role != "user" || prepared.Messages[2].Content != policy.OrdinaryPrompt {
		t.Fatalf("mixed source-unavailable input did not use its exact ordinary lane: %#v", prepared)
	}
	for _, forbidden := range []string{rawAccount, "query bank account", "OLD_CASE_PROMPT_SENTINEL", "OLD_BACKGROUND_SENTINEL"} {
		for _, message := range prepared.Messages {
			if strings.Contains(message.Content, forbidden) {
				t.Fatalf("protected mixed input reached ordinary provider lane: %q in %#v", forbidden, prepared.Messages)
			}
		}
	}
}

func TestPrepareInitialProviderMessagesCaseFactPromptIsNotOrdinaryOnly(t *testing.T) {
	prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
		SystemPrompt: "system", CaseSensitive: true,
		UserPrompt: "张某实际控制甲公司，金额为 2645472 元。",
	})
	if prepared.OrdinaryResultInputIsolated {
		t.Fatalf("case-fact prompt gained ordinary-only provenance: %#v", prepared)
	}
}

func TestBindInitialProviderIngressAliasesTargetsCurrentExactUserMessage(t *testing.T) {
	const providerText = "analyze acct:1"
	prepared := InitialProviderMessagesV1{Messages: []domainmodel.Message{
		{Role: "user", Content: providerText},
		{Role: "assistant", Content: "prior result"},
		{Role: "user", Content: providerText},
	}}
	securityContext := newLoopCaseContextV2(t, "thread-alias-binding", "turn-alias-binding", "/workspace", "case-a")
	if err := BindInitialProviderIngressAliasesV1(
		securityContext,
		InitialProviderIngressConsumptionV1{
			Text: providerText, Aliases: []domaincaseentity.ModelEntityAliasV1{"acct:1"},
		},
		&prepared,
	); err != nil {
		t.Fatal(err)
	}
	if prepared.Messages[0].PrivateProviderSemanticBinding != nil ||
		prepared.Messages[2].PrivateProviderSemanticBinding == nil {
		t.Fatalf("provider alias authority was not bound only to the current exact user message: %#v", prepared.Messages)
	}
	if err := BindInitialProviderIngressAliasesV1(
		securityContext,
		InitialProviderIngressConsumptionV1{
			Text: "missing", Aliases: []domaincaseentity.ModelEntityAliasV1{"acct:1"},
		},
		&prepared,
	); err == nil {
		t.Fatal("missing current provider ingress target was accepted")
	}
}
