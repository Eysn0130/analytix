# analytix upstream absorption targets

Status: Reference / capability targets with historical implementation notes.
Current as of: 2026-07-10 for the source roster, admission boundary, and
construction packets below.
Source of truth for current gaps: current code plus
`upstream-capability-audit-2026-07-10.md`.

This file answers the practical question:

```text
What exactly should analytix absorb from the nine registered upstream sources,
and how do we prove the absorption made analytix better?
```

Companion documents (the implementation plan is historical):

```text
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
docs/analytix/upstreams/code-level-absorption-blueprint.md
docs/analytix/upstreams/code-level-implementation-plan.md
docs/analytix/upstreams/continuous-absorption-strategy.md
```

## Decision Rule

Do not absorb upstream code because it exists. Absorb only when it improves an
analytix-owned capability and can be proven through contracts, tests, QA, or
scorecards.

```text
upstream idea -> analytix design -> analytix contract -> tests/QA -> scorecard
```

Code, test, prompt, documentation, binary, or asset reuse also requires:

```text
pinned source/license/provenance -> admitted reuse mode -> notice destination
-> adapted implementation -> tests/QA -> scorecard
```

No-license and all-rights-reserved sources are behavior-only clean-room inputs
unless a written authorization record covers the exact material. Root licenses
do not automatically cover gitlinks, vendored directories, generated extracts,
or file-level exceptions.

Source roles are not exclusive. Start from the analytix capability, compare all
relevant upstreams, choose the best proven design, and land it through
analytix-owned contracts.

## Strategic Priority

The first hard engine priority is DeepSeek performance:

```text
analytix DeepSeek behavior must match DeepSeek-Reasonix first,
then exceed it on model reply speed, cache hit rate, and desktop-visible
streaming responsiveness.
```

No release note, benchmark, or product claim may say analytix is better than
DeepSeek-Reasonix for DeepSeek unless it includes measured evidence for time to
first token, tokens per second, end-to-end turn latency, cache hit/miss tokens,
cache hit rate by stable prefix hash, prefix-change reasons, provider/model/
endpoint attribution, and renderer streaming frame cost.

2026-06-30 governance update, revised 2026-07-02:

- Kun `Create Loop` / Loop creation is not a current automatic absorption
  target and must not enter analytix as implied Kun parity. Future analytix-
  native workflow automation may still be approved by a separate product spec.
- Future analytix UI development that absorbs or ports interface patterns,
  screens, navigation entries, or component behavior from any upstream requires
  explicit approval before implementation. Approval must identify the source,
  the analytix landing surface, and the proof that the result fits analytix
  better than the current analytix UI.

## Reasonix Absorption Targets

Reasonix is the default engine/runtime research lens and the mandatory
DeepSeek/runtime comparison source. It is not the exclusive engine source.
Absorb implementation ideas and behavior behind analytix contracts; do not
expose Reasonix product identity as the analytix surface.

