# Economical monitoring

Use for delegated work that needs a wake route or unattended follow-up. Ordinary
direct work and instruction audits do not need a live monitor. This guidance
uses current native tools; it does not install a scheduler or create one merely
because this file was read.

An explicit no-automation choice takes precedence over the scheduled backstop
below. Use the verified native result/follow-up route and bounded active joins;
leave existing matching automations paused. Do not reactivate or rebind them,
or create heartbeat/cron/watchers, without a new explicit user instruction.

## Events first, one bounded backstop when needed

The Slice pushes only actionable changes: attention required, review ready,
brief completion or terminal acknowledgement. Keep ordinary execution evidence
with its owner and route necessary Slice detail to the Controller. A new local
RED/GREEN result or test number alone is not an Owner-actionable delivery change.
Push to the Owner only for a reserved decision, real blocker, material change
to the accepted target/architecture or delivery path, stable review/completion,
or urgent safety event. Do not forward ordinary progress or acknowledge ACKs.

For a short active join, use the native wait with the latest per-target cursor
within current tool and communication limits. Do not turn repeated 50-second
waits into an all-day supervision loop. Before yielding indefinitely, establish
an actual turn-triggering event route or an authorized scheduled backstop;
mailbox delivery alone does not wake a finished turn. If neither is available,
state that unattended monitoring is not active and protect ownership. Do not
claim completion or promise a wake that the platform cannot provide.

When the user requests unattended monitoring, prefer one heartbeat on the
active Controller task (on the Owner only if it is also Controller). Start with
about 30 minutes for ordinary long-running construction, adjusting to expected
command duration and recovery needs; 30–60 minutes is an example, not a mandatory
interval or a stale-writer deadline. Event delivery remains immediate when the
route supports it. Do not schedule a watcher for every Slice or a second Owner
watcher of the same work.

Each scheduled run consumes resources even if it stays silent. A missing update
is a reason for a bounded health check, not evidence that an agent died. After
one overdue check, persist the observation and next useful check time rather than
retrying continuously. No automatic quota reset, extra account, or model downgrade.

## Set up through the current application contract

Resolve the exact active Controller id and current authority before scheduling.
Inspect matching existing automation configuration, then use the native
`automation_update` tool to view/update an existing matching heartbeat or create
one when authorized. Preserve unrelated fields and notification preferences.
Do not edit scheduler TOML, invent API parameters, silently revive old paused
jobs, or replace a thread heartbeat with a standalone cron task.

Use the [heartbeat prompt](../assets/heartbeat-prompt.md) with concrete current
ids and record location. Remove template guidance before submitting. Record the
returned automation id, binding, schedule, last handled event/cursor and next
check in the compact control record. Verify the actual tool result; a prepared
prompt or paused job does not establish active monitoring. Local scheduled work
requires the machine and application to be available. Event availability and
scheduler persistence are separate facts.

`ACTIVE` and a next-run timestamp establish configuration, not a successful
scheduled turn. If a natural run has occurred, inspect one bounded run receipt
and its relevant effects; otherwise mark that behavior unobserved. Do not
manually trigger repeated turns or wait for a tick just to claim validation.

## Recover a step or a pending decision

For an authorized step that has side effects or could be started twice, record
the brief-scoped `step_id` and launch intent before invoking it; retain the
returned in-flight session/command identity and then its completion evidence.
Use a native idempotency key when that operation supports one. After an
interruption, first reattach to that command or inspect its recorded result and
effects. A missing launch receipt is uncertain execution, not permission to
start again. If identity cannot be resolved, pause only dependent work and
report the gap. Reuse existing command/lease records; small read-only probes do
not need a second journal or empty in-flight fields. A `step_id` alone is not an
exactly-once guarantee and never replaces writer/process boundaries.

For an event that needs an Owner decision, retain one `pending_owner_event`:
`event_id` (reuse `decision_request_id` where present), Owner/brief/revision,
sent/delivery status, correlated decision acknowledgement, and `next_check_at`.
A successful message-tool receipt proves transport delivery only. Mark the
decision resolved only from a matching actual response. A receipt-only ACK may
adjust the expected check time but does not authorize dependent work.

At an overdue check, do at most one bounded status/delivery confirmation for
the same unresolved event, retaining its identity if a lost delivery needs
resubmission. Record `confirmation_used` and the next useful check, then back
off instead of reminding the Owner on every tick. A changed response or new
material event can justify a new decision; elapsed time alone cannot.

