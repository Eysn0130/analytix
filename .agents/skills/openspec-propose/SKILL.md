---
name: openspec-propose
description: Create an OpenSpec change and all planning artifacts required for implementation. Use when the user wants a complete proposal, design, delta specs, and task plan for a new feature or fix.
---

# OpenSpec Propose

Create a change and make it implementation-ready. Do not edit application code.

## Resolve planning scope

- Require `openspec` CLI 1.6.0 or newer.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on every supported follow-up command.
- Otherwise use the nearest repository OpenSpec root.
- Treat CLI JSON fields such as `planningHome`, `changeRoot`, `artifactPaths`, and `actionContext` as authoritative for paths and edit scope.

## Create the change

1. Establish a clear change intent. Ask the user in chat and wait only when missing information would materially change scope or behavior.
2. Derive a concise kebab-case name from the request when no name is supplied.
3. Run `openspec list --json` for name discovery. If context already identifies an existing change to extend or complete, reuse it with the update workflow and its CLI-reported missing-artifact contracts. Ask only when a name collision leaves revise-versus-new intent unresolved; do not overwrite another change.
4. Create the scaffold:

   ```bash
   openspec new change "<name>"
   ```

5. Inspect the artifact graph:

   ```bash
   openspec status --change "<name>" --json
   ```

## Build artifacts to apply-ready

Repeat until the CLI/schema reports the `applyRequires` dependencies satisfied. A legitimate `skipped` artifact may satisfy a dependency just as `done` does; do not manufacture its output or loop forever. If the graph has no ready action but remains blocked, resolve the reported dependency rather than repeating status calls:

1. Select an artifact whose status is `ready`.
2. Load its schema-specific contract:

   ```bash
   openspec instructions <artifact-id> --change "<name>" --json
   ```

3. Read relevant completed dependency outputs before drafting; do not attempt to read a legitimately skipped artifact. Reuse unchanged context already read.
4. Use `template` as the structure and obey `instruction`, `context`, and `rules`. Treat `context` and `rules` as constraints; never copy their wrapper blocks into the artifact.
5. Write to the concrete path required by the instructions. If `resolvedOutputPath` contains a glob, derive concrete files from the instruction instead of writing to the glob literal.
6. Verify the new files exist, then rerun `openspec status --change "<name>" --json` before continuing.

Use Codex's native planning mechanism for this multi-step loop when available. Keep progress visible and concise.

## Finish

Run `openspec status --change "<name>"` and report:

- the change name and resolved location;
- every artifact created;
- any assumptions or remaining uncertainty;
- readiness for `$openspec-apply-change`.

## Guardrails

- Create all artifacts required by the active schema, not a hardcoded filename set.
- Keep requirements testable and preserve the schema's exact format rules.
- Stop on a real design conflict or unsafe ambiguity; otherwise make reversible assumptions and continue.
- Never implement the planned code in this skill.
