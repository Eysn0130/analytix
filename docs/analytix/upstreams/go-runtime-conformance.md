# Go runtime conformance plan

Status: Normative public-boundary invariants plus a chronological cutover and
conformance ledger. Historical stage rows are not current readiness evidence.

This file tracks the staged introduction and final default delivery of the Go
runtime backend for analytix. The Go runtime default must keep conforming to the
same renderer, preload, main, HTTP/SSE, provider, settings, and rollback
contracts.

Authoritative rules:

```text
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/upstreams/code-level-implementation-plan.md
```

## Current Principle

```text
Renderer -> window.analytix -> preload -> main -> runtime contract
```

The renderer must not know whether the runtime implementation is TypeScript or
Go. The public runtime CLI remains `analytix serve`.

Go is the only production analytix runtime backend after the final delivery
pass. The former explicit `ANALYTIX_RUNTIME_BACKEND=typescript` rollback path
is retired from executable production/package/build boundaries; remaining
TypeScript runtime oracles live only under `*-test-support` or shared
conformance helpers. P3A through P4C and the G0/G5 inventory did not scaffold
Go; they strengthened the TypeScript runtime contract fixtures that Go
reproduced before the fallback was removed.

The G1 through G5 rows below preserve historical stage names and earlier
"default backend pending" wording. The G6/final delivery rows explain the
cutover decision, but current readiness still requires current code and freshly
executed applicable gates. Deterministic Go default is complete, and
D-0251/D-0252/D-0253 live evidence is post-cutover validation rather than a
startup gate.

Rust is not a first-stage UI/runtime rewrite path and does not imply Tauri.
Rust may be considered later only as an isolated native helper for diff,
search, indexing, file watching, PTY/sandbox integration, or performance probes
when profiling proves the need. Renderer and preload contracts must remain
backend-neutral either way.

## 2026-07-01 - Goal Todo Evidence Final-Readiness Closure

Go runtime now owns the former goal/todo execution surface instead of relying
on TypeScript agent tools: active-goal turns advertise `get_goal`,
`create_goal`, `todo_list`, `todo_write`, `complete_step`, and `update_goal`;
ordinary non-goal prompts, including generic questions containing the word
`goal`, keep the bounded tool set. `complete_step` accepts Reasonix-style
structured evidence (`verification`, `diff`, `files`, `manual`), verifies
command/path receipts against durable tool results, rejects mismatched todo step
text / index pairs before evidence is appended, advances the matching todo
item, appends the goal evidence ledger, and replays
`goal_updated` / `todos_updated`. `update_goal status:"complete"` now fails
until evidence exists and todos are complete, and strict-completion goals must
record a requested self-check before terminal completion.

Relevant evidence:

```text
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/internal/server/durable_store.go
packages/runtime-go/runtime_server_test.go
```

Regression:

```text
go test . -run 'TestRuntimeServerDirectLightweightPromptsDoNotAdvertiseToolsToProvider|TestRuntimeServer(GoalToolsRequireEvidenceBeforeCompletion|GoalTodoCompleteStepAndFinalReadiness|CompleteStepRejectsMismatchedTodoIndexBeforeEvidence)' -count=1
go test ./...
```

## 2026-07-01 - Subagent Execution Identity Closure

Go child-agent runs now persist the full execution tuple on durable child-run
records: `providerId`, `model`, `endpointFormat`, `variant`, `modelSource`, and
a redacted `modelExecution` ref. This aligns the Go child-run store with the
Reasonix native-continuation safety properties and the OpenCode
provider/model/variant binding without importing their storage layout.

Relevant evidence:

```text
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/internal/jobs/lineage.go
packages/runtime-go/runtime_server_test.go
packages/runtime-go/internal/server/subagent_source_test.go
```

The regressions cover profile provider/model execution refs, `/runtime/tools`,
thread-summary and SSE replay preservation, source identity inheritance, and
provider/model/endpoint/variant drift rejection during `continue_from` /
`fork_from`.

## 2026-07-01 - Provider Fallback Hard-Fail Closure

Go runtime now validates inherited thread models, explicit request models, and
internal subagent child-run models before provider default fallback. The durable
store no longer fabricates `deepseek-chat` for model-less threads, so a missing
model stays visibly missing instead of becoming a hidden execution ref.
The shared/test-support `ModelExecutionRef` contract now follows the same
OpenCode-style hard binding: `providerId` is required, `source:"fallback"` and
`fallbackReason` are rejected, and the only default-runtime source is the
auditable `runtime-default` value with a concrete provider/model tuple.
Go child-run `modelExecution` metadata follows the same rule: missing
`providerId` or `modelId` now fails before child-run persistence, and unknown
model execution sources normalize to `runtime-default` rather than a silent
thread/default fallback label.

Relevant evidence:

```text
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/internal/server/durable_store.go
packages/runtime-go/internal/provider/provider.go
packages/runtime-go/runtime_server_test.go
packages/runtime-go/internal/server/model_execution_ref_test.go
packages/runtime-go/internal/server/durable_store_test.go
packages/runtime-go/internal/provider/provider_test.go
packages/runtime/src/contracts/model-execution-ref.ts
src/renderer/src/agent/analytix-contract.ts
packages/runtime/src/loop-test-support/agent-loop.ts
packages/runtime/src/delegation-test-support/delegation-runtime.ts
packages/runtime/tests/contracts.test.ts
packages/runtime/tests/child-agent-executor.test.ts
packages/runtime/tests/delegation-runtime.test.ts
packages/runtime/tests/loop.test.ts
```

The regressions cover stale thread-model rejection, subagent child model
rejection before child turn creation, no invented durable default model, and
bounded-provider validation for the execution config path. They also cover
provider-bound Go `modelExecution` refs, including `runtime-default` source
normalization. The TS test-support regressions cover schema rejection of
fallback execution refs and child/delegation rejection when an inherited
execution ref lacks a provider.

## 2026-07-01 - MCP Workspace Trust Closure

Go MCP loading now parses `trustScope` and `trustedWorkspaceRoots`, skips
workspace-scoped servers outside the current trusted roots, reports only
redacted trust diagnostics, and includes trust data in MCP fingerprints so
schema-cache reuse cannot bypass workspace trust changes.

Relevant evidence:

```text
packages/runtime-go/internal/mcp/manager.go
packages/runtime-go/internal/mcp/manager_test.go
```

This absorbs CodexDesktop-style plugin/resource boundary discipline without
absorbing auth bypass patches or changing the analytix MCP/plugin product
boundary.

## 2026-07-01 - Hub Marketplace Fallback Boundary

The Hub marketplace sync path no longer ships an implicit insecure IP fallback.
Fallback transport now requires an explicit
`ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_BASE_URL`, fallback TLS verification
is not disabled by default, and plugin package downloads still enforce SHA-256
before extraction.

Relevant evidence:

```text
src/main/services/hub-agent-marketplace-service.ts
src/main/services/hub-agent-marketplace-service.test.ts
```

This keeps CodexDesktop-style marketplace/resource layout as a positive
reference while rejecting hidden auth or transport bypass behavior.

## 2026-07-01 - Live Reasonix DeepSeek Parity Gate

The Go runtime now has a repeatable live parity command for the remaining
Reasonix speed/cache acceptance edge:

```text
npm run runtime:go:reasonix-live-parity -- --json
```

The command resolves the configured analytix DeepSeek credential from local
settings without printing it, runs the analytix live DeepSeek cache probe, runs
Reasonix's env-gated `TestRealDeepSeekCacheProbe`, and runs two isolated
Reasonix CLI `deepseek-pro` turns with `reasonix run --metrics`. It writes any
Reasonix credential only to a temporary `REASONIX_HOME/.env`, uses a temporary
home/workspace to avoid reading user Reasonix sessions, and deletes the
temporary roots after the run.

Latest live result on 2026-07-01:

```text
runtime_go_reasonix_live_parity.status:passed
analytix.deepseek_v4_pro.warm_elapsed_ms:762
analytix.deepseek_v4_pro.cache_hit_rate:0.9868480725623583
reasonix.cli.deepseek_v4_pro.warm_elapsed_ms:1752
reasonix.cli.deepseek_v4_pro.cache_hit_rate:0.9909121507909795
reasonix.provider.deepseek_v4_flash.warm_cache_rate:0.88
reasonix.provider.deepseek_v4_flash.reasoning_prompt_delta:521
```

This is live validation evidence, not a production code path. It proves the
current analytix Go runtime keeps DeepSeek cache hit behavior and warm-response
latency in the Reasonix range while retaining analytix's explicit
provider/model/profile boundary and secret redaction.

## Stages

| Stage | Scope | Status | Evidence |
| --- | --- | --- | --- |
| G0 | Contract inventory and TypeScript golden fixtures. | closed for inventory baseline; fixture hardening continues | 2026-06-20 G0/G5 inventory section below. |
| G1 | Go health/readiness/config/capabilities scaffold. | historical stage closed; current status is covered by G6 and formal runtime-go validation | Historical evidence is archive context below. Current evidence uses `scripts/runtime-go-product-regression.mjs`, `scripts/runtime-go-speed-cache-gate.mjs`, `scripts/runtime-go-preflight.mjs`, and `packages/runtime-go/cmd/runtime-server/main.go`. |
| G2 | Go read/list/search/archive/fork/session routes and SSE replay. | historical stage closed; current route replay is folded into product regression | Historical evidence is archive context below. Current evidence uses runtime route contract tests, `scripts/runtime-go-product-regression.mjs`, and `packages/runtime-go/internal/protocol/route_replay.go`. |
| G3 | Go model adapter, streaming, event recorder, and usage. | historical stage closed; current provider/usage/cache coverage is formalized under speed/cache and product regression gates | Historical evidence is archive context below. Current evidence uses `scripts/runtime-go-speed-cache-gate.mjs`, `packages/runtime-go/internal/provider/provider.go`, and `packages/runtime-go/internal/server/usage.go`. |
| G4 | Go tools, approvals, user input, MCP/plugin, shell/file/diff. | historical stage closed; current tools/MCP coverage is formalized under product regression and live evidence gates | Historical evidence is archive context below. Current evidence uses `packages/runtime-go/internal/agent/approval_user_input_contract.go`, `packages/runtime-go/internal/mcp/manager.go`, and `scripts/runtime-go-product-regression.mjs`. |
| G5 | Go full agent loop, compaction, cache accounting, resume, interrupt. | historical stage closed; current full loop and replay coverage is formalized under product regression and runtime-server tests | Historical evidence is archive context below. Current evidence uses `packages/runtime-go/cmd/runtime-server/main.go`, `packages/runtime-go/internal/server/runtime_server.go`, `packages/runtime-go/runtime_server_test.go`, and `scripts/runtime-go-product-regression.mjs`. |
| G6 | Go default backend and TypeScript runtime retirement. | Go default delivery accepted on 2026-06-25 after Electron dev app/runtime smoke, local Go runtime smoke, DeepSeek live provider probe, deterministic provider/bridge/settings/runtime tests, product-sovereignty scan, typecheck, runtime build, and full app build. The current closure pass retires TypeScript as a production rollback runtime: `scripts/runtime-go-ts-retirement-scan.mjs` proves zero production imports, package exports, build-config entries, packaged `dist` modules, or forbidden source-root modules for the former TS agent loop/server/model/tool/service runtime. Residual TS code is test-support/conformance-only. | `docs/analytix/upstreams/final-go-runtime-delivery-report.md`, `scripts/runtime-go-preflight.mjs`, `scripts/runtime-go-product-regression.mjs`, `scripts/runtime-go-speed-cache-gate.mjs`, `scripts/runtime-go-live-validation.mjs`, `scripts/runtime-go-live-evidence-collector.mjs`, `scripts/runtime-go-default-readiness-report.mjs`, `scripts/runtime-go-rollback-retirement-report.mjs`, `scripts/runtime-go-ts-retirement-scan.mjs`, `docs/analytix/upstreams/reasonix-integration-topology.md`, `docs/analytix/upstreams/go-runtime-retirement-checklist.md`, `docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json`, `packages/runtime-go/internal/server/runtime_server.go`, `packages/runtime-go/internal/provider/provider.go`, `packages/runtime-go/internal/mcp/manager.go`, `packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go`, `packages/runtime-go/internal/upstreamaudit/baseline_absorption.go`, `packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go`, `packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go`, `packages/runtime-go/internal/readiness/readiness_semantics.go`, `packages/runtime-go/internal/research/autoresearch_state.go`, `packages/runtime/src/contracts/capabilities.ts`, `packages/runtime/src/contracts/events.ts`, `packages/runtime/tests/go-runtime-conformance.test.ts`, `src/main/runtime/analytix-adapter.ts`, `src/main/runtime/analytix-adapter.test.ts`, `scripts/scan-product-sovereignty.cjs`. |

## 2026-06-23 - D-0250A Runtime-Go Domain Package Organization

D-0250A was a formal architecture cleanup. At that historical stage it was not
Go default cutover evidence, did not set `ANALYTIX_GO_RUNTIME_G6_READY=1`, and
did not remove the TypeScript fallback; the current G6 row supersedes that
archive state with Go-only production runtime retirement evidence.

The Go runtime package now follows the intended Analytix product layer plus Go
runtime core shape while borrowing Reasonix's stronger engine layering:

- `internal/server` owns HTTP contract handlers, route replay, durable stores,
  test-only replay handlers, runtime server contract routes, G5 compatibility
  replay, and production-candidate readiness routes.
- `internal/provider` owns provider request/stream/usage/cache/prefix logic,
  provider replay contracts, G3 conformance, and the test-only provider
  handler.
- `internal/mcp` owns MCP lifecycle/search/call/reconnect/redaction contracts,
  G4 conformance, and the test-only G4 manager handler.
- `internal/agent`, `internal/jobs`, `internal/goal`,
  `internal/research`, and `internal/readiness` own agent gates, lineage,
  goal evidence, AutoResearch state, and G6 readiness respectively.
- `internal/contracts` owns shared product/runtime boundary helpers.
- `internal/upstreamaudit` owns the formal Kun/Reasonix audit matrices and
  topology evidence without exposing Reasonix product entry points.

The root package is intentionally small. Historical `live_local*`, `shadow*`,
and numbered evidence shims have been removed or folded into formal contract
tests. Current root shims are limited to product/runtime domain compatibility
and are covered by `packages/runtime-go/root_shims_test.go`.

D-0250A leaves the desktop UI, preload bridge, top-level `runtime` settings,
product entry point, provider configuration surface, and `analytix serve`
unchanged. D-0250B is the next phase for real default cutover evidence:
credentialed provider matrix, credentialed MCP matrix, packaged QA, strict
D-0248 pass, and the explicit operator gate.

## 2026-06-24 - D-0250B Cutover Evidence Intake

D-0250B established the real cutover evidence intake and default-candidate
preflight contract. At that historical stage it did not enable Go by default
and did not remove the TypeScript fallback; the current G6 row supersedes that
archive state with Go-only production runtime retirement evidence.

New/updated evidence contract:

```text
scripts/d0250b-go-runtime-cutover-report.mjs
docs/analytix/upstreams/d0250b-go-runtime-cutover-report.json
docs/analytix/upstreams/d0250b-go-runtime-cutover-summary.md
src/main/runtime/analytix-adapter.ts
packages/runtime-go/internal/readiness/readiness.go
packages/runtime/src/contracts/capabilities.ts
```

The strict D-0248 gate now requires JSON evidence for:

- credentialed provider matrix: DeepSeek, OpenAI-compatible,
  Anthropic-compatible, and custom endpoint.
- credentialed MCP execution: connect, tool discovery/search, tool call,
  approval/user-input, reconnect, and redaction.
- packaged desktop QA: packaged startup, health, thread list, turn create,
  SSE replay, and the historical Go runtime candidate / TypeScript rollback
  gate. Current G6 replaces the rollback gate with Go-only runtime boundary and
  TS-retirement scan evidence.
- operator gate: `ANALYTIX_GO_RUNTIME_G6_READY=1` plus a passed
  `d0250b-go-runtime-operator-gate`, `d0251-go-runtime-operator-gate`, or
  `d0252-go-default-candidate-operator-gate` JSON file.

Evidence JSON is rejected when it contains secret-like keys or obvious bearer /
token query material. MCP evidence must not add a top-level MCP-indexer UI or
Reasonix public protocol surface. DeepSeek cache/prefix evidence remains a
DeepSeek-specific enhancement and does not narrow or rewrite OpenAI-compatible,
Anthropic-compatible, or custom endpoint request semantics.

Current result: D-0250B-intake code-stage closed the auditable evidence intake,
schema, and blocked-report path. D-0250C-live remained
post-cutover-live-validation-pending until the real credentialed provider/MCP
runs, packaged desktop QA, and operator JSON were supplied. The current G6 row
supersedes this historical fallback state.

## 2026-06-24 - D-0250C Reasonix Superiority Code Stage

D-0250C-code-stage closes the source-level superiority evidence matrix without
claiming live cutover. The Go runtime exposes the matrix through:

```text
/v1/runtime/info.capabilities.upstreamAbsorption.reasonixSuperiorityMatrixD0250C
```

New evidence/report paths:

```text
packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go
packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go
packages/runtime-go/d0250c_reasonix_superiority_matrix_test.go
scripts/d0250c-go-runtime-code-stage-report.mjs
docs/analytix/upstreams/d0250c-reasonix-superiority-matrix.json
docs/analytix/upstreams/d0250c-go-runtime-code-stage-report.json
docs/analytix/upstreams/d0250c-go-runtime-code-stage-summary.md
```

The matrix has 13 deterministic rows: `absorb=1`, `adapt=5`,
`keep-kun=5`, `reject=1`, and `defer=1`. Reasonix is absorbed/adapted only
for runtime engine strengths such as DeepSeek release/cache guard telemetry,
provider stream/usage/reasoning guards, MCP lazy/schema/reconnect/redaction,
job lineage, approval/user-input/history repair, and goal/AutoResearch
discipline. Analytix/Kun remains stronger and is kept for the product layer,
stable prefix-shape contract, provider endpoint family/custom-endpoint request
contract, MCP search/meta-tool boundary, and durable runtime event/SSE/thread
route baseline.

Current result: D-0250C-code-stage is closed with
`codeStageClosed:true`. The matrix-level `goDefaultCutoverCandidate:false` and
older `liveCutoverStatus:"blocked"` evidence remain safety markers for
post-cutover live validation only. D-0250C-live still requires real
credentialed provider matrix JSON, credentialed MCP execution JSON, packaged
desktop QA JSON, `ANALYTIX_GO_RUNTIME_G6_READY=1`, and operator gate JSON
before live-provider/MCP/packaged/operator claims can pass. No LLM
answer-quality evidence is used, no Reasonix public protocol/top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI is added. The current
G6 row supersedes this historical fallback state with Go-only production
runtime retirement evidence.

## 2026-06-23 - D-0249 Reasonix Integration Topology

D-0249 turns the Reasonix absorption map into code-level runtime-info evidence
instead of a standalone narrative. The Go runtime exposes:

```text
/v1/runtime/info.capabilities.upstreamAbsorption.reasonixIntegrationTopologyD0249
```

The topology binds five Reasonix engine capability families into existing
Analytix contracts:

- DeepSeek cache/prefix telemetry -> Chat provider path, provider settings,
  usage/cache diagnostics, and Go provider matrix evidence.
- Agent loop/job/sub-agent lineage -> Chat turns, `/goal`, task tools, and
  runtime event/SSE child lineage.
- MCP lifecycle/search/call/reconnect/redaction -> existing MCP registry,
  tool calls, approval/user-input, and MCP audit events.
- AutoResearch project state -> `/goal --research`,
  `.analytix/autoresearch`, and `autoresearch_state_audit`.
- Workflow/Create Loop planner discipline -> internal chat/goal/tool planner
  events only.

Guardrails:

- No Reasonix public protocol is added.
- No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product
  entry is added.
- `window.analytix`, top-level `runtime` settings, `analytix serve`, product
  identity, and the multi-model provider baseline stay unchanged.
- `goDefaultCutoverCandidate` can be true only when the strict D-0243/D-0248
  evidence readiness is true; it is runtime-info evidence, not a startup gate.

Additional hardening:

- Go readiness now validates credentialed provider, credentialed MCP, and
  packaged QA evidence JSON files instead of accepting `STATUS=passed` alone.
- The Electron main adapter uses the same evidence-file checks before allowing
  `go-runtime-candidate`.
- `scan:product-sovereignty` now requires D-0249 topology tokens and still
  rejects default Go/product-surface drift.

Current strict preflight remains blocked by missing real external evidence:
`provider-matrix-credentialed`, `mcp-matrix-credentialed`, `packaged-qa`, and
`explicit-env-gate`.

## 2026-06-23 - D-0248 Strict G6 Evidence Gate And Preflight

D-0248 upgrades the D-0247 readiness report into a strict gate while preserving
the TypeScript default backend.

Strict gate:

```text
node scripts/d0247-go-runtime-readiness-report.mjs --gate --json
```

The strict mode exits non-zero when any required hard gate is `missing`,
`skipped`, or `failed`. It can exit zero only when every hard gate is `passed`
and `defaultBackendReady:true`. Dry-run output remains audit-only and cannot be
used as cutover evidence.

Preflight:

```text
npm run runtime:go:preflight -- --json --gate
```

The preflight runs Go runtime tests, runtime conformance, adapter tests,
contracts/goal tests, preload/settings tests, renderer route-surface tests,
product sovereignty scan, typecheck, runtime build, packaged QA scaffold, and
the strict readiness gate. Missing credentialed provider evidence, missing
credentialed MCP evidence, missing packaged QA, missing explicit
`ANALYTIX_GO_RUNTIME_G6_READY=1`, or missing D-0250B/D-0251/D-0252 operator gate JSON keeps
the result blocked. `--expect-blocked` may be used to record a code-stage
preflight when local checks pass but external live evidence is unavailable; it
is not default-backend authorization.

Credentialed evidence inputs:

```text
ANALYTIX_D0243_PROVIDER_MATRIX_STATUS=passed
ANALYTIX_D0243_PROVIDER_MATRIX_STATUS_EVIDENCE=/path/to/provider-matrix.json
ANALYTIX_D0243_MCP_MATRIX_STATUS=passed
ANALYTIX_D0243_MCP_MATRIX_STATUS_EVIDENCE=/path/to/mcp-matrix.json
ANALYTIX_D0243_PACKAGED_QA_STATUS=passed
ANALYTIX_D0243_PACKAGED_QA_STATUS_EVIDENCE=/path/to/packaged-qa.json
ANALYTIX_GO_RUNTIME_G6_READY=1
ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE=/path/to/operator-gate.json
```

Provider evidence must contain passed credentialed probes for DeepSeek,
OpenAI-compatible, Anthropic-compatible, and custom endpoint families. MCP
evidence must contain a passed credentialed execution probe covering connect,
tool discovery/search, tool call, approval/user-input, reconnect, and
redaction, without exposing an MCP-indexer top-level UI. Packaged QA evidence
must be a passed `d0243-packaged-go-runtime-qa` report covering packaged app
startup, health, thread list, turn create, SSE replay, and the historical Go
runtime candidate / TypeScript rollback gate; current G6 evidence adds the
Go-only runtime boundary and TS-retirement scan. The operator gate evidence must be a passed
`d0250b-go-runtime-operator-gate`, `d0251-go-runtime-operator-gate`, or
`d0252-go-default-candidate-operator-gate` JSON file with no secrets, explicit evidence
review, and Go default-candidate approval. Fake/local matrix proof is still
local regression evidence only.

Exit plan:

```text
docs/analytix/upstreams/go-runtime-retirement-checklist.md
```

This checklist records the TypeScript runtime paths that must be retained until
real strict G6 passes, plus the D-0250 deletion order for temporary
conformance/shadow/proof routes after Go becomes the default backend.

## 2026-06-23 - D-0246 AutoResearch Go Runtime Project State

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
`/v1/threads/:id/turns` may create project-local AutoResearch state when the
prompt is `/goal --research`. `/v1/threads/:id/events` replays an
`autoresearch_state_audit` runtime event. No new public route, renderer bridge,
settings schema, UI entry, or `/goal` completion semantics are added.

Go result:

```text
packages/runtime-go/internal/research/autoresearch_state.go
packages/runtime-go/d0246_autoresearch_state_test.go
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
```

Shared contract result:

```text
packages/runtime/src/contracts/events.ts
packages/runtime/tests/contracts.test.ts
packages/runtime/tests/runtime-event-reducer.test.ts
packages/runtime/tests/go-runtime-conformance.test.ts
```

State contract:

```text
.analytix/autoresearch/<threadId>/task_spec.md
.analytix/autoresearch/<threadId>/progress.json
.analytix/autoresearch/<threadId>/findings.jsonl
.analytix/autoresearch/<threadId>/directions_tried.json
.analytix/autoresearch/<threadId>/iteration_log.jsonl
```

Current matrix state:

```text
D-0244: red=0, deferred=0, AutoResearch project-local state green
D-0245: red=0, deferred=0, autoresearch-project-state green
D-0243: default Go backend still not ready unless explicit durable/provider/MCP/packaged QA gates pass
```

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/contracts.test.ts tests/runtime-event-reducer.test.ts tests/goal-tools.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-23 - D-0247 G6 Readiness Semantics And Cutover Gate

D-0247 does not redefine the Go runtime goal. It closes the readiness-language
gap left after D-0246: D-0244/D-0245 matrix green means the Reasonix capability
matrix and Kun/Analytix baseline guard no longer have red/deferred rows; it does
not mean the Go runtime is ready to become the default backend.

Runtime-info contract:

```text
capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix.capabilityMatrixGreen
capabilities.upstreamAbsorption.reasonixAbsorptionMatrixD0245.absorptionMatrixGreen
capabilities.upstreamAbsorption.readinessSemanticsD0247
capabilities.upstreamAbsorption.defaultBackendReadinessD0243
```

`readyForG6` remains present for compatibility, but it is now documented and
tested as a matrix-green alias only. `readinessSemanticsD0247` states that
matrix green does not enable the Go default backend, skipped checks are not
passes, and fake/local provider or MCP matrices do not count as credentialed
passes.

Main adapter gate:

```text
ANALYTIX_RUNTIME_BACKEND=go-runtime-candidate
ANALYTIX_GO_RUNTIME_CANDIDATE=1
ANALYTIX_GO_RUNTIME_G6_READY=1
ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE=/path/to/operator-gate.json
durableRestartProof=passed
credentialedProviderMatrix=passed with complete D-0243 provider env
credentialedMCPMatrix=passed with MCP command/url env
packagedQa=passed
```

Missing any item keeps or rolls back to the TypeScript runtime. D-0246 cleared
the AutoResearch blocker, but G6/default backend readiness remains incomplete
until the credentialed provider/MCP matrices, packaged QA, durable restart
drill, product sovereignty scan, typecheck, and runtime build all have current
evidence. `scripts/d0247-go-runtime-readiness-report.mjs` consolidates that
evidence and reports skipped dry-run checks as skipped, not passed.

Validation focus:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
node scripts/d0247-go-runtime-readiness-report.mjs --dry-run --json
```

## 2026-06-23 - D-0244 Reasonix Capability Red Matrix

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Source baseline:

```text
/Users/sun/Projects/DeepSeek-Reasonix
7032f39336f4ae5f216e1fcb3368e5f679723490
```

Contracts touched:
`/v1/runtime/info` now accepts an analytix-owned optional
`capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix` metadata object.
It does not expose Reasonix SessionAPI/config roots, Reasonix CLI identity,
MCP-indexer public routes, public sub-agent/job protocol, a renderer-visible
Go route, or a backend switcher.

Go result:

```text
packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go
packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go
packages/runtime-go/d0244_reasonix_red_matrix_test.go
packages/runtime-go/internal/goal/goal_evidence.go
packages/runtime-go/d0244_goal_evidence_test.go
packages/runtime-go/internal/mcp/lifecycle.go
packages/runtime-go/d0244_mcp_lifecycle_test.go
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/runtime_server.go
```

Shared contract result:

```text
packages/runtime/src/contracts/capabilities.ts
packages/runtime/src/contracts/events.ts
packages/runtime/tests/go-runtime-conformance.test.ts
```

Current matrix state:

```text
green: provider/cache streaming, DeepSeek prefix cache, approval/user-input gates, MCP lifecycle/search/call/reconnect/redaction, job/sub-agent lineage, durable replay, crash/restart recovery, candidate rollback, Goal evidence FSM, AutoResearch project-local state
red: none
deferred: none
rejected: Reasonix public protocol/config/identity surfaces
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts -t "runs the D-0242 Go runtime server"
```

## 2026-06-23 - D-0243 G6 Readiness Hardening Slice

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, settings schema, UI route, public runtime route, or
default backend changed. `/v1/runtime/info` remains the existing public schema;
D-0243 readiness is evaluated by the main adapter/canary and test scaffolds.

Go result:

```text
packages/runtime-go/g6_readiness.go
packages/runtime-go/g6_readiness_test.go
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
packages/runtime-go/cmd/runtime-server/main.go
```

D-0243 adds:

```text
provider matrix scaffold for DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom endpoint modes
env-gated credentialed provider probes that skip without key/baseUrl/model
DeepSeek cache hit/prefix-stability benchmark record format
fake MCP matrix for connect/search/call/reconnect/approval/redaction
env-gated credentialed MCP scaffold that skips without configuration
runtime durable root mode through --runtime-durable-root
restart recovery for events.jsonl, highestSeq, pending approval/user-input gates, and turn sequence
```

Main adapter gate:
`src/main/runtime/analytix-adapter.ts` now resolves D-0243 G6 readiness from
internal evidence env vars. The default status is `ready:false`. The
`go-runtime-candidate` canary still probes real contract endpoints, but now it
also requires:

```text
ANALYTIX_GO_RUNTIME_G6_READY=1
ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE=/path/to/operator-gate.json
ANALYTIX_D0243_DURABLE_RESTART_PROOF=passed
ANALYTIX_D0243_PROVIDER_MATRIX_STATUS=passed
ANALYTIX_D0243_MCP_MATRIX_STATUS=passed
ANALYTIX_D0243_PACKAGED_QA_STATUS=passed
```

If any value is missing, skipped, or failed, Go startup rolls back to the
TypeScript runtime.

Packaged QA:
`scripts/d0243-packaged-go-runtime-qa.mjs` is an executable checklist for
packaged app startup, Go candidate gate, health, thread list, turn create, SSE
replay, and rollback. Missing app/runtime state is reported as `skipped`, not
pass.

Temporary path deletion:
D-0241 provider-live, durable-replay, approval-user-input, MCP manager, and
job-lineage proof routes remain post-D-0243/G6 delete candidates because the
runtime contract plus restart drill now cover their durable replay/gate/MCP/job
evidence. D-0242 temp durable startup remains for conformance tests; the
candidate durable root is the migration path.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
npm run qa:d0243:packaged-go-runtime -- --dry-run --json
```

## 2026-06-23 - D-0242 Go Runtime Server Contract Slice

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, settings schema, UI route, public product entry, or
default backend changed. D-0242 adds a separate internal binary:

```text
packages/runtime-go/cmd/runtime-server
```

It serves an analytix runtime contract subset rather than the D-0241 internal
proof route family:

```text
GET /health
GET /v1/runtime/info
GET /v1/threads
GET /v1/threads/:id
PATCH /v1/threads/:id
POST /v1/threads/:id/fork
POST /v1/sessions/:id/resume-thread
POST /v1/sessions/:id/resume
GET /v1/threads/:id/events
POST /v1/threads/:id/turns
POST /v1/approvals/:id
POST /v1/user-inputs/:id
```

Go result:
`packages/runtime-go/internal/server/runtime_server.go` connects D-0241
components into the contract server, with `packages/runtime-go/runtime_server.go`
kept as the root public entry shim:

```text
GoHTTPProviderClient over local fake live HTTP/SSE provider
temp durable event/session sink as thread, turn, and SSE replay storage
approval/user-input manager behind runtime approval/input routes
fake MCP manager result emitted through turn replay
parent goal/thread-bound job lineage emitted as runtime pipeline metadata
usage/cache accounting with DeepSeek cache hit/miss and stable prefix diagnostics
```

The turn route persists `turn_started`, user item creation, approval and
user-input gate requests, MCP result, assistant deltas, usage/cache accounting,
job lineage, and `turn_completed`. Approval denial and user-input submission
can be resolved through contract routes; replay preserves status while keeping
submitted answers out of events.

Main adapter gate:
The runtime candidate backend is accepted only when both internal env gates are
present:

```text
ANALYTIX_RUNTIME_BACKEND=go-runtime-candidate
ANALYTIX_GO_RUNTIME_CANDIDATE=1
```

`src/main/runtime/analytix-adapter.ts` starts `cmd/runtime-server`, requires
real contract canary checks, verifies `/v1/runtime/go` and
`/v1/internal/go-production-candidate/boundary` remain hidden on the contract
server, and rolls back by stopping Go, deleting temp durable state, recording
fallback status, and starting TypeScript.

D-0241 cleanup:
The D-0241 proof route family remains available only as an internal regression
reference. The provider-live, durable-replay, approval-user-input, MCP manager,
and job-lineage proof slices are now covered by D-0242 runtime contract replay
and are marked as post-D-0242/G6 delete candidates.

