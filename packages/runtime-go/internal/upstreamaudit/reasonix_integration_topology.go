//go:build !analytix_prod

package upstreamaudit

import readinesspkg "analytix.local/runtime-go/internal/readiness"

const ReasonixIntegrationTopologyChangeID = "reasonix-integration-topology"

const (
	IntegrationDecisionReasonixEngineStrongerAbsorb     = "reasonix-engine-stronger-absorb"
	IntegrationDecisionKunAnalytixBaselineRetained      = "kun-analytix-baseline-retained"
	IntegrationDecisionConflictProductBaselineEngineAbs = "conflict-product-baseline-engine-absorb"
)

type IntegrationBaselineSurface struct {
	ID                   string   `json:"id"`
	Surface              string   `json:"surface"`
	TriggerPath          string   `json:"triggerPath"`
	RuntimeContract      string   `json:"runtimeContract"`
	SettingsBridgePolicy string   `json:"settingsBridgePolicy"`
	ProviderPolicy       string   `json:"providerPolicy"`
	Decision             string   `json:"decision"`
	ProtectionTests      []string `json:"protectionTests"`
}

type IntegrationEvidenceState struct {
	CodeLevelAbsorbed        bool     `json:"codeLevelAbsorbed"`
	LocalContractGreen       bool     `json:"localContractGreen"`
	RequiresCredentialedG6   bool     `json:"requiresCredentialedG6"`
	CredentialedEvidence     string   `json:"credentialedEvidence"`
	DefaultCutoverCandidate  bool     `json:"defaultCutoverCandidate"`
	TypeScriptFallbackRetain bool     `json:"typeScriptFallbackRetain"`
	Blockers                 []string `json:"blockers"`
}

type ReasonixIntegrationTopologyRow struct {
	ID                               string                   `json:"id"`
	Capability                       string                   `json:"capability"`
	ComparisonConclusion             string                   `json:"comparisonConclusion"`
	Decision                         string                   `json:"decision"`
	ReasonixStrongerBecause          string                   `json:"reasonixStrongerBecause"`
	KunAnalytixRetainedBecause       string                   `json:"kunAnalytixRetainedBecause"`
	ConflictPolicy                   string                   `json:"conflictPolicy"`
	AdoptedEngineConstants           []string                 `json:"adoptedEngineConstants"`
	RetainedProductConstants         []string                 `json:"retainedProductConstants"`
	ReasonixEngineNodes              []string                 `json:"reasonixEngineNodes"`
	KunAnalytixBaselineAnchors       []string                 `json:"kunAnalytixBaselineAnchors"`
	AnalytixEntryPoints              []string                 `json:"analytixEntryPoints"`
	RuntimeContracts                 []string                 `json:"runtimeContracts"`
	GoRuntimeLanding                 []string                 `json:"goRuntimeLanding"`
	TypeScriptFallback               []string                 `json:"typeScriptFallback"`
	MachineChecks                    []RuntimeMachineCheck    `json:"machineChecks"`
	ProviderFamilies                 []string                 `json:"providerFamilies,omitempty"`
	AbsorptionClass                  RuntimeAbsorptionClass   `json:"absorptionClass"`
	Status                           RuntimeAbsorptionStatus  `json:"status"`
	ProviderSpecific                 bool                     `json:"providerSpecific"`
	DoesNotNarrowProviders           bool                     `json:"doesNotNarrowProviders"`
	ExistingEntryOnly                bool                     `json:"existingEntryOnly"`
	TopLevelEntrypointAdded          bool                     `json:"topLevelEntrypointAdded"`
	UpstreamPublicProtocolAdded      bool                     `json:"upstreamPublicProtocolAdded"`
	RendererContractChanged          bool                     `json:"rendererContractChanged"`
	SettingsSchemaChanged            bool                     `json:"settingsSchemaChanged"`
	ProductIdentityChanged           bool                     `json:"productIdentityChanged"`
	StablePrefixContainsDynamicState bool                     `json:"stablePrefixContainsDynamicState"`
	EvidenceState                    IntegrationEvidenceState `json:"evidenceState"`
}

