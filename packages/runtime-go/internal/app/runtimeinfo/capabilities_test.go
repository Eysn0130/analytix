package runtimeinfo

import (
	"reflect"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestDefaultProviderInfoSanitizesAvailability(t *testing.T) {
	info := DefaultProviderInfo(domainmodel.TurnConfig{
		Model:                   "mimo-v2",
		ProviderID:              "xiaomi",
		Family:                  "openai-compatible",
		EndpointFormat:          "chat_completions",
		APIKey:                  "test-key",
		BaseURL:                 "https://provider.example/v1",
		SupportsImageInput:      true,
		CacheTelemetrySupported: true,
	})
	if info["providerId"] != "xiaomi" || info["model"] != "mimo-v2" || info["providerFamily"] != "openai-compatible" {
		t.Fatalf("provider identity mismatch: %#v", info)
	}
	if info["providerAvailable"] != true || info["hasApiKey"] != true || info["hasBaseUrl"] != true {
		t.Fatalf("provider availability mismatch: %#v", info)
	}
	if info["supportsImageInput"] != true || info["cacheTelemetrySupported"] != true {
		t.Fatalf("provider capabilities mismatch: %#v", info)
	}
	invalid := DefaultProviderInfo(domainmodel.TurnConfig{ReasoningEffort: " high "})
	if _, exists := invalid["reasoningEffort"]; exists {
		t.Fatalf("invalid reasoning effort was normalized into runtime info: %#v", invalid)
	}
}

func TestPublicReasoningEffortUsesExactClosedProjection(t *testing.T) {
	for _, effort := range []string{"", "auto", "off", "low", "medium", "high", "max"} {
		if projected := publicReasoningEffort(effort); projected != effort {
			t.Fatalf("valid effort projection mismatch: got=%q want=%q", projected, effort)
		}
	}
	for _, effort := range []string{" high ", "HIGH", "adaptive", "xhigh"} {
		if projected := publicReasoningEffort(effort); projected != "" {
			t.Fatalf("invalid effort leaked into public projection: raw=%q projected=%q", effort, projected)
		}
	}
}

func TestSubagentCapabilityStateRequiresDurableTools(t *testing.T) {
	available := SubagentCapabilityState(true, []string{"read", "task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job"})
	if available["status"] != "available" || available["available"] != true || available["parallelExecutionAvailable"] != true {
		t.Fatalf("available subagent state mismatch: %#v", available)
	}
	missingParallel := SubagentCapabilityState(true, []string{"read", "task", "wait", "bash_output", "kill_shell", "restart_job"})
	if missingParallel["available"] != false || missingParallel["reason"] != "parallel_tasks tool is unavailable" {
		t.Fatalf("missing parallel_tasks state mismatch: %#v", missingParallel)
	}
	missingStore := SubagentCapabilityState(false, []string{"task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job"})
	if missingStore["available"] != false || missingStore["reason"] != "durable child-run store is unavailable" {
		t.Fatalf("missing durable store state mismatch: %#v", missingStore)
	}
}

func TestWebCapabilityAndDiagnosticsUseConfiguredProvider(t *testing.T) {
	config := WebConfig{Enabled: true, FetchEnabled: true, SearchEnabled: true, Provider: "mcp-web", MaxFetchBytes: 4096, DefaultFetchBytes: 1024}
	state := WebCapabilityState(config)
	if state["status"] != "available" || state["provider"] != "mcp-web" || state["fetchEnabled"] != true || state["searchEnabled"] != true {
		t.Fatalf("web state mismatch: %#v", state)
	}
	fetch, _ := state["fetch"].(map[string]any)
	if fetch["provider"] != "mcp-web" {
		t.Fatalf("fetch provider mismatch: %#v", fetch)
	}
	diagnostics := WebProviderDiagnostics(config)
	record, _ := diagnostics[0].(map[string]any)
	if record["provider"] != "mcp-web" || record["maxFetchBytes"] != float64(4096) {
		t.Fatalf("web diagnostics mismatch: %#v", diagnostics)
	}
}

func TestMergeWebConfigReadsCapabilityDocumentAndPolicies(t *testing.T) {
	document := map[string]any{
		"capabilities": map[string]any{
			"web": map[string]any{
				"enabled":       true,
				"fetch_enabled": true,
				"searchEnabled": true,
				"provider":      "mcp-web",
				"allow_domains": []any{"docs.example.com", "docs.example.com"},
				"denyDomains":   []any{"internal.example.com"},
				"maxFetchBytes": float64(8192),
			},
		},
	}
	raw, ok := WebConfigMap(document)
	if !ok {
		t.Fatal("expected capabilities.web config")
	}
	config := MergeWebConfig(DefaultWebConfig(), raw)
	if !config.Enabled || !config.FetchEnabled || !config.SearchEnabled || config.Provider != "mcp-web" {
		t.Fatalf("web config booleans/provider mismatch: %#v", config)
	}
	if config.MaxFetchBytes != 8192 || config.DefaultFetchBytes != DefaultWebMaxFetchBytes {
		t.Fatalf("web config fetch limits mismatch: %#v", config)
	}
	if len(config.AllowDomains) != 1 || config.AllowDomains[0] != "docs.example.com" {
		t.Fatalf("allow domains should be unique and ordered: %#v", config.AllowDomains)
	}
	if len(config.DenyDomains) != 1 || config.DenyDomains[0] != "internal.example.com" {
		t.Fatalf("deny domains mismatch: %#v", config.DenyDomains)
	}

	defaulted := MergeWebConfig(WebConfig{}, map[string]any{"max_fetch_bytes": float64(-1)})
	if defaulted.MaxFetchBytes != DefaultWebMaxFetchBytes || defaulted.DefaultFetchBytes != DefaultWebMaxFetchBytes {
		t.Fatalf("invalid limits should default: %#v", defaulted)
	}
}

func TestMergeSandboxConfigReadsNestedMCPDocument(t *testing.T) {
	document := map[string]any{
		"capabilities": map[string]any{
			"mcp": map[string]any{
				"sandbox": map[string]any{
					"allow_write":       []any{"/tmp/project", "/tmp/project"},
					"protectedReadDirs": []any{"/Users/me/Library"},
				},
			},
		},
	}
	raw, ok := SandboxConfigMap(document)
	if !ok {
		t.Fatal("expected nested capabilities.mcp.sandbox config")
	}
	config := MergeSandboxConfig(SandboxConfig{
		AllowWriteRoots:   []string{"/workspace"},
		ProtectedReadDirs: []string{"/Users/me/Pictures"},
	}, raw)
	if len(config.AllowWriteRoots) != 2 || config.AllowWriteRoots[0] != "/workspace" || config.AllowWriteRoots[1] != "/tmp/project" {
		t.Fatalf("sandbox allow roots should merge uniquely and preserve order: %#v", config.AllowWriteRoots)
	}
	if len(config.ProtectedReadDirs) != 2 || config.ProtectedReadDirs[1] != "/Users/me/Library" {
		t.Fatalf("sandbox protected dirs mismatch: %#v", config.ProtectedReadDirs)
	}
}

func TestMergeVisionBridgeConfigReadsCapabilityDocument(t *testing.T) {
	document := map[string]any{
		"capabilities": map[string]any{
			"visionBridge": map[string]any{
				"enabled":                             true,
				"mode":                                "always",
				"provider_id":                         "xiaomi",
				"base_url":                            "https://api.xiaomimimo.com/v1/",
				"apiKey":                              "bridge-secret",
				"endpointFormat":                      "responses",
				"model":                               "mimo-v2.5",
				"max_image_bytes":                     float64(8192),
				"maxScreenshotsPerTurn":               float64(2),
				"observationCacheTtlMs":               float64(30000),
				"inject_policy":                       "observation_text",
				"fallbackWhenPrimaryImageUnsupported": false,
				"semanticProbeStatus":                 "supported",
			},
		},
	}
	raw, ok := VisionBridgeConfigMap(document)
	if !ok {
		t.Fatal("expected capabilities.visionBridge config")
	}
	config := MergeVisionBridgeConfig(DefaultVisionBridgeConfig(), raw)
	if !config.Enabled || config.Mode != "always" || config.ProviderID != "xiaomi" || config.Model != "mimo-v2.5" {
		t.Fatalf("vision bridge identity mismatch: %#v", config)
	}
	if config.BaseURL != "https://api.xiaomimimo.com/v1" || config.APIKey != "bridge-secret" || config.EndpointFormat != "responses" {
		t.Fatalf("vision bridge endpoint fields mismatch: %#v", config)
	}
	if config.MaxImageBytes != 8192 || config.MaxScreenshotsPerTurn != 2 || config.ObservationCacheTTLMS != 30000 {
		t.Fatalf("vision bridge limits mismatch: %#v", config)
	}
	if config.FallbackWhenPrimaryImageUnsupported || config.SemanticProbeStatus != "supported" {
		t.Fatalf("vision bridge booleans/status mismatch: %#v", config)
	}

	defaulted := MergeVisionBridgeConfig(VisionBridgeConfig{}, map[string]any{
		"mode":                "invalid",
		"semanticProbeStatus": "invalid",
	})
	if defaulted.Mode != "auto" || defaulted.EndpointFormat != "chat_completions" || defaulted.SemanticProbeStatus != "unknown" {
		t.Fatalf("invalid fields should default: %#v", defaulted)
	}
}

func TestNormalizeVisionBridgeConfigAppliesDefaults(t *testing.T) {
	config := NormalizeVisionBridgeConfig(VisionBridgeConfig{})
	if config.MaxImageBytes != DefaultVisionBridgeMaxImageBytes ||
		config.MaxScreenshotsPerTurn != DefaultVisionBridgeScreenshotsPerTurn ||
		config.DefaultMaxImageBytes != DefaultVisionBridgeMaxImageBytes ||
		config.DefaultScreenshotsPerTurn != DefaultVisionBridgeScreenshotsPerTurn {
		t.Fatalf("vision bridge defaults were not normalized: %#v", config)
	}
}

func TestApplyDefaultModelCapabilityProjectsProviderModelAndReasoning(t *testing.T) {
	capabilities := map[string]any{
		"model": map[string]any{
			"reasoning": map[string]any{
				"supportedEfforts": []any{"off", "high", "max"},
				"defaultEffort":    "max",
				"requestProtocol":  "deepseek-chat-completions",
			},
		},
	}
	ApplyDefaultModelCapability(capabilities, ModelCapabilityConfig{
		Model:                     "mimo-v2.5-pro",
		ProviderID:                "xiaomi-token-plan",
		Family:                    "openai-compatible",
		EndpointFormat:            "chat_completions",
		InputModalities:           []string{"text", "image"},
		MessageParts:              []string{"text", "image_url"},
		SupportsImageInput:        true,
		ReasoningEffort:           "high",
		ReasoningProtocol:         "mimo-chat-completions",
		ReasoningSupportedEfforts: []string{"off", "low", "medium", "high"},
		ReasoningDefaultEffort:    "high",
	})
	model, _ := capabilities["model"].(map[string]any)
	if model["id"] != "mimo-v2.5-pro" || model["providerId"] != "xiaomi-token-plan" || model["supportsImageInput"] != true {
		t.Fatalf("model capability mismatch: %#v", model)
	}
	reasoning, _ := model["reasoning"].(map[string]any)
	if reasoning["requestProtocol"] != "mimo-chat-completions" || reasoning["defaultEffort"] != "high" ||
		!reflect.DeepEqual(reasoning["supportedEfforts"], []any{"off", "low", "medium", "high"}) {
		t.Fatalf("model reasoning mismatch: %#v", reasoning)
	}
}

func TestApplyDefaultModelCapabilityOmitsInvalidReasoningMetadata(t *testing.T) {
	for _, testCase := range []ModelCapabilityConfig{
		{ReasoningProtocol: "mimo-chat-completions", ReasoningSupportedEfforts: nil, ReasoningDefaultEffort: "high"},
		{ReasoningProtocol: "mimo-chat-completions", ReasoningSupportedEfforts: []string{"off", "high"}, ReasoningDefaultEffort: "medium"},
		{ReasoningProtocol: "mimo-chat-completions", ReasoningSupportedEfforts: []string{"off", " high "}, ReasoningDefaultEffort: "high"},
		{ReasoningProtocol: "mimo-chat-completions", ReasoningSupportedEfforts: []string{"off", "high"}, ReasoningDefaultEffort: "HIGH"},
		{ReasoningProtocol: "none", ReasoningSupportedEfforts: []string{"auto"}, ReasoningDefaultEffort: "auto"},
	} {
		capabilities := map[string]any{"model": map[string]any{"reasoning": map[string]any{"supportedEfforts": []any{"off", "high", "max"}}}}
		ApplyDefaultModelCapability(capabilities, testCase)
		model := capabilities["model"].(map[string]any)
		if _, exists := model["reasoning"]; exists {
			t.Fatalf("invalid reasoning metadata must fail closed: %#v", model["reasoning"])
		}
	}
}

func TestApplyDefaultModelCapabilityOmitsImplicitNonDeepSeekReasoning(t *testing.T) {
	capabilities := map[string]any{
		"model": map[string]any{
			"reasoning": map[string]any{
				"supportedEfforts": []any{"off", "high", "max"},
				"defaultEffort":    "max",
				"requestProtocol":  "deepseek-chat-completions",
			},
		},
	}
	ApplyDefaultModelCapability(capabilities, ModelCapabilityConfig{
		Model:          "claude-sonnet-4",
		ProviderID:     "anthropic",
		Family:         "anthropic",
		EndpointFormat: "messages",
		InputModalities: []string{
			"text",
		},
		MessageParts: []string{"text"},
	})
	model, _ := capabilities["model"].(map[string]any)
	if _, exists := model["reasoning"]; exists {
		t.Fatalf("expected unsupported reasoning metadata to be omitted: %#v", model["reasoning"])
	}
}

func TestMCPCapabilityConfiguredButDisconnectedRemainsEnabled(t *testing.T) {
	state := MCPCapabilityState(DefaultMCPSearchConfig(), map[string]any{
		"available": false, "configuredServerCount": 1, "connectedServerCount": 0,
	})
	if state["enabled"] != true || state["available"] != false || state["status"] != "unavailable" {
		t.Fatalf("configured disconnected MCP must be enabled but unavailable: %#v", state)
	}
}

func TestDefaultReasoningProtocolRequiresExplicitHostProfile(t *testing.T) {
	for _, family := range []string{"deepseek", "openai-compatible"} {
		if got := DefaultReasoningProtocol(ModelCapabilityConfig{Family: family}); got != "none" {
			t.Fatalf("family %q implicitly enabled protocol %q", family, got)
		}
	}
	if got := DefaultReasoningProtocol(ModelCapabilityConfig{
		Family: "openai-compatible", ReasoningProtocol: "deepseek-chat-completions",
	}); got != "deepseek-chat-completions" {
		t.Fatalf("explicit host profile was not projected: %q", got)
	}
}

func TestMCPCapabilityStateProjectsSearchAndCatalog(t *testing.T) {
	config := MCPSearchConfig{
		Enabled:                true,
		Mode:                   "auto",
		AutoThresholdToolCount: 3,
		TopKDefault:            2,
		TopKMax:                4,
		MinScore:               0.25,
	}
	state := MCPCapabilityState(config, map[string]any{
		"available":             true,
		"configuredServerCount": 2,
		"connectedServerCount":  1,
		"indexedToolCount":      5,
		"advertisedToolCount":   4,
		"promptCount":           3,
		"resourceCount":         2,
		"catalogFingerprint":    "fp-1",
		"catalogDrift":          true,
		"status":                "available",
	})
	if state["available"] != true || state["status"] != "available" || state["toolCount"] != float64(5) {
		t.Fatalf("mcp state mismatch: %#v", state)
	}
	search, _ := state["search"].(map[string]any)
	if search["active"] != true || search["topKDefault"] != float64(2) || search["catalogFingerprint"] != "fp-1" {
		t.Fatalf("mcp search diagnostics mismatch: %#v", search)
	}
	catalog, _ := state["catalog"].(map[string]any)
	if catalog["toolCount"] != float64(5) || catalog["advertisedToolCount"] != float64(4) || catalog["catalogDrift"] != true {
		t.Fatalf("mcp catalog diagnostics mismatch: %#v", catalog)
	}
}

func TestMCPSearchDiagnosticsRemainDisabledWhenCatalogIsAvailable(t *testing.T) {
	config := DefaultMCPSearchConfig()
	diagnostics := MCPSearchDiagnostics(config, map[string]any{
		"available":           true,
		"indexedToolCount":    5,
		"advertisedToolCount": 4,
		"status":              "available",
	})
	if diagnostics["enabled"] != false || diagnostics["available"] != false || diagnostics["active"] != false {
		t.Fatalf("disabled MCP search must remain unavailable and inactive with a live catalog: %#v", diagnostics)
	}
	if diagnostics["indexedToolCount"] != float64(5) || diagnostics["advertisedToolCount"] != float64(4) {
		t.Fatalf("disabled MCP search diagnostics should preserve non-authorizing catalog counts: %#v", diagnostics)
	}
}

func TestMergeMCPSearchConfigAndNestedMaps(t *testing.T) {
	config := DefaultMCPSearchConfig()
	document := map[string]any{
		"mcpSearch": map[string]any{
			"enabled":                true,
			"mode":                   "search",
			"autoThresholdToolCount": float64(40),
			"topKDefault":            float64(12),
			"topKMax":                float64(3),
			"minScore":               float64(0.42),
		},
		"capabilities": map[string]any{
			"mcp": map[string]any{
				"search": map[string]any{
					"mode":        "auto",
					"topKDefault": float64(6),
					"topKMax":     float64(9),
				},
			},
		},
	}
	if search, ok := MCPSearchConfigMap(document, "mcpSearch"); ok {
		config = MergeMCPSearchConfig(config, search)
	} else {
		t.Fatal("expected top-level mcpSearch config")
	}
	if search, ok := MCPNestedConfigMap(document, "search"); ok {
		config = MergeMCPSearchConfig(config, search)
	} else {
		t.Fatal("expected nested mcp search config")
	}
	if !config.Enabled || config.Mode != "auto" || config.AutoThresholdToolCount != 40 || config.TopKDefault != 6 || config.TopKMax != 9 || config.MinScore != 0.42 {
		t.Fatalf("merged MCP search config mismatch: %#v", config)
	}

	clamped := MergeMCPSearchConfig(DefaultMCPSearchConfig(), map[string]any{"topKDefault": float64(12), "topKMax": float64(3)})
	if clamped.TopKMax != 12 {
		t.Fatalf("topKMax should clamp to topKDefault, got %#v", clamped)
	}
}

func TestMCPSearchShouldScopeAndResultLimit(t *testing.T) {
	config := MCPSearchConfig{Enabled: true, Mode: "auto", AutoThresholdToolCount: 3, TopKDefault: 8, TopKMax: 5}
	if !MCPSearchShouldScope(config, 3, true) {
		t.Fatalf("expected auto search to scope at threshold")
	}
	if MCPSearchShouldScope(config, 2, true) {
		t.Fatalf("did not expect auto search below threshold")
	}
	if MCPSearchShouldScope(config, 3, false) {
		t.Fatalf("did not expect search without prompt")
	}
	if MCPSearchShouldScope(MCPSearchConfig{Enabled: true, Mode: "direct"}, 10, true) {
		t.Fatalf("did not expect direct mode search")
	}
	if MCPSearchShouldScope(MCPSearchConfig{Enabled: false, Mode: "search"}, 10, true) {
		t.Fatalf("did not expect disabled search mode")
	}
	if MCPSearchShouldScope(MCPSearchConfig{Enabled: false, Mode: "auto", AutoThresholdToolCount: 1}, 10, true) {
		t.Fatalf("did not expect disabled auto mode to execute search")
	}
	if got := MCPSearchResultLimit(config); got != 5 {
		t.Fatalf("expected clamped search limit 5, got %d", got)
	}
	if got := MCPSearchResultLimit(MCPSearchConfig{}); got != 5 {
		t.Fatalf("expected default search limit 5, got %d", got)
	}
}

func TestComputerUseCapabilityRequiresStateAndActionTools(t *testing.T) {
	state := ComputerUseCapabilityState([]any{map[string]any{
		"id":        "analytix-computer-use",
		"enabled":   true,
		"available": true,
		"toolCount": float64(2),
		"toolNames": []any{
			"mcp__analytix-computer-use__get_app_state",
			"mcp__analytix-computer-use__click",
		},
	}})
	if state["available"] != true || state["reason"] != "Analytix Computer Use MCP backend is connected" {
		t.Fatalf("computer use state mismatch: %#v", state)
	}
	missingAction := ComputerUseCapabilityState([]any{map[string]any{
		"id":        "analytix-computer-use",
		"enabled":   true,
		"available": true,
		"toolNames": []any{"mcp__analytix-computer-use__get_app_state"},
	}})
	if missingAction["available"] != false || missingAction["reason"] != "computer-use MCP backend is missing required tools" {
		t.Fatalf("missing action state mismatch: %#v", missingAction)
	}
}

func TestVisionBridgeCapabilityStateRedactsReadiness(t *testing.T) {
	config := VisionBridgeConfig{
		Enabled:                   true,
		Mode:                      "auto",
		ProviderID:                "openai",
		BaseURL:                   "http://127.0.0.1:8787/v1",
		Model:                     "gpt-4.1-mini",
		SemanticProbeStatus:       "supported",
		DefaultScreenshotsPerTurn: 4,
		DefaultMaxImageBytes:      5 * 1024 * 1024,
	}
	state := VisionBridgeCapabilityState(config)
	if state["status"] != "available" || state["available"] != true || state["reason"] != "vision bridge is available" {
		t.Fatalf("vision bridge state mismatch: %#v", state)
	}
	if state["providerId"] != "openai" || state["model"] != "gpt-4.1-mini" || state["maxScreenshotsPerTurn"] != float64(4) {
		t.Fatalf("vision bridge metadata mismatch: %#v", state)
	}
	config.SemanticProbeStatus = "unknown"
	state = VisionBridgeCapabilityState(config)
	if state["available"] != false || state["reason"] != "vision bridge semantic probe has not passed" {
		t.Fatalf("vision bridge unsupported state mismatch: %#v", state)
	}
}
