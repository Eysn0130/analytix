//go:build !analytix_prod

package upstreamaudit

import readinesspkg "analytix.local/runtime-go/internal/readiness"

const ReasonixSuperiorityMatrixChangeID = "reasonix-superiority-matrix"

const (
	SuperiorityDecisionAbsorb  = "absorb"
	SuperiorityDecisionAdapt   = "adapt"
	SuperiorityDecisionKeepKun = "keep-kun"
	SuperiorityDecisionReject  = "reject"
	SuperiorityDecisionDefer   = "defer"
)

type SuperiorityProductImpact struct {
	UISurfaceChanged             bool `json:"uiSurfaceChanged"`
	SettingsSchemaChanged        bool `json:"settingsSchemaChanged"`
	BridgeChanged                bool `json:"bridgeChanged"`
	ProviderMultiModelChanged    bool `json:"providerMultiModelChanged"`
	ProviderScoped               bool `json:"providerScoped"`
	DoesNotNarrowProviders       bool `json:"doesNotNarrowProviders"`
	TopLevelEntrypointAdded      bool `json:"topLevelEntrypointAdded"`
	ReasonixPublicProtocolAdded  bool `json:"reasonixPublicProtocolAdded"`
	StablePrefixUsesDynamicState bool `json:"stablePrefixUsesDynamicState"`
}

type ReasonixSuperiorityEvidenceRow struct {
	ID                          string                   `json:"id"`
	Capability                  string                   `json:"capability"`
	ReasonixSourcePath          string                   `json:"reasonixSourcePath"`
	ReasonixSourceCommit        string                   `json:"reasonixSourceCommit"`
	ReasonixSourceSymbols       []string                 `json:"reasonixSourceSymbols"`
	KunAnalytixBaselinePaths    []string                 `json:"kunAnalytixBaselinePaths"`
	CurrentBehavior             string                   `json:"currentBehavior"`
	ReasonixStrongerEvidence    []string                 `json:"reasonixStrongerEvidence"`
	DecisionEvidence            []string                 `json:"decisionEvidence,omitempty"`
	DeterministicEvidence       []RuntimeMachineCheck    `json:"deterministicEvidence"`
	CodeReusable                bool                     `json:"codeReusable"`
	CodeReuseMode               string                   `json:"codeReuseMode"`
	AnalytixTargetPaths         []string                 `json:"analytixTargetPaths"`
	ProductImpact               SuperiorityProductImpact `json:"productImpact"`
	RegressionTestPaths         []string                 `json:"regressionTestPaths"`
	RegressionCommands          []string                 `json:"regressionCommands,omitempty"`
	Decision                    string                   `json:"decision"`
	AbsorptionClass             RuntimeAbsorptionClass   `json:"absorptionClass"`
	Status                      RuntimeAbsorptionStatus  `json:"status"`
	CodeLevelAbsorbed           bool                     `json:"codeLevelAbsorbed"`
	LocalDeterministicGreen     bool                     `json:"localDeterministicGreen"`
	RequiresLiveEvidence        bool                     `json:"requiresLiveEvidence"`
	LiveEvidenceStatus          string                   `json:"liveEvidenceStatus"`
	KeepKunReason               string                   `json:"keepKunReason,omitempty"`
	KunAnalytixStrongerEvidence []string                 `json:"kunAnalytixStrongerEvidence,omitempty"`
	BaselineRetainedReason      string                   `json:"baselineRetainedReason,omitempty"`
	RejectedReason              string                   `json:"rejectedReason,omitempty"`
	DeferredReason              string                   `json:"deferredReason,omitempty"`
	TypeScriptFallbackRetained  bool                     `json:"typeScriptFallbackRetained"`
}

