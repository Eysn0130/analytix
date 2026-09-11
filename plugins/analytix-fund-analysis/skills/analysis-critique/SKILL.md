---
name: analysis-critique
description: 用于复核资金研判答复、计划、专项资金核算、资金流向图谱、证据表或报告是否遗漏侦查方向、核验意见薄弱、金额统计范围错误、过度下结论、表达机械或需要补证。适合用户要求“审核/复盘/是否完整/哪里错了”时使用；只给经侦复核意见，不新增案件事实、不展示内部流程。
---

# 研判完整性复核

研判完整性复核用于检查 Analytix 资金研判是否完整、是否过度下结论、是否缺少
侦查方向或材料化要素。它帮助选择下一步核验动作，不新增案件事实。

Read the shared boundary before answering:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library for completion packages, typology lenses, and
anti-patterns:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).
Read [database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
when critiquing custom SQL, notebook, report figures, field coverage, failed
queries, sample checks, or source-of-truth conflicts.

## Use when

- The user asks whether an answer, plan, report section, dossier, or lab result
  is complete, mechanical, too narrow, or missing important fund-analysis angles.
- The user asks which Analytix workflow, command, lane, or next analysis should be
  used after looking at current case/data context.
- A prior response may have skipped object dossier, full-case coverage, negative
  search, continuation, device/IP/MAC, group-association, platform, tax/contract,
  cost-normality, or evidence-boundary checks.

## Not for

- Producing new case facts from scratch.
- Data-source quality audits; use `data-quality`.
- Report conclusion 核验通过/当前证据不足 review; use `claim-review`.
- Executing full-case analysis; use `full-case-analysis`.

## Workflow

1. Identify the artifact being critiqued: answer, plan, dossier, lab result,
   full-case result, report section, graph, or next-step list.
2. Compare it against the relevant completion package and anti-patterns.
3. Mark gaps as missing lane, weak evidence, wrong route, overclaim, or safe.
4. Recommend the smallest next focused workflow or command for each important gap.
5. Do not invent missing facts; say which verification workflow should verify them.
6. If a prior answer relied on custom SQL, notebook output, sample rows, schema
   guesses, or a disputed amount, critique whether it used the right diagnostic
   gate: source preflight, schema profile, explain/diagnose, governed aggregate,
   and claim review. Missing database-site diagnostics are a material gap only
   when the semantic fact pack was insufficient, conflicting, or challenged.

## Material Quality Blocks

Mark the answer `需修订` when it is factually plausible but still not usable as
case material because it has no table, no abnormal-feature scan, no case significance,
no next investigative action, or only explains a calculation range / support card.
Pair Amount critique must look for conclusion, period,
amount/count, large-transaction table, account/time concentration, difference
business meaning, abnormality, unsupported limits, and downstream proof action.
Graph, dossier, full-case, and report critique must name the missing hero
deliverable: fund-flow figure, account/subject table, report section, appendix,
or evidence request list.
For SQL/notebook-based artifacts, mark the answer `需修订` when it exposes raw
rows, treats a sample as a total, hides the checked source boundary, skips
source-of-truth selection across conflicting values, or gives续调建议 before the
current case data in scope has been analyzed to its evidence boundary.

## Output Contract

Return:

- critique verdict: 可直接采用、需修订、需改走其他专项、或暂不形成报告级表述;
- missing or weak lanes, especially Top20 inflow/outflow, device/IP/MAC,
  group-association, platform/virtual-asset, tax/contract, cost-normality,
  negative-search, continuation, and evidence-boundary coverage;
- overclaim or mechanical-output risks;
- missing material-quality elements: table, abnormal feature, case meaning,
  unsupported limit, or next proof action;
- recommended next focused workflow, command, or verification family;
- safe corrected direction, without adding unsupported facts.

## Completion Gate

The critique is complete only when the user can see what is good enough, what is
missing, and exactly which Analytix workflow should run next.
