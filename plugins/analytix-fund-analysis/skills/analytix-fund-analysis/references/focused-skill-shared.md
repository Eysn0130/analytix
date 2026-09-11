# Focused Skill Shared Contract

This reference is shared by Analytix focused skills. It is a production boundary
contract, not an eval fixture or report template.

For public-security/economic-investigation task expansion, read
[economic-investigation-analysis](economic-investigation-analysis.md) when the
request asks for a dossier, full-case analysis, suspicious features,
source/destination tracing, delivery material, negative search, amount
challenge, or case-wide report-style output.
Read [economic-investigation-language](economic-investigation-language.md) when
the output needs user-facing wording, report-style phrasing, table/graph labels,
or when any support fact contains English/internal status fields.
Read [analytix-workflow-context](analytix-workflow-context.md) when the request
touches current-case sync, cleaned exports, analysis indexes, visual artifacts,
report continuation, attachments, or multi-turn workflow state.
Read [public-security-official-writing](public-security-official-writing.md)
when the output is a report, formal material, table/appendix note, claim review,
subpoena/evidence request, or any answer that may be copied into case materials.

## Operating Order

Every focused skill should run in this order:

1. Classify the task lane before choosing tools.
2. Apply the Investigation Objective Gate: identify what the user wants the
   analysis to prove or support, and ask only when the missing purpose would
   materially change object scope, direction, time window, evidence standard, or
   delivery surface. If the purpose is reasonably inferable, proceed with that
   assumption, verify facts, and state the evidence boundary instead of turning
   the workflow into a questionnaire.
3. Resolve the current case and source/scope boundary only as much as the lane
   needs.
4. Resolve facts through `analytix_funds` semantic MCP tools first.
5. Use the smallest tool set that can complete the selected lane.
6. Separate deterministic facts, statistical features, investigative leads,
   evidence gaps, and forbidden-as-fact material.
7. If the semantic toolbox cannot answer an explicit custom computation,
   reproducible-analysis, SQL, or notebook request, hand off to `case-workbench`
   and preserve the internal source/scope, validation, and evidence boundary;
   user-facing prose should translate these into 本次依据、统计范围、核验意见、
   暂不能认定 and 需补证事项.
   If the user explicitly requests a cleaned-detail file, `case-workbench` may
   call `export_cleaned_case_data`; keep SQL/notebook paths read-only, require
   export confirmation, and do not claim delivery before CSV/XLSX read
   inspection passes.
8. Apply the skill's completion gate before answering.
9. Hand off to a more specific focused skill when the user changes lane.

Do not let the compatibility entry, `funds_investigate`, answer-card prose,
workbench previews, or a report gate replace the focused skill's own workflow
or final Chinese investigation answer.

## Investigation Objective Gate

Borrow the mature Data Analytics pattern of starting from the decision, and the
Investment Banking / Public Equity Investing pattern that routers select a lead
owner without doing the specialist work. For Analytix, translate that into a
case-investigation purpose check before tools:

- Identify whether the user is asking to verify a relationship, recompute an
  amount, trace source/destination, build an account or subject dossier, find
  suspicious features, prepare evidence requests, create charts/tables, or form
  report material.
- Ask a concise clarification only when the missing purpose would change the
  controlling source, account/subject scope, fund direction, time window,
  success-state filter, deduplication rule, evidence maturity threshold, or
  requested deliverable. Ask at most three concrete questions, in case-material
  language, and do not expose tool names or workflow names.
- If the purpose is clear or safely inferable from ordinary case language,
  proceed without asking. State the assumed purpose only when it affects the
  evidence boundary or the user may reasonably expect a different scope.
- Never use clarification as a way to avoid source-backed work. A broad prompt
  such as `某人资金研判` already implies a subject dossier; `继续追一层` implies
  fund tracing; `做个图` implies a funds-flow or evidence visual owner; `这个金额
  对吗` implies an amount challenge.
- After the first supported answer, include the next investigative direction
  only when it follows from reviewed facts: next-hop tracing, Top counterparty
  profile, duplicate/换卡复核, account-control evidence, product/KYC/order
  materials, platform/merchant/tax-contract subpoenas, or report/appendix
  materialization. Label each as 已有数据支持、线索候选、需补证, or 当前证据不足.
