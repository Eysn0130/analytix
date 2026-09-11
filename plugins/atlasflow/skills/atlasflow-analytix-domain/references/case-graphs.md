# Case Graph Diagram Guidance

Use case diagrams for visual summaries that can be attached to reports, case briefings, and evidence packages.

Recommended mappings:

- 人员关系图: `architecture`
- 案件关联图: `architecture`
- 证据材料链图: `workflow`
- 资金分析图谱: `dataflow`
- 关联图谱: `architecture`
- 股权穿透图谱: `architecture`
- 资金流向图: `dataflow`
- 行为轨迹/时间线: `lifecycle` or `workflow`
- 疑点-证据-主体对应图: `workflow`

Case diagram quality rules:

- Use `emphasis` only for the primary path or core relationship.
- Put uncertain findings in cards or tags, not definitive edge labels.
- Prefer aggregate labels such as `12笔 / 320万元` over raw full transaction lists.
- Keep identifiers redacted unless the current workspace is authorized for full identifiers.
- Add a card for `补证建议` when a critical relation lacks direct evidence.

