package evidence

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"
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