Reasonix absorption:
D-0242 absorbs Reasonix provider/cache, gate, MCP lifecycle, and job-lineage
discipline through analytix contract code. It does not import Reasonix
SessionAPI, config roots, public routes, or product identity.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
npm run scan:product-sovereignty
```

## 2026-06-23 - D-0241 Production-Candidate Go Runtime Parity Slice

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, settings schema, UI route, public runtime route, or
default backend changed. D-0241 adds an internal route family:

```text
GET /v1/internal/go-production-candidate/boundary
GET /v1/internal/go-production-candidate/provider-live
GET /v1/internal/go-production-candidate/durable-replay
GET /v1/internal/go-production-candidate/approval-user-input
GET /v1/internal/go-production-candidate/mcp-manager
GET /v1/internal/go-production-candidate/job-lineage
GET /v1/internal/go-production-candidate/single-baseline-checklist
GET /v1/internal/go-production-candidate/canary
```

Go result:
`packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go` adds a
production-candidate manager slice, with
`packages/runtime-go/live_production_candidate.go` kept as the root
compatibility shim, covering:

```text
GoHTTPProviderClient over local fake live HTTP/SSE servers
DeepSeek/OpenAI-compatible/Anthropic-compatible request shape and stream parsing
provider-family-aware usage/cache parsing with DeepSeek native cache field precedence
stable prefix-shape capture that excludes dynamic user text
durable event/session writes and replay through the D-0236 event sink
approval/user-input pending, deny, submit, cancel, timeout, and replay manager
fake MCP lifecycle/search/call manager with denied no-execute proof
parent goal/thread bound job/sub-agent lineage proof
D0241SingleBaselineChecklist in
packages/runtime-go/internal/upstreamaudit/single_baseline_checklist.go for
delete-after-D0241, delete-after-G6, and forbidden-now paths
```

Main adapter gate:
The production candidate backend is accepted only when both internal env gates
are present:

```text
ANALYTIX_RUNTIME_BACKEND=go-production-candidate
ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE=1
```

`src/main/runtime/analytix-adapter.ts` extends the canary beyond fixture
boundary checks to require live provider fake server, durable replay,
approval/user-input manager, fake MCP manager, job lineage, and
single-baseline checklist success. Failure stops Go, cleans temporary durable
state, records fallback status, and starts the TypeScript runtime.

Reasonix absorption:
D-0241 code-level absorbs the stronger Reasonix provider/cache, approval/input
gate, MCP lifecycle, and job/sub-agent lineage disciplines, but reimplements
them behind analytix contracts. Reasonix public SessionAPI, config roots,
public routes, and renderer-visible subagent protocol remain rejected.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-production-candidate-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
```

## 2026-06-23 - D-0240 G6 Retirement Cleanup Proof

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, settings schema, UI route, public runtime route, or
default backend changed. D-0240 extends the D-0239 kernel oracle and sidecar
with:

```text
GET /v1/conformance/kernel/g6-retirement-cleanup
```

Go result:
The route returns an explicit cleanup matrix:

```text
currentRetainedPaths: TypeScript runtime, current process supervisor, and the single backend-neutral main adapter
deleteAfterG6: transition conformance gate, /v1/conformance/* route family, kernel/minimal-loop fixtures, shadow-only G5 replay
forbiddenRedundancy: deprecated bridge aliases, duplicate settings schemas, renderer-visible Go route, UI runtime switcher, Reasonix SessionAPI
g6Blockers: production Go managers, packaged QA, crash/rollback drills, credentialed provider/MCP matrix, TS retirement migration plan
```

The route proves `g6Ready:false`, `noImmediateProductionDelete:true`,
`tsRuntimeRetained:true`, and `presentForbiddenRedundancyCount:0`.

Rationale:
D-0240 prevents the migration from becoming a permanent dual-path architecture
by making the post-G6 deletion list executable conformance evidence. It also
protects the current release from deleting the TypeScript runtime while it is
still the production default.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-kernel-live-scaffold-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-23 - D-0239 Go Kernel Live Scaffold

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, settings schema, UI route, or public runtime contract
changed. The new Go kernel routes live only under
`/v1/conformance/kernel/*` inside the internal/test/conformance sidecar.

Fixtures used:

```text
packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-oracle.json
packages/runtime/src/conformance/fixtures/go-durable-sidecar-oracle.json
packages/runtime/src/conformance/fixtures/go-minimal-agent-loop-oracle.json
packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json
packages/runtime/src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-oracle.json
packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json
```

Go result:
`packages/runtime-go/internal/server/live_local_kernel.go` adds a
fixture-backed kernel map, with `packages/runtime-go/live_local_kernel.go`
kept as the root compatibility shim. Routes:

```text
GET /v1/conformance/kernel/boundary
GET /v1/conformance/kernel/capabilities
GET /v1/conformance/kernel/absorption-proof
GET /v1/conformance/kernel/job-orchestration
GET /v1/conformance/kernel/snapshot
```

The kernel scaffold verifies, in one live-local sidecar, that durable event
sink, minimal loop controller, provider-aware cache accounting,
approval/user-input gate replay, MCP catalog recovery, and parent-goal-bound
job/sub-agent orchestration fixtures are simultaneously available under
analytix contracts. It records no provider calls, tool execution, approval
execution, MCP connection, credential read, file mutation, real workspace
write, renderer-visible Go route, or default Go backend.

Reasonix absorption:
D-0239 classifies and lands stronger Reasonix kernel ideas behind analytix
contracts:

```text
session-scoped Job Manager -> contract-reimplement fixture
subagent continue/fork lineage guard -> contract-reimplement fixture
loop controller and durable event sink -> contract-reimplement fixture
MCP lazy catalog recovery -> code-port-and-adapt fixture
provider-aware cache accounting -> code-port-and-adapt fixture
Reasonix SessionAPI/config/public routes -> reject
live production Go job/subagent execution -> defer
```

Kun boundary:
The route is conformance-only and not a product entry. No Workflow, Create
Loop, Subagent, AutoResearch, MCP-indexer, runtime switcher, or Go renderer
route is exposed.

G6 deletion list after default-backend proof:

```text
fixture-only /v1/conformance/kernel route family
temporary kernel scaffold oracle once production Go managers replay equivalent evidence
shadow-only job/subagent fixture rows superseded by live permission-gated Go jobs
transition scan exceptions for conformance-only kernel paths
```

Differences:
This is not live job/sub-agent execution, not a production Go Job Manager, not
live MCP/provider parity, not packaged desktop QA, not a default backend, and
not TypeScript runtime retirement.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-kernel-live-scaffold-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-23 - D-0238 Backend-Neutral Main Adapter Gate

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
`src/main/runtime/analytix-adapter.ts` now owns a backend-neutral runtime
selection gate. At D-0238 time, the default remained the TypeScript
`analytix serve` runtime and the Go path could be requested only with both
internal env gates:

```text
ANALYTIX_RUNTIME_BACKEND=go-conformance
ANALYTIX_GO_RUNTIME_CONFORMANCE=1
```

No renderer, preload, settings schema, public bridge, visible runtime switcher,
or top-level navigation route changed. `window.analytix`, top-level `runtime`
settings, and `analytix serve` remain the public contract. Current startup
status is superseded by G6: Go is the default runtime core and TypeScript is
explicit rollback only.

Historical Go result:
the early `packages/runtime-go/cmd/conformance-sidecar` entry accepted
`--runtime-token`, so the main adapter could launch the fixture-backed sidecar
with the current analytix runtime bearer token instead of the fixture default.
That entry has been retired; the current test/conformance-only sidecar is
`packages/runtime-go/cmd/contract-sidecar`, and production launches
`packages/runtime-go/cmd/runtime-server`.

Canary and rollback:
The Go backend is accepted only after all checks pass:

```text
GET /health
GET /v1/threads?limit=1
GET /v1/conformance/loop/boundary
GET /v1/runtime/go -> 404
```

The loop boundary must report fixture-only conformance, minimal-loop prototype,
no default Go backend, no renderer-visible Go routes, no Reasonix public
protocol, and no Electron main connection. If launch or canary fails, the
adapter stops the Go sidecar, cleans the temp durable root, records the
fallback reason, and starts the TypeScript runtime.

Reasonix absorption:
D-0238 moves Reasonix-style controller/event-sink/cache/gate/MCP recovery
discipline one layer closer to the desktop boundary by giving Electron main an
internal backend-neutral gate and canary. It still rejects Reasonix SessionAPI,
config roots, public route names, renderer protocol, product identity, and
default-backend authority.

Kun boundary:
Kun-derived product entry points and desktop UX remain unchanged. No Workflow,
Create Loop, Subagent, AutoResearch, MCP-indexer, or Go runtime switcher entry
is exposed.

G6 deletion list after default-backend proof:

```text
temporary conformance sidecar launcher envs
fixture-only /v1/conformance/* desktop gate path
TypeScript-only process helper branch that duplicates backend start/stop once Go is production-equivalent
any fallback status fields that are only needed for the transition gate
stale Go shadow fixtures superseded by live production Go conformance
```

Differences:
This is not a live production Go backend, not G6, not packaged desktop route
QA, not live provider/MCP superiority, and not permission to retire the
TypeScript runtime.

Validation:

