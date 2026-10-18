package evidence

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

func TestTerminalPublicationIntentCanonicalizesNumericTelemetry(t *testing.T) {
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt:      time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
		TerminalStatus: "completed",
		Usage: map[string]any{
			"inputTokens":  7,
			"outputTokens": int64(9_007_199_254_740_993),
			"nested":       []any{uint32(11), map[string]any{"cachedTokens": 13}},
		},
		CacheDiagnostics: map[string]any{
			"hits": 17,
		},
	}, "success")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := intent.Usage["inputTokens"].(json.Number); !ok || got != json.Number("7") {
		t.Fatalf("inputTokens was not canonicalized: %#v", intent.Usage["inputTokens"])
	}
	if got, ok := intent.Usage["outputTokens"].(json.Number); !ok || got != json.Number("9007199254740993") {
		t.Fatalf("outputTokens lost exact numeric representation: %#v", intent.Usage["outputTokens"])
	}

	body, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	var cloned TerminalPublicationIntent
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&cloned); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cloned, intent) {
		t.Fatalf("strict JSON clone changed canonical intent:\noriginal=%#v\ncloned=%#v", intent, cloned)
	}
}

func TestTerminalPublicationIntentRejectsNonJSONTelemetry(t *testing.T) {
	if _, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt:        time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
		TerminalStatus:   "completed",
		CacheDiagnostics: map[string]any{"invalid": func() {}},
	}, "success"); err == nil {
		t.Fatal("terminal publication intent accepted non-JSON telemetry")
	}
}

func TestPublicationIntentOutputCapIsOptionalAndTerminalBound(t *testing.T) {
	input := TerminalPublicationIntentInput{CreatedAt: time.Unix(1, 0).UTC().Format(time.RFC3339Nano), TerminalStatus: "completed"}
	old, err := NewTerminalPublicationIntent(input, "success")
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, _ := json.Marshal(old)
	legacyBytes := []byte(`{"schemaVersion":1,"createdAt":"1970-01-01T00:00:01Z","usage":{},"cacheDiagnostics":{},"terminalStatus":"completed","discard":false,"cancelled":false,"cancelledPendingGates":0}`)
	if !bytes.Equal(oldBytes, legacyBytes) {
		t.Fatal("legacy publication signing bytes changed")
	}
	input.EndReason = &domainterminal.EndReason{Kind: domainterminal.EndReasonMaxTokens}
	intent, err := NewTerminalPublicationIntent(input, "success")
	if err != nil || intent.EndReason == nil {
		t.Fatal("valid cap intent rejected")
	}
	if _, err := NewTerminalPublicationIntent(input, "approval_denied"); err == nil {
		t.Fatal("cap overrode denial")
	}
	input.EndReason.Kind = "private-provider-reason"
	if _, err := NewTerminalPublicationIntent(input, "success"); err == nil {
		t.Fatal("unknown cap vocabulary accepted")
	}
}

func TestTerminalPublicationIntentAcceptsClosedAttemptTelemetryPairWithoutPayloadChanges(t *testing.T) {
	telemetry := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, map[string]any{
		"providerAttemptTelemetrySchema": domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1,
		"providerAttemptTelemetryValid":  false, "providerAttemptTelemetryError": "invalid_or_incomplete", "providerAttemptInventoryComplete": false,
	})
	input := TerminalPublicationIntentInput{CreatedAt: "2026-10-18T01:02:03Z", TerminalStatus: "completed", Usage: telemetry.PublicUsageMap(), CacheDiagnostics: telemetry.PublicCacheDiagnosticsMap()}
	before, err := json.Marshal([]any{input.Usage, input.CacheDiagnostics})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTerminalPublicationIntent(input, "success")
	if err != nil || ValidateTerminalPublicationIntent(intent, "success") != nil {
		t.Fatalf("closed real telemetry pair was rejected: %v", err)
	}
	persisted, err := json.Marshal([]any{intent.Usage, intent.CacheDiagnostics})
	if err != nil || !bytes.Equal(before, persisted) || intent.CacheDiagnostics["providerAttemptTelemetryError"] != "invalid_or_incomplete" {
		t.Fatal("scan-only view changed terminal publication payload")
	}
	after, err := json.Marshal([]any{input.Usage, input.CacheDiagnostics})
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("terminal publication mutated its original input maps")
	}
}

func TestTerminalPublicationIntentRejectsUnsafeAttemptTelemetryAndOriginalBudgetOverflow(t *testing.T) {
	for _, fault := range []string{"wrong usage", "unknown usage", "unknown diagnostic", "raw enum", "raw sibling", "credential usage", "credential diagnostics", "reasoning usage", "reasoning diagnostics", "nested diagnostics", "usage string budget", "diagnostic string budget", "diagnostic node budget", "diagnostic depth budget"} {
		t.Run(fault, func(t *testing.T) {
			telemetry := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, map[string]any{
				"providerAttemptTelemetrySchema": domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1,
				"providerAttemptTelemetryValid":  false, "providerAttemptTelemetryError": "invalid_or_incomplete", "providerAttemptInventoryComplete": false,
			})
			input := TerminalPublicationIntentInput{CreatedAt: "2026-10-18T01:02:03Z", TerminalStatus: "completed", Usage: telemetry.PublicUsageMap(), CacheDiagnostics: telemetry.PublicCacheDiagnosticsMap()}
			switch fault {
			case "wrong usage":
				input.Usage["totalTokens"] = 1
			case "unknown usage":
				input.Usage["unrecognized"] = true
			case "unknown diagnostic":
				input.CacheDiagnostics["unrecognized"] = true
			case "raw enum":
				input.CacheDiagnostics["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			case "raw sibling":
				input.CacheDiagnostics["lastError"] = "PRIVATE_PROVIDER_FAILURE"
			case "credential usage":
				input.Usage["apiKey"] = "sk-test-private-value"
			case "credential diagnostics":
				input.CacheDiagnostics["apiKey"] = "sk-test-private-value"
			case "reasoning usage":
				input.Usage["details"] = "<think>PRIVATE_REASONING</think>"
			case "reasoning diagnostics":
				input.CacheDiagnostics["details"] = "<think>PRIVATE_REASONING</think>"
			case "nested diagnostics":
				input.CacheDiagnostics = map[string]any{"nested": input.CacheDiagnostics}
			case "usage string budget":
				input.Usage["oversized"] = strings.Repeat("x", (1<<20)+1)
			case "diagnostic string budget":
				input.CacheDiagnostics["oversized"] = strings.Repeat("x", (1<<20)+1)
			case "diagnostic node budget":
				input.CacheDiagnostics["oversized"] = make([]any, 10_001)
			case "diagnostic depth budget":
				var nested any = "leaf"
				for depth := 0; depth < 65; depth++ {
					nested = map[string]any{"nested": nested}
				}
				input.CacheDiagnostics["oversized"] = nested
			}
			before, err := json.Marshal([]any{input.Usage, input.CacheDiagnostics})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NewTerminalPublicationIntent(input, "success"); err == nil {
				t.Fatal("unsafe or oversized telemetry acquired a publication exception")
			}
			after, err := json.Marshal([]any{input.Usage, input.CacheDiagnostics})
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected telemetry validation changed its original input maps")
			}
		})
	}
}
