//go:build !analytix_prod

package provider

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type G3ProviderConformanceContract struct {
	ID                    string                         `json:"id"`
	Mode                  string                         `json:"mode"`
	SourceContractIDs     []string                       `json:"sourceContractIds"`
	ProductBoundary       contracts.ProductBoundary      `json:"productBoundary"`
	ProviderUsageMatrix   []G3ProviderUsageCaseSummary   `json:"providerUsageMatrix"`
	RequestShapeCaseIDs   []string                       `json:"requestShapeCaseIds"`
	RequestShapeMatrix    []G3ProviderRequestShapeCase   `json:"requestShapeMatrix"`
	Streaming             G3ProviderStreamingConformance `json:"streaming"`
	CacheDiagnostics      G3ProviderCacheDiagnostics     `json:"cacheDiagnostics"`
	CacheDriftAttribution ProviderDriftAttribution       `json:"cacheDriftAttribution"`
	CacheAccounting       G3ProviderCacheAccounting      `json:"cacheAccounting"`
	ExpectedOutput        map[string]any                 `json:"expectedOutput"`
}

type G3ProviderUsageCaseSummary struct {
	ID             string                 `json:"id"`
	EndpointFormat string                 `json:"endpointFormat"`
	BaseURL        string                 `json:"baseUrl"`
	Model          string                 `json:"model"`
	ResponseBody   map[string]any         `json:"responseBody"`
	ExpectedUsage  G3ProviderUsageSummary `json:"expectedUsage"`
}

type G3ProviderRequestShapeCase struct {
	ID                  string   `json:"id"`
	EndpointFormat      string   `json:"endpointFormat"`
	BaseURL             string   `json:"baseUrl"`
	Model               string   `json:"model"`
	ReasoningEffort     string   `json:"reasoningEffort,omitempty"`
	ExpectedURL         string   `json:"expectedUrl"`
	RequiredHeaders     []string `json:"requiredHeaders"`
	ForbiddenHeaders    []string `json:"forbiddenHeaders"`
	RequiredBodyFields  []string `json:"requiredBodyFields"`
	ForbiddenBodyFields []string `json:"forbiddenBodyFields"`
	ExpectedToolShape   string   `json:"expectedToolShape"`
}

type G3ProviderUsageSummary struct {
	PromptTokens     *int     `json:"promptTokens"`
	CompletionTokens *int     `json:"completionTokens"`
	ReasoningTokens  *int     `json:"reasoningTokens"`
	TotalTokens      *int     `json:"totalTokens"`
	CacheHitTokens   *int     `json:"cacheHitTokens"`
	CacheMissTokens  *int     `json:"cacheMissTokens"`
	CacheHitRate     *float64 `json:"cacheHitRate"`
	Absent           []string `json:"absent,omitempty"`
}

type G3ProviderCacheAccounting struct {
	RawPayloadParsedCaseIDs             []string `json:"rawPayloadParsedCaseIds"`
	RawTelemetrySupportedCaseIDs        []string `json:"rawTelemetrySupportedCaseIds"`
	RawMatchesExpectedUsageCaseIDs      []string `json:"rawMatchesExpectedUsageCaseIds"`
	TelemetrySupportedCaseIDs           []string `json:"telemetrySupportedCaseIds"`
	UnsupportedUnknownCaseIDs           []string `json:"unsupportedUnknownCaseIds"`
	DeepSeekCaseIDs                     []string `json:"deepseekCaseIds"`
	OpenAICacheCaseIDs                  []string `json:"openaiCacheCaseIds"`
	AnthropicCacheCaseIDs               []string `json:"anthropicCacheCaseIds"`
	TotalCacheHitTokens                 int      `json:"totalCacheHitTokens"`
	TotalCacheMissTokens                int      `json:"totalCacheMissTokens"`
	AggregateCacheHitRate               float64  `json:"aggregateCacheHitRate"`
	UnsupportedProvidersCountedAsMisses bool     `json:"unsupportedProvidersCountedAsMisses"`
}

type G3ProviderStreamingConformance struct {
	ThreadID             string   `json:"threadId"`
	SinceSeq             int      `json:"sinceSeq"`
	SSEFrames            []string `json:"sseFrames"`
	ExpectedKindsInOrder []string `json:"expectedKindsInOrder"`
	ExpectedUsageCaseID  string   `json:"expectedUsageCaseId"`
}

