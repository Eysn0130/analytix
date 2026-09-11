## 1. Accepted Admission Artifacts

- [x] 1.1 Add or update accepted absorption admission documentation that links to the Wave 00 packet, Reasonix speed/cache contract, row-level admission gates, and no-omission checklist.
- [x] 1.2 Add or update the accepted Hard Gate Evidence template with prompt boundary, token delta, prefix hash, tool schema hash, cache telemetry, compact metrics, UI A/B, agent handoff, and privacy/trust fields.
- [x] 1.3 Define the accepted row verdicts `ready`, `blocked`, and `not-applicable`, including the rule that unknown telemetry or unmeasured prompt weight cannot be `ready`.
- [x] 1.4 Define the accepted stop conditions that block implementation when prompt boundary, Context Epoch dependency, Hard Gate Evidence, Superpowers dependency, or live Reasonix evidence is missing.

## 2. Agent Handoff And Review

- [x] 2.1 Add or update the accepted bounded task brief template for subagent-assisted absorption work.
- [x] 2.2 Add or update the accepted report artifact and review package template with path-based findings and pass/blocked/needs-revision verdicts.
- [x] 2.3 Add or update the accepted progress ledger template for long-running or multi-agent absorption rounds.
- [x] 2.4 Document that the main thread owns matrix updates, conflict resolution, OpenSpec packet creation, and final admission decisions.
- [x] 2.5 Document that Superpowers remains reference-only and blocked from installation until plugin/skill admission evidence exists.
- [x] 2.6 Document the OpenSpec/Superpowers phase model: OpenSpec owns source of truth and admission; Superpowers can only assist approved execution or review after admission.

## 3. Validation Path

- [x] 3.1 Record whether this apply remains docs-only or includes a narrow non-runtime matrix/admission checker.
- [x] 3.2 If a checker is included, constrain it to OpenSpec/research artifacts and keep it out of runtime, renderer, provider request, preload, packaging, and plugin execution paths.
- [x] 3.3 Validate one low-risk `not-applicable` example and one high-risk `blocked` example against the accepted Hard Gate Evidence shape.
- [x] 3.4 Verify future P0 proposals such as `context-epoch-foundation` and `tool-output-budget-and-prune` can reference the accepted admission gate.

## 4. Guardrails And Verification

- [x] 4.1 Confirm no Analytix application code, runtime behavior, renderer UI, provider behavior, settings schema, package identity, or plugin installation is changed by this apply.
- [x] 4.2 Run `openspec status --change absorption-admission-gate` and confirm all required artifacts are complete before apply work starts.
- [x] 4.3 Run `git diff --check` after apply edits.
- [x] 4.4 Update the absorption research progress note with final accepted artifact paths and any deferred checker decision.
