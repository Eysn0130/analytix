# Controller Protocol

Applies to commissioned multi-task control, writer transfer and recovery.
Single-agent direct work uses the compact record in SKILL.md; do not instantiate
unused packets, routes or review barriers. This protocol is model-neutral.
Existing explicit first-turn BOOTSTRAP and retired-epoch boundaries remain.

## Core records

- **Acceptance denominator**: one observable behavior or evidence obligation
  that can independently be red, green, blocked, or not executed.
- **ControllerSnapshotV2**: branch, `HEAD`/tree/status/index/stash, protected
  state, canonical `owner_command_thread_id` and direct-message route, selected
  change and denominator, every unrevoked writer lease, latest control packet/
  event sequence, review round/barrier generation/barrier id/candidate
  fingerprint, evidence pointers, and exact next action.
- **ControlPacketV2**: `owner_command_thread_id`, `controller_epoch`, `brief_id`,
  monotonically increasing `brief_revision`, and
  `action=CONTINUE|REVIEW_SEAL|REVISE|COMPLETE_BRIEF|TERMINAL_FREEZE|CANCEL`.
- **WriterLeaseV2**: the behavioral fence for an exclusive write scope. It
  records the control identity, writer task, baseline `HEAD`/tree, ownership
  envelope, discovered-path allowance, and state `ACTIVE|PAUSED|REVOKED`.
  Existing leases default to repository-wide exclusion. Only new briefs with
  mutually acknowledged `SCOPED_PARALLEL_WRITERS_V1` may use the scoped mode below.
- **MeaningfulProgressV2**: a changed diff/canary, a located or removed blocker,
  a changed acceptance sub-denominator, a focused commit, or new evidence that
  changes the next decision.
- **ReviewFindingV2**: stable `finding_id` and exactly one primary class:
  `MUST_FIX`, `FIX_NOW`, `DEFER`, `CHALLENGE`, or `BASELINE`.
- **Safe boundary**: no command is mid-mutation, candidate and Git state are
  exact, and writer ownership can transfer without ambiguity.

Waiting, repeated reads, tool-call count, token expenditure, commentary volume,
and “still running” are not meaningful progress.

## State machine

States apply to the named brief and its reserved scope. With explicitly adopted
scoped concurrency, unrelated writers may continue; a Controller authority
handoff still closes every writer/lease it would transfer.

| State | Required evidence | Valid exit |
| --- | --- | --- |
| `BOOTSTRAP` | Fresh `ControllerSnapshotV2`, reachable Owner push route, and finite next denominator | `READY`, `BLOCKED`, `HANDOFF` |
| `READY` | Execution shape, bounded brief, conflicting writer proven absent | `WRITING`, `BLOCKED` |
| `WRITING` | Active lease and highest packet echoed by writer | `REVIEW_SEALED` |
| `REVIEW_SEALED` | Safe candidate boundary and paused lease | `REVIEW` |
| `REVIEW` | Whole candidate diff, checks, classified findings | `REVISE`, `COMPLETE_BRIEF`, `TERMINAL_FREEZE` |
| `REVISE` | Consolidated same-denominator findings and active lease | `WRITING` |
| `COMPLETE_BRIEF` | Accepted candidate, matching completion acknowledgement, inactive processes, revoked lease | `ACCEPTED` (task remains reusable) |
| `TERMINAL_FREEZE` | Exact final candidate, inactive processes, revoked lease | `ACCEPTED`, `BLOCKED`, `HANDOFF` |
| `ACCEPTED` | Truthful level and fresh Controller verification | `BOOTSTRAP`, `HANDOFF` |
| `BLOCKED` | Exact blocker, affected denominator, evidence, unblock condition, and no active writer | `BOOTSTRAP`, `HANDOFF` |
| `HANDOFF` | No unrevoked writer lease/process and compact handoff delta | successor `BOOTSTRAP` |

`REVIEW_SEALED` is recoverable; its `PAUSED` lease keeps its reservation and
blocks every conflicting new writer (all repository writers for legacy leases). `TERMINAL_FREEZE` and `CANCEL` are
terminal. Never run an independent verifier after terminal freeze if a failure
would need the retired writer to revise. When review lanes are selected,
increment `review_barrier_generation`, bind them to a
`review_barrier_id=<brief/revision/review_round/generation/fingerprint>`, and
issue no `REVISE`, completion or terminal packet until that barrier closes.

