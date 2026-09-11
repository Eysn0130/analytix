package terminaltelemetry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const (
	TerminalCacheDiagnosticsSchemaV1 = "terminal-cache-diagnostics.v1"
	PrefixBaselineSchemaV1           = "cache-prefix-baseline.v1"
	ProviderAttemptTelemetrySchemaV1 = "provider-attempt-telemetry.v1"
	terminalCacheDiagnosticsRejected = "rejected"
	maxTerminalDiagnosticKeys        = 64
	maxTerminalDiagnosticList        = 512
	maxTerminalDiagnosticString      = 1024
)

type terminalCacheStateV1 uint8

const (
	terminalCacheEmptyV1 terminalCacheStateV1 = iota
	terminalCacheAcceptedV1
	terminalCacheRejectedV1
)

type terminalHashFieldV1 uint8

const (
	terminalPrefixHashV1 terminalHashFieldV1 = iota
	terminalSystemHashV1
	terminalPrefixItemsHashV1
	terminalToolsHashV1
	terminalToolSourcesHashV1
	terminalCacheContinuityDigestV1
	terminalCacheProviderNamespaceDigestV1
	terminalContextEpochDigestV1
	terminalContextEpochRegistryDigestV1
	terminalHashFieldCountV1
)

var terminalHashFieldNamesV1 = [terminalHashFieldCountV1]string{
	"prefixHash", "systemHash", "prefixItemsHash", "toolsHash", "toolSourcesHash",
	"cacheContinuityDigest", "cacheProviderNamespaceDigest", "contextEpochDigest", "contextEpochRegistryDigest",
}

type terminalBoolFieldV1 uint8

const (
	terminalPrefixChangedV1 terminalBoolFieldV1 = iota
	terminalToolSourceChangedV1
	terminalCacheTelemetrySupportedV1
	terminalCacheTelemetryPresentV1
	terminalProviderNativeCacheTelemetryV1
	terminalCacheBaselineObservedV1
	terminalProviderAttemptTelemetryValidV1
	terminalContextEpochStateValidV1
	terminalBoolFieldCountV1
)

var terminalBoolFieldNamesV1 = [terminalBoolFieldCountV1]string{
	"prefixChanged", "toolSourceChanged", "cacheTelemetrySupported", "cacheTelemetryPresent",
	"providerNativeCacheTelemetry", "cacheBaselineObserved", "providerAttemptTelemetryValid", "contextEpochStateValid",
}

type terminalNumberFieldV1 uint8

const (
	terminalToolSchemaTokensV1 terminalNumberFieldV1 = iota
	terminalToolCountV1
	terminalFirstTokenLatencyMsV1
	terminalDurationMsV1
	terminalCacheHitTokensV1
	terminalCacheMissTokensV1
	terminalProviderLogicalCallCountV1
	terminalProviderAttemptCountV1
	terminalContextEpochV1
	terminalNumberFieldCountV1
)

var terminalNumberFieldNamesV1 = [terminalNumberFieldCountV1]string{
	"toolSchemaTokens", "toolCount", "firstTokenLatencyMs", "durationMs", "cacheHitTokens", "cacheMissTokens",
	"providerLogicalCallCount", "providerAttemptCount", "contextEpoch",
}

type terminalUsageV1 struct {
	promptTokens, completionTokens, reasoningTokens, totalTokens int
	cacheHitTokens, cacheMissTokens                              int
	hasCacheHit, hasCacheMiss                                    bool
	costUSD, costCNY, cacheSavingsUSD, cacheSavingsCNY           float64
	priceConfigured                                              bool
}

type terminalTokenAggregateV1 struct {
	present               bool
	complete              bool
	knownObservationCount uint64
	observationCount      uint64
	value                 uint64
	hasValue              bool
}

type terminalAttemptStatusesV1 struct {
	present       bool
	succeeded     uint64
	failed        uint64
	cancelled     uint64
	timedOut      uint64
	streamAborted uint64
}

type terminalAttemptRateV1 struct {
	present     bool
	known       bool
	numerator   uint64
	denominator uint64
}

type terminalContextImpactV1 struct {
	present         bool
	stablePrefix    bool
	dynamicContext  bool
	turnTail        bool
	diagnosticsOnly bool
}

type terminalPromptRouteV1 uint8

const (
	terminalPromptRouteUnknownV1 terminalPromptRouteV1 = iota
	terminalPromptRouteDirectAnswerV1
	terminalPromptRouteLightAgentV1
	terminalPromptRouteToolAgentV1
	terminalPromptRouteSubagentAgentV1
)

type terminalCacheDiagnosticsV1 struct {
	state terminalCacheStateV1
	route terminalPromptRouteV1

	hashes      [terminalHashFieldCountV1]string
	hashPresent [terminalHashFieldCountV1]bool
	bools       [terminalBoolFieldCountV1]bool
	boolPresent [terminalBoolFieldCountV1]bool
	numbers     [terminalNumberFieldCountV1]uint64
	numPresent  [terminalNumberFieldCountV1]bool

	cacheRate terminalAttemptRateV1

	hasCacheTelemetrySource   bool
	hasCacheBaselineSchema    bool
	hasProviderAttemptSchema  bool
	hasProviderAttemptError   bool
	prefixChangeReasons       []string
	toolSourceChangeReasons   []string
	contextEpochChangeReasons []string
	attemptStatuses           terminalAttemptStatusesV1
	attemptInputTokens        terminalTokenAggregateV1
	attemptOutputTokens       terminalTokenAggregateV1
	attemptCacheHitTokens     terminalTokenAggregateV1
	attemptCacheMissTokens    terminalTokenAggregateV1
	attemptRate               terminalAttemptRateV1
	contextImpact             terminalContextImpactV1
}

