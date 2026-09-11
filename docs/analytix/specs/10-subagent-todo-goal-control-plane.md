# analytix subagent, todo, goal, and background control-plane spec

Status: Normative control-plane contract with implemented-slice history.

Currentness note (2026-07-10): the current worktree includes Phase 11A scoped
child todo projection and Phase 12A explicit mapped-completion acceptance.
Earlier statements that defer all scoped child todos are superseded by Sections
7.4 and 7.5, the implementation table below, and the final `Still deferred`
list. Child projections still cannot create parent todos, merge evidence into a
parent goal automatically, or complete a parent goal.

## 1. Purpose

This spec defines the best-of-breed upgrade path for the analytix Go runtime
subagent, todo, goal, plan, background job, agent profile, permission, and
desktop UX system.

The target is not a wholesale import from any upstream. analytix keeps the
current Go runtime evidence-backed core, product identity, `window.analytix`
bridge, and top-level runtime settings. Upstream ideas are absorbed only through
analytix-owned contracts, tests, and renderer surfaces.

## 2. Upstream Roles

| Upstream | Research role and reuse posture |
| --- | --- |
| DeepSeek-Reasonix | Evidence-backed `todo_write` / `complete_step` / goal closure, read-only child boundary, transcript continuation, plus session lease/atomic persistence/meta-CAS/recovery ideas. Direct reuse requires file-level MIT provenance and notice. |
| opencode | `primary` / `subagent` / `all` agent modes, permission inheritance, child session identity, resumable task id UX. |
| gajae-code | Long-task control-plane vocabulary, spawn gate, fork-context and isolation design, plus session-tree/handoff and external-control contracts. Direct reuse remains blocked until inherited copyright provenance is resolved. |
| hermes-agent | Read-only observer/middleware lifecycle, correlation/redaction/fail-open semantics, `SessionSource`, restart recovery, FIFO delivery, compaction integrity, searchable state, and environment abstraction. Its module-level background registry is reference material, not a replacement for Analytix durable task jobs. |
| Kun | Desktop-friendly profile metadata, `toolPolicy`, child-run GUI events, plan/todo synchronization. |
| CodexDesktop-Rebuild | Desktop presentation, worker/event-boundary, and interaction behavior reference only. The reviewed repository has no project-wide license and ignored extracts are not source evidence, so no control-plane code or asset reuse is admitted. |
| claude-code | Proprietary public behavior baseline for subagent/background/plan/todo UX compatibility; clean-room only absent written authorization. |
| claw-code | Low-confidence/museum behavior and small verified Rust lifecycle fixtures. Its Task/Team/Cron registries are in-memory and it has no production worker fleet, so parity prose and heartbeat/status names are not durability evidence. |
| lazycodex | The committed Codex plugin snapshot is a real source for LSP/CodeGraph MCP, team/worktree lifecycle, bootstrap, continuation/rules, and evidence verification. The OmO core gitlink was not initialized/reviewed and contributes no admitted evidence. |

## 3. Target Contracts

### 3.1 Agent Profile and Mode

The runtime subagent profile contract is:

```text
mode: primary | subagent | all
hidden: boolean
description: string
model/providerId/endpointFormat/variant/effort overrides
toolPolicy: readOnly | inherit
allowedTools / blockedTools
blockedMcpServers / blockedSkills
promptPreamble / systemPrompt
maxSteps / tokenBudget / timeBudgetMs
color / icon / name
```

Rules:

- `readOnly` remains the default. `inherit` is allowed only from explicit user
  input, a profile, or a future safety policy.
- `primary` profiles are not advertised as `delegate_task` targets.
- `subagent` and `all` profiles may be advertised to the parent.
- `hidden` profiles are not shown in ordinary tool descriptions, but may be used
  by internal workflows.
- Profiles can narrow tools, prompt, model, and budgets; they cannot override
  the runtime safety floor.
- Subagents do not receive subagent, job, goal, todo, plan, or user-input tools
  unless a future slice implements safe recursive depth and budget controls.

### 3.2 Subagent Execution

The existing `delegate_task`, `task`, `parallel_tasks`, and `run_skill`
contracts remain compatible.

The normalized task result JSON is:

```text
kind/status/childId/childRunId/childThreadId/jobId/label/name
summary/error/evidence/usage/profile/toolPolicy/toolScope/toolInvocations
durationMs/queuedMs/background/parallelGroupId/parallelIndex
returnFormat/topLevelSubagentRouteExposed
```

Rules:

- Foreground child runs block until completion and return summary.
- Background child runs return `jobId` immediately and remain controllable.
- `parallel_tasks` runs a dependency DAG and returns
  `parallelGroupId/taskCount/tasks/topLevelSubagentRouteExposed:false`.
- `continue_from` and `fork_from` require strict source compatibility.
- Dependency summaries are injected only as bounded text.
- `returnFormat` may request `summary`, `evidence`, or `transcriptRef`.
- `label` is the UI title; `prompt` remains the child assignment.

### 3.3 Background Control Plane

The runtime must keep `wait`, `bash_output`, and `kill_shell` compatible.
Additional control-plane fields and aliases may be added only when backed by
real runtime behavior.

Supported first-class operations:

- list jobs for a parent thread;
- wait for jobs;
- read output with `offset`, `cursor`, `since`, `limit`, and tail behavior;
- kill jobs and persist terminal status;
- expose heartbeat/stale diagnostics from persisted timestamps and output size;
- report queued/running/completed/failed/killed/interrupted/aborted states;
- include duration, queued time, usage, profile, tool policy, and tool count.
- queue bounded steer messages for running background child jobs, admit them
  only at child turn safe boundaries, and audit queued/admitted/rejected state.
- cooperatively pause and resume background child jobs at safe child-turn
  boundaries without interrupting in-flight provider streams or tool execution.
- review isolated child worktree diffs and record explicit parent reject /
  cleanup decisions without applying merges.
- accept reviewed clean isolated child worktree diffs only after explicit
  parent approval and clean-patch preflight.
- report conflicted isolation accepts, dry-run bounded parent-provided repair
  patches, and apply a clean repair patch only after explicit parent approval.

Deferred operations:

- foreground child-run steer;
- advanced worktree conflict resolution UX and fast-apply proofs after parent
  HEAD advances.

