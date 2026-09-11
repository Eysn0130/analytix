# Release evidence gate - 2026-06-20

Scope:

Post-4f515da currentness closeout for
`codex/reasonix-goal-control-delta-oracle` at `4f515da`, followed by a release
evidence gate audit and a post-`4f47031f` upstream drift recheck. This report
does not claim release readiness.

## Upstream currentness

| Source | Ref | Current result | Status |
| --- | --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` | checked |
| Kun | `develop` | `9605e20f422c90054d930e4f1a0000886b353895` | checked |
| Kun | `v0.2.13` | tag `201a1469ffbd911f6b95d450b78471e51b42acae` | checked |
| Kun | `v0.2.14` | tag `06be05d76223208724c07301fa0f830641a06e6f` | checked |
| Reasonix | `main-v2` | `48e5b990671ca1e579b895080e74babc1e17c333` | checked |

Reasonix moved past the post-4f515da `bc8249c3` baseline. The only substantive
store-sidecar commit before `d02457ee` was `98a57ded`, which centralizes
Reasonix session sidecar path derivation in `internal/store`; it remains
future Go store-boundary guidance only. The later substantive delta is
`3e625b91`, merged by `48e5b990`, which introduces Reasonix SessionAPI /
driving-port interface segregation and migrates the bot gateway to
`Lifecycle + TurnControl + Approvals`. That control-port boundary is a
`should absorb` next batch for analytix, but no current TypeScript runtime,
renderer protocol, settings schema, bridge, CLI, Go scaffold, or Rust/Tauri
path was added by this report. Kun `develop` also advanced past stable master
with `d09d52b`, a Windows installer process-stop fix. A later P0
product-entry/installer batch absorbed that behavior as analytix-native NSIS
logic using `ANALYTIX_*` naming; Windows machine upgrade QA remains required.

2026-06-21 addendum: a fresh upstream recheck found Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e`, Kun `develop`
`247076f297170c3d0c558baffb073c629894faea`, Kun `v0.2.13` / `v0.2.14` tags
`201a1469ffbd911f6b95d450b78471e51b42acae` /
`06be05d76223208724c07301fa0f830641a06e6f`, and Reasonix `main-v2`
`49c14762b7da9234525e717830e39a64a2220911`. Historical Reasonix `5d1ad2a`
is no longer described as latest; it remains record-only drift before the
current `49c14762` input.

## Validation

| Gate | Evidence | Result |
| --- | --- | --- |
| Runtime full suite | `npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1` | pass: 70 files, 681 tests |
| Runtime typecheck | `npm --prefix packages/runtime run typecheck` | pass |
| App typecheck | `npm run typecheck` | pass |
| Focused main/renderer tests | `npm run test -- src/main/analytix-regression.test.ts src/main/settings-store.test.ts src/main/ipc/app-ipc-schemas.test.ts src/main/analytix-runtime-supervisor.test.ts src/main/provider-connection.test.ts src/main/upstream-models.test.ts src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/agent/analytix-mapper.test.ts src/renderer/src/thread/thread-long-thread-smoke.test.ts` | pass: 9 files, 138 tests |
| macOS arm64 package path | `npm run dist:mac:arm64` | pass: built `dist/analytix-0.1.0-mac-arm64.dmg` and `dist/analytix-0.1.0-mac-arm64.zip`; signing and notarization skipped |
| Whitespace | `git diff --check` | pass |
| Forbidden production identity/protocol scan | `rg -n "window\\.kunGui|agents\\.kun|kun serve|KUN_|Reasonix-native|reasonix-native|packages/runtime-go|Cargo\\.toml|\\bTauri\\b|\\btauri\\b|Go backend scaffold|Rust/Tauri" src packages/runtime/src packages/runtime/package.json package.json electron-builder.config.cjs scripts .github build --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no production hits |
| Go/Rust runtime file scan | `rg --files -g '!node_modules' -g '!dist' -g '!out' -g '!docs/legacy/**' \| rg '(^|/)(packages/runtime-go|go\\.mod|go\\.sum|Cargo\\.toml|Cargo\\.lock|src-tauri)(/|$)|\\.(go|rs)$'` | pass: no hits |
| Preserved analytix surface scan | `rg` over production paths for `window.analytix`, `analytix serve`, `ANALYTIX_*`, and `packages/runtime` | pass: expected current analytix surfaces present |
| P2.2 cache/provider fixture proof | `npm --prefix packages/runtime run test -- tests/provider-cache-proof.test.ts tests/cache.test.ts tests/usage-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files, 43 tests; proves stable prefix/tool hash, provider/model/endpoint attribution, DeepSeek/Responses/Anthropic cache parsing, and unsupported-provider no-guess fallback |

