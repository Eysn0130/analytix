# Casegraph Minimum Roadmap

Use this reference when improving analytix casegraph without a broad rewrite. The goal is a compact evidence graph that helps Codex reason over the current case project with fewer tool calls, while keeping every factual claim tied to deterministic facts.

## Current Boundary

- Do not replace `funds_investigate` with raw graph wandering. Casegraph is a fact substrate behind the front door.
- Do not expose internal ids, audit refs, query ids, artifact ids, or evidence refs in user-facing answers.
- Do not draw Mermaid edges unless a returned transaction edge is supported.
- Do not use golden/oracle facts in production paths.

## Minimum Nodes And Edges

- Subject nodes: holder name plus id number when available. Name-only groups are diagnostic, not report-grade identity.
- Account nodes: normalized account/card key, holder, id number, direct/candidate status, registration status, and quality flags.
- Counterparty nodes: counterparty name/account key, blank-name/account-missing class, unit/person/account-only class, and match confidence.
- Transaction edges: direction, amount, time, source account, counterparty, txn id status, edge status, and source scope.
- Same-fact family nodes: duplicate txn id, same-holder cross-account candidate, card-replacement candidate, natural within-account duplicate.
- Rule-hit nodes: data-quality gate, account identity gate, counterparty gap gate, report gate, claim-review gate.
- Asset/cash break nodes: wealth-management subscription/redemption, loan/repayment, transfer/deposit, cash withdrawal/deposit, missing counterparty break.
- Evidence pack nodes: bounded row sample, deterministic aggregate, supported flow edge, source boundary, and missing source boundary.
- Claim support nodes: verified claim, corrected claim, unsupported claim, forbidden phrasing, next review action.

## Minimal Next Shape

1. Keep rank facts as graph node summaries: top accounts, holders, counterparties, and metric-specific rankings must carry metric and scope.
2. Keep trace facts as supported edge packs: one-hop and next-hop facts must carry edge status and stop conditions.
3. Keep claim facts as review packs: verified/corrected/unsupported/boundary/forbidden must point to the deterministic fact family, not report prose alone.
4. Keep quality risks as first-class graph facts: same-fact family, blank holder, missing counterparty, account-only counterparty, and candidate account must remain visible.
5. Keep asset and cash breaks separate: wealth-management and transfers are asset-conversion or continuation leads, not cash destination facts.

## Release Criteria

- `funds_investigate(intent="casegraph")` can answer scope/rank/trace/claim-support questions from the compact graph without opening low-level tools.
- A report task can explain why a claim is verified, corrected, unsupported, or forbidden without drawing unsupported flow edges.
- An investigation task can propose next queries while keeping every hypothesis in a supported, lead, missing, downgraded, or forbidden state.
- B2 must show no default discovery loop after a complete answer card.