type G3ProviderCacheDiagnostics struct {
	PrefixHash                     string   `json:"prefixHash"`
	SystemHash                     string   `json:"systemHash"`
	PrefixItemsHash                string   `json:"prefixItemsHash"`
	ToolsHash                      string   `json:"toolsHash"`
	ToolSchemaTokens               int      `json:"toolSchemaTokens"`
	Provider                       string   `json:"provider"`
	ProviderID                     string   `json:"providerId"`
	EndpointFormat                 string   `json:"endpointFormat"`
	Model                          string   `json:"model"`
	SanitizedRequestURL            string   `json:"sanitizedRequestUrl"`
	ForbiddenDiagnosticsSubstrings []string `json:"forbiddenDiagnosticsSubstrings"`
}

type ProviderPrefixShape struct {
	PrefixHash      string `json:"prefixHash"`
	SystemHash      string `json:"systemHash"`
	PrefixItemsHash string `json:"prefixItemsHash"`
	ToolsHash       string `json:"toolsHash"`
	ProviderID      string `json:"providerId"`
	EndpointFormat  string `json:"endpointFormat"`
	Model           string `json:"model"`
}

type ProviderDriftAttribution struct {
	PreviousShape              ProviderPrefixShape    `json:"previousShape"`
	CurrentShape               ProviderPrefixShape    `json:"currentShape"`
	Usage                      G3ProviderUsageSummary `json:"usage"`
	ExpectedReasons            []string               `json:"expectedReasons"`
	ExpectedTelemetrySupported bool                   `json:"expectedTelemetrySupported"`
}

type G5ProviderDriftAttribution struct {
	PreviousPrefixHash    string   `json:"previousPrefixHash"`
	CurrentPrefixHash     string   `json:"currentPrefixHash"`
	SystemHashStable      bool     `json:"systemHashStable"`
	PrefixItemsHashStable bool     `json:"prefixItemsHashStable"`
	ToolsHashChanged      bool     `json:"toolsHashChanged"`
	ProviderChanged       bool     `json:"providerChanged"`
	ModelChanged          bool     `json:"modelChanged"`
	EndpointFormatChanged bool     `json:"endpointFormatChanged"`
	ExpectedReasons       []string `json:"expectedReasons"`
	TelemetrySupported    bool     `json:"telemetrySupported"`
	CacheHitRateKnown     bool     `json:"cacheHitRateKnown"`
}

func BuildG3ProviderConformanceOutput(contract G3ProviderConformanceContract) map[string]any {
	usageCaseIDs := make([]string, 0, len(contract.ProviderUsageMatrix))
	for _, item := range contract.ProviderUsageMatrix {
		usageCaseIDs = append(usageCaseIDs, item.ID)
	}
	return map[string]any{
		"stage":                   "G3",
		"mode":                    contract.Mode,
		"sourceContractIds":       contract.SourceContractIDs,
		"providerUsageCaseIds":    usageCaseIDs,
		"requestShapeCaseIds":     contract.RequestShapeCaseIDs,
		"requestShapeSummary":     BuildG3ProviderRequestShapeSummary(contract.RequestShapeMatrix),
		"streamingKinds":          contract.Streaming.ExpectedKindsInOrder,
		"cacheTelemetrySupported": true,
		"cacheDriftAttribution":   BuildProviderDriftAttribution(contract.CacheDriftAttribution),
		"cacheAccounting":         BuildG3ProviderCacheAccounting(contract.ProviderUsageMatrix),
		"productBoundary":         contract.ProductBoundary,
	}
}