```text
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
npm run typecheck
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0237 Go Minimal Agent Loop Skeleton

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or production TypeScript
runtime contract changed. The loop proof is enabled only when the Go
conformance sidecar is launched with a temp durable root and the TS-owned
`go-minimal-agent-loop-oracle.json` fixture. `analytix serve` remains the
public runtime CLI.

Fixtures used:

```text
packages/runtime/src/conformance/fixtures/go-minimal-agent-loop-oracle.json
packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json
packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json
packages/runtime/src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-oracle.json
packages/runtime/src/conformance/fixtures/approval-user-input-route-oracle.json
packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-oracle.json
packages/runtime/src/conformance/fixtures/provider-cache-oracle.json
```

TypeScript oracle:
The TS runtime remains authoritative for model request shape, stable system
prefix/cache diagnostics, tool catalog fingerprinting, runtime event ordering,
approval/user-input route contracts, MCP catalog behavior, and durable replay
semantics. D-0237 adds a TS-owned minimal loop oracle covering a single user
turn, assistant reasoning/text, tool call readiness, denied approval, tool
result, submitted and cancelled user-input gates, DeepSeek/OpenAI/Anthropic
cache telemetry, MCP catalog visibility without connection, step-limit/cancel/
resume boundaries, and recovered durable state equality.

Go result:
`packages/runtime-go/internal/server/live_local_loop.go` adds conformance-only
routes below `/v1/conformance/loop/*`;
`packages/runtime-go/live_local_loop.go` remains a root shim for existing
conformance call sites:

```text
GET  /v1/conformance/loop/boundary
POST /v1/conformance/loop/run
POST /v1/conformance/loop/cancel
POST /v1/conformance/loop/resume
GET  /v1/conformance/loop/replay
GET  /v1/conformance/loop/recovered-state
GET  /v1/conformance/loop/threads/:id/events
```

The historical loop skeleton was fixture-scripted. It never read API keys,
called a provider, executed tools, opened MCP connections, mutated a real
workspace, or connected to Electron. Current production loop evidence is
covered by the formal runtime server, product-regression, and speed/cache gates;
`cmd/contract-sidecar` remains test/conformance-only.

Reasonix absorption:
D-0237 supports Reasonix agent-kernel loop-controller, event-sink, cache
accounting, approval/user-input gate replay, and MCP catalog recovery
discipline behind analytix-owned contracts. It does not adopt Reasonix public
protocol, SessionAPI, config root, UI control plane, live provider behavior, or
live MCP parity.

Boundary clarification:
The generic live-local sidecar boundary still reports
`tempDurableStorePrototype:false` because G2/G3/G4 replay can run without the
temp durable store. Durable and loop routes now return durable/loop-specific
nested product boundaries where `tempDurableStorePrototype:true`; loop routes
also report `minimalAgentLoopPrototype:true` and `fixtureBackedLoopOnly:true`.

Differences:
This is not a production Go backend, live Go agent loop, live provider client,
live approval/user-input/MCP manager, packaged restart drill, default backend,
G6, or release-readiness evidence.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-minimal-agent-loop-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-23 - D-0236 Go Temp Durable Event/Session Store + SSE Replay

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or production TypeScript
runtime contract changed. The Go durable store is opt-in through
`LiveLocalSidecarConfig.DurableTempDir` and starts only from Go/TS conformance
tests.

Fixtures used:

```text
packages/runtime/src/conformance/fixtures/go-durable-sidecar-oracle.json
packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json
```

TypeScript oracle:
The TypeScript `RuntimeEventRecorder`, `FileSessionStore`, `FileThreadStore`,
SSE route, thread/session service, approval/user-input route, usage/cache
event schema, and G2 route fixture remain authoritative. D-0236 adds a
TS-owned durable oracle with DeepSeek, OpenAI, and Anthropic cache telemetry,
approval denial, user-input submission without persisted answers, and MCP tool
catalog replay.

Go result:
`packages/runtime-go/internal/server/durable_store.go` and
`packages/runtime-go/internal/server/live_local_durable.go` add an isolated
temp-dir durable store and local conformance routes below
`/v1/conformance/durable/*`; `packages/runtime-go/live_local_durable.go`
remains a root shim for existing conformance call sites:

```text
GET  /v1/conformance/durable/boundary
GET  /v1/conformance/durable/snapshot
GET  /v1/conformance/durable/threads
GET  /v1/conformance/durable/threads/:id
PATCH /v1/conformance/durable/threads/:id
POST /v1/conformance/durable/threads/:id/fork
POST /v1/conformance/durable/sessions/:id/resume
POST /v1/conformance/durable/event
POST /v1/conformance/durable/events/malformed
GET  /v1/conformance/durable/events/replay
GET  /v1/conformance/durable/threads/:id/events
GET  /v1/conformance/durable/threads/:id/highest-seq
GET  /v1/conformance/durable/recovered-state
```

The store writes only under a caller-provided temp dir, appends
newline-terminated `events.jsonl`, assigns stable per-thread `seq`, returns
`highestSeq()` from the persisted max, filters and sorts `loadEventsSince`,
skips malformed JSONL with line/path/error/preview diagnostics, proves
persist-before-publish order, and supports caught-up SSE replay plus
`Last-Event-ID` / `since_seq` equivalence.

Sidecar entry:
the historical `packages/runtime-go/cmd/conformance-sidecar` entry has been
retired. Current conformance tests use the formal contract-sidecar naming, while
production startup and packaged builds use `packages/runtime-go/cmd/runtime-server`.

Reasonix absorption:
D-0236 supports Reasonix agent-kernel durable event/session recovery discipline
without adopting Reasonix public protocol, SessionAPI, config root, or UI
surface. It proves cache hit/miss telemetry survives durable replay for
DeepSeek, OpenAI Responses, and Anthropic Messages; approval and user-input gate
events survive replay without executing tools or persisting submitted answers;
and MCP manager/catalog state can be reconstructed without connecting to real
MCP servers or reading credentials.

Isolation proof:
The durable store is disabled by default. It rejects non-temp durable roots,
does not read provider/API/MCP credentials, does not call external network,
does not touch a real workspace, does not connect Electron, and does not expose
renderer-visible Go routes. Existing G4 manager zero-attempt boundaries remain
unchanged.

Differences:
This is a temp durable runtime base, not production runtime replacement, live Go
agent loop, live provider client, live MCP client, packaged restart QA, default
backend, G6, or release-readiness evidence.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-durable-sidecar-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-23 - D-0235 G4 Approval/User-Input/MCP Manager Live-Local Prototype

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or TypeScript runtime HTTP
contract changed. The new Go behavior remains confined to `packages/runtime-go`
and starts only from Go tests.

Fixtures used:

```text
packages/runtime/src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-oracle.json
packages/runtime/src/conformance/fixtures/approval-user-input-route-oracle.json
packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-oracle.json
```

TypeScript oracle:
The TypeScript approval/user-input and MCP lifecycle fixtures remain
authoritative for status/body shapes, pending gate counts, replay kinds,
submitted-answer privacy, remote-entry control-port boundaries, MCP lifecycle
tool lists, approval annotations, search meta-tools, reconnect classification,
known override diagnostics, and secret redaction.

Go result:
`packages/runtime-go/internal/mcp/live_local_g4.go` adds a fixture-backed G4
manager handler under the test/conformance-only sidecar;
`packages/runtime-go/live_local_g4.go` remains a root shim for existing
conformance call sites. It exposes only local conformance routes below
`/v1/conformance/g4/manager/*` for:

```text
GET  /v1/conformance/g4/manager/boundary
GET  /v1/conformance/g4/manager/tool-catalog
POST /v1/conformance/g4/manager/approval/decision
GET  /v1/conformance/g4/manager/approval/replay
POST /v1/conformance/g4/manager/user-input/cancel
POST /v1/conformance/g4/manager/user-input/submit
POST /v1/conformance/g4/manager/user-input/validate
GET  /v1/conformance/g4/manager/user-input/replay
GET  /v1/conformance/g4/manager/remote-entry
GET  /v1/conformance/g4/manager/mcp/lifecycle
GET  /v1/conformance/g4/manager/mcp/approval-annotations
GET  /v1/conformance/g4/manager/mcp/search
GET  /v1/conformance/g4/manager/mcp/call-reconnect
GET  /v1/conformance/g4/manager/mcp/background-reconnect
GET  /v1/conformance/g4/manager/mcp/diagnostics
```

Isolation proof:
`TestLiveLocalSidecarG4ApprovalUserInputReplayMatchesTypeScriptOracle` and
`TestLiveLocalSidecarG4MCPReplayMatchesTypeScriptOracle` start a real local
`httptest.Server`, exercise the fixture-backed G4 conformance routes, and
assert denied approvals do not execute, user-input cancel/submit bodies match
the oracle, invalid structured choice requests do not open gates, resolved
events omit answers, remote entry keeps only approved control ports, MCP search
hides untrusted workspace tools, reconnect behavior is fixture-classified, and
diagnostics redact secrets.

Snapshot proof:
`LiveLocalSidecarSnapshot` now records isolated G4 replay counters while
`ApprovalExecutionAttempts`, `ToolExecutionAttempts`, `MCPConnectionAttempts`,
`MCPCredentialAttempts`, `CredentialReadAttempts`, `FileMutationAttempts`,
`EventsJSONLWriteAttempts`, and `RealWorkspaceWriteAttempts` remain `0`.

Differences:
This is an isolated G4 manager prototype for local fixture replay, not live Go
approval execution, live Go user-input backend parity, live Go MCP client
parity, credentialed MCP matrix, Electron connection, renderer-visible Go
route, default backend, G6, or release-readiness evidence.

Decision:
Record D-0235 as the movement from G3 provider/cache prototype to G4 isolated
approval/user-input/MCP manager prototype evidence. Reasonix gate/manager
discipline is absorbed as analytix-owned fixture replay; Reasonix public
control plane, SessionAPI, MCP-indexer product entry, and renderer-visible Go
routes remain rejected.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0234 G3 Provider/Cache Streaming Live-Local Prototype

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or TypeScript runtime HTTP
contract changed. The new Go behavior remains confined to `packages/runtime-go`
and starts only from Go tests.

Fixtures used:

```text
packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json
packages/runtime/src/conformance/fixtures/provider-cache-oracle.json
```

TypeScript oracle:
The TypeScript provider/cache fixtures remain authoritative for request URL,
header, body, tool shape, provider usage parsing, cache accounting, stable
prefix, drift attribution, and streaming order. The Go harness consumes the
G3 oracle instead of reading provider credentials or calling external model
services.

Go result:
`packages/runtime-go/internal/provider/live_local_provider.go` adds a
fixture-backed G3 provider handler under the test/conformance-only sidecar;
`packages/runtime-go/live_local_provider.go` remains a root shim for existing
conformance call sites. It exposes only local conformance routes for:

```text
GET  /v1/conformance/g3/provider/boundary
POST /v1/conformance/g3/provider/usage
POST /v1/conformance/g3/provider/request-shape
GET  /v1/conformance/g3/provider/stream
GET  /v1/conformance/g3/provider/cache-accounting
GET  /v1/conformance/g3/provider/cache-drift
GET  /v1/conformance/g3/provider/cache-diagnostics
```

The harness replays the 5 provider usage cases, 7 request-shape cases, SSE
`item_delta -> usage -> turn_completed` order, DeepSeek native cache precedence,
OpenAI Responses `cached_tokens`, Anthropic cache read/create fields,
unsupported-cache unknown behavior, aggregate hit/miss accounting, and cache
drift attribution from the TS-owned oracle.

Isolation proof:
`TestLiveLocalSidecarG3ProviderUsageAndCacheReplayMatchesTypeScriptOracle`,
`TestLiveLocalSidecarG3ProviderRequestShapeReplayMatchesTypeScriptOracle`, and
`TestLiveLocalSidecarG3ProviderStreamingReplayMatchesTypeScriptOracle` start a
real local `httptest.Server`, exercise the fixture-backed G3 conformance
routes, and assert no external network, API-key read, provider credential, or
real provider call occurs. `LiveLocalSidecarSnapshot` records only local stub
replay counters while `ProviderCallAttempts` remains `0`.

Stable-prefix proof:
The cache diagnostics route returns bounded hashes and provider/model/endpoint
attribution while keeping forbidden prompt, tool text, API-key, and
Authorization substrings out of diagnostics. Dynamic runtime state remains out
of the stable prefix.

Differences:
This is a provider/cache live-local prototype for local fixture replay, not a
live Go provider client. There is no external model call, credentialed provider
matrix, live provider/cache superiority claim, Electron connection,
renderer-visible Go route, default backend, G6, or release-readiness claim.

Decision:
Record D-0234 as the movement from G2 lifecycle prototype to G3 provider/cache
streaming prototype evidence. Reasonix provider/cache lessons are absorbed as
analytix-owned request-shape, usage/cache, stable-prefix, and drift contracts;
Reasonix public protocol, SessionAPI, config roots, provider protocol, and
route names remain rejected.

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Rollback:
Remove `packages/runtime-go/internal/provider/live_local_provider.go`,
`packages/runtime-go/live_local_provider.go`, the `ProviderOracle` harness
wiring, the G3 live-local snapshot counters/tests, and the focused TS source
guard. No Electron, renderer, settings, TypeScript runtime, or
`analytix serve` code would need rollback.

Remaining risks:
Real Go provider clients, durable event/session stores, live approval/user-input
execution, MCP credentials, file mutation, packaged desktop QA, backend-neutral
adapter selection, and G6 backend defaulting remain future gates.

## 2026-06-23 - D-0233 Isolated Mutating G2 Live-Local Lifecycle Prototype

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or TypeScript runtime HTTP
contract changed. The new Go code remains confined to `packages/runtime-go`.

Fixtures added:
No new oracle fixture. The prototype consumes the existing TypeScript-owned G2
route replay oracle:

```text
packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json
```

TypeScript oracle:
G2 remains the authority for thread archive/search/read/update, fork,
resume-thread, and SSE replay status/body/frame shape. The Go sidecar now uses
that oracle for both read-only and mutating route cases.

Go result:
`packages/runtime-go/internal/server/live_local_store.go` owns the isolated
in-memory G2 store for the live-local sidecar harness, with
`packages/runtime-go/live_local_store.go` retained as a root shim.
`packages/runtime-go/internal/server/live_local_harness.go` owns
`NewLiveLocalSidecarHarness`; `packages/runtime-go/live_local.go` is retained
as the root shim. The harness starts only in Go tests, serves the four mutating
G2 lifecycle routes from the oracle, records the mutation sequence in
`LiveLocalSidecarSnapshot`, and keeps the existing G1 health/runtime-info/tools
responses plus G2 route/SSE replay.

Covered mutating routes:

```text
PATCH /v1/threads/thr_g2_beta
PATCH /v1/threads/thr_g2_read
POST /v1/threads/thr_g2_parent/fork
POST /v1/sessions/thr_g2_source/resume-thread
```

Isolation proof:
`TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptOracle` starts
a real `httptest.Server`, checks all 11 G2 route cases against the oracle, and
asserts the mutation snapshot contains only the expected archive/update/fork/
resume state. `TestLiveLocalSidecarMutationsDoNotTouchFilesystem` uses a temp
workspace `events.jsonl` sentinel and source-token guard to prove this sidecar
does not write real workspace files or event logs.

Differences:
This is live-local prototype behavior for an isolated in-memory fixture-backed
G2 lifecycle slice, not a live Go runtime backend. There is no durable Go
session store, provider client, MCP client, approval/user-input gate manager,
file mutation, Electron connection, renderer-visible Go route, or `analytix
serve` replacement.

Decision:
Record D-0233 as an evidence improvement from read-only G1/G2 live-local
sidecar to isolated mutating G2 lifecycle sidecar. It may inform future
backend-neutral route work, but it cannot support a G6, default-backend,
release-readiness, or live-provider/MCP/gate/file parity claim.

Validation:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```

Rollback:
Remove `packages/runtime-go/live_local_store.go` and revert the focused
live-local harness/test changes. No Electron, renderer, settings, TypeScript
runtime, or `analytix serve` code would need rollback.

Remaining risks:
Durable Go event/session stores, live provider calls, approval/user-input
execution, MCP credentials, file mutation, packaged desktop QA, and backend
selection remain future G5/G6 gates.

## 2026-06-23 - D-0232 G1/G2 Live-Local Sidecar Prototype

Branch:
local `codex/reasonix-goal-control-delta-oracle` worktree.

Contracts touched:
No renderer, preload, Electron main, settings, CLI, or TypeScript runtime HTTP
contract changed. The new Go code is confined to `packages/runtime-go`.

Fixtures added:
No new oracle fixture. The prototype consumes existing TypeScript-owned
fixtures:

```text
packages/runtime/src/conformance/fixtures/go-g1-shadow-oracle.json
packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json
packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json
```

TypeScript oracle:
G1 remains the authority for `/health`, `/v1/runtime/info`, and
`/v1/runtime/tools`. G2 remains the authority for thread list/search/read and
SSE replay frames. G5 remains the authority for rollback/default-backend guards.

Go result:
`packages/runtime-go/internal/server/live_local_harness.go` adds
`NewLiveLocalSidecarHandler`, with `packages/runtime-go/live_local.go` kept as
the root shim. The handler starts locally in tests by combining G1 routes with
G2 `GET` route replay. The sidecar filters mutating `PATCH`/`POST` fixture
routes and uses fixture SSE frames only.
`TestLiveLocalSidecarPrototypeMatchesTypeScriptOracle`
starts a real `httptest.Server`, checks status/body/SSE frames against the
oracle, and confirms mutating routes, approval execution, and `/v1/runtime/go`
stay unexposed.

Differences:
This is live-local prototype behavior for a read-only fixture-backed slice, not
a live Go runtime backend. There is no session store, provider client, MCP
client, approval/user-input gate manager, file mutation, Electron connection,
or `analytix serve` replacement.

Decision:
Record D-0232 as the first Go live-local sidecar prototype proof. It may be
used as a backend-neutral adapter harness seed, but it cannot support a G6,
default-backend, release-readiness, or live-provider/MCP parity claim.

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Rollback:
Remove `packages/runtime-go/live_local.go` and the focused Go/TS test additions.
No Electron, renderer, settings, or `analytix serve` code would need rollback.

Remaining risks:
Mutating route replay, live provider calls, approval/user-input execution, MCP
credentials, file mutation, durable Go event stores, packaged desktop QA, and
backend selection remain future G5/G6 gates.

## Fixture Families

| Fixture | TypeScript oracle | Go result | Status |
| --- | --- | --- | --- |
| health/readiness | `packages/runtime/tests/runtime-factory.test.ts`, `packages/runtime/tests/http-server.test.ts`, `packages/runtime/tests/node-http-server.test.ts`. |  | frozen for G0; Go pending |
| config normalization | `packages/runtime/tests/contracts.test.ts`, `packages/runtime/tests/mcp-config.test.ts`, app settings/provider focused tests. |  | frozen for G0; Go pending |
| thread/session list/search/archive/fork | `packages/runtime/tests/thread-service.test.ts`, `packages/runtime/tests/hybrid-store.test.ts`, `packages/runtime/tests/file-session-store.test.ts`, desktop store/runtime tests, and `go-g2-route-replay-oracle.json`. | G2 shadow replays exact route contracts; D-0233 live-local sidecar executes archive/update/fork/resume against isolated in-memory fixture state only. | TS-owned oracle active; production Go router/durable store pending |
| provider request URL/body/header | `packages/runtime/tests/model-client.test.ts`, `packages/runtime/tests/provider-cache-proof.test.ts`, `src/main/provider-connection.test.ts`, provider settings tests, `provider-cache-oracle.json`. | G3/G5 shadow replays request-shape ids, endpoint families, custom full endpoints, and cache accounting. | shadow oracle-ready; live Go provider client pending |
| streaming text/reasoning deltas and SSE replay | `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/runtime-event-recorder.test.ts`, `packages/runtime/tests/runtime-event-reducer.test.ts`, `src/renderer/src/agent/analytix-runtime.test.ts`, mapper/projection tests. |  | frozen for G0; Go pending |
| tool call lifecycle | `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/builtin-tools.test.ts`, `packages/runtime/tests/capability-registry.test.ts`, `packages/runtime/tests/tool-call-repair.test.ts`, `packages/runtime/tests/tool-storm-breaker.test.ts`. |  | frozen for G0; Go pending |
| tool result images/files | `packages/runtime/src/shared/tool-result-image.ts`, `packages/runtime/src/loop/tool-result-image.test.ts`, `packages/runtime/tests/attachment-store.test.ts`, `packages/runtime/tests/builtin-tools.test.ts`, model-client image tests. | G5 shadow computes `toolResultFileImageBoundary` from TS-owned source/test proof, covering inline image kinds, evicted base64, newest-image cap, attachment `localFilePath`, text fallback `FilePath`, and renderer generated-file meta. | shadow oracle-ready; live Go file/image bridge pending |
| approvals/user input | Denied approval oracle in `packages/runtime/tests/builtin-tools.test.ts`; structured/cancelled GUI input oracles remain in `builtin-tools.test.ts`, `user-input-disabled.test.ts`, `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/approval-user-input-route-oracle.test.ts`, and HTTP endpoint tests. | G4/G5 shadow replays deny/cancel/replay/resume/remote-entry gate behavior and answer privacy from `approval-user-input-route-oracle.json`. | shadow oracle-ready; live Go gate manager pending |
| remote-entry control port | `packages/runtime/tests/remote-entry-control-port.test.ts` proves remote/bot-like entries receive only lifecycle info, restricted turn start/steer/interrupt/get, approval decision, and user-input resolution. Type-level and runtime assertions exclude goal, checkpoint, memory, raw session storage, thread service, tool host access, and per-turn provider/approval/sandbox/path-attachment overrides. |  | frozen for G0/G1/G2/G4/G5; Go pending |
| MCP malformed schema normalization | `packages/runtime/tests/mcp-tool-provider.test.ts` proves malformed MCP `inputSchema` is normalized before advertisement/model catalog exposure. | G5 shadow computes `mcpMalformedSchemaBoundary` from TS-owned source/test proof, covering non-object schema defaults, invalid `properties` removal, string-only `required`, output-schema omission, and preserved normalized tool names. | shadow oracle-ready; live Go MCP client pending |
| plan/goal lifecycle | `packages/runtime/tests/create-plan-tool.test.ts`, `packages/runtime/tests/goal-tools.test.ts`, `packages/runtime/tests/goal-repetition-guard.test.ts`, plan-mode cases in `packages/runtime/tests/loop.test.ts`. |  | frozen for G0; Go pending |
| interrupt/resume/fork/archive/search | `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/thread-service.test.ts`, `src/renderer/src/store/chat-store-thread-actions.test.ts`, `src/renderer/src/lib/thread-fork-registry.test.ts`, `go-g2-route-replay-oracle.json`, and `packages/runtime/tests/go-runtime-conformance.test.ts`. | G2/G5 shadow replays exact thread/session route status, body hashes, SSE frames, fork/resume/archive/search, and auth boundaries. | exact route shadow closed; packaged route QA and live Go HTTP server pending |
| sidecar summary version authority | `packages/runtime/tests/hybrid-store.test.ts` |  | frozen for G0; Go pending |
| goal persistence off controller lock | Current TS runtime has no global controller lock; `ThreadService` uses async store/event calls and logs persistence failures. | G5 shadow computes `goalPersistenceOffLock` from TS-owned source proof and warning tests. | shadow oracle-ready; live Go goal persistence pending |
| context compaction | `packages/runtime/tests/loop.test.ts`, `packages/runtime/tests/context-compactor.test.ts`, `packages/runtime/tests/token-economy.test.ts`. |  | frozen for G0; Go pending |
| model-history repair | `packages/runtime/tests/model-history-repair.test.ts`, `packages/runtime/tests/request-history-hygiene.test.ts`, model-client history repair cases. |  | frozen for G0; Go pending |
| usage/cache accounting | `packages/runtime/tests/usage-service.test.ts`, `packages/runtime/tests/cache.test.ts`, `packages/runtime/tests/model-client.test.ts`, `packages/runtime/tests/provider-cache-proof.test.ts`, `packages/runtime/tests/loop.test.ts`, hybrid-store usage indexing tests, `provider-cache-oracle.json`. | G3/G5 shadow replays DeepSeek/OpenAI/Anthropic cache telemetry, custom request-shape-only coverage, stable prefix, drift attribution, privacy guard, and offline parity seal. | shadow oracle-ready; credentialed/live Go provider matrix pending |
| checkpoint metadata and conversation rewind | `packages/runtime/tests/checkpoint-rewind-oracle.test.ts` proves versioned `axcp_` checkpoint metadata, changed-file normalization, workspace escape rejection, append-only `checkpoint_captured` projection, and non-mutating conversation-only rewind planning. |  | frozen for G5; Go pending |
| checkpoint rewind plan and apply | `packages/runtime/tests/checkpoint-rewind-plan.test.ts` and `packages/runtime/tests/checkpoint-rewind-apply.test.ts`. |  | frozen for G5; Go pending |
| schedule/Connect Phone thread reuse | `src/main/schedule-runtime.test.ts`, `src/main/telegram-runtime.test.ts`, `src/main/claw-runtime.test.ts`, renderer Connect Phone/schedule tests. |  | frozen for G0; Go pending |
| events.jsonl replay | `packages/runtime/tests/file-session-store.test.ts`, `packages/runtime/tests/runtime-event-recorder.test.ts`, `packages/runtime/tests/hybrid-store.test.ts`. | G5 shadow computes `eventJsonlReplayBoundary` from TS-owned source/test proof, covering newline-terminated append, filter/sort replay, highestSeq max, persist-before-publish, concurrent seq uniqueness, and usage compaction carryover. | shadow oracle-ready; live Go event store pending |
| malformed event recovery | `packages/runtime/tests/file-session-store.test.ts`, `packages/runtime/tests/hybrid-store.test.ts`, malformed/history repair tests. | G5 shadow computes `malformedJsonlLineSkipped` and compaction-failure append-only behavior from TS-owned tests. | shadow oracle-ready; live Go event recovery pending |

## Comparison Requirements

Compare these fields for every relevant fixture:

```text
HTTP status
response schema
SSE event sequence
durable event log
projected transcript / ThreadRow state
usage summary
error shape
sanitized logs/traces
```

## Backend-Neutral Desktop Gate

Before a Go stage is marked complete:

```text
1. Electron main can select the backend internally.
2. preload and renderer APIs do not change.
3. settings remain top-level runtime.
4. thread projection output remains equivalent.
5. approvals, user input, usage, and replay remain equivalent.
6. rollback to the previous backend is documented.
```

## Stage Record Template

```text
## YYYY-MM-DD - G<stage> <summary>

Branch:
Contracts touched:
Fixtures added:
TypeScript oracle:
Go result:
Differences:
Decision:
Validation:
Rollback:
Remaining risks:
```

## 2026-06-20 - G4/G5 goal persistence lock requirement

Branch:
local `master` worktree.

Contracts touched:
None. This is a future Go runtime conformance requirement, not a renderer or
HTTP/SSE contract change.

Fixtures added:
No Go fixture yet. The TypeScript oracle evidence is architectural: there is no
global controller mutex around `ThreadService.setGoal`, `ThreadService.clearGoal`,
`AgentLoop.transitionGoalStatus`, or `AgentLoop.finishGoalElapsedTimer`; these
paths await the thread store and event recorder directly. Existing
`packages/runtime/tests/thread-service.test.ts` covers sanitized warning behavior
for goal persistence failures.

TypeScript oracle:
Goal persistence may be awaited by the caller, but it must not share a
controller/status/approval mutex because the TS runtime has no such lock.

Go result:
Pending. When Go goal persistence is implemented, it must follow the Reasonix
`341f720` shape: snapshot cheap in-memory goal state under any controller lock,
then perform durable disk writes outside locks shared with status polling,
approvals, user input, or runtime progress reads. Concurrent writes must be
serialized without torn files.

Differences:
The TS runtime does not need a lock-splitting code change. The future Go runtime
does.

Decision:
Record `341f720` as a G4/G5 conformance gate. Do not invent a TypeScript
controller-lock abstraction solely to mirror Reasonix internals.

Validation:
Runtime goal persistence warning tests remain in the required P2.1 validation
set. Future Go validation must add a controller contention/race fixture before
marking G4/G5 complete.

Rollback:
Not applicable until Go implementation begins.

Remaining risks:
Without this gate, a future Go backend could pass basic goal API tests while
still blocking approval/status responsiveness during disk writes.

## 2026-06-20 - G5 checkpoint rewind oracle fixture

Branch:
local `codex/p4a-checkpoint-rewind-safety-oracle` worktree.

Contracts touched:
`packages/runtime/src/contracts/checkpoints.ts` and the runtime
`checkpoint_captured` event. Renderer/preload/main settings and public CLI are
unchanged.

Fixtures added:
`packages/runtime/tests/checkpoint-rewind-oracle.test.ts`.

TypeScript oracle:
The TypeScript runtime can create deterministic, versioned, analytix-owned
checkpoint metadata with `axcp_` ids, normalize changed files with
first-touch-wins behavior, reject path escape outside the workspace, project
`checkpoint_captured` events, and plan a conversation-only rewind by retaining
events before the target turn boundary.

Go result:
Pending. A future Go G5 backend must reproduce the same metadata schema, id
prefix, event kind, path safety, replay projection, and conversation rewind
plan before it can claim checkpoint/rewind parity.

Differences:
P4A does not implement a Go backend, file restore, git refs, or renderer
controls. Combined code+conversation rewind is deferred to P4B/P4C.

Decision:
Record P4A as a G5 oracle fixture. Do not scaffold Go for checkpoint/rewind
until the TypeScript contract and review UI behavior are complete enough to
compare deterministically.

Validation:
`npm --prefix packages/runtime run test -- tests/checkpoint-rewind-oracle.test.ts --no-file-parallelism --maxWorkers=1`.

Rollback:
Remove the checkpoint contract/event/domain files and focused fixture before
any Go work begins if the contract changes.

Remaining risks:
Future Go could pass metadata-only tests while missing crash-safe file restore,
combined rewind, or review UI parity. Those remain P4B/P4C gates.

## 2026-06-20 - G5 auditable rewind restore plan fixture

Branch:
local `codex/p4b-full-rewind-review-ui` worktree.

Contracts touched:
`packages/runtime/src/contracts/checkpoints.ts` adds the `axrp_` rewind plan
schema. `src/shared/analytix-endpoints.ts` and the desktop IPC allow-list add
the read-only plan route. Renderer/preload/main bridge identity remains
`window.analytix`.

Fixtures added:
`packages/runtime/tests/checkpoint-rewind-plan.test.ts`, plus IPC/provider/UI
projection tests in the desktop app.

TypeScript oracle:
The TypeScript runtime can create an auditable checkpoint rewind plan for
`code`, `conversation`, and `combined` scopes without applying files or
rewriting persisted history. The plan is `applyMode: plan_only`,
`destructive: false`, blocks path escape / absolute persisted paths / symlink
risk, summarizes conversation projection counts, and excludes raw prompts,
secret fixture values, and full file content.

Go result:
Pending. A future Go G5 backend must reproduce P4A checkpoint metadata/events
and P4B plan-only semantics before claiming checkpoint/rewind parity. Go must
not expose Reasonix-native route/settings/event shapes to the renderer.

Differences:
P4B does not implement Go, destructive file restore, event-log rewrite, git
refs, snapshot storage, Rust helpers, or desktop destructive-restore QA.

Decision:
Record P4B as a stronger G5 oracle fixture. P4C must add destructive restore
and desktop QA before Go can be asked to match apply behavior.

Validation:
`npm --prefix packages/runtime run test -- tests/checkpoint-rewind-plan.test.ts --no-file-parallelism --maxWorkers=1`.

Rollback:
Remove the `axrp_` plan contract, route/service wiring, and focused tests if a
future P4C design changes the plan schema before Go work begins.

Remaining risks:
Future Go could match plan-only semantics while still lacking crash-safe
destructive apply. That is intentionally out of P4B and remains a P4C/G5 gate.

## 2026-06-20 - G5 confirmed rewind restore apply fixture

Branch:
local `codex/p4c-confirmed-rewind-restore-apply` worktree.

Contracts touched:
`packages/runtime/src/contracts/checkpoints.ts` adds `axra_` apply results,
`axrr_` rescue records, explicit apply confirmation, snapshot evidence, and
apply response schemas. `packages/runtime/src/contracts/events.ts` adds
append-only `checkpoint_rewind_rescue_created` and
`checkpoint_rewind_applied` events. The desktop route/IPC/provider path is
analytix-owned; renderer/preload/main bridge identity remains
`window.analytix`.

Fixtures added:
`packages/runtime/tests/checkpoint-rewind-apply.test.ts`, plus IPC/provider UI
entry coverage in the desktop app.

TypeScript oracle:
The TypeScript runtime can execute a confirmed checkpoint rewind apply only
from a P4B `CheckpointRewindPlan`. It validates the plan against the current
checkpoint/event-derived plan, blocks path escape, absolute paths, symlinks,
staged/untracked git conflicts, manual-review files, missing snapshot/hash
evidence, snapshot content/hash mismatches, and stale/tampered plans. It also
blocks parent-directory symlink escapes and revalidates file/git safety inside
the file mutation queue. Before file mutation it records an analytix-owned
rescue record. It supports created delete, modified restore, deleted restore,
noop, conversation-only audit, combined code+conversation apply, idempotent
repeat calls, and partial-failure audit that preserves already-applied file
results.

Go result:
Pending. A future Go G5 backend must reproduce the P4A metadata/event oracle,
P4B plan-only semantics, and P4C confirmed apply/rescue/audit semantics before
claiming checkpoint/rewind parity. Go must not expose Reasonix-native
route/settings/event shapes or Kun git refs to the renderer.

Differences:
P4C still does not implement a Go backend, Rust helper, Kun refs, direct git
reset/checkout, silent transcript rewrite, or automatic production snapshot
capture. Modified/deleted content restore requires snapshot evidence in the
apply contract; missing evidence is a hard block.

Decision:
Record P4C as the destructive-apply G5 oracle fixture. Move to G0/G5
conformance inventory only as conformance work: desktop smoke exists, but a
live desktop apply fixture and packaged release QA remain separate gates.

Validation:
`npm --prefix packages/runtime run test -- tests/checkpoint-rewind-apply.test.ts --no-file-parallelism --maxWorkers=1`.

Rollback:
Remove the apply route/service wiring, `axra_`/`axrr_` schemas/events, UI
confirmation control, and focused fixture if a later snapshot-store design
changes the apply contract before Go work begins.

Remaining risks:
Future Go could pass the apply oracle while missing renderer confirmation UX or
desktop crash/restart behavior. Those remain desktop QA and release-readiness
gates, not reasons to start a Go backend in P4C.

## 2026-06-20 - G0/G5 conformance inventory runtime baseline

Branch:
local `codex/p4c-confirmed-rewind-restore-apply` at `b3e1674` when the
inventory started; target commit branch is
`codex/g0-g5-conformance-inventory-runtime-baseline`.

Goal:
Close the next conformance planning loop after P4C without starting a Go
backend scaffold. This inventory freezes the TypeScript runtime contract
surface that any future Go runtime must reproduce, classifies Kun/Reasonix
inputs, and records the G5 golden/oracle backlog.

Runtime baseline attribution:

| Baseline red light | Resolution | Reason |
| --- | --- | --- |
| `persists toolKind from the advertised tool metadata` | Updated the test stream to return a final assistant text after the `file_change` tool. | Current loop contract treats an empty post-file-change stop as an unsafe/incomplete final answer and retries/fails after recovery; the toolKind persistence path was otherwise correct. |
| `uses reported prompt tokens as a compaction pressure signal` | Updated the tiny history fixture so the local estimate remains below the soft threshold but reported usage stays inside the prompt-token trust factor. | Current compaction contract intentionally ignores reported `prompt_tokens` above `PROMPT_TOKEN_TRUST_FACTOR` times the estimate because some providers report cumulative cache-read inflation. |
| `plans normal, aggressive, and force compaction levels` | Updated the same prompt-token fixture to cover normal/aggressive/force without crossing the inflation guard. | The expected compaction modes remain valid; the old fixture conflicted with the newer anti-inflation contract. |

TypeScript runtime contract inventory:

| Contract surface | TS authority to preserve | Current oracle evidence | G0/G5 note |
| --- | --- | --- | --- |
| Thread/session | `ThreadService`, `FileThreadStore`, `FileSessionStore`, `HybridThreadStore`, runtime routes. | `thread-service.test.ts`, `file-session-store.test.ts`, `hybrid-store.test.ts`, desktop thread store tests. | Freeze list/search/archive/fork/session summaries before any Go G2 route work. |
| SSE replay / event log | `RuntimeEventRecorder`, `InMemoryEventBus`, `events.jsonl`, event reducer and renderer mapper. | `runtime-event-recorder.test.ts`, `runtime-event-reducer.test.ts`, `loop.test.ts`, renderer mapper/projection tests. | Go must match event order, sequence high-water, replay projection, and malformed-line tolerance. |
| Checkpoint/rewind plan/apply | `CheckpointRewindService`, checkpoint contracts, checkpoint routes, append-only audit events. | `checkpoint-rewind-oracle.test.ts`, `checkpoint-rewind-plan.test.ts`, `checkpoint-rewind-apply.test.ts`. | These are G5 golden fixtures; live desktop apply/crash-restart is still production evidence, not a Go scaffold blocker. |
| Safety audit | Approval denial, sandbox policy, LLM debug redaction, checkpoint rescue/apply audit. | `builtin-tools.test.ts`, `agent-loop-sandbox.test.ts`, `llm-debug-recorder.test.ts`, checkpoint apply tests. | Stricter safety/privacy wins over upstream parity when conflicts arise. |
| Tool calls | Capability registry, LocalToolHost, built-in tools, tool repair, storm guard, tool result images. | `loop.test.ts`, `builtin-tools.test.ts`, `capability-registry.test.ts`, `tool-call-repair.test.ts`, `tool-storm-breaker.test.ts`, image/file result tests. | Go G4/G5 must preserve advertised tool metadata including `toolKind`. |
| Approvals | Runtime approval policy, GUI approval gate, pending approval events. | `builtin-tools.test.ts`, approval cases in `loop.test.ts`, IPC/renderer card tests where focused. | Denied approval must never execute the tool body. |
| User input | GUI input tools, disabled IM turns, user input gate and resume. | `builtin-tools.test.ts`, `user-input-disabled.test.ts`, `loop.test.ts`. | Go must not block remote/IM turns waiting on unavailable GUI input. |
| Plan/goal | `create_plan`, goal tools, goal timer/persistence, plan-mode tool restrictions. | `create-plan-tool.test.ts`, `goal-tools.test.ts`, `goal-repetition-guard.test.ts`, `loop.test.ts`. | Future Go lock design must keep goal writes outside status/approval critical paths. |
| Model-history repair | History hygiene, tool-call/tool-result pair healing, compaction tail safety. | `model-history-repair.test.ts`, `request-history-hygiene.test.ts`, `context-compactor.test.ts`, model-client history cases. | Go cannot claim parity if provider requests contain invalid tool-call history. |
| Cache accounting | Immutable prefix, canonical tool schemas, optional `usage.cacheDiagnostics`, token economy. | `cache.test.ts`, `loop.test.ts`, `model-client.test.ts`, `token-economy.test.ts`. | Cache diagnostics stay optional, backend-neutral, privacy-bounded, and stable against schema ordering noise. |
| Usage | Usage service, event usage, provider usage parsing, hybrid usage indexing. | `usage-service.test.ts`, `model-client.test.ts`, `loop.test.ts`, `hybrid-store.test.ts`. | Go must match prompt/completion/cache hit/miss semantics before cost claims. |
| Provider request/stream parsing | OpenAI chat, Anthropic messages, OpenAI responses compatibility, full custom endpoint mode, stream usage, reasoning fields. | `model-client.test.ts`, provider connection/settings focused tests. | URL construction and JSON body shape must be compared separately. |

Capability matrix:

| Source | Absorb what | Why better for analytix | Proof that it is better | Conflict authority |
| --- | --- | --- | --- | --- |
| Kun 0.2.13 baseline | Preserve product-workflow expectations: Code/Write/SDD/Connect Phone/schedule, generated files, approvals, review, file changes, provider presets, and desktop command reachability. | Prevents the analytix identity/runtime migration from regressing the product lineage users already expect. | P1.1/P1.2/P1.3/P3A/P4A/P4B/P4C regressions, renderer/main focused tests, desktop smoke where recorded. | analytix specs 01-09; D-0001/D-0002; no old identity, bridge, settings, or UI shell. |
| Kun 0.2.14/current `8602476` | Absorb scoped deltas already closed or corrected: provider/runtime routing, Connect Phone Telegram entry, hidden/unexposed Create Loop internals, checkpoint/rewind safety requirement, desktop utility lessons, and Windows installer process-stop behavior from later develop. | Keeps analytix ahead of stable/current Kun drift while preserving the target Kun product position, `window.analytix`, top-level `runtime`, and runtime HTTP/SSE contracts. | Kun sync ledger, absorption ledger, scorecard sections P1.1 through P4C plus P0 product-entry correction, full runtime suite, app typechecks, focused renderer/main tests. | D-0006/D-0008/D-0009 plus the product-position rule in Spec 08; reject Kun refs, `kun serve`, `window.kunGui`, `KUN_*`, old settings, and unapproved top-level Workflow navigation. |
| Reasonix `main-v2` `be67a498` | Absorb engine ideas: cache-shape diagnostics, provider/cache/tool lifecycle lessons, malformed MCP schema guard, denied approval no-execute proof, checkpoint/rewind plan/apply semantics, and future Go lock guidance. | Converts terminal/runtime engine strengths into backend-neutral analytix contracts without leaking Reasonix public protocol. | Reasonix sync ledger, P2.2/P3A/P4A/P4B/P4C tests, `model-client.test.ts`, `mcp-tool-provider.test.ts`, `builtin-tools.test.ts`, checkpoint fixtures. | D-0003/D-0004/D-0007/D-0009; analytix contracts outrank Reasonix CLI/settings/event shapes. |

G5 golden/oracle list:

| Area | Freeze now | Add before Go G5 parity claim | Missing production evidence |
| --- | --- | --- | --- |
| Agent loop tool/use lifecycle | `loop.test.ts`, `builtin-tools.test.ts`, `tool-call-repair.test.ts`, `tool-storm-breaker.test.ts`. | Route-level transcript fixtures that replay the same tool-call SSE sequence across TS and Go. | Live multi-step desktop coding task with approvals and file review. |
| Compaction and model-history repair | `loop.test.ts`, `context-compactor.test.ts`, `model-history-repair.test.ts`, `request-history-hygiene.test.ts`. | Golden JSON fixtures for normal/aggressive/force compaction and repaired provider request history. | Long real-provider sessions with cache and compaction telemetry. |
| Cache/usage accounting | `cache.test.ts`, `usage-service.test.ts`, `model-client.test.ts`, usage cases in `loop.test.ts`. | Cross-backend usage snapshots covering native cache hit/miss, OpenAI responses, Anthropic messages, and DeepSeek compatibility. | Live provider credential matrix and cost reconciliation. |
| Resume/interrupt/fork/archive/search | Existing `loop.test.ts`, `thread-service.test.ts`, file/hybrid store tests, renderer thread action tests. | HTTP route fixtures for resume-thread, interrupt discard/keep, fork lineage, archive/search, and usage replay. | Packaged app restart/resume QA. |
| Approvals and user input | `builtin-tools.test.ts`, `user-input-disabled.test.ts`, `loop.test.ts`. | Cross-backend pending approval and user-input endpoint fixtures including denial/cancel/resume. | Desktop plus Connect Phone concurrent approval/user-input QA. |
| Checkpoint/rewind/apply | `checkpoint-rewind-oracle.test.ts`, `checkpoint-rewind-plan.test.ts`, `checkpoint-rewind-apply.test.ts`. | Crash/restart apply recovery fixture and route-level SSE/audit replay comparison. | Live desktop apply fixture, crash/restart QA, packaged QA, automatic snapshot-capture decision. |
| Provider request/stream parsing | `model-client.test.ts` and provider focused tests. | Provider-golden request/stream snapshots reused by TS and Go runners. | Live provider matrix with sanitized 404/error guidance evidence. |

Decision:
G0 is closed for the inventory baseline only. G1/G2/G5 Go implementation is
still pending, and no Go backend scaffold, Rust helper, or backend switch is
introduced by this batch.

Validation:
`npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1`
passes after the baseline test contract updates. Full closure also requires
runtime typecheck, app typecheck, related renderer/main focused tests,
`git diff --check`, and production identity/protocol scan.

Remaining risks:
Release/G0 product completion still depends on Spec 07 blockers: verified
analytix remote, release metadata and URL, signing/notarization, Windows NSIS,
packaged QA, live rewind apply, crash/restart apply recovery, and live provider
matrix evidence.

## 2026-06-20 - Reasonix goal/control delta oracle addendum

Branch:
local `codex/reasonix-goal-control-delta-oracle` from
`codex/g0-g5-conformance-inventory-runtime-baseline` at `96b43ed`.

Upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `ab24a77f0f68fcb1b2c160361e4c50cd92b02928` |
| Kun | `v0.2.13` tag / peeled | `201a1469ffbd911f6b95d450b78471e51b42acae` / `2ba8decc2f56862e7f677fcf89bbc3d402ec3a23` |
| Kun | `v0.2.14` tag / peeled | `06be05d76223208724c07301fa0f830641a06e6f` / `8f2040349fba47fcd8e8b94f50b131943af839b2` |
| Reasonix | `main-v2` | `bc8249c307b261ef7ad05a0d2d1409c7b83b95ff` |

Reasonix delta classification:

| Commit | Classification | G0/G5 effect |
| --- | --- | --- |
| `dbaea8433380a4351fd1c2362ea9df175cb61e51` | Absorbed as an analytix-owned `GoalControlMachine` helper, not as Reasonix marker or sidecar protocol. | G5 must preserve internal goal-state transition boundaries, blocked-audit guidance, no-tool recovery, and empty file-change final recovery/failure. |
| `bb06f5b4b6c78d58147799130dcd51db0c5e8913` | Classified only; no analytix `internal/inspect` or redundant Reasonix frontend lockfile exists. | No G0 fixture change. |
| `726036bd15e09a208a59d3685b9de0dcb8c9811b` / `bc8249c3` | Recorded as post-request approvalManager drift and deferred. | Future G4 approval-control cleanup must preserve denied-approval no-execute and GUI approval routes. |

New direct oracles:

| Area | TypeScript oracle | Go parity requirement |
| --- | --- | --- |
| Post-file-change final answer | `packages/runtime/tests/loop.test.ts` now verifies a `file_change` tool followed by empty final text gets one `Tool continuation recovery:` request and then fails with `empty_post_tool_continuation` item/event/`turn_failed`. | Go loop must produce the same retry count, error code, severity, item/event sequence, and final turn status. |
| Prompt-token anti-inflation | `packages/runtime/tests/loop.test.ts` now verifies `promptTokens` above `PROMPT_TOKEN_TRUST_FACTOR` are ignored so inflated cache-read counts do not trigger compaction. | Go compaction pressure must use the same trust-factor guard and fall back to the local estimate. |
| Goal control state | `packages/runtime/src/shared/goal-control-machine.ts` centralizes goal instructions, blocked audit guidance, no-tool recovery, empty post-tool recovery, and non-progress goal tool classification. | Go controller/loop may use locks internally, but public analytix `ThreadGoal`, `goal_updated`, HTTP/SSE, and renderer contracts must remain identical. |

Provider/settings and bridge boundaries:

```text
Provider behavior remains the shared app/provider/runtime contract: top-level
`runtime` settings select provider/model/baseUrl/endpointFormat, provider
profiles remain under `provider`, and URL construction, headers, request body,
stream parsing, usage, and reasoning fields must be tested separately.

Bridge canonical domains remain: renderer -> window.analytix -> preload -> main
-> analytix runtime HTTP/SSE. Go work must not add `window.kunGui`,
`agents.kun`, Reasonix-native renderer protocols, `kun serve`, or `KUN_*`.
The public runtime CLI remains `analytix serve`.
```

Status:
This addendum keeps G0 closed for inventory plus oracle hardening. It does not
start G1/G2/G5 implementation, approvalManager rewiring, Rust adoption, or
release readiness.

## 2026-06-20 - Reasonix SessionAPI Control-Port G0 Addendum

Upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Reasonix | requested `main-v2` baseline | `48e5b990671ca1e579b895080e74babc1e17c333` |
| Reasonix | current `main-v2` recheck | `c202f97035cd353c4bf3bbd2dc6e94ed22710e67` |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | requested `develop` checkpoint | `9605e20f422c90054d930e4f1a0000886b353895` |
| Kun | current `develop` recheck | `247076f297170c3d0c558baffb073c629894faea` |

Reasonix delta:

| Commit | Classification | G0/G5 effect |
| --- | --- | --- |
| `3e625b91d9a7ee265d5e7fb88ed8f2fdae731d35` / `48e5b990` | Scoped-absorbed as a TypeScript runtime control-port/oracle batch before Go G1/G2. | G0 records a lifecycle/turn/approval/user-input-only remote-entry boundary. G5 must preserve that remote/bot-like entry points cannot access goal/checkpoint/memory/storage surfaces unless a future analytix contract explicitly grants them. |

Implemented TypeScript oracle:

| Boundary | TypeScript oracle | Go parity requirement |
| --- | --- | --- |
| Lifecycle | `RemoteEntryControlPort.lifecycle.info()` exposes analytix runtime info only. No public HTTP/SSE or bridge change. | Same backend-neutral runtime info; no Reasonix public protocol. |
| Turn control | `RemoteEntryControlPort.turns` can `startTurn`, `steerTurn`, `interruptTurn`, and `getTurn` for an existing analytix thread; remote starts are limited to prompt/display text, registered attachment ids, and user-input-disable intent. | Same turn event/status semantics and no renderer-visible backend difference. |
| Approvals/user input | `RemoteEntryControlPort.approvals` can deny/allow existing approvals and resolve existing user-input prompts through current gates/events. | Denied approval no-execute, pending prompt replay, user-input cancel/submit, and Connect Phone/schedule behavior remain equivalent. |
| Excluded surfaces | `packages/runtime/tests/remote-entry-control-port.test.ts` uses `@ts-expect-error` and runtime key checks to prove no `goal`, `checkpointRewindService`, `memoryStore`, `sessionStore`, `threadService`, `threadStore`, or `toolHost` access. | Negative fixtures must stay green for any Go remote-entry adapter. |

Status:
This addendum is now implemented for the TypeScript runtime oracle. It does not
start Go code, Rust helper work, a backend switch, or a Reasonix public
protocol. G1/G2 may start only as conformance work that preserves this boundary
alongside the existing route/SSE fixtures.

Validation:
`npm --prefix packages/runtime run test -- tests/remote-entry-control-port.test.ts --no-file-parallelism --maxWorkers=1`
and `npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1`.

## 2026-06-20 - P0 product-entry and installer G0 addendum

Upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | requested `main-v2` | `c202f97035cd353c4bf3bbd2dc6e94ed22710e67` |
| Reasonix | historical P0-time observed `main-v2` | `5d1ad2ae8cb7a0dbc8775fb3f05e6c204627c2b1` |
| Reasonix | current 2026-06-21 recheck `main-v2` | `49c14762b7da9234525e717830e39a64a2220911` |

Oracle update:

| Area | TypeScript/current oracle | Future Go/desktop parity requirement |
| --- | --- | --- |
| Product entry position | `workflow` is absent from `AppRoute`, `openWorkflow`, Sidebar, and Workbench main route. Sidebar, Workbench source-surface, and chat-store action tests guard this. | A Go backend must not force renderer navigation changes. Any future Workflow exposure needs spec-approved UI placement independent of backend work. |
| Create Loop internals | `src/renderer/src/workflow/create-loop-runtime.ts` remains an isolated renderer-side future capability with focused tests. | Go G5 may later provide workflow runtime support only behind an approved analytix contract; no top-level route is implied. |
| Windows installer process-stop | `build/installer.nsh` stops old bundled processes under `$INSTDIR` before upgrade and is included through `electron-builder.config.cjs`; names are `ANALYTIX_INSTALLER_*`. | Release/G6 cannot claim packaged parity until Windows NSIS machine upgrade QA proves the include works. |
| Reasonix historical/current drift | Historical `5d1ad2a` App-to-SessionAPI port and current `49c14762` drift are classified record-only for this P0/cache proof stage. | Go G1/G2 must start from conformance fixtures, not from copying Reasonix desktop SessionAPI surfaces. |

Status:
This addendum keeps Go at G1 pending. It strengthens G0/G5 product-surface and
packaging evidence but does not scaffold Go, switch backends, add Rust/Tauri,
or absorb Reasonix approvalManager.

## 2026-06-21 - Cache/Provider Proof Before Go G1

Upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Reasonix | `main-v2` | `49c14762b7da9234525e717830e39a64a2220911` |

Gate update:

| Gate | Requirement |
| --- | --- |
| Product lineage | Kun `0.2.13` -> `0.2.14` product entry parity remains the product baseline. Go work must not reintroduce the removed top-level Workflow entry. |
| Reasonix cache proof | Existing TypeScript `CachePrefixShape` / `CacheDiagnostics` tests plus `packages/runtime/tests/provider-cache-proof.test.ts` are the starting oracle. They verify stable prefix hash, canonical tool schema hash, provider/model/endpoint attribution, cache hit/miss handling, and unsupported-provider behavior before Go cache/provider work can claim parity. |
| Provider matrix | Go G1/G2 cannot alter OpenAI, Anthropic, OpenAI-compatible, custom endpoint, DeepSeek, write-inline, scheduled detector, model-list, or provider-probe request semantics. |
| Rust | Rust remains helper-only after profiling evidence. No Rust runtime rewrite, Tauri migration, or renderer contract change is part of G1. |

Status:
At this checkpoint Go G1 was still pending until the dirty P0/installer/cache
batch closed. This status is superseded by the later D-0014 Batch 5 G1 shadow
scaffold entry below. Fixture-backed Reasonix cache/provider proof exists for
the TypeScript oracle, while live provider credentials remain an explicit
blocker for final cache superiority claims.

## 2026-06-21 - Reasonix Agent-Kernel Fixture Families

This addendum records future Go fixture families implied by the Reasonix
agent-kernel route. It does not start Go code and does not authorize a default
Go backend.

| Fixture family | TypeScript oracle first | Go stage gate |
| --- | --- | --- |
| permission gate | `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan approval vs tool approval, denied no-execute, sandbox mode, headless/subagent approval rules. | G4/G5 cannot pass without identical safety decisions and event/audit output. |
| evidence ledger | `complete_step`, evidence ledger records, requirement status, proof links, missing evidence reasons. | G5 must preserve append-only evidence and replayable task completion state. |
| complete_step | Runtime/state fixtures prove completion cannot be marked without evidence or required approvals. | G5 loop parity requires the same completion state and failure reasons. |
| AutoResearch state | `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, requirement-by-requirement evidence audit. | G5 may implement long-running tasks only behind analytix-owned project-local state. |
| task/parallel_tasks | `task`, `parallel_tasks`, subagent transcript continuation/fork, nested event rendering, and permission propagation. | G5/G6 require deterministic transcript projection and safe cancellation/retry. |
| background jobs | Job lifecycle, restart recovery, cancellation, status replay, and approval/user-input handoff. | G5 must match TS job event and status fixtures before release QA. |
| token economy | token economy mode, stable system prefix, canonical tool schema hash, cache diagnostics, usage accounting. | G3/G5 must match provider/cache/usage snapshots before cost claims. |
| dynamic tool source | `connect_tool_source`, dynamic tools, stable tool schema, MCP/plugin lifecycle, malformed schema fallback. | G4 must match tool registry and schema hash behavior. |
| MCP lifecycle | MCP Client connect/disconnect/reload/error/cancel behavior with approvals and sanitized traces. | G4 cannot pass while leaking secrets or changing tool-call transcript shape. |
| sandbox | File/shell/protected path/approval restrictions and process cancellation. | G4/G5 must preserve stricter analytix behavior even if Reasonix allows more. |
| planner/executor | planner/executor Coordinator, model pairing, approval boundary, streaming reconciliation, and evidence handoff. | G5 requires task-success and cost benchmarks before claiming improvement. |
| Goal Runtime | Goal Runtime state machine, blocked-state detection, no-tool recovery, progress/non-progress classification, and goal_updated events. | G5 must match current `ThreadGoal`/HTTP/SSE/renderer projections. |

Go kernel components that may eventually implement these fixtures are Provider
Registry, Tool Registry, Controller, Session, Event Sink, Job Manager,
Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime. They
must remain backend-neutral implementation details. Renderer/preload/main
bridge shape, `analytix serve`, top-level `runtime`, and public event semantics
do not change.

G1-G6 are non-skippable:

```text
G1 cannot include default backend selection.
G2 cannot change renderer-visible protocol.
G3 cannot weaken provider request/body/header/stream/usage behavior.
G4 cannot weaken permission, approval, user-input, MCP, or sandbox behavior.
G5 cannot claim agent-kernel parity without the fixture families above.
G6 cannot default Go without desktop QA, release QA, rollback, and scorecard evidence.
```

Rust remains outside the main runtime. It may be considered only for
profiling-backed shell, sandbox, file-watcher, search/index, or native helper
boundaries, not UI, not Tauri migration, and not the agent/runtime kernel.

## 2026-06-21 - D-0014 Batch 1 TS Oracle Addendum

Status:
Scoped TypeScript runtime proof added. Go G1-G6 remain pending and
non-skippable. No Go scaffold, runtime backend switch, Rust scaffold, or
renderer/preload/main bridge change is authorized by this addendum.

New or hardened TypeScript oracles:

| Fixture family | TypeScript oracle | Future Go parity requirement |
| --- | --- | --- |
| permission gate | `packages/runtime/tests/builtin-tools.test.ts` covers denied approval no-execute and ask/auto/yolo posture mapping. | Go must produce the same execute/block decisions, approval events, and sandbox behavior without exposing Reasonix public posture settings. |
| plan/tool approval split | `packages/runtime/src/loop/agent-loop.test.ts` proves plan mode keeps mutating/guarded tools out of the tool schema while retaining `create_plan`. | Go planner support must not let plan approval bypass tool approval or expand the renderer route surface. |
| complete_step | `packages/runtime/src/tool-test-support/tool/goal-tools.ts` adds `complete_step`; `packages/runtime/tests/goal-tools.test.ts` proves concrete evidence is required and recorded. | Go Goal Runtime must preserve the same tool name, output shape, append-only evidence behavior, and missing-evidence error semantics. |
| evidence ledger | `ThreadGoal.evidenceLedger` is part of the TypeScript contract and renderer mapper pass-through. | Go storage/event replay must preserve ledger entries across HTTP/SSE and renderer projection without Reasonix sidecars or public protocol. |
| Goal completion gate | `ThreadService.setGoal(... status: complete)` and `update_goal complete` reject missing evidence. | Go service/tool layers must both reject evidence-free completion; no alternative API may bypass this gate. |
| blocked-state detection | `packages/runtime/tests/loop.test.ts` proves no-progress failed goal turns can set the goal `blocked` and emit `goal_auto_resume_exhausted`. | Go loop/controller must match the status/event sequence and not hide blocked state behind backend-private state. |
| headless/subagent approval | `packages/runtime/tests/child-agent-executor.test.ts` proves headless child runs inherit `approvalPolicy: never` and cannot execute a guarded tool. | Go job/subagent execution must inherit or narrow the parent permission policy; background/headless work cannot create an approval bypass. |

Remaining G5 gaps before any Reasonix parity claim:

```text
route-level approval and user-input regression fixtures;
renderer approval/review/generated-files non-regression evidence;
longer blocked-state repeated-condition audit;
AutoResearch project-local state and requirement audit;
task/parallel/background/coordinator semantics;
live provider and desktop release evidence where relevant.
```

## 2026-06-21 - D-0014 Batch 2 TS Oracle Addendum

Status:
Scoped TypeScript runtime proof added for AutoResearch project-local state.
Go G1-G6 remain pending and non-skippable. No Go scaffold, runtime backend
switch, Rust scaffold, Tauri migration, or renderer/preload/main bridge change
is authorized by this addendum.

New or hardened TypeScript oracles:

| Fixture family | TypeScript oracle | Future Go parity requirement |
| --- | --- | --- |
| `/goal --research` | The existing chat/goal command path can create a research goal without a new top-level AutoResearch route. | Go must preserve the same runtime goal contract and must not expose Reasonix public protocol or UI shape. |
| AutoResearch state | `AutoResearchProjectStore` creates/resumes `.analytix/autoresearch/<threadId>/task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`. | Go G5 must write equivalent analytix-owned project-local state and reject path escape without touching `REASONIX.md` or `AGENTS.md`. |
| direction tracking | `record_research_direction` writes durable attempted directions and iteration-log records. | Go Goal Runtime must preserve direction audit output and event ordering without creating background-job bypasses. |
| requirement evidence audit | `complete_step requirement_id` updates requirement status and research completion is rejected until every requirement has evidence. | Go service and tool layers must both reject incomplete research goals and preserve append-only evidence ledger semantics. |
| restart/resume | Store fixtures resume existing progress across instances. | Go storage must survive process restart and replay the same descriptor/requirement audit. |
| stable prefix/tool schema isolation | Loop fixtures inject AutoResearch paths as turn context only; stable system prefix and tool schema are not polluted by project-local state. | Go cache/prefix behavior must keep long-task state out of stable prefix/tool-schema hashes unless a future spec deliberately changes the contract. |

Remaining G5 gaps before any Reasonix parity claim:

```text
route-level approval and user-input regression fixtures;
renderer approval/review/generated-files non-regression evidence;
desktop crash/restart drill for AutoResearch state;
Batch 3 tool/context economy with multi-provider cache diagnostics;
Batch 4 task/parallel/background/coordinator semantics;
live provider and release evidence where relevant.
```

## 2026-06-21 - D-0014 Batch 3 TS Oracle Addendum

Status:
Scoped TypeScript runtime proof added for dynamic tool-source lifecycle and
context-economy cache diagnostics. Go G1-G6 remain pending and non-skippable.
No Go scaffold, runtime backend switch, Rust scaffold, Tauri migration, or
renderer/preload/main bridge change is authorized by this addendum.

New or hardened TypeScript oracles:

| Fixture family | TypeScript oracle | Future Go parity requirement |
| --- | --- | --- |
| dynamic tool source | `CapabilityRegistry.connectToolSource` / `disconnectToolSource` prove source lifecycle can change without exposing Reasonix protocol. | Go Tool Registry must support equivalent source lifecycle behind analytix contracts and produce the same advertised tools and diagnostics. |
| stable tool schema | Canonical tool catalog fingerprints stay stable across dynamic source connection order while provider-owned advertisement order remains intact. | Go must keep canonical model tool schema fingerprints deterministic across source reconnects and preserve provider-owned tool ordering where it is semantically meaningful. |
| tool-source diagnostics | Registry diagnostics include tool count, tool names, and catalog fingerprints by source. | Go diagnostics must expose source lifecycle and catalog fingerprints without raw prompt/tool/provider payloads or secrets. |
| cache diagnostics | `CacheDiagnostics` reports `toolSourceChanged` separately from `prefixChanged`; tool-source metadata is outside `prefixHash`. | Go cache diagnostics must distinguish source lifecycle from true stable-prefix/tool-schema drift. |
| token economy/history/memory/compaction | Focused tests keep token economy, request-history hygiene, memory retrieval, compaction archive/history, usage, and provider-cache behavior green. | Go G3/G5 must reproduce these oracle snapshots before cost/cache or long-context superiority claims. |
| multi-provider cache fallback | Provider-cache proof keeps unsupported provider telemetry unknown instead of all-miss. | Go provider registry must not hard-code DeepSeek-only cache assumptions. |

Remaining G5 gaps before any Reasonix parity claim:

```text
live provider cache/cost matrix;
history/memory retrieval ranking benchmarks;
MCP lifecycle UI/operations beyond fixture diagnostics;
Batch 4 task/parallel/background/coordinator semantics;
desktop long-session and release evidence;
Go G1-G6 implementation and rollback plan.
```

## 2026-06-21 - D-0014 Batch 5 G1 Shadow Scaffold

Status:
G0/G1 shadow conformance remains TypeScript-owned, and an isolated Go G1 shadow
scaffold now exists under `packages/runtime-go`. Go G2-G6 remain pending and
non-skippable. This addendum authorizes only conformance-only Go handler code
for `/health`, `/v1/runtime/info`, and `/v1/runtime/tools`; it does not
authorize a default Go backend, renderer-visible Go route, Electron main
integration, Reasonix public protocol, Rust scaffold, Tauri migration, or
renderer/preload/main bridge change.

Conformance fixtures:

| Fixture | TypeScript oracle | Future Go parity requirement |
| --- | --- | --- |
| kernel component manifest | `packages/runtime/src/conformance/go-runtime-kernel-conformance.ts` enumerates Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime. | Any Go implementation must map every component to TS oracle tests before claiming G1/G2/G5 progress. |
| shared G1 route oracle | `packages/runtime/src/conformance/fixtures/go-g1-shadow-oracle.json` records the TS oracle status codes, auth behavior, JSON shape, tool diagnostics, and capability registry body for `/health`, `/v1/runtime/info`, and `/v1/runtime/tools`. | Go and TS tests must compare against the same fixture before G2 starts. |
| product boundary flags | The manifest and fixture hard-code `shadow-conformance-only`, bridge unchanged, `analytix serve` unchanged, no Reasonix public protocol, no default Go backend, and no renderer-visible Go routes. | Go work must preserve these flags until a later spec explicitly changes a backend selection contract. |
| G1 Go shadow routes | `packages/runtime-go` implements only the three G1 conformance handlers and `go test ./...` compares them with the shared TS oracle fixture. | G2 fixture replay must continue without changing Electron, renderer, preload, settings, or the default runtime path. |
| G2 Go shadow route replay | `packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json` drives both TypeScript HTTP/SSE conformance and Go `shadow_g2.go` replay tests for thread list/search/archive, read/update, fork, resume-thread, and SSE `since_seq`. | G3+ must keep the same shadow-only boundary until a later backend-selection spec exists. |
| AutoResearch requirement audit hardening | `packages/runtime/tests/autoresearch-store.test.ts` now rejects unknown `requirement_id` evidence without writing findings. | Go Goal Runtime must reject unknown requirement evidence and preserve append-only findings only for valid requirements or general evidence. |

Kernel component mapping now covers:

```text
Provider Registry
Tool Registry
Controller
Session
Event Sink
Job Manager
Permission Gate
MCP Client
Memory/History Retrieval
Goal Runtime
```

Validation:

```text
go test ./... (inside packages/runtime-go with a local Go toolchain)
npm --prefix packages/runtime test -- autoresearch-store.test.ts go-runtime-conformance.test.ts
```

Remaining G1-G6 gaps before any Go runtime claim:

```text
G2 shadow route replay is accepted only as a fixture-backed conformance slice.
No desktop supervisor backend flag exists.
No default backend selection or rollback path exists.
No G3 provider/model stream implementation exists.
No G4 tool/MCP/sandbox implementation exists.
No G5 agent loop, Goal Runtime, Job Manager, cache, compaction, or resume
implementation exists.
No rollback, packaged QA, Windows QA, signing, or release evidence exists for
a Go backend.
```

## 2026-06-21 - D-0014 Batch 4 TS Oracle Addendum

Status:
Scoped TypeScript runtime proof added for evidence-ledgered delegated
collaboration. Go G1-G6 remain pending and non-skippable. No Go scaffold,
runtime backend switch, Rust scaffold, Tauri migration, or renderer/preload/main
bridge change is authorized by this addendum.

New or hardened TypeScript oracles:

| Fixture family | TypeScript oracle | Future Go parity requirement |
| --- | --- | --- |
| delegated evidence handoff | Completed `delegate_task` child runs can append evidence to an active parent `ThreadGoal.evidenceLedger`. | Go Goal Runtime and Job Manager must preserve child-to-parent evidence handoff before task/parallel parity claims. |
| nested event projection | Child lifecycle metadata carries `evidenceLedgered` / `evidenceLedgerError` through runtime events, event reducer, and renderer mapper. | Go event sink must preserve nested child metadata without exposing Reasonix event semantics. |
| fan-out and maxParallel | Existing fixtures cover single-message `delegate_task` fan-out, queueing at `maxParallel`, queued abort, failure, and interruption. | Go Job Manager must match queue/cancel/failure behavior before enabling background or parallel jobs. |
| permission inheritance | Batch 1 headless/subagent approval inheritance remains a prerequisite for all delegated work. | Go child/headless execution must inherit or narrow parent permissions; no background bypass is allowed. |
| product boundary | No `parallel_tasks`, background job route, planner/executor Coordinator, or top-level Subagent UI is exposed in this proof. | Go work cannot introduce those surfaces without a separate spec and TS oracle fixtures. |

Remaining G5 gaps before any Reasonix parity claim:

```text
first-class task and parallel_tasks contracts;
durable background Job Manager with restart/resume and cancellation routes;
planner/executor Coordinator and model-pairing benchmarks;
transcript continuation/fork proof for child runs;
desktop nested event rendering QA;
Go G1-G6 implementation and rollback plan.
```

## 2026-06-21 - Reasonix Latest Currentness and G2 Shadow Route Replay

Status:
Reasonix `main-v2` latest recheck moved from the D-0014 governance baseline
`49c14762b7da9234525e717830e39a64a2220911` to
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e`. The substantive latest changes
are `45367085` codebase-memory MCP auto-indexing and `d9e453a1` MiMo built-ins
to custom-provider migration. These are parity-hardening inputs for MCP/tool
source startup semantics and provider config behavior. G2 is implemented only
as shadow route replay against TypeScript oracle fixtures.

G2 scope remains read/list/search/archive/fork/session routes plus SSE replay.
It is not a default backend, not Electron supervisor integration, not a
renderer-visible Go route, and not a Reasonix public protocol import.

G2 route replay matrix:

| G2 surface | TypeScript oracle to freeze first | Go shadow replay requirement | Current status |
| --- | --- | --- | --- |
| Runtime health/info/tools precondition | G1 shared oracle fixture and `go-runtime-conformance.test.ts`. | G2 runner keeps G1 route status/body/auth unchanged while adding new replay fixtures. | pass |
| Thread list/read | `go-g2-route-replay-oracle.json` and `go-runtime-conformance.test.ts`. | Same status codes, list summaries, read body, and `latestSeq`. | pass |
| Search/archive | same G2 oracle. | Same archive/search result schemas and no renderer contract changes. | pass |
| Fork/session lineage | same G2 oracle. | Same fork parent/child ids, workspace metadata, and summary body. | pass |
| Resume-thread | same G2 oracle. | Same `POST /v1/sessions/{id}/resume-thread` status/body. | pass |
| SSE replay | same G2 oracle and Go `shadow_g2.go`. | Same `since_seq` replay frames and caught-up empty replay. | pass |
| Usage summary | provider/cache oracle fixtures. | Same prompt/completion/cache hit/miss fields where supported; unsupported cache telemetry remains unknown. | fixture-backed in TypeScript; live providers blocked |

Reasonix latest parity inputs:

| Latest Reasonix item | Future fixture implication | Non-goal |
| --- | --- | --- |
| codebase-memory MCP auto-indexing | Future G4/G5 Tool Registry and MCP Client fixtures should model known cwd-aware indexers that need workspace-root cwd, low-priority execution, and background startup under a shared host. | Do not expose Reasonix plugin protocol or add a top-level MCP/indexer UI entry in G2. |
| MiMo built-ins -> custom providers | Future G3 provider fixtures should compare custom-provider normalization, provider/model aliases, and migration behavior against analytix Xiaomi/MiMo product requirements. | Do not remove analytix Xiaomi/MiMo presets or claim live provider parity without credentials. |

Production scan evidence:

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Latest Reasonix ref | `git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2` | `91fe06db6177bb052fc8ae1a3081d60bfc104a4e` |
| Kun baseline refs | `git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14` | unchanged: `8602476`, `247076f`, `201a146`, `06be05d` |
| G1/G2 TS oracle | `npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts tests/provider-cache-proof.test.ts tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files, 10 tests |
| G1/G2 Go package | temporary official `go1.26.4.darwin-arm64` from `go.dev`, SHA256 verified, then `cd packages/runtime-go && go test ./...` | pass; temporary toolchain removed |
| Default backend scan | `rg` for `runtime-go`, backend-switch, renderer-visible Go, and related terms across `src`, `preload`, `shared`, config, scripts, workflows | pass: no hits outside conformance-owned runtime files |
| G2 shadow replay files | `packages/runtime-go/shadow_g2.go`, `packages/runtime-go/shadow_test.go`, and shared G2 oracle | accepted as shadow-only conformance; no Electron/default backend path |

Remaining risks:

```text
G2 exists only as static shadow replay, not a general Go backend.
No desktop supervisor backend flag or rollback plan exists.
No live provider matrix or cost reconciliation exists.
No packaged Go backend QA exists.
```

## 2026-06-21 - G3/G4 Shadow Oracle Addendum

Status:
G3 and G4 now have TypeScript-owned shadow oracle fixtures and Go output
comparison tests. This is conformance evidence only. It does not implement a
Go provider runtime, Go tool host, Electron supervisor integration, default Go
backend, renderer-visible Go route, or `analytix serve` replacement.

New fixtures:

| Stage | Fixture | Coverage |
| --- | --- | --- |
| G3 | `go-g3-provider-streaming-usage-cache-oracle.json` | Provider usage matrix for unsupported OpenAI-compatible, DeepSeek prompt cache, Responses cached tokens, Anthropic cache fields; streaming event kinds and sanitized cache diagnostics. |
| G4 | `go-g4-tools-approval-user-input-mcp-oracle.json` | Tool catalog order, denied approval no-execute, cancelled user input, MCP lifecycle/known override, planner read-only toolset, and remote-entry boundary. |

Go shadow output:

```text
packages/runtime-go/shadow_g3g4.go builds G3/G4 conformance output from the
shared JSON fixtures. `shadow_test.go` compares that output to each fixture's
`expectedOutput`.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining G3/G4 blockers:

```text
No live provider streaming implementation in Go.
No live Go tool host or MCP client implementation.
No backend selection or rollback path.
No packaged Electron QA for Go.
No default backend or renderer-visible Go route.
```

## 2026-06-21 - G5 Oracle Inventory Addendum For 9e56 MCP/Task Jobs

Status:
This addendum updates the future G5 oracle inventory only. No Go files are
changed by the Reasonix 9e56 stage, and no Go backend path is authorized.

New TypeScript oracle inputs:

| Fixture family | TypeScript oracle | Future Go G5 requirement |
| --- | --- | --- |
| MCP startup retry-all | `packages/runtime/tests/mcp-tool-provider.test.ts` retries every failed startup server and keeps a suspended source from late-registering tools. | A Go MCP client/registry must retry all eligible failed servers and must not reinstall disabled providers after a late connection. |
| MCP source tombstone | `CapabilityRegistry.suspendToolSource` / `resumeToolSource` and `capability-registry.test.ts`. | Go Tool Registry must model a disable tombstone/generation before live per-server enable/disable can claim parity. |
| Task job routes | `packages/runtime/tests/task-job-orchestration-oracle.test.ts` covers authenticated `/v1/runtime/task-jobs/wait|output|kill`, partial output, bounded wait, kill, and not-found. | Go Job Manager must match route auth, status/body/error shape, output offsets, cancellation, and durable store behavior before G5 loop claims. |
| Provider probe diagnostics | `src/main/provider-connection.test.ts` covers secret redaction for model-list probe URL/body/network errors. | Any future Go/provider control path must keep request attribution secret-safe and must not leak keys in diagnostics. |

Remaining G5 blockers:

```text
No Go full agent loop.
No Go Job Manager implementation.
No Go cache/compaction/resume/interrupt implementation.
No cross-backend live MCP or provider matrix.
No Electron supervisor integration, rollback plan, packaged QA, or default
backend contract.
```

## 2026-06-21 - G5 Full-Loop Oracle Inventory Closure

Status:
This update adds a machine-readable G5 inventory fixture and manifest binding.
It is TypeScript-owned and shadow-only. No Go files are changed, and no Go
backend path is authorized.

New fixture:

```text
packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json
```

Inventory:

| G5 surface | TypeScript oracle source | Future Go requirement |
| --- | --- | --- |
| Full loop | `loop.test.ts`, `goal-tools.test.ts`, `goal-repetition-guard.test.ts`, `approval-user-input-route-oracle.test.ts` | Match turn control, goal evidence, approval/user-input, and no-progress recovery without changing event semantics. |
| Job Manager | `task-job-orchestration-oracle.test.ts`, `delegation-runtime.test.ts`, `child-agent-executor.test.ts` | Match internal task/parallel/background/planner contract, wait/output/kill, dependency validation, nested metadata, parent Goal evidence, and transcript identity. |
| Cache/compaction | `provider-cache-proof.test.ts`, `context-compactor.test.ts`, `request-history-hygiene.test.ts`, `token-economy.test.ts` | Match stable prefix, provider usage/cache telemetry, cache curve guard, compaction, and history repair. |
| Resume/interrupt | `loop.test.ts`, `thread-service.test.ts`, `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts` | Match thread/session route replay, SSE ordering, resume-thread, and interrupt semantics. |
| MCP/indexer | `mcp-tool-lifecycle-oracle.test.ts`, `mcp-tool-provider.test.ts`, `capability-registry.test.ts`, `mcp-config.test.ts` | Match retry-all, tombstone/suspension, codegraph/codebase-memory cwd/priority/backgroundStart, and secret-safe diagnostics. |

Product boundary:

```text
electronMainConnected: false
defaultGoBackendEnabled: false
rendererVisibleGoRoutesAllowed: false
reasonixPublicProtocolAllowed: false
analytixServeContractUnchanged: true
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts
```

Remaining blockers:

```text
No Go full-loop implementation, no Electron supervisor integration, no
rollback/default backend decision, no packaged QA, and no live provider/MCP
matrix. G5 remains an inventory, not parity.
```

## 2026-06-21 - G5 Shadow Implementation Attempt Blocked By Toolchain

Status:
The stage attempted to advance G5 from TS-owned oracle inventory toward shadow
implementation slices for jobs/cache/session/resume/interrupt/MCP replay. No Go
files were changed because the local environment does not provide a `go`
executable, and this project requires `cd packages/runtime-go && go test ./...`
for any Go change.

Observed blocker:

```text
command -v go && go version
```

Result:

```text
no output; command exited non-zero
```

Decision:

```text
Do not make unvalidated Go changes in this stage. Keep G5 as the TS-owned
inventory fixture until a Go toolchain is available and the package test can
run locally.
```

Future G5 implementation slices:

| Slice | Required evidence before claim |
| --- | --- |
| Jobs | Go shadow output must match task/parallel execution, dependency order, output offsets, kill/wait, and restart stale-job reconciliation fixtures. |
| Cache | Go shadow output must match stable prefix, cache hit/miss parsing, cache curve guard, and unsupported-provider unknown behavior. |
| Session/resume/interrupt | Go shadow output must match thread/session route replay, SSE order, resume-thread, interrupt semantics, and transcript identity. |
| MCP/indexer | Go shadow output must match retry-all, tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics fixtures. |

Product boundary:

```text
No Electron main integration.
No renderer-visible Go routes.
No `analytix serve` replacement.
No default Go backend.
No Reasonix public protocol.
```

## 2026-06-21 - G5 Shadow Output Slice

Status:
G5 now has a Go shadow output slice. This supersedes the earlier local
toolchain blocker for this stage, because a temporary official Go toolchain was
downloaded from `go.dev`, SHA256-verified, used for `gofmt` and package tests,
and not committed.

New Go files / tests:

```text
packages/runtime-go/internal/server/g5_shadow.go
packages/runtime-go/shadow_g5.go
packages/runtime-go/shadow_test.go
```

G5 output now replays:

| Slice | Shadow output evidence |
| --- | --- |
| Source oracles | `sourceOracleIds` from `go-g5-full-loop-oracle.json`. |
| Full loop | TS-owned full-loop test inventory. |
| Jobs | Job manager tests, internal task-job routes, and required behaviors including output offsets, dependency guards, planner read-only, nested metadata, parent Goal evidence, and transcript identity. |
| Cache/compaction | Provider-cache, context compactor, request-history hygiene, and token-economy tests. |
| Session/resume/interrupt | Loop, thread-service, G2 route replay, and G3/G4 conformance tests. |
| MCP/indexer | MCP lifecycle, provider, registry, and config tests. |
| Boundary | Electron main disconnected, default Go backend disabled, renderer-visible Go route disallowed, Reasonix public protocol disallowed. |

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining blockers:

```text
No Go full-loop executor, no live Job Manager, no live Go cache/session/MCP
runtime, no Electron supervisor integration, no rollback/default backend
decision, no packaged QA, and no live provider/MCP matrix. This is shadow
implementation evidence, not G5 parity.
```

## 2026-06-21 - G5 Composite Shadow Replay

Status:
G5 now has cross-fixture shadow replay. `BuildG5ShadowSlicesOutput` reads the
same TypeScript-owned fixtures that define the runtime contract and emits a Go
summary for jobs, cache, session/resume/SSE, and MCP/indexer.

Composite inputs:

| Fixture | Replayed fields |
| --- | --- |
| `task-job-orchestration-oracle.json` | task/parallel tool names, internal routes, dependency order, planner read-only tools, nested metadata, transcript identity flags. |
| `provider-cache-oracle.json` | stable prefix hash, tools hash, provider usage case ids, release guard statuses, privacy forbidden substrings, live superiority policy. |
| `go-g2-route-replay-oracle.json` | route ids, resume route ids, fork route ids, SSE replay route ids. |
| `mcp-tool-lifecycle-oracle.json` | provider id, retry failed servers, expected connected servers, known override, active live-local paths, late tombstone, redacted diagnostic. |

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining blockers:

```text
Composite replay is not live runtime execution. No Go full-loop executor, live
Job Manager, cache/session/MCP runtime, Electron integration, backend default,
rollback plan, packaged QA, or live provider/MCP matrix exists.
```

## 2026-06-21 - G5 Runner Shadow Replay Extension

Status:
G5 composite replay now includes the new planner/executor and durable runner
restart drill fields from the TypeScript task-job oracle. Go remains
shadow-only and does not own runtime execution.

New replay fields:

| Slice | Shadow output evidence |
| --- | --- |
| Planner/executor | Planner kind `planner`, planner policy `readOnly`, executor policy `inherit`, failure and cancellation status, and booleans requiring failure/cancellation propagation. |
| Durable runner restart | Rehydrated running/queued job ids, expected rehydrated count, wait status, kill status, and output appended after restart. |

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining blockers:

```text
No live Go planner/executor, no Go durable Job Manager, no Go cache/session/MCP
runtime, no Electron integration, no backend default, no rollback plan, no
packaged QA, and no live provider/MCP matrix exists.
```

## 2026-06-21 - G5 Control Shadow Replay Extension

Status:
G5 composite replay now includes a `controlReplay` slice for the Reasonix
881-era controls absorbed behind analytix contracts: cancel/result pairing,
task-job parent cancellation, and runtime step-limit resolution.

New replay fields:

| Slice | Shadow output evidence |
| --- | --- |
| Cancel | Running-turn cancel remains escapable; accepted tool calls are paired by call id; completed batch results persist; cancelled/unstarted calls use `tool_call_cancelled` with `aborted` result status; new parallel tools are not scheduled after cancel. |
| Task jobs | Parent abort kills running durable jobs, preserves output offsets, and returns skipped unstarted parallel tasks with `cancelled: parent turn aborted before task execution`. |
| Step limits | Default limit is 64, user-global/session/turn/planner/headless overrides are represented, default `0` can disable the guard, dynamic budget state is not in stable prefix, and the failure code remains `turn_step_limit_exceeded`. |

## 2026-06-21 - G5 Executable Control Shadow Cases

Status:
Scoped shadow-conformance extension implemented.

What changed:

```text
Go G5 now executes pure in-memory control cases from the TS-owned oracle instead
of only copying the high-level controlReplay object.
```

Evidence:

| Area | Fixture / Go proof |
| --- | --- |
| Cancelled batch results | `go-g5-full-loop-oracle.json` now includes `controlExecutableCases.cancel`; `BuildG5ControlExecutableOutput` preserves completed tool results, emits `tool_call_cancelled` for running/unstarted calls, and schedules no new calls after cancel. |
| Task-job parent cancel | `controlExecutableCases.taskJobs` proves completed/running/queued jobs become completed/killed/skipped with output offsets preserved. |
| Step limits | `controlExecutableCases.stepLimits` proves default/user-global/session/turn/planner/headless/zero-default/delegate inheritance without putting dynamic limits into the stable prefix. |
| Test | `packages/runtime-go/shadow_test.go` compares the executable output to the TS-owned fixture. |

Validation:

```text
/tmp/analytix-go-toolchain/go/bin/go version
go version go1.25.11 darwin/arm64
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
ok github.com/analytix/runtime-go
```

Boundary:

```text
No live Go full-loop executor, no Go Job Manager, no Electron integration, no
renderer-visible Go route, no analytix serve replacement, and no default Go
backend. G6 remains blocked by live/runtime/packaging/rollback evidence.
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && go test ./...
```

Boundary:

```text
This is still shadow replay only. No Go full-loop executor, no Electron main
integration, no default Go backend, and no renderer-visible Go control route
exists.
```

## 2026-06-21 - Approval/User-Input Cleanup Guard, No Go Score Increase

Status:
No Go runtime code changed in the approval/user-input abort cleanup batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

G3/G4/G5 shadow conformance remains green after the TypeScript runtime adds
approval expiration and user-input abort replay closure.

Boundary:

```text
This batch does not implement a Go approval manager, Go user-input gate, live
Go loop, renderer-visible Go route, default Go backend, or G6 rollback path.
```

## 2026-06-21 - Renderer Approval Live-Card Tests, No Go Score Increase

Status:
No Go runtime code changed in the renderer approval live-card store evidence
batch.

Evidence:

```text
npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The batch proves TypeScript renderer store behavior for main-thread and side
approval cards. It does not change G1-G6 Go fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go approval manager, Go user-input gate, live
Go loop, renderer-visible Go route, default Go backend, Electron integration,
or G6 rollback path.
```

## 2026-06-21 - Preload Bridge/API Sovereignty Test, No Go Score Increase

Status:
No Go runtime code changed in the preload bridge/API sovereignty batch.

Evidence:

```text
npm run test -- src/preload/preload-sandbox.test.ts
```

Result:

The batch proves TypeScript/Electron preload source boundaries: only
`analytix` is exposed through `contextBridge`, `Window` declares only
`analytix`, and shared public API facade names do not expose Kun, Reasonix,
DeepSeek, or deprecated GUI bridge identities. It does not change G1-G6 Go
fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go bridge, Go preload path, live Go loop,
renderer-visible Go route, default Go backend, Electron integration, or G6
rollback path.
```

## 2026-06-21 - Renderer Thread Lifecycle Oracle, No Go Score Increase

Status:
No Go runtime code changed in the renderer thread lifecycle HTTP oracle batch.

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

Result:

The batch proves TypeScript renderer adapter behavior for thread lifecycle
calls through `/v1/threads` and existing analytix runtime bridge methods. It
does not change G1-G6 Go fixtures or Go shadow output.

Boundary:

```text
This batch does not implement Go thread routes, live Go SSE, renderer-visible
Go routing, default Go backend, Electron integration, or G6 rollback path.
```

## 2026-06-21 - Settings/Provider EndpointFormat Test, No Go Score Increase

Status:
No Go runtime code changed in the settings/provider endpoint-format persistence
batch.

Evidence:

```text
npm run test -- src/main/settings-store.test.ts
```

Result:

The batch proves Electron/main settings persistence for runtime and provider
endpoint formats. It does not change G1-G6 Go fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go provider client, Go settings loader, live Go
loop, renderer-visible Go route, default Go backend, Electron integration, or
G6 rollback path.
```

## 2026-06-21 - Go G5 Planner-Forbidden Shadow Replay

Status:
Go runtime code changed only in shadow conformance. G5 now consumes the
TS-owned `plannerForbiddenToolset` from the task-job oracle and emits it in
`jobReplay`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ShadowSlicesOutput` now proves Go G5 shadow can replay both
`plannerReadOnlyToolset` and `plannerForbiddenToolset` from the TS-owned
task-job oracle.

Boundary:

```text
This batch does not implement a Go planner/executor, Go Job Manager, live Go
loop, provider/cache/session/MCP runtime, renderer-visible Go route, default
Go backend, Electron integration, or G6 rollback path.
```

## 2026-06-21 - G4 Structured User-Input Validation Shadow

Status:
Go runtime code changed only in shadow conformance. G4 tools/approval/user-input
output now records the structured user-input invalid-case contract from the
TS-owned fixture.

Evidence:

```text
npm --prefix packages/runtime test -- tests/builtin-tools.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG4ToolsConformanceOutput` now emits:

- `userInputValidationCode: "invalid_user_input_request"`;
- `userInputValidationCases` for too many questions, one-option choice sets,
  too many options, and duplicate labels;
- `userInputInvalidOpensGate:false`.

Boundary:

```text
This batch does not implement a Go approval/user-input manager, live Go loop,
provider/cache/session/MCP runtime, renderer-visible Go route, default Go
backend, Electron integration, G6 rollback path, or Reasonix ask protocol.
```

## 2026-06-21 - Write-Inline Custom Endpoint Tests, No Go Score Increase

Status:
No Go runtime code changed in the write-inline custom full endpoint proof
batch.

Evidence:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The batch proves TypeScript main-process Write behavior for custom full
endpoint URLs ending in `/responses` and `/messages`. It does not change G1-G6
Go fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go provider client, Go write-inline path, live
Go loop, renderer-visible Go route, default Go backend, Electron integration,
or G6 rollback path.
```

## 2026-06-21 - Auto-Model Route Cache Test, No Go Score Increase

Status:
No Go runtime code changed in the auto-model route cache lifecycle proof batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto model" --no-file-parallelism --maxWorkers=1
```

Result:

The batch proves TypeScript `AgentLoop` behavior for same-turn auto route reuse
and next-turn re-routing. It does not change G1-G6 Go fixtures or Go shadow
output.

Boundary:

```text
This batch does not implement a Go auto-router, Go planner/executor, live Go
loop, renderer-visible Go route, default Go backend, Electron integration, or
G6 rollback path.
```

## 2026-06-21 - Provider Request-Shape Matrix Shadow IDs

Status:
Go runtime code changed only in shadow conformance. G3/G5 outputs now carry
request-shape case ids from the TypeScript provider-cache oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

G3/G5 shadow output now includes request-shape case ids for:

- DeepSeek official chat completions;
- OpenAI-compatible chat completions;
- OpenAI Responses;
- Anthropic Messages;
- custom Responses full endpoint mode.

Boundary:

```text
This batch does not implement a Go provider client, live Go loop,
provider/session/MCP runtime, renderer-visible Go route, default Go backend,
Electron integration, or G6 rollback path.
```

## 2026-06-21 - Combined Control Composition Shadow

Status:
Go runtime code changed only in shadow conformance. G5 control executable output
now computes a combined control case across auto-route cache, step-limit, and
cancelled tool-result invariants.

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto route|step limit" --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes:

- same-turn auto-route reuse until the resolved step limit fires;
- next-turn reroute requirement;
- stable-prefix exclusion for classifier and step-limit control state;
- step-limit error code `turn_step_limit_exceeded`;
- cancelled result code `tool_call_cancelled`;
- completed result preservation and unstarted result status `aborted`.

Boundary:

```text
This batch does not implement a Go auto-router, Go AgentLoop, Go Job Manager,
provider/session/MCP runtime, renderer-visible Go route, default Go backend,
Electron integration, or G6 rollback path.
```

## 2026-06-21 - Write-Inline Provider Tests, No Go Score Increase

Status:
No Go runtime code changed in the write-inline provider request-surface proof
batch.

Evidence:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The batch proves TypeScript main-process write-inline behavior for OpenAI
Responses and Anthropic Messages endpoint formats. It does not change G1-G6 Go
fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go provider client, Go write-inline path, live
Go loop, renderer-visible Go route, default Go backend, Electron integration,
or G6 rollback path.
```

## 2026-06-21 - Go G5 Planner Tool-Policy Executable Shadow

Status:
Go runtime code changed only in shadow conformance. G5 control executable
output now computes planner tool-policy gating from TS-owned fixture input.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes:

- Plan step 0 advertised tools: read-only toolset plus `create_plan`;
- Plan step 1 advertised tools: `create_plan` only;
- forged `task` call rejection: `failed` / `tool_dispatch_rejected` /
  `executed:false`.

Boundary:

```text
This batch does not implement a Go planner/executor, Go Job Manager, live Go
loop, provider/cache/session/MCP runtime, renderer-visible Go route, default
Go backend, Electron integration, or G6 rollback path.
```

## 2026-06-21 - Planner Task-Tool Gating Added To TS Oracle

Status:
Go runtime code is unchanged. The TypeScript oracle for task-job/planner
behavior gained an explicit `plannerForbiddenToolset`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "keeps internal task tools out of plan mode" --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Future Go G5 loop work must preserve that internal `task` and `parallel_tasks`
tools are not part of Plan mode's read-only tool surface, and forged task calls
are rejected before child work starts.

Boundary:

```text
This batch does not implement a Go planner/executor, Go Job Manager, live Go
loop, renderer-visible Go route, default Go backend, Electron integration, or
G6 rollback path.
```

## 2026-06-21 - User-Input Live-Card Tests, No Go Score Increase

Status:
No Go runtime code changed in the user-input live-card store evidence batch.

Evidence:

```text
npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The batch proves TypeScript renderer store behavior for main-thread and side
user-input cards, including item id and request id matching. It does not change
G1-G6 Go fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go approval manager, Go user-input gate, live
Go loop, renderer-visible Go route, default Go backend, Electron integration,
or G6 rollback path.
```

## 2026-06-21 - Provider Selection Test, No Go Score Increase

Status:
No Go runtime code changed in the provider-selection request-shape oracle batch.

Evidence:

```text
npm --prefix packages/runtime test -- src/adapters/model/multi-provider-model-client.test.ts
```

Result:

The batch proves TypeScript runtime dispatch behavior: a thread-selected custom
provider uses the exact custom full endpoint and Messages request shape, while a
missing provider falls back to the default OpenAI-compatible request path.

Boundary:

```text
This batch does not implement a Go provider client, live Go loop,
renderer-visible Go route, default Go backend, Electron integration, or G6
rollback path.
```

## 2026-06-21 - Scheduled Detector Endpoint Tests, No Go Score Increase

Status:
No Go runtime code changed in the scheduled detector custom endpoint inference
batch.

Evidence:

```text
npm run test -- src/main/claw-scheduled-task-detector.test.ts
```

Result:

The batch proves TypeScript main-process scheduled detector behavior for custom
full `/messages` and `/chat/completions` endpoints. It does not change G1-G6 Go
fixtures or Go shadow output.

Boundary:

```text
This batch does not implement a Go scheduled-task detector, Go provider client,
live Go loop, renderer-visible Go route, default Go backend, Electron
integration, or G6 rollback path.
```

## 2026-06-21 - Forbidden Public Route Tests, No Go Score Increase

Status:
No Go runtime code changed in the forbidden public runtime route oracle batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts
```

Result:

The TypeScript runtime router now proves `/v1/runtime/go` and
`/v1/runtime/go/health` return structured 404s. Go remains shadow-only.

Boundary:

```text
This batch does not implement a Go HTTP server, live Go loop,
renderer-visible Go route, default Go backend, Electron integration, or G6
rollback path.
```

## 2026-06-21 - Rehydrated Task-Job Route Test, No Go Score Increase

Status:
No Go runtime code changed in the rehydrated task-job route oracle batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Result:

The TypeScript oracle now proves rehydrated running/queued task jobs remain
operable through authenticated `/v1/runtime/task-jobs/output`, `wait`, and
`kill` routes. This strengthens the G5 durable-runner contract without changing
Go shadow output.

Boundary:

```text
This batch does not implement a Go Job Manager, Go HTTP server, live Go loop,
renderer-visible Go route, default Go backend, Electron integration, or G6
rollback path.
```

## 2026-06-21 - Task-Job Approval Denial Test, No Go Score Increase

Status:
No Go runtime code changed in the task-job approval deny no-execute batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Result:

The TypeScript oracle now proves denied `task` and `parallel_tasks` calls create
no durable jobs or child runs. Future Go approval/job manager work must preserve
approval-before-side-effect ordering.

Boundary:

```text
This batch does not implement a Go approval manager, Go Job Manager, Go HTTP
server, live Go loop, renderer-visible Go route, default Go backend, Electron
integration, or G6 rollback path.
```

## 2026-06-21 - Go G5 Task-Job Approval Deny Shadow Replay

Status:
Go G5 shadow replay now consumes the TS-owned task-job approval denial fixture.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`task-job-orchestration-oracle.json` now records `approvalDenyNoExecute`, and
`go-g5-full-loop-oracle.json` requires `jobReplay.approvalDenyNoExecute`.
`BuildG5ShadowSlicesOutput` emits denied tool names, approval ids,
`createsDurableJobs: false`, and `createsChildRuns: false` from the TS-owned
source fixture.

Boundary:

```text
This is executable Go shadow replay only. It does not implement a Go approval
manager, Go Job Manager, Go HTTP server, live Go loop, renderer-visible Go
route, default Go backend, Electron integration, G6 rollback path, Reasonix
public sub-agent/job protocol, or packaged desktop approval QA.
```

## 2026-06-21 - Go G5 Task Approval Deny Executable Control

Status:
Go G5 control executable output now computes task approval denial no-side-effect
results from the TS-owned oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now includes
`controlExecutableCases.approvalDeny`. `BuildG5ControlExecutableOutput` calls
`replayG5ApprovalDeny`, which returns denied tool names, approval ids,
`approvalItemCount`, `createsDurableJobs: false`, and `createsChildRuns:
false`. The executable case is derived from the TS task-job oracle's
`approvalDenyNoExecute` record.

Boundary:

```text
This is pure Go shadow control. It does not implement a Go approval manager,
Go Job Manager, Go HTTP server, live Go loop, renderer-visible Go route,
default Go backend, Electron integration, G6 rollback path, Reasonix public
sub-agent/job protocol, or packaged desktop approval QA.
```

## 2026-06-21 - Go G3/G5 Provider Cache Accounting Shadow

Status:
Go G3 provider conformance output and Go G5 cache replay now compute cache
accounting from the TS-owned provider/cache oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g3-provider-streaming-usage-cache-oracle.json` now includes
`cacheAccounting`, and `go-g5-full-loop-oracle.json` carries the same accounting
inside `cacheReplay`. `BuildG3ProviderConformanceOutput` and
`BuildG5ShadowSlicesOutput` compute supported cache telemetry case ids,
unsupported unknown case ids, DeepSeek/OpenAI/Anthropic case ids, total cache
hit/miss tokens, aggregate hit rate, and `unsupportedProvidersCountedAsMisses:
false` from fixture-owned provider usage cases.

Boundary:

```text
This is fixture-only Go shadow accounting. It does not implement a Go provider
client, live Go loop, renderer-visible Go route, default Go backend, Electron
integration, G6 rollback path, live provider superiority, credentialed
provider matrix, Reasonix provider protocol, or packaged provider settings QA.
```

## 2026-06-21 - Auto-Router Failure Isolation, No Go Score Increase

Status:
No Go runtime code changed in the auto-router failure usage isolation batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t 'falls back to a concrete heuristic model without recording router usage'
```

Result:

The TypeScript runtime loop now proves `_auto_router` failure falls back to a
heuristic concrete model while router usage/cache telemetry is not recorded as
main thread usage and is not accumulated by `UsageService`.

Boundary:

```text
This batch does not implement a Go router, Go provider client, live Go loop,
renderer-visible Go route, default Go backend, Electron integration, G6
rollback path, Reasonix auto-plan config, or managed settings rebuild proof.
```

## 2026-06-21 - Go G5 Task-Job Stale Restart Reconciliation Shadow

Status:
Go G5 executable control now computes queued/running stale task-job restart
reconciliation from the TS-owned task-job oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`task-job-orchestration-oracle.json` now includes
`durableRunner.staleReconcile` for a running orphan, queued orphan, restart
reason, expected failed status, and reconciled count. `go-g5-full-loop-oracle.json`
adds `controlExecutableCases.taskJobs.staleReconcile`, and
`replayG5TaskJobStaleReconcile` returns both jobs as failed with the restart
reason.

Boundary:

```text
This is pure Go shadow control. It does not implement a Go Job Manager, live Go
task/job execution, Go HTTP route, renderer-visible Go route, default Go
backend, Electron integration, G6 rollback path, Reasonix public sub-agent/job
protocol, SessionAPI, or packaged desktop restart QA.
```

## 2026-06-21 - Go G4 MCP Annotation Approval No-Execute Shadow

Status:
Go G4 tools/approval/user-input/MCP shadow now computes the annotated MCP
approval no-execute claim from the TS-owned MCP lifecycle oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`mcp-tool-lifecycle-oracle.json` now includes `approvalAnnotations` for a
destructive/openWorld MCP tool. `go-g4-tools-approval-user-input-mcp-oracle.json`
adds `mcpApprovalAnnotatedNoExecute`, and `BuildG4ToolsConformanceOutput`
computes it from the fixture decision/executed fields.

Boundary:

```text
This is pure Go shadow control. It does not implement a Go MCP client, live Go
tool execution, Go HTTP route, renderer-visible Go route, default Go backend,
Electron integration, Reasonix MCP public protocol, MCP-indexer top-level entry,
live MCP credential matrix, or packaged desktop approval QA.
```

## 2026-06-21 - Go G4 User-Input Submitted Route Shadow

Status:
Go G4 tools/approval/user-input/MCP shadow now computes submitted-answer echo
and answer-free SSE replay claims from the TS-owned user-input route oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`approval-user-input-route-oracle.json` now includes `submittedUserInput`.
`go-g4-tools-approval-user-input-mcp-oracle.json` adds
`userInputSubmittedAnswersEchoed` and `userInputResolvedEventOmitsAnswers`, and
`BuildG4ToolsConformanceOutput` computes both from fixture-owned submitted-route
fields.

Boundary:

```text
This is pure Go shadow control. It does not implement a Go user-input route,
live Go tool execution, Go HTTP route, renderer-visible Go route, default Go
backend, Electron integration, Reasonix public user-input protocol, answer
archival in SSE history, live cross-device delivery, or packaged GUI live-card QA.
```

## 2026-06-21 - Task-Job Route Auth Matrix, No Go Score Increase

Status:
No Go runtime code changed in the task-job route auth matrix batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

Result:

The TypeScript runtime route oracle now verifies `/v1/runtime/task-jobs/wait`,
`/v1/runtime/task-jobs/output`, and `/v1/runtime/task-jobs/kill` all reject
missing runtime tokens with 401.

Boundary:

```text
This batch does not implement Go task-job routes, Go HTTP auth, live Go tool
execution, renderer-visible Go route, default Go backend, Electron integration,
Reasonix public job protocol, or packaged desktop route QA.
```

## 2026-06-21 - Connect Phone Product Copy, No Go Score Increase

Status:
No Go runtime code changed in the Connect Phone product-copy sovereignty batch.

Evidence:

```text
npm run test -- src/main/claw-runtime.test.ts src/shared/app-settings.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts
```

Result:

Renderer locales, main runtime replies, IPC mirror initialization errors, and
shared prompt natural-language hints now use Connect Phone wording. Internal
`claw` names remain compatibility contracts.

Boundary:

```text
This batch does not implement Go HTTP routes, Go Connect Phone handling, live Go
tool execution, renderer-visible Go route, default Go backend, Electron
integration, Reasonix public protocol, Rust/Tauri path, or release readiness.
```

## 2026-06-21 - Managed Runtime Provider Currentness, No Go Score Increase

Status:
No Go runtime code changed in the managed runtime provider currentness and
identity guard batch.

Evidence:

```text
npm --prefix packages/runtime test -- tests/analytix-system-prompt.test.ts tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

TypeScript/shared/main tests now prove selected provider endpoint/profile
currentness changes the managed runtime settings key and child provider env
snapshot. Runtime prompt and preload/settings tests guard model-visible/public
identity.

Boundary:

```text
This batch does not implement Go provider clients, Go HTTP routes, live Go tool
execution, renderer-visible Go route, default Go backend, Electron integration,
Reasonix public protocol, user-visible auto-plan settings, Rust/Tauri path, or
release readiness.
```

## 2026-06-21 - Provider Endpoint URL Builder Parity, No Go Score Increase

Status:
No Go runtime code changed in the provider endpoint URL builder parity batch.

Evidence:

```text
npm run test -- src/shared/openai-compat-url.test.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Shared TypeScript URL construction now covers versioned responses/messages
bases and known endpoint paths for Schedule and Write auxiliary consumers.

Boundary:

```text
This batch does not implement Go provider clients, Go HTTP routes, live Go tool
execution, renderer-visible Go route, default Go backend, Electron integration,
Reasonix provider protocol, Rust/Tauri path, or release readiness.
```

## 2026-06-22 - Go G5 User-Input Gate Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves submitted/cancelled user-input gate
behavior from TS/G4 route evidence into G5 executable shadow calculations.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records `controlExecutableCases.userInput`
and `shadowSlicesExpectedOutput.approvalUserInputReplay`. The Go shadow package
computes submitted status, answer count, HTTP answer echo, replay answer
omission, cancelled status, late resolve rejection, and pending-after counts
from TS-owned fixtures.

Boundary:

```text
This batch does not implement a Go user-input manager, Go user-input HTTP
route, live Go task execution, renderer-visible Go route, default Go backend,
Electron integration, Reasonix ask/session protocol, Rust/Tauri path, packaged
desktop live-card QA, or release readiness.
```

## 2026-06-22 - Go G5 AutoResearch Project-Local State Shadow

Status:
Go G5 remains shadow-only. This batch moves AutoResearch project-local state and
requirement-audit boundaries into G5 executable shadow calculations.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/autoresearch-store.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records `controlExecutableCases.autoResearch`.
The TypeScript conformance test creates real `AutoResearchProjectStore` state
under `.analytix/autoresearch/<threadId>/`, verifies the required state files,
and proves unknown requirement evidence does not write findings. The Go shadow
package computes the same project-local boundary, no `REASONIX.md`/`AGENTS.md`
write, no stable-prefix/tool-schema pollution, and no top-level route exposure.

Boundary:

```text
This batch does not implement a Go AutoResearch state store, Go HTTP route,
live Go task execution, renderer-visible Go route, default Go backend, Electron
integration, Reasonix AutoResearch/project protocol, top-level AutoResearch
navigation, Rust/Tauri path, packaged desktop long-task QA, or release
readiness.
```

## 2026-06-22 - Go G5 MCP Lifecycle Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves MCP retry, tombstone/resume, and
diagnostic-redaction boundaries into G5 executable shadow calculations.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-lifecycle-oracle.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records `controlExecutableCases.mcpLifecycle`.
The TypeScript conformance test derives it from `mcp-tool-lifecycle-oracle.json`
and `runLiveLocalIndexerProof`, while the Go shadow package computes retry
attempts, connected/error ids, active paths after tombstone/resume, restart
snapshot state, and redacted diagnostics with `leaksSecret:false`.

Boundary:

```text
This batch does not implement a Go MCP client, Go MCP HTTP route, live Go tool
execution, renderer-visible Go route, default Go backend, Electron integration,
Reasonix MCP protocol, top-level MCP-indexer navigation, credentialed MCP
matrix, Rust/Tauri path, packaged desktop MCP QA, or release readiness.
```

## 2026-06-22 - Go G5 Checkpoint/Rewind Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves checkpoint/rewind safety into G5
executable shadow calculations while keeping the TypeScript runtime checkpoint
oracle authoritative.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records
`controlExecutableCases.checkpointRewind`. The TypeScript conformance test
builds a real combined `CheckpointRewindPlan` from analytix checkpoint events
and path risks, proving ready/blocked file counts, path escape blocking,
symlink blocking, legal `..name` handling, and append-only conversation audit.
The Go shadow package computes the same output from fixture data, including
`axcp_` / `axrp_` / `axra_` / `axrr_` id-prefix boundaries, explicit
`APPLY_CHECKPOINT_REWIND` confirmation, no transcript rewrite, no git refs, no
Reasonix protocol, and no top-level route exposure.

Boundary:

```text
This batch does not implement a Go checkpoint store, Go checkpoint HTTP route,
live Go file mutation, renderer-visible Go route, default Go backend, Electron
integration, Reasonix checkpoint/rewind protocol, Kun git-ref checkpoint
behavior, Rust/Tauri path, packaged desktop rewind QA, or release readiness.
```

## 2026-06-22 - Go G5 Remote-Entry Boundary Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves the remote-entry control-port
boundary into G5 executable shadow calculations while keeping TypeScript
runtime control ports authoritative.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records
`controlExecutableCases.remoteEntry`. The TypeScript conformance test derives
the case from `approval-user-input-route-oracle.json`, which already proves
remote/bot-like entries expose only `approvals`, `lifecycle`, and `turns`,
reject approval-policy overrides, and omit goal, checkpoint, memory, session,
thread-store, thread-service, and tool-host control planes. The Go shadow
package computes the same narrow surface, including no Reasonix protocol and no
top-level route exposure.

Boundary:

```text
This batch does not implement a Go remote-entry HTTP route, live Go turn
control, renderer-visible Go route, default Go backend, Electron integration,
Reasonix SessionAPI/control protocol, Rust/Tauri path, packaged desktop
remote-entry QA, or release readiness.
```

## 2026-06-22 - Go G5 Resume Pending Gates Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves the session-resume pending
approval/user-input gate boundary into G5 executable shadow calculations while
keeping TypeScript runtime session resume authoritative.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records
`controlExecutableCases.resumePendingGates`. The TypeScript conformance test
derives the case from `approval-user-input-route-oracle.json`, where the source
thread keeps pending approval/user-input items while the resumed thread receives
expired/cancelled copies. The Go shadow package computes the same statuses,
`pendingAfterResume:0`, `answersCopiedToResume:false`, no Reasonix protocol,
and no top-level route exposure.

Boundary:

```text
This batch does not implement a Go session-resume route, live Go approval or
user-input gate manager, renderer-visible Go route, default Go backend,
Electron integration, Reasonix ask/session protocol, Rust/Tauri path, packaged
desktop resume QA, or release readiness.
```

## 2026-06-22 - D-0063 Go G5 Provider Cache Release Guard Executable Shadow

Status:
Go G5 remains shadow-only. This batch moves the offline provider cache release
guard into G5 executable shadow calculations while keeping TypeScript provider
clients and cache guard logic authoritative.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g5-full-loop-oracle.json` now records `cacheReplay.releaseGuard`. The
TypeScript conformance test derives that output from
`evaluateOfflineCacheCurveGuard(providerCache.releaseGuard)`, and the Go shadow
package computes the same fixture-only release guard output: tail averages,
case statuses, collapse counts, low-tail allowance, threshold/window metadata,
and overall pass.

Boundary:

```text
This batch does not implement a Go provider client, live provider/cache
superiority proof, credentialed provider matrix, renderer-visible Go route,
default Go backend, Electron integration, Reasonix provider protocol,
Rust/Tauri path, packaged desktop provider QA, or release readiness.
```

## 2026-06-22 - D-0071 Auto-Router Request Contract Fingerprint Currentness

Status:
No Go runtime code changed. This batch strengthens the TypeScript auto-router
currentness oracle that future Go router work would need to match before any
G5/G6 backend decision.

Evidence:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`buildAutoModelRouterFingerprint` now exposes explicit request-contract inputs
while preserving the default `AUTO_MODEL_ROUTER_FINGERPRINT`. The TypeScript
proof verifies that max-token, temperature, and reasoning-effort drift changes
the route-cache fingerprint, adapting Reasonix rebuild-on-enable currentness
without adopting Reasonix public auto-plan settings.

Boundary:

```text
This batch does not implement a Go router, Go provider client, live auto-plan
toggle, renderer-visible Go route, default Go backend, Electron integration,
Reasonix config/controller protocol, Rust/Tauri path, packaged desktop
controller QA, or release readiness.
```

## 2026-06-22 - D-0072 Connect Phone Copy Sovereignty

Status:
No Go runtime code changed. This batch is a desktop/runtime prompt and renderer
compatibility copy correction.

Evidence:

```text
npm run test -- src/shared/app-settings.test.ts src/main/schedule-runtime.test.ts src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts src/renderer/src/store/chat-store-helpers.test.ts src/renderer/src/store/chat-store-claw-actions.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

New Connect Phone prompt headings, incoming thread titles, schedule schema
descriptions, and webhook log messages use Connect Phone copy. Legacy Claw
heading/title recognition remains compatibility-only for old sessions.

Boundary:

```text
This batch does not implement Go routing, Go Connect Phone handlers,
renderer-visible Go routes, default Go backend, Electron integration,
Reasonix/Kun public protocol, Rust/Tauri path, packaged desktop QA, or release
readiness.
```

## 2026-06-22 - D-0073 Durable Runner Restart Executable Shadow

Status:
Go G5 remains shadow-only. This batch adds a pure Go executable replay for the
TypeScript-owned durable task-job restart drill.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.taskJobs.restartDrill` now records a running and queued
job after restart. TypeScript conformance derives the case from
`task-job-orchestration-oracle.json`, and Go shadow computes rehydrated count,
combined output/`nextOffset: 2`, and queued kill status/error.

Boundary:

```text
This batch does not implement a live Go Job Manager, Go task-job routes,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
SessionAPI/public sub-agent protocol, Rust/Tauri path, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0070 Custom Chat Full Endpoint Request Shape

Status:
No Go runtime code changed. This batch extends the TypeScript provider/cache
oracle that future Go provider work must eventually match, and G3/G5 shadow
fixtures now replay the additional request-shape id.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`provider-cache-oracle.json` now includes
`custom-chat-full-endpoint-request-shape`. The TypeScript proof verifies that
`custom_endpoint` with a `/chat/completions` URL keeps the exact URL and uses
OpenAI-compatible chat headers/body/tool shape instead of Responses or
Anthropic Messages fields. G3/G5 oracle fixtures include the same id for
shadow-only replay.

Boundary:

```text
This batch does not implement a Go provider client, live provider/cache
superiority proof, credentialed provider matrix, renderer-visible Go route,
default Go backend, Electron integration, Reasonix provider protocol,
Rust/Tauri path, packaged desktop provider QA, or release readiness.
```

## 2026-06-22 - D-0066 Auto-Router Classifier Request Contract

Status:
No Go runtime code changed. This batch is TypeScript auto-router contract
coverage and documentation only.

Evidence:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`auto-model-router.test.ts` now proves the TS `_auto_router` classifier request
is an isolated short-JSON side path with no tools, no immutable prefix, no
main-turn context instructions, deterministic model/sampling controls, and
timeout fallback to heuristic concrete model/reasoning. Timeout drift also
changes the classifier fingerprint.

Boundary:

```text
This batch does not implement a Go auto-router, Go classifier client, live Go
turn control, renderer-visible Go route, default Go backend, Electron
integration, Reasonix auto-plan/controller protocol, Rust/Tauri path, packaged
desktop model-routing QA, or release readiness.
```

## 2026-06-22 - D-0067 Planner-Executor Transcript Propagation

Status:
Go G5 remains shadow-only. This batch extends TypeScript planner-executor
runtime behavior and mirrors the requirement in G5 shadow output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`runPlannerExecutorCoordinator` now accepts optional `transcriptFor(task)` and
persists the returned `TaskJobTranscriptRef` on executor jobs. The TypeScript
oracle creates a fork ref through `resolveTranscriptOperation`, proves both
actual executor jobs carry it, and keeps skipped dependency work from creating
a job. G5 fixture/schema and Go shadow output replay
`requiresTranscriptPropagation` and `transcriptPropagationMode`.

Boundary:

```text
This batch does not implement a live Go Job Manager, Go task route, renderer-
visible Go route, default Go backend, Electron integration, Reasonix
Subagent/SessionAPI/transcript protocol, top-level Subagent/Workflow route,
Rust/Tauri path, packaged desktop sub-agent QA, or release readiness.
```

## 2026-06-22 - D-0068 MCP Search Meta-Tool Trust Boundary

Status:
Go G4 remains shadow-only. This batch extends TypeScript MCP search-mode
runtime coverage and mirrors the requirement in G4 shadow output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-provider.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`mcp-tool-provider.test.ts` now proves MCP search meta-tools are advertised only
as internal tools, trusted workspace search/describe/call works, untrusted
workspace search sees 0 tools, untrusted `mcp_call` returns an unknown-tool
error without client execution, and denied `mcp_call` returns an approval item
before execution. G4 fixture/schema and Go shadow output replay the
advertised/trust/no-execute evidence.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route, renderer-
visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path, packaged
desktop MCP QA, credentialed MCP matrix, or release readiness.
```

## 2026-06-22 - D-0069 Custom Messages Full Endpoint Request Shape

Status:
No Go runtime code changed. This batch extends the TypeScript provider/cache
oracle that Go provider work must eventually match.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`provider-cache-oracle.json` now includes
`custom-messages-full-endpoint-request-shape`. The TypeScript proof verifies
that `custom_endpoint` with a `/messages` URL keeps the exact URL and uses
Anthropic Messages headers/body/tool shape instead of OpenAI chat/responses
fields.

Boundary:

```text
This batch does not implement a Go provider client, live provider/cache
superiority proof, credentialed provider matrix, renderer-visible Go route,
default Go backend, Electron integration, Reasonix provider protocol,
Rust/Tauri path, packaged desktop provider QA, or release readiness.
```

## 2026-06-22 - D-0074 Approval/User-Input Abort Cleanup Executable Shadow

Status:
Go G5 remains shadow-only. This batch adds a pure Go executable replay for the
TypeScript-owned approval/user-input abort-cleanup route oracle.

Evidence:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.abortCleanup` now records `appr_abort_cleanup_1` and
`input_abort_cleanup_1`. TypeScript conformance derives the case from
`approval-user-input-route-oracle.json`, and Go shadow computes approval
`expired`, user-input `cancelled`, late approval decision `409`, late user-input
resolve `404`, `pendingAfterCleanup: 0`, replay event kinds, and product
boundary booleans.

Boundary:

```text
This batch does not implement a live Go approval manager, live Go user-input
manager, Go task-job routes, renderer-visible Go route, default Go backend,
Electron integration, Reasonix SessionAPI/public ask protocol, Rust/Tauri path,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0075 Abort Cleanup Replay Summary Closure

Status:
Go G5 remains shadow-only. This batch closes the replay-summary side of the
D-0074 abort-cleanup proof.

Evidence:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlReplay.abortCleanup` and `shadowSlicesExpectedOutput.approvalUserInputReplay`
now carry expired approval, cancelled user-input, late approval decision `409`,
late user-input resolve `404`, no pending gates after cleanup, and replay event
kinds. Go shadow computes the `approvalUserInputReplay` fields from
`approval-user-input-route-oracle.json`.

Boundary:

```text
This batch does not implement a live Go approval manager, live Go user-input
manager, renderer-visible Go route, default Go backend, Electron integration,
Reasonix SessionAPI/public ask protocol, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0076 MCP Search Meta-Tool Replay

Status:
Go G5 remains shadow-only. This batch carries MCP search meta-tool trust and
denied no-execute evidence into the G5 `mcpReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay` now includes meta-tool names, trusted
tool id, unknown-tool error, untrusted searched-tool count `0`, `on-request`
call policy, and denied no-execute. Go shadow computes these fields from
`mcp-tool-lifecycle-oracle.json`.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0189 Runtime HTTP Route Sovereignty Shadow

Status:
Go G5 remains shadow-only. This batch adds source-derived runtime HTTP/SSE
route sovereignty to the G5 executable oracle; it does not implement or expose
a live Go HTTP server.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.runtimeHttpRouteSovereignty` now records the 44-route
`analytix serve` HTTP table, `/health` unauthenticated exception, authenticated
route count, `/v1/threads/:id/events` SSE route, thread lifecycle routes,
approval/user-input routes, internal task-job routes, compatibility-only
singular user-input route, forbidden route token absence, and product-boundary
booleans. The TypeScript oracle derives those values from source, and
`BuildG5ControlExecutableOutput` computes the same expected output.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated route identity, add
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public routes, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, live Go HTTP server, packaged route QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0190 Runtime HTTP Auth Matrix Proof

Status:
Go G5 remains shadow-only. This batch adds actual TypeScript HTTP dispatch
evidence that every route in the D-0189 G5 route-sovereignty fixture requires
auth except `/health`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`go-runtime-conformance.test.ts` now dispatches each
`controlExecutableCases.runtimeHttpRouteSovereignty.routes` entry without an
authorization header. `/health` returns 200, and all 43 `/v1/*` routes return
structured 401 `{ code: "unauthorized", message: "unauthorized" }`, including
SSE, task-job, approval, user-input, and resume-thread routes.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/public route protocol, add Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer routes, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path, live
Go HTTP server, packaged route QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0193 Renderer Runtime Endpoint Builder Proof

Status:
Go G5 remains shadow-only. This batch adds renderer provider evidence that GUI
runtime paths remain tied to shared analytix endpoint constants/builders.

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

Result:

`AnalytixRuntimeProvider` uses shared health/thread root constants and existing
shared dynamic-route builders. Renderer tests prove slash/query/fragment text
in thread, turn, approval, user-input, and session ids is encoded before the
bridge `runtimeRequest` call, while requests remain analytix-owned paths.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/public route protocol, add Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer routes, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path, live
Go HTTP server, packaged route QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0192 Shared Endpoint Builder Sovereignty Proof

Status:
Go G5 remains shadow-only. This batch adds shared endpoint builder evidence
that future Go route work must preserve before any G6 backend selection.

Evidence:

```text
npm run test -- src/shared/analytix-endpoints.test.ts
```

Result:

`src/shared/analytix-endpoints.test.ts` proves shared path builders URL-encode
ids for thread, turn, checkpoint, approval, user-input, session, attachment,
and memory routes. Exported shared endpoint strings remain analytix-owned,
canonical user-input stays plural `/v1/user-inputs/{id}`, and forbidden
Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer/session-api tokens are absent.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/public route protocol, add Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer routes, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path, live
Go HTTP server, packaged route QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0191 Runtime Forbidden Route Dispatch Proof

Status:
Go G5 remains shadow-only. This batch adds actual TypeScript HTTP dispatch
evidence that forbidden route tokens in the D-0189 G5 route-sovereignty fixture
are not registered routes.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`go-runtime-conformance.test.ts` now dispatches each
`controlExecutableCases.runtimeHttpRouteSovereignty.forbiddenRouteTokens` entry
with valid auth through the real TypeScript router. Every forbidden token
returns structured 404 `{ code: "not_found", message: "route not found" }`.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/public route protocol, add Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer routes, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path, live
Go HTTP server, packaged route QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0155 Browser Bridge / Visible Entry Sovereignty Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch strengthens product
sovereignty evidence around the browser preview bridge, visible sidebar entry
surface, and scan path freshness.

Evidence:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Result:

`browser-analytix-bridge.ts` is now covered as an analytix-owned bridge surface
in route-surface tests and scan freshness. The rendered sidebar rejects
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer labels.

Boundary:

```text
This batch does not implement live Go routes, a live Go bridge, default Go
backend, renderer-visible Go route, Reasonix SessionAPI/public protocol,
Electron integration, Rust/Tauri path, packaged desktop QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0156 Browser Preview SSE Bridge Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds executable browser
preview SSE bridge evidence in renderer tests only.

Evidence:

```text
npm run test -- src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Browser preview `window.analytix.runtime.startSse` now has executable evidence
for analytix proxy URL construction, `Last-Event-ID`, and SSE
`id`/`event`/`data` normalization.

Boundary:

```text
This batch does not implement live Go routes, a live Go bridge, default Go
backend, renderer-visible Go route, Reasonix SessionAPI/public protocol,
Electron integration, Rust/Tauri path, packaged browser/desktop QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0157 Browser Preview Settings Sovereignty Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds shared/browser-preview
settings normalization evidence only.

Evidence:

```text
npm run test -- src/shared/app-settings.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Browser preview settings now have executable evidence that legacy/Reasonix
agent envelopes are stripped on load and re-save while valid top-level
`runtime.model` and `runtime.endpointFormat` persist.

Boundary:

```text
This batch does not implement live Go settings handling, a live Go bridge,
default Go backend, renderer-visible Go route, Reasonix config/auto-plan
protocol, Electron integration, Rust/Tauri path, packaged settings QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0158 IPC Settings Patch Sovereignty Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds desktop IPC settings
patch sanitization evidence only.

Evidence:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Renderer-to-main settings patch validation now strips top-level legacy/Reasonix
agent envelopes before strict validation while keeping legal analytix settings
patch fields.

Boundary:

```text
This batch does not implement live Go settings handling, a live Go bridge,
default Go backend, renderer-visible Go route, Reasonix config/auto-plan
protocol, Electron integration, Rust/Tauri path, packaged settings QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0160 Workflow Singular Route Negative Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch updates the live
TypeScript HTTP negative-route test so it covers the singular `/v1/workflow`
path already replayed by Go G4/G5 forbidden-route fixtures.

Evidence:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The TypeScript runtime router returns structured 404 for `/v1/workflow`
alongside `/v1/workflows`, `/v1/create-loop`, `/v1/subagents`,
`/v1/autoresearch`, `/v1/mcp-indexer`, Reasonix public routes, and
renderer-visible Go route probes.

Boundary:

```text
This batch does not implement live Go HTTP routes, renderer-visible Go routes,
default Go backend, Reasonix public protocol, Workflow/Create Loop public
routes, Electron integration, Rust/Tauri path, packaged route QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0161 Runtime Proof Freshness Scan Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch updates product-sovereignty
scan freshness and the top-level conformance-plan currentness table; it does
not add Go behavior.

Evidence:

```text
npm run scan:product-sovereignty
```

Result:

`scan:product-sovereignty` now requires provider-cache, approval/user-input,
G2/G3/G4/G5 conformance fixtures/tests, and Go shadow source paths to remain
present. It also scans active runtime routes, shared endpoint templates, IPC
schema, and renderer runtime client source for forbidden public route/protocol
strings. The conformance table now records G2 exact route replay as closed and
G3/G4/G5 provider/cache plus approval/user-input evidence as active shadow
oracle input.

Boundary:

```text
This batch does not implement live Go HTTP routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public
protocol, Workflow/Create Loop public routes, Electron integration, Rust/Tauri
path, packaged route/provider QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0163 Renderer SSE Bridge Cursor Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds renderer SSE
bridge/cursor evidence only.

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`AnalytixRuntimeProvider.subscribeThreadEvents` starts SSE through
`window.analytix.runtime.startSse(threadId, sinceSeq, streamId)`, keeps event
dispatch and cleanup tied to the same generated stream id, and does not use
`runtimeRequest` as a renderer-side SSE path.

Boundary:

```text
This batch does not implement live Go HTTP/SSE routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public SSE
protocol, Workflow/Create Loop public routes, Electron integration, Rust/Tauri
path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0165 Main Runtime Request Forbidden Route Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds TypeScript main IPC
runtime request schema/handler proof only.

Evidence:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/main/ipc/register-app-ipc-handlers.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Main IPC runtime requests explicitly reject Reasonix public routes,
renderer-visible Go routes, and Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route families before calling the runtime request adapter.

Boundary:

```text
This batch does not implement live Go HTTP routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public
runtime protocol, Workflow/Create Loop public routes, Electron integration,
Rust/Tauri path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0169 G5 Provider Cache Coverage Floor Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes the provider/cache coverage
floor into executable control output without adding a live Go provider client.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`go-g5-full-loop-oracle.json` now includes
`controlExecutableCases.providerCacheCoverageFloor`. TypeScript conformance
derives the control case from the provider-cache oracle, while Go shadow
computes required and covered provider families, usage and request-shape ids,
endpoint formats, telemetry-supported cases, unsupported unknown cache
behavior, custom full endpoint exact-URL/tool-shape evidence, DeepSeek/OpenAI/
Anthropic splits, no live credentials, no live superiority claim, no Reasonix
protocol, and no top-level route exposure.

Boundary:

```text
This batch does not implement a live Go provider client, live Go provider
matrix, default Go backend, renderer-visible Go route, Electron integration,
Reasonix provider protocol, Rust/Tauri path, packaged provider settings QA, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0168 Provider Cache Coverage Floor Has No Go Runtime Delta

Status:
Go G3/G5 remain shadow-only and unchanged. This batch tightens the TypeScript
provider-cache oracle schema and focused tests so future Go provider/cache work
cannot silently lose the required provider family matrix.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`ProviderLiveLocalHttpProof.coveredProviderFamilies` now accepts only
DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and
custom full endpoint families. `provider-cache-proof.test.ts` derives that
coverage from the oracle cases and pins required usage ids, request-shape ids,
endpoint formats, telemetry-supported cases, unsupported unknown cache
behavior, and custom `/responses`, `/messages`, and `/chat/completions` full
endpoint cases.

Boundary:

```text
This batch does not implement a live Go provider client, live Go provider
matrix, default Go backend, renderer-visible Go route, Electron integration,
Reasonix provider protocol, Rust/Tauri path, packaged provider settings QA, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0167 Runtime Desktop Bridge Proof Freshness Scan Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds product-sovereignty
scan freshness for TypeScript desktop runtime bridge proof paths only.

Evidence:

```text
npm run scan:product-sovereignty
```

Result:

`scan:product-sovereignty` now requires renderer provider/client, browser
preview bridge, preload bridge/types, main IPC schema/handler, main SSE IPC,
shared API/endpoints, and focused bridge proof tests to remain present.

Boundary:

```text
This batch does not implement live Go HTTP/SSE routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public
runtime protocol, Workflow/Create Loop public routes, Electron integration,
Rust/Tauri path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0166 Preload Runtime Request IPC Bridge Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds TypeScript preload
runtime request bridge proof only.

Evidence:

```text
npm run test -- src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Preload maps `window.analytix.runtime.runtimeRequest(path, method, body)` only
to `runtime:request` IPC with `{ path, method, body }`, without Reasonix/Kun/
Go/Workflow runtime request IPC channel names.

Boundary:

```text
This batch does not implement live Go HTTP routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public
runtime protocol, Workflow/Create Loop public routes, Electron integration,
Rust/Tauri path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0164 Main/Preload SSE IPC Bridge Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds TypeScript desktop
preload/main SSE IPC proof only.

Evidence:

```text
npm run test -- src/main/runtime-sse-ipc.test.ts src/main/ipc/app-ipc-schemas.test.ts src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The desktop SSE chain proves preload `runtime:sse:*` IPC mapping, strict
`threadId`/`sinceSeq`/`streamId` start schema, analytix-owned
`/v1/threads/:id/events` fetch path with initial cursor headers, generated
stream id error payloads, and exact `stopSse` stream id matching.

Boundary:

```text
This batch does not implement live Go HTTP/SSE routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public SSE
protocol, Workflow/Create Loop public routes, Electron integration, Rust/Tauri
path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0162 Renderer Runtime Request Surface Proof Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch adds renderer provider
request-path evidence only.

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`AnalytixRuntimeProvider` calls for common runtime flows stay on `/health` or
analytix-owned `/v1/*` HTTP paths and reject Reasonix public routes,
renderer-visible Go routes, and Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer public surfaces.

Boundary:

```text
This batch does not implement live Go HTTP routes, live Go provider/client
behavior, renderer-visible Go routes, default Go backend, Reasonix public
protocol, Workflow/Create Loop public routes, Electron integration, Rust/Tauri
path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0159 Settings Sovereignty Scan Freshness Has No Go Delta

Status:
Go G5 remains shadow-only and unchanged. This batch only extends the
product-sovereignty source scan path freshness for settings and bridge
chokepoints.

Evidence:

```text
npm run scan:product-sovereignty
```

Result:

`scan:product-sovereignty` now requires shared settings normalization, active
runtime settings, settings-store persistence, IPC settings patch schema,
preload, browser preview bridge, and their focused guard tests to remain
present.

Boundary:

```text
This batch does not implement live Go settings handling, a live Go bridge,
default Go backend, renderer-visible Go route, Reasonix config/auto-plan
protocol, Electron integration, Rust/Tauri path, packaged settings QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0153 Auto-Router Recommendation/Currentness Control Shadow

Status:
Go G5 remains shadow-only. This batch expands
`controlExecutableCases.autoRouterClassifier` with recommendation parsing and
recent-context currentness fields derived from TS-owned auto-router helpers.

Evidence:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes accepted/rejected classifier
recommendation counts, `pro/max` acceptance, `model:"auto"` and malformed
recommendation rejection, active-turn exclusion from recent context, and
historical tool-result summary preservation.

Boundary:

```text
This batch does not implement a live Go auto-router, Reasonix controller/
session protocol, public auto-plan setting, renderer-visible Go route, default
Go backend, Electron integration, Rust/Tauri path, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0154 Exact Session Route Replay Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes thread/session route evidence
from status/inventory summaries into `controlExecutableCases.sessionRouteReplay`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes an 11-row exact route replay
matrix with method/path/auth/status/body-hash/SSE-hash fields, exact JSON and
SSE route counts, runtime-token coverage, unauthorized route ids, and key
archive/search/fork/resume/SSE hashes from TS-owned G2 fixtures.

Boundary:

```text
This batch does not implement a live Go HTTP server, Reasonix SessionAPI/public
route protocol, renderer-visible Go route, default Go backend, Electron
integration, Rust/Tauri path, packaged desktop route QA, or release readiness.
```

## 2026-06-22 - D-0149 Context Compaction Boundary Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes latest-compaction
effective-history boundary behavior into
`controlExecutableCases.compactionBoundary`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes effective ids, dropped ids,
latest compaction id, latest-first status, noop/older compaction dropping,
post-compaction user preservation, pre-boundary user dropping, and product/cache
boundary flags from the TS-owned fixture.

Boundary:

```text
This batch does not implement a live Go history manager, live Go thread/session
routes, renderer-visible Go routes, default Go backend, Reasonix
SessionAPI/controller protocol, Electron integration, Rust/Tauri path,
packaged desktop long-history QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0150 User-Input Structured Validation Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes structured `request_user_input`
validation into `controlExecutableCases.userInput.structuredChoiceValidation`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes max questions, option bounds,
case-insensitive duplicate-label behavior, invalid result code, invalid case
count, the four reject booleans, invalid-no-gate status, and product-boundary
flags from the TS-owned G4/G5 fixtures.

Boundary:

```text
This batch does not implement a live Go approval/user-input manager,
renderer-visible Go routes, default Go backend, Reasonix ask/session protocol,
Electron integration, Rust/Tauri path, packaged desktop approval/user-input QA,
G6 readiness, or release readiness.
```

## 2026-06-22 - D-0151 Planner Gate Matrix Control Shadow

Status:
Go G5 remains shadow-only. This batch expands `controlExecutableCases.planner`
from a single forged task row into a typed planner gate matrix.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes normal agent `create_plan`
hiding, Plan capability advertisement, model step-0 read-only + `create_plan`
advertisement, later-step `create_plan` narrowing, rejected tool names/count,
and all-forged-calls-rejected status for `task`, `parallel_tasks`, `bash`,
`edit`, `write`, and `echo`.

Boundary:

```text
This batch does not implement a live Go planner/executor, live Go Job Manager,
Reasonix planner/session/task protocol, public auto-plan setting, renderer-
visible Go route, default Go backend, Electron integration, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, packaged
desktop Plan QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0152 Desktop Bridge/Settings Sovereignty Control Shadow

Status:
Go G5 remains shadow-only. This batch adds
`controlExecutableCases.desktopSovereignty` to bind desktop bridge and settings
schema proof into the G5 executable gate.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes single `analytix` bridge
exposure, `Window.analytix` ownership, analytix-owned facade domains,
Reasonix auto-plan drop proof, legacy agent envelope drop proof, top-level
runtime endpoint-format persistence proof, no deprecated bridge alias, no
deprecated settings fallback write, no Reasonix protocol, and no top-level
route exposure.

Boundary:

```text
This batch does not implement live Go desktop integration, a Go settings store,
Reasonix public protocol/config root, deprecated bridge/settings fallback,
renderer-visible Go route, default Go backend, Electron integration, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path,
packaged desktop settings QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0126 Approval/User-Input Route Replay Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes approval/user-input route replay
from `shadowSlicesExpectedOutput.approvalUserInputReplay` into
`controlExecutableCases.approvalUserInputRouteReplay`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes approval deny route body/status,
second-decision `409`, replay event order, submitted user-input answer echo,
resolved-event answer omission, cancelled user-input `404` late resolve,
abort-cleanup late action statuses, and no-pending-gates state from the
TS-owned approval/user-input route oracle.

Boundary:

```text
This batch does not implement a live Go approval/user-input manager,
renderer-visible Go route, default Go backend, Reasonix SessionAPI/ask
protocol, Electron integration, Rust/Tauri path, packaged desktop
approval-card QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0127 MCP Search Refresh Drift Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP search refresh drift from
`shadowSlicesExpectedOutput.mcpReplay.searchRefreshDrift` into
`controlExecutableCases.mcpSearchRefreshDrift`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes MCP refresh-drift server id,
initial tool names, expanded tool names, indexed count, catalog-drift boolean,
and no-top-level-route state from TS-owned MCP lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0128 MCP Approval Annotation Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP approval annotation evidence
from `shadowSlicesExpectedOutput.mcpReplay.approvalAnnotations` into
`controlExecutableCases.mcpApprovalAnnotations`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes MCP server id, tool name,
normalized tool name, destructive/open-world hints, approval id, deny decision,
result kind, execution flag, and denied-no-execute state from TS-owned MCP
lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, live Go approval manager,
Go MCP route, renderer-visible Go route, default Go backend, Electron
integration, Reasonix MCP-indexer/approval protocol, top-level MCP-indexer
route, Rust/Tauri path, credentialed MCP matrix, packaged desktop MCP/approval
QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0129 MCP Search Workspace Boundary Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP search workspace trust
boundary evidence from `shadowSlicesExpectedOutput.mcpReplay.searchWorkspaceBoundary`
into `controlExecutableCases.mcpSearchWorkspaceBoundary`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes trusted workspace, untrusted
workspace, query, trusted tool id, untrusted searched-tools count, unknown-tool
error, call policy, and denied-no-execute state from TS-owned MCP lifecycle
oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, live Go approval manager,
Go MCP route, renderer-visible Go route, default Go backend, Electron
integration, Reasonix MCP-indexer protocol, top-level MCP-indexer route,
Rust/Tauri path, credentialed MCP matrix, packaged desktop MCP QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0130 MCP Core Lifecycle Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP core lifecycle evidence from
`shadowSlicesExpectedOutput.mcpReplay.lifecycle` into
`controlExecutableCases.mcpCoreLifecycle`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes connect tool names/diagnostics,
disconnect reason/tool diagnostics, reload tool names and schema-order
stability, cancel-before-start no-execute state, and approved error shape from
TS-owned MCP lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0133 Task-Job Tool Contract Boundary Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes task/parallel task tool contract
and route-boundary evidence from `shadowSlicesExpectedOutput.jobReplay` into
`controlExecutableCases.taskJobs.toolContractBoundary`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes `task`/`parallel_tasks` tool
names, internal-runtime-only flags, permission/evidence/dependency/read-only
gates, runtime task-job routes, protected route auth, unauthorized status,
forbidden top-level routes, no Reasonix protocol, and no top-level route
exposure from TS-owned task-job oracle inputs.

Boundary:

```text
This batch does not implement a live Go Job Manager, live Go task-job routes,
renderer-visible Go route, default Go backend, public Reasonix sub-agent/job
protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
route, G6 readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0135 Nested Child SSE Metadata Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes nested child SSE metadata
evidence from `shadowSlicesExpectedOutput.jobReplay` into
`controlExecutableCases.taskJobs.nestedSseMetadata`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes parent call id, child run id,
nested SSE metadata fields, evidence ledger metadata key coverage, parent/child
distinctness, active-goal requirement, no Reasonix protocol, and no top-level
route exposure from TS-owned task-job oracle inputs.

Boundary:

```text
This batch does not implement a live Go Job Manager, live Go task-job routes,
renderer-visible Go route, default Go backend, public Reasonix SessionAPI/
sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route, G6 readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0134 Task Transcript Identity Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes transcript continue/fork identity
evidence from `shadowSlicesExpectedOutput.jobReplay` into
`controlExecutableCases.taskJobs.transcriptIdentity`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes source id, continue target id,
fork target id, incompatible identity error, same-transcript identity
requirement, continue-preserves-target, fork-creates-distinct-target,
continue-target-matches-source, fork-target-distinct-from-source, no Reasonix
protocol, and no top-level route exposure from TS-owned task-job oracle inputs.

Boundary:

```text
This batch does not implement a live Go Job Manager, live Go task-job routes,
renderer-visible Go route, default Go backend, public Reasonix SessionAPI/
sub-agent/job protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route, G6 readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0132 Product Boundary Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes global product-boundary evidence
from G5 metadata into `controlExecutableCases.productBoundary`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes bridge/serve stability,
Reasonix public protocol disabled, default Go backend disabled, renderer-visible
Go route disabled, Electron main disconnected, and default Go backend not
enabled from TS-owned G5 oracle inputs.

Boundary:

```text
This batch does not implement a live Go backend, renderer-visible Go route,
default Go backend, Electron integration, Reasonix public protocol, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path,
G6 readiness, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0131 MCP Background Reconnect Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP background reconnect
evidence from `shadowSlicesExpectedOutput.mcpReplay.backgroundReconnect` into
`controlExecutableCases.mcpBackgroundReconnect`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes failed server ids, suspended
provider id/reason, connected/error server outcomes, attempts per failed
server, retry-all-failed coverage, and no-runtime-restart state from TS-owned
MCP lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0136 MCP Known Override Diagnostics Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP known override diagnostics
from `shadowSlicesExpectedOutput.mcpReplay.knownOverrideDiagnostics` into
`controlExecutableCases.mcpKnownOverrideDiagnostics`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes known override diagnostic rows,
variant count, override kinds, workspace roots, explicit-cwd server ids,
daemon-timeout server ids, all-low-priority/all-background-start booleans, and
product-boundary flags from TS-owned MCP lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0137 MCP Live-Local Indexer Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP live-local indexer lifecycle
evidence from `shadowSlicesExpectedOutput.mcpReplay.liveLocalIndexer` into
`controlExecutableCases.mcpLiveLocalIndexer`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes live-local indexer server/cwd,
retry server ids, attempts, retry map, initial/resume/active paths, tombstone
count, snapshot restart, late tombstone, secret-safe diagnostic,
execution-error redaction, and product-boundary flags from TS-owned MCP
lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0138 MCP Search Meta-Tool Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes MCP search meta-tool
advertisement and trust/no-execute evidence from
`shadowSlicesExpectedOutput.mcpReplay.searchMetaToolNames` and related summary
fields into `controlExecutableCases.mcpSearchMetaTools`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes search meta-tool names/count,
refresh tool advertisement, trusted/untrusted workspace fields, trusted tool
id, query, unknown-tool error, untrusted searched count, `on-request` call
policy, denied no-execute, and product-boundary flags from TS-owned MCP
lifecycle oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0139 Provider Cache Privacy Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider cache diagnostics
privacy and live-superiority policy from
`shadowSlicesExpectedOutput.cacheReplay.providerCachePrivacy` into
`controlExecutableCases.providerCachePrivacy`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes diagnostics field count,
forbidden diagnostics substring count, no forbidden diagnostics leakage,
fixture-only policy, no live credential use, no live superiority claim, and
product-boundary flags from TS-owned provider/cache oracle inputs.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Electron
integration, Reasonix provider protocol, credentialed provider matrix,
packaged provider QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0140 Provider Cache Inventory Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider cache prefix/tool/case
inventory from loose `cacheReplay` summary fields into
`controlExecutableCases.providerCacheInventory`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes stable prefix hash, tools hash,
provider usage case ids/count, request-shape case ids/count, prefix
equivalence, tools hash stability, and product-boundary flags from TS-owned
provider/cache oracle inputs.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Electron
integration, Reasonix provider protocol, dynamic stable-prefix material,
credentialed provider matrix, packaged provider QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0141 Session Route Inventory Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes thread/session route inventory
from the G2 route oracle into `controlExecutableCases.sessionRouteInventory`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes route id inventory, JSON/SSE
route groupings, event/resume/fork/archive/search/read-update groupings,
runtime-token protected route count, unauthorized route ids, and
product-boundary flags from the TS-owned G2 route oracle.

Boundary:

```text
This batch does not implement live Go thread/session routes, renderer-visible
Go routes, default Go backend, Electron integration, Reasonix SessionAPI or
public route protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer navigation, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0142 Approval/User-Input Inventory Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes approval/user-input gate
inventory from the approval/user-input route oracle into
`controlExecutableCases.approvalUserInputInventory`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes gate ids, approval ids,
user-input ids, route kind inventory, replay kind order, abort replay kind
order, answer count, HTTP/SSE answer privacy flags, late action statuses,
pending-after sum, and product-boundary flags from TS-owned approval/user-input
oracle inputs.

Boundary:

```text
This batch does not implement a live Go approval/user-input manager,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
ask/session public protocol, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation, packaged approval-card QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0143 Task Planner Toolset Inventory Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes task/sub-agent planner toolset
inventory from the task-job oracle into
`controlExecutableCases.taskJobs.plannerToolsetInventory`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes planner read-only tools,
forbidden task tools, tool counts, read-only exclusion of task tools, forbidden
toolset match against `task` / `parallel_tasks`, planner/executor policy, and
product-boundary flags from TS-owned task-job oracle inputs.

Boundary:

```text
This batch does not implement a live Go planner/executor or Job Manager,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
public sub-agent/job/planner protocol, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer navigation, packaged sub-agent QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0144 MCP Core Lifecycle Boundary Seal Control Shadow

Status:
Go G5 remains shadow-only. This batch seals MCP core lifecycle provider identity
and product-boundary flags inside `controlExecutableCases.mcpCoreLifecycle`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes `providerId: "mcp:research"`,
`usesReasonixProtocol: false`, and `topLevelRouteExposed: false` alongside the
existing MCP connect/disconnect/reload/cancel/error lifecycle output from
TS-owned MCP oracle inputs.

Boundary:

```text
This batch does not implement a live Go MCP client, renderer-visible Go route,
default Go backend, Electron integration, Reasonix MCP-indexer public protocol,
top-level MCP-indexer navigation, credentialed MCP matrix, packaged MCP QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0145 Provider Request-Shape Exact Matrix Control Shadow

Status:
Go G3/G5 remain shadow-only. This batch promotes provider request-shape proof
from summary counts into exact matrix output in G3/G5 shadow replay.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG3ProviderConformanceOutput`, `BuildG5ShadowSlicesOutput`, and
`BuildG5ControlExecutableOutput` now replay the 7-case provider request-shape
matrix, including exact URLs, required/forbidden headers, required/forbidden
body fields, reasoning-effort presence, and tool schema family.

Boundary:

```text
This batch does not implement a live Go provider client, live credentialed
provider matrix, renderer-visible Go route, default Go backend, Electron
integration, Reasonix provider protocol, packaged provider settings QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0146 Combined Step/Cancel/Cache Trace Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes combined auto-route cache,
step-limit, and cancel evidence from summary booleans into typed trace output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes same-turn router calls, main/max
model steps, next-turn reroute calls, stable-prefix isolation flags,
accepted/cancel result counts, completed/aborted counts, and exact cancel
results for the combined control case.

Boundary:

```text
This batch does not implement a live Go agent loop, renderer-visible Go route,
default Go backend, Electron integration, Reasonix controller/session protocol,
top-level Workflow/Create Loop/Subagent/AutoResearch navigation, packaged
cancel/cache QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0148 History Repair Pair-Integrity Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes model-history repair and
tool-call/result pair integrity into executable control shadow.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes `historyRepair` from simplified
TS-owned fixture items. It preserves complete multi-tool blocks and bridge
items, drops orphan results, drops missing-result calls, drops duplicate
results, and records that repair state is not stable-prefix material.

Boundary:

```text
This batch does not implement a live Go agent loop, live Go history manager,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
SessionAPI/controller protocol, public history protocol, packaged long-history
QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0147 Step-Limit Override/Delegate Matrix Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes step-limit evidence from scalar
fields into an override/delegate matrix.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes a 9-row step-limit matrix for
default, user-global, session, turn, planner, headless, zero-default,
delegate parent-half, and delegate min-floor cases. Each row records configured,
fallback, effective value, source, stable-prefix status, disable-guard status,
and delegate floor status.

Boundary:

```text
This batch does not implement a live Go agent loop, renderer-visible Go route,
default Go backend, Electron integration, Reasonix controller/session protocol,
public auto-plan setting, packaged step-limit QA, G6 readiness, or release
readiness.
```

Parallel research follow-up:

```text
Continue G5 executable shadows for context compaction, model-history repair,
tool-call/result pair integrity, checkpoint rewind/recovery, approval/user-input
route replay, MCP lifecycle replay, and provider request/cache matrices. Keep
all of them fed from TS-owned oracle fixtures until G5/G6 gates explicitly
allow a live Go backend.
```

## 2026-06-22 - D-0110 MCP Refresh Catalog Drift Replay

Status:
Go G5 remains shadow-only. This batch carries MCP search catalog refresh drift
evidence into the G5 `mcpReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay.searchRefreshDrift` now includes server
id, initial tool names, expanded tool names, total indexed count,
`catalogDrift: true`, and `topLevelRouteExposed: false`. Go shadow computes
these fields from `mcp-tool-lifecycle-oracle.json`.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer public lifecycle protocol, top-level MCP-indexer route, Rust/Tauri
path, credentialed MCP matrix, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0111 Thread/SSE Route Auth Replay

Status:
Go G5 remains shadow-only. This batch carries missing-token thread/SSE route
authorization evidence from the G2 replay oracle into G5 `sessionReplay`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g2-route-replay-oracle.json` now includes
`events-unauthorized-since-seq` with `auth: none` and a 401 JSON error.
`shadowSlicesExpectedOutput.sessionReplay.routeStatusReplay.auth` records the
protected route id/path/status/body code and zero SSE frames. Go shadow
computes the same summary from G2 routes.

Boundary:

```text
This batch does not implement live Go HTTP serving, renderer-visible Go routes,
default Go backend, Electron integration, Reasonix SessionAPI/thread protocol,
top-level hidden capability routes, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0113 Task-Job Route Executable Control Shadow

Status:
Go G5 remains shadow-only. This batch upgrades the existing task-job route
executable summary into a typed `controlExecutableCases.taskJobs` replay.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.taskJobs.routeExecutable` now mirrors
`task-job-orchestration-oracle.routeExecutable`, including unauthorized,
output, wait, kill, missing-output, and rehydrated route status fields.
`BuildG5ControlExecutableOutput` computes a typed route executable output from
those inputs and Go tests compare it to the TS-owned expected output.

Boundary:

```text
This batch does not implement live Go task-job routes, a live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch
route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0114 Task-Job Lifecycle Control Shadow

Status:
Go G5 remains shadow-only. This batch upgrades task-job foreground/background
lifecycle evidence from summary replay into typed control executable replay.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.taskJobs.lifecycle` now mirrors the TS-owned task-job
foreground, background, and wait/output/kill lifecycle oracle.
`BuildG5ControlExecutableOutput` computes the lifecycle output and Go tests
compare it to the TS-owned expected result.

Boundary:

```text
This batch does not implement live Go task-job routes, a live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch
route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0115 Planner-Executor Control Shadow

Status:
Go G5 remains shadow-only. This batch upgrades planner/executor failure,
cancellation, output-offset, and transcript-propagation evidence from summary
replay into typed control executable replay.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.taskJobs.plannerExecutor` now mirrors the TS-owned
planner/executor task-job oracle. `BuildG5ControlExecutableOutput` computes the
planner/executor output and Go tests compare it to the TS-owned expected
result.

Boundary:

```text
This batch does not implement live Go task-job routes, a live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch
route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0107 Plan Step Cancel/Cache Has No Go Runtime Delta

Scope:

```text
D-0107 adds deterministic TypeScript AgentLoop evidence for Plan mode
step-gating, cancellation, and cache baseline behavior. It does not change Go
shadow code, Go fixtures, Go routes, or backend selection.
```

Boundary:

| Area | Decision |
| --- | --- |
| Go G5/G6 | No new Go G5/G6 fixture is required for this slice; the authoritative behavior remains in TypeScript AgentLoop. |
| Default backend | Go is still not a default backend and no renderer-visible Go route is exposed. |
| Future absorption | If Go later executes Plan mode, this loop fixture is an input oracle for cancelled follow-up steps and cache baseline handling. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0108 MCP Stdio Error Redaction Has No Go Runtime Delta

Scope:

```text
D-0108 changes the TypeScript MCP provider and executable stdio MCP oracle.
It does not change Go shadow code, Go fixtures, Go routes, or backend
selection.
```

Boundary:

| Area | Decision |
| --- | --- |
| Go G5/G6 | No new Go executable field is required for this slice; the behavior is provider-layer redaction in TS runtime. |
| Default backend | Go remains shadow-only and is not a default backend. |
| Future absorption | If Go later executes MCP tools, this fixture becomes an input oracle for protocol-level `isError` result redaction. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0109 Workflow/Create Loop Quarantine Has No Go Runtime Delta

Scope:

```text
D-0109 strengthens product-sovereignty source scans around dormant renderer
workflow/create-loop code. It does not change Go shadow code, Go fixtures, Go
routes, or backend selection.
```

Boundary:

| Area | Decision |
| --- | --- |
| Go G5/G6 | No Go fixture change is required; this is renderer/source entry-surface evidence. |
| Default backend | Go remains shadow-only and is not a default backend. |
| Renderer-visible Go route | No renderer route or Go-specific route is exposed. |

Validation:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0104 Parallel Task Dependency Executable Shadow

Status:
Go G5 remains shadow-only. This batch upgrades `parallel_tasks` dependency
validation from `jobReplay.parallelValidation` summary data into
`controlExecutableCases.taskJobs.parallelValidation` executable output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

Go shadow now runs a deterministic dependency validator over the fixture-owned
`validPlan` and `invalidPlans`. It emits valid order `a -> b -> c` and exact
error strings for `single_task`, `duplicate_id`, `self_dependency`, `cycle`,
and `unknown_dependency`. TypeScript conformance derives the expected fixture
from `task-job-orchestration-oracle.json`; Go unit tests compare the computed
output to that expected oracle.

Boundary:

```text
This batch does not implement a live Go Job Manager, public sub-agent/job
protocol, renderer-visible Go route, default Go backend, Electron integration,
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0106 Auto-Plan Payload Boundary Has No Go Runtime Delta

Status:
No Go runtime code changes. The batch is a TypeScript HTTP/loop contract guard
proving Reasonix-shaped `autoPlan` / `auto_plan` payload fields do not enable
Plan mode or advertise `create_plan`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts -t "auto-plan payload" --no-file-parallelism --maxWorkers=1
```

Result:

The captured start-turn request remains agent-mode with no `guiPlan`, and the
model request tool list does not include `create_plan`. G5/G6 gates remain
unchanged because this is not a Go backend behavior.

Boundary:

```text
This batch does not implement a Go auto-plan controller, live Go router,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
SessionAPI/config protocol, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer route, Rust/Tauri path, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0101 Auto-Plan Config Boundary Has No Go Runtime Delta

Status:
Go G5 remains shadow-only. This batch changes settings/config rejection tests
only; no Go runtime route, backend selector, or shadow fixture is modified.

Evidence:

```text
npm run test -- src/shared/app-settings.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- src/config/analytix-config.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

Reasonix `agent.auto_plan`, root `autoPlan` / `auto_plan`, runtime
`autoPlan` / `auto_plan`, and `analytix serve` config auto-plan roots are
rejected or stripped before they can become an analytix runtime contract.

Boundary:

```text
This batch does not implement a live Go auto-router, renderer-visible Go route,
default Go backend, Reasonix controller protocol, product auto-plan setting,
Rust/Tauri path, packaged settings QA, or release readiness.
```

## 2026-06-22 - D-0102 Task Parent Goal Evidence Executable Replay

Status:
Go G5 remains shadow-only. This batch promotes parent-goal evidence from the
G5 `jobReplay` summary into `controlExecutableCases.taskJobs`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.taskJobs.parentGoalEvidence` now records active-goal
and missing-goal event metadata. Go shadow computes `ledgeredWhenActiveGoal`
and `errorsWithoutActiveGoal` from the TS-owned fixture, and Go tests compare
the executable output to the oracle.

Boundary:

```text
This batch does not implement a live Go Job Manager, public sub-agent/job
protocol, renderer-visible Go route, default Go backend, Electron integration,
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0103 Product Sovereignty Scan Has No Go Runtime Delta

Status:
Go G5 remains shadow-only. This batch adds a reusable product-sovereignty scan
command and renderer route-surface coverage; no Go runtime fixture, route,
backend selector, or shadow output changes.

Evidence:

```text
npm run scan:product-sovereignty
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

The scan now checks default Go backend/Rust/Tauri activation and Rust/Tauri
files as part of the same product-sovereignty gate used for upstream identity
and top-level route surfaces.

Boundary:

```text
This batch does not implement a live Go backend, renderer-visible Go route,
default Go backend, Go G6 readiness, Rust/Tauri path, packaged desktop QA, or
release readiness.
```

## 2026-06-22 - D-0081 Approval/User-Input Route Body Replay

Status:
Go G5 remains shadow-only. This batch carries approval/user-input HTTP body and
structured prompt evidence into the G5 `approvalUserInputReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.approvalUserInputReplay` now includes
`approvalRoute`, `userInputSubmitRoute`, and `userInputCancelRoute`. The replay
summary preserves deny/cancel/submit request bodies, HTTP response bodies,
prompt question ids and option labels, pending gate counts, late resolve
statuses, and the rule that submitted-answer details are echoed by the HTTP
response but redacted from the SSE resolved event. Go shadow computes these
fields from `approval-user-input-route-oracle.json`.

Boundary:

```text
This batch does not implement a live Go approval/user-input manager, public
Reasonix ask/session protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0082 MCP Lifecycle Detail Replay

Status:
Go G5 remains shadow-only. This batch carries MCP reconnect, known-override,
and search workspace-boundary details into the G5 `mcpReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay` now includes `backgroundReconnect`,
`knownOverrideDiagnostics`, and `searchWorkspaceBoundary`. The replay summary
preserves retry/error server ids, suspended provider details, no-restart
requirement, codegraph/codebase-memory override diagnostics, trusted/untrusted
workspace search boundaries, and denied no-execute behavior. Go shadow computes
these fields from `mcp-tool-lifecycle-oracle.json`.

Boundary:

```text
This batch does not implement a live Go MCP client/indexer, Reasonix
MCP-indexer public lifecycle protocol, renderer-visible Go route, default Go
backend, Electron integration, top-level MCP-indexer navigation, Rust/Tauri
path, credentialed MCP matrix, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0083 Provider Usage Parser Precedence Replay

Status:
Go G5 remains shadow-only. This batch carries provider usage parser precedence
into the G5 `cacheReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.cacheReplay` now includes `usageParserReplay`. The
replay summary preserves unsupported-provider absent cache fields, DeepSeek
native cache precedence over `prompt_tokens_details.cached_tokens`, OpenAI
responses cached-token hit/miss calculation, and Anthropic read/creation cache
field accounting. Go shadow computes these fields from
`provider-cache-oracle.json`.

Boundary:

```text
This batch does not implement a live Go provider client, Reasonix provider
protocol, renderer-visible Go route, default Go backend, Electron integration,
credentialed provider matrix, packaged provider settings QA, Rust/Tauri path,
or release readiness.
```

## 2026-06-22 - D-0084 Auto-Router Classifier Currentness Replay

Status:
Go G5 remains shadow-only. This batch carries auto-router classifier
currentness, isolated request-shape, and timeout fallback evidence into
`controlExecutableCases`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`controlExecutableCases.autoRouterClassifier` now records classifier model,
timeout, fingerprint, isolated request shape, fingerprint drift invalidation,
timeout fallback, and product-boundary flags. TypeScript conformance ties the
fixture to live `AUTO_MODEL_ROUTER_*` constants and fingerprint builder; Go
shadow computes the expected output from that TS-owned fixture.

Boundary:

```text
This batch does not implement a public auto-plan setting, Reasonix controller
protocol, project/local auto-plan override, live Go auto-router, renderer-
visible Go route, default Go backend, Electron integration, packaged planner
QA, Rust/Tauri path, or release readiness.
```

## 2026-06-22 - D-0094 Task Parent Goal Evidence Replay

Status:
Go G5 remains shadow-only. This batch carries task/sub-agent parent-goal
evidence requirements into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.parentGoalEvidence` now includes
`requiresActiveGoal:true`, `evidenceLedgered`, `evidenceLedgerError`,
`usesReasonixProtocol:false`, and `topLevelRouteExposed:false`. Go shadow reads
these values from `task-job-orchestration-oracle.json`.

Boundary:

```text
This batch does not implement a live Go Job Manager, public sub-agent/job
protocol, renderer-visible Go route, default Go backend, Electron integration,
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0095 Planner-Executor Detail Replay

Status:
Go G5 remains shadow-only. This batch carries planner-executor detail evidence
into the G5 `jobReplay.plannerExecutor` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.plannerExecutor` now includes
`skippedReason`, `cancelReason`, `outputOffsetJobCount`, and
`transcriptPropagationJobCount`, in addition to the existing policy/status and
propagation flags. Go shadow reads these values from
`task-job-orchestration-oracle.json`.

Boundary:

```text
This batch does not implement a planner product toggle, live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch/
MCP-indexer route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0096 Task Tool Contract Boundary Replay

Status:
Go G5 remains shadow-only. This batch carries internal `task` and
`parallel_tasks` tool-contract boundaries into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.toolContractBoundary` now includes task
fields `prompt`, `run_in_background`, `continue_from`, and `fork_from`, plus
parallel task fields `tasks` and `depends_on`. It also records
`internalRuntimeOnly:true`, permission/dependency/planner-read-only gates,
`usesReasonixProtocol:false`, and `topLevelRouteExposed:false`.

Boundary:

```text
This batch does not implement a planner product toggle, live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch/
MCP-indexer route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0090 Provider Live-Local HTTP Proof Has No Go Runtime Delta

Status:
Go G5 remains shadow-only. This batch strengthens the TypeScript provider/cache
authority with no-credential local HTTP execution, but it does not add Go
provider execution.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`provider-cache-proof.test.ts` now executes all provider usage cases and all
request-shape cases through a local HTTP provider while preserving the original
provider base URL for request construction. This improves the source evidence
future Go G3/G5 cache/provider replays can consume, but no Go fixture, Go
shadow code, Go HTTP route, or Go backend selection changes in this batch.

Boundary:

```text
This batch does not implement a live Go provider client, live Go provider
matrix, default Go backend, renderer-visible Go route, Electron integration,
Reasonix provider protocol, Rust/Tauri path, packaged provider settings QA, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0093 G5 MCP Approval Annotation Replay

Status:
Go G5 remains shadow-only. This batch carries MCP approval annotation and
denied no-execute evidence into `mcpReplay` without adding a live Go approval
manager or MCP client.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay.approvalAnnotations` now records server
id, tool name, normalized tool name, destructive/open-world flags, approval id,
deny decision, approval result kind, `executed:false`, and
`deniedNoExecute:true`. Go shadow computes the summary from the TS-owned MCP
lifecycle oracle.

Boundary:

```text
This batch does not implement a live Go approval manager, live Go MCP client,
Go MCP route, renderer-visible Go route, default Go backend, Electron
integration, Reasonix MCP-indexer protocol, top-level MCP-indexer route,
Rust/Tauri path, credentialed MCP matrix, packaged desktop QA, Go G5/G6 parity,
or release readiness.
```

## 2026-06-22 - D-0092 G5 MCP Live-Local Indexer Summary Replay

Status:
Go G5 remains shadow-only. This batch carries richer MCP live-local indexer
proof into `mcpReplay` without adding a live Go MCP client.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay.liveLocalIndexer` now records server id,
cwd, low-priority/background-start flags, retry attempts, active paths,
tombstone count, snapshot restart, late tombstone, redacted diagnostics,
`leaksSecret:false`, and `topLevelRouteExposed:false`. Go shadow computes the
nested summary from the TS-owned MCP lifecycle oracle.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop QA, Go G5/G6 parity, or release
readiness.
```

## 2026-06-22 - D-0091 G5 Provider Live-Local Proof Summary Replay

Status:
Go G5 remains shadow-only. This batch carries provider live-local proof metadata
into the G5 `cacheReplay` summary without adding Go provider execution.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`provider-cache-oracle.json` now records `liveLocalHttpProof`, and
`shadowSlicesExpectedOutput.cacheReplay.liveLocalHttpProof` includes
fixture-only local HTTP transport, no live credentials, original provider base
URL preservation, provider usage/request-shape/post counts, endpoint formats,
provider families, and no-live-superiority policy. Go shadow computes the
counts and endpoint formats from the TS-owned provider-cache oracle.

Boundary:

```text
This batch does not implement a live Go provider client, live Go provider
matrix, default Go backend, renderer-visible Go route, Electron integration,
Reasonix provider protocol, Rust/Tauri path, packaged provider settings QA, Go
G5/G6 parity, or release readiness.
```

## 2026-06-22 - D-0088 G5 Provider Streaming Usage Replay

Status:
Go G5 remains shadow-only. This batch carries G3 provider streaming and usage
event evidence into the G5 `providerStreamingReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.providerStreamingReplay` now includes the G3 source
oracle id, streaming thread/since-seq/frame count, `item_delta` -> `usage` ->
`turn_completed` event order, `deepseek-prompt-cache` usage case id, parsed
prompt/completion/reasoning/total/cache hit/cache miss/cache hit-rate values,
and `usageEventMatchesExpectedCase: true`. Go shadow computes these fields from
the G3 provider streaming/usage/cache oracle.

Boundary:

```text
This batch does not implement a live Go provider client, provider request or
stream parser changes, renderer-visible Go route, default Go backend, Electron
integration, Reasonix provider protocol, credentialed provider matrix,
packaged provider settings QA, Rust/Tauri path, or release readiness.
```

## 2026-06-22 - D-0089 G5 Task-Job Route Executable Replay

Status:
Go G5 remains shadow-only. This batch carries internal task-job route
executable evidence into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`task-job-orchestration-oracle.routeExecutable` now records unauthorized
`401`, output route status/job status/next offset/replay offset, wait completed
status/result, kill status/error, missing output `404`, and rehydrated
output/wait/kill statuses. `shadowSlicesExpectedOutput.jobReplay.routeExecutable`
now carries the same summary, and Go shadow computes it from the TS-owned
task-job oracle.

Boundary:

```text
This batch does not implement a live Go Job Manager, live Go task-job route
server, renderer-visible Go route, default Go backend, Electron integration,
Reasonix SessionAPI, public sub-agent/job protocol, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, packaged desktop
QA, or release readiness.
```

## 2026-06-22 - D-0083 Cache Drift Attribution Replay

Status:
Go G5 remains shadow-only. This batch carries provider-cache drift attribution
into the G5 `cacheReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.cacheReplay.driftAttribution` now includes
previous/current prefix hashes, stable system/prefixItems booleans, changed
tool/provider/model/endpoint flags, expected reasons `tools`, `provider`, and
`model`, plus unsupported telemetry/cache-hit-rate state. Go shadow computes
these fields from `provider-cache-oracle.json`.

Boundary:

```text
This batch does not implement a live Go provider client, provider route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
provider protocol, live provider/cache superiority, dynamic-context stable
prefix, Rust/Tauri path, packaged settings QA, or release readiness.
```

## 2026-06-22 - D-0084 G3 Cache Drift Attribution Replay

Status:
Go G3 remains shadow-only. This batch carries provider-cache drift attribution
into the G3 provider streaming/usage/cache output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g3-provider-streaming-usage-cache-oracle.json` now contains raw
`cacheDriftAttribution` shapes from the provider-cache oracle and
`expectedOutput.cacheDriftAttribution` with the compact replay result. Go
shadow computes previous/current prefix hashes, stable system/prefixItems
booleans, changed tool/provider/model/endpoint flags, expected reasons, and
unsupported telemetry/cache-hit-rate state.

Boundary:

```text
This batch does not implement a live Go provider client, provider route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
provider protocol, provider request behavior changes, dynamic-context stable
prefix, Rust/Tauri path, packaged settings QA, or release readiness.
```

## 2026-06-22 - D-0085 G3 Provider Request Shape Replay

Status:
Go G3 remains shadow-only. This batch carries provider request URL/body/tool
shape evidence into the G3 provider streaming/usage/cache output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`go-g3-provider-streaming-usage-cache-oracle.json` now contains
`requestShapeMatrix` cases from the provider-cache oracle and
`expectedOutput.requestShapeSummary` with compact request-surface evidence.
Go shadow computes case count, exact URL count, endpoint families, custom full
endpoint ids, tool-shape families, and required/forbidden body-field family
counts.

Boundary:

```text
This batch does not implement a live Go provider client, provider route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
provider protocol, provider request behavior changes, Rust/Tauri path,
packaged settings QA, or release readiness.
```

## 2026-06-22 - D-0087 G5 Session Route Status Replay

Status:
Go G5 remains shadow-only. This batch carries G2 thread/session route status
and SSE replay semantics into the G5 `sessionReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.sessionReplay.routeStatusReplay` now includes route
counts, JSON/SSE route counts, `200`/`201` counts, archive/search status and
counts, read latest seq, update workspace, fork side lineage, resume
session/message summary, and SSE replay/caught-up frame/event evidence. Go
shadow computes these fields from `go-g2-route-replay-oracle.json`.

Boundary:

```text
This batch does not implement live Go thread/session routes, renderer-visible
Go route, default Go backend, Electron integration, Reasonix SessionAPI/public
protocol, Rust/Tauri path, packaged desktop restart/resume QA, or release
readiness.
```

## 2026-06-22 - D-0086 G5 Provider Request Shape Replay

Status:
Go G5 remains shadow-only. This batch carries provider request URL/body/tool
shape evidence into the G5 full-loop `cacheReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.cacheReplay.requestShapeReplay` now contains compact
request-surface evidence from the provider-cache oracle. Go shadow computes
case count, exact URL count, endpoint families, custom full endpoint ids,
tool-shape families, and required/forbidden body-field family counts.

Boundary:

```text
This batch does not implement a live Go provider client, provider route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
provider protocol, provider request behavior changes, Rust/Tauri path,
packaged settings QA, or release readiness.
```

## 2026-06-22 - D-0081 Approval Decision Route Replay

Status:
Go G5 remains shadow-only. This batch carries approval decision route and replay
ordering evidence into the G5 `approvalUserInputReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.approvalUserInputReplay` now includes approval id,
deny decision, denied status, no pending approval after resolution, duplicate
decision `409`, replay `sinceSeq`, and expected replay event kinds. Go shadow
computes these fields from `approval-user-input-route-oracle.json`.

Boundary:

```text
This batch does not implement a live Go approval manager, live Go
user-input manager, renderer-visible Go route, default Go backend, Electron
integration, Reasonix SessionAPI/public ask protocol, Rust/Tauri path,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0082 Offline Provider/Cache Parity Seal

Status:
Go G5 remains shadow-only. This batch carries fixture-only provider/cache parity
evidence into the G5 `cacheReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.cacheReplay.offlineParitySeal` now includes
fixture-only/no-live-superiority policy, DeepSeek stable/equivalent prefix hash
stability, prefix item/tool hash stability, DeepSeek stable cache hit/miss/rate,
diagnostics prefix unchanged + telemetry supported, usage/request-shape counts,
endpoint-format families, and release guard status/threshold/tail window. Go
shadow computes these fields from `provider-cache-oracle.json`.

Boundary:

```text
This batch does not implement a live provider matrix, live Go provider client,
default Go backend, renderer-visible Go route, Reasonix provider protocol,
Electron integration, packaged provider settings QA, or release readiness.
```

## 2026-06-22 - D-0077 Parallel Task Dependency Validation Replay

Status:
Go G5 remains shadow-only. This batch carries `parallel_tasks` dependency
validation evidence into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.parallelValidation` now includes
`depends_on`, valid order, invalid case ids, and error strings for single task,
duplicate id, self dependency, cycle, and unknown dependency. Go shadow
computes these fields from `task-job-orchestration-oracle.json`.

Boundary:

```text
This batch does not implement a live Go Job Manager, public sub-agent/job
protocol, renderer-visible Go route, default Go backend, Electron integration,
top-level Subagent/Workflow/Create Loop route, Rust/Tauri path, packaged
desktop QA, or release readiness.
```

## 2026-06-22 - D-0078 Task Job Lifecycle Replay

Status:
Go G5 remains shadow-only. This batch carries base `task` lifecycle evidence
into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.lifecycle` now includes foreground task
completion, background cross-turn running/output/final completion, and
wait/output/kill killed status plus cancellation error. Go shadow computes
these fields from `task-job-orchestration-oracle.json`.

Boundary:

```text
This batch does not implement a live Go Job Manager, public sub-agent/job
protocol, renderer-visible Go route, default Go backend, Electron integration,
top-level Subagent/Workflow/Create Loop/AutoResearch route, Rust/Tauri path,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0079 Task Job Route Boundary Replay

Status:
Go G5 remains shadow-only. This batch carries task-job route auth and forbidden
top-level route evidence into the G5 `jobReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.jobReplay.routeBoundary` now includes protected
route ids `wait` / `output` / `kill`, unauthorized status `401`, and forbidden
top-level routes `/v1/workflow`, `/v1/create-loop`, and `/v1/autoresearch`.
Go shadow computes these fields from `task-job-orchestration-oracle.json`.

Boundary:

```text
This batch does not implement live Go task-job routes, a live Go Job Manager,
public sub-agent/job protocol, renderer-visible Go route, default Go backend,
Electron integration, top-level Subagent/Workflow/Create Loop/AutoResearch
route, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0116 Provider Cache Release Guard Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider/cache release-guard
evidence from a summary replay into `controlExecutableCases.providerCacheReleaseGuard`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes fixture-only cache-hit tail
averages, allowed-low case counts, collapse counts, and pass/fail status from
the TS-owned provider/cache oracle. The control case proves the same offline
release guard through Go typed shadow output instead of relying only on
`cacheReplay.releaseGuard`.

Boundary:

```text
This batch does not implement a live Go provider client, live DeepSeek/OpenAI/
Anthropic/custom provider matrix, renderer-visible Go route, default Go
backend, Reasonix provider protocol, Electron integration, Rust/Tauri path,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0117 Provider Usage Parser Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider usage parser precedence
from `cacheReplay.usageParserReplay` into
`controlExecutableCases.providerUsageParser`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes typed DeepSeek native cache
precedence, OpenAI Responses cached-token usage, Anthropic cache field
inclusion, and unsupported-provider absent-field output from the TS-owned
provider/cache oracle.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0118 Provider Request Shape Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider request URL/body/tool
shape replay from `cacheReplay.requestShapeReplay` into
`controlExecutableCases.providerRequestShape`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes typed exact URL counts, endpoint
families, custom full endpoint ids, tool-shape families, and required/forbidden
body-field family counts from the TS-owned provider/cache oracle.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0119 Provider Cache Accounting Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider cache accounting from
`cacheReplay.cacheAccounting` into
`controlExecutableCases.providerCacheAccounting`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes typed supported telemetry case
ids, unsupported unknown case ids, DeepSeek/OpenAI/Anthropic provider-family
case ids, total cache hit/miss tokens, aggregate cache hit rate, and the
no-unsupported-miss invariant from the TS-owned provider/cache oracle.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0121 Provider Streaming Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider streaming usage replay
from `shadowSlicesExpectedOutput.providerStreamingReplay` into
`controlExecutableCases.providerStreaming`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes typed SSE event kinds, expected
event order, usage event match status, prompt/completion/reasoning/cache
tokens, cache hit rate, telemetry support, and product boundary from the
TS-owned G3 provider streaming oracle.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0122 Provider Offline Parity Seal Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider offline parity seal
from `shadowSlicesExpectedOutput.cacheReplay.offlineParitySeal` into
`controlExecutableCases.providerOfflineParitySeal`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes fixture-only/live-superiority
policy, stable prefix equivalence, prefix-items/tools hash stability, DeepSeek
cache hit/miss telemetry, request-shape endpoint format coverage, and release
guard status from minimal TS-owned provider/cache oracle inputs.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0123 Session Route Status Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes thread/session route status
from `shadowSlicesExpectedOutput.sessionReplay.routeStatusReplay` into
`controlExecutableCases.sessionRouteStatus`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes route/status counts,
archive/search counts, read/update fields, fork lineage, session resume
summary, SSE replay/caught-up frame counts, replay event names, and
missing-token auth rejection from the TS-owned G2 route oracle.

Boundary:

```text
This batch does not implement live Go thread/session routes, renderer-visible
Go routes, default Go backend, Reasonix SessionAPI/job protocol, Electron
integration, Rust/Tauri path, packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0124 Provider Live-Local HTTP Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider live-local HTTP proof
from `shadowSlicesExpectedOutput.cacheReplay.liveLocalHttpProof` into
`controlExecutableCases.providerLiveLocalHttpProof`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes fixture-only local HTTP policy,
usage/request-shape case counts, expected POST count, endpoint format coverage,
provider family coverage, and no-live-superiority claim status from TS-owned
provider/cache oracle inputs.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0170 Provider Request-Shape Derived Replay

Status:
Go G3/G5 remain shadow-only. This batch strengthens provider request-shape
replay so URL/header/body/tool shape evidence is derived from fixture inputs,
not only copied as an exact matrix.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG3ProviderConformanceOutput` and `BuildG5ControlExecutableOutput` now
compute derived URL match ids, header-shape match ids, body-shape match ids,
tool-shape match ids, custom full-endpoint exact-url ids, and
`customFullEndpointAppendedPathCount: 0` from the TS-owned provider/cache
oracle. The replay covers DeepSeek chat, OpenAI-compatible chat, OpenAI
Responses, Anthropic Messages, and custom full `/responses`, `/messages`, and
`/chat/completions` endpoint cases.

Boundary:

```text
This batch does not implement a live Go provider client, change provider URL
or body construction, expose Reasonix provider protocol, add renderer-visible
Go routes, enable a default Go backend, add Electron integration, Rust/Tauri
path, packaged provider QA, or release readiness.
```

## 2026-06-22 - D-0171 Post-881 Runtime Proof Freshness V2

Status:
Go remains shadow-only and unchanged. This batch strengthens the reusable
product-sovereignty scan so post-881 runtime proof leaves and core proof tokens
cannot silently disappear.

Evidence:

```text
npm run scan:product-sovereignty
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/task-job-orchestration-oracle.test.ts tests/mcp-tool-lifecycle-oracle.test.ts tests/create-plan-tool.test.ts --no-file-parallelism --maxWorkers=1
```

Result:

`scan:product-sovereignty` now requires task-job and MCP lifecycle fixture/test
leaves in `runtimeProofFreshnessPaths`, plus a `post881RuntimeProofTokenPaths`
set. Positive token scans pin provider coverage/request-shape evidence,
planner/task-job evidence, and MCP lifecycle/search/approval evidence.

Boundary:

```text
This batch does not implement live Go provider/MCP/job clients, change runtime
behavior, expose renderer-visible Go routes, enable default Go backend, expose
Reasonix public protocol, add Electron integration, Rust/Tauri path, packaged
QA, or release readiness.
```

## 2026-06-22 - D-0172 Provider Cache Accounting Raw Payload Replay

Status:
Go G3/G5 remain shadow-only. This batch strengthens provider cache accounting
so the shadow output parses raw provider `responseBody.usage` payloads before
computing cache telemetry and totals.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`BuildG3ProviderConformanceOutput` and `BuildG5ControlExecutableOutput` now
compute raw parsed case ids, raw telemetry-supported case ids,
raw expected-usage match ids, supported/unsupported ids, provider-family ids,
hit/miss totals, and aggregate hit rate from TS-owned provider/cache raw
payloads. The replay covers DeepSeek prompt/native cache telemetry, OpenAI
Responses cached tokens, Anthropic read/creation cache fields, and unsupported
OpenAI-compatible cache telemetry.

Boundary:

```text
This batch does not implement a live Go provider client, change provider URL
or body construction, expose Reasonix provider protocol, add renderer-visible
Go routes, enable a default Go backend, add Electron integration, Rust/Tauri
path, packaged provider QA, or release readiness.
```

## 2026-06-22 - D-0173 Provider Raw Accounting Proof Freshness

Status:
Go remains shadow-only and unchanged. This batch strengthens the TypeScript
oracle and product-sovereignty scan that future Go G3/G5 provider accounting
must continue to match.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Result:

`provider-cache-proof.test.ts` now derives raw usage snapshots and aggregate
cache accounting from `provider-cache-oracle.json` response bodies directly:
raw parsed ids, raw telemetry-supported ids, expected-usage matches,
unsupported unknown cases, DeepSeek/OpenAI Responses/Anthropic ids,
`2930/720` totals, and aggregate hit rate. `scan:product-sovereignty` now
requires raw accounting proof tokens in the post-881 runtime proof paths.

Boundary:

```text
This batch does not implement a live Go provider client, change provider URL
or body construction, expose Reasonix provider protocol, add renderer-visible
Go routes, enable a default Go backend, add Electron integration, Rust/Tauri
path, packaged provider QA, or release readiness.
```

## 2026-06-22 - D-0174 Combined Step/Cancel/Cache Proof Freshness

Status:
Go remains shadow-only and unchanged. This batch strengthens the focused G5
oracle and scan freshness for the combined auto-route cache, step-limit, and
cancel trace.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Result:

`go-runtime-conformance.test.ts` now has a dedicated check for
`controlExecutableCases.combined`, proving same-turn route-cache reuse through
step limit, next-turn reroute, classifier and step-limit stable-prefix
isolation, completed/running/unstarted cancel result pairing, and product
boundary flags. `scan:product-sovereignty` now requires combined trace proof
tokens in the post-881 runtime proof paths.

Boundary:

```text
This batch does not implement a live Go agent loop, change TypeScript runtime
behavior, expose Reasonix controller/session protocol, add renderer-visible Go
routes, enable a default Go backend, add Electron integration, Rust/Tauri path,
packaged cancel/cache QA, or release readiness.
```

## 2026-06-22 - D-0175 Approval/User-Input Proof Freshness

Status:
Go remains shadow-only and unchanged. This batch strengthens the focused G5
oracle and scan freshness for approval/user-input gate safety.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

Result:

`go-runtime-conformance.test.ts` now has a dedicated check for approval/user
input gates, proving denied task tools create no durable jobs or child runs,
HTTP user-input submission may echo answers while resolved events omit answers,
invalid structured choices do not open gates, abort cleanup expires/cancels
pending gates with late `409`/`404`, resumed pending gates clear answers, and
product boundary flags remain false. `scan:product-sovereignty` now requires
approval/user-input proof tokens in the post-881 runtime proof paths.

Boundary:

```text
This batch does not implement a live Go approval/user-input manager, change
TypeScript runtime behavior, expose Reasonix ask/session protocol, add
renderer-visible Go routes, enable a default Go backend, add Electron
integration, Rust/Tauri path, packaged approval-card QA, or release readiness.
```

## 2026-06-22 - D-0176 Session Route Proof Freshness

Status:
Go remains shadow-only and unchanged. This batch strengthens the focused G5
oracle and scan freshness for thread/session route replay.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

Result:

`go-runtime-conformance.test.ts` now has a dedicated check for session routes,
proving archive/search, read/update, fork, resume-thread, SSE replay/caught-up,
missing-token auth, exact JSON body hashes, exact SSE frame hash, route
inventory groups, and product boundary flags remain tied to the analytix-owned
G2 route oracle. `scan:product-sovereignty` now requires session route replay
proof tokens in the post-881 runtime proof paths and keeps the G2 route oracle
fixture in scope.

Boundary:

```text
This batch does not implement a live Go thread/session router, change
TypeScript runtime behavior, expose Reasonix SessionAPI/public event protocol,
add renderer-visible Go routes, enable a default Go backend, add Electron
integration, Rust/Tauri path, packaged route walkthrough QA, or release
readiness.
```

## 2026-06-22 - D-0177 Release Evidence Final-Gate Freshness

Status:
Go remains shadow-only and unchanged. This batch strengthens QA evidence
freshness for the post-881 proof chain.

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

Result:

`scan-product-sovereignty.cjs` now includes the release evidence gate in
explicit proof paths and checks final command gate entries for D-0172 through
D-0178. The scan also requires the release evidence file to keep the required
command names visible: runtime package tests, workspace tests, typecheck,
runtime build, non-cached Go shadow tests, and product-sovereignty scan.

Boundary:

```text
This batch does not implement live Go behavior, change TypeScript runtime
behavior, expose Reasonix public protocol, add renderer-visible Go routes,
enable a default Go backend, add Electron integration, Rust/Tauri path,
packaged desktop QA, or release readiness.
```

## 2026-06-22 - D-0178 Post-881 Stage Closure Snapshot

Status:
Go remains shadow-only and unchanged. This batch strengthens stage evidence
framing for the post-881 proof chain.

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

Result:

`docs/analytix/qa/post-881-stage-closure-2026-06-22.md` now records the current
fixture-level capability floor, absorbed Reasonix delta families, Kun baseline
preservation, verified stronger-than areas, and remaining open gates.
`scan-product-sovereignty.cjs` requires the closure path and core tokens so the
snapshot cannot silently disappear.

Boundary:

```text
This batch does not implement live Go behavior, change TypeScript runtime
behavior, expose Reasonix public protocol, add renderer-visible Go routes,
enable a default Go backend, add Electron integration, Rust/Tauri path,
packaged desktop QA, live provider/cache superiority, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0180 Custom Provider Telemetry Seal

Status:
Go remains shadow-only and unchanged as a backend. This batch strengthens the
provider/cache G5 control shadow so custom full endpoints are explicitly
request-shape-only and cannot be counted as cache telemetry.

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.providerCacheCoverageFloor.expected` now includes
`customFullEndpointTelemetryCaseIds: []`,
`customFullEndpointRequestShapeOnly: true`,
`telemetrySupportedExcludesCustomFullEndpoints: true`, and
`customProviderCacheTelemetryClaimAllowed: false`. TypeScript conformance and
Go shadow compute the same fields from the TS-owned provider/cache oracle.

Boundary:

```text
This batch does not implement live Go behavior, change provider runtime
behavior, add custom provider cache telemetry, expose Reasonix provider
protocol, add renderer-visible Go routes, enable a default Go backend, add
Electron integration, Rust/Tauri path, packaged provider QA, live
provider/cache superiority, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0184 MCP Malformed Schema Boundary Shadow

Status:
Go remains shadow-only and unchanged as a backend. This batch moves malformed
MCP schema normalization from a blank G4/G5 Go-result row into G5 executable
control shadow.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-provider.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.mcpMalformedSchemaBoundary` records TS-owned source
paths for `mcp-tool-provider.ts` and `mcp-tool-provider.test.ts`. TypeScript
conformance derives the malformed `inputSchema` fixtures, normalized advertised
tool names, default schema for non-object input, removal of non-record
`properties`, string-only `required`, and output-schema omission for
non-record values. Go shadow computes the same expected output.

Boundary:

```text
This batch does not implement a live Go MCP client, change TypeScript MCP
runtime behavior, expose Reasonix MCP-indexer protocol, add renderer-visible Go
routes, enable a default Go backend, add credentialed MCP QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0183 Event JSONL Replay Boundary Shadow

Status:
Go remains shadow-only and unchanged as a backend. This batch moves
`events.jsonl` replay and malformed-event recovery from blank Go-result rows
into G5 executable control shadow.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/runtime-event-recorder.test.ts tests/file-session-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.eventJsonlReplayBoundary` records TS-owned source paths
for `FileSessionStore`, `loop.test.ts`, `runtime-event-recorder.test.ts`, and
`file-session-store.test.ts`. TypeScript conformance derives newline-terminated
JSONL append, `loadEventsSince` filter/sort semantics, `highestSeq` max
behavior, malformed line recovery, persist-before-publish ordering, concurrent
seq uniqueness, persisted high-water caching, usage compaction carryover, and
compaction-failure append-only recovery. Go shadow computes the same expected
output.

Boundary:

```text
This batch does not implement a live Go event store, change TypeScript runtime
event persistence, expose Reasonix event protocol, add renderer-visible Go
routes, enable a default Go backend, add packaged long-thread replay QA, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0182 Tool Result File/Image Boundary Shadow

Status:
Go remains shadow-only and unchanged as a backend. This batch moves the
tool-result image/file fixture family from a blank Go-result row into G5
executable control shadow.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts src/loop/tool-result-image.test.ts tests/attachment-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.toolResultFileImageBoundary` records TS-owned source
paths for `tool-result-image`, attachment-store tests, and renderer mapper
tests. TypeScript conformance derives inline image kinds, base64 eviction with
metadata preservation, newest-image cap behavior, attachment `localFilePath`,
text fallback `FilePath`, DeepSeek v4 text fallback routing, and generated-file
meta lifting. Go shadow computes the same expected output.

Boundary:

```text
This batch does not implement a live Go file/image bridge, change attachment
storage, expose Reasonix file or SessionAPI protocol, add renderer-visible Go
routes, enable a default Go backend, add packaged attachment QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0181 Goal Persistence Off-Lock Control Shadow

Status:
Go remains shadow-only and unchanged as a backend. This batch closes the
previous `future Go lock fixture needed` gap by promoting goal persistence
off-lock behavior into G5 executable control shadow.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/thread-service.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.goalPersistenceOffLock` records the TS-owned
`ThreadService` source paths, `setGoal` / `clearGoal` persist-before-event
ordering, empty forbidden lock substring matches, persistence failure warning
proof, and product-boundary booleans. TypeScript conformance derives the case
from real source files, and Go shadow computes the same expected output.

Boundary:

```text
This batch does not implement live Go goal persistence, add controller locks,
change TypeScript runtime behavior, expose Reasonix controller protocol, add
renderer-visible Go routes, enable a default Go backend, add Electron
integration, Rust/Tauri path, packaged goal QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0179 Sub-Agent Review Matrix

Status:
Go remains shadow-only and unchanged. This batch records the six-lane sub-agent
review as scan-visible QA evidence and corrects the D-0177 final-gate range
wording.

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

Result:

`docs/analytix/qa/post-881-subagent-review-2026-06-22.md` now records
subagentAReasonixAgentKernel, subagentBKunBaseline, subagentCRuntimeCache,
subagentDMcpToolSubagent, subagentEGoRuntime, and subagentFQaDocs review
results. The matrix also pins customProviderRequestShapeOnly,
goRuntimeConformanceRangeCorrected, and subagentOpenGates tokens so the
sub-agent evidence cannot silently disappear.

Boundary:

```text
This batch does not implement live Go behavior, change TypeScript runtime
behavior, expose Reasonix public protocol, add renderer-visible Go routes,
enable a default Go backend, add Electron integration, Rust/Tauri path,
packaged desktop QA, live provider/cache superiority, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0187 Renderer Route-Surface Sovereignty Shadow

Status:
Go G5 remains shadow-only. This batch adds a renderer route-surface sovereignty
control case so Kun-compatible top-level entries and dormant workflow
quarantine are executable shadow inputs.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.rendererRouteSurfaceSovereignty` now includes
Workbench route-surface tests, route/store sources, browser preview bridge,
plugin marketplace, shell navigation, Workbench shell, and base shell CSS
source files. It proves the top-level `AppRoute` union remains
`chat/write/settings/plugins/claw/schedule`, forbidden Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer route tokens and entrypoint symbols are
absent, dormant Workflow/Create Loop code remains quarantined, browser preview
installs only `window.analytix`, plugin marketplace safe-area state is
propagated, and shell titlebar controls stay no-drag/safe-inset aware.
`BuildG5ControlExecutableOutput` computes the same expected output.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
UI/session protocol, add Kun/deprecated bridge aliases, add old runtime-shaped
settings fallback, enable a default Go backend, add renderer-visible Go routes,
add Electron Go integration, Rust/Tauri path, packaged route walkthrough, G6
readiness, or release readiness.
```

## 2026-06-22 - D-0188 Package / Runtime CLI Identity Shadow

Status:
Go G5 remains shadow-only. This batch adds a package/runtime identity control
case so `analytix serve`, release/package naming, afterPack runtime validation,
and `ANALYTIX_*` release env ownership are executable shadow inputs.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.packageRuntimeIdentity` now includes root/runtime
package manifests, electron-builder config, app identity, main AppUserModelID,
runtime binary resolver, runtime CLI entry/usage, afterPack validation,
packaging config tests, and release workflow sources. It proves root
package/product name stay `analytix`, runtime package/bin stay
`analytix-runtime` and `analytix -> ./dist/cli/serve-entry.js`,
builder app id/product/artifact/NSIS names stay analytix-owned, Windows
AppUserModelID stays `com.analytix.desktop`, bundled runtime resolution and
afterPack validation require `packages/runtime/dist/cli/serve-entry.js`,
the CLI supports only `analytix serve`, and release workflow env names stay
`ANALYTIX_*`. `BuildG5ControlExecutableOutput` computes the same expected
output.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
CLI/session protocol, add Kun/DeepSeek public identity, enable a default Go
backend, add renderer-visible Go routes, add Electron Go integration,
Rust/Tauri path, packaged artifact walkthrough, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0186 Desktop Main IPC Boundary Shadow

Status:
Go G5 remains shadow-only. This batch adds a dedicated main IPC boundary
control case so runtime request allow-list and SSE cursor/stop/batch semantics
are executable shadow inputs.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.desktopMainIpcBoundary` now includes main IPC schema and
handler source files, allowed analytix route examples, forbidden public route
examples, strict runtime request proof, strict SSE start proof, thread events
route proof, stream-id stop proof, reconnect cursor proof, and 100ms batching
proof. `BuildG5ControlExecutableOutput` computes the same expected output.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/main IPC protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0194 Main IPC Endpoint Builder Allow-list Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It extends the
TypeScript desktop boundary proof by showing the main IPC runtime request
schema accepts encoded shared endpoint builder output and rejects raw
extra-segment compatibility drift.

Evidence:

```text
npx vitest run src/main/ipc/app-ipc-schemas.test.ts
```

Result:

`runtimeRequestPayloadSchema` accepts encoded shared builder output for thread,
turn, checkpoint, approval, user-input, session, attachment, and memory routes.
It rejects unencoded dynamic route ids that create extra route segments and
rejects singular `/v1/user-input/:id` at the main IPC boundary. The canonical
desktop bridge contract remains plural `/v1/user-inputs/{id}` and analytix-
owned `/health` or `/v1/*` route shapes.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/main IPC protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0195 Main IPC Runtime Adapter Handoff Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It extends the
desktop boundary proof by showing the registered main IPC handler preserves
encoded analytix paths when handing requests to the TypeScript runtime adapter
and blocks invalid paths before adapter invocation.

Evidence:

```text
npx vitest run src/main/ipc/register-app-ipc-handlers.test.ts
```

Result:

`registerAppIpcHandlers` forwards encoded shared endpoint builder output to
`runtimeRequest` unchanged, preserving method and body values. Raw
extra-segment dynamic paths and singular `/v1/user-input/:id` compatibility
drift reject with `Invalid payload for runtime:request` and do not call the
runtime adapter.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/main IPC protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0196 Preload Runtime Request Bridge Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It adds executable
preload bridge evidence for the TypeScript desktop path from `window.analytix`
to the main IPC `runtime:request` channel.

Evidence:

```text
npx vitest run src/preload/preload-runtime-request.test.ts
```

Result:

The preload test mocks Electron, loads the exposed `analytix` API, and proves
`api.runtime.runtimeRequest` and diagnostics runtime request compatibility both
invoke `runtime:request` with encoded shared endpoint paths, method, and body
unchanged. The public bridge remains `window.analytix`.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/preload protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0197 Runtime Host URL Handoff Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript runtime adapter proof by verifying encoded analytix paths stay
encoded when forwarded to the managed `analytix serve` HTTP host.

Evidence:

```text
npx vitest run src/main/runtime/analytix-adapter.test.ts
```

Result:

`runtimeRequestViaHost` sends encoded shared endpoint paths and query strings
unchanged to the runtime host. The focused test uses a real local HTTP server
and confirms POST method, bearer auth, custom header, JSON content type, and
body preservation.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/runtime-host protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0198 Preload SSE Bridge Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It adds executable
preload bridge evidence for the TypeScript desktop SSE path from
`window.analytix` to the main IPC `runtime:sse:*` channels.

Evidence:

```text
npx vitest run src/preload/preload-sse-bridge.test.ts
```

Result:

The preload SSE test mocks Electron, loads the exposed `analytix` API, and
proves `startSse` / `stopSse` preserve thread id, cursor, stream id, and stop
id into analytix-owned IPC channels. Event, end, and error listeners forward
payloads without exposing Electron event objects and remove listeners on
unsubscribe.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/SSE protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0199 Main SSE Host URL Encoding Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript main SSE IPC proof by verifying dangerous thread ids are encoded
before fetching the managed `analytix serve` event stream.

Evidence:

```text
npx vitest run src/main/runtime-sse-ipc.test.ts
```

Result:

`registerRuntimeSseIpc` constructs analytix-owned `/v1/threads/{id}/events`
URLs with encoded thread ids, preserves `since_seq`, `Last-Event-ID`,
`Accept: text/event-stream`, and bearer auth, and returns fatal host failures
through `runtime:sse-error` for the same stream id.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/SSE protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0200 Renderer Runtime Client Bridge Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer bridge proof by verifying `rendererRuntimeClient` reads
only `window.analytix` for runtime request and SSE facade operations.

Evidence:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

Result:

`rendererRuntimeClient` forwards runtime request path/method/body unchanged,
forwards SSE start/stop and listener handlers unchanged, and does not read
throwing `window.kun` / `window.reasonix` legacy alias getters.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/renderer bridge protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0201 Renderer Settings Bridge Proof

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer settings bridge proof by verifying `rendererRuntimeClient`
uses `window.analytix.settings` for top-level runtime settings patches.

Evidence:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

Result:

`rendererRuntimeClient.setSettings` forwards a top-level `runtime` patch to
`window.analytix.settings.setSettings`, caches the returned settings, and does
not read throwing `window.kun` / `window.reasonix` legacy alias getters.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix config
roots or public settings protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop settings QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0202 Renderer Runtime Provider Alias Guard

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer provider proof by verifying the provider test bridge keeps
legacy aliases unread across existing route, approval/user-input, fork/resume,
and encoding tests.

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
```

Result:

`AnalytixRuntimeProvider` tests now run with throwing `window.kun` /
`window.reasonix` getters installed by the shared bridge helper. Existing
tests continue to prove analytix-owned HTTP routes, approval/user-input
submission/cancel, fork/resume, thread lifecycle, and dynamic route id encoding.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/renderer bridge protocol, add Kun/deprecated bridge aliases, enable
a default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged desktop QA, G6 readiness, or release
readiness.
```

## 2026-06-22 - D-0203 Renderer Provider Runtime Client Facade Seal

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer provider source by routing archive/restore through
`rendererRuntimeClient.runtimeRequest` and scanning against direct runtime
bridge bypass.

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
npm run scan:product-sovereignty
```

Result:

`archiveThread` now uses the renderer runtime client facade. The product
sovereignty scan forbids direct `window.analytix.runtime.runtimeRequest` calls
inside `analytix-runtime.ts`.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0204 Side Conversation Relation Provider Contract

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer store/provider contract by routing side conversation
promotion through `AgentProvider.updateThreadRelation`.

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts
npm run scan:product-sovereignty
```

Result:

`AnalytixRuntimeProvider.updateThreadRelation` sends relation PATCH requests
through `rendererRuntimeClient.runtimeRequest`, and `promoteSideConversation`
uses that provider contract. The product sovereignty scan forbids direct
runtime request bridge calls in provider and side-store sources.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged side-conversation QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0205 Renderer Usage Runtime Client Facade Seal

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer generic runtime request path by routing usage/debug
requests through `rendererRuntimeClient.runtimeRequest`.

Evidence:

```text
npx vitest run src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts
npm run scan:product-sovereignty
```

Result:

Usage hooks and settings diagnostics keep their `/v1/usage` and
`/v1/debug/llm-rounds` paths but use the renderer runtime client facade. The
product sovereignty scan forbids direct generic runtime request bridge calls
across renderer production source.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged usage/dashboard QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0206 Renderer Settings Read Facade Seal

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer settings read path by routing ordinary settings reads
through `rendererRuntimeClient.getSettings`.

Evidence:

```text
npx vitest run src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts src/renderer/src/agent/runtime-client.test.ts
npm run scan:product-sovereignty
```

Result:

Keyboard shortcut settings, speech-to-text settings, and initial usage model
label reads use the renderer runtime client facade. The product sovereignty scan
forbids direct `window.analytix.settings.getSettings` in renderer production
source.

Boundary:

```text
This batch does not expose Reasonix settings protocol, add Kun/deprecated bridge
aliases, enable a default Go backend, add renderer-visible Go routes, add
Electron Go integration, Rust/Tauri path, packaged settings QA, G6 readiness,
or release readiness.
```

## 2026-06-22 - D-0207 Renderer Named Bridge API Allow-list

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer bridge boundary by making remaining direct
`window.analytix.runtime.*` and `window.analytix.settings.*` production calls
explicit allow-listed named contracts.

Evidence:

```text
node --check scripts/scan-product-sovereignty.cjs
npm run scan:product-sovereignty
```

Result:

The product sovereignty scan now allows only named runtime APIs for config file
access, provider probing, runtime restart, upstream model fetch, and runtime
status subscription, plus named settings write APIs `setSettings` and
`saveSettingsSilent`. Generic runtime HTTP requests and ordinary settings reads
remain forbidden as direct renderer bridge calls.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged bridge QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0208 Renderer Optional Bridge Bypass Seal

Status:
Go G5 remains shadow-only. This batch does not add Go code. It strengthens the
TypeScript renderer bridge boundary by treating optional-chaining access to
`window.analytix.runtime.*` and `window.analytix.settings.*` as the same direct
bridge surface as dot access.

Evidence:

```text
node --check scripts/scan-product-sovereignty.cjs
rg -n "window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings|window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest" src/renderer/src --glob '!**/*.test.ts'
npm run scan:product-sovereignty
```

Result:

Connect Phone dialog settings loading now uses `rendererRuntimeClient`, plugin
marketplace diagnostics no longer probe the generic runtime bridge directly,
and product-sovereignty scans cover both dot and optional-chain direct bridge
forms.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged bridge QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0209 Renderer Bridge Allow-list G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends the existing
`controlExecutableCases.desktopSovereignty` control case so renderer direct
bridge allow-list evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererBridgeAllowList` now records allowed named runtime
and settings APIs, source-derived direct method inventories, empty violation
sets, zero generic bypass counts, and optional-chain scan coverage. Go shadow
computes `rendererNamedRuntimeApisAllowListed`,
`rendererNamedSettingsApisAllowListed`, `rendererGenericRuntimeBypassAbsent`,
`rendererSettingsReadBypassAbsent`, and `optionalChainBridgeAccessScanned`.

