# Release evidence gate - 2026-06-21

Scope:
Reasonix 9e56 currentness, MCP startup parity, internal task-job runtime
routes, provider probe diagnostic safety, and shell native-titlebar safe-area
closure.

This gate is not a release-ready claim. Live provider/MCP QA, packaged desktop
QA, Windows NSIS verification, signing/notarization, release metadata, verified
remote, Go G5/full-loop parity, and default Go backend readiness remain open.

Current-status note (2026-07-02): this file is historical release-gate evidence.
Later Go-default and retired-backend decisions supersede its backend-readiness
wording. Top-level route absence here is a current-surface proof for that
stage, not a permanent ban on future analytix-native product entries approved
through specs, UX rationale, tests, desktop QA, and scan updates.

## Currentness

| Source | Evidence | Result |
| --- | --- | --- |
| Reasonix | `91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b` classified in `reasonix-sync.md`. | pass: todo commits record-only; MCP commits reimplemented behind analytix contracts. |
| Kun | Product-entry baseline remains v0.2.13 -> v0.2.14. | pass: no new Workflow/Create Loop route or Kun identity added. |

## Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| Shell safe-area baseline | `npm run test -- src/shared/window-chrome.test.ts src/renderer/src/AppShell.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/WindowsTitleBar.test.ts` | pass: 4 files / 9 tests. |
| MCP/task-job runtime proof | `npm --prefix packages/runtime test -- capability-registry.test.ts mcp-tool-provider.test.ts task-job-orchestration-oracle.test.ts` | pass: 4 files / 33 tests. |
| Provider probe diagnostic safety | `npm run test -- src/main/provider-connection.test.ts` | pass: 1 file / 9 tests. |

## Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 721 tests |
| App tests | `npm run test` | pass: 204 files / 1457 tests |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | not run in this stage because no Go files changed | n/a |
| Workflow top-level production scan | `rg "Workflow|openWorkflow|createLoop|create-loop" ...` across Sidebar, Workbench, route/store files | pass: no hits |
| Kun/Reasonix identity/protocol scan | production path scan for Kun bridge/identity and Reasonix public protocol terms | pass: no hits |
| Go/Rust/Tauri/default backend scan | production path scan for runtime-go/default backend/Rust/Tauri terms | pass with known benign pre-existing comment: `src/main/services/worktree-service.ts` mentions replacing TalkCody's Rust lock map; no backend/default path hit |
| Docs/spec/ledger/scorecard consistency scan | docs grep for 9e56, task-job routes, MCP retry, and boundary terms | pass |

## Accepted Claims

```text
Reasonix 9e56 currentness is classified.
MCP startup fixture parity now includes retry-all failed startup servers and
late-provider suspension.
Task jobs now have authenticated internal runtime wait/output/kill routes.
Provider probe/model-list diagnostics redact secret-bearing URL/body/error text.
Kun 0.2.13 -> 0.2.14 product-entry baseline remains intact.
```

## Rejected Claims

```text
No full Reasonix task/parallel/planner parity.
No live provider/cache superiority.
No live MCP/indexer parity.
No Local Whisper/tray parity.
No release readiness.
No default Go backend.
```

## Collaborative Execution / Cache / MCP / Go G5 Addendum

Scope:
Post-9e56 dirty shell closure, Reasonix collaborative-execution contract,
offline provider cache curve guard, MCP/indexer lifecycle fixture, Go G5
TS-owned oracle inventory, and Kun desktop baseline boundary.

This is still not a release-ready claim. Live provider/MCP QA, packaged
desktop QA, Windows NSIS verification, signing/notarization, release metadata,
verified remote, Local Whisper/tray QA, Go full-loop parity, and default Go
backend readiness remain open.

## Addendum Currentness

| Source | Evidence | Result |
| --- | --- | --- |
| Reasonix | local `/Users/sun/Projects/DeepSeek-Reasonix` was fetched read-only; `origin/main-v2` and remote `refs/heads/main-v2` resolve to `9e56c3276880ced538b3375329b8a0ebbe63a67b`. | pass: no newer Reasonix drift beyond the classified 9e56 set. |
| Kun | refs rechecked: `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master` `8602476`, `develop` `247076f`. | pass: product-entry baseline remains v0.2.13 -> v0.2.14. |

## Addendum Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| Dirty shell safe-area closure | `git diff --check`; `npm run test -- src/shared/window-chrome.test.ts src/main/window-chrome-config.test.ts src/renderer/src/components/Workbench.route-surface.test.ts`; `npm run typecheck` | pass: shell baseline was reviewed and committed separately before runtime work continued. |
| Collaborative/cache/MCP/Go focused proof | `npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts provider-cache-proof.test.ts mcp-tool-lifecycle-oracle.test.ts go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts` | pass: 5 files / 15 tests. |

## Addendum Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 723 tests |
| App tests | `npm run test` | pass: 205 files / 1458 tests |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | not run in this addendum because no Go files changed | n/a |
| Workflow/Create Loop/Subagent/AutoResearch top-level production scan | targeted route/store scan for `route === 'workflow'`, `openWorkflow`, `WorkflowCreateLoopView`, and related top-level route hooks | pass: only negative test assertions / hidden future internals; no top-level production route or sidebar entry restored |
| Kun identity / Reasonix public protocol scan | production scan for Kun bridge/identity and Reasonix public protocol terms | pass with known compatibility-only references: legacy migration/docs and analytix-owned conformance text; no `window.kunGui`, Kun bridge, Kun identity, or Reasonix public protocol surface |
| Default Go backend / Rust / Tauri scan | targeted production scan for `runtime-go`, default Go backend, Go backend selection, `Tauri`, `Cargo.toml`, and Rust backend terms | pass: no production backend/default path hit |
| Docs/spec/ledger/scorecard consistency scan | spec 08/09, sync ledgers, conflict decision, Go conformance, scorecard, and this release evidence updated for the same boundary | pass |

## Addendum Accepted Claims

```text
Dirty shell safe-area baseline was handled before runtime work.
Reasonix collaborative-execution proof advanced to internal task/parallel
contracts, dependency guards, nested SSE metadata, parent Goal evidence keys,
wait/output/kill, and transcript continue/fork constraints.
Provider/cache proof now includes an offline Reasonix-style cache curve guard;
DeepSeek cache proof is at least Reasonix-style and multi-provider coverage is
broader than Reasonix's DeepSeek-only guard.
MCP/indexer proof now includes retry-all failed startup servers, late suspended
provider tombstones, and codegraph/codebase-memory cwd/priority/backgroundStart.
Go G5 now has a TS-owned full-loop oracle inventory and remains shadow-only.
Kun v0.2.13 -> v0.2.14 product-entry baseline remains intact.
```

## Addendum Rejected Claims

```text
No full Reasonix task/parallel/planner-executor parity.
No live provider/cache superiority.
No live MCP/indexer parity.
No Kun tray session menu or Local Whisper parity.
No Go G5/full-loop parity.
No release readiness.
No default Go backend.
```

## Executable Runtime / Cache Guard / MCP Live-Local / Kun Tray Addendum

Scope:
Executable internal Reasonix task/parallel runtime slice, runtime-owned
DeepSeek cache curve proof, fake live-local MCP/indexer lifecycle proof,
analytix-native Kun tray session menu, and Go G5 shadow implementation attempt.
Earlier tray-deferred rows in this file remain historical; this addendum
supersedes them for code absorption, while packaged tray parity/QA remains
open.

This is still not a release-ready claim. Live provider/MCP QA, packaged
desktop QA, Windows NSIS verification, signing/notarization, release metadata,
verified remote, Local Whisper, Go full-loop parity, and default Go backend
readiness remain open.

## Executable Addendum Currentness

| Source | Evidence | Result |
| --- | --- | --- |
| Reasonix | `origin/main-v2` and remote `refs/heads/main-v2` resolve to `9e56c3276880ced538b3375329b8a0ebbe63a67b`. | pass: no newer Reasonix drift beyond the classified 9e56 set. |
| Kun | refs rechecked: `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master` `8602476`, `develop` `247076f`. | pass: product-entry baseline remains v0.2.13 -> v0.2.14. |

## Executable Addendum Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| Kun tray session menu | `npm run test -- src/main/tray-session-menu.test.ts src/main/index.test.ts src/main/window-chrome-config.test.ts` | pass: tray menu grouping/opening and existing main chrome tests passed before final full gate. |
| Executable task graph | `npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: internal `task`/`parallel_tasks` providers, dependency order, background output offset, and restart reconciliation proof passed. |
| Cache guard | `npm --prefix packages/runtime test -- provider-cache-proof.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: runtime-owned offline cache curve guard and DeepSeek native cache precedence proof passed. |
| MCP live-local fixture | `npm --prefix packages/runtime test -- mcp-tool-lifecycle-oracle.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: fake local retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics proof passed. |
| Go G5 toolchain check | `command -v go && go version` | blocked: no output; command exited non-zero. No Go files changed. |

## Executable Addendum Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 726 tests |
| App tests | `npm run test` | pass: 207 files / 1464 tests |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | not run because no Go files changed and no `go` command is available locally | n/a / blocked for Go G5 implementation |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entrance scan | targeted scan across Sidebar, Workbench, App, route/store/navigation files | pass: no top-level production route or sidebar entry; only an unrelated sidebar comment remains in targeted scan, while broader hits are internal/future capability files or settings capability status. |
| Kun identity scan | production scan for `window.kun`, `kunGui`, `KunAgent`, `Kun Desktop`, `kun serve`, and `KUN_` | pass: zero hits |
| Reasonix public protocol scan | production scan for Reasonix protocol terms | pass with known allowed hits: legacy settings migration fields and Go shadow `ReasonixPublicProtocolAllowed: false`; no Reasonix public protocol surface. |
| Default Go backend / Electron-to-Go / Rust / Tauri scan | targeted production scan for `runtime-go`, default Go backend, Electron-to-Go, Tauri, Cargo, and Rust backend terms | pass: no production backend/default path hit |
| Docs/spec/ledger/scorecard/release evidence consistency scan | spec 08/09, sync ledgers, conflict decisions, Go conformance, scorecard, and this release evidence updated for the same boundary | pass |

## Executable Addendum Accepted Claims

```text
Reasonix task/parallel behavior is absorbed only as an internal executable
analytix runtime provider backed by DelegationRuntime and DurableTaskJobManager.
Job output offsets, dependency order, parent context, and stale-job restart
reconciliation are fixture-backed.
DeepSeek cache proof is at least Reasonix-style, and analytix cache fixtures
are broader than Reasonix's focused cache guard across Responses, Anthropic,
reasoning tokens, canonical tool schemas, stable prefix, and unsupported
fallback behavior.
MCP/indexer proof now includes a fake live-local lifecycle fixture for
retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume,
and secret-safe diagnostics.
Kun tray session menu is absorbed as an analytix-native main-process desktop
affordance without changing renderer routes or preload bridge contracts.
Kun v0.2.13 -> v0.2.14 product-entry baseline remains intact.
```

## Executable Addendum Rejected Claims

```text
No full Reasonix planner/executor Coordinator parity.
No Reasonix public task/planner/plugin protocol.
No live provider/cache superiority.
No live MCP/indexer parity.
No Local Whisper parity.
No Go G5/full-loop implementation or parity.
No release readiness.
No default Go backend.
```

## Reasonix bfe398 / Go G5 Shadow Output Addendum

Scope:
Reasonix `main-v2` drift from `9e56c3276880ced538b3375329b8a0ebbe63a67b`
to `bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`, plus the first Go G5 shadow
output slice. This is not a release-ready claim and not a Go backend claim.

## bfe398 Addendum Currentness

| Source | Evidence | Result |
| --- | --- | --- |
| Reasonix | `git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2` | pass: latest remote is `bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`. |
| Reasonix drift | `9e56c327..bfe398cc` diff/log | classified: `3569f3a4` PowerShell 7 standard path detection and `32addaec` pwsh chaining guidance; record/future shell compatibility input only. |
| Kun | `git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14 refs/tags/v0.2.13^{} refs/tags/v0.2.14^{}` | pass: `develop` `247076f`, `master` `8602476`, `v0.2.13` tag `201a146` peeled `2ba8dec`, `v0.2.14` tag `06be05d` peeled `8f20403`. |

## bfe398 Addendum Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| G5 TS conformance | `npm --prefix packages/runtime test -- go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 7 tests |
| Go toolchain | `curl -fsSLO https://go.dev/dl/go1.26.4.darwin-arm64.tar.gz`; SHA256 check | pass: official `go1.26.4.darwin-arm64`, SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53` verified. |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass |

## bfe398 Addendum Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 726 tests |
| App tests | `npm run test` | pass: 207 files / 1464 tests |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entrance scan | targeted scan across Sidebar, Workbench, App, route/store/navigation files | pass: no top-level production route or sidebar entry; only an unrelated sidebar comment remains. |
| Kun identity scan | production scan for `window.kun`, `kunGui`, `KunAgent`, `Kun Desktop`, `kun serve`, and `KUN_` | pass: zero hits |
| Reasonix public protocol scan | production scan for Reasonix protocol terms | pass with known allowed hits: legacy settings migration fields and Go shadow `ReasonixPublicProtocolAllowed: false`; no Reasonix public protocol surface. |
| Default Go backend / Electron-to-Go / Rust / Tauri scan | targeted production scan for `runtime-go`, default Go backend, Electron-to-Go, Tauri, Cargo, and Rust backend terms | pass: no production backend/default path hit |
| Docs/spec/ledger/scorecard/release evidence consistency scan | spec 08/09, sync ledgers, conflict decisions, Go conformance, scorecard, and this release evidence updated for bfe398/G5 output boundary | pass |

## bfe398 Addendum Accepted Claims

```text
Reasonix bfe398 currentness is classified. The drift is shell/sandbox
PowerShell compatibility input and is not imported in this stage.
Go G5 advanced from inventory-only to a tested shadow output slice. The Go
package now replays the TS-owned G5 full-loop oracle inventory: source oracles,
full-loop tests, job manager routes and behaviors, cache/compaction tests,
resume/interrupt tests, MCP/indexer tests, remaining blockers, and product
boundary flags.
The G5 shadow output keeps Electron main disconnected, `analytix serve`
unchanged, renderer-visible Go routes disallowed, Reasonix public protocol
disallowed, and default Go backend disabled.
```

## bfe398 Addendum Rejected Claims

```text
No Reasonix shell/sandbox code import.
No Go G5 runtime parity.
No live Go full-loop executor.
No Electron-to-Go routing.
No default Go backend readiness.
No live provider/cache superiority.
No live MCP/indexer parity.
No release readiness.
```

## Go G5 Composite Shadow Replay Addendum

Scope:
Cross-fixture G5 shadow replay for jobs, cache, session/resume/SSE, and
MCP/indexer. This strengthens the bfe398 Go shadow output evidence, but remains
conformance-only and does not create a Go backend.

## Composite Replay Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| G5 TS source-fixture binding | `npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass |

## Composite Replay Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 727 tests |
| App tests | `npm run test` | pass: 207 files / 1464 tests |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entrance scan | targeted scan across Sidebar, Workbench, App, route/store/navigation files | pass: no top-level production route or sidebar entry; only an unrelated sidebar comment remains. |
| Kun identity scan | production scan for `window.kun`, `kunGui`, `KunAgent`, `Kun Desktop`, `kun serve`, and `KUN_` | pass: zero hits |
| Reasonix public protocol scan | production scan for Reasonix protocol terms | pass with known allowed hits: legacy settings migration fields and Go shadow `ReasonixPublicProtocolAllowed: false`; no Reasonix public protocol surface. |
| Default Go backend / Electron-to-Go / Rust / Tauri scan | targeted production scan for `runtime-go`, default Go backend, Electron-to-Go, Tauri, Cargo, and Rust backend terms | pass: no production backend/default path hit |
| Docs/spec/ledger/scorecard/release evidence consistency scan | spec 08/09, sync ledgers, conflict decisions, Go conformance, scorecard, and this release evidence updated for composite G5 replay boundary | pass |

## Composite Replay Accepted Claims

```text
Go G5 shadow now computes `shadowSlicesExpectedOutput` from concrete source
fixtures rather than only replaying the G5 inventory fixture.
Jobs replay is tied to task/parallel tool names, internal routes, dependency
order, planner read-only tools, nested metadata, and transcript identity flags.
Cache replay is tied to stable prefix hash, tools hash, provider usage case ids,
release-guard statuses, privacy substrings, and live-superiority policy.
Session replay is tied to G2 route ids, resume/fork route ids, and SSE replay
route ids.
MCP replay is tied to retry-all, expected connected servers, known override,
live-local active paths, late tombstone, and redacted diagnostics.
```

## Composite Replay Rejected Claims

```text
No live Go full-loop executor.
No live Go Job Manager, cache runtime, session runtime, or MCP client.
No Electron-to-Go routing.
No default Go backend.
No live provider/cache superiority.
No live MCP/indexer parity.
No release readiness.
```

## Planner Executor / Shell / Live-Local / G5 Runner Addendum

Scope:
Reasonix `main-v2` remains
`bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`. Kun refs remain the v0.2.13 ->
v0.2.14 product-entry baseline: `v0.2.13` `201a1469`, `v0.2.14` `06be05d`,
`master` `8602476`, and `develop` `247076f`.

## Planner Executor Addendum Focused Evidence

| Gate | Command | Result |
| --- | --- | --- |
| Runtime focused proof | `npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts builtin-tools.test.ts provider-cache-proof.test.ts mcp-tool-lifecycle-oracle.test.ts go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 5 files / 62 tests |
| Runtime package typecheck | `npm --prefix packages/runtime run typecheck` | pass |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass with temporary official `go1.26.4.darwin-arm64`, SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53` verified |

## Planner Executor Addendum Final Command Gate

| Gate | Command / evidence | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 735 tests |
| App tests | `npm run test` | pass: 207 files / 1465 tests; only Node `punycode` deprecation warnings observed |
| Typecheck | `npm run typecheck` | pass |
| Runtime build | `npm run build:runtime` | pass |
| Go package | `cd packages/runtime-go && PATH=<temporary-go>/go/bin:$PATH go test ./...` | pass with temporary official `go1.26.4.darwin-arm64`, SHA256 `b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53` verified |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entrance scan | targeted scan across Sidebar, Workbench, App, route/store/navigation files | pass: only negative tests keeping Workflow/Create Loop out of top-level sidebar/action surfaces matched |
| Kun identity scan | production scan for `window.kun`, `kunGui`, `KunAgent`, `Kun Desktop`, `kun serve`, and `KUN_` | pass: only preload sandbox forbidden-bridge test matched |
| Reasonix public protocol scan | production scan for Reasonix protocol terms | pass with allowed hits only: Go conformance false flags, legacy migration tests, and `REASONIX.md` forbidden instruction text; no public protocol surface |
| Default Go backend / Electron-to-Go / Rust / Tauri scan | targeted production scan for `runtime-go`, default Go backend, Electron-to-Go, Tauri, Cargo, and Rust backend terms | pass: only conformance flags explicitly setting default Go backend to false matched |
| Docs/spec/ledger/scorecard/release evidence consistency scan | spec 08/09, sync ledgers, conflict decisions, Go conformance, scorecard, and this release evidence updated for planner/executor shell live-local G5 runner boundary | pass: latest boundary appears in spec 08 section 36, spec 09 section 24, absorption ledger, reasonix sync, conflict decision D-0022, Go conformance, scorecard, and this release evidence |

## Planner Executor Addendum Accepted Claims

```text
Reasonix planner/executor semantics are absorbed only as internal analytix
runtime evidence: read-only planner job, executor DAG waves, failed-dependency
skips, cancellation propagation, output offsets, and parent metadata.
Durable queued/running jobs can be rehydrated after restart and then output,
wait, or kill through the existing job manager contract.
Reasonix bfe398 PowerShell value is absorbed as analytix shell fixtures:
standard pwsh path fallback, pwsh chaining support, and Windows PowerShell
unquoted chaining guard.
DeepSeek cache telemetry is proved against an executable local provider server
with native hit/miss precedence and reasoning tokens.
MCP/indexer proof now runs an executable stdio fake server with restart/resume,
tombstone, cwd, low-priority/backgroundStart, and secret-safe diagnostics.
Go G5 composite shadow replay includes planner/executor and durable-runner
restart fields while remaining shadow-only.
Kun v0.2.13 -> v0.2.14 product-entry baseline remains intact.
```

## Planner Executor Addendum Rejected Claims

```text
No full Reasonix public task/planner/plugin protocol.
No Reasonix SessionAPI or frontend task cards.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Kun identity or bridge alias.
No Local Whisper parity.
No live provider/cache superiority.
No live MCP/indexer parity.
No live Go full-loop executor, Electron-to-Go routing, or default Go backend.
No Rust/Tauri rewrite.
No release readiness.
```

## Reasonix 881 Control / Step-Limit / G5 Shadow Addendum

Scope:
Requested Reasonix target `881b2f2f2644d1873c7867f29c64cae7be7d9c24`
is no longer the current `origin/main-v2`; fetched current is
`9ada14176629b1d59d7ed78446951b2bb5954904`. This addendum classifies and
absorbs only `bfe398cc..881b2f2f` control semantics. Post-881 auto-plan drift is
recorded, not imported.

Kun refs:

```text
v0.2.13 / kun-v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
v0.2.14 / kun-v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
master 8602476c5c449b4561473ad5f31081ee93dc782e
develop 247076f297170c3d0c558baffb073c629894faea
```

Stash evidence:

```text
codex-preserve-workbench-preload-before-reasonix-parity:
  duplicate / covered-by-HEAD semantic; retained, not dropped.
codex-preserve-loading-page-before-reasonix-parity:
  duplicate / covered-by-HEAD semantic; retained, not dropped.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime loop + contracts + config | `npm --prefix packages/runtime test -- loop.test.ts contracts.test.ts analytix-config.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files / 129 tests |
| Task-job cancellation aggregation | `npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |
| Delegation child step inheritance | `npm --prefix packages/runtime test -- delegation-runtime.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 13 tests |
| Go G5 control shadow fixture | `npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| GUI-managed runtime settings | `npm run test -- app-settings.test.ts app-ipc-schemas.test.ts analytix-process.test.ts` | pass: 3 files / 131 tests |

Accepted claims:

```text
Reasonix cancelled-batch result preservation is absorbed behind
AgentLoop.dispatchToolCalls.
Runtime step limits are analytix-owned under top-level runtime settings and
thread/turn metadata; dynamic limits are not added to stable prefix.
Task-job parent cancellation preserves completed/killed/skipped output evidence.
Go G5 controlReplay shadows cancel, task-job cancellation, and step-limit gates.
Kun v0.2.13 -> v0.2.14 product-entry baseline remains intact.
```

Post-881 / Kun / Go executable control evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router classifier guard | `npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 11 tests |
| Runtime loop non-regression | `npm --prefix packages/runtime test -- tests/loop.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 78 tests |
| Kun provider default correction | `npm run test -- src/shared/app-settings-provider.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 28 tests |
| Go G5 executable control shadow | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 1.286s` |
| Go toolchain provenance | `go.dev/dl/?mode=json` + SHA256 verification for `go1.25.11.darwin-arm64.tar.gz` | pass: `cd8d4920e7930d55da1a5a57ba43a64b1305f71cdf2ca3c76cd8c549272b1680` |

Rejected claims:

```text
No Reasonix public protocol, TUI cancel UX, SessionAPI, or config root.
No post-881 auto-plan parity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Kun identity or deprecated bridge alias.
No default Go backend, Electron-to-Go routing, or renderer-visible Go route.
No Rust/Tauri rewrite.
No live provider/cache superiority, live MCP/indexer parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 745 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1472 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` with official temporary `go1.25.11.darwin-arm64` | pass: `ok github.com/analytix/runtime-go 1.286s` |
| Go toolchain provenance | `shasum -a 256 /tmp/analytix-go-dl/go1.25.11.darwin-arm64.tar.gz`; official `go.dev/dl` metadata | pass: `cd8d4920e7930d55da1a5a57ba43a64b1305f71cdf2ca3c76cd8c549272b1680` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_" src packages --glob '!**/*.test.ts'` | pass: no hits |
| Reasonix public protocol scan | `rg -n "Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only G5 fixture assertions that the renderer must not observe Go-specific routes or Reasonix public protocol |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits; no deprecated bridge or old runtime settings write path |
| Default Go / Rust / Tauri scan | `rg -n "default Go backend|defaultGoBackendEnabled: true|defaultGoBackendAllowed: true|Electron-to-Go|renderer-visible Go route|Tauri|tauri|Cargo\\.toml|rust backend|Rust/Tauri rewrite" src packages electron-builder.config.cjs package.json --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "9ada1417|controlExecutableCases|AUTO_MODEL_ROUTER_FINGERPRINT|128_000|D-0024|go1\\.25\\.11" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src packages/runtime/tests src/shared packages/runtime-go` | pass: expected synchronized refs, code evidence, and evidence records only |

## Approval/User-Input Abort Cleanup Addendum

Scope:
Reasonix approvalManager/control cleanup value absorbed behind analytix
`ApprovalGate`, `UserInputGate`, `AgentLoop`, and existing HTTP/SSE contracts.
No Reasonix SessionAPI, public approval protocol, renderer route, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Gate + loop + route oracle | `npm --prefix packages/runtime test -- tests/ports.test.ts tests/loop.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 102 tests |
| Renderer live approval mapping | `npm run test -- src/renderer/src/agent/analytix-mapper.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 35 tests |
| Go G3/G4/G5 conformance guard | `npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
Approval abort cleanup is fixture-backed: pending approvals expire on turn
interrupt, replay emits approval_resolved: expired, late allow/deny cannot
execute cancelled work, and cancelled tool-result pairing is preserved.

User-input abort cleanup is fixture-backed: request_user_input emits
user_input_resolved: cancelled on turn interrupt, gate state is cleared, and
late HTTP resolve returns not found.

Renderer live mapping is fixture-backed: approval_resolved: expired updates
existing approval cards to the current error state instead of leaving them
pending until reload.
```

Rejected claims:

```text
No full Reasonix approval-manager parity.
No Reasonix SessionAPI or public approval/user-input protocol.
No renderer-visible Reasonix route.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No default Go backend or renderer-visible Go route.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 748 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1471 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0025|abortCleanup|ApprovalGate\\.expire|approval_resolved: expired|user_input_resolved: cancelled|live mapper|Renderer live mapping|late approval 409|late user-input 404|Approval/User-Input Abort Cleanup" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src packages/runtime/tests src/renderer/src/agent` | pass: expected synchronized refs, code evidence, and evidence records only |

## Renderer Approval Live-Card Store Evidence Addendum

Scope:
Approval/user-input abort cleanup evidence is extended from mapper-level proof
to renderer store-level proof. Main chat and side conversation sinks update
existing approval cards through analytix-owned store state. No Reasonix
SessionAPI, public ask protocol, renderer route, bridge/settings/provider
change, default Go backend, Rust/Tauri path, Kun identity, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer main + side approval live-card store tests | `npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 33 tests |

Accepted claims:

```text
Renderer store live-card evidence is fixture-backed: live approval status
updates mutate existing main-thread approval cards, and side conversation
approval status updates stay scoped to side blocks.
```

Rejected claims:

```text
No full Reasonix frontend approval parity.
No packaged desktop click-through or crash/restart QA claim.
No Reasonix SessionAPI or public approval/user-input protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No default Go backend or renderer-visible Go route.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 748 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1474 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` with `go1.25.11 darwin/arm64` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0026|Renderer Approval Live-Card|chat-store-runtime\\.test\\.ts|chat-store-side-actions\\.test\\.ts|side conversation approval|store-level proof" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/renderer/src/store` | pass: expected synchronized refs, code evidence, and evidence records only |

## User-Input Live-Card Store Evidence Addendum

Scope:
Approval/user-input abort cleanup evidence is extended to user-input renderer
store proof. Main chat and side conversation sinks update existing user-input
cards through analytix-owned store state and support runtime item id plus
input/request id matching. No Reasonix SessionAPI, public ask protocol,
renderer route, bridge/settings/provider change, default Go backend,
Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/
MCP-indexer entry is introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer main + side user-input live-card store tests | `npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 35 tests |

Accepted claims:

```text
Renderer store live-card evidence is fixture-backed: live user-input status
updates mutate existing main-thread user-input cards by item id or input id,
and side conversation user-input status updates stay scoped to side blocks.
```

Rejected claims:

```text
No full Reasonix ask/user-input frontend parity.
No packaged desktop click-through or crash/restart QA claim.
No Reasonix SessionAPI or public approval/user-input protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No default Go backend or renderer-visible Go route.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 748 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1476 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0027|User-Input Live-Card|user-input live-card|onUserInputStatus|chat-store-runtime\\.ts|chat-store-side-actions\\.ts|itemId.*requestId" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/renderer/src/store` | pass: expected synchronized refs, code evidence, and existing architecture notes only |

## Planner Gating For Internal Task Tools Addendum

Scope:
Reasonix task/planner orchestration value is tightened behind analytix-owned
runtime contracts. Internal `task` and `parallel_tasks` tools remain available
only as internal runtime tools, are not advertised in Plan mode, and forged
Plan-mode task calls are rejected before child work executes. No Reasonix
SessionAPI, public task/planner protocol, renderer route,
bridge/settings/provider change, default Go backend, Rust/Tauri path, Kun
identity, or top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is
introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Plan-mode task-tool gating | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "keeps internal task tools out of plan mode" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 1 selected test |
| Task-job oracle | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |
| Go G3/G4/G5 conformance guard | `npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |

Accepted claims:

```text
Planner gating is fixture-backed: internal task/parallel child-job tools are
not advertised during Plan mode, and a forged task call is rejected by active
tool policy without executing child work.
```

Rejected claims:

```text
No full Reasonix planner/executor Coordinator parity.
No Reasonix public task/planner protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No default Go backend or renderer-visible Go route.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 749 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1476 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0028|Planner Gating|plannerForbiddenToolset|keeps internal task tools out of plan mode|TASK_TOOL_CONTRACT|PARALLEL_TASKS_TOOL_CONTRACT" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src packages/runtime/tests` | pass: expected synchronized refs, code evidence, and earlier task-contract records only |

