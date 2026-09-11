# analytix code-level upstream absorption blueprint

Status: Reference / historical implementation blueprint with a current
admission addendum.
Current as of: 2026-07-10 for source coverage, license/provenance admission,
production ownership, and the nine-source reuse map below. The dated audit is
`upstream-capability-audit-2026-07-10.md`; pinned source facts live in
`upstream-sources.json`, and code-level intake records live in
`code-reuse-provenance.md`.

This file is the code-level companion to:

```text
docs/analytix/upstreams/absorption-targets.md
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
```

It answers a narrower question:

```text
If analytix is allowed to reuse upstream code directly, which code should be
reused, how should it land, and how do we prove it made analytix stronger?
```

License, authorization, and provenance are hard admission gates for every
code, test, prompt, document, binary, or asset reuse decision. Direct reuse is
allowed only when the pinned source material has an admitted reuse policy, all
file-level and vendored exceptions have been reviewed, required notices have a
destination, and the result is adapted behind analytix identity, settings,
bridge, runtime contracts, tests, and QA gates. Missing evidence means
behavior-only clean-room reimplementation or rejection, not provisional copy.

Source roles in this file are default research lenses for the reviewed
snapshot, not exclusive capability boundaries. A future batch may compare any
analytix capability against all relevant registered sources: Reasonix, Kun,
OpenCode, CodexDesktop-Rebuild, Claude Code, Claw Code, Gajae Code, Hermes
Agent, and LazyCodex.

## 1. Historical Reviewed Upstream Snapshot

The table and chronological notes in this section preserve the upstream state
reviewed during the 2026-06-20 to 2026-06-30 implementation waves. They are not
current source authority. Use `upstream-sources.json` and run
`npm run audit:upstreams` before new work.

| Upstream | Branch | Reviewed commit | Default research lens for analytix |
| --- | --- | --- | --- |
| `esengine/DeepSeek-Reasonix` | `main-v2` | requested checkpoint `c202f970`; historical 2026-06-20 observed remote `5d1ad2a`; current 2026-06-21 recheck `49c14762` | Default engine/runtime lens for Go implementation, provider/cache/tool/plugin/checkpoint/session-list behavior. The initial code-level map began at `33545f6`; `bc8249c3` is the post-4f515da goal/control baseline. `48e5b990` / `3e625b91` has been scoped-absorbed as analytix `RemoteEntryControlPort`; `c202f970`, historical `5d1ad2a`, and current `49c14762` SessionAPI/App/cache-relevant drift are currentness inputs only until a behavior delta is classified. |
| `KunAgent/Kun` | `master` | `8602476` | Default stable desktop product workflow lens. The previous `ab24a77` develop line is now merged into master, including Workflow/Create Loop, Telegram Connect, provider routing fixes, preload sandbox fixes, git checkpoint, tray session menu, and desktop workflow changes. |
| `KunAgent/Kun` | `develop` | `247076f` | Develop advanced beyond stable master and the requested `9605e20f` checkpoint. Keep stable `8602476` as the product baseline; `d09d52b0` is now absorbed as an analytix-native Windows installer process-stop fix, while newer speech/SSE/UI drift remains separate currentness input. |

Re-run this audit for every future upstream batch. Do not assume these commit
paths still represent the latest upstream behavior.

P0/P2 start refresh on 2026-06-20 found Reasonix `main-v2` at `ef7bf97`.
The original `33545f6` audit remains the basis for the code-level map, while
the new `f6ba755` memory/control lock-splitting commit is deferred from the
session-sidecar batch and recorded in `reasonix-sync.md`.

P2.1 refresh on 2026-06-20 found Reasonix `main-v2` at `6d404d8`. The
session-sidecar version semantics from `7ebb08e` are absorbed; `341f720` is a
future Go G4/G5 goal-persistence responsiveness requirement.

P3A refresh on 2026-06-20 found Reasonix `main-v2` at `be67a498`. The only
drift from `6d404d8` is `ad3d742` / merge `be67a498` in `desktop/tabs.go` and
`desktop/tabs_topic_test.go`, so the existing Reasonix tool/MCP/sandbox/
checkpoint/control map remains current for this slice.

Post-4f515da refresh on 2026-06-20 moved the active Reasonix baseline to
`bc8249c3` for the goal/control oracle slice. A later refresh found
`main-v2` at `d02457ee`; the only new substantive commit, `98a57ded`, moves
Reasonix sidecar path derivation into `internal/store` with no intended
behavior change. Analytix records this as future Go store-boundary guidance
only; it does not reopen the current TypeScript runtime or renderer contract.

Architecture/spec v1 closure on 2026-06-20 found Kun `master` at `8602476`.
This supersedes the earlier Kun `master` `8f20403` / `develop` `ab24a77`
split for implementation planning: the large develop diff is now stable-master
drift, but landing rules are unchanged.

Post-4f47031f recheck on 2026-06-20 found Reasonix `main-v2` at
`48e5b990`. The substantive new commit `3e625b91` introduces a Reasonix
SessionAPI driving port and migrates the bot gateway to
`Lifecycle + TurnControl + Approvals`. Analytix has now absorbed this as the
analytix-owned `RemoteEntryControlPort` and oracle tests, without exposing
Reasonix-native protocol or starting a Go scaffold. A later recheck found
Reasonix `main-v2` at `c202f970`; its serve/acp/cli SessionAPI migrations were
recorded for classification. The historical 2026-06-20 observed remote was
`5d1ad2a`, adding desktop App-to-SessionAPI port work; the 2026-06-21 recheck
moved the current input to `49c14762`. These remain record-only until a
behavior delta is classified.

Post-4f47031f recheck also found Kun `develop` at `9605e20f`; its new
substantive delta beyond stable master is `d09d52b`, a Windows installer
process-stop fix. A later recheck found Kun `develop` at `247076f`; newer
speech/SSE/UI work is record-only until classified. P0 absorbs `d09d52b0` as
analytix-native NSIS include logic with `ANALYTIX_*` naming and no `KUN_*`
leakage; it does not reopen the stable master product workflow baseline.

