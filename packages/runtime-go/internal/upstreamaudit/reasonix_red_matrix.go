//go:build !analytix_prod

package upstreamaudit

import "sort"

const (
	ReasonixSourcePath   = "/Users/sun/Projects/_upstreams/DeepSeek-Reasonix"
	ReasonixSourceCommit = "19d532d452e99c54e3c62c918d8469f06ea252ce"
)

type RuntimeAbsorptionClass string

const (
	AbsorptionReplace             RuntimeAbsorptionClass = "replace"
	AbsorptionCodePortAndAdapt    RuntimeAbsorptionClass = "code-port-and-adapt"
	AbsorptionContractReimplement RuntimeAbsorptionClass = "contract-reimplement"
	AbsorptionReject              RuntimeAbsorptionClass = "reject"
	AbsorptionDefer               RuntimeAbsorptionClass = "defer"
)

type RuntimeAbsorptionStatus string

const (
	AbsorptionStatusGreen    RuntimeAbsorptionStatus = "green"
	AbsorptionStatusRed      RuntimeAbsorptionStatus = "red"
	AbsorptionStatusRejected RuntimeAbsorptionStatus = "rejected"
	AbsorptionStatusDeferred RuntimeAbsorptionStatus = "deferred"
)

type RuntimeMachineCheck struct {
	ID                      string   `json:"id"`
	Kind                    string   `json:"kind"`
	Status                  string   `json:"status"`
	Evidence                string   `json:"evidence,omitempty"`
	ExpectedBlocked         *bool    `json:"expectedBlocked,omitempty"`
	AcceptedAsCodePhasePass *bool    `json:"acceptedAsCodePhasePass,omitempty"`
	DefaultBackendReady     *bool    `json:"defaultBackendReady,omitempty"`
	BlockerIDs              []string `json:"blockerIds,omitempty"`
}

type ReasonixCapabilityAuditRow struct {
	ID                           string                  `json:"id"`
	Capability                   string                  `json:"capability"`
	ReasonixSources              []string                `json:"reasonixSources"`
	AnalytixLanding              string                  `json:"analytixLanding"`
	AbsorptionClass              RuntimeAbsorptionClass  `json:"absorptionClass"`
	Status                       RuntimeAbsorptionStatus `json:"status"`
	MachineChecks                []RuntimeMachineCheck   `json:"machineChecks"`
	AnalytixEvidence             []string                `json:"analytixEvidence"`
	Blockers                     []string                `json:"blockers,omitempty"`
	ReplacesAnalytixWeakness     string                  `json:"replacesAnalytixWeakness,omitempty"`
	DeleteCandidates             []string                `json:"deleteCandidates,omitempty"`
	UsesReasonixPublicProtocol   bool                    `json:"usesReasonixPublicProtocol"`
	UsesReasonixConfigRoot       bool                    `json:"usesReasonixConfigRoot"`
	ChangesRendererContract      bool                    `json:"changesRendererContract"`
	ChangesProductIdentity       bool                    `json:"changesProductIdentity"`
	RequiresKunProductEntryDrift bool                    `json:"requiresKunProductEntryDrift"`
}

type ReasonixCapabilityAuditMatrix struct {
	SchemaVersion                   int                          `json:"schemaVersion"`
	ChangeID                        string                       `json:"changeId"`
	SourcePath                      string                       `json:"sourcePath"`
	SourceCommit                    string                       `json:"sourceCommit"`
	RuntimeContract                 string                       `json:"runtimeContract"`
	ReadyForG6                      bool                         `json:"readyForG6"`
	CapabilityMatrixGreen           bool                         `json:"capabilityMatrixGreen"`
	DefaultBackendReady             bool                         `json:"defaultBackendReady"`
	DefaultBackendReadinessGate     string                       `json:"defaultBackendReadinessGate"`
	Rows                            []ReasonixCapabilityAuditRow `json:"rows"`
	GreenCount                      int                          `json:"greenCount"`
	RedCount                        int                          `json:"redCount"`
	RejectedCount                   int                          `json:"rejectedCount"`
	DeferredCount                   int                          `json:"deferredCount"`
	ForbiddenPublicProtocolRowCount int                          `json:"forbiddenPublicProtocolRowCount"`
	RendererContractChangedRowCount int                          `json:"rendererContractChangedRowCount"`
	ProductIdentityChangedRowCount  int                          `json:"productIdentityChangedRowCount"`
	KunProductEntryDriftRowCount    int                          `json:"kunProductEntryDriftRowCount"`
	PostG6DeleteCandidates          []string                     `json:"postG6DeleteCandidates"`
	G6Blockers                      []string                     `json:"g6Blockers"`
	ReadinessSemantics              []string                     `json:"readinessSemantics"`
	Notes                           []string                     `json:"notes"`
}

