---
name: quick-fact
description: 用于当前案件的轻量事实快查：姓名/主体是否和案件资金有关联、严格命中/模糊命中/排除对象/不能确认、Top 排名、最大账户/主体/对手方和低风险单点金额/笔数事实。特定双方资金往来核验和金额争议必须交给 pair-amount-investigation。
---

# 资金事实快查

This workflow answers ordinary bounded current-case-project facts with the smallest
sufficient semantic fact tool. It does not own Pair Amount; amount challenges
route to `pair-amount-investigation`.

Desktop first-response rule: do not name skills, implementation files, command-line work, or local paths.
If progress text is required, write exactly `正在核验当前案件事实。`.

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
only after handing off to a heavier owner; ordinary quick facts should not start
with schema/profile, SQL, notebook, or sample-row inspection.

## Use when

- The user asks for a Top/ranking fact, largest account, largest holder, largest
  counterparty, total amount, count, or bounded current-case fact.
- The answer can be resolved without building a complete account/subject
  dossier, full-case analysis tree, graph, or report.
- The user is checking a low-risk single number or count inside the current
  case project, without two-party amount reconciliation, duplicate/key risk,
  or competing口径.
- The user asks whether one or more names/subjects are connected to case funds,
  especially wording like `A、B 是否和案件资金有关联`. This is a 30-second
  姓名/主体关联快查, not a full investigation.

## Not for

- Card/person/company dossiers, full-case analysis, suspicious-feature scans,
  report materialization, or open-ended "keep digging" tasks.
- Pair Amount or amount-challenge questions such as `A 转给 B 多少钱`,
  `金额范围对吗`, duplicate/card-replacement disputes, or same-fact risk. Use
  `pair-amount-investigation`.
- Field-coverage disputes, sample inspection, custom SQL, or reproducible
  calculations. Hand off to `case-workbench`, `data-quality`, or `claim-review`.

## Workflow

1. Restate the bounded metric internally: object, direction, rank/order,
   amount/count basis, time window, and success-transaction scope. If the user
   asks a two-party amount, duplicate/key dispute, or competing统计范围, stop this
   lane and hand off to `pair-amount-investigation`.
2. For ordinary ranking questions, use exactly one of `rank_accounts`, `rank_holders`, or `rank_counterparties`.
3. Use `success_filter="all"` unless the user explicitly asks for successful
   transactions only.
4. For 姓名/主体关联快查, answer through `funds_investigate` with the original
   question and focus names. Group the visible answer as 严格命中, 模糊命中,
   排除对象, and 不能确认. Strict hit means exact equality in current-case
   transaction name fields; near-name rows must not be merged into the target.
   If strict hits are zero, say 0 explicitly and place near names under 排除对象
   or 不能确认.
5. For Pair Amount / 特定双方资金往来核验 / 金额争议核验, quick-fact may provide only
   locator support. It must not write the final amount conclusion, turn support
   facts into a reusable answer paragraph, or treat `rank_counterparties` rows as final.
   Hand off to `pair-amount-investigation`.
6. Do not add casegraph, coverage, audit, profile, or navigator tools after an
   ordinary ranking result unless a required fact is missing. If the wording asks
   for a dossier or full-case review, hand off to the matching focused skill.
7. If the bounded fact becomes a source conflict or custom calculation, stop the
   quick-fact lane and hand off. Do not use a preview sample or rank row as the
   final basis for a money-flow conclusion.

## Output Contract

Return a compact answer: direct answer first; no internal workflow/tool/command
names, local paths, `SKILL.md`, `MCP`, `Shell`, or `JSON/debug`; for
姓名/主体关联快查, separate 严格命中、模糊命中、排除对象、不能确认; include metric
definition, direction, scope, time window, amount/count, Top rows when relevant,
coverage or missing-field impact, 本次依据, 核验意见, 支持程度, and a clear
handoff when Pair Amount or amount challenges must go to `pair-amount-investigation`.

## Completion Gate

Ordinary low-risk ranking turns are complete when the bounded ranking question is answered.
Pair Amount turns are not complete in quick-fact. Do not attach JSON/debug dumps,
tool explanations, internal routing, report gates, or unrelated "next step" blocks.
A new object, scope, time window, hypothesis, or continuation may route to any matching focused skill.
