---
name: analytix-rc-control
description: Coordinate an explicitly commissioned Analytix Owner, delivery or RC controller, including Slice dispatch and review, follow-up, economical monitoring, safe handoffs and evidence-based acceptance. Ordinary fixes, read-only audits and editing this skill do not activate the controller protocol.
---

# Analytix RC Control

Deliver the complete authorized Analytix outcome with the least coordination
and verification that establishes it. Efficiency means fewer avoidable loops,
not a smaller accepted requirement, weaker gate, or premature completion.
The current user request governs scope and authority; applicable AGENTS.md,
accepted specs, current code and fresh evidence govern the work.

This skill does not select, downgrade, cap, or replace the main model or its
reasoning effort. Sol and Astra retain their native reasoning, tool use,
planning and implementation choices. Use the protocol to resolve real ownership
and evidence problems, not to simulate a less capable engineer.

## Start with the delivery outcome

Resolve the current authorized milestone and its observable finish line from
accepted specs and the selected OpenSpec change. Do not inherit a historical
first-runnable track, fixed task order, model choice, command budget, or stale
PASS. Keep one compact working record: outcome, current candidate/HEAD, writer,
remaining requirement or blocker, evidence, and next useful action.

Choose a coherent delivery bundle that crosses every producer, consumer,
migration and public seam required by that outcome. It may contain several
related OpenSpec tasks. Split where work is independently acceptable, where
ownership conflicts, or where context isolation materially improves delivery;
do not create a new Slice, plan, report, or review gate for each file or micro-fix.
A finite acceptance denominator makes coverage visible; it is not a replacement
for the user-visible product outcome.

Before adding work, identify the unmet accepted requirement or observed failure
it closes. Necessary root-cause repair and propagation remain in scope. Record
independent improvements for later without constructing them now. Finish the
whole authorized scope, including attributable corrections, integration,
documentation and appropriate evidence; then stop expanding it.

## Choose the execution mode

- **Direct work:** the coordinating agent implements and reviews a coherent
  bundle itself when it has a clear path. Use the compact working record and
  ordinary Git/ownership checks. No fabricated peer packets, event route,
  review barrier or Owner-task lookup is required for a single-agent task.
- **Internal delegation:** use a bounded helper only when authorized and its
  independent research, implementation or challenge can improve the result.
  Parent retains integration and acceptance. Read
  [fleet routing](references/luna-fleet-routing.md) when selecting a role.
- **Commissioned multi-task controller:** when the user has authorized a
  controller fleet, use [control protocol](references/control-protocol.md) for
  peer writer leases, ordering, reserved Owner decisions and actionable events.
  User-visible peer creation requires explicit or existing standing authority;
  an internal subagent is not a new user-owned task.

For Owner-level task dispatch, follow-up and delivery synthesis, read
[Owner orchestration](references/owner-orchestration.md), including the distinction
between a finished brief, waiting for a reserved decision and completed delivery.
For an actual monitoring requirement, read
[economical monitoring](references/monitoring.md).
Read only the reference needed for the current mode or transition. Editing or
auditing this skill does not commission a product controller or authorize
contacting another task. Skill names and templates never confer permissions.

## Reconstruct only what may have changed

A commissioned controller takeover, rotation or recovery requires a complete
read-only BOOTSTRAP before product mutation: authority/Owner route, current
Git/index/protected state, writer leases and relevant process/FD evidence,
selected accepted target and active-change status index, and next finite outcome.
Inspect full OpenSpec bodies only for the selected target and real dependencies.
Respect an explicit first-turn-only bootstrap or permanent retirement instruction
for that epoch. A permanently retired task identity cannot be reused for writing,
even under a new brief; this does not retire other tasks or the project itself.
Completing BOOTSTRAP is not a reason to end the turn: continue into the next
authorized action when its gates pass, unless the user explicitly required a
bootstrap-only turn or an unresolved boundary actually blocks it.

For direct work, start from current Git and the affected contract. Escalate to
full reconstruction when writer ownership, target, candidate or history is
uncertain. Within a healthy epoch refresh deltas, not unchanged history or every
process on the host. Task status or a CWD handle alone does not prove a writer.
Use `OBSERVED`, `THREAD-REPORTED`, `INFERRED`, `UNVERIFIED-BLOCKED` where those
provenance distinctions affect a decision.

## Continue within settled authority

Build/continue/finish authority covers ordinary reversible choices needed for
the accepted outcome under repository rules, including the granted focused
local commit. Ask only about a material unresolved choice or an operation whose
required authorization is missing. Check prior and standing grants first.
Present a concrete reviewable result before a final reserved approval when
independent preparation can be completed safely.

