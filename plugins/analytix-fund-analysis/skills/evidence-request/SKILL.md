---
name: evidence-request
description: 用于把当前资金研判中的证据缺口整理成补调取证、协查、续查和核验清单，包括银行/支付/平台流水、账户明细、对手方身份、IP/MAC/设备日志、商户订单、发票合同、资产登记、税务材料和下一步追查路径。只形成取证建议，不证明新事实或撰写报告。
---

# 补充取证清单

本能力把当前资金研判事实、证据缺口和待补证线索整理成可执行的补调取证/调证清单。
它服务办案补证和续查安排，说明每项材料拟证明什么，不生成新的交易事实。

Read the shared boundary before answering:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library for delivery packs, evidence gaps, and typology
boundaries:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).
Read [public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
for 补调取证清单的公文式表达、证明价值、证据成熟度和法律敏感降级.

## Use when

- The user asks for 补调清单, 续调清单, 取证清单, 下一步追查清单,
  evidence pack, subpoena priorities, or next verification actions.
- A dossier, lab pass, trace, full-case analysis, critique, or report review
  found 待补证线索 that need external proof.
- The missing proof involves bank/payment/platform records, account identity,
  IP/MAC/device logs, merchant/order records, invoices/contracts, asset
  registration, tax documents, feedback failures, or continuation paths.

## Not for / Do Not Use

- Producing new transaction facts from scratch.
- Deciding legal liability, guilt, ownership, tax crime, laundering, or gang
  status.
- Full-case analysis, report writing, or report-conclusion review.

## Workflow

1. Identify the finding or gap that needs proof.
2. Group requests by target: bank, payment institution, platform/merchant,
   telecom/device/IP/MAC, tax/invoice/contract, asset registry, company/project,
   person/counterparty, or internal data repair.
3. Use `generate_followup_investigation_list` when available; add targeted
   trace, risk, evidence-pack, or continuation-list validation only when needed.
4. Prioritize by amount, risk, missing proof value, feasibility, and whether the
   request can upgrade a lead into 已核事实.
5. State what each request is expected to prove and what it cannot prove alone.

## Output Contract

Return an Evidence Request List with:

- target institution/person/system;
- request item and field/time scope;
- linked lead or gap;
- expected proof value;
- what the material is expected to prove / `证明`;
- evidence boundary and forbidden upgrade before the material is obtained;
- priority (`P1`/`P2`/`P3` or high/medium/low) and reason;
- table-after-analysis note when the list is attached to report-grade material:
  explain proof value, current evidence limit, and which material should be obtained next;
- next action owner when known.

## Completion Gate

The list is complete only when the user can act on it without guessing what to
request, why it matters, and which conclusion still lacks proof.
