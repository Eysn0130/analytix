# analytix code-level upstream implementation plan

Status: Historical execution plan.
Snapshot window: 2026-06-20 through 2026-06-30. This file preserves completed
and superseded stage reasoning; it is not the current construction authority.
For new work, use `upstream-capability-audit-2026-07-10.md`, the current
construction packets in `absorption-targets.md`, the pinned
`upstream-sources.json` manifest, and an approved scoped OpenSpec change.

This is the execution plan for turning the upstream absorption strategy into
implementation work in follow-up threads.

This plan is capability-first. `Main upstream` or stage labels identify where a
past batch expected to look first; they do not create exclusive source
territories. Future implementation batches may compare all relevant upstreams
and choose the strongest proven design.

Companion documents:

```text
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
docs/analytix/upstreams/absorption-targets.md
docs/analytix/upstreams/code-level-absorption-blueprint.md
docs/analytix/upstreams/go-runtime-conformance.md
docs/analytix/benchmarks/quality-gates.md
```

## 1. Historical Upstream Checkpoint

Checked on 2026-06-20. The word "current" inside this section is preserved as
part of that dated record and does not describe the 2026-07-10 source state.

| Upstream | Branch | Currentness checkpoint | Notes |
| --- | --- | --- | --- |
| `esengine/DeepSeek-Reasonix` | `main-v2` | requested checkpoint `c202f970`; historical 2026-06-20 observed remote `5d1ad2a`; current 2026-06-21 recheck `49c14762` | `bc8249c3` is the post-4f515da goal/control implementation baseline. `3e625b91` / merge `48e5b990` is scoped-absorbed as analytix `RemoteEntryControlPort`; `1280c0f` / `9ae96d8` / `c202f970`, `dbb6f83a` / historical `5d1ad2a`, and current `49c14762` SessionAPI/App/cache-relevant drift are currentness records until a behavior delta is classified. |
| `KunAgent/Kun` | `master` | `8602476` | Stable master now includes the previous `ab24a77` develop line via merge #444. The drift from `8f20403` contains Workflow/Create Loop, Telegram Connect, provider routing and endpoint fixes, preload sandbox fixes, git checkpoint, tray session menu, worktree/project grouping, composer/sidebar fixes, and v0.2.15 docs. |
| `KunAgent/Kun` | `develop` | `247076f` | Develop has advanced past the requested `9605e20f` checkpoint. `d09d52b0` is implemented in this P0 batch as analytix-native NSIS process-stop logic; newer speech/SSE/UI drift remains classified-only until separate batches. |

Before any new implementation batch starts, run `npm run audit:upstreams`,
consult the all-source audit, and update the affected source ledger plus
`code-reuse-provenance.md`. The paragraphs below retain the old two-source
workflow as history. The post-4f515da currentness pass on
2026-06-20 confirmed Kun `master` at `8602476`, Kun `develop` at
`247076f`, Kun `v0.2.13` / `v0.2.14` tags at `201a146` / `06be05d`, and
Reasonix requested `main-v2` checkpoint `c202f970`, the historical 2026-06-20
observed remote `5d1ad2a`, and the current 2026-06-21 recheck `49c14762`. The active analytix
branch baseline now includes the already-absorbed
Reasonix `bc8249c3` goal/control slice, the scoped `3e625b91` / `48e5b990`
remote-entry control-port slice, the P0 product-entry correction, and the
Kun `d09d52b0` installer process-stop absorption.

Current runtime status as of 2026-06-25:

```text
Go runtime default delivery is active through go-runtime-default.
ANALYTIX_RUNTIME_BACKEND=typescript is retired and rejected by the current
adapter. Older plan rows that describe Go as scaffold-only, TypeScript as the
default, explicit rollback, or live D-0251/D-0252/D-0253 evidence as a
default-startup blocker are historical stage records. Current next work is
post-cutover live validation, scorecard/release-strength closure, and
continuing Kun 0.2.14/current absorption without changing the product-entry
boundary.
```

## 2. /goal Usage Decision

Default rule: do not start a `/goal` for documentation-only audit threads.
Exception: when the user explicitly requests `/goal` for a concrete staged
implementation batch, start one goal for that stage only.

