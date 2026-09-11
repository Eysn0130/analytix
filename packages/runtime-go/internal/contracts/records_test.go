package contracts

import (
	"encoding/json"
	"math"
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