2026-06-21 validation addendum:

| Gate | Evidence | Result |
| --- | --- | --- |
| Upstream currentness | `git ls-remote` for Kun `master` / `develop` / `v0.2.13` / `v0.2.14` and Reasonix `main-v2` | pass: Kun `8602476` / `247076f` / `201a146` / `06be05d`; Reasonix `49c14762` |
| Route-surface guards | `npm run test -- src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts -- --runInBand` | pass: 3 files, 9 tests |
| Runtime full suite | `npm --prefix packages/runtime test` | pass: 74 files, 707 tests |
| Runtime cache/model focused suite | `npm --prefix packages/runtime run test -- tests/model-client.test.ts tests/provider-cache-proof.test.ts tests/cache.test.ts tests/usage-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files, 88 tests |
| Runtime loop cache focused suite | `npm --prefix packages/runtime run test -- tests/loop.test.ts -t "cache" --no-file-parallelism --maxWorkers=1` | pass: 1 test, 71 skipped |
| Runtime typecheck | `npm --prefix packages/runtime run typecheck` | pass |
| App typecheck | `npm run typecheck` | pass |
| App test suite | `npm run test` | pass: 199 files, 1442 tests |
| Runtime build | `npm run build:runtime` | pass |
| App build | `npm run build` | pass |
| Lint | `npm run lint` | pass with 0 errors and 14 existing `react-hooks/exhaustive-deps` warnings |
| Whitespace | `git diff --check` | pass |
| Bridge domain scan | `rg -n "window\\.analytix\\.(?!settings|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|updates|logs|app|diagnostics)\\b|window\\.analytix\\?\\.(?!settings|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|updates|logs|app|diagnostics)\\b" src/renderer/src src/preload src/shared --pcre2` | pass: no hits |
| Forbidden identity/protocol scan | `rg -n "window\\.kunGui|agents\\.kun|kun serve|KUN_|Reasonix-native|reasonix-native|packages/runtime-go|Cargo\\.toml|\\bTauri\\b|\\btauri\\b|Go backend scaffold|Rust/Tauri" src packages/runtime/src packages/runtime/package.json package.json electron-builder.config.cjs scripts .github build --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no hits |
| Go/Rust scaffold scan | `rg --files ... | rg '(^|/)(packages/runtime-go|go\\.mod|go\\.sum|Cargo\\.toml|Cargo\\.lock|src-tauri)(/|$)|\\.(go|rs)$'` | pass: no hits |
| D-0014 Batch 1 focused runtime proof | `npm --prefix packages/runtime test -- goal-tools.test.ts builtin-tools.test.ts child-agent-executor.test.ts loop.test.ts` | pass: 5 files, 129 tests; proves `complete_step` evidence ledger, evidence-gated goal completion, denied no-execute, ask/auto/yolo posture mapping, plan/tool split, blocked-state event/status, and headless/subagent policy inheritance |
| D-0014 Batch 2 focused runtime proof | `npm --prefix packages/runtime test -- autoresearch-store.test.ts goal-tools.test.ts http-server.test.ts loop.test.ts` | pass: 6 files, 130 tests; proves `/goal --research`, project-local AutoResearch state, restart/resume, `record_research_direction`, `complete_step requirement_id`, requirement completion gate, and stable-prefix isolation |
| D-0014 Batch 2 focused renderer proof | `npm run test -- src/renderer/src/components/chat/FloatingComposer.test.ts src/renderer/src/store/chat-store-maintenance-actions.test.ts src/renderer/src/agent/analytix-mapper.test.ts` | pass: 3 files, 97 tests; proves existing chat/goal command and renderer projection carry research metadata without adding a top-level UI route |
| D-0014 Batch 3 dynamic tool-source proof | `npm --prefix packages/runtime test -- capability-registry.test.ts cache.test.ts mcp-tool-provider.test.ts loop.test.ts` | pass: 6 files, 134 tests; proves `connect_tool_source` semantics through dynamic registry connect/disconnect, connection-order-stable canonical catalog fingerprints, provider-owned ordering, source diagnostics, and cache diagnostics that keep source lifecycle outside `prefixChanged` |
| D-0014 Batch 3 context-economy regression proof | `npm --prefix packages/runtime test -- token-economy.test.ts request-history-hygiene.test.ts context-compactor.test.ts compaction-history.test.ts memory-store.test.ts usage-service.test.ts provider-cache-proof.test.ts` | pass: 8 files, 44 tests; proves token economy, request-history hygiene, compaction archive/history, memory retrieval, usage accounting, multi-provider cache telemetry, and unsupported-provider unknown fallback remain green |
| D-0014 Batch 3 renderer diagnostics proof | `npm run test -- src/renderer/src/agent/analytix-mapper.test.ts` | pass: 1 file, 33 tests; renderer mapping remains compatible with optional cache diagnostic fields |
| D-0014 Batch 4 delegated collaboration proof | `npm --prefix packages/runtime test -- delegation-runtime.test.ts child-agent-executor.test.ts loop.test.ts runtime-event-reducer.test.ts` | pass: 5 files, 102 tests; proves `delegate_task` fan-out, maxParallel queueing, queued abort/failure/interruption paths, headless child execution, active parent Goal evidence handoff, and nested event reducer evidence metadata |
| D-0014 Batch 4 renderer projection proof | `npm run test -- src/renderer/src/agent/analytix-mapper.test.ts` | pass: 1 file, 33 tests; proves nested child metadata including `evidenceLedgered` survives renderer projection without a new top-level UI |
| D-0014 Batch 5 G0/G1 shadow conformance proof | `npm --prefix packages/runtime test -- autoresearch-store.test.ts go-runtime-conformance.test.ts` | pass: 2 files, 5 tests; proves the ten Go kernel components are mapped to TS oracle tests, G1 `/health` / `/v1/runtime/info` / `/v1/runtime/tools` shadow route contracts stay analytix-owned, no Go default/backend/protocol is implied, and unknown AutoResearch requirement evidence is rejected |
| Final focused runtime proof refresh | `npm --prefix packages/runtime test -- goal-tools.test.ts builtin-tools.test.ts child-agent-executor.test.ts loop.test.ts autoresearch-store.test.ts http-server.test.ts capability-registry.test.ts cache.test.ts mcp-tool-provider.test.ts token-economy.test.ts request-history-hygiene.test.ts context-compactor.test.ts compaction-history.test.ts memory-store.test.ts usage-service.test.ts provider-cache-proof.test.ts delegation-runtime.test.ts runtime-event-reducer.test.ts go-runtime-conformance.test.ts` | pass: 23 files, 289 tests |
| Final focused renderer proof refresh | `npm run test -- src/renderer/src/components/chat/FloatingComposer.test.ts src/renderer/src/store/chat-store-maintenance-actions.test.ts src/renderer/src/agent/analytix-mapper.test.ts src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts src/renderer/src/thread/thread-long-thread-smoke.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/Workbench.route-surface.test.ts` | pass: 8 files, 142 tests |

