# Command Router

Use this file only when the root skill cannot route the user's intent with the compact lane rules, or when the user explicitly asks for a `/analytix ...` command menu.

## Stable Namespace

Treat `/analytix` as the stable command namespace. Bare `/analytix` is an entry/status command: resolve the current case and summarize the available investigative lanes without writing a report. Route “全案分析” or “完整研判” to `full-case-analysis` by default; route tables, charts, dashboards, evidence packs, and appendix inventories to `visual-evidence` unless the user explicitly asks to write formal narrative material. Enter the report workflow only when the user explicitly asks for “生成报告”, `/analytix report`, report writing/update, or formal report material.

## Command Table

| Command | Use For | Primary MCP Flow |
| --- | --- | --- |
| `/analytix ask` | Fuzzy natural-language shortcut when the best semantic tool is unclear | `funds_investigate(intent="auto")` as navigator; use its compact support facts only to select one lead owner or a targeted semantic tool; do not copy card titles or drafts into the final answer |
| `/analytix casegraph` | Analytix casegraph v1: compact scope, quality gates, top entities, duplicate/data-quality boundaries | `get_current_case` -> `get_case_scope_map` / `get_casegraph`; do not scan every specialist tool |
| `/analytix flowgraph` | 资金穿透图和可核验交易边 | `build_fund_flow_graph(holder_name=..., via_holder_name=...)`; 图中只画端点完整、金额时间清楚的交易边；同名待核、端点缺失、旁路线索和需补证记录进入待核事项或补证事项，不以箭头表示 |
| `/analytix visual` | Evidence-bound tables, charts, dashboard cards, appendix/workbook inventory, and visual/table QA | `get_case_scope_map` once -> `get_evidence_pack` -> at most one targeted ranking/trace/list tool; do not call `run_full_case_analysis` or `hypothesis_probe` just to assemble a delivery pack; state recommended visual forms, evidence boundaries, evidence gaps, and next owner skill; rankings, matrices, heatmaps, dashboards, appendices, and workbooks are not transaction paths, control relations, or legal conclusions |
| `/analytix sql` | Explicit custom口径 / bounded SQL analysis when semantic tools cannot answer the current-case-project question | route to `case-workbench`; preflight `get_case_scope_map` -> `inspect_case_schema` -> quality boundary when needed -> `run_case_sql`; SQL uses only schema `sql_name` / `sql_identifier`, never Chinese `display_name`; current case only, read-only, cleaned `fc_*_norm` / `analysis_*` only, row-limited, purpose-tagged, no `fc_*_raw`, no DDL/DML, evidence boundary required; if unavailable, return a 专项资金核算能力缺口说明 |
| `/analytix notebook` | Replayable notebook request boundary | P0 containment: `create_case_notebook` is hidden and rejected before execution; return a 专项资金核算能力缺口说明 and create no file, job, staging directory, or attachment |
| `/analytix export` | Cleaned-detail/export request boundary | P0 containment: `export_cleaned_case_data` is hidden and rejected before execution; return an export capability gap and create no file, job, staging directory, or attachment |
| `/analytix` | Entry/status command | `get_current_case` -> compact lane summary; do not run full-case/report gate |
| `/analytix report` | Report request boundary; formal preparation/publication remains host-controlled | during P0 containment do not call or simulate `run_full_case_analysis` for either `write_report` value; return the unavailable capability, checked scope, missing evidence, and proof actions; create no draft or file until the host evidence/claim gate issues a valid PublicationReceipt and atomically publishes |
| `/analytix scope` | Data import, cleaning, usable scope | `get_case_scope_map` -> `get_scope_coverage`; use returned quality boundaries before report-grade totals |
| `/analytix map` | CodeGraph-style data/scope graph before free investigation | `get_case_scope_map` / `get_casegraph`; return compact nodes/gates/boundaries, not raw tool sweeps |
| `/analytix audit` | 导入清洗、非标 DuckDB 表、新表头、账户维表未登记户名、明细空户名字段、换卡/补卡/同事实重复体检 | `audit_case_data_quality` -> `resolve_duplicate_families` when duplicate/card-replacement risk is in scope |
| `/analytix duplicates` | 重复数据、换卡/补卡、同账号不同卡、同事实跨账户候选族 | `resolve_duplicate_families(scope_mode="same_holder_accounts" or explicit account set)`; full-case blind dedupe must not deduct totals |
| `/analytix compare-scopes` | 全案行数、金额、户名树、报告去重范围复核 | `get_case_scope_map` -> `get_scope_coverage` -> `compare_analysis_scopes`; report-grade disagreements require explicit validation |
| `/analytix rank` | 全案/户名/账户 Top 排名 | choose exactly one of `rank_accounts` / `rank_holders` / `rank_counterparties` by requested dimension; 未指定案件编号时使用当前案件; default `metric="turnover"`、`success_filter="all"`、`limit=3..10`; answer from returned rows/scope/warnings and stop |
| `/analytix amount` | 特定双方资金往来核验、金额争议、重复/换卡/同事实金额风险 | route to `pair-amount-investigation`; use `rank_counterparties` only as locator evidence, `funds_investigate` only as support/navigator, and `run_case_sql` only for one bounded cleaned/analysis aggregate when semantic facts are insufficient; final answer must include原明细/高置信去重/重复或证据不足金额、差异原因、异常特征、不能认定事项和下一步取证 |
| `/analytix account <account>` | Account statistics and features | `get_current_case` -> `get_case_reconciliation` -> `resolve_account_scope` -> `get_scope_coverage` -> `analyze_account_full` -> bounded evidence rows -> `get_evidence_pack` |
| `/analytix owner <name>` | 人员/户名登记账户和待补证账户线索 | `analyze_holder_full(holder_name=...)`; 区分登记账户和待补证账户线索，不得把待核线索写成已确认归属 |
| `/analytix holder <name>` | Holder/person dossier | `analyze_holder_full(holder_name=...)` -> targeted `rank_accounts` / `hypothesis_probe` only when the user asks for Top accounts or clues |
| `/analytix discovery` | 开放式发现现金、理财、工程、涉诉、资产、三方支付、IP/MAC、商户平台、票税合同等线索 | `get_current_case` -> optional `resolve_owner_scope` -> `run_discovery_scan` / `hypothesis_probe` |
| `/analytix lab` | 开放式深挖入口：先形成证据约束的假设卡，再挑高价值线索继续查证 | `hypothesis_probe(probe_type="investigative_patterns", question=..., holder_name=..., keywords=[...])`; 区分已有数据支持、需补证和不能认定为事实的事项 |
| `/analytix destination` | 某卡/某人资金去向和补调对象 | `trace_subject_top_outflows` for subject Top outflows; `trace_fund_next_hop` / `trace_fund` / `build_fund_flow_graph` for one-hop continuation or seed tracing |
| `/analytix outflows` | 某人/某户名/某账户集合 Top 出账、穿透到无下游、补调清单 | `trace_subject_top_outflows` -> `validate_continuation_list` only for appendix/follow-up lists |
| `/analytix probe` | Agent 自定义侦查假设校验 | scope -> `hypothesis_probe(probe_type/keywords/hypothesis)` |
| `/analytix pattern` | 不规律模式深挖：重复交易号、缺失对手、金额集中、人名/理财关键词链路 | scope -> `hypothesis_probe(probe_type="investigative_patterns", keywords=[person/channel/business words])` -> targeted trace/classify/QA |
| `/analytix cash` | 现金存取、户名缺失、ATM/柜面 | scope -> `detect_cash_breakpoints` -> `classify_missing_counterparty_business` when missing-holder/cash/product ambiguity exists -> bounded rows -> evidence pack |
| `/analytix financial` | 理财、基金、证券、保险类资金线索 | scope -> `detect_financial_product_flows` -> bounded evidence rows |
| `/analytix topic` | 工程、涉诉、资产消费等专题线索 | scope -> `detect_project_litigation_asset_leads` or `hypothesis_probe(keywords=...)` |
| `/analytix cash-bridge` | 同日/次日取现-存现对应特征 | scope -> bounded cash rows -> 待核线索表 -> evidence pack |
| `/analytix payment` | 支付宝、财付通、微信、银联、网联 | scope -> rule hits/keyword slices -> evidence pack |
| `/analytix crypto` | 虚拟资产购置或出入金线索 | scope -> strict keyword/rule hits -> evidence pack; no strong conclusion without strong evidence |
| `/analytix platform` | 支付机构、商户号、商户名、钱包、OTC、交易所或平台型对手方线索 | scope -> merchant/channel/platform slices -> evidence pack; no platform-control or virtual-asset conclusion without platform/KYC/order/wallet evidence |
| `/analytix device` | IP、MAC、柜员号、网点、地点、终端、凭证等交易环境重合 | scope -> transaction-environment coverage -> overlap lead cards; shared environment is not actual operator/control proof |
| `/analytix group` | 团伙/共同控制/共同通道关联线索 | casegraph + account/contact/device/channel overlaps -> Group Association Lead Card; do not state gang/co-offending without external proof |
| `/analytix tax` | 票税、发票、合同、物流/服务、项目成本与回流线索 | scope -> keyword/rule hits -> cost-normality and document-gap review; no false-invoice/tax conclusion without documents and ledger proof |
| `/analytix evidence` | 补调清单、续调清单、取证清单、证据包和下一步核实事项 | lead/gap -> target institution/person/system -> request item -> expected proof value -> boundary; requested record is not yet proven |
| `/analytix critique` | 复核当前研判/计划/回答是否漏掉经侦分析角度、是否路由错误或证据边界过弱 | artifact/answer -> completion-package comparison -> missing lanes and next focused skill; critique is not new fact generation |
| `/analytix path <seed>` | 资金链路、下一跳、回流、闭环 | scope -> `trace_fund_next_hop` or `trace_fund` -> 资金流向图 + 依据表; tracing tolerance is a ratio (`0.03` means 3%), not a yuan amount |
| `/analytix role` | 账户角色分类 | scope -> account stats/dashboard/risk scan -> 账户角色线索卡 |
| `/analytix asset` | 购车、购房、理财、保险、借贷、物业停车 | scope -> rule hits/slices -> evidence pack |
| `/analytix control` | 代持、实际控制、共用环境 | scope -> stats/dashboard/slices -> overlap clue cards |
| `/analytix plan` | 复杂度、分工需要和执行路径 | `get_current_case` -> `plan_case_analysis` |
| `/analytix qa` | 数字、口径、措辞、报告/附件缺口复核 | re-check totals, units, dates, deterministic fact origin, unsupported conclusions; for attachments call `validate_continuation_list` |
| `/analytix review` | 既有报告研判结论、金额、路径、图示和法律敏感表述复核 | route to `claim-review`; use `validate_report_claims` as support first, then write a review conclusion and per-conclusion table; stop when support has corrections/boundaries or `max_additional_tools=0`; do not run schema/SQL/rank/lab/full-case unless a named conclusion still lacks required data |
| `/analytix qc` | 研判材料交付复核：事实正确但缺少结论、依据、异常、案件意义、不能认定事项、补证动作或泄露工程词 | route to `delivery-qc`; for report claims use `validate_report_claims` as evidence support, otherwise review the drafted output against the investigative spine; do not generate new case facts |