2026-06-30 final collaborative-execution recheck found Reasonix `main-v2` at
`ff379b42448b5d0de941b2e2a24eeb94e9ca7ec2` and OpenCode
`anomalyco/opencode` `dev` at `8289883de8a4f8e5fa5324a25b72a6b54bb24f7e`.
For this closed collaborative-execution review, Reasonix was the primary
code-level source for durable child session semantics and OpenCode was a
reference for provider/model inheritance, task-session continuation, and
subagent permission derivation. Future batches must still compare all relevant
sources capability-first. No OpenCode source was copied.

## 2. Reuse Modes

Use one of these modes for every upstream code item.

These modes classify implementation technique after admission; they do not
grant reuse rights. `copy-with-rename`, `port-and-adapt`, and
`wrap-behind-contract` require a permissive or separately authorized source
record plus a destination notice. A `reference-only-*` or
`blocked-provenance` source may use only independently written
`contract-reimplement` behavior until its restriction is resolved.

2026-06-30 UI absorption approval rule, generalized 2026-07-02: upstream
interface code, component behavior, navigation patterns, and screen layouts may
not be ported into analytix before explicit approval. The approval record must
name the source files or feature surface, the analytix destination, and the
tests/QA/scorecard evidence that will prove the port improves analytix rather
than replacing its UI direction.

| Mode | Meaning | Allowed examples | Required proof |
| --- | --- | --- | --- |
| `copy-with-rename` | Copy code or tests almost directly, then rename identity and imports. | Self-contained tests, fixtures, pure helpers, benchmark harnesses. | New file has analytix names, passes local tests, and has no current-product upstream identity leak. |
| `port-and-adapt` | Reuse implementation logic but adapt types, events, settings, errors, and storage. | Provider clients, tool execution, compaction, retry logic, route behavior. | Contract tests prove the adapted behavior matches analytix protocol. |
| `wrap-behind-contract` | Keep upstream-shaped internal implementation but expose only analytix contracts. | Future Go packages behind `analytix serve` HTTP/SSE. | Renderer and preload remain unchanged; conformance fixtures pass. |
| `contract-reimplement` | Use upstream behavior as a reference but write analytix-native code. | UI flows, bridge APIs, settings schema, release identity. | Product workflow benchmark passes without architecture or identity regression. |
| `reject` | Do not reuse. | Product identity, old bridge names, old settings envelopes, UI replacements that fight analytix. | Rejection is recorded in `conflict-decisions.md` or the relevant sync ledger. |

Direct code reuse is not a license to merge whole repositories. The unit of
absorption is a capability, contract, test, or implementation domain.

## 3. Analytix Landing Boundaries

| Analytix area | What may land there | What must not land there |
| --- | --- | --- |
| `src/renderer/src` | Analytix UI flows and product components. Upstream UI ideas or code may be ported only when they preserve the upgraded UI direction and pass explicit approval. | Upstream product identity, upstream bridge names, `window.kunGui`, terminal-only assumptions, current-product `Kun` naming. |
| `src/preload` | `window.analytix` bridge only. | Deprecated bridge aliases or upstream-native renderer contracts. |
| `src/main` | Electron integration, runtime supervision, settings store, Connect Phone integration, packaging glue. | Old process managers, old runtime-control panels, or settings that write upstream-native schemas. |
| `src/shared` | Shared schemas, provider contracts, endpoint behavior, settings types. | Unadapted upstream event/config shapes. |
| `packages/runtime` | Public TypeScript schemas, config, telemetry, `analytix serve` launcher, and retained test/conformance oracles. | A production TypeScript agent loop or fallback backend. |
| `packages/runtime-go` | The only production agent core, HTTP/SSE runtime implementation, use cases, and adapters behind analytix contracts. | A second public product, public `reasonix` CLI, renderer-visible Go-specific events, or a route back to the retired TypeScript loop. |
| `docs/analytix/benchmarks` | Scorecards, scenarios, quality gates, benchmark evidence. | Claims of superiority without repeatable evidence. |

## 4. Reasonix Code-Level Absorption Map

Reasonix should be treated as a default engine implementation lens for this
code-level map. Its Go code is especially valuable, but it must not define
analytix product identity or desktop contracts, and future capability work may
choose stronger ideas from other upstreams.