P3A exception used in this branch:

```text
/goal P3A: Kun baseline fidelity audit + Reasonix Tool/MCP/Sandbox/Checkpoint boundary code slice.
```

Use `/goal` in the new implementation thread when the work enters a concrete
stage that may span multiple turns. Recommended objectives:

```text
/goal Complete P0 upstream absorption baseline and fixture inventory for analytix.
/goal Complete P1 Kun delta closure without regressing analytix UI/contracts.
/goal Complete P2 Reasonix provider/cache/session-sidecar absorption with tests.
/goal Complete post-cutover Go runtime live validation and D-0253 fallback-retirement evidence for analytix.
```

One `/goal` should cover one stage only. Do not use one giant goal for all
stages; it will make blocking conditions and evidence too blurry.

## 3. Non-Negotiable Implementation Rules

```text
1. analytix is the product owner.
2. Public runtime CLI remains analytix serve.
3. Renderer uses window.analytix only.
4. Active settings remain top-level runtime.
5. Renderer must not know TypeScript vs Go backend.
6. Upstream code lands only through analytix contracts.
7. Tests or fixtures should be ported before large implementation code.
8. Benchmark/scorecard evidence is required before "stronger than upstream" claims.
9. No current-product Kun or Reasonix identity may leak into UI, package, bridge, or runtime contract.
```

## 4. Historical Stage Overview