| Target | What to absorb | Analytix landing | Proof that it is better |
| --- | --- | --- | --- |
| Go runtime backend | Go architecture, static binary distribution, cross-platform build discipline. | Go backend behind `analytix serve`; renderer remains backend-neutral. | G0-G6 conformance, startup/memory/replay benchmarks, desktop QA with Go backend. |
| DeepSeek prefix-cache stability | Stable prefix strategy, cache-preserving sessions, cache hit/miss accounting. | Runtime prompt/cache policy and provider usage parsing. | Go prefix/provider/usage tests, `npm run runtime:go:speed-cache-gate`, token/cost scorecard, no dynamic workspace data in stable prefix, live provider matrix before superiority claims. |
| Provider/config registry | Config-driven provider definitions and OpenAI-compatible endpoint flexibility. | Top-level `runtime` and provider settings, never Reasonix-native settings. | Provider URL/body/header matrix, stream/usage/reasoning parsing tests, provider 404 guidance. |
| Two-model planner/executor | Planner/executor separation where it improves task success or cost. | Analytix runtime capability flag and UI model controls if productized. | Agent task benchmark improves success/cost without breaking cache, approvals, or transcript. |
| Plugin/MCP tool execution | Stdio JSON-RPC/MCP-compatible plugin process model, built-in tool registration ideas. | `packages/runtime-go` app/outbound adapters, public contracts where needed, and desktop approval flow. | Tool lifecycle fixtures, approval/denial tests, file/image result propagation tests. |
| Tool schema canonicalization | Stable tool schemas and deterministic registration. | Shared runtime contracts and cache-aware tool registry. | Go tool-catalog/prefix-shape tests, the speed/cache gate, and malformed tool-call/tool-result history tests. |
| Sandbox and permissions | Clear engine permission model and CLI permission ergonomics. | Analytix approval policy, sandbox mode, and desktop settings. | Safety benchmarks: denied tools do not run, sandbox honored, user input cancellable. |
| Checkpoints/rewind | Snapshot safety net and patch rollback ideas. | Analytix file-change review, restore, and generated files/history flows. | Fixture that applies edits, rewinds safely, and preserves transcript/event durability. |
| Context compaction and repair | Context estimation, compaction, model-history repair, long-session stability. | Runtime loop services and thread replay model. | Long-thread replay, compaction continuation, malformed event recovery fixtures. |
| CLI/package/release engineering | npm-installed native binary, cross-compile targets, checksums, signing ideas. | Internal runtime packaging and release pipeline, still named analytix. | Packaging QA, `analytix serve` readiness, artifact/signing/checksum checks. |
| Benchmark discipline | Upstream benchmark/prod-test structure and performance habits. | `docs/analytix/benchmarks` plus automated fixtures where feasible. | Scorecard updated for affected dimensions and release claim gate satisfied. |

### Reasonix Agent-Kernel Five-Batch Targets

These targets describe a risk-priority order, not a source-exclusive roadmap.
They prevent sub-agent, `parallel_tasks`, cache-only work, or Go scaffold from
skipping the task/permission safety kernel unless a future analytix-native spec
proves an equivalent or stronger safety path.

| Batch | Absorb what | Analytix landing | Proof that it is better | Priority |
| --- | --- | --- | --- | --- |
| Task closure and permission kernel | `permission Gate`; approval posture `ask` / `auto` / `yolo`; plan approval separated from tool approval; `complete_step`; evidence ledger; Goal state machine; blocked-state detection; headless/subagent approval rules. | Existing chat, goal, runtime approval, and evidence contracts. | Denied/auto/yolo approval fixtures, plan-vs-tool approval tests, goal completion and blocked-state tests, evidence ledger scorecard row, no weakened sandbox or user-input behavior. | `must` / first |
| Long-running task system | `/goal --research`; AutoResearch project-local state; `task_spec.md`; `progress.json`; `findings.jsonl`; `directions_tried.json`; `iteration_log.jsonl`; requirement-by-requirement evidence audit. | analytix-owned project-local goal/runtime state. | Restart/resume fixtures prove durable progress and evidence audit without modifying `REASONIX.md`, `AGENTS.md`, stable system prefix, tool schema, or renderer-visible Reasonix protocol. | `must` after batch 1 |
| Tool surface and context economy | token economy mode; `connect_tool_source`; dynamic tools; stable tool schema; history/memory on-demand retrieval; compaction archive; cache diagnostics. | Runtime provider/tool/context contracts and scorecarded provider matrix. | Multi-provider request/body/header/stream/usage snapshots pass; unsupported cache telemetry remains unknown rather than 0% hit/all miss; cache/provider benchmarks improve without DeepSeek-only behavior. | `should` after batches 1-2 |
| Collaborative execution model | `task`; `parallel_tasks`; background jobs; planner/executor Coordinator; subagent transcript continuation/fork; nested event rendering. | Product-grade execution inside existing chat / goal / runtime / settings surfaces. | Parallel/background tasks remain permission-gated, transcript-continuable, forkable, rendered as nested events, and audited; `subagents.enabled` / `delegate_task` alone is insufficient. | `should` / needs redesign |
| Go runtime kernel | Provider Registry; Tool Registry; Controller; Session; Event Sink; Job Manager; Permission Gate; MCP Client; Memory/History Retrieval; Goal Runtime. | Backend-neutral Go runtime behind `analytix serve` and TS oracle G0-G6. | Go matches TS oracle fixtures for the previous batches, improves at least one engine dimension, and leaves renderer/preload/main bridge unchanged. | `defer` until TS oracle stable |

2026-06-30 collaborative execution status:

- Reasonix source reference: latest `main-v2` recheck at
  `ff379b42448b5d0de941b2e2a24eeb94e9ca7ec2`. The latest delta from
  `046f575e2d2007921ae5ff711436b6a9ca72125a` remains outside the
  child-session absorption target; the relevant source files remain
  `internal/agent/subagent_store.go`, `internal/agent/task.go`, and
  `internal/agent/parallel_tasks.go`.
- OpenCode auxiliary reference: `anomalyco/opencode` default `dev` at
  `8289883de8a4f8e5fa5324a25b72a6b54bb24f7e`, reviewed for
  `packages/opencode/src/tool/task.ts`,
  `packages/opencode/src/session/prompt.ts`,
  `packages/opencode/src/session/session.ts`,
  `packages/opencode/src/provider/provider.ts`,
  `packages/opencode/src/agent/agent.ts`, and
  `packages/opencode/src/agent/subagent-permissions.ts`. No OpenCode code was
  copied; the borrowed ideas are provider/model inheritance from parent
  message metadata, explicit subagent permission derivation, task_id session
  continuation, background task metadata, provider registry breadth, and
  continuation through session metadata instead of prompt-level transcript
  replay.
- Analytix landing: durable child side threads, child continue/fork lineage
  guards, stale child-run reconciliation on restart, durable task-job to
  child-run linkage, thread-summary projection, renderer summary rows that can
  open linked child threads in the side panel, `ModelExecutionRef` propagation
  from parent thread/model selection into `ToolHostContext`, delegation input,
  `ChildAgentExecutor`, child thread creation, runtime events, TS/Go summary
  routes, and renderer summary metadata while preserving Go-owned output/kill/
  restart task actions. The 2026-06-30 Electron live-command shadow registry,
  process snapshot action merge, manual restart, and raw output recovery were
  later classified as unsafe parallel authority and retired. The remaining
  `background-task:*` compatibility surface is strict metadata-only IPC with
  output withheld and no spawn, restart, or PID-signalling authority. Go
  thread-summary/task routes remain the sole effect path. Renderer-switch
  failure rollback for
  thread handoff, renderer-side fail-switch projection without worktree
  registry writes, and the Go thread-summary mirror.
- Reuse classification: `port-and-adapt` / `contract-reimplement` behind
  analytix `DelegationRuntime`, `ChildAgentExecutor`, `ThreadService`,
  `DurableTaskJobManager`, runtime HTTP summaries, and renderer contracts.
- Provider and continuation hardening: child subagents now inherit parent
  `providerId`, `modelId`, `endpointFormat`, optional `variant`, and execution
  `source` unless the subagent profile or explicit input overrides the model.
  The same metadata is persisted on child run records and replayed to the
  right summary. Durable task `continue_from` / `fork_from` no longer injects
  `Continue from prior subagent transcript` or `Previous transcript` into the
  child prompt; the transcript reference selects reuse/fork semantics instead.
  Explicit `request.providerId` or `thread.providerId` misses now raise
  structured `provider_not_found`; they do not fall back to the default
  provider. `ModelExecutionRef` includes provider/model/variant/endpoint,
  base/custom endpoint fingerprints, capability fingerprint, source,
  `resolvedAt`, and fallback-only `fallbackReason`.
- Boundary held: no Reasonix public SessionAPI, task protocol, settings root,
  CLI identity, top-level Subagent UI, or Kun Create Loop surface is exposed.
- Proof recorded in focused TS runtime, renderer summary, Go summary, G3/G4
  conformance tests, and refreshed G5 Go conformance. The release workflow,
  summary-route sovereignty counts, shared endpoint builders, renderer provider
  route encoding and payload preservation, executable summary task
  output/kill/restart behavior, `backgroundTasks` preload facade, summary
  metadata-only background-task IPC rejection/withheld projections, the
  absence of renderer process-shadow registration and Electron process effects,
  plus thread
  handoff renderer-failure rollback and renderer fail-switch projection are now
  fixture-backed evidence.