## Routing Fallbacks

- If the user does not use the exact command but clearly asks the same thing in Chinese, route by intent.
- When one phrase maps to multiple skills, use this primary-owner order:
  `evidence-request` owns 补调/续调/取证清单; `claim-review` only reviews claims,
  attachments, Mermaid, and sensitive wording; `graph-visualization` owns graph
  presentation; `fund-tracing` owns path proof and continuation; `case-context`
  owns casegraph scope; `visual-evidence` owns tables, charts, dashboards, and
  appendix/workbook packaging from already-reviewed facts; `case-workbench`
  owns explicit SQL/notebook/custom口径/reproducible computation gaps after
  semantic tools are insufficient.
- Do not treat an evidence pack, Top20 table, feature table, visual dashboard,
  or appendix inventory as full-case analysis. During P0 containment, a
  case-wide analysis tree or report request receives a capability/evidence
  boundary; `run_full_case_analysis` is not advertised or executable.
- If several commands could match and the uncertainty is about data/source/scope, call `get_case_scope_map` first; otherwise call `plan_case_analysis` and follow its lane/tool budget.
- If a simple account/holder answer is enough, do not run the full report workflow.
- If semantic tools cannot satisfy an explicit custom computation, route to
  `case-workbench` before declaring the whole task impossible. Do not use local
  weak sources, source detail rows, unverified calculations, or fake delivery.
- If the task is open-ended investigative exploration, start with `hypothesis_probe` or a targeted trace/rank tool. `answer_card_complete` and `max_additional_tools` are same-turn budget hints only; they must not block a new continuation, rescope/recompute, negative search, or hypothesis probe.