Preserve credential, real-user-data, external-write, destructive, release and
formal-acceptance boundaries. A changed architecture or contract requires Owner
alignment only when it is an unsettled change to the accepted target; implementing
an already approved migration does not require approval again for each step.
A blocker pauses dependent work only. Complete independent authorized work when
ownership permits; do not infer the missing approval from silence.

In a commissioned Owner/Controller fleet, reserved decisions must reach the
canonical `owner_command_thread_id` through its verified authorized route.
Preserve that authority; a controller-local final is not Owner delivery. A direct
user task uses its current user channel. See the protocol for correlation rules.

## Make validation decisive and reusable

Select checks from the changed behavior and claimed readiness level. For a bug,
prefer a focused regression that demonstrably detects the original fault when
feasible. Do not delete working code to manufacture a test-first chronology.
Use isolated synthetic fixtures; never echo secrets or treat a test name as
permission for live effects. Expand to integration, migration, UI, package or
formal evidence when those surfaces determine success, not because a skill
contains a checklist.

Bind each reusable evidence result to the candidate fingerprint (including
relevant uncommitted/generated inputs), command, environment, owner, exit state
and supported claim. The coordinating agent separately reviews the task-owned
diff and evidence. Reuse applicable results from the same stable candidate;
rerun only checks invalidated by changes, environmental differences, missing
provenance, or a necessary independent challenge. A commit with identical tested
content does not itself invalidate evidence. Formal exact-artifact rules remain.

Reassess the nearest product checkpoint after a bundle; execute it again when
its result can change the next decision. Do not repeatedly run an unchanged
suite merely to restate success, satisfy each role, or fill a progress report.
Research and diagnostics end when enough evidence selects the next action.
Repeated failures require a new hypothesis or adjustment; investigate or change
approach instead of retrying blindly. Missing evidence still limits the claim.
For an unattributed historical failure, separate the incident's unknown cause
from the current invariant that must be proved. Follow the protocol's
[failure disposition](references/control-protocol.md#unattributed-failure-disposition):
neither repeated GREEN nor permanent investigation is an acceptance strategy.

## Review, integrate and close

Classify findings against the accepted outcome: `MUST_FIX` blocks correctness
or the claim; `FIX_NOW` is attributable required cleanup/proof; `DEFER` is
independently useful work; `CHALLENGE` lacks support or scope; `BASELINE` is
preserved unrelated state. Do not turn every review suggestion into construction.
Consolidate related corrections and verify their interactions.

A useful failed experiment can narrow the cause without immediately reducing a
finding count. Continue while evidence changes the next decision and the approach
remains credible; reassess repeated no-progress cycles. Never close an unresolved
required finding through exhaustion, and do not require arbitrary review rounds.
Use an independent verifier when its perspective can materially change confidence.

Before writer transfer, prove the previous writer and its mutation commands
inactive. In fleet mode `REVIEW_SEAL` is recoverable. A mutually supported
`COMPLETE_BRIEF` closes an accepted brief and revokes its lease without retiring
a healthy task; a new brief requires a fresh baseline and lease. Existing
`TERMINAL_FREEZE`/`CANCEL` and explicit permanent retirement stay irreversible. Preserve writer-exclusion and
stale-message fences in the protocol. New briefs may explicitly adopt its
scope-based parallel-writer capability; existing repository-exclusive leases
remain exclusive. No source mutation is hidden in a review.

Report only the established level: `CANDIDATE`, `FOCUSED_ACCEPTED`,
`PRODUCT_ACCEPTED`, `FORMAL_RC_ACCEPTED`, or the exact `BLOCKED` scope. A focused
pass never substitutes for unfinished product, package or formal requirements.
State what changed, decisive evidence, residual required gaps and next action.
When the authorized outcome is complete, finish without inventing new checks.

Use native Goal tracking only on explicit user request. Prefer actionable
events for delegated work; mailbox delivery alone cannot wake a finished turn.
Use bounded native waits for short joins and a verified wake route or authorized
low-frequency heartbeat for unattended work. Report a missing monitoring route
without claiming completion. Do not create a perpetual wait loop or a watcher
per Slice. Monitoring details and its prompt are loaded only when needed.
When the user excludes automations, use native turn-triggering Slice results
and Owner follow-ups only; do not create, reactivate or rebind a scheduler.
An unavailable wake route is reported, not repaired by overriding that choice.

Rotate only at a safe boundary when context/ownership uncertainty or repeated
reconstruction makes a fresh controller more effective. Compaction, elapsed time,
Slice count and token expenditure alone are not rotation rules. Use the compact
[handoff template](assets/controller-handoff-v2.md) for an actual fleet handoff;
[slice brief](assets/slice-brief-v2.md) and [events](assets/slice-event-v1.md) are
for delegated writing. Never fill them with transcripts, unsafe logs or secrets.
