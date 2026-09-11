package runtimeinfo

import "testing"

func TestRuntimeInfoConfigLoadersKeepDefaultsAndMergeDocuments(t *testing.T) {
	document := map[string]any{
		"capabilities": map[string]any{
			"mcp": map[string]any{
				"sandbox": map[string]any{
					"allow_write": []any{"/workspace"},
				},
				"search": map[string]any{
					"enabled":     true,
					"topKDefault": float64(7),
				},
			},
			"web": map[string]any{
				"enabled":       true,
				"fetch_enabled": true,
				"provider":      "mcp-web",
			},
			"visionBridge": map[string]any{
				"enabled": true,
				"mode":    "always",
				"model":   "vision-model",
			},
		},
	}

	sandbox := LoadSandboxConfig(document, SandboxConfig{ProtectedReadDirs: []string{"/protected"}})
	if len(sandbox.AllowWriteRoots) != 1 || sandbox.AllowWriteRoots[0] != "/workspace" || len(sandbox.ProtectedReadDirs) != 1 {
		t.Fatalf("sandbox config mismatch: %#v", sandbox)
	}
	web := LoadWebConfig(document)
	if !web.Enabled || !web.FetchEnabled || web.Provider != "mcp-web" || web.MaxFetchBytes != DefaultWebMaxFetchBytes {
		t.Fatalf("web config mismatch: %#v", web)
	}
	vision := LoadVisionBridgeConfig(document)
	if !vision.Enabled || vision.Mode != "always" || vision.Model != "vision-model" || vision.EndpointFormat != "chat_completions" {
		t.Fatalf("vision config mismatch: %#v", vision)
	}
	search := LoadMCPSearchConfig(document, DefaultMCPSearchConfig())
	if !search.Enabled || search.TopKDefault != 7 || search.TopKMax < search.TopKDefault {
		t.Fatalf("mcp search config mismatch: %#v", search)
	}
	fromDocument := LoadMCPSearchConfigFromDocument(document, true)
	if !fromDocument.Enabled || fromDocument.TopKDefault != 7 {
		t.Fatalf("mcp search from document mismatch: %#v", fromDocument)
	}
	defaultSearch := LoadMCPSearchConfigFromDocument(document, false)
	if defaultSearch.Enabled || defaultSearch.TopKDefault != DefaultMCPSearchConfig().TopKDefault {
		t.Fatalf("mcp search should use defaults when config is absent: %#v", defaultSearch)
	}
}
