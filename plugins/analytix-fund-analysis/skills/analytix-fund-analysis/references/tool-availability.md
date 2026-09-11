# Tool Availability

Reserved production namespace: `analytix_funds`.

## Current production state

The ordinary plugin `.mcp.json` remains disabled. If its JavaScript protocol
process is started for controlled validation, `tools/list` contains exactly
the fixed `count_case_rows` and `analyze_account_flows` host-capture schemas,
both marked `taskSupport=forbidden`. That process cannot read the case source,
perform account-flow analysis, or form a provider catalog; the count protocol
can only validate and echo an exact host-supplied projection. Cached schemas,
source-only modules, local files, and the legacy inventory below are not
evidence that a source is online or that a tool is provider-visible.

Production case-fact admission instead belongs exclusively to the Go host. It
may compose the reserved, exact, installed-generation `analytix_funds` binding
only after package/source identity validates. `count_case_rows` remains an
internal plumbing canary. `analyze_account_flows` is the only provider-visible
funds tool, and only while current case, context epoch, immutable dataset
snapshot, source readiness, execution grant, and evidence authority all remain
current. Ordinary MCP configuration cannot mint, replace, or widen this
binding; revocation removes the analysis tool without a JavaScript fallback.

The fixed account-flow result carries exact inflow, outflow, count, coverage,
currentness, row lineage, and source-field support through typed evidence,
`ClaimRecord`, and the Final Evidence Gate. Signed net is derived only from the
exact gated inflow/outflow claims. Counterparty facts are omitted or reported
as gaps unless separately supported. The host-private evidence carrier is
consumed or discarded exactly once and cannot enter provider/model input,
public messages/events, generic tool results, logs, or telemetry.

Direct Source Preview is a separate local UI/data-plane use case and never
enters the Agent or MCP catalog. AcceptedSlotDisplay resolves only gated typed
slots against their originally bound immutable snapshot. Complete PII may
reach only these trusted local typed sinks; `full|masked` changes the final
local projection, not retained source, claims, receipts, or model-visible
history.

## Target availability contract

The broad inventory below is a later target, not the current provider catalog
or ordinary `tools/list` response. Its staged admission requires separately
accepted strict schemas, source capabilities, evidence/final gates and bounded
product evidence. Write/report tools additionally require their accepted
publication authority and atomic-delivery contracts. Adding a dormant module
to the package or receiving investigator approval alone does not satisfy these
conditions.

When those gates exist, the intended semantic toolbox covers current case,
scope/coverage, casegraph/fundgraph, Pair Amount, rankings, account/holder
profiles, tracing, hypothesis probes, quality review, duplicate review,
continuation validation, report-claim validation, and the `funds_investigate`
navigator. Pair Amount should precede the broad navigator so an ordinary
`A 转给 B 多少钱` question starts from the focused two-party amount source.

Low-level source detail rows, debug payloads, write/export tools, and
report-heavy workflows remain separately gated even after read-only facts are
re-admitted. Ordinary Agent use must choose the smallest sufficient semantic
fact tool.

Controlled Case Workbench tools are target conditional tools, not currently
visible production tools. If later admitted, they may be discoverable like
mature database MCP tools, yet executable only for
explicit SQL/notebook/custom口径/reproducible analysis gaps after semantic tools
prove insufficient or conflict. They must remain current-case, read-only,
cleaned/analysis-scope, row-limited, purpose-tagged, and audited. They are not a
shortcut for weak sources, source-table tables, unverified calculations, or fake
delivery.

These gates are evidence-control boundaries, not thought-control boundaries:
Codex may form hypotheses, compare possible explanations, and escalate to
workbench when the user changes scope, requests a custom口径, challenges a
result, asks for replayable SQL/notebook output, or a semantic fact conflicts
with another supported fact.

Database-site diagnostic tools are part of the same later governed workbench
lane:
`profile_case_schema`, `explain_case_sql`, `diagnose_case_sql`,
`count_case_rows`, `preview_case_rows`, `inspect_workbench_history`, and
`case_sql_recipes`. They
would be visible only after their own admission so the model could follow a
schema/profile/explain/diagnose ladder without broad full-discovery mode. They
help inspect field coverage,
query plans, failures, bounded samples, replay history, and reviewed query
templates before or after a governed calculation. Schema inventory should carry
semantic hints such as account dimension, daily aggregate, detail index,
coercive-measure table, metric columns, and join hints so the model can quickly
turn an investigation question into the correct cleaned/analysis query. Recipes
should include concrete parameterized SQL templates, not generic advice to hunt
for fields. They do not make SQL the
ordinary first step, do not expose raw rows as user facts, and do not replace
focused skill ownership of the final Chinese investigative answer.

