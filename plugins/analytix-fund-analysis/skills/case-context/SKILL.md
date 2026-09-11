---
name: case-context
description: 用于核验当前 analytix 案件项目、数据范围、清洗流水、交易环境/IP/MAC/渠道、开户信息、联系方式、住址、任务反馈、强制措施覆盖情况和案件关系上下文。适合案件来源预检、数据覆盖说明和下一步研判分流；不用于主体画像、全案报告或报告事实结论复核。
---

# 案件数据范围核验

Understand the current analytix case project, scope, coverage, and casegraph context
before deeper analysis. This is the Data Analytics-style preflight layer for
case-project fund work.

Read the shared boundary when summarizing facts or recommending a handoff:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library when case scope must decide whether to run a
dossier, full-case tree, feature scan, trace, or delivery pack:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).
Read the workflow context when case-project binding, cleaned exports, reports,
attachments, graph images, or multi-turn continuation affect the preflight:
[analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md).

## Use when

- The user asks which case project is active, what data is available, or what the
  current scope/coverage/time span looks like.
- The user asks for casegraph, scope map, source/import/clean/index coverage,
  account-opening/contact/address/transaction-environment/IP/MAC/merchant/
  channel/task-feedback/coercive-measure coverage, or preflight before deeper
  work.
- A focused owner needs shared context for the current case project, cleaned export
  surface, report/attachment directories, available visual artifact paths, or
  multi-turn continuation state.
- Another focused skill needs current-case-project context before proceeding.

## Not for

- Complete object dossiers. Use `account-dossier` or `subject-dossier`.
- Full-case analysis or report generation. Use `full-case-analysis` or
  `report-builder`.
- Claim review. Use `claim-review`.

## Workflow

1. Use `get_current_case` only when status or case-project resolution is needed.
2. Use `get_case_scope_map` for scope, data range, top entities, and available
   lanes.
3. Use `get_scope_coverage` when totals, report-grade coverage, or scope
   disagreements matter.
4. Use `get_casegraph` for compact entity/quality context.
5. Separate cleaned transaction detail, account-opening/registry facts,
   person/contact/address facts, transaction-environment/IP/MAC/teller/branch/
   location/merchant/channel coverage, task feedback and coercive-measure
   coverage, holder index, counterparty index, casegraph context, and report
   scope when those layers affect downstream analysis.
6. Treat import/cleaning information as read-only coverage/audit status; do not
   execute import, cleaning, or source mutation from this skill.
7. Treat casegraph/fundgraph as context. Do not use graph nodes as proof of
   unsupported fund flows.

Default first-turn path: use each context tool at most once:
`get_current_case`, `get_case_scope_map`, `get_scope_coverage`, and
`get_casegraph`. Do not repeat scope-map calls, do not call data-quality audit,
and do not start full-case/report/dossier work from a context-card request.
If a coverage lane is missing, state the gap and recommend the owning focused
skill instead of sweeping more tools.

## Output Contract

Return a Case Scope Card:

- current case project and case-project binding warning if any
- analysis scope, time span, available cleaned transaction, account-opening,
  contact/address, transaction-environment/IP/MAC/merchant/channel,
  task-feedback, and coercive-measure coverage
- cleaned export, report, attachment, graph/image, and analysis-index surfaces
  when the downstream task depends on them
- quality boundaries and missing fields that affect facts
- 1-3 recommended focused skill handoffs
- a plain boundary that this context card did not run full-case analysis and
  did not generate a report

## Completion Gate

The skill is complete when the user can tell what case project and scope are active and
which focused workflow should run next. It is incomplete if it asks for `case_id`
when case-project resolution is available, starts full-case analysis from status,
or outputs raw casegraph/debug payload.
