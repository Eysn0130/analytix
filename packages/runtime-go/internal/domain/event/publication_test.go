package event

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

func TestPublicRecordRejectsInternalCaseEntityReferenceAtEveryPublicProjection(t *testing.T) {
	t.Parallel()
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{
		"kind": "accepted_final_label",
		"details": map[string]any{
			"text": "已核验主体 " + string(reference),
		},
	}

	err = ValidatePublicRecord(value)
	if !errors.Is(err, ErrInternalEntityReferenceProjection) ||
		!errors.Is(err, domaincaseentity.ErrPublicValueInternalReferenceV1) {
		t.Fatalf("internal reference crossed public-record validation: %v", err)
	}
	if public, ok := SanitizePublicValue(value, false); ok || public != nil {
		t.Fatalf("internal reference crossed public sanitation: %#v", public)
	}
}

func TestPublicRecordAllowsValidatedNaturalCaseEntityDisplayLabel(t *testing.T) {
	t.Parallel()
	label, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 4,
		SafeSuffix:    "5678",
		Institution:   "中国银行",
		AccountType:   "储蓄账户",
	})
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"kind": "accepted_final_label", "text": label.Text}

	if err := ValidatePublicRecord(value); err != nil {
		t.Fatalf("validated natural label was rejected: %v", err)
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok || public == nil {
		t.Fatal("validated natural label was withheld from public sanitation")
	}
	body, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), label.Text) || domaincaseentity.ContainsReferenceV1(string(body)) {
		t.Fatalf("validated natural label was not preserved safely: %s", body)
	}
}

func TestValidatePublicRecordRejectsPrivateReasoning(t *testing.T) {
	tests := []any{
		map[string]any{"kind": "assistant_reasoning_delta", "text": "private"},
		map[string]any{"kind": "item_completed", "item": map[string]any{"kind": "assistant_reasoning", "text": "private"}},
		map[string]any{"kind": "tool_call_ready", "item": map[string]any{"kind": "tool_call", "reasoningContent": "private"}},
		map[string]any{"kind": "assistant_text_delta", "text": "<think>private</think>public"},
		map[string]any{"kind": "tool_result", "output": `{"reasoning_content":"PRIVATE_REASONING_SENTINEL"}`},
		map[string]any{"kind": "tool_result", "output": `{"thinking_content":"PRIVATE_THINKING_SENTINEL"}`},
		map[string]any{"kind": "tool_result", "output": `{"reasoning":"PRIVATE_REASONING_SENTINEL"}`},
		map[string]any{"kind": "tool_result", "output": `{"type":"thinking","text":"PRIVATE_REASONING_SENTINEL"}`},
		map[string]any{"kind": "tool_result", "output": `{"payload":"{\"reasoning\":\"PRIVATE_REASONING_SENTINEL\"}"}`},
		map[string]any{"kind": "provider_item", "type": "response.reasoning_summary_text.delta", "delta": "private"},
		map[string]any{"kind": "provider_item", "item": map[string]any{"kind": "redacted_thinking", "data": "private"}},
		map[string]any{"kind": "tool_result", "output": map[string]any{"thinkingSignature": "PRIVATE_SIGNATURE_SENTINEL"}},
		map[string]any{"kind": "tool_call", "arguments": map[string]any{"Reasoning-Content": "PRIVATE_REASONING_SENTINEL"}},
		map[string]any{"kind": "provider_error", "error": "<think class=\"private\">PRIVATE_REASONING_SENTINEL</think>"},
		map[string]any{"kind": "provider_error", "error": "public</thi"},
		map[string]any{"kind": "provider_error", "error": "public<ThInK data-private='1'>PRIVATE_REASONING_SENTINEL</tHiNk >"},
		map[string]any{
			"kind":              "compaction",
			"schemaVersion":     float64(2),
			"reasoningExcluded": true,
			"summary":           `{"assistant_reasoning":"PRIVATE_REASONING_SENTINEL"}`,
		},
	}
	for _, test := range tests {
		if err := ValidatePublicRecord(test); !errors.Is(err, ErrPrivateReasoningPersistence) {
			t.Fatalf("private reasoning should be rejected: value=%#v err=%v", test, err)
		}
	}
}

