# D-0250B Go Runtime Cutover Report

Generated at: 2026-06-24T17:50:54.121Z
Status: intake-closed
D-0250B intake closed: true
D-0250C live cutover status: post-cutover-live-validation-pending
Strict G6 passed: false
Go default candidate: true
TypeScript default-path fallback retained: false
TypeScript rollback override retained: false
Reasonix engine/runtime only: true

## Evidence Intake

| ID | Status | Evidence | Missing reason |
| --- | --- | --- | --- |
| provider-matrix-credentialed | skipped |  | ANALYTIX_D0243_PROVIDER_MATRIX_STATUS is skipped; credentialed provider evidence remains post-cutover live validation |
| mcp-matrix-credentialed | skipped |  | ANALYTIX_D0243_MCP_MATRIX_STATUS is skipped; credentialed MCP evidence remains post-cutover live validation |
| packaged-qa | skipped | /Users/sun/Projects/analytix/docs/analytix/upstreams/d0248-packaged-go-runtime-qa-report.json | ANALYTIX_D0243_PACKAGED_QA_STATUS is skipped; packaged desktop QA remains post-cutover live validation |
| explicit-env-gate | missing |  | ANALYTIX_GO_RUNTIME_G6_READY is not set to 1; operator evidence remains post-cutover live validation |

## Missing Evidence

- provider-matrix-credentialed: skipped - ANALYTIX_D0243_PROVIDER_MATRIX_STATUS is skipped; credentialed provider evidence remains post-cutover live validation
- mcp-matrix-credentialed: skipped - ANALYTIX_D0243_MCP_MATRIX_STATUS is skipped; credentialed MCP evidence remains post-cutover live validation
- packaged-qa: skipped - ANALYTIX_D0243_PACKAGED_QA_STATUS is skipped; packaged desktop QA remains post-cutover live validation
- explicit-env-gate: missing - ANALYTIX_GO_RUNTIME_G6_READY is not set to 1; operator evidence remains post-cutover live validation

## Rollback

- Go runtime is selected when ANALYTIX_RUNTIME_BACKEND is unset or set to analytix/go/go-runtime-default.
- Do not use ANALYTIX_RUNTIME_BACKEND=typescript for rollback; the current adapter returns a retired_backend diagnostic.
- src/main/runtime/analytix-adapter.ts rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as a retired-backend diagnostic and does not start TS runtime.

## Next Exit Plan

- D-0250C: Collect real credentialed provider, MCP execution, packaged QA, and operator JSON as post-cutover live validation.
- D-0250D: Continue replacement and rollback validation while Go remains the default runtime core.
