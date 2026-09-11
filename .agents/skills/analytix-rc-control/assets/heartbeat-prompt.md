# Controller heartbeat prompt template

This is preparation, not an installed automation. Use only after monitoring is
authorized and the current Controller/target ids and record are verified.
Submit the adapted prose through the native heartbeat automation tool; set the
schedule and notification preferences in its structured fields, not this prompt.
Do not instantiate this template when the user selected no automation.

> Follow up on the authorized Analytix delivery controlled by <Controller task
> id / epoch>, with Owner <Owner task id>. Read the compact control record at
> <authorized private record path>. Use $analytix-rc-control and its monitoring
> reference for this follow-up. Inspect only the known active Slice ids recorded
> there, using a bounded native snapshot and saved per-target cursors. Verify
> identity and actual status before acting; turn completion may be failure.
> Ignore stale/duplicate events and exclude already handled terminal targets.
> Use the record's accepted scope/next-gap references, current brief and lease.
> Keep BRIEF_DONE, WAIT_OWNER and DELIVERY_COMPLETE distinct from execution
> phases and control packets. Review a finished brief, continue already-authorized
> work after safe closure, or await the actual missing decision; do not infer
> new authority. A small direct Controller package needs no artificial Slice.
> Before a potentially repeated or side-effecting step, reconcile its existing
> step/session identity and completion evidence. An uncertain launch receipt
> does not justify starting again; pause only dependent work.
> For a pending Owner event, distinguish transport delivery from decision ACK,
> retain its correlation and next_check_at, and make at most one overdue
> confirmation for unchanged state before backing off. A matching Owner decision
> that leaves no executable action requires pausing the actual automation, not
> merely extending its cadence; a recorded timestamp alone saves no runs.
> Before a quiet return, check both unhandled events and the current executable,
> authorized and unexecuted next_action, even when there are no Slices or new
> events. Reconcile step/session and lease state before executing it. If neither
> requires action, stay quiet and finish this pass without more
> history reads, scans, tests or a waiting loop. On review readiness or a real
> blocker, obtain only the missing decisive evidence and coordinate the next
> action within the current brief and authority. Preserve writer-exclusion leases,
> human-imposed command limits and reserved approvals. Never let a tick or a
> queued future intent preempt a healthy in-flight step, advance an obsolete
> brief, or grant missing Owner authority. For explicitly adopted scoped leases,
> preserve exclusion for conflicting scopes/resources and serialized integration;
> legacy repository-exclusive leases stay exclusive. Record only changed
> control state. Local RED/GREEN results or test numbers alone do not notify
> the Owner. Push only reserved decisions, real blockers, material target,
> architecture or delivery-path changes, stable review/completion or urgent
> safety events; keep ordinary evidence local and do not acknowledge ACKs. An
> Owner-selected recoverable checkpoint may host a short maintenance window
> before the whole brief finishes, with candidate/evidence preserved, actual
> command safety and explicit release of conflicting reservations. A future
> intent alone cannot trigger that transfer. Pause
> on DELIVERY_COMPLETE of this authorized outcome, cancellation or transfer;
> a single brief's end is not that finish line. For persistent quota/access
> failure, pause if tools remain usable; otherwise record that an available
> coordinator/user must do so. Do not claim a model that cannot start will pause
> itself or an unverified platform circuit breaker will do it. For transfer,
> verify the old monitor paused before rebinding and verifying the successor.
> Record the actual resumption route and condition. Do not create another
> watcher, reset quota, change model settings or awaken a retired task.
