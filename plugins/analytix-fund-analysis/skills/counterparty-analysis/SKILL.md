---
name: counterparty-analysis
description: 用于研判重点对手方、共同对手方、关联资金通道、支付机构/商户/平台线索、缺失对手方、疑似账户型对手方、共享通道线索和补证优先级。适合对手方结构、通道特征和调证对象分析；不用于完整资金穿透或正式报告生成。
---

# 对手方分析

Counterparty Analysis explains who received from or paid into an account,
subject, or case segment. It ranks real counterparties, highlights shared
channels, and keeps missing-name or account-only clues out of the fact column.

Read the shared boundary before answering:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Read the domain task library for payment-channel, missing counterparty,
public-to-private, project/company channel, and subpoena-priority wording:
[economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md).

## Use when

- The user asks about counterparties, common counterparties, associated fund
  channels, top payers/payees, shared accounts, payment institutions, merchants,
  or platform-like counterparties.
- Opponent names are missing, account-only, business-like, or need subpoena
  prioritization.
- The user wants to understand channels without asking for a full multi-hop path.

## Not for / Do Not Use

- Full path tracing, next-hop continuation, or fund-flow path proof.
- A complete account/subject dossier unless counterparties are the only missing
  lane.
- Report generation.

## Workflow

1. Determine object, direction, scope, rank metric, time window, and whether the
   question is deterministic, common/shared, or missing-name oriented.
2. Use `rank_counterparties` once for deterministic counterparties, common
   counterparty questions, amount challenges, and scope recompute prompts.
   Start with the broadest holder/account scope requested by the user and
   answer from returned scope, coverage, warnings, and rows; do not add date or
   card splits unless the user explicitly asks a new follow-up.
3. Use `trace_subject_top_outflows` for destination leads from known subjects
   when the user asks where funds may have gone.
4. Use `classify_missing_counterparty_business` when opponent names are blank,
   account-only, or business-like.
5. Label counterparty roles as deterministic counterparty, shared channel,
   account-like/missing-name lead, payment institution, merchant/platform lead,
   related-subject lead needing proof, or subpoena target.
6. Hand off to `fund-tracing` when the user asks to prove a path or continue
   one more hop.

## Output Contract

Separate:

- deterministic counterparties with amount/count/direction;
- shared or common channels;
- missing-name/account-only/business-like leads;
- public-to-private, payment-channel, project/company, or related-subject leads
  when current data supports them as leads;
- merchant/platform/virtual-asset-like leads and required enrichment when
  current data supports them as leads;
- subpoena or data-enrichment priorities;
- conclusions that lack proof and should not be asserted.

## Completion Gate

Do not present a ranked counterparty as a proven downstream path. Stop once the
counterparty landscape and evidence limits are clear.
