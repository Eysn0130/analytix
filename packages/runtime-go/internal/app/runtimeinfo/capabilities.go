package runtimeinfo

import (
	"encoding/json"
	"net/url"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type WebConfig struct {
	Enabled           bool
	FetchEnabled      bool
	SearchEnabled     bool
	Provider          string
	AllowDomains      []string
	DenyDomains       []string
	MaxFetchBytes     int
	DefaultFetchBytes int
}

type SandboxConfig struct {
	AllowWriteRoots   []string
	ProtectedReadDirs []string
}

func SandboxConfigMap(document map[string]any) (map[string]any, bool) {
	return MCPSearchConfigMap(document, "sandbox")
}

func MergeSandboxConfig(base SandboxConfig, raw map[string]any) SandboxConfig {
	base.AllowWriteRoots = uniqueStrings(append(base.AllowWriteRoots, stringList(firstNonEmptyAny(
		raw["allowWriteRoots"],
		raw["allowWrite"],
		raw["allow_write"],
	))...))
	base.ProtectedReadDirs = uniqueStrings(append(base.ProtectedReadDirs, stringList(firstNonEmptyAny(
		raw["protectedReadDirs"],
		raw["protectedDirs"],
		raw["protected_dirs"],
	))...))
	return base
}

const (
	DefaultWebTimeoutMS     = 15000
	DefaultWebMaxFetchBytes = 1000000
	MinWebFetchBytes        = 4096
)

func DefaultWebConfig() WebConfig {
	return WebConfig{
		Provider:          "fetch",
		MaxFetchBytes:     DefaultWebMaxFetchBytes,
		DefaultFetchBytes: DefaultWebMaxFetchBytes,
	}
}

func WebConfigMap(document map[string]any) (map[string]any, bool) {
	return capabilityConfigMap(document, "web")
}

func MergeWebConfig(base WebConfig, raw map[string]any) WebConfig {
	if enabled, ok := raw["enabled"].(bool); ok {
		base.Enabled = enabled
	}
	if enabled, ok := firstNonNilAny(raw["fetchEnabled"], raw["fetch_enabled"]).(bool); ok {
		base.FetchEnabled = enabled
	}
	if enabled, ok := firstNonNilAny(raw["searchEnabled"], raw["search_enabled"]).(bool); ok {
		base.SearchEnabled = enabled
	}
	if providerID := strings.TrimSpace(firstNonEmptyAnyString(raw["provider"])); providerID != "" {
		base.Provider = providerID
	}
	base.AllowDomains = uniqueStrings(append(base.AllowDomains, stringList(firstNonEmptyAny(raw["allowDomains"], raw["allow_domains"]))...))
	base.DenyDomains = uniqueStrings(append(base.DenyDomains, stringList(firstNonEmptyAny(raw["denyDomains"], raw["deny_domains"]))...))
	if value, ok := numericAny(firstNonEmptyAny(raw["maxFetchBytes"], raw["max_fetch_bytes"])); ok && value > 0 {
		base.MaxFetchBytes = value
	}
	if base.MaxFetchBytes < 1 {
		base.MaxFetchBytes = DefaultWebMaxFetchBytes
	}
	if base.DefaultFetchBytes < 1 {
		base.DefaultFetchBytes = DefaultWebMaxFetchBytes
	}
	return base
}

func DefaultProviderInfo(config domainmodel.TurnConfig) map[string]any {
	info := map[string]any{
		"providerId":              config.ProviderID,
		"model":                   config.Model,
		"endpointFormat":          config.EndpointFormat,
		"providerFamily":          config.Family,
		"providerAvailable":       strings.TrimSpace(config.APIKey) != "" && strings.TrimSpace(config.BaseURL) != "" && strings.TrimSpace(config.Model) != "",
		"hasApiKey":               strings.TrimSpace(config.APIKey) != "",
		"hasBaseUrl":              strings.TrimSpace(config.BaseURL) != "",
		"cacheTelemetrySupported": config.CacheTelemetrySupported,
		"supportsImageInput":      config.SupportsImageInput,
	}
	if config.ContextWindowTokens > 0 {
		info["contextWindowTokens"] = float64(config.ContextWindowTokens)
	}
	if effort, valid := domainmodel.ProjectReasoningEffortV1(config.ReasoningEffort); valid && effort != "" {
		info["reasoningEffort"] = effort
	}
	return info
}

type ModelCapabilityConfig struct {
	Model                     string
	ProviderID                string
	Family                    string
	EndpointFormat            string
	InputModalities           []string
	MessageParts              []string
	SupportsImageInput        bool
	ContextWindowTokens       int
	ReasoningEffort           string
	ReasoningProtocol         string
	ReasoningSupportedEfforts []string
	ReasoningDefaultEffort    string
}

func ModelCapabilityConfigFromTurnConfig(config domainmodel.TurnConfig) ModelCapabilityConfig {
	return ModelCapabilityConfig{
		Model:                     config.Model,
		ProviderID:                config.ProviderID,
		Family:                    config.Family,
		EndpointFormat:            config.EndpointFormat,
		InputModalities:           config.InputModalities,
		MessageParts:              config.MessageParts,
		SupportsImageInput:        config.SupportsImageInput,
		ContextWindowTokens:       config.ContextWindowTokens,
		ReasoningEffort:           config.ReasoningEffort,
		ReasoningProtocol:         config.ReasoningProtocol,
		ReasoningSupportedEfforts: append([]string(nil), config.ReasoningSupportedEfforts...),
		ReasoningDefaultEffort:    config.ReasoningDefaultEffort,
	}
}

type VisionBridgeConfig struct {
	Enabled                             bool
	Mode                                string
	ProviderID                          string
	BaseURL                             string
	Model                               string
	APIKey                              string
	EndpointFormat                      string
	SemanticProbeStatus                 string
	MaxImageDimension                   int
	DefaultMaxImageDimension            int
	MaxScreenshotsPerTurn               int
	DefaultScreenshotsPerTurn           int
	MaxImageBytes                       int
	DefaultMaxImageBytes                int
	ObservationCacheTTLMS               int
	InjectPolicy                        string
	FallbackWhenPrimaryImageUnsupported bool
}

const (
	DefaultVisionBridgeMaxImageBytes         = 1500000
	DefaultVisionBridgeMaxImageDimension     = 1280
	DefaultVisionBridgeScreenshotsPerTurn    = 4
	DefaultVisionBridgeObservationCacheTTLMS = 120000
)

func DefaultVisionBridgeConfig() VisionBridgeConfig {
	return VisionBridgeConfig{
		Mode:                                "auto",
		EndpointFormat:                      "chat_completions",
		MaxImageDimension:                   DefaultVisionBridgeMaxImageDimension,
		DefaultMaxImageDimension:            DefaultVisionBridgeMaxImageDimension,
		MaxImageBytes:                       DefaultVisionBridgeMaxImageBytes,
		DefaultMaxImageBytes:                DefaultVisionBridgeMaxImageBytes,
		MaxScreenshotsPerTurn:               DefaultVisionBridgeScreenshotsPerTurn,
		DefaultScreenshotsPerTurn:           DefaultVisionBridgeScreenshotsPerTurn,
		ObservationCacheTTLMS:               DefaultVisionBridgeObservationCacheTTLMS,
		InjectPolicy:                        "observation_text",
		FallbackWhenPrimaryImageUnsupported: true,
		SemanticProbeStatus:                 "unknown",
	}
}

func VisionBridgeConfigMap(document map[string]any) (map[string]any, bool) {
	return capabilityConfigMap(document, "visionBridge")
}

func MergeVisionBridgeConfig(base VisionBridgeConfig, raw map[string]any) VisionBridgeConfig {
	if enabled, ok := raw["enabled"].(bool); ok {
		base.Enabled = enabled
	}
	if rawMode := firstNonEmptyAnyString(raw["mode"]); rawMode != "" {
		if mode := NormalizeVisionBridgeMode(rawMode); mode != "" {
			base.Mode = mode
		}
	}
	if providerID := strings.TrimSpace(firstNonEmptyAnyString(raw["providerId"], raw["provider_id"])); providerID != "" {
		base.ProviderID = providerID
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmptyAnyString(raw["baseUrl"], raw["base_url"])), "/"); baseURL != "" {
		base.BaseURL = baseURL
	}
	if apiKey, ok := firstNonNilAny(raw["apiKey"], raw["api_key"]).(string); ok {
		base.APIKey = strings.TrimSpace(apiKey)
	}
	if model := strings.TrimSpace(firstNonEmptyAnyString(raw["model"])); model != "" {
		base.Model = model
	}
	if endpointFormat := domainmodel.NormalizeEndpointFormat(firstNonEmptyAnyString(raw["endpointFormat"], raw["endpoint_format"])); endpointFormat != "" {
		base.EndpointFormat = endpointFormat
	}
	if value, ok := numericAny(firstNonEmptyAny(raw["maxImageBytes"], raw["max_image_bytes"])); ok && value > 0 {
		base.MaxImageBytes = value
	}
	if value, ok := numericAny(firstNonEmptyAny(raw["maxImageDimension"], raw["max_image_dimension"])); ok && value > 0 {
		base.MaxImageDimension = value
	}
	if value, ok := numericAny(firstNonEmptyAny(raw["maxScreenshotsPerTurn"], raw["max_screenshots_per_turn"])); ok && value > 0 {
		base.MaxScreenshotsPerTurn = value
	}
	if value, ok := numericAny(firstNonEmptyAny(raw["observationCacheTtlMs"], raw["observation_cache_ttl_ms"])); ok && value > 0 {
		base.ObservationCacheTTLMS = value
	}
	if injectPolicy := strings.TrimSpace(firstNonEmptyAnyString(raw["injectPolicy"], raw["inject_policy"])); injectPolicy != "" {
		base.InjectPolicy = injectPolicy
	}
	if fallback, ok := firstNonNilAny(raw["fallbackWhenPrimaryImageUnsupported"], raw["fallback_when_primary_image_unsupported"]).(bool); ok {
		base.FallbackWhenPrimaryImageUnsupported = fallback
	}
	if status := NormalizeVisionBridgeProbeStatus(firstNonEmptyAnyString(raw["semanticProbeStatus"], raw["semantic_probe_status"])); status != "" {
		base.SemanticProbeStatus = status
	}
	if base.Mode == "" {
		base.Mode = "auto"
	}
	if base.EndpointFormat == "" {
		base.EndpointFormat = "chat_completions"
	}
	if base.MaxImageBytes < 1 {
		base.MaxImageBytes = DefaultVisionBridgeMaxImageBytes
	}
	if base.DefaultMaxImageBytes < 1 {
		base.DefaultMaxImageBytes = DefaultVisionBridgeMaxImageBytes
	}
	if base.MaxImageDimension < 1 {
		base.MaxImageDimension = DefaultVisionBridgeMaxImageDimension
	}
	if base.DefaultMaxImageDimension < 1 {
		base.DefaultMaxImageDimension = DefaultVisionBridgeMaxImageDimension
	}
	if base.MaxScreenshotsPerTurn < 1 {
		base.MaxScreenshotsPerTurn = DefaultVisionBridgeScreenshotsPerTurn
	}
	if base.DefaultScreenshotsPerTurn < 1 {
		base.DefaultScreenshotsPerTurn = DefaultVisionBridgeScreenshotsPerTurn
	}
	if base.ObservationCacheTTLMS < 1 {
		base.ObservationCacheTTLMS = DefaultVisionBridgeObservationCacheTTLMS
	}
	if strings.TrimSpace(base.InjectPolicy) == "" {
		base.InjectPolicy = "observation_text"
	}
	if strings.TrimSpace(base.SemanticProbeStatus) == "" {
		base.SemanticProbeStatus = "unknown"
	}
	return base
}

