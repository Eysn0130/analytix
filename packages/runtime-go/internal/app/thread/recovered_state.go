package thread

import (
	"sort"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type RecoveredStateInput struct {
	ThreadID    string
	Events      []map[string]any
	Diagnostics any
}

func BuildRecoveredState(input RecoveredStateInput) map[string]any {
	eventKinds := make([]string, 0, len(input.Events))
	approvalStatuses := []string{}
	userInputStatuses := []string{}
	mcpToolNames := []string{}
	providerTotals := map[string]map[string]int{}
	totalCacheHit := 0
	totalCacheMiss := 0
	latestMCPFingerprint := ""
	for _, event := range input.Events {
		kind := stringField(event, "kind")
		eventKinds = append(eventKinds, kind)
		switch kind {
		case "usage":
			provider := providerFromUsageEvent(event)
			if provider == "" {
				provider = "unknown"
			}
			if _, ok := providerTotals[provider]; !ok {
				providerTotals[provider] = map[string]int{"cacheHitTokens": 0, "cacheMissTokens": 0, "events": 0}
			}
			hit, miss := cacheTokensFromUsageEvent(event)
			providerTotals[provider]["cacheHitTokens"] += hit
			providerTotals[provider]["cacheMissTokens"] += miss
			providerTotals[provider]["events"] += 1
			totalCacheHit += hit
			totalCacheMiss += miss
		case "approval_requested", "approval_resolved":
			approvalStatuses = append(approvalStatuses, stringField(event, "status"))
		case "user_input_requested", "user_input_resolved":
			userInputStatuses = append(userInputStatuses, stringField(event, "status"))
		case "tool_catalog_changed":
			latestMCPFingerprint = stringField(event, "fingerprint")
			if names, ok := event["toolNames"].([]any); ok {
				for _, value := range names {
					if name, ok := value.(string); ok {
						mcpToolNames = append(mcpToolNames, name)
					}
				}
			}
		}
	}
	providers := make([]string, 0, len(providerTotals))
	for provider := range providerTotals {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	sort.Strings(mcpToolNames)
	highestSeq := 0
	for _, event := range input.Events {
		if seq, ok := contracts.NumericSeq(event["seq"]); ok && seq > highestSeq {
			highestSeq = seq
		}
	}
	return map[string]any{
		"threadId":   input.ThreadID,
		"eventKinds": eventKinds,
		"highestSeq": highestSeq,
		"usageCacheAccounting": map[string]any{
			"providers":            providers,
			"byProvider":           providerTotals,
			"totalCacheHitTokens":  totalCacheHit,
			"totalCacheMissTokens": totalCacheMiss,
			"cacheTelemetryLost":   false,
		},
		"approvals": map[string]any{
			"statuses":              approvalStatuses,
			"recoveredFromReplay":   contracts.ContainsString(approvalStatuses, "pending") && contracts.ContainsString(approvalStatuses, "denied"),
			"approvalExecutionUsed": false,
		},
		"userInputs": map[string]any{
			"statuses":             userInputStatuses,
			"recoveredFromReplay":  contracts.ContainsString(userInputStatuses, "pending") && contracts.ContainsString(userInputStatuses, "submitted"),
			"submittedAnswersLost": false,
			"answersPersisted":     false,
		},
		"mcp": map[string]any{
			"latestFingerprint": latestMCPFingerprint,
			"toolNames":         mcpToolNames,
			"mcpConnectionUsed": false,
			"credentialRead":    false,
		},
		"diagnostics": input.Diagnostics,
	}
}

func providerFromUsageEvent(event map[string]any) string {
	diagnostics, _ := event["cacheDiagnostics"].(map[string]any)
	for _, key := range []string{"provider", "providerId"} {
		if value, _ := diagnostics[key].(string); value != "" {
			return value
		}
	}
	return ""
}

func cacheTokensFromUsageEvent(event map[string]any) (int, int) {
	usage, _ := event["usage"].(map[string]any)
	hit, _ := contracts.NumericSeq(usage["cacheHitTokens"])
	miss, _ := contracts.NumericSeq(usage["cacheMissTokens"])
	if hit == 0 || miss == 0 {
		diagnostics, _ := event["cacheDiagnostics"].(map[string]any)
		if hit == 0 {
			hit, _ = contracts.NumericSeq(diagnostics["cacheHitTokens"])
		}
		if miss == 0 {
			miss, _ = contracts.NumericSeq(diagnostics["cacheMissTokens"])
		}
	}
	return hit, miss
}