These are schema and design candidates until the runtime can safely coordinate
foreground event loops and reviewed filesystem merge.

### 3.4 Todo, Goal, Evidence

The current evidence-backed core is preserved:

- `todo_write` remains complete-list replacement.
- Todos remain capped at 200 items with at most one `in_progress`.
- `complete_step` requires evidence.
- `update_goal status=complete` requires evidence ledger and completed todos.
- `update_goal status=blocked` requires the same reason across three goal turns.
- Subagents do not write parent todos by default.
- A child may return an evidence bundle, but the parent decides whether to call
  `complete_step`.
- Child evidence can close parent goal steps only when host-verifiable receipt
  provenance is recorded.

Implemented follow-up:

- `todo_patch` / `todo_ops` for append/start/done/drop/note on parent-owned
  todos.

Implemented follow-up:

- scoped child todo lists and parent-visible projections (Phase 11A);
- explicit, approved acceptance that can complete only an existing mapped
  parent todo after parent revision validation (Phase 12A).

Still deferred:

- plan markdown checkbox sync that merges `source=plan` without overwriting
  manual todo state;
- automatic parent todo creation and automatic child-evidence/parent-goal
  merge.

### 3.5 Context Budget

Runtime state that changes every turn must not enter the stable cache prefix.

Rules:

- Parent context receives compact child summaries, not full transcripts.
- Dependency summaries are bounded.
- Active todos are injected as incomplete items plus a small recent-completed
  tail.
- Background jobs are injected as compact status notes.
- Provider request assembly may dynamically include only the schemas relevant
  to a routed turn so ordinary requests stay bounded. This narrows one provider
  payload, not product capability: the one Agent's ordinary code, file, shell,
  Git, Plan, Todo, thread, subagent, compaction, Skills, and ordinary MCP
  abilities remain available and discoverable, and case authority cannot
  remove them or create a `caseMode`/`generalMode` tool switch.

### 3.6 Desktop UX

Renderer support must remain on `window.analytix` and existing thread/SSE
contracts.

The desktop should show:

- parent timeline child task cards;
- open-child-thread affordances;
- background job status and output diagnostics;
- collapsible parallel groups;
- todo/goal state;
- evidence failure reasons that are understandable to the user.

The first slice should reuse existing `SubagentCallCard`, thread summary, todo
panel, and plan/todo sync components rather than redesigning the workbench.

## 4. Implementation Slices

| Slice | Status | Scope |
| --- | --- | --- |
| 1. Profile schema and routing | Implemented | Profile mode/hidden/description/execution override/tool narrowing/budget/UI metadata are runtime-owned. Tool descriptions advertise only visible `subagent`/`all` profiles, primary profiles cannot be delegated, invalid configured modes fail validation, and child depth filters subagent/job/goal/todo/plan/user-input tools. Default policy remains `readOnly`. |
| 2. Task result contract | Implemented | Foreground/background/parallel task results carry `kind`, `status`, `childId`, `childRunId`, `childThreadId`, `jobId`, profile metadata, `returnFormat`, usage, tool count, duration, queue time, and `topLevelSubagentRouteExposed:false`. `parallel_tasks` keeps DAG dependency handling and bounded dependency summaries. |
| 3. Background registry and heartbeat | Implemented | `list_jobs`, `wait`, `bash_output`, and `kill_shell` share durable task-job records. Output supports offset/cursor/since/tail limits; kill persists terminal state; diagnostics expose heartbeat/stale/output/status fields. Desktop IPC now allows the canonical `/v1/runtime/task-jobs/list` path. |
| 4. Evidence bundle | Implemented for return material | Child final answers can surface structured evidence bundles in tool results. Timeline events expose only `evidenceBundleStatus` and `evidenceCount`, not full evidence. Parent goal/todo completion is not automatic and still requires host-verified evidence receipts. |
| 5. Todo operations and plan sync | Partially implemented | `todo_write`, goal completion, blocked gating, and existing plan/todo behavior are preserved. `todo_ops` and `todo_patch` apply ordered append/start/done/drop/note operations through the same todo store and `todos_updated` event. Phase 11A/12A add scoped child projections and explicit mapped-completion acceptance; richer plan markdown merge, automatic parent todo creation, and automatic evidence/goal merge remain deferred. |
| 6. Renderer UX | Implemented first-stage closure | Existing child cards, process rows, runtime chips, and parallel groups now show job id, profile/mode, tool policy, return format, budgets, evidence parse status, heartbeat/stale diagnostics, and child-thread navigation when `childThreadId` is present. |
| 7. Subagent steer queue | Implemented Phase 3 slice | `/v1/runtime/task-jobs/steer` lets the parent thread send a bounded message to its own running background child job. Messages are stored on the durable job record, injected into the child turn steering queue only when child lineage exists, promoted at provider/model boundaries, and surfaced through child steer events and the existing child card UI. No model tool is exposed. |
| 8. Cooperative pause/resume | Implemented Phase 4 slice | `/v1/runtime/task-jobs/pause` and `/v1/runtime/task-jobs/resume` let the parent request a background child pause, admit the pause only at child loop safe boundaries, persist `pause_requested`/`paused`/`resume_requested`/`resuming`, and resume the active child runner without changing tool policy, tool scope, approval, or sandbox state. No model tool is exposed. |
| 9. Worktree isolation metadata | Implemented Phase 5 slice | Background child jobs may request `isolationMode=worktree` only with explicit/profile `toolPolicy=inherit`. The runtime rejects dirty parents, creates an analytix-owned Git worktree, runs the child in that workspace, and persists branch/path/base/current commit, changed files, diff summary, and `mergeStatus=not_requested`. No merge is applied. |
| 10. Isolation review and cleanup | Implemented Phase 6/7 slice | Parent-owned reviewed isolated child jobs can be inspected, rejected, and cleaned up through explicit runtime endpoints. Review and cleanup validate durable ownership and analytix-owned worktree/branch metadata; renderer shows inspect/reject/cleanup but no accept merge control. |
| 11. Isolation accept clean diff | Implemented Phase 8 slice | Parent-owned reviewed clean isolated child jobs can be accepted through explicit approval. Runtime requires clean parent workspace, matching base commit, non-conflicted changed files, dry-run patch success, and durable `AcceptDecision`; no auto-cleanup or conflict resolution is performed. |
| 12. Accepted isolation lifecycle | Implemented Phase 9 slice | Accepted isolated child jobs keep audit receipts after patch apply and can be manually cleaned through the existing cleanup endpoint. Cleanup links to the latest `AcceptDecision`, removes only durable analytix-owned worktree/branch records, never touches the parent workspace, and remains explicit rather than automatic. |
| 13. Conflicted repair workflow | Implemented Phase 10 slice | Conflicted isolated accepts can produce a read-only `ConflictReport`; parent-provided repair patches can be dry-run checked and, only with explicit approval, applied to a clean parent workspace with matching `HEAD`. Repair never auto-merges, silently resolves conflicts, commits, or cleans up worktrees. |
| 14. Scoped child todo projections | Implemented Phase 11A slice | Parent-owned terminal child jobs can expose durable child todo lists, create parent-visible proposal projections, and reject proposals without mutating parent todos or goals. Evidence identifiers are preserved as review material. |
| 15. Explicit projection acceptance | Implemented Phase 12A slice | A parent can explicitly approve selected completed child todos to complete only existing todos referenced by `parentTodoRef`. Runtime validates parent ownership and todo revision, persists before/after digests and the decision receipt, and never creates parent todos or completes the parent goal. |

