# Owner orchestration and delivery

Read when commissioning, coordinating or recovering user-visible delivery tasks.
For writer transitions use [control protocol](control-protocol.md); for an
actual unattended monitoring requirement use [monitoring](monitoring.md).

## Keep authority and context small

Owner Command and 总指挥 name the same canonical decision entry, not two
approval layers. It holds the human's accepted outcome, priority,
reserved decisions and final synthesis. A Controller may operate an epoch of
that delivery; a Slice owns a coherent implementation candidate. These are
responsibilities, not permission grants. The human retains approvals reserved
to the user. A request to publish task briefs is not product-release authority.

Use only the roles that improve the work. The Owner may also be the Controller
and execute directly. Adding a Controller or Slice is justified by independent
work, noisy execution context or a clear ownership boundary, not a required
hierarchy. Preserve the selected model and effort unless an applicable grant
explicitly selects another configuration.

Keep the Owner focused on changed product decisions, delivery-impacting blockers,
accepted bundles and final outcome. The Controller resolves ordinary diagnosis,
correction, validation and implementation ordering inside settled authority.
Do not forward every failed probe or budget adjustment for a new Owner ruling.
A human-imposed one-shot, no-rerun or exact-target fence remains binding.
A finite governance/audit assignment ends after its deliverable and handoff;
it does not become another daily dispatcher. Route its necessary revisions
through the current responsible coordinator, at a natural boundary unless a
real safety issue or invalidated candidate requires intervention.

