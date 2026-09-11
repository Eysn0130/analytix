# OpenSpec And Superpowers Practice Plan

This is the accepted practice plan for combining OpenSpec and Superpowers in
future analytix upstream absorption work.

## Final Rule

OpenSpec is the decision system. Superpowers is only an optional execution and
review technique.

They are not peers:

```text
OpenSpec decides what is allowed.
Superpowers may help execute or review allowed work.
```

If they disagree, OpenSpec wins.

## Current Operating Mode

Do not install Superpowers yet.

Use the accepted templates in this directory manually:

1. Start with an OpenSpec change.
2. Fill the Hard Gate Evidence template for every risky row.
3. If subagents are useful, dispatch them with `agent-task-brief-template.md`.
4. Require file-based reports and `agent-review-package-template.md`.
5. Track long rounds with `agent-progress-ledger-template.md`.
6. Synthesize decisions in the main thread and update OpenSpec, not
   `docs/superpowers/*`.

This gives analytix the useful Superpowers discipline without adding plugin
prompt weight, hook risk, or a second planning system.

## Allowed Superpowers-Inspired Practices

| Practice | Allowed form | Prompt/cache rule |
| --- | --- | --- |
| Brainstorming | Use only before or inside OpenSpec proposal drafting. | Conclusions must be captured in OpenSpec; broad brainstorming transcript is not durable context. |
| Planning | Translate into OpenSpec design/tasks. | Superpowers plan cannot replace OpenSpec tasks. |
| Subagent execution | Use bounded task briefs with named input files. | No full controller transcript or full upstream document dump. |
| Review loop | Use review packages with exact path findings. | Review output is a finding, not an override. |
| Progress tracking | Use a progress ledger outside prompts. | Ledger is store-only until explicitly summarized. |
| TDD/execution rhythm | Allowed only after OpenSpec tasks are accepted. | Tests must map to OpenSpec requirements. |

## Forbidden Uses

- Installing Superpowers before plugin/skill admission evidence exists.
- Treating `docs/superpowers/*` as analytix requirements.
- Letting Superpowers generate implementation scope outside OpenSpec.
- Running Superpowers hooks without trust, timeout, redaction, and prompt impact
  review.
- Passing raw child transcripts, broad accumulated summaries, or whole upstream
  documents into parent context.
- Using Superpowers worktree or finish-branch workflow as a default analytix
  process.
- Allowing Superpowers review output to override an OpenSpec requirement or
  admission verdict.

## Future Installation Gate

Superpowers may be reconsidered only after a future OpenSpec change explicitly
passes plugin/skill admission.

Required evidence:

| Evidence | Required answer |
| --- | --- |
| Plugin version and source | Exact installed version and manifest path. |
| Hook behavior | No active session-start hook, or explicit trusted hook admission. |
| Trust boundary | Workspace/plugin trust state and root confinement. |
| Prompt impact | Normal direct-answer token delta is `0` or budgeted. |
| Prefix impact | Prefix hash before/after is stable for same prompt/config. |
| Tool schema impact | Tool schema hash before/after is stable or intentionally changed. |
| Files written | No `docs/superpowers/*` source of truth unless explicitly rejected as non-authoritative notes. |
| Runtime impact | No runtime, renderer, provider, preload, packaging, or settings mutation from browsing or planning. |
| Rollback | Clear uninstall/disable path and no-op behavior. |

Until every field is accepted, Superpowers remains reference-only and
not-installed.

## Phase Contract

### Phase 1: Research Intake

Use `/Users/sun/Projects/_upstreams/analytix-absorption` as research input.
Create or update an OpenSpec proposal when a row is ready to move forward.

Superpowers role: none, or reference-only thinking discipline.

### Phase 2: Proposal, Design, Specs

OpenSpec owns all durable planning artifacts.

Superpowers role: none. Any generated ideas must be rewritten into OpenSpec
language and reviewed against the admission gate.

### Phase 3: Apply

Apply only accepted OpenSpec tasks. If a task is broad enough for agents, create
task briefs from the accepted OpenSpec task list.

Superpowers role: optional local execution rhythm after plugin admission, or
manual use of the accepted templates before plugin admission.

### Phase 4: Review

Review against OpenSpec requirements, Hard Gate Evidence, and path-based
findings.

Superpowers role: optional reviewer-loop structure only. Reviewer output cannot
override OpenSpec.

### Phase 5: Archive

Archive the OpenSpec change so accepted requirements move into
`openspec/specs`.

Superpowers role: none.

## Next Absorption Flow

For the next P0 proposal, use this order:

```text
select row
  -> fill Hard Gate Evidence
  -> create OpenSpec change
  -> write specs/design/tasks
  -> optionally dispatch bounded agent tasks
  -> review package
  -> apply
  -> validate
  -> archive
```

Recommended next rows:

1. `context-epoch-foundation`
2. `tool-output-budget-and-prune`

Both must cite this directory before implementation.

## Conflict Rule

When a Superpowers-derived recommendation conflicts with OpenSpec:

1. Keep OpenSpec unchanged.
2. Record the Superpowers output as a review finding.
3. Decide whether to update OpenSpec through a new proposal or reject the
   finding.
4. Never implement the conflicting recommendation directly.

