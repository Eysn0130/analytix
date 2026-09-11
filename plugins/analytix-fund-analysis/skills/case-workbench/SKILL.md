---
name: case-workbench
description: 用于当前 analytix 案件项目专项资金核算和自定义统计范围核验：可回放计算、对公转个人重算、摘要缺失纳入总额、工资劳务报销可识别范围、无法识别性质、金融产品专题、清洗明细导出和附件化分析。在语义事实不足、用户要求自定义统计范围/可回放核算、事实冲突、金额挑战或专项附件时使用只读、限行、可复核分析路径。
---

# 专项资金核算与自定义统计范围核验

Use when the current case-project question needs explicit SQL/notebook/custom
calculation, reproducible computation, conflict resolution, result challenge, or
cleaned-detail export that semantic tools cannot answer directly. The workbench
is support/evidence; the focused lead owner writes the final answer.

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md),
[tool-availability](../analytix-fund-analysis/references/tool-availability.md),
and [analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md).
Read [database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
only when a semantic gap or source conflict requires schema/profile/diagnose/explain/preview/history.

## Use when

- The user explicitly asks for SQL, notebook, 自定义统计范围, 可回放分析, or reproducible computation.
- A focused skill names a real semantic-tool gap: custom denominator, feature cross-tab,
  chart-ready extract, diagnostic aggregation, or recompute.
- The user challenges an amount, names a conflicting fact, or asks to test a
  specific investigative hypothesis against cleaned/analysis tables.
- The user asks for cleaned-detail export, appendix workbook, or chart/table rows.

## Not for

- Ordinary Top, amount, account, holder, counterparty, tracing, suspicious-feature, or full-case questions already covered by semantic tools.
- weak-source substitution, 弱来源替代, fake delivery, 假交付, unverified calculations, unbounded exports,
  `fc_*_raw`, DDL/DML, extension loading, file/network reads, permission bypass, cleaning/import repair, or cross-case joins.
- Upgrading a bounded workbench result into a full-case total, transaction path, legal conclusion, or report-grade claim without later owner validation.

## Workflow

1. Name the gap: which semantic fact was insufficient, what custom calculation is needed, and why workbench is justified.
2. Build the internal source record / `case_source_envelope`: `case_identity`,
   `source_of_truth`, `source_scope`, `data_quality_state`, `metric_scope`,
   `validation_state`, `delivery_state`, and `gap_card`.
3. Preflight only what is needed: case-project binding if unclear, `get_case_scope_map`,
   one broad-enough `inspect_case_schema`, and `audit_case_data_quality`.
4. SQL must use `table.sql_name`, `column.sql_name`, or `sql_identifier` from
   `inspect_case_schema`; Chinese `display_name` labels are UI-only. This is a
   schema-first database MCP lane, not blind DuckDB exploration. Prefer the
   returned `semantic_hints` for table roles, metric columns, and join hints
   before drafting SQL.
5. Use the database-site ladder when uncertain: `profile_case_schema` for field coverage/ranges,
   `count_case_rows` for lightweight row-count sizing, `explain_case_sql` for plan/risk,
   `diagnose_case_sql` for failed/empty/conflict queries, `preview_case_rows` only
   for filtered privacy-projected samples, recipes for reviewed templates, and
   `inspect_workbench_history` only for digest comparison.
6. For repeated patterns, start from `case_sql_recipes` templates for full-case
   overview, holder/account overview, Top counterparties, one-hop amount,
   keyword evidence, coercive measures, downstream destinations, or duplicate
   review; render parameters into complete SQL, then validate through
   schema/profile/explain/run. During P0 containment, the only executable path is
   `run_case_sql` for bounded aggregates/evidence rows. `create_case_notebook`
   and `export_cleaned_case_data` are hidden and rejected before execution;
   notebook/export requests receive a capability boundary and create no file,
   job, staging directory, or attachment.
7. Enforce current case only, read-only, cleaned `fc_*_norm` and approved
   `analysis_*` views only, no unbounded `SELECT *`, no external file/network/secret access,
   no DDL/DML, row limit at or below 500, explicit purpose, complete SQL, and no raw/debug payload.
   If backend workbench is unavailable, local DuckDB fallback may be used only
   with an already resolved case id or Analytix runtime current-case id; never scan folders or guess cases.
8. Treat every successful `run_case_sql` result as an evidence input, not a
   finished answer. Preserve `evidence_card`, `source_hash`, `metric_scope`,
   `validation_state`, and `source_refs.query_ids`; reconcile the result with
   semantic MCP facts or prior calculations before the focused owner writes a
   report-grade amount, path, chart, or table.
9. If controlled execution is unavailable, held, or returns no executed table,
   return a 专项资金核算缺口说明. Do not fabricate query results.
10. After execution, answer the bounded finding first, then state 本次依据、统计范围、
   核验意见、交付情况和暂不能认定事项 in user-facing language. Hand off to the
   lead owner for visual, critique, claim, or report use.

## Output Contract

Return why 专项资金核算 was needed, current-case cleaned/analysis 范围, row limit,
metric/time basis, execution result, 核验意见, compact result, capability gap,
evidence card / source hash / validation state, and productization target. Do
not expose weak-source labels, source tables, large JSON, raw rows, tail
instructions, or local paths. If the result becomes a report-grade table, the
next owner must add 表后研判意见：异常特征、证明价值、暂不能认定事项和下一步调取材料。

## Completion Gate

专项资金核算 is complete only when it returns an audited bounded result or a clear
controlled-tool gap plus delivery state. It fails if it skips `audit_case_data_quality`,
repeats `run_case_sql` to reread the same metric, uses workbench as the ordinary
default, hides bounded custom computation status, or drops `evidence_card`,
`source_hash`, `metric_scope`, `validation_state`, or `source_refs.query_ids`.
It does not fail for
deliberate exploration of a named hypothesis or custom metric when the result
remains current-case, read-only, bounded, and evidence-bound.
