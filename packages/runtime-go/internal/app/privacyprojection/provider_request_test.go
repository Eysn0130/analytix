package privacyprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestProjectProviderRequestProjectsToolArgumentsAndSerializedDiffWithoutRebindingSource(t *testing.T) {
	const path = "/Users/private-owner/Case Files/source.csv"
	const code = "// ordinary source comment\nexport const value = 1\n"
	arguments, _ := json.Marshal(map[string]any{"path": path, "content": code})
	content, _ := json.Marshal(map[string]any{"path": path, "diff": "--- a/" + path + "\n+++ b/" + path + "\n@@ -1 +1 @@\n-old\n+new\n", "bytes_written": 13})
	request := domainmodel.Request{Messages: []domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call-file", Name: "write_file", Arguments: arguments}}},
		{Role: "tool", ToolCallID: "call-file", Name: "write_file", Content: string(content)},
	}}
	before, err := json.Marshal(request.Messages)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectProviderRequestForEffect(domainsecurity.TurnSecurityContext{}, request, true)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projected.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-owner") || strings.Contains(string(encoded), "source.csv") || !strings.Contains(string(encoded), "[PRIVATE_PATH]") {
		t.Fatal("serialized request retained private argument or diff paths")
	}
	if projected.Messages[0].ToolCalls[0].ID != "call-file" || projected.Messages[1].ToolCallID != "call-file" {
		t.Fatal("projection changed tool pairing")
	}
	var projectedArgs map[string]any
	if json.Unmarshal(projected.Messages[0].ToolCalls[0].Arguments, &projectedArgs) != nil || projectedArgs["content"] != code {
		t.Fatal("projection replaced ordinary code with a path placeholder")
	}
	var result map[string]any
	if json.Unmarshal([]byte(projected.Messages[1].Content), &result) != nil || result["bytes_written"] != float64(13) ||
		!strings.Contains(result["diff"].(string), "@@ -1 +1 @@\n-old\n+new\n") {
		t.Fatal("projection lost physical effect metadata or diff hunks")
	}
	after, err := json.Marshal(request.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("projection mutated private source request")
	}
}

func TestProjectProviderRequestMasksEveryCaseTextLane(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	rawAccount := "6222020000000000000"
	request := domainmodel.Request{
		SystemPrompt: "System account " + rawAccount,
		Messages: []domainmodel.Message{
			{Role: "user", Content: "核查账号 " + rawAccount},
			{Role: "user", Parts: []domainmodel.MessagePart{{Type: "text", Text: "附件账号 " + rawAccount}}},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
				ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"accountId":"` + rawAccount + `","amountMinor":"123456789012345678"}`),
			}}},
		},
		Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "Look up a host-authorized account reference.",
			Parameters:   json.RawMessage(`{"type":"object","properties":{"accountId":{"type":"string"},"apiKey":{"type":"string"}},"required":["accountId","apiKey"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}},"required":["status"],"additionalProperties":false}`),
		}},
	}

	projected, err := ProjectProviderRequest(securityContext, request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(projected.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(projected.SystemPrompt+string(body), rawAccount) {
		t.Fatalf("full account survived provider projection: %s", body)
	}
	if count := strings.Count(projected.SystemPrompt+string(body), "[ACCOUNT]"); count < 4 {
		t.Fatalf("provider projection omitted an account lane: count=%d body=%s", count, body)
	}
	if !strings.Contains(string(projected.Messages[2].ToolCalls[0].Arguments), `"amountMinor":"123456789012345678"`) {
		t.Fatalf("typed amount was changed by privacy projection: %s", projected.Messages[2].ToolCalls[0].Arguments)
	}
	if !strings.Contains(request.Messages[0].Content, rawAccount) || !strings.Contains(request.Messages[1].Parts[0].Text, rawAccount) {
		t.Fatal("provider projection mutated its caller-owned request")
	}
	if err := ValidateProviderRequest(projected); err != nil {
		t.Fatalf("projected request failed validation: %v", err)
	}
}

func TestCaseDelegationProviderCommitmentGrammarIsOrdinaryProjectionStable(t *testing.T) {
	// a, j, and p are the nibble encodings for derived hex 0, 9, and f. The
	// all-j case therefore proves that even hypothetical all-decimal derived
	// bytes cannot form a decimal identifier at the generic privacy boundary.
	for name, token := range map[string]string{
		"all-zero-derived":    "cmt1_" + strings.Repeat("a", 64),
		"all-nine-derived":    "cmt1_" + strings.Repeat("j", 64),
		"all-maximum-derived": "cmt1_" + strings.Repeat("p", 64),
	} {
		t.Run(name, func(t *testing.T) {
			if err := domainjob.ValidateCaseDelegationProviderCommitmentTokenV1(token); err != nil {
				t.Fatalf("closed provider commitment grammar rejected: %v", err)
			}
			if projected := ProjectOrdinaryText(token); projected != token {
				t.Fatalf("provider commitment collided with generic privacy projection: got=%q want=%q", projected, token)
			}
		})
	}
}

func TestAttachmentVirtualPathIsOrdinaryProjectionStable(t *testing.T) {
	const text = "FilePath: attachment://att_0123456789abcdef01234567"
	if projected := ProjectOrdinaryText(text); projected != text {
		t.Fatalf("virtual attachment path changed: got=%q want=%q", projected, text)
	}
}

func TestProjectProviderRequestMasksAuthorityEntityReferenceForEveryEffect(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	const rawAccount = "6222021234567890123"
	request := domainmodel.Request{
		SystemPrompt: "Use the host-verified entity reference; never infer complete identifiers.",
		Messages: []domainmodel.Message{
			{Role: "user", Content: "核查账号 " + string(reference) + "；旁路账号 " + rawAccount},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
				ID:   "call-reference",
				Name: "lookup",
				Arguments: json.RawMessage(`{"accountRef":"` + string(reference) +
					`","rawAccount":"` + rawAccount + `"}`),
			}}},
		},
	}
	strict, err := ProjectProviderRequestForEffect(securityContext, request, false)
	if err != nil {
		t.Fatal(err)
	}
	strictBody, err := json.Marshal(strict.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(strictBody), string(reference)) ||
		domaincaseentity.ContainsReferenceCandidateV1(string(strictBody)) ||
		strings.Contains(string(strictBody), rawAccount) ||
		!strings.Contains(string(strictBody), "[ACCOUNT]") {
		t.Fatalf("strict case provider projection retained host-private identity or complete PII: %s", strictBody)
	}

	ordinary, err := ProjectProviderRequestForEffect(securityContext, request, true)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryBody, err := json.Marshal(ordinary.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if domaincaseentity.ContainsReferenceCandidateV1(string(ordinaryBody)) ||
		strings.Contains(string(ordinaryBody), rawAccount) {
		t.Fatalf("ordinary provider effect retained case identity or complete PII: %s", ordinaryBody)
	}

	obfuscated := request
	obfuscated.Messages = []domainmodel.Message{{
		Role: "user", Content: "核查账号 cer1_" + strings.Repeat("aaaa-", 16),
	}}
	projectedObfuscated, err := ProjectProviderRequestForEffect(securityContext, obfuscated, false)
	if err != nil {
		t.Fatal(err)
	}
	if domaincaseentity.ContainsReferenceCandidateV1(projectedObfuscated.Messages[0].Content) ||
		!strings.Contains(projectedObfuscated.Messages[0].Content, "[ACCOUNT]") {
		t.Fatalf("unverified reference candidate crossed strict provider projection: %#v", projectedObfuscated.Messages)
	}

	for name, continuation := range map[string]string{
		"uppercase suffix":  "A",
		"outside suffix":    "q",
		"decimal suffix":    "7",
		"underscore suffix": "_",
		"full-width suffix": "Ａ",
	} {
		t.Run(name, func(t *testing.T) {
			continued := request
			continued.Messages = []domainmodel.Message{{
				Role: "user", Content: "核查账号 " + string(reference) + continuation,
			}}
			projected, projectErr := ProjectProviderRequestForEffect(
				securityContext,
				continued,
				false,
			)
			if projectErr != nil {
				t.Fatal(projectErr)
			}
			if strings.Contains(projected.Messages[0].Content, string(reference)) ||
				domaincaseentity.ContainsReferenceCandidateV1(projected.Messages[0].Content) {
				t.Fatalf("non-token reference continuation crossed provider projection: %q", projected.Messages[0].Content)
			}
		})
	}

	for name, prefix := range map[string]string{
		"uppercase prefix":  "A",
		"outside prefix":    "q",
		"decimal prefix":    "7",
		"underscore prefix": "_",
		"full-width prefix": "Ａ",
	} {
		t.Run(name, func(t *testing.T) {
			continued := request
			continued.Messages = []domainmodel.Message{{
				Role: "user", Content: "核查账号 " + prefix + string(reference),
			}}
			projected, projectErr := ProjectProviderRequestForEffect(
				securityContext,
				continued,
				false,
			)
			if projectErr != nil {
				t.Fatal(projectErr)
			}
			if strings.Contains(projected.Messages[0].Content, string(reference)) ||
				domaincaseentity.ContainsReferenceCandidateV1(projected.Messages[0].Content) {
				t.Fatalf("non-token reference prefix crossed provider projection: %q", projected.Messages[0].Content)
			}
		})
	}
}