func BuildG3ProviderCacheAccounting(cases []G3ProviderUsageCaseSummary) G3ProviderCacheAccounting {
	out := G3ProviderCacheAccounting{
		RawPayloadParsedCaseIDs:        make([]string, 0, len(cases)),
		RawTelemetrySupportedCaseIDs:   make([]string, 0, len(cases)),
		RawMatchesExpectedUsageCaseIDs: make([]string, 0, len(cases)),
		TelemetrySupportedCaseIDs:      make([]string, 0, len(cases)),
		UnsupportedUnknownCaseIDs:      make([]string, 0),
		DeepSeekCaseIDs:                make([]string, 0),
		OpenAICacheCaseIDs:             make([]string, 0),
		AnthropicCacheCaseIDs:          make([]string, 0),
	}
	for _, item := range cases {
		parsedUsage := ParsedProviderUsageFromRawPayload(item.EndpointFormat, item.BaseURL, item.ResponseBody)
		out.RawPayloadParsedCaseIDs = append(out.RawPayloadParsedCaseIDs, item.ID)
		if UsageSummaryMatchesExpected(parsedUsage, item.ExpectedUsage) {
			out.RawMatchesExpectedUsageCaseIDs = append(out.RawMatchesExpectedUsageCaseIDs, item.ID)
		}
		if parsedUsage.CacheHitTokens == nil || parsedUsage.CacheMissTokens == nil {
			if parsedUsage.CacheHitRate == nil {
				out.UnsupportedUnknownCaseIDs = append(out.UnsupportedUnknownCaseIDs, item.ID)
			}
			continue
		}
		out.RawTelemetrySupportedCaseIDs = append(out.RawTelemetrySupportedCaseIDs, item.ID)
		out.TelemetrySupportedCaseIDs = append(out.TelemetrySupportedCaseIDs, item.ID)
		out.TotalCacheHitTokens += *parsedUsage.CacheHitTokens
		out.TotalCacheMissTokens += *parsedUsage.CacheMissTokens
		if strings.Contains(item.BaseURL, "deepseek") {
			out.DeepSeekCaseIDs = append(out.DeepSeekCaseIDs, item.ID)
		}
		if item.EndpointFormat == "responses" {
			out.OpenAICacheCaseIDs = append(out.OpenAICacheCaseIDs, item.ID)
		}
		if item.EndpointFormat == "messages" {
			out.AnthropicCacheCaseIDs = append(out.AnthropicCacheCaseIDs, item.ID)
		}
	}
	denominator := out.TotalCacheHitTokens + out.TotalCacheMissTokens
	if denominator > 0 {
		out.AggregateCacheHitRate = float64(out.TotalCacheHitTokens) / float64(denominator)
	}
	return out
}