- If the user's stated goal is report-grade conclusion, legal wording, or
  external evidence request, raise the evidence maturity bar instead of
  upgrading leads into facts. The result must say what is proven, what is only a
  lead, and what material would close the gap.

## Audience And Language

Write ordinary answers for public-security/economic-investigation users, not
plugin maintainers. This applies to final answers, graph text, report text,
appendix/table titles, failure explanations, and progress narration.

Translate implementation status into case-material language:

| Internal term | User-facing wording |
| --- | --- |
| follow-up | 下一步追查 / 补充核验 |
| Controlled Workbench / Workbench | 专项资金核算 / 受控复核 / 自定义统计范围核验 |
| case_id | 当前案件 / 案件编号 |
| Mermaid | 资金流向图 / 资金穿透图 / 关系图 |
| supported seed | 本轮可核验交易 / 可作为追踪起点的交易 |
| supported | 已有数据支持 |
| needs_review | 需补证 / 待复核 |
| candidate | 线索候选 / 疑似关联 |
| blocked | 当前证据不足 / 来源未就绪 |
| validation state | 核验状态 |
| delivery contract | 交付要求 / 成果验收要求 |
| source envelope | 本次依据与统计范围 |
| source boundary | 本次依据与统计范围 |
| artifact | 报告 / 附件 / 图表 / 证据包 |
| claim | 研判结论 / 事实判断 |
| evidence ledger | 证据记录 / 核验依据 |
| graph node / edge | 图谱节点 / 资金链路 |

Internal tool names, MCP names, skill names, raw rows, JSON/debug payloads,
audit ids, local paths, and implementation process should stay in `_meta`,
audit, doctor, eval, or evidence records. They must not appear in ordinary user
prose unless the user explicitly asks to debug the plugin.

Do not announce workflow or implementation names to ordinary case users. The
first visible assistant message must not be a plan, promise, or workflow
explanation; default to no progress before tool use. For
example, do not write `quick-fact`, `SKILL.md`, `List MCP resources`, `Shell`,
`cat`, `grep`, or local filesystem paths in progress text or final answers.
Say `姓名/主体关联快查`, `资金事实快查`, `专项资金核算`, `资金穿透核验`, or
`正在核验当前案件事实` instead. User-visible progress must not say
`我会`, `我将`, `先`, `按...口径`, `读取案件数据口径`, `可用交易明细`,
`先读取...技能说明`, `先看...文件`, or `按...规则先...`.

Visible answer wording must also stay out of source-window language. Do not
write `当前案件可见`, `当前可见`, `本窗口`, `核验窗口`, `全区间`, `全期间`,
`多账户`, `多个账户`, `完整往来`, `历史往来`, `集中链路`, `簇`, `cluster`,
`Mermaid`, or
`资金边` in ordinary answers, graph text, report text, or figure labels. Use
case-material wording instead: `经已清洗流水复核`, `已调取流水显示`,
`本案已清洗流水显示`, `本次转账涉及多张付款/收款账户`,
`长期转账关系`, `集中转账`, `集中交易组`, `资金流向图`, and `资金链路`.
Before sending any visible answer, scan tables and row notes for the exact
phrases `全期间扣除该集中链路后`, `零散历史往来`, `需结合用途材料复核`,
`同事实去重`, `证据状态`, and instruction-contract wording about preserving or renaming report terms. Rewrite them as case-material language:
`集中交易之外的逐笔往来`, `摘要/类型提示用途线索，需先核对流水备注、交易类型、回单和双方说明`,
`重复风险复核`, `核验意见`, `需在材料中列明`, and `不得混同`.

## Analytical Expansion Rule

Do not shrink domain language into a simple lookup. A brief user prompt may be a
complete workflow:

- `某卡/某账户资金研判` means 账户资金画像：账户基本情况、登记/开户信息、资金流入、资金流出、重点对手方、账户角色、异常特征、暂不能认定事项和核查建议。
- `某人/某公司资金研判` means 主体资金画像：账户基本情况、登记账户清单、重点账户表、资金流入流出、重点对手方、异常特征和核查建议。
- `全案分析/完整研判/跑完整功能树` means Full Case Analysis.
- `继续追一层/资金来源/资金去向` means Fund Tracing.
- `团伙关联/共同控制/IP/MAC/设备/支付通道/虚拟资产/票税合同` means
	  Investigation Lab or a dossier lane with explicit 支持程度 and evidence boundaries.