| Upstream source | What to absorb | Analytix landing | Reuse mode | Proof |
| --- | --- | --- | --- | --- |
| `internal/control` | Transport-neutral controller ideas: approvals, attachments, input, refs, slash commands, auto-plan, runtime status, shell cancel, rewind orchestration. | Runtime controller facade behind `analytix serve`, main runtime adapter, route handlers. | `port-and-adapt` now, `wrap-behind-contract` for Go stages. | Approval/user-input/attachment/rewind conformance and desktop QA. |
| `internal/serve` | HTTP/SSE serving shape, broadcaster, title cache, CSRF discipline, model switching tests. | `packages/runtime-go/internal/adapters/inbound` and the public TypeScript contracts/launcher where the boundary changes. | `port-and-adapt`. | SSE replay, `/health`, model switch, fork/archive/search/usage route parity. |
| `internal/agent` | Agent loop behavior: cache shape, compaction, coordinator, parallel tasks, subagent store, usage, repeat/storm guards, reasoning language, final readiness. | `packages/runtime-go/internal/app`, domain/ports, and conformance fixtures. | `port-and-adapt`; admitted tests may become fixtures. | Agent task benchmarks, cache-hit tests, long-session compaction and repair fixtures. |
| `internal/provider`, `internal/provider/openai`, `internal/provider/anthropic` | Provider registry, request/stream parsing, retries, model fetch, images, thinking/reasoning fields, schema canonicalization. | Shared provider contract and runtime model adapters. | `port-and-adapt`. | URL/body/header matrix, streaming chunks, usage/cache accounting, provider 404 guidance tests. |
| `internal/plugin` | MCP-compatible plugin lifecycle, stdio/HTTP transports, canonicalization, lazy loading, hot add, stats, prompt/resources handling. | Runtime tool/plugin provider and desktop marketplace plumbing. | `port-and-adapt`. | Tool lifecycle, cancellation, malformed plugin, cache-stable schema, approval tests. |
| `internal/tool` and `internal/tool/builtin` | Built-in tool ideas: shell, grep/glob/ls/read, edit/multiedit, code index, web fetch, todo, protected dirs, encoding handling. | Analytix tool adapters with existing approval and sandbox policies. | `port-and-adapt`; pure fixtures may be `copy-with-rename`. | File mutation, encoding, protected path, timeout/cancel, transcript/file result propagation tests. |
| `internal/permission`, `internal/sandbox`, `internal/proc` | Permission and sandbox semantics, shell process lifecycle, platform-specific guards. | Approval policy, sandbox mode, command execution service. | `port-and-adapt`. | Denied tools do not run, command cancellation is reliable, sandbox restrictions are honored. |
| `internal/checkpoint` and `docs/CHECKPOINTS.md` | Snapshot and rewind model for code and conversation safety. | Analytix file-change review, restore, generated files, and thread history flows. | `port-and-adapt`. | Apply edits, rewind code-only/conversation-only/both, then resume without event corruption. |
| `internal/history`, `internal/memory`, `internal/retrieval`, `internal/skill`, `internal/instruction` | Session memory, retrieval, skills, instructions, project memory patterns. | Analytix memory/skills layer and prompt construction. | `port-and-adapt`. | Retrieval relevance, skill loading, stable prefix, privacy filtering, long-thread replay tests. |
| `internal/bot`, `internal/botruntime`, `docs/BOT_GUIDE*` | Feishu/Lark/WeChat remote task entry, approvals, commands, bot runtime lifecycle. | Connect Phone implementation and scheduled task integration. | `contract-reimplement` for UI, `port-and-adapt` for engine pieces. | Remote task creates/reuses thread, approvals round trip, streaming is not overwritten by fetch reconciliation. |
| `benchmarks`, `cmd/e2ebench`, `prod_test` | Mutation/e2e benchmark discipline and repeatable engine evaluation. | `docs/analytix/benchmarks` plus executable benchmark scripts when implemented. | `copy-with-rename` or `port-and-adapt`. | Scorecard rows have raw evidence and are reproducible. |
| `cmd/reasonix`, `Makefile`, `.goreleaser.yaml`, `npm` packaging | Single static Go binary, npm-installed native binary pattern, cross-platform release discipline. | Internal `analytix serve` runtime binary packaging. | `contract-reimplement`. | `analytix serve` starts, package artifacts have analytix identity, checksums/signing pipeline works. |
| `desktop` | Desktop-adjacent Go/Wails process ideas only. | Analytix Electron remains the desktop product. | `contract-reimplement` or `reject`. | No renderer contract or UI shell regression. |

Latest delta to prioritize:

| Reasonix commit | What changed | Analytix decision |
| --- | --- | --- |
| `249a4f8` | Session list caches turn count and preview in `.meta` sidecars, with tests and a benchmark. | `must absorb` for long-history desktop/sidebar performance after analytix contract adaptation. |
| `73e2025` | Fork/branch sessions seed sidecar counts/previews and goal-state persistence failures are logged. | `must absorb` for fork/branch reliability and persistence observability. |
| `5db1d0b` | Removes now-redundant project-session disk cache after sidecar-only listing makes it unnecessary. | `must absorb` where analytix has equivalent redundant cache/invalidation complexity. |
| `f6ba755` | Moves memory-write disk I/O off the Reasonix controller lock. | `should absorb` in a later memory/control responsiveness batch; not part of the first session-sidecar slice. |
| `dbaea843` / `bc8249c3` | Extracts Reasonix goal-control state into a goal machine and merges the scoped goal/control delta. | Already absorbed as analytix-owned `GoalControlMachine` oracles without exposing Reasonix markers, sidecars, renderer protocol, or Go scaffold. |
| `98a57ded` / `d02457ee` | Centralizes Reasonix session sidecar path derivation in `internal/store`. | Record-only for current analytix. Use as future Go store-boundary guidance after TS oracle fixtures are scheduled. |
| `3e625b91` / `48e5b990` | Introduces a Reasonix SessionAPI driving port and migrates the bot gateway to narrow lifecycle/turn/approval interfaces. | Scoped-absorbed as analytix-owned `RemoteEntryControlPort` boundaries and tests; remote/bot-like entry cannot reach goal/checkpoint/memory/storage surfaces and does not expose Reasonix protocol. |
| `49c14762` currentness recheck | Current Reasonix main-v2 input after historical `5d1ad2a` App-to-SessionAPI drift. | Record-only for code drift; P2.2 cache/provider proof is implemented through analytix fixtures, not by copying Reasonix public protocol. |
| `adf228b3` currentness recheck | Reasonix main-v2 input on 2026-06-29. `internal/agent/subagent_store.go` continuation/fork/stale-run ideas and `internal/agent/task.go` / `internal/agent/parallel_tasks.go` orchestration concepts remain the source for collaborative execution. | `port-and-adapt`: TS runtime now has durable child side threads, continue/fork lineage guards, stale child-run abort reconciliation, and task-job `childRunId` linkage through thread-summary and renderer contracts. No Reasonix public SessionAPI, task protocol, settings root, CLI identity, or top-level Subagent UI is exposed. |
| `046f575e` currentness recheck | Latest Reasonix main-v2 input on 2026-06-30. The delta from `adf228b3` is desktop safety/config/provider maintenance, with no new `internal/agent` child-session semantic change. | `record`: keep the collaborative execution landing pinned to the existing `internal/agent/subagent_store.go`, `task.go`, and `parallel_tasks.go` absorption points; do not import Reasonix desktop bridge, SessionAPI, CLI identity, or public task protocol. |
| `ff379b42` currentness recheck | Latest Reasonix `main-v2` input on 2026-06-30 final closure. No new public product surface is imported; the relevant durable child-session semantics still map to `internal/agent/subagent_store.go`, `task.go`, and `parallel_tasks.go`. | `record` plus closure proof: analytix keeps Electron/HTTP/SSE contracts, adds structured `provider_not_found`, full `ModelExecutionRef`, profile-level provider/model override, transcript identity mismatch guards, durable continue/fork without transcript prompt injection, renderer summary provider/model display, and side-panel child thread opening. |

