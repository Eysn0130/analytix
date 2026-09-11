---
name: openspec-update-change
description: Revise an existing OpenSpec change and reconcile its planning artifacts without editing implementation code. Use when decisions, scope, requirements, design, or tasks have changed and the plan must become coherent again.
---

# OpenSpec Update Change

Revise existing planning artifacts and restore consistency. Never edit implementation code in this skill.

## Select scope

- Require `openspec` CLI 1.6.0 or newer.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on supported follow-up commands.
- Use the supplied name or a uniquely identified change in the current conversation. Otherwise inspect `openspec list --json` and ask only if the intended target is still ambiguous. Recency alone never selects a mutation target.

## Load the change

Run:

```bash
openspec status --change "<name>" --json
```

Use these fields as the contract:

- `schemaName` and `artifacts` for workflow state;
- `planningHome`, `changeRoot`, and `actionContext` for scope;
- `artifactPaths.<id>.existingOutputPaths` for files that may be edited.

Never write to a glob-valued `resolvedOutputPath` or assume artifact ids from the default schema. If completing the authorized revision requires a missing artifact, obtain its CLI instruction contract and concrete output path before creating it. Ask only if doing so introduces an unresolved material decision or exceeds the requested planning scope.

## Reconcile artifacts

1. Read every existing artifact that the requested revision may affect.
2. If the user supplied a concrete revision, start there. Otherwise perform a coherence review for contradictions, gaps, stale decisions, and duplicated work.
3. Trace the revision in every direction across existing artifacts. A design or task change may require an earlier proposal or requirement update.
4. Reconcile all affected artifacts within the authorized revision. A request for review-only stops at proposed edits; approved edits and routine propagation do not require per-artifact confirmation. Ask only about an unresolved material choice, and continue independent authorized work.
5. For a substantial rewrite, load its current contract first:

   ```bash
   openspec instructions <artifact-id> --change "<name>" --json
   ```

6. Apply authorized edits to concrete CLI-reported files, preserving accepted content outside the revision.
7. Rerun status and validate the change:

   ```bash
   openspec validate "<name>" --type change --strict --json
   ```

If validation exposes an existing unrelated baseline issue, distinguish it from the revision instead of hiding it.

## Finish

Report revised and rejected artifacts, deferred missing artifacts, validation results, and the appropriate next action:

- create missing planning artifacts through their OpenSpec instruction contracts;
- use `$openspec-apply-change` when implementation must catch up;
- use `$openspec-archive-change` when planning and implementation are complete;
- use `$openspec-propose` when the intent has changed enough to require a separate change.

## Guardrails

- Edit planning artifacts only.
- Create missing artifacts only through their schema and CLI contracts within authorized planning scope.
- Preserve coherent accepted content not affected by the revision.
- Do not convert a material scope change into a silent wording edit.
