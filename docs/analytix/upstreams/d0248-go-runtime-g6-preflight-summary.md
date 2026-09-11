# D-0248 Go Runtime G6 Preflight

Generated at: 2026-06-24T17:50:50.530Z
Mode: strict
Passed: true
Expected blocked: false
Default backend ready: true
Go default backend enabled: true
Renderer-visible Go switcher: false

## Blockers

- none

## Commands

| ID | Status | Command | Reason |
| --- | --- | --- | --- |
| go-runtime-go-test | passed | `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...` |  |
| runtime-conformance | passed | `npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1` |  |
| adapter-tests | passed | `npm run test -- src/main/runtime/analytix-adapter.test.ts --run` |  |
| contracts-goal-tests | passed | `npm --prefix packages/runtime run test -- tests/contracts.test.ts tests/runtime-event-reducer.test.ts tests/goal-tools.test.ts --no-file-parallelism --maxWorkers=1` |  |
| preload-settings-tests | passed | `npm run test -- src/preload/preload-runtime-request.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings.test.ts --run` |  |
| renderer-route-surface-tests | passed | `npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/chat/FloatingComposer.test.ts --run` |  |
| product-sovereignty-scan | passed | `npm run scan:product-sovereignty` |  |
| typecheck | passed | `npm run typecheck` |  |
| build-runtime | passed | `npm run build:runtime` |  |
| packaged-qa | skipped | `/Users/sun/.local/node/node-v22.22.1-darwin-arm64/bin/node scripts/d0251-packaged-go-runtime-qa.mjs --json` | packaged QA is not fully passed; see packaged QA evidence |

## Readiness Gate

- Command: `/Users/sun/.local/node/node-v22.22.1-darwin-arm64/bin/node scripts/d0247-go-runtime-readiness-report.mjs --json --gate --out-json docs/analytix/upstreams/d0248-go-runtime-readiness-gate-report.json --out-md docs/analytix/upstreams/d0248-go-runtime-readiness-gate-summary.md`
- Exit code: 0
- Evidence: /Users/sun/Projects/analytix/docs/analytix/upstreams/d0248-go-runtime-readiness-gate-report.json

Dry-run and expected-blocked reports are not cutover authorization.