- 2026-06-30 validation evidence: `git diff --check`, `npm run typecheck`,
  `npm run build:runtime`, `npm run test` (320 files / 2816 tests), the two
  required focused vitest matrices, focused renderer summary tests, and
  `cd packages/runtime-go && go test ./internal/server` pass after the
  provider/ModelExecutionRef and prompt-injection fixes. Isolated
  Electron/CDP smoke with seeded `thr_ui_parent` / `thr_ui_child` also passes:
  `window.analytix` is present with no `window.kunGui` / `window.reasonix*`
  aliases, `/v1/runtime/info` reports `providerId: anthropic-main` and
  `endpointFormat: messages`, `/v1/threads/thr_ui_parent/summary` returns the
  child `modelExecution`, direct SSE replay and renderer `startSse` from
  `sinceSeq=3` replay the child event at seq 6, the right "Pinned summary"
  panel renders `Provider audit` with `anthropic-main/claude-3-5-sonnet`, the
  child-agent inspector renders `endpoint messages`, `source thread`, and the
  child message, and `openThreadInSidePanel('thr_ui_child')` hydrates the side
  panel from the standard child thread detail/subscription path.
- 2026-06-30 local dev UI proof: `npm run dev` rebuilt the runtime and launched
  the Electron renderer on `http://localhost:5175/`; Playwright opened the
  renderer, toggled the right "切换置顶摘要" panel, selected an existing durable
  subagent thread, observed summary rows with `childThreadId`, `canOpenThread`,
  cache, token usage, and `deepseek-v4-pro` execution metadata, then clicked a
  subagent row and verified the child content opened through the side-panel
  child-thread path without switching the parent active thread. Screenshot:
  `output/playwright/analytix-summary-child-open.png`.

Do not absorb from Reasonix:

- public `reasonix` CLI identity as analytix product surface;
- Reasonix-native event shapes in renderer code;
- Reasonix-native settings/config as active analytix settings;
- terminal-only assumptions that remove desktop workflows;
- upstream-implied top-level AutoResearch, Workflow, Subagent, or terminal-style
  UI surfaces without an explicit analytix-native product spec;
- DeepSeek-only request, stream, usage, or cache assumptions;
- engine shortcuts that weaken approvals, sandbox, trace privacy, or user data
  durability.

## Kun Absorption Targets

Kun is the inherited product-workflow regression floor and a default product
research lens. It is not the exclusive UI/product source and not a future
capability ceiling. Absorb user workflows and product behavior through
analytix UI, analytix contracts, and current product identity.

| Target | What to absorb | Analytix landing | Proof that it is better |
| --- | --- | --- | --- |
| Code workbench workflow | Project binding, chat around code, tool approvals, file edits, reviewable changes. | Current analytix Code UI and runtime events. | Code workspace benchmark, tool approval tests, file-change/review UI QA. |
| Write workspace | Markdown workspace, file tree, preview/export, inline completion, selected text actions. | Analytix Write route and settings. | Write workflow benchmark and export/inline completion tests. |
| SDD / requirement-first flow | Requirement draft, design/prototype, plan, todo, review, requirement history. | Analytix SDD route and plan/review contracts. | SDD benchmark: draft -> plan -> work -> review remains linked and restorable. |
| Connect Phone / IM entry | Feishu/Lark/WeChat style task entry, webhook/relay, manual and recurring tasks. | Analytix Connect Phone user-facing copy; `claw` only as internal compatibility where allowed. | Connect Phone and schedule benchmarks, thread reuse tests, no user-visible old naming. |
| Schedule | One-time and recurring task creation, runtime thread reuse, task context persistence. | Analytix schedule settings and runtime thread contracts. | Schedule benchmark and runtime thread reuse fixtures. |
| Workflow / Loop automation | Do not absorb Kun `Create Loop` / Loop creation as implied parity or an unreviewed top-level entry. Workflow automation ideas from any source may be reconsidered through a separate analytix-native proposal. | No current automatic product surface. Future route/sidebar/command/workbench placement requires explicit spec approval, UX rationale, route-surface tests, and desktop QA. | Current scans prove no unapproved Create Loop entry or Kun identity/bridge/schema leak; future approved exposure must update those scans deliberately. |
| MCP and Skills | Task-specific tools and project/global skill loading. | Analytix plugin/skills marketplace and runtime tool provider. | Marketplace QA, skill loading tests, tool contract tests. |
| Git checkpoint and tray/session utilities | Git checkpoint service, branch/worktree pickers, tray session menu, project grouping. | Analytix desktop project/session utilities and review safety layer. | Git checkpoint tests, tray/session QA, branch picker and worktree grouping tests. |
| Multimodal/media capabilities | Images, vision, speech, image/audio/video generation where providers support it. | Provider capability model and renderer attachment contracts. | Attachment `localFilePath`/`FilePath` propagation tests and provider capability tests. |
| Provider presets | DeepSeek, Xiaomi MiMo, MiniMax, OpenAI-compatible provider presets and UX. | Analytix provider settings and connection probe. | Provider preset tests, request contract tests, sanitized error guidance. |
| Local HTTP/SSE runtime workflow | Shared runtime boundary for Code/Write/Connect Phone/schedule. | `packages/runtime-go` production runtime plus `packages/runtime` public contracts/launcher. | Runtime conformance, SSE replay, thread/fork/archive/search/usage parity tests. |
| GUI command details | Composer commands, model picker, reasoning controls, plan/review/new/fork/archive entries. | Analytix upgraded UI, not upstream UI replacement. | Component tests and desktop QA showing commands remain reachable. |
| Legacy import lessons | Migration/import edge cases and old user data awareness. | Explicit legacy-kun import/migration boundary. | Idempotent migration/import tests and no active old settings writes. |

