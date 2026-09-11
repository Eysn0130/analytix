package server

import (
	"strings"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
)

func TestRuntimeModelExecutionRefRequiresConcreteProviderModel(t *testing.T) {
	if _, err := appmodel.ExecutionRef("", "model-a", "", "chat_completions", "https://provider.example/v1", "thread"); err == nil || !strings.Contains(err.Error(), "providerId") {
		t.Fatalf("missing providerId should be rejected, got %v", err)
	}
	if _, err := appmodel.ExecutionRef("provider-a", "", "", "chat_completions", "https://provider.example/v1", "thread"); err == nil || !strings.Contains(err.Error(), "modelId") {
		t.Fatalf("missing modelId should be rejected, got %v", err)
	}
	ref, err := appmodel.ExecutionRef("provider-a", "model-a", "fast", "custom_endpoint", "https://provider.example/v1/chat/completions", "runtime-default")
	if err != nil {
		t.Fatalf("valid model execution ref rejected: %v", err)
	}
	if ref["providerId"] != "provider-a" ||
		ref["modelId"] != "model-a" ||
		ref["variant"] != "fast" ||
		ref["endpointFormat"] != "custom_endpoint" ||
		ref["source"] != "runtime-default" ||
		stringField(ref, "customFullEndpointFingerprint") == "" ||
		stringField(ref, "capabilityFingerprint") == "" ||
		stringField(ref, "resolvedAt") == "" {
		t.Fatalf("model execution ref should be provider-bound and runtime-default aware: %#v", ref)
	}
	if source := appmodel.ExecutionSourceName("unexpected-source"); source != "runtime-default" {
		t.Fatalf("unexpected source should resolve to runtime-default, got %q", source)
	}
}