`COMPLETE_BRIEF` is an optional extension to V2: use it only when the brief
explicitly offers it and the writer echoes support before writing. It closes
that accepted brief permanently, revokes its lease and rejects later packets
for that brief, but does not retire the task identity. Reuse requires a new
brief id, fresh baseline and new lease after inactive-process proof. Without
that capability acknowledgement, keep the existing terminal protocol; never
reinterpret an old terminal action or permanent retirement as ordinary brief
completion. An empty `.closed` artifact is neither acknowledgement nor evidence.

A read-only review lane closes by returning, by acknowledged cancellation, or
by Controller abandonment followed by a persisted generation increment; reject
all results carrying a lower generation. Choose its decision deadline from the
expected check and the next action, not a universal timer. Continue without
that lane only when the remaining evidence is sufficient; otherwise report its
coverage as `UNVERIFIED-BLOCKED`. A writable lane can never be abandoned this
way: prove its commands inactive before lease transfer.

## Select and maintain finite denominators

Use the selected OpenSpec change and accepted specs as the planning authority.
Do not create a second feature database merely to run this protocol.

A Slice has one coherent delivery outcome with a finite set of related
acceptance obligations. It may cross layers when every touched
producer, consumer, migration seam, test, or public boundary is mandatory to
prove that same behavior end to end. This is propagation, not scope growth.

Split independently acceptable work when it improves ownership or delivery;
combine coherent dependencies when separate briefs would only repeat setup.
A new acceptance decision is needed when work belongs to a
different requirement or owner, introduces another durable owner/protocol/
migration/trust decision, changes an excluded public contract, or needs altered
success criteria to pass.

When one broad OpenSpec task repeatedly produces focused Slices and the
remaining work can no longer be named finitely, pause dispatch and decompose the
authorized task into ordered implementation sub-denominators. The Controller
may refine implementation tasks autonomously when accepted behavior does not
change. Use the appropriate OpenSpec planning/update workflow when requirements,
design, compatibility, or product behavior would change.

Before dispatch, the Controller should be able to name:

1. the red gap or missing proof;
2. the one behavior/evidence denominator;
3. mandatory propagation;
4. the success evidence;
5. what remains independently useful and therefore excluded.

If those cannot be stated without guessing, the next action is investigation or
planning, not a writing Slice.

## Execution authority and autonomy

Apply pressure to the next authorized result, not to ceremony around it.

| Owner | Autonomous inside current authority | Reserved boundary |
| --- | --- | --- |
| Controller | Choose denominator and execution shape; allocate diagnostic/test/runtime budgets; investigate; retire or replace a same-target brief; direct-write or dispatch; classify findings; verify; admit a focused commit; continue the epoch or hand off | Changed target/architecture/public contract; persistence or migration semantics; material security/privacy/trust/dependency/cost; credentials; destructive/external action; package/release/publication/formal verdict; peer-task creation without standing authority |
| Slice implementer | Choose implementation and probes; add necessary tests; repair attributable failures; follow same-denominator propagation; revise the ownership envelope under the discovered-path rule; challenge feedback with evidence | New denominator/owner/protocol/trust decision; protected or overlapping path; unavailable authority; destructive/external action; changed success criterion; no non-guessing path |
| Luna role | Choose the reads, probes, checks, or implementation details needed to satisfy its bounded role contract | Permission, authority, Git delivery, overall scope, acceptance, or work outside the contract |

A reserved boundary requires a decision only when the settled task and existing
explicit grants do not already resolve it. Implementing an approved migration or
contract change does not reopen its approval at each step.

Do not ask to continue after ordinary files, tests, failures, or dependency-valid
actions. Record material implementation rulings in the candidate report, not a
second speculative plan.

### Distinguish Controller budgets from user fences

Command counts, diagnostic attempts, focused test allowances, and runtime
execution budgets set by the Controller are behavioral fences for one brief,
not new permission boundaries. An explicit human one-shot, no-rerun, exact-target
or first-turn-only restriction is not a Controller budget: preserve it until
that authority changes it. A new brief, epoch or skill version cannot reset it.
Within standing delivery authority, the Controller may issue a
higher revision, terminally close an exhausted brief, create a new finite
denominator, and assign a fresh ordinary reversible budget without asking the
Owner, provided the accepted product target and every reserved boundary stay
unchanged and the success criteria and gate semantics are not weakened or
reclassified. Before another brief receives a writer, obtain the old brief's
matching completion or terminal acknowledgement, revoke its lease, and
independently prove its mutation processes inactive. Do not convert budget exhaustion, a fixture
defect, or an ordinary RED into an Owner decision merely because a prior brief
stopped.

