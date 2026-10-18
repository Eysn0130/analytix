package responses

import (
	"reflect"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBodyCarriesHostOutputTokenBudget(t *testing.T) {
	body := Body(domainmodel.Request{Model: "bounded", MaxOutputTokens: 321})
	if body["max_output_tokens"] != 321 {
		t.Fatalf("responses output token budget missing: %#v", body)
	}
}

func TestBodyRequestsSummaryOnlyForExplicitEnabledResponsesReasoning(t *testing.T) {
	for _, test := range []struct {
		name, protocol, effort string
		want                   map[string]any
	}{
		{"low", "openai-responses", "low", map[string]any{"effort": "low", "summary": "auto"}},
		{"medium", "openai-responses", "medium", map[string]any{"effort": "medium", "summary": "auto"}},
		{"high", "openai-responses", "high", map[string]any{"effort": "high", "summary": "auto"}},
		{"max", "openai-responses", "max", map[string]any{"effort": "high", "summary": "auto"}},
		{"disabled", "openai-responses", "off", nil},
		{"unset", "openai-responses", "", nil},
		{"legacy-protocol", "", "high", map[string]any{"effort": "high"}},
		{"other-family", "anthropic-thinking", "high", nil},
		{"unknown-protocol", "unrecognized", "high", map[string]any{"effort": "high"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := Body(domainmodel.Request{Model: "synthetic", ReasoningProtocol: test.protocol, ReasoningEffort: test.effort})
			reasoning, present := body["reasoning"]
			if test.want == nil {
				if present {
					t.Fatal("disabled or unrelated reasoning gained an opt-in")
				}
			} else if !reflect.DeepEqual(reasoning, test.want) {
				t.Fatalf("reasoning = %#v, want %#v", reasoning, test.want)
			}
		})
	}
}
