---
name: openspec-archive-change
description: Validate, synchronize, and archive a completed OpenSpec change through the canonical CLI lifecycle. Use when implementation is finished and the user wants to finalize the change safely.
---

# OpenSpec Archive Change

Archive through the OpenSpec CLI. Do not reimplement archiving with raw filesystem moves.

## Select scope

- Require `openspec` CLI 1.6.0 or newer.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on supported follow-up commands.
- Use the supplied name or a uniquely identified change in the current conversation. Otherwise inspect `openspec list --json` and ask only if the intended target is still ambiguous. Recency alone never selects a mutation target.

## Preflight

1. Run `openspec status --change "<name>" --json`.
2. Use `artifactPaths` to read the concrete tasks file when one exists.
3. Report incomplete artifacts and unchecked tasks. Ask for explicit confirmation before archiving with either condition.
4. Inspect delta specs listed in `artifactPaths.specs.existingOutputPaths` and summarize which accepted specs the archive will update.
5. Do not archive when validation or sync intent is materially unclear until the user resolves it.

## Archive

By default, run the canonical validated lifecycle:

```bash
openspec archive "<name>" --yes --json
```

Use `--skip-specs` only when the user explicitly chooses to archive without updating main specs. Never use `--no-validate` unless the user explicitly accepts the validation risk after seeing the failure.

## Verify

After success:

1. Run `openspec list --json` and confirm the change is no longer active.
2. Run `openspec doctor --json` and confirm the resolved OpenSpec root remains healthy.
3. Use the archive command's JSON and current diff to verify the archive destination and accepted spec updates.
4. Report the archived path, validation result, spec synchronization status, and any confirmed incomplete work.

## Guardrails

- Do not manually create an archive directory or move the change directory.
- Do not silently skip spec synchronization or validation.
- Do not claim completion until post-archive discovery and health checks pass.
- Preserve all files owned by the change, including its `.openspec.yaml` metadata.
