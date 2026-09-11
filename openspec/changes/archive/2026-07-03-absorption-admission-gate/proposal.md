## Why

Analytix has a detailed upstream absorption research program, but no formal
OpenSpec gate that prevents a capability row from entering implementation with
unknown prompt weight, prefix impact, tool schema impact, cache telemetry, UI
cache neutrality, or subagent handoff risk. This must be fixed before any P0/P1
upstream feature absorption starts.

## What Changes

- Add an absorption admission gate that classifies each future absorption row
  as `ready`, `blocked`, or `not-applicable`.
- Require Hard Gate Evidence for risky rows, including prompt boundary, normal
  and activated token deltas, prefix hash, tool schema hash, cache telemetry
  source, compact metrics, UI same-prompt A/B, and privacy/trust fields.
- Require row-level blockers from the absorption research matrix before a row
  can become implementation-ready.
- Add a bounded agent handoff and review contract for subagent-assisted audit,
  proposal, and implementation work.
- Keep Superpowers reference-only; plugin installation is explicitly out of
  scope until plugin/skill admission evidence exists.
- Keep this change process/spec focused. It must not change Analytix runtime,
  renderer, provider request behavior, or bundled application code.

## Capabilities

### New Capabilities

- `absorption-admission-gate`: Defines the required admission status, evidence
  fields, prompt/cache boundaries, row blocker handling, and stop conditions for
  future upstream absorption rows.
- `agent-handoff-review`: Defines bounded task briefs, report artifacts, review
  packages, progress ledgers, final synthesis ownership, and Superpowers
  reference-only rules for agent-assisted absorption work.

### Modified Capabilities

- None.

## Impact

- Adds OpenSpec artifacts under
  `openspec/changes/absorption-admission-gate/`.
- Uses research inputs from
  `/Users/sun/Projects/_upstreams/analytix-absorption`, especially Wave 00,
  Hard Gate Evidence, row-level admission gates, and ADR-0002.
- Future implementation may add docs, checklist wiring, or a narrow
  non-runtime matrix/admission checker after this proposal is accepted.
- No application code, runtime behavior, provider behavior, renderer UI,
  package identity, or plugin installation changes are included in this
  proposal.
