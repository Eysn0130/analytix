# analytix runtime-go

This package contains the default Go runtime core for analytix.
Electron main selects `go-runtime-default` when `ANALYTIX_RUNTIME_BACKEND` is
unset, `analytix`, `go`, `go-runtime`, or `go-runtime-default`.
`ANALYTIX_RUNTIME_BACKEND=typescript` is retained only as a retired-backend
diagnostic path and must not start the TypeScript runtime.

Reasonix is absorbed here only as runtime engine strength. The package must not
expose upstream product entries, upstream session/config protocols, a
renderer-visible Go switcher, Workflow/Create Loop/Subagent product shells, or
an MCP-indexer public route.

## Current Shape

The production runtime server is the product path. It owns:

- `GET /health`
- `GET /v1/runtime/info`
- `GET /v1/runtime/tools`
- thread create/list/read/patch/delete/fork/resume routes
- SSE replay and live event streaming
- turns, approvals, user inputs, goals, todos, usage, attachments, memory,
  compact/review, steer/interrupt, and checkpoint rewind audit routes
- MCP diagnostics and tool catalog metadata without public upstream routes

The Go runtime keeps the analytix product contract while absorbing
reference engine provider parsing, cache diagnostics, stream handling, tool loop
repair, MCP lifecycle handling, background jobs, and sub-agent orchestration
inside the runtime core.

## Production Boundaries

Production serve mode must use:

- real provider configuration
- a real HTTP client
- real runtime stores
- the production MCP/tool manager
- unavailable/empty MCP diagnostics when no MCP configuration exists

Production serve mode must not start local provider simulators, load default
MCP sample specs, expose contract replay routes as product capabilities, or
publish temporary implementation names in `/v1/runtime/info` or
`/v1/runtime/tools`.

Contract replay handlers and sidecars are kept for tests only. They live behind
test/conformance entry points and are not a product API.

## Internal Packages

- `internal/domain/*`: pure runtime value models for provider requests, MCP
  specs, events, threads, tools, jobs, sub-agents, and goal evidence. These
  packages must not own HTTP, filesystem, process, or adapter side effects.
  Provider endpoint normalization, provider-family classification, stable
  hashing, and canonical tool-schema JSON helpers now live in
  `internal/domain/model` so app services can reuse them without importing the
  transitional server package.
- `internal/ports`: application-facing interfaces for event replay/recording,
  provider calls, MCP, jobs, process execution, and clocks.
- `internal/app/*`: use-case service seams for terminal jobs, sub-agents,
  tool catalogs, turn lifecycle, loop control, goal/todo closure, plan-mode
  create_plan target resolution, sessions,
  model resolution, MCP, and usage aggregation. These packages depend on domain and ports only; turn
  lifecycle record/event construction such as turn start, turn completion,
  failure item/event construction, tool-call ready item/event construction, pending gate request/continuation/cancellation field construction,
  assistant-delta terminal materialization, and
  attachment model-input resolution/authorization now live in
  `internal/app/turn` rather than in the HTTP handler package, runtime
  capability-state projection for default model metadata, attachments, memory,
  MCP search/catalog, subagents, computer-use, web, and vision bridge now lives in
  `internal/app/runtimeinfo`, goal/todo tool service orchestration, completion
  response shaping, token-budget parsing, host-evidence path matching,
  blocked-reason normalization, todo step matching, and strict-completion self-check prompts now live in
  `internal/app/goal`, plan-mode create_plan fallback arguments, reserved/free-form
  target identity guards, path normalization, sandbox decision shaping, content
  hash response shaping, and Plan-mode tool filtering now live in
  `internal/app/plan`, and
  workspace status projection now lives in `internal/app/session`. Model
  execution refs, turn execution config resolution, provider config/model resolution, provider
  family/endpoint helpers, stable prefix-shape/cache diagnostics, provider
  history projection plus pending tool-call restore and tool-call/tool-result repair, tool schema materialization hashes,
  prompt-route/MCP-pinning/user-input normalization, runtime tool contract
  diagnostics, goal/todo tool schema contracts, approval/blocked-tool policy, sub-agent/job tool
  classification, sub-agent task request parsing, parallel-task DAG
  validation, wave execution, dependency handoff, aggregate result shaping,
  child display-name/output/usage/parallel
  metadata shaping, lifecycle event payload projection, child-thread summary
  and tool-count projection, sub-agent profile
  config defaults/merge/diagnostics/application, continuation/fork source identity guards, background task-job
  wait/output/kill request parsing, service orchestration for runtime tools and summary routes, response projection plus state registry/slot/cursor handling and context/diagnostic/output views, thread-summary subagent/task/status/display/output,
  command task/source/output projection, MCP search config defaults/merge/nested
  discovery, sandbox root list parsing, web config defaults/domain policy/fetch-limit parsing,
  vision bridge config defaults/probe-status parsing,
  builtin/goal/plan/subagent-job/skill/user-input tool schema materialization,
  MCP prompt-scoped catalog selection, tool progress settlement, tool result redaction/cancellation output,
  runtime command diagnostics,
  foreground bash request validation plus foreground/background bash service-owned durable start, runner, and result shaping, bounded terminal output buffering,
  agent-loop step-limit precedence/resolution, provider identity prompt shaping, case-fund tool-scope/final-answer recovery policy, interrupted-stream recovery classification and prompt shaping, provider retry/delay diagnostics, provider-progress event construction, retry/error redaction, repeat-success and failure-storm loop guards, read-file text view,
  UTF-8/UTF-16 codec normalization, exact text edit/delete-range planning,
  notebook edit normalization, code-index
  request parsing/symbol extraction/filtering/formatting, glob/grep pattern
  normalization/matching, and
  usage/cache event maps plus windowed usage aggregation now live
  under `internal/app/goal`, `internal/app/plan`, `internal/app/model`, `internal/app/subagent`, `internal/app/toolcatalog`,
  `internal/app/runtimeinfo`, `internal/app/terminal`, `internal/app/filetools`,
  `internal/app/loop`,
  `internal/app/threadsummary`, and
  `internal/app/usage` with server wrappers kept only for compatibility during
  migration; command diagnostics plus foreground/background bash execution consume
  process ports/adapters instead of importing OS process APIs directly.
