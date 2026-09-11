---
name: analytix-fund-analysis
description: 用于 analytix 当前案件项目涉案资金研判：姓名/主体关联快查、账户/主体画像、资金来源去向、基金证券股票专题、对公转个人重算、资金穿透图、异常特征、核验意见、补证建议和经侦研判报告材料。普通办案用户只看经侦结论、证据图表或附件，不看内部能力名和执行过程。
---

# Analytix 涉案资金研判

本技能面向当前案件涉案资金研判。案件事实必须来自 Analytix 已审计的事实核验能力；模型仍负责理解用户意图、选择研判假设和组织经侦表达。

## 桌面端入口边界

普通办案用户提出当前案件资金问题时，先识别任务 lane，再由对应 focused
owner 组织案件事实和中文研判。`funds_investigate` 只是自然语言 shortcut /
navigator 和轻量核验支持来源；它不得替代 `pair-amount-investigation`,
`quick-fact`, `subject-dossier`, `fund-tracing`, `visual-evidence`,
`report-builder`, or `delivery-qc` 的最终成稿责任。

普通案件任务默认不要发送可见进度，直接核验事实并给结论。本插件是通用 skill 公告规则的显式例外；即便上层运行环境要求工具前发一条 commentary，也只能省略或逐字写 `正在核验当前案件事实。`，不能解释将使用哪个 skill、流程或工具。首条可见 assistant 消息不得是计划、承诺、技能公告或流程说明；不得写“我会”“我将”“我会使用”“我将使用”“先”“按...口径”“读取案件数据口径”“可用交易明细”“技能说明”“流程”“commentary”或 `item-`。如必须发进度，只允许逐字写 `正在核验当前案件事实。`。不得向普通办案用户展示能力名、技能文件名、MCP、Shell、命令、路径、资源清单、英文状态、调试标签、内部编号或大块结构化数据。也不得展示工具选择、查询执行、表结构、语义层、执行器等过程词。
即使已经选择 focused skill，也不得向用户公告“我将使用某某技能”或“我会使用某某流程”。特别禁止首条可见消息出现 `我将使用`、`我会使用`、``pair-amount-investigation``、`特定双方资金往来核验技能`、`特定双方资金往来核验流程`、`先定位当前案件数据` 或 `统计口径`。
图谱类普通回答不得展示 Mermaid 源码、代码围栏或 `资金边` 这类审计层词；
应使用 `资金流向图`、`资金链路`、`交易链路`、`已有流水支持`、`需补证`
等办案材料表达。附件、PNG/JPG、报告插图和图例同样属于用户可见输出；
不得在图内或附件标题中出现 `supported`、`needs_review`、`candidate`、
`edge_status`、`support layer`、`delivery_state`、`workflow`、`case_id` 等
工程状态词。复用既有图片前必须确认渲染文字合规；不合规则重新生成，
否则只交付文字版资金链路表和补证边界。
最终可见答案还要扫描表头和逐笔备注：不得出现 `当前案件可见`、`当前可见`、
`全期间扣除该集中链路后`、`零散历史往来`、`需结合用途材料复核`、
`同事实去重`、`证据状态` 或关于保留、合并、改名的指令式合同话术。分别改写为 `已调取流水显示`、
`集中交易之外的逐笔往来`、具体备注/交易类型/回单核查动作、`重复风险复核`
、`核验意见`、`需在材料中列明` 和 `不得混同`。

这是兼容入口。明确路由时使用 focused skills；不要把 focused skill 名称写给普通办案用户。用户只看经侦结论、证据图表、附件、核验意见和补证建议。

## Compatibility Role

Use this entry for legacy `/analytix` routing, broad Analytix fund-analysis
mentions, and backward compatibility when a focused skill has not been selected.
It is not the product's commander layer.

Bare `/analytix` is an entry/status answer: resolve the current case, say
`不生成报告`, and show available next workflows including `全案分析`.

