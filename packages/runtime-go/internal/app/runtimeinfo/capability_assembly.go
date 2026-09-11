package runtimeinfo

type RuntimeCapabilitiesInput struct {
	DefaultModel ModelCapabilityConfig
	MCP          map[string]any
	Skills       map[string]any
	Subagents    map[string]any
	Web          map[string]any
	VisionBridge map[string]any
	ComputerUse  map[string]any
	Attachments  map[string]any
	Memory       map[string]any
}

func RuntimeCapabilities(input RuntimeCapabilitiesInput) map[string]any {
	capabilities := RuntimeCapabilitiesResponse()
	ApplyDefaultModelCapability(capabilities, input.DefaultModel)
	MergeCapabilityState(capabilities, "mcp", input.MCP)
	MergeCapabilityState(capabilities, "skills", input.Skills)
	MergeCapabilityState(capabilities, "subagents", input.Subagents)
	MergeCapabilityState(capabilities, "web", input.Web)
	MergeCapabilityState(capabilities, "visionBridge", input.VisionBridge)
	MergeCapabilityState(capabilities, "computerUse", input.ComputerUse)
	MergeCapabilityState(capabilities, "attachments", input.Attachments)
	MergeCapabilityState(capabilities, "memory", input.Memory)
	return capabilities
}

func MergeCapabilityState(capabilities map[string]any, name string, state map[string]any) {
	if len(state) == 0 {
		return
	}
	target, ok := capabilities[name].(map[string]any)
	if !ok {
		return
	}
	for key, value := range state {
		target[key] = value
	}
}
