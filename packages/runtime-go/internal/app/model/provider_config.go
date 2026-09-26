package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const defaultDeepSeekBaseURL = "https://api.deepseek.com"
const defaultAnalytixModel = "deepseek-v4-flash"

var ErrUnsupportedReasoningEffort = errors.New("provider reasoning effort is unsupported")

// Missing credentials are a fixed admission failure. Do not interpolate a
// user-configured identity into an error that can enter durable failure records.
var ErrMissingProviderKey = errors.New("provider configuration error: credential is required")

type RuntimeProviderConfigSet struct {
	defaultProviderID     string
	defaultBaseURL        string
	defaultAPIKey         string
	defaultEndpointFormat string
	defaultModel          string
	providers             map[string]domainmodel.ModelProviderConfig
	configurationErr      error
}

type ExecutionModelError struct {
	ProviderID     string
	Family         string
	EndpointFormat string
	BaseURL        string
	Kind           string
	Message        string
	HasAPIKey      bool
}

type TurnExecutionInput struct {
	RequestProviderID     string
	RequestModel          string
	RequestEndpointFormat string
	RequestEffort         string
	ThreadProviderID      string
	ThreadModel           string
	InternalUsageSource   string
	InternalChildRunID    string
}

type TurnExecutionResult struct {
	ProviderID  string
	Model       string
	Effort      string
	ThreadModel string
	Config      domainmodel.TurnConfig
	Authority   TurnExecutionAuthority
}

// TurnExecutionAuthority is a process-local fence for one physical Provider
// effect. It is never serialized or exposed through the public runtime
// contract; the production composition uses it only to reject results whose
// Registry winner changed while the effect was in flight.
type TurnExecutionAuthority struct {
	RegistryRevision          uint64
	RegistryIncarnation       string
	ProviderID                string
	ProviderRevision          uint64
	ProviderGeneration        uint64
	ProviderIncarnation       string
	ProviderCredentialRef     string
	ProviderCredentialPurpose string
}

func (e *ExecutionModelError) Error() string {
	if e == nil {
		return ""
	}
	providerID := firstNonEmpty(e.ProviderID, e.Family, "model provider")
	kind := strings.TrimSpace(e.Kind)
	if kind != "" {
		kind = " (" + kind + ")"
	}
	message := strings.TrimSpace(e.Message)
	if message != "" {
		message = ": " + message
	}
	return fmt.Sprintf("provider %s failed%s%s", providerID, kind, message)
}

func (e *ExecutionModelError) Diagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"providerId":     e.ProviderID,
		"family":         e.Family,
		"endpointFormat": e.EndpointFormat,
		"baseUrl":        e.BaseURL,
		"status":         float64(0),
		"kind":           e.Kind,
		"message":        e.Message,
		"hasApiKey":      e.HasAPIKey,
		"authStatus":     "none",
		"retryable":      false,
	}
}

