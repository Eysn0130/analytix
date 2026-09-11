# Analytix RC SliceEventV1

Send only actionable events. Do not stream ordinary commentary or raw logs.
Omit inapplicable optional fields; reuse existing packet/event identifiers.
For an agreed CONTROL_APPLIED event, identify the applied packet and boundary
once. A transport receipt means received, not applied, stopped or accepted;
never acknowledge an acknowledgement. Keep a prior in-flight command's own
identity/revision until its result is reconciled.

```text
SliceEventV1
controller_epoch: <epoch>
brief_id: <id>
brief_revision: <highest valid received>
applied_revision: <actual applied revision, only when different or confirming application>
event_seq: <monotonic integer within this brief>
event: NEEDS_ATTENTION | REVIEW_READY | BRIEF_COMPLETE_ACK | TERMINAL_ACK | CONTROL_APPLIED (if agreed)
lease_state: ACTIVE | PAUSED | REVOKED
review_round: <integer or none>
review_barrier_generation: <monotonic integer within this brief or none>
review_barrier_id: <brief/revision/round/generation/fingerprint or none>
candidate_fingerprint: <diff/tree evidence id or none>
meaningful_progress: <diff/canary/blocker/denominator/evidence delta>
changed_paths: <compact list or evidence path>
command_state: <running/safe/complete plus decisive exit state>
ownership_delta: <none or same-denominator paths and reason>
blocker_or_findings: <compact summary or report path>
next_action: <one exact action>
```

Use `NEEDS_ATTENTION` only for a decision, blocked scope, failed control path,
or genuinely stale lease—not for normal command duration. `REVIEW_READY` means
the writer reached a safe boundary and paused edits; a duplicate with the same
control identity, sequence/round/barrier generation/id, and fingerprint is a
no-op, and a lower-generation result is stale.
`BRIEF_COMPLETE_ACK` requires previously agreed support and the matching highest
`COMPLETE_BRIEF` packet, an accepted candidate, inactive commands and a revoked
lease. It closes the brief, not the task identity.
`TERMINAL_ACK` means the terminal packet was observed, commands are inactive,
and the lease is revoked.
