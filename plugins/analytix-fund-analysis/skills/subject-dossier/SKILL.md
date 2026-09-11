---
name: subject-dossier
description: "用于对人员、户名、公司或组织开展完整主体资金画像：身份与开户事实、联系方式/地址/单位/设备/IP/MAC/渠道关联、账户集合、登记账户与待补证账户线索、账户角色、资金流入流出、重点对手方、异常特征、证据缺口和核查建议。不是单账户画像或正式报告写作。"
---

# 主体资金画像

主体资金画像用于对人员、户名、公司、组织或点名主体开展系统资金研判；“分析某人资金”“某公司资金画像”“名下账户研判”默认扩展为可读经侦材料。

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for full coverage, source/destination, typology leads, and case-material wording.

Desktop/source boundary: 普通案件用户默认不看进度；如必须发工具前消息，只能逐字写 `正在核验当前案件事实。`
不要公告 workflow、skill、SQL、DuckDB、本地路径或文件搜索。当前案件主体画像默认以
`analyze_holder_full` 为完整 fact pack；该包已有账户范围、资金进出、Top 账户和
重点对手方时，直接成稿，不再追加排行、SQL、Python、本地 DuckDB、旧报告或 navigator。
Do not use local DuckDB for ordinary current-case-project subject profiles.

## Use when

- 用户点名人员、公司、户名、组织、嫌疑人、被害人或相关主体。
- 问题需要登记账户、待补证账户线索、开户/身份/联系方式/地址/单位/设备/IP/MAC/渠道关联、
  账户角色、资金流入流出、重点对手方、异常特征和补证建议。
- 用户询问某主体是否控制、使用或关联当前案件账户。

## Not for / Do Not Use

- 单账户画像，交给 `account-dossier`。
- 低风险 Top/ranking 或轻量事实，交给 `quick-fact`。
- 特定双方资金往来核验，交给 `pair-amount-investigation`。
- 全案研判或正式报告，交给 `full-case-analysis` / `report-builder`。

## Workflow

1. 首选 `analyze_holder_full`，保留登记账户、待补证账户线索、开户/登记事实、
   联系方式/地址/单位/设备/IP/MAC/渠道关联、账户统计、Top 账户、重点对手方和身份缺口。
2. 若用户问与某名对象的资金关系，用 `investigate_pair_amount` 固定方向金额；未指定方向时，
   对两个方向各核验一次。双方资金往来事实不能被排行覆盖。
3. 只有 holder fact pack 缺少可见答案必需的 Top 表，或用户指定不同排序指标时，才各追加一次
   `rank_accounts` 或 `rank_counterparties`。
4. 只有用户要求可疑特征、否定搜索、现金/金融产品、设备/IP/MAC/渠道、团伙关联、
   资产项目或税票合同线索时，才追加 `hypothesis_probe`。
5. 语义事实不足时写清主体画像证据缺口和补证动作；只有用户要求自定义/可回放范围或挑战结果时，
   才转 `case-workbench`。不要用本地文件、旧材料或目录搜索补当前案件事实。

## Output Contract

答案用公安经侦材料口吻，结论先行，至少覆盖：

- `账户基本情况`、`登记账户清单`、`重点账户表`;
- `资金流入`、`资金流出`、`重点对手方`;
- `异常特征`、`可疑用途或去向`、`核验意见`、`核查建议`;
- 登记账户和待补证账户线索必须分开；待补证线索写 `需补开户资料/流水/回单` 或
  `不能直接认定为名下账户`；
- 写具体账号、期间、金额、对手方和限制，不写 `资金边`、`工具返回`、`本轮命中`、
  `按...口径`、`多账户`、`多个账户`、`完整往来` 或 `历史往来`。

若 Top 对手方名称、账号或金额在一次必要排行后仍未固定，写 `当前材料尚未固定重点对手方明细`
并列入 `核验意见` / `下一步核查建议`，不要重复同一排行意图。不要建议“调取完整流水”
来替代已导入流水内复核；应区分已调取流水内导出/核对和外部开户、回单、余额连续、
产品/KYC/订单、用途或关系证明材料。

## Completion Gate

完整主体画像必须覆盖账户集合和资金特征，而不只是第一个匹配账户；必须出现可见
`下一步核查/补证建议`，并至少点名三类具体动作：当前案件复核/导出、开户/控制材料、
回单/余额连续、对手方关系、产品/订单/KYC 或用途材料。不得暴露内部路由、工具名或固定报告门。
