# Analytix Workflow Context

This reference teaches the plugin how Analytix case work moves across the
desktop product. It is workflow context, not a memory store, not a case-fact
template, and not a replacement for source-backed verification.

## Current Case And Scope

- The current analytix case project workspace is the default case source. The
  case-project chain is current conversation workspace -> `.analytix/case-project.json`
  -> analytix data-analysis case -> `analytix_funds` MCP.
- Ordinary tool calls should omit explicit case identifiers when the case
  project can be resolved. If the chain is unavailable, return a current-case
  project source gap and recovery action; do not scan local folders, DuckDB
  files, history, global case state, or old outputs.
- If an explicit `case_id` is supplied, it must match the current case project
  binding. A mismatch is a blocker, not a reason to read another case.
- A source/scope envelope should be available to focused owners: case identity,
  source of truth, source scope, data-quality state, metric scope, validation
  state, delivery state, and gap note.

## Cleaned Tables And Analysis Indexes

- Ordinary facts come from cleaned transaction facts and approved analysis
  indexes, not raw source files or source-detail rows.
- Cleaned export requests should preserve Analytix cleaned-table field names,
  Chinese headers, row order, and field order unless the user explicitly asks
  for an added review sheet.
- During P0 containment, `export_cleaned_case_data` and
  `create_case_notebook` are hidden and rejected before execution. Export or
  notebook requests receive a capability boundary and create no file, job,
  staging directory, or attachment. Their write/delivery workflows remain
  disabled until the host grant, evidence, artifact, and publication gates are
  authoritative.
- For empty-counterparty review, separate cash-like positive-amount records from
  zero-amount interest or non-cash records, and record exclusion reasons.
- Custom computation belongs to the controlled current-case workbench: read
  only, cleaned `fc_*_norm` or approved `analysis_*` scope, row limited,
  purpose tagged, auditable, and not a fake production fact.

## Analysis, Graphs, Images, And Attachments

Graph and image delivery and the Attachment directory are part of the same
multi-turn continuation context: a later answer must know whether a table,
image, report figure, workbook, or appendix was generated, inspected, blocked,
or still only requested.

- Analysis indexes support rankings, dossiers, amount reconciliation, tracing,
  graph generation, evidence tables, and full-case analysis.
- Fund-flow diagrams should be generated from supported transaction edges only.
  Candidate, missing, or partial endpoints must be written as leads or breaks,
  not confirmed arrows.
- Visual evidence requests may require tables, Mermaid, PNG, JPG, workbook
  sheets, report figures, or appendix files. A visual artifact is not delivered
  until an available Analytix/Codex delivery surface has generated it and it has
  been inspected, or a blocker is stated.
- Attachments and workbooks should carry case, scope, metric, unit, time window,
  evidence status, and owner skill. Chat prose is not a substitute for a
  requested attachment.
- The plugin may guide and verify delivery, but it is not a standalone PDF
  renderer, OCR pipeline, asset-registration connector, or general evidence
  acquisition system. External materials remain proof-action requests unless
  the backend or user-provided source has supplied them.

## Reports And Multi-Turn Continuation

Use multi-turn continuation to preserve the current case project, selected subject,
analysis scope, generated artifact paths, report section status, and next
verification path across ordinary follow-up turns.

- Report work may continue an existing report, add a section, update terms,
  recompute amounts, check every displayed figure against detail rows, or
  revise tone into public-security/economic-investigation material.
- Continuing a report should preserve the existing structure and only patch the
  requested section unless the user asks for a full rewrite.
- Full-report amount checks should recompute displayed totals, trace which
  table or paragraph used each amount, and correct wording when a display basis
  could mislead.
- Multi-turn analysis should preserve the current case project, selected subject,
  hypothesis, evidence gaps, graph/table/report delivery state, and next
  verification path inside Analytix-owned runtime artifacts. Do not write
  cross-case facts into global memory.

## Domain Workflow Defaults

Focused owners should know these common Analytix case workflows:

- 清洗明细导出 and cleaned-field appendix creation.
- 空对手复核 with cash/non-cash and zero-amount separation.
- 取现口径, same-day cash bridge candidates, and cash break boundaries.
- Mermaid/PNG/JPG fund-flow and relationship graph materialization with render
  checks when an Analytix/Codex delivery surface is available; otherwise a
  bounded evidence table plus delivery gap.
- Report continuation, local section rewrite, full-report amount recomputation,
  and terminology cleanup.
- Project-fund, related-company, relatives/close contacts, asset end,
  material/labor/cost appearance, and benefit-transfer deep dives.

Historical no-plugin threads are behavior baselines only. Do not store their
subjects, amounts, dates, fixed conclusions, golden text, or evidence rows in
production references.

## Delivery Spine

Every substantive user-facing investigation answer should answer:

1. What was checked.
2. What conclusion is supported.
3. Which statements, accounts, paths, tables, or artifacts support it.
4. What is abnormal.
5. Why it matters to the case.
6. What still cannot be determined.
7. What material should be obtained next and why.

Use evidence maturity language such as 已有流水支持, 高可信支持, 线索, 需复核,
暂不能认定, and 可形成材料候选. Keep engineering terms in support/audit records.