## 5. First-Stage Acceptance Matrix

| Area | Completed behavior | Verification |
| --- | --- | --- |
| Runtime tooling | Ordinary turns do not get subagent/job tools unless routed; subagents do not receive subagent/job/goal/todo/plan/user-input tools; `list_jobs` is filtered at child depth; profile visibility and read-only defaults are tested. | `packages/runtime-go/internal/app/toolcatalog/*_test.go`, `packages/runtime-go/internal/app/subagent/*_test.go` |
| Background jobs | Running/completed/failed/killed/interrupted/aborted states are durable; list/wait/output/kill return compact job views with heartbeat/stale/output diagnostics; output reads support bounded since/tail/cursor behavior. | `packages/runtime-go/internal/app/subagent/task_job_service_test.go`, `job_view_test.go`, server task-job tests |
| Evidence and goal safety | Child evidence is returned as material only; child events carry parse status/count; parent `complete_step` and `update_goal complete/blocked` gates remain evidence-backed. | `output_test.go`, existing goal/todo tests |
| Renderer contract | Child runtime metadata and job diagnostics preserve new fields through contract, mapper, chips, process rows, and locales. | `src/renderer/src/agent/*test.ts`, `src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts` |
| Desktop bridge | `window.analytix` remains the only renderer bridge. Runtime task job list/wait/output/kill paths are canonical `/v1/runtime/task-jobs/*` paths in shared endpoint constants and main IPC allow-list. | `src/shared/analytix-endpoints.test.ts`, `src/main/ipc/app-ipc-schemas.test.ts`, `runtime-client.test.ts` |
| Phase 3 steer | Parent-owned background child jobs can accept bounded steer messages. Completed/failed/killed/aborted/interrupted jobs reject steer; cross-parent requests reject; terminal state expires queued steer messages; renderer shows the affordance only for running/queued steer-capable child jobs. | `steer_test.go`, `lineage_test.go`, `steering_service_test.go`, `task_jobs_test.go`, mapper/runtime-client/card tests |
| Phase 4 pause/resume | Parent-owned running/queued background child jobs can request pause; the child enters `paused` only at provider/tool/queued safe boundaries; paused jobs are not marked stalled; kill still works; pending steer remains queued while paused and is admitted after resume. Renderer shows pause/resume controls only for eligible child jobs. | `pause_test.go`, `lineage_test.go`, `runtime_state_test.go`, task-job HTTP tests, mapper/runtime-client/card tests |
| Phase 5 worktree isolation | Explicit writable background child jobs can run inside analytix-owned Git worktrees. Read-only/default-policy/foreground requests are rejected; dirty parent workspaces are rejected; child changes are summarized and shown without auto-merge. | `worktree_test.go`, `requests_test.go`, `lineage_test.go`, `job_view_test.go`, mapper/card tests |
| Phase 6/7 isolation decision | Parent-owned isolated child worktrees can be reviewed, rejected, and cleaned after review/rejection. Cross-parent/non-isolated/active cleanup requests reject; cleanup removes only durable analytix-owned worktree paths and `codex/subagent/*` branches. No accept merge control is exposed. | `worktree_review.go` service tests, process worktree tests, task-job HTTP tests, mapper/runtime-client/card tests |
| Phase 8 clean accept | Parent-owned reviewed clean isolated child worktrees can be accepted with explicit approval. Cross-parent/non-isolated/unreviewed/conflicted/dirty-parent requests reject; clean patches apply to the parent workspace, receipts persist, and child worktrees are retained. | `worktree_review.go` service tests, process worktree tests, task-job HTTP tests, mapper/runtime-client/card tests |
| Phase 9 accepted cleanup | Accepted isolated child worktrees can be manually cleaned after an `AcceptDecision` exists. Cleanup rejects cross-parent/non-owned/active/already-cleaned records, does not require a clean parent workspace, preserves accepted parent changes, and records a `CleanupReceipt` linked to the accept receipt. | `worktree_review.go` service tests, process worktree tests, lineage tests, mapper/card tests |
| Phase 10 conflict repair | Conflicted isolated child worktrees can produce an audit-only conflict report. Repair check validates bounded patch size/files/paths and only dry-runs. Repair accept requires explicit approval, a clean parent workspace, matching parent `HEAD`, a clean repair review, and records `RepairDecision`; it does not cleanup or commit. | `worktree_review.go` service tests, process worktree tests, lineage tests, task-job HTTP tests, mapper/runtime-client/card tests |
| Phase 11A child todo projection | Parent-owned terminal child jobs can snapshot child todos into durable proposals; reject is idempotent; projection/reject never mutates parent todos or parent goals; renderer exposes compact proposal metadata. | `packages/runtime-go/internal/app/subagent/child_todos.go`, focused child todo/service/HTTP tests, renderer mapper/runtime-client/card tests |
| Phase 12A projection acceptance | Explicit approval plus a matching parent todo revision can complete only selected existing mapped parent todos. Decisions are durable and idempotent; skipped items are recorded; no parent todo creation, evidence merge, or goal completion occurs. | focused child todo acceptance/service/HTTP tests, shared endpoint and IPC tests, renderer mapper/runtime-client/card tests |