Reasonix code is strongest where it improves engine quality: cache, provider
compatibility, tool execution, plugin lifecycle, checkpointing, and Go
distribution. It should not replace analytix desktop UX.

### 4.1 Historical Reasonix Agent-Kernel Five-Batch Reuse Map

The code-level map below was authoritative for the 2026-06 Reasonix
implementation sequence. It is retained as evidence that permission,
evidence, and Goal closure preceded collaborative execution and the Go
cutover; it is not current construction authority.

| Batch | Reasonix source families | What to reuse | Reuse mode | What to reject |
| --- | --- | --- | --- | --- |
| 1. Task closure and permission kernel | `internal/permission`, `internal/sandbox`, `internal/control`, `internal/agent`, approval manager/control helpers | `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan approval vs tool approval separation, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, headless/subagent approval rules. | `port-and-adapt` for semantics; `copy-with-rename` only for pure fixtures; `wrap-behind-contract` for future Go Permission Gate. | Reasonix public protocol, terminal prompt UX, controller-lock public shape, renderer-visible approval/user-input events. |
| 2. Long-running task system | AutoResearch / research mode state, task progress stores, evidence audit helpers | `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, requirement-by-requirement evidence audit. | `contract-reimplement` for storage and UI; `port-and-adapt` for audit logic. | Writes to `REASONIX.md` or `AGENTS.md`, stable system prefix changes, tool schema changes, renderer-visible Reasonix protocol. |
| 3. Tool surface and context economy | `internal/tool`, `internal/plugin`, `internal/history`, `internal/memory`, `internal/retrieval`, provider/cache helpers | token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory on-demand retrieval, compaction archive, cache diagnostics. | `port-and-adapt` for provider/tool behavior; `wrap-behind-contract` for registries; `copy-with-rename` for fixtures. | DeepSeek-only assumptions, cache telemetry guessing, raw prompt/tool/file/provider-body diagnostics. |
| 4. Collaborative execution model | `internal/agent` coordinator, task and subagent orchestration, job runners, event rendering concepts | `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, nested event rendering. | `needs redesign` for product semantics; `port-and-adapt` only after batch 1 and 3 gates; `contract-reimplement` for renderer projection. | Claiming completion by only enabling `subagents.enabled` or `delegate_task`, top-level Subagent/AutoResearch UI, unpermissioned parallel execution. |
| 5. Go runtime kernel | Go packages for provider/tool/controller/session/event/job/permission/MCP/memory/goal | Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, Goal Runtime. | `wrap-behind-contract` and `contract-reimplement` against TS oracle G0-G6. | Reasonix CLI/settings identity, Reasonix public event semantics, renderer/backend switcher, renderer/preload/main bridge contract drift, Rust/Tauri migration, and top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product-entry drift. |

2026-06-30 collaborative execution landing (historical paths):

Several TypeScript production paths named in this table were later retired and
removed. Current ownership is under `packages/runtime-go/internal/app/subagent`,
the remaining Go HTTP/summary adapters, public TypeScript contracts/oracles,
and the renderer summary components. Do not recreate the removed TypeScript
delegation/runtime files to satisfy these historical rows.

| Reasonix source | Analytix landing | Status and proof |
| --- | --- | --- |
| `internal/agent/subagent_store.go` (`PrepareContinue`, fork preparation, stale running cleanup) | `packages/runtime/src/delegation/delegation-runtime.ts`, `packages/runtime/src/delegation/child-agent-executor.ts`, `packages/runtime/src/contracts/model-execution-ref.ts`, and `packages/runtime/src/services/thread-service.ts` | `port-and-adapt`: child runs use durable side threads, `continue` appends to the same child session, `fork` clones to a new side thread, parent lineage is validated, queued/running child records are reconciled to aborted after restart, and parent `ModelExecutionRef` metadata is preserved into child runs instead of falling back to the default provider. Covered by `child-agent-executor.test.ts` and `delegation-runtime.test.ts`. |
| `internal/agent/task.go` and `internal/agent/parallel_tasks.go` | Current Go subagent/task-job application packages, Go thread-summary routes, public TypeScript contracts, `src/renderer/src/components/summary/TaskSummaryRows.tsx`, and `src/renderer/src/components/summary/SubagentInspectorPanel.tsx` | `port-and-adapt` / `contract-reimplement`: task jobs persist `childRunId` as soon as the child starts, summary DTOs carry linked `childThreadId`/`canOpenThread` plus provider/model/endpoint/source metadata, durable `continue_from` / `fork_from` uses transcript refs rather than prompt-level transcript injection, and the renderer can open the child thread while output/kill/restart actions stay on the Go runtime routes. The former Electron live-command registration, manual restart/raw-output capture, PID signalling, and process-snapshot action shadowing are rejected as parallel authority and removed. Electron retains only strict metadata projection and fixed withheld output compatibility. Covered by task-job orchestration, thread-summary, renderer summary, endpoint/payload tests, metadata-only background-task IPC tests, Electron no-effect architecture guards, G3/G4 conformance, and G5 executable Go summary action conformance. |
| Desktop thread handoff rollback | `src/main/services/thread-handoff-service.ts`, `src/renderer/src/store/chat-store-thread-actions.ts`, and `src/renderer/src/components/chat/above-composer/ThreadHandoffProgressModal.tsx` | `contract-reimplement`: worktree handoff preserves analytix renderer/main authority. If the renderer cannot finalize the target workspace/thread switch after git worktree preparation, `failSwitch` marks the operation failed, surfaces terminal state, and restores the source stash so thread/workspace state stays recoverable; the renderer store does not update thread workspace or write the worktree registry until runtime workspace update succeeds. Covered by thread handoff service success, retry, cancellation, renderer-switch-failure rollback tests, renderer store success finalization tests, and renderer fail-switch projection tests. |

Electron smoke on 2026-06-30 covered the collaborative execution UI path with
seeded parent/child threads: `window.analytix` was the only renderer bridge,
runtime info and thread summary preserved `anthropic-main` / `messages`, direct
SSE replay plus renderer `startSse` replayed the child event after reconnect,
the pinned summary rendered the subagent provider/model line, the child-agent
inspector loaded the child timeline and ModelExecutionRef metadata, and
`openThreadInSidePanel('thr_ui_child')` hydrated the side panel through the
standard thread detail/subscription path.

2026-06-30 OpenCode auxiliary review:

| OpenCode source | Analytix lesson | Reuse decision |
| --- | --- | --- |
| `packages/opencode/src/tool/task.ts` | `task_id` resumes an existing child session, a fresh child session records parent/session metadata, child model defaults to the parent message provider/model unless the subagent declares one, and background task metadata stays structured. | `reference-only`: implemented through analytix `ModelExecutionRef`, task-job transcript refs, durable child thread metadata, and summary DTOs; no OpenCode source copied. The implementation keeps endpoint format and provider identity from the parent thread and records explicit/model-profile source changes. |
| `packages/opencode/src/agent/subagent-permissions.ts` and `agent/agent.ts` | Subagent permissions are derived explicitly, preserving parent deny/external-directory constraints and requiring subagent-declared task/todo powers. | `reference-only`: analytix keeps its approval/sandbox/tool-host contracts and records subagent tool kind/permission inheritance without adopting OpenCode permission schemas. |
| `packages/opencode/src/provider/provider.ts`, `session/session.ts`, and `session/prompt.ts` | Provider registry breadth, model/provider/variant persistence on sessions, and prompt execution through the selected provider reinforce that continuation must use session metadata rather than default-provider fallback or prompt transcript replay. | `reference-only`: analytix retains shared provider settings, endpoint-format handling, custom full endpoint mode, and runtime HTTP/SSE contracts while extending transcript identity with provider/endpoint/source fields. |

2026-06-30 closure proof: `git diff --check`, `npm run typecheck`,
`npm run build:runtime`, `npm run test` (320 files / 2816 tests), the required
runtime and renderer/main/preload focused vitest matrices, and
`cd packages/runtime-go && go test ./internal/server` pass. `npm run dev` plus
Playwright verified the right summary surface, provider/model summary metadata,
`canOpenThread` child rows, and child side-panel opening without a parent-thread
switch.

Future diffs against Reasonix `main-v2` must first classify changes into this
map as `copy-with-rename`, `port-and-adapt`, `wrap-behind-contract`,
`contract-reimplement`, `reject`, or `defer pending evidence`. Current drift
after `49c14762` must be rechecked and classified before any implementation.

## 5. Kun Code-Level Absorption Map

Kun should be treated as the product workflow and existing desktop-runtime
lineage source. Because analytix already comes from the Kun 0.2.13 lineage,
future Kun work is baseline fidelity audit, 0.2.14 / `8602476` delta closure,
and regression repair during the upgrade to analytix.

| Upstream source | What to absorb | Analytix landing | Reuse mode | Proof |
| --- | --- | --- | --- | --- |
| `kun/src/contracts` | Runtime contracts for approvals, attachments, events, items, threads, turns, usage, workspace, review, endpoint formats. | `packages/runtime/src/contracts` and `src/shared` analytix schemas. | `copy-with-rename` for compatible tests; otherwise `port-and-adapt`. | Contract tests and no active `agents.kun`/`window.kunGui`/`kun serve` regression. |
| `kun/src/server/routes` | HTTP/SSE route behavior for sessions, turns, events, approvals, user inputs, skills, memory, review, usage, workspace. | Current analytix runtime route surface. | `port-and-adapt`. | GUI route parity: new/resume/fork/archive/search/replay/usage all pass. |
| `kun/src/services` | Thread, turn, usage, review, runtime event recorder behavior. | Runtime services and event persistence. | `port-and-adapt`. | JSONL replay, highest sequence, usage aggregation, review card tests. |
| `kun/src/loop` | Cache-first loop, append-only session log, compaction, context estimator, history healing, tool-call repair, steering queue, storm breaker. | TypeScript runtime oracle and future Go conformance fixtures. | `port-and-adapt`; preserve good tests. | Long-thread, malformed history, compaction continuation, tool storm, usage/cache tests. |
| `kun/src/adapters/model` | Endpoint-format behavior, retries, pricing, proxy fetch, tool-argument repair, image/tool result handling. | Analytix provider adapters and shared endpoint contract. | `port-and-adapt`. | Provider URL/body matrix, retries, image/tool result tests, sanitized errors. |
| `kun/src/adapters/tool` | Built-in tools, MCP, Skills, media generation, computer use, memory tools, sandbox policy, rate limiting, file mutation queue. | Analytix runtime tool providers and approval UI. | `port-and-adapt`. | Tool lifecycle, approval, file/image result, media fallback, sandbox and rate-limit tests. |
| `kun/src/cache`, `kun/src/telemetry`, `kun/src/prompt` | Cache economics, prompt layout, observability, cost accounting. | Runtime prompt/cache policy and usage reporting. | `port-and-adapt`. | Cache hit/miss, token/cost, prompt stable-prefix checks. |
| `src/renderer/src/agent` | Runtime client, event mapper, thread timing, registry behavior. | Analytix renderer agent adapter. | `port-and-adapt`; no upstream names. | Streaming projection, frame scheduler, measured rows, no forced scroll regressions. |
| `src/renderer/src/components/write`, `src/renderer/src/write`, `docs/WRITE_*` | Write workspace, inline completion/edit, RAG, recent edits, image paste, markdown projection, exports. | Analytix Write route and document workspace. | `port-and-adapt`; `contract-reimplement` for UI refinements. | Write benchmark, inline edit/completion tests, export and image path propagation tests. |
| `src/renderer/src/components/sdd`, `src/renderer/src/sdd` | Requirement draft, PM skill frameworks, plan/prototype/verify prompts, trace and history. | Analytix SDD route and plan/review contracts. | `port-and-adapt`. | SDD benchmark: draft to design to plan to agent work to review remains linked and restorable. |
| `src/renderer/src/components/schedule` and schedule-related main/runtime code | Recurring tasks, task defaults, thread reuse, task context. | Analytix schedule UI and runtime task contracts. | `port-and-adapt`. | Schedule creates/reuses threads, survives restart, does not corrupt active streaming. |
| Connect Phone / `claw` / Feishu streaming docs | IM task entry, webhook/relay, streaming reconciliation, manual and scheduled remote tasks. | Analytix Connect Phone; `claw` only where allowed as internal compatibility. | `port-and-adapt`; UI copy is `contract-reimplement`. | Remote streaming deltas survive fetch reconciliation; approvals and user inputs work from desktop and IM. |
| `src/main/workflow-runtime.ts`, `src/renderer/src/components/workflow`, `src/shared/workflow-*`, `docs/workflow-loop*` from Kun | Kun `Create Loop` / Loop creation and workflow canvas entry. | No analytix landing surface. | `reject`. | Conflict decision plus scans proving no Create Loop route/sidebar/workbench command, no current-product Kun naming, and no bridge/settings leak. |
| Kun master `8602476` provider and preload fixes | `thread.providerId`, per-provider HTTP clients, GLM custom full endpoints, removed deprecated Xiaomi model, sandbox preload startup fix. | Analytix provider settings/runtime client and preload safety. | `port-and-adapt`; sandbox fix may be `already covered` if no node builtin is required in preload. | Provider matrix, settings save/load, sandbox preload startup tests. |
| Kun master `8602476` Git/tray/worktree improvements | Git checkpoint service, tray session menu, worktree/project grouping, branch picker refinements. | Analytix project/session utilities and review safety. | `port-and-adapt` or `contract-reimplement` for UI; checkpoint refs/storage must be analytix-named. | Git checkpoint tests, tray/session QA, branch picker and worktree grouping tests. |
| `src/renderer/src/components/chat`, `plan`, `todo`, review cards | Chat command details, plan/review/todo presentation, generated file panels. | Existing analytix upgraded UI. | `contract-reimplement` unless code fits current UI exactly. | Desktop QA confirms commands remain reachable and hot path remains smooth. |
| `docs/model-provider-presets.md` and settings code | Provider preset UX and capability wiring. | Top-level `runtime` settings and provider probe. | `port-and-adapt`. | Settings save/load never writes old agent envelopes; provider probe and 404 guidance tests pass. |

Latest Kun stable delta to prioritize:

| Kun commit/range | What changed | Analytix decision |
| --- | --- | --- |
| `8f20403..8602476` | Master merged develop (#444), moving Workflow/Create Loop, Telegram Connect, provider fixes, preload sandbox, git checkpoint, tray/worktree/composer fixes into stable master. | Use this as the current stable Kun absorption baseline. |
| `5772400` | GLM coding providers use custom full `/chat/completions` endpoints; compaction keeps tool-result tails paired with calls. | `must absorb` in P1.1 provider/runtime critical fixes. |
| `694fc4e` | Removes deprecated Xiaomi `mimo-v2-flash` model. | `must absorb` in P1.1 provider preset cleanup. |
| `6bd1edc` | Adds per-provider runtime routing through `thread.providerId` and `MultiProviderModelClient`. | `must absorb` behind analytix `runtime` settings and HTTP/SSE contracts. |
| `a502a6d` | Improves Telegram Connect settings/UI and adds Telegram runtime. | `should absorb` in P1.2 Connect Phone / Telegram. |
| Workflow/Create Loop commits (`5488b9a` through `a43a2d5`) | Adds Dify/n8n-style workflow builder, typed variables, node descriptors, hook triggers, run history, custom modules, human approval node. | `reject for current surface`: do not absorb as implied Kun parity or an unreviewed top-level entry. Future workflow automation may be reconsidered through an analytix-native spec, UX rationale, route-surface tests, and desktop QA. |
| `8cbf8e6` | Adds turn rollback git checkpoints. | `should absorb` in P3 checkpoint/review safety with analytix naming. |
| `ec42254` and worktree/sidebar/composer fixes | Adds dynamic tray session menu and desktop picker/grouping polish. | `should absorb` through current analytix UI patterns. |
| `b14e31f` / `dd3252d` | Fixes sandboxed preload startup when node builtins are needed. | `already covered` in current analytix preload if no node builtin is used; keep a regression guard. |
| `d09d52b` on develop `9605e20f` | Stops bundled processes under the Windows install directory before NSIS upgrade. | Absorbed in P0 as analytix-native `build/installer.nsh` plus `electron-builder.config.cjs` `nsis.include`, with `ANALYTIX_*` naming and no `KUN_*` env leakage; Windows machine QA still blocks release readiness. |

Kun code is strongest where it preserves product breadth. It should not
reintroduce old identity, old UI structure, or old bridge/settings names.

## 6. Historical Two-Source Conflict Shorthand

This table preserves the 2026-06 Kun/Reasonix starting lenses. It does not
assign ownership to an upstream. The current nine-source map in Section 6.1
supersedes it for new work; Analytix remains the owner and all relevant sources
must be compared.

| Capability | 2026-06 starting lens | Current decision rule |
| --- | --- | --- |
| Product flow and UI entry point | Analytix | Kun UI may inform analysis only after explicit approval; do not preserve or port Kun entry layers by default. |
| Workflow/Loop builder | Analytix rejection list | Kun `Create Loop` / Loop creation must not be absorbed or exposed. Any unrelated future workflow concept requires a new analytix-native proposal and explicit approval before code work. |
| Runtime HTTP/SSE contract | Analytix | Override neither upstream; adapt both behind analytix contracts. |
| Agent loop internals | Analytix Go runtime is the only production owner; Reasonix/Kun provide admitted research and retained contract/conformance evidence only | Choose the Analytix-native implementation that passes conformance and improves benchmark score; do not recreate a TypeScript production extension or rollback loop. |
| Provider request/stream parsing | Analytix shared provider contract | Use whichever upstream implementation better satisfies URL/body/usage/error fixtures. |
| Tool execution | Reasonix engine patterns plus Kun desktop integration | Use stricter approval/sandbox behavior and better transcript/file result propagation. |
| MCP/plugin lifecycle | Reasonix engine patterns | Keep Kun marketplace/skills product flow when it is better for desktop users. |
| Write/SDD/Connect Phone/Schedule | Kun | Reasonix bot/runtime ideas may improve remote execution but not replace UI. |
| Checkpoint/rewind | Reasonix engine patterns | Must integrate with analytix review/generated-files/history UI before release. |
| Cache and token economics | Reasonix for prefix-cache discipline, Kun for current runtime telemetry | Benchmark decides final prompt/cache layout. |
| Packaging | Reasonix Go binary discipline | Product identity, release channel, and installer naming remain analytix. |

If evidence is inconclusive, classify the item as `defer pending evidence`,
write a spike task, and keep the current analytix behavior.

### 6.1 Current Nine-Source Reuse Map

This map supersedes the older two-source ownership shorthand for new work.
Every row still requires a file-level entry in `code-reuse-provenance.md`
before implementation material is copied or substantially adapted.

| Source | Highest-value current use | Current reuse posture |
| --- | --- | --- |
| DeepSeek-Reasonix | Atomic persistence, session lease/CAS, recovery, event indexing, provider/cache and Windows safety tests. | MIT `port-and-adapt` is eligible with notice; map into Go domain/app/adapters and keep analytix protocols. |
| Kun | Design workflow, OAuth/MCP security, memory provenance, repo map, product-flow regression evidence. | PolyForm Noncommercial and required notice; direct reuse is limited to admitted noncommercial scope or separate written authorization. |
| OpenCode | Durable inbox/coordinator/context epoch, timeline E2E, MCP OAuth/resources/prompts, LSP/formatter, snapshot algorithms. | MIT reuse is eligible with notice; adapt Effect/SQLite/Solid-specific designs to Go/React contracts. |
| CodexDesktop-Rebuild | Version-drift, extract-manifest and behavior observation only. | No repository-wide source/asset license evidence: clean-room/reference only; ignored generated extracts are not reproducible source evidence. |
| Claude Code | Attach/resume, remote control, managed settings, plugin lifecycle, layered review behavior. | Proprietary/all-rights-reserved: behavior-only clean-room requirements and tests. |
| Claw Code | Small Rust lifecycle/cancellation/workspace fixtures after code verification. | MIT with notice, but low-confidence/museum maturity; never use its in-memory registries or shell heuristics as production proof. |
| Gajae Code | External control contracts, session tree/handoff, plugin quarantine, receipts and native benchmark ideas. | Direct reuse blocked until pi-mono/oh-my-pi and vendored copyright lineage is resolved; contract-reimplement only. |
| Hermes Agent | Read-only observer ABI, compaction integrity, recovery/FIFO, searchable memory and environment abstraction. | Root MIT may be admitted with notice; excluded file-level Anthropic/Apache materials require their own decision and notices. |
| LazyCodex | LSP/CodeGraph plugins and evidence-receipt guards from committed components. | Root MIT components may be admitted with notice; the uninitialized OmO gitlink and component exceptions are not admitted. |

## 7. Proof That Absorption Is Better

Every code-level absorption must compare three baselines where applicable:

```text
1. upstream baseline: every relevant source at its manifest-pinned commit
2. current analytix baseline before the change
3. analytix after absorption
```

The absorbed change is better only when at least one relevant dimension improves
without regressing mandatory gates.

| Dimension | Acceptable evidence |
| --- | --- |
| Correctness | Contract tests, route fixtures, provider fixtures, replay fixtures. |
| Agent success | Benchmark scenarios showing higher task completion or fewer repair loops. |
| Cost/cache | Stable-prefix checks, cache hit/miss accounting, lower token cost for same task. |
| Latency/smoothness | Runtime timing traces, Electron desktop QA, virtualizer/scroll/composer evidence. |
| Safety | Approval, sandbox, denied tool, protected path, credential and trace privacy tests. |
| Product breadth | Code/Write/SDD/Connect Phone/Schedule workflows complete and reachable. |
| Maintainability | Ledger entry, conflict decision, adapted contracts, and scorecard updated. |

Conformance alone proves analytix did not drift. It does not prove analytix is
stronger. A stronger claim requires benchmark or QA evidence.

2026-06-25 Go default update:

```text
Go runtime default delivery is active through go-runtime-default. The earlier
2026-06-20 G0/G5 inventory remains the TypeScript oracle history that Go must
continue to match for thread/session, SSE replay, checkpoint/rewind plan/apply,
safety audit, tool calls, approvals, user input, plan/goal, model-history
repair, cache accounting, usage, and provider request/stream parsing.
```

| Source | Absorb what | Proof required |
| --- | --- | --- |
| Kun 0.2.13 | Product-lineage baseline preservation. | P1.1-P4C regressions, renderer/main focused tests, desktop QA records. |
| Kun 0.2.14/current | Scoped stable drift already absorbed through analytix contracts. | Kun sync ledger, absorption ledger, scorecard, identity/protocol scan. |
| Reasonix main-v2 | Post-4f515da baseline `bc8249c3`, record-only remote `d02457ee`, scoped-absorbed `48e5b990` / `3e625b91` control-port input, requested `c202f970` SessionAPI drift, and current `49c14762` governance baseline: engine/runtime ideas behind analytix contracts. | Model/cache/MCP/tool/checkpoint fixtures, `GoalControlMachine` oracles, `RemoteEntryControlPort` boundary tests, agent-kernel five-batch route, latest ledgers, Go-default contract equivalence, retired-backend diagnostics for `ANALYTIX_RUNTIME_BACKEND=typescript`, and current retirement authorization. |

The Go default backend has landed. This proof update still does not authorize
Reasonix CLI/settings identity, public Reasonix protocol, renderer/backend
switchers, Rust/Tauri migration, or new top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer product entries.

## 8. Recommended Implementation Sequence

Use this sequence for a major upstream absorption wave.

1. Capture upstream snapshots.
   Run `npm run audit:upstreams`; record branch, commit, license/provenance,
   release notes, file counts, and affected domains in the source ledger and
   `code-reuse-provenance.md`.

2. Classify by target.
   Map every meaningful item to `absorption-targets.md`. If no target exists,
   add one or classify the item as `reject` / `defer pending evidence`.

3. Establish proof before implementation where possible.
   After admission, port eligible tests/fixtures with notices; for restricted
   sources, write clean-room tests from independently specified behavior.

4. Port the implementation behind analytix boundaries.
   Adapt names, settings, event shapes, storage, errors, and UI hooks before
   exposing the behavior.

5. Run focused validation.
   Use the smallest relevant tests first, then desktop QA and benchmark gates
   for cross-layer changes.

6. Update evidence.
   Sync ledger, conflict decisions, benchmark scorecard, and quality gate notes
   must agree with the code.

7. Reject partial identity leakage.
   Any new public `Kun`, `kun serve`, `window.kunGui`, `Reasonix` product
   surface, or upstream-native settings shape blocks completion unless it is
   explicitly inside legacy/import documentation.

## 9. Historical First Code Absorption Batches

These rows preserve the 2026-06 implementation order. They are not current
construction authority; use the construction packets in
`absorption-targets.md` and the dated nine-source audit.

2026-06-21 update: rows below describe historical early absorption waves. At
that time, section 4.1 superseded any order that would put cache-only,
sub-agent, `parallel_tasks`, or Go scaffold work before task closure and
permission kernel proof.

| Order | Batch | Why first | Primary proof |
| --- | --- | --- | --- |
| 1 | Kun master `8602476` stable critical fixes for provider endpoints/models, per-provider runtime routing, preload sandbox guard, composer/worktree/sidebar/tray behavior, plus Write, SDD, Connect Phone, schedule, and provider preset parity validation. | analytix started from an earlier Kun base; closing the stable master drift prevents product regression without copying Kun UI or identity. | Product workflow benchmarks, provider matrix, preload startup guard, desktop route parity tests. |
| 2 | Reasonix session-sidecar, provider/cache, and tool-schema fixtures. | Engine and desktop-history quality can improve without replacing the whole runtime. | Historical session-list evidence; current equivalents include Go provider/history/usage/cache tests and `npm run runtime:go:speed-cache-gate`; live provider evidence remains separate. |
| 3 | Reasonix MCP/plugin lifecycle and tool cancellation semantics. | Improves tool reliability and desktop trust. | Tool lifecycle and approval/sandbox tests. |
| 4 | Reasonix checkpoint/rewind model integrated with analytix review UI. | Big visible safety upgrade over both upstreams when combined with desktop review. | Rewind fixtures and generated-files/review QA. |
| 5 | Go runtime default contract equivalence and post-cutover validation. | Keeps the landed Go default path contract-first without widening the renderer/product surface. | Go runtime tests, adapter tests, product-sovereignty scan, D-0251 live validation when real inputs exist, and D-0253 retirement authorization before physical fallback deletion. |
| 6 | Go agent-loop release-strength closure using Reasonix internals and retained TypeScript contract/oracle fixtures. | Moves analytix toward a stronger engine claim while preserving GUI behavior and release rollback. | G3-G6 conformance, agent/cache benchmarks, desktop QA, and no overclaiming of live provider/MCP/packaged superiority. |
| 7 | Connect Phone remote execution hardening using Kun workflow plus Reasonix bot runtime ideas. | Makes desktop, IM, schedule, and agent loop one coherent product surface. | Remote streaming, approvals, user input, and schedule benchmarks. |

## 10. Review Conclusion

The direction is correct only if analytix treats admitted upstream material as
evidence and bounded implementation input, never as architectural ownership.

The winning shape is:

```text
Kun product breadth and Design research
+ Reasonix persistence and engine depth
+ OpenCode execution/timeline/MCP evidence
+ Hermes recovery/observer/compaction evidence
+ bounded lessons from Claude, CodexDesktop, Claw, Gajae, and LazyCodex
+ analytix upgraded UI, identity, contracts, and desktop QA
= a stronger analytix product
```

The risky shape is:

```text
whole-repo merge
+ mixed settings and event shapes
+ copied UI shells
+ no scorecard
= a bigger but weaker product
```

Therefore, the correct route is admitted code-level absorption or clean-room
contract reimplementation through analytix-owned contracts, with benchmark
proof before any claim that analytix is stronger than a relevant upstream.