// TerminalTelemetryV1 is the closed, host-owned value that may cross the case
// publication boundary. It contains no map, interface, provider Result,
// chunks, tool calls, assistant text, raw identifiers, or reasoning content.
// The zero value represents deterministic host-only terminal telemetry.
type TerminalTelemetryV1 struct {
	usage       terminalUsageV1
	diagnostics terminalCacheDiagnosticsV1
}

// ProviderUsageMap returns the one closed usage snapshot shape admitted at
// the ordinary-terminal publication boundary.
func ProviderUsageMap(usage domainmodel.Usage) map[string]any {
	usage = domainmodel.NormalizeUsage(usage)
	// A terminal snapshot is a closed accounting record. Provider-reported
	// totals are advisory and cannot contradict the exact public components.
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	hasCacheTelemetry := usage.HasCacheTelemetry()
	cacheTotal := 0
	if hasCacheTelemetry {
		cacheTotal = usage.CacheHitTokens + usage.CacheMissTokens
	}
	cacheHitRate := any(nil)
	if cacheTotal > 0 {
		cacheHitRate = float64(usage.CacheHitTokens) / float64(cacheTotal)
	}
	cacheableTokenHitRate := any(nil)
	if cacheTotal > 0 {
		cacheableTokenHitRate = float64(usage.CacheHitTokens) / float64(cacheTotal)
	}
	totalInputTokenHitRate := any(nil)
	if usage.PromptTokens > 0 && cacheTotal > 0 {
		totalInputTokenHitRate = float64(usage.CacheHitTokens) / float64(usage.PromptTokens)
	}
	out := map[string]any{
		"promptTokens":              usage.PromptTokens,
		"completionTokens":          usage.CompletionTokens,
		"reasoningTokens":           usage.ReasoningTokens,
		"totalTokens":               usage.TotalTokens,
		"cacheHitRate":              cacheHitRate,
		"cacheableTokenHitRate":     cacheableTokenHitRate,
		"totalInputTokenHitRate":    totalInputTokenHitRate,
		"cacheMissReasons":          []any{},
		"cacheSuggestions":          []any{},
		"costUsd":                   usage.CostUSD,
		"costCny":                   usage.CostCNY,
		"priceConfigured":           usage.PriceConfigured,
		"cacheSavingsUsd":           usage.CacheSavingsUSD,
		"cacheSavingsCny":           usage.CacheSavingsCNY,
		"tokenEconomySavingsTokens": 0,
		"turns":                     1,
	}
	if hasCacheTelemetry {
		out["cachedTokens"] = usage.CacheHitTokens
		out["cacheHitTokens"] = usage.CacheHitTokens
		out["cacheMissTokens"] = usage.CacheMissTokens
	}
	return out
}

func NewTerminalTelemetryV1(usage domainmodel.Usage, diagnostics map[string]any) TerminalTelemetryV1 {
	normalized := domainmodel.NormalizeUsage(usage)
	projection := TerminalTelemetryV1{usage: terminalUsageV1{
		promptTokens: normalized.PromptTokens, completionTokens: normalized.CompletionTokens,
		reasoningTokens: normalized.ReasoningTokens, totalTokens: normalized.TotalTokens,
		cacheHitTokens: normalized.CacheHitTokens, cacheMissTokens: normalized.CacheMissTokens,
		hasCacheHit: normalized.HasCacheHit, hasCacheMiss: normalized.HasCacheMiss,
		costUSD: normalized.CostUSD, costCNY: normalized.CostCNY,
		cacheSavingsUSD: normalized.CacheSavingsUSD, cacheSavingsCNY: normalized.CacheSavingsCNY,
		priceConfigured: normalized.PriceConfigured,
	}}
	if len(diagnostics) == 0 {
		return projection
	}
	validated, ok := parseTerminalCacheDiagnosticsV1(diagnostics)
	if !ok || !terminalTelemetryUsageConsistentV1(projection.usage, validated) {
		projection.diagnostics.state = terminalCacheRejectedV1
		return projection
	}
	projection.diagnostics = validated
	return projection
}

func (projection TerminalTelemetryV1) PublicUsageMap() map[string]any {
	usage := projection.usage
	return ProviderUsageMap(domainmodel.Usage{
		PromptTokens: usage.promptTokens, CompletionTokens: usage.completionTokens,
		ReasoningTokens: usage.reasoningTokens, TotalTokens: usage.totalTokens,
		CacheHitTokens: usage.cacheHitTokens, CacheMissTokens: usage.cacheMissTokens,
		HasCacheHit: usage.hasCacheHit, HasCacheMiss: usage.hasCacheMiss,
		CostUSD: usage.costUSD, CostCNY: usage.costCNY,
		CacheSavingsUSD: usage.cacheSavingsUSD, CacheSavingsCNY: usage.cacheSavingsCNY,
		PriceConfigured: usage.priceConfigured,
	})
}

