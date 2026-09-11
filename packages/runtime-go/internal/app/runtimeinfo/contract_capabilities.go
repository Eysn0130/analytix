package runtimeinfo

import (
	"strings"

	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
)

func RuntimeInfoResponse(startedAt string) PublicRuntimeInfoV2 {
	capabilities := ProjectPublicRuntimeCapabilities(RuntimeCapabilitiesResponse())
	capabilities.Attachments.AllowedMimeTypes = []string{"image/png", "image/jpeg", "image/webp"}
	capabilities.Attachments.AllowedDocumentMimeTypes = []string{"application/pdf", "text/plain", "text/markdown", "text/csv", "application/json"}
	return PublicRuntimeInfoV2{
		SchemaVersion: PublicRuntimeDiagnosticsSchemaVersion,
		Status:        "ready",
		ListenerScope: "loopback",
		Port:          0,
		StartedAt:     startedAt,
		Insecure:      false,
		Storage:       PublicStorageStateV2{Configured: true, Available: true},
		ExecutionPolicy: PublicExecutionPolicyV2{
			ApprovalPolicy: executionpolicy.DefaultApprovalPolicy,
			SandboxMode:    executionpolicy.DefaultSandboxMode,
		},
		Provider: PublicProviderStateV2{
			ID: "deepseek", Model: "deepseek-chat", Family: "deepseek", EndpointFormat: "chat_completions",
			Available: false, APIKeyConfigured: false, BaseURLConfigured: true, CacheTelemetrySupported: true,
			SupportsImageInput: false, ContextWindowTokens: 1000000,
		},
		NetworkProxy: PublicNetworkProxyStateV2{
			Mode: "auto", Configured: false, Source: "unknown", Valid: false, CredentialsMasked: false,
		},
		Capabilities: capabilities,
	}
}

func RuntimeCapabilitiesResponse() map[string]any {
	return map[string]any{
		"contractVersion": float64(1),
		"model": map[string]any{
			"id":                  "deepseek-chat",
			"inputModalities":     []any{"text"},
			"outputModalities":    []any{"text"},
			"supportsToolCalling": true,
			"contextWindowTokens": float64(1000000),
			"messageParts":        []any{"text"},
			"reasoning": map[string]any{
				"supportedEfforts": []any{"off", "high", "max"},
				"defaultEffort":    "max",
				"requestProtocol":  "deepseek-chat-completions",
			},
		},
		"cli": map[string]any{
			"serve": AvailableState(),
			"run":   UnavailableState("not implemented"),
			"chat":  UnavailableState("not implemented"),
			"exec":  UnavailableState("not implemented"),
		},
		"mcp": map[string]any{
			"status":            "disabled",
			"enabled":           false,
			"available":         false,
			"reason":            "MCP is disabled by config",
			"configuredServers": float64(0),
			"connectedServers":  float64(0),
			"toolCount":         float64(0),
			"promptCount":       float64(0),
			"resourceCount":     float64(0),
			"catalog": map[string]any{
				"status":        "disabled",
				"reason":        "MCP is disabled by config",
				"toolCount":     float64(0),
				"promptCount":   float64(0),
				"resourceCount": float64(0),
			},
			"search": map[string]any{
				"enabled":             false,
				"mode":                "auto",
				"active":              false,
				"indexedToolCount":    float64(0),
				"advertisedToolCount": float64(0),
			},
		},
		"web": map[string]any{
			"status":        "disabled",
			"enabled":       false,
			"available":     false,
			"reason":        "web access is disabled by config",
			"fetch":         DisabledState("web fetch is disabled by config"),
			"search":        DisabledState("web search is disabled by config"),
			"fetchEnabled":  false,
			"searchEnabled": false,
		},
		"skills": map[string]any{
			"status":           "disabled",
			"enabled":          false,
			"available":        false,
			"reason":           "Skills are disabled by config",
			"configuredRoots":  float64(0),
			"discoveredSkills": float64(0),
		},
		"subagents": map[string]any{
			"status":            "disabled",
			"enabled":           false,
			"available":         false,
			"reason":            "subagents are disabled by config",
			"maxParallel":       float64(0),
			"maxChildRuns":      float64(0),
			"defaultToolPolicy": "readOnly",
			"profiles":          []any{},
		},
		"attachments": map[string]any{
			"status":                        "disabled",
			"enabled":                       false,
			"available":                     false,
			"reason":                        "attachments are disabled by config",
			"maxImageBytes":                 float64(5242880),
			"maxImageDimension":             float64(4096),
			"allowedMimeTypes":              []any{"image/png", "image/jpeg", "image/webp"},
			"allowedDocumentMimeTypes":      []any{"application/pdf", "text/plain", "text/markdown", "text/csv", "application/json"},
			"maxDocumentBytes":              float64(10485760),
			"maxDocumentTextChars":          float64(200000),
			"textFallbackMaxBase64Bytes":    float64(524288),
			"textFallbackMaxImageDimension": float64(1280),
			"textFallbackPreferredMimeType": "image/webp",
		},
		"memory": map[string]any{
			"status":             "disabled",
			"enabled":            false,
			"available":          false,
			"reason":             "memory is disabled by config",
			"mode":               "manual",
			"storeOnly":          true,
			"modelInjection":     false,
			"automaticCapture":   false,
			"scopes":             []any{"user", "workspace", "project"},
			"maxInjectedRecords": float64(0),
		},
		"imageGen":  DisabledState("image generation is disabled by config"),
		"speechGen": DisabledState("speech generation is disabled by config"),
		"musicGen":  DisabledState("music generation is disabled by config"),
		"videoGen":  DisabledState("video generation is disabled by config"),
		"computerUse": map[string]any{
			"status":    "disabled",
			"enabled":   false,
			"available": false,
			"reason":    "computer use is disabled by config",
			"mode":      "auto",
		},
		"visionBridge": map[string]any{
			"status":                "disabled",
			"enabled":               false,
			"available":             false,
			"reason":                "vision bridge is disabled by config",
			"mode":                  "auto",
			"maxScreenshotsPerTurn": float64(4),
			"maxImagesPerTurn":      float64(4),
			"maxImageBytes":         float64(1500000),
			"maxImageDimension":     float64(1280),
			"semanticProbeStatus":   "unknown",
		},
	}
}