func TestProjectProviderRequestPreservesOnlyHostBoundModelAliases(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	digitHeavyEvidenceDigest := "6222021234567890123" + strings.Repeat("a", 45)
	message := domainmodel.Message{
		Role: "user",
		Content: "请分析银行账号 acct:1 与银行卡号 card:2；联系电话 13800138000。\n\n" +
			`<analytix_host_verified_case_entity_semantics>{"entities":[` +
			`{"alias":"acct:1","entityType":"bank_account_number","financialAccountType":"bank_account_number"},` +
			`{"alias":"card:2","entityType":"bank_card_number","financialAccountType":"bank_card_number"}` +
			`],"longitudinal":{"schemaVersion":1,"scopeBindingDigest":"` +
			providerCaseLongitudinalScopeBindingDigestV1(securityContext.CaseBindingHash) +
			`","selectionBudget":32,"items":[{` +
			`"kind":"evidence_claim_reference","referenceKind":"evidence","referenceDigest":"` + strings.Repeat("c", 64) + `","digest":"` +
			digitHeavyEvidenceDigest + `","currentness":"current","snapshotBindingDigest":"` + strings.Repeat("b", 64) +
			`"}],"omittedTotal":0,"omittedCoverage":[` +
			`{"kind":"current_verified_fact","count":0},` +
			`{"kind":"historical_comparison_fact","count":0},` +
			`{"kind":"key_relationship","count":0},` +
			`{"kind":"counterevidence_refuted_finding","count":0},` +
			`{"kind":"data_gap","count":0},` +
			`{"kind":"evidence_claim_reference","count":0},` +
			`{"kind":"snapshot_difference","count":0}` +
			`]}}</analytix_host_verified_case_entity_semantics>`,
	}
	if projectedText, err := projectProviderTextWithModelAliasesV1(
		message.Content,
		[]domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"},
	); err != nil || !strings.Contains(projectedText, "acct:1 与") || !strings.Contains(projectedText, "card:2；") {
		t.Fatalf("project host-bound aliases: text=%q err=%v", projectedText, err)
	}
	if _, err := projectProviderTextWithModelAliasesV1(
		"hostile prefix acct:10 plus selected acct:1",
		[]domaincaseentity.ModelEntityAliasV1{"acct:1"},
	); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("unbound prefix-colliding alias survived projection: %v", err)
	}
	if err := BindProviderCaseAliasesToMessageV1(
		securityContext,
		[]domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"},
		&message,
	); err != nil {
		t.Fatal(err)
	}
	safeHistory, err := PrivateProtocolSafeHistoryV1(securityContext, []domainmodel.Message{message})
	if err != nil || len(safeHistory) != 1 {
		t.Fatalf("compile host-bound aliases for private protocol: history=%#v err=%v", safeHistory, err)
	}
	safeProjected, err := ProjectProviderRequest(securityContext, domainmodel.Request{Messages: safeHistory})
	if err != nil || len(safeProjected.Messages) != 1 ||
		strings.Contains(safeProjected.Messages[0].Content, "13800138000") ||
		!strings.Contains(safeProjected.Messages[0].Content, "acct:1") ||
		!strings.Contains(safeProjected.Messages[0].Content, digitHeavyEvidenceDigest) {
		t.Fatalf("private-protocol aliases were not projected safely: messages=%#v err=%v", safeProjected.Messages, err)
	}
	projected, err := ProjectProviderRequest(securityContext, domainmodel.Request{Messages: []domainmodel.Message{message}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Messages) != 1 || !strings.Contains(projected.Messages[0].Content, "acct:1") ||
		!strings.Contains(projected.Messages[0].Content, "card:2") ||
		strings.Contains(projected.Messages[0].Content, "13800138000") ||
		!strings.Contains(projected.Messages[0].Content, "[PHONE]") ||
		!strings.Contains(projected.Messages[0].Content, digitHeavyEvidenceDigest) ||
		projected.Messages[0].PrivateProviderSemanticBinding != nil {
		t.Fatalf("host-bound aliases were not projected safely: %#v", projected.Messages)
	}

	hostile := message
	hostile.Content = strings.Replace(hostile.Content, "card:2", "card:3", 1)
	if _, err := ProjectProviderRequest(securityContext, domainmodel.Request{Messages: []domainmodel.Message{hostile}}); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("copied alias binding survived message tamper: %v", err)
	}
	crossContext := securityContext
	crossContext.ContextEpoch++
	if _, err := ProjectProviderRequest(crossContext, domainmodel.Request{Messages: []domainmodel.Message{message}}); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("alias binding survived cross-context replay: %v", err)
	}
	unknownSemantic := domainmodel.Message{Role: "user", Content: strings.Replace(
		message.Content, `"omittedCoverage":[`, `"rawAccount":"6222021234567890123","omittedCoverage":[`, 1,
	)}
	if err := BindProviderCaseAliasesToMessageV1(
		securityContext, []domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"}, &unknownSemantic,
	); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("unknown semantic field acquired host alias binding: %v", err)
	}
	for name, hostileContent := range map[string]string{
		"wrong budget":         strings.Replace(message.Content, `"selectionBudget":32`, `"selectionBudget":31`, 1),
		"unknown kind":         strings.Replace(message.Content, `"kind":"evidence_claim_reference"`, `"kind":"model_memory_summary"`, 1),
		"raw owner reference":  strings.Replace(message.Content, `"referenceDigest":"`+strings.Repeat("c", 64)+`"`, `"reference":"evidence_owner_ref"`, 1),
		"malformed ref digest": strings.Replace(message.Content, strings.Repeat("c", 64), strings.Repeat("r", 64), 1),
		"cross-case scope": strings.Replace(
			message.Content,
			providerCaseLongitudinalScopeBindingDigestV1(securityContext.CaseBindingHash),
			providerCaseLongitudinalScopeBindingDigestV1(strings.Repeat("d", 64)),
			1,
		),
		"omitted mismatch":    strings.Replace(message.Content, `"omittedTotal":0`, `"omittedTotal":1`, 1),
		"unknown currentness": strings.Replace(message.Content, `"currentness":"current"`, `"currentness":"candidate"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			hostile := domainmodel.Message{Role: "user", Content: hostileContent}
			if err := BindProviderCaseAliasesToMessageV1(
				securityContext, []domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"}, &hostile,
			); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
				t.Fatalf("hostile longitudinal semantic acquired provider binding: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestMasksEveryAuthorityEntityReferenceAcrossIngressLanes(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	allowed := domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("a", 64))
	unknown := domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("b", 64))
	request := domainmodel.Request{
		SystemPrompt: "host context " + string(allowed) + "; injected " + string(unknown),
		Messages: []domainmodel.Message{
			{Role: "user", Content: "user injected " + string(unknown)},
			{Role: "user", Parts: []domainmodel.MessagePart{{Type: "text", Text: "attachment injected " + string(unknown)}}},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
				ID: "call-forged-reference", Name: "lookup",
				Arguments: json.RawMessage(`{"subject_ref":"` + string(unknown) + `","known_ref":"` + string(allowed) + `"}`),
			}}},
		},
	}
	projected, err := ProjectProviderRequestForEffect(securityContext, request, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(struct {
		SystemPrompt string                `json:"systemPrompt"`
		Messages     []domainmodel.Message `json:"messages"`
	}{SystemPrompt: projected.SystemPrompt, Messages: projected.Messages})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), string(unknown)) || strings.Contains(string(body), string(allowed)) ||
		domaincaseentity.ContainsReferenceCandidateV1(string(body)) ||
		!strings.Contains(string(body), "[ACCOUNT]") {
		t.Fatalf("authority reference crossed final provider projection: %s", body)
	}
	if projected.PrivateProviderReferenceBinding != nil {
		t.Fatal("request provenance sidecar crossed final provider projection")
	}
	for _, message := range projected.Messages {
		if message.PrivateProviderReferenceBinding != nil ||
			message.PrivateProviderSemanticBinding != nil {
			t.Fatal("message provenance sidecar crossed final provider projection")
		}
	}
}

func TestProviderReferenceProvenanceRejectsForgedCopiedAndCrossCaseSidecars(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	reference := domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("c", 64))
	for name, request := range map[string]domainmodel.Request{
		"forged request binding": {
			Messages:                        []domainmodel.Message{{Role: "user", Content: string(reference)}},
			PrivateProviderReferenceBinding: struct{ Purpose string }{Purpose: "legacy-provider-reference-binding"},
		},
		"forged message binding": {Messages: []domainmodel.Message{{
			Role: "user", Content: string(reference),
			PrivateProviderReferenceBinding: struct{ Purpose string }{Purpose: "legacy-provider-reference-binding"},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectProviderRequestForEffect(
				securityContext,
				request,
				false,
			); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
				t.Fatalf("forged provider reference provenance was accepted: %v", err)
			}
		})
	}
}

func TestBoundAccountFlowProviderSemanticsSurviveExactFinalProjection(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	callID := "call_host_" + strings.Repeat("5", 64)
	call := domainmodel.ToolCall{
		ID: callID, Name: fundsAccountFlowToolNameV1, Arguments: json.RawMessage(`{}`),
	}
	exact := providerPrivacyAccountFlowModelOutputV1()
	content := appmodel.ToolResultContentForModel(exact)
	message := domainmodel.Message{
		Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: content,
	}
	bound, err := BindAccountFlowProviderSemanticV1(
		securityContext,
		call,
		exact,
		&message,
	)
	if err != nil || !bound || message.PrivateProviderSemanticBinding == nil {
		t.Fatalf("bind exact provider semantics: bound=%v err=%v message=%#v", bound, err, message)
	}
	serialized, err := json.Marshal(message)
	if err != nil || strings.Contains(string(serialized), "canonicalSHA256") || strings.Contains(string(serialized), "contextDigest") {
		t.Fatalf("private semantic binding serialized: body=%s err=%v", serialized, err)
	}
	request := domainmodel.Request{Messages: appmodel.SanitizeToolPairing([]domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}},
		message,
	})}
	projected, err := ProjectProviderRequestForEffect(securityContext, request, false)
	if err != nil {
		t.Fatalf("final provider projection: %v", err)
	}
	if got := projected.Messages[1].Content; got != content {
		t.Fatalf("bound provider semantics changed at final projection:\nwant=%s\n got=%s", content, got)
	}
	if projected.Messages[1].PrivateProviderSemanticBinding != nil {
		t.Fatal("attempt-local provider semantic binding crossed the final projection")
	}
	for _, exactValue := range []string{
		"12345678901234",
		"10000000000000",
		"123456789012",
		"srow1_" + strings.Repeat("1", 64),
		strings.Repeat("2", 64),
		strings.Repeat("3", 64),
	} {
		if !strings.Contains(projected.Messages[1].Content, exactValue) {
			t.Fatalf("final projection lost exact semantic value %q: %s", exactValue, projected.Messages[1].Content)
		}
	}

	tampered := request
	tampered.Messages = appmodel.CloneProviderMessages(request.Messages)
	tampered.Messages[1].Content = strings.Replace(tampered.Messages[1].Content, "10000000000000", "10000000000001", 1)
	if _, err := ProjectProviderRequestForEffect(securityContext, tampered, false); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("digest-mismatched exact provider semantics were accepted: %v", err)
	}
}

func TestBoundCaseForegroundSemanticUsesItsOwnExactFinalValidator(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	call := domainmodel.ToolCall{
		ID: "call_host_" + strings.Repeat("7", 64), Name: "task", Arguments: json.RawMessage(`{}`),
	}
	slot, err := domainnative.NewAccountFlowDelegatedAnswerSlotV1(
		securityContext.ContextDigest, providerPrivacyAccountFlowModelOutputV1(),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := domainjob.CaseForegroundChildResultV1{
		SchemaVersion: domainjob.CaseForegroundChildResultSchemaVersionV1, Purpose: domainjob.CaseForegroundChildResultPurposeV1,
		DelegationDigest: strings.Repeat("8", 64), EntityAliases: []domaincaseentity.ModelEntityAliasV1{"acct:1"},
		Claims: []domainjob.CaseDelegatedClaimReferenceV1{
			{Digest: strings.Repeat("1", 64), InvestigationState: domaincaseentity.InvestigationOpenV1},
			{Digest: strings.Repeat("2", 64), InvestigationState: domaincaseentity.InvestigationOpenV1},
			{Digest: strings.Repeat("3", 64), InvestigationState: domaincaseentity.InvestigationOpenV1},
		},
		Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{{Digest: strings.Repeat("4", 64), Currentness: domaincaseentity.SnapshotCurrentV1}},
		Gaps:     append([]string{}, slot.Gaps...), AnswerSlots: []domainnative.AccountFlowDelegatedAnswerSlotV1{slot},
		Currentness: domaincaseentity.SnapshotCurrentV1,
	}
	if err := domainjob.ValidateCaseForegroundChildResultShapeV1(result); err != nil {
		t.Fatal(err)
	}
	exact := map[string]any{
		"kind": "subagent_task", "childRunId": "child-run-case", "jobId": "child-run-case", "status": "completed",
		"handoffReceiptDigest": strings.Repeat("5", 64), "submissionDigest": strings.Repeat("6", 64),
		"privacyProjectionDigest": strings.Repeat("6", 64), "childCompletionReceiptDigest": strings.Repeat("9", 64),
		"typedResultAccepted": true, "finalGateRequired": true,
		"answerSlotCount": 1, "gapCount": len(result.Gaps), "caseResult": result,
		"factAnswerAllowed": false, "evidenceAuthority": false,
		"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
	}
	canonical, err := canonicalCaseForegroundProviderOutputV1(exact)
	if err != nil {
		t.Fatal(err)
	}
	message := domainmodel.Message{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: string(canonical)}
	bound, err := BindCaseForegroundProviderSemanticV1(securityContext, call, exact, &message)
	if err != nil || !bound || message.PrivateProviderSemanticBinding == nil {
		t.Fatalf("case foreground semantic did not bind: bound=%v err=%v message=%#v", bound, err, message)
	}
	request := domainmodel.Request{Messages: appmodel.SanitizeToolPairing([]domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}}, message,
	})}
	projected, err := ProjectProviderRequestForEffect(securityContext, request, false)
	if err != nil {
		t.Fatalf("case task wrapper was routed to the wrong final validator: %v", err)
	}
	if projected.Messages[1].Name != "task" || projected.Messages[1].Content != string(canonical) ||
		projected.Messages[1].PrivateProviderSemanticBinding != nil {
		t.Fatalf("case task wrapper lost its exact private projection: %#v", projected.Messages[1])
	}
	if !strings.Contains(projected.Messages[1].Content, `"caseResult"`) ||
		!strings.Contains(projected.Messages[1].Content, slot.InflowMinor) {
		t.Fatalf("current parent provider attempt lost the host-created typed handoff: %s", projected.Messages[1].Content)
	}
	if replay, err := ProjectProviderRequestForEffect(securityContext, request, false); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) ||
		!reflect.DeepEqual(replay, domainmodel.Request{}) {
		t.Fatalf("private typed handoff was projected into a second provider attempt: request=%#v err=%v", replay, err)
	}
	raceMessage := domainmodel.Message{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: string(canonical)}
	if bound, err := BindCaseForegroundProviderSemanticV1(securityContext, call, exact, &raceMessage); err != nil || !bound {
		t.Fatalf("case foreground race binding failed: bound=%v err=%v", bound, err)
	}
	raceRequest := domainmodel.Request{Messages: appmodel.SanitizeToolPairing([]domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}}, raceMessage,
	})}
	raceResults := make(chan error, 8)
	for index := 0; index < cap(raceResults); index++ {
		go func() {
			_, raceErr := ProjectProviderRequestForEffect(securityContext, raceRequest, false)
			raceResults <- raceErr
		}()
	}
	raceSuccesses := 0
	for index := 0; index < cap(raceResults); index++ {
		raceErr := <-raceResults
		if raceErr == nil {
			raceSuccesses++
		} else if !errors.Is(raceErr, ErrProviderPrivacyAuthorityUnavailable) {
			t.Fatalf("case foreground race returned an unexpected error: %v", raceErr)
		}
	}
	if raceSuccesses != 1 {
		t.Fatalf("private typed handoff race successes=%d want=1", raceSuccesses)
	}
	safeMessage := domainmodel.Message{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: string(canonical)}
	if bound, err := BindCaseForegroundProviderSemanticV1(securityContext, call, exact, &safeMessage); err != nil || !bound {
		t.Fatalf("case foreground safe-history binding failed: bound=%v err=%v", bound, err)
	}
	safeSource := appmodel.SanitizeToolPairing([]domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}}, safeMessage,
	})
	safeHistory, err := PrivateProtocolSafeHistoryV1(securityContext, safeSource)
	if err != nil || len(safeHistory) != 1 || safeHistory[0].Role != "user" ||
		!strings.Contains(safeHistory[0].Content, `\"caseResult\"`) ||
		!strings.Contains(safeHistory[0].Content, slot.InflowMinor) {
		t.Fatalf("case private safe history lost the typed carrier: history=%#v err=%v", safeHistory, err)
	}
	restartedBody, err := json.Marshal(safeHistory)
	if err != nil {
		t.Fatal(err)
	}
	var restartedHistory []domainmodel.Message
	if err := json.Unmarshal(restartedBody, &restartedHistory); err != nil {
		t.Fatal(err)
	}
	if restarted, restartErr := ProjectProviderRequestForEffect(
		securityContext, domainmodel.Request{Messages: restartedHistory}, false,
	); restartErr == nil {
		restartedProviderBody, _ := json.Marshal(restarted.Messages)
		for _, forbidden := range []string{slot.InflowMinor, result.Claims[0].Digest, result.Evidence[0].Digest, slot.QueryHash} {
			if bytes.Contains(restartedProviderBody, []byte(forbidden)) {
				t.Fatalf("restart without a private binding retained exact typed value %q: %s", forbidden, restartedProviderBody)
			}
		}
	}
	recompiled, err := PrivateProtocolSafeHistoryV1(securityContext, safeHistory)
	if err != nil || len(recompiled) != 1 || recompiled[0].Content != safeHistory[0].Content {
		t.Fatalf("private-protocol recompilation changed the attempt carrier: history=%#v err=%v", recompiled, err)
	}
	projectedSafe, err := ProjectProviderRequestForEffect(
		securityContext, domainmodel.Request{Messages: recompiled}, false,
	)
	if err != nil || len(projectedSafe.Messages) != 1 || projectedSafe.Messages[0].Role != "user" ||
		projectedSafe.Messages[0].PrivateProviderSemanticBinding != nil ||
		!strings.Contains(projectedSafe.Messages[0].Content, slot.InflowMinor) {
		t.Fatalf("private-protocol provider attempt lost the typed carrier: request=%#v err=%v", projectedSafe, err)
	}
	if replay, err := ProjectProviderRequestForEffect(
		securityContext, domainmodel.Request{Messages: safeHistory}, false,
	); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) || !reflect.DeepEqual(replay, domainmodel.Request{}) {
		t.Fatalf("private-protocol typed carrier was consumed twice: request=%#v err=%v", replay, err)
	}
	public, err := canonicalCaseForegroundPublicOutputFromPrivateV1(string(canonical))
	if err != nil {
		t.Fatal(err)
	}
	unboundSafe, err := PrivateProtocolSafeHistoryV1(securityContext, []domainmodel.Message{{Role: "user", Content: string(public)}})
	if err != nil || len(unboundSafe) != 1 || strings.Contains(unboundSafe[0].Content, `"caseResult"`) {
		t.Fatalf("unbound public metadata was upgraded to a typed carrier: history=%#v err=%v", unboundSafe, err)
	}
	tampered := request
	tampered.Messages = appmodel.CloneProviderMessages(request.Messages)
	tampered.Messages[1].Name = fundsAccountFlowToolNameV1
	if _, err := ProjectProviderRequestForEffect(securityContext, tampered, false); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("case semantic binding accepted an account-flow-name rewrite: %v", err)
	}
}

