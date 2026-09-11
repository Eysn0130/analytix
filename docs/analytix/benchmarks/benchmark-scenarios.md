# analytix benchmark scenarios

Status: Normative scenario families followed by historical dated result rows.
Current as of: 2026-07-10 for this lifecycle note only.

Sections before “Scenario Record Template” define evidence expectations. Dated
records after the template preserve their original baseline and result; old
TypeScript/SQLite/shadow/default-backend statements are not current Go runtime
architecture evidence.

These scenarios define the minimum evidence families for proving analytix gets
stronger through Kun and Reasonix absorption.

## Product Workflow

| Scenario | Success criteria | Evidence |
| --- | --- | --- |
| Code workspace task | Thread starts, streams, uses tools, edits files, requests approval, and produces reviewable changes. | Test, trace, or desktop QA. |
| Write workspace | Opens workspace, edits Markdown, inline completion works, selected-text actions work, preview/export works. | Test or desktop QA. |
| SDD flow | Requirement draft, plan generation/update, history restore, and requirement-plan link work. | Test or desktop QA. |
| Connect Phone task | Settings save, manual task runs through analytix runtime thread, context preserved. | Test or desktop QA. |
| Schedule task | Scheduled/manual task creates or reuses runtime thread and preserves task context. | Test or desktop QA. |
| Plugin marketplace | Loads, searches/filters, install state renders, current product naming is analytix. | Test or desktop QA. |
| Settings | Top-level `runtime` plus provider/write/speech/media/update/memory/worktree/import settings persist. | Test. |

## Agent Task Success

| Scenario | Success criteria | Evidence |
| --- | --- | --- |
| Small edit with tests | Correct file edit, test command run, final result explains outcome. | Fixture. |
| Multi-file refactor | Search, edit, and review preserve behavior. | Fixture. |
| Failing test repair | Agent diagnoses failing test and repairs only relevant code. | Fixture. |
| Tool approval denied | Denied tool does not run and agent recovers. | Fixture. |
| Tool result file/image | Result path propagates through contracts and transcript. | Fixture. |
| Command failure recovery | Agent handles command failure without corrupting state. | Fixture. |
| Context compaction | Thread continues after compaction with correct history. | Fixture. |
| Fork/resume | Fork and resume preserve transcript and workspace state. | Fixture. |

## Runtime And Provider

| Scenario | Success criteria | Evidence |
| --- | --- | --- |
| Health/readiness | `ANALYTIX_READY` and health route match contract. | Conformance. |
| SSE replay | Events replay from `events.jsonl` without duplication or loss. | Conformance. |
| Session-list sidecar | Thread list reads preview and turn count from metadata sidecars after first legacy backfill, without decoding full message JSONL. | Test + scorecard. |
| Provider URL/body/header | Endpoint format affects URL, headers, body, stream parsing, usage, and reasoning fields correctly. | Conformance. |
| Provider 404 guidance | Error includes sanitized baseUrl, requestUrl, provider, model, endpointFormat, status, and summary. | Test. |
| Cache accounting | Provider-native cache hit/miss fields are parsed when available. | Test. |
| Retry behavior | Retryable failures retry without duplicating durable final events. | Test. |

## Thread Smoothness

| Scenario | Success criteria | Evidence |
| --- | --- | --- |
| 120-turn thread | Long thread loads with virtualized rows and stable scroll. | Renderer smoke + desktop QA. |
| Near-bottom streaming | Active turn follows bottom without layout jumps. | Trace/QA. |
| Away-from-bottom streaming | User reading history is not forced to bottom. | Trace/QA. |
| History prepend | First visible historical row preserves visual offset. | Test/QA. |
| Markdown finalization | Streaming stays lightweight; rich render finalizes after completion or idle budget. | Trace/test. |
| Terminal burst | Terminal output does not freeze composer or thread scroll. | QA/trace. |
| Long reasoning + long final answer | Long reasoning and final answer stream through active-stream row subscriptions without whole-panel or whole-timeline token updates. | `scripts/streaming-ui-benchmark.mjs --gate`. |
| Provider/proxy burst | A batched intermediary is diagnosed separately from renderer hot-path regressions. | `scripts/streaming-ui-benchmark.mjs --include-proxy-burst` and `scripts/model-stream-cadence-probe.mjs`. |
| Side conversation streaming | Right-side active stream renders live text without rewriting the persisted side conversation per token. | Store test + browser benchmark/QA. |

## Safety

| Scenario | Success criteria | Evidence |
| --- | --- | --- |
| Approval required | Restricted tool waits for approval. | Test. |
| Approval denied | Denied tool does not run. | Test. |
| User input cancel | Cancellation resumes or stops the turn according to contract. | Test. |
| Sandbox mode | File/shell behavior respects sandbox. | Test. |
| Trace privacy | No API keys, tokens, full prompts, assistant output, direct personal contact/payment identifiers. | Test/inspection. |
| Legacy import | Legacy import is explicit, idempotent, and isolated. | Test. |

## Scenario Record Template

```text
## YYYY-MM-DD - <scenario>

Batch:
Branch:
Upstream source:
Dimension:
Baseline:
Result:
Evidence:
Score:
Regression:
Follow-up:
```

## 2026-06-21 - Approval/User-Input Abort Cleanup

Batch:
Reasonix approvalManager/control cleanup value behind analytix gates.

Branch:
`codex/reasonix-goal-control-delta-oracle`

Upstream source:
Reasonix approvalManager/control drift previously deferred in the
goal-control governance batch; no public Reasonix protocol imported.

Dimension:
Safety, approval, user input, SSE replay, runtime contract reliability.

Baseline:
Approval waits could remain pending at the gate when a turn was interrupted,
and `request_user_input` abort relied on item finalization without emitting a
closed `user_input_resolved` replay event.