Do not absorb from Kun:

- old product identity, `kun serve`, `window.kunGui`, `agents.kun`, old release
  identity, or current-product Kun naming;
- Kun `Create Loop` / Loop creation as an unreviewed analytix feature or entry
  point; future analytix-native workflow automation requires explicit spec
  approval;
- Kun UI patterns, screens, navigation entries, or component behavior without
  explicit approval before implementation;
- old UI layout when it conflicts with upgraded analytix UI;
- removed provider switchers, old runtime-control panels, or old process
  managers;
- shortcuts that bypass thread projection, virtualizer, frame scheduler, or
  desktop QA;
- changes that only match Kun parity but do not fit analytix product direction.

## OpenCode Comparison Targets

OpenCode is an auxiliary comparison source. Use it with Reasonix, Hermes Agent,
Kun, CodexDesktop-Rebuild, and current analytix to decide which design is best
for an agent capability.

| Target | What to compare | Analytix landing | Proof that it is better |
| --- | --- | --- | --- |
| Provider registry breadth | Provider/model inheritance, explicit provider misses, metadata persistence. | Shared provider contracts, `ModelExecutionRef`, and child task inheritance. | Provider route tests prove explicit misses do not silently fall back, and child runs keep provider/model/endpoint attribution. |
| Sub-agent permissions | Permission derivation for delegated tasks. | Analytix approval/sandbox/user-input contracts and child thread metadata. | Denied no-execute, inherited policy, headless child, and renderer summary fixtures pass. |
| TODO/task semantics | Task/TODO state, continuation, completion, and background metadata. | Goal, Plan, task-job, evidence ledger, and thread-summary contracts. | Restart/resume, parent-child lineage, TODO evidence, and output/action tests pass. |
| Session continuation | Continuation through metadata rather than prompt transcript injection. | Durable child-thread reuse/fork semantics. | Cache benchmark proves no previous-transcript prompt injection; summary route can reopen child threads. |
| Background work | Output, kill, restart, durable status, and action visibility. | Go runtime task-job/thread-summary routes; Electron keeps metadata-only compatibility projection. | Fixtures prove Go is the sole effect authority, Electron output is always withheld, raw output/reasoning is not persisted by new writes, and legacy stores remain read-only until journal migration. |

Do not absorb from OpenCode:

- OpenCode identity, public protocol, or UI shell;
- weaker permission defaults than analytix;
- TODO/task semantics that bypass Goal, Plan, evidence, or thread projection;
- provider fallback behavior that hides explicit configuration errors;
- continuation patterns that reduce DeepSeek cache stability.

## CodexDesktop-Rebuild Reference Targets

CodexDesktop-Rebuild is the default desktop-feel reference. It is not the only
source of desktop architecture or interaction ideas. It should improve the
analytix experience, not replace the analytix product.