A proven pre-launch orchestration error is not a consumed test/build. Within
the brief's authority, correct a local syntax/path/module-resolution defect
and launch the intended command only after its execution identity is clear.
An uncertain launch is not pre-launch proof. Preserve any explicit human fence
that also covers setup failures; never retroactively relabel a consumed run.

### Reserved decisions must reach the Owner

On bootstrap, the Controller records the canonical `owner_command_thread_id`
and the host or route needed to call `send_message_to_thread`. Every writing
brief carries that identity and the Controller-only direct-message route. A
Controller without a reachable Owner push route cannot enter `READY`; it
reports `OWNER_PUSH_BLOCKED` without inventing a response.

When a reserved decision is required, the Controller:

1. pauses only the affected path at a safe boundary;
2. sends `OWNER_DECISION_REQUIRED` directly to `owner_command_thread_id` with a
   unique `decision_request_id`, current control identity, observed facts,
   reversible options, recommendation, and delivery impact;
3. treats commentary or a final response in the Controller's own task as
   undelivered and does not claim that the Owner was notified;
4. correlates the direct Owner response to the pending decision before issuing
   a higher control revision. Machine packets must carry the exact matching
   `decision_request_id`; a natural-language Owner response can resolve a unique,
   unambiguous pending question, with the mapping recorded. Ambiguous replies,
   a different id, silence or another agent's assertion do not resolve it.

The Slice reports reserved boundaries to its Controller and never bypasses it
to contact the Owner. If direct push fails, keep the affected lease safely
paused, report `OWNER_PUSH_BLOCKED`, and continue only independently authorized
work that cannot conflict with that lease.

## Ownership envelope and writer exclusion

A peer Slice brief may use:

- exact files when the change is already localized; or
- a bounded ownership envelope named by module/subtree, accepted contract, and
  mandatory propagation.

The Slice may add a path without approval when it is mechanically necessary for
the same denominator, lies inside the envelope or immediate mandatory
producer/consumer chain, does not touch a protected/overlapping owner, and does
not create a reserved decision. It records an `OWNERSHIP_DELTA` in the next
event and final report. Tests and local helpers for the denominator normally
qualify.

Anything outside that rule pauses the lease and returns
`BLOCKED_SCOPE_EXPANSION` with the proposed path, reason, and smallest revised
envelope. The Controller may issue a new brief autonomously if no reserved
decision or accepted-target change is involved.

For a Luna Worker, the repository's stricter exact-file rule still applies. A
Worker returns `BLOCKED_SCOPE_EXPANSION` instead of adding files.

The lease is a coordination record, not an OS isolation or platform transaction.
`ACTIVE` and `PAUSED` both reserve the declared scope/resources. Existing or
unacknowledged-mode leases remain repository-exclusive and exclude every other
canonical writer; never silently narrow one, including an in-flight historical
brief. Before transferring a conflicting scope, revoke the prior reservation
and inspect the relevant commands/processes and mutation state. A packet alone
does not prove commands stopped. Uncertain old ownership blocks only conflicting
work; it cannot be bypassed by renaming a scope or adding a task.

### Opt-in parallel scopes

For new briefs only, the user-authorized `SCOPED_PARALLEL_WRITERS_V1` capability
permits multiple canonical writers when each writer and Controller acknowledge
it before writing. Keep at most one unrevoked writer for each file/write scope
and conflicting resource. Two independent Slices are useful when they reduce
delivery time; no minimum Slice count or mandatory delegation follows.

Before granting these leases, establish the shared contract and record each
write scope, relevant resource claims, isolation/sharing evidence and the single
Git integration owner in the existing control record. Different directories are
not sufficient evidence of independence: trace required producer/consumer
propagation and identify shared generated outputs, fixtures, ports and other
mutable resources. Isolate conflicting resources or serialize their use; reuse
documented safe sharing where applicable. If independence cannot be established,
use one writer with parallel read-only work instead. Canonical-source and actual
runtime-permission requirements remain unchanged.