2026-06-21 latest-currentness / production-scan addendum:

| Gate | Evidence | Result |
| --- | --- | --- |
| Reasonix latest currentness | `git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2` | pass: `91fe06db6177bb052fc8ae1a3081d60bfc104a4e` |
| Reasonix latest delta | `git log --oneline 49c14762b7da9234525e717830e39a64a2220911..analysis/main-v2`; `git diff --stat 49c14762b7da9234525e717830e39a64a2220911..analysis/main-v2` in `/Users/sun/Projects/DeepSeek-Reasonix` | pass: substantive commits are `45367085` codebase-memory MCP auto-indexing and `d9e453a1` MiMo built-ins -> custom providers; 32 files changed |
| Kun baseline currentness | `git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14` | pass: `master` `8602476`, `develop` `247076f`, `v0.2.13` `201a146`, `v0.2.14` `06be05d` |
| G1/G2 TS oracle refresh | `npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts tests/provider-cache-proof.test.ts tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files, 10 tests; covers G2 route replay, approval/user-input route parity, provider-cache oracle, and MCP/tool lifecycle oracle |
| G1/G2 Go package refresh | Downloaded official `go1.26.4.darwin-arm64.tar.gz` from `go.dev`, verified SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53`, then ran `cd packages/runtime-go && go test ./...` with the temporary toolchain | pass: `github.com/analytix/runtime-go`; temporary toolchain removed |
| Product-entry guards | `npm run test -- src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts -- --runInBand` | pass: 3 files, 9 tests |
| Bridge domain scan | `rg -n "window\\.analytix\\.(?!settings|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|updates|logs|app|diagnostics)\\b|window\\.analytix\\?\\.(?!settings|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|updates|logs|app|diagnostics)\\b" src/renderer/src src/preload src/shared --pcre2` | pass: no hits |
| Forbidden production identity/protocol scan | `rg -n "window\\.kunGui|agents\\.kun|kun serve|KUN_|Reasonix-native|reasonix-native|Cargo\\.toml|\\bTauri\\b|\\btauri\\b|Rust/Tauri" src packages/runtime/src packages/runtime/package.json package.json electron-builder.config.cjs scripts .github build --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no hits |
| Go/Rust file scope scan | `rg --files -g '!node_modules' -g '!dist' -g '!out' -g '!docs/legacy/**' \| rg '(^|/)(go\\.mod|go\\.sum|Cargo\\.toml|Cargo\\.lock|src-tauri)(/|$)|\\.(rs)$'` | expected: `packages/runtime-go/go.mod` only; no Cargo/Rust/Tauri files |
| Default Go/backend scan | `rg -n "default Go backend|renderer-visible Go route|backend switch|Electron supervisor|runtime-go|packages/runtime-go|go\\.mod" src src/preload src/shared electron-builder.config.cjs package.json scripts .github --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no hits in production app paths |
| G2 shadow replay evidence | `packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json`; `packages/runtime/tests/go-runtime-conformance.test.ts`; `packages/runtime-go/shadow_g2.go`; `packages/runtime-go/shadow_test.go` | pass: TypeScript HTTP/SSE contracts and Go shadow replay both consume the same oracle; still shadow-only and not a default backend |
| Top-level Workflow route scan | `rg -n "\\bworkflow\\b|openWorkflow" src/renderer/src/components/chat/Sidebar.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/store/chat-store-types.ts src/renderer/src/store/chat-store-app-actions.ts` | pass: no hits |
| Provider/live release env scan | checked presence only for `DEEPSEEK_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `MIMO_API_KEY`, `MINIMAX_API_KEY`, `ANALYTIX_RELEASE_BASE_URL`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER`, `CSC_LINK` | blocked: all checked env vars missing |
| Release metadata scan | `rg -n "https://release-domain\\.invalid/analytix|repository|homepage" electron-builder.config.cjs package.json` | blocker: placeholder release URL plus `homepage: "http://TBD"` and `repository.url: "TBD"` remain |
| Whitespace | `git diff --check` | pass |
| Stage artifact scope | `git status --short --branch`; diff review | G2/parity fixtures and tests are stage artifacts; unrelated renderer toolbar work is left outside this stage commit scope |

