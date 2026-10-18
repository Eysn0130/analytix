package evidence

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
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
