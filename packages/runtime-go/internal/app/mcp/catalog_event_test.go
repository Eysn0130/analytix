package mcp

import "testing"

func TestBuildCatalogChangedEventRequiresThreadDriftAndFingerprint(t *testing.T) {
	if event, ok := BuildCatalogChangedEvent(CatalogChangedEventInput{}); ok || event != nil {
		t.Fatalf("empty input should not build event: %#v", event)
	}
	if event, ok := BuildCatalogChangedEvent(CatalogChangedEventInput{
		ThreadID:    "thread-a",
		Diagnostics: map[string]any{"catalogDrift": true},
	}); ok || event != nil {
		t.Fatalf("missing fingerprint should not build event: %#v", event)
	}
}

func TestBuildCatalogChangedEventUsesAdvertisedCountAndSortedTools(t *testing.T) {
	event, ok := BuildCatalogChangedEvent(CatalogChangedEventInput{
		ThreadID: " thread-a ",
		Diagnostics: map[string]any{
			"catalogDrift":        true,
			"catalogFingerprint":  "fp-1",
			"advertisedToolCount": float64(3),
			"indexedToolCount":    float64(9),
		},
		ToolNames: []string{"z_tool", "a_tool"},
	})
	if !ok {
		t.Fatalf("expected event")
	}
	if event["threadId"] != "thread-a" || event["toolCount"] != 3 {
		t.Fatalf("event identity mismatch: %#v", event)
	}
	tools, _ := event["toolNames"].([]any)
	if len(tools) != 2 || tools[0] != "a_tool" || tools[1] != "z_tool" {
		t.Fatalf("tools should be sorted: %#v", event)
	}
}

func TestBuildCatalogChangedEventFallsBackToIndexedCount(t *testing.T) {
	event, ok := BuildCatalogChangedEvent(CatalogChangedEventInput{
		ThreadID: "thread-a",
		Diagnostics: map[string]any{
			"catalogDrift":       true,
			"catalogFingerprint": "fp-1",
			"indexedToolCount":   float64(7),
		},
	})
	if !ok {
		t.Fatalf("expected event")
	}
	if event["toolCount"] != 7 {
		t.Fatalf("indexed tool count mismatch: %#v", event)
	}
}