- `有没有/是否出现/查一下` with names, aliases, remarks, or keywords may mean
  Negative Search with searched-field boundaries.

Only use `quick-fact` when the request is truly bounded to one low-risk metric,
ranking, or count. Two-party amount reconciliation and amount disputes are
owned by `pair-amount-investigation`.

## Professional Judgment Rule

Every substantive focused answer needs investigative judgment: explain the
abnormal pattern, case significance, unsupported limit, and next proof action.
A fact card, ranking number, graph shape, or workbench result is not enough
when the user asked for funds investigation material.

## Fact Boundary

- Use `analytix_funds` MCP tools for current-case-project facts. Default to semantic
  tools as the low-context source map, but do not turn that preference into a
  hard route. Use 专项资金核算 / Workbench when the user asks for a custom or
  reproducible computation, or when semantic facts are unavailable,
  incomplete, inconsistent, challenged, or insufficient for the stated
  question.
- Do not treat weak sources, unverified calculations, bounded previews, or raw
  rows as source-backed facts.
- Ordinary investigation facts must come from Analytix cleaned tables and
  analysis indexes exposed through MCP facts. Do not use `fc_*_raw`, source
  files, unreviewed calculations, or source detail rows as a shortcut around the cleaning
  module.
- 专项资金核算 is still an Analytix-owned MCP/backend path: current case
  only, read-only, cleaned `fc_*_norm` / `analysis_*` scope only, row-limited,
  purpose-tagged, auditable, and carrying 核验状态.
- Import, cleaning, indexing, export, permission, and UI workflows belong to the
  Analytix main system. The plugin may audit coverage and analytical readiness;
  it must not execute import, clean data, repair tables, or change rules.
- If a required source lane is unavailable, stop report-grade conclusions and
  name the gap. If the missing lane is optional enrichment, continue with the
  strongest available facts and label the limitation. Do not treat missing
  contact/address/IP/MAC/task-feedback/coercive-measure fields as proof that no
  relationship, device clue, task feedback, or enforcement clue exists.
- A custom workbench result may answer the current bounded question when it is
  audited and safe. Recommending a new semantic MCP tool is a productization
  follow-up, not a reason to refuse the current controlled analysis.
- Omit `case_id` for ordinary tool calls so analytix_funds resolves the
  current case project from the runtime workspace and `.analytix/case-project.json`.
  The required source chain is `current conversation workspace -> case-project
  binding -> analytix data-analysis case -> analytix_funds MCP`; if that source
  is unavailable, return a 当前案件项目来源缺口说明 and recovery actions. Do not ask
  the user to browse case folders, do not list directories, do not search
  `*.duckdb`, do not scan `Downloads/case`, and do not infer `case_id` from
  historical output, fixtures, cached text, previous failed tool calls, or a
  global case state. If an explicit `case_id` does not match the current case
  project binding, stop with that gap and do not try other ids.
- Shell is only for reading the selected skill/reference instructions or
  plugin engineering checks. Do not chain a skill/reference read with `find`,
  `rg`, `ls`, local data discovery, report discovery, or workspace searches for
  `.duckdb`, `.db`, `.sqlite`, `.csv`, `.xlsx`, `.parquet`, screenshots, or old
  outputs. Such local discovery is a source-boundary failure for ordinary
  current-case facts.
- `funds_investigate` is a navigator and support-material compiler. It must not
  monopolize ranking, dossier, tracing, lab, full-case, or report decisions.

## Task Lanes