Result:
Pending approvals expire on turn abort, emit `approval_resolved: expired`, and
late allow/deny cannot execute the old tool call. User-input abort now emits
`user_input_resolved: cancelled`; late user-input resolve returns 404. Live
SSE maps expired approvals onto the existing approval card error state.

Evidence:
`ports.test.ts`, `loop.test.ts`,
`approval-user-input-route-oracle.test.ts`, `analytix-mapper.test.ts`, and
G3/G4/G5 conformance focused tests.

Score:
Safety/replay fixture score improves; live renderer-card, live provider/MCP,
Go runtime, and release scores are unchanged.

Regression:
No Reasonix SessionAPI, public approval protocol, top-level Workflow/Subagent
entry, Kun identity, default Go backend, Rust/Tauri path, or provider/cache
request-shape change.

Follow-up:
Renderer approval-card live QA, packaged crash/restart drills, live
provider/MCP matrix, and release packaging gates.

## 2026-06-21 - Post-881 Router Guard / G5 Control / Kun Context Window

Batch:
Reasonix `881b2f2f..9ada1417` currentness, Go G5 executable control shadow, and
Kun 0.2.14 provider default correction.

Branch:
`codex/reasonix-goal-control-delta-oracle`

Upstream source:
Reasonix commits `01d9b173`, `2db7acf6`, merge `9ada1417`; Kun v0.2.14
context-window default behavior.

Dimension:
Runtime classifier governance, Go shadow conformance, provider defaults.

Baseline:
Post-881 auto-plan drift was record-only; Go G5 `controlReplay` was a shadow
summary; shared provider default unknown-model context window was `24_000`.

Result:
Auto-model-router cache keys include a classifier contract fingerprint; Go G5
computes fixture-owned cancel/task-job/step-limit executable cases; shared
provider default context window is `128_000` with explicit overrides preserved.

Evidence:
`auto-model-router.test.ts`, `go-runtime-conformance.test.ts`,
`packages/runtime-go go test ./...`, `app-settings-provider.test.ts`, and
forbidden-surface scans recorded in release evidence.

Score:
Classifier governance and Kun provider-default evidence improve; Go backend
score remains capped because the proof is shadow-only.

Regression:
No Reasonix config/CLI protocol, no top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer, no default Go backend, no Rust/Tauri path, and no
provider/cache request-shape change.

Follow-up:
Approval abort/replay hardening, live provider/cache matrix, live MCP/indexer
QA, packaged desktop QA, and G6 rollback/default-backend evidence.

## 2026-06-20 - Reasonix Session-List Sidecar

Batch:
P0/P2 Reasonix session-sidecar absorption.

Upstream source:
Reasonix `main-v2` target commits `249a4f8`, `73e2025`, and `5db1d0b`;
remote refreshed to `ef7bf97`.

Dimension:
Runtime contract reliability, thread smoothness, maintainability.

Baseline:
Analytix hybrid storage already had metadata JSONL and a rebuildable SQLite
index, but thread summaries did not expose preview/turn counts and filesystem
fallback/backfill could still hydrate messages to rebuild listing summaries.

Result:
Analytix `ThreadSummary` now carries optional `preview`, `turnCount`, and
`messageCount`; hybrid metadata sidecars persist those fields; legacy metadata
is backfilled once; forked sessions seed sidecar summary fields at creation.

Evidence:
`npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts
tests/contracts.test.ts tests/domain.test.ts tests/thread-service.test.ts
--no-file-parallelism --maxWorkers=1`
and `npm run test -- src/renderer/src/agent/analytix-mapper.test.ts`;
`npm --prefix packages/runtime run typecheck`; `npm run typecheck`;
`git diff --check`.

Score:
4 for the scoped session-list/fork sidecar surface after focused automated
tests. This is not a whole-engine Reasonix parity claim.

Regression:
No public Reasonix event/config/CLI shape was introduced. Renderer mapping uses
analytix `CoreThreadSummaryJson`.

Follow-up:
Keep provider/cache/tool-schema P2 work and real Electron sidebar timing QA as
separate batches.

## 2026-06-20 - Reasonix Sidecar Summary Version

Batch:
P2.1 Reasonix sidecar-version and goal-lock refresh.

Upstream source:
Reasonix `main-v2` target commit `7ebb08e`; remote refreshed to
`6d404d80094bb4153bbaec4d7a2739e875d5c8c7`.

Dimension:
Runtime contract reliability, thread smoothness, maintainability.

Baseline:
The first P2 sidecar landing trusted summaries when preview/count fields were
present. That left a zero count ambiguous: it could mean either an authoritative
empty summary or a legacy summary that had not been derived from messages.

Result:
Analytix hybrid sidecar summaries now carry `schemaVersion: 1`. Rebuild/list
paths trust zero-count summaries only when the version marks them authoritative;
legacy/missing-version summaries decode once, append a stamped sidecar summary,
and preserve the thread `updatedAt`.

Evidence:
`packages/runtime/tests/hybrid-store.test.ts` adds fixtures for authoritative
zero-count summaries, legacy missing-version backfill/stamping, and unchanged
activity time. The full P2.1 validation command set is recorded in
`reasonix-sync.md`.

Score:
4 for the scoped sidecar authority behavior after automated tests. This is not
a whole-runtime Go parity claim.

Regression:
No Reasonix `.meta`, CLI, config, event, or product identity was exposed.
Renderer contracts remain analytix-owned.

Follow-up:
Future Go G2 storage/listing must pass the same sidecar-version oracle. Existing
SQLite rows remain rebuildable index entries; this gate applies when rebuilding
from filesystem sidecars and on all new summary writes.