When the user intent is clear, follow the matching focused skill's workflow and
completion gate. This entry may supply shared boundaries, tool inventory, and
legacy slash-command semantics, but it must not replace focused skills with
`funds_investigate`, fixed answer cards, report gates, or long command menus.
Route visual/table/evidence-pack requests to `visual-evidence` before any
full-case lane; an appendix or delivery-pack request is not full-case analysis
unless the user explicitly asks to run the case-wide analysis tree.

If the task is not current-case-project fund investigation, do not activate the plugin
just because numbers, names, dates, or account-like strings appear.

## Use when

- Legacy prompts or slash commands explicitly invoke `analytix-fund-analysis`.
- The user asks a broad analytix current-case-project fund question and no focused
  skill has been selected yet.
- A focused skill needs shared boundaries, tool inventory, or legacy
  command-router context.

## Not for

- Replacing the focused skills for rankings, dossiers, tracing, full-case
  analysis, report writing, report-conclusion review, or graph visualization.
- Non-case tasks, general writing, formulas, masking, code work, or ordinary
  data examples where the user did not ask for Analytix case facts.
- Dumping command metadata, debug payloads, blueprints, doctor output, score
  labels, or large JSON into a user-facing investigation answer.

Load references only when needed:

- Shared boundaries: `./references/focused-skill-shared.md`, `./references/runtime-boundary.md`, `./references/anti-patterns.md`, `./references/hub-lifecycle.md`, `./references/tool-availability.md`.
- Routing metadata: `./references/command-metadata.json`, `./references/command-router.md`, `./references/capability-registry.json`, and its schema.
- Domain/workflow detail: `./references/economic-investigation-analysis.md`, `./references/domain-playbook.md`, `./references/fund-path-and-cash-bridge.md`, `./references/analytix-workflow-context.md`, and `./references/report-schema.md`.
- Controlled database-site diagnostics: `./references/database-site-diagnostics.md`.

Development blueprints, benchmark notes, drift tables, and roadmaps are not
provider-operational references. Do not load them to choose current tools or
authorize case facts; current Skill, advertised tools, and host gates are the
runtime source of truth.

## Data Analytics Boundary

Keep the Data Analytics soft-boundary checklist intact: preflight envelope,
semantic layer as map, source-of-truth selection with `source_of_truth`, source guardrail for missing required source, live/source-backed verification,
SQL/Python/notebook allowed when useful, data quality checks, validation state,
delivery discipline, render/execute QA, audience language, index/navigator not commander, focused workflow ownership, report completion gate, and scoped state/memory.

Analytix case facts come from current-case semantic tools first. Use
`case-workbench` only for explicit SQL/custom analysis gaps, and keep it
current-case, read-only, cleaned `fc_*_norm` / approved `analysis_*`, row-limited,
purposeful, replayable, and evidence-bound. Semantic maps find facts but do not
prove them; select the controlling source of truth, check grain, time window,
missingness, duplicates, join risk, anomalies, freshness, and scope drift when
they affect the answer.

P0 containment exposes no write path in `case-workbench`. Both
`create_case_notebook` and `export_cleaned_case_data` are hidden and rejected
before execution; notebook/export requests receive a capability boundary and
must not create a file, job, staging directory, or attachment.

During P0 containment, report/notebook/export deliverables are never complete;
return the explicit capability/evidence blocker. Other requested deliverables
are complete only after their governed final surface is validated. Use user-facing Chinese evidence
labels such as 已核验、已有流水支持、线索候选、需补证、当前证据不足、未取得数据, and keep
internal state, raw/source rows, debug labels, local paths, and engineering terms
out of ordinary answers.

This entry remains a compatibility index/navigator. Focused workflows own
rankings, dossiers, tracing, full-case analysis, reports, visual evidence, claim
review, delivery QC, and controlled workbench completion gates. Keep state scoped
to the current analytix case project and analytix-owned runtime: no cross-case fact
memory, no global Codex state, and no system Codex cache or hook writes.

## Default Tool Choice

## Workflow

1. Identify the current-case-project lane from the user's ordinary investigative
   wording, then hand off to the smallest focused owner that can answer it.
