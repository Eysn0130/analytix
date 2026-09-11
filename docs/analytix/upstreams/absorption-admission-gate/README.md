# Absorption Admission Gate

This directory is the accepted repo-local process gate for future analytix
upstream absorption work.

The gate turns research from
`/Users/sun/Projects/_upstreams/analytix-absorption` into a strict OpenSpec
entry path. A future absorption row must be admitted here before it becomes an
implementation task.

## Source Of Truth

OpenSpec is the source of truth for analytix absorption scope, requirements,
design, tasks, admission verdicts, and final acceptance.

Superpowers is not a parallel planning system. Its useful ideas are absorbed
only as optional execution and review mechanics:

- bounded task briefs;
- file-based report artifacts;
- review packages with path-based findings;
- progress ledgers outside runtime prompts;
- fresh, narrow subagent context.

If Superpowers output conflicts with an accepted OpenSpec requirement,
OpenSpec controls and the Superpowers output becomes a review finding.

## Phase Model

| Phase | OpenSpec role | Superpowers role |
| --- | --- | --- |
| Research intake | Owns capability rows, prompt boundary, blockers, and admission evidence. | Reference-only method for bounded briefs and report handoffs. |
| Proposal/design/spec | Authoritative artifact location and acceptance contract. | Not a source of truth; do not create `docs/superpowers/*` requirements. |
| Apply implementation | Owns task list and done criteria. | Optional local working method only after OpenSpec scope is accepted and plugin admission allows it. |
| Review | Owns requirement compliance and gate status. | Optional reviewer loop using the OpenSpec packet, report files, and exact path findings. |
| Plugin installation | Requires OpenSpec admission first. | Blocked until manifest, hooks, trust, prompt, prefix, and tool-schema impact are reviewed. |

## Verdicts

| Verdict | Meaning |
| --- | --- |
| `ready` | All required evidence exists and row-level blockers are clear. |
| `blocked` | Evidence is missing, a dependency is absent, or a stop condition is true. |
| `not-applicable` | The row does not touch the gate area and explains why. |

Unknown cache telemetry, unmeasured prompt weight, unknown prompt boundary, or
unreviewed plugin/subagent behavior must never be treated as `ready`.

## Required Evidence

Use `hard-gate-evidence-template.md` for every P0/P1 row that touches provider
history, context, tools, MCP, memory, instructions, compaction, hooks,
skills/plugins, subagents, runtime bridge, or UI polling.

At minimum, risky rows must answer:

- prompt boundary;
- normal-turn token delta;
- activated-turn token delta;
- prefix hash before/after;
- tool schema hash before/after;
- cache telemetry source;
- compact frequency and summary model calls when applicable;
- UI same-prompt open/closed A/B when runtime-adjacent UI is involved;
- trust, redaction, and secret handling;
- task brief, report artifact, review package, and progress ledger when
  subagents or reviewers are used.

## Stop Conditions

Block implementation when:

- the prompt boundary is unknown;
- Hard Gate Evidence is missing for a high-risk row;
- the row requires Context Epoch and Context Epoch does not exist;
- unknown cache telemetry is reported as `0`;
- a UI or diagnostic surface mutates prompt history, prefix, request shape, or
  tool schema without same-prompt A/B proof;
- a subagent handoff depends on raw transcript replay or broad pasted context;
- a proposal requires installing Superpowers before plugin/skill admission
  evidence exists;
- a proposal claims live Reasonix or DeepSeek parity without credentialed live
  evidence.

## Accepted Templates

| Template | Purpose |
| --- | --- |
| `hard-gate-evidence-template.md` | Standard evidence shape for risky rows. |
| `agent-task-brief-template.md` | Bounded brief for subagent-assisted research or implementation. |
| `agent-review-package-template.md` | Reviewer input and verdict format. |
| `agent-progress-ledger-template.md` | Durable state for long-running or multi-agent rounds. |
| `validation-examples.md` | Example `not-applicable` and `blocked` verdicts. |
| `openspec-superpowers-practice-plan.md` | Final practice plan for combining OpenSpec and Superpowers without creating a second source of truth. |

## Validation Path

This apply is docs-only. No checker is introduced yet.

The next safe step is to manually run this gate on one low-risk row and one
high-risk row. After that, analytix can decide whether a narrow non-runtime
matrix/admission checker is worth adding. Any checker must operate only on
OpenSpec and research artifacts; it must not run in runtime, renderer,
provider, preload, packaging, or plugin execution paths.

## Next P0 Consumers

The next OpenSpec proposals should reference this gate before implementation:

- `context-epoch-foundation`;
- `tool-output-budget-and-prune`.
