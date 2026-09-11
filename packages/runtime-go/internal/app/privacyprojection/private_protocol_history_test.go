package privacyprojection

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPrivateProtocolSafeHistoryRebindsValidatedFundsAliases(t *testing.T) {
	context := providerPrivacyCaseContext(t)
	fundsCall := domainmodel.ToolCall{
		ID: "call_host_" + strings.Repeat("6", 64), Name: fundsAccountFlowToolNameV1,
		Arguments: json.RawMessage(`{}`),
	}
	readCall := domainmodel.ToolCall{
		ID: "call_host_" + strings.Repeat("7", 64), Name: "read",
		Arguments: json.RawMessage(`{"path":"note.txt"}`),
	}
	exact := providerPrivacyAccountFlowModelOutputV1()
	fundsMessage := domainmodel.Message{
		Role: "tool", Name: fundsCall.Name, ToolCallID: fundsCall.ID,
		Content: appmodel.ToolResultContentForModel(exact),
	}
	if applicable, err := BindAccountFlowProviderSemanticV1(
		context,
		fundsCall,
		exact,
		&fundsMessage,
	); err != nil || !applicable {
		t.Fatalf("bind funds semantics: applicable=%t err=%v", applicable, err)
	}
	const privateAccount = "6222021234567890123"
	authorityRef := "cer1_" + strings.Repeat("a", 64)
	safe, err := PrivateProtocolSafeHistoryV1(context, []domainmodel.Message{
		{Role: "user", Content: "analyze"},
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{fundsCall, readCall}},
		fundsMessage,
		{Role: "tool", Name: readCall.Name, ToolCallID: readCall.ID, Content: "account " + privateAccount + " ref " + authorityRef},
	})
	if err != nil {
		t.Fatalf("compile private protocol safe history: %v", err)
	}
	safe, err = PrivateProtocolSafeHistoryV1(context, safe)
	if err != nil {
		t.Fatalf("recompile private protocol safe history: %v", err)
	}
	semantics, err := AccountFlowProviderSemanticsForCaseDelegationV1(context, safe)
	if err != nil || len(semantics) != 1 || !reflect.DeepEqual(semantics[0], exact) {
		t.Fatalf("safe-history account-flow origin did not reopen the exact semantic: semantics=%#v err=%v", semantics, err)
	}
	projected, err := ProjectProviderRequestForEffect(
		context,
		domainmodel.Request{Messages: safe},
		false,
	)
	if err != nil {
		t.Fatalf("project rebound safe history: %v", err)
	}
	body, err := json.Marshal(projected.Messages)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{
		exact.Data.SubjectAlias,
		exact.Data.InflowMinor,
		exact.Data.QueryHash,
		exact.Data.Transactions[0].EvidenceRef,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("safe history lost trusted semantic %q: %s", expected, text)
		}
	}
	if strings.Contains(text, privateAccount) || strings.Contains(text, authorityRef) {
		t.Fatalf("safe history leaked raw account or authority ref: %s", text)
	}
	for _, message := range projected.Messages {
		if message.Role == "tool" || len(message.ToolCalls) != 0 ||
			message.PrivateProviderSemanticBinding != nil ||
			message.PrivateProviderReferenceBinding != nil {
			t.Fatalf("private protocol or sidecar crossed final projection: %#v", projected.Messages)
		}
	}
}

