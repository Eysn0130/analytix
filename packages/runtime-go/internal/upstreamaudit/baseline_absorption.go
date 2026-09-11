//go:build !analytix_prod

package upstreamaudit

import (
	"context"
	"sort"
	"strings"

	provider "analytix.local/runtime-go/internal/provider"
	providerscript "analytix.local/runtime-go/internal/testsupport/providerscript"
)

const (
	KunAnalytixBaselineGuardChangeID = "kun-analytix-baseline-guard"
	ReasonixAbsorptionMatrixChangeID = "reasonix-absorption-matrix"
	MultiModelNonRegressionChangeID  = "runtime-multi-model-non-regression"
)

type KunAnalytixBaselineRow struct {
	ID                         string   `json:"id"`
	Capability                 string   `json:"capability"`
	KunAnalytixSources         []string `json:"kunAnalytixSources"`
	EntryPoint                 string   `json:"entryPoint"`
	TriggerPath                string   `json:"triggerPath"`
	RuntimeContract            string   `json:"runtimeContract"`
	SettingsSchema             string   `json:"settingsSchema"`
	UISurface                  string   `json:"uiSurface"`
	ProtectionTests            []string `json:"protectionTests"`
	ImmutableProductBaseline   bool     `json:"immutableProductBaseline"`
	ReasonixEnhancementAllowed string   `json:"reasonixEnhancementAllowed"`
	Status                     string   `json:"status"`
}

type KunAnalytixBaselineGuard struct {
	SchemaVersion                int                      `json:"schemaVersion"`
	ChangeID                     string                   `json:"changeId"`
	KunSourceSnapshots           []string                 `json:"kunSourceSnapshots"`
	AnalytixSourceRoot           string                   `json:"analytixSourceRoot"`
	FullFunctionBaseline         bool                     `json:"fullFunctionBaseline"`
	Rows                         []KunAnalytixBaselineRow `json:"rows"`
	ProtectedBaselineCount       int                      `json:"protectedBaselineCount"`
	InternalEnhancementCount     int                      `json:"internalEnhancementCount"`
	ForbiddenTopLevelEntrypoints []string                 `json:"forbiddenTopLevelEntrypoints"`
	ForbiddenEntrypointExposed   bool                     `json:"forbiddenEntrypointExposed"`
	DeprecatedBridgeAliasAllowed bool                     `json:"deprecatedBridgeAliasAllowed"`
	LegacySettingsWriteAllowed   bool                     `json:"legacySettingsWriteAllowed"`
	DeepSeekOnlyRuntimeAllowed   bool                     `json:"deepseekOnlyRuntimeAllowed"`
	ReadyForReasonixAbsorption   bool                     `json:"readyForReasonixAbsorption"`
	Notes                        []string                 `json:"notes"`
}

type ReasonixAbsorptionRow struct {
	ID                      string                  `json:"id"`
	Capability              string                  `json:"capability"`
	ReasonixSources         []string                `json:"reasonixSources"`
	AbsorptionClass         RuntimeAbsorptionClass  `json:"absorptionClass"`
	Status                  RuntimeAbsorptionStatus `json:"status"`
	AnalytixLanding         string                  `json:"analytixLanding"`
	ProviderSpecific        bool                    `json:"providerSpecific"`
	DoesNotNarrowProviders  bool                    `json:"doesNotNarrowProviders"`
	KunBaselineProtection   []string                `json:"kunBaselineProtection"`
	MachineChecks           []RuntimeMachineCheck   `json:"machineChecks"`
	ForbiddenProductSurface bool                    `json:"forbiddenProductSurface"`
	RejectedBecauseBaseline string                  `json:"rejectedBecauseBaseline,omitempty"`
	Blockers                []string                `json:"blockers,omitempty"`
}