func NormalizeVisionBridgeConfig(config VisionBridgeConfig) VisionBridgeConfig {
	if !config.FallbackWhenPrimaryImageUnsupported &&
		config.DefaultMaxImageBytes == 0 &&
		config.DefaultScreenshotsPerTurn == 0 {
		config.FallbackWhenPrimaryImageUnsupported = true
	}
	return MergeVisionBridgeConfig(config, nil)
}

func NormalizeVisionBridgeMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "auto", "":
		return "auto"
	case "always":
		return "always"
	case "off", "disabled", "false":
		return "off"
	default:
		return ""
	}
}

func NormalizeVisionBridgeProbeStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "unknown", "supported", "unsupported", "semantic_failed", "auth_failed", "http_failed", "timeout", "failed", "stale":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

type MCPSearchConfig struct {
	Enabled                bool
	Mode                   string
	AutoThresholdToolCount int
	TopKDefault            int
	TopKMax                int
	MinScore               float64
}

func DefaultMCPSearchConfig() MCPSearchConfig {
	return MCPSearchConfig{
		Enabled:                false,
		Mode:                   "auto",
		AutoThresholdToolCount: 24,
		TopKDefault:            5,
		TopKMax:                10,
		MinScore:               0.15,
	}
}

func MCPSearchConfigMap(document map[string]any, key string) (map[string]any, bool) {
	if direct, ok := document[key].(map[string]any); ok {
		return direct, true
	}
	return MCPNestedConfigMap(document, key)
}

