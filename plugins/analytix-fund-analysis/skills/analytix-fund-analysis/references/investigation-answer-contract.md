# Investigation Answer Contract

This reference adapts the schema-first validation ideas from
`567-labs/instructor` to Analytix case-facing answers and reports. It is not a
model scorer and not a new final-answer tool. It is an internal structure that
focused skills use before turning verified facts into公安经侦 language.

## Core Principle

The final visible answer may be natural language, a report paragraph, a table
caption, or a report section. Before it is shown, the supporting material should
fit a `FinalInvestigationAnswer` structure:

1. `investigation_objective`: what the user is trying to determine, the object,
   scope, time window, and source boundary.
2. `conclusion`: the answer-first judgment and evidence maturity.
3. `verified_facts`: facts backed by MCP/DuckDB evidence references, source
   hashes, evidence ids, artifact ids, or other replayable source anchors.
4. `fund_flow_paths`: only supported transaction paths and edges. Candidate,
   missing, or incomplete edges belong in `evidence_boundaries`.
5. `risk_patterns`: abnormal features tied to verified facts or source refs.
6. `evidence_boundaries`: what current data cannot prove.
7. `next_proof_actions`: targeted records to obtain, the linked evidence gap,
   time/field scope, proof value, priority, and forbidden upgrade boundary for
   each action.

## Validation Rules

- A conclusion without `evidence_maturity` fails validation.
- A verified fact without `evidence_refs` or `source_refs` fails validation.
- A fund-flow edge with `candidate`, `partial`, `missing`, `needs_review`, or
  similar status fails validation if it appears in a supported path.
- A risk pattern without `related_fact_ids` or source refs fails validation.
- Legal-sensitive upgrades such as `违法所得`, `赃款`, `实际控制`, `代持`,
  `最终归属`, `犯罪团伙`, `共同犯罪`, `坐实`, or `锁定` fail unless the wording
  is explicitly downgraded to `涉嫌`, `线索`, `需复核`, `待补证`, or
  `暂不能认定`.
- A final answer without evidence boundaries and proof actions fails validation.
- A proof action that lacks target, material, time/field scope, linked gap, or
  forbidden-upgrade boundary emits a validation warning; it remains a补证建议,
  not a claim that the material has been obtained.
- User-visible text must not expose `DuckDB`, `MCP`, `schema`, `query_id`,
  `tool`, `workflow`, `doctor`, `eval`, `case_id`, raw JSON, or local paths.
- Validation failures and warnings produce internal `audit_events` with code,
  path, category, fact policy, and repair action. These events stay in MCP
  `_meta`/audit evidence and are not public case language.

## Reask Boundary

Borrow Instructor's reasking idea only for structure repair. A model may be
asked to add a missing field, move a candidate edge to `evidence_boundaries`, or
translate internal wording into case language. It must not be asked to invent a
missing fact.

When a validation error says a fact, amount, transaction edge, evidence ref, or
source hash is missing, the next action is one of:

- return to MCP/DuckDB or a generated artifact to obtain the source-backed fact;
- downgrade the item to `evidence_boundaries`;
- state that the current data cannot support the claim.

## Visibility

The user should not see this structure as JSON unless they explicitly request
machine-readable output. Ordinary final answers should read as公安经侦 material:
conclusion first, then facts and basis, abnormal meaning, evidence boundary, and
next proof action.

## Implementation Points

- Machine-readable schema: `investigation-answer.schema.json`.
- Runtime validator: `mcp/investigation-answer-contract.mjs`.
- Deterministic smoke test: `scripts/investigation-answer-contract-smoke.mjs`.
- Delivery owner: `delivery-qc`; report owner: `report-builder`.
- Truth source: MCP/DuckDB, generated artifacts, or reviewed attachments. The
  schema validates structure and evidence anchoring; it does not create facts.
- Internal audit: validation failure, delivery-QC interception, and SQL diagnose
  reasons may be recorded as `audit_events`; they are quality evidence, not
  user-visible findings and not a substitute for DuckDB/MCP facts.
