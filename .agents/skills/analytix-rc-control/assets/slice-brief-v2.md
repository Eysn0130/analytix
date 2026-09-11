# Analytix RC Slice Brief V2

Copy this template, remove instructional text, and omit fields that do not
affect execution.

## Control identity

- `controller_epoch`:
- `brief_id`:
- `brief_revision`:
- `action`: `CONTINUE`
- `owner_command_thread_id`:
- Owner Command task and Controller-only direct `send_message_to_thread`
  host/route:
- `controller_thread_id`:
- `controller_host_id` (`hostId`):
- `actionable_event_route`: `trigger-turn: send_message_to_thread (discover current callable schema)`
  / `durable-wait: wait_agent + collaboration mailbox`
- `actionable_event_route_proof`: verified trigger-turn fact / outstanding
  durable-wait fact (successful mailbox delivery alone is not proof)
- Writer task/role:
- Supported normal close, if selected: `COMPLETE_BRIEF` / `BRIEF_COMPLETE_ACK`;
  writer echoes support before writing (otherwise retain legacy terminal close):

## Fresh facts and authority

- Branch, baseline `HEAD`/tree, worktree/index/stash:
- Protected/unrelated state:
- Current user authority and execution mode:
- Controller-owned diagnostic/test/runtime budget and exhaustion behavior:
- Explicit human one-shot/no-rerun/exact-target fences (not renewable budgets):
- Selected OpenSpec change and requirement/task ids:
- Reserved product/permission decisions:

## Delivery milestone and acceptance target

- Milestone contribution in one sentence:
- Nearest runnable checkpoint and current RED or missing proof:
- Finite internal acceptance denominator:
- Observable finish line and maximum claim level:
- Mandatory end-to-end propagation:
- Independently useful exclusions:

## Writer lease and autonomy

- Lease state: `ACTIVE`
- Lease mode: repository-exclusive by default; new briefs may offer
  `SCOPED_PARALLEL_WRITERS_V1` only with explicit Controller/writer adoption
  before writing, declared scope/resource isolation and single integration owner:
- Exact files or bounded ownership envelope:
- Discovered-path allowance for same-denominator propagation:
- Conflicting resource reservations/isolation (only when applicable):
- Paths/owners that remain protected:
- Decisions the implementer may make without escalation:
- Boundaries requiring a new brief or reserved decision:
- Git integration by the coordinating agent; candidate author returns changes:

## Evidence

- Stable accepted spec/code/evidence pointers; prefer pointers over copied rules:
- Minimum decisive validation and public/runtime/package seam:
- Diagnosis/research question and stop condition, if any:
- Required report paths and writer, if any:
- Luna lanes only if they have marginal value:

The writer reads this self-contained brief, applicable `AGENTS.md`, and the
listed pointers. It does not need the full RC Skill, Controller history, or
complete control protocol unless this brief explicitly names a required part.

## Message continuity

Accept dispatch only from the bound Controller within current authority;
actual direct user instructions retain their real priority. Ordinary messages
do not cancel this brief or its in-flight step/session. Retain an early future
brief only as a short intent; do not execute it. Apply a consolidated valid
same-brief supplement at a safe command boundary; a candidate-invalidating
revision needs REVIEW_SEAL before REVISE. An authorized urgent stop is handled
promptly, but never claim processes stopped from receipt or turn status alone.

The Owner may select a recoverable checkpoint for a short authorized maintenance
window before this entire brief finishes. Preserve its candidate/evidence and
next action; prove relevant commands inactive and explicitly release conflicting
reservations before handover. Resume only with a valid fresh lease. An ordinary
future intent or PAUSED label does not authorize that transfer.

Distinguish received/applied revision and safe/accepted state when relevant.
Use one matching application event when needed (piggyback on the next necessary
event, or agreed CONTROL_APPLIED), not ACKs of ACKs. No queue-after-turn or
priority parameter is assumed. Preserve step/session identity to avoid reruns.

## Push and return contract

- Send `SliceEventV1` to the Controller for `NEEDS_ATTENTION`, `REVIEW_READY`,
  supported `BRIEF_COMPLETE_ACK`, and `TERMINAL_ACK`; increment `event_seq`, bind review events to the current
  `review_round`, `review_barrier_generation`, `review_barrier_id`, and
  `candidate_fingerprint`, and include `OWNERSHIP_DELTA` in the next event when
  the discovered-path rule is used.
- Use the exact verified actionable-event route above. A successful
  `collaboration.send_message` is mailbox delivery only and cannot wake a
  Controller after its turn has ended. If the brief selects `durable-wait`, the
  Controller keeps a bounded wait for a short join or establishes an authorized
  heartbeat for unattended work. Without a verified wake route, it reports that
  monitoring is not active and must not promise `AWAITING_SLICE_EVENT` delivery.
- Preserve the actual applied revision and record a higher received revision
  as pending until the safe command boundary. Resolve it before starting another
  step; do not relabel or repeat the in-flight command. `REVIEW_SEAL` pauses
  edits at a safe boundary. `REVISE` may reactivate the same-denominator lease.
  A mutually supported `COMPLETE_BRIEF` closes this accepted brief and lease;
  it allows reuse only under a new brief and lease after inactive-process proof.
  `TERMINAL_FREEZE` and `CANCEL` permanently retire this writer task.
- Return reserved decisions to the Controller only. The Controller owns the
  mandatory direct push to `owner_command_thread_id`; the Slice must not bypass
  it or treat a Controller-local final response as Owner delivery.
- Return `CANDIDATE_RESULT` with received/applied packet state, lease/process state,
  changed paths, diff summary, meaningful progress, commands and exit states,
  acceptance-target evidence, nearest-checkpoint impact, remaining blocker or
  risk, and exact next action.
- Do not claim product or formal RC acceptance.
