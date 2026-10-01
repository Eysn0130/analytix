package model

import (
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestNormalizeProviderResultBindsIdentityAndPrefixToHostRequest(t *testing.T) {
	request := domainmodel.Request{
		ProviderID: "host-provider", Family: "deepseek", EndpointFormat: "chat_completions", Model: "host-model", Route: "tool_agent",
		SystemPrompt: "stable", Tools: []domainmodel.ToolSchema{{Name: "read_file", Source: "builtin", Parameters: []byte(`{"type":"object","properties":{},"additionalProperties":false}`)}},
	}
	result := NormalizeProviderResult(request, domainmodel.Result{
		ProviderID: "SOL_PRIVATE_TRACE_7C", Family: "private-family", EndpointFormat: "private-format",
		RequestURL: "https://example.invalid/6222021234567890", RequestBodyFields: []string{"reasoning_content"},
		PrefixShape:         domainmodel.PrefixShape{ProviderID: "SOL_PRIVATE_TRACE_7C", Model: "private-model", ToolSourceIDs: []string{"private"}},
		Usage:               domainmodel.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12, FinishReason: "private branch alpha"},
		FirstTokenLatencyMs: 999, HasFirstTokenLatency: true, DurationMs: 999, HasDuration: true,
		FirstRawTextLatencyMs: 999, HasFirstRawTextLatency: true,
		FirstReasoningLatencyMs: 999, HasFirstReasoningLatency: true,
	})
	if result.ProviderID != request.ProviderID || result.Family != request.Family || result.EndpointFormat != request.EndpointFormat ||
		result.RequestURL != "" || len(result.RequestBodyFields) != 0 || result.PrefixShape.ProviderID != request.ProviderID ||
		result.PrefixShape.Model != request.Model || result.PrefixShape.Route != request.Route || result.Usage.FinishReason != "unknown" ||
		result.HasFirstTokenLatency || result.HasDuration || result.HasFirstRawTextLatency || result.HasFirstReasoningLatency ||
		result.FirstRawTextLatencyMs != 0 || result.FirstReasoningLatencyMs != 0 {
		t.Fatalf("provider result retained adapter authority: %#v", result)
	}
}