If the backend SQL parser or governed workbench endpoint is unavailable, the MCP
runtime may use the analytix-owned local DuckDB fallback only when the current
case project is already resolved from the workspace `.analytix/case-project.json`
binding. An explicit `case_id` is accepted only when it matches that binding.
The fallback must not scan folders, guess case ids, read historical outputs, or
expand beyond `analysis_*` and `fc_*_norm` tables.

If a backend semantic tool is unavailable, it must return a structured
`SEMANTIC_TOOL_UNAVAILABLE` capability gap instead of a JSON-RPC error. The gap
must say which semantic surface failed, prohibit fabricated profiles/flows/report
claims, and point Codex to the current-case DuckDB workbench path:
`get_scope_coverage`, `inspect_case_schema`, `count_case_rows`,
`case_sql_recipes`, `run_case_sql`, and `diagnose_case_sql`. This keeps the
model able to inspect the database现场 like a restricted database MCP while
preserving current-case isolation and evidence boundaries.

Workbench results may answer the current bounded task when the audited result is
available and the output labels its scope and validation status. Reusable
custom口径 should be recorded as a productization candidate, but the plugin must
not automatically upgrade itself, modify backend/MCP schemas, or pretend the new
semantic tool already exists.

`resources/list` exposes the root skill, command metadata, command router, tool availability, runtime boundary, Hub lifecycle, anti-patterns, focused-skill shared contract, Analytix workflow context, plugin benchmark notes, casegraph roadmap, and capability registry files so models that prefer MCP resources can load the same progressive-disclosure guidance without guessing bare tool names. Golden/oracle eval rubric files are eval-only and are not exposed as production MCP resources.

`prompts/list` exposes Agent workflow prompts for schema-first current-case database exploration, query diagnosis, fund-flow graph construction, visual evidence packs, and formal public-security investigation report preparation. These prompts guide Codex into the right MCP tools and evidence boundaries; they are not answer templates, model-evaluation prompts, or case facts. `prompts/get` must return reusable workflow text without local paths, eval fixtures, golden answers, or fixed-case conclusions.

Pair Amount is both a focused-owner workflow and a first-class fact tool. For `A 转给 B 多少钱`, amount-basis disputes, duplicate/card-replacement, or same-fact amount risk, call `investigate_pair_amount` first and use `pair-amount-investigation` to compose the final answer. `rank_counterparties`, `funds_investigate`, duplicate audit, and controlled workbench results are support evidence only.

## Legacy and later target inventory (currently not advertised)

- `investigate_pair_amount`: first-class current-case two-party amount fact source. Use it first for `A 转给 B 多少钱`, amount disputes, duplicate/card-replacement, same-fact risks, or raw-vs-effective differences. For unrestricted natural questions it returns the full payer-to-receiver detail aggregate as the controlling scope, plus focused transfer concentrations as abnormal/downstream clues, account counts/numbers, row-level transaction candidates, duplicate boundaries, rank cross-check differences for audit/proof-boundary review, and next evidence actions for the focused owner. Do not turn rank cross-check differences into a competing visible amount row unless the user asks about the discrepancy.
- `funds_investigate`: natural-language shortcut/navigator and support source for fuzzy current-case-project fund questions. Do not use it as the first fact call or final answer owner for Pair Amount; it routes such requests back to `investigate_pair_amount`/`pair-amount-investigation`.
- `get_current_case`: resolve the current analytix case project. Its `case_dashboard_stats` are analytix case-dashboard metadata only; do not use them as analysis coverage.
- `get_case_status`: case metadata and skill surfaces. Its dashboard counters are metadata only; use scope-map/coverage/rank tools for report-grade counts.
- `get_import_overview`: import files and import jobs.
- `get_cleaning_overview`: cleaning jobs and impacts.
- `get_case_data_pipeline_overview`: publication-blocked import, cleaning, stats, and tree diagnostics; it does not establish case facts or report readiness.
- `get_case_scope_map`: CodeGraph-style low-context case data/scope graph. It composes schema inventory, source audit, data-quality audit, transaction-environment coverage, reconciliation, coverage, and tree/pipeline metrics into nodes, edges, mandatory gates, and allowed next tools without source detail rows, local paths, or unverified calculations.
- `get_stats_meta`: stats status and transaction date range.
- `get_stats_tree`: by-name or by-card analysis tree.
- `query_stats_rows`: counterparty ranking rows.
- `query_stats_txn_rows`: bounded transaction rows from stats scope.
- `query_account_txn_rows`: bounded rows for one account.
- `get_analysis_dashboard`: trend, structure, counterparty, cash, heatmap, flow, and anomaly panels.
- `query_txn_slice`: bounded transaction slice.
- `export_cleaned_case_data`: P0-hidden write tool. It is not advertised or
  executable; export requests return a capability boundary and create no file,
  job, staging directory, or attachment.