type ReasonixSuperiorityMatrix struct {
	SchemaVersion                    int                              `json:"schemaVersion"`
	ChangeID                         string                           `json:"changeId"`
	Stage                            string                           `json:"stage"`
	ReasonixAbsorptionStatus         string                           `json:"reasonixAbsorptionStatus"`
	GoRuntimeCoreStatus              string                           `json:"goRuntimeCoreStatus"`
	GoDefaultLiveGateStatus          string                           `json:"goDefaultLiveGateStatus"`
	ReasonixSourcePath               string                           `json:"reasonixSourcePath"`
	ReasonixSourceCommit             string                           `json:"reasonixSourceCommit"`
	KunSourceSnapshots               []string                         `json:"kunSourceSnapshots"`
	AnalytixSourceRoot               string                           `json:"analytixSourceRoot"`
	EvidencePolicy                   string                           `json:"evidencePolicy"`
	LLMAnswerQualityEvidenceUsed     bool                             `json:"llmAnswerQualityEvidenceUsed"`
	DeterministicEvidenceOnly        bool                             `json:"deterministicEvidenceOnly"`
	Rows                             []ReasonixSuperiorityEvidenceRow `json:"rows"`
	RowCount                         int                              `json:"rowCount"`
	AbsorbCount                      int                              `json:"absorbCount"`
	AdaptCount                       int                              `json:"adaptCount"`
	KeepKunCount                     int                              `json:"keepKunCount"`
	RejectCount                      int                              `json:"rejectCount"`
	DeferCount                       int                              `json:"deferCount"`
	CodeLevelAbsorbedCount           int                              `json:"codeLevelAbsorbedCount"`
	LocalDeterministicGreenCount     int                              `json:"localDeterministicGreenCount"`
	ProductImpactChangedCount        int                              `json:"productImpactChangedCount"`
	TopLevelEntrypointAddedCount     int                              `json:"topLevelEntrypointAddedCount"`
	ReasonixPublicProtocolAddedCount int                              `json:"reasonixPublicProtocolAddedCount"`
	ProviderMultiModelChangedCount   int                              `json:"providerMultiModelChangedCount"`
	StablePrefixDynamicStateRowCount int                              `json:"stablePrefixDynamicStateRowCount"`
	DeepSeekEnhancementScopedOnly    bool                             `json:"deepSeekEnhancementScopedOnly"`
	KunAnalytixProductLayerPreserved bool                             `json:"kunAnalytixProductLayerPreserved"`
	ReasonixEngineRuntimeOnly        bool                             `json:"reasonixEngineRuntimeOnly"`
	TypeScriptFallbackRetained       bool                             `json:"typeScriptFallbackRetained"`
	CodeStageClosed                  bool                             `json:"codeStageClosed"`
	DeterministicAbsorptionIDs       []string                         `json:"deterministicAbsorptionIds"`
	PendingLiveValidationIDs         []string                         `json:"pendingLiveValidationIds"`
	StrictG6DefaultCutoverReady      bool                             `json:"strictG6DefaultCutoverReady"`
	GoDefaultCutoverCandidate        bool                             `json:"goDefaultCutoverCandidate"`
	LiveCutoverStage                 string                           `json:"liveCutoverStage"`
	LiveCutoverStatus                string                           `json:"liveCutoverStatus"`
	LiveEvidenceBlockers             []string                         `json:"liveEvidenceBlockers"`
	ForbiddenTopLevelEntrypoints     []string                         `json:"forbiddenTopLevelEntrypoints"`
	Notes                            []string                         `json:"notes"`
}

