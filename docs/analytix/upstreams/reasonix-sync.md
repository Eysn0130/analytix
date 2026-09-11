# Reasonix upstream sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"deepseek-reasonix","reviewedCommit":"9eb9511f8b2049a47ee2fef7597256151ac824cb","parity":"not-proven","capabilityBenchmarkV1":null} -->

Reasonix is treated as an agent/runtime upstream for analytix. Its changes may
inform Go runtime implementation, DeepSeek/cache behavior, model adapters,
stream parsing, tool execution, MCP/plugin support, context handling, CLI
engineering, and engine performance.

Reasonix does not decide analytix UI, product identity, settings schema, bridge,
runtime contract, release identity, or desktop QA gates.

This ledger is historical evidence, not a permanent source boundary. Entries
classified as `reject` or "no top-level route" mean the reviewed batch did not
pass the then-current analytix product/contract gate. Future work may reopen
the same capability through a capability-first comparison across all relevant
upstreams, but it must land through an analytix-native spec, contract, UX
rationale, tests, and product-sovereignty scan updates.

Current status as of 2026-07-20: `go-runtime-default` is the current runtime
path and `ANALYTIX_RUNTIME_BACKEND=typescript` remains retired. The older
claim that code-stage absorption was complete is not present-tense readiness
evidence. A fresh white-box audit at `9eb9511` found unresolved cache-release,
tool-authority, MCP-identity, evidence-readiness, and reasoning-publication
gaps. Parity and cache superiority therefore remain `not-proven` until the
deterministic CapabilityBenchmarkV1 and equivalent live provider workload pass.

## Baseline

| Field | Value |
| --- | --- |
| upstream role | Engine capability source. |
| target direction | Absorb Reasonix capabilities into analytix contracts first, then use them to guide Go runtime stages. |
| default landing rule | Adapt engine behavior behind analytix HTTP/SSE or app-server contracts. |

## 2026-07-20 - Current white-box cache and authority audit

| Field | Value |
| --- | --- |
| Branch / commit | `main-v2` / `9eb9511f8b2049a47ee2fef7597256151ac824cb` |
| Audit scope | Boot-prefix stability, canonical tool schemas, provider cache accounting, MCP execution, tool authority, final readiness, recovery, persistence, and reasoning surfaces. |
| Test posture | Current implementation and its tests were inspected at the pinned commit. Prior provider cache measurements are historical inputs; no equal-provider live comparison was run for this refresh. |

Current code evidence and decisions:

| Capability | Current implementation evidence | Decision and analytix requirement |
| --- | --- | --- |
| Byte-stable prefix and schema shape | `internal/boot/boot.go`, `internal/boot/prompt_stability_test.go`, `internal/provider/schema_canonicalize.go`, and `internal/agent/cache_shape.go` stabilize the boot prefix, sort tool schemas, hash prefix components, and attribute provider-native hit/miss tokens. | `adapt`; retain a byte-stable immutable prefix, canonical schemas, native usage fields, and deterministic change attribution. Mutable workspace, evidence, selected text, timestamps, and tool results stay after the stable prefix. |
| Reasoning and cache history | `internal/provider/openai/openai.go` and its tests avoid re-uploading response-only reasoning on applicable provider paths. | `adapt`, but strengthen: reasoning must also be absent from SSE, UI, event logs, durable history, compaction, reports, and exports, not merely omitted from selected provider requests. |
| Release cache guard | `scripts/cache-guard.sh` runs `TestReleaseCacheHitGuard`, but converts `CACHE_GUARD_WARNING` lines to workflow warnings and never enables the strict threshold path. Live cache tests can skip without credentials, and a zero warm hit is not by itself a hard release failure. | `reject` as a release authority. Analytix must fail closed on missing/invalid benchmark evidence and compare the same provider, model, account, request bytes, tool catalog, and workload. A lower token bill or historical percentage is not proof of superiority. |
| Advertised versus executable tools | Portable aliases and execution lookup are not cryptographically or structurally bound to the exact provider-advertised catalog for the turn. | `reject` and benchmark against Analytix `ExecutionGrant`: provider-originated calls must match the frozen advertised name, provider/server identity, schema hash, argument/scope hash, approval, context digest, and expiry. |
| MCP result and identity | The client executes current tools but does not make negotiated `serverInfo`/protocol identity authoritative and does not preserve the full `isError`, `_meta`, `safeToAnswer`, blocker, partial coverage, and evidence-receipt envelope as a host-verified outcome. | `adapt` lifecycle and cache-as-optimization only. A current-run live probe, verified identity, case-bound health, semantic status, and host-issued receipt remain mandatory. Cached catalog data cannot prove availability. |
| Receipt/readiness gate | `internal/evidence/readiness_audit.go`, `internal/agent/final_readiness_test.go`, and related agent paths contain useful readiness accounting, but nil evidence/plan or selected terminal branches can bypass case-grade support; model recovery can still produce the final text. | `adapt` failure/success receipt separation, but replace final authority with the single host Final Evidence Gate. No writer, recovery, loop guard, cancellation, failure, timeout, or empty-evidence path may publish case facts. |
| Reasoning publication | Reasoning is a first-class provider/event/UI concept in current Reasonix paths even though some IM rendering drops it. | `reject` for analytix public and durable surfaces. Provider reasoning may be consumed transiently in memory only and must be zero bytes in accepted events, UI, history, compaction, persistence, reports, and exports. |

The cache comparison gate remains deliberately unclaimed. Analytix may be
declared stronger only after an executable benchmark proves both: (1) no worse
provider-native cache-hit behavior under identical bytes and environment, and
(2) the additional same-thread/turn/case/epoch/snapshot evidence and final
publication invariants. Cache efficiency cannot trade away safety.

## 2026-07-18 - Third source refresh and cache baseline

| Field | Value |
| --- | --- |
| Branch / commit | `main-v2` / `2335d0df9ea4029108ed965f76c2efff30fe6cf4` |
| Previous same-day pin | `40ef98de92a30a273ee582ec682ab338483109d2` |
| Delta | Two commits; 56 files, 4,595 insertions, 205 deletions. |

The range adds fleet scheduling, path-bound tools, profile contracts, write
claims, and desktop settings work. It does not modify Reasonix's cache tests,
but the latest production tool surface is broader than the echo-registry cache
fixture. The strict upstream cache baseline was rerun at this exact commit:
prefix rates `0%, 53%, 68%`, 14-turn peak `93%`, and eight-turn aggregate
`78%` with a final-call rate of `88%`. These numbers are comparison inputs,
not proof that Analytix reaches or exceeds Reasonix. The future deterministic
benchmark must compare equivalent behavior and provider-native hit, miss, and
input tokens without padding or reasoning replay, while preserving zero
reasoning bytes in every Analytix public or durable surface.

## 2026-07-18 - Second source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main-v2` / `40ef98de92a30a273ee582ec682ab338483109d2` |
| Previous same-day pin | `c966d0279629f814731cd39171b1a725ae1ab489` |
| Delta | Eight commits; 124 files, 16,212 insertions, 797 deletions. |

This range contains one agent-path correction that honors
`finish_reason=stop` for a reasoning-only final, plus CLI transcript work and
release, site, and theme changes. The refresh records source identity and
routes the agent correction into the provider/final-readiness comparison; it
does not establish cache superiority, runtime parity, or completed absorption.
Code, tests, and documentation in the range remain subject to file-level
admission and deterministic Analytix benchmarks.

## 2026-07-18 - Earlier same-day currentness delta

| Field | Value |
| --- | --- |
| Branch / commit | `main-v2` / `c966d0279629f814731cd39171b1a725ae1ab489` |
| Previous reviewed pin | `6826411dd158c9594465864090b91fd0f3f86807` |
| Delta | Eight commits; 164 files, 6,152 insertions, 11,873 deletions. |

This fetch removes the Memory v5 execution compiler, revises MCP persistent
session/trust handling, repairs stalled error-body/session-switch behavior, and
contains desktop/TUI history and model-switch fixes. These are current intake
facts only: code, tests, and documentation still require capability-by-
capability admission, and no cache or runtime superiority follows from the
updated pin. Analytix retains the exact `ExecutionGrant`, current-run source
probe, same-case evidence, and final publication requirements while comparing
the new implementation.

## 2026-07-13 - Prior current intake

| Field | Value |
| --- | --- |
| Branch / commit / tag | `main-v2` / `ad9c3fc138b3e7b953405d94b96027b3275c4a50` / dated reviewed checkout |
| License evidence | MIT, pinned license blob `bc45a281d8050c59c9b833ea2d0b1fb6e02602c0`. |
| Admitted posture | File-level source/destination provenance and the Reasonix MIT notice are required before direct or substantial reuse. |

Current delta decisions:

| Capability | Decision |
| --- | --- |
| Cross-process session lease and shared CLI/ACP/serve/desktop lease keeper | `P0 should absorb` through an Analytix persisted-data ownership contract. |
| Uniform atomic write, Windows replace fallback, revision/CAS ledger, conflict recovery branch and recovery GC | `P0 should absorb`; requires migration/crash/contention/Windows tests before touching production data. |
| Event index and bounded recovery | `P0/P1 should absorb`; design together with `events.jsonl`, `messages.jsonl`, highest sequence, SSE cursors, retention, and restart replay. |
| Windows sandbox hardening, plugin packaging, ACP, memory candidates and secret redaction | `P1/P2 compare and absorb selectively` behind existing Analytix product and security boundaries. |
| Reasonix public CLI, SessionAPI/config roots, product UI, or protocol | `reject`; no second product or renderer-visible runtime dialect. |
| Background evidence lease, restart recovery, and delivery-readiness cascade added after `afdcd161` | `adapt as deterministic benchmarks`; host-issued evidence and exact turn/case/epoch/snapshot authority remain mandatory. Reasonix metadata salvage and any `[unverified]` final path are rejected for case facts. |
| DeepSeek cache changes after `afdcd161` | No new mechanism in this range proves a cache advantage over analytix; cache superiority remains `not-proven` until the byte-stable prefix and tail-hit benchmark passes. |

Current Analytix risk: `json_map.go` still publishes JSON through direct
`os.WriteFile`, durable coordination is process-local, and large event/search
paths still scan full JSONL. A separate persisted-data change must solve this;
this audit does not mutate persisted formats.

Existing Analytix files `internal/diff/diff.go`, `internal/netclient/netclient.go`,
and `internal/provider/provider.go` remain in the provenance review queue. Their
similarity alone is not proof of copying, but any confirmed Reasonix-derived
material must receive the required MIT notice before release.

## Review Checklist

```text
1. Which Reasonix release/tag/commit range was reviewed?
2. Which engine capabilities changed?
3. Does the change affect request URL/body/header behavior?
4. Does the change affect stream parsing, usage parsing, reasoning fields, retries, or cache accounting?
5. Does the change affect tool calling, approvals, user input, MCP, shell/file/diff/apply?
6. Does the change require a contract change or only an implementation change?
7. Can the behavior be captured in TypeScript golden fixtures before Go implementation?
8. What conformance tests prove equivalence?
```

## 2026-06-23 - D-0246 AutoResearch Go Runtime Project State

Source:

```text
Reasonix upstream: https://github.com/esengine/DeepSeek-Reasonix
Local comparison checkout: /Users/sun/Projects/DeepSeek-Reasonix
Pinned comparison commit: 7032f39336f4ae5f216e1fcb3368e5f679723490
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| AutoResearch project-local state | `contract-reimplement` | Accept under the existing `/goal --research` turn/SSE contract. The Go runtime writes `.analytix/autoresearch/<threadId>/task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`. |
| Stale/pivot audit | `contract-reimplement` | Emit `autoresearch_state_audit` as an analytix-owned audit-only runtime event. The reducer ignores it for UI projection; SSE replay persists it across restart. |
| Unknown requirement evidence | `contract-reimplement` | Reject unknown requirement ids before writing findings. |
| Reasonix AutoResearch product protocol | `reject` | Do not add `/v1/autoresearch`, `REASONIX.md`, `AGENTS.md`, Reasonix config roots, stable-prefix/tool-schema state, or a top-level AutoResearch UI. |

Evidence:

```text
packages/runtime-go/internal/research/autoresearch_state.go
packages/runtime-go/d0246_autoresearch_state_test.go
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
packages/runtime/src/contracts/events.ts
packages/runtime/tests/contracts.test.ts
packages/runtime/tests/runtime-event-reducer.test.ts
packages/runtime/tests/go-runtime-conformance.test.ts
scripts/scan-product-sovereignty.cjs
```

Boundary:

```text
D-0246 clears the D-0244/D-0245 AutoResearch deferred row. It does not change
ordinary /goal semantics, does not affect non-research turns, does not narrow
non-DeepSeek providers, and does not make Go the default backend.
```

## 2026-06-23 - D-0245 Baseline-Governed Reasonix Absorption

Source:

```text
Reasonix upstream: https://github.com/esengine/DeepSeek-Reasonix
Local comparison checkout: /Users/sun/Projects/DeepSeek-Reasonix
Pinned comparison commit: 7032f39336f4ae5f216e1fcb3368e5f679723490
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Multi-model runtime turn routing | `contract-reimplement` | Accept into Go runtime server under analytix `/v1/threads/:id/turns` and SSE contracts. DeepSeek, OpenAI-compatible, Anthropic-compatible, and `custom_endpoint` are all covered by fake-provider tests and TS conformance. |
| DeepSeek cache/prefix optimization | `code-port-and-adapt` | Accept as provider-specific enhancement only. DeepSeek receives `thinking`/reasoning/cache diagnostics; OpenAI-compatible, Anthropic-compatible, and `custom_endpoint` do not inherit DeepSeek-only request fields or cache diagnostics. |
| Goal evidence audit | `contract-reimplement` | Keep as audit/evidence quality event. It does not replace `create_goal`, `get_goal`, `update_goal`, renderer goal actions, or goal resume semantics. |
| MCP lifecycle/search/call/reconnect | `contract-reimplement` | Keep as internal analytix runtime lifecycle audit and tool-catalog event, not a public MCP-indexer product route. |
| Sub-agent/job lineage | `contract-reimplement` | Keep as parent goal/thread lineage in runtime events. Do not expose a top-level Subagent UI or public Reasonix job protocol. |
| AutoResearch project state | `contract-reimplement` | D-0246 lands Go project-local `.analytix/autoresearch/<threadId>/` state and `autoresearch_state_audit` under existing `/goal --research` turn/SSE contract, without a top-level AutoResearch entry. |
| Reasonix SessionAPI/config/CLI/product identity | `reject` | Reject because it would replace the Kun/Analytix product baseline instead of enhancing the engine. |

Evidence:

```text
packages/runtime-go/internal/upstreamaudit/baseline_absorption.go
packages/runtime-go/d0245_baseline_absorption_test.go
packages/runtime-go/internal/research/autoresearch_state.go
packages/runtime-go/d0246_autoresearch_state_test.go
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
packages/runtime/src/contracts/events.ts
packages/runtime/src/contracts/capabilities.ts
packages/runtime/tests/go-runtime-conformance.test.ts
packages/runtime/tests/runtime-event-reducer.test.ts
scripts/scan-product-sovereignty.cjs
```

Boundary:

```text
Reasonix DeepSeek capability absorption is provider-specific enhancement, not
provider narrowing. Kun/Analytix remains the product, UI, settings, bridge,
runtime contract, slash-command, /goal, desktop, and release baseline.
```

## 2026-06-23 - D-0244 Reasonix Capability Red Matrix

Source:

```text
/Users/sun/Projects/DeepSeek-Reasonix
7032f39336f4ae5f216e1fcb3368e5f679723490
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Provider/cache streaming | `code-port-and-adapt` | Mark green with Go provider client/runtime turn/SSE evidence. |
| DeepSeek prefix cache | `code-port-and-adapt` | Mark green with runtime cache diagnostics and D-0243 benchmark record evidence. |
| Approval/user-input gates | `contract-reimplement` | Mark green with analytix approval/user-input routes, answer-free replay, and restart recovery. |
| MCP lifecycle/search/call | `contract-reimplement` | Mark green with Go live-local MCP lifecycle audit covering initialize, namespaced tools, search, approval denied no-execute, approved call, reconnect, schema stability, and credential redaction under the runtime SSE contract. |
| Job/sub-agent lineage | `contract-reimplement` | Mark green for runtime-only parent thread/goal lineage in SSE; no public sub-agent protocol. |
| Durable replay and crash/restart | `contract-reimplement` | Mark green through Go durable store, candidate root, `highestSeq`, pending gate recovery, and turn sequence continuation. |
| Candidate rollback | `replace` | Mark green as the runtime-server contract plus adapter rollback replaces D-0241 proof-route reliance. |
| Goal evidence FSM | `contract-reimplement` | Mark green with Go goal evidence audit kernel, command-match/project-check conformance, durable SSE block/recovery event, and restart replay evidence. |
| AutoResearch project-local state | `contract-reimplement` | Mark green with D-0246 Go store/helper, five required project-local state files, unknown-requirement rejection, stale/pivot `autoresearch_state_audit`, and restart replay evidence. |
| Reasonix public protocol/config/identity | `reject` | Continue rejecting SessionAPI, config roots, CLI identity, MCP-indexer public route, and public job/sub-agent protocol. |

Evidence:

```text
packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go
packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go
packages/runtime-go/d0244_reasonix_red_matrix_test.go
packages/runtime-go/internal/goal/goal_evidence.go
packages/runtime-go/d0244_goal_evidence_test.go
packages/runtime-go/internal/mcp/lifecycle.go
packages/runtime-go/d0244_mcp_lifecycle_test.go
packages/runtime-go/internal/research/autoresearch_state.go
packages/runtime-go/d0246_autoresearch_state_test.go
packages/runtime-go/runtime_server.go
packages/runtime/src/contracts/capabilities.ts
packages/runtime/src/contracts/events.ts
packages/runtime/tests/go-runtime-conformance.test.ts
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts -t "runs the D-0242 Go runtime server"
/tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Boundary:

```text
D-0244's red/deferred capability rows are now clear after D-0246, but this is
not default-backend readiness. D-0243 still gates Go defaulting on durable
restart, provider matrix, MCP matrix, packaged QA, and explicit readiness envs
without adding Reasonix protocol, UI entries, or product identity.
```

## 2026-06-23 - D-0243 G6 Readiness Hardening Slice

Source:

```text
Reasonix cache-first provider discipline, env/keyring credential boundary,
MCP connect/search/call/reconnect/redaction behavior, restart/crash recovery
expectations, and analytix D-0242 runtime server contract.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Credentialed provider matrix | `contract-reimplement` | Add an analytix-owned matrix scaffold for DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom endpoint modes. Fake probes run locally; credentialed probes are env-gated and skip when missing. |
| DeepSeek cache benchmark record | `code-port-and-adapt` | Preserve Reasonix cache-first prefix discipline as a repeatable DeepSeek cache hit/prefix-stability record format without putting dynamic state in the stable prefix. |
| MCP matrix | `contract-reimplement` | Run fake MCP connect/search/call/reconnect/approval/redaction proof and keep optional credentialed MCP as env-gated skip. No MCP-indexer public route is added. |
| Crash/restart durable recovery | `contract-reimplement` | Add candidate durable root mode and restart drill for `events.jsonl`, `highestSeq`, replay, pending approval/user-input recovery, and turn sequence continuation. |
| G6 readiness gate | `contract-reimplement` | Main adapter computes D-0243 readiness status, defaults `ready:false`, and rolls back to TypeScript unless durable/provider/MCP/packaged QA evidence is passed. |
| Reasonix public protocol/config roots | `reject` | Do not import Reasonix SessionAPI, config roots, product identity, MCP-indexer route, or public subagent/job protocol. |

Evidence:

```text
packages/runtime-go/g6_readiness.go
packages/runtime-go/g6_readiness_test.go
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
packages/runtime/tests/go-runtime-conformance.test.ts
src/main/runtime/analytix-adapter.ts
src/main/runtime/analytix-adapter.test.ts
scripts/d0243-packaged-go-runtime-qa.mjs
```

Boundary:

```text
D-0243 makes G6 hard gates executable and skippable where external credentials
or packaged app state are missing. It does not claim G6/default backend,
release readiness, live superiority, or TypeScript retirement.
```

## 2026-06-23 - D-0242 Go Runtime Server Contract Slice

Source:

```text
Reasonix provider/cache, approval/input gate, MCP lifecycle, and job-lineage
discipline as already isolated in D-0241; analytix runtime HTTP/SSE contract;
D-0236 durable store; D-0238 main adapter rollback gate.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Runtime server binary | `contract-reimplement` | Add `packages/runtime-go/cmd/runtime-server` serving an analytix `/v1/*` contract subset behind an internal gate, not Reasonix public routes. |
| Provider/cache through turn contract | `code-port-and-adapt` | Use D-0241 `GoHTTPProviderClient` as the turn model backend, preserving DeepSeek native cache precedence and provider-family-aware OpenAI/Anthropic/custom handling. |
| Durable event/session replay | `contract-reimplement` | Make thread, turn, and SSE replay read events written by the Go contract server's durable sink. |
| Approval/user-input gate | `contract-reimplement` | Expose minimal analytix approval and user-input contract routes; deny prevents execution and submitted answers are excluded from replay events. |
| MCP/job lineage | `contract-reimplement` | Emit fake MCP result and parent goal/thread-bound job lineage as runtime events/capabilities without top-level product entries. |
| Reasonix SessionAPI/config/public routes | `reject` | Keep SessionAPI, config roots, public subagent/job protocol, and Reasonix identity out of public contracts. |
| Default Go backend / TypeScript retirement | `absorbed` | Go runtime is the default core after deterministic local gates; live credentialed matrices, packaged QA, rollback drills, and crash recovery remain post-cutover validation, while TypeScript is explicit rollback only. |

Evidence:

```text
packages/runtime-go/cmd/runtime-server
packages/runtime-go/runtime_server.go
packages/runtime-go/runtime_server_test.go
packages/runtime/tests/go-runtime-conformance.test.ts
src/main/runtime/analytix-adapter.ts
src/main/runtime/analytix-adapter.test.ts
```

Boundary:

```text
D-0242 moves D-0241 components into an analytix runtime contract subset. It is
not a Reasonix protocol import, not a default backend switch, not packaged
release readiness, and not a renderer-visible subagent/MCP/backend surface.
```

## 2026-06-23 - D-0241 Production-Candidate Go Runtime Parity Slice

Source:

```text
Reasonix provider/cache request and stream handling, native DeepSeek cache
telemetry precedence, approval/input gate discipline, MCP lifecycle patterns,
job/sub-agent lineage rules, and analytix D-0236 durable store plus main
adapter gate.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Provider client abstraction | `code-port-and-adapt` | Add `GoHTTPProviderClient` with DeepSeek/OpenAI-compatible/Anthropic-compatible fake live HTTP/SSE tests; keep credentials disabled and no external network. |
| DeepSeek cache and prefix stability | `code-port-and-adapt` | Prefer DeepSeek native cache hit/miss fields when present, preserve stable prefix-shape hashing, and keep OpenAI/Anthropic/custom endpoint parsing provider-family-aware. |
| Durable event/session replay | `contract-reimplement` | Write turn, delta, usage, and completion events through the D-0236 Go durable sink, then replay from the same sink instead of consuming fixture event drafts. |
| Approval/user-input manager | `contract-reimplement` | Implement pending, deny, submit, cancel, timeout, and replay; denied tools are not executed and submitted answers are not persisted in replay events. |
| MCP manager shape | `contract-reimplement` | Use a local fake MCP transport for connect/search/call/disconnect lifecycle proof; do not read credentials or expose an MCP-indexer product route. |
| Job/sub-agent lineage | `contract-reimplement` | Keep job/sub-agent capability runtime-only and bound to parent goal/thread lineage; no Reasonix public subagent protocol enters renderer/main contracts. |
| Reasonix SessionAPI/config/public routes | `reject` | Keep all D-0241 routes under analytix internal `/v1/internal/go-production-candidate/*` and no renderer-visible Go route. |
| Default Go backend / TypeScript retirement | `defer` | Require G6 packaged desktop QA, crash recovery, credentialed provider/MCP matrix, rollback drills, and TypeScript retirement migration before default selection. |

Evidence:

```text
packages/runtime-go/live_production_candidate.go
packages/runtime-go/live_production_candidate_test.go
packages/runtime/tests/go-production-candidate-conformance.test.ts
src/main/runtime/analytix-adapter.ts
src/main/runtime/analytix-adapter.test.ts
```

Boundary:

```text
D-0241 is production-candidate runtime code behind an internal gate. It is not
Reasonix public protocol import, not packaged release readiness, not a
renderer-visible subagent/MCP/backend surface, and not a default Go backend.
```

## 2026-06-23 - D-0240 G6 Retirement Cleanup Proof

Source:

```text
D-0238 backend-neutral gate, D-0239 Go kernel scaffold, Reasonix route/job
boundary review, analytix runtime sovereignty specs, and Kun product baseline.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Post-G6 delete list | `contract-reimplement` | Add `deleteAfterG6` fixture evidence for internal conformance env gates, `/v1/conformance/*` route families, kernel/minimal-loop oracles, and shadow-only G5 replay after default-backend proof. |
| Retained current default paths | `contract-reimplement` | Explicitly retain the TypeScript runtime package, main process supervisor, and single backend-neutral adapter until G6 and retirement migration pass. |
| Forbidden redundancy matrix | `reject` | Keep deprecated bridge aliases, duplicate settings schemas, renderer-visible Go routes, UI runtime switchers, and Reasonix public protocols absent. |
| G6 blocker ledger | `defer` | Keep production Go loop/provider/MCP/job managers, packaged desktop route QA, crash/rollback drills, credentialed provider/MCP matrix, and TypeScript retirement migration as open G6 requirements. |
| Immediate TypeScript runtime deletion | `reject` | Do not delete the production default runtime while Go is still conformance-only. |

Evidence:

```text
packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-oracle.json
packages/runtime-go/live_local_kernel.go
packages/runtime-go/live_local_kernel_test.go
packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts
scripts/scan-product-sovereignty.cjs
```

Boundary:

```text
D-0240 is cleanup and retirement planning evidence. It is not G6, not a
default-backend switch, not a live Reasonix protocol import, not production Go
manager parity, and not TypeScript runtime retirement.
```

## 2026-06-23 - D-0239 Go Kernel Live Scaffold

Source:

```text
Reasonix `internal/jobs`, `internal/agent/subagent_store_test.go`,
`internal/plugin/lazy.go`, `internal/plugin/canonicalize.go`,
`internal/provider`, D-0236 durable store, D-0237 minimal loop, and D-0238
main adapter gate.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Session-scoped Job Manager | `contract-reimplement` | Add analytix-owned Go kernel job-orchestration fixture under `/v1/conformance/kernel/job-orchestration`; no production job execution yet. |
| Subagent continue/fork lineage guard | `contract-reimplement` | Represent same-parent continue, sibling fork rejection, failed transcript reuse rejection, and stale-running interruption as parent-goal-bound fixture evidence. |
| Loop controller and durable event sink | `contract-reimplement` | Combine D-0236 durable store and D-0237 minimal loop into the kernel capability map. |
| MCP lazy catalog recovery | `code-port-and-adapt` | Map Reasonix lazy/catalog recovery and canonicalization lessons to G4 MCP catalog/search/reconnect plus kernel component evidence. |
| Provider-aware cache accounting | `code-port-and-adapt` | Map Reasonix provider/cache discipline to G3 cache-accounting component evidence with DeepSeek/OpenAI/Anthropic coverage. |
| Reasonix SessionAPI/config/public routes | `reject` | Keep kernel evidence under analytix `/v1/conformance/kernel/*`; no upstream route, config root, or renderer-visible protocol. |
| Live production Go job/subagent execution | `defer` | Keep fixture-only until permission gates, packaged QA, crash recovery, and G6 rollback evidence exist. |

Evidence:

```text
packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-oracle.json
packages/runtime-go/live_local_kernel.go
packages/runtime-go/live_local_kernel_test.go
packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts
```

Boundary:

```text
D-0239 is a Go live-local kernel scaffold and absorption proof. It is not a
Reasonix SessionAPI import, not live job/subagent execution, not a Go default
backend, not packaged route QA, and not a top-level Subagent/Workflow entry.
```

## 2026-06-23 - D-0238 Backend-Neutral Adapter Gate

Source:

```text
Reasonix backend/controller separation, D-0236 temp durable store, D-0237
minimal loop sidecar, and analytix main runtime adapter contracts.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Backend-neutral main adapter | `contract-reimplement` | Add an analytix-owned runtime backend gate in `src/main/runtime/analytix-adapter.ts`; Go runtime is default. |
| Internal Go conformance launch | `code-port-and-adapt` | Historical gate is superseded by Go default delivery. The remaining `cmd/contract-sidecar` entry is test/conformance-only and disabled for packaged Analytix; production launches `cmd/runtime-server`. |
| Runtime token alignment | `code-port-and-adapt` | Add `--runtime-token` to the Go sidecar so internal gates use the analytix runtime bearer token instead of fixture-only `tok-1`. |
| Canary boundary | `contract-reimplement` | Require `/health`, `/v1/threads?limit=1`, fixture-only `/v1/conformance/loop/boundary`, and hidden `/v1/runtime/go` before accepting the Go sidecar. |
| Rollback | `contract-reimplement` | Historical D-0238 behavior stopped Go, cleaned temp durable state, recorded fallback reason, and returned to the then-default TypeScript runtime. Current analytix startup fails closed; `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic and must not start TS runtime. |
| Reasonix public SessionAPI/config/routes | `reject` | Do not expose upstream protocol, Go renderer route, settings root, public backend switcher, or product identity. |
| Live Go provider/MCP/job/backend parity | `defer` | Keep this as an internal conformance gate until production Go loop/provider/MCP/job managers and packaged QA exist. |

Evidence:

```text
npm run test -- src/main/runtime/analytix-adapter.test.ts --run
npm run typecheck
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Boundary:

```text
D-0238 starts a Go live-local scaffold from Electron main only through an
internal/test/conformance gate. It is not default backend readiness, G6,
Reasonix SessionAPI, live provider superiority, live MCP parity, or a
renderer-visible Go route.
```

## 2026-06-23 - D-0237 Go Minimal Agent Loop Skeleton

Source:

```text
Reasonix agent-kernel loop-controller/event-sink/cache-accounting discipline,
analytix TypeScript `AgentLoop`/`RuntimeEventRecorder` contracts, G2 route
state, G3 provider/cache fixture, G4 approval/user-input/MCP manager fixture,
and the D-0236 temp durable event/session store.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Minimal loop controller state | `contract-reimplement` | Add a conformance-only Go loop route that consumes a fixture-scripted model turn, step-limit proof, cancel proof, and resume proof without creating a production Job Manager. |
| Event sink / persist-before-publish | `contract-reimplement` | Record every loop event through the D-0236 temp durable store before JSON replay or SSE, proving replay can reconstruct the same recovered state. |
| Stable prefix and model request shape | `contract-reimplement` | Pin the stable system prefix, tool catalog fingerprint, DeepSeek chat-completions request shape, and dynamic-state exclusion to TS-owned provider/cache fixtures. |
| Provider-aware cache telemetry | `contract-reimplement` | Preserve DeepSeek, OpenAI Responses, and Anthropic Messages cache hit/miss telemetry through loop events and recovered durable state without changing custom endpoint behavior. |
| Approval/user-input gate replay | `contract-reimplement` | Replay denied approval with no tool execution, submitted user-input with answer-free event persistence, and cancelled user-input gate state. |
| MCP catalog recovery | `contract-reimplement` | Expose MCP catalog visibility from fixture data while keeping MCP connection and credential reads at `0`. |
| Reasonix public protocol, SessionAPI, config root, UI control plane | `reject` | Keep all loop proof under analytix-owned `/v1/conformance/loop/*` routes and do not expose renderer-visible Go routes. |
| Live provider superiority, live MCP parity, production backend, G6 | `defer` | D-0237 is a minimal conformance proof only; default backend and release readiness remain future gates. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-minimal-agent-loop-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Boundary:

```text
This supports Reasonix-style loop-controller, event-sink, cache, gate replay,
and MCP catalog recovery behind analytix contracts. It is not Reasonix
SessionAPI/public protocol, not a live Go agent backend, not live provider/MCP
parity, not default backend, not G6, and not release readiness.
```

## 2026-06-23 - D-0236 Go Temp Durable Event/Session Store + SSE Replay

Source:

```text
Reasonix durable agent-kernel/session recovery discipline, analytix
RuntimeEventRecorder/FileSessionStore/SSE route contracts, G2 route oracle, and
the D-0235 follow-up gate to move Go live-local evidence from in-memory manager
replay toward an isolated durable runtime base.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Temp durable `events.jsonl` append/replay | `contract-reimplement` | Add an opt-in Go conformance store that writes only under a test-provided temp dir, appends newline-terminated JSONL, assigns stable per-thread `seq`, returns persisted max `highestSeq`, and filters/sorts replay after `seq`. |
| Malformed event recovery | `contract-reimplement` | Skip bad JSONL lines during replay while returning line/path/error/preview diagnostics. |
| SSE replay cursor semantics | `contract-reimplement` | Prove caught-up replay is empty and `Last-Event-ID` is equivalent to `since_seq` for persisted events. |
| Thread/session temp durable state | `code-port-and-adapt` | Persist test-side thread list/search/archive/fork/resume state under `/v1/conformance/durable/*` without touching production Electron or TS runtime stores. |
| Cache/accounting recovery | `contract-reimplement` | Preserve DeepSeek, OpenAI Responses, and Anthropic Messages cache hit/miss telemetry through durable replay. DeepSeek handling remains provider-aware and does not alter OpenAI/Anthropic/custom endpoint contracts. |
| Approval/user-input/MCP replay recovery | `contract-reimplement` | Reconstruct approval denial, submitted user-input gate status without persisted answers, and MCP tool-catalog state from durable events while keeping execution/connect/credential attempts at `0`. |
| Reasonix public protocol, SessionAPI, config root, UI control plane | `reject` | Keep all durable proof under analytix-owned conformance routes and `window.analytix` / `analytix serve` boundaries. |
| Live Go agent loop, production durable backend, default Go backend | `defer` | D-0236 is a reversible temp durable base only; G6/default backend and live loop remain future gates. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-durable-sidecar-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

Boundary:

```text
This supports Reasonix-style durable agent-kernel recovery behind analytix
contracts. It is not Reasonix SessionAPI/public protocol, not a production Go
runtime store, not a live provider/MCP/gate backend, not G6, not release
readiness, and not a default backend.
```

## 2026-06-23 - D-0235 Go G4 Approval/User-Input/MCP Manager Live-Local Sidecar

Source:

```text
Reasonix approval/user-input/MCP manager discipline, analytix
approval-user-input and MCP lifecycle oracles, and the D-0234 follow-up gate to
move Go live-local evidence from fixture-backed G3 provider/cache streaming
replay into isolated G4 manager replay.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Approval deny no-execute replay | `contract-reimplement` | Add local Go conformance routes that accept the TS-owned denial body, return the oracle status/body, reject a second decision, and keep approval/tool execution attempts at `0`. |
| User-input cancel/submit/validation replay | `contract-reimplement` | Replay cancel and submit route status/body shapes, structured invalid-choice cases, answer echo in HTTP response, answer-free resolved event evidence, and remote `disableUserInput` preservation. |
| MCP lifecycle/search/reconnect diagnostics replay | `contract-reimplement` | Replay connect/reload/disconnect/cancel/error, approval annotations, search meta-tools, trusted/untrusted workspace boundary, call-time reconnect classification, background reconnect classification, known override diagnostics, and secret redaction from TS-owned fixtures. |
| G4 live-local isolation counters | `code-port-and-adapt` | Add `LiveLocalSidecarSnapshot` G4 replay counters while keeping `ApprovalExecutionAttempts`, `ToolExecutionAttempts`, `MCPConnectionAttempts`, `MCPCredentialAttempts`, `CredentialReadAttempts`, `FileMutationAttempts`, `EventsJSONLWriteAttempts`, and `RealWorkspaceWriteAttempts` at `0`. |
| Reasonix public control plane, SessionAPI, MCP-indexer product route | `reject` | Do not expose upstream protocol, route names, config roots, renderer-visible Go routes, or `analytix serve` replacement. |
| Live Go MCP/approval/user-input parity | `defer` | Fixture-backed local replay cannot claim live Go MCP client parity, approval execution parity, credentialed MCP matrix, packaged QA, G6, or release readiness. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Boundary:

```text
This moves Go live-local evidence from G3 provider/cache streaming replay to G4
isolated approval/user-input/MCP manager replay. It is not a live Go MCP
client, not live approval/user-input backend parity, not G6, not release
readiness, not Electron integration, not Reasonix public protocol, and not a
default backend.
```

## 2026-06-23 - D-0234 Go G3 Provider/Cache Streaming Live-Local Sidecar

Source:

```text
Reasonix provider/cache diagnostics discipline, analytix provider-cache oracle,
and the D-0233 follow-up gate to move Go live-local evidence from isolated G2
lifecycle replay into fixture-backed G3 provider/cache streaming replay.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Fixture-backed provider usage parser | `code-port-and-adapt` | Add a local Go harness route that parses the 5 TS-owned provider usage payloads without reading credentials or calling external providers. |
| Provider request-shape replay | `contract-reimplement` | Recompute all 7 DeepSeek/OpenAI-compatible/Responses/Anthropic/custom full endpoint URL/header/body/tool-shape cases through analytix-owned helpers. |
| Streaming event order replay | `contract-reimplement` | Replay the G3 SSE frames in order `item_delta -> usage -> turn_completed` and compare the usage event to `deepseek-prompt-cache`. |
| Cache accounting and drift attribution | `contract-reimplement` | Preserve DeepSeek native hit/miss precedence, OpenAI Responses cached tokens, Anthropic cache read/create accounting, unsupported unknown behavior, aggregate hit/miss, stable-prefix, and drift attribution. |
| Diagnostics privacy | `contract-reimplement` | Return bounded cache hashes and provider/model/endpoint attribution while keeping prompt text, tool text, API-key, and Authorization substrings out of diagnostics. |
| Reasonix public provider/cache protocol, SessionAPI, config roots | `reject` | Do not expose upstream protocol, route names, config shape, renderer-visible Go route, or `analytix serve` replacement. |
| Live provider/cache superiority and credentialed provider matrix | `defer` | Fixture-backed local HTTP replay cannot claim live external provider quality or release readiness. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Boundary:

```text
This moves Go live-local evidence from G2 lifecycle replay to G3
provider/cache streaming replay. It is not a live Go provider client, not G6,
not release readiness, not Electron integration, not Reasonix public protocol,
and not live provider/cache superiority.
```

## 2026-06-23 - D-0233 Go Isolated Mutating G2 Lifecycle Sidecar

Source:

```text
Reasonix sidecar/runtime serve discipline, analytix G2 route replay oracle, and
the D-0232 follow-up gate to admit mutating routes only inside isolated
test/conformance state.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Isolated mutable sidecar store | `code-port-and-adapt` | Add an in-memory G2 lifecycle store behind `NewLiveLocalSidecarHarness`; keep it in `packages/runtime-go` tests only. |
| Thread archive/update/fork/resume lifecycle | `contract-reimplement` | Execute the four G2 mutating oracle routes with exact TS-owned status/body shape. |
| Sidecar proof discipline | `contract-reimplement` | Record mutation ids, archived thread, fork id, and resumed session in `LiveLocalSidecarSnapshot`; prove temp `events.jsonl` is untouched. |
| G5 rollback/default-backend guards | `contract-reimplement` | Keep `electronMainConnected`, `defaultGoBackendEnabled`, and `rendererVisibleGoRoutesAllowed` false. |
| Reasonix public protocol / SessionAPI / default backend | `reject` | Do not expose upstream route names, config roots, event shapes, renderer-visible Go routes, or `analytix serve` replacement. |
| Provider live call, approval execution, MCP credential, file mutation | `defer` | Keep outside this slice until later G5/G6 gates and credentialed/package QA exist. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```

Boundary:

```text
This moves Go live-local from read-only G1/G2 replay to an isolated mutating G2
lifecycle prototype. It is not a live Go backend, not G6, not release
readiness, not Electron integration, and not live provider/MCP, approval
execution, or file mutation parity.
```

## 2026-06-23 - D-0232 Go Live-Local Sidecar Prototype

Source:

```text
Reasonix sidecar/runtime serve discipline, analytix G1/G2/G5 oracle fixtures,
and the post-881 requirement to start a live-local sidecar prototype without
making Go the default backend.
```

Classification:

| Item | Class | analytix decision |
| --- | --- | --- |
| Live-local HTTP sidecar harness | `code-port-and-adapt` | Add `packages/runtime-go/NewLiveLocalSidecarHandler` as a Go test/conformance-only local HTTP handler. |
| Health/runtime info/tools inventory | `contract-reimplement` | Serve G1 `/health`, `/v1/runtime/info`, and `/v1/runtime/tools` responses from the TypeScript-owned oracle fixture. |
| Read-only route/SSE replay | `contract-reimplement` | Replay only G2 `GET` routes and fixture SSE frames; filter mutating `PATCH`/`POST` route fixtures from the sidecar. |
| G5 rollback/default-backend guards | `contract-reimplement` | Reuse G5 product-boundary expectations so `electronMainConnected`, `defaultGoBackendEnabled`, and `rendererVisibleGoRoutesAllowed` remain false. |
| Reasonix public protocol / SessionAPI / default backend | `reject` | Do not expose upstream route names, config roots, event shapes, renderer-visible Go routes, or `analytix serve` replacement. |
| Provider live call, approval execution, MCP credential, file mutation | `defer` | Keep outside this slice until later G5/G6 gates and credentialed/package QA exist. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Boundary:

```text
This moves Go from pure shadow replay to a live-local prototype only for the
read-only fixture-backed sidecar slice. It is not a live Go backend, not G6,
not release readiness, not Electron integration, and not live provider/MCP or
approval execution parity.
```

## 2026-06-20 - Reasonix main-v2 code-level audit

Source:

```text
Reasonix version/tag: main-v2 snapshot
Commit range: 33545f6
Release notes: not used for this audit; repository source and README/docs were reviewed
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
Reasonix is the engine/runtime upstream. The reviewed tree confirms the main
absorption value is Go runtime structure, transport-neutral control layer,
provider/cache behavior, MCP/plugin lifecycle, built-in tools, permissions,
sandboxing, checkpoint/rewind, memory/retrieval/skills, bot runtime ideas, and
benchmark/release discipline.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `internal/control`, `internal/serve` | runtime boundary | `must absorb` | Port controller and HTTP/SSE ideas behind analytix contracts. | Add conformance fixtures before Go backend stages. |
| `internal/agent` | agent loop | `must absorb` | Use Go engine patterns for cache, compaction, coordination, usage, guards, and subagents. | Compare against TypeScript oracle and benchmark. |
| `249a4f8` session-list sidecar metadata | runtime/history performance | `must absorb` | Adapt sidecar cached preview/turn-count behavior into analytix session/thread listing. | Add session-list benchmark and legacy backfill fixture. |
| `73e2025` fork/branch sidecar seeding and goal-state warnings | runtime/history reliability | `must absorb` | Seed fork/branch previews and turn counts; surface persistence failures in sanitized logs. | Add fork/branch sidecar and persistence-warning fixtures. |
| `5db1d0b` redundant disk cache removal | runtime/history simplification | `must absorb` | Remove equivalent redundant project-session disk cache when sidecar-only listing makes it unnecessary. | Add cache invalidation and long-history listing benchmark. |
| `internal/provider/*` | provider/cache | `must absorb` | Port request/stream/usage/cache parsing ideas into analytix provider contract. | Expand provider URL/body/stream/usage matrix. |
| `internal/plugin`, `internal/tool`, `internal/tool/builtin` | tools/MCP | `should absorb` | Port plugin lifecycle and tool semantics with stricter analytix approvals and sandboxing. | Add lifecycle, cancellation, and protected-path tests. |
| `internal/permission`, `internal/sandbox`, `internal/checkpoint` | safety | `should absorb` | Integrate stricter permission and checkpoint/rewind behavior with desktop review UI. | Add safety and rewind benchmarks. |
| `internal/bot`, `internal/botruntime` | remote entry | `optional` | Use as engine reference for Connect Phone and scheduled remote tasks. | Require product-flow redesign before UI exposure. |
| `benchmarks`, `cmd/e2ebench`, packaging | benchmark/release | `should absorb` | Reuse benchmark discipline and Go binary packaging ideas under analytix identity. | Add executable benchmark and release checks later. |
| Reasonix public CLI/settings/event identity | identity/protocol | `reject` | Do not expose Reasonix product identity or native event/settings shapes as analytix surface. | Naming and contract scans remain mandatory. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No contract change accepted by this audit; future code must adapt behind analytix routes. | Renderer remains backend-neutral. | G0-G6 conformance when implemented. |
| Provider contract | Future implementation may improve parsing and cache accounting without changing shared schema unless explicitly designed. | Existing provider settings remain top-level `runtime`. | Provider matrix and sanitized error guidance tests. |
| Tool contract | Future implementation may improve lifecycle/cancellation; UI approval semantics stay analytix-owned. | Tool events and file/image result contracts remain analytix-shaped. | Tool lifecycle and approval tests. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0/G1/G2 | Reasonix `internal/control` and `internal/serve` inform scaffold and event replay. | Health/session/event/approval fixtures. |
| G3/G4 | Reasonix `internal/tool`, `internal/plugin`, `internal/permission`, `internal/sandbox` inform tools and approvals. | Tool lifecycle, cancellation, sandbox, user input. |
| G5/G6 | Reasonix `internal/agent`, `internal/provider`, memory/retrieval/skills inform full agent parity. | Agent, cache, provider, long-session, desktop QA benchmarks. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix Go engine vs analytix TypeScript runtime | Historical: TypeScript runtime was the oracle until conformance proved Go parity. Current: Go runtime is the only production agent runtime. | `go-runtime-conformance.md` |
| Reasonix terminal/product assumptions vs analytix desktop workbench | analytix desktop UI and contracts win. | `code-level-absorption-blueprint.md` |

Absorbed:

```text
No implementation was absorbed in this audit entry. The audit produced the
code-level landing plan in code-level-absorption-blueprint.md.
```

Skipped:

```text
Reasonix public CLI identity, native settings shape, native renderer-visible
events, and terminal-only product boundaries are not accepted as analytix
surface.
```

Validation:

```text
Reviewed repository snapshot 33545f6, inspected key Go engine directories and
docs, identified latest session-list sidecar/fork-branch/cache simplification
deltas, added code-level absorption blueprint and implementation plan, and ran
git diff --check.
```

Remaining risks:

```text
The next implementation batch must convert useful Reasonix tests into analytix
fixtures before porting large Go engine code. This audit does not by itself
prove Reasonix parity or engine superiority.
```

## 2026-06-20 - Reasonix main-v2 P0/P2 start refresh

Source:

```text
Reasonix version/tag: main-v2
Commit range: 33545f6..ef7bf970050f2293d85d44e689be1ec108b56fc0
Release notes: not used; remote HEAD and local diff were inspected directly
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
Starting the P0/P2 session-sidecar absorption batch found that Reasonix
main-v2 advanced from the earlier 33545f6 audit checkpoint to ef7bf97. The only
non-merge code commit in this range is f6ba755, which moves memory-write disk
I/O off the Reasonix controller lock. This does not change the current
session-sidecar batch scope, but it is a relevant controller responsiveness
improvement for a later memory/control absorption pass.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `249a4f8` session-list sidecar metadata | runtime/history performance | `must absorb` | Still in the current P2 scope; adapt cached previews and turn counts through analytix session/thread contracts. | Add analytix fixtures before implementation. |
| `73e2025` fork/branch sidecar seeding and goal-state warnings | runtime/history reliability | `must absorb` | Still in the current P2 scope; seed fork/branch sidecars and log goal-state persistence failures without exposing Reasonix events. | Add fork/branch and sanitized-log fixtures. |
| `5db1d0b` redundant disk cache removal | runtime/history simplification | `must absorb` | Still in the current P2 scope; keep sidecar-only listing plus memory cache where analytix has equivalent redundancy. | Add consistency and invalidation tests. |
| `f6ba755` memory-write disk I/O off controller lock | runtime/control responsiveness | `should absorb` | Defer from this session-sidecar batch because it touches memory/control locking, not session history listing. | Plan a memory/controller lock contention fixture and adapt behind analytix runtime contracts. |
| `ef7bf97` merge commit | upstream metadata | `optional` | Record as the refreshed remote HEAD only. | No direct implementation. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No accepted public contract change in this refresh. | Renderer remains backend-neutral through `window.analytix`. | Existing P2 session-list/fork tests plus future memory/control fixture. |
| Session/thread storage | Current batch still targets sidecar metadata behind analytix runtime routes. | No Reasonix-native storage or event shape is exposed to renderer code. | Add analytix sidecar fixtures before implementation. |
| Memory/control internals | `f6ba755` suggests an internal lock-splitting improvement only. | Defer until a memory/control batch can prove no approval/status regression. | Future controller concurrency test. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Session listing/fork sidecar behavior remains relevant to backend-neutral read/list/fork routes. | TypeScript oracle fixtures first. |
| G4/G5 | `f6ba755` informs future memory/control concurrency semantics. | Memory write, approval/status, and runtime status conformance before Go parity claims. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix controller internals vs current analytix P2 scope | Defer `f6ba755` from the session-sidecar batch; do not broaden the batch without fixtures. | `code-level-implementation-plan.md` |

Absorbed:

```text
No implementation was absorbed by this refresh entry. It updates the upstream
baseline before code changes.
```

Skipped:

```text
Reasonix native controller, memory, settings, CLI, and event shapes remain
outside the current analytix public surface. The f6ba755 lock-splitting idea is
deferred to a later controller/memory batch rather than mixed into P2
session-sidecar work.
```

Validation:

```text
Ran git status --short --branch in /Users/sun/Projects/analytix. Ran
git ls-remote for Kun master/develop and Reasonix main-v2. Kun remained at
master 8f2040349fba47fcd8e8b94f50b131943af839b2 and develop
ab24a77f0f68fcb1b2c160361e4c50cd92b02928. Reasonix main-v2 resolved to
ef7bf970050f2293d85d44e689be1ec108b56fc0. Fetched
/tmp/analytix-upstreams/DeepSeek-Reasonix and inspected
33545f6..origin/main-v2 with git log, git diff --stat, and git diff
--name-status.
```

Remaining risks:

```text
The P2 batch still needs analytix tests/fixtures before implementation. The new
Reasonix memory/control improvement is not covered by this batch and should not
be claimed as absorbed until a focused controller/memory pass lands.
```

## 2026-06-20 - P0/P2 session-sidecar absorption implemented

Source:

```text
Reasonix version/tag: main-v2
Commit range: 33545f6..ef7bf970050f2293d85d44e689be1ec108b56fc0
Implemented target commits: 249a4f8, 73e2025, 5db1d0b
Deferred from refreshed HEAD: f6ba755
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
Analytix absorbed the Reasonix session-list sidecar behavior through analytix
thread/session contracts. The landing uses analytix `metadata.jsonl` sidecars
instead of exposing Reasonix `.meta` files or native event/config shapes.
Thread summaries now carry optional cached preview/count fields; hybrid
sidecars persist those fields; legacy metadata is backfilled once; forked
threads seed summary metadata at creation; and goal persistence failures now
emit sanitized warnings before surfacing the error.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `249a4f8` sidecar preview/counts | runtime/history performance | `must absorb` | Absorbed as `ThreadSummary.preview`, `ThreadSummary.turnCount`, and `ThreadSummary.messageCount` plus hybrid sidecar summary metadata. | Add real Electron sidebar timing trace in a later QA pass. |
| `73e2025` fork/branch sidecar seed | runtime/history reliability | `must absorb` | Absorbed for `ThreadService.fork` through hybrid sidecar summaries and fork summary tests. | Branch-like future surfaces should reuse the same summary contract. |
| `73e2025` goal-state persistence warning | runtime observability | `must absorb` | Absorbed as sanitized `console.warn` on analytix goal persistence failures before rethrowing. | Consider injecting a structured runtime logger in a later observability pass. |
| `5db1d0b` redundant project-session disk cache removal | runtime/history simplification | `must absorb` | Analytix keeps sidecar-backed list plus memory caches; no new project-session disk cache was introduced. | Audit any future desktop project/session cache before adding disk persistence. |
| `f6ba755` memory-write off controller lock | runtime/control responsiveness | `should absorb` | Deferred from this batch to avoid mixing memory/control locking into session-history work. | Plan a controller/memory contention fixture. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| `packages/runtime/src/contracts/threads.ts` | Adds optional summary fields `preview`, `turnCount`, and `messageCount`. | Existing clients can ignore the fields; renderer remains backend-neutral. | `packages/runtime/tests/contracts.test.ts`, `src/renderer/src/agent/analytix-mapper.test.ts`. |
| Hybrid thread storage | Metadata lines include a small `summary` object; SQLite index gains `turn_count`. | Old metadata without summary is read and backfilled once; existing metadata remains valid. | `packages/runtime/tests/hybrid-store.test.ts`. |
| Goal persistence | Failed goal upserts warn with thread id/action/error and still throw. | Public goal event shape is unchanged. | `packages/runtime/tests/thread-service.test.ts`. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Go read/list/fork routes must produce the same optional summary fields from sidecar metadata. | Future TypeScript oracle fixture should reuse the new hybrid tests. |
| G4/G5 | Goal persistence warnings remain an observability requirement; `f6ba755` remains future memory/control input. | Future structured logger and memory/control tests. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix `.meta` filename vs analytix storage layout | Use analytix `metadata.jsonl` sidecar and `ThreadSummary` contract. | `code-level-absorption-blueprint.md` |
| Reasonix native event/config/CLI surfaces | Rejected; no renderer-visible Reasonix protocol was added. | `absorption-targets.md` |

Absorbed:

```text
packages/runtime/src/contracts/threads.ts
packages/runtime/src/domain/thread.ts
packages/runtime/src/adapters/hybrid/hybrid-thread-store.ts
packages/runtime/src/services/thread-service.ts
src/renderer/src/agent/analytix-contract.ts
src/renderer/src/agent/analytix-mapper.ts
src/renderer/src/agent/types.ts
focused runtime and renderer tests
benchmark scenario and upstream scorecard entries
```

Skipped:

```text
Reasonix public CLI identity, native `.meta` file contract, native event/config
shape, and the new f6ba755 memory/control lock split are not part of this
implemented slice.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts
tests/contracts.test.ts tests/domain.test.ts tests/thread-service.test.ts
--no-file-parallelism --maxWorkers=1

npm run test -- src/renderer/src/agent/analytix-mapper.test.ts

npm --prefix packages/runtime run typecheck

npm run typecheck

git diff --check
```

Remaining risks:

```text
No Electron sidebar timing trace was captured in this batch. Provider/cache and
tool-schema P2 work remain separate. The local better-sqlite3 native module
exits with status 137 in this environment, so hybrid tests probe it in a child
process and fall back to filesystem sidecar coverage when unavailable.
```

## 2026-06-20 - P2.1 sidecar-version and goal-lock refresh

Source:

```text
Reasonix version/tag: main-v2
Commit range: ef7bf970050f2293d85d44e689be1ec108b56fc0..6d404d80094bb4153bbaec4d7a2739e875d5c8c7
Implemented target commits: 7ebb08e
Classified target commits: 341f720
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
Reasonix added a schema-version gate for sidecar listing counts so a recorded
zero is no longer confused with "not recorded yet". Analytix absorbed the same
authority rule in its `metadata.jsonl` sidecar summary, without adopting the
Reasonix `.meta` contract. Reasonix also moved goal-state disk writes outside
its Go controller lock; the current TypeScript runtime has async store/event
calls but no equivalent global controller mutex, so the behavior is recorded as
a future Go runtime conformance requirement instead of forcing an artificial
TypeScript abstraction.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `7ebb08e` sidecar count schema version | runtime/history consistency | `must absorb` | Absorbed as `summary.schemaVersion` on analytix hybrid `metadata.jsonl`; only versioned summaries are authoritative, including zero-count summaries. | Future summary schema changes must bump this version and rederive legacy sidecars. |
| `341f720` goal-state writes off controller lock | runtime/control responsiveness | `should absorb` | No equivalent TypeScript controller lock exists; do not add one. Record as Go G4/G5 conformance so goal persistence must not block status/approval/controller critical sections. | Add Go controller contention fixture when the Go backend reaches goal persistence. |
| `6d404d8` merge commit | upstream metadata | `optional` | Record as refreshed remote HEAD only. | No direct implementation. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Hybrid thread storage | `summary.schemaVersion` marks preview/counts as content-derived and authoritative. | Old summaries without `schemaVersion` remain readable, are decoded once, and are backfilled without changing thread `updatedAt`. | `packages/runtime/tests/hybrid-store.test.ts`. |
| Runtime HTTP/SSE and renderer bridge | No public route, event, bridge, or renderer contract change. | Renderer still consumes optional `ThreadSummary` preview/count fields through analytix contracts only. | Existing mapper and contract tests. |
| Goal persistence | No TypeScript code change for `341f720`; current path has no shared controller mutex. | Existing sanitized warning behavior from the prior P2 batch remains. | `packages/runtime/tests/thread-service.test.ts`; Go conformance record below. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Go list/read/fork routes must preserve analytix `metadata.jsonl` summary version semantics or an equivalent backend-neutral authority marker. | TypeScript hybrid-store sidecar-version fixtures become oracle tests. |
| G4/G5 | Go goal persistence must snapshot state under any controller lock and write durable state outside locks shared with status, approvals, user input, or runtime polling. | Add a controller contention/race fixture before claiming Go goal parity. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix `BranchMeta.SchemaVersion` vs analytix storage | Use analytix `summary.schemaVersion` inside `metadata.jsonl`; do not expose Reasonix `.meta`. | `code-level-implementation-plan.md` |
| Reasonix Go controller lock fix vs TS runtime architecture | TS has no global controller lock equivalent, so record as Go conformance instead of inventing a lock. | `go-runtime-conformance.md` |

Absorbed:

```text
packages/runtime/src/adapters/hybrid/hybrid-thread-store.ts
packages/runtime/tests/hybrid-store.test.ts
docs/analytix/upstreams/reasonix-sync.md
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/code-level-implementation-plan.md
docs/analytix/upstreams/go-runtime-conformance.md
docs/analytix/benchmarks/benchmark-scenarios.md
docs/analytix/benchmarks/upstream-scorecard.md
```

Skipped:

```text
Reasonix public CLI, native `.meta`, native config/event shapes, and Go
controller internals remain outside analytix public surface. No Kun develop
workflow/Loop code was merged.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts
tests/contracts.test.ts tests/domain.test.ts tests/thread-service.test.ts
--no-file-parallelism --maxWorkers=1

npm run test -- src/renderer/src/agent/analytix-mapper.test.ts

npm --prefix packages/runtime run typecheck

npm run typecheck

git diff --check
```

Remaining risks:

```text
Existing SQLite rows are still treated as rebuildable index entries; the
sidecar-version authority rule is enforced when rebuilding from filesystem
metadata and on new writes. A future Go backend still needs an explicit
controller-lock contention fixture for goal persistence before Go parity can be
claimed.
```

## 2026-06-20 - Reasonix main-v2 P1.1 preflight refresh

Source:

```text
Reasonix version/tag: main-v2
Commit range: 6d404d80094bb4153bbaec4d7a2739e875d5c8c7..be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
New non-merge commit: ad3d742
Release notes: not used; remote HEAD and local diff were inspected directly
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
Before starting the Kun P1.1 stable-critical implementation, Reasonix
main-v2 was refreshed and had advanced by one desktop-focused merge. The new
change gates legacy topic migration behind a per-directory marker so desktop
tab listing does not rescan already-migrated session directories on each
render. This is useful future migration/listing performance input, but it does
not touch the current P1.1 Kun scope: provider endpoints, Xiaomi model presets,
per-provider runtime routing, compaction tool-call/tool-result repair, or
preload sandbox startup guards.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `ad3d742` per-directory legacy topic migration marker | desktop/history migration performance | `should absorb` | Defer from Kun P1.1; evaluate only when analytix adds or revisits an equivalent topic/session migration pass. | Add a migration-marker fixture if analytix gains a repeated legacy topic migration scan. |
| `be67a49` merge commit | upstream metadata | `optional` | Record as refreshed remote HEAD only. | No direct implementation. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime/provider/preload P1.1 contracts | No accepted contract change from this Reasonix refresh. | Kun P1.1 remains scoped to analytix provider/runtime/preload behavior. | P1.1 provider/runtime tests remain required. |
| Future history migration | Possible internal performance marker only if analytix has an equivalent repeated migration scan. | Marker must be analytix-named and must not hide deferred or partial legacy sessions. | Future migration-marker fixture. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Future Go session/topic listing should avoid repeated full legacy migration scans after a complete pass. | Backend-neutral listing/migration fixture before any Go parity claim. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix desktop topic marker vs current Kun P1.1 scope | Defer; do not broaden P1.1 beyond the Kun stable critical fixes. | `code-level-implementation-plan.md` |

Absorbed:

```text
No implementation was absorbed by this refresh entry.
```

Skipped:

```text
Reasonix desktop topic migration marker behavior is not part of P1.1 and is
deferred because current P1.1 targets Kun stable provider/runtime/preload
critical fixes.
```

Validation:

```text
Ran git ls-remote for Kun master/develop and Reasonix main-v2. Kun remained at
master 8602476c5c449b4561473ad5f31081ee93dc782e and develop
ab24a77f0f68fcb1b2c160361e4c50cd92b02928. Reasonix main-v2 resolved to
be67a498adcaed6e33dcf3cbd25395e9cbccd6fa. Fetched
/tmp/analytix-upstreams/DeepSeek-Reasonix and inspected
6d404d80094bb4153bbaec4d7a2739e875d5c8c7..be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
with git log, git diff --stat, and git diff --name-status.
```

Remaining risks:

```text
This refresh does not prove or implement the deferred migration-marker idea.
It only confirms that the new Reasonix HEAD does not change the Kun P1.1
provider/runtime/preload implementation scope.
```

## 2026-06-20 - Reasonix main-v2 P1.3 workflow review

Source:

```text
Reasonix version/tag: main-v2
Reviewed HEAD: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
New non-merge commit since P1.1/P1.2 baseline: ad3d742
Release notes: not used; remote HEAD and local source were inspected directly
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix using origin/main-v2
```

Summary:

```text
P1.3 re-reviewed Reasonix as an agent/runtime upstream for workflow-relevant
engine primitives. The reviewed tree has strong controller, approval, user
input, background job, subagent, event stream, provider/cache, MCP/plugin,
permission, sandbox, and session-history ideas. However, Reasonix does not
provide a product Workflow/Create Loop builder or analytix-compatible workflow
contract at this HEAD. The only current HEAD delta, ad3d742, gates legacy topic
migration with a per-directory marker and is unrelated to the P1.3 Create Loop
product slice.
```

Reviewed files and capabilities:

| Area | Files reviewed | P1.3 decision |
| --- | --- | --- |
| Agent loop / task orchestration | `internal/agent/agent.go`, `task.go`, `parallel_tasks.go`, `subagent_store.go`, `save.go`, `branch.go`, `migrate.go`, `compact.go`, `prune.go`, `cache_shape.go` | Useful future engine input; not a product workflow builder. Defer orchestration primitives to P2.2 / Go runtime conformance. |
| Controller / approval / user input | `internal/control/controller.go`, `input.go`, `approval_e2e_test.go`, `goal_concurrency_test.go`, `replay_pending_test.go` | Keep as future runtime-controller evidence; P1.3 uses existing analytix runtime approval/user-input cards and detects pending gates from projected thread blocks. |
| Jobs / artifacts / background execution | `internal/jobs/jobs.go`, `artifacts.go`, `internal/tool/builtin/bgjobs.go` | Valuable for future durable workflow/background tasks; do not introduce Reasonix job protocol in P1.3. |
| Event stream / serve | `internal/event/event.go`, `sync.go`, `internal/serve/broadcaster.go`, `serve.go`, `wire.go` | Defer SSE broadcaster/drop/replay ideas to Go runtime and workflow persistence work. |
| Provider/cache/context | `internal/provider/provider.go`, `internal/provider/openai/openai.go`, `internal/provider/anthropic/anthropic.go`, `retry.go`, `schema_canonicalize.go` | Defer to P2.2 provider/cache fixtures; do not mix with P1.3 product workflow UI. |
| Tool / MCP / permissions / sandbox | `internal/tool/tool.go`, `internal/plugin/plugin.go`, `lazy.go`, `internal/permission/permission.go`, `internal/sandbox/sandbox.go` | Defer to P3 tool/MCP/sandbox lifecycle. |
| Current HEAD delta | `desktop/tabs.go`, `desktop/tabs_topic_test.go` from `ad3d742` | Defer to P3 session/history migration performance; unrelated to Create Loop. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No Reasonix public contract is accepted in P1.3. | Renderer remains on `window.analytix` and existing analytix runtime routes. | P1.3 renderer workflow tests plus existing P1.1/P1.2 regressions. |
| Workflow product surface | No Reasonix UI or protocol is exposed. | P1.3 Workflow view is analytix-native and uses existing `AgentProvider` methods. | `create-loop-runtime.test.ts`, `WorkflowCreateLoopView.test.ts`. |
| Future engine/runtime | Reasonix controller/jobs/events/provider/cache/tool ideas remain documented inputs. | Future code must adapt behind analytix contracts. | P2.2/P3/Go conformance fixtures before absorption claims. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Session/topic migration markers and event replay remain relevant for backend-neutral list/replay routes. | Add analytix migration-marker and SSE replay fixtures before Go parity claims. |
| G4/G5 | Controller approval/user-input replay, background jobs, goal/memory writes off critical locks, and subagent orchestration remain useful. | Add controller contention, pending-prompt replay, job lifecycle, and subagent continuation fixtures. |
| G6 | Reasonix engine primitives may improve long workflow reliability only after they pass analytix desktop workflow QA. | No Go default switch without desktop QA and rollback. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix engine primitives vs P1.3 product workflow slice | Do not absorb Reasonix code in P1.3; the batch needs a product Workflow/Create Loop surface, not a new engine protocol. | spec 08 / `code-level-implementation-plan.md` |
| Reasonix native event/job/session protocols vs analytix renderer contract | Analytix HTTP/SSE and `window.analytix` remain the only renderer-visible contract. | `code-level-absorption-blueprint.md` |

Absorbed:

```text
No Reasonix implementation code was absorbed in P1.3.
```

Skipped:

```text
Reasonix native workflow/task/job/event/settings/CLI surfaces are not accepted
as analytix product surface. ad3d742 topic migration marker is deferred because
it belongs to session/history migration performance, not Create Loop.
```

Validation:

```text
Ran git ls-remote for Kun master/develop and Reasonix main-v2. Kun remained at
master 8602476c5c449b4561473ad5f31081ee93dc782e and develop
ab24a77f0f68fcb1b2c160361e4c50cd92b02928. Reasonix main-v2 resolved to
be67a498adcaed6e33dcf3cbd25395e9cbccd6fa. Fetched
/tmp/analytix-upstreams/DeepSeek-Reasonix and inspected origin/main-v2 files
listed above. A read-only sub-agent independently reviewed the same HEAD and
confirmed that no Reasonix code should enter P1.3.
```

Remaining risks:

```text
P1.3 does not improve Reasonix engine parity. Provider/cache/tool lifecycle,
background jobs, event replay, checkpoint/rewind, topic migration markers, and
Go controller semantics remain future P2.2/P3/Go work.
```

## 2026-06-20 - Reasonix main-v2 P2.2 provider/cache/tool lifecycle absorption

Source:

```text
Reasonix version/tag: main-v2
Reviewed HEAD: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Remote drift since P1.3: none beyond the already-classified ad3d742 merge range
Release notes: not used; remote HEAD and local source were inspected directly
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix using origin/main-v2
Kun conflict check: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
```

Summary:

```text
P2.2 reviewed Reasonix as the primary provider/cache/tool lifecycle upstream.
Analytix already had a broad provider client for chat completions, OpenAI
Responses, Anthropic Messages, custom full endpoints, streaming usage parsing,
reasoning fields, transient retry, model-history repair, request-history
hygiene, tool catalog fingerprinting, approvals, user input, event recording,
and SSE replay. The concrete missing Reasonix capability was cache-shape
diagnostics that explain cache hit/miss behavior when the stable prefix, tool
schema set, provider, or model changes.
```

Reviewed files and capabilities:

| Area | Reasonix files reviewed | analytix mapping |
| --- | --- | --- |
| Provider endpoint contract | `internal/provider/provider.go`, `internal/provider/openai/openai.go`, `internal/provider/anthropic/anthropic.go`, `internal/provider/retry.go`, `internal/provider/schema_canonicalize.go` | Existing `packages/runtime/src/adapters/model/compat-model-client.ts`, `multi-provider-model-client.ts`, `src/shared/openai-compat-url.ts`, `src/main/provider-connection.ts`, write-inline, scheduled detector, and model-list paths already cover URL/body/header/stream/usage/reasoning/error logging. |
| Cache lifecycle | `internal/agent/cache_shape.go`, `internal/agent/cache_diagnostics_test.go` | Accepted as analytix `prefix-cache-diagnostics.ts` plus optional `usage.cacheDiagnostics` runtime event fields. |
| Tool lifecycle | `internal/tool/tool.go`, `internal/plugin/plugin.go`, `internal/plugin/lazy.go`, schema canonicalization helpers | Existing analytix tool host, canonical tool catalog fingerprint, request-history hygiene, approval/user-input gates, and model-history repair remain the contract; P2.2 adds tool-schema cache-drift attribution only. |
| Session/event lifecycle | `internal/control/controller.go`, `internal/event/event.go`, `internal/serve/broadcaster.go`, `internal/agent/save.go`, `branch.go` | Existing `RuntimeEventRecorder`, `FileSessionStore`, `HybridSessionStore`, `InMemoryEventBus`, and SSE replay stay authoritative; no Reasonix native event protocol is exposed. |
| Provider diagnostics | Reasonix provider/error handling and tests | Existing analytix sanitized HTTP failure logs already include provider, sanitized baseUrl/requestUrl, endpoint format, model, status, and summarized response body; no rewrite needed in P2.2. |

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Cache prefix shape capture and compare | provider/cache | `should absorb` | Absorbed as backend-neutral `CachePrefixShape` and `CacheDiagnostics` with prefix hash, system/mode/prefix/tool hashes, provider/model metadata, prefix-change reasons, and provider cache hit/miss tokens. | Include this field in future provider/cache fixture reviews. |
| Canonical tool schema cache attribution | tool/cache | `should absorb` | Absorbed by sorting tool specs by name and schema keys before hashing, so order noise does not trigger false cache-drift reports while real tool schema changes are attributed as `tools`. | Broader MCP/plugin lifecycle remains separate. |
| Reasonix public cache/job/event protocol | protocol | `reject` | Do not expose Reasonix-native events, settings, CLI, or renderer protocols. The only contract change is optional analytix `usage.cacheDiagnostics`. | Naming/protocol scans remain mandatory. |
| Full provider rewrite | provider | `defer pending evidence` | Existing analytix provider client already handles the requested endpoint families and diagnostics. Rewriting would broaden risk without focused proof. | Add live-provider matrix only when credentials/QA are available. |
| MCP/plugin lazy lifecycle, tool cancellation, checkpoints, stream reconnect after partial output, Go controller work | tool/session/safety | `defer pending evidence` | Valuable, but too broad for the cache diagnostics slice and overlaps P3/Go stages. | P3 safety/checkpoint/durable workflow and future Go conformance. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| `packages/runtime/src/contracts/events.ts` | `usage` events may include optional `cacheDiagnostics`. | Existing clients can ignore the field; usage aggregation remains unchanged. | Focused runtime loop test and runtime typecheck. |
| Renderer DTO | `CoreRuntimeEventJson` accepts optional `cacheDiagnostics`. | Mapper continues to ignore unknown diagnostics for UI projection; no UI change. | App typecheck and SSE mapper review. |
| Provider/settings/bridge | No settings, provider preset, preload bridge, CLI, or UI contract change. | `window.analytix`, top-level `runtime`, and `analytix serve` remain the only active surfaces. | Identity/protocol scan. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G3 | Go provider adapters must preserve provider-native cache hit/miss parsing and may emit the same optional diagnostics through analytix `usage` events. | TypeScript cache diagnostics tests become oracle fixtures. |
| G4/G5 | Go tool/plugin lifecycle must preserve canonical tool schema hashing or an equivalent backend-neutral cache-shape proof. | Tool lifecycle and MCP fixtures before Go parity claims. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix cache diagnostics expose engine internals | Accept only privacy-bounded hashes, counts, and metadata on analytix usage events; no raw prompt, tool args, tool output, file contents, API keys, or full response text. | spec 08 / spec 09 |
| Kun provider/workflow surfaces vs Reasonix engine slice | Kun is regression/conflict input only for P2.2; do not absorb new Kun code because P1.1/P1.2/P1.3 are already scoped and closed. | `absorption-ledger.md` |

Absorbed:

```text
packages/runtime/src/cache/prefix-cache-diagnostics.ts
packages/runtime/src/contracts/events.ts
packages/runtime/src/loop/agent-loop.ts
src/renderer/src/agent/analytix-contract.ts
packages/runtime/tests/cache.test.ts
packages/runtime/tests/loop.test.ts
```

Skipped:

```text
Reasonix UI, public CLI identity, settings shape, native event protocol, full
provider rewrite, lazy plugin lifecycle, checkpoint/rollback, stream reconnect
protocol changes, and Go runtime work were not absorbed. Kun code was not
absorbed in P2.2; it was used only to confirm no P1.1/P1.2/P1.3 conflict.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/cache.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/loop.test.ts -t "cache prefix diagnostics" --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/workflow/create-loop-runtime.test.ts src/renderer/src/components/workflow/WorkflowCreateLoopView.test.ts -- --runInBand
npm run test -- src/renderer/src/components/chat/ConnectPhoneView.test.ts src/main/telegram-runtime.test.ts src/main/claw-runtime.test.ts -- --runInBand
npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts tests/model-client.test.ts tests/context-compactor.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run typecheck
npm run typecheck
git diff --check HEAD
production identity/protocol scan for window.kunGui, window.reasonix, kun serve,
reasonix serve, and old bridge/settings fallback strings
```

Remaining risks:

```text
P2.2 improves diagnostics for cache/tool/provider lifecycle, not full live
provider parity, full Reasonix tool/plugin lifecycle, durable workflow storage,
checkpoint/rollback, or Go runtime parity. Live provider matrix QA remains a
release-gate activity when credentials are available.
```

## 2026-06-20 - Reasonix main-v2 P3A tool/MCP/safety/checkpoint boundary slice

Source:

```text
Reasonix version/tag: main-v2
Reviewed HEAD: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Previous historical checkpoint: 6d404d80094bb4153bbaec4d7a2739e875d5c8c7
Remote drift since 6d404d8: ad3d742 + merge be67a498, limited to desktop/tabs.go and desktop/tabs_topic_test.go
Release notes: not used; remote HEAD and local source were inspected directly
Local comparison branch: /Users/sun/Projects/DeepSeek-Reasonix origin/main-v2
Kun conflict check: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
```

Summary:

```text
P3A reviewed Reasonix as an engine/runtime source for tool, MCP/plugin,
permission, sandbox, checkpoint, and control boundaries. The current HEAD
be67a498 has no new changes in those target directories compared with the
historical 6d404d8 checkpoint, so P3A focuses on code-level absorption of
bounded safety behavior already present in the Reasonix map: MCP schema
normalization/canonicalization and denied-approval tool safety. Checkpoint and
rewind remain design/test-prep only for P4.
```

Reviewed files and capabilities:

| Area | Reasonix files reviewed | analytix mapping |
| --- | --- | --- |
| MCP/plugin schema and lifecycle | `internal/plugin/canonicalize.go`, `internal/plugin/stdio_cancel_test.go`, `internal/plugin/lazy.go`, `internal/plugin/plugin.go` | Analytix normalizes malformed MCP input schemas before advertising tools and keeps broader stdio cancellation/lazy lifecycle as future P3/P4/Go work. |
| Tool execution | `internal/tool/tool.go`, `internal/tool/builtin/*`, `internal/tool/registry_canon_test.go` | Existing `LocalToolHost`, sandbox policy, tool catalog fingerprint, and built-in tool tests remain the TS runtime oracle. |
| Permission and approvals | `internal/permission/permission.go`, `internal/control/approval_e2e_test.go`, `internal/control/input.go` | Analytix keeps GUI approval gates and now directly proves denied approvals do not execute the tool body. |
| Sandbox | `internal/sandbox/sandbox.go`, `internal/tool/builtin/confine.go`, protected-dir tests | Analytix keeps current approval/sandbox modes; protected-path and process sandbox expansion are deferred beyond P3A. |
| Checkpoint/rewind | `internal/checkpoint/checkpoint.go`, `internal/control/rewind_e2e_test.go` | Valuable model for P4, but P3A does not implement rewind or expose Reasonix protocols. |

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| MCP malformed input schema normalization before advertisement | MCP/tool | `should absorb` | Absorbed in `packages/runtime/src/adapters/tool/mcp-tool-provider.ts`; invalid or non-object input schemas become a bounded object schema before entering the tool catalog/model request path. | Future Go G4 fixture and broader MCP lifecycle tests. |
| Denied GUI approval never executes tool body | Approval/tool safety | `should absorb` | Added direct focused proof in `packages/runtime/tests/builtin-tools.test.ts`; denied approvals return an approval item and leave the tool function untouched. | Future Go G4 approval conformance. |
| MCP stdio cancellation and hung server timeout | MCP/plugin | `defer pending evidence` | Existing analytix passes abort signals and timeouts to MCP calls; full hung-stdio process fixture remains future work. | P3B cancellation fixture. |
| Permission rules and sandbox protected paths | Permission/sandbox | `defer pending evidence` | Keep current analytix sandbox and read-before-edit guards; expand protected-path fixtures separately. | P3B protected-path suite. |
| Checkpoint / rewind | Checkpoint/history | `defer` | Record Reasonix snapshot/rewind model for P4. Do not add UI or event-log rewrite in P3A. | P4 checkpoint/rewind implementation plan. |
| Reasonix public CLI/settings/event protocol | Identity/protocol | `reject` | Continue exposing only analytix contracts. | Identity/protocol scan. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| MCP tool descriptor normalization | Malformed `inputSchema` is normalized inside the runtime before the registry/model catalog sees it. | Valid object schemas remain valid; invalid schemas become a safe object fallback instead of leaking malformed data to providers. | `packages/runtime/tests/mcp-tool-provider.test.ts`. |
| Approval/tool execution | No API change; focused test pins existing denial behavior. | Existing approval UI and event contract remain unchanged. | `packages/runtime/tests/builtin-tools.test.ts`. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G4 | Go MCP/plugin tooling must normalize malformed schemas before advertising or sending them to providers. | Reproduce the P3A MCP schema fixture. |
| G4 | Go approval gate must prove denied tools are never executed. | Reproduce the denied-approval fixture and pending approval event behavior. |
| G5 | Checkpoint/rewind may enter after P4 design and TS oracle fixtures exist. | No Go rewind parity claim before P4 tests land. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix checkpoint/rewind vs Kun git checkpoint | Defer implementation; future checkpoint must be analytix-owned and may not leak `refs/kun/checkpoints` or Reasonix-native renderer protocols. | D-0006 |
| Engine safety work vs product workflow scope | Keep P3A runtime-scoped; do not reopen full Workflow builder or UI changes. | D-0002 / D-0003 |

Absorbed:

```text
packages/runtime/src/adapters/tool/mcp-tool-provider.ts
packages/runtime/tests/mcp-tool-provider.test.ts
packages/runtime/tests/builtin-tools.test.ts
```

Skipped:

```text
Full MCP/plugin lazy lifecycle, hung stdio process fixture, protected-path
sandbox expansion, checkpoint/rewind implementation, Go scaffold, Rust helper,
Reasonix public identity, and Reasonix-native renderer/event protocols.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/mcp-tool-provider.test.ts -t "normalizes malformed MCP tool schemas" --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/builtin-tools.test.ts -t "does not execute a tool after GUI approval is denied" --no-file-parallelism --maxWorkers=1
P2.2 cache diagnostics, P1.3 workflow regressions, P1.2 Connect Phone
regressions, P1.1 runtime regressions, runtime/app typechecks,
git diff --check HEAD, and production identity/protocol scan passed before
commit.
```

Remaining risks:

```text
P3A closes only a narrow tool/MCP/approval boundary. It does not close full
Reasonix MCP/plugin lifecycle parity, sandbox parity, checkpoint/rewind, Go G4,
or release/G0 readiness.
```

## 2026-06-20 - P4A checkpoint/rewind engine oracle

Source:

```text
Reasonix version/tag: main-v2 snapshot
Remote HEAD: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix origin/main-v2
Kun conflict check: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
```

Summary:

```text
P4A performs engine/runtime checkpoint/rewind 能力的代码级吸收. Reasonix
provides the engine idea: per-turn checkpoint boundaries, first-touch file
capture semantics, path safety, conversation boundary awareness, and rewind
that can be reasoned about without relying on product UI. Analytix adapts those
ideas into runtime contracts and oracle fixtures, not Reasonix CLI/settings or
renderer-native protocols.
```

Reviewed files and capabilities:

| Area | Reasonix files reviewed | analytix mapping |
| --- | --- | --- |
| Per-turn checkpoint metadata | `internal/checkpoint/checkpoint.go`, `docs/CHECKPOINTS.md` | `CheckpointMetadataSchema` with schema version, `axcp_` id, workspace, turnId, changed files, createdAt, and status. |
| First-touch file capture | `internal/checkpoint/checkpoint.go` tests | `createAnalytixCheckpointMetadata` keeps the first normalized changed-file entry for a path. |
| Path safety | `internal/checkpoint` path validation | P4A rejects checkpoint changed files that escape the workspace. |
| Conversation rewind boundary | `internal/control/rewind_e2e_test.go`, control rewind paths | `planConversationOnlyRewindFromEvents` retains events before the target turn boundary and projects the safe transcript. |
| Reasonix CLI/settings/event protocol | `internal/cli/rewind.go`, runtime control surfaces | Rejected; renderer/preload/main keep analytix contracts. |

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Checkpoint metadata/event contract | runtime/history | `should absorb` | Added analytix `CheckpointMetadataSchema` and `checkpoint_captured` event projection. | Future route/service integration in P4B. |
| First-touch changed-file normalization | file safety | `should absorb` | Added focused oracle coverage that duplicate changed-file paths keep the first captured metadata. | Future file snapshot/restore tests. |
| Workspace escape rejection | file safety | `must absorb` | Added runtime helper validation and focused test. | Keep in any future git or non-git storage backend. |
| Conversation-only rewind planning | event replay/history | `should absorb` | Added non-mutating plan from runtime events and projection. | P4B combined rewind; P4C review UI. |
| Reasonix CLI/settings/renderer protocol | identity/protocol | `reject` | No Reasonix-native public protocol is exposed. | Identity/protocol scan. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Checkpoint metadata | New runtime-owned schema in `packages/runtime/src/contracts/checkpoints.ts`. | Additive; no existing HTTP/SSE route requires it yet. | `packages/runtime/tests/checkpoint-rewind-oracle.test.ts`. |
| Runtime events | Adds `checkpoint_captured` to the runtime event union and reducer projection. | Additive event kind; current renderer event JSON is broad and P4A emits no UI route event by default. | `RuntimeEventSchema.parse` in the focused fixture. |
| Event replay | Projection now includes `checkpoints`. | Existing transcript and turn projection remain unchanged. | Conversation-only rewind fixture. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G5 | Go must reproduce checkpoint metadata, event projection, changed-file path safety, and conversation-only rewind planning. | Reproduce `packages/runtime/tests/checkpoint-rewind-oracle.test.ts`. |
| G5/P4B | Go cannot claim full rewind parity until combined file+conversation restore and review UI fixtures exist. | Future P4B/P4C gates. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix git-free snapshots vs Kun git rollback | P4A starts with a git-free append-only oracle; future restore can choose storage only after analytix contract/review proof. | D-0006 / D-0007 |
| Reasonix rewind CLI vs analytix renderer contract | Reject Reasonix CLI/settings/protocol; expose future controls only through analytix runtime/review contracts. | D-0007 |

Absorbed:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/contracts/events.ts
packages/runtime/src/domain/checkpoint-oracle.ts
packages/runtime/src/domain/runtime-event-reducer.ts
packages/runtime/tests/checkpoint-rewind-oracle.test.ts
```

Skipped:

```text
Combined code+conversation restore, file snapshot storage, event-log rewrite,
Reasonix CLI/settings/protocol, Go backend scaffold, Rust helper, and renderer
review controls.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-oracle.test.ts --no-file-parallelism --maxWorkers=1
Full closure also re-runs P3A, P2.2, P1.3, P1.2, P1.1 regressions, typechecks,
git diff --check HEAD, and production identity/protocol scan.
```

Remaining risks:

```text
P4A is an oracle slice. It does not prove crash-safe file restore, combined
rewind, review UI, desktop QA, Go runtime parity, or release/G0 readiness.
```

## 2026-06-20 - P4B combined rewind plan-only absorption

Source:

```text
Reasonix version/tag: main-v2
Remote HEAD recheck: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Local branch: codex/p4b-full-rewind-review-ui
```

Summary:

```text
P4B converts more of Reasonix's checkpoint/rewind engine idea into
analytix-owned contracts. The accepted slice is not file restore execution; it
is a combined code+conversation rewind plan that can be audited by runtime and
renderer code without mutating workspace files or rewriting durable history.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Combined code+conversation rewind planning | runtime / history | `should absorb` | Added `axrp_` rewind plan schema with `code`, `conversation`, and `combined` scopes. Conversation continues to use event projection; files are metadata-only restore actions. | P4C destructive apply and crash-recovery resume. |
| Path and symlink safety | file restore safety | `must absorb` | P4B blocks path escape, absolute persisted paths, and symlink targets in the plan. It also fixes P4A's over-strict `..name` rejection. | P4C should add real snapshot/git apply fixtures for staged/untracked files. |
| Runtime route/service boundary | HTTP contract | `should absorb` | Added `POST /v1/threads/{id}/checkpoints/{checkpoint}/rewind-plan`, service wiring, shared endpoint, IPC allow-list, and renderer provider method. | Future apply route must define a new contract first. |
| Reasonix native CLI/settings/protocol | identity / renderer protocol | `reject` | No Reasonix renderer-visible rewind protocol, CLI command, settings envelope, or Go backend scaffold. | Keep identity/protocol scan in every P4/G0 batch. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Checkpoint/rewind contract | Adds `CheckpointRewindPlan` response with `axrp_` ids, `plan_only`, `destructive: false`, file risks, and summarized conversation projection. | Additive; no existing route changes shape. | `packages/runtime/tests/checkpoint-rewind-plan.test.ts`. |
| Runtime HTTP/IPC | Adds a rewind-plan route and desktop allow-list entry. | Existing runtimeRequest bridge remains `window.analytix`; no new bridge alias. | `src/main/ipc/app-ipc-schemas.test.ts`, `src/renderer/src/agent/analytix-runtime.test.ts`. |
| Review/history UI | Existing `file_change`/ChangeInspector/TurnChangeSummary can display `meta.rewindPlan` without diff/apply controls. | Existing file diffs and generated-file panels are unchanged. | `src/renderer/src/components/chat/derive-turn-sections.test.ts`. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G5 | Go must reproduce P4A checkpoint metadata/events and P4B `axrp_` plan-only semantics before claiming checkpoint parity. | Reproduce `checkpoint-rewind-oracle.test.ts` and `checkpoint-rewind-plan.test.ts`. |
| G5/P4C | Go cannot claim destructive restore parity until apply/desktop QA fixtures exist. | Future P4C destructive restore + desktop QA. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix snapshot restore vs analytix no-silent-write rule | P4B stops at auditable plan; destructive apply waits for P4C. | D-0006 / D-0008 |
| Reasonix native rewind protocol vs analytix UI ownership | Renderer sees only analytix provider/ToolBlock metadata, not Reasonix protocol. | D-0003 / D-0008 |

Absorbed:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/domain/checkpoint-oracle.ts
packages/runtime/src/services/checkpoint-rewind-service.ts
packages/runtime/src/server/routes/checkpoints.ts
src/shared/analytix-endpoints.ts
src/main/ipc/app-ipc-schemas.ts
src/renderer/src/agent/analytix-runtime.ts
src/renderer/src/components/ChangeInspector.tsx
src/renderer/src/components/chat/message-timeline-cards.tsx
```

Skipped:

```text
Destructive file apply, event-log rewrite, file snapshot storage, git refs,
Reasonix CLI/settings/protocol, Go backend scaffold, Rust helper, and full
desktop destructive-restore QA.
```

Validation:

```text
New focused tests:
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-plan.test.ts tests/checkpoint-rewind-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/components/chat/derive-turn-sections.test.ts -- --runInBand
Full closure also re-runs P4A, P3A, P2.2, P1.3, P1.2, P1.1 regressions, typechecks, git diff --check HEAD, and identity/protocol scan.
```

Remaining risks:

```text
P4B proves an auditable restore/rewind plan and review UI display only. It does
not apply files, rewrite transcripts, recover after destructive apply, or prove
packaged desktop QA. Those are P4C gates; Go remains future G5 oracle work.
```

## 2026-06-20 - P4C confirmed rewind restore apply

Source:

```text
Reasonix version/tag: main-v2 be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Commit range: no new Reasonix fetch in this code slice; P4C builds on the P4B upstream checkpoint and local baseline.
Release notes: not used.
Local comparison branch: codex/p4c-confirmed-rewind-restore-apply
```

Summary:

```text
P4C performs code-level absorption of Reasonix checkpoint/rewind engine
semantics behind analytix contracts: plan-based apply, snapshot/hash safety,
rescue-before-mutation, append-only audit, and idempotent restore behavior.
No Reasonix public protocol, CLI, renderer route, settings shape, Go backend,
or Rust helper is exposed.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Plan-based destructive rewind | checkpoint / runtime apply | `should absorb` | Apply accepts only a P4B `CheckpointRewindPlan`, validates it against the current event-derived plan, and rejects stale/tampered shortcuts. | Future Go G5 must reproduce this oracle. |
| Snapshot/hash safety | file restore | `should absorb` | Modified/deleted restore requires matching before snapshot evidence and recomputes snapshot content hashes; created delete requires afterHash/current hash agreement; missing or mismatched evidence blocks. | Automatic production snapshot capture/storage is deferred until privacy/recovery design is proven. |
| Symlink and mutation-time revalidation | file restore safety | `must absorb` | Parent-directory symlink escapes, final symlinks, staged changes, and file/git drift before mutation are blocked; partial failures preserve already-applied file results in audit. | Future Go G5 must reproduce these fixtures. |
| Rescue-before-mutation | crash recovery | `should absorb` | `checkpoint_rewind_rescue_created` records an `axrr_` rescue record before destructive file mutation. | Desktop crash/restart QA remains release evidence. |
| Conversation rewind | runtime history | `should absorb with analytix adaptation` | Conversation restore is `checkpoint_rewind_applied` append-only audit; it does not rewrite `events.jsonl` or `messages.jsonl`. | A future projection/migration strategy must be separately tested before any rewrite. |
| Reasonix native protocol / Go runtime / Rust helper | protocol / backend | `reject` / `defer` | No Reasonix route/settings/event protocol is exposed; no Go backend or Rust helper is started in P4C. | G0/G5 conformance inventory can follow after P4C QA. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime checkpoint contract | Adds `axra_` apply result, `axrr_` rescue record, confirmation, snapshot evidence, and apply response schemas. | Additive; P4B `plan_only` response remains unchanged. | `packages/runtime/tests/checkpoint-rewind-apply.test.ts`. |
| Runtime event contract | Adds `checkpoint_rewind_rescue_created` and `checkpoint_rewind_applied`. | Additive audit events; reducer treats them as audit-only and does not rewrite transcript projection. | Focused apply tests and runtime typecheck. |
| HTTP/IPC/provider | Adds `/v1/threads/{id}/checkpoints/{checkpoint}/rewind-apply`, shared endpoint, IPC allow-list, renderer provider method. | Additive; existing plan route remains read-only. | IPC/provider tests. |
| UI review surface | Adds confirmation controls to existing ChangeInspector / TurnChangeSummary plan display. | No new shell and no Reasonix protocol exposure. | Typecheck plus desktop QA gate. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G5 | P4C becomes the destructive-apply oracle fixture after P4A metadata and P4B plan-only semantics. | Go must match apply confirmation, plan validation, rescue event, snapshot content/hash blocking, parent/final symlink blocking, mutation-time revalidation, partial-failure audit, append-only conversation audit, and idempotency before parity claims. |
| G0/G1 | No backend scaffold is introduced. | Proceed only as conformance inventory, not a runtime switch. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix rewind protocol vs analytix contract ownership | Adapt engine semantics behind analytix route/provider/UI only. | D-0009 |
| Destructive apply vs transcript safety | Append-only audit now; no silent event/message rewrite. | D-0007 / D-0009 |

Absorbed:

```text
packages/runtime/src/contracts/checkpoints.ts
packages/runtime/src/contracts/events.ts
packages/runtime/src/services/checkpoint-rewind-service.ts
packages/runtime/src/server/routes/checkpoints.ts
packages/runtime/src/server/routes/index.ts
src/shared/analytix-endpoints.ts
src/main/ipc/app-ipc-schemas.ts
src/renderer/src/agent/analytix-contract.ts
src/renderer/src/agent/analytix-runtime.ts
src/renderer/src/components/RewindPlanApplyControls.tsx
src/renderer/src/components/ChangeInspector.tsx
src/renderer/src/components/chat/message-timeline-cards.tsx
```

Skipped:

```text
Reasonix CLI/settings/protocol, Go backend scaffold, Rust helper, direct event
rewrite, automatic production snapshot capture, git refs, and any renderer
dependency on Reasonix-native shapes.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-apply.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/components/chat/derive-turn-sections.test.ts --no-file-parallelism --maxWorkers=1
P4A/P4B/P3A/P2.2/P1.3/P1.2/P1.1 regressions, runtime/app typechecks, git diff --check, identity/protocol scan, and docs/analytix/qa/p4c-desktop-qa-2026-06-20.md desktop smoke are P4C closure evidence.
```

Remaining risks:

```text
Destructive apply is available and tested through explicit snapshot evidence,
but production automatic snapshot capture remains a follow-up. Release closure
still depends on a live desktop apply fixture, crash/restart apply QA, packaged QA,
identity/protocol scan evidence, and Spec 07 release/push blockers.
```

## 2026-06-20 - G0/G5 conformance inventory runtime baseline

Source:

```text
Reasonix version/tag: main-v2 be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Commit range: no new Reasonix fetch in this inventory slice; P4C remains the upstream checkpoint.
Local comparison branch: codex/p4c-confirmed-rewind-restore-apply at b3e1674 before the inventory branch.
```

Summary:

```text
This batch does not absorb new Reasonix code and does not start a Go backend.
It converts the current TypeScript runtime behavior into a G0/G5 conformance
inventory so a future Go runtime can be judged against analytix contracts
rather than Reasonix-native protocols.
```

Capability matrix:

| Source | Absorb what | Why better | How to prove better | Conflict authority |
| --- | --- | --- | --- | --- |
| Reasonix `main-v2` engine ideas | Cache-shape diagnostics, provider/cache/tool lifecycle lessons, MCP malformed schema defense, denied-approval no-execute proof, checkpoint/rewind semantics, and future Go lock guidance. | Analytix gains engine rigor while keeping desktop UI, `window.analytix`, top-level `runtime`, and HTTP/SSE contracts. | P2.2/P3A/P4A/P4B/P4C focused tests, full runtime suite, `model-client.test.ts`, `mcp-tool-provider.test.ts`, `builtin-tools.test.ts`, checkpoint fixtures. | D-0003, D-0004, D-0007, D-0009. |
| Reasonix Go/runtime shape | Use only as future implementation inspiration after G0-G6 conformance gates. | Prevents a second runtime from drifting away from analytix renderer/preload/main contracts. | Go must reproduce TS oracle outputs for HTTP status, response schema, SSE sequence, durable logs, transcript projection, usage, error shape, and sanitized traces. | Spec 08 sections 11-13; G0/G5 inventory in `go-runtime-conformance.md`. |

G5 oracle inventory:

| Contract surface | Reasonix capability to absorb | TypeScript oracle / missing gate |
| --- | --- | --- |
| Provider request/stream parsing | Robust adapter behavior, stream usage, reasoning/tool deltas. | Freeze `model-client.test.ts`; add cross-backend provider-golden snapshots before Go G5 parity. |
| Cache/usage accounting | Stable prefix/cache hit-miss diagnostics. | Freeze `cache.test.ts`, `usage-service.test.ts`, loop cache diagnostics; live provider matrix remains missing. |
| Tool/MCP/approval safety | Tool schema normalization, lifecycle recovery, denial safety. | Freeze `mcp-tool-provider.test.ts`, `builtin-tools.test.ts`, loop tool metadata tests; route-level Go comparison remains pending. |
| Agent loop compaction/history repair | Long-session context management and valid provider history. | Freeze `loop.test.ts`, `context-compactor.test.ts`, `model-history-repair.test.ts`; golden JSON fixtures still needed before Go parity. |
| Checkpoint/rewind/apply | Rewind semantics adapted behind analytix plan/apply contracts. | Freeze P4A/P4B/P4C checkpoint tests; live desktop apply and crash/restart evidence remain missing. |

Skipped:

```text
No Reasonix CLI/settings/event protocol, Go scaffold, Rust helper, backend
switch, renderer protocol, or terminal-first workflow replacement was added.
```

Validation:

```text
npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1
```

The runtime suite baseline failures were traced to stale tests, not weakened
runtime contracts: post-`file_change` turns now require final assistant text,
and prompt-token compaction now rejects provider counts above the trust factor.

Remaining risks:

```text
Reasonix parity is not claimed for a Go backend. G1-G6 remain unimplemented;
cross-backend route fixtures, live provider matrix, packaged app QA, live
checkpoint apply, and crash/restart recovery evidence are still required.
```

## 2026-06-20 - Reasonix goal/control delta oracle

Source:

```text
Reasonix version/tag: main-v2 bc8249c307b261ef7ad05a0d2d1409c7b83b95ff
Requested range: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa..c23d40f74ca998941238154d6c0422676b139673
Actual refreshed range: be67a498adcaed6e33dcf3cbd25395e9cbccd6fa..bc8249c307b261ef7ad05a0d2d1409c7b83b95ff
Kun recheck: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928, v0.2.13 tag 201a1469ffbd911f6b95d450b78471e51b42acae / peeled 2ba8decc2f56862e7f677fcf89bbc3d402ec3a23, v0.2.14 tag 06be05d76223208724c07301fa0f830641a06e6f / peeled 8f2040349fba47fcd8e8b94f50b131943af839b2
Local branch: codex/reasonix-goal-control-delta-oracle from 96b43ed
```

Summary:

```text
Reasonix split goal-control state into a goalMachine and then, after the
requested merge point, split approval/ask bookkeeping into an approvalManager.
Analytix absorbed only the goal/control lesson behind its current TypeScript
runtime contracts: goal continuation, no-tool recovery, empty post-file-change
recovery, and goal resume state now live in an analytix-owned helper instead
of Reasonix-native marker, CLI, settings, or event shapes.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `dbaea8433380a4351fd1c2362ea9df175cb61e51` goal FSM extraction | goal/control runtime | `should absorb` | Added `packages/runtime/src/loop/goal-control-machine.ts` as an analytix-owned helper for goal instructions, blocked-audit text, no-tool recovery, empty post-file-change recovery, and non-progress tool classification. Persistence remains `ThreadService` / `threadStore`; no Reasonix sidecar or marker protocol is exposed. | Future Go G5 must preserve the same oracle behavior and keep persistence writes outside approval/status critical sections. |
| `bb06f5b4b6c78d58147799130dcd51db0c5e8913` inspect/package-lock cleanup | Reasonix repo hygiene | `classify only` | No analytix `internal/inspect` package or redundant Reasonix frontend lockfile exists. Root `package-lock.json` remains the Electron app lockfile; `object-inspect` is only an npm transitive dependency. | No code action. Recheck only if analytix later vendors Reasonix Go sources. |
| `726036bd15e09a208a59d3685b9de0dcb8c9811b` approvalManager extraction | approval/control runtime | `defer pending focused batch` | Analytix already has approval gate, runtime approval events/routes, renderer cards, and denied-approval no-execute tests. Do not expand this goal-control batch into approval manager rewiring. | Candidate future P3/G4 approval-control cleanup with route/renderer regressions. |
| `bc8249c307b261ef7ad05a0d2d1409c7b83b95ff` merge head | upstream drift | `record` | Recorded as true refreshed Reasonix HEAD for the goal/control batch, superseding the requested `c23d40f` merge point for audit purposes. | Later currentness refreshes supersede this as the drift starting point; see the `d02457ee` store-sidecar entry below. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Goal/control loop | Internal extraction only. `ThreadGoal`, `update_goal`, runtime events, HTTP/SSE, and renderer-visible shapes stay unchanged. | Compatible; no Reasonix `[goal:*]` marker or public protocol is adopted. | `packages/runtime/tests/goal-repetition-guard.test.ts`, `packages/runtime/tests/goal-tools.test.ts`, `packages/runtime/tests/loop.test.ts`. |
| Post-`file_change` final answer | Direct oracle now covers one recovery retry followed by `empty_post_tool_continuation` failure when the model still returns empty final text. | Compatible with the current stricter loop contract. | `loop.test.ts` "recovers once then fails when a file_change tool is followed by an empty final answer". |
| Prompt-token trust | Direct oracle now covers ignoring provider `promptTokens` above `PROMPT_TOKEN_TRUST_FACTOR` so inflated cache-read counts do not trigger compaction. | Compatible with the current anti-inflation contract. | `loop.test.ts` "ignores prompt token reports inflated beyond the trust factor". |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0 | G0 inventory now includes direct post-file-change and prompt-token anti-inflation oracle rows. | Keep these fixtures frozen before any backend comparison. |
| G4/G5 | Go must reproduce analytix goal-control helper behavior: blocked audit instructions, no-tool recovery, non-progress goal tools, empty file-change final recovery/failure, and failed-goal resume exhaustion. | Cross-backend loop fixture must match TS item/event/status output. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix goal markers/sidecar vs analytix thread goal contract | Absorb the FSM separation idea only; public state remains `ThreadGoal`, `goal_updated`, `window.analytix`, and analytix runtime HTTP/SSE. | D-0011 |
| Reasonix approvalManager drift after requested merge | Record and defer; approval/user-input split needs its own batch because it touches renderer/IPC expectations. | D-0011 |

Absorbed:

```text
packages/runtime/src/loop/goal-control-machine.ts
packages/runtime/src/loop/agent-loop.ts
packages/runtime/tests/loop.test.ts
```

Skipped:

```text
Reasonix public CLI/settings/event shape, Reasonix goal markers, Reasonix
goal-state sidecar format, Go controller locks, `internal/inspect` deletion,
desktop/frontend package-lock deletion, approvalManager rewiring, Kun identity,
`kun serve`, `window.kunGui`, `agents.kun`, and `KUN_*`.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/loop.test.ts tests/goal-tools.test.ts tests/goal-repetition-guard.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
Full Reasonix approval/control parity is not claimed. Go G1-G6 remain
unimplemented. Provider/settings schema, bridge canonical domains, release
metadata, packaged QA, live provider matrix, and verified analytix remote remain
separate gates.
```

## 2026-06-20 - Reasonix store-sidecar authority refresh

Source:

```text
Reasonix version/tag: main-v2 d02457ee7802256d66b9860276a4f00cd5baea56
Commit range: bc8249c307b261ef7ad05a0d2d1409c7b83b95ff..d02457ee7802256d66b9860276a4f00cd5baea56
Release notes: not used; remote HEAD and local git diff were inspected directly
Local comparison ref: refs/remotes/reasonix/main-v2 fetched from https://github.com/esengine/DeepSeek-Reasonix.git
```

Summary:

```text
Reasonix main-v2 advanced past the post-4f515da analytix goal/control baseline.
The only substantive commit is 98a57ded, merged by d02457ee, which moves
session sidecar path derivation into a new leaf `internal/store` package. The
upstream change is documented as byte-identical and no-migration. Analytix
records it as future Go store-boundary guidance only; it does not justify
changing the current TypeScript runtime, renderer protocol, settings schema,
or release evidence gate.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `98a57ded7b11924843519fc3b2230fe871755ae7` store-sidecar authority | session persistence / Go module boundary | `defer pending Go store batch` | No current code absorption. Analytix already owns session metadata through TypeScript file/hybrid stores and `metadata.jsonl` sidecars. Keep the Reasonix leaf-store idea as a future Go G2/G5 implementation boundary after route fixtures are scheduled. | Add a future Go store fixture that proves session meta, goal state, checkpoint, jobs, and cleanup marker paths match analytix contracts without exposing Reasonix file names. |
| `d02457ee7802256d66b9860276a4f00cd5baea56` merge head | upstream drift | `record` | Record as latest checked Reasonix `main-v2` HEAD. The post-4f515da analytix branch baseline remains `bc8249c3`; `be67a498` is now only a historical P3A checkpoint. | Future Reasonix batches start their drift check from `d02457ee` unless upstream moves again. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No accepted public contract change. | Renderer remains `window.analytix` -> main -> analytix runtime HTTP/SSE. | Existing runtime route and SSE fixtures remain authoritative. |
| Session/thread storage | No current TS storage change. | Existing `ThreadSummary`, hybrid store, checkpoint, goal, job, and metadata behavior remain analytix-owned. | Existing file/hybrid/checkpoint/goal tests remain the oracle; future Go store tests are pending. |
| Release gate | No release blocker is cleared by this upstream refactor. | Spec 07 blockers still apply. | Release evidence gate must still record live apply, crash/restart, packaged QA, provider matrix, release URL/repo metadata, signing/notarization, Windows NSIS, and verified remote status. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G2 | Potential leaf `store` package boundary for session sidecar layout. | Must reproduce analytix session/thread route fixtures before any Go storage module is accepted. |
| G5 | Potential shared persistence path authority for goal/checkpoint/jobs sidecars. | Must match TypeScript oracle paths/events and keep Reasonix-native filenames out of renderer-visible contracts. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix `internal/store` as upstream layout authority vs analytix current TS store contracts | Record the boundary idea only. Do not add Go/Rust runtime code, do not expose Reasonix store paths, and do not change `window.analytix` / top-level `runtime`. | D-0011 / Spec 08 section 24 |

Absorbed:

```text
No implementation was absorbed by this refresh entry.
```

Skipped:

```text
No Reasonix-native renderer protocol, Reasonix sidecar filenames, Go backend
scaffold, Rust/Tauri rewrite, Kun identity, `kun serve`, `window.kunGui`,
`agents.kun`, or `KUN_*` was added.
```

Validation:

```text
git fetch https://github.com/esengine/DeepSeek-Reasonix.git main-v2:refs/remotes/reasonix/main-v2 --prune
git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2
git log --oneline bc8249c307b261ef7ad05a0d2d1409c7b83b95ff..reasonix/main-v2
git diff --stat bc8249c307b261ef7ad05a0d2d1409c7b83b95ff..reasonix/main-v2
git show --stat --patch 98a57ded
```

Remaining risks:

```text
The future approvalManager batch remains deferred. Go G1-G6 are still
unimplemented. Release readiness remains blocked by Spec 07 evidence gaps and
the verified analytix remote requirement.
```

## 2026-06-20 - Reasonix SessionAPI Control-Port Currentness Refresh

Source:

```text
Reasonix version/tag: main-v2 48e5b990671ca1e579b895080e74babc1e17c333
Commit range: d02457ee7802256d66b9860276a4f00cd5baea56..48e5b990671ca1e579b895080e74babc1e17c333
Release notes: not used; remote HEAD and local git diff were inspected directly
Local comparison branch: /tmp/analytix-upstream-audit-reasonix-20260620 origin/main-v2
```

Commit log:

```text
3e625b91 refactor(control): introduce the SessionAPI driving port; migrate bot to it
```

Diff stat:

```text
internal/bot/gateway.go  | 20 +++++++++---
internal/bot/render.go   |  3 +-
internal/control/port.go | 83 ++++++++++++++++++++++++++++++++++++++++++++++++
3 files changed, 99 insertions(+), 7 deletions(-)
```

Summary:

```text
Reasonix introduced a driving-port boundary for frontend entry points. The new
control port splits lifecycle, turn-driving, and approval/ask behavior into
narrow interfaces, then migrates the bot gateway away from the full concrete
Controller. The stated behavior is unchanged upstream, but the architecture
matters for analytix because Connect Phone, scheduled tasks, and future
bot-like remote entry points should not gain accidental access to goal,
checkpoint, memory, or other controller surfaces.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `3e625b91` SessionAPI driving port | runtime control boundary / remote entry safety | `superseded` | Later scoped-absorbed as analytix-owned `RemoteEntryControlPort` boundaries and oracle tests. Keep public state as analytix HTTP/SSE, `ThreadGoal`, approval/user-input routes, and renderer contracts. | See the later control-port absorption section. |
| Reasonix bot gateway migration | bot / remote entry | `port-and-adapt` | Treat as proof that remote entry points should receive only lifecycle, turn, and approval capabilities. Do not copy Reasonix bot protocol or names. | Use Connect Phone / scheduled-task regressions as analytix proof. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No public change in this currentness patch. | Required; renderer must not know Reasonix ports. | Future control-port batch should preserve runtime route and SSE fixtures. |
| Approvals/user input | Narrows who can drive approvals/asks internally. | Compatible if existing approval routes/cards and denial no-execute behavior remain unchanged. | Future approval/user-input/Connect Phone focused tests. |
| Goal/checkpoint/memory | Remote entry points must not gain these surfaces accidentally. | Compatible because current public contracts stay unchanged. | Future negative oracle proving remote/bot-like control cannot call excluded surfaces. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0 | Add control-port boundary to the conformance inventory before Go work. | Document the TS oracle and expected port shape. |
| G1/G2 | Do not start until current TS runtime control boundary is explicit enough to compare. | Backend-neutral health/session routes remain unchanged. |
| G4/G5 | Go controller may use different internals, but remote entry points must satisfy the same lifecycle/turn/approval-only boundary and approval safety. | Cross-backend approval/user-input/tool/goal fixtures. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix `SessionAPI` naming vs analytix runtime contracts | Absorb the boundary idea, not the public name or protocol. | D-0011 |
| Control-port batch vs Go scaffold | Do the TypeScript oracle first; no Go scaffold in this patch. | Spec 08 sections 11-13 |

Absorbed:

```text
No code is absorbed in this currentness patch. The next batch should implement
or test an analytix-owned control-port boundary.
```

Skipped:

```text
Reasonix public CLI/settings/event protocol, Reasonix bot protocol, Go backend
scaffold, Rust/Tauri rewrite, Kun identity, `kun serve`, `window.kunGui`,
`agents.kun`, and `KUN_*`.
```

Validation:

```text
git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2
git log --oneline d02457ee7802256d66b9860276a4f00cd5baea56..origin/main-v2
git diff --stat d02457ee7802256d66b9860276a4f00cd5baea56..origin/main-v2
git show --stat --patch 3e625b91
```

Remaining risks:

```text
The control-port idea is not implemented in analytix yet. ApprovalManager
extraction remains deferred. Go G1-G6, live release QA, Windows NSIS machine
verification, release metadata, signing/notarization, and verified analytix
remote remain open.
```

## 2026-06-20 - Reasonix SessionAPI Control-Port Absorption Implemented

Source:

```text
Requested Reasonix baseline: main-v2 48e5b990671ca1e579b895080e74babc1e17c333
Implemented target commit: 3e625b91d9a7ee265d5e7fb88ed8f2fdae731d35
Current remote recheck: main-v2 c202f97035cd353c4bf3bbd2dc6e94ed22710e67
Post-target drift observed: 1280c0f port-serve/acp migration, 9ae96d8 port-cli migration, c202f97 merge
Local comparison branch: /tmp/analytix-upstreams/DeepSeek-Reasonix origin/main-v2
```

Summary:

```text
Analytix absorbed the Reasonix 3e625b91 interface-segregation idea without
copying the Reasonix public SessionAPI name, bot protocol, Go controller, or
renderer-visible event shape. The TypeScript runtime now has an
analytix-owned RemoteEntryControlPort composed from narrow services/gates:
lifecycle info, restricted turn start/steer/interrupt/get, approval decisions,
and user-input resolution. Remote turn starts cannot override provider,
approval policy, sandbox mode, GUI plan mode, or arbitrary local-path
attachments. Goal, checkpoint, memory, raw session storage, tool host, and
thread service internals stay off the remote/bot-like entry surface.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `3e625b91` driving-port/interface segregation | runtime control boundary / remote entry safety | `scoped absorbed` | Absorbed as `RemoteEntryControlPort` plus `createRemoteEntryControlPort`, not as Reasonix `SessionAPI`. | Future Connect Phone/schedule code may depend on this narrow port when those entry points are refactored. |
| Reasonix bot gateway migration | bot / remote entry | `port-and-adapt` | Converted into negative TypeScript and runtime oracle tests proving remote entries do not receive goal/checkpoint/memory/storage surfaces. | Keep bot/IM entry regressions on existing analytix Connect Phone and schedule tests. |
| `1280c0f`, `9ae96d8`, `c202f97` post-target port migrations | upstream drift | `record` | Current remote confirms Reasonix is continuing the same interface-segregation direction; do not broaden this batch past the requested 48e5/3e625 baseline. | Re-evaluate only if future serve/CLI migration changes behavior, not just frontend dependency wiring. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime HTTP/SSE | No public route or SSE contract change. | Renderer remains `window.analytix` -> preload -> main -> analytix runtime HTTP/SSE. | Existing runtime route/SSE suite plus remote-entry focused tests. |
| Runtime internal control surface | Adds a narrow `remoteEntryControl` object to `ServerRuntime`. | Full GUI runtime still owns thread/goal/checkpoint/memory/debug/storage services; remote entries get only the narrow object. | `packages/runtime/tests/remote-entry-control-port.test.ts`. |
| Approvals/user input | Internal remote entry can resolve existing gates through existing approval/user-input event contracts with request validation and duplicate-decision guards. | Existing approval routes/cards and denied no-execute behavior are unchanged. | `remote-entry-control-port.test.ts`, `ports.test.ts`, `builtin-tools.test.ts`, `user-input-disabled.test.ts`, `loop.test.ts`. |
| Goal/checkpoint/memory/storage | Explicitly excluded from the remote entry port. | Existing GUI and tool contracts remain available through full runtime surfaces only. | Type-level `@ts-expect-error` assertions and runtime key checks in `remote-entry-control-port.test.ts`. |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0 | Adds a concrete TS oracle for remote/bot-like entry boundaries before Go work. | Go must expose an equivalent analytix-owned narrow entry surface, not Reasonix public protocol. |
| G1/G2 | Health/session/turn implementations must keep renderer and desktop bridge backend-neutral. | `remote-entry-control-port.test.ts` becomes a required cross-backend fixture family once Go exists. |
| G4/G5 | Tool approval/user-input and goal/checkpoint/memory internals must not leak into remote entry adapters. | Denied approval no-execute, user-input disabled turns, and goal-loop tests remain required regressions. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Reasonix `SessionAPI`, `Lifecycle`, `TurnControl`, `Approvals` public naming vs analytix runtime ownership | Keep the concept, use analytix `RemoteEntryControlPort` naming and existing contracts. | D-0011 |
| Full `ServerRuntime` vs remote/bot-like entry needs | Full runtime remains a GUI/server composition object; remote entrants receive only the narrow `remoteEntryControl` object. | Spec 08 section 9 |
| Post-target Reasonix port migrations vs requested batch scope | Record currentness drift, but do not absorb additional serve/CLI migration code unless behavior changes. | Spec 08 section 7 |

Absorbed:

```text
packages/runtime/src/ports/remote-entry-control.ts
packages/runtime/src/services/remote-entry-control-port.ts
packages/runtime/src/server/routes/server-runtime.ts
packages/runtime/src/server/runtime-factory.ts
packages/runtime/tests/http-server-test-harness.ts
packages/runtime/tests/remote-entry-control-port.test.ts
```

Skipped:

```text
Reasonix public `SessionAPI` naming, Reasonix bot protocol, Go controller code,
Go scaffold, Rust/Tauri rewrite, renderer bridge changes, new HTTP routes,
goal/checkpoint/memory/storage exposure to remote entries, Kun identity,
`kun serve`, `window.kunGui`, `agents.kun`, and `KUN_*`.
```

Validation:

```text
git status --short --branch
git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2
git -C /tmp/analytix-upstreams/DeepSeek-Reasonix fetch origin main-v2:refs/remotes/origin/main-v2 --prune
git -C /tmp/analytix-upstreams/DeepSeek-Reasonix show --stat --oneline 3e625b91d9a7ee265d5e7fb88ed8f2fdae731d35
npm --prefix packages/runtime run test -- tests/remote-entry-control-port.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run typecheck
npm run typecheck
git diff --check
strict production identity/protocol scan with legacy-settings/test-fixture allowlist
diff and source Go/Rust scaffold scans
```

Remaining risks:

```text
This closes the requested TypeScript control-port/oracle batch, not full
Reasonix controller parity. The later Reasonix serve/acp/cli port migrations are
recorded as currentness drift only. Go G1/G2, live release QA, Windows NSIS,
release metadata, signing/notarization, and verified analytix remote remain
open gates.
```

## 2026-06-20 - Post-control-port currentness refresh

Source:

```text
Requested Reasonix main-v2 checkpoint: c202f97035cd353c4bf3bbd2dc6e94ed22710e67
Historical 2026-06-20 observed origin/main-v2 after fetch: 5d1ad2ae8cb7a0dbc8775fb3f05e6c204627c2b1
Current 2026-06-21 recheck: 49c14762b7da9234525e717830e39a64a2220911
Historical delta: c202f970..5d1ad2a
Current delta for future classification: 5d1ad2a..49c14762
```

Summary:

```text
The user-requested `c202f970` checkpoint is valid and remains the baseline for
the completed RemoteEntryControlPort absorption. The remote advanced during
the P0 batch to historical observed `5d1ad2a`, merging `dbb6f83a`
App-to-SessionAPI port work in desktop files. The latest recheck for this
stage is `49c14762`; both ranges are classified as currentness drift only and
are not absorbed with the P0 product-entry correction or Kun installer fix.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `dbb6f83a` / `5d1ad2a` desktop App migration to SessionAPI port | desktop/runtime adapter structure | `record` / `should assess` | Do not copy Reasonix public SessionAPI or desktop app wiring into analytix. The already-absorbed analytix `RemoteEntryControlPort` remains the scoped runtime boundary. | Future Reasonix app-port review only if a behavior delta improves analytix remote entry, runtime health, or desktop session safety. |
| `726036bd` approvalManager, still present before this delta | approval/control internals | `defer` | Do not start approvalManager in this P0 batch. | Start only after P0 entry correction, Kun installer, and release-safety evidence are closed or explicitly reprioritized. |
| Go G1/G2 scaffold pressure from Reasonix port layers | runtime backend | `defer` | No Go scaffold, backend switch, Rust helper, or Tauri work in this batch. | Begin G1 only as a conformance stage after TypeScript oracles and release blockers are stable. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Renderer/preload bridge | none | `window.analytix` remains the only renderer bridge. | Identity/protocol scan. |
| Runtime HTTP/SSE | none | `RemoteEntryControlPort` stays internal to the runtime composition; no Reasonix SessionAPI protocol leaks. | Existing remote-entry control-port oracle remains the guard. |
| Product routes/navigation | none | Reasonix does not decide analytix UI entries. | P0 Sidebar/Workbench/chat-store route guards cover the active UI correction. |

Remaining risks:

```text
Reasonix approvalManager and later SessionAPI port migrations may still contain
useful control-plane cleanup, but they must wait for a separate scoped batch.
Go G1 starts only after this P0 correction/installer batch and its validation
are complete; it must preserve `window.analytix`, top-level `runtime`, and the
TypeScript oracle surfaces.
```

## 2026-06-21 - Reasonix Cache/Engine Proof Methodology Hardening

Source:

```text
Reasonix main-v2 latest recheck: 49c14762b7da9234525e717830e39a64a2220911
Kun baseline context: v0.2.13 201a1469, v0.2.14 06be05d, master 8602476
Related decisions: D-0012, D-0013
```

Summary:

```text
Reasonix remains the engine/runtime upstream for analytix. Code-level reuse is
authorized for useful engine capabilities, but completion requires benchmark
proof. The existing P2.2 cache diagnostics are not a from-scratch task; the
next stage is proof closure for prefix stability, cache hit/miss behavior,
multi-provider compatibility, and Go conformance fixtures.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Reasonix cache shape / prefix-cache discipline | provider/cache | `should absorb` / proof required | Already scoped-absorbed as analytix `CachePrefixShape` / `CacheDiagnostics`; next work must prove parity or improvement against Reasonix behavior. | Add benchmark/provider matrix evidence before any stronger-than-Reasonix claim. |
| Reasonix provider usage/cache parsing | provider/runtime | `should absorb` | Reuse parser and fixture ideas behind analytix provider contracts only. | Prove DeepSeek cache hit/miss support and graceful unsupported behavior for other providers. |
| Reasonix public protocol/settings/CLI identity | protocol/product | `reject` | Do not expose Reasonix-native protocol, settings root, terminal-only product boundary, or renderer-visible events. | Identity/protocol scan remains mandatory. |
| Reasonix `main-v2` latest drift `49c14762` | currentness | `record` | Treat as the current upstream input for the next classification batch; do not mix into unrelated Kun product-entry or installer work. | Re-run diff before implementation. |

Proof requirements:

```text
stable prefix hash does not drift across equivalent turns
tool schema canonicalization ignores ordering noise
provider/model/endpointFormat drift is attributed
cache hit/miss tokens are parsed where available
unsupported providers remain functional and do not receive altered request shapes
tools, approvals, user input, generated files, and review UI do not regress
diagnostics stay sanitized and do not include prompts, tool args/output, files, API keys, or raw provider bodies
```

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0 | Existing TypeScript cache/provider diagnostics become oracle fixtures. | Freeze tests and benchmark scenarios before Go cache/provider work. |
| G1/G2 | Go may expose health/config/cache-shape shadow behavior only after current TS oracle is stable. | Renderer and preload remain backend-neutral. |
| G3/G5 | Go provider/agent-loop work must match cache diagnostics and provider matrix behavior. | TS/Go golden snapshots and scorecard evidence required. |

Remaining risks:

```text
Fixture-backed provider evidence now exists for the TypeScript oracle, but live
provider credentials are still needed for a real cache-hit comparison. Without
live provider evidence, analytix may claim scoped fixture-backed diagnostics,
but not final Reasonix cache parity or superiority.
```

## 2026-06-21 - P2.2 Cache/Provider Fixture Proof Closure

Source:

```text
Reasonix main-v2 current recheck: 49c14762b7da9234525e717830e39a64a2220911
Kun baseline context: v0.2.13 201a1469, v0.2.14 06be05d, master/develop 8602476/247076f
Related decisions: D-0012, D-0013
```

Summary:

```text
The existing analytix P2.2 cache diagnostics are advanced from scoped code
slice to fixture-backed proof. This is not a Go rewrite and not a Reasonix
public protocol import. The proof locks the TypeScript oracle for stable prefix
shape, canonical tool schema hashing, provider/model/endpoint attribution,
provider cache hit/miss parsing, and graceful unsupported behavior.
```

Implementation:

| Area | Result |
| --- | --- |
| Stable prefix / tools | Added `packages/runtime/tests/provider-cache-proof.test.ts` fixtures proving equivalent turns keep the same `prefixHash`, `toolsHash`, and `prefixItemsHash` despite volatile ids, tool order, and schema key-order noise. |
| Drift attribution | ProviderId / endpointFormat / model / tool changes are attributed through sanitized `CacheDiagnostics` fields without raw prompt or tool description text. |
| Provider cache matrix | Fixtures cover OpenAI-compatible unsupported usage, DeepSeek native `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`, OpenAI Responses `input_tokens_details.cached_tokens`, and Anthropic/MiniMax `cache_read_input_tokens` / `cache_creation_input_tokens`. |
| Unsupported fallback | `CompatModelClient.mapUsage`, streaming usage merge, and `UsageCounter` now preserve `cacheHitTokens` / `cacheMissTokens` as absent when a provider does not expose cache telemetry, instead of converting unknown support to 0% hit / all miss. |
| Runtime event contract | `CacheDiagnostics` now carries `cacheTelemetrySupported`; cache hit/miss tokens remain optional and are omitted for unsupported providers. |

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Fixture-backed P2.2 cache/provider proof | provider/cache | `should absorb` / implemented | Accept as TypeScript oracle evidence behind analytix runtime contracts. | Keep in Go G0/G3/G5 fixtures before Go cache/provider work. |
| Live provider cache-hit comparison | provider/cache | `defer pending evidence` | No live credentials are available in this workspace; do not claim final Reasonix cache superiority. | Run live DeepSeek/OpenAI/Anthropic/custom endpoint matrix when credentials are available. |
| Reasonix `49c14762` code drift | currentness | `record` | Current SHA recorded; no new code copied until a behavior delta is classified. | Re-run diff before next Reasonix implementation batch. |

Validation:

```text
git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2
npm --prefix packages/runtime run test -- tests/provider-cache-proof.test.ts tests/cache.test.ts tests/usage-service.test.ts --no-file-parallelism --maxWorkers=1
git diff --check
```

Remaining risks:

```text
Live provider credentials, packaged desktop QA, route-surface regressions,
identity/protocol scans, and full typecheck/test/build gates remain required
for final stage closure. Fixture evidence is enough to close the first
Reasonix P2.2 proof loop, but not enough to claim live provider superiority.
```

## 2026-06-21 - Agent-Kernel Five-Batch Governance Closure

Source:

```text
Reasonix main-v2 current recheck: 49c14762b7da9234525e717830e39a64a2220911
Kun context: v0.2.13 201a1469, v0.2.14 06be05d, master/develop 8602476/247076f
Related decisions: D-0012, D-0013, D-0014
```

Summary:

```text
This is a docs/spec/upstreams governance closure, not a runtime implementation
closure. It fixes the previous omission that described Reasonix mostly through
sub-agent and DeepSeek cache lenses. The authoritative route now treats
Reasonix as an agent/runtime kernel source: permissions, task closure,
long-running task state, tool/context economy, collaborative execution, and
future Go runtime kernel.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Task closure and permission kernel | permission / evidence / Goal | `must absorb` / first | Prioritize `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan approval vs tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules before sub-agent/parallel expansion. | Open the next implementation batch here. |
| Long-running task system | AutoResearch / research goal state | `must absorb` after permission kernel | Land `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, and requirement-by-requirement evidence audit in analytix-owned state only. | Design storage fixtures and restart/resume proof. |
| Tool surface and context economy | tools / context / cache / providers | `should absorb` | Extend token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory retrieval, compaction archive, and cache diagnostics without DeepSeek-only assumptions. | Provider matrix and scorecard evidence. |
| Collaborative execution model | task / parallel / background / coordinator | `needs redesign` / defer until gates | Treat `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, and nested event rendering as product-grade execution work after permissions/evidence/tool economy. | Do not claim completion from `subagents.enabled` or `delegate_task`. |
| Go runtime kernel | backend-neutral runtime | `defer pending TS oracle` | Future Go may carry Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime only behind analytix contracts and G0-G6. | No Go scaffold in this governance batch. |
| Reasonix `49c14762` and later drift | currentness | `record` | The SHA is recorded as the current baseline for this closure. Future drift must be rechecked and classified before implementation. | Re-run `git ls-remote` and diff on the next Reasonix code batch. |

Rejected surfaces:

```text
Reasonix terminal product shape
Reasonix public protocol/settings/CLI identity
renderer-visible Reasonix event semantics
top-level AutoResearch / Workflow / Subagent navigation
DeepSeek-only behavior
subagents.enabled or delegate_task as a parity claim
Go/Rust/Tauri scaffold as a shortcut
```

Validation:

```text
git diff --check
keyword scan for permission Gate, complete_step, evidence ledger,
AutoResearch, task_spec.md, connect_tool_source, parallel_tasks,
background jobs, planner/executor, Provider Registry, and Goal Runtime
route-surface, identity/protocol, and Go/Rust scaffold scans
```

Remaining risks:

```text
This closes the missing governance route only. It does not implement the full
Reasonix agent kernel, does not close Reasonix parity/superiority, and does not
remove release blockers such as Windows NSIS machine QA, signing/notarization,
packaged desktop QA, live provider matrix, or verified release metadata.
```

## 2026-06-21 - Agent-Kernel Batch 1 TS Proof Start

Source:

```text
Reasonix baseline for this implementation: main-v2 49c14762b7da9234525e717830e39a64a2220911
Upstream drift policy: no new Reasonix current drift is mixed into this batch;
the next Reasonix code batch must re-run currentness and classification first.
Related decisions: D-0013, D-0014
```

Summary:

```text
This is the first scoped runtime implementation for D-0014 Batch 1, not a
full Reasonix agent-kernel closure. Analytix now has an owned proof path for
task completion evidence: `complete_step` appends to `ThreadGoal.evidenceLedger`,
and goal completion is rejected without evidence at both the model-tool and
ThreadService layers. The implementation keeps `window.analytix`, top-level
`runtime` settings, `analytix serve`, and runtime HTTP/SSE unchanged.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `complete_step` evidence ledger | task closure / Goal | `must absorb` / implemented in TS | Implemented as an analytix-owned goal tool and `ThreadGoal.evidenceLedger`; renderer mapper only passes the field through. | Add richer requirement-by-requirement audit in the later AutoResearch batch. |
| Goal completion evidence gate | Goal state machine | `must absorb` / implemented in TS | `update_goal complete` and `ThreadService.setGoal(... status: complete)` reject missing evidence. | Add release-level UI/UX treatment only if a future spec requests visible evidence review. |
| Permission posture proof | permission Gate | `must absorb` / fixture-backed | Focused fixtures cover denied approval no-execute and the ask/auto/yolo mapping without adding a Reasonix public posture enum. | Broaden to route/API approval fixtures before claiming full Batch 1 closure. |
| Plan approval vs tool approval | plan/tool boundary | `must absorb` / fixture-backed | Plan-mode tool filtering proves plan approval does not widen mutating tool approval. | Keep `create_plan` gated; do not add Workflow top-level entry. |
| Blocked-state detection | Goal Runtime | `must absorb` / fixture-backed | Failed no-progress goal turns can transition the active goal to `blocked` and emit `goal_updated` plus `goal_auto_resume_exhausted`. | Add longer multi-turn blocked audit evidence before final Batch 1 parity claim. |
| Headless/subagent approval rules | subagent safety | `must absorb` / fixture-backed | Headless child runs inherit `approvalPolicy: never`; attempted tools receive `approval_policy_blocked` and do not execute. | Do not productize `parallel_tasks` until Batch 4. |

Rejected or unchanged:

```text
Reasonix public protocol/settings/CLI identity
Reasonix terminal UI shape
top-level Workflow / AutoResearch / Subagent navigation
DeepSeek-only runtime behavior
Go/Rust/Tauri scaffold or backend switch
Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`
```

Validation:

```text
npm --prefix packages/runtime test -- goal-tools.test.ts builtin-tools.test.ts child-agent-executor.test.ts loop.test.ts
```

Remaining risks:

```text
This starts Batch 1 but does not close full Reasonix parity or superiority.
Approval routes, renderer approval/review surfaces, user-input regressions,
broader runtime suite, app typecheck, full test/build gates, live provider QA,
and release evidence blockers still need validation before broader closure.
```

## 2026-06-21 - Agent-Kernel Batch 2 AutoResearch TS Proof

Source:

```text
Reasonix baseline for this implementation: main-v2 49c14762b7da9234525e717830e39a64a2220911
Upstream drift policy: no new Reasonix current drift is mixed into this batch;
the next Reasonix code batch must re-run currentness and classification first.
Related decisions: D-0013, D-0014
```

Summary:

```text
This is the second scoped runtime implementation for D-0014, not a full
Reasonix long-task parity closure. Analytix now has an owned TypeScript proof
for `/goal --research` through the existing chat/goal surface, backed by
project-local `.analytix/autoresearch/<threadId>/` state. The proof covers
`task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`,
`iteration_log.jsonl`, restart/resume behavior, attempted-direction tracking,
and requirement-by-requirement evidence audit through `complete_step
requirement_id`.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `/goal --research` | long-running task entry | `must absorb` / implemented in TS | Added as an existing goal/chat command path and runtime goal patch. It does not add a top-level AutoResearch route or Reasonix UI. | Add richer renderer evidence review only through future goal/runtime specs. |
| AutoResearch project-local state | long-running task state | `must absorb` / implemented in TS | State lands under `.analytix/autoresearch/<threadId>/` with `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`. | Add cleanup/export policy before release if needed. |
| Requirement evidence audit | task closure / Goal | `must absorb` / fixture-backed | Research goals cannot be marked complete until every requirement has evidence recorded through `complete_step requirement_id`. | Broaden to route-level and UI evidence surfacing before parity claims. |
| Direction tracking | research loop economy | `should absorb` / fixture-backed | Added `record_research_direction` so attempted directions are durable in `directions_tried.json` and `iteration_log.jsonl`. | Later Batch 3 can connect this to token economy/history retrieval. |
| Restart/resume | durability | `must absorb` / fixture-backed | Store fixtures prove project-local state can be resumed across store instances without relying on final text claims. | Add crash/restart desktop drill in release evidence. |
| Stable prefix/tool schema non-pollution | context safety | `must preserve` / fixture-backed | AutoResearch state is injected as per-turn context only; stable system prefix and tool schema do not receive project file contents. | Batch 3 must keep this invariant while adding dynamic tools/context economy. |
| Reasonix public protocol/settings/terminal shape | product boundary | `reject` | No Reasonix CLI/settings root, public event protocol, terminal-only UX, or top-level AutoResearch UI is exposed. | Keep identity/protocol scans mandatory. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime goal contract | `ThreadGoal.research` and evidence `requirementId` are analytix-owned optional fields. | Existing non-research goals keep the same behavior except stronger evidence gating from Batch 1. | `goal-tools.test.ts`, `http-server.test.ts`, `loop.test.ts`. |
| Renderer goal command | `/goal --research` maps to the existing `setActiveThreadGoal` path. | No new top-level route, sidebar entry, or renderer-visible Reasonix event semantics. | `FloatingComposer.test.ts`, `chat-store-maintenance-actions.test.ts`, mapper tests. |
| Project-local state | `.analytix/autoresearch/<threadId>/` is runtime-owned task state. | Does not write `REASONIX.md`, `AGENTS.md`, stable prefix data, or tool schema state. | `autoresearch-store.test.ts`, loop context fixture. |

Rejected or unchanged:

```text
Reasonix public protocol/settings/CLI identity
Reasonix terminal UI shape
top-level Workflow / AutoResearch / Subagent navigation
DeepSeek-only runtime behavior
Go/Rust/Tauri scaffold or backend switch
Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`
```

Validation:

```text
npm --prefix packages/runtime test -- autoresearch-store.test.ts goal-tools.test.ts http-server.test.ts loop.test.ts
npm run test -- src/renderer/src/components/chat/FloatingComposer.test.ts src/renderer/src/store/chat-store-maintenance-actions.test.ts src/renderer/src/agent/analytix-mapper.test.ts
npm run typecheck
npm run build:runtime
```

Remaining risks:

```text
This closes only scoped TypeScript proof for Batch 2. It does not implement
full Reasonix AutoResearch parity, background jobs, collaborative execution,
planner/executor coordination, live-provider research loops, desktop
crash/restart drills, or release-strength evidence. Batch 3 must still prove
multi-provider tool/context economy without DeepSeek-only assumptions.
```

## 2026-06-21 - Agent-Kernel Batch 3 Tool Source and Context Economy TS Proof

Source:

```text
Reasonix baseline for this implementation: main-v2 49c14762b7da9234525e717830e39a64a2220911
Upstream drift policy: no new Reasonix current drift is mixed into this batch;
the next Reasonix code batch must re-run currentness and classification first.
Related decisions: D-0013, D-0014
```

Summary:

```text
This is the third scoped runtime implementation for D-0014, not a full
Reasonix tool/context parity closure. Analytix now has an owned proof that
dynamic tool-source lifecycle changes are observable without destabilizing the
model-visible tool schema or stable cache prefix. `CapabilityRegistry` supports
dynamic source connect/disconnect semantics, canonical tool catalog fingerprints
stay stable across source connection order, and runtime cache diagnostics
report `toolSourceChanged` / `toolSourceIds` separately from `prefixChanged`.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `connect_tool_source` semantics | dynamic tool source lifecycle | `should absorb` / implemented in TS | Implemented as analytix-owned `connectToolSource` / `disconnectToolSource` registry semantics and diagnostics; no Reasonix public protocol is exposed. | Add user-facing source controls only through a future settings/runtime spec. |
| Dynamic tool schema stability | stable tool schema | `must preserve` / fixture-backed | Dynamic provider connection order no longer changes the canonical tool catalog fingerprint, while provider-owned advertisement order remains intact for tools such as MCP search meta tools. | Batch 4 task/subagent tools must preserve the same canonical fingerprint invariant. |
| Tool-source cache diagnostics | cache diagnostics | `should absorb` / implemented in TS | `CacheDiagnostics` now reports `toolSourceChanged`, `toolSourceChangeReasons`, `toolSourcesHash`, and `toolSourceIds` without adding source metadata to `prefixHash`. | Renderer may display diagnostics later without changing protocol identity. |
| Token economy | context economy | `already covered` / regression-checked | Existing token economy and usage-savings fixtures remain green. | Expand live cost evidence before superiority claims. |
| History/memory on-demand retrieval | context economy | `already covered` / regression-checked | Existing memory-store and request-history hygiene fixtures remain green. | Add richer retrieval ranking benchmarks before parity claims. |
| Compaction archive/history | context economy | `already covered` / regression-checked | Existing context-compactor and compaction-history fixtures remain green. | Add long-session desktop QA before release claims. |
| Multi-provider behavior | provider/cache | `must preserve` / regression-checked | Provider-cache fixtures for DeepSeek, OpenAI-compatible/Responses, Anthropic/MiMo-style usage, and unsupported-provider unknown fallback remain green. | Credentialed live provider matrix remains a blocker. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime tool registry | `CapabilityRegistry` gains dynamic connect/disconnect methods and richer diagnostics. | Existing tool host interface, approval policy, user-input flow, and model tool schema stay unchanged. | `capability-registry.test.ts`, `mcp-tool-provider.test.ts`. |
| Runtime cache diagnostics | Optional tool-source diagnostics are added outside `prefixChanged` semantics. | Historical usage events remain parseable because new fields are optional. | `cache.test.ts`, `loop.test.ts`. |
| Renderer contract | Optional cache-diagnostic fields are added for pass-through only. | No new UI route or renderer-visible Reasonix event semantics. | `analytix-mapper.test.ts`. |

Rejected or unchanged:

```text
Reasonix public protocol/settings/CLI identity
Reasonix terminal UI shape
top-level Workflow / AutoResearch / Subagent navigation
DeepSeek-only cache behavior
provider request body/header/stream/usage rewrites
Go/Rust/Tauri scaffold or backend switch
Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`
```

Validation:

```text
npm --prefix packages/runtime test -- capability-registry.test.ts cache.test.ts mcp-tool-provider.test.ts loop.test.ts
npm --prefix packages/runtime test -- token-economy.test.ts request-history-hygiene.test.ts context-compactor.test.ts compaction-history.test.ts memory-store.test.ts usage-service.test.ts provider-cache-proof.test.ts
npm run test -- src/renderer/src/agent/analytix-mapper.test.ts
npm run typecheck
```

Remaining risks:

```text
This closes only scoped TypeScript proof for dynamic tool-source/context
economy. It does not implement full Reasonix parity for dynamic tools, MCP
lifecycle UI, history/memory retrieval ranking, live provider cache/cost
evidence, collaborative execution, or Go runtime kernel work. Batch 4 must not
start unless Batch 1 and Batch 3 gates stay green.
```

## 2026-06-21 - Agent-Kernel Batch 4 Delegated Collaboration Evidence Proof

Source:

```text
Reasonix baseline for this implementation: main-v2 49c14762b7da9234525e717830e39a64a2220911
Upstream drift policy: no new Reasonix current drift is mixed into this batch;
the next Reasonix code batch must re-run currentness and classification first.
Related decisions: D-0013, D-0014
```

Summary:

```text
This is the fourth scoped runtime implementation for D-0014, not a full
Reasonix collaborative-execution parity closure. Analytix did not add a
`parallel_tasks` product surface or a top-level Subagent UI. Instead, the
existing `delegate_task`/child-agent substrate is now tied into the Batch 1
task-closure kernel: completed child runs can attach evidence to the parent
active Goal ledger, and nested child lifecycle metadata carries
`evidenceLedgered` / `evidenceLedgerError` through runtime event replay and
renderer projection.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `delegate_task` evidence handoff | collaborative execution / Goal | `should absorb` / implemented in TS | Completed child runs call an analytix-owned parent evidence hook; runtime factory records child completion into the active parent Goal ledger when one exists. | Future task/parallel APIs must keep this evidence handoff mandatory. |
| Existing parallel fan-out | collaborative execution | `already covered` / regression-checked | Existing loop fixture proves multiple `delegate_task` calls from one assistant message fan out in one batch. | Do not claim this equals Reasonix `parallel_tasks` product model. |
| Queue/cancel/failure paths | background/parallel safety | `already covered` / regression-checked | Existing `DelegationRuntime` fixtures cover `maxParallel` queueing, queued abort, child failure, and parent interruption states. | Full background jobs still need resumable job ids, durable cancellation routes, and restart proof. |
| Nested event rendering | runtime projection | `should absorb` / fixture-backed | Child lifecycle metadata now preserves `evidenceLedgered` in runtime reducer and renderer mapper output. | Product UI can later display richer nested events without a new top-level route. |
| Planner/executor Coordinator | collaborative execution | `defer` | Not implemented in this scoped proof. | Needs separate spec for planner/executor contracts, model pairing, cost budget, and evidence handoff. |
| `task` / `parallel_tasks` / background jobs | collaborative execution | `defer` | Not implemented or exposed. Existing `delegate_task` remains a bounded optional substrate. | Must be permission-gated, evidence-ledgered, cancellable, resumable, and fork/continue aware before productization. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Delegation runtime | `ChildRunRecord` carries optional `evidenceLedgered` / `evidenceLedgerError`; `DelegationRuntime` accepts an optional parent evidence hook. | Existing `delegate_task` inputs, tool name, approval policy, maxParallel behavior, and child execution semantics remain unchanged. | `delegation-runtime.test.ts`, `child-agent-executor.test.ts`, `loop.test.ts`. |
| Runtime events/projection | Child metadata carries optional evidence status through event schema and reducer. | Historical events remain parseable because new fields are optional. | `runtime-event-reducer.test.ts`. |
| Renderer projection | Child metadata pass-through preserves evidence status. | No new top-level UI or Reasonix renderer protocol. | `analytix-mapper.test.ts`. |

Rejected or unchanged:

```text
Reasonix public protocol/settings/CLI identity
Reasonix terminal UI shape
top-level Workflow / AutoResearch / Subagent navigation
new `parallel_tasks` product surface
background Job Manager productization
planner/executor Coordinator productization
Go/Rust/Tauri scaffold or backend switch
Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`
```

Validation:

```text
npm --prefix packages/runtime test -- delegation-runtime.test.ts child-agent-executor.test.ts loop.test.ts runtime-event-reducer.test.ts
npm run test -- src/renderer/src/agent/analytix-mapper.test.ts
npm run typecheck
```

Remaining risks:

```text
This closes only scoped TypeScript proof for evidence-ledgered delegated
collaboration. It does not implement full Reasonix `task`, `parallel_tasks`,
background jobs, planner/executor Coordinator, transcript continuation/fork,
or resumable job lifecycle parity. Batch 5 Go backend/scaffold work remains
blocked until full TS oracle validation stays green and the Go G0-G6 boundary
is explicitly opened; the following section records only TS shadow conformance.
```

## 2026-06-21 - Agent-Kernel Batch 5 G1 Shadow Scaffold

Source:

```text
Reasonix baseline for this implementation: main-v2 49c14762b7da9234525e717830e39a64a2220911
Upstream drift policy: no new Reasonix current drift is mixed into this batch;
the next Reasonix code batch must re-run currentness and classification first.
Related decisions: D-0013, D-0014
```

Summary:

```text
This is the fifth scoped implementation for D-0014, now advanced from
TypeScript-only G0/G1 shadow conformance to an isolated Go G1 shadow scaffold.
Analytix added `packages/runtime-go` only as conformance/shadow code for
`/health`, `/v1/runtime/info`, and `/v1/runtime/tools`; it did not add default
backend selection, Electron main integration, renderer-visible Go routes,
Reasonix public protocol, Rust scaffold, or Tauri migration. The shared oracle
fixture and tests keep the ten future Go runtime kernel components tied to
current TypeScript authority. The Kun 0.2.13 product-entry baseline is
unchanged by this batch.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Go kernel component manifest | Go runtime / conformance | `should absorb` / implemented in TS | `packages/runtime/src/conformance/go-runtime-kernel-conformance.ts` maps Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime to TS authority files and oracle tests. | Future Go G2+ work must continue consuming this as its implementation checklist. |
| G1 shadow route oracle | health/config/capabilities | `must before Go G1` / fixture-backed | `packages/runtime/src/conformance/fixtures/go-g1-shadow-oracle.json` and `packages/runtime/tests/go-runtime-conformance.test.ts` verify `/health`, `/v1/runtime/info`, and `/v1/runtime/tools` against existing analytix HTTP/auth/contracts. | Keep this fixture as the cross-backend runner source for G2 route replay. |
| Product boundary flags | backend neutrality | `must preserve` / fixture-backed | Manifest encodes bridge unchanged, `analytix serve` unchanged, no Reasonix protocol, no default Go backend, and no renderer-visible Go route. | Keep static scans mandatory before any Go implementation commit. |
| AutoResearch requirement audit hardening | Goal Runtime oracle | `must preserve` / implemented in TS | Unknown `requirement_id` evidence is rejected without writing findings, making requirement-by-requirement audit stricter. | Future Go Goal Runtime must match this rejection path. |
| Go G1 shadow scaffold | runtime implementation | `should absorb` / implemented as shadow-only | `packages/runtime-go` implements only the three G1 conformance handlers and compares them against the TS oracle fixture with `go test ./...`. | G2+ route replay, provider/cache shadow parity, backend switching, rollback, and desktop QA remain deferred. |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
| Runtime conformance manifest and Go G1 shadow package | New internal manifest, shared fixture, and isolated Go handler package. | No public HTTP/SSE, renderer, preload, settings, CLI, or default backend behavior changes. | `go-runtime-conformance.test.ts`; `go test ./...` in `packages/runtime-go`. |
| AutoResearch evidence audit | Unknown `requirement_id` now fails instead of writing orphan findings. | Valid requirement evidence and general research evidence remain supported. | `autoresearch-store.test.ts`. |

Rejected or unchanged:

```text
Reasonix public protocol/settings/CLI identity
Reasonix terminal UI shape
top-level Workflow / AutoResearch / Subagent navigation
default Go backend or renderer-visible Go routes
G2+ route migration
agent-loop Go rewrite
Rust/Tauri scaffold or backend switch
Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`
```

Validation:

```text
go test ./... (inside packages/runtime-go with a local Go toolchain)
npm --prefix packages/runtime test -- autoresearch-store.test.ts go-runtime-conformance.test.ts
```

Remaining risks:

```text
This closes only G1 shadow scaffold proof. It does not implement a default Go
backend, Electron supervisor integration, cross-backend route comparison, Go G2
session/SSE replay, Go G3 provider streaming, Go G4 tools/MCP/sandbox, Go G5
agent loop/Goal Runtime/Job Manager, rollback, desktop QA, packaged QA, or
release readiness.
```

## 2026-06-21 - Reasonix Latest Currentness, Parity Hardening, and G2 Replay Draft

Source:

```text
Previous analytix Reasonix governance baseline: main-v2 49c14762b7da9234525e717830e39a64a2220911
Latest Reasonix main-v2 recheck: 91fe06db6177bb052fc8ae1a3081d60bfc104a4e
Substantive commits: 45367085bfed769e09021375a9120dd2d459cee2, d9e453a1bedd675c226660cab033da35421b141d
Merge commits: 85ff9865, 91fe06db
Kun baseline context: v0.2.13 201a1469, v0.2.14 06be05d, master/develop 8602476/247076f
Related decisions: D-0013, D-0014, D-0015
```

Summary:

```text
Reasonix latest moved beyond the D-0014 Batch 1-5 governance baseline. This
stage classifies the latest drift, hardens analytix-owned parity fixtures, and
adds Go G2 shadow route replay behind the existing TypeScript oracle. No
Reasonix public protocol, settings shape, renderer bridge, CLI, or product
entry is imported. Xiaomi/MiMo presets remain analytix product behavior; live
provider parity is still blocked by missing credentials.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `45367085` codebase-memory MCP auto-indexing | MCP / dynamic tool source / workspace indexing | `wrap-behind-contract` | Record the pattern: known cwd-aware indexer MCP servers may need workspace-root cwd, low-priority scheduling, and background startup even under a shared host so initialize can trigger auto-indexing. This stage adds generic MCP/tool lifecycle oracle coverage for connect/disconnect/reload/cancel/error and secret-safe diagnostics, but does not hard-code Reasonix protocol or plugin names into renderer-visible contracts. | Future Batch 3/G4 work should add an analytix-owned codebase-memory/indexer fixture for shared-host background start and low-priority process hints if product requirements call for it. |
| `d9e453a1` MiMo built-ins migrated to custom providers | provider config / presets / model switcher | `reject` for preset removal; `contract-reimplement` for future legacy repair | Reasonix removed MiMo as official built-ins and treats OpenAI-compatible vendors as custom provider instances. Analytix currently keeps Xiaomi/MiMo presets for chat, speech, token-plan, and first-run UX, so deleting them would violate current product behavior. The migration/repair idea remains useful only behind analytix provider profiles and top-level `runtime` settings. | Future provider batch may add explicit legacy MiMo/custom-provider import repair and unknown-official-provider no-silent-fallback tests; live provider credentials are required before cache/provider superiority claims. |
| `85ff9865` / `91fe06db` merge commits | currentness | `record` | Record as latest branch heads only. | Future code batch starts diff/classification from `91fe06db` unless upstream moves again. |
| Go G2 read/list/search/archive/fork/session/SSE replay | Go runtime conformance | `shadow-only closed` | Added a shared G2 oracle and Go shadow replay handler/tests. G2 reuses TS route fixtures and keeps Go shadow-only; no renderer/preload/main bridge change, default backend, Electron supervisor switch, or renderer-visible Go route is authorized. | Future Go G3/G4/G5 work must keep using TS oracles and needs a separate backend-selection spec before any production routing. |

Contract impact:

| Contract | Current decision |
| --- | --- |
| `window.analytix` / preload / renderer | Unchanged. No Reasonix event protocol or Go route is visible to the renderer. |
| Settings | Active settings remain top-level `runtime` with provider profiles under current analytix settings. No Reasonix settings root and no MiMo preset removal in this batch. |
| Runtime HTTP/SSE | Go runtime is the default desktop runtime core. G1/G2 Go shadow package remains conformance-only. |
| Product entry | Kun 0.2.13 -> 0.2.14 baseline remains active. No top-level Workflow, AutoResearch, Subagent, or MCP-indexer route is added. |

Validation / production scans:

| Gate | Evidence | Result |
| --- | --- | --- |
| Reasonix latest | `git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2` | pass: `91fe06db6177bb052fc8ae1a3081d60bfc104a4e` |
| Reasonix delta log/stat | `git log --oneline 49c14762..analysis/main-v2`; `git diff --stat 49c14762..analysis/main-v2` | pass: substantive commits are codebase-memory MCP auto-indexing and MiMo/custom-provider migration; 32 files changed |
| Kun baseline | `git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14` | pass: `8602476` / `247076f` / `201a146` / `06be05d` |
| Go G1/G2 TS oracle | `npm --prefix packages/runtime run test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts tests/provider-cache-proof.test.ts tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1` | pass: 4 files, 10 tests |
| Go G1/G2 Go package | temporary official `go1.26.4.darwin-arm64` from `go.dev`, SHA256 verified, then `cd packages/runtime-go && go test ./...` | pass; temporary toolchain removed |
| Product route guards | `npm run test -- src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts -- --runInBand` | pass: 3 files, 9 tests |
| Bridge domain scan | spec 06 `window.analytix` domain-regex scan | pass: no hits |
| Forbidden identity/protocol scan | `rg` for `window.kunGui`, `agents.kun`, `kun serve`, `KUN_`, Reasonix-native, Cargo/Tauri/Rust terms in production paths | pass: no hits |
| Go scope scan | `rg` for default/backend switch terms in `src`, `preload`, `shared`, config, scripts, workflows | pass: no hits |
| G2 shadow replay | shared G2 oracle + `packages/runtime-go/shadow_g2.go` / `shadow_test.go` | pass as shadow-only conformance; no Electron/default backend path |
| Live provider matrix | credential presence scan for DeepSeek/OpenAI/Anthropic/MiMo/MiniMax keys | blocked: all checked provider keys missing |

Remaining risks:

```text
Live provider matrix and cost reconciliation remain unclosed; this is the most
important blocker for any MiMo/provider/cache superiority claim. Go G2 route
replay is accepted only as shadow conformance; no desktop supervisor backend
flag, rollback, packaged QA, Windows QA, signing, release evidence, or default
Go backend exists.
```

## 2026-06-21 - P0 Engine Parity Closure Against Reasonix 91fe06db

Source:

```text
Reasonix main-v2: 91fe06db6177bb052fc8ae1a3081d60bfc104a4e
Key reference files:
  internal/agent/cache_shape.go
  internal/agent/cachehit_e2e_test.go
  internal/provider/openai/openai.go
  internal/provider/anthropic/anthropic.go
  internal/agent/task.go
  internal/agent/parallel_tasks.go
  internal/jobs/jobs.go
  internal/tool/builtin/bgjobs.go
  internal/plugin/known_overrides.go
  internal/plugin/plugin.go
  internal/plugin/lazy.go
  internal/plugin/transport_stdio.go
  internal/plugin/stdio_cancel_test.go
```

Summary:

```text
This stage absorbs Reasonix engine behavior only behind analytix contracts. It
does not import Reasonix public protocol, CLI identity, settings root, renderer
event shape, or terminal-first product surfaces.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| DeepSeek/OpenAI/Anthropic cache and reasoning usage | provider/cache | `must absorb` | Implemented in the TypeScript runtime provider/cache proof matrix with sanitized attribution and canonical tool schema hash. | Live provider matrix and cost reconciliation remain required before superiority claims. |
| `task`, `parallel_tasks`, background jobs, planner/executor | task orchestration | `contract-oracle first` | Added analytix-owned task job contract/oracle plus durable manager skeleton; kept routes internal and did not expose top-level Workflow/Subagent/AutoResearch. | Future live background restart/resume and desktop nested-card QA. |
| codebase-memory/codegraph known overrides | MCP/tool lifecycle | `wrap-behind-contract` | Implemented stdio workspace-root cwd fallback only when cwd is not explicit, plus low-priority/background-start diagnostics, cancel/timeout/schema-cache and secret-safe proof. | Live indexer operation and product UI remain deferred. |
| Go G3/G4 | Go conformance | `shadow-only` | Added TS-owned G3/G4 fixtures and Go output comparison; no Electron main integration or default backend. | Go G5/full loop and rollback plan. |

Validation:

```text
Focused runtime suites passed for provider/cache, MCP lifecycle, task jobs, and
Go G3/G4 conformance. `packages/runtime-go` passed `go test ./...` with a
temporary official Go 1.26.4 Darwin arm64 toolchain after SHA256 verification.
```

Remaining risks:

```text
Live provider/MCP QA, packaged desktop QA, release metadata, signing, Windows
NSIS, and default Go backend readiness remain blockers.
```

## YYYY-MM-DD - Template

Source:

```text
Reasonix version/tag:
Commit range:
Release notes:
Local comparison branch:
```

Summary:

```text
<What changed upstream and why it matters to analytix>
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
|  |  | `must absorb` / `should absorb` / `optional` / `reject` / `needs redesign` / `defer pending evidence` |  |  |

Contract impact:

| Contract | Change | Backward compatibility | Tests |
| --- | --- | --- | --- |
|  |  |  |  |

Go runtime impact:

| Stage | Impact | Gate |
| --- | --- | --- |
| G0/G1/G2/G3/G4/G5/G6 |  |  |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
|  |  |  |

Absorbed:

```text
<List accepted changes and target analytix files/modules>
```

Skipped:

```text
<List rejected/deferred changes and why>
```

Validation:

```text
<Commands, conformance tests, traces, GUI QA, or blockers>
```

Remaining risks:

```text
<Known gaps and next actions>
```

## 2026-06-21 - Reasonix 9e56 Currentness, MCP Startup, And Task Job Route Parity

Source:

```text
Reasonix main-v2 range:
  91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b
Substantive commits:
  dffb6973 fix(todo): preserve cleared todo state on reload
  02b25d8b fix(todo): keep archived todo args bounded after clear
  7ba42955 fix(mcp): remove lazy startup mode
  e5a48d9c fix(mcp): add retry all for available servers
  315c95b3 fix(mcp): address startup retry review feedback
Merge commit:
  027e7dc0 Remove lazy MCP startup mode
```

Summary:

```text
This refresh closes the next currentness gate after the P0 engine-parity
stage. Todo drift is record-only because analytix owns structured todo state.
MCP startup drift is reimplemented behind analytix capability contracts.
Task-job work advances from oracle/manager skeleton to authenticated internal
runtime routes, without adding a product-level Workflow/Subagent/AutoResearch
surface.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `dffb6973` empty todo reload | todo | `record` | Analytix `todo_write` persists structured empty todo lists through `ThreadService.setTodos`; no transcript-rebuild import is needed. | Add a future todo empty-clear regression only if transcript-derived todo state is introduced. |
| `02b25d8b` bounded archived todo args | todo / UI archive | `record` | Analytix does not use Reasonix frontend tool-args archive compaction for todo state. | Keep as a docs invariant; do not copy Reasonix `useController` archive behavior. |
| `7ba42955` remove lazy MCP startup | MCP startup | `wrap-behind-contract` | Keep analytix `McpServerConfig.enabled/backgroundStart/trustScope` and parallel enabled-server startup; do not import Reasonix lazy/tier config. | Live indexer QA remains future work. |
| `e5a48d9c` retry all available MCP servers | MCP retry | `contract-reimplement` | Background reconnect now has test coverage for retrying every failed startup server. | Manual retry-all UI/API remains a separate analytix control decision. |
| `315c95b3` retry review feedback | MCP control drift | `contract-reimplement` | `CapabilityRegistry.suspendToolSource` prevents late reconnect from reinstalling disabled providers; `resumeToolSource` requires explicit re-enable. | Future live per-server enable/disable can build on the tombstone semantics. |
| `027e7dc0` merge | MCP currentness | `record` | Merge-only record for the MCP batch. | None. |

Absorbed:

```text
packages/runtime/src/adapters/tool/capability-registry.ts
packages/runtime/src/adapters/tool/mcp-tool-provider.ts
packages/runtime/src/server/runtime-factory.ts
packages/runtime/src/delegation/job-manager.ts
packages/runtime/src/server/routes/task-jobs.ts
packages/runtime/src/server/routes/index.ts
packages/runtime/src/server/routes/server-runtime.ts
src/main/provider-connection.ts
```

Rejected / deferred:

```text
Reasonix todo transcript rebuild/archive UI code
Reasonix lazy/tier MCP config model
Reasonix SessionAPI / CapabilitiesPanel retry-all protocol
Reasonix public task/parallel/planner protocol
top-level Workflow/Subagent/AutoResearch/MCP-indexer routes
default Go backend or Electron-to-Go routing
```

Validation:

```text
npm --prefix packages/runtime test -- capability-registry.test.ts mcp-tool-provider.test.ts task-job-orchestration-oracle.test.ts
npm run test -- src/main/provider-connection.test.ts
```

Remaining risks:

```text
First-class `task`, `parallel_tasks`, planner/executor Coordinator, restart
crash drill, live MCP/indexer QA, live provider matrix, and Go G5/full-loop
parity remain open.
```

## 2026-06-21 - Collaborative Execution, Cache Curve, MCP Indexer, And Go G5 Oracle Closure

Source:

```text
Reasonix main-v2 current target:
  9e56c3276880ced538b3375329b8a0ebbe63a67b
Referenced Reasonix implementation families:
  internal/agent/task.go
  internal/agent/parallel_tasks.go
  internal/jobs/jobs.go
  internal/agent/cachehit_e2e_test.go
```

Summary:

```text
This stage advances analytix-owned runtime oracles for Reasonix collaborative
execution and cache behavior after the 9e56 currentness closure. It does not
import Reasonix public protocol, SessionAPI, frontend surfaces, or default Go
backend behavior.
```

Absorbed:

| Reasonix source idea | analytix result |
| --- | --- |
| `task` / background job fields | Internal `TASK_TOOL_CONTRACT` pins `prompt`, `run_in_background`, `continue_from`, `fork_from`, permission gate, internal-only scope, and parent Goal evidence eligibility. |
| `parallel_tasks` dependency model | Internal `PARALLEL_TASKS_TOOL_CONTRACT` pins `depends_on`; `normalizeParallelTaskPlan` maps snake_case to TypeScript runtime shape; validation rejects fewer than two tasks, duplicate ids, self dependencies, unknown dependencies, and cycles. |
| jobs wait/output/kill | Existing authenticated `/v1/runtime/task-jobs/wait|output|kill` remains the only runtime route surface; oracle now ties it to first-class task/background/planner constraints. |
| nested child events / evidence | Oracle pins parent call id, child run id, `evidenceLedgered`, and `evidenceLedgerError` metadata as backend-neutral SSE/projection requirements. |
| DeepSeek cache guard | Provider-cache proof adds an offline cache curve release guard with 90% tail threshold and a bounded too-small-window allowed-low case. |
| MCP retry/indexer lifecycle | MCP lifecycle oracle covers retry-all failed startup servers, suspended late-provider tombstones, and codegraph/codebase-memory cwd/priority/backgroundStart behavior. |
| Go G5 proof | `go-g5-full-loop-oracle.json` freezes TS-owned full-loop/job/cache/compaction/resume/interrupt/MCP inventory while Go remains shadow-only. |

Rejected / deferred:

```text
Reasonix public task/parallel/planner protocol
Reasonix SessionAPI or frontend task cards
top-level Subagent/Workflow/AutoResearch/MCP-indexer navigation
live provider/cache superiority without credentials
live MCP/indexer parity without runtime environment QA
default Go backend or Electron main integration
```

Validation:

```text
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts provider-cache-proof.test.ts mcp-tool-lifecycle-oracle.test.ts go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts
git diff --check
```

Remaining risks:

```text
Full planner/executor Coordinator, restart/resume crash drill, desktop nested
card QA, live provider/MCP matrix, and Go G5 implementation remain open.
```

## 2026-06-21 - Executable Internal Runtime, Cache Guard, And Live-Local MCP Addendum

Source:

```text
Reasonix main-v2 current target:
  9e56c3276880ced538b3375329b8a0ebbe63a67b
Referenced Reasonix implementation families:
  internal/agent/task.go
  internal/agent/parallel_tasks.go
  internal/jobs/jobs.go
  internal/agent/cachehit_e2e_test.go
```

Summary:

```text
This addendum moves analytix from contract/oracle-only collaborative execution
to an internal executable task provider. It also makes the cache curve guard a
runtime-owned module and adds a fake local MCP/indexer lifecycle proof. Public
protocol, frontend task surfaces, and default backend behavior remain rejected.
```

Absorbed:

| Reasonix source idea | analytix result |
| --- | --- |
| `task` / `parallel_tasks` tool execution | `buildTaskJobToolProviders` registers internal-only `task` and `parallel_tasks` providers backed by `DelegationRuntime` and `DurableTaskJobManager`. |
| Background job output | `DurableTaskJobManager.output` accepts `offset` and returns `nextOffset`, so clients can read repeatable partial output without transcript scraping. |
| Restart-safe durable jobs | Runtime startup calls `reconcileRunningJobs()` to fail stale queued/running jobs after a process restart instead of leaving phantom active work. |
| Parallel dependency order | Executable test coverage proves dependency ordering for `parallel_tasks` in addition to the existing validation oracle for duplicate/self/unknown/cycle cases. |
| Cache hit curve proof | `evaluateOfflineCacheCurveGuard` is an analytix-owned release guard, not a copied Reasonix test helper; DeepSeek native hit/miss fields win over conflicting generic cached-token telemetry. |
| MCP/indexer lifecycle | `simulateLiveLocalIndexerLifecycle` proves retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics using local fixtures only. |

Rejected / deferred:

```text
Reasonix public task/planner protocol
Reasonix SessionAPI / frontend task cards
Reasonix plugin protocol
top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation
live provider/cache superiority without credentials
live MCP/indexer parity without live environment QA
full planner/executor Coordinator and durable runner rehydration
default Go backend or Electron main integration
```

Validation:

```text
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- provider-cache-proof.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- mcp-tool-lifecycle-oracle.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run typecheck
git diff --check
```

Remaining risks:

```text
Planner/executor Coordinator parity, task-card desktop QA, runner rehydration
after restart beyond stale-job reconciliation, live provider/cache matrix, live
MCP/indexer QA, and Go G5 implementation remain open.
```

## 2026-06-21 - Reasonix bfe398 PowerShell Drift And Go G5 Shadow Slice

Source:

```text
Reasonix main-v2 range:
  9e56c3276880ced538b3375329b8a0ebbe63a67b..bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0
Substantive commits:
  3569f3a4 fix(sandbox): detect standard pwsh install path
  32addaec fix(shell): describe pwsh chaining support
Merge commits:
  50ff43d8 merge main-v2 into pwsh compatibility PR
  bfe398cc Merge pull request #4957 from linze0721/fix/daf48ba5-00c6-4ab1-8150-148d9363260f
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| standard PowerShell 7 path detection | shell/sandbox compatibility | `record / future contract-reimplement` | Analytix desktop terminal already checks the standard PowerShell 7 MSI path for interactive terminal sessions. Runtime `bash` execution is capability-gated and not changed in this G5 slice. | Add a future runtime shell-tool compatibility fixture only if Windows host-shell behavior becomes part of the analytix runtime contract. |
| PowerShell chaining guidance | shell tool UX | `record` | Reasonix updates tool description text for pwsh `&&`/`||`. Analytix prompt/tool guidance remains product-owned and is not copied in this stage. | Future Windows shell UX batch may compare builtin bash tool instructions. |

Go G5 absorbed in this stage:

```text
packages/runtime-go/shadow_g5.go
packages/runtime-go/shadow_test.go
packages/runtime/src/conformance/fixtures/go-g5-full-loop-oracle.json
packages/runtime/src/conformance/runtime-parity-fixtures.ts
packages/runtime/tests/go-runtime-conformance.test.ts
```

Decision:

```text
Go G5 advances from TS-owned inventory only to a shadow output slice. The Go
package now replays the G5 full-loop inventory into a comparable output that
lists source oracles, full-loop tests, job routes/behaviors, cache/compaction
tests, resume/interrupt tests, MCP/indexer tests, blockers, and product
boundary flags. It remains shadow-only and is not wired into Electron main or
`analytix serve`.
```

Rejected / deferred:

```text
Reasonix shell/sandbox code copy, Reasonix public protocol, renderer-visible Go
routes, default Go backend, Electron-to-Go routing, live Go executor,
planner/executor Coordinator parity, live provider/MCP parity, and release
readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining risks:

```text
Go G5 is still a shadow output slice, not a live full-loop executor. Full
jobs/cache/session/resume/interrupt/MCP implementation, Electron integration,
rollback/default-backend decision, packaged QA, and live provider/MCP matrix
remain open.
```

## 2026-06-21 - Go G5 Composite Shadow Replay Addendum

Summary:

```text
The G5 shadow slice now goes beyond replaying the G5 inventory file. Go reads
the task-job, provider-cache, G2 route replay, and MCP lifecycle fixtures and
builds a composite shadow replay output from their concrete fields.
```

Absorbed:

| Slice | Go shadow replay evidence |
| --- | --- |
| Jobs | `task` / `parallel_tasks` names, internal task-job routes, dependency order, planner read-only tools, nested SSE metadata fields, and transcript identity flags are read from `task-job-orchestration-oracle.json`. |
| Cache | Stable prefix hash, tools hash, provider usage case ids, offline release guard statuses, forbidden diagnostics substrings, and live-superiority policy are read from `provider-cache-oracle.json`. |
| Session/resume/interrupt | G2 route ids, resume route ids, fork route ids, and SSE replay route ids are read from `go-g2-route-replay-oracle.json`. |
| MCP/indexer | Provider id, retry failed servers, expected connected servers, known override, live-local active paths, late tombstone path, and redacted diagnostics are read from `mcp-tool-lifecycle-oracle.json`. |

Rejected / deferred:

```text
This is not a live Go executor, not a Go Job Manager, not a Go MCP client, not
Go cache/session runtime behavior, not Electron integration, not a default Go
backend, and not release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Remaining risks:

```text
The composite replay proves cross-fixture Go shadow conformance only. Live
jobs/cache/session/resume/interrupt/MCP implementation, backend selection,
rollback, packaged QA, and live provider/MCP validation remain open.
```

## 2026-06-21 - Planner Executor Coordinator, Windows Shell, Live-Local Evidence, And G5 Runner Shadow

Source:

```text
Reasonix main-v2 current target:
  bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0
Referenced Reasonix implementation families:
  internal/agent/coordinator.go
  internal/agent/task.go
  internal/agent/parallel_tasks.go
  internal/jobs/jobs.go
  internal/agent/cachehit_e2e_test.go
  PowerShell compatibility commits 3569f3a4 and 32addaec
Kun refs rechecked:
  v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
```

Summary:

```text
This stage turns several previously deferred or oracle-only Reasonix items into
analytix-owned executable proof. It still keeps every public contract unchanged:
`window.analytix`, top-level `runtime` settings, `analytix serve`, and Renderer
-> preload -> main -> runtime HTTP/SSE remain authoritative.
```

Absorbed:

| Reasonix source idea | analytix result |
| --- | --- |
| planner/executor coordinator | Added `runPlannerExecutorCoordinator` as an internal runtime helper: planner jobs use read-only policy, executor waves follow the validated DAG, failed dependencies are skipped instead of executed, cancellation kills active jobs, output offsets are recorded, and parent call/child run metadata is preserved. |
| jobs durable runner | `DurableTaskJobManager.rehydrateRunnableJobs` restarts persisted queued/running jobs when a runner is available and fails them explicitly when it is not. The restart drill proves rehydrated jobs can output, wait, and kill. |
| `parallel_tasks` failure propagation | Internal `parallel_tasks` execution now treats failed dependencies as blockers and returns skipped downstream tasks with `isError: true`. |
| PowerShell 7 standard path and pwsh chaining | Runtime shell selection checks standard PowerShell install paths after `where` lookup, recognizes pwsh as supporting `&&` / `||`, blocks unsupported unquoted chaining for Windows PowerShell, and allows quoted literal text. |
| DeepSeek cache live-local evidence | Provider-cache proof now talks to an executable local OpenAI-compatible server, verifies the final request path, and proves native DeepSeek hit/miss telemetry wins over conflicting generic cached-token fields with reasoning-token parsing intact. |
| MCP/indexer lifecycle | The previous fake lifecycle proof is backed by an executable stdio MCP fake indexer server with persistent state, restart/resume, tombstone, cwd, low-priority/backgroundStart diagnostics, and secret-safe output. |
| Go G5 shadow | Go composite replay now reads planner/executor and durable-runner restart fields from `task-job-orchestration-oracle.json` and includes them in `shadowSlicesExpectedOutput`; it remains shadow-only. |

Rejected / deferred:

```text
Reasonix public task/planner protocol
Reasonix SessionAPI / frontend task cards
Reasonix plugin protocol
Reasonix shell/sandbox code copy
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
Kun identity or bridge aliases
Local Whisper bundled resources without import-boundary and package QA
default Go backend, Electron-to-Go routing, or renderer-visible Go routes
Rust/Tauri rewrite
live provider/cache superiority without credentials and cost reconciliation
live MCP/indexer parity without real environment QA
release readiness
```

Validation:

```text
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts builtin-tools.test.ts provider-cache-proof.test.ts mcp-tool-lifecycle-oracle.test.ts go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run typecheck
temporary official go1.26.4.darwin-arm64 from go.dev, SHA256 b62ad2b6d7d2464f12a5bcad7ff47f19d08325773b5efd21610e445a05a9bf53 verified
cd packages/runtime-go && go test ./...
```

Final command gates and forbidden-surface scans are recorded in the release
evidence file after the final rerun.

## 2026-06-21 - Route-Surface Boundary For Reasonix Orchestration

Source:

```text
Reasonix target/current refs remain the post-881 currentness set:
881b2f2f..9ada14176629b1d59d7ed78446951b2bb5954904.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix orchestration as a renderer top-level route | reject | Do not expose Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer as analytix top-level navigation. |
| Reasonix orchestration behind runtime contracts | contract-reimplement / defer by capability | Keep task, planner, research, MCP, and delegated execution work behind analytix-owned runtime/goal/tool contracts and existing UI surfaces. |
| Product-entry proof | document-only / contract-reimplement | Add renderer tests that enforce the boundary for `AppRoute`, app actions, Workbench stage, shell navigation, and sidebar active views. |

Evidence:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts
```

Boundary:

```text
No Reasonix SessionAPI, public task/planner protocol, public ask protocol,
new bridge/settings route, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry, default Go backend, or Rust/Tauri path is
introduced by this proof.
```

## 2026-06-21 - Structured User-Input Choice Validation

Source:

```text
Reasonix approval/user-input/control drift previously classified under the
post-881 control hardening stream.
No new Reasonix public protocol is imported.
```

Classification:

| Reasonix delta / idea | Classification | analytix decision |
| --- | --- | --- |
| malformed structured GUI input should not create pending ask/input state | contract-reimplement | Absorbed as analytix runtime tool validation for `request_user_input`. |
| invalid user-input gate behavior belongs in cross-runtime evidence | code-port-and-adapt | G4 oracle/schema/test and Go shadow output record invalid cases, stable error code, and `opensGateOnInvalid:false`. |
| Reasonix ask/session/control public protocol | reject | Do not expose Reasonix SessionAPI, ask protocol, controller route, or renderer route. |
| desktop visual/live user-input parity | defer | Requires packaged/live desktop QA; this batch is runtime/oracle proof only. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Runtime tool contract | `request_user_input` rejects more than three questions, one-option choice sets, more than three options, and duplicate labels before opening a gate. |
| Stable error surface | Invalid structured choices return `invalid_user_input_request` and do not call `awaitUserInput`. |
| Go G4 shadow | `go-g4-tools-approval-user-input-mcp-oracle.json` and `BuildG4ToolsConformanceOutput` record invalid cases and `opensGateOnInvalid:false`. |

Rejected / deferred:

```text
Reasonix public ask/session/control protocol, frontend protocol parity,
desktop live QA, renderer route, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry, default Go backend, Kun identity,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/builtin-tools.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-07-14 - Windows process-tree admission and handle-lifecycle adaptation

Source and provenance:

```text
Repository: /Users/sun/Projects/_upstreams/DeepSeek-Reasonix
Reviewed ref: origin/main-v2
Reviewed commit: 417335f027ac92e20f191999a093368d02698764
Source files: internal/proc/kill_windows.go, internal/proc/kill_windows_test.go,
              internal/proc/kill_other.go, internal/proc/kill_other_test.go
Relevant source history: 281888825b1b816f05e9a617f515f36d7a291de4,
                         daf60e8f6cee9ff698098f49f8a888a064a7309d,
                         3c42ea97163f7105a7540059590c807c8c049790
License: MIT; LICENSE blob bc45a281d8050c59c9b833ea2d0b1fb6e02602c0
```

Classification and decision:

| Mechanism | Decision | Analytix strengthening |
| --- | --- | --- |
| Start Windows child suspended and assign it to a kill-on-close Job Object before resume | `code-port-and-adapt` | `StartTracked` fails closed and kills/waits when Job creation, limit installation, assignment, or resume fails; it never returns a successful uncontained `job=0` fallback. |
| Terminate the Job to reap descendants | `code-port-and-adapt` | Repeated cancel calls keep the handle open; only the single `ReapTracked` owner closes it, preventing stale-handle reuse/ABA. |
| PATH-resolved `taskkill /T` fallback | `reject` | A production tracked start requires kernel Job authority. `KillTree` is direct-child cleanup only and cannot silently substitute a PATH executable for containment. |
| Unix process-group containment | `adapt` | Analytix starts the child in an independent session, retains negative-PID tree kill, and tests a live grandchild. This is shell containment only; it is not the stricter no-descendant data-helper admission boundary. |

Analytix implementation and evidence:

```text
packages/runtime-go/internal/proc/kill_windows.go
packages/runtime-go/internal/proc/kill_windows_test.go
packages/runtime-go/internal/proc/kill_other.go
packages/runtime-go/internal/proc/kill_other_test.go
packages/runtime-go/internal/adapters/outbound/mcp/stdio/process.go
packages/runtime-go/internal/adapters/outbound/mcp/stdio/process_test.go
packages/runtime-go/internal/server/product_regression_matrix_test.go

go test ./internal/proc ./internal/adapters/outbound/process -count=1
go test -race ./internal/proc ./internal/adapters/outbound/process -count=1
go test ./internal/adapters/outbound/mcp/stdio ./internal/proc -count=1
go test -race ./internal/adapters/outbound/mcp/stdio ./internal/proc -count=1
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/proc
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c ./internal/proc
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c
  ./internal/adapters/outbound/mcp/stdio
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c
  ./internal/adapters/outbound/mcp/stdio
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/proc
go test -tags analytix_prod ./internal/server -run
  '^TestProductRegressionMatrix(CoversRequiredProductSurfaces|ReferencesResolve|JSONSnapshot)$' -count=1
```

The current macOS run proves ordinary/race behavior and Windows/Linux build
compatibility only. It does not claim Windows execution or package parity; an
approved Windows host must still run the Job Object tests and packaged desktop
shutdown matrix. This slice also does not close data-helper executable/cwd
identity, macOS loaded-image verification, or the single Go data-execution
authority tasks.

## 2026-06-22 - D-0189 Runtime HTTP Route Sovereignty Shadow

Source:

```text
Reasonix frontend/runtime route review, Kun product-entry baseline, and
analytix `analytix serve` runtime HTTP/SSE source table.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Analytix HTTP route table, auth guard, and not-found behavior | `contract-reimplement` | Prove the active `buildRouter` route inventory, `/health` exception, `/v1/*` auth guard coverage, and structured not-found response from analytix-owned sources. |
| SSE thread events route | `contract-reimplement` | Keep event streaming on `/v1/threads/:id/events` and shared analytix endpoint templates. |
| Internal task-job control routes | `code-port-and-adapt` | Keep `/v1/runtime/task-jobs/{wait,output,kill}` as authenticated internal runtime endpoints, not product navigation. |
| Singular `/v1/user-input/:id` route | `document-only` | Record it as compatibility-only while the canonical shared template remains plural `/v1/user-inputs/{id}`. |
| Reasonix SessionAPI, workflow/create-loop/subagent/autoresearch/mcp-indexer public routes | `reject` | Do not expose upstream public protocol or hidden-capability routes. |
| Live Go HTTP server route walkthrough | `defer` | Keep live Go HTTP serving, packaged route walkthrough, and G6 backend selection for later gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives `runtimeHttpRouteSovereignty` from `routes/index.ts`, router first-match behavior, HTTP server not-found handling, endpoint templates, and HTTP server tests. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require exact 44-route inventory, auth counts, SSE route, task-job routes, compatibility route, and forbidden-route absence. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `routeCountExact`, `onlyHealthUnauthenticated`, `allRuntimeRoutesAnalytixOwned`, `sseRouteAnalytixThreadEvents`, `threadLifecycleRoutesPresent`, `approvalUserInputRoutesPresent`, and product-boundary booleans. |
| Scan guard | `scan:product-sovereignty` now checks D-0189 runtime HTTP route sovereignty proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
Reasonix/Kun/DeepSeek route or product identity
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0231 MCP Search Refresh Drift Evidence Closure

Source:

```text
Reasonix MCP/search currentness value is useful when catalog refresh detects
new tools without turning the MCP indexer into a public product route. Analytix
already had runtime/G5/Go evidence; this batch makes that evidence scan-visible
and stage-closure-visible.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP catalog refresh/currentness drift | `contract-reimplement` | Keep refresh drift as an internal analytix MCP search/tool invariant. |
| D-0231 scan/docs closure | `document-only` | No runtime code change; require existing proof tokens in `scan:product-sovereignty`. |
| Reasonix MCP public protocol / MCP-indexer route | `reject` | Do not expose upstream MCP routes, indexer product entry, or SessionAPI. |
| Credentialed MCP matrix / live Go MCP client | `defer` / `reject` | Go remains shadow-only; live MCP QA is a future gate. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `mcp-tool-lifecycle-oracle` records initial and expanded catalog tool names. |
| Runtime execution | `mcp-tool-lifecycle-oracle.test.ts` exercises `mcp_refresh_catalog` and validates `totalIndexed` / `catalogDrift`. |
| G5/Go shadow | `controlExecutableCases.mcpSearchRefreshDrift` and `BuildG5ControlExecutableOutput` replay the drift fields. |
| Scan guard | `scan:product-sovereignty` now requires `mcpSearchRefreshDrift` and refresh drift tokens. |

Rejected / deferred:

```text
Reasonix MCP public protocol, SessionAPI, public indexer route, or upstream MCP
control plane
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
credentialed MCP matrix or live MCP operations claim
Kun/deprecated GUI bridge aliases
renderer-visible Go route or default Go backend
live Go MCP client
packaged MCP walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0230 Plan/Auto-Route State Reset G5 Shadow

Source:

```text
Reasonix post-881 classifier/planner currentness value is useful when enabling
or disabling planner-like behavior cannot reuse stale controller/classifier
state. Analytix maps that value to turn-scoped Plan cancellation reset and
post-cancel auto-route currentness without importing Reasonix public
auto-plan/config/controller surfaces.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Classifier rebuild/currentness after planner state changes | `contract-reimplement` | Prove cancelled Plan state does not leak into later normal or auto-routed turns. |
| Post-cancel normal turn reset | `code-port-and-adapt` | Add runtime proof that fixed normal turns have no Plan mode instruction, no required `create_plan`, and no `create_plan` advertisement. |
| Post-cancel auto reroute | `code-port-and-adapt` | Add runtime/G5/Go proof that `model: "auto"` reruns `_auto_router` after cancellation with an isolated router request. |
| User/project auto-plan config | `document-only` / `defer` | Record as not absorbed into product settings or project overrides. |
| Reasonix SessionAPI/controller protocol | `reject` | Keep planner/currentness behind analytix runtime contracts. |
| Live Go loop/default backend | `defer` / `reject` | Go remains G5 shadow-only until later executable gates pass. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `loop.test.ts` covers Plan cancellation followed by normal and auto turns. |
| G5 conformance | `controlExecutableCases.planCancelStateReset` records reset fields, auto-router fields, stable-prefix cleanliness, and product-boundary booleans. |
| Go shadow | `BuildG5ControlExecutableOutput` emits reset/no-leak, normal/auto tool hiding, auto reroute, isolated router request, current recommendation, and boundary outputs. |
| Scan guard | `scan:product-sovereignty` requires D-0230 tokens while forbidden public auto-plan/controller, top-level route, bridge alias, and default Go backend scans remain active. |

Rejected / deferred:

```text
Reasonix SessionAPI, controller protocol, public auto-plan setting, project
override, or config root
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
Kun/deprecated GUI bridge aliases or legacy runtime-shaped settings fallback
dynamic classifier/planner/cancel state in stable prefix or tool schema
renderer-visible Go route or default Go backend
live Go loop, packaged Plan/auto-route QA, G6 readiness, or release readiness
Rust/Tauri migration
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0229 Plan Step/Cancel/Cache G5 Shadow

Source:

```text
Reasonix post-881 planner/cancel/cache value is useful when planner tool
gating, interruption, retry, and provider cache accounting stay coherent. This
batch promotes the existing analytix Plan-mode boundary guard into G5/Go shadow
without importing Reasonix controller protocol or public auto-plan settings.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Plan-mode first-step tool inventory | `contract-reimplement` | Keep `create_plan` + read-only tools available while `bash` remains excluded in explicit Plan mode. |
| Follow-up forced planner tool | `contract-reimplement` | Preserve the `requiredToolName === create_plan` follow-up and one-tool inventory. |
| Cancelled follow-up cache baseline | `code-port-and-adapt` | Reuse the original cache baseline after an aborted follow-up and keep `prefixChanged: false`. |
| DeepSeek cache telemetry preservation | `code-port-and-adapt` | Preserve two usage events and the `80/20` hit/miss fixture evidence. |
| Reasonix public controller/config route | `reject` | No SessionAPI, controller API, public auto-plan setting, or renderer-visible route is exposed. |
| Live Go loop/default backend | `defer` / `reject` | Go remains G5 shadow-only until later executable gates pass. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `loop.test.ts` keeps the D-0107 Plan-mode step/cancel/cache runtime behavior executable in TypeScript. |
| G5 conformance | `controlExecutableCases.planStepCancelCache` records mode, model, request count, tool sets, abort/retry statuses, usage count, cache hit/miss values, and boundary booleans. |
| Go shadow | `BuildG5ControlExecutableOutput` emits `planStepCancelCache` with step gating, follow-up-only-plan, cancelled-baseline, retry-baseline, cache telemetry, and product-boundary outputs. |
| Scan guard | `scan:product-sovereignty` requires D-0229 tokens while forbidden top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, bridge alias, and default Go backend scans remain active. |

Rejected / deferred:

```text
Reasonix SessionAPI, controller protocol, public auto-plan setting, or config root
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
Kun/deprecated GUI bridge aliases or legacy runtime-shaped settings fallback
dynamic planner/cache state in stable prefix or tool schema
renderer-visible Go route or default Go backend
live Go loop, packaged Plan QA, G6 readiness, or release readiness
Rust/Tauri migration
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/loop.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0228 MCP Call-Time Reconnect G5 Shadow

Source:

```text
Reasonix MCP/tool lifecycle value is useful when transient transport failures
recover without conflating deterministic protocol errors with stale sessions.
Analytix promotes existing `callMcpToolWithReconnect` classification into G5
shadow while keeping MCP behind analytix runtime/tool contracts.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Stale MCP connection retry | `code-port-and-adapt` | Accept transport-looking tool-call failures as one reconnect + retry in internal MCP provider behavior. |
| Protocol validation error no-retry | `contract-reimplement` | Preserve deterministic protocol errors as tool-result failures without tearing down healthy MCP sessions. |
| MCP call reconnect G5 replay | `code-port-and-adapt` | Add `mcpCallReconnect` to the MCP lifecycle oracle, G5 control case, and Go shadow output. |
| Reasonix MCP public protocol / MCP-indexer route | `reject` | Do not expose upstream MCP routes, indexer product entry, or SessionAPI. |
| Live Go MCP client/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `mcp-tool-provider.ts` keeps `looksLikeMcpTransportError` as the retry boundary; deterministic server-side failures do not reconnect. |
| Runtime execution | `mcp-tool-lifecycle-oracle.test.ts` runs real `buildMcpToolProviders` paths for stale connection retry and protocol error no-retry. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `mcpCallReconnect` and expected retry/no-retry booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes transport retry, protocol no-retry, attempt counts, close counts, success, and error-code outputs from the TS-owned fixture. |
| Scan guard | `scan:product-sovereignty` requires D-0228 MCP call-time reconnect tokens while forbidden MCP-indexer/top-level route scans remain active. |

Rejected / deferred:

```text
Reasonix MCP public protocol, SessionAPI, public indexer route, or upstream MCP control plane
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
credentialed MCP matrix or live MCP operations claim
Kun/deprecated GUI bridge aliases
renderer-visible Go route or default Go backend
live Go MCP client
packaged MCP walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0227 AutoResearch Direction Tracking G5 Shadow

Source:

```text
Reasonix long-task research value is useful only when attempted research
directions are durable, auditable, and isolated from public product protocols.
Analytix promotes existing `record_research_direction` behavior into Go G5
shadow while keeping it inside the analytix goal/runtime contract.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Direction tracking shadow replay | `code-port-and-adapt` | Promote `directions_tried.json` and `iteration_log.jsonl` writes into `controlExecutableCases.autoResearch`. |
| Active research-goal requirement | `contract-reimplement` | Keep `ThreadService.recordResearchDirection` guarded by an active research goal and prove it with `goal-tools.test.ts`. |
| `record_research_direction` tool contract | `code-port-and-adapt` | Accept the internal local tool name as analytix runtime behavior, not a public Reasonix protocol. |
| Reasonix project protocol/top-level AutoResearch route | `reject` | Do not expose Reasonix project state, SessionAPI, or a top-level AutoResearch entry. |
| Live Go research backend/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` reads `autoresearch-store.ts`, `goal-tools.ts`, `thread-service.ts`, and `goal-tools.test.ts` for the tool, file writes, and active-goal guard. |
| Runtime execution | The conformance test creates real `.analytix/autoresearch/<threadId>/` state, calls `recordDirection`, and reads `directions_tried.json` plus `iteration_log.jsonl`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require direction input, tool name, active-goal guard, and four expected direction-tracking booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes direction file, iteration-log, tool-name, and active-goal outputs from the TS-owned fixture. |
| Scan guard | `scan:product-sovereignty` requires D-0227 direction-tracking G5 tokens while forbidden top-level AutoResearch route scans remain active. |

Rejected / deferred:

```text
Reasonix project protocol, SessionAPI, public research route, or upstream state root
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
writing `REASONIX.md` or `AGENTS.md` for AutoResearch state
stable-prefix/tool-schema pollution with research state
Kun/deprecated GUI bridge aliases
renderer-visible Go route or default Go backend
live Go research bridge
packaged long-task walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- go-runtime-conformance
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0226 Renderer Settings Read Facade G5 Shadow

Source:

```text
Reasonix post-881 frontend/settings discipline is useful only when ordinary
renderer settings reads remain behind the analytix renderer runtime client.
Analytix promotes the D-0206 settings-read facade seal into Go G5 shadow
without exposing Reasonix config roots or a live Go settings bridge.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Settings-read facade shadow replay | `code-port-and-adapt` | Promote keyboard shortcut, speech-to-text, and initial usage model-label settings-read evidence into `desktopSovereignty.rendererSettingsReadFacadeMatrix`. |
| Settings-changed event sync proof | `contract-reimplement` | Preserve `analytix:settings-changed` event sync for hook consumers while initial usage model-label remains one-shot read evidence. |
| Direct renderer settings read bypass | `reject` | Keep `window.analytix.settings.getSettings` rejected in renderer production source. |
| Reasonix config/settings public protocol | `reject` | Do not expose upstream config roots, SessionAPI, settings protocol, or bridge aliases. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from keyboard shortcut settings, voice dictation settings, initial usage heatmap source, and `scan-product-sovereignty.cjs`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererSettingsReadFacadeMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes settings reader coverage, event-sync preservation, direct bridge rejection, and scan guard presence. |
| Scan guard | `scan:product-sovereignty` requires D-0226 matrix/output tokens while the renderer-wide direct settings read bypass scan remains active. |

Rejected / deferred:

```text
Reasonix SessionAPI, config root, settings public protocol, or upstream route names
direct renderer `window.analytix.settings.getSettings` bypass
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop/settings bridge
packaged settings walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0225 Renderer Usage Runtime Client Facade G5 Shadow

Source:

```text
Reasonix post-881 usage/debug observability is useful only when renderer
generic runtime HTTP requests remain sealed behind the analytix runtime client
facade. Analytix promotes the D-0205 usage/debug facade seal into Go G5 shadow
without exposing Reasonix usage/debug protocol or a live Go desktop bridge.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Usage facade shadow replay | `code-port-and-adapt` | Promote thread/day/model usage facade evidence into `desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` and Go G5 shadow output. |
| Settings diagnostics replay | `code-port-and-adapt` | Record token economy savings and LLM debug runtime requests as settings diagnostics that use the same renderer runtime client facade. |
| Direct renderer generic runtime bridge bypass | `reject` | Keep `window.analytix.runtime.runtimeRequest` rejected in renderer production source. |
| Reasonix usage/debug public protocol | `reject` | Do not expose upstream usage/debug/session protocol, route names, or bridge aliases. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from usage hooks, usage tests, settings diagnostics sources, and `scan-product-sovereignty.cjs`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererUsageRuntimeClientFacadeMatrix` plus seven expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes thread/day/model usage coverage, settings diagnostics coverage, direct bridge rejection, scan guard presence, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0225 matrix/output tokens while the renderer-wide direct runtime bridge bypass scan remains active. |

Rejected / deferred:

```text
Reasonix SessionAPI, usage/debug public protocol, or upstream route names
direct renderer `window.analytix.runtime.runtimeRequest` bypass
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged usage/dashboard walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0224 Side Conversation Relation G5 Shadow

Source:

```text
Reasonix post-881 frontend/session quality is useful only when side
conversation promotion remains an analytix provider operation. Analytix
promotes the D-0204 side relation contract into Go G5 shadow without exposing
Reasonix side/session protocol or direct renderer bridge calls.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Side conversation relation shadow replay | `code-port-and-adapt` | Promote side relation provider/store evidence into `desktopSovereignty.sideConversationRelationContractMatrix` and Go G5 shadow output. |
| Optional provider relation contract | `contract-reimplement` | Keep `AgentProvider.updateThreadRelation?` as an analytix-owned provider contract. |
| Side promotion through provider | `contract-reimplement` | Keep `promoteSideConversation` calling `provider.updateThreadRelation(sideId, 'primary')`, then refreshing threads and closing side state. |
| Direct side-store runtime bridge bypass | `reject` | Product-sovereignty scan continues to reject direct runtime bridge calls in side-store source. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `types.ts`, `analytix-runtime.ts`, `chat-store-side-actions.ts`, `chat-store-side-actions.test.ts`, and `scan-product-sovereignty.cjs`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `sideConversationRelationContractMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes optional provider contract proof, provider promotion, refresh/close behavior, direct bridge rejection, scan guard presence, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0224 matrix/output tokens plus final gate evidence. |

Rejected / deferred:

```text
Reasonix side/session public protocol
direct renderer `window.analytix.runtime.runtimeRequest` bypass
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged side-conversation walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0223 Renderer Provider Facade Seal G5 Shadow

Source:

```text
Reasonix post-881 frontend/session quality is useful only when renderer
provider HTTP calls are sealed behind the analytix runtime client facade.
Analytix promotes the D-0203 provider facade seal into Go G5 shadow without
reintroducing direct renderer bridge calls or Reasonix public protocol.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer provider facade seal shadow replay | `code-port-and-adapt` | Promote provider facade seal evidence into `desktopSovereignty.rendererProviderFacadeSealMatrix` and Go G5 shadow output. |
| Archive/restore through runtime client | `contract-reimplement` | Keep `archiveThread` on `rendererRuntimeClient.runtimeRequest` and the analytix thread PATCH route/body. |
| Relation PATCH through runtime client | `contract-reimplement` | Keep `updateThreadRelation` on the same provider/runtime-client contract. |
| Direct renderer bridge bypass | `reject` | Product-sovereignty scan continues to reject `window.analytix.runtime.runtimeRequest` in renderer production source. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `src/renderer/src/agent/analytix-runtime.ts`, `analytix-runtime.test.ts`, and `scan-product-sovereignty.cjs`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererProviderFacadeSealMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes runtime client use, direct bridge rejection, archive/restore coverage, relation PATCH coverage, scan guard presence, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0223 matrix/output tokens plus final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public frontend bridge protocol
direct renderer `window.analytix.runtime.runtimeRequest` bypass
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged provider walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0222 Renderer Provider Alias Guard G5 Shadow

Source:

```text
Reasonix post-881 frontend/session bridge value is useful only when the
renderer provider stays behind analytix-owned routes and never reads legacy
bridge aliases. Analytix promotes the D-0202 provider alias guard into Go G5
shadow without exposing Reasonix SessionAPI, Kun aliases, or a Go desktop
bridge.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer provider alias guard shadow replay | `code-port-and-adapt` | Promote provider alias guard evidence into `desktopSovereignty.rendererProviderAliasGuardMatrix` and Go G5 shadow output. |
| Provider route ownership under throwing aliases | `contract-reimplement` | Keep `AnalytixRuntimeProvider` tests running with throwing `window.kun` / `window.reasonix` getters unread. |
| Lifecycle, approval/user-input, fork/resume, dynamic encoding coverage | `contract-reimplement` | Keep provider route, lifecycle, gate, fork/resume, and encoded route-id proofs tied to analytix HTTP paths. |
| Reasonix/Kun bridge aliases and public route/session protocol | `reject` | No Reasonix SessionAPI/public bridge protocol, no `window.kun`, and no `window.reasonix` surface is accepted. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `src/renderer/src/agent/analytix-runtime.ts` and `analytix-runtime.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererProviderAliasGuardMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes throwing alias installation, runtime route coverage, lifecycle/gate coverage, fork/resume/dynamic encoding coverage, forbidden-route rejection, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0222 matrix/output tokens plus final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public frontend bridge protocol
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged provider walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0221 Renderer Settings Bridge G5 Shadow

Source:

```text
Reasonix post-881 settings/currentness value is useful only when renderer
settings reads and writes stay behind analytix top-level settings contracts.
Analytix promotes the D-0201 renderer settings bridge proof into Go G5 shadow
without exposing Reasonix settings roots or legacy agent envelopes.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer settings bridge shadow replay | `code-port-and-adapt` | Promote settings cache/write/top-level runtime patch evidence into `desktopSovereignty.rendererSettingsBridgeMatrix` and Go G5 shadow output. |
| Settings read cache and write refresh | `contract-reimplement` | Keep renderer settings reads cached in `rendererRuntimeClient` and refreshed after `setSettings`. |
| Top-level runtime patch preservation | `contract-reimplement` | Keep `runtime.model` and `runtime.approvalPolicy` patches flowing through `window.analytix.settings.setSettings`. |
| Reasonix/Kun settings/bridge aliases | `reject` | Unit proof keeps throwing `window.kun` / `window.reasonix` getters unread and no Reasonix settings root is written. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `src/renderer/src/agent/runtime-client.ts` and `runtime-client.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererSettingsBridgeMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes analytix settings API use, settings read cache, write refresh, top-level runtime patch preservation, legacy alias unread proof, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0221 matrix/output tokens plus final gate evidence. |

Rejected / deferred:

```text
Reasonix settings protocol, config root, or legacy agent settings envelope
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged settings walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/runtime-client.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0220 Renderer Runtime Client Bridge G5 Shadow

Source:

```text
Reasonix post-881 frontend/session bridge value is useful only when renderer
code stays behind the analytix-owned runtime client facade. Analytix promotes
the D-0200 renderer client proof into Go G5 shadow without exposing Reasonix
bridge aliases, SessionAPI, or a Go desktop bridge.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer runtime client bridge shadow replay | `code-port-and-adapt` | Promote request/restart/SSE passthrough evidence into `desktopSovereignty.rendererRuntimeClientBridgeMatrix` and Go G5 shadow output. |
| Runtime request argument preservation | `contract-reimplement` | Keep `rendererRuntimeClient.runtimeRequest(path, method?, body?)` as an analytix facade that forwards encoded shared endpoint paths unchanged. |
| Runtime restart passthrough | `contract-reimplement` | Keep explicit runtime restarts on `window.analytix.runtime.restartRuntime`, not an upstream controller protocol. |
| SSE control and listener passthrough | `contract-reimplement` | Keep renderer `startSse`/`stopSse` and event/end/error listeners behind `window.analytix.runtime`. |
| Reasonix/Kun bridge aliases | `reject` | Unit proof keeps throwing `window.kun` / `window.reasonix` getters unread. |
| Live Go desktop bridge/default backend | `defer` / `reject` | Go remains G5 shadow-only and cannot become a renderer-visible route or default backend from this evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `src/renderer/src/agent/runtime-client.ts` and `runtime-client.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `rendererRuntimeClientBridgeMatrix` plus six expected booleans. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes request argument preservation, restart passthrough, SSE control/listener passthrough, legacy alias unread proof, and unit-proof presence. |
| Scan guard | `scan:product-sovereignty` requires D-0220 matrix/output tokens plus final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public frontend bridge protocol
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go desktop bridge
packaged renderer walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0219 Main SSE Host URL Encoding G5 Shadow

Source:

```text
D-0199 main SSE host URL encoding proof and Reasonix post-881 route/session
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main SSE host encoding shadow replay | `code-port-and-adapt` | Promote main SSE host URL/header/error evidence into `desktopMainIpcBoundary.mainSseHostEncodingMatrix` and Go G5 shadow output. |
| Encoded thread events path | `contract-reimplement` | Keep dangerous renderer thread ids encoded into `/v1/threads/{id}/events` before fetching managed `analytix serve`. |
| Cursor/header preservation | `contract-reimplement` | Preserve `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, bearer auth, and stream id across the main SSE host fetch. |
| Forbidden SSE route guard | `contract-reimplement` | Keep Reasonix/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer SSE route tokens rejected by path construction evidence. |
| Reasonix public SSE/session protocol | `reject` | Do not expose SessionAPI, upstream SSE route names, or controller streams through main IPC. |
| Live Go SSE/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require main SSE host encoding matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives encoded path, cursor/header, error delivery, and forbidden-route booleans from main SSE sources and tests. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes thread/cursor encoding, header/stream-id preservation, forbidden-route rejection, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` now requires D-0219 main SSE host encoding G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public SSE/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go SSE server, Go main bridge, or Electron Go bridge
packaged SSE walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0218 Preload SSE Bridge G5 Shadow

Source:

```text
D-0198 preload SSE bridge proof and Reasonix post-881 route/session boundary
review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload SSE bridge shadow replay | `code-port-and-adapt` | Promote SSE start/stop and listener evidence into `desktopSovereignty.preloadSseBridgeMatrix` and Go G5 shadow output. |
| Start/stop argument preservation | `contract-reimplement` | Keep `window.analytix.runtime.startSse/stopSse` passing thread id, cursor, and stream id unchanged to analytix IPC. |
| Payload-only listener delivery | `contract-reimplement` | Keep event/end/error wrappers forwarding payloads only and omitting Electron event objects. |
| Listener cleanup symmetry | `contract-reimplement` | Keep unsubscribe cleanup removing the exact wrapped listener on each SSE channel. |
| Reasonix public SSE/session protocol | `reject` | Do not expose SessionAPI, upstream SSE route names, or controller streams through preload. |
| Live Go SSE/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require preload SSE bridge matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives start/stop, payload-only wrapper, cleanup, and analytix-only exposure booleans from preload sources and tests. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes start/stop preservation, payload-only listener delivery, cleanup proof, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` now requires D-0218 preload SSE bridge G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public SSE/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go SSE server, Go preload bridge, or Electron Go bridge
packaged SSE walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0217 Runtime Host URL Handoff G5 Shadow

Source:

```text
D-0197 runtime host URL handoff proof and Reasonix post-881 route/session
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime host handoff shadow replay | `code-port-and-adapt` | Promote `runtimeRequestViaHost` evidence into `desktopMainIpcBoundary.runtimeHostHandoffMatrix` and Go G5 shadow output. |
| Encoded path/query preservation | `contract-reimplement` | Keep encoded shared endpoint path segments and encoded query values unchanged when building the managed runtime host URL. |
| Method/header/body preservation | `contract-reimplement` | Preserve POST method, JSON body, bearer auth, custom header, and content type through the host fetch. |
| ensureRuntime host selection | `contract-reimplement` | Use settings returned by `ensureRuntime` before deriving the managed analytix runtime base URL. |
| Reasonix SessionAPI / public host route protocol | `reject` | Do not expose SessionAPI, route names, or upstream control-plane URLs through the host handoff. |
| Live Go HTTP server/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require runtime host handoff matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives base URL, normalized path join, auth/header/body forwarding, and ensureRuntime booleans from analytix adapter sources and tests. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes encoded path/query preservation, method/header/body preservation, ensured settings use, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` now requires D-0217 runtime host handoff G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go HTTP server, Go main/preload bridge, or Electron Go bridge
packaged route walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0216 Preload Runtime Request Bridge G5 Shadow

Source:

```text
D-0196 preload runtime request bridge proof and Reasonix post-881 route /
session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload runtime request bridge shadow replay | `code-port-and-adapt` | Promote preload runtime/diagnostics facade evidence into `desktopSovereignty.preloadRuntimeRequestBridgeMatrix` and Go G5 shadow output. |
| Path/method/body preservation | `contract-reimplement` | Keep `window.analytix.runtime.runtimeRequest(path, method, body)` passing arguments unchanged to main IPC. |
| Diagnostics same-channel behavior | `contract-reimplement` | Keep diagnostics runtime requests on the same analytix `runtime:request` IPC channel. |
| Analytix-only bridge exposure | `contract-reimplement` | Keep preload exposing only `analytix`, not Reasonix/Kun/deprecated aliases. |
| Reasonix public bridge/session protocol | `reject` | Do not expose SessionAPI, controller route names, or upstream protocol through renderer/preload/main/runtime. |
| Live Go preload/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require preload runtime request bridge matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives path/method/body, diagnostics same-channel, analytix-only exposure, and unit-test booleans from analytix preload sources. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes runtime facade preservation, diagnostics same-channel behavior, analytix-only exposure, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` now requires D-0216 preload runtime request bridge G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go HTTP server, Go preload bridge, or Electron Go bridge
packaged preload/route walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0215 Main IPC Runtime Adapter Handoff G5 Shadow

Source:

```text
D-0195 main IPC runtime adapter handoff proof and Reasonix post-881 route /
session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main IPC runtime adapter handoff shadow replay | `code-port-and-adapt` | Promote registered handler handoff evidence into `desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` and Go G5 shadow output. |
| Encoded path preservation | `contract-reimplement` | Keep encoded shared endpoint paths unchanged from main IPC handler to runtime adapter. |
| Method/body preservation | `contract-reimplement` | Preserve method and body arguments through `runtimeRequest(request.path, request.method, request.body)`. |
| Reject-before-adapter ordering | `contract-reimplement` | Keep raw dynamic and singular compatibility paths rejected before adapter invocation. |
| Reasonix public bridge/session protocol | `reject` | Do not expose SessionAPI, controller route names, or upstream protocol through renderer/main/runtime. |
| Live Go main IPC/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require main IPC runtime-adapter handoff matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives encoded path, method/body, reject-before-call, and unit-test booleans from analytix main IPC handler sources. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes encoded-path preservation, method/body preservation, raw dynamic rejection, reject-before-call, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` now requires D-0215 main IPC runtime-adapter handoff G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go HTTP server or Electron Go bridge
packaged route/SSE/IPC walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0214 Main IPC Endpoint Builder G5 Shadow

Source:

```text
D-0194 main IPC endpoint builder allow-list proof and Reasonix post-881 route /
session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main IPC endpoint builder shadow replay | `code-port-and-adapt` | Promote runtime request schema allow-list evidence into `desktopMainIpcBoundary.endpointBuilderAllowListMatrix` and Go G5 shadow output. |
| Shared endpoint template compilation | `contract-reimplement` | Keep main IPC schema compiled from analytix shared templates, not Reasonix route names. |
| Encoded dynamic route acceptance | `contract-reimplement` | Require encoded thread, turn, checkpoint, approval, user-input, session, attachment, and memory ids. |
| Raw dynamic route and singular user-input rejection | `contract-reimplement` | Keep raw extra route segments and `/v1/user-input/:id` compatibility out of main IPC accepted paths. |
| Reasonix public bridge/session protocol | `reject` | Do not expose SessionAPI, controller route names, or upstream protocol through renderer/main/runtime. |
| Live Go main IPC/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require main IPC endpoint-builder matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives shared-template, accepted encoded request, raw-rejection, singular-user-input, and unit-test booleans from analytix main IPC sources. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes accepted shared paths, raw dynamic rejection, shared-template use, unit-proof presence, and singular user-input rejection. |
| Scan freshness | `scan:product-sovereignty` now requires D-0214 main IPC endpoint-builder G5 proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go HTTP server or Electron Go bridge
packaged route/SSE/IPC walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0190 Runtime HTTP Auth Matrix Proof

Source:

```text
Reasonix route/control review, D-0189 runtime route sovereignty oracle, and
the active analytix TypeScript HTTP router.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Auth-before-handler behavior for the active route table | `contract-reimplement` | Dispatch every D-0189 registered route without auth through the real TypeScript router and require `/health` 200 plus 43 structured 401 responses. |
| SSE/task-job/approval/user-input/resume auth gates | `contract-reimplement` | Prove these sensitive routes reject before route-specific body parsing or side effects when auth is missing. |
| Reasonix route/control protocol | `reject` | Do not import SessionAPI or Reasonix public route names. |
| Live Go HTTP server matrix | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Actual HTTP dispatch | `go-runtime-conformance.test.ts` now runs `requires auth on every registered /v1 runtime route from the G5 route-sovereignty fixture`. |
| Fixture tie | The matrix is driven by `controlExecutableCases.runtimeHttpRouteSovereignty.routes` and `authenticatedRouteCount`, so route additions must update both the route oracle and auth proof. |
| Scan guard | `scan:product-sovereignty` now checks D-0190 runtime HTTP auth matrix proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0192 Shared Endpoint Builder Sovereignty Proof

Source:

```text
Reasonix route/control review, analytix shared endpoint contract, and renderer
/ main runtime path builders.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Shared runtime endpoint templates | `contract-reimplement` | Keep endpoint templates centralized in `src/shared/analytix-endpoints.ts` and analytix-owned. |
| Route-id encoding safety | `contract-reimplement` | Prove thread, turn, checkpoint, approval, user-input, session, attachment, and memory ids are URL-encoded by builders. |
| Singular user-input compatibility route | `document-only` | Keep singular `/v1/user-input/:id` as server compatibility only; shared template remains plural `/v1/user-inputs/{id}`. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Keep forbidden upstream and hidden-capability tokens out of exported shared endpoint strings. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Shared endpoint test | `src/shared/analytix-endpoints.test.ts` proves URL encoding, canonical plural user-input template, and forbidden token absence from shared endpoint exports. |
| Scan guard | `scan:product-sovereignty` now checks D-0192 shared endpoint builder sovereignty proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm run test -- src/shared/analytix-endpoints.test.ts
```

## 2026-06-22 - D-0193 Renderer Runtime Endpoint Builder Proof

Source:

```text
Reasonix route/control review, D-0192 shared endpoint builders, and the
renderer `AnalytixRuntimeProvider`.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer runtime root paths | `contract-reimplement` | Use shared `ANALYTIX_HEALTH_PATH` and `ANALYTIX_THREADS_PATH` constants for connect/list/create-thread root calls. |
| Renderer dynamic route-id encoding | `contract-reimplement` | Prove thread, turn, approval, user-input, and session ids are encoded before bridge runtime requests. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Renderer provider continues to call only analytix-owned `/health` and `/v1/*` paths. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer provider code | `AnalytixRuntimeProvider` now uses shared constants for health/thread root paths and existing shared builders for dynamic routes. |
| Renderer provider test | `analytix-runtime.test.ts` proves dangerous ids containing slash/query/fragment text are encoded before `runtimeRequest`. |
| Scan guard | `scan:product-sovereignty` now checks D-0193 renderer runtime endpoint builder proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

## 2026-06-22 - D-0194 Main IPC Endpoint Builder Allow-list Proof

Source:

```text
Reasonix route/control review, D-0192 shared endpoint builders, D-0193 renderer
provider paths, and the analytix main IPC runtime request schema.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main IPC acceptance of encoded builder paths | `contract-reimplement` | Prove the `runtimeRequest` schema accepts shared builder output for encoded thread, turn, checkpoint, approval, user-input, session, attachment, and memory ids. |
| Raw dynamic route segment injection | `contract-reimplement` | Reject unencoded dynamic ids that create extra route segments before they reach the runtime adapter. |
| Singular user-input compatibility path | `document-only` / `reject` | Keep singular `/v1/user-input/:id` as server compatibility only; main IPC shared allow-list stays plural `/v1/user-inputs/{id}`. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream route identity through the renderer bridge or main IPC allow-list. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Main IPC schema test | `app-ipc-schemas.test.ts` now proves encoded output from shared builders is accepted by `runtimeRequestPayloadSchema`. |
| Compatibility boundary | The same test rejects raw extra-segment paths and the singular user-input compatibility route at the main IPC boundary. |
| Scan guard | `scan:product-sovereignty` checks D-0194 main IPC endpoint builder allow-list proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
singular user-input path as a public shared/IPC contract
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/main/ipc/app-ipc-schemas.test.ts
```

## 2026-06-22 - D-0195 Main IPC Runtime Adapter Handoff Proof

Source:

```text
D-0194 main IPC schema proof, analytix `registerAppIpcHandlers`, and the
Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Encoded path handoff to runtime adapter | `contract-reimplement` | Prove `runtime:request` handler forwards encoded shared builder paths to `runtimeRequest` unchanged. |
| Invalid path no-forward behavior | `contract-reimplement` | Prove raw extra-segment and singular compatibility paths fail validation before adapter invocation. |
| Bridge/adapter ownership | `contract-reimplement` | Keep the public bridge as `window.analytix.runtime.runtimeRequest`; the adapter receives analytix-owned paths only. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream route identity through IPC handler registration. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Handler pass-through | `register-app-ipc-handlers.test.ts` proves encoded builder output is passed to the runtime adapter unchanged with method/body preserved. |
| Handler rejection | The same test proves raw extra-segment and singular user-input compatibility paths reject before `runtimeRequest` is called. |
| Scan guard | `scan:product-sovereignty` checks D-0195 main IPC runtime adapter handoff proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
singular user-input path as a public shared/IPC contract
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/main/ipc/register-app-ipc-handlers.test.ts
```

## 2026-06-22 - D-0196 Preload Runtime Request Bridge Proof

Source:

```text
D-0193 renderer provider paths, D-0194/D-0195 main IPC proofs, preload
`window.analytix` facade, and Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload runtime request argument handoff | `contract-reimplement` | Prove `api.runtime.runtimeRequest` invokes `runtime:request` with encoded path, method, and body unchanged. |
| Diagnostics runtime request compatibility | `contract-reimplement` | Keep diagnostics runtime requests on the same analytix IPC channel and payload shape. |
| Analytix bridge ownership | `contract-reimplement` | The exposed bridge remains `window.analytix`; no alias or Reasonix public protocol is added. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream route identity through preload. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Preload bridge test | `preload-runtime-request.test.ts` mocks Electron and calls the exposed `analytix` API to prove encoded shared paths reach `ipcRenderer.invoke('runtime:request', { path, method, body })`. |
| Diagnostics bridge test | The same test proves diagnostics fallback uses the same analytix IPC channel. |
| Scan guard | `scan:product-sovereignty` checks D-0196 preload runtime request bridge proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge aliases
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/preload/preload-runtime-request.test.ts
```

## 2026-06-22 - D-0197 Runtime Host URL Handoff Proof

Source:

```text
D-0193 through D-0196 runtime request bridge proof chain, `runtimeRequestViaHost`,
and Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime host URL path preservation | `contract-reimplement` | Prove encoded shared endpoint paths remain encoded when `runtimeRequestViaHost` sends the HTTP request to `analytix serve`. |
| Query/body/header preservation | `contract-reimplement` | Preserve query strings, method, auth, custom headers, content type, and body through the runtime host request. |
| Analytix runtime boundary | `contract-reimplement` | The target remains the managed analytix runtime base URL from top-level `runtime` settings. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream route identity through runtime host URL construction. |
| Live Go HTTP server route walkthrough | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime adapter test | `analytix-adapter.test.ts` uses a real local HTTP server to prove encoded shared paths and query strings arrive unchanged at the runtime host. |
| Request shape | The same test proves POST method, bearer auth, custom header, JSON content type, and body preservation. |
| Scan guard | `scan:product-sovereignty` checks D-0197 runtime host URL handoff proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge aliases
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/main/runtime/analytix-adapter.test.ts
```

## 2026-06-22 - D-0198 Preload SSE Bridge Proof

Source:

```text
Runtime request bridge proof chain D-0193 through D-0197, preload
`window.analytix` SSE facade, and Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload SSE start/stop handoff | `contract-reimplement` | Prove `api.runtime.startSse` and `stopSse` invoke analytix-owned `runtime:sse:*` IPC channels with arguments unchanged. |
| SSE event wrapper payload preservation | `contract-reimplement` | Prove event/end/error wrappers forward payloads without leaking Electron event objects to renderer handlers. |
| Analytix bridge ownership | `contract-reimplement` | The exposed bridge remains `window.analytix`; no alias or Reasonix public protocol is added. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream SSE route identity through preload. |
| Live Go SSE route walkthrough | `defer` | Keep Go serving and packaged SSE walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Preload SSE bridge test | `preload-sse-bridge.test.ts` mocks Electron, loads the exposed `analytix` API, and proves `startSse` / `stopSse` argument preservation. |
| SSE listener proof | The same test triggers `runtime:sse-event`, `runtime:sse-end`, and `runtime:sse-error` wrappers and proves payload-only delivery plus unsubscribe cleanup. |
| Scan guard | `scan:product-sovereignty` checks D-0198 preload SSE bridge proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public SSE protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge aliases
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go SSE server, packaged SSE walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/preload/preload-sse-bridge.test.ts
```

## 2026-06-22 - D-0199 Main SSE Host URL Encoding Proof

Source:

```text
D-0198 preload SSE bridge proof, main `registerRuntimeSseIpc`,
`analytixThreadEventsPath`, and Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main SSE thread-id URL encoding | `contract-reimplement` | Prove dangerous thread ids are encoded before fetching `/v1/threads/{id}/events` from the managed runtime host. |
| SSE cursor/header preservation | `contract-reimplement` | Preserve `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, and bearer auth. |
| Analytix route ownership | `contract-reimplement` | SSE host URL remains analytix-owned `/v1/threads/{id}/events`, not upstream public protocol. |
| Reasonix SessionAPI / hidden SSE protocol | `reject` | Do not expose upstream SSE route identity through main IPC. |
| Live Go SSE route walkthrough | `defer` | Keep Go serving and packaged SSE walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Main SSE IPC test | `runtime-sse-ipc.test.ts` proves slash/query/fragment thread ids become encoded runtime host event paths. |
| Cursor/header proof | The same test proves `since_seq`, `Last-Event-ID`, `Accept`, and auth header preservation. |
| Scan guard | `scan:product-sovereignty` checks D-0199 main SSE host URL encoding proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public SSE protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge aliases
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go SSE server, packaged SSE walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/main/runtime-sse-ipc.test.ts
```

## 2026-06-22 - D-0200 Renderer Runtime Client Bridge Proof

Source:

```text
D-0196 through D-0199 bridge/SSE proof chain, renderer `runtime-client`, and
Reasonix route/control boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer runtime request facade | `contract-reimplement` | Prove `rendererRuntimeClient.runtimeRequest` forwards encoded paths/method/body through `window.analytix.runtime` unchanged. |
| Renderer SSE facade | `contract-reimplement` | Prove `startSse`, `stopSse`, and SSE listener registration forward arguments/handlers unchanged. |
| Legacy bridge alias guard | `contract-reimplement` | Test legacy alias getters that throw if `window.kun` or `window.reasonix` is read. |
| Reasonix SessionAPI / hidden route protocol | `reject` | Do not expose upstream bridge or route identity through renderer runtime client. |
| Packaged renderer route walkthrough | `defer` | Keep packaged desktop walkthrough for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer client test | `runtime-client.test.ts` proves runtime request and SSE facade argument preservation through `window.analytix`. |
| Alias guard | The same test installs throwing `window.kun` / `window.reasonix` getters and confirms they are not read. |
| Scan guard | `scan:product-sovereignty` checks D-0200 renderer runtime client bridge proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public bridge protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge aliases
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged runtime/SSE walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

## 2026-06-22 - D-0201 Renderer Settings Bridge Proof

Source:

```text
D-0200 renderer client bridge proof, active renderer `runtime-client`, top-level
runtime settings sovereignty evidence, and Reasonix settings/config boundary
review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer settings facade | `contract-reimplement` | Prove `rendererRuntimeClient.setSettings` forwards patches through `window.analytix.settings` only. |
| Top-level runtime settings patch | `contract-reimplement` | Preserve `runtime.model` and `runtime.approvalPolicy` under analytix-owned top-level `runtime` settings. |
| Legacy bridge alias guard | `contract-reimplement` | Keep throwing `window.kun` / `window.reasonix` getters unread during settings writes. |
| Reasonix config root or public settings protocol | `reject` | Do not expose Reasonix config names, SessionAPI, or public settings schema. |
| Packaged settings walkthrough | `defer` | Keep packaged settings UI QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer client test | `runtime-client.test.ts` proves a top-level `runtime` patch reaches `window.analytix.settings.setSettings` unchanged and refreshes the renderer cache from the returned settings. |
| Alias guard | The same test installs throwing `window.kun` / `window.reasonix` getters and confirms they are not read. |
| Scan guard | `scan:product-sovereignty` checks D-0201 renderer settings bridge proof tokens and requires final release evidence for this gate. |

Rejected / deferred:

```text
Reasonix config root or public settings protocol
old runtime-shaped settings fallback
deprecated bridge aliases
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged settings walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

## 2026-06-22 - D-0202 Renderer Runtime Provider Alias Guard

Source:

```text
Renderer `AnalytixRuntimeProvider` route-surface tests, D-0193 endpoint builder
proof, D-0200 renderer client bridge proof, and Reasonix frontend/session
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer provider legacy alias guard | `contract-reimplement` | Install throwing `window.kun` / `window.reasonix` getters in the shared provider test bridge helper. |
| Thread lifecycle, approval/user-input, fork/resume provider paths | `contract-reimplement` | Reuse existing provider tests to prove these calls stay on analytix-owned `/v1/*` routes while aliases remain unread. |
| Dynamic route id encoding | `contract-reimplement` | Keep the existing URL-encoding provider proof under the stronger alias guard. |
| Reasonix SessionAPI or public bridge protocol | `reject` | Do not expose upstream protocol or renderer-visible route names. |
| Packaged renderer walkthrough | `defer` | Keep packaged desktop QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer provider test | `analytix-runtime.test.ts` now installs throwing legacy alias getters for all `installDsGui` provider tests. |
| Route coverage | Existing tests cover analytix-owned HTTP routes, thread lifecycle archive/search, approval/user-input compatibility endpoints, fork/resume, and dynamic id encoding under the alias guard. |
| Scan guard | `scan:product-sovereignty` checks D-0202 renderer runtime provider alias guard proof tokens and requires final release evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public bridge protocol
deprecated bridge aliases
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged renderer walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
```

## 2026-06-22 - D-0203 Renderer Provider Runtime Client Facade Seal

Source:

```text
D-0202 renderer provider alias guard, active `AnalytixRuntimeProvider`
archive-thread implementation, and Reasonix frontend/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Archive-thread provider request facade | `code-port-and-adapt` | Route archive/restore through `rendererRuntimeClient.runtimeRequest` instead of direct `window.analytix.runtime.runtimeRequest`. |
| Provider direct bridge bypass scan | `contract-reimplement` | Add a product-sovereignty scan that forbids direct runtime request bridge calls inside `analytix-runtime.ts`. |
| Thread lifecycle route ownership | `contract-reimplement` | Keep archive/restore on `analytixThreadPath(threadId)` and existing lifecycle tests. |
| Reasonix SessionAPI or public bridge protocol | `reject` | Do not expose upstream protocol or renderer-visible route names. |
| Packaged renderer walkthrough | `defer` | Keep packaged desktop QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer provider source | `archiveThread` now uses `rendererRuntimeClient.runtimeRequest`, matching the rest of the provider runtime request surface. |
| Scan guard | `scan:product-sovereignty` requires archive-thread facade proof tokens and forbids `window.analytix.runtime.runtimeRequest` in `analytix-runtime.ts`. |
| Focused validation | `analytix-runtime.test.ts` keeps thread lifecycle coverage passing under the D-0202 legacy alias guard. |

Rejected / deferred:

```text
Reasonix SessionAPI or public bridge protocol
deprecated bridge aliases or direct provider bridge bypass
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged renderer walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
```

## 2026-06-22 - D-0204 Side Conversation Relation Provider Contract

Source:

```text
D-0203 renderer provider facade seal, side conversation promotion source,
`AnalytixRuntimeProvider` thread lifecycle surface, and Reasonix
frontend/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Thread relation provider contract | `contract-reimplement` | Add optional `AgentProvider.updateThreadRelation` and implement it in `AnalytixRuntimeProvider` through `rendererRuntimeClient.runtimeRequest`. |
| Side conversation promotion route | `code-port-and-adapt` | Route `promoteSideConversation` through the provider contract instead of direct `window.analytix.runtime.runtimeRequest`. |
| Direct bridge bypass scan | `contract-reimplement` | Extend the product-sovereignty scan to forbid direct runtime request bridge calls in `chat-store-side-actions.ts`. |
| Reasonix side/session protocol | `reject` | Do not expose upstream SessionAPI, side-conversation protocol, or renderer route names. |
| Packaged side-conversation walkthrough | `defer` | Keep packaged desktop QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Provider contract | `AgentProvider` now has optional `updateThreadRelation`, and `AnalytixRuntimeProvider` sends `{ relation }` via analytix-owned thread PATCH. |
| Store route | `promoteSideConversation` calls `provider.updateThreadRelation(sideId, 'primary')` and then refreshes threads/tears down the side panel. |
| Tests | `analytix-runtime.test.ts` proves the provider PATCH body; `chat-store-side-actions.test.ts` proves side promotion uses the provider and refreshes the thread list. |
| Scan guard | `scan:product-sovereignty` requires D-0204 proof tokens and forbids direct runtime request bridge calls in provider and side-store sources. |

Rejected / deferred:

```text
Reasonix SessionAPI or side-conversation public protocol
deprecated bridge aliases or direct store/provider bridge bypass
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged side-conversation walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts
```

## 2026-06-22 - D-0205 Renderer Usage Runtime Client Facade Seal

Source:

```text
D-0204 direct bridge bypass scan, renderer usage hooks/settings diagnostics
surfaces, provider/cache accounting evidence, and Reasonix frontend/session
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Thread/day/model usage transport | `code-port-and-adapt` | Route usage hooks through `rendererRuntimeClient.runtimeRequest` while preserving `/v1/usage` paths and parsing. |
| Token economy savings and LLM debug transport | `code-port-and-adapt` | Route settings diagnostics usage/debug requests through the renderer runtime client facade. |
| Renderer-wide direct runtime request scan | `contract-reimplement` | Extend product-sovereignty scan to forbid `window.analytix.runtime.runtimeRequest` in renderer production source. |
| Reasonix usage/debug protocol | `reject` | Do not expose upstream usage/session protocol or public route names. |
| Packaged usage/dashboard walkthrough | `defer` | Keep packaged desktop QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Usage hooks | `loadThreadUsage`, `loadDailyUsage`, and `loadModelUsage` use `rendererRuntimeClient.runtimeRequest` with unchanged analytix `/v1/usage` paths. |
| Settings diagnostics | token economy savings and LLM round debug requests use the renderer runtime client facade. |
| Tests | Existing usage/settings tests keep request paths and parsing stable after the transport seal. |
| Scan guard | `scan:product-sovereignty` requires D-0205 proof tokens and forbids direct runtime request bridge calls across renderer production source. |

Rejected / deferred:

```text
Reasonix SessionAPI, usage/debug public protocol, or upstream route names
deprecated bridge aliases or direct renderer runtime request bypass
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged usage/dashboard walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts
```

## 2026-06-22 - D-0206 Renderer Settings Read Facade Seal

Source:

```text
D-0201 renderer settings bridge proof, D-0205 renderer runtime client facade
seal, keyboard/voice/usage renderer settings readers, and Reasonix settings
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer settings read transport | `code-port-and-adapt` | Route keyboard shortcuts, speech-to-text settings, and initial usage model-label reads through `rendererRuntimeClient.getSettings`. |
| Renderer direct settings read scan | `contract-reimplement` | Extend product-sovereignty scan to forbid `window.analytix.settings.getSettings` in renderer production source. |
| Settings write APIs | `document-only` | Keep explicit `setSettings` / `saveSettingsSilent` write contracts separate from read-facade sealing. |
| Reasonix settings protocol | `reject` | Do not expose upstream config roots or public settings protocol. |
| Packaged settings walkthrough | `defer` | Keep packaged desktop settings QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer reads | `useKeyboardShortcutSettings`, `useSpeechToTextSettings`, and `InitialSessionUsageHeatmap` use `rendererRuntimeClient.getSettings`. |
| Tests | Existing heatmap and runtime-client tests keep settings reads/caching behavior covered. |
| Scan guard | `scan:product-sovereignty` requires D-0206 proof tokens and forbids direct renderer settings reads. |

Rejected / deferred:

```text
Reasonix config roots or public settings protocol
deprecated bridge aliases or direct renderer settings read bypass
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged settings walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npx vitest run src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts src/renderer/src/agent/runtime-client.test.ts
```

## 2026-06-22 - D-0207 Renderer Named Bridge API Allow-list

Source:

```text
D-0200 renderer runtime client bridge proof, D-0201 renderer settings bridge
proof, D-0205 generic runtime request facade seal, D-0206 settings read facade
seal, and Reasonix frontend/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Named renderer runtime preload APIs | `contract-reimplement` | Keep only explicit analytix-owned direct APIs for config file, provider probe, runtime restart, upstream model fetch, and runtime status subscription. |
| Named renderer settings write APIs | `contract-reimplement` | Keep only `setSettings` and `saveSettingsSilent` as direct write contracts. |
| Generic runtime/settings bridge bypass | `reject` | Keep `runtimeRequest` and `getSettings` out of renderer production direct calls; they must use the renderer client facade. |
| Reasonix/Kun bridge aliases or public session protocol | `reject` | Do not expose upstream bridge names, SessionAPI, config roots, or public protocol. |
| Packaged bridge walkthrough | `defer` | Keep packaged desktop bridge QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Scan helper | `scan:product-sovereignty` now has an allow-list helper for direct `window.analytix.runtime.*` / `window.analytix.settings.*` production calls. |
| Runtime allow-list | Allowed direct runtime methods are `fetchUpstreamModels`, `getAnalytixConfigFile`, `onRuntimeStatus`, `openAnalytixConfigDir`, `probeModelProvider`, `restartRuntime`, and `setAnalytixConfigFile`. |
| Settings allow-list | Allowed direct settings methods are `setSettings` and `saveSettingsSilent`. |
| Generic bypass guard | Existing scan guards still forbid direct `window.analytix.runtime.runtimeRequest` and `window.analytix.settings.getSettings` in renderer production source. |

Rejected / deferred:

```text
Reasonix SessionAPI, public bridge protocol, config roots, or upstream route names
deprecated bridge aliases or arbitrary direct window API expansion
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged bridge walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
node --check scripts/scan-product-sovereignty.cjs
npm run scan:product-sovereignty
```

## 2026-06-23 - D-0208 Renderer Optional Bridge Bypass Seal

Source:

```text
D-0207 named bridge allow-list, renderer optional-chaining bridge audit, Connect
Phone dialog settings read path, plugin marketplace MCP diagnostics refresh,
and Reasonix frontend/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Optional-chaining bridge access | `contract-reimplement` | Treat `window.analytix?.runtime?.*` and `window.analytix?.settings?.*` as the same direct bridge surface as dot access. |
| Connect Phone dialog settings read | `code-port-and-adapt` | Route `SidebarClawDialog` settings loading through `rendererRuntimeClient.getSettings`. |
| Plugin marketplace runtime diagnostics guard | `code-port-and-adapt` | Remove direct optional-chain probing of generic `runtimeRequest`; provider diagnostics stay behind the renderer runtime facade. |
| Generic runtime/settings bridge bypass | `reject` | Keep both dot and optional-chain `runtimeRequest` / `getSettings` out of renderer production source. |
| Packaged bridge walkthrough | `defer` | Keep packaged desktop bridge QA for release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Scan guard | `scan:product-sovereignty` now uses optional-chain-aware regex constants for generic bypass scans and named API allow-list scans. |
| Renderer settings path | Connect Phone add/manage dialog reads settings through `rendererRuntimeClient.getSettings`, not `window.analytix?.settings?.getSettings`. |
| Renderer runtime path | Plugin marketplace no longer touches `window.analytix?.runtime?.runtimeRequest` for MCP diagnostics refresh. |
| Boundary | Direct named APIs remain explicit contracts; generic runtime HTTP and ordinary settings reads remain behind `rendererRuntimeClient`. |

Rejected / deferred:

```text
Reasonix SessionAPI, public bridge protocol, config roots, or upstream route names
deprecated bridge aliases or arbitrary optional-chain bridge expansion
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged bridge walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
node --check scripts/scan-product-sovereignty.cjs
rg -n "window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings|window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest" src/renderer/src --glob '!**/*.test.ts'
npm run scan:product-sovereignty
```

## 2026-06-23 - D-0209 Renderer Bridge Allow-list G5 Shadow

Source:

```text
D-0207 named bridge allow-list, D-0208 optional bridge bypass seal, existing
`desktopSovereignty` G5 control case, and Reasonix frontend/session boundary
review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer named bridge allow-list executable replay | `code-port-and-adapt` | Extend `controlExecutableCases.desktopSovereignty` with renderer direct method inventory, allow-list violations, generic bypass counts, and optional-chain scan proof. |
| TypeScript source-derived bridge inventory | `contract-reimplement` | Derive direct runtime/settings methods from real renderer production source, not from Reasonix protocol or hand-written prose. |
| Go G5 shadow replay | `code-port-and-adapt` | Go computes allow-listed runtime/settings methods, generic bypass absence, and optional-chain scan coverage from the TS-owned fixture. |
| Live Go desktop bridge | `defer` | Keep Electron/main/runtime bridge execution TypeScript-owned until G5/G6 readiness. |
| Reasonix public bridge/session protocol | `reject` | Do not expose upstream bridge names, SessionAPI, config roots, or renderer-visible Go routes. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `desktopSovereignty.rendererBridgeAllowList`. |
| TS conformance | `go-runtime-conformance.test.ts` reads renderer production source and scan script source to derive direct methods, violations, bypass counts, and optional-chain coverage. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `rendererNamedRuntimeApisAllowListed`, `rendererNamedSettingsApisAllowListed`, `rendererGenericRuntimeBypassAbsent`, `rendererSettingsReadBypassAbsent`, and `optionalChainBridgeAccessScanned`. |
| Scan freshness | `scan:product-sovereignty` requires these D-0209 G5 proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI, public bridge protocol, config roots, or upstream route names
deprecated bridge aliases or arbitrary direct/optional bridge expansion
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go desktop bridge, packaged bridge walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0210 Runtime HTTP Auth Matrix G5 Shadow

Source:

```text
D-0190 runtime HTTP auth matrix proof, D-0189 runtime HTTP route sovereignty,
existing `runtimeHttpRouteSovereignty` G5 control case, and Reasonix
route/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime HTTP auth matrix shadow replay | `code-port-and-adapt` | Extend `controlExecutableCases.runtimeHttpRouteSovereignty` with `authMatrix` and Go-computed auth coverage output. |
| Auth-before-handler invariant | `contract-reimplement` | Preserve analytix `analytix serve` rule: `/health` is 200 without auth, all 43 `/v1/*` routes reject missing auth with structured 401. |
| Sensitive route protection | `contract-reimplement` | Explicitly include SSE, task-job wait, approval, user-input, and resume-thread route keys in the protected matrix. |
| Live Go HTTP server | `defer` | Keep Go shadow-only until G5/G6 readiness and rollback/packaged QA gates. |
| Reasonix public route/session protocol | `reject` | Do not expose SessionAPI, upstream route names, or renderer-visible Go routes. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `runtimeHttpRouteSovereignty.authMatrix`. |
| TS conformance | `go-runtime-conformance.test.ts` continues dispatching every registered route and now compares results to the fixture auth matrix. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes health 200, all protected routes 401, stable unauthorized body shape, full route coverage, and sensitive-route protection. |
| Scan freshness | `scan:product-sovereignty` requires D-0210 auth-matrix G5 proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI, public route/session protocol, or upstream route names
renderer-visible Go route, live Go HTTP server, or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge/settings fallback, Kun identity, or Rust/Tauri migration
packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0211 Runtime Forbidden Route Dispatch G5 Shadow

Source:

```text
D-0191 runtime forbidden route dispatch proof, D-0189 forbidden token list,
existing `runtimeHttpRouteSovereignty` G5 control case, and Reasonix
route/session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Forbidden dispatch shadow replay | `code-port-and-adapt` | Extend `controlExecutableCases.runtimeHttpRouteSovereignty` with `forbiddenDispatchMatrix` and Go-computed structured 404 coverage output. |
| Upstream route protocol rejection | `contract-reimplement` | Preserve analytix rule that Reasonix/Kun/Go/session route families remain absent even with valid runtime auth. |
| Hidden capability route rejection | `contract-reimplement` | Preserve absence of Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer runtime routes. |
| Live Go HTTP server | `defer` | Keep Go shadow-only until G5/G6 readiness and rollback/packaged QA gates. |
| Reasonix public route/session protocol | `reject` | Do not expose SessionAPI, upstream route names, or renderer-visible Go routes. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix`. |
| TS conformance | `go-runtime-conformance.test.ts` dispatches each forbidden token with valid auth and compares status/body to the fixture matrix. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes structured not_found, full-token coverage, protocol-token rejection, hidden-surface rejection, and valid-auth dispatch mode. |
| Scan freshness | `scan:product-sovereignty` requires D-0211 forbidden-dispatch G5 proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI, public route/session protocol, or upstream route names
renderer-visible Go route, live Go HTTP server, or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge/settings fallback, Kun identity, or Rust/Tauri migration
packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-23 - D-0212 Shared Endpoint Builder G5 Shadow

Source:

```text
D-0192 shared endpoint builder proof, D-0189 route sovereignty, existing
`runtimeHttpRouteSovereignty` G5 control case, and Reasonix route/session
boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Shared endpoint builder shadow replay | `code-port-and-adapt` | Extend `controlExecutableCases.runtimeHttpRouteSovereignty` with `sharedEndpointBuilderMatrix` and Go-computed encoding/template ownership output. |
| Route-id injection prevention | `contract-reimplement` | Preserve analytix rule that route ids containing slash/query/fragment text are URL-encoded before becoming path segments. |
| Canonical user-input endpoint | `contract-reimplement` | Keep shared user input template plural `/v1/user-inputs/{id}` and do not export singular compatibility as canonical. |
| Live Go HTTP server | `defer` | Keep Go shadow-only until G5/G6 readiness and rollback/packaged QA gates. |
| Reasonix public route/session protocol | `reject` | Do not expose SessionAPI, upstream route names, or renderer-visible Go routes. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix`. |
| TS conformance | `go-runtime-conformance.test.ts` derives 18 shared builder outputs, exported endpoint strings, forbidden-token absence, and unit-test proof presence. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes case count, encoded route-id preservation, template ownership, canonical plural user-input, sensitive builder coverage, and unit-proof presence. |
| Scan freshness | `scan:product-sovereignty` requires D-0212 shared-endpoint G5 proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI, public route/session protocol, or upstream route names
renderer-visible Go route, live Go HTTP server, or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
deprecated bridge/settings fallback, Kun identity, or Rust/Tauri migration
packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0191 Runtime Forbidden Route Dispatch Proof

Source:

```text
Reasonix route/control review, D-0189 forbidden route token list, and the
active analytix TypeScript HTTP router.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Forbidden upstream/hidden route absence | `contract-reimplement` | Dispatch each forbidden token with valid auth through the real router and require structured 404. |
| Reasonix SessionAPI / route protocol | `reject` | Keep `/v1/reasonix`, `/session-api`, and related upstream route families unregistered. |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route exposure | `reject` | Keep hidden capability route families absent from the runtime HTTP surface. |
| Live Go HTTP server matrix | `defer` | Keep Go serving and packaged route walkthrough for G6/release gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Actual HTTP dispatch | `go-runtime-conformance.test.ts` now runs `returns structured not_found for forbidden public route tokens from the G5 route-sovereignty fixture`. |
| Fixture tie | The matrix is driven by `runtimeHttpRouteSovereignty.forbiddenRouteTokens`, so forbidden-route governance remains attached to the route sovereignty oracle. |
| Scan guard | `scan:product-sovereignty` now checks D-0191 runtime HTTP forbidden route dispatch proof tokens and final gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
renderer-visible Go route or default Go backend
Rust/Tauri migration
live Go HTTP server, packaged route walkthrough, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0155 Browser Preview Bridge And Visible Entry Sovereignty

Source:

```text
Reasonix public protocol/bridge rejection, D-0152 desktop bridge sovereignty,
D-0154 session route replay, and the Kun entry-baseline preservation rule.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Browser preview bridge source guard | `contract-reimplement` | Treat the dev browser bridge as an analytix-owned public facade surface: it may install only `window.analytix`, use the analytix runtime proxy, and preserve `/v1/threads/:id/events` SSE path semantics. |
| Visible top-level entry smoke | `contract-reimplement` | Render the sidebar and reject Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer visible labels from the top-level shell. |
| Product-sovereignty scan freshness | `contract-reimplement` | Make `scan:product-sovereignty` fail if Workbench, Sidebar, preload/shared contracts, browser bridge, or guard tests disappear from expected coverage. |
| Reasonix public bridge/session protocol | `reject` | Do not expose Reasonix `SessionAPI`, public protocol names, deprecated aliases, or renderer-visible Go routes. |
| Live Go backend / packaged desktop walkthrough | `defer` | This batch adds source/render/scan evidence only; it does not advance G5/G6 runtime readiness. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Browser bridge | `Workbench.route-surface.test.ts` reads `browser-analytix-bridge.ts` and proves only `window.analytix` plus analytix runtime proxy/SSE path are present. |
| Visible shell | `Sidebar.test.ts` statically renders the sidebar and rejects Kun/Reasonix-style hidden-capability entry labels. |
| Scan guard | `scripts/scan-product-sovereignty.cjs` now has path freshness checks and includes browser preview bridge in the quarantine path set. |

Rejected / deferred:

```text
Reasonix public protocol or SessionAPI
window.kun/window.reasonix/window.deepseek/window.analytixGui/kunGui/reasonixGui
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
default Go backend or renderer-visible Go route
packaged desktop browser/route walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0156 Browser Preview SSE Executable Bridge Proof

Source:

```text
D-0155 browser preview bridge sovereignty and Reasonix session/SSE public
protocol rejection.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Browser preview SSE proxy path | `contract-reimplement` | Prove `window.analytix.runtime.startSse` in browser preview requests the analytix `/v1/threads/:id/events` proxy path. |
| SSE replay cursor header | `contract-reimplement` | Preserve analytix `since_seq` replay semantics by asserting `Last-Event-ID`. |
| SSE event normalization | `code-port-and-adapt` | Keep renderer-facing payloads analytix-owned by normalizing SSE `id` to `seq` and `event` to `kind`. |
| Reasonix SessionAPI/public event protocol | `reject` | Do not expose upstream session route names, event shapes, or public protocol to browser preview. |
| Packaged browser walkthrough/live Go bridge | `defer` | Unit-level executable proof only; no G6 or release-readiness claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Browser preview bridge | `browser-analytix-bridge.test.ts` now exercises `startSse` against a fake SSE response and asserts proxy URL/header/payload behavior. |
| Renderer contract | Event payloads arrive with `streamId`, `events`, `seq`, and `kind`, matching analytix renderer expectations. |

Rejected / deferred:

```text
Reasonix SessionAPI/public SSE protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged browser/desktop route walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0157 Browser Preview Settings Sovereignty Proof

Source:

```text
D-0152 desktop bridge/settings sovereignty, D-0155 browser bridge sovereignty,
and Reasonix config/auto-plan rejection.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Shared top-level rejected field stripping | `contract-reimplement` | Extend `normalizeAppSettings` so top-level `agentProvider`/`agents`/`deepseek`/`reasonix` are stripped like Reasonix `agent`/`autoPlan`/`auto_plan`. |
| Browser preview settings load/re-save proof | `contract-reimplement` | Prove polluted browser preview localStorage preserves valid top-level `runtime` fields but drops rejected app/runtime fields on load and save. |
| Runtime-level rejected fields | `code-port-and-adapt` | Reuse existing `mergeAnalytixRuntimeSettings` strip behavior for runtime-level `agentProvider`/`agents`/`deepseek`/`reasonix` and auto-plan shapes. |
| Reasonix config root / auto-plan setting | `reject` | Do not expose Reasonix config shape, project/local auto-plan override, or public settings protocol. |
| Packaged settings QA / live Go settings bridge | `defer` | Unit/shared tests only; no default Go backend or release-readiness claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Shared settings | `app-settings.test.ts` proves normalized app settings drop top-level and runtime legacy/Reasonix envelopes. |
| Browser preview | `browser-analytix-bridge.test.ts` proves browser preview load and re-save keep `runtime.model` / `endpointFormat` but drop rejected app/runtime fields. |

Rejected / deferred:

```text
Reasonix config root or public auto-plan setting
legacy Kun/Reasonix agentProvider fallback
deprecated settings fallback writes
renderer-visible Go route or default Go backend
packaged settings walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/shared/app-settings.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0158 IPC Settings Patch Sovereignty Proof

Source:

```text
D-0157 browser preview settings sovereignty, desktop IPC settings schema, and
Reasonix config/auto-plan rejection.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| IPC top-level rejected field stripping | `contract-reimplement` | Strip `agent`, `autoPlan`, `auto_plan`, `agentProvider`, `agents`, `deepseek`, `reasonix`, and `quickChat` before strict settings patch validation. |
| Valid settings patch preservation | `contract-reimplement` | Keep legal analytix `locale`, `disabledSkillIds`, provider patch, and runtime patch fields even when the patch is polluted. |
| Runtime nested legacy shapes | `reject` | Continue rejecting runtime-level `agentProvider`/`agents`/`reasonix` shapes instead of treating them as fallback runtime config. |
| Reasonix config root / public auto-plan setting | `reject` | Do not expose Reasonix config shape, project/local auto-plan override, or public settings protocol. |
| Packaged settings QA / live Go settings bridge | `defer` | Unit-level IPC proof only; no default Go backend or release-readiness claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| IPC schema | `app-ipc-schemas.ts` strips top-level legacy/Reasonix patch keys before strict schema validation. |
| IPC tests | `app-ipc-schemas.test.ts` proves polluted renderer patches preserve legal analytix fields and drop rejected envelopes. |

Rejected / deferred:

```text
Reasonix config root or public auto-plan setting
legacy Kun/Reasonix agentProvider fallback
deprecated settings fallback writes
renderer-visible Go route or default Go backend
packaged settings walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0160 Workflow Singular Route Negative Proof

Source:

```text
Reasonix public route rejection, task-job forbidden top-level route oracle,
and Go G4/G5 forbidden route replay.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Singular Workflow route rejection | `contract-reimplement` | Add `/v1/workflow` to the live `http-server.test.ts` forbidden upstream public route table. |
| Oracle alignment | `contract-reimplement` | Align live HTTP proof with task-job / Go G4/G5 fixtures that already record `/v1/workflow` as forbidden. |
| Workflow/Create Loop public protocol | `reject` | Do not expose Workflow, Create Loop, Subagent, AutoResearch, MCP-indexer, SessionAPI, or Reasonix public route names. |
| Packaged route QA / live Go HTTP routes | `defer` | Unit-level HTTP router proof only; Go remains shadow-only and no default backend is enabled. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| HTTP router | `/v1/workflow` and `/v1/workflows` both return structured 404s. |
| Conformance alignment | Live HTTP negative route coverage now matches the singular path already used by task-job and Go route-boundary oracles. |

Rejected / deferred:

```text
Reasonix SessionAPI or public protocol
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
packaged route walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0161 Runtime Proof Freshness Scan

Source:

```text
Reasonix provider/cache, approval/user-input, route replay, and Go shadow
conformance review; D-0160 runtime route negative proof.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime proof path freshness | `contract-reimplement` | Add `runtimeProofFreshnessPaths` to `scan:product-sovereignty` for provider/cache, approval/user-input, G2/G3/G4/G5 fixtures/tests, and Go shadow sources. |
| Active route/client forbidden scan | `contract-reimplement` | Scan runtime routes, endpoint templates, IPC schema, and renderer runtime client for forbidden Reasonix/Go/Workflow public route strings. |
| Go conformance currentness table | `document-only` | Update the top-level Go conformance table so G2 exact route replay is closed and G3/G4/G5 provider/cache plus approval/user-input oracle evidence is active. |
| New behavior oracle / live Go backend | `defer` / `reject` | Do not add provider/cache behavior, credentialed provider matrix, live Go HTTP/provider client, or default backend. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Product sovereignty scan | `scan:product-sovereignty` now fails if runtime conformance proof files or active route/client source surfaces disappear or expose forbidden public route strings. |
| Go conformance docs | Currentness now reflects closed G2 exact replay and active provider/cache plus approval/user-input shadow evidence. |

Rejected / deferred:

```text
Reasonix public protocol or SessionAPI
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
new provider/cache behavior oracle or credentialed provider matrix
live Go HTTP/provider client, renderer-visible Go route, or default Go backend
packaged route/provider QA
Rust/Tauri rewrite
```

Focused validation:

```text
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0162 Renderer Runtime Request Surface Proof

Source:

```text
Reasonix public route rejection, renderer runtime adapter review, and D-0161
runtime route/client source scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer runtime path contract | `contract-reimplement` | Add an executable `AnalytixRuntimeProvider` test that captures common renderer runtime `runtimeRequest` paths. |
| Forbidden public route rejection | `contract-reimplement` | Assert captured paths do not match Reasonix public routes, renderer-visible Go routes, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public routes. |
| Reasonix/Kun public protocol | `reject` | Do not expose SessionAPI, Kun public protocol, deprecated bridge/settings fallback, or top-level hidden capability route. |
| Packaged desktop route QA / live Go | `defer` / `reject` | Unit-level renderer provider proof only; no live Go backend, renderer-visible Go route, or packaged desktop claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer provider | Connect/list/create/turn/steer/interrupt/compact/goal/todos/approval/user-input/fork/resume calls stay on analytix-owned runtime paths. |
| Product boundary | Captured renderer paths reject Reasonix/Go/Workflow public surfaces. |

Rejected / deferred:

```text
Reasonix public protocol or SessionAPI
Kun public protocol or identity
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
packaged desktop route walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0163 Renderer SSE Bridge Cursor Proof

Source:

```text
Reasonix SSE/session protocol review, analytix renderer runtime adapter, and
D-0156 browser preview SSE bridge proof.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer SSE bridge cursor contract | `contract-reimplement` | Extend `AnalytixRuntimeProvider.subscribeThreadEvents` coverage so the renderer proves it starts through `window.analytix.runtime.startSse(threadId, sinceSeq, streamId)`. |
| Stream id continuity and cleanup | `contract-reimplement` | Assert a generated non-empty stream id is used for event dispatch and passed back to `stopSse` during cleanup. |
| SSE side-channel avoidance | `contract-reimplement` | Assert renderer SSE subscription does not issue a direct `runtimeRequest`; bridge/main/browser SSE route proofs remain separate. |
| Reasonix public SSE/session protocol | `reject` | Do not expose SessionAPI, Reasonix SSE routes, Kun protocol, deprecated bridge/settings fallback, or hidden top-level capability routes. |
| Packaged desktop/live Go SSE QA | `defer` / `reject` | Unit-level renderer bridge proof only; no live Go backend, renderer-visible Go route, or packaged desktop claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Renderer provider | `subscribeThreadEvents('thr_1', 2, ...)` calls `startSse('thr_1', 2, streamId)` with a generated stream id. |
| Cleanup | Aborted renderer streams call `stopSse(streamId)` for the same stream id that delivered events. |
| Product boundary | The SSE path stays behind `window.analytix.runtime` and does not create Reasonix/Go/Workflow public surfaces. |

Rejected / deferred:

```text
Reasonix public SSE or SessionAPI protocol
Kun public protocol or identity
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
packaged desktop SSE walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0164 Main/Preload SSE IPC Bridge Proof

Source:

```text
D-0163 renderer SSE bridge proof, Reasonix public SSE/session rejection, and
analytix desktop preload/main SSE IPC review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload SSE IPC facade | `contract-reimplement` | `preload-sandbox.test.ts` now proves `window.analytix.runtime.startSse/stopSse` map only to `runtime:sse:*` IPC. |
| Main SSE start schema | `contract-reimplement` | `app-ipc-schemas.test.ts` proves `sseStartPayloadSchema` preserves trimmed `threadId`/`sinceSeq`/`streamId`, permits omitted stream id, and rejects Reasonix-style extra fields. |
| Main SSE route/header/cursor | `contract-reimplement` | `runtime-sse-ipc.test.ts` proves first fetch uses `/v1/threads/:id/events?since_seq=...`, runtime auth, and initial `Last-Event-ID` when the cursor is non-zero. |
| Stream id continuity and stop precision | `contract-reimplement` | Main IPC proof covers generated stream ids in error payloads and `runtime:sse:stop` aborting only the matching stream id. |
| Reconnect/throttle/safe-send | `code-port-and-adapt` | Existing main SSE IPC tests continue to cover highest-flushed cursor reconnect, 100ms batching, and destroyed renderer safe-stop. |
| Product/bridge sovereignty | `reject` | Do not expose Reasonix/Kun public SSE protocol, deprecated bridge aliases, renderer-visible Go routes, or hidden Workflow-style public surfaces. |
| Packaged/live SSE QA | `defer` | Unit/source proof only; packaged desktop streaming/reconnect walkthrough and live Go SSE parity remain future release evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Preload bridge | SSE stays under `window.analytix.runtime` and `runtime:sse:*` IPC. |
| Main IPC schema | Start payload remains `threadId` + `sinceSeq` + optional `streamId`; Reasonix session fields are rejected by strict schema. |
| Main worker | Runtime fetch path/header/cursor and generated stream id error payload are pinned by `runtime-sse-ipc.test.ts`. |
| Stop lifecycle | Wrong stream ids do not abort an in-flight SSE start; matching ids do. |

Rejected / deferred:

```text
Reasonix public SSE routes or SessionAPI
Kun public protocol or bridge alias
window.kunGui / window.reasonix / deprecated bridge fallback
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
Rust/Tauri rewrite
packaged desktop SSE walkthrough or release readiness
```

Focused validation:

```text
npm run test -- src/main/runtime-sse-ipc.test.ts src/main/ipc/app-ipc-schemas.test.ts src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0165 Main Runtime Request Forbidden Route Proof

Source:

```text
D-0162 renderer runtime request surface proof, D-0164 desktop SSE IPC proof,
and analytix main IPC runtime request allow-list review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main runtime request forbidden route table | `contract-reimplement` | `app-ipc-schemas.test.ts` explicitly rejects Reasonix, renderer-visible Go, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer request paths. |
| Handler boundary no-call proof | `contract-reimplement` | `register-app-ipc-handlers.test.ts` proves invalid `runtime:request` payloads are rejected before `runtimeRequest` is invoked. |
| Analytix allow-list ownership | `contract-reimplement` | Public renderer runtime requests remain limited to modeled analytix endpoint templates in `app-ipc-schemas.ts`. |
| Reasonix/Kun/Go public runtime protocols | `reject` | Do not expose Reasonix SessionAPI/runtime routes, Kun public protocol, renderer-visible Go routes, or hidden top-level capability routes. |
| Packaged/live route QA | `defer` | Unit-level IPC schema/handler proof only; packaged desktop route walkthrough remains release evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Main IPC schema | Forbidden upstream public route families throw `runtime request path is not allowed`. |
| Main IPC handler | Invalid route payloads throw `Invalid payload for runtime:request` and do not call `runtimeRequest`. |
| Product boundary | Runtime requests stay on analytix-owned endpoint templates; forbidden strings remain only as negative test evidence. |

Rejected / deferred:

```text
Reasonix public runtime routes or SessionAPI
Kun public protocol or identity
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
Rust/Tauri rewrite
packaged desktop route walkthrough or release readiness
```

Focused validation:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/main/ipc/register-app-ipc-handlers.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0166 Preload Runtime Request IPC Bridge Proof

Source:

```text
D-0162 renderer runtime request proof, D-0165 main runtime request allow-list
proof, and analytix preload bridge review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Preload runtimeRequest IPC facade | `contract-reimplement` | `preload-sandbox.test.ts` proves `runtimeRequest(path, method, body)` maps to `runtime:request` with `{ path, method, body }`. |
| Upstream IPC channel rejection | `reject` | The same source guard rejects Reasonix/Kun/Go/Workflow runtime request IPC channel names. |
| Runtime route allow-list handoff | `contract-reimplement` | D-0166 completes the handoff into the D-0165 main IPC allow-list; preload does not own route authorization. |
| Packaged/live route QA | `defer` | Source-level preload proof only; packaged desktop route walkthrough remains release evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Preload bridge | Runtime HTTP requests stay under `window.analytix.runtime.runtimeRequest` and `runtime:request` IPC. |
| Product boundary | No Reasonix, Kun, Go, or Workflow IPC channel names are introduced for runtime requests. |

Rejected / deferred:

```text
Reasonix public runtime routes or SessionAPI
Kun public protocol or bridge alias
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
Rust/Tauri rewrite
packaged desktop route walkthrough or release readiness
```

Focused validation:

```text
npm run test -- src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0167 Runtime Desktop Bridge Proof Freshness Scan

Source:

```text
D-0156 browser runtime/SSE bridge proof and D-0162 through D-0166 renderer,
preload, main IPC runtime request/SSE bridge proofs.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime desktop bridge proof freshness | `contract-reimplement` | Add `runtimeDesktopBridgeProofPaths` to `scan:product-sovereignty` so bridge proof source/test files cannot silently disappear. |
| Renderer/preload/main bridge coverage | `contract-reimplement` | Freshness list covers renderer provider/client, browser preview bridge, preload bridge/types, main IPC schema/handler, main SSE IPC, shared API/endpoints, and focused tests. |
| New runtime behavior | `document-only` | This batch changes the scan guard only; no runtime request, SSE, provider, or UI behavior changes. |
| Reasonix/Kun/Go public surfaces | `reject` | Do not expose Reasonix/Kun public protocols, renderer-visible Go routes, or hidden Workflow-style routes. |
| Packaged/live route QA | `defer` | Freshness scan is not packaged desktop route/SSE walkthrough evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Product-sovereignty scan | `npm run scan:product-sovereignty` now fails if the desktop runtime bridge proof sources/tests disappear. |
| Product boundary | The scan remains path freshness plus existing forbidden-surface checks; forbidden upstream route strings stay in tests/docs only. |

Rejected / deferred:

```text
Reasonix public runtime/SSE routes or SessionAPI
Kun public protocol or bridge alias
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route
renderer-visible Go route or default Go backend
Rust/Tauri rewrite
packaged desktop route/SSE walkthrough or release readiness
```

Focused validation:

```text
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0169 G5 Provider Cache Coverage Floor Control Shadow

Source:

```text
D-0168 provider/cache coverage floor, Reasonix provider/cache currentness, and
analytix Go G5 executable control shadow.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider/cache coverage floor executable replay | `code-port-and-adapt` | Add `controlExecutableCases.providerCacheCoverageFloor` so Go G5 computes the provider-family matrix from TS-owned fixture cases. |
| DeepSeek/OpenAI/Anthropic/custom split | `contract-reimplement` | Replay DeepSeek usage/request-shape ids, OpenAI-compatible chat request-shape id, OpenAI Responses usage/request-shape ids, Anthropic usage/request-shape ids, and custom full endpoint exact-URL/tool-shape ids. |
| Live provider execution | `defer` | Keep Go provider client and credentialed provider matrix out of this batch. |
| Reasonix provider protocol | `reject` | Do not expose Reasonix provider routes, config roots, SessionAPI, or public runtime protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS oracle | `go-runtime-conformance.test.ts` derives the control case from `provider-cache-oracle.json`. |
| Go shadow | `shadow_g5.go` computes coverage floor output from fixture case data, and `shadow_test.go` compares it to the TS-owned expected output. |
| Product boundary | Output includes `liveCredentialsUsed:false`, `mayClaimLiveSuperiority:false`, `usesReasonixProtocol:false`, and `topLevelRouteExposed:false`. |

Rejected / deferred:

```text
Reasonix provider public protocol or config shape
live Go provider client or default Go backend
credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix
live provider/cache superiority claim
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged provider settings QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0170 Provider Request-Shape Derived Replay

Source:

```text
Reasonix provider/cache currentness, D-0145 request-shape exact matrix replay,
D-0168/D-0169 provider coverage floor, and analytix Base URL / Endpoint format
contracts.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Request URL derivation replay | `contract-reimplement` | Derive expected URL from `baseUrl` and `endpointFormat`, including `/v1` path appending and custom full endpoint exact URL preservation. |
| Header/body/tool-shape derivation replay | `contract-reimplement` | Derive OpenAI chat, OpenAI Responses, Anthropic Messages, and custom full endpoint header/body/tool families from fixture inputs. |
| G3/G5 Go shadow replay | `code-port-and-adapt` | Extend G3 and G5 shadow outputs with derived URL/header/body/tool match ids, custom full endpoint exact-url ids, and appended-path count `0`. |
| Provider runtime behavior change | `reject` | Do not change active URL construction, headers, request bodies, stream parsing, usage parsing, or provider defaults in this batch. |
| Live provider matrix / Go provider client | `defer` / `reject` | Keep credentialed provider QA and Go backend activation outside this fixture-only proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS fixtures/schema | `runtime-parity-fixtures.ts` requires derived match ids and custom full endpoint exact-url proof in request-shape summaries. |
| TS conformance | `go-runtime-conformance.test.ts` and `go-runtime-g3-g4-conformance.test.ts` derive the new fields from `provider-cache-oracle.json`. |
| Go shadow | `shadow_g3g4.go` and `shadow_g5.go` compute URL/header/body/tool-shape matches from fixture inputs, and Go tests compare them to TS-owned fixtures. |

Rejected / deferred:

```text
Reasonix provider public protocol or config shape
provider runtime behavior change
live Go provider client or default Go backend
credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged provider settings QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0171 Post-881 Runtime Proof Freshness V2

Source:

```text
D-0151 through D-0170 post-881 proof chain, Reasonix planner/task/MCP/provider
review, and reusable product-sovereignty scan gate.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider proof freshness tokens | `contract-reimplement` | Require scan-visible tokens for provider coverage floor and request-shape derived replay. |
| Planner/task proof freshness tokens | `contract-reimplement` | Require scan-visible planner/task-job tokens for planner gating, task planner inventory, and parallel dependency validation. |
| MCP proof freshness tokens | `contract-reimplement` | Require scan-visible MCP lifecycle/search/approval tokens for MCP/indexer absorption evidence. |
| Runtime behavior change | `reject` | Do not change provider, planner, task-job, MCP, approval/user-input, or route behavior in this scan-only batch. |
| Live Go/provider/MCP/job client | `defer` / `reject` | Keep live clients and backend activation outside this proof-freshness guard. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Scan gate | `scan-product-sovereignty.cjs` now requires task-job/MCP lifecycle fixture/test leaves and positive post-881 proof tokens. |
| Provider proof | The scan must still see `providerCacheCoverageFloor`, `derivedUrlMatchCaseIds`, and `customFullEndpointAppendedPathCount`. |
| Planner/task proof | The scan must still see `plannerToolsetInventory`, `parallelValidation`, `controlExecutableCases.planner`, and `create_plan`. |
| MCP proof | The scan must still see `mcpCoreLifecycle`, `mcpSearchMetaTools`, `mcpLiveLocalIndexer`, and `mcpApprovalAnnotations`. |

Rejected / deferred:

```text
Reasonix public protocol or config shape
provider/runtime behavior changes
live Go provider/MCP/job clients or default Go backend
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
credentialed provider/MCP matrix, packaged QA, or release readiness
```

Focused validation:

```text
npm run scan:product-sovereignty
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/task-job-orchestration-oracle.test.ts tests/mcp-tool-lifecycle-oracle.test.ts tests/create-plan-tool.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0172 Provider Cache Accounting Raw Payload Replay

Source:

```text
Reasonix provider/cache accounting review, D-0099 usage parser precedence,
D-0119 provider cache accounting control shadow, and analytix provider-cache
oracle raw response bodies.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Raw provider usage to cache accounting | `contract-reimplement` | Derive prompt/completion/reasoning/cache hit/miss/rate values from fixture `responseBody.usage` before computing accounting totals. |
| DeepSeek native/prompt cache precedence | `contract-reimplement` | Preserve native `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` precedence, then OpenAI-compatible `prompt_tokens_details.cached_tokens` fallback. |
| G3/G5 Go shadow replay | `code-port-and-adapt` | Carry raw `responseBody` into G3/G5 fixtures and have Go shadow compute raw parsed ids, raw telemetry-supported ids, expected-usage match ids, provider-family ids, totals, and aggregate rate. |
| Active provider behavior change | `reject` | Do not change URL/body construction, stream parsing, runtime model clients, or provider defaults in this batch. |
| Live provider matrix / default Go backend | `defer` / `reject` | Keep credentialed provider QA and Go backend activation outside this raw-fixture proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS fixtures/schema | `runtime-parity-fixtures.ts` requires raw `responseBody` on provider usage/accounting rows and raw replay fields on `cacheAccounting`. |
| TS conformance | `go-runtime-conformance.test.ts` and `go-runtime-g3-g4-conformance.test.ts` parse raw provider payloads before deriving cache accounting. |
| Go shadow | `shadow_g3g4.go` and `shadow_g5.go` compute the same accounting from raw payload maps. |

Rejected / deferred:

```text
Reasonix provider public protocol or config shape
provider runtime behavior change
live Go provider client or default Go backend
credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged provider settings QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0173 Provider Raw Accounting Proof Freshness

Source:

```text
D-0172 raw provider cache accounting replay, Reasonix provider/cache review,
and analytix product-sovereignty scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Direct raw accounting provider-cache proof | `contract-reimplement` | Add a provider-cache oracle test that parses raw `responseBody.usage` into usage snapshots and aggregate accounting. |
| Raw accounting scan freshness | `contract-reimplement` | Require raw accounting tokens in `scan:product-sovereignty` so D-0172 proof cannot silently disappear. |
| Go/runtime behavior change | `reject` | Do not change active provider clients, request URL/body behavior, stream parsing, or Go backend status. |
| Live provider matrix | `defer` | Keep credentialed DeepSeek/OpenAI/Anthropic/custom QA outside this scan/test batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Direct oracle | `provider-cache-proof.test.ts` proves raw parsed ids, telemetry-supported ids, expected-usage matches, provider-family ids, `2930/720` totals, and aggregate hit rate from raw payloads. |
| Scan gate | `scan-product-sovereignty.cjs` checks raw accounting proof tokens across post-881 runtime proof paths. |

Rejected / deferred:

```text
Reasonix provider public protocol or config shape
provider runtime behavior change
live Go provider client or default Go backend
credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged provider settings QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0174 Combined Step/Cancel/Cache Proof Freshness

Source:

```text
D-0146 combined step/cancel/cache trace control shadow, Reasonix agent-kernel
step/cancel/cache review, and analytix product-sovereignty scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Focused combined trace oracle | `contract-reimplement` | Add a dedicated G5 conformance test for same-turn route-cache reuse, step-limit boundary, cancel result pairing, stable-prefix isolation, and product boundary flags. |
| Combined trace scan freshness | `contract-reimplement` | Require combined trace tokens in `scan:product-sovereignty` so D-0146 proof cannot silently disappear. |
| Go/runtime behavior change | `reject` | Do not change the active TypeScript agent loop, provider cache logic, cancellation behavior, or Go backend status. |
| Live Go loop / packaged cancel-cache QA | `defer` | Keep live agent-loop parity and packaged walkthroughs outside this focused proof batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Focused oracle | `go-runtime-conformance.test.ts` proves `controlExecutableCases.combined.expected` from G5 fixture inputs and boundary fields. |
| Scan gate | `scan-product-sovereignty.cjs` checks combined route-cache/step/cancel proof tokens across post-881 runtime proof paths. |

Rejected / deferred:

```text
Reasonix controller/session public protocol or config shape
public auto-plan setting or upstream route names
live Go agent loop or default Go backend
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged long-running cancel/cache QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0175 Approval/User-Input Proof Freshness

Source:

```text
Reasonix approval/user-input lifecycle review, analytix approval-user-input
route oracle, and product-sovereignty scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Focused approval/user-input gate oracle | `contract-reimplement` | Add a dedicated G5 conformance test for denied no-execute, user-input answer privacy, structured validation, abort cleanup, and resume cleanup. |
| Approval/user-input scan freshness | `contract-reimplement` | Require gate proof tokens in `scan:product-sovereignty` so approval/user-input evidence cannot silently disappear. |
| Go/runtime behavior change | `reject` | Do not change the active TypeScript approval/user-input managers, routes, or Go backend status. |
| Live Go gate manager / packaged approval-card QA | `defer` | Keep live parity and packaged walkthroughs outside this focused proof batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Focused oracle | `go-runtime-conformance.test.ts` proves approval deny no-execute, answer privacy, late `409`/`404`, pending cleanup, resume answer omission, and boundary flags. |
| Scan gate | `scan-product-sovereignty.cjs` checks approval/user-input route replay and inventory tokens across post-881 runtime proof paths. |

Rejected / deferred:

```text
Reasonix ask/session public protocol or config shape
live Go approval/user-input manager or default Go backend
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged approval-card QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0176 Session Route Proof Freshness

Source:

```text
Reasonix session/fork/SSE replay review, analytix G2 route replay oracle,
G5 control executable oracle, and product-sovereignty scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Focused thread/session route oracle | `contract-reimplement` | Add a dedicated G5 conformance test for archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, and boundary flags. |
| Exact route body/SSE hashes | `code-port-and-adapt` | Carry archive/search/fork/resume response hashes, replay SSE hash, caught-up SSE hash, and unauthorized body hash from the TS-owned G2 oracle into G5 proof. |
| Session route scan freshness | `contract-reimplement` | Require session route replay proof tokens in `scan:product-sovereignty` so thread/SSE route evidence cannot silently disappear. |
| Go/runtime behavior change | `reject` | Do not change active TypeScript thread/session routes, renderer bridge, runtime HTTP/SSE contract, or Go backend status. |
| Live Go router / packaged route QA | `defer` | Keep live parity and packaged thread/fork/resume/archive/search walkthroughs outside this focused proof batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Focused oracle | `go-runtime-conformance.test.ts` proves route status, route inventory, exact body/SSE hash replay, auth rejection, and shadow-only boundary flags. |
| Scan gate | `scan-product-sovereignty.cjs` checks session route replay tokens across post-881 runtime proof paths and includes the G2 route oracle fixture. |

Rejected / deferred:

```text
Reasonix SessionAPI/public event protocol or route names
live Go thread/session router or default backend
renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged route walkthrough QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

## 2026-06-22 - D-0177 Release Evidence Final-Gate Freshness

Source:

```text
Post-881 proof chain D-0172 through D-0177, release evidence gate, and
product-sovereignty scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Final-gate evidence freshness | `contract-reimplement` | Add machine-checkable scan coverage for D-0172 through D-0177 final command gate entries. |
| Required command visibility | `document-only` | Require release evidence text to keep runtime package tests, workspace tests, typecheck, runtime build, non-cached Go shadow tests, and product-sovereignty scan commands visible. |
| Runtime/provider/Go behavior change | `reject` | Do not change active TypeScript runtime, provider clients, renderer bridge, runtime HTTP/SSE contract, or Go backend status. |
| Packaged/live release QA | `defer` | Keep packaged walkthroughs, live provider/MCP matrices, and release readiness outside this scan-only batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Scan gate | `scan-product-sovereignty.cjs` includes `releaseEvidenceProofPaths` and checks final gate tokens for D-0172 through D-0177. |
| Stage closure | Release evidence command coverage is now part of the repeatable product-sovereignty scan instead of a manual-only convention. |

Rejected / deferred:

```text
Reasonix public protocol, config roots, or controller API
runtime/provider behavior changes
live Go backend, renderer-visible Go route, or G6 readiness
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged desktop QA or release readiness
```

Focused validation:

```text
npm run scan:product-sovereignty
git diff --check
```

## 2026-06-22 - D-0178 Post-881 Stage Closure Snapshot

Source:

```text
Reasonix `881b2f2f..9ada14176629b1d59d7ed78446951b2bb5954904`,
D-0172 through D-0177 proof chain, upstream scorecard, and release evidence.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Capability floor snapshot | `contract-reimplement` | Add a machine-scannable closure artifact for current fixture-level Reasonix parity floors. |
| Absorbed Reasonix delta families | `document-only` | Record auto-plan/currentness, planner gating, step/cancel/cache, provider cache accounting, MCP lifecycle, and task/job orchestration classifications. |
| Stronger-than statement | `document-only` | Limit stronger-than claims to verified product sovereignty, multi-provider fixture coverage, bridge safety, forbidden-surface governance, and evidence closure. |
| Live parity/release claims | `reject` / `defer` | Keep live provider/cache superiority, packaged QA, live Go backend, G6 readiness, and release readiness open. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Closure artifact | `post-881-stage-closure-2026-06-22.md` lists current floors, absorbed deltas, Kun baseline preservation, stronger-than evidence, and remaining gates. |
| Scan gate | `scan-product-sovereignty.cjs` requires closure tokens including `post881StageClosureCapabilityFloor`, `reasonixAbsorbedDeltas`, `kunBaselinePreserved`, `analytixExceedsReasonixWhere`, and `remainingOpenGates`. |

Rejected / deferred:

```text
Reasonix public protocol, config roots, or controller API
live provider/cache superiority matrix
live Go backend, renderer-visible Go route, or G6 readiness
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged desktop QA or release readiness
```

Focused validation:

```text
npm run scan:product-sovereignty
git diff --check
```

## 2026-06-22 - D-0168 Provider Cache Coverage Floor

Source:

```text
Reasonix provider/cache and DeepSeek cache-currentness review; existing
analytix provider-cache oracle and Go G3/G5 shadow inputs.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| DeepSeek cache telemetry floor | `contract-reimplement` | Keep required DeepSeek prompt-cache and native-cache-precedence usage cases in the analytix-owned provider-cache oracle. |
| OpenAI/Anthropic/custom provider non-regression floor | `contract-reimplement` | Derive and assert provider-family coverage for OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full endpoint request shapes. |
| Provider family metadata schema | `contract-reimplement` | Tighten `ProviderLiveLocalHttpProof.coveredProviderFamilies` from loose strings to the five accepted fixture families. |
| Live credential matrix | `defer` | Keep this as fixture/local-HTTP evidence; live provider/cache superiority remains blocked by credentialed QA. |
| Reasonix public provider protocol | `reject` | Do not expose Reasonix provider routes, config roots, or public runtime protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime test | `provider-cache-proof.test.ts` now fails if required usage ids, request-shape ids, endpoint formats, custom full endpoints, unsupported unknown cache posture, or provider-family coverage are removed. |
| Fixture schema | `runtime-parity-fixtures.ts` constrains provider-family proof to DeepSeek, OpenAI-compatible, OpenAI Responses, Anthropic Messages, and custom full endpoints. |
| Go shadow input | Existing Go G3/G5 shadow parses the same provider-cache oracle; no Go runtime execution was added. |

Rejected / deferred:

```text
Reasonix provider public protocol or config shape
live provider/cache superiority claim
credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix
default Go backend or renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, or top-level hidden entries
Rust/Tauri rewrite
packaged provider settings QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0159 Settings Sovereignty Scan Freshness Proof

Source:

```text
D-0155 browser bridge/entry sovereignty, D-0157 browser settings
normalization, D-0158 IPC settings patch sovereignty, and Reasonix config /
auto-plan rejection.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Settings chokepoint path freshness | `contract-reimplement` | Add `settingsSovereigntyPaths` to `scan:product-sovereignty` for shared normalize/runtime settings, main settings store, IPC schema, preload, and browser preview bridge paths. |
| Guard test path freshness | `contract-reimplement` | Require the focused settings/bridge guard tests to remain present: shared settings, settings-store, IPC schema, preload sandbox, and browser preview bridge. |
| Reasonix config/public auto-plan surface | `reject` | Do not expose Reasonix config roots, project/local auto-plan settings, SessionAPI, or public settings protocol. |
| Packaged settings QA / live Go bridge | `defer` | Scan freshness only; no packaged settings walkthrough, live Go settings bridge, or default backend claim. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Product sovereignty scan | `scripts/scan-product-sovereignty.cjs` now fails if settings/bridge chokepoints or their guard tests are missing. |
| Release evidence | `npm run scan:product-sovereignty` is the focused executable proof for this guard batch. |

Rejected / deferred:

```text
Reasonix config root or public auto-plan setting
legacy Kun/Reasonix agent-provider fallback
deprecated settings fallback writes
renderer-visible Go route or default Go backend
packaged settings walkthrough
Rust/Tauri rewrite
```

Focused validation:

```text
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0149 Go G5 Context Compaction Boundary Replay

Source:

```text
Reasonix long-thread/currentness review and analytix context compaction
runtime semantics.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Latest compaction boundary for model history | `contract-reimplement` | Use analytix `effectiveHistoryAfterLatestCompaction` semantics as the TS-owned oracle. |
| Go executable replay | `code-port-and-adapt` | Add fixture-only Go replay of effective/dropped ids and boundary flags. |
| Noop/older compaction promotion | `reject` | Do not let `replacedTokens: 0` or stale older compactions become the current model history boundary. |
| Compaction control state in provider cache prefix | `reject` | Keep immutable prefix stable and free of dynamic history-control state. |
| Live Go history manager | `defer` | Keep behind G5/G6 gates. |
| Reasonix controller/session protocol | `reject` | Do not expose upstream protocol, public route names, or renderer-visible Go routes. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.compactionBoundary`. |
| TS conformance | `go-runtime-conformance.test.ts` derives expected output from `effectiveHistoryAfterLatestCompaction`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same effective/dropped id set without becoming a default backend. |

Rejected / deferred:

```text
Reasonix SessionAPI/controller protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop long-history QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0150 Go G5 User-Input Structured Validation Replay

Source:

```text
Reasonix AskTool/user-input validation review and analytix request_user_input
runtime semantics.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Structured choice request validation | `contract-reimplement` | Use analytix `request_user_input` semantics as the TS-owned contract. |
| Go executable replay | `code-port-and-adapt` | Add fixture-only Go replay of invalid case count and reject booleans. |
| Invalid request opening pending gate | `reject` | Invalid structured input returns `invalid_user_input_request` and opens no gate. |
| Reasonix `ask` protocol/name | `reject` | Do not expose upstream ask/session protocol, public routes, CLI names, or product copy. |
| Live Go approval/user-input manager | `defer` | Keep behind G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.userInput.structuredChoiceValidation`. |
| TS conformance | `go-runtime-conformance.test.ts` derives expected output from the G4 structured validation oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes invalid case count and reject booleans without becoming a default backend. |

Rejected / deferred:

```text
Reasonix ask/session protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval/user-input QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0151 Go G5 Planner Gate Matrix Replay

Source:

```text
Reasonix planner/auto-plan gating review and analytix Plan mode tool-policy
runtime semantics.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner enable/disable gate | `contract-reimplement` | Use analytix Plan mode semantics: normal agent mode hides `create_plan`, Plan mode gates tools by step. |
| Blocked-tool rejection matrix | `code-port-and-adapt` | Add fixture-only Go replay for forged `task`, `parallel_tasks`, `bash`, `edit`, `write`, and `echo` rejection. |
| Plan capability gate evidence | `contract-reimplement` | Record capability advertisement separately from model step advertisement so gate tools do not become step-0 model tools. |
| Reasonix planner/session public protocol | `reject` | Do not expose SessionAPI, planner/task protocol, public routes, or Reasonix config names. |
| Public auto-plan setting / live Go planner | `defer` | Future product controls must use top-level `runtime` settings; live Go planner stays behind G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.planner` gate and rejection matrix fields. |
| TS conformance | `go-runtime-conformance.test.ts` derives step output through `resolvePlanModeToolSpecs` and validates the blocked-tool matrix. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes capability output, step output, rejected tool names/count, and product-boundary flags without becoming a default backend. |

Rejected / deferred:

```text
Reasonix planner/session/task public protocol
Reasonix auto-plan config/root or project override
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop Plan QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0152 Go G5 Desktop Bridge/Settings Sovereignty Replay

Source:

```text
Reasonix public protocol/config review, analytix preload bridge and settings
store sovereignty tests.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Single desktop bridge | `contract-reimplement` | Keep only `window.analytix` and `Window.analytix`; reject Kun/Reasonix/deprecated aliases. |
| Reasonix auto-plan settings shapes | `contract-reimplement` | Keep migration/drop behavior: `autoPlan` / `auto_plan` may be read only for cleanup and must not persist. |
| Legacy runtime settings envelopes | `contract-reimplement` | Drop `agentProvider` / `agents` writes and keep endpoint-format persistence under top-level `runtime` / `provider.providers`. |
| Go executable replay | `code-port-and-adapt` | Add fixture-only Go replay for bridge/settings proof ids and boundary booleans. |
| Reasonix public config/protocol | `reject` | Do not expose SessionAPI, config roots, bridge aliases, or settings fallback paths. |
| Live Go desktop integration | `defer` | Keep behind G5/G6 gates and packaged QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.desktopSovereignty`. |
| TS conformance | `go-runtime-conformance.test.ts` reads real preload/window/API/settings sources and derives bridge names, facade domains, and proof ids. |
| Desktop tests | `preload-sandbox.test.ts` and `settings-store.test.ts` remain the live desktop/source authorities. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the desktop sovereignty output without reading desktop files or enabling Go. |

Rejected / deferred:

```text
Reasonix public protocol/config root or auto-plan setting
deprecated bridge alias or settings fallback write path
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop settings QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0101 Reasonix Auto-Plan Config Boundary Guard

Source:

```text
Reasonix `881b2f2f..9ada1417`, especially `01d9b173` user-only
auto-plan config and project/local override rejection.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix user-level `auto_plan` setting | `document-only / defer` | Do not add a product auto-plan setting in this batch. Future controls must be analytix-owned top-level `runtime` settings. |
| Project/local `auto_plan` override rejection | `contract-reimplement` | Drop `agent.auto_plan`, root `autoPlan` / `auto_plan`, and runtime `autoPlan` / `auto_plan` from GUI settings normalization and persistence. |
| Runtime config parser boundary | `contract-reimplement` | Keep `analytix serve` config strict so Reasonix `agent.auto_plan`, `runtime.auto_plan`, and `serve.autoPlan` are rejected. |
| Reasonix CLI/config/controller protocol | `reject` | Do not import `reasonix config auto-plan`, `--local`, project overrides, SessionAPI, or controller rebuild protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Shared settings | `normalizeAppSettings`, `mergeAnalytixRuntimeSettings`, and runtime settings keys ignore Reasonix auto-plan shapes. |
| Settings store | Reload + patch persistence drops Reasonix auto-plan roots instead of writing them to `analytix-settings.json`. |
| Runtime config | `AnalytixConfigSchema` rejects upstream auto-plan config roots. |

Rejected / deferred:

```text
Reasonix public auto-plan setting
project/local auto-plan override
Reasonix controller API or SessionAPI
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop planner/settings QA
```

Focused validation:

```text
npm run test -- src/shared/app-settings.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- src/config/analytix-config.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This proves the runtime gate contract and G4 shadow output. It does not prove
packaged desktop visual behavior, live remote entry behavior, or full Reasonix
ask/user-input parity.
```

## 2026-06-21 - Write-Inline Custom Full Endpoint Proof

Source:

```text
Reasonix provider/cache request-surface discipline from the post-881 parity
stream. No Reasonix provider protocol is imported.
```

Classification:

| Reasonix delta / idea | Classification | analytix decision |
| --- | --- | --- |
| custom provider full endpoint URLs must stay explicit | contract-reimplement | Prove write-inline does not append paths to custom `/responses` or `/messages` URLs. |
| body/header/parser must follow the resolved endpoint suffix | code-port-and-adapt | Custom `/responses` uses Responses body/parser without Anthropic headers; custom `/messages` uses Messages body/parser with Anthropic-style headers. |
| Reasonix provider/cache public protocol | reject | Do not expose Reasonix provider protocol, config roots, controller APIs, or release claims. |
| live provider/cache superiority | defer | Requires credentials/live matrix; this batch is fixture-backed request-surface proof only. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Custom Responses endpoint | Write-inline POSTs to the exact `/responses` full URL and sends `input` / `max_output_tokens`. |
| Custom Messages endpoint | Write-inline POSTs to the exact `/messages` full URL and sends `system` / `messages` / `max_tokens` with Anthropic-style headers. |
| Provider parser proof | The same tests parse Responses `output_text` and Messages `content[].text`. |

Rejected / deferred:

```text
Reasonix provider protocol, credentialed live provider matrix, live cache/cost
superiority, settings schema changes, provider default changes, default Go
backend, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer,
Kun identity, Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is fixture-backed desktop request-surface proof. It does not exercise live
custom provider credentials, provider availability, or cost/cache accounting.
```

## 2026-06-21 - Auto-Model Route Cache Lifecycle Proof

Source:

```text
Reasonix sibling repo: /Users/sun/Projects/DeepSeek-Reasonix
Compared range: 881b2f2f2644d1873c7867f29c64cae7be7d9c24..9ada14176629b1d59d7ed78446951b2bb5954904
Relevant commit: 2db7acf6 fix: rebuild auto-plan classifier on enable
```

Classification:

| Reasonix delta / idea | Classification | analytix decision |
| --- | --- | --- |
| classifier currentness/rebuild should prevent stale route reuse | contract-reimplement | Absorbed as loop-level proof that auto route cache is scoped to one turn and keyed by classifier fingerprint. |
| same-turn route reuse should avoid duplicate classifier calls | code-port-and-adapt | `AgentLoop` test proves a tool-driven second model step reuses the first route. |
| user-level auto-plan setting | document-only / defer | No public auto-plan setting in analytix; future setting would need top-level `runtime` contract. |
| project/local auto-plan override and Reasonix controller API | reject | Do not import Reasonix config root, local/project `auto_plan`, or desktop controller API. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Same-turn route cache | A multi-step `model:"auto"` turn calls `_auto_router` once and reuses the selected model/reasoning route after a tool call. |
| Cross-turn currentness | The next turn in the same thread calls `_auto_router` again. |
| Stable-prefix safety | Classifier/cache state remains in runtime route-cache state and does not enter immutable prefix content. |

Rejected / deferred:

```text
Reasonix public auto-plan config, project/local auto-plan overrides,
Reasonix desktop controller API, full auto-plan parity, default Go backend,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer, Kun identity,
Rust/Tauri rewrite, and release readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto model" --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This proves AgentLoop cache/currentness behavior. It does not add a desktop
auto-plan setting or validate packaged controller behavior.
```

## 2026-06-21 - Provider Request-Shape Oracle Matrix

Source:

```text
Reasonix provider/cache request-surface discipline from the post-881 parity
stream. No Reasonix provider protocol is imported.
```

Classification:

| Reasonix delta / idea | Classification | analytix decision |
| --- | --- | --- |
| provider request URL/header/body shape should be part of cache proof | contract-reimplement | Added `provider-cache-oracle.json.requestShapeCases` and executable `CompatModelClient` assertions. |
| DeepSeek/OpenAI/Anthropic/custom must not regress differently | code-port-and-adapt | Matrix covers DeepSeek official chat, OpenAI-compatible chat, Responses, Anthropic Messages, and custom Responses full endpoint. |
| Go provider client parity | defer | G3/G5 shadow replays request-shape case ids only; no Go provider client. |
| Reasonix provider public protocol | reject | Do not expose Reasonix provider protocol, config roots, controller APIs, or release claims. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Request URL/body/header matrix | Mocked `CompatModelClient` requests are checked against oracle cases. |
| DeepSeek boundary | DeepSeek official chat can carry DeepSeek thinking/reasoning fields; OpenAI-compatible/custom Responses explicitly forbid `thinking`. |
| Anthropic boundary | Messages requests carry Anthropic headers, `system` / `messages` / `max_tokens`, and `input_schema` tools only in Messages mode. |
| Go shadow | G3/G5 output records request-shape case ids without a live Go provider client. |

Rejected / deferred:

```text
Reasonix provider protocol, credentialed live provider matrix, live cache/cost
superiority, settings schema changes, provider default changes, Go provider
client, default Go backend, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer, Kun identity, Rust/Tauri rewrite, and release
readiness.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0071 Auto-Router Request Contract Fingerprint Currentness

Source:

```text
Reasonix 2db7acf6 rebuild auto-plan classifier on enable and analytix
auto-model-router currentness guard.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Classifier request contract drift | `contract-reimplement` | Fingerprint the classifier request contract fields so request-shape drift changes route-cache keys. |
| Rebuild-on-enable stale classifier guard | `code-port-and-adapt` | Adapt the stale-controller guard as analytix `AUTO_MODEL_ROUTER_FINGERPRINT` currentness rather than a Reasonix desktop controller rebuild. |
| Reasonix user/project auto-plan config | `reject` / `document-only` | Do not import `reasonix config auto-plan`, local/project override, or Reasonix settings shape. |
| Live desktop auto-plan toggle parity | `defer` | Future product setting must use top-level `runtime` settings and analytix bridge/runtime contracts. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime helper | `buildAutoModelRouterFingerprint` accepts explicit request-contract inputs while preserving default `AUTO_MODEL_ROUTER_FINGERPRINT`. |
| Runtime test | `auto-model-router.test.ts` proves max-token, temperature, and reasoning-effort drift change the fingerprint. |
| Product boundary | No Reasonix auto-plan config, controller/SessionAPI protocol, bridge/settings fallback, default Go backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Rejected / deferred:

```text
Reasonix config auto-plan
project/local auto-plan override
user-visible auto-plan settings
Reasonix controller API or public protocol
packaged desktop controller QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0072 Connect Phone Copy Sovereignty

Source:

```text
Kun/Reasonix product identity boundary review and sub-agent B product-surface
scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| New Connect Phone prompt/title/schema/log copy | `contract-reimplement` | Use Connect Phone for new user/model/log visible strings. |
| Legacy Claw prompt/title recognition | `code-port-and-adapt` | Keep old headings and `[Claw:]` / `[Claw IM:]` title recognizers for historical sessions. |
| Internal `claw` compatibility names | `document-only` | Keep field/file/function names where they are storage or compatibility contracts. |
| Public Claw/Kun/Reasonix identity | `reject` | Do not expose as product copy, bridge, settings, route, or top-level UI. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Prompt copy | `app-settings-prompts.ts` emits Connect Phone headings and unwraps legacy Claw headings. |
| Runtime copy | `claw-runtime.ts` creates `[Connect Phone:...]` titles and Connect Phone webhook log messages. |
| Schedule copy | `claw-schedule-mcp-server.ts` schema description and scheduled-task comment say Connect Phone. |
| Renderer compatibility | Thread recognizers support new Connect Phone titles plus legacy Claw titles. |

Rejected / deferred:

```text
public Claw identity
Kun/Reasonix product identity
renaming compatibility storage fields
removing legacy Claw-titled thread support
packaged desktop QA
release readiness
```

Focused validation:

```text
npm run test -- src/shared/app-settings.test.ts src/main/schedule-runtime.test.ts src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts src/renderer/src/store/chat-store-helpers.test.ts src/renderer/src/store/chat-store-claw-actions.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0073 Go G5 Durable Runner Restart Executable Shadow

Source:

```text
Reasonix sub-agent/job orchestration review, analytix task-job restart oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Restart-visible durable jobs | `code-port-and-adapt` | Add G5 executable replay for rehydrated running/queued jobs. |
| Internal task-job route semantics | `contract-reimplement` | Bind expected wait/output/kill results to `task-job-orchestration-oracle.json`. |
| Live Go Job Manager/default backend | `defer` / `reject` | Keep Go G5 shadow-only until G5/G6 gates are satisfied. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, sub-agent route names, or top-level product entries. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records `controlExecutableCases.taskJobs.restartDrill`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the expected restart drill from the TS task-job oracle. |
| Go shadow | `shadow_g5.go` computes rehydrated count, combined output, next offset, and queued kill result. |
| Product boundary | No renderer-visible Go route, default Go backend, Reasonix protocol, Subagent top-level entry, Kun identity, Rust/Tauri path, or bridge/settings fallback is added. |

Rejected / deferred:

```text
live Go Job Manager
default Go backend
renderer-visible Go task/job routes
Reasonix SessionAPI or public sub-agent/job protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
packaged desktop QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0126 Go G5 Approval/User-Input Route Replay Control Shadow

Source:

```text
Reasonix approval/user-input lifecycle, step/cancel/cache stability review, and
analytix approval-user-input route oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Approval decision route replay | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix oracle. |
| User-input submit/cancel route replay | code-port-and-adapt | Carry into Go G5 control shadow only, preserving HTTP answer echo and resolved-event answer omission. |
| GUI gate/replay contract | contract-reimplement | Keep `window.analytix -> preload -> main -> analytix runtime HTTP/SSE` and TS runtime authority. |
| Reasonix SessionAPI / ask protocol | reject | Do not expose upstream public protocol or route names. |
| Live Go gate manager / default backend | defer / reject | Keep behind G5/G6 gates and do not make Go default. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.approvalUserInputRouteReplay` uses a minimal TS-owned approval/user-input oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed approval route, submit/cancel route, replay, late-action, and abort-cleanup output. |
| Runtime contract | Existing approval/user-input oracle remains the authority; no Reasonix public protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix SessionAPI or public ask protocol
live Go approval/user-input manager
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval-card QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0130 Go G5 MCP Core Lifecycle Control Shadow

Source:

```text
Reasonix MCP lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP connect/disconnect/reload lifecycle | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix MCP lifecycle oracle. |
| MCP cancel/error lifecycle | contract-reimplement | Preserve cancel-before-start no-execute and approved `tool_execution_failed` shape as analytix runtime contract evidence. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or navigation. |
| Live Go MCP client / credentialed MCP matrix | defer | Keep future work behind G5/G6 and packaged QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpCoreLifecycle` uses a minimal TS-owned core lifecycle oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes connect/disconnect diagnostics, reload schema order, cancel no-execute, and approved error shape. |
| Runtime contract | Existing MCP lifecycle oracle remains the authority; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0133 Go G5 Task-Job Tool Contract Boundary Control Shadow

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| `task` / `parallel_tasks` internal tool contract | contract-reimplement | Carry into `controlExecutableCases.taskJobs.toolContractBoundary`, owned by analytix task-job oracle. |
| Go shadow fixture replay | code-port-and-adapt | Compute tool/route boundary output in Go shadow only. |
| Reasonix public sub-agent/job protocol | reject | Do not expose upstream route names, SessionAPI, top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer surfaces. |
| Live Go Job Manager / packaged renderer QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.toolContractBoundary` records task/parallel tool contracts, runtime routes, protected routes, forbidden top-level routes, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes internal runtime-only flags, permission/evidence/dependency/read-only gates, route auth, unauthorized status, no Reasonix protocol, and no top-level route exposure. |
| Runtime contract | Existing TypeScript task-job runtime remains authoritative; no Reasonix public job protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol or route names
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
renderer-visible Go route or packaged nested-card QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0135 Go G5 Nested Child SSE Metadata Control Shadow

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job child event
oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Nested child SSE metadata | contract-reimplement | Carry into `controlExecutableCases.taskJobs.nestedSseMetadata`, owned by analytix task-job oracle. |
| Go shadow fixture replay | code-port-and-adapt | Compute nested child event metadata output in Go shadow only. |
| Reasonix SessionAPI/public job protocol | reject | Do not expose upstream route names, SessionAPI, top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer surfaces. |
| Live Go Job Manager / packaged renderer QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.nestedSseMetadata` records parent/child ids, metadata fields, evidence ledger keys, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes parent/child distinctness, metadata-key coverage, active-goal requirement, no Reasonix protocol, and no top-level route exposure. |
| Runtime contract | Existing TypeScript task-job runtime and analytix event projection remain authoritative; no Reasonix SessionAPI or public job protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix SessionAPI/public sub-agent/job protocol or route names
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
renderer-visible Go route or packaged nested-card QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0134 Go G5 Task Transcript Identity Control Shadow

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job transcript
oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Transcript continue/fork identity | contract-reimplement | Carry into `controlExecutableCases.taskJobs.transcriptIdentity`, owned by analytix task-job oracle. |
| Go shadow fixture replay | code-port-and-adapt | Compute transcript identity output in Go shadow only. |
| Reasonix SessionAPI/public job protocol | reject | Do not expose upstream route names, SessionAPI, top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer surfaces. |
| Live Go Job Manager / packaged renderer QA | defer | Keep future work behind G5/G6 gates and packaged QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.transcriptIdentity` records source/continue/fork ids, incompatible identity error, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes continue-target-matches-source, fork-target-distinct-from-source, same identity requirement, no Reasonix protocol, and no top-level route exposure. |
| Runtime contract | Existing TypeScript task-job runtime remains authoritative; no Reasonix SessionAPI or public job protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix SessionAPI/public sub-agent/job protocol or route names
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
renderer-visible Go route or packaged nested-card QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0132 Go G5 Product Boundary Control Shadow

Source:

```text
Reasonix Go/runtime boundary review, analytix product sovereignty rules, and
the G5 full-loop oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Go runtime boundary proof | contract-reimplement | Carry product-boundary invariants into `controlExecutableCases.productBoundary`, owned by analytix G5 oracle. |
| Go shadow fixture replay | code-port-and-adapt | Compute the boundary output in Go shadow only. |
| Reasonix public protocol / route names | reject | Do not expose upstream protocol, renderer route, public SessionAPI, MCP-indexer, workflow, or job protocol. |
| Default Go backend / Electron integration | reject / defer | Default backend remains TypeScript; any future Electron integration stays behind G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.productBoundary` records bridge/serve stability and Go disabled state. |
| Go shadow | `BuildG5ControlExecutableOutput` computes no Reasonix protocol, no default Go backend, no renderer-visible Go route, no Electron main connection, and no enabled Go backend. |
| Runtime contract | `window.analytix`, top-level runtime settings, `analytix serve`, and Renderer -> preload -> main -> runtime HTTP/SSE remain the only product-owned path. |

Rejected / deferred:

```text
Reasonix public protocol or route names
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
default Go backend or renderer-visible Go route
Electron integration, packaged desktop QA, G6 readiness, or release readiness
Rust/Tauri rewrite path
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0131 Go G5 MCP Background Reconnect Control Shadow

Source:

```text
Reasonix MCP lifecycle/reconnect review and analytix MCP tool lifecycle oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP background reconnect retry outcome | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix MCP lifecycle oracle. |
| Suspended provider/reason and retry coverage contract | contract-reimplement | Preserve analytix-owned reconnect semantics: failed server ids, connected/error outcomes, attempts per failed server, retry-all-failed coverage, and no runtime restart. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, navigation, or public MCP-indexer surface. |
| Live Go MCP client / credentialed MCP matrix | defer | Keep future work behind G5/G6 and packaged QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpBackgroundReconnect` uses the TS-owned reconnect oracle and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes reconnect outcome fields and compares them against the TS oracle. |
| Runtime contract | Existing MCP lifecycle oracle remains the authority; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0136 Go G5 MCP Known Override Diagnostics Control Shadow

Source:

```text
Reasonix MCP lifecycle/config override review and analytix MCP tool lifecycle
oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP known override diagnostics/config | contract-reimplement | Carry `codegraph` and `codebase-memory` override diagnostics into analytix-owned MCP lifecycle evidence, preserving effective cwd, workspace root, priority, and background-start semantics. |
| Go G5 known-override diagnostic replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned MCP lifecycle oracle and compare it to expected output. |
| Reasonix MCP-indexer public protocol / route names | reject | Do not expose upstream MCP-indexer protocol, top-level route, navigation, or public renderer surface. |
| Live Go MCP client / credentialed MCP matrix / packaged MCP QA | defer | Keep future work behind G5/G6 gates, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpKnownOverrideDiagnostics` uses the TS-owned known-override variants and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes diagnostics rows, override kinds, workspace roots, explicit-cwd and daemon-timeout server ids, low-priority/background-start booleans, and product-boundary flags. |
| Runtime contract | Existing MCP lifecycle oracle remains authoritative; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0137 Go G5 MCP Live-Local Indexer Control Shadow

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP live-local indexer lifecycle | contract-reimplement | Carry retry, tombstone, restart, active path, diagnostic redaction, and execution-error redaction semantics into analytix-owned MCP lifecycle evidence. |
| Go G5 live-local indexer replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned MCP lifecycle oracle and compare it to expected output. |
| Reasonix MCP-indexer public protocol / route names | reject | Do not expose upstream MCP-indexer protocol, top-level route, navigation, or public renderer surface. |
| Live Go MCP client / credentialed MCP matrix / packaged MCP QA | defer | Keep future work behind G5/G6 gates, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpLiveLocalIndexer` uses the TS-owned live-local indexer fixture and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes retry ids/map, initial/resume/active paths, tombstone count, restart status, late tombstone, secret-safe diagnostic, execution-error redaction, and product-boundary flags. |
| Runtime contract | Existing MCP lifecycle oracle remains authoritative; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0138 Go G5 MCP Search Meta-Tool Control Shadow

Source:

```text
Reasonix MCP/indexer currentness review and analytix MCP search meta-tool
oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP search meta-tool advertised/trust/no-execute | contract-reimplement | Carry meta-tool advertised set, refresh tool presence, workspace trust, unknown-tool error, `on-request` policy, and denied no-execute semantics into analytix-owned MCP lifecycle evidence. |
| Go G5 search meta-tool replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned MCP lifecycle oracle and compare it to expected output. |
| Reasonix MCP-indexer public protocol / route names | reject | Do not expose upstream MCP-indexer protocol, top-level route, navigation, or public renderer surface. |
| Live Go MCP client / credentialed MCP matrix / packaged MCP QA | defer | Keep future work behind G5/G6 gates, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpSearchMetaTools` uses the TS-owned search meta-tools fixture and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes meta-tool names/count, refresh advertisement, trusted/untrusted workspace fields, unknown-tool error, `on-request` policy, denied no-execute, and product-boundary flags. |
| Runtime contract | Existing MCP lifecycle oracle remains authoritative; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0139 Go G5 Provider Cache Privacy Control Shadow

Source:

```text
Reasonix provider/cache privacy and live-superiority review, analytix
provider-cache oracle, and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Provider cache diagnostics privacy | contract-reimplement | Carry forbidden diagnostics substring policy into analytix-owned provider/cache evidence and prove diagnostics metadata does not leak stable prefix text, tool schema text, API key text, or `Authorization`. |
| Go G5 provider cache privacy replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned provider-cache oracle and compare it to expected output. |
| Reasonix provider protocol / live superiority claim | reject | Do not expose upstream provider protocol or claim live provider/cache superiority from fixture-only proof. |
| Credentialed provider matrix / packaged provider QA | defer | Keep future work behind G5/G6 gates, live credentials, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.providerCachePrivacy` uses TS-owned provider/cache privacy, diagnostics, live credential policy, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes diagnostics field count, forbidden substring count, no leak, no live credential use, no live superiority claim, and product-boundary flags. |
| Runtime contract | Existing provider/cache oracle remains authoritative; no Reasonix provider protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix provider public protocol
live provider/cache superiority claim
live Go provider client or default Go backend
renderer-visible Go route
credentialed provider matrix or packaged provider QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0140 Go G5 Provider Cache Inventory Control Shadow

Source:

```text
Reasonix provider/cache prefix currentness review, analytix provider-cache
oracle, and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Provider cache prefix/tool inventory | contract-reimplement | Carry stable prefix hash, canonical tools hash, provider usage case ids, and request-shape case ids into analytix-owned provider/cache evidence. |
| Go G5 provider cache inventory replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned provider-cache oracle and compare it to expected output. |
| Reasonix provider protocol / dynamic prefix material | reject | Do not expose upstream provider protocol or move file snippets, timestamps, selected text, credentials, or Reasonix sidecars into stable prefix. |
| Credentialed provider matrix / packaged provider QA | defer | Keep future work behind G5/G6 gates, live credentials, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.providerCacheInventory` uses TS-owned stable prefix, provider usage ids, request-shape ids, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes stable prefix hash, tools hash, usage/request-shape ids and counts, prefix equivalence, tools hash stability, and product-boundary flags. |
| Runtime contract | Existing provider/cache oracle remains authoritative; no Reasonix provider protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix provider public protocol
dynamic context in stable prefix
live Go provider client or default Go backend
renderer-visible Go route
credentialed provider matrix or packaged provider QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0141 Go G5 Session Route Inventory Control Shadow

Source:

```text
Reasonix session/fork/SSE route review, analytix G2 route oracle, and Go G5
executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Thread/session route inventory | contract-reimplement | Carry route id inventory and route-family grouping into analytix-owned runtime HTTP/SSE evidence for list/archive/search/read/update/fork/resume/SSE/auth. |
| Go G5 session route inventory replay | code-port-and-adapt | Add pure Go control shadow computation from the TS-owned G2 route oracle and compare it to expected output. |
| Reasonix SessionAPI / public route protocol | reject | Do not expose upstream SessionAPI, route names, renderer-visible Go routes, or top-level capability navigation. |
| Packaged route QA / live Go HTTP server | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.sessionRouteInventory` uses TS-owned G2 route summaries and expected inventory output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes route ids, JSON/SSE/event/resume/fork/archive/search/read-update groups, runtime-token count, unauthorized ids, and product-boundary flags. |
| Runtime contract | Existing TypeScript route/SSE oracle remains authoritative; no Reasonix SessionAPI enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
packaged desktop thread/fork/resume/archive/search walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0142 Go G5 Approval/User-Input Inventory Control Shadow

Source:

```text
Reasonix approval/user-input lifecycle review, analytix approval/user-input
route oracle, and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Approval/user-input gate inventory | contract-reimplement | Carry approval/user-input gate ids, route kinds, replay kinds, late action statuses, and answer privacy flags into analytix-owned gate/event evidence. |
| Go G5 approval/user-input inventory replay | code-port-and-adapt | Add pure Go control shadow computation from TS-owned oracle-derived input and compare it to expected output. |
| Reasonix ask/session public protocol | reject | Do not expose upstream ask/session protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Packaged approval-card QA / live Go gate manager | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.approvalUserInputInventory` uses TS-owned approval/user-input ids, replay kinds, late statuses, pending counters, and expected output. |
| Go shadow | `BuildG5ControlExecutableOutput` computes gate ids, approval ids, user-input ids, route kinds, replay kind inventory, answer privacy flags, late statuses, pending-after sum, and product-boundary flags. |
| Runtime contract | Existing TypeScript approval/user-input oracle remains authoritative; no Reasonix ask/session protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix ask/session public protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
packaged desktop approval-card/user-input walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0143 Go G5 Task Planner Toolset Inventory Control Shadow

Source:

```text
Reasonix planner/sub-agent job orchestration review, analytix task-job oracle,
and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Planner read-only/forbidden task toolset inventory | contract-reimplement | Carry planner read-only tools, forbidden `task` / `parallel_tasks`, policy values, and no-overlap checks into analytix-owned task-job evidence. |
| Go G5 planner toolset inventory replay | code-port-and-adapt | Add pure Go control shadow computation from TS-owned task-job oracle-derived input and compare it to expected output. |
| Reasonix public sub-agent/job/planner protocol | reject | Do not expose upstream SessionAPI, public job protocol, route names, renderer-visible Go routes, or top-level capability navigation. |
| Packaged sub-agent QA / live Go Job Manager | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.plannerToolsetInventory` uses TS-owned planner read-only toolset, forbidden task toolset, task tool names, and policy values. |
| Go shadow | `BuildG5ControlExecutableOutput` computes read-only and forbidden tool counts, read-only exclusion of task tools, forbidden match against task tools, policy values, and product-boundary flags. |
| Runtime contract | Existing TypeScript task-job oracle remains authoritative; no Reasonix sub-agent/job/planner protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix public sub-agent/job/planner protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
packaged desktop sub-agent/planner walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0148 Go G5 History Repair Pair-Integrity Control Shadow

Source:

```text
Reasonix agent-kernel history legality/cache stability review, analytix
`repairModelHistoryItems`, and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Tool-call/result pair integrity | contract-reimplement | Carry analytix `repairModelHistoryItems` semantics into a TS-owned G5 control case: complete multi-tool blocks survive, orphan results are dropped, missing-result calls are dropped, duplicate results are dropped, and bridge text survives. |
| Go G5 history repair replay | code-port-and-adapt | Add pure Go shadow computation over simplified fixture items and compare it to TS-derived expected output. |
| Dynamic repair state in cache prefix | reject | Do not allow history-repair bookkeeping to enter stable prefix or provider cache keys. |
| Reasonix SessionAPI/controller/public history protocol | reject | Do not expose upstream protocol, route names, renderer-visible Go routes, public auto-plan settings, or top-level capability navigation. |
| Packaged long-history QA / live Go loop | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.historyRepair` stores simplified history items plus expected repaired/dropped ids and product-boundary flags. |
| TS conformance | `go-runtime-conformance.test.ts` derives expected output from `repairModelHistoryItems`, not from hand-written assumptions. |
| Go shadow | `BuildG5ControlExecutableOutput` computes repaired ids, dropped ids, kept call/result call ids, and boundary flags. |
| Runtime contract | Existing TypeScript runtime history repair remains authoritative; no Reasonix public protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix SessionAPI or controller protocol
renderer-visible Go route or default Go backend
dynamic history-repair state in stable prefix/cache key
public auto-plan setting or top-level Workflow/Create Loop/Subagent/AutoResearch route
packaged desktop long-history walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0147 Go G5 Step-Limit Override/Delegate Matrix Control Shadow

Source:

```text
Reasonix agent-kernel step-limit/planner delegation review, analytix G5
full-loop oracle, and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Step-limit override/delegate matrix | contract-reimplement | Carry default/user/session/turn/planner/headless/zero/delegate effective values into analytix-owned G5 control evidence. |
| Go G5 matrix replay | code-port-and-adapt | Add pure Go shadow computation from TS-owned step-limit fixture input and compare the matrix to expected output. |
| Reasonix controller/session protocol or public auto-plan setting | reject | Do not expose upstream controller/session protocol, renderer-visible Go routes, public auto-plan settings, or top-level capability navigation. |
| Packaged step-limit QA / live Go loop | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.stepLimits.expected.matrix` records 9 override/delegate rows, including zero-default disable guard and delegate min-floor. |
| Go shadow | `BuildG5ControlExecutableOutput` computes the matrix from fixture inputs rather than trusting scalar summary fields. |
| Runtime contract | Existing TypeScript step-limit behavior remains authoritative; no Reasonix public protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix controller/session protocol
renderer-visible Go route or default Go backend
public auto-plan setting or top-level Workflow/Create Loop/Subagent/AutoResearch route
packaged desktop step-limit walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

Parallel sub-agent delta classification:

| Reasonix commit / delta | Classification | analytix decision |
| --- | --- | --- |
| `01d9b173 fix: make auto-plan a user-level setting` | document-only / defer / reject / contract-reimplement | Record the negative contract that auto-plan must not be project/local override driven. Do not add `reasonix config auto-plan`, `reasonix.toml [agent] auto_plan`, or a public analytix auto-plan setting in this batch. |
| `2db7acf6 fix: rebuild auto-plan classifier on enable` | contract-reimplement / defer / reject | Absorb the invariant that enabling/disabling planner/classifier behavior must not reuse stale classifier/router/cache state. Do not port Reasonix `desktop/settings_app.go` controller rebuild or SessionAPI. |
| Planner enable/disable public surface | reject / defer / contract-reimplement | Keep analytix-owned explicit planning contracts such as `mode: "plan"`, `guiPlan`, and `create_plan`; do not expose Reasonix planner config or CLI entry. |
| Step/cancel/cache interaction in this range | contract-reimplement / code-port-and-adapt / defer | This Reasonix range has no new step/cancel/cache code delta; continue proving the shared control semantics through analytix G5/G6 oracle and Go shadow evidence. |
| `9ada1417` merge boundary | document-only | Treat as the current-head marker for the classified range, not as a separate absorbable capability. |

## 2026-06-22 - D-0146 Go G5 Combined Step/Cancel/Cache Trace Control Shadow

Source:

```text
Reasonix agent-kernel step/cancel/cache review, analytix G5 full-loop oracle,
and Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Combined auto-route cache, step-limit, and cancel trace | contract-reimplement | Carry router-call counts, model-step limits, stable-prefix flags, cancel result counts, and exact result pairing into analytix-owned G5 control evidence. |
| Go G5 combined trace replay | code-port-and-adapt | Add pure Go shadow computation from TS-owned combined fixture input and compare full trace to expected output. |
| Reasonix controller/session protocol or auto-plan product surface | reject | Do not expose upstream controller/session protocol, renderer-visible Go routes, public auto-plan setting, or top-level Workflow/Create Loop/Subagent/AutoResearch navigation. |
| Packaged long-running cancel/cache QA / live Go agent loop | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.combined.expected` now records router steps, step-limit state, stable-prefix isolation flags, cancel result counts, and exact result rows. |
| Go shadow | `BuildG5ControlExecutableOutput` computes the same trace from the combined fixture rather than trusting summary booleans. |
| Runtime contract | Existing TypeScript loop/cache/cancel behavior remains authoritative; no Reasonix public protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix controller/session protocol
renderer-visible Go route or default Go backend
public auto-plan setting or top-level Workflow/Create Loop/Subagent/AutoResearch route
packaged desktop long-running cancel/cache walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0145 Go G3/G5 Provider Request-Shape Exact Matrix Control Shadow

Source:

```text
Reasonix provider/cache request-shape review, analytix provider-cache oracle,
and Go G3/G5 shadow conformance.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| Provider request-shape exact URL/header/body/tool matrix | contract-reimplement | Carry DeepSeek/OpenAI-compatible/Responses/Anthropic/custom full-endpoint request-shape details into analytix-owned provider/cache evidence. |
| Go G3/G5 exact matrix replay | code-port-and-adapt | Add pure Go shadow computation that clones and compares the TS-owned request-shape matrix in G3 summary, G5 cache replay, and G5 control output. |
| Reasonix provider protocol/settings surface | reject | Do not expose upstream provider protocol, route names, deprecated settings fallback, renderer-visible Go routes, or default Go backend. |
| Credentialed live provider matrix / packaged provider QA | defer | Keep future work behind G5/G6 gates, live credential policy, packaged settings walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Provider oracle | `provider-cache-oracle.json.requestShapeCases` remains the authority for exact URL, header, body-field, reasoning, and tool-shape expectations. |
| G3/G5 fixtures | G3 `expectedOutput.requestShapeSummary`, G5 `shadowSlicesExpectedOutput.cacheReplay.requestShapeReplay`, and G5 `controlExecutableCases.providerRequestShape.expected` all carry the exact matrix. |
| Go shadow | `BuildG3ProviderConformanceOutput`, `BuildG5ShadowSlicesOutput`, and `BuildG5ControlExecutableOutput` replay the matrix without a live Go provider client. |

Rejected / deferred:

```text
Reasonix provider protocol or public route names
renderer-visible Go route or default Go backend
deprecated provider/settings fallback
credentialed live provider matrix or live superiority claim
packaged desktop provider settings walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0144 Go G5 MCP Core Lifecycle Boundary Seal Control Shadow

Source:

```text
Reasonix MCP/indexer lifecycle review, analytix MCP tool lifecycle oracle, and
Go G5 executable control shadow.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP core lifecycle provider/boundary seal | contract-reimplement | Carry `mcp:research` provider identity and no Reasonix protocol/top-level route flags into analytix-owned MCP lifecycle control evidence. |
| Go G5 MCP lifecycle boundary replay | code-port-and-adapt | Add pure Go control shadow computation from TS-owned MCP oracle-derived input and compare provider/boundary output to expected values. |
| Reasonix MCP-indexer public protocol/top-level route | reject | Do not expose upstream MCP-indexer protocol, route names, renderer-visible Go routes, or top-level MCP-indexer navigation. |
| Credentialed/live MCP matrix / packaged MCP QA | defer | Keep future work behind G5/G6 gates, packaged desktop walkthroughs, rollback evidence, and release QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpCoreLifecycle` now records `providerId: "mcp:research"` and explicit product-boundary flags. |
| Go shadow | `BuildG5ControlExecutableOutput` computes provider identity, no Reasonix protocol, and no top-level route exposure alongside connect/disconnect/reload/cancel/error fields. |
| Runtime contract | Existing TypeScript MCP oracle remains authoritative; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
renderer-visible Go route or default Go backend
top-level MCP-indexer route/navigation
credentialed live MCP client matrix
packaged desktop MCP walkthrough
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0129 Go G5 MCP Search Workspace Boundary Control Shadow

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP trusted/untrusted workspace search boundary | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix MCP lifecycle oracle. |
| Unknown-tool and denied-call no-execute | contract-reimplement | Keep analytix internal meta-tools authoritative; untrusted search count remains `0`, unknown tool errors do not execute, and `mcp_call` stays `on-request`. |
| Reasonix MCP-indexer public protocol | reject | Do not expose upstream protocol, route names, or navigation. |
| Live Go MCP client / credentialed MCP matrix | defer | Keep future work behind G5/G6 and packaged QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpSearchWorkspaceBoundary` uses a minimal TS-owned workspace-boundary oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes trusted/untrusted workspaces, query, trusted tool id, untrusted searched count, unknown-tool error, call policy, and denied no-execute. |
| Runtime contract | Existing MCP lifecycle oracle remains the authority; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0128 Go G5 MCP Approval Annotation Control Shadow

Source:

```text
Reasonix MCP/tool approval lifecycle review and analytix MCP tool lifecycle
oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP destructive/open-world approval annotations | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix MCP lifecycle oracle. |
| Denied MCP no-execute proof | contract-reimplement | Keep analytix approval gate authoritative; deny returns approval result and does not execute MCP client. |
| Reasonix MCP-indexer/approval public protocol | reject | Do not expose upstream protocol, route names, or navigation. |
| Live Go MCP client / Go approval manager | defer | Keep future work behind G5/G6 and packaged QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpApprovalAnnotations` uses a minimal TS-owned approval annotation oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes destructive/open-world hints, normalized tool name, approval id, deny decision, approval result kind, and denied no-execute. |
| Runtime contract | Existing MCP lifecycle oracle remains the authority; no Reasonix MCP-indexer or approval protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer or approval public protocol
top-level MCP-indexer route or navigation
live Go MCP client, live Go approval manager, or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP/approval QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0127 Go G5 MCP Search Refresh Drift Control Shadow

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Reasonix delta / capability | Classification | analytix decision |
| --- | --- | --- |
| MCP catalog refresh drift | code-port-and-adapt | Carry into Go G5 control shadow only, computed from analytix MCP lifecycle oracle. |
| MCP search/describe/call/refresh catalog boundary | contract-reimplement | Keep as internal analytix meta-tool evidence; no top-level route or public indexer protocol. |
| Reasonix MCP-indexer public lifecycle | reject | Do not expose upstream protocol, route names, or navigation. |
| Live Go MCP client / credentialed MCP matrix | defer | Keep future work behind G5/G6 and packaged QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 control fixture | `controlExecutableCases.mcpSearchRefreshDrift` uses a minimal TS-owned refresh drift oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes initial tools, expanded tools, indexed count, catalog drift, and no top-level route. |
| Runtime contract | Existing MCP lifecycle oracle remains the authority; no Reasonix MCP-indexer protocol enters renderer/preload/main/runtime contracts. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
renderer-visible Go route
credentialed MCP matrix or packaged MCP QA
Go G5 runtime parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0105 Write Sidebar and Connect Phone Placeholder Sovereignty

Source:

```text
Product-sovereignty follow-up from upstream absorption gates.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public route/protocol expansion | `reject` | Strengthen scans around Write/sidebar entry surfaces and Reasonix bridge/token variants. |
| Connect Phone placeholder copy | `contract-reimplement` | Use analytix-owned `[Connect Phone:...]` copy for newly generated placeholders. |
| Legacy `[Claw:]` recovery | `code-port-and-adapt` | Keep recognizers only for historical compatibility. |

Evidence:

```text
npm run test -- src/renderer/src/store/chat-store-claw-actions.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

Boundary:

```text
No Reasonix public protocol, SessionAPI, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer entry, default Go backend, renderer-visible
Go route, Kun identity, deprecated bridge/settings fallback, Rust/Tauri path,
packaged desktop QA, or release readiness is added.
```

## 2026-06-22 - D-0106 HTTP Auto-Plan Payload Boundary Guard

Source:

```text
Reasonix post-881 auto-plan enable/disable review, especially public
`autoPlan` / `auto_plan` style config and controller-triggered plan mode.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix `autoPlan` / `auto_plan` HTTP payload shape | `reject` | Add a negative HTTP fixture proving these fields do not enter the analytix turn contract. |
| Explicit analytix Plan mode | `contract-reimplement` | Only `mode: "plan"` / `guiPlan` can advertise `create_plan`. |
| User/project auto-plan setting | `document-only` / `defer` | Keep deferred unless a future product spec adds it under top-level `runtime` and `window.analytix`. |
| Reasonix controller rebuild / SessionAPI | `reject` | Do not expose upstream public protocol or local override semantics. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| HTTP contract | `http-server.test.ts` sends `autoPlan` and `auto_plan` to `/v1/threads/:id/turns`. |
| Runtime/loop proof | The captured turn stays agent-mode with no `guiPlan`, and the model request does not advertise `create_plan`. |
| Harness | `http-server-test-harness.ts` supports injected capture models for route-level assertions. |

Rejected / deferred:

```text
Reasonix auto-plan public setting
Reasonix project/local auto-plan override
Reasonix controller rebuild protocol or SessionAPI
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
default Go backend or renderer-visible Go route
packaged desktop Plan mode QA or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts -t "auto-plan payload" --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0107 Plan Step Cancel/Cache Boundary Guard

Source:

```text
Reasonix post-881 planner gating and step/cancel/cache stability review,
mapped onto analytix AgentLoop Plan mode and provider cache diagnostics.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner step gating | `contract-reimplement` | Explicit analytix Plan mode keeps step 0 as read-only tools plus `create_plan`, then narrows the unsatisfied follow-up step to `create_plan`. |
| Cancelled follow-up cache baseline | `contract-reimplement` | Interrupting a follow-up Plan step without usage does not replace the prior cache prefix baseline. |
| DeepSeek cache accounting during planner flow | `contract-reimplement` | Usage events preserve provider-native hit/miss/rate telemetry and sanitized provider/endpoint/model diagnostics. |
| Reasonix auto-plan public protocol | `reject` | Do not import auto-plan config fields, controller rebuild protocol, SessionAPI, or public planner route. |
| Packaged Plan mode QA / live provider matrix | `defer` | Keep deterministic local loop proof in this batch. |
| Go runtime execution | `defer` | No Go runtime change; TS loop remains authoritative until G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime test | `loop.test.ts` now runs a real AgentLoop Plan turn, interrupts the narrowed follow-up step, and starts another Plan turn. |
| Step proof | Captured requests show first-step tools include `ls` and `create_plan`, while the aborted second request advertises only `create_plan`. |
| Cache proof | Two usage events both report `cacheDiagnostics.prefixChanged: false`; the aborted request had no usage and therefore did not advance `cachePrefixShapes`. |
| Provider proof | The fake DeepSeek-compatible client records provider `deepseek`, endpoint format `chat_completions`, model `plan-cache-cancel`, cache hit `80`, miss `20`, and rate `0.8`. |

Rejected / deferred:

```text
Reasonix public auto-plan setting or controller protocol
Reasonix SessionAPI or public planner route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live provider/cache superiority claim
packaged desktop Plan mode QA
Go default backend or renderer-visible Go route
Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0108 MCP Stdio Execution-Error Redaction

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix executable stdio MCP fake
indexer proof.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP `isError` payload redaction | `contract-reimplement` | Redact nested MCP result content before it enters model-visible `tool_result.output`. |
| MCP thrown error redaction | `contract-reimplement` | Rethrow non-abort MCP failures with redacted messages so `LocalToolHost` cannot persist raw secrets. |
| Executable stdio indexer failure proof | `code-port-and-adapt` | Extend the fake indexer fixture with `mcp_codegraph_index_fail_diagnostic` execution evidence. |
| Reasonix MCP-indexer public protocol | `reject` | Do not expose upstream route, SessionAPI, public lifecycle protocol, or top-level MCP-indexer navigation. |
| Credentialed MCP matrix / packaged QA | `defer` | Keep deterministic local stdio proof in this batch. |
| Go runtime execution | `defer` | No Go runtime change; TS MCP provider remains authoritative. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime source | `mcp-tool-provider.ts` redacts both thrown errors and returned MCP result payloads. |
| Oracle fixture | `mcp-tool-lifecycle-oracle.json` records `secretSafeError: "Authorization=<redacted> fake indexer diagnostic"` and `leaksSecret:false`. |
| Runtime test | `mcp-tool-lifecycle-oracle.test.ts` proves the executable stdio fake indexer returns a model-visible `tool_result` without `Bearer lifecycle-secret`. |

Rejected / deferred:

```text
Reasonix MCP-indexer public lifecycle protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
credentialed third-party MCP matrix
packaged desktop MCP QA
Kun identity, deprecated bridge/settings fallback, Rust/Tauri migration
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0109 Workflow/Create Loop Quarantine Scan

Source:

```text
Kun/Reasonix product sovereignty review and forbidden-surface scan gaps.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Dormant workflow/create-loop quarantine | `contract-reimplement` | Add test and scan evidence that dormant code is not imported by top-level entry surfaces. |
| Existing isolated workflow runtime code | `document-only` / `defer` | Keep dormant and unmounted; no product route is added. |
| Reasonix public workflow/create-loop protocol | `reject` | Do not expose upstream route names, SessionAPI, or workflow public protocol. |
| Kun-absent top-level entries | `reject for current surface` | Do not add Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation without a future analytix-native spec, UX rationale, route-surface tests, desktop QA, and scan update. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Route-surface test | `Workbench.route-surface.test.ts` verifies quarantined workflow symbols are absent from entry surfaces. |
| Scan gate | `scan-product-sovereignty.cjs` repeats the quarantine check for release evidence. |
| Product boundary | No renderer route, preload API, tray/menu shortcut, or hidden marketplace entry is added. |

Focused validation:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - D-0103 Product Sovereignty Scan Engineering

Source:

```text
Reasonix/Kun absorption QA review and repeated release-evidence forbidden
surface scans.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Product-sovereignty forbidden scans | `contract-reimplement` | Add `npm run scan:product-sovereignty` to guard top-level entries, identity leaks, deprecated bridges/settings fallback, default Go/Rust/Tauri enabling, and Connect Phone copy. |
| Plugin Marketplace route-surface coverage | `contract-reimplement` | Include `PluginMarketplaceView.tsx` in the route-surface forbidden-token test and scan path. |
| Reasonix/Kun public protocol or product identity | `reject` | Keep scans negative; do not expose upstream identity, SessionAPI, or public route names. |
| Live feature parity claim | `defer` | This is QA evidence, not a new runtime/product capability. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Script | `scripts/scan-product-sovereignty.cjs` wraps the release-evidence forbidden scans. |
| Package command | `package.json` exposes `scan:product-sovereignty`. |
| Renderer test | `Workbench.route-surface.test.ts` now scans Plugin Marketplace as part of the top-level route surface. |

Rejected / deferred:

```text
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
window.kun / Reasonix identity or protocol exposure
deprecated bridge/settings fallback
default Go backend or Rust/Tauri activation
release readiness claim from scan alone
```

Focused validation:

```text
npm run scan:product-sovereignty
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0093 G5 MCP Approval Annotation Replay

Source:

```text
Reasonix MCP/tool approval lessons and analytix MCP lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Destructive/open-world MCP approval metadata | code-port-and-adapt | Add `mcpReplay.approvalAnnotations` computed from the TS MCP lifecycle oracle. |
| Denied approval no-execute | contract-reimplement | Replay `executed:false` and `deniedNoExecute:true` for high-risk MCP tools. |
| Live Go approval manager / MCP client | reject | Go remains shadow-only and does not own live approvals or MCP execution. |
| Credentialed MCP matrix | defer | Keep real MCP server QA outside this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 replay | Server id, tool name, normalized tool name, destructive/open-world flags, approval id, deny decision, approval result kind, and no-execute are pinned. |
| Go shadow | `BuildG5ShadowSlicesOutput` emits the same approval annotation summary from `mcp-tool-lifecycle-oracle.json`. |
| Boundary | No live Go approval manager, live Go MCP client, MCP route, Reasonix protocol, Go backend, or renderer surface changed. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer navigation
live Go approval manager, live Go MCP client, or default Go backend
credentialed MCP server matrix
packaged MCP lifecycle QA
Kun identity or deprecated bridge/settings fallback
Rust/Tauri migration
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0125 Provider Drift Attribution Control Shadow

Source:

```text
Reasonix provider/cache parity review, provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider drift attribution replay | `code-port-and-adapt` | Promote existing drift attribution summary into `controlExecutableCases.providerDriftAttribution`. |
| Stable prefix drift contract | `contract-reimplement` | Keep drift expectations in analytix-owned provider-cache fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live cache superiority matrix | `defer` | No credentialed provider cache superiority matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerDriftAttribution`. |
| TS conformance | `go-runtime-conformance.test.ts` derives drift attribution expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider drift attribution output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider cache superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0124 Provider Live-Local HTTP Control Shadow

Source:

```text
Reasonix provider/cache parity review, provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider live-local HTTP proof replay | `code-port-and-adapt` | Promote existing live-local HTTP summary into `controlExecutableCases.providerLiveLocalHttpProof`. |
| Provider URL/body family coverage | `contract-reimplement` | Keep local HTTP expectations in analytix-owned provider-cache fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live credential matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom provider matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerLiveLocalHttpProof`. |
| TS conformance | `go-runtime-conformance.test.ts` derives live-local HTTP expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider live-local HTTP output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0123 Session Route Status Control Shadow

Source:

```text
Reasonix session/fork/SSE replay review, analytix G2 route oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Thread/session route status replay | `code-port-and-adapt` | Promote existing route status summary into `controlExecutableCases.sessionRouteStatus`. |
| Route/SSE/auth contract | `contract-reimplement` | Keep route expectations in analytix-owned G2 route oracle fixtures. |
| Reasonix SessionAPI/job protocol | `reject` | Do not expose upstream session/job route names or public orchestration protocol. |
| Live Go HTTP routes/default backend | `defer` / `reject` | No renderer-visible Go route or default Go backend is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.sessionRouteStatus`. |
| TS conformance | `go-runtime-conformance.test.ts` derives route status expected output from `go-g2-route-replay-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed session route status output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix SessionAPI or public job/session protocol
live Go thread/session routes
renderer-visible Go route
default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0112 Runtime Settings Legacy-Agent Pollution Guard

Source:

```text
Reasonix auto-plan/config review and analytix active settings schema.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime-contained Reasonix agent shape | `reject` | Strip `agent`, `agentProvider`, `agents`, `deepseek`, and `reasonix` when they appear under active `runtime`. |
| IPC settings patch pollution | `contract-reimplement` | Reject runtime patches containing legacy/Reasonix agent-shaped keys. |
| Explicit top-level runtime settings | `contract-reimplement` | Continue to save only analytix-owned `runtime` settings fields. |
| Reasonix project/local config parity | `defer` / `reject` | Future UI controls must be analytix-owned, not upstream config roots. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime normalizer | `stripReasonixAutoPlanRuntimeFields` now removes runtime-contained legacy agent shapes before merge/migration spread. |
| Shared test | `app-settings.test.ts` proves normalized runtime has no rejected fields and cache key is unchanged. |
| IPC test | `app-ipc-schemas.test.ts` proves active settings patches reject legacy agent-shaped runtime keys. |

Rejected / deferred:

```text
Reasonix auto-plan config roots
Reasonix public settings protocol
Kun/DeepSeek legacy agent settings envelope
deprecated bridge/settings fallback
packaged settings QA and release readiness
```

Focused validation:

```text
npm run test -- src/shared/app-settings.test.ts -t "Reasonix auto-plan" --no-file-parallelism --maxWorkers=1
npm run test -- src/main/ipc/app-ipc-schemas.test.ts -t "legacy.*settings|agent-shaped" --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0113 Task-Job Route Executable Control Shadow

Source:

```text
Reasonix task/job route lifecycle review, existing analytix task-job route
oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Task-job route executable status replay | `code-port-and-adapt` | Promote the existing route executable summary into `controlExecutableCases.taskJobs.routeExecutable`. |
| Internal task-job route contract | `contract-reimplement` | Keep unauthorized/output/wait/kill/missing/rehydrated route behavior in analytix-owned fixtures. |
| Reasonix public SessionAPI/job protocol | `reject` | Do not expose public Reasonix routes or protocol names. |
| Live Go Job Manager/default backend | `defer` / `reject` | Continue G5 shadow-only; no renderer-visible Go route. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.taskJobs.routeExecutable`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the control case from `task-job-orchestration-oracle.routeExecutable`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed route executable output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix SessionAPI/job protocol
live Go Job Manager
renderer-visible Go route
default Go backend
top-level Subagent/Workflow/Create Loop/AutoResearch entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0114 Task-Job Lifecycle Control Shadow

Source:

```text
Reasonix task/job lifecycle review, existing analytix task-job oracle, and Go
G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Task-job foreground/background lifecycle replay | `code-port-and-adapt` | Promote existing lifecycle summary into `controlExecutableCases.taskJobs.lifecycle`. |
| Internal task-job lifecycle contract | `contract-reimplement` | Keep foreground/background/wait-output-kill behavior in analytix-owned fixtures. |
| Reasonix public lifecycle/session protocol | `reject` | Do not expose public Reasonix routes or protocol names. |
| Live Go Job Manager/default backend | `defer` / `reject` | Continue G5 shadow-only; no renderer-visible Go route. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.taskJobs.lifecycle`. |
| TS conformance | `go-runtime-conformance.test.ts` derives lifecycle expected output from the task-job oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed lifecycle output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix SessionAPI/job lifecycle protocol
live Go Job Manager
renderer-visible Go route
default Go backend
top-level Subagent/Workflow/Create Loop/AutoResearch entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0115 Planner-Executor Control Shadow

Source:

```text
Reasonix planner/executor task orchestration review, existing analytix
task-job oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner/executor failure and cancellation replay | `code-port-and-adapt` | Promote existing planner/executor summary into `controlExecutableCases.taskJobs.plannerExecutor`. |
| Internal transcript/output-offset propagation | `contract-reimplement` | Keep propagation behavior in analytix-owned fixtures. |
| Reasonix public planner/job protocol | `reject` | Do not expose public Reasonix routes or protocol names. |
| Live Go Job Manager/default backend | `defer` / `reject` | Continue G5 shadow-only; no renderer-visible Go route. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.taskJobs.plannerExecutor`. |
| TS conformance | `go-runtime-conformance.test.ts` derives planner/executor expected output from the task-job oracle. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed planner/executor output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix SessionAPI/planner/job protocol
live Go Job Manager
renderer-visible Go route
default Go backend
top-level Subagent/Workflow/Create Loop/AutoResearch entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0116 Provider Cache Release Guard Control Shadow

Source:

```text
Reasonix provider/cache parity review, existing analytix provider/cache oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider cache release-guard replay | `code-port-and-adapt` | Promote existing release-guard summary into `controlExecutableCases.providerCacheReleaseGuard`. |
| Offline cache-hit tail/collapse contract | `contract-reimplement` | Keep thresholds and expected output in analytix-owned fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider superiority matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerCacheReleaseGuard`. |
| TS conformance | `go-runtime-conformance.test.ts` derives release-guard expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed release-guard output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0117 Provider Usage Parser Control Shadow

Source:

```text
Reasonix provider/cache parity review, existing analytix provider/cache oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider usage parser precedence replay | `code-port-and-adapt` | Promote existing usage parser summary into `controlExecutableCases.providerUsageParser`. |
| DeepSeek/OpenAI Responses/Anthropic usage contract | `contract-reimplement` | Keep parser expectations in analytix-owned fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerUsageParser`. |
| TS conformance | `go-runtime-conformance.test.ts` derives usage parser expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider usage parser output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0118 Provider Request Shape Control Shadow

Source:

```text
Reasonix provider/cache parity review, existing analytix provider/cache oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider request-shape replay | `code-port-and-adapt` | Promote existing request-shape summary into `controlExecutableCases.providerRequestShape`. |
| Provider URL/body/tool-shape contract | `contract-reimplement` | Keep request expectations in analytix-owned fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerRequestShape`. |
| TS conformance | `go-runtime-conformance.test.ts` derives request-shape expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider request-shape output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0119 Provider Cache Accounting Control Shadow

Source:

```text
Reasonix provider/cache parity review, existing analytix provider/cache oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider cache accounting replay | `code-port-and-adapt` | Promote existing cache accounting summary into `controlExecutableCases.providerCacheAccounting`. |
| DeepSeek/OpenAI Responses/Anthropic cache accounting contract | `contract-reimplement` | Keep accounting expectations in analytix-owned fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerCacheAccounting`. |
| TS conformance | `go-runtime-conformance.test.ts` derives cache accounting expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider cache accounting output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0120 Release/Package Identity Sovereignty Guard

Source:

```text
Reasonix/Kun product-sovereignty review and current release/package
configuration.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Release/package identity scan | `contract-reimplement` | Extend analytix-owned scan coverage to package manifests, builder config, scripts, workflows, and build configuration. |
| Packaging identity assertions | `contract-reimplement` | Assert root package, app id, product name, artifact template, NSIS names, and runtime CLI bin are analytix. |
| Reasonix/Kun package identity | `reject` | Do not allow upstream identity to become public package/release identity. |
| Packaged release smoke | `defer` | Keep packaged artifact QA separate from source/config guard proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Scan | `scan-product-sovereignty.cjs` checks release/package identity surfaces. |
| Test | `packaging-config.test.ts` directly asserts analytix release identity and runtime bin. |

Rejected / deferred:

```text
Kun/Reasonix package identity
deprecated bridge/settings fallback
default Go backend or renderer-visible Go route
Rust/Tauri migration
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged Electron smoke and release readiness
```

Focused validation:

```text
npm run scan:product-sovereignty
npm run test -- src/main/packaging-config.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0121 Provider Streaming Control Shadow

Source:

```text
Reasonix provider/cache parity review, G3 provider streaming oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider streaming usage replay | `code-port-and-adapt` | Promote existing provider streaming summary into `controlExecutableCases.providerStreaming`. |
| SSE usage/cache event contract | `contract-reimplement` | Keep streaming expectations in analytix-owned G3 provider oracle fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider streaming matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom streaming matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerStreaming`. |
| TS conformance | `go-runtime-conformance.test.ts` derives streaming expected output from `go-g3-provider-streaming-usage-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed provider streaming output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider streaming superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0122 Provider Offline Parity Seal Control Shadow

Source:

```text
Reasonix provider/cache parity review, provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider offline parity seal replay | `code-port-and-adapt` | Promote existing offline parity summary into `controlExecutableCases.providerOfflineParitySeal`. |
| Stable prefix/cache telemetry contract | `contract-reimplement` | Keep prefix/cache expectations in analytix-owned provider-cache fixtures. |
| Reasonix public provider/cache protocol | `reject` | Do not expose upstream provider protocol, route names, or settings shape. |
| Live provider superiority matrix | `defer` | No credentialed DeepSeek/OpenAI/Anthropic/custom superiority matrix is claimed by this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` now stores `controlExecutableCases.providerOfflineParitySeal`. |
| TS conformance | `go-runtime-conformance.test.ts` derives offline parity expected output from `provider-cache-oracle.json`. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed offline parity output and `shadow_test.go` compares it to expected. |

Rejected / deferred:

```text
Reasonix provider/cache public protocol
live Go provider client
renderer-visible Go route
default Go backend
credentialed live provider superiority claims
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0096 G5 Task Tool Contract Boundary Replay

Source:

```text
Reasonix task/sub-agent tool contract review, analytix task-job oracle, and Go
G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Task/parallel tool contract replay | `code-port-and-adapt` | Add G5 `jobReplay.toolContractBoundary` for `task` and `parallel_tasks` field names and safety flags. |
| Internal runtime and gate evidence | `contract-reimplement` | Preserve `internalRuntimeOnly`, permission gate, dependency validation, and planner read-only requirements from the TS task-job oracle. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, top-level Subagent/Workflow navigation, or upstream task protocol. |
| Live Go Job Manager | `defer` | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for task/parallel tool-contract fields. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.toolContractBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the boundary summary from task/parallel tool-contract source fields. |

Rejected / deferred:

```text
Reasonix public task/sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
packaged desktop task/sub-agent QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0095 G5 Planner-Executor Detail Replay

Source:

```text
Reasonix planner/sub-agent job orchestration review, analytix task-job oracle,
and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner-executor detail replay | `code-port-and-adapt` | Extend G5 `jobReplay.plannerExecutor` with skipped/cancel reasons and output/transcript job counts. |
| Dependency failure and cancellation propagation evidence | `contract-reimplement` | Preserve analytix-owned `dependency failed: b` and `planner executor cancelled` semantics from the TS task-job oracle. |
| Reasonix planner/sub-agent public protocol | `reject` | Do not expose SessionAPI, public job routes, top-level Subagent/Workflow navigation, or upstream task protocol. |
| Live Go Job Manager | `defer` | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for planner-executor reasons and job counts. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require the extra planner-executor detail fields. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the fields from `TaskPlannerExecutorOracle`. |

Rejected / deferred:

```text
Reasonix public planner/sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
packaged desktop planner/sub-agent QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0092 G5 MCP Live-Local Indexer Summary Replay

Source:

```text
Reasonix MCP/indexer lifecycle lessons and analytix MCP lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP live-local indexer replay summary | code-port-and-adapt | Add nested `mcpReplay.liveLocalIndexer` computed from the TS MCP lifecycle oracle. |
| Secret-safe diagnostics replay | contract-reimplement | Replay redacted diagnostic and `leaksSecret:false` alongside tombstone/restart evidence. |
| Live Go MCP client or MCP-indexer route | reject | Go remains shadow-only and renderer never sees a Go MCP route. |
| Credentialed MCP matrix | defer | Keep real MCP server QA outside this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 replay | Server id, cwd, low-priority/background-start, retry attempts, active paths, tombstone count, snapshot restart, late tombstone, redacted diagnostic, no secret leak, and no top-level route are now pinned. |
| Go shadow | `BuildG5ShadowSlicesOutput` emits the same nested summary from `mcp-tool-lifecycle-oracle.json`. |
| Boundary | No live Go MCP client, MCP route, Reasonix MCP-indexer protocol, Go backend, or renderer surface changed. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer navigation
live Go MCP client or default Go backend
credentialed MCP server matrix
packaged MCP lifecycle QA
Kun identity or deprecated bridge/settings fallback
Rust/Tauri migration
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0091 G5 Provider Live-Local Proof Summary Replay

Source:

```text
Reasonix provider/cache lessons and analytix D-0090 provider live-local HTTP
proof.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Live-local proof metadata | contract-reimplement | Add `provider-cache-oracle.liveLocalHttpProof` so proof scope is explicit and count-checked. |
| G5 cache replay summary | code-port-and-adapt | Add `cacheReplay.liveLocalHttpProof` computed from TS-owned provider/cache oracle data. |
| Go provider HTTP execution | reject | Go does not make provider HTTP requests or become the provider runtime. |
| Credentialed provider matrix | defer | Keep real credentials and external cache behavior outside this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Oracle metadata | Fixture-only local HTTP transport, no live credentials, original provider base URL preservation, 5 usage cases, 7 request-shape cases, and 12 expected POSTs are pinned. |
| G5 shadow | Go computes the same counts and endpoint formats from the TS oracle. |
| Boundary | No provider behavior, settings, bridge, runtime route, Go backend, or renderer surface changed. |

Rejected / deferred:

```text
Reasonix provider public protocol
credentialed live provider/cache matrix
external load testing
live provider/cache superiority
live Go provider client or default Go backend
packaged provider settings QA
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0090 Provider Live-Local HTTP Executable Proof

Source:

```text
Reasonix provider/cache lessons and analytix provider-cache oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider usage executable proof | contract-reimplement | Execute all provider usage cases against a local HTTP provider and validate mapped usage. |
| Request URL/header/body executable proof | contract-reimplement | Execute all request-shape cases against local HTTP while preserving original provider URL/host request construction. |
| DeepSeek host-derived thinking / cache telemetry | code-port-and-adapt | Keep original provider base URL in the client and forward transport to local HTTP so host-specific shaping remains visible. |
| Credentialed provider matrix | defer | Keep real external provider testing outside this no-credential proof. |
| Reasonix provider protocol / live superiority | reject | Do not expose Reasonix protocol or claim live superiority. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime proof | `provider-cache-proof.test.ts` covers 5 usage cases and 7 request-shape cases through local HTTP. |
| Provider surface | DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full endpoint families are all included. |
| Boundary | No provider client behavior, settings, bridge, runtime route, Go backend, or renderer surface changed. |

Rejected / deferred:

```text
Reasonix provider public protocol
credentialed live provider/cache matrix
external load testing
live provider/cache superiority
live Go provider client or default Go backend
packaged provider settings QA
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is fixture/local request-shape proof. It does not exercise live provider
credentials, provider availability, or cost/cache accounting.
```

## 2026-06-21 - Go G5 Planner-Forbidden Shadow Replay

Source:

```text
Reasonix sibling repo: /Users/sun/Projects/DeepSeek-Reasonix
Compared range remains: 881b2f2f2644d1873c7867f29c64cae7be7d9c24..9ada14176629b1d59d7ed78446951b2bb5954904
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Internal task tools must not run during planning | `contract-reimplement` | Kept as analytix Plan mode active-tool policy and TS task-job oracle. |
| Cross-backend planner-forbidden proof | `code-port-and-adapt` | Go G5 shadow now consumes `plannerForbiddenToolset` and emits it in `jobReplay`. |
| Reasonix public task/planner protocol | `reject` | No SessionAPI, task cards, Workflow/Subagent navigation, or controller protocol is copied. |
| Live Go planner/executor parity | `defer` | Requires G5/G6 live runtime, rollback, packaging, and Electron evidence first. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Go G5 shadow job replay | `packages/runtime-go/shadow_g5.go` reads the same forbidden task-tool list as the TS oracle. |
| Drift guard | `go-g5-full-loop-oracle.json`, schema, and TS test bind expected output to the TS task-job oracle. |

Rejected / deferred:

```text
Reasonix public planner/task protocol, full Reasonix planner/executor parity,
live Go runtime behavior, default Go backend, renderer-visible Go route,
Electron-to-Go routing, live provider/cache superiority, live MCP/indexer
parity, release readiness, Kun identity, deprecated bridge/settings fallback,
and Rust/Tauri rewrite.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Write-Inline Provider Request-Surface Proof

Source:

```text
Reasonix provider/cache discipline remains a code-level input; no new
Reasonix provider commits were copied in this batch.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Endpoint-format-specific request shapes | `contract-reimplement` | Prove analytix write-inline keeps Responses and Messages URL/body/header/parser behavior separate. |
| Provider discipline beyond runtime loop | `code-port-and-adapt` | Apply the Reasonix provider/cache review lens to desktop write-inline, not only runtime model-client tests. |
| Live provider/cache superiority | `defer` | Requires credentialed DeepSeek/OpenAI/Anthropic/custom matrix and cost reconciliation. |
| Reasonix provider public protocol | `reject` | No Reasonix SessionAPI/controller/provider protocol copied. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Responses write-inline proof | Test locks `/v1/responses`, `input`, `max_output_tokens`, and no Anthropic header leakage. |
| Messages write-inline proof | Test locks `/v1/messages`, `system` / `messages` / `max_tokens`, Anthropic-style headers, and parser output. |

Rejected / deferred:

```text
Live provider/cache superiority, credentialed provider matrix, cost
reconciliation, Reasonix provider protocol, default Go backend,
renderer-visible Go route, Electron-to-Go routing, release readiness, Kun
identity, deprecated bridge/settings fallback, and Rust/Tauri rewrite.
```

Validation:

```text
npm run test -- src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Go G5 Planner Tool-Policy Executable Shadow

Source:

```text
Reasonix planner/executor drift remains a code-level input. The public
Reasonix task/planner protocol is still rejected.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner read-only + create_plan gate | `code-port-and-adapt` | Go G5 executable shadow now computes the same advertised-tool policy from TS-owned fixture input. |
| Forged internal task rejection | `contract-reimplement` | Go computes forged `task` as `failed` / `tool_dispatch_rejected` / `executed:false`. |
| Live Go planner/executor parity | `defer` | Requires G5/G6 live runtime, rollback, packaging, and Electron evidence first. |
| Reasonix public task/planner protocol | `reject` | No SessionAPI, task cards, Workflow/Subagent navigation, or controller protocol is copied. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Go G5 control executable planner gate | `packages/runtime-go/shadow_g5.go` computes step 0, step 1, and forged call output. |
| Drift guard | `go-g5-full-loop-oracle.json`, schema, and TS test bind planner executable inputs to the TS task-job oracle. |

Rejected / deferred:

```text
Reasonix public planner/task protocol, full Reasonix planner/executor parity,
live Go runtime behavior, default Go backend, renderer-visible Go route,
Electron-to-Go routing, live provider/cache superiority, live MCP/indexer
parity, release readiness, Kun identity, deprecated bridge/settings fallback,
and Rust/Tauri rewrite.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Renderer Approval Live-Card Store Evidence

Source:

```text
Reasonix approvalManager/control drift already classified in the approval
abort cleanup batch. This addendum adds no new upstream delta; it closes a
local analytix evidence gap for renderer live-card behavior.
```

Classification:

| Item | Classification | analytix decision |
| --- | --- | --- |
| Live approval-card status update | `contract-reimplement` | Keep the existing analytix `ThreadEventSink` contract and prove `onApprovalStatus` updates the current approval block in store state. |
| Side conversation approval status | `code-port-and-adapt` inside analytix-owned UI state | Prove side SSE subscriptions update side blocks only, preserving main thread state and product-owned side surface semantics. |
| Reasonix frontend SessionAPI / ask protocol | `reject` | No Reasonix protocol, public route, or top-level Workflow/Subagent/AutoResearch UI is imported. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Main thread renderer store | `chat-store-runtime.test.ts` covers approval request plus live status resolution on the same block. |
| Side conversation renderer store | `chat-store-side-actions.test.ts` covers side-only approval status updates through the side subscription sink. |
| Product boundary | No bridge, settings, provider, Go, Rust/Tauri, or route change. |

Rejected / deferred:

```text
Reasonix frontend ask protocol
full desktop visual click-through QA
packaged app crash/restart QA
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
default Go backend or renderer-visible Go route
live provider/cache superiority and live MCP/indexer parity
release readiness
```

Focused validation:

```text
npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Planner Gating For Internal Task Tools

Source:

```text
Reasonix planner/executor and task orchestration capabilities were previously
classified as contract-reimplement behind analytix runtime contracts. This
addendum closes the planner-gating safety gap for internal child-job tools.
```

Classification:

| Item | Classification | analytix decision |
| --- | --- | --- |
| Internal task/parallel tool availability | `contract-reimplement` | Keep `task` / `parallel_tasks` as internal runtime tools only; do not expose Reasonix task protocol or top-level Subagent/Workflow UI. |
| Planner forbidden tool surface | `code-port-and-adapt` into analytix oracle | Record `plannerForbiddenToolset: ["task", "parallel_tasks"]` and prove Plan mode excludes those tools. |
| Forged planner task execution | `contract-reimplement` | Active tool policy rejects forged `task` calls in Plan mode before child work executes. |
| Reasonix public planner/task protocol | `reject` | No Reasonix planner route, SessionAPI, task cards, or shell navigation entry is imported. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Plan-mode advertisement | `loop.test.ts` covers `task` / `parallel_tasks` registered in the tool host but absent from the model request. |
| Active policy rejection | The same test proves a forged `task` call returns `tool_dispatch_rejected` and the task execute function is not called. |
| Runtime oracle | `task-job-orchestration-oracle.json` and `runtime-parity-fixtures.ts` pin planner-forbidden task tools. |

Rejected / deferred:

```text
Reasonix public task/planner protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
full Reasonix planner/executor Coordinator parity
desktop nested-card QA
default Go backend or renderer-visible Go route
live provider/cache superiority and live MCP/indexer parity
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "keeps internal task tools out of plan mode" --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - User-Input Live-Card Store Evidence

Source:

```text
Reasonix approvalManager/control drift already classified in the approval
abort cleanup batch. This addendum closes the symmetric renderer evidence gap
for user-input resolved events.
```

Classification:

| Item | Classification | analytix decision |
| --- | --- | --- |
| Live user-input-card status update | `contract-reimplement` | Keep the existing analytix `ThreadEventSink` contract and prove `onUserInputStatus` updates current user-input blocks in store state. |
| Side conversation user-input identity | `code-port-and-adapt` inside analytix-owned UI state | Use the runtime item id for side user-input blocks and match status events by item id or request id. |
| Reasonix frontend SessionAPI / ask protocol | `reject` | No Reasonix protocol, public route, or top-level Workflow/Subagent/AutoResearch UI is imported. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Main thread renderer store | `chat-store-runtime.ts` matches user-input status by block id or request id; `chat-store-runtime.test.ts` covers input-id fallback cancellation. |
| Side conversation renderer store | `chat-store-side-actions.ts` uses stable runtime item ids and preserves answers/error messages; `chat-store-side-actions.test.ts` covers side-only user-input status updates. |
| Product boundary | No bridge, settings, provider, Go, Rust/Tauri, or route change. |

Rejected / deferred:

```text
Reasonix frontend ask protocol
full desktop visual click-through QA
packaged app crash/restart QA
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
default Go backend or renderer-visible Go route
live provider/cache superiority and live MCP/indexer parity
release readiness
```

Focused validation:

```text
npm run test -- src/renderer/src/store/chat-store-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Approval/User-Input Abort Cleanup And Replay Closure

Source:

```text
Reasonix approvalManager/control separation drift previously classified in
D-0011 / D-0014 as useful but too broad for the goal-control batch.
No new Reasonix public protocol, SessionAPI, or frontend surface is imported.
```

Classification:

| Reasonix source idea | Classification | analytix decision |
| --- | --- | --- |
| Approval manager owns pending cleanup | `contract-reimplement` | Add `ApprovalGate.expire()` and make `AgentLoop` expire pending approvals when the turn aborts while awaiting a GUI decision. |
| Late approval after abort cannot execute | `contract-reimplement` | Expired approvals reject the pending promise, emit `approval_resolved: expired`, return 409 on later HTTP allow/deny, and preserve the existing cancelled tool-result path. |
| User input abort should replay as closed | `contract-reimplement` | `request_user_input` abort now records `user_input_resolved: cancelled`; late GUI submit/cancel returns 404 because the gate is already cleared. |
| Reasonix SessionAPI / frontend ask protocol | `reject` | Keep existing analytix HTTP/SSE routes and renderer projection; do not expose Reasonix-native approval/user-input protocol. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Approval abort cleanup | Pending approval gates are expired on turn abort and cannot be revived by late GUI decisions. |
| User-input replay closure | Abort while awaiting structured GUI input records both requested and resolved/cancelled events for SSE replay. |
| Renderer live mapping | `approval_resolved: expired` maps to the existing approval block error state, so live streams do not leave the card pending. |
| Route oracle | `approval-user-input-route-oracle.json` pins late approval to 409, late user-input resolve to 404, and replay kinds for cleanup. |
| Tool history safety | Cancelled approval/user-input waits continue through existing synthetic `tool_call_cancelled` results rather than producing denied or dangling approval items. |

Rejected / deferred:

```text
Reasonix SessionAPI, frontend approval cards, public approval manager protocol,
top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation, live renderer
approval-card QA, live provider/MCP superiority, Go backend parity, default Go
backend, Rust/Tauri rewrite, and release readiness.
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/ports.test.ts tests/loop.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-mapper.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
git diff --check
```

## 2026-06-21 - Post-881 Auto-Plan Currentness And Analytix Router Guard

Source:

```text
Reasonix sibling repo: /Users/sun/Projects/DeepSeek-Reasonix
Compared range: 881b2f2f2644d1873c7867f29c64cae7be7d9c24..9ada14176629b1d59d7ed78446951b2bb5954904
Commits:
  01d9b173 fix: make auto-plan a user-level setting
  2db7acf6 fix: rebuild auto-plan classifier on enable
  9ada1417 merge for user-level auto-plan
```

Classification:

| Reasonix commit | Subject | Classification | analytix decision |
| --- | --- | --- | --- |
| `01d9b173` | make auto-plan a user-level setting | `document-only` / `defer` | Analytix has no public auto-plan setting. If added later, it must live under top-level `runtime` settings and use `window.analytix` -> main -> runtime config/restart, not Reasonix config roots. |
| `01d9b173` | reject local/project auto-plan scope | `reject` | Do not add `reasonix config auto-plan --local`, project auto-plan overrides, or Reasonix CLI/config protocol to analytix. |
| `2db7acf6` | rebuild auto-plan classifier on enable | `contract-reimplement` | Absorbed the lifecycle invariant as an analytix-owned auto-model-router classifier fingerprint. Same-turn route-cache keys now include the classifier contract fingerprint, so a future classifier prompt/model/timeout change cannot reuse stale routing. |
| `9ada1417` | merge | `record-only` | Merge context only. No Reasonix public protocol copied. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Classifier rebuild guard | `packages/runtime/src/loop/auto-model-router.ts` exports `AUTO_MODEL_ROUTER_FINGERPRINT` from classifier model, prompt, timeout, response format, max tokens, temperature, and reasoning effort. |
| Same-turn cache invalidation | `AgentLoop.resolveTurnModel` includes the classifier fingerprint in the auto-model route cache key. Existing per-turn cleanup remains unchanged. |
| Proof | `packages/runtime/tests/auto-model-router.test.ts` proves model, prompt, and timeout changes drift the fingerprint while equivalent defaults remain stable. |

Rejected / deferred:

```text
Reasonix auto-plan public config, local/project auto-plan override,
Reasonix CLI/config protocol, Reasonix desktop controller API, full post-881
auto-plan parity, and any stable-prefix mutation for classifier state.
```

Validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/loop.test.ts --no-file-parallelism --maxWorkers=1
```

Remaining risks:

```text
This is internal/executable and shadow evidence, not full Reasonix parity.
Desktop nested-card QA, packaged app QA, Windows host validation, Local Whisper
resources/permissions, live provider credentials, live MCP/indexer operation,
Go live runtime behavior, backend selection, rollback, signing, and release
metadata remain open.
```

## 2026-06-21 - Reasonix 881 Currentness, Cancelled Batch Results, Step Limits, And G5 Control Shadow

Source:

```text
Reasonix sibling repo: /Users/sun/Projects/DeepSeek-Reasonix
Fetched ref: origin/main-v2
Requested target: 881b2f2f2644d1873c7867f29c64cae7be7d9c24
Current origin/main-v2: 9ada14176629b1d59d7ed78446951b2bb5954904
Currentness drift after 881: 2db7acf6 fix: rebuild auto-plan classifier on enable; 9ada1417 merge
Kun refs rechecked:
  v0.2.13 / kun-v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 / kun-v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
```

Note: `881b2f2f` here is the Reasonix merge commit hash in the
`bfe398cc..881b2f2f` range, not GitHub PR `#881`.

Classification:

| Reasonix commit | Subject | Classification | analytix decision |
| --- | --- | --- | --- |
| `53d515bb` | `fix: make running turns escapable after cancel` | `port-and-adapt` + `contract-reimplement` | Absorbed the runtime value: cancel no longer drops already accepted tool calls; analytix persists completed/cancelled `tool_result` pairs behind existing turn interrupt/SSE semantics. TUI Esc/Ctrl+C UX is not copied. |
| `803a707f` | merge for TUI cancel escape | `record` | Recorded as merge context only. No TUI public protocol copied. |
| `f9461704` | `fix(agent): preserve cancelled batch results` | `port-and-adapt` | Absorbed in `AgentLoop.dispatchToolCalls`: fulfilled batch results persist, rejected/aborted and unstarted calls receive synthetic `tool_call_cancelled` results in provider call order. |
| `79af368a` | merge for cancel batch results and parallel stop | `record` | Recorded as merge context only. |
| `62180a99` | `Make agent step limits user-global` | `contract-reimplement` | Reimplemented as analytix-owned `runtimeTuning.stepLimits` plus thread/session and turn overrides; no Reasonix config root or dynamic prompt-prefix state copied. |
| `881b2f2f` | merge for user-global step limits | `record` | Recorded as the requested target merge. |
| `2db7acf6`, `9ada1417` | auto-plan currentness drift after 881 | `record` / future review | Current `main-v2` is beyond 881. This stage records the drift but does not import Reasonix auto-plan classifier behavior. |

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Running-turn cancel | Existing `interruptTurn` remains the public contract; runtime loop now persists paired cancelled results before returning `aborted`. |
| Cancelled batch results | Parallel read-only/delegation batches preserve completed/failed/cancelled/unstarted evidence by call id and order. |
| Durable task cancellation | Parent abort propagates into foreground/background task jobs; `parallel_tasks` returns completed, killed, and skipped child evidence instead of discarding the batch. |
| Step limits | `runtimeTuning.stepLimits` supports default, user-global, planner, and headless budgets; thread `runtimeStepLimits.maxModelSteps` and per-turn `maxModelSteps` override at runtime. `0` on the resolved limit means unlimited. |
| Stable prefix/cache | Step budgets stay in runtime config/tool context/turn metadata and are not written to the immutable system prefix, few-shots, or context instructions. |
| Go G5 | `go-g5-full-loop-oracle.json` adds `controlReplay` for cancel, task-job cancel aggregation, and step-limit controls; Go remains shadow-only. |

Rejected / deferred:

```text
Reasonix public control/session/task/planner protocol
Reasonix TUI Esc/Ctrl+C implementation and CLI status model
Reasonix auto-plan classifier drift after 881
Reasonix config root or public setting names
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
Kun identity, deprecated bridge aliases, or Reasonix public protocol
default Go backend, Electron-to-Go routing, renderer-visible Go routes
Rust/Tauri rewrite
live provider/cache superiority, live MCP/indexer parity, release readiness
```

Focused validation already run:

```text
npm --prefix packages/runtime test -- loop.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- task-job-orchestration-oracle.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- delegation-runtime.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- app-settings.test.ts app-ipc-schemas.test.ts analytix-process.test.ts
```

Final command gates and forbidden-surface scans are recorded in the release
evidence file after the final rerun.

## 2026-06-21 - Auto-Route Step/Cancel Control-Composition Oracle

Source:

```text
Reasonix sibling repo: /Users/sun/Projects/DeepSeek-Reasonix
Relevant post-881 risk cluster: classifier currentness, user-global step
limits, running-turn cancel, and cancelled batch-result preservation.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| auto-route cache must stay turn-scoped when step limits fire | `contract-reimplement` | Added a real `AgentLoop` test: `model:"auto"` routes once, reuses the route for two model steps, then fails at the user-global step limit without writing limit state into stable prefix/context instructions. |
| cancel and step/cache invariants must remain compatible | `code-port-and-adapt` | Added a G5 combined executable fixture: auto-route cache reuse, next-turn reroute, step-limit error, stable-prefix exclusion, completed-result preservation, and unstarted cancelled result status are computed together. |
| live Go full-loop implementation | `defer` | Go computes the combined G5 control case in shadow only; no Go provider/session/MCP/job runtime, Electron route, or default backend is introduced. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime loop | `loop.test.ts` now proves auto-route cache and step limit interact without stable-prefix drift. |
| G5 shadow | `go-g5-full-loop-oracle.json`, TS schema/tests, and `packages/runtime-go/shadow_g5.go` now carry/compute a combined control case across cache, step, and cancel invariants. |
| Product boundary | No Reasonix public protocol, no new renderer route, no default Go backend, and no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry. |

Rejected / deferred:

```text
Reasonix public control/session/task/planner protocol
Reasonix auto-plan user setting or local/project override
live Go runtime parity or default Go backend
Electron-to-Go routing or renderer-visible Go routes
live provider/cache superiority
live MCP/indexer parity
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "auto route|step limit" --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Preload Bridge/API Sovereignty Oracle

Source:

```text
Reasonix post-881 frontend/session protocol review and analytix renderer bridge
contract.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public SessionAPI / frontend protocol as renderer bridge | `reject` | Do not expose Reasonix public protocol, SessionAPI names, or frontend-driving facade names through preload, `Window`, or shared bridge types. |
| Code-level runtime/session ideas behind analytix contracts | `contract-reimplement` / `defer` | Continue to absorb runtime value only through `window.analytix`, top-level `runtime` settings, and runtime HTTP/SSE contracts. |
| Bridge/API sovereignty proof | `contract-reimplement` | Add executable source-level tests that preload exposes only `analytix`, the renderer `Window` type declares only `analytix`, and shared public API names stay analytix-owned. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Preload bridge | `preload-sandbox.test.ts` extracts `contextBridge.exposeInMainWorld(...)` calls and requires exactly `['analytix']`. |
| Renderer global type | The same test extracts `interface Window` properties and requires exactly `['analytix']`. |
| Shared API facade | The test requires `AnalytixApi` and rejects exported `KunApi`, `KunGuiApi`, `ReasonixApi`, `ReasonixSessionAPI`, `DeepSeekApi`, and `AnalytixGuiApi`. |
| Product boundary | No Reasonix public protocol, bridge alias, deprecated settings fallback, provider default, Go/Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix SessionAPI or frontend protocol as public renderer bridge
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
default Go backend or renderer-visible Go route
Rust/Tauri rewrite
packaged desktop QA and release readiness
```

Focused validation:

```text
npm run test -- src/preload/preload-sandbox.test.ts
```

## 2026-06-21 - Renderer Thread Lifecycle HTTP Oracle

Source:

```text
Reasonix thread/session control protocol review and analytix renderer runtime
adapter.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix SessionAPI / thread lifecycle protocol in renderer | `reject` | Do not expose Reasonix public thread/session protocol through the renderer. |
| Renderer lifecycle calls through analytix HTTP/SSE | `contract-reimplement` | Pin list/search/archive/restore/rename/workspace/delete to `/v1/threads` analytix endpoints, complementing existing fork/resume/SSE tests. |
| Packaged desktop thread-list QA | `defer` | Keep as release-readiness work; this batch is a renderer adapter oracle with mocked bridge calls. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Thread list/search | `analytix-runtime.test.ts` proves search/include-archived/archived-only/limit options produce the analytix `/v1/threads` query string. |
| Thread mutations | The same test proves archive, restore, rename, workspace update, and delete use `/v1/threads/:id` with analytix-owned bodies. |
| Adjacent lifecycle routes | Existing tests already cover `/v1/threads/:id/fork`, `/v1/sessions/:id/resume-thread`, and SSE event subscription. |
| Product boundary | No Reasonix public protocol, bridge alias, deprecated settings fallback, provider default, Go/Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix SessionAPI or public thread/session control protocol
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
live Go route handling or default Go backend
Rust/Tauri rewrite
packaged desktop QA and release readiness
```

Focused validation:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

## 2026-06-21 - Settings/Provider EndpointFormat Persistence Oracle

Source:

```text
Reasonix provider/config drift review and analytix top-level runtime settings
contract.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix config root or CLI provider setting protocol | `reject` | Do not add Reasonix config roots, public settings names, or CLI config protocol. |
| EndpointFormat persistence under analytix settings | `contract-reimplement` | Persist runtime `endpointFormat` under top-level `runtime` and custom provider endpoint format under `provider.providers`. |
| Live provider/cache superiority | `defer` | Keep credentialed provider matrix and cost/cache comparison as future release evidence; this batch is offline settings persistence proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime settings | `settings-store.test.ts` proves `runtime.providerId`, `runtime.model`, and `runtime.endpointFormat` persist and reload from `analytix-settings.json`. |
| Provider profile | The same test proves custom provider `endpointFormat: "custom_endpoint"` and a full `/messages` base URL persist under `provider.providers`. |
| Legacy settings rejection | The persisted file omits top-level `agentProvider` and `agents`. |
| Product boundary | No Reasonix config protocol, bridge alias, provider default change, Go/Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix provider/config root or public settings protocol
Kun/deprecated agent settings fallback
live provider/cache superiority or credentialed matrix
Go provider client or default Go backend
Rust/Tauri rewrite
release readiness
```

Focused validation:

```text
npm run test -- src/main/settings-store.test.ts
```

## 2026-06-21 - Runtime Provider-Selection Request-Shape Oracle

Source:

```text
Reasonix provider routing/cache review and analytix runtime model-dispatch
contract.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public provider protocol or config root | `reject` | Do not expose Reasonix provider/session/control APIs or config names. |
| Thread provider selection feeding model request shape | `contract-reimplement` | Prove `MultiProviderModelClient` selects the thread provider and produces the expected URL/header/body shape through analytix-owned clients. |
| Missing provider fallback to default provider | `contract-reimplement` | Preserve a safe default OpenAI-compatible fallback instead of leaking upstream protocol state. |
| Live provider/cache superiority | `defer` | Keep credentialed matrix and live cache/cost comparison as release evidence; this batch uses fake fetch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Custom full endpoint dispatch | `multi-provider-model-client.test.ts` proves `providerId: "custom-messages"` uses the exact configured `/messages` URL without appending another endpoint path. |
| Request shape | The same test proves Messages headers/body fields are used for the selected provider, while OpenAI Responses fields and chat stream options stay absent. |
| Default fallback | The same test proves a missing provider falls back to `/chat/completions` with bearer auth and chat body shape. |
| Diagnostics | Runtime diagnostics stay on provider id, provider base URL, endpoint format, and configured model fields. |
| Product boundary | No Reasonix public protocol, bridge alias, provider default change, Go/Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix provider/session/control protocol
Reasonix config root or public settings protocol
Kun/deprecated agent settings fallback
live provider/cache superiority or credentialed matrix
Go provider client or default Go backend
Rust/Tauri rewrite
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- src/adapters/model/multi-provider-model-client.test.ts
```

## 2026-06-21 - Scheduled Detector Custom Endpoint Inference Oracle

Source:

```text
Reasonix provider request-shape review and analytix scheduled reminder detector.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public provider protocol or config root | `reject` | Do not expose Reasonix provider/session/control APIs or config names. |
| Scheduled detector custom endpoint inference | `contract-reimplement` | Prove custom full `/messages` and `/chat/completions` URLs are exact and infer the matching body/header/parser family. |
| Live scheduled-task provider matrix | `defer` | Keep credentialed provider testing as release evidence; this batch uses fake fetch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Custom `/messages` | `claw-scheduled-task-detector.test.ts` proves exact URL usage, Messages headers/body shape, and no Responses `input`. |
| Custom `/chat/completions` | The same test proves exact URL usage, chat messages body, and JSON response format. |
| Parser family | Fake provider responses are parsed through the inferred endpoint family. |
| Product boundary | No Reasonix public protocol, bridge alias, provider default change, Go/Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix provider/session/control protocol
Reasonix config root or public settings protocol
Kun/deprecated agent settings fallback
live scheduled-task provider matrix
Go scheduled-task detector or default Go backend
Rust/Tauri rewrite
release readiness
```

Focused validation:

```text
npm run test -- src/main/claw-scheduled-task-detector.test.ts
```

## 2026-06-21 - Forbidden Public Runtime Route Oracle

Source:

```text
Reasonix public protocol review, Kun top-level entry baseline, and Go G5/G6
runtime gate.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public `/v1/reasonix/*` protocol | `reject` | Keep Reasonix route names absent from the runtime HTTP surface. |
| Renderer-visible Go runtime routes before G5/G6 | `reject` | Keep `/v1/runtime/go` and `/v1/runtime/go/health` absent while Go remains shadow-only. |
| Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level routes | `reject` | Keep these capabilities hidden behind internal analytix contracts, not public HTTP route surfaces. |
| Runtime negative route oracle | `contract-reimplement` | Add HTTP tests that forbidden upstream routes return structured 404 responses. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Reasonix protocol rejection | `http-server.test.ts` proves `/v1/reasonix/sessions` and `/v1/reasonix/threads/:id` return 404. |
| Go route gating | The same test proves `/v1/runtime/go` and `/v1/runtime/go/health` return 404. |
| Hidden capability rejection | The same test proves Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route names return 404. |
| Product boundary | No Reasonix public protocol, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix SessionAPI or public protocol
Kun public protocol or forbidden product-entry routes
renderer-visible Go route or default Go backend
Rust/Tauri rewrite
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts
```

## 2026-06-21 - Rehydrated Task-Job Route Oracle

Source:

```text
Reasonix sub-agent/job orchestration review, durable task-job restart drill,
and analytix internal runtime task-job routes.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose Reasonix job/session route names or public orchestration API. |
| Rehydrated job output/wait/kill through analytix routes | `contract-reimplement` | Keep restart-continuity behavior behind authenticated `/v1/runtime/task-jobs/*` routes. |
| Top-level Subagent/Workflow navigation or route | `reject` | Internal task jobs remain hidden behind runtime contracts. |
| Packaged desktop restart QA | `defer` | Keep live desktop restart and child-agent execution QA as release evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Output after restart | `task-job-orchestration-oracle.test.ts` now reads combined pre/post-restart output through `/v1/runtime/task-jobs/output`. |
| Wait after restart | The same test polls `/v1/runtime/task-jobs/wait` for the rehydrated running job's completion status. |
| Kill after restart | The same test kills a rehydrated queued job through `/v1/runtime/task-jobs/kill` and verifies wait returns killed status/error. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
unauthenticated task-job control
default Go backend or renderer-visible Go route
Rust/Tauri rewrite
packaged desktop restart QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

## 2026-06-22 - D-0063 Go G5 Provider Cache Release Guard Executable Shadow

Source:

```text
Reasonix cache curve guard review, analytix provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Offline provider cache release guard | `code-port-and-adapt` | Add `cacheReplay.releaseGuard` and pure Go release guard calculation for tail averages, statuses, collapse counts, and overall pass/fail. |
| Analytix TS guard source of truth | `contract-reimplement` | Derive expected output from `evaluateOfflineCacheCurveGuard(providerCache.releaseGuard)`. |
| Reasonix provider/cache public protocol | `reject` | Do not expose upstream provider/cache route names or public protocol fields. |
| Live provider/cache superiority | `reject` | Fixture and shadow evidence cannot claim live DeepSeek/OpenAI/Anthropic superiority. |
| Credentialed provider matrix / live Go provider client | `defer` | Keep live provider QA and Go provider implementation outside G5 shadow. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records release guard tail averages, statuses, collapse counts, low-tail allowance, threshold/window metadata, and overall pass. |
| TS conformance | `go-runtime-conformance.test.ts` derives expected output from the TS cache guard helper. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the same output from `provider-cache-oracle.json`. |
| Product boundary | No Reasonix provider protocol, live superiority claim, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
live provider/cache superiority claim
Reasonix provider/cache public protocol
Go provider client or default Go backend
renderer-visible Go route
credentialed provider matrix and packaged provider QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0064 Auto-Route Step/Cancel AgentLoop Composition Proof

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 auto-plan classifier/currentness review
and analytix TypeScript AgentLoop.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Auto-route cache plus step/cancel composition | `contract-reimplement` | Add a real AgentLoop test that routes once, reuses the route across a second step, then interrupts a parallel tool batch. |
| Step-limit metadata isolation | `contract-reimplement` | Prove step budget reaches tool context but does not enter stable prefix or model-visible context. |
| Reasonix classifier/controller rebuild semantics | `defer` | Product auto-plan setting and live controller rebuild remain future design work. |
| Reasonix public controller/SessionAPI | `reject` | Do not expose upstream route/API names or public protocol fields. |
| Default Go backend / Go controller parity | `reject` | This is TypeScript AgentLoop proof only. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Auto-route cache | `_auto_router` runs once and `deepseek-v4-pro` / `max` is reused for the second model step. |
| Step metadata | `runtimeStepLimits.currentMaxModelSteps` reaches tools while model request prefix/context stays unchanged. |
| Cancel behavior | Completed tool results remain completed; interrupted/not-started calls become cancelled tool results. |
| Product boundary | No Reasonix auto-plan setting, public controller protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix auto-plan config parity
Reasonix controller/SessionAPI public protocol
planner enable/disable product settings
dynamic step/cancel state in stable prefix
packaged desktop interruption QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "reuses the auto route across a step before preserving cancelled parallel tool results"
```

## 2026-06-22 - D-0065 Planner Step-Limit AgentLoop Proof

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 planner gating/currentness review and
analytix TypeScript AgentLoop.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Planner-specific step gating | `contract-reimplement` | Add a real AgentLoop plan-mode test proving `plannerMaxModelSteps` gates the turn independently of higher default/user-global limits. |
| Planner budget prompt/cache isolation | `contract-reimplement` | Prove planner step-budget text does not enter stable prefix or model-visible context. |
| Reasonix planner enable/disable product controls | `defer` | Product planner toggles and live controller rebuild remain future design work. |
| Reasonix public controller/SessionAPI | `reject` | Do not expose upstream route/API names or public protocol fields. |
| Default Go backend / Go planner parity | `reject` | This is TypeScript AgentLoop proof only. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Planner gate | Plan-mode turn with `plannerMaxModelSteps: 2` fails after two model requests and records `turn_step_limit_exceeded`. |
| Tool surface | Requests advertise `create_plan` under existing plan-mode contracts. |
| Prompt/cache hygiene | Prefix length stays stable and context omits `plannerMaxModelSteps`, `maxModelSteps`, and step-budget text. |
| Product boundary | No Reasonix planner setting, public controller protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix planner enable/disable product controls
Reasonix controller/SessionAPI public protocol
dynamic planner budget state in stable prefix
Go planner backend parity or default Go backend
packaged desktop plan-mode QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "uses the planner step limit for plan-mode turns"
```

## 2026-06-22 - Go G5 User-Input Gate Executable Shadow

Source:

```text
Reasonix approval/user-input/control lifecycle review, analytix
approval-user-input route oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Submitted user-input gate result | `code-port-and-adapt` | Add pure Go G5 `replayG5UserInput` to compute submitted status, answer count, HTTP answer echo, and replay answer omission. |
| Cancelled input late resolve safety | `contract-reimplement` | Bind cancelled status, pending-after zero, and `secondResolveStatus:404` into `controlExecutableCases.userInput`. |
| Shared source fixture | `contract-reimplement` | `BuildG5ShadowSlicesOutput` now consumes `approval-user-input-route-api-gates-v1` alongside task/cache/session/MCP fixtures. |
| Reasonix public ask/session protocol | `reject` | Do not expose Reasonix ask route, SessionAPI, or public protocol. |
| Live Go user-input manager | `defer` | Keep TypeScript runtime authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records submitted/cancelled user-input executable cases and expected output. |
| TS conformance | `go-runtime-conformance.test.ts` binds G5 cases to the existing approval/user-input route oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes submitted/cancelled output; `shadow_test.go` compares it to the oracle. |
| Product boundary | No Reasonix public protocol, ask route, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or top-level hidden capability route is added. |

Rejected / deferred:

```text
Reasonix ask/session public protocol
Go user-input HTTP route or live manager
renderer-visible Go route or default Go backend
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop live-card QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - Go G5 AutoResearch Project-Local State Shadow

Source:

```text
Reasonix long-running task / AutoResearch state review, analytix
AutoResearchProjectStore, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Project-local long-task state | `code-port-and-adapt` | Add `controlExecutableCases.autoResearch` and pure Go `replayG5AutoResearch` for `.analytix/autoresearch/<threadId>/`. |
| Requirement audit rejection | `contract-reimplement` | TS conformance proves unknown `requirement_id` evidence is rejected and does not write findings. |
| Stable prefix/tool schema isolation | `contract-reimplement` | G5 expected output keeps research state outside stable prefix and tool schema. |
| Reasonix public AutoResearch/project protocol | `reject` | Do not expose Reasonix project state files, route names, or top-level AutoResearch UI. |
| Live Go AutoResearch manager | `defer` | Keep TypeScript runtime authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records AutoResearch state path, required files, unknown requirement id, and expected no-public-file/no-route output. |
| TS conformance | `go-runtime-conformance.test.ts` creates real `AutoResearchProjectStore` state and verifies unknown evidence rejection. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes project-local state output; `shadow_test.go` compares it to the oracle. |
| Product boundary | No top-level AutoResearch route, Reasonix public protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix public AutoResearch/project protocol
top-level AutoResearch route or navigation
Go live AutoResearch state store or default backend
renderer-visible Go route
packaged desktop long-task restart QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/autoresearch-store.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - Go G5 MCP Lifecycle Executable Shadow

Source:

```text
Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Retry-all failed MCP startup servers | `code-port-and-adapt` | Add `controlExecutableCases.mcpLifecycle` and pure Go `replayG5MCPLifecycle` for retry attempts and connected/error server ids. |
| Live-local tombstone/resume | `code-port-and-adapt` | Compute active paths after late tombstone plus resume files from the TS MCP oracle. |
| Secret-safe diagnostics | `contract-reimplement` | Pin `Authorization=<redacted>` and `leaksSecret:false` in G5 expected output. |
| Reasonix MCP public protocol / MCP-indexer UI | `reject` | Do not expose upstream MCP/indexer routes or top-level navigation. |
| Live credentialed MCP matrix / Go MCP client | `defer` | Keep fake/local fixtures and TS runtime authority until G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records retry, connected/error ids, live-local paths, tombstone, resume, and redaction expected output. |
| TS conformance | `go-runtime-conformance.test.ts` derives the case from `mcp-tool-lifecycle-oracle.json` and `runLiveLocalIndexerProof`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes MCP lifecycle output; `shadow_test.go` compares it to the oracle. |
| Product boundary | No MCP-indexer top-level route, Reasonix MCP protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix MCP public protocol
top-level MCP-indexer route or navigation
Go live MCP client or default backend
renderer-visible Go route
credentialed MCP server matrix, packaged desktop MCP QA, and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-lifecycle-oracle.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0110 MCP Refresh Catalog Drift Replay

Source:

```text
Reasonix MCP/indexer currentness review and analytix MCP search meta-tool
lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP catalog refresh drift | `contract-reimplement` | Execute `mcp_refresh_catalog` after fake-client catalog expansion and require `catalogDrift: true`. |
| MCP search currentness metadata | `contract-reimplement` | Pin initial/expanded tool names and expected total indexed count in the TS-owned MCP oracle. |
| Go G5 refresh drift summary | `code-port-and-adapt` | Add `mcpReplay.searchRefreshDrift` to G5 fixture/schema/test and Go shadow output. |
| Reasonix MCP-indexer public lifecycle | `reject` | Do not expose upstream public MCP-indexer protocol, route, or top-level navigation. |
| Live Go MCP client / credentialed matrix | `defer` | Keep TypeScript MCP runtime authoritative until G5/G6 and live QA gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime oracle | `mcp-tool-lifecycle-oracle.test.ts` proves refresh drift with fake MCP catalog expansion. |
| Source fixture | `mcp-tool-lifecycle-oracle.json` records refresh drift fields under `searchMetaTools`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `mcpReplay.searchRefreshDrift`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the refresh drift summary with no top-level route exposure. |

Rejected / deferred:

```text
Reasonix MCP-indexer public lifecycle protocol
top-level MCP-indexer route or navigation
live Go MCP client or default Go backend
credentialed MCP server matrix
packaged desktop MCP QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0111 Thread/SSE Route Auth Replay

Source:

```text
Reasonix session/fork/SSE route review and analytix G2 route replay oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Missing-token thread/SSE rejection | `contract-reimplement` | Add a G2 route fixture for `/v1/threads/:id/events` without authorization and require 401. |
| G5 session replay auth summary | `code-port-and-adapt` | Add `routeStatusReplay.auth` with protected route id/path/status/body code and zero SSE frames. |
| Reasonix SessionAPI/thread protocol | `reject` | Do not expose upstream public thread/session control protocol. |
| Live Go HTTP route/default backend | `reject` | Keep Go G5 shadow-only with no renderer-visible route. |
| Packaged desktop restart/resume QA | `defer` | Keep release-readiness QA separate. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G2 fixture | `go-g2-route-replay-oracle.json` includes an unauthorized SSE events route. |
| TS conformance | `go-runtime-conformance.test.ts` dispatches the route without a bearer token and validates the TS HTTP response. |
| G5 conformance | `go-g5-full-loop-oracle.json` and schema require unauthorized count and auth summary. |
| Go shadow | `shadow_test.go` honors per-route auth mode; `shadow_g5.go` computes the same summary. |

Rejected / deferred:

```text
Reasonix SessionAPI/public thread protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop restart/resume QA
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - Go G5 Checkpoint/Rewind Executable Shadow

Source:

```text
Reasonix checkpoint/rewind engine review, analytix P4 checkpoint oracle, and Go
G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Checkpoint/rewind executable safety | `code-port-and-adapt` | Add `controlExecutableCases.checkpointRewind` and pure Go `replayG5CheckpointRewind` for id-prefix, path, confirmation, and audit invariants. |
| Analytix checkpoint oracle source of truth | `contract-reimplement` | Derive ready/blocked files and conversation audit from `buildAuditableCheckpointRewindPlan`, not Reasonix public protocol. |
| Reasonix checkpoint/rewind protocol | `reject` | Do not expose upstream route/API names, SessionAPI fields, or renderer-visible checkpoint protocol. |
| Kun git-ref checkpoint behavior | `reject` | Do not adopt `refs/kun/checkpoints`, direct `git reset`, or direct `git checkout` restore semantics. |
| Live Go checkpoint store/routes | `defer` | Keep TypeScript runtime checkpoint/rewind services authoritative until G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records `axcp_` / `axrp_` / `axra_` / `axrr_` ids, path risks, confirmation, and append-only event kinds. |
| TS conformance | `go-runtime-conformance.test.ts` builds a real combined rewind plan from analytix checkpoint events and path risks. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes path escape blocking, symlink blocking, legal `..name` readiness, explicit confirmation, no transcript rewrite, no git refs, and no public route. |
| Product boundary | No Reasonix checkpoint protocol, Kun checkpoint ref contract, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix checkpoint/rewind public protocol
Kun git-ref checkpoint semantics
Go live checkpoint store, live file mutation, or default backend
renderer-visible Go route
packaged desktop rewind QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - Go G5 Remote-Entry Boundary Executable Shadow

Source:

```text
Reasonix SessionAPI/control-port review, analytix remote-entry control-port
oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Remote-entry narrow port boundary | `code-port-and-adapt` | Add `controlExecutableCases.remoteEntry` and pure Go `replayG5RemoteEntry` for allowed ports and forbidden control planes. |
| Analytix route oracle source of truth | `contract-reimplement` | Derive expected keys from `approval-user-input-route-oracle.json`, not Reasonix SessionAPI. |
| Reasonix SessionAPI/public control protocol | `reject` | Do not expose upstream route/API names or grant full runtime-controller access. |
| Goal/checkpoint/memory/storage/tool-host access | `reject` | Remote entries stay limited to lifecycle, turn, approval, and user-input controls. |
| Live Go remote-entry route | `defer` | Keep TypeScript runtime control ports authoritative until G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records allowed `approvals`/`lifecycle`/`turns`, forbidden control-plane keys, and rejected override keys. |
| TS conformance | `go-runtime-conformance.test.ts` binds the G5 case to `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes forbidden keys absent, rejected override not accepted, no goal/checkpoint/memory/storage/tool-host access, no Reasonix protocol, and no public route. |
| Product boundary | No Reasonix SessionAPI, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix SessionAPI or public remote-entry protocol
remote access to goal/checkpoint/memory/storage/tool-host internals
Go live remote-entry route or default backend
renderer-visible Go route
packaged desktop remote-entry QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0062 Go G5 Resume Pending Gates Executable Shadow

Source:

```text
Reasonix approval/user-input resume lifecycle review, analytix
approval-user-input route oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Resume pending gate executable safety | `code-port-and-adapt` | Add `controlExecutableCases.resumePendingGates` and pure Go `replayG5ResumePendingGates` for source/resumed gate statuses. |
| Analytix route oracle source of truth | `contract-reimplement` | Derive expected statuses from `approval-user-input-route-oracle.json`, not Reasonix ask/session protocol. |
| Reasonix ask/session public protocol | `reject` | Do not expose upstream route/API names or SessionAPI fields. |
| Actionable resumed gates / copied answers | `reject` | Resumed approval is expired, resumed user input is cancelled, and answers are not copied. |
| Live Go resume/gate managers | `defer` | Keep TypeScript runtime session resume and gate managers authoritative until G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` records source pending statuses, resumed expired/cancelled statuses, and no copied answers. |
| TS conformance | `go-runtime-conformance.test.ts` binds the G5 case to `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes `pendingAfterResume:0`, no answer copy, no Reasonix protocol, and no public route. |
| Product boundary | No Reasonix ask/session protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability route is added. |

Rejected / deferred:

```text
Reasonix ask/session public protocol
actionable resumed approval/user-input gates
copying submitted answers into resumed thread state
Go live session-resume route or default backend
renderer-visible Go route
packaged desktop resume QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Managed Runtime Provider Currentness And Identity Guard

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 auto-plan classifier/currentness review,
plus Kun baseline sovereignty review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Managed runtime rebuild lifecycle for classifier/provider currentness | `contract-reimplement` | Close the provider/config part by sharing `buildAnalytixRuntimeSettingsKey` between main ensure and settings-apply change detection. |
| Provider env snapshot currentness | `contract-reimplement` | Prove endpoint/profile drift refreshes `ANALYTIX_MODEL_PROVIDERS` for the managed child runtime. |
| Reasonix user/project auto-plan config | `reject` / `document-only` | Do not import `reasonix config auto-plan`, local/project override, or Reasonix settings shape. |
| Reasonix controller/public protocol | `reject` | No SessionAPI/controller route or renderer-visible protocol is exposed. |
| Live provider/cache superiority | `defer` | No external credential matrix in this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Settings key | `buildAnalytixRuntimeSettingsKey` canonicalizes `resolveAnalytixRuntimeSettings(settings)` and is used by main runtime fingerprint/change checks. |
| Provider drift | `app-settings-provider.test.ts` proves UI-only changes do not change the key, while selected provider URL/endpoint/profile drift does. |
| Managed env | `analytix-process.test.ts` proves provider base URL, endpoint format, context window, and reasoning protocol drift update the serialized provider env. |
| Product identity | `analytix-system-prompt.test.ts` forbids model-visible `Claw/Kun/Reasonix`; preload/settings tests reject upstream public facade domains and `agents.kun` fallback. |

Rejected / deferred:

```text
Reasonix public auto-plan config or controller API
project/local auto-plan override
Reasonix public protocol or SessionAPI
live provider/cache superiority
default Go backend or renderer-visible Go route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/analytix-system-prompt.test.ts tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/shared/app-settings-provider.test.ts src/main/analytix-process.test.ts src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Provider Endpoint URL Builder Parity

Source:

```text
Reasonix provider/cache request-shape review and analytix auxiliary provider
consumer audit.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Versioned provider endpoint bases | `code-port-and-adapt` | Shared `upstreamOpenAiModelEndpointUrl` handles `/v2`, `/v3`, and arbitrary `vN` bases for responses/messages. |
| Known endpoint path stripping | `contract-reimplement` | responses/messages auxiliary consumers avoid duplicated `/responses` or `/messages` suffixes. |
| Scheduled detector URL parity | `code-port-and-adapt` | Scheduled detector uses the shared helper instead of its local simplified builder. |
| Write-inline URL non-regression | `contract-reimplement` | Write-inline also uses the shared helper and retains existing custom endpoint behavior. |
| Live provider/cache superiority | `defer` | No credentialed provider matrix in this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Shared helper | `openai-compat-url.test.ts` covers versioned bases, `/beta`, and known endpoint path stripping. |
| Scheduled detector | `claw-scheduled-task-detector.test.ts` proves `/v2/responses?tenant=west` and `/v3/messages/` are used without duplicated paths. |
| Write-inline | `write-inline-completion-service.test.ts` is run with the helper migration to guard custom endpoint behavior. |
| Product boundary | No Reasonix provider protocol, public config, bridge/settings fallback, default Go backend, or top-level hidden capability route is added. |

Focused validation:

```text
npm run test -- src/shared/openai-compat-url.test.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Connect Phone Product Copy Has No Reasonix Protocol Delta

Source:

```text
Kun baseline Connect Phone product-sovereignty review; no Reasonix code delta
is required for this batch.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Connect Phone user-visible/model-visible copy | `contract-reimplement` | Landed as analytix product-copy guard over renderer locales, main runtime replies, and shared prompts. |
| Reasonix public protocol or SessionAPI | `reject` | No Reasonix route/API/name is exposed. |
| Internal `claw` compatibility names | `document-only` | Keep internal names for existing settings, IPC, prompt markers, and thread recovery. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Locale guard | `connect-phone-product-copy.test.ts` scans user-visible strings for standalone `Claw`. |
| Runtime replies | `claw-runtime.test.ts` now expects Connect Phone command/model wording. |
| Prompt copy | `app-settings.test.ts` expects Connect Phone agent tool hints while preserving marker unwrap compatibility. |
| Product boundary | No Reasonix public protocol, top-level hidden-capability route, default Go backend, Rust/Tauri path, Kun identity, or deprecated bridge/settings fallback is added. |

Rejected / deferred:

```text
Reasonix public protocol
Reasonix SessionAPI
internal `claw` rename
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
default Go backend or renderer-visible Go route
live Connect Phone desktop/remote QA and release readiness
```

Focused validation:

```text
npm run test -- src/main/claw-runtime.test.ts src/shared/app-settings.test.ts src/renderer/src/locales/connect-phone-product-copy.test.ts
```

## 2026-06-21 - Task-Job Stale Restart Reconciliation Oracle

Source:

```text
Reasonix tool/sub-agent/job orchestration review and analytix durable task-job
restart behavior.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Queued/running stale restart reconciliation | `contract-reimplement` | Prove queued and running stale task jobs are reconciled to `failed` after runtime restart. |
| Go G5 executable restart shadow | `code-port-and-adapt` | Add pure Go shadow calculation for stale jobs -> failed from TS-owned fixture. |
| Reasonix public job/session protocol | `reject` | Do not expose Reasonix SessionAPI, job route names, or public orchestration API. |
| Live Go Job Manager/default backend | `defer` | Keep Go task/job behavior in G5 shadow until G5/G6 gates are satisfied. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS oracle | `durableRunner.staleReconcile` records `task_orphan_running`, `task_orphan_queued`, restart reason, failed status, and count. |
| Runtime test | `task-job-orchestration-oracle.test.ts` writes both stale records and verifies both become failed. |
| Go shadow | `go-g5-full-loop-oracle.json`, `shadow_g5.go`, and `shadow_test.go` compute and verify the same output. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
Reasonix SessionAPI or route names
live Go Job Manager or default Go backend
renderer-visible Go task/job routes
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop restart QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - MCP Annotation Approval No-Execute Oracle

Source:

```text
Reasonix MCP/tool lifecycle review and analytix MCP tool provider annotation
policy.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Destructive/openWorld MCP annotation approval | `contract-reimplement` | Prove annotated MCP tools require approval and denied approval prevents `client.callTool`. |
| Go G4 MCP no-execute shadow | `code-port-and-adapt` | Add pure Go shadow calculation for `mcpApprovalAnnotatedNoExecute` from TS-owned fixture fields. |
| Reasonix MCP public protocol / MCP-indexer entry | `reject` | Do not expose Reasonix MCP route names, public protocol, or top-level MCP-indexer entry. |
| Live MCP credential matrix | `defer` | Keep this batch fake-client only. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS oracle | `approvalAnnotations` records normalized MCP tool name, annotations, approval id, deny decision, and executed false. |
| Runtime test | `mcp-tool-lifecycle-oracle.test.ts` denies the annotated MCP approval and verifies the MCP client is not called. |
| Go shadow | `go-g4-tools-approval-user-input-mcp-oracle.json`, `shadow_g3g4.go`, and `shadow_test.go` compute and verify the no-execute boolean. |
| Product boundary | No Reasonix MCP public protocol, MCP-indexer top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix MCP public protocol
MCP-indexer top-level entry
live MCP credential compatibility matrix
Go MCP client or default Go backend
renderer-visible Go MCP routes
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop approval QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - User-Input Submitted Route Oracle

Source:

```text
Reasonix user-input lifecycle review and analytix approval/user-input route
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Submitted user-input answers route | `contract-reimplement` | Prove submitted answers are returned through HTTP and delivered to the waiting runtime gate. |
| Answer-free SSE replay boundary | `contract-reimplement` | Keep answers out of `user_input_resolved` replay events while recording submitted status. |
| Go G4 submitted-route shadow | `code-port-and-adapt` | Add pure Go shadow calculation for answer echo and answer-free replay flags. |
| Reasonix public user-input protocol | `reject` | Do not expose Reasonix user-input route names or public protocol. |
| Packaged GUI live-card QA | `defer` | Keep this batch route/oracle only. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS oracle | `submittedUserInput` records submitted answers, expected response/resolution, and resolved event boundary. |
| Runtime test | `approval-user-input-route-oracle.test.ts` submits answers, verifies HTTP/gate resolution, and verifies SSE replay omits answers. |
| Go shadow | `go-g4-tools-approval-user-input-mcp-oracle.json`, `shadow_g3g4.go`, and `shadow_test.go` compute and verify the submitted-route flags. |
| Product boundary | No Reasonix public protocol, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix public user-input protocol
answer archival in SSE history
live cross-device user-input delivery
Go user-input route or default Go backend
renderer-visible Go routes
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged GUI live-card QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Task-Job Route Auth Matrix Oracle

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job runtime route
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Task-job wait/output/kill auth matrix | `contract-reimplement` | Prove all task-job control routes require runtime token and reject missing auth with 401. |
| Reasonix public job/session protocol | `reject` | Do not expose Reasonix job/session route names or public orchestration API. |
| Go route auth parity | `defer` | No Go task-job routes exist before G5/G6 gates. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| TS oracle | `routeContract.authMatrix` records protected wait/output/kill routes and unauthorized 401. |
| Runtime test | `task-job-orchestration-oracle.test.ts` verifies missing-token wait/output/kill all return 401. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Reasonix public job/session protocol
unauthenticated task-job control
Go task-job route auth before G5/G6 gates
renderer-visible Go routes
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop route QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

## 2026-06-21 - Go G5 Task-Job Approval Deny Shadow Replay

Source:

```text
Reasonix tool approval/sub-agent orchestration review, analytix task-job
approval deny oracle, and Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Go G5 consumes task approval no-side-effect proof | `contract-reimplement` | Carry `approvalDenyNoExecute` from the TS task-job oracle into G5 `jobReplay` and Go shadow output. |
| Live Go approval/job manager | `defer` | Do not implement live Go approvals or jobs before G5/G6 gates. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose Reasonix job/session route names or public orchestration API. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` records denied tool names, approval ids, and no durable-job/child-run side effects. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.approvalDenyNoExecute`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the same approval deny no-execute data from `BuildG5ShadowSlicesOutput`. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Go approval manager
Go Job Manager
renderer-visible Go route or default Go backend
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop approval-card QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Go G3/G5 Provider Cache Accounting Shadow

Source:

```text
Reasonix provider/cache accounting review and analytix provider-cache oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider cache accounting in Go shadow | `code-port-and-adapt` | Compute telemetry-supported/unsupported case ids, provider-family ids, hit/miss totals, and aggregate hit rate in Go G3/G5 shadow output. |
| DeepSeek/OpenAI/Anthropic cache semantics | `contract-reimplement` | Keep DeepSeek native cache precedence, OpenAI Responses cached tokens, Anthropic cache fields, and unsupported unknown semantics in analytix-owned fixtures. |
| Live provider superiority | `defer` | Do not claim live provider/cache superiority without credentialed provider QA. |
| Reasonix provider protocol | `reject` | Do not expose Reasonix provider protocol, settings shape, or public route. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G3 fixture | `go-g3-provider-streaming-usage-cache-oracle.json` records `cacheAccounting`. |
| G5 fixture | `go-g5-full-loop-oracle.json` carries the same accounting under `cacheReplay`. |
| TS conformance | `go-runtime-g3-g4-conformance.test.ts` and `go-runtime-conformance.test.ts` derive expected accounting from `provider-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` and `shadow_g5.go` compute accounting without live provider calls. |
| Product boundary | No Reasonix provider protocol, default Go backend, renderer-visible Go route, Kun public protocol, Rust/Tauri path, or new top-level route is added. |

Rejected / deferred:

```text
live DeepSeek/OpenAI/Anthropic/custom provider superiority
credentialed provider matrix
Reasonix provider protocol
default Go backend or renderer-visible Go route
packaged provider settings QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Auto-Router Failure Usage Isolation

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 auto-plan classifier/currentness review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Classifier failure fallback | `contract-reimplement` | Keep the useful classifier path as analytix `_auto_router`, falling back to heuristic when classifier fails. |
| Router usage/cache isolation | `contract-reimplement` | Prove router usage/cache telemetry is not recorded as main thread usage. |
| Reasonix user/project auto-plan config | `reject` / `document-only` | Do not import `reasonix config auto-plan`, project/local override, or Reasonix settings shape. |
| Managed runtime provider/settings rebuild lifecycle | `document-only` | Closed by the later Managed Runtime Provider Currentness And Identity Guard proof; this Auto-Router entry now cross-references that evidence instead of keeping a stale defer. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Loop fallback | `loop.test.ts` proves router failure falls back to `deepseek-v4-flash` heuristic selection. |
| Router isolation | The same test verifies the router request has `tools: []` and `prefix: []`. |
| Usage isolation | The same test emits router usage/cache telemetry before failure and proves no main thread usage event or usage-service accumulation occurs. |
| Managed runtime currentness | Later `app-settings-provider.test.ts` and `analytix-process.test.ts` prove provider endpoint/profile drift changes the runtime key and managed child provider env snapshot. |
| Product boundary | No Reasonix auto-plan config, public protocol, default Go backend, renderer-visible Go route, or top-level hidden capability entry is added. |

Rejected / deferred:

```text
Reasonix `config auto-plan`
project/local auto-plan override
user-visible auto-plan settings
default Go backend or renderer-visible Go route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t 'falls back to a concrete heuristic model without recording router usage'
```

## 2026-06-21 - Go G5 Task Approval Deny Executable Control

Source:

```text
Reasonix tool approval/sub-agent orchestration review, analytix task-job
approval deny oracle, and Go G5 executable control shadow.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Executable approval denial control | `code-port-and-adapt` | Add pure Go G5 `replayG5ApprovalDeny` to compute denied tool names, approval ids/count, and no-side-effect booleans. |
| TS oracle source of truth | `contract-reimplement` | Derive `controlExecutableCases.approvalDeny` from analytix `approvalDenyNoExecute`, not Reasonix public protocol. |
| Live Go approval/job manager | `defer` | Do not implement live Go approvals or jobs before G5/G6 gates. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose Reasonix job/session route names or public orchestration API. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Executable input | `go-g5-full-loop-oracle.json` records `controlExecutableCases.approvalDeny`. |
| TS conformance | `go-runtime-conformance.test.ts` binds the executable input to the TS task-job oracle. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the output, and `shadow_test.go` compares it to the oracle expected output. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
Go approval manager
Go Job Manager
renderer-visible Go route or default Go backend
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
packaged desktop approval-card QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-21 - Task-Job Approval Deny No-Execute Oracle

Source:

```text
Reasonix tool approval/sub-agent orchestration review and analytix task-job
providers.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Approval gate before task-job creation | `contract-reimplement` | Prove real `task` and `parallel_tasks` providers stop at denied GUI approval before durable side effects. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose Reasonix job/session route names or public orchestration API. |
| Live renderer approval-card QA | `defer` | Keep packaged desktop approval interaction as release evidence. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| `task` denial | `task-job-orchestration-oracle.test.ts` returns an approval item for denied `task` execution. |
| `parallel_tasks` denial | The same test returns an approval item for denied `parallel_tasks` execution. |
| No durable jobs | The same test proves `DurableTaskJobManager.list()` stays empty. |
| No child runs | The same test proves `DelegationRuntime.diagnostics()` reports no child runs. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Rust/Tauri path, Kun public protocol, or new top-level route is added. |

Rejected / deferred:

```text
creating child jobs before approval resolution
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
default Go backend or renderer-visible Go route
packaged desktop approval-card QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts
```

## 2026-06-22 - D-0066 Auto-Router Classifier Request Contract

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 auto-plan classifier/currentness review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Classifier request isolation | `contract-reimplement` | Keep the useful classifier as analytix `_auto_router`: isolated turn id, no tools, no prefix, no context instructions, short JSON path. |
| Classifier timeout fallback | `contract-reimplement` | Slow classifier calls are aborted and fall back to heuristic concrete model/reasoning. |
| Classifier contract drift fingerprint | `contract-reimplement` | Timeout/model/prompt changes remain part of `AUTO_MODEL_ROUTER_FINGERPRINT`, so route cache currentness stays tied to the classifier contract. |
| Reasonix auto-plan config/project override | `reject` | Do not import `reasonix config auto-plan`, project/local overrides, or Reasonix settings shape. |
| Product planner/auto-plan toggles | `defer` | User-visible planner enable/disable and live controller rebuild remain future design work. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Router request shape | `auto-model-router.test.ts` asserts `_auto_router` request shape: model, system prompt, JSON response format, `stream:false`, `maxTokens:96`, `temperature:0`, `reasoningEffort:"off"`, `tools: []`, and `prefix: []`. |
| Router prompt boundary | The same test proves recent context and latest request are wrapped inside one classifier user item rather than leaking main-turn prefix/tool state. |
| Timeout fallback | A slow classifier request is internally aborted and resolves to heuristic `deepseek-v4-pro` / `max`. |
| Fingerprint currentness | The timeout override changes the classifier fingerprint, keeping route-cache rebuild semantics analytix-owned. |
| Product boundary | No Reasonix auto-plan config, public controller protocol, default Go backend, renderer-visible Go route, Kun identity, Rust/Tauri path, or top-level hidden capability entry is added. |

Rejected / deferred:

```text
Reasonix `config auto-plan`
project/local auto-plan override
user-visible planner/auto-plan toggles
Reasonix controller/SessionAPI protocol
default Go backend or renderer-visible Go route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route
packaged desktop QA and release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0067 Planner-Executor Transcript Propagation

Source:

```text
Reasonix task/sub-agent transcript continuation/fork review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Executor transcript ref propagation | `code-port-and-adapt` | Add optional `transcriptFor(task)` to the analytix planner-executor coordinator, propagating only `TaskJobTranscriptRef`. |
| Transcript identity guard | `contract-reimplement` | Use analytix `resolveTranscriptOperation` before durable job creation; incompatible identity remains rejected. |
| Go G5 shadow replay | `code-port-and-adapt` | Mirror transcript propagation requirements in G5 fixture/schema and Go shadow output. |
| Reasonix public sub-agent/session protocol | `reject` | Do not expose SessionAPI, public job routes, or upstream transcript protocol. |
| Product Subagent route | `reject` | No top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer entry. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Coordinator | `runPlannerExecutorCoordinator` accepts optional `transcriptFor(task)` and passes the resolved ref to executor jobs. |
| Runtime oracle | `task-job-orchestration-oracle.test.ts` proves real executor jobs persist the fork transcript ref while skipped dependencies create no job. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require transcript propagation flags. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the same shadow-only transcript propagation evidence. |
| Product boundary | No Reasonix public protocol, Subagent top-level route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Rejected / deferred:

```text
Reasonix SessionAPI/public transcript protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
packaged desktop sub-agent QA
full Reasonix planner/executor parity
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0068 MCP Search Meta-Tool Trust Boundary

Source:

```text
Reasonix MCP/indexer lifecycle and tool discovery review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP search meta-tool trust boundary | `contract-reimplement` | Search-mode meta-tools operate only over workspace-trusted records. |
| Unknown/untrusted meta-call no-execute | `contract-reimplement` | Untrusted `toolId` returns a tool error without calling the MCP client. |
| `mcp_call` approval posture | `code-port-and-adapt` | Keep `mcp_call` as `on-request`; denied approval does not execute the MCP tool. |
| G4 shadow replay | `code-port-and-adapt` | Mirror search meta-tool advertised/trust/no-execute evidence in G4 fixture and Go shadow. |
| Top-level MCP-indexer / Reasonix MCP protocol | `reject` | Do not expose upstream MCP-indexer route or public protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime test | `mcp-tool-provider.test.ts` proves trusted search/describe/call works while untrusted search returns 0 and untrusted call does not execute. |
| Approval gate | The same test proves denied `mcp_call` returns an approval item before MCP client execution. |
| Fixture/schema | `mcp-tool-lifecycle-oracle.json` and `runtime-parity-fixtures.ts` record `searchMetaTools`. |
| G4 shadow | `go-g4-tools-approval-user-input-mcp-oracle.json`, `go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go/shadow_g3g4.go` replay the trust/no-execute evidence. |
| Product boundary | No MCP-indexer top-level route, Reasonix MCP protocol, renderer-visible Go route, default Go backend, Kun identity, Rust/Tauri path, or hidden capability entry is added. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer navigation or HTTP route
credentialed MCP server matrix
live Go MCP client or default Go backend
packaged desktop MCP QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-provider.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0069 Custom Messages Full Endpoint Request Shape

Source:

```text
Reasonix provider/cache request-shape review and analytix provider-cache
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Custom `/messages` full endpoint | `contract-reimplement` | Keep custom full endpoint URLs exact and infer the Messages request/response family from the path. |
| Anthropic Messages body/header shape | `contract-reimplement` | Require `system`, `messages`, `max_tokens`, Anthropic tool `input_schema`, `x-api-key`, and `anthropic-version`. |
| OpenAI body fallback for custom messages | `reject` | Do not send chat/responses body fields to `/messages`. |
| Live provider superiority | `defer` | Fixture evidence does not replace credentialed provider matrix. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture | `provider-cache-oracle.json` adds `custom-messages-full-endpoint-request-shape`. |
| Runtime test | `provider-cache-proof.test.ts` request-shape loop verifies exact URL, headers, required/forbidden body fields, and Anthropic tool shape. |
| Product boundary | No Reasonix provider protocol, bridge/settings fallback, default Go backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Rejected / deferred:

```text
Reasonix provider protocol
live OpenAI/Anthropic/custom provider superiority
credentialed provider matrix
default Go backend or renderer-visible Go route
packaged provider settings QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - D-0070 Custom Chat Full Endpoint Request Shape

Source:

```text
Reasonix provider/cache request-shape review and analytix provider-cache
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Custom `/chat/completions` full endpoint | `contract-reimplement` | Keep custom full endpoint URLs exact and infer the OpenAI-compatible chat request/response family from the path. |
| Chat body/header/tool shape | `contract-reimplement` | Require `messages`, `tools`, `reasoning_effort`, Bearer `Authorization`, and OpenAI function tool shape. |
| Go G3/G5 request-shape replay | `code-port-and-adapt` | Mirror the TS-owned request-shape id in Go shadow fixtures only. |
| Responses/Messages fallback fields | `reject` | Do not send `input`, `system`, `max_output_tokens`, `thinking`, or Anthropic headers to custom chat endpoints. |
| Live provider superiority | `defer` | Fixture evidence does not replace credentialed provider matrix. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture | `provider-cache-oracle.json` adds `custom-chat-full-endpoint-request-shape`. |
| Runtime test | `provider-cache-proof.test.ts` request-shape loop verifies exact URL, headers, required/forbidden body fields, and OpenAI tool shape. |
| Go shadow | G3/G5 oracle fixtures now include the custom chat request-shape id for shadow replay. |
| Product boundary | No Reasonix provider protocol, bridge/settings fallback, default Go backend, Kun identity, Rust/Tauri path, or hidden top-level entry is added. |

Rejected / deferred:

```text
Reasonix provider protocol
live OpenAI/Anthropic/custom provider superiority
credentialed provider matrix
default Go backend or renderer-visible Go route
packaged provider settings QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0087 G5 Session Route Status Replay

Source:

```text
Reasonix session/fork/SSE route review, analytix G2 route oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Archive/search route semantics | `contract-reimplement` | Add G5 summary fields for archived status, archived-only count, and archived search result count/status. |
| Read/update/fork/resume route semantics | `contract-reimplement` | Add latest seq, updated workspace, fork side relation/parent lineage, and resume session/message summary. |
| SSE replay/caught-up semantics | `code-port-and-adapt` | Go shadow computes replay/caught-up frame counts and replay event names from G2 SSE frames. |
| Reasonix SessionAPI/public route protocol | `reject` | Keep analytix HTTP/SSE route oracle authoritative; no Reasonix route names or public protocol. |
| Live Go route/default backend | `defer` / `reject` | Keep Go route replay shadow-only until G5/G6 gates and backend-selection rules are satisfied. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `sessionReplay.routeStatusReplay`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the G5 route status summary from G2 route cases. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes route counts/statuses, archive/search/read/update/fork/resume fields, and SSE event names. |
| Boundary | No public Reasonix protocol, renderer-visible Go route, top-level UI entry, or default Go backend is introduced. |

Rejected / deferred:

```text
Reasonix SessionAPI/public protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop restart/resume QA, Go G5/G6 parity, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0089 G5 Task-Job Route Executable Replay

Source:

```text
Reasonix sub-agent/job lifecycle review and analytix task-job route oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Internal wait/output/kill route semantics | contract-reimplement | Add `task-job-orchestration-oracle.routeExecutable` for unauthorized, output, wait, kill, missing output, and rehydrated cases. |
| Go G5 route executable replay | code-port-and-adapt | Emit `jobReplay.routeExecutable` from `BuildG5ShadowSlicesOutput`. |
| Reasonix job/sub-agent lifecycle value | contract-reimplement | Absorb as analytix-owned internal task-job route evidence only. |
| Reasonix public SessionAPI/job protocol | reject | Do not expose upstream routes, Subagent route, Workflow/Create Loop route, or public job protocol. |
| Live Go Job Manager/default backend | defer / reject | Keep shadow-only until G5/G6 gates and rollback evidence exist. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` stores route executable expected responses. |
| Runtime proof | `task-job-orchestration-oracle.test.ts` binds real TS route assertions to the oracle. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.routeExecutable`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the route executable summary from the TS-owned fixture. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
Reasonix SessionAPI
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or Go task-job route server
renderer-visible Go route or default Go backend
packaged desktop sub-agent/task-job QA
Go G5/G6 parity or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0086 G5 Provider Request Shape Replay

Source:

```text
Reasonix provider/cache review, analytix provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| G5 provider request-shape summary | `contract-reimplement` | Add compact request-shape replay to G5 `cacheReplay`, derived from the TS provider-cache oracle. |
| Go G5 request-shape computation | `code-port-and-adapt` | `BuildG5ShadowSlicesOutput` computes exact URL count, endpoint/tool-shape families, custom full endpoint ids, and body-field counts. |
| OpenAI/Anthropic/custom non-regression | `contract-reimplement` | Keep OpenAI chat/responses, Anthropic messages, and custom full endpoint evidence visible in full-loop summary. |
| Provider runtime behavior change | `reject` | Do not change provider request URL/body, headers, stream parsing, usage parsing, or cache diagnostics format. |
| Live provider superiority/default Go backend | `defer` / `reject` | Keep live provider matrix and Go backend activation outside this offline proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `cacheReplay.requestShapeReplay`. |
| TS conformance | `go-runtime-conformance.test.ts` derives G5 request-shape replay from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes request-shape replay in G5 output. |
| Boundary | No public Reasonix provider protocol, top-level UI entry, default Go backend, or provider request behavior change is introduced. |

Rejected / deferred:

```text
Reasonix provider public protocol
runtime provider request behavior changes
live provider/cache superiority claim
live Go provider client, renderer-visible Go route, or default Go backend
packaged provider settings QA, Go G5/G6 parity, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0085 G3 Provider Request Shape Replay

Source:

```text
Reasonix provider/cache review, analytix provider-cache oracle, and Go G3/G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| G3 provider request-shape matrix | `contract-reimplement` | Add full request-shape cases to G3 provider conformance: URL, headers, body fields, endpoint format, and tool shape. |
| Go G3 request-shape summary | `code-port-and-adapt` | `BuildG3ProviderConformanceOutput` computes exact URL count, endpoint/tool-shape families, custom full endpoint ids, and body-field counts. |
| OpenAI/Anthropic/custom non-regression | `contract-reimplement` | Keep OpenAI chat/responses, Anthropic messages, and custom full endpoint surfaces in one oracle-derived matrix. |
| Provider runtime behavior change | `reject` | Do not change provider request URL/body, stream parsing, usage parsing, or cache diagnostics format. |
| Live provider superiority/default Go backend | `defer` / `reject` | Keep live provider matrix and Go backend activation outside this offline proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g3-provider-streaming-usage-cache-oracle.json` and `runtime-parity-fixtures.ts` now require request-shape matrix and summary. |
| TS conformance | `go-runtime-g3-g4-conformance.test.ts` compares raw G3 request-shape matrix and summary against `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` computes request-shape summary in G3 output. |
| Boundary | No public Reasonix provider protocol, top-level UI entry, default Go backend, or provider request behavior change is introduced. |

Rejected / deferred:

```text
Reasonix provider public protocol
runtime provider request behavior changes
live provider/cache superiority claim
live Go provider client, renderer-visible Go route, or default Go backend
packaged provider settings QA, Go G5/G6 parity, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0084 G3 Cache Drift Attribution Replay

Source:

```text
Reasonix provider/cache review, analytix provider-cache oracle, and Go G3/G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| G3 provider cache drift attribution | `contract-reimplement` | Add raw drift shapes and compact output to the G3 provider streaming/usage/cache oracle. |
| Go G3 drift computation | `code-port-and-adapt` | `BuildG3ProviderConformanceOutput` computes stable system/prefixItems and changed tool/provider/model/endpoint flags. |
| TS provider-cache authority | `contract-reimplement` | G3 drift input/output are derived from `ProviderCacheOracle`, not hand-maintained separately. |
| Provider runtime behavior change | `reject` | Do not change provider request URL/body, stream parsing, usage parsing, or cache diagnostics format. |
| Live provider superiority/default Go backend | `defer` / `reject` | Keep live provider matrix and Go backend activation outside this offline proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g3-provider-streaming-usage-cache-oracle.json` and `runtime-parity-fixtures.ts` now require G3 drift attribution. |
| TS conformance | `go-runtime-g3-g4-conformance.test.ts` compares raw G3 drift input and expected output against `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g3g4.go` emits compact drift attribution in the G3 output. |
| Boundary | No public Reasonix provider protocol, top-level UI entry, default Go backend, or dynamic stable-prefix material is introduced. |

Rejected / deferred:

```text
Reasonix provider public protocol
runtime provider request behavior changes
dynamic context or credentials in stable prefix
live provider/cache superiority claim
live Go provider client, renderer-visible Go route, or default Go backend
packaged provider settings QA, Go G5/G6 parity, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0082 Offline Provider/Cache Parity Seal

Source:

```text
Reasonix provider/cache currentness review, analytix provider-cache oracle, and
Go G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| DeepSeek stable prefix equivalence | `contract-reimplement` | Add G5 `cacheReplay.offlineParitySeal.stablePrefixEquivalent` plus prefix-item/tool hash stability from the TS provider-cache oracle. |
| DeepSeek fixture cache hit proof | `contract-reimplement` | Replay stable prefix hit/miss/rate `80/20/0.8` and diagnostics prefix unchanged + telemetry supported. |
| Multi-provider request/usage coverage | `code-port-and-adapt` | Replay usage/request-shape counts and endpoint families for chat, responses, messages, and custom endpoints. |
| Release cache curve guard | `contract-reimplement` | Replay release guard `pass`, threshold `90`, and tail window `4`. |
| Live provider/cache superiority | `defer` / `reject` | Keep `fixtureOnly: true` and `mayClaimLiveSuperiority: false`; no credentialed provider matrix in this batch. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `provider-cache-oracle.json` remains the authority for prefix, usage, request shape, release guard, privacy, and live credential policy. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `cacheReplay.offlineParitySeal`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the offline parity seal from `ProviderCacheOracle`. |

Rejected / deferred:

```text
Live provider/cache superiority
Credentialed provider matrix
Reasonix provider protocol
Go provider client/default backend
Packaged provider settings QA
Release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0074 Go G5 Approval/User-Input Abort Cleanup Executable Shadow

Source:

```text
Reasonix post-881 approval/user-input lifecycle review and analytix
approval-user-input route oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Approval abort cleanup replay | `code-port-and-adapt` | Mirror `appr_abort_cleanup_1` in G5 executable shadow, yielding `expired` and late approval decision `409`. |
| User-input abort cleanup replay | `code-port-and-adapt` | Mirror `input_abort_cleanup_1`, yielding `cancelled`, late resolve `404`, and no pending gates after cleanup. |
| Replay event order | `contract-reimplement` | Pin `approval_requested` -> `approval_resolved` -> `user_input_requested` -> `user_input_resolved` as analytix-owned SSE replay evidence. |
| Reasonix approval/ask public protocol | `reject` | Do not expose Reasonix SessionAPI, approval/user-input protocol, controller routes, or renderer-visible Go routes. |
| Live Go approval/user-input manager | `defer` | Keep TypeScript runtime authoritative until G5/G6 gates are substantially complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.abortCleanup`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the G5 case from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes expired/cancelled statuses, late GUI action statuses, pending-after cleanup, replay kinds, and product-boundary booleans. |
| Product boundary | No Reasonix public protocol, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, default Go backend, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path is added. |

Rejected / deferred:

```text
Reasonix SessionAPI or public approval/user-input protocol
live Go approval/user-input manager
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval-card QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-23 - D-0213 Renderer Runtime Endpoint Builder G5 Shadow

Source:

```text
D-0193 renderer runtime endpoint builder proof and Reasonix post-881 route /
session boundary review.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Renderer provider endpoint builder shadow replay | `code-port-and-adapt` | Promote renderer provider path construction into `desktopSovereignty.rendererProviderEndpointMatrix` and Go G5 shadow output. |
| Root runtime path constants | `contract-reimplement` | Keep `/health` and `/v1/threads` sourced from analytix shared constants, not Reasonix route names. |
| Dynamic route-id injection prevention | `contract-reimplement` | Require encoded thread, turn, approval, user-input, and session ids before bridge `runtimeRequest`. |
| Reasonix public bridge/session protocol | `reject` | Do not expose SessionAPI, controller route names, or upstream protocol through renderer/main/runtime. |
| Live Go desktop bridge/default backend | `defer` | Keep Go shadow-only until G5/G6 gates prove a live backend path. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require renderer-provider endpoint matrix inputs and expected outputs. |
| TypeScript source proof | `go-runtime-conformance.test.ts` derives shared-root, facade, unit-test, encoded-id, and forbidden-path booleans from analytix renderer sources. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes shared-root, encoded dynamic route-id, analytix-owned runtime path, facade, and unit-proof outputs. |
| Scan freshness | `scan:product-sovereignty` now requires D-0213 renderer-provider endpoint proof tokens plus release final-gate evidence. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route/session protocol
deprecated bridge alias or old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
renderer-visible Go route or default Go backend
live Go HTTP server or Electron Go bridge
packaged route/SSE walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0179 Six-Lane Sub-Agent Review Matrix

Source:

```text
Parallel sub-agent review of Reasonix agent-kernel, Kun baseline,
runtime/cache, MCP/tool/sub-agent, Go runtime, and QA/docs streams.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Auto-plan classifier/currentness drift | `contract-reimplement` | Keep the classifier/currentness fingerprint inside analytix runtime fixtures and scan proof tokens. |
| Planner gating | `contract-reimplement` | Preserve explicit plan-mode and planner tool narrowing without public upstream planner toggles. |
| Step/cancel/cache interaction | `contract-reimplement` | Keep combined route-cache, cancel, and stable-prefix proof in G5 fixtures. |
| Provider cache accounting | `code-port-and-adapt` | Accept DeepSeek/OpenAI Responses/Anthropic raw usage accounting. |
| Custom provider telemetry | `contract-reimplement` | Limit the custom provider claim to full-endpoint request shape; do not claim independent custom cache telemetry. |
| MCP lifecycle and approval annotations | `code-port-and-adapt` | Keep MCP search/call/refresh/cancel/error lifecycle internal to analytix runtime contracts. |
| Sub-agent/task-job orchestration | `code-port-and-adapt` | Keep task and parallel task orchestration internal, not a public Reasonix protocol. |
| Reasonix SessionAPI/controller/config roots | `reject` | Do not expose upstream protocol, identity, or route names. |
| Live Go backend/G6/release readiness | `defer` | Keep behind G5/G6, packaged QA, and rollback gates. |

Evidence:

```text
docs/analytix/qa/post-881-subagent-review-2026-06-22.md
scripts/scan-product-sovereignty.cjs
```

Boundary:

```text
No Reasonix public protocol, top-level Subagent/Workflow/Create Loop route,
MCP-indexer product route, renderer-visible Go route, default Go backend,
Kun identity, deprecated bridge/settings fallback, Rust/Tauri path, packaged
desktop QA, live provider/cache superiority, G6 readiness, or release readiness
is claimed by this review matrix.
```

## 2026-06-22 - D-0180 Custom Provider Telemetry Seal

Source:

```text
Reasonix provider/cache review, D-0179 customProviderRequestShapeOnly boundary,
and analytix provider-cache oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Custom full endpoint exact URL/body/header proof | `contract-reimplement` | Keep custom `/responses`, `/messages`, and `/chat/completions` full endpoints in request-shape coverage. |
| Custom provider cache telemetry claim | `reject` | Add executable evidence that custom full endpoint telemetry ids are `[]`. |
| DeepSeek/OpenAI Responses/Anthropic cache accounting | `code-port-and-adapt` | Continue deriving supported cache accounting from raw provider-like usage payloads. |
| Go G5 replay of the boundary | `code-port-and-adapt` | Add the request-shape-only seal to `controlExecutableCases.providerCacheCoverageFloor`. |
| Live credentialed provider matrix | `defer` | Keep live superiority and custom cache telemetry behind future credentialed evidence. |

Evidence:

```text
packages/runtime/tests/provider-cache-proof.test.ts
packages/runtime/tests/go-runtime-conformance.test.ts
packages/runtime-go/shadow_g5.go
```

Boundary:

```text
No provider runtime behavior change, live provider/cache superiority claim,
custom provider cache telemetry, Reasonix provider public protocol, default Go
backend, renderer-visible Go route, Kun identity, deprecated bridge/settings
fallback, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
entry, Rust/Tauri path, packaged provider QA, G6 readiness, or release readiness
is added.
```

## 2026-06-22 - D-0181 Goal Persistence Off-Lock Control Shadow

Source:

```text
Reasonix `341f720` goal-state off-lock write guidance and analytix
`ThreadService` goal persistence tests.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Goal persistence outside controller/status/approval locks | `code-port-and-adapt` | Accept as G5 source/fixture proof without adding a TypeScript controller lock. |
| Persist-before-event goal ordering | `contract-reimplement` | Prove `setGoal` and `clearGoal` persist thread state before emitting goal events. |
| Persistence failure warning and surfacing | `contract-reimplement` | Keep `[analytix] goal persistence failed` warning and original error propagation. |
| Reasonix controller protocol / lock model | `reject` | Do not expose upstream controller API or import its public protocol. |
| Live Go goal manager | `defer` | Keep actual Go goal persistence behind future G5/G6 implementation and rollback gates. |

Evidence:

```text
packages/runtime/tests/go-runtime-conformance.test.ts
packages/runtime/tests/thread-service.test.ts
packages/runtime-go/shadow_g5.go
```

Boundary:

```text
No TypeScript runtime behavior change, live Go goal manager, Reasonix
controller protocol, renderer-visible Go route, default Go backend, Kun
identity, deprecated bridge/settings fallback, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entry, Rust/Tauri path, packaged goal QA,
G6 readiness, or release readiness is added.
```

## 2026-06-22 - D-0153 Go G5 Auto-Router Recommendation/Currentness Replay

Source:

```text
Reasonix `881b2f2f..9ada1417` post-881 auto-plan classifier/currentness review,
analytix auto-model-router tests, and D-0153 explorer gap scan.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Classifier recommendation parsing trust matrix | `contract-reimplement` | Represent the useful Reasonix classifier-currentness invariant as analytix `_auto_router` recommendation parsing: `pro/max` and noisy `v4-flash` are accepted, `model:"auto"` and malformed output are rejected. |
| Recent-context currentness boundary | `contract-reimplement` | Use `recentAutoRouterContext` as the source of truth: active-turn text is excluded and historical tool results are summarized before classifier input. |
| Go G5 executable replay | `code-port-and-adapt` | Extend `controlExecutableCases.autoRouterClassifier` and Go replay output with accepted/rejected counts, pro/max acceptance, auto/malformed rejection, active-turn exclusion, and tool-summary preservation. |
| Reasonix auto-plan controller/config/session protocol | `reject` | Do not expose upstream SessionAPI, public auto-plan setting, project override, controller route, or renderer-visible Go route. |
| Exact per-route session replay / packaged desktop QA / live Go auto-router | `defer` | D-0153 explorer scan recommends exact G5 session route replay as the next high-value batch; packaged desktop and live Go remain outside this slice. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime source | `parseAutoRouteRecommendation` and `recentAutoRouterContext` generate the D-0153 cases. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require recommendation parsing and context-boundary fields under `controlExecutableCases.autoRouterClassifier`. |
| TS conformance | `go-runtime-conformance.test.ts` derives fixture fields from live auto-router helpers instead of hand-written constants. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the expanded auto-router classifier output and `shadow_test.go` compares it with the TS-owned oracle. |

Rejected / deferred:

```text
Reasonix SessionAPI or public auto-plan protocol
Reasonix controller/config root or project auto-plan override
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
renderer-visible Go route or default Go backend
Kun identity or deprecated bridge/settings fallback
Rust/Tauri migration
packaged desktop router/settings QA
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0154 Go G5 Exact Session Route Replay

Source:

```text
Reasonix session/fork/SSE route review, D-0153 explorer recommendation, and
analytix G2 route replay oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Thread list/archive/search/read/update exact route contract | `contract-reimplement` | Derive method/path/auth/status/body-hash rows from the analytix G2 `/v1/threads` oracle. |
| Fork/resume exact route contract | `contract-reimplement` | Preserve analytix `/v1/threads/:id/fork` and `/v1/sessions/:id/resume-thread` route shapes through exact body hashes and status rows. |
| SSE replay/caught-up/auth exact route contract | `code-port-and-adapt` | Carry SSE frame count, event names, frame hash, and unauthorized JSON body hash into G5 control shadow. |
| Reasonix SessionAPI/public route protocol | `reject` | Do not expose upstream session/job route names, renderer-visible Go routes, or public protocol. |
| Live Go HTTP server / packaged desktop route QA | `defer` | Keep G5 shadow-only until G6/live-server/packaged-route evidence exists. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `go-g2-route-replay-oracle.json` remains the authority for thread list/archive/search/read/update, fork, resume, and SSE frames. |
| G5 fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.sessionRouteReplay`. |
| TS conformance | `go-runtime-conformance.test.ts` derives per-route hashes and matrix rows from G2 routes. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes exact route counts, key hashes, and product-boundary flags. |

Rejected / deferred:

```text
Reasonix SessionAPI or public route protocol
live Go HTTP server or renderer-visible Go routes
default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
Kun identity or deprecated bridge/settings fallback
Rust/Tauri migration
packaged desktop route QA
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0104 Go G5 Parallel Task Dependency Executable Shadow

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job
orchestration oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| `parallel_tasks` DAG validation | `code-port-and-adapt` | Add G5 executable shadow inputs for a valid `depends_on` chain and invalid dependency plans. |
| Invalid dependency rejection semantics | `contract-reimplement` | Compute exact single-task, duplicate-id, self-dependency, cycle, and unknown-dependency errors in Go shadow from analytix-owned fixture inputs. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, top-level Subagent navigation, or Workflow/Create Loop routes. |
| Live Go Job Manager/default backend | `defer` / `reject` | Keep TypeScript runtime authoritative until G5/G6 gates, rollback, and packaged QA evidence exist. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| G5 fixture | `go-g5-full-loop-oracle.json` adds `controlExecutableCases.taskJobs.parallelValidation`. |
| TS conformance | `go-runtime-conformance.test.ts` ties fixture inputs and expected errors to `task-job-orchestration-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` executes the validator; `shadow_test.go` compares output to the TS-owned expected oracle. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
renderer-visible Go route
packaged desktop sub-agent/task-job QA
G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0102 Go G5 Task Parent Goal Evidence Executable Replay

Source:

```text
Reasonix sub-agent/job orchestration review, analytix task-job oracle, and
D-0094 parent-goal evidence G5 summary.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Parent-goal evidence executable replay | `code-port-and-adapt` | Add `controlExecutableCases.taskJobs.parentGoalEvidence` and compute it in Go. |
| Active-goal evidence keys | `contract-reimplement` | Preserve `evidenceLedgered` and `evidenceLedgerError` as analytix-owned nested metadata. |
| Missing-goal error path | `contract-reimplement` | Replay that child task evidence errors when no active parent goal exists. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, top-level Subagent navigation, or upstream task protocol. |
| Live Go Job Manager | `defer` | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for active-goal and evidence event-key requirements. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` now require executable parent-goal evidence. |
| Go shadow | `packages/runtime-go/shadow_g5.go` derives `ledgeredWhenActiveGoal` and `errorsWithoutActiveGoal` from event metadata. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
packaged desktop sub-agent/task-job QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0097 Go G5 Approval/User-Input Route Body And Prompt Replay

Source:

```text
Reasonix post-881 gate/currentness review and analytix approval/user-input
route oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Approval deny HTTP body replay | `contract-reimplement` | Replay `decision: deny`, denial reason, response status/body, pending-before, and pending-after from the analytix route oracle. |
| User-input submit prompt/body replay | `code-port-and-adapt` | Replay prompt, structured question option labels, answer request body, HTTP response body, and resolved-event redaction in G5 shadow. |
| User-input cancel prompt/body replay | `contract-reimplement` | Replay cancel body, response body, late `404`, and pending cleanup from the analytix route oracle. |
| Reasonix public ask/session protocol | `reject` | Do not expose SessionAPI, Reasonix public approval/user-input routes, or renderer-visible Go routes. |
| Live Go gate manager | `defer` | Keep TypeScript runtime authoritative until G5/G6 readiness and rollback gates pass. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `approval-user-input-route-oracle.json` remains the single source for route body, prompt, and replay semantics. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` now require `approvalRoute`, `userInputSubmitRoute`, and `userInputCancelRoute`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes route body and prompt summaries from TS-owned fixtures. |

Rejected / deferred:

```text
Reasonix SessionAPI or public ask protocol
live Go approval/user-input manager
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval-card QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/approval-user-input-route-oracle.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0098 Go G5 MCP Lifecycle Detail Replay

Source:

```text
Reasonix MCP/tool lifecycle review and analytix MCP lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Background reconnect detail replay | `contract-reimplement` | Replay failed server ids, suspended provider/reason, connected/error server ids, retry attempts, and no-restart requirement. |
| Known override diagnostics | `code-port-and-adapt` | Replay codegraph and codebase-memory effective cwd, low-priority, background-start, workspace root, and daemon timeout/explicit cwd fields. |
| MCP search workspace boundary | `contract-reimplement` | Replay trusted/untrusted workspace, query, trusted tool id, unknown-tool error, on-request policy, and denied no-execute proof. |
| Reasonix MCP-indexer public lifecycle | `reject` | Do not expose upstream MCP-indexer route, public lifecycle protocol, or renderer-visible Go route. |
| Live Go MCP client/indexer | `defer` | Keep TS runtime authoritative until G5/G6 gates and credentialed MCP matrix exist. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `mcp-tool-lifecycle-oracle.json` remains the single source for reconnect, known override, and search trust semantics. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` now require `backgroundReconnect`, `knownOverrideDiagnostics`, and `searchWorkspaceBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes the MCP detail summary from TS-owned fixtures. |

Rejected / deferred:

```text
Reasonix MCP-indexer public lifecycle protocol
top-level MCP-indexer route or navigation
live Go MCP client/indexer
renderer-visible Go route or default Go backend
credentialed MCP matrix or packaged MCP QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0099 Go G5 Provider Usage Parser Precedence Replay

Source:

```text
Reasonix provider/cache review and analytix provider-cache oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| DeepSeek native cache precedence | `contract-reimplement` | Replay that native `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` win over `prompt_tokens_details.cached_tokens`. |
| OpenAI responses cached tokens | `contract-reimplement` | Replay `input_tokens_details.cached_tokens` hit/miss calculation and output reasoning-token preservation. |
| Anthropic cache fields | `contract-reimplement` | Replay `cache_read_input_tokens` / `cache_creation_input_tokens` as prompt/cache accounting inputs. |
| G5 provider parser summary | `code-port-and-adapt` | Compute `cacheReplay.usageParserReplay` from TS-owned provider-cache fixture in Go shadow. |
| Reasonix provider protocol | `reject` | Do not expose upstream provider protocol or change analytix provider contracts. |
| Live provider matrix / Go provider client | `defer` | Keep fixture-only proof until credentialed matrix and G5/G6 gates exist. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `provider-cache-oracle.json` remains the single source for raw response body and expected usage semantics. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` now require `cacheReplay.usageParserReplay`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes parser-precedence summary from provider usage cases. |

Rejected / deferred:

```text
Reasonix provider public protocol
live credentialed provider superiority claim
live Go provider client
renderer-visible Go route or default Go backend
packaged provider settings QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0100 Go G5 Auto-Router Classifier Currentness Replay

Source:

```text
Reasonix post-881 auto-plan classifier rebuild/currentness review and
analytix auto-model-router tests.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| `2db7acf6` rebuild classifier on enable | `contract-reimplement` | Represent rebuild value as classifier contract fingerprint currentness and route-cache invalidation evidence. |
| Classifier isolated request shape | `contract-reimplement` | Replay no prefix/tools/context-instruction carryover for the short JSON side path. |
| Classifier timeout fallback | `contract-reimplement` | Replay timeout abort -> heuristic concrete model/reasoning fallback. |
| G5 executable shadow replay | `code-port-and-adapt` | Add `controlExecutableCases.autoRouterClassifier` and Go replay output. |
| `01d9b173` user-level auto-plan setting | `document-only / defer` | Do not add product auto-plan settings in this batch. |
| Reasonix CLI/config/project override/controller protocol | `reject` | Do not import public Reasonix auto-plan protocol or SessionAPI. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Runtime source | `auto-model-router.ts` remains the authority for classifier constants and fingerprint builder. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` now require `autoRouterClassifier`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes expected classifier currentness/fallback booleans from TS-owned fixtures. |

Rejected / deferred:

```text
Reasonix auto-plan public setting
project/local auto-plan override
Reasonix controller API or SessionAPI
public task/planner protocol
renderer-visible Go route or default Go backend
packaged desktop planner QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0094 G5 Task Parent Goal Evidence Replay

Source:

```text
Reasonix sub-agent/job orchestration review, analytix task-job oracle, and Go
G5 shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Parent-goal evidence replay | `code-port-and-adapt` | Add G5 `jobReplay.parentGoalEvidence` and compute it in Go shadow from the TS task-job oracle. |
| Analytix evidence event keys | `contract-reimplement` | Preserve `evidenceLedgered` and `evidenceLedgerError` as analytix-owned nested SSE/evidence metadata. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, top-level Subagent navigation, or upstream task protocol. |
| Live Go Job Manager | `defer` | Keep TypeScript task/job orchestration authoritative until G5/G6 gates are complete. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for active-goal and evidence event-key requirements. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.parentGoalEvidence`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads `parentGoalEvidence` and emits active-goal, event-key, no-Reasonix-protocol, and no-top-level-route flags. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route
live Go Job Manager or default Go backend
packaged desktop sub-agent/task-job QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0088 G5 Provider Streaming Usage Replay

Source:

```text
Reasonix provider/cache lessons and analytix G3 provider streaming/usage/cache
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Provider streaming usage event order | contract-reimplement | G5 `providerStreamingReplay.streamingKinds` pins `item_delta`, `usage`, and `turn_completed`. |
| Usage payload comparison | code-port-and-adapt | Go shadow parses SSE `data` payload for `usage` and compares prompt/completion/reasoning/total/cache fields with the G3 provider usage case. |
| DeepSeek cache telemetry in G5 summary | contract-reimplement | `deepseek-prompt-cache` usage hit/miss/rate is visible in full-loop shadow output. |
| Reasonix provider protocol or live provider client | reject | Do not expose Reasonix protocol, change provider request/stream behavior, or add live Go provider client. |
| Credentialed provider matrix / default Go backend | defer / reject | Keep live superiority and default backend claims outside this fixture-only proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `providerStreamingReplay`. |
| TS conformance | `go-runtime-conformance.test.ts` derives the replay summary from `go-g3-provider-streaming-usage-cache-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` parses usage SSE frames and checks declared usage values. |

Rejected / deferred:

```text
Reasonix provider public protocol
provider request/stream parser behavior changes
live provider/cache superiority
credentialed provider matrix
live Go provider client, default Go backend, or renderer-visible Go route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged provider settings QA, Go G5/G6 parity, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0083 Cache Drift Attribution Replay

Source:

```text
Reasonix provider/cache review, analytix provider-cache oracle, and Go G5
shadow conformance.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Cache drift reason replay | `contract-reimplement` | Add G5 `cacheReplay.driftAttribution` with previous/current prefix hashes and expected reasons from the TS provider-cache oracle. |
| Stable system/prefixItems proof | `contract-reimplement` | Require `systemHashStable: true` and `prefixItemsHashStable: true` so drift is not attributed to dynamic stable-prefix pollution. |
| Tool/provider/model/endpoint flags | `code-port-and-adapt` | Go shadow computes changed tool hash, provider, model, and endpoint format from oracle shapes. |
| Dynamic context in stable prefix | `reject` | Do not move file snippets, timestamps, selected text, credentials, or Reasonix sidecar material into stable cache prefix. |
| Live provider superiority | `defer` / `reject` | Keep `telemetrySupported: false` and `cacheHitRateKnown: false` for this drift fixture; credentialed provider matrix remains separate. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require `cacheReplay.driftAttribution`. |
| TS conformance | `go-runtime-conformance.test.ts` derives drift expected output from `ProviderCacheOracle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads `ProviderDriftAttribution` and emits the same replay summary. |
| Provider boundary | The proof remains offline and does not alter provider request URL/body behavior or public protocol. |

Rejected / deferred:

```text
Reasonix provider public protocol
dynamic context or credentials in stable prefix
live provider/cache superiority claim
live Go provider client, renderer-visible Go route, or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged provider settings QA, Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0081 Go G5 Approval Decision Route Replay

Source:

```text
Reasonix approval/user-input lifecycle review and analytix approval-user-input
route oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Approval decision route replay | `contract-reimplement` | Add approval id, deny decision, denied status, and pending-after cleanup to G5 `approvalUserInputReplay`. |
| Late approval decision rejection | `contract-reimplement` | Replay second approval decision status `409`. |
| Approval/user-input replay order | `code-port-and-adapt` | Replay `sinceSeq: 0` and expected approval/user-input event kind order from the TS route oracle. |
| Live Go approval manager | `defer` | Keep TypeScript approval gate authoritative until G5/G6 readiness. |
| Reasonix public ask/session protocol | `reject` | Do not expose upstream protocol, renderer-visible Go routes, or hidden top-level navigation. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `approval-user-input-route-oracle.json` remains the authority for approval decision and replay-order values. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require approval route replay fields. |
| Go shadow | `packages/runtime-go/shadow_g5.go` decodes `approval` and `replay` source fields and emits them in `approvalUserInputReplay`. |

Rejected / deferred:

```text
Reasonix SessionAPI or public approval/user-input protocol
live Go approval manager
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval-card QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0078 Go G5 Task Job Lifecycle Replay

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job
orchestration oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Foreground task completion | `code-port-and-adapt` | Add G5 `jobReplay.lifecycle.foreground` from the TS task-job oracle. |
| Background task cross-turn lifecycle | `contract-reimplement` | Replay running-across-turn status, partial output, final completed status, and final result. |
| Wait/output/kill cancellation | `contract-reimplement` | Replay killed status and cancellation error for task-job route safety evidence. |
| Live Go Job Manager | `defer` | Keep G5 shadow-only until G5/G6 gates and rollback evidence exist. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, Subagent route, Workflow/Create Loop route, AutoResearch route, or upstream task protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for foreground/background/wait-output-kill lifecycle values. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.lifecycle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` decodes `foreground`, `background`, and `waitOutputKill` and emits the lifecycle summary. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch route
live Go Job Manager or default Go backend
packaged desktop sub-agent/task-job QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0079 Go G5 Task Job Route Boundary Replay

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job
orchestration oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Protected task-job route ids | `contract-reimplement` | Add G5 `jobReplay.routeBoundary.protectedRoutes` from the TS task-job oracle. |
| Runtime-token unauthorized status | `contract-reimplement` | Replay unauthorized `401` for task-job route auth. |
| Forbidden top-level route list | `code-port-and-adapt` | Replay `/v1/workflow`, `/v1/create-loop`, and `/v1/autoresearch` as forbidden route evidence. |
| Live Go task-job routes | `defer` | Keep G5 shadow-only until G5/G6 gates and rollback evidence exist. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, Subagent route, Workflow/Create Loop route, AutoResearch route, or upstream task protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for auth and forbidden top-level route values. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.routeBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` decodes `routeContract.authMatrix` and `forbiddenTopLevelRoutes`, then emits the route-boundary summary. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop/AutoResearch route
live Go task-job routes, live Go Job Manager, or default Go backend
packaged desktop sub-agent/task-job QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0076 Go G5 MCP Search Meta-Tool Replay

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP search meta-tool replay | `code-port-and-adapt` | Add G5 `mcpReplay.searchMetaToolNames` and related fields from the TS MCP lifecycle oracle. |
| Workspace trust boundary | `contract-reimplement` | Record `searchUntrustedSearchedTools: 0` and unknown-tool error so untrusted meta-calls cannot reach indexed tools. |
| Approval before MCP client execution | `contract-reimplement` | Record `searchCallPolicy: on-request` and `searchCallDeniedNoExecute: true`. |
| Live Go MCP client / MCP-indexer route | `defer` / `reject` | Keep G5 shadow-only and do not expose MCP-indexer as a top-level product surface. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require MCP search meta-tool replay fields. |
| TS conformance | `go-runtime-conformance.test.ts` derives G5 replay fields from `mcp-tool-lifecycle-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits meta-tool names, trusted tool id, unknown-tool error, untrusted search count, call policy, and denied no-execute. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer navigation or HTTP route
live Go MCP client or default Go backend
credentialed MCP server matrix
packaged desktop MCP QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0080 Go G5 MCP Core Lifecycle Replay

Source:

```text
Reasonix MCP/indexer lifecycle review and analytix MCP tool lifecycle oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| MCP connect/disconnect diagnostics | `code-port-and-adapt` | Add G5 `mcpReplay.lifecycle` connect/disconnect tool names, availability, tool counts, and disconnect reason from the TS MCP lifecycle oracle. |
| Reload schema-order stability | `contract-reimplement` | Replay reload tool names and `schemaOrderStable: true`. |
| Cancel no-execute | `contract-reimplement` | Replay cancel error substring and `cancelExecuted: false`. |
| Approved tool execution error | `contract-reimplement` | Replay `tool_execution_failed` with `approved: true`, distinct from denied approval no-execute. |
| Live Go MCP client / MCP-indexer route | `defer` / `reject` | Keep G5 shadow-only and do not expose MCP-indexer as a top-level product surface. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `mcp-tool-lifecycle-oracle.json` remains the authority for connect/disconnect/reload/cancel/error lifecycle fields. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `mcpReplay.lifecycle`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` decodes MCP lifecycle source fields and emits the lifecycle summary. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer navigation or HTTP route
live Go MCP client or default Go backend
credentialed MCP server matrix
packaged desktop MCP QA
release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0077 Go G5 Parallel Task Dependency Validation Replay

Source:

```text
Reasonix sub-agent/job orchestration review and analytix task-job
orchestration oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| `parallel_tasks` valid dependency order | `code-port-and-adapt` | Add G5 `jobReplay.parallelValidation.validOrder` and dependency field from the TS task-job oracle. |
| Invalid dependency rejection | `contract-reimplement` | Replay five invalid cases: single task, duplicate id, self dependency, cycle, and unknown dependency. |
| Live Go Job Manager | `defer` | Keep G5 shadow-only until G5/G6 gates and rollback evidence exist. |
| Reasonix public sub-agent/job protocol | `reject` | Do not expose SessionAPI, public job routes, Subagent route, Workflow/Create Loop route, or upstream task protocol. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` remains the authority for valid order and error strings. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `go-runtime-conformance.test.ts` require `jobReplay.parallelValidation`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` emits the validation summary from `TaskParallelOracle`. |

Rejected / deferred:

```text
Reasonix public sub-agent/job protocol
top-level Subagent/Workflow/Create Loop route
live Go Job Manager or default Go backend
packaged desktop sub-agent/task-job QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/task-job-orchestration-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 2026-06-22 - D-0187 Renderer Route-Surface Sovereignty Shadow

Source:

```text
Reasonix post-881 frontend/session surface review, Kun 0.2.14 top-level entry
baseline, and analytix Workbench route-surface tests.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Internal orchestration code without top-level UI entry | `contract-reimplement` | Keep dormant Workflow/Create Loop code quarantined and prove no top-level route tokens or entrypoint symbols. |
| Browser preview bridge ownership | `contract-reimplement` | Keep browser preview on `window.analytix` only, not Kun/Reasonix aliases. |
| Plugin marketplace shell safe-area behavior | `code-port-and-adapt` | Preserve shell safe-area/collapsed-sidebar propagation without adding Kun-absent entries. |
| Reasonix public UI/session protocol | `reject` | Do not expose Reasonix route names, SessionAPI, or top-level hidden-capability navigation. |
| Packaged desktop route walkthrough | `defer` | Keep as release-readiness QA; this batch is source-derived G5 shadow proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from Workbench, route-surface tests, chat route types, browser preview bridge, plugin marketplace, shell navigation, Workbench shell, and base shell CSS. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.rendererRouteSurfaceSovereignty`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes route-union, forbidden-token absence, Workflow quarantine, browser preview bridge, plugin safe-area, no-drag shell, native safe inset, and boundary booleans. |
| Scan guard | `scan:product-sovereignty` now checks D-0187 renderer route-surface sovereignty proof tokens. |

Rejected / deferred:

```text
Reasonix public UI/session protocol
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
Kun/deprecated GUI bridge aliases
renderer-visible Go route or default Go backend
old runtime-shaped settings fallback
packaged desktop route walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0188 Package / Runtime CLI Identity Shadow

Source:

```text
Reasonix frontend/runtime protocol review, Kun packaging baseline, and
analytix package/runtime CLI/release identity sources.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Public runtime CLI identity | `contract-reimplement` | Keep the public runtime CLI as `analytix serve`, backed by `packages/runtime/dist/cli/serve-entry.js`. |
| Runtime ready handshake | `contract-reimplement` | Keep `ANALYTIX_READY` and `service: analytix` / `mode: serve` as the GUI startup contract. |
| Release/package identity | `contract-reimplement` | Keep root package/product, builder app id, artifact, NSIS names, AppUserModelID, and release env names analytix-owned. |
| Bundled runtime validation | `code-port-and-adapt` | Keep afterPack validation focused on the bundled analytix runtime CLI and dependencies. |
| Reasonix public CLI/session protocol | `reject` | Do not expose Reasonix command names, SessionAPI, or public runtime protocol. |
| Packaged artifact walkthrough | `defer` | Keep signed/packaged artifact QA as release-readiness work; this batch is source-derived G5 shadow proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from package manifests, builder config, app identity, main AppUserModelID, binary resolver, runtime CLI, afterPack, packaging tests, and release workflow. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.packageRuntimeIdentity`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes package/product/bin/CLI/ready/afterPack/release-env identity booleans. |
| Scan guard | `scan:product-sovereignty` now checks D-0188 package/runtime identity proof tokens. |

Rejected / deferred:

```text
Reasonix public CLI/session protocol
Kun/DeepSeek public product identity
renderer-visible Go route or default Go backend
Rust/Tauri migration
packaged artifact walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0186 Desktop Main IPC Boundary Shadow

Source:

```text
Reasonix frontend/session route review, analytix main IPC runtime request
allow-list, and runtime SSE IPC cursor/stop/batch guards.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Main runtime request allow-list | `contract-reimplement` | Record as `desktopMainIpcBoundary.runtimeRequest`, rejecting forbidden upstream/public routes before runtime adapter calls. |
| Main SSE cursor/stop/batch semantics | `contract-reimplement` | Record start payload strictness, thread events route, stream-id stop matching, reconnect cursor, and 100ms batching. |
| Reasonix SessionAPI/main route protocol | `reject` | Do not expose upstream route names or bridge/session protocol through main IPC. |
| Renderer-visible Go route/default backend | `reject` | `/v1/runtime/go` stays forbidden and Go remains shadow-only. |
| Packaged desktop bridge walkthrough | `defer` | Keep as release-readiness QA; this batch is source-derived G5 shadow proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `app-ipc-schemas`, `register-app-ipc-handlers`, and `runtime-sse-ipc` sources plus their tests. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.desktopMainIpcBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes runtime request strictness, forbidden route rejection, handler no-execute, SSE route/cursor/stop/batch, and boundary booleans. |
| Scan guard | `scan:product-sovereignty` now checks D-0186 desktop main IPC proof tokens. |

Rejected / deferred:

```text
Reasonix SessionAPI or public main IPC protocol
renderer-visible Go route or default Go backend
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop bridge walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0185 Desktop Bridge IPC Sovereignty Shadow

Source:

```text
Reasonix frontend/session protocol review, analytix preload/runtime IPC bridge
guards, and top-level runtime settings sovereignty evidence.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Runtime request IPC bridge through `window.analytix.runtime` | `contract-reimplement` | Record as `runtimeRequestUsesAnalytixIpc` inside the existing `desktopSovereignty` G5 case. |
| SSE IPC bridge through `window.analytix.runtime` | `contract-reimplement` | Record as `runtimeSseUsesAnalytixIpc` with `runtime:sse:start/stop/event/end/error` channels. |
| Shared public API type ownership | `contract-reimplement` | Record `publicApiTypesAnalytixOwned` and reject exported Kun/Reasonix public API names. |
| Reasonix SessionAPI/frontend protocol | `reject` | Do not expose upstream bridge, route, config, or session protocol names. |
| Packaged desktop bridge walkthrough | `defer` | Keep as release-readiness QA; this batch is source-derived G5 shadow proof. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from preload, window types, shared API, settings-store proof, and runtime normalizer sources. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require runtime IPC channel names, no forbidden IPC exposure, and nine desktop sovereignty proof ids. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes bridge-only, runtime-request IPC, SSE IPC, public API type, settings-write, and no-forbidden-channel booleans. |
| Scan guard | `scan:product-sovereignty` now checks desktop bridge IPC proof tokens and keeps preload/shared API sources in post-881 proof freshness paths. |

Rejected / deferred:

```text
Reasonix SessionAPI or public frontend protocol
Kun/deprecated GUI bridge aliases
old runtime-shaped settings fallback
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop bridge walkthrough
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0184 MCP Malformed Schema Boundary Shadow

Source:

```text
Reasonix post-881 MCP lifecycle quality is useful only when malformed tool
schemas remain safe inside analytix-owned MCP tool advertisement and model
catalog contracts.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Non-object MCP `inputSchema` safe default | `contract-reimplement` | Record as `mcpMalformedSchemaBoundary` G5 shadow proof sourced from analytix MCP provider tests. |
| Invalid `properties` and mixed `required` normalization | `code-port-and-adapt` | Keep as future Go MCP catalog parity evidence. |
| Non-record output schema omission | `contract-reimplement` | Keep model/catalog exposure schema-safe. |
| Reasonix MCP-indexer protocol | `reject` | Do not expose upstream MCP-indexer route names or renderer-visible Go route. |
| Live Go MCP client / credentialed MCP QA | `defer` | Keep behind future G5/G6 implementation and packaged/credentialed MCP QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `mcp-tool-provider.ts` and `mcp-tool-provider.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.mcpMalformedSchemaBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes safe default, property drop, required filtering, advertised names, and output-schema omission flags. |

Rejected / deferred:

```text
Reasonix MCP-indexer public protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go MCP client
credentialed MCP QA
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-provider.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0183 Event JSONL Replay Boundary Shadow

Source:

```text
Reasonix post-881 event/session durability is useful only as an engine quality
constraint. Analytix keeps event persistence under its own `events.jsonl`,
runtime recorder, HTTP/SSE, and Go G5 shadow contracts.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Event append/replay/highestSeq contract | `contract-reimplement` | Record as `eventJsonlReplayBoundary` G5 shadow proof sourced from analytix file session tests. |
| Persist-before-publish and concurrent seq uniqueness | `code-port-and-adapt` | Keep as future Go event-store parity evidence. |
| Malformed JSONL recovery and usage compaction carryover | `contract-reimplement` | Keep as long-thread/OOM safety evidence under analytix retention policy. |
| Reasonix SessionAPI/event protocol | `reject` | Do not expose upstream event/session route names or renderer-visible Go route. |
| Live Go event store | `defer` | Keep behind future G5/G6 implementation and packaged long-thread replay QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `file-session-store.ts`, `loop.test.ts`, `runtime-event-recorder.test.ts`, and `file-session-store.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.eventJsonlReplayBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes append/replay/highestSeq/malformed/usage-compaction boundary flags. |

Rejected / deferred:

```text
Reasonix public event or SessionAPI protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go event store
packaged long-thread replay QA
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/runtime-event-recorder.test.ts tests/file-session-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0182 Tool Result File/Image Boundary Shadow

Source:

```text
Reasonix post-881 file/image/tool-result preservation is useful only as an
engine quality constraint. Analytix keeps the contract owned by attachment
metadata, model request fallback, renderer projection, and Go G5 shadow
fixtures.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Tool-result image extraction and recent-image cap | `code-port-and-adapt` | Record as `toolResultFileImageBoundary` G5 shadow proof sourced from analytix tests. |
| Attachment `localFilePath` and text fallback `FilePath` | `contract-reimplement` | Keep as analytix attachment/model request contract; no Reasonix file protocol. |
| Generated-file / attachment meta lifting | `contract-reimplement` | Keep renderer projection proof in `analytix-mapper.test.ts`; no new route. |
| Reasonix SessionAPI/file route names | `reject` | Do not expose upstream public protocol or renderer-visible Go route. |
| Live Go file/image bridge | `defer` | Keep behind future G5/G6 implementation and packaged attachment QA. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Source proof | `go-runtime-conformance.test.ts` derives the case from `tool-result-image.ts`, `tool-result-image.test.ts`, `attachment-store.test.ts`, and `analytix-mapper.test.ts`. |
| G5 conformance | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` require `controlExecutableCases.toolResultFileImageBoundary`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` computes inline image kind preservation, evicted base64 omission, newest-image cap, `localFilePath` preservation, fallback `FilePath`, and generated-file meta flags. |

Rejected / deferred:

```text
Reasonix public file or SessionAPI protocol
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
live Go file/image bridge
packaged attachment QA
Go G6 readiness or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts src/loop/tool-result-image.test.ts tests/attachment-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 2026-06-22 - D-0075 Go G5 Abort Cleanup Replay Summary Closure

Source:

```text
D-0074 executable shadow follow-through and analytix approval-user-input route
oracle.
```

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| Abort cleanup `controlReplay` summary | `contract-reimplement` | Add `controlReplay.abortCleanup` with expired/cancelled statuses, late `409`/`404`, no pending gates, replay kinds, and product-boundary booleans. |
| Approval/user-input replay slice | `code-port-and-adapt` | Compute abort cleanup fields in Go `BuildG5ShadowSlicesOutput` from the TS approval/user-input oracle. |
| Live Go gate managers | `defer` | Keep TypeScript approval/user-input gates authoritative until G5/G6 readiness. |
| Reasonix public ask/session protocol | `reject` | Do not expose upstream protocol, renderer-visible Go routes, or hidden top-level navigation. |

Absorbed evidence:

| Area | analytix-owned result |
| --- | --- |
| Fixture/schema | `go-g5-full-loop-oracle.json` and `runtime-parity-fixtures.ts` now require abort cleanup replay-summary fields. |
| TS conformance | `go-runtime-conformance.test.ts` derives these fields from `approval-user-input-route-oracle.json`. |
| Go shadow | `packages/runtime-go/shadow_g5.go` reads `abortCleanup` from the source oracle and emits the same summary in `approvalUserInputReplay`. |

Rejected / deferred:

```text
Reasonix SessionAPI or public approval/user-input protocol
live Go approval/user-input manager
renderer-visible Go route or default Go backend
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
packaged desktop approval-card QA
Go G5 parity, G6 readiness, or release readiness
```

Focused validation:

```text
npm --prefix packages/runtime test -- tests/approval-user-input-route-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```