- `run_case_sql`: conditional Controlled Case Workbench query runner for
  explicit custom current-case-project analysis gaps. It accepts only current-case,
  read-only, cleaned `fc_*_norm` / `analysis_*` scope, bounded row limits, and a
  purpose. If unavailable, return a 专项核算缺口说明 rather than fabricating
  an unverified calculation.
- `explain_case_sql`: conditional database-site diagnostic for current-case SQL.
  It validates a SELECT/WITH query, returns involved tables, plan/risk summary,
  timing/row estimate where available, and no detail rows. Use before custom
  calculations, slow queries, or disputed metrics.
- `diagnose_case_sql`: conditional diagnostic for failed, empty, conflicting, or
  risky current-case SQL. It returns error class and corrective direction, not a
  replacement amount or final conclusion.
- `count_case_rows`: current-case row-count helper aligned to
  `crystaldba/postgres-mcp` style safe database inspection. It counts one
  allowed `analysis_*` or `fc_*_norm` table with an optional simple filter and
  returns only `COUNT(*)`; it does not expose samples or support totals,
  rankings, fund paths, or report conclusions by itself.
- `profile_case_schema`: conditional schema profiler for allowed cleaned and
  analysis tables. It returns semantic field tags, coverage/null/distinct
  metrics, amount/date ranges, and safe direction/status enums without raw rows.
- `preview_case_rows`: conditional privacy-projected sample inspection. It
  requires a table, filter, purpose, and limit <= 20; visible samples must be
  labelled as non-aggregate examples and cannot support totals.
- `inspect_workbench_history`: conditional provenance reader for recent governed
  queries, plan/profile/preview calls, and validation outcomes. It returns
  digests, purposes, involved tables, duration, row count, truncation, error
  class, and evidence references without sensitive paths or full SQL.
- `case_sql_recipes`: reviewed, parameterized query-template catalog for common
  economic-investigation calculations such as one-hop amount, subject account
  overview, Top counterparties, downstream destinations, replacement-card review,
  cash, financial products, asset consumption, and device/contact/address links.
  Recipes should return parameter names, preferred tables, and SQL templates over
  `analysis_account_dim`, `analysis_txn_daily_agg`, `analysis_txn_detail_idx`,
  `fc_account_norm`, `fc_coercive_measure_norm`, and trace/path indexes where
  applicable. Recipes are reusable analysis patterns, not fixed case answers.
- `create_case_notebook`: P0-hidden write tool. It is not advertised or
  executable; notebook requests return a capability boundary and create no
  file, job, staging directory, or attachment.
