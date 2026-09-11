---
name: investigation-lab
description: 用于开放式异常资金线索研判和假设核验，包括异常交易模式、账户角色、身份/联系方式/住址/设备/IP/MAC 关联、团伙关联、生活卡特征、中转/汇聚账户、现金断点、支付通道/商户/平台/虚拟资产线索、理财证券基金、资产项目诉讼税务合同跨境线索、对公转个人两层统计、否定性搜索和下一步研判队列。不用于简单 Top 事实或正式报告写作。
---

# 异常资金线索研判

本能力用于开放式异常资金特征和线索研判：围绕当前案件提出并核验假设，同时说明每条线索的证据强度。

Read the shared boundary before answering:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for feature families, typology lenses, negative search, and delivery-pack wording.

## Use when

- The user asks for suspicious features, abnormal patterns, account roles,
  identity/contact/address/device/IP/MAC correlation, group-association leads,
  living-card checks, transit/convergence accounts, negative search,
  payment-channel/merchant/platform/virtual-asset clues, overseas/cross-border
  clues, phone/address/employer/contact leads, branch/teller/device/IP/MAC
  clues, shared counterparties, gang/common-control leads,
  asset/project/litigation/tax-contract/cross-border clues, public-to-private
  two-layer statistics, cost normality review, cash breaks, financial-product
  leads, or "keep digging".
- A full dossier found possible leads and the user wants deeper exploration.
- The task is exploratory, but still bounded to the current case project and available
  facts.

## Not for

- Simple Top facts or one-number lookups.
- Formal report writing or report QA.
- Turning weak clues into conclusions.

## Workflow

1. Translate the user's question into a small hypothesis queue with explicit
   search fields and expected evidence.
2. Use `hypothesis_probe` for suspicious features, role classification,
   identity/contact/address/device/IP/MAC correlation, group-association leads,
   negative search, asset/project leads, litigation leads, tax/contract clues,
   payment-channel/merchant/platform/virtual-asset clues, cross-border clues,
   public-to-private two-layer statistics, cost normality review, cash breaks,
   and financial-product clues.
3. Cover likely feature families before closing an "异常特征跑一遍" request:
   frequency/time, amount concentration, fast-in-fast-out, living-card normality,
   transit/convergence, return flow, cash same-deposit/withdraw, payment channel,
   merchant/platform/virtual-asset lead, financial product, securities/fund/
   insurance product, house/vehicle/high-value consumption endpoint,
   overseas/cross-border clues, phone/address/employer/contact overlap,
   outlet/teller/device/IP/MAC clues, common counterparties, gang/common-control
   leads, ticket/tax/contract clues, project funds, main-material/labor/cost
   appearance and benefit-transfer leads, public-to-private,
   total-vs-classified wages/reimbursement, cost normality,
   litigation/enforcement, coercive measures, group association, and
   identity/contact/address/device/IP/MAC correlation.
4. Add at most one targeted rank, trace, or quality tool when a high-value lead
   needs immediate confirmation.
5. Label each result as 已核事实、统计特征、线索候选、未发现、证据不足,
   or 需补充数据.
6. End with a prioritized next-action queue instead of a generic report gate.

## Output Contract

Return:

- tested hypotheses and evidence strength;
- feature family, supporting facts, typology lens, and downgrade reason when
  applicable;
- account role leads, normality downgrades, and forbidden upgrades;
- identity/contact/address/device/IP/MAC correlations, group-association leads,
  and forbidden control/ownership/operator/gang upgrades;
- public-to-private total/identifiable/unclassified splits when relevant;
- payment-channel, merchant/platform, virtual-asset, tax/contract, and
  cross-border leads when relevant;
- cash, empty-counterparty, living-card/transit/convergence, financial-product,
  asset-consumption, overseas, phone/address/employer, device/IP/MAC/outlet/
  teller, common-counterparty, gang/common-control, tax/contract, project-fund,
  material/labor/cost appearance, and benefit-transfer leads when relevant;
- verified facts separated from leads needing proof;
- negative findings and conclusions that lack proof;
- data gaps or enrichment needs;
- prioritized next investigative actions.

## Completion Gate

Do not over-close exploratory work. Stop when the current lead set is triaged
and the best next checks are explicit.
