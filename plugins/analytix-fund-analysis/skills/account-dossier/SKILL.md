---
name: account-dossier
description: "用于对指定银行卡、账户或账户标识开展完整账户资金画像：开户/登记事实、开户主体联系方式/地址线索、交易环境/IP/MAC/渠道线索、账户基本情况、资金流入流出、重点对手方、账户角色、异常特征、资金来源去向线索、证据缺口和核查建议。不是简单 Top 排名或全案分析。"
---

# 账户资金画像

账户资金画像用于在当前案件内对一张卡、一个账号或账户标识开展完整研判。用户说“分析这张卡/这个账户”时，即使问题很短，也应输出可读的经侦账户材料。

Read the shared boundary before answering:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library when suspicious features, source/destination,
typology leads, or case-material wording matter:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).

## Use when

- The user names a card number, account number, or account key and asks for
  analysis, profile, opening/account status, abnormal behavior,
  source/destination, or fund features.
- A seemingly simple account question should be expanded into a full account
  review: basic facts, inflow/outflow, counterparties, anomalies, and leads.
- The user asks to continue from a specific account already discovered in a
  prior answer.

## Not for / Do Not Use

- A person/company/holder-level review that needs all accounts under a subject.
- A one-line Top/ranking fact that does not need behavior analysis.
- Full-case analysis or formal report writing.

## Workflow

1. Resolve the account scope with `analyze_account_full`; keep verified account
   facts, investigative leads, filtered items, and missing fields separate.
2. Capture account-opening/registry facts when available: holder, id/certificate
   clue, linked contact/address/employer clues, bank/branch, account type,
   open/close/status, plus data span, transaction coverage, inflow/outflow
   totals, counts, success-status scope, and any quality warnings.
3. Add `rank_counterparties` only when top counterparties are absent, incomplete,
   or explicitly requested.
4. Use tracing or hypothesis tools only for source/destination leads, suspicious
   behavior, return-flow clues, or explicit continuation.
5. Test common account feature families for 研判/异常/画像:
   living-card normality, business-use personal card, transit/pass-through,
   convergence/collection, dispersion/payout, fast-in-fast-out, split/round
   amounts, missing counterparties, cash same-deposit/withdraw candidates,
   transaction-environment/IP/MAC/teller/branch/location clues, financial
   products, payment channels, merchant/platform/virtual-asset leads, asset
   endpoints, cross-border clues, and return-flow clues.
6. Separate verified facts from investigative leads; do not convert a top
   counterparty into a proven path without supported edges.

Default first-turn path: finish from `analyze_account_full`; add
`rank_counterparties` only for missing/detail counterparties and
`hypothesis_probe` only for missing anomaly/role leads. Do not add holder-wide,
casegraph, evidence-pack, scope, navigator, or trace tools unless requested or
a blocking gap is reported.

## Output Contract

Return a public-security economic-investigation style account profile with:

- 账户基本情况和当前案件范围;
- 登记/开户信息;
- 资金流入;
- 资金流出;
- 重点对手方;
- 账户角色;
- 异常特征;
- 资金来源或去向线索;
- 核验意见;
- 核查建议;
- account-opening/registry facts, linked holder contact/address clues, account
  type, and status when available;
- time span, transaction volume, inflow/outflow totals and counts;
- top counterparties by direction, amount, and count;
- account role clues and downgrade reason;
- behavior and anomaly features, including transaction-environment/IP/MAC/
  channel clues when available;
- source/destination or continuation leads;
- feature status, downgrade reason, and next proof for suspicious clues;
- evidence gaps, missing fields, and practical next checks.

## Completion Gate

The answer is complete only when the account has a full dossier, not just a
single number. Do not emit raw JSON, tool manuals, or report templates.