- `get_case_reconciliation`: case-level table/index/daily-aggregate reconciliation; required before report-grade full-case claims.
- `audit_case_data_quality`: import/cleaning lineage, empty-holder group, transaction-environment/IP/MAC/merchant/channel coverage, materialized-index freshness, and card-replacement/same-fact duplicate candidate audit; run before report-grade totals when cleaning, dedupe, nonstandard fields, sparse environment fields, or card replacement may affect interpretation.
- `resolve_duplicate_families`: same-fact duplicate/card-replacement candidate family resolver. Default and report-safe use is `scope_mode="same_holder_accounts"` with a holder/person or explicit account set; `full_case_review` is a risk map only and must not be used for blind dedupe or amount deduction. Scope accounts are returned as `selected_account_count` plus a bounded preview so large holder sets do not crowd out investigative reasoning.
- `inspect_case_schema`: read-only case DuckDB schema inventory through the backend, returning tables, columns, row counts, analysis-scope roles, semantic hints, key/metric columns, join hints, and SQL-safe `sql_name` / `sql_identifier` mappings without exposing source detail rows or SQL. Chinese `display_name` labels are UI-only; any `run_case_sql` must use the SQL-safe names. If `schema_status=temporarily_unavailable`, treat table/count fields as retry-boundary placeholders, not missing-table facts.
- `audit_unindexed_sources`: read-only audit for transaction-looking tables, new/nonstandard sources, or schema drift that may sit outside the normalized transaction and analysis-index scope. If `schema_status=temporarily_unavailable`, `audit_status=needs_review` means retry schema inventory; do not write core-table-missing or unindexed-source facts.
- `get_scope_coverage`: account/file/date scope coverage and field-quality metrics. When the backend coverage service is unavailable but the current case DuckDB is resolved, it may fall back to local read-only coverage over `analysis_txn_detail_idx` and core `analysis_*` / `fc_*_norm` tables, returning table counts, transaction detail row count, account/holder/counterparty counts, date range, inflow/outflow, turnover, and source-boundary warnings.
- `resolve_account_scope`: card/account/account-key resolution into backend account_keys.
- `resolve_holder_scope`: holder/name/id resolution into account sets with ambiguity warnings.
- `resolve_owner_scope`: subject/person scope resolver that separates direct registered accounts from candidate linked accounts and unresolved high-value accounts.
- `compare_analysis_scopes`: compares detail-index, directional-row, by-name tree, and report-dedupe candidate scopes before report-grade totals.
- `rank_accounts`: deterministic full-case/holder/account-scope account ranking by inflow, outflow, turnover, transaction count, or max single amount.
- `rank_holders`: deterministic full-case/holder/account-scope holder account-set ranking.
- `rank_counterparties`: deterministic full-case/holder/account-scope counterparty ranking.
- `hypothesis_probe`: deterministic discovery probe for Agent-proposed hypotheses, keywords, holder/account scopes, follow-up candidates, device/IP/MAC/channel/group-association/tax-contract leads when supported by cleaned fields, and `probe_type="investigative_patterns"` open-ended irregularity mining.
- `run_investigation_lab`: bounded hypothesis lab for explicit exploratory requests. It is not part of the ordinary default toolbox and is not a substitute for the P0-hidden full-case/report entry. Use `hypothesis_probe` first for ordinary hypotheses, then at most one targeted rank/trace/quality follow-up. Large scopes and returned cards remain candidate leads and evidence gaps, never whole-case conclusions or host-verified claims.
- `run_discovery_scan`: MCP alias of `hypothesis_probe` for broad discovery.
- `trace_holder_destinations`: MCP alias of `hypothesis_probe` for source/destination and follow-up-target aggregation.
- `detect_cash_breakpoints`: MCP alias of `hypothesis_probe` for cash, missing-counterparty, ATM/POS/counter, and cash-breakpoint leads.
- `detect_financial_product_flows`: MCP alias of `hypothesis_probe` for wealth-management, fund, securities, insurance, and subscription/redemption leads.
- `detect_project_litigation_asset_leads`: MCP alias of `hypothesis_probe` for project/business, litigation/enforcement, and asset-consumption leads.
- `generate_followup_investigation_list`: MCP alias of `hypothesis_probe` for ranked supplementary evidence targets.
- `trace_subject_top_outflows`: subject/account-scope Top outflow tracing, bounded in-case next-hop candidates, terminal classification, and follow-up request list.
- `classify_missing_counterparty_business`: missing holder/counterparty business classification with financial-product-over-cash correction rules.
- `validate_continuation_list`: appendix/follow-up list quality gate for blank fields, duplicate txn ids, yuan/wan unit closure, and detail-index consistency.
- `analyze_account_full`: backend-composed full account analysis with coverage, stats, counterparties, behavior profile, and rule hits.
- `analyze_holder_full`: backend-composed full holder account-set analysis.
- `validate_report_claims`: report claim/fact-ref validation gate.
- `get_account_stats`: account-level statistics.
- `get_rule_hits`: abnormal rule hits.
- `trace_fund_next_hop`: next-hop tracing; `amount_tolerance` is a relative ratio in `[0,1]`, not a yuan amount.
- `trace_fund`: deterministic fund tracing; `tolerance_amount` is a relative ratio in `[0,1]`, not a yuan amount.
- `get_evidence_pack`: bounded evidence package for visual tables, appendix
  inventory, report support, and narrowed account/lead review; rows support
  evidence status and do not create legal conclusions. Pass only case, account,
  entity, or transaction ids; do not invent `budget`/`include_*` parameters.