This addendum does not close release readiness. It updates the currentness and
production-scan evidence after Reasonix latest moved to `91fe06db`, and it
keeps live provider matrix, packaged QA, release metadata, signing/notarization,
Windows NSIS, and verified remote as blockers.

## Dirty worktree ownership checkpoint

The current dirty tree is validated locally but should not be committed as one
large change. Several authority documents contain overlapping hunks for P0,
cache/provider proof, five-batch governance, Batch 1-5, and release evidence.

| Theme | Primary files | Commit guidance |
| --- | --- | --- |
| P0 Workflow entry correction | `src/renderer/src/components/Workbench.tsx`, `src/renderer/src/components/chat/Sidebar.tsx`, `src/renderer/src/store/chat-store-app-actions.ts`, route-surface tests, affected specs/ledgers/scorecard | Commit separately from runtime kernel work if partial doc hunks can be isolated. |
| Kun installer process-stop | `electron-builder.config.cjs`, `build/installer.nsh`, Kun sync/ledger/scorecard evidence | Commit with Windows installer docs only; Windows machine QA remains blocker. |
| Reasonix cache/provider proof | `packages/runtime/src/adapters/model/compat-model-client.ts`, cache/usage files, `provider-cache-proof.test.ts`, cache/scorecard docs | Commit after provider/cache-focused staging and validation. |
| Agent-kernel governance | `重构升级方案.md`, specs 08/09, upstream target/blueprint/plan/docs | Docs-only commit if separated from implementation evidence hunks. |
| Batch 1 permission/evidence/Goal | `goal-tools.ts`, `thread-service.ts`, `threads.ts`, `goal-control-machine.ts`, Batch 1 tests, renderer goal contract/mapper | Implementation commit must include shared contract + runtime + renderer mapper/tests together. |
| Batch 2 AutoResearch | `packages/runtime/src/research/autoresearch-store.ts`, `goal-tools.ts`, `thread-service.ts`, `http-server.test.ts`, `autoresearch-store.test.ts`, renderer goal command tests | Shares `goal-tools.ts` and `thread-service.ts` with Batch 1; use patch staging or combine with Batch 1. |
| Batch 3 tool/context economy | `capability-registry.ts`, cache diagnostics, tool catalog fingerprint, usage counter, capability/cache/usage tests, renderer mapper optional diagnostics | Runtime-focused commit after multi-provider cache tests. |
| Batch 4 delegated collaboration evidence | delegation runtime/provider, event contract/reducer, runtime factory, renderer child metadata mapper/tests | Runtime + renderer projection commit; do not include task/parallel productization claims. |
| Batch 5 G0/G1 shadow conformance | `packages/runtime/src/conformance/go-runtime-kernel-conformance.ts`, `go-runtime-conformance.test.ts`, conformance/scorecard docs | TS-only conformance commit; no Go scaffold or backend switch. |
| Connect Phone / Sidebar / Write UI | Connect Phone, Sidebar, Workbench top bar, Write Workspace, locales, CSS, write tests | Separate renderer/UI commit; not part of Reasonix kernel proof. |

