# Go Runtime Retirement Checklist

This is the current formal checklist for Go-runtime default validation,
post-cutover live evidence, and future TypeScript fallback retirement. Historical
D-0249/D-0250 sections remain below as archive context only.

This checklist is now a post-cutover fallback-retirement and live-validation
plan. Go runtime default delivery is active through `go-runtime-default`.
TypeScript runtime is not the default path; `ANALYTIX_RUNTIME_BACKEND=typescript`
is a retired-backend diagnostic in the current adapter. This checklist is not
authorization to physically delete remaining historical TypeScript source today.

Current release status as of 2026-06-25:

- D-0248 strict deterministic gate is complete.
- D-0250C code-stage Reasonix absorption is complete.
- Go deterministic core is the default runtime path.
- Reasonix code-stage absorption remains engine/runtime-only and preserves the
  Kun/Analytix product baseline.
- D-0251/D-0252/D-0253 provider, MCP, packaged, operator, soak, and fallback
  retirement evidence is post-cutover live validation. Missing external
  credentials, MCP commands, packaged runtime state, or operator approval must
  not be treated as blockers for Go default startup.

Current gate note (updated 2026-08-18): after an RC source freeze,
`npm run runtime:go:rc-control-plane -- --json` may prove only the source-bound
validation control plane. Release packaging depends on
`npm run runtime:go:release-gate`, which succeeds only when its final JSON has
`passed: true` after product sovereignty, preflight, production Go validation,
artifact admission, the active OpenSpec RC ledger, and authorized formal
product evidence close. Historical stage-numbered command names remain archive
context only.

## D-0250A Architecture Cleanup Guardrail

D-0250A closed runtime-go package organization only. At D-0250A time it was not
G6 cutover evidence and did not authorize Go as the default backend. That
historical status is superseded by the final Go runtime delivery pass: Go is
now default, while D-0251/D-0252/D-0253 live evidence remains post-cutover
validation and fallback-retirement authorization.

D-0250A keeps the Analytix product layer unchanged:

- No renderer-visible Go switcher or Reasonix protocol is added.
- `window.analytix`, top-level `runtime` settings, product entry points, and
  `analytix serve` remain unchanged.
- `packages/runtime-go` root files remain only as public compatibility shims,
  legacy conformance test wrappers, or cmd construction entry points.
- Production runtime core is organized under `internal/server`,
  `internal/provider`, `internal/mcp`, `internal/agent`, `internal/jobs`,
  `internal/goal`, `internal/research`, `internal/readiness`, and
  `internal/contracts`.
- Evidence, topology, matrix, and checklist assets live under
  `internal/evidence/test-only` or stay as thin root shims.

## D-0249 Retain Until Fallback Retirement

Keep these paths until a separate D-0253 fallback-retirement authorization has
passed with candidate soak, replacement-test evidence, rollback-strategy
evidence, product sovereignty, typecheck, runtime build, and explicit operator
approval. They are no longer evidence that TypeScript is the default runtime:

| Area | Retained path | Protection |
| --- | --- | --- |
| Production runtime | `packages/runtime` | `npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` |
| Public CLI | `packages/runtime` build output for `analytix serve` | `npm run build:runtime` |
| Desktop adapter fallback | `src/main/runtime/analytix-adapter.ts` TypeScript startup path | `npm run test -- src/main/runtime/analytix-adapter.test.ts --run` |
| Preload bridge | `src/preload/index.ts` and `window.analytix` only | `npm run test -- src/preload/preload-runtime-request.test.ts src/preload/preload-sandbox.test.ts --run` |
| Settings schema | top-level `runtime` settings in `src/shared` | `npm run test -- src/shared/app-settings.test.ts --run` |
| Renderer route surface | Workbench, Sidebar, FloatingComposer product entries | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/chat/FloatingComposer.test.ts --run` |
| Product sovereignty | No Reasonix public protocol or renderer Go switcher | `npm run scan:product-sovereignty` |

The default backend is Go. Missing, skipped, fixture-only, or uncredentialed
D-0251/D-0252/D-0253 live evidence blocks live-evidence claims and fallback
physical retirement only; it does not move the default runtime back to
TypeScript.

## D-0249 Current Topology Landing

D-0249 has a code-level integration topology. Its historical wording was not
default-backend authorization by itself; current default authorization comes
from the later deterministic Go delivery pass.

Evidence now lives in:

```text
packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go
packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go
packages/runtime-go/d0249_reasonix_integration_topology_test.go
packages/runtime/src/contracts/capabilities.ts
packages/runtime/tests/go-runtime-conformance.test.ts
src/main/runtime/analytix-adapter.ts
docs/analytix/upstreams/reasonix-integration-topology.md
```

The topology covers Reasonix DeepSeek cache/prefix telemetry, job/sub-agent
lineage, MCP lifecycle/search/call/reconnect/redaction, AutoResearch project
state, and Workflow/Create Loop planner discipline through existing Analytix
Chat, `/goal`, tool, MCP registry, approval/user-input, runtime event/SSE,
provider adapter, and job-lineage contracts.

Do not use `reasonixIntegrationTopologyD0249.goDefaultCutoverCandidate` as a
desktop startup gate. Current desktop startup uses `go-runtime-default` unless
an unsupported backend is requested. The older `go-runtime-candidate` gates are
retained for legacy evidence collection and post-cutover live validation.

## Formal Runtime-Go Evidence Intake

Code-stage engine absorption is proven by source comparison, deterministic
contracts, runtime contracts, product-regression tests, product-sovereignty
scan, typecheck, runtime build, and preflight. It does not require a long-cycle
large-model benchmark.

Real provider, MCP, packaged desktop, and final operator evidence is now
required only for post-cutover live validation, candidate/soak claims, and
future fallback physical retirement.

Current commands and external inputs:

```text
npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate
npm run runtime:go:packaged-gui-smoke -- --json
npm run runtime:go:packaged-soak -- --json --actual
ANALYTIX_RUNTIME_READY=1
ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1
```

Provider evidence must contain passed credentialed probes for DeepSeek and at
least one non-DeepSeek provider family: OpenAI-compatible,
Anthropic-compatible, or custom endpoint. Evidence must include endpoint
family, endpoint format, request shape, stream parsing,
usage parsing, cache telemetry, error handling, and redaction fields. Custom
endpoint evidence must prove `custom-full-endpoint-no-append`; DeepSeek cache
telemetry must stay provider-scoped. MCP evidence must contain a passed
credentialed execution probe covering MCP connect, tool discovery/search, tool
call, approval/user-input, reconnect, and redaction. Packaged QA/soak evidence
must prove real packaged app startup or an explicitly marked deterministic
contract-only state; final acceptance requires actual packaged evidence where
the gate says it is required. Operator evidence must come through the formal
runtime-go live-evidence/default-readiness-report path and record the explicit env
gate, credentialed evidence review, default-candidate approval, target commit,
canonical evidence digests, report paths, and evidence-reviewed rows.
Evidence JSON must be auditable and must not include API keys, bearer tokens,
runtime tokens, passwords, or secret query material.

When these inputs are unavailable, D-0250B/D-0251/D-0252/D-0253 reports may
record `live_blocked` or `blocked` post-cutover validation. Do not fabricate
pass evidence and do not use missing external evidence to undo the deterministic
Go default path.

## D-0250 Delete After Real G6 Pass

Delete only after D-0253 fallback-retirement authorization has passed and a
packaged desktop QA/soak report proves startup, thread list, turn create, SSE
replay, rollback behavior, replacement-test coverage, and operator approval.

1. Remove superseded D-0241 internal candidate routes under
   `/v1/internal/go-production-candidate/*` after their provider, durable,
   approval/user-input, MCP, and job-lineage evidence is covered by Go runtime
   contract tests and D-0248 evidence.
2. Remove fixture-only `/v1/conformance/kernel/*` routes after equivalent Go
   runtime contract and packaged QA evidence is current.
3. Remove shadow-only G5 replay fixtures only after the Go runtime server owns
   the same event, goal, approval, user-input, MCP, and job lineage contracts.
4. Remove transition env gates for conformance sidecars after the desktop
   adapter has one production backend path and a separate rollback procedure.
5. Remove TypeScript runtime default startup only after the retained-path tests
   above are replaced by Go-default equivalents and a migration note records
   the rollback plan.

Protection tests to keep while deleting:

```text
git diff --check
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
npm --prefix packages/runtime run test -- tests/contracts.test.ts tests/runtime-event-reducer.test.ts tests/goal-tools.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/preload/preload-runtime-request.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings.test.ts --run
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/chat/FloatingComposer.test.ts --run
npm run scan:product-sovereignty
npm run typecheck
npm run build:runtime
npm run runtime:go:preflight -- --json --gate
npm run runtime:go:release-gate
npm run runtime:go:engine-absorption-report -- --json
```

## D-0253 Structured Fallback Retirement Checklist

Each retained path must keep an explicit replacement-test, rollback-strategy,
operator approval, and fallback removal policy before any fallback deletion
work can be authorized.

| retainedPath | category | replacementTestId | rollbackEvidenceId | operatorReviewId | fallbackRemovalAllowed |
| --- | --- | --- | --- | --- | --- |
| `packages/runtime` | `typescript-runtime-fallback` | `go-default-runtime-conformance` | `pre-retirement-typescript-rollback-evidence` | `operator-retirement-approval` | `after-d0253-authorization` |
| `src/main/runtime/analytix-adapter.ts` | `desktop-runtime-adapter` | `adapter-go-default-tests` | `pre-retirement-typescript-rollback-evidence` | `operator-retirement-approval` | `after-d0253-authorization` |
| `src/preload/index.ts` | `window-analytix-bridge` | `preload-settings-tests` | `data-settings-events-compatibility-evidence` | `operator-retirement-approval` | `never-remove` |
| `src/shared/app-settings-runtime.ts` | `top-level-runtime-settings` | `preload-settings-tests` | `data-settings-events-compatibility-evidence` | `operator-retirement-approval` | `never-remove` |
| `src/renderer/src/agent/analytix-runtime.ts` | `renderer-runtime-client` | `renderer-route-chat-tests` | `data-settings-events-compatibility-evidence` | `operator-retirement-approval` | `never-remove` |

## Current Engine Absorption Result

- Engine absorption code-stage is reported by
  `npm run runtime:go:engine-absorption-report -- --json`.
- The superiority matrix has 13 deterministic rows:
  `absorb=1`, `adapt=5`, `keep-kun=5`, `reject=1`, `defer=1`.
- Reasonix is absorbed/adapted only for engine/runtime capabilities; Analytix/Kun
  keeps product entry, UI, settings, bridge, provider endpoint family, MCP
  search/meta-tool boundary, and durable SSE/thread routes.
- The report remains engine/runtime-only and does not create Reasonix product
  entrypoints, public routes, settings roots, or renderer surfaces.
- TypeScript executable runtime fallback is retired; `ANALYTIX_RUNTIME_BACKEND=typescript`
  is a retired-backend diagnostic, not an execution path.

## Current Post-Cutover Live Validation Result

- The current live evidence collector is
  `npm run runtime:go:live-evidence -- --json`.
- The aggregate live evidence lives under
  `docs/analytix/upstreams/runtime-go-live-evidence/`.
- The 2026-06-29 current-worktree live evidence report is `passed`. Provider,
  MCP, packaged desktop QA, packaged GUI bridge smoke, packaged session soak,
  and operator approval all passed with `finalGateBlocked:false`.
- The current final gate has no evidence-hygiene blocker after the relevant
  evidence and gate changes were intentionally staged for review.
- The operator input has been satisfied for the current staged-worktree release
  gate. This authorizes the current release gate result only; it does not
  require or restore a TypeScript executable rollback path.

## Current Candidate Gate

- `npm run runtime:go:default-readiness-report -- --gate --json` is the formal
  candidate gate.
- The report requires passed live evidence, strict preflight, engine absorption
  evidence, and operator candidate approval before it will emit final
  candidate/soak evidence.
- The report revalidates provider protocol fields, credentialed MCP probe
  details, packaged process/runtime/rollback evidence bindings, operator report
  paths, operator evidence-reviewed rows, canonical evidence digests, and
  target commit coverage; minimal top-level `passed:true` JSON is not candidate
  evidence.
- The report records `goDefaultBackendEnabled:true`,
  `typeScriptFallbackRetained:false`, `rendererVisibleGoSwitcher:false`, and the
  retired-backend diagnostic for `ANALYTIX_RUNTIME_BACKEND=typescript`.
- With `ANALYTIX_RUNTIME_READY=1` and
  `ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1`, the current candidate gate
  passed with `goDefaultReady:true`, `finalAcceptanceReady:true`,
  `finalGateBlocked:false`, and no final gate blockers.
- `npm run runtime:go:release-gate` also passed product sovereignty,
  `runtime:go:preflight -- --json --gate`, and
  `go test -tags analytix_prod ./...` for the current staged worktree.

## Current Rollback-Retirement Gate

- `npm run runtime:go:rollback-retirement-evidence -- --json` is the current
  evidence command. It reruns the cutover report and Electron adapter
  retired-backend tests. It does not synthesize legacy soak/operator evidence,
  start TS runtime, or print credentials.
- `npm run runtime:go:rollback-retirement-report -- --json` is the current
  report command. It requires the rollback evidence command plus packaged QA to
  pass, and reports production fixture/legacy retirement counters from the
  formal runtime-go validation path.
- The report records `defaultPathFallbackRetired:true`,
  `explicitTypeScriptOverrideRetained:true`, and
  `typeScriptCodePathDeleted:false`. Physical deletion of explicit TypeScript
  rollback code remains a separate release decision after product, packaged,
  and operator acceptance.
- Historical `docs/analytix/upstreams/d0253-retirement/*` files are archived
  evidence from the earlier staged retirement plan. They are not current
  executable validation entrypoints and must not be used to claim production
  parity.
- The current release gate operator evidence has passed for the current
  staged-worktree validation path. This still treats historical rollback
  evidence as archive material rather than a current executable fallback.