func BuildG3ProviderRequestShapeSummary(cases []G3ProviderRequestShapeCase) map[string]any {
	exactURLCount := 0
	derivedURLMatchCaseIDs := make([]string, 0)
	headerShapeMatchCaseIDs := make([]string, 0)
	bodyShapeMatchCaseIDs := make([]string, 0)
	toolShapeMatchCaseIDs := make([]string, 0)
	endpointFormats := make([]string, 0)
	seenEndpointFormats := map[string]bool{}
	fullEndpointCaseIDs := make([]string, 0)
	toolShapes := make([]string, 0)
	seenToolShapes := map[string]bool{}
	customFullEndpointExactURLCaseIDs := make([]string, 0)
	customFullEndpointAppendedPathCount := 0
	for _, item := range cases {
		if item.ExpectedURL != "" {
			exactURLCount += 1
		}
		if DerivedG3ProviderRequestURL(item) == item.ExpectedURL {
			derivedURLMatchCaseIDs = append(derivedURLMatchCaseIDs, item.ID)
		}
		if sameStringSlice(DerivedG3ProviderRequiredHeaders(item), item.RequiredHeaders) &&
			sameStringSlice(DerivedG3ProviderForbiddenHeaders(item), item.ForbiddenHeaders) {
			headerShapeMatchCaseIDs = append(headerShapeMatchCaseIDs, item.ID)
		}
		if sameStringSlice(DerivedG3ProviderRequiredBodyFields(item), item.RequiredBodyFields) &&
			sameStringSlice(DerivedG3ProviderForbiddenBodyFields(item), item.ForbiddenBodyFields) {
			bodyShapeMatchCaseIDs = append(bodyShapeMatchCaseIDs, item.ID)
		}
		if DerivedG3ProviderToolShape(item) == item.ExpectedToolShape {
			toolShapeMatchCaseIDs = append(toolShapeMatchCaseIDs, item.ID)
		}
		if !seenEndpointFormats[item.EndpointFormat] {
			seenEndpointFormats[item.EndpointFormat] = true
			endpointFormats = append(endpointFormats, item.EndpointFormat)
		}
		if item.EndpointFormat == "custom_endpoint" {
			fullEndpointCaseIDs = append(fullEndpointCaseIDs, item.ID)
			if DerivedG3ProviderRequestURL(item) == item.BaseURL && item.ExpectedURL == item.BaseURL {
				customFullEndpointExactURLCaseIDs = append(customFullEndpointExactURLCaseIDs, item.ID)
			}
			if item.ExpectedURL != item.BaseURL {
				customFullEndpointAppendedPathCount += 1
			}
		}
		if !seenToolShapes[item.ExpectedToolShape] {
			seenToolShapes[item.ExpectedToolShape] = true
			toolShapes = append(toolShapes, item.ExpectedToolShape)
		}
	}
	return map[string]any{
		"caseCount":                           len(cases),
		"exactUrlCount":                       exactURLCount,
		"derivedUrlMatchCaseIds":              derivedURLMatchCaseIDs,
		"headerShapeMatchCaseIds":             headerShapeMatchCaseIDs,
		"bodyShapeMatchCaseIds":               bodyShapeMatchCaseIDs,
		"toolShapeMatchCaseIds":               toolShapeMatchCaseIDs,
		"matrix":                              CloneG3ProviderRequestShapeCases(cases),
		"endpointFormats":                     endpointFormats,
		"fullEndpointCaseIds":                 fullEndpointCaseIDs,
		"toolShapes":                          toolShapes,
		"customFullEndpointExactUrlCaseIds":   customFullEndpointExactURLCaseIDs,
		"customFullEndpointAppendedPathCount": customFullEndpointAppendedPathCount,
		"requiredBodyFieldFamilies": map[string]any{
			"messagesFieldCaseCount":   countG3RequiredBodyField(cases, "messages"),
			"inputFieldCaseCount":      countG3RequiredBodyField(cases, "input"),
			"systemFieldCaseCount":     countG3RequiredBodyField(cases, "system"),
			"thinkingFieldCaseCount":   countG3RequiredBodyField(cases, "thinking"),
			"maxOutputTokensCaseCount": countG3RequiredBodyField(cases, "max_output_tokens"),
			"maxTokensCaseCount":       countG3RequiredBodyField(cases, "max_tokens"),
		},
		"forbiddenBodyFieldFamilies": map[string]any{
			"thinkingForbiddenCaseCount":        countG3ForbiddenBodyField(cases, "thinking"),
			"systemForbiddenCaseCount":          countG3ForbiddenBodyField(cases, "system"),
			"inputForbiddenCaseCount":           countG3ForbiddenBodyField(cases, "input"),
			"maxOutputTokensForbiddenCaseCount": countG3ForbiddenBodyField(cases, "max_output_tokens"),
		},
	}
}

func CloneG3ProviderRequestShapeCases(cases []G3ProviderRequestShapeCase) []G3ProviderRequestShapeCase {
	out := make([]G3ProviderRequestShapeCase, 0, len(cases))
	for _, item := range cases {
		out = append(out, G3ProviderRequestShapeCase{
			ID:                  item.ID,
			EndpointFormat:      item.EndpointFormat,
			BaseURL:             item.BaseURL,
			Model:               item.Model,
			ReasoningEffort:     item.ReasoningEffort,
			ExpectedURL:         item.ExpectedURL,
			RequiredHeaders:     append([]string{}, item.RequiredHeaders...),
			ForbiddenHeaders:    append([]string{}, item.ForbiddenHeaders...),
			RequiredBodyFields:  append([]string{}, item.RequiredBodyFields...),
			ForbiddenBodyFields: append([]string{}, item.ForbiddenBodyFields...),
			ExpectedToolShape:   item.ExpectedToolShape,
		})
	}
	return out
}

func DerivedG3ProviderRequestURL(item G3ProviderRequestShapeCase) string {
	switch item.EndpointFormat {
	case "custom_endpoint":
		return item.BaseURL
	case "responses":
		return AppendEndpointPath(item.BaseURL, "/v1/responses")
	case "messages":
		return AppendEndpointPath(item.BaseURL, "/v1/messages")
	default:
		return AppendEndpointPath(item.BaseURL, "/v1/chat/completions")
	}
}