| Target | What to absorb | Analytix landing | Proof that it is better |
| --- | --- | --- | --- |
| Thread virtualizer | Bottom-distance preservation, measured rows, scroll controller behavior. | Analytix-derived `ThreadVirtualizer` and message timeline scheduler. | Long-thread desktop QA proves no forced scroll and no row overlap while streaming. |
| Worker split | Runtime, markdown, code highlight, diff, and heavy panel work off the hot render path. | Renderer worker boundaries and projection buffers. | Flame/trace or Playwright evidence shows lower frame cost during streaming and scrolling. |
| App-server facade | Stable UI-facing state boundary and connection signals. | Analytix runtime client/facade above HTTP/SSE. | Reconnect, resume, SSE replay, and offline/online UI state tests pass. |
| Composer responsiveness | Controller/view split, command/menu latency hiding, attachment state isolation. | Floating composer controller and renderer store boundaries. | Typing, command menu, attachment, and submit QA stays responsive under streaming load. |
| Interaction rhythm | Motion, loading, icon, panel, empty-state, and density lessons. | Analytix visual tokens and owned assets. | Desktop screenshots/QA prove improved polish without Codex identity or workflow loss. |

Do not absorb from CodexDesktop-Rebuild:

- Codex product identity or implementation ownership naming;
- minified chunks as production source modules;
- visual changes that erase analytix information density or workflows;
- desktop feel changes without real screenshot, trace, or Playwright evidence.

## Hermes Agent Reference Targets

Hermes Agent is a self-improving-agent and cross-channel operations reference.
It may also inform sub-agent, tool RPC, scheduling, and product operations when
evidence beats other sources. Use it carefully; most ideas need analytix-native
redesign before code lands.

| Target | What to absorb | Analytix landing | Proof that it is better |
| --- | --- | --- | --- |
| Closed learning loop | Memory nudges, skill creation from experience, skill self-improvement, session search. | Analytix Skills, memory, thread search, and opt-in learning controls. | Privacy/audit tests plus task-reuse benchmark prove improvement without hidden memory mutation. |
| Cross-channel gateway | Delivery, interruption, continuity, and scheduled output ideas. | Connect Phone, schedule, and runtime thread reuse. | Connect Phone/schedule QA proves durable thread continuity and approval/user-input safety. |
| Sub-agent parallelism | Isolated parallel workstreams and tool RPC ideas. | Task-job, child thread, approval, and tool-host contracts. | Three-way Reasonix/OpenCode/Hermes comparison plus restart/lineage/cache fixtures pass. |
| Terminal backends | Local/Docker/SSH/serverless backend portability ideas. | Analytix tool-host and sandbox abstractions. | Safety tests prove sandbox, redaction, output capture, and desktop registry compatibility. |
| Trajectory compression | Batch trajectory and compression ideas for agent-quality research. | Agent benchmark datasets and scorecards, not default UI. | Dataset generation is redacted, opt-in, reproducible, and linked to benchmark improvements. |

Do not absorb from Hermes Agent:

- Hermes CLI identity, config shape, public memory protocol, or messaging
  identity;
- autonomous memory/skill mutation without analytix opt-in and audit controls;
- gateway behavior that bypasses Connect Phone, schedule, approvals, or
  thread/session durability;
- terminal backend behavior that weakens sandbox, redaction, or trace privacy;
- trajectory collection without a privacy review and benchmark purpose.

## Claude Code Behavior Targets

Claude Code is a proprietary behavior baseline, not a code source. Analytix may
independently specify and test durable attach/resume, background-agent UX,
remote-control security, enterprise managed settings, plugin lifecycle, and
layered review behavior. It must not copy Claude Code code, prompts, plugin or
documentation text, binaries, or assets without separate written authorization.

## Claw Code Reference Targets

Claw Code is a low-confidence museum/reference source. Use verified code and
tests only for small lifecycle, cancellation, workspace, or Rust fixture ideas.
Do not treat its in-memory task/team/cron registries or conflicting parity
tables as production evidence, and do not adopt shell-string heuristics as an
Analytix approval or sandbox boundary.

## Gajae Code Reference Targets

Gajae is valuable for coordinator/RPC/ACP contract design, session trees and
handoff, plugin compile-before-copy and drift quarantine, notification SDKs,
durable receipts, and native scan/AST/PTY benchmarks. Direct reuse remains
`blocked-provenance` until its pi-mono/oh-my-pi and vendored copyright chains
are resolved. Analytix should contract-reimplement useful behavior through Go,
Hub, Connect Phone, and existing HTTP/SSE boundaries rather than importing Bun
or GJC product protocols.