## Go G5 Planner-Forbidden Shadow Replay Addendum

Scope:
Go G5 shadow output now consumes the TS-owned `plannerForbiddenToolset` from
the task-job oracle and records it in `shadowSlicesExpectedOutput.jobReplay`.
No live Go planner/executor, Electron route, renderer route, bridge/settings/
provider change, default Go backend, Rust/Tauri path, Kun identity, or
top-level Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Go G5 conformance shadow binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow package tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Go G5 shadow carries planner-forbidden task-tool evidence from the TypeScript
task-job oracle while remaining non-default and non-live.
```

Rejected claims:

```text
No Go runtime parity.
No default Go backend or renderer-visible Go route.
No full Reasonix planner/executor Coordinator parity.
No Reasonix public task/planner protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 749 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1476 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0029|Go G5 Planner-Forbidden|plannerForbiddenToolset|BuildG5ShadowSlicesOutput|planner-forbidden task-tool" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime-go packages/runtime/src packages/runtime/tests` | pass: expected synchronized refs, code evidence, and previous planner-gating records only |

## Write-Inline Provider Request-Surface Addendum

Scope:
Write-inline provider request-surface evidence now covers OpenAI Responses and
Anthropic Messages endpoint formats. The batch changes tests only and adds no
runtime route, bridge/settings/provider default, default Go backend, Rust/Tauri
path, Kun identity, Reasonix public protocol, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Write-inline provider request-surface tests | `npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 30 tests |

Accepted claims:

```text
Write-inline provider request-surface evidence is fixture-backed for OpenAI
Responses and Anthropic Messages URL/body/header/parser behavior.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix or cost reconciliation.
No full Reasonix provider parity.
No Reasonix public provider protocol.
No default Go backend or renderer-visible Go route.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 749 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1478 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0030|Write-Inline Provider Request-Surface|write-inline provider request-surface|OpenAI Responses body shape|Anthropic Messages body shape|write-inline-completion-service\\.test\\.ts" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/main/services` | pass: expected synchronized refs, test evidence, and earlier write-inline references only |

## Go G5 Planner Tool-Policy Executable Shadow Addendum

Scope:
Go G5 control executable output now computes planner tool-policy gating from
TS-owned fixture input. The batch adds no live Go planner/executor, Electron
route, renderer route, bridge/settings/provider default, default Go backend,
Rust/Tauri path, Kun identity, Reasonix public protocol, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Go G5 conformance planner executable binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow package tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Go G5 executable shadow now computes planner step 0/step 1 advertised tools and
forged internal task rejection while remaining non-default and non-live.
```

Rejected claims:

```text
No Go runtime parity.
No default Go backend or renderer-visible Go route.
No full Reasonix planner/executor Coordinator parity.
No Reasonix public task/planner protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No live provider/cache or live MCP superiority.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 749 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1478 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0031|Go G5 Planner Tool-Policy|controlExecutableCases\\.planner|replayG5PlannerPolicy|forged internal task rejection|planner step 0" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime-go packages/runtime/src packages/runtime/tests` | pass: expected synchronized refs, code evidence, and evidence records only |

## Structured User-Input Choice Validation Addendum

Scope:
`request_user_input` now rejects malformed structured choice requests before
opening an analytix GUI user-input gate. The batch adds no Reasonix ask/session
protocol, renderer route, bridge/settings/provider default, default Go backend,
Rust/Tauri path, Kun identity, or top-level Workflow/Subagent/AutoResearch/
MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime structured user-input validation and G4 oracle binding | `npm --prefix packages/runtime test -- tests/builtin-tools.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 44 tests |
| Go G4 shadow package tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix rejects malformed structured GUI input choices before opening a
pending user-input gate, and G4/Go shadow evidence records the stable error
contract.
```

Rejected claims:

```text
No full Reasonix ask/user-input frontend parity.
No Reasonix public ask/session/control protocol.
No desktop packaged/live QA claim.
No default Go backend or renderer-visible Go route.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Kun identity, deprecated bridge/settings fallback, or Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 750 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1478 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G4 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0032|Structured User-Input Choice|invalid_user_input_request|userInputValidation|request_user_input.*options" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime-go packages/runtime/src packages/runtime/tests` | pass: expected synchronized refs, code evidence, and evidence records only |

## Write-Inline Custom Full Endpoint Addendum

Scope:
Write-inline provider request-surface evidence now covers custom full endpoint
URLs ending in `/responses` and `/messages`. The batch changes tests only and
adds no runtime route, bridge/settings/provider default, default Go backend,
Rust/Tauri path, Kun identity, Reasonix public protocol, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Write-inline custom full endpoint tests | `npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 32 tests |

Accepted claims:

```text
Write-inline provider request-surface evidence is fixture-backed for custom
full endpoint Responses and Messages URL/body/header/parser behavior.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix or cost reconciliation.
No full Reasonix provider parity.
No Reasonix public provider protocol.
No settings schema or provider default change.
No default Go backend or renderer-visible Go route.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 750 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1480 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0033|Write-Inline Custom Full Endpoint|custom full endpoint|custom.*responses|custom.*messages" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/main/services/write-inline-completion-service.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Auto-Model Route Cache Lifecycle Addendum

Scope:
`AgentLoop` now has loop-level proof that `model:"auto"` route selection is
cached within a multi-step turn and recalculated for the following turn. The
batch changes tests only and adds no Reasonix auto-plan setting, local/project
config, desktop controller API, stable-prefix classifier state, default Go
backend, Rust/Tauri path, Kun identity, or top-level
Workflow/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| AgentLoop auto-route cache lifecycle | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto model" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 1 selected test |

Accepted claims:

```text
Analytix auto model routing reuses one classifier route inside a multi-step
turn and re-runs the classifier on the following turn.
```

Rejected claims:

```text
No Reasonix auto-plan parity.
No user-level auto-plan setting or project/local auto-plan override.
No Reasonix controller API or public protocol.
No stable-prefix classifier/cache mutation.
No default Go backend or renderer-visible Go route.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 751 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1480 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0034|Auto-Model Route Cache|auto model route cache|same-turn auto route|next-turn" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/loop.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Provider Request-Shape Oracle Matrix Addendum

Scope:
Provider/cache conformance now includes request URL/header/body shape cases for
DeepSeek official chat, OpenAI-compatible chat, OpenAI Responses, Anthropic
Messages, and custom Responses full endpoint mode. G3/G5 Go shadow output only
replays request-shape case ids; no Go provider client or default backend is
introduced.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider request-shape oracle and Go shadow bindings | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow package tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix provider/cache proof now centrally covers request-shape invariants for
DeepSeek, OpenAI-compatible, Responses, Anthropic Messages, and custom
Responses full endpoint mode.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix or cost reconciliation.
No Go provider client or default Go backend.
No Reasonix public provider protocol.
No settings schema or provider default change.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 752 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1480 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go G3/G5 shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'` | pass: no top-level entry hits |
| Kun/Reasonix identity scan | `rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts'` | pass: only Go fixture assertions that Reasonix public protocol must not be exposed |
| Deprecated bridge/settings fallback scan | `rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|legacy.*agent|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'` | pass: only explicit legacy `agentThreadIds` import compatibility hits |
| Default Go/Rust/Tauri scan | `rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0035|Provider Request-Shape Oracle|requestShapeCases|request-shape case ids|DeepSeek official chat" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime-go packages/runtime/src packages/runtime/tests` | pass: expected synchronized refs, fixture/schema/test evidence, and evidence records only |

## Kun Top-Level Route-Surface Oracle Addendum

Scope:
Renderer route-surface evidence now pins the Kun v0.2.13 -> v0.2.14 product
entry boundary for `AppRoute`, app actions, Workbench stage rendering, shell
navigation, and sidebar active views. This batch changes tests/docs only and
adds no new product surface, runtime route, bridge/settings/provider default,
default Go backend, Rust/Tauri path, Kun identity, Reasonix public protocol, or
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer route-surface oracle | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts` | pass: 2 files / 11 tests |

Accepted claims:

```text
Analytix has executable route-surface evidence that Kun-target-absent
orchestration features are not exposed as top-level navigation entries.
```

Rejected claims:

```text
No new product navigation.
No full Kun current parity.
No full Reasonix planner/subagent/orchestration parity.
No Reasonix public task/planner/session/ask protocol.
No bridge/settings/provider/default Go backend/Rust/Tauri/Kun identity change.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 752 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1481 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## D-0063 Go G5 Provider Cache Release Guard Executable Shadow Addendum

Scope:
Provider cache release guard evidence now has a G5 executable shadow case
derived from the existing analytix provider-cache oracle and TS offline cache
curve guard. This batch changes oracle, conformance, Go shadow, and docs only;
it adds no live provider superiority claim, Reasonix provider protocol,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path, Kun identity, or
deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 provider cache release guard binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts` | pass: 2 files / 13 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.408s` |

Accepted claims:

```text
Analytix G5 shadow computes the offline provider cache release guard from
TS-owned fixtures: tail averages, statuses, collapse counts, low-tail allowance,
and overall pass.
```

Rejected claims:

```text
No live provider/cache superiority claim.
No credentialed provider matrix or packaged provider QA.
No Go provider client, renderer-visible Go route, or default Go backend.
No Reasonix provider protocol parity.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1499 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no user-facing locale hits |

## D-0065 Planner Step-Limit AgentLoop Addendum

Scope:
The TypeScript AgentLoop now has a focused proof that `plannerMaxModelSteps`
gates real plan-mode turns and stays out of model-visible prompt/cache
surfaces. This batch changes test/docs only; it adds no Reasonix planner
setting, controller API, public protocol, renderer-visible Go route, default Go
backend, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| AgentLoop planner step-limit proof | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "uses the planner step limit for plan-mode turns"` | pass: 1 file / 1 test |

Accepted claims:

```text
Analytix AgentLoop applies `plannerMaxModelSteps` to plan-mode turns, records the
planner step-limit failure, advertises `create_plan`, and keeps planner budget
state out of model-visible prefix/context.
```

Rejected claims:

```text
No Reasonix planner enable/disable product controls.
No Reasonix controller/SessionAPI protocol parity.
No Go planner backend readiness or default Go backend.
No packaged desktop plan-mode QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 769 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1499 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no user-facing locale hits |

## D-0064 Auto-Route Step/Cancel Composition Addendum

Scope:
The TypeScript AgentLoop now has a focused composition proof for auto-route
cache, step-limit metadata, and interrupted parallel tool results. This batch
changes test/docs only; it adds no Reasonix auto-plan setting, controller API,
public protocol, renderer-visible Go route, default Go backend, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path,
Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| AgentLoop auto-route step/cancel composition | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "reuses the auto route across a step before preserving cancelled parallel tool results"` | pass: 1 file / 1 test |

Accepted claims:

```text
Analytix AgentLoop routes `model:"auto"` once, reuses the selected model on the
next step, keeps step-limit metadata out of model-visible prefix/context, and
preserves completed/cancelled parallel tool results on interruption.
```

Rejected claims:

```text
No Reasonix auto-plan config parity.
No Reasonix controller/SessionAPI protocol parity.
No planner enable/disable product setting.
No packaged desktop interruption QA or release readiness.
No default Go backend, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 768 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1499 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no user-facing locale hits |

## Go G5 Checkpoint/Rewind Executable Shadow Addendum

Scope:
Checkpoint/rewind safety now has a G5 executable shadow case that is derived
from the existing analytix checkpoint oracle. This batch changes oracle,
conformance, Go shadow, and docs only; it adds no Reasonix checkpoint/rewind
protocol, Kun git-ref checkpoint semantics, renderer-visible Go route, default
Go backend, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
route, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 checkpoint/rewind binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.428s` |

Accepted claims:

```text
Analytix G5 shadow computes checkpoint/rewind safety boundaries from
TS-owned checkpoint oracle data: analytix id prefixes, path escape and symlink
blocking, legal dot-dot-prefixed filename readiness, explicit confirmation,
append-only audit, no transcript rewrite, no git refs, and no public route.
```

Rejected claims:

```text
No live Go checkpoint store, Go route, or default Go backend.
No Reasonix checkpoint/rewind protocol parity.
No Kun git-ref checkpoint parity.
No renderer-visible Go route, packaged desktop rewind QA, or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## D-0062 Go G5 Resume Pending Gates Executable Shadow Addendum

Scope:
Session resume pending approval/user-input gate safety now has a G5 executable
shadow case derived from the existing analytix approval/user-input route oracle.
This batch changes oracle, conformance, Go shadow, and docs only; it adds no
Reasonix ask/session protocol, renderer-visible Go route, default Go backend,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 resume pending gates binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.500s` |

Accepted claims:

```text
Analytix G5 shadow computes that resumed threads keep approval/user-input audit
visibility without leaving pending gates actionable or copying submitted
answers.
```

Rejected claims:

```text
No live Go session-resume route, approval manager, user-input manager, or
default Go backend.
No Reasonix ask/session protocol parity.
No renderer-visible Go route, packaged desktop resume QA, or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1499 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no user-facing locale hits |

## Go G5 Remote-Entry Boundary Executable Shadow Addendum

Scope:
Remote-entry control-port boundaries now have a G5 executable shadow case
derived from the existing analytix approval/user-input route oracle. This batch
changes oracle, conformance, Go shadow, and docs only; it adds no Reasonix
SessionAPI/control protocol, renderer-visible Go route, default Go backend,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 remote-entry binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.411s` |

Accepted claims:

```text
Analytix G5 shadow computes that remote entries expose only lifecycle, turn,
approval, and user-input controls while omitting goal, checkpoint, memory,
storage, thread-service/thread-store, and tool-host control planes.
```

Rejected claims:

```text
No live Go remote-entry route or default Go backend.
No Reasonix SessionAPI/control protocol parity.
No renderer-visible Go route, packaged desktop remote-entry QA, or release
readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## Go G5 User-Input Gate Executable Shadow Addendum

Scope:
Submitted/cancelled user-input gate semantics now participate in G5 executable
shadow evidence. This batch changes conformance fixtures/schema, Go shadow
calculation, Go/TS tests, and docs only; it adds no Reasonix ask/session public
protocol, Go user-input HTTP route, renderer-visible Go route, default Go
backend, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 user-input fixture binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.478s` |

Accepted claims:

```text
Analytix Go G5 shadow computes submitted/cancelled user-input gate outcomes
from TS-owned fixtures and keeps submitted answers out of SSE replay.
```

Rejected claims:

```text
No Reasonix ask/session protocol parity.
No live Go user-input manager or Go HTTP user-input route.
No renderer-visible Go route, default Go backend, or Go runtime parity.
No packaged desktop live-card QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale identity scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale hits |
| Docs/spec/ledger consistency | `rg -n "D-0057|Go G5 User-Input Gate|controlExecutableCases\\.userInput|approvalUserInputReplay|replayG5UserInput|approval-user-input-route-api-gates-v1" 重构升级方案.md docs/analytix packages/runtime/src/conformance packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go` | pass: expected synchronized refs only |

## Go G5 AutoResearch Project-Local State Shadow Addendum

Scope:
AutoResearch project-local state and requirement-audit boundaries now
participate in G5 executable shadow evidence. This batch changes conformance
fixtures/schema, Go shadow calculation, Go/TS tests, and docs only; it adds no
Reasonix AutoResearch/project public protocol, Go AutoResearch HTTP route,
renderer-visible Go route, default Go backend, top-level AutoResearch/
Subagent/Workflow/Create Loop/MCP-indexer route, Rust/Tauri path, Kun identity,
or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 AutoResearch fixture binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/autoresearch-store.test.ts` | pass: 2 files / 9 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.416s` |

Accepted claims:

```text
Analytix Go G5 shadow computes AutoResearch project-local state boundaries,
unknown requirement rejection, and prefix/tool-schema isolation from TS-owned
contracts.
```

Rejected claims:

```text
No Reasonix AutoResearch/project protocol parity.
No top-level AutoResearch route or navigation.
No live Go AutoResearch manager or Go HTTP AutoResearch route.
No renderer-visible Go route, default Go backend, or Go runtime parity.
No packaged desktop long-task QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale identity scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale hits |
| Docs/spec/ledger consistency | `rg -n "D-0058|Go G5 AutoResearch|controlExecutableCases\\.autoResearch|replayG5AutoResearch|AutoResearch project-local" 重构升级方案.md docs/analytix packages/runtime/src/conformance packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go` | pass: expected synchronized refs only |

## Go G5 MCP Lifecycle Executable Shadow Addendum

Scope:
MCP retry, tombstone/resume, and redaction boundaries now participate in G5
executable shadow evidence. This batch changes conformance fixtures/schema, Go
shadow calculation, Go/TS tests, and docs only; it adds no Reasonix MCP public
protocol, Go MCP HTTP route, renderer-visible Go route, default Go backend,
top-level MCP-indexer/Subagent/Workflow/Create Loop/AutoResearch route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP lifecycle fixture binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-lifecycle-oracle.test.ts` | pass: 2 files / 9 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.407s` |

Accepted claims:

```text
Analytix Go G5 shadow computes MCP retry, tombstone/resume, and redaction
boundaries from TS-owned fake/local fixtures.
```

Rejected claims:

```text
No Reasonix MCP protocol parity.
No top-level MCP-indexer route or navigation.
No live Go MCP client or Go HTTP MCP route.
No renderer-visible Go route, default Go backend, or Go runtime parity.
No credentialed MCP server matrix, packaged desktop MCP QA, or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale identity scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale hits |
| Docs/spec/ledger consistency | `rg -n "D-0059|Go G5 MCP Lifecycle|controlExecutableCases\\.mcpLifecycle|replayG5MCPLifecycle|MCP retry" 重构升级方案.md docs/analytix packages/runtime/src/conformance packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go` | pass: expected synchronized refs only |

## Provider Endpoint URL Builder Parity Addendum

Scope:
Schedule and Write auxiliary provider consumers now share endpoint URL
construction for versioned responses/messages bases and known endpoint paths.
This batch changes the shared URL helper, scheduled detector URL construction,
write-inline helper usage, tests, and docs; it adds no Reasonix provider
protocol, Kun public protocol, deprecated bridge/settings fallback, default Go
backend, renderer-visible Go route, Rust/Tauri path, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer route, live provider credential matrix,
or release-readiness claim.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Shared URL + scheduled detector + write-inline | `npm run test -- src/shared/openai-compat-url.test.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 47 tests |

Accepted claims:

```text
Analytix auxiliary provider consumers share endpoint URL construction for
versioned responses/messages bases and known endpoint paths.
```

Rejected claims:

```text
No live provider/cache superiority or credentialed provider matrix.
No Reasonix provider protocol parity.
No packaged Schedule/Write QA or release readiness.
No Go provider client, renderer-visible Go route, default Go backend, or Go
runtime parity.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1496 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no standalone Claw locale hits |
| Docs/spec/ledger consistency | `rg -n "D-0056|Provider Endpoint URL Builder Parity|upstreamOpenAiModelEndpointUrl|v2/responses|v3/messages|known endpoint path" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/shared/openai-compat-url.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Managed Runtime Provider Currentness And Identity Guard Addendum

Scope:
Managed runtime rebuild decisions and child provider snapshots now include
selected provider endpoint/model-profile currentness. Model-visible prompt copy,
public facade domains, and legacy Kun agent envelopes are guarded under
analytix-owned contracts. This batch changes shared settings helpers, main
runtime env tests, runtime prompt copy/tests, preload/settings tests, and docs;
it adds no Reasonix public auto-plan config, project/local override, controller
API, SessionAPI, default Go backend, renderer-visible Go route, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Rust/Tauri path,
Kun identity, or live provider credential matrix.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime prompt + auto-router | `npm --prefix packages/runtime test -- tests/analytix-system-prompt.test.ts tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 6 tests |
| Shared/main/preload/settings currentness guards | `npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files / 84 tests |
| Post-type-fix main focused retest | `npm run test -- src/main/analytix-process.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 29 tests |

Accepted claims:

```text
Analytix keys selected provider currentness into managed runtime rebuild
decisions and child provider snapshots while guarding model-visible and public
API identity under analytix contracts.
```

Rejected claims:

```text
No Reasonix auto-plan config parity or user-visible auto-plan setting.
No Reasonix public protocol, controller API, or SessionAPI.
No live provider/cache superiority or credentialed provider matrix.
No internal `claw` schema/IPC/type/settings rename.
No default Go backend, renderer-visible Go route, Go provider client, or Go
runtime parity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 767 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1492 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no standalone Claw locale hits |
| Docs/spec/ledger consistency | `rg -n "D-0055|Managed Runtime Provider Currentness|buildAnalytixRuntimeSettingsKey|agents\\.kun|ANALYTIX_MODEL_PROVIDERS|analytix-system-prompt" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/shared/app-settings-runtime.ts src/main/analytix-process.test.ts packages/runtime/tests/analytix-system-prompt.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Connect Phone Product Copy Sovereignty Addendum

Scope:
Connect Phone user-visible and model-visible natural-language copy now avoids
standalone Claw wording while internal `claw` compatibility contracts remain.
This batch changes renderer locales/tests, main runtime reply copy, IPC error
copy, shared prompt natural-language hints, and docs only. It adds no Kun
identity, Reasonix public protocol, deprecated bridge/settings fallback,
renderer-visible Go route, default Go backend, top-level Subagent/Workflow/
Create Loop/AutoResearch/MCP-indexer route, Rust/Tauri path, or release-ready
live Connect Phone QA claim.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Connect Phone locale/runtime/prompt copy | `npm run test -- src/main/claw-runtime.test.ts src/shared/app-settings.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts` | pass: 3 files / 109 tests |

Accepted claims:

```text
Analytix guards Connect Phone user-visible and model-visible copy against
standalone Claw wording while preserving internal `claw` compatibility
contracts.
```

Rejected claims:

```text
No internal `claw` schema/IPC/type/settings rename.
No Kun identity, deprecated bridge/settings fallback, or Kun public protocol.
No Reasonix public protocol or SessionAPI.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No Go route, default Go backend, or renderer-visible Go backend.
No Rust/Tauri migration.
No live Connect Phone desktop/remote QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 766 tests |
| Workspace tests | `npm run test` | pass: 208 files / 1488 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale copy scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no user-visible locale hits |

## Task-Job Route Auth Matrix Addendum

Scope:
Task-job wait/output/kill now have oracle-backed unauthorized 401 coverage.
This batch changes oracle/test/docs only; it adds no Reasonix public job/session
protocol, unauthenticated job control, renderer-visible Go route, default Go
backend, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job route auth matrix | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts` | pass: 1 file / 8 tests |

Accepted claims:

```text
Analytix task-job wait/output/kill routes are authenticated internal runtime
routes and reject missing runtime tokens.
```

Rejected claims:

```text
No Reasonix public job protocol parity.
No unauthenticated task-job control.
No Go route auth readiness, renderer-visible Go route, or default Go backend.
No packaged desktop route QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 766 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## User-Input Submitted Route Addendum

Scope:
Submitted user-input answers now have analytix HTTP/gate resolution proof, while
SSE replay remains answer-free. This batch changes oracle/test/shadow/docs only;
it adds no Reasonix public user-input protocol, answer archival in SSE history,
renderer-visible Go route, default Go backend, top-level
Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route, Rust/Tauri path,
Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| User-input submitted route + G4 binding | `npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts` | pass: 2 files / 4 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.450s` |

Accepted claims:

```text
Analytix user-input submitted answers are echoed through HTTP/gate resolution,
while SSE replay records only submitted status.
```

Rejected claims:

```text
No Reasonix public user-input protocol parity.
No answer archival in SSE history.
No live cross-device user-input delivery claim.
No Go user-input route, renderer-visible Go route, or default Go backend.
No packaged GUI live-card QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 766 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## MCP Annotation Approval No-Execute Addendum

Scope:
Destructive/openWorld MCP annotations now feed analytix approval gating, and
denied GUI approval prevents MCP client execution. This batch changes
oracle/test/shadow/docs only; it adds no Reasonix MCP public protocol,
MCP-indexer top-level entry, renderer-visible Go route, default Go backend,
live MCP credential matrix, top-level Subagent/Workflow/Create Loop/AutoResearch
route, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP annotation approval + G4 binding | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts` | pass: 2 files / 5 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.428s` |

Accepted claims:

```text
Analytix MCP annotations feed approval gating, and denied annotated MCP tools
do not execute in TS lifecycle tests or Go G4 shadow evidence.
```

Rejected claims:

```text
No Reasonix MCP public protocol parity.
No MCP-indexer top-level entry.
No live MCP credential compatibility matrix.
No Go MCP client, renderer-visible Go route, or default Go backend.
No packaged desktop approval QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0036|Kun Top-Level Route-Surface|Top-Level Route-Surface|route-surface oracle|forbiddenTopLevel|forbidden top-level" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Auto-Route Step/Cancel Control-Composition Addendum

Scope:
Runtime and G5 shadow evidence now compose auto-route cache reuse, user-global
step-limit failure, stable-prefix exclusion, and cancelled tool-result
preservation. This batch changes runtime tests, G5 fixtures/schema, Go shadow
code/tests, and docs only. It adds no Reasonix public protocol, auto-plan
setting, bridge/settings/provider default, default Go backend, renderer-visible
Go route, Rust/Tauri path, Kun identity, or top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime auto-route/step-limit composition | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto route|step limit" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 5 selected tests |
| G5 conformance fixture/schema | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go G5 shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix has runtime and G5 shadow evidence that auto-route cache, step-limit,
and cancel-result preservation compose without stable-prefix drift.
```

Rejected claims:

```text
No Reasonix auto-plan parity or public setting.
No Reasonix public control/session/task/planner protocol.
No live Go runtime, default Go backend, or renderer-visible Go route.
No live provider/cache superiority or live MCP/indexer parity.
No new Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No Rust/Tauri rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 753 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1481 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0037|Auto-Route Step/Cancel|Control Composition|control-composition|combined control|routeCacheReusedUntilStepLimit" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance packages/runtime/tests packages/runtime-go` | pass: expected synchronized refs, fixture/schema/test evidence, and evidence records only |

## Preload Bridge/API Sovereignty Addendum

Scope:
Source-level preload/API tests now pin the public renderer bridge to
`window.analytix`. This batch changes tests/docs only and adds no new bridge,
Reasonix SessionAPI/frontend protocol, Kun/deprecated GUI bridge alias, old
settings fallback, provider default, default Go backend, renderer-visible Go
route, Rust/Tauri path, Kun identity, or top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Preload bridge/API oracle | `npm run test -- src/preload/preload-sandbox.test.ts` | pass: 1 file / 3 tests |

Accepted claims:

```text
Analytix has executable preload/API evidence that the renderer public bridge
is `window.analytix` only.
```

Rejected claims:

```text
No new public bridge.
No Reasonix SessionAPI/frontend protocol parity.
No Kun/deprecated GUI bridge alias or old settings fallback.
No provider/default Go backend/Rust/Tauri/Kun identity change.
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 753 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1483 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0038|Preload Bridge/API Sovereignty|window\\.analytix|AnalytixApi|ReasonixSessionAPI|KunGuiApi" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/preload src/shared` | pass: expected synchronized refs, test evidence, and evidence records only |

## Renderer Thread Lifecycle HTTP Addendum

Scope:
Renderer runtime adapter tests now pin thread lifecycle calls to analytix-owned
HTTP paths. This batch changes tests/docs only and adds no new bridge, Reasonix
SessionAPI/thread protocol, Kun public protocol, deprecated bridge/settings
fallback, provider default, default Go backend, renderer-visible Go route,
Rust/Tauri path, Kun identity, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer thread lifecycle oracle | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts` | pass: 1 file / 24 tests |

Accepted claims:

```text
Analytix has renderer adapter evidence that thread lifecycle actions stay on
the analytix `/v1/threads` HTTP contract.
```

Rejected claims:

```text
No Reasonix SessionAPI/thread protocol parity.
No Kun public protocol or deprecated bridge/settings fallback.
No live Go route readiness or default Go backend.
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No packaged desktop QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 753 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1484 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0039|Renderer Thread Lifecycle|thread lifecycle HTTP|archive\\+match|/v1/threads\\?limit=25" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/renderer/src/agent/analytix-runtime.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Settings/Provider EndpointFormat Persistence Addendum

Scope:
Settings-store tests now pin runtime and provider endpoint-format persistence
to analytix-owned settings envelopes. This batch changes tests/docs only and
adds no Reasonix config root, Kun/deprecated settings envelope, provider
default change, bridge alias, default Go backend, renderer-visible Go route,
Rust/Tauri path, Kun identity, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Settings endpoint-format persistence oracle | `npm run test -- src/main/settings-store.test.ts` | pass: 1 file / 21 tests |

Accepted claims:

```text
Analytix has settings-store evidence that endpoint-format choices persist under
top-level `runtime` and `provider.providers`.
```

Rejected claims:

```text
No Reasonix config-root or provider settings protocol.
No Kun/deprecated `agentProvider` / `agents` settings envelope.
No live provider/cache superiority or credentialed provider matrix completion.
No Go provider client, default Go backend, or renderer-visible Go route.
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 78 files / 753 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1485 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0040|Settings/Provider EndpointFormat|endpoint-format choices|custom-messages|provider\\.providers" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/main/settings-store.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Runtime Provider-Selection Request-Shape Addendum

Scope:
Runtime model-dispatch tests now pin thread provider selection to analytix-owned
provider request shapes. This batch changes tests/docs only and adds no
Reasonix provider protocol, Reasonix config root, Kun/deprecated settings
envelope, provider default change, bridge alias, default Go backend,
renderer-visible Go route, Rust/Tauri path, Kun identity, or top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime provider-selection request-shape oracle | `npm --prefix packages/runtime test -- src/adapters/model/multi-provider-model-client.test.ts` | pass: 1 file / 2 tests |

Accepted claims:

```text
Analytix has runtime fake-fetch evidence that thread provider selection drives
the expected custom full endpoint and default fallback request shapes.
```

Rejected claims:

```text
No Reasonix provider protocol, config root, or SessionAPI.
No Kun/deprecated provider identity or settings envelope.
No live provider/cache superiority or credentialed provider matrix completion.
No Go provider client, default Go backend, or renderer-visible Go route.
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 755 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1485 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0041|Runtime Provider-Selection|provider-selection request-shape|custom-messages|gateway\\.example" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/adapters/model/multi-provider-model-client.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Scheduled Detector Custom Endpoint Inference Addendum

Scope:
Scheduled reminder detector tests now pin custom full endpoint inference for
`/messages` and `/chat/completions`. This batch changes tests/docs only and
adds no Reasonix provider protocol, Reasonix config root, Kun/deprecated
settings envelope, provider default change, bridge alias, default Go backend,
renderer-visible Go route, Rust/Tauri path, Kun identity, or top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Scheduled detector custom endpoint inference oracle | `npm run test -- src/main/claw-scheduled-task-detector.test.ts` | pass: 1 file / 5 tests |

Accepted claims:

```text
Analytix has scheduled detector fake-fetch evidence for custom `/messages` and
`/chat/completions` endpoint inference.
```

Rejected claims:

```text
No Reasonix provider protocol, config root, or SessionAPI.
No Kun/deprecated provider identity or settings envelope.
No live scheduled-task provider matrix or provider/cache superiority.
No Go scheduled detector, default Go backend, or renderer-visible Go route.
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 755 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0042|Scheduled Detector|scheduled detector fake-fetch|custom-path/messages|custom-path/chat/completions" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/main/claw-scheduled-task-detector.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Forbidden Public Runtime Route Addendum

Scope:
Runtime HTTP tests now prove forbidden upstream public routes return structured
404 responses. This batch changes tests/docs only and adds no Reasonix public
protocol, Kun public protocol, default Go backend, renderer-visible Go route,
Rust/Tauri path, Kun identity, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer route.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Forbidden public runtime route oracle | `npm --prefix packages/runtime test -- tests/http-server.test.ts` | pass: 1 file / 48 tests |

Accepted claims:

```text
Analytix has runtime HTTP negative-route evidence that forbidden upstream
public routes stay absent.
```

Rejected claims:

```text
No Reasonix public protocol or SessionAPI.
No Kun public protocol or forbidden product-entry route.
No live Go route, default Go backend, or renderer-visible Go backend.
No Rust/Tauri migration or native backend rewrite.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 764 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0043|Forbidden Public Runtime Route|negative-route|/v1/reasonix|/v1/runtime/go|/v1/mcp-indexer" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/http-server.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Rehydrated Task-Job Route Addendum

Scope:
Runtime task-job restart tests now prove rehydrated running/queued jobs remain
operable through authenticated `/v1/runtime/task-jobs/output|wait|kill` routes.
This batch changes tests/docs only and adds no Reasonix public sub-agent/job
protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer
route, default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Rehydrated task-job route oracle | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts` | pass: 1 file / 7 tests |

Accepted claims:

```text
Analytix has runtime HTTP harness evidence that rehydrated task jobs remain
operable through authenticated internal routes.
```

Rejected claims:

```text
No Reasonix public sub-agent/job protocol or SessionAPI.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No live Go route, default Go backend, or renderer-visible Go backend.
No packaged desktop restart QA or live child-agent execution parity.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 764 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0044|Rehydrated Task-Job Route|rehydrated task jobs|/v1/runtime/task-jobs/(output|wait|kill)" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/task-job-orchestration-oracle.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Task-Job Approval Deny No-Execute Addendum

Scope:
Runtime task-job provider tests now prove denied `task` and `parallel_tasks`
approvals create no durable jobs or child runs. This batch changes tests/docs
only and adds no Reasonix public sub-agent/job protocol, top-level Subagent/
Workflow/Create Loop/AutoResearch/MCP-indexer route, default Go backend,
renderer-visible Go route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job approval deny no-execute oracle | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts` | pass: 1 file / 8 tests |

Accepted claims:

```text
Analytix has runtime tool-host evidence that denied task-job approvals create
no durable jobs or child runs.
```

Rejected claims:

```text
No Reasonix public sub-agent/job protocol or SessionAPI.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No live Go route, default Go backend, or renderer-visible Go backend.
No packaged desktop approval-card QA or live child-agent approval flow.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0045|Task-Job Approval|approval deny no-execute|call_task_denied|call_parallel_denied" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/task-job-orchestration-oracle.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Go G5 Task-Job Approval Deny Shadow Replay Addendum

Scope:
Go G5 shadow replay now consumes the TS-owned task-job approval deny no-execute
fixture. This batch changes conformance fixtures, Go shadow output, tests, and
docs only; it adds no Go approval manager, Go Job Manager, Go HTTP server,
renderer-visible Go route, default Go backend, Reasonix public sub-agent/job
protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer
route, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job + Go G5 conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix Go G5 shadow replay consumes TS-owned task-job approval denial
evidence and preserves the no-side-effect contract in fixtures.
```

Rejected claims:

```text
No Go approval manager or Go Job Manager readiness.
No live Go task/job execution, Go route, default Go backend, or renderer-visible
Go backend.
No Reasonix public sub-agent/job protocol or SessionAPI.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No packaged desktop approval-card QA or live child-agent approval flow.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0046|Go G5 Task-Job Approval|approvalDenyNoExecute|appr_call_task_denied|createsDurableJobs" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures packages/runtime-go packages/runtime/tests/go-runtime-conformance.test.ts` | pass: expected synchronized refs, code evidence, and evidence records only |

## Go G5 Task Approval Deny Executable Control Addendum

Scope:
Go G5 executable shadow now computes task approval denial no-side-effect output
from TS-owned fixtures. This batch changes conformance fixtures, Go shadow
output, tests, and docs only; it adds no Go approval manager, Go Job Manager,
Go HTTP server, renderer-visible Go route, default Go backend, Reasonix public
sub-agent/job protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/
MCP-indexer route, Rust/Tauri path, Kun identity, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Go G5 conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.227s` |

Accepted claims:

```text
Analytix Go G5 executable shadow computes task approval denial no-side-effect
results from TS-owned fixtures.
```

Rejected claims:

```text
No Go approval manager or Go Job Manager readiness.
No live Go task/job execution, Go route, default Go backend, or renderer-visible
Go backend.
No Reasonix public sub-agent/job protocol or SessionAPI.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No packaged desktop approval-card QA or live child-agent approval flow.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0047|Go G5 Task Approval Deny Executable|controlExecutableCases\\.approvalDeny|replayG5ApprovalDeny|approvalItemCount" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures packages/runtime-go packages/runtime/tests/go-runtime-conformance.test.ts` | pass: expected synchronized refs, code evidence, and evidence records only |

## Go G3/G5 Provider Cache Accounting Shadow Addendum

Scope:
Go G3/G5 shadow now computes provider cache accounting from TS-owned fixtures.
This batch changes conformance fixtures, Go shadow output, tests, and docs
only; it adds no live provider credential matrix, Go provider client, Go HTTP
server, renderer-visible Go route, default Go backend, Reasonix provider
protocol, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer
route, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache + Go G3/G5 conformance | `npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.431s` |

Accepted claims:

```text
Analytix Go G3/G5 shadow computes provider cache accounting from TS-owned
fixtures, including unsupported providers staying unknown.
```

Rejected claims:

```text
No live provider/cache superiority or credentialed provider matrix.
No Go provider client, Go route, default Go backend, or renderer-visible Go
backend.
No Reasonix provider protocol.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer route.
No packaged provider settings QA.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0048|Go G3/G5 Provider Cache Accounting|cacheAccounting|buildG3ProviderCacheAccounting|buildG5ProviderCacheAccounting|unsupportedProvidersCountedAsMisses" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures packages/runtime-go packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts packages/runtime/tests/go-runtime-conformance.test.ts` | pass: expected synchronized refs, code evidence, and evidence records only |

## Auto-Router Failure Usage Isolation Addendum

Scope:
Auto-router failure fallback now proves classifier usage/cache telemetry is not
recorded as main turn usage. This batch changes runtime loop tests and docs
only; it adds no Reasonix auto-plan config, project/local override,
user-visible auto-plan setting, Reasonix public protocol, Go route, default Go
backend, top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route,
Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router failure isolation | `npm --prefix packages/runtime test -- tests/loop.test.ts -t 'falls back to a concrete heuristic model without recording router usage'` | pass: 1 file / 1 test |

Accepted claims:

```text
Analytix auto-router failure falls back to heuristic routing without recording
router usage/cache telemetry as main turn usage.
```

Rejected claims:

```text
No Reasonix auto-plan config parity.
No user-visible auto-plan settings.
No managed runtime settings rebuild lifecycle coverage.
No live provider/cache superiority.
No default Go backend, renderer-visible Go route, or Go runtime parity.
No release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Docs/spec/ledger consistency | `rg -n "D-0049|Auto-Router Failure Usage Isolation|without recording router usage|_auto_router|UsageService" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/loop.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Task-Job Stale Restart Reconciliation Addendum

Scope:
Queued and running stale task jobs now reconcile to explicit failed state after
runtime restart in TS runtime tests, and the same behavior is mirrored in Go G5
executable shadow. This batch changes oracle/test/shadow/docs only; it adds no
Reasonix public sub-agent/job protocol, SessionAPI, renderer-visible Go route,
default Go backend, top-level Subagent/Workflow/Create Loop/AutoResearch/
MCP-indexer route, Rust/Tauri path, Kun identity, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job stale restart + Go G5 binding | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.508s` |

Accepted claims:

```text
Analytix reconciles queued and running stale task jobs to explicit failed state
in TS runtime tests and Go G5 executable shadow.
```

Rejected claims:

```text
No Reasonix public sub-agent/job protocol parity.
No Reasonix SessionAPI or public job route.
No live Go Job Manager, renderer-visible Go route, or default Go backend.
No packaged desktop restart QA or release readiness.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
top-level hidden capability route.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 79 files / 765 tests |
| Workspace tests | `npm run test` | pass: 207 files / 1487 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |

## Auto-Router Classifier Request Contract Addendum

Scope:
Auto-router unit tests now pin `_auto_router` as an isolated short-JSON
classifier path with timeout fallback and fingerprinted currentness. This batch
changes runtime tests and docs only; it adds no Reasonix auto-plan config,
project/local override, user-visible planner toggle, Reasonix public protocol,
Go route, default Go backend, top-level Subagent/Workflow/Create Loop/
AutoResearch/MCP-indexer route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router classifier contract | `npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |

Accepted claims:

```text
Analytix `_auto_router` stays an isolated short-JSON classifier path with
fingerprinted currentness and heuristic timeout fallback.
```

Rejected claims:

```text
No Reasonix auto-plan config parity.
No user-visible planner/auto-plan toggles.
No Reasonix controller/SessionAPI protocol.
No Go auto-router parity, Go route, default Go backend, or renderer-visible Go
backend.
No packaged desktop model-routing QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1499 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0066|Auto-Router Classifier Request|isolated short-JSON|auto-model-router.test.ts" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/auto-model-router.test.ts` | pass: expected synchronized refs, test evidence, and evidence records only |

## Planner-Executor Transcript Propagation Addendum

Scope:
Planner-executor runtime tests now prove identity-checked transcript refs can
be persisted on durable executor jobs, and G5 shadow evidence replays the
requirement. This batch changes internal runtime/job metadata, conformance
fixtures, Go shadow output, tests, and docs only; it adds no Reasonix
Subagent/SessionAPI protocol, public task/job route, renderer-visible Go route,
default Go backend, top-level Subagent/Workflow/Create Loop/AutoResearch/
MCP-indexer route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Planner executor transcript + Go G5 binding | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.488s` |

Accepted claims:

```text
Analytix planner-executor jobs can persist identity-checked transcript refs as
internal durable task-job metadata, and G5 shadow evidence replays the
requirement.
```

Rejected claims:

```text
No Reasonix public Subagent/SessionAPI/transcript protocol.
No top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
route.
No live Go Job Manager, Go route, default Go backend, or renderer-visible Go
backend.
No packaged desktop sub-agent QA, full Reasonix planner/executor parity, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0067|Planner-Executor Transcript Propagation|requiresTranscriptPropagation|transcriptFor" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/delegation/planner-executor-coordinator.ts packages/runtime/tests/task-job-orchestration-oracle.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, code evidence, and evidence records only |

## MCP Search Meta-Tool Trust Boundary Addendum

Scope:
MCP search-mode runtime tests now prove meta-tools respect workspace trust and
approval before MCP client execution, and G4 shadow evidence replays the
requirement. This batch changes internal MCP/tool lifecycle fixtures, Go shadow
output, tests, and docs only; it adds no Reasonix MCP-indexer protocol, public
MCP route, renderer-visible Go route, default Go backend, top-level
MCP-indexer/Workflow/Create Loop/Subagent/AutoResearch route, Rust/Tauri path,
Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP search meta-tools + G4 binding | `npm --prefix packages/runtime test -- tests/mcp-tool-provider.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 23 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.409s` |

Accepted claims:

```text
Analytix MCP search meta-tools respect workspace trust and approval before MCP
client execution, and G4 shadow evidence replays the no-execute boundary.
```

Rejected claims:

```text
No Reasonix MCP-indexer public protocol.
No top-level MCP-indexer, Workflow, Create Loop, Subagent, or AutoResearch
route.
No live Go MCP client, Go route, default Go backend, or renderer-visible Go
backend.
No credentialed MCP matrix, packaged desktop MCP QA, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0068|MCP Search Meta-Tool Trust|searchMetaTools|mcpSearchCallDeniedNoExecute" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/mcp-tool-provider.test.ts packages/runtime/src/conformance/fixtures/mcp-tool-lifecycle-oracle.json packages/runtime-go/shadow_g3g4.go` | pass: expected synchronized refs, test evidence, and evidence records only |

## Custom Messages Full Endpoint Request Shape Addendum

Scope:
Provider-cache oracle now covers custom full `/messages` endpoint exact URL and
Anthropic Messages request shape. This batch changes provider/cache fixtures
and docs only; it adds no Reasonix provider protocol, public provider route,
renderer-visible Go route, default Go backend, top-level hidden capability
route, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider request-shape oracle | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |

Accepted claims:

```text
Analytix has fake-fetch evidence that custom full `/messages` endpoints keep
exact URL and Anthropic Messages request shape.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed OpenAI/Anthropic/custom provider matrix.
No Reasonix provider protocol.
No Go provider client, Go route, default Go backend, or renderer-visible Go
backend.
No packaged provider settings QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0069|Custom Messages Full Endpoint|custom-messages-full-endpoint-request-shape" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/provider-cache-oracle.json packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` | pass: expected synchronized refs, fixtures, and evidence records only |

## Custom Chat Full Endpoint Request Shape Addendum

Scope:
Provider-cache oracle now covers custom full `/chat/completions` endpoint exact
URL and OpenAI-compatible chat request shape. This batch changes
provider/cache fixtures and docs only; it adds no Reasonix provider protocol,
public provider route, renderer-visible Go route, default Go backend, top-level
hidden capability route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider request-shape oracle and Go shadow bindings | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix has fake-fetch evidence that custom full `/chat/completions`
endpoints keep exact URL and OpenAI-compatible chat request shape.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed OpenAI/Anthropic/custom provider matrix.
No Reasonix provider protocol.
No Go provider client, Go route, default Go backend, or renderer-visible Go
backend.
No packaged provider settings QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0070|Custom Chat Full Endpoint|custom-chat-full-endpoint-request-shape" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/provider-cache-oracle.json packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json` | pass: expected synchronized refs, fixtures, and evidence records only |

## Auto-Router Fingerprint Currentness Addendum

Scope:
Auto-router fingerprint proof now covers classifier request-contract drift.
This batch changes the runtime auto-router helper, its focused tests, and docs
only; it adds no Reasonix auto-plan config, project/local auto-plan override,
public controller protocol, renderer-visible Go route, default Go backend,
top-level hidden capability route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router fingerprint currentness | `npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |

Accepted claims:

```text
Analytix has runtime evidence that auto-router classifier request-contract
drift changes route-cache fingerprints.
```

Rejected claims:

```text
No Reasonix auto-plan config parity.
No local/project auto-plan override.
No Reasonix controller API or public protocol.
No Go router, Go route, default Go backend, or renderer-visible Go backend.
No packaged desktop controller QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0071|Auto-Router Fingerprint Currentness|Auto-Router Request Contract Fingerprint|AutoModelRouterFingerprintInput" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/loop/auto-model-router.ts packages/runtime/tests/auto-model-router.test.ts` | pass: expected synchronized refs, helper, test evidence, and evidence records only |

## Connect Phone Copy Sovereignty Addendum

Scope:
New Connect Phone prompt/title/schema/log copy uses Connect Phone while legacy
Claw recognizers remain compatibility-only for old sessions. This batch changes
copy, compatibility recognizers, tests, and docs only; it adds no Kun/Reasonix
identity, deprecated bridge/settings fallback, renderer-visible Go route,
default Go backend, top-level hidden capability route, or Rust/Tauri path.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Connect Phone prompt/runtime/renderer tests | `npm run test -- src/shared/app-settings.test.ts src/main/schedule-runtime.test.ts src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts src/renderer/src/store/chat-store-helpers.test.ts src/renderer/src/store/chat-store-claw-actions.test.ts --no-file-parallelism --maxWorkers=1` | pass: 5 files / 117 tests |
| New-copy Claw scan | `if rg -n "export const CLAW_.*\\[Claw|Optional Claw IM|Claw IM webhook|title:.*Claw IM" src/shared/app-settings-prompts.ts src/shared/app-settings-types.ts src/main/claw-schedule-mcp-server.ts src/main/claw-runtime.ts; then exit 1; fi` | pass: no new-copy hits |

Accepted claims:

```text
Analytix emits Connect Phone copy for new Connect Phone prompts, titles,
schema descriptions, and logs while retaining legacy Claw recognizers.
```

Rejected claims:

```text
No complete internal compatibility rename.
No removal of legacy Claw history support.
No Kun/Reasonix/Claw public product identity.
No Go route, default Go backend, or renderer-visible Go backend.
No packaged desktop Connect Phone QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: cached `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0072|Connect Phone Copy Sovereignty|Connect Phone copy" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/shared/app-settings-prompts.ts src/main/claw-runtime.ts src/renderer/src/store/chat-store-helpers.ts` | pass: expected synchronized refs, tests, and evidence records only |

## Go G5 Durable Runner Restart Executable Shadow Addendum

Scope:
Go G5 shadow now executable-replays durable task-job restart output, offset,
and queued kill behavior from TS-owned fixtures. This batch changes G5
fixtures, conformance tests, Go shadow code, and docs only; it adds no live Go
Job Manager, renderer-visible Go route, default Go backend, top-level hidden
capability route, Rust/Tauri path, Kun identity, Reasonix protocol, or
deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 durable runner restart conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow can executable-replay durable task-job restart output,
offset, and kill behavior from TS-owned fixtures.
```

Rejected claims:

```text
No live Go Job Manager.
No Go task-job routes, Go route, default Go backend, or renderer-visible Go
backend.
No Reasonix SessionAPI or public sub-agent/job protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop sub-agent/task-job QA or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0073|Durable Runner Restart|restartDrill|replayG5TaskJobRestartDrill" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_test.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Approval/User-Input Abort Cleanup Executable Shadow Addendum

Scope:
Go G5 shadow now executable-replays approval/user-input abort cleanup state,
late GUI action statuses, pending cleanup, and replay event order from
TS-owned fixtures. This batch changes G5 fixtures, conformance tests, Go shadow
code, and docs only; it adds no live Go approval/user-input manager,
renderer-visible Go route, default Go backend, top-level hidden capability
route, Rust/Tauri path, Kun identity, Reasonix protocol, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 abort cleanup conformance | `npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow can executable-replay approval/user-input abort cleanup
states, late GUI action statuses, pending cleanup, and replay event order from
TS-owned fixtures.
```

Rejected claims:

```text
No live Go approval manager or user-input manager.
No Go task-job routes, Go route, default Go backend, or renderer-visible Go
backend.
No Reasonix SessionAPI or public approval/user-input protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop approval-card QA, Go G5 parity, G6 readiness, or release
readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0074|Abort Cleanup|abortCleanup|replayG5AbortCleanup|lateApprovalDecisionStatus" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_test.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Abort Cleanup Replay Summary Addendum

Scope:
G5 `controlReplay`, `approvalUserInputReplay`, and executable shadow now all
cover approval/user-input abort cleanup from TS-owned fixtures. This batch
changes G5 fixtures, conformance tests, Go shadow slice output, and docs only;
it adds no live Go approval/user-input manager, renderer-visible Go route,
default Go backend, top-level hidden capability route, Rust/Tauri path, Kun
identity, Reasonix protocol, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 abort cleanup replay-summary conformance | `npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow summary and executable output both cover
approval/user-input abort cleanup from TS-owned fixtures.
```

Rejected claims:

```text
No live Go approval manager or user-input manager.
No Go route, default Go backend, or renderer-visible Go backend.
No Reasonix SessionAPI or public approval/user-input protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop approval-card QA, Go G5 parity, G6 readiness, or release
readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0075|Abort Cleanup Replay Summary|abortReplayKinds|pendingAfterAbortCleanup|controlReplay\\.abortCleanup" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 MCP Search Meta-Tool Replay Addendum

Scope:
G5 `mcpReplay` now carries MCP search meta-tool trust and denied no-execute
evidence from TS-owned fixtures. This batch changes G5 fixtures, conformance
tests, Go shadow slice output, and docs only; it adds no live Go MCP client,
renderer-visible Go route, default Go backend, top-level MCP-indexer route,
Rust/Tauri path, Kun identity, Reasonix protocol, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP search replay conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 11 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers MCP search meta-tool trust and denied
no-execute behavior from TS-owned fixtures.
```

Rejected claims:

```text
No live Go MCP client.
No Go MCP route, Go route, default Go backend, or renderer-visible Go backend.
No Reasonix MCP-indexer protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No credentialed MCP matrix, packaged desktop MCP QA, Go G5 parity, G6
readiness, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0076|MCP Search Meta-Tool Replay|searchMetaToolNames|searchCallDeniedNoExecute|searchUntrustedSearchedTools" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Parallel Task Dependency Validation Addendum

Scope:
G5 `jobReplay` now carries `parallel_tasks` dependency validation evidence from
TS-owned fixtures. This batch changes G5 fixtures, conformance tests, Go shadow
slice output, and docs only; it adds no live Go Job Manager,
renderer-visible Go route, default Go backend, top-level Subagent/Workflow/
Create Loop route, Rust/Tauri path, Kun identity, Reasonix protocol, or
deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 parallel validation conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers `parallel_tasks` dependency validation from
TS-owned fixtures.
```

Rejected claims:

```text
No live Go Job Manager.
No public sub-agent/job protocol.
No Go route, default Go backend, or renderer-visible Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop sub-agent/task-job QA, Go G5 parity, G6 readiness, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0077|Parallel Task Dependency Validation|parallelValidation|single_task|unknownDependencyError" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Task Job Lifecycle Replay Addendum

Scope:
G5 `jobReplay` now carries base task-job lifecycle evidence from TS-owned
fixtures: foreground completion, background cross-turn output/final completion,
and wait/output/kill cancellation status/error. This batch changes G5 fixtures,
conformance tests, Go shadow slice output, and docs only; it adds no live Go
Job Manager, renderer-visible Go route, default Go backend, top-level
Subagent/Workflow/Create Loop/AutoResearch route, Rust/Tauri path, Kun identity,
Reasonix protocol, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task lifecycle conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers base task-job lifecycle from TS-owned
fixtures.
```

Rejected claims:

```text
No live Go Job Manager.
No public sub-agent/job protocol.
No Go route, default Go backend, or renderer-visible Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop sub-agent/task-job QA, Go G5 parity, G6 readiness, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0078|Task Job Lifecycle|jobReplay\\.lifecycle|foreground complete|waitOutputKill" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Task Job Route Boundary Replay Addendum

Scope:
G5 `jobReplay` now carries task-job route auth and forbidden top-level route
evidence from TS-owned fixtures: protected wait/output/kill route ids,
unauthorized `401`, and `/v1/workflow` / `/v1/create-loop` /
`/v1/autoresearch` as forbidden surfaces. This batch changes G5 fixtures,
conformance tests, Go shadow slice output, and docs only; it adds no live Go
task-job routes, live Go Job Manager, renderer-visible Go route, default Go
backend, top-level Subagent/Workflow/Create Loop/AutoResearch route,
Rust/Tauri path, Kun identity, Reasonix protocol, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task route-boundary conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers task-job route auth and forbidden top-level
route evidence from TS-owned fixtures.
```

Rejected claims:

```text
No live Go task-job routes.
No live Go Job Manager.
No public sub-agent/job protocol.
No Go route, default Go backend, or renderer-visible Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop sub-agent/task-job QA, Go G5 parity, G6 readiness, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0079|Task Job Route Boundary|jobReplay\\.routeBoundary|forbiddenTopLevelRoutes|unauthorizedStatus" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 MCP Core Lifecycle Replay Addendum

Scope:
G5 `mcpReplay` now carries MCP connect/disconnect/reload/cancel/error lifecycle
evidence from TS-owned fixtures. This batch changes G5 fixtures, conformance
tests, Go shadow slice output, and docs only; it adds no live Go MCP client,
Go MCP route, renderer-visible Go route, default Go backend, top-level
MCP-indexer route, Rust/Tauri path, Kun identity, Reasonix protocol, or
deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP lifecycle conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 11 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers MCP core lifecycle evidence from TS-owned
fixtures.
```

Rejected claims:

```text
No live Go MCP client.
No Go MCP route, Go route, default Go backend, or renderer-visible Go backend.
No Reasonix MCP-indexer protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No credentialed MCP matrix, packaged desktop MCP QA, Go G5 parity, G6
readiness, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0080|MCP Core Lifecycle|mcpReplay\\.lifecycle|connectToolNames|cancelExecuted|schemaOrderStable" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Approval Decision Route Replay Addendum

Scope:
G5 `approvalUserInputReplay` now carries approval decision route and replay
ordering evidence from TS-owned fixtures. This batch changes G5 fixtures,
conformance tests, Go shadow slice output, and docs only; it adds no live Go
approval manager, renderer-visible Go route, default Go backend, Reasonix
SessionAPI/ask protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route, Rust/Tauri path, Kun identity, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 approval decision conformance | `npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay covers approval decision route evidence from
TS-owned fixtures.
```

Rejected claims:

```text
No live Go approval manager.
No Reasonix SessionAPI/ask protocol.
No Go route, default Go backend, or renderer-visible Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop approval-card QA, Go G5 parity, G6 readiness, or release
readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0081|Approval Decision Route|approvalDecision|secondApprovalDecisionStatus|replayKindsInOrder" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Offline Provider/Cache Parity Seal Addendum

Scope:
G5 `cacheReplay` now carries a fixture-only provider/cache parity seal from
TS-owned fixtures. The seal records DeepSeek stable/equivalent prefix equality,
prefix item/tool hash stability, stable cache hit/miss/rate, diagnostics
support, usage/request-shape counts, endpoint families, release guard status,
and explicit no-live-superiority policy. This batch changes G5 fixtures,
conformance tests, Go shadow slice output, and docs only; it adds no live
provider matrix, live Go provider client, renderer-visible Go route, default Go
backend, Reasonix provider protocol, Kun identity, Rust/Tauri path, or
deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache parity conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix has fixture-backed offline provider/cache parity evidence for
DeepSeek stable prefix/cache and multi-provider request/usage regressions.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go backend.
No Reasonix provider protocol.
No packaged provider settings QA, Go G5 parity, G6 readiness, or release
readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0082|Offline Provider/Cache Parity|offlineParitySeal|stablePrefixEquivalent|mayClaimLiveSuperiority|requestShapeEndpointFormats" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Cache Drift Attribution Replay Addendum

Scope:
G5 `cacheReplay` now carries provider-cache drift attribution from TS-owned
fixtures. The replay records previous/current prefix hashes, stable
system/prefixItems proof, changed tool/provider/model/endpoint flags, expected
reasons, and unsupported telemetry/cache-hit-rate status. This batch changes G5
fixtures, conformance tests, Go shadow slice output, and docs only; it adds no
live provider matrix, live Go provider client, renderer-visible Go route,
default Go backend, Reasonix provider protocol, dynamic-context stable prefix,
Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache drift conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replay attributes provider-cache drift to allowed
tool/provider/model changes while preserving stable system/prefixItems hashes.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go backend.
No Reasonix provider protocol.
No dynamic workspace context, selected text, timestamps, credentials, or
sidecar material in stable prefix.
No packaged provider settings QA, Go G5 parity, G6 readiness, or release
readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0083|Cache Drift Attribution|driftAttribution|toolsHashChanged|cacheHitRateKnown|expectedReasons" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G3 Cache Drift Attribution Replay Addendum

Scope:
G3 provider conformance now carries cache drift attribution from TS-owned
fixtures. The G3 fixture stores raw previous/current drift shapes and
`expectedOutput` stores compact attribution output; Go shadow computes that
output in `BuildG3ProviderConformanceOutput`. This batch changes G3 fixtures,
conformance tests, Go shadow output, and docs only; it adds no live provider
matrix, provider request behavior change, live Go provider client,
renderer-visible Go route, default Go backend, Reasonix provider protocol,
dynamic-context stable prefix, Kun identity, Rust/Tauri path, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache G3 drift conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G3 provider conformance computes cache drift attribution from
TS-owned raw shapes while preserving stable prefix hygiene.
```