func TestAccountFlowSafeHistoryOriginFailsClosed(t *testing.T) {
	context := providerPrivacyCaseContext(t)
	call := domainmodel.ToolCall{
		ID: "call_host_" + strings.Repeat("8", 64), Name: fundsAccountFlowToolNameV1,
		Arguments: json.RawMessage(`{}`),
	}
	exact := providerPrivacyAccountFlowModelOutputV1()
	message := domainmodel.Message{
		Role: "tool", Name: call.Name, ToolCallID: call.ID,
		Content: appmodel.ToolResultContentForModel(exact),
	}
	if bound, err := BindAccountFlowProviderSemanticV1(context, call, exact, &message); err != nil || !bound {
		t.Fatalf("bind account-flow semantic: bound=%v err=%v", bound, err)
	}
	safe, err := PrivateProtocolSafeHistoryV1(context, []domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}}, message,
	})
	if err != nil || len(safe) != 1 {
		t.Fatalf("compile exact account-flow safe history: history=%#v err=%v", safe, err)
	}

	withRehashedContent := func(rewrite func(string) string) []domainmodel.Message {
		t.Helper()
		hostile := appmodel.CloneProviderMessages(safe)
		hostile[0].Content = rewrite(hostile[0].Content)
		binding := hostile[0].PrivateProviderSemanticBinding.(boundAccountFlowSafeHistoryV1)
		messageSHA256, digestErr := providerReferenceMessageSHA256V1(hostile[0])
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		binding.messageSHA256 = messageSHA256
		hostile[0].PrivateProviderSemanticBinding = binding
		return hostile
	}
	for name, hostile := range map[string][]domainmodel.Message{
		"wrong tool": withRehashedContent(func(content string) string {
			return strings.Replace(content, `"tool":"`+fundsAccountFlowToolNameV1+`"`, `"tool":"read"`, 1)
		}),
		"truncated": withRehashedContent(func(content string) string {
			return strings.TrimSuffix(content, "}") + `,"truncated":true}`
		}),
		"ambiguous": withRehashedContent(func(content string) string {
			return strings.Replace(content, `"status":"completed"`, `"status":"ambiguous_result_withheld"`, 1)
		}),
		"tampered content": withRehashedContent(func(content string) string {
			return strings.Replace(content, exact.Data.InflowMinor, "10000000000001", 1)
		}),
	} {
		t.Run(name, func(t *testing.T) {
			if semantics, err := AccountFlowProviderSemanticsForCaseDelegationV1(context, hostile); err == nil || semantics != nil {
				t.Fatalf("hostile safe envelope reopened account-flow semantics: semantics=%#v err=%v", semantics, err)
			}
		})
	}
	duplicate := append(appmodel.CloneProviderMessages(safe), appmodel.CloneProviderMessages(safe)...)
	if semantics, err := AccountFlowProviderSemanticsForCaseDelegationV1(context, duplicate); err == nil || semantics != nil {
		t.Fatalf("duplicate safe origin reopened account-flow semantics: semantics=%#v err=%v", semantics, err)
	}
	serialized, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	var restarted []domainmodel.Message
	if err := json.Unmarshal(serialized, &restarted); err != nil {
		t.Fatal(err)
	}
	if semantics, err := AccountFlowProviderSemanticsForCaseDelegationV1(context, restarted); err != nil || len(semantics) != 0 {
		t.Fatalf("durable restart reconstructed safe-history provenance: semantics=%#v err=%v", semantics, err)
	}
	arbitrary := []domainmodel.Message{{Role: "user", Content: safe[0].Content}}
	if semantics, err := AccountFlowProviderSemanticsForCaseDelegationV1(context, arbitrary); err != nil || len(semantics) != 0 {
		t.Fatalf("arbitrary user envelope acquired account-flow provenance: semantics=%#v err=%v", semantics, err)
	}
	if _, err := ProjectProviderRequestForEffect(context, domainmodel.Request{Messages: restarted}, false); err != nil &&
		!errors.Is(err, ErrProviderPrivacyAuthorityUnavailable) {
		t.Fatalf("restarted envelope failed with an unexpected error: %v", err)
	}
}

func TestPrivateProtocolSafeHistoryRejectsLegacyReferenceSidecars(t *testing.T) {
	context := providerPrivacyCaseContext(t)
	authorityRef := "cer1_" + strings.Repeat("b", 64)
	for name, messages := range map[string][]domainmodel.Message{
		"message": {{
			Role: "user", Content: authorityRef,
			PrivateProviderReferenceBinding: struct{ Purpose string }{Purpose: "legacy"},
		}},
		"tool result": {{
			Role: "tool", ToolCallID: "call_host_" + strings.Repeat("8", 64), Content: authorityRef,
			PrivateProviderReferenceBinding: struct{ Purpose string }{Purpose: "legacy"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if safe, err := PrivateProtocolSafeHistoryV1(context, messages); err == nil || safe != nil {
				t.Fatalf("legacy reference sidecar survived safe-history admission: safe=%#v err=%v", safe, err)
			}
		})
	}
}