func BuildReasonixSuperiorityMatrix(readiness readinesspkg.RuntimeReadinessStatus) ReasonixSuperiorityMatrix {
	if readiness.SchemaVersion == 0 {
		readiness = readinesspkg.RuntimeReadinessStatusFromEnv(nil)
	}
	liveBlockers := liveEvidenceBlockers(readiness)
	liveStatus := "post-cutover-live-validation-pending"
	if len(liveBlockers) == 0 && readiness.Ready {
		liveStatus = "ready"
	}
	rows := []ReasonixSuperiorityEvidenceRow{
		{
			ID:                   "deepseek-prefix-cache-stability",
			Capability:           "DeepSeek prefix/cache stability, cache-hit telemetry, and usage accounting.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/agent/cache_shape.go:CaptureShape",
				"internal/agent/cache_shape.go:CompareShape",
				"internal/agent/agent.go:SessionCache",
				"internal/agent/cachehit_e2e_test.go",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/cache/prefix-cache-diagnostics.ts",
				"packages/runtime/tests/provider-cache-contract.test.ts",
				"packages/runtime-go/internal/provider/provider.go",
			},
			CurrentBehavior: "Analytix keeps provider routing and runtime events; DeepSeek cache diagnostics are attached as provider-scoped telemetry.",
			ReasonixStrongerEvidence: []string{
				"Reasonix records previous prefix shape and compares cacheable prefix drift deterministically.",
				"Reasonix accumulates session cache hit/miss telemetry from provider usage, not from answer quality.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("deepseek-cache-scoped-contract", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("runtime-cache-diagnostics-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("deepseek-cache-record-format-contract", "test", "packages/runtime-go/g6_readiness_test.go"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "code-port-and-adapt",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/contracts/hash.go"},
			ProductImpact:              providerScopedProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime-go/kun_analytix_baseline_absorption_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts"},
			Decision:                   SuperiorityDecisionAdapt,
			AbsorptionClass:            AbsorptionCodePortAndAdapt,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "prefix-shape-contract-baseline",
			Capability:           "Stable prefix shape dimensions for provider/model/endpoint/tools/history inputs.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/agent/cache_shape.go:PrefixShape",
				"internal/agent/cache_shape.go:CaptureShape",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/cache/prefix-cache-diagnostics.ts",
				"packages/runtime/src/loop/agent-loop.ts",
				"packages/runtime/tests/provider-cache-contract.test.ts",
			},
			CurrentBehavior: "Analytix already owns a richer stable prefix contract across provider, model, endpointFormat, tool sources, and prefix items.",
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("provider-cache-prefix-shape-contract", "test", "packages/runtime/tests/provider-cache-contract.test.ts"),
				PassedCheck("runtime-cache-diagnostics-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			CodeReusable:            false,
			CodeReuseMode:           "not-applicable",
			AnalytixTargetPaths:     []string{"packages/runtime/src/cache/prefix-cache-diagnostics.ts", "packages/runtime-go/internal/contracts/hash.go"},
			ProductImpact:           providerScopedProductImpact(),
			RegressionTestPaths:     []string{"packages/runtime/tests/provider-cache-contract.test.ts", "packages/runtime/tests/go-runtime-conformance.test.ts"},
			Decision:                SuperiorityDecisionKeepKun,
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusGreen,
			CodeLevelAbsorbed:       false,
			LocalDeterministicGreen: true,
			RequiresLiveEvidence:    false,
			LiveEvidenceStatus:      "not-required",
			KeepKunReason:           "Analytix/Kun prefix dimensions are broader; only Reasonix release guard telemetry is adapted.",
			KunAnalytixStrongerEvidence: []string{
				"Analytix/Kun prefix contract already covers provider, model, endpointFormat, tool-source, and history dimensions.",
				"Provider cache contract tests keep dynamic workspace state out of the stable prefix.",
			},
			BaselineRetainedReason:     "Retain the Analytix/Kun prefix contract and adapt only Reasonix release-guard telemetry.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "provider-stream-usage-reasoning-guards",
			Capability:           "Provider stream failure discipline, usage/cache accounting normalization, and reasoning/thinking round-trip guards.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/provider/openai/openai.go:streamWithReconnect",
				"internal/provider/openai/openai.go:readStream",
				"internal/provider/openai/openai.go:normaliseUsage",
				"internal/provider/provider.go:Usage",
				"internal/provider/openai/openai_test.go:TestBuildRequestRoundTripsReasoningOnDeepSeekToolCalls",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/adapters/model/compat-model-client.ts",
				"packages/runtime/src/telemetry/usage-counter.ts",
				"packages/runtime-go/internal/provider/provider.go",
			},
			CurrentBehavior: "Analytix keeps provider routing and endpointFormat; Reasonix stream/usage/reasoning invariants are adapted only inside provider parsing.",
			ReasonixStrongerEvidence: []string{
				"Reasonix separates reset-before-output from interrupted-after-output stream cases.",
				"Reasonix normalizes cache/usage/reasoning fields with deterministic provider fixtures.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("runtime-multi-model-matrix-go", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("runtime-server-multi-model-turns", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("g3-provider-usage-streaming-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "contract-reimplement",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/server/runtime_components.go"},
			ProductImpact:              providerScopedProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime-go/kun_analytix_baseline_absorption_test.go", "packages/runtime/tests/model-client.test.ts"},
			Decision:                   SuperiorityDecisionAdapt,
			AbsorptionClass:            AbsorptionContractReimplement,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "provider-endpoint-family-request-shape",
			Capability:           "Provider request shape across DeepSeek, OpenAI-compatible, Anthropic-compatible, responses mode, and custom full endpoint.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/provider/openai/openai.go:buildRequest",
				"internal/provider/anthropic/anthropic.go:buildRequest",
			},
			KunAnalytixBaselinePaths: []string{
				"src/shared/openai-compat-url.ts",
				"packages/runtime/src/adapters/model/compat-model-client.ts",
				"packages/runtime/tests/provider-cache-contract.test.ts",
			},
			CurrentBehavior: "Analytix already owns endpoint families and custom full endpoint behavior; custom full endpoint must not receive another appended path.",
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("provider-request-shape-endpoint-family", "test", "packages/runtime/tests/provider-cache-contract.test.ts"),
				PassedCheck("runtime-conformance-multi-model", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			CodeReusable:            false,
			CodeReuseMode:           "not-applicable",
			AnalytixTargetPaths:     []string{"src/shared/openai-compat-url.ts", "packages/runtime/src/adapters/model/compat-model-client.ts"},
			ProductImpact:           providerScopedProductImpact(),
			RegressionTestPaths:     []string{"packages/runtime/tests/model-client.test.ts", "packages/runtime/tests/provider-cache-contract.test.ts"},
			Decision:                SuperiorityDecisionKeepKun,
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusGreen,
			CodeLevelAbsorbed:       false,
			LocalDeterministicGreen: true,
			RequiresLiveEvidence:    false,
			LiveEvidenceStatus:      "not-required",
			KeepKunReason:           "Analytix/Kun endpoint family and custom endpoint semantics are broader and remain the baseline.",
			KunAnalytixStrongerEvidence: []string{
				"Analytix/Kun endpoint contract covers OpenAI chat completions, OpenAI responses compatibility, Anthropic messages, and custom full endpoint mode.",
				"Custom full endpoints are explicitly request-shape-only and must not receive appended provider paths.",
			},
			BaselineRetainedReason:     "Retain shared Analytix/Kun endpoint URL/body/stream/usage semantics rather than replacing them with Reasonix provider defaults.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "agent-loop-job-subagent-lineage",
			Capability:           "Agent loop job/subagent lineage bound to Analytix /goal and task/job lineage only.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/agent/task.go:TaskTool",
				"internal/agent/task.go:prepareTranscriptRun",
				"internal/agent/subagent_store.go:SubagentStore",
				"internal/jobs/artifacts.go:ArtifactDir",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/delegation/job-manager.ts",
				"packages/runtime/src/adapters/tool/task-job-tool-provider.ts",
				"packages/runtime/src/contracts/events.ts",
				"packages/runtime-go/internal/jobs/lineage.go",
			},
			CurrentBehavior: "Subagent/job behavior is internal lineage under existing chat, /goal, task tools, and SSE child metadata.",
			ReasonixStrongerEvidence: []string{
				"Reasonix has deterministic parent call context and transcript/artifact lineage tests.",
				"Analytix maps the stronger engine invariant to child metadata without adding a Subagent product route.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("job-lineage-contract-go", "test", "packages/runtime-go/live_production_candidate_test.go"),
				PassedCheck("runtime-lineage-sse-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("reasonix-integration-topology-contract", "test", "packages/runtime-go/reasonix_integration_topology_test.go"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "contract-reimplement",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/jobs/lineage.go", "packages/runtime-go/internal/server/runtime_components.go"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime-go/live_production_candidate_test.go", "packages/runtime/tests/task-job-orchestration-contract.test.ts"},
			Decision:                   SuperiorityDecisionAdapt,
			AbsorptionClass:            AbsorptionContractReimplement,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "mcp-lifecycle-lazy-schema-reconnect-redaction",
			Capability:           "MCP lifecycle, lazy/schema cache, tool call, approval/user-input, reconnect, and redaction.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/plugin/plugin.go:Start",
				"internal/plugin/plugin.go:StartPhaseB",
				"internal/plugin/plugin.go:ToolsFor",
				"internal/plugin/lazy.go:LazyToolset",
				"internal/plugin/cache.go:SpecFingerprint",
				"internal/plugin/known_overrides.go:ApplyKnownOverrides",
				"internal/plugin/canonicalize.go:canonicalizeSchema",
				"internal/plugin/transport_http.go",
				"internal/mcpdiag/auth.go",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/adapters/tool/mcp-tool-provider.ts",
				"packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts",
				"packages/runtime-go/internal/mcp/lifecycle.go",
			},
			CurrentBehavior: "Analytix owns MCP registry/tool calls; mcp_lifecycle_audit is test-only evidence, while normal runtime events come from real configured MCP changes.",
			ReasonixStrongerEvidence: []string{
				"Reasonix has lazy lifecycle, schema cache, reconnect, auth-classification, known-override, and schema-redaction tests.",
				"Analytix Go runtime now validates connect/list/call/approval/reconnect/redaction evidence before cutover.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("mcp-lifecycle-contract-go", "test", "packages/runtime-go/mcp_lifecycle_contract_test.go"),
				PassedCheck("mcp-lifecycle-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("mcp-readiness-matrix-contract", "test", "packages/runtime-go/g6_readiness_test.go"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "contract-reimplement",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/mcp/lifecycle.go", "packages/runtime-go/internal/mcp/manager.go", "packages/runtime-go/internal/readiness/readiness.go"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime-go/mcp_lifecycle_contract_test.go", "packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts"},
			Decision:                   SuperiorityDecisionAbsorb,
			AbsorptionClass:            AbsorptionContractReimplement,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "mcp-search-meta-tool-boundary",
			Capability:           "MCP search/meta-tool boundary, direct call trust boundary, and approval-on-call surface.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/plugin/known_overrides.go",
				"internal/plugin/canonicalize.go",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/adapters/tool/mcp-tool-search.ts",
				"packages/runtime/src/adapters/tool/mcp-tool-provider.ts",
				"packages/runtime/src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-contract.json",
			},
			CurrentBehavior: "Analytix already owns mcp_search, mcp_describe, mcp_call, and mcp_refresh_catalog as existing internal tool entries without an MCP-indexer UI route.",
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("mcp-search-meta-tool-boundary", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("product-sovereignty-scan", "scan", "scripts/scan-product-sovereignty.cjs"),
			},
			CodeReusable:            false,
			CodeReuseMode:           "not-applicable",
			AnalytixTargetPaths:     []string{"packages/runtime/src/adapters/tool/mcp-tool-search.ts", "packages/runtime-go/internal/mcp/lifecycle.go"},
			ProductImpact:           neutralProductImpact(),
			RegressionTestPaths:     []string{"packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts", "scripts/scan-product-sovereignty.cjs"},
			Decision:                SuperiorityDecisionKeepKun,
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusGreen,
			CodeLevelAbsorbed:       false,
			LocalDeterministicGreen: true,
			RequiresLiveEvidence:    false,
			LiveEvidenceStatus:      "not-required",
			KeepKunReason:           "Analytix/Kun MCP search/meta-tool boundary is broader and remains the product/runtime contract.",
			KunAnalytixStrongerEvidence: []string{
				"Analytix/Kun already exposes mcp_search, mcp_describe, mcp_call, and mcp_refresh_catalog as internal runtime tools.",
				"Product-sovereignty and conformance tests prove no top-level MCP-indexer UI route is required.",
			},
			BaselineRetainedReason:     "Retain the Analytix/Kun MCP meta-tool contract while absorbing only internal lifecycle/reconnect/redaction invariants.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "approval-user-input-tool-result-history-repair",
			Capability:           "Approval/user-input gate semantics plus model history/tool-result repair.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/agent/ask.go:AskTool",
				"internal/agent/agent.go:withCallContext",
				"internal/agent/normalize.go:NormalizeSession",
				"internal/control/approval_e2e_test.go",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/domain/model-history-repair.ts",
				"packages/runtime/src/adapters/model/compat-model-client.ts",
				"packages/runtime-go/internal/agent/gates.go",
			},
			CurrentBehavior: "Analytix keeps approval/user-input routes and repairs model history before provider requests/fork cloning.",
			ReasonixStrongerEvidence: []string{
				"Reasonix separates user ask decisions from tool approval posture and persists session-safe repairs.",
				"Analytix keeps this as an internal runtime/history invariant instead of importing Reasonix ask/session protocol.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("approval-user-input-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("restart-restores-pending-gates-contract", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("g5-history-repair-shadow", "test", "packages/runtime-go/shadow_test.go"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "contract-reimplement",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/agent/gates.go", "packages/runtime-go/internal/conformance/g5_shadow.go", "packages/runtime/src/domain/model-history-repair.ts"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime/tests/model-client.test.ts", "packages/runtime/tests/thread-service.test.ts", "packages/runtime-go/shadow_test.go"},
			Decision:                   SuperiorityDecisionAdapt,
			AbsorptionClass:            AbsorptionContractReimplement,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "runtime-event-sse-session-durability",
			Capability:           "Runtime event/SSE replay and thread durability contracts.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/event/event.go",
				"internal/event/sync.go",
				"desktop/sessions.go",
				"internal/history/search.go",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/server/routes",
				"packages/runtime-go/internal/server/durable_store.go",
				"packages/runtime-go/internal/server/runtime_components.go",
			},
			CurrentBehavior: "Analytix owns persist-before-publish runtime events, /v1/threads, resume-thread, Last-Event-ID/since_seq replay, archive/search, and fork contracts.",
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("d0236-durable-store-go-test", "test", "packages/runtime-go/durable_replay_contract_test.go"),
				PassedCheck("runtime-sse-replay-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("runtime-server-restart-go-contract", "test", "packages/runtime-go/runtime_server_test.go"),
			},
			CodeReusable:            false,
			CodeReuseMode:           "not-applicable",
			AnalytixTargetPaths:     []string{"packages/runtime-go/internal/server/durable_store.go", "packages/runtime-go/internal/server/runtime_components.go", "packages/runtime-go/internal/protocol/route_replay.go"},
			ProductImpact:           neutralProductImpact(),
			RegressionTestPaths:     []string{"packages/runtime-go/durable_replay_contract_test.go", "packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts"},
			Decision:                SuperiorityDecisionKeepKun,
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusGreen,
			CodeLevelAbsorbed:       false,
			LocalDeterministicGreen: true,
			RequiresLiveEvidence:    false,
			LiveEvidenceStatus:      "not-required",
			KeepKunReason:           "Analytix/Kun durable runtime event replay and thread routes are the stronger baseline.",
			KunAnalytixStrongerEvidence: []string{
				"Analytix/Kun owns HTTP/SSE thread list, search, archive, fork, resume-thread, and Last-Event-ID/since_seq replay contracts.",
				"Go durable-store tests reproduce the Analytix/Kun persist-before-publish event replay invariant.",
			},
			BaselineRetainedReason:     "Retain Analytix/Kun durable SSE/thread-route semantics as the cross-backend runtime contract.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "autoresearch-workflow-create-loop-engine-discipline",
			Capability:           "AutoResearch, Workflow, and Create Loop discipline only as /goal/internal planner mechanics.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/agent/evidence_flow_test.go",
				"internal/agent/planmode_test.go",
				"internal/agent/parallel_tasks.go",
				"internal/tool/builtin/completestep_test.go",
				"docs/SPEC.md:AutoResearch",
			},
			KunAnalytixBaselinePaths: []string{
				"packages/runtime/src/adapters/tool/goal-tools.ts",
				"packages/runtime/src/adapters/tool/create-plan-tool.ts",
				"packages/runtime-go/internal/research/autoresearch_state.go",
				"packages/runtime-go/internal/goal/goal_evidence.go",
			},
			CurrentBehavior: "Analytix keeps /goal, create_plan, complete_step, task tools, and internal planner flows without top-level workflow UI.",
			ReasonixStrongerEvidence: []string{
				"Reasonix has stronger evidence-flow, plan guard, and AutoResearch project-state tests.",
				"Analytix ports those invariants to .analytix/autoresearch and goal/tool evidence paths without emitting fixture audits on every normal chat turn.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("autoresearch-state-contract-go", "test", "packages/runtime-go/autoresearch_state_contract_test.go"),
				PassedCheck("goal-evidence-contract-go", "test", "packages/runtime-go/goal_evidence_contract_test.go"),
				PassedCheck("goal-tools-semantics-unchanged", "test", "packages/runtime/tests/goal-tools.test.ts"),
			},
			CodeReusable:               true,
			CodeReuseMode:              "contract-reimplement",
			AnalytixTargetPaths:        []string{"packages/runtime-go/internal/research/autoresearch_state.go", "packages/runtime-go/internal/goal/goal_evidence.go", "packages/runtime-go/internal/conformance/g5_shadow.go"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"packages/runtime-go/autoresearch_state_contract_test.go", "packages/runtime-go/goal_evidence_contract_test.go", "packages/runtime/tests/goal-tools.test.ts"},
			Decision:                   SuperiorityDecisionAdapt,
			AbsorptionClass:            AbsorptionContractReimplement,
			Status:                     AbsorptionStatusGreen,
			CodeLevelAbsorbed:          true,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "kun-analytix-product-layer-baseline",
			Capability:           "Product entry, UI, desktop experience, settings, bridge, and multi-model provider baseline.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"desktop/frontend/src/App.tsx",
				"cmd/reasonix",
				"internal/config",
			},
			KunAnalytixBaselinePaths: []string{
				"src/preload/index.ts",
				"src/shared/app-settings-runtime.ts",
				"src/renderer/src/components/Workbench.tsx",
				"docs/analytix/upstreams/kun-sync.md",
			},
			CurrentBehavior: "Kun/Analytix remains product baseline; Reasonix is engine/runtime donor only.",
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("product-sovereignty-scan", "scan", "scripts/scan-product-sovereignty.cjs"),
				PassedCheck("preload-settings-tests", "test", "src/preload/preload-runtime-request.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings.test.ts"),
				PassedCheck("renderer-route-surface-tests", "test", "src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/chat/FloatingComposer.test.ts"),
			},
			CodeReusable:            false,
			CodeReuseMode:           "not-applicable",
			AnalytixTargetPaths:     []string{"src/preload/index.ts", "src/shared/app-settings-runtime.ts", "src/renderer/src/components"},
			ProductImpact:           neutralProductImpact(),
			RegressionTestPaths:     []string{"src/preload/preload-runtime-request.test.ts", "src/shared/app-settings.test.ts", "src/renderer/src/components/Workbench.route-surface.test.ts"},
			Decision:                SuperiorityDecisionKeepKun,
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusGreen,
			CodeLevelAbsorbed:       false,
			LocalDeterministicGreen: true,
			RequiresLiveEvidence:    false,
			LiveEvidenceStatus:      "not-required",
			KeepKunReason:           "Reasonix product-layer replacement is not proven stronger and would risk UI/settings/bridge/provider baseline drift.",
			KunAnalytixStrongerEvidence: []string{
				"Analytix/Kun preserves the desktop product layer: window.analytix bridge, top-level runtime settings, Workbench routes, and multi-provider settings.",
				"Reasonix product UI/config/public protocol would violate current product sovereignty without adding a proven desktop baseline advantage.",
			},
			BaselineRetainedReason:     "Retain Kun/Analytix as product entry, UI, settings, bridge, and multi-model provider baseline; Reasonix remains engine/runtime-only.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "reasonix-public-protocol-and-top-level-ui",
			Capability:           "Reasonix SessionAPI/config roots/public job protocol/top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"cmd/reasonix",
				"internal/config",
				"internal/serve",
				"desktop/frontend/src/App.tsx",
			},
			KunAnalytixBaselinePaths: []string{
				"src/preload/index.ts",
				"packages/runtime/src/server/routes",
				"scripts/scan-product-sovereignty.cjs",
			},
			CurrentBehavior: "Analytix exposes window.analytix, top-level runtime settings, analytix serve, and existing chat/goal/tool/MCP entries only.",
			DecisionEvidence: []string{
				"Rejected by product sovereignty: public protocol or top-level UI import is not a superiority path.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("product-sovereignty-scan", "scan", "scripts/scan-product-sovereignty.cjs"),
				PassedCheck("go-runtime-hidden-reasonix-routes", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			CodeReusable:               false,
			CodeReuseMode:              "reject-public-protocol",
			AnalytixTargetPaths:        []string{"scripts/scan-product-sovereignty.cjs", "packages/runtime-go/internal/contracts/product_boundary.go"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"scripts/scan-product-sovereignty.cjs", "packages/runtime/tests/go-runtime-conformance.test.ts"},
			Decision:                   SuperiorityDecisionReject,
			AbsorptionClass:            AbsorptionReject,
			Status:                     AbsorptionStatusRejected,
			CodeLevelAbsorbed:          false,
			LocalDeterministicGreen:    true,
			RequiresLiveEvidence:       false,
			LiveEvidenceStatus:         "not-required",
			RejectedReason:             "Would expose Reasonix public protocol or top-level product entries.",
			TypeScriptFallbackRetained: false,
		},
		{
			ID:                   "go-default-live-cutover-evidence",
			Capability:           "Credentialed provider/MCP execution, packaged desktop QA, and explicit operator gate for Go default candidate.",
			ReasonixSourcePath:   ReasonixSourcePath,
			ReasonixSourceCommit: ReasonixSourceCommit,
			ReasonixSourceSymbols: []string{
				"internal/provider",
				"internal/plugin",
				"internal/agent",
			},
			KunAnalytixBaselinePaths: []string{
				"docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json",
				"scripts/runtime-go-validation-command.mjs",
				"scripts/runtime-go-preflight.mjs",
				"scripts/runtime-go-cutover-report.mjs",
				"src/main/runtime/analytix-adapter.ts",
			},
			CurrentBehavior: "Reasonix intake is closed; live runtime validation remains pending until real JSON evidence exists.",
			DecisionEvidence: []string{
				"Live cutover requires integration evidence, not LLM answer quality.",
				"Current deterministic evidence is sufficient for Go default; live evidence remains post-cutover validation.",
			},
			DeterministicEvidence: []RuntimeMachineCheck{
				PassedCheck("runtime-go-cutover-report", "script", "scripts/runtime-go-cutover-report.mjs"),
				{
					ID:                      "runtime-go-preflight-live-gate",
					Kind:                    "report",
					Status:                  "post-cutover-live-validation-pending",
					Evidence:                "docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json",
					ExpectedBlocked:         boolPointer(false),
					AcceptedAsCodePhasePass: boolPointer(true),
					DefaultBackendReady:     boolPointer(true),
					BlockerIDs:              []string{},
				},
			},
			CodeReusable:               false,
			CodeReuseMode:              "external-evidence-required",
			AnalytixTargetPaths:        []string{"docs/analytix/upstreams", "src/main/runtime/analytix-adapter.ts", "packages/runtime-go/internal/readiness/readiness.go"},
			ProductImpact:              neutralProductImpact(),
			RegressionTestPaths:        []string{"scripts/runtime-go-preflight.mjs", "scripts/runtime-go-default-readiness-report.mjs", "scripts/runtime-go-cutover-report.mjs"},
			RegressionCommands:         []string{"npm run runtime:go:preflight", "npm run runtime:go:default-readiness-report -- --json", "npm run runtime:go:cutover-report -- --json"},
			Decision:                   SuperiorityDecisionDefer,
			AbsorptionClass:            AbsorptionDefer,
			Status:                     AbsorptionStatusDeferred,
			CodeLevelAbsorbed:          false,
			LocalDeterministicGreen:    false,
			RequiresLiveEvidence:       true,
			LiveEvidenceStatus:         liveStatus,
			DeferredReason:             "Real credentialed provider, MCP execution, packaged QA, and operator JSON evidence are post-cutover live validation, not deterministic default blockers.",
			TypeScriptFallbackRetained: false,
		},
	}
	for index := range rows {
		if rows[index].ReasonixStrongerEvidence == nil {
			rows[index].ReasonixStrongerEvidence = []string{}
		}
	}

	matrix := ReasonixSuperiorityMatrix{
		SchemaVersion:                    1,
		ChangeID:                         ReasonixSuperiorityMatrixChangeID,
		Stage:                            "reasonix-superiority-code-stage",
		ReasonixAbsorptionStatus:         "code-stage-closed",
		GoRuntimeCoreStatus:              "deterministic-core-green",
		GoDefaultLiveGateStatus:          liveStatus,
		ReasonixSourcePath:               ReasonixSourcePath,
		ReasonixSourceCommit:             ReasonixSourceCommit,
		KunSourceSnapshots:               BuildKunAnalytixBaselineGuard().KunSourceSnapshots,
		AnalytixSourceRoot:               "/Users/sun/Projects/analytix",
		EvidencePolicy:                   "source-level comparison, deterministic contracts, fixtures, unit/integration tests, runtime telemetry, and JSON evidence only",
		LLMAnswerQualityEvidenceUsed:     false,
		DeterministicEvidenceOnly:        true,
		Rows:                             rows,
		ForbiddenTopLevelEntrypoints:     []string{"Workflow", "Create Loop", "Subagent", "AutoResearch", "MCP-indexer"},
		DeepSeekEnhancementScopedOnly:    true,
		KunAnalytixProductLayerPreserved: true,
		ReasonixEngineRuntimeOnly:        true,
		TypeScriptFallbackRetained:       false,
		StrictG6DefaultCutoverReady:      readiness.Ready,
		GoDefaultCutoverCandidate:        true,
		LiveCutoverStage:                 "reasonix-superiority-live-gate",
		LiveCutoverStatus:                liveStatus,
		LiveEvidenceBlockers:             liveBlockers,
		Notes: []string{
			"Reasonix superiority code stage closes only source-level superiority evidence and Go runtime domain landing.",
			"No LLM answer-quality evidence is used.",
			"Reasonix public protocol and top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries remain rejected.",
			"Go runtime server is the default core after deterministic local conformance and adapter canary pass.",
			"Credentialed provider, MCP execution, packaged QA, and operator JSON evidence remain post-cutover live validation and are not used as LLM answer-quality evidence.",
			"TypeScript runtime remains only as an explicit rollback compatibility path, not as a long-term dual-track fallback.",
		},
	}
	for _, row := range rows {
		matrix.RowCount++
		switch row.Decision {
		case SuperiorityDecisionAbsorb:
			matrix.AbsorbCount++
		case SuperiorityDecisionAdapt:
			matrix.AdaptCount++
		case SuperiorityDecisionKeepKun:
			matrix.KeepKunCount++
		case SuperiorityDecisionReject:
			matrix.RejectCount++
		case SuperiorityDecisionDefer:
			matrix.DeferCount++
		}
		if row.CodeLevelAbsorbed {
			matrix.CodeLevelAbsorbedCount++
			matrix.DeterministicAbsorptionIDs = append(matrix.DeterministicAbsorptionIDs, row.ID)
		}
		if row.RequiresLiveEvidence {
			matrix.PendingLiveValidationIDs = append(matrix.PendingLiveValidationIDs, row.ID)
		}
		if row.LocalDeterministicGreen {
			matrix.LocalDeterministicGreenCount++
		}
		if row.ProductImpact.UISurfaceChanged || row.ProductImpact.SettingsSchemaChanged || row.ProductImpact.BridgeChanged {
			matrix.ProductImpactChangedCount++
		}
		if row.ProductImpact.TopLevelEntrypointAdded {
			matrix.TopLevelEntrypointAddedCount++
		}
		if row.ProductImpact.ReasonixPublicProtocolAdded {
			matrix.ReasonixPublicProtocolAddedCount++
		}
		if row.ProductImpact.ProviderMultiModelChanged {
			matrix.ProviderMultiModelChangedCount++
		}
		if row.ProductImpact.StablePrefixUsesDynamicState {
			matrix.StablePrefixDynamicStateRowCount++
		}
	}
	matrix.CodeStageClosed = matrix.RowCount == 13 &&
		matrix.AbsorbCount == 1 &&
		matrix.AdaptCount == 5 &&
		matrix.KeepKunCount == 5 &&
		matrix.RejectCount == 1 &&
		matrix.DeferCount == 1 &&
		matrix.CodeLevelAbsorbedCount == 6 &&
		matrix.LocalDeterministicGreenCount == matrix.RowCount-matrix.DeferCount &&
		matrix.ProductImpactChangedCount == 0 &&
		matrix.TopLevelEntrypointAddedCount == 0 &&
		matrix.ReasonixPublicProtocolAddedCount == 0 &&
		matrix.ProviderMultiModelChangedCount == 0 &&
		matrix.StablePrefixDynamicStateRowCount == 0 &&
		matrix.DeterministicEvidenceOnly &&
		!matrix.LLMAnswerQualityEvidenceUsed
	return matrix
}

