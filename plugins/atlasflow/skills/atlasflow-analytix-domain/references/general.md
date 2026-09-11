# General Diagram Guidance

Use AtlasFlow core behavior for non-domain requests:

- System architecture: `architecture`
- Business workflow: `workflow`
- API/request trace: `sequence`
- Data lineage or pipeline: `dataflow`
- State/status model: `lifecycle`

When a request includes both general and case/public-security terms, choose the domain-specific route first and keep the final AtlasFlow JSON schema-compatible.