## Spec 07 blockers

| Release evidence item | Current status | Release implication |
| --- | --- | --- |
| Live rewind apply | Automated checkpoint rewind apply tests pass through the runtime suite, and prior P4C desktop smoke proved startup/runtime health. No live desktop UI click-through of a real rewind apply control was executed in this pass. | blocker |
| Crash/restart recovery | Runtime apply tests cover rescue/audit behavior, but no packaged crash/restart recovery drill was executed in this pass. | blocker |
| Packaged QA | macOS arm64 dmg/zip packaging passed locally. The packaged app was not launched from the artifact in this pass; macOS x64, Linux AppImage, and Windows NSIS were not built here. | partial/blocker |
| Live provider matrix | Provider/static tests and fixture-backed cache/provider proof passed, but no credentialed live provider matrix or live cost reconciliation was run. | blocker |
| Release URL | `electron-builder.config.cjs` still defaults to `https://release-domain.invalid/analytix`. | blocker |
| Repository metadata | `package.json` still has `homepage: "http://TBD"` and `repository.url: "TBD"`. | blocker |
| Signing/notarization | Local env check showed no mac signing or Apple notarization credentials; the mac packaging log skipped signing and notarization. | blocker |
| Windows NSIS | NSIS x64 remains configured, but this macOS pass cannot verify Windows runtime behavior. | blocker |
| Verified analytix remote | `legacy-origin` fetch and push both still point to `https://github.com/KunAgent/Kun.git`. No verified analytix remote is configured. | push blocker |

## Closure status