- `internal/adapters/inbound/*`: inbound runtime protocols such as HTTP JSON
  routes and SSE replay/live encoding. Shared SSE heartbeat and setup-error
  event shape lives in `internal/adapters/inbound/sse`. Shared HTTP route
  matching, auth/dispatch shell, JSON response helpers,
  request-body/map-body validation helpers,
  attachment/memory/usage/workspace-status route response contracts, raw JSON
  responses, capability-state shapes, and
  durable SSE frame writers now live in
  `internal/adapters/inbound/httpapi` while `internal/server` keeps
  compatibility wrappers during route migration.
- `internal/server/path_policy.go`: transitional compatibility wrappers for
  workspace/write-root/protected-path checks. New path policy behavior belongs
  in `internal/adapters/outbound/filestore`, keeping `tool_catalog.go` focused
  on tool schema assembly while route/tool callers are migrated.
- `internal/adapters/outbound/*`: outbound implementations for file stores,
  provider wire clients, MCP transports, schema caches, and redaction. JSONL
  line reading plus append/atomic rewrite helpers, JSON map-file read/write,
  text-file read/decode, directory listing/find/glob walking, code-index file
  discovery and parsing IO,
  gitignore matcher/repo-root walking plus bounded grep file scanning/context-window
  collection, symlink-aware workspace/write-root/protected-path policy, persistent attachment/memory records, and legacy durable directory copy helpers now start in
  `internal/adapters/outbound/filestore` and are consumed through transitional
  server wrappers while the durable store is migrated. OS command probing for
  runtime diagnostics lives in `internal/adapters/outbound/process` behind
  `ports.CommandProbe`; workspace status git/filesystem probing lives there
  behind `ports.WorkspaceStatusProbe`. Provider request URL/body/header
  assembly now starts in
  `internal/adapters/outbound/provider/{client,openai,responses,anthropic,compat,schema,stream,usage}`
  so OpenAI chat completions, OpenAI responses, Anthropic messages, custom full
  endpoint paths, reasoning fields, canonical tool schema shapes, usage
  accounting, pricing, cache diagnostics, HTTP stream execution/reconnect,
  provider error redaction, stream chunk parsing, tool-call assembly, and cache
  telemetry extraction have adapter-owned regression
  coverage while the transitional provider facade delegates through
  compatibility wrappers. MCP HTTP client/request wiring, stdio process startup, shared
  JSON-RPC/catalog protocol decoding, schema-cache persistence, and
  diagnostic/catalog redaction now live in
  `internal/adapters/outbound/mcp/{http,stdio,protocol,cache,redaction}` while
  the transitional MCP manager delegates through compatibility wrappers.
  Web fetch request parsing, URL policy, SSRF/IP guards, proxy-aware transport,
  bounded HTTP execution, URL redaction, source identity, telemetry, and
  HTML/plain-text extraction now live in
  `internal/adapters/outbound/webfetch`, with the server retaining only
  runtime-tool orchestration and progress recording.
