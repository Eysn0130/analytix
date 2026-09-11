---
name: fund-tracing
description: 用于当前案件资金来源、去向、下一跳、现金承接、理财/证券/基金去向和资金穿透图。输出主链、旁路线索、未调取端点、资金断点、核验意见和补证建议，不暴露图谱内部状态。
---

# 资金追踪

本能力沿当前案件已核交易链路追踪资金来源、去向和断点，同时确保每条路径结论可复核。

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for upstream source, downstream destination, return-flow, cash bridge,
asset/financial-product/payment/cross-border stops, and continuation wording.

Desktop/source boundary: 普通案件用户默认不看进度；如必须发工具前消息，只能逐字写 `正在核验当前案件事实。`
不要写 `我先`、`我会`、`先`、`按...口径`、tool/skill 名、SQL、DuckDB、
本地路径或文件搜索计划。普通追踪事实来自 `trace_subject_top_outflows`、
`build_fund_flow_graph`、`trace_fund_next_hop`、`trace_fund`，必要时用
`investigate_pair_amount` 固定起始边；不得用 shell、本地 DuckDB、旧报告或缓存补当前案件事实。
Do not use local DuckDB for ordinary tracing facts.

## Use when

- 用户问资金来源、去向、回流、前手、后手、终点、下一跳、继续追一层或资金断点。
- 用户问现金同存同取、资产端点、理财/证券/基金、支付/商户/平台/虚拟资产或跨境线索。
- 已有追踪起点、主体、账户、交易、金额或时间窗口。
- 用户要求 `来源去向图`、`资金流向图`、`资金穿透图` 或 `导出图`。

## Not for / Do Not Use

- 简单 Top/ranking、完整账户/主体画像或正式报告。
- 证据不足的叙事箭头、图形装饰或把线索画成确定事实。

## Workflow

1. 内部界定起点、方向、深度、时间窗口、金额容差和可证实资金链路标准。
2. 普通 `继续追下游/向下追一层/后续出账去向`：若起始两方关系未固定，可先做一次
   start-edge check；随后用一次 `trace_subject_top_outflows`。返回出账金额、笔数、
   主要去向或终止类别后停止工具链并成稿。
3. `画资金流向图` 或资金穿透图请求用 `build_fund_flow_graph`；有已知 seed 时用
   `trace_fund_next_hop`，明确多跳且 seed 充分时用 `trace_fund`。
4. 如果图谱调用缺起点或临时失败，而主体/对手方上下文明确，可用一次
   `trace_subject_top_outflows` 补足普通去向；仍不足时写资金断点和所需 seed。
5. 现金、资产、金融产品、支付通道、平台、虚拟资产或跨境端点只能写成线索和证明缺口，
   不写最终受益人、所有权、犯罪性质或法律控制结论。
6. 只有用户明确要求自定义/可回放范围、挑战路径金额或语义事实冲突时，才转
   `case-workbench`；它不是普通追踪的第一来源。
7. Default graph depth for pair-to-downstream follow-ups is 来源形成 ->
   付款主体账户 -> 收款主体账户 -> 现金/理财/证券/基金/支付产品 ->
   赎回或分流 -> 接续去向/未调取端点.
8. Never write tool-window wording such as `本窗口` or `核验窗口`.
   If current-case data already contains downstream outflows, write them first.
   Do not move already visible transactions into `下一步取证`.
   Keep `下一步` short and subordinate to current-case analysis.

## Output Contract

普通追踪答案结论先行，保留事实包返回的 Top destination 金额，先写元再写万元，例如
`20,000,000.00 元（2,000.00 万元）`。下游答案应包含 `结论`、`下游去向`、
`资金断点`、`暂不能认定`、`下一步核查建议`，并在结论或表格引言里写明
`资金链路` 或 `接续链路`。

图谱类答案必须出现 `资金流向图`，并覆盖主链、下游去向、旁路线索、未调取端点/资金断点、
现金/理财/证券/基金去向、核验意见和补证建议。不要展示 Mermaid 源码、代码围栏、
`资金边`、`supported`、`candidate`、`terminal_category`、`当前案件可见`、
`多账户`、`完整往来`、`历史往来` 或 `集中链路`；使用 `已调取流水显示`、
`资金链路`、`交易链路`、`需复核`、`需补证`、`未调取端点`。

## Completion Gate

不得画证据不足的资金箭头或暗示资金守恒。完成答案必须让办案人看清已见承接/分流、
断点、暂不能认定事项和下一步核查建议；下一步只能证明或延伸已列事实，不能替代当前案件分析。