func TestUnboundAccountFlowSemanticLookalikesUseGenericProjection(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	call := domainmodel.ToolCall{
		ID: "call_host_" + strings.Repeat("6", 64), Name: fundsAccountFlowToolNameV1,
		Arguments: json.RawMessage(`{}`),
	}
	content := appmodel.ToolResultContentForModel(providerPrivacyAccountFlowModelOutputV1())
	for name, messages := range map[string][]domainmodel.Message{
		"unbound tool result": {
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}},
			{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: content},
		},
		"user text lookalike": {
			{Role: "user", Content: content},
		},
	} {
		t.Run(name, func(t *testing.T) {
			projected, err := ProjectProviderRequestForEffect(
				securityContext,
				domainmodel.Request{Messages: messages},
				false,
			)
			if err != nil {
				t.Fatal(err)
			}
			got := projected.Messages[len(projected.Messages)-1].Content
			if got == content || strings.Contains(got, "12345678901234") ||
				strings.Contains(got, "srow1_"+strings.Repeat("1", 64)) ||
				strings.Contains(got, strings.Repeat("2", 64)) {
				t.Fatalf("unbound semantic lookalike bypassed generic projection: %s", got)
			}
		})
	}
	forged := domainmodel.Request{Messages: []domainmodel.Message{{
		Role: "user", Content: content,
		PrivateProviderSemanticBinding: struct {
			Purpose string
		}{Purpose: domainnative.AccountFlowProviderModelPurposeV1},
	}}}
	if _, err := ProjectProviderRequestForEffect(securityContext, forged, false); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("non-host-owned semantic binding was accepted: %v", err)
	}
}

