# Report Schema

Use this reference for `/analytix report`, formal case-material drafting,
report review, and full-case result materialization. It encodes public-security
economic-investigation report shapes, not a generic analytics report.
Use [public-security-official-writing](public-security-official-writing.md) for
公文式篇章、证据成熟度、法律敏感降级 and table-after-analysis requirements.
Use [investigation-answer-contract](investigation-answer-contract.md) as the
internal schema-first contract before drafting or revising final report text:
the report may be prose, but its findings must be traceable to conclusion,
verified facts, supported fund-flow paths, abnormal patterns, evidence
boundaries, and next proof actions. For report-grade wording, pass that
internal object to `validate_report_claims` as `final_investigation_answer` so
missing source refs, unsupported path edges, legal upgrades, and absent proof
actions become deterministic review failures rather than prose-only reminders.

Standard case-material headings include 基本情况、资金流入流出、重点对手方、
异常特征、资金去向、核验意见、补证建议. Adapt the order to the selected
report mode, but do not omit the investigative meaning and proof boundary.

## Scenario Activation Gate

A report mode selects a document shape; it does not establish a case scenario.
Before outlining, derive the active scenario sections only from the host-authoritative
EvidenceReceipt registry. A section is active only when every factual proposition
planned for that section is supported by registry receipts that:

- belong to the same thread, turn, case binding, context epoch, and dataset snapshot;
- passed host integrity and membership verification after a successful source call;
- declare a source capability that supports the exact claim type and fields; and
- cover the stated entities, accounts, dates, direction, amount, pagination, and
  completeness needed by the wording.

A user prompt, case theory, report mode, model/MCP assertion, candidate receipt,
non-empty evidence id, old report, cached result, or transaction source alone cannot
activate a scenario outside its source capability. One receipt that activates a
section does not bulk-authorize the other sentences or cells in that section.

Apply these minimum boundaries before per-claim validation:

| Scenario section | Minimum source-capability receipts | P0 boundary |
| --- | --- | --- |
| Engineering/project identity | Authoritative project, contract, procurement, or payment record identifying the exact project and parties. | Transaction descriptions alone do not establish that a payment is project money. |
| Project-fund flow | The project/payment receipt above plus field-matched transaction receipts for each rendered flow. | Omit the section when either side of the binding is absent. |
| Benefit-transfer lead | Field-matched transaction receipts plus independent corporate, relationship, asset, contract, or other source-capable receipts required by the proposed lead. | Do not infer the lead from cost labels, public-to-private shape, amount similarity, or a user theory. |
| Collusive-bidding or bid-rigging lead | Authoritative bid records and the independent relationship, device/edit-metadata, communication, or transaction receipts required by the exact claim. | Legal-characterization wording also requires the configured evidence combination and a current human-review receipt. |
| Bribery/corruption lead | Field-matched transaction receipts plus authoritative relationship, communication, duty, contract, or case-material receipts required by the exact claim. | Legal-characterization wording also requires the configured evidence combination and a current human-review receipt. |

If a scenario is not active, omit its heading, narrative, table, chart, and placeholder
entirely. Do not add a guessed story, a `待核` scenario paragraph, a zero-filled
template, or a list of every possible economic-crime typology. Put genuine missing
evidence in the general evidence-boundary/next-proof section. Name a scenario there
only when the user explicitly requested that scenario, and describe only the source
material to obtain, never that the scenario exists in the current case.

An ordinary full-case report is therefore not a typology checklist. It contains the
verified core sections and only those thematic sections activated by the same-case
receipt set. While the host PublicationReceipt and human-review rules remain
unavailable, legal-characterization sections remain omitted rather than drafted as
provisional stories.

## Report Mode Selection

Choose one primary mode before drafting:

| Mode | Use when | Required owner |
| --- | --- | --- |
| Full-case briefing material | 全案分析、经侦资金分析研判汇报材料、按人/账户/公司展开；专题只按 Scenario Activation Gate 纳入。 | `report-builder` after `full-case-analysis` |
| One-person / one-company dossier | 一人一档、一企一档、某人/某公司专题研判。 | `subject-dossier` or `account-dossier` -> `report-builder` |
| Project / corruption / bid-topic report | 工程项目、串通招投标、职务犯罪、利益输送、项目款穿透。 | `investigation-lab` / `fund-tracing` -> `report-builder` |
| Fund-destination and continuation list | 资金去向追踪、续调清单、补调账户清单、下一步工作计划。 | `fund-tracing` -> `evidence-request` -> `report-builder` |
| Single-lead verification memo | 某笔资金来源、某条链路、某个疑点、金额统计范围复核。 | `pair-amount-investigation` / `fund-tracing` / `data-quality` / `claim-review` |

Do not use one universal outline for all reports. A full-case report needs
coverage and subject inventory. A project-topic report needs case theory,
person/company relationships, project funds, benefit-transfer paths, cost
normality, and asset endpoints. A continuation list needs trace stops and
supplementary evidence targets.