When a matching Owner decision leaves `WAIT_OWNER` with no executable action,
pause the actual native automation and record the resumption condition and
verified turn-triggering route. Do not merely lengthen its interval. For an
unresolved delivery or an external condition that truly needs periodic checking,
use only the bounded backstop that remains authorized; after the bounded
confirmation, prefer pause when a native reply will wake the task. A future
timestamp in JSON does not change scheduler cost. Preserve undelivered decisions
explicitly. A correlated reply permits restoring monitoring only when the user
still authorizes automation and actual unattended work needs it.

## One monitoring pass

Reuse already-loaded current guidance. Reload a reference only when it is
missing from usable context or its relevant rules changed, not on every tick.

1. Read the compact control record and consume a bounded native status/event
   snapshot for known active targets using their latest cursors. Wait APIs may
   return only the first changed/completed target. Update only returned targets;
   retain other cursors. Exclude already handled terminal targets from later
   waits so one old failure cannot repeatedly win and starve the active target.
2. Match Owner/Controller/epoch/brief/writer identity before revision and event
   ordering. Accept only the exact pending terminal acknowledgement from a
   retiring writer as defined in the control protocol; it cannot restore write
   authority. Drop other retired, stale or duplicate events. A platform `turnCompleted` event
   may contain `failed` or `interrupted`; inspect the actual turn status and
   error. Task `idle`, delivery acknowledgement, or a file named `.closed` does
   not prove successful work, inactive writers or an accepted candidate.
3. Before returning quietly, also check the current authorized `next_action`.
   An already-ready, unexecuted action can be useful work even when no new
   event arrived or there are no Slices. Execute it only after reconciling its
   step/session state and current authority/lease. A healthy in-flight command
   is reconciled or reattached, not preempted by a tick or a future intent.
   Pending messages follow the current brief's safe-boundary and stale-event
   rules; a tick cannot grant missing Owner authority. If there is neither an
   unhandled event nor an executable authorized action, remain quiet and finish
   this monitoring pass.
   Do not reread history, rescan Git/processes, run tests/builds, or sleep in a
   loop merely to produce activity. The product task remains pending; finishing
   a monitor tick is not declaring the delivery complete.
4. On a real event, fetch only the evidence needed for the decision. A task-read
   `turnLimit` or per-output character cap may still return many items and a
   large preview. Filter the tool response before displaying it to the model:
   retain identity, actual status/error, bounded actionable messages and evidence
   pointers; omit previews, reasoning, raw logs and unchanged tool transcripts.
5. Coordinate a needed clarification or review through the valid control packet
   within existing authority. Monitoring is not a second source writer. Do not
   infer inactivity from status alone, interrupt a healthy command because time
   passed, issue another lease, or expand execution scope from an event body.
6. Apply the delivery boundaries in [Owner orchestration](owner-orchestration.md).
   `BRIEF_DONE` leads to review and safe next-work selection; `WAIT_OWNER` follows
   the pending-event/backoff rules above. Pause for `DELIVERY_COMPLETE` of the
   current authorized outcome, cancellation or Controller transfer. A brief's
   completion alone neither finishes the delivery nor authorizes the next one.

## Failure and transfer limits

On persistent quota/access failure, pause through the native tool if execution
and tool access still work, report the condition once and record what permits
resumption. If the model never starts or cannot call the tool, it cannot execute
that instruction. An available Owner/coordinator or the user must pause through
the existing native route/UI; report pending pause, not successful fail-stop.
Claim platform-enforced automatic disabling only when the installed contract or
an actual failure receipt establishes it. Do not invent scheduler fields, assume
an automatic circuit breaker, add another watcher or reset quota. Silence is
not free and lack of a failure notification is not health evidence.

For Controller transfer, first pause and verify the old automation binding,
preserve pending events/step evidence/cursors with the handoff, and prove old
mutation and leases inactive. After successor bootstrap accepts the new identity,
update the existing automation through the native tool where supported and
verify the new target/status. If replacement is necessary, the old monitor must
remain paused before the successor one is activated. Never wake a retired task.

Keep raw evidence with its owner. The compact record need only preserve last
meaningful progress, handled event/cursor, phase/command state, blocker, next
useful action/check and monitor state. Do not accumulate a heartbeat transcript
in the Owner's context or create a new report for every quiet tick.