Boundary:

```text
This batch does not expose Reasonix SessionAPI/renderer bridge protocol, add
Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged bridge QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0210 Runtime HTTP Auth Matrix G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.runtimeHttpRouteSovereignty` so the D-0190 auth matrix
is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`runtimeHttpRouteSovereignty.authMatrix` now records `/health` 200, all 43
protected `/v1/*` route keys, canonical unauthorized status/body, and sensitive
SSE/task-job/approval/user-input/resume route keys. Go shadow computes
`healthUnauthenticatedStatusOk`,
`allAuthenticatedRoutesRejectMissingAuth`, `unauthorizedBodyShapeStable`,
`authMatrixCoversAllRegisteredRoutes`, and `sensitiveRoutesProtected`.

Boundary:

```text
This batch does not implement a live Go HTTP server, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0211 Runtime Forbidden Route Dispatch G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.runtimeHttpRouteSovereignty` so the D-0191 forbidden
route dispatch matrix is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` now records valid runtime
auth mode, 404 status/body, all 13 forbidden route tokens, six upstream
protocol tokens, and seven hidden-surface tokens. Go shadow computes
`forbiddenDispatchReturnsStructuredNotFound`,
`forbiddenDispatchCoversAllTokens`, `forbiddenProtocolTokensRejected`,
`forbiddenHiddenSurfaceTokensRejected`, and
`forbiddenDispatchUsesRuntimeAuth`.

Boundary:

```text
This batch does not implement a live Go HTTP server, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0212 Shared Endpoint Builder G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.runtimeHttpRouteSovereignty` so the D-0192 shared
endpoint builder matrix is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` now records 18 shared
builder cases, encoded slash/query/fragment ids, exported endpoint strings,
forbidden-token absence, canonical plural user-input state, sensitive builder
names, and source unit-test proof. Go shadow computes
`sharedEndpointBuilderCaseCountExact`,
`sharedEndpointBuildersEncodeRouteIds`, `sharedEndpointTemplatesAnalytixOwned`,
`sharedEndpointCanonicalUserInputPlural`,
`sharedEndpointSensitiveBuildersPresent`, and
`sharedEndpointBuilderUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go HTTP server, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0213 Renderer Runtime Endpoint Builder G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer-provider runtime path
construction is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererProviderEndpointMatrix` now records analytix shared
root paths, dangerous slash/query/fragment source ids, encoded dynamic runtime
request paths for turns, steer, interrupt, compact, approval, user-input, fork,
and resume-thread, renderer runtime-client facade use, ownership guard proof,
and unit-test proof. Go shadow computes
`rendererProviderUsesSharedRootPaths`,
`rendererProviderEncodesDynamicRouteIds`,
`rendererProviderRuntimePathsAnalytixOwned`,
`rendererProviderUsesRuntimeClientFacade`, and
`rendererProviderEndpointBuilderUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go desktop bridge, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0214 Main IPC Endpoint Builder G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopMainIpcBoundary` so main IPC runtime request
schema endpoint-builder allow-list evidence is executable shadow input and
output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopMainIpcBoundary.endpointBuilderAllowListMatrix` now records encoded
shared-builder requests for thread, fork, turns, steer, interrupt, checkpoint
rewind, approval, user-input, session resume, attachment content, and memory
record paths; raw dynamic route rejections; singular user-input rejection;
shared template compilation; and unit-test proof. Go shadow computes
`mainIpcEndpointBuilderAcceptsSharedPaths`,
`mainIpcEndpointBuilderRejectsRawDynamicRoutes`,
`mainIpcEndpointBuilderUsesSharedTemplates`,
`mainIpcEndpointBuilderUnitProofPresent`, and
`mainIpcEndpointBuilderRejectsSingularUserInput`.

Boundary:

```text
This batch does not implement a live Go main IPC bridge, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged IPC/route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0215 Main IPC Runtime Adapter Handoff G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopMainIpcBoundary` so registered main IPC
runtime-adapter handoff evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` now records seven encoded
adapter calls, six rejected-before-adapter raw/singular calls,
parse-before-adapter ordering, encoded path proof, method/body proof, and
reject-before-adapter proof. Go shadow computes
`mainIpcRuntimeAdapterPreservesEncodedPaths`,
`mainIpcRuntimeAdapterPreservesMethodAndBody`,
`mainIpcRuntimeAdapterRejectsRawDynamicRoutes`,
`mainIpcRuntimeAdapterRejectsBeforeCall`, and
`mainIpcRuntimeAdapterUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go main IPC bridge, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged IPC/route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0231 MCP Search Refresh Drift Evidence Closure

Status:
Go G5 remains shadow-only. This batch does not change Go or TypeScript runtime
behavior; it makes the already-existing `controlExecutableCases.mcpSearchRefreshDrift`
proof mandatory in product-sovereignty scans and closure docs.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`mcpSearchRefreshDrift` records `mcp_refresh_catalog`, server `github`, initial
`search_issues`, expanded `search_issues` plus `create_issue`,
`totalIndexed: 2`, and `catalogDrift: true`. Runtime tests already exercise the
refresh path through the MCP provider. Go shadow already replays the same
refresh drift output. D-0231 adds a scan guard so this proof cannot disappear
without failing `scan:product-sovereignty`.

Boundary:

```text
This batch does not change runtime behavior, implement a live Go MCP client,
expose Reasonix MCP public protocol or SessionAPI, add a top-level MCP-indexer
route, add Kun/deprecated bridge aliases, enable a default Go backend, add
renderer-visible Go routes, add Rust/Tauri path, credentialed MCP matrix,
packaged MCP QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0230 Plan/Auto-Route State Reset G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.planCancelStateReset` so cancelled Plan turn state reset
and post-cancel auto-route currentness are executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`planCancelStateReset` records the source runtime test, previous Plan mode,
aborted status, `create_plan`, fixed normal model, normal/auto no-advertise
sets, absent normal/auto `modeInstruction`, absent normal/auto
`requiredToolName`, post-cancel `model: "auto"`, one `_auto_router` call,
isolated router request with zero tools and zero prefix items, current
`deepseek-v4-pro` / `max` recommendation, stable-prefix cleanliness, and
product-boundary booleans. Go shadow computes cancelled-plan no-leak,
normal/auto create-plan hiding, normal/auto no required plan tool, auto reroute,
router isolation, current recommendation, stable-prefix cleanliness, and
boundary outputs.

Boundary:

```text
This batch does not change TypeScript runtime behavior beyond adding proof,
implement a live Go loop, expose Reasonix SessionAPI/controller/config
protocol, add a public auto-plan setting or project override, add a top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, add Kun/deprecated
bridge aliases, enable a default Go backend, add renderer-visible Go routes,
add Rust/Tauri path, packaged Plan/auto-route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0229 Plan Step/Cancel/Cache G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.planStepCancelCache` so explicit Plan-mode
step/cancel/cache behavior is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/loop.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`planStepCancelCache` records Plan mode, the `plan-cache-cancel` model, three
requests, aborted follow-up status, retry failure status, step-0
`create_plan` + `ls`, `bash` exclusion, follow-up forced to `create_plan`, two
usage events, DeepSeek `chat_completions` `80/20` hit/miss telemetry,
`prefixChanged: false`, and empty prefix-change reasons. The TypeScript
conformance test also asserts the D-0107 `loop.test.ts` source still contains
the required plan follow-up, stable-prefix, and cache telemetry evidence. Go
shadow computes step gating, follow-up-only-plan, cancelled-baseline, retry
baseline reuse, cache telemetry preservation, forbidden-shell exclusion, and
product-boundary booleans.

Boundary:

```text
This batch does not change TypeScript runtime behavior, implement a live Go
loop, expose Reasonix SessionAPI/controller/config protocol, add a top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, add Kun/deprecated
bridge aliases, enable a default Go backend, add renderer-visible Go routes,
add Rust/Tauri path, packaged Plan QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0228 MCP Call-Time Reconnect G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.mcpCallReconnect` so MCP tool-call reconnect
classification is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`mcp-tool-lifecycle-oracle` now records call-time reconnect classification:
stale connection transport errors retry once, close the stale client, and
succeed on the second client; deterministic MCP protocol validation errors
return `tool_execution_failed` without reconnecting. The TypeScript oracle test
runs the real `buildMcpToolProviders` call path. Go shadow computes
`transportErrorRetried`, `protocolErrorRetried`, stale/protocol attempt counts,
close counts, stale success, protocol error code, and product-boundary booleans.

Boundary:

```text
This batch does not change live TypeScript MCP behavior, implement a live Go
MCP client, expose Reasonix MCP public protocol or SessionAPI, add a top-level
MCP-indexer route, add Kun/deprecated bridge aliases, enable a default Go
backend, add renderer-visible Go routes, add Rust/Tauri path, credentialed MCP
matrix, packaged MCP QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0227 AutoResearch Direction Tracking G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.autoResearch` so direction tracking is executable
shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- go-runtime-conformance
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.autoResearch` now records a direction, outcome,
summary, `record_research_direction` tool name, and active research-goal
requirement. The TypeScript conformance test creates real project-local state,
calls `AutoResearchProjectStore.recordDirection`, reads
`directions_tried.json`, reads `iteration_log.jsonl`, verifies the
`direction_recorded` log entry, and keeps unknown requirement evidence rejected
without findings writes. Go shadow computes `directionTrackingFileWritten`,
`iterationLogRecordsDirection`, `recordResearchDirectionToolPresent`, and
`recordDirectionRequiresActiveResearchGoal`.

