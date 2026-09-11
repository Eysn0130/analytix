package thread

import "testing"

func TestBuildRecoveredStateProjectsReplayDiagnostics(t *testing.T) {
	state := BuildRecoveredState(RecoveredStateInput{
		ThreadID: "thr_1",
		Diagnostics: []any{
			map[string]any{"line": float64(2), "message": "bad json"},
		},
		Events: []map[string]any{
			{"kind": "turn_started", "seq": float64(1)},
			{"kind": "usage", "seq": float64(2), "usage": map[string]any{"cacheHitTokens": float64(3)}, "cacheDiagnostics": map[string]any{"provider": "openai", "cacheMissTokens": float64(5)}},
			{"kind": "usage", "seq": float64(3), "cacheDiagnostics": map[string]any{"providerId": "anthropic", "cacheHitTokens": float64(7), "cacheMissTokens": float64(11)}},
			{"kind": "approval_requested", "status": "pending", "seq": float64(4)},
			{"kind": "approval_resolved", "status": "denied", "seq": float64(5)},
			{"kind": "user_input_requested", "status": "pending", "seq": float64(6)},
			{"kind": "user_input_resolved", "status": "submitted", "seq": float64(7)},
			{"kind": "tool_catalog_changed", "fingerprint": "fp", "toolNames": []any{"write_file", "read_file"}, "seq": float64(8)},
		},
	})
	if state["threadId"] != "thr_1" || state["highestSeq"] != 8 {
		t.Fatalf("identity/highest seq mismatch: %#v", state)
	}
	usage, _ := state["usageCacheAccounting"].(map[string]any)
	if usage["totalCacheHitTokens"] != 10 || usage["totalCacheMissTokens"] != 16 {
		t.Fatalf("usage totals mismatch: %#v", usage)
	}
	providers, _ := usage["providers"].([]string)
	if len(providers) != 2 || providers[0] != "anthropic" || providers[1] != "openai" {
		t.Fatalf("providers not sorted: %#v", providers)
	}
	approvals, _ := state["approvals"].(map[string]any)
	if approvals["recoveredFromReplay"] != true {
		t.Fatalf("approval replay recovery mismatch: %#v", approvals)
	}
	userInputs, _ := state["userInputs"].(map[string]any)
	if userInputs["recoveredFromReplay"] != true {
		t.Fatalf("user input replay recovery mismatch: %#v", userInputs)
	}
	mcp, _ := state["mcp"].(map[string]any)
	names, _ := mcp["toolNames"].([]string)
	if mcp["latestFingerprint"] != "fp" || len(names) != 2 || names[0] != "read_file" || names[1] != "write_file" {
		t.Fatalf("mcp projection mismatch: %#v", mcp)
	}
}
