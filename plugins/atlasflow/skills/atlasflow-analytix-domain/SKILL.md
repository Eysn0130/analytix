---
name: atlasflow-analytix-domain
description: Use with AtlasFlow when the user needs public-security, official-document, case-project, or general business diagrams. Route domain requests into AtlasFlow architecture, workflow, sequence, dataflow, or lifecycle JSON IR without changing AtlasFlow core renderer behavior. Use for personnel relationship diagrams, case association diagrams, evidence material chains, fund-analysis graphs, association graphs, equity-piercing graphs, fund-flow diagrams, public-security procedure maps, official-document circulation maps, and general business process diagrams.
license: MIT
metadata:
  version: "0.1"
  author: tt-a1i
  requires: atlasflow
---

# AtlasFlow Analytix Domain Skill

Use this skill as a domain front door for AtlasFlow. Do not edit AtlasFlow core renderer files. Convert the user's domain task into a small AtlasFlow JSON document, then render with the `atlasflow` skill.

## Analytix Case Outputs

Use this skill for analytix case-project graph deliverables, including:

- 人员关系图
- 案件关联图
- 证据材料链图
- 资金分析图谱
- 关联图谱
- 股权穿透图谱
- 资金流向图
- 公安业务流程图
- 公文流转/收发文办理图
- 通用业务流程、数据流、生命周期和系统关系图

## Required Flow

1. Classify the request into one domain:
   - `public-security`
   - `official-doc`
   - `case`
   - `general`
2. Pick an AtlasFlow diagram type:
   - `architecture` for relationship/association/equity structures.
   - `workflow` for procedure, evidence, circulation, handling, and approval routes.
   - `dataflow` for funds, material/data movement, and lineage.
   - `lifecycle` for states, statuses, deadlines, terminal outcomes.
   - `sequence` for call chains or timed interaction traces.
3. Read the matching reference:
   - `references/public-security.md`
   - `references/official-docs.md`
   - `references/case-graphs.md`
   - `references/general.md`
4. Produce AtlasFlow JSON using only the core AtlasFlow schemas.
5. Render and validate:
   - `node ../atlasflow/bin/atlasflow.mjs render <type> <input>.json <output>.html`
   - `node ../atlasflow/bin/atlasflow.mjs validate <type> <input>.json --json`

## Safety And Wording

- Public-security and case diagrams are visual organization aids, not legal conclusions.
- Do not infer guilt, liability, or intent from graph topology alone.
- Keep personal identifiers redacted unless the user explicitly provides a synthetic or authorized dataset.
- Label uncertain relationships as `待核验`, `疑似`, or `需补证` rather than definitive findings.
- Prefer evidence-backed labels such as `交易记录支持`, `材料提及`, `同账户/同证件号匹配`, or `人工标注`.

## Domain IR Adapter

When the user provides a compact domain IR, use:

```bash
node scripts/domain-ir-to-atlasflow.mjs <input.domain.json> <output.atlasflow.json>
```

Then render the generated AtlasFlow JSON with the core AtlasFlow CLI.