## 6. Deferred Items

- No old product name, runtime-control panel, bridge alias, or upstream public
  protocol.
- No recursive subagent spawning in child runs without a separate safe-depth
  implementation.
- No default `inherit` tool policy.
- No full child transcript injection into parent turns.
- No whole-workspace formatting or unrelated renderer redesign.
- Foreground child-run `steer` / `send_message` is deferred because a blocking
  foreground run does not yet have a separate parent-side event loop admission
  surface.
- Advanced worktree conflict resolution and fast-apply after parent `HEAD`
  advances are deferred because they need richer conflict UX and additional
  filesystem safety tests.
- Plan-checkbox merge semantics remain deferred. Scoped child projections and
  explicit mapped-completion acceptance are implemented; automatic parent todo
  creation and automatic evidence/goal merge still require separate ownership
  and conflict rules.
- `blockedSkills` currently remains a profile metadata/blocklist contract while
  child agents do not receive skill tools in this slice; enforcement becomes
  active only if skill tools are safely exposed to children later.

## 7. Second-Stage Real Ability Design

Second-stage work must not add placeholder buttons, schemas, or endpoints. A
feature is implementable only when its runtime admission, persistence, eventing,
and tests are present in the same slice.

### 7.1 Subagent Steer / Send Message

Goal: allow a parent run to send a bounded steering message to a running child
run without racing the child's current model/tool turn.

Implemented Phase 3 runtime contract:

- Parent messages are appended to the durable task-job record as
  `SteerMessage` with `parentThreadId`, `childRunId`, `jobId`, source turn/call
  provenance, created/admitted timestamps, and `queued|admitted|rejected|expired`
  status.
- `/v1/runtime/task-jobs/steer` accepts only parent-owned running/queued
  background child jobs with child lineage. It rejects terminal jobs,
  cross-parent requests, foreground child runs, missing lineage, oversized
  messages, and per-child pending-queue overflow.
- The child run consumes steer through the existing turn steering queue. If the
  child turn is already bound, the message is admitted to the durable child turn
  as pending steering; otherwise it remains queued on the job until
  `startRuntimeTurn` binds child thread/turn lineage.
- Provider/model request preparation promotes pending turn steering at a safe
  boundary and records the job steer as `admitted`. It does not inject into an
  in-flight model request or host tool execution.
- Terminal job updates expire any still-queued steer messages so killed/failed
  children do not retain stale pending state.
- Renderer events use `child_steer_queued`, `child_steer_admitted`, and
  `child_steer_rejected` with compact child metadata. The full steer text is not
  copied into parent timeline metadata.
- No `steer_child` model tool is exposed in this slice. Steering is HTTP /
  desktop UI only, so child agents cannot recursively call it.

Implemented tests:

- valid running background child steer queued;
- steer admission metadata returned at provider safe boundary;
- completed/terminal child steer rejected;
- cross-parent, missing job/lineage, too-long message, and pending-queue limit
  rejected;
- terminal job update expires queued steer messages;
- child steer event payload contains `steerMessageId`, `jobId`, `childRunId`,
  and compact child metadata;
- shared endpoint, IPC allow-list, runtime client, mapper, and child-card
  affordance tests.

### 7.2 Cooperative Pause / Resume

Goal: pause a background child run safely without pretending to suspend an
in-flight model request or host tool process.

Required runtime contract:

- Pause requests are durable records on the child job, not process signals.
- The durable job status model includes `running`, `pause_requested`, `paused`,
  `resume_requested`, `resuming`, and terminal statuses
  `completed|failed|killed|aborted|interrupted`.
- `PauseRequest` records `id`, `parentThreadId`, `childRunId`, `jobId`,
  `status=requested|paused|rejected|expired|resumed`, requested/paused/resumed
  timestamps, rejected reason, and source turn id.
- The resume guard is host-side and durable: a `ResumeToken` record carries
  `resumeToken`, `issuedAt`, `expiresAt`, `childRunId`, and `parentThreadId`.
  The active runner accepts resume only when parent ownership, job state, and
  current pause request match the persisted token guard.
- The loop checks for pause at safe points: before a model request, after a
  model response is persisted, and after tool results are persisted before the
  next model turn. A queued child can pause before entering the model loop.
- A running host tool is never interrupted by pause. An in-flight provider
  stream is never interrupted by pause; a pause request waits until the stream
  completes and the loop reaches the next safe boundary.
- Kill has higher priority than pause/resume. Kill from paused moves directly
  to `killed`, invalidates the resume token, and unblocks the paused runner by
  cancelling its context.
- Pause/resume never changes `toolPolicy`, `toolScope`, approval policy, or
  sandbox mode.
- Paused children do not consume pending steer. Resume unblocks the child, then
  the normal steering promotion boundary admits pending steer in FIFO order.
- Stale paused jobs surface through heartbeat diagnostics with
  `paused=true`; paused jobs are not classified as stalled and do not
  auto-resume.
- Renderer events use `child_pause_requested`, `child_paused`,
  `child_resume_requested`, `child_resumed`, and `child_pause_rejected` with
  compact child metadata. No prompt/evidence transcript is copied into pause
  metadata.

Required tests:

- pause requested during model turn takes effect at the next boundary;
- pause requested during tool execution waits for tool result persistence;
- running/queued child pause request is accepted, terminal and cross-parent
  requests reject, and missing active child control rejects without pretending
  to pause;
- resume restores a paused active child to runnable state through the host-side
  resume token guard;
- kill from paused is terminal and cannot resume;
- stale paused job is listed with heartbeat diagnostics.
- paused child does not admit pending steer until resume.

### 7.3 Writable Isolation / Worktree Merge

Goal: let an explicitly writable child work in an isolated worktree, then let
the parent decide whether to merge the result.

Phase 5 target: make writable children isolated before making them mergeable.
The first production slice creates and observes isolated worktrees, but does
not auto-merge or silently resolve conflicts.

Isolation modes:

- `none`: current behavior. Child uses the parent workspace and is usually
  `readOnly`.
- `worktree`: create a Git worktree at a clean base commit and run the writable
  child there.
- `container` / `fs_snapshot`: future modes only. They require a separate
  sandbox lifecycle and are not part of Phase 5.

Child worktree lifecycle:

1. `create`: validate parent ownership, profile/tool policy, git availability,
   base commit, and dirty-tree policy; create an analytix-owned worktree.
2. `run`: child receives the isolated workspace path and cannot write the
   parent workspace.
3. `summarize_diff`: on completion or failure, collect changed files, diff
   stats, base/current commit, and child-reported tests.
4. `request_merge`: child job enters a merge-review state visible to the
   parent; no changes are applied yet.
5. `accepted`: parent explicitly accepts a clean diff or merge plan.
6. `rejected`: parent rejects the result; the worktree remains available for
   inspection until cleanup.
7. `cleaned`: only analytix-owned branches/worktrees are removed, and only
   after receipts have been persisted.

Branch and worktree naming:

- Branch namespace: `codex/subagent/<parentThread>/<jobId>`.
- Worktree path should be under an analytix-owned worktree root and include the
  sanitized parent thread id and job id.
- Existing user-owned branches are never reused. If the branch/path already
  exists, runtime must either resume a matching durable child record or reject.

Dirty tree policy:

- Default: reject if the parent workspace has uncommitted changes.
- An explicit future override may allow snapshotting tracked changes, but only
  when parent approval and provenance are recorded.
- Child worktrees must never overwrite or clean parent uncommitted changes.
- Dirty checks must include tracked, untracked, and conflicted files. Conflict
  markers block isolation until resolved.

Merge policy:

- Merge is a separate parent-owned action, not part of child completion.
- Clean diffs may be fast-applied only after explicit parent approval.
- Conflicts must be detected before touching parent files when possible; if a
  merge attempt produces conflicts, the UI must show conflicted files and stop.
- No silent conflict resolution, no hidden `git reset --hard`, and no automatic
  apply into a dirty parent workspace.

Evidence and provenance:

- Child job id and child thread id.
- Worktree path and branch.
- Base commit, current commit, and parent workspace path.
- Changed files grouped by added/modified/deleted/renamed/conflicted.
- Diff summary and bounded patch reference, not full unbounded diff text in
  parent context.
- Test commands attempted by the child, exit status, and output summaries.
- Merge request id, parent approval id, and cleanup decision.

Cleanup policy:

- Accepted worktrees may be cleaned after merge receipts and resulting commit /
  patch provenance are durable.
- Rejected worktrees are retained for a bounded inspection window.
- Failed child worktrees are not auto-deleted; they remain available for audit
  unless the parent explicitly cleans them.
- Cleanup is limited to analytix-owned worktree roots and
  `codex/subagent/*` branches that match the durable job record.

Renderer UX:

- `SubagentCallCard` shows isolated workspace status, branch, base commit, and
  changed file counts.
- The child card exposes inspect-diff affordance first. Accept/reject controls
  appear only after a merge request exists.
- The timeline shows merge-review state without embedding full patch contents.
- Conflict output is understandable: conflicted file list, parent dirty-state
  reason, and next available actions.

Safety:

- `readOnly` children cannot enable writable isolation.
- `inherit` / writable child runs require an explicit profile or user request.
- No auto merge without parent approval.
- No destructive cleanup before the parent can inspect.
- Isolated workspace creation must preserve the existing child-depth tool
  restrictions: child agents still do not receive subagent/job/goal/todo/plan
  or user-input tools by default.

Implemented Phase 5 minimal slice:

- Add durable isolation metadata to child job records:
  `isolationMode`, `worktreePath`, `worktreeBranch`, `baseCommit`,
  `currentCommit`, `changedFiles`, `diffSummary`, `mergeStatus`.
- Add a runtime-owned worktree creation port backed by Git commands or an
  existing desktop worktree service adapter. The Go runtime must not call a
  renderer-only API.
- Allow only background child runs with explicit writable policy to request
  `isolationMode=worktree`.
- Create the worktree before child start and set the child workspace to the
  isolated path.
- On child completion/failure, summarize changed files and persist metadata.
- Do not implement merge in the first slice.

Required tests:

- read-only child cannot request writable worktree;
- dirty parent tree rejects isolation by default;
- isolated child writes do not modify parent worktree before merge;
- create/list/output surfaces worktree metadata and changed-file summary;
- failed child keeps the worktree for audit;
- cleanup refuses paths/branches that do not match durable analytix ownership;
- merge conflict UX tests are deferred until merge endpoints exist.

### 7.3.1 Phase 6/7 Merge Review And Parent Decisions

Goal: let the parent inspect and review isolated child diffs before any merge
action exists. Phase 6 must keep merge application, conflict resolution, and
cleanup separate from review so the UI never implies that inspected changes
have been accepted.

Merge status lifecycle:

- `not_requested`: child has isolated worktree metadata but no parent review.
- `review_requested`: parent requested a bounded diff review and the runtime
  refreshed changed-file metadata.
- `clean`: future status for a verified clean apply plan, after parent
  workspace and conflict checks pass.
- `conflicted`: review or future apply planning found conflicts.
- `accepted`: parent approved applying a clean plan.
- `rejected`: parent rejected the child result.
- `cleanup_requested`: parent requested cleanup and runtime is validating the
  analytix-owned worktree/branch record.
- `cleaned`: analytix-owned worktree and branch were removed after durable
  cleanup receipts.

Review request contract:

- Endpoint: `POST /v1/runtime/task-jobs/isolation-review`.
- Input: `thread_id` / `threadId`, `job_id` / `jobId`,
  optional `client_request_id` / `clientRequestId`.
- Output: review id, job id, child run id, child thread id, worktree path,
  branch, base/current commit, changed files grouped by
  `added|modified|deleted|renamed|conflicted`, bounded diff stat, and
  `mergeStatus`.
- Review refreshes durable isolation metadata from the child worktree only. It
  does not modify the parent workspace and does not apply a patch.
- Review rejects cross-parent jobs, missing jobs, non-isolated jobs, and
  worktree paths outside the analytix-owned worktree root.
- The parent context receives only compact status and changed-file counts; full
  patches are never injected into model context.