func NewRuntimeProviderConfigSet(input domainmodel.RuntimeProviderConfigInput) RuntimeProviderConfigSet {
	set := RuntimeProviderConfigSet{
		defaultProviderID: strings.TrimSpace(input.DefaultProviderID),
		defaultBaseURL:    strings.TrimRight(strings.TrimSpace(input.DefaultBaseURL), "/"),
		defaultAPIKey:     strings.TrimSpace(input.DefaultAPIKey),
		defaultModel:      strings.TrimSpace(input.DefaultModel),
		providers:         map[string]domainmodel.ModelProviderConfig{},
	}
	defaultFormat, supplied, err := domainmodel.ParseEndpointFormat(input.DefaultEndpointFormat)
	if err != nil {
		set.configurationErr = errors.New("provider configuration error: default endpoint format is invalid")
		return set
	}
	if !supplied {
		set.defaultEndpointFormat = "chat_completions"
	} else {
		set.defaultEndpointFormat = defaultFormat
	}
	if strings.TrimSpace(input.ModelProvidersJSON) != "" {
		var parsed domainmodel.ModelProvidersConfig
		if err := decodeModelProvidersConfig(input.ModelProvidersJSON, &parsed); err != nil {
			set.configurationErr = errors.New("provider configuration error: model providers JSON is invalid")
			return set
		}
		if id := strings.TrimSpace(parsed.DefaultProviderID); id != "" {
			set.defaultProviderID = id
		}
		for _, provider := range parsed.Providers {
			id := strings.TrimSpace(provider.ID)
			endpointFormat, providerSupplied, parseErr := domainmodel.ParseEndpointFormat(provider.EndpointFormat)
			if parseErr != nil {
				set.configurationErr = errors.New("provider configuration error: provider endpoint format is invalid")
				return set
			}
			if !providerSupplied {
				endpointFormat = "chat_completions"
			}
			for modelID, profile := range provider.ModelProfiles {
				profileFormat, profileSupplied, profileErr := domainmodel.ParseEndpointFormat(profile.EndpointFormat)
				if profileErr != nil {
					set.configurationErr = errors.New("provider configuration error: model profile endpoint format is invalid")
					return set
				}
				if profileSupplied {
					profile.EndpointFormat = profileFormat
				}
				if profile.Reasoning != nil {
					if strings.TrimSpace(profile.Reasoning.RequestProtocol) != "" &&
						normalizeReasoningProtocol(profile.Reasoning.RequestProtocol) == "" {
						set.configurationErr = errors.New("provider configuration error: model profile reasoning protocol is invalid")
						return set
					}
					if err := validateReasoningEffortConfiguration(profile.Reasoning); err != nil {
						set.configurationErr = errors.New("provider configuration error: model profile reasoning effort is invalid")
						return set
					}
				}
				provider.ModelProfiles[modelID] = profile
			}
			baseURL := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
			if baseURL == "" && endpointFormat != "custom_endpoint" {
				baseURL = defaultProviderBaseURL(provider)
			}
			if id == "" || (baseURL == "" && endpointFormat != "custom_endpoint") {
				set.configurationErr = errors.New("provider configuration error: provider identity or base URL is invalid")
				return set
			}
			if _, duplicate := set.providers[id]; duplicate {
				set.configurationErr = errors.New("provider configuration error: provider identity is duplicated")
				return set
			}
			provider.ID = id
			provider.BaseURL = baseURL
			provider.APIKey = strings.TrimSpace(provider.APIKey)
			provider.EndpointFormat = endpointFormat
			set.providers[id] = provider
		}
	}
	return set
}

func decodeModelProvidersConfig(raw string, output *domainmodel.ModelProvidersConfig) error {
	if output == nil || len(raw) > 4<<20 {
		return errors.New("model providers JSON is invalid")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("model providers JSON has trailing content")
	}
	return nil
}

func (set RuntimeProviderConfigSet) ConfigurationError() error {
	return set.configurationErr
}

func (set RuntimeProviderConfigSet) TurnConfig(providerID, providerModel string) domainmodel.TurnConfig {
	return set.turnConfig(providerID, providerModel, false)
}

func (set RuntimeProviderConfigSet) TurnConfigForExecution(providerID, providerModel string) domainmodel.TurnConfig {
	return set.turnConfig(providerID, providerModel, true)
}

