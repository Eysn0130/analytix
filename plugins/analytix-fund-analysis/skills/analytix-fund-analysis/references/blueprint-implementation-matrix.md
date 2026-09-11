# Case-Neutral Blueprint Implementation Matrix

This document is a case-neutral engineering status resource for the Analytix
fund-analysis plugin. It is not case evidence, a source receipt, a formal
report, or release evidence. It MUST NOT contain real case names, subjects,
case identifiers, local case paths, historical evidence directories, dataset
counts, account identifiers, or copied case findings.

Status vocabulary: `done`, `partial`, `missing`, `wrong`, `overfit`.
`partial` remains a release blocker when a row controls facts, citations,
reports, PII, source readiness, or user-visible conclusions.

## P0 Publication Boundary

- Tool output and provider text are untrusted proposal material.
- A successful transport, a non-empty evidence id, a rendered file, or a
  structurally complete answer card does not authorize a fact.
- Without a current host-issued EvidenceReceipt from the authoritative
  registry, factual claims remain `unsupported` or `unresolved`.
- Without a valid host-issued PublicationReceipt, no generated artifact is a
  formal report.
- `write_report=false` performs no report, staging, manifest, image, material
  pack, or substitute-report file write.
- A report service failure returns a deterministic blocker. It does not create
  a local substitute report or broaden the available facts.
- Missing, invalid, empty, or partial values stay unknown or partial. They do
  not become zero, no-hit, no relationship, or a whole-case conclusion.
- Local DuckDB analysis may produce bounded candidate data only. It cannot
  self-issue evidence authority or publication authority.

## Test Pyramid

| Layer | Acceptance scope | Minimum proof |
| --- | --- | --- |
| L0 static closure | Schemas, allowlists, references, resources, and runtime contracts agree. | `node --check`, focused contracts, `doctor`, reference equality, `git diff --check`. |
| L1 deterministic containment | Missing values, cache isolation, strict dispatch, resource safety, and zero-write behavior are deterministic. | P0 containment and focused contracts with temporary roots. |
| L2 evidence kernel | Current source identity, grants, receipts, claims, and exact support are host verified. | Same-thread/turn/case/epoch/snapshot hostile and failure tests. |
| L3 publication | Accepted answers and formal reports pass the final evidence/publication gates. | Final-answer and PublicationReceipt tests with no free-text or file bypass. |
| L4 desktop/package | Renderer, preload, main, Go runtime, restart, and package paths preserve the gates. | Real UI and current-platform package validation. |
| L5 release | All deterministic redline metrics are zero or 100% as specified. | Fresh release-gate output with zero skips. |

## Blueprint Implementation Matrix

| Blueprint item | Target capability | Implemented files | Status | Test | Evidence path | Release blocker |
| --- | --- | --- | --- | --- | --- | --- |
| Current-source preflight | Admit facts only after a current-run, case-bound source probe. | Go runtime source readiness and plugin source-boundary modules. | partial | Native probe and stale-catalog tests | Fresh deterministic test output only; never a case directory. | yes - live identity, connection epoch, and snapshot binding remain P1. |
| Advertisement/execution allowlist | Re-resolve the exact advertised tool and strict schema immediately before execution. | Go execution grant path and plugin MCP dispatch. | partial | Unadvertised, unknown-schema, wrong-server, and expired-grant tests | Focused contract output. | yes - all execution modes must be closed. |
| Semantic ToolOutcome | Preserve transport status, semantic status, blocker, coverage, and MCP error state separately. | Plugin MCP result runtime and Go adapters. | partial | HTTP-success/semantic-failure and partial-coverage tests | Focused contract output. | yes - host receipt issuance remains P1. |
| Evidence and claim defaults | Default to unsupported/unresolved and require registry membership plus field-level support. | Evidence ledger, answer card, claim review, and future host registry. | partial | Fake/mismatched citation tests | Focused contract output. | yes - authoritative receipt and verifier registries remain P1. |
| Missing and partial semantics | Preserve unknown values and prohibit partial-to-complete upgrades. | Numeric normalizers, graphs, cards, workbench, and report guards. | partial | Empty-is-not-zero and partial-coverage contracts | Focused contract output. | yes - every externally derived field must be covered. |
| Case-bound factual caching | Never use active/current aliases for factual results. | Frontdoor and analysis cache owners. | partial | A/B/A case-binding isolation tests | Focused contract output. | yes - all remaining factual caches require frozen context keys. |
| PII projection | Mask restricted identifiers in ordinary model, chat, log, and report surfaces. | Agent-context hygiene and artifact policy. | partial | Structured/free-text/deep-nesting PII contracts | Focused contract output. | yes - controlled full-PII artifact authorization remains P2. |
| Report containment | Block formal report generation until verified claims and host publication authority exist. | Report publication guard and report-capable tool dispatch. | partial | Report receipt, backend-failure, and zero-write tests | Focused contract output. | yes - atomic publication and PublicationReceipt registry remain P1/P2. |
| Progressive resource safety | Expose only case-neutral operational references to providers. | `mcp/progressive-resources.mjs` and this synchronized reference. | done | Traverse every resource definition and read through the provider path | P0 containment contract output. | no for the scoped resource leak; broader P0/P1 gates still block release. |
| Release readiness | Compute readiness only from fresh implementation and deterministic validation. | OpenSpec tasks and release gates. | partial | Full P0-P4 matrix | Current worktree validation only. | yes - the platform is not release-ready while any P0/P1 item remains. |

## Case-Neutral Upstream Adaptation Markers

These markers record design vocabulary only; their presence is not proof that
the capability is implemented or safe to publish.

| Source family | Case-neutral mechanism | Current admission state | Required proof |
| --- | --- | --- | --- |
| Data Analytics | source-of-truth, `validation_state`, delivery, focused workflows, `case_source_envelope`, report, artifact | partial | Current source access, quality checks, reproducibility, and host publication tests. |
| Investment Banking | Navigator not commander, focused owner, support layer, `final_answer_owned_by_focused_skill`, hero deliverable | partial | One router, current native reads, source/as-of records, manifest, and client-ready QC. |
| Public Equity Investing | professional judgment, `unsupported`, claim-review, evidence maturity, `暂不能认定`, 下一步 | partial | Fact/inference/conflict/as-of classification and citation readiness. |
| Postgres/database MCP | `inspect_case_schema`, `run_case_sql`, `explain_case_sql`, `diagnose_case_sql`, `count_case_rows`, `preview_case_rows`, read-only, `fc_*_norm`, `analysis_*` | partial | Parser/binder policy, read-only connection, table/function allowlists, timeout, row/byte limits, and deterministic diagnostics. |
| Instructor | `FinalInvestigationAnswer`, `verified_facts`, `evidence_refs`, `fund_flow_paths`, `evidence_boundaries`, `next_proof_actions`, `repair_actions`, `audit_events` | partial | Schema-first validation and at most one structure-only repair that cannot add facts or support. |

## Release Readiness

This resource records no release candidate, package hash, installed cache,
case database result, front-door transcript, user environment, or historical
pass. Fresh checks are required after every relevant source change.

Minimum case-neutral validation for this resource slice:

- `node --check mcp/progressive-resources.mjs`
- `node --check scripts/p0-containment-contract.mjs`
- `node scripts/p0-containment-contract.mjs`
- `node scripts/doctor.mjs --json`
- verify the root and embedded copies are byte-identical
- `git diff --check` for the scoped files

Passing this slice proves only that the provider resource surface is
case-neutral and fail-closed for the listed historical content. It does not
complete P0, P1, the OpenSpec change, or the single project Goal.