| Lane | Correct owner | Failure pattern |
| --- | --- | --- |
| Quick fact / ranking | `quick-fact` | Running dossier/full-case/report workflow for a bounded Top/count question, or using quick-fact to finalize a two-party amount dispute. |
| 金额核验 / amount challenge | `pair-amount-investigation`; `rank_counterparties`, `funds_investigate`, `data-quality`, and `case-workbench` are support only | Finalizing from `rank_counterparties` alone; repeating a card title or answer draft as the final answer; outputting only one bare amount; failing to reconcile holder/account scope, counterparty grain, 原明细/高置信去重统计, time window, 同事实/换卡风险, 核验意见, 本次依据, 已有数据支持金额, 证据不足金额, difference reason, account/time concentration, and next-hop or Top-counterparty follow-up. |
| Account/card dossier | `account-dossier` | Answering a dossier request with only a Top row or one aggregate. |
| Person/company dossier | `subject-dossier` | Treating 线索候选-linked accounts as owned accounts. |
| Counterparty/channel | `counterparty-analysis` | Writing missing or account-like counterparties as confirmed identities. |
| Source/destination/next hop | `fund-tracing` | Drawing evidence-insufficient narrative arrows or stopping continuation because an earlier card was complete. |
| Open hypothesis / suspicious lead | `investigation-lab` | Turning leads into facts or producing tool descriptions instead of a hypothesis queue. |
| Full-case analysis | `full-case-analysis` | Treating full-case analysis as report writing or a short summary. |
| Formal report / attachment | `report-builder` | Generating report wording without claim validation. |
| Evidence request | `evidence-request` | Treating requested evidence as already obtained proof or omitting proof value and target institution. |
| Visual/table evidence | `visual-evidence` | Treating charts, ranking tables, matrices, or appendix rows as new facts or 已有数据支持的交易路径. |
| Controlled custom analysis or explicit cleaned export | `case-workbench` | Using weak sources, raw tables, fabricated results, arbitrary-directory writes, unverified calculations, or claiming a queued/uninspected export as delivered. |
| Analysis critique | `analysis-critique` | Critique generating new facts instead of naming missing lanes, route errors, and next focused skill. |
| Claim / QA review | `claim-review` | Returning only a 当前证据不足 flag without corrected wording or proof actions. |
| Delivery QC | `delivery-qc` | Facts are correct but the visible output lacks conclusion, table/rows, abnormality, case significance, limits, next evidence, or leaks implementation vocabulary. |
| Non-fund task | none | Calling case tools or imposing fund-analysis structure. |

## Evidence Language

- Separate `已证实事实`, `统计特征`, `可疑线索`, `证据缺口`, and `下一步`.
- 线索候选 accounts, same-fact duplicate families, cash breaks, missing
  counterparties, and asset/wealth-management leads are `线索/需复核` until a
  deterministic tool returns support.
- Account-opening facts support identity, account scope, bank/branch, account
  type, and 状态. They do not prove actual control, transaction purpose, or
  fund flow.
- Account role labels such as living card, transit account, convergence account,
  business-use personal card, cash-heavy account, or asset-consumption account
  are 线索候选 fund-flow roles, not legal identities or offense roles.
- Shared IP/MAC, teller, branch, location, merchant, receipt, terminal, payment
  channel, phone, address, employer, or legal representative are correlation
  leads. They do not prove actual operator, actual controller, gang membership,
  co-offending, nominee holding, platform-account control, or fund-flow purpose.
- 资金流向图或箭头只能使用已有数据支持且端点完整的交易边。
- Tables, charts, dashboards, and appendix rows must show scope, unit, time
  window, metric, direction, and 支持程度. A Top ranking, heatmap,
  matrix, or concentration chart is not an 已有数据支持的交易路径.
- Do not expose internal ids, source detail rows, large JSON, golden/oracle
  wording, diagnostic labels, `write_blocked`, report gate, doctor, score,
  blueprint text, tool names, or MCP names in ordinary user answers.

## Answer Shape

Focused answers should usually use this compact order:

1. Direct answer or current 核验状态.
2. Scope and statistical basis: case, object, time window, direction, metric, coverage.
3. Deterministic facts with amounts/counts/date ranges when available.
4. Statistical features or ranked observations.
5. Leads and evidence gaps with 支持程度 labels.
6. Next investigation or validation actions.

Skip sections that do not apply, but never skip scope when it affects the fact.
Do not lead with tool names, schema fields, protocol details, or capability
descriptions unless the user is debugging the plugin.

## Internal Structured Answer Contract

Before a substantive focused answer, report paragraph, table note, flow-chart
caption, or evidence-request summary is shown, organize the supporting material
as an internal `FinalInvestigationAnswer`. This is not visible JSON for ordinary
users. It is the经侦 answer spine that keeps the final wording source-backed:

- `investigation_objective`: purpose, object, scope, time window, and source
  boundary.
- `conclusion`: answer-first judgment with evidence maturity.
- `verified_facts`: facts with MCP/DuckDB, artifact, attachment, or other
  replayable evidence refs.
