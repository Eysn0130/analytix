---
name: visual-evidence
description: 用于当前案件证据表、Top 表、统计图、资金流向图、附件清单、证据包和可视化材料交付。适合用户要求“做表/图表/附件/证据包/Top20/导出材料”时使用；图表标题、图例、附件名称使用公安经侦业务语言，标明已有流水支持、需补证、线索、未调取端点和资金断点。
---

# 证据图表与附件

Use when reviewed current-case-project facts need a reader-facing table, chart,
dashboard, appendix workbook, Mermaid, PNG, JPG, report figure, or attachment.
This skill owns the visual/table delivery contract and QA. It does not create
new case facts or implement a standalone export, render, OCR, or asset-registry
system.

Read [focused-skill-shared](../analytix-fund-analysis/references/focused-skill-shared.md).
Use [economic-investigation-analysis](../analytix-fund-analysis/references/economic-investigation-analysis.md)
for delivery packs, [analytix-workflow-context](../analytix-fund-analysis/references/analytix-workflow-context.md)
for exports/images/attachments, and [report-schema](../analytix-fund-analysis/references/report-schema.md)
when the visual enters formal material. Read
[public-security-official-writing](../analytix-fund-analysis/references/public-security-official-writing.md)
when a table, Top list, flow table, figure, or appendix may be copied into case materials. Read
[database-site-diagnostics](../analytix-fund-analysis/references/database-site-diagnostics.md)
only when source preflight, schema/profile, SQL explain, bounded preview, or query-history provenance is required.

## Use when

- 用户要求表格、图表、看板、附件、证据包、Top20 表、特征表、流向表、续调表、
  Excel/CSV handoff, appendix workbook, chart/table QA, PNG/JPG 或报告插图。
- 用户要求把已复核交易边渲染成关系图、资金流向图、资金穿透图或 Mermaid。
- dossier、trace、lab、full-case、report 或 evidence-request 需要 reader-facing visual evidence。

## Not for / Do Not Use

- 从零证明交易事实、下一跳或控制关系。
- 无可视交付请求时运行全案、报告写作、claim review 或纯图谱推理。
- 让排名、矩阵或 chart shape 暗示交易路径、控制关系或法律结论。

## Workflow

1. Identify the surface: table, chart, dashboard, workbook, flow figure, rendered image, or visual QA.
2. Write the visual contract: question, reader action, source of truth, scope, metric/unit/window/direction,
   核验意见, and sparse-data fallback. Custom SQL/notebook/disputed metrics need source diagnostics first.
3. Fresh delivery packs use `get_case_scope_map` once, then `get_evidence_pack`, then at most one targeted
   rank/trace/list tool for the missing table family. Do not call full-case, dossier, lab, or report tools
   merely to assemble visuals.
4. Choose the simplest form: exact facts -> table; concentration -> leaderboard/bar plus exact table;
   time movement -> timeline only with enough points; cross-tabs -> matrix/heatmap; supported chain ->
   flow table plus `graph-visualization`.
5. Label rows as 已证实事实、统计特征、线索候选、需补证, or 来源未就绪. Fund-flow exports use
   资金流向图、资金穿透图、关系图、资金链路依据表、核验意见, or 补证建议.
6. Do Data Analytics-style QA: chart form matches the investigative question, units/direction are visible,
   labels fit, sparse charts are downgraded, and the visual does not imply legal/control conclusions.
7. For report-grade tables, Top lists, flow tables, continuation lists, and appendix inventories, write a
   table-after-analysis paragraph: abnormal feature, proof value, evidence limit, and next proof action.
8. For requested artifacts, use the available Analytix/Codex delivery surface
   to create or refresh the table/chart/Mermaid/PNG/JPG/workbook/report figure
   only when that surface is available, then inspect the delivered surface:
   files readable, labels fit, images nonblank. If no governed delivery surface
   exists, deliver an evidence table plus a clear artifact delivery gap instead
   of claiming a generated file.

<!-- release-contract: table, chart; Mermaid, PNG, JPG; available Analytix/Codex delivery surface or explicit blocker; inspect the delivered surface; images are nonblank; read/render inspection; no standalone OCR/export/render system; chat summary cannot claim file delivery. -->

## Output Contract

Return the selected surface, 本次依据、统计范围、单位、时间范围、核验意见,
fact inventory, missing facts, delivered file or explicit blocker, QA result,
downgrade reason where needed, and next owner when a fact, path, request list,
or report conclusion still needs validation. Report-grade tables must include
表后研判意见 rather than ending at rows, rankings, or attachment inventories.

Image text is user-facing text. PNG/JPG/report figures, thumbnails, alt text,
attachment titles, legends, nodes, and edge labels must not show `supported`,
`needs_review`, `candidate`, `edge_status`, `support layer`, `delivery_state`,
`workflow`, `case_id`, or other implementation state. Use `已有流水支持`,
`需补证`, `线索`, `未调取端点`, `资金断点`, or `核验意见`. Regenerate noncompliant
artifacts or deliver a compliant evidence table plus image delivery gap.

## Completion Gate

Visual Evidence is complete only when the user can inspect the evidence without
confusing a ranking, aggregation, lead link, or chart shape for a supported
transaction path or legal conclusion. Concrete chart, dashboard, workbook,
appendix, PNG/JPG, or report figure requests require a produced artifact plus
inspection when the delivery surface exists, or a clear blocked delivery gap.
A chat summary cannot claim file delivery.
