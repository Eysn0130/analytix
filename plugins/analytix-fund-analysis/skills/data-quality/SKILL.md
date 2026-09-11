---
name: data-quality
description: 用于只读复核案件导入、清洗、索引、开户、联系方式、住址、交易环境/IP/MAC/渠道、任务反馈、强制措施等数据覆盖情况，以及重复交易、换卡补卡、户名/对手方/环境字段缺失、金额统计范围争议和数据质量边界。不能执行导入清洗、撰写报告或替代主体画像。
---

# 数据质量与口径复核

Assess whether current-case-project data, scope, duplicate handling, and amount
statistics are trustworthy enough for analysis, dossiers, full-case work, reports, or
report conclusion review.

Read the shared boundary before finalizing:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library for amount challenge, negative-search boundary,
and delivery impact wording:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).
Read the database-site diagnostic boundary when a quality dispute needs field
coverage, empty-result, schema-profile, or governed SQL verification:
[database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md).

## Use when

- The user asks about import/cleaning/account-opening coverage, missing
  holder/counterparty/contact/address/transaction-environment/IP/MAC/merchant/
  channel fields, task feedback, coercive measures, duplicates, card
  replacement, same-fact families, amount discrepancies, or report-grade
  reliability.
- The user challenges a number, scope, time window, direction, dedupe scope,
  success filter, cleaned/index/report layer, or full-case versus narrow total.
- A dossier, full-case, report, or report-conclusion review task needs a quality gate before
  using totals or writing conclusions.

## Not for

- Writing reports. Use `report-builder` after quality facts are known.
- Broad object dossiers. Use this only for the quality portion, then return to
  the dossier owner.
- General suspicious-pattern exploration. Use `investigation-lab`.
- Executing import, cleaning, rule repair, table mutation, or source-file
  ingestion. Hand those back to Analytix main modules.

## Workflow

1. Frame the quality question: source/import/clean/index coverage,
   account-opening/contact/address/transaction-environment/IP/MAC/merchant/
   channel/task-feedback/coercive-measure coverage, duplicate/card replacement,
   missing fields, scope comparison, or amount/statistical-scope challenge.
2. Use `audit_case_data_quality` for import, cleaning, missing field, source,
   and quality coverage.
3. Use `resolve_duplicate_families` for same-fact, replacement-card, or duplicate
   transaction concerns.
4. Use `compare_analysis_scopes` when the disagreement is full-case totals versus
   a narrower scope.
5. When semantic quality tools do not explain a field-coverage gap, empty
   result, performance risk, or amount challenge, use database-site diagnostics
   in this order: `profile_case_schema` for coverage and semantic tags,
   `diagnose_case_sql` for failed or empty checks, `explain_case_sql` before any
   custom aggregate SQL, and `preview_case_rows` only to inspect a bounded sample.
   Samples cannot change totals or support report-level amounts.
6. State whether each issue changes ordinary analysis, account/subject dossiers,
   account role classification, full-case facts, report wording, or attachment
   rows.
7. For amount challenges, show prior statistical scope, corrected statistical
   scope, 已有数据支持的 amount/count, evidence-insufficient portion, and
   corrected wording.

For ordinary user-facing QA text, use `统计范围`, `金额核验情况`, `来源边界`,
and `核验意见`. Do not use `金额口径`, `口径提示`, `口径差异说明`, or
`按...口径` as headings or progress wording.

## Output Contract

Return a QA Review:

- quality question and affected scope
- 已有数据支持的 facts and lead-only issues
- audited data layer: import/cleaned, analysis index, or report scope, plus
  account-opening/registry/contact/address/transaction-environment/IP/MAC/
  channel/task-feedback/coercive-measure coverage when relevant
- amounts/counts that cannot be deducted or upgraded
- report/dossier impact
- smallest next verification or repair action

## Completion Gate

The review is complete only when the user can see what is supported by data,
what remains 线索候选/需补证, and whether the issue blocks report-grade wording. It
fails if it silently adjusts totals, treats duplicate leads as confirmed, or
replaces quality evidence with a generic warning.