func TestValidatePublicRecordRejectsCredentialsAndOrdinaryPII(t *testing.T) {
	credential := map[string]any{
		"kind":    "tool_progress",
		"message": "Authorization: Bearer opaque-event-secret-123",
	}
	if err := ValidatePublicRecord(credential); !errors.Is(err, ErrCredentialProjection) {
		t.Fatalf("credential error = %v, want credential projection rejection", err)
	}

	structuredCredential := map[string]any{
		"kind":    "tool_progress",
		"details": map[string]any{"apiKey": "opaque-event-secret-123"},
	}
	if err := ValidatePublicRecord(structuredCredential); !errors.Is(err, ErrCredentialProjection) {
		t.Fatalf("structured credential error = %v, want credential projection rejection", err)
	}

	restrictedPII := map[string]any{
		"kind":    "tool_progress",
		"message": "account number: 6222020202020202020",
	}
	if err := ValidatePublicRecord(restrictedPII); !errors.Is(err, ErrPrivacyProjection) {
		t.Fatalf("PII error = %v, want privacy projection rejection", err)
	}
}

func TestValidatePublicRecordPreservesTypedReasoningMarkupFailure(t *testing.T) {
	t.Parallel()
	err := ValidatePublicRecord(map[string]any{"kind": "provider_error", "error": "public</thi"})
	if !errors.Is(err, ErrPrivateReasoningPersistence) || !errors.Is(err, domainreasoningmarkup.ErrIncomplete) {
		t.Fatalf("ValidatePublicRecord() error = %v, want private-reasoning and incomplete causes", err)
	}
}

func TestValidatePublicRecordRejectsProviderAssistantDraft(t *testing.T) {
	values := []map[string]any{
		{
			"kind": "assistant_text_delta", "threadId": "thread-1", "turnId": "turn-1",
			"item": map[string]any{"kind": "assistant_text", "text": "unaccepted provider draft"},
		},
		{
			"kind": "item_completed", "threadId": "thread-1", "turnId": "turn-1",
			"item": map[string]any{
				"id": "item_turn-1_assistant_process_1", "turnId": "turn-1", "kind": "assistant_text",
				"status": "completed", "text": "legacy unaccepted provider draft",
			},
		},
	}
	for _, value := range values {
		if err := ValidatePublicRecord(value); !errors.Is(err, ErrAssistantDraftPersistence) {
			t.Fatalf("assistant draft should be rejected before persistence: value=%#v err=%v", value, err)
		}
	}
}

func TestValidatePublicRecordAllowsUserToMentionReasoningSyntax(t *testing.T) {
	value := map[string]any{
		"kind": "user_message",
		"text": "请解释 <think>、</thi 与 reasoning_content 字段的安全风险",
	}
	if err := ValidatePublicRecord(value); err != nil {
		t.Fatalf("user-authored text is untrusted input, not provider private reasoning: %v", err)
	}
}

func TestValidatePublicRecordAllowsReasoningUsageMetadata(t *testing.T) {
	for _, effort := range []string{"", "auto", "off", "low", "medium", "high", "max"} {
		value := map[string]any{
			"kind":            "usage",
			"reasoningEffort": effort,
			"usage":           map[string]any{"reasoningTokens": float64(42)},
		}
		if err := ValidatePublicRecord(value); err != nil {
			t.Fatalf("typed reasoning usage metadata %q should remain public: %v", effort, err)
		}
	}
	for _, effort := range []string{" high ", "HIGH", "unsupported"} {
		value := map[string]any{"kind": "usage", "reasoningEffort": effort}
		if err := ValidatePublicRecord(value); !errors.Is(err, ErrPrivateReasoningPersistence) {
			t.Fatalf("invalid reasoning effort %q was accepted: %v", effort, err)
		}
	}
}

func TestValidatePublicRecordAllowsOrdinaryPublicReasoningDiscussion(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"The model explains its reasoning and thinking in public terms.",
		"I was thinking: verify the source first.",
		"reasoning_content is a field name discussed in documentation.",
		`{"message":"I was thinking: verify the source first."}`,
	} {
		value := map[string]any{"kind": "provider_progress", "message": text}
		if err := ValidatePublicRecord(value); err != nil {
			t.Fatalf("ordinary public discussion %q was rejected: %v", text, err)
		}
	}
}

func TestSanitizePublicValueRejectsProviderNativeReasoningDiscriminator(t *testing.T) {
	t.Parallel()
	for _, value := range []map[string]any{
		{"type": "thinking", "text": "PRIVATE"},
		{"kind": "thinking_delta", "text": "PRIVATE"},
		{"type": "response.reasoning_text.delta", "delta": "PRIVATE"},
	} {
		if public, ok := SanitizePublicValue(value, false); ok || public != nil {
			t.Fatalf("provider-native reasoning discriminator crossed sanitation: %#v", public)
		}
	}
}