func DerivedG3ProviderToolShape(item G3ProviderRequestShapeCase) string {
	if item.EndpointFormat == "responses" || strings.HasSuffix(item.BaseURL, "/responses") {
		return "responses-function"
	}
	if item.EndpointFormat == "messages" || strings.HasSuffix(item.BaseURL, "/messages") {
		return "anthropic-input-schema"
	}
	return "openai-function"
}

func DerivedG3ProviderRequiredHeaders(item G3ProviderRequestShapeCase) []string {
	if DerivedG3ProviderToolShape(item) == "anthropic-input-schema" {
		return []string{"Authorization", "x-api-key", "anthropic-version"}
	}
	return []string{"Authorization"}
}

func DerivedG3ProviderForbiddenHeaders(item G3ProviderRequestShapeCase) []string {
	if DerivedG3ProviderToolShape(item) == "anthropic-input-schema" {
		return []string{}
	}
	return []string{"x-api-key", "anthropic-version"}
}

func DerivedG3ProviderRequiredBodyFields(item G3ProviderRequestShapeCase) []string {
	switch DerivedG3ProviderToolShape(item) {
	case "responses-function":
		return []string{"model", "stream", "input", "tools", "max_output_tokens"}
	case "anthropic-input-schema":
		return []string{"model", "stream", "system", "messages", "tools", "max_tokens"}
	default:
		fields := []string{"model", "stream", "messages", "tools"}
		if strings.Contains(item.BaseURL, "deepseek") {
			fields = append(fields, "thinking")
		}
		return append(fields, "reasoning_effort")
	}
}

func DerivedG3ProviderForbiddenBodyFields(item G3ProviderRequestShapeCase) []string {
	switch DerivedG3ProviderToolShape(item) {
	case "responses-function":
		return []string{"messages", "system", "thinking"}
	case "anthropic-input-schema":
		return []string{"input", "max_output_tokens", "thinking"}
	default:
		fields := []string{"input", "system", "max_output_tokens"}
		if !strings.Contains(item.BaseURL, "deepseek") {
			fields = append(fields, "thinking")
		}
		return fields
	}
}

func BuildProviderDriftAttribution(drift ProviderDriftAttribution) G5ProviderDriftAttribution {
	return G5ProviderDriftAttribution{
		PreviousPrefixHash:    drift.PreviousShape.PrefixHash,
		CurrentPrefixHash:     drift.CurrentShape.PrefixHash,
		SystemHashStable:      drift.PreviousShape.SystemHash == drift.CurrentShape.SystemHash,
		PrefixItemsHashStable: drift.PreviousShape.PrefixItemsHash == drift.CurrentShape.PrefixItemsHash,
		ToolsHashChanged:      drift.PreviousShape.ToolsHash != drift.CurrentShape.ToolsHash,
		ProviderChanged:       drift.PreviousShape.ProviderID != drift.CurrentShape.ProviderID,
		ModelChanged:          drift.PreviousShape.Model != drift.CurrentShape.Model,
		EndpointFormatChanged: drift.PreviousShape.EndpointFormat != drift.CurrentShape.EndpointFormat,
		ExpectedReasons:       append([]string(nil), drift.ExpectedReasons...),
		TelemetrySupported:    drift.ExpectedTelemetrySupported,
		CacheHitRateKnown:     drift.Usage.CacheHitRate != nil,
	}
}

