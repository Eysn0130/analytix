# D-0248 Go Runtime G6 Readiness Gate

Generated at: 2026-06-24T17:50:50.526Z
Mode: strict-gate
Default backend ready: passed
Go default backend enabled: true
Renderer-visible Go switcher: false

Dry-run output is audit-only and cannot be used as cutover evidence.

## Blockers

- none

## Checks

| ID | Required | Status | Evidence | Reason |
| --- | --- | --- | --- | --- |
| go-server-conformance | yes | passed |  |  |
| durable-restart-proof | yes | passed |  |  |
| provider-matrix-local | yes | passed |  |  |
| provider-matrix-credentialed | no | skipped |  | ANALYTIX_D0243_PROVIDER_MATRIX_STATUS is skipped; credentialed provider evidence remains post-cutover live validation |
| mcp-matrix-local | yes | passed |  |  |
| mcp-matrix-credentialed | no | skipped |  | ANALYTIX_D0243_MCP_MATRIX_STATUS is skipped; credentialed MCP evidence remains post-cutover live validation |
| packaged-qa | no | skipped | /Users/sun/Projects/analytix/docs/analytix/upstreams/d0248-packaged-go-runtime-qa-report.json | ANALYTIX_D0243_PACKAGED_QA_STATUS is skipped; packaged desktop QA remains post-cutover live validation |
| product-sovereignty | yes | passed |  |  |
| typecheck | yes | passed |  |  |
| build-runtime | yes | passed |  |  |
| explicit-env-gate | no | missing |  | ANALYTIX_GO_RUNTIME_G6_READY is not set to 1; operator evidence remains post-cutover live validation |

## Commands

- `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./... && npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1 && npm run test -- src/main/runtime/analytix-adapter.test.ts --run`
- `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./... && npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1`
- `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...`
- `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...`
- `npm run scan:product-sovereignty`
- `npm run typecheck`
- `npm run build:runtime`

## Notes

- D-0244/D-0245 matrix green is absorption readiness only; deterministic local gates now authorize the default Go backend.
- Dry-run reports are audit previews and cannot substitute for recorded command evidence.
- Skipped or missing local hard gates are blockers in --gate/--strict mode.
- Credentialed provider/MCP, packaged desktop QA, and operator evidence remain post-cutover live validation and do not block the default local gate.
- DeepSeek cache/prefix-hit proof is a DeepSeek-specific enhancement; OpenAI-compatible, Anthropic-compatible, and custom endpoint request semantics remain separate.