Boundary:

```text
This batch does not change TypeScript runtime behavior, implement a live Go
AutoResearch backend, expose Reasonix project/session protocol, add a
top-level AutoResearch route, write `REASONIX.md` or `AGENTS.md`, put research
state into stable prefix or tool schema, add Kun/deprecated bridge aliases,
enable a default Go backend, add renderer-visible Go routes, add Rust/Tauri
path, packaged long-task QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0226 Renderer Settings Read Facade G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer settings-read facade
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererSettingsReadFacadeMatrix` now records
`rendererRuntimeClient.getSettings`, forbidden
`window.analytix.settings.getSettings`, settings readers for keyboard
shortcuts, speech-to-text, and initial usage model label, source-only settings
client proof, settings-changed event preservation, direct settings bridge
rejection, scan guard, and settings-read facade token scan proof. Go shadow
computes `rendererSettingsReadFacadeCoversKeyboardShortcuts`,
`rendererSettingsReadFacadeCoversSpeechToText`,
`rendererSettingsReadFacadeCoversUsageModelLabel`,
`rendererSettingsReadFacadePreservesSettingsChangedEvent`,
`rendererSettingsReadFacadeRejectsDirectBridge`, and
`rendererSettingsReadFacadeScanGuardPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop/settings bridge, expose Reasonix config/settings/session protocol,
allow direct renderer settings bridge bypass, add Kun/deprecated bridge
aliases, add old runtime-shaped settings fallback, enable a default Go backend,
add renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged settings QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0225 Renderer Usage Runtime Client Facade G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer usage/debug runtime
client facade evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` now records
`rendererRuntimeClient.runtimeRequest`, forbidden
`window.analytix.runtime.runtimeRequest`, thread/day/model usage loaders,
token economy and LLM debug diagnostics loaders, source-only runtime client
proof, direct bridge rejection, usage unit proof, scan guard, and usage facade
token scan proof. Go shadow computes
`rendererUsageRuntimeClientCoversThreadUsage`,
`rendererUsageRuntimeClientCoversDailyUsage`,
`rendererUsageRuntimeClientCoversModelUsage`,
`rendererUsageRuntimeClientCoversSettingsDiagnostics`,
`rendererUsageRuntimeClientRejectsDirectBridge`,
`rendererUsageRuntimeClientScanGuardPresent`, and
`rendererUsageRuntimeClientUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, expose Reasonix usage/debug/session protocol, allow direct
renderer runtime bridge bypass, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged usage/dashboard QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0224 Side Conversation Relation G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so side conversation relation
provider/store evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.sideConversationRelationContractMatrix` now records
provider method `updateThreadRelation`, store action `promoteSideConversation`,
relation `primary`, optional provider contract proof, provider implementation
through runtime client, store provider-call proof, refresh/close proof, direct
bridge rejection, unit proof, and scan guard proof. Go shadow computes
`sideConversationRelationContractOptionalProvider`,
`sideConversationRelationPromotesThroughProvider`,
`sideConversationRelationRefreshesAndCloses`,
`sideConversationRelationRejectsDirectBridge`,
`sideConversationRelationScanGuardPresent`, and
`sideConversationRelationUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, expose Reasonix side/session protocol, allow direct side-store
runtime bridge bypass, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged side QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0223 Renderer Provider Facade Seal G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer provider facade seal
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererProviderFacadeSealMatrix` now records
`rendererRuntimeClient.runtimeRequest`, forbidden
`window.analytix.runtime.runtimeRequest`, sealed provider methods,
source-only runtime client proof, archive/restore source proof, relation PATCH
source proof, lifecycle unit proof, direct-bypass scan guard, and provider
facade token scan proof. Go shadow computes
`rendererProviderFacadeSealUsesRuntimeClient`,
`rendererProviderFacadeSealRejectsDirectBridge`,
`rendererProviderFacadeSealCoversArchiveRestore`,
`rendererProviderFacadeSealCoversRelationPatch`,
`rendererProviderFacadeSealScanGuardPresent`, and
`rendererProviderFacadeSealUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, allow direct renderer bridge bypass, expose Reasonix
SessionAPI/public bridge protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged provider QA, G6 readiness, or release readiness.
```