func (set RuntimeProviderConfigSet) ResolveTurnExecution(input TurnExecutionInput) (TurnExecutionResult, error) {
	if err := domainmodel.ValidateReasoningEffortV1(input.RequestEffort); err != nil {
		return TurnExecutionResult{}, err
	}
	if set.configurationErr != nil {
		return TurnExecutionResult{}, set.configurationErr
	}
	providerID := strings.TrimSpace(input.RequestProviderID)
	model := strings.TrimSpace(input.RequestModel)
	effort := input.RequestEffort
	if providerID == "" {
		providerID = strings.TrimSpace(input.ThreadProviderID)
	}
	threadModel := ""
	if model == "" {
		threadModel = strings.TrimSpace(input.ThreadModel)
		model = threadModel
	}
	if providerID != "" && !set.HasProvider(providerID) {
		return TurnExecutionResult{}, domainfailure.NewError(domainfailure.CodeProviderNotConfigured, nil)
	}
	explicitExecution := strings.TrimSpace(input.RequestModel) != "" ||
		strings.TrimSpace(input.InternalUsageSource) != "" ||
		strings.TrimSpace(input.InternalChildRunID) != ""
	if model != "" && (strings.TrimSpace(input.RequestModel) != "" || threadModel != "" || strings.TrimSpace(input.InternalUsageSource) != "" || strings.TrimSpace(input.InternalChildRunID) != "") {
		if err := set.ValidateExecutionModel(providerID, model); err != nil {
			return TurnExecutionResult{}, err
		}
	}
	config := set.TurnConfig(providerID, model)
	if explicitExecution {
		config = set.TurnConfigForExecution(providerID, model)
	}
	endpointFormat, endpointSupplied, endpointErr := domainmodel.ParseEndpointFormat(input.RequestEndpointFormat)
	if endpointErr != nil {
		return TurnExecutionResult{}, errors.New("provider configuration error: turn endpoint format is invalid")
	}
	if endpointSupplied {
		config.EndpointFormat = endpointFormat
		config.Family = ProviderFamily(config.ProviderID, config.BaseURL, config.Model, config.EndpointFormat)
		config.CacheTelemetrySupported = config.Family == "deepseek"
		config.DeepSeekPrefixEnhancement = config.Family == "deepseek"
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return TurnExecutionResult{}, errors.New("provider configuration error: baseUrl is required")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return TurnExecutionResult{}, ErrMissingProviderKey
	}
	if effort != "" && SupportsReasoningEffort(config) {
		if len(config.ReasoningSupportedEfforts) > 0 && !hasString(config.ReasoningSupportedEfforts, effort) {
			return TurnExecutionResult{}, ErrUnsupportedReasoningEffort
		}
		config.ReasoningEffort = effort
	}
	return TurnExecutionResult{
		ProviderID:  config.ProviderID,
		Model:       config.Model,
		Effort:      config.ReasoningEffort,
		ThreadModel: threadModel,
		Config:      config,
	}, nil
}

func (set RuntimeProviderConfigSet) turnConfig(providerID, providerModel string, preserveUnlistedModel bool) domainmodel.TurnConfig {
	requestedProviderID := strings.TrimSpace(providerID)
	requestedModel := strings.TrimSpace(providerModel)
	selectedProviderID := requestedProviderID
	if selectedProviderID == "" {
		selectedProviderID = set.defaultProviderID
	}
	if requestedModel == "" && selectedProviderID == set.defaultProviderID {
		requestedModel = set.defaultModel
	}
	if selectedProviderID != "" {
		if provider, ok := set.providers[selectedProviderID]; ok {
			return turnConfigFromProviderWithOptions(provider, requestedModel, preserveUnlistedModel)
		}
	}
	if set.defaultBaseURL != "" {
		defaultProvider := domainmodel.ModelProviderConfig{
			ID:             firstNonEmpty(selectedProviderID, "deepseek"),
			APIKey:         set.defaultAPIKey,
			BaseURL:        set.defaultBaseURL,
			EndpointFormat: set.defaultEndpointFormat,
			Models:         []string{set.defaultModel},
		}
		return turnConfigFromProviderWithOptions(defaultProvider, requestedModel, preserveUnlistedModel)
	}
	fallbackProvider := domainmodel.ModelProviderConfig{
		ID:             firstNonEmpty(selectedProviderID, "deepseek"),
		APIKey:         set.defaultAPIKey,
		EndpointFormat: set.defaultEndpointFormat,
		Models:         []string{set.defaultModel},
	}
	fallbackProvider.BaseURL = defaultProviderBaseURL(fallbackProvider)
	if fallbackProvider.BaseURL == "" {
		fallbackProvider.BaseURL = defaultDeepSeekBaseURL
	}
	return turnConfigFromProviderWithOptions(fallbackProvider, requestedModel, preserveUnlistedModel)
}

func (set RuntimeProviderConfigSet) HasProvider(providerID string) bool {
	requestedProviderID := strings.TrimSpace(providerID)
	if requestedProviderID == "" {
		return true
	}
	if _, ok := set.providers[requestedProviderID]; ok {
		return true
	}
	if len(set.providers) == 0 {
		return true
	}
	return requestedProviderID == strings.TrimSpace(set.defaultProviderID)
}

