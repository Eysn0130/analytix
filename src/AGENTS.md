# analytix Desktop Agent Guide

This guide adds desktop-specific guidance for `src/`. Inherit the repository
rules from the root `AGENTS.md`.

## Ownership And Stable Seams

- `src/renderer/src`: React UI, interaction, projection, and virtualization.
- `src/main`: Electron lifecycle, OS integration, runtime process, persistence,
  provider connectivity, and IPC.
- `src/preload`: the context-isolated renderer bridge.
- `src/shared`: cross-layer settings, IPC, and desktop types.

Use the bridge and settings identities from the root product anchors; write
only the current settings shape.

## Complete Observable Changes

For behavior or contract changes, trace the relevant live call graph before
choosing files; text or styling edits do not require unrelated runtime
inspection. For runtime-facing changes, use the root's applicable API slice
and only the consumers the contract reaches. Keep Go-private routes and
upstream-provider protocols behind main/preload-owned desktop contracts.

## Providers And Model Requests

Provider changes to URLs, endpoint formats, profiles, headers, bodies,
streaming, usage, cache, errors, or reasoning are cross-runtime contracts.
Confirm current producers and consumers rather than relying on a static file
inventory.

- Keep URL selection separate from request-body construction.
- A custom full endpoint keeps its explicit path; a blank base URL uses the
  selected provider default.
- Endpoint format governs the whole applicable request/response path, not just
  URL construction.
- Diagnose model-request failures with sanitized provider, model, format, and
  status evidence; never log credentials or unsafe bodies.
- A schema accepting or persisting a field does not prove the production Go
  runtime consumes it.

## Renderer And UX

- Preserve observable product entries, keyboard and pointer interactions,
  focus, selection, accessibility, and scroll position unless the accepted
  target changes them.
- Streaming behavior spans runtime events, projection, active buffers,
  scheduling, virtualization, measurement, and scroll anchoring. Verify the
  coupled path; never force a reader away from history as tokens arrive.
- Preserve `localFilePath` / `FilePath` through attachment storage, runtime
  events, desktop contracts, generated-file projections, and timeline media
  when local media is part of the behavior.
- The desktop contract includes lifecycle, turns, steering and interruption,
  compaction, approvals, user input, goals/todos/jobs, usage, attachments,
  skills, diagnostics, and workspace state where applicable; chat rendering is
  not the only consumer.
- Execution-policy UI and migrations must not silently widen effective
  foreground, unattended, or child-agent authority. Resolve current semantics
  from accepted specs, Go policy code, and focused tests.

## Validation

For ordinary maintenance, use [bounded owner routing](../docs/analytix/development-baseline.md#bounded-maintenance-routing);
the mapper covers docs and two fixed fixtures, not all affected dependencies.

Use focused tests for the changed seam. Cross-layer settings, provider,
runtime, or preload changes need the affected type checks. Add the appropriate
build when bundling, generated output, or integration could change; do not run
both a package build and its equivalent aggregate wrapper. UI and packaged
claims require the corresponding runtime or UI evidence. Source `./scripts/use-analytix-cache.sh` for commands as required by
the root guide and use `docs/analytix/development-runbook.md` for wider gates.
