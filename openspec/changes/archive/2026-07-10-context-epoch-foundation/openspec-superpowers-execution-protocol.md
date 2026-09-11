# OpenSpec And Superpowers Execution Protocol

This protocol applies only to `context-epoch-foundation`.

## Final Authority

OpenSpec is the source of truth for this change:

- `proposal.md` defines why the change exists.
- `design.md` defines the accepted architecture.
- `specs/context-epoch/spec.md` defines required behavior.
- `tasks.md` defines implementation order.
- `hard-gate-evidence.md` controls whether implementation is admitted.

Superpowers is not installed and is not a source of requirements.

## Current Mode

Use Superpowers methodology manually and partially:

1. Create bounded task briefs from `tasks.md`.
2. Give agents exact file paths and exact output schemas.
3. Require report files instead of broad returned prose.
4. Require review packages with exact path findings.
5. Keep a progress ledger for multi-agent or long-running rounds.
6. Synthesize decisions in the main thread and update OpenSpec artifacts.

Do not create `docs/superpowers/*` requirements or plans for this change.

## Allowed Uses

| Superpowers method | Allowed in this change | Constraint |
| --- | --- | --- |
| Brainstorming | Already converted into OpenSpec artifacts. | Future ideas must update OpenSpec, not create a parallel plan. |
| Writing plans | Use `tasks.md` as the plan. | Do not duplicate tasks into `docs/superpowers/plans`. |
| Subagent-driven development | Allowed only after Hard Gate Evidence is accepted or the user explicitly starts apply work. | Use bounded briefs, report files, review packages, and explicit model choice. |
| TDD | Required for behavior changes during apply. | Tests must map to `specs/context-epoch/spec.md`. |
| Code review | Required before archive. | Review reads paths/packages, not parent conversation history. |
| Worktrees/finish branch | Not default. | Use Codex/app/native git flow only if the user asks. |

## Forbidden Uses

- Installing Superpowers before plugin/skill admission evidence exists.
- Loading Superpowers bootstrap into every session for this change.
- Running hooks or marketplace install flows.
- Letting Superpowers write authoritative requirements.
- Dispatching subagents with controller history, raw upstream docs, full
  transcripts, or broad summaries.
- Asking reviewers to ignore OpenSpec requirements.
- Treating a reviewer report as an override of OpenSpec.

## Task Brief Contract

Every implementation or review lane must include:

- `parent_openspec_change: context-epoch-foundation`
- exact task id from `tasks.md`
- exact requirements from `specs/context-epoch/spec.md`
- exact allowed input paths
- forbidden assumptions
- required report path
- required review package path
- pass/fail criteria

## Review Contract

Reviewers must check:

- spec compliance against `specs/context-epoch/spec.md`;
- no stable-prefix drift for default epoch;
- no tool-schema drift unless explicitly justified;
- no raw source content or local diagnostics in prompt-visible data;
- no UI/cache mutation added outside scope;
- Hard Gate Evidence fields completed or still correctly blocked;
- tests and gate commands are named with results.

## Go / No-Go

Implementation may begin only when one of these is true:

1. `hard-gate-evidence.md` is updated from `blocked` to `ready`, with measured
   baseline evidence; or
2. the implementation starts specifically with the evidence-producing tasks in
   Section 1 of `tasks.md`.

Archive may happen only after:

- all `tasks.md` items are complete;
- `hard-gate-evidence.md` is accepted;
- `openspec validate context-epoch-foundation` passes;
- relevant runtime tests and speed/cache gate evidence are attached;
- review package findings are resolved or explicitly accepted.