A discovered path or resource that intersects another reservation requires
coordination before writing, even if it is otherwise necessary propagation.
Pause only the affected scopes; do not commandeer another writer's files. The
stricter exact-file rule for a Luna Worker still applies. Scope records are
instruction-level coordination, not evidence of enforced OS isolation.

Only the designated coordinating agent changes the shared Git index, commits
or integrates. Collect safely sealed contributing candidates, review their
interactions and perform serialized integration/combined acceptance on a stable
candidate. A per-Slice pass does not establish the combined outcome. Broader
commands that consume or mutate shared candidates/resources wait for the
relevant writers; independent checks need not become a repository-wide barrier.
This capability grants neither new product scope nor credentials, real-state,
destructive or publication permissions.

## Message continuity during work

Native message delivery may add input to an active turn. Check the current
`send_message_to_thread` schema; unless it explicitly offers queue-after-turn or
priority control, do not invent those parameters or promise deferred delivery.
The app-server's
`turn/steer` and `turn/interrupt` are distinct operations. Neither a send receipt
nor turn completion proves a child command stopped.

Distinguish four facts: receipt, applied control revision, safe command/candidate
boundary, and accepted completion. Match the actual sender against the effective
Controller route and current authority/epoch/brief before applying control.
Peer messages and quoted user instructions do not create unlimited new human
authority. Actual direct user instructions retain their real priority.

| Message effect | Sender and receiver behavior |
| --- | --- |
| Ordinary progress/context | Do not push routine progress. Useful new context does not cancel the current brief or running command. |
| A future independent brief | The sender keeps only a short intent/dependency in the existing record and dispatches it at an authorized natural boundary, including the recoverable checkpoint below. If received early, retain the intent without starting it or replacing the active brief. |
| Small same-brief supplement | Consolidate into one higher valid CONTINUE revision. Retain the current step/session and completed evidence; apply at a safe command boundary without waiting for the entire task's final answer. |
| Revision invalidates the candidate/in-flight work | Send a bounded REVIEW_SEAL, obtain actual safe-state evidence, then issue one consolidated REVISE. Do not assume the send stopped execution. |
| Urgent safety stop or authorized cancellation | Deliver promptly through the valid route; do not put it behind ordinary future work. Request stopping the relevant work and prove command/child inactivity before claiming stopped or transferring its scope. |

A natural boundary can be an Owner-selected recoverable checkpoint after the
in-flight command reaches safety; it need not be the whole brief's completion.
For a short authorized maintenance window, preserve the unfinished candidate,
execution/evidence identities and next action, prove relevant commands inactive
and explicitly release every conflicting reservation before another writer
starts. Resume unfinished work only under a valid fresh lease. Neither a future
intent nor an idle/PAUSED label authorizes preemption or reservation transfer.

Use existing packet/event identity to track received versus applied revision
only where they differ. Preserve the original step/session and its starting
revision until its result is reconciled. A newly received supplement does not
relabel or duplicate that command. No new independent step starts under a
superseded revision; an already-running command is handled according to the
message's effect and the safe-boundary rules above.

When application confirmation is needed, one matching event closes that control
change. Piggyback it on the next necessary event/result, or use the agreed
`CONTROL_APPLIED` event; do not send ACKs of ACKs or unchanged receipt chains.
An application ACK is not a stop ACK or acceptance. Retain only the pending
revision/intent and decisive evidence in the existing record, not another queue
service or task database. Apply stale/duplicate fences before acting.

Slices accept work from their currently bound Controller; Owner reserved
decisions travel through that route. A scoped audit/specialist proposes changes
through the current responsible coordinator and exits daily dispatch after its
own delivery. This does not interpose another approval layer on ordinary work.

## Control ordering and review convergence

- A writer's next step uses the highest valid applied revision for its epoch
  and brief. Track a received-but-pending revision separately and resolve it at
  the safe command boundary before launching another step; echo applied state
  truthfully in events/results. Delayed lower revisions are no-ops. It
  rechecks the highest identity immediately before each new mutation command.
- `REVIEW_SEAL` lets an atomic command reach safety, then pauses edits and
  returns the exact candidate. It does not revoke the brief. Increment
  `review_round` for every newly sealed candidate.
- The Controller reviews the whole candidate, waits for the current review
  barrier to close, assigns stable ids and one primary class to findings, and
  sends them in one consolidated `REVISE` packet. Do not stream a queue while
  edits are underway.