func BuildReasonixCapabilityAuditMatrix() ReasonixCapabilityAuditMatrix {
	rows := []ReasonixCapabilityAuditRow{
		{
			ID:              "provider-cache-streaming",
			Capability:      "Provider registry, request-shape discipline, streaming parser, usage/cache telemetry.",
			ReasonixSources: []string{"internal/provider", "internal/provider/openai", "internal/provider/anthropic", "internal/agent/agent.go"},
			AnalytixLanding: "Go runtime provider client behind analytix turn/SSE contract.",
			AbsorptionClass: AbsorptionCodePortAndAdapt,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("provider-runtime-contract-go", "test", "packages/runtime-go/live_production_candidate_test.go"),
				PassedCheck("runtime-turn-sse-usage-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("provider-readiness-matrix-contract", "benchmark", "packages/runtime-go/g6_readiness_test.go"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/readiness/readiness.go", "packages/runtime-go/live_production_candidate.go", "packages/runtime-go/runtime_server.go"},
			ReplacesAnalytixWeakness: "Provider/cache evidence is now exercised by the Go runtime turn path instead of a disconnected fixture-only route.",
		},
		{
			ID:              "multi-model-non-regression",
			Capability:      "Kun/Analytix multi-model provider baseline under Reasonix DeepSeek capability absorption.",
			ReasonixSources: []string{"internal/provider", "internal/agent/agent.go", "docs/index.html"},
			AnalytixLanding: "Go runtime provider turn config and full-function baseline guard.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-multi-model-matrix-go", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("runtime-server-multi-model-turns", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("runtime-conformance-multi-model", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/upstreamaudit/baseline_absorption.go", "packages/runtime-go/runtime_server.go", "packages/runtime/src/contracts/capabilities.ts"},
			ReplacesAnalytixWeakness: "Go runtime turn/server no longer hardcodes DeepSeek; DeepSeek cache/prefix is provider-specific and does not narrow OpenAI-compatible, Anthropic-compatible, or custom_endpoint providers.",
		},
		{
			ID:              "deepseek-prefix-cache",
			Capability:      "DeepSeek cache-first prefix stability with prefix-shape diagnostics.",
			ReasonixSources: []string{"internal/agent/agent.go", "docs/SPEC.md"},
			AnalytixLanding: "Provider-family-aware cache diagnostics on runtime-server usage events.",
			AbsorptionClass: AbsorptionCodePortAndAdapt,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("prefix-shape-go-contract", "test", "packages/runtime-go/live_production_candidate_test.go"),
				PassedCheck("runtime-cache-diagnostics-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("deepseek-cache-record-format-contract", "benchmark", "packages/runtime-go/g6_readiness_test.go"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/contracts/hash.go", "packages/runtime-go/live_production_candidate.go", "packages/runtime-go/runtime_server.go"},
			ReplacesAnalytixWeakness: "Cache accounting is attached to analytix runtime events and no longer only summarized in docs.",
		},
		{
			ID:              "context-contract-maintenance",
			Capability:      "Provider-visible tool contract snapshot, command-environment diagnostics, and stale tool-result snip hygiene.",
			ReasonixSources: []string{"internal/tool/contract.go", "internal/environment/probe.go", "internal/agent/prune.go", "docs/TOOL_CONTRACT.md", "docs/SPEC.md"},
			AnalytixLanding: "Analytix-owned tool contract diagnostics on /v1/runtime/tools plus runtime command probes and snipHint-driven request-history hygiene.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("tool-contract-diagnostics-ts", "test", "packages/runtime/tests/capability-registry.test.ts"),
				PassedCheck("go-runtime-tool-contract-surface", "test", "packages/runtime-go/internal/server/mcp_tool_schema_test.go"),
				PassedCheck("runtime-tools-contract-diagnostics", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("runtime-command-probe-boundary", "test", "packages/runtime/tests/runtime-factory.test.ts"),
				PassedCheck("go-runtime-command-diagnostics", "test", "packages/runtime-go/internal/server/mcp_tool_schema_test.go"),
				PassedCheck("runtime-command-diagnostics-conformance", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("request-history-snip-hygiene", "test", "packages/runtime/tests/request-history-hygiene.test.ts"),
			},
			AnalytixEvidence: []string{
				"packages/runtime/src/ports/tool-host.ts",
				"packages/runtime/src/adapters/tool/capability-registry.ts",
				"packages/runtime/src/server/runtime-factory.ts",
				"packages/runtime-go/internal/server/runtime_components.go",
				"packages/runtime-go/internal/server/mcp_tool_schema_test.go",
				"packages/runtime/src/loop/request-history-hygiene.ts",
				"packages/runtime/src/environment/command-probe.ts",
			},
			ReplacesAnalytixWeakness: "Runtime diagnostics now expose the same canonical provider-visible tool surface used for model requests without leaking snip hints or upstream protocols.",
		},
		{
			ID:              "approval-user-input-gates",
			Capability:      "Approval and user-input gates with deny no-execute, answer-free replay, and restart recovery.",
			ReasonixSources: []string{"internal/permission", "internal/control", "docs/SPEC.md"},
			AnalytixLanding: "Analytix approval/user-input HTTP routes and SSE events.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("approval-user-input-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("restart-restores-pending-gates-contract", "test", "packages/runtime-go/runtime_server_test.go"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/agent/gates.go", "packages/runtime-go/runtime_server.go", "packages/runtime-go/runtime_server_test.go"},
			ReplacesAnalytixWeakness: "Pending gates are recovered from durable runtime events after a Go candidate restart.",
		},
		{
			ID:              "mcp-lifecycle-search-call",
			Capability:      "MCP connect/search/call/reconnect and approval/redaction discipline.",
			ReasonixSources: []string{"internal/plugin", "internal/mcpdiag", "docs/SPEC.md"},
			AnalytixLanding: "Analytix-owned live-local MCP lifecycle audit under the runtime event contract.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-mcp-contract-replay", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("mcp-readiness-matrix-contract", "test", "packages/runtime-go/g6_readiness_test.go"),
				PassedCheck("mcp-lifecycle-contract-go", "test", "packages/runtime-go/mcp_lifecycle_contract_test.go"),
				PassedCheck("mcp-lifecycle-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/mcp/lifecycle.go", "packages/runtime-go/runtime_server.go", "packages/runtime/src/contracts/events.ts"},
			ReplacesAnalytixWeakness: "MCP lifecycle/search/call/reconnect/redaction is now emitted through the Go runtime server contract instead of remaining contract-replay-only or env-skipped.",
		},
		{
			ID:              "job-subagent-lineage",
			Capability:      "Task/background/sub-agent lineage bound to parent goal/thread state.",
			ReasonixSources: []string{"internal/jobs", "internal/agent", "docs/SPEC.md"},
			AnalytixLanding: "Runtime-only job lineage event in analytix SSE; no public sub-agent route.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("job-lineage-contract-go", "test", "packages/runtime-go/live_production_candidate_test.go"),
				PassedCheck("runtime-lineage-sse-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/jobs/lineage.go", "packages/runtime-go/live_production_candidate.go", "packages/runtime-go/runtime_server.go"},
			ReplacesAnalytixWeakness: "Lineage is emitted through analytix runtime events instead of an upstream public sub-agent protocol.",
		},
		{
			ID:              "durable-replay",
			Capability:      "Durable event/session replay with stable seq, Last-Event-ID, malformed JSONL diagnostics, and session lifecycle.",
			ReasonixSources: []string{"internal/event", "internal/serve", "docs/SPEC.md"},
			AnalytixLanding: "Go durable event/session store used by runtime-server contract.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("d0236-durable-store-go-test", "test", "packages/runtime-go/durable_replay_contract_test.go"),
				PassedCheck("runtime-sse-replay-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/server/durable_store.go", "packages/runtime-go/runtime_server.go"},
			ReplacesAnalytixWeakness: "Runtime-server replay reads the same events.jsonl sink it writes instead of replaying static SSE fixtures.",
		},
		{
			ID:              "crash-restart-recovery",
			Capability:      "Crash/restart recovery of events.jsonl, highestSeq, pending gates, and turn sequence continuation.",
			ReasonixSources: []string{"internal/evidence/readiness_audit.go", "internal/serve", "internal/jobs"},
			AnalytixLanding: "Candidate durable root mode for Go runtime server.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-server-restart-go-contract", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("runtime-server-restart-ts-conformance", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/server/durable_store.go", "packages/runtime-go/runtime_server.go"},
			ReplacesAnalytixWeakness: "Go candidate state now survives process restart without falling back to in-memory gate maps.",
		},
		{
			ID:              "candidate-rollback",
			Capability:      "Internal Go candidate rollback to TypeScript unless G6 readiness evidence passes.",
			ReasonixSources: []string{"internal/evidence", "docs/SPEC.md"},
			AnalytixLanding: "Electron main adapter G6 readiness gate and fallback reason.",
			AbsorptionClass: AbsorptionReplace,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("d0238-backend-gate-rollback", "test", "src/main/runtime/analytix-adapter.test.ts"),
				PassedCheck("runtime-readiness-retired-backend-contract", "test", "src/main/runtime/analytix-adapter.test.ts"),
			},
			AnalytixEvidence:         []string{"src/main/runtime/analytix-adapter.ts", "src/main/runtime/analytix-adapter.test.ts"},
			ReplacesAnalytixWeakness: "Internal Go selection is now a rollback-safe candidate gate, not a permanent evidence route.",
			DeleteCandidates: []string{
				"/v1/internal/go-production-candidate/provider-live",
				"/v1/internal/go-production-candidate/approval-user-input",
				"/v1/internal/go-production-candidate/mcp-manager",
				"/v1/internal/go-production-candidate/job-lineage",
			},
		},
		{
			ID:              "goal-evidence-kernel",
			Capability:      "Goal FSM, completion evidence audit, blocked-state detection, and permission/evidence closure.",
			ReasonixSources: []string{"internal/evidence", "internal/agent/evidence_flow_test.go", "docs/GOAL_ENFORCEMENT.zh-CN.md"},
			AnalytixLanding: "Analytix goal/runtime governance behind existing plan/goal contract.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("goal-evidence-contract-go", "test", "packages/runtime-go/goal_evidence_contract_test.go"),
				PassedCheck("goal-evidence-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("goal-blocked-audit-restart-contract", "test", "packages/runtime-go/runtime_server_test.go"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/goal/goal_evidence.go", "packages/runtime-go/runtime_server.go", "packages/runtime/src/contracts/events.ts"},
			ReplacesAnalytixWeakness: "Goal completion evidence now has an analytix-owned Go audit kernel and durable SSE event instead of a doc-only gap against Reasonix.",
		},
		{
			ID:              "autoresearch-project-state",
			Capability:      "AutoResearch project-local state with task_spec/progress/findings/directions/iteration log and stale pivot audit.",
			ReasonixSources: []string{"docs/GUIDE.zh-CN.md", "internal/evidence", "internal/agent"},
			AnalytixLanding: "Project-local analytix goal state and autoresearch_state_audit, not stable prefix or tool schema.",
			AbsorptionClass: AbsorptionContractReimplement,
			Status:          AbsorptionStatusGreen,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("autoresearch-state-contract-go", "test", "packages/runtime-go/autoresearch_state_contract_test.go"),
				PassedCheck("autoresearch-state-runtime-sse", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("autoresearch-state-contract-ts", "test", "packages/runtime/tests/contracts.test.ts"),
				PassedCheck("autoresearch-state-reducer-ignore-ts", "test", "packages/runtime/tests/runtime-event-reducer.test.ts"),
				PassedCheck("autoresearch-state-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence:         []string{"packages/runtime-go/internal/research/autoresearch_state.go", "packages/runtime-go/runtime_server.go", "packages/runtime/src/contracts/events.ts"},
			ReplacesAnalytixWeakness: "AutoResearch project state is now a durable Go runtime contract event and project-local state directory instead of TS/shadow-only evidence.",
		},
		{
			ID:              "reasonix-public-protocol",
			Capability:      "Reasonix SessionAPI, config roots, CLI identity, MCP-indexer route, and public sub-agent/job protocol.",
			ReasonixSources: []string{"cmd/reasonix", "internal/config", "internal/serve", "docs/SPEC.md"},
			AnalytixLanding: "Rejected; analytix keeps window.analytix, top-level runtime settings, and analytix serve.",
			AbsorptionClass: AbsorptionReject,
			Status:          AbsorptionStatusRejected,
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("product-sovereignty-scan", "scan", "scripts/scan-product-sovereignty.cjs"),
				PassedCheck("go-runtime-hidden-reasonix-routes", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			AnalytixEvidence: []string{"scripts/scan-product-sovereignty.cjs", "packages/runtime-go/runtime_server.go"},
		},
	}
	return buildReasonixCapabilityAuditMatrix(rows)
}

func buildReasonixCapabilityAuditMatrix(rows []ReasonixCapabilityAuditRow) ReasonixCapabilityAuditMatrix {
	matrix := ReasonixCapabilityAuditMatrix{
		SchemaVersion:   1,
		ChangeID:        "reasonix-capability-audit",
		SourcePath:      ReasonixSourcePath,
		SourceCommit:    ReasonixSourceCommit,
		RuntimeContract: "analytix Go runtime server /v1 runtime HTTP/SSE contract",
		Rows:            rows,
		Notes: []string{
			"readyForG6 is a legacy alias for capabilityMatrixGreen; it is not default Go backend readiness.",
			"Red rows are G6 blockers until they have code-level analytix evidence and passing machine checks.",
			"Rejected rows are deliberate product-sovereignty decisions, not implementation gaps.",
			"Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing.",
		},
	}
	postG6DeleteCandidates := map[string]bool{}
	blockers := map[string]bool{}
	for _, row := range rows {
		switch row.Status {
		case AbsorptionStatusGreen:
			matrix.GreenCount += 1
		case AbsorptionStatusRed:
			matrix.RedCount += 1
			for _, blocker := range row.Blockers {
				blockers[row.ID+": "+blocker] = true
			}
		case AbsorptionStatusRejected:
			matrix.RejectedCount += 1
		case AbsorptionStatusDeferred:
			matrix.DeferredCount += 1
			for _, blocker := range row.Blockers {
				blockers[row.ID+": "+blocker] = true
			}
		}
		if row.UsesReasonixPublicProtocol || row.UsesReasonixConfigRoot {
			matrix.ForbiddenPublicProtocolRowCount += 1
		}
		if row.ChangesRendererContract {
			matrix.RendererContractChangedRowCount += 1
		}
		if row.ChangesProductIdentity {
			matrix.ProductIdentityChangedRowCount += 1
		}
		if row.RequiresKunProductEntryDrift {
			matrix.KunProductEntryDriftRowCount += 1
		}
		for _, candidate := range row.DeleteCandidates {
			postG6DeleteCandidates[candidate] = true
		}
	}
	matrix.PostG6DeleteCandidates = SortedKeys(postG6DeleteCandidates)
	matrix.G6Blockers = SortedKeys(blockers)
	matrix.CapabilityMatrixGreen = matrix.RedCount == 0 &&
		matrix.DeferredCount == 0 &&
		matrix.ForbiddenPublicProtocolRowCount == 0 &&
		matrix.RendererContractChangedRowCount == 0 &&
		matrix.ProductIdentityChangedRowCount == 0 &&
		matrix.KunProductEntryDriftRowCount == 0
	matrix.ReadyForG6 = matrix.CapabilityMatrixGreen
	matrix.DefaultBackendReady = false
	matrix.DefaultBackendReadinessGate = "runtime durable restart evidence + credentialed provider matrix + credentialed MCP matrix + packaged QA + ANALYTIX_RUNTIME_READY=1"
	matrix.ReadinessSemantics = []string{
		"capabilityMatrixGreen means Reasonix capability audit has no red/deferred/forbidden rows.",
		"readyForG6 remains for compatibility and equals capabilityMatrixGreen only.",
		"defaultBackendReady is false here because capability audit cannot satisfy the runtime default-backend gate.",
	}
	return matrix
}

func PassedCheck(id string, kind string, evidence string) RuntimeMachineCheck {
	return RuntimeMachineCheck{ID: id, Kind: kind, Status: "passed", Evidence: evidence}
}

func ExpectedBlockedCheck(id string, kind string, evidence string, defaultBackendReady bool, blockerIDs []string) RuntimeMachineCheck {
	return RuntimeMachineCheck{
		ID:                      id,
		Kind:                    kind,
		Status:                  "expected-blocked",
		Evidence:                evidence,
		ExpectedBlocked:         boolPointer(true),
		AcceptedAsCodePhasePass: boolPointer(true),
		DefaultBackendReady:     &defaultBackendReady,
		BlockerIDs:              blockerIDs,
	}
}

func MissingCheck(id string, kind string) RuntimeMachineCheck {
	return RuntimeMachineCheck{ID: id, Kind: kind, Status: "red"}
}

func SortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