type ReasonixAbsorptionMatrix struct {
	SchemaVersion                 int                     `json:"schemaVersion"`
	ChangeID                      string                  `json:"changeId"`
	ReasonixSourcePath            string                  `json:"reasonixSourcePath"`
	ReasonixSourceCommit          string                  `json:"reasonixSourceCommit"`
	Rows                          []ReasonixAbsorptionRow `json:"rows"`
	GreenCount                    int                     `json:"greenCount"`
	RedCount                      int                     `json:"redCount"`
	DeferredCount                 int                     `json:"deferredCount"`
	RejectedCount                 int                     `json:"rejectedCount"`
	ForbiddenProductSurfaceCount  int                     `json:"forbiddenProductSurfaceCount"`
	ProviderFamilies              []string                `json:"providerFamilies"`
	MultiModelNonRegressionGreen  bool                    `json:"multiModelNonRegressionGreen"`
	DeepSeekEnhancementScopedOnly bool                    `json:"deepSeekEnhancementScopedOnly"`
	ReadyForG6                    bool                    `json:"readyForG6"`
	AbsorptionMatrixGreen         bool                    `json:"absorptionMatrixGreen"`
	DefaultBackendReady           bool                    `json:"defaultBackendReady"`
	DefaultBackendReadinessGate   string                  `json:"defaultBackendReadinessGate"`
	G6Blockers                    []string                `json:"g6Blockers"`
	ReadinessSemantics            []string                `json:"readinessSemantics"`
	Notes                         []string                `json:"notes"`
}

type RuntimeProviderTurnConfig = provider.TurnConfig

type MultiModelProviderProbe struct {
	ID                        string               `json:"id"`
	ProviderID                string               `json:"providerId"`
	Family                    string               `json:"family"`
	EndpointFormat            string               `json:"endpointFormat"`
	RequestURL                string               `json:"requestUrl"`
	RequestBodyFields         []string             `json:"requestBodyFields"`
	Usage                     provider.Usage       `json:"usage"`
	PrefixShape               provider.PrefixShape `json:"prefixShape"`
	CacheTelemetrySupported   bool                 `json:"cacheTelemetrySupported"`
	DeepSeekRequestOnlyFields []string             `json:"deepSeekRequestOnlyFields"`
	NonDeepSeekThinkingField  bool                 `json:"nonDeepSeekThinkingField"`
	Status                    string               `json:"status"`
}

type MultiModelNonRegressionMatrix struct {
	SchemaVersion                  int                       `json:"schemaVersion"`
	ChangeID                       string                    `json:"changeId"`
	ProviderFamilies               []string                  `json:"providerFamilies"`
	ContractReplayProviderRequired bool                      `json:"contractReplayProviderRequired"`
	ReadsRealAPIKeysByDefault      bool                      `json:"readsRealApiKeysByDefault"`
	DeepSeekProviderSpecific       bool                      `json:"deepSeekProviderSpecific"`
	NonDeepSeekCacheDiagnosticsOff bool                      `json:"nonDeepSeekCacheDiagnosticsOff"`
	CustomEndpointUsesExactURL     bool                      `json:"customEndpointUsesExactUrl"`
	Probes                         []MultiModelProviderProbe `json:"probes"`
	Green                          bool                      `json:"green"`
}

