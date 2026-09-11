## Why

Analytix needs an explicit Context Epoch boundary before absorbing memory,
instruction discovery, compaction recovery, tool-output pruning, or plugin
work. Without that boundary, dynamic context can silently drift into every
turn, destabilize the stable prefix, and make speed/cache regressions hard to
diagnose.

This change prepares the first Wave 1 implementation packet after the
absorption admission gate. It defines Context Epoch as the source registry,
snapshot, reconcile/replace, and prefix-change reason layer that keeps
dynamic context intentional and measurable.

## What Changes

- Introduce a `context-epoch` capability covering source registry entries,
  epoch snapshots, baseline sequence tracking, safe reconcile/replace points,
  prefix-change reason reporting, and restart/compact recovery behavior.
- Require Context Epoch state to remain outside the stable prefix unless an
  explicit epoch bump changes provider-visible context.
- Require same-prompt prefix/tool-schema evidence before and after Context
  Epoch activation.
- Define Context Epoch as a prerequisite boundary for future memory,
  instruction discovery, compaction recovery, and context-budget work.
- Preserve the OpenSpec/Superpowers split: OpenSpec owns the proposal, design,
  requirements, admission verdicts, and archive; Superpowers-inspired methods
  may only be used later for bounded task briefs, report files, review
  packages, and progress ledgers.

## Capabilities

### New Capabilities

- `context-epoch`: Defines source registry, epoch snapshot, reconcile/replace,
  prefix reason, prompt-boundary, and recovery rules for dynamic context.

### Modified Capabilities

- None.

## Impact

- Future runtime landing is expected in `packages/runtime-go/internal/app/model`,
  `packages/runtime-go/internal/app/thread`, provider history composition, and
  durable thread metadata.
- Future desktop/shared impact is diagnostics-only unless an explicit UI
  surface is later proposed.
- Future validation must include prefix hash before/after, tool schema hash
  before/after, normal-turn token delta, compact/restart behavior, and
  `npm run runtime:go:speed-cache-gate` or an explicit blocked gate note.
- This proposal does not install Superpowers and does not modify runtime,
  renderer, main, preload, provider, settings, or packaging code.
