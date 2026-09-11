---
name: full-case-analysis
description: 用于当前选中案件的全案资金研判、系统性分析和分析树执行，覆盖清洗流水、交易环境/IP/MAC/渠道、开户信息、人员联系方式/住址、任务反馈、强制措施、主体角色、团伙关联线索、异常特征、资金来源去向和证据缺口。适合“全案分析/完整研判/跑完整分析树”；除非用户明确要求生成报告，否则不进入正式报告写作。
---

# 全案资金研判

全案资金研判用于系统性复核当前案件资金事实。用户只说 “analyze this case”
或“全案分析”时，也应扩展为完整覆盖，而不是给出薄摘要或计划。

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for full-case lanes, typology lenses, and delivery-pack expectations. Read
[analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md)
for report continuation, attachments, visuals, exports, or multi-turn handoff.
Read [public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
when the full-case answer includes report-grade tables, appendix-style rankings, formal material,
evidence requests, legal-sensitive wording, or copyable case-material paragraphs.

Desktop/source boundary: 普通案件用户默认不看进度；如必须发工具前消息，只能逐字写 `正在核验当前案件事实。`
P0 隔离期内 `run_full_case_analysis` 不对 Agent 广告，且无论 `write_report` 取值均在执行前拒绝。
不得模拟该工具，也不得用 scope、rank、quality、schema、navigator、shell、本地 DuckDB、旧报告或
文件搜索拼接全案事实。只可对用户明确限定的单一 lane 使用当前已广告的证据工具；没有宿主签发的
同案、同轮、同快照有效回执和完整覆盖时，固定输出全案能力/证据缺口、已检查范围和补证动作，
不得给出全案事实结论。用户明确要求自定义/可回放核算或挑战数字时，才转 `case-workbench`。
Do not use local DuckDB for ordinary full-case fact finding.

## Use when

- 用户要求全案分析、完整研判、系统性分析、跑完整分析树、把当前案件跑一遍或全量分析。
- 期望答案一次覆盖数据规模、时间跨度、主体/账户、事实层覆盖、进出账、重点主体/账户/
  对手方排行或优先核查对象、账户角色、异常特征、资金来源去向、证据缺口和下一步。

## Not for / Do Not Use

- 单账户/单主体画像、低风险 Top fact、特定双方资金往来核验、证据图表附件或正式报告写作。

## Workflow

1. P0 隔离期不得调用或模拟 `run_full_case_analysis`，也不得生成全案事实草稿。
2. `plan_case_analysis` 仅用于用户明确要计划；计划不是事实回执，不能解除发布阻断。
3. 不为了填清单追加排行、画像、trace、quality、schema、SQL 或本地搜索；固定说明当前
   全案能力不可用、已检查范围和缺失 lane，并列入 `下一步核查建议（续调清单）`。
4. 不列举金额、笔数、账户、主体、方向、日期、关系、设备、报价、异常特征或法律定性，
   也不生成排行、链路、表格、图、附件或报告结构。

## Output Contract

只返回：当前全案事实发布能力不可用、已检查的能力范围、缺失的数据源/宿主回执/覆盖 lane，
以及取得证据和恢复来源的建议。不得使用 `研判结论`、`初步研判结论` 或 `研判认为` 标题/措辞，
因为这些会把能力边界误呈现为案件结论。

## Completion Gate

P0 隔离期只允许完成能力/证据边界答复：说明当前全案事实发布能力不可用、已检查范围、
缺失 lane 和补证动作。不得输出 `研判结论`、金额、笔数、账户、主体关系、设备标识、
报价或法律定性；不得启动第二轮同题工具 pass。输出前扫描并改写工程词、source-window 词、
图谱内部词和法律过度推断。
