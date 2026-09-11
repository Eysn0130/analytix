# Data Analytics installed-plugin absorption ledger

Status: Pinned white-box audit complete; implementation and benchmark remain
open.

## Current installed source

- Package: `data-analytics` `0.2.8-13ceeea1f599`
- Local root:
  `/Users/sun/.codex/plugins/cache/openai-curated-remote/data-analytics/0.2.8-13ceeea1f599`
- Manifest SHA-256:
  `a3af158bb9ac14c316aee39371731be3e6825c135dbcbddf82bec07407d035f8`
- Router SHA-256:
  `2239188d6ac667a9e4b546ffb441f17ac3cfe90fe7683ac68506789ef8dabeb3`
- License posture: the manifest says `Proprietary`; the package root has no
  standalone LICENSE/NOTICE file. Goal-thread authorization permits study and
  approved reuse, but release provenance still needs a durable authorization
  record covering the exact reused bytes and destination.
- Installed-category discovery found this as the only cached manifest whose
  interface category is exactly `Data & Analytics`.

## As-built review

The strongest mechanisms are deeper than prompt advice:

- focused skills require fresh catalog/metadata discovery, compare authority,
  freshness, definition, grain, coverage, and directness, and stop rather than
  invent actuals when the required source is unavailable;
- data-quality and validation workflows check grain, missingness, duplicates,
  join cardinality, time windows, exclusions, independent recomputation, and
  record-level boundary samples;
- notebook guidance makes SQL/Python work reproducible and keeps assumptions,
  source notes, and rerun steps with the analysis;
- the MCP artifact server enforces bounded snapshots: at most 50 datasets,
  2,000 rows per dataset, and 3,000,000 encoded bytes. Widget rows, cells, data
  points, and payload bytes have separate bounds;
- report delivery distinguishes structural validation, rendering, and delivery
  confirmation, and has deterministic tests for hostile/invalid payloads and
  portable artifact behavior.

Fresh focused validation:

```text
node --test tests/source-discovery-contract.test.mjs tests/mcp-server.test.mjs tests/report-delivery-contract.test.mjs
89 passed, 0 failed, 0 skipped, 0 todo
```

This is not an evidence-publication authority. Source identifiers in an
artifact are supplied by the caller and checked mainly for schema/membership;
they are not host-issued receipts that prove same thread, turn, case, epoch,
snapshot, query, source identity, or field-level claim support. Several nested
schema areas remain intentionally extensible. A renderable artifact can pass
structural QA without establishing fact truth, PII authority, or a publication
receipt.

## Admission decisions

| Capability | Decision | Analytix landing | Deterministic proof required |
| --- | --- | --- | --- |
| Fresh source discovery and no-source stop | adapt | Go current-run source probe plus capability-specific source matrix | Cached catalog/disconnected/spoofed/empty/partial cases cannot mint grants or facts. |
| Data-quality dimensions | adopt as typed checks | Immutable snapshot admission and claim coverage | Grain/null/duplicate/join/freshness/currency/timezone checks bind exact snapshot hashes. |
| Independent recomputation and boundary samples | adapt | Funds verification and ClaimRecord verifier | Recomputed values and sampled source-record ids match exact receipt fields. |
| Reproducible notebook/report companion | adapt | Controlled support artifact after evidence admission | Notebook cannot publish facts and contains no raw reasoning or ordinary full PII. |
| Bounded artifact payload | adopt and strengthen | Report staging and controlled-artifact ingress | Row/cell/dataset/byte/depth limits fail before allocation and before filesystem visibility. |
| Structural/render QA as publication proof | reject | Final Evidence Gate and PublicationReceipt remain authoritative | Render success with fake/mismatched citations is blocked. |

No row is reached or exceeded from this audit. OpenSpec tasks 8.14, 8.19,
8.20, and the P2 report/data tasks remain open.