func TestSanitizePublicValueDropsLegacyEmptyReasoningEffort(t *testing.T) {
	value := map[string]any{
		"kind":  "thread_updated",
		"turns": []any{map[string]any{"id": "turn-1", "reasoningEffort": ""}},
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("legacy empty reasoningEffort should be migrated as unset")
	}
	body, _ := json.Marshal(public)
	if strings.Contains(string(body), "reasoningEffort") {
		t.Fatalf("legacy empty reasoningEffort survived sanitation: %s", body)
	}
}

func TestReasoningMetadataKeysCannotSmugglePrivateContent(t *testing.T) {
	values := []map[string]any{
		{"kind": "usage", "usage": map[string]any{"reasoningTokens": "PRIVATE_REASONING_SENTINEL"}},
		{"kind": "usage", "usage": map[string]any{"reasoningTokens": map[string]any{"value": "PRIVATE_REASONING_SENTINEL"}}},
		{"kind": "usage", "usage": map[string]any{"reasoningTokens": float64(1.5)}},
		{"kind": "usage", "usage": map[string]any{"reasoningTokens": float64(1 << 53)}},
		{"kind": "turn_started", "reasoningEffort": "PRIVATE_REASONING_SENTINEL"},
		{"kind": "usage", "reasoningDurationMs": float64(4)},
		{"kind": "compaction", "reasoningExcluded": "PRIVATE_REASONING_SENTINEL"},
		{"kind": "compaction", "reasoningExclusionProof": "sha256:" + strings.Repeat("a", 64) + "PRIVATE_REASONING_SENTINEL"},
	}
	for _, value := range values {
		if err := ValidatePublicRecord(value); !errors.Is(err, ErrPrivateReasoningPersistence) {
			t.Fatalf("invalid reasoning metadata was accepted: value=%#v err=%v", value, err)
		}
		if public, ok := SanitizePublicValue(value, false); ok || public != nil {
			t.Fatalf("invalid reasoning metadata crossed public sanitation: %#v", public)
		}
	}
}

func TestOrdinaryRecordRejectsNeutralWrappedRestrictedEvidence(t *testing.T) {
	value := map[string]any{
		"kind": "review_completed",
		"review": map[string]any{
			"output": map[string]any{
				"schemaVersion": 1,
				"purpose":       "analytix.source-row-lineage/v1",
				"lineageDigest": strings.Repeat("a", 64),
			},
		},
	}
	if err := ValidatePublicRecord(value); !errors.Is(err, ErrRestrictedEvidenceProjection) {
		t.Fatalf("restricted evidence reached an ordinary record: %v", err)
	}
	if public, ok := SanitizePublicValue(value, false); ok || public != nil {
		t.Fatalf("restricted evidence was sanitized into a public value: %#v", public)
	}
	if durable, ok := SanitizeDurableValue(value, false); ok || durable != nil {
		t.Fatalf("restricted evidence was sanitized into generic durable history: %#v", durable)
	}
}

func TestOrdinaryEventAndDurableProjectionRejectTypedLocalResponses(t *testing.T) {
	for _, test := range []struct {
		kind   string
		canary string
	}{
		{kind: "import_mapping_preview", canary: "NEUTRAL_EVENT_CANARY_IMPORT"},
		{kind: "cleaning_diff_preview", canary: "NEUTRAL_EVENT_CANARY_CLEANING"},
		{kind: "direct_source_preview", canary: "NEUTRAL_EVENT_CANARY_DIRECT"},
		{kind: "accepted_slot_display", canary: "NEUTRAL_EVENT_CANARY_ACCEPTED"},
	} {
		value := map[string]any{
			"kind": "review_completed",
			"payload": map[string]any{
				"schemaVersion": 1,
				"kind":          test.kind,
				"nested":        map[string]any{"displayValue": test.canary},
			},
		}
		if err := ValidatePublicRecord(value); !errors.Is(err, ErrRestrictedEvidenceProjection) {
			t.Fatalf("typed-local response reached ordinary event: kind=%s err=%v", test.kind, err)
		}
		if public, ok := SanitizePublicValue(value, false); ok || public != nil {
			t.Fatalf("typed-local response reached public event projection: kind=%s value=%#v", test.kind, public)
		}
		if durable, ok := SanitizeDurableValue(value, false); ok || durable != nil {
			t.Fatalf("typed-local response reached durable history: kind=%s value=%#v", test.kind, durable)
		}
	}
}

