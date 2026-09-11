---
name: index
description: 用于 analytix 当前案件项目资金研判总入口和路由。当用户直接问案件资金、对象关联、资金去向、金融产品、对公转个人、资金流向图、报告材料或补证建议时，先把普通办案语言路由到对应经侦核验流程，不展示内部过程。
---

# Analytix Index

Navigator not commander: route current-case-project fund requests to the right focused
owner while keeping facts inside the `analytix_funds` MCP boundary.

Read the shared boundary when any case fact, handoff, or answer boundary is
needed: [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
For lane wording use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md);
for current-case sync, cleaned exports, report/attachment directories,
graph/image generation, or multi-turn continuation use
[analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md).

## Use when

- The user invokes `/analytix`, mentions Analytix fund analysis broadly, or asks
  what current-case-project workflows are available.
- The user gives a fund-analysis request but has not named a clear object,
  tracing seed, quality issue, full-case task, report, or report-conclusion review.
- The user needs help choosing the right Analytix workflow.

## Not for

- Full-case analysis, dossiers, formal reports, report-conclusion review,
  evidence requests, route critique, visual/table artifacts, SQL/notebook
  computation, or non-case tasks. Route to the focused owner below.

## Workflow

1. Resolve whether the user is asking for orientation or a specific lane.
2. Apply the Investigation Objective Gate before routing. If the user purpose
   changes the owner, scope, direction, evidence maturity threshold, or delivery
   surface, ask at most three concrete case-material questions. If the purpose
   is inferable, route directly and let the focused owner verify facts and state
   evidence boundaries.
3. If status is needed, use `get_current_case`; if scope is needed, hand off to `case-context`.
4. Route to the focused owner:

   | User intent | Focused skill |
   | --- | --- |
   | Current case, scope, coverage, casegraph | `case-context` |
   | Data-quality, duplicate, source/scope challenge | `data-quality` |
   | Top/ranking/ordinary bounded fact or count | `quick-fact` |
   | Pair Amount / amount challenge such as `A 转给 B 多少钱` | `pair-amount-investigation` lead owner; `rank_counterparties`, `funds_investigate`, `data-quality`, and `case-workbench` are support only |
   | Named card/account analysis | `account-dossier` |
   | Person/holder/company analysis | `subject-dossier` |
   | Counterparties, shared channels, missing counterparties | `counterparty-analysis` |
   | Source, destination, next hop, path | `fund-tracing` |
   | Suspicious features, account roles, negative search, hypothesis queue | `investigation-lab` |
   | Full-case analysis tree, full analysis, run all lanes | `full-case-analysis` |
   | Formal report or attachment generation | `report-builder` |
   | Supplementary evidence, subpoena, or continuation request list | `evidence-request` |
   | Critique answer completeness, route, missing lanes, or next skill | `analysis-critique` |
   | Report conclusion, amount/path/graph/legal wording review | `claim-review` |
   | Delivery quality, non-template professional judgment, engineering-word leak check | `delivery-qc` |
   | Casegraph/fundgraph/fund-flow visualization | `graph-visualization` |
   | Evidence tables, charts, dashboards, appendix workbooks, or visual/table QA | `visual-evidence` |
   | Explicit SQL/notebook/custom口径/reproducible computation gap, or cleaned-detail export | `case-workbench` |

5. Do not sweep tools from the index. Bare `/analytix` is entry/status only:
   call `get_current_case` and `get_case_scope_map` at most once each, say
   `不生成报告`, and include `全案分析` as one available next workflow.

## Output Contract

For orientation, answer with the current case-project status when available, the 2-4 most
relevant focused workflows, and a concrete next prompt. Keep it user-facing:
avoid internal ids, doctor/eval labels, blueprint text, or command-menu dumps
unless the user asks for the menu.

The index never finalizes substantive case analysis from cards, workbench
summaries, or navigator text. Focused owners write the final Chinese
investigation answer; support layers provide facts, validation, tables, graphs,
and artifact status.

## Completion Gate

The index is complete when it has either routed to a focused skill or given a
compact orientation. It is incomplete if it starts report generation, calls broad
analysis tools, or answers a dossier/full-case task itself.