- `scan_case_risks`: case-level structured leads.
- `plan_case_analysis`: conditional sub-agent lane, tool budget, shared context, and output-schema plan for complex analysis or formal reports. Large holder/account scopes are context-compact (`account_key_count`, bounded previews, truncated flags); use holder/id scope or the original explicit account list for follow-up tools, not the preview alone. In deferred Codex tool discovery, call it as `mcp__analytix_funds__plan_case_analysis`; an `unsupported call` on the bare name means the namespace was omitted.
- `run_full_case_analysis`: P0-quarantined full-case/report entry. It is not
  advertised or executable for either `write_report` value until the host
  EvidenceReceipt/Claim/PublicationReceipt pipeline is authoritative. Do not
  simulate it or replace it with a sweep of weak tools. For a full-case/report
  request, return the capability/evidence boundary, checked scope, missing lanes,
  and proof actions; no full-case fact or report file may be produced.

## Focused Owner Surfaces

- `pair-amount-investigation`: lead owner for two-party amount reconciliation.
  It must separate raw detail, effective/dedup, duplicate/evidence-insufficient,
  optional principal/fee, account/time concentration, difference explanation,
  abnormal features, case significance, unsupported limits, and next evidence.
- `delivery-qc`: final delivery gate for substantive case answers, reports,
  visuals, and appendices. It checks the investigative spine and blocks
  template-like or engineering-word-leaking output.

## Missing Or Limited Capabilities

If the backend does not provide a specialized detector, do not fake it in the plugin. Downgrade to a candidate lead and explain the missing tool or evidence.

Examples:

- account role classifier unavailable -> use account stats/dashboard/risk scan and mark as candidate.
- exact cash bridge detector unavailable -> use `detect_cash_breakpoints` / `hypothesis_probe` as a candidate lead; for Top outflow continuation use `trace_subject_top_outflows`, then ask for supplementary evidence when terminals are unmatched.
- open-ended suspicious-feature discovery -> use `run_investigation_lab` first, cover `mandatory_review_cards`, then continue only from the highest-value hypothesis cards.
- import/cleaning/dedup/card-replacement question -> use `audit_unindexed_sources`, `audit_case_data_quality`, and `resolve_duplicate_families` before rankings or narrative conclusions.
- same amount/time/balance/counterparty across multiple cards/accounts -> use `resolve_duplicate_families(scope_mode="same_holder_accounts")`; never dedupe full-case flows without same-holder/person scope or explicit investigator-confirmed account set.
- new table header / nonstandard DuckDB / possible missing imported source -> use `get_case_scope_map` first for the compact口径图谱, then drill into `inspect_case_schema` / `audit_unindexed_sources`; if `schema_status=temporarily_unavailable`, retry instead of treating zero counts as facts; if `audit_status=needs_review`, do not use "全量覆盖" wording.
- repeated txn id / missing-counterparty / amount-concentration irregularity question -> use `hypothesis_probe(probe_type="investigative_patterns", keywords=[...])` before narrative conclusions.
- IP/MAC/device/teller/branch/merchant overlap, group association, platform/virtual-asset, or tax-contract question -> use `hypothesis_probe` or a currently advertised bounded profile tool as a lead generator; do not infer operator, control, gang, platform ownership, crypto transfer, or tax offense without external evidence.
- commingled fund allocator unavailable -> avoid exact identity wording.
- report validator unavailable -> return the report capability/evidence boundary; do not manually promote tool-returned totals into report facts.
- semantic tools do not cover an explicit custom口径 / reproducible computation
  -> first try an alternate semantic path; if still insufficient, use
  `case-workbench` with bounded `run_case_sql`. Notebook requests are P0-blocked;
  return the capability gap and recommended semantic-tool productization target.
- required source lane unavailable -> stop report-grade conclusions and name the
  missing lane; optional enrichment unavailable -> continue with the strongest
  available facts and label the gap.

## Source And Delivery Blockers

- current analytix case project cannot be resolved through
  `current conversation workspace -> .analytix/case-project.json -> analytix
  data-analysis case -> analytix_funds MCP`; return a Case Source Blocker with recovery
  actions only. Do not list local directories, search `*.duckdb`, scan case
  folders, infer `case_id` from history/fixtures/cache, or retry semantic tools
  with guessed ids. An explicit invalid `case_id` is final for that turn.
- required source unavailable but report-grade conclusion is requested;
- weak source, semantic map, raw preview, or unverified calculation used as an
  equivalent source of truth;
- unbounded transaction export into the model;
- `fc_*_raw` / source-file reads in ordinary fund-analysis answers;
- plugin-side all-pairs cash bridge matcher;
- plugin-side commingled fund allocator;
- legal/tax conclusion engine.
