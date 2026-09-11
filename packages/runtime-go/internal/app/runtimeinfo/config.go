package runtimeinfo

func LoadSandboxConfig(document map[string]any, base SandboxConfig) SandboxConfig {
	if sandbox, ok := SandboxConfigMap(document); ok {
		return MergeSandboxConfig(base, sandbox)
	}
	return base
}

func LoadWebConfig(document map[string]any) WebConfig {
	settings := DefaultWebConfig()
	raw, ok := WebConfigMap(document)
	if !ok {
		return settings
	}
	return MergeWebConfig(settings, raw)
}

func LoadVisionBridgeConfig(document map[string]any) VisionBridgeConfig {
	settings := DefaultVisionBridgeConfig()
	raw, ok := VisionBridgeConfigMap(document)
	if !ok {
		return settings
	}
	return MergeVisionBridgeConfig(settings, raw)
}

func LoadMCPSearchConfig(document map[string]any, base MCPSearchConfig) MCPSearchConfig {
	settings := base
	if search, ok := MCPSearchConfigMap(document, "mcpSearch"); ok {
		settings = MergeMCPSearchConfig(settings, search)
	}
	if search, ok := MCPNestedConfigMap(document, "search"); ok {
		settings = MergeMCPSearchConfig(settings, search)
	}
	return settings
}

func LoadMCPSearchConfigFromDocument(document map[string]any, ok bool) MCPSearchConfig {
	settings := DefaultMCPSearchConfig()
	if !ok {
		return settings
	}
	return LoadMCPSearchConfig(document, settings)
}