Parent decision contract:

- `MergeDecision`: `id`, `parentThreadId`, `childRunId`, `jobId`, `decision`,
  `createdAt`, optional `reason`, and optional `approvalId`.
- `CleanupReceipt`: `id`, `worktreePath`, `branch`, `removed`,
  `retainedReason`, and `createdAt`.
- Reject records the parent decision and leaves the worktree available for
  inspection until cleanup.
- Cleanup requires durable job record match, analytix-owned path, and
  `codex/subagent/*` branch match. Cleanup must refuse user-owned paths or
  branches and must persist a receipt for the destructive operation.

Phase 8 accept clean diff contract:

- Endpoint: `POST /v1/runtime/task-jobs/isolation-accept`.
- Input: `thread_id` / `threadId`, `job_id` / `jobId`,
  `approval_id` / `approvalId`, and optional `merge_request_id` /
  `mergeRequestId` plus `client_request_id` / `clientRequestId`.
- `AcceptDecision`: `id`, `parentThreadId`, `childRunId`, `jobId`,
  `mergeRequestId`, `approvalId`, `baseCommit`, `parentHeadBefore`,
  `parentHeadAfter`, `changedFiles`, `appliedPatchDigest`, and `createdAt`.
- Preconditions:
  - job is parent-owned and terminal;
  - job is an isolated worktree job;
  - `mergeStatus` is `review_requested` or `clean`;
  - changed files contain no conflicted entries;
  - parent workspace is clean;
  - parent `HEAD` still equals the isolated `baseCommit`, unless a future
    fast-apply proof is added;
  - explicit parent/user approval id is present;
  - cleanup has not already happened.
- Parent dirty tree rejects accept/merge. Review may still run because it does
  not touch the parent workspace.
- Conflict output must list conflicted files and stop. No silent conflict
  resolution and no hidden `git reset --hard`.
- Accept applies a bounded patch only after a dry-run succeeds. If dry-run
  fails, runtime records `mergeStatus=conflicted` and returns without applying.
- Accept never cleans up automatically. Cleanup remains a separate parent
  action after inspection.

Phase 9 accepted lifecycle contract:

- State transition: `accepted -> cleanup_requested -> cleaned`.
- Accepted cleanup reuses `POST /v1/runtime/task-jobs/isolation-cleanup`.
- Preconditions:
  - job is parent-owned, isolated, terminal, and `mergeStatus=accepted`;
  - at least one durable `AcceptDecision` exists;
  - worktree path and branch match the durable analytix-owned record;
  - active/running child jobs still reject cleanup.
- `CleanupReceipt` links to the latest `AcceptDecision` through
  `acceptDecisionId`.
- Cleanup accepted jobs does not inspect or mutate the parent workspace and
  does not require the parent workspace to be clean. This is important because
  accepted patches may intentionally leave the parent workspace dirty.
- Cleanup must never run `git reset --hard`, delete non-owned paths, delete
  user-owned branches, or clean a path that is not represented by the durable
  job record.
- Already `cleaned` jobs deterministically reject repeat cleanup rather than
  running another destructive operation.

Phase 10 conflicted repair contract:

- Endpoints:
  - `POST /v1/runtime/task-jobs/isolation-conflict-report`;
  - `POST /v1/runtime/task-jobs/isolation-repair-check`;
  - `POST /v1/runtime/task-jobs/isolation-repair-accept`.
- Additional merge states:
  - `conflicted`: clean accept dry-run failed or review found conflicts.
  - `repair_review_requested`: parent is preparing a repair review. This may
    be transient in the UI; durable states below record the result.
  - `repair_checked`: repair patch dry-run applies cleanly.
  - `repair_rejected`: repair patch dry-run failed or was unsafe.
  - `repair_accepted`: explicitly approved repair patch was applied.
- `ConflictReport`: `id`, `parentThreadId`, `childRunId`, `jobId`,
  `baseCommit`, `parentHeadAtConflict`, `childHeadAtConflict`,
  `sourcePatchDigest`, `conflictFiles`, `conflictSummary`, `createdAt`, and
  `touchedParentWorkspace=false`.
- `RepairPatchReview`: `id`, `conflictReportId`, `parentThreadId`,
  `childRunId`, `jobId`, `repairPatchDigest`, `expectedParentHead`,
  `changedFiles`, `dryRunStatus=clean|conflicted|rejected`,
  `rejectedReason`, and `createdAt`. The full patch is stored only in the
  durable job record for later audited accept and is not injected into parent
  model context.
- `RepairDecision`: `id`, `repairReviewId`, `approvalId`,
  `parentHeadBefore`, `parentHeadAfter`, `appliedPatchDigest`, and `createdAt`.
- Conflict reports are read-only: they inspect child worktree metadata and
  parent Git `HEAD`, but do not modify parent workspace files.
- Repair checks enforce bounded patch size, bounded changed-file count, path
  safety, and expected parent `HEAD`; they run `git apply --check` only.
- Repair accept requires an explicit `approvalId`, a clean parent workspace,
  unchanged parent `HEAD`, and a previously clean repair review. It re-runs
  patch validation and dry-run before applying. Failure records conflict or
  rejection state and never silently resolves conflicts.
- Repair accept does not auto-cleanup, does not commit, does not run
  `git reset --hard`, and does not apply into a dirty parent workspace.

### 7.4 Todo Ops / Plan Sync

Goal: keep `todo_write` as the compatibility full-replacement API while adding
auditable small todo operations that preserve parent ownership and goal evidence
gates.

Required runtime contract:

- `todo_ops` applies ordered operations: `append`, `start`, `done`, `drop`,
  and `note`.
- `todo_patch` may be a compatibility alias only if it uses the same real
  executor and tests.
- Operations run on the parent thread todo store and emit the existing
  `todos_updated` event.
- Todos remain capped at 200 items with at most one `in_progress`.
- Sources are explicit: `manual`, `plan`, or `child`. Child runs still do not
  receive parent todo tools; child-sourced todos can be applied only by the
  parent after reviewing returned material.
- `done` updates todo state only. It does not call `complete_step`, does not
  create goal evidence, and cannot mark a goal complete.
- Plan markdown sync is a separate merge layer that may only update
  `source=plan` records and must not overwrite `source=manual` records.

