//go:build analytix_prod

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type productRegressionRow struct {
	ContractID           string   `json:"contractId"`
	Priority             string   `json:"priority"`
	Required             bool     `json:"required"`
	GateIDs              []string `json:"gateIds"`
	Feature              string   `json:"feature"`
	KunAnalytixBaseline  string   `json:"kunAnalytixBaseline"`
	ReasonixEngine       string   `json:"reasonixEngine"`
	CurrentStatus        string   `json:"currentStatus"`
	Evidence             []string `json:"evidence"`
	RepairFiles          []string `json:"repairFiles"`
	Tests                []string `json:"tests"`
	Risk                 string   `json:"risk"`
	CacheImpact          string   `json:"cacheImpact"`
	ProviderCompatImpact string   `json:"providerCompatImpact"`
	SpeedImpact          string   `json:"speedImpact"`
}

type productRegressionMetadata struct {
	ContractID string
	Priority   string
	GateIDs    []string
	Required   bool
}

var productRegressionMetadataByFeature = map[string]productRegressionMetadata{
	"app startup":                         {ContractID: "p0-app-startup", Priority: "P0", GateIDs: []string{"runtime-go-health-smoke", "runtime-go-product-regression"}, Required: true},
	"Go runtime default":                  {ContractID: "p0-go-runtime-default", Priority: "P0", GateIDs: []string{"production-runtime-build-tag", "runtime-go-health-smoke"}, Required: true},
	"TypeScript retired backend":          {ContractID: "p0-typescript-retired-backend", Priority: "P0", GateIDs: []string{"typescript-retired-backend", "legacy-child-retired", "analytix-serve-go-launcher", "packaged-go-boundary"}, Required: true},
	"window.analytix bridge":              {ContractID: "p0-window-analytix-bridge", Priority: "P0", GateIDs: []string{"p0-preload-bridge-contract", "p0-renderer-browser-bridge-contract"}, Required: true},
	"new session":                         {ContractID: "p0-new-session", Priority: "P0", GateIDs: []string{"runtime-go-contracts", "go-runtime-conformance"}, Required: true},
	"plain text chat":                     {ContractID: "p0-plain-text-chat", Priority: "P0", GateIDs: []string{"runtime-go-contracts", "go-runtime-streaming-contract", "typescript-retired-backend"}, Required: true},
	"streaming first and delta events":    {ContractID: "p0-streaming-first-delta", Priority: "P0", GateIDs: []string{"main-renderer-streaming-contract", "go-runtime-streaming-contract", "go-runtime-interrupted-stream-recovery-contract"}, Required: true},
	"speed cache parity gate":             {ContractID: "p0-speed-cache-parity", Priority: "P0", GateIDs: []string{"runtime-go-speed-cache-gate", "live-validation-reporting-contract"}, Required: true},
	"DeepSeek provider":                   {ContractID: "p0-deepseek-provider", Priority: "P0", GateIDs: []string{"go-provider-cache-contract", "provider-settings-resolution"}, Required: true},
	"OpenAI-compatible provider":          {ContractID: "p0-openai-compatible-provider", Priority: "P0", GateIDs: []string{"go-provider-cache-contract", "go-runtime-tool-progress-contract"}, Required: true},
	"Anthropic messages provider":         {ContractID: "p0-anthropic-messages-provider", Priority: "P0", GateIDs: []string{"go-provider-cache-contract", "go-runtime-tool-progress-contract"}, Required: true},
	"provider profile settings":           {ContractID: "p0-provider-profile-settings", Priority: "P0", GateIDs: []string{"provider-settings-resolution", "app-settings-provider"}, Required: true},
	"model list and provider probe":       {ContractID: "p0-model-list-provider-probe", Priority: "P0", GateIDs: []string{"provider-connection", "app-settings-provider"}, Required: true},
	"thread list search archive":          {ContractID: "p0-thread-list-search-archive", Priority: "P0", GateIDs: []string{"runtime-go-contracts"}, Required: true},
	"thread rewind":                       {ContractID: "p1-thread-rewind", Priority: "P1", GateIDs: []string{"runtime-go-contracts", "runtime-go-packaged-session-soak"}, Required: true},
	"compaction history hygiene":          {ContractID: "p1-compaction-history-hygiene", Priority: "P1", GateIDs: []string{"go-runtime-compaction-history-contract"}, Required: true},
	"workspace checkpoint id":             {ContractID: "p1-workspace-checkpoint-id", Priority: "P1", GateIDs: []string{"runtime-event-contract", "runtime-go-contracts"}, Required: true},
	"desktop git checkpoint":              {ContractID: "p1-desktop-git-checkpoint", Priority: "P1", GateIDs: []string{"desktop-git-checkpoint", "preload-runtime-request"}, Required: true},
	"checkpoint file rewind apply":        {ContractID: "p2-checkpoint-file-rewind-apply", Priority: "P2", GateIDs: []string{"runtime-go-contracts", "go-checkpoint-private-snapshot-contract", "typescript-checkpoint-rewind-apply-contract", "renderer-checkpoint-rewind-apply-control"}, Required: true},
	"resume and fork":                     {ContractID: "p0-resume-fork", Priority: "P0", GateIDs: []string{"runtime-go-packaged-session-soak", "runtime-go-contracts"}, Required: true},
	"SSE replay":                          {ContractID: "p0-sse-replay", Priority: "P0", GateIDs: []string{"runtime-go-packaged-session-soak", "main-renderer-streaming-contract"}, Required: true},
	"usage cost and input composer usage": {ContractID: "p0-usage-cost-input-composer", Priority: "P0", GateIDs: []string{"p0-usage-cost-cache-contract", "p0-renderer-usage-contract"}, Required: true},
	"attachments":                         {ContractID: "p0-attachments", Priority: "P0", GateIDs: []string{"p0-attachments-vision-contract", "p0-legacy-ts-attachments-vision-contract"}, Required: true},
	"image vision payload":                {ContractID: "p0-image-vision-payload", Priority: "P0", GateIDs: []string{"p0-attachments-vision-contract", "p0-legacy-ts-attachments-vision-contract", "go-provider-cache-contract"}, Required: true},
	"approval":                            {ContractID: "p0-approval", Priority: "P0", GateIDs: []string{"p0-approval-user-input-contract", "go-runtime-approval-contract"}, Required: true},
	"user input":                          {ContractID: "p0-user-input", Priority: "P0", GateIDs: []string{"p0-approval-user-input-contract", "go-runtime-tool-progress-contract"}, Required: true},
	"built-in tools":                      {ContractID: "p0-built-in-tools", Priority: "P0", GateIDs: []string{"runtime-go-contracts", "production-product-regression-matrix", "go-runtime-diff-engine-contract", "go-runtime-file-diff-contract", "go-runtime-multi-edit-contract", "go-runtime-move-file-contract", "go-runtime-notebook-edit-contract", "go-runtime-delete-range-contract", "go-runtime-delete-symbol-contract", "go-runtime-glob-contract", "go-runtime-grep-contract", "go-runtime-grep-pruning-contract", "go-runtime-utf16-file-encoding-contract", "go-runtime-code-index-contract", "go-runtime-web-fetch-contract", "go-runtime-repeat-guard-contract", "go-runtime-failure-storm-contract"}, Required: true},
	"plan mode create_plan":               {ContractID: "p1-plan-mode-create-plan", Priority: "P1", GateIDs: []string{"runtime-go-contracts", "go-runtime-plan-mimo-provider-contract", "typescript-retired-backend"}, Required: true},
	"cache-first review gate":             {ContractID: "p0-cache-first-review-gate", Priority: "P0", GateIDs: []string{"cache-first-review-gate"}, Required: true},
	"MCP stdio http":                      {ContractID: "p0-mcp-stdio-http", Priority: "P0", GateIDs: []string{"p0-mcp-runtime-contract", "p0-mcp-ui-contract"}, Required: true},
	"subagent task parallel durable":      {ContractID: "p1-subagent-task-parallel-durable", Priority: "P1", GateIDs: []string{"go-runtime-subagent-contract", "typescript-retired-backend", "p0-provider-settings-probe-contract"}, Required: true},
	"background shell jobs":               {ContractID: "p1-background-shell-jobs", Priority: "P1", GateIDs: []string{"go-runtime-background-job-contract"}, Required: true},
	"renderer timeline cards":             {ContractID: "p0-renderer-timeline-cards", Priority: "P0", GateIDs: []string{"renderer-timeline-contract", "p0-packaged-gui-smoke-contract"}, Required: true},
	"error display":                       {ContractID: "p0-error-display", Priority: "P0", GateIDs: []string{"p0-error-display-contract", "renderer-timeline-contract", "go-provider-cache-contract", "go-runtime-interrupted-stream-recovery-contract"}, Required: true},
	"dev packaged smoke":                  {ContractID: "p2-dev-packaged-smoke", Priority: "P2", GateIDs: []string{"p0-packaged-gui-smoke-contract", "p0-packaged-session-soak-contract"}, Required: true},
}