func TestOrdinaryRecordRejectsSerializedRestrictedReferenceButAllowsPublicHash(t *testing.T) {
	hash := strings.Repeat("b", 64)
	restricted := map[string]any{
		"kind":   "review_completed",
		"output": `{"rawArtifactManifestDigest":"` + hash + `","rawArtifactManifestSha256":"` + hash + `","rawArtifactManifestByteLength":42}`,
	}
	if err := ValidatePublicRecord(restricted); !errors.Is(err, ErrRestrictedEvidenceProjection) {
		t.Fatalf("serialized restricted reference reached an ordinary record: %v", err)
	}
	public := map[string]any{"kind": "publication_metric", "reportSha256": hash, "datasetSnapshotId": "snapshot-v2"}
	if err := ValidatePublicRecord(public); err != nil {
		t.Fatalf("ordinary public hashes were over-blocked: %v", err)
	}
}

func TestSanitizePublicValueRemovesNestedReasoningButPreservesUserText(t *testing.T) {
	value := map[string]any{
		"reasoningContent": "PRIVATE_TOP",
		"turns": []any{map[string]any{
			"thinking_content": "PRIVATE_TURN",
			"items": []any{
				map[string]any{"kind": "user_message", "text": "user literally wrote <think>example</think>"},
				map[string]any{"kind": "assistant_reasoning", "text": "PRIVATE_ITEM"},
				map[string]any{
					"id": "item_turn-1_assistant", "turnId": "turn-1", "kind": "assistant_text",
					"status": "completed", "text": "<think>PRIVATE_INLINE</think>public answer",
				},
			},
		}},
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("public sanitizer unexpectedly rejected the complete record")
	}
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{"PRIVATE_TOP", "PRIVATE_TURN", "PRIVATE_ITEM", "PRIVATE_INLINE", "reasoningContent", "thinking_content", "assistant_reasoning"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("sanitized record retained %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "user literally wrote") || !strings.Contains(serialized, "example") || !strings.Contains(serialized, "public answer") {
		t.Fatalf("sanitizer removed public/user-authored text: %s", serialized)
	}
	if err := ValidatePublicRecord(public); err != nil {
		t.Fatalf("sanitized value must satisfy the public boundary: %v", err)
	}
}

func TestSanitizePublicValueWithholdsOrdinaryToolArgumentBytes(t *testing.T) {
	value := map[string]any{
		"kind": "tool_call",
		"arguments": map[string]any{
			"path": " note.txt ", "content": "new line\nkeep\n",
		},
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("ordinary tool call projection was rejected")
	}
	arguments := public.(map[string]any)["arguments"].(map[string]any)
	if arguments["messageKey"] != "tool_arguments_withheld" || arguments["privatePayloadWithheld"] != true {
		t.Fatalf("public sanitation did not emit the closed argument projection: %#v", arguments)
	}
	if _, exists := arguments["path"]; exists {
		t.Fatalf("raw tool arguments crossed the public boundary: %#v", arguments)
	}
}

func TestSanitizeDurableValueWithholdsRawToolResultBytes(t *testing.T) {
	contextDigest := strings.Repeat("a", 64)
	grantID := strings.Repeat("b", 64)
	value := map[string]any{
		"kind": "item_completed", "threadId": "thread-1", "turnId": "turn-1", "seq": float64(7),
		"item": map[string]any{
			"id": "result-1", "threadId": "thread-1", "turnId": "turn-1", "kind": "tool_result",
			"role": "tool", "status": "completed", "toolName": "mcp__funds__query", "callId": "call-1", "isError": false,
			"contextDigest": contextDigest, "contextEpoch": uint64(4), "executionGrantId": grantID,
			"output": map[string]any{"account": "RAW_TOOL_RESULT_SENTINEL", "amount": 2645472},
			"media":  []any{map[string]any{"path": "/private/case/evidence.png"}},
		},
	}
	durable, ok := SanitizeDurableValue(value, false)
	if !ok {
		t.Fatal("durable tool-result projection was rejected")
	}
	body, _ := json.Marshal(durable)
	for _, forbidden := range []string{"RAW_TOOL_RESULT_SENTINEL", "/private/case/evidence.png", "\"amount\":2645472", "\"media\""} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("raw tool result survived durable projection %q: %s", forbidden, body)
		}
	}
	record := durable.(map[string]any)
	if record["seq"] != float64(7) {
		t.Fatalf("event cursor changed during nested projection: %#v", record)
	}
	item := record["item"].(map[string]any)
	if item["contextDigest"] != contextDigest || item["contextEpoch"] != uint64(4) || item["executionGrantId"] != grantID || item["callId"] != "call-1" {
		t.Fatalf("durable result lost grant settlement authority: %#v", item)
	}
	output := item["output"].(map[string]any)
	if output["messageKey"] != "legacy_output_withheld" || output["evidenceAuthority"] != false {
		t.Fatalf("durable result did not use the closed legacy projection: %#v", output)
	}
}

