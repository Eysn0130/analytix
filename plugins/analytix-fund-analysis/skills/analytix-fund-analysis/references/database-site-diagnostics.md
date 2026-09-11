# 数据库现场勘查规则

This reference governs controlled database-site diagnostics for the current
Analytix case. It adapts mature database MCP ideas from
`crystaldba/postgres-mcp`: schema/object inspection, restricted safe SQL,
forced read-only execution, query timeout, explain-plan review, slow-path
diagnostics, health checks, and deterministic workload/index analysis. Analytix
maps those ideas to DuckDB + cleaned tables + analysis indexes. It is a support
layer, not the final Chinese answer owner.

## Purpose

Use database-site diagnostics only when semantic facts are insufficient,
conflicting, challenged by the user, or need a replayable SQL/notebook/table
artifact. SQL, Python, notebook, and DuckDB are normal capabilities, but they
enter after source preflight and source-of-truth selection.

Ordinary case answers must not mention Postgres, MCP, Workbench, query ids,
debug payloads, SQL dumps, raw rows, or internal workflow terms. Translate the
work into公安经侦 language:

- 表结构 -> 数据表结构核验
- schema profile -> 字段覆盖、空值率、取值范围核验
- EXPLAIN -> 查询计划核验
- preview -> 隐私投影样本核验，样本不可汇总
- recipe -> 可回放专项核算模板
- query history -> 既往专项核算摘要

## Tool Ladder

1. Semantic source first: pair amount, rank/profile/trace/quality/full-case tools
   control ordinary facts.
2. Source preflight: current case project, scope map, schema inventory, data quality, and
   source-of-truth selection. `inspect_case_schema` must return complete
   cleaned/analysis metadata for the current case project, including table roles,
   SQL-safe identifiers, semantic hints, key columns, metric columns, and join
   hints. Metadata inspection is allowed to exceed the ordinary 500 data-row
   limit because it does not expose transaction rows.
3. Field profile: `profile_case_schema` when a SQL/notebook needs field coverage,
   null rate, distinct scale, direction enum, amount/date min-max, or key-field
   availability.
4. Row count: `count_case_rows` for lightweight row counting before fetching
   rows or choosing a query window. It returns only `COUNT(*)` for one allowed
   cleaned/analysis table, with an optional simple filter, and cannot support
   an amount, ranking, ownership, or path claim by itself.
5. Query plan: `explain_case_sql` before executing risky or unfamiliar custom
   SQL. It returns plan/risk only, never rows.
6. Diagnosis: `diagnose_case_sql` when SQL fails, returns empty, conflicts with
   semantic facts, references a wrong field/table, or appears too slow.
7. Sample: `preview_case_rows` only with explicit purpose, table, filter, and
   row limit <= 20. It returns privacy-projected samples. Samples cannot support
   totals, rankings, fund paths, ownership, or legal conclusions.
8. Execution: `run_case_sql` only for one bounded aggregate/evidence-table/chart
   result after the above gates when needed. A successful execution must return
   `evidence_card`, `source_hash`, `metric_scope`, `validation_state`, and
   `source_refs.query_ids`; this is the handoff object for source-of-truth
   selection, Evidence Ledger coverage, visual/report use, and claim review.
9. Replay/artifact: `create_case_notebook` for a durable, replayable analysis
   pack; use `export_cleaned_case_data` only when the user explicitly requests a
   cleaned detail export.
10. History/recipes: `inspect_workbench_history` and `case_sql_recipes` support
   conflict review and repeatable templates; neither is a case fact. Recipes
   should be concrete parameterized SQL templates over reviewed cleaned/analysis
   tables such as `analysis_account_dim`, `analysis_txn_daily_agg`,
   `analysis_txn_detail_idx`, `fc_account_norm`, `fc_coercive_measure_norm`, and
   trace/path indexes, not generic advice to "find the right table".
11. Health/slow-path check: when a semantic tool or SQL path becomes slow,
    empty, inconsistent, or cache-dependent, inspect local query log, table
    row counts, materialized index status, graph endpoint integrity, and
    involved-table plan risk before retrying. This is the DuckDB/current-case
    analogue of `crystaldba/postgres-mcp` database health, top-query, and
    index-analysis tools; it must diagnose tool/data readiness, not produce a
    new case conclusion by itself.

The database-site ladder is default-visible so the model can diagnose a
legitimate SQL/notebook gap without enabling broad development discovery. It is
still gated by purpose, current-case resolution, read-only SQL, row limits, and
allowed table scope.

When the backend SQL parser or governed workbench endpoint is unavailable, MCP
may use the Analytix-owned local DuckDB fallback for the same ladder only after
the current case has already been resolved by explicit `case_id` or Analytix
runtime environment. The fallback must not scan local case folders, infer case
ids from old outputs, read historical evidence files, or broaden the allowed
tables beyond `analysis_*` and `fc_*_norm`.

## Guardrails

The backend must enforce:

- current case only; no directory scan, no `*.duckdb` discovery, no cross-case
  joins;
- single SELECT/WITH only;
- structured DuckDB SQL parsing as the primary guardrail when the backend is
  available, with keyword scanning and strict table extraction as local fallback
  auxiliary defenses;
- no DDL/DML, transaction control, COPY/EXPORT, INSTALL/LOAD, ATTACH/DETACH,
  PRAGMA, external files, network, secrets, extension loading, or arbitrary
  database credentials;
- base tables only in cleaned `fc_*_norm` and approved `analysis_*`;
- no raw/source tables;
- no unbounded `SELECT *`; bounded preview is sample-only;
- explicit purpose, row limit, validation state, evidence/provenance, and audit
  record for every diagnostic, profile, preview, explain, recipe, history, SQL,
  and notebook result.
- successful SQL results must carry a case-workbench evidence card; a table or
  preview without source hash, metric scope, validation state, and evidence
  boundary cannot support a report-grade claim.

## Source-Of-Truth Selection

When sources conflict, do not choose the largest number, latest result, or
most convenient table. Select the controlling source by grain, scope, time
window, dedupe state, source coverage, and evidence traceability:

- semantic pair amount or dedicated focused tool controls ordinary amount facts;
- rank/profile/sample/preview can locate or explain but cannot control report
  amounts or fund paths;
- SQL aggregate can control only the exact custom scope it executed;
- notebook/report artifacts control only after their executed cells and claim
  review pass;
- unresolved conflict becomes Claim Gap with a concrete next proof action.

## 经侦 Output Requirements

For amount questions, final prose must include period, count, amount, main
accounts, large or concentrated dates, same-fact/card-change risk, visible
downstream direction, abnormal meaning, and matters that cannot yet be found.

For flow diagrams, first form the main supported chain, side leads, breakpoints,
and proof gaps. Do not draw unsupported or headless paths.

For subject/account dossiers, cover registered accounts, key accounts, inflow
and outflow scale, top counterparties, time/amount concentrations, cash,
financial-product, asset-consumption, IP/MAC/contact/address/equity leads when
available, and evidence gaps when absent.

Give continuation requests only after the current DuckDB data within the user
question has been analyzed to its boundary. Continuation must name object,
account/name, period, material, and proof purpose.
