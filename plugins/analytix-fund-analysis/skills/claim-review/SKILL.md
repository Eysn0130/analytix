---
name: claim-review
description: 用于复核报告、材料、图表或既有结论中的研判事实、金额统计范围、资金路径、主体关系、法律敏感表述和附件措辞。适合用户要求“这份报告哪些结论能写/哪些要降级/金额或路径是否支持”时使用；输出已有数据支持、需补证或暂不能认定，并给出纠正表述。
---

# 报告事实结论复核

Use when the user asks whether an existing report conclusion, amount, path,
attachment, graph edge, or legal-sensitive wording is correct. Claim Review is a
quality gate; it does not generate new investigation facts.

Read shared boundaries:
[focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for downgrade language, amount challenges, and safe case-material wording.
Use [public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
for 公安公文式纠正表述、证据成熟度 and legal-sensitive downgrade language.
Use [investigation-answer-contract](../analytix-fund-analysis/references/investigation-answer-contract.md)
when the reviewed report or final answer has an internal `FinalInvestigationAnswer`
spine; pass it to `validate_report_claims` as `final_investigation_answer` so
missing evidence refs, unsupported flow edges, legal upgrades, visible engineering
leaks, missing evidence boundaries, and missing proof actions become review
failures instead of prose-only advice.
Use [database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
when a disputed conclusion requires governed schema/profile, plan, failure, or
bounded sample checks before a claim-scoped calculation.

## Use when

- The user provides or names a report conclusion, number, path, attachment, graph
  edge, or paragraph and asks whether it can be used.
- A report/graph needs 核验通过、当前证据不足、降级为线索, or 需补证 judgment.
- The task is to correct overclaimed language into fact-bound wording.

## Not for

- Generating new facts, full-case analysis, dossiers, or open-ended digging.
- Proving a path from scratch when no report conclusion exists yet.

## Workflow

1. Extract reviewed conclusions and classify them as amount, count, path,
   identity, ownership/control, inference, legal-sensitive wording, or attachment.
2. When report text, disputed conclusions, or an internal `FinalInvestigationAnswer`
   structure are provided, run `validate_report_claims` first and include
   `final_investigation_answer` when available. Do not run full-case analysis
   first unless the user asks to regenerate a whole-case source snapshot.
3. If validation support already has corrected amounts, evidence-insufficient
   flows, source-boundary notes, or a same-turn stop, use it as evidence and
   write the final review as the claim-review owner. Do not copy the support
   title or chase the same conclusion with rankings, hypotheses, or full-case tools.
4. If same-turn full-case facts already exist, treat them as context and still
   run `validate_report_claims` once before report-grade wording.
5. Use database-site diagnostics before custom SQL when the reviewed conclusion
   still has a field, table, empty-result, performance, or source-conflict
   question. `profile_case_schema`, `explain_case_sql`, and
   `diagnose_case_sql` can support the review path; `preview_case_rows` is only
   a privacy-projected sample and cannot support totals.
6. Use one bounded `run_case_sql` only when a named amount/path conclusion
   remains 需补证 after diagnostics and names a concrete counterparty, holder
   pair, time window, or duplicate guard. Keep it conclusion-scoped,
   cleaned/analysis-scope, read-only, aggregate-only, and row-limited.
7. Use `validate_continuation_list` for attachment, continuation, or fund-flow
   diagram edge lists; use quality/tracing only for a specific missing proof.
8. Return corrected wording that preserves evidence strength and removes
   unsupported certainty.

## Output Contract

For report review, lead with a concise review conclusion, then use this
user-facing review table:

| 研判结论 | 核验状态 | 核验意见 | 纠正表述/数值 | 补证动作 |
|---|---|---|---|---|
| 待复核金额/路径/表述 | 核验通过/当前证据不足/降级为线索/需补证 | 本次依据、清洗范围、路径断点 | 安全表述或纠正值 | 一项具体补证动作 |

Use these labels when applicable: 已复核事实、需纠正、未获支持的研判结论、
未支持/不能确认资金流、降级/线索、本次依据与统计范围、正式报告暂不出具、复核动作、禁用表述.

For each conclusion include reviewed scope, corrected value or safe wording,
transaction/count denominator when relevant, path/fund-flow diagram status, and
why a submitted value/path/ownership/legal conclusion is unsupported or must be
downgraded. Only transaction edges backed by current-case data can be drawn as
deterministic fund flow; aggregate fragments and lead edges stay as leads.
Forbidden phrases such as 违法所得、实际控制、代持、最终归属 need safe replacement
language unless supported outside bank-flow data.
Bank-flow-only review must downgrade 违法所得、赃款、非法所得、洗钱事实成立、虚开事实成立、
实际控制、代持、最终归属、犯罪团伙、共同犯罪、坐实、锁定 and similar certainty language
unless separate legal, identity, control, platform, tax, contract, invoice, device, testimony, or asset
records are reviewed in the submitted conclusion.

## Completion Gate

Do not add new conclusions beyond the reviewed report conclusions. The answer is
complete only when every submitted conclusion has 核验状态, 当前不能认定原因,
corrected or downgraded wording, evidence-insufficient flow/fund-flow diagram
boundary where needed, and next proof action. If the review conclusion or
per-conclusion table is missing, rewrite before responding. Never expose
validator card titles, internal states, or support-only drafts as the final
review.