## Full-Case Briefing Material

Use for broad public-security/economic-investigation briefing materials.

Recommended structure:

1. 前言
   - state data volume, time span, involved persons/companies/accounts, and
     analysis direction in plain case-material language;
   - summarize the main discovered issues, not the tool process.
2. 报告依据和分析范围
   - data source category, cleaned analysis scope, time span, included account
     and subject scope, and material limitations.
3. 总体资金情况
   - transaction volume, inflow/outflow amounts and counts, account/holder scale,
     account balance/retention if available, Top holder/account/counterparty
     rankings, and overall fund characteristics.
4. 重点对象资金分析
   - for each person: identity/account facts, account set, inflow Top20,
     outflow Top20, account roles, asset/financial-product/litigation/enforcement
     clues, suspicious transfer or concealment clues, evidence gaps, and next
     proof.
5. 对公账户资金研判（conditional）
   - include only when same-case corporate/account and transaction receipts support
     the rendered company/account facts; do not assign a project role, related-party
     role, or fund purpose by template.
6. 已激活的专题研判（conditional）
   - include only thematic findings activated by the Scenario Activation Gate;
     do not enumerate unsupported typologies, and omit this heading when none is active.
7. 资金链路
   - 可证实资金链路表 and readable 资金流向图. Split diagrams by one
     story per chart.
8. 综合研判意见
   - facts backed by evidence, risk leads, evidence boundary, and what remains
     unverified.
9. 下一步工作建议
   - bank/payment/platform/account/asset/tax/contract/device evidence requests.
10. 复核说明与附件目录
   - amount review 核验状态, report-conclusion QA 核验状态, attachment list, and omitted or
     unavailable evidence.

## One-Person / One-Company Dossier

Use when a report section or standalone material is organized by subject.

Required per-subject sequence:

1. 基本情况
   - identity or company registry facts, direct accounts, 线索候选 accounts,
     bank/account type/状态, contact/address/employer/legal-representative
     correlations when available.
2. 名下账户资金情况
   - account set, time span, inflow/outflow amounts and counts, balance/retention
     if available, account role leads, and quality warnings.
3. 主要来款情况
   - a short narrative paragraph first, then Top20 inflow counterparty table.
4. 主要付款情况
   - a short narrative paragraph first, then Top20 outflow counterparty table.
5. 资产、理财、涉诉、查控和现金线索
   - vehicle/house/insurance/wealth-management/court/tax/legal/cash findings with
     exact tables where needed.
6. 可疑资金情况
   - transfer, concealment, fast pass-through, public-to-private, missing
     counterparty, group-association, or transaction-environment leads.
7. 研判意见和补证方向
   - what has 已有数据支持, what is only a lead, and which records should be obtained.

## Project / Corruption / Bid-Topic Report

Use for project funds, bid-rigging, bribery/corruption leads, benefit transfer,
company channels, or public-to-private project flows.

Selecting this mode records the user's requested subject only. It does not activate
any project, relationship, benefit-transfer, bidding, or bribery fact. Apply the
Scenario Activation Gate to every section below. If the relevant capabilities and
receipts are absent, retain only verified general scope, source boundary, and evidence
requests; omit the inactive numbered sections rather than filling them with a case
theory or placeholder narrative.

Recommended structure:

1. 前言
   - summarize only the verified data scale and active claim scope; do not restate
     a user-supplied scenario as a fact.
2. 涉案人员及公司关系（conditional）
   - include only field-matched corporate or authoritative relationship receipts;
     otherwise omit the entire section and any relationship map.
3. 人员及公司账户资金情况（conditional）
   - subject/account table by holder, account count, inflow/outflow, balance,
     major counterparties, and account roles only for exact verified fields.
4. 项目资金情况（conditional）
   - include only when authoritative project/payment receipts and matching transaction
     receipts activate the exact source, recipient, flow, and purpose claims.
5. 已激活的专题情况（conditional）
   - include only exact evidence-backed leads permitted by the source-capability
     combination; never add the other project/corruption/bid typologies for coverage.
6. 差额、分配或利益测算（conditional）
   - include only verified inputs and a permitted deterministic calculation whose
     lineage is receipt-bound; user-provided assumptions or case theory do not activate it.
7. 侦查取证重点
   - contracts, bidding files, invoices, ledgers, bank receipts, tax records,
     asset registry, communication/device/platform records, and witness/personnel
     evidence.
8. 综合研判结论
   - one case-material conclusion, not a list of tool findings.

## Fund-Destination And Continuation List

Use for "钱去了哪里", one-more-hop tracing, continuation accounts, and subpoena
planning.

Recommended structure:

1. 总体情况
   - seed subjects/accounts, time window, total traced amount, visible stop
     points, and unavailable downstream evidence.
2. 重点资金转移研判
   - main person/company transfer paths, short readable paragraphs, and exact
     detail tables.
