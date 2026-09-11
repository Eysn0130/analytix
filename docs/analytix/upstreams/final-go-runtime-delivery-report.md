# Final Go Runtime Delivery Report

Status: Historical dated delivery evidence, not current release authorization.
Applies to: the source baseline and reconciled worktree recorded below.
Current authority: `docs/analytix/README.md`, `packages/runtime-go/README.md`,
current code/tests, and a freshly executed applicable gate.

The word “Final” is the report's stage name. It does not make its provider,
package, GUI, benchmark, or readiness observations permanent facts.

Generated at: 2026-06-27 09:22 CST
Reconciled for current worktree: 2026-06-29 CST

Source baseline commit: `1327277366b156cf1ec68ff96faa59006f710960`

Status: goal-control delivery is still active. Go runtime is the default core.
Current adapter code rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as a retired
backend; older rollback evidence in this directory is useful audit material,
not current startup guidance. Current release authorization must be rerun on
the current worktree through `npm run runtime:go:release-gate` and the final
validation set. A stale `sourceCommit` in an older JSON report is not current
acceptance.

## Conclusion

- Go runtime default core is active. `ANALYTIX_RUNTIME_BACKEND` unset, `analytix`, `go`, `go-runtime`, or `go-runtime-default` resolves to `go-runtime-default`.
- TypeScript is not retained as a default-path fallback. `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired backend and is rejected by the desktop adapter.
- Default Go startup now fails closed if the Go runtime cannot start; it does
  not silently fall back to TypeScript.
- Release packaging is blocked by `runtime:go:release-gate`, which runs product
  sovereignty, preflight with `--gate`, and `go test -tags analytix_prod ./...`
  before macOS/Windows/Linux packaging jobs.
- The desktop real path was validated with Electron dev app startup and Go runtime HTTP/SSE probes.
- DeepSeek live validation passed with env-injected credentials only. No API key or credential value was written to code, docs, JSON evidence, fixtures, or git diff.
- 2026-06-27 short live validation read top-level provider settings correctly and passed the real DeepSeek cache probe with `hasApiKey=true`, model `deepseek-v4-pro`, cache hit rate about `0.9868`, and no secret material in output. Non-DeepSeek live provider validation is still `not_configured`, so final credentialed-provider coverage is not complete.
- Other provider live validation remains pending until real non-DeepSeek keys are configured, but deterministic non-regression passed for OpenAI chat completions, OpenAI responses, Anthropic messages, and custom full endpoints.
- Actual packaged GUI smoke and packaged QA passed against `dist/mac-arm64/analytix.app` with temporary user data, synthetic provider credentials, no provider network call, `window.analytix` preserved, provider-profile settings patch accepted, runtime restart/health/info passed, and no bundled Go source in the packaged artifact. Packaged QA still reports missing final coverage for `credentialed-provider` and `operator-gate`.
- No Reasonix top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer UI was added. Product sovereignty scan passed.

## 2026-06-27 Goal-Control Evidence Update

- `npm run runtime:go:packaged-gui-smoke -- --json --actual --app-path dist/mac-arm64/analytix.app` passed with `actualPackagedAppLaunched:true`, `settingsProfilePatchAccepted:true`, `settingsCompatPatchUsed:false`, `runtimeInfoProductClean:true`, and `providerNetworkCalled:false`.
- `npm run qa:runtime:packaged -- --json --actual --app-path dist/mac-arm64/analytix.app` passed and wrote `docs/analytix/upstreams/runtime-go-live-evidence/packaged-go-runtime-qa.json`; the report keeps `finalAcceptanceReady:false` because `credentialed-provider` and `operator-gate` coverage remain missing.
- `npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate` returned `partial`: DeepSeek cache probe passed, non-DeepSeek provider was `not_configured`, and `secretsPrinted:false`.
- `npm run runtime:go:product-regression -- --json` passed after the sub-agent profile/settings and Go task schema updates. The matrix still records actual packaged smoke as a separate final gate rather than treating deterministic smoke as enough.

## Delivery Fix

During final app-path QA, the current local settings were found to have a configured API key but an empty `runtime.runtimeToken`. Existing TypeScript runtime semantics treat an empty runtime token as an insecure local runtime mode, but the Go default path rejected the empty token before startup and the Go server defaulted empty token to `tok-1`.

The final patch aligns Go default with existing desktop settings semantics:

- `src/main/runtime/analytix-adapter.ts` now passes `--insecure` to the Go runtime when `isAnalytixRuntimeInsecure(runtime)` is true.
- `packages/runtime-go/cmd/runtime-server/main.go` accepts `--insecure` and reports an empty ready token when launched with empty runtime token.
- `packages/runtime-go/internal/server/runtime_server.go` and `packages/runtime-go/internal/adapters/inbound/httpapi/response.go` allow unauthenticated local requests only in explicit insecure mode.
- Tests cover empty-token insecure runtime and no-auth adapter requests.

This preserves bearer-token auth when a runtime token exists.

## Release Closure Audit Update

2026-06-25 final audit found and closed one Go-default contract gap: the Go
runtime server default path did not yet cover every renderer baseline endpoint
needed by the Kun/Analytix desktop contract. The server now includes
thread-create/delete, runtime tool diagnostics, skills, attachments with
`localFilePath`, memory, goal/todos, compact, review, steer, interrupt, and
checkpoint rewind audit endpoints in addition to existing health/runtime-info,
thread/session/SSE/turn/approval/user-input routes.

`packages/runtime-go/runtime_server_test.go` now has
`TestRuntimeServerDefaultContractCoversRendererBaselineEndpoints`, proving those
baseline routes are present without adding a renderer-visible Go route,
Reasonix SessionAPI/config root, or Reasonix Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer product entry.

## Contract Equivalence Closure Update

2026-06-25 follow-up runtime QA found and closed Go-default contract
regressions in the `POST /v1/threads/:id/turns` path and local state stores.
The Go server now parses, persists, and replays the Analytix turn contract
fields used by the renderer and TypeScript runtime contract, including `mode`,
`guiPlan`, `attachmentIds`, `fileReferences`, `approvalPolicy`, `sandboxMode`,
`disableUserInput`, and `maxModelSteps`.

The user item, turn record, and SSE replay now retain attachments, file
references, GUI plan, per-turn mode, approval policy, sandbox mode, user-input
disablement, and model-step limits so resumed renderer projection does not lose
those fields. Approval and user-input gates now respect per-turn
`approvalPolicy`, `sandboxMode`, and `disableUserInput`; headless/IM or
explicitly disabled user-input turns no longer force a blocking user-input
request.

Attachments and memory are now persisted under the configured Go runtime
`dataDir` instead of process-local maps. Attachment persistence includes
content files, metadata, SHA-256 hash, scope, `localFilePath`, and diagnostics;
memory persistence supports list/create/patch/delete/diagnostics and survives
server restart. Runtime tools, skills, and MCP diagnostics were also corrected
so unavailable Go-default capabilities report `available:false` or disabled
status with a reason instead of advertising unconfigured MCP/skills/subagent
readiness.

This update did not add renderer, preload, settings, or UI product entries; it
did not add a renderer-visible Go route; and it did not add Reasonix
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product surfaces.
Reasonix-derived behavior remains limited to engine/runtime contract
absorption. No new live provider, live MCP, packaged-app, or credentialed
evidence is claimed by this update.

## Startup Diagnostics Follow-Up

2026-06-25 follow-up QA found and closed a startup regression introduced by the
combination of honest Go diagnostics and the desktop Go-default canary. The Go
runtime correctly reports MCP and subagents as `available:false` with reasons
when those live systems are not connected, but
`src/main/runtime/analytix-adapter.ts` still required
`capabilities.mcp.available === true` and
`capabilities.subagents.available === true` before accepting
`go-runtime-default`. That could reject the default Go runtime during desktop
startup.

The adapter canary now validates diagnostics honesty instead of assumed
availability: `available` must be a boolean, and unavailable capabilities must
include a non-empty `reason`. The formal packaged/live evidence scripts were
aligned to record actual `mcpAvailable` and
`subagentsAvailable` values while using `mcpDiagnosticHonest` and
`subagentsDiagnosticHonest` as the contract evidence. This preserves the rule
that MCP, skills, and subagents must not advertise unconfigured availability in
the Go default contract server.

Follow-up dev smoke on this worktree ran `npm run dev`, built runtime/main/
preload/renderer, started Electron, and confirmed `GET /health` returned
`{"mode":"serve","service":"analytix","status":"ok"}` on `127.0.0.1:8901`.
`GET /v1/runtime/info` showed MCP and subagents as `available:false` with
explicit reasons. Electron/dev were stopped after the smoke and
`127.0.0.1:8901` no longer had a `runtime-server` listener. The desktop adapter
now recognizes stale Analytix Go `runtime-server` processes during managed Go
startup, waits after forced Go child termination, and uses an isolated canary
thread so old durable threads cannot poison default-startup SSE replay.

No renderer, preload, settings, UI, product entry, provider selection, or
Reasonix public surface changed in this follow-up.

## Desktop And Runtime QA

Packaged app build was not executed in this pass because full packaging is heavier than the requested minimum. Local app/runtime QA was executed instead.

Electron dev app smoke:

- `npm run dev` launched the app.
- Renderer loaded successfully.
- Runtime `/health` returned `{"mode":"serve","service":"analytix","status":"ok"}` on `127.0.0.1:8901`.
- Unauthenticated `/v1/runtime/info` returned `200`, `host:"127.0.0.1"`, `insecure:true`, and upstream absorption capabilities.
- Thread list returned `200`.
- Turn create returned `202`.
- SSE replay returned `200` and included `turn_started`, `item_created`, `assistant_reasoning_delta`, `assistant_text_delta`, `usage`, `pipeline_stage`, and `turn_completed`.
- `/v1/runtime/go` returned `404`, proving no renderer-visible Go route was added.
- The original dev app/runtime were shut down in that pass; the follow-up
  startup stabilization smoke above confirmed no `runtime-server` listener
  remained on `127.0.0.1:8901` after dev exit.

Local runtime binary smoke:

- Temporary `runtime-server` binary started with empty runtime token plus `--insecure`.
- `/health`, `/v1/runtime/info`, `/v1/threads`, turn create, and SSE replay passed.
- `ready.runtimeToken === ""` and `runtimeInfo.insecure === true`.
- `/v1/runtime/go` returned `404`.

## Provider Acceptance

DeepSeek live probe:

- Status: passed.
- Endpoint family/format: OpenAI chat completions.
- Stream parsing: passed, SSE mode, 19 frames, 18 JSON frames, 0 parse errors.
- Usage parsing: passed; token fields included `prompt_tokens`, `completion_tokens`, `total_tokens`, `prompt_cache_hit_tokens`, and `prompt_cache_miss_tokens`.
- Cache telemetry: passed; `providerScoped:true`; fields were `prompt_cache_hit_tokens` and `prompt_cache_miss_tokens`.
- Request shape: passed; stream requested; `max_tokens`; no DeepSeek-specific request fields.
- Error handling: passed with expected HTTP 400 intentional error probe.
- Redaction: passed; credential value recorded false.

Other provider live probes:

- OpenAI-compatible: skipped, pending real `ANALYTIX_D0251_OPENAI_COMPAT_*` env.
- Anthropic-compatible: skipped, pending real `ANALYTIX_D0251_ANTHROPIC_COMPAT_*` env.
- Custom endpoint: skipped, pending real `ANALYTIX_D0251_CUSTOM_ENDPOINT_*` env.

Deterministic provider non-regression passed for OpenAI chat completions, OpenAI responses, Anthropic messages, custom full endpoints, provider cache accounting, URL construction, request bodies, headers, usage parsing, and DeepSeek telemetry isolation.

## Validation Commands

- `git diff --check`: passed.
- `cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...`: passed.
- tracked-file secret scan: passed, 1873 files scanned; only synthetic test
  keys or env-variable references were found, no real secret value was
  recorded or printed.
- `npm run test -- src/main/runtime-go-packaged-contract-report.test.ts src/main/runtime-go-engine-absorption-report.test.ts --run`: passed, formal runtime-go report wrapper tests.
- `npm run runtime:go:rollback-retirement-evidence -- --json`: passed in this historical snapshot; current adapter now rejects TypeScript backend startup.
- `npm run runtime:go:rollback-retirement-report -- --json`: passed in this historical snapshot; the report is audit evidence, not current rollback startup guidance.
- `npm --prefix packages/runtime run test -- tests/contracts.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1`: passed, 2 files, 53 tests.
- `npm run scan:product-sovereignty`: passed.
- `npm run typecheck`: passed.
- `npm run build:runtime`: passed.
- `npm run build`: passed.

Additional targeted validation:

- `npm --prefix packages/runtime run test -- tests/provider-cache-contract.test.ts --no-file-parallelism --maxWorkers=1`: passed.
- `npm run test -- src/shared/openai-compat-url.test.ts src/shared/app-settings-provider.test.ts src/main/provider-connection.test.ts src/main/services/write-inline-completion-service.test.ts src/main/claw-scheduled-task-detector.test.ts src/preload/preload-runtime-request.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings.test.ts src/renderer/src/agent/runtime-client.test.ts src/renderer/src/agent/analytix-runtime.test.ts --run`: passed, 10 files, 193 tests.
- `npm --prefix packages/runtime run test -- src/adapters/model/compat-model-client.endpoint-format.test.ts src/adapters/model/multi-provider-model-client.test.ts --no-file-parallelism --maxWorkers=1`: passed, 2 files, 4 tests.
- DeepSeek live provider probe with env-injected credentials and `--no-write`: passed for DeepSeek, other live provider keys pending.
- Electron dev app local smoke: passed.

## Historical Final Gate Snapshot

- Go default: passed in the recorded deterministic/package QA snapshot.
- TypeScript retired-backend handling: current adapter rejects `ANALYTIX_RUNTIME_BACKEND=typescript`.
- DeepSeek live: passed in the recorded env-injected probe.
- Other providers deterministic: passed.
- Other providers live: pending real credentials in the 2026-06-27 snapshot.
- Settings/provider/bridge/UI sovereignty: passed by targeted tests and product sovereignty scan.
- API key leakage: none recorded by live probe output; no credential value was written to tracked files.

## Current Release-Gate Closure

As of the 2026-06-29 post-complete audit on commit
`fd0c5d0417c80f22105a02293e135bc950df17c5`, current release authorization no
longer comes from this historical delivery report alone. It comes from the
current staged-worktree gate commands below, rerun with refreshed live evidence,
`ANALYTIX_RUNTIME_READY=1` and
`ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1`.

- `npm run runtime:go:live-evidence -- --json`: `passed`; provider, MCP,
  packaged desktop QA, packaged GUI bridge smoke, packaged session soak, and
  operator components all passed with `finalGateBlocked:false`.
- `npm run runtime:go:default-readiness-report -- --gate --json`: `passed`;
  `goDefaultReady:true`, `deterministicDefaultReady:true`,
  `finalAcceptanceReady:true`, and `finalAcceptanceStatus:"ready"`.
- `npm run runtime:go:release-gate`: `passed`; product sovereignty, strict
  runtime-go preflight, and `go test -tags analytix_prod ./...` all passed.

Current release closure: the current staged worktree satisfies the release
gate. Current adapter code rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as a
retired backend; rollback-retirement artifacts remain audit evidence rather
than an authorized production startup path.