Required tests:

- append/start/done/drop/note operations preserve a single active todo;
- over-200 todo state is rejected;
- `todo_patch` alias uses the same executor when enabled;
- child-depth preflight blocks `todo_ops` and `todo_patch`;
- goal completion remains blocked without evidence even if todos are done;
- plan merge never overwrites manual todo records.

### 7.5 Implemented Slice History

Previous second-stage slice: `todo_ops` / `todo_patch`.

- `todo_ops` and `todo_patch` share one real executor.
- Supported operations are `append`, `start`, `done`, `drop`, and `note`.
- `todo_write` remains full-list replacement and now has runtime enforcement of
  the 200-item cap.
- Todo notes are preserved as bounded metadata.
- `done` does not write goal evidence and cannot complete a goal.
- Child-depth materialization and preflight block both new tools.

Current Phase 3 slice: background child `steer` / `send_message`.

- Implemented as `/v1/runtime/task-jobs/steer`, renderer runtime client helper,
  IPC allow-list, SSE event mapping, and `SubagentCallCard` affordance.
- Bounded by parent ownership, background-only lineage, terminal-state
  rejection, message length limit, pending queue limit, and existing approval /
  sandbox policy. Steer never changes `toolPolicy` or grants tools.
- Admission is cooperative: queued messages are consumed by the child at the
  existing model/provider steering boundary, not by interrupting an in-flight
  request.

Current Phase 4 slice: cooperative `pause` / `resume`.

- Implemented as `/v1/runtime/task-jobs/pause` and
  `/v1/runtime/task-jobs/resume`, renderer runtime client helpers, IPC
  allow-list entries, SSE event mapping, and `SubagentCallCard` controls.
- Pause requests are parent-owned, background-only, lineage-bound, active-runner
  controls. Terminal jobs, foreground jobs, cross-parent requests, and jobs
  without child lineage are rejected.
- Admission is cooperative: the active child runner enters `paused` only at
  model/provider or post-tool safe boundaries. It never freezes a provider
  stream or a running host tool.
- Resume unblocks the active child runner through a persisted host-side resume
  token guard. If the runner is gone after process restart, resume is rejected
  rather than faked.
- Paused jobs are excluded from stalled diagnostics. Kill from paused remains
  terminal. Pending steer is retained and admitted only after resume.

Current Phase 5 slice: writable child worktree isolation metadata.

- Implemented by `isolationMode=worktree` on background `task` only.
- Requires explicit/profile `toolPolicy=inherit`; read-only, default-policy,
  foreground, `continue_from`, and `fork_from` requests are rejected.
- Parent dirty tree rejects isolation by default, including tracked,
  untracked, and conflicted files.
- Runtime creates an analytix-owned Git worktree under the configured
  `subagent-worktrees` root with branch namespace
  `codex/subagent/<parentThread>/<jobId>`.
- Child thread workspace is switched to the isolated worktree path before the
  child starts. Child writes do not modify the parent workspace.
- Completion and failure summarize changed files, bounded diff stat, base and
  current commit, branch, path, and `mergeStatus=not_requested`.
- Renderer shows isolated workspace metadata and changed-file counts; no merge
  controls are shown.

Current Phase 6 minimal slice: merge review only.

- Implemented as `/v1/runtime/task-jobs/isolation-review`, renderer runtime
  client helper, IPC allow-list entry, and child-card inspect affordance.
- Review validates parent ownership and worktree isolation, refreshes durable
  metadata from the child worktree, groups changed files, and marks
  `mergeStatus=review_requested` or `conflicted`.
- Review does not touch the parent workspace, does not apply patches, does not
  auto-merge, and does not clean worktrees.

Current Phase 7 minimal slice: parent reject and cleanup.

- Implemented as `/v1/runtime/task-jobs/isolation-reject` and
  `/v1/runtime/task-jobs/isolation-cleanup`, renderer runtime client helpers,
  IPC allow-list entries, and child-card reject/cleanup affordances for
  reviewed terminal isolated jobs.
- Reject validates parent ownership and worktree isolation, requires reviewed
  state, persists `MergeDecision`, and marks `mergeStatus=rejected`.
- Cleanup validates parent ownership, terminal state, reviewed/rejected state,
  durable analytix-owned path, and `codex/subagent/*` branch ownership before
  removing the worktree/branch and persisting `CleanupReceipt`.
- Cleanup never touches the parent workspace and does not imply merge
  acceptance. No accept merge control is shown.

Current Phase 8 minimal slice: accept clean patch.

- Implemented as `/v1/runtime/task-jobs/isolation-accept`, renderer runtime
  client helper, IPC allow-list entry, and child-card Accept affordance only
  for terminal `review_requested` / `clean` isolated jobs.
- Accept requires explicit `approvalId`, validates parent ownership, terminal
  state, reviewed/clean merge status, non-conflicted changed files, clean
  parent workspace, and matching parent `HEAD == baseCommit`.
- Runtime generates a bounded patch from the child worktree, dry-runs
  `git apply --check`, applies only if clean, persists `AcceptDecision`, and
  marks `mergeStatus=accepted`.
- Accept does not auto-clean the child worktree and does not create commits.

Current Phase 9 minimal slice: accepted cleanup lifecycle.

- Accepted isolated jobs can use the existing cleanup endpoint after the
  accept receipt exists.
- Cleanup records `CleanupReceipt.acceptDecisionId`, removes only the durable
  analytix-owned worktree and `codex/subagent/*` branch, and leaves parent
  workspace contents untouched even when accepted patches are uncommitted.
- Renderer shows cleanup for accepted, uncleaned isolated child jobs and hides
  accept/reject/cleanup actions after `mergeStatus=cleaned`.

Current Phase 10 minimal slice: conflict report and repair patch workflow.

- Implemented as `/v1/runtime/task-jobs/isolation-conflict-report`,
  `/v1/runtime/task-jobs/isolation-repair-check`, and
  `/v1/runtime/task-jobs/isolation-repair-accept`, plus renderer runtime
  client helpers, IPC allow-list entries, metadata mapping, and child-card
  conflict/repair affordances.