func ParsedProviderUsageFromRawPayload(endpointFormat string, baseURL string, responseBody map[string]any) G3ProviderUsageSummary {
	usage := nestedMap(responseBody, "usage")
	if endpointFormat == "responses" {
		inputTokens := intFromUsagePayload(usage, "input_tokens")
		cachedTokens := intFromUsagePayload(nestedMap(usage, "input_tokens_details"), "cached_tokens")
		cacheMissTokens := inputTokens - cachedTokens
		return G3ProviderUsageSummary{
			PromptTokens:     intPointer(inputTokens),
			CompletionTokens: intPointer(intFromUsagePayload(usage, "output_tokens")),
			ReasoningTokens:  optionalIntPointer(intFromUsagePayload(nestedMap(usage, "output_tokens_details"), "reasoning_tokens")),
			TotalTokens:      intPointer(intFromUsagePayload(usage, "total_tokens")),
			CacheHitTokens:   intPointer(cachedTokens),
			CacheMissTokens:  intPointer(cacheMissTokens),
			CacheHitRate:     floatPointer(float64(cachedTokens) / float64(inputTokens)),
		}
	}
	if endpointFormat == "messages" {
		inputTokens := intFromUsagePayload(usage, "input_tokens")
		outputTokens := intFromUsagePayload(usage, "output_tokens")
		cacheReadTokens := intFromUsagePayload(usage, "cache_read_input_tokens")
		cacheCreationTokens := intFromUsagePayload(usage, "cache_creation_input_tokens")
		promptTokens := inputTokens + cacheReadTokens + cacheCreationTokens
		cacheMissTokens := inputTokens + cacheCreationTokens
		return G3ProviderUsageSummary{
			PromptTokens:     intPointer(promptTokens),
			CompletionTokens: intPointer(outputTokens),
			TotalTokens:      intPointer(promptTokens + outputTokens),
			CacheHitTokens:   intPointer(cacheReadTokens),
			CacheMissTokens:  intPointer(cacheMissTokens),
			CacheHitRate:     floatPointer(float64(cacheReadTokens) / float64(cacheReadTokens+cacheMissTokens)),
		}
	}
	promptTokens := intFromUsagePayload(usage, "prompt_tokens")
	promptDetails := nestedMap(usage, "prompt_tokens_details")
	completionDetails := nestedMap(usage, "completion_tokens_details")
	nativeHitTokens := intFromUsagePayload(usage, "prompt_cache_hit_tokens")
	nativeMissTokens := intFromUsagePayload(usage, "prompt_cache_miss_tokens")
	cachedTokens := intFromUsagePayload(promptDetails, "cached_tokens")
	parsed := G3ProviderUsageSummary{
		PromptTokens:     intPointer(promptTokens),
		CompletionTokens: intPointer(intFromUsagePayload(usage, "completion_tokens")),
		ReasoningTokens:  optionalIntPointer(intFromUsagePayload(completionDetails, "reasoning_tokens")),
		TotalTokens:      intPointer(intFromUsagePayload(usage, "total_tokens")),
	}
	if nativeHitTokens > 0 || nativeMissTokens > 0 {
		parsed.CacheHitTokens = intPointer(nativeHitTokens)
		parsed.CacheMissTokens = intPointer(nativeMissTokens)
		parsed.CacheHitRate = floatPointer(float64(nativeHitTokens) / float64(nativeHitTokens+nativeMissTokens))
	} else if cachedTokens > 0 {
		parsed.CacheHitTokens = intPointer(cachedTokens)
		parsed.CacheMissTokens = intPointer(promptTokens - cachedTokens)
		parsed.CacheHitRate = floatPointer(float64(cachedTokens) / float64(promptTokens))
	}
	return parsed
}

func UsageSummaryMatchesExpected(parsed G3ProviderUsageSummary, expected G3ProviderUsageSummary) bool {
	return intPointerEqual(parsed.PromptTokens, expected.PromptTokens) &&
		intPointerEqual(parsed.CompletionTokens, expected.CompletionTokens) &&
		intPointerEqual(parsed.ReasoningTokens, expected.ReasoningTokens) &&
		intPointerEqual(parsed.TotalTokens, expected.TotalTokens) &&
		intPointerEqual(parsed.CacheHitTokens, expected.CacheHitTokens) &&
		intPointerEqual(parsed.CacheMissTokens, expected.CacheMissTokens) &&
		floatPointerEqual(parsed.CacheHitRate, expected.CacheHitRate)
}

func countG3RequiredBodyField(cases []G3ProviderRequestShapeCase, field string) int {
	count := 0
	for _, item := range cases {
		if stringSliceContains(item.RequiredBodyFields, field) {
			count += 1
		}
	}
	return count
}

func countG3ForbiddenBodyField(cases []G3ProviderRequestShapeCase, field string) int {
	count := 0
	for _, item := range cases {
		if stringSliceContains(item.ForbiddenBodyFields, field) {
			count += 1
		}
	}
	return count
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func nestedMap(input map[string]any, field string) map[string]any {
	value, _ := input[field].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func intFromUsagePayload(usage map[string]any, field string) int {
	switch value := usage[field].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func intPointer(value int) *int {
	return &value
}

func optionalIntPointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}

func intPointerEqual(left *int, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func floatPointerEqual(left *float64, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
