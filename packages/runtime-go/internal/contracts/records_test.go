package contracts

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNumericSeqRejectsNegativeZero(t *testing.T) {
	for _, value := range []any{math.Copysign(0, -1), json.Number("-0")} {
		if _, ok := NumericSeq(value); ok {
			t.Fatalf("negative-zero sequence was accepted: %#v", value)
		}
	}
	if value, ok := NumericSeq(float64(0)); !ok || value != 0 {
		t.Fatalf("canonical zero was rejected: value=%d ok=%v", value, ok)
	}
}

func TestThreadSummaryPreviewTruncatesOnRuneBoundary(t *testing.T) {
	text := strings.Repeat("证", 161)
	summary := ThreadSummary(map[string]any{
		"id": "thread-preview",
		"turns": []any{map[string]any{
			"items": []any{map[string]any{"kind": "assistant_text", "text": text}},
		}},
	})
	preview, _ := summary["preview"].(string)
	if !utf8.ValidString(preview) || utf8.RuneCountInString(preview) != 160 {
		t.Fatalf("preview is not a 160-rune UTF-8 projection: %q", preview)
	}
}

func TestThreadSummaryPreservesRequiredModelWithoutChangingSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present bool
		model   any
	}{
		{name: "missing"}, {name: "empty", present: true, model: ""},
		{name: "configured", present: true, model: "synthetic/model"},
		{name: "invalid null", present: true, model: nil}, {name: "invalid type", present: true, model: float64(7)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := map[string]any{"id": "thread-local", "turns": []any{}, "privateField": "must-not-cross"}
			if tc.present {
				source["model"] = tc.model
			}
			before := CloneMap(source)
			summary := ThreadSummary(source)
			model, present := summary["model"]
			expected := tc.model
			if !tc.present {
				expected = ""
			}
			if !present || !reflect.DeepEqual(model, expected) {
				t.Fatalf("model=%#v present=%v, want %#v", model, present, expected)
			}
			if _, present := summary["providerId"]; present {
				t.Fatal("summary invented a provider")
			}
			if _, present := summary["privateField"]; present {
				t.Fatal("unknown field crossed the summary projection")
			}
			if !reflect.DeepEqual(source, before) {
				t.Fatal("summary projection mutated the source thread")
			}
		})
	}
}
