# Analytix RC Controller Handoff V2

Emit only facts that changed or that the successor cannot cheaply reconstruct.
Use repository/evidence pointers instead of transcripts, logs, repeated Skill
rules, or the full accepted commit chain.

## Control boundary

- Source epoch/task and handoff reason:
- Canonical `owner_command_thread_id` and direct-message host/route:
- Latest reserved `decision_request_id` and delivery status, if any:
- Pending Owner event identity, transport receipt versus decision ACK,
  confirmation already used, next useful check and wake/resumption route, if any:
- Latest control packet, event sequence, review round/barrier generation/id/
  candidate fingerprint, and retired writer task:
- Completion/freeze/cancel acknowledgement and inactive-process proof:
- Explicit human command/target fences and pending decision mapping:
- Monitor id/binding, paused-state proof, last handled per-target cursors:
- User's automation choice; when prohibited, native result route only and no
  scheduler creation, reactivation or rebinding:
- Authority for successor task creation and control-route change, if granted:
- Lease modes, scoped reservations/resource isolation and single integration
  owner, if the new scoped capability was explicitly adopted:
- Pending received/applied revision, short future intent and original in-flight
  step/result identity, if relevant (not copied message transcripts):
- Active writer: `none` (required for this Controller authority transfer):
- Unrevoked writer lease: `none` (required):

## Fresh repository delta

- Branch, `HEAD`/tree, worktree/index/stash:
- Protected/unrelated state:
- New focused commits/candidates since the prior boundary:

## Delivery delta

- Current user-authorized outcome and selected OpenSpec change:
- `delivery_scope_ref`, `next_gap_ref` and current execution/delivery phase
  (brief done, awaiting a reserved decision, or entire authorized outcome complete):
- Unresolved side-effecting step identity, session/command and completion/effect
  evidence, if any; omit for ordinary read-only probes:
- Newly accepted/blocked denominators and evidence pointers:
- Remaining finite denominators in dependency order:
- Exact blockers, baseline failures, and unblocking conditions:

## Successor start

- Recommended next denominator and why it is dependency-valid:
- Red gap, proposed ownership envelope, and decisive validation:
- For an unresolved failure, current invariant and possible-result dispositions;
  historical unknowns do not become either an automatic PASS or an endless retry:
- First fresh fact the successor must verify:

This handoff is an unverified index. If a writer process or unrevoked lease
remains or cannot be independently disproved, do not hand off; remain
`WAITING`/`BLOCKED` until a safe boundary.
The successor must independently re-establish the Owner push route before it
enters `READY` or dispatches a writing brief.
