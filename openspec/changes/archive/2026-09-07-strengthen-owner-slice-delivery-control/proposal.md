## Why

The instruction overhaul removed many local rituals, but the resumed Owner/AH audit exposes remaining coordination gaps: repeated waiting, oversized task reads, ambiguous task identities, and no complete heartbeat or safe Slice-reuse contract. Efficient delivery needs a thin Owner, one active construction authority and explicit evidence-based recovery without lowering accepted outcomes.

## What Changes

- Define Owner orchestration, optional Controller epochs, result-based Slice follow-ups and milestone closure.
- Use actionable events with an optional authorized low-frequency heartbeat backstop, persisted cursors and bounded summaries; no quiet-state polling loop or implicit scheduler installation.
- Separate ordinary brief completion from explicit permanent task retirement, preserving legacy terminal fences.
- Define safe Controller/Owner handoff, scheduler ownership transfer and stale identity rejection.
- Correct OpenSpec existing-change, approved removal and legitimate skipped-artifact handling; repair demonstrated remaining host instruction conflicts.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-handoff-review`: extend bounded delivery governance to Owner orchestration, monitoring, follow-up and safe continuity.

## Impact

RC Skill, its conditional references and templates, canonical OpenSpec guidance, and narrowly affected host Skills. No Analytix production code, product requirements, model/effort or sandbox/approval changes. No product task is resumed, no new peer task or live heartbeat is created, and no external deployment or upstream framework is installed by this maintenance change.