- `internal/server/durable_*.go`: transitional durable store split anchors.
  Core construction, event log/replay, goal/todo state, thread/session
  mutations, turn/compaction settlement, and restart/sidecar recovery now live
  in separate same-package files while the store migrates toward outbound
  repositories and app services.
- `internal/server/runtime_server.go`: compatibility facade only. The old
  same-package test factory is isolated in `runtime_compat_factory.go`, and
  concrete handler wiring is centralized behind `runtime_components.go` so
  runtimeapp can own production assembly.
- `internal/runtimeapp`: the runtime composition root used by `analytix serve`
  and the root compatibility shims. It now assembles durable stores, attachment
  and memory stores, provider clients/config, MCP manager/catalog settings,
  job manager, sandbox/web/vision/step-limit config, and server handler
  components before handing the HTTP surface to the transitional server
  adapter. Legacy `internal/server` factories remain as compatibility entry
  points for same-package tests while production assembly enters through
  runtimeapp.
- `internal/architecture`: architecture guard tests that prevent domain/app
  imports from drifting back into HTTP, adapter, filesystem, conformance, or
  product-route dependencies.
- `internal/app/loop`: the production provider/tool state machine, private
  draft handling, bounded recovery, continuation authority, and per-step live
  tool-catalog enforcement. `internal/server/agent_loop.go` only binds host
  adapters and immutable turn authority into this application service.
- `internal/agent`: approval/user-input gate state, runtime item shapes, and
  minimal loop contract helpers.
- `internal/contracts`: shared runtime contract helpers and product-boundary
  records.
- `internal/conformance`: `!analytix_prod` shadow/replay contract code such as
  G5 full-loop, control executable, usage/SSE replay, and upstream parity
  fixtures. These helpers are not part of the production HTTP/runtime surface.
- `internal/domain/goal`: pure goal/todo and evidence-audit value logic.
- `internal/goal`: compatibility re-export for older goal evidence contract tests.
- `internal/jobs`: parent/child run lineage and background job records.
- `internal/mcp`: MCP lifecycle, schema handling, manager tests, and contract
  replay handlers. It now delegates HTTP transport setup, stdio transport
  startup, JSON-RPC/catalog protocol decoding, schema-cache persistence, and
  diagnostics/catalog redaction to outbound MCP adapters.
- `internal/netclient`: HTTP client helpers for runtime production paths.
- `internal/proc`: process helpers for controlled runtime/job execution.
- `internal/provider`: transitional provider facade for stream execution,
  provider config compatibility, and provider matrix support. It now delegates
  provider config/model resolution to `internal/app/model` and request
  URL/body/header construction, HTTP stream execution/reconnect, provider
  error redaction, stream parsing, prefix-shape capture, and usage/cache
  diagnostics to app services and outbound provider adapters.
- `internal/readiness`: readiness checks, provider matrix, MCP matrix, and
  operator evidence validation.
- `internal/research`: AutoResearch state compatibility.
- `internal/server`: HTTP server, durable stores, runtime
  readiness, production capability metadata, agent-loop host wiring, sub-agent execution,
  usage, attachments, and SSE replay. This package is now a transitional
  compatibility facade with same-package split anchors: `runtime_server.go`
  holds handler construction wiring, `runtime_handler.go` owns handler state
  contracts, and behavior is split
  across `routes.go`, `thread_routes.go`,
  `turn_start.go`, `turn_cancel.go`, `turn_routes.go`,
  `turn_terminal.go`, `turn_finalize.go`, `runtime_restore.go`, `agent_loop.go`,
  `tools_execution.go`, `goal_tools.go`, `tool_settlement.go`,
  `subagent_jobs.go`, `native_read_tools.go`, `web_fetch_tool.go`,
  `terminal.go`, `file_mutation_tools.go`, `vision_bridge.go`,
  `runtime_helpers.go`, `runtime_info_routes.go`,
  `usage.go`, `capabilities.go`, `tool_catalog.go`, `skills.go`,
  `mcp_config.go`, `subagents_config.go`, `runtime_settings.go`,
  `checkpoint.go`, `checkpoint_routes.go`, `gates_routes.go`, and
  `durable_core.go`, `durable_event_log.go`, `durable_goals_todos.go`,
  `durable_threads.go`, `durable_turns.go`, `durable_recovery.go`, and
  `durable_steering.go`. These files are the migration anchors for moving
  behavior toward domain/app/ports/adapters without changing contracts.
- `internal/testsupport`: local provider and process helpers used only by
  tests.