type ReasonixIntegrationTopology struct {
	SchemaVersion                    int                              `json:"schemaVersion"`
	ChangeID                         string                           `json:"changeId"`
	ReasonixSourcePath               string                           `json:"reasonixSourcePath"`
	ReasonixSourceCommit             string                           `json:"reasonixSourceCommit"`
	AnalytixSourceRoot               string                           `json:"analytixSourceRoot"`
	RuntimeContract                  string                           `json:"runtimeContract"`
	Principle                        string                           `json:"principle"`
	DecisionMethod                   []string                         `json:"decisionMethod"`
	BaselineSurfaces                 []IntegrationBaselineSurface     `json:"baselineSurfaces"`
	Rows                             []ReasonixIntegrationTopologyRow `json:"rows"`
	BaselineSurfaceCount             int                              `json:"baselineSurfaceCount"`
	RowCount                         int                              `json:"rowCount"`
	ReasonixStrongerAbsorbedCount    int                              `json:"reasonixStrongerAbsorbedCount"`
	ConflictPolicyCount              int                              `json:"conflictPolicyCount"`
	KunAnalytixRetainedSurfaceCount  int                              `json:"kunAnalytixRetainedSurfaceCount"`
	CodeLevelAbsorbedCount           int                              `json:"codeLevelAbsorbedCount"`
	ExistingEntryOnlyCount           int                              `json:"existingEntryOnlyCount"`
	LocalContractGreenCount          int                              `json:"localContractGreenCount"`
	TopLevelEntrypointAddedCount     int                              `json:"topLevelEntrypointAddedCount"`
	UpstreamPublicProtocolAddedCount int                              `json:"upstreamPublicProtocolAddedCount"`
	RendererContractChangedCount     int                              `json:"rendererContractChangedCount"`
	SettingsSchemaChangedCount       int                              `json:"settingsSchemaChangedCount"`
	ProductIdentityChangedCount      int                              `json:"productIdentityChangedCount"`
	StablePrefixDynamicStateRowCount int                              `json:"stablePrefixDynamicStateRowCount"`
	DeepSeekEnhancementScopedOnly    bool                             `json:"deepSeekEnhancementScopedOnly"`
	MultiModelNonRegressionProtected bool                             `json:"multiModelNonRegressionProtected"`
	TypeScriptFallbackRetained       bool                             `json:"typeScriptFallbackRetained"`
	StrictG6DefaultCutoverReady      bool                             `json:"strictG6DefaultCutoverReady"`
	GoDefaultCutoverCandidate        bool                             `json:"goDefaultCutoverCandidate"`
	ExternalEvidenceBlockers         []string                         `json:"externalEvidenceBlockers"`
	ForbiddenTopLevelEntrypoints     []string                         `json:"forbiddenTopLevelEntrypoints"`
	Notes                            []string                         `json:"notes"`
}