func providerPrivacyAccountFlowModelOutputV1() domainnative.AccountFlowProviderModelOutputV1 {
	output := domainnative.AccountFlowProviderModelOutputV1{
		SchemaVersion: 3, Purpose: domainnative.AccountFlowProviderModelPurposeV1,
		SemanticStatus: domainevidence.SemanticPartial,
		Data: domainnative.AccountFlowProviderSemanticResultV1{
			SubjectAlias:   "acct:1",
			StartInclusive: "2026-07-01T00:00:00.000000Z", EndInclusive: "2026-07-31T23:59:59.000000Z",
			Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
			InflowMinor: "12345678901234", OutflowMinor: "2345678901234", NetMinor: "10000000000000",
			TransactionCount: 2, EvidenceTransactionCount: 2, EvidenceRowLimit: 2,
			AggregateComplete: false, EvidenceRowsComplete: true, CounterpartySemanticsComplete: false,
			Currentness: domainnative.AccountFlowProviderCurrentnessCurrentV1,
			Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{
				State: domainnative.AccountFlowCoveragePartialV1,
				Gaps: []string{
					domainnative.AccountFlowGapDuplicateSourceRowsV1,
					domainnative.AccountFlowGapCounterpartyResolutionV1,
				},
				NormalizedSnapshotRows: 123456789012, AcceptedSnapshotRows: 2,
				DuplicateSnapshotRows: 123456789010, ObservedMatchingRows: 2,
			},
			Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{
				{
					EvidenceRef:  "srow1_" + strings.Repeat("1", 64),
					Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
					OccurredAt:   "2026-07-03T01:02:03.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
					AmountMinor: "12345678901234", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
				},
				{
					EvidenceRef:  "srow1_" + strings.Repeat("4", 64),
					Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
					OccurredAt:   "2026-07-04T01:02:03.000000Z", Direction: domainnative.AccountFlowDirectionOutflowV1,
					AmountMinor: "2345678901234", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
				},
			},
			QueryHash: strings.Repeat("2", 64), ResultHash: strings.Repeat("3", 64),
		},
	}
	outcome, err := domainnative.NewAccountFlowProviderOutcomeV1(
		output.Data.SubjectAlias, output.Data.AggregateComplete, output.Data.EvidenceRowsComplete,
		output.Data.QueryHash, output.Data.ResultHash,
	)
	if err != nil {
		panic(err)
	}
	output.Data.Outcome = outcome
	return output
}