Rejected claims:

```text
No live provider/cache superiority.
No provider request behavior changes.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go backend.
No Reasonix provider protocol.
No dynamic workspace context, selected text, timestamps, credentials, or
sidecar material in stable prefix.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0084|G3 Cache Drift|cacheDriftAttribution|BuildG3ProviderConformanceOutput|Go G3 Cache Drift" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts packages/runtime-go/shadow_g3g4.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G3 Provider Request Shape Replay Addendum

Scope:
G3 provider conformance now carries provider request URL/body/tool-shape
coverage from TS-owned fixtures. The G3 fixture stores `requestShapeMatrix` and
`expectedOutput.requestShapeSummary`; Go shadow computes the summary in
`BuildG3ProviderConformanceOutput`. This batch changes G3 fixtures,
conformance tests, Go shadow output, and docs only; it adds no live provider
matrix, provider request behavior change, live Go provider client,
renderer-visible Go route, default Go backend, Reasonix provider protocol, Kun
identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache G3 request-shape conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G3 provider conformance replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Rejected claims:

```text
No live provider/cache superiority.
No provider request behavior changes.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go backend.
No Reasonix provider protocol.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0085|G3 Provider Request Shape|requestShapeMatrix|requestShapeSummary|BuildG3ProviderConformanceOutput|provider request URL/body" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g3-provider-streaming-usage-cache-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts packages/runtime-go/shadow_g3g4.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Provider Request Shape Replay Addendum

Scope:
G5 full-loop shadow now carries provider request URL/body/tool-shape coverage
from TS-owned fixtures. The G5 fixture stores `cacheReplay.requestShapeReplay`;
Go shadow computes the summary in `BuildG5ShadowSlicesOutput`. This batch
changes G5 fixtures, conformance tests, Go shadow output, and docs only; it
adds no live provider matrix, provider request behavior change, live Go
provider client, renderer-visible Go route, default Go backend, Reasonix
provider protocol, Kun identity, Rust/Tauri path, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache G5 request-shape conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 15 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 full-loop shadow replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Rejected claims:

```text
No live provider/cache superiority.
No provider request behavior changes.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go backend.
No Reasonix provider protocol.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0086|G5 Provider Request Shape|requestShapeReplay|BuildG5ShadowSlicesOutput|provider request URL/body" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Session Route Status Replay Addendum

Scope:
G5 full-loop shadow now carries thread/session route status and SSE replay
semantics from TS-owned G2 HTTP/SSE fixtures. The G5 fixture stores
`sessionReplay.routeStatusReplay`; Go shadow computes the summary in
`BuildG5ShadowSlicesOutput`. This batch changes G5 fixtures, conformance tests,
Go shadow output, and docs only; it adds no live Go route server,
renderer-visible Go route, default Go backend, Reasonix SessionAPI/public
protocol, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G2/G5 route status conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 full-loop shadow replays thread/session route status and SSE
semantics from TS-owned G2 HTTP/SSE fixtures.
```

Rejected claims:

```text
No live Go route server.
No renderer-visible Go route or default Go backend.
No Reasonix SessionAPI/public protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop restart/resume QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0087|Session Route Status|routeStatusReplay|BuildG5ShadowSlicesOutput|thread/session route status|SSE replay" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Provider Streaming Usage Replay Addendum

Scope:
G5 full-loop shadow now carries provider streaming usage evidence from the
TS-owned G3 provider streaming/usage/cache oracle. The G5 fixture stores
`providerStreamingReplay`; Go shadow computes the summary in
`BuildG5ShadowSlicesOutput` by parsing the fixture SSE `usage` frame and
comparing it with the `deepseek-prompt-cache` expected usage case. This batch
changes G5 fixtures, conformance tests, Go shadow output, and docs only; it
adds no live Go provider client, provider request/stream parser behavior,
renderer-visible Go route, default Go backend, Reasonix provider protocol, Kun
identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 provider streaming usage conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 full-loop shadow replays provider streaming usage evidence from
TS-owned G3 fixtures.
```

Rejected claims:

```text
No live provider/cache superiority.
No provider request or stream parser behavior changes.
No credentialed provider matrix.
No live Go provider client, default Go backend, or renderer-visible Go route.
No Reasonix provider protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0088|Provider Streaming Usage|providerStreamingReplay|BuildG5ShadowSlicesOutput|provider streaming usage|usageEventMatchesExpectedCase" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_g3g4.go packages/runtime-go/shadow_test.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Go G5 Task-Job Route Executable Replay Addendum

Scope:
G5 full-loop shadow now carries internal task-job route executable evidence from
the TS-owned task-job orchestration oracle. The source fixture stores
`routeExecutable`; TS route tests bind real wait/output/kill route assertions to
that oracle; G5 stores `jobReplay.routeExecutable`; Go shadow computes the
summary in `BuildG5ShadowSlicesOutput`. This batch changes fixtures,
conformance tests, Go shadow output, and docs only; it adds no live Go
task-job route server, renderer-visible Go route, default Go backend, Reasonix
SessionAPI/public job protocol, Kun identity, Rust/Tauri path, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job route executable conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 full-loop shadow replays internal task-job route executable
evidence from TS-owned fixtures.
```

Rejected claims:

```text
No live Go Job Manager.
No live Go task-job route server.
No renderer-visible Go route or default Go backend.
No Reasonix SessionAPI/public sub-agent/job protocol.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer.
No packaged desktop task-job QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 770 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0089|Task-Job Route Executable|routeExecutable|jobReplay\\.routeExecutable|task-job route executable|BuildG5ShadowSlicesOutput" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/task-job-orchestration-oracle.json packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/task-job-orchestration-oracle.test.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_test.go` | pass: expected synchronized refs, fixture, schema, tests, and evidence records only |

## Provider Live-Local HTTP Executable Proof Addendum

Scope:
Provider/cache proof now executes all provider usage cases and all
request-shape cases through a no-credential local HTTP provider while
preserving the original provider base URL/host for request construction. This
batch changes `provider-cache-proof.test.ts` and docs only; it adds no provider
client behavior change, settings change, bridge change, runtime route, default
Go backend, live Go provider client, Reasonix provider protocol, Kun identity,
Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider live-local HTTP proof | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |

Accepted claims:

```text
Analytix provider/cache proof executes every provider usage and request-shape
oracle case through no-credential local HTTP while preserving provider request
construction.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed provider matrix or external load testing.
No provider request, stream, or usage behavior changes.
No live Go provider client, default Go backend, or renderer-visible Go route.
No Reasonix provider protocol.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 772 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0090|Provider Live-Local HTTP|live-local HTTP|provider live-local|forwardToLocalProvider|executes every provider usage oracle case|executes every provider request-shape oracle case" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/tests/provider-cache-proof.test.ts` | pass: expected synchronized refs, test helpers, specs, ledger, scorecard, and evidence records only |

## G5 Provider Live-Local Proof Summary Replay Addendum

Scope:
Provider live-local HTTP proof is now explicit oracle metadata and G5 shadow
summary. `provider-cache-oracle.liveLocalHttpProof` records fixture-only local
HTTP transport, no live credentials, original provider base URL preservation,
5 usage cases, 7 request-shape cases, 12 expected POSTs, endpoint formats,
provider families, and no-live-superiority policy. G5 `cacheReplay.liveLocalHttpProof`
derives counts and endpoint formats from the TS-owned oracle. This batch adds
no live Go provider client, provider behavior change, settings change, bridge
change, runtime route, default Go backend, Reasonix provider protocol, Kun
identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/G5 live-local summary conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 16 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays fixture-owned provider live-local HTTP proof
metadata from the TS provider-cache oracle.
```

Rejected claims:

```text
No live Go provider parity.
No live provider/cache superiority.
No credentialed provider matrix or external load testing.
No provider request, stream, or usage behavior changes.
No default Go backend or renderer-visible Go route.
No Reasonix provider protocol.
No packaged provider settings QA, Go G5/G6 parity, or release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0091|G5 Provider Live-Local Proof Summary|liveLocalHttpProof|Provider live-local HTTP proof metadata|provider live-local proof summary|BuildG5ShadowSlicesOutput|providerLiveLocalHttpProofSummary|buildProviderLiveLocalHTTPProof" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/provider-cache-oracle.json packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/provider-cache-proof.test.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 MCP Live-Local Indexer Summary Replay Addendum

Scope:
G5 shadow now carries richer MCP live-local indexer summary evidence. The new
`mcpReplay.liveLocalIndexer` fields include server id, cwd,
low-priority/background-start flags, retry attempts, active paths, tombstone
count, snapshot restart, late tombstone, redacted diagnostics,
`leaksSecret:false`, and `topLevelRouteExposed:false`. This batch changes G5
fixtures, conformance tests, Go shadow output, and docs only; it adds no live
Go MCP client, MCP route, default Go backend, renderer-visible Go route,
Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, Kun identity,
Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP live-local indexer summary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays MCP live-local indexer lifecycle evidence from the
TS-owned MCP oracle.
```

Rejected claims:

```text
No live Go MCP client readiness.
No Reasonix MCP-indexer protocol parity.
No top-level MCP-indexer navigation.
No credentialed MCP matrix or packaged MCP QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0092|G5 MCP Live-Local Indexer|mcpReplay\\.liveLocalIndexer|liveLocalIndexer|buildG5MCPLiveLocalIndexerReplay|MCP live-local indexer summary|runLiveLocalIndexerProof" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 MCP Approval Annotation Replay Addendum

Scope:
G5 shadow now carries destructive/open-world MCP approval annotation evidence.
The new `mcpReplay.approvalAnnotations` fields include server id, original tool
name, normalized tool name, destructive/open-world flags, approval id, deny
decision, approval result kind, `executed:false`, and `deniedNoExecute:true`.
This batch changes G5 fixtures, conformance tests, Go shadow output, and docs
only; it adds no live Go approval manager, live Go MCP client, MCP route,
default Go backend, renderer-visible Go route, Reasonix MCP-indexer protocol,
top-level MCP-indexer navigation, Kun identity, Rust/Tauri path, or deprecated
bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP approval annotation conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays destructive/open-world MCP approval metadata and
denied no-execute evidence from the TS-owned MCP oracle.
```

Rejected claims:

```text
No live Go approval-manager readiness.
No live Go MCP client readiness.
No Reasonix MCP-indexer protocol parity.
No top-level MCP-indexer navigation.
No credentialed MCP matrix or packaged MCP QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0093|G5 MCP Approval Annotation|mcpReplay\\.approvalAnnotations|approvalAnnotations|buildG5MCPApprovalAnnotationReplay|deniedNoExecute|destructive/open-world MCP" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Task Parent Goal Evidence Replay Addendum

Scope:
G5 shadow now carries task/sub-agent parent-goal evidence requirements. The new
`jobReplay.parentGoalEvidence` fields include `requiresActiveGoal:true`,
`evidenceLedgered`, `evidenceLedgerError`, `usesReasonixProtocol:false`, and
`topLevelRouteExposed:false`. This batch changes G5 fixtures, conformance
tests, Go shadow output, and docs only; it adds no live Go Job Manager, public
sub-agent/job protocol, default Go backend, renderer-visible Go route, top-level
Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation, Kun identity,
Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task parent-goal evidence conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays task parent-goal evidence requirements from the
TS-owned task-job oracle.
```

Rejected claims:

```text
No live Go Job Manager readiness.
No Reasonix public sub-agent/job protocol parity.
No top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation.
No packaged sub-agent/task-job QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0094|G5 Task Parent Goal Evidence|jobReplay\\.parentGoalEvidence|parentGoalEvidence|TaskParentGoalEvidence|evidenceLedgered|evidenceLedgerError" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Planner-Executor Detail Replay Addendum

Scope:
G5 shadow now carries planner-executor detail evidence. The extended
`jobReplay.plannerExecutor` fields include `skippedReason`, `cancelReason`,
`outputOffsetJobCount`, and `transcriptPropagationJobCount`. This batch changes
G5 fixtures, conformance tests, Go shadow output, and docs only; it adds no
planner product toggle, live Go Job Manager, public sub-agent/job protocol,
default Go backend, renderer-visible Go route, top-level Subagent/Workflow/
Create Loop/AutoResearch/MCP-indexer navigation, Kun identity, Rust/Tauri path,
or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 planner-executor detail conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays planner-executor detail evidence from the TS-owned
task-job oracle.
```

Rejected claims:

```text
No planner product toggle.
No live Go Job Manager readiness.
No Reasonix public planner/sub-agent/job protocol parity.
No top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation.
No packaged planner/sub-agent QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0095|G5 Planner-Executor Detail|jobReplay\\.plannerExecutor|skippedReason|cancelReason|outputOffsetJobCount|transcriptPropagationJobCount" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Task Tool Contract Boundary Replay Addendum

Scope:
G5 shadow now carries internal `task` and `parallel_tasks` tool-contract
boundary evidence. The new `jobReplay.toolContractBoundary` fields include
task and parallel field names, `internalRuntimeOnly:true`, permission gate,
dependency validation, planner read-only toolset requirement,
`usesReasonixProtocol:false`, and `topLevelRouteExposed:false`. This batch
changes G5 fixtures, conformance tests, Go shadow output, and docs only; it adds
no planner product toggle, live Go Job Manager, public sub-agent/job protocol,
default Go backend, renderer-visible Go route, top-level Subagent/Workflow/
Create Loop/AutoResearch/MCP-indexer navigation, Kun identity, Rust/Tauri path,
or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task tool-contract boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays internal task tool-contract boundaries from the
TS-owned task-job oracle.
```

Rejected claims:

```text
No planner product toggle.
No live Go Job Manager readiness.
No Reasonix public task/sub-agent/job protocol parity.
No top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation.
No packaged task/sub-agent QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1500 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0096|G5 Task Tool Contract Boundary|jobReplay\\.toolContractBoundary|toolContractBoundary|ParallelTaskToolContract|internalRuntimeOnly|requiresPermissionGate|requiresDependencyValidation|requiresPlannerReadOnlyToolset" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Approval/User-Input Route Body Replay Addendum

Scope:
G5 shadow now carries approval/user-input route body and structured prompt
shape evidence. The new `approvalUserInputReplay.approvalRoute`,
`userInputSubmitRoute`, and `userInputCancelRoute` fields include deny/cancel/
submit request bodies, HTTP response bodies, prompt question option labels,
pending counts, late resolve status, and the HTTP-answer-echo/SSE-answer-
redaction boundary. This batch changes G5 fixtures, conformance tests, Go
shadow output, and docs only; it adds no live Go approval/user-input manager,
default Go backend, renderer-visible Go route, Reasonix public ask/session
protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 approval/user-input route body conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays approval/user-input route bodies and structured
prompt shape from the TS-owned approval/user-input oracle.
```

Rejected claims:

```text
No live Go approval/user-input manager readiness.
No Reasonix public ask/session protocol parity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation.
No packaged approval-card QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1502 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0097|Approval/User-Input Route Body|approvalUserInputReplay\\.(approvalRoute|userInputSubmitRoute|userInputCancelRoute)|userInputSubmitRoute|userInputCancelRoute|approvalRoute|prompt option|SSE answer redaction" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 MCP Lifecycle Detail Replay Addendum

Scope:
G5 shadow now carries MCP reconnect, known-override, and search
workspace-boundary detail evidence. The new
`mcpReplay.backgroundReconnect`, `knownOverrideDiagnostics`, and
`searchWorkspaceBoundary` fields include failed/suspended/connected/error
server ids, retry attempts, no-restart requirement, codegraph/codebase-memory
override diagnostics, trusted/untrusted workspace boundaries, query,
unknown-tool error, on-request policy, and denied no-execute behavior. This
batch changes G5 fixtures, conformance tests, Go shadow output, and docs only;
it adds no live Go MCP client/indexer, default Go backend, renderer-visible Go
route, Reasonix MCP-indexer public lifecycle protocol, top-level MCP-indexer
navigation, Kun identity, Rust/Tauri path, or deprecated bridge/settings
fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP lifecycle detail conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 9 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays MCP reconnect, known-override, and search
workspace-boundary details from the TS-owned MCP lifecycle oracle.
```

Rejected claims:

```text
No live Go MCP client/indexer readiness.
No Reasonix MCP-indexer lifecycle protocol parity.
No top-level MCP-indexer navigation.
No credentialed MCP matrix or packaged MCP QA.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1502 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0098|MCP Lifecycle Detail|mcpReplay\\.(backgroundReconnect|knownOverrideDiagnostics|searchWorkspaceBoundary)|backgroundReconnect|knownOverrideDiagnostics|searchWorkspaceBoundary|retryAllFailedServers|Reasonix MCP-indexer lifecycle" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Provider Usage Parser Precedence Replay Addendum

Scope:
G5 shadow now carries provider usage parser precedence evidence. The new
`cacheReplay.usageParserReplay` fields include unsupported-provider absent
cache telemetry, DeepSeek native `prompt_cache_*` precedence over
`prompt_tokens_details.cached_tokens`, OpenAI responses cached-token hit/miss
calculation, Anthropic read/creation cache accounting, and reasoning-token
preservation. This batch changes provider/cache fixtures, conformance tests,
Go shadow output, and docs only; it adds no live provider protocol change, live
Go provider client, default Go backend, renderer-visible Go route, Reasonix
provider public protocol, Kun identity, Rust/Tauri path, or deprecated bridge/
settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 provider usage parser conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 18 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays provider usage parser precedence from the TS-owned
provider-cache oracle.
```

Rejected claims:

```text
No live provider/cache superiority claim.
No credentialed provider matrix or packaged provider settings QA.
No Reasonix provider protocol parity.
No live Go provider client, default Go backend, or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1502 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0099|Provider Usage Parser Precedence|cacheReplay\\.usageParserReplay|usageParserReplay|deepseekNativePrecedence|openaiResponsesCachedTokens|anthropicCacheFields|unsupportedAbsentFields" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_g3g4.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## G5 Auto-Router Classifier Currentness Replay Addendum

Scope:
G5 shadow now carries auto-router classifier currentness evidence. The new
`controlExecutableCases.autoRouterClassifier` fields include classifier model,
timeout, fingerprint, isolated short-JSON request shape, fingerprint drift
invalidation, timeout fallback, and product-boundary flags. TypeScript
conformance binds the fixture to live `AUTO_MODEL_ROUTER_*` constants and
`buildAutoModelRouterFingerprint()`; Go shadow replays the TS-owned fixture.
This batch changes G5 fixtures, conformance tests, Go shadow output, and docs
only; it adds no public auto-plan setting, project/local override, Reasonix
controller protocol, live Go auto-router, default Go backend, renderer-visible
Go route, Kun identity, Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 auto-router classifier conformance | `npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 12 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 shadow replays auto-router classifier currentness and timeout
fallback from TS-owned runtime constants.
```

Rejected claims:

```text
No Reasonix auto-plan setting parity.
No project/local auto-plan override or Reasonix controller/SessionAPI protocol.
No live Go auto-router, default Go backend, or renderer-visible Go route.
No packaged planner QA.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 773 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1502 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0100|Auto-Router Classifier Currentness|autoRouterClassifier|AUTO_MODEL_ROUTER_FINGERPRINT|classifier currentness|timeout fallback|public auto-plan setting" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_test.go` | pass: expected synchronized refs, fixture, schema, tests, Go shadow, scorecard, and evidence records only |

## Reasonix Auto-Plan Config Boundary Addendum

Scope:
Reasonix `01d9b173` is absorbed as a negative settings/config boundary:
`agent.auto_plan`, root `autoPlan` / `auto_plan`, runtime `autoPlan` /
`auto_plan`, and `analytix serve` auto-plan config roots are stripped or
rejected. This batch changes settings normalization, settings persistence
tests, runtime config parser tests, and docs only. It adds no product
auto-plan toggle, Reasonix config CLI/controller protocol, project/local
override, renderer-visible Go route, default Go backend, Kun identity,
Rust/Tauri path, or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| GUI settings and persistence | `npm run test -- src/shared/app-settings.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 91 tests |
| Runtime config boundary | `npm --prefix packages/runtime test -- src/config/analytix-config.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |

Accepted claims:

```text
Analytix rejects Reasonix auto-plan public config shapes and keeps future
auto-plan controls behind analytix-owned runtime settings.
```

Rejected claims:

```text
No Reasonix auto-plan setting parity.
No project/local auto-plan override or Reasonix controller/SessionAPI protocol.
No product planner/auto-plan toggle.
No live Go auto-router, default Go backend, or renderer-visible Go route.
No packaged settings QA.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 774 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1505 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0101|Auto-Plan Config Boundary|auto-plan config|agent\\.auto_plan|autoPlan|auto_plan|Reasonix Auto-Plan Config Boundary|AnalytixConfigSchema auto-plan" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa src/shared/app-settings-runtime.ts src/shared/app-settings-normalize.ts src/shared/app-settings.test.ts src/main/settings-store.test.ts packages/runtime/src/config/analytix-config.test.ts` | pass: expected synchronized sanitizer, tests, docs, scorecard, and evidence records only |

## G5 Task Parent Goal Evidence Executable Replay Addendum

Scope:
G5 control executable output now carries `taskJobs.parentGoalEvidence`. The
fixture records active-goal and missing-goal event metadata; Go shadow derives
`ledgeredWhenActiveGoal` and `errorsWithoutActiveGoal` from the TS-owned
oracle. This batch changes G5 fixtures, conformance tests, Go shadow output,
and docs only. It adds no public sub-agent/job protocol, top-level Subagent,
Workflow/Create Loop/AutoResearch/MCP-indexer entry, live Go Job Manager,
default Go backend, renderer-visible Go route, Kun identity, Rust/Tauri path,
or deprecated bridge/settings fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task parent-goal conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 executable shadow replays task parent-goal evidence metadata from
TS-owned fixtures.
```

Rejected claims:

```text
No Reasonix public sub-agent/job protocol parity.
No live Go Job Manager, default Go backend, or renderer-visible Go route.
No top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer entry.
No packaged desktop sub-agent/task-job QA.
No Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration, or
release readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 774 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1505 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Top-level forbidden entry scan | `if rg -n "Workflow|Create Loop|Subagent|AutoResearch|MCP-indexer|workflowCreateLoop" src/renderer/src/App.tsx src/renderer/src/main.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/components/SessionHeader.tsx src/renderer/src/components/chat src/renderer/src/store --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production top-level entry hits |
| Kun/Reasonix identity scan | `if rg -n "window\\.kun|kunGui|KunAgent|Kun Desktop|kun serve|KUN_|Reasonix public protocol|reasonix public protocol|Reasonix SessionAPI|reasonix protocol" src packages --glob '!**/*.test.ts' --glob '!packages/runtime/src/conformance/fixtures/*.json'; then exit 1; fi` | pass: no production identity/protocol hits |
| Deprecated bridge/settings fallback scan | `if rg -n "window\\.kun|window\\.deepseek|kunGui|agents\\.kun|KUN_|runtime-shaped|deprecated bridge|window\\.analytixGui|window\\.deepseek" src/shared src/main src/preload src/renderer/src packages/runtime/src --glob '!**/*.test.ts'; then exit 1; fi` | pass: no deprecated bridge/settings fallback hits |
| Default Go/Rust/Tauri scan | `if rg -n "defaultGoBackend\\s*[:=]\\s*true|defaultGoBackendEnabled\\s*[:=]\\s*true|defaultGoBackendAllowed\\s*[:=]\\s*true|ANALYTIX_GO|GO_BACKEND|runtimeBackend|src-tauri|tauri\\.conf|Cargo\\.toml|Cargo\\.lock" src packages electron-builder.config.cjs package.json scripts .github build --glob '!**/*.test.ts'; then exit 1; fi` | pass: no production enabling hits |
| Cargo/Tauri/Rust file scan | `if rg --files | rg '(^|/)(Cargo\\.toml|Cargo\\.lock|tauri\\.conf\\.(json|json5)|.*\\.rs$|src-tauri/)'; then exit 1; fi` | pass: no files |
| Connect Phone locale scan | `if rg -n "\\bClaw\\b" src/renderer/src/locales --glob '!**/*.test.ts'; then exit 1; fi` | pass: no locale copy hits |
| Docs/spec/ledger consistency | `rg -n "D-0102|Task Parent Goal Evidence Executable|parentGoalEvidence|ledgeredWhenActiveGoal|errorsWithoutActiveGoal|G5 Task Parent Goal Evidence" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go/shadow_g5.go packages/runtime-go/shadow_test.go` | pass: expected synchronized fixture, schema, tests, Go shadow, docs, scorecard, and evidence records only |

## Product Sovereignty Scan Engineering Addendum

Scope:
The repeated release forbidden-surface scans are now available through
`npm run scan:product-sovereignty`, and the route-surface oracle includes
`PluginMarketplaceView.tsx`. This batch changes the reusable scan script,
root package script, renderer route-surface test, and docs only. It adds no
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, no
Reasonix public protocol, no Kun identity, no deprecated bridge/settings
fallback, no default Go backend, no renderer-visible Go route, and no
Rust/Tauri path.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Context Compaction Boundary Addendum

Scope:
Latest-compaction effective-history boundary behavior now has a G5 executable
shadow proof. The fixture covers latest positive compaction selection,
noop/older/pre-boundary drops, post-boundary preservation, stable-prefix
isolation, and no Reasonix protocol/top-level route exposure.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 compaction boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.470s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay latest-compaction effective-history
boundary behavior from TS-owned fixtures without exposing Go or Reasonix
thread/session routes.
```

Rejected claims:

```text
No live Go history manager, live Go loop, Reasonix SessionAPI/controller
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop long-history QA, G6 readiness, or release readiness.
```

Final command gate for D-0149:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.215s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Route-surface oracle | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 4 tests |

Accepted claims:

```text
Analytix has a reusable product-sovereignty scan gate for upstream absorption
batches.
```

Rejected claims:

```text
No release readiness claim from this scan alone.
No packaged desktop QA, signing/notarization, or installer proof.
No Reasonix public protocol parity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Kun identity or deprecated bridge/settings fallback.
No default Go backend, renderer-visible Go route, Rust/Tauri migration, or
live Go runtime readiness.
```

Final command gate for this addendum:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 774 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1505 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: reusable forbidden-surface scan passed |
| Docs/spec/ledger consistency | `rg -n "D-0103|Product Sovereignty Scan|scan:product-sovereignty|PluginMarketplaceView|forbidden-surface|product-sovereignty" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa package.json scripts/scan-product-sovereignty.cjs src/renderer/src/components/Workbench.route-surface.test.ts` | pass: expected synchronized scan script, package script, route-surface test, specs, ledgers, scorecard, and evidence records only |

## Auto-Router Rebuild Lifecycle Ledger Reconciliation Addendum

Scope:
The Auto-Router Failure Usage Isolation records are reconciled with the later
Managed Runtime Provider Currentness evidence. This is a document-only
correction: auto-router failure fallback and usage/cache isolation remain
covered by runtime loop tests, while provider/settings rebuild currentness is
covered by settings-key and managed child env snapshot tests. It does not add
a Reasonix auto-plan setting, controller rebuild protocol, public SessionAPI,
default Go backend, renderer-visible Go route, or product entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router failure isolation | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "falls back to a concrete heuristic model without recording router usage" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 1 passed, 84 skipped |
| Managed runtime currentness | `npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 58 tests |

Accepted claims:

```text
Analytix auto-router failure isolation and managed runtime provider/settings
currentness are both covered by existing analytix-owned tests.
```

Rejected claims:

```text
No Reasonix controller rebuild parity.
No Reasonix auto-plan config or user-visible auto-plan setting.
No project/local auto-plan override.
No live provider/cache superiority.
No default Go backend, renderer-visible Go route, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, release readiness, Kun identity,
or Rust/Tauri migration.
```

## Go G5 Parallel Task Dependency Executable Shadow Addendum

Scope:
G5 `parallel_tasks` dependency validation is now executable shadow evidence:
`controlExecutableCases.taskJobs.parallelValidation` declares the fixture-owned
valid/invalid plans, and Go shadow computes valid order plus exact invalid
dependency errors. This changes only conformance fixtures/tests and Go shadow
code; it adds no renderer-visible route, no default Go backend, no Reasonix
public sub-agent/job protocol, and no top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime/Go conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |

Accepted claims:

```text
Analytix G5 executable shadow covers `parallel_tasks` dependency validation
from TS-owned fixtures.
```

Rejected claims:

```text
No Reasonix public sub-agent/job protocol parity.
No live Go Job Manager, default Go backend, renderer-visible Go route, G6
readiness, or release readiness.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No packaged desktop sub-agent/task-job QA.
No Kun identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

## Write Sidebar and Connect Phone Placeholder Sovereignty Addendum

Scope:
The reusable product-sovereignty scan and route-surface oracle now include
Write/sidebar entry surfaces, and newly generated Connect Phone mapped
conversation placeholders use `[Connect Phone:...]` instead of `[Claw:...]`.
Legacy `[Claw:]` recognizers remain compatibility-only.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer sovereignty tests | `npm run test -- src/renderer/src/store/chat-store-claw-actions.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 12 tests |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
Analytix product-sovereignty scans cover Write/sidebar entry surfaces, and new
Connect Phone placeholders no longer emit `[Claw:...]`.
```

Rejected claims:

```text
No packaged desktop Connect Phone QA or release readiness.
No removal of internal compatibility names or legacy Claw recognizers.
No Reasonix public protocol, Kun identity, deprecated bridge/settings fallback,
default Go backend, renderer-visible Go route, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, or Rust/Tauri migration.
```

Final command gate for the D-0104 / D-0105 continuation:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 774 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1506 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Docs/spec/ledger consistency | `rg -n "D-0104|D-0105|parallelValidation|Write Sidebar and Connect Phone Placeholder|scan:product-sovereignty|Connect Phone placeholders" 重构升级方案.md docs/analytix/upstreams docs/analytix/specs docs/analytix/benchmarks docs/analytix/qa packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json packages/runtime/src/conformance/runtime-parity-fixtures.ts packages/runtime/tests/go-runtime-conformance.test.ts packages/runtime-go scripts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-claw-actions.ts src/renderer/src/store/chat-store-claw-actions.test.ts` | pass: expected synchronized fixture, schema, tests, Go shadow, scan, route-surface, specs, ledgers, scorecard, and evidence records only |

## HTTP Auto-Plan Payload Boundary Addendum

Scope:
Reasonix-shaped `autoPlan` / `auto_plan` start-turn payload fields are now
covered by a negative HTTP/loop fixture. The request remains an agent turn,
does not persist `guiPlan`, and the captured model request does not advertise
`create_plan`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| HTTP auto-plan boundary | `npm --prefix packages/runtime test -- tests/http-server.test.ts -t "auto-plan payload" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 1 passed, 48 skipped |

Accepted claims:

```text
Analytix ignores Reasonix-shaped `autoPlan` / `auto_plan` start-turn fields;
only explicit `mode: "plan"` / `guiPlan` can advertise `create_plan`.
```

Rejected claims:

```text
No Reasonix auto-plan config parity, user/project auto-plan setting,
controller rebuild protocol, SessionAPI, packaged Plan mode QA, release
readiness, default Go backend, renderer-visible Go route, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

## Plan Step Cancel/Cache Boundary Addendum

Scope:
Explicit analytix Plan mode now has a deterministic AgentLoop fixture covering
step narrowing, interrupt handling, and cache diagnostics in one flow.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Plan step cancel/cache boundary | `npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 1 passed, 85 skipped |

Accepted claims:

```text
Analytix Plan mode step 0 advertises read-only tools plus `create_plan`;
the unsatisfied follow-up step advertises only `create_plan`; interrupting that
follow-up aborts the turn and does not advance the cache prefix baseline.
```

Rejected claims:

```text
No Reasonix auto-plan config parity, SessionAPI/controller protocol, public
planner route, packaged Plan mode QA, live provider superiority, release
readiness, default Go backend, renderer-visible Go route, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0107:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1506 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 User-Input Structured Validation Addendum

Scope:
Structured `request_user_input` validation now has a G5 executable shadow
proof. The fixture covers max questions, option bounds, case-insensitive
duplicate-label rejection, invalid result code, invalid case count, no-gate on
invalid requests, and no Reasonix protocol/top-level route exposure.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G4/G5 user-input validation conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.386s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay structured request_user_input
validation from TS-owned fixtures without exposing Reasonix ask/session
protocol.
```

Rejected claims:

```text
No live Go approval/user-input manager, Reasonix ask/session protocol,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri migration,
packaged desktop approval/user-input QA, G6 readiness, or release readiness.
```

Final command gate for D-0150:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.181s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Planner Gate Matrix Addendum

Scope:
Plan mode tool-policy gating now has a G5 executable shadow matrix. The fixture
covers normal agent mode hiding `create_plan`, Plan capability advertisement,
step 0 read-only + `create_plan`, step >0 only `create_plan`, and forged
`task`, `parallel_tasks`, `bash`, `edit`, `write`, and `echo` rejection without
execution.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Planner gate conformance + create_plan tests | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/create-plan-tool.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 33 tests |
| Loop Plan mode forged-tool tests | `npm --prefix packages/runtime test -- tests/loop.test.ts --no-file-parallelism --maxWorkers=1 -t "Plan mode\|create_plan\|forged"` | pass: 1 file / 9 tests, 77 skipped by filter |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.197s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay planner gate matrix from TS-owned
fixtures without exposing Reasonix planner/session protocol or Go planner
routes.
```

Rejected claims:

```text
No live Go planner/executor, Reasonix planner/session/task protocol, public
auto-plan setting, renderer-visible Go route, default Go backend, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity,
Rust/Tauri migration, packaged desktop Plan QA, G6 readiness, or release
readiness.
```

Final command gate for D-0151:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.404s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Browser Preview SSE Executable Bridge Addendum

Scope:
Browser preview SSE bridge behavior now has executable unit coverage. The test
starts `window.analytix.runtime.startSse`, verifies the analytix
`/v1/threads/:id/events` proxy path and `Last-Event-ID`, and checks SSE
`id`/`event`/`data` normalization into renderer payload fields.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Browser preview SSE bridge | `npm run test -- src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |

Accepted claims:

```text
Analytix browser preview SSE bridge has executable evidence for analytix proxy
path, cursor header, and event normalization.
```

Rejected claims:

```text
No Reasonix SessionAPI/public protocol, live Go bridge, default Go backend,
renderer-visible Go route, packaged browser/desktop QA, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0156:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1511 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.462s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Browser Preview Settings Sovereignty Addendum

Scope:
Browser preview settings now have shared and bridge-level evidence that
legacy/Reasonix app and runtime envelopes are stripped on load and re-save.
Valid top-level `runtime.model` and `runtime.endpointFormat` remain under the
analytix-owned settings schema.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Shared/browser preview settings sovereignty | `npm run test -- src/shared/app-settings.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 75 tests |

Accepted claims:

```text
Analytix browser preview settings drop legacy/Reasonix agent envelopes on load
and re-save while preserving valid top-level runtime settings.
```

Rejected claims:

```text
No Reasonix config/auto-plan parity, deprecated settings fallback, default Go
backend, renderer-visible Go route, packaged settings QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0158:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1512 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.425s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0157:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1512 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.436s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## IPC Settings Patch Sovereignty Addendum

Scope:
Renderer-to-main settings patch validation now strips top-level legacy/Reasonix
envelopes before strict schema validation. Valid analytix patch fields survive,
while runtime-nested legacy agent-shaped fields remain rejected.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| IPC settings patch sovereignty | `npm run test -- src/main/ipc/app-ipc-schemas.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 38 tests |

Accepted claims:

```text
Analytix IPC settings patches drop top-level legacy/Reasonix envelopes before
strict validation while preserving valid analytix settings patch fields.
```

Rejected claims:

```text
No Reasonix config/auto-plan parity, deprecated settings fallback, default Go
backend, renderer-visible Go route, packaged settings QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

## Settings Sovereignty Scan Freshness Addendum

Scope:
Product-sovereignty scanning now requires the settings/bridge sovereignty
chokepoints and their focused guard tests to remain present: shared settings
normalization/runtime settings, settings-store persistence, IPC settings patch
schema, preload, and browser preview bridge.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Settings sovereignty scan freshness | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Analytix product-sovereignty scans include settings/bridge sovereignty
chokepoints and guard tests as required paths.
```

Rejected claims:

```text
No Reasonix config/auto-plan parity, deprecated settings fallback, default Go
backend, renderer-visible Go route, packaged settings QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0159:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1512 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.433s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Workflow Singular Route Negative Addendum

Scope:
The live TypeScript HTTP router negative-route test now covers singular
`/v1/workflow` alongside plural `/v1/workflows`, aligning runtime route proof
with task-job and Go G4/G5 forbidden route fixtures.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime forbidden route proof | `npm --prefix packages/runtime test -- tests/http-server.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 50 tests |

Accepted claims:

```text
Analytix live HTTP route tests prove singular `/v1/workflow` remains absent
alongside plural `/v1/workflows`.
```

Rejected claims:

```text
No Workflow product-entry readiness, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0160:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1512 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.428s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Runtime Proof Freshness Addendum

Scope:
Product-sovereignty scanning now requires runtime conformance proof files to
remain present, including provider-cache, approval/user-input, G2/G3/G4/G5
fixtures/tests, and Go shadow sources. It also scans active runtime routes,
shared endpoint templates, IPC schema, and renderer runtime client source for
forbidden public route/protocol strings.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime proof freshness scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Analytix product-sovereignty scans include runtime conformance proof freshness
and active route/client forbidden-surface checks.
```

Rejected claims:

```text
No new provider/cache behavior, credentialed provider parity, live Go routes,
renderer-visible Go route, default Go backend, packaged route/provider QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0161:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1512 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.434s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Renderer Runtime Request Surface Addendum

Scope:
Renderer runtime provider behavior now has executable request-path evidence.
The test captures common `AnalytixRuntimeProvider` runtime requests and proves
they stay on `/health` or analytix-owned `/v1/*` paths while rejecting
Reasonix public routes, renderer-visible Go routes, and hidden-capability
public routes.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer runtime request surface | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 25 tests |

Accepted claims:

```text
Analytix renderer runtime provider requests stay on analytix-owned HTTP routes
and avoid forbidden upstream public surfaces.
```

Rejected claims:

```text
No packaged desktop QA, Reasonix public protocol parity, live Go routes,
renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0162:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1513 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.410s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Renderer SSE Bridge Cursor Addendum

Scope:
Renderer SSE subscription behavior now has executable bridge/cursor evidence.
The test proves `AnalytixRuntimeProvider.subscribeThreadEvents` starts streams
through `window.analytix.runtime.startSse(threadId, sinceSeq, streamId)`,
dispatches events for that generated stream id, avoids `runtimeRequest` as an
SSE side channel, and calls `stopSse(streamId)` for cleanup.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer SSE bridge cursor | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 25 tests |

Accepted claims:

```text
Analytix renderer SSE subscription stays on the window.analytix bridge/cursor
contract and cleans up the same generated stream id.
```

Rejected claims:

```text
No packaged desktop SSE QA, Reasonix public protocol parity, live Go routes,
renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0163:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1513 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.416s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Main/Preload SSE IPC Bridge Addendum

Scope:
Desktop SSE IPC behavior now has preload/main proof. The tests prove
`window.analytix.runtime.startSse/stopSse` map only to `runtime:sse:*` IPC,
`sseStartPayloadSchema` preserves trimmed `threadId`/`sinceSeq`/`streamId`
and rejects Reasonix-style extra fields, main fetches
`/v1/threads/:id/events?since_seq=...` with runtime auth and initial
`Last-Event-ID`, generated stream ids appear in error payloads, and
`stopSse` aborts only the matching stream id.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Main/preload SSE IPC bridge | `npm run test -- src/main/runtime-sse-ipc.test.ts src/main/ipc/app-ipc-schemas.test.ts src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 49 tests |

Accepted claims:

```text
Analytix desktop SSE IPC preserves the analytix-owned thread cursor and stream
id contract from preload to main.
```

Rejected claims:

```text
No packaged desktop SSE QA, Reasonix public protocol parity, live Go routes,
renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0164:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1517 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.421s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Main Runtime Request Forbidden Route Addendum

Scope:
Main IPC runtime request behavior now has explicit forbidden-route evidence.
The tests prove Reasonix public routes, renderer-visible Go routes, and
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route families are
rejected by `runtimeRequestPayloadSchema`, and invalid `runtime:request`
payloads do not call the runtime request adapter.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Main runtime request forbidden routes | `npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/main/ipc/register-app-ipc-handlers.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 54 tests; Node `punycode` deprecation warning only |

Accepted claims:

```text
Analytix main IPC rejects upstream public runtime routes before they reach the
runtime request adapter.
```

Rejected claims:

```text
No packaged desktop route QA, Reasonix public protocol parity, live Go routes,
renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0165:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1519 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.419s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Preload Runtime Request IPC Bridge Addendum

Scope:
Preload runtime request behavior now has source guard evidence. The test proves
`window.analytix.runtime.runtimeRequest(path, method, body)` maps only to
`ipcRenderer.invoke('runtime:request', { path, method, body })` and does not
introduce Reasonix/Kun/Go/Workflow runtime request IPC channels.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Preload runtime request IPC bridge | `npm run test -- src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |

Accepted claims:

```text
Analytix preload runtime requests use only the analytix-owned runtime:request
IPC channel.
```

Rejected claims:

```text
No packaged desktop route QA, Reasonix public protocol parity, live Go routes,
renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0166:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.404s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Runtime Desktop Bridge Proof Freshness Addendum

Scope:
The reusable product-sovereignty scan now includes
`runtimeDesktopBridgeProofPaths`. It requires renderer provider/client,
browser preview bridge, preload bridge/types, main IPC schema/handler, main
SSE IPC, shared API/endpoints, and the focused runtime request/SSE bridge guard
tests to remain present.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime desktop bridge proof freshness | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Analytix product-sovereignty scan guards the desktop runtime bridge proof
chain for path freshness.
```

Rejected claims:

```text
No packaged desktop route/SSE QA, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0167:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 777 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.433s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Desktop Bridge/Settings Sovereignty Control Shadow Addendum

Scope:
Desktop bridge/settings sovereignty now has a G5 executable control shadow. The
fixture is derived from live analytix-owned desktop sources and proves a single
`window.analytix` bridge, `Window.analytix` typing, analytix-owned facade
domains, Reasonix `autoPlan`/`auto_plan` settings drops, legacy `agentProvider`
/`agents` envelope stripping, top-level `runtime.endpointFormat` persistence,
and no deprecated bridge/settings fallback or Reasonix protocol exposure.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Desktop sovereignty conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Preload/settings sovereignty tests | `npm run test -- src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 27 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.200s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay desktop bridge/settings sovereignty
from live source-derived fixtures without exposing Reasonix public protocol,
deprecated bridge aliases, legacy settings envelopes, or renderer-visible Go
routes.
```

Rejected claims:

```text
No Reasonix desktop protocol, public auto-plan setting, deprecated
window.kun/window.reasonix alias, legacy agent settings write path,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri migration,
packaged desktop settings QA, G6 readiness, or release readiness.
```

Final command gate for D-0152:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.200s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Auto-Router Recommendation/Currentness Control Shadow Addendum

Scope:
Auto-router classifier currentness evidence now covers recommendation parsing
and recent-context boundaries in G5 executable control shadow. The fixture is
derived from `parseAutoRouteRecommendation` and `recentAutoRouterContext`, then
Go shadow computes accepted/rejected recommendation counts, `pro/max`
acceptance, `model:"auto"` and malformed rejection, active-turn exclusion, and
tool-result summary preservation.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Auto-router + G5 conformance | `npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 12 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.426s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay auto-router recommendation parsing
and recent-context currentness from TS-owned helpers without exposing Reasonix
controller/session protocol or public auto-plan settings.
```

Rejected claims:

```text
No Reasonix controller/session protocol, public auto-plan setting, project
auto-plan override, renderer-visible Go route, default Go backend, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity,
deprecated bridge/settings fallback, Rust/Tauri migration, packaged desktop
router QA, G6 readiness, or release readiness.
```

Final command gate for D-0153:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.426s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Exact Session Route Replay Control Shadow Addendum

Scope:
Thread/session route evidence now includes an exact G5 executable control
shadow replay matrix derived from the G2 route oracle. The matrix records
method, path, setup, auth, response kind, status, request-body hash,
response-body shape/hash, SSE frame count, SSE event names, and SSE frame hash
for list/archive/search/read/update, fork, resume, replay SSE, caught-up SSE,
and unauthorized SSE access.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 session route conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.210s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay exact thread/session route
contracts for fork/resume/archive/search/SSE from TS-owned G2 fixtures.
```

Rejected claims:

```text
No Reasonix SessionAPI/public route protocol, live Go HTTP server,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/
settings fallback, Rust/Tauri migration, packaged desktop route QA, G6
readiness, or release readiness.
```

Final command gate for D-0154:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.210s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Nested Child SSE Metadata Control Shadow Addendum

Scope:
G5 nested child SSE metadata evidence now runs through typed control shadow,
not only summary replay. The fixture uses the TS-owned task-job oracle in
`controlExecutableCases.taskJobs.nestedSseMetadata`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 nested child SSE metadata conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | initially failed on duplicate helper, then pass after reusing existing `stringSliceContains`: `ok github.com/analytix/runtime-go 0.386s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay nested child SSE metadata from
TS-owned fixtures: parent call id, child run id, nested SSE metadata fields,
evidence ledger metadata key coverage, parent/child distinctness, active-goal
requirement, no Reasonix protocol, and no top-level route exposure.
```

Rejected claims:

```text
No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, packaged
nested-card QA, G6 readiness, release readiness, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0135:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Task Transcript Identity Control Shadow Addendum

Scope:
G5 task transcript continue/fork identity evidence now runs through typed
control shadow, not only summary replay. The fixture uses the TS-owned task-job
oracle in `controlExecutableCases.taskJobs.transcriptIdentity`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task transcript identity conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.418s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay task transcript identity from
TS-owned fixtures: source id, continue target id, fork target id, incompatible
identity error, same-transcript identity requirement, continue-target-matches-
source, fork-target-distinct-from-source, no Reasonix protocol, and no
top-level route exposure.
```

Rejected claims:

```text
No live Go Job Manager, public Reasonix SessionAPI/sub-agent/job protocol,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, packaged
nested-card QA, G6 readiness, release readiness, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0134:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Task-Job Tool Contract Boundary Control Shadow Addendum

Scope:
G5 task-job tool contract and route-boundary evidence now runs through typed
control shadow, not only summary replay. The fixture uses the TS-owned task-job
oracle in `controlExecutableCases.taskJobs.toolContractBoundary`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task-job tool boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.439s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay task-job tool contract boundary
from TS-owned fixtures: internal-only `task`/`parallel_tasks` tool contracts,
permission/evidence/dependency/read-only gates, runtime task-job routes,
protected route auth, unauthorized status, forbidden top-level routes, no
Reasonix protocol, and no top-level route exposure.
```

Rejected claims:

```text
No live Go Job Manager, public Reasonix sub-agent/job protocol, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/navigation,
renderer-visible Go route, default Go backend, packaged nested-card QA, G6
readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0133:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Product Boundary Control Shadow Addendum

Scope:
G5 product-boundary evidence now runs through typed control shadow, not only
manifest metadata. The fixture uses the TS-owned G5 product boundary in
`controlExecutableCases.productBoundary`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 product boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.374s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay product-boundary invariants from
TS-owned fixtures: bridge/serve stability, no Reasonix public protocol, no
default Go backend, no renderer-visible Go route, no Electron main connection,
and no enabled Go backend.
```

Rejected claims:

```text
No live Go backend, Reasonix public protocol, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route/navigation, renderer-visible Go route,
default Go backend, Electron integration, packaged desktop QA, G6 readiness,
release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration.
```

Final command gate for D-0132:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Background Reconnect Control Shadow Addendum

Scope:
G5 MCP background reconnect evidence now runs through typed control shadow, not
only summary replay. The fixture uses the TS-owned reconnect oracle in
`controlExecutableCases.mcpBackgroundReconnect`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP background reconnect conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.377s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP background reconnect behavior
from TS-owned fixtures: failed server ids, suspended provider/reason,
connected/error outcomes, retry attempts, retry-all-failed coverage, and
no-runtime-restart state.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0131:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Known Override Diagnostics Control Shadow Addendum

Scope:
G5 MCP known override diagnostics now run through typed control shadow, not
only summary replay. The fixture uses the TS-owned known-override variants in
`controlExecutableCases.mcpKnownOverrideDiagnostics`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP known override diagnostics conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.381s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP known override diagnostics
from TS-owned fixtures: override kinds, effective cwd, workspace roots,
explicit-cwd server ids, daemon-timeout server ids, low-priority/background-
start booleans, and product-boundary flags.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0136:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Live-Local Indexer Control Shadow Addendum

Scope:
G5 MCP live-local indexer lifecycle now runs through typed control shadow, not
only summary replay. The fixture uses the TS-owned live-local indexer source in
`controlExecutableCases.mcpLiveLocalIndexer`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP live-local indexer conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.397s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP live-local indexer lifecycle
from TS-owned fixtures: retry ids/map, initial/resume/active paths, tombstone
count, snapshot restart, late tombstone, secret-safe diagnostic,
execution-error redaction, and product-boundary flags.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0137:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Search Meta-Tool Control Shadow Addendum

Scope:
G5 MCP search meta-tool advertised/trust/no-execute evidence now runs through
typed control shadow, not only summary replay. The fixture uses the TS-owned
search meta-tools source in `controlExecutableCases.mcpSearchMetaTools`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP search meta-tool conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.445s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP search meta-tool
advertised/trust/no-execute behavior from TS-owned fixtures: meta-tool
names/count, refresh tool advertisement, trusted/untrusted workspace fields,
unknown-tool error, `on-request` policy, denied no-execute, and
product-boundary flags.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0138:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Privacy Control Shadow Addendum

Scope:
Provider cache diagnostics privacy and live-superiority policy now run through
typed control shadow, not only summary replay. The fixture uses TS-owned
provider/cache privacy, diagnostics, and live credential policy in
`controlExecutableCases.providerCachePrivacy`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 provider cache privacy conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.428s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider cache diagnostics privacy
and no-live-superiority policy from TS-owned fixtures: bounded diagnostics,
forbidden substring no-leak, no live credentials, no live superiority claim,
and no Reasonix protocol/top-level route exposure.
```

Rejected claims:

```text
No live Go provider client, Reasonix provider protocol, credentialed provider
matrix, live provider/cache superiority claim, renderer-visible Go route,
default Go backend, packaged provider QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0139:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Inventory Control Shadow Addendum

Scope:
Provider cache stable-prefix/tool/case inventory now runs through typed control
shadow, not only loose summary replay. The fixture uses TS-owned provider/cache
stable prefix, provider usage ids, and request-shape ids in
`controlExecutableCases.providerCacheInventory`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 provider cache inventory conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.386s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider cache prefix/tool/case-id
inventory from TS-owned fixtures: stable prefix hash, canonical tools hash,
provider usage ids/count, request-shape ids/count, prefix equivalence, tools
hash stability, and no Reasonix protocol/top-level route exposure.
```

Rejected claims:

```text
No live Go provider client, Reasonix provider protocol, dynamic stable-prefix
material, credentialed provider matrix, live provider/cache superiority claim,
renderer-visible Go route, default Go backend, packaged provider QA, G6
readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0140:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Session Route Inventory Control Shadow Addendum

Scope:
Thread/session route inventory now runs through typed control shadow, not only
route status replay. The fixture uses TS-owned G2 route summaries in
`controlExecutableCases.sessionRouteInventory`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 session route inventory conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.429s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay thread/session route inventory
from TS-owned fixtures: route ids, JSON/SSE/event/resume/fork/archive/search/
read-update groups, runtime-token count, unauthorized route ids, and no
Reasonix protocol/top-level route exposure.
```

Rejected claims:

```text
No live Go HTTP server, Reasonix SessionAPI/public route protocol,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, packaged desktop route QA, G6
readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0141:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Approval/User-Input Inventory Control Shadow Addendum

Scope:
Approval/user-input gate inventory now runs through typed control shadow, not
only route replay. The fixture uses TS-owned approval/user-input ids, replay
kinds, answer privacy flags, late statuses, and pending counters in
`controlExecutableCases.approvalUserInputInventory`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 approval/user-input inventory conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.463s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay approval/user-input gate inventory
from TS-owned fixtures: gate ids, approval ids, user-input ids, route kinds,
replay kind order, abort replay kind order, answer count, HTTP answer echo
versus resolved-event no-answer privacy, late statuses, pending-after sum, and
no Reasonix protocol/top-level route exposure.
```

Rejected claims:

```text
No live Go approval/user-input manager, Reasonix ask/session public protocol,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, packaged desktop approval-card QA, G6
readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0142:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Task Planner Toolset Inventory Control Shadow Addendum

Scope:
Task/sub-agent planner read-only and forbidden toolset inventory now runs
through typed control shadow, not only job replay summary. The fixture uses
TS-owned planner toolsets, task tool names, and policy values in
`controlExecutableCases.taskJobs.plannerToolsetInventory`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 task planner toolset conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass after reusing existing `stringSliceContains`: `ok github.com/analytix/runtime-go 0.402s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay task planner toolset inventory
from TS-owned fixtures: read-only tools, forbidden `task`/`parallel_tasks`,
tool counts, read-only exclusion of task tools, forbidden task-tool match,
planner/executor policy, and no Reasonix protocol/top-level route exposure.
```

Rejected claims:

```text
No live Go planner/executor or Job Manager, Reasonix public sub-agent/job/
planner protocol, renderer-visible Go route, default Go backend, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, packaged desktop
sub-agent QA, G6 readiness, release readiness, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0143:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Core Lifecycle Control Shadow Addendum

Scope:
G5 MCP core lifecycle evidence now runs through typed control shadow, not only
summary replay. The fixture uses a minimal TS-owned core lifecycle oracle in
`controlExecutableCases.mcpCoreLifecycle`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP core lifecycle conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass after fixing empty slice parity: `ok github.com/analytix/runtime-go 0.402s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP core lifecycle behavior from
TS-owned fixtures: connect/disconnect diagnostics, reload schema order, cancel
no-execute, and approved error shape.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0130:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Request-Shape Exact Matrix Control Shadow Addendum

Scope:
G3/G5 provider request-shape evidence now runs through exact matrix replay, not
only summary counts. The matrix is sourced from
`provider-cache-oracle.json.requestShapeCases` and appears in G3
`requestShapeSummary`, G5 `cacheReplay.requestShapeReplay`, and G5
`controlExecutableCases.providerRequestShape.expected`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G3/G5 provider request-shape conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow executable, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.407s` focused; final `0.219s` |

Accepted claims:

```text
Analytix Go G3/G5 shadow can executable-replay provider request-shape exact
matrix from TS-owned fixtures: exact URL, required/forbidden headers,
required/forbidden body fields, reasoning-effort presence, and tool-shape
family for DeepSeek chat, OpenAI-compatible chat, Responses, Anthropic
Messages, and custom full endpoints.
```

Rejected claims:

```text
No live Go provider client, Reasonix provider protocol, renderer-visible Go
route, default Go backend, credentialed live provider matrix, live superiority
claim, packaged desktop provider QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Fixture-cache note:

```text
This batch uses `go test -count=1` for Go fixture validation. A cached Go test
can miss JSON oracle changes, so fixture-driven shadow gates should force a
non-cached run when evidence files change. The non-cached run also verifies the
D-0144 `mcpReplay.lifecycle` provider/boundary fields.
```

Final command gate for D-0145:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.219s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Combined Step/Cancel/Cache Trace Control Shadow Addendum

Scope:
G5 combined auto-route cache, step-limit, and cancel evidence now runs through
exact trace replay, not only summary booleans. The trace is stored in
`controlExecutableCases.combined.expected`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 combined step/cancel/cache conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.180s` focused; final `0.205s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay combined step/cancel/cache trace:
same-turn router calls, main/max model steps, next-turn reroute calls,
classifier/step-limit stable-prefix flags, accepted/cancel result counts,
completed/aborted counts, and exact cancel result rows.
```

Rejected claims:

```text
No live Go agent loop, Reasonix controller/session protocol, renderer-visible
Go route, default Go backend, public auto-plan product surface, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, packaged desktop
cancel/cache QA, G6 readiness, release readiness, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0146:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.205s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Step-Limit Override/Delegate Matrix Control Shadow Addendum

Scope:
G5 step-limit override and delegate inheritance evidence now runs through an
exact 9-row matrix, not only scalar fields. The matrix is stored in
`controlExecutableCases.stepLimits.expected.matrix`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 step-limit matrix conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.493s` |
| Fixture JSON parse | `node -e "JSON.parse(require('fs').readFileSync('packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json','utf8')); console.log('json ok')"` | pass: `json ok` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay step-limit override/delegate
matrix rows for default, user-global, session, turn, planner, headless,
zero-default, delegate parent-half, and delegate min-floor behavior, while
keeping dynamic step limits out of the stable prefix.
```

Rejected claims:

```text
No live Go agent loop, Reasonix controller/session protocol, renderer-visible
Go route, default Go backend, public auto-plan product surface, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, packaged desktop
step-limit QA, G6 readiness, release readiness, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0147:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Fixture JSON parse | `node -e "JSON.parse(require('fs').readFileSync('packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json','utf8')); console.log('json ok')"` | pass: `json ok` |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.493s` |

Parallel research checkpoint:

```text
Sub-agent A-F review was read-only. It confirmed that Reasonix
881b2f2f..9ada1417 should be absorbed as auto-plan/classifier contract
invariants, that Kun v0.2.13/v0.2.14 does not permit new top-level capability
navigation in analytix, that provider/cache and MCP/sub-agent work should
continue as analytix-owned oracle/runtime contracts, and that Go remains G5/G6
shadow-gated only.
```

## History Repair Pair-Integrity Control Shadow Addendum

Scope:
G5 model-history repair and tool-call/result pair integrity evidence now runs
through executable Go shadow. The case is stored in
`controlExecutableCases.historyRepair`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 history repair conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: focused `ok github.com/analytix/runtime-go 0.183s`; final `0.511s` |
| Fixture JSON parse | `node -e "JSON.parse(require('fs').readFileSync('packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json','utf8')); console.log('json ok')"` | pass: `json ok` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay model-history repair pair
integrity: complete multi-tool blocks survive, orphan results are dropped,
missing-result calls are dropped, duplicate results are dropped, bridge text is
preserved, and repair bookkeeping stays out of stable prefix state.
```

Rejected claims:

```text
No live Go agent loop, live Go history manager, Reasonix SessionAPI/controller
protocol, renderer-visible Go route, default Go backend, public auto-plan
product surface, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer route, packaged desktop long-history QA, G6 readiness, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

Final command gate for D-0148:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Fixture JSON parse | `node -e "JSON.parse(require('fs').readFileSync('packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json','utf8')); console.log('json ok')"` | pass: `json ok` |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.511s` |

## MCP Core Lifecycle Boundary Seal Control Shadow Addendum

Scope:
G5 MCP core lifecycle provider identity and product-boundary flags now run
through typed control shadow. The fixture uses TS-owned MCP lifecycle inputs in
`controlExecutableCases.mcpCoreLifecycle` and requires `providerId:
"mcp:research"`, `usesReasonixProtocol: false`, and `topLevelRouteExposed:
false` in expected output.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP core lifecycle boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP core lifecycle provider/
boundary seal from TS-owned fixtures: `mcp:research`, no Reasonix protocol,
and no top-level route exposure, alongside connect/disconnect/reload/cancel/
error lifecycle output.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer public protocol, top-level
MCP-indexer route/navigation, renderer-visible Go route, default Go backend,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

Final command gate for D-0144:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Search Workspace Boundary Control Shadow Addendum

Scope:
G5 MCP search workspace boundary evidence now runs through typed control shadow,
not only summary replay. The fixture uses a minimal TS-owned workspace-boundary
oracle in `controlExecutableCases.mcpSearchWorkspaceBoundary`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP search workspace boundary conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.398s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP search workspace trust
boundaries from TS-owned fixtures: trusted/untrusted workspaces, query, trusted
tool id, untrusted search count, unknown-tool error, call policy, and denied
no-execute state.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0129:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Approval Annotation Control Shadow Addendum

Scope:
G5 MCP approval annotation evidence now runs through typed control shadow, not
only summary replay. The fixture uses a minimal TS-owned approval annotation
oracle in `controlExecutableCases.mcpApprovalAnnotations`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP approval annotation conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.459s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP approval annotations from
TS-owned fixtures: destructive/open-world hints, normalized tool name,
approval id, deny decision, approval result kind, and denied no-execute state.
```

Rejected claims:

```text
No live Go MCP client, live Go approval manager, Reasonix MCP-indexer/approval
protocol, top-level MCP-indexer route/navigation, renderer-visible Go route,
default Go backend, credentialed MCP matrix, packaged desktop MCP/approval QA,
G6 readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0128:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 MCP Search Refresh Drift Control Shadow Addendum

Scope:
G5 MCP search refresh drift evidence now runs through typed control shadow, not
only summary replay. The fixture uses a minimal TS-owned refresh drift oracle in
`controlExecutableCases.mcpSearchRefreshDrift`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 MCP search refresh drift conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.413s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay MCP search refresh catalog drift
from TS-owned fixtures: initial tool names, expanded tool names, indexed count,
catalog drift, and no-top-level-route state.
```

Rejected claims:

```text
No live Go MCP client, Reasonix MCP-indexer protocol, top-level MCP-indexer
route/navigation, renderer-visible Go route, default Go backend, credentialed
MCP matrix, packaged desktop MCP QA, G6 readiness, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0127:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Go G5 Approval/User-Input Route Replay Control Shadow Addendum

Scope:
G5 approval/user-input route replay evidence now runs through typed control
shadow, not only summary replay. The fixture uses a minimal TS-owned
approval/user-input oracle and expected output in
`controlExecutableCases.approvalUserInputRouteReplay`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 approval/user-input route replay conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow executable | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.413s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay approval/user-input route behavior
from TS-owned fixtures: approval deny, submit/cancel, replay order, late action
rejection, abort cleanup, and no-pending-gates state.
```

Rejected claims:

```text
No live Go approval/user-input manager, Reasonix SessionAPI/ask protocol,
renderer-visible Go route, default Go backend, packaged desktop approval-card
QA, G6 readiness, release readiness, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0126:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Thread/SSE Route Auth Replay Addendum

Scope:
G2/G5 route replay now proves that thread SSE replay is protected by the
analytix runtime token. The new `events-unauthorized-since-seq` fixture omits
the bearer token and expects structured 401 JSON, while G5
`routeStatusReplay.auth` records the protected route id/path/status/body code
and zero SSE frames.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G2/G5 route auth conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.165s` |

Accepted claims:

```text
Analytix route replay proves thread SSE replay rejects missing runtime tokens,
and G5 shadow replays that auth boundary from TS-owned fixtures.
```

Rejected claims:

```text
No Reasonix SessionAPI parity, unauthenticated SSE replay, live Go HTTP route,
default Go backend, renderer-visible Go route, packaged desktop restart/resume
QA, release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration.
```

Final command gate for D-0111:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1507 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Runtime Settings Legacy-Agent Pollution Guard Addendum

Scope:
Active analytix runtime settings now reject runtime-contained legacy agent
shapes from Reasonix/Kun-style envelopes. Migration may still read explicit
legacy import paths, but normalized top-level `runtime` settings and IPC
settings patches do not persist `agent`, `agentProvider`, `agents`, `deepseek`,
or `reasonix` runtime pollution.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime settings normalization | `npm run test -- src/shared/app-settings.test.ts -t "Reasonix auto-plan" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 2 tests |
| IPC settings schema boundary | `npm run test -- src/main/ipc/app-ipc-schemas.test.ts -t "legacy.*settings\|agent-shaped" --no-file-parallelism --maxWorkers=1` | pass: 1 file / 2 tests |

Accepted claims:

```text
Analytix-owned active runtime settings preserve top-level runtime schema
sovereignty and reject legacy agent/provider envelopes before save or IPC patch.
```

Rejected claims:

```text
No Reasonix project config parity, public Reasonix settings protocol, old agent
provider switcher restoration, DeepSeek legacy active envelope, Kun identity,
deprecated bridge/settings fallback, default Go backend, renderer-visible Go
route, Rust/Tauri migration, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry.
```

Final command gate for D-0112:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Task-Job Route Executable Control Shadow Addendum

Scope:
G5 task-job route executable evidence now runs through typed control shadow,
not only summary replay. `controlExecutableCases.taskJobs.routeExecutable`
mirrors the TS-owned task-job route oracle and Go computes unauthorized,
output, wait, kill, missing-output, and rehydrated route status output.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job route control conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.489s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay task-job route status behavior from
TS-owned fixtures without exposing Go or Reasonix routes.
```

Rejected claims:

```text
No live Go task-job route readiness, live Go Job Manager, Reasonix SessionAPI
or public job protocol, renderer-visible Go route, default Go backend,
top-level Subagent/Workflow/Create Loop/AutoResearch route, Kun identity,
Rust/Tauri migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0113:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Task-Job Lifecycle Control Shadow Addendum

Scope:
G5 task-job lifecycle evidence now runs through typed control shadow, not only
summary replay. `controlExecutableCases.taskJobs.lifecycle` mirrors the
TS-owned task-job foreground, background, and wait/output/kill lifecycle oracle.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Task-job lifecycle control conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.419s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay task-job lifecycle behavior from
TS-owned fixtures without exposing Go or Reasonix lifecycle routes.
```

Rejected claims:

```text
No live Go task-job lifecycle readiness, live Go Job Manager, Reasonix
SessionAPI or public lifecycle protocol, renderer-visible Go route, default Go
backend, top-level Subagent/Workflow/Create Loop/AutoResearch route, Kun
identity, Rust/Tauri migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0114:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Planner-Executor Control Shadow Addendum

Scope:
G5 planner/executor evidence now runs through typed control shadow, not only
summary replay. `controlExecutableCases.taskJobs.plannerExecutor` mirrors the
TS-owned planner/executor task-job oracle for failure, cancellation,
output-offset, and transcript propagation.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Planner-executor control conformance | `npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.449s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay planner/executor propagation from
TS-owned fixtures without exposing Go or Reasonix planner/job routes.
```

Rejected claims:

```text
No live Go planner/job readiness, live Go Job Manager, Reasonix SessionAPI or
public planner/job protocol, renderer-visible Go route, default Go backend,
top-level Subagent/Workflow/Create Loop/AutoResearch route, Kun identity,
Rust/Tauri migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0115:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Release Guard Control Shadow Addendum

Scope:
G5 provider/cache release-guard evidence now runs through typed control shadow,
not only summary replay. `controlExecutableCases.providerCacheReleaseGuard`
mirrors the TS-owned provider/cache oracle for fixture-only cache-hit tail
average, allowed-low case count, collapse count, and pass/fail status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider cache release guard conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 16 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.483s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider cache release guard from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0116:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Usage Parser Control Shadow Addendum

Scope:
G5 provider usage parser evidence now runs through typed control shadow, not
only summary replay. `controlExecutableCases.providerUsageParser` mirrors the
TS-owned provider/cache oracle for DeepSeek native cache precedence, OpenAI
Responses cached tokens, Anthropic cache fields, and unsupported fallback.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider usage parser conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 16 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.385s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider usage parser precedence
from TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0117:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Request Shape Control Shadow Addendum

Scope:
G5 provider request-shape evidence now runs through typed control shadow, not
only summary replay. `controlExecutableCases.providerRequestShape` mirrors the
TS-owned provider/cache oracle for exact URL count, endpoint families, custom
full endpoint ids, tool-shape families, and required/forbidden body field
counts.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider request-shape conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 16 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider request shape from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0118:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Accounting Control Shadow Addendum

Scope:
G5 provider cache accounting evidence now runs through typed control shadow,
not only summary replay. `controlExecutableCases.providerCacheAccounting`
mirrors the TS-owned provider/cache oracle for supported telemetry ids,
unsupported unknown ids, DeepSeek/OpenAI/Anthropic provider-family ids,
hit/miss totals, aggregate cache hit rate, and unsupported fallback behavior.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider cache accounting conformance | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 16 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.384s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider cache accounting from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0119:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1508 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Release/Package Identity Sovereignty Guard Addendum

Scope:
Product-sovereignty guard coverage now includes release/package identity
surfaces. The scan covers package manifests, Electron builder config, scripts,
workflows, and build configuration; the packaging test directly asserts
analytix root package identity, app id, product name, artifact template, NSIS
names, and runtime CLI bin.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Packaging identity guard | `npm run test -- src/main/packaging-config.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |

Accepted claims:

```text
Analytix release/package identity is guarded at source/config level.
```

Rejected claims:

```text
No packaged Electron smoke, signing/notarization readiness, release readiness,
Kun/Reasonix public identity acceptance, default Go backend, renderer-visible
Go route, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
route, or Rust/Tauri migration.
```

Final command gate for D-0120:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Streaming Control Shadow Addendum

Scope:
G5 provider streaming evidence now runs through typed control shadow, not only
summary replay. `controlExecutableCases.providerStreaming` mirrors the TS-owned
G3 provider streaming oracle for SSE event order, usage event matching,
prompt/completion/reasoning/cache token fields, cache hit rate, telemetry
support, and product boundary.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider streaming conformance | `npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 8 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.482s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider streaming usage from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider streaming matrix, live Go provider client, Reasonix provider/cache
public protocol, renderer-visible Go route, default Go backend, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity,
Rust/Tauri migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0121:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Offline Parity Seal Control Shadow Addendum

Scope:
G5 provider offline parity evidence now runs through typed control shadow, not
only summary replay. `controlExecutableCases.providerOfflineParitySeal` mirrors
minimal TS-owned provider-cache oracle inputs for stable prefix equivalence,
DeepSeek cache telemetry, request-shape coverage, release guard status, and
fixture-only/live-superiority policy.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider offline parity conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.404s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider offline parity seal from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0122:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Session Route Status Control Shadow Addendum

Scope:
G5 thread/session route evidence now runs through typed control shadow, not
only summary replay. `controlExecutableCases.sessionRouteStatus` mirrors the
TS-owned G2 route oracle for archive/search, read/update, fork, session resume,
SSE replay/caught-up, event names, and missing-token auth rejection.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Session route status conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.479s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay thread/session route status from
TS-owned fixtures without exposing Go or Reasonix session routes.
```

Rejected claims:

```text
No live Go thread/session routes, Reasonix SessionAPI/job protocol, renderer-
visible Go route, default Go backend, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri migration, packaged
desktop QA, or release readiness.
```

Final command gate for D-0123:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Live-Local HTTP Control Shadow Addendum

Scope:
G5 provider live-local HTTP evidence now runs through typed control shadow, not
only summary replay. `controlExecutableCases.providerLiveLocalHttpProof`
mirrors minimal TS-owned provider-cache oracle inputs for fixture policy, local
HTTP transport, usage/request-shape counts, expected POST count, endpoint
formats, provider families, and no-live-superiority status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider live-local HTTP conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.391s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider live-local HTTP proof from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
provider matrix, live Go provider client, Reasonix provider/cache public
protocol, renderer-visible Go route, default Go backend, top-level Workflow/
Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri
migration, packaged desktop QA, or release readiness.
```

Final command gate for D-0124:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Drift Attribution Control Shadow Addendum

Scope:
G5 provider drift attribution evidence now runs through typed control shadow,
not only summary replay. `controlExecutableCases.providerDriftAttribution`
mirrors TS-owned provider-cache oracle drift inputs for previous/current prefix
hashes, stable system/prefix-items hashes, tools/provider/model/endpoint drift,
expected reasons, telemetry support, and cache-hit-rate known status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider drift attribution conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 6 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.465s` |

Accepted claims:

```text
Analytix Go G5 shadow can executable-replay provider drift attribution from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Rejected claims:

```text
No live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/custom
cache matrix, live Go provider client, Reasonix provider/cache public protocol,
renderer-visible Go route, default Go backend, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer route, Kun identity, Rust/Tauri migration,
packaged desktop QA, or release readiness.
```

Final command gate for D-0125:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1509 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## MCP Refresh Catalog Drift Replay Addendum

Scope:
MCP search catalog currentness now has an executable drift proof. The fake MCP
catalog starts with `search_issues`, expands to `search_issues` +
`create_issue`, and `mcp_refresh_catalog` must return `totalIndexed: 2` and
`catalogDrift: true`. G5 shadow replays that TS-owned proof as
`mcpReplay.searchRefreshDrift` with `topLevelRouteExposed: false`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP refresh drift + G5 conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 9 tests |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go 0.466s` |

Accepted claims:

```text
Analytix detects MCP search catalog drift through `mcp_refresh_catalog` and
G5 shadow replays the result from TS-owned fixtures.
```

Rejected claims:

```text
No Reasonix MCP-indexer public lifecycle protocol, top-level MCP-indexer route
or navigation, live Go MCP client, default Go backend, renderer-visible Go
route, credentialed MCP matrix, packaged MCP QA, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0110:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1507 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Workflow/Create Loop Quarantine Scan Addendum

Scope:
Dormant Workflow/Create Loop source remains present, but top-level app entry
surfaces now have explicit test and scan coverage proving those symbols are
quarantined.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Route-surface quarantine | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 5 tests |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Workflow/Create Loop symbols are rejected from top-level app entry surfaces,
including Workbench/Sidebar/Write/Plugins/store/preload/tray/settings shortcut
sources.
```

Rejected claims:

```text
No removal of dormant workflow files, top-level Workflow/Create Loop route,
Subagent/AutoResearch/MCP-indexer entry, Reasonix public protocol, packaged
desktop QA, release readiness, default Go backend, Kun identity, deprecated
bridge/settings fallback, or Rust/Tauri migration.
```

Final command gate for D-0109:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Route-surface quarantine | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 5 tests |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1507 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |

## MCP Stdio Execution-Error Redaction Addendum

Scope:
MCP tool-result privacy now covers actual stdio MCP execution failures, not
only diagnostics summaries. The executable fake indexer returns a
protocol-level `isError` payload containing a secret; analytix redacts it
before it becomes model-visible `tool_result.output`.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP stdio execution-error redaction | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 3 tests |

Accepted claims:

```text
Analytix redacts both thrown MCP errors and MCP protocol-level `isError`
payloads before model-visible tool-result persistence.
```

Rejected claims:

```text
No Reasonix MCP-indexer public protocol, top-level MCP-indexer route/navigation,
credentialed MCP matrix, packaged MCP QA, release readiness, default Go
backend, renderer-visible Go route, Kun identity, deprecated bridge/settings
fallback, or Rust/Tauri migration.
```

Final command gate for D-0108:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| MCP focused tests | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 24 tests |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1506 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...` | pass: `ok github.com/analytix/runtime-go (cached)` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Browser Bridge And Visible Entry Sovereignty Addendum

Scope:
Browser preview bridge and visible sidebar entry surfaces are now included in
product-sovereignty evidence. The route-surface test reads the browser preview
bridge source and rejects deprecated aliases; the sidebar render smoke rejects
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer visible labels; the
sovereignty scan now has path freshness coverage for Workbench, Sidebar,
preload/shared contracts, browser bridge, and guard tests.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Browser bridge + visible entry tests | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 12 tests |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Analytix has source and rendered-shell evidence that browser preview bridge and
visible sidebar entry surfaces stay on analytix-owned contracts.
```

Rejected claims:

```text
No Reasonix public protocol, Kun Workflow/Create Loop top-level navigation,
Subagent/AutoResearch/MCP-indexer product entry, deprecated bridge/settings
fallback, default Go backend, renderer-visible Go route, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

Final command gate for D-0155:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 776 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1510 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.404s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Coverage Floor Addendum

Scope:
Provider/cache oracle breadth is now executable evidence. The schema and
runtime test prove DeepSeek, OpenAI-compatible chat, OpenAI Responses,
Anthropic Messages, and custom full endpoint coverage cannot silently disappear
from the fixture chain that Go G3/G5 consumes.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache coverage floor | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 19 tests |

Accepted claims:

```text
Provider/cache fixtures now have an executable coverage floor across DeepSeek,
OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full
endpoints.
Unsupported provider cache telemetry remains unknown rather than counted as
misses.
Go G3/G5 shadow inputs remain aligned with the tightened TypeScript oracle.
```

Rejected claims:

```text
No live provider/cache superiority.
No credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix.
No Reasonix provider public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, or release readiness.
```

Final command gate for D-0168:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 778 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.429s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## G5 Provider Cache Coverage Floor Control Shadow Addendum

Scope:
The provider/cache coverage floor from D-0168 is now replayed by Go G5
`controlExecutableCases.providerCacheCoverageFloor`. Go derives provider family
coverage, endpoint-format breadth, telemetry-supported/unsupported case ids,
custom full-endpoint exact URL/tool-shape evidence, and product-boundary flags
from the TS-owned oracle. This remains shadow-only and does not create a live
Go provider client or default backend.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Provider/cache coverage floor + Go binding | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 19 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.191s` |

Accepted claims:

```text
Go G5 shadow can executable-replay the five-family provider/cache coverage
floor across DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic
Messages, and custom full endpoints.
Custom full-endpoint exact URL behavior and tool-shape coverage are preserved
as analytix-owned fixture evidence.
Unsupported provider cache telemetry remains unknown and is not promoted to a
miss-count claim.
```

Rejected claims:

```text
No live Go provider client.
No credentialed provider/cache matrix or live cache superiority claim.
No Reasonix provider public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, G6 readiness, or release readiness.
```

Final command gate for D-0173:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 779 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.492s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Combined Step/Cancel/Cache Proof Freshness Addendum

Scope:
Combined route-cache, step-limit, and cancel evidence is now a focused G5
oracle proof and a product-sovereignty scan freshness requirement. This
strengthens D-0146 without changing active TypeScript runtime behavior or Go
backend status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Focused combined G5 trace proof | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 7 tests |
| Product sovereignty combined token scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
G5 conformance directly proves same-turn route-cache reuse through step limit,
next-turn reroute, stable-prefix isolation, completed/running/unstarted cancel
result pairing, and product boundary flags.
Product-sovereignty scan now requires combined step/cancel/cache proof tokens
across the post-881 runtime proof paths.
```

Rejected claims:

```text
No runtime behavior change.
No live Go agent loop.
No Reasonix controller/session public protocol or config shape.
No public auto-plan setting, default Go backend, or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
cancel/cache QA, G6 readiness, or release readiness.
```

Final command gate for D-0174:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 780 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.413s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Approval/User-Input Proof Freshness Addendum

Scope:
Approval/user-input gate safety is now a focused G5 oracle proof and a
product-sovereignty scan freshness requirement. This strengthens the gate
evidence without changing active TypeScript runtime behavior or Go backend
status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Focused approval/user-input G5 proof | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 8 tests |
| Product sovereignty approval/user-input token scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
G5 conformance directly proves denied approval no-execute, user-input answer
privacy, invalid structured-choice no-open behavior, abort cleanup, resume
pending gate cleanup, and product boundary flags.
Product-sovereignty scan now requires approval/user-input proof tokens across
the post-881 runtime proof paths.
```

Rejected claims:

```text
No runtime behavior change.
No live Go approval/user-input manager.
No Reasonix ask/session public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
approval-card QA, G6 readiness, or release readiness.
```

Final command gate for D-0175:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 781 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.411s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Session Route Proof Freshness Addendum

Scope:
Thread/session route replay is now a focused G5 oracle proof and a
product-sovereignty scan freshness requirement. This strengthens route/SSE
evidence without changing active TypeScript runtime behavior or Go backend
status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Focused session route G5 proof | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Product sovereignty session route token scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
G5 conformance directly proves archive/search, read/update, fork,
resume-thread, SSE replay/caught-up, unauthorized auth, exact body/SSE hashes,
route inventory, and product boundary flags.
Product-sovereignty scan now requires session route replay proof tokens across
the post-881 runtime proof paths and includes the G2 route oracle fixture.
```

Rejected claims:

```text
No runtime behavior change.
No live Go thread/session router.
No Reasonix SessionAPI/public event protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
route walkthrough QA, G6 readiness, or release readiness.
```

Final command gate for D-0176:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.473s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Release Evidence Final-Gate Freshness Addendum

Scope:
Post-881 release evidence final command gates are now machine-checkable by
`scan:product-sovereignty`. This strengthens stage-closure evidence without
changing active TypeScript runtime behavior, provider behavior, bridge
contracts, or Go backend status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty release-evidence gate scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
Product-sovereignty scan now includes the release evidence file in proof path
freshness.
Product-sovereignty scan now requires final command gate entries for D-0172
through D-0177.
Product-sovereignty scan now requires the release evidence file to keep runtime
package tests, workspace tests, typecheck, runtime build, non-cached Go shadow
tests, and product-sovereignty scan commands visible.
```

Rejected claims:

```text
No runtime behavior change.
No provider behavior change.
No live Go backend or renderer-visible Go route.
No replacement for actually running the full gate suite.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
desktop QA, live provider/MCP matrix, G6 readiness, or release readiness.
```

Final command gate for D-0177:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.486s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Post-881 Stage Closure Snapshot Addendum

Scope:
The current post-881 capability floor, absorbed Reasonix delta families, Kun
baseline preservation, verified stronger-than areas, and remaining open gates
are now captured in a machine-scannable closure snapshot. This strengthens
stage-closure evidence without changing active TypeScript runtime behavior,
provider behavior, bridge contracts, or Go backend status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty closure snapshot scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
`post-881-stage-closure-2026-06-22.md` records the fixture-level capability
floor, absorbed Reasonix deltas, Kun baseline preservation, verified
stronger-than areas, and remaining open gates.
Product-sovereignty scan now requires closure snapshot tokens for capability
floor, Reasonix absorption, Kun baseline, stronger-than evidence, and open
gates.
```

Rejected claims:

```text
No runtime behavior change.
No provider behavior change.
No live Go backend or renderer-visible Go route.
No live Reasonix parity or live provider/cache superiority claim.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
desktop QA, live provider/MCP matrix, G6 readiness, or release readiness.
```

Final command gate for D-0178:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.489s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0172:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 778 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.412s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Raw Accounting Proof Freshness Addendum

Scope:
Raw provider cache accounting is now a direct provider-cache oracle invariant
and a product-sovereignty scan freshness requirement. This strengthens D-0172
without changing active provider behavior or Go backend status.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Direct provider-cache raw accounting proof | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 12 tests |
| Product sovereignty raw accounting token scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Provider-cache proof directly derives raw usage snapshots and aggregate cache
accounting from provider response bodies.
Product-sovereignty scan now requires raw accounting proof tokens across the
post-881 runtime proof paths.
Unsupported provider cache telemetry remains unknown and is not promoted to a
miss-count claim.
```

Rejected claims:

```text
No provider runtime behavior change.
No live Go provider client.
No credentialed provider/cache matrix or live superiority claim.
No Reasonix provider public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, G6 readiness, or release readiness.
```

## Post-881 Sub-Agent Review Matrix Addendum

Scope:
The six-lane sub-agent review is now a scan-visible QA artifact. It records
Reasonix agent-kernel, Kun baseline, runtime/cache, MCP/tool/sub-agent, Go
runtime, and QA/docs findings; it also corrects the custom provider claim to
request-shape-only and records the Go conformance D-0177 range wording fix.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty sub-agent review scan | `npm run scan:product-sovereignty` | pass: new review matrix path and tokens are scan-visible |

Accepted claims:

```text
Sub-agent review findings are reduced to a machine-scannable decision matrix.
Reasonix deltas are classified as contract-reimplement, code-port-and-adapt,
document-only, reject, or defer.
Custom provider proof is request-shape/full-endpoint coverage only, not
independent custom cache telemetry.
```

Rejected claims:

```text
No runtime behavior change.
No live provider/cache superiority matrix.
No live Go router/provider/gate/job manager.
No default Go backend or renderer-visible Go route.
No Reasonix public protocol or config shape.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
QA, G6 readiness, or release readiness.
```

Final command gate for D-0179:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.420s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Custom Provider Telemetry Seal Addendum

Scope:
Custom full endpoints now have executable request-shape-only proof. The
provider-cache oracle, TypeScript conformance, and Go G5 shadow all require
custom full endpoint telemetry ids to remain empty while DeepSeek/OpenAI
Responses/Anthropic cache telemetry remains supported.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Custom provider telemetry seal | `npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 21 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.400s` |

Accepted claims:

```text
Custom full endpoint ids remain request-shape cases.
customFullEndpointTelemetryCaseIds is empty.
telemetrySupportedExcludesCustomFullEndpoints is true.
customProviderCacheTelemetryClaimAllowed is false.
```

Rejected claims:

```text
No provider runtime behavior change.
No independent custom provider cache telemetry.
No live provider/cache superiority matrix.
No live Go provider client or default Go backend.
No renderer-visible Go route.
No Reasonix public provider protocol or config shape.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, G6 readiness, or release readiness.
```

Final command gate for D-0180:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.196s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Goal Persistence Off-Lock Control Shadow Addendum

Scope:
Goal persistence off-lock behavior is now source-derived G5 control-shadow
evidence. The proof reads real `ThreadService` source and its focused test to
pin persist-before-event ordering, warning/error behavior, and absence of
controller/status/approval lock coupling.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Goal persistence off-lock control shadow | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/thread-service.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 24 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.420s` |

Accepted claims:

```text
setGoal persists before goal_updated.
clearGoal persists before goal_cleared.
No forbidden controller/status/approval lock substrings are present.
Goal persistence failures warn with analytix identity and surface the original
error.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No live Go goal manager.
No TypeScript controller-lock migration.
No Reasonix controller protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
goal QA, G6 readiness, or release readiness.
```

Final command gate for D-0181:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.223s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0182:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.233s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0183:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.176s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0184:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.220s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Desktop Bridge IPC Sovereignty Shadow Addendum

Scope:
The existing G5 `desktopSovereignty` control case now covers preload runtime
request and SSE IPC channel ownership. It records `runtime:request`,
`runtime:sse:start`, `runtime:sse:stop`, `runtime:sse-event`,
`runtime:sse-end`, and `runtime:sse-error`, plus an empty forbidden
Reasonix/Kun/Go/workflow IPC exposure list.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Desktop bridge IPC conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.487s` |

Accepted claims:

```text
G5 desktop sovereignty now computes runtimeRequestUsesAnalytixIpc,
runtimeSseUsesAnalytixIpc, publicApiTypesAnalytixOwned, and
forbiddenRuntimeIpcExposed from source-derived fixture inputs.
Product-sovereignty scan now requires D-0185 desktop bridge IPC proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI/frontend protocol.
No Kun/deprecated GUI bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri migration, packaged desktop bridge walkthrough, G6 readiness, or
release readiness.
```

Final command gate for D-0185:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.177s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Desktop Main IPC Boundary Shadow Addendum

Scope:
G5 control shadow now covers Electron main runtime request and SSE IPC boundary
enforcement. It records main IPC schema/handler sources, allowed analytix route
examples, forbidden public route examples, strict request proof, handler
no-execute proof, strict SSE payload proof, `/v1/threads/{id}/events`, matching
stream stop, reconnect cursor, and 100ms event batching.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Desktop main IPC conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.457s` |

Accepted claims:

```text
G5 desktop main IPC boundary now computes runtimeRequestSchemaStrict,
runtimeRequestHandlerRejectsBeforeRuntimeCall, sseRejectsReasonixSessionPayload,
sseUsesAnalytixThreadEventsRoute, sseReconnectCursorPreserved, and
sseBatchesEventsAt100ms from source-derived fixture inputs.
Product-sovereignty scan now requires D-0186 desktop main IPC proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI/main IPC protocol.
No Kun/deprecated GUI bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri migration, packaged desktop bridge walkthrough, G6 readiness, or
release readiness.
```

Final command gate for D-0186:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.173s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Renderer Route-Surface Sovereignty Shadow Addendum

Scope:
G5 control shadow now covers renderer top-level route sovereignty and dormant
workflow quarantine. It records Workbench route-surface, route/store,
browser-preview bridge, plugin marketplace, shell navigation, Workbench shell,
and base shell CSS sources; proves the `AppRoute` union stays
`chat/write/settings/plugins/claw/schedule`; rejects forbidden Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer route tokens and entrypoint symbols;
keeps dormant Workflow/Create Loop code quarantined; and preserves
`window.analytix` browser preview bridge plus shell safe-area/no-drag behavior.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer route-surface conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.458s` |