func RuntimeServerContractCapabilities() map[string]any {
	return RuntimeCapabilitiesResponse()
}

func RuntimeToolsResponse() PublicRuntimeToolsV2 {
	return PublicRuntimeToolsV2{
		SchemaVersion: PublicRuntimeDiagnosticsSchemaVersion,
		ProviderCount: 0,
		ToolContracts: PublicToolContractCatalogV2{Count: 0, CatalogHash: strings.Repeat("a", 64)},
		MCPServers:    []PublicMCPServerDiagnosticV2{},
		MCPSearch: PublicMCPSearchV2{
			Enabled: false, Mode: "auto", Active: false, Available: false, ReasonCode: "disabled_by_config",
			IndexedToolCount: 0, AdvertisedToolCount: 0, AutoThresholdToolCount: 0, TopKDefault: 0, TopKMax: 0,
			MinScore: 0, CatalogDrift: false,
		},
		MCPPromptCount:   0,
		MCPResourceCount: 0,
		Commands:         []PublicCommandDiagnosticV2{},
		NetworkProxy: PublicNetworkProxyStateV2{
			Mode: "off", Configured: false, Source: "unknown", Valid: true, CredentialsMasked: true,
		},
		WebProviderCount: 0,
		Skills: PublicSkillDiagnosticsV2{
			Enabled: false, Available: false, ReasonCode: "disabled_by_config",
			ConfiguredRootCount: 0, SkillCount: 0, ValidationErrorCount: 0,
		},
		Attachments: PublicAttachmentDiagnosticsV2{
			Enabled: false, Count: 0, TotalBytes: 0, MaxImageBytes: 0, MaxImageDimension: 0,
			AllowedMimeTypes: []string{}, AllowedDocumentMimeTypes: []string{}, MaxDocumentBytes: 0, MaxDocumentTextChars: 0,
		},
		Memory: PublicMemoryDiagnosticsV2{
			Enabled: false, Status: "unavailable", ReasonCode: "memory_store_unavailable",
		},
		Subagents: PublicSubagentDiagnosticsV2{
			Status: "disabled", Enabled: false, Available: false, ReasonCode: "disabled_by_config",
			Active: 0, Queued: 0, ProfileCount: 0, MaxParallel: 0, MaxChildRuns: 0, DefaultToolPolicy: "readOnly",
			InternalLineageAvailable: false, ParallelExecutionAvailable: false, TaskToolAvailable: false,
			ParallelTasksToolAvailable: false, BackgroundTaskJobsAvailable: false, BackgroundShellAvailable: false,
			BackgroundSubagentJobsAvailable: false,
		},
	}
}

func AvailableState() map[string]any {
	return map[string]any{
		"status":    "available",
		"enabled":   true,
		"available": true,
	}
}

func UnavailableState(reason string) map[string]any {
	return map[string]any{
		"status":    "unavailable",
		"enabled":   false,
		"available": false,
		"reason":    reason,
	}
}

func DisabledState(reason string) map[string]any {
	return map[string]any{
		"status":    "disabled",
		"enabled":   false,
		"available": false,
		"reason":    reason,
	}
}