- Conflict report requires a parent-owned terminal isolated job with
  `mergeStatus=conflicted`; it refreshes child worktree metadata, records
  conflict files and patch digest, and explicitly records
  `touchedParentWorkspace=false`.
- Repair check requires a durable conflict report, validates repair patch size,
  paths, changed-file count, and parent `HEAD`, then performs only
  `git apply --check`. It persists `RepairPatchReview` as `repair_checked` or
  `repair_rejected` without modifying the parent workspace.
- Repair accept requires a clean repair review, explicit `approvalId`, clean
  parent workspace, matching parent `HEAD`, and a successful repeated dry-run
  before applying the repair patch. It persists `RepairDecision`, marks
  `mergeStatus=repair_accepted`, and does not cleanup or commit.

Phase 11 scoped child todo / goal ownership contract:

- `TodoScope` values:
  - `parent`: the active parent thread todo list owned by the parent agent;
  - `child`: the durable child-thread/run todo list owned by the child agent;
  - `projection`: parent-visible proposal material derived from a child list.
- `ChildTodoList`: `id`, `parentThreadId`, `childThreadId`, `childRunId`,
  `jobId`, `scope=child`, `items`, `createdAt`, `updatedAt`, and
  `sourceTurnId`.
- `ChildTodoItem`: `id`, `content`, `status=pending|in_progress|completed|blocked|canceled`,
  `evidenceIds`, optional `parentTodoRef`, `createdAt`, and `updatedAt`.
- `ChildTodoProjection`: `id`, `parentThreadId`, `childThreadId`,
  `childRunId`, `jobId`, `childTodoListId`, `projectedItems`, `summary`,
  `evidenceIds`, `status=proposed|accepted|rejected|superseded`,
  `createdAt`, `acceptedAt`, and `rejectedAt`.
- Goal ownership remains parent-owned. A child may create child-scoped todos
  and return evidence toward a parent goal, but it cannot complete the parent
  goal and cannot mutate `/v1/threads/{parentThreadId}/todos`.
- Parent inspection of child todos is explicit through task-job child todo
  APIs and renderer projection metadata. Parent todo mutation requires a later
  explicit projection-accept workflow with approval and mapped parent todo ids.
- Projection rejection is durable and idempotent for already rejected
  projections; rejection never removes child evidence or mutates parent todos.
- Child todo projection metadata is compact in parent timelines: counts,
  status, summary, and evidence id counts are surfaced, not full transcripts.

Current Phase 11A minimal slice: scoped child todo projections.

- Implemented as `/v1/runtime/task-jobs/child-todos`,
  `/v1/runtime/task-jobs/child-todos/project`, and
  `/v1/runtime/task-jobs/child-todos/reject`, plus shared endpoint constants,
  IPC allow-list entries, renderer runtime client helpers, metadata mapping,
  and child-card projection UI.
- Projection creation validates parent ownership, child lineage, and terminal
  child job state; it reads the child thread todo list, snapshots it as a
  `ChildTodoList`, builds a proposed `ChildTodoProjection`, and persists both
  to the durable task-job lineage record.
- Parent thread todos are not read-modified-written by projection or reject.
  Existing `todo_write` and `todo_ops` remain parent-thread tools only.
- Evidence ids from child todo items and child evidence bundle output are
  preserved in projection metadata for parent review, but no parent goal step
  is completed automatically.
- Renderer shows child todo projections as proposals and exposes reject only
  for proposed terminal child projections. No accept UI is shown in this slice.

Phase 12 child todo projection accept contract:

- Endpoint: `POST /v1/runtime/task-jobs/child-todos/accept`.
- `ProjectionDecision`: `id`, `parentThreadId`, `childThreadId`,
  `childRunId`, `jobId`, `projectionId`, `decision=accepted|rejected`,
  `approvalId`, `acceptedItems`, `skippedItems`,
  `parentTodosBeforeDigest`, `parentTodosAfterDigest`,
  `expectedParentTodosUpdatedAt`, and `createdAt`.
- `AcceptedProjectionItem`: `childTodoId`, `parentTodoId`,
  `action=complete_existing`, `previousParentStatus`, `nextParentStatus`, and
  `evidenceIds`.
- Accept is parent-owned, requires explicit `approvalId`, and is allowed only
  for `status=proposed` projections. Already accepted projections return their
  existing decision receipt without re-mutating parent todos or emitting a
  second event.
- Phase 12A only supports `complete_existing`: a completed child todo may mark
  an existing parent todo referenced by `parentTodoRef` as `completed`.
  Unmapped, unselected, missing, already completed, or incomplete child todos
  are recorded in `skippedItems`; no implicit content matching or parent todo
  creation is allowed.
- Accept requires `expectedParentTodosUpdatedAt` to match the current parent
  todo list `updatedAt` before any parent todo write. Stale parent todos are
  rejected.
- If one or more parent todo statuses change, runtime writes parent todos
  through the normal parent todo store and emits the durable `todos_updated`
  event. If all selected items are skipped, the decision is still durable but
  no parent todo mutation or `todos_updated` event occurs.
- Accept persists the projection decision and marks the projection `accepted`.
  It does not complete the parent goal, create goal evidence, or call
  `complete_step`.
- Renderer shows Accept only for proposed projections with completed mapped
  child todos and a loaded parent todo revision. Accepted/rejected/superseded
  projections hide active Accept controls and show decision metadata instead.

Current Phase 12A minimal slice: explicit projection accept.

- Implemented as `/v1/runtime/task-jobs/child-todos/accept`, plus shared
  endpoint constants, IPC allow-list entry, renderer runtime client helper,
  metadata mapping, and child-card Accept affordance.
- Runtime validates parent ownership, projection status, `approvalId`, and
  parent todo revision before applying selected mapped completions.
- Decisions preserve evidence ids, accepted/skipped item lists, before/after
  parent todo digests, and approval provenance for replay and audit.
- Parent goals remain parent-owned and are not completed automatically.

Still deferred:

- plan markdown checkbox merge;
- automatic parent todo creation from child projections;
- automatic child evidence to parent todo merge.
- foreground child-run steer;
- fast-apply proofs when parent `HEAD` has advanced safely;
- multi-file interactive conflict resolution UX beyond bounded repair patches;
- auto-cleanup after accept.
- auto-commit after accept or repair accept.
