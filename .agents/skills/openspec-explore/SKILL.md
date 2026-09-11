---
name: openspec-explore
description: Explore OpenSpec requirements, design choices, or a named change when the user requests investigation or options. Ordinary diagnosis with an instruction to fix is not exploration-only.
---

# OpenSpec Explore

Act as a grounded thinking partner. During an exploration-only request, investigate without implementing application code. If the user then authorizes implementation, continue in implementation mode with the relevant guide or apply skill; no special exit command is required.
This is a stance, not a fixed workflow: there is no required sequence, artifact, or conclusion.

## Resolve context

- Require `openspec` CLI 1.6.0 or newer when OpenSpec commands are needed.
- If the user names an OpenSpec store, run `openspec store list --json`, select its id, and preserve `--store <id>` on supported follow-up commands.
- Otherwise use the nearest repository OpenSpec root.
- Use `openspec list --json` when change discovery would help answer the current question; do not require it for a self-contained investigation.
- From the resolved root, read `openspec/config.yaml` (or `config.yml`) when present. Use `context` as project background; apply an artifact's `rules` only when writing that artifact, and never reproduce either as output.
- When the user mentions a change or the conversation clearly identifies one, run `openspec status --change "<name>" --json`. Use `changeRoot`, `artifactPaths`, and `actionContext` as the scope contract, and read relevant files from `artifactPaths.<id>.existingOutputPaths`.
- If several changes could be relevant, keep exploring generally or ask a natural clarifying question. Never infer an artifact write target from recency alone.

## Explore

- Ground reasoning in the current code, accepted specs, and existing change artifacts.
- Surface assumptions, constraints, risks, unknowns, and integration points.
- Compare viable approaches and explain tradeoffs at the user's level.
- Use a compact diagram or table only when it materially clarifies relationships.
- Ask questions that unlock a decision; avoid a scripted interrogation.
- Challenge inconsistencies between the idea, code, and accepted requirements.

## Capture decisions

Offer to update OpenSpec artifacts when the discussion produces a durable decision:

- scope or motivation -> proposal;
- behavior or acceptance criteria -> delta spec;
- architecture or tradeoff -> design;
- newly identified work -> tasks.

Do not auto-capture. Exploration or agreement with an idea is not permission to write. When the user explicitly asks to capture a decision, resolve the target change and artifact scope from context (ask only if ambiguous), then use CLI-reported concrete paths and the relevant `openspec instructions <artifact-id> --change "<name>" --json` contract. Never write to a glob literal.

Keep capture proportional: a narrow requested artifact edit may stay in explore; use `$openspec-propose` for a new complete change and `$openspec-update-change` for a broader reconciliation of an existing change. Neither handoff is required merely to continue thinking.

## Finish

When useful, summarize the current understanding, recommended direction, and unresolved questions. Suggest `$openspec-propose`, `$openspec-update-change`, or `$openspec-apply-change` only when that action follows from the discussion; reaching a conclusion is optional.

## Guardrails

- An exploration-only request does not authorize implementation, destructive commands, or task completion. A later explicit implementation request changes the phase, not the user's safety boundaries.
- Do not invent project facts; inspect the codebase when evidence is available.
- Do not force the discussion into an OpenSpec artifact before the idea is ready.
- Do not turn exploration into a mandatory planning ceremony.