Accepted claims:

```text
G5 renderer route-surface sovereignty now computes
appRouteUnionKunCompatible, noForbiddenTopLevelRouteTokens,
noForbiddenEntrypointSymbols, dormantWorkflowCodeQuarantined,
browserPreviewBridgeAnalytixOnly, pluginMarketplaceSafeAreaPropagates,
shellNavigationNoDrag, and nativeControlsSafeInset from source-derived fixture
inputs.
Product-sovereignty scan now requires D-0187 renderer route-surface proof
tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix UI/session protocol.
No Kun/deprecated GUI bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Rust/Tauri migration, packaged desktop route walkthrough, G6 readiness, or
release readiness.
```

Final command gate for D-0187:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.172s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Package / Runtime CLI Identity Shadow Addendum

Scope:
G5 control shadow now covers package/runtime CLI identity. It records root and
runtime package manifests, electron-builder config, app identity, main
AppUserModelID, runtime binary resolver, runtime CLI entry/usage, afterPack
validation, packaging config test, and release workflow sources; proves package
and release identity stay analytix-owned; keeps runtime bin and usage as
`analytix serve`; and keeps `ANALYTIX_READY` plus `ANALYTIX_*` release env
ownership visible.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Package/runtime identity conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.179s` |

Accepted claims:

```text
G5 package/runtime CLI identity now computes rootPackageNameAnalytix,
runtimeBinAnalytixServeEntry, builderAppIdAnalytix, nsisNamesAnalytix,
windowsAppUserModelIdAnalytix, resolveBundledServeEntry,
serveEntryAllowsOnlyAnalytixServe, readyHandshakeAnalytix,
afterPackRequiresServeEntry, and releaseEnvAnalytixPrefixed from source-derived
fixture inputs.
Product-sovereignty scan now requires D-0188 package/runtime identity proof
tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix CLI/session protocol.
No Kun/DeepSeek public product identity.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, packaged artifact walkthrough, G6 readiness, or
release readiness.
```

Final command gate for D-0188:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.195s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0200 Renderer Runtime Client Bridge Proof

Scope:

```text
D-0200 proves the renderer runtime client facade preserves runtime request and
SSE arguments through `window.analytix.runtime` and does not read legacy bridge
aliases.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer runtime client bridge proof | `npx vitest run src/renderer/src/agent/runtime-client.test.ts` | pass: 1 file / 5 tests |

Accepted claims:

```text
`rendererRuntimeClient.runtimeRequest` forwards path/method/body unchanged.
`startSse`, `stopSse`, and SSE listener registration forward arguments and
handlers unchanged. Throwing `window.kun` and `window.reasonix` getters are not
read.
Product-sovereignty scan now requires D-0200 renderer runtime client bridge
proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public bridge protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged runtime/SSE walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0200:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1535 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.415s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0201 Renderer Settings Bridge Proof

Scope:

```text
D-0201 proves the renderer settings client facade preserves top-level runtime
settings patches through `window.analytix.settings` and does not read legacy
bridge aliases.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer settings bridge proof | `npx vitest run src/renderer/src/agent/runtime-client.test.ts` | pass: 1 file / 6 tests |

Accepted claims:

```text
`rendererRuntimeClient.setSettings` forwards a top-level `runtime` patch through
`window.analytix.settings.setSettings` unchanged, including `runtime.model` and
`runtime.approvalPolicy`.
The renderer settings cache is refreshed from the returned settings.
Throwing `window.kun` and `window.reasonix` getters are not read.
Product-sovereignty scan now requires D-0201 renderer settings bridge proof
tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix config root, SessionAPI, or public settings protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged settings walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0201:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.425s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0202 Renderer Runtime Provider Alias Guard

Scope:

```text
D-0202 proves renderer runtime provider tests run with throwing legacy bridge
alias getters while covering analytix-owned route, approval/user-input,
fork/resume, and dynamic route-id encoding paths.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer runtime provider alias guard | `npx vitest run src/renderer/src/agent/analytix-runtime.test.ts` | pass: 1 file / 26 tests |

Accepted claims:

```text
`AnalytixRuntimeProvider` tests now install throwing `window.kun` and
`window.reasonix` getters through the shared provider bridge helper.
Existing provider tests continue to cover analytix-owned HTTP routes, thread
lifecycle archive/search, approval/user-input submit/cancel, fork/resume, and
dynamic route-id encoding under the alias guard.
Product-sovereignty scan now requires D-0202 renderer runtime provider alias
guard proof tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public bridge protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged renderer walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0202:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.488s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0203 Renderer Provider Runtime Client Facade Seal

Scope:

```text
D-0203 routes archive/restore through `rendererRuntimeClient.runtimeRequest`
and adds a product-sovereignty scan against direct runtime request bridge
bypasses inside `analytix-runtime.ts`.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer provider runtime client facade | `npx vitest run src/renderer/src/agent/analytix-runtime.test.ts` | pass: 1 file / 26 tests |
| Direct provider bridge bypass check | `npm run scan:product-sovereignty` | pass: scan forbids `window.analytix.runtime.runtimeRequest` in `analytix-runtime.ts` |

Accepted claims:

```text
`AnalytixRuntimeProvider.archiveThread` now uses
`rendererRuntimeClient.runtimeRequest`, matching the rest of the renderer
provider runtime request surface.
Archive/restore keeps the same analytix thread route and PATCH body.
Product-sovereignty scan now requires D-0203 archive facade proof tokens,
forbids direct provider bridge bypass, and requires final gate evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI or public bridge protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged renderer walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0203:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.477s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0204 Side Conversation Relation Provider Contract

Scope:

```text
D-0204 routes side conversation promotion through
`AgentProvider.updateThreadRelation`, implements the Analytix provider relation
PATCH through `rendererRuntimeClient.runtimeRequest`, and extends the
product-sovereignty scan against direct runtime bridge bypasses in side-store
source.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Side relation provider contract | `npx vitest run src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts` | pass: 2 files / 37 tests |
| Direct provider/store bridge bypass check | `npm run scan:product-sovereignty` | pass: scan forbids `window.analytix.runtime.runtimeRequest` in provider and side-store sources |

Accepted claims:

```text
`AgentProvider.updateThreadRelation` is an analytix-owned optional provider
contract.
`AnalytixRuntimeProvider.updateThreadRelation` sends relation PATCH requests
through `rendererRuntimeClient.runtimeRequest`.
`promoteSideConversation` calls `provider.updateThreadRelation(sideId,
'primary')`, refreshes threads, and closes the side panel.
Product-sovereignty scan now requires D-0204 side relation proof tokens,
forbids direct store/provider bridge bypass, and requires final gate evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI, side-conversation public protocol, or public bridge
protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged side-conversation
walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0204:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.405s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0205 Renderer Usage Runtime Client Facade Seal

Scope:

```text
D-0205 routes renderer thread/day/model usage, token economy savings, and LLM
debug runtime HTTP requests through `rendererRuntimeClient.runtimeRequest`, and
extends product-sovereignty scan coverage to forbid direct generic runtime
request bridge calls in renderer production source.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer usage runtime client facade | `npx vitest run src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts` | pass: 4 files / 39 tests |
| Renderer direct runtime request bypass check | `npm run scan:product-sovereignty` | pass: scan forbids `window.analytix.runtime.runtimeRequest` in renderer production source |

Accepted claims:

```text
Thread, daily, and model usage loaders now use
`rendererRuntimeClient.runtimeRequest`.
Settings token economy savings and LLM debug runtime HTTP requests use the same
facade.
Request paths, parsing, timeout, and error behavior remain covered by existing
usage/settings tests.
Product-sovereignty scan now requires D-0205 proof tokens, forbids direct
generic runtime request bridge calls in renderer production source, and
requires final gate evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI, usage/debug public protocol, or public bridge protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged usage/dashboard walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0205:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.436s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0206 Renderer Settings Read Facade Seal

Scope:

```text
D-0206 routes ordinary renderer settings reads through
`rendererRuntimeClient.getSettings` and extends product-sovereignty scan
coverage to forbid direct `window.analytix.settings.getSettings` in renderer
production source.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer settings read facade | `npx vitest run src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts src/renderer/src/agent/runtime-client.test.ts` | pass: 2 files / 14 tests |
| Renderer direct settings read bypass check | `npm run scan:product-sovereignty` | pass: scan forbids `window.analytix.settings.getSettings` in renderer production source |

Accepted claims:

```text
Keyboard shortcut settings, speech-to-text settings, and initial usage model
label reads now use `rendererRuntimeClient.getSettings`.
Existing settings-changed event listeners remain unchanged.
Product-sovereignty scan now requires D-0206 proof tokens, forbids direct
renderer settings read bypass, and requires final gate evidence.
```

Rejected claims:

```text
No Reasonix config root, SessionAPI, public settings protocol, or public bridge
protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged settings walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0206:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.448s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0207 Renderer Named Bridge API Allow-list

Scope:

```text
D-0207 makes remaining direct renderer `window.analytix.runtime.*` and
`window.analytix.settings.*` production calls explicit named allow-listed
contracts. Generic runtime HTTP requests and ordinary settings reads remain
sealed behind `rendererRuntimeClient`.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Scan syntax | `node --check scripts/scan-product-sovereignty.cjs` | pass |
| Direct bridge allow-list spot check | `rg -n "window\\.analytix\\.(runtime|settings)\\.[A-Za-z0-9_]+" src/renderer/src --glob '!**/*.test.ts'` | pass: all matches are allowed named APIs |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
Direct renderer runtime bridge calls are limited to named analytix preload
contracts for config file access, provider probe, runtime restart, upstream
model fetch, and runtime status subscription.
Direct renderer settings bridge calls are limited to named settings writes.
Generic runtime HTTP requests and ordinary settings reads remain forbidden as
direct renderer bridge calls.
Product-sovereignty scan now requires D-0207 proof tokens, named bridge
allow-list scans, and final gate evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI, public bridge/session protocol, config root, or public
route protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged bridge walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0207:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.389s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0208 Renderer Optional Bridge Bypass Seal

Scope:

```text
D-0208 treats optional-chain direct bridge access as the same renderer
production surface as dot access. It removes optional-chain generic
runtime/settings bypasses and extends product-sovereignty scans to cover both
forms.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Scan syntax | `node --check scripts/scan-product-sovereignty.cjs` | pass |
| Optional-chain/dot generic bypass check | `rg -n "window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings|window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest" src/renderer/src --glob '!**/*.test.ts'` | pass: no matches |
| Renderer runtime client / dialog helper tests | `npx vitest run src/renderer/src/agent/runtime-client.test.ts src/renderer/src/components/chat/SidebarClawDialogHelpers.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 7 tests |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
Optional-chaining direct bridge access is governed by the same renderer scan
rules as dot access.
Connect Phone dialog settings loading now uses `rendererRuntimeClient`.
Plugin marketplace diagnostics no longer probe generic `runtimeRequest`
directly.
Product-sovereignty scan now requires D-0208 proof tokens and final gate
evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI, public bridge/session protocol, config root, or public
route protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged bridge walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0208:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.431s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0209 Renderer Bridge Allow-list G5 Shadow

Scope:

```text
D-0209 promotes renderer bridge allow-list evidence into the existing G5
`desktopSovereignty` executable shadow. The fixture is source-derived from
renderer production code and product-sovereignty scan patterns; Go computes the
same allow-list and bypass-absence output without enabling a live Go bridge.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 desktop sovereignty conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.397s` focused, `0.199s` final |
| Scan syntax | `node --check scripts/scan-product-sovereignty.cjs` | pass |
| Whitespace | `git diff --check` | pass |

Accepted claims:

```text
`desktopSovereignty.rendererBridgeAllowList` records named runtime/settings
allow-lists, direct source-derived methods, empty violation sets, zero generic
bypass counts, and optional-chain scan coverage.
Go shadow computes renderer named API allow-list status and generic bypass
absence from the TS-owned fixture.
Product-sovereignty scan now requires D-0209 G5 proof tokens and final gate
evidence.
```

Rejected claims:

```text
No Reasonix SessionAPI, public bridge/session protocol, config root, or public
route protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go bridge, packaged bridge walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0209:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.181s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0210 Runtime HTTP Auth Matrix G5 Shadow

Scope:

```text
D-0210 promotes the D-0190 runtime HTTP auth matrix into
`runtimeHttpRouteSovereignty` G5 executable shadow. The real TypeScript router
still dispatches every registered route without auth; Go computes auth-matrix
coverage from the TS-owned fixture without implementing a live HTTP server.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 runtime HTTP auth matrix conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.446s` |

Accepted claims:

```text
`runtimeHttpRouteSovereignty.authMatrix` records `/health` 200, all 43
protected `/v1/*` route keys, canonical unauthorized status/body, and sensitive
SSE/task-job/approval/user-input/resume route keys.
The real TypeScript router compares every unauthenticated route dispatch
against the fixture matrix.
Go shadow computes health public status, all-protected-route 401 status,
unauthorized body shape, full matrix coverage, and sensitive route protection.
Product-sovereignty scan now requires D-0210 auth matrix proof tokens and final
gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0210:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.446s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0211 Runtime Forbidden Route Dispatch G5 Shadow

Scope:

```text
D-0211 promotes the D-0191 runtime forbidden route dispatch matrix into
`runtimeHttpRouteSovereignty` G5 executable shadow. The real TypeScript router
still dispatches every forbidden route token with valid auth; Go computes
structured 404 coverage from the TS-owned fixture without implementing a live
HTTP server.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 runtime forbidden dispatch conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.172s` |

Accepted claims:

```text
`runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` records valid runtime
auth mode, 404 status/body, all 13 forbidden route tokens, six upstream
protocol tokens, and seven hidden-surface tokens.
The real TypeScript router compares every valid-auth forbidden route dispatch
against the fixture matrix.
Go shadow computes structured not_found, full-token coverage, protocol-token
rejection, hidden-surface rejection, and valid-auth dispatch mode.
Product-sovereignty scan now requires D-0211 forbidden-dispatch proof tokens
and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0211:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.192s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0212 Shared Endpoint Builder G5 Shadow

Scope:

```text
D-0212 promotes the D-0192 shared endpoint builder proof into
`runtimeHttpRouteSovereignty` G5 executable shadow. The TypeScript shared
endpoint contract remains authoritative for renderer/main/runtime path
construction; Go computes encoding/template ownership coverage from the
TS-owned fixture without implementing a live HTTP server.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 shared endpoint builder conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.379s` |

Accepted claims:

```text
`runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` records 18 shared
builder cases, encoded slash/query/fragment ids, exported endpoint strings,
forbidden-token absence, canonical plural user-input state, sensitive builder
names, and source unit-test proof.
Go shadow computes builder count, encoded route-id preservation, template
ownership, canonical plural user-input, sensitive builder coverage, and
unit-proof presence.
Product-sovereignty scan now requires D-0212 shared-endpoint G5 proof tokens
and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0212:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.180s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0213 Renderer Runtime Endpoint Builder G5 Shadow

Scope:

```text
D-0213 promotes the D-0193 renderer provider endpoint-builder proof into
`desktopSovereignty` G5 executable shadow. The TypeScript renderer provider
remains authoritative for live GUI path construction; Go computes shared-root,
encoded dynamic route-id, analytix-owned runtime path, facade, and unit-proof
coverage from the TS-owned fixture without implementing a live desktop bridge
or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer provider endpoint conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.384s` |

Accepted claims:

```text
`desktopSovereignty.rendererProviderEndpointMatrix` records shared root paths,
dangerous slash/query/fragment source ids, encoded dynamic runtime request
paths for turns, steer, interrupt, compact, approval, user-input, fork, and
resume-thread, renderer runtime-client facade use, ownership guard proof, and
unit-test proof.
Go shadow computes shared-root, encoded dynamic route-id, analytix-owned
runtime path, facade, and unit-proof outputs.
Product-sovereignty scan now requires D-0213 renderer-provider endpoint G5
proof tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, live Go desktop bridge, packaged
route walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0213:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.313s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0214 Main IPC Endpoint Builder G5 Shadow

Scope:

```text
D-0214 promotes the D-0194 main IPC endpoint-builder allow-list proof into
`desktopMainIpcBoundary` G5 executable shadow. The TypeScript main IPC schema
remains authoritative for live runtime request acceptance; Go computes encoded
shared-path acceptance, raw dynamic route rejection, shared-template use,
unit-proof presence, and singular user-input rejection from the TS-owned
fixture without implementing a live main IPC bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 main IPC endpoint conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.387s` |

Accepted claims:

```text
`desktopMainIpcBoundary.endpointBuilderAllowListMatrix` records encoded
shared-builder requests for thread, fork, turns, steer, interrupt, checkpoint,
approval, user-input, session, attachment, and memory runtime request paths;
raw dynamic route rejections; singular user-input rejection; shared template
compilation; and unit-test proof.
Go shadow computes accepted shared paths, raw dynamic rejection, shared-template
use, unit-proof presence, and singular user-input rejection.
Product-sovereignty scan now requires D-0214 main IPC endpoint-builder G5 proof
tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, live Go main IPC bridge, packaged
route walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0214:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.208s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0215 Main IPC Runtime Adapter Handoff G5 Shadow

Scope:

```text
D-0215 promotes the D-0195 main IPC runtime-adapter handoff proof into
`desktopMainIpcBoundary` G5 executable shadow. The TypeScript main IPC handler
remains authoritative for live runtime adapter invocation; Go computes encoded
path preservation, method/body preservation, raw dynamic route rejection,
reject-before-call ordering, and unit-proof presence from the TS-owned fixture
without implementing a live main IPC bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 main IPC handoff conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.396s` |

Accepted claims:

```text
`desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` records seven encoded
adapter calls, six rejected-before-adapter raw/singular calls,
parse-before-adapter ordering, encoded path proof, method/body proof, and
reject-before-adapter proof.
Go shadow computes encoded-path preservation, method/body preservation, raw
dynamic rejection, reject-before-call ordering, and unit-proof presence.
Product-sovereignty scan now requires D-0215 main IPC runtime-adapter handoff
G5 proof tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, live Go main IPC bridge, packaged
route walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0215:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.227s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0222 Renderer Provider Alias Guard G5 Shadow

Scope:

```text
D-0222 promotes the D-0202 renderer provider alias guard proof into
`desktopSovereignty` G5 executable shadow. The TypeScript
`AnalytixRuntimeProvider` remains authoritative for live renderer provider
calls; Go computes throwing alias installation, runtime route coverage,
lifecycle/gate coverage, fork/resume dynamic encoding coverage,
forbidden-route rejection, and unit-proof presence from the TS-owned fixture
without implementing a live Go desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer provider alias guard conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Renderer provider unit proof | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand` | pass: 1 file / 26 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.387s` |

Accepted claims:

```text
`desktopSovereignty.rendererProviderAliasGuardMatrix` records the
`installDsGui` helper, forbidden `kun` / `reasonix` aliases, guarded provider
tests, route/lifecycle/approval-user-input/fork-resume/dynamic-encoding
coverage, source-only analytix proof, and forbidden-route guard proof.
Go shadow computes throwing alias installation, provider route coverage,
lifecycle/gate coverage, fork/resume dynamic encoding coverage,
forbidden-route rejection, and unit-proof presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No Reasonix SessionAPI, public frontend bridge protocol, config root, or
public control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged provider walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0222:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.467s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0235 Go G4 Approval/User-Input/MCP Manager Live-Local Sidecar

Scope:

```text
D-0235 advances the test/conformance-only Go live-local sidecar from G3
provider/cache streaming replay to fixture-backed G4 approval/user-input/MCP
manager replay. The sidecar consumes TS-owned approval-user-input and MCP
lifecycle oracles and serves only local conformance routes below
`/v1/conformance/g4/manager/*`. It remains disconnected from Electron main,
preload, renderer, settings, and `analytix serve`.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime package G4 source/oracle guard | `npm --prefix packages/runtime test` | pass: 80 files / 786 tests |
| Go G4 approval/user-input/MCP manager sidecar | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.451s` |

Accepted claims:

```text
Go live-local evidence improved from a G3 provider/cache prototype to a
test/conformance-only G4 isolated manager prototype.
The local harness replays approval deny no-execute, pending gate counts,
second decision/resolve statuses, user-input cancel/submit status/body shapes,
structured invalid-choice no-gate behavior, HTTP answer echo with answer-free
resolved events, remote-entry allowed/forbidden control ports, MCP
connect/reload/disconnect/cancel/error, MCP approval annotation deny
no-execute, search meta-tools, untrusted workspace hiding, unknown tool error,
transport retry/protocol no-retry classification, background reconnect
classification, known override diagnostics, and secret redaction from
TS-owned fixtures.
`ApprovalExecutionAttempts`, `ToolExecutionAttempts`, `MCPConnectionAttempts`,
`MCPCredentialAttempts`, `CredentialReadAttempts`, `FileMutationAttempts`,
`EventsJSONLWriteAttempts`, and `RealWorkspaceWriteAttempts` remain `0`.
```

Rejected claims:

```text
No real tool execution.
No real approval execution.
No live Go user-input backend parity.
No real MCP server connection.
No MCP credential/API-key read.
No real workspace or `events.jsonl` mutation.
No TypeScript runtime replacement.
No `analytix serve` backend switch.
No Electron main, preload, renderer, settings, or route-surface connection.
No Reasonix SessionAPI, public control plane, MCP-indexer route, config root,
or product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No live MCP parity, credentialed MCP matrix, packaged desktop QA, G6, default
backend, release readiness, Rust, or Tauri claim.
```

Final command gate for D-0235:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 786 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.451s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0234 Go G3 Provider/Cache Streaming Live-Local Sidecar

Scope:

```text
D-0234 advances the test/conformance-only Go live-local sidecar from isolated
G2 lifecycle replay to fixture-backed G3 provider/cache streaming replay. The
sidecar consumes TS-owned provider/cache oracles and serves only local
conformance routes for provider usage parsing, request shape, SSE streaming,
cache accounting, cache drift, and bounded cache diagnostics. It remains
disconnected from Electron main, preload, renderer, settings, and `analytix
serve`.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Focused TS G3/G5 source and oracle guard | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go G3 live-local provider/cache sidecar | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.433s` |

Accepted claims:

```text
Go live-local evidence improved from a G2 lifecycle prototype to a
test/conformance-only G3 provider/cache streaming prototype.
The local harness replays 5 provider usage cases, 7 request-shape cases,
`item_delta -> usage -> turn_completed` SSE order, DeepSeek native hit/miss
precedence, OpenAI Responses cached tokens, Anthropic cache read/create fields,
unsupported cache unknown behavior, aggregate hit/miss accounting, stable
prefix diagnostics, and cache drift attribution from TS-owned fixtures.
`ProviderCallAttempts` remains `0`, diagnostics do not leak forbidden prompt,
tool, API-key, or Authorization substrings, and all provider/cache evidence is
fixture-backed.
```

Rejected claims:

```text
No real external provider call.
No real API-key read or provider credential use.
No TypeScript runtime replacement.
No `analytix serve` backend switch.
No Electron main, preload, renderer, settings, or route-surface connection.
No Reasonix SessionAPI, public provider/cache protocol, config root, or
product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No live provider/cache superiority, credentialed provider matrix, packaged
provider QA, G6, default backend, release readiness, Rust, or Tauri claim.
```

Final command gate for D-0234:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 786 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.468s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0233 Go Isolated Mutating G2 Lifecycle Sidecar

Scope:

```text
D-0233 advances the D-0232 test/conformance-only Go live-local sidecar from
read-only G1/G2 replay to isolated mutating G2 lifecycle replay. The sidecar
serves the four TS-owned mutating route fixtures for thread archive,
thread title/workspace update, side fork, and resume-thread through an
in-memory harness store and records state only in `LiveLocalSidecarSnapshot`.
It remains disconnected from Electron main, preload, renderer, settings, and
`analytix serve`.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Go isolated mutating G2 sidecar | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.212s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Accepted claims:

```text
Go live-local evidence improved from a read-only G1/G2 prototype to a
test/conformance-only isolated mutating G2 lifecycle prototype.
The sidecar aligns `PATCH /v1/threads/thr_g2_beta`,
`PATCH /v1/threads/thr_g2_read`, `POST /v1/threads/thr_g2_parent/fork`, and
`POST /v1/sessions/thr_g2_source/resume-thread` with the TS-owned G2 oracle.
Mutation state is confined to an in-memory harness snapshot, and a temp
`events.jsonl` sentinel remains unchanged.
Forbidden `/v1/runtime/go`, Reasonix, Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer routes remain unexposed, while rollback/default
backend guards remain false.
```

Rejected claims:

```text
No TypeScript runtime replacement.
No `analytix serve` backend switch.
No Electron main, preload, renderer, settings, or route-surface connection.
No Reasonix SessionAPI, public protocol, config root, or product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No live provider call, credentialed MCP matrix, approval execution, real
workspace/event-log mutation, or file mutation.
No durable Go store, G6, default backend, packaged walkthrough, release
readiness, Rust, or Tauri claim.
```

Final command gate for D-0233:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 786 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.212s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0232 Go Live-Local Sidecar Prototype

Scope:

```text
D-0232 moves Go runtime evidence from shadow-only to a test/conformance-only
live-local sidecar prototype. The Go handler is started only by tests, serves
G1 health/runtime-info/tools plus filtered G2 GET route and SSE replay fixture
cases, and compares live HTTP status/body/SSE frames against the TypeScript
oracle. It also proves G5 product-boundary guards remain false for Electron
main connection, default Go backend, and renderer-visible Go routes.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Go live-local sidecar conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 12 tests |
| Go sidecar replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.233s` |

Accepted claims:

```text
Go runtime has advanced from shadow-only to a test/conformance-only
live-local sidecar prototype.
The prototype serves analytix-owned G1 health/runtime info/tools and G2
read-only route/SSE replay fixtures through a real local Go HTTP server.
Mutating route replay, provider live calls, approval/user-input execution, MCP
credentials, file mutation, Electron main wiring, renderer-visible Go routes,
default Go backend, and Reasonix public protocol remain blocked.
Product-sovereignty scan now requires Go live-local sidecar prototype tokens
and a D-0232 final command gate.
```

Rejected claims:

```text
No TypeScript runtime replacement.
No `analytix serve` backend switch.
No Electron main, preload, renderer, settings, or route-surface connection.
No Reasonix SessionAPI, public protocol, config root, or product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No live provider call, credentialed MCP matrix, approval execution, or file
mutation.
No G6, default backend, packaged walkthrough, release readiness, Rust, or
Tauri claim.
```

Final command gate for D-0232:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 786 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.447s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0231 MCP Search Refresh Drift Evidence Closure

Scope:

```text
D-0231 does not change runtime behavior. It makes existing MCP catalog
refresh/currentness drift evidence mandatory in `scan:product-sovereignty` and
stage closure docs. The TypeScript MCP oracle and Go G5 shadow already replay
`mcp_refresh_catalog`, expanded catalog drift, `totalIndexed`, and
`catalogDrift`; this batch prevents that proof from disappearing silently.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP refresh/conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 14 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.542s` |

Accepted claims:

```text
`mcp-tool-lifecycle-oracle` and its runtime test already prove
`mcp_refresh_catalog` sees initial `search_issues`, expanded `search_issues` +
`create_issue`, `totalIndexed: 2`, and `catalogDrift: true`.
`controlExecutableCases.mcpSearchRefreshDrift` and Go shadow already replay the
same fields.
`scan:product-sovereignty` now requires `mcpSearchRefreshDrift`,
`mcp_refresh_catalog`, `catalogDrift`, `totalIndexed`, and boundary tokens.
```

Rejected claims:

```text
No runtime behavior change.
No Reasonix SessionAPI, MCP public protocol, config root, or public control
plane.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No credentialed MCP matrix or live MCP operations claim.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go MCP client, packaged MCP walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0231:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 785 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.178s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0230 Plan/Auto-Route State Reset G5 Shadow

Scope:

```text
D-0230 promotes cancelled Plan turn state reset and post-cancel auto-route
currentness into `controlExecutableCases.planCancelStateReset` and Go G5
executable shadow. The TypeScript loop remains authoritative for live
behavior; Go computes reset/no-leak, normal/auto tool hiding, auto reroute,
router isolation, current recommendation, stable-prefix cleanliness, and
product-boundary evidence from the TS-owned fixture without implementing a
live Go loop or default backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Plan reset/conformance | `npm --prefix packages/runtime test -- tests/loop.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 98 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.468s` |

Accepted claims:

```text
`loop.test.ts` proves a cancelled Plan turn is followed by a fixed normal turn
and a `model: "auto"` turn with no Plan `modeInstruction`, no inherited
`requiredToolName: create_plan`, and no `create_plan` advertisement.
The post-cancel auto turn invokes `_auto_router` once with no tools, no prefix,
and no Plan mode instruction, then accepts the current `deepseek-v4-pro` /
`max` recommendation.
`controlExecutableCases.planCancelStateReset` and Go shadow compute the same
reset/currentness and product-boundary outputs.
```

Rejected claims:

```text
No Reasonix SessionAPI, controller protocol, config root, public auto-plan
setting, or project override.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No dynamic classifier/planner/cancel state in the stable prefix or tool schema.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go loop, packaged Plan/auto-route walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0230:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 785 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.189s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0229 Plan Step/Cancel/Cache G5 Shadow

Scope:

```text
D-0229 promotes existing explicit Plan-mode step/cancel/cache behavior into
`controlExecutableCases.planStepCancelCache` and Go G5 executable shadow. The
TypeScript loop remains authoritative for live behavior; Go computes planner
gating, aborted follow-up cache-baseline preservation, retry reuse, cache
telemetry, and product-boundary evidence from the TS-owned fixture without
implementing a live Go loop or default backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Plan step/cancel/cache conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/loop.test.ts --no-file-parallelism --maxWorkers=1` | pass: 2 files / 97 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.172s` |