## LazyCodex Reference Targets

The committed LazyCodex plugin tree is a real code source for LSP/CodeGraph,
team/worktree, bootstrap, continuation, rules, and evidence-verification ideas.
The OmO `src/` gitlink was not initialized in this audit and is not admitted.
Prefer LSP/CodeGraph as optional Hub/MCP plugins, port realpath/symlink/non-empty
evidence gates into Analytix receipts, and reject opt-out telemetry or broad
default prompt growth.

## Current Construction Packets

The current all-source audit replaces old commit-specific priority lists for
new work:

| Priority | Packet | Main comparison sources | Required first proof |
| --- | --- | --- | --- |
| P0 | Atomic persistence, revision/CAS, cross-process lease, recovery branch, event index and bounded replay | Reasonix, OpenCode, Analytix | Persisted-data migration spec plus crash, contention, Windows replace and replay tests. |
| P0 | Secure defaults, production-config truth and artifact hygiene | Accepted `secure-runtime-execution-defaults`, `production-config-truthfulness`, `repository-evidence-hygiene`, and `windows-qa-operator-access` specs | Fresh settings migration, Go policy, secret storage, config-truthfulness, and repository-history verification. |
| P1 | Context Epoch foundation | Accepted `context-epoch` capability from archived `context-epoch-foundation`, OpenCode, Reasonix, Hermes | Default request body/prefix/tool hashes remain byte-identical; focused default/inactive/active/compact/restart/corruption fixtures and the speed/cache gate pass. |
| P1 | Durable input inbox/coordinator | OpenCode, Reasonix, Hermes | Idempotent queue/steer replay, ownership, wake-up, restart, and cross-epoch rejection proof; this remains a separate consumer of Context Epoch. |
| P1 | Compaction integrity, observers, searchable memory and Connect Phone recovery | Hermes, Gajae, Reasonix | Tool-pair/multimodal/retry/recovery, redaction, FIFO/dedupe and opt-in tests. |
| P1 | MCP OAuth/resources/prompts and LSP/CodeGraph | OpenCode, Kun, Lazy, Gajae | OS secret/trust tests, protocol fixtures, plugin lifecycle and schema/token benchmark. |
| P1/P2 | Design workflow, advanced auth, offline speech and external control | Kun, Gajae, Claude behavior | Separate product/security specs and real Electron/platform QA. |

Context Epoch dependency status: the foundation implementation is validated,
but it does not automatically admit a consumer. Memory retrieval, instruction
discovery, post-compact content recovery, tool-output budgeting, hooks,
plugins/skills, MCP-provided context, subagent inheritance, and any UI
diagnostics must cite the Context Epoch hard-gate evidence and add their own
bounded reader, permissions, same-prompt request-shape, redaction, and failure
fixtures. Until then those consumer rows remain blocked or separately scoped.

## Proof Patterns

Use these proof patterns to decide whether an absorption is better.

| Claim | Required proof |
| --- | --- |
| "Better product workflow than Kun" | Analytix supports the Kun workflow, keeps upgraded UI, passes product workflow benchmark, and has no identity/schema regression. |
| "Better engine than Reasonix for absorbed area" | Analytix reaches Reasonix parity for the target engine fixture, then improves success, cost, latency, reliability, or desktop integration. |
| "Better DeepSeek/cache behavior" | Fixture evidence proves stable prefix/tool hashes, provider attribution, cache hit/miss parsing, and unsupported fallback; live provider evidence is required before claiming final cache superiority. |
| "Better DeepSeek speed than Reasonix" | Measured time-to-first-token, tokens/sec, end-to-end latency, provider request/stream parity, cache hit-rate by stable prefix hash, and desktop streaming frame cost beat the Reasonix baseline. |
| "Better tool/MCP behavior" | Tool lifecycle fixtures pass, approvals remain stricter, file/image results reach transcript and UI. |
| "Better sub-agent/TODO behavior" | Reasonix/OpenCode/Hermes comparison is recorded, task-job and Goal contracts own the landing, permissions remain stricter, restart/lineage/cache/render fixtures pass. |
| "Better learning loop than Hermes ideas" | Memory/skill changes are opt-in, auditable, private, and benchmarked for task reuse without hidden protocol or product identity leaks. |
| "Better Go runtime" | The current Go-only production runtime passes fresh conformance and improves at least one intended engine dimension without weakening public contracts or desktop workflows. |
| "Better desktop experience" | Real Electron QA or renderer smoke proves long-thread, streaming, composer, terminal, and panel behavior did not regress. |
| "Better maintainability" | Sync ledger, conflict decision, contract diff, tests, and scorecard all match. |