func (set RuntimeProviderConfigSet) ValidateExecutionModel(providerID string, providerModel string) error {
	requestedModel := strings.TrimSpace(providerModel)
	if requestedModel == "" {
		return nil
	}
	requestedProviderID := strings.TrimSpace(providerID)
	selectedProviderID := requestedProviderID
	if selectedProviderID == "" {
		selectedProviderID = set.defaultProviderID
	}
	provider, ok := set.providers[selectedProviderID]
	if !ok {
		return nil
	}
	if !providerHasBoundedModelCatalog(provider) {
		return nil
	}
	if _, ok := canonicalProviderModel(provider, requestedModel); ok {
		return nil
	}
	config := turnConfigFromProviderWithOptions(provider, "", false)
	message := fmt.Sprintf("model %q is not configured for provider %q", requestedModel, config.ProviderID)
	if fallback := firstProviderModelForRequest(provider); fallback != "" {
		message += fmt.Sprintf("; configured default is %q", fallback)
	}
	return &ExecutionModelError{
		ProviderID:     config.ProviderID,
		Family:         config.Family,
		EndpointFormat: config.EndpointFormat,
		BaseURL:        config.BaseURL,
		Kind:           "invalid_model",
		Message:        message,
		HasAPIKey:      strings.TrimSpace(provider.APIKey) != "",
	}
}

func (set RuntimeProviderConfigSet) Diagnostics() []map[string]any {
	ids := make([]string, 0, len(set.providers))
	for id := range set.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		id := strings.TrimSpace(set.defaultProviderID)
		if id == "" {
			id = "deepseek"
		}
		return []map[string]any{providerConfigDiagnostic(set.TurnConfig(id, set.defaultModel), 0)}
	}
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		provider := set.providers[id]
		out = append(out, providerConfigDiagnostic(set.TurnConfig(id, ""), len(provider.Models)))
	}
	return out
}

func providerConfigDiagnostic(config domainmodel.TurnConfig, modelCount int) map[string]any {
	hasAPIKey := strings.TrimSpace(config.APIKey) != ""
	hasBaseURL := strings.TrimSpace(config.BaseURL) != ""
	resolvedModel := strings.TrimSpace(config.Model)
	available := hasAPIKey && hasBaseURL && resolvedModel != ""
	reason := "provider is configured"
	switch {
	case !hasAPIKey:
		reason = "provider API key is not configured"
	case !hasBaseURL:
		reason = "provider base URL is not configured"
	case resolvedModel == "":
		reason = "provider model is not configured"
	}
	return map[string]any{
		"id":                      config.ProviderID,
		"kind":                    "model",
		"enabled":                 true,
		"available":               available,
		"reason":                  reason,
		"hasApiKey":               hasAPIKey,
		"hasBaseUrl":              hasBaseURL,
		"model":                   resolvedModel,
		"modelCount":              float64(modelCount),
		"family":                  config.Family,
		"endpointFormat":          config.EndpointFormat,
		"cacheTelemetrySupported": config.CacheTelemetrySupported,
		"supportsImageInput":      config.SupportsImageInput,
	}
}

func SupportsReasoningEffort(config domainmodel.TurnConfig) bool {
	switch normalizeReasoningProtocol(config.ReasoningProtocol) {
	case "deepseek-chat-completions", "deepseek-messages", "glm-chat-completions", "mimo-chat-completions", "openai-responses", "anthropic-thinking":
		return true
	default:
		return false
	}
}

func turnConfigFromProvider(provider domainmodel.ModelProviderConfig, requestedModel string) domainmodel.TurnConfig {
	return turnConfigFromProviderWithOptions(provider, requestedModel, false)
}

