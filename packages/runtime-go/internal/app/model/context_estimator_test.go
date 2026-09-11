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