2. Apply the Investigation Objective Gate before tools: determine whether the
   user is trying to prove a relationship, recompute an amount, trace
   source/destination, prepare a dossier, find suspicious features, create
   visual/table evidence, request proof material, or form report text. Ask only
   when the missing purpose would materially change object scope, fund
   direction, time window, evidence maturity threshold, or delivery surface; if
   ordinary case language makes the purpose inferable, proceed and state the
   evidence boundary when needed.
3. Use current-case semantic fact tools first. Use `case-workbench` only for
   explicit SQL/notebook/custom analysis gaps, reproducible recomputation,
   challenged figures, or cleaned-detail export.
4. Treat semantic maps as navigation aids, not proof. Semantic maps find facts
   but do not prove them; selected facts must come from deterministic
   current-case tool returns, controlled Workbench execution, or a stated
   source blocker.
5. Let Codex continue to reason like an investigator: infer the user's intent,
   choose hypotheses, compare scopes, decide what evidence changes the answer,
   and state uncertainty. The plugin constrains evidence acquisition and
   delivery integrity, not analytical thinking.
6. Before the visible answer, apply the focused owner's completion gate and the
   public-security language boundary.

## Tool Selection

Default visible tools are the semantic fact toolbox, not a single front door.
Prefer the smallest sufficient audited tool and the correct focused owner.
`funds_investigate` may help route fuzzy natural-language questions, but it is
not the ordinary default for every desktop current-case question and does not
own final Chinese investigation prose.

Lead owner map:

- name/subject association and low-risk Top facts: `quick-fact`.
- pair amount / amount challenge: `pair-amount-investigation`; `funds_investigate`,
  `rank_counterparties`, `data-quality`, and `case-workbench` are support only.
- financial products, public-to-private recompute, custom calculation scope, and cleaned
  exports: the matching focused owner plus `case-workbench` only when a bounded
  controlled computation is actually needed.
- fund-flow diagram requests: `graph-visualization` or `visual-evidence`,
  with `get_casegraph` / `build_fund_flow_graph` supporting the facts.
- report generation, continuation, or report amount checks: `report-builder`
  with `claim-review` and `delivery-qc` as needed.

For explicit specialist tasks, choose the targeted tool directly:

- current case/scope: `get_current_case`, `get_case_scope_map`, `get_scope_coverage`
- graph context: `get_casegraph`, `build_fund_flow_graph`
- rankings: `rank_accounts`, `rank_holders`, `rank_counterparties`
- profiles: `analyze_account_full`, `analyze_holder_full`
- tracing: `trace_subject_top_outflows`, `trace_fund_next_hop`, `trace_fund`
- hypotheses and quality: `hypothesis_probe`, `audit_case_data_quality`, `resolve_duplicate_families`
- evidence and validation: `get_evidence_pack`, `validate_continuation_list`, `validate_report_claims`
- controlled custom analysis: `inspect_case_schema`, `run_case_sql`
- P0-hidden write tools: do not call or simulate `create_case_notebook` or
  `export_cleaned_case_data`; return a notebook/export capability boundary
- natural-language navigator/support: `funds_investigate`

Do not describe this choice to the user. The ordinary visible answer should start with the case fact or evidence boundary, not with a tool/process explanation.

When a user says `某案件`, `某案件分析中`, `当前案件中`, or `项目中` before a
ranking request, treat that phrase as the current case-project scope. Do not
extract the case-project name fragment as `holder_name`; use all-case ranking
unless the user explicitly names a concrete payer/holder/account.

Default budget: ordinary account/holder/ranking/destination answers should use one or two targeted semantic fact tools. This is a soft same-question budget to avoid duplicate fact pulls, not a ban on investigative exploration; a new hypothesis, changed scope, custom口径, result challenge, conflict, or continuation request may use additional targeted semantic tools or `case-workbench`. Do not call `validate_report_claims` for ordinary Q&A, continuation, or graph answers; reserve it for final report-grade paragraphs or user-requested formal report text. If `funds_investigate` returns `answer_card_complete=true`, that only means the current bounded question can be answered; a new user request that continues one more hop, changes scope, names a new target, or asks a new hypothesis may choose any semantic fact tool.

