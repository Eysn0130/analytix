package job

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCaseDelegationContextProviderAndDurableProjectionExcludePrivateInput(t *testing.T) {
	contextValue, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-parent", TurnID: "turn-parent", WorkspaceRealPath: "/workspace/private-case",
		CaseID: "case-1", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-1"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("sources")), ContextEpoch: 7, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	callID := jobTestHostToolCallID("case-delegation")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextValue, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: time.Now().UTC(),
	})
	binding, err := NewSecurityBinding(contextValue, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	semantic := CaseDelegationSemanticContextV1{
		TaskKind: CaseDelegationTaskKindV1, Currentness: domaincaseentity.SnapshotCurrentV1,
		Entities: []CaseDelegatedEntitySemanticV1{{
			Alias: "acct:1", EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			ResolutionDigest:     domainsecurity.SHA256Hex([]byte("private-reference-resolution")),
		}},
		Claims:        []CaseDelegatedClaimReferenceV1{{Digest: domainsecurity.SHA256Hex([]byte("claim")), InvestigationState: domaincaseentity.InvestigationOpenV1}},
		Evidence:      []CaseDelegatedEvidenceReferenceV1{{Digest: domainsecurity.SHA256Hex([]byte("evidence")), Currentness: domaincaseentity.SnapshotCurrentV1}},
		Continuations: []CaseDelegatedContinuationReferenceV1{{Digest: domainsecurity.SHA256Hex([]byte("continuation")), Currentness: domaincaseentity.SnapshotCurrentV1}},
		AnswerSlots: []CaseDelegatedAnswerSlotCommitmentV1{{
			Digest: strings.Repeat("1", 64), ClaimCount: 3, EvidenceCount: 1,
		}},
	}
	delegation, err := NewCaseDelegationContextV1(binding, semantic)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := CaseDelegationProviderPromptV1(delegation, binding)
	if err != nil {
		t.Fatal(err)
	}
	delegationToken := CaseDelegationProviderCommitmentTokenV1(delegation.DelegationDigest)
	answerSlotToken := CaseDelegationProviderCommitmentTokenV1(semantic.AnswerSlots[0].Digest)
	if ValidateCaseDelegationProviderCommitmentTokenV1(delegationToken) != nil ||
		ValidateCaseDelegationProviderCommitmentTokenV1(answerSlotToken) != nil {
		t.Fatalf("provider commitment grammar is invalid: delegation=%q slot=%q", delegationToken, answerSlotToken)
	}
	const rawPrompt = "RAW_PROVIDER_PROMPT_8E2A 13800138000 /Users/private reverseMap"
	longReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	durable, err := json.Marshal(Record{
		Kind: "subagent", Name: CaseDelegationNameV1, Label: CaseDelegationLabelV1, Prompt: prompt,
		SecurityBinding: binding, CaseDelegation: delegation,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{prompt, string(durable)} {
		for _, forbidden := range []string{rawPrompt, "13800138000", "/Users/private", string(longReference), "reverseMap"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("case delegation projection leaked %q: %s", forbidden, body)
			}
		}
	}
	if !strings.Contains(prompt, `"alias":"acct:1"`) || !strings.Contains(prompt, delegationToken) ||
		!strings.Contains(prompt, answerSlotToken) || ValidateExecutableCaseDelegationV1(
		"subagent", CaseDelegationNameV1, CaseDelegationLabelV1, prompt, binding, delegation,
	) != nil {
		t.Fatalf("typed provider projection is incomplete or private: %s", prompt)
	}
	for _, forbidden := range []string{
		delegation.DelegationDigest, semantic.AnswerSlots[0].Digest, semantic.Entities[0].ResolutionDigest,
		semantic.Claims[0].Digest, semantic.Evidence[0].Digest, semantic.Continuations[0].Digest,
		`"claims"`, `"evidence"`, `"continuations"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("provider prompt exposed raw host-private digest %q: %s", forbidden, prompt)
		}
	}
	for _, count := range []string{`"claimCount":3`, `"evidenceCount":1`, `"gapCount":0`} {
		if !strings.Contains(prompt, count) {
			t.Fatalf("provider prompt omitted closed answer-slot count %q: %s", count, prompt)
		}
	}
	if projected := ProjectPersistableUntrustedOutputV1(prompt); projected != prompt {
		t.Fatalf("provider-safe commitment changed under ordinary projection: got=%q want=%q", projected, prompt)
	}
	providerSelection := CaseForegroundChildSelectionV1{
		SchemaVersion: CaseForegroundChildResultSchemaVersionV1,
		Purpose:       CaseForegroundChildSelectionPurposeV1, DelegationDigest: delegationToken,
		AnswerSlotDigest: answerSlotToken,
	}
	providerSelectionBody, err := json.Marshal(providerSelection)
	if err != nil {
		t.Fatal(err)
	}
	normalizedSelection, canonicalSelection, err := ParseCaseForegroundChildSelectionV1(providerSelection, delegation)
	if err != nil || normalizedSelection.DelegationDigest != delegation.DelegationDigest ||
		normalizedSelection.AnswerSlotDigest != semantic.AnswerSlots[0].Digest ||
		string(canonicalSelection) != string(providerSelectionBody) {
		t.Fatalf("exact current provider selector did not resolve host-private digests: normalized=%#v canonical=%s err=%v", normalizedSelection, canonicalSelection, err)
	}
	providerSelection.AnswerSlotDigest = CaseDelegationProviderCommitmentTokenV1(domainsecurity.SHA256Hex([]byte("other-slot")))
	if _, _, err := ParseCaseForegroundChildSelectionV1(providerSelection, delegation); err == nil {
		t.Fatal("well-formed token outside the exact current delegation resolved host-private authority")
	}
	for name, rawDigest := range map[string]string{
		"ordinary-hex":   strings.Repeat("a", 64),
		"account-shaped": strings.Repeat("9", 64),
	} {
		t.Run("raw-selector-"+name, func(t *testing.T) {
			rawSelection := CaseForegroundChildSelectionV1{
				SchemaVersion:    CaseForegroundChildResultSchemaVersionV1,
				Purpose:          CaseForegroundChildSelectionPurposeV1,
				DelegationDigest: rawDigest, AnswerSlotDigest: rawDigest,
			}
			if _, _, err := ParseCaseForegroundChildSelectionV1(rawSelection, delegation); err == nil {
				t.Fatal("raw SHA selector bypassed the closed provider token boundary")
			}
		})
	}
	normalized := NormalizePersistedRecordV1(Record{
		Kind: "subagent", Name: CaseDelegationNameV1, Label: CaseDelegationLabelV1, Prompt: prompt,
		SecurityBinding: binding, CaseDelegation: delegation,
	})
	if normalized.Prompt != prompt || ValidateExecutableCaseDelegationV1(
		normalized.Kind, normalized.Name, normalized.Label, normalized.Prompt, normalized.SecurityBinding, normalized.CaseDelegation,
	) != nil {
		t.Fatalf("host-created case prompt changed during durable normalization: got=%q want=%q", normalized.Prompt, prompt)
	}
}

func TestCaseDelegationContextRejectsCrossBoundaryAndMalformedSemantic(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	binding := fixture.binding
	valid := CaseDelegationSemanticContextV1{
		TaskKind: CaseDelegationTaskKindV1, Currentness: domaincaseentity.SnapshotCurrentV1,
		Entities: []CaseDelegatedEntitySemanticV1{{
			Alias: "acct:1", EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			ResolutionDigest:     domainsecurity.SHA256Hex([]byte("resolution")),
		}},
		Claims: []CaseDelegatedClaimReferenceV1{}, Evidence: []CaseDelegatedEvidenceReferenceV1{}, Continuations: []CaseDelegatedContinuationReferenceV1{},
	}
	delegation, err := NewCaseDelegationContextV1(binding, valid)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CaseDelegationContextV1){
		"parent-thread":    func(value *CaseDelegationContextV1) { value.ParentThreadID = "cross-thread" },
		"parent-turn":      func(value *CaseDelegationContextV1) { value.ParentTurnID = "cross-turn" },
		"parent-context":   func(value *CaseDelegationContextV1) { value.ParentContextDigest = strings.Repeat("1", 64) },
		"workspace":        func(value *CaseDelegationContextV1) { value.ParentWorkspaceScopeDigest = strings.Repeat("2", 64) },
		"case":             func(value *CaseDelegationContextV1) { value.ParentCaseID = "cross-case" },
		"case-binding":     func(value *CaseDelegationContextV1) { value.ParentCaseBindingHash = strings.Repeat("3", 64) },
		"snapshot":         func(value *CaseDelegationContextV1) { value.ParentDatasetSnapshotID = "cross-snapshot" },
		"source-manifest":  func(value *CaseDelegationContextV1) { value.ParentSourceManifestHash = strings.Repeat("4", 64) },
		"epoch":            func(value *CaseDelegationContextV1) { value.ParentContextEpoch++ },
		"security-binding": func(value *CaseDelegationContextV1) { value.SecurityBindingDigest = strings.Repeat("5", 64) },
		"delegation-digest": func(value *CaseDelegationContextV1) {
			value.DelegationDigest = strings.Repeat("0", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			poisoned := CloneCaseDelegationContextV1(delegation)
			mutate(poisoned)
			if ValidateCaseDelegationContextV1(poisoned, binding) == nil {
				t.Fatal("cross-boundary delegation was accepted")
			}
		})
	}
	duplicated := valid
	duplicated.Entities = append(append([]CaseDelegatedEntitySemanticV1(nil), valid.Entities...), valid.Entities[0])
	if _, err := NewCaseDelegationContextV1(binding, duplicated); err == nil {
		t.Fatal("duplicate delegated alias was accepted")
	}
	unsupported := valid
	unsupported.Entities = append([]CaseDelegatedEntitySemanticV1(nil), valid.Entities...)
	unsupported.Entities[0].Alias = "person:1"
	if _, err := NewCaseDelegationContextV1(binding, unsupported); err == nil {
		t.Fatal("unsupported delegated entity type was accepted")
	}
}