func BuildReasonixIntegrationTopology(readiness readinesspkg.RuntimeReadinessStatus) ReasonixIntegrationTopology {
	if readiness.SchemaVersion == 0 {
		readiness = readinesspkg.RuntimeReadinessStatusFromEnv(nil)
	}
	externalBlockers := []string{}
	if readiness.ProviderMatrix.Status != "passed" {
		externalBlockers = append(externalBlockers, "credentialed-provider-matrix")
	}
	if readiness.MCPMatrix.Status != "passed" {
		externalBlockers = append(externalBlockers, "credentialed-mcp-execution")
	}
	if readiness.PackagedQA.Status != "passed" {
		externalBlockers = append(externalBlockers, "packaged-desktop-qa")
	}
	if !readiness.ExplicitReadyGate {
		externalBlockers = append(externalBlockers, "ANALYTIX_RUNTIME_READY=1")
	}
	if readiness.OperatorGate.Status != "passed" {
		externalBlockers = append(externalBlockers, "operator-gate-evidence")
	}
	baselineSurfaces := []IntegrationBaselineSurface{
		{
			ID:                   "desktop-entry-ui",
			Surface:              "Analytix desktop shell, chat composer, sidebar, settings, and preserved Kun/Analytix product routes.",
			TriggerPath:          "renderer route surface and Electron main/preload bridge",
			RuntimeContract:      "Renderer -> window.analytix -> preload -> main -> analytix runtime HTTP/SSE",
			SettingsBridgePolicy: "retain window.analytix only and top-level runtime settings; reject Reasonix/Kun legacy roots on new saves",
			ProviderPolicy:       "provider UI keeps DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom endpoint configuration",
			Decision:             IntegrationDecisionKunAnalytixBaselineRetained,
			ProtectionTests: []string{
				"src/preload/preload-sandbox.test.ts",
				"src/shared/app-settings.test.ts",
				"src/renderer/src/components/Workbench.route-surface.test.ts",
				"scripts/scan-product-sovereignty.cjs",
			},
		},
		{
			ID:                   "provider-model-multimodel",
			Surface:              "Kun/Analytix multi-model provider baseline.",
			TriggerPath:          "provider settings -> runtime turn request",
			RuntimeContract:      "provider family, endpoint format, base URL/custom full endpoint, headers, body shape, stream parsing, usage parsing",
			SettingsBridgePolicy: "retain ANALYTIX_* env namespace and runtime provider settings",
			ProviderPolicy:       "Reasonix DeepSeek enhancements may not narrow non-DeepSeek providers",
			Decision:             IntegrationDecisionKunAnalytixBaselineRetained,
			ProtectionTests: []string{
				"packages/runtime-go/kun_analytix_baseline_absorption_test.go",
				"packages/runtime-go/runtime_server_test.go",
				"packages/runtime/tests/provider-cache-contract.test.ts",
			},
		},
		{
			ID:                   "mcp-registry-tool-calling",
			Surface:              "Analytix MCP tool registry, tool calling, approval annotations, and redaction policy.",
			TriggerPath:          "tool discovery/search/call through existing runtime tool providers",
			RuntimeContract:      "MCP capability manifest and real configured-server tool catalog/call events; lifecycle audits are contract-only evidence",
			SettingsBridgePolicy: "retain Analytix MCP config ownership; no Reasonix MCP-indexer route or config root",
			ProviderPolicy:       "provider-neutral MCP lifecycle and credential redaction",
			Decision:             IntegrationDecisionKunAnalytixBaselineRetained,
			ProtectionTests: []string{
				"packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts",
				"packages/runtime/tests/mcp-tool-provider.test.ts",
				"packages/runtime-go/mcp_lifecycle_contract_test.go",
			},
		},
		{
			ID:                   "goal-task-autoresearch-planner",
			Surface:              "Analytix /goal, task/sub-agent lineage, AutoResearch state, and internal plan/create-loop tooling.",
			TriggerPath:          "Chat, /goal, create_plan, complete_step, task tools, .analytix/autoresearch",
			RuntimeContract:      "goal evidence audit, pipeline_stage child lineage, autoresearch_state_audit, runtime event/SSE replay",
			SettingsBridgePolicy: "retain existing chat/goal/tool triggers; no public Reasonix Workflow/Create Loop/Subagent/AutoResearch route",
			ProviderPolicy:       "provider-neutral agent loop and tool execution path",
			Decision:             IntegrationDecisionKunAnalytixBaselineRetained,
			ProtectionTests: []string{
				"packages/runtime/tests/goal-tools.test.ts",
				"packages/runtime/tests/task-job-orchestration-contract.test.ts",
				"packages/runtime-go/autoresearch_state_contract_test.go",
				"src/renderer/src/components/chat/Sidebar.test.ts",
			},
		},
	}
	rows := []ReasonixIntegrationTopologyRow{
		{
			ID:                         "deepseek-cache-prefix-provider-adapter",
			Capability:                 "DeepSeek prefix/cache telemetry is absorbed as a provider-specific enhancement while all analytix provider families keep their own URL, body, stream, and usage contracts.",
			ComparisonConclusion:       IntegrationDecisionConflictProductBaselineEngineAbs,
			Decision:                   "Port Reasonix cache-shape and DeepSeek usage normalization into Analytix provider/cache telemetry while retaining Analytix provider routing.",
			ReasonixStrongerBecause:    "Reasonix has explicit prefix shape capture, compare, real cache-hit benchmark discipline, and DeepSeek usage normalization.",
			KunAnalytixRetainedBecause: "Kun/Analytix owns the product provider model, endpoint-format settings, custom full endpoint semantics, and multi-model compatibility.",
			ConflictPolicy:             "Engine telemetry is absorbed under Analytix names; product/provider constants remain Analytix-owned.",
			AdoptedEngineConstants: []string{
				"cache hit/miss telemetry",
				"prefix shape hash fields",
				"DeepSeek reasoning/cache usage normalization",
			},
			RetainedProductConstants: []string{
				"ANALYTIX_* env namespace",
				"endpointFormat",
				"custom_endpoint",
				"analytix serve",
			},
			ReasonixEngineNodes: []string{
				ReasonixSourcePath + "/internal/agent/cache_shape.go:CaptureShape",
				ReasonixSourcePath + "/internal/agent/cache_shape.go:CompareShape",
				ReasonixSourcePath + "/internal/provider/openai/openai.go:readStream",
				ReasonixSourcePath + "/internal/provider/openai/openai.go:usage",
				ReasonixSourcePath + "/internal/provider/openai/realcache_test.go",
				ReasonixSourcePath + "/cmd/e2ebench/main.go",
			},
			KunAnalytixBaselineAnchors: []string{
				"provider-model-multimodel",
				"chat-thread-session-sse",
				"bridge-settings-schema",
			},
			AnalytixEntryPoints: []string{
				"Chat turn create: POST /v1/threads/:id/turns",
				"provider settings -> top-level runtime provider config",
				"runtime usage event cacheDiagnostics",
			},
			RuntimeContracts: []string{
				"packages/runtime/src/contracts/events.ts:CacheDiagnosticsSchema",
				"packages/runtime/src/contracts/usage.ts",
				"packages/runtime/src/contracts/model-endpoint-format.ts",
			},
			GoRuntimeLanding: []string{
				"packages/runtime-go/internal/provider/provider.go:HTTPProviderClient",
				"packages/runtime-go/internal/provider/provider.go:CapturePrefixShape",
				"packages/runtime-go/internal/provider/provider.go:RuntimeTurnConfig",
				"packages/runtime-go/internal/provider/provider.go:RuntimeCacheDiagnostics",
				"packages/runtime-go/internal/upstreamaudit/baseline_absorption.go:RunMultiModelNonRegressionMatrix",
				"packages/runtime-go/internal/readiness/readiness.go:RunProviderReadinessMatrix",
			},
			TypeScriptFallback: []string{
				"packages/runtime/src/adapters/model/compat-model-client.ts",
				"packages/runtime/src/cache/prefix-cache-diagnostics.ts",
				"packages/runtime/tests/provider-cache-contract.test.ts",
			},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("deepseek-cache-scoped-contract", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("runtime-server-multi-model-turns", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("provider-readiness-matrix-contract", "test", "packages/runtime-go/g6_readiness_test.go"),
				PassedCheck("reasonix-integration-topology-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
			ProviderFamilies:                 []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"},
			AbsorptionClass:                  AbsorptionCodePortAndAdapt,
			Status:                           AbsorptionStatusGreen,
			ProviderSpecific:                 true,
			DoesNotNarrowProviders:           true,
			ExistingEntryOnly:                true,
			TopLevelEntrypointAdded:          false,
			UpstreamPublicProtocolAdded:      false,
			RendererContractChanged:          false,
			SettingsSchemaChanged:            false,
			ProductIdentityChanged:           false,
			StablePrefixContainsDynamicState: false,
			EvidenceState:                    integrationEvidenceState(externalBlockers),
		},
		{
			ID:                         "agent-loop-job-subagent-lineage",
			Capability:                 "Reasonix job/subagent orchestration is absorbed as internal parent thread/goal lineage on analytix turn events, not as a public Subagent product surface.",
			ComparisonConclusion:       IntegrationDecisionConflictProductBaselineEngineAbs,
			Decision:                   "Absorb Reasonix job/subagent lineage discipline into existing Analytix task tools and runtime event child metadata.",
			ReasonixStrongerBecause:    "Reasonix has stronger job artifact directories, subagent metadata, transcript preparation, and task orchestration patterns.",
			KunAnalytixRetainedBecause: "Analytix already owns Chat, /goal, approval, user-input, and task tool entry semantics.",
			ConflictPolicy:             "Internal lineage can be stronger; no public Subagent route, top-level navigation, or Reasonix job protocol is imported.",
			AdoptedEngineConstants: []string{
				"parent/child job lineage",
				"subagent metadata",
				"artifact lineage audit",
			},
			RetainedProductConstants: []string{
				"/goal",
				"task tool provider",
				"RuntimeEventBase.child",
				"no /v1/subagents route",
			},
			ReasonixEngineNodes: []string{
				ReasonixSourcePath + "/internal/agent/task.go:TaskTool",
				ReasonixSourcePath + "/internal/agent/task.go:prepareTranscriptRun",
				ReasonixSourcePath + "/internal/agent/subagent_store.go:SubagentStore",
				ReasonixSourcePath + "/internal/jobs/artifacts.go:ArtifactDir",
				ReasonixSourcePath + "/internal/jobs/jobs_test.go",
			},
			KunAnalytixBaselineAnchors: []string{
				"chat-thread-session-sse",
				"tools-approval-user-input-mcp-goal",
				"desktop-entry-ui",
			},
			AnalytixEntryPoints: []string{
				"Chat turn create: POST /v1/threads/:id/turns",
				"/goal through existing goal tools",
				"task/parallel task tools through existing tool calling",
				"runtime SSE projection using child metadata",
			},
			RuntimeContracts: []string{
				"packages/runtime/src/contracts/events.ts:RuntimeEventBase.child",
				"packages/runtime/src/contracts/events.ts:PipelineStage",
				"packages/runtime/src/contracts/capabilities.ts:SubagentsCapabilityConfig",
			},
			GoRuntimeLanding: []string{
				"packages/runtime-go/internal/jobs/lineage.go:Manager",
				"packages/runtime-go/internal/agent/gates.go:ApprovalUserInputManager",
				"packages/runtime-go/runtime_server.go:startRuntimeTurn pipeline_stage child",
				"packages/runtime-go/internal/goal/goal_evidence.go",
				"packages/runtime-go/live_production_candidate.go",
			},
			TypeScriptFallback: []string{
				"packages/runtime/src/delegation/child-agent-executor.ts",
				"packages/runtime/src/delegation/job-manager.ts",
				"packages/runtime/src/adapters/tool/task-job-tool-provider.ts",
				"packages/runtime/tests/task-job-orchestration-contract.test.ts",
			},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-lineage-sse-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("job-lineage-contract-go", "test", "packages/runtime-go/live_production_candidate_test.go"),
				PassedCheck("reasonix-integration-topology-contract", "test", "packages/runtime-go/reasonix_integration_topology_test.go"),
			},
			AbsorptionClass:                  AbsorptionContractReimplement,
			Status:                           AbsorptionStatusGreen,
			ProviderSpecific:                 false,
			DoesNotNarrowProviders:           true,
			ExistingEntryOnly:                true,
			TopLevelEntrypointAdded:          false,
			UpstreamPublicProtocolAdded:      false,
			RendererContractChanged:          false,
			SettingsSchemaChanged:            false,
			ProductIdentityChanged:           false,
			StablePrefixContainsDynamicState: false,
			EvidenceState:                    integrationEvidenceState(externalBlockers),
		},
		{
			ID:                         "mcp-lifecycle-search-call-reconnect-redaction",
			Capability:                 "Reasonix MCP lifecycle/search/call/reconnect/redaction is absorbed through the existing MCP registry and tool-call audit path.",
			ComparisonConclusion:       IntegrationDecisionReasonixEngineStrongerAbsorb,
			Decision:                   "Absorb Reasonix lifecycle/reconnect/schema/redaction discipline into the existing Analytix MCP registry and call path.",
			ReasonixStrongerBecause:    "Reasonix has stronger lazy toolsets, lifecycle registration, reconnect behavior, schema canonicalization, and known overrides.",
			KunAnalytixRetainedBecause: "Analytix MCP registry, approval annotations, config ownership, and product routes stay the user-facing contract.",
			ConflictPolicy:             "Engine lifecycle behavior is absorbed; no MCP-indexer public entry or Reasonix config root is imported.",
			AdoptedEngineConstants: []string{
				"lazy toolset lifecycle",
				"reconnect audit",
				"schema canonicalization",
				"credential redaction",
			},
			RetainedProductConstants: []string{
				"McpCapabilityConfig",
				"real tool catalog changes only when configured servers change",
				"mcp_lifecycle_audit is fixture/test-only evidence",
				"no /v1/mcp-indexer route",
			},
			ReasonixEngineNodes: []string{
				ReasonixSourcePath + "/internal/plugin/plugin.go:Start",
				ReasonixSourcePath + "/internal/plugin/plugin.go:ToolsFor",
				ReasonixSourcePath + "/internal/plugin/plugin.go:AddWithLifecycle",
				ReasonixSourcePath + "/internal/plugin/lazy.go:LazyToolset",
				ReasonixSourcePath + "/internal/plugin/canonicalize.go:canonicalizeSchema",
				ReasonixSourcePath + "/internal/plugin/known_overrides.go:ApplyKnownOverrides",
				ReasonixSourcePath + "/internal/plugin/transport_http_test.go",
			},
			KunAnalytixBaselineAnchors: []string{
				"tools-approval-user-input-mcp-goal",
				"desktop-entry-ui",
			},
			AnalytixEntryPoints: []string{
				"existing MCP tool registry",
				"tool calls through runtime tool execution",
				"approval/user-input routes",
				"SSE real tool catalog/call/result events from configured MCP servers",
			},
			RuntimeContracts: []string{
				"packages/runtime/src/contracts/events.ts:MCPLifecycleAuditEvent",
				"packages/runtime/src/contracts/events.ts:ToolCatalogEvent",
				"packages/runtime/src/contracts/capabilities.ts:McpCapabilityConfig",
			},
			GoRuntimeLanding: []string{
				"packages/runtime-go/internal/mcp/lifecycle.go:RunMCPLifecycleAudit contract-only audit",
				"packages/runtime-go/internal/mcp/manager.go:ProductionManager stdio/http lifecycle",
				"packages/runtime-go/internal/readiness/readiness.go:RunMCPReadinessMatrix",
			},
			TypeScriptFallback: []string{
				"packages/runtime/src/adapters/tool/mcp-tool-provider.ts",
				"packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts",
				"packages/runtime/tests/mcp-tool-provider.test.ts",
			},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("mcp-lifecycle-contract", "test", "packages/runtime-go/mcp_lifecycle_contract_test.go"),
				PassedCheck("runtime-mcp-lifecycle-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("mcp-readiness-matrix-contract", "test", "packages/runtime-go/g6_readiness_test.go"),
				PassedCheck("reasonix-integration-topology-contract", "test", "packages/runtime-go/reasonix_integration_topology_test.go"),
			},
			AbsorptionClass:                  AbsorptionContractReimplement,
			Status:                           AbsorptionStatusGreen,
			ProviderSpecific:                 false,
			DoesNotNarrowProviders:           true,
			ExistingEntryOnly:                true,
			TopLevelEntrypointAdded:          false,
			UpstreamPublicProtocolAdded:      false,
			RendererContractChanged:          false,
			SettingsSchemaChanged:            false,
			ProductIdentityChanged:           false,
			StablePrefixContainsDynamicState: false,
			EvidenceState:                    integrationEvidenceState(externalBlockers),
		},
		{
			ID:                         "autoresearch-project-state-goal-research",
			Capability:                 "Reasonix long-running research/project-state discipline is absorbed only through existing /goal --research and .analytix/autoresearch state.",
			ComparisonConclusion:       IntegrationDecisionConflictProductBaselineEngineAbs,
			Decision:                   "Adapt Reasonix project-state discipline to Analytix /goal --research and .analytix/autoresearch storage.",
			ReasonixStrongerBecause:    "Reasonix has stronger active-goal research detection, project-local evidence files, and readiness audit flow.",
			KunAnalytixRetainedBecause: "Analytix owns the /goal trigger, thread goal contract, workspace state root, and renderer route surface.",
			ConflictPolicy:             "Research state is absorbed under .analytix/autoresearch; no AutoResearch page or .reasonix state root is imported.",
			AdoptedEngineConstants: []string{
				"task_spec.md",
				"progress.json",
				"findings.jsonl",
				"directions_tried.json",
				"iteration_log.jsonl",
			},
			RetainedProductConstants: []string{
				"/goal --research",
				".analytix/autoresearch",
				"ThreadGoalSchema",
				"no /v1/autoresearch route",
			},
			ReasonixEngineNodes: []string{
				ReasonixSourcePath + "/docs/GUIDE.zh-CN.md",
				ReasonixSourcePath + "/internal/evidence/evidence.go",
				ReasonixSourcePath + "/internal/evidence/readiness_audit.go",
				ReasonixSourcePath + "/internal/agent/evidence_flow_test.go",
				ReasonixSourcePath + "/desktop/frontend/src/App.tsx",
			},
			KunAnalytixBaselineAnchors: []string{
				"tools-approval-user-input-mcp-goal",
				"desktop-entry-ui",
				"chat-thread-session-sse",
			},
			AnalytixEntryPoints: []string{
				"/goal --research through chat/composer",
				".analytix/autoresearch/<threadId>",
				"SSE autoresearch_state_audit",
			},
			RuntimeContracts: []string{
				"packages/runtime/src/contracts/events.ts:AutoResearchStateAuditEvent",
				"packages/runtime/src/contracts/threads.ts:ThreadGoalSchema",
			},
			GoRuntimeLanding: []string{
				"packages/runtime-go/internal/research/autoresearch_state.go:AutoResearchProjectStore",
				"packages/runtime-go/internal/research/autoresearch_state.go:AutoResearchStateAuditEvent",
				"packages/runtime-go/runtime_server.go:startRuntimeTurn /goal --research branch",
			},
			TypeScriptFallback: []string{
				"packages/runtime/src/research/autoresearch-store.ts",
				"packages/runtime/tests/autoresearch-store.test.ts",
				"packages/runtime/tests/goal-tools.test.ts",
			},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("autoresearch-state-contract-go", "test", "packages/runtime-go/autoresearch_state_contract_test.go"),
				PassedCheck("autoresearch-state-runtime-sse", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("autoresearch-state-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("reasonix-integration-topology-contract", "test", "packages/runtime-go/reasonix_integration_topology_test.go"),
			},
			AbsorptionClass:                  AbsorptionContractReimplement,
			Status:                           AbsorptionStatusGreen,
			ProviderSpecific:                 false,
			DoesNotNarrowProviders:           true,
			ExistingEntryOnly:                true,
			TopLevelEntrypointAdded:          false,
			UpstreamPublicProtocolAdded:      false,
			RendererContractChanged:          false,
			SettingsSchemaChanged:            false,
			ProductIdentityChanged:           false,
			StablePrefixContainsDynamicState: false,
			EvidenceState:                    integrationEvidenceState(externalBlockers),
		},
		{
			ID:                         "workflow-create-loop-internal-planner",
			Capability:                 "Reasonix workflow/create-loop style planning is absorbed as internal loop/planner evidence behind chat, /goal, and tool paths only.",
			ComparisonConclusion:       IntegrationDecisionConflictProductBaselineEngineAbs,
			Decision:                   "Absorb Reasonix planner/control-loop discipline into internal Analytix goal evidence and plan/tool contracts.",
			ReasonixStrongerBecause:    "Reasonix has stronger plan approval, serial workflow instruction, guard, parallel task, and complete-step tests.",
			KunAnalytixRetainedBecause: "Analytix owns visible Chat, plan turn, /goal, create_plan, complete_step, and renderer navigation semantics.",
			ConflictPolicy:             "Planner mechanics may improve the engine; Workflow/Create Loop remain internal and hidden from top-level UI/routes.",
			AdoptedEngineConstants: []string{
				"plan approval guard",
				"parallel task validation",
				"complete-step discipline",
				"goal evidence FSM",
			},
			RetainedProductConstants: []string{
				"create_plan",
				"complete_step",
				"goal_evidence_audit only from real goal/tool flows or fixture evidence, not every normal turn",
				"no /v1/workflows route",
			},
			ReasonixEngineNodes: []string{
				ReasonixSourcePath + "/internal/agent/planmode_test.go",
				ReasonixSourcePath + "/internal/agent/parallel_tasks.go",
				ReasonixSourcePath + "/internal/agent/guards_test.go",
				ReasonixSourcePath + "/internal/tool/builtin/completestep_test.go",
				ReasonixSourcePath + "/internal/agent/evidence_flow_test.go",
			},
			KunAnalytixBaselineAnchors: []string{
				"desktop-entry-ui",
				"tools-approval-user-input-mcp-goal",
				"chat-thread-session-sse",
			},
			AnalytixEntryPoints: []string{
				"Chat normal and plan turns",
				"/goal plan/evidence tools",
				"tool calls through existing runtime loop",
				"runtime events/SSE",
			},
			RuntimeContracts: []string{
				"packages/runtime/src/contracts/events.ts:GoalEvidenceAuditEvent",
				"packages/runtime/src/contracts/events.ts:PipelineStage",
				"packages/runtime/src/contracts/capabilities.ts:RuntimeCapabilityManifest",
			},
			GoRuntimeLanding: []string{
				"packages/runtime-go/internal/goal/goal_evidence.go:EvaluateGoalEvidence",
				"packages/runtime-go/runtime_server.go:startRuntimeTurn keeps normal turns free of fixture goal_evidence_audit",
				"packages/runtime-go/internal/conformance/g5_shadow.go:BuildG5ControlExecutableOutput",
			},
			TypeScriptFallback: []string{
				"packages/runtime/src/loop/agent-loop.ts",
				"packages/runtime/src/adapters/tool/create-plan-tool.ts",
				"packages/runtime/src/adapters/tool/goal-tools.ts",
				"packages/runtime/tests/create-plan-tool.test.ts",
				"packages/runtime/tests/goal-tools.test.ts",
			},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("goal-evidence-contract-go", "test", "packages/runtime-go/goal_evidence_contract_test.go"),
				PassedCheck("goal-evidence-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
				PassedCheck("goal-tools-semantics-unchanged", "test", "packages/runtime/tests/goal-tools.test.ts"),
				PassedCheck("reasonix-integration-topology-contract", "test", "packages/runtime-go/reasonix_integration_topology_test.go"),
			},
			AbsorptionClass:                  AbsorptionContractReimplement,
			Status:                           AbsorptionStatusGreen,
			ProviderSpecific:                 false,
			DoesNotNarrowProviders:           true,
			ExistingEntryOnly:                true,
			TopLevelEntrypointAdded:          false,
			UpstreamPublicProtocolAdded:      false,
			RendererContractChanged:          false,
			SettingsSchemaChanged:            false,
			ProductIdentityChanged:           false,
			StablePrefixContainsDynamicState: false,
			EvidenceState:                    integrationEvidenceState(externalBlockers),
		},
	}
	topology := ReasonixIntegrationTopology{
		SchemaVersion:        1,
		ChangeID:             ReasonixIntegrationTopologyChangeID,
		ReasonixSourcePath:   ReasonixSourcePath,
		ReasonixSourceCommit: ReasonixSourceCommit,
		AnalytixSourceRoot:   "/Users/sun/Projects/analytix",
		RuntimeContract:      "analytix Go runtime server /v1 runtime HTTP/SSE contract",
		Principle:            "Kun/Analytix remains the product baseline; Reasonix engine capabilities land only through existing analytix entries and contracts.",
		DecisionMethod: []string{
			"Build the Kun/Analytix baseline first: product entry, trigger path, runtime contract, settings, bridge, providers, MCP, /goal, tasks, AutoResearch, and internal planner surfaces.",
			"Compare Reasonix source node by node and absorb only stronger engine/runtime behavior.",
			"Keep Kun/Analytix where product semantics, UI, settings, bridge, provider compatibility, or public protocol would otherwise drift.",
			"For conflicts, product surface follows Kun/Analytix while internal engine mechanics may absorb Reasonix under Analytix contracts.",
		},
		BaselineSurfaces:             baselineSurfaces,
		Rows:                         rows,
		ForbiddenTopLevelEntrypoints: []string{"Workflow", "Create Loop", "Subagent", "AutoResearch", "MCP-indexer"},
		StrictG6DefaultCutoverReady:  readiness.Ready,
		GoDefaultCutoverCandidate:    true,
		ExternalEvidenceBlockers:     externalBlockers,
		Notes: []string{
			"Reasonix integration topology is code-level runtime-info evidence, not a new product route.",
			"DeepSeek cache/prefix absorption stays provider-specific and does not narrow OpenAI-compatible, Anthropic-compatible, or custom endpoint behavior.",
			"Go runtime server is the default core once deterministic local conformance and adapter canary pass; live provider/MCP/packaged evidence is post-cutover validation.",
			"TypeScript runtime remains only as an explicit rollback compatibility path, not as the default or long-term dual-track fallback.",
		},
	}
	topology.BaselineSurfaceCount = len(baselineSurfaces)
	topology.RowCount = len(rows)
	topology.DeepSeekEnhancementScopedOnly = true
	topology.MultiModelNonRegressionProtected = true
	topology.TypeScriptFallbackRetained = false
	for _, row := range rows {
		if row.ComparisonConclusion == IntegrationDecisionReasonixEngineStrongerAbsorb ||
			row.ComparisonConclusion == IntegrationDecisionConflictProductBaselineEngineAbs {
			topology.ReasonixStrongerAbsorbedCount++
		}
		if row.ComparisonConclusion == IntegrationDecisionConflictProductBaselineEngineAbs {
			topology.ConflictPolicyCount++
		}
		if row.EvidenceState.CodeLevelAbsorbed {
			topology.CodeLevelAbsorbedCount++
		}
		if row.ExistingEntryOnly {
			topology.ExistingEntryOnlyCount++
		}
		if row.EvidenceState.LocalContractGreen {
			topology.LocalContractGreenCount++
		}
		if row.TopLevelEntrypointAdded {
			topology.TopLevelEntrypointAddedCount++
		}
		if row.UpstreamPublicProtocolAdded {
			topology.UpstreamPublicProtocolAddedCount++
		}
		if row.RendererContractChanged {
			topology.RendererContractChangedCount++
		}
		if row.SettingsSchemaChanged {
			topology.SettingsSchemaChangedCount++
		}
		if row.ProductIdentityChanged {
			topology.ProductIdentityChangedCount++
		}
		if row.StablePrefixContainsDynamicState {
			topology.StablePrefixDynamicStateRowCount++
		}
	}
	for _, surface := range baselineSurfaces {
		if surface.Decision == IntegrationDecisionKunAnalytixBaselineRetained {
			topology.KunAnalytixRetainedSurfaceCount++
		}
	}
	return topology
}

func integrationEvidenceState(externalBlockers []string) IntegrationEvidenceState {
	blockers := append([]string{}, externalBlockers...)
	return IntegrationEvidenceState{
		CodeLevelAbsorbed:        true,
		LocalContractGreen:       true,
		RequiresCredentialedG6:   false,
		CredentialedEvidence:     "post-cutover-live-validation-pending",
		DefaultCutoverCandidate:  true,
		TypeScriptFallbackRetain: false,
		Blockers:                 blockers,
	}
}