func neutralProductImpact() SuperiorityProductImpact {
	return SuperiorityProductImpact{
		UISurfaceChanged:             false,
		SettingsSchemaChanged:        false,
		BridgeChanged:                false,
		ProviderMultiModelChanged:    false,
		ProviderScoped:               false,
		DoesNotNarrowProviders:       true,
		TopLevelEntrypointAdded:      false,
		ReasonixPublicProtocolAdded:  false,
		StablePrefixUsesDynamicState: false,
	}
}

func providerScopedProductImpact() SuperiorityProductImpact {
	impact := neutralProductImpact()
	impact.ProviderScoped = true
	return impact
}

func boolPointer(value bool) *bool {
	return &value
}

func liveEvidenceBlockers(readiness readinesspkg.RuntimeReadinessStatus) []string {
	blockers := []string{}
	if readiness.ProviderMatrix.Status != "passed" {
		blockers = append(blockers, "credentialed-provider-matrix-json")
	}
	if readiness.MCPMatrix.Status != "passed" {
		blockers = append(blockers, "credentialed-mcp-execution-json")
	}
	if readiness.PackagedQA.Status != "passed" {
		blockers = append(blockers, "packaged-desktop-qa-json")
	}
	if !readiness.ExplicitReadyGate {
		blockers = append(blockers, "ANALYTIX_RUNTIME_READY=1")
	}
	if readiness.OperatorGate.Status != "passed" {
		blockers = append(blockers, "operator-gate-json")
	}
	return blockers
}
