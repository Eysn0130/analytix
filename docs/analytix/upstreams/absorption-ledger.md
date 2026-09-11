# analytix cross-upstream absorption ledger

Status: Reference / chronological batch ledger
Applies to: 跨上游吸收批次；不证明当前实现、parity 或 superiority
Source of truth for current behavior: current code, accepted specs, active
OpenSpec tasks and fresh validation
Current handover: [`../handovers/2026-07-25-first-stage-pause.md`](../handovers/2026-07-25-first-stage-pause.md)

Use this file when a change spans more than one upstream or affects the shared
analytix architecture.

Current status note (2026-06-25): Go runtime default delivery is active through
`go-runtime-default`. Current adapter code rejects
`ANALYTIX_RUNTIME_BACKEND=typescript` as a retired backend; older rows that say
TypeScript remains available as explicit rollback are historical evidence and
must not be used as current operator guidance. Entries before the final
delivery rows are historical stage records. Any older row that says Go remains
shadow-only, TypeScript remains the default, or live D-0251/D-0252/D-0253
evidence blocks default startup is superseded for current code by the final Go
runtime delivery report, current conformance plan, and current spec addendum.
D-0251/D-0252/D-0253 live provider/MCP/packaged/operator/soak evidence remains
post-cutover validation.

Governance note (2026-07-02): this ledger records decisions made under the
batch context of the row. `reject`, "forbidden", and "no top-level route"
entries are current-surface gates for that batch, not permanent bans on future
analytix-native capabilities. Reopen capability questions by comparing all
relevant upstreams and landing through explicit specs, UX rationale, tests,
desktop QA, and product-sovereignty scan updates.

## Active Batches

| Batch | Upstream(s) | Branch | Status | Owner | Notes |
| --- | --- | --- | --- | --- | --- |
| 2026-06-26 Formal rollback-retirement validation entrypoints | Reasonix / Kun / Go | Legacy D-0253 executable collector/report retired; formal `runtime-go-rollback-retirement-*` gates own rollback-retirement evidence | scoped code/test/docs closed; Go runtime default remains active; TypeScript backend retired in current adapter; historical D-0253 evidence files are archive data, not current validation entrypoints | Codex | Removed the old `scripts/d0253-retirement-evidence-collector.mjs`, `scripts/d0253-fallback-retirement-report.mjs`, and their legacy focused test. Added tracked formal report tests for `runtime-go-rollback-retirement-evidence`, `runtime-go-rollback-retirement-report`, and `runtime-go-engine-absorption-report`, and wired those tests into `runtime:go:product-regression` as `formal-runtime-report-contracts` so P5退场不依赖手工 targeted run. The current rollback-retirement report passed with packaged QA, `directLegacyExposureCount:0`, `internalLegacyDelegateCount:0`, and `temporaryGoProdViolationCount:0`; product-regression, speed/cache gate, product-sovereignty scan, typecheck, build:runtime, and diff-check passed through `runtime:go:preflight`. No renderer/preload/settings/UI product entry changed, no Reasonix public protocol was exposed, and physical TypeScript rollback code deletion remains a separate release decision. |
| 2026-06-25 Go default diagnostics startup fix | Reasonix / Kun / Go | Desktop adapter canary, stale Go runtime cleanup, and packaged/live evidence gates accept honest unavailable MCP/subagent diagnostics | scoped code/test/docs closed; Go runtime default remains active; no live MCP/subagent availability is claimed | Codex | Fixed the startup regression where `go-runtime-default` could be rejected because the main adapter still required `capabilities.mcp.available === true` and `capabilities.subagents.available === true` after the Go contract server correctly stopped advertising fake availability. The adapter now requires a boolean `available` field plus a non-empty `reason` when unavailable. D-0251/D-0252 QA fixture evidence now records `mcpDiagnosticHonest`/`subagentsDiagnosticHonest` without claiming availability. Managed Go startup recognizes stale Analytix Go `runtime-server` commands, waits after forced Go child termination, and creates an isolated canary thread so old durable threads cannot poison SSE replay. Follow-up `npm run dev` smoke built runtime/main/preload/renderer, started Electron, confirmed `/health` and `/v1/runtime/info` on `127.0.0.1:8901`, verified local fake-provider thread/turn/SSE/delete, and confirmed no `runtime-server` listener remained after exit. Kun/Analytix product layer, settings, bridge, UI, provider selection, and Reasonix engine/runtime-only boundary remained unchanged. |
| 2026-06-25 Go runtime contract equivalence closure | Reasonix / Kun / Go | Go default contract server turn body, attachments, memory, approval/user-input, and diagnostics parity | scoped code/test/docs closed; Go runtime default remains active; TypeScript backend retired in current adapter; no new live provider/MCP/packaged evidence claimed | Codex | Closed Go-default runtime contract regressions against the renderer and TypeScript turn contracts by preserving `mode`, `guiPlan`, `attachmentIds`, `fileReferences`, `approvalPolicy`, `sandboxMode`, `disableUserInput`, and `maxModelSteps` through turn create, durable thread state, user item projection, and SSE replay. Replaced process-local Go attachment and memory maps with `dataDir` persistent stores, including attachment content, metadata, hash, scope, `localFilePath`, diagnostics, and restart reads; memory now supports list/create/patch/delete/diagnostics across restart. Approval/user-input gates now respect policy, sandbox, headless/IM, and disabled-user-input semantics. Tools/skills/MCP diagnostics no longer advertise fake availability when real Go MCP/skills are not connected. Deterministic validations passed for Go tests, targeted runtime/renderer vitest, product-sovereignty scan, typecheck, runtime build, and full app build. Kun/Analytix product layer, settings, bridge, and UI remained unchanged, and Reasonix remains engine/runtime-only with no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entry. |
| 2026-06-25 Final Go runtime delivery validation | Reasonix / Kun / Go | Final Go default delivery report and empty-token insecure desktop fix | delivery acceptance passed; Go runtime default active; TypeScript backend retired in current adapter; remaining provider live keys/full packaged run are optional post-cutover validation | Codex | Added `docs/analytix/upstreams/final-go-runtime-delivery-report.md` with Electron dev app/runtime smoke, local Go runtime smoke, DeepSeek live provider probe, deterministic provider/bridge/settings/runtime tests, product-sovereignty scan, typecheck, runtime build, and full app build evidence. Aligned the Go default path with existing empty `runtime.runtimeToken` semantics by passing `--insecure` only when settings are explicitly insecure; bearer-token auth remains active when a runtime token exists. No API key material was written to tracked files, no renderer/preload/settings/UI product entry changed, no renderer-visible Go route was added, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. |
| 2026-06-24 D-0251 evidence secret scanner hardening | Reasonix / Kun / Go | D-0251 collector, D-0247/D-0252/D-0253 gates, and adapter G6 readiness now reject header-style and bare secret material | D-0251/D-0252/D-0253 remain blocked without real provider/MCP/packaged/operator evidence; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Expanded evidence redaction/scanning for `x-api-key`, API-key header fields, bearer/query tokens, and bare `sk-...` material. Failed provider probe errors are summarized after redaction before being written. Strict readiness and adapter gates reject evidence JSON containing header-style secret material, while redacted placeholders are accepted as safe summaries. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, and `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend. |
| 2026-06-24 D-0251/D-0253 strict evidence chain hardening | Reasonix / Kun / Go | Historical G6 strict chain, D-0252, and D-0253 require D-0251 protocol proof, packaged process/runtime proof, command artifact SHA256, canonical digests, report paths, review rows, and target commits; current release validation enters through `runtime:go:preflight` and `runtime:go:release-gate` | D-0251/D-0252/D-0253 remain blocked without real provider/MCP/packaged/operator/soak evidence; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Hardened the historical D-0247/D-0248 readiness scripts, D-0252 candidate report, D-0253 fallback-retirement report, D-0253 evidence collector, and the Electron adapter gate so thin `passed:true` JSON cannot authorize G6 readiness, D-0252 candidate, or D-0253 fallback retirement. D-0248 now uses the D-0251 packaged QA harness. D-0253 requires command evidence artifacts with matching `evidenceSha256` and operator digest bindings for D-0252, soak, replacement, and rollback JSON. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. |
| 2026-06-24 D-0251 packaged QA harness hardening | Reasonix / Kun / Go | `npm run qa:d0251:packaged-go-runtime -- --json` now collects D-0251 packaged launch/runtime/SSE/rollback evidence | D-0251 live remains blocked without real packaged app/runtime/operator inputs; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added a D-0251 packaged desktop QA harness and switched the live evidence collector to use it. The harness keeps the compatibility evidence id `d0243-packaged-go-runtime-qa`, but requires a real packaged launch command, runtime health, authenticated thread list, turn create, SSE replay, `go-runtime-candidate` env, and structured rollback proof JSON before passing. It records sanitized runtime/provider URLs and hashes instead of launch commands, tokens, or credential-bearing custom endpoints. |
| 2026-06-24 D-0253 retirement evidence collector | Reasonix / Kun / Go | `npm run runtime:go:d0253-retirement-evidence` writes structured soak/replacement/rollback/operator evidence files | D-0253 remains blocked; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added a D-0253 collector that creates `candidate-soak.json`, `replacement-tests.json`, `rollback-strategy.json`, and `operator-retirement-approval.json` in the schema consumed by the retirement report. It writes blocked evidence when live inputs are absent, records `filesDeleted:false`, and prevents D-0253 from relying on missing files or hand-written placeholders. |
| 2026-06-24 D-0250C to D-0252 live-ready bridge | Reasonix / Kun / Go | Historical D-0250C reporting computed report-level live readiness from D-0250B intake and strict G6 evidence; current engine absorption evidence enters through `npm run runtime:go:engine-absorption-report -- --json` | D-0250C-live is post-cutover-live-validation-pending because external live evidence is absent; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Fixed the D-0250C-to-D-0252 chain so future passed D-0251/D-0250B/strict-G6 evidence can produce a report-level `live-cutover-ready` / `goDefaultCutoverCandidate:true` without changing the underlying D-0250C matrix safety proof, which remains code-stage-only with matrix-level `goDefaultCutoverCandidate:false`. Added a focused test proving the bridge. |
| 2026-06-24 D-0252/D-0253 evidence hardening | Reasonix / Kun / Go | `npm run runtime:go:d0252-default-readiness-report` and `npm run runtime:go:d0253-retirement-report` now require deeper machine-field evidence | D-0252 and D-0253 remain blocked; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Tightened D-0252 so minimal readiness/D-0250C/MCP JSON cannot authorize candidate: strict G6 must have all hard gates passed and no blockers, D-0250C must be live-cutover ready while preserving Analytix/Kun product boundaries, MCP must include credentialed probe details, and operator digests use canonical JSON plus target-commit coverage. Tightened D-0253 so minimal `passed:true` soak/rollback/operator placeholders cannot authorize retirement. Candidate soak must prove Go candidate coverage while preserving hidden Go switcher and no Reasonix public protocol; rollback must prove pre-retirement TypeScript recovery plus post-retirement release/data/settings/event-store compatibility; operator approval must be JSON plus explicit env gate. |
| 2026-06-24 D-0252/D-0253 candidate and fallback-retirement gate reports | Reasonix / Kun / Go | `npm run runtime:go:d0252-default-readiness-report` and `npm run runtime:go:d0253-retirement-report` now write auditable blocked endgame reports | D-0252 remains blocked because D-0251 live evidence and strict G6 are not passed; D-0253 remains blocked because no candidate soak, replacement-test, rollback-strategy, or retirement approval evidence exists; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added D-0252 and D-0253 report harnesses, npm scripts, focused tests, and generated blocked JSON/markdown under `docs/analytix/upstreams/d0252-candidate/` and `docs/analytix/upstreams/d0253-retirement/`. D-0252 emits `go-runtime-default` adapter env only after D-0251 live evidence, strict G6 `defaultBackendReady:true`, D-0250C live-ready evidence, and operator candidate approval pass. D-0253 only authorizes retirement evidence closure after D-0252 candidate, soak, replacement tests, rollback strategy, and explicit approval pass; it records `filesDeleted:false` and does not delete bridge/settings/product code. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. |
| 2026-06-24 D-0251 live evidence collector harness | Reasonix / Kun / Go | `npm run runtime:go:d0251-live-evidence` now writes an auditable live cutover evidence bundle under `docs/analytix/upstreams/d0251-live/` | D-0251 harness/code-stage closed; live evidence is `live_blocked` on this machine because credentialed provider, MCP execution, packaged QA, and operator approval inputs are missing; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added `scripts/d0251-live-evidence-collector.mjs`, npm `runtime:go:d0251-live-evidence`, focused collector tests, and generated blocked JSON for provider matrix, MCP execution, packaged desktop QA, operator gate, aggregate report, and summary. The collector records endpoint family/request shape/stream parsing/usage/cache/error/redaction fields, keeps custom endpoint full paths un-appended, scopes DeepSeek cache telemetry to DeepSeek only, preserves the compatibility operator gate id `d0250b-go-runtime-operator-gate`, and refuses to write fake passed evidence when external conditions are absent. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. |
| 2026-06-24 D-0250C Reasonix superiority code-stage matrix | Reasonix / Kun / Go | Go runtime-info exposes `reasonixSuperiorityMatrixD0250C`; current code-stage report/matrix JSON is generated by `npm run runtime:go:engine-absorption-report -- --json` | D-0250C-code-stage closed; D-0250C-live post-cutover-live-validation-pending by real credentialed provider/MCP/packaged/operator JSON evidence; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added `packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go`, root shim/test, Go report command, TS capability schema, runtime conformance assertions, and `docs/analytix/upstreams/d0250c-go-runtime-code-stage-report.json`. The 13-row matrix uses only source-level deterministic evidence: `absorb=1`, `adapt=5`, `keep-kun=5`, `reject=1`, `defer=1`. Reasonix is absorbed/adapted only for engine/runtime strengths such as DeepSeek cache release guard, stream/usage/reasoning guards, MCP lazy/schema/reconnect/redaction, job lineage, approval/user-input/history repair, and goal/AutoResearch discipline. Analytix/Kun keeps the product layer, prefix-shape contract, provider endpoint family/custom endpoint contract, MCP search/meta-tool boundary, and durable runtime event/SSE/thread routes. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. |
| 2026-06-24 D-0250B cutover evidence intake + operator JSON gate | Reasonix / Kun / Go | D-0250B now has an auditable cutover report and strict evidence intake for credentialed provider, credentialed MCP, packaged QA, and operator gate JSON | D-0250B-intake code-stage closed; D-0250C-live blocked on real external evidence; Go runtime default active; post-cutover live validation pending; TypeScript backend retired in current adapter | Codex | Added `scripts/d0250b-go-runtime-cutover-report.mjs`, `npm run runtime:go:d0250b-report`, `operatorGate` readiness contract, stricter provider/MCP/packaged/operator JSON validation, no-secret evidence scans, MCP coverage requirements for connect/search/call/approval/reconnect/redaction, packaged QA coverage requirements, and operator evidence path support via `ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE` / `ANALYTIX_D0250B_OPERATOR_GATE_EVIDENCE`. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired backend, and Reasonix remains engine/runtime-only. |
| 2026-06-23 D-0249 Reasonix integration topology + strict evidence validation | Reasonix / Kun / Go | Go runtime-info now exposes `reasonixIntegrationTopologyD0249`, and both Go/runtime-main readiness checks require real evidence JSON for credentialed provider, credentialed MCP, and packaged QA hard gates | code/test topology closed; Go runtime default active; credentialed provider/MCP/packaged/operator evidence is post-cutover live validation | Codex | Added `packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go`, topology tests, strict TS capability schema, Go runtime conformance assertions, main-adapter evidence-file validation, product-sovereignty scan coverage, and `docs/analytix/upstreams/d0249-reasonix-integration-topology.md`. The topology binds Reasonix DeepSeek cache/prefix telemetry, job/sub-agent lineage, MCP lifecycle/search/call/reconnect/redaction, AutoResearch project state, and Workflow/Create Loop planner discipline to existing Analytix Chat, `/goal`, tool calling, MCP registry, approval/user-input, runtime event/SSE, provider adapter, and job-lineage paths. No Reasonix public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, renderer/preload/settings/product identity change, or Reasonix public runtime surface was added. |
| 2026-06-23 D-0248 strict G6 evidence gate + switch preflight | Reasonix / Kun / Go | Historical D-0247 readiness report supported D-0248 `--gate` / `--strict`; current local switch preflight chain is `npm run runtime:go:preflight -- --json --gate` | code/test gate closed; Go default backend is enabled by deterministic local gates; credentialed provider matrix, credentialed MCP execution, packaged desktop QA, explicit env gate, and full command evidence remain post-cutover live validation | Codex | Added the historical D-0248 switch preflight script and npm entry, strict missing/skipped/failed hard-gate exit semantics, credentialed provider/MCP/packaged evidence-path validation, expected-blocked code-stage reporting, adapter tests for skipped hard gates, fake pass rejection, missing packaged QA fallback to TypeScript, and `docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md`. Dry-run and expected-blocked reports are audit-only and cannot authorize switching. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, no renderer-visible Go switcher was added, and no Reasonix public Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entry was added. |
| 2026-06-23 D-0247 G6 readiness semantics + default backend cutover gate | Reasonix / Kun / Go | Runtime-info now separates D-0244/D-0245 matrix-green semantics from D-0243 default-backend readiness, and main adapter now selects `go-runtime-default` by default while keeping `go-runtime-candidate` as a legacy explicit gate | scoped code/test/docs closed; D-0244/D-0245 matrix green does not imply default backend readiness; credentialed provider/MCP, packaged QA, explicit env gate, product sovereignty, typecheck/build, and rollback evidence remain post-cutover validation | Codex | Added readiness semantics now homed at `packages/runtime-go/internal/readiness/readiness_semantics.go`, `capabilities.upstreamAbsorption.readinessSemanticsD0247`, `defaultBackendReadinessD0243`, matrix fields `capabilityMatrixGreen`, `absorptionMatrixGreen`, and `defaultBackendReady`, plus `scripts/d0247-go-runtime-readiness-report.mjs`. `readyForG6` remains a compatibility alias for matrix green only. Current `src/main/runtime/analytix-adapter.ts` selects Go by default after deterministic local gates and rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as retired; fake/local matrix passes and dry-run skipped checks do not count as credentialed passes. D-0246 cleared the AutoResearch project-state blocker, but it did not complete G6/default backend readiness. No renderer/preload/settings/UI product entry changed, Go default backend is enabled by deterministic local gates, and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry was added. |
| 2026-06-23 D-0246 AutoResearch Go runtime project-local state | Reasonix / Kun / Go | Go runtime server now writes `.analytix/autoresearch/<threadId>/` state for `/goal --research` turns and replays `autoresearch_state_audit` under the analytix SSE contract | scoped code/test/docs closed; D-0244/D-0245 red/deferred AutoResearch rows green; credentialed provider/MCP, packaged QA, default backend, and TS retirement still gated by D-0243/G6 | Codex | Added `packages/runtime-go/internal/research/autoresearch_state.go`, `d0246_autoresearch_state_test.go`, runtime-server `/v1/threads/:id/turns` integration, `autoresearch_state_audit` TS event schema, reducer audit-only proof, Go/TS conformance assertions, and product-sovereignty scan tokens. The Go store creates `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`; rejects unknown requirement evidence without findings writes; sanitizes thread path segments; writes no `REASONIX.md` or `AGENTS.md`; keeps state out of stable system prefix/tool schema; and exposes no `/v1/autoresearch` route or top-level AutoResearch UI. |
| 2026-06-23 D-0245 Multi-model non-regression + Kun baseline guard | Kun / Reasonix / Go | Go runtime server turn path now routes DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom_endpoint through analytix provider contracts while exposing a D-0245 Kun/Analytix full-function baseline guard | scoped code/test/docs closed; AutoResearch project state now green; credentialed provider/MCP, packaged QA, default backend, and TS retirement still gated | Codex | Added `packages/runtime-go/internal/upstreamaudit/baseline_absorption.go`, `d0245_baseline_absorption_test.go`, provider-family turn config in `runtime_server.go`, runtime-info `capabilities.upstreamAbsorption.kunAnalytixBaselineGuard` and `reasonixAbsorptionMatrixD0245`, shared capability schema, runtime conformance assertions, `/goal` audit-only reducer proof, and product-sovereignty scan freshness. D-0246 updates `autoresearch-project-state` from deferred to green in this matrix; D-0247 clarifies `ReadyForG6` as a compatibility alias for matrix green only, while default Go runtime remains controlled by D-0243 readiness gates. Kun source of truth is `KunAgent/Kun` tags `v0.2.13` (`2ba8decc2f56862e7f677fcf89bbc3d402ec3a23`) and `v0.2.14` (`8f2040349fba47fcd8e8b94f50b131943af839b2`); Reasonix source of truth is `esengine/DeepSeek-Reasonix` with local checkout pinned at `7032f39336f4ae5f216e1fcb3368e5f679723490` for code comparison. The matrix explicitly states Reasonix DeepSeek cache/prefix absorption is provider-specific enhancement, not provider narrowing; non-DeepSeek providers keep their request URL/body/header/usage/cache diagnostics separate. No renderer/preload/settings/UI product entry changed and no Reasonix public protocol or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry was added. |
| 2026-06-23 D-0244 Reasonix capability red matrix | Reasonix / Kun / Go | Go runtime server now exposes an analytix-owned Reasonix capability red matrix, Goal evidence audit event, MCP lifecycle audit event, and AutoResearch state audit event under runtime contracts | scoped code/test/docs closed for D-0244 red/deferred capability rows; default backend, packaged QA, and TS retirement still gated | Codex | Added `packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go`, `d0244_reasonix_red_matrix_test.go`, `d0244_goal_evidence.go`, `d0244_goal_evidence_test.go`, `d0244_mcp_lifecycle.go`, `d0244_mcp_lifecycle_test.go`, runtime-info `capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix`, shared capability/event schema support, and runtime conformance assertions. Matrix pins `/Users/sun/Projects/DeepSeek-Reasonix` at `7032f39336f4ae5f216e1fcb3368e5f679723490`; marks provider/cache, DeepSeek prefix, approval/user-input, MCP lifecycle/search/call/reconnect/redaction, job lineage, durable replay, crash/restart, candidate rollback, Goal evidence FSM, and D-0246 AutoResearch project-local state green; rejects Reasonix SessionAPI/config/identity/public job/MCP-indexer surfaces. No renderer/preload/settings/UI product entry changed and no Reasonix public protocol was added. |
| 2026-06-23 D-0243 G6 readiness hardening slice | Reasonix / Kun / Go | D-0243 readiness scaffold adds provider/MCP matrices, candidate durable root restart drill, packaged QA script, and main-adapter G6 status | scoped code/test/docs closed; actual credentialed provider/MCP runs, packaged app walkthrough, default Go backend, and TS retirement still gated | Codex | Added `packages/runtime-go/g6_readiness.go`, `g6_readiness_test.go`, candidate durable root support in `cmd/runtime-server`, restart recovery tests in Go and runtime conformance, adapter G6 readiness status/canary checks, and `scripts/d0243-packaged-go-runtime-qa.mjs`. Provider scaffold covers DeepSeek/OpenAI-compatible/Anthropic-compatible/custom endpoint and skips env-gated credentialed probes when missing. MCP scaffold runs fake connect/search/call/reconnect/approval/redaction and skips optional credentialed configuration. `go-runtime-candidate` canary now requires durable restart, provider matrix, MCP matrix, and packaged QA statuses before accepting Go; otherwise it rolls back to TypeScript. No renderer/preload/settings/UI contract changed, no renderer-visible Go switcher or Reasonix public protocol was added, and D-0241/D-0242 temporary paths are tracked as post-G6 delete candidates. |
| 2026-06-23 D-0242 Go runtime server contract slice | Reasonix / Kun / Go | `packages/runtime-go/cmd/runtime-server` now serves the default Go runtime contract with a legacy `go-runtime-candidate` canary gate | scoped code/test/docs closed; credentialed provider/MCP matrix, packaged desktop QA, crash recovery drills, and full loop parity remain post-cutover validation | Codex | Added `runtime_server.go`, `runtime_server_test.go`, `cmd/runtime-server`, expanded `go-runtime-conformance.test.ts`, adapter canary tests, and product-sovereignty scans. The server exposes `/health`, `/v1/runtime/info`, thread list/read/patch/fork, session resume, SSE replay, turn create, approval, and user-input routes behind the default `go-runtime-default` adapter path, with the legacy `go-runtime-candidate` gate retained for explicit compatibility. It connects D-0241 fake live provider, durable event/session sink, approval/user-input manager, fake MCP manager, and parent-bound job lineage into replayable runtime events with usage/cache telemetry and answer-free input replay. Go runtime is default, renderer/preload/settings/UI stay unchanged, no Reasonix SessionAPI/config/public route enters, no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added, and D-0241 proof slices covered by the contract are marked post-D-0242/G6 delete candidates. |
| 2026-06-23 D-0241 Go production-candidate runtime parity slice | Reasonix / Kun / Go | `packages/runtime-go` now has `/v1/internal/go-production-candidate/*` plus main-adapter `go-production-candidate` canary gate | scoped code/test/docs closed; default Go backend, credentialed provider/MCP matrix, packaged desktop QA, crash recovery drills, and TypeScript retirement migration deferred | Codex | Added `live_production_candidate.go`, `live_production_candidate_test.go`, `go-production-candidate-conformance.test.ts`, and adapter gate tests. The slice uses local fake live HTTP/SSE provider servers for DeepSeek/OpenAI-compatible/Anthropic-compatible request, stream, usage/cache, and prefix-shape proofs; writes real durable events before replay; implements approval/user-input pending/deny/submit/cancel/timeout/replay with denied no-execute and answer-free replay events; runs fake MCP lifecycle/search/call without credentials; proves parent goal/thread job-subagent lineage; and lists D-0241/G6 delete targets plus forbidden-now redundancy. It keeps TypeScript default, no renderer/preload/settings/UI change, no Reasonix SessionAPI/config/public route, no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, and rollback stops Go, cleans temp state, then returns to TypeScript. |
| 2026-06-23 D-0240 G6 retirement cleanup proof | Reasonix / Kun / Go | `packages/runtime-go` live-local kernel sidecar now has `/v1/conformance/kernel/g6-retirement-cleanup` returning retained default-runtime paths, post-G6 delete targets, forbidden redundancy checks, and open G6 blockers | scoped code/test/docs closed; G6/default backend, live Go managers, packaged QA, credentialed provider/MCP matrix, crash/rollback drills, and TypeScript retirement migration deferred | Codex | Extended `go-kernel-live-scaffold-oracle.json`, `live_local_kernel.go`, `live_local_kernel_test.go`, `go-kernel-live-scaffold-conformance.test.ts`, and product-sovereignty scans. The proof reports `g6Ready:false`, `noImmediateProductionDelete:true`, `tsRuntimeRetained:true`, `presentForbiddenRedundancyCount:0`, keeps TypeScript runtime/default process/adapter paths retained, lists conformance env gates, fixture routes/oracles, and shadow-only G5 replay for deletion after G6, and records no deprecated bridge alias, duplicate settings schema, renderer-visible Go route, UI runtime switcher, or Reasonix public protocol. |
| 2026-06-23 D-0239 Go kernel live scaffold absorption proof | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now has `/v1/conformance/kernel/*` routes summarizing durable event sink, minimal loop controller, provider-aware cache accounting, approval/user-input gates, MCP catalog recovery, and job/sub-agent orchestration fixtures | scoped code/test/docs closed; live Go Job Manager, live subagent execution, live MCP/provider parity, packaged QA, G6/default backend, and TS retirement deferred | Codex | Added `go-kernel-live-scaffold-oracle.json`, `live_local_kernel.go`, `live_local_kernel_test.go`, and `go-kernel-live-scaffold-conformance.test.ts`. The scaffold classifies Reasonix session-scoped Job Manager, subagent continue/fork lineage, loop/event sink, MCP lazy catalog recovery, and provider-aware cache accounting into analytix-owned `contract-reimplement` / `code-port-and-adapt` evidence. It rejects Reasonix SessionAPI/config/public routes, keeps job/subagent execution fixture-only and parent-goal-bound, exposes no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no renderer-visible Go route, no default Go backend, and records zero provider/tool/approval/MCP/credential/file/workspace side effects. |
| 2026-06-23 D-0238 Backend-neutral adapter gate + Go conformance canary | Reasonix / Kun / Go | Electron main runtime adapter can now select a Go conformance backend only through a dual internal gate while Go runtime is default | scoped code/test/docs closed; production Go backend, live provider/MCP/job managers, packaged route QA, G6/default backend, and TS retirement deferred | Codex | Added `resolveAnalytixRuntimeBackendGate`, Go conformance sidecar launch, runtime-token override, temp durable root startup, health/thread/loop-boundary/hidden-route canary, fallback-to-TS rollback, backend status reporting, and adapter tests. The proof keeps `window.analytix`, top-level `runtime` settings, `analytix serve`, no renderer-visible Go route, no Reasonix public protocol/SessionAPI/config root, no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no default Go backend, no live superiority claim, and no G6 readiness. |
| 2026-06-23 D-0237 Go minimal agent loop skeleton | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now has a conformance-only minimal loop route that combines G2 route state, G3 provider/cache fixture, G4 approval/user-input/MCP manager fixture, and the D-0236 temp durable event/session store | scoped code/test/docs closed; production Go backend, Electron connection, live provider/MCP/gate managers, default backend, packaged restart QA, G6/release readiness deferred | Codex | Added `go-minimal-agent-loop-oracle.json`, `live_local_loop.go`, `live_local_loop_test.go`, sidecar fixture loading for G3/G4/approval/MCP/loop, and `go-minimal-agent-loop-conformance.test.ts`. The proof runs only below `/v1/conformance/loop/*`, uses a fixture-scripted model turn, records every event through the temp durable store before replay/SSE, and verifies assistant reasoning/text, tool call readiness, denied approval, non-executing tool result, submitted/cancelled user-input gates, DeepSeek/OpenAI/Anthropic cache telemetry, MCP catalog visibility without connection, step-limit/cancel/resume boundaries, and recovered state equality. It also clarifies durable-vs-generic product boundaries while preserving `window.analytix`, top-level `runtime` settings, `analytix serve`, no Reasonix public protocol/SessionAPI/config root, no renderer-visible Go route, no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no live superiority claim, no G6 readiness, and no release readiness. |
| 2026-06-23 D-0236 Go temp durable event/session store + SSE replay | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now has an explicit opt-in temp durable event/session store plus a TS-callable Go conformance sidecar | scoped code/test/docs closed; production runtime replacement, Electron connection, live Go loop, default backend, packaged restart QA, live provider/MCP clients deferred | Codex | Added `live_local_durable_store.go`, `live_local_durable.go`, `cmd/conformance-sidecar`, `go-durable-sidecar-oracle.json`, `live_local_durable_test.go`, and `go-durable-sidecar-conformance.test.ts`. The Go store writes only under caller-provided temp dirs, appends newline-terminated `events.jsonl`, assigns stable per-thread `seq`, returns persisted max `highestSeq`, filters/sorts `loadEventsSince`, skips malformed JSONL with diagnostics, proves persist-before-publish order, proves concurrent seq uniqueness, supports caught-up SSE and `Last-Event-ID`/`since_seq` equivalence, and persists temp thread/session list/search/archive/fork/resume state. Durable replay preserves DeepSeek/OpenAI/Anthropic cache accounting, approval/user-input gate events, answer-free user-input resolved events, and MCP tool-catalog recovery while proving no real workspace mutation, no credential read, no provider/MCP live call, no Electron main wiring, no `analytix serve` replacement, no renderer-visible Go route, no Reasonix public protocol/SessionAPI/config root, no top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no G6 readiness, and no release readiness. |
| 2026-06-23 D-0235 Go G4 approval/user-input/MCP manager live-local sidecar | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now has a fixture-backed G4 manager harness that replays TS-owned approval denial, user-input cancel/submit/structured validation, remote-entry control-port boundary, MCP lifecycle/search/approval annotation/reconnect/diagnostics, and secret redaction cases; Go remains test/conformance-only | scoped code/test/docs closed; live approval execution, live Go MCP client, credentialed MCP matrix, Electron connection, default backend, durable Go store, file mutation deferred | Codex | Added `live_local_g4.go`, G4 sidecar wiring, explicit G4 oracle decoding, G4 stub replay counters on `LiveLocalSidecarSnapshot`, and focused tests for approval/user-input and MCP replay. The Go tests start a real local `httptest.Server`, verify exact TS-owned approval/user-input status/body shapes, second decision/resolve statuses, invalid structured choice no-gate behavior, answer-free resolved events, remote-entry allowed/forbidden ports, MCP connect/reload/disconnect/cancel/error, search meta-tools, untrusted workspace hiding, call-time reconnect, background reconnect, known override diagnostics, and secret redaction while proving `ApprovalExecutionAttempts:0`, `ToolExecutionAttempts:0`, `MCPConnectionAttempts:0`, `MCPCredentialAttempts:0`, `CredentialReadAttempts:0`, `FileMutationAttempts:0`, `EventsJSONLWriteAttempts:0`, `RealWorkspaceWriteAttempts:0`, no Reasonix public protocol, no Electron main wiring, no `analytix serve` replacement, no default Go backend, no renderer-visible Go route, no live MCP/approval parity claim, no Rust/Tauri path, no G6 readiness, and no release readiness. |
| 2026-06-23 D-0234 Go G3 provider/cache streaming live-local sidecar | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now has a fixture-backed G3 provider/cache streaming harness that replays TS-owned provider usage, request-shape, SSE, cache accounting, cache drift, and diagnostics cases; Go remains test/conformance-only | scoped code/test/docs closed; live provider client, credentialed provider matrix, Electron connection, default backend, durable Go store, live MCP/gate/file mutation deferred | Codex | Added `live_local_provider.go`, `ProviderOracle` sidecar wiring, G3 stub replay counters on `LiveLocalSidecarSnapshot`, and focused tests for usage/cache, request shape, and streaming. The Go test starts a real local `httptest.Server`, verifies 5 usage cases, 7 request-shape cases, `item_delta -> usage -> turn_completed` frames, DeepSeek/OpenAI Responses/Anthropic cache accounting, cache drift, and diagnostics privacy while proving `ProviderCallAttempts:0`, no external network, no API-key read, no provider credentials, no Reasonix public protocol, no Electron main wiring, no `analytix serve` replacement, no default Go backend, no renderer-visible Go route, no live provider/cache superiority, no Rust/Tauri path, no G6 readiness, and no release readiness. |
| 2026-06-23 D-0233 Go isolated mutating G2 lifecycle sidecar | Reasonix / Kun / Go | `packages/runtime-go` live-local sidecar now accepts the four TS-owned G2 mutating lifecycle routes into an isolated in-memory harness store; Go remains test/conformance-only | scoped code/test/docs in progress; Electron connection, default backend, durable Go store, live provider/MCP/gate/file mutation deferred | Codex | Added `live_local_store.go`, `NewLiveLocalSidecarHarness`, `LiveLocalSidecarSnapshot`, and `TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptOracle`. The Go test starts a real local `httptest.Server`, verifies all 11 G2 oracle route cases including `PATCH /v1/threads/thr_g2_beta`, `PATCH /v1/threads/thr_g2_read`, `POST /v1/threads/thr_g2_parent/fork`, and `POST /v1/sessions/thr_g2_source/resume-thread`, records isolated archive/update/fork/resume state, proves a temp `events.jsonl` sentinel is unchanged, and keeps rollback guards `electronMainConnected:false`, `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`. No Electron main wiring, `analytix serve` replacement, Reasonix public protocol, provider live call, approval execution, MCP credential, real workspace/event-log mutation, default Go backend, renderer-visible Go route, Rust/Tauri path, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0232 Go live-local sidecar prototype | Reasonix / Kun / Go | `packages/runtime-go` now has a test/conformance-only live-local sidecar handler that serves G1 health/info/tools plus G2 GET-only route/SSE replay from TypeScript-owned fixtures; Go is no longer only handlerless shadow for this read-only slice | scoped code/test/docs in progress; Electron connection, default backend, live provider/MCP/gate/file mutation deferred | Codex | Added `NewLiveLocalSidecarHandler` and `TestLiveLocalSidecarPrototypeMatchesTypeScriptOracle`. The Go test starts a real local `httptest.Server`, verifies `/health`, `/v1/runtime/info`, `/v1/runtime/tools`, seven read-only G2 route cases, exact SSE frames, and G5 rollback guards: `electronMainConnected:false`, `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`. The handler filters non-GET G2 routes and rejects `PATCH`, `POST`, approval execution, and `/v1/runtime/go`. No Electron main wiring, `analytix serve` replacement, Reasonix public protocol, provider live call, MCP credential, file mutation, default Go backend, renderer-visible Go route, Rust/Tauri path, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0231 MCP search refresh drift evidence closure | Reasonix / Kun / Go | Existing MCP catalog refresh/currentness drift runtime/G5/Go evidence is now scan-visible and stage-closure-visible; Go remains shadow-only | scoped docs/scan closed; live Go MCP client and credentialed MCP matrix deferred | Codex | Promoted already-existing `mcp_refresh_catalog` / `mcpSearchRefreshDrift` evidence into the post-881 proof scan and closure docs. The runtime oracle already proves initial `search_issues`, expanded `search_issues` + `create_issue`, `totalIndexed: 2`, and `catalogDrift: true`; G5/Go shadow already replays it. No runtime behavior change, Reasonix MCP public protocol, MCP-indexer top-level entry, Kun identity, deprecated bridge/settings fallback, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go MCP client, credentialed MCP QA, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0230 Plan/auto-route state reset G5 shadow | Reasonix / Kun / Go | Cancelled Plan turns now have explicit runtime/G5/Go shadow evidence that later normal and `model: "auto"` turns do not inherit Plan state and rerun current auto routing; Go remains shadow-only | scoped code/test/docs closed; public auto-plan/controller and live Go loop rejected/deferred | Codex | Added a real `loop.test.ts` case for Plan cancellation followed by a fixed normal turn and an auto-routed turn. The normal/auto requests have no `modeInstruction`, no `requiredToolName`, and no `create_plan` advertisement; the auto turn invokes `_auto_router` once with an isolated router request and receives the current `deepseek-v4-pro` / `max` recommendation. `controlExecutableCases.planCancelStateReset` and Go G5 shadow replay the same booleans. No public auto-plan setting, project override, Reasonix controller protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go loop, packaged Plan QA, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0229 Plan step/cancel/cache G5 shadow | Reasonix / Kun / Go | Explicit Plan mode step gating, aborted follow-up, and DeepSeek cache telemetry now have standalone G5/Go shadow evidence; Go remains shadow-only | scoped code/test/docs closed; live Go loop and packaged Plan QA deferred | Codex | Promoted the existing D-0107 plan step/cancel/cache runtime test into `controlExecutableCases.planStepCancelCache` and Go G5 shadow. The fixture records step-0 `create_plan` + `ls`, exclusion of `bash`, follow-up forced to `create_plan`, aborted follow-up status, retry behavior, two usage events, unchanged stable prefix, and preserved DeepSeek hit/miss telemetry. No Reasonix controller protocol, public auto-plan setting, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go loop, packaged Plan QA, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0228 MCP call-time reconnect G5 shadow | Reasonix / Kun / Go | MCP tool-call reconnect classification now has explicit oracle/G5/Go shadow evidence for transport-looking stale connections retrying once and deterministic protocol errors not reconnecting; Go remains shadow-only | scoped code/test/docs closed; live Go MCP client and credentialed MCP matrix deferred | Codex | Promoted existing `callMcpToolWithReconnect` behavior into `mcp-tool-lifecycle-oracle`, `controlExecutableCases.mcpCallReconnect`, and Go G5 shadow. The fixture records stale connection retry, close count, successful retry instance, protocol validation error no-retry, and tool-result error code. TS oracle tests run the real MCP provider path; Go computes the same classification without implementing an MCP client. No Reasonix MCP public protocol, MCP-indexer top-level entry, Kun identity, deprecated bridge/settings fallback, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go MCP client, credentialed MCP QA, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0227 AutoResearch direction tracking G5 shadow | Reasonix / Kun / Go | AutoResearch direction recording now has explicit G5/Go shadow evidence for `record_research_direction`, `directions_tried.json`, `iteration_log.jsonl`, and active research-goal gating; Go remains shadow-only | scoped code/test/docs closed; live Go research bridge and top-level AutoResearch entry rejected/deferred | Codex | Promoted the existing D-0014 AutoResearch direction tracking behavior into `controlExecutableCases.autoResearch`. The TS conformance test creates project-local state, records a direction, reads `directions_tried.json` and `iteration_log.jsonl`, verifies the `record_research_direction` tool and active research-goal guard, and preserves unknown-requirement rejection. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix project protocol, Kun identity, hidden top-level AutoResearch route, deprecated bridge/settings fallback, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go research bridge, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0226 Renderer settings read facade G5 shadow | Reasonix / Kun / Go | Renderer settings-read facade replay now proves keyboard shortcut, speech-to-text, and usage model-label reads stay behind `rendererRuntimeClient.getSettings`; direct settings bridge bypass scans remain active; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged settings walkthrough deferred | Codex | Promoted the D-0206 settings-read facade seal into `controlExecutableCases.desktopSovereignty.rendererSettingsReadFacadeMatrix`. The source-derived case records the settings client facade, forbidden direct bridge token, settings readers, source-only settings client proof, settings-changed event preservation, direct bridge rejection, and scan guard. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix settings/config protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged settings walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0225 Renderer usage runtime client facade G5 shadow | Reasonix / Kun / Go | Renderer usage/debug facade replay now proves thread/day/model usage loaders plus token economy and LLM debug diagnostics stay behind `rendererRuntimeClient.runtimeRequest`; direct runtime bridge bypass scans remain active; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged usage walkthrough deferred | Codex | Promoted the D-0205 usage/debug facade seal into `controlExecutableCases.desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix`. The source-derived case records the runtime client facade, forbidden direct bridge token, usage loaders, diagnostics loaders, source-only runtime client proof, direct bridge rejection, usage unit proof, and scan guard. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix SessionAPI/usage protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged usage walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0224 Side conversation relation G5 shadow | Reasonix / Kun / Go | Side conversation promotion replay now proves `AgentProvider.updateThreadRelation` is optional analytix-owned contract, promotion calls the provider, refreshes and closes side state, and direct runtime bridge bypass scans remain active; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged side walkthrough deferred | Codex | Promoted the D-0204 side conversation relation provider contract into `controlExecutableCases.desktopSovereignty.sideConversationRelationContractMatrix`. The source-derived case records the provider method, store action, `primary` relation, optional provider contract proof, provider implementation through runtime client, store provider-call proof, refresh/close proof, direct bridge rejection, unit proof, and scan guard. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix SessionAPI/side protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged side walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0223 Renderer provider facade seal G5 shadow | Reasonix / Kun / Go | Renderer provider facade seal replay now proves archive/restore and relation PATCH stay behind `rendererRuntimeClient.runtimeRequest` while direct runtime bridge bypass scans remain active; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged provider walkthrough deferred | Codex | Promoted the D-0203 provider facade seal proof into `controlExecutableCases.desktopSovereignty.rendererProviderFacadeSealMatrix`. The source-derived case records the runtime client facade, forbidden direct bridge token, sealed provider methods, source-only runtime client use, archive/restore proof, relation PATCH proof, lifecycle unit proof, direct-bypass scan guard, and provider facade token scan. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix SessionAPI/public bridge protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged provider walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0222 Renderer provider alias guard G5 shadow | Reasonix / Kun / Go | Renderer provider alias guard replay now proves provider route ownership, lifecycle/gate coverage, fork/resume encoding, and forbidden route rejection under throwing legacy bridge aliases; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged provider walkthrough deferred | Codex | Promoted the D-0202 renderer provider alias guard proof into `controlExecutableCases.desktopSovereignty.rendererProviderAliasGuardMatrix`. The source-derived case records the `installDsGui` helper, forbidden `kun` / `reasonix` aliases, guarded provider tests, route/lifecycle/approval-user-input/fork-resume/dynamic-encoding coverage, source-only analytix proof, and forbidden-route guard proof. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix SessionAPI/public bridge protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged provider walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0221 Renderer settings bridge G5 shadow | Reasonix / Kun / Go | Renderer settings bridge replay now proves settings read cache, write refresh, top-level runtime patch preservation, and legacy alias unread guard; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged settings walkthrough deferred | Codex | Promoted the D-0201 renderer settings bridge proof into `controlExecutableCases.desktopSovereignty.rendererSettingsBridgeMatrix`. The source-derived case records top-level `runtime` patch values, settings cache expectations, analytix settings API source proof, cache/write refresh proof, unit proof, and legacy alias unread proof. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix settings protocol/config root, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged settings walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0220 Renderer runtime client bridge G5 shadow | Reasonix / Kun / Go | Renderer runtime client replay now proves request argument preservation, restart passthrough, SSE control/listener passthrough, and legacy alias unread guard; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged renderer walkthrough deferred | Codex | Promoted the D-0200 renderer runtime client bridge proof into `controlExecutableCases.desktopSovereignty.rendererRuntimeClientBridgeMatrix`. The source-derived case records encoded runtime request path variants, request argument counts, restart proof, SSE start/stop calls, listener APIs, source passthrough proof, unit proof, and legacy alias unread proof. Go G5 shadow computes the same output without changing TypeScript renderer behavior. No Reasonix SessionAPI/public bridge protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged renderer walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0219 Main SSE host URL encoding G5 shadow | Reasonix / Kun / Go | Main SSE host request replay now proves encoded thread events path, cursor headers, auth/accept, stream id, and forbidden route guard; Go remains shadow-only | scoped code/test/docs closed; live Go SSE server and packaged SSE walkthrough deferred | Codex | Promoted the D-0199 main SSE host URL encoding proof into `controlExecutableCases.desktopMainIpcBoundary.mainSseHostEncodingMatrix`. The source-derived case records dangerous source thread id, encoded thread id, `/v1/threads/{id}/events`, `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, bearer auth, stream id, error payload, source path/header proof, unit proof, and forbidden-route guard. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public SSE protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go SSE server, packaged SSE walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0218 Preload SSE bridge G5 shadow | Reasonix / Kun / Go | Preload SSE start/stop and payload-only listener replay now proves SSE stays on analytix IPC; Go remains shadow-only | scoped code/test/docs closed; live Go SSE server and packaged SSE walkthrough deferred | Codex | Promoted the D-0198 preload SSE bridge proof into `controlExecutableCases.desktopSovereignty.preloadSseBridgeMatrix`. The source-derived case records start/stop/event/end/error channels, thread id, cursor, stream ids, event/end/error payloads, payload-only wrapper proof, cleanup proof, and `analytix` bridge exposure proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public SSE protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go SSE server, packaged SSE walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0217 Runtime host URL handoff G5 shadow | Reasonix / Kun / Go | Main runtime adapter host request replay now proves encoded path/query, method/body, auth/header/content-type, and ensured settings host selection; Go remains shadow-only | scoped code/test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Promoted the D-0197 runtime host URL handoff proof into `controlExecutableCases.desktopMainIpcBoundary.runtimeHostHandoffMatrix`. The source-derived case records `runtimeRequestViaHost`, `getRuntimeBaseUrlForSettings`, encoded thread/turn ids, query preservation, POST body, bearer auth, custom proof header, JSON content type, source proof, and ensureRuntime port-switch proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public route protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0216 Preload runtime request bridge G5 shadow | Reasonix / Kun / Go | Preload runtime request facade replay now proves path/method/body and diagnostics calls stay on analytix `runtime:request`; Go remains shadow-only | scoped code/test/docs closed; live Go bridge and packaged preload walkthrough deferred | Codex | Promoted the D-0196 preload runtime request bridge proof into `controlExecutableCases.desktopSovereignty.preloadRuntimeRequestBridgeMatrix`. The source-derived case records runtime and diagnostics facade calls, encoded thread/turn/user-input ids, path/method/body preservation, same analytix IPC channel use, and `analytix` bridge exposure proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public route protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go bridge, packaged preload walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0215 Main IPC runtime adapter handoff G5 shadow | Reasonix / Kun / Go | Main IPC registered handler handoff replay now proves encoded paths/methods/bodies reach the adapter unchanged; Go remains shadow-only | scoped code/test/docs closed; live Go HTTP server and packaged IPC walkthrough deferred | Codex | Promoted the D-0195 main IPC runtime-adapter handoff proof into `controlExecutableCases.desktopMainIpcBoundary.runtimeAdapterHandoffMatrix`. The source-derived case records seven encoded adapter calls, six raw/singular rejected-before-adapter calls, parse-before-adapter ordering, encoded path proof, method/body proof, and no-execute rejection proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public route protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0214 Main IPC endpoint builder G5 shadow | Reasonix / Kun / Go | Main IPC runtime request schema replay now includes encoded shared-builder allow-list matrix; Go remains shadow-only | scoped code/test/docs closed; live Go HTTP server and packaged IPC walkthrough deferred | Codex | Promoted the D-0194 main IPC endpoint-builder allow-list proof into `controlExecutableCases.desktopMainIpcBoundary.endpointBuilderAllowListMatrix`. The source-derived case records encoded shared-builder requests for thread/fork/turn/steer/interrupt/checkpoint/approval/user-input/session/attachment/memory paths, raw dynamic route rejections, singular user-input compatibility rejection, shared template compilation, and unit-test proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public route protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-23 D-0213 Renderer runtime endpoint builder G5 shadow | Reasonix / Kun / Go | Renderer provider endpoint construction is replayed in desktop sovereignty Go shadow; Go remains shadow-only | scoped code/test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Promoted the D-0193 renderer provider endpoint-builder proof into `controlExecutableCases.desktopSovereignty.rendererProviderEndpointMatrix`. The TS source-derived case records shared root path constants, encoded dynamic thread/turn/approval/user-input/session ids, analytix-owned `/v1/*` runtime paths, runtime-client facade use, and unit-test proof. Go G5 shadow computes the same output without changing TypeScript runtime behavior. No Reasonix SessionAPI/public route protocol, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0193 Renderer runtime endpoint builder proof | Reasonix / Kun / Go | Renderer provider path construction uses shared analytix endpoint constants/builders; Go remains shadow-only | scoped code/test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Updated `AnalytixRuntimeProvider` connect/list/create-thread root calls to use `ANALYTIX_HEALTH_PATH` and `ANALYTIX_THREADS_PATH`, and added renderer provider evidence that dynamic thread, turn, approval, user-input, and session ids containing slash/query/fragment text are URL-encoded before calling `window.analytix.runtime.runtimeRequest`. This ties GUI runtime calls to the shared endpoint contract and proves sensitive routes remain analytix-owned. No runtime behavior change, Reasonix SessionAPI/public route protocol, Kun identity, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0192 Shared endpoint builder sovereignty proof | Reasonix / Kun / Go | Shared analytix endpoint template and path-builder encoding proof; Go remains shadow-only | scoped test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Added `src/shared/analytix-endpoints.test.ts` proving shared route builders URL-encode thread, turn, checkpoint, approval, user-input, session, attachment, and memory ids so slashes/query/hash text cannot inject route segments; shared templates remain analytix-owned; canonical user input stays plural `/v1/user-inputs/{id}`; and forbidden Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer/session-api tokens are absent from exported endpoint strings. No runtime behavior change, Reasonix SessionAPI/public route protocol, Kun identity, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0191 Runtime forbidden route dispatch proof | Reasonix / Kun / Go | Actual not-found dispatch matrix for forbidden upstream/hidden public route tokens; Go remains shadow-only | scoped test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Added a focused runtime conformance test that reads `runtimeHttpRouteSovereignty.forbiddenRouteTokens`, dispatches each token through the real TypeScript HTTP router with valid auth, and proves all return structured `{ code: "not_found", message: "route not found" }`. This turns forbidden Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route absence into actual router behavior. No runtime behavior change, Reasonix SessionAPI/public route protocol, Kun identity, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0190 Runtime HTTP auth matrix proof | Reasonix / Kun / Go | Actual unauthenticated dispatch matrix for the active `analytix serve` route table; Go remains shadow-only | scoped test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Added a focused runtime conformance test that reads `controlExecutableCases.runtimeHttpRouteSovereignty.routes`, dispatches every registered route through the real TypeScript HTTP router without auth, and proves `/health` returns 200 while all 43 `/v1/*` routes return structured `{ code: "unauthorized", message: "unauthorized" }` before route-specific body parsing or side effects. It explicitly covers SSE, task-job, approval, user-input, and resume-thread routes. No runtime behavior change, Reasonix SessionAPI/public route protocol, Kun identity, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0189 Runtime HTTP route sovereignty shadow | Reasonix / Kun / Go | `analytix serve` HTTP/SSE route inventory, auth guard, not-found, and internal task-job route evidence; Go remains shadow-only | scoped test/docs closed; live Go HTTP server and packaged route walkthrough deferred | Codex | Added `controlExecutableCases.runtimeHttpRouteSovereignty`, sourced from runtime route registration, router matching, HTTP server not-found handling, shared endpoint templates, and HTTP server tests. It proves the public route table is exactly 44 entries, `/health` is the only unauthenticated route, authenticated `/v1/*` routes stay analytix-owned, SSE remains `/v1/threads/:id/events`, thread lifecycle plus approval/user-input routes are present, task-job routes remain internal runtime endpoints, singular `/v1/user-input/:id` is compatibility-only, and forbidden Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route tokens are absent. No runtime behavior change, Reasonix SessionAPI/public route protocol, Kun identity, hidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0188 Package/runtime CLI identity shadow | Reasonix / Kun / Go | `analytix serve`, release/package identity, and bundled runtime CLI evidence; Go remains shadow-only | scoped test/docs closed; packaged artifact walkthrough deferred | Codex | Added `controlExecutableCases.packageRuntimeIdentity`, sourced from root/runtime package manifests, electron-builder config, app identity, main AppUserModelID, runtime CLI, binary resolver, afterPack validation, packaging config tests, and release workflow. It proves root package/product name, runtime package/bin, builder app id/product/artifact/NSIS names, app product name, Windows AppUserModelID, bundled `packages/runtime/dist/cli/serve-entry.js`, `analytix serve [options]`, `ANALYTIX_READY`, afterPack runtime validation, and `ANALYTIX_*` release env ownership stay analytix-owned. No runtime behavior change, Reasonix/Kun/DeepSeek public identity, Reasonix CLI/session protocol, default Go backend, renderer-visible Go route, Rust/Tauri path, packaged artifact walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0187 Renderer route-surface sovereignty shadow | Reasonix / Kun / Go | Renderer top-level route/UI sovereignty evidence; Go remains shadow-only | scoped test/docs closed; packaged desktop walkthrough deferred | Codex | Added `controlExecutableCases.rendererRouteSurfaceSovereignty`, sourced from Workbench route-surface tests and renderer shell sources. It proves the top-level `AppRoute` union remains `chat/write/settings/plugins/claw/schedule`, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer tokens and entrypoint symbols are absent from top-level surfaces, dormant Workflow/Create Loop code remains quarantined, browser preview exposes only `window.analytix`, plugin marketplace tabs receive collapsed-sidebar safe-area state, and shell navigation/titlebar regions stay no-drag/safe-inset. No runtime behavior change, Reasonix public UI protocol, Kun/deprecated bridge alias, forbidden top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, packaged desktop walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0186 Desktop main IPC boundary shadow | Reasonix / Kun / Go | Main runtime request/SSE IPC schema evidence; Go remains shadow-only | scoped test/docs closed; packaged desktop walkthrough deferred | Codex | Added `controlExecutableCases.desktopMainIpcBoundary`, sourced from main IPC schemas/handlers and runtime SSE IPC tests. It proves runtime request paths are strict and analytix allow-list based, forbidden Reasonix/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes are rejected before the runtime call, SSE start rejects Reasonix session payloads, main SSE uses `/v1/threads/{id}/events`, stop parses/matches stream ids, reconnect preserves `Last-Event-ID`, and pending events batch at 100ms. No runtime behavior change, Reasonix SessionAPI/frontend protocol, Kun bridge alias, default Go backend, renderer-visible Go route, hidden top-level entry, Rust/Tauri path, packaged bridge walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0185 Desktop bridge IPC sovereignty shadow | Reasonix / Kun / Go | Preload runtime request/SSE IPC evidence; Go remains shadow-only | scoped test/docs closed; packaged desktop walkthrough deferred | Codex | Extended `controlExecutableCases.desktopSovereignty` so it now records `runtimeIpcChannels`, forbidden runtime IPC channel exposure, `runtimeRequestUsesAnalytixIpc`, `runtimeSseUsesAnalytixIpc`, and `publicApiTypesAnalytixOwned`. The source-derived G5 conformance proof reads `src/preload/index.ts`, `src/preload/index.d.ts`, `src/shared/analytix-api.ts`, `src/main/settings-store.test.ts`, and `src/shared/app-settings-runtime.ts`; Go shadow computes the same expected output. No runtime behavior change, Reasonix SessionAPI/frontend protocol, Kun bridge alias, deprecated settings fallback, default Go backend, renderer-visible Go route, hidden top-level entry, Rust/Tauri path, packaged bridge walkthrough, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0184 MCP malformed schema boundary shadow | Reasonix / Kun / Go | MCP schema normalization evidence; Go remains shadow-only | scoped test/docs closed; live Go MCP client deferred | Codex | Added `controlExecutableCases.mcpMalformedSchemaBoundary`, sourced from `mcp-tool-provider.ts` and `mcp-tool-provider.test.ts`, proving malformed MCP `inputSchema` arrays default to a safe object schema, non-record `properties` are dropped, mixed `required` arrays keep only strings, advertised normalized tool names are preserved, and non-record output schemas are omitted. Go shadow computes the same expected output. No runtime behavior change, live Go MCP client, Reasonix MCP-indexer protocol, default Go backend, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, Rust/Tauri path, credentialed MCP QA, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0183 Event JSONL replay boundary shadow | Reasonix / Kun / Go | Event replay/malformed recovery evidence; Go remains shadow-only | scoped test/docs closed; live Go event store deferred | Codex | Added `controlExecutableCases.eventJsonlReplayBoundary`, sourced from `FileSessionStore`, `loop.test.ts`, `runtime-event-recorder.test.ts`, and `file-session-store.test.ts`, proving newline-terminated `events.jsonl` append, replay filter/sort, highestSeq max, malformed JSONL line skipping, persist-before-publish ordering, concurrent seq uniqueness, persisted high-water caching, usage compaction carryover, and compaction-failure append-only recovery. Go shadow computes the same expected output. No runtime behavior change, live Go event store, Reasonix event protocol, default Go backend, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, Rust/Tauri path, packaged long-thread replay QA, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0182 Tool result file/image boundary shadow | Reasonix / Kun / Go | File/image result metadata evidence; Go remains shadow-only | scoped test/docs closed; live Go file/image bridge deferred | Codex | Added `controlExecutableCases.toolResultFileImageBoundary`, sourced from `tool-result-image`, attachment-store, and renderer mapper tests, proving inline image kinds, base64 eviction with metadata preservation, newest-image cap, attachment `localFilePath`, text fallback `FilePath`, DeepSeek v4 text fallback routing, and generated-file/attachment meta lifting. Go shadow computes the same expected output. No runtime behavior change, live Go file/image bridge, Reasonix file protocol, default Go backend, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, Rust/Tauri path, packaged attachment QA, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0181 Goal persistence off-lock control shadow | Reasonix / Kun / Go | Goal persistence lock-split evidence; Go remains shadow-only | scoped test/docs closed; live Go goal manager deferred | Codex | Added `controlExecutableCases.goalPersistenceOffLock`, sourced from real `ThreadService` and `thread-service.test.ts`, proving set/clear goal persist-before-event ordering, empty controller/status/approval lock substring matches, persistence failure warning, and original error surfacing. Go shadow computes the same expected output. No TypeScript runtime behavior change, live Go goal manager, Reasonix controller protocol, default Go backend, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, hidden top-level entry, Rust/Tauri path, packaged QA, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0180 Custom provider telemetry seal | Reasonix / Kun / Go | Provider/cache request-shape-only boundary; Go remains shadow-only | scoped test/docs closed; live provider QA deferred | Codex | Added explicit `customFullEndpointTelemetryCaseIds: []`, `customFullEndpointRequestShapeOnly`, `telemetrySupportedExcludesCustomFullEndpoints`, and `customProviderCacheTelemetryClaimAllowed: false` proof to `controlExecutableCases.providerCacheCoverageFloor`, TypeScript conformance, provider-cache proof, and Go G5 shadow. This proves custom full endpoints remain request-shape-only while DeepSeek/OpenAI Responses/Anthropic cache accounting stays supported. No provider runtime behavior change, custom cache telemetry, live superiority claim, default Go backend, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, Reasonix protocol, hidden top-level entry, Rust/Tauri path, packaged QA, G6 readiness, or release readiness was added. |
| 2026-06-22 D-0178 Post-881 stage closure snapshot | Reasonix / Kun / Go | Machine-scannable capability floor / stronger-than / open-gates artifact; no runtime behavior change | scoped docs/scan closed; live QA deferred | Codex | Added `docs/analytix/qa/post-881-stage-closure-2026-06-22.md` and wired it into `scan:product-sovereignty`. The snapshot states the current fixture-level capability floor, Reasonix absorbed delta classifications, Kun baseline preservation, analytix-over-Reasonix evidence areas, and remaining open gates. The scan now requires tokens for `post881StageClosureCapabilityFloor`, `reasonixAbsorbedDeltas`, `kunBaselinePreserved`, `analytixExceedsReasonixWhere`, and `remainingOpenGates`. No runtime behavior, provider behavior, Go backend status, renderer routes, settings, bridge contracts, Kun identity, Reasonix public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries, Rust/Tauri path, packaged QA, or release readiness was added. |
| 2026-06-22 D-0177 Release evidence final-gate freshness | Reasonix / Kun / Go | Product-sovereignty scan now checks post-881 final gate evidence; no runtime behavior change | scoped scan/docs closed; live QA deferred | Codex | Extended `scan:product-sovereignty` with `releaseEvidenceProofPaths` and a per-token final gate scan for D-0172 through D-0177, requiring the release evidence file to keep runtime package tests, workspace tests, typecheck, runtime build, non-cached Go shadow tests, and product-sovereignty scan commands visible. This strengthens stage-closure proof without changing runtime behavior, provider behavior, Go backend status, renderer routes, settings, bridge contracts, Kun identity, Reasonix public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries, Rust/Tauri path, packaged QA, or release readiness. |
| 2026-06-22 D-0176 Session route proof freshness | Reasonix / Kun / Go | Focused G5 session route oracle and scan guard; no runtime behavior change | scoped test/docs closed; live Go route gates deferred | Codex | Added a focused G5 conformance test proving thread archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized SSE auth, exact body/SSE hashes, route inventory, and product boundary flags remain tied to the analytix-owned G2 route oracle. Extended `scan:product-sovereignty` with session route replay proof tokens and added `go-g2-route-replay-oracle.json` to the post-881 proof freshness paths. No runtime behavior change, live Go router, Reasonix SessionAPI/public protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, packaged thread route walkthrough, or release readiness was added. |
| 2026-06-22 D-0175 Approval/user-input proof freshness | Reasonix / Kun / Go | Focused G5 approval/user-input oracle and scan guard; no runtime behavior change | scoped test/docs closed; live Go gates deferred | Codex | Added a focused G5 conformance test proving approval deny no-execute does not create durable jobs or child runs, user-input submit/cancel preserves HTTP answer echo while omitting answers from resolved events, structured-choice invalid prompts do not open gates, abort cleanup expires/cancels pending gates with late `409`/`404`, resume pending gates clears copied answers, and boundary flags remain false. Extended `scan:product-sovereignty` with approval/user-input proof tokens so gate evidence cannot silently disappear. No runtime behavior change, live Go approval/user-input manager, Reasonix ask/session protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, packaged approval-card QA, or release readiness was added. |
| 2026-06-22 D-0174 Combined step/cancel/cache proof freshness | Reasonix / Kun / Go | Focused G5 combined trace oracle and scan guard; no runtime behavior change | scoped test/docs closed; live Go loop deferred | Codex | Added a focused `go-runtime-conformance.test.ts` check for `controlExecutableCases.combined`, proving same-turn route cache reuse through step limit, next-turn reroute, classifier/step-limit stable-prefix isolation, completed/running/unstarted cancel result pairing, and no Reasonix protocol/default Go backend/renderer-visible Go route exposure. Extended `scan:product-sovereignty` with combined step/cancel/cache proof tokens so D-0146 evidence cannot silently disappear. No runtime behavior change, live Go agent loop, Reasonix controller/session protocol, public auto-plan setting, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, packaged cancel/cache QA, or release readiness was added. |
| 2026-06-22 D-0173 Provider raw accounting proof freshness | Reasonix / Kun / Go | Provider/cache direct oracle and scan guard; no runtime behavior change | scoped test/docs closed; live provider QA deferred | Codex | Added a direct provider-cache oracle test that derives raw usage summaries and aggregate cache accounting from `provider-cache-oracle.json` response bodies, proving parsed case ids, telemetry-supported ids, expected-usage matches, unsupported unknown posture, DeepSeek/OpenAI Responses/Anthropic splits, `2930/720` hit/miss totals, and aggregate hit rate without relying on Go conformance alone. Extended `scan:product-sovereignty` with raw accounting proof tokens so D-0172 evidence cannot silently disappear. No provider runtime behavior change, live Go provider client, default Go backend, renderer-visible Go route, Reasonix public provider protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, credentialed provider matrix, or packaged provider QA was added. |
| 2026-06-22 D-0172 Provider cache accounting raw payload replay | Reasonix / Kun / Go | Provider/cache raw usage replay; Go remains shadow-only | scoped shadow code/docs closed; live provider QA deferred | Codex | Strengthened provider cache accounting so G3/G5 shadow output now derives hit/miss totals, provider-family ids, unsupported unknown posture, and aggregate hit rate from raw fixture `responseBody.usage` payloads instead of trusting copied `expectedUsage` summaries. The fixture proves DeepSeek prompt/native cache precedence, OpenAI Responses cached tokens, Anthropic cache read/creation fields, and unsupported OpenAI-compatible cache telemetry without changing active provider behavior. No live Go provider client, default Go backend, renderer-visible Go route, Reasonix public provider protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, credentialed provider matrix, or packaged provider QA was added. |
| 2026-06-22 D-0171 Post-881 runtime proof freshness v2 | Reasonix / Kun / Go | Reusable proof freshness guard; no runtime behavior change | scoped scan/docs closed; live QA deferred | Codex | Extended `scan:product-sovereignty` so post-881 runtime proof freshness covers task-job and MCP lifecycle fixture/test leaves in addition to provider/cache, approval/user-input, G2/G3/G4/G5, and Go shadow sources. Added positive token scans for provider coverage/request-shape evidence (`providerCacheCoverageFloor`, `derivedUrlMatchCaseIds`, `customFullEndpointAppendedPathCount`), planner/task evidence (`plannerToolsetInventory`, `parallelValidation`, `controlExecutableCases.planner`, `create_plan`), and MCP lifecycle/search/approval evidence (`mcpCoreLifecycle`, `mcpSearchMetaTools`, `mcpLiveLocalIndexer`, `mcpApprovalAnnotations`). No provider runtime behavior change, live Go provider/MCP/job client, default Go backend, renderer-visible Go route, Reasonix public protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, credentialed provider/MCP matrix, or packaged QA was added. |
| 2026-06-22 D-0170 Provider request-shape derived replay | Reasonix / Kun / Go | Provider URL/body/header exact replay; Go remains shadow-only | scoped shadow code/docs closed; live provider QA deferred | Codex | Strengthened the existing provider request-shape oracle so G3/G5 shadow output now derives URL, header shape, body shape, tool shape, custom full-endpoint exact URL ids, and zero custom appended-path count from fixture inputs instead of relying only on copied matrix rows. This proves DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom `/responses`/`/messages`/`/chat/completions` full endpoints keep analytix-owned Base URL / Endpoint format semantics. No provider runtime behavior change, live Go provider client, default Go backend, renderer-visible Go route, Reasonix public provider protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, credentialed provider matrix, or packaged provider QA was added. |
| 2026-06-22 D-0169 G5 provider cache coverage floor control shadow | Reasonix / Kun / Go | Provider/cache coverage executable shadow; Go remains shadow-only | scoped shadow code/docs closed; live provider QA deferred | Codex | Promoted the D-0168 provider/cache coverage floor into `controlExecutableCases.providerCacheCoverageFloor`: TS conformance now derives the case from the provider-cache oracle, and Go G5 shadow computes required/covered provider families, usage/request-shape ids, endpoint formats, telemetry-supported cases, unsupported unknown cache posture, custom full endpoint exact-URL cases/tool shapes, DeepSeek/OpenAI/Anthropic splits, no live credentials, no live superiority claim, and no Reasonix/top-level route exposure. No provider behavior change, live Go provider client, default Go backend, renderer-visible Go route, Reasonix public provider protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, credentialed provider matrix, or packaged provider QA was added. |
| 2026-06-22 D-0168 Provider cache coverage floor | Reasonix / Kun / Go | Provider/cache oracle coverage guard; Go remains shadow-only | scoped schema/test/docs closed; live provider QA deferred | Codex | Tightened the provider-cache oracle schema so live-local proof provider families must cover DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full endpoints; added executable runtime tests deriving provider-family coverage, required usage case ids, request-shape ids, endpoint formats, telemetry-supported cases, unsupported unknown cache posture, and three custom full endpoint request-shape cases from the oracle. No provider behavior change, Reasonix public provider protocol, Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, default Go backend, renderer-visible Go route, Rust/Tauri path, credentialed provider matrix, or packaged provider QA was added. |
| 2026-06-22 D-0167 Runtime desktop bridge proof freshness scan | Reasonix / Kun / Go | Runtime bridge proof guard; Go remains shadow-only | scoped scan/docs closed; packaged route QA deferred | Codex | Extended `scan:product-sovereignty` with `runtimeDesktopBridgeProofPaths` so the renderer runtime provider/client, browser preview bridge, preload bridge/types, main IPC schemas/handlers, main SSE IPC, shared API/endpoints, and their focused guard tests must remain present. This keeps D-0156 and D-0162 through D-0166 runtime request/SSE bridge evidence from disappearing without adding Reasonix public protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA. |
| 2026-06-22 D-0166 Preload runtime request IPC bridge proof | Reasonix / Kun / Go | Preload runtime request facade proof; Go remains shadow-only | scoped test/docs closed; packaged route QA deferred | Codex | Added preload source guard proving `window.analytix.runtime.runtimeRequest(path, method, body)` maps only to `ipcRenderer.invoke('runtime:request', { path, method, body })` and does not introduce Reasonix/Kun/Go/Workflow runtime request IPC channels. No Reasonix public protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0165 Main runtime request forbidden route proof | Reasonix / Kun / Go | Main runtime request allow-list proof; Go remains shadow-only | scoped test/docs closed; packaged route QA deferred | Codex | Added explicit main IPC runtime request forbidden-route tests: `runtimeRequestPayloadSchema` now rejects Reasonix public routes, renderer-visible Go routes, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes, and a `runtime:request` handler test proves invalid payloads are rejected before `runtimeRequest` is invoked. No Reasonix public protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0164 Main/Preload SSE IPC bridge proof | Reasonix / Kun / Go | Desktop SSE IPC contract proof; Go remains shadow-only | scoped test/docs closed; packaged desktop QA deferred | Codex | Added desktop SSE IPC proof that preload maps `window.analytix.runtime.startSse/stopSse` only to `runtime:sse:*`, `sseStartPayloadSchema` preserves trimmed `threadId`/`sinceSeq`/`streamId` and rejects Reasonix-style extra fields, and `runtime-sse-ipc.test.ts` proves omitted stream ids are generated, first fetch uses `/v1/threads/:id/events?since_seq=...` with initial `Last-Event-ID`, error payloads carry the generated stream id, and `runtime:sse:stop` only aborts the matching stream id. No Reasonix public SSE protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0163 Renderer SSE bridge cursor proof | Reasonix / Kun / Go | Renderer SSE contract proof; Go remains shadow-only | scoped test/docs closed; packaged desktop QA deferred | Codex | Extended `analytix-runtime.test.ts` so `AnalytixRuntimeProvider.subscribeThreadEvents` proves the renderer SSE path starts through `window.analytix.runtime.startSse(threadId, sinceSeq, streamId)`, uses a generated non-empty stream id for event dispatch, avoids `runtimeRequest` as an SSE side channel, and calls `stopSse` with the same stream id on cleanup. No Reasonix public SSE protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0162 Renderer runtime request surface proof | Reasonix / Kun / Go | Renderer runtime contract proof; Go remains shadow-only | scoped test/docs closed; packaged desktop QA deferred | Codex | Added renderer-level executable proof in `analytix-runtime.test.ts`: real `AnalytixRuntimeProvider` calls for connect/list/create/turn/steer/interrupt/compact/goal/todos/approval/user-input/fork/resume now capture all `runtimeRequest` paths and assert they stay on `/health` or analytix-owned `/v1/*` routes while rejecting `/v1/reasonix`, `/v1/runtime/go`, `/v1/workflow(s)`, `/v1/create-loop`, `/v1/subagents?`, `/v1/autoresearch`, and `/v1/mcp-indexer`. No Reasonix public protocol, Kun identity, deprecated bridge/settings fallback, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0161 Runtime proof freshness scan | Reasonix / Kun / Go | Runtime conformance proof guard; Go remains shadow-only | scoped scan/docs closed; live Go/provider QA deferred | Codex | Extended `scan:product-sovereignty` with `runtimeProofFreshnessPaths` covering provider-cache, approval/user-input, G2/G3/G4/G5 fixtures/tests, and Go shadow sources; added active route/client forbidden-surface scanning for runtime routes, endpoint templates, IPC schema, and renderer runtime client. Also updated `go-runtime-conformance.md` currentness so G2 exact route replay is marked closed and provider-cache plus approval/user-input oracle evidence is shown as active G3/G4/G5 shadow input. No new provider/cache behavior oracle, Reasonix public protocol, Kun identity, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, default Go backend, renderer-visible Go route, Rust/Tauri path, credentialed provider matrix, or packaged QA was added. |
| 2026-06-22 D-0160 Workflow singular route negative proof | Reasonix / Kun / Go | Reasonix public-protocol rejection; Kun entry baseline unchanged; Go remains shadow-only | scoped test/docs closed; packaged route QA deferred | Codex | Extended the live TypeScript HTTP negative-route oracle so singular `/v1/workflow` returns structured 404 alongside plural `/v1/workflows`, `/v1/create-loop`, `/v1/subagents`, `/v1/autoresearch`, `/v1/mcp-indexer`, Reasonix public routes, and renderer-visible Go routes. This aligns live router proof with task-job / Go G4/G5 forbidden route fixtures that already record `/v1/workflow`. No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route, Reasonix SessionAPI/public protocol, Kun identity, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged route QA was added. |
| 2026-06-22 D-0159 Settings sovereignty scan freshness proof | Reasonix / Kun | Reasonix config/auto-plan rejection; Kun entry baseline unchanged | scoped scan/docs closed; packaged settings QA deferred | Codex | Extended `scan:product-sovereignty` with `settingsSovereigntyPaths` so shared settings normalization, active runtime settings, settings-store persistence, IPC settings patch schema, preload, browser preview bridge, and their focused guard tests must remain present. This converts the D-0155 through D-0158 bridge/settings sovereignty proofs into a repeatable path freshness gate without adding Reasonix config roots, public auto-plan settings, Kun/deprecated settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged settings QA. |
| 2026-06-22 D-0158 IPC settings patch sovereignty proof | Reasonix / Kun | Reasonix config/auto-plan rejection; Kun entry baseline unchanged | scoped code/tests/docs closed; packaged settings QA deferred | Codex | Extended desktop IPC settings patch sanitization so top-level `agent`/`autoPlan`/`auto_plan` are stripped before schema validation alongside `agentProvider`/`agents`/`deepseek`/`reasonix`/`quickChat`. `app-ipc-schemas.test.ts` now proves a polluted renderer->main settings patch keeps valid `locale`, `disabledSkillIds`, provider media fields, and runtime fields while dropping legacy Kun/Reasonix envelopes before persistence can occur. Runtime-level legacy agent-shaped keys still reject. No Reasonix config root/auto-plan setting, Kun/deprecated settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged settings QA was added. |
| 2026-06-22 D-0157 Browser preview settings sovereignty proof | Reasonix / Kun | Reasonix config/auto-plan rejection; Kun entry baseline unchanged | scoped code/tests/docs closed; packaged settings QA deferred | Codex | Extended settings normalization and browser-preview bridge tests so top-level `agentProvider`/`agents`/`deepseek`/`reasonix` envelopes are stripped alongside Reasonix `agent`/`autoPlan`/`auto_plan`, while runtime-level rejected fields continue to be stripped by `mergeAnalytixRuntimeSettings`. `browser-analytix-bridge.test.ts` now seeds polluted browser preview settings, proves top-level `runtime.model`/`endpointFormat` survive, and proves rejected app/runtime fields do not survive load or re-save. `app-settings.test.ts` now covers the shared normalize contract directly. No Reasonix config root/auto-plan setting, Kun/deprecated settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged settings QA was added. |
| 2026-06-22 D-0156 Browser preview SSE executable bridge proof | Reasonix / Kun | Reasonix public-protocol rejection; Kun entry baseline unchanged | scoped test/docs closed; packaged browser QA deferred | Codex | Promoted browser preview runtime/SSE bridge evidence from source guard into executable test coverage: `browser-analytix-bridge.test.ts` now starts a browser-preview SSE stream through `window.analytix.runtime.startSse`, proves the request goes to `/__analytix-runtime`-style analytix proxy paths with `Last-Event-ID`, and verifies SSE `id`/`event`/`data` normalize into analytix event payloads with `seq` and `kind`. No Reasonix SessionAPI/public protocol, Kun/deprecated bridge alias, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop/browser QA was added. |
| 2026-06-22 D-0155 Browser preview bridge and visible entry sovereignty | Reasonix / Kun | Reasonix public-protocol rejection; Kun entry baseline preserved | scoped tests/scan/docs closed; packaged desktop QA deferred | Codex | Added source and rendered-shell guards for product sovereignty outside the Go path: `Workbench.route-surface.test.ts` now includes the browser preview bridge in Workflow/Create Loop quarantine and proves it installs only `window.analytix` while proxying runtime/SSE through analytix paths; `Sidebar.test.ts` renders the sidebar and rejects visible Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer labels; `scan:product-sovereignty` now has path freshness coverage for Workbench, Sidebar, preload/shared contracts, browser bridge, and the guard tests. No Reasonix public protocol, Kun/deprecated bridge alias, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, default Go backend, renderer-visible Go route, Rust/Tauri path, or packaged desktop QA was added. |
| 2026-06-22 D-0154 G5 exact session route replay control shadow | Reasonix / Kun | Reasonix session/fork/SSE route review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go HTTP server deferred | Codex | Promoted thread/session route proof from status/inventory summaries into `controlExecutableCases.sessionRouteReplay`: TS conformance now derives an 11-row exact replay matrix from the G2 route oracle with method, path, setup, auth, response kind, status, request-body hash, response-body shape/hash, SSE frame count, SSE event names, and SSE frame hash; Go G5 shadow executable-replays exact JSON body route count, exact SSE route count, runtime-token coverage, unauthorized ids, archive/search/fork/resume body hashes, replay/caught-up SSE hashes, and no Reasonix protocol/top-level/renderer Go/default backend flags. No live Go HTTP server, Reasonix SessionAPI/public route protocol, renderer-visible Go route, default backend, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, Kun identity, deprecated bridge/settings fallback, Rust/Tauri path, or packaged route QA was added. |
| 2026-06-22 D-0153 G5 auto-router recommendation/currentness control shadow | Reasonix / Kun | Reasonix post-881 classifier currentness review; Kun baseline unchanged | scoped shadow/docs/tests closed; session exact route replay deferred | Codex | Extended `controlExecutableCases.autoRouterClassifier` with TS-derived recommendation parsing and recent-context boundary evidence: Go G5 shadow now executable-replays two accepted classifier recommendations, two rejected recommendations (`auto` and malformed), pro/max acceptance, active-turn exclusion from recent context, tool-result summary preservation, isolated `_auto_router` request, timeout fallback, classifier fingerprint currentness, no stable-prefix classifier state, no Reasonix controller protocol, and no top-level route exposure. No public auto-plan setting, Reasonix SessionAPI/config root, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, deprecated bridge/settings fallback, Rust/Tauri path, or packaged desktop QA was added. Next high-value deferred candidate is exact per-route G5 session replay for fork/resume/archive/search/SSE. |
| 2026-06-22 D-0152 G5 desktop bridge/settings sovereignty control shadow | Reasonix / Kun | Reasonix config/protocol review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go desktop integration deferred | Codex | Promoted desktop bridge and settings sovereignty into `controlExecutableCases.desktopSovereignty`: TS conformance now reads real preload/window/API/settings sources and Go G5 shadow executable-replays single `window.analytix` bridge exposure, `Window.analytix` type ownership, analytix-owned facade domains, Reasonix `autoPlan`/`auto_plan` drop proof, legacy `agentProvider`/`agents` envelope drop proof, top-level `runtime` endpoint-format persistence proof, no deprecated bridge alias, no deprecated settings fallback write, no Reasonix protocol, and no top-level route exposure. No live Go desktop integration, Reasonix public protocol/config root, deprecated bridge/settings fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0151 G5 planner gate matrix control shadow | Reasonix / Kun | Reasonix planner/auto-plan review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go planner deferred | Codex | Promoted planner enable/disable and blocked-tool policy into `controlExecutableCases.planner`: Go G5 shadow now executable-replays normal agent hiding `create_plan`, plan capability gate advertisement, step 0 read-only plus `create_plan`, step >0 only `create_plan`, and rejection of forged `task`, `parallel_tasks`, `bash`, `edit`, `write`, and `echo` calls with no execution. No live Go planner/executor, Reasonix planner/session public protocol, public auto-plan setting, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0150 G5 user-input structured validation control shadow | Reasonix / Kun | Reasonix AskTool/user-input review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go gate manager deferred | Codex | Promoted structured `request_user_input` validation into `controlExecutableCases.userInput.structuredChoiceValidation`: Go G5 shadow now executable-replays max questions, option-count bounds, case-insensitive duplicate-label dedupe, invalid result code, invalid case count, four reject booleans, invalid-no-gate behavior, no Reasonix protocol, and no top-level route exposure. No live Go approval/user-input manager, Reasonix ask/session public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0149 G5 context compaction boundary control shadow | Reasonix / Kun | Reasonix long-thread/currentness review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go loop deferred | Codex | Promoted latest-compaction effective-history boundary behavior into `controlExecutableCases.compactionBoundary`: Go G5 shadow now executable-replays effective ids, dropped ids, latest positive compaction boundary, noop/older/pre-boundary drops, post-boundary preservation, stable-prefix isolation, no Reasonix protocol, and no top-level route exposure. No live Go history manager, Reasonix SessionAPI/controller protocol, public auto-plan setting, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0148 G5 history repair pair-integrity control shadow | Reasonix / Kun | Reasonix model-history/tool-result repair review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go loop deferred | Codex | Promoted model-history repair and tool-call/result pair integrity into `controlExecutableCases.historyRepair`: Go G5 shadow now executable-replays complete multi-tool blocks, assistant/approval/user-input bridge items, orphan result drops, missing-result call drops, duplicate result drops, stable-prefix isolation, no Reasonix protocol, and no top-level route exposure. No live Go agent loop, Reasonix SessionAPI/controller protocol, public auto-plan setting, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0147 G5 step-limit override/delegate matrix control shadow | Reasonix / Kun | Reasonix agent-kernel step-limit/delegation review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go loop deferred | Codex | Promoted step-limit evidence from scalar fields into a 9-row override/delegate matrix: Go G5 shadow now executable-replays default, user-global, session, turn, planner, headless, zero-default, delegate parent-half, and delegate min-floor rows with configured/fallback/effective/source plus stable-prefix, disable-guard, and delegate-floor flags. No live Go agent loop, Reasonix controller/session protocol, public auto-plan setting, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0146 G5 combined step/cancel/cache trace control shadow | Reasonix / Kun | Reasonix agent-kernel step/cancel/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go loop deferred | Codex | Promoted combined auto-route cache, step-limit, and cancel evidence from summary booleans into exact trace replay: Go G5 shadow now executable-replays same-turn router calls, main/max model steps, next-turn router calls, classifier/step-limit stable-prefix flags, accepted/cancel result counts, completed/aborted result counts, and exact cancel result rows. No live Go agent loop, Reasonix controller/session protocol, public auto-plan setting, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0145 G3/G5 provider request-shape exact matrix control shadow | Reasonix / Kun | Reasonix provider/cache request-shape review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Promoted provider request-shape evidence from summary counts into exact matrix replay: Go G3/G5 shadow now executable-replays all 7 TS-owned request-shape cases with exact URL, required/forbidden headers, required/forbidden body fields, reasoning-effort presence, and tool-shape family. Also forced Go fixture validation with `go test -count=1` so JSON oracle updates cannot be hidden by test cache. No live Go provider client, Reasonix provider protocol, credentialed live provider matrix, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0144 G5 MCP core lifecycle boundary seal control shadow | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Sealed MCP core lifecycle provider identity and product-boundary flags inside `controlExecutableCases.mcpCoreLifecycle`: Go shadow now executable-replays `providerId: "mcp:research"`, no Reasonix protocol, and no top-level route exposure alongside connect/disconnect/reload/cancel/error output from the TS-owned MCP lifecycle oracle. No live Go MCP client, Reasonix MCP-indexer public protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0143 G5 task planner toolset inventory control shadow | Reasonix / Kun | Reasonix planner/sub-agent job review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Promoted task/sub-agent planner read-only and forbidden toolset inventory from G5 `jobReplay` summary into `controlExecutableCases.taskJobs.plannerToolsetInventory`: Go shadow now executable-replays planner read-only tools, forbidden `task`/`parallel_tasks`, tool counts, read-only exclusion of task tools, forbidden task-tool match, planner/executor policy, no Reasonix protocol, and no top-level route exposure. No live Go planner/executor, public sub-agent/job/planner protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0142 G5 approval/user-input inventory control shadow | Reasonix / Kun | Reasonix approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; packaged approval-card QA deferred | Codex | Promoted approval/user-input gate inventory from the route oracle into `controlExecutableCases.approvalUserInputInventory`: Go shadow now executable-replays gate ids, approval ids, user-input ids, route kinds, replay kind order, abort replay kind order, answer count, HTTP answer echo vs resolved-event no-answer privacy, late statuses, pending-after sum, no Reasonix protocol, and no top-level route exposure. No live Go approval/user-input manager, Reasonix ask/session public protocol, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0141 G5 session route inventory control shadow | Reasonix / Kun | Reasonix session/fork/SSE route review; Kun baseline unchanged | scoped shadow/docs/tests closed; packaged route QA deferred | Codex | Promoted thread/session route inventory from the G2 route oracle into `controlExecutableCases.sessionRouteInventory`: Go shadow now executable-replays route ids, JSON/SSE/event/resume/fork/archive/search/read-update groupings, runtime-token protected route count, unauthorized route ids, no Reasonix protocol, and no top-level route exposure. No live Go HTTP server, Reasonix SessionAPI/public route protocol, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0140 G5 provider cache inventory control shadow | Reasonix / Kun | Reasonix provider/cache prefix currentness review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Promoted provider cache prefix/tool/case-id inventory from loose G5 `cacheReplay` summary fields into `controlExecutableCases.providerCacheInventory`: Go shadow now executable-replays stable prefix hash, canonical tools hash, provider usage case ids/count, request-shape case ids/count, prefix equivalence, tools hash stability, no Reasonix protocol, and no top-level route exposure. No live Go provider client, Reasonix provider protocol, dynamic stable-prefix material, credentialed provider matrix, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0139 G5 provider cache privacy control shadow | Reasonix / Kun | Reasonix provider/cache privacy review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Promoted provider cache diagnostics privacy and live-superiority policy from G5 `cacheReplay` summary into `controlExecutableCases.providerCachePrivacy`: Go shadow now executable-replays diagnostics field count, forbidden diagnostics substring count, no stable-prefix/tool-schema/API-key/header leak, fixture-only policy, no live credentials, no live superiority claim, no Reasonix protocol, and no top-level route exposure. No live Go provider client, Reasonix provider protocol, credentialed provider matrix, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0138 G5 MCP search meta-tool control shadow | Reasonix / Kun | Reasonix MCP/indexer currentness review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted MCP search meta-tool advertised/trust/no-execute evidence from G5 `mcpReplay` summary into `controlExecutableCases.mcpSearchMetaTools`: Go shadow now executable-replays meta-tool names/count, refresh tool advertisement, trusted/untrusted workspace fields, trusted tool id, unknown-tool error, `on-request` policy, denied no-execute, no Reasonix protocol, and no top-level route exposure. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0137 G5 MCP live-local indexer control shadow | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted MCP live-local indexer lifecycle from G5 `mcpReplay` summary into `controlExecutableCases.mcpLiveLocalIndexer`: Go shadow now executable-replays server/cwd, retry ids/map, initial/resume/active paths, tombstone count, snapshot restart, late tombstone, secret-safe diagnostic, execution-error redaction, no Reasonix protocol, and no top-level route exposure. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0136 G5 MCP known override diagnostics control shadow | Reasonix / Kun | Reasonix MCP lifecycle/config override review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted MCP known override diagnostics from G5 `mcpReplay` summary into `controlExecutableCases.mcpKnownOverrideDiagnostics`: Go shadow now executable-replays `codegraph`/`codebase-memory` override kinds, effective cwd, workspace roots, explicit-cwd server ids, daemon-timeout server ids, all-low-priority/all-background-start booleans, no Reasonix protocol, and no top-level route exposure. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0135 G5 nested child SSE metadata control shadow | Reasonix / Kun | Reasonix task/sub-agent job review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Promoted nested child SSE metadata evidence from G5 `jobReplay` summary into `controlExecutableCases.taskJobs.nestedSseMetadata`: Go shadow now executable-replays parent call id, child run id, nested SSE metadata fields, evidence ledger metadata keys, parent/child distinctness, active-goal requirement, no Reasonix protocol, and no top-level route exposure. No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0134 G5 task transcript identity control shadow | Reasonix / Kun | Reasonix task/sub-agent job review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Promoted transcript continue/fork identity evidence from G5 `jobReplay` summary into `controlExecutableCases.taskJobs.transcriptIdentity`: Go shadow now executable-replays source id, continue target id, fork target id, incompatible identity error, same-transcript identity requirement, continue-preserves-target, fork-creates-distinct-target, continue-target-matches-source, fork-target-distinct-from-source, no Reasonix protocol, and no top-level route exposure. No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0133 G5 task-job tool contract boundary control shadow | Reasonix / Kun | Reasonix task/sub-agent job review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Promoted task/parallel task tool contract and route-boundary evidence from G5 `jobReplay.toolContractBoundary`/`routeBoundary` summary into `controlExecutableCases.taskJobs.toolContractBoundary`: Go shadow now executable-replays `task`/`parallel_tasks` internal runtime-only flags, permission/evidence/dependency/read-only requirements, runtime task-job routes, protected route auth, unauthorized status, forbidden top-level routes, no Reasonix protocol, and no top-level route exposure. No live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0132 G5 product boundary control shadow | Reasonix / Kun | Reasonix Go/runtime boundary review; Kun baseline unchanged | scoped shadow/docs/tests closed; default Go backend rejected | Codex | Promoted the global G5 product boundary from summary/full-loop metadata into `controlExecutableCases.productBoundary`: Go shadow now executable-replays `window.analytix`/preload-main bridge stability, `analytix serve` contract stability, no Reasonix public protocol, no default Go backend, no renderer-visible Go route, no Electron main connection, and no enabled Go backend. No live Go backend switch, Reasonix protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0131 G5 MCP background reconnect control shadow | Reasonix / Kun | Reasonix MCP lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted `mcpReplay.backgroundReconnect` from G5 shadow summary into `controlExecutableCases.mcpBackgroundReconnect`: Go shadow now executable-replays failed server ids, suspended provider/reason, connected/error server outcomes, retry attempts, all-failed retry coverage, and no-runtime-restart state from the TS-owned MCP lifecycle oracle. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0130 G5 MCP core lifecycle control shadow | Reasonix / Kun | Reasonix MCP lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted `mcpReplay.lifecycle` from G5 shadow summary into `controlExecutableCases.mcpCoreLifecycle`: Go shadow now executable-replays connect/disconnect/reload/cancel/error core MCP lifecycle behavior, including tool catalog diagnostics, schema-order stability, cancel-before-start no-execute, and approved error shape from the TS-owned MCP lifecycle oracle. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0129 G5 MCP search workspace boundary control shadow | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted `mcpReplay.searchWorkspaceBoundary` from G5 shadow summary into `controlExecutableCases.mcpSearchWorkspaceBoundary`: Go shadow now executable-replays trusted/untrusted workspace search boundaries, query, trusted tool id, untrusted search count `0`, unknown-tool error, `on-request` call policy, and denied no-execute state from the TS-owned MCP lifecycle oracle. No live Go MCP client, live Go approval manager, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0128 G5 MCP approval annotation control shadow | Reasonix / Kun | Reasonix MCP/tool approval lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP/approval managers deferred | Codex | Promoted `mcpReplay.approvalAnnotations` from G5 shadow summary into `controlExecutableCases.mcpApprovalAnnotations`: Go shadow now executable-replays destructive/open-world MCP tool metadata, normalized tool name, approval id, deny decision, approval result kind, and denied no-execute state from the TS-owned MCP lifecycle oracle. No live Go MCP client, live Go approval manager, Reasonix MCP-indexer/approval protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0127 G5 MCP search refresh drift control shadow | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Promoted `mcpReplay.searchRefreshDrift` from G5 shadow summary into `controlExecutableCases.mcpSearchRefreshDrift`: Go shadow now executable-replays refresh catalog drift from initial tool names to expanded tool names, total indexed count, catalog drift, and no-top-level-route state from the TS-owned MCP lifecycle oracle. No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0126 G5 approval/user-input route replay control shadow | Reasonix / Kun | Reasonix approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go gate managers deferred | Codex | Promoted `approvalUserInputReplay` from G5 shadow summary into `controlExecutableCases.approvalUserInputRouteReplay`: Go shadow now executable-replays approval deny route body/status, submitted and cancelled user-input routes, replay order, late action rejection, abort cleanup, and no-pending-gates state from the TS-owned approval/user-input route oracle. No live Go approval/user-input manager, Reasonix SessionAPI/ask protocol, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry was added. |
| 2026-06-22 D-0109 Workflow/Create Loop quarantine scan | Kun / Reasonix | Kun baseline/product sovereignty review; Reasonix public protocol rejected | scoped scan/test closed; dormant code remains quarantined | Codex | Added explicit quarantine evidence that dormant `WorkflowCreateLoopView` and `create-loop-runtime` code cannot be imported or invoked by top-level app entry surfaces: Workbench/Sidebar/Write/Plugins/store/preload/tray/settings shortcuts now participate in a route-surface test and reusable `scan:product-sovereignty` check for `WorkflowCreateLoopView`, `runCreateLoopWorkflow`, `findPendingWorkflowGate`, and `analytix-create-loop`. No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, Kun identity, Reasonix protocol, default Go backend, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0108 MCP stdio execution-error redaction | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped runtime fix/test closed; credentialed MCP QA deferred | Codex | Fixed MCP tool execution privacy so both thrown MCP errors and protocol-level `isError` result payloads are redacted before they become model-visible `tool_result` output. The executable stdio fake indexer now calls `mcp_codegraph_index_fail_diagnostic` and proves `Authorization=<redacted>` is preserved while `Bearer lifecycle-secret` is absent. No Reasonix MCP-indexer public protocol, top-level MCP-indexer route/navigation, Kun identity, default Go backend, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0107 Plan step cancel/cache boundary guard | Reasonix / Kun | Reasonix planner step/cancel/cache review; Kun baseline unchanged | scoped loop test closed; packaged Plan mode QA deferred | Codex | Added a real AgentLoop fixture proving explicit analytix Plan mode keeps step gating and cache diagnostics stable across cancellation: step 0 advertises read-only tools plus `create_plan`, step 1 narrows to `create_plan`, interrupting step 1 aborts the turn, and the next Plan turn still reports `prefixChanged: false` from the original DeepSeek-style cache baseline. No Reasonix auto-plan protocol, public SessionAPI, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, Kun identity, default Go backend, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0106 HTTP auto-plan payload boundary guard | Reasonix / Kun | Reasonix `01d9b173` auto-plan config review; Kun baseline unchanged | scoped HTTP/runtime test closed; product auto-plan setting deferred | Codex | Added an HTTP start-turn negative fixture proving Reasonix-shaped `autoPlan` / `auto_plan` payload fields are stripped by the analytix `StartTurnRequest` contract: the turn remains an agent turn, `guiPlan` is absent, and the model request does not advertise `create_plan`. Only analytix-owned `mode: "plan"` / `guiPlan` can enter Plan mode. No Reasonix controller/config protocol, public SessionAPI, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, Kun identity, default Go backend, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0105 Write sidebar and Connect Phone placeholder sovereignty | Kun / Reasonix | Kun baseline unchanged; Reasonix public protocol rejected | scoped scan/copy/tests closed; packaged desktop QA deferred | Codex | Expanded route-surface and `scan:product-sovereignty` coverage to include Write sidebar, workspace mode tabs, and sidebar projects section, plus camel/no-hyphen forbidden entry variants. New generated Connect Phone mapped-conversation placeholders now use `[Connect Phone:...]` while legacy `[Claw:]` recognizers remain compatibility-only. No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, Reasonix public protocol, Kun identity, default Go backend, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0104 G5 parallel task dependency executable shadow | Reasonix / Kun | Reasonix sub-agent/job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Promoted `parallel_tasks` dependency validation from G5 `jobReplay` summary into `controlExecutableCases.taskJobs.parallelValidation`: Go shadow now executes valid DAG ordering plus single-task, duplicate-id, self-dependency, cycle, and unknown-dependency rejection against TS-owned expected strings. No live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0096 G5 task tool contract boundary replay | Reasonix / Kun | Reasonix task/sub-agent tool contract review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added G5 `jobReplay.toolContractBoundary` so `task` and `parallel_tasks` field names, internal-runtime-only flags, permission/dependency validation, planner read-only requirement, no Reasonix protocol, and no top-level route replay from the TS task-job oracle through Go shadow. No planner product toggle, live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0095 G5 planner-executor detail replay | Reasonix / Kun | Reasonix planner/sub-agent job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added G5 `jobReplay.plannerExecutor` detail fields for skipped dependency reason, cancel reason, output-offset job count, and transcript-propagation job count, all replayed from the TS task-job oracle through Go shadow. No planner product toggle, live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0094 G5 task parent goal evidence replay | Reasonix / Kun | Reasonix sub-agent/job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added G5 `jobReplay.parentGoalEvidence` so task/sub-agent parent-goal evidence requirements, ledgered/error event keys, no Reasonix protocol, and no top-level route replay from the TS task-job oracle through Go shadow. No live Go Job Manager, public sub-agent/job protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0093 G5 MCP approval annotation replay | Reasonix / Kun | Reasonix MCP/tool approval review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go approval/MCP client deferred | Codex | Added G5 `mcpReplay.approvalAnnotations` so destructive/open-world MCP tool metadata, normalized tool name, approval id, deny decision, approval result kind, and denied no-execute state replay from the TS MCP lifecycle oracle through Go shadow. No live Go approval manager, live Go MCP client, Reasonix MCP/indexer protocol, top-level MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0092 G5 MCP live-local indexer summary replay | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Expanded G5 `mcpReplay.liveLocalIndexer` so MCP live-local indexer evidence includes server/cwd/priority/background-start, retry attempts, active paths, tombstone count, snapshot restart, secret redaction/no-leak, late tombstone, and no top-level route. The summary derives from the TS MCP lifecycle oracle and `runLiveLocalIndexerProof` while Go only computes fixture output. No live Go MCP client, MCP route, Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0091 G5 provider live-local proof summary replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go provider deferred | Codex | Added `provider-cache-oracle.liveLocalHttpProof` and G5 `cacheReplay.liveLocalHttpProof` so local HTTP provider proof is no longer only implicit in test names. TS and Go derive usage/request-shape/post counts and endpoint formats from oracle arrays, preserving fixture-only/no-live-credentials/no-live-superiority policy. No live Go provider client, provider behavior change, settings/bridge/runtime route change, Reasonix protocol, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0090 provider live-local HTTP executable proof | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped tests/docs closed; credentialed provider matrix deferred | Codex | Expanded `provider-cache-proof.test.ts` so all 5 provider usage cases and all 7 request-shape cases execute through a local HTTP provider while preserving original provider URL/host request construction. This proves DeepSeek native cache, unsupported OpenAI-compatible unknown cache, OpenAI Responses cached tokens, Anthropic cache fields, and custom full endpoint path/header/body/tool-shape behavior without live credentials. No provider client behavior, settings, bridge, runtime route, default Go backend, Reasonix protocol, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0089 G5 task-job route executable replay | Reasonix / Kun | Reasonix task/job review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added `task-job-orchestration-oracle.routeExecutable` and G5 `jobReplay.routeExecutable`: unauthorized `401`, output route status/offset/replay offset, wait completed result, kill status/error, missing output `404`, and rehydrated output/wait/kill statuses now bind TS route tests to Go shadow summary. No live Go task-job route, public sub-agent/job protocol, renderer-visible Go route, default backend, Reasonix SessionAPI, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0088 G5 provider streaming usage replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G5 `providerStreamingReplay` derived from the G3 provider streaming/usage/cache oracle: SSE frame count, `item_delta`/`usage`/`turn_completed` event order, DeepSeek usage prompt/completion/reasoning/total/cache hit/miss/rate, telemetry support, and product boundary now emit through Go shadow. No provider request/stream parser behavior, renderer-visible Go route, default backend, Reasonix protocol, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0087 G5 session route status replay | Reasonix / Kun | Reasonix session/fork/SSE review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go route/backend deferred | Codex | Added G5 `sessionReplay.routeStatusReplay` derived from the G2 route oracle: route/status counts, archive/search counts and status, read latest seq/update workspace, fork side relation/parent lineage, resume session/message summary, SSE replay/caught-up frame counts, and replay event names now emit through Go shadow. No renderer-visible Go route, default backend, Reasonix protocol, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry was added. |
| 2026-06-22 D-0086 G5 provider request-shape replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G5 `cacheReplay.requestShapeReplay` to the full-loop shadow summary: exact URL count, endpoint families, custom full endpoint ids, tool-shape families, and required/forbidden body-field family counts now derive from `ProviderCacheOracle.requestShapeCases` and Go shadow. No provider URL/body/header/stream/usage behavior, live provider matrix, Reasonix protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0085 G3 provider request-shape replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G3 `requestShapeMatrix` and `requestShapeSummary` to the provider streaming/usage/cache oracle: exact URL count, endpoint families, custom full endpoint ids, tool-shape families, and required/forbidden body-field family counts now derive from `ProviderCacheOracle.requestShapeCases` and Go shadow. No provider URL/body behavior, stream parser, usage parser, live provider matrix, Reasonix protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0084 G3 cache drift attribution replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G3 `cacheDriftAttribution` to the provider streaming/usage/cache oracle: raw previous/current shapes now stay tied to `ProviderCacheOracle`, expected output computes stable system/prefixItems proof, changed tool/provider/model/endpoint flags, expected reasons, and unsupported telemetry/cache-hit-rate status through Go shadow. No provider URL/body behavior, live provider matrix, Reasonix protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0083 cache drift attribution replay | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G5 `cacheReplay.driftAttribution` derived from the provider-cache oracle: previous/current prefix hashes, stable system/prefixItems proof, changed tool/provider/model/endpoint flags, expected reasons `tools`/`provider`/`model`, and fixture-only unknown telemetry/cache-hit-rate evidence now replay through Go shadow. No dynamic context was admitted into the stable prefix, and no live provider superiority, Reasonix protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0082 offline provider/cache parity seal | Reasonix / Kun | Reasonix provider/cache review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider matrix deferred | Codex | Added G5 `cacheReplay.offlineParitySeal` derived from the provider-cache oracle: stable/equivalent DeepSeek prefix hashes, prefix item/tool hash stability, DeepSeek stable cache hit/miss/rate, diagnostics support, multi-provider usage/request-shape counts, endpoint families, release guard status, and fixture-only/no-live-superiority boundary now replay through Go shadow. No live credential matrix, Reasonix provider protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0081 Go G5 approval decision route replay | Reasonix / Kun | Reasonix approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go approval manager deferred | Codex | Added G5 `approvalUserInputReplay` fields for approval decision route evidence: approval id, deny decision, denied status, pending-after cleanup, second decision `409`, replay `sinceSeq`, and replay kinds now derive from the TS approval/user-input route oracle and Go shadow. No Reasonix ask/session protocol, live Go approval manager, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0080 Go G5 MCP core lifecycle replay | Reasonix / Kun | Reasonix MCP lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go MCP client deferred | Codex | Added G5 `mcpReplay.lifecycle` for MCP connect/disconnect/reload/cancel/error evidence: tool names, availability/tool counts, disconnect reason, schema-order stability, cancel no-execute, and approved error code now replay from the TS MCP lifecycle oracle through Go shadow. No live Go MCP client, MCP route, top-level MCP-indexer, Reasonix protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0079 Go G5 task job route boundary replay | Reasonix / Kun | Reasonix sub-agent/job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added G5 `jobReplay.routeBoundary` for internal task-job route auth and forbidden top-level routes: protected wait/output/kill route ids, unauthorized `401`, and `/v1/workflow` / `/v1/create-loop` / `/v1/autoresearch` rejection evidence now replay from the TS task-job oracle through Go shadow. No live Go task-job routes, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-22 D-0078 Go G5 task job lifecycle replay | Reasonix / Kun | Reasonix sub-agent/job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go Job Manager deferred | Codex | Added G5 `jobReplay.lifecycle` for the base task job lifecycle: foreground completion, background cross-turn running/output/final completion, and wait/output/kill cancellation status/error are now derived from the TS task-job oracle and emitted by Go shadow. No live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, default Go backend, Kun identity, Rust/Tauri path, or hidden product entry was added. |
| 2026-06-20 code-level upstream audit | Kun / Reasonix | local audit snapshots | complete | Codex | Produced `code-level-absorption-blueprint.md` and `code-level-implementation-plan.md`; no implementation absorbed. |
| 2026-06-20 P0/P2 Reasonix session-sidecar absorption | Reasonix | `main-v2` refreshed to `ef7bf97` | complete | Codex | Absorbed `249a4f8`, `73e2025`, and `5db1d0b` through analytix thread summaries and hybrid sidecars; `f6ba755` memory/control lock split is deferred. |
| 2026-06-20 P2.1 Reasonix sidecar-version absorption | Reasonix | `main-v2` refreshed to `6d404d8` | complete | Codex | Absorbed `7ebb08e` with analytix `metadata.jsonl` summary `schemaVersion`; classified `341f720` as future Go G4/G5 controller-lock conformance because TS runtime has no equivalent global controller lock. |
| 2026-06-20 architecture/spec v1 closure and Kun master drift audit | Kun / Reasonix | Kun `master` `8602476`, Kun `develop` `ab24a77`, Reasonix `main-v2` `6d404d8` | complete | Codex | Closed planning baseline after Kun master merged develop; no code implementation absorbed. |
| 2026-06-20 P1.1 Kun stable critical code absorption | Kun / Reasonix | Kun `master` `8602476`; Kun `develop` `ab24a77`; Reasonix `main-v2` `be67a49` | closed for code slice | Codex | Absorbed Kun provider/runtime/preload critical fixes through analytix-native contracts, including SQLite `provider_id` list summaries and sidecar rebuild proof; Reasonix `ad3d742` was classified and deferred because it does not affect P1.1. |
| 2026-06-20 P1.2 Connect Phone / Telegram absorption | Kun / Reasonix | Kun `master` `8602476`; Kun `develop` `ab24a77`; Reasonix `main-v2` `be67a49` | closed for code slice | Codex | Absorbed Kun Telegram Connect as an analytix-native Connect Phone runtime slice with local token verification, long polling, private-chat allowlist, inbound attachment propagation, Telegram reply/mirror paths, and provider/model-preserving runtime turns; no Reasonix code change was required. |
| 2026-06-20 P1.3 Workflow / Create Loop absorption | Kun / Reasonix | Kun `master` `8602476`; Kun `develop` `ab24a77`; Reasonix `main-v2` `be67a49` | superseded by P0 entry correction | Codex | The underlying Create Loop state machine/tests were retained, but the renderer Workflow route/top-level sidebar entry was later found to be a product-position error for the 0.2.13/0.2.14 baseline and was removed/hidden by the P0 product-entry correction row below. |
| 2026-06-20 P2.2 Reasonix provider/cache/tool lifecycle absorption | Reasonix / Kun | Reasonix `main-v2` `be67a49`; Kun `master` `8602476`; Kun `develop` `ab24a77` | closed for scoped code slice | Codex | Absorbed Reasonix cache-shape diagnostics as optional analytix runtime `usage.cacheDiagnostics`: stable prefix/tool/provider/model hashes, prefix-change reasons, provider cache hit/miss tokens, and sanitized evidence for tool-schema cache drift. Kun was checked only for P1.1/P1.2/P1.3 regression risk. |
| 2026-06-20 P3A Tool/MCP/Sandbox/Checkpoint boundary | Reasonix / Kun | Reasonix `main-v2` `be67a498`; Kun `master` `8602476`; Kun `develop` `ab24a77` | closed for scoped code slice | Codex | Rechecked upstream HEADs; confirmed Reasonix `6d404d8..be67a498` target-engine drift is only desktop topic migration; added MCP malformed schema normalization and denied-approval-no-execute proof as TS runtime/Go G4 oracle fixtures. Kun was audited as 0.2.13 baseline fidelity plus 0.2.14/8602476 delta/regression input. |
| 2026-06-20 P4A Checkpoint/Rewind safety oracle | Reasonix / Kun | Reasonix `main-v2` `be67a498`; Kun `master` `8602476`; Kun `develop` `ab24a77` | closed for scoped code slice | Codex | Rechecked upstream HEADs with no drift; converted Reasonix engine/runtime checkpoint/rewind capability into analytix-owned `axcp_` checkpoint metadata, `checkpoint_captured` event projection, path-escape guard, first-touch-wins changed-file oracle, and conversation-only rewind planning. Kun was handled as 0.2.13 baseline fidelity, 0.2.14/8602476 checkpoint delta closure, and upgrade-regression input; no Kun refs, bridge, UI, or identity were adopted. |
| 2026-06-20 P4B Full rewind/review UI plan-only slice | Reasonix / Kun | Reasonix `main-v2` `be67a498`; Kun `master` `8602476`; Kun `develop` `ab24a77` | closed for scoped code slice | Codex | Rechecked upstream HEADs with no drift; added analytix-owned `axrp_` auditable rewind plans for code-only, conversation-only, and combined scopes, plus route/service/shared endpoint/IPC/provider and existing review/history/ChangeInspector display. P4B is plan-only: no file apply, no event rewrite, no git refs, no Kun/Reasonix identity. |
| 2026-06-20 P4C Confirmed rewind restore apply | Reasonix / Kun | Reasonix `main-v2` `be67a498`; Kun `master` `8602476`; Kun `develop` `ab24a77` | closed for scoped code slice with desktop smoke; release/live apply fixture remains gated | Codex | Added confirmed destructive apply based only on P4B `CheckpointRewindPlan`: `axra_` apply ids, `axrr_` rescue records, explicit confirmation phrase, route/service/shared endpoint/IPC/provider, and existing ChangeInspector/TurnChangeSummary controls. Apply blocks stale/tampered plans, path escape, absolute paths, parent/final symlinks, staged/untracked conflicts, manual-review files, missing snapshot/hash evidence, and snapshot content/hash mismatches; mutation revalidates safety inside the file queue. Conversation restore is append-only audit rather than transcript rewrite. |
| 2026-06-20 G0/G5 conformance inventory runtime baseline | Kun / Reasonix | Kun 0.2.13 baseline, Kun `master` `8602476`, Reasonix `main-v2` `be67a498`, local `b3e1674` | closed for inventory baseline; Go implementation pending | Codex | Closed the full runtime suite baseline red lights by updating tests to match current post-file-change and prompt-token-trust contracts, froze the TS runtime contract inventory for thread/session, SSE replay, checkpoint/rewind plan/apply, safety audit, tools, approvals, user input, plan/goal, model-history repair, cache, usage, and provider parsing, and recorded G5 golden/oracle gaps without starting Go scaffold. |
| 2026-06-20 Reasonix goal/control delta oracle | Reasonix / Kun | Reasonix `main-v2` refreshed to `bc8249c3`; Kun `master` `8602476`; Kun `develop` `ab24a77`; Kun `v0.2.13`/`v0.2.14` tags rechecked | closed for scoped code/doc slice; Go implementation pending | Codex | Absorbed Reasonix `dbaea843` as an analytix-owned `GoalControlMachine` helper and direct runtime oracles for empty post-file-change final-answer failure and prompt-token anti-inflation. Classified `bb06f5b4` inspect/lockfile cleanup as no-op for analytix and `726036bd` approvalManager drift as deferred. No Reasonix/Kun public protocol, identity, settings, bridge, CLI, or Go scaffold was added. |
| 2026-06-20 post-goal-control Reasonix store-sidecar refresh | Reasonix / Kun | Reasonix `main-v2` refreshed to `d02457ee`; Kun `master` `8602476`; Kun `develop` `ab24a77`; Kun `v0.2.13`/`v0.2.14` tags rechecked | record-only currentness refresh; Go implementation pending | Codex | Classified Reasonix `98a57ded` / merge `d02457ee` as a byte-identical store-sidecar authority refactor. No current analytix code was absorbed because TS runtime storage contracts remain authoritative; record the leaf-store idea for a future Go G2/G5 store batch. `bc8249c3` remains the post-4f515da branch baseline and `be67a498` is historical P3A evidence. |
| 2026-06-20 post-4f47031f upstream currentness refresh | Reasonix / Kun | Reasonix `main-v2` refreshed to `48e5b990`; Kun `master` `8602476`; Kun `develop` `9605e20f`; Kun `v0.2.13`/`v0.2.14` tags rechecked | currentness refresh; superseded by control-port absorption row | Codex | Classified Reasonix `3e625b91` / merge `48e5b990` as the SessionAPI/control-port batch that the following row scoped-absorbed. Classified Kun `d09d52b` on develop as a later Windows installer process-stop `must absorb` release batch. Release blockers and Go G1-G6 remain open. |
| 2026-06-20 Reasonix remote-entry control-port absorption | Reasonix / Kun | Reasonix target `48e5b990` / `3e625b91`, current `main-v2` `c202f970`; Kun `master` `8602476`, current `develop` `247076f` | closed for scoped code/doc slice; Go implementation pending | Codex | Absorbed Reasonix interface segregation as analytix-owned `RemoteEntryControlPort` plus type/runtime oracle tests. Remote/bot-like entries get lifecycle, turn control, approvals, and user-input only; goal, checkpoint, memory, raw storage, thread service, tool host, and Reasonix public protocol remain excluded. Kun `d09d52b` moved to the following P0 Windows installer absorption row. |
| 2026-06-20 P0 product-entry correction + Kun Windows installer process-stop | Kun / Reasonix | Kun `master` `8602476`; Kun `develop` `247076f`; Kun `v0.2.13` `201a146`; Kun `v0.2.14` `06be05d`; Reasonix requested `c202f970`, historical observed `5d1ad2a`, current recheck `49c14762` | closed for code/doc/static-validation slice; Windows machine QA pending | Codex | Corrected the P1.3 product-position error by removing/hiding the analytix top-level Workflow route/sidebar/workbench entry while retaining Create Loop internals as unexposed future capability; absorbed Kun `d09d52b0` as analytix-native `build/installer.nsh` plus NSIS include to stop old bundled processes under `$INSTDIR` before Windows upgrade; classified Kun speech/SSE/UI drift and Reasonix SessionAPI app-port drift as next-batch records only. |
| 2026-06-21 long-term absorption methodology + cache/provider proof closure | Kun / Reasonix | Kun `master` `8602476`; Kun `develop` `247076f`; Kun `v0.2.13` `201a146`; Kun `v0.2.14` `06be05d`; Reasonix `main-v2` `49c14762` | docs/spec governance plus fixture-backed P2.2 proof update; live provider QA pending | Codex | Hardened the permanent rules and added fixture-backed provider/cache evidence: stable prefix hash, canonical tool schema hash, provider/model/endpoint attribution, DeepSeek/Responses/Anthropic cache parsing, and unsupported-provider no-guess fallback are covered by `packages/runtime/tests/provider-cache-proof.test.ts`; future `/goal` work should close whole stages using the stage-closure playbook instead of returning after each small item. |
| 2026-06-21 Reasonix agent-kernel five-batch governance closure | Reasonix / Kun | Reasonix `main-v2` `49c14762`; Kun baseline unchanged | docs/spec governance closure; runtime implementation pending | Codex | Closed the missing 019ee5ed-stage governance gap by making Reasonix agent-kernel absorption a five-batch route: task closure and permission kernel first, then long-running AutoResearch state, tool/context economy, collaborative execution, and Go runtime kernel. This adds no UI top-level entry, no Go/Rust scaffold, and no Reasonix public protocol; it sets the next implementation batch to permission/evidence/Goal rather than sub-agent/parallel or Go. |
| 2026-06-21 D-0014 Batch 1 task-closure and permission-kernel TS proof | Reasonix | Reasonix `main-v2` `49c14762` governance baseline; no new upstream drift mixed into this implementation | scoped TS runtime implementation; broader Reasonix parity pending | Codex | Landed first Batch 1 proof behind analytix contracts: `complete_step` records an append-only `ThreadGoal.evidenceLedger`; `update_goal complete` and `ThreadService.setGoal(... complete)` now require evidence; focused fixtures cover denied approval no-execute, ask/auto/yolo posture mapping, plan approval vs tool approval separation, blocked-state event/status, and headless/subagent approval policy inheritance. No Go/Rust scaffold, top-level Workflow, Reasonix protocol, or Kun identity was added. |
| 2026-06-21 D-0014 Batch 2 AutoResearch project-local state | Reasonix | Reasonix `main-v2` `49c14762` governance baseline; no new upstream drift mixed into this implementation | scoped TS runtime implementation; broader long-task parity pending | Codex | Landed `/goal --research` through the existing goal/chat surface and project-local `.analytix/autoresearch/<threadId>/` state: `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`. Requirement evidence is recorded through `complete_step requirement_id`; `record_research_direction` writes attempted directions; research goals cannot complete until every requirement has evidence. State is not written to `REASONIX.md`, `AGENTS.md`, stable system prefix, dynamic tool schema, Reasonix protocol, or a new top-level UI. |
| 2026-06-21 D-0014 Batch 3 tool-source/context economy proof | Reasonix | Reasonix `main-v2` `49c14762` governance baseline; no new upstream drift mixed into this implementation | scoped TS runtime implementation; broader tool-economy parity pending | Codex | Hardened dynamic tool-source lifecycle behind analytix contracts: `connectToolSource` / `disconnectToolSource` support dynamic provider changes, canonical tool catalog fingerprints stay stable across source connection order, tool-source diagnostics expose provider lifecycle/catalog fingerprints, and cache diagnostics report `toolSourceChanged` without putting source metadata into the stable prefix/tool schema. Existing token economy, request-history hygiene, compaction archive/history, memory retrieval, usage, and provider-cache fixtures remain green. |
| 2026-06-21 D-0014 Batch 4 delegated collaboration evidence proof | Reasonix | Reasonix `main-v2` `49c14762` governance baseline; no new upstream drift mixed into this implementation | scoped TS runtime implementation; broader collaborative-execution parity pending | Codex | Hardened the existing `delegate_task`/child-agent substrate without adding `parallel_tasks` or a top-level Subagent UI: completed child runs can attach evidence to the parent active Goal ledger, child lifecycle events carry `evidenceLedgered` / `evidenceLedgerError`, runtime event replay and renderer projection preserve the nested metadata, and existing maxParallel queue/cancel/failure/fan-out fixtures remain green. This is not a full task/background/planner-executor product model. |
| 2026-06-21 D-0014 Batch 5 Go runtime G1 shadow scaffold | Reasonix | Reasonix `main-v2` `49c14762` governance baseline; no new upstream drift mixed into this implementation | isolated Go shadow scaffold implemented; default backend pending | Codex | Added the shared oracle fixture `packages/runtime/src/conformance/fixtures/go-g1-shadow-oracle.json` and `packages/runtime-go` as a conformance-only Go package for `/health`, `/v1/runtime/info`, and `/v1/runtime/tools`. Existing `go-runtime-kernel-conformance.ts` still maps Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime to TS oracle tests. Go tests compare status codes, auth, JSON shape, tool diagnostics, and capability registry semantics against the TS oracle. No default Go backend, Electron main integration, renderer-visible Go route, Reasonix public protocol, Rust scaffold, or Tauri path was added. |
| 2026-06-21 Reasonix latest currentness, parity hardening, and Go G2 shadow replay | Reasonix / Kun | Reasonix `main-v2` refreshed to `91fe06db`; Kun `master` `8602476`; Kun `develop` `247076f`; Kun `v0.2.13` / `v0.2.14` tags rechecked | scoped code/docs/tests closed; live provider and default backend pending | Codex | Rechecked latest Reasonix drift (`45367085` codebase-memory MCP auto-indexing and `d9e453a1` MiMo built-ins -> custom providers), classified codebase-memory as a future `wrap-behind-contract` MCP indexer-policy input, rejected deleting analytix Xiaomi/MiMo presets, and recorded MiMo legacy/custom-provider migration as a future `contract-reimplement` candidate. Added G2 shadow route replay oracle/tests for thread/session read/list/search/archive/fork/resume and SSE replay, Go shadow-only replay handler/tests, approval/user-input route oracle, provider-cache oracle, and MCP/tool lifecycle oracle. No default Go backend, renderer-visible Go route, Reasonix public protocol, or top-level Workflow entry was added. Live provider matrix remains unclosed. |
| 2026-06-21 Reasonix P0 engine parity + Kun desktop hardening | Reasonix / Kun | Reasonix `main-v2` `91fe06db`; Kun `develop` `247076f`; Kun `master` `8602476`; Kun `v0.2.13` / `v0.2.14` tags rechecked | scoped stage closed; live provider/MCP/packaged QA pending | Codex | Closed the scoped P0 stage behind analytix contracts: provider/cache parity matrix now covers DeepSeek/OpenAI-compatible/Responses/Anthropic cache and reasoning tokens; task/background/planner oracle plus durable job manager skeleton covers foreground/background/wait/output/kill/parallel dependency/nested event/permission/transcript/planner cases; MCP known overrides cover codegraph/codebase-memory cwd and lifecycle diagnostics; Go G3/G4 shadow oracles match TS fixtures; Kun SSE IPC throttle/reconnect/destroyed-renderer safety and shell-level navigation controls are absorbed. No Reasonix public protocol, Kun identity, top-level Workflow/Create Loop, default Go backend, Rust/Tauri path, or Electron-to-Go switch was added. |
| 2026-06-21 Reasonix 9e56 currentness + task-job route parity | Reasonix / Kun | Reasonix `main-v2` `9e56c327`; Kun baseline unchanged | scoped runtime/docs/tests closed; live provider/MCP/Go G5 pending | Codex | Classified `91fe06db..9e56c327`: todo clear/archive commits are record-only; lazy MCP removal is `wrap-behind-contract`; retry-all and late-provider review feedback are `contract-reimplement`. Added MCP retry-all failed-server proof, registry source suspension/resume, internal authenticated `/v1/runtime/task-jobs/wait|output|kill` routes, bounded job wait, and provider probe secret-safe diagnostics. Dirty shell safe-area baseline was reviewed and committed separately. No Reasonix public protocol, Kun identity, top-level Workflow/Create Loop, default Go backend, Rust/Tauri path, or Electron-to-Go switch was added. |
| 2026-06-21 collaborative execution/cache/MCP/Go G5 oracle closure | Reasonix / Kun | Reasonix `main-v2` `9e56c327`; Kun `v0.2.13` `201a146`, `v0.2.14` `06be05d`, `master` `8602476`, `develop` `247076f` | scoped oracle/runtime proof closed; live parity/release pending | Codex | Closed the next fixture/oracle layer after dirty shell safe-area: internal `task` / `parallel_tasks` contracts now pin permission gate, parent Goal evidence, background, transcript continue/fork, and `depends_on`; parallel validation rejects duplicate/self/unknown/cycle cases; provider-cache proof adds an offline Reasonix-style cache curve guard; MCP lifecycle oracle covers retry-all failed startup servers, late suspended provider tombstones, and codegraph/codebase-memory cwd/priority/backgroundStart; Go G5 full-loop oracle inventory is TS-owned and shadow-only. Kun desktop delta remains within the 0.2.13 -> 0.2.14 entry baseline; tray session menu and Local Whisper are still future import-boundary/QA items. No Reasonix public protocol, Kun identity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, default Go backend, Rust/Tauri path, or Electron-to-Go switch was added. |
| 2026-06-21 Reasonix planner/executor, Windows shell, live-local provider/MCP, and Go G5 runner shadow | Reasonix / Kun | Reasonix `main-v2` `bfe398cc`; Kun `v0.2.13` `201a146`, `v0.2.14` `06be05d`, `master` `8602476`, `develop` `247076f` | scoped runtime/docs/tests closed; full live parity/release pending | Codex | Upgraded the prior oracle layer into analytix-owned executable evidence: added a planner-readonly/executor coordinator proof with dependency ordering, failed-dependency skips, cancellation, output offsets, parent metadata, and read-only planner toolset; added durable runner rehydration for queued/running jobs after restart with output/wait/kill evidence; absorbed Reasonix bfe398 PowerShell 7 standard-path and pwsh chaining semantics as runtime shell fixtures; added a live-local DeepSeek-compatible provider server proof; replaced in-memory MCP indexer proof with an executable stdio fake server covering restart/resume/tombstone/cwd/low-priority/backgroundStart/redaction; and extended Go G5 composite shadow replay with planner/executor and durable-runner restart fields. Kun remains only the 0.2.13 -> 0.2.14 product-entry baseline; Local Whisper/tray/package QA stay gated. No Reasonix public protocol, Kun identity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, default Go backend, Rust/Tauri path, or Electron-to-Go switch was added. |
| 2026-06-21 Reasonix post-881 auto-router guard + Go G5 executable control + Kun context window | Reasonix / Kun | Reasonix `881b2f2f..9ada1417`; Kun `v0.2.14` context-window delta rechecked | scoped code/docs/tests closed; live parity/release pending | Codex | Classified post-881 Reasonix auto-plan user-level and classifier-rebuild commits without importing Reasonix config/CLI/protocol. Absorbed the classifier lifecycle value as an analytix-owned auto-model-router contract fingerprint so same-turn route cache keys drift when the classifier prompt/model/timeout contract changes. Advanced Go G5 from `controlReplay` passthrough to fixture-owned executable control cases for cancel, task-job cancellation, and step-limit resolution while keeping Go shadow-only. Corrected the shared provider unknown-model default context window from `24_000` to `128_000`, matching Kun 0.2.14/runtime defaults, with explicit profile overrides preserved. No post-881 auto-plan parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, default Go backend, Reasonix public protocol, Kun identity, Rust/Tauri path, or release readiness is claimed. |
| 2026-06-22 Go G5 user-input gate executable shadow | Reasonix / Kun | Reasonix post-881 approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for submitted/cancelled user-input gates: TS conformance binds the case to `approval-user-input-route-api-gates-v1`, Go shadow computes submitted answer echo, replay answer omission, cancelled status, late resolve rejection, and pending-after counts. No Reasonix ask protocol, Go user-input route, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or top-level hidden capability entry was added. |
| 2026-06-22 Go G5 AutoResearch project-local state shadow | Reasonix / Kun | Reasonix long-task/AutoResearch state review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for AutoResearch project-local state: TS conformance creates real `.analytix/autoresearch/<threadId>/` state, verifies required files, rejects unknown requirement evidence without findings writes, and Go shadow computes no `REASONIX.md`/`AGENTS.md`, no stable-prefix/tool-schema pollution, and no top-level AutoResearch route. No Reasonix project protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or top-level hidden capability entry was added. |
| 2026-06-22 Go G5 MCP lifecycle executable shadow | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live MCP/Go parity pending | Codex | Added G5 executable shadow proof for MCP lifecycle/indexer behavior: TS conformance derives retry attempts, connected/error ids, tombstone/resume active paths, and redaction from `mcp-tool-lifecycle-oracle.json`; Go shadow computes the same output without a live MCP client. No Reasonix MCP protocol, MCP-indexer top-level route, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 Go G5 checkpoint/rewind executable shadow | Reasonix / Kun | Reasonix checkpoint/rewind engine review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for checkpoint/rewind safety: TS conformance derives ready/blocked files and conversation audit from the existing analytix checkpoint oracle, while Go shadow computes `axcp_`/`axrp_`/`axra_`/`axrr_` prefix boundaries, path escape blocking, symlink blocking, legal `..name` handling, explicit confirmation, append-only audit, no git refs, and no public route. No Reasonix checkpoint protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or top-level hidden capability entry was added. |
| 2026-06-22 Go G5 remote-entry boundary executable shadow | Reasonix / Kun | Reasonix SessionAPI/control-port review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for the remote-entry boundary: TS conformance derives allowed `approvals`/`lifecycle`/`turns` ports and forbidden goal/checkpoint/memory/storage/tool-host keys from `approval-user-input-route-oracle.json`; Go shadow computes forbidden control planes absent, policy override rejected, no Reasonix protocol, and no top-level route. No Reasonix SessionAPI, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 Go G5 resume pending gates executable shadow | Reasonix / Kun | Reasonix approval/user-input resume lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for pending approval/user-input gates across session resume: TS conformance derives source pending and resumed expired/cancelled statuses from `approval-user-input-route-oracle.json`; Go shadow computes no pending gates after resume, no answers copied, no Reasonix protocol, and no top-level route. No Reasonix ask/session protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 D-0074 Go G5 approval/user-input abort cleanup executable shadow | Reasonix / Kun | Reasonix post-881 approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 executable shadow proof for abort cleanup: TS conformance derives expired approval, cancelled user-input, late approval decision `409`, late user-input resolve `404`, pending-after cleanup `0`, and replay event kinds from `approval-user-input-route-oracle.json`; Go shadow computes the same output without a live approval/user-input manager. No Reasonix ask/session protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 D-0075 Go G5 abort cleanup replay summary closure | Reasonix / Kun | Reasonix post-881 approval/user-input lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Closed the replay-summary side of D-0074: G5 `controlReplay.abortCleanup` and `approvalUserInputReplay` now carry expired/cancelled statuses, late `409`/`404`, no pending gates after cleanup, and replay kinds; Go `BuildG5ShadowSlicesOutput` computes those fields from the TS approval/user-input oracle. No live Go gate manager, Reasonix protocol, default backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 D-0076 Go G5 MCP search meta-tool replay | Reasonix / Kun | Reasonix MCP/indexer lifecycle review; Kun baseline unchanged | scoped shadow/docs/tests closed; live MCP/Go parity pending | Codex | Promoted the existing MCP search meta-tool trust/no-execute oracle into G5 `mcpReplay`: TS conformance and Go shadow now carry meta-tool names, trusted tool id, untrusted searched count `0`, unknown-tool error, `on-request` call policy, and denied no-execute. No live Go MCP client, MCP-indexer top-level entry, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 D-0077 Go G5 parallel task dependency validation replay | Reasonix / Kun | Reasonix sub-agent/job orchestration review; Kun baseline unchanged | scoped shadow/docs/tests closed; live Go parity/release pending | Codex | Added G5 `jobReplay.parallelValidation` for `parallel_tasks`: TS conformance and Go shadow now carry `depends_on`, valid order, and single-task/duplicate-id/self-dependency/cycle/unknown-dependency error strings from the task-job oracle. No live Go Job Manager, public sub-agent/job protocol, top-level Subagent/Workflow/Create Loop route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 Go G5 provider cache release guard executable shadow | Reasonix / Kun | Reasonix cache curve guard review; Kun baseline unchanged | scoped shadow/docs/tests closed; live provider/Go parity pending | Codex | Added G5 executable shadow proof for the provider cache release guard: TS conformance derives expected output from `evaluateOfflineCacheCurveGuard`, while Go shadow computes tail averages, collapse counts, low-tail allowance, and overall pass/fail from `provider-cache-oracle.json`. No live provider superiority claim, Reasonix provider protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry was added. |
| 2026-06-22 D-0064 auto-route step/cancel AgentLoop proof | Reasonix / Kun | Reasonix post-881 agent-kernel composition review; Kun baseline unchanged | scoped test/docs closed; product auto-plan parity deferred | Codex | Added a real TypeScript `AgentLoop` test proving `model:"auto"` routes once, reuses the selected model across a second step, keeps step-limit metadata out of model-visible prefix/context, and preserves completed/cancelled parallel tool results when the turn is interrupted. No Reasonix auto-plan setting, controller API, public protocol, default Go backend, Kun identity, Rust/Tauri path, or hidden top-level entry was added. |
| 2026-06-22 D-0065 planner step-limit AgentLoop proof | Reasonix / Kun | Reasonix planner gating/currentness review; Kun baseline unchanged | scoped test/docs closed; product planner controls deferred | Codex | Added a real TypeScript `AgentLoop` plan-mode test proving `plannerMaxModelSteps` gates plan turns independently of the higher default/user-global limit, advertises the plan tool, records the planner step-limit failure, and keeps planner budget state out of stable prefix/model-visible context. No Reasonix auto-plan setting, planner product toggle, controller API, default Go backend, Kun identity, Rust/Tauri path, or hidden top-level entry was added. |
| 2026-06-21 approval/user-input abort cleanup | Reasonix | Reasonix approvalManager/control drift previously deferred under D-0011/D-0014; no new Kun delta | scoped runtime/oracle proof closed; broader desktop/live parity pending | Codex | Reimplemented the useful approval-manager cleanup value behind analytix gates: `ApprovalGate.expire()` clears pending approvals on turn abort, rejects late allow/deny, emits `approval_resolved: expired`, and preserves cancelled tool-result pairing. `request_user_input` abort now records `user_input_resolved: cancelled` so SSE replay is closed. Route oracle fixes late approval to 409 and late user-input resolve to 404; renderer mapping updates existing approval cards on `approval_resolved`. No Reasonix SessionAPI, public protocol, renderer route, or top-level Workflow/Subagent entry was added. |
| 2026-06-21 structured user-input choice validation | Reasonix | Reasonix approval/user-input/control hardening; no new Kun delta | scoped runtime/G4 shadow proof closed; live desktop QA pending | Codex | Reimplemented the useful GUI input contract value behind analytix `request_user_input`: structured choices now reject more than three questions, one-option choice sets, more than three options, and duplicate labels before opening a user-input gate. G4 oracle and Go shadow output record `invalid_user_input_request`, invalid cases, and `opensGateOnInvalid:false`. No Reasonix ask protocol, bridge/settings/provider change, renderer route, top-level Workflow/Subagent entry, default Go backend, or Rust/Tauri path was added. |
| 2026-06-21 write-inline custom full endpoint proof | Reasonix | Reasonix provider/cache request-surface discipline; no new Kun delta | scoped test/docs proof closed; live provider matrix pending | Codex | Extended write-inline provider proof to custom full endpoint URLs ending in `/responses` and `/messages`: the exact URL is used without appended paths, Responses uses `input` / `max_output_tokens` without Anthropic headers, and Messages uses `system` / `messages` / `max_tokens` with Anthropic-style headers and parser. No Reasonix provider protocol, settings schema change, provider default change, bridge, top-level Workflow/Subagent entry, default Go backend, or Rust/Tauri path was added. |
| 2026-06-21 auto-model route cache lifecycle proof | Reasonix | Reasonix post-881 classifier rebuild/currentness drift; no new Kun delta | scoped runtime test/docs proof closed; auto-plan product surface deferred | Codex | Extended the auto-router fingerprint/cache-key absorption with a real `AgentLoop` multi-step turn proof: `_auto_router` runs once for a `model:"auto"` turn that continues after a tool call, the second step reuses the selected model/reasoning route, and the next turn reruns the classifier. No Reasonix auto-plan setting, local/project config, controller API, stable-prefix mutation, top-level Workflow/Subagent entry, default Go backend, or Rust/Tauri path was added. |
| 2026-06-21 provider request-shape oracle matrix | Reasonix | Reasonix provider/cache request-surface discipline; no new Kun delta | scoped oracle/test/docs proof closed; live provider matrix pending | Codex | Added `provider-cache-oracle.json.requestShapeCases` and executable `CompatModelClient` proof for DeepSeek official chat, OpenAI-compatible chat, Responses, Anthropic Messages, and custom Responses full endpoint request URL/header/body shapes. G3/G5 shadow outputs now carry request-shape case ids only. No Reasonix provider protocol, settings schema/default change, bridge, top-level Workflow/Subagent entry, default Go backend, Go provider client, or Rust/Tauri path was added. |

## Item Tracker

| Item | Upstream | Area | Class | Decision | Target analytix layer | Status | Validation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Go G5 user-input structured validation control shadow | Reasonix / Kun | Go G5 / user input / structured choices / approval privacy | contract-reimplement / code-port-and-adapt / reject / defer | Carry G4 `request_user_input` structured choice validation into `controlExecutableCases.userInput.structuredChoiceValidation`; prove too-many questions, single option, too-many options, duplicate labels, invalid code, no pending gate on invalid, no Reasonix protocol, and no top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go gate manager deferred | `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 context compaction boundary control shadow | Reasonix / Kun | Go G5 / context compaction / model-history boundary / cache stability | contract-reimplement / code-port-and-adapt / reject / defer | Carry latest-compaction effective-history semantics into `controlExecutableCases.compactionBoundary`; prove latest positive compaction becomes the boundary, noop/older/pre-boundary items are dropped, post-boundary items remain, compaction state stays out of stable prefix, and no Reasonix protocol/top-level route is exposed. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go loop deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 history repair pair-integrity control shadow | Reasonix / Kun | Go G5 / model history repair / tool-call-result pairing / cache stability | contract-reimplement / code-port-and-adapt / reject / defer | Carry `repairModelHistoryItems` pair-integrity semantics into `controlExecutableCases.historyRepair`; prove complete multi-tool blocks survive, orphan results/missing calls/duplicate results are dropped, bridge text is preserved, repair state stays out of stable prefix, and no Reasonix protocol/top-level route is exposed. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go loop deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 step-limit override/delegate matrix control shadow | Reasonix / Kun | Go G5 / agent loop controls / step limit / planner delegation | contract-reimplement / code-port-and-adapt / reject / defer | Carry step-limit override/delegate matrix into `controlExecutableCases.stepLimits.expected`; prove effective values for default/user/session/turn/planner/headless/zero/delegate rows, stable-prefix isolation, zero guard disable, and delegate min-floor without Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go loop deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 combined step/cancel/cache trace control shadow | Reasonix / Kun | Go G5 / agent loop controls / auto-route cache / step limit / cancel | contract-reimplement / code-port-and-adapt / reject / defer | Carry combined step/cancel/cache trace into `controlExecutableCases.combined.expected`; prove same-turn route cache reuse through step limit, next-turn reroute, stable-prefix isolation, cancel result counts, and exact result rows without Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go loop deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G3/G5 provider request-shape exact matrix control shadow | Reasonix / Kun | Go G3/G5 / provider cache / request URL-body-header shape | contract-reimplement / code-port-and-adapt / reject / defer | Carry provider request-shape exact matrix into G3/G5 shadow outputs; prove exact URL, required/forbidden headers, required/forbidden body fields, reasoning-effort presence, and tool-shape family for 7 TS-owned cases without Reasonix protocol or default Go backend. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live provider matrix deferred | `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `packages/runtime-go go test -count=1 ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP core lifecycle boundary seal control shadow | Reasonix / Kun | Go G5 / MCP lifecycle / provider identity / product boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry MCP core lifecycle provider identity into `controlExecutableCases.mcpCoreLifecycle`, computed by Go shadow from TS-owned MCP oracle-derived input; prove `mcp:research`, no Reasonix protocol, and no top-level route exposure alongside lifecycle replay. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task planner toolset inventory control shadow | Reasonix / Kun | Go G5 / task jobs / planner gating / sub-agent orchestration | contract-reimplement / code-port-and-adapt / reject / defer | Carry task/sub-agent planner toolset inventory into `controlExecutableCases.taskJobs.plannerToolsetInventory`, computed by Go shadow from TS-owned task-job oracle-derived input; prove read-only tools, forbidden `task`/`parallel_tasks`, counts, no overlap, policy values, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 approval/user-input inventory control shadow | Reasonix / Kun | Go G5 / approval gate / user-input gate / replay inventory / answer privacy | contract-reimplement / code-port-and-adapt / reject / defer | Carry approval/user-input gate inventory into `controlExecutableCases.approvalUserInputInventory`, computed by Go shadow from TS-owned oracle-derived input; prove gate ids, approval/user-input id grouping, route kinds, replay kind order, answer count, HTTP answer echo vs resolved-event no-answer privacy, late statuses, pending-after sum, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; packaged approval-card QA deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 session route inventory control shadow | Reasonix / Kun | Go G5 / thread session routes / SSE replay / route surface inventory | contract-reimplement / code-port-and-adapt / reject / defer | Carry thread/session route inventory into `controlExecutableCases.sessionRouteInventory`, computed by Go shadow from the TS-owned G2 route oracle; prove route ids, JSON/SSE/event/resume/fork/archive/search/read-update grouping, runtime-token route count, unauthorized route id, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; packaged route QA deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 provider cache inventory control shadow | Reasonix / Kun | Go G5 / provider cache / prefix and tool inventory / currentness boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry provider cache prefix/tool/case-id inventory into `controlExecutableCases.providerCacheInventory`, computed by Go shadow from the TS-owned provider-cache oracle; prove stable prefix hash, canonical tools hash, provider usage/request-shape ids and counts, prefix equivalence, tools hash stability, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live provider matrix deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 provider cache privacy control shadow | Reasonix / Kun | Go G5 / provider cache / diagnostics privacy / live-superiority boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry provider cache diagnostics privacy into `controlExecutableCases.providerCachePrivacy`, computed by Go shadow from the TS-owned provider-cache oracle; prove bounded diagnostics, no forbidden substring leak, no live credentials, no live superiority claim, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live provider matrix deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP search meta-tool control shadow | Reasonix / Kun | Go G5 / MCP lifecycle / search meta-tools / trust boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry MCP search meta-tool advertised/trust/no-execute behavior into `controlExecutableCases.mcpSearchMetaTools`, computed by Go shadow from the TS-owned MCP lifecycle oracle; prove meta-tool set, refresh advertisement, workspace trust, unknown-tool error, `on-request` policy, denied no-execute, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP live-local indexer control shadow | Reasonix / Kun | Go G5 / MCP lifecycle / live-local indexer / redaction boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry MCP live-local indexer lifecycle into `controlExecutableCases.mcpLiveLocalIndexer`, computed by Go shadow from the TS-owned MCP lifecycle oracle; prove retry ids/map, initial/resume/active paths, tombstone/restart, late tombstone, diagnostic and execution-error redaction, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP known override diagnostics control shadow | Reasonix / Kun | Go G5 / MCP lifecycle / known override diagnostics / config boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry MCP known override diagnostics into `controlExecutableCases.mcpKnownOverrideDiagnostics`, computed by Go shadow from the TS-owned MCP lifecycle oracle; prove override kinds, effective cwd, workspace roots, explicit-cwd and daemon-timeout server ids, low-priority/background-start flags, and no Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 nested child SSE metadata control shadow | Reasonix / Kun | Go G5 / task jobs / child SSE metadata / evidence ledger | contract-reimplement / code-port-and-adapt / reject / defer | Carry nested child SSE metadata into `controlExecutableCases.taskJobs.nestedSseMetadata`, computed by Go shadow from the TS-owned task-job oracle; prove parent/child ids and evidence ledger metadata keys survive for analytix projection without Reasonix protocol/top-level route exposure. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task transcript identity control shadow | Reasonix / Kun | Go G5 / task jobs / transcript continue-fork identity | contract-reimplement / code-port-and-adapt / reject / defer | Carry transcript continue/fork identity into `controlExecutableCases.taskJobs.transcriptIdentity`, computed by Go shadow from the TS-owned task-job oracle; prove continue target stays the source, fork target is distinct, incompatible identity has a deterministic error, and no Reasonix protocol/top-level route is exposed. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task-job tool contract boundary control shadow | Reasonix / Kun | Go G5 / task jobs / sub-agent orchestration / route boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry `task`/`parallel_tasks` tool contract and task-job route boundary into `controlExecutableCases.taskJobs.toolContractBoundary`, computed by Go shadow from the TS-owned task-job oracle; prove internal-only tools, permission/dependency/read-only gates, runtime route auth, forbidden top-level routes, and no Reasonix protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 product boundary control shadow | Reasonix / Kun | Go G5 / product sovereignty / backend boundary | contract-reimplement / code-port-and-adapt / reject / defer | Carry global product-boundary invariants into `controlExecutableCases.productBoundary`, computed by Go shadow from the TS-owned G5 oracle; prove no Reasonix public protocol, default Go backend, renderer-visible Go route, or bridge/serve contract drift. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go backend switch rejected | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP background reconnect control shadow | Reasonix | Go G5 / MCP lifecycle / reconnect / retry | code-port-and-adapt / contract-reimplement / reject / defer | Carry MCP failed-server reconnect evidence into `controlExecutableCases.mcpBackgroundReconnect`, computed by Go shadow from the TS-owned MCP lifecycle oracle; keep MCP reconnect internal and reject Reasonix MCP-indexer public protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP core lifecycle control shadow | Reasonix | Go G5 / MCP lifecycle / catalog / cancel / error | code-port-and-adapt / contract-reimplement / reject / defer | Carry MCP connect/disconnect/reload/cancel/error core lifecycle into `controlExecutableCases.mcpCoreLifecycle`, computed by Go shadow from a TS-owned MCP lifecycle oracle; keep MCP behavior internal and reject Reasonix MCP-indexer public protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP search workspace boundary control shadow | Reasonix | Go G5 / MCP lifecycle / workspace trust / no-execute | code-port-and-adapt / contract-reimplement / reject / defer | Carry trusted/untrusted MCP search workspace boundary into `controlExecutableCases.mcpSearchWorkspaceBoundary`, computed by Go shadow from a TS-owned MCP lifecycle oracle; keep MCP search internal and reject Reasonix MCP-indexer public protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP approval annotation control shadow | Reasonix | Go G5 / MCP lifecycle / tool approval / no-execute | code-port-and-adapt / contract-reimplement / reject / defer | Carry destructive/open-world MCP approval annotations into `controlExecutableCases.mcpApprovalAnnotations`, computed by Go shadow from a TS-owned MCP lifecycle oracle; keep MCP approval behavior internal and reject Reasonix approval/MCP-indexer public protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP/approval managers deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP search refresh drift control shadow | Reasonix | Go G5 / MCP lifecycle / catalog refresh / indexer boundary | code-port-and-adapt / contract-reimplement / reject / defer | Carry MCP search catalog refresh drift into `controlExecutableCases.mcpSearchRefreshDrift`, computed by Go shadow from a TS-owned minimal MCP lifecycle oracle; keep MCP tools internal and reject Reasonix MCP-indexer public protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 approval/user-input route replay control shadow | Reasonix | Go G5 / approval gate / user-input gate / SSE replay | code-port-and-adapt / contract-reimplement / reject / defer | Carry approval deny, user-input submit/cancel, replay-order, late-action, and abort-cleanup route semantics into `controlExecutableCases.approvalUserInputRouteReplay`, computed by Go shadow from a TS-owned minimal oracle; keep TypeScript runtime authority and reject Reasonix public ask/session protocol. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval/user-input managers deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Offline provider/cache parity seal | Reasonix | provider/cache / DeepSeek prefix / multi-provider regression | contract-reimplement / code-port-and-adapt / defer | Carry fixture-only provider/cache parity evidence into G5 `cacheReplay.offlineParitySeal`, computed by Go shadow from the TS provider-cache oracle; keep live provider superiority claims disabled. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live credential matrix deferred | `provider-cache-proof.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 approval decision route replay | Reasonix | Go G5 / approval gate / SSE replay | contract-reimplement / code-port-and-adapt / defer | Carry approval decision route and replay-order evidence into G5 `approvalUserInputReplay`, computed by Go shadow from the TS approval/user-input route oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval manager deferred | `approval-user-input-route-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP core lifecycle replay | Reasonix | Go G5 / MCP lifecycle / tool source state | code-port-and-adapt / contract-reimplement / defer | Carry MCP connect/disconnect/reload/cancel/error evidence into G5 `mcpReplay.lifecycle`, computed by Go shadow from the TS MCP lifecycle oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `mcp-tool-lifecycle-oracle.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task job route boundary replay | Reasonix | Go G5 / task jobs / route auth / product boundary | contract-reimplement / code-port-and-adapt / defer | Carry task-job route auth and forbidden top-level route evidence into G5 `jobReplay.routeBoundary`, computed by Go shadow from the TS task-job oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go task-job routes deferred | `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task job lifecycle replay | Reasonix | Go G5 / task jobs / wait-output-kill | code-port-and-adapt / contract-reimplement / defer | Carry base task job lifecycle evidence into G5 `jobReplay.lifecycle`, computed by Go shadow from the TS task-job oracle: foreground completion, background cross-turn output/final completion, and killed wait/output status/error. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 approval/user-input abort cleanup executable shadow | Reasonix | Go G5 / approval gate / user-input gate / SSE replay | code-port-and-adapt / contract-reimplement / defer | Carry the TS-owned abort-cleanup oracle into Go G5 `controlExecutableCases.abortCleanup`, computing expired approval, cancelled user-input, late GUI action statuses, no pending gates, and replay event order while keeping TypeScript runtime authority. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval/user-input managers deferred | `approval-user-input-route-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 abort cleanup replay summary closure | Reasonix | Go G5 / approval-user-input replay / SSE replay | contract-reimplement / code-port-and-adapt / defer | Add abort-cleanup fields to G5 `controlReplay` and `approvalUserInputReplay`, and compute them in Go shadow slices from the TS approval/user-input route oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval/user-input managers deferred | `approval-user-input-route-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 MCP search meta-tool replay | Reasonix | Go G5 / MCP lifecycle / tool trust | code-port-and-adapt / contract-reimplement / defer | Carry MCP search meta-tool trust/no-execute evidence into G5 `mcpReplay`, computed by Go shadow from the TS MCP lifecycle oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go MCP client deferred | `mcp-tool-lifecycle-oracle.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 parallel task dependency validation replay | Reasonix | Go G5 / task jobs / sub-agent orchestration | code-port-and-adapt / contract-reimplement / defer | Carry `parallel_tasks` dependency validation into G5 `jobReplay`, computed by Go shadow from the TS task-job oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Kun product/runtime delta map | Kun | UI / runtime / provider / QA | must / should / reject | Absorb product workflows and runtime deltas through analytix contracts; reject old identity and bridge/settings. | renderer / shared / packages/runtime / docs | planned for implementation | `kun-sync.md`, `absorption-targets.md`, `code-level-absorption-blueprint.md` |
| Kun master 8602476 stable drift | Kun | provider / Connect Phone / workflow / desktop utility / safety | must / should / redesign / reject / already covered | Treat previous develop audit as stable-master drift; split into P1.1/P1.2/P1.3/P3 instead of merging whole Kun UI. | renderer / main / shared / packages/runtime / docs | planned for implementation | `kun-sync.md`, `code-level-implementation-plan.md` |
| Kun P1.1 stable critical provider/runtime fixes | Kun | provider / runtime / preload | must / already covered | Absorb `6bd1edc`, `5772400`, `694fc4e`, `b14e31f`, and `dd3252d` as analytix-native provider presets, `thread.providerId` routing, compaction tail repair, and preload guard tests. | shared / renderer / main / preload / packages/runtime / docs | closed for code slice | `packages/runtime/tests/hybrid-store.test.ts` covers SQLite list `providerId`, index deletion sidecar rebuild, and damaged `messages.jsonl` sidecar preservation; contract/domain/thread-service/model-client/context-compactor tests, preload/settings/renderer tests, typechecks, and `git diff --check` pass |
| Kun P1.2 Connect Phone / Telegram runtime | Kun | Connect Phone / IM / runtime relay | should absorb | Absorb Kun `a502a6d` Telegram runtime behavior, but land it as Connect Phone: `window.analytix.connectPhone.connectTelegramBot`, top-level settings, local Telegram runtime managed by `ClawRuntime`, and analytix HTTP/SSE turn contracts. Reject Kun identity, old bridge aliases, old settings envelopes, external relay-only guidance, and runtime-control UI. | shared / preload / main / renderer / docs | closed for code slice | Focused tests cover Telegram token schema/verification, settings normalization, Connect Phone UI helpers, IPC handler, inbound Telegram text/image relay with attachment upload, IM approval/user-input disablement, provider/model propagation, local GUI-to-Telegram mirror, and schedule provider labels; P1.1 regression tests, typechecks, and `git diff --check` pass. |
| Kun P1.3 Workflow / Create Loop | Kun | workflow automation / product UI | redesign / corrected | Retain only the underlying analytix-native Create Loop state machine/tests as unexposed future capability. The top-level Workflow route/sidebar/workbench entry was a product-position error for the Kun 0.2.13/0.2.14 baseline and is removed/hidden unless a future spec explicitly approves a changed entry level. | renderer / docs | corrected and validated | `src/renderer/src/store/chat-store-types.ts`, `src/renderer/src/components/chat/Sidebar.tsx`, and `src/renderer/src/components/Workbench.tsx` no longer expose `workflow`; `WorkflowCreateLoopView` and `create-loop-runtime` remain isolated with tests. |
| Reasonix main-v2 P1.1 preflight delta | Reasonix | desktop migration performance | should | Defer `ad3d742` per-directory legacy topic migration marker from P1.1; revisit only with an equivalent analytix migration pass. | docs / future migration code | deferred follow-up | `reasonix-sync.md` preflight entry |
| Reasonix main-v2 P1.3 workflow review | Reasonix | agent/runtime orchestration | should / defer | Reviewed `be67a498adcaed6e33dcf3cbd25395e9cbccd6fa` for agent loop, controller, approvals, user input, jobs, events, memory, session persistence, provider/cache/context, MCP/plugin, and desktop topic migration. No P1.3 code was absorbed because the relevant value is engine primitives, not a product workflow builder; defer to P2.2 provider/cache/tool lifecycle, P3 history/checkpoint/session migration, and Go runtime conformance. | docs / future runtime code | deferred follow-up | `reasonix-sync.md` P1.3 entry and sub-agent read-only audit |
| Reasonix P2.2 cache prefix diagnostics | Reasonix | provider / cache / tool lifecycle | should absorb | Adapt Reasonix `cache_shape` diagnostics into analytix runtime events without exposing Reasonix protocols: capture stable system/mode/prefix/tool/provider/model hashes, canonicalize tool schemas, attach prefix-change reasons and provider cache telemetry support to usage events, parse cache hit/miss tokens where providers report them, leave unsupported providers unknown instead of guessing misses, and keep prompt text/tool descriptions/tool args/output/secrets out of diagnostics. Defer full provider rewrite, MCP/plugin lifecycle, checkpoints, stream reconnect protocol changes, and Go runtime work. | packages/runtime / renderer contract / docs | closed for scoped code slice plus fixture-backed proof; live provider QA pending | `packages/runtime/tests/provider-cache-proof.test.ts`, focused cache/usage tests, P1.3 and P1.2 regressions, runtime package tests, runtime and app typechecks, `git diff --check`, and identity/protocol scan |
| Reasonix P3A MCP malformed schema guard | Reasonix | MCP / tool catalog / provider safety | should absorb | Normalize malformed MCP `inputSchema` values before they enter the analytix registry or model tool catalog; preserve valid object schemas and use a bounded object fallback for invalid ones. | packages/runtime | closed for scoped code slice | `packages/runtime/tests/mcp-tool-provider.test.ts` focused fixture; future Go G4 oracle |
| Reasonix P3A denied approval no-execute proof | Reasonix | approval / tool safety | should absorb | Pin analytix GUI approval safety: a denied approval returns an approval item and never calls the tool implementation. | packages/runtime | closed for scoped code slice | `packages/runtime/tests/builtin-tools.test.ts` focused fixture; future Go G4 oracle |
| Kun P3A baseline fidelity audit | Kun | Code / Write / SDD / Connect Phone / Schedule / provider / MCP / media / workflow / GUI commands | baseline regression / 0.2.14+ delta / already preserved / reject / redesign / defer | Treat Kun as lineage: verify 0.2.13 baseline fidelity, preserve closed P1.1/P1.2/P1.3 scoped slices, classify 8602476 checkpoint/tray/workflow deltas without expanding P3A. | docs / regression suite | closed for audit slice | `kun-sync.md`; closure suite includes P1.1/P1.2/P1.3 regressions and identity scan |
| P4A analytix checkpoint metadata oracle | Reasonix / Kun | checkpoint / review safety / event replay | should absorb | Create versioned analytix-owned checkpoint metadata with `axcp_` ids, workspace, turnId, changed files, createdAt, and status. Reject Kun `refs/kun/checkpoints`, direct `git reset --hard`, Kun bridge/UI/settings, and Reasonix CLI/settings/protocol. | packages/runtime / docs | closed for scoped code slice | `packages/runtime/tests/checkpoint-rewind-oracle.test.ts`; future Go G5 oracle |
| P4A conversation-only rewind oracle | Reasonix | runtime event projection / history | should absorb | Plan a non-mutating rewind from durable analytix events by retaining events before the target turn boundary, projecting the safe transcript, and reporting removed turn ids. Defer combined file restore and review UI controls to P4B/P4C. | packages/runtime / docs | closed for scoped code slice | `packages/runtime/tests/checkpoint-rewind-oracle.test.ts`; no file rewrite or git refs in P4A |
| P4B auditable rewind restore plan | Reasonix / Kun | checkpoint / review safety / UI boundary | should absorb | Add `axrp_` plan-only restore/rewind contract, read-only runtime service/route, shared endpoint and IPC allow-list, renderer provider method, and existing `file_change`/ChangeInspector/TurnChangeSummary display. Block path escape, absolute persisted paths, and symlink risks; do not apply files or rewrite events. | packages/runtime / shared / main / renderer / docs | closed for scoped code slice | `packages/runtime/tests/checkpoint-rewind-plan.test.ts`, P4A fixture, IPC/provider/UI projection tests; future Go G5 oracle |
| P4C confirmed destructive rewind apply | Reasonix / Kun | checkpoint / restore apply / crash recovery / UI confirmation | should absorb | Execute only a reviewed P4B `CheckpointRewindPlan` after explicit confirmation. Create an analytix-owned `axrr_` rescue record before file mutation, apply created/modified/deleted/noop actions only when current hashes and snapshot evidence prove safety, revalidate snapshot content hashes and symlink/git safety, and record `checkpoint_rewind_applied` as append-only audit for code/conversation/combined scopes. Reject git reset/checkout, Kun refs, Reasonix protocols, and silent transcript rewrite. | packages/runtime / shared / main / renderer / docs | closed for scoped code slice with desktop smoke; release/live apply fixture remains gated; Spec 07 release/push blockers still apply | `packages/runtime/tests/checkpoint-rewind-apply.test.ts`; P4A/P4B/P3A/P2.2/P1.3/P1.2/P1.1 regressions; runtime/app typechecks; `docs/analytix/qa/p4c-desktop-qa-2026-06-20.md` |
| G0/G5 runtime conformance inventory | Kun / Reasonix | runtime contracts / Go oracle / baseline tests | must for future Go | Preserve the TypeScript runtime as the golden oracle before any Go scaffold: freeze contract surfaces for thread/session, SSE replay, checkpoint/rewind plan/apply, safety audit, tool calls, approvals, user input, plan/goal, model-history repair, cache accounting, usage, and provider request/stream parsing; classify Kun 0.2.13, Kun 0.2.14/current, and Reasonix `main-v2` capability inputs; record G5 fixture gaps and missing production evidence. | docs / packages/runtime tests / future Go runtime | closed for inventory baseline; implementation pending | `docs/analytix/upstreams/go-runtime-conformance.md`; full runtime suite passes; related renderer/main tests, typechecks, identity/protocol scan, and release QA remain closure gates |
| Reasonix engine map | Reasonix | runtime / provider / tool / Go / QA | must / should / optional / reject | Absorb engine code through analytix contracts; reject Reasonix public identity and native renderer-visible protocols. | main / shared / packages/runtime / future Go runtime / docs | planned for implementation | `reasonix-sync.md`, `go-runtime-conformance.md`, `code-level-absorption-blueprint.md` |
| Reasonix agent-kernel five-batch route | Reasonix | permission / evidence / long-running tasks / tool economy / collaboration / Go kernel | must first / should / needs redesign / defer | Future Reasonix absorption must proceed in order: `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan-vs-tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules before AutoResearch, tool economy, `parallel_tasks`, background jobs, planner/executor, or Go kernel work. | docs / packages/runtime / future Go runtime | governance closed; Batch 1-4 scoped TS proofs closed; Batch 5 G0/G1 shadow manifest implemented | Batch 1-5 focused runtime/renderer/conformance tests, specs/ledgers/scorecard, `npm --prefix packages/runtime test`, `npm run test`, `npm run typecheck`, `npm run build:runtime`, and static scans |
| D-0014 Batch 1 complete_step + evidence ledger | Reasonix | permission / evidence / Goal | must absorb first | Implement the first task-closure proof as analytix-owned runtime state: `complete_step` writes `ThreadGoal.evidenceLedger`, goal completion requires evidence at tool and service layers, and renderer projection passes the ledger without adding UI entry points. | packages/runtime / renderer contract / docs | scoped TS proof closed; wider release/parity gates pending | Focused runtime tests pass for goal tools, permission posture, plan/tool separation, blocked-state, and subagent/headless policy inheritance. |
| D-0014 Batch 2 AutoResearch project-local state | Reasonix | long-running task / research state / evidence audit | must absorb after Batch 1 | Implement `/goal --research` inside existing goal/chat surface with analytix-owned project-local files, `record_research_direction`, and requirement-by-requirement audit. | packages/runtime / renderer goal command / docs | scoped TS proof closed; wider release/parity gates pending | `packages/runtime/tests/autoresearch-store.test.ts`, focused goal/http/loop tests, renderer goal command/store tests, typecheck, and build:runtime. |
| D-0014 Batch 3 dynamic tool source and context economy | Reasonix | tool surface / context economy / cache diagnostics | should absorb after batches 1-2 | Implement `connect_tool_source` semantics as analytix-owned dynamic source lifecycle, keep model tool schemas stable, record source diagnostics outside the stable prefix, and prove token economy/history/memory/compaction/cache behavior stays multi-provider safe. | packages/runtime / renderer contract / docs | scoped TS proof closed; wider provider/live gates pending | `packages/runtime/tests/capability-registry.test.ts`, `cache.test.ts`, `mcp-tool-provider.test.ts`, `loop.test.ts`, token economy/history/compaction/memory/usage/provider-cache focused tests, typecheck, and build:runtime. |
| D-0014 Batch 4 delegated collaboration evidence proof | Reasonix | collaborative execution / subagent evidence / nested event rendering | should absorb after batches 1 and 3 | Harden the existing `delegate_task` substrate so completed child work is permission-gated, cancellable, queued by maxParallel, projected as nested events, and evidence-ledgered against an active parent Goal. | packages/runtime / renderer projection / docs | scoped TS proof closed; wider task/background/coordinator parity pending | `packages/runtime/tests/delegation-runtime.test.ts`, `child-agent-executor.test.ts`, `loop.test.ts`, `runtime-event-reducer.test.ts`, renderer mapper tests, typecheck, and build:runtime. |
| D-0014 Batch 5 Go runtime G1 shadow scaffold | Reasonix | Go kernel / conformance / backend-neutral runtime | should start only after Batch 1-4 TS oracle | Add an isolated Go shadow package that implements only G1 health/info/tools handlers against the TypeScript oracle fixture before any backend switch or G2 route replay. | packages/runtime / packages/runtime-go / docs | G1 shadow scaffold closed; default backend and G2-G6 pending | `packages/runtime/tests/go-runtime-conformance.test.ts`, `go test ./...` in `packages/runtime-go`, focused route contract assertions, and scaffold scans proving no Rust/Tauri/default backend path. |
| Reasonix latest currentness + Go G2 shadow replay | Reasonix / Kun | provider config / MCP lifecycle / Go route conformance / release evidence | classify before code; close scoped shadow proof; defer live parity | Record Reasonix `91fe06db` as the latest upstream input after `49c14762`: codebase-memory MCP auto-indexing is a future `wrap-behind-contract` dynamic tool-source/MCP startup candidate, MiMo built-ins migration rejects deleting analytix Xiaomi/MiMo presets and records custom-provider migration as future import/repair work. Add G2 shadow route replay only; do not change renderer/preload/main, `analytix serve`, top-level `runtime`, or default backend. | packages/runtime / packages/runtime-go / docs | scoped shadow proof closed; live provider/default backend pending | Upstream ls-remote/log/stat; `go-runtime-conformance.test.ts`; `approval-user-input-route-oracle.test.ts`; `provider-cache-proof.test.ts`; `mcp-tool-lifecycle-oracle.test.ts`; `go test ./...` with temporary SHA-verified official Go; production scans; route-surface guards; release evidence gate. Live provider matrix is still missing. |
| Reasonix P0 engine parity + Kun desktop hardening | Reasonix / Kun | provider/cache / task jobs / MCP known override / SSE IPC / shell desktop / Go G3-G4 | scoped absorb; defer live parity | Close the scoped stage behind analytix contracts: cache/usage parsing covers DeepSeek, OpenAI-compatible/Responses, Anthropic, reasoning tokens, canonical tool schema hash, and sanitized request attribution; task/background/planner oracle adds durable job manager skeleton without product routes; MCP known overrides cover codegraph/codebase-memory cwd and lifecycle diagnostics; Kun SSE IPC and shell navigation hardening are absorbed; Go G3/G4 remain shadow-only fixtures. | packages/runtime / packages/runtime-go / src/main / renderer / docs | scoped stage closed; live provider/MCP/packaged QA pending | Focused provider/cache, task-job, MCP lifecycle, SSE IPC, shell renderer, Go G3/G4 TS oracle, and `go test ./...` with temporary SHA-verified official Go; full final gate rerun after docs edits. |
| Reasonix 9e56 MCP/task-job route parity | Reasonix / Kun | MCP startup / task jobs / provider diagnostics / shell safe-area | scoped absorb; defer full parity | Treat todo commits as record-only, reimplement MCP retry-all and late-provider disable semantics behind `CapabilityRegistry`, and expose task-job wait/output/kill only as authenticated internal runtime routes. Provider probe/model-list diagnostics redact secrets. Kun contribution is shell safe-area closure only; Local Whisper/tray remain deferred. | packages/runtime / src/main / renderer shell / docs | scoped code slice closed; live parity pending | Focused runtime MCP/task-job tests and provider probe test pass; final full gate passed: `git diff --check`, `npm --prefix packages/runtime test`, `npm run test`, `npm run typecheck`, `npm run build:runtime`, production scans, and release evidence update. |
| Reasonix collaborative execution/cache/MCP/Go G5 oracle closure | Reasonix / Kun | task/parallel/background/planner contract / cache curve / MCP indexer / Go G5 / shell safe-area | scoped absorb; defer live/full parity | Add internal `TASK_TOOL_CONTRACT` and `PARALLEL_TASKS_TOOL_CONTRACT`, normalize `depends_on`, reject invalid dependency graphs, expand task-job oracle for nested SSE/parent evidence/transcript constraints, add offline cache curve release guard, expand MCP lifecycle fixture to codebase-memory/codegraph retry/tombstone behavior, and freeze Go G5 TS-owned full-loop oracle inventory. | packages/runtime / docs / renderer shell | scoped code slice closed; full live parity pending | Focused runtime conformance suite passed for task-job/provider-cache/MCP/Go G5. Final full gate and scans are recorded in release evidence after this docs batch. |
| Reasonix planner/executor + shell/live-local/G5 runner shadow | Reasonix / Kun | planner/executor coordinator / durable runner / Windows shell / provider cache / MCP stdio indexer / Go G5 | scoped absorb; defer live/full parity | Reimplement Reasonix planner/executor and bfe398 shell value behind analytix contracts: planner jobs are read-only, executor waves propagate dependency failure and cancellation, durable queued/running jobs can be rehydrated after restart, Windows PowerShell blocks unsupported unquoted `&&`/`||` while pwsh permits chaining, DeepSeek cache telemetry is proved against an executable local provider, MCP indexer proof uses an executable stdio fake server, and Go G5 shadow consumes the new planner/runner fixture fields. | packages/runtime / packages/runtime-go / docs | scoped code slice closed; release/live parity pending | Focused runtime suites pass for task jobs, builtin shell tools, provider cache, MCP lifecycle, and Go G5 conformance; Go package tests pass with a temporary SHA-verified official Go toolchain. Final full gate and scans are recorded in release evidence. |
| Reasonix post-881 auto-plan currentness + Go G5 executable control + Kun context window | Reasonix / Kun | auto-plan classifier lifecycle / Go G5 control / provider defaults | contract-reimplement / document-only / reject / defer | Treat `01d9b173` user-level auto-plan as document-only/defer unless analytix later adds an auto-plan setting under top-level `runtime`; reject Reasonix `config auto-plan --local`; absorb `2db7acf6` only as an auto-router classifier fingerprint/cache-rebuild guard; add Go executable control cases for cancel/task-job/step-limit replay; absorb Kun 0.2.14 unknown-model `128_000` context-window default. | packages/runtime / packages/runtime-go / shared / docs | scoped code/docs tests closed; live parity/release pending | `auto-model-router.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...` with SHA-verified Go 1.25.11, `app-settings-provider.test.ts`, focused loop/provider tests, forbidden scans, and release evidence. |
| Reasonix approval/user-input abort cleanup | Reasonix | approval gate / user input gate / SSE replay / late GUI actions | contract-reimplement | Add an analytix-owned approval expiration primitive and abort-aware loop cleanup: pending approval promises reject, gate state becomes `expired`, `approval_resolved` is replayable, late GUI allow/deny returns conflict, user-input abort records `cancelled`, late GUI input resolve returns not found, and live mapper updates existing approval cards. | packages/runtime / renderer mapper / docs | scoped code/docs tests closed; desktop live QA pending | `ports.test.ts`, `loop.test.ts`, `approval-user-input-route-oracle.test.ts`, `analytix-mapper.test.ts`, G3/G4/G5 conformance focused tests, forbidden scans, and release evidence. |
| Structured `request_user_input` choice validation | Reasonix | user-input gate / tool schema / G4 conformance | contract-reimplement | Keep free-form GUI input available, but reject malformed structured choice requests before opening an analytix user-input gate: max three questions, two to three options when options are provided, case-insensitive duplicate-label rejection, and stable `invalid_user_input_request` result code. | packages/runtime / packages/runtime-go / docs | scoped code/docs tests closed; desktop live QA pending | `builtin-tools.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `packages/runtime-go go test ./...`, full final gates, forbidden scans, and release evidence. |
| Write-inline custom full endpoint proof | Reasonix | provider request URL/body/header/parser | contract-reimplement / code-port-and-adapt | Prove custom full endpoint mode is explicit for Write: `/responses` and `/messages` full URLs are used exactly and resolve to matching request/response shapes instead of receiving another appended endpoint path. | main write-inline tests / docs | scoped test/docs closed; live provider matrix pending | `write-inline-completion-service.test.ts`, full final gates, forbidden scans, and release evidence. |
| Auto-model route cache lifecycle proof | Reasonix | auto-router / classifier currentness / turn cache | contract-reimplement | Prove `AgentLoop` caches one auto-route within a multi-step turn and re-runs `_auto_router` on the following turn, preserving currentness without adding Reasonix auto-plan settings or classifier state to the stable prefix. | packages/runtime loop tests / docs | scoped test/docs closed; auto-plan product parity deferred | `loop.test.ts -t "auto model"`, full final gates, forbidden scans, and release evidence. |
| Auto-router request contract fingerprint currentness | Reasonix | auto-router / classifier currentness / route-cache key | contract-reimplement / code-port-and-adapt / reject / defer | Fingerprint classifier request-contract fields so max-token, temperature, or reasoning-effort drift changes route-cache keys, adapting Reasonix rebuild-on-enable value without importing Reasonix auto-plan config or controller protocol. | packages/runtime loop tests / docs | scoped test/docs closed; product auto-plan parity deferred | `auto-model-router.test.ts`, final gates, forbidden scans, and release evidence. |
| Provider request-shape oracle matrix | Reasonix | provider URL/body/header/tool shape / cache proof / Go shadow ids | contract-reimplement / code-port-and-adapt | Centralize provider request-shape invariants in `provider-cache-oracle.json` and execute them against `CompatModelClient`, while G3/G5 Go shadow only replays request-shape ids. | packages/runtime conformance fixtures / tests / packages/runtime-go / docs | scoped code/docs tests closed; live provider matrix pending | `provider-cache-proof.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, full final gates, forbidden scans, and release evidence. |
| Custom chat full endpoint request-shape oracle | Reasonix | provider URL/body/header/tool shape / cache proof / Go shadow ids | contract-reimplement / code-port-and-adapt / defer | Add `custom-chat-full-endpoint-request-shape` so custom full `/chat/completions` URLs stay exact and use OpenAI-compatible chat request shape, while G3/G5 Go shadow only replays the TS-owned id. | packages/runtime conformance fixtures / tests / packages/runtime-go / docs | scoped code/docs tests closed; live provider matrix pending | `provider-cache-proof.test.ts`, `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G3/G5 provider cache accounting shadow | Reasonix | provider cache accounting / DeepSeek cache / OpenAI Anthropic custom non-regression | code-port-and-adapt / contract-reimplement / defer | Add fixture-owned `cacheAccounting` and pure Go G3/G5 calculations for supported cache telemetry case ids, unsupported unknown cases, hit/miss totals, aggregate hit rate, and provider-family coverage. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live provider credential matrix deferred | `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, `provider-cache-proof.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Settings/provider endpointFormat persistence oracle | Reasonix / Kun | settings schema / provider profiles / endpointFormat persistence | contract-reimplement / reject | Add a settings-store write/reload proof that runtime `endpointFormat` and custom provider `endpointFormat` persist under top-level `runtime` and `provider.providers`, while old `agentProvider` / `agents` envelopes are not written. | main settings-store tests / docs | scoped test/docs closed; live provider matrix pending | `settings-store.test.ts`, full final gates, forbidden scans, and release evidence. |
| Runtime provider-selection request-shape oracle | Reasonix | provider selection / custom full endpoint / fallback request shape | contract-reimplement | Add a `MultiProviderModelClient` proof that a thread-selected custom provider uses its exact `/messages` full endpoint and Messages body/header shape, while a missing provider falls back to the default OpenAI-compatible URL/body/header path. | packages/runtime model tests / docs | scoped test/docs closed; live provider matrix pending | `multi-provider-model-client.test.ts`, full final gates, forbidden scans, and release evidence. |
| Scheduled detector custom endpoint inference oracle | Reasonix / Kun | scheduled task detector / custom endpoint URL-body-parser inference | contract-reimplement | Prove the scheduled reminder detector uses exact custom full `/messages` and `/chat/completions` URLs and infers the matching body/header/parser family without appending another endpoint path. | main scheduled detector tests / docs | scoped test/docs closed; live provider matrix pending | `claw-scheduled-task-detector.test.ts`, final gates, forbidden scans, and release evidence. |
| Forbidden public runtime route oracle | Reasonix / Kun / Go | HTTP route sovereignty / public protocol rejection | contract-reimplement / reject | Extend the runtime HTTP server negative route oracle so Reasonix public routes, renderer-visible Go routes, and top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer HTTP surfaces return 404. | packages/runtime HTTP tests / docs | scoped test/docs closed; future route additions must update conflict decisions | `http-server.test.ts`, final gates, forbidden scans, and release evidence. |
| Rehydrated task-job route oracle | Reasonix / Go | durable task jobs / authenticated runtime routes / restart continuity | contract-reimplement | Extend the durable task-job restart proof so rehydrated running/queued jobs remain operable through authenticated `/v1/runtime/task-jobs/output|wait|kill` routes after restart. | packages/runtime task-job oracle / docs | scoped test/docs closed; live sub-agent route parity rejected | `task-job-orchestration-oracle.test.ts`, final gates, forbidden scans, and release evidence. |
| Go G5 durable runner restart executable shadow | Reasonix / Go | durable task jobs / restart continuity / G5 executable shadow | code-port-and-adapt / contract-reimplement / reject / defer | Promote the TS-owned restart drill into `controlExecutableCases.taskJobs.restartDrill`, with Go shadow computing rehydrated count, combined output/next offset, and queued kill result. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go Job Manager deferred | `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Task-job approval deny no-execute oracle | Reasonix | tool approval / task jobs / child-run safety | contract-reimplement | Prove real `task` and `parallel_tasks` providers return approval items and create no durable jobs or child runs when GUI approval is denied. | packages/runtime task-job oracle / docs | scoped test/docs closed; live GUI approval QA pending | `task-job-orchestration-oracle.test.ts`, final gates, forbidden scans, and release evidence. |
| Go G5 task-job approval deny shadow replay | Reasonix | Go G5 / task jobs / approval gate | contract-reimplement / defer | Carry the TS-owned approval deny no-execute oracle into Go G5 `jobReplay.approvalDenyNoExecute`, so future Go job/approval work must preserve approval-before-side-effect ordering. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Go G5 task approval deny executable control | Reasonix | Go G5 / task jobs / approval gate | code-port-and-adapt / contract-reimplement / defer | Add a pure Go G5 `controlExecutableCases.approvalDeny` calculation that returns denied tool names, approval ids/count, and no durable-job/child-run side effects from the TS-owned task-job oracle. | packages/runtime conformance fixtures / packages/runtime-go / docs | scoped shadow code/docs tests closed; live Go approval manager deferred | `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, final gates, forbidden scans, and release evidence. |
| Auto-router failure usage isolation | Reasonix | auto-plan classifier / route fallback / usage cache isolation | contract-reimplement / reject / document-only | Prove `_auto_router` failure falls back to heuristic while router usage/cache telemetry is not recorded as main thread usage and does not pollute tool/prefix surface. Reject Reasonix user/project auto-plan config; cross-reference the later managed runtime provider/settings rebuild proof. | packages/runtime loop tests / docs | scoped test/docs closed; settings/provider rebuild proof closed by Managed Runtime Provider Currentness and Identity Guard | `loop.test.ts -t "falls back to a concrete heuristic model without recording router usage"`, managed runtime currentness tests, final gates, forbidden scans, and release evidence. |
| Kun top-level route-surface oracle | Kun / Reasonix | product entry / route actions / Workbench shell | contract-reimplement / reject | Turn the Kun 0.2.13 -> 0.2.14 entry boundary into executable renderer tests: `AppRoute`, app actions, Workbench stage, shell navigation, and sidebar active view cannot expose Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer as top-level entries. Internal Create Loop/task/research/runtime code remains allowed when hidden behind analytix contracts. | renderer route-surface tests / docs | scoped test/docs closed; future product-entry changes must update conflict decisions | `Workbench.route-surface.test.ts`, `chat-store-app-actions.test.ts`, focused route-surface test run, final gates and forbidden scans in release evidence. |
| Auto-route step/cancel control-composition oracle | Reasonix | auto-router / step limit / cancel / Go G5 control | contract-reimplement / code-port-and-adapt | Add a real `AgentLoop` proof that auto-route cache is reused until a user-global step limit fires without stable-prefix drift, plus a G5 shadow combined executable case proving auto-route cache, step-limit, and cancelled tool-result invariants stay compatible. | runtime loop tests / G5 fixtures / packages/runtime-go / docs | scoped code/docs tests closed; live Go/runtime parity pending | `loop.test.ts -t "auto route|step limit"`, `go-runtime-conformance.test.ts`, `packages/runtime-go go test ./...`, full final gates and forbidden scans in release evidence. |
| Preload bridge/API sovereignty oracle | Kun / Reasonix | preload bridge / renderer Window type / shared public API names | contract-reimplement / reject | Turn the `window.analytix` sovereignty boundary into executable tests: preload may expose only the `analytix` bridge, `Window` may declare only `analytix`, and shared public API exports cannot introduce Reasonix/Kun/deprecated GUI facade names. | preload/shared tests / docs | scoped test/docs closed; public protocol drift guarded | `preload-sandbox.test.ts`, full final gates, forbidden scans, and release evidence. |
| Renderer thread lifecycle HTTP oracle | Kun / Reasonix | renderer runtime adapter / thread list-search-archive-restore-rename-workspace-delete | contract-reimplement | Extend the analytix renderer runtime provider test oracle so thread list/search/archive filters and thread lifecycle mutations use analytix HTTP paths under `/v1/threads`, complementing existing fork/resume/SSE tests. | renderer runtime tests / docs | scoped test/docs closed; packaged desktop QA pending | `analytix-runtime.test.ts`, full final gates, forbidden scans, and release evidence. |
| Renderer approval live-card store evidence | Reasonix | renderer store / approval live cards / side conversations | contract-reimplement | Extend the approval cleanup proof from mapper-level evidence to analytix-owned renderer store evidence: main-thread `buildThreadEventSink` updates an existing approval card from live status events, and side conversation SSE sinks update side blocks without touching main thread blocks. No Reasonix frontend protocol, bridge, settings, provider, Go/Rust/Tauri, or top-level navigation surface is added. | renderer tests / docs | scoped test/docs closed; packaged desktop QA pending | `chat-store-runtime.test.ts`, `chat-store-side-actions.test.ts`, full final gates, forbidden scans, and release evidence. |
| Renderer user-input live-card store evidence | Reasonix | renderer store / user-input live cards / side conversations | contract-reimplement / code-port-and-adapt | Extend the same renderer store proof to `request_user_input`: main-thread status updates can match by item id or input/request id, side user-input blocks now use runtime item ids, and side status updates remain scoped to side blocks while preserving answers/error messages. No Reasonix ask protocol, bridge, settings, provider, Go/Rust/Tauri, or top-level navigation surface is added. | renderer store / renderer tests / docs | scoped code/docs tests closed; packaged desktop QA pending | `chat-store-runtime.ts`, `chat-store-side-actions.ts`, `chat-store-runtime.test.ts`, `chat-store-side-actions.test.ts`, full final gates, forbidden scans, and release evidence. |
| Planner gating for internal task tools | Reasonix | planner gating / task jobs / active tool policy | contract-reimplement / code-port-and-adapt | Keep `task` / `parallel_tasks` behind analytix internal runtime contracts while proving Plan mode excludes them from advertised tools and rejects forged `task` calls before child work executes. The task-job oracle now records `plannerForbiddenToolset`. No Reasonix public task/planner protocol, bridge, settings, provider, Go/Rust/Tauri, or top-level navigation surface is added. | runtime loop / task-job oracle / docs | scoped code/docs tests closed; full planner parity pending | `loop.test.ts`, `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`, full final gates, forbidden scans, and release evidence. |
| Managed runtime provider currentness and identity guard | Reasonix / Kun | runtime settings key / provider snapshot / product identity | contract-reimplement / reject / defer | Promote the runtime-affecting settings key into shared code and prove provider endpoint/model-profile drift changes managed runtime rebuild decisions and `ANALYTIX_MODEL_PROVIDERS`; close the model-visible Connect Phone prompt leak; reject `agentProvider: "kun"` / `agents.kun` fallback and public `claw/kun/reasonix` facade domains. | shared settings / main runtime env / runtime prompt / preload/settings tests / docs | scoped code/docs tests closed; live provider and public auto-plan settings deferred | `app-settings-provider.test.ts`, `analytix-process.test.ts`, `analytix-system-prompt.test.ts`, `preload-sandbox.test.ts`, `settings-store.test.ts`, full final gates, forbidden scans, and release evidence. |
| Connect Phone copy sovereignty proof | Kun / Reasonix | product identity / Connect Phone prompts / legacy compatibility | contract-reimplement / code-port-and-adapt / reject | Replace newly generated `Claw IM` prompt/title/schema/log copy with `Connect Phone` while retaining legacy Claw title/prompt recognizers for historical sessions only. | shared prompts / main Connect Phone runtime / renderer thread recognizers / docs | scoped code/docs tests closed; legacy compatibility retained | `app-settings.test.ts`, `schedule-runtime.test.ts`, `MessageTimeline.tool-summary.test.ts`, `chat-store-helpers.test.ts`, `chat-store-claw-actions.test.ts`, final gates, forbidden scans, and release evidence. |
| Provider endpoint URL builder parity | Reasonix / Kun | provider URL construction / scheduled detector / write-inline non-regression | code-port-and-adapt / contract-reimplement | Move versioned endpoint and known-path stripping into shared `upstreamOpenAiModelEndpointUrl`; scheduled detector and write-inline now share the same URL builder for responses/messages/custom endpoint families, and tests cover `/v2`/`/v3` endpoint bases without duplicated paths. | shared URL helper / main scheduled detector / write-inline tests / docs | scoped code/docs tests closed; live provider matrix deferred | `openai-compat-url.test.ts`, `claw-scheduled-task-detector.test.ts`, `write-inline-completion-service.test.ts`, final gates, forbidden scans, and release evidence. |
| Reasonix session-list sidecar delta | Reasonix | runtime / QA | must | Absorb `249a4f8`, `73e2025`, and `5db1d0b` behavior after adapting to analytix session/thread contracts. | packages/runtime / main / docs | complete | `reasonix-sync.md`, focused runtime/renderer tests, `npm run typecheck`, `git diff --check`, `upstream-scorecard.md` |
| Reasonix sidecar count authority | Reasonix | runtime / history consistency | must | Absorb `7ebb08e` by versioning analytix sidecar summaries so zero counts can be trusted and legacy summaries backfill once. | packages/runtime / docs | complete | `packages/runtime/tests/hybrid-store.test.ts`, `reasonix-sync.md`, `upstream-scorecard.md` |
| Reasonix goal-state off-lock write | Reasonix | runtime / goal responsiveness | should | Do not invent a TS controller lock abstraction; record `341f720` as a Go runtime G4/G5 requirement that goal persistence writes happen outside status/approval/controller critical sections. | future Go runtime / docs | conformance requirement | `go-runtime-conformance.md`, `reasonix-sync.md` |
| Reasonix memory-write controller lock split | Reasonix | runtime / memory / control | should | Defer `f6ba755` from the session-sidecar batch; later adapt the off-controller-lock persistence idea behind analytix runtime contracts. | packages/runtime / future Go runtime / docs | deferred follow-up | `reasonix-sync.md` refresh entry |
| Reasonix goalMachine extraction | Reasonix | goal/control runtime | should absorb | Absorb `dbaea843` by extracting analytix-owned goal continuation state into `GoalControlMachine`: active-goal instruction text, blocked audit guidance, no-tool repetition recovery, empty post-file-change recovery/failure, and non-progress goal tool classification. Keep persistence in existing `ThreadGoal`/`goal_updated` contracts and do not expose Reasonix markers or sidecars. | packages/runtime / docs / future Go runtime | closed for scoped code slice | `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/goal-tools.test.ts`, `packages/runtime/tests/goal-repetition-guard.test.ts`; `reasonix-sync.md`; future Go G5 oracle |
| Reasonix inspect/package-lock cleanup | Reasonix | repo hygiene | classify only | `bb06f5b4` deletes Reasonix `internal/inspect` and a redundant frontend lockfile. Analytix has no equivalent Go inspect package or nested Reasonix lockfile; root `package-lock.json` remains authoritative for the Electron app. | docs | no-op classified | `reasonix-sync.md` |
| Reasonix approvalManager extraction | Reasonix | approval/control runtime | partially absorbed / defer broader UI | `726036bd` / `bc8249c3` moves approval/ask bookkeeping in Reasonix. Analytix absorbed the abort-cleanup invariant through gate expiration, SSE replay, and live approval-resolved mapper status; deeper approval-manager refactors remain future work. | packages/runtime / main / renderer / docs | abort cleanup closed; broader approval-control parity pending | Current: `ports.test.ts`, `loop.test.ts`, `approval-user-input-route-oracle.test.ts`, `analytix-mapper.test.ts`. Future: desktop live route QA. |
| Reasonix store-sidecar authority | Reasonix | session persistence / Go module boundary | defer pending Go store batch | `98a57ded` / `d02457ee` centralizes Reasonix session sidecar path derivation in `internal/store` with no intended behavior change. Analytix already owns TS file/hybrid store contracts; record the idea only for a future Go store module after route fixtures exist. | future Go runtime / docs | record-only currentness refresh | `reasonix-sync.md`; no code absorption in this batch |
| Reasonix SessionAPI driving port | Reasonix | runtime control boundary / remote entry safety | scoped absorbed | `3e625b91` / `48e5b990` splits Reasonix frontend-driving control into lifecycle, turn-driving, and approval/ask sub-ports. Analytix absorbed the idea as `RemoteEntryControlPort` and `createRemoteEntryControlPort`, not as Reasonix public `SessionAPI`, and proved remote/bot-like entries cannot reach goal/checkpoint/memory/storage control planes. | packages/runtime / docs / future Go runtime | closed for scoped code/doc slice | `packages/runtime/tests/remote-entry-control-port.test.ts` plus full runtime suite; future Go G1/G2/G4/G5 must reproduce the same narrow entry boundary |
| Kun Windows installer process-stop | Kun | Windows release / NSIS upgrade safety | must absorb | `d09d52b0` stops bundled processes under the install directory before Windows upgrade. Absorbed as analytix-native `build/installer.nsh` with `ANALYTIX_INSTALLER_*` environment variables and `electron-builder.config.cjs` `nsis.include`; no `KUN_*`, Kun artifact, or old product identity is introduced. | build / electron-builder / docs | implemented; Windows machine QA pending | Static config/source scan plus future Windows NSIS machine verification and release evidence gate |
| Kun develop pre-release candidate | Kun | UI / runtime / provider / Connect Phone / workflow / QA | must / should / redesign | Superseded by Kun master `8602476` merge of `ab24a77`; keep this row as historical pre-review only. | renderer / main / shared / packages/runtime / docs | superseded by stable-master drift | `kun-sync.md`, `code-level-implementation-plan.md` |

## 2026-06-21 - Reasonix P0 Engine Parity + Kun Desktop Hardening

Goal:

```text
Close the scoped Reasonix P0 engine parity and Kun desktop hardening stage
without changing analytix product contracts.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider/cache P0 | `prefix-cache-diagnostics.ts`, `tool-catalog-fingerprint.ts`, `compat-model-client.ts`, `provider-cache-proof.test.ts`, `model-client.test.ts`, `usage-service.test.ts`, `cache.test.ts`. |
| Task/background/planner oracle | `delegation/job-manager.ts`, `task-job-orchestration-oracle.json`, `task-job-orchestration-oracle.test.ts`, `delegation-runtime.test.ts`, `child-agent-executor.test.ts`. |
| MCP/codebase-memory override | `mcp-tool-provider.ts`, `mcp-tool-lifecycle-oracle.json`, `mcp-tool-provider.test.ts`, `mcp-tool-lifecycle-oracle.test.ts`, `mcp-config.test.ts`. |
| Kun SSE IPC | `src/main/runtime-sse-ipc.ts`, `src/main/runtime-sse-ipc.test.ts`. |
| Kun shell hardening | `ShellNavigationControls.tsx`, Workbench/Write/Connect Phone focused tests and shell CSS/locales. |
| Go G3/G4 shadow conformance | `go-g3-provider-streaming-usage-cache-oracle.json`, `go-g4-tools-approval-user-input-mcp-oracle.json`, `go-runtime-g3-g4-conformance.test.ts`, `packages/runtime-go/shadow_g3g4.go`. |

Skipped:

```text
Reasonix public protocol, Reasonix settings root, Reasonix renderer event
shape, Kun identity, top-level Workflow/Create Loop, top-level Subagent /
AutoResearch / MCP indexer UI, default Go backend, Electron-to-Go routing,
Rust/Tauri rewrite, Local Whisper bundled resources, tray session menu, live
provider/MCP parity, and packaged release readiness.
```

Validation:

```text
Focused suites passed for provider/cache, MCP lifecycle, task jobs, runtime SSE
IPC, Go G3/G4 conformance, Go shadow package, and renderer shell hardening.
Final stage validation is rerun after docs edits in the release evidence gate.
```

Remaining risks:

```text
Live provider/MCP QA, packaged desktop QA, Windows NSIS, signing/notarization,
release metadata, verified analytix remote, and Go G5 remain next-stage gates.
```

## 2026-06-21 - Collaborative Execution / Cache Curve / MCP Indexer / Go G5 Oracle Closure

Goal:

```text
Close the next post-9e56 fixture/oracle layer without changing analytix product
contracts or exposing upstream-native protocols.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Collaborative execution contract | `packages/runtime/src/delegation/job-manager.ts`, `task-job-orchestration-oracle.json`, `task-job-orchestration-oracle.test.ts`. |
| Provider cache curve | `provider-cache-oracle.json`, `provider-cache-proof.test.ts`. |
| MCP/indexer lifecycle | `mcp-tool-lifecycle-oracle.json`, `mcp-tool-lifecycle-oracle.test.ts`, existing `mcp-tool-provider.test.ts` and `capability-registry.test.ts`. |
| Go G5 oracle inventory | `go-g5-full-loop-oracle.json`, `go-runtime-kernel-conformance.ts`, `go-runtime-conformance.test.ts`. |
| Kun desktop boundary | shell/native titlebar safe-area commits and route-surface/native chrome guards; tray/Local Whisper stay future gated. |

Skipped:

```text
Reasonix public task/parallel/planner protocol, top-level Workflow/Create Loop,
top-level Subagent/AutoResearch/MCP-indexer UI, Kun identity, tray session menu
productization, Local Whisper bundled resources, live provider/MCP parity,
Go Electron integration, default Go backend, Rust/Tauri rewrite, and release
readiness claims.
```

Validation:

```text
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts provider-cache-proof.test.ts mcp-tool-lifecycle-oracle.test.ts go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts
git diff --check
```

Remaining risks:

```text
Full Reasonix planner/executor parity, restart/resume crash drills, live MCP
indexer QA, live provider/cache matrix, packaged desktop tray/Local Whisper QA,
Go full loop/jobs/cache/compaction/resume/interrupt, rollback/default backend,
and release readiness remain open.
```

## Batch Template

```text
## YYYY-MM-DD - <batch name>

Goal:

Upstream sources:
  Kun:
  Reasonix:
  CodexDesktop-Rebuild:

analytix contracts touched:

Accepted items:

Rejected/deferred items:

Conflicts and decisions:

Implementation notes:

Validation:

Remaining risks:
```

## 2026-06-21 - Executable Runtime / Cache / MCP Live-Local / Kun Tray Gate

Goal:

```text
Move the previous oracle-only collaborative execution, cache, MCP/indexer,
and Kun tray candidates into executable or release-gated analytix-owned proof,
without changing public analytix contracts.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Reasonix task/parallel executable slice | `packages/runtime/src/adapters/tool/task-job-tool-provider.ts`, `packages/runtime/src/server/runtime-factory.ts`, `packages/runtime/src/delegation/job-manager.ts`, `packages/runtime/src/server/routes/task-jobs.ts`, `task-job-orchestration-oracle.test.ts`. |
| Restart/resume-safe job evidence | Startup reconciliation marks stale queued/running jobs failed after restart, and output reads support offsets with `nextOffset` for repeatable waits. |
| DeepSeek cache curve proof | `packages/runtime/src/cache/offline-cache-curve-guard.ts`, `provider-cache-proof.test.ts`, `go-g3-provider-streaming-usage-cache-oracle.json`; DeepSeek native hit/miss fields now take precedence over conflicting generic cached-token fields. |
| MCP live-local indexer proof | `packages/runtime/src/conformance/mcp-live-local-indexer.ts`, `mcp-tool-lifecycle-oracle.json`, `mcp-tool-lifecycle-oracle.test.ts`; fake local fixture covers retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics. |
| Kun tray session menu | `src/main/tray-session-menu.ts`, `src/main/tray-session-menu.test.ts`, `src/main/index.ts`; absorbed as main-process analytix menu that opens runtime threads in windows without adding renderer/preload bridge aliases. |
| Go G5 shadow implementation | No Go files changed. Local `go` executable is unavailable, so safe G5 implementation is blocked by toolchain validation. Existing TS-owned G5 inventory remains authoritative. |

Skipped:

```text
Reasonix public task/planner protocol, Reasonix SessionAPI/frontend task cards,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
Reasonix plugin protocol, Kun identity/bridge aliases, Local Whisper bundled
resources, default Go backend, Electron-to-Go routing, Rust/Tauri rewrite,
live provider superiority, live MCP parity, and release readiness.
```

Validation:

```text
Focused executable runtime, cache, MCP live-local, and tray suites passed.
Full command gates are recorded in the release evidence file after the final
stage rerun. Go validation is not run because no Go files changed and this
machine has no `go` command available.
```

Remaining risks:

```text
Full Reasonix planner/executor Coordinator, durable runner rehydration beyond
restart reconciliation, desktop nested-card QA, live provider/cache matrix,
live MCP/indexer QA, packaged tray QA, Local Whisper import boundary, Go G5
implementation with a Go toolchain, packaged release QA, signing, Windows, and
release metadata remain open.
```

## 2026-06-21 - Reasonix bfe398 Drift + Go G5 Shadow Output Slice

Goal:

```text
Classify the new Reasonix PowerShell compatibility drift and move Go G5 from
inventory-only to a tested shadow output slice, without adding a Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Reasonix bfe398 drift | Classified PowerShell 7 path detection and pwsh chaining guidance as shell/sandbox compatibility input. No code import in this stage. |
| Go G5 shadow output | `packages/runtime-go/shadow_g5.go`, `shadow_test.go`, `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, `go-runtime-conformance.test.ts`. |
| Toolchain evidence | Temporary official `go1.26.4.darwin-arm64` from `go.dev` was SHA-verified and used only for `gofmt` / `go test ./...`. |

Skipped:

```text
Reasonix shell/sandbox code copy, Reasonix public protocol, default Go backend,
Electron-to-Go routing, renderer-visible Go route, live Go full-loop executor,
live provider/MCP parity, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && go test ./...
```

Remaining risks:

```text
Go G5 still lacks live jobs/cache/session/resume/interrupt/MCP runtime behavior,
Electron integration, rollback/default-backend plan, packaged QA, and live
provider/MCP matrix.
```

## 2026-06-21 - Reasonix 881 Cancel / Batch / Step-Limit Control Gate

Goal:

```text
Absorb the runtime control value from Reasonix bfe398cc..881b2f2f while keeping
analytix contracts public and TS-owned. Record that Reasonix origin/main-v2 is
now 9ada1417, so 881 is no longer current HEAD.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Cancelled batch results | `packages/runtime/src/loop/agent-loop.ts`, `packages/runtime/tests/loop.test.ts`. Completed tool results persist; cancelled/never-started calls get paired `tool_call_cancelled` results. |
| Task-job parent cancel | `packages/runtime/src/delegation/job-manager.ts`, `packages/runtime/src/adapters/tool/task-job-tool-provider.ts`, `packages/runtime/tests/task-job-orchestration-oracle.test.ts`. Parent abort kills running jobs and returns skipped unstarted tasks. |
| User-global/default/session step limits | `src/shared/app-settings-*`, `src/main/analytix-process.ts`, `packages/runtime/src/config/analytix-config.ts`, `packages/runtime/src/contracts/{threads,turns}.ts`, `packages/runtime/src/loop/agent-loop.ts`, `packages/runtime/tests/{loop,contracts,delegation-runtime}.test.ts`. |
| Go G5 control shadow | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, `go-runtime-conformance.test.ts`, `packages/runtime-go/shadow_g5.go`. |

Skipped:

```text
Reasonix TUI cancel implementation, Reasonix public protocol, Reasonix config
root, auto-plan currentness drift after 881, live Go execution, default Go
backend, Electron-to-Go routing, Rust/Tauri rewrite, live provider/cache
superiority, live MCP/indexer parity, release readiness.
```

Stash audit:

```text
codex-preserve-workbench-preload-before-reasonix-parity: duplicate/covered by
current HEAD semantics; retained, not dropped, because not byte-identical.
codex-preserve-loading-page-before-reasonix-parity: duplicate/covered by
current HEAD semantics; retained, not dropped, because not byte-identical.
```

Validation:

```text
npm --prefix packages/runtime test -- loop.test.ts contracts.test.ts analytix-config.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- delegation-runtime.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- app-settings.test.ts app-ipc-schemas.test.ts analytix-process.test.ts
```

## 2026-06-21 - Go G5 Composite Shadow Replay

Goal:

```text
Move G5 shadow evidence from a single inventory output to cross-fixture replay
for jobs, cache, session/resume/SSE, and MCP/indexer.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Go composite replay | `packages/runtime-go/shadow_g5.go` now accepts task-job, provider-cache, G2 route replay, and MCP lifecycle fixtures and builds `BuildG5ShadowSlicesOutput`. |
| TypeScript binding | `go-runtime-conformance.test.ts` ties `shadowSlicesExpectedOutput` to the same source fixtures so expected output cannot drift independently. |
| Fixture evidence | `go-g5-full-loop-oracle.json` records `shadowSlicesExpectedOutput` for job replay, cache replay, session replay, MCP replay, and product boundary. |

Skipped:

```text
Live Go runtime behavior, Electron main integration, renderer-visible Go route,
default Go backend, Reasonix public protocol, live provider/MCP parity, and
release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && go test ./...
```

Remaining risks:

```text
Go G5 composite replay is still conformance-only. It does not run turns, tools,
providers, sessions, or MCP servers.
```

## 2026-06-21 - Go G5 Planner-Forbidden Shadow Replay

Goal:

```text
Consume the new plannerForbiddenToolset task-job oracle in Go G5 shadow output
without turning Go into a live backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Go G5 job replay | `packages/runtime-go/shadow_g5.go` reads `PlannerForbiddenToolset` and emits it in `BuildG5ShadowSlicesOutput`. |
| TS conformance binding | `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, and `go-runtime-conformance.test.ts` require `plannerForbiddenToolset` to match the TS task-job oracle. |
| Product boundary | No renderer route, bridge/settings/provider change, default Go backend, Reasonix public protocol, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Live Go planner/executor, Go Job Manager, Go provider/cache/session/MCP
runtime, Electron-to-Go routing, renderer-visible Go route, default Go
backend, Rust/Tauri rewrite, packaged QA, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
Go G5 remains shadow-only. It can replay the planner-forbidden fixture but does
not execute planner turns, child jobs, providers, sessions, or MCP servers.
```

## 2026-06-21 - Write-Inline Provider Request-Surface Proof

Goal:

```text
Extend provider/cache non-regression evidence to the desktop write-inline
Responses and Messages endpoint paths without changing provider behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Responses write-inline path | `src/main/services/write-inline-completion-service.test.ts` proves `/v1/responses`, `input`, `max_output_tokens`, and no Anthropic header leakage. |
| Messages write-inline path | The same test file proves `/v1/messages`, `system` / `messages` / `max_tokens`, Anthropic-style headers, and response parsing. |
| Product boundary | No Reasonix public provider protocol, bridge/settings/provider default change, Go/Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Credentialed live provider matrix, live provider/cache superiority, cost
reconciliation, Reasonix provider protocol, default Go backend, and release
readiness.
```

## 2026-06-22 - D-0101 Reasonix Auto-Plan Config Boundary Guard

Goal:

```text
Absorb the useful Reasonix `01d9b173` boundary that auto-plan must not be
project/local configurable, without importing Reasonix public settings or
controller protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| GUI settings normalization | `src/shared/app-settings-normalize.ts` drops root `agent`, `autoPlan`, and `auto_plan` shapes. |
| Runtime settings merge/key | `src/shared/app-settings-runtime.ts` strips runtime `autoPlan` / `auto_plan` before merge, migration, and settings-key generation. |
| Settings persistence | `src/main/settings-store.test.ts` proves re-save drops Reasonix auto-plan shapes. |
| Runtime config parser | `packages/runtime/src/config/analytix-config.test.ts` proves `agent.auto_plan`, `runtime.auto_plan`, and `serve.autoPlan` are rejected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Project/local auto-plan override rejection | contract-reimplement | Accepted as an analytix settings/config boundary guard. |
| Reasonix user-level auto-plan setting | document-only / defer | No product setting is added. |
| Reasonix config CLI/controller protocol | reject | Do not expose upstream config commands, SessionAPI, or rebuild controller. |
| Go/backend behavior | reject | No Go backend/default route changes. |

Validation:

```text
npm run test -- src/shared/app-settings.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- src/config/analytix-config.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is a settings/config boundary proof. It does not implement a product
auto-plan toggle, packaged settings QA, Reasonix controller rebuild parity, or
release readiness.
```

Validation:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is fixture-backed desktop request-surface proof. It does not exercise live
OpenAI/Anthropic/custom credentials or prove live cache/cost superiority.
```

## 2026-06-21 - Go G5 Planner Tool-Policy Executable Shadow

Goal:

```text
Move planner-forbidden Go G5 evidence from replay-only into the executable
control shadow without connecting Go as a runtime backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Go executable planner gate | `packages/runtime-go/shadow_g5.go` computes step 0 read-only + `create_plan`, step 1 `create_plan` only, and forged `task` rejection. |
| Go package proof | `packages/runtime-go/shadow_test.go` compares the computed planner output with TS-owned fixture expectations. |
| TS conformance binding | `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, and `go-runtime-conformance.test.ts` bind planner executable input to the task-job oracle. |
| Product boundary | No renderer route, bridge/settings/provider change, default Go backend, Reasonix public protocol, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Live Go planner/executor, Go Job Manager, Go provider/cache/session/MCP
runtime, Electron-to-Go routing, renderer-visible Go route, default Go
backend, Rust/Tauri rewrite, packaged QA, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
Go G5 remains shadow-only. It can compute the planner gate fixture but does not
execute planner turns, child jobs, providers, sessions, or MCP servers.
```

## 2026-06-21 - Structured User-Input Choice Validation

Goal:

```text
Convert Reasonix-style GUI input robustness into an analytix-owned runtime tool
contract and G4 shadow proof without importing Reasonix ask/session protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime tool validation | `packages/runtime/src/adapters/tool/local-tool-host.ts` rejects malformed structured `request_user_input` choices before `awaitUserInput`. |
| Runtime unit proof | `packages/runtime/tests/builtin-tools.test.ts` covers too many questions, one option, too many options, duplicate labels, stable error code, and no opened gate. |
| G4/Go shadow proof | `go-g4-tools-approval-user-input-mcp-oracle.json`, `runtime-parity-fixtures.ts`, `go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go/shadow_g3g4.go` record invalid cases and `opensGateOnInvalid:false`. |
| Product boundary | No Reasonix ask/session protocol, renderer route, bridge/settings/provider change, default Go backend, Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Reasonix public ask/session/control protocol, frontend protocol parity,
packaged desktop live QA, live remote entry QA, Go approval/user-input manager,
default Go backend, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/builtin-tools.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is runtime/oracle proof. It does not exercise packaged desktop rendering,
remote-entry live behavior, or a live Go user-input gate.
```

## 2026-06-21 - Write-Inline Custom Full Endpoint Proof

Goal:

```text
Extend provider request-surface proof to custom full endpoint URLs for
Responses and Messages without changing provider behavior or settings schema.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Custom Responses full endpoint | `src/main/services/write-inline-completion-service.test.ts` proves the exact `/responses` URL, Responses body shape, and no Anthropic header leakage. |
| Custom Messages full endpoint | The same test proves the exact `/messages` URL, Messages body shape, Anthropic-style headers, and response parser. |
| Product boundary | No Reasonix provider protocol, bridge/settings/provider default change, Go/Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Credentialed live provider matrix, live provider/cache superiority, cost
reconciliation, Reasonix provider protocol, default Go backend, and release
readiness.
```

Validation:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is fixture-backed desktop request-surface proof. It does not exercise live
custom provider credentials or prove live cache/cost superiority.
```

## 2026-06-21 - Auto-Model Route Cache Lifecycle Proof

Goal:

```text
Tie the post-881 classifier rebuild/currentness absorption to real AgentLoop
multi-step behavior without importing Reasonix auto-plan product settings.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Same-turn route reuse | `packages/runtime/tests/loop.test.ts` proves one `_auto_router` call serves a multi-step `model:"auto"` turn after a tool call. |
| Next-turn currentness | The same test starts a following turn in the same thread and proves `_auto_router` runs again. |
| Product boundary | No Reasonix auto-plan setting, local/project config root, desktop controller API, bridge/settings/provider default change, Go/Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Reasonix public auto-plan config, project/local auto-plan overrides,
Reasonix controller API, full auto-plan parity, default Go backend,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto model" --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is loop-level cache/currentness proof. It does not add or validate a
desktop auto-plan setting, packaged desktop controller behavior, or live
provider/cache cost superiority.
```

## 2026-06-21 - Provider Request-Shape Oracle Matrix

Goal:

```text
Move provider request URL/header/body invariants into the provider-cache oracle
so cache proof covers request-shape non-regression as well as usage telemetry.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider request-shape oracle | `provider-cache-oracle.json` records DeepSeek chat, OpenAI-compatible chat, Responses, Anthropic Messages, and custom Responses full endpoint request-shape cases. |
| Executable TS proof | `provider-cache-proof.test.ts` executes each case with `CompatModelClient`, mocked fetch, and URL/header/body/tool-shape assertions. |
| G3/G5 shadow binding | `go-g3-provider-streaming-usage-cache-oracle.json`, `go-g5-full-loop-oracle.json`, TS schemas/tests, and Go shadow output carry request-shape case ids. |
| Product boundary | No Reasonix provider protocol, settings schema/provider default change, bridge, Go provider client, default Go backend, Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry. |

Skipped:

```text
Credentialed live provider matrix, live provider/cache superiority, cost
reconciliation, Go provider client, default Go backend, Rust/Tauri rewrite, and
release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-backed local request-shape proof. It does not exercise live
provider credentials, live network behavior, or live cache/cost superiority.
```

## 2026-06-21 - Preload Bridge/API Sovereignty Oracle

Goal:

```text
Promote the renderer bridge ownership rule from static scans into executable
tests without importing Kun or Reasonix public frontend protocols.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Preload bridge ownership | `preload-sandbox.test.ts` parses `contextBridge.exposeInMainWorld(...)` calls and proves the only exposed name is `analytix`. |
| Renderer Window type ownership | The same test parses `src/preload/index.d.ts` and proves `Window` declares only `analytix`. |
| Shared API name ownership | The same test proves `AnalytixApi` remains exported while `KunApi`, `KunGuiApi`, `ReasonixApi`, `ReasonixSessionAPI`, `DeepSeekApi`, and `AnalytixGuiApi` are not exported public API names. |
| Product boundary | No Reasonix SessionAPI, Kun/deprecated bridge alias, old settings fallback, provider default, Go backend, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Reasonix public frontend protocol, Kun/deprecated GUI bridge aliases, desktop
packaged QA, live runtime/provider parity, default Go backend, Rust/Tauri
rewrite, and release readiness.
```

Validation:

```text
npm run test -- src/preload/preload-sandbox.test.ts
```

Remaining risks:

```text
This is a source-level bridge/API oracle. It does not replace packaged desktop
smoke testing or live runtime/provider validation.
```

## 2026-06-21 - Renderer Thread Lifecycle HTTP Oracle

Goal:

```text
Strengthen the renderer-side proof that thread lifecycle operations stay on the
analytix HTTP runtime contract rather than adopting upstream public protocols.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Thread list/search filters | `analytix-runtime.test.ts` proves `listThreads({ search, includeArchived, archivedOnly, limit })` calls `/v1/threads?limit=25&search=archive+match&include_archived=true&archived_only=true`. |
| Thread lifecycle mutations | The same test proves archive, restore, rename, workspace update, and delete call `/v1/threads/:id` with `PATCH`/`DELETE` and analytix-owned JSON bodies. |
| Existing lifecycle neighbors | Existing tests already cover `/v1/threads/:id/fork`, `/v1/sessions/:id/resume-thread`, and runtime SSE subscription dispatch. |
| Product boundary | No Reasonix SessionAPI, Kun/deprecated bridge alias, old settings fallback, provider default, Go backend, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Packaged desktop thread-list QA, Reasonix public thread/session protocol,
Kun public protocol, live Go route handling, default Go backend, Rust/Tauri
rewrite, and release readiness.
```

Validation:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

Remaining risks:

```text
This is renderer adapter proof with mocked `window.analytix.runtime`. Runtime
server route conformance and Go shadow replay remain covered by their own
oracles; packaged desktop QA is still separate.
```

## 2026-06-21 - Settings/Provider EndpointFormat Persistence Oracle

Goal:

```text
Close the settings half of provider request-shape proof: endpoint-format
choices must persist in analytix-owned settings without writing legacy agent
settings envelopes.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime endpoint format | `settings-store.test.ts` proves a patched top-level `runtime.endpointFormat` survives disk persistence and reload. |
| Custom provider endpoint format | The same test proves a custom provider profile with `endpointFormat: "custom_endpoint"` and full `/messages` base URL persists under `provider.providers`. |
| Legacy envelope exclusion | The persisted `analytix-settings.json` has no top-level `agentProvider` or `agents` fields after the endpoint-format round trip. |
| Product boundary | No Reasonix config root, Kun public settings schema, provider default change, bridge alias, default Go backend, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Credentialed live provider matrix, live provider/cache superiority, Go provider
client, Reasonix config protocol, Kun settings identity, default Go backend,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm run test -- src/main/settings-store.test.ts
```

Remaining risks:

```text
This is settings persistence proof. Runtime request-shape and cache behavior
remain covered by provider oracles; live credentials and live cache/cost
comparisons are still separate release blockers.
```

## 2026-06-21 - Runtime Provider-Selection Request-Shape Oracle

Goal:

```text
Close the runtime dispatch half of provider request-shape proof: a thread's
provider selection must feed the correct analytix-owned model client without
exposing Reasonix provider protocols or mutating default provider behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Thread-selected custom provider | `multi-provider-model-client.test.ts` proves a thread with `providerId: "custom-messages"` uses the exact configured custom full `/messages` URL and Messages request shape. |
| Header/body shape | The same test proves the selected provider carries Messages headers, `model`, `system`, and `messages` fields without OpenAI Responses `input` or chat `stream_options`. |
| Missing provider fallback | The same test proves a removed/missing thread provider falls back to the default OpenAI-compatible `/chat/completions` URL with bearer auth and chat body shape. |
| Diagnostics boundary | `diagnosticsForRequest` reports provider id, base URL, endpoint format, and configured model without adopting Reasonix public provider protocol names. |
| Product boundary | No Reasonix config root, Kun public settings schema, provider default change, bridge alias, default Go backend, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Credentialed live provider matrix, live provider/cache superiority, Go provider
client, Reasonix public provider protocol, Kun settings identity, default Go
backend, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- src/adapters/model/multi-provider-model-client.test.ts
```

Remaining risks:

```text
This is offline runtime model-dispatch proof with fake fetch. Live provider
credentials, live cache/cost deltas, and packaged desktop settings UI QA remain
separate release blockers.
```

## 2026-06-21 - Scheduled Detector Custom Endpoint Inference Oracle

Goal:

```text
Close another provider consumer path: scheduled reminder detection must infer
the request/response family from custom full endpoint URLs without appending a
second provider path or adopting upstream public protocols.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Custom `/messages` endpoint | `claw-scheduled-task-detector.test.ts` proves a full `/messages` URL is used exactly, carries Messages headers/body shape, and omits Responses `input`. |
| Custom `/chat/completions` endpoint | The same test proves a full `/chat/completions` URL is used exactly with chat body shape and JSON response format. |
| Parser family | The same fake-fetch responses prove detector parsing follows the inferred Messages or chat-completions family. |
| Product boundary | No Reasonix config root, Kun public settings schema, provider default change, bridge alias, default Go backend, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Credentialed live scheduled-task provider matrix, live provider/cache
superiority, Go scheduled-task detector, Reasonix public provider protocol, Kun
settings identity, default Go backend, Rust/Tauri rewrite, and release
readiness.
```

Validation:

```text
npm run test -- src/main/claw-scheduled-task-detector.test.ts
```

Remaining risks:

```text
This is main-process fake-fetch evidence. Live scheduled-task provider
credentials and packaged desktop schedule QA remain separate release blockers.
```

## 2026-06-21 - Forbidden Public Runtime Route Oracle

Goal:

```text
Convert the product-sovereignty forbidden-surface rules into runtime HTTP
negative route evidence, not only static scans or renderer route tests.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Reasonix protocol rejection | `http-server.test.ts` proves `/v1/reasonix/sessions` and `/v1/reasonix/threads/:id` return structured 404s. |
| Go route gating | The same test proves `/v1/runtime/go` and `/v1/runtime/go/health` are absent while Go remains shadow-only. |
| Top-level hidden capability rejection | The same test proves `/v1/workflow`, `/v1/workflows`, `/v1/create-loop`, `/v1/subagents`, `/v1/autoresearch`, and `/v1/mcp-indexer` return 404. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, renderer-visible Go route, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Live Go backend routing, Reasonix SessionAPI parity, Kun public protocol,
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product routes,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts
```

Remaining risks:

```text
This is HTTP router negative evidence. It does not replace packaged desktop QA
or future review for any intentional route additions.
```

## 2026-06-21 - Rehydrated Task-Job Route Oracle

Goal:

```text
Close the route-level restart continuity gap for internal task/sub-agent jobs:
after a restart rehydrates running/queued jobs, authenticated analytix runtime
routes must still output, wait, and kill them.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Rehydrated output route | `task-job-orchestration-oracle.test.ts` now wires the restarted `DurableTaskJobManager` into the HTTP harness and reads combined pre/post-restart output through `/v1/runtime/task-jobs/output`. |
| Rehydrated wait route | The same test polls `/v1/runtime/task-jobs/wait` until the rehydrated running job reaches the oracle completion status. |
| Rehydrated kill route | The same test kills a rehydrated queued job through `/v1/runtime/task-jobs/kill` and verifies wait returns the killed status/error. |
| Product boundary | Routes remain authenticated internal runtime routes; no Reasonix public protocol, Subagent top-level route, default Go backend, renderer-visible Go route, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Reasonix public sub-agent/job protocol, top-level Subagent or Workflow route,
live child-agent execution after desktop restart, default Go backend, Go route
serving, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Remaining risks:

```text
This is runtime HTTP harness evidence with fake durable jobs. Packaged desktop
restart QA and live sub-agent execution remain separate release blockers.
```

## 2026-06-21 - Task-Job Approval Deny No-Execute Oracle

Goal:

```text
Close the task/sub-agent approval safety gap: real `task` and `parallel_tasks`
providers must stop at the approval gate and create no durable jobs or child
runs when the GUI denies execution.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| `task` denial | `task-job-orchestration-oracle.test.ts` executes the real task-job provider with `awaitApproval: deny` and receives an approval item for `task` without a tool result. |
| `parallel_tasks` denial | The same test executes the real parallel task provider with `awaitApproval: deny` and receives an approval item for `parallel_tasks`. |
| No durable side effects | The same test verifies `DurableTaskJobManager.list()` returns no jobs and `DelegationRuntime.diagnostics()` reports no child runs. |
| Product boundary | This stays inside the existing analytix permission gate and internal runtime contracts; no Reasonix public protocol, top-level Subagent route, default Go backend, renderer-visible Go route, Rust/Tauri path, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is added. |

Skipped:

```text
Live renderer approval-card click QA, packaged desktop child-run denial QA,
Reasonix public sub-agent/job protocol, default Go backend, Go approval manager,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Remaining risks:

```text
This is runtime tool-host evidence. It does not replace packaged desktop GUI
approval-card QA.
```

## 2026-06-21 - Go G5 Task-Job Approval Deny Shadow Replay

Goal:

```text
Make Go G5 shadow replay consume the same TS-owned approval deny no-execute
evidence as the task-job oracle, without implementing or exposing a Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `task-job-orchestration-oracle.json` records denied tool names, approval ids, `createsDurableJobs: false`, and `createsChildRuns: false`. |
| TS conformance binding | `runtime-parity-fixtures.ts`, `go-g5-full-loop-oracle.json`, and `go-runtime-conformance.test.ts` require `jobReplay.approvalDenyNoExecute` to match the task-job oracle. |
| Go shadow output | `packages/runtime-go/shadow_g5.go` reads the source fixture and emits `approvalDenyNoExecute` from `BuildG5ShadowSlicesOutput`. |
| Product boundary | This remains shadow-only; no Go approval manager, Go job manager, Go HTTP route, default Go backend, renderer-visible Go route, Reasonix public protocol, Rust/Tauri path, or top-level hidden capability entry is added. |

Skipped:

```text
Live Go approval/job manager, Go loop, Electron integration, G6 rollback,
packaged desktop approval-card QA, Reasonix public sub-agent/job protocol,
default Go backend, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow replay of the fixture. It does not run live Go jobs
or replace the TypeScript runtime permission gate.
```

## 2026-06-21 - Go G5 Task Approval Deny Executable Control

Goal:

```text
Move task approval deny evidence from replay-only into a pure Go G5 executable
control calculation while keeping TypeScript runtime contracts authoritative.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Executable input | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.approvalDeny` with attempted tool names, approval ids, and expected no-side-effect output. |
| TS source binding | `go-runtime-conformance.test.ts` derives the executable case from `task-job-orchestration-oracle.json` `approvalDenyNoExecute`. |
| Go calculation | `packages/runtime-go/shadow_g5.go` adds `replayG5ApprovalDeny`, returning denied tool names, approval ids/count, and no durable jobs or child runs. |
| Go assertion | `packages/runtime-go/shadow_test.go` compares the executable output with the TS-owned oracle expected output. |
| Product boundary | This remains shadow-only; no Go approval manager, Go job manager, Go HTTP route, default Go backend, renderer-visible Go route, Reasonix public protocol, Rust/Tauri path, or top-level hidden capability entry is added. |

Skipped:

```text
Live Go approval/job manager, Go loop, Electron integration, G6 rollback,
packaged desktop approval-card QA, Reasonix public sub-agent/job protocol,
default Go backend, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is pure executable shadow control. It does not run live Go jobs or replace
the TypeScript runtime permission gate.
```

## 2026-06-21 - Go G3/G5 Provider Cache Accounting Shadow

Goal:

```text
Move provider cache accounting from TypeScript-only fixture proof into pure
Go G3/G5 shadow calculations without using live provider credentials.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `go-g3-provider-streaming-usage-cache-oracle.json` and `go-g5-full-loop-oracle.json` add `cacheAccounting` for supported telemetry cases, unsupported unknown cases, provider-family case ids, hit/miss totals, aggregate hit rate, and the no-unsupported-miss invariant. |
| TS source binding | `go-runtime-g3-g4-conformance.test.ts` derives `cacheAccounting` from `provider-cache-oracle.json` `providerUsageCases`. |
| G5 source binding | `go-runtime-conformance.test.ts` requires G5 `cacheReplay.cacheAccounting` to match the same provider-cache oracle. |
| Go calculation | `packages/runtime-go/shadow_g3g4.go` and `shadow_g5.go` compute accounting output from `providerUsageMatrix.expectedUsage` / provider-cache oracle cases. |
| Provider coverage | DeepSeek native prompt cache precedence, OpenAI Responses cached tokens, Anthropic cache fields, and unsupported OpenAI-compatible unknown semantics stay in one oracle family. |
| Product boundary | This remains shadow/fake-fixture proof; no live provider credential matrix, Reasonix provider protocol, default Go backend, renderer-visible Go route, Kun/deprecated settings identity, Rust/Tauri path, or top-level hidden capability entry is added. |

Skipped:

```text
Live DeepSeek/OpenAI/Anthropic/custom provider superiority, credentialed
provider matrix, packaged provider settings QA, default Go backend, Rust/Tauri
rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline fixture and fake-provider evidence. It does not replace live
credentialed provider/cache QA.
```

## 2026-06-21 - Auto-Router Failure Usage Isolation

Goal:

```text
Prove the post-881 classifier path can fail and fall back without recording its
own usage/cache telemetry as main turn usage.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Fallback | `loop.test.ts` now proves `_auto_router` failure falls back to heuristic `deepseek-v4-flash`. |
| Router request isolation | The same test asserts the router request has `tools: []` and `prefix: []`. |
| Usage/cache isolation | The same test sends router usage/cache telemetry before failure and asserts no thread `usage` event is recorded and `UsageService` remains zero. |
| Product boundary | No Reasonix `config auto-plan`, project/local auto-plan override, Reasonix public protocol, default Go backend, renderer-visible Go route, or top-level hidden capability entry is added. |

Skipped:

```text
User-visible auto-plan settings, Reasonix CLI/config compatibility, default Go
backend, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t 'falls back to a concrete heuristic model without recording router usage'
npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This proves runtime-loop isolation for classifier failure. Main-process
provider/settings rebuild currentness is covered by the later Managed Runtime
Provider Currentness And Identity Guard entry; this entry still does not
implement a Reasonix controller rebuild protocol or user-visible auto-plan
setting.
```

## 2026-06-21 - Task-Job Stale Restart Reconciliation Oracle

Goal:

```text
Prove runtime restart reconciliation handles both queued and running stale task
jobs, and mirror the contract in Go G5 executable shadow without exposing a
public upstream job protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `task-job-orchestration-oracle.json` adds `durableRunner.staleReconcile` for running id, queued id, reason, expected failed status, and reconciled count. |
| TS runtime test | `task-job-orchestration-oracle.test.ts` writes queued + running stale task-job records and asserts both reconcile to `failed`. |
| G5 source binding | `go-runtime-conformance.test.ts` binds `controlExecutableCases.taskJobs.staleReconcile` to the task-job oracle. |
| Go calculation | `packages/runtime-go/shadow_g5.go` computes stale task jobs -> failed and `shadow_test.go` compares to TS-owned oracle. |
| Product boundary | This remains runtime/oracle evidence; no Reasonix public job protocol, SessionAPI, default Go backend, renderer-visible Go route, Rust/Tauri path, Kun identity, deprecated bridge/settings fallback, or top-level hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Stale queued/running restart reconciliation | contract-reimplement | Accepted into analytix-owned task-job oracle and runtime test. |
| Go G5 executable restart shadow | code-port-and-adapt | Accepted as shadow-only pure calculation. |
| Reasonix public job/session protocol | reject | Do not expose public upstream route/API names. |
| Live Go Job Manager/default backend | defer / reject | Do not promote before G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline runtime/oracle and Go shadow evidence. It does not replace
packaged desktop restart QA, live Go task/job execution, or G6 rollback proof.
```

## 2026-06-21 - MCP Annotation Approval No-Execute Oracle

Goal:

```text
Prove destructive/openWorld MCP tool annotations feed analytix approval gating
and denied approval prevents MCP client execution.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `mcp-tool-lifecycle-oracle.json` adds `approvalAnnotations` for source server/tool, normalized tool name, annotations, approval id, deny decision, approval result kind, and no-execute flag. |
| TS lifecycle test | `mcp-tool-lifecycle-oracle.test.ts` builds a fake MCP client, denies the annotated tool approval, and asserts `callTool` is never invoked. |
| G4 source binding | `go-runtime-g3-g4-conformance.test.ts` binds G4 `mcp.approvalAnnotations` and expected output to the MCP lifecycle oracle. |
| Go calculation | `packages/runtime-go/shadow_g3g4.go` computes `mcpApprovalAnnotatedNoExecute` from fixture decision/executed fields. |
| Product boundary | This remains runtime/oracle evidence; no Reasonix MCP public protocol, MCP-indexer top-level entry, default Go backend, renderer-visible Go route, Rust/Tauri path, Kun identity, deprecated bridge/settings fallback, or live MCP credential matrix is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP annotation-driven approval gate | contract-reimplement | Accepted into analytix-owned MCP lifecycle oracle and runtime test. |
| Go G4 MCP no-execute shadow | code-port-and-adapt | Accepted as fixture-only pure calculation. |
| Reasonix MCP public protocol / MCP-indexer entry | reject | Do not expose public upstream route/API names or top-level entry. |
| Live MCP credential matrix | defer | Keep this as fake-client lifecycle proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline fake-client lifecycle and Go shadow evidence. It does not
replace packaged desktop approval QA or credentialed MCP server compatibility.
```

## 2026-06-21 - User-Input Submitted Route Oracle

Goal:

```text
Prove submitted user-input answers are delivered through analytix HTTP/gate
resolution while SSE replay keeps answers out of event history.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `approval-user-input-route-oracle.json` adds `submittedUserInput` with answer request, expected submitted resolution, expected HTTP response, and answer-free resolved event boundary. |
| TS route test | `approval-user-input-route-oracle.test.ts` submits answers to `/v1/user-inputs/:id`, asserts response/gate resolution echo answers, and asserts SSE replay omits answers. |
| G4 source binding | `go-runtime-g3-g4-conformance.test.ts` binds G4 `submittedRoute` to the approval/user-input oracle. |
| Go calculation | `packages/runtime-go/shadow_g3g4.go` computes `userInputSubmittedAnswersEchoed` and `userInputResolvedEventOmitsAnswers`. |
| Product boundary | This remains runtime/oracle evidence; no Reasonix public user-input protocol, default Go backend, renderer-visible Go route, Rust/Tauri path, Kun identity, deprecated bridge/settings fallback, or top-level hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Submitted answer HTTP/gate route | contract-reimplement | Accepted into analytix-owned user-input route oracle and test. |
| Answer-free SSE replay boundary | contract-reimplement | Accepted as runtime privacy/history boundary. |
| Go G4 submitted-route shadow | code-port-and-adapt | Accepted as fixture-only pure calculation. |
| Reasonix public user-input protocol | reject | Do not expose public upstream route/API names. |
| Packaged GUI live-card QA | defer | Keep this as route/oracle proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline HTTP/SSE route and Go shadow evidence. It does not replace
packaged GUI live-card QA or live cross-device delivery checks.
```

## 2026-06-21 - Task-Job Route Auth Matrix Oracle

Goal:

```text
Prove task-job wait/output/kill remain authenticated internal runtime routes
instead of public upstream job control APIs.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source fixture | `task-job-orchestration-oracle.json` adds `routeContract.authMatrix` for wait/output/kill and unauthorized 401. |
| TS route test | `task-job-orchestration-oracle.test.ts` checks wait/output/kill all reject missing runtime tokens. |
| Product boundary | No Reasonix public job/session protocol, default Go backend, renderer-visible Go route, Rust/Tauri path, Kun identity, deprecated bridge/settings fallback, or top-level hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Task-job route auth matrix | contract-reimplement | Accepted into analytix-owned task-job oracle and route test. |
| Reasonix public job protocol | reject | Do not expose public upstream route/API names. |
| Go route auth parity | defer | No Go task-job route exists before G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Remaining risks:

```text
This is offline HTTP route evidence. It does not replace packaged desktop route
QA or any future Go route auth proof after G5/G6 gate work.
```

## 2026-06-21 - Connect Phone Product Copy Sovereignty

Goal:

```text
Prove Connect Phone user-visible and model-visible natural-language copy no
longer leaks the internal Claw implementation name.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Renderer locale guard | `src/renderer/src/locales/connect-phone-product-copy.test.ts` scans English/Chinese common/settings locale values for standalone `Claw`. |
| Runtime reply copy | `src/main/claw-runtime.ts` returns Connect Phone wording for IM help/model replies, retired task reply, and disabled webhook response. |
| IPC error copy | `src/main/ipc/register-app-ipc-handlers.ts` uses Connect Phone wording for mirror runtime initialization failures. |
| Prompt copy | `src/shared/app-settings-prompts.ts` emits `Connect Phone agent` and `Connect Phone skill policy` natural-language hints while preserving old marker unwrap. |
| Product boundary | Internal `claw` type/schema/IPC/setting names, prompt markers, and thread recovery remain compatibility internals; no Kun identity, Reasonix protocol, default Go backend, Rust/Tauri path, or new top-level hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Connect Phone locale/user copy | contract-reimplement | Accepted with renderer locale scan. |
| IM/runtime reply copy | contract-reimplement | Accepted in main runtime tests. |
| Prompt natural-language copy | contract-reimplement | Accepted in shared prompt tests with old/new unwrap compatibility. |
| Internal `claw` compatibility names | document-only | Keep as allowed internals; do not rename in this batch. |
| Kun/Reasonix public identity/protocol | reject | Do not expose upstream public names or routes. |

Validation:

```text
npm run test -- src/main/claw-runtime.test.ts src/shared/app-settings.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts
```

Remaining risks:

```text
This is product-copy and unit-test evidence. It does not replace live Connect
Phone desktop/remote QA, and it does not rename internal `claw` contracts.
```

## 2026-06-21 - Managed Runtime Provider Currentness And Identity Guard

Goal:

```text
Prove selected provider endpoint/model-profile currentness participates in the
managed runtime rebuild boundary and child provider snapshot, while model-visible
and public API identity remains analytix-owned.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Shared runtime key | `src/shared/app-settings-runtime.ts` exports `buildAnalytixRuntimeSettingsKey` and `analytixRuntimeSettingsEqual` over `resolveAnalytixRuntimeSettings`. |
| Main runtime apply boundary | `src/main/index.ts` uses the shared key for `ensureRuntime` fingerprinting and settings-apply runtime-change detection. |
| Provider drift proof | `src/shared/app-settings-provider.test.ts` proves UI-only changes do not affect the key, while selected provider endpoint/profile drift does. |
| Managed env proof | `src/main/analytix-process.test.ts` proves `ANALYTIX_MODEL_PROVIDERS` refreshes base URL, endpoint format, context window, and reasoning protocol drift. |
| Model-visible identity | `packages/runtime/tests/analytix-system-prompt.test.ts` forbids `Claw/Kun/Reasonix` in `ANALYTIX_SYSTEM_PROMPT`. |
| Bridge/settings fallback | `src/preload/preload-sandbox.test.ts` rejects upstream public facade domains, and `src/main/settings-store.test.ts` rejects `agentProvider: "kun"` / `agents.kun` fallback persistence. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Reasonix classifier/config rebuild currentness | contract-reimplement | Accepted as analytix runtime settings key and managed provider snapshot proof. |
| Provider endpoint/profile currentness | contract-reimplement | Accepted in shared/main tests without changing provider defaults. |
| Model-visible Connect Phone identity | contract-reimplement | Accepted for system prompt natural language. |
| Kun legacy `agents.kun` fallback | reject | Do not migrate or persist as active runtime settings. |
| Reasonix public auto-plan config/controller | reject / defer | Do not expose; future setting must be analytix top-level `runtime` only. |
| Live credentialed provider/cache matrix | defer | Keep offline proof only. |

Validation:

```text
npm --prefix packages/runtime test -- tests/analytix-system-prompt.test.ts tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This closes provider/config currentness for managed runtime rebuild boundaries.
It does not add user-visible auto-plan settings, live provider credential QA,
internal `claw` contract renames, default Go backend, or packaged restart QA.
```

## 2026-06-21 - Provider Endpoint URL Builder Parity

Goal:

```text
Make auxiliary provider consumers share endpoint URL construction so versioned
responses/messages bases and known endpoint paths do not duplicate suffixes.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Shared helper | `src/shared/openai-compat-url.ts` adds `upstreamOpenAiModelEndpointUrl` for responses/messages/custom endpoint URL construction. |
| Shared tests | `src/shared/openai-compat-url.test.ts` covers `/v2`, `/v3`, `/beta`, query strings, and known endpoint path stripping. |
| Scheduled detector | `src/main/claw-scheduled-task-detector.ts` uses the shared helper; tests cover versioned responses/messages fake-fetch requests. |
| Write-inline | `src/main/services/write-inline-completion-service.ts` uses the shared helper; focused write-inline tests guard custom full endpoints. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Versioned endpoint base handling | code-port-and-adapt | Accepted in shared helper and scheduled detector tests. |
| Known endpoint path stripping | contract-reimplement | Accepted for responses/messages auxiliary consumers. |
| Write-inline helper consolidation | contract-reimplement | Accepted as non-regression cleanup. |
| Reasonix provider protocol | reject | Do not copy upstream protocol/config shape. |
| Live credentialed provider matrix | defer | Keep this as fake-fetch/unit proof. |

Validation:

```text
npm run test -- src/shared/openai-compat-url.test.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is shared helper and fake-fetch evidence. It does not replace live
provider/cache credentialed QA, packaged Schedule/Write QA, or release readiness.
```

## 2026-06-22 - Go G5 User-Input Gate Executable Shadow

Goal:

```text
Move submitted/cancelled user-input route semantics into G5 executable shadow
without exposing Reasonix ask/session protocol or a live Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.userInput`, `controlExecutableCases.userInput`, and `shadowSlicesExpectedOutput.approvalUserInputReplay`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records submitted answer echo, replay answer omission, cancelled status, late resolve rejection, and pending-after counts. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives the G5 user-input executable case from `approval-user-input-route-api-gates-v1`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `replayG5UserInput` and source-bound approval/user-input replay output; `shadow_test.go` compares it with the oracle. |
| Product boundary | No Reasonix ask protocol, SessionAPI, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or top-level hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Submitted user-input gate result | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| Replay answer omission | contract-reimplement | Accepted as analytix-owned privacy invariant. |
| Cancelled input late resolve rejection | contract-reimplement | Accepted in G5 executable case and Go output. |
| Reasonix ask/session public protocol | reject | Do not expose upstream route/API names. |
| Live Go user-input manager | defer | Keep TypeScript runtime authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace packaged renderer
live-card QA, live Go user-input routing, G6 rollback, or release readiness.
```

## 2026-06-22 - Go G5 AutoResearch Project-Local State Shadow

Goal:

```text
Move AutoResearch project-local state and requirement-audit boundaries into G5
executable shadow without adding a top-level AutoResearch product surface.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.autoResearch` and `controlExecutableCases.autoResearch`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records project-local state path, expected files, unknown requirement id, and no-public-file/no-route expected output. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` creates real `AutoResearchProjectStore` state and verifies unknown requirement rejection without findings writes. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `replayG5AutoResearch`; `shadow_test.go` compares it with the oracle. |
| Product boundary | No top-level AutoResearch route, Reasonix project protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Project-local research state | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| Requirement audit rejection | contract-reimplement | Accepted through real TS store proof and G5 expected output. |
| Stable prefix/tool schema isolation | contract-reimplement | Accepted as G5 shadow invariant. |
| Reasonix public AutoResearch/project protocol | reject | Do not expose upstream route/API names or top-level navigation. |
| Live Go AutoResearch manager | defer | Keep TypeScript runtime authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/autoresearch-store.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace packaged
long-running task QA, live Go AutoResearch state, G6 rollback, or release
readiness.
```

## 2026-06-22 - Go G5 MCP Lifecycle Executable Shadow

Goal:

```text
Move MCP retry, tombstone/resume, and redaction semantics into G5 executable
shadow without exposing MCP-indexer or Reasonix MCP protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.mcpLifecycle` and `controlExecutableCases.mcpLifecycle`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records failed server ids, connected/error ids, attempts, live-local paths, tombstone, resume, and redaction expected output. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives the G5 case from `mcp-tool-lifecycle-oracle.json` and `runLiveLocalIndexerProof`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `replayG5MCPLifecycle`; `shadow_test.go` compares it with the oracle. |
| Product boundary | No MCP-indexer top-level route, Reasonix MCP protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Retry-all failed MCP startup servers | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| Live-local tombstone/resume | code-port-and-adapt | Accepted through TS live-local proof and Go shadow output. |
| Secret-safe diagnostics | contract-reimplement | Accepted as `leaksSecret:false` G5 invariant. |
| Reasonix MCP public protocol / MCP-indexer UI | reject | Do not expose upstream route/API names or top-level navigation. |
| Live credentialed MCP matrix / Go MCP client | defer | Keep fake/local fixtures and TypeScript runtime authority until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-lifecycle-oracle.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace credentialed MCP
server compatibility, packaged desktop MCP QA, live Go MCP client behavior, G6
rollback, or release readiness.
```

## 2026-06-22 - D-0060 Go G5 Checkpoint/Rewind Executable Shadow

Goal:

```text
Move checkpoint/rewind safety into G5 executable shadow without adding a Go
checkpoint route, Kun git-ref checkpoint contract, or Reasonix checkpoint
protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.checkpointRewind` and `controlExecutableCases.checkpointRewind`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records ready/blocked files, path escape blocking, symlink blocking, legal `..name` handling, and append-only conversation audit. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives the case from the existing analytix checkpoint/rewind oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes id-prefix boundaries, explicit confirmation, no transcript rewrite, no git refs, no Reasonix protocol, and no public route. |
| Product boundary | No Reasonix checkpoint/rewind protocol, Kun git-ref checkpoint behavior, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Checkpoint/rewind executable safety | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| Analytix-owned id/path/audit boundaries | contract-reimplement | Accepted through existing TS oracle plus G5 expected output. |
| Reasonix checkpoint/rewind public protocol | reject | Do not expose upstream route/API names. |
| Kun git-ref checkpoint contract | reject | Keep analytix `axcp_` / `axrp_` / `axra_` / `axrr_` evidence ids authoritative. |
| Live Go checkpoint store or route | defer | Keep TypeScript runtime authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace packaged desktop
rewind QA, live Go file mutation, G6 rollback, or release readiness.
```

## 2026-06-22 - D-0061 Go G5 Remote-Entry Boundary Executable Shadow

Goal:

```text
Move remote-entry control-port boundaries into G5 executable shadow without
exposing Reasonix SessionAPI/control protocol or a renderer-visible Go route.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.remoteEntry` and `controlExecutableCases.remoteEntry`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records allowed control ports, forbidden control planes, rejected policy override, no Reasonix protocol, and no top-level route. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives the case from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the narrow remote-entry output without a live Go HTTP route. |
| Product boundary | No Reasonix SessionAPI/control protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Remote-entry narrow control surface | contract-reimplement | Accepted as analytix-owned lifecycle/turn/approval/user-input port proof. |
| G5 executable shadow output | code-port-and-adapt | Accepted as pure Go shadow calculations against TS fixture. |
| Reasonix SessionAPI/control public protocol | reject | Do not expose upstream route/API names or public protocol fields. |
| Goal/checkpoint/memory/storage/tool-host access from remote entries | reject | Keep these planes absent unless a future analytix contract explicitly grants them. |
| Live Go remote-entry route | defer | Keep TypeScript runtime control ports authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace packaged
remote-entry QA, live Go turn control, G6 rollback, or release readiness.
```

## 2026-06-22 - D-0062 Go G5 Resume Pending Gates Executable Shadow

Goal:

```text
Move pending approval/user-input resume safety into G5 executable shadow without
exposing Reasonix ask/session protocol or a live Go gate manager.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `controlReplay.resumePendingGates` and `controlExecutableCases.resumePendingGates`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records source pending approval/user-input statuses, resumed expired/cancelled statuses, no pending gates after resume, and no copied answers. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives the case from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `replayG5ResumePendingGates`; `shadow_test.go` compares it with the oracle. |
| Product boundary | No Reasonix ask/session protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Resume pending gate executable safety | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| Source/resumed gate status derivation | contract-reimplement | Accepted from the analytix approval/user-input route oracle. |
| Reasonix ask/session public protocol | reject | Do not expose upstream route/API names or SessionAPI fields. |
| Actionable resumed gates / copied answers | reject | Resumed approval is expired, resumed user input is cancelled, and answers are not copied. |
| Live Go resume/gate managers | defer | Keep TypeScript runtime session resume and gate managers authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow-only executable evidence. It does not replace packaged renderer
resume QA, live Go approval/user-input routing, G6 rollback, or release
readiness.
```

## 2026-06-22 - D-0063 Go G5 Provider Cache Release Guard Executable Shadow

Goal:

```text
Move the offline provider cache release guard into G5 executable shadow without
turning it into a live provider superiority claim or default Go backend signal.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 schema | `packages/runtime/src/conformance/runtime-parity-fixtures.ts` requires `cacheReplay.releaseGuard`. |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` records release guard tail averages, statuses, collapse counts, low-tail allowance, and overall pass. |
| TS conformance | `packages/runtime/tests/go-runtime-conformance.test.ts` derives expected output with `evaluateOfflineCacheCurveGuard(providerCache.releaseGuard)`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same release guard output from `provider-cache-oracle.json`; `shadow_test.go` compares it with the oracle. |
| Product boundary | No live provider superiority claim, Reasonix provider protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Offline cache curve release guard | code-port-and-adapt | Accepted as Go G5 pure executable shadow. |
| TS cache guard as source of truth | contract-reimplement | Expected output is derived from analytix `evaluateOfflineCacheCurveGuard`, not Reasonix public provider protocol. |
| Live DeepSeek/OpenAI/Anthropic superiority claim | reject | Fixture/shadow evidence cannot claim live provider/cache superiority. |
| Default Go backend signal | reject | G5 remains shadow-only and cannot route renderer/main traffic to Go. |
| Credentialed provider matrix | defer | Live provider/cache proof remains a separate QA and release evidence item. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline fixture and shadow-only executable evidence. It does not replace
live provider credentials, cost/cache reconciliation, packaged desktop QA, G6
rollback, or release readiness.
```

## 2026-06-22 - D-0064 Auto-Route Step/Cancel AgentLoop Composition Proof

Goal:

```text
Prove the real TypeScript AgentLoop keeps auto-route cache, step-limit metadata,
and interrupted parallel tool results stable in one scenario.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS AgentLoop proof | `packages/runtime/tests/loop.test.ts` adds a real `model:"auto"` two-step turn that interrupts during a parallel tool batch. |
| Auto-route cache | The test proves `_auto_router` runs once and the selected `deepseek-v4-pro` / `max` route is reused for the second model step. |
| Step-limit hygiene | The same test proves `currentMaxModelSteps` reaches tools while step-limit text stays out of model-visible prefix/context. |
| Cancel/result stability | Completed warmup/first tool results are preserved, and interrupted/not-started calls are recorded as cancelled tool results. |
| Product boundary | No Reasonix auto-plan setting, controller API, SessionAPI, public protocol, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Auto-route cache plus cancel/step composition | contract-reimplement | Accepted as analytix-owned AgentLoop test coverage. |
| Reasonix classifier/controller rebuild semantics | defer | Product auto-plan setting and live controller rebuild remain future design work. |
| Reasonix public controller/SessionAPI | reject | Do not expose upstream route/API names or public protocol fields. |
| Dynamic step state in stable prefix | reject | Step-limit metadata remains runtime/tool context only. |
| Default Go backend | reject | This batch changes TypeScript tests only and does not affect Go routing. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "reuses the auto route across a step before preserving cancelled parallel tool results"
```

Remaining risks:

```text
This is a focused TS AgentLoop proof. It does not implement Reasonix auto-plan
settings, planner enable/disable product controls, live provider/cache
superiority, packaged desktop interruption QA, or release readiness.
```

## 2026-06-22 - D-0065 Planner Step-Limit AgentLoop Proof

Goal:

```text
Prove the real TypeScript AgentLoop applies planner-specific step limits to
plan-mode turns without leaking planner budget state into model-visible prompt
content.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS AgentLoop proof | `packages/runtime/tests/loop.test.ts` adds a real plan-mode turn with `plannerMaxModelSteps: 2` and higher default/user-global limits. |
| Planner gating | The test proves the turn stops after two planner model requests with `turn_step_limit_exceeded` and event details `maxModelSteps: 2`. |
| Tool surface | The same test proves plan-mode requests advertise the analytix `create_plan` tool. |
| Cache/prompt hygiene | The model-visible prefix and context omit planner/step budget text. |
| Product boundary | No Reasonix auto-plan setting, planner product toggle, controller API, SessionAPI, public protocol, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Planner-specific step gating | contract-reimplement | Accepted as analytix-owned AgentLoop test coverage. |
| Planner budget isolation from prompt/cache | contract-reimplement | Accepted as stable-prefix/model-context hygiene proof. |
| Reasonix planner enable/disable product controls | defer | Product planner controls remain future design work. |
| Reasonix controller/SessionAPI protocol | reject | Do not expose upstream route/API names or public protocol fields. |
| Default Go backend | reject | This batch changes TypeScript tests only and does not affect Go routing. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "uses the planner step limit for plan-mode turns"
```

Remaining risks:

```text
This is a focused TS AgentLoop proof. It does not implement Reasonix planner
enable/disable settings, live controller rebuild, packaged desktop plan-mode
QA, Go planner parity, or release readiness.
```

## 2026-06-22 - D-0066 Auto-Router Classifier Request Contract

Goal:

```text
Prove the analytix `_auto_router` classifier remains an isolated short-JSON
side path with timeout fallback and fingerprinted currentness.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Router request shape | `packages/runtime/tests/auto-model-router.test.ts` asserts `_auto_router` uses the DeepSeek flash classifier, compact JSON response mode, `stream:false`, `maxTokens:96`, `temperature:0`, `reasoningEffort:"off"`, `tools: []`, and `prefix: []`. |
| Prompt boundary | The classifier user item wraps selected mode, recent context, and latest request without main-turn context instructions. |
| Timeout fallback | A slow classifier request is aborted by the router timeout and falls back to heuristic `deepseek-v4-pro` / `max`. |
| Fingerprint currentness | Timeout drift changes the classifier fingerprint, preserving route-cache rebuild semantics. |
| Product boundary | No Reasonix auto-plan config, controller/SessionAPI protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Isolated classifier request shape | contract-reimplement | Accepted as analytix-owned auto-router unit coverage. |
| Classifier timeout fallback | contract-reimplement | Accepted as safe heuristic fallback behavior. |
| Reasonix auto-plan config/project override | reject | Do not expose upstream config or settings shape. |
| Product planner/auto-plan toggles | defer | Keep user-visible toggles and live controller rebuild for future design. |
| Default Go backend / Reasonix protocol | reject | This batch changes TypeScript tests/docs only and exposes no new protocol. |

Validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is focused router contract proof. It does not implement product
auto-plan/planner toggles, packaged desktop QA, live provider/cache superiority,
Go auto-router parity, or release readiness.
```

## 2026-06-22 - D-0067 Planner-Executor Transcript Propagation

Goal:

```text
Propagate analytix-owned transcript refs through planner executor durable jobs
without exposing Reasonix sub-agent/session protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Coordinator | `packages/runtime/src/delegation/planner-executor-coordinator.ts` accepts optional `transcriptFor(task)` and writes the returned `TaskJobTranscriptRef` to executor jobs. |
| Runtime oracle | `task-job-orchestration-oracle.test.ts` uses `resolveTranscriptOperation` to create a fork ref, then proves both executor jobs persist it. |
| Fixture/schema | `task-job-orchestration-oracle.json` and `runtime-parity-fixtures.ts` require transcript propagation fields. |
| G5 shadow | `go-g5-full-loop-oracle.json`, `go-runtime-conformance.test.ts`, and `packages/runtime-go/shadow_g5.go` replay the requirement as shadow-only evidence. |
| Product boundary | No Reasonix SessionAPI/job protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Executor transcript ref propagation | code-port-and-adapt | Accepted behind analytix durable job records. |
| Transcript identity guard | contract-reimplement | Accepted through existing `resolveTranscriptOperation`. |
| Reasonix public sub-agent/session protocol | reject | Do not expose upstream protocol or route names. |
| Live Go Job Manager/default backend | reject | G5 remains shadow-only. |
| Packaged desktop sub-agent QA | defer | Keep live UI QA and release readiness separate. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This does not implement a public Subagent product surface, live Go Job Manager,
full Reasonix planner/executor parity, packaged desktop QA, or release
readiness.
```

## 2026-06-22 - D-0068 MCP Search Meta-Tool Trust Boundary

Goal:

```text
Prove MCP search discovery and meta-call tools stay behind workspace trust and
approval gates without exposing an MCP-indexer product surface.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime proof | `packages/runtime/tests/mcp-tool-provider.test.ts` proves trusted search/describe/call behavior and untrusted no-execute behavior. |
| Approval proof | The same test proves denied `mcp_call` does not call the underlying MCP client. |
| Fixture/schema | `mcp-tool-lifecycle-oracle.json` and `runtime-parity-fixtures.ts` record `searchMetaTools`. |
| G4 shadow | `go-g4-tools-approval-user-input-mcp-oracle.json`, `go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go/shadow_g3g4.go` replay advertised/trust/no-execute evidence. |
| Product boundary | No Reasonix MCP protocol, MCP-indexer route, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP search meta-tool trust boundary | contract-reimplement | Accepted behind analytix workspace trust checks. |
| `mcp_call` approval/no-execute | code-port-and-adapt | Accepted behind existing approval gate. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol or route names. |
| Live Go MCP client/default backend | reject | G4/G5 remain shadow/conformance only. |
| Credentialed MCP matrix / packaged QA | defer | Keep live MCP QA and release readiness separate. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-provider.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This does not implement a public MCP-indexer surface, credentialed MCP matrix,
live Go MCP client, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0069 Custom Messages Full Endpoint Request Shape

Goal:

```text
Prove custom full `/messages` endpoints keep exact URL semantics and Anthropic
Messages request shape in the provider/cache oracle.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Fixture | `packages/runtime/src/conformance/fixtures/provider-cache-oracle.json` adds `custom-messages-full-endpoint-request-shape`. |
| Runtime proof | `packages/runtime/tests/provider-cache-proof.test.ts` verifies exact URL, required/forbidden headers/body fields, and Anthropic tool shape. |
| Product boundary | No Reasonix provider protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Custom `/messages` full endpoint | contract-reimplement | Accepted in provider-cache oracle. |
| Anthropic Messages body/header shape | contract-reimplement | Accepted through fake-fetch request-shape proof. |
| OpenAI fallback body on `/messages` | reject | Prevented by required/forbidden field assertions. |
| Live provider matrix | defer | Keep credentialed provider QA separate. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is fixture/fake-fetch request-shape proof. It does not prove live provider
credentials, packaged settings QA, Go provider client readiness, or release
readiness.
```

## 2026-06-22 - D-0194 Main IPC Endpoint Builder Allow-list Proof

Goal:

```text
Prove the main IPC runtime request boundary accepts encoded shared endpoint
builder output and rejects raw compatibility drift without widening the public
desktop contract.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Main IPC schema proof | `src/main/ipc/app-ipc-schemas.test.ts` accepts shared builder output for encoded thread, turn, checkpoint, approval, user-input, session, attachment, and memory paths. |
| Raw path rejection | The same test rejects unencoded extra-segment dynamic ids before runtime adapter calls. |
| Compatibility boundary | Singular `/v1/user-input/:id` remains server compatibility only and is rejected by the main IPC allow-list. |
| Product boundary | No Reasonix route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Encoded builder paths at main IPC | contract-reimplement | Accepted as the desktop bridge/schema contract. |
| Raw dynamic segment injection rejection | contract-reimplement | Accepted as main IPC route safety proof. |
| Singular user-input compatibility route | document-only / reject | Kept server-only; not exposed as shared or bridge-visible contract. |
| Reasonix SessionAPI/public routes | reject | Not imported. |
| Live Go HTTP server or packaged route walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/main/ipc/app-ipc-schemas.test.ts
```

Remaining risks:

```text
This is main IPC schema proof. It does not prove packaged desktop route
walkthrough, live Go HTTP server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0195 Main IPC Runtime Adapter Handoff Proof

Goal:

```text
Prove the registered main IPC runtime request handler preserves encoded
analytix endpoint paths into the runtime adapter and blocks invalid paths
before adapter invocation.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Handler pass-through | `src/main/ipc/register-app-ipc-handlers.test.ts` proves encoded shared builder paths are handed to `runtimeRequest` unchanged. |
| Method/body preservation | The same test checks adapter call order and exact path/method/body arguments. |
| Invalid path no-forward | Raw extra-segment dynamic ids and singular `/v1/user-input/:id` reject before `runtimeRequest` is called. |
| Product boundary | No Reasonix route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Encoded path handler handoff | contract-reimplement | Accepted as the registered IPC-to-adapter contract. |
| Invalid path no-forward | contract-reimplement | Accepted as main IPC side-effect boundary proof. |
| Singular user-input compatibility route | document-only / reject | Kept server-only; not forwarded as bridge-visible contract. |
| Reasonix SessionAPI/public routes | reject | Not imported. |
| Live Go HTTP server or packaged route walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/main/ipc/register-app-ipc-handlers.test.ts
```

Remaining risks:

```text
This is main IPC handler proof. It does not prove packaged desktop route
walkthrough, live Go HTTP server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0196 Preload Runtime Request Bridge Proof

Goal:

```text
Prove the exposed preload `window.analytix` facade preserves encoded runtime
request paths, method, and body into the main IPC `runtime:request` channel.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime facade proof | `src/preload/preload-runtime-request.test.ts` loads the exposed `analytix` API and calls `api.runtime.runtimeRequest`. |
| IPC payload preservation | The test proves encoded shared endpoint paths, method, and body reach `ipcRenderer.invoke('runtime:request', { path, method, body })` unchanged. |
| Diagnostics compatibility | Diagnostics runtime request fallback uses the same analytix IPC channel and payload shape. |
| Product boundary | No Reasonix route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Preload runtime request bridge | contract-reimplement | Accepted as executable `window.analytix` facade proof. |
| Diagnostics runtime request compatibility | contract-reimplement | Accepted only on the same analytix IPC channel. |
| Deprecated bridge aliases | reject | Not exposed. |
| Reasonix SessionAPI/public routes | reject | Not imported. |
| Live Go HTTP server or packaged route walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/preload/preload-runtime-request.test.ts
```

Remaining risks:

```text
This is preload bridge unit proof. It does not prove packaged desktop route
walkthrough, live Go HTTP server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0197 Runtime Host URL Handoff Proof

Goal:

```text
Prove `runtimeRequestViaHost` preserves encoded analytix endpoint paths and
request metadata when forwarding to the managed runtime HTTP host.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime host URL proof | `src/main/runtime/analytix-adapter.test.ts` uses a real local HTTP server to inspect the URL received by the host. |
| Encoded path/query preservation | The test proves encoded thread/turn ids and encoded query values remain encoded in `req.url`. |
| Request metadata preservation | POST method, bearer auth, custom header, JSON content type, and body are preserved. |
| Product boundary | No Reasonix route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Runtime host encoded URL handoff | contract-reimplement | Accepted as the managed `analytix serve` request contract. |
| Runtime host request metadata | contract-reimplement | Accepted as request-shape preservation proof. |
| Reasonix SessionAPI/public routes | reject | Not imported. |
| Default Go backend | reject | Not enabled. |
| Live Go HTTP server or packaged route walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/main/runtime/analytix-adapter.test.ts
```

Remaining risks:

```text
This is runtime adapter unit proof. It does not prove packaged desktop route
walkthrough, live Go HTTP server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0198 Preload SSE Bridge Proof

Goal:

```text
Prove the exposed preload `window.analytix` SSE facade preserves start/stop
arguments and payload-only event delivery on analytix-owned IPC channels.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| SSE start/stop proof | `src/preload/preload-sse-bridge.test.ts` loads the exposed `analytix` API and calls `api.runtime.startSse` / `stopSse`. |
| Listener payload proof | The test triggers event/end/error wrappers and proves renderer handlers receive payloads only. |
| Unsubscribe cleanup | The test proves unsubscribe callbacks remove the exact `runtime:sse-*` listeners. |
| Product boundary | No Reasonix SSE protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Preload SSE start/stop bridge | contract-reimplement | Accepted as executable `window.analytix` SSE facade proof. |
| Payload-only listener delivery | contract-reimplement | Accepted as renderer event boundary proof. |
| Deprecated bridge aliases | reject | Not exposed. |
| Reasonix SessionAPI/public SSE routes | reject | Not imported. |
| Live Go SSE server or packaged walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/preload/preload-sse-bridge.test.ts
```

Remaining risks:

```text
This is preload SSE bridge unit proof. It does not prove packaged desktop SSE
walkthrough, live Go SSE server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0199 Main SSE Host URL Encoding Proof

Goal:

```text
Prove main SSE IPC encodes renderer-provided thread ids and preserves cursor
headers before fetching the managed runtime event stream.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Main SSE URL proof | `src/main/runtime-sse-ipc.test.ts` starts SSE with a thread id containing slash/query/fragment text and inspects the fetch URL. |
| Cursor/header proof | The test proves `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, and bearer auth preservation. |
| Error channel proof | Fatal host failure returns `runtime:sse-error` with the requested stream id. |
| Product boundary | No Reasonix SSE protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Main SSE thread-id encoding | contract-reimplement | Accepted as analytix-owned event route construction proof. |
| SSE cursor/header preservation | contract-reimplement | Accepted as reconnect cursor safety proof. |
| Reasonix SessionAPI/public SSE routes | reject | Not imported. |
| Default Go backend | reject | Not enabled. |
| Live Go SSE server or packaged walkthrough | defer | Keep for G6/release-readiness gates. |

Validation:

```text
npx vitest run src/main/runtime-sse-ipc.test.ts
```

Remaining risks:

```text
This is main SSE IPC unit proof. It does not prove packaged desktop SSE
walkthrough, live Go SSE server behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0200 Renderer Runtime Client Bridge Proof

Goal:

```text
Prove the renderer runtime client preserves runtime request and SSE arguments
through `window.analytix.runtime` without reading legacy bridge aliases.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime request facade proof | `src/renderer/src/agent/runtime-client.test.ts` proves path-only, path+method, and path+method+body calls are forwarded unchanged. |
| SSE facade proof | The same test proves `startSse`, `stopSse`, and SSE listener registrations forward arguments/handlers unchanged. |
| Legacy alias guard | Throwing `window.kun` / `window.reasonix` getters remain unread. |
| Product boundary | No Reasonix bridge protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer runtime request facade | contract-reimplement | Accepted as executable `window.analytix.runtime` proof. |
| Renderer SSE facade | contract-reimplement | Accepted as renderer runtime/SSE argument preservation proof. |
| Deprecated bridge aliases | reject | Not read or exposed. |
| Reasonix SessionAPI/public bridge | reject | Not imported. |
| Packaged runtime/SSE walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

Remaining risks:

```text
This is renderer client unit proof. It does not prove packaged desktop runtime
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0201 Renderer Settings Bridge Proof

Goal:

```text
Prove the renderer settings client preserves top-level runtime settings patches
through `window.analytix.settings` without reading legacy bridge aliases.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Settings facade proof | `src/renderer/src/agent/runtime-client.test.ts` proves `setSettings` forwards a top-level `runtime` patch to `window.analytix.settings.setSettings` unchanged. |
| Runtime settings ownership | The same test keeps `runtime.model` and `runtime.approvalPolicy` under the active top-level `runtime` settings object. |
| Legacy alias guard | Throwing `window.kun` / `window.reasonix` getters remain unread during settings writes. |
| Product boundary | No Reasonix config root, public settings protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer settings facade | contract-reimplement | Accepted as executable `window.analytix.settings` proof. |
| Top-level runtime settings patch | contract-reimplement | Accepted as renderer settings sovereignty proof. |
| Deprecated bridge aliases | reject | Not read or exposed. |
| Reasonix config/public settings protocol | reject | Not imported. |
| Packaged settings walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

Remaining risks:

```text
This is renderer client unit proof. It does not prove packaged desktop settings
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0202 Renderer Runtime Provider Alias Guard

Goal:

```text
Prove the renderer runtime provider keeps legacy bridge aliases unread across
existing thread lifecycle, approval/user-input, fork/resume, and route encoding
tests.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider alias guard | `src/renderer/src/agent/analytix-runtime.test.ts` installs throwing `window.kun` / `window.reasonix` getters in the shared provider bridge helper. |
| Route and interaction coverage | Existing tests cover analytix-owned routes, archive/search lifecycle, approval/user-input submit/cancel, fork/resume, and dynamic route id encoding under the alias guard. |
| Product boundary | No Reasonix bridge protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer provider alias guard | contract-reimplement | Accepted as executable provider-layer bridge proof. |
| Approval/user-input and fork/resume provider paths | contract-reimplement | Accepted under existing analytix route tests. |
| Deprecated bridge aliases | reject | Not read or exposed. |
| Reasonix SessionAPI/public bridge | reject | Not imported. |
| Packaged renderer walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
```

Remaining risks:

```text
This is renderer provider unit proof. It does not prove packaged desktop
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0203 Renderer Provider Runtime Client Facade Seal

Goal:

```text
Seal the renderer provider runtime request surface so archive/restore also
uses `rendererRuntimeClient.runtimeRequest`, and scan against direct provider
bridge bypass.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Archive facade unification | `src/renderer/src/agent/analytix-runtime.ts` routes `archiveThread` through `rendererRuntimeClient.runtimeRequest`. |
| Direct bypass guard | `scripts/scan-product-sovereignty.cjs` forbids `window.analytix.runtime.runtimeRequest` in `analytix-runtime.ts`. |
| Lifecycle proof | `src/renderer/src/agent/analytix-runtime.test.ts` keeps archive/restore route tests passing under the D-0202 alias guard. |
| Product boundary | No Reasonix bridge protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Archive-thread provider facade | code-port-and-adapt | Accepted as internal renderer provider source hardening. |
| Direct bridge bypass scan | contract-reimplement | Accepted as product-sovereignty regression guard. |
| Deprecated bridge aliases | reject | Not read or exposed. |
| Reasonix SessionAPI/public bridge | reject | Not imported. |
| Packaged renderer walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer provider source/unit proof. It does not prove packaged desktop
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0204 Side Conversation Relation Provider Contract

Goal:

```text
Route side conversation promotion through an analytix provider relation
contract and forbid direct runtime request bridge bypass in side-store source.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider contract | `src/renderer/src/agent/types.ts` adds optional `updateThreadRelation`; `AnalytixRuntimeProvider` implements it through `rendererRuntimeClient.runtimeRequest`. |
| Store route | `src/renderer/src/store/chat-store-side-actions.ts` calls `provider.updateThreadRelation(sideId, 'primary')` before refreshing threads and closing the side panel. |
| Tests | `analytix-runtime.test.ts` proves relation PATCH body; `chat-store-side-actions.test.ts` proves promotion uses the provider and refreshes threads. |
| Direct bypass guard | `scripts/scan-product-sovereignty.cjs` forbids direct runtime request bridge calls in provider and side-store sources. |
| Product boundary | No Reasonix side/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Thread relation provider contract | contract-reimplement | Accepted as analytix-owned renderer provider contract. |
| Side conversation promotion path | code-port-and-adapt | Accepted as store-to-provider hardening. |
| Direct bridge bypass scan | contract-reimplement | Accepted as product-sovereignty regression guard. |
| Reasonix SessionAPI/side protocol | reject | Not imported. |
| Packaged side-conversation walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer store/provider source and unit proof. It does not prove
packaged desktop side-conversation walkthrough, live Go bridge behavior, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0205 Renderer Usage Runtime Client Facade Seal

Goal:

```text
Route renderer usage/debug generic runtime HTTP requests through
`rendererRuntimeClient.runtimeRequest` and scan against direct generic runtime
request bridge bypass across renderer production source.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Usage hooks | `use-thread-usage.ts`, `use-daily-usage.ts`, and `use-model-usage.ts` call `rendererRuntimeClient.runtimeRequest` for `/v1/usage` requests. |
| Settings diagnostics | `settings-section-agents.tsx` and `settings-section-llm-debug.tsx` use the same facade for token economy and LLM debug requests. |
| Tests | Existing usage/settings tests keep path construction, parsing, timeout, and error behavior stable. |
| Direct bypass guard | `scripts/scan-product-sovereignty.cjs` forbids `window.analytix.runtime.runtimeRequest` across renderer production source. |
| Product boundary | No Reasonix usage/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer usage transport | code-port-and-adapt | Accepted as internal renderer bridge hardening. |
| Renderer-wide direct runtime request scan | contract-reimplement | Accepted as product-sovereignty regression guard. |
| Dedicated preload APIs | document-only | Config/probe/restart/model-list APIs remain named contracts outside generic runtime HTTP requests. |
| Reasonix usage/debug protocol | reject | Not imported. |
| Packaged usage/dashboard walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer usage/settings source and unit proof. It does not prove
packaged usage/dashboard walkthrough, live Go bridge behavior, G6 readiness, or
release readiness.
```

## 2026-06-23 - D-0225 Renderer Usage Runtime Client Facade G5 Shadow

Goal:

```text
Promote the D-0205 renderer usage/debug runtime client facade seal into G5
desktop sovereignty executable shadow without changing live renderer behavior
or enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `rendererUsageRuntimeClientFacadeMatrix` and seven expected output booleans. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new matrix and `go-runtime-conformance.test.ts` derives it from real usage/settings sources and tests. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes usage loader coverage, diagnostics coverage, direct bridge rejection, scan guard presence, and unit proof. |
| Product boundary | No Reasonix usage/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Usage facade G5 replay | code-port-and-adapt | Accepted as pure executable shadow. |
| Settings diagnostics G5 replay | code-port-and-adapt | Accepted as facade-backed diagnostics proof. |
| Direct runtime bridge bypass | reject | Renderer production source still cannot call `window.analytix.runtime.runtimeRequest` directly. |
| Live Go usage bridge/default backend | defer / reject | Keep behind future G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove packaged usage/dashboard
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0206 Renderer Settings Read Facade Seal

Goal:

```text
Route ordinary renderer settings reads through `rendererRuntimeClient` and scan
against direct `window.analytix.settings.getSettings` in renderer production
source.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Keyboard shortcuts | `keyboard-shortcut-settings.ts` reads settings through `rendererRuntimeClient.getSettings`. |
| Voice dictation | `use-voice-dictation.ts` reads speech settings through the same facade. |
| Usage heatmap | `InitialSessionUsageHeatmap.tsx` reads the runtime model label through the same facade. |
| Direct read guard | `scripts/scan-product-sovereignty.cjs` forbids `window.analytix.settings.getSettings` in renderer production source. |
| Product boundary | No Reasonix settings protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer settings read transport | code-port-and-adapt | Accepted as internal renderer bridge hardening. |
| Renderer direct settings read scan | contract-reimplement | Accepted as product-sovereignty regression guard. |
| Settings write APIs | document-only | Explicit `setSettings` / `saveSettingsSilent` write contracts remain separate. |
| Reasonix settings protocol | reject | Not imported. |
| Packaged settings walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
npx vitest run src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts src/renderer/src/agent/runtime-client.test.ts
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer settings-read source and unit proof. It does not prove
packaged settings walkthrough, live Go bridge behavior, G6 readiness, or
release readiness.
```

## 2026-06-23 - D-0231 MCP Search Refresh Drift Evidence Closure

Goal:

```text
Make the existing MCP catalog refresh/currentness drift evidence
machine-scannable and stage-closure-visible without changing runtime behavior
or exposing a Reasonix MCP/indexer public protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime oracle | `mcp-tool-lifecycle-oracle.json` already records `mcp_refresh_catalog`, initial `search_issues`, expanded `search_issues` + `create_issue`, `expectedTotalIndexed: 2`, and `expectedCatalogDrift: true`. |
| Runtime execution | `mcp-tool-lifecycle-oracle.test.ts` already runs the refresh path and verifies `totalIndexed` plus `catalogDrift`. |
| G5/Go shadow | `controlExecutableCases.mcpSearchRefreshDrift` and Go shadow already replay the same drift/currentness fields. |
| Scan guard | `scan:product-sovereignty` now requires `mcpSearchRefreshDrift`, `mcp_refresh_catalog`, `catalogDrift`, `totalIndexed`, and boundary tokens. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP catalog refresh/currentness drift | `contract-reimplement` | Treat refresh drift as an analytix-owned internal MCP search/tool invariant. |
| D-0231 docs/scan closure | `document-only` | No runtime behavior change; promote existing evidence into mandatory scan/closure docs. |
| Reasonix MCP public protocol / MCP-indexer entry | `reject` | Keep MCP lifecycle behind analytix runtime/tool contracts. |
| Live Go MCP client/default backend | `defer` / `reject` | Keep behind future G5/G6 gates and credentialed MCP QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is evidence-closure proof. It does not prove a live Go MCP client,
credentialed MCP matrix, packaged MCP walkthrough, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0230 Plan/Auto-Route State Reset G5 Shadow

Goal:

```text
Promote Plan cancellation -> normal/auto turn state reset into G5 executable
shadow so Reasonix classifier/planner currentness value is covered without
exposing a public auto-plan setting, Reasonix controller protocol, or default
Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime execution | `loop.test.ts` proves a cancelled Plan turn is followed by a fixed normal turn and a `model: "auto"` turn without leaking `modeInstruction`, `requiredToolName`, or `create_plan` advertisement. |
| Auto-router currentness | The same test proves the post-cancel auto turn invokes `_auto_router` once with no tools/prefix/mode instruction and accepts `deepseek-v4-pro` + `max`. |
| G5 conformance | `go-g5-full-loop-oracle.json` adds `planCancelStateReset`; `runtime-parity-fixtures.ts` validates reset, router, stable-prefix, and product-boundary fields. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes cancelled-plan no-leak, normal/auto tool hiding, auto reroute, isolated router request, current recommendation, stable-prefix cleanliness, and boundary outputs. |
| Product boundary | No public auto-plan setting, project override, Reasonix controller protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Reasonix classifier rebuild on enable/disable | `contract-reimplement` | Map the value to analytix turn-scoped Plan/auto-route state isolation and currentness. |
| Plan cancellation reset proof | `code-port-and-adapt` | Add executable TypeScript test and G5/Go replay. |
| Post-cancel auto-router reroute | `code-port-and-adapt` | Prove the next `model: "auto"` turn reruns `_auto_router` with an isolated classifier request. |
| Public auto-plan settings / project overrides | `reject` / `defer` | Do not add Reasonix config roots, controller API, local override, or product toggle. |
| Live Go loop/default backend | `defer` / `reject` | Keep Go behind future G5/G6 gates; this is executable shadow evidence only. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove a live Go loop, packaged
Plan/auto-route walkthrough, public auto-plan setting, G6 readiness, default
backend readiness, or release readiness.
```

## 2026-06-23 - D-0229 Plan Step/Cancel/Cache G5 Shadow

Goal:

```text
Promote the existing Plan-mode step/cancel/cache boundary guard into G5
executable shadow so Reasonix planner/cancel/cache value is covered without
exposing a public auto-plan setting, Reasonix controller protocol, or default
Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime source proof | `loop.test.ts` already proves step 0 advertises `create_plan` + `ls` but not `bash`, the follow-up is forced to `create_plan`, the follow-up can abort, and the retry preserves DeepSeek cache diagnostics. |
| G5 conformance | `go-g5-full-loop-oracle.json` adds `planStepCancelCache`; `runtime-parity-fixtures.ts` validates the mode, tool sets, abort/retry statuses, usage count, cache hit/miss values, and product-boundary booleans. |
| TS executable oracle | `go-runtime-conformance.test.ts` derives expected booleans from the planner control case and asserts the D-0107 runtime test still contains the required `requiredToolName`, `prefixChanged: false`, and `cacheHitTokens: 80` evidence. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes step-0 read-only+plan, follow-up-only-plan, cancelled-step cache-baseline preservation, retry baseline reuse, usage count, cache telemetry, forbidden-shell exclusion, and boundary outputs. |
| Product boundary | No public auto-plan setting, Reasonix controller protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Plan-mode step tool narrowing | `contract-reimplement` | Keep planner gating inside analytix runtime mode/tool contracts. |
| Aborted plan follow-up cache baseline | `code-port-and-adapt` | Preserve D-0107 behavior and make it replayable in G5/Go shadow. |
| DeepSeek cache telemetry across cancelled plan step | `code-port-and-adapt` | Preserve hit/miss diagnostics without putting dynamic state in the stable prefix. |
| Reasonix public auto-plan/controller protocol | `reject` | Do not expose Reasonix config roots, SessionAPI, controller API, or renderer-visible route. |
| Live Go loop/default backend | `defer` / `reject` | Keep Go behind G5/G6 gates; this is executable shadow evidence only. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/loop.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove a live Go loop, packaged
Plan-mode walkthrough, G6 readiness, default backend readiness, or release
readiness.
```

## 2026-06-23 - D-0228 MCP Call-Time Reconnect G5 Shadow

Goal:

```text
Promote existing MCP tool-call reconnect classification into G5 executable
shadow without exposing a Reasonix MCP protocol, MCP-indexer route, or live Go
MCP client.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| MCP lifecycle oracle | `mcp-tool-lifecycle-oracle.json` adds `callReconnect` for stale connection retry and deterministic protocol error no-retry. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new oracle and `mcp-tool-lifecycle-oracle.test.ts` runs real `buildMcpToolProviders` call paths. |
| G5 conformance | `go-g5-full-loop-oracle.json` adds `mcpCallReconnect`; `go-runtime-conformance.test.ts` derives expected transport/protocol classification from the MCP oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes retry/no-retry classification, attempt counts, close counts, and success/error outputs from the TS-owned fixture. |
| Product boundary | No Reasonix MCP public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP stale connection call retry | code-port-and-adapt | Accepted as internal MCP runtime behavior and shadow replay. |
| Deterministic MCP protocol error no-retry | contract-reimplement | Accepted as product-safe classification to avoid tearing down healthy sessions for validation failures. |
| Reasonix MCP public protocol / MCP-indexer entry | reject | Keep MCP lifecycle behind analytix runtime/tool contracts. |
| Live Go MCP client/default backend | defer / reject | Keep behind future G5/G6 gates and credentialed MCP QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove a live Go MCP client,
credentialed MCP matrix, packaged MCP walkthrough, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0227 AutoResearch Direction Tracking G5 Shadow

Goal:

```text
Promote existing AutoResearch direction tracking into G5 executable shadow
without exposing a top-level AutoResearch route, Reasonix project protocol, or
live Go research backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds direction, outcome, summary, `recordDirectionToolName`, active-goal gating, and expected direction-tracking output booleans. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new fields and `go-runtime-conformance.test.ts` derives them from `autoresearch-store`, `goal-tools`, `thread-service`, and `goal-tools.test.ts`. |
| Runtime execution proof | The conformance test creates real `.analytix/autoresearch/<threadId>/` state, calls `recordDirection`, and reads `directions_tried.json` plus `iteration_log.jsonl`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes direction file, iteration-log, tool-name, and active-goal guard outputs from the TS-owned fixture. |
| Product boundary | No Reasonix project protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| AutoResearch direction tracking replay | code-port-and-adapt | Accepted as executable shadow for existing `record_research_direction`. |
| Active research-goal guard | contract-reimplement | Accepted as analytix goal contract proof; direction records require an active research goal. |
| Reasonix project protocol or top-level AutoResearch entry | reject | Keep state project-local under `.analytix/autoresearch` and reachable only through existing goal/runtime contracts. |
| Live Go research backend/default backend | defer / reject | Keep behind future G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove a live Go research bridge,
packaged long-task walkthrough, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0226 Renderer Settings Read Facade G5 Shadow

Goal:

```text
Promote the D-0206 renderer settings-read facade seal into G5 desktop
sovereignty executable shadow without changing live renderer behavior or
enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `rendererSettingsReadFacadeMatrix` and six expected output booleans. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new matrix and `go-runtime-conformance.test.ts` derives it from real settings-read sources and scan tokens. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes settings reader coverage, settings-changed event preservation, direct bridge rejection, and scan guard presence. |
| Product boundary | No Reasonix settings/config protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Settings-read facade G5 replay | code-port-and-adapt | Accepted as pure executable shadow. |
| Settings-changed event proof | contract-reimplement | Accepted as source-derived hook sync evidence. |
| Direct settings read bridge bypass | reject | Renderer production source still cannot call `window.analytix.settings.getSettings` directly. |
| Live Go settings bridge/default backend | defer / reject | Keep behind future G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove packaged settings
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0207 Renderer Named Bridge API Allow-list

Goal:

```text
Make the remaining direct renderer `window.analytix.runtime.*` and
`window.analytix.settings.*` production calls explicit named contracts, while
continuing to reject generic runtime HTTP and settings read bridge bypasses.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime allow-list | `scripts/scan-product-sovereignty.cjs` allows only named runtime APIs for config file access, provider probe, runtime restart, upstream model fetch, and runtime status subscription. |
| Settings allow-list | The same scan allows only `setSettings` and `saveSettingsSilent` as direct named settings write APIs. |
| Generic bypass guard | The existing scan keeps forbidding direct `window.analytix.runtime.runtimeRequest` and `window.analytix.settings.getSettings` in renderer production source. |
| Proof freshness | Product-sovereignty scan now requires D-0207 final-gate release evidence. |
| Product boundary | No Reasonix public bridge/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Named renderer runtime APIs | contract-reimplement | Accepted as explicit analytix preload contracts. |
| Named renderer settings write APIs | contract-reimplement | Accepted as explicit write contracts separate from settings reads. |
| Generic runtime/settings bridge bypass | reject | `runtimeRequest` and `getSettings` stay behind the renderer client facade. |
| Reasonix/Kun bridge aliases or public protocol | reject | Not imported. |
| Packaged bridge walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
node --check scripts/scan-product-sovereignty.cjs
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer bridge source-scan proof. It does not prove packaged desktop
bridge walkthrough, live Go bridge behavior, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0208 Renderer Optional Bridge Bypass Seal

Goal:

```text
Close the optional-chaining blind spot in renderer bridge scans and remove the
remaining optional-chain generic runtime/settings bridge bypasses from
production renderer source.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Connect Phone dialog | `SidebarClawDialog.tsx` reads settings through `rendererRuntimeClient.getSettings` instead of `window.analytix?.settings?.getSettings`. |
| Plugin marketplace diagnostics | `PluginMarketplaceView.tsx` no longer probes `window.analytix?.runtime?.runtimeRequest`; diagnostics remain behind the provider/runtime facade. |
| Optional-chain-aware scans | `scripts/scan-product-sovereignty.cjs` uses named regex constants that cover both `.` and `?.` for direct generic bypass scans and named API allow-list scans. |
| Proof freshness | Product-sovereignty scan now requires D-0208 final-gate release evidence. |
| Product boundary | No Reasonix public bridge/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Optional-chaining direct bridge access | contract-reimplement | Accepted as the same bridge-governance surface as dot access. |
| Connect Phone settings read transport | code-port-and-adapt | Accepted as renderer client facade hardening. |
| Plugin marketplace generic runtime probe removal | code-port-and-adapt | Accepted as renderer client facade hardening. |
| Generic runtime/settings bridge bypass | reject | Both dot and optional-chain forms are forbidden. |
| Packaged bridge walkthrough | defer | Keep for release-readiness gates. |

Validation:

```text
node --check scripts/scan-product-sovereignty.cjs
rg -n "window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings|window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest" src/renderer/src --glob '!**/*.test.ts'
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is renderer source/scan proof. It does not prove packaged desktop bridge
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0209 Renderer Bridge Allow-list G5 Shadow

Goal:

```text
Promote renderer direct bridge allow-list evidence into the existing Go G5
`desktopSovereignty` executable shadow without enabling a live Go backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `desktopSovereignty.rendererBridgeAllowList`, direct method inventories, bypass counts, optional-chain coverage, and expected output booleans. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new shape and `go-runtime-conformance.test.ts` derives it from real renderer production source plus `scan-product-sovereignty.cjs`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes named runtime/settings allow-list status, generic bypass absence, and optional-chain scan coverage. |
| Scan guard | `scripts/scan-product-sovereignty.cjs` requires D-0209 G5 proof tokens and final gate evidence. |
| Product boundary | No Reasonix public bridge/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Renderer bridge allow-list executable replay | code-port-and-adapt | Accepted as pure G5 shadow proof. |
| TS source-derived direct bridge inventory | contract-reimplement | Accepted under analytix-owned renderer/preload contracts. |
| Live Go desktop bridge/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public bridge/session protocol | reject | Not imported. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove packaged desktop bridge
walkthrough, live Go bridge behavior, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0210 Runtime HTTP Auth Matrix G5 Shadow

Goal:

```text
Promote runtime HTTP auth matrix evidence into `runtimeHttpRouteSovereignty`
G5 executable shadow without implementing a live Go HTTP server.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `runtimeHttpRouteSovereignty.authMatrix` with health 200, protected route keys, unauthorized status/body, and sensitive route keys. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the new matrix and `go-runtime-conformance.test.ts` dispatches every registered route against the matrix. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes health status, all-protected-route 401 status, unauthorized body shape, full matrix coverage, and sensitive route protection. |
| Scan guard | `scripts/scan-product-sovereignty.cjs` requires D-0210 auth matrix proof tokens and final gate evidence. |
| Product boundary | No Reasonix public route/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Runtime HTTP auth matrix executable replay | code-port-and-adapt | Accepted as pure G5 shadow proof. |
| Auth-before-handler invariant | contract-reimplement | Accepted under analytix-owned `analytix serve` routes. |
| Live Go HTTP server/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public route/session protocol | reject | Not imported. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove live Go HTTP server
readiness, packaged route walkthrough, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0211 Runtime Forbidden Route Dispatch G5 Shadow

Goal:

```text
Promote runtime forbidden route dispatch evidence into
`runtimeHttpRouteSovereignty` G5 executable shadow without implementing a live
Go HTTP server.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` with valid-auth mode, 404 status/body, all forbidden tokens, protocol tokens, and hidden-surface tokens. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the matrix and `go-runtime-conformance.test.ts` dispatches every forbidden token against the matrix. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes structured not_found, full-token coverage, protocol-token rejection, hidden-surface rejection, and valid-auth dispatch mode. |
| Scan guard | `scripts/scan-product-sovereignty.cjs` requires D-0211 forbidden-dispatch proof tokens and final gate evidence. |
| Product boundary | No Reasonix public route/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Runtime forbidden route dispatch executable replay | code-port-and-adapt | Accepted as pure G5 shadow proof. |
| Upstream protocol route rejection | contract-reimplement | Accepted under analytix-owned `analytix serve` routes. |
| Hidden capability route rejection | contract-reimplement | Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route tokens remain structured 404 with valid auth. |
| Live Go HTTP server/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public route/session protocol | reject | Not imported. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove live Go HTTP server
readiness, packaged route walkthrough, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0212 Shared Endpoint Builder G5 Shadow

Goal:

```text
Promote shared endpoint builder evidence into `runtimeHttpRouteSovereignty` G5
executable shadow without implementing a live Go HTTP server.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` with 18 encoded builder cases, exported endpoint strings, forbidden-token absence, and canonical user-input state. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the matrix and `go-runtime-conformance.test.ts` derives it from shared endpoint source/test evidence. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes builder count, encoded route-id preservation, template ownership, plural user-input canonical status, sensitive builder coverage, and unit-proof presence. |
| Scan guard | `scripts/scan-product-sovereignty.cjs` requires D-0212 shared-endpoint G5 proof tokens and final gate evidence. |
| Product boundary | No Reasonix public route/session protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, renderer-visible Go route, default Go backend, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Shared endpoint builder executable replay | code-port-and-adapt | Accepted as pure G5 shadow proof. |
| Route-id injection prevention | contract-reimplement | Shared builders keep slash/query/fragment text encoded in path segments. |
| Canonical plural user-input endpoint | contract-reimplement | `/v1/user-inputs/{id}` remains exported canonical template; singular compatibility is not exported. |
| Live Go HTTP server/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public route/session protocol | reject | Not imported. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not prove live Go HTTP server
readiness, packaged route walkthrough, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0179 Post-881 Sub-Agent Review Matrix

Goal:

```text
Make the six-lane sub-agent review itself a scan-visible QA artifact and
correct provider/cache and Go conformance wording drift before the next code
absorption batch.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| QA artifact | `docs/analytix/qa/post-881-subagent-review-2026-06-22.md` records lanes A-F, delta classifications, customProviderRequestShapeOnly, goRuntimeConformanceRangeCorrected, and subagentOpenGates. |
| Product scan | `scripts/scan-product-sovereignty.cjs` requires the sub-agent review file and core tokens. |
| Provider claim correction | `post-881-stage-closure-2026-06-22.md` now separates DeepSeek/OpenAI Responses/Anthropic raw cache accounting from custom provider request-shape-only proof. |
| Go conformance correction | `go-runtime-conformance.md` now states the D-0177 release evidence scan covered D-0172 through D-0178. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Six-lane sub-agent review evidence | document-only | Accepted as scan-visible QA evidence. |
| Reasonix delta classification matrix | contract-reimplement | Accepted as an analytix-owned decision record, not upstream protocol adoption. |
| Custom provider cache telemetry claim | reject | Keep only request-shape/full-endpoint coverage until real telemetry exists. |
| Live Go/backend/provider/MCP parity | defer | Keep open gates explicit. |

Validation:

```text
git diff --check
npm --prefix packages/runtime test
npm run test
npm run typecheck
npm run build:runtime
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```

Remaining risks:

```text
This batch is QA evidence and scan wiring. It does not implement live provider
credentials, packaged walkthroughs, live Go managers, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0180 Custom Provider Telemetry Seal

Goal:

```text
Turn the D-0179 custom provider request-shape-only boundary into executable
runtime and Go G5 shadow proof.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider proof | `provider-cache-proof.test.ts` asserts custom full endpoint ids exist only in request-shape cases and do not appear in telemetry-supported usage ids. |
| TS conformance | `go-runtime-conformance.test.ts` derives `customFullEndpointTelemetryCaseIds: []` and request-shape-only booleans from the provider-cache oracle. |
| Fixture schema | `runtime-parity-fixtures.ts` requires the new seal fields in `providerCacheCoverageFloor.expected`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same seal fields from fixture inputs. |
| Scan guard | `scan-product-sovereignty.cjs` now searches for the custom telemetry seal tokens. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Custom provider request-shape-only proof | contract-reimplement | Accepted as explicit TS/Go G5 control evidence. |
| Custom provider cache telemetry | reject | Keep telemetry ids empty until credentialed/raw telemetry evidence exists. |
| Go G5 executable shadow | code-port-and-adapt | Accepted as shadow-only computation, not a backend. |
| Live provider/cache matrix | defer | Remains an open release gate. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is fixture/control-shadow proof. It does not add custom provider cache
telemetry, live credentials, packaged provider QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0181 Goal Persistence Off-Lock Control Shadow

Goal:

```text
Close the Go conformance "future Go lock fixture needed" gap with
source-derived G5 executable shadow evidence.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS source proof | `go-runtime-conformance.test.ts` reads `ThreadService` and `thread-service.test.ts` to derive persist-before-event order, no forbidden lock substrings, and warning/error proof. |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `goalPersistenceOffLock`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the off-lock output from fixture inputs, and `shadow_test.go` compares it to expected. |
| Scan guard | `scan-product-sovereignty.cjs` checks goal persistence off-lock proof tokens. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Goal persistence off controller lock | code-port-and-adapt | Accepted as source/fixture proof for future Go parity. |
| Persist-before-event ordering | contract-reimplement | Accepted for `setGoal` and `clearGoal`. |
| TS controller lock abstraction | reject | Do not invent a TypeScript lock just to mirror Reasonix. |
| Live Go goal persistence manager | defer | Keep behind future G5/G6 implementation gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/thread-service.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is source/fixture/control-shadow proof. It does not implement live Go goal
persistence, packaged goal QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0182 Tool Result File/Image Boundary Shadow

Goal:

```text
Promote the tool result images/files Go conformance row into source-derived
G5 executable shadow evidence without changing runtime behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS source/test proof | `go-runtime-conformance.test.ts` reads `tool-result-image.ts`, `tool-result-image.test.ts`, `attachment-store.test.ts`, and `analytix-mapper.test.ts`. |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `toolResultFileImageBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the file/image boundary output from fixture inputs, and `shadow_test.go` compares it to expected. |
| Scan guard | `scan-product-sovereignty.cjs` checks tool result file/image proof tokens and the final D-0182 gate. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Inline image result extraction and newest-image cap | code-port-and-adapt | Accepted as G5 shadow evidence for future Go model-history/file bridge parity. |
| Attachment `localFilePath` and text fallback `FilePath` propagation | contract-reimplement | Accepted as analytix-owned attachment contract evidence. |
| Generated-file and tool attachment meta lifting | contract-reimplement | Accepted as renderer projection evidence without adding new UI entry points. |
| Reasonix file/SessionAPI protocol | reject | Do not expose upstream file routes, controller protocol, or renderer-visible Go route. |
| Live Go file/image bridge and packaged attachment QA | defer | Keep behind future G5/G6 and release gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts src/loop/tool-result-image.test.ts tests/attachment-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is source/fixture/control-shadow proof. It does not implement live Go
attachment storage, packaged attachment QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0183 Event JSONL Replay Boundary Shadow

Goal:

```text
Promote events.jsonl replay and malformed recovery into source-derived G5
executable shadow evidence without changing runtime behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS source/test proof | `go-runtime-conformance.test.ts` reads `file-session-store.ts`, `loop.test.ts`, `runtime-event-recorder.test.ts`, and `file-session-store.test.ts`. |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `eventJsonlReplayBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the event replay boundary output from fixture inputs, and `shadow_test.go` compares it to expected. |
| Scan guard | `scan-product-sovereignty.cjs` checks event JSONL replay proof tokens and the final D-0183 gate. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| `events.jsonl` append/replay/highestSeq semantics | contract-reimplement | Accepted as analytix-owned event persistence contract evidence. |
| Runtime recorder persist-before-publish and sequence uniqueness | code-port-and-adapt | Accepted as future Go event-store parity evidence. |
| Malformed JSONL recovery and usage compaction carryover | contract-reimplement | Accepted as long-thread/OOM safety proof without changing retention policy. |
| Reasonix event/session protocol | reject | Do not expose upstream event protocol or renderer-visible Go route. |
| Live Go event store / packaged long-thread replay QA | defer | Keep behind future G5/G6 and release gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/runtime-event-recorder.test.ts tests/file-session-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is source/fixture/control-shadow proof. It does not implement live Go
event persistence, packaged long-thread replay QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0184 MCP Malformed Schema Boundary Shadow

Goal:

```text
Promote malformed MCP schema normalization into source-derived G5 executable
shadow evidence without changing runtime behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| TS source/test proof | `go-runtime-conformance.test.ts` reads `mcp-tool-provider.ts` and `mcp-tool-provider.test.ts`. |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `mcpMalformedSchemaBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the malformed-schema boundary output from fixture inputs, and `shadow_test.go` compares it to expected. |
| Scan guard | `scan-product-sovereignty.cjs` checks MCP malformed schema proof tokens and the final D-0184 gate. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Non-object MCP input schema safe default | contract-reimplement | Accepted as analytix-owned MCP advertisement safety evidence. |
| Invalid `properties` / mixed `required` normalization | code-port-and-adapt | Accepted as future Go MCP catalog parity evidence. |
| Non-record output schema omission | contract-reimplement | Accepted to keep model/catalog exposure schema-safe. |
| Reasonix MCP-indexer protocol | reject | Do not expose upstream MCP-indexer protocol or renderer-visible Go route. |
| Live Go MCP client / credentialed MCP QA | defer | Keep behind future G5/G6 and release gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-provider.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is source/fixture/control-shadow proof. It does not implement live Go MCP
execution, credentialed MCP QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0172 Provider Cache Accounting Raw Payload Replay

Goal:

```text
Move provider cache accounting proof from copied expected summaries to raw
provider response payload replay while keeping Go shadow-only.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G3/G5 fixtures | `go-g3-provider-streaming-usage-cache-oracle.json` and `go-g5-full-loop-oracle.json` carry raw `responseBody` on provider usage/accounting rows. |
| Runtime schema/test | `runtime-parity-fixtures.ts`, `go-runtime-conformance.test.ts`, and `go-runtime-g3-g4-conformance.test.ts` validate and derive raw accounting output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` and `shadow_g5.go` compute raw parsed ids, telemetry-supported ids, expected-usage match ids, provider-family ids, hit/miss totals, and aggregate hit rate. |
| Product boundary | No active provider runtime behavior, Reasonix public protocol, renderer-visible Go route, default backend, top-level hidden entry, Kun identity, or Rust/Tauri path is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Raw provider usage to cache accounting | contract-reimplement | Accepted through TS-owned raw payload fixtures. |
| Go G3/G5 replay | code-port-and-adapt | Accepted as pure executable shadow proof. |
| Active provider request/stream/client changes | reject | Keep current TypeScript provider contracts authoritative. |
| Credentialed provider matrix | defer | Keep live provider QA separate from fixture evidence. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is fixture-backed raw replay proof. It does not prove live provider
credentials, packaged settings QA, Go provider client readiness, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0173 Provider Raw Accounting Proof Freshness

Goal:

```text
Make raw provider cache accounting evidence a direct provider-cache oracle
invariant and a reusable product-sovereignty scan token.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime proof | `provider-cache-proof.test.ts` now derives raw usage snapshots and aggregate cache accounting directly from `provider-cache-oracle.json` response bodies. |
| Scan freshness | `scan-product-sovereignty.cjs` requires raw accounting proof tokens such as `rawProviderCacheAccounting`, `rawPayloadParsedCaseIds`, and `rawUsageFromProviderPayload`. |
| Product boundary | The proof is fixture-only and does not change provider clients, URL/body behavior, Go backend status, renderer routes, or product navigation. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Direct raw accounting oracle | contract-reimplement | Accepted as provider-cache proof, independent of Go conformance. |
| Raw proof freshness scan | contract-reimplement | Accepted so evidence cannot silently disappear. |
| Active provider runtime changes | reject | Keep current TypeScript provider clients authoritative. |
| Credentialed provider matrix | defer | Keep live provider QA separate from fixture evidence. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is direct fixture proof plus scan freshness. It does not prove live
provider credentials, packaged settings QA, Go provider client readiness,
G6 readiness, or release readiness.
```

## 2026-06-22 - D-0174 Combined Step/Cancel/Cache Proof Freshness

Goal:

```text
Make the combined step/cancel/cache trace a focused G5 oracle and reusable
product-sovereignty scan token.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Focused G5 proof | `go-runtime-conformance.test.ts` now has a dedicated combined trace test for route-cache reuse, step-limit boundary, cancel result pairing, stable-prefix isolation, and backend boundary flags. |
| Scan freshness | `scan-product-sovereignty.cjs` requires combined step/cancel/cache proof tokens such as `routeCacheReusedUntilStepLimit`, `sameTurnRouterCalls`, `nextTurnRouterCalls`, and `cancelResults`. |
| Product boundary | The proof is fixture-only and does not change the runtime loop, Go backend status, renderer routes, settings, or product navigation. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Focused combined step/cancel/cache oracle | contract-reimplement | Accepted as analytix-owned G5 proof for route-cache, step-limit, cancel, and stable-prefix composition. |
| Combined trace scan freshness | contract-reimplement | Accepted so D-0146 evidence cannot silently disappear. |
| Live Go agent loop/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix controller/session protocol or public auto-plan setting | reject | Do not expose upstream protocol or product surface. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is focused fixture proof plus scan freshness. It does not prove live Go
agent-loop parity, packaged long-running cancel/cache QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0175 Approval/User-Input Proof Freshness

Goal:

```text
Make approval/user-input gate safety a focused G5 oracle and reusable
product-sovereignty scan token.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Focused G5 proof | `go-runtime-conformance.test.ts` now has a dedicated approval/user-input test for denied no-execute, answer privacy, structured validation, abort cleanup, resume cleanup, and backend boundary flags. |
| Scan freshness | `scan-product-sovereignty.cjs` requires approval/user-input proof tokens such as `approvalUserInputRouteReplay`, `approvalUserInputInventory`, `resolvedEventIncludesAnswers`, and `answersCopiedToResume`. |
| Product boundary | The proof is fixture-only and does not change live approval/user-input managers, Go backend status, renderer routes, settings, or product navigation. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Focused approval/user-input gate oracle | contract-reimplement | Accepted as analytix-owned G5 proof for no-execute, answer privacy, late rejection, and resume cleanup. |
| Approval/user-input proof freshness scan | contract-reimplement | Accepted so gate evidence cannot silently disappear. |
| Live Go approval/user-input manager/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix ask/session public protocol | reject | Do not expose upstream protocol or product surface. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is focused fixture proof plus scan freshness. It does not prove live Go
approval/user-input manager parity, packaged approval-card QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0176 Session Route Proof Freshness

Goal:

```text
Make thread/session route replay a focused G5 oracle and reusable
product-sovereignty scan token.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Focused G5 proof | `go-runtime-conformance.test.ts` now has a dedicated session route test for archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, exact route hashes, inventory, and backend boundary flags. |
| Scan freshness | `scan-product-sovereignty.cjs` requires session route replay proof tokens such as `sessionRouteReplay`, `archiveResponseHash`, `forkResponseHash`, `resumeResponseHash`, `replaySseHash`, and `unauthorizedBodyHash`. |
| Product boundary | The proof is fixture-only and does not change live TypeScript routes, Go backend status, renderer routes, settings, or product navigation. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Focused thread/session route oracle | contract-reimplement | Accepted as analytix-owned G5 proof for archive/search/fork/resume/SSE/auth route semantics. |
| Exact body/SSE hash replay | code-port-and-adapt | Accepted as shadow-only replay from the TS-owned G2 route oracle. |
| Session route proof freshness scan | contract-reimplement | Accepted so route/SSE evidence cannot silently disappear. |
| Live Go router/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix SessionAPI/public route protocol | reject | Do not expose upstream protocol or product surface. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

Remaining risks:

```text
This is focused fixture proof plus scan freshness. It does not prove live Go
thread/session router parity, packaged route walkthrough QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0177 Release Evidence Final-Gate Freshness

Goal:

```text
Make post-881 release evidence final gates machine-checkable by the
product-sovereignty scan.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Release evidence path freshness | `scan-product-sovereignty.cjs` now includes `docs/analytix/qa/release-evidence-gate-2026-06-21.md` in explicit proof paths. |
| Final gate token scan | The scan requires final command gate entries for D-0172 through D-0177. |
| Required command coverage | The scan requires the release evidence file to keep runtime package tests, workspace tests, typecheck, runtime build, non-cached Go shadow tests, and product-sovereignty scan commands visible. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Final-gate evidence freshness | contract-reimplement | Accepted as analytix-owned QA guardrail for stage closure. |
| Release evidence command coverage | document-only | The scan checks recorded evidence text; it does not execute the commands by itself. |
| Runtime/provider/Go behavior | reject | Do not change active runtime behavior, provider behavior, bridge contracts, or Go backend status. |
| Packaged/live release QA | defer | Keep packaged walkthroughs and release readiness outside this scan-only batch. |

Validation:

```text
npm run scan:product-sovereignty
git diff --check
```

Remaining risks:

```text
This is release-evidence freshness proof. It does not replace running the full
gate suite, packaged desktop QA, live provider/MCP matrices, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0178 Post-881 Stage Closure Snapshot

Goal:

```text
Make the current post-881 capability floor, stronger-than evidence, Kun
baseline preservation, and remaining open gates machine-scannable.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Closure snapshot | `docs/analytix/qa/post-881-stage-closure-2026-06-22.md` records fixture-level floors, absorbed Reasonix delta families, Kun baseline preservation, stronger-than evidence, and open gates. |
| Scan freshness | `scan-product-sovereignty.cjs` requires the snapshot path and core tokens for floor, absorption, Kun baseline, stronger-than, and open-gate sections. |
| Product boundary | The snapshot repeats no top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no Reasonix public protocol, no default Go backend, and no Rust/Tauri rewrite. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Stage closure capability floor | contract-reimplement | Accepted as an analytix-owned QA artifact summarizing current proof scope. |
| Reasonix absorbed/open classification | document-only | The snapshot points to existing D-0172 through D-0177 evidence; it does not add runtime behavior. |
| Stronger-than evidence statement | document-only | Allowed only for verified fixture/contract/governance dimensions, not live superiority. |
| Full release completion | defer | Keep live provider/cache, packaged QA, G6, signing, and distribution readiness open. |

Validation:

```text
npm run scan:product-sovereignty
git diff --check
```

Remaining risks:

```text
This is a closure snapshot and scan-freshness proof. It does not replace live
provider/cache superiority testing, packaged desktop QA, G6 backend selection,
or release readiness.
```

## 2026-06-22 - D-0149 Go G5 Context Compaction Boundary Executable Shadow

Goal:

```text
Promote latest-compaction effective-history boundary behavior into the G5
control executable oracle without enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` adds `controlExecutableCases.compactionBoundary`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the shape and `go-runtime-conformance.test.ts` derives expected values from `effectiveHistoryAfterLatestCompaction`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify effective/dropped ids. |
| Product/cache boundary | No renderer-visible Go route, default Go backend, Reasonix protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or stable-prefix compaction state is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Latest effective-history boundary after compaction | contract-reimplement | Accepted as analytix-owned runtime history semantics. |
| Fixture-only Go replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Noop/older compaction as current boundary | reject | `replacedTokens: 0` and older compactions are dropped from the effective post-boundary history. |
| Dynamic compaction state in stable prefix | reject | Keep provider cache prefix stable and free of runtime control bookkeeping. |
| Live Go history manager/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public controller/session protocol | reject | Do not expose upstream protocol or route names. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go history
manager, Go runtime routes, packaged desktop long-history QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0150 Go G5 User-Input Structured Validation Control Shadow

Goal:

```text
Promote structured request_user_input validation into the G5 control executable
oracle without exposing Reasonix ask/session protocol or enabling Go gates.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` extends `controlExecutableCases.userInput` with `structuredChoiceValidation`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the shared G4/G5 validation shape and `go-runtime-conformance.test.ts` derives G5 expected values from the G4 oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes invalid case count and the four reject booleans. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix ask/session protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, deprecated bridge/settings fallback, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Structured choice validation | contract-reimplement | Accepted through analytix-owned `request_user_input` semantics. |
| Fixture-only Go replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Invalid request opening a pending gate | reject | Invalid structured input returns `invalid_user_input_request` and opens no gate. |
| Reasonix `ask` protocol/name | reject | Do not expose upstream ask/session protocol, CLI, route, or product copy. |
| Live Go user-input manager/default backend | defer / reject | Keep future work behind G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go
approval/user-input manager, packaged desktop approval-card QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0126 Go G5 Approval/User-Input Route Replay Control Shadow

Goal:

```text
Promote approval/user-input route replay from summary evidence into an
executable Go shadow case without enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.approvalUserInputRouteReplay` with a minimal TS-owned approval/user-input oracle and expected replay output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the narrowed control oracle and `go-runtime-conformance.test.ts` derives expected values from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed approval/user-input route replay output. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix SessionAPI/ask protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Approval deny route replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Submitted/cancelled user-input route replay | code-port-and-adapt | Accepted as pure G5 executable shadow, including HTTP answer echo and resolved-event answer omission. |
| analytix approval/user-input contract | contract-reimplement | Expected output stays tied to TS-owned approval/user-input oracle. |
| Live Go approval/user-input manager | defer | Keep future work behind G5/G6 gates. |
| Reasonix public ask/session protocol | reject | Do not expose upstream protocol or route names. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go
approval/user-input manager, Go routes, packaged desktop approval-card QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0127 Go G5 MCP Search Refresh Drift Control Shadow

Goal:

```text
Promote MCP search refresh catalog drift from summary evidence into an
executable Go shadow case without enabling Go MCP as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpSearchRefreshDrift` with a minimal TS-owned refresh drift oracle and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP search refresh drift output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP catalog refresh drift replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| analytix MCP lifecycle contract | contract-reimplement | Expected output stays tied to TS-owned MCP lifecycle oracle and internal meta-tools. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0130 Go G5 MCP Core Lifecycle Control Shadow

Goal:

```text
Promote MCP connect/disconnect/reload/cancel/error lifecycle evidence from
summary output into an executable Go shadow case without enabling Go MCP.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpCoreLifecycle` with a minimal TS-owned core lifecycle oracle and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP core lifecycle output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP connect/disconnect/reload/cancel/error replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| MCP lifecycle contract | contract-reimplement | Expected output stays tied to TS-owned MCP lifecycle oracle and internal tool-source semantics. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0133 Go G5 Task-Job Tool Contract Boundary Control Shadow

Goal:

```text
Promote task/parallel task tool contract and route-boundary evidence from G5
summary output into an executable Go shadow case without enabling a live Go Job
Manager.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.toolContractBoundary` with TS-owned task/parallel tool contracts, runtime routes, protected routes, forbidden top-level routes, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed task-job tool-boundary output. |
| Product boundary | No live Go Job Manager, public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Task/parallel task tool contract boundary | contract-reimplement | Accepted as a TS-owned G5 control case for internal-runtime-only tools, permission/evidence/dependency/read-only requirements, route auth, and forbidden top-level routes. |
| Go shadow replay | code-port-and-adapt | Accepted as pure fixture computation in Go shadow. |
| Reasonix public sub-agent/job protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go Job Manager / packaged sub-agent QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
live Go task-job routes, renderer nested-card packaged QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0135 Go G5 Nested Child SSE Metadata Control Shadow

Goal:

```text
Promote nested child SSE metadata evidence from G5 summary output into an
executable Go shadow case without enabling a live Go Job Manager or Reasonix
SessionAPI.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.nestedSseMetadata` with TS-owned parent/child ids, metadata field list, parent-goal evidence keys, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed nested SSE metadata output. |
| Product boundary | No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Nested child SSE metadata | contract-reimplement | Accepted as a TS-owned G5 control case proving parent/child ids and evidence ledger keys remain available for analytix projection. |
| Go shadow replay | code-port-and-adapt | Accepted as pure fixture computation in Go shadow. |
| Reasonix SessionAPI/public job protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go Job Manager / renderer packaged QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
live Go task-job routes, renderer nested-card packaged QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0134 Go G5 Task Transcript Identity Control Shadow

Goal:

```text
Promote transcript continue/fork identity evidence from G5 summary output into
an executable Go shadow case without enabling a live Go Job Manager or Reasonix
SessionAPI.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.transcriptIdentity` with TS-owned transcript source/continue/fork ids, incompatible error, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed transcript identity output. |
| Product boundary | No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route, default backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Transcript continue/fork identity | contract-reimplement | Accepted as a TS-owned G5 control case proving continue target preserves source identity and fork target is distinct. |
| Go shadow replay | code-port-and-adapt | Accepted as pure fixture computation in Go shadow. |
| Reasonix SessionAPI/public job protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go Job Manager / renderer packaged QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
live Go task-job routes, renderer nested-card packaged QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0132 Go G5 Product Boundary Control Shadow

Goal:

```text
Promote global product-boundary evidence from G5 metadata into an executable Go
shadow case without enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.productBoundary` with TS-owned product boundary, Electron/default-backend disabled flags, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from the G5 oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed product-boundary output. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Product-boundary executable gate | contract-reimplement | Accepted as a TS-owned G5 control case that directly asserts analytix bridge/serve sovereignty and Go disabled state. |
| Go shadow replay | code-port-and-adapt | Accepted as pure fixture computation in Go shadow. |
| Reasonix public protocol/default Go backend | reject | Do not expose upstream protocol, route names, renderer-visible Go route, or backend switch. |
| Electron integration / G6 readiness | defer | Keep future work behind explicit G5/G6 gates and packaged QA. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go backend,
Electron main integration, G6 readiness, packaged desktop QA, or release
readiness.
```

## 2026-06-22 - D-0131 Go G5 MCP Background Reconnect Control Shadow

Goal:

```text
Promote MCP background reconnect evidence from summary output into an
executable Go shadow case without enabling Go MCP.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpBackgroundReconnect` with the TS-owned reconnect oracle and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the reconnect oracle/output and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP reconnect output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP failed-server reconnect replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Analytix MCP lifecycle contract | contract-reimplement | Expected output stays tied to the TS-owned MCP lifecycle oracle and internal tool-source semantics. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Product-boundary and tool/job follow-ups | defer | Sub-agent scans identified product-boundary, task route, transcript, child SSE, known-override, and live-local indexer typed gates as future slices. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0136 Go G5 MCP Known Override Diagnostics Control Shadow

Goal:

```text
Promote MCP known override diagnostics from summary output into an executable
Go shadow case without enabling Go MCP.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpKnownOverrideDiagnostics` with the TS-owned known-override variants and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates known-override diagnostic rows/output and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed known-override diagnostics output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP known override diagnostics/config | contract-reimplement | Accepted as analytix-owned MCP lifecycle diagnostic evidence for override kind, effective cwd, workspace root, low priority, and background start. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed MCP matrix / packaged MCP QA | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0137 Go G5 MCP Live-Local Indexer Control Shadow

Goal:

```text
Promote MCP live-local indexer lifecycle evidence from summary output into an
executable Go shadow case without enabling Go MCP.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpLiveLocalIndexer` with the TS-owned live-local indexer fixture and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates live-local indexer source/output and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed live-local indexer output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP live-local indexer lifecycle | contract-reimplement | Accepted as analytix-owned MCP lifecycle evidence for retry, tombstone, restart, active path, and redaction behavior. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed MCP matrix / packaged MCP QA | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0138 Go G5 MCP Search Meta-Tool Control Shadow

Goal:

```text
Promote MCP search meta-tool advertised/trust/no-execute evidence from summary
output into an executable Go shadow case without enabling Go MCP.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpSearchMetaTools` with the TS-owned search meta-tools fixture and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates search meta-tools source/output and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed search meta-tools output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP search meta-tool advertised/trust/no-execute | contract-reimplement | Accepted as analytix-owned MCP lifecycle evidence for meta-tool set, workspace trust, `on-request` policy, and denied no-execute behavior. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed MCP matrix / packaged MCP QA | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0139 Go G5 Provider Cache Privacy Control Shadow

Goal:

```text
Promote provider cache diagnostics privacy and live-superiority policy from
summary output into an executable Go shadow case without enabling Go providers.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerCachePrivacy` with TS-owned privacy, diagnostics, live credential policy, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates provider cache privacy source/output and `go-runtime-conformance.test.ts` derives expected values from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed provider cache privacy output. |
| Product boundary | No live Go provider client, renderer-visible Go route, default Go backend, Reasonix provider protocol, credentialed provider matrix, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider cache diagnostics privacy | contract-reimplement | Accepted as analytix-owned provider/cache evidence for bounded diagnostics and forbidden substring no-leak behavior. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix provider protocol / live superiority claim | reject | Do not expose upstream protocol or claim live provider superiority from fixture-only proof. |
| Live Go provider/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed provider matrix / packaged provider QA | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go provider
client, credentialed provider matrix, packaged provider QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0140 Go G5 Provider Cache Inventory Control Shadow

Goal:

```text
Promote provider cache prefix/tool/case-id inventory from loose summary fields
into an executable Go shadow case without enabling Go providers.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerCacheInventory` with TS-owned stable prefix, usage ids, request-shape ids, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates provider cache inventory source/output and `go-runtime-conformance.test.ts` derives expected values from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed provider cache inventory output. |
| Product boundary | No live Go provider client, renderer-visible Go route, default Go backend, Reasonix provider protocol, dynamic stable-prefix material, credentialed provider matrix, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider cache prefix/tool inventory | contract-reimplement | Accepted as analytix-owned provider/cache evidence for stable prefix hash, tools hash, usage matrix ids, and request-shape ids. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix provider protocol / dynamic prefix material | reject | Do not expose upstream protocol or place dynamic context/credentials in stable prefix. |
| Live Go provider/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed provider matrix / packaged provider QA | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go provider
client, credentialed provider matrix, packaged provider QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0141 Go G5 Session Route Inventory Control Shadow

Goal:

```text
Promote thread/session route inventory from the G2 route oracle into an
executable Go shadow case without enabling Go thread/session routes.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.sessionRouteInventory` with TS-owned route summaries and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates session route inventory source/output and `go-runtime-conformance.test.ts` derives expected values from `go-g2-route-replay-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed session route inventory output. |
| Product boundary | No live Go HTTP server, renderer-visible Go route, default Go backend, Reasonix SessionAPI/public route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Thread/session route inventory | contract-reimplement | Accepted as analytix-owned runtime HTTP/SSE evidence for list/archive/search/read/update/fork/resume/SSE/auth route families. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix SessionAPI / public route protocol | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go HTTP server/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged route QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement live Go thread/session
routes, packaged desktop thread/fork/resume/archive/search QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0142 Go G5 Approval/User-Input Inventory Control Shadow

Goal:

```text
Promote approval/user-input gate inventory from the route oracle into an
executable Go shadow case without enabling Go approval/user-input managers.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.approvalUserInputInventory` with TS-owned approval/user-input ids, replay kinds, answer privacy flags, late statuses, pending counters, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates approval/user-input inventory input/output and `go-runtime-conformance.test.ts` derives expected values from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed approval/user-input inventory output. |
| Product boundary | No live Go approval/user-input manager, renderer-visible Go route, default Go backend, Reasonix ask/session public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Approval/user-input gate inventory | contract-reimplement | Accepted as analytix-owned gate/event evidence for approval decision, user-input submit/cancel, abort cleanup, replay kinds, and answer privacy. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix ask/session public protocol | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go approval/user-input manager/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged approval-card QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement live Go approval/
user-input managers, packaged desktop approval-card QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0143 Go G5 Task Planner Toolset Inventory Control Shadow

Goal:

```text
Promote task/sub-agent planner read-only and forbidden toolset inventory from
summary replay into an executable Go shadow case without enabling a Go
planner/executor.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.plannerToolsetInventory` with TS-owned planner toolsets, task tool names, policy values, and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates planner toolset inventory input/output and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed planner toolset inventory output. |
| Product boundary | No live Go planner/executor, live Go Job Manager, renderer-visible Go route, default Go backend, Reasonix public sub-agent/job/planner protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Planner read-only/forbidden task toolset inventory | contract-reimplement | Accepted as analytix-owned planner/sub-agent safety evidence: planner gets read-only tools and `task`/`parallel_tasks` remain forbidden. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix public sub-agent/job/planner protocol | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go planner/executor/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged sub-agent QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go planner/
executor or Job Manager, packaged desktop sub-agent QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0145 Go G3/G5 Provider Request-Shape Exact Matrix Control Shadow

Goal:

```text
Promote provider request-shape evidence from summary counts into exact matrix
replay without enabling a live Go provider client or provider protocol surface.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider oracle | `provider-cache-oracle.json.requestShapeCases` remains the TS-owned authority for 7 exact URL/header/body/tool-shape cases. |
| G3/G5 fixtures | `go-g3-provider-streaming-usage-cache-oracle.json` and `go-g5-full-loop-oracle.json` now carry `requestShapeSummary.matrix` / `requestShapeReplay.matrix` / `providerRequestShape.expected.matrix`. |
| Runtime schema/test | `runtime-parity-fixtures.ts`, `go-runtime-g3-g4-conformance.test.ts`, and `go-runtime-conformance.test.ts` require the exact matrix from the provider-cache oracle. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` and `shadow_g5.go` clone and replay exact matrix fields while preserving empty arrays for JSON parity. |
| Product boundary | No live Go provider client, Reasonix provider protocol, renderer-visible Go route, default Go backend, deprecated settings fallback, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider request-shape exact URL/header/body/tool matrix | contract-reimplement | Accepted as analytix-owned provider/cache evidence for DeepSeek, OpenAI-compatible, Responses, Anthropic Messages, and custom full endpoints. |
| Go G3/G5 shadow replay | code-port-and-adapt | Accepted as pure executable shadow. |
| Reasonix provider protocol/settings surface | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, old settings fallback, or default Go backend. |
| Live credentialed provider matrix / superiority claims | defer / reject | Keep future work behind live credential policy and never claim live superiority from fixture-only evidence. |
| Packaged provider settings QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go provider
client, credentialed provider matrix, packaged provider QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0146 Go G5 Combined Step/Cancel/Cache Trace Control Shadow

Goal:

```text
Promote combined auto-route cache, step-limit, and cancel evidence from summary
booleans into exact executable trace replay without enabling a live Go loop.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` expands `controlExecutableCases.combined.expected` with router calls, model steps, stable-prefix flags, result counts, and exact cancel result rows. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the trace and `go-runtime-conformance.test.ts` derives expected values from the combined fixture/control replay. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the trace from combined inputs instead of trusting summary booleans. |
| Product boundary | No live Go agent loop, Reasonix controller/session protocol, renderer-visible Go route, default Go backend, public auto-plan setting, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Combined step/cancel/cache trace | contract-reimplement | Accepted as analytix-owned evidence for route-cache reuse, step-limit boundary, stable-prefix isolation, and cancel result pairing. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix controller/session protocol or public auto-plan setting | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go loop/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged long-running cancel/cache QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go agent loop,
packaged long-running cancel/cache QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0148 Go G5 History Repair Pair-Integrity Control Shadow

Goal:

```text
Promote model-history repair and tool-call/result pair integrity from TS-only
unit proof into executable Go G5 control shadow without enabling a live Go loop.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.historyRepair` with simplified history items and expected repaired/dropped ids. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the case and `go-runtime-conformance.test.ts` derives expected output from `repairModelHistoryItems`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` replays the pair-integrity algorithm for complete tool-call blocks, result blocks, and bridge items. |
| Product boundary | No live Go agent loop, Reasonix SessionAPI/controller protocol, renderer-visible Go route, default Go backend, public auto-plan setting, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Tool-call/result pair integrity | contract-reimplement | Accepted as analytix-owned history hygiene evidence: complete multi-tool blocks survive while orphan results, missing-result calls, and duplicate results are dropped. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow using simplified fixture items, not a live Go history manager. |
| Dynamic repair state in stable prefix | reject | Repair bookkeeping remains model-history hygiene and must not enter the immutable prefix/cache key. |
| Reasonix SessionAPI/controller protocol or public history protocol | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go loop/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged long-history QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go agent loop,
packaged long-history QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0147 Go G5 Step-Limit Override/Delegate Matrix Control Shadow

Goal:

```text
Promote step-limit override and delegate inheritance evidence from scalar
fields into an executable matrix without enabling a live Go loop.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` expands `controlExecutableCases.stepLimits.expected` with a 9-row override/delegate matrix. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the matrix and `go-runtime-conformance.test.ts` derives expected rows from the step-limit fixture/control replay. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes default/user/session/turn/planner/headless/zero/delegate rows, including delegate min-floor. |
| Product boundary | No live Go agent loop, Reasonix controller/session protocol, renderer-visible Go route, default Go backend, public auto-plan setting, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Step-limit override/delegate matrix | contract-reimplement | Accepted as analytix-owned evidence for override precedence, planner/headless limits, zero-default disable guard, delegate parent-half, and delegate floor. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix controller/session protocol or public auto-plan setting | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Live Go loop/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Packaged step-limit QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Parallel research checkpoint:

| Track | Read-only result |
| --- | --- |
| A. Reasonix agent-kernel | `881b2f2f..9ada1417` contains auto-plan user-level config and classifier rebuild deltas; classify the contract as "classifier enable/disable must not reuse stale router/classifier/cache state". Reject Reasonix public config/SessionAPI/controller protocol. |
| B. Kun baseline | Kun v0.2.13/v0.2.14/current Kun entry surfaces do not justify top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer in analytix. Keep those as reject/defer unless a future analytix spec says otherwise. |
| C. Runtime/cache | DeepSeek/OpenAI/Anthropic/custom cache and request-shape evidence is already strong at fixture/oracle level, but live provider superiority remains deferred. Next release guard should cover repeat-turn cache hit stability and no provider URL/body regression. |
| D. MCP/tool/sub-agent | Absorb approval/user-input/MCP/sub-agent invariants as analytix runtime contracts; continue rejecting Reasonix MCP-indexer public protocol and top-level navigation. |
| E. Go runtime | Continue pure Go G5 executable shadows for full-loop control areas such as compaction, history repair, tool-call/result pairing, and checkpoint rewind. Live Go loop, G6, and default backend stay deferred/rejected. |
| F. QA/docs | Release evidence must keep command gates, forbidden-surface scans, and explicit rejected claims close to each batch; D-0147 release evidence now records non-cached Go validation. |

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go agent loop,
packaged step-limit QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0144 Go G5 MCP Core Lifecycle Boundary Seal Control Shadow

Goal:

```text
Seal MCP core lifecycle provider identity and product-boundary flags inside an
executable Go shadow case without enabling a Go MCP client or exposing
Reasonix MCP-indexer protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `providerId: "mcp:research"` and product-boundary flags to `controlExecutableCases.mcpCoreLifecycle` plus expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates provider/boundary fields and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP core lifecycle boundary output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer public protocol, top-level MCP-indexer route, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP core lifecycle provider/boundary seal | contract-reimplement | Accepted as analytix-owned MCP lifecycle safety evidence: provider identity is `mcp:research` and boundary flags stay false. |
| Go shadow replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Reasonix MCP-indexer public protocol/top-level route | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, or top-level MCP-indexer navigation. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Credentialed MCP QA / release walkthrough | defer | Preserve as future release evidence, not claimed by this offline fixture. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0128 Go G5 MCP Approval Annotation Control Shadow

Goal:

```text
Promote MCP approval annotation/no-execute evidence from summary output into an
executable Go shadow case without enabling Go MCP or Go approval managers.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpApprovalAnnotations` with a minimal TS-owned approval annotation oracle and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP approval annotation output. |
| Product boundary | No live Go MCP client, live Go approval manager, renderer-visible Go route, default Go backend, Reasonix MCP-indexer/approval protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP approval annotation replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Destructive/open-world no-execute gate | contract-reimplement | Expected output stays tied to TS-owned MCP lifecycle oracle and analytix approval gate semantics. |
| Live Go MCP client / Go approval manager | defer | Keep future work behind G5/G6 gates. |
| Reasonix MCP-indexer/approval public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
live Go approval manager, credentialed MCP matrix, packaged desktop MCP/
approval QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0129 Go G5 MCP Search Workspace Boundary Control Shadow

Goal:

```text
Promote MCP search workspace trust-boundary evidence from summary output into
an executable Go shadow case without enabling Go MCP or a public MCP-indexer.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.mcpSearchWorkspaceBoundary` with a minimal TS-owned workspace-boundary oracle and expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the control oracle and `go-runtime-conformance.test.ts` derives expected values from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify typed MCP search workspace boundary output. |
| Product boundary | No live Go MCP client, renderer-visible Go route, default Go backend, Reasonix MCP-indexer protocol, top-level MCP-indexer route/navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP search workspace boundary replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Trusted/untrusted workspace search contract | contract-reimplement | Expected output stays tied to TS-owned MCP lifecycle oracle and internal meta-tool semantics. |
| Live Go MCP client/default backend | defer / reject | Keep future work behind G5/G6 gates and do not make Go default. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or top-level navigation. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go MCP client,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0110 MCP Refresh Catalog Drift Replay

Goal:

```text
Promote MCP search catalog refresh/currentness drift into the executable MCP
lifecycle oracle and G5 shadow replay without exposing a public MCP-indexer
surface.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source oracle | `packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-oracle.json` records initial/expanded MCP search catalog names and expected refresh drift. |
| Runtime proof | `packages/runtime/tests/mcp-tool-lifecycle-oracle.test.ts` executes `mcp_refresh_catalog` after fake-client catalog expansion and verifies `totalIndexed: 2` plus `catalogDrift: true`. |
| G5 replay | `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, and `go-runtime-conformance.test.ts` require `mcpReplay.searchRefreshDrift`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` maps the TS-owned refresh drift fixture into shadow-only output with `topLevelRouteExposed: false`. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP refresh catalog drift | contract-reimplement | Accepted as an analytix-owned MCP search meta-tool invariant. |
| G5 refresh drift replay | code-port-and-adapt | Accepted as pure Go shadow output derived from TS fixtures. |
| Reasonix MCP-indexer public lifecycle | reject | Do not expose upstream MCP-indexer protocol, route, or top-level entry. |
| Live Go MCP client / default backend | defer / reject | Keep TypeScript runtime authoritative until G5/G6 gates. |
| Credentialed MCP matrix / packaged QA | defer | Keep this batch deterministic and fake-client only. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is oracle/shadow proof. It does not implement live Go MCP execution,
Reasonix MCP-indexer protocol parity, credentialed MCP server QA, packaged
desktop MCP QA, Go default backend readiness, or release readiness.
```

## 2026-06-22 - D-0111 Thread/SSE Route Auth Replay

Goal:

```text
Promote runtime thread/SSE route authorization into the G2/G5 replay oracle so
fork/resume/archive/search/SSE evidence also proves missing-token rejection.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G2 fixture | `go-g2-route-replay-oracle.json` adds `events-unauthorized-since-seq` with `auth: none` and 401 response. |
| TS conformance | `go-runtime-conformance.test.ts` omits the runtime token for `auth: none` routes and verifies the TS HTTP router response. |
| G5 replay | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `routeStatusReplay.auth` and an unauthorized status count. |
| Go shadow | `shadow_test.go` respects route auth mode; `shadow_g5.go` replays the auth summary without exposing a Go route. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Thread/SSE route auth replay | contract-reimplement | Accepted as analytix runtime-token boundary evidence. |
| G5 auth summary | code-port-and-adapt | Accepted as shadow-only replay from G2 route fixtures. |
| Reasonix SessionAPI/public route protocol | reject | Do not expose upstream thread/session protocol. |
| Renderer-visible Go route/default backend | reject | Keep Go shadow-only. |
| Packaged desktop route QA | defer | Keep this batch deterministic and oracle-backed. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is route-oracle proof. It does not implement live Go HTTP serving,
Reasonix SessionAPI parity, renderer-visible Go routes, packaged desktop
restart/resume QA, default Go backend readiness, or release readiness.
```

## 2026-06-22 - D-0112 Runtime Settings Legacy-Agent Pollution Guard

Goal:

```text
Prevent Reasonix/Kun-shaped agent settings from being saved through the active
top-level runtime settings path.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime normalization | `src/shared/app-settings-runtime.ts` strips `agent`, `agentProvider`, `agents`, `deepseek`, and `reasonix` when they appear inside `runtime`. |
| Shared settings proof | `src/shared/app-settings.test.ts` proves runtime-key stability and absence of rejected fields after normalization. |
| IPC patch proof | `src/main/ipc/app-ipc-schemas.test.ts` proves settings patches reject Reasonix-shaped keys inside `runtime`. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Runtime settings pollution guard | contract-reimplement | Accepted as analytix top-level `runtime` sovereignty proof. |
| Reasonix local/project agent config | reject | Do not persist upstream agent config shapes through active runtime settings. |
| Legacy Kun/DeepSeek agent settings fallback | reject | Do not recreate old agent settings envelopes or bridge fallbacks. |
| Import compatibility notes | document-only | Explicit migration/import code can still read legacy shapes before rewriting them. |

Validation:

```text
npm run test -- src/shared/app-settings.test.ts -t "Reasonix auto-plan" --no-file-parallelism --maxWorkers=1
npm run test -- src/main/ipc/app-ipc-schemas.test.ts -t "legacy.*settings|agent-shaped" --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is settings schema/normalization proof. It does not add user-visible
auto-plan settings, Reasonix config parity, packaged settings QA, or release
readiness.
```

## 2026-06-22 - D-0113 Task-Job Route Executable Control Shadow

Goal:

```text
Promote existing task-job route executable evidence from G5 summary replay into
Go typed control executable shadow.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.routeExecutable`. |
| TS schema/test | `runtime-parity-fixtures.ts` and `go-runtime-conformance.test.ts` tie the control case to `task-job-orchestration-oracle.routeExecutable`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed route executable output and `shadow_test.go` compares it with the TS-owned expected result. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Route executable control replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Internal task-job route contract | contract-reimplement | Kept behind analytix runtime fixtures and `analytix serve` boundaries. |
| Reasonix public job/session protocol | reject | Do not expose public Reasonix task/job routes. |
| Live Go Job Manager/default backend | defer / reject | G5 remains shadow-only until G6 gates pass. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not add live Go task-job routes,
renderer-visible Go routes, default backend readiness, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0114 Task-Job Lifecycle Control Shadow

Goal:

```text
Promote task-job foreground/background/wait-output-kill lifecycle evidence from
G5 summary replay into Go typed control executable shadow.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.lifecycle`. |
| TS schema/test | `runtime-parity-fixtures.ts` and `go-runtime-conformance.test.ts` derive lifecycle control expected output from the task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed lifecycle output and `shadow_test.go` compares it with the TS-owned expected result. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Task-job lifecycle control replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Internal job lifecycle contract | contract-reimplement | Kept behind analytix-owned fixtures and runtime contracts. |
| Reasonix public job/session lifecycle protocol | reject | Do not expose public Reasonix lifecycle routes. |
| Live Go Job Manager/default backend | defer / reject | G5 remains shadow-only until G6 gates pass. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not add live Go task-job routes,
renderer-visible Go routes, default backend readiness, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0115 Planner-Executor Control Shadow

Goal:

```text
Promote planner/executor failure, cancellation, output-offset, and transcript
propagation evidence from G5 summary replay into Go typed control executable
shadow.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.plannerExecutor`. |
| TS schema/test | `runtime-parity-fixtures.ts` and `go-runtime-conformance.test.ts` derive planner/executor control expected output from the task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed planner/executor output and `shadow_test.go` compares it with the TS-owned expected result. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Planner/executor control replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Internal planner/executor job contract | contract-reimplement | Kept behind analytix-owned fixtures and runtime contracts. |
| Reasonix public job/session protocol | reject | Do not expose public Reasonix lifecycle routes or task protocol. |
| Live Go Job Manager/default backend | defer / reject | G5 remains shadow-only until G6 gates pass. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not add live Go task-job routes,
renderer-visible Go routes, default backend readiness, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0116 Provider Cache Release Guard Control Shadow

Goal:

```text
Promote provider/cache release-guard evidence from G5 summary replay into Go
typed control executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerCacheReleaseGuard`. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes fixture-only tail averages, allowed-low cases, collapse counts, and status; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider cache release guard control replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Offline cache curve guard contract | contract-reimplement | Kept behind analytix-owned provider/cache fixtures and release-guard thresholds. |
| Reasonix provider protocol or public provider surface | reject | Do not expose Reasonix protocol or provider route names. |
| Live provider superiority / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live provider matrix, renderer-visible Go route, default
backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0117 Provider Usage Parser Control Shadow

Goal:

```text
Promote provider usage parser precedence from G5 summary replay into Go typed
control executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerUsageParser` with TS-owned provider usage inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed DeepSeek/OpenAI Responses/Anthropic/unsupported parser output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider usage parser precedence replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Analytix provider usage/cache contract | contract-reimplement | Kept behind provider-cache fixtures for native DeepSeek precedence, Responses cached tokens, Anthropic cache fields, and unsupported fallback. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live provider matrix, renderer-visible Go route, default
backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0118 Provider Request Shape Control Shadow

Goal:

```text
Promote provider request URL/body/tool-shape replay from G5 summary replay into
Go typed control executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerRequestShape` with TS-owned request-shape inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed URL/body/tool-shape summary output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider request-shape replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Analytix provider URL/body contract | contract-reimplement | Kept behind provider-cache fixtures for chat completions, Responses, Messages, and custom full endpoints. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live provider matrix, renderer-visible Go route, default
backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0119 Provider Cache Accounting Control Shadow

Goal:

```text
Promote provider cache accounting from G5 summary replay into Go typed control
executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerCacheAccounting` with minimal TS-owned usage-accounting inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed provider cache accounting output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider cache accounting replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Analytix provider cache accounting contract | contract-reimplement | Kept behind provider-cache fixtures for DeepSeek, Responses, Anthropic, and unsupported fallback. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live provider matrix, renderer-visible Go route, default
backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0120 Release/Package Identity Sovereignty Guard

Goal:

```text
Extend product-sovereignty evidence from app/runtime source into release and
package identity configuration without adding product functionality.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Product sovereignty scan | `scripts/scan-product-sovereignty.cjs` now scans root/runtime package manifests, `electron-builder.config.cjs`, `scripts`, `.github`, and `build` for Kun/Reasonix identity leaks. |
| Packaging test | `src/main/packaging-config.test.ts` asserts root package identity, app id, product name, artifact template, NSIS shortcut/uninstall names, and runtime CLI bin. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Release/package identity guard | contract-reimplement | Accepted as analytix-owned sovereignty evidence. |
| Kun/Reasonix release identity | reject | Do not allow upstream identity in package/release/script/workflow/build configuration. |
| Packaged Electron smoke | defer | Keep build-artifact QA separate from source/config guard evidence. |

Validation:

```text
npm run scan:product-sovereignty
npm run test -- src/main/packaging-config.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is source/config guard evidence. It does not perform packaged Electron
smoke testing, signing/notarization validation, or release readiness.
```

## 2026-06-22 - D-0121 Provider Streaming Control Shadow

Goal:

```text
Promote provider streaming usage replay from G5 summary replay into Go typed
control executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerStreaming` with minimal G3 provider streaming inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from the G3 provider oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed SSE/usage output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider streaming usage replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Analytix SSE usage/cache contract | contract-reimplement | Kept behind G3 provider streaming oracle fixtures. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider streaming matrix / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live streaming provider matrix, renderer-visible Go route,
default backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0122 Provider Offline Parity Seal Control Shadow

Goal:

```text
Promote provider offline parity seal from a G5 summary replay into Go typed
control executable shadow without enabling Go as a provider backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerOfflineParitySeal` with minimal provider-cache inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed offline parity output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider offline parity seal replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Stable prefix/cache telemetry contract | contract-reimplement | Kept behind analytix provider-cache oracle fixtures. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider superiority / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, live provider superiority matrix, renderer-visible Go route,
default backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0123 Session Route Status Control Shadow

Goal:

```text
Promote thread/session route status from a G5 summary replay into Go typed
control executable shadow without enabling Go HTTP routes.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.sessionRouteStatus` with G2 route oracle inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `go-g2-route-replay-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed session route status output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Thread/session route status replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Route/SSE/auth contract | contract-reimplement | Kept behind analytix G2 route replay fixtures. |
| Reasonix SessionAPI/job protocol | reject | Do not expose upstream session/job route names or public orchestration protocol. |
| Live Go HTTP routes/default backend | defer / reject | Keep TypeScript runtime authoritative until G5/G6 gates and rollback evidence exist. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add live Go
thread/session routes, renderer-visible Go route, default backend readiness,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0124 Provider Live-Local HTTP Control Shadow

Goal:

```text
Promote provider live-local HTTP proof from a G5 summary replay into Go typed
control executable shadow without enabling a live Go provider client.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerLiveLocalHttpProof` with minimal provider-cache inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed live-local HTTP proof output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider live-local HTTP proof replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Provider URL/body family coverage | contract-reimplement | Kept behind analytix provider-cache oracle fixtures. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live credential matrix / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, credentialed provider matrix, renderer-visible Go route,
default backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0125 Provider Drift Attribution Control Shadow

Goal:

```text
Promote provider drift attribution from a G5 summary replay into Go typed
control executable shadow without enabling a live Go provider client.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 control fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.providerDriftAttribution` with provider-cache drift inputs. |
| TS schema/test | `runtime-parity-fixtures.ts` validates the control case and `go-runtime-conformance.test.ts` derives expected output from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes typed drift attribution output; `shadow_test.go` compares output to expected. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Provider drift attribution replay | code-port-and-adapt | Accepted as G5 shadow-only Go control executable evidence. |
| Stable prefix drift contract | contract-reimplement | Kept behind analytix provider-cache oracle fixtures. |
| Reasonix public provider/cache protocol | reject | Do not expose upstream provider protocol, route names, or settings shape. |
| Live cache superiority / default Go backend | defer / reject | Credentialed provider matrix and Go backend readiness remain out of scope. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only executable shadow proof. It does not add a live Go
provider client, credentialed cache matrix, renderer-visible Go route, default
backend readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0104 Go G5 Parallel Task Dependency Executable Shadow

Goal:

```text
Promote `parallel_tasks` dependency validation from a G5 summary replay into
an executable Go shadow case without enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.parallelValidation`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the executable shape and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` executes DAG validation and invalid-case rejection; `shadow_test.go` compares output to the TS-owned expected oracle. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix public sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| `parallel_tasks` valid DAG order | code-port-and-adapt | Accepted as pure G5 executable shadow using analytix-owned fixture inputs. |
| Duplicate/self/cycle/unknown/single-task rejection | contract-reimplement | Accepted with exact expected error strings in the G5 fixture. |
| Reasonix public sub-agent/job protocol | reject | Do not expose SessionAPI, public job routes, or top-level Subagent/Workflow navigation. |
| Live Go Job Manager/default backend | defer / reject | Keep TypeScript runtime authoritative until G5/G6 gates and rollback evidence exist. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
Go task-job routes, packaged desktop sub-agent/task-job QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0105 Write Sidebar and Connect Phone Placeholder Sovereignty

Goal:

```text
Close product-sovereignty scan gaps around Write-mode sidebar entry surfaces
and stop generating transient `[Claw:...]` placeholders for Connect Phone
threads.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Route-surface oracle | `Workbench.route-surface.test.ts` now scans `WriteSidebar.tsx`, `WorkspaceModeTabs.tsx`, and `SidebarProjectsSection.tsx` in addition to the main shell, Sidebar, Plugin Marketplace, schedule, and store sources. |
| Reusable scan | `scripts/scan-product-sovereignty.cjs` includes the same Write/sidebar paths and stronger forbidden-entry / deprecated-identity token variants. |
| Connect Phone copy | `chat-store-claw-actions.ts` now creates placeholder titles as `[Connect Phone:...]`; `chat-store-claw-actions.test.ts` covers the mapped-conversation placeholder path. |
| Compatibility boundary | Legacy `[Claw:]` / `[Claw IM:]` recognizers remain compatibility-only for historical sessions. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Write sidebar entry-surface coverage | contract-reimplement | Accepted as product-sovereignty gate coverage. |
| Connect Phone placeholder copy | contract-reimplement | Accepted for newly generated transient thread titles. |
| Legacy Claw recognizers | code-port-and-adapt | Retained only to recover historical managed sessions. |
| Public Kun/Reasonix identity or deprecated bridge fallback | reject | Do not expose product identity, protocol, route, settings, or bridge aliases. |

Validation:

```text
npm run test -- src/renderer/src/store/chat-store-claw-actions.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

Remaining risks:

```text
This is scan/copy proof. It does not remove internal compatibility names,
legacy recovery support, packaged desktop Connect Phone QA, or release
readiness blockers.
```

## 2026-06-22 - D-0106 HTTP Auto-Plan Payload Boundary Guard

Goal:

```text
Prove Reasonix-shaped `autoPlan` / `auto_plan` start-turn payload fields cannot
implicitly enable Plan mode or advertise `create_plan`.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| HTTP contract | `packages/runtime/tests/http-server.test.ts` starts an agent thread through `/v1/threads/:id/turns` with `autoPlan: true` and `auto_plan: true`. |
| Runtime proof | The stored turn has no `mode: "plan"` override and no `guiPlan`; captured model tools do not include `create_plan`. |
| Test harness | `http-server-test-harness.ts` accepts an injected capture model while preserving default harness behavior. |
| Product boundary | Plan mode remains analytix-owned through explicit `mode: "plan"` / `guiPlan`, not Reasonix public config fields. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Reasonix `autoPlan` / `auto_plan` payload shape | reject | Strip/ignore at HTTP start-turn contract boundary. |
| Analytix explicit Plan mode | contract-reimplement | Keep `mode: "plan"` / `guiPlan` as the only accepted public turn contract. |
| Reasonix user/project auto-plan setting or controller rebuild protocol | document-only / defer / reject | Do not add product setting, local override, SessionAPI, or controller protocol. |
| Go backend involvement | defer | No Go runtime code changed; TypeScript HTTP/loop contract remains authoritative. |

Validation:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts -t "auto-plan payload" --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is an HTTP/loop negative fixture. It does not implement a user-visible
auto-plan setting, Reasonix controller rebuild parity, packaged desktop Plan
mode QA, or release readiness.
```

## 2026-06-22 - D-0107 Plan Step Cancel/Cache Boundary Guard

Goal:

```text
Prove explicit analytix Plan mode keeps step gating and provider cache
diagnostics stable when a follow-up model step is cancelled.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| AgentLoop contract | `packages/runtime/tests/loop.test.ts` starts an explicit `mode: "plan"` turn with a DeepSeek-style fake provider. |
| Step gating proof | Step 0 advertises read-only tools plus `create_plan`; the follow-up step, before `create_plan` succeeds, advertises only `create_plan`. |
| Cancel/cache proof | Interrupting the follow-up step aborts the turn; the next Plan turn still reports `cacheDiagnostics.prefixChanged: false` against the step-0 baseline. |
| Provider cache accounting | The fixture records DeepSeek-like `cacheHitTokens`, `cacheMissTokens`, `cacheHitRate`, provider, endpoint format, and model in usage events. |
| Product boundary | Plan mode remains analytix-owned through explicit `mode: "plan"` / `guiPlan`, not Reasonix public protocol. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Planner step tool narrowing | contract-reimplement | Accepted in analytix AgentLoop Plan mode. |
| Cancelled follow-up step cache stability | contract-reimplement | Aborted steps without usage do not advance the cache prefix baseline. |
| DeepSeek cache telemetry preservation | contract-reimplement | Provider-native hit/miss/rate fields remain in usage events. |
| Reasonix auto-plan controller/config protocol | reject | Do not expose upstream settings, SessionAPI, controller protocol, or public planner route. |
| Packaged Plan mode QA / live provider matrix | defer | Keep deterministic local loop proof in this batch. |
| Go backend involvement | defer | No Go runtime change; TypeScript loop remains authoritative until G5/G6 gates. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is a deterministic AgentLoop fixture. It does not add a product
auto-plan setting, Reasonix controller rebuild parity, packaged desktop Plan
mode QA, credentialed cache telemetry, Go default backend, or release
readiness.
```

## 2026-06-22 - D-0108 MCP Stdio Execution-Error Redaction

Goal:

```text
Prove model-visible MCP tool results do not leak server secrets when a stdio
MCP tool returns a protocol-level `isError` payload or throws.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime fix | `mcp-tool-provider.ts` now redacts MCP result payloads before they become `tool_result.output`, and rethrows non-abort MCP errors with redacted messages. |
| Executable MCP proof | `mcp-tool-lifecycle-oracle.test.ts` calls `mcp_codegraph_index_fail_diagnostic` through the executable stdio fake indexer. |
| Oracle | `mcp-tool-lifecycle-oracle.json` records the expected redacted error text and `leaksSecret:false`. |
| Schema | `runtime-parity-fixtures.ts` validates the new execution-error redaction fields. |
| Product boundary | The fix stays inside analytix MCP/tool contracts and does not expose Reasonix MCP-indexer protocol or navigation. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP protocol `isError` result redaction | contract-reimplement | Accepted: redact nested result text before persistence/model feedback. |
| MCP thrown error redaction | contract-reimplement | Accepted: non-abort MCP errors are rethrown as redacted messages so `LocalToolHost` cannot persist secrets. |
| Reasonix MCP-indexer public lifecycle | reject | Do not add top-level MCP-indexer route/navigation or upstream public protocol. |
| Credentialed MCP matrix / packaged desktop QA | defer | This batch is deterministic stdio MCP proof only. |
| Go backend involvement | defer | No Go code changed; TypeScript MCP provider remains authoritative for this runtime behavior. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is executable local MCP proof. It does not prove credentialed third-party
MCP servers, packaged desktop MCP QA, Go MCP client readiness, default Go
backend, or release readiness.
```

## 2026-06-22 - D-0109 Workflow/Create Loop Quarantine Scan

Goal:

```text
Make the dormant Workflow/Create Loop code quarantine explicit in tests and
the reusable product-sovereignty scan.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Route-surface test | `Workbench.route-surface.test.ts` now imports dormant workflow sources to prove they exist, then asserts their symbols are absent from top-level entry surfaces. |
| Product-sovereignty scan | `scan-product-sovereignty.cjs` adds a quarantine scan across Workbench, Sidebar, Write, Plugins, store, preload, tray menu, and settings shortcuts. |
| Forbidden symbols | The scan rejects `WorkflowCreateLoopView`, `runCreateLoopWorkflow`, `findPendingWorkflowGate`, and `analytix-create-loop` in entry surfaces. |
| Product boundary | Dormant workflow/create-loop code remains unmounted and is not promoted to an app route, menu, shortcut, tray, preload API, or runtime public protocol. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Workflow/Create Loop quarantine proof | contract-reimplement | Accepted as product-sovereignty evidence. |
| Dormant workflow runtime source | document-only / defer | Leave existing isolated code in place; do not expose it without a future product spec. |
| Kun-absent top-level entry | reject for current surface | Do not add Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation or route entries without a future analytix-native spec, UX rationale, route-surface tests, desktop QA, and scan update. |
| Reasonix public workflow protocol | reject | Do not expose upstream protocol, SessionAPI, or public route names. |

Validation:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Remaining risks:

```text
This is source-scan and route-surface proof. It does not remove dormant
workflow/create-loop files, audit external plugin metadata, replace packaged
desktop QA, or claim release readiness.
```

## 2026-06-22 - D-0102 Go G5 Task Parent Goal Evidence Executable Replay

Goal:

```text
Promote task/sub-agent parent-goal evidence from G5 summary into executable
Go shadow while keeping all task-job behavior behind analytix runtime contracts.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.taskJobs.parentGoalEvidence`. |
| TS binding | `go-runtime-conformance.test.ts` derives keys and expected metadata from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes active-goal ledger success and missing-goal error booleans. |
| Go test | `shadow_test.go` compares executable output with the TS-owned oracle. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Parent-goal executable replay | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Analytix evidence event keys | contract-reimplement | Accepted as `evidenceLedgered` / `evidenceLedgerError`, not Reasonix protocol. |
| Reasonix public sub-agent/job protocol | reject | Do not expose SessionAPI/job routes or top-level Subagent navigation. |
| Live Go Job Manager | defer | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
Go task-job routes, packaged desktop sub-agent QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0103 Product Sovereignty Scan Engineering

Goal:

```text
Turn the release-evidence forbidden surface scans into a reusable engineering
gate so future Kun/Reasonix absorption batches can prove analytix sovereignty
without hand-copying shell commands.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Scan command | `package.json` adds `scan:product-sovereignty`. |
| Scan implementation | `scripts/scan-product-sovereignty.cjs` checks forbidden top-level entries, identity/protocol leaks, deprecated bridge/settings fallback, default Go/Rust/Tauri enabling, Rust/Tauri files, and Connect Phone locale copy. |
| Renderer route surface | `Workbench.route-surface.test.ts` includes `PluginMarketplaceView.tsx` in the forbidden route-token source. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Product-sovereignty scan engineering | contract-reimplement | Accepted as a repeatable QA gate. |
| Plugin Marketplace route-surface scan | contract-reimplement | Accepted to prevent MCP-indexer/top-level entry drift through Plugins. |
| Reasonix/Kun public protocol exposure | reject | Scans stay negative and do not expose upstream surfaces. |
| Release readiness | defer | Scan engineering is necessary evidence, not release completion. |

Validation:

```text
npm run scan:product-sovereignty
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
The script improves repeatability but does not replace packaged desktop QA,
live provider/MCP credentials, signing/notarization, Windows NSIS verification,
or Go G6 readiness.
```

## 2026-06-22 - D-0097 Go G5 Approval/User-Input Route Body And Prompt Replay

Goal:

```text
Promote approval/user-input HTTP body and prompt-shape evidence into the G5
shadow replay summary without implementing a live Go gate manager.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source oracle | `packages/runtime/src/conformance/fixtures/approval-user-input-route-oracle.json` remains the authority for approval deny, user-input cancel, user-input submit, prompt questions/options, and SSE replay semantics. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `approvalUserInputReplay.approvalRoute`, `userInputSubmitRoute`, and `userInputCancelRoute`. |
| TS conformance | `go-runtime-conformance.test.ts` derives route-body and prompt replay fields from the approval/user-input oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same route-body and prompt summary from TS-owned fixtures. |
| Product boundary | No Reasonix public ask/session protocol, renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Approval deny request/response body replay | contract-reimplement | Accepted as analytix-owned HTTP route evidence. |
| User-input submit/cancel prompt shape replay | code-port-and-adapt | Accepted in G5 shadow summary from the existing route oracle. |
| SSE resolved event answer redaction | contract-reimplement | Preserve `includesAnswers:false` while the HTTP submit response still echoes answers. |
| Reasonix public ask/session protocol | reject | Keep approval/user-input behind analytix runtime contracts. |
| Live Go approval/user-input manager | defer | TypeScript runtime remains the execution authority until G5/G6 gates. |

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay proof. It does not prove packaged desktop approval-card
QA, live Go approval/user-input management, default Go backend readiness, or
release readiness.
```

## 2026-06-22 - D-0098 Go G5 MCP Lifecycle Detail Replay

Goal:

```text
Promote MCP reconnect, known-override, and search workspace-boundary evidence
into the G5 shadow replay summary without exposing MCP-indexer as a product
entry or Go runtime backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source oracle | `packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-oracle.json` remains the authority for reconnect, known override, search trust, and local indexer details. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `mcpReplay.backgroundReconnect`, `knownOverrideDiagnostics`, and `searchWorkspaceBoundary`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the replay fields from the MCP lifecycle oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes reconnect, known-override, and search workspace summaries from TS-owned fixtures. |
| Product boundary | No Reasonix MCP lifecycle protocol, top-level MCP-indexer entry, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Background reconnect details | contract-reimplement | Accepted as fixture-owned retry/error/restart summary. |
| Known override variants | code-port-and-adapt | Accepted for codegraph/codebase-memory cwd/priority/background-start diagnostics. |
| MCP search workspace boundary | contract-reimplement | Accepted for trusted/untrusted workspace and denied no-execute proof. |
| Reasonix MCP-indexer public lifecycle | reject | Keep MCP/indexer behind analytix runtime/tool contracts. |
| Live Go MCP client/indexer | defer | TypeScript runtime remains authoritative until G5/G6 gates. |

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay proof. It does not prove credentialed MCP matrix,
packaged desktop MCP QA, live Go MCP client readiness, or release readiness.
```

## 2026-06-22 - D-0099 Go G5 Provider Usage Parser Precedence Replay

Goal:

```text
Promote provider usage-parser precedence evidence into the G5 shadow replay
summary, proving DeepSeek native cache fields, OpenAI responses cached tokens,
Anthropic cache fields, and unsupported-provider absence semantics remain
stable.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source oracle | `packages/runtime/src/conformance/fixtures/provider-cache-oracle.json` remains the authority for raw provider response bodies and expected usage parsing. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `cacheReplay.usageParserReplay`. |
| TS conformance | `go-runtime-conformance.test.ts` computes parser-precedence replay from raw `responseBody` plus `expectedUsage`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same usage-parser replay from TS-owned fixtures; `shadow_g3g4.go` preserves expected absent cache fields. |
| Product boundary | No provider protocol change, Reasonix public protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| DeepSeek native cache precedence | contract-reimplement | Accepted: `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` win over `prompt_tokens_details.cached_tokens`. |
| OpenAI responses cached tokens | contract-reimplement | Accepted: `input_tokens_details.cached_tokens` produces hit/miss and preserves reasoning tokens. |
| Anthropic cache fields | contract-reimplement | Accepted: read/creation cache fields are included in prompt/cache accounting. |
| G5 usage-parser replay | code-port-and-adapt | Accepted as shadow-only proof from provider-cache oracle. |
| Live credentialed provider matrix | defer | Keep separate from fixture-only proof. |
| Go provider client/default backend | reject / defer | Do not enable Go provider client or default Go backend before G5/G6 gates. |

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture/shadow proof. It does not prove credentialed provider matrix,
packaged provider settings QA, live Go provider client readiness, or release
readiness.
```

## 2026-06-22 - D-0100 Go G5 Auto-Router Classifier Currentness Replay

Goal:

```text
Promote post-881 auto-plan classifier currentness/rebuild evidence into the G5
shadow gate without adding Reasonix public auto-plan settings or controller
protocols.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime constants | `packages/runtime/src/loop/auto-model-router.ts` remains the source for classifier model, prompt, timeout, request shape, and fingerprint. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `controlExecutableCases.autoRouterClassifier`. |
| TS conformance | `go-runtime-conformance.test.ts` compares the fixture to live `AUTO_MODEL_ROUTER_*` constants and `buildAutoModelRouterFingerprint()`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` replay the classifier currentness/fallback output from TS-owned fixtures. |
| Product boundary | No public auto-plan setting, project/local override, Reasonix controller protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Classifier contract fingerprint currentness | contract-reimplement | Accepted: model/prompt/timeout/maxTokens/temperature/reasoning drift invalidates route-cache contract. |
| Isolated classifier request shape | contract-reimplement | Accepted: short JSON request has no prefix, tools, or context-instruction carryover. |
| Timeout fallback | contract-reimplement | Accepted: timed-out classifier aborts and falls back to heuristic concrete model/reasoning. |
| G5 auto-router replay | code-port-and-adapt | Accepted as shadow-only evidence from TS-owned runtime constants. |
| Reasonix public auto-plan setting/config/controller API | reject | Do not import `reasonix config auto-plan`, project overrides, SessionAPI, or controller protocol. |
| Product planner/auto-plan toggles | defer | Future product controls must use top-level `runtime` settings and `window.analytix`. |

Focused validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is runtime-constant/shadow proof. It does not prove product auto-plan
settings, packaged desktop planner QA, Go auto-router parity, or release
readiness.
```

## 2026-06-22 - D-0088 G5 Provider Streaming Usage Replay

Goal:

```text
Carry G3 provider streaming and usage-event evidence into the G5 full-loop
shadow summary without changing live provider behavior.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `providerStreamingReplay`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the replay shape; `go-runtime-conformance.test.ts` derives it from the G3 provider oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` parses the G3 SSE `usage` event and compares it with the expected provider usage case. |
| Product boundary | No provider request/stream parser change, renderer-visible Go route, default Go backend, Reasonix protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| G3 streaming usage replay in G5 | contract-reimplement | Accepted as `providerStreamingReplay` from TS-owned G3 oracle data. |
| Go SSE usage payload parsing | code-port-and-adapt | Go shadow parses the `usage` SSE data payload and compares declared usage fields. |
| DeepSeek cache telemetry visibility | contract-reimplement | Accepted as fixture-only full-loop evidence for `deepseek-prompt-cache`. |
| Live provider/client behavior | reject | No provider request/stream parser, settings, bridge, or runtime server behavior changes. |
| Credentialed provider matrix / default Go backend | defer / reject | Keep outside this shadow-only batch. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture-only shadow proof. It does not prove live provider credentials,
packaged provider settings QA, Go provider client readiness, Go G5/G6 parity,
or release readiness.
```

## 2026-06-22 - D-0089 G5 Task-Job Route Executable Replay

Goal:

```text
Promote internal task-job wait/output/kill route evidence into the TS-owned
oracle and G5 shadow summary without exposing live Go routes.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` adds `routeExecutable`. |
| Runtime test | `task-job-orchestration-oracle.test.ts` now asserts route responses from `routeExecutable`. |
| G5 conformance | `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, and `go-runtime-conformance.test.ts` require `jobReplay.routeExecutable`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the route executable summary from the task-job oracle. |
| Product boundary | No live Go task-job route, public sub-agent/job protocol, renderer-visible Go route, default Go backend, Reasonix SessionAPI, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Internal task-job route responses | contract-reimplement | Accepted as analytix-owned `/v1/runtime/task-jobs/*` oracle fields. |
| Go route executable replay summary | code-port-and-adapt | Accepted as G5 shadow-only `jobReplay.routeExecutable`. |
| Live Go Job Manager/routes | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public sub-agent/job protocol | reject | Do not expose upstream protocol, route names, or top-level Subagent/Workflow entry. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is route executable shadow proof. It does not implement a live Go Job
Manager, Go task-job route server, packaged desktop sub-agent/task-job QA,
Go G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0092 G5 MCP Live-Local Indexer Summary Replay

Goal:

```text
Make MCP live-local indexer proof visible in G5 replay summary without adding a
live Go MCP client or public MCP-indexer surface.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `mcpReplay.liveLocalIndexer`. |
| TS binding | `go-runtime-conformance.test.ts` derives active paths, tombstone count, restart flag, secret redaction, and no-leak evidence from `runLiveLocalIndexerProof`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the nested live-local indexer summary from the TS MCP lifecycle oracle. |
| Product boundary | No live Go MCP client, MCP route, Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP live-local indexer replay summary | code-port-and-adapt | Accepted as shadow summary derived from TS-owned lifecycle fixtures. |
| Secret-safe diagnostics in G5 replay | contract-reimplement | Accepted as `leaksSecret:false` plus redacted diagnostic in `mcpReplay.liveLocalIndexer`. |
| Live Go MCP client / MCP-indexer route | reject | Do not implement from this evidence. |
| Credentialed MCP matrix | defer | Keep real MCP server QA outside this batch. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay over a TS-owned live-local fixture. It does not prove live
Go MCP readiness, credentialed MCP server parity, packaged MCP QA, Go G5/G6
parity, or release readiness.
```

## 2026-06-22 - D-0096 G5 Task Tool Contract Boundary Replay

Goal:

```text
Make `task` and `parallel_tasks` internal tool-contract boundaries replayable
in G5 without exposing a public sub-agent/job protocol or top-level route.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `jobReplay.toolContractBoundary`. |
| TS binding | `go-runtime-conformance.test.ts` derives task/parallel field names and internal safety flags from the task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads full task/parallel tool-contract fields and emits the boundary summary. |
| Product boundary | No planner product toggle, live Go Job Manager, public sub-agent/job protocol, Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer top-level route, default backend, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Task/parallel tool contract replay | code-port-and-adapt | Accepted as G5 shadow summary derived from TS-owned task-job fixtures. |
| Internal-runtime-only and permission/dependency gates | contract-reimplement | Accepted as analytix-owned evidence that task orchestration remains internal and gated. |
| Reasonix public sub-agent/job protocol | reject | Do not expose SessionAPI/job protocol, renderer-visible Go routes, or top-level Subagent/Workflow navigation. |
| Live Go Job Manager | defer | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay over TS-owned task tool-contract evidence. It does not
prove live Go Job Manager readiness, packaged task/sub-agent QA, Go G5/G6
parity, or release readiness.
```

## 2026-06-22 - D-0095 G5 Planner-Executor Detail Replay

Goal:

```text
Make planner-executor failure/cancel/offset/transcript details replayable in
G5 without changing planner behavior or exposing a public sub-agent/job
protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require extra `jobReplay.plannerExecutor` detail fields. |
| TS binding | `go-runtime-conformance.test.ts` derives skipped reason, cancel reason, output-offset job count, and transcript-propagation job count from the task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads the same `TaskPlannerExecutorOracle` fields and emits the summary. |
| Product boundary | No planner product toggle, live Go Job Manager, public sub-agent/job protocol, Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer top-level route, default backend, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Planner-executor detail replay | code-port-and-adapt | Accepted as G5 shadow summary derived from TS-owned task-job fixtures. |
| Dependency failure/cancel reasons | contract-reimplement | Accepted as analytix-owned evidence for planner/executor failure and cancellation propagation. |
| Reasonix planner/sub-agent public protocol | reject | Do not expose SessionAPI/job protocol, renderer-visible Go routes, or top-level Subagent/Workflow navigation. |
| Live Go Job Manager | defer | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay over TS-owned planner-executor evidence. It does not
prove live Go Job Manager readiness, packaged planner/sub-agent QA, Go G5/G6
parity, or release readiness.
```

## 2026-06-22 - D-0094 G5 Task Parent Goal Evidence Replay

Goal:

```text
Make task/sub-agent parent-goal evidence replayable in G5 without exposing a
public sub-agent/job protocol, live Go Job Manager, or top-level route.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `jobReplay.parentGoalEvidence`. |
| TS binding | `go-runtime-conformance.test.ts` derives active-goal requirement and evidence ledger/error event keys from the task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads `parentGoalEvidence` from the task-job oracle and emits the same summary. |
| Product boundary | No live Go Job Manager, public sub-agent/job protocol, Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer top-level route, default backend, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Parent goal evidence replay | code-port-and-adapt | Accepted as G5 shadow summary derived from TS-owned task-job fixtures. |
| Analytix evidence event keys | contract-reimplement | Accepted as `evidenceLedgered` / `evidenceLedgerError`, proving parent-goal updates stay on analytix-owned event metadata. |
| Reasonix public sub-agent/job protocol | reject | Do not expose SessionAPI/job protocol, renderer-visible Go routes, or top-level Subagent navigation. |
| Live Go Job Manager | defer | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay over TS-owned task-job evidence. It does not prove live
Go Job Manager readiness, packaged sub-agent/task-job QA, Go G5/G6 parity, or
release readiness.
```

## 2026-06-22 - D-0093 G5 MCP Approval Annotation Replay

Goal:

```text
Make destructive/open-world MCP approval metadata replayable in G5 without
adding a live Go approval manager, live MCP client, or public MCP protocol.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `mcpReplay.approvalAnnotations`. |
| TS binding | `go-runtime-conformance.test.ts` derives normalized tool name, destructive/open-world flags, approval id, deny decision, approval result kind, and no-execute from the MCP lifecycle oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the approval annotation summary from the same oracle. |
| Product boundary | No live Go approval manager, live Go MCP client, MCP route, Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, default backend, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| MCP destructive/open-world approval metadata | code-port-and-adapt | Accepted as G5 shadow summary derived from TS-owned lifecycle fixtures. |
| Denied MCP approval no-execute evidence | contract-reimplement | Accepted as `deniedNoExecute:true` in `mcpReplay.approvalAnnotations`. |
| Live Go approval manager / MCP client | reject | Do not implement from this evidence. |
| Credentialed MCP matrix | defer | Keep real MCP server QA outside this batch. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is shadow replay over TS-owned approval metadata. It does not prove live
Go approval-manager readiness, live MCP readiness, credentialed MCP server
parity, packaged MCP QA, Go G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0091 G5 Provider Live-Local Proof Summary Replay

Goal:

```text
Make provider live-local HTTP proof replayable as oracle metadata and G5 shadow
summary without implementing a live Go provider client.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider oracle | `provider-cache-oracle.json` records `liveLocalHttpProof` with fixture-only, no-live-credentials, original-base-url, counts, endpoint formats, provider families, and no-live-superiority fields. |
| Runtime proof | `provider-cache-proof.test.ts` validates the metadata against the actual usage/request-shape oracle arrays. |
| G5 shadow | `go-g5-full-loop-oracle.json`, `runtime-parity-fixtures.ts`, `go-runtime-conformance.test.ts`, and `packages/runtime-go/shadow_g5.go` add `cacheReplay.liveLocalHttpProof`. |
| Product boundary | No provider client behavior, settings, bridge, runtime route, default Go backend, live Go provider client, Reasonix protocol, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Live-local proof metadata | contract-reimplement | Accepted as TS-owned oracle metadata derived from usage/request-shape cases. |
| G5 summary replay | code-port-and-adapt | Go computes the compact summary from the TS oracle, not by making provider HTTP requests. |
| Credentialed provider matrix | defer | Keep real external credentials outside this batch. |
| Live Go provider parity | reject | Do not claim or implement from this summary. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is a replayable summary of no-credential local HTTP proof. It does not
prove real provider credentials, live provider/cache superiority, packaged
provider settings QA, live Go provider readiness, Go G5/G6 parity, or release
readiness.
```

## 2026-06-22 - D-0090 Provider Live-Local HTTP Executable Proof

Goal:

```text
Raise provider usage/request-shape evidence from fixture/fake-fetch checks to
local HTTP executable proof without using live provider credentials.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime test | `provider-cache-proof.test.ts` executes all `providerUsageCases` through a local HTTP provider and validates usage output. |
| Request-shape proof | The same test executes all `requestShapeCases` through local HTTP while preserving original provider URL/host request construction. |
| Provider coverage | DeepSeek native cache, unsupported OpenAI-compatible unknown cache, OpenAI Responses cached tokens, Anthropic cache fields, and custom full endpoints are all covered. |
| Product boundary | No provider client behavior, settings, bridge, runtime route, default Go backend, Reasonix protocol, Kun identity, Rust/Tauri path, or hidden product entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Live-local provider usage proof | contract-reimplement | Accepted as a test-only executable proof over the existing provider-cache oracle. |
| Live-local request URL/header/body proof | contract-reimplement | Accepted for all request-shape cases without changing runtime behavior. |
| Provider-specific host currentness | code-port-and-adapt | Preserve original provider host while forwarding transport to local HTTP so DeepSeek `thinking` behavior remains covered. |
| Credentialed live provider matrix | defer | Keep real API credentials and external load outside this batch. |
| Live provider superiority / default Go backend | reject | Do not claim from this evidence. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is live-local HTTP proof. It does not prove real provider credentials,
external cache hit behavior, packaged provider settings QA, live provider/cache
superiority, Go provider client readiness, Go G5/G6 parity, or release
readiness.
```

## 2026-06-22 - D-0083 Cache Drift Attribution Replay

Goal:

```text
Promote provider-cache drift attribution into G5 shadow replay so cache
diagnostics can distinguish allowed tool/provider/model drift from forbidden
stable-prefix pollution.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Provider oracle | `provider-cache-oracle.json` remains the source for previous/current shapes, expected reasons, and unsupported telemetry. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `cacheReplay.driftAttribution`. |
| TS conformance | `go-runtime-conformance.test.ts` derives every drift flag from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes system/prefix stability, changed tool/provider/model/endpoint flags, and unknown cache-hit-rate status. |
| Product boundary | No dynamic workspace context, selected text, credentials, Reasonix sidecar material, live provider matrix, Go route, default backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Cache drift reason replay | contract-reimplement | Accepted as fixture-derived G5 shadow evidence. |
| Stable system/prefixItems proof | contract-reimplement | Accepted to prove drift does not come from stable prefix pollution. |
| Tool/provider/model/endpoint drift flags | code-port-and-adapt | Accepted as Go shadow calculations from TS-owned oracle fields. |
| Dynamic context in stable prefix | reject | Keep workspace snippets, timestamps, selected text, and credentials out of stable prefix/cache-visible diagnostics. |
| Live provider superiority | defer / reject | Keep credentialed provider matrix and superiority claims out of this fixture-only proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline drift-attribution proof. It does not prove live provider cache
rates, packaged settings QA, Go provider client readiness, Go G5 parity, or
release readiness.
```

## 2026-06-22 - D-0084 G3 Cache Drift Attribution Replay

Goal:

```text
Move cache drift attribution evidence into the G3 provider streaming/usage/
cache gate so provider conformance can explain drift before the G5 summary.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G3 fixture | `go-g3-provider-streaming-usage-cache-oracle.json` adds raw `cacheDriftAttribution` input and compact expected output. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the G3 drift input/output and `go-runtime-g3-g4-conformance.test.ts` derives both from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` computes drift attribution in `BuildG3ProviderConformanceOutput`. |
| Product boundary | No provider URL/body behavior, stream parser, live provider matrix, Go route, default backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| G3 provider drift attribution | contract-reimplement | Accepted as provider gate evidence tied to TS provider-cache oracle. |
| Go G3 drift computation | code-port-and-adapt | Accepted as pure shadow computation from raw shapes. |
| Stable prefix hygiene | contract-reimplement | Accepted through system/prefixItems stability flags. |
| Runtime provider behavior change | reject | Keep URL/body, usage parsing, and streaming behavior unchanged. |
| Live provider superiority/default Go backend | defer / reject | Keep credentialed provider QA and Go backend activation outside this proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline G3 provider-gate proof. It does not prove live provider cache
rates, packaged provider settings QA, Go provider client readiness, Go G5/G6
parity, or release readiness.
```

## 2026-06-22 - D-0085 G3 Provider Request Shape Replay

Goal:

```text
Move provider request URL/body/tool-shape evidence into the G3 provider gate so
future Go provider conformance cannot hide behind request-shape ids alone.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G3 fixture | `go-g3-provider-streaming-usage-cache-oracle.json` adds `requestShapeMatrix` and `expectedOutput.requestShapeSummary`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates request-shape matrix/summary and `go-runtime-g3-g4-conformance.test.ts` derives both from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` computes case count, exact URL count, endpoint/tool-shape families, custom full endpoint ids, and body-field family counts. |
| Product boundary | No provider URL/body behavior, stream parser, usage parser, live provider matrix, Go route, default backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| G3 request-shape matrix | contract-reimplement | Accepted as provider gate evidence tied to TS provider-cache oracle. |
| Go G3 request-shape summary | code-port-and-adapt | Accepted as pure shadow computation from request-shape cases. |
| OpenAI/Anthropic/custom non-regression | contract-reimplement | Accepted through endpoint/tool-shape and body-field family counts. |
| Runtime provider URL/body change | reject | Keep request construction, stream parsing, and usage parsing unchanged. |
| Live provider superiority/default Go backend | defer / reject | Keep credentialed provider QA and Go backend activation outside this proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline G3 provider-gate proof. It does not prove live provider
credentials, packaged provider settings QA, Go provider client readiness, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0086 G5 Provider Request Shape Replay

Goal:

```text
Carry provider request URL/body/tool-shape evidence from the G3 provider gate
into the G5 full-loop shadow summary.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `cacheReplay.requestShapeReplay`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the G5 summary and `go-runtime-conformance.test.ts` derives it from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same request-shape summary from provider-cache request-shape cases. |
| Product boundary | No provider URL/body/header/stream/usage behavior, live provider matrix, Go route, default backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| G5 request-shape replay summary | contract-reimplement | Accepted as full-loop summary evidence tied to TS provider-cache oracle. |
| Go G5 request-shape computation | code-port-and-adapt | Accepted as pure shadow computation from request-shape cases. |
| OpenAI/Anthropic/custom non-regression | contract-reimplement | Accepted through endpoint/tool-shape and body-field family counts. |
| Runtime provider URL/body change | reject | Keep request construction, headers, stream parsing, and usage parsing unchanged. |
| Live provider superiority/default Go backend | defer / reject | Keep credentialed provider QA and Go backend activation outside this proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline G5 full-loop summary proof. It does not prove live provider
credentials, packaged provider settings QA, Go provider client readiness, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0087 G5 Session Route Status Replay

Goal:

```text
Carry thread/session archive/search/fork/resume/SSE route semantics from the G2
HTTP/SSE oracle into the G5 full-loop shadow summary.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `sessionReplay.routeStatusReplay`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the route summary and `go-runtime-conformance.test.ts` derives it from G2 route cases. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes route/status counts, archive/search/read/update/fork/resume fields, and SSE frame/event evidence. |
| Product boundary | No renderer-visible Go route, default backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Archive/search route semantics | contract-reimplement | Accepted as G5 summary evidence tied to G2 HTTP route oracle. |
| Read/update/fork/resume route semantics | contract-reimplement | Accepted through latest seq, workspace, lineage, relation, and resume summary fields. |
| SSE replay/caught-up semantics | code-port-and-adapt | Accepted as Go shadow calculation from G2 SSE frames. |
| Renderer-visible Go route/default backend | reject | Keep Go route replay shadow-only. |
| Packaged desktop restart/resume QA | defer | Keep packaged QA and G5/G6 parity outside this fixture proof. |

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is offline G5 route-summary proof. It does not prove packaged desktop
restart/resume QA, live Go route handling, Go G5/G6 parity, or release
readiness.
```

## 2026-06-22 - D-0071 Auto-Router Request Contract Fingerprint Currentness

Goal:

```text
Close a narrow stale-classifier gap from Reasonix 2db7acf6 by proving
auto-router request contract drift changes analytix route-cache fingerprints.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Runtime helper | `packages/runtime/src/loop/auto-model-router.ts` exposes `AutoModelRouterFingerprintInput` and fingerprints request contract defaults explicitly. |
| Runtime proof | `packages/runtime/tests/auto-model-router.test.ts` proves default fingerprint stability plus max-token, temperature, and reasoning-effort drift. |
| Product boundary | No Reasonix auto-plan config, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Classifier request contract drift | contract-reimplement | Accepted in auto-router fingerprint helper and tests. |
| Rebuild-on-enable stale classifier guard | code-port-and-adapt | Accepted as route-cache key currentness, not as Reasonix controller rebuild. |
| Reasonix local/project auto-plan config | reject | Do not import upstream config roots or CLI shape. |
| Live desktop auto-plan toggle parity | defer | Future setting must be analytix-owned under top-level `runtime`. |

Validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is runtime currentness proof. It does not implement user-visible
auto-plan settings, packaged desktop controller rebuild QA, Go provider/router
execution, or release readiness.
```

## 2026-06-22 - D-0072 Connect Phone Copy Sovereignty Proof

Goal:

```text
Remove newly generated `Claw IM` user/model/log copy while keeping internal
compatibility names and legacy recognizers intact.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Prompt headings | `src/shared/app-settings-prompts.ts` now emits Connect Phone managed/agent headings and still unwraps legacy Claw headings. |
| Runtime title/log copy | `src/main/claw-runtime.ts` now creates `[Connect Phone:...]` thread titles and Connect Phone webhook log messages. |
| Schedule schema/comment | `src/main/claw-schedule-mcp-server.ts` and `src/shared/app-settings-types.ts` describe the channel as Connect Phone. |
| Renderer compatibility | `chat-store-helpers.ts` and `chat-store-claw-actions.ts` recognize new Connect Phone titles and legacy Claw titles. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Connect Phone product copy sovereignty | contract-reimplement | Accepted for new generated prompt/title/schema/log strings. |
| Legacy Claw history compatibility | code-port-and-adapt | Retained in recognizers/unwrap paths only. |
| Internal `claw` data fields | document-only | Kept as compatibility implementation details. |
| Public Claw/Kun/Reasonix identity | reject | Do not expose as product copy, bridge, settings, route, or navigation. |

Validation:

```text
npm run test -- src/shared/app-settings.test.ts src/main/schedule-runtime.test.ts src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts src/renderer/src/store/chat-store-helpers.test.ts src/renderer/src/store/chat-store-claw-actions.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is copy/compatibility proof. It does not rename internal compatibility
types, storage fields, IPC method names, or historical Claw-titled threads.
```

## 2026-06-22 - D-0073 Go G5 Durable Runner Restart Executable Shadow

Goal:

```text
Promote durable task-job restart continuity from a G5 summary into an
executable Go shadow case without enabling Go as a backend.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| G5 fixture | `packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.restartDrill`. |
| Runtime schema/test | `runtime-parity-fixtures.ts` validates the shape and `go-runtime-conformance.test.ts` derives expected values from `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` and `shadow_test.go` compute and verify rehydrated restart output. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Durable runner restart continuity | code-port-and-adapt | Accepted as pure G5 executable shadow. |
| Analytix task-job route semantics | contract-reimplement | Expected output stays tied to TS-owned task-job oracle. |
| Live Go Job Manager/default backend | defer / reject | Keep future work behind G5/G6 gates. |
| Reasonix public sub-agent/job protocol | reject | Do not expose upstream protocol or route names. |

Validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is executable shadow proof. It does not implement a live Go Job Manager,
Go task-job routes, packaged desktop sub-agent QA, or release readiness.
```

## 2026-06-22 - D-0070 Custom Chat Full Endpoint Request Shape

Goal:

```text
Prove custom full `/chat/completions` endpoints keep exact URL semantics and
OpenAI-compatible chat request shape in the provider/cache oracle.
```

Accepted items:

| Area | Files / evidence |
| --- | --- |
| Fixture | `packages/runtime/src/conformance/fixtures/provider-cache-oracle.json` adds `custom-chat-full-endpoint-request-shape`. |
| Runtime proof | `packages/runtime/tests/provider-cache-proof.test.ts` verifies exact URL, required/forbidden headers/body fields, and OpenAI tool shape. |
| Go shadow | `go-g3-provider-streaming-usage-cache-oracle.json` and `go-g5-full-loop-oracle.json` include the request-shape id for shadow replay. |
| Product boundary | No Reasonix provider protocol, renderer-visible Go route, default Go backend, bridge/settings fallback, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Classification:

| Delta / idea | Classification | Decision |
| --- | --- | --- |
| Custom `/chat/completions` full endpoint | contract-reimplement | Accepted in provider-cache oracle. |
| OpenAI-compatible body/header/tool shape | contract-reimplement | Accepted through fake-fetch request-shape proof. |
| Go request-shape replay | code-port-and-adapt | Accepted as G3/G5 shadow-only fixture id replay. |
| Responses/Messages fallback fields | reject | Prevented by required/forbidden field assertions. |
| Live provider matrix | defer | Keep credentialed provider QA separate. |

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Remaining risks:

```text
This is fixture/fake-fetch request-shape proof. It does not prove live provider
credentials, packaged settings QA, Go provider client readiness, or release
readiness.
```