| Closure type | Status | Reason |
| --- | --- | --- |
| Scoped currentness closure | pass after currentness patch plus 2026-06-21 addendum | Required docs now distinguish latest Reasonix remote `91fe06db`, previous governance baseline `49c14762`, historical `5d1ad2a`, post-4f515da baseline `bc8249c3`, record-only store-sidecar `d02457ee`, and historical P3A checkpoint `be67a498`; validation passed for the scoped cache/provider fixture gate. |
| Release closure | not closed | Spec 07 blockers remain: live apply, crash/restart, packaged-app launch matrix, live providers, release URL/repo metadata, signing/notarization, Windows NSIS, and verified remote. |
| Final product closure | not closed | Full Reasonix parity / superiority is not closed; Go G1-G6, release QA, live provider evidence, and final product readiness gates remain incomplete. |

## Upstream remaining gaps

| Upstream | Remaining gap before broader closure |
| --- | --- |
| Kun 0.2.13 | Keep lineage-regression coverage and desktop QA evidence; do not regress Code/Write/SDD/Connect Phone/Schedule while release blockers remain open. |
| Kun 0.2.14/current | Scoped stable master drift is absorbed behind analytix contracts. The later P0 product-entry/installer batch absorbed the Kun Windows installer process-stop behavior as code/doc, while Windows machine NSIS upgrade QA remains incomplete. Full workflow builder parity, live Connect Phone QA, full packaged release QA, and public release metadata remain incomplete. |
| Reasonix `91fe06db` latest | Goal/control oracle is absorbed through `bc8249c3` and the control-port slice through `48e5b990` / `3e625b91`. ApprovalManager remains a deferred focused batch; `d02457ee` store-sidecar authority, historical `5d1ad2a`, previous `49c14762` governance baseline, and latest `91fe06db` MCP/provider drift are classified. This stage adds fixture-backed route/API/provider-cache/MCP-lifecycle hardening and Go G2 shadow route replay; live provider evidence remains missing. |

Next batch recommendation:

```text
D-0014 Batch 1-4 now have scoped TypeScript proof in this dirty worktree, and
Batch 5 now has an isolated `packages/runtime-go` G1/G2 shadow scaffold for
health/info/tools plus thread/session read-list/search/archive/fork/SSE replay
against TypeScript oracle fixtures. The next implementation boundary is G3/G4
provider/tool work only after a fresh currentness check and explicit fixture
scope. Do not create a default Go backend, Rust scaffold, Tauri migration,
renderer-visible Go route, Reasonix public protocol, or backend switch in the
mixed worktree, and do not claim release readiness until Spec 07 live QA,
packaged launch, provider, metadata, signing, Windows NSIS, and verified remote
blockers are closed.
```

Go/Rust order:

```text
Do not start Rust/Tauri. Batch 5 now has a G1 shadow package, not a default
backend. Future Go work must stay shadow-only and move next through G2
read/list/search/archive/fork/session/SSE replay against TypeScript oracle
fixtures before any G3+ provider/streaming work or backend-selection discussion.
```

## 2026-06-21 P0 engine parity + desktop hardening addendum

Scope:

```text
Reasonix P0 engine parity and Kun desktop hardening stage on
codex/reasonix-goal-control-delta-oracle. This addendum records stage evidence
only; it does not claim release readiness.
```

Validation recorded before final full-gate rerun:

| Gate | Evidence | Result |
| --- | --- | --- |
| Dirty baseline handling | initial Write toolbar hover diff was validated and committed separately; later shell navigation/update-action dirty baselines were validated with focused tests and committed separately | pass |
| Provider/cache P0 focused suite | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/model-client.test.ts tests/usage-service.test.ts tests/cache.test.ts --no-file-parallelism --maxWorkers=1` | pass |
| MCP known override/lifecycle focused suite | `npm --prefix packages/runtime test -- tests/mcp-tool-provider.test.ts tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-config.test.ts tests/capability-registry.test.ts --no-file-parallelism --maxWorkers=1` | pass |
| Task/background/planner oracle focused suite | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/delegation-runtime.test.ts tests/child-agent-executor.test.ts --no-file-parallelism --maxWorkers=1` | pass |
| Kun SSE IPC focused suite | `npm run test -- src/main/runtime-sse-ipc.test.ts` | pass |
| Go G3/G4 TS conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass |
| Go shadow package | official temporary `go1.26.4.darwin-arm64` from `go.dev`, SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53` verified, then `cd packages/runtime-go && go test ./...` | pass; toolchain not committed |
| Renderer shell hardening | `npm run test -- src/renderer/src/components/shell/ShellNavigationControls.test.tsx src/renderer/src/components/write/WriteWorkspaceToolbar.test.ts src/renderer/src/components/chat/ConnectPhoneView.test.ts src/renderer/src/components/Workbench.route-surface.test.ts` | pass |
| Whitespace during stage | `git diff --check` before docs update | pass |

Final command gate after docs update:

| Gate | Evidence | Result |
| --- | --- | --- |
| Runtime full suite | `npm --prefix packages/runtime test` | pass: 78 files, 718 tests |
| App full suite | `npm run test` | pass: 203 files, 1453 tests; only Node `punycode` deprecation warnings observed |
| App typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go G3/G4 shadow package | official temporary `go1.26.4.darwin-arm64` from `go.dev`, SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53` verified, then `cd packages/runtime-go && go test ./...` | pass; toolchain removed after validation |
| Whitespace | `git diff --check` | pass |
| Workflow top-level production scan | `rg -n "\\bworkflow\\b|openWorkflow|WorkflowCreateLoopView" src/renderer/src/components/chat/Sidebar.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/store/chat-store-types.ts src/renderer/src/store/chat-store-app-actions.ts` | pass: no hits |
| Kun/Reasonix identity/protocol production scan | `rg -n "window\\.kunGui|agents\\.kun|kun serve|KUN_|Reasonix-native|reasonix-native|Reasonix public protocol" src packages/runtime/src packages/runtime/package.json package.json electron-builder.config.cjs scripts .github build --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no hits |
| Go/Rust/Tauri/default backend production scan | `rg -n "default Go backend|renderer-visible Go route|backend switch|Electron supervisor|runtime-go|packages/runtime-go|go\\.mod|Cargo\\.toml|Cargo\\.lock|\\bTauri\\b|\\btauri\\b|src-tauri|Rust/Tauri" src src/preload src/shared electron-builder.config.cjs package.json scripts .github --glob '!**/*.test.ts' --glob '!**/*.test.tsx'` | pass: no hits |
| Go/Rust file-scope scan | `rg --files -g '!node_modules' -g '!dist' -g '!out' -g '!docs/legacy/**' \| rg '(^|/)(go\\.mod|go\\.sum|Cargo\\.toml|Cargo\\.lock|src-tauri)(/|$)|\\.(go|rs)$'` | expected: only `packages/runtime-go` shadow files; no Cargo/Rust/Tauri files |
| Docs/spec/ledger/scorecard consistency scan | checked P0/G3/G4/SSE/MCP/task/default-Go/Workflow language across `重构升级方案.md`, specs 08/09, upstream ledgers, Go conformance, scorecard, and this evidence gate | pass: all required artifacts contain the 2026-06-21 P0 closure evidence and retain release-readiness blockers |

Current blocker state:

| Blocker | Status |
| --- | --- |
| Live provider matrix | blocked: provider credentials not available in this environment. |
| Live MCP/indexer QA | not run; fixture/lifecycle proof only. |
| Packaged desktop QA | not run in this stage; required before release readiness. |
| Windows NSIS | not run; no Windows environment. |
| Signing/notarization | blocked by missing credentials. |
| Release metadata / verified remote | remains a release/push blocker until real analytix release URL/repository/remote are configured. |
| Go backend readiness | not ready; G1-G4 remain shadow/conformance-only, G5 pending. |

Closure wording:

```text
The scoped Reasonix P0 engine parity + Kun desktop hardening stage is closed by
fixture-backed runtime/provider/MCP/task/Go conformance, Kun SSE IPC hardening,
full command-gate validation, and production-route scans. Release readiness,
live provider/MCP parity, packaged QA, signing/notarization, Windows NSIS, and
default Go backend remain open next-stage gates.
```