Use one existing compact control record, alongside the selected OpenSpec tasks:
Owner/Controller task ids and epoch, authorized outcome and claim level, current
writer/brief/revision and lease, candidate/evidence pointers, remaining required
gap, next action, and monitoring cursor/next check if relevant. During a Slice's
canonical write lease, Controller bookkeeping belongs in its authorized private
control record, not unleased edits to canonical source or OpenSpec files.
An existing repository-exclusive lease still excludes all other canonical
writers. For new, mutually adopted scoped leases, the
[writer-exclusion protocol](control-protocol.md#opt-in-parallel-scopes) governs
independent Slices, resource conflicts and serialized Git integration.
Record only meaningful deltas. Do not build another feature database or copy
transcripts. A status document is not product evidence.

Organize the current evidence view by acceptance obligation, retaining the
most recent still-valid evidence for each obligation and pointers to the
complete history kept outside the active context. Preserve literal failure
classifications, candidate and execution identities, and what changes invalidate
each result. A newer local PASS does not erase an unresolved failure or replace
evidence for another obligation. Keep remaining gaps and next action current;
do not delete evidence by a fixed byte threshold or create a duplicate ledger.

## Separate the brief from the delivery

In that same record, point `delivery_scope_ref` to the authorized outcome and
accepted milestone/spec, and `next_gap_ref` to the next dependency-valid gap in
existing OpenSpec or a bounded evidence locator. References describe scope;
they do not authorize the next action. Do not copy the complete task table.

Use `delivery_phase` at the following boundaries, alongside the existing
execution `phase` and control packet. These are bookkeeping states, not new
ControlPacket actions, acceptance claims or writer leases.

| Delivery state | Meaning and next decision |
| --- | --- |
| `BRIEF_DONE` | This brief's work is ready for review. Complete its required review and normal lease closure; do not claim the whole delivery complete or stop follow-up solely because a brief ended. |
| `WAIT_OWNER` | A reserved decision or authority for the next bundle is missing. Record the correlated pending event and continue only independently authorized work; no new scope or lease is inferred. |
| `DELIVERY_COMPLETE` | The entire current user-authorized outcome meets its acceptance criteria. Close ownership and pause its monitor; an unfinished wider project does not keep this completed assignment alive. |

Keep the existing execution phase while work is active; do not force it into a
terminal label. After brief acceptance, the Controller may choose and issue the
next dependency-valid brief inside already-settled authority after safe writer
closure. An explicit Owner-reserved next-package decision still requires that
decision. Do not send ordinary breakdown, correction or validation back for
reapproval, or require a Slice for a small package the Controller can complete.

`WAIT_OWNER` needs a real wake route and a next useful check or recorded pause
condition, not perpetual polling. The [monitoring reference](monitoring.md)
owns pending-event acknowledgement, interrupted-step recovery and backoff rules.

A correlated decision to keep waiting resolves the delivery question, not the
underlying defect. If it leaves no executable action, record the concrete
unblock condition and pause any authorized scheduled backstop. The Owner must
either commission a falsifiable next investigation/repair inside settled
authority, identify a genuinely reserved human decision, or safely release the
conflicting reservation for independent work or handoff. Do not retain an idle
repository-exclusive reservation indefinitely merely to preserve a candidate;
candidate preservation and write ownership are separate. Release still requires
the matching acknowledgement and process/state proof, never elapsed time alone.

## Issue useful work and follow up

Sending is not scheduling after a turn. Keep a future brief's short intent and
dependency locally until the recipient's natural boundary; do not stream full
future prompts into an active task. That boundary may be an Owner-selected
recoverable checkpoint for a short authorized maintenance window, without
waiting for the whole long brief to finish. Preserve its candidate/evidence,
prove relevant commands safe and explicitly release conflicting reservations
before handing over writes. Ordinary future intent cannot cause that handover.
Follow the
[message-continuity rules](control-protocol.md#message-continuity-during-work)
for same-brief supplements, invalidating revisions and urgent stops. A received
message, applied revision, safe stop and accepted result are separate facts.
Use one matching application event when needed, never an ACK-of-ACK loop.

1. Choose the next dependency-valid bundle from accepted requirements. Define
   its user-visible finish line, necessary propagation, ownership and decisive
   evidence. A handful of related tasks can share one Slice.
2. For a new user-visible task, first verify current creation authority and
   project/environment selection using the current native tools. Creation is
   asynchronous: a queued `clientThreadId` is not a usable `threadId`. Bind the
   verified task id, not a guessed title or pasted link. Do not change model,
   effort or environment merely to save coordination cost.
3. For an existing healthy Slice, send a concise, correlated follow-up through
   the current native task-message tool; preserve its settings. Internal-agent
   mailbox delivery and a turn-triggering follow-up are distinct capabilities.
   Verify which route can awaken the recipient instead of assuming delivery
   means execution. Do not spawn another task for a routine correction.
4. While writing, issue same-bundle clarifications as one higher `CONTINUE`
   revision that the Slice observes at a safe command boundary. If a correction
   changes the candidate under review or invalidates ongoing edits, first use
   `REVIEW_SEAL`, review the stable candidate, and send one consolidated `REVISE`.
   A sent packet is not proof it was applied; require the matching echoed packet.
5. At review, classify findings and reuse still-valid evidence. Verify the
   accepted outcome and necessary integration rather than demanding every
   suggested improvement. Do not run product commands concurrently with a
   writer whose candidate or shared resources they could change.
6. Close an accepted brief using a supported `COMPLETE_BRIEF`, or retire it with
   the existing terminal protocol. A new brief needs a fresh baseline and lease.
   A permanently retired task is never reused. Add independent next work only
   after the previous ownership transition is proved safe.

For a user-selected no-automation workflow, every Slice brief carries the
verified native `send_message_to_thread` result route to this Owner/Controller.
The Slice sends review-ready or blocked evidence; that message starts the
coordinator's review, which then issues the next authorized package. Use a
bounded native wait when actively joining, not heartbeat/cron or a watcher.
If quota or access prevents the Slice from sending, there is no guaranteed
automatic recovery: retain the command/candidate and report the need for a
user/native resume when execution is available. Do not create a scheduler to
hide that limitation.

Use brief scope and outcomes to supervise construction; routine commentary,
number of commits, number of checks and time spent are not progress measures.
A coherent failure investigation may be valuable even before it yields a fix.

## Deliver the actual product outcome

Maintain a finite mapping from each required behavior and readiness obligation
to its evidence or exact gap. Include the public journey, persistence/recovery,
security, operability and package surfaces when they are part of the accepted
outcome. Do not add all these surfaces to every local edit. Do not shrink the
outcome to whatever focused checks currently pass.

After accepting a bundle, select the next gap or run the nearest product
checkpoint if it can change that selection. Consolidate related corrections.
When the complete authorized outcome is demonstrated, the Owner synthesizes
what shipped locally, decisive evidence and any separately reserved external
operation. A controller-local success or an OpenSpec checkmark does not itself
establish product/formal acceptance. Preserve the required recipient and actual
delivery acknowledgement for the final result.

## Rotate without losing control

Prefer keeping the canonical Owner thin and rotating execution context at safe
bundle boundaries. Rotate a Slice or Controller when repeated reconstruction,
contradictory state, scope drift or unreliable review makes a fresh context more
useful. Do not impose a fixed number of turns, tokens, hours or compactions; use
context telemetry only if the current tool actually exposes it.

Before transfer: resolve or safely close review lanes, prove mutation commands
inactive, revoke the outgoing writer lease, pause its scheduled monitor, and
persist a compact [handoff](../assets/controller-handoff-v2.md). Outstanding
user-imposed constraints travel with the handoff. An unavailable outgoing task
is not proof its processes stopped. If writer ownership cannot be disproved,
block transfer; a new context cannot bypass that uncertainty.

The successor verifies current Git/protected state, source/evidence identity,
Owner route, pending decisions and absence of conflicting writers in BOOTSTRAP.
Only after it accepts the new control identity may monitoring be rebound and a
new write lease granted. Late packets from the former epoch are inert.

Replacing the canonical Owner itself changes the authority route and requires
explicit user authority for that replacement. Do not silently create another
Owner, infer it from a title, fork full history as a cure for long context, or
keep two active control authorities for the same milestone. A fresh authorized
Controller can reconstruct retained evidence while the original Owner remains
the stable decision channel.
