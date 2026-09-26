package model

import (
	"strings"
	"unicode/utf8"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const (
	FallbackContextWindowTokensV1 = 128_000
	contextSoftPercentV1          = 75
	contextHardPercentV1          = 85
)

type ContextThresholdsV1 struct {
	ContextWindowTokens int
	SoftThresholdTokens int
	HardThresholdTokens int
	UsedFallback        bool
}

func ResolveContextThresholdsV1(contextWindowTokens int) ContextThresholdsV1 {
	usedFallback := contextWindowTokens <= 0
	if usedFallback {
		contextWindowTokens = FallbackContextWindowTokensV1
	}
	return ContextThresholdsV1{
		ContextWindowTokens: contextWindowTokens,
		SoftThresholdTokens: contextWindowTokens * contextSoftPercentV1 / 100,
		HardThresholdTokens: contextWindowTokens * contextHardPercentV1 / 100,
		UsedFallback:        usedFallback,
	}
}

func EstimateThreadPreflightTokensV1(thread map[string]any, prompt string, contextWindowTokens int) int {
	messages := ProviderHistoryFromThread(thread)
	return EstimateMessagesPreflightTokensV1(messages, prompt, contextWindowTokens)
}

func EstimateMessagesPreflightTokensV1(messages []domainmodel.Message, prompt string, contextWindowTokens int) int {
	messages = CloneProviderMessages(messages)
	messages = append(messages, domainmodel.Message{Role: "user", Content: prompt})
	estimate := EstimateMessagesTokensV1(messages)
	window := ResolveContextThresholdsV1(contextWindowTokens).ContextWindowTokens
	reserve := window / 10
	if reserve < 256 {
		reserve = 256
	}
	if reserve > 8_192 {
		reserve = 8_192
	}
	return estimate + reserve
}

func EstimateProviderRequestTokensV1(request domainmodel.Request) int {
	tokens := EstimateTextTokensV1(request.SystemPrompt) + EstimateMessagesTokensV1(request.Messages)
	for _, schema := range request.Tools {
		tokens += 12 + EstimateTextTokensV1(schema.Name) + EstimateTextTokensV1(schema.Description)
		if len(schema.Parameters) > 0 {
			tokens += EstimateTextTokensV1(string(schema.Parameters))
		}
	}
	return tokens
}

func EstimateMessagesTokensV1(messages []domainmodel.Message) int {
	tokens := 0
	for _, message := range messages {
		tokens += 8 + EstimateTextTokensV1(message.Role) + EstimateTextTokensV1(message.Content)
		for _, part := range message.Parts {
			tokens += 4 + EstimateTextTokensV1(part.Type) + EstimateTextTokensV1(part.Text) + EstimateTextTokensV1(part.ImageURL) +
				EstimateTextTokensV1(part.MediaType) + EstimateTextTokensV1(part.Data) + EstimateTextTokensV1(part.Signature)
		}
		for _, call := range message.ToolCalls {
			tokens += 8 + EstimateTextTokensV1(call.ID) + EstimateTextTokensV1(call.Name) + EstimateTextTokensV1(string(call.Arguments))
		}
	}
	return tokens
}

func EstimateTextTokensV1(value string) int {
	var estimate TextTokenEstimatorV1
	estimate.WriteString(value)
	return estimate.Tokens()
}

// TextTokenEstimatorV1 applies the same conservative estimate to append-only
// fragments without retaining or rescanning the full output. An incomplete UTF-8
// suffix (at most three bytes) counts as invalid bytes until the next fragment
// completes it, exactly as EstimateTextTokensV1 does for each observed prefix.
type TextTokenEstimatorV1 struct {
	tokens      int
	asciiBytes  int
	pendingUTF8 string
}

func (estimate *TextTokenEstimatorV1) WriteString(value string) {
	if estimate.pendingUTF8 != "" {
		value = estimate.pendingUTF8 + value
		estimate.pendingUTF8 = ""
	}
	for len(value) > 0 {
		if value[0] <= 0x7f {
			estimate.asciiBytes++
			value = value[1:]
			continue
		}
		estimate.tokens += (estimate.asciiBytes + 3) / 4
		estimate.asciiBytes = 0
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 && !utf8.FullRuneInString(value) {
			// Clone the tiny suffix so it cannot retain a large source buffer.
			estimate.pendingUTF8 = strings.Clone(value)
			return
		}
		estimate.tokens++
		value = value[size:]
	}
}

func (estimate *TextTokenEstimatorV1) Tokens() int {
	return estimate.tokens + (estimate.asciiBytes+3)/4 + len(estimate.pendingUTF8)
}