- `internal/upstreamaudit`: source-level Kun/Reasonix absorption matrices used
  for audit reports, excluded from production builds.

Root-package shims remain only to preserve public test APIs. The implementation
belongs in the internal packages above.

## Runtime Gates

The formal validation commands are the current release gates:

- `npm run runtime:go:speed-cache-gate -- --json`
- `npm run runtime:go:product-regression -- --json`
- `npm run runtime:go:preflight -- --json`
- `npm run runtime:go:rc-control-plane -- --json`
- `npm run runtime:go:release-gate`
- `npm run runtime:go:live-evidence -- --json`
- `npm run runtime:go:packaged-gui-smoke -- --json`
- `npm run runtime:go:packaged-soak -- --json`
- `npm run qa:runtime:packaged -- --json`

`runtime:go:preflight` runs product regression, speed/cache, typecheck,
runtime build, and whitespace checks. It also reads the live evidence bundle
from `docs/analytix/upstreams/runtime-go-live-evidence`.

`runtime:go:rc-control-plane` runs the flattened source validation matrix and
may exit successfully only when `controlPlaneReady` is true. Its receipt never
claims product, package, or release PASS. `runtime:go:release-gate` is the
product packaging blocker and exits successfully only when its final JSON has
`passed: true`; unresolved artifact admission, active OpenSpec `RC_REQUIRED`
tasks, preflight, or authorized Electron/package/A0/B1 formal evidence keep it
non-zero even when `controlPlaneReady` is true.

The shared matrix runs product sovereignty, preflight with `--gate`, and
`go test -p=1 -count=1 -tags analytix_prod ./...`. Full-module gates serialize
Go packages because the root package already runs isolated internal shards;
this preserves every test while preventing nested package/process concurrency
from starving runtime startup checks.

The live evidence report records:

- provider matrix status for DeepSeek plus at least one non-DeepSeek provider
- MCP execution status
- packaged desktop QA status
- packaged GUI bridge smoke status
- packaged session soak status
- operator gate status
- `missingExternalInputIds`
- `finalGateBlocked`
- `finalGateBlockers`

Live evidence must not record credential values. Reports may record
`hasApiKey`, provider id, model, endpoint family, usage numbers, cache hit/miss
numbers, hashes, and pass/fail status.

## Provider Coverage

The provider matrix must cover:

- DeepSeek chat completions
- OpenAI-compatible chat completions
- Anthropic messages
- custom full endpoint mode

Settings resolution must read top-level `provider.activeProviderId` and
`provider.providers[]`, then select the matching provider profile for
`apiKey`, `baseUrl`, `endpointFormat`, and `models`. Environment variables are
accepted as an explicit live-validation override.

DeepSeek cache telemetry is provider scoped. Non-DeepSeek providers must not
receive DeepSeek-only request fields, and missing cache telemetry must remain
unknown instead of being reported as a known zero.

## MCP And Tools

No MCP configuration means unavailable/empty MCP capability metadata. It must
not fall back to a default sample catalog.

MCP validation covers stdio/http connection, tool discovery/search, tool call,
approval/user-input behavior, reconnect, redaction, and hidden upstream public
routes. Production capabilities report honest `available`/`enabled` state for
MCP, tools, skills, sub-agents, and background jobs.

## Sub-Agents

The Go runtime supports Analytix-owned sub-agent execution:

- `task` / delegate tool
- `parallel_tasks`
- durable child run transcript storage
- parent/child event linkage
- `continue_from` and `fork_from` semantics
- background child jobs
- per-sub-agent model, effort, prompt, and tool scope
- usage/cache attribution for sub-agent work
- recursion and meta-tool filtering
- safe foreground boundaries for shell/job tools

These capabilities are runtime internals and tool contracts. They do not expose
Reasonix product entry points.

## Local Validation

Useful checks while editing this package:

```sh
cd packages/runtime-go && gofmt -w <changed-go-files>
cd packages/runtime-go && go test -p=1 -count=1 ./...
npm run runtime:go:speed-cache-gate -- --json
npm run runtime:go:product-regression -- --json
npm run runtime:go:preflight -- --json
npm run runtime:go:rc-control-plane -- --json
npm run runtime:go:release-gate
npm run typecheck
npm run build:runtime
npm run test
git diff --check
```

Run the RC control plane only once after a source freeze; its success is not
product/package/release PASS. Current product release-candidate completion
requires the local gates, a release-gate result with final `passed: true`, and
current worktree evidence to pass. Additional live evidence for real
providers/MCP/operator review remains required for the claims that depend on
those external systems and for any future physical TypeScript source removal.