- `fund_flow_paths`: only supported transaction paths; candidate, incomplete,
  or missing edges belong in `evidence_boundaries`.
- `risk_patterns`: abnormal features tied to verified facts or source refs.
- `evidence_boundaries`: what current materials cannot prove.
- `next_proof_actions`: targeted records to obtain, target/material,
  time/field scope, linked evidence gap, proof value, priority, and forbidden
  upgrade boundary for each action.

For report-grade, claim-review, or fact-heavy delivery, pass this structure as
`final_investigation_answer` into claim validation when that path is available.
For ordinary focused answers, use the same spine internally and render it into
public-security economic-investigation prose. Missing structure may be repaired;
missing facts, amounts, transaction edges, ownership/control claims, or legal
conclusions must return to MCP/DuckDB, be downgraded into evidence boundaries,
or be stated as currently unsupported. Do not use a retry or style pass to fill
facts that the current case data did not return.
Validation failure, delivery-QC interception, and SQL diagnose reasons may be
recorded as internal `audit_events`; ordinary case users see only the translated
fact, boundary, and proof-action wording.

For substantive answers, also apply the investigation spine: 查什么、结论是什么、
依据哪些流水/账户/链路、异常在哪里、案件意义是什么、还不能认定什么、下一步调什么
材料. Answers that only restate a support card, audit checklist, or number fail
the delivery gate even when the facts are technically correct.

Before finalizing, apply the final-answer style gate in
`economic-investigation-language.md`: preserve evidence spans, remove AI-style
openers/closers, avoid empty case-significance language, and keep the answer in
public-security economic-investigation register. Do not "humanize" by adding
chatty tone, jokes, unsourced authority, rhetorical questions, or invented
emotion. The final pass is two-step: first reread for fact fidelity, then reread
for residual formulaic writing. If a sentence does not preserve a fact, explain
an abnormal feature, state case meaning, mark an evidence limit, or identify a
proof action, delete it or rewrite it into one of those functions. For report or
case-material wording, also apply `public-security-official-writing.md`: use
the correct material type, evidence-maturity label, legal-sensitive downgrade,
and table-after-analysis requirement.

## Old-Thread Behavior Contract

Historical no-plugin sessions are behavior baselines, not production facts. The
plugin must preserve the useful work pattern while replacing raw DuckDB work
with audited MCP facts:

- confirm scope/coverage before full-case or report-grade claims;
- turn user relationships and motives into hypotheses before upgrading them;
- recompute after scope, amount, direction, time window, or dedupe changes;
- perform field-bounded negative search and state no-hit boundaries;
- continue one more hop from valuable 可证实资金链路 or explicit tracing starts;
- turn technical findings into case-material language with proof gaps.
- preserve cleaned export field order and Chinese headers unless the user asks
  for review columns;
- distinguish empty-counterparty cash-like positive records from zero-amount
  interest/non-cash records;
- continue existing reports by patching the requested section, recomputing
  displayed figures, and correcting misleading amount wording;
- use the available Analytix/Codex delivery surface to generate or refresh
  visual artifacts when the user asks for PNG/JPG/Mermaid, table, workbook,
  attachment, or report figure delivery; inspect any produced artifact, and
  state a delivery gap when no governed surface exists.

External proof materials such as account-opening records, receipts, invoices,
contracts, OCR text, and asset-registration records are proof-action requests or
user/Analytix-provided attachment facts. Skills must not claim the plugin
fetched, OCR-processed, or verified those materials unless a governed backend or
source result says so.

Never write old-case subjects, amounts, transaction ids, fixed time windows, or
golden phrasing into production answers or references.

## Impeccable-Style Anti-Patterns

Refuse and rewrite these outputs:

- Tool manual instead of investigation result.
- Large JSON or raw structured payload pasted into the answer.
- Report-gate language in ordinary analysis.
- `write_blocked`, `diagnostic`, `doctor`, score, blueprint, `/goal`, golden, or
  oracle wording in user-facing investigation answers.
- Weak source, raw table, unverified calculation, or query-result language used
  as a shortcut around Analytix source-backed facts.
- 专项资金核算 result written as full-case truth when it is only a
  bounded custom query, preview, chart-ready table, or reproducible notebook
  deliverable.
- Bounded rows or samples summarized as full-case totals.
- 线索候选 account, cash bridge, duplicate family, missing counterparty, asset
  lead, or control lead written as confirmed fact.