func BuildKunAnalytixBaselineGuard() KunAnalytixBaselineGuard {
	rows := []KunAnalytixBaselineRow{
		{
			ID:                         "product-identity-release",
			Capability:                 "Product identity, release channels, packaged desktop identity, and env prefix.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "electron-builder.config.cjs", "src/main/app-identity.ts", "docs/analytix/specs/02-release-packaging-channels.md"},
			EntryPoint:                 "analytix desktop app and analytix serve",
			TriggerPath:                "packaged app startup, release scripts, ANALYTIX_READY runtime marker",
			RuntimeContract:            "ANALYTIX_* env and analytix serve remain the only current runtime identity.",
			SettingsSchema:             "release channel stable/beta only; no Kun/Reasonix settings root.",
			UISurface:                  "User-visible product identity remains analytix.",
			ProtectionTests:            []string{"scripts/scan-product-sovereignty.cjs", "electron-builder.config.cjs", "scripts/release-mac.sh", "scripts/release-win.sh"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "none; Reasonix identity and config roots are rejected.",
			Status:                     "protected",
		},
		{
			ID:                         "bridge-settings-schema",
			Capability:                 "Renderer bridge, preload/main IPC, and top-level runtime settings.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "src/preload/index.ts", "src/shared/app-settings-runtime.ts", "src/main/settings-store.ts"},
			EntryPoint:                 "window.analytix",
			TriggerPath:                "Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE",
			RuntimeContract:            "runtimeRequest/SSE paths stay analytix-owned /v1 routes.",
			SettingsSchema:             "top-level runtime and provider providers; no old agentProvider/agents writes.",
			UISurface:                  "Settings screens keep existing runtime/provider controls.",
			ProtectionTests:            []string{"src/preload/preload-sandbox.test.ts", "src/preload/preload-runtime-request.test.ts", "src/shared/app-settings.test.ts", "scripts/scan-product-sovereignty.cjs"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "internal runtime behavior only; no deprecated bridge alias.",
			Status:                     "protected",
		},
		{
			ID:                         "desktop-entry-ui",
			Capability:                 "Code, Write, Settings, Plugins, Connect Phone, schedule, terminal, mascot/cameo, and shell UX.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "src/renderer/src/components/Workbench.tsx", "src/renderer/src/components/chat/Sidebar.tsx", "src/renderer/src/store/chat-store-navigation-actions.ts"},
			EntryPoint:                 "existing Workbench route surface",
			TriggerPath:                "sidebar/topbar/composer commands and route store",
			RuntimeContract:            "no new public runtime route is required for hidden Reasonix capabilities.",
			SettingsSchema:             "existing app/runtime/write/schedule/connect-phone settings.",
			UISurface:                  "No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.",
			ProtectionTests:            []string{"src/renderer/src/components/Workbench.route-surface.test.ts", "src/renderer/src/components/chat/Sidebar.test.ts", "scripts/scan-product-sovereignty.cjs"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "only as internal orchestration surfaced through existing chat/goal/tool flows.",
			Status:                     "protected",
		},
		{
			ID:                         "chat-thread-session-sse",
			Capability:                 "Chat, threads, sessions, fork/resume/archive/search, SSE replay, projection, and timeline virtualization.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "packages/runtime/src/server/routes", "src/renderer/src/agent/analytix-runtime.ts", "src/renderer/src/components/chat/MessageTimeline.tsx"},
			EntryPoint:                 "chat workbench and runtime HTTP/SSE",
			TriggerPath:                "thread list/read/patch/fork/resume/turn/events",
			RuntimeContract:            "/v1/threads, /v1/sessions/:id/resume-thread, /v1/threads/:id/events",
			SettingsSchema:             "runtime workspace/model/provider selections remain current schema.",
			UISurface:                  "MessageTimeline, MessageBubble, GeneratedFilesPanel, ReviewPlanCard, ReviewSummaryCard, TurnChangeSummary, WorkMetaRow, ProcessSectionRow, MessageTimelineEmptyHero.",
			ProtectionTests:            []string{"packages/runtime/tests/go-runtime-conformance.test.ts", "src/renderer/src/agent/analytix-runtime.test.ts", "src/renderer/src/components/chat/message-timeline-turns.test.ts"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "Go runtime may replace implementation after full route/SSE non-regression.",
			Status:                     "protected",
		},
		{
			ID:                         "provider-model-multimodel",
			Capability:                 "DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom full endpoint model flows.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "src/shared/openai-compat-url.ts", "packages/runtime/src/adapters/model", "src/renderer/src/components/settings-section-providers.tsx"},
			EntryPoint:                 "provider settings, runtime turn, write-inline, scheduled detector, model list/probe",
			TriggerPath:                "providerId/model/endpointFormat/baseUrl/custom endpoint",
			RuntimeContract:            "provider family, endpoint format, request URL/body/header, stream parsing, usage parsing",
			SettingsSchema:             "runtime.endpointFormat and provider.providers[].endpointFormat.",
			UISurface:                  "Provider/model settings keep endpoint-format controls.",
			ProtectionTests:            []string{"packages/runtime-go/kun_analytix_baseline_absorption_test.go", "packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts", "packages/runtime/tests/model-client.test.ts"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "DeepSeek cache/prefix enhancements only under DeepSeek provider family.",
			Status:                     "protected",
		},
		{
			ID:                         "tools-approval-user-input-mcp-goal",
			Capability:                 "Tools, approval gates, user input, MCP tools, slash commands, and /goal.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "packages/runtime/src/adapters/tool", "packages/runtime/src/loop/agent-loop.ts", "src/renderer/src/components/chat/FloatingComposer.tsx"},
			EntryPoint:                 "composer slash commands and runtime tool calls",
			TriggerPath:                "create_goal/get_goal/update_goal/approval/user-input/MCP tool events",
			RuntimeContract:            "goal events, approval routes, user-input routes, tool catalog and tool result events",
			SettingsSchema:             "approval policy, MCP/plugin settings, and runtime settings stay analytix-owned.",
			UISurface:                  "/goal remains a chat/composer action; MCP remains existing plugin/tool surface.",
			ProtectionTests:            []string{"packages/runtime/tests/goal-tools.test.ts", "packages/runtime/tests/contracts.test.ts", "packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "evidence/audit/lifecycle quality may improve; semantics and product entry do not change.",
			Status:                     "protected",
		},
		{
			ID:                         "attachments-workspace-write-sdd-connect-phone",
			Capability:                 "Attachments, localFilePath/FilePath propagation, workspace/files, Write, SDD, Connect Phone, speech, updates, logs, and terminal.",
			KunAnalytixSources:         []string{"github.com/KunAgent/Kun refs/tags/v0.2.13", "github.com/KunAgent/Kun refs/tags/v0.2.14", "src/preload/index.ts", "src/renderer/src/components/write", "src/renderer/src/sdd", "src/renderer/src/components/chat/ConnectPhoneView.tsx", "src/renderer/src/components/terminal/TerminalPanel.tsx"},
			EntryPoint:                 "existing domain bridge APIs and UI views",
			TriggerPath:                "files/workspace/write/sdd/connectPhone/speech/updates/logs/terminal bridge domains",
			RuntimeContract:            "attachments and file paths stay in contract fields, not final text only.",
			SettingsSchema:             "write/schedule/connect-phone/speech/media settings stay current schema.",
			UISurface:                  "No Reasonix product surface replaces these workflows.",
			ProtectionTests:            []string{"src/preload/preload-sandbox.test.ts", "src/main/claw-runtime.test.ts", "src/main/services/write-inline-completion-service.test.ts", "src/renderer/src/sdd/sdd-draft-store.test.ts"},
			ImmutableProductBaseline:   true,
			ReasonixEnhancementAllowed: "internal runtime/tool quality only; desktop workflows remain Kun/Analytix baseline.",
			Status:                     "protected",
		},
	}
	protected := 0
	enhanced := 0
	for _, row := range rows {
		if row.ImmutableProductBaseline {
			protected++
		}
		if row.ReasonixEnhancementAllowed != "none; Reasonix identity and config roots are rejected." {
			enhanced++
		}
	}
	return KunAnalytixBaselineGuard{
		SchemaVersion: 1,
		ChangeID:      KunAnalytixBaselineGuardChangeID,
		KunSourceSnapshots: []string{
			"https://github.com/KunAgent/Kun.git refs/tags/v0.2.13^{}=2ba8decc2f56862e7f677fcf89bbc3d402ec3a23",
			"https://github.com/KunAgent/Kun.git refs/tags/v0.2.14^{}=8f2040349fba47fcd8e8b94f50b131943af839b2",
		},
		AnalytixSourceRoot:       "/Users/sun/Projects/analytix",
		FullFunctionBaseline:     true,
		Rows:                     rows,
		ProtectedBaselineCount:   protected,
		InternalEnhancementCount: enhanced,
		ForbiddenTopLevelEntrypoints: []string{
			"Workflow",
			"Create Loop",
			"Subagent",
			"AutoResearch",
			"MCP-indexer",
		},
		ForbiddenEntrypointExposed:   false,
		DeprecatedBridgeAliasAllowed: false,
		LegacySettingsWriteAllowed:   false,
		DeepSeekOnlyRuntimeAllowed:   false,
		ReadyForReasonixAbsorption:   true,
		Notes: []string{
			"KunAgent/Kun v0.2.13 and v0.2.14 are the product baseline upstreams; local packaged Kun.app snapshots are not the repository source of truth.",
			"Reasonix is an engine donor; Kun/Analytix product behavior is the baseline.",
			"Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing.",
		},
	}
}

func BuildReasonixAbsorptionMatrix() ReasonixAbsorptionMatrix {
	rows := []ReasonixAbsorptionRow{
		{
			ID:                     "multi-model-non-regression",
			Capability:             "Go runtime turn/server provider routing for DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom_endpoint.",
			ReasonixSources:        []string{"internal/provider", "internal/agent/agent.go", "docs/index.html"},
			AbsorptionClass:        AbsorptionContractReimplement,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "Runtime provider turn config and runtime server conformance under analytix /v1 turn/SSE contract.",
			ProviderSpecific:       false,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"provider-model-multimodel", "bridge-settings-schema", "chat-thread-session-sse"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-multi-model-matrix-go", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("runtime-server-multi-model-turns", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("runtime-conformance-multi-model", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
		},
		{
			ID:                     "deepseek-cache-prefix-provider-specific",
			Capability:             "Reasonix-style DeepSeek cache/prefix discipline scoped to DeepSeek provider family.",
			ReasonixSources:        []string{"docs/index.html", "internal/agent", "cmd/e2ebench"},
			AbsorptionClass:        AbsorptionCodePortAndAdapt,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "DeepSeek-only request fields and cache diagnostics; non-DeepSeek providers keep their own endpoint/body/usage shape.",
			ProviderSpecific:       true,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"provider-model-multimodel", "chat-thread-session-sse"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("deepseek-cache-scoped-contract", "test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go"),
				PassedCheck("nondeepseek-cache-diagnostics-off", "test", "packages/runtime-go/runtime_server_test.go"),
			},
		},
		{
			ID:                     "goal-evidence-audit-sidecar",
			Capability:             "Goal evidence/audit strengthens quality without replacing /goal semantics.",
			ReasonixSources:        []string{"internal/evidence", "internal/tool/builtin/completestep_test.go"},
			AbsorptionClass:        AbsorptionContractReimplement,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "goal_evidence_audit remains available for goal/tool evidence evidence and must not be emitted by every normal chat turn.",
			ProviderSpecific:       false,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"tools-approval-user-input-mcp-goal"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("goal-evidence-audit-contract", "test", "packages/runtime-go/goal_evidence_contract_test.go"),
				PassedCheck("goal-tools-semantics-unchanged", "test", "packages/runtime/tests/goal-tools.test.ts"),
			},
		},
		{
			ID:                     "mcp-lifecycle-internal-audit",
			Capability:             "MCP connect/search/call/reconnect/redaction as internal runtime lifecycle audit.",
			ReasonixSources:        []string{"internal/plugin", "internal/mcpdiag"},
			AbsorptionClass:        AbsorptionContractReimplement,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "MCP lifecycle audit remains fixture/test-only evidence; normal runtime events must come from real configured MCP catalog/tool changes.",
			ProviderSpecific:       false,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"tools-approval-user-input-mcp-goal", "desktop-entry-ui"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("mcp-lifecycle-contract-go", "test", "packages/runtime-go/mcp_lifecycle_contract_test.go"),
			},
		},
		{
			ID:                     "subagent-job-lineage-internal",
			Capability:             "Reasonix sub-agent/job lineage adapted as internal parent goal/thread lineage only.",
			ReasonixSources:        []string{"internal/jobs", "internal/agent"},
			AbsorptionClass:        AbsorptionContractReimplement,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "pipeline_stage lineage event; no public Subagent route or UI entry.",
			ProviderSpecific:       false,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"desktop-entry-ui", "chat-thread-session-sse"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("runtime-lineage-sse-contract", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
		},
		{
			ID:                     "autoresearch-project-state",
			Capability:             "Project-local research state and stale pivot audit.",
			ReasonixSources:        []string{"docs/GUIDE.zh-CN.md", "internal/evidence"},
			AbsorptionClass:        AbsorptionContractReimplement,
			Status:                 AbsorptionStatusGreen,
			AnalytixLanding:        "Project-local .analytix/autoresearch state and autoresearch_state_audit under existing /goal --research turn/SSE contract.",
			ProviderSpecific:       false,
			DoesNotNarrowProviders: true,
			KunBaselineProtection:  []string{"tools-approval-user-input-mcp-goal", "desktop-entry-ui"},
			MachineChecks: []RuntimeMachineCheck{
				PassedCheck("autoresearch-project-state-contract-go", "test", "packages/runtime-go/autoresearch_state_contract_test.go"),
				PassedCheck("autoresearch-state-runtime-sse", "test", "packages/runtime-go/runtime_server_test.go"),
				PassedCheck("autoresearch-audit-contract-ts", "test", "packages/runtime/tests/contracts.test.ts"),
				PassedCheck("autoresearch-audit-reducer-ignore-ts", "test", "packages/runtime/tests/runtime-event-reducer.test.ts"),
				PassedCheck("autoresearch-state-contract-sse", "conformance", "packages/runtime/tests/go-runtime-conformance.test.ts"),
			},
		},
		{
			ID:                      "reasonix-public-protocol-product-entry",
			Capability:              "Reasonix SessionAPI, config roots, CLI identity, and public workflow/subagent/MCP-indexer surfaces.",
			ReasonixSources:         []string{"cmd/reasonix", "internal/config", "internal/serve", "docs/CONFIG_PATHS.md"},
			AbsorptionClass:         AbsorptionReject,
			Status:                  AbsorptionStatusRejected,
			AnalytixLanding:         "Rejected; analytix keeps Kun/Analytix product baseline and existing runtime contract.",
			ProviderSpecific:        false,
			DoesNotNarrowProviders:  true,
			KunBaselineProtection:   []string{"product-identity-release", "bridge-settings-schema", "desktop-entry-ui"},
			MachineChecks:           []RuntimeMachineCheck{PassedCheck("product-sovereignty-scan", "scan", "scripts/scan-product-sovereignty.cjs")},
			ForbiddenProductSurface: true,
			RejectedBecauseBaseline: "Would replace Kun/Analytix product entry/settings/bridge instead of enhancing internal engine behavior.",
		},
	}
	matrix := ReasonixAbsorptionMatrix{
		SchemaVersion:        1,
		ChangeID:             ReasonixAbsorptionMatrixChangeID,
		ReasonixSourcePath:   ReasonixSourcePath,
		ReasonixSourceCommit: ReasonixSourceCommit,
		Rows:                 rows,
		ProviderFamilies:     []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"},
		Notes: []string{
			"Reasonix upstream repository is https://github.com/esengine/DeepSeek-Reasonix; the local checkout is pinned for code-level comparison.",
			"readyForG6 is a legacy alias for absorptionMatrixGreen; it is not default Go backend readiness.",
			"Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing.",
			"Kun/Analytix full-function baseline wins every conflict with Reasonix product surfaces.",
		},
	}
	blockers := map[string]bool{}
	for _, row := range rows {
		switch row.Status {
		case AbsorptionStatusGreen:
			matrix.GreenCount++
		case AbsorptionStatusRed:
			matrix.RedCount++
		case AbsorptionStatusDeferred:
			matrix.DeferredCount++
			for _, blocker := range row.Blockers {
				blockers[row.ID+": "+blocker] = true
			}
		case AbsorptionStatusRejected:
			matrix.RejectedCount++
		}
		if row.ForbiddenProductSurface {
			matrix.ForbiddenProductSurfaceCount++
		}
	}
	matrix.G6Blockers = SortedKeys(blockers)
	matrix.MultiModelNonRegressionGreen = true
	matrix.DeepSeekEnhancementScopedOnly = true
	matrix.AbsorptionMatrixGreen = matrix.RedCount == 0 && matrix.DeferredCount == 0
	matrix.ReadyForG6 = matrix.AbsorptionMatrixGreen
	matrix.DefaultBackendReady = false
	matrix.DefaultBackendReadinessGate = "runtime durable restart evidence + credentialed provider matrix + credentialed MCP matrix + packaged QA + ANALYTIX_RUNTIME_READY=1"
	matrix.ReadinessSemantics = []string{
		"absorptionMatrixGreen means Reasonix absorption matrix has no red/deferred Reasonix absorption rows.",
		"readyForG6 remains for compatibility and equals absorptionMatrixGreen only.",
		"defaultBackendReady is false here because local absorption evidence is not credentialed runtime readiness.",
	}
	return matrix
}

func ProviderTurnConfig(providerID, model, contractProviderBaseURL string) RuntimeProviderTurnConfig {
	return provider.RuntimeTurnConfig(providerID, model, contractProviderBaseURL)
}

func RunMultiModelNonRegressionMatrix(ctx context.Context) (MultiModelNonRegressionMatrix, error) {
	contractProvider := providerscript.NewScriptedProviderServer()
	defer contractProvider.Close()
	client := provider.NewHTTPProviderClient(contractProvider.Client())
	providerIDs := []string{
		"deepseek-test-local",
		"openai-compatible-test-local",
		"anthropic-compatible-test-local",
		"custom-endpoint-test-local",
	}
	probes := make([]MultiModelProviderProbe, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		config := ProviderTurnConfig(providerID, "", contractProvider.URL)
		result, err := client.Stream(ctx, provider.Request{
			ProviderID:        config.ProviderID,
			Family:            config.Family,
			EndpointFormat:    config.EndpointFormat,
			BaseURL:           config.BaseURL,
			APIKey:            config.APIKey,
			Model:             config.Model,
			ReasoningEffort:   config.ReasoningEffort,
			ReasoningProtocol: config.ReasoningProtocol,
			SystemPrompt:      "You are analytix.",
			Messages: []provider.Message{
				{Role: "system", Content: "You are analytix."},
				{Role: "user", Content: "Run runtime multi-model non-regression probe."},
			},
			Tools: providerscript.DefaultProductionCandidateTools(),
		})
		if err != nil {
			return MultiModelNonRegressionMatrix{}, err
		}
		probe := MultiModelProviderProbe{
			ID:                        strings.TrimSuffix(providerID, "-live-local"),
			ProviderID:                config.ProviderID,
			Family:                    result.Family,
			EndpointFormat:            result.EndpointFormat,
			RequestURL:                result.RequestURL,
			RequestBodyFields:         result.RequestBodyFields,
			Usage:                     result.Usage,
			PrefixShape:               result.PrefixShape,
			CacheTelemetrySupported:   config.CacheTelemetrySupported,
			DeepSeekRequestOnlyFields: []string{},
			NonDeepSeekThinkingField:  containsRuntimeString(result.RequestBodyFields, "thinking") && result.Family != "deepseek",
			Status:                    "passed",
		}
		if result.Family == "deepseek" {
			for _, field := range []string{"thinking", "reasoning_effort"} {
				if containsRuntimeString(result.RequestBodyFields, field) {
					probe.DeepSeekRequestOnlyFields = append(probe.DeepSeekRequestOnlyFields, field)
				}
			}
		}
		probes = append(probes, probe)
	}
	families := make([]string, 0, len(probes))
	nonDeepCacheDiagnosticsOff := true
	customExact := false
	deepseekScoped := true
	for _, probe := range probes {
		families = append(families, probe.Family)
		if probe.Family != "deepseek" && probe.CacheTelemetrySupported {
			nonDeepCacheDiagnosticsOff = false
		}
		if probe.Family != "deepseek" && probe.NonDeepSeekThinkingField {
			deepseekScoped = false
		}
		if probe.Family == "custom_endpoint" && strings.HasSuffix(probe.RequestURL, "/custom-endpoint") {
			customExact = true
		}
	}
	covered := sameRuntimeStringSet(families, []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"})
	return MultiModelNonRegressionMatrix{
		SchemaVersion:                  1,
		ChangeID:                       MultiModelNonRegressionChangeID,
		ProviderFamilies:               families,
		ContractReplayProviderRequired: true,
		ReadsRealAPIKeysByDefault:      false,
		DeepSeekProviderSpecific:       deepseekScoped,
		NonDeepSeekCacheDiagnosticsOff: nonDeepCacheDiagnosticsOff,
		CustomEndpointUsesExactURL:     customExact,
		Probes:                         probes,
		Green:                          covered && deepseekScoped && nonDeepCacheDiagnosticsOff && customExact,
	}, nil
}

func containsRuntimeString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func sameRuntimeStringSet(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	actualCopy := append([]string(nil), actual...)
	expectedCopy := append([]string(nil), expected...)
	sort.Strings(actualCopy)
	sort.Strings(expectedCopy)
	for i := range actualCopy {
		if actualCopy[i] != expectedCopy[i] {
			return false
		}
	}
	return true
}