## Historical 2026-06-20 G0/G5 Runtime Conformance Inventory

This inventory preserves the pre-cutover bridge between upstream absorption and
the then-future Go runtime work. It did not start a Go backend. Current
production is Go-only; the rows below are historical oracle evidence, not
current construction instructions.

| Source | Absorb what | Why it is better | Proof required | Conflict authority |
| --- | --- | --- | --- | --- |
| Kun 0.2.13 baseline | Product-lineage behavior for Code/Write/SDD/Connect Phone/schedule, reviewable file changes, approvals, generated files, provider presets, and desktop command reachability. | Ensures analytix did not regress the user workflows it inherited while migrating identity/runtime contracts. | P1.1-P4C regressions, renderer/main focused tests, desktop smoke/QA evidence. | Specs 01-09, D-0001, D-0002. |
| Kun 0.2.14/current | Scoped stable/current drift already absorbed or corrected: provider routing, Telegram Connect Phone, hidden Create Loop internals, checkpoint/rewind safety, Windows installer process-stop, and desktop utility lessons. | Keeps analytix current without restoring Kun identity, UI shell, settings/bridge structures, or unapproved top-level Workflow navigation. | Kun sync ledger, absorption ledger, scorecard, full runtime suite, route-surface tests, identity scan. | D-0006, D-0008, D-0009, D-0012. |
| Reasonix main-v2 | Engine/runtime ideas: cache diagnostics, provider/stream parsing rigor, MCP/tool lifecycle, approval safety, checkpoint/rewind semantics, five-batch agent-kernel route, and future Go lock guidance. | Gives analytix stronger runtime evidence while keeping desktop and HTTP/SSE contracts backend-neutral and permission/evidence/Goal gates first. | Reasonix sync ledger, Go prefix/provider/usage/MCP/tool/checkpoint tests, speed/cache gate, agent-kernel scorecard rows, and current source audit. | D-0003, D-0004, D-0007, D-0009, D-0013, D-0014. |

The TypeScript oracle surfaces to keep frozen before Go work are:

```text
thread/session
SSE replay
checkpoint/rewind plan/apply
safety audit
tool calls
approvals
user input
plan/goal
model-history repair
cache accounting
usage
provider request/stream parsing
```

G5 may not claim parity until the frozen fixtures and the missing production
evidence listed in `go-runtime-conformance.md` are satisfied.

## Release Claim Gate

Do not say analytix is stronger than the relevant upstreams unless the current
release report can point to:

```text
docs/analytix/upstreams/kun-sync.md
docs/analytix/upstreams/reasonix-sync.md
docs/analytix/upstreams/opencode-sync.md
docs/analytix/upstreams/codexdesktop-rebuild-sync.md
docs/analytix/upstreams/claude-code-sync.md
docs/analytix/upstreams/claw-code-sync.md
docs/analytix/upstreams/gajae-code-sync.md
docs/analytix/upstreams/hermes-agent-sync.md
docs/analytix/upstreams/lazycodex-sync.md
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/conflict-decisions.md
docs/analytix/upstreams/upstream-sources.json
docs/analytix/upstreams/upstream-capability-audit-2026-07-10.md
docs/analytix/upstreams/code-reuse-provenance.md
docs/analytix/benchmarks/upstream-scorecard.md
docs/analytix/benchmarks/benchmark-scenarios.md
docs/analytix/benchmarks/quality-gates.md
docs/analytix/qa/
```

and show:

- what was absorbed;
- what was rejected;
- what was redesigned;
- which benchmark dimensions improved;
- which known gaps remain.

## Initial Audit

This matrix closes the practical gap left by pure governance docs:

```text
Spec 08 says how to absorb safely.
Spec 09 says how to measure strength.
This file says what to absorb and how each target proves value.
```