func MCPNestedConfigMap(document map[string]any, key string) (map[string]any, bool) {
	if mcpConfig, ok := document["mcp"].(map[string]any); ok {
		if nested, ok := mcpConfig[key].(map[string]any); ok {
			return nested, true
		}
	}
	if capabilities, ok := document["capabilities"].(map[string]any); ok {
		if mcpConfig, ok := capabilities["mcp"].(map[string]any); ok {
			if nested, ok := mcpConfig[key].(map[string]any); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func capabilityConfigMap(document map[string]any, key string) (map[string]any, bool) {
	if direct, ok := document[key].(map[string]any); ok {
		return direct, true
	}
	if capabilities, ok := document["capabilities"].(map[string]any); ok {
		if nested, ok := capabilities[key].(map[string]any); ok {
			return nested, true
		}
	}
	return nil, false
}

func MergeMCPSearchConfig(base MCPSearchConfig, raw map[string]any) MCPSearchConfig {
	if enabled, ok := raw["enabled"].(bool); ok {
		base.Enabled = enabled
	}
	if mode := strings.TrimSpace(stringField(raw, "mode")); mode == "direct" || mode == "search" || mode == "auto" {
		base.Mode = mode
	}
	if value, ok := numericAny(raw["autoThresholdToolCount"]); ok && value > 0 {
		base.AutoThresholdToolCount = value
	}
	if value, ok := numericAny(raw["topKDefault"]); ok && value > 0 {
		base.TopKDefault = value
	}
	if value, ok := numericAny(raw["topKMax"]); ok && value > 0 {
		base.TopKMax = value
	}
	if _, ok := raw["minScore"]; ok {
		if value := floatFromAny(raw["minScore"]); value >= 0 {
			base.MinScore = value
		}
	}
	if base.TopKMax < base.TopKDefault {
		base.TopKMax = base.TopKDefault
	}
	return base
}

func AttachmentCapabilityState(
	maxImageBytes int,
	maxImageDimension int,
	allowedMimeTypes []string,
	allowedDocumentMimeTypes []string,
	maxDocumentBytes int,
	maxDocumentTextChars int,
	fallbackMaxBase64Bytes int,
	fallbackMaxImageDimension int,
	fallbackPreferredMimeType string,
) map[string]any {
	return map[string]any{
		"status":                        "available",
		"enabled":                       true,
		"available":                     true,
		"reason":                        "persistent attachment store is available",
		"maxImageBytes":                 float64(maxImageBytes),
		"maxImageDimension":             float64(maxImageDimension),
		"allowedMimeTypes":              stringListAny(allowedMimeTypes),
		"allowedDocumentMimeTypes":      stringListAny(allowedDocumentMimeTypes),
		"maxDocumentBytes":              float64(maxDocumentBytes),
		"maxDocumentTextChars":          float64(maxDocumentTextChars),
		"textFallbackMaxBase64Bytes":    float64(fallbackMaxBase64Bytes),
		"textFallbackMaxImageDimension": float64(fallbackMaxImageDimension),
		"textFallbackPreferredMimeType": fallbackPreferredMimeType,
	}
}

func MemoryCapabilityState() map[string]any {
	return map[string]any{
		"status":             "available",
		"enabled":            true,
		"available":          true,
		"reason":             "manual persistent record store is available; model injection and automatic capture are unavailable",
		"mode":               "manual",
		"storeOnly":          true,
		"modelInjection":     false,
		"automaticCapture":   false,
		"scopes":             []any{"user", "workspace", "project"},
		"maxInjectedRecords": float64(0),
	}
}

func ApplyDefaultModelCapability(capabilities map[string]any, config ModelCapabilityConfig) {
	modelCapability, ok := capabilities["model"].(map[string]any)
	if !ok {
		return
	}
	modelCapability["id"] = config.Model
	modelCapability["providerId"] = config.ProviderID
	modelCapability["family"] = config.Family
	modelCapability["endpointFormat"] = config.EndpointFormat
	modelCapability["inputModalities"] = stringListAny(config.InputModalities)
	modelCapability["messageParts"] = stringListAny(config.MessageParts)
	modelCapability["supportsImageInput"] = config.SupportsImageInput
	if config.ContextWindowTokens > 0 {
		modelCapability["contextWindowTokens"] = float64(config.ContextWindowTokens)
	} else {
		delete(modelCapability, "contextWindowTokens")
	}
	configuredReasoningProtocol := strings.TrimSpace(config.ReasoningProtocol)
	supportedReasoningEfforts := normalizedPublicReasoningEfforts(config.ReasoningSupportedEfforts)
	defaultReasoningEffort, validDefaultEffort := domainmodel.ProjectReasoningEffortV1(config.ReasoningDefaultEffort)
	if configuredReasoningProtocol == "" || configuredReasoningProtocol == "none" ||
		!validDefaultEffort || defaultReasoningEffort == "" ||
		len(supportedReasoningEfforts) == 0 || !containsString(supportedReasoningEfforts, defaultReasoningEffort) {
		delete(modelCapability, "reasoning")
		return
	}
	modelCapability["reasoning"] = map[string]any{
		"supportedEfforts": stringListAny(supportedReasoningEfforts),
		"defaultEffort":    defaultReasoningEffort,
		"requestProtocol":  configuredReasoningProtocol,
	}
}

func normalizedPublicReasoningEfforts(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized, valid := domainmodel.ProjectReasoningEffortV1(value)
		if !valid || normalized == "" {
			return nil
		}
		if _, ok := seen[normalized]; ok {
			return nil
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func DefaultReasoningProtocol(config ModelCapabilityConfig) string {
	if protocol := strings.TrimSpace(config.ReasoningProtocol); protocol != "" {
		return protocol
	}
	return "none"
}

func MCPCapabilityState(searchConfig MCPSearchConfig, diagnostics map[string]any) map[string]any {
	available := boolField(diagnostics, "available")
	configuredServers := floatFromAny(diagnostics["configuredServerCount"])
	enabled := available || boolField(diagnostics, "enabled") || configuredServers > 0
	status := "unavailable"
	if available {
		status = "available"
	} else if !enabled {
		status = "disabled"
	}
	return map[string]any{
		"available":         available,
		"enabled":           enabled,
		"status":            status,
		"reason":            stringField(diagnostics, "reason"),
		"configuredServers": configuredServers,
		"connectedServers":  floatFromAny(diagnostics["connectedServerCount"]),
		"search":            MCPSearchDiagnostics(searchConfig, diagnostics),
		"toolCount":         floatFromAny(diagnostics["indexedToolCount"]),
		"promptCount":       floatFromAny(diagnostics["promptCount"]),
		"resourceCount":     floatFromAny(diagnostics["resourceCount"]),
		"catalog":           MCPCatalogDiagnostics(diagnostics),
	}
}

func MCPSearchDiagnostics(config MCPSearchConfig, diagnostics map[string]any) map[string]any {
	available := config.Enabled && boolField(diagnostics, "available")
	indexedToolCount := floatFromAny(diagnostics["indexedToolCount"])
	advertisedToolCount := floatFromAny(diagnostics["advertisedToolCount"])
	searchActive := available && MCPSearchActive(config, int(indexedToolCount))
	return map[string]any{
		"enabled":                config.Enabled,
		"mode":                   config.Mode,
		"active":                 searchActive,
		"available":              available,
		"reason":                 stringField(diagnostics, "reason"),
		"indexedToolCount":       indexedToolCount,
		"advertisedToolCount":    advertisedToolCount,
		"autoThresholdToolCount": float64(config.AutoThresholdToolCount),
		"topKDefault":            float64(config.TopKDefault),
		"topKMax":                float64(config.TopKMax),
		"minScore":               config.MinScore,
		"catalogFingerprint":     stringField(diagnostics, "catalogFingerprint"),
		"catalogDrift":           boolField(diagnostics, "catalogDrift"),
	}
}

func MCPCatalogDiagnostics(diagnostics map[string]any) map[string]any {
	return map[string]any{
		"status":              stringField(diagnostics, "status"),
		"reason":              stringField(diagnostics, "reason"),
		"toolCount":           floatFromAny(diagnostics["indexedToolCount"]),
		"advertisedToolCount": floatFromAny(diagnostics["advertisedToolCount"]),
		"promptCount":         floatFromAny(diagnostics["promptCount"]),
		"resourceCount":       floatFromAny(diagnostics["resourceCount"]),
		"catalogFingerprint":  stringField(diagnostics, "catalogFingerprint"),
		"catalogDrift":        boolField(diagnostics, "catalogDrift"),
	}
}

func MCPSearchActive(config MCPSearchConfig, toolCount int) bool {
	return MCPSearchShouldScope(config, toolCount, true)
}

func MCPSearchShouldScope(config MCPSearchConfig, toolCount int, hasPrompt bool) bool {
	if !config.Enabled || toolCount <= 0 || !hasPrompt {
		return false
	}
	switch config.Mode {
	case "direct":
		return false
	case "search":
		return config.Enabled
	default:
		return toolCount >= config.AutoThresholdToolCount
	}
}

func MCPSearchResultLimit(config MCPSearchConfig) int {
	limit := config.TopKDefault
	if limit <= 0 {
		limit = 5
	}
	maxLimit := config.TopKMax
	if maxLimit <= 0 {
		maxLimit = limit
	}
	return minInt(limit, maxLimit)
}

func SubagentCapabilityState(durableChildRunStore bool, toolNames []string) map[string]any {
	tools := map[string]bool{}
	for _, name := range toolNames {
		if normalized := strings.TrimSpace(name); normalized != "" {
			tools[normalized] = true
		}
	}
	taskToolAvailable := tools["task"]
	parallelTasksToolAvailable := tools["parallel_tasks"]
	waitAvailable := tools["wait"]
	outputAvailable := tools["bash_output"] || tools["task_output"]
	killAvailable := tools["kill_shell"] || tools["kill_task"]
	modelJobToolsAvailable := durableChildRunStore && waitAvailable && outputAvailable && killAvailable
	parallelExecutionAvailable := durableChildRunStore && taskToolAvailable && parallelTasksToolAvailable
	available := parallelExecutionAvailable && modelJobToolsAvailable
	reason := ""
	if !durableChildRunStore {
		reason = "durable child-run store is unavailable"
	} else if !taskToolAvailable {
		reason = "task tool is unavailable"
	} else if !parallelTasksToolAvailable {
		reason = "parallel_tasks tool is unavailable"
	} else if !modelJobToolsAvailable {
		reason = "task job tools are unavailable"
	}
	state := map[string]any{
		"status":                          map[bool]string{true: "available", false: "unavailable"}[available],
		"enabled":                         true,
		"available":                       available,
		"internalLineageAvailable":        durableChildRunStore,
		"durableChildRunStore":            durableChildRunStore,
		"parallelExecutionAvailable":      parallelExecutionAvailable,
		"taskToolAvailable":               taskToolAvailable,
		"parallelTasksToolAvailable":      parallelTasksToolAvailable,
		"backgroundTaskJobsAvailable":     modelJobToolsAvailable,
		"backgroundSubagentJobsAvailable": durableChildRunStore && taskToolAvailable && modelJobToolsAvailable,
		"taskJobThreadScopeSupported":     modelJobToolsAvailable,
		"modelJobToolsAvailable":          modelJobToolsAvailable,
		"backgroundShellAvailable":        waitAvailable && outputAvailable && killAvailable,
		"topLevelRouteExposed":            false,
		"profilesAvailable":               false,
		"profiles":                        []any{},
	}
	if reason != "" {
		state["reason"] = reason
	} else if available {
		state["reason"] = "subagents are available"
	}
	return state
}

func ComputerUseCapabilityState(serverDiagnostics []any) map[string]any {
	state := map[string]any{
		"status":    "unavailable",
		"enabled":   false,
		"available": false,
		"mode":      "auto",
		"reason":    "computer-use MCP backend is not connected",
	}
	for _, raw := range serverDiagnostics {
		server, ok := raw.(map[string]any)
		if !ok || !ComputerUseServerID(stringField(server, "id")) {
			continue
		}
		toolNames := stringList(server["toolNames"])
		serverID := stringField(server, "id")
		hasStateTool := containsString(toolNames, "mcp__"+serverID+"__get_app_state")
		hasActionTool := containsString(toolNames, "mcp__"+serverID+"__select_text") ||
			containsString(toolNames, "mcp__"+serverID+"__click") ||
			containsString(toolNames, "mcp__"+serverID+"__type_text") ||
			containsString(toolNames, "mcp__"+serverID+"__set_value")
		available := boolField(server, "available") && hasStateTool && hasActionTool
		state["enabled"] = boolField(server, "enabled")
		state["available"] = available
		state["status"] = map[bool]string{true: "available", false: "unavailable"}[available]
		state["backendId"] = serverID
		state["preferredBackendId"] = "analytix-computer-use"
		state["provider"] = "mcp"
		state["toolCount"] = floatFromAny(server["toolCount"])
		if available {
			state["reason"] = "Analytix Computer Use MCP backend is connected"
		} else if reason := stringField(server, "failure"); reason != "" {
			state["reason"] = reason
		} else if !boolField(server, "available") {
			state["reason"] = "computer-use MCP backend is configured but not connected"
		} else if !hasStateTool || !hasActionTool {
			state["reason"] = "computer-use MCP backend is missing required tools"
		}
		return state
	}
	return state
}

func ComputerUseServerID(id string) bool {
	switch strings.TrimSpace(id) {
	case "analytix-computer-use", "computer-use":
		return true
	default:
		return false
	}
}

func WebCapabilityState(config WebConfig) map[string]any {
	fetchState := disabledState("web fetch is disabled by config")
	searchState := disabledState("web search is disabled by config")
	if config.Enabled && config.FetchEnabled {
		fetchState = availableState()
		fetchState["provider"] = WebProviderID(config)
	}
	if config.Enabled && config.SearchEnabled {
		searchState = unavailableState("web search provider is unavailable")
	}
	available := boolField(fetchState, "available") || boolField(searchState, "available")
	reason := ""
	if !config.Enabled {
		reason = "web access is disabled by config"
	} else if !available {
		reason = "web tools are disabled by config"
	}
	return map[string]any{
		"status":        map[bool]string{true: "available", false: "disabled"}[available],
		"enabled":       config.Enabled,
		"available":     available,
		"reason":        reason,
		"provider":      WebProviderID(config),
		"fetchEnabled":  config.Enabled && config.FetchEnabled,
		"searchEnabled": config.Enabled && config.SearchEnabled,
		"fetch":         fetchState,
		"search":        searchState,
	}
}

func WebProviderDiagnostics(config WebConfig) []any {
	if !config.Enabled {
		return []any{}
	}
	fetchAvailable := config.FetchEnabled
	searchAvailable := false
	reason := ""
	if !fetchAvailable && !config.SearchEnabled {
		reason = "web tools are disabled by config"
	} else if config.SearchEnabled && !searchAvailable && !fetchAvailable {
		reason = "web search provider is unavailable"
	}
	record := map[string]any{
		"id":              "web",
		"enabled":         true,
		"available":       fetchAvailable || searchAvailable,
		"fetchAvailable":  fetchAvailable,
		"searchAvailable": searchAvailable,
		"provider":        WebProviderID(config),
		"maxFetchBytes":   float64(WebMaxFetchBytes(config)),
	}
	if reason != "" {
		record["reason"] = reason
	}
	return []any{record}
}

func WebProviderID(config WebConfig) string {
	if providerID := strings.TrimSpace(config.Provider); providerID != "" {
		return providerID
	}
	return "fetch"
}

func WebFetchEnabled(config WebConfig) bool {
	return config.Enabled && config.FetchEnabled
}

func WebMaxFetchBytes(config WebConfig) int {
	if config.MaxFetchBytes > 0 {
		return config.MaxFetchBytes
	}
	return config.DefaultFetchBytes
}

func VisionBridgeCapabilityState(config VisionBridgeConfig) map[string]any {
	ready, reason := VisionBridgeReadyReason(config)
	status := "unavailable"
	if !config.Enabled || config.Mode == "off" {
		status = "disabled"
	} else if ready {
		status = "available"
		if reason == "" {
			reason = "vision bridge is available"
		}
	}
	state := map[string]any{
		"status":                status,
		"enabled":               config.Enabled,
		"available":             ready,
		"mode":                  firstNonEmptyAnyString(config.Mode, "auto"),
		"maxScreenshotsPerTurn": float64(VisionBridgeMaxScreenshots(config)),
		"maxImagesPerTurn":      float64(VisionBridgeMaxScreenshots(config)),
		"maxImageBytes":         float64(VisionBridgeMaxImageBytes(config)),
		"maxImageDimension":     float64(VisionBridgeMaxImageDimension(config)),
		"semanticProbeStatus":   firstNonEmptyAnyString(config.SemanticProbeStatus, "unknown"),
	}
	if reason != "" {
		state["reason"] = reason
	}
	if providerID := strings.TrimSpace(config.ProviderID); providerID != "" {
		state["providerId"] = providerID
	}
	if model := strings.TrimSpace(config.Model); model != "" {
		state["model"] = model
	}
	return state
}

func VisionBridgeReadyReason(config VisionBridgeConfig) (bool, string) {
	if !config.Enabled || config.Mode == "off" {
		return false, "vision bridge is disabled by config"
	}
	if strings.TrimSpace(config.ProviderID) == "" {
		return false, "vision bridge provider is not configured"
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return false, "vision bridge base URL is not configured"
	}
	if strings.TrimSpace(config.Model) == "" {
		return false, "vision bridge model is not configured"
	}
	if VisionBridgeRequiresAPIKey(config.BaseURL) && strings.TrimSpace(config.APIKey) == "" {
		return false, "vision bridge API key is not configured"
	}
	if strings.TrimSpace(config.SemanticProbeStatus) != "supported" {
		return false, "vision bridge semantic probe has not passed"
	}
	return true, ""
}

func VisionBridgeRequiresAPIKey(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return true
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}

func VisionBridgeMaxScreenshots(config VisionBridgeConfig) int {
	if config.MaxScreenshotsPerTurn > 0 {
		return config.MaxScreenshotsPerTurn
	}
	return config.DefaultScreenshotsPerTurn
}

func VisionBridgeMaxImageBytes(config VisionBridgeConfig) int {
	if config.MaxImageBytes > 0 {
		return config.MaxImageBytes
	}
	return config.DefaultMaxImageBytes
}

func VisionBridgeMaxImageDimension(config VisionBridgeConfig) int {
	if config.MaxImageDimension > 0 {
		return config.MaxImageDimension
	}
	return config.DefaultMaxImageDimension
}

func availableState() map[string]any {
	return map[string]any{"status": "available", "enabled": true, "available": true}
}

func unavailableState(reason string) map[string]any {
	return map[string]any{"status": "unavailable", "enabled": false, "available": false, "reason": reason}
}

func disabledState(reason string) map[string]any {
	return map[string]any{"status": "disabled", "enabled": false, "available": false, "reason": reason}
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	default:
		return 0
	}
	return 0
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed), true
		}
	}
	return 0, false
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func firstNonNilAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case []string:
			if len(typed) > 0 {
				return typed
			}
		case []any:
			if len(typed) > 0 {
				return typed
			}
		case nil:
			continue
		default:
			return value
		}
	}
	return nil
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func stringList(value any) []string {
	raw := []any{}
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func containsString(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