func (projection TerminalTelemetryV1) PublicCacheDiagnosticsMap() map[string]any {
	diagnostics := projection.diagnostics
	switch diagnostics.state {
	case terminalCacheEmptyV1:
		return map[string]any{}
	case terminalCacheRejectedV1:
		return map[string]any{
			"terminalCacheDiagnosticsSchema":      TerminalCacheDiagnosticsSchemaV1,
			"terminalCacheDiagnosticsValid":       false,
			"terminalCacheDiagnosticsDisposition": terminalCacheDiagnosticsRejected,
		}
	case terminalCacheAcceptedV1:
		return diagnostics.publicMap()
	default:
		return map[string]any{
			"terminalCacheDiagnosticsSchema":      TerminalCacheDiagnosticsSchemaV1,
			"terminalCacheDiagnosticsValid":       false,
			"terminalCacheDiagnosticsDisposition": terminalCacheDiagnosticsRejected,
		}
	}
}

// ValidateTerminalTelemetryPublicMapsV1 accepts only the exact public maps
// that TerminalTelemetryV1 itself can produce. It is the replay/publication
// guard for persisted ordinary terminal telemetry; permissive legacy maps do
// not become current host diagnostics merely because they are valid JSON.
func ValidateTerminalTelemetryPublicMapsV1(usageMap, diagnosticsMap map[string]any) bool {
	usage, ok := terminalPublicUsageFromMapV1(usageMap)
	if !ok || diagnosticsMap == nil {
		return false
	}
	projection := NewTerminalTelemetryV1(usage, diagnosticsMap)
	return terminalCanonicalMapEqualV1(usageMap, projection.PublicUsageMap()) &&
		terminalCanonicalMapEqualV1(diagnosticsMap, projection.PublicCacheDiagnosticsMap())
}

func terminalPublicUsageFromMapV1(value map[string]any) (domainmodel.Usage, bool) {
	if value == nil {
		return domainmodel.Usage{}, false
	}
	readInt := func(key string) (int, bool) {
		raw, present := value[key]
		if !present {
			return 0, false
		}
		number, ok := terminalUnsignedNumber(raw)
		if !ok || number > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(number), true
	}
	prompt, promptOK := readInt("promptTokens")
	completion, completionOK := readInt("completionTokens")
	reasoning, reasoningOK := readInt("reasoningTokens")
	total, totalOK := readInt("totalTokens")
	turns, turnsOK := readInt("turns")
	tokenEconomy, tokenEconomyOK := readInt("tokenEconomySavingsTokens")
	if !promptOK || !completionOK || !reasoningOK || !totalOK || !turnsOK || !tokenEconomyOK ||
		turns != 1 || tokenEconomy != 0 || reasoning > completion || total < prompt+completion ||
		!terminalEmptyPublicListV1(value["cacheMissReasons"]) || !terminalEmptyPublicListV1(value["cacheSuggestions"]) {
		return domainmodel.Usage{}, false
	}
	readAmount := func(key string) (float64, bool) {
		raw, present := value[key]
		if !present {
			return 0, false
		}
		var amount float64
		switch typed := raw.(type) {
		case int:
			amount = float64(typed)
		case int64:
			amount = float64(typed)
		case uint64:
			amount = float64(typed)
		case float32:
			amount = float64(typed)
		case float64:
			amount = typed
		case json.Number:
			parsed, err := typed.Float64()
			if err != nil {
				return 0, false
			}
			amount = parsed
		default:
			return 0, false
		}
		return amount, amount >= 0 && amount <= 1e12 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
	}
	costUSD, costUSDOK := readAmount("costUsd")
	costCNY, costCNYOK := readAmount("costCny")
	savingsUSD, savingsUSDOK := readAmount("cacheSavingsUsd")
	savingsCNY, savingsCNYOK := readAmount("cacheSavingsCny")
	priceConfigured, priceOK := value["priceConfigured"].(bool)
	if !costUSDOK || !costCNYOK || !savingsUSDOK || !savingsCNYOK || !priceOK {
		return domainmodel.Usage{}, false
	}
	_, cachedPresent := value["cachedTokens"]
	_, hitPresent := value["cacheHitTokens"]
	_, missPresent := value["cacheMissTokens"]
	if cachedPresent != hitPresent || hitPresent != missPresent {
		return domainmodel.Usage{}, false
	}
	cacheHit, cacheMiss := 0, 0
	if hitPresent {
		var hitOK, missOK, cachedOK bool
		cacheHit, hitOK = readInt("cacheHitTokens")
		cacheMiss, missOK = readInt("cacheMissTokens")
		cached, cachedOK := readInt("cachedTokens")
		if !hitOK || !missOK || !cachedOK || cached != cacheHit || cacheHit+cacheMiss > prompt {
			return domainmodel.Usage{}, false
		}
	}
	return domainmodel.Usage{
		PromptTokens: prompt, CompletionTokens: completion, ReasoningTokens: reasoning, TotalTokens: total,
		CacheHitTokens: cacheHit, CacheMissTokens: cacheMiss, HasCacheHit: hitPresent, HasCacheMiss: missPresent,
		CostUSD: costUSD, CostCNY: costCNY, CacheSavingsUSD: savingsUSD, CacheSavingsCNY: savingsCNY,
		PriceConfigured: priceConfigured,
	}, true
}