3. 重点资金去向及待补证事项
   - destination categories, next-hop account availability, stop reason, and
     evidence needed.
4. 续调账户清单摘要
   - holder, account/card, institution, related amount, transaction time window,
     reason for request, priority, and expected proof.
5. 下一步工作计划
   - ordered verification actions, not generic advice.
6. 附件目录
   - exact CSV/table/material names and purpose.

## Single-Lead Verification Memo

Use for one disputed amount, one fund source/destination, or one special lead.

Recommended structure:

1. 核查结论
2. 口径与去重校验
3. 直接交易明细
4. 前手来源或后续去向
5. 资金来源/去向判断
6. 下一步补证方向

## Finding Card

Each key finding should include:

- feature name;
- 核验状态: `已核验事实`, `统计特征`, `可疑特征`, `线索`, `需复核`, or `不可判断`;
- amount, count, and date range;
- involved accounts and counterparties;
- exact table or transaction facts when returned; do not expose opaque internal
  query or audit ids;
- reason and investigative meaning;
- next verification request.

If any finding lacks source refs, evidence maturity, a boundary, or a proof
action, do not patch the prose by assumption. Either return to MCP/DuckDB or a
reviewed artifact for the missing fact, or downgrade the finding to a line of
evidence boundary and补证 direction.

## Writing Rules

- Write as public-security economic-investigation material, not a plugin log,
  SQL note, notebook, or generic data-analysis report.
- Use public-security material title hierarchy unless the user asks for another
  style: main title `关于××案涉案资金分析研判情况的报告` or the matching
  dossier/topic title; first-level headings use `一、二、三、`; second-level
  headings use `（一）（二）（三）`; third-level headings use `1. 2. 3.`.
- Prefer case-material section names such as `基本情况`, `数据来源及分析范围`,
  `总体资金情况`, `重点对象资金分析`, `可疑资金特征`, `资金流向研判`,
  `综合研判意见`, and `下一步工作建议`.
- Tables should be named as evidence attachments, for example
  `表1 ××主要来款对手方Top20（单位：元）`, `表2 ××账户资金流向明细`,
  `表3 续调账户清单`. A table title must state the object, metric or purpose,
  and unit when relevant.
- Start with facts and the case-material conclusion. Put caveats where they
  affect interpretation; do not turn the opening into a methodology disclaimer.
- Narrative may use `万元` for readability. Tables should preserve exact amounts
  and units when precision matters.
- Dates in narrative should usually use `YYYY-MM-DD`; exact timestamps belong in
  detail tables when needed.
- Do not use technical expressions such as `DuckDB`, `analysis_txn_detail_idx`,
  `txn_dedup`, `query id`, `tool`, `gate`, `doctor`, `eval`, `write_blocked`,
  `父子级`, or `本轮`.
- Avoid vague analytics labels such as `本案账户`, `外部资金来源`, `账户侧`, or
  `前序补入线索` unless the sentence immediately explains who paid whom, when,
  how much, through which visible account, and what it suggests.
- Each major finding should follow this order: fact paragraph -> table or
  资金流向图/evidence -> analysis meaning -> boundary and next proof.
- Tables should not appear immediately under a section title without a short
  explanatory paragraph.
- Each evidence table, Top table, flow table, and continuation list must be followed
  by a short analysis paragraph explaining the abnormality, proof value, evidence
  limit, and next proof action.
- 资金流向图 must show only 已有数据支持的交易边. Split dense graphs
  into several small diagrams, each explaining one issue.
- Do not write legal conclusions from bank-flow data alone. Use `涉嫌`, `可疑`,
  `线索`, `需进一步调取`, and `待补证` when evidence is incomplete.
- Do not use conclusive wording such as `证实犯罪`, `已构成`, `实际控制`,
  `黑社会性质组织`, `共同犯罪`, `洗钱事实成立`, `虚开事实成立`, or `非法所得`
  unless the user has supplied separate legal/proof material and the report conclusion has
  passed review.
- If a report file or attachment is generated, mention the returned path or id
  in the handoff, not inside ordinary report paragraphs.

## Quality Gate

Before presenting report-grade material, verify:

- the selected report mode matches user intent;
- every scenario heading is backed by the required same-case source capabilities and
  host EvidenceReceipts, and every inactive scenario is absent rather than templated;
- an ordinary full-case report does not enumerate all typologies as fixed coverage;
- every headline amount, count, Top20, time window, and account/holder set is
  backed by deterministic facts or marked unavailable;
- duplicate/card-replacement and scope boundaries are handled;
- each table has a clear purpose and exact units;
- every 资金流向图 edge has 已有数据支持 or is clearly separated as 线索候选 context;
- the report has no raw query dumps, raw table names, plugin instructions, debug labels,
  or internal ids;
- the material separates data facts, suspicious features, investigative leads,
  evidence gaps, and forbidden legal upgrades.
