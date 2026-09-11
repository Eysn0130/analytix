package runtimeinfo

import "testing"

func TestRuntimeCapabilitiesAssemblesModelAndRuntimeStates(t *testing.T) {
	capabilities := RuntimeCapabilities(RuntimeCapabilitiesInput{
		DefaultModel: ModelCapabilityConfig{
			Model:              "mimo-v2.5",
			ProviderID:         "xiaomi",
			Family:             "openai-compatible",
			EndpointFormat:     "chat_completions",
			InputModalities:    []string{"text", "image"},
			MessageParts:       []string{"text", "image_url"},
			SupportsImageInput: true,
			ReasoningEffort:    "high",
			ReasoningProtocol:  "mimo-chat-completions",
		},
		MCP:          map[string]any{"status": "available", "toolCount": float64(2)},
		Skills:       map[string]any{"enabled": true, "discoveredSkills": float64(3)},
		Subagents:    map[string]any{"available": true, "maxParallel": float64(4)},
		Web:          map[string]any{"fetchEnabled": true, "provider": "fetch"},
		VisionBridge: map[string]any{"enabled": true, "mode": "always"},
		ComputerUse:  map[string]any{"available": true, "mode": "mcp"},
		Attachments:  map[string]any{"enabled": true, "maxImageBytes": float64(1024)},
		Memory:       map[string]any{"enabled": true, "activeCount": float64(2)},
	})
	model, _ := capabilities["model"].(map[string]any)
	if model["id"] != "mimo-v2.5" || model["providerId"] != "xiaomi" || model["supportsImageInput"] != true {
		t.Fatalf("model capability mismatch: %#v", model)
	}
	for _, tc := range []struct {
		name string
		key  string
		want any
	}{
		{"mcp", "toolCount", float64(2)},
		{"skills", "discoveredSkills", float64(3)},
		{"subagents", "maxParallel", float64(4)},
		{"web", "provider", "fetch"},
		{"visionBridge", "mode", "always"},
		{"computerUse", "mode", "mcp"},
		{"attachments", "maxImageBytes", float64(1024)},
		{"memory", "activeCount", float64(2)},
	} {
		section, _ := capabilities[tc.name].(map[string]any)
		if section[tc.key] != tc.want {
			t.Fatalf("%s.%s = %#v, want %#v in %#v", tc.name, tc.key, section[tc.key], tc.want, section)
		}
	}
}

func TestRuntimeCapabilitiesLeavesDefaultStatesWhenInputIsEmpty(t *testing.T) {
	capabilities := RuntimeCapabilities(RuntimeCapabilitiesInput{})
	mcp, _ := capabilities["mcp"].(map[string]any)
	if mcp["status"] != "disabled" || mcp["toolCount"] != float64(0) {
		t.Fatalf("empty input should keep default MCP state: %#v", mcp)
	}
	catalog, _ := mcp["catalog"].(map[string]any)
	if catalog["status"] != "disabled" {
		t.Fatalf("empty input should keep default MCP catalog disabled: %#v", catalog)
	}
}