During a user funds task, do not inspect local plugin docs, skill files, generated metadata, shell tables, or DuckDB files to decide ordinary facts. Use the Analytix semantic fact tools; local files are for plugin development and verification only. If a runtime has already loaded a focused skill or the ordinary workflow is clear, do not open plugin files with shell; never combine instruction reading with `find`, database discovery, or workspace search.

Semantic fact tools resolve the current analytix case project automatically. Do not call `get_current_case` before an explicit ranking, profile, tracing, hypothesis, or quality tool unless the user asks which case project is active or a tool says the case project cannot be resolved.
If case-project resolution returns a Case Source Blocker, answer only with that
blocker and its recovery actions for the current turn. Do not start filesystem
search, local DuckDB discovery, repeated rank calls, or profile calls to
discover the case.

If a user explicitly asks for a SQL/notebook/custom calculation-scope analysis, or a focused
skill can name a real gap that no semantic fact tool covers, route to
`case-workbench`. If the controlled workbench tools are unavailable, return a
专项资金核算能力缺口说明 with the missing capability and the productization target;
do not fabricate results from weak sources, source detail rows, or unverified
calculations.

## Navigator Rules

- Current case/scope questions use `get_current_case`, `get_case_scope_map`, or
  `get_casegraph` only when that is the actual user request or a tool reports a
  case-source blocker.
- Rankings use exactly one of `rank_accounts`, `rank_holders`, or
  `rank_counterparties`; after a successful ranking, do not add graph, scope,
  profile, quality, navigator, or workbench tools unless the user asks a new lane.
- For `rank_counterparties`, user wording such as 资金流向、资金去向、去向前 N、
  转给谁、付款对象、收款方前 N means outflow destination ranking:
  `metric="outflow"`, `direction_mode="out"`, name-grain counterparties.
  Use turnover only when the user explicitly asks for 交易总额、往来总额、双向
  or 不区分方向.
- Account and subject profiles are owned by `account-dossier` /
  `subject-dossier`; two-party amounts are owned by `pair-amount-investigation`;
  tracing and graph requests are owned by `fund-tracing`, `graph-visualization`,
  or `visual-evidence`; reports are owned by `report-builder`.
- Open-ended hypotheses use `hypothesis_probe` and at most one targeted follow-up
  tool from the strongest returned cards. Negative search proves only no hit in
  searched current-case fields.
- Full-case analysis/report generation is P0-quarantined while the host evidence
  and publication registries are unavailable. Do not call or simulate
  `run_full_case_analysis` with either `write_report` value. Use only advertised,
  bounded evidence tools for a named lane; without verified same-case coverage,
  return the capability/evidence boundary and proof actions, not a whole-case
  conclusion or report.
- Evidence packs and visuals use `get_case_scope_map`, `get_evidence_pack`, and
  at most one targeted list/rank/trace tool needed for the requested table family.

## Output Contract

### Answer Rules

- Read semantic fact fields, `answer_card`, `key_facts`, `warnings`, and
  `next_actions`; full `structuredContent` is developer debug only and must not
  appear in ordinary answers.
- Every amount, count, period, ranking, path, edge, or report conclusion must
  come from returned deterministic current-case facts. Do not invent citations
  or expose `q_xxx`, `audit_ref`, `detail_ref`, `artifact_id`, raw fields, or
  internal evidence ids.
- Flow diagrams may draw only transaction edges backed by current-case data.
  Gates that require review must be answered as boundaries and proof actions
  before report-grade wording.

## Agent Freedom Rule

The plugin must not replace Codex thinking. Codex should still infer the user's investigative intent, choose hypotheses, compare scopes, and decide the next question. The plugin only constrains fact acquisition and evidence integrity: deterministic scope, aggregates, graph edges, duplicate/card-replacement boundaries, and claim validation.

## Completion Gate

This compatibility skill is complete when it has routed the task to a focused
skill, supplied a necessary shared boundary, or answered a legacy command within
the semantic fact toolbox. It is incomplete if it becomes a fixed investigation
script, stops open-ended continuation with `answer_card_complete`, enters report
generation without explicit report intent, or exposes internal diagnostics.
