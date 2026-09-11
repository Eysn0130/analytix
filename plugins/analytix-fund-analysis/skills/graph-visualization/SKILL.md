---
name: graph-visualization
description: 用于当前案件关系图、资金流向图、资金穿透图、主体关系图和核验意见图表达。适合用户要求“画图/资金流向图/穿透图/关系图/下游图”时使用；图中区分资金主链、旁路线索、已有流水支持、需补证、未调取端点和资金断点，不把候选线索画成确定事实。
---

# 资金流向图谱与穿透表达

本能力用于当前案件关系图、资金流向图和资金穿透图表达。资金流向图谱帮助办案人看清主体、账户、
对手方和资金链路，不替代交易事实、回单、流水和余额承接核验。

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).

Desktop/source boundary: 普通案件用户默认不看进度；如必须发工具前消息，只能逐字写 `正在核验当前案件事实。`
不要写 `我先`、`我会`、`按...口径`、tool/skill 名、SQL、DuckDB、本地路径或文件搜索计划。
当前案件事实来自语义工具；普通图请求不得用本地 `*.duckdb`、shell、旧报告、缓存、schema 或 SQL
补图；never search local `*.duckdb` for ordinary graph facts.

## Use when

- 用户要求案件关系图、资金流向图、资金穿透图、主体关系图、可视化路径、下游图或图谱核查。
- 复杂案件需要关系/资金链路 context map 才能继续研判。
- 报告或答案需要已复核交易边的视觉支持。

## Not for / Do Not Use

- 把线索画成证明、装饰性绘图、无图请求下的排行/画像/追踪/报告。

## Workflow

1. 先判断是 broad relationship map 还是 narrow fund-flow path。
2. Broad map 使用 case graph 做主体/账户/对手方 context；只有交易端点、时间、金额、方向足够清楚时才画资金边。
3. Named pair plus downstream graph：用一次 `investigate_pair_amount` 固定付款到收款边，
   再用一次 `trace_subject_top_outflows` 或 `build_fund_flow_graph` 看收款侧承接。
   不要逐个下游对手方重复特定双方资金往来核验。
4. 图中只放端点明确、金额/时间明确、无归属或连续性缺口的交易链路；同名待核、聚合对手方、
   缺失端点、线索和需外部证明的对象放在待核列表/补证事项。
5. 若没有可画交易链路，给资金断点和下一步核查动作；不要用占位箭头、零值路径或 lead arrow。
6. 真实 PNG/JPG/report figure 只有在可用 Analytix/Codex 交付面生成并检查非空、文字合规后才能声称交付；
   否则给文本 `资金流向图`、链路表和交付缺口。
7. Do not reduce a graph follow-up to a three-line summary or three-row table
   when facts support layered `来源溯源`、付款方资金形成、转入收款方、
   收款方承接/现金或产品、`赎回后分流` and 接续去向/未调取端点.

## Output Contract

图请求必须包含 `资金流向图`、结论、图范围、主链/旁路线索、下游去向、资金断点、
待补证事项和依据表。金额先写元再写万元，例如 `20,000,000.00 元（2,000.00 万元）`。
可画边表应列时间、金额、付款方、收款方、账号和绘制理由；待补证线索列在图外。
交付顺序覆盖 `结论`、`资金来源`、`主要资金链路`、`下游去向`、`资金断点`、`待补证事项`，
并附 compact table；资金断点也可内部对应 funds break。

可见答案、图例、节点/边标签、alt text、文件名和附件标题都不得含 Mermaid 源码、代码围栏、
`资金边`、`supported`、`needs_review`、`candidate`、`edge_status`、
`support layer`、`workflow`、`case_id`、`当前案件可见`、`多账户`、`完整往来`、
`历史往来` 或 `集中链路`。使用 `资金链路`、`交易链路`、`已有流水支持`、`需复核`、
`需补证`、`线索`、`未调取端点`、`资金断点`、`核验意见`。

## Completion Gate

读者必须能看清哪些箭头有交易记录、涉及哪些账户、哪些事项仍需证明。图形形状不能替代报告结论。
若只给三行摘要、裸 `账户组`，或把缺失/同名/聚合/线索端点画成确定箭头，必须重写。
有任何需复核端点、产品赎回、余额连续或最终受益人缺口时，必须给 `下一步核查建议` 或
`下一步补证建议`。