func TestProjectProviderRequestAcceptsSensitiveLookingSchemaIdentifiersWithoutContent(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	request := domainmodel.Request{Tools: []domainmodel.ToolSchema{{
		Name:        "account_lookup",
		Description: "Look up a host-authorized reference.",
		Parameters: json.RawMessage(`{
			"$schema":"http://json-schema.org/draft-07/schema#",
			"type":"object",
			"properties":{
				"accountId":{"type":"string","format":"account-reference"},
				"apiKey":{"type":"string"},
				"password":{"type":["string","null"],"enum":["enabled","disabled",null]}
			},
			"required":["accountId","apiKey"],
			"additionalProperties":false
		}`),
		OutputSchema: json.RawMessage(`{
			"$schema":"http://json-schema.org/draft-07/schema#",
			"type":"object",
			"$defs":{"accountId":{"type":"string"}},
			"properties":{"accountId":{"$ref":"#/$defs/accountId"}},
			"required":["accountId"],
			"additionalProperties":false
		}`),
	}}}

	projected, err := ProjectProviderRequest(securityContext, request)
	if err != nil {
		t.Fatalf("schema identifiers were mistaken for credential or PII content: %v", err)
	}
	if string(projected.Tools[0].Parameters) != string(request.Tools[0].Parameters) ||
		string(projected.Tools[0].OutputSchema) != string(request.Tools[0].OutputSchema) {
		t.Fatal("provider schema contract was silently rewritten")
	}
	if err := ValidateProviderRequest(projected); err != nil {
		t.Fatalf("clean schema failed final provider validation: %v", err)
	}
	ordinaryProjected, err := ProjectProviderRequestForEffect(securityContext, request, true)
	if err != nil {
		t.Fatalf("clean draft-07 schema failed ordinary provider projection: %v", err)
	}
	if string(ordinaryProjected.Tools[0].Parameters) != string(request.Tools[0].Parameters) ||
		string(ordinaryProjected.Tools[0].OutputSchema) != string(request.Tools[0].OutputSchema) {
		t.Fatal("ordinary provider projection silently rewrote the draft-07 schema contract")
	}
}

func TestProjectProviderRequestProjectsCredentialsAcrossTextAndArguments(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	const (
		rawAccount = "6222020000000000000"
		rawToken   = "sk-providercredential123"
	)
	request := domainmodel.Request{
		SystemPrompt: "Authorization: Bearer " + rawToken + "; account " + rawAccount,
		Messages: []domainmodel.Message{{
			Role:    "assistant",
			Content: "credential=" + rawToken + " account=" + rawAccount,
			Parts:   []domainmodel.MessagePart{{Type: "text", Text: "token " + rawToken + " account " + rawAccount}},
			ToolCalls: []domainmodel.ToolCall{{
				ID:   "call-credential",
				Name: "lookup",
				Arguments: json.RawMessage(`{"authorization":"Bearer ` + rawToken + `","accountId":"` + rawAccount + `",` +
					`"note":"account ` + rawAccount + rawToken + `"}`),
			}},
		}},
	}

	projected, err := ProjectProviderRequest(securityContext, request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(projected.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(projected.SystemPrompt+string(body), rawToken) || strings.Contains(projected.SystemPrompt+string(body), rawAccount) {
		t.Fatalf("provider request retained credential or full PII: prompt=%q messages=%s", projected.SystemPrompt, body)
	}
	if !strings.Contains(projected.SystemPrompt+string(body), "<redacted>") || !strings.Contains(projected.SystemPrompt+string(body), "[ACCOUNT]") {
		t.Fatalf("provider request omitted required projection sentinels: prompt=%q messages=%s", projected.SystemPrompt, body)
	}
	if !strings.Contains(string(request.Messages[0].ToolCalls[0].Arguments), rawToken) {
		t.Fatal("provider projection mutated caller-owned tool arguments")
	}
	if err := ValidateProviderRequest(projected); err != nil {
		t.Fatalf("credential-safe request failed final validation: %v", err)
	}
}

func TestProjectProviderRequestRejectsPrivateStructuralKeys(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	unknownReference := "cer1_" + strings.Repeat("b", 64)
	for name, key := range map[string]string{
		"case reference":       unknownReference,
		"complete account":     "6222020000000000000",
		"relative source path": "private/case.duckdb/ledger",
		"windows source path":  `private\case.duckdb`,
		"SQL text":             "SELECT FROM transactions",
	} {
		t.Run(name, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]any{
				"outer": map[string]any{key: "value"},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := domainmodel.Request{Messages: []domainmodel.Message{{
				Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
					ID: "call-private-structural-key", Name: "lookup", Arguments: arguments,
				}},
			}}}
			if _, projectErr := ProjectProviderRequestForEffect(
				securityContext,
				request,
				false,
			); !errors.Is(projectErr, ErrProviderPrivacyAuthorityUnavailable) {
				t.Fatalf("private structural key crossed provider projection: %v", projectErr)
			}
			if validateErr := ValidateProviderRequest(request); !errors.Is(
				validateErr,
				ErrProviderPrivacyAuthorityUnavailable,
			) {
				t.Fatalf("private structural key crossed final validation: %v", validateErr)
			}
		})
	}
}