func turnConfigFromProviderWithOptions(provider domainmodel.ModelProviderConfig, requestedModel string, preserveUnlistedModel bool) domainmodel.TurnConfig {
	resolvedModel := providerModelForRequest(provider, requestedModel, preserveUnlistedModel)
	profile, hasProfile := providerProfileForModel(provider, resolvedModel)
	if resolvedModel == "" {
		resolvedModel = "deepseek-chat"
	}
	endpointFormat := domainmodel.NormalizeEndpointFormat(provider.EndpointFormat)
	if endpointFormat == "" {
		endpointFormat = "chat_completions"
	}
	reasoningProtocol := ""
	reasoningSupportedEfforts := []string(nil)
	reasoningDefaultEffort := ""
	if hasProfile {
		if strings.TrimSpace(profile.EndpointFormat) != "" {
			if format := domainmodel.NormalizeEndpointFormat(profile.EndpointFormat); format != "" {
				endpointFormat = format
			}
		}
		if profile.Reasoning != nil {
			reasoningProtocol = normalizeReasoningProtocol(profile.Reasoning.RequestProtocol)
			reasoningSupportedEfforts = normalizeReasoningEfforts(profile.Reasoning.SupportedEfforts)
			reasoningDefaultEffort = normalizeReasoningEffort(profile.Reasoning.DefaultEffort)
			if !hasString(reasoningSupportedEfforts, reasoningDefaultEffort) {
				reasoningDefaultEffort = ""
			}
		}
	}
	family := providerFamily(provider.ID, provider.BaseURL, resolvedModel, endpointFormat)
	if profile.Reasoning == nil && isOfficialDeepSeekReasoningDefault(provider, resolvedModel, endpointFormat) {
		reasoningProtocol = "deepseek-chat-completions"
		reasoningSupportedEfforts = []string{"off", "high", "max"}
		reasoningDefaultEffort = "high"
	}
	inputModalities := []string{"text"}
	messageParts := []string{"text"}
	supportsImage := false
	if hasProfile {
		inputModalities = normalizedInputModalities(profile.InputModalities)
		messageParts = normalizedMessageParts(profile.MessageParts)
		supportsImage = hasString(inputModalities, "image")
	} else if modelIDLooksMultimodal(resolvedModel) {
		inputModalities = []string{"text", "image"}
		messageParts = defaultVisionMessageParts(endpointFormat)
		supportsImage = true
	}
	config := domainmodel.TurnConfig{
		ProviderID:                strings.TrimSpace(provider.ID),
		Family:                    family,
		EndpointFormat:            endpointFormat,
		BaseURL:                   strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/"),
		ProxyURL:                  strings.TrimSpace(provider.ModelProxyURL),
		APIKey:                    strings.TrimSpace(provider.APIKey),
		Model:                     resolvedModel,
		ReasoningProtocol:         reasoningProtocol,
		ReasoningSupportedEfforts: reasoningSupportedEfforts,
		ReasoningDefaultEffort:    reasoningDefaultEffort,
		CacheTelemetrySupported:   family == "deepseek",
		DeepSeekPrefixEnhancement: family == "deepseek",
		SupportsImageInput:        supportsImage,
		InputModalities:           inputModalities,
		MessageParts:              messageParts,
		ContextWindowTokens:       profile.ContextWindowTokens,
		Pricing:                   pricingForModel(provider, resolvedModel, family),
	}
	if reasoningProtocol != "none" && reasoningDefaultEffort != "" {
		config.ReasoningEffort = reasoningDefaultEffort
	}
	return config
}

func isOfficialDeepSeekReasoningDefault(
	provider domainmodel.ModelProviderConfig,
	model string,
	endpointFormat string,
) bool {
	if strings.TrimSpace(provider.ID) != "deepseek" || endpointFormat != "chat_completions" ||
		!strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-") {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(provider.BaseURL))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() != "api.deepseek.com" ||
		parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	return path == "" || path == "/v1"
}

func providerModelForRequest(provider domainmodel.ModelProviderConfig, requestedModel string, preserveUnlistedModel bool) string {
	resolvedModel := strings.TrimSpace(requestedModel)
	if resolvedModel == "" {
		return firstProviderModelForRequest(provider)
	}
	if canonical, ok := canonicalProviderModel(provider, resolvedModel); ok {
		return canonical
	}
	if preserveUnlistedModel {
		return resolvedModel
	}
	if providerHasBoundedModelCatalog(provider) {
		if fallback := firstProviderModelForRequest(provider); fallback != "" {
			return fallback
		}
	}
	return resolvedModel
}