| Stage | Purpose | Default inputs | Output |
| --- | --- | --- | --- |
| P0 | Baseline, exact diff inventory, fixture plan. | Kun / Reasonix | Updated ledgers, failing fixtures where useful, no behavior changes. |
| P1.1 | Close Kun stable critical baseline/delta fixes from master `8602476`. | Kun | Provider endpoints/models, per-provider runtime routing, preload sandbox guard, composer/worktree/sidebar/tray stable fixes. |
| P1.2 | Harden Connect Phone / Telegram regression and delta behavior. | Kun + analytix | Telegram runtime/settings plus Connect Phone thread, provider, approval, and streaming QA through analytix naming. |
| P1.3 | Redesign Workflow / Create Loop as an analytix-native scoped slice. | Kun plus all relevant product/agent references | Keep Create Loop internals hidden unless a future analytix-native spec approves product entry placement; no top-level Workflow route is implied by historical Kun baseline parity. |
| P2.2 | Absorb and prove Reasonix provider/cache engine behavior. | Reasonix | Provider/cache/cost/stream parsing fixtures beyond the completed session-sidecar work; first fixture-backed proof now lives in `packages/runtime/tests/provider-cache-proof.test.ts`. |
| P3 | Strengthen tools, MCP/plugin, sandbox, approvals. | Reasonix + Kun | More reliable tools without weakening desktop control; Kun side is checkpoint/product-safety delta review. |
| P4A | Add checkpoint/rewind safety oracle and minimal code slice. | Reasonix engine + Kun product-safety evidence | Analytix-owned `axcp_` metadata, `checkpoint_captured` event projection, code-only changed-file oracle, and conversation-only rewind plan. |
| G0-G6 | Preserve Go default through conformance, release rollback, and retirement gates. | Reasonix + Kun oracle | Go runtime default is active; `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic and must not launch TS runtime. |
| P8 | Release strength claim gate. | analytix evidence | Scorecard-backed claim that analytix is stronger in verified areas. |

## 4.1 Architecture/Spec V1 Freeze Decision

The v1 architecture/spec baseline is frozen enough to start the next
implementation batch when these documents agree:

```text
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
docs/analytix/upstreams/kun-sync.md
docs/analytix/upstreams/reasonix-sync.md
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/code-level-absorption-blueprint.md
docs/analytix/upstreams/code-level-implementation-plan.md
```

Frozen means future upstream absorption can start from this checkpoint and must
update the ledgers when upstream HEAD changes. It does not mean release closure,
future source-specific refactors, or Go runtime parity are complete.

## 5. P0 Baseline And Inventory

Goal:

```text
Freeze exact upstream inputs and decide the first implementation batch.
```

Tasks:

1. Refresh upstream commits with `git ls-remote` or local fetch.
2. Update `kun-sync.md` and `reasonix-sync.md`.
3. Compare current analytix files against selected upstream directories.
4. Choose one batch only.
5. Copy or write failing tests/fixtures before implementation when possible.

Acceptance:

```text
git diff --check passes
sync ledger records exact commits
absorption-ledger records chosen batch
quality gate lists required tests and benchmark dimensions
```

Stop conditions:

```text
upstream commit changed mid-batch
diff scope is too broad for one review
target behavior cannot be proven by tests or QA
```

## 6. P1 Kun Stable Master Delta Closure

Why first:

```text
analytix started from Kun 0.2.13. Before deep Reasonix engine work or Go
runtime replacement, analytix must preserve 0.2.13 baseline fidelity and repair
any upgrade regressions in Code, Write, SDD, Connect Phone, Schedule, provider
presets, MCP/Skills, multimodal/media, runtime workflow, and GUI command
details. Kun master `8602476` moved the previous develop line into stable
master, so follow-up Kun work is 0.2.14 / `8602476` delta closure rather than a
speculative develop merge.
```

Primary source areas:

```text
kun/src/contracts
kun/src/server/routes
kun/src/services
kun/src/loop
kun/src/adapters/model
kun/src/adapters/tool
src/renderer/src/components/write
src/renderer/src/write
src/renderer/src/components/sdd
src/renderer/src/sdd
src/renderer/src/components/schedule
Connect Phone / claw-related flows
docs/WRITE_*
docs/model-provider-presets.md
```

P1 implementation order:

1. P1.1 Kun stable critical fixes:
   provider endpoint/model fixes, per-provider runtime routing, preload sandbox
   startup guard, composer/worktree/sidebar/tray stable desktop fixes, and
   context-compaction repair fixtures.
2. P1.2 Connect Phone / Telegram:
   Telegram runtime/settings and Connect Phone streaming, approval, provider,
   and schedule integration through analytix naming.
3. P1.3 Workflow / Create Loop analytix redesign:
   product/UX proposal, analytix contract shape, workflow runtime fixtures,
   then a controlled implementation. Do not direct-port the Kun UI shell.
4. Write and SDD workflow parity that remains outside the stable critical
   fixes.
5. Desktop QA for upgraded analytix UI.

Kun master `8602476` stable drift:

| Area | Decision |
| --- | --- |
| GLM custom full endpoints and Xiaomi deprecated model removal | `must absorb` in P1.1. |
| Per-provider runtime routing with `thread.providerId` | `must absorb` in P1.1; current analytix has provider ids in UI/settings but not equivalent runtime client routing. |
| Preload sandbox fix | `already covered` by current preload avoiding node builtins, but P1.1 should add/keep a sandbox startup guard. |
| Composer/worktree/sidebar/tray stable fixes | `should absorb` in P1.1 through current analytix UI patterns. |
| Telegram Connect runtime | `should absorb` in P1.2. |
| Workflow builder / Create Loop | `redesign`; high-value, but too large for direct UI merge. |
| Git checkpoint service | `should absorb` in P3 with analytix naming and review/history integration. |

Acceptance:

```text
npm run typecheck
relevant runtime and renderer tests
route parity tests for session/thread/fork/archive/search/replay/usage
Write benchmark passes
SDD benchmark passes
Schedule and Connect Phone smoke passes
no current-product Kun identity regression
```

## 7. P2 Reasonix Low-Risk Engine Wins

Why second:

```text
These changes can improve analytix engine quality without replacing the whole
runtime or disturbing the renderer.
```

Primary source areas:

```text
internal/provider/*
internal/agent/cache_shape.go
internal/provider/schema_canonicalize.go
internal/plugin/canonicalize.go
internal/agent/branch.go
internal/agent/save.go
internal/control/controller.go
internal/agent/listsessions_sidecar_test.go
internal/agent/listsessions_bench_test.go
```

Must-absorb latest Reasonix delta:

| Reasonix commit | Behavior | Analytix reason |
| --- | --- | --- |
| `249a4f8` | Cache session turn count and preview in `.meta` sidecar; list sessions without decoding whole `.jsonl`. | Directly improves long-history sidebar/session picker performance. |
| `73e2025` | Seed fork/branch sidecar counts, warn on goal-state persistence failures, remove dead copy condition. | Improves fork/branch consistency and persistence observability. |
| `5db1d0b` | Remove redundant project-session disk cache after sidecar-only reads make it unnecessary. | Simplifies consistency and prevents active-session changes from invalidating directory cache. |
| `7ebb08e` | Version sidecar counts instead of using `Turns == 0` as "missing". | Prevents empty/zero-count authoritative summaries from being re-decoded forever and lets legacy sidecars backfill once. |
| `341f720` | Write goal state off the Reasonix controller lock. | No equivalent TypeScript controller lock exists today; the requirement must be preserved for future Go G4/G5 runtime work. |

Out-of-scope for this first P2 session-sidecar slice:

| Reasonix commit | Behavior | Analytix reason |
| --- | --- | --- |
| `f6ba755` | Move memory-write disk I/O off the Reasonix controller lock. | Valuable controller responsiveness work, but it touches memory/control locking rather than session history listing and needs separate analytix fixtures. |
| `98a57ded` / `d02457ee` | Move Reasonix session sidecar path derivation into `internal/store`. | Behavior is byte-identical upstream and analytix already centralizes session metadata behind its TypeScript hybrid/session stores. Record as future Go store-boundary guidance, not current TS runtime work. |
| `3e625b91` / `48e5b990` | Introduce a Reasonix `SessionAPI` driving port, split `Lifecycle`, `TurnControl`, and `Approvals`, and migrate the bot gateway to the narrower interface. | Scoped-absorbed as analytix-owned `RemoteEntryControlPort` and oracle tests. This remains out of the P2 session-sidecar slice, but no longer needs to be described as a future control-port batch. |

Completed P2 session-sidecar and first cache/provider proof scope:

```text
Reasonix 249a4f8 / 73e2025 / 5db1d0b / 7ebb08e are absorbed for the
session-sidecar surface. 341f720 is recorded as a future Go G4/G5 conformance
requirement. P2.2 cache/provider fixture proof now covers stable prefix hash,
canonical tool schema hash, provider/model/endpoint attribution, DeepSeek /
Responses / Anthropic cache parsing, and unsupported-provider no-guess fallback.
Do not claim live Reasonix cache superiority until credentialed provider matrix
evidence exists.
```

P2.2 implementation order:

1. Add provider request/stream/usage/cache fixtures. First fixture proof is
   complete in `provider-cache-proof.test.ts`; keep extending it before code
   ports.
2. Port Reasonix provider/cache improvements through analytix shared provider
   contracts.
3. Update benchmark scorecard for provider/cache/cost behavior.
4. Keep renderer/backend neutrality and top-level `runtime` settings.
5. Run live provider matrix only when credentials are available; otherwise
   record it as an explicit blocker.

Acceptance:

```text
session list stays O(number of sessions metadata reads) after first legacy backfill
fork/branch sessions appear without full transcript decode
goal-state persistence failures are visible in sanitized logs
provider URL/body/header/stream/usage fixtures pass
provider-cache proof covers unsupported cache telemetry without guessing misses
git diff --check passes
```

## 8. P3 Tools, MCP, Plugin, Sandbox, Approvals

Primary source areas:

```text
Reasonix internal/plugin
Reasonix internal/tool and internal/tool/builtin
Reasonix internal/permission
Reasonix internal/sandbox
Kun kun/src/adapters/tool
Kun kun/src/skills
```

Implementation order:

1. Tool lifecycle fixtures.
2. Plugin stdio/HTTP cancellation and malformed plugin tests.
3. Approval and denied-tool tests.
4. Sandbox/protected-path tests.
5. File/image result propagation tests.

P3A scoped slice:

```text
P3A remains on the TypeScript runtime and Electron/React desktop. It does not
start a full workflow builder, full checkpoint/rewind, Go scaffold, or Rust
helper. The accepted code slice is: normalize malformed MCP input schemas before
they enter the analytix tool registry/model tool catalog, and add focused
denied-approval safety proof that a refused GUI approval never executes the
tool function. These tests become Go G4 oracle fixtures later.
```

Acceptance:

```text
denied tools never run
cancelled tools stop cleanly
MCP/plugin tool schemas remain cache-stable
file/image results reach runtime event log and renderer transcript
desktop approval UI remains analytix-owned
```

## 9. P4 Checkpoint And Rewind

Primary source areas:

```text
Reasonix internal/checkpoint
Reasonix docs/CHECKPOINTS.md
Reasonix internal/control rewind/fork paths
analytix review/generated-files/history UI
```

Implementation order:

1. P4A adds checkpoint fixtures for code-only metadata and conversation-only
   rewind without mutating files or rewriting user data.
2. P4A adds event-log replay/projection proof through an append-only
   analytix-owned `checkpoint_captured` event.
3. P4B adds combined code+conversation rewind as an auditable `plan_only`
   contract, plus runtime route/service and existing review/history UI display.
4. P4C adds destructive restore/apply, crash-recovery storage, generated-files
   confirmation, and desktop QA for interrupted turn and resumed session.

P4A implemented slice:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/contracts/events.ts
packages/runtime/src/domain/checkpoint-oracle.ts
packages/runtime/src/domain/runtime-event-reducer.ts
packages/runtime/tests/checkpoint-rewind-oracle.test.ts
```

P4B implemented slice:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/domain/checkpoint-oracle.ts
packages/runtime/src/services/checkpoint-rewind-service.ts
packages/runtime/src/server/routes/checkpoints.ts
src/shared/analytix-endpoints.ts
src/main/ipc/app-ipc-schemas.ts
src/renderer/src/agent/analytix-runtime.ts
src/renderer/src/components/ChangeInspector.tsx
src/renderer/src/components/chat/message-timeline-cards.tsx
packages/runtime/tests/checkpoint-rewind-plan.test.ts
```

P4C implemented slice:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/contracts/events.ts
packages/runtime/src/services/checkpoint-rewind-service.ts
packages/runtime/src/server/routes/checkpoints.ts
packages/runtime/src/server/routes/index.ts
src/shared/analytix-endpoints.ts
src/main/ipc/app-ipc-schemas.ts
src/renderer/src/agent/analytix-contract.ts
src/renderer/src/agent/analytix-runtime.ts
src/renderer/src/components/RewindPlanApplyControls.tsx
src/renderer/src/components/ChangeInspector.tsx
src/renderer/src/components/chat/message-timeline-cards.tsx
packages/runtime/tests/checkpoint-rewind-apply.test.ts
```

P4A deliberately does not use git. The oracle stores checkpoint state in
analytix-owned runtime metadata/events (`axcp_` ids and `checkpoint_captured`)
so replay, crash recovery, and future review UI can be proven without
introducing destructive restore behavior. P4B keeps that boundary and adds
analytix-owned `axrp_` rewind plans with `applyMode: plan_only` and
`destructive: false`; it blocks path escape, absolute persisted paths, and
symlink risks before any apply route exists. If P4C uses git or snapshots, refs
and storage must be analytix-named and must not use Kun refs.

P4C keeps the P4B plan as the only apply authority. The destructive route
requires explicit confirmation, rejects stale/tampered plans, records an
analytix-owned `axrr_` rescue event before file mutation, and writes an
append-only `checkpoint_rewind_applied` audit event with `axra_` apply ids.
File apply supports created-file delete, modified-file restore, deleted-file
restore, noop, manual-review, and blocked states. Modified/deleted content
restore requires matching snapshot evidence in the apply contract; missing
snapshot/hash evidence or mismatched snapshot content hashes are blocked instead
of inferred. Parent/final symlinks and lock-time file/git drift are blocked;
partial mutation failures must preserve already-applied file results in the
audit. Conversation restore is append-only audit and does not rewrite durable
transcript files. Automatic
production snapshot capture/storage remains a follow-up enhancement unless a
future capture path can prove privacy and recovery boundaries without leaking
full file contents into normal chat/tool output.

Acceptance:

```text
P4A: checkpoint metadata is versioned, deterministic, and analytix-owned
P4A/P4B: changed-file paths cannot escape the workspace; legal `..name` files are not over-rejected
P4A: conversation-only rewind can be planned from event replay without mutating events.jsonl
P4B: code-only restore plan is metadata-only, auditable, and non-destructive
P4B: combined code+conversation plan is returned through analytix route/IPC/provider contracts
P4B: review/history/ChangeInspector display shows plan status without fake diffs or apply controls
P4C: destructive restore runs only after explicit confirmation and P4B plan validation
P4C: destructive file restore creates an analytix-owned rescue record before mutation
P4C: created/modified/deleted/noop/manual_review/blocked restore outcomes are tested
P4C: path escape, absolute paths, parent/final symlinks, staged/untracked conflicts, snapshot content/hash mismatch, and missing snapshot/hash evidence are blocked
P4C: mutation-time revalidation and partial-failure audit preserve real applied/failed outcomes
P4C: conversation restore is append-only audit and preserves transcript/tool history
P4C: desktop smoke proves startup/render/runtime health; live apply UI fixture remains a release gate
```

## 10. P5 Go Runtime Post-Cutover Validation

Current status as of 2026-06-25:

```text
Go runtime default delivery is active through go-runtime-default.
ANALYTIX_RUNTIME_BACKEND=typescript is retired and rejected by the current
adapter. D-0251/D-0252/D-0253 live provider, MCP, packaged, operator, and soak
evidence is post-cutover validation and authorization work, not a requirement
to redo the default switch.
```

Primary source areas:

```text
Reasonix internal/control
Reasonix internal/serve
Reasonix internal/provider
docs/analytix/upstreams/go-runtime-conformance.md
```

Current implementation order:

1. Run post-cutover D-0251 live validation for credentialed provider families,
   credentialed MCP execution, packaged desktop QA, and operator evidence when
   real inputs are available.
2. Keep deterministic Go-default contract equivalence green: adapter, runtime
   routes, attachments, memory, approvals, user-input, SSE replay, and provider
   boundary tests.
3. Prepare D-0252 candidate/soak and retired-backend deletion authorization
   evidence without treating TypeScript as a current startup path.
4. Keep backend selection inside Electron main/runtime supervisor and hidden
   from renderer/preload/settings product surfaces.

Acceptance:

```text
renderer and preload unchanged
analytix serve remains public CLI
Go default remains active
ANALYTIX_RUNTIME_BACKEND=typescript retired-backend diagnostic remains documented and tested
D-0251/D-0252/D-0253 missing live evidence blocks only live/retirement claims
desktop QA proves Go default contract parity when release-strength claims need it
legacy-origin is not pushed while it points at Kun
```

## 11. P6 Go Agent-Loop Parity

Do not start P6 before P1-P3 are stable.
Do not start P6 from P4C alone: P4C closes a scoped rewind/apply slice, while
G5 still needs cross-backend agent-loop, compaction, cache, usage, provider,
approval, user-input, resume/interrupt, and checkpoint oracle fixtures.

Implementation order:

1. Provider streaming and usage parity.
2. Tool call lifecycle parity.
3. Approval/user input parity.
4. Compaction and model-history repair parity.
5. Cache hit/miss and token accounting parity.
6. Full agent task benchmark.

Acceptance:

```text
G3-G6 conformance passes
agent benchmark at least Reasonix parity for absorbed areas
desktop workflow remains at least Kun parity
Go/default runtime evidence improves at least one engine dimension before a
release-strength superiority claim
```

## 12. P7 Remote Execution And Schedule Hardening

Primary source areas:

```text
Kun Connect Phone / claw flows
Kun schedule flows
Kun Feishu/Lark streaming design notes
Reasonix internal/bot
Reasonix internal/botruntime
```

Implementation order:

1. Remote task creates or reuses the correct analytix thread.
2. Streaming deltas survive fetch reconciliation.
3. Desktop and IM approvals share the same policy.
4. Scheduled tasks preserve context and usage accounting.
5. Restart recovery restores pending remote/scheduled tasks.

Acceptance:

```text
Connect Phone user-facing copy remains current
claw stays internal only where allowed
remote approvals round trip
scheduled task thread reuse is deterministic
streaming UI does not lose deltas
```

## 13. P8 Strength Claim Gate

Analytix may claim it is stronger than an upstream only for verified surfaces.

Required evidence:

```text
docs/analytix/upstreams/kun-sync.md
docs/analytix/upstreams/reasonix-sync.md
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/conflict-decisions.md
docs/analytix/benchmarks/upstream-scorecard.md
docs/analytix/benchmarks/benchmark-scenarios.md
docs/analytix/benchmarks/quality-gates.md
docs/analytix/qa/
```

Claim rules:

```text
1. Product breadth claim requires stronger-than-Kun workflow evidence.
2. Engine claim requires at least Reasonix parity plus one measured improvement.
3. Desktop claim requires Electron QA, not only unit tests.
4. Safety claim requires no known approval/sandbox/privacy regression.
5. Release notes must state remaining gaps.
```

## 14. Historical Follow-Up Recommendation

Start with post-cutover validation if the goal is to close current release
truth gaps:

```text
/goal Complete Go post-cutover live validation and D-0253 fallback-retirement evidence for analytix.
```

Then collect or update only:

```text
D-0251 credentialed provider/MCP/packaged/operator evidence when real inputs exist
D-0252 candidate/soak readiness without redoing the default switch
D-0253 replacement-test, rollback-strategy, soak, and operator authorization
scorecard/release-strength closure that does not overclaim live superiority
Kun 0.2.14/current drift absorption through existing analytix product boundaries
```

Do not start a new default-switch scaffold. The default switch has already
landed; the remaining work is evidence, release-strength closure, and explicit
rollback retirement authorization.

## 15. Historical Status: Reasonix Agent-Kernel Five Batches

The earlier P2 sidecar/cache batches are now scoped closures or fixture-backed
proof. Future construction must not let cache, subagent, or Go work skip the
task/permission safety floor. The default risk order remains:

| Order | Batch | Implementation meaning | Do not start before | Current status / acceptance shape |
| --- | --- | --- | --- | --- |
| 1 | Task closure and permission kernel | Implement or harden `permission Gate`, approval posture `ask` / `auto` / `yolo`, separate plan approval and tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules. | First, unless an explicit analytix spec proves an equivalent or stronger safety path. | Scoped TypeScript proof closed. Focused runtime/renderer tests and scorecard rows prove completion/evidence semantics and no weakened approval/sandbox/user-input behavior. |
| 2 | Long-running task system | Add `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, and requirement-by-requirement evidence audit. | Batch 1. | Scoped TypeScript proof closed. Restart/resume and evidence-audit fixtures prove long work is durable without polluting `REASONIX.md`, `AGENTS.md`, stable system prefix, tool schema, or Reasonix renderer protocol. |
| 3 | Tool surface and context economy | Extend token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory on-demand retrieval, compaction archive, and cache diagnostics across providers. | Batch 1 and the relevant long-task state contracts. | Scoped TypeScript proof closed. Dynamic source and provider-cache fixtures prove multi-provider behavior and unsupported telemetry fallback remain safe. |
| 4 | Collaborative execution model | Productize `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, and nested event rendering. | Batches 1 and 3. | Scoped delegated-execution proof closed only. Existing `delegate_task` can be evidence-ledgered, but first-class `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, transcript continuation/fork, and desktop nested QA remain pending. |
| 5 | Go runtime kernel | Keep Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime behind analytix contracts while Go is default. | Stable TS oracle, Go-default contract equivalence, retired-backend diagnostic tests, and future deletion authorization. | Go runtime default is active and contract-equivalence fixes are closed. `ANALYTIX_RUNTIME_BACKEND=typescript` is retired; no renderer/preload/settings switcher, Reasonix public protocol, or top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entry is introduced. |

The next implementation boundary is post-cutover evidence and release-strength
closure, not a Go G1 scaffold. Kun product-entry corrections remain in force:
no top-level Workflow/Create Loop route, Sidebar entry, or Workbench stage may
be restored to carry Reasonix concepts.