func TestProjectProviderRequestRejectsCaseReferencesInToolSchemaForEveryEffect(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	unknownReference := "cer1_" + strings.Repeat("2", 64)

	requests := map[string]domainmodel.Request{
		"name": {Tools: []domainmodel.ToolSchema{{
			Name: unknownReference, Description: "lookup",
			Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		}}},
		"description": {Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "lookup " + unknownReference,
			Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		}}},
		"property key": {Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "lookup",
			Parameters: json.RawMessage(`{"type":"object","properties":{"` + unknownReference + `":{"type":"string"}},"additionalProperties":false}`),
		}}},
		"title": {Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "lookup",
			Parameters: json.RawMessage(`{"type":"object","title":"` + unknownReference + `","additionalProperties":false}`),
		}}},
		"output id": {Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "lookup",
			Parameters:   json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"$id":"` + unknownReference + `","type":"object","additionalProperties":false}`),
		}}},
	}

	for name, request := range requests {
		for _, ordinaryEffect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ordinary=%t", name, ordinaryEffect), func(t *testing.T) {
				_, err := ProjectProviderRequestForEffect(
					securityContext,
					request,
					ordinaryEffect,
				)
				if !errors.Is(err, ErrProviderSchemaContainsPII) {
					t.Fatalf("case reference crossed provider schema boundary: %v", err)
				}
			})
		}
	}
}

func TestProjectProviderRequestRejectsCredentialBearingToolIdentityAndSchemas(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	const rawToken = "sk-providercredential123"
	for name, request := range map[string]domainmodel.Request{
		"tool call name": {
			Messages: []domainmodel.Message{{
				Role: "assistant",
				ToolCalls: []domainmodel.ToolCall{{
					ID: "call-secret-name", Name: "password=" + rawToken, Arguments: json.RawMessage(`{}`),
				}},
			}},
		},
		"schema name": {
			Tools: []domainmodel.ToolSchema{{
				Name: "password=" + rawToken, Description: "lookup", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		},
		"schema description": {
			Tools: []domainmodel.ToolSchema{{
				Name: "lookup", Description: "Authorization: Bearer " + rawToken, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		},
		"schema parameters": {
			Tools: []domainmodel.ToolSchema{{
				Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","description":"api_key=` + rawToken + `","additionalProperties":false}`),
			}},
		},
		"output schema": {
			Tools: []domainmodel.ToolSchema{{
				Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","description":"Authorization: Bearer ` + rawToken + `","additionalProperties":false}`),
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequest(securityContext, request)
			if err == nil {
				t.Fatal("credential-bearing provider contract was accepted")
			}
		})
	}
}

func TestProjectProviderRequestRejectsNonObjectOrInvalidToolJSON(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	for name, arguments := range map[string]json.RawMessage{
		"array":         json.RawMessage(`[]`),
		"scalar":        json.RawMessage(`"value"`),
		"duplicate key": json.RawMessage(`{"value":"first","value":"second"}`),
		"trailing data": json.RawMessage(`{} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequest(securityContext, domainmodel.Request{Messages: []domainmodel.Message{{
				Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call-invalid", Name: "lookup", Arguments: arguments}},
			}}})
			if !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
				t.Fatalf("invalid provider JSON was not rejected fail-closed: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestRejectsNonObjectOrInvalidSchemaJSON(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	for name, parameters := range map[string]json.RawMessage{
		"array":         json.RawMessage(`[]`),
		"duplicate key": json.RawMessage(`{"type":"object","type":"string"}`),
		"trailing data": json.RawMessage(`{} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequest(securityContext, domainmodel.Request{Tools: []domainmodel.ToolSchema{{
				Name: "lookup", Description: "lookup", Parameters: parameters,
			}}})
			if !errors.Is(err, ErrProviderSchemaContainsPII) {
				t.Fatalf("invalid provider schema was not rejected fail-closed: %v", err)
			}
		})
	}
}

func TestValidateProviderRequestRejectsUnprojectedCredentialAndPII(t *testing.T) {
	const (
		rawAccount = "6222020000000000000"
		rawToken   = "sk-providercredential123"
	)
	for name, request := range map[string]domainmodel.Request{
		"message credential": {Messages: []domainmodel.Message{{Role: "user", Content: "Bearer " + rawToken}}},
		"arguments PII": {Messages: []domainmodel.Message{{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
			ID: "call-pii", Name: "lookup", Arguments: json.RawMessage(`{"accountId":"` + rawAccount + `"}`),
		}}}}},
		"schema credential": {Tools: []domainmodel.ToolSchema{{
			Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","default":"Bearer ` + rawToken + `","additionalProperties":false}`),
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProviderRequest(request); err == nil {
				t.Fatal("unprojected provider request passed final validation")
			}
		})
	}
}