func TestSanitizePublicValueDropsNestedExecutionGrant(t *testing.T) {
	value := map[string]any{
		"kind":           "tool_call_ready",
		"executionGrant": map[string]any{"argsHash": "LOW_ENTROPY_ARGUMENT_HASH_SENTINEL"},
		"item": map[string]any{
			"kind": "tool_call", "toolName": "read_file", "callId": "call-1",
			"arguments":      map[string]any{"path": "/private/case/account.txt"},
			"executionGrant": map[string]any{"argsHash": "NESTED_GRANT_SENTINEL"},
		},
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("public tool-call-ready projection was rejected")
	}
	serialized, _ := json.Marshal(public)
	for _, forbidden := range []string{"LOW_ENTROPY_ARGUMENT_HASH_SENTINEL", "NESTED_GRANT_SENTINEL", "/private/case/account.txt", "executionGrant"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("public event retained %q: %s", forbidden, serialized)
		}
	}
}

func TestSanitizePublicValueStripsGeneralTerminalAuthorityButDurableRetainsIt(t *testing.T) {
	value := map[string]any{
		"id": "thread-1",
		"generalTerminalPublication": map[string]any{
			"commitId": "GENERAL_OUTBOX_SENTINEL",
		},
		"turns": []any{map[string]any{
			"id":                        "turn-1",
			"generalTerminalCASBinding": map[string]any{"bindingDigest": "GENERAL_BINDING_SENTINEL"},
			"items": []any{map[string]any{
				"kind": "assistant_text", "text": "public answer",
				"generalTerminalCASBinding": map[string]any{"bindingDigest": "NESTED_BINDING_SENTINEL"},
			}},
		}},
		"generalTerminalCommitId": "GENERAL_EVENT_SENTINEL",
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("public sanitizer rejected the record")
	}
	publicBody, _ := json.Marshal(public)
	for _, forbidden := range []string{
		"generalTerminal", "GENERAL_OUTBOX_SENTINEL", "GENERAL_BINDING_SENTINEL",
		"NESTED_BINDING_SENTINEL", "GENERAL_EVENT_SENTINEL",
	} {
		if strings.Contains(string(publicBody), forbidden) {
			t.Fatalf("public projection retained internal terminal authority %q: %s", forbidden, publicBody)
		}
	}
	if !strings.Contains(string(publicBody), "public answer") {
		t.Fatalf("public projection removed the accepted ordinary assistant text: %s", publicBody)
	}
	durable, ok := SanitizeDurableValue(value, false)
	if !ok {
		t.Fatal("durable sanitizer rejected the record")
	}
	durableBody, _ := json.Marshal(durable)
	for _, required := range []string{"GENERAL_OUTBOX_SENTINEL", "GENERAL_BINDING_SENTINEL", "GENERAL_EVENT_SENTINEL"} {
		if !strings.Contains(string(durableBody), required) {
			t.Fatalf("private durable projection lost recovery authority %q: %s", required, durableBody)
		}
	}
}

func TestSanitizePublicValueDropsNeutralWrappedPrivateTerminalContract(t *testing.T) {
	value := map[string]any{
		"kind": "error",
		"details": map[string]any{
			"archive": map[string]any{
				"schemaVersion": "general-terminal-publication-archive.v1",
				"purpose":       "analytix.general-terminal-publication-archive/v1",
				"archiveDigest": "NEUTRAL_ARCHIVE_DIGEST_SENTINEL",
			},
		},
	}
	public, ok := SanitizePublicValue(value, false)
	if !ok {
		t.Fatal("outer public record was rejected instead of dropping only the private child")
	}
	body, _ := json.Marshal(public)
	for _, forbidden := range []string{
		"general-terminal-publication-archive.v1",
		"analytix.general-terminal-publication-archive/v1",
		"NEUTRAL_ARCHIVE_DIGEST_SENTINEL",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("neutral wrapper leaked private terminal contract %q: %s", forbidden, body)
		}
	}
	if !ContainsPrivateTerminalAuthority(value) {
		t.Fatal("sidecar/import semantic detector missed a neutral-wrapped private terminal contract")
	}
}
