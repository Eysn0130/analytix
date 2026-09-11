## Context

The prior governance change narrowed entry triggers, but a second body-level
review found conflicting late-stage instructions in installed skills. The
interrupted Owner/Controller tasks also demonstrate that completion events,
small read parameters and files named closed are insufficient recovery evidence.
See proposal.md for scope and the dated follow-up review for source evidence.

## Goals / Non-Goals

**Goals:** Preserve a thin Owner decision channel, coherent Slice autonomy,
reviewable follow-up, economical monitoring and safe context transfer. Correct
remaining workflow contradictions at their actual instruction owners.

**Non-Goals:** Product implementation, lower acceptance criteria, model or
permission changes, automatic task restart, live scheduling or installing a
third-party orchestration framework.

## Decisions

- Keep the RC entrypoint small; Owner orchestration and monitoring are conditional
  references. Use current native task/automation tools rather than a custom daemon
  or Claude-specific scheduler. One Controller heartbeat, when authorized, backs
  actionable events; its cadence is an operational choice, not a kill deadline.
- Extend the existing behavioral protocol with opt-in COMPLETE_BRIEF. Explicit
  support and matching acknowledgement close a brief and revoke its lease while
  leaving a healthy task reusable under a fresh brief. Legacy terminal retirement
  remains permanent. This avoids repeatedly creating tasks solely because normal
  work finished without weakening stale-message or single-writer fences.
- Keep the Owner as the accepted decision route; rotation transfers execution
  after inactive-command proof, lease revocation, monitor pause and fresh bootstrap.
  Replacing the Owner itself requires explicit authority. No fixed token or turn
  quota constrains model reasoning.
- Monitor structured status and correlated events, not transcripts. Handle failed
  completion, first-target-only wait results, duplicate cursors and empty artifacts.
  Store compact changed state with its authorized owner; no concurrent canonical
  bookkeeping while a Slice writes.
- Retain installed OpenSpec CLI 1.11.0: required lifecycle/schema capabilities are
  available. Fix guidance for existing change reuse, legitimate skipped artifacts,
  and already-approved delta removals. No package update is needed for these fixes.
- Correct installed skill body contradictions in place with private before/after
  snapshots. Cache replacement can overwrite these local changes; do not install
  an automatic reapplication hook or silently alter future versions.

## Risks / Trade-offs

- Behavioral leases are not atomic runtime locks → validate current identity and
  acknowledgements at command boundaries and independently prove inactive mutation.
- A silent heartbeat still costs a scheduled run → event-first operation, bounded
  passes, one monitor per active Controller and pause on terminal/external suspension.
- New normal-close semantics could be misapplied to old tasks → explicit capability
  acknowledgement, immutable old retirement, fresh brief id and lease.
- Thread summaries can omit relevant evidence → read only the exact missing evidence;
  do not equate an idle task or marker file with accepted completion.
- Documentation validation cannot prove future unattended behavior → label the actual
  native snapshot and independent scenario review separately from unexecuted live
  scheduling and product construction.

## Migration Plan

Update skill entrypoint/references/templates and the affected host instructions;
validate current schema, links, scenario decisions and preserved dirty state;
synchronize and archive this governance change; deliver a focused local commit.
Existing running or retired briefs keep their original explicit fences. A future
commissioned Controller reads the new skill and advertises optional normal close
before using it. Roll back only task-owned edits using reviewed diffs/private
snapshots, preserving unrelated changes and historical evidence.
