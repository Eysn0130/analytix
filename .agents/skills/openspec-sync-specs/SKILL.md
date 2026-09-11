---
name: openspec-sync-specs
description: Merge delta specs from an active OpenSpec change into accepted main specs without archiving the change. Use when the user wants an idempotent spec synchronization with validation and a clear change summary.
---

# OpenSpec Sync Specs

Merge delta intent into accepted main specs while leaving the change active.

## Select scope

- Require `openspec` CLI 1.6.0 or newer.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on supported follow-up commands.
- Use the supplied name or a uniquely identified change in the current conversation. Otherwise inspect `openspec list --json` and ask only if the intended target is still ambiguous. Recency alone never selects a mutation target.

## Resolve paths

Run:

```bash
openspec status --change "<name>" --json
```

- Use `artifactPaths.specs.existingOutputPaths` as the complete delta spec file list.
- Use `changeRoot`, `planningHome.root`, and `actionContext` to enforce scope.
- For the standard layout, derive the capability from `<changeRoot>/specs/<capability>/spec.md` and target `<planningHome.root>/openspec/specs/<capability>/spec.md`.
- If the selected store or schema reports a different layout, follow its CLI paths and instructions instead of assuming the standard path.

Stop with a clear message when no delta specs exist.

## Merge each capability

Read the delta and current main spec before writing. Apply operations as intent, not as blind file replacement:

- `ADDED`: add a missing requirement; if it already exists, reconcile it idempotently.
- `MODIFIED`: update the matching requirement while preserving scenarios and details not superseded by the full delta requirement.
- `REMOVED`: remove the complete matching requirement block, including its scenarios.
- `RENAMED`: rename the exact source requirement to the target name without changing unrelated content.

For a new capability, create its main `spec.md` with the required purpose and requirements structure. Ask before writing only when mapping is ambiguous or accepted behavior would be lost without being explicitly superseded by the approved delta. An already approved removal or replacement does not require approval again.

After each capability, validate it:

```bash
openspec validate "<capability>" --type spec --strict --json
```

Fix regressions introduced by the merge before continuing. Preserve pre-existing unrelated failures in the report.

## Verify idempotence

Re-read each delta and merged main spec. Confirm that applying the same operations again would produce no additional change. Review the final diff for unintended edits.

## Finish

Report every capability and requirement added, modified, removed, or renamed; list validation results and any baseline caveat; confirm that the change remains active and can later use `$openspec-archive-change`.

## Guardrails

- Never replace a whole main spec with a partial delta.
- Never drop accepted scenarios not superseded by the delta.
- Never claim sync success while affected specs fail because of the new merge.
- Keep edits inside the CLI-reported planning root.