- Shared IP/MAC, device, teller, branch, location, merchant, phone, address, or
  channel overlap written as actual controller, operator, gang/co-offending
  relationship, or same physical person.
- Platform, merchant, wallet, OTC, exchange, or virtual-asset clue written as
  confirmed gambling settlement, laundering, underground banking, or crypto
  transfer without platform/KYC/order/wallet evidence.
- Aggregated Top counterparty drawn as a transaction path.
- 资金流向图箭头 without an 已有数据支持的交易链路.
- Legal-sensitive conclusion from bank-flow data alone.
- Same-turn repeated tools with the same object and same statistical scope when no new fact is
  needed.

## 资金画像完成门

账户、卡号、主体、人员或公司画像应覆盖：开户或登记事实、统计范围、
账户结构、时间跨度、资金流入流出金额和笔数、重点对手方、账户角色判断、
行为或异常特征、交易环境/IP/MAC/网点/商户/渠道线索、资金来源或去向线索、
暂不能认定事项和核查建议。

主体画像还应覆盖登记账户、待补证账户线索、分账户角色、联系方式/住址/单位/
法定代表人关联、共同对手方或共同渠道、IP/MAC/设备/商户/渠道重合、
已有数据支持的关联人员或公司线索、生活卡或低风险账户降级说明，以及补调
流水、开户资料、回单、平台/KYC/订单等证据的优先级。

## Full Case Completion

Full-case analysis must cover: case data volume, time span, import/cleaning
coverage, cleaned/index/report coverage, account-opening coverage,
person/contact/address coverage, task feedback and coercive-measure coverage,
transaction-environment/IP/MAC/merchant/channel coverage,
full-case inflow/outflow amount and count, holder/account ranking, each key
subject's account situation, account role distribution, Top20 counterparties,
public-to-private total-vs-classified statistics, cost normality review,
suspicious features, group-association leads, supported fund-flow context, cash
same-deposit/withdraw candidates, asset/financial-product/payment
channel/merchant/platform/virtual-asset/cross-border/tax-contract leads,
investigation-lab lead families, and continuation/subpoena queue. It is not the
same as report generation.

Plan-only output is not full-case analysis. Use a plan only when the user asks
how to run the case, when a required lane is blocked, or when the analysis must
be staged for cost/risk reasons.

## Feature And Typology Output

When reporting suspicious features, include feature family, supporting facts,
核验意见, typology lens if useful, downgrade reason, and next proof. A
typology lens such as two-card, running-score, gambling settlement, underground
banking, illegal fundraising, duty embezzlement, bribery, false invoice, or
project-case lead is an investigative hypothesis, never a legal conclusion.

Treat normal living-card behavior as a possible downgrade, not as suspicion by
default. Treat cash bridge, asset ownership, overseas/cross-border transfer, and
actual-control findings as leads until external proof closes the gap.
Treat shared phone, shared address, employer, legal representative, bank feedback
and coercive-measure records as identity/coverage/enforcement leads, not as
standalone control, ownership, or offense proof.
Treat shared IP/MAC, teller, branch, location, merchant, terminal, receipt, and
payment-platform fields as transaction-environment leads, not as proof of the
same operator or organized group. Treat virtual-asset, OTC, wallet, exchange,
invoice, contract, tax, logistics, or platform keywords as subpoena targets until
external platform/document evidence supports them.

## Report Boundary

Enter report preparation only when the user explicitly asks for a report,
formal material, draft, or attachment. During P0 containment,
`run_full_case_analysis` is neither advertised nor executable for either
`write_report` value. Return a report capability/evidence boundary and proof
actions; do not create a draft or file. Formal publication remains blocked until
the host registry validates same-case evidence, claims, dataset snapshot, PII
projection, and the rendered report and issues a PublicationReceipt. Paths, file
inspection, and model-supplied receipt IDs never authorize release.

## Handoff Rules

- A broad or ambiguous request may start at `index`, then route to the focused
  owner.
- A dossier may call a trace, rank, hypothesis, or quality tool, but the dossier
  skill owns the final dossier answer.
- A full-case task may collect facts across lanes, but report writing is a
  separate explicit handoff to `report-builder`.
- Claim review may request a targeted fact check, but it must not generate new
  investigation conclusions beyond pass/block/downgrade and proof actions.
