package model

import (
	"strings"
	"testing"
)

func TestExecutionRefRequiresProviderAndModel(t *testing.T) {
	if _, err := ExecutionRef("", "model-a", "", "chat_completions", "https://provider.example/v1", "thread"); err == nil || !strings.Contains(err.Error(), "providerId") {
		t.Fatalf("expected providerId validation error, got %v", err)
	}
	if _, err := ExecutionRef("provider-a", "", "", "chat_completions", "https://provider.example/v1", "thread"); err == nil || !strings.Contains(err.Error(), "modelId") {
		t.Fatalf("expected modelId validation error, got %v", err)
	}
}

func TestExecutionRefPreservesCustomEndpointFingerprint(t *testing.T) {
	ref, err := ExecutionRef("provider-a", "model-a", "fast", "custom_endpoint", "https://provider.example/v1/chat/completions", "runtime-default")
	if err != nil {
		t.Fatalf("ExecutionRef failed: %v", err)
	}
	if ref["providerId"] != "provider-a" || ref["modelId"] != "model-a" || ref["variant"] != "fast" {
		t.Fatalf("unexpected ref identity: %#v", ref)
	}
	if ref["endpointFormat"] != "custom_endpoint" {
		t.Fatalf("expected custom endpoint format, got %#v", ref["endpointFormat"])
	}
	if ref["customFullEndpointFingerprint"] == "" || ref["baseUrlFingerprint"] != nil {
		t.Fatalf("expected custom full endpoint fingerprint only, got %#v", ref)
	}
	if ref["capabilityFingerprint"] == "" {
		t.Fatalf("expected capability fingerprint: %#v", ref)
	}
}

func TestProviderFamilyNormalizesEndpointFormat(t *testing.T) {
	if got := ProviderFamily("provider-a", "https://api.example/v1/messages", "claude-3", "messages"); got != "anthropic-compatible" {
		t.Fatalf("unexpected messages family: %s", got)
	}
	if got := ProviderFamily("provider-a", "https://api.example/custom", "model", "full_url"); got != "custom_endpoint" {
		t.Fatalf("unexpected custom family: %s", got)
	}
}