func providerHasBoundedModelCatalog(provider domainmodel.ModelProviderConfig) bool {
	for _, candidate := range provider.Models {
		if strings.TrimSpace(candidate) != "" {
			return true
		}
	}
	for key := range provider.ModelProfiles {
		if strings.TrimSpace(key) != "" {
			return true
		}
	}
	return false
}

func firstProviderModelForRequest(provider domainmodel.ModelProviderConfig) string {
	if strings.TrimSpace(provider.ID) == "deepseek" {
		if canonical, ok := canonicalProviderModel(provider, defaultAnalytixModel); ok {
			return canonical
		}
	}
	for _, candidate := range provider.Models {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	keys := make([]string, 0, len(provider.ModelProfiles))
	for key := range provider.ModelProfiles {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key)
		}
	}
	return ""
}

func canonicalProviderModel(provider domainmodel.ModelProviderConfig, model string) (string, bool) {
	requested := normalizeProviderModelLookup(model)
	if requested == "" {
		return "", false
	}
	if provider.ModelProfiles != nil {
		if _, ok := provider.ModelProfiles[model]; ok {
			return strings.TrimSpace(model), true
		}
		keys := make([]string, 0, len(provider.ModelProfiles))
		for key := range provider.ModelProfiles {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if normalizeProviderModelLookup(key) == requested {
				return strings.TrimSpace(key), true
			}
			profile := provider.ModelProfiles[key]
			for _, alias := range profile.Aliases {
				if normalizeProviderModelLookup(alias) == requested {
					return strings.TrimSpace(key), true
				}
			}
		}
	}
	for _, candidate := range provider.Models {
		if normalizeProviderModelLookup(candidate) == requested {
			return strings.TrimSpace(candidate), true
		}
	}
	return "", false
}

func providerProfileForModel(provider domainmodel.ModelProviderConfig, model string) (domainmodel.ModelProviderProfile, bool) {
	if provider.ModelProfiles == nil {
		return domainmodel.ModelProviderProfile{}, false
	}
	if profile, ok := provider.ModelProfiles[model]; ok {
		return profile, true
	}
	if canonical, ok := canonicalProviderModel(provider, model); ok {
		profile, hasProfile := provider.ModelProfiles[canonical]
		return profile, hasProfile
	}
	return domainmodel.ModelProviderProfile{}, false
}

func normalizeProviderModelLookup(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func normalizedInputModalities(modalities []string) []string {
	out := []string{}
	for _, modality := range modalities {
		trimmed := strings.TrimSpace(modality)
		if (trimmed == "text" || trimmed == "image") && !hasString(out, trimmed) {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"text"}
	}
	return out
}

func normalizedMessageParts(parts []string) []string {
	out := []string{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if (trimmed == "text" || trimmed == "image_url" || trimmed == "input_image") && !hasString(out, trimmed) {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"text"}
	}
	return out
}

func modelIDLooksMultimodal(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	if normalized == "mimo-v2.5" || normalized == "mimo-v2-omni" {
		return true
	}
	if strings.HasPrefix(normalized, "qwen3.7-plus") ||
		strings.HasPrefix(normalized, "qwen3.7-max") ||
		strings.HasPrefix(normalized, "qwen3.6-plus") ||
		strings.HasPrefix(normalized, "qwen3.6-flash") {
		return true
	}
	tokens := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '/' || r == ':'
	})
	for _, token := range tokens {
		if token == "audio" {
			return false
		}
	}
	if strings.HasPrefix(normalized, "gpt-4o") {
		return true
	}
	if strings.HasPrefix(normalized, "qwen-vl") || strings.HasPrefix(normalized, "qwen3-vl") {
		return true
	}
	for _, token := range tokens {
		switch token {
		case "vl", "vision", "visual", "multimodal", "omni":
			return true
		}
	}
	return false
}

