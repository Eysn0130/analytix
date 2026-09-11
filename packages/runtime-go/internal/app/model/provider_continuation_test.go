package model

import (
	"encoding/json"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestProviderContinuationSafeShapeDigestExcludesPrivateContentButBindsSafeShape(t *testing.T) {
	messages := []domainmodel.Message{
		{Role: "user", Content: "case prompt with full account 6222020000000000000"},
		{
			Role: "assistant",
			Parts: []domainmodel.MessagePart{
				{Type: "thinking", Text: "private reasoning", Signature: "private-signature"},
				{Type: "text", Text: "proposed text"},
			},
			ToolCalls: []domainmodel.ToolCall{{
				ID:        "call-1",
				Name:      "mcp__funds__query",
				Arguments: json.RawMessage(`{"account":"6222020000000000000"}`),
			}},
		},
		{Role: "tool", Name: "mcp__funds__query", ToolCallID: "call-1", Content: "private tool result"},
	}
	manifestHash := domainsecurity.SHA256Hex([]byte("tool-manifest"))
	registryDigest := domainsecurity.SHA256Hex([]byte("registry"))
	baseline := ProviderContinuationSafeShapeDigest(messages, manifestHash, registryDigest, 1)
	if !domainsecurity.IsSHA256Hex(baseline) {
		t.Fatalf("expected safe continuation digest, got %q", baseline)
	}

	privateChanged := append([]domainmodel.Message(nil), messages...)
	privateChanged[0].Content = "different private prompt"
	privateChanged[1].Parts = append([]domainmodel.MessagePart(nil), messages[1].Parts...)
	privateChanged[1].Parts[0].Text = "different private reasoning"
	privateChanged[1].Parts[0].Signature = "different-private-signature"
	privateChanged[1].Parts[1].Text = "different proposed text"
	privateChanged[2].Content = "different private tool result"
	if got := ProviderContinuationSafeShapeDigest(privateChanged, manifestHash, registryDigest, 1); got != baseline {
		t.Fatalf("private content changed safe digest: got %s want %s", got, baseline)
	}

	callChanged := append([]domainmodel.Message(nil), messages...)
	callChanged[1].ToolCalls = append([]domainmodel.ToolCall(nil), messages[1].ToolCalls...)
	callChanged[1].ToolCalls[0].ID = "call-2"
	if got := ProviderContinuationSafeShapeDigest(callChanged, manifestHash, registryDigest, 1); got == baseline {
		t.Fatal("tool-call identity did not change continuation digest")
	}
	if got := ProviderContinuationSafeShapeDigest(messages, manifestHash, registryDigest, 2); got == baseline {
		t.Fatal("provider call sequence did not change continuation digest")
	}
	attachmentPlanDigest := domainsecurity.SHA256Hex([]byte("attachment-plan"))
	exact, err := ProviderContinuationPayloadBytes("system-a", messages, attachmentPlanDigest, manifestHash, registryDigest, 1)
	if err != nil {
		t.Fatalf("build exact provider continuation payload: %v", err)
	}
	changedExact, err := ProviderContinuationPayloadBytes("system-a", privateChanged, attachmentPlanDigest, manifestHash, registryDigest, 1)
	if err != nil || string(changedExact) == string(exact) {
		t.Fatalf("exact payload did not bind private semantic content: err=%v", err)
	}
	changedSystem, err := ProviderContinuationPayloadBytes("system-b", messages, attachmentPlanDigest, manifestHash, registryDigest, 1)
	if err != nil || string(changedSystem) == string(exact) {
		t.Fatalf("exact payload did not bind system prompt: err=%v", err)
	}
	changedPlan, err := ProviderContinuationPayloadBytes("system-a", messages, domainsecurity.SHA256Hex([]byte("other-plan")), manifestHash, registryDigest, 1)
	if err != nil || string(changedPlan) == string(exact) {
		t.Fatalf("exact payload did not bind attachment plan: err=%v", err)
	}
}

func TestProviderContinuationCallRouteHashBindsPromptAndToolManifest(t *testing.T) {
	config := domainmodel.TurnConfig{ProviderID: "provider", Family: "deepseek", EndpointFormat: "chat-completions", BaseURL: "https://example.invalid", Model: "model"}
	manifest := domainsecurity.SHA256Hex([]byte("manifest"))
	baseline := ProviderContinuationCallRouteHash(config, "agent", manifest)
	if !domainsecurity.IsSHA256Hex(baseline) {
		t.Fatalf("invalid provider continuation route hash: %q", baseline)
	}
	if ProviderContinuationCallRouteHash(config, "plan", manifest) == baseline ||
		ProviderContinuationCallRouteHash(config, "agent", domainsecurity.SHA256Hex([]byte("other"))) == baseline {
		t.Fatal("provider continuation route did not bind prompt route and manifest")
	}
}

func TestProviderContinuationRouteHashRejectsInvalidReasoningEffortWithoutNormalization(t *testing.T) {
	base := domainmodel.TurnConfig{
		ProviderID: "provider", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: "https://example.invalid", Model: "model", ReasoningEffort: "high",
	}
	if hash := ProviderContinuationRouteHash(base); !domainsecurity.IsSHA256Hex(hash) {
		t.Fatalf("valid exact effort did not produce route hash: %q", hash)
	}
	for _, effort := range []string{" high ", "HIGH", "adaptive", "xhigh"} {
		config := base
		config.ReasoningEffort = effort
		if hash := ProviderContinuationRouteHash(config); hash != "" {
			t.Fatalf("invalid effort was normalized into continuation authority: effort=%q hash=%q", effort, hash)
		}
	}
}
