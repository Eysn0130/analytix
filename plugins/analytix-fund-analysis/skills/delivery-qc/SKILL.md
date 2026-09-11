---
name: delivery-qc
description: 用于涉案资金研判最终输出复核，检查普通问答、金额核验、主体画像、资金流向图、证据表、附件和报告材料是否具备结论、依据、异常特征、案件意义、核验意见和补证动作。阻断事实正确但研判浅、表格缺失、口吻机械、工程词外泄或误导办案的低质答案。
---

# 研判材料交付复核

This workflow checks Analytix case-facing answers, reports, graphs, tables, and appendix packs.
It does not create new facts; it checks whether the owner turned facts into usable material.

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md),
[anti-patterns](../analytix-fund-analysis/references/anti-patterns.md), and
[economic-investigation-language](../analytix-fund-analysis/references/economic-investigation-language.md)
when judging detailed wording risks. Read
[public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
when judging reports, formal materials, claim review, evidence requests, tables, and appendix text.
Use [investigation-answer-contract](../analytix-fund-analysis/references/investigation-answer-contract.md)
as the internal schema-first spine for final answers and report sections: conclusion,
verified facts, supported fund-flow paths, abnormal features, evidence boundaries, and proof actions.

## Use when

- A substantive answer, amount review, dossier, graph, evidence table, full-case result,
  report paragraph, image, workbook, or appendix pack is about to be shown to a case user.
- The user asks whether an answer is professional, complete, usable as material, or too mechanical.
- A support layer produced facts but the final answer still looks like a tool card, audit checklist,
  implementation note, or generic template.

## Not for

- Generating new facts, running SQL, tracing funds, drawing graphs, or writing reports by itself.
- Replacing the lead owner for pair amounts, dossiers, full-case analysis, visual evidence,
  report building, claim review, or workbench calculations.

## Workflow

Check the output against this investigative spine:

1. 查什么: object, scope, time window, amount/count basis, and source boundary.
2. 结论是什么: direct result with evidence maturity.
3. 依据哪些流水/账户/链路: accounts, transactions, graph edges, or evidence tables in办案语言.
4. 异常在哪里: concentration, repetition, missing fields, cash/product/platform clues,
   same-fact risk, purpose ambiguity, or account-role features.
5. 案件意义是什么: why the fact matters for source, destination, role, relationship,
   benefit-transfer, concealment, or follow-up proof.
6. 还不能认定什么: ownership, control, purpose, final flow, legal conclusion, cash/asset
   conversion, or cross-case inference gaps.
7. 下一步调什么材料: institution, subject/account/time scope, record type, and expected proof value.

These checks are substance checks, not mandatory headings; compact answers still must be understandable without internal tool context.
When the output is report-grade or fact-heavy, treat missing `FinalInvestigationAnswer`
elements as validation failures in the Instructor sense: fix structure or return to
MCP/DuckDB for facts; never ask the model to invent a missing amount, transaction edge,
or evidence reference.

## Blocking Patterns

Block or rewrite when the output:

- gives only figures, rankings, or card summaries with no investigative judgment;
- lacks a table for two-party amount, Top list, graph basis, report amount, or requested evidence pack;
- omits abnormal features, case meaning, evidence boundary, or current-case facts already returned;
- exposes tool names, `case_id`, workflow/support terms, JSON/debug, local paths, query/table/executor wording,
  English runtime states, Mermaid source, or image labels such as `supported`, `candidate`, `edge_status`;
- writes `账户组`、`收款端`、`多账户`、`多个账户`、`完整往来`、`历史往来`、
  `其余 N 笔`、`核验口径`、`金额口径`、`证据状态`、`同事实去重`、`本轮命中`;
- treats already visible current-case transaction facts as missing, omits 已见承接/分流,
  truncates 20 笔以内 complete returned row-level transfer tables, or folds rows into
  `其余 X 笔` / `小额转账`;
- relies on interchangeable boilerplate about 主要集中转账, uses headings as the main answer,
  or gives more than four top-level headings for a natural pair amount question; that is a
  strong rewrite trigger unless the user asked for a formal report;
- uses AI-style openers/closers such as `结论先说`、`直接说结论`、`值得注意的是`、
  `不难发现`、`综上所述`、`总的来说`、`总体来看`、`归根结底`, or pads the answer with
  `不仅是...更是...`、`首先/其次/最后`、`具有重要意义`、`有力支撑`、`奠定基础`、
  `赋能`、`形成闭环` instead of concrete case meaning;
- tries to "humanize" official case material with casual chat, internet slang, jokes,
  rhetorical questions, first-person performance, or unsourced expert/industry authority;
- uses meta-commentary or customer-service tails such as `下面我会`、`让我们`、`接下来`、
  `希望这对你有帮助`、`如需我可以继续`, or self-media/consulting language such as
  `深刻揭示`、`底层逻辑`、`核心密码`、`关键抓手`、`保驾护航`、`降本增效`、`提质增效`、
  `全链路闭环`、`体系化推进`;
- uses public-document empty phrases such as `高度重视`、`压实责任`、`形成合力`、`纵深推进`、
  `取得实效`、`坚实基础`、`强力支撑` instead of evidence-specific proof actions;
- writes legal-sensitive conclusions such as `违法所得`、`赃款`、`非法所得`、`洗钱事实成立`、
  `虚开事实成立`、`实际控制`、`代持`、`最终归属`、`犯罪团伙`、`共同犯罪`、`坐实`、
  `锁定` from bank-flow facts alone;
- puts a table, Top list, evidence attachment, or flow table into report-grade material without a
  table-after-analysis paragraph explaining abnormality, proof value, evidence limit, and next proof;
- makes next steps more than a short proof-value tail; next steps are not a substitute for the current-case研判;
  answers like a work plan instead of interpreting current case facts; gives the correct total
  without transfer-relationship analysis, 集中交易以外逐笔往来, account spread, or row cues;
  ignores investigative_row_reading / low-value-row warning support; stays at a basic accounting level;
  forgets that current-case detail aggregate controls the answer over rank/focus/old-report amounts;
  drops early, low-value, terminal, batch, repayment, remittance rows; uses residual buckets such as
  `全期间扣除该集中链路后` or `零散历史往来`; or asks for generic 后续出账明细 after continuation facts
  already returned instead of explaining how to prove承接关系;
- treats a line as confirmed when facts only support `线索`、`需复核`、`当前证据不足` or `暂不能认定`;
- tells the user to补调 already imported/current-case facts instead of first interpreting them;
- claims a report, chart, workbook, notebook, PNG/JPG, or appendix exists before an available Analytix/Codex delivery surface has generated it and inspection has passed.

Use concrete replacements: `相关账户（列明账号）`、`收款账户`、`多张登记账户`、
`多张付款/收款账户`、`长期转账关系`、`集中交易以外逐笔往来`、`统计范围`、
`核验意见`、`重复风险复核`、`已有流水支持`、`资金链路`、`需补证`、`暂不能认定`。

<!-- release-contract: block 核验口径, 全期间同名收款人, 重点收款账户核验, implementation vocabulary; rewrite if output omits abnormal features or case significance; evidence maturity terms include 高可信支持 and 可形成材料候选. -->

## Output Contract

Do not expose this QC workflow to ordinary case users. For developers, name the missing delivery element, owner skill, and minimum rewrite.

## Completion Gate

The gate passes only when the output is answer-first, source-backed, useful, free of engineering vocabulary, and clear about support, leads, and proof actions.