Accepted claims:

```text
`planStepCancelCache` records Plan mode, `plan-cache-cancel`, three requests,
aborted follow-up status, retry failure status, step-0 `create_plan` + `ls`,
`bash` exclusion, follow-up forced to `create_plan`, two usage events, DeepSeek
`chat_completions` `80/20` hit/miss telemetry, `prefixChanged: false`, empty
prefix-change reasons, and product-boundary booleans.
Go shadow computes step-0 read-only+plan, follow-up-only-plan, cancelled-step
cache-baseline preservation, next-plan retry baseline reuse, all-prefix-false,
cache telemetry preservation, forbidden-shell exclusion, and boundary outputs.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, controller protocol, config root, or public auto-plan
setting.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No dynamic planner/cache state in the stable prefix or tool schema.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go loop, packaged Plan walkthrough, G6 readiness,
or release readiness.
```

Final command gate for D-0229:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.471s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0228 MCP Call-Time Reconnect G5 Shadow

Scope:

```text
D-0228 promotes existing MCP tool-call reconnect classification into
`controlExecutableCases.mcpCallReconnect` and Go G5 executable shadow. The
TypeScript MCP provider remains authoritative for live behavior; Go computes
transport retry, protocol no-retry, attempt counts, close counts, success, and
error-code evidence from the TS-owned fixture without implementing a live Go
MCP client or default backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| MCP reconnect oracle/provider/conformance | `npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 35 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.424s` |

Accepted claims:

```text
`mcp-tool-lifecycle-oracle` records call-time reconnect classification:
transport-looking stale connection errors retry once, close the stale client,
and succeed on the second instance; deterministic MCP protocol validation
errors return `tool_execution_failed` without reconnecting.
`controlExecutableCases.mcpCallReconnect` and Go shadow compute retry/no-retry,
attempt counts, close counts, stale success, protocol error code, and product
boundary booleans.
```

Rejected claims:

```text
No live TypeScript MCP behavior change.
No Reasonix SessionAPI, MCP public protocol, config root, or public control
plane.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No credentialed MCP matrix or live MCP operations claim.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go MCP client, packaged MCP walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0228:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.246s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0227 AutoResearch Direction Tracking G5 Shadow

Scope:

```text
D-0227 promotes existing AutoResearch direction tracking into
`controlExecutableCases.autoResearch` and Go G5 executable shadow. The
TypeScript AutoResearch store and goal tool remain authoritative for live
behavior; Go computes direction file, iteration-log, tool-name, and
active-goal guard evidence from the TS-owned fixture without implementing a
live Go AutoResearch backend or default backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 AutoResearch direction conformance | `npm --prefix packages/runtime test -- go-runtime-conformance` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.478s` |

Accepted claims:

```text
`controlExecutableCases.autoResearch` records direction text, outcome, summary,
`record_research_direction`, active research-goal requirement, and expected
direction-tracking booleans.
The TypeScript conformance test creates project-local state, calls
`recordDirection`, reads `directions_tried.json`, reads `iteration_log.jsonl`,
verifies `direction_recorded`, keeps unknown requirement evidence rejected, and
keeps findings unchanged for that rejected write.
Go shadow computes direction file, iteration-log, tool-name, and active-goal
guard outputs.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, project protocol, config root, or public control plane.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No `REASONIX.md` or `AGENTS.md` AutoResearch state writes.
No stable-prefix or dynamic tool-schema research state pollution.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go AutoResearch backend, packaged long-task
walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0227:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.201s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0226 Renderer Settings Read Facade G5 Shadow

Scope:

```text
D-0226 promotes the D-0206 renderer settings-read facade seal into
`desktopSovereignty.rendererSettingsReadFacadeMatrix` and Go G5 executable
shadow. The TypeScript settings-read sources remain authoritative for live
behavior; Go computes keyboard shortcut, speech-to-text, usage model-label,
settings-changed event, direct bridge rejection, and scan guard evidence from
the TS-owned fixture without implementing a live Go desktop/settings bridge or
backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer settings-read facade conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.178s` |

Accepted claims:

```text
`desktopSovereignty.rendererSettingsReadFacadeMatrix` records the settings
client facade, forbidden direct bridge token, keyboard shortcut,
speech-to-text, and initial usage model-label readers, source-only settings
client proof, settings-changed event preservation, direct bridge rejection,
scan guard, and settings-read facade token scan proof.
Go shadow computes settings reader coverage, event-sync preservation, direct
bridge rejection, and scan guard presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No direct renderer `window.analytix.settings.getSettings` permission.
No Reasonix SessionAPI, settings/config public protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop/settings bridge, packaged settings
walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0226:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.181s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0225 Renderer Usage Runtime Client Facade G5 Shadow

Scope:

```text
D-0225 promotes the D-0205 renderer usage/debug runtime client facade seal into
`desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` and Go G5
executable shadow. The TypeScript usage hooks and settings diagnostics remain
authoritative for live behavior; Go computes thread/day/model usage coverage,
settings diagnostics coverage, direct bridge rejection, scan guard presence,
and unit-proof presence from the TS-owned fixture without implementing a live
Go desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer usage facade conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Renderer usage/settings unit proof | `npm run test -- src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts -- --runInBand` | pass: 4 files / 39 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.468s` |

Accepted claims:

```text
`desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` records the runtime
client facade, forbidden direct bridge token, thread/day/model usage loaders,
token economy and LLM debug diagnostics loaders, source-only runtime client
proof, direct bridge rejection, usage unit proof, scan guard, and usage facade
token scan proof.
Go shadow computes thread/day/model usage coverage, settings diagnostics
coverage, direct bridge rejection, scan guard presence, and unit-proof
presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No direct renderer `window.analytix.runtime.runtimeRequest` permission.
No Reasonix SessionAPI, usage/debug public protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged usage walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0225:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.172s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0224 Side Conversation Relation G5 Shadow

Scope:

```text
D-0224 promotes the D-0204 side conversation relation provider/store proof
into `desktopSovereignty` G5 executable shadow. The TypeScript side store
remains authoritative for live side promotion; Go computes optional provider
contract proof, provider promotion, refresh/close behavior, direct bridge
rejection, scan guard presence, and unit-proof presence from the TS-owned
fixture without implementing a live Go desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 side conversation relation conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Provider and side-store unit proof | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts -- --runInBand` | pass: 2 files / 37 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.375s` |

Accepted claims:

```text
`desktopSovereignty.sideConversationRelationContractMatrix` records provider
method `updateThreadRelation`, store action `promoteSideConversation`,
relation `primary`, optional provider contract proof, provider implementation
through runtime client, store provider-call proof, refresh/close proof, direct
bridge rejection, unit proof, and scan guard proof.
Go shadow computes optional provider contract proof, provider promotion,
refresh/close behavior, direct bridge rejection, scan guard presence, and
unit-proof presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No direct side-store `window.analytix.runtime.runtimeRequest` permission.
No Reasonix side/session public protocol, SessionAPI, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged side walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0224:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.413s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0223 Renderer Provider Facade Seal G5 Shadow

Scope:

```text
D-0223 promotes the D-0203 renderer provider facade seal proof into
`desktopSovereignty` G5 executable shadow. The TypeScript
`AnalytixRuntimeProvider` remains authoritative for live renderer provider
calls; Go computes runtime client use, direct bridge rejection,
archive/restore coverage, relation PATCH coverage, scan guard presence, and
unit-proof presence from the TS-owned fixture without implementing a live Go
desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer provider facade seal conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Renderer provider unit proof | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand` | pass: 1 file / 26 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.438s` |

Accepted claims:

```text
`desktopSovereignty.rendererProviderFacadeSealMatrix` records
`rendererRuntimeClient.runtimeRequest`, forbidden
`window.analytix.runtime.runtimeRequest`, sealed provider methods,
source-only runtime client proof, archive/restore source proof, relation PATCH
source proof, lifecycle unit proof, direct-bypass scan guard, and provider
facade token scan proof.
Go shadow computes runtime client use, direct bridge rejection,
archive/restore coverage, relation PATCH coverage, scan guard presence, and
unit-proof presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No direct renderer `window.analytix.runtime.runtimeRequest` permission.
No Reasonix SessionAPI, public frontend bridge protocol, config root, or
public control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged provider walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0223:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.259s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0221 Renderer Settings Bridge G5 Shadow

Scope:

```text
D-0221 promotes the D-0201 renderer settings bridge proof into
`desktopSovereignty` G5 executable shadow. The TypeScript renderer client
remains authoritative for live settings calls; Go computes analytix settings
API use, settings read cache, write refresh, top-level runtime patch
preservation, legacy alias unread proof, and unit-proof presence from the
TS-owned fixture without implementing a live Go desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer settings bridge conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Renderer runtime client unit proof | `npm run test -- src/renderer/src/agent/runtime-client.test.ts -- --runInBand` | pass: 1 file / 6 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.377s` |

Accepted claims:

```text
`desktopSovereignty.rendererSettingsBridgeMatrix` records top-level `runtime`
patch values, settings cache expectations, analytix settings API source proof,
cache/write refresh proof, unit proof, and legacy alias unread proof.
Go shadow computes analytix settings API use, settings read cache, write
refresh, top-level runtime patch preservation, legacy alias unread status, and
unit-proof presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No Reasonix settings protocol, config root, legacy agent settings envelope, or
public control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged settings walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0221:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.188s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0220 Renderer Runtime Client Bridge G5 Shadow

Scope:

```text
D-0220 promotes the D-0200 renderer runtime client bridge proof into
`desktopSovereignty` G5 executable shadow. The TypeScript renderer client
remains authoritative for live renderer calls; Go computes runtime request
argument preservation, runtime restart passthrough, SSE control/listener
passthrough, legacy alias unread proof, and unit-proof presence from the
TS-owned fixture without implementing a live Go desktop bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 renderer runtime client bridge conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.385s` |

Accepted claims:

```text
`desktopSovereignty.rendererRuntimeClientBridgeMatrix` records encoded runtime
request path variants, request argument counts, restart passthrough, SSE
start/stop calls, listener APIs, source passthrough proof, unit proof, and
legacy alias unread proof.
Go shadow computes renderer request argument preservation, restart passthrough,
SSE control/listener preservation, legacy alias unread status, and unit-proof
presence.
```

Rejected claims:

```text
No TypeScript renderer behavior change.
No Reasonix SessionAPI, public frontend bridge protocol, config root, or
public control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go desktop bridge, packaged renderer walkthrough,
G6 readiness, or release readiness.
```

Final command gate for D-0220:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.384s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0219 Main SSE Host URL Encoding G5 Shadow

Scope:

```text
D-0219 promotes the D-0199 main SSE host URL encoding proof into
`desktopMainIpcBoundary` G5 executable shadow. The TypeScript
`registerRuntimeSseIpc` bridge remains authoritative for live managed runtime
SSE calls; Go computes encoded thread/cursor preservation, header/stream-id
preservation, forbidden-route rejection, and unit-proof presence from the
TS-owned fixture without implementing a live Go SSE server or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 main SSE host encoding conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.439s` |

Accepted claims:

```text
`desktopMainIpcBoundary.mainSseHostEncodingMatrix` records dangerous source
thread id, encoded thread id, `/v1/threads/{id}/events`, `since_seq`,
`Last-Event-ID`, `Accept: text/event-stream`, bearer auth, stream id, error
payload, source proof, unit proof, and forbidden-route guard.
Go shadow computes encoded thread/cursor preservation, header/stream-id
preservation, forbidden-route rejection, and unit-proof presence.
Product-sovereignty scan now requires D-0219 main SSE host encoding G5 proof
tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public SSE/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go SSE server, live Go main bridge, packaged SSE
walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0219:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.413s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0218 Preload SSE Bridge G5 Shadow

Scope:

```text
D-0218 promotes the D-0198 preload SSE bridge proof into
`desktopSovereignty` G5 executable shadow. The TypeScript preload bridge remains
authoritative for live `window.analytix` SSE exposure; Go computes start/stop
argument preservation, payload-only listener delivery, listener cleanup, and
unit-proof presence from the TS-owned fixture without implementing a live Go
SSE server or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 preload SSE bridge conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.422s` |

Accepted claims:

```text
`desktopSovereignty.preloadSseBridgeMatrix` records runtime SSE start/stop/
event/end/error channels, thread id, cursor, stream ids, event/end/error
payloads, payload-only wrapper proof, listener cleanup proof, and analytix-only
bridge exposure proof.
Go shadow computes start/stop argument preservation, payload-only listener
delivery, cleanup symmetry, and unit-proof presence.
Product-sovereignty scan now requires D-0218 preload SSE bridge G5 proof tokens
and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public SSE/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go SSE server, live Go preload bridge, packaged
SSE walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0218:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.483s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0217 Runtime Host URL Handoff G5 Shadow

Scope:

```text
D-0217 promotes the D-0197 runtime host URL handoff proof into
`desktopMainIpcBoundary` G5 executable shadow. The TypeScript
`runtimeRequestViaHost` adapter remains authoritative for live managed runtime
HTTP calls; Go computes encoded path/query preservation, method/header/body
preservation, ensured settings host selection, and unit-proof presence from the
TS-owned fixture without implementing a live Go HTTP server or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 runtime host handoff conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.430s` |

Accepted claims:

```text
`desktopMainIpcBoundary.runtimeHostHandoffMatrix` records
`runtimeRequestViaHost`, `getRuntimeBaseUrlForSettings`, encoded thread/turn
ids, query preservation, POST body, bearer auth, custom proof header, JSON
content type, source proof, and ensureRuntime port-switch proof.
Go shadow computes encoded path/query preservation, method/header/body
preservation, ensured settings use, and unit-proof presence.
Product-sovereignty scan now requires D-0217 runtime host handoff G5 proof
tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, live Go main/preload bridge,
packaged route walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0217:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.401s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-23 - D-0216 Preload Runtime Request Bridge G5 Shadow

Scope:

```text
D-0216 promotes the D-0196 preload runtime request bridge proof into
`desktopSovereignty` G5 executable shadow. The TypeScript preload bridge remains
authoritative for live `window.analytix` exposure; Go computes runtime facade
path/method/body preservation, diagnostics same-channel behavior, analytix-only
bridge exposure, and unit-proof presence from the TS-owned fixture without
implementing a live preload bridge or backend.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| G5 preload runtime request bridge conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |
| Go shadow replay | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.460s` |

Accepted claims:

```text
`desktopSovereignty.preloadRuntimeRequestBridgeMatrix` records runtime and
diagnostics facade calls, encoded thread/turn/user-input ids, path/method/body
preservation, same-channel diagnostics behavior, and analytix-only bridge
exposure proof.
Go shadow computes runtime request path/method/body preservation, diagnostics
same-channel behavior, analytix-only exposure, and unit-proof presence.
Product-sovereignty scan now requires D-0216 preload runtime request bridge G5
proof tokens and final gate evidence.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI, public route/session protocol, config root, or public
control plane.
No Kun/DeepSeek public product identity.
No deprecated bridge alias or old runtime-shaped settings fallback.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, live Go preload bridge, packaged
route walkthrough, G6 readiness, or release readiness.
```

Final command gate for D-0216:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1536 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.514s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0199 Main SSE Host URL Encoding Proof

Scope:

```text
D-0199 closes the main SSE IPC URL-construction leg for dangerous thread ids.
`runtime:sse:start` encodes renderer-provided thread ids before fetching
managed runtime event routes and preserves cursor headers.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Main SSE host URL encoding proof | `npx vitest run src/main/runtime-sse-ipc.test.ts` | pass: 1 file / 6 tests |

Accepted claims:

```text
The main SSE IPC test proves slash/query/fragment thread ids are encoded into
`/v1/threads/{id}/events`, while `since_seq`, `Last-Event-ID`,
`Accept: text/event-stream`, bearer auth, and stream id error delivery are
preserved.
Product-sovereignty scan now requires D-0199 main SSE host URL encoding proof
tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public SSE protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go SSE server, packaged SSE walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0199:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1533 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.433s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0198 Preload SSE Bridge Proof

Scope:

```text
D-0198 adds executable preload SSE bridge evidence. The exposed
`window.analytix` runtime facade preserves `startSse` / `stopSse` arguments and
delivers SSE event/end/error payloads without exposing Electron event objects.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Preload SSE bridge proof | `npx vitest run src/preload/preload-sse-bridge.test.ts` | pass: 1 file / 2 tests |

Accepted claims:

```text
The preload SSE test mocks Electron, loads the exposed `analytix` API, and
proves `startSse` / `stopSse` invoke `runtime:sse:*` with arguments unchanged.
Event, end, and error wrappers forward payloads only and remove listeners on
unsubscribe.
Product-sovereignty scan now requires D-0198 preload SSE bridge proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public SSE protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go SSE server, packaged SSE walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0198:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 212 files / 1532 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.422s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0197 Runtime Host URL Handoff Proof

Scope:

```text
D-0197 closes the main runtime adapter to managed runtime HTTP host leg.
`runtimeRequestViaHost` preserves encoded shared endpoint paths, query values,
method, auth, custom header, content type, and body.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime host URL handoff proof | `npx vitest run src/main/runtime/analytix-adapter.test.ts` | pass: 1 file / 3 tests |

Accepted claims:

```text
The runtime adapter test uses a real local HTTP server and proves encoded
shared thread/turn route ids plus encoded query values arrive unchanged at the
runtime host. POST method, bearer auth, custom header, JSON content type, and
body are preserved.
Product-sovereignty scan now requires D-0197 runtime host URL handoff proof
tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0197:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 211 files / 1530 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.419s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0196 Preload Runtime Request Bridge Proof

Scope:

```text
D-0196 closes the preload leg of the runtime request chain. The exposed
`window.analytix` runtime facade passes encoded shared endpoint paths, method,
and body into the main `runtime:request` IPC channel unchanged.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Preload runtime request bridge proof | `npx vitest run src/preload/preload-runtime-request.test.ts` | pass: 1 file / 2 tests |

Accepted claims:

```text
The preload test mocks Electron, loads the exposed `analytix` API, and proves
`api.runtime.runtimeRequest` invokes `runtime:request` with encoded shared
endpoint path, method, and body unchanged. Diagnostics runtime request
compatibility uses the same analytix IPC channel.
Product-sovereignty scan now requires D-0196 preload runtime request bridge
proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No deprecated bridge alias.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0196:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 211 files / 1529 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.400s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0195 Main IPC Runtime Adapter Handoff Proof

Scope:

```text
D-0195 closes the registered handler handoff after D-0194. The main
`runtime:request` handler forwards encoded shared endpoint builder paths to the
runtime adapter unchanged and blocks invalid paths before adapter invocation.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Main IPC runtime adapter handoff proof | `npx vitest run src/main/ipc/register-app-ipc-handlers.test.ts` | pass: 1 file / 16 tests; Node `punycode` deprecation warning only |

Accepted claims:

```text
Encoded shared builder paths for thread, checkpoint, approval, user-input,
session, attachment, and memory routes are passed to `runtimeRequest`
unchanged, with method and body preserved. Raw extra-segment paths and singular
`/v1/user-input/:id` reject before the runtime adapter is called.
Product-sovereignty scan now requires D-0195 main IPC runtime adapter handoff
proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0195:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass after rerun: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 210 files / 1527 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.474s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Runtime package note:

```text
The first full runtime package run hit a transient-looking unrelated planner
executor failure (`planner executor job disappeared: parallel_task_coord_9`).
The failing file rerun passed 1 file / 8 tests, and the full runtime package
suite rerun passed 80 files / 784 tests.
```

## 2026-06-22 - D-0194 Main IPC Endpoint Builder Allow-list Proof

Scope:

```text
D-0194 closes the endpoint builder proof chain at the main IPC boundary.
`runtimeRequestPayloadSchema` accepts encoded shared endpoint builder output
and rejects raw extra-segment or singular compatibility drift.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Main IPC endpoint builder allow-list proof | `npx vitest run src/main/ipc/app-ipc-schemas.test.ts` | pass: 1 file / 42 tests |

Accepted claims:

```text
Main IPC accepts encoded shared builder output for thread, turn, checkpoint,
approval, user-input, session, attachment, and memory paths. Raw unencoded ids
that create extra route segments are rejected, and singular `/v1/user-input/:id`
remains excluded from the main IPC shared allow-list.
Product-sovereignty scan now requires D-0194 main IPC endpoint builder
allow-list proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0194:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 210 files / 1525 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.412s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0193 Renderer Runtime Endpoint Builder Proof

Scope:

```text
D-0193 extends shared endpoint proof into renderer `AnalytixRuntimeProvider`.
GUI runtime root calls use shared endpoint constants, and dynamic route ids are
encoded before crossing the `window.analytix.runtime.runtimeRequest` bridge.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Renderer runtime endpoint builder proof | `npm run test -- src/renderer/src/agent/analytix-runtime.test.ts` | pass: 1 file / 26 tests |

Accepted claims:

```text
Renderer runtime provider root calls use `ANALYTIX_HEALTH_PATH` and
`ANALYTIX_THREADS_PATH`. Tests prove slash/query/fragment text in thread, turn,
approval, user-input, and session ids is encoded before bridge `runtimeRequest`
calls, while paths remain analytix-owned `/health` or `/v1/*`.
Product-sovereignty scan now requires D-0193 renderer runtime endpoint builder
proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0193:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 210 files / 1523 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.402s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0192 Shared Endpoint Builder Sovereignty Proof

Scope:

```text
D-0192 adds shared endpoint builder evidence for route-id URL encoding,
canonical plural user-input templates, and exported endpoint forbidden-token
absence. It does not change runtime behavior or expose new routes.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Shared endpoint builder sovereignty | `npm run test -- src/shared/analytix-endpoints.test.ts` | pass: 1 file / 2 tests |

Accepted claims:

```text
Shared analytix endpoint builders now have unit evidence that route ids are
URL-encoded for thread, turn, checkpoint, approval, user-input, session,
attachment, and memory paths. Exported shared endpoint strings remain
analytix-owned, canonical user-input stays plural `/v1/user-inputs/{id}`, and
Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer/session-api tokens are absent.
Product-sovereignty scan now requires D-0192 shared endpoint builder
sovereignty proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0192:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 210 files / 1522 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.181s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0191 Runtime Forbidden Route Dispatch Proof

Scope:

```text
D-0191 adds actual TypeScript HTTP dispatch evidence for every forbidden route
token listed in `controlExecutableCases.runtimeHttpRouteSovereignty`.
Each token is requested with valid auth and must return structured not_found.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime forbidden route dispatch conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 11 tests |

Accepted claims:

```text
The active TypeScript HTTP router now has fixture-driven forbidden route
dispatch coverage for Reasonix/Kun/Go/Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer public route tokens. With valid auth, each token
returns `{ code: "not_found", message: "route not found" }`, proving forbidden
route families are absent rather than hidden authenticated stubs.
Product-sovereignty scan now requires D-0191 runtime HTTP forbidden route
dispatch proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0191:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 784 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.397s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0190 Runtime HTTP Auth Matrix Proof

Scope:

```text
D-0190 adds actual TypeScript HTTP dispatch evidence for every route listed in
`controlExecutableCases.runtimeHttpRouteSovereignty.routes`. It proves
`/health` remains public while all 43 `/v1/*` routes return canonical
structured 401 without auth.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime HTTP auth matrix conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 10 tests |

Accepted claims:

```text
The active TypeScript HTTP router now has fixture-driven unauthenticated
dispatch coverage for every registered D-0189 route. SSE, task-job, approval,
user-input, and resume-thread route keys are explicitly included in the 401
matrix, while `/health` remains the only public 200 route.
Product-sovereignty scan now requires D-0190 runtime HTTP auth matrix proof
tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0190:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 783 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.421s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## 2026-06-22 - D-0189 Runtime HTTP Route Sovereignty Shadow

Scope:

```text
D-0189 adds source-derived G5 executable proof for the active `analytix serve`
HTTP/SSE route table. It proves routeCountExact, onlyHealthUnauthenticated,
allRuntimeRoutesAnalytixOwned, sseRouteAnalytixThreadEvents,
threadLifecycleRoutesPresent, approvalUserInputRoutesPresent,
taskJobRoutesInternalOnly, notFoundStructured, noForbiddenRoutes, and
singularUserInputCompatibilityOnly without changing active runtime behavior or
Go backend status.
```

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Runtime route sovereignty conformance | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 1 file / 9 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.402s` |

Accepted claims:

```text
G5 runtime HTTP route sovereignty now computes exact 44-route inventory,
`/health` as the only unauthenticated route, authenticated `/v1/*` ownership,
`/v1/threads/:id/events` SSE ownership, thread lifecycle and
approval/user-input route presence, internal task-job route presence,
compatibility-only singular user-input route status, structured not-found
evidence, and forbidden-route-token absence from source-derived fixture inputs.
Product-sovereignty scan now requires D-0189 runtime HTTP route sovereignty
proof tokens.
```

Rejected claims:

```text
No TypeScript runtime behavior change.
No Reasonix SessionAPI or public route protocol.
No Kun/DeepSeek public product identity.
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
No renderer-visible Go route or default Go backend.
No Rust/Tauri migration, live Go HTTP server, packaged route walkthrough, G6
readiness, or release readiness.
```

Final command gate for D-0189:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 782 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.188s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

Final command gate for D-0169:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 778 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.197s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Request-Shape Derived Replay Addendum

Scope:
Provider request-shape replay now derives URL/header/body/tool-shape matches
from fixture inputs in both Go G3 and Go G5 shadow output. This strengthens the
Base URL / Endpoint format evidence for DeepSeek, OpenAI-compatible chat,
OpenAI Responses, Anthropic Messages, and custom full `/responses`,
`/messages`, and `/chat/completions` endpoints. It does not change active
provider runtime behavior.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Request-shape derived replay | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 19 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: focused `ok github.com/analytix/runtime-go 0.217s`; final `0.245s` |

Accepted claims:

```text
Go G3/G5 shadow derives provider request-shape URL/header/body/tool matches
from TS-owned fixtures.
Custom full endpoint cases preserve exact URLs, and custom appended-path count
is 0.
Provider request-shape evidence covers DeepSeek chat, OpenAI-compatible chat,
OpenAI Responses, Anthropic Messages, and custom full endpoints.
```

Rejected claims:

```text
No provider runtime behavior change.
No live Go provider client.
No credentialed provider/cache matrix or live superiority claim.
No Reasonix provider public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, G6 readiness, or release readiness.
```

Final command gate for D-0170:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 778 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.245s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Post-881 Runtime Proof Freshness V2 Addendum

Scope:
The reusable product-sovereignty scan now guards key post-881 runtime proof
leaves and positive proof tokens. This is a regression guard only: it does not
change runtime behavior, enable Go, or expose new product routes.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Product sovereignty proof freshness | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |
| Post-881 proof leaves | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/task-job-orchestration-oracle.test.ts tests/mcp-tool-lifecycle-oracle.test.ts tests/create-plan-tool.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files / 44 tests |

Accepted claims:

```text
Product-sovereignty scan now requires task-job and MCP lifecycle fixture/test
leaves in runtime proof freshness.
Product-sovereignty scan now checks positive post-881 provider, planner/task,
and MCP proof tokens.
```

Rejected claims:

```text
No runtime behavior change.
No live Go provider/MCP/job client.
No default Go backend or renderer-visible Go route.
No Reasonix public protocol or config shape.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
QA, G6 readiness, or release readiness.
```

Final command gate for D-0171:

| Gate | Command | Result |
| --- | --- | --- |
| Whitespace | `git diff --check` | pass |
| Runtime package tests | `npm --prefix packages/runtime test` | pass: 80 files / 778 tests |
| Workspace tests | `npm run test` | pass: 209 files / 1520 tests; Node `punycode` deprecation warning only |
| Workspace typecheck | `npm run typecheck` | pass |
| Runtime package build | `npm run build:runtime` | pass |
| Go shadow tests, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.410s` |
| Product sovereignty scan | `npm run scan:product-sovereignty` | pass: `[scan:product-sovereignty] pass` |

## Provider Cache Accounting Raw Payload Replay Addendum

Scope:
Provider cache accounting now parses raw provider `responseBody.usage` payloads
before computing cache telemetry. G3/G5 fixtures carry raw usage payloads, TS
conformance derives expected accounting from those payloads, and Go shadow
computes the same fields from raw maps. This strengthens DeepSeek/OpenAI
Responses/Anthropic accounting plus custom request-shape non-regression proof
without changing active provider runtime behavior.

Focused evidence:

| Gate | Command | Result |
| --- | --- | --- |
| Raw provider accounting replay | `npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1` | pass: 3 files / 19 tests |
| Go shadow replay, non-cached | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` | pass: `ok github.com/analytix/runtime-go 0.419s` |

Accepted claims:

```text
Go G3/G5 shadow derives provider cache accounting from raw provider usage
payload fixtures.
Raw replay covers DeepSeek prompt/native cache telemetry, OpenAI Responses
cached tokens, Anthropic read/creation cache fields, and unsupported
OpenAI-compatible cache telemetry.
Unsupported provider cache telemetry remains unknown and is not promoted to a
miss-count claim.
```

Rejected claims:

```text
No provider runtime behavior change.
No live Go provider client.
No credentialed provider/cache matrix or live superiority claim.
No Reasonix provider public protocol or config shape.
No default Go backend or renderer-visible Go route.
No Kun identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri migration, packaged
provider QA, G6 readiness, or release readiness.
```