func defaultVisionMessageParts(endpointFormat string) []string {
	if domainmodel.NormalizeEndpointFormat(endpointFormat) == "responses" {
		return []string{"text", "input_image"}
	}
	return []string{"text", "image_url"}
}

func hasString(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func pricingForModel(provider domainmodel.ModelProviderConfig, model string, family string) *domainmodel.Pricing {
	key := strings.TrimSpace(model)
	if key != "" {
		if provider.Prices != nil {
			if price := provider.Prices[key]; price != nil {
				return price
			}
		}
		if profile, ok := provider.ModelProfiles[key]; ok && profile.Price != nil {
			return profile.Price
		}
	}
	if provider.Price != nil {
		return provider.Price
	}
	if family == "deepseek" || strings.Contains(strings.ToLower(provider.ID), "deepseek") {
		return defaultDeepSeekPricing(key)
	}
	return nil
}

func defaultDeepSeekPricing(model string) *domainmodel.Pricing {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "deepseek-v4-flash", "deepseek-chat":
		return &domainmodel.Pricing{CacheHit: 0.02, Input: 1, Output: 2, Currency: "CNY"}
	case "deepseek-v4-pro", "deepseek-reasoner":
		return &domainmodel.Pricing{CacheHit: 0.025, Input: 3, Output: 6, Currency: "CNY"}
	default:
		return nil
	}
}

func DefaultDeepSeekPricing(model string) *domainmodel.Pricing {
	return defaultDeepSeekPricing(model)
}

func defaultProviderBaseURL(provider domainmodel.ModelProviderConfig) string {
	joined := strings.ToLower(provider.ID + " " + provider.Name + " " + provider.EndpointFormat + " " + strings.Join(provider.Models, " "))
	switch {
	case domainmodel.NormalizeEndpointFormat(provider.EndpointFormat) == "custom_endpoint":
		return ""
	case strings.Contains(joined, "anthropic") || strings.Contains(joined, "claude"):
		return "https://api.anthropic.com"
	case strings.Contains(joined, "openai") || strings.Contains(joined, "gpt"):
		return "https://api.openai.com/v1"
	case strings.Contains(joined, "deepseek") || joined == "":
		return defaultDeepSeekBaseURL
	default:
		return ""
	}
}

func providerFamily(providerID, baseURL, model, endpointFormat string) string {
	joined := strings.ToLower(providerID + " " + baseURL + " " + model)
	switch {
	case endpointFormat == "custom_endpoint":
		return "custom_endpoint"
	case endpointFormat == "messages" || strings.Contains(joined, "anthropic") || strings.Contains(joined, "claude"):
		return "anthropic-compatible"
	case strings.Contains(joined, "deepseek"):
		return "deepseek"
	default:
		return "openai-compatible"
	}
}

func normalizeReasoningProtocol(value string) string {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "/")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "none", "deepseek-chat-completions", "deepseek-messages", "glm-chat-completions", "mimo-chat-completions", "openai-responses", "anthropic-thinking":
		return normalized
	default:
		return ""
	}
}

func normalizeReasoningEfforts(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeReasoningEffort(value); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func normalizeReasoningEffort(value string) string {
	projected, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid || projected == "" {
		return ""
	}
	return projected
}

func validateReasoningEffortConfiguration(reasoning *domainmodel.ModelProviderReasoning) error {
	if reasoning == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(reasoning.SupportedEfforts))
	for _, effort := range reasoning.SupportedEfforts {
		projected, valid := domainmodel.ProjectReasoningEffortV1(effort)
		if !valid || projected == "" {
			return domainmodel.ErrInvalidReasoningEffort
		}
		if _, duplicate := seen[projected]; duplicate {
			return domainmodel.ErrInvalidReasoningEffort
		}
		seen[projected] = struct{}{}
	}
	if reasoning.DefaultEffort == "" {
		return nil
	}
	projected, valid := domainmodel.ProjectReasoningEffortV1(reasoning.DefaultEffort)
	if !valid || projected == "" {
		return domainmodel.ErrInvalidReasoningEffort
	}
	if len(seen) > 0 {
		if _, supported := seen[projected]; !supported {
			return ErrUnsupportedReasoningEffort
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
