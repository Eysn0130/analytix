---
name: report-builder
description: 用于生成、续写、修订或整理公安经侦资金研判报告、专题核验材料、单线索核查材料和附件说明。适合用户明确要求“写报告/生成材料/完善报告/报告附件”时使用；报告先给研判结论，再写基本情况、资金流入流出、重点对手方、异常特征、资金去向、核验意见和补证建议，不展示内部状态。
---

# 经侦研判报告

Report Builder turns reviewed evidence into formal public-security economic-investigation material.
It owns report delivery; it must not take over ordinary analysis.

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for wording, [report-schema](../analytix-fund-analysis/references/report-schema.md)
for structure, and [analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md)
for continuation, amount checks, visuals, and attachments. Read
[public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
for 公安公文式篇章、证据成熟度、法律敏感降级和表后研判要求. Read
[investigation-answer-contract](../analytix-fund-analysis/references/investigation-answer-contract.md)
before drafting final report wording: internally structure the material as
`FinalInvestigationAnswer`, then pass that structure to `validate_report_claims`
as `final_investigation_answer`; users see only公安经侦自然语言, not JSON. Read
[database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
only when a report figure, disputed amount, custom 专项资金核算, or attachment table needs governed source verification.

Desktop/source boundary: 普通案件用户默认不看进度；如必须发工具前消息，只能逐字写 `正在核验当前案件事实。`
不要公告 workflow、tool、SQL、DuckDB、本地路径或文件搜索。P0 隔离期内
`run_full_case_analysis` 不对 Agent 广告，且无论 `write_report` 取值均在执行前拒绝；
不得模拟该工具，也不得从 shell、本地 DuckDB、旧报告、缓存或其他弱来源拼接报告事实。
Do not use local DuckDB for ordinary report figures.

## Active P0 Containment Gate

报告请求只返回宿主固定的能力/证据缺口：说明当前正式报告准备与发布能力不可用、已检查范围、
缺失的同案同轮同快照 EvidenceReceipt、claim validation、PublicationReceipt，以及补证动作。
不得生成报告草稿、摘要、金额核对、表格、图、附件、staging、manifest 或文件；不得调用
`run_full_case_analysis`、报告 fallback 或模型补写。用户提供既有报告时转 `claim-review`
做逐项复核，但复核结果仍不能授权新报告发布。

## Use when

- 用户明确要求报告、材料、附件、草稿、生成、导出、续写、修订或正式文书。
- 已复核分析需要转为公安经侦研判报告、专题核验材料、单线索核查材料或附件说明。
- 用户要求修补报告段落、核对报告金额、插入图表/附件或 QA 正式措辞。

## Not for / Do Not Use

- 无报告意图的普通全案分析、quick fact、账户/主体画像、开放式深挖或图表交付。

## Workflow (Post-P0 target; inactive until the host publication pipeline is authoritative)

1. 先确认报告意图，再选择 `report-schema` 中的材料模式：全案简报、单人/单公司画像、
   项目/职务/招投标专题、资金去向与续调清单、单线索核查材料。
2. 内部明确案件身份、权威事实来源、统计范围、数据质量、金额依据、核验意见、
   交付状态和未解决事项；对外写成本次依据、统计范围、暂不能认定和需补证事项。
   casegraph/fundgraph 只是地图，只有有交易边或 fact support 时才能支撑结论。
3. 在列提纲前执行 `report-schema` 的 Scenario Activation Gate。工程/项目、项目款、
   利益输送、串通投标/围标、行贿/受贿等专题，只有宿主权威 registry 中同 thread、turn、
   case binding、context epoch、dataset snapshot 的有效 `EvidenceReceipt`，且 receipt 的
   source capability 支持该具体 claim 时才可出现。报告模式、用户案情描述、模型/MCP 自报、
   candidate receipt、旧报告、缓存和非空 evidence id 均不能激活专题。未激活时整段省略：
   不写标题、叙事、表格、图、`待核` 占位或假设故事。普通全案不是 typology checklist，
   不得为了“覆盖完整”固定写入所有专题。
4. 只有宿主 EvidenceReceipt、claim、PublicationReceipt registry 与原子发布链全部接通并
   通过对应回归后，才可启用受控 full-case staging。P0 隔离期两种 `write_report` 取值均不得
   执行；不得重试、改走本地 fallback 或让模型自报 receipt。路径、可打开、渲染和
   inspection 状态都不是发布证明。
5. 正式措辞前先把结论、已核事实、资金路径、异常特征、证据边界和补证动作整理成
   `FinalInvestigationAnswer` 内部结构，再对紧凑 claims/facts 调用 `validate_report_claims`
   一次，并传入 `final_investigation_answer`；返回 unsupported
   或 missing source boundary 时，直接把可见报告降级为 `暂不能认定`、`需复核`、`需补证`，
   同一轮不要反复验证。
6. 路径/续调/附件结论用 `validate_continuation_list`。Top20 表、特征表、流向表、图表、
   dashboard、workbook、PNG/JPG 或报告插图交给 `visual-evidence` 协调可用
   Analytix/Codex 交付面并检查；无可用生成/渲染能力时写交付缺口，不得声称已生成。
7. 报告金额、附件统计或自定义表需要先经过 source preflight、schema/profile/explain/diagnose
   和证据记录，再进入正式文字；sample preview 或 query plan 不能单独支撑报告金额。

<!-- release-contract: selected report mode; full-case briefing; one-person/one-company dossier; project/corruption/bid-topic report; single-lead verification memo; proof; continue; continue an existing report; full-report amount verification; preserve the existing report structure; recompute the displayed figures; verifying the modified section; materialized draft or explicit delivery gap. -->

## Output Contract (Post-P0 target; inactive during P0 containment)

正式材料必须标题后先写 `## 初步研判结论` 或 `## 研判结论`，再写基本情况、资金流入流出、
重点对手方、异常特征、资金去向、核验意见、补证建议。重要发现要同时写事实、表格/链路支持、
案件意义、边界和证明动作。
这些是可选材料类别，不是要求填满的事实模板。任何场景化标题或段落都必须先通过 Scenario
Activation Gate；没有对应 source capability 和同案宿主回执时整段不出现，不能用“暂无材料”、
“待核工程款”或假设情节占位。只有用户明确询问某专题时，才可在通用补证建议中列取得该类
权威材料的动作，且不得暗示当前案件已经存在该专题事实。
每张金额表、Top 表、资金链路表或续调清单后，必须补一段表后研判意见，说明异常特征、
证明价值、仍不能认定事项和下一步材料；不得让表格或附件目录替代研判意见。
涉及 `违法所得`、`赃款`、`洗钱`、`虚开`、`实际控制`、`代持`、`最终归属`、
`犯罪团伙`、`共同犯罪` 等法律敏感表述时，必须按 `public-security-official-writing`
降级，除非用户提供的外部证据和 claim review 已支持。

报告生成最终答复不得只是文件清单。普通用户可见回答必须先给 `研判报告摘要`，至少包含：
`一、事实摘要`、`二、资金分析`、`三、异常特征`、`四、暂不能认定事项`、`五、下一步侦查建议`。
只有宿主权威 registry 中存在与同案、同轮、同数据快照、claim ledger、PII 投影和报告哈希
一致的有效 `PublicationReceipt`，才可说明正式报告已发布。文件路径、文件存在、可打开、渲染
通过、inspection 状态或模型自报 receipt 均不得升级为发布成功。当前发布链尚未接通时只能
交付能力/证据缺口，不生成非发布报告草稿或分析材料，不得展示 `/Users/...`、`/var/...`
或任何本机绝对路径。

报告涉及双方资金往来金额争议时，`## 金额核对` 必须从已核事实中写明：复核后有效金额/笔数、
初筛明细金额/笔数、已确认重复记录金额/笔数、用户质疑的竞争金额及其业务解释。若竞争
金额来自集中交易重复放大形态，必须明确不能作为新增转账金额。

报告续写或金额修补只改用户请求的部分，除非用户要求重写全文；凡可见报告含金额、笔数、
路径总额或争议金额，必须有独立 `## 金额核对`，并写本次依据、已核金额/笔数、
需复核数字和 `暂不能认定` 项。不得写 `按...口径`、`金额口径`、`来源边界`、`当前案件可见`、
`多账户`、`完整往来`、`历史往来`、`集中链路`、`证据状态`、`同事实去重` 或工程词；
使用 `统计范围`、`核验范围`、`核验意见`、`重复风险复核`、`需在材料中列明` 和 `不得混同`。
生成报告的最终可见答复只要正文出现 `元`、`万元`、笔数或具体资金链路金额，就必须在
`研判报告摘要` 之后、交付清单之前另起 `## 金额核对`。这一节不得省略、改名或并入
`资金分析`，且至少写两方有效金额/笔数、初筛流水记录、已确认重复记录、竞争金额为何
不能作为新增转账金额，以及本次依据和核验意见。

## Completion Gate

P0 隔离期只有能力/证据缺口答复可以完成：必须确认没有报告草稿、摘要、表格、附件、路径或
文件写入，没有调用或模拟隐藏的 full-case 工具，没有把未激活场景写成事实。正式报告完成
条件仅在宿主发布链接通并通过对应迁移与回归后启用。
