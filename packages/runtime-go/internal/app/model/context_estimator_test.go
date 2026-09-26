package model

import (
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestContextEstimatorUsesCJKAndConservativeFallback(t *testing.T) {
	if got := EstimateTextTokensV1("abcdefgh"); got != 2 {
		t.Fatalf("ascii estimate = %d", got)
	}
	if got := EstimateTextTokensV1("案件证据"); got != 4 {
		t.Fatalf("CJK estimate = %d", got)
	}
	thresholds := ResolveContextThresholdsV1(0)
	if !thresholds.UsedFallback || thresholds.ContextWindowTokens != FallbackContextWindowTokensV1 ||
		thresholds.SoftThresholdTokens != 96_000 || thresholds.HardThresholdTokens != 108_800 {
		t.Fatalf("fallback thresholds = %#v", thresholds)
	}
}

func TestProviderRequestEstimateIncludesToolsAndPrivateParts(t *testing.T) {
	base := domainmodel.Request{SystemPrompt: "system", Messages: []domainmodel.Message{{Role: "user", Content: "hello"}}}
	withPayload := base
	withPayload.Messages = []domainmodel.Message{{Role: "user", Content: "hello", Parts: []domainmodel.MessagePart{{Type: "image", Data: strings.Repeat("x", 400)}}}}
	withPayload.Tools = []domainmodel.ToolSchema{{Name: "read", Description: strings.Repeat("y", 80), Parameters: []byte(`{"type":"object"}`)}}
	if EstimateProviderRequestTokensV1(withPayload) <= EstimateProviderRequestTokensV1(base)+100 {
		t.Fatalf("request estimate omitted bounded request material: base=%d payload=%d", EstimateProviderRequestTokensV1(base), EstimateProviderRequestTokensV1(withPayload))
	}
}

func TestIncrementalTextEstimateMatchesEveryLegacyPrefix(t *testing.T) {
	// Preserve the prior whole-string algorithm independently of the incremental
	// implementation, including its conservative treatment of incomplete UTF-8.
	legacy := func(text string) int {
		tokens, ascii := 0, 0
		for _, r := range text {
			if r <= 0x7f {
				ascii++
			} else {
				tokens += (ascii+3)/4 + 1
				ascii = 0
			}
		}
		return tokens + (ascii+3)/4
	}
	for _, text := range []string{
		"", "abcdefghijk", "a案件证据bc🙂def\uFFFDg",
		"a\xff\xfe\x80bc\xe4\xb8", "\xf0\x9f\x99\x82\xc0\xaf\xed\xa0\x80z", "a\xe4\xb8x",
	} {
		for size := 1; size <= len(text)+1; size++ {
			var estimate TextTokenEstimatorV1
			for start := 0; start < len(text); start += size {
				end := min(start+size, len(text))
				estimate.WriteString(text[start:end])
				estimate.WriteString("")
				if got, want := estimate.Tokens(), legacy(text[:end]); got != want {
					t.Fatalf("prefix estimate changed: text=%q size=%d end=%d got=%d want=%d", text, size, end, got, want)
				}
			}
			if estimate.Tokens() != legacy(text) || EstimateTextTokensV1(text) != legacy(text) {
				t.Fatalf("final estimate changed: text=%q size=%d", text, size)
			}
		}
	}
}
