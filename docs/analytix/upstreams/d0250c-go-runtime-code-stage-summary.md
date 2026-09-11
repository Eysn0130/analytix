# D-0250C Go Runtime Code-Stage Report

Generated at: 2026-06-24T17:50:57.102Z
Status: code-stage-closed
Reasonix absorption status: code-stage-closed
Go runtime core status: deterministic-core-green
Go default live gate status: post-cutover-live-validation-pending
Code stage closed: true
D-0250C live cutover status: post-cutover-live-validation-pending
Strict G6 passed: false
Go default candidate: true
TypeScript default-path fallback retained: false
TypeScript rollback override retained: false
Kun/Analytix product layer preserved: true
Reasonix engine/runtime only: true
LLM answer-quality evidence used: false

## Superiority Matrix

Rows: 13
Decisions: absorb=1, adapt=5, keep-kun=5, reject=1, defer=1

| ID | Decision | Status | Absorption/live gate |
| --- | --- | --- | --- |
| deepseek-prefix-cache-stability | adapt | green | not-required |
| prefix-shape-contract-baseline | keep-kun | green | not-required |
| provider-stream-usage-reasoning-guards | adapt | green | not-required |
| provider-endpoint-family-request-shape | keep-kun | green | not-required |
| agent-loop-job-subagent-lineage | adapt | green | not-required |
| mcp-lifecycle-lazy-schema-reconnect-redaction | absorb | green | not-required |
| mcp-search-meta-tool-boundary | keep-kun | green | not-required |
| approval-user-input-tool-result-history-repair | adapt | green | not-required |
| runtime-event-sse-session-durability | keep-kun | green | not-required |
| autoresearch-workflow-create-loop-engine-discipline | adapt | green | not-required |
| kun-analytix-product-layer-baseline | keep-kun | green | not-required |
| reasonix-public-protocol-and-top-level-ui | reject | rejected | not-required |
| go-default-live-cutover-evidence | defer | deferred | post-cutover-live-validation-pending |

## Deterministic Absorption IDs

- deepseek-prefix-cache-stability
- provider-stream-usage-reasoning-guards
- agent-loop-job-subagent-lineage
- mcp-lifecycle-lazy-schema-reconnect-redaction
- approval-user-input-tool-result-history-repair
- autoresearch-workflow-create-loop-engine-discipline

## Pending Live Validation IDs

- go-default-live-cutover-evidence

## Missing Live Evidence

- provider-matrix-credentialed
- mcp-matrix-credentialed
- packaged-qa
- explicit-env-gate
- credentialed-provider-matrix-json
- credentialed-mcp-execution-json
- packaged-desktop-qa-json
- ANALYTIX_GO_RUNTIME_G6_READY=1
- operator-gate-json

## D-0250B Missing Evidence

- provider-matrix-credentialed
- mcp-matrix-credentialed
- packaged-qa
- explicit-env-gate

## D-0250C Live Evidence Blockers

- credentialed-provider-matrix-json
- credentialed-mcp-execution-json
- packaged-desktop-qa-json
- ANALYTIX_GO_RUNTIME_G6_READY=1
- operator-gate-json

## Row Validation

- Passed: true
- Rows: 13
- Failures: none

## Evidence Paths

- D-0250B cutover report: /Users/sun/Projects/analytix/docs/analytix/upstreams/d0250b-go-runtime-cutover-report.json
- D-0248 preflight report: /Users/sun/Projects/analytix/docs/analytix/upstreams/d0248-go-runtime-g6-preflight-report.json
- D-0248 readiness gate report: /Users/sun/Projects/analytix/docs/analytix/upstreams/d0248-go-runtime-readiness-gate-report.json

## Rollback

- TypeScript runtime is no longer the default/fallback and is not available as
  an executable rollback override. `ANALYTIX_RUNTIME_BACKEND=typescript` is a
  retired-backend diagnostic; operational rollback means restoring a previous
  release.
- Go runtime is the default core after deterministic local conformance and adapter canary; live JSON evidence is post-cutover validation.
- Do not use ANALYTIX_RUNTIME_BACKEND=typescript for rollback; the current adapter returns a retired_backend diagnostic.

## Next Exit Plan

- D-0250C-live: Collect credentialed provider matrix, credentialed MCP execution, packaged desktop QA, and operator gate JSON as post-cutover live validation; do not fake missing live evidence.
- D-0250D: Keep validating replacement tests and rollback instructions while Go remains the default runtime core.