func terminalEmptyPublicListV1(value any) bool {
	switch typed := value.(type) {
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	default:
		return false
	}
}

func terminalCanonicalMapEqualV1(left, right map[string]any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func parseTerminalCacheDiagnosticsV1(input map[string]any) (terminalCacheDiagnosticsV1, bool) {
	if len(input) == 0 || len(input) > maxTerminalDiagnosticKeys {
		return terminalCacheDiagnosticsV1{}, len(input) == 0
	}
	out := terminalCacheDiagnosticsV1{state: terminalCacheAcceptedV1}
	var observedCacheRate float64
	observedCacheRatePresent := false
	for key, value := range input {
		switch key {
		case "prefixHash", "systemHash", "prefixItemsHash", "toolsHash", "toolSourcesHash",
			"cacheContinuityDigest", "cacheProviderNamespaceDigest", "contextEpochDigest", "contextEpochRegistryDigest":
			field, ok := terminalHashFieldForNameV1(key)
			if !ok || !out.setHash(field, value) {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "provider", "providerId", "endpointFormat", "model":
			// Raw provider identity is intentionally not retained in a case final.
			// It remains available in private provider-attempt authority records.
			if _, ok := terminalBoundedString(value); !ok {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "route":
			route, ok := terminalPromptRouteFromAnyV1(value)
			if !ok || out.route != terminalPromptRouteUnknownV1 {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.route = route
		case "toolSourceIds":
			// Tool source ids are execution/catalog data, not terminal evidence.
			if _, ok := terminalStringList(value, terminalGenericStringAllowed); !ok {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "cacheTelemetrySource":
			if value != "provider_usage" {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.hasCacheTelemetrySource = true
		case "cacheBaselineSchema":
			if value != PrefixBaselineSchemaV1 {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.hasCacheBaselineSchema = true
		case "providerAttemptTelemetrySchema":
			if value != ProviderAttemptTelemetrySchemaV1 {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.hasProviderAttemptSchema = true
		case "providerAttemptTelemetryError":
			if value != "invalid_or_incomplete" {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.hasProviderAttemptError = true
		case "prefixChanged", "toolSourceChanged", "cacheTelemetrySupported", "cacheTelemetryPresent",
			"providerNativeCacheTelemetry", "cacheBaselineObserved", "providerAttemptTelemetryValid", "contextEpochStateValid":
			field, ok := terminalBoolFieldForNameV1(key)
			flag, valid := value.(bool)
			if !ok || !valid || out.boolPresent[field] {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.boolPresent[field], out.bools[field] = true, flag
		case "toolSchemaTokens", "toolCount", "firstTokenLatencyMs", "durationMs", "cacheHitTokens", "cacheMissTokens",
			"providerLogicalCallCount", "providerAttemptCount", "contextEpoch":
			field, ok := terminalNumberFieldForNameV1(key)
			number, valid := terminalUnsignedNumber(value)
			if !ok || !valid || out.numPresent[field] {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.numPresent[field], out.numbers[field] = true, number
		case "cacheHitRate":
			rate, ok := terminalRate(value)
			if !ok || observedCacheRatePresent {
				return terminalCacheDiagnosticsV1{}, false
			}
			observedCacheRatePresent, observedCacheRate = true, rate
		case "prefixChangeReasons":
			reasons, ok := terminalStringList(value, terminalPrefixReasonAllowed)
			if !ok || out.prefixChangeReasons != nil {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.prefixChangeReasons = reasons
		case "toolSourceChangeReasons":
			reasons, ok := terminalStringList(value, terminalToolSourceReasonAllowed)
			if !ok || out.toolSourceChangeReasons != nil {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.toolSourceChangeReasons = reasons
		case "contextEpochChangeReasons":
			reasons, ok := terminalStringList(value, terminalContextReasonAllowed)
			if !ok || out.contextEpochChangeReasons != nil {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.contextEpochChangeReasons = reasons
		case "providerAttemptStatuses":
			parsed, ok := parseTerminalAttemptStatusesV1(value)
			if !ok || out.attemptStatuses.present {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.attemptStatuses = parsed
		case "providerAttemptInputTokens":
			if !parseTerminalTokenAggregateIntoV1(value, &out.attemptInputTokens) {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "providerAttemptOutputTokens":
			if !parseTerminalTokenAggregateIntoV1(value, &out.attemptOutputTokens) {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "providerAttemptCacheHitTokens":
			if !parseTerminalTokenAggregateIntoV1(value, &out.attemptCacheHitTokens) {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "providerAttemptCacheMissTokens":
			if !parseTerminalTokenAggregateIntoV1(value, &out.attemptCacheMissTokens) {
				return terminalCacheDiagnosticsV1{}, false
			}
		case "providerAttemptCacheRate":
			parsed, ok := parseTerminalAttemptRateV1(value)
			if !ok || out.attemptRate.present {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.attemptRate = parsed
		case "contextEpochImpact":
			parsed, ok := parseTerminalContextImpactV1(value)
			if !ok || out.contextImpact.present {
				return terminalCacheDiagnosticsV1{}, false
			}
			out.contextImpact = parsed
		default:
			return terminalCacheDiagnosticsV1{}, false
		}
	}
	if !out.validateCrossField(observedCacheRatePresent, observedCacheRate) {
		return terminalCacheDiagnosticsV1{}, false
	}
	return out, true
}

func (diagnostics terminalCacheDiagnosticsV1) publicMap() map[string]any {
	out := map[string]any{}
	if route, ok := diagnostics.route.publicString(); ok {
		out["route"] = route
	}
	for field := terminalHashFieldV1(0); field < terminalHashFieldCountV1; field++ {
		if diagnostics.hashPresent[field] {
			out[terminalHashFieldNamesV1[field]] = diagnostics.hashes[field]
		}
	}
	for field := terminalBoolFieldV1(0); field < terminalBoolFieldCountV1; field++ {
		if diagnostics.boolPresent[field] {
			out[terminalBoolFieldNamesV1[field]] = diagnostics.bools[field]
		}
	}
	for field := terminalNumberFieldV1(0); field < terminalNumberFieldCountV1; field++ {
		if diagnostics.numPresent[field] {
			out[terminalNumberFieldNamesV1[field]] = diagnostics.numbers[field]
		}
	}
	if diagnostics.cacheRate.present && diagnostics.cacheRate.known {
		out["cacheHitRate"] = float64(diagnostics.cacheRate.numerator) / float64(diagnostics.cacheRate.denominator)
	}
	if diagnostics.hasCacheTelemetrySource {
		out["cacheTelemetrySource"] = "provider_usage"
	}
	if diagnostics.hasCacheBaselineSchema {
		out["cacheBaselineSchema"] = PrefixBaselineSchemaV1
	}
	if diagnostics.hasProviderAttemptSchema {
		out["providerAttemptTelemetrySchema"] = ProviderAttemptTelemetrySchemaV1
	}
	if diagnostics.hasProviderAttemptError {
		out["providerAttemptTelemetryError"] = "invalid_or_incomplete"
	}
	putTerminalStringList(out, "prefixChangeReasons", diagnostics.prefixChangeReasons)
	putTerminalStringList(out, "toolSourceChangeReasons", diagnostics.toolSourceChangeReasons)
	putTerminalStringList(out, "contextEpochChangeReasons", diagnostics.contextEpochChangeReasons)
	if diagnostics.attemptStatuses.present {
		out["providerAttemptStatuses"] = map[string]any{
			"succeeded": diagnostics.attemptStatuses.succeeded, "failed": diagnostics.attemptStatuses.failed,
			"cancelled": diagnostics.attemptStatuses.cancelled, "timedOut": diagnostics.attemptStatuses.timedOut,
			"streamAborted": diagnostics.attemptStatuses.streamAborted,
		}
	}
	putTerminalTokenAggregate(out, "providerAttemptInputTokens", diagnostics.attemptInputTokens)
	putTerminalTokenAggregate(out, "providerAttemptOutputTokens", diagnostics.attemptOutputTokens)
	putTerminalTokenAggregate(out, "providerAttemptCacheHitTokens", diagnostics.attemptCacheHitTokens)
	putTerminalTokenAggregate(out, "providerAttemptCacheMissTokens", diagnostics.attemptCacheMissTokens)
	if diagnostics.attemptRate.present {
		out["providerAttemptCacheRate"] = map[string]any{
			"known": diagnostics.attemptRate.known, "numerator": diagnostics.attemptRate.numerator,
			"denominator": diagnostics.attemptRate.denominator,
		}
	}
	if diagnostics.contextImpact.present {
		out["contextEpochImpact"] = map[string]any{
			"stablePrefix": diagnostics.contextImpact.stablePrefix, "dynamicContext": diagnostics.contextImpact.dynamicContext,
			"turnTail": diagnostics.contextImpact.turnTail, "diagnosticsOnly": diagnostics.contextImpact.diagnosticsOnly,
		}
	}
	return out
}

func terminalPromptRouteFromAnyV1(value any) (terminalPromptRouteV1, bool) {
	text, ok := value.(string)
	if !ok {
		return terminalPromptRouteUnknownV1, false
	}
	switch text {
	case "direct_answer":
		return terminalPromptRouteDirectAnswerV1, true
	case "light_agent":
		return terminalPromptRouteLightAgentV1, true
	case "tool_agent":
		return terminalPromptRouteToolAgentV1, true
	case "subagent_agent":
		return terminalPromptRouteSubagentAgentV1, true
	default:
		return terminalPromptRouteUnknownV1, false
	}
}

func (route terminalPromptRouteV1) publicString() (string, bool) {
	switch route {
	case terminalPromptRouteDirectAnswerV1:
		return "direct_answer", true
	case terminalPromptRouteLightAgentV1:
		return "light_agent", true
	case terminalPromptRouteToolAgentV1:
		return "tool_agent", true
	case terminalPromptRouteSubagentAgentV1:
		return "subagent_agent", true
	default:
		return "", false
	}
}

func (diagnostics *terminalCacheDiagnosticsV1) validateCrossField(observedRatePresent bool, observedRate float64) bool {
	hitPresent := diagnostics.numPresent[terminalCacheHitTokensV1]
	missPresent := diagnostics.numPresent[terminalCacheMissTokensV1]
	if hitPresent != missPresent || (observedRatePresent && !hitPresent) {
		return false
	}
	if hitPresent {
		hit := diagnostics.numbers[terminalCacheHitTokensV1]
		miss := diagnostics.numbers[terminalCacheMissTokensV1]
		if hit > math.MaxUint64-miss {
			return false
		}
		total := hit + miss
		diagnostics.cacheRate = terminalAttemptRateV1{present: true, known: total > 0, numerator: hit, denominator: total}
		if observedRatePresent {
			if total == 0 {
				if observedRate != 0 {
					return false
				}
			} else if math.Abs(observedRate-float64(hit)/float64(total)) > 1e-12 {
				return false
			}
		}
	}
	if diagnostics.hasCacheTelemetrySource && !hitPresent {
		return false
	}
	if diagnostics.boolPresent[terminalCacheTelemetryPresentV1] {
		telemetryPresent := diagnostics.bools[terminalCacheTelemetryPresentV1]
		if telemetryPresent != hitPresent || (!telemetryPresent && (observedRatePresent || diagnostics.hasCacheTelemetrySource)) {
			return false
		}
	}

	attemptValidPresent := diagnostics.boolPresent[terminalProviderAttemptTelemetryValidV1]
	attemptValid := attemptValidPresent && diagnostics.bools[terminalProviderAttemptTelemetryValidV1]
	attemptDetailsPresent := diagnostics.attemptStatuses.present || diagnostics.attemptInputTokens.present ||
		diagnostics.attemptOutputTokens.present || diagnostics.attemptCacheHitTokens.present ||
		diagnostics.attemptCacheMissTokens.present || diagnostics.attemptRate.present ||
		diagnostics.numPresent[terminalProviderLogicalCallCountV1] || diagnostics.numPresent[terminalProviderAttemptCountV1]
	if diagnostics.hasProviderAttemptSchema != attemptValidPresent {
		return false
	}
	if !diagnostics.hasProviderAttemptSchema {
		if diagnostics.hasProviderAttemptError || attemptDetailsPresent {
			return false
		}
	} else if attemptValid {
		if diagnostics.hasProviderAttemptError {
			return false
		}
		if !diagnostics.numPresent[terminalProviderLogicalCallCountV1] || !diagnostics.numPresent[terminalProviderAttemptCountV1] ||
			!diagnostics.attemptStatuses.present || !diagnostics.attemptInputTokens.present || !diagnostics.attemptOutputTokens.present ||
			!diagnostics.attemptCacheHitTokens.present || !diagnostics.attemptCacheMissTokens.present || !diagnostics.attemptRate.present {
			return false
		}
		logical := diagnostics.numbers[terminalProviderLogicalCallCountV1]
		attempts := diagnostics.numbers[terminalProviderAttemptCountV1]
		if logical > attempts {
			return false
		}
		statuses := diagnostics.attemptStatuses
		statusTotal, ok := terminalCheckedSum(statuses.succeeded, statuses.failed, statuses.cancelled, statuses.timedOut, statuses.streamAborted)
		if !ok || statusTotal != attempts {
			return false
		}
	} else if !diagnostics.hasProviderAttemptError || attemptDetailsPresent {
		return false
	}
	if !validateTerminalTokenAggregateV1(diagnostics.attemptInputTokens) ||
		!validateTerminalTokenAggregateV1(diagnostics.attemptOutputTokens) ||
		!validateTerminalTokenAggregateV1(diagnostics.attemptCacheHitTokens) ||
		!validateTerminalTokenAggregateV1(diagnostics.attemptCacheMissTokens) {
		return false
	}

	if diagnostics.hasCacheBaselineSchema {
		if !diagnostics.hashPresent[terminalCacheContinuityDigestV1] || diagnostics.hashes[terminalCacheContinuityDigestV1] == "" ||
			!diagnostics.hashPresent[terminalCacheProviderNamespaceDigestV1] || diagnostics.hashes[terminalCacheProviderNamespaceDigestV1] == "" {
			return false
		}
	}
	if diagnostics.boolPresent[terminalContextEpochStateValidV1] {
		stateValid := diagnostics.bools[terminalContextEpochStateValidV1]
		contextDetailsPresent := diagnostics.numPresent[terminalContextEpochV1] ||
			diagnostics.hashPresent[terminalContextEpochDigestV1] || diagnostics.hashPresent[terminalContextEpochRegistryDigestV1] ||
			diagnostics.contextEpochChangeReasons != nil || diagnostics.contextImpact.present
		if stateValid {
			if !diagnostics.numPresent[terminalContextEpochV1] || !diagnostics.hashPresent[terminalContextEpochDigestV1] ||
				diagnostics.hashes[terminalContextEpochDigestV1] == "" || !diagnostics.hashPresent[terminalContextEpochRegistryDigestV1] ||
				diagnostics.hashes[terminalContextEpochRegistryDigestV1] == "" || diagnostics.contextEpochChangeReasons == nil ||
				!diagnostics.contextImpact.present {
				return false
			}
		} else if contextDetailsPresent {
			return false
		}
	} else if diagnostics.numPresent[terminalContextEpochV1] || diagnostics.hashPresent[terminalContextEpochDigestV1] ||
		diagnostics.hashPresent[terminalContextEpochRegistryDigestV1] || diagnostics.contextEpochChangeReasons != nil || diagnostics.contextImpact.present {
		return false
	}
	return true
}

func validateTerminalTokenAggregateV1(aggregate terminalTokenAggregateV1) bool {
	if !aggregate.present {
		return true
	}
	if aggregate.knownObservationCount > aggregate.observationCount || aggregate.complete != aggregate.hasValue {
		return false
	}
	return !aggregate.complete || aggregate.knownObservationCount == aggregate.observationCount
}

func terminalTelemetryUsageConsistentV1(usage terminalUsageV1, diagnostics terminalCacheDiagnosticsV1) bool {
	detailsPresent := diagnostics.numPresent[terminalCacheHitTokensV1] ||
		diagnostics.numPresent[terminalCacheMissTokensV1] || diagnostics.cacheRate.present ||
		diagnostics.hasCacheTelemetrySource
	if !diagnostics.boolPresent[terminalCacheTelemetryPresentV1] {
		return !detailsPresent
	}
	present := diagnostics.bools[terminalCacheTelemetryPresentV1]
	usagePresent := usage.hasCacheHit || usage.hasCacheMiss || usage.cacheHitTokens > 0 || usage.cacheMissTokens > 0
	if present != usagePresent {
		return false
	}
	if !present {
		return !detailsPresent
	}
	return diagnostics.numPresent[terminalCacheHitTokensV1] && diagnostics.numPresent[terminalCacheMissTokensV1] &&
		diagnostics.cacheRate.present && diagnostics.hasCacheTelemetrySource &&
		diagnostics.numbers[terminalCacheHitTokensV1] == uint64(usage.cacheHitTokens) &&
		diagnostics.numbers[terminalCacheMissTokensV1] == uint64(usage.cacheMissTokens)
}

func terminalCheckedSum(values ...uint64) (uint64, bool) {
	var total uint64
	for _, value := range values {
		if total > math.MaxUint64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func (diagnostics *terminalCacheDiagnosticsV1) setHash(field terminalHashFieldV1, value any) bool {
	if field >= terminalHashFieldCountV1 || diagnostics.hashPresent[field] {
		return false
	}
	hash, ok := terminalSHA256OrEmpty(value)
	if !ok {
		return false
	}
	diagnostics.hashPresent[field], diagnostics.hashes[field] = true, hash
	return true
}

func terminalHashFieldForNameV1(name string) (terminalHashFieldV1, bool) {
	for field, candidate := range terminalHashFieldNamesV1 {
		if name == candidate {
			return terminalHashFieldV1(field), true
		}
	}
	return 0, false
}

func terminalBoolFieldForNameV1(name string) (terminalBoolFieldV1, bool) {
	for field, candidate := range terminalBoolFieldNamesV1 {
		if name == candidate {
			return terminalBoolFieldV1(field), true
		}
	}
	return 0, false
}

func terminalNumberFieldForNameV1(name string) (terminalNumberFieldV1, bool) {
	for field, candidate := range terminalNumberFieldNamesV1 {
		if name == candidate {
			return terminalNumberFieldV1(field), true
		}
	}
	return 0, false
}

func terminalSHA256OrEmpty(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	if text == "" {
		return "", true
	}
	if len(text) != 64 {
		return "", false
	}
	decoded, err := hex.DecodeString(text)
	return text, err == nil && len(decoded) == 32 && strings.ToLower(text) == text
}

func terminalBoundedString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok || len(text) > maxTerminalDiagnosticString || !utf8.ValidString(text) {
		return "", false
	}
	for _, character := range text {
		if character == 0 || character < 0x20 || character == 0x7f {
			return "", false
		}
	}
	return text, true
}

func terminalUnsignedNumber(value any) (uint64, bool) {
	const maximum uint64 = 9_007_199_254_740_991
	var number uint64
	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0, false
		}
		number = uint64(typed)
	case int8:
		if typed < 0 {
			return 0, false
		}
		number = uint64(typed)
	case int16:
		if typed < 0 {
			return 0, false
		}
		number = uint64(typed)
	case int32:
		if typed < 0 {
			return 0, false
		}
		number = uint64(typed)
	case int64:
		if typed < 0 {
			return 0, false
		}
		number = uint64(typed)
	case uint:
		number = uint64(typed)
	case uint8:
		number = uint64(typed)
	case uint16:
		number = uint64(typed)
	case uint32:
		number = uint64(typed)
	case uint64:
		number = typed
	case float32:
		floatValue := float64(typed)
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || floatValue < 0 || math.Trunc(floatValue) != floatValue {
			return 0, false
		}
		number = uint64(floatValue)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed < 0 || math.Trunc(typed) != typed {
			return 0, false
		}
		number = uint64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed < 0 {
			return 0, false
		}
		number = uint64(parsed)
	default:
		return 0, false
	}
	return number, number <= maximum
}

func terminalRate(value any) (float64, bool) {
	var rate float64
	switch typed := value.(type) {
	case float32:
		rate = float64(typed)
	case float64:
		rate = typed
	case int:
		rate = float64(typed)
	case int64:
		rate = float64(typed)
	case uint64:
		rate = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		rate = parsed
	default:
		return 0, false
	}
	return rate, !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 0 && rate <= 1
}

func terminalStringList(value any, allowed func(string) bool) ([]string, bool) {
	var values []string
	switch typed := value.(type) {
	case []string:
		values = typed
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			values = append(values, text)
		}
	default:
		return nil, false
	}
	if len(values) > maxTerminalDiagnosticList {
		return nil, false
	}
	out := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, text := range values {
		if _, ok := terminalBoundedString(text); !ok || !allowed(text) {
			return nil, false
		}
		if _, duplicate := seen[text]; duplicate {
			return nil, false
		}
		seen[text] = struct{}{}
		out[index] = text
	}
	sort.Strings(out)
	return out, true
}

func terminalGenericStringAllowed(string) bool { return true }

func terminalPrefixReasonAllowed(value string) bool {
	switch value {
	case "system", "tools", "prefix", "provider", "provider_id", "endpoint_format", "model":
		return true
	default:
		return false
	}
}

func terminalToolSourceReasonAllowed(value string) bool {
	return value == "tools" || value == "tool_sources"
}

func terminalContextReasonAllowed(value string) bool {
	return domaincontextepoch.IsValidChangeReason(domaincontextepoch.ChangeReason(value))
}

func parseTerminalAttemptStatusesV1(value any) (terminalAttemptStatusesV1, bool) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 5 {
		return terminalAttemptStatusesV1{}, false
	}
	values := make([]uint64, 0, 5)
	for _, key := range []string{"succeeded", "failed", "cancelled", "timedOut", "streamAborted"} {
		number, ok := terminalUnsignedNumber(record[key])
		if !ok {
			return terminalAttemptStatusesV1{}, false
		}
		values = append(values, number)
	}
	return terminalAttemptStatusesV1{
		present: true, succeeded: values[0], failed: values[1], cancelled: values[2],
		timedOut: values[3], streamAborted: values[4],
	}, true
}

func parseTerminalTokenAggregateIntoV1(value any, target *terminalTokenAggregateV1) bool {
	if target == nil || target.present {
		return false
	}
	record, ok := value.(map[string]any)
	if !ok || len(record) < 3 || len(record) > 4 {
		return false
	}
	complete, ok := record["complete"].(bool)
	if !ok {
		return false
	}
	known, ok := terminalUnsignedNumber(record["knownObservationCount"])
	if !ok {
		return false
	}
	observations, ok := terminalUnsignedNumber(record["observationCount"])
	if !ok || known > observations {
		return false
	}
	parsed := terminalTokenAggregateV1{
		present: true, complete: complete, knownObservationCount: known, observationCount: observations,
	}
	if raw, exists := record["value"]; exists {
		parsed.value, ok = terminalUnsignedNumber(raw)
		parsed.hasValue = ok
		if !ok || !complete {
			return false
		}
	} else if complete {
		return false
	}
	for key := range record {
		if key != "complete" && key != "knownObservationCount" && key != "observationCount" && key != "value" {
			return false
		}
	}
	*target = parsed
	return true
}

func parseTerminalAttemptRateV1(value any) (terminalAttemptRateV1, bool) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 3 {
		return terminalAttemptRateV1{}, false
	}
	known, ok := record["known"].(bool)
	if !ok {
		return terminalAttemptRateV1{}, false
	}
	numerator, ok := terminalUnsignedNumber(record["numerator"])
	if !ok {
		return terminalAttemptRateV1{}, false
	}
	denominator, ok := terminalUnsignedNumber(record["denominator"])
	if !ok || numerator > denominator || (known && denominator == 0) || (!known && (numerator != 0 || denominator != 0)) {
		return terminalAttemptRateV1{}, false
	}
	return terminalAttemptRateV1{present: true, known: known, numerator: numerator, denominator: denominator}, true
}

func parseTerminalContextImpactV1(value any) (terminalContextImpactV1, bool) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 4 {
		return terminalContextImpactV1{}, false
	}
	values := make([]bool, 0, 4)
	for _, key := range []string{"stablePrefix", "dynamicContext", "turnTail", "diagnosticsOnly"} {
		flag, ok := record[key].(bool)
		if !ok {
			return terminalContextImpactV1{}, false
		}
		values = append(values, flag)
	}
	return terminalContextImpactV1{
		present: true, stablePrefix: values[0], dynamicContext: values[1], turnTail: values[2], diagnosticsOnly: values[3],
	}, true
}

func putTerminalStringList(target map[string]any, key string, values []string) {
	if values != nil {
		target[key] = append([]string{}, values...)
	}
}

func putTerminalTokenAggregate(target map[string]any, key string, aggregate terminalTokenAggregateV1) {
	if !aggregate.present {
		return
	}
	record := map[string]any{
		"complete": aggregate.complete, "knownObservationCount": aggregate.knownObservationCount,
		"observationCount": aggregate.observationCount,
	}
	if aggregate.hasValue {
		record["value"] = aggregate.value
	}
	target[key] = record
}
