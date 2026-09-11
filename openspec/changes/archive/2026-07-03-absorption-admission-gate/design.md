## Context

Analytix has an upstream absorption research workspace at
`/Users/sun/Projects/_upstreams/analytix-absorption`. That workspace already
defines the capability taxonomy, best-of absorption matrix, row-level admission
gates, Reasonix speed/cache preservation contract, Wave 00 preparation packet,
Hard Gate Evidence template, and bounded agent handoff templates.

The missing piece is a repo-local OpenSpec change that turns those research
rules into accepted requirements before implementation begins. Without that
gate, a future feature could enter implementation while its prompt boundary,
normal-turn token delta, prefix/tool hash impact, UI cache neutrality, or
subagent transcript boundary is unknown.

This design is intentionally process/spec first. It does not alter Analytix
runtime behavior, renderer behavior, provider requests, package identity, or
plugin installation state.

## Goals / Non-Goals

**Goals:**

- Define a mandatory admission gate for future upstream absorption rows.
- Require Hard Gate Evidence or an explicit blocked status for risky rows.
- Standardize the `ready`, `blocked`, and `not-applicable` verdicts.
- Preserve Reasonix-grade first feedback, cache behavior, stable prefix shape,
  and canonical tool schema discipline.
- Make bounded task briefs, report artifacts, review packages, and progress
  ledgers mandatory when subagents or reviewers are used.
- Keep Superpowers reference-only until plugin/skill admission is explicitly
  accepted later.

**Non-Goals:**

- Implement a runtime feature.
- Change provider request bodies, streaming, model history, tool schemas,
  renderer panels, settings, preload bridges, or packaged app behavior.
- Install Superpowers or any other plugin.
- Claim live DeepSeek/Reasonix parity from local fixtures.
- Copy upstream product identity, config roots, routes, or raw implementation.

## Decisions

### Decision: Use OpenSpec As The Accepted Gate

The accepted gate will live in OpenSpec artifacts, not in Superpowers docs or
ad hoc research notes. The `_upstreams` workspace remains the evidence and
preparation source; `openspec/changes/absorption-admission-gate` becomes the
repo-local source of truth for the next apply step.

Alternative considered: keep all gate rules only in `_upstreams`. That is too
easy to bypass during implementation because apply threads naturally look first
at OpenSpec artifacts.

### Decision: Keep Gate Status Explicit

Every future absorption row must resolve to one of:

| Status | Meaning |
| --- | --- |
| `ready` | All required evidence exists and row-level blockers are clear. |
| `blocked` | Evidence is missing, a dependency is absent, or a stop condition is true. |
| `not-applicable` | The row does not touch the gate area and explains why. |

Unknown telemetry, unmeasured prompt weight, or unreviewed plugin/subagent
behavior must not be treated as `ready`.

### Decision: Require Hard Gate Evidence For Risky Rows

The gate will use the Hard Gate Evidence shape prepared in
`templates/hard-gate-evidence-template.md`. Required evidence includes prompt
boundary, token deltas, prefix hash, tool schema hash, provider-native cache
telemetry source, compact metrics, UI same-prompt A/B, trust/redaction, and
agent handoff fields where applicable.

Alternative considered: a checklist-only gate. A checklist is useful, but it is
not enough for rows touching model history, tool schemas, memory, compaction,
MCP, plugins, hooks, subagents, or runtime-adjacent UI.

### Decision: Treat Agent Handoff As A Prompt Boundary

Superpowers-derived practices are absorbed only as bounded handoff mechanics:
task brief, report file, review package, progress ledger, and final synthesis
ownership. Subagents must receive named files and narrow instructions, not the
controller's full transcript or broad upstream document dumps.

Alternative considered: install Superpowers and use its native workflow. That
is deferred because analytix already uses OpenSpec and because plugin/skill
loading itself requires admission evidence.

### Decision: Combine OpenSpec And Superpowers By Phase

OpenSpec and Superpowers must not be peers. OpenSpec is the decision system and
source of truth; Superpowers is, at most, an optional execution and review
method used after OpenSpec has accepted scope, requirements, design, and tasks.

| Phase | OpenSpec role | Superpowers role |
| --- | --- | --- |
| Research intake | Owns capability rows, prompt boundary, blockers, and admission evidence. | Reference-only method for bounded briefs and report handoffs. |
| Proposal/design/spec | Authoritative artifact location and acceptance contract. | Not used as a source of truth; no `docs/superpowers/*` requirements. |
| Apply implementation | Owns task list and done criteria. | Optional local working method for executing approved tasks, if plugin admission allows it. |
| Review | Owns requirement compliance and gate status. | Optional reviewer-loop method using OpenSpec packet, report files, and exact path findings. |
| Plugin installation | Requires OpenSpec admission first. | Blocked until manifest, hook, trust, prompt, prefix, and tool-schema impact are reviewed. |

This keeps Superpowers useful without letting it create a second planning
system. If the two disagree, OpenSpec wins.

### Decision: Start Docs/Process First, Then Optional Checker

The first apply should update accepted docs/templates and may add a narrow
non-runtime checker only if the scope remains small. Any checker must operate on
OpenSpec/research artifacts and must not run as part of the Analytix runtime
prompt path.

## Risks / Trade-offs

- Gate becomes paperwork only -> Mitigation: specs require blocking statuses,
  evidence fields, and reviewer verdicts before a row is implementation-ready.
- Gate slows down useful work -> Mitigation: low-risk rows can use
  `not-applicable` with evidence; high-risk rows must pay the review cost.
- Evidence is faked or too vague -> Mitigation: scenarios require exact
  artifact paths, provider-native telemetry source, and explicit unknown
  handling.
- Subagent reports bloat parent context -> Mitigation: report files and terse
  status paths are mandatory; raw transcript handoff is forbidden.
- Superpowers becomes a parallel source of truth -> Mitigation: installation is
  blocked until plugin admission exists, and OpenSpec remains authoritative.

## Migration Plan

1. Accept this OpenSpec proposal.
2. Apply docs/process changes that wire the Wave 00 packet, Hard Gate Evidence
   template, no-omission checklist, and agent handoff templates into the
   repo-local absorption workflow.
3. Optionally add a narrow non-runtime admission checker after the docs contract
   is accepted.
4. Use the gate for the next P0 proposals: `context-epoch-foundation` and
   `tool-output-budget-and-prune`.

Rollback is simple: archive or revise the OpenSpec change before apply. No
runtime migration is needed because this change does not alter application
state.

## Open Questions

- Should the first apply include a checker script, or should it remain docs-only
  until one row has been manually reviewed with the new gate?
- Where should accepted Hard Gate Evidence artifacts live after apply:
  `docs/analytix/upstreams`, `openspec/changes/<change>/evidence`, or both?
- Should `not-applicable` verdicts require a reviewer signature for P0 rows?