func TestProjectProviderRequestRejectsUninspectableCaseParts(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	const rawSentinel = "PRIVATE_CASE_IMAGE_SENTINEL_731"
	for name, part := range map[string]domainmodel.MessagePart{
		"remote image URL": {Type: "image_url", ImageURL: "https://private.example.test/" + rawSentinel},
		"data URL":         {Type: "image_url", ImageURL: "data:image/png;base64," + rawSentinel},
		"inline data":      {Type: "image", Data: rawSentinel},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequest(securityContext, domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Parts: []domainmodel.MessagePart{part}}}})
			if !errors.Is(err, ErrUninspectableProviderPart) {
				t.Fatalf("uninspectable part was not rejected: %v", err)
			}
			if strings.Contains(err.Error(), rawSentinel) {
				t.Fatalf("uninspectable rejection reflected private payload: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestRejectsUninspectableGeneralParts(t *testing.T) {
	securityContext := providerPrivacyGeneralContext(t)
	const rawSentinel = "PRIVATE_GENERAL_IMAGE_SENTINEL_731"
	for name, part := range map[string]domainmodel.MessagePart{
		"remote image URL": {Type: "image_url", ImageURL: "https://private.example.test/" + rawSentinel},
		"data URL":         {Type: "image_url", ImageURL: "data:image/png;base64," + rawSentinel},
		"inline data":      {Type: "image", Data: rawSentinel},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequestForEffect(
				securityContext,
				domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Parts: []domainmodel.MessagePart{part}}}},
				true,
			)
			if !errors.Is(err, ErrUninspectableProviderPart) {
				t.Fatalf("uninspectable general part was not rejected: %v", err)
			}
			if strings.Contains(err.Error(), rawSentinel) {
				t.Fatalf("uninspectable general rejection reflected private payload: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestRejectsInternalBase64MediaEnvelopes(t *testing.T) {
	securityContext := providerPrivacyGeneralContext(t)
	for name, content := range map[string]string{
		"legacy attachment fallback": "[Attached file]\nMIME: image/png\nBase64:\n```base64\nUFJJVkFURV9JTUFHRQ==\n```\n[/Attached file]",
		"legacy SDD image map":       "Image Reference Map:\nImage 1: wireframe.png\nBase64:\n```base64\nUFJJVkFURV9JTUFHRQ==\n```",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequestForEffect(
				securityContext,
				domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: content}}},
				true,
			)
			if !errors.Is(err, ErrUninspectableProviderPart) {
				t.Fatalf("internal encoded media envelope was not rejected: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestRejectsPIIInAdvertisedSchema(t *testing.T) {
	securityContext := providerPrivacyCaseContext(t)
	const (
		rawAccount = "6222020000000000000"
		rawToken   = "sk-providercredential123"
	)
	for name, schema := range map[string]domainmodel.ToolSchema{
		"description":             {Name: "lookup", Description: "Example account " + rawAccount, Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
		"parameter title":         {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","title":"account ` + rawAccount + `","additionalProperties":false}`)},
		"parameter default":       {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"accountId":{"type":"string","default":"` + rawAccount + `"}},"additionalProperties":false}`)},
		"numeric account default": {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"accountId":{"type":"number","default":` + rawAccount + `}},"additionalProperties":false}`)},
		"parameter examples":      {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","examples":[{"authorization":"Bearer ` + rawToken + `"}],"additionalProperties":false}`)},
		"parameter const":         {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"accountId":{"const":"` + rawAccount + `"}},"additionalProperties":false}`)},
		"output default": {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"authorization":{"default":"Bearer ` + rawToken + `"}},"additionalProperties":false}`)},
		"malformed additionalProperties": {Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object","additionalProperties":"false"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProjectProviderRequest(securityContext, domainmodel.Request{Tools: []domainmodel.ToolSchema{schema}})
			if !errors.Is(err, ErrProviderSchemaContainsPII) {
				t.Fatalf("PII-bearing schema was not rejected: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestProjectsGeneralOrdinaryTextWithoutMintingCaseAuthority(t *testing.T) {
	securityContext := providerPrivacyGeneralContext(t)
	const (
		rawAccount = "6222020000000000000"
		rawToken   = "sk-providercredential123"
	)
	request := domainmodel.Request{
		SystemPrompt: "Authorization: Bearer " + rawToken,
		Messages: []domainmodel.Message{{
			Role:    "user",
			Content: "ordinary request mentions account " + rawAccount,
			Parts:   []domainmodel.MessagePart{{Type: "text", Text: "credential " + rawToken + " account " + rawAccount}},
		}},
		Tools: []domainmodel.ToolSchema{{
			Name: "read", Description: "Read an ordinary workspace file.",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		}},
	}

	projected, err := ProjectProviderRequestForEffect(securityContext, request, true)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(struct {
		SystemPrompt string
		Messages     []domainmodel.Message
		Tools        []domainmodel.ToolSchema
	}{
		SystemPrompt: projected.SystemPrompt,
		Messages:     projected.Messages,
		Tools:        projected.Tools,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), rawAccount) || strings.Contains(string(body), rawToken) {
		t.Fatal("general provider request retained a complete identifier or credential")
	}
	if projected.SystemPrompt == request.SystemPrompt ||
		projected.Messages[0].Content == request.Messages[0].Content ||
		projected.Messages[0].Parts[0].Text == request.Messages[0].Parts[0].Text {
		t.Fatal("general provider request omitted a credential or identifier projection lane")
	}
	if !strings.Contains(request.Messages[0].Content, rawAccount) || !strings.Contains(request.SystemPrompt, rawToken) {
		t.Fatal("general provider projection mutated its caller-owned request")
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatal("general privacy projection minted case evidence authority")
	}
	if err := ValidateProviderRequest(projected); err != nil {
		t.Fatalf("projected general request failed validation: %v", err)
	}
}

func TestProjectProviderRequestProjectsAbsentContextWithoutMintingCaseAuthority(t *testing.T) {
	const rawAccount = "6222020000000000000"
	request := domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "ordinary text account " + rawAccount}}}
	projected, err := ProjectProviderRequest(domainsecurity.TurnSecurityContext{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(projected.Messages[0].Content, rawAccount) ||
		projected.Messages[0].Content == request.Messages[0].Content {
		t.Fatal("absent context bypassed ordinary provider privacy projection")
	}
	protected := domainmodel.Request{Tools: []domainmodel.ToolSchema{{
		Name: "mcp__analytix_funds__count_case_rows", Description: "protected",
		Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}}
	if _, err := ProjectProviderRequest(domainsecurity.TurnSecurityContext{}, protected); !errors.Is(err, ErrOrdinaryProviderContainsCaseEffect) {
		t.Fatalf("absent context minted case provider authority: %v", err)
	}
}

func TestProjectProviderRequestRejectsInvalidOrMisclassifiedGeneralContext(t *testing.T) {
	securityContext := providerPrivacyGeneralContext(t)
	request := domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "ordinary text"}}}
	if _, err := ProjectProviderRequestForEffect(securityContext, request, false); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("general provider request accepted a non-ordinary effect classification: %v", err)
	}
	securityContext.ContextEpoch = 0
	if _, err := ProjectProviderRequestForEffect(securityContext, request, true); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("ordinary provider request accepted an invalid nonzero context: %v", err)
	}
}