- Re-review covers corrections and their interactions with the original candidate.
  Progress may reduce required findings, establish missing evidence, or eliminate
  a plausible cause with a decisive experiment. A failed probe is not automatically
  a stalled round. Continue only with new evidence or a substantive adjustment;
  reassess repeated no-progress or recurring defects. Split, restart, block or
  hand off when that better serves the same accepted outcome. Never accept
  through exhaustion or change a gate to manufacture progress.
- Supported `COMPLETE_BRIEF` revokes that accepted brief and lease after matching
  `BRIEF_COMPLETE_ACK`; only a new brief may reuse the non-retired task.
- `TERMINAL_FREEZE`/`CANCEL` revoke the lease permanently. After acknowledgement,
  later instructions cannot reactivate the task.

Finding classes are mutually exclusive. Classify from evidence in this order:
valid and denominator-blocking is `MUST_FIX`; valid and required for the current
clean finish is `FIX_NOW`; valid but independently acceptable is `DEFER`;
unsupported or mis-scoped is `CHALLENGE`; unrelated preserved state is
`BASELINE`.

Finding meanings:

- `MUST_FIX`: correctness, security/privacy, public contract, or claimed
  denominator cannot pass.
- `FIX_NOW`: attributable in-scope repair or proof needed to finish cleanly.
- `DEFER`: valid, independent work with its own denominator.
- `CHALLENGE`: unsupported, mis-scoped, or contradicted by evidence.
- `BASELINE`: preserved unrelated state or failure.

An unresolved `MUST_FIX` blocks acceptance regardless of elapsed time, token
cost, correction count, or handoff pressure.

### Unattributed failure disposition

Preserve the historical failure, then define the current required invariant
and the evidence that could change its disposition. Unknown incident cause,
missing observability, and a current correctness defect are different claims;
do not merge them into an unbounded demand to reproduce the exact old timing.

Choose a distinguishing fault model or controlled interleaving and name the
next decision for its possible results before executing it. Follow the real
producer, durable state, public projection and recovery seams required by the
invariant; use closed diagnostics rather than unsafe raw errors. A test of the
detector proves detection, not the product fix. Likewise, successful drain or
process exit alone does not prove durable completion.

The reviewer may close the current finding only with evidence that establishes
the accepted invariant for the relevant failure classes, including any needed
repair and negative/recovery checks. Historical cause may remain explicitly
unknown only if exact incident attribution is not itself an accepted requirement
and no required correctness risk remains unresolved. Repeated non-reproduction,
time spent or narrower wording cannot satisfy this rule.

If the next experiment cannot distinguish a hypothesis or support acceptance,
stop that experiment lane. The Controller must choose a materially different
in-scope approach, safely split independent work, or escalate the exact missing
authority/evidence with a proposed disposition. Keep the affected claim blocked;
do not renew the same no-progress test under another brief number.

## Push-based monitoring

The preferred path is event-driven:

1. Controller dispatches and takes one start snapshot.
2. Peer Slice sends `SliceEventV1` directly to the Controller for
   `NEEDS_ATTENTION`, `REVIEW_READY`, `BRIEF_COMPLETE_ACK` and `TERMINAL_ACK`
   (or agreed `CONTROL_APPLIED` when needed), with monotonic
   `event_seq`; review events also carry `review_round` and
   `review_barrier_generation`/`review_barrier_id`/`candidate_fingerprint`.
3. Use a verified turn-triggering route or authorized heartbeat for unattended
   monitoring; use bounded native waits for short joins. Read
   [monitoring](monitoring.md) instead of repeating unchanged waits indefinitely.
4. Controller first matches the current epoch, brief and writer identity. A
   retired writer may supply the first exact `TERMINAL_ACK` for the currently
   pending terminal packet only; validate that packet identity and safe-state
   evidence, then close the pending acknowledgement. This receipt never restores
   a lease or permits writing. Reject all other retired-writer messages and
   other identities before comparing numeric revisions, generations or sequences.
   It rejects a lower revision or event sequence, ignores an exact
   duplicate, and refreshes only facts changed by a newer event. A repeated
   `REVIEW_READY` is actionable only when its review round or candidate
   fingerprint changed under a valid higher control revision.

Event-to-state mapping:

| Event | Required state/effect |
| --- | --- |
| `NEEDS_ATTENTION` | Remain `WRITING` while the lease is safely active; if the writer pauses, enter `REVIEW_SEALED`; resolve one decision without treating the event as completion |
| `REVIEW_READY` | Writer is safe, lease is `PAUSED`, enter or refresh `REVIEW_SEALED` for the named review round/fingerprint |
| `BRIEF_COMPLETE_ACK` | Supported completion packet is highest observed, candidate is accepted, commands inactive and lease revoked; close this brief without retiring the task |
| `TERMINAL_ACK` | Terminal packet is highest observed, commands are inactive, lease is `REVOKED`; enter `ACCEPTED`, `BLOCKED`, or `HANDOFF` only after fresh verification |

If direct push is unavailable, use the native wait mechanism with the latest
cursor and a bounded timeout within current tool and communication limits.
A timeout with no changed event does not justify transcript/Git/process polling
or duplicate execution. Use the monitoring reference to choose an economical
wake route for longer work. If no route exists, report the monitoring limitation
and protect ownership; do not claim delegated work complete. Native completion
notifications must be checked for actual failed/interrupted status; they do not
replace a correlated Slice event and candidate evidence.

Use a compact backstop only when the platform cannot signal, the writer missed
an agreed event, or a lease may be stale. Ask for phase, highest packet, last
meaningful progress, changed paths, command state, blocker, and next action.
Raw logs stay with their owner unless one excerpt distinguishes a failure.

Revision and event ordering are behavioral fences unless the host exposes an
atomic compare/reject or lease primitive. Use such a primitive when available.
Without one, never claim transactional exclusion. Before transferring a
conflicting scope, reusing a task or transferring Controller authority, fail
close at mutation-command boundaries and prove the relevant prior commands
inactive. A new explicitly adopted nonconflicting scoped writer does not require
unrelated writers to stop; an existing repository-exclusive lease still does.

## Adaptive Slice and epoch boundaries

The following are reassessment signals, not hard limits:

- context compaction or repeated large summaries;
- elapsed work, changed paths, or diff size growing faster than accepted
  denominators;
- repeated scope correction, failed probes, or review cycles without net
  progress;
- repeated re-reading of accepted work or handoff history;
- rising Token/context cost without a changed next decision.

At a Slice signal, the coordinating agent chooses one of: continue an inseparable near closure;
seal a coherent candidate and split independent residuals; restart from a
better brief; or block the affected path. Record why.

At an epoch signal, rotate only at a safe boundary when a fresh Controller is
expected to improve correctness or cost. Hard rotation conditions are loss of
the control tuple, contradictory live instructions, inability to disprove a
stale writer, or review/acceptance quality no longer being trustworthy. First
compaction alone is not hard rotation. Slice count and wall time alone are not
hard rotation.

Keep `ControllerSnapshotV2` compact after every accepted or blocked Slice so an
epoch can rotate cheaply when it should, without rotating early merely because
it can.

## Diagnosis and research

Deep reasoning is a falsifiable loop:

1. state the failing seam or missing proof and accepted invariant;
2. reproduce the narrowest red signal when feasible;
3. inspect the real producer/consumer, persistence, authority, and public seams
   needed to separate plausible causes;
4. run the smallest distinguishing probe;
5. use official documentation or a commit-pinned upstream primary source only
   when an external changing fact remains material;
6. stop when more reading would not change the next probe, brief, split, or
   acceptance decision.

Exa or another search system discovers sources; it is not the evidence owner.
For upstream reuse, retain URL, exact commit/version, license/provenance,
relevant path, observed fact, unknowns, and no-copy boundary. Follow the
repository upstream-admission process before copying code, prompts, Skills, or
assets.

## Acceptance and Git delivery

After leaving writing mode, the Controller refreshes Git, performs a separate
review of the complete task-owned diff, and checks the evidence-to-candidate
binding. Reuse valid checks under SKILL.md; rerun invalidated or necessary
independent checks, not an identical suite at every coordination layer. For a direct Controller Slice, use fresh-context verification
when risk or uncertainty warrants it. The coordinating agent owns staging and local commits under current authority;
peer and Luna writers return candidate changes without staging or committing.

Report the narrowest truthful level: `CANDIDATE`, `FOCUSED_ACCEPTED`,
`PRODUCT_ACCEPTED`, `FORMAL_RC_ACCEPTED`, or `BLOCKED`. A focused pass cannot
close a broad OpenSpec row, product milestone, package, or formal RC denominator
whose other evidence is incomplete.
