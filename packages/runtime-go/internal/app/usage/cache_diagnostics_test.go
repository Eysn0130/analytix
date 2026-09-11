package usage

import "testing"

func TestLatestCacheDiagnosticsUsesLatestMatchingUsageEvent(t *testing.T) {
	older := map[string]any{"prefix": "old"}
	latest := map[string]any{"prefix": "latest"}
	otherTurn := map[string]any{"prefix": "other"}
	events := []map[string]any{
		{"kind": "usage", "turnId": "turn-1", "cacheDiagnostics": older},
		{"kind": "pipeline_stage", "turnId": "turn-1", "cacheDiagnostics": map[string]any{"prefix": "ignored"}},
		{"kind": "usage", "turnId": "turn-2", "cacheDiagnostics": otherTurn},
		{"kind": "usage", "turnId": "turn-1", "cacheDiagnostics": latest},
	}
	got := LatestCacheDiagnostics(events, "turn-1")
	if got["prefix"] != "latest" {
		t.Fatalf("latest diagnostics mismatch: %#v", got)
	}
	latest["prefix"] = "mutated"
	if got["prefix"] != "latest" {
		t.Fatalf("diagnostics should be cloned: %#v", got)
	}
}

func TestLatestCacheDiagnosticsCanSearchAllTurns(t *testing.T) {
	events := []map[string]any{
		{"kind": "usage", "turnId": "turn-1", "cacheDiagnostics": map[string]any{"prefix": "old"}},
		{"kind": "usage", "turnId": "turn-2", "cacheDiagnostics": map[string]any{"prefix": "latest"}},
	}
	got := LatestCacheDiagnostics(events, "")
	if got["prefix"] != "latest" {
		t.Fatalf("all-turn diagnostics mismatch: %#v", got)
	}
	if got := LatestCacheDiagnostics(events, "missing"); got != nil {
		t.Fatalf("missing turn should not return diagnostics: %#v", got)
	}
}