func TestProjectProviderRequestGeneralOrdinaryRejectsCaseEffects(t *testing.T) {
	securityContext := providerPrivacyGeneralContext(t)
	for name, protected := range map[string]domainmodel.Request{
		"advertised funds schema": {
			Tools: []domainmodel.ToolSchema{{
				Name: "mcp__analytix_funds__count_case_rows", Description: "protected",
				Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		},
		"historical funds call": {
			Messages: []domainmodel.Message{{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
				ID: "call-funds", Name: "mcp__analytix_funds__count_case_rows", Arguments: json.RawMessage(`{}`),
			}}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectProviderRequestForEffect(securityContext, protected, true); !errors.Is(err, ErrOrdinaryProviderContainsCaseEffect) {
				t.Fatalf("general ordinary provider accepted a protected case effect: %v", err)
			}
		})
	}
}

func TestProjectProviderRequestAllowsOnlyProjectedOrdinaryEffectAtCaseBoundary(t *testing.T) {
	securityContext := providerPrivacyOrdinaryBoundaryContext(t)
	rawAccount := "6222020000000000000"
	request := domainmodel.Request{
		Messages: []domainmodel.Message{{Role: "user", Content: "修改代码；案件账号 " + rawAccount + " 暂无授权"}},
		Tools: []domainmodel.ToolSchema{{
			Name: "read", Description: "Read an ordinary workspace file.",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		}},
	}
	projected, err := ProjectProviderRequestForEffect(securityContext, request, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(projected.Messages[0].Content, rawAccount) || !strings.Contains(projected.Messages[0].Content, "[ACCOUNT]") {
		t.Fatalf("ordinary boundary request was not privacy projected: %#v", projected.Messages)
	}
	if _, err := ProjectProviderRequest(securityContext, request); !errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("strict case projection accepted boundary-only authority: %v", err)
	}

	for name, protected := range map[string]domainmodel.Request{
		"advertised funds schema": {
			Tools: []domainmodel.ToolSchema{{
				Name: "mcp__analytix_funds__count_case_rows", Description: "protected",
				Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		},
		"historical funds call": {
			Messages: []domainmodel.Message{{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
				ID: "call-funds", Name: "mcp__analytix_funds__count_case_rows", Arguments: json.RawMessage(`{}`),
			}}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectProviderRequestForEffect(securityContext, protected, true); !errors.Is(err, ErrOrdinaryProviderContainsCaseEffect) {
				t.Fatalf("ordinary provider accepted protected case effect: %v", err)
			}
		})
	}
}

func TestProviderCatalogForLogicalEffectSeparatesOrdinaryCaseAndFunds(t *testing.T) {
	advertisements := []domainmcp.ToolAdvertisementV1{
		{Name: "mcp__docs__lookup"},
		{Name: "mcp__analytix_funds__analyze_account_flows"},
	}
	schemas := []domainmodel.ToolSchema{
		{Name: "read"},
		{Name: "stage_case_report"},
		{Name: "mcp__docs__lookup"},
		{Name: "mcp__analytix_funds__analyze_account_flows"},
	}

	ordinaryAdvertisements, ordinarySchemas, err := ProviderCatalogForLogicalEffectV1(
		domainsecurity.LogicalEffectOrdinary, advertisements, schemas,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := providerCatalogNamesV1(ordinaryAdvertisements, ordinarySchemas); strings.Join(got, ",") != "mcp__docs__lookup,read,mcp__docs__lookup" {
		t.Fatalf("ordinary catalog = %v", got)
	}

	caseAdvertisements, caseSchemas, err := ProviderCatalogForLogicalEffectV1(
		domainsecurity.LogicalEffectCaseData, advertisements, schemas,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := providerCatalogNamesV1(caseAdvertisements, caseSchemas); strings.Join(got, ",") != "mcp__docs__lookup,read,stage_case_report,mcp__docs__lookup" {
		t.Fatalf("case-data catalog = %v", got)
	}

	fundsAdvertisements, fundsSchemas, err := ProviderCatalogForLogicalEffectV1(
		domainsecurity.LogicalEffectFundsData, advertisements, schemas,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := providerCatalogNamesV1(fundsAdvertisements, fundsSchemas); strings.Join(got, ",") != "mcp__docs__lookup,mcp__analytix_funds__analyze_account_flows,read,stage_case_report,mcp__docs__lookup,mcp__analytix_funds__analyze_account_flows" {
		t.Fatalf("funds-data catalog = %v", got)
	}

	if projectedAdvertisements, projectedSchemas, err := ProviderCatalogForLogicalEffectV1(
		domainsecurity.LogicalEffect("unknown"), advertisements, schemas,
	); !errors.Is(err, ErrProviderLogicalEffectInvalid) || projectedAdvertisements != nil || projectedSchemas != nil {
		t.Fatalf("invalid logical effect did not fail closed: advertisements=%v schemas=%v err=%v", projectedAdvertisements, projectedSchemas, err)
	}
}

func providerCatalogNamesV1(
	advertisements []domainmcp.ToolAdvertisementV1,
	schemas []domainmodel.ToolSchema,
) []string {
	names := make([]string, 0, len(advertisements)+len(schemas))
	for _, advertisement := range advertisements {
		names = append(names, advertisement.Name)
	}
	for _, schema := range schemas {
		names = append(names, schema.Name)
	}
	return names
}

func providerPrivacyCaseContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	threadID, turnID := "thread-provider-privacy", "turn-provider-privacy"
	workspace, caseID := "/tmp/analytix-provider-privacy", "case-provider-privacy"
	riskDigest := domainsecurity.SHA256Hex([]byte("provider-privacy-risk"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: riskDigest,
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:       domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"provider-privacy-binding-observation",
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, riskDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: domainsecurity.SHA256Hex([]byte("provider-privacy-case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("provider-privacy"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("provider-privacy-source-manifest")),
		ContextEpoch:       1, IssuedAt: time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC),
		PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		t.Fatalf("invalid case context: %#v err=%v", securityContext, err)
	}
	return securityContext
}

func providerPrivacyGeneralContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-provider-general", TurnID: "turn-provider-general",
		WorkspaceRealPath: "/tmp/analytix-provider-general",
		TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		t.Fatalf("invalid general context: %#v err=%v", securityContext, err)
	}
	return securityContext
}

func providerPrivacyOrdinaryBoundaryContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	threadID, turnID := "thread-provider-boundary", "turn-provider-boundary"
	workspace := "/tmp/analytix-provider-boundary"
	riskDigest := domainsecurity.SHA256Hex([]byte("provider-boundary-risk"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: riskDigest,
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:       domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"provider-boundary-binding-observation",
		)),
		BlockerCode: domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, riskDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC),
		PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
	if err != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		t.Fatalf("invalid ordinary boundary context: %#v err=%v", securityContext, err)
	}
	return securityContext
}

func TestContinuationProviderReferenceSurvivesFinalPrivacyProjection(t *testing.T) {
	// A decimal-heavy SHA is legal authority, but is not safe prose. The wire
	// reference must survive the same projection as source text and tool args.
	raw := strings.Repeat("9", 64)
	snapshot := domainthread.TaskContinuationSnapshotV1{StateDigest: raw, UserHistory: &domainthread.ContinuationUserHistoryV1{Version: "continuation-user-history.v1", ScopeDigest: raw, Sources: []domainthread.ContinuationUserSourceV1{{Reference: raw, Digest: raw, Text: strings.Repeat("original context ", 2000)}}}}
	view := domainthread.ProviderContinuationMapV1(snapshot)
	body, _ := json.Marshal(view)
	refs := view["userHistory"].(map[string]any)["sources"].([]map[string]any)
	ref := refs[0]["reference"].(string)
	args, _ := json.Marshal(map[string]any{"reference": ref})
	request := domainmodel.Request{Messages: []domainmodel.Message{
		{Role: "user", Content: string(body)},
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "read-source", Name: "read_task_history", Arguments: args}}},
		{Role: "tool", ToolCallID: "read-source", Name: "read_task_history", Content: string(args)},
	}}
	projected, err := ProjectProviderRequestForEffect(domainsecurity.TurnSecurityContext{}, request, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal([]byte(projected.Messages[0].Content), &got) != nil {
		t.Fatal("projection lost JSON")
	}
	sources := got["userHistory"].(map[string]any)["sources"].([]any)
	if sources[0].(map[string]any)["reference"] != ref || string(projected.Messages[1].ToolCalls[0].Arguments) != string(args) || projected.Messages[2].Content != string(args) {
		t.Fatal("final privacy projection changed the resolvable source reference")
	}
	if got["sourceSnapshotReference"] != view["sourceSnapshotReference"] {
		t.Fatal("snapshot identity changed in final projection")
	}
	if ProjectOrdinaryText("account 6222020000000000000") == "account 6222020000000000000" {
		t.Fatal("ordinary PII projection was relaxed")
	}
	if snapshot.UserHistory.Sources[0].Reference != raw {
		t.Fatal("provider view rewrote sealed source")
	}
}