func productRegressionMatrix() []productRegressionRow {
	return []productRegressionRow{
		productRow("app startup", "Electron main starts the bundled analytix runtime and keeps /health reachable.", "Runtime diagnostics stay owned by the engine layer.", "covered-by-runtime-health-smoke", []string{"src/main/runtime/analytix-adapter.ts", "src/main/analytix-process.ts", "scripts/runtime-go-runtime-health-smoke.mjs"}, []string{"src/main/runtime/analytix-adapter.ts", "src/main/analytix-process.ts", "scripts/runtime-go-runtime-health-smoke.mjs"}, []string{"src/main/runtime/analytix-adapter.test.ts", "src/main/analytix-process.test.ts", "npm run runtime:go:health-smoke -- --json"}, "packaged app smoke still requires an external app launch", "none", "all providers", "startup path"),
		productRow("Go runtime default", "Go runtime is the default bundled runtime boundary.", "Production composition root uses real provider, MCP, stores, and event replay.", "covered-prod-contract", []string{"packages/runtime-go/internal/server/runtime_components.go", "packages/runtime-go/cmd/runtime-server/main.go", "scripts/runtime-go-runtime-health-smoke.mjs"}, []string{"packages/runtime-go/internal/server/runtime_components.go", "packages/runtime-go/cmd/runtime-server/main.go", "scripts/runtime-go-default-readiness-report.mjs"}, []string{"packages/runtime-go/internal/server/capabilities_prod_test.go", "packages/runtime-go/runtime_server_test.go", "npm run runtime:go:health-smoke -- --json", "npm run runtime:go:default-readiness-report -- --json"}, "full app launch remains a separate smoke", "low", "all providers", "runtime HTTP/SSE"),
		productRow("TypeScript retired backend", "ANALYTIX_RUNTIME_BACKEND=typescript returns a retired_backend diagnostic and never starts the TypeScript agent runtime.", "Go runtime-server is the only production agent runtime; packages/runtime ships contracts/config/telemetry and a Go launcher, while server/loop/provider runtime dist is absent from packaged builds.", "covered-by-retirement-evidence", []string{"src/main/runtime/analytix-adapter.ts", "src/main/analytix-process.ts", "packages/runtime/src/cli/serve-entry.ts", "packages/runtime/tsconfig.build.json"}, []string{"src/main/runtime/analytix-adapter.ts", "src/main/analytix-process.ts", "packages/runtime/src/cli/serve-entry.ts", "packages/runtime/package.json", "scripts/runtime-go-rollback-retirement-evidence.mjs", "scripts/runtime-go-rollback-retirement-report.mjs"}, []string{"src/main/runtime/analytix-adapter.test.ts", "src/main/analytix-process.test.ts", "packages/runtime/tests/serve-entry-go-launcher.test.ts", "src/main/packaging-config.test.ts", "npm run runtime:go:rollback-retirement-evidence -- --json", "npm run runtime:go:rollback-retirement-report -- --json"}, "historical TS source remains only as migration reference until fully deleted; production exports, launcher, and package dist cannot execute it", "medium", "all providers", "backend gate, packaged boundary, and CLI runtime"),
		productRow("window.analytix bridge", "Renderer product code talks to the desktop only through window.analytix; preload and browser-preview bridges must keep runtime request, SSE, settings, file path, and workspace operations on Analytix-owned APIs without deprecated upstream aliases.", "Engine routes stay behind the Electron IPC/runtime facade rather than exposing engine-specific public routes.", "covered-preload-renderer-contract", []string{"src/preload/index.ts", "src/shared/analytix-api.ts", "src/main/ipc/app-ipc-schemas.ts", "src/main/ipc/register-app-ipc-handlers.ts", "src/renderer/src/lib/browser-analytix-bridge.ts"}, []string{"src/preload/index.ts", "src/shared/analytix-api.ts", "src/main/ipc/app-ipc-schemas.ts", "src/main/ipc/register-app-ipc-handlers.ts", "src/renderer/src/lib/browser-analytix-bridge.ts"}, []string{"src/preload/preload-sandbox.test.ts", "src/preload/preload-runtime-request.test.ts", "src/preload/preload-sse-bridge.test.ts", "src/main/ipc/app-ipc-schemas.test.ts", "src/main/ipc/register-app-ipc-handlers.test.ts", "src/renderer/src/lib/browser-analytix-bridge.test.ts"}, "browser preview is a development facade only; Electron preload remains authoritative in app smoke", "low", "all providers", "main SSE bridge and runtime request IPC"),
		productRow("new session", "Threads and sessions remain the product entry point; starting a clean new session must clear the selected timeline without dropping a busy previous thread from completion watch.", "Durable thread/session store backs the Go runtime route.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/thread_routes.go", "src/renderer/src/store/chat-store-thread-actions.ts"}, []string{"packages/runtime-go/internal/server/thread_routes.go", "src/renderer/src/store/chat-store-thread-actions.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts", "src/renderer/src/store/chat-store-thread-actions.test.ts"}, "materialized runtime thread creation and clean draft creation are distinct product flows", "none", "all providers", "SSE abort and background completion watch"),
		productRow("plain text chat", "A normal assistant turn keeps provider draft text private and atomically publishes the complete host-gated candidate; renderer send-path creates the optimistic user row, forwards composer provider/model to runtime, starts SSE for host progress, and the retired TypeScript test-support model request carries Kun's workspace/time runtime context as per-turn environment data.", "Agent loop emits typed host progress and usage while withholding provider prose until terminal publication; volatile runtime context is sent through contextInstructions rather than the stable system prefix.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go", "packages/runtime/src/loop-test-support/agent-loop.ts", "src/renderer/src/store/chat-store-thread-actions.ts"}, []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go", "packages/runtime/src/loop-test-support/agent-loop.ts", "src/renderer/src/store/chat-store-thread-actions.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime-go/internal/provider/provider_test.go", "packages/runtime/tests/loop.test.ts", "src/renderer/src/store/chat-store-thread-actions.test.ts"}, "live provider reachability is outside deterministic unit scope", "low", "DeepSeek/OpenAI-compatible/Anthropic/messages/custom", "first token path and renderer SSE subscription"),
		productRow("streaming first and delta events", "Renderer receives the first visible event immediately and later deltas per frame.", "Typed stream chunks separate reasoning, text, tool calls, usage, retry/error, and bounded interrupted-stream recovery.", "covered-contract-with-interrupted-stream-recovery", []string{"src/main/runtime-sse-ipc.ts", "src/renderer/src/thread/streaming/streaming-delta-scheduler.ts", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go"}, []string{"src/main/runtime-sse-ipc.ts", "src/renderer/src/thread/streaming/streaming-delta-scheduler.ts", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go"}, []string{"src/main/runtime-sse-ipc.test.ts", "src/renderer/src/thread/streaming/streaming-delta-scheduler.test.ts", "packages/runtime-go/runtime_server_test.go"}, "browser paint timing still needs app smoke; interrupted-stream recovery is bounded to three attempts and asks the model not to repeat visible text or reuse partial tool-call arguments", "medium", "all streaming providers", "first event, frame scheduler, and interrupted-stream tail recovery"),
		productRow("speed cache parity gate", "Feedback speed and cache correctness are product gates, not optional performance polish.", "Reasonix typed streaming, partial tool dispatch, retry, tool progress, prefix shape, and cache usage diagnostics are grouped as one repeatable gate.", "covered-hard-gate", []string{"scripts/runtime-go-speed-cache-gate.mjs", "scripts/runtime-go-performance-check.mjs", "scripts/runtime-go-live-validation.mjs"}, []string{"scripts/runtime-go-speed-cache-gate.mjs", "scripts/runtime-go-performance-check.mjs", "scripts/runtime-go-live-validation.mjs"}, []string{"npm run runtime:go:speed-cache-gate", "src/main/runtime-go-live-validation.test.ts"}, "browser paint timing and gated live provider probes remain separate smoke layers; report-only live validation may be partial when a non-DeepSeek profile is not configured", "high", "DeepSeek/OpenAI-compatible/Anthropic/messages/custom", "first event, tool progress, retry, cache usage, and live evidence reporting"),
		productRow("DeepSeek provider", "DeepSeek remains a model provider, not the product identity.", "DeepSeek cache usage and reasoning fields are provider-scoped.", "covered-provider-contract", []string{"packages/runtime-go/internal/provider/provider.go", "src/shared/app-settings-provider.ts"}, []string{"packages/runtime-go/internal/provider/provider.go", "src/shared/app-settings-provider.ts"}, []string{"packages/runtime-go/internal/provider/provider_test.go", "src/shared/app-settings-provider.test.ts"}, "short live probe is separate and must not print keys", "high", "DeepSeek/OpenAI-compatible", "provider stream parser"),
		productRow("OpenAI-compatible provider", "Custom OpenAI-compatible profile settings keep baseUrl and endpointFormat semantics.", "Shared parser handles chat-completions, OpenAI responses function_call items, second-round tool output history, and nested cached_tokens.", "covered-provider-and-loop-contract", []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go", "src/shared/openai-compat-url.ts"}, []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go", "src/shared/openai-compat-url.ts", "scripts/runtime-go-speed-cache-gate.mjs"}, []string{"packages/runtime-go/internal/provider/provider_test.go", "packages/runtime-go/runtime_server_test.go", "src/shared/analytix-endpoints.test.ts", "npm run runtime:go:speed-cache-gate"}, "provider-specific auth remains configured by user settings", "high", "OpenAI-compatible/custom", "provider stream parser and second-round tool loop"),
		productRow("Anthropic messages provider", "Anthropic/messages stays a first-class endpoint family.", "tool_use, tool_result, signed thinking, and second-round tool history round-trip without DeepSeek field leakage.", "covered-provider-and-loop-contract", []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go"}, []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/app/loop/runtime_runner.go", "packages/runtime-go/internal/server/agent_loop.go", "scripts/runtime-go-speed-cache-gate.mjs"}, []string{"packages/runtime-go/internal/provider/provider_test.go", "packages/runtime-go/runtime_server_test.go", "npm run runtime:go:speed-cache-gate"}, "live Anthropic key is not required for deterministic contract", "high", "Anthropic/messages", "provider stream parser and second-round tool loop"),
		productRow("provider profile settings", "Top-level provider.activeProviderId and provider.providers[] own model settings; onboarding must not write legacy runtime credential fields. Renderer request paths, including plan-mode, SDD assistant thread creation, and SDD plan upgrade turns, must preserve providerId/model selection for saved multi-provider profiles such as Xiaomi/MiMo.", "Engine reads provider profile shape without replacing the product settings schema.", "covered-settings-contract", []string{"src/shared/app-settings-provider.ts", "src/renderer/src/components/initial-setup-save.ts", "src/renderer/src/agent/analytix-runtime.ts", "src/renderer/src/components/Workbench.tsx", "src/renderer/src/components/workbench-plan-controller.ts", "scripts/runtime-go-performance-check.mjs"}, []string{"src/shared/app-settings-provider.ts", "src/renderer/src/components/initial-setup-save.ts", "src/renderer/src/agent/analytix-runtime.ts", "src/renderer/src/components/Workbench.tsx", "src/renderer/src/components/workbench-plan-controller.ts", "scripts/runtime-go-performance-check.mjs"}, []string{"src/shared/app-settings-provider.test.ts", "src/renderer/src/components/initial-setup-save.test.ts", "src/renderer/src/agent/analytix-runtime.test.ts", "src/renderer/src/components/workbench-plan-controller.test.ts", "src/renderer/src/components/Workbench.route-surface.test.ts", "npm run typecheck", "node scripts/runtime-go-performance-check.mjs --self-test-settings-resolution --json --gate"}, "live profile values must remain sanitized; provider-matrix can classify Xiaomi as OpenAI-compatible, so MiMo identity requires renderer request and live-validation evidence; plan controller and SDD assistant thread creation must keep providerId with model to avoid shared-model misrouting", "medium", "all providers", "startup, probe path, plan-mode, and renderer runtime request model selection"),
		productRow("model list and provider probe", "Renderer/main can list configured models, probe provider connectivity with actionable Base URL and Endpoint format diagnostics, import fetched model ids into the active provider profile, and pass the provider proxy setting into the Go runtime without writing legacy runtime credential fields or treating that proxy as generic MCP destination authority.", "Provider diagnostics stay sanitized and provider-scoped; provider proxy validation remains separate, while generic MCP HTTP/SSE permits only direct public destinations with complete DNS-answer validation, numeric-IP dialing, preserved TLS ServerName, and no redirect or proxy fallback.", "covered-desktop-settings-contract", []string{"src/main/upstream-models.ts", "src/main/provider-connection.ts", "src/main/runtime/analytix-adapter.ts", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/components/settings-section-providers.tsx", "src/shared/app-settings-provider.ts"}, []string{"src/main/upstream-models.ts", "src/main/provider-connection.ts", "src/main/runtime/analytix-adapter.ts", "packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/mcp/manager.go", "packages/runtime-go/internal/adapters/outbound/mcp/http/client.go", "packages/runtime-go/internal/adapters/outbound/mcp/http/remote_destination.go", "packages/runtime-go/internal/server/runtime_info_routes.go", "src/renderer/src/components/settings-section-providers.tsx", "src/shared/app-settings-provider.ts"}, []string{"src/main/provider-connection.test.ts", "src/main/upstream-models.test.ts", "src/main/runtime/analytix-adapter.test.ts", "packages/runtime-go/internal/provider/provider_test.go", "packages/runtime-go/internal/mcp/manager_test.go", "packages/runtime-go/internal/adapters/outbound/mcp/http/remote_destination_test.go", "packages/runtime-go/runtime_server_test.go", "src/renderer/src/components/settings-section-agents.test.ts", "src/shared/app-settings-provider.test.ts"}, "external network failures remain user-facing diagnostics without recording credential values; generic remote MCP rejects configured and environment proxies rather than letting an intermediary re-resolve a validated destination, and trusted local MCP uses stdio or host transport", "low", "all providers and generic MCP HTTP/SSE", "probe path, settings profile import, provider proxy HTTP transport, and direct public-only MCP remote transport"),
		productRow("thread list search archive", "Thread list, search, and archive remain product routes; archiving the active thread clears the selected timeline in the renderer.", "Durable store indexes thread metadata and usage.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/thread_routes.go", "packages/runtime-go/internal/server/durable_store.go", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/internal/server/thread_routes.go", "packages/runtime-go/internal/server/durable_store.go", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/http-server.test.ts", "src/renderer/src/agent/analytix-runtime.test.ts", "src/renderer/src/store/chat-store-maintenance-actions.test.ts"}, "large stores need longer soak", "none", "all providers", "none"),
		productRow("thread rewind", "Ordinary uncommitted history may be durably rewound; a committed accepted final is append-only until a signed supersession contract exists.", "Successful ordinary rewind emits a thread_rewound replay barrier, while committed case authority rejects rewind before any workspace or conversation mutation.", "covered-runtime-contract", []string{"packages/runtime-go/internal/server/thread_routes.go", "packages/runtime-go/internal/server/durable_store.go", "packages/runtime/src/services-test-support/turn-service.ts", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/internal/server/thread_routes.go", "packages/runtime-go/internal/server/durable_store.go", "packages/runtime/src/services-test-support/turn-service.ts", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/http-server.test.ts", "src/renderer/src/agent/analytix-mapper.test.ts"}, "signed accepted-final supersession and full Reasonix-style checkpoint scopes remain follow-ups", "medium", "all providers", "authority-aware rewind barrier"),
		productRow("compaction history hygiene", "Manual and automatic compaction must emit Kun-compatible compaction events, preserve active Goal, unfinished Todos, bounded latest user constraints, and trace-only evidence references without authority upgrades, rewrite visible history, and keep summaries available to future provider calls.", "Reasonix cache-first compaction discipline is absorbed as deterministic Go pre-turn admission, history rewrite, closed continuation snapshots, and a final request hard limit before later model-summary upgrades.", "covered-runtime-contract", []string{"packages/runtime-go/internal/app/thread/auto_compaction.go", "packages/runtime-go/internal/app/turn/compaction.go", "packages/runtime-go/internal/app/model/context_estimator.go", "packages/runtime-go/internal/app/model/provider_history.go", "packages/runtime-go/internal/server/turn_start.go"}, []string{"packages/runtime-go/internal/domain/thread/compaction_contract.go", "packages/runtime-go/internal/app/thread/compaction_security.go", "packages/runtime-go/internal/app/loop/provider_stream.go", "scripts/runtime-go-speed-cache-gate.mjs"}, []string{"packages/runtime-go/internal/app/thread/auto_compaction_test.go", "packages/runtime-go/internal/app/turn/task_continuation_test.go", "packages/runtime-go/auto_compaction_runtime_test.go", "packages/runtime-go/runtime_server_test.go", "npm run runtime:go:speed-cache-gate"}, "Go compaction uses deterministic host snapshots rather than model summaries; case-thread evidence authority remains fail-closed until the trusted Evidence Registry archive exists", "medium", "all providers", "pre-turn admission, restartable continuation, provider request hard limit, history repair, and cache prefix stability"),
		productRow("workspace checkpoint id", "Kun-compatible workspaceCheckpointId must round-trip on start-turn requests, persisted turns, user items, and replay events.", "Reasonix checkpoint engine can later restore code/conversation snapshots without replacing this product field.", "covered-runtime-contract", []string{"packages/runtime/src/contracts/turns.ts", "packages/runtime/src/contracts/items.ts", "packages/runtime-go/internal/server/turn_start.go"}, []string{"packages/runtime/src/contracts/turns.ts", "packages/runtime/src/contracts/items.ts", "packages/runtime/src/services-test-support/turn-service.ts", "packages/runtime-go/internal/server/turn_start.go"}, []string{"packages/runtime/tests/contracts.test.ts", "packages/runtime/tests/http-server.test.ts", "packages/runtime-go/runtime_server_test.go"}, "desktop Git checkpoint now supplies the product field; non-Git snapshot scopes remain a follow-up", "low", "all providers", "history and replay metadata"),
		productRow("desktop git checkpoint", "The desktop bridge creates a Git checkpoint before send and can restore it before rewind/resend or via assistant rollback.", "Reasonix snapshot checkpoints can later generalize this beyond Git working trees.", "covered-desktop-contract", []string{"src/main/services/git-checkpoint-service.ts", "src/main/ipc/register-app-ipc-handlers.ts", "src/preload/index.ts", "src/renderer/src/store/chat-store-thread-actions.ts", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"src/shared/git-checkpoint.ts", "src/main/services/git-checkpoint-service.ts", "src/shared/analytix-api.ts", "src/renderer/src/components/chat/MessageTimeline.tsx", "src/renderer/src/components/chat/message-timeline-bubbles.tsx"}, []string{"src/main/services/git-checkpoint-service.test.ts", "src/main/ipc/register-app-ipc-handlers.test.ts", "src/preload/preload-runtime-request.test.ts", "src/renderer/src/store/chat-store-thread-actions.test.ts", "src/renderer/src/store/chat-store-maintenance-actions.test.ts"}, "restore is destructive and remains confirmation-gated; non-Git workspaces skip this bridge", "none", "all providers", "send preflight"),
		productRow("checkpoint file rewind apply", "Checkpoint apply must not stay audit-only when a confirmed rewind plan and snapshot evidence are available, and renderer timeline/inspector controls must keep the apply action visible and confirmation-gated.", "Reasonix checkpoint/rewind safety ideas are absorbed as runtime private first-touch snapshot capture, plan validation, snapshot evidence, path safety, rescue, and apply events.", "covered-go-typescript-and-renderer-contract", []string{"packages/runtime-go/internal/server/checkpoint.go", "packages/runtime-go/runtime_server_test.go", "packages/runtime/src/contracts/checkpoints.ts", "packages/runtime/src/services-test-support/checkpoint-rewind-service.ts", "src/renderer/src/agent/analytix-contract.ts", "src/renderer/src/components/RewindPlanApplyControls.tsx"}, []string{"packages/runtime-go/internal/server/checkpoint.go", "packages/runtime-go/runtime_server_test.go", "packages/runtime/src/contracts/checkpoints.ts", "packages/runtime/src/services-test-support/checkpoint-rewind-service.ts", "packages/runtime/src/domain/checkpoint-rewind-contract.ts", "src/renderer/src/agent/analytix-contract.ts", "src/renderer/src/components/RewindPlanApplyControls.tsx", "src/renderer/src/components/chat/message-timeline-cards.tsx"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/checkpoint-rewind-apply.test.ts", "src/renderer/src/components/RewindPlanApplyControls.test.ts", "npm run runtime:go:product-regression"}, "desktop Git checkpoint and runtime snapshot apply now have separate contracts; remaining risk is one unified product surface for choosing code/conversation/both restore in the GUI", "medium", "all providers", "file mutation path and renderer control"),
		productRow("resume and fork", "Resume/fork preserve parent history and UI navigation semantics; active renderer actions refresh thread lists, record fork lineage, and select the target thread.", "History repair keeps tool-call/tool-result pairing valid, including legacy persisted turns with duplicate or missing call ids.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/durable_threads.go", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/internal/server/durable_threads.go", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/store/chat-store-maintenance-actions.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime-go/internal/server/history_repair_test.go", "packages/runtime-go/internal/provider/provider_test.go", "src/renderer/src/store/chat-store-maintenance-actions.test.ts"}, "complex interrupted live streams need staged coverage", "high", "all providers", "history repair path and UI thread selection"),
		productRow("SSE replay", "Thread event replay supports since_seq and Last-Event-ID, and renderer subscriptions advance cursors only for delivered batches.", "Recorder can replay persisted events without breaking live order.", "covered-runtime-main-renderer-contract", []string{"packages/runtime-go/internal/protocol/route_replay.go", "packages/runtime-go/internal/server/durable_store.go", "src/main/runtime-sse-ipc.ts", "src/renderer/src/agent/analytix-runtime.ts"}, []string{"packages/runtime-go/internal/protocol/route_replay.go", "packages/runtime-go/internal/server/durable_store.go", "src/main/runtime-sse-ipc.ts", "src/renderer/src/agent/analytix-runtime.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "src/main/runtime-sse-ipc.test.ts", "src/renderer/src/agent/analytix-runtime.test.ts", "src/renderer/src/store/chat-store-runtime.test.ts"}, "cross-process replay is covered by smoke", "low", "all providers", "live/replay event order"),
		productRow("usage cost and input composer usage", "Usage appears in thread/model/day views and composer affordances, live SSE usage chips keep token/cost/cache fields, and missing provider pricing must be shown as not configured rather than zero cost.", "Usage source attribution and cache tokens are parsed from provider-native usage; priceConfigured separates explicit free/zero pricing from unknown pricing.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/usage.go", "packages/runtime-go/internal/adapters/outbound/usageindexfs/store.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/hooks/use-thread-usage.ts", "src/renderer/src/hooks/use-daily-usage.ts", "src/renderer/src/hooks/use-model-usage.ts"}, []string{"packages/runtime-go/internal/server/usage.go", "packages/runtime-go/internal/adapters/outbound/usageindexfs/store.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/hooks/use-thread-usage.ts", "src/renderer/src/hooks/use-daily-usage.ts", "src/renderer/src/hooks/use-model-usage.ts"}, []string{"packages/runtime-go/internal/server/usage_test.go", "packages/runtime-go/internal/adapters/outbound/usageindexfs/store_test.go", "packages/runtime/tests/usage-service.test.ts", "src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/hooks/use-thread-usage.test.ts", "src/renderer/src/hooks/use-daily-usage.test.ts", "src/renderer/src/hooks/use-model-usage.test.ts"}, "live provider usage cost still depends on configured provider pricing, but thread/daily/model renderer contracts no longer collapse unknown pricing into zero-cost certainty; cachedTokens-only events remain unknown rather than cache-hit evidence", "high", "all providers", "usage event path, snake/camel live usage fields, and renderer usage chips"),
		productRow("attachments", "Uploads require a host-existing thread and its canonical workspace; each upload receives a distinct owner identity, while blob SHA-256, byte size, MIME, provider projection, and private upload intent/disposition are integrity checked without merging metadata or authority across owners.", "One Go use case commits private intent, exact durable files, live binding recheck, owner membership, and terminal disposition before HTTP 201. A frozen executable case TSC and signed AttachmentUseReceipt then bind exact thread, turn, case, epoch, snapshot, context, owner, blob, and provider projection before effects; legacy, stale, partial, corrupt, missing, or mismatched inputs remain non-executable.", "partial-cross-layer-attachment-contract", []string{"packages/runtime-go/internal/domain/attachment/owner.go", "packages/runtime-go/internal/domain/attachment/upload_transaction.go", "packages/runtime-go/internal/domain/attachment/use_receipt.go", "packages/runtime-go/internal/adapters/outbound/filestore/attachments.go", "packages/runtime-go/internal/app/attachmentauthority/service.go", "packages/runtime-go/internal/app/turn/attachments.go", "packages/runtime-go/internal/server/turn_start.go", "src/renderer/src/components/Workbench.tsx"}, []string{"packages/runtime-go/internal/domain/attachment/upload_transaction.go", "packages/runtime-go/internal/adapters/outbound/attachmentauthority/store.go", "packages/runtime-go/internal/adapters/outbound/filestore/attachment_recovery.go", "packages/runtime-go/internal/adapters/inbound/httpapi/attachments.go", "packages/runtime-go/internal/app/attachmentauthority/service.go", "packages/runtime-go/internal/app/attachmentpipeline/service.go", "packages/runtime-go/internal/server/routes.go", "packages/runtime-go/internal/server/turn_start.go", "packages/runtime-go/internal/runtimeapp/app.go"}, []string{"packages/runtime-go/internal/domain/attachment/upload_transaction_test.go", "packages/runtime-go/internal/adapters/outbound/attachmentauthority/store_test.go", "packages/runtime-go/internal/adapters/outbound/filestore/attachment_recovery_test.go", "packages/runtime-go/internal/app/attachmentauthority/service_test.go", "packages/runtime-go/internal/app/attachmentpipeline/service_test.go", "packages/runtime-go/internal/server/attachment_security_test.go", "packages/runtime-go/internal/runtimeapp/startup_test.go"}, "P1 remains open for the closed desktop attachment display/action contract, legacy migration, target-platform file identity and permission fault tests, controlled-artifact integration, and remaining exhaustive owner/use crash cuts.", "high", "vision and non-vision providers", "owner upload, current case admission, vision bridge, provider payload, persistence, and SSE projection"),
		productRow("image vision payload", "Vision models receive image payloads while non-vision models get bounded text fallback.", "Provider serialization stays endpoint-family specific.", "covered-provider-and-legacy-ts-contract", []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime/src/model-test-support/model/compat-model-client.ts", "packages/runtime/src/attachments/attachment-store.ts"}, []string{"packages/runtime-go/internal/provider/provider.go", "packages/runtime/src/model-test-support/model/compat-model-client.ts", "packages/runtime/src/attachments/attachment-store.ts"}, []string{"packages/runtime-go/internal/provider/provider_test.go", "packages/runtime/tests/model-client.test.ts", "packages/runtime/tests/attachment-store.test.ts", "npm run runtime:go:product-regression -- --json"}, "live multimodal provider smoke remains separate; retired TypeScript image serialization test support is guarded by runtime package tests inside product-regression", "high", "DeepSeek/OpenAI-compatible/Anthropic/messages/custom", "provider request body and retired TypeScript vision fallback"),
		productRow("approval", "Tools that require approval pause, denied tools do not execute, and renderer cards appear only from approval events.", "Agent loop preserves gate events across resume/replay.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/gates_routes.go", "packages/runtime-go/internal/agent/gates.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts"}, []string{"packages/runtime-go/internal/server/gates_routes.go", "packages/runtime-go/internal/agent/gates.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts", "src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/store/chat-store-runtime.test.ts"}, "live approval provider smoke remains covered by runtime loop contracts", "low", "all providers", "tool dispatch path"),
		productRow("user input", "user_input is advertised and shown only when the model calls the tool; free-text questions must not invent submit-only options.", "Gate events carry request/resolution state without leaking submitted answers in replay.", "covered-runtime-and-renderer-contract", []string{"packages/runtime-go/internal/server/gates_routes.go", "packages/runtime-go/internal/agent/gates.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts"}, []string{"packages/runtime-go/internal/server/gates_routes.go", "packages/runtime-go/internal/agent/gates.go", "scripts/runtime-go-speed-cache-gate.mjs", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime/tests/go-runtime-conformance.test.ts", "src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/store/chat-store-runtime.test.ts", "npm run runtime:go:speed-cache-gate"}, "live user-input provider smoke remains covered by runtime loop contracts", "low", "all providers", "tool dispatch path"),
		productRow(
			"built-in tools",
			"Kun/Analytix built-in tool names, policies, and runtime tools/capabilities remain product-owned; production and renderer diagnostics must not expose temporary validation, replay-only, or local-only tool surfaces as product tools. The Go read tool keeps Kun-compatible 1-based pagination, read-before-edit safety, Reasonix-style ordered multi-edit, Reasonix-style move_file/notebook_edit/delete_range/delete_symbol, Kun-compatible grep glob/context/column output plus Reasonix-style grep pruning, repo .gitignore handling, UTF-16/BOM text preservation, glob/code_index, config-gated web_fetch, and file writer results visible enough for timeline/change summaries.",
			"Tool schemas are canonicalized for cache-stable hashing; the read_file alias absorbs numbered 0-based pagination output for stronger edit targeting while keeping raw read content in the edit guard. Reasonix-style bounded unified diff metadata is absorbed for write_file/edit_file/multi_edit/notebook_edit/delete_range/delete_symbol tool results without provider-visible before/after full text, edit_file accepts old_string/new_string plus replace_all, multi_edit is a first-class atomic file-change tool, move_file is a first-class file move with workspace, allow_write, and full-access path policy plus destination-exists protection and cross-device fallback, notebook_edit is a first-class .ipynb cell edit tool that preserves notebook JSON and clears code outputs, delete_range is a fresh-read-gated exact-anchor deletion tool with duplicate/missing/reversed-anchor guards, delete_symbol is a fresh-read-gated Go AST symbol deletion tool with kind/parent disambiguation and multi-name-spec protection, text file tools preserve UTF-8 BOM and detected UTF-16 BOM/no-BOM encoding across read/write/edit/delete_range/delete_symbol while grep can scan UTF-16 text, read-only tools support explicit external reads under danger-full-access or configured read roots and can be repeated, read-root aliases resolve only for read tools and can display tokenized external paths, grep is a read-only streaming file search that supports glob filtering, context rows, columns, repo/nested .gitignore rules, and prunes hidden/dependency/VCS/protected dirs, glob is a read-only recursive file matcher with absolute external pattern support plus vendor/protected-dir pruning, code_index is a read-only lightweight symbol index with stable sorting/filtering and protected-dir pruning, web_fetch is a config-gated read-only fetch tool with SSRF guards, bounded body reads, proxy support, and HTML text extraction, every provider call that resolves to a physical side effect receives a durable semantic side-effect intent before dispatch so an equivalent second call is blocked before execution, inline skill reads remain repeatable, and failure-storm guard annotates the third consecutive same tool failure with a visible loop_guard nudge.",
			"covered-runtime-renderer-capability-diff-multi-edit-move-file-notebook-edit-delete-range-delete-symbol-grep-glob-code-index-web-fetch-repeat-failure-storm-and-utf16-contract",
			[]string{"packages/runtime-go/internal/server/tool_catalog.go", "packages/runtime-go/internal/diff/diff.go", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/components/chat/message-timeline-process.tsx", "src/renderer/src/components/settings-section-agents.tsx", "src/renderer/src/components/plugin-marketplace-runtime.ts"},
			[]string{"packages/runtime-go/internal/server/tool_catalog.go", "packages/runtime-go/internal/diff/diff.go", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/components/chat/message-timeline-process.tsx", "src/renderer/src/components/settings-section-agents.tsx", "src/renderer/src/components/plugin-marketplace-runtime.ts"},
			[]string{"packages/runtime-go/internal/server/capabilities_prod_test.go", "packages/runtime-go/internal/diff/diff_test.go", "packages/runtime-go/runtime_server_test.go", "src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/components/settings-section-agents.test.ts", "src/renderer/src/components/plugin-marketplace-runtime.test.ts", "npm run runtime:go:product-regression -- --json"},
			"future tool descriptions need cache review; renderer diagnostics intentionally filter validation-only MCP/tool markers; diff metadata, multi-edit schema aliases, move_file schema, notebook_edit schema, delete_range schema, delete_symbol schema, grep schema, glob schema, code_index schema, and web_fetch schema are provider-visible only when the related tool is advertised; grep behavior now prunes nested dependency/VCS/cache/hidden dirs on recursive workspace searches while preserving direct explicit file/root reads, supports legacy TypeScript-compatible glob/context/column fields, and honors repo/nested .gitignore patterns without claiming global Git excludes parity; external read-root aliases are read-only and intentionally not resolved by writers, so editing configured allow_write roots still uses explicit local paths; file tools now preserve UTF-8 BOM and detected UTF-16 BOM/no-BOM text but do not yet claim GB18030/global Git excludes/notebook non-UTF JSON parity; delete_symbol is Go-only and tells models to use delete_range for non-Go files; code_index is a lightweight fallback rather than LSP/call-graph semantics; web_fetch remains disabled by default and requires capabilities.web.enabled/fetchEnabled; repeat guard is intentionally per-turn and only counts successful write-like calls; failure storm guard only annotates failed tool results and excludes policy/sandbox/approval blocks",
			"medium",
			"all tool-capable providers",
			"tool schema hash, capability diagnostics, read/edit safety, bounded diff generation, atomic multi-edit execution, move execution path policy, notebook cell editing, exact-anchor range deletion, Go AST symbol deletion, UTF-16 text preservation, streaming grep search/pruning/context/gitignore, recursive glob matching, symbol indexing, web fetch policy/proxy/text extraction, write-loop suppression, and failure-storm recovery visibility",
		),
		productRow("plan mode create_plan", "Plan-mode turns advertise create_plan only in Plan/GUI plan context, keep the investigation toolset read-only, reject forged write/bash/task calls, save GUI-owned Markdown plans under .analytixsdd/plan, drop stale GUI plan contexts back to a normal agent turn, and keep the selected provider/model when the user starts a new plan with a non-DeepSeek model.", "Reasonix-style tool policy filtering and loop continuation are absorbed into the Go runtime while preserving Analytix create_plan semantics. Provider routing stays product-owned: MiMo/Xiaomi plan turns must use the selected OpenAI-compatible profile, resolve deprecated model aliases through provider modelProfiles, and keep the chat-completions endpoint instead of falling back to DeepSeek.", "covered-go-plan-mimo-routing-contract", []string{"packages/runtime-go/internal/server/plan_tools.go", "packages/runtime-go/internal/provider/provider.go", "src/renderer/src/agent/analytix-runtime.ts"}, []string{"packages/runtime-go/internal/server/plan_tools.go", "packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/runtime_server_test.go", "src/renderer/src/agent/analytix-runtime.ts"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime-go/internal/provider/provider_test.go", "src/renderer/src/agent/analytix-runtime.test.ts", "npm run runtime:go:product-regression -- --json"}, "future planner changes must preserve normal-turn create_plan isolation, stale GUI plan downgrade, stable tool schema outside Plan context, and MiMo/Xiaomi provider routing/model alias normalization; current Analytix intentionally canonicalizes deprecated model aliases before provider send", "low", "all tool-capable providers", "Plan-context tool schema, local plan write path, and renderer/runtime request provider/model selection"),
		productRow("cache-first review gate", "Cache-sensitive product surfaces must declare impact and focused guards before being treated as complete.", "Reasonix cache-first contribution rule is mapped to Analytix provider, tool, MCP, history, attachment, settings, and runtime-event surfaces.", "covered-engineering-gate", []string{"scripts/cache-first-review-gate.mjs"}, []string{"scripts/cache-first-review-gate.mjs"}, []string{"src/main/cache-first-review-gate.test.ts"}, "local product-regression now runs the cache-first review gate test; review metadata still depends on maintainer discipline outside local tests", "high", "DeepSeek/OpenAI-compatible/Anthropic/messages/custom", "first token, SSE, renderer buffering, tool progress"),
		productRow("MCP stdio http", "No MCP config means unavailable/empty; configured MCP uses product diagnostics only.", "Production MCP manager supports stdio/http, reconnect, redaction, stable catalog fingerprints, prompts/resources catalog diagnostics, tracked stdio process-tree containment, and runtime turn-level tool loops.", "covered-production-contract", []string{"packages/runtime-go/internal/mcp/manager.go", "packages/runtime-go/internal/adapters/outbound/mcp/stdio/process.go", "packages/runtime-go/internal/server/tools_execution.go"}, []string{"packages/runtime-go/internal/mcp/manager.go", "packages/runtime-go/internal/adapters/outbound/mcp/stdio/process.go", "packages/runtime-go/internal/proc/kill_other.go", "packages/runtime-go/internal/proc/kill_windows.go", "packages/runtime-go/internal/server/tools_execution.go", "scripts/runtime-go-speed-cache-gate.mjs"}, []string{"packages/runtime-go/internal/mcp/manager_test.go", "packages/runtime-go/internal/adapters/outbound/mcp/stdio/process_test.go", "packages/runtime-go/internal/proc/kill_other_test.go", "packages/runtime-go/internal/proc/kill_windows_test.go", "packages/runtime-go/internal/server/capabilities_prod_test.go", "packages/runtime-go/runtime_server_test.go", "npm run runtime:go:speed-cache-gate"}, "credentialed user MCP smoke and Windows Job Object execution remain environment-dependent; prompts/resources are diagnostics-only and do not alter provider-visible tool schemas", "high", "all providers", "tool schema cache, reconnect, prompts/resources catalog, process cleanup, and tool loop"),
		productRow("subagent task parallel durable", "Kun child delegation is available through Analytix tools/settings/timeline, and skills marked runAs=subagent execute as isolated child runs rather than a foreign product entry.", "Task, parallel_tasks, run_skill with runAs=subagent, durable child runs, GUI-managed/default profiles, per-subagent model/effort/tool scope, dependency result handoff, continue/fork locking, ancestor fork_from, parent/child links, subagent usage/cache attribution across restart, and background job hooks are runtime-owned.", "covered-runtime-settings-and-timeline-contract", []string{"packages/runtime-go/internal/server/subagent_jobs.go", "packages/runtime-go/internal/jobs/lineage.go", "src/shared/app-settings-runtime.ts", "src/renderer/src/components/settings-section-agents.tsx", "src/renderer/src/components/chat/message-timeline-tools.ts"}, []string{"packages/runtime-go/internal/server/subagent_jobs.go", "packages/runtime-go/internal/jobs/lineage.go", "scripts/runtime-go-speed-cache-gate.mjs", "scripts/runtime-go-product-regression.mjs", "src/shared/app-settings-runtime.ts", "src/renderer/src/components/settings-section-agents.tsx", "src/renderer/src/components/chat/message-timeline-tools.ts", "src/renderer/src/components/chat/message-timeline-process.tsx"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime-go/internal/server/capabilities_prod_test.go", "src/renderer/src/components/settings-section-agents.test.ts", "src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts", "npm run runtime:go:speed-cache-gate", "npm run runtime:go:product-regression"}, "nested child-agent/job drill-down is now renderer-visible from structured metadata; remaining risk is actual visual QA for deeply nested child transcripts beyond the summarized timeline rows", "medium", "all providers", "tool schema, tool progress, and background jobs"),
		productRow("background shell jobs", "Long shell commands must not block the parent turn and must stay under existing approval/sandbox/product timeline contracts.", "Reasonix background job model is absorbed as bash(run_in_background), durable output, wait/output/kill, wait-all for the current parent thread, Reasonix-style timeout_seconds alias, structured stalled diagnostics, process-group cancel/reap, and foreground-only subagent bash.", "covered-runtime-contract", []string{"packages/runtime-go/internal/server/terminal.go", "packages/runtime-go/internal/jobs/lineage.go", "packages/runtime-go/internal/proc/kill_other.go", "packages/runtime-go/internal/proc/kill_windows.go"}, []string{"packages/runtime-go/internal/server/terminal.go", "packages/runtime-go/internal/server/capabilities.go", "packages/runtime-go/internal/jobs/lineage.go", "packages/runtime-go/internal/proc/kill_other.go", "packages/runtime-go/internal/proc/kill_windows.go"}, []string{"packages/runtime-go/runtime_server_test.go", "packages/runtime-go/internal/server/bash_reap_test.go", "packages/runtime-go/internal/server/task_job_diagnostics_test.go", "packages/runtime-go/internal/jobs/lineage_test.go", "packages/runtime-go/internal/proc/kill_other_test.go", "packages/runtime-go/internal/proc/kill_windows_test.go"}, "Windows Job Object containment is implemented and cross-compiled; execution on an approved Windows host remains unverified", "medium", "DeepSeek/OpenAI-compatible/custom tool schema; subagent schema remains foreground-only", "detached command progress, stalled diagnostics, process cleanup, and output reads"),
		productRow("renderer timeline cards", "Timeline cards render tool, approval, user-input, usage, child-run, and error events; an early partial tool card must merge with the later completed result for the same call id; completed turns keep tool-preface assistant narration inside the expanded process timeline and surface only the final assistant segment as the answer body.", "Typed events give the renderer partial tool dispatch, progress, and terminal result states without splitting one provider tool call into duplicate cards.", "covered-renderer-contract", []string{"src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/thread/projection/derive-turn-sections.ts", "src/renderer/src/components/chat", "src/renderer/src/store/chat-store-runtime.ts", "scripts/runtime-go-packaged-session-soak.mjs"}, []string{"src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/thread/projection/derive-turn-sections.ts", "src/renderer/src/components/chat", "src/renderer/src/store/chat-store-runtime.ts", "scripts/runtime-go-product-regression.mjs", "scripts/runtime-go-packaged-session-soak.mjs"}, []string{"src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/components/chat/derive-turn-sections.test.ts", "src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts", "src/renderer/src/store/chat-store-runtime.test.ts", "npm run runtime:go:packaged-soak -- --json --actual"}, "static renderer tests cover component rendering, Kun final-answer boundary, and camel/snake Go lifecycle field shapes; actual packaged soak now covers runtime->main/preload bridge->renderer SSE replay for tool timeline events with a local contract provider", "low", "all providers", "partial tool dispatch, renderer frame scheduler, final-answer boundary, snake/camel tool lifecycle fields, attachment local file paths, and packaged SSE replay"),
		productRow("error display", "Provider/runtime errors remain visible and actionable in formatter, store, and timeline rows without leaking secrets.", "Retry, interrupted stream recovery, auth/configuration, insufficient-balance provider errors, and empty-final recovery emit visible events with sanitized diagnostics.", "covered-renderer-provider-contract", []string{"src/renderer/src/lib/format-runtime-error.ts", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts", "src/renderer/src/components/chat/message-timeline-tools.ts", "packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/server/terminal.go"}, []string{"src/renderer/src/lib/format-runtime-error.ts", "src/renderer/src/agent/analytix-mapper.ts", "src/renderer/src/store/chat-store-runtime.ts", "src/renderer/src/components/chat/message-timeline-tools.ts", "packages/runtime-go/internal/provider/provider.go", "packages/runtime-go/internal/server/terminal.go"}, []string{"src/renderer/src/lib/format-runtime-error.test.ts", "src/renderer/src/agent/analytix-mapper.test.ts", "src/renderer/src/store/chat-store-runtime.test.ts", "src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts", "packages/runtime-go/internal/provider/provider_test.go", "packages/runtime-go/runtime_server_test.go"}, "provider-specific error copy evolves with presets; visual styling remains covered by renderer static tests; recovered interrupted streams show provider_retrying rather than terminal provider_error", "low", "all providers", "retry, recovery, and provider error timeline path"),
		productRow("dev packaged smoke", "Development and packaged app startup keep the same runtime boundary, ports, provider profile settings contract, Xiaomi/MiMo top-level runtime settings, packaged session turn path, SSE replay, tool timeline, fork/resume, and usage evidence.", "Runtime health/info/tools/event paths are deterministic enough for smoke automation while packaged session soak uses a local contract provider to avoid external network credentials.", "covered-by-smoke-scripts; actual packaged smoke additionally requires settingsProfilePatchAccepted:true and mimoProfileSettingsAccepted:true, and actual packaged session soak requires toolTimelineOk:true", []string{"scripts/runtime-go-validation-command.mjs", "scripts/runtime-go-performance-check.mjs", "scripts/runtime-go-packaged-gui-smoke.mjs", "scripts/runtime-go-packaged-session-soak.mjs"}, []string{"scripts/runtime-go-validation-command.mjs", "scripts/runtime-go-performance-check.mjs", "scripts/runtime-go-packaged-gui-smoke.mjs", "scripts/runtime-go-packaged-session-soak.mjs", "scripts/runtime-go-engine-absorption-report.mjs"}, []string{"src/main/runtime-go-packaged-contract-report.test.ts", "src/main/runtime-go-engine-absorption-report.test.ts", "npm run runtime:go:packaged-gui-smoke -- --json", "npm run runtime:go:packaged-gui-smoke -- --json --actual", "npm run runtime:go:packaged-soak -- --json --actual", "npm run typecheck", "npm run build:runtime", "npm run test", "go test ./...", "go test -tags analytix_prod ./..."}, "packaged GUI launch still depends on local OS app smoke; stale dist packages may fail actual gate until rebuilt", "low", "all providers", "startup/SSE bridge/MiMo settings/tool timeline"),
	}
}

func productRow(feature, baseline, engine, status string, evidence, repairFiles, tests []string, risk, cacheImpact, providerImpact, speedImpact string) productRegressionRow {
	metadata := productRegressionMetadataByFeature[feature]
	return productRegressionRow{
		ContractID:           metadata.ContractID,
		Priority:             metadata.Priority,
		Required:             metadata.Required,
		GateIDs:              append([]string{}, metadata.GateIDs...),
		Feature:              feature,
		KunAnalytixBaseline:  baseline,
		ReasonixEngine:       engine,
		CurrentStatus:        status,
		Evidence:             evidence,
		RepairFiles:          repairFiles,
		Tests:                tests,
		Risk:                 risk,
		CacheImpact:          cacheImpact,
		ProviderCompatImpact: providerImpact,
		SpeedImpact:          speedImpact,
	}
}

func TestProductRegressionMatrixCoversRequiredProductSurfaces(t *testing.T) {
	matrix := productRegressionMatrix()
	required := []string{
		"app startup",
		"Go runtime default",
		"TypeScript retired backend",
		"window.analytix bridge",
		"new session",
		"plain text chat",
		"streaming first and delta events",
		"speed cache parity gate",
		"DeepSeek provider",
		"OpenAI-compatible provider",
		"Anthropic messages provider",
		"provider profile settings",
		"model list and provider probe",
		"thread list search archive",
		"thread rewind",
		"compaction history hygiene",
		"workspace checkpoint id",
		"desktop git checkpoint",
		"checkpoint file rewind apply",
		"resume and fork",
		"SSE replay",
		"usage cost and input composer usage",
		"attachments",
		"image vision payload",
		"approval",
		"user input",
		"built-in tools",
		"plan mode create_plan",
		"cache-first review gate",
		"MCP stdio http",
		"subagent task parallel durable",
		"background shell jobs",
		"renderer timeline cards",
		"error display",
		"dev packaged smoke",
	}
	rows := map[string]productRegressionRow{}
	contractIDs := map[string]string{}
	contractIDPattern := regexp.MustCompile(`^p[0-2]-[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, row := range matrix {
		if row.ContractID == "" ||
			row.Priority == "" ||
			len(row.GateIDs) == 0 ||
			!row.Required {
			t.Fatalf("product regression row missing contract metadata: %#v", row)
		}
		if !contractIDPattern.MatchString(row.ContractID) {
			t.Fatalf("product regression row has invalid contract id %q: %#v", row.ContractID, row)
		}
		if row.Priority != "P0" && row.Priority != "P1" && row.Priority != "P2" {
			t.Fatalf("product regression row has invalid priority %q: %#v", row.Priority, row)
		}
		if previous, exists := contractIDs[row.ContractID]; exists {
			t.Fatalf("duplicate product regression contract id %s for %s and %s", row.ContractID, previous, row.Feature)
		}
		contractIDs[row.ContractID] = row.Feature
		if row.Feature == "" ||
			row.KunAnalytixBaseline == "" ||
			row.ReasonixEngine == "" ||
			row.CurrentStatus == "" ||
			row.Risk == "" ||
			row.CacheImpact == "" ||
			row.ProviderCompatImpact == "" ||
			row.SpeedImpact == "" ||
			len(row.Evidence) == 0 ||
			len(row.RepairFiles) == 0 ||
			len(row.Tests) == 0 {
			t.Fatalf("product regression row missing required columns: %#v", row)
		}
		if _, exists := rows[row.Feature]; exists {
			t.Fatalf("duplicate product regression row: %s", row.Feature)
		}
		rows[row.Feature] = row
	}
	for _, feature := range required {
		if _, ok := rows[feature]; !ok {
			t.Fatalf("required product regression surface missing: %s", feature)
		}
	}
	for feature := range productRegressionMetadataByFeature {
		if _, ok := rows[feature]; !ok {
			t.Fatalf("product regression metadata references missing feature: %s", feature)
		}
	}
	requiredPriorities := map[string]bool{"P0": false, "P1": false, "P2": false}
	for _, row := range matrix {
		requiredPriorities[row.Priority] = true
	}
	for priority, seen := range requiredPriorities {
		if !seen {
			t.Fatalf("product regression matrix must include at least one %s contract", priority)
		}
	}
	raw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(raw))
	for _, forbidden := range []string{"d024", "d025", "oracle", "fixture", "fake provider", "fake mcp", "mcplocalproof", "contractproof", "reasonix public"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("formal product regression matrix must not depend on temporary evidence markers %q: %s", forbidden, string(raw))
		}
	}
}

func TestProductRegressionMatrixGateIDsResolveToShortContracts(t *testing.T) {
	commandIDs := map[string]bool{}
	for _, scriptPath := range []string{
		"../../../../scripts/runtime-go-product-regression.mjs",
		"../../../../scripts/runtime-go-speed-cache-gate.mjs",
	} {
		raw, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("read validation script %s: %v", scriptPath, err)
		}
		for _, match := range regexp.MustCompile(`id:\s*'([^']+)'`).FindAllStringSubmatch(string(raw), -1) {
			commandIDs[match[1]] = true
		}
	}
	aliases := map[string]string{
		"app-settings-provider":            "p0-provider-settings-probe-contract",
		"provider-connection":              "p0-provider-settings-probe-contract",
		"preload-runtime-request":          "desktop-git-checkpoint",
		"runtime-go-health-smoke":          "p0-runtime-health-smoke",
		"runtime-go-packaged-session-soak": "p0-packaged-session-soak-contract",
		"runtime-go-product-regression":    "production-product-regression-matrix",
		"runtime-go-speed-cache-gate":      "p0-speed-cache-gate",
	}
	var unresolved []string
	for _, row := range productRegressionMatrix() {
		for _, gateID := range row.GateIDs {
			if commandIDs[gateID] {
				continue
			}
			alias, ok := aliases[gateID]
			if ok && commandIDs[alias] {
				continue
			}
			unresolved = append(unresolved, row.Feature+":"+gateID)
		}
	}
	if len(unresolved) > 0 {
		t.Fatalf("product regression gate ids must resolve to formal short contracts: %s", strings.Join(unresolved, ", "))
	}
}

func TestProductRegressionMatrixReferencesResolve(t *testing.T) {
	repoRoot := filepath.Clean("../../../../")
	rootPackageJSON := productRegressionJSONFile(t, filepath.Join(repoRoot, "package.json"))
	rootScripts := productRegressionStringMap(productRegressionMapValue(rootPackageJSON["scripts"]))
	var unresolved []string

	for _, row := range productRegressionMatrix() {
		for _, entry := range append(append([]string{}, row.Evidence...), append(row.RepairFiles, row.Tests...)...) {
			if !productRegressionReferenceResolves(repoRoot, rootScripts, entry) {
				unresolved = append(unresolved, row.Feature+":"+entry)
			}
		}
	}
	if len(unresolved) > 0 {
		t.Fatalf("product regression matrix references must resolve to files or known commands: %s", strings.Join(unresolved, ", "))
	}
}

func productRegressionReferenceResolves(repoRoot string, rootScripts map[string]string, entry string) bool {
	entry = strings.TrimSpace(entry)
	switch {
	case entry == "":
		return false
	case strings.HasPrefix(entry, "npm run "):
		parts := strings.Fields(entry)
		return len(parts) >= 3 && rootScripts[parts[2]] != ""
	case strings.HasPrefix(entry, "node "):
		parts := strings.Fields(entry)
		return len(parts) >= 2 && productRegressionPathExists(repoRoot, parts[1])
	case strings.HasPrefix(entry, "go test "):
		return true
	default:
		return productRegressionPathExists(repoRoot, entry)
	}
}

func productRegressionPathExists(repoRoot string, entry string) bool {
	clean := filepath.Clean(entry)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return false
	}
	_, err := os.Stat(filepath.Join(repoRoot, clean))
	return err == nil
}

func productRegressionJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return parsed
}

func productRegressionMapValue(value any) map[string]any {
	record, _ := value.(map[string]any)
	return record
}

func productRegressionStringMap(record map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range record {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func TestProductRegressionMatrixJSONSnapshot(t *testing.T) {
	raw, err := json.Marshal(productRegressionMatrix())
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(productRegressionMatrix())
	if err != nil || len(raw) == 0 || !bytes.Equal(raw, second) {
		t.Fatalf("product regression matrix snapshot is not deterministic: err=%v", err)
	}
	if os.Getenv("ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON") == "1" {
		t.Logf("ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON=%s", raw)
	}
}

func TestProductRegressionMatrixProductionRuntimeSignals(t *testing.T) {
	server := httptest.NewServer(NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
	}))
	defer server.Close()

	health := productRegressionJSON(t, server.URL+"/health", "")
	if health["service"] != "analytix" || health["mode"] != "serve" || health["status"] != "ok" {
		t.Fatalf("health must expose the analytix serve contract: %#v", health)
	}

	info := productionRuntimeJSON(t, server.URL+"/v1/runtime/info")
	capabilities := productRegressionMapField(t, info, "capabilities")
	mcp := productRegressionMapField(t, capabilities, "mcp")
	catalog := productRegressionMapField(t, mcp, "catalog")
	if boolField(mcp, "available") || boolField(mcp, "enabled") ||
		stringField(mcp, "status") != "disabled" || stringField(mcp, "reasonCode") != "disabled_by_config" ||
		mcp["configuredServers"] != float64(0) || mcp["connectedServers"] != float64(0) || mcp["toolCount"] != float64(0) ||
		stringField(catalog, "status") != "unavailable" || stringField(catalog, "reasonCode") != "unavailable" ||
		catalog["toolCount"] != float64(0) || boolField(catalog, "catalogDrift") {
		t.Fatalf("unconfigured production MCP must be explicitly disabled with an unavailable empty catalog: %#v", mcp)
	}
	attachments := productRegressionMapField(t, capabilities, "attachments")
	if !boolField(attachments, "available") || !boolField(attachments, "enabled") {
		t.Fatalf("persistent attachment store must be available in the Go runtime contract: %#v", attachments)
	}
	subagents := productRegressionMapField(t, capabilities, "subagents")
	if !boolField(subagents, "available") ||
		!boolField(subagents, "taskToolAvailable") ||
		!boolField(subagents, "parallelTasksToolAvailable") ||
		!boolField(subagents, "durableChildRunStore") ||
		boolField(subagents, "topLevelRouteExposed") {
		t.Fatalf("subagent capability must stay real but nested under the Analytix contract: %#v", subagents)
	}
	assertNoProductionRuntimeMarkers(t, "product regression runtime info", info)

	tools := productionRuntimeJSON(t, server.URL+"/v1/runtime/tools?refresh=1")
	networkProxy := productRegressionMapField(t, tools, "networkProxy")
	if boolField(networkProxy, "configured") ||
		networkProxy["valid"] != true ||
		stringField(networkProxy, "source") != "environment" ||
		stringField(networkProxy, "mode") != "auto" ||
		networkProxy["credentialsMasked"] != true {
		t.Fatalf("unconfigured production runtime tools must expose auto/env network proxy diagnostics: %#v", tools)
	}
	if _, exists := networkProxy["summary"]; exists {
		t.Fatalf("public proxy diagnostics must not expose an unbounded summary: %#v", networkProxy)
	}
	mcpSearch := productRegressionMapField(t, tools, "mcpSearch")
	if boolField(mcpSearch, "enabled") ||
		stringField(mcpSearch, "mode") != "auto" ||
		boolField(mcpSearch, "active") ||
		boolField(mcpSearch, "available") ||
		floatFromAny(mcpSearch["indexedToolCount"]) != 0 ||
		floatFromAny(mcpSearch["advertisedToolCount"]) != 0 {
		t.Fatalf("unconfigured production runtime tools must not expose MCP tools: %#v", tools)
	}
	toolSubagents := productRegressionMapField(t, tools, "subagents")
	if !boolField(toolSubagents, "available") || boolField(toolSubagents, "topLevelRouteExposed") {
		t.Fatalf("runtime tools must expose nested subagent diagnostics without public product routes: %#v", toolSubagents)
	}
	toolContracts := productRegressionMapField(t, tools, "toolContracts")
	if floatFromAny(toolContracts["count"]) == 0 || len(stringField(toolContracts, "catalogHash")) != 64 {
		t.Fatalf("runtime tools must expose a closed tool-catalog count and hash: %#v", toolContracts)
	}
	commands, ok := tools["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("runtime tools must expose command diagnostics: %#v", tools["commands"])
	}
	hasCommandStatus := false
	for _, raw := range commands {
		diagnostic, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("command diagnostic is not an object: %#v", raw)
		}
		_, hasFound := diagnostic["found"]
		if stringField(diagnostic, "binary") != "" && hasFound {
			hasCommandStatus = true
		}
	}
	if !hasCommandStatus {
		t.Fatalf("command diagnostics must include binary/found status: %#v", commands)
	}
	if floatFromAny(tools["mcpPromptCount"]) != 0 || floatFromAny(tools["mcpResourceCount"]) != 0 {
		t.Fatalf("runtime tools must expose empty MCP prompt/resource counts: %#v", tools)
	}
	for _, retired := range []string{"mcpPrompts", "mcpResources"} {
		if _, exists := tools[retired]; exists {
			t.Fatalf("runtime tools exposed retired raw field %s: %#v", retired, tools)
		}
	}
	assertNoProductionRuntimeMarkers(t, "product regression runtime tools", tools)

	for _, forbidden := range []string{"/v1/workflow", "/v1/workflows", "/v1/create-loop", "/v1/subagents", "/v1/autoresearch", "/v1/mcp-indexer", "/v1/reasonix", "/v1/conformance"} {
		response := productRegressionJSON(t, server.URL+forbidden, DefaultRuntimeToken)
		if response["code"] != "not_found" {
			t.Fatalf("forbidden top-level route %s must remain hidden: %#v", forbidden, response)
		}
	}
}

func productRegressionJSON(t *testing.T, url string, token string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status %d for %s: %#v", res.StatusCode, url, body)
	}
	return body
}

func productRegressionMapField(t *testing.T, record map[string]any, field string) map[string]any {
	t.Helper()
	value, ok := record[field].(map[string]any)
	if !ok {
		t.Fatalf("field %s is not an object: %#v", field, record[field])
	}
	return value
}
