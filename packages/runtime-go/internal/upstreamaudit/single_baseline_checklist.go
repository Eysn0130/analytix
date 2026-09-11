//go:build !analytix_prod

package upstreamaudit

func SingleBaselineChecklist() map[string]any {
	contractReplayDeleteCandidates := []map[string]any{
		{"id": "fixture-only-provider-evidence-superseded", "path": "packages/runtime-go/internal/provider/provider_contract_replay_handler.go", "condition": "covered by live production-candidate provider client tests"},
		{"id": "fixture-only-g4-manager-evidence-superseded", "path": "packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go", "condition": "covered by live production-candidate gate and MCP manager tests"},
	}
	postRuntimeReadinessDeleteCandidates := []map[string]any{
		{"id": "provider-live-evidence-route", "path": "/v1/internal/go-production-candidate/provider-live", "condition": "covered by runtime provider readiness matrix plus runtime turn replay"},
		{"id": "durable-replay-evidence-route", "path": "/v1/internal/go-production-candidate/durable-replay", "condition": "covered by candidate durable root crash/restart drill"},
		{"id": "approval-user-input-evidence-route", "path": "/v1/internal/go-production-candidate/approval-user-input", "condition": "covered by restart recovery of pending approval/user-input gates"},
		{"id": "mcp-manager-evidence-route", "path": "/v1/internal/go-production-candidate/mcp-manager", "condition": "covered by runtime MCP readiness matrix"},
		{"id": "job-lineage-evidence-route", "path": "/v1/internal/go-production-candidate/job-lineage", "condition": "covered by runtime replay and readiness gate"},
	}
	deleteAfterG6 := []map[string]any{
		{"id": "internal-production-candidate-gate", "path": "src/main/runtime/analytix-adapter.ts", "condition": "Go backend is default and packaged rollback drills pass"},
		{"id": "contract-sidecar-routes", "path": "packages/runtime-go/cmd/contract-sidecar", "condition": "production Go runtime routes replace conformance routes"},
		{"id": "typescript-default-runtime", "path": "packages/runtime", "condition": "G6 TS retirement migration lands"},
		{"id": "shadow-fixture-contracts", "path": "packages/runtime/src/conformance/fixtures/go-*.json", "condition": "live cross-backend release tests replace shadow replay"},
	}
	forbiddenNow := []map[string]any{
		{"id": "renderer-visible-go-switcher", "token": "renderer Go backend switcher", "present": false},
		{"id": "deprecated-bridge-alias", "token": "legacy bridge alias family", "present": false},
		{"id": "duplicate-settings-schema", "token": "legacy runtime-shaped settings save", "present": false},
		{"id": "reasonix-public-protocol", "token": "upstream session/config route family", "present": false},
		{"id": "top-level-hidden-capability-route", "token": "Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry", "present": false},
	}
	return map[string]any{
		"machineTestable":                      true,
		"contractReplayDeleteCandidates":       contractReplayDeleteCandidates,
		"postRuntimeReadinessDeleteCandidates": postRuntimeReadinessDeleteCandidates,
		"deleteAfterG6":                        deleteAfterG6,
		"forbiddenNow":                         forbiddenNow,
		"presentForbiddenRedundancyCount":      countPresentForbidden(forbiddenNow),
		"tsRuntimeDefaultRetained":             true,
		"defaultGoBackendEnabled":              false,
		"rendererPreloadMainContractChange":    false,
		"rendererVisibleGoSwitcher":            false,
		"runtimeReadinessDefaultReady":         false,
	}
}

func countPresentForbidden(items []map[string]any) int {
	count := 0
	for _, item := range items {
		if item["present"] == true {
			count++
		}
	}
	return count
}
