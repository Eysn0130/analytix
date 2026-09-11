---
name: openspec-apply-change
description: Implement and verify a selected, authorized OpenSpec change. Use when the user applies or continues that change; ordinary implementation does not require creating an OpenSpec workflow.
---

# OpenSpec Apply Change

Implement the selected change through verified task completion.

## Select scope

- Require `openspec` CLI 1.6.0 or newer.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on supported follow-up commands.
- Use a supplied change name. Otherwise infer it from clear conversation context, or auto-select only when exactly one active change exists.
- If selection remains ambiguous, run `openspec list --json`, present the relevant active changes, ask the user in chat, and wait.
- Announce the selected change before editing.

## Load the implementation contract

1. Run:

   ```bash
   openspec status --change "<name>" --json
   openspec instructions apply --change "<name>" --json
   ```

2. Treat `planningHome`, `changeRoot`, `actionContext`, and `contextFiles` as authoritative.
3. Read every file listed in `contextFiles` before implementing. Within a healthy session, reuse already-read unchanged artifacts; reload changed content and relevant requirements when the target changes or context is no longer available. A continuation message alone does not require replaying the full document history.
4. If the apply state is `blocked`, report the missing planning artifacts and create them through their OpenSpec instruction contracts before resuming when completing those artifacts is necessary to the already authorized change and introduces no unresolved material decision. Otherwise ask only about the missing decision and continue independent tasks.
5. If the apply state is `all_done`, verify the implementation and report that the change is ready for archive. The status command's artifact `done` or `isComplete` fields describe planning completion, not product acceptance.

## Implement pending tasks

Work in dependency order, grouping coherent tasks when that avoids repeated setup and validation. Do not treat list order as a dependency that the contract does not establish.

When authorized changes share an obligation, map it to one implementation and reuse evidence only where candidate identity and coverage remain valid. Separate checklist entries do not by themselves require duplicate implementation or validation.

For each task or coherent group:

1. State the task being implemented.
2. Inspect the relevant source of truth, affected contracts, and nearest repository instructions.
3. Make the smallest complete code and documentation change required by the task.
4. Run the narrowest validation that proves the task.
5. Mark the task `- [x]` only after implementation and validation succeed.
6. Continue until the authorized scope is done. A blocker pauses dependent work only; complete independently useful authorized tasks.

Keep existing user work and unrelated changes intact. If the implementation exposes a design contradiction, pause the affected path and resolve the contradiction through the update contract within existing authority. Ask only for a material unsettled decision; do not require a new user command merely to switch skills.

## Finish

Rerun `openspec instructions apply --change "<name>" --json` and report:

- tasks completed in this session;
- overall completed and remaining counts;
- validations run and their results;
- any unresolved blocker;
- readiness for `$openspec-archive-change` when all work is complete.

## Guardrails

- Never assume artifact filenames; use `contextFiles` and CLI-reported paths.
- Never check off a task based on partial work or unverified evidence.
- Do not silently rewrite accepted requirements to match the implementation.
- Persist through ordinary failures by diagnosing and retrying safe alternatives; pause only on a genuine decision or authority boundary.