## 2026-06-23 - D-0222 Renderer Provider Alias Guard G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer provider alias guard
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererProviderAliasGuardMatrix` now records the
`installDsGui` helper, forbidden `kun` / `reasonix` aliases, guarded provider
tests, route/lifecycle/approval-user-input/fork-resume/dynamic-encoding
coverage, source-only analytix proof, and forbidden-route guard proof. Go
shadow computes `rendererProviderAliasGuardInstallsThrowingAliases`,
`rendererProviderAliasGuardCoversRuntimeRoutes`,
`rendererProviderAliasGuardCoversLifecycleAndGates`,
`rendererProviderAliasGuardCoversForkResumeAndEncoding`,
`rendererProviderAliasGuardRejectsForbiddenRoutes`, and
`rendererProviderAliasGuardUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, expose Reasonix SessionAPI/public bridge protocol, add
Kun/deprecated bridge aliases, add old runtime-shaped settings fallback, enable
a default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged provider QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0221 Renderer Settings Bridge G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer settings bridge
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/runtime-client.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererSettingsBridgeMatrix` now records top-level
`runtime` patch values, settings cache expectations, analytix settings API
source proof, cache/write refresh proof, unit proof, and legacy alias unread
proof. Go shadow computes `rendererSettingsBridgeUsesAnalytixSettingsApi`,
`rendererSettingsBridgeCachesReads`,
`rendererSettingsBridgeRefreshesCacheAfterWrite`,
`rendererSettingsBridgePreservesTopLevelRuntimePatch`,
`rendererSettingsBridgeLegacyAliasesUnread`, and
`rendererSettingsBridgeUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, expose Reasonix settings protocol/config root, add
Kun/deprecated bridge aliases, add old runtime-shaped settings fallback, enable
a default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged settings QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0220 Renderer Runtime Client Bridge G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so renderer runtime client bridge
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.rendererRuntimeClientBridgeMatrix` now records encoded
runtime request path variants, argument counts, runtime restart proof, SSE
start/stop calls, listener APIs, source passthrough proof, unit proof, and
legacy alias unread proof. Go shadow computes
`rendererRuntimeClientRuntimeRequestPreservesArguments`,
`rendererRuntimeClientRestartUsesRuntimeApi`,
`rendererRuntimeClientSseControlsPreserveArguments`,
`rendererRuntimeClientSseListenersPreserveHandlers`,
`rendererRuntimeClientLegacyAliasesUnread`, and
`rendererRuntimeClientUnitProofPresent`.

Boundary:

```text
This batch does not change TypeScript renderer behavior, implement a live Go
desktop bridge, expose Reasonix SessionAPI/public frontend protocol, add
Kun/deprecated bridge aliases, add old runtime-shaped settings fallback, enable
a default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged renderer QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0219 Main SSE Host URL Encoding G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopMainIpcBoundary` so main SSE host URL encoding
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopMainIpcBoundary.mainSseHostEncodingMatrix` now records dangerous source
thread id, encoded thread id, `/v1/threads/{id}/events`, `since_seq`,
`Last-Event-ID`, `Accept: text/event-stream`, bearer auth, stream id, error
payload, source proof, unit proof, and forbidden-route guard. Go shadow
computes `mainSseHostEncodesThreadIdAndCursor`,
`mainSseHostPreservesHeadersAndStreamId`,
`mainSseHostRejectsForbiddenRouteTokens`, and
`mainSseHostEncodingUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go SSE server, expose Reasonix
SessionAPI/public SSE protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged SSE QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0218 Preload SSE Bridge G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so preload SSE bridge evidence is
executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.preloadSseBridgeMatrix` now records
`runtime:sse:start/stop/event/end/error` channels, start/stop calls, event/end/
error payloads, payload-only wrapper proof, cleanup proof, unit proof, and
analytix-only bridge exposure. Go shadow computes
`preloadSseStartStopPreservesArguments`,
`preloadSsePayloadListenersOmitElectronEvent`,
`preloadSseListenerCleanupUsesSameWrapper`, and
`preloadSseBridgeUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go SSE server, expose Reasonix
SessionAPI/public SSE protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged SSE QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0217 Runtime Host URL Handoff G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopMainIpcBoundary` so runtime host URL handoff
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopMainIpcBoundary.runtimeHostHandoffMatrix` now records
`runtimeRequestViaHost`, `getRuntimeBaseUrlForSettings`, encoded dynamic ids,
query preservation, POST body, bearer auth, custom proof header, JSON content
type, source proof, and ensureRuntime port-switch proof. Go shadow computes
`mainRuntimeHostHandoffPreservesEncodedPathAndQuery`,
`mainRuntimeHostHandoffPreservesMethodHeadersBody`,
`mainRuntimeHostHandoffUsesEnsuredSettings`, and
`mainRuntimeHostHandoffUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go HTTP server, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged route QA, G6 readiness, or release
readiness.
```

## 2026-06-23 - D-0216 Preload Runtime Request Bridge G5 Shadow

Status:
Go G5 remains shadow-only. This batch extends
`controlExecutableCases.desktopSovereignty` so preload runtime request bridge
evidence is executable shadow input and output.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`desktopSovereignty.preloadRuntimeRequestBridgeMatrix` now records runtime and
diagnostics facade calls, encoded dynamic ids, path/method/body preservation,
same-channel diagnostics behavior, and analytix-only bridge exposure proof. Go
shadow computes `preloadRuntimeRequestPreservesPathMethodBody`,
`preloadDiagnosticsRuntimeRequestUsesSameChannel`,
`preloadRuntimeRequestExposesOnlyAnalytixApi`, and
`preloadRuntimeRequestUnitProofPresent`.

Boundary:

```text
This batch does not implement a live Go preload bridge, expose Reasonix
SessionAPI/public route protocol, add Kun/deprecated bridge aliases, enable a
default Go backend, add renderer-visible Go routes, add Electron Go
integration, Rust/Tauri path, packaged preload/route QA, G6 readiness, or
release readiness.
```

## 2026-06-22 - D-0185 Desktop Bridge IPC Sovereignty Shadow

Status:
Go G5 remains shadow-only. This batch extends the existing desktop sovereignty
control case so runtime request and SSE IPC bridge names are executable shadow
inputs instead of prose-only scan evidence.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Result:

`controlExecutableCases.desktopSovereignty` now includes `runtimeIpcChannels`,
`forbiddenRuntimeIpcChannels`, `forbiddenRuntimeIpcChannelsExposed`,
`runtimeRequestUsesAnalytixIpc`, `runtimeSseUsesAnalytixIpc`,
`publicApiTypesAnalytixOwned`, and `forbiddenRuntimeIpcExposed`. The TypeScript
oracle derives them from real preload/shared/settings sources, and
`BuildG5ControlExecutableOutput` computes the same expected output.

Boundary:

```text
This batch does not change TypeScript runtime behavior, expose Reasonix
SessionAPI/frontend protocol, add Kun/deprecated bridge aliases, add old
runtime-shaped settings fallback, enable a default Go backend, add
renderer-visible Go routes, add Electron Go integration, Rust/Tauri path,
packaged desktop QA, G6 readiness, or release readiness.
```

## 2026-06-22 - D-0125 Provider Drift Attribution Control Shadow

Status:
Go G5 remains shadow-only. This batch promotes provider drift attribution from
`shadowSlicesExpectedOutput.cacheReplay.driftAttribution` into
`controlExecutableCases.providerDriftAttribution`.

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`BuildG5ControlExecutableOutput` now computes previous/current prefix hashes,
system hash stability, prefix-item stability, tools/provider/model/endpoint
format changes, expected drift reasons, telemetry support, and cache hit-rate
known status from TS-owned provider/cache oracle inputs.

Boundary:

```text
This batch does not implement a live Go provider client, live provider/cache
superiority matrix, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Electron integration, Rust/Tauri path, packaged desktop QA,
or release readiness.
```

## 2026-06-22 - D-0080 MCP Core Lifecycle Replay

Status:
Go G5 remains shadow-only. This batch carries MCP connect/disconnect/reload/
cancel/error evidence into the G5 `mcpReplay` summary.

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

Result:

`shadowSlicesExpectedOutput.mcpReplay.lifecycle` now includes connect and
disconnect tool names/availability/tool counts, disconnect reason, reload tool
names and schema-order stability, cancel-before-start no-execute evidence, and
approved `tool_execution_failed` error shape. Go shadow computes these fields
from `mcp-tool-lifecycle-oracle.json`.

Boundary:

```text
This batch does not implement a live Go MCP client, Go MCP route,
renderer-visible Go route, default Go backend, Electron integration, Reasonix
MCP-indexer protocol, top-level MCP-indexer route, Rust/Tauri path,
credentialed MCP matrix, packaged desktop QA, or release readiness.
```
