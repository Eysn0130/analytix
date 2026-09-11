# analytix agent quality and product benchmark spec

Status: Normative benchmark method with dated upstream research and result
snapshots.
Current as of: 2026-07-10 for this lifecycle note.
Source of truth for a benchmark claim: the pinned source commit, reproducible
harness, raw artifact, and fresh run against the identified analytix commit.

Rows labelled `pass`, scores, route-absence observations, and upstream claims
later in this file retain their recorded stage/date. They are not permanent
facts and must not be promoted to current product superiority without rerun.

## 1. Purpose

This spec defines how analytix proves that upstream absorption makes the
product stronger over time.

Spec `08` governs how future upstream changes are absorbed without breaking
analytix architecture. This spec adds the missing measurement layer:

```text
spec 08: absorb correctly
spec 09: prove analytix became stronger
```

The long-term goal is not to merge more upstream code. The goal is to make
analytix measurably stronger than the relevant upstream baselines in the
product surfaces analytix owns:

- product workflow completeness;
- coding-agent task success;
- runtime/tool reliability;
- provider/cache/cost efficiency;
- long-thread desktop smoothness;
- cross-platform release readiness;
- safety, approval, user-input, and trace privacy;
- maintainability of future upstream absorption.

Source roles in this benchmark are default research lenses, not exclusive
capability boundaries. A scorecard dimension may compare Kun, Reasonix,
OpenCode, Hermes Agent, CodexDesktop-Rebuild, Claude Code behavior, Claw Code,
Gajae Code, LazyCodex, current analytix, and future admitted sources whenever
they are relevant. Route-surface absence claims are
current-state gates; they do not permanently ban future analytix-native product
entries when a later spec, UX rationale, tests, desktop QA, and sovereignty
scan approve the change.

## 2. Upstream Snapshot

This snapshot records the public upstream positions reviewed on 2026-06-20.
It is a planning input, not a permanent benchmark result.

### 2.1 Reasonix Snapshot

Source:

```text
https://github.com/esengine/DeepSeek-Reasonix
```

Observed strengths:

- default branch has moved to `main-v2`;
- Reasonix 1.0+ is a ground-up Go rewrite;
- legacy TypeScript releases live on the `v1` branch for maintenance;
- focus is a DeepSeek-native terminal coding agent;
- single static Go binary distribution;
- config-driven providers, tools, agents, and plugins;
- MCP-compatible external tools over stdio JSON-RPC;
- two-model executor/planner setup;
- DeepSeek prefix-cache stability and cost focus;
- cross-platform prebuilt binaries and code signing;
- upstream contains a `benchmarks` directory;
- current public activity is high enough to treat it as a fast-moving runtime
  source.

Analytix absorption implication:

```text
Reasonix should feed analytix engine benchmarks, Go runtime conformance,
cache/cost tests, tool/plugin reliability, and CLI/runtime packaging ideas.
It must not control analytix UI, settings, bridge, or renderer protocol.
```

### 2.2 Kun Snapshot

Source:

```text
https://github.com/KunAgent/Kun
```

Observed strengths:

- desktop AI agent workspace with Code and Write modes;
- requirement-first workflow from clarification to design, plan, coding, and
  review;
- SDD, Write, Connect Phone, schedule, plugin, MCP/Skills, multimodal/media
  capabilities, and provider presets;
- local `kun serve` HTTP/SSE runtime boundary;
- Code / Write / Connect Phone share the same local runtime boundary;
- initial release snapshot reviewed here was Kun `0.2.14`; the snapshot-era
  stable absorption baseline was Kun master `8602476`, which includes the
  v0.2.15-era workflow, provider, Connect Phone, preload, tray, and checkpoint
  drift;
- it is the closest upstream for product workflow parity.

Analytix absorption implication:

```text
Kun should feed analytix product workflow tests, UI command coverage, SDD/Write
behavior, Connect Phone/schedule flows, provider preset behavior, and desktop
feature completeness. It must not overwrite the upgraded analytix UI or restore
old product identity.
```

### 2.3 Current Nine-Source Benchmark Routing

The snapshots above are historical. Current comparison inputs must come from
`docs/analytix/upstreams/upstream-sources.json` and a fresh
`npm run audit:upstreams` result. Use these benchmark roles:

| Source | Benchmark contribution |
| --- | --- |
| Reasonix | Persistence/recovery, provider/cache, event replay, Go/Windows runtime reliability. |
| Kun | Product workflow, Design, provider/auth/MCP UX, desktop breadth. |
| OpenCode | Durable execution, timeline continuity, MCP breadth, LSP/formatter. |
| CodexDesktop-Rebuild | Desktop behavior and measured interaction quality only; generated extracts are not source baselines. |
| Claude Code | Clean-room behavioral acceptance for attach/resume, remote control, managed policy, plugin and review UX. |
| Claw Code | Small verified lifecycle fixtures only; exclude claimed but in-memory/stub maturity. |
| Gajae Code | External control, session tree/handoff, plugin quarantine, receipts, native performance; behavior-only while provenance is blocked. |
| Hermes Agent | Observer overhead/failure isolation, compaction integrity, channel recovery, search and environment backends. |
| LazyCodex | LSP/CodeGraph plugin and evidence-verification behavior from committed files only. |

A benchmark proves performance or behavior, not permission to reuse the
measured implementation. Reuse still requires the spec 08 admission gate.

## 3. Benchmark Philosophy

Use three evidence layers:

### 3.0 Current-State Addendum (2026-06-25)

The D-024x/D-0250 evidence rules below preserve their historical stage meaning.
Rows that say TypeScript is the default runtime, Go is shadow-only, or default
backend readiness is blocked by live D-0251/D-0252/D-0253 evidence are
superseded for current startup behavior by the 2026-06-25 Go runtime delivery:
`go-runtime-default` is active by default, and TypeScript remains only as
`ANALYTIX_RUNTIME_BACKEND=typescript` retired-backend diagnostic.

This addendum does not relax release-strength claims. The scorecard, desktop
QA, credentialed provider matrix, credentialed MCP execution, packaged desktop
QA, and operator-reviewed evidence still limit any claim of live provider/MCP/
packaged superiority or fallback physical retirement. DeepSeek-specific cache
or live-probe evidence remains provider-scoped and must not be generalized to
OpenAI-compatible, Anthropic-compatible, or custom endpoint modes.

Reasonix remains the default engine/runtime source for this evidence family.
Analytix must not add Reasonix CLI/settings identity, public Reasonix protocol,
renderer-visible backend switchers, Rust/Tauri migration, or top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entries as an
implicit side effect of Go default validation. Future product entries require
their own analytix-native scorecard and approval path.

### 3.1 D-0238 Backend Gate Evidence Rule

D-0238 may be counted as improvement evidence only for backend-neutral desktop
integration discipline:

```text
main adapter can launch a Go conformance sidecar through dual internal env gates
canary rejects default-backend or renderer-visible Go-route claims
failure rolls back to the TypeScript runtime
renderer/preload/settings/UI contracts remain unchanged
```

It must not be counted as evidence for G6, default Go backend readiness, live
provider/cache superiority, live MCP parity, packaged desktop route QA, or
TypeScript runtime retirement.

### 3.2 D-0239 Kernel Scaffold Evidence Rule

D-0239 may be counted as improvement evidence for Reasonix absorption coverage
and Go kernel integration shape:

```text
kernel route maps durable event sink, loop controller, cache accounting,
approval/user-input gate, MCP catalog recovery, and job/sub-agent orchestration
Reasonix session Job Manager and subagent lineage semantics are classified
job/sub-agent routes remain fixture-only, parent-goal-bound, and non-public
forbidden surface and side-effect counts remain zero
```

It must not be counted as evidence for live job/sub-agent execution, live MCP
or provider parity, G6 default backend readiness, packaged desktop route QA,
or product-surface completeness.

### 3.3 D-0240 G6 Cleanup Evidence Rule

D-0240 may be counted as improvement evidence for migration maintainability and
runtime sovereignty:

```text
g6-retirement-cleanup route lists retained TypeScript/default-runtime paths
deleteAfterG6 lists conformance gates, fixture routes, and shadow-only proofs
forbiddenRedundancy proves deprecated bridges, duplicate settings schemas,
renderer-visible Go routes, runtime switchers, and Reasonix public protocols
are absent
g6Blockers names the live parity, packaged QA, rollback, credentialed matrix,
and retirement-plan work still required before default-backend selection
```

It must not be counted as evidence for G6 itself, TypeScript runtime
retirement, live Go provider/MCP/job parity, release readiness, or a desktop UI
runtime selector.

### 3.4 D-0241 Production-Candidate Parity Evidence Rule

D-0241 may be counted as improvement evidence for Go runtime production
candidate parity slices:

```text
DeepSeek/OpenAI-compatible/Anthropic-compatible fake live HTTP/SSE provider clients parse stream, usage, cache, and prefix-shape telemetry
durable event/session replay reads back events written by the Go loop proof
approval/user-input manager covers pending, deny, submit, cancel, timeout, replay, no denied execution, and no answer persistence in replay events
fake MCP manager proves lifecycle/search/call boundaries without credentials or public MCP-indexer routes
job/sub-agent lineage remains runtime-only and parent goal/thread bound
main adapter production-candidate canary rolls back to TypeScript on failure
```

It must not be counted as evidence for default Go backend readiness, packaged
desktop QA, credentialed provider/MCP matrix parity, TypeScript runtime
retirement, public Reasonix protocol import, or renderer-visible backend
selection. Those remain G6 blockers.

### 3.5 D-0242 Runtime Contract Evidence Rule

D-0242 may be counted as improvement evidence for moving Go from internal proof
routes into an analytix runtime contract subset:

```text
cmd/runtime-server exposes /health, /v1/runtime/info, thread/session/fork/resume, SSE replay, turn create, approval, and user-input routes
turn create uses the D-0241 fake live provider, durable event/session sink, gate manager, fake MCP manager, and parent-bound job lineage
SSE replay proves event sequence, usage/cache accounting, answer-free user-input replay, denied approval no-execute, MCP result, and job lineage
main adapter go-runtime-candidate canary probes real contract endpoints and rolls back to TypeScript on failure
default TypeScript backend, renderer/preload/main public contract, settings schema, and Kun-derived UI remain unchanged
```

It must not be counted as G6, default Go backend readiness, credentialed
provider/MCP parity, packaged desktop QA, TypeScript runtime retirement, public
Reasonix protocol import, or user-visible backend selection. D-0241 internal
proof routes covered by this contract become delete candidates, not permanent
parallel architecture.

### 3.6 D-0243 G6 Readiness Hardening Evidence Rule

D-0243 may be counted as improvement evidence for making the G6/default-backend
gate measurable:

```text
main adapter has D-0243 G6 readiness status and defaults it to ready:false
go-runtime-candidate canary requires ANALYTIX_GO_RUNTIME_G6_READY=1, durable restart, credentialed provider matrix, credentialed MCP matrix, and packaged QA statuses before accepting Go
provider matrix scaffold covers DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom endpoint modes with env-gated credentialed skips
fake provider matrix passes are local proof only and do not count as credentialed provider matrix passes
DeepSeek cache hit/prefix stability uses a repeatable benchmark record format
MCP matrix scaffold runs fake connect/search/call/reconnect/approval/redaction and skips optional credentialed probes when unconfigured
fake MCP matrix passes are local proof only and do not count as credentialed MCP matrix passes
runtime-server crash/restart drill uses a candidate durable root and verifies events.jsonl, highestSeq, replay, pending approval/user-input recovery, and turn sequence continuation
packaged desktop QA has an executable checklist for startup, candidate gate, health, thread list, turn create, SSE replay, and rollback
```

It must not be counted as completed G6, default Go backend readiness, release
readiness, or TypeScript runtime retirement while any credentialed/live or
packaged evidence remains skipped. Skipped provider/MCP/packaged probes are
explicit evidence of missing external state, not pass evidence. The D-0243
status also must not expand renderer-visible runtime contracts, add a backend
switcher, or introduce Reasonix public protocol.

### 3.7 D-0244 Reasonix Red Matrix Evidence Rule

D-0244 may be counted as improvement evidence for making Reasonix absorption
gaps executable and contract-visible:

```text
Go runtime code owns a D-0244 Reasonix capability red matrix pinned to the local Reasonix checkout
runtime info exposes only analytix-owned matrix metadata under capabilities.upstreamAbsorption
green rows require passing machine checks and code-level analytix evidence
red/deferred rows name failing check IDs and G6 blockers
provider/cache, DeepSeek prefix, approval/user-input, MCP, job/sub-agent lineage, durable replay, crash/restart, rollback, Goal evidence, AutoResearch, and rejected public-protocol rows are all represented
Goal evidence counts green only with a Go evidence audit kernel, runtime event schema, durable SSE replay, and restart evidence
MCP counts green only with Go-owned lifecycle/search/call/reconnect/redaction evidence, runtime event schema, durable SSE replay, and no MCP-indexer public surface
```

It must not be counted as completed G6, default Go backend readiness, full
Reasonix parity, live credentialed MCP/provider parity, AutoResearch Go
runtime ownership, or TypeScript runtime retirement while any red or deferred
row remains. It also must not expose Reasonix public protocol, add UI/backend
switchers, or alter the Kun-derived product-entry baseline.

### 3.8 D-0247 Readiness Semantics Evidence Rule

D-0247 may be counted as improvement evidence for making G6 readiness semantics
auditable and preventing accidental default-backend cutover:

```text
runtime info exposes capabilityMatrixGreen, absorptionMatrixGreen, readinessSemanticsD0247, and defaultBackendReadinessD0243
readyForG6 remains only as a compatibility alias for matrix green
readinessSemanticsD0247 records matrixGreenEnablesDefaultBackend:false, skippedCountsAsPassed:false, and fakeMatrixCountsAsCredentialedPass:false
main adapter keeps TypeScript unless ANALYTIX_GO_RUNTIME_G6_READY=1, durable restart, credentialed provider matrix, credentialed MCP matrix, and packaged QA all pass
consolidated readiness report aggregates Go server conformance, durable restart, provider/MCP matrices, packaged QA, product sovereignty, typecheck, and runtime build evidence
```

It must not be counted as completed G6, default Go backend readiness, release
readiness, or TypeScript runtime retirement while any D-0243 hard gate remains
missing, skipped, fake-only, or uncredentialed. D-0246 cleared the AutoResearch
blocker in the matrix, but D-0247 explicitly keeps that separate from default
backend readiness.

| Layer | Purpose | Example |
| --- | --- | --- |
| Conformance | Proves analytix preserves required behavior. | TS runtime and Go runtime produce equivalent SSE events. |
| Comparative scorecard | Proves analytix is stronger than upstreams in target surfaces. | analytix supports Kun workflow plus Reasonix cache/tool strengths. |
| Desktop QA | Proves product behavior works in the actual Electron app. | 120-turn thread, streaming, composer, settings, Write, SDD, terminal. |

Conformance alone is not enough. It proves no drift. It does not prove
analytix is stronger. Comparative benchmarks and desktop QA provide that proof.

## 4. Benchmark Dimensions

Each upstream absorption batch should map affected changes to one or more
benchmark dimensions.

| Dimension | What it measures | Primary upstream comparison |
| --- | --- | --- |
| Product workflow | End-to-end Code, Write, SDD, Design, Connect Phone, schedule, plugin, provider settings. | Kun plus relevant Claude behavior and Hermes channel evidence |
| Agent task success | Ability to modify code, run commands, use tools, recover from errors, and complete reviewable work. | Reasonix, OpenCode, Hermes, Gajae, Claude behavior |
| Runtime contract reliability | HTTP/SSE stability, replay, fork/archive/search, usage, approvals, user input, persistence/recovery. | Reasonix, OpenCode, Hermes, Gajae |
| Tool and MCP reliability | Tool registration, invocation, approval, result mapping, image/file outputs, OAuth/resources/prompts, errors. | Reasonix, OpenCode, Kun, Gajae, Lazy |
| Provider/cache/cost efficiency | Base URL/body/header correctness, stream parsing, usage parsing, cache hit/miss, token cost. | Reasonix, OpenCode, Kun |
| Thread smoothness | Long-thread rendering, streaming, bottom-distance anchoring, markdown finalization, terminal bursts. | OpenCode E2E plus CodexDesktop clean-room behavior reference |
| Cross-platform desktop | macOS/Windows/Linux app startup, packaging, icons, tray/dock, terminal, file paths. | Analytix release specs plus bounded Kun/Reasonix/Hermes/Lazy/Claw evidence |
| Safety and privacy | Approval policy, sandbox, user input, local trace sanitization, secret handling. | Analytix safety bar compared with all relevant sources |
| Maintainability | Contract isolation, test coverage, ledger updates, conflict decisions, backend-neutral renderer. | analytix specs |
| Reuse compliance | Pinned material, license/provenance, file/vendor exceptions, notices, and authorization. | `upstream-sources.json` and `code-reuse-provenance.md`; benchmark success never grants reuse rights |

### 4.1 Reasonix Agent-Kernel Benchmark Dimensions

Reasonix parity or superiority cannot be claimed from sub-agent enablement,
DeepSeek cache fixtures, or Go scaffold alone. Each five-batch agent-kernel
stage needs its own benchmark dimension and scorecard evidence.

| Reasonix batch | Benchmark dimension | Required evidence before parity/stronger claim |
| --- | --- | --- |
| Task closure and permission kernel | permission/evidence/Goal | `permission Gate`, approval posture `ask` / `auto` / `yolo`, separate plan approval and tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules are covered by fixtures and scorecard rows. |
| Long-running task system | AutoResearch/long-running task | `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, and requirement-by-requirement evidence audit survive restart/resume without polluting stable prefix or renderer-visible Reasonix protocol. |
| Tool surface and context economy | token economy/tool schema/history-memory/cache | token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory on-demand retrieval, compaction archive, cache diagnostics, provider matrix behavior, and unsupported cache telemetry fallback are proven across DeepSeek, OpenAI, Anthropic, MiMo, and custom endpoints. |
| Collaborative execution model | task/parallel/background/coordinator | `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, and nested event rendering are product-grade and permission-gated; merely enabling `subagents.enabled` or `delegate_task` does not count. |
| Go runtime kernel | Go runtime kernel | The current Go-only production core keeps Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime conformant with public contracts and retained G0-G6 evidence before a superiority claim. |

No batch may be reported as Reasonix parity or stronger-than-Reasonix unless
its benchmark dimension, upstream baseline, analytix post-change score,
missing gaps, and proof artifacts are recorded in
`docs/analytix/benchmarks/upstream-scorecard.md`.
The Kun 0.2.13 product-entry baseline remains a separate required gate for all
five Reasonix agent-kernel batches.

2026-06-21 Batch 1 status: analytix has a scoped TypeScript proof for
`complete_step`, evidence ledger, evidence-gated goal completion,
denied-approval no-execute, ask/auto/yolo posture mapping, plan/tool approval
separation, blocked-state event/status, and headless/subagent approval policy
inheritance. This raises the task-closure proof from route-only governance to
fixture-backed implementation, but it is not sufficient for a Reasonix parity
or stronger-than-Reasonix claim until route approval, user-input, renderer
review/generated-files, broader runtime/app, and release evidence gaps are
closed or explicitly scored as blockers.

2026-06-21 Batch 2 status: analytix has a scoped TypeScript proof for
`/goal --research`, project-local AutoResearch state, `task_spec.md`,
`progress.json`, `findings.jsonl`, `directions_tried.json`,
`iteration_log.jsonl`, `record_research_direction`, restart/resume, and
requirement-by-requirement evidence audit through `complete_step
requirement_id`. This raises long-running task proof from governance route to
fixture-backed implementation, but it is not sufficient for a Reasonix parity
or stronger-than-Reasonix claim until desktop crash/restart, live provider
research loops, renderer evidence review, Batch 3 tool/context economy, and
release evidence gaps are closed or explicitly scored as blockers.

2026-06-21 Batch 3 status: analytix has a scoped TypeScript proof for dynamic
tool-source lifecycle, connection-order-stable canonical tool catalog
fingerprints, source diagnostics, tool-source cache diagnostics, token economy,
request-history hygiene,
memory/history retrieval, compaction archive/history, usage accounting, and
multi-provider cache telemetry fallback. This raises tool/context economy from
cache-only fixture proof to source-lifecycle-aware proof, but it is not
sufficient for a Reasonix parity or stronger-than-Reasonix claim until live MCP
operations, retrieval ranking, long-session desktop QA, live provider
cache/cost matrix, and release evidence gaps are closed or explicitly scored
as blockers.

2026-06-21 Batch 4 status: analytix has a scoped TypeScript proof for
evidence-ledgered delegated collaboration, existing `delegate_task` fan-out,
maxParallel queueing, queued abort/failure/interruption paths, nested child
event replay, and renderer projection of `evidenceLedgered` metadata. This
 raises collaborative execution from detached delegate output to Goal-aware
evidence proof, but it is not sufficient for a Reasonix parity or
stronger-than-Reasonix claim until first-class `task` / `parallel_tasks`,
durable background jobs, planner/executor coordination, transcript
continuation/fork, desktop nested-event QA, and release evidence gaps are
closed or explicitly scored as blockers.

2026-06-21 Batch 5 status: analytix has a TypeScript-owned G0/G1 oracle and an
isolated Go G1 shadow scaffold for the Go runtime kernel. The shared oracle
fixture, `go-runtime-conformance.test.ts`, and `packages/runtime-go` verify
that Provider Registry, Tool Registry, Controller, Session, Event Sink, Job
Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal
Runtime remain mapped to TS oracle files/tests, and that `/health`,
`/v1/runtime/info`, and `/v1/runtime/tools` match status codes, auth behavior,
JSON shape, tool diagnostics, and capability registry semantics. This raises Go
readiness from TS-only planning to fixture-backed shadow implementation, but it
is not sufficient for Go runtime parity, Reasonix parity, or release readiness
until G2+ route replay, cross-backend runners, rollback, desktop QA, packaged
QA, and scorecard evidence exist.

## 5. Scorecard Scale

Use a five-level scale:

| Score | Meaning |
| --- | --- |
| 0 | Missing or broken. |
| 1 | Exists but unstable, incomplete, or manually verified only. |
| 2 | Parity with upstream or previous analytix behavior. |
| 3 | Stronger than upstream in at least one measured way. |
| 4 | Stronger than upstream and covered by automated tests or repeatable QA. |
| 5 | Stronger, tested, documented, and stable across release gates. |

An item is allowed to ship as normal product work at score `3` when risk is
low. A runtime backend switch, cross-layer contract change, or public release
gate should require score `4` or `5`.

## 6. Required Benchmark Artifacts

Benchmark artifacts live in:

```text
docs/analytix/benchmarks/
  README.md
  upstream-scorecard.md
  benchmark-scenarios.md
  quality-gates.md
```

Each upstream absorption batch should update the scorecard when it affects a
measured dimension.

Concrete absorption targets live in:

```text
docs/analytix/upstreams/absorption-targets.md
docs/analytix/upstreams/code-level-absorption-blueprint.md
docs/analytix/upstreams/code-level-implementation-plan.md
```

Those files bind each target, code-level reuse candidate, and implementation
stage to the proof pattern that shows the absorption made analytix better.

Minimum update fields:

```text
date
batch / branch
upstream source
dimension
baseline score
new score
evidence
remaining gap
```

## 7. Product Workflow Benchmarks

Product workflow benchmarks prove analytix remains stronger than Kun as a full
desktop product.

Required scenarios:

- Code thread starts from a real workspace, uses model settings, streams,
  edits files, requests approval, and produces reviewable changes;
- Write workspace opens, edits Markdown, uses inline completion, uses selected
  text actions, previews, and exports;
- SDD creates requirement draft, generates/updates plan, links work back to
  requirement context, and preserves history;
- Connect Phone saves settings, creates a manual task, and reuses analytix
  runtime threads;
- Schedule creates or reuses runtime threads and preserves task context;
- Workflow / Create Loop is excluded from current Kun `0.2.13`/`0.2.14`
  product-navigation parity unless a future analytix workflow automation spec
  explicitly approves its entry level. Until then, benchmarks may cover only
  unexposed internals and must not treat a top-level Workflow navigation entry
  as current baseline proof;
- Plugin marketplace loads, filters, and preserves current product naming;
- Settings reads and writes top-level `runtime`, providers, write, speech,
  media, updates, memory, worktree, legacy import, and Connect Phone settings.

Kun absorption should not be considered complete if accepted Kun workflow
changes are not represented in these scenarios.

Product-entry parity is part of the benchmark. A Kun-derived capability cannot
score as product parity unless its analytix entry position, route level, visible
command, settings placement, and trigger path match the target Kun version or
are covered by a separate analytix spec decision and renderer tests.

## 8. Agent Task Benchmarks

Agent task benchmarks prove analytix absorbs Reasonix engine strengths without
becoming a terminal-only agent.

Recommended fixture families:

- small code edit with tests;
- multi-file refactor with search and patch;
- failing test diagnosis and repair;
- tool approval and denial path;
- file/image tool result propagation;
- command failure recovery;
- context compaction and continuation;
- fork/resume after interruption;
- review of generated changes;
- checkpoint metadata and conversation-only rewind planning from event replay;
- provider 404/base URL/endpoint-format guidance.

Each fixture should record:

```text
task prompt
workspace fixture
expected files or behavior
allowed tools
approval policy
success criteria
runtime events
final transcript projection
```

## 9. Runtime And Go Backend Benchmarks

Runtime benchmarks extend spec `08` conformance.

Score `2` means Go and TypeScript are equivalent for a fixture. Score `3+`
requires Go or absorbed engine work to improve one of:

- startup time;
- memory use;
- event replay speed;
- tool call latency;
- stream parsing correctness;
- cache hit/miss reporting;
- model request error quality;
- crash recovery;
- binary distribution simplicity;
- cross-platform reliability.

Go runtime cannot become default unless:

- required conformance fixtures pass;
- regression score is at least parity for all required dimensions;
- at least one target engine dimension is measurably stronger;
- desktop QA passes with Go backend;
- rollback is documented.

## 10. Provider, Cache, And Cost Benchmarks

Reasonix is especially relevant here because of its DeepSeek-native and
prefix-cache focus.

Required measurements:

- final request URL for each endpoint format;
- request body shape;
- provider headers;
- stream parsing behavior;
- usage parsing;
- cache hit/miss fields when provider supplies them;
- optional cache diagnostics on usage events, including prefix-change reasons,
  canonical tool-schema stability, provider/model metadata, and cache hit/miss
  token counts without raw prompt or tool content;
- retry behavior;
- sanitized error body and user-facing guidance;
- token count and estimated cost where safe to calculate.

Reasonix cache/prefix-cache proof requires:

- two equivalent turns where the second turn keeps the same stable prefix hash;
- canonical tool schema hashing that ignores tool order and schema key order;
- explicit prefix-change reasons for system, mode, prefix, tools, provider, or
  model drift;
- provider-native cache hit/miss token parsing when the provider supplies it;
- graceful unsupported diagnostics when OpenAI, Anthropic, or custom compatible
  providers do not expose equivalent cache fields;
- evidence that cache optimization does not alter non-cache provider request
  bodies, headers, stream parsing, usage parsing, approvals, user input, tool
  execution, generated files, or review UI.

Historical P2.2 fixture note (2026-06-21):

```text
packages/runtime/tests/provider-cache-proof.test.ts is the TypeScript oracle
for the first fixture-backed cache/provider proof. It covers stable prefix hash,
canonical tool schema hash, provider/model/endpoint attribution,
DeepSeek/Responses/Anthropic cache parsing, and unsupported-provider no-guess
fallback. It does not replace live provider credentials or desktop release QA,
so final Reasonix cache superiority still requires live matrix evidence.
```

That historical file is no longer present. Current proof must use the Go
provider/history/usage/cache tests, `npm run runtime:go:speed-cache-gate`, and
fresh live-provider evidence where a superiority claim requires it.

Never place API keys, tokens, full prompts, full assistant output, or direct
personal contact/payment identifiers in benchmark artifacts. Cache diagnostics
must also exclude raw system prompts, user prompts, tool arguments, tool output,
file contents, and full unsanitized provider response bodies.

## 11. Thread Smoothness Benchmarks

Thread smoothness remains a core differentiator for analytix desktop.

Required scenario:

```text
120+ turns
long markdown
code fences
tool calls
approvals
request_user_input
file-change summaries
review plan and review summary cards
generated files
terminal burst
Dev Browser detection card
streaming active turn near bottom
streaming active turn while user reads history
history prepend
```

Measurements:

- composer input latency;
- React commit samples;
- mounted row count;
- virtualized total height correctness;
- bottom-distance corrections;
- markdown finalization timing;
- terminal burst impact;
- forced-scroll regressions;
- desktop screenshots or trace evidence.

## 12. Safety Benchmarks

Safety regressions are product regressions.

Required checks:

- approval policy is preserved;
- denied tools do not run;
- user input can be submitted and cancelled;
- sandbox mode is honored;
- traces remain local-only and sanitized;
- model request logs sanitize keys/tokens;
- MCP/tool errors do not expose secrets;
- legacy imports are explicit and idempotent;
- checkpoint and rewind metadata/events are analytix-owned, path-safe, and
  replayable before any destructive restore UI exists;
- checkpoint rewind plans are auditable and non-destructive until a separate
  apply contract proves file restore, event replay, and desktop QA;
- Go backend does not weaken any permission path.

## 13. Upstream Comparison Matrix

Each release-readiness report should include a compact matrix:

| Surface | Kun | Reasonix | analytix target | analytix evidence |
| --- | --- | --- | --- | --- |
| Product workflow | strong | narrow | stronger than Kun | product workflow benchmark |
| Agent engine | moderate | strong | at least Reasonix parity, then stronger | agent/runtime benchmark |
| Desktop smoothness | moderate | terminal-first | stronger than both | thread smoothness QA |
| Provider/cache | useful | strong DeepSeek focus | strongest combined | provider/cache benchmark |
| Cross-platform release | desktop app | single binary/desktop emerging | strongest combined | release QA |
| Safety and approval | product-integrated | engine-integrated | stricter and clearer | safety benchmark |
| Maintainability | upstream product | upstream engine | analytix contract-first | sync ledger + conformance |

## 14. Acceptance Criteria

An upstream absorption batch is benchmark-complete only when:

- impacted dimensions are listed;
- each impacted dimension has baseline and post-change score;
- evidence is linked or summarized;
- accepted regressions are explicitly justified;
- unresolved gaps have owners or follow-up tasks;
- scorecard and sync ledger agree on absorbed/rejected/deferred items.

A release is not allowed to claim "stronger than Kun and Reasonix" unless:

- product workflow score is stronger than Kun baseline for target surfaces;
- engine/runtime score is at least Reasonix parity for absorbed engine
  dimensions;
- desktop smoothness score is stronger than both upstreams for long-thread GUI;
- safety score has no known regression;
- benchmark evidence is reproducible or clearly marked as manual QA.

## 15. Not Complete Conditions

Do not call a batch benchmark-complete if:

- it only proves conformance but claims superiority;
- it lacks a baseline;
- it lacks evidence for changed dimensions;
- it ignores a known upstream improvement;
- it improves engine behavior but regresses desktop UX;
- it improves UI but breaks runtime contracts;
- it changes product entry level without target-version parity evidence or an
  independent analytix spec decision;
- it absorbs Reasonix cache/provider logic but lacks prefix stability,
  cache-hit/miss, provider matrix, or non-regression evidence;
- it improves performance by weakening safety, approval, privacy, or data
  durability;
- scorecard and implementation notes disagree.

## 16. Initial Audit

This spec completes the measurement layer missing from spec `08`.

| Need | Covered by |
| --- | --- |
| Prove analytix is stronger, not just compatible | scorecard scale and upstream comparison matrix |
| Prevent blind upstream merges | benchmark artifacts and acceptance criteria |
| Preserve product workflow leadership over Kun | product workflow benchmarks |
| Preserve or exceed Reasonix engine strengths | agent, runtime, provider/cache benchmarks |
| Protect desktop feel | thread smoothness benchmarks |
| Protect trust | safety benchmarks |
| Keep Go runtime honest | Go backend benchmark gates |

Initial result:

```text
The benchmark layer is complete enough to evaluate future upstream absorption.
Actual scores must be produced by future sync batches and release-readiness
runs; this spec defines the measurement contract.
```

## 17. 2026-06-21 Latest Currentness And G2 Evidence Rule

Reasonix latest currentness recheck:

```text
Previous Reasonix governance baseline: 49c14762b7da9234525e717830e39a64a2220911
Latest Reasonix main-v2: 91fe06db6177bb052fc8ae1a3081d60bfc104a4e
Latest substantive deltas: codebase-memory MCP auto-indexing, MiMo built-ins
to custom-provider migration.
Kun baseline remains: 0.2.13 -> 0.2.14 product-entry baseline with master
8602476 and develop 247076f unchanged.
```

Benchmark effect:

| Dimension | Current score effect | Why |
| --- | --- | --- |
| Tool and MCP reliability | Evidence readiness improves; full parity score does not advance yet. | Reasonix codebase-memory auto-indexing identifies a fixture target for cwd-aware indexer MCP startup under shared hosts. This stage adds generic MCP/tool lifecycle fixture coverage for connect/disconnect/reload/cancel/error and secret-safe diagnostics, but not the specific codebase-memory auto-index product behavior. |
| Provider/cache/cost efficiency | No score increase. | MiMo/custom-provider migration is a provider-config input, but analytix keeps Xiaomi/MiMo product presets and lacks live credentialed provider evidence. |
| Go runtime kernel | G2 shadow-readiness advances; Go backend parity does not. | The shared G2 oracle now validates TypeScript HTTP/SSE routes and Go shadow replay for list/search/archive/read/update/fork/resume-thread and SSE `since_seq`; no Electron integration or default backend exists. |
| Product workflow governance | Remains guarded. | Kun 0.2.13/0.2.14 still do not justify a top-level Workflow route; Reasonix latest drift does not change product entry placement. |
| Release strength | No score increase. | Live provider matrix, packaged QA, Windows NSIS, signing/notarization, release metadata, and verified remote remain blockers. |

Scoring rule:

```text
Currentness scans and G2 shadow route replay are evidence infrastructure. They
may justify scorecard rows under maintainability or readiness, but they must
not raise a parity/superiority score for provider/cache, MCP reliability, Go
backend runtime, or release strength until implementation, repeatable tests,
and required live/provider/desktop evidence exist.
```

Required scorecard wording for this stage:

```text
Use "shadow-closed", "record-only", "blocked", or "candidate" for Reasonix
latest MCP/provider deltas and Go G2 route replay. Do not use "backend parity",
"superiority", or "release-ready" unless the corresponding tests and release
gates are actually complete.
```

## 18. 2026-06-21 P0 Engine Parity And Desktop Hardening Score Rule

This stage advances scoped benchmark evidence for Reasonix P0 engine parity and
Kun desktop hardening, but it does not advance release-readiness or live
superiority scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache/cost efficiency | fixture score advances; live score unchanged | Provider/cache oracle covers DeepSeek prompt cache hit/miss, OpenAI-compatible and Responses nested cached tokens, Anthropic cache read/create, reasoning tokens, canonical schema hash, sanitized request attribution, and unsupported unknown fallback. | Credentialed live provider matrix, write-inline/scheduled detector/provider-probe live evidence, and cost reconciliation remain blockers. |
| Tool and MCP reliability | fixture score advances | MCP lifecycle oracle and provider tests now include codegraph/codebase-memory known override, workspace-root cwd fallback, explicit cwd preservation, background/low-priority diagnostics, cancel/timeout, schema cache, and secret redaction. | Live MCP operations and desktop indexer lifecycle QA remain blockers. |
| Runtime contract reliability | fixture score advances | Task/background/planner oracle covers foreground/background continuation, wait/output/kill, parallel dependency/cycle validation, nested event metadata, permission inheritance, transcript continue/fork, and planner read-only toolset. SSE IPC tests cover 100ms throttle, reconnect `since_seq`, and destroyed-renderer safe stop. | Restart/crash desktop drill, packaged long-thread QA, and live nested-card QA remain blockers. |
| Go runtime kernel | G3/G4 shadow-readiness advances; backend score unchanged | G3 provider streaming/usage/cache and G4 tools/approval/user-input/MCP fixtures are TS-owned and Go shadow output matches them. | No default Go backend, G5 full loop, rollback, packaged Go QA, or Electron integration exists. |
| Product workflow / desktop | desktop hardening evidence improves | Shell-level sidebar/back/forward/new-chat controls preserve Code/Write/Settings/Plugins/Connect Phone/Schedule entry baseline and reduce duplicate page toggles. | Full packaged desktop QA and Local Whisper/tray session menu are deferred. |

Allowed wording:

```text
Scoped Reasonix P0 engine parity fixtures are closed for provider/cache,
task/background/planner oracle, MCP known override, and Go G3/G4 shadow output.
Kun desktop hardening is closed for SSE IPC throttle and shell navigation
baseline. The stage remains blocked for live provider/MCP QA, packaged desktop
QA, release readiness, and default Go backend.
```

Forbidden wording:

```text
Do not claim Reasonix full parity, stronger-than-Reasonix provider/cache,
live MCP parity, release-ready desktop, Go backend parity, or default Go backend
from this stage.
```

## 19. 2026-06-21 Reasonix 9e56 Runtime Parity Score Rule

This rule refines section 18 after the Reasonix
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b`
currentness check.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Tool and MCP reliability | fixture score may advance | Retry-all failed MCP startup servers and suspended-source late reconnect are now covered by runtime tests, while codegraph/codebase-memory cwd/priority diagnostics remain covered. | Live MCP/indexer operation, desktop lifecycle QA, and Reasonix plugin protocol parity are not claimed. |
| Runtime contract reliability | internal-runtime score may advance | Authenticated `/v1/runtime/task-jobs/wait|output|kill` routes make the task-job manager reachable through the serve runtime without adding top-level Workflow/Subagent/AutoResearch routes. | First-class Reasonix `task`, `parallel_tasks`, planner/executor Coordinator, restart/resume crash drill, and desktop nested-card QA remain missing. |
| Provider/cache/cost efficiency | diagnostic safety improves; live score unchanged | Provider probe/model-list failure diagnostics redact URL, body excerpt, and network-error secrets; provider-cache oracle remains offline/golden. | Live provider credentials, write-inline/scheduled/probe live matrix, and cost reconciliation remain blockers. |
| Product workflow / desktop | shell safety improves; product-entry score unchanged | Native titlebar safe-area and shell navigation controls are tested and committed separately. | Local Whisper and tray session menu remain deferred; no new top-level Workflow/Create Loop entry is allowed. |
| Go runtime kernel | no score increase | No Go code changed in this addendum; G5 remains an oracle inventory requirement. | Full Go agent loop, jobs, cache, compaction, resume/interrupt, rollback, packaged QA, and default backend remain absent. |

Allowed wording:

```text
MCP startup fixture parity and internal task-job runtime routes have advanced.
Provider probe diagnostics are secret-safe. The Kun product-entry baseline and
default TypeScript runtime remain intact.
```

Forbidden wording:

```text
Do not claim full Reasonix task/parallel/planner parity, live provider/cache
superiority, live MCP/indexer parity, Go G5 parity, release readiness, or a
default Go backend from this score change.
```

## 20. 2026-06-21 Collaborative Execution / Cache Curve / MCP Indexer / Go G5 Score Rule

This score rule covers the post-9e56 closure stage that first closed the dirty
shell safe-area baseline, then advanced TypeScript-owned runtime oracles.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Collaborative execution reliability | oracle score advances | `TASK_TOOL_CONTRACT`, `PARALLEL_TASKS_TOOL_CONTRACT`, `task-job-orchestration-oracle.json`, and tests now cover internal task/parallel contracts, `depends_on` normalization, duplicate/self/unknown/cycle rejection, background wait/output/kill, nested SSE metadata, parent Goal evidence keys, and transcript continue/fork identity. | No full Reasonix planner/executor Coordinator, restart/crash drill, or desktop nested-card QA. |
| Provider/cache efficiency | fixture score advances | Offline cache curve guard models Reasonix release-cache behavior, while provider matrix still covers DeepSeek, OpenAI Responses, Anthropic, reasoning tokens, canonical tool schema, stable prefix, and unsupported fallback. | No live provider superiority, cost reconciliation, or credentialed write-inline/scheduled/probe/model-list matrix. |
| MCP/indexer reliability | fixture score advances | MCP lifecycle oracle covers retry-all failed startup servers, late suspended-provider tombstone behavior, and codegraph/codebase-memory cwd/low-priority/backgroundStart overrides. | No live MCP/indexer QA or Reasonix plugin protocol parity. |
| Go runtime kernel | G5 readiness advances; backend score unchanged | `go-g5-full-loop-oracle.json` and manifest binding freeze TS-owned full-loop/job/cache/compaction/resume/interrupt/MCP oracle inventory. | No Go full loop, Electron main integration, rollback, packaged QA, or default backend. |
| Product workflow / desktop | shell safety improves; product-entry score unchanged | Dirty native titlebar/safe-area closure is separately validated and committed. Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/Connect Phone/Schedule. | Tray session menu and Local Whisper remain future import-boundary/QA work. |

Allowed wording:

```text
This stage improves fixture/oracle readiness and internal runtime reliability.
It may say DeepSeek cache proof is Reasonix-style and multi-provider oracle
coverage is broader. It may not increase live parity, release, or default
backend scores.
```

Forbidden wording:

```text
Do not claim full Reasonix collaborative-execution parity, live provider/cache
superiority, live MCP/indexer parity, Kun tray/Local Whisper parity, Go G5
runtime parity, release readiness, or a default Go backend.
```

## 21. 2026-06-21 Executable Internal Runtime And Desktop Tray Score Rule

This score rule covers the executable/internal-runtime slice after section 20.
It may improve implementation depth inside already capped fixture/readiness
scores, but it must not raise live parity, release, or default-backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Collaborative execution reliability | implementation depth improves; capped fixture score unchanged | Internal `task` and `parallel_tasks` tools now execute through runtime providers, prove dependency order, support output offsets, and reconcile stale queued/running jobs after restart. | Full Reasonix planner/executor Coordinator, durable runner rehydration, and desktop nested-card QA. |
| Provider/cache efficiency | fixture depth improves; live score unchanged | Runtime-owned cache curve guard plus DeepSeek native hit/miss precedence over conflicting generic cache telemetry. | Live provider/cache superiority, cost reconciliation, and credentialed write-inline/scheduled/probe/model-list matrix. |
| MCP/indexer reliability | fixture depth improves; live score unchanged | Fake live-local indexer proof covers retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics. | Live MCP/indexer operation and Reasonix plugin protocol parity are not claimed. |
| Product workflow / desktop | desktop affordance improves; product-entry score unchanged | Tray session menu is analytix-native, main-process-only, and opens existing runtime threads without renderer route or bridge changes. | Packaged tray QA and Local Whisper import-boundary QA. |
| Go runtime kernel | no score increase | No Go files changed because `go` is unavailable locally and Go package tests cannot be run. | G5 jobs/cache/session/resume/interrupt/MCP shadow implementation, backend selection, rollback, packaged QA, and default backend. |

Allowed wording:

```text
Executable internal runtime proof, cache-guard depth, fake live-local MCP
fixture depth, and tray session menu absorption are complete for this stage.
```

Forbidden wording:

```text
Do not claim full Reasonix collaborative execution parity, live DeepSeek cache
superiority, live MCP/indexer parity, Go G5 runtime parity, release readiness,
Kun identity, Reasonix public protocol, or a default Go backend.
```

## 22. 2026-06-21 Go G5 Shadow Output Score Rule

This rule covers the first Go G5 shadow implementation slice. It improves
shadow evidence depth but does not change backend, release, or live parity
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | shadow depth improves; backend score unchanged | `packages/runtime-go/shadow_g5.go` replays the G5 full-loop oracle inventory into a comparable Go output, and `go test ./...` passes with a temporary SHA-verified official Go toolchain. | No live Go full-loop executor, jobs/cache/session/MCP implementation, Electron integration, rollback/default-backend plan, or packaged QA. |
| Currentness governance | readiness maintained | Reasonix bfe398 PowerShell compatibility drift is classified as shell/sandbox record/future input. | Superseded by section 24 for runtime shell fixtures; native Windows host/package QA remains future scoped work. |

Allowed wording:

```text
Go G5 shadow output slice is implemented and fixture-verified.
```

Forbidden wording:

```text
Do not claim Go G5 runtime parity, live provider/cache superiority, live
MCP/indexer parity, release readiness, or a default Go backend.
```

## 23. 2026-06-21 Go G5 Composite Shadow Replay Score Rule

This rule covers cross-fixture G5 replay. It improves conformance depth but
does not change live/runtime/backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | shadow evidence improves; backend score unchanged | Go computes G5 `shadowSlicesExpectedOutput` from task-job, provider-cache, G2 route replay, and MCP lifecycle fixtures. | No live Go runtime, no Electron integration, no rollback/default-backend plan, no packaged QA. |
| Provider/cache and MCP fixture discipline | cross-fixture evidence improves | G5 replay binds cache release guard and MCP live-local/tombstone/redaction details into one Go output. | Live provider/cache superiority and live MCP/indexer parity remain unclaimed. |

Allowed wording:

```text
G5 composite shadow replay is fixture-verified across jobs/cache/session/MCP.
```

Forbidden wording:

```text
Do not claim Go G5 parity, live superiority, release readiness, or default Go
backend readiness.
```

## 24. 2026-06-21 Planner Executor / Shell / Live-Local / G5 Runner Score Rule

This score rule covers the scoped executable runtime gate that follows section
23. It improves fixture depth and internal runtime reliability. It must not
raise live provider, live MCP, release, or default-backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | implementation depth improves inside capped fixture score | Planner-readonly/executor coordinator proof covers DAG waves, dependency-failure skips, cancellation propagation, output offsets, parent metadata, and read-only planner tools; durable runner rehydration proves output/wait/kill after restart. | No packaged desktop crash drill, no public Reasonix planner protocol, and no top-level workflow UI. |
| Windows shell reliability | fixture score advances | Runtime shell tests cover standard pwsh path fallback, pwsh chaining support, Windows PowerShell unquoted chaining guard, and quoted literal allowance. | Native Windows host validation and packaged terminal QA remain release gates. |
| Provider/cache efficiency | fixture depth improves; live score unchanged | Executable local DeepSeek-compatible provider proves final `/v1/chat/completions` request path, native hit/miss precedence, cache rate, and reasoning-token parsing. | Credentialed live provider/cache superiority, cost reconciliation, and live write-inline/scheduled/probe/model-list evidence remain blocked. |
| MCP/indexer reliability | fixture depth improves; live score unchanged | Executable stdio fake MCP indexer server proves persistent restart/resume, tombstones, cwd, low-priority/backgroundStart diagnostics, and secret redaction. | No live MCP/indexer parity or Reasonix plugin protocol parity. |
| Go runtime kernel | shadow evidence improves; backend score unchanged | G5 composite replay now includes planner/executor and durable-runner restart fields from TS fixtures. | No live Go full-loop executor, Go Job Manager, Electron integration, rollback/default-backend plan, or packaged QA. |
| Product workflow / desktop | product-entry score unchanged | Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/Connect Phone/Schedule; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level route is added. | Local Whisper and platform package QA remain gated. |

Allowed wording:

```text
Planner/executor, durable runner rehydration, Windows shell compatibility,
live-local provider cache, executable MCP fake indexer, and Go G5 runner shadow
evidence are stronger than the previous fixture layer.
```

Forbidden wording:

```text
Do not claim full Reasonix parity, live provider/cache superiority, live
MCP/indexer parity, Go G5 runtime parity, release readiness, Kun identity,
Reasonix public protocol, or default Go backend readiness.
```

## 25. 2026-06-21 Reasonix 881 Control Score Rule

This rule covers the control gate after section 24. It improves runtime-control
and shadow-conformance depth without changing live provider, live MCP, release,
or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Cancel/batch reliability | implementation depth improves | Completed, failed, cancelled, and unstarted tool results are preserved by call id/order after cancel. | No Reasonix TUI or public control protocol. |
| Task-job reliability | implementation depth improves | Parent abort kills running task jobs and returns completed/killed/skipped aggregate evidence with offsets. | No public Reasonix task protocol or packaged crash evidence. |
| Step-budget governance | contract depth improves | `runtimeTuning.stepLimits`, thread/session overrides, turn overrides, planner/headless handling, and delegation `max_steps` exist without stable-prefix mutation. | No Reasonix config root or post-881 auto-plan behavior. |
| Go runtime kernel | shadow evidence improves; backend score unchanged | G5 `controlReplay` covers cancel, task-job cancel aggregation, and step-limit controls. | No live Go runtime, Electron integration, rollback/default-backend plan, or packaged QA. |
| Product workflow / desktop | product-entry score unchanged | Kun entry baseline remains Code/Write/Settings/Plugins/Connect Phone/Schedule; duplicate preload/loading stashes remain retained for audit. | Local Whisper and platform package QA remain gated. |

Allowed wording:

```text
Reasonix 881 cancel/batch/step-limit control value is implemented behind
analytix runtime contracts and mirrored in Go G5 control shadow evidence.
```

Forbidden wording:

```text
Do not claim full Reasonix parity, post-881 auto-plan parity, live provider or
MCP superiority, Go G5 runtime parity, release readiness, Kun identity,
Reasonix public protocol, default Go backend, or Rust/Tauri migration.
```

## 26. 2026-06-21 Post-881 Router Guard And Executable Control Score Rule

This rule covers the next scoped proof after section 25. It improves internal
classifier-lifecycle, Go shadow, and Kun-provider-default evidence, but does
not raise live provider/MCP/release/default-backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Classifier/currentness governance | implementation guard improves | `AUTO_MODEL_ROUTER_FINGERPRINT` is derived from classifier model, prompt, timeout, response shape, max tokens, temperature, and reasoning effort; auto route cache keys include that fingerprint. | No Reasonix public auto-plan setting or post-881 auto-plan parity. |
| Go runtime kernel | shadow evidence improves; backend score unchanged | Go computes fixture-owned cancel, task-job cancellation, and step-limit executable cases instead of only echoing `controlReplay`. | No live Go full-loop executor, Electron integration, rollback/default-backend plan, or packaged QA. |
| Kun provider baseline | provider-default parity improves | Unknown explicit text model profiles default to `128_000` context tokens, matching Kun 0.2.14 and runtime defaults; explicit context windows still override. | Broader Kun Local Whisper/package QA remains gated. |
| Provider/cache live evidence | no score change | No provider/cache request body or usage parsing behavior changed. | Credentialed live matrix and cost reconciliation remain blockers. |

Allowed wording:

```text
Post-881 Reasonix classifier lifecycle value is represented as an
analytix-owned auto-router fingerprint guard; Go G5 control proof is now
executable shadow evidence; Kun 0.2.14 provider context defaults are aligned.
```

Forbidden wording:

```text
Do not claim post-881 auto-plan parity, live provider/cache superiority, live
MCP parity, Go runtime parity, default Go backend, release readiness, Reasonix
public protocol, Kun identity, or Rust/Tauri migration.
```

## 27. 2026-06-21 Approval/User-Input Abort Cleanup Score Rule

This rule covers the scoped approval/user-input cleanup proof after section 26.
It improves runtime contract reliability and replay safety, but does not raise
live provider/MCP, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Safety and approval | implementation depth improves | `ApprovalGate.expire()` clears pending approvals on turn abort, emits `approval_resolved: expired`, rejects late allow/deny, and preserves denied/no-execute boundaries. | No full Reasonix approval-manager or renderer live-card parity claim. |
| Runtime contract reliability | replay evidence improves | `approval-user-input-route-oracle.json` pins cleanup replay kinds plus late approval 409 and late user-input 404 behavior. | Crash/restart GUI card QA remains separate. |
| User input | implementation depth improves | Abort while awaiting `request_user_input` records `user_input_resolved: cancelled` and clears the gate. | No new public user-input protocol or AutoResearch/Subagent UI. |
| Renderer live mapping | implementation depth improves | `approval_resolved: expired` updates existing approval cards through the live mapper instead of leaving them pending. | Full desktop live-card QA remains separate. |
| Go runtime kernel | no score increase | G3/G4/G5 conformance focused tests remain green; no Go files changed. | No live Go approval/user-input runtime or default backend. |
| Product boundary governance | unchanged but reaffirmed | No Reasonix SessionAPI, public protocol, top-level Workflow/Subagent route, Kun identity, Rust/Tauri path, or default Go backend is added. | Full final release evidence still requires complete gate reruns. |

Allowed wording:

```text
Analytix has stronger fixture-backed approval/user-input abort cleanup and SSE
replay safety than the previous gate.
```

Forbidden wording:

```text
Do not claim full Reasonix parity, Reasonix approval-manager parity, live
provider/cache superiority, live MCP parity, Go runtime parity, release
readiness, default Go backend, Reasonix public protocol, Kun identity, or
Rust/Tauri migration.
```

## 28. 2026-06-21 Renderer Approval Live-Card Store Score Rule

This rule covers renderer store-level proof after section 27. It improves
desktop-runtime evidence for approval resolution, but it does not raise release
readiness, live provider/MCP, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Renderer live approval cards | implementation evidence improves | Main-thread `buildThreadEventSink` updates an existing approval block from `onApprovalStatus` instead of leaving it pending or duplicating it. | Full visual desktop click-through and packaged crash/restart QA remain separate. |
| Side/nested execution UI | scoped evidence improves | Side conversation SSE sink updates the side approval card and leaves main thread blocks untouched. | Full nested event rendering QA remains separate. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider, Go, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Approval resolved-state evidence now reaches renderer store live cards for both
main chat and side conversations.
```

Forbidden wording:

```text
Do not claim full Reasonix frontend approval parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 29. 2026-06-21 User-Input Live-Card Store Score Rule

This rule covers renderer store-level proof for user-input resolved events
after section 28. It improves desktop-runtime evidence for GUI gate closure,
but it does not raise release readiness, live provider/MCP, or Go backend
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Renderer live user-input cards | implementation evidence improves | Main-thread `buildThreadEventSink` updates an existing user-input block from `onUserInputStatus` by item id or input/request id. | Full visual desktop click-through and packaged crash/restart QA remain separate. |
| Side/nested execution UI | scoped evidence improves | Side conversation user-input cards use runtime item ids and side status updates stay scoped to side blocks. | Full nested visual rendering QA remains separate. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider, Go, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
User-input resolved-state evidence now reaches renderer store live cards for
both main chat and side conversations.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input frontend parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 30. 2026-06-21 Planner Gating For Internal Task Tools Score Rule

This rule covers internal task/parallel planner gating after section 29. It
improves collaborative-execution safety evidence, but it does not raise release
readiness, live provider/MCP, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Collaborative execution safety | implementation evidence improves | Plan mode does not advertise internal `task` / `parallel_tasks`, and forged `task` calls are rejected without executing child work. | Full Reasonix planner/executor Coordinator and desktop nested-card QA remain separate. |
| Runtime oracle readiness | conformance evidence improves | `task-job-orchestration-oracle.json` now pins `plannerForbiddenToolset` for TS and future Go gates. | Go full-loop executor, rollback, and packaged QA remain absent. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider, Go, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix planner gating is stronger: internal child-job tools cannot leak into
Plan mode or run through forged calls.
```

Forbidden wording:

```text
Do not claim full Reasonix planner/executor parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 31. 2026-06-21 Go G5 Planner-Forbidden Shadow Replay Score Rule

This rule covers Go G5 shadow consumption of the planner-forbidden task-tool
oracle after section 30. It improves cross-backend conformance evidence, but
does not raise live Go backend, provider/MCP, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | shadow evidence improves; backend score unchanged | Go G5 `jobReplay` now includes `plannerForbiddenToolset` sourced from the TS task-job oracle. | No live Go planner/executor, jobs, providers, sessions, MCP, Electron integration, rollback, or packaged QA. |
| Runtime oracle readiness | conformance evidence improves | `go-g5-full-loop-oracle.json`, schema, and TS test all assert the forbidden toolset. | Still shadow-only and not a public runtime protocol. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider, Go default, Rust/Tauri, Kun identity, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Go G5 shadow now carries the same planner-forbidden task-tool evidence as the
TypeScript runtime oracle.
```

Forbidden wording:

```text
Do not claim Go runtime parity, full Reasonix planner/executor parity, live
provider/cache superiority, live MCP parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 35. 2026-06-21 Write-Inline Custom Full Endpoint Score Rule

This rule covers custom full endpoint write-inline proof after section 34. It
improves provider request-surface evidence, but it does not raise live
provider/cache, cost, release-readiness, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | regression evidence improves | `write-inline-completion-service.test.ts` proves custom `/responses` and `/messages` URLs are used exactly and keep their matching body/header/parser shape. | Credentialed live provider matrix remains blocked. |
| Provider/cache/cost efficiency | fixture evidence improves; live score unchanged | Custom provider request shape no longer depends only on generic endpointFormat tests. | No live cache superiority or cost reconciliation. |
| Product boundary governance | unchanged but reaffirmed | No Reasonix provider protocol, settings schema change, provider default change, Go/Rust/Tauri path, Kun identity, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Write-inline now has fixture-backed proof for custom full endpoint Responses
and Messages request surfaces.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 36. 2026-06-21 Auto-Model Route Cache Lifecycle Score Rule

This rule covers loop-level auto model route cache proof after section 35. It
improves classifier currentness evidence, but it does not raise Reasonix
auto-plan parity, release-readiness, live provider/cache, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Agent routing quality | implementation evidence improves | `loop.test.ts` proves a multi-step `model:"auto"` turn reuses one classifier route while the next turn re-routes. | No public auto-plan setting or controller API. |
| Cache/currentness safety | fixture evidence improves; live score unchanged | Route cache remains turn-scoped and does not enter stable prefix state. | Live cost/cache superiority remains blocked. |
| Product boundary governance | unchanged but reaffirmed | No Reasonix config/protocol, settings schema change, provider default change, Go/Rust/Tauri path, Kun identity, or top-level Workflow/Subagent entry was added. | Release readiness and full Reasonix parity remain blocked. |

Allowed wording:

```text
Analytix auto model routing has loop-level currentness proof for same-turn reuse
and next-turn recalculation.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan parity, user-level auto-plan settings,
project/local auto-plan overrides, live provider/cache superiority, release
readiness, default Go backend, Reasonix public protocol, Kun identity, or
Rust/Tauri migration.
```

## 37. 2026-06-21 Provider Request-Shape Oracle Matrix Score Rule

This rule covers provider request-shape oracle coverage after section 36. It
improves provider non-regression and Go shadow readiness evidence, but it does
not raise live provider/cache, cost, release-readiness, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | implementation evidence improves | `provider-cache-proof.test.ts` executes request-shape cases from `provider-cache-oracle.json` against `CompatModelClient`. | Credentialed live provider matrix remains blocked. |
| Provider/cache fixture readiness | fixture evidence improves; live score unchanged | DeepSeek/OpenAI-compatible/Responses/Anthropic/custom request shape cases now sit beside cache usage and cache curve fixtures. | No live cache superiority or cost reconciliation. |
| Go/runtime oracle readiness | shadow evidence improves; backend score unchanged | G3/G5 shadow output carries request-shape case ids without implementing a Go provider client. | No live Go provider/session/MCP/runtime integration. |
| Product boundary governance | unchanged but reaffirmed | No Reasonix provider protocol, settings schema change, provider default change, Go/Rust/Tauri path, Kun identity, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Provider/cache proof now includes a centralized request-shape oracle matrix for
major endpoint families and custom full endpoint mode.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 34. 2026-06-21 Structured User-Input Choice Validation Score Rule

This rule covers malformed structured GUI input requests after section 33. It
improves control-safety evidence and G4 oracle readiness, but it does not raise
desktop live QA, release readiness, or Go backend scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Approval/user-input control safety | implementation evidence improves | `request_user_input` rejects malformed structured choices before opening a pending GUI gate. | Packaged desktop visual/live user-input QA remains separate. |
| Runtime oracle readiness | conformance evidence improves | G4 fixture/schema/test and Go shadow output record invalid cases, `invalid_user_input_request`, and `opensGateOnInvalid:false`. | Go live approval/user-input manager remains absent. |
| Product boundary governance | unchanged but reaffirmed | No Reasonix ask protocol, bridge/settings/provider change, default Go backend, Rust/Tauri path, Kun identity, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix has G4-backed runtime proof that malformed structured GUI input
choices fail before user-input gates are opened.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input parity, desktop live QA, live
provider/cache superiority, live MCP parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 32. 2026-06-21 Write-Inline Provider Request-Surface Score Rule

This rule covers write-inline provider request-surface proof after section 31.
It improves provider non-regression evidence, but it does not raise live
provider/cache or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache/cost efficiency | fixture evidence improves; live score unchanged | `write-inline-completion-service.test.ts` pins OpenAI Responses URL/body/header shape and Anthropic Messages URL/body/header/parser shape. | Credentialed live provider matrix, cache superiority, and cost reconciliation remain blocked. |
| Provider request safety | regression evidence improves | Responses tests reject Anthropic header leakage; Messages tests prove the Anthropic body and parser path. | Scheduled/probe/model-list live checks remain separate. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider default, Go default, Rust/Tauri, Kun identity, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Write-inline now has fixture-backed provider request-surface proof for
Responses and Messages endpoint formats.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 33. 2026-06-21 Go G5 Planner Tool-Policy Executable Shadow Score Rule

This rule covers planner tool-policy execution inside the Go G5 control shadow.
It improves cross-backend conformance evidence, but it does not raise live Go
backend, provider/MCP, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | executable shadow evidence improves; backend score unchanged | `BuildG5ControlExecutableOutput` now computes planner step 0 / step 1 advertised tools and forged `task` rejection. | No live Go planner/executor, Job Manager, providers, sessions, MCP, Electron integration, rollback, or packaged QA. |
| Collaborative execution safety | cross-backend evidence improves | Planner forbidden task tools are no longer only replayed in `jobReplay`; they are exercised by a Go control case. | Full Reasonix planner/executor Coordinator and desktop nested-card QA remain separate. |
| Product boundary governance | unchanged but reaffirmed | No public route, bridge, settings, provider default, Go default, Rust/Tauri, Kun identity, or top-level navigation surface changed. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Go G5 executable shadow now proves the planner read-only/create_plan gate and
forged internal task rejection.
```

Forbidden wording:

```text
Do not claim Go runtime parity, full Reasonix planner/executor parity, live
provider/cache superiority, live MCP parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 38. 2026-06-21 Kun Top-Level Route-Surface Oracle Score Rule

This rule covers renderer route-surface evidence. It improves product-boundary
governance, but it does not raise release readiness, live desktop QA, Kun
current parity, or Reasonix orchestration parity scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product boundary governance | evidence improves; score unchanged | `Workbench.route-surface.test.ts` scans Workbench, shell navigation, sidebar, `AppRoute`, and app actions for forbidden top-level entries. | Packaged desktop QA and future product-entry specs remain separate. |
| Kun target-version fidelity | evidence improves; score unchanged | `chat-store-app-actions.test.ts` proves no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route/action is available. | Does not prove full Kun current parity. |
| Reasonix capability absorption safety | evidence improves; score unchanged | Internal task/research/planner capabilities remain allowed only when hidden behind analytix runtime contracts. | Full Reasonix orchestration parity remains open. |

Allowed wording:

```text
Analytix has route-surface tests proving Kun-target-absent orchestration
features are not top-level navigation entries.
```

Forbidden wording:

```text
Do not claim full Kun current parity, full Reasonix planner/subagent parity,
new product navigation, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 39. 2026-06-21 Auto-Route Step/Cancel Control Composition Score Rule

This rule covers the composed control oracle after isolated auto-route,
step-limit, and cancellation evidence. It improves control-safety and G5 shadow
readiness, but it does not raise live Go backend, live provider/cache, packaged
QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Control-safety composition | evidence improves; score unchanged | `loop.test.ts` proves auto-route reuse until a user-global step limit without stable-prefix drift. | Does not prove packaged desktop interruption UX. |
| Go/runtime oracle readiness | shadow evidence improves; backend score unchanged | `go-g5-full-loop-oracle.json` and Go shadow compute combined cache/step/cancel output. | No live Go loop, providers, sessions, MCP, rollback, or Electron integration. |
| Cache determinism | evidence improves; score unchanged | Dynamic route/step state is asserted outside stable prefix/context instructions. | Live provider/cache superiority and cost reconciliation remain blocked. |

Allowed wording:

```text
Analytix has composed runtime/G5 evidence for auto-route cache, step-limit, and
cancel-result preservation.
```

Forbidden wording:

```text
Do not claim live Go backend readiness, Reasonix auto-plan parity, live
provider/cache superiority, live MCP parity, release readiness, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 40. 2026-06-21 Preload Bridge/API Sovereignty Score Rule

This rule covers the source-level preload/API ownership oracle. It improves
product-boundary governance, but it does not raise live runtime, provider,
Go-backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product boundary governance | evidence improves; score unchanged | `preload-sandbox.test.ts` proves preload exposes only `analytix`, `Window` declares only `analytix`, and shared public API names stay analytix-owned. | Packaged desktop QA and future bridge changes remain separate. |
| Kun target-version fidelity | evidence improves; score unchanged | Deprecated Kun GUI bridge aliases and public facade names cannot return without failing tests. | Does not prove full Kun current parity. |
| Reasonix absorption safety | evidence improves; score unchanged | Reasonix SessionAPI/frontend protocol names remain rejected at the public renderer boundary. | Does not prove full Reasonix frontend/session parity. |

Allowed wording:

```text
Analytix has executable preload/API evidence that renderer bridge ownership
remains `window.analytix`.
```

Forbidden wording:

```text
Do not claim a new public bridge, full Reasonix SessionAPI parity, full Kun
current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 41. 2026-06-21 Renderer Thread Lifecycle HTTP Score Rule

This rule covers the renderer runtime adapter thread lifecycle oracle. It
improves runtime-contract reliability evidence, but it does not raise packaged
desktop QA, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `analytix-runtime.test.ts` pins list/search/archive/restore/rename/workspace/delete to analytix `/v1/threads` calls. | Packaged desktop sidebar/thread-list QA remains separate. |
| Reasonix absorption safety | evidence improves; score unchanged | Renderer thread lifecycle does not use Reasonix SessionAPI or public thread/session protocol. | Does not prove full Reasonix session frontend parity. |
| Kun target-version fidelity | evidence improves; score unchanged | The test prevents lifecycle drift to Kun public protocol or deprecated bridge assumptions. | Does not prove full Kun current parity. |

Allowed wording:

```text
Analytix has renderer adapter proof that thread lifecycle actions stay on the
analytix HTTP runtime contract.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, full Reasonix SessionAPI parity, full Kun
current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 42. 2026-06-21 Settings/Provider EndpointFormat Persistence Score Rule

This rule covers endpoint-format persistence in desktop settings. It improves
settings/provider non-regression evidence, but it does not raise live
provider/cache, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `settings-store.test.ts` pins runtime and provider endpoint-format persistence before provider request-shape oracles consume those settings. | Credentialed provider matrix remains separate. |
| Product boundary governance | evidence improves; score unchanged | Persisted settings omit legacy `agentProvider` and `agents` envelopes. | Does not prove packaged settings UI QA. |
| Reasonix/Kun absorption safety | evidence improves; score unchanged | Endpoint format does not depend on Reasonix config roots or Kun/deprecated settings identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has settings-store evidence that endpoint-format choices persist in
analytix-owned settings.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 65. 2026-06-22 D-0063 Go G5 Provider Cache Release Guard Shadow Score Rule

This rule covers the provider cache release guard G5 executable shadow proof. It
improves offline provider/cache evidence, but it does not raise live provider,
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider cache safety | evidence improves; score unchanged | G5 fixture and TS conformance derive release guard output from the analytix offline cache curve guard. | Credentialed provider matrix remains separate. |
| Go G5 gate quality | executable shadow evidence improves; backend score unchanged | Go shadow computes tail averages, statuses, collapse counts, low-tail allowance, and overall pass. | No Go provider client, route, rollback, or default backend. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, renderer-visible Go route, bridge/settings fallback, Kun identity, live superiority claim, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence that the offline provider cache release guard is
computed consistently across TypeScript and Go fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, default Go backend, Reasonix
provider protocol parity, renderer-visible Go routes, credentialed provider
matrix, packaged provider QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 66. 2026-06-22 D-0064 Auto-Route Step/Cancel Composition Score Rule

This rule covers the AgentLoop auto-route cache, step-limit, and cancellation
composition proof. It improves runtime reliability evidence, but it does not
raise packaged QA, live provider, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `loop.test.ts` proves auto route is reused across a second step before a parallel tool batch is interrupted. | Packaged interruption QA remains separate. |
| Cache/prefix hygiene | evidence improves; score unchanged | Step-limit metadata reaches tool context but stays out of stable prefix and model-visible context. | Live provider/cache superiority remains separate. |
| Product sovereignty | unchanged | The proof adds no Reasonix auto-plan setting, controller protocol, renderer-visible Go route, Kun identity, or hidden top-level capability entry. | Product auto-plan parity remains rejected/deferred. |

Allowed wording:

```text
Analytix has AgentLoop test evidence that auto-route cache, step-limit metadata,
and cancelled parallel tool results compose safely.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, planner enable/disable product
controls, Reasonix controller/SessionAPI protocol parity, live provider/cache
superiority, default Go backend, packaged desktop QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 67. 2026-06-22 D-0065 Planner Step-Limit Score Rule

This rule covers the AgentLoop planner-specific step-limit proof. It improves
runtime/planner reliability evidence, but it does not raise packaged QA, Go
backend, product planner, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `loop.test.ts` proves plan-mode uses `plannerMaxModelSteps` and records the planner budget in the error event. | Packaged plan-mode QA remains separate. |
| Cache/prefix hygiene | evidence improves; score unchanged | Planner budget state stays out of stable prefix and model-visible context. | Live provider/cache superiority remains separate. |
| Product sovereignty | unchanged | The proof adds no Reasonix planner setting, controller protocol, renderer-visible Go route, Kun identity, or hidden top-level capability entry. | Product planner toggles remain rejected/deferred. |

Allowed wording:

```text
Analytix has AgentLoop test evidence that planner-specific step limits apply to
plan-mode turns without polluting prompt/cache surfaces.
```

Forbidden wording:

```text
Do not claim Reasonix planner enable/disable product controls, Reasonix
controller/SessionAPI protocol parity, live provider/cache superiority, default
Go backend, packaged desktop QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 45. 2026-06-21 Forbidden Public Runtime Route Score Rule

This rule covers runtime HTTP negative-route proof. It improves product-boundary
evidence, but it does not raise live Go/backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `http-server.test.ts` proves forbidden upstream public routes return structured 404s. | Packaged desktop QA remains separate. |
| Product boundary governance | evidence improves; score unchanged | Reasonix routes, renderer-visible Go routes, and forbidden top-level capability routes stay absent. | Any future intentional route must update specs/ledger. |
| Reasonix/Kun/Go absorption safety | evidence improves; score unchanged | Behavior is absorbed only behind analytix contracts; public protocol and default Go backend remain rejected. | Full live Go parity and release readiness remain unproven. |

Allowed wording:

```text
Analytix has runtime HTTP negative-route proof that forbidden public routes stay
absent.
```

Forbidden wording:

```text
Do not claim Reasonix public protocol parity, Kun public protocol parity, live
Go backend readiness, default Go backend, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 46. 2026-06-21 Rehydrated Task-Job Route Score Rule

This rule covers route-level restart continuity for internal task jobs. It
improves sub-agent/job orchestration evidence, but it does not raise live Go,
packaged desktop restart, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `task-job-orchestration-oracle.test.ts` proves output/wait/kill routes still work after rehydrate. | Packaged desktop restart QA remains separate. |
| Sub-agent/job orchestration | evidence improves; score unchanged | Durable task jobs stay controllable behind authenticated analytix routes. | No top-level Subagent route or public Reasonix protocol. |
| Reasonix/Go absorption safety | evidence improves; score unchanged | G5 durable-runner behavior is strengthened while Go remains shadow-only. | Live Go backend readiness remains unproven. |

Allowed wording:

```text
Analytix has runtime HTTP harness proof that rehydrated task jobs remain
operable through authenticated internal routes.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow routes, live Go backend readiness, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 47. 2026-06-21 Task-Job Approval Deny Score Rule

This rule covers approval-before-side-effect proof for internal task-job
providers. It improves safety evidence, but it does not raise packaged desktop
approval, live Go, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Approval/user-input safety | evidence improves; score unchanged | `task-job-orchestration-oracle.test.ts` proves denied `task` and `parallel_tasks` approvals create no durable jobs or child runs. | Packaged desktop approval-card QA remains separate. |
| Sub-agent/job orchestration | evidence improves; score unchanged | Real task-job providers stop at the permission gate. | No public Reasonix protocol or top-level Subagent route. |
| Reasonix/Go absorption safety | evidence improves; score unchanged | Go future work must preserve approval-before-side-effect ordering. | Live Go approval manager remains unproven. |

Allowed wording:

```text
Analytix has runtime tool-host proof that denied task-job approvals create no
durable jobs or child runs.
```

Forbidden wording:

```text
Do not claim packaged desktop approval QA, Reasonix public sub-agent/job
protocol parity, top-level Subagent or Workflow routes, live Go backend
readiness, default Go backend, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 48. 2026-06-21 Go G5 Task-Job Approval Deny Shadow Replay Score Rule

This rule covers Go G5 shadow consumption of task-job approval deny no-execute
evidence. It improves Go gate quality, but it does not raise live Go/backend,
packaged approval, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | shadow evidence improves; backend score unchanged | `go-g5-full-loop-oracle.json` and Go shadow output now carry `approvalDenyNoExecute`. | No live Go approval manager, Go Job Manager, Electron integration, rollback, or default backend. |
| Approval/user-input safety | evidence improves; score unchanged | Go G5 must now replay the TS-owned deny/no-side-effect contract. | Packaged desktop approval-card QA remains separate. |
| Reasonix/Go absorption safety | evidence improves; score unchanged | Reasonix-style orchestration remains behind analytix permission gates and fixture-owned Go shadow replay. | No Reasonix public protocol or top-level Subagent route. |

Allowed wording:

```text
Analytix Go G5 shadow replay consumes TS-owned task-job approval denial
evidence and preserves the no-side-effect contract in fixtures.
```

Forbidden wording:

```text
Do not claim Go approval manager readiness, live Go task/job execution, Go G5
runtime parity, default Go backend, packaged desktop approval QA, Reasonix
public sub-agent/job protocol, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 49. 2026-06-21 Go G5 Task Approval Deny Executable Control Score Rule

This rule covers pure Go G5 executable shadow control for task approval denial.
It improves Go gate quality, but it does not raise live Go/backend, packaged
approval, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | executable shadow evidence improves; backend score unchanged | `BuildG5ControlExecutableOutput` now computes `approvalDeny` no-side-effect output. | No live Go approval manager, Go Job Manager, Electron integration, rollback, or default backend. |
| Approval/user-input safety | evidence improves; score unchanged | Go G5 computes denied tool names, approval ids/count, and no durable-job/child-run side effects from the TS oracle. | Packaged desktop approval-card QA remains separate. |
| Reasonix/Go absorption safety | evidence improves; score unchanged | Reasonix-style task orchestration remains analytix-owned and executable only as fixture shadow. | No Reasonix public protocol or top-level Subagent route. |

Allowed wording:

```text
Analytix Go G5 executable shadow computes task approval denial no-side-effect
results from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Go approval manager readiness, live Go task/job execution, Go G5
runtime parity, default Go backend, packaged desktop approval QA, Reasonix
public sub-agent/job protocol, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 50. 2026-06-21 Go G3/G5 Provider Cache Accounting Score Rule

This rule covers fixture-only Go G3/G5 shadow accounting for provider cache
telemetry. It improves provider/cache and Go-gate evidence, but it does
not raise live provider superiority, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | G3 `cacheAccounting` and G5 `cacheReplay.cacheAccounting` compute supported telemetry cases, unsupported unknown cases, hit/miss totals, and aggregate hit rate. | Live credentialed provider matrix remains separate. |
| Go runtime kernel | shadow evidence improves; backend score unchanged | Go G3 output and G5 `cacheReplay` compute accounting from fixture-owned provider usage cases. | No Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/provider absorption safety | evidence improves; score unchanged | DeepSeek/OpenAI/Anthropic semantics stay analytix-owned and fixture-bound; unsupported providers are not counted as misses. | No Reasonix provider protocol or live superiority claim. |

Allowed wording:

```text
Analytix Go G3/G5 shadow computes provider cache accounting from TS-owned
fixtures, including unsupported providers staying unknown.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed DeepSeek/OpenAI/
Anthropic/custom provider parity, Go provider-client readiness, default Go
backend, Reasonix provider protocol, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 51. 2026-06-21 Auto-Router Failure Usage Isolation Score Rule

This rule covers internal auto-router failure fallback and usage/cache
isolation. It improves post-881 Reasonix classifier absorption evidence, but it
does not raise live provider/cache, managed settings rebuild, or release scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Auto-router/currentness | evidence improves; score unchanged | `loop.test.ts` proves classifier failure falls back to heuristic routing. | Managed runtime provider/settings rebuild proof is covered by the later currentness guard; Reasonix controller parity remains rejected. |
| Provider/cache accounting | evidence improves; score unchanged | Router usage/cache telemetry is not recorded as main thread usage or `UsageService` accumulation. | Live provider/cache superiority remains separate. |
| Reasonix absorption safety | evidence improves; score unchanged | Useful classifier behavior stays analytix-owned without importing Reasonix auto-plan config. | No user-visible auto-plan setting or Reasonix public protocol. |

Allowed wording:

```text
Analytix auto-router failure falls back to heuristic routing without recording
router usage/cache telemetry as main turn usage.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible auto-plan settings,
Reasonix controller rebuild parity, live provider/cache superiority, default Go
backend, release readiness, Kun identity, or Rust/Tauri migration.
```

## 44. 2026-06-21 Scheduled Detector Custom Endpoint Score Rule

This rule covers scheduled detector custom endpoint inference. It improves
offline provider-consumer non-regression evidence, but it does not raise live
provider/cache, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `claw-scheduled-task-detector.test.ts` proves custom `/messages` and `/chat/completions` full endpoints infer the expected URL/header/body/parser behavior. | Credentialed scheduled-task provider matrix remains separate. |
| Product boundary governance | evidence improves; score unchanged | Schedule keeps using analytix settings/runtime contracts and no upstream public protocol. | Does not prove packaged Schedule QA. |
| Reasonix/Kun absorption safety | evidence improves; score unchanged | Endpoint inference is absorbed without Reasonix provider protocol or Kun/deprecated settings identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has scheduled detector fake-fetch evidence for custom full endpoint
inference.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 52. 2026-06-21 Task-Job Stale Restart Reconciliation Score Rule

This rule covers queued/running stale task-job restart reconciliation. It
improves offline orchestration safety evidence and Go G5 shadow quality, but it
does not raise Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `task-job-orchestration-oracle.test.ts` proves queued and running orphan jobs reconcile to failed on restart. | Packaged desktop restart QA remains separate. |
| Reasonix absorption safety | evidence improves; score unchanged | Reasonix-style job lifecycle discipline is absorbed into analytix-owned task-job contracts. | No Reasonix public job protocol or SessionAPI. |
| Go runtime shadow quality | evidence improves; score unchanged | Go G5 executable shadow computes stale task jobs -> failed from TS-owned oracle. | No Go Job Manager, renderer route, or default backend. |

Allowed wording:

```text
Analytix reconciles queued and running stale task jobs to explicit failed state
in TS runtime tests and Go G5 executable shadow.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, live Go task/job
execution, Go default backend, renderer-visible Go routes, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 53. 2026-06-21 MCP Annotation Approval No-Execute Score Rule

This rule covers annotation-driven MCP approval/no-execute proof. It improves
offline MCP safety evidence, but it does not raise live MCP, Go backend,
packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Tool approval safety | evidence improves; score unchanged | `mcp-tool-lifecycle-oracle.test.ts` proves denied destructive/openWorld MCP tools do not call the MCP client. | Packaged approval-card QA remains separate. |
| Reasonix absorption safety | evidence improves; score unchanged | MCP lifecycle discipline is absorbed without Reasonix MCP public protocol. | No MCP-indexer top-level entry or public protocol parity. |
| Go runtime shadow quality | evidence improves; score unchanged | Go G4 shadow computes `mcpApprovalAnnotatedNoExecute` from TS-owned fixture fields. | No Go MCP client, renderer route, or default backend. |

Allowed wording:

```text
Analytix MCP annotations feed approval gating, and denied annotated MCP tools
do not execute in TS lifecycle tests or Go G4 shadow evidence.
```

Forbidden wording:

```text
Do not claim Reasonix MCP public protocol parity, MCP-indexer top-level entry,
live MCP credential compatibility, Go MCP client readiness, default Go backend,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 54. 2026-06-21 User-Input Submitted Route Score Rule

This rule covers submitted user-input answer routing. It improves offline
user-input lifecycle evidence, but it does not raise packaged GUI, live
cross-device delivery, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| User-input lifecycle | evidence improves; score unchanged | `approval-user-input-route-oracle.test.ts` proves submitted answers are echoed through HTTP and gate resolution. | Packaged GUI live-card QA remains separate. |
| Runtime/SSE privacy boundary | evidence improves; score unchanged | SSE replay records submitted status without answers. | No claim that answers are archived in event history. |
| Go runtime shadow quality | evidence improves; score unchanged | Go G4 shadow computes submitted-answer echo and answer-free replay flags. | No Go user-input backend, renderer route, or default backend. |

Allowed wording:

```text
Analytix user-input submitted answers are echoed through HTTP/gate resolution,
while SSE replay records only submitted status.
```

Forbidden wording:

```text
Do not claim Reasonix public user-input protocol parity, answer archival in SSE
history, live cross-device user-input delivery, Go user-input backend readiness,
default Go backend, release readiness, Kun identity, or Rust/Tauri migration.
```

## 55. 2026-06-21 Task-Job Route Auth Matrix Score Rule

This rule covers task-job route authentication proof. It improves offline
runtime contract evidence, but it does not raise Go backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | `task-job-orchestration-oracle.test.ts` proves wait/output/kill reject missing runtime tokens. | Packaged desktop route QA remains separate. |
| Reasonix absorption safety | evidence improves; score unchanged | Internal job control is absorbed without public Reasonix job protocol. | No public SessionAPI/job route parity claim. |
| Go runtime shadow quality | unchanged | No Go runtime route is exposed or changed. | Go route auth remains deferred until Go routes exist behind G5/G6 gates. |

Allowed wording:

```text
Analytix task-job wait/output/kill routes are authenticated internal runtime
routes and reject missing runtime tokens.
```

Forbidden wording:

```text
Do not claim Reasonix public job protocol parity, unauthenticated task-job
control, Go route auth readiness, default Go backend, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 56. 2026-06-21 Connect Phone Product Copy Score Rule

This rule covers Connect Phone product-copy sovereignty. It improves product
identity evidence, but it does not raise live Connect Phone QA, Go backend, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | Locale scan rejects standalone `Claw` in English/Chinese user-visible strings. | Internal `claw` compatibility rename remains separate. |
| Connect Phone workflow | evidence improves; score unchanged | IM command/runtime replies and prompt tool hints use Connect Phone wording. | Live Connect Phone desktop/remote QA remains release-gated. |
| Kun baseline preservation | evidence improves; score unchanged | Code/Write/Settings/Plugins/Connect Phone/Schedule entry baseline unchanged. | Local Whisper/tray packaged QA remains future work. |

Allowed wording:

```text
Analytix guards Connect Phone user-visible and model-visible copy against
standalone Claw wording without renaming internal compatibility contracts.
```

Forbidden wording:

```text
Do not claim live Connect Phone superiority, internal `claw` contract rename,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 57. 2026-06-21 Managed Runtime Provider Currentness Score Rule

This rule covers managed runtime provider currentness and product-identity
guards. It improves offline runtime-contract and product-sovereignty evidence,
but it does not raise live provider/cache, Go backend, packaged QA, or release
readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Runtime contract reliability | evidence improves; score unchanged | Runtime-affecting settings key is shared by main ensure/restart decisions and changes when selected provider endpoint/profile currentness changes. | Packaged restart QA remains separate. |
| Provider request safety | evidence improves; score unchanged | Managed child `ANALYTIX_MODEL_PROVIDERS` snapshot refreshes provider base URL, endpoint format, and reasoning profile drift. | Credentialed provider matrix remains separate. |
| Product sovereignty | evidence improves; score unchanged | Model-visible system prompt, public facade domains, and Kun legacy envelope fallback are guarded. | Internal `claw` compatibility rename remains separate. |

Allowed wording:

```text
Analytix keys selected provider currentness into managed runtime rebuild
decisions and guards model-visible/public API identity under analytix contracts.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, live provider/cache superiority,
packaged restart QA, release readiness, default Go backend, Kun identity,
Reasonix public protocol, or Rust/Tauri migration.
```

## 58. 2026-06-21 Provider Endpoint URL Builder Parity Score Rule

This rule covers shared URL construction for auxiliary provider consumers. It
improves offline provider request-safety evidence, but it does not raise live
provider/cache, packaged Schedule/Write QA, Go backend, or release-readiness
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | Shared helper and fake-fetch tests cover versioned responses/messages endpoint bases and known path stripping. | Credentialed provider matrix remains separate. |
| Auxiliary consumer parity | evidence improves; score unchanged | Scheduled detector and write-inline use the same URL helper. | Packaged Schedule/Write provider QA remains separate. |
| Product sovereignty | unchanged | No bridge/settings/protocol/product surface changes. | Live provider superiority remains unproven. |

Allowed wording:

```text
Analytix auxiliary provider consumers share endpoint URL construction for
versioned responses/messages bases and known endpoint paths.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix,
Reasonix provider protocol parity, packaged Schedule/Write QA, release
readiness, default Go backend, Kun identity, or Rust/Tauri migration.
```

## 59. 2026-06-22 Go G5 User-Input Gate Shadow Score Rule

This rule covers the submitted/cancelled user-input G5 executable shadow proof.
It improves offline Go-gate evidence, but it does not raise live Go backend,
desktop-live-card QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go G5 gate quality | evidence improves; score unchanged | `go-g5-full-loop-oracle.json`, `go-runtime-conformance.test.ts`, and `packages/runtime-go/shadow_test.go` now pin submitted/cancelled user-input gate outcomes. | No live Go user-input manager or Electron routing. |
| Approval/user-input safety | evidence improves; score unchanged | Submitted answers are echoed only by HTTP/gate response while resolved replay events omit answers; cancelled gates reject late resolves. | Packaged renderer live-card QA remains separate. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask protocol, Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence for submitted/cancelled user-input gate behavior
without leaking submitted answers into replay.
```

Forbidden wording:

```text
Do not claim live Go backend parity, Reasonix ask/session protocol parity,
renderer-visible Go routes, packaged desktop live-card QA, release readiness,
default Go backend, Kun identity, or Rust/Tauri migration.
```

## 60. 2026-06-22 / 2026-06-23 Go AutoResearch State Score Rule

This rule now covers the D-0246 Go runtime contract implementation for
AutoResearch project-local state. It improves long-task state durability and
audit evidence, but it does not raise default Go backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Long-running task state | implementation evidence improves; score unchanged | `packages/runtime-go/internal/research/autoresearch_state.go` and Go tests prove `.analytix/autoresearch/<threadId>/` files, requirement audit, unknown requirement rejection, and restart/resume preservation. | Default Go backend remains gated. |
| Runtime event contract | implementation evidence improves; score unchanged | `autoresearch_state_audit` is schema-validated, reducer-ignored, and replayed by Go runtime conformance. | No new renderer-visible protocol. |
| Cache/prefix hygiene | evidence improves; score unchanged | Audit fields and product scan pin research state outside stable prefix and tool schema. | No live provider/cache superiority claim. |
| Product sovereignty | unchanged | The proof adds no top-level AutoResearch route, Reasonix public project protocol, renderer route, or Kun identity. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix Go runtime has project-local AutoResearch state for /goal --research
turns, with audit-only SSE replay and no top-level AutoResearch surface.
```

Forbidden wording:

```text
Do not claim default Go backend, full live Go backend parity, Reasonix
AutoResearch/project protocol parity, top-level AutoResearch navigation,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 61. 2026-06-22 Go G5 MCP Lifecycle Shadow Score Rule

This rule covers the MCP lifecycle/indexer G5 executable shadow proof. It
improves offline MCP gate evidence, but it does not raise live credentialed MCP,
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | G5 fixture and Go shadow cover retry attempts, connected/error server ids, tombstone/resume active paths, and redaction. | No credentialed MCP server matrix. |
| Tool/privacy safety | evidence improves; score unchanged | `leaksSecret:false` and redacted diagnostics are bound into G5 expected output. | No packaged desktop MCP QA. |
| Product sovereignty | unchanged | The proof adds no top-level MCP-indexer route, Reasonix MCP protocol, Go route, or Kun identity. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence for MCP retry, tombstone/resume, and redaction
boundaries without exposing MCP-indexer as a product surface.
```

Forbidden wording:

```text
Do not claim live MCP parity, Go MCP client readiness, live Go backend parity,
default Go backend, Reasonix MCP protocol parity, top-level MCP-indexer
navigation, packaged desktop QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 62. 2026-06-22 Go G5 Checkpoint/Rewind Shadow Score Rule

This rule covers the checkpoint/rewind G5 executable shadow proof. It improves
offline recovery-safety gate evidence, but it does not raise live checkpoint,
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Checkpoint/rewind safety | evidence improves; score unchanged | G5 fixture and TS conformance derive ready/blocked files, path escape blocking, symlink blocking, legal `..name` handling, and append-only conversation audit from the analytix checkpoint oracle. | Packaged desktop rewind QA remains separate. |
| Go G5 gate quality | executable shadow evidence improves; backend score unchanged | Go shadow computes checkpoint/plan/apply/rescue id-prefix boundaries, explicit confirmation, no transcript rewrite, no git refs, and no public route. | No live Go checkpoint store, Go route, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | The proof adds no Reasonix checkpoint protocol, Kun git-ref restore contract, renderer-visible Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence that checkpoint/rewind safety preserves
analytix-owned ids, path guards, explicit confirmation, and append-only audit.
```

Forbidden wording:

```text
Do not claim live Go checkpoint parity, default Go backend, Reasonix
checkpoint/rewind protocol parity, Kun git-ref checkpoint parity, packaged
desktop rewind QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 63. 2026-06-22 Go G5 Remote-Entry Boundary Shadow Score Rule

This rule covers the remote-entry G5 executable shadow proof. It improves
offline control-boundary evidence, but it does not raise live remote-entry, Go
backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Remote-entry safety | evidence improves; score unchanged | G5 fixture and TS conformance derive allowed `approvals`/`lifecycle`/`turns` ports and forbidden goal/checkpoint/memory/storage/tool-host keys from the analytix remote-entry oracle. | Packaged desktop remote-entry QA remains separate. |
| Go G5 gate quality | executable shadow evidence improves; backend score unchanged | Go shadow computes forbidden control planes absent, policy override rejected, no Reasonix protocol, and no top-level route. | No live Go remote-entry route, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | The proof adds no Reasonix SessionAPI, renderer-visible Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence that remote entries remain limited to
lifecycle, turn, approval, and user-input control ports.
```

Forbidden wording:

```text
Do not claim live Go remote-entry parity, default Go backend, Reasonix
SessionAPI/protocol parity, renderer-visible Go routes, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 64. 2026-06-22 Go G5 Resume Pending Gates Shadow Score Rule

This rule covers the resume pending approval/user-input gates G5 executable
shadow proof. It improves offline resume-safety evidence, but it does not raise
live resume, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Resume safety | evidence improves; score unchanged | G5 fixture and TS conformance derive source pending and resumed expired/cancelled gate statuses from the analytix approval/user-input route oracle. | Packaged desktop resume QA remains separate. |
| Approval/user-input safety | evidence improves; score unchanged | Go shadow computes `pendingAfterResume:0` and `answersCopiedToResume:false`. | No live Go approval/user-input manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask/session protocol, renderer-visible Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has G5 shadow evidence that resumed threads preserve gate audit
visibility without leaving pending approval/user-input gates actionable.
```

Forbidden wording:

```text
Do not claim live Go resume parity, default Go backend, Reasonix ask/session
protocol parity, renderer-visible Go routes, packaged desktop resume QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 43. 2026-06-21 Runtime Provider-Selection Request-Shape Score Rule

This rule covers runtime provider selection and request-shape proof. It
improves offline provider non-regression evidence, but it does not raise live
provider/cache, Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `multi-provider-model-client.test.ts` proves selected custom providers and missing-provider fallback produce the expected URL/header/body shapes. | Credentialed provider matrix remains separate. |
| Runtime contract reliability | evidence improves; score unchanged | Provider diagnostics remain analytix-owned and do not expose Reasonix protocol fields. | Does not prove packaged desktop provider settings UI. |
| Reasonix/Kun absorption safety | evidence improves; score unchanged | Routing behavior is absorbed without Reasonix public provider protocol or Kun/deprecated settings identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has runtime fake-fetch evidence that provider selection drives the
expected request shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 65. 2026-06-22 Auto-Router Classifier Contract Score Rule

This rule covers the `_auto_router` request-shape and timeout-fallback proof.
It improves offline auto-router/currentness evidence, but it does not raise
live provider, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Auto-router/currentness | evidence improves; score unchanged | `auto-model-router.test.ts` proves isolated classifier request shape, timeout fallback, and fingerprint drift. | No product auto-plan/planner toggles or live controller rebuild UX. |
| Provider/cache safety | evidence improves; score unchanged | Router classifier uses no tools/prefix and falls back without promoting timeout to main turn failure. | Live provider/cache superiority remains separate. |
| Product sovereignty | unchanged | The proof adds no Reasonix public protocol, Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has router-unit evidence that `_auto_router` stays an isolated
short-JSON classifier with timeout fallback and fingerprinted currentness.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible planner toggles,
Reasonix SessionAPI/controller protocol, Go auto-router parity, live
provider/cache superiority, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 66. 2026-06-22 Planner-Executor Transcript Propagation Score Rule

This rule covers planner-executor transcript propagation. It improves offline
sub-agent/job orchestration evidence, but it does not raise live Subagent, Go
backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | `task-job-orchestration-oracle.test.ts` proves executor jobs persist identity-checked fork transcript refs. | No public Subagent product surface or packaged nested-card QA. |
| Go G5 gate quality | evidence improves; score unchanged | G5 fixture and Go shadow replay transcript propagation flags. | No live Go Job Manager or Electron routing. |
| Product sovereignty | unchanged | The proof adds no Reasonix public protocol, Go route, bridge/settings fallback, Kun identity, or top-level hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has internal planner-executor evidence for durable transcript-ref
propagation behind task-job contracts.
```

Forbidden wording:

```text
Do not claim Reasonix public Subagent/SessionAPI parity, top-level Subagent or
Workflow navigation, live Go Job Manager readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 67. 2026-06-22 MCP Search Meta-Tool Trust Score Rule

This rule covers MCP search meta-tool trust/no-execute evidence. It improves
offline MCP lifecycle evidence, but it does not raise live MCP credential,
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | `mcp-tool-provider.test.ts` proves search meta-tools honor workspace trust and approval. | No credentialed MCP server matrix. |
| Go G4 gate quality | evidence improves; score unchanged | G4 fixture and Go shadow replay meta-tool advertised/trust/no-execute booleans. | No live Go MCP client or Electron routing. |
| Product sovereignty | unchanged | The proof adds no MCP-indexer top-level route, Reasonix public protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has MCP search meta-tool evidence for workspace trust and approval
before MCP client execution.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer protocol parity, top-level MCP-indexer
navigation, credentialed MCP matrix readiness, live Go MCP client readiness,
default Go backend, packaged desktop MCP QA, release readiness, Kun identity,
or Rust/Tauri migration.
```

## 68. 2026-06-22 Custom Messages Full Endpoint Score Rule

This rule covers custom full `/messages` endpoint request-shape proof. It
improves offline provider non-regression evidence, but it does not raise live
provider/cache, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `provider-cache-proof.test.ts` now covers custom full `/messages` exact URL and Anthropic body/header/tool shape. | No credentialed provider matrix. |
| Runtime contract reliability | evidence improves; score unchanged | Custom endpoint inference remains behind analytix provider/cache oracle. | Does not prove packaged settings UI. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has fake-fetch evidence that custom full `/messages` endpoints keep
exact URL and Anthropic Messages request shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 70. 2026-06-22 Auto-Router Fingerprint Currentness Score Rule

This rule covers auto-router classifier request-contract fingerprint proof. It
improves offline currentness evidence, but it does not raise live auto-plan,
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Auto-router currentness | evidence improves; score unchanged | `auto-model-router.test.ts` proves request-contract drift changes the fingerprint. | No user-visible auto-plan setting or packaged controller QA. |
| Runtime contract reliability | evidence improves; score unchanged | Route-cache keys remain analytix-owned and avoid stable-prefix classifier state. | Does not prove live provider/router credential behavior. |
| Product sovereignty | unchanged | The proof adds no Reasonix config/protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has runtime evidence that auto-router classifier request-contract
drift changes route-cache fingerprints.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, local/project auto-plan
overrides, packaged desktop controller rebuild QA, default Go backend,
renderer-visible Go route, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 71. 2026-06-22 Connect Phone Copy Sovereignty Score Rule

This rule covers Connect Phone copy sovereignty. It improves product identity
evidence, but it does not raise packaged desktop QA or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | New generated Connect Phone prompts/titles/schema/logs use Connect Phone copy. | Internal compatibility names still exist by design. |
| Renderer compatibility | evidence improves; score unchanged | Renderer recognizers support new Connect Phone titles and legacy Claw titles. | No packaged desktop IM walkthrough. |
| Release readiness | unchanged | Focused tests prove prompt/render/schema behavior only. | Packaged Connect Phone QA remains open. |

Allowed wording:

```text
Analytix emits Connect Phone copy for new Connect Phone prompts, titles,
schema descriptions, and logs while retaining legacy Claw recognizers.
```

Forbidden wording:

```text
Do not claim all internal compatibility names were renamed, legacy Claw
history support was removed, packaged desktop Connect Phone QA, release
readiness, Kun identity, Reasonix protocol parity, or Rust/Tauri migration.
```

## 72. 2026-06-22 Go G5 Durable Runner Restart Score Rule

This rule covers durable task-job restart executable shadow proof. It improves
Go G5 gate quality, but it does not raise live Go backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow now executable-replays durable runner restart output/offset/kill behavior. | No live Go Job Manager. |
| Task-job orchestration | evidence improves; score unchanged | Expected values are derived from the TS task-job oracle. | No packaged desktop sub-agent/task-job QA. |
| Product sovereignty | unchanged | The proof adds no Reasonix protocol, renderer Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay durable task-job restart output,
offset, and kill behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 69. 2026-06-22 Custom Chat Full Endpoint Score Rule

This rule covers custom full `/chat/completions` endpoint request-shape proof.
It improves offline provider non-regression evidence, but it does not raise
live provider/cache, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `provider-cache-proof.test.ts` now covers custom full `/chat/completions` exact URL and OpenAI-compatible chat body/header/tool shape. | No credentialed provider matrix. |
| Runtime contract reliability | evidence improves; score unchanged | Custom endpoint inference now covers `/responses`, `/messages`, and `/chat/completions` in the provider-cache oracle. | Does not prove packaged settings UI. |
| Go G3/G5 gate quality | evidence improves; score unchanged | G3/G5 shadow fixtures replay the request-shape id only. | No live Go provider client or default backend. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has fake-fetch evidence that custom full `/chat/completions`
endpoints keep exact URL and OpenAI-compatible chat request shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 73. 2026-06-22 Go G5 Abort Cleanup Score Rule

This rule covers approval/user-input abort cleanup executable shadow proof. It
improves Go G5 gate quality, but it does not raise live Go backend, packaged
QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow now executable-replays abort cleanup state, late GUI action statuses, and replay kinds. | No live Go approval/user-input manager. |
| Approval/user-input reliability | evidence improves; score unchanged | Expected values are derived from the TS approval/user-input route oracle. | No packaged desktop approval-card walkthrough. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask/session protocol, renderer Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay approval/user-input abort cleanup
states, late GUI action statuses, and replay event order from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 77. 2026-06-22 Reasonix Auto-Plan Config Boundary Score Rule

This rule covers the settings/config guard for Reasonix auto-plan config
shapes. It improves product-sovereignty evidence but does not implement an
auto-plan product toggle.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | GUI normalization and settings persistence drop Reasonix `agent.auto_plan`, `autoPlan`, and `auto_plan`. | No packaged settings walkthrough. |
| Runtime contract safety | evidence improves; score unchanged | Runtime settings keys ignore rejected auto-plan fields, and `analytix serve` config rejects upstream roots. | No product auto-plan setting. |
| Reasonix/Kun absorption safety | unchanged | The boundary is absorbed without Reasonix protocol, Kun identity, or new top-level route. | Full product planner parity remains unclaimed. |

Allowed wording:

```text
Analytix rejects Reasonix auto-plan public config shapes and preserves
analytix-owned runtime settings boundaries.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan setting parity, project/local auto-plan
overrides, controller rebuild parity, packaged settings QA, default Go backend,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 78. 2026-06-22 Go G5 Task Parent Goal Evidence Executable Score Rule

This rule covers executable shadow replay for task parent-goal evidence. It
improves sub-agent/job orchestration evidence but does not raise live Go or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | Executable G5 output now derives active-goal ledger success and missing-goal error booleans. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from TS-owned fixtures and Go tests compare to the oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no public Subagent route, Reasonix protocol, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 executable shadow replays task parent-goal evidence metadata from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, live Go Job Manager
readiness, default Go backend, top-level Subagent navigation, packaged desktop
sub-agent QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 78. 2026-06-22 Go G5 MCP Lifecycle Detail Replay Score Rule

This rule covers G5 replay evidence for MCP reconnect, known-override, and
search workspace-boundary details. It improves MCP lifecycle and Go G5 evidence
quality, but it does not raise live Go backend, credentialed MCP, packaged QA,
or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | G5 replay now carries reconnect retry/error details, known override diagnostics, and search trust boundaries. | No credentialed MCP server matrix. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the MCP detail summary from the TS MCP lifecycle oracle. | No live Go MCP client/indexer. |
| Product sovereignty | unchanged | The proof adds no MCP-indexer top-level route, Reasonix lifecycle protocol, renderer Go route, Kun identity, or Rust/Tauri path. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers MCP reconnect, known-override, and search
workspace-boundary details from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client/indexer readiness, default Go backend
readiness, Reasonix MCP-indexer lifecycle protocol parity, top-level
MCP-indexer navigation, credentialed MCP matrix, packaged desktop MCP QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 79. 2026-06-22 Go G5 Provider Usage Parser Precedence Score Rule

This rule covers G5 replay evidence for provider usage parser precedence. It
improves provider/cache and Go G5 evidence quality, but it does not raise live
provider, credentialed matrix, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | G5 replay now carries unsupported absence, DeepSeek native precedence, OpenAI responses cached-token, and Anthropic cache-field parser evidence. | No credentialed provider matrix. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes parser-precedence summary from the TS provider-cache oracle. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, renderer Go route, default Go backend, Kun identity, or Rust/Tauri path. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers provider usage parser precedence from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, live Go provider client,
default Go backend readiness, packaged provider settings QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

## 80. 2026-06-22 Go G5 Auto-Router Classifier Currentness Score Rule

This rule covers G5 replay evidence for auto-router classifier currentness,
contract drift invalidation, request isolation, and timeout fallback. It
improves agent-kernel and Go G5 evidence quality, but it does not raise product
auto-plan, packaged QA, live Go backend, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Auto-plan/currentness | evidence improves; score unchanged | G5 replay now carries classifier fingerprint currentness, drift invalidation, isolated request shape, and timeout fallback. | No public product auto-plan setting or packaged planner QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes classifier-currentness output from TS-owned fixtures. | No live Go auto-router or default backend. |
| Product sovereignty | unchanged | The proof adds no Reasonix controller protocol, project override, renderer Go route, Kun identity, or Rust/Tauri path. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers auto-router classifier currentness and
timeout fallback from TS-owned runtime constants.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan setting parity, project/local auto-plan
override support, Reasonix controller/SessionAPI protocol parity, live Go
auto-router readiness, default Go backend readiness, packaged planner QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 77. 2026-06-22 Go G5 Approval/User-Input Route Body Replay Score Rule

This rule covers G5 replay evidence for approval/user-input route bodies and
structured prompt shape. It improves approval/user-input and Go G5 evidence
quality, but it does not raise live Go backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Approval/user-input reliability | evidence improves; score unchanged | G5 replay now carries deny/cancel/submit request and response bodies plus prompt option labels. | No packaged desktop approval-card walkthrough. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the route-body and prompt summary from the TS approval/user-input oracle. | No live Go approval/user-input manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask/session protocol, renderer Go route, Kun identity, hidden top-level entry, or Rust/Tauri path. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers approval/user-input route bodies and
structured prompt shape from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 93. 2026-06-22 G5 Task Parent Goal Evidence Score Rule

This rule covers G5 replay evidence for task/sub-agent parent-goal evidence.
It improves sub-agent/job orchestration evidence, but it does not raise live Go
backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | G5 `jobReplay.parentGoalEvidence` now carries active-goal and evidence event-key requirements. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix protocol, Subagent/Workflow/Create Loop route, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers task parent-goal evidence from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go Job Manager readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 94. 2026-06-22 G5 Planner-Executor Detail Score Rule

This rule covers G5 replay evidence for planner-executor detail fields. It
improves planner/sub-agent orchestration evidence, but it does not raise live
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Planner/sub-agent orchestration | evidence improves; score unchanged | G5 `jobReplay.plannerExecutor` now carries skipped/cancel reasons and output/transcript job counts. | No packaged desktop planner/sub-agent QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix planner protocol, Subagent/Workflow/Create Loop route, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers planner-executor detail evidence from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public planner/sub-agent/job protocol parity, top-level
Subagent or Workflow navigation, live Go Job Manager readiness, default Go
backend, packaged desktop planner/sub-agent QA, release readiness, Kun identity,
or Rust/Tauri migration.
```

## 95. 2026-06-22 G5 Task Tool Contract Boundary Score Rule

This rule covers G5 replay evidence for internal task tool-contract
boundaries. It improves task/sub-agent orchestration evidence, but it does not
raise live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Task/sub-agent orchestration | evidence improves; score unchanged | G5 `jobReplay.toolContractBoundary` now carries task/parallel field names and safety gates. | No packaged desktop task/sub-agent QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix task protocol, Subagent/Workflow/Create Loop route, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers internal task tool-contract boundaries from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public task/sub-agent/job protocol parity, top-level
Subagent or Workflow navigation, live Go Job Manager readiness, default Go
backend, packaged desktop task/sub-agent QA, release readiness, Kun identity,
or Rust/Tauri migration.
```

## 89. 2026-06-22 Provider Live-Local HTTP Proof Score Rule

This rule covers no-credential local HTTP execution for provider usage and
request-shape oracle cases. It improves provider/cache evidence quality, but it
does not raise live provider, packaged QA, Go backend, or release-readiness
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | All usage and request-shape oracle cases execute through a local HTTP provider. | No credentialed provider matrix or live cache hit proof. |
| Provider request safety | evidence improves; score unchanged | Request path, headers, body fields, and tool shape are validated over real HTTP POST. | No packaged settings walkthrough. |
| Product sovereignty | unchanged | The proof adds no Reasonix protocol, Go backend, Kun identity, deprecated bridge/settings fallback, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix provider/cache proof executes all provider usage and request-shape
oracle cases through no-credential local HTTP.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, provider behavior changes, Reasonix provider protocol parity,
default Go backend, packaged provider settings QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 90. 2026-06-22 G5 Provider Live-Local Proof Summary Score Rule

This rule covers G5 shadow replay of the provider live-local HTTP proof
summary. It improves cross-backend evidence auditability, but it does not raise
live provider, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | `provider-cache-oracle.liveLocalHttpProof` pins fixture-only local HTTP proof scope. | No credentialed provider matrix or live cache hit proof. |
| Go G5 gate quality | evidence improves; score unchanged | G5 `cacheReplay.liveLocalHttpProof` derives counts and endpoint formats from the TS oracle. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix protocol, Go backend, Kun identity, deprecated bridge/settings fallback, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replays fixture-owned provider live-local HTTP proof
metadata from the TS provider-cache oracle.
```

Forbidden wording:

```text
Do not claim live Go provider parity, live provider/cache superiority,
credentialed provider matrix readiness, provider behavior changes, Reasonix
provider protocol parity, default Go backend, packaged provider settings QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 92. 2026-06-22 G5 MCP Approval Annotation Score Rule

This rule covers G5 shadow replay of MCP approval annotations for
destructive/open-world tools. It improves approval/MCP evidence quality, but it
does not raise live MCP, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Approval/user-input reliability | evidence improves; score unchanged | G5 `mcpReplay.approvalAnnotations` now carries deny/no-execute approval evidence for high-risk MCP tools. | No packaged approval-card walkthrough. |
| MCP lifecycle | evidence improves; score unchanged | Destructive/open-world MCP metadata is replayed from the TS oracle. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the annotation summary without owning live approvals. | No live Go approval manager or MCP client. |

Allowed wording:

```text
Analytix G5 shadow replays destructive/open-world MCP approval metadata and
denied no-execute evidence from the TS-owned MCP oracle.
```

Forbidden wording:

```text
Do not claim live Go approval-manager readiness, live Go MCP client readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation, default
Go backend, credentialed MCP matrix readiness, packaged desktop MCP QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 91. 2026-06-22 G5 MCP Live-Local Indexer Summary Score Rule

This rule covers G5 shadow replay of MCP live-local indexer lifecycle evidence.
It improves MCP/indexer evidence quality, but it does not raise live MCP, Go
backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | G5 `mcpReplay.liveLocalIndexer` now carries retry, tombstone, restart, active path, and secret-safe fields. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the nested summary from the TS MCP lifecycle oracle. | No live Go MCP client. |
| Product sovereignty | unchanged | The proof adds no MCP-indexer route, Reasonix protocol, Go backend, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replays MCP live-local indexer lifecycle evidence from the
TS-owned MCP oracle.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP-indexer protocol
parity, top-level MCP-indexer navigation, default Go backend, credentialed MCP
matrix readiness, packaged desktop MCP QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 87. 2026-06-22 Go G5 Provider Streaming Usage Score Rule

This rule covers G5 full-loop shadow evidence for provider streaming usage and
cache telemetry. It improves provider/cache and future Go evidence quality, but
it does not raise live provider, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | G5 `providerStreamingReplay` carries G3 streaming event order and parsed DeepSeek usage/cache telemetry. | No credentialed provider matrix. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow parses SSE usage payload and compares it to the TS-owned provider usage case. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no provider behavior change, Reasonix protocol, renderer Go route, default backend, Kun identity, or hidden product entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays provider streaming usage evidence from
TS-owned G3 fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix,
live Go provider client readiness, default Go backend, provider request/stream
parser changes, Reasonix provider protocol parity, packaged provider settings
QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 88. 2026-06-22 Go G5 Task-Job Route Executable Score Rule

This rule covers G5 shadow evidence for internal task-job route executable
behavior. It improves task/job orchestration and future Go evidence quality,
but it does not raise live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Task/job orchestration | evidence improves; score unchanged | TS route tests bind wait/output/kill behavior to `routeExecutable`. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | G5 `jobReplay.routeExecutable` carries unauthorized/output/wait/kill/missing/rehydrated route summary. | No live Go Job Manager or Go route server. |
| Product sovereignty | unchanged | The proof adds no Reasonix protocol, renderer Go route, default backend, Kun identity, or hidden product entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays internal task-job route executable
evidence from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, live Go task-job route server,
default Go backend, Reasonix SessionAPI/sub-agent protocol parity, top-level
Subagent/Workflow/Create Loop navigation, packaged desktop task-job QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 82. 2026-06-22 Go G5 Cache Drift Attribution Score Rule

This rule covers G5 replay evidence for provider-cache drift attribution. It
improves cache-diagnostic and Go G5 gate evidence, but it does not raise live
provider, live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache diagnostics | evidence improves; score unchanged | G5 `cacheReplay.driftAttribution` records previous/current prefix hashes, expected reasons, and changed tool/provider/model/endpoint flags. | No credentialed live provider matrix. |
| Stable prefix hygiene | evidence improves; score unchanged | `systemHashStable` and `prefixItemsHashStable` prove this fixture does not rely on dynamic-context stable-prefix pollution. | Live cache-rate drift still needs provider QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the drift attribution fields from the TS provider-cache oracle. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, dynamic stable-prefix material, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay attributes provider-cache drift to allowed
tool/provider/model changes while preserving stable prefix hygiene.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, Reasonix provider protocol parity,
live Go provider client readiness, default Go backend, packaged provider
settings QA, release readiness, Kun identity, dynamic-context stable prefix, or
Rust/Tauri migration.
```

## 83. 2026-06-22 Go G3 Cache Drift Attribution Score Rule

This rule covers G3 provider conformance evidence for cache drift attribution.
It improves provider-gate and future Go evidence quality, but it does not raise
live provider, live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache diagnostics | evidence improves; score unchanged | G3 provider oracle now carries raw drift shapes and compact attribution output tied to `ProviderCacheOracle`. | No credentialed live provider matrix. |
| Go G3 gate quality | evidence improves; score unchanged | Go shadow computes drift attribution in `BuildG3ProviderConformanceOutput`. | No live Go provider client. |
| Runtime contract stability | unchanged | Provider URL/body behavior, usage parsing, and streaming parser are unchanged. | Packaged provider settings QA remains separate. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, dynamic stable-prefix material, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G3 provider conformance computes cache drift attribution from
TS-owned raw shapes while preserving stable prefix hygiene.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, provider request behavior changes,
Reasonix provider protocol parity, live Go provider client readiness, default Go
backend, packaged provider settings QA, release readiness, Kun identity,
dynamic-context stable prefix, or Rust/Tauri migration.
```

## 84. 2026-06-22 Go G3 Provider Request Shape Score Rule

This rule covers G3 provider conformance evidence for request URL/body/tool
shape replay. It improves provider-gate and future Go evidence quality, but it
does not raise live provider, live Go backend, packaged QA, or release-readiness
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | G3 provider oracle now carries request-shape matrix and compact request-surface summary tied to `ProviderCacheOracle`. | No credentialed live provider matrix. |
| OpenAI/Anthropic/custom non-regression | evidence improves; score unchanged | Summary covers endpoint families, full endpoint cases, tool-shape families, and required/forbidden body fields. | Live provider endpoint QA remains separate. |
| Go G3 gate quality | evidence improves; score unchanged | Go shadow computes request-shape summary in `BuildG3ProviderConformanceOutput`. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, provider behavior change, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G3 provider conformance replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, provider request behavior changes,
Reasonix provider protocol parity, live Go provider client readiness, default Go
backend, packaged provider settings QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 86. 2026-06-22 Go G5 Session Route Status Score Rule

This rule covers G5 full-loop shadow evidence for thread/session route status
and SSE replay semantics. It improves runtime-contract and future Go evidence
quality, but it does not raise live Go backend, packaged QA, or release-readiness
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Thread/session runtime contract | evidence improves; score unchanged | G5 `sessionReplay.routeStatusReplay` carries archive/search/read/update/fork/resume/SSE semantics from G2 oracle. | Packaged desktop restart/resume QA remains separate. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes route status replay from G2 route responses and SSE frames. | No live Go route server. |
| Product sovereignty | unchanged | The proof adds no Reasonix SessionAPI, renderer-visible Go route, default backend, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays thread/session route status and SSE
semantics from TS-owned G2 HTTP/SSE fixtures.
```

Forbidden wording:

```text
Do not claim live Go route readiness, default Go backend, Reasonix SessionAPI
protocol parity, packaged desktop restart/resume QA, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

## 85. 2026-06-22 Go G5 Provider Request Shape Score Rule

This rule covers G5 full-loop shadow evidence for provider request URL/body/tool
shape replay. It improves provider non-regression and future Go evidence
quality, but it does not raise live provider, live Go backend, packaged QA, or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | G5 `cacheReplay.requestShapeReplay` carries compact request-surface summary tied to `ProviderCacheOracle`. | No credentialed live provider matrix. |
| OpenAI/Anthropic/custom non-regression | evidence improves; score unchanged | Summary covers endpoint families, full endpoint cases, tool-shape families, and required/forbidden body fields. | Live provider endpoint QA remains separate. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes request-shape replay in `BuildG5ShadowSlicesOutput`. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, Go route, Kun identity, provider behavior change, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, provider request behavior changes,
Reasonix provider protocol parity, live Go provider client readiness, default Go
backend, packaged provider settings QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 81. 2026-06-22 Offline Provider/Cache Parity Seal Score Rule

This rule covers fixture-only provider/cache parity evidence. It improves
DeepSeek cache/prefix and multi-provider regression evidence, but it does not
raise live provider, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Provider/cache proof | evidence improves; score unchanged | G5 `cacheReplay.offlineParitySeal` records DeepSeek stable prefix/cache and multi-provider request/usage coverage. | No credentialed provider matrix. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the seal from the TS provider-cache oracle. | No live Go provider client. |
| Product sovereignty | unchanged | The proof adds no Reasonix provider protocol, default Go backend, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix has fixture-backed offline provider/cache parity evidence for
DeepSeek stable prefix/cache and multi-provider request/usage regressions.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 77. 2026-06-22 Go G5 Task Job Lifecycle Replay Score Rule

This rule covers G5 replay evidence for the base `task` lifecycle. It improves
task/job orchestration evidence, but it does not raise live Go backend,
packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | G5 `jobReplay.lifecycle` now carries foreground, background, and wait/output/kill lifecycle fields. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no Subagent/Workflow/Create Loop/AutoResearch route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers base task-job lifecycle from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go Job Manager readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 78. 2026-06-22 Go G5 Task Job Route Boundary Replay Score Rule

This rule covers G5 replay evidence for task-job route auth and forbidden
top-level route boundaries. It improves task/job orchestration evidence, but it
does not raise live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | G5 `jobReplay.routeBoundary` now carries protected route ids, `401`, and forbidden top-level routes. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go task-job routes or Job Manager. |
| Product sovereignty | unchanged | The proof adds no Subagent/Workflow/Create Loop/AutoResearch route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers task-job route auth and forbidden top-level
route evidence from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go task-job route readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 79. 2026-06-22 Go G5 MCP Core Lifecycle Replay Score Rule

This rule covers G5 replay evidence for MCP connect/disconnect/reload/cancel/
error lifecycle fields. It improves MCP lifecycle evidence, but it does not
raise live Go backend, credentialed MCP, packaged QA, or release-readiness
scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | G5 `mcpReplay.lifecycle` now carries connect/disconnect/reload/cancel/error fields. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS MCP lifecycle oracle. | No live Go MCP client. |
| Product sovereignty | unchanged | The proof adds no MCP-indexer route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers MCP core lifecycle evidence from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer protocol parity, top-level MCP-indexer
navigation, live Go MCP client readiness, default Go backend, credentialed MCP
matrix readiness, packaged desktop MCP QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 80. 2026-06-22 Go G5 Approval Decision Route Replay Score Rule

This rule covers G5 replay evidence for approval decision routing and replay
order. It improves approval/user-input evidence, but it does not raise live Go
backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Approval/user-input reliability | evidence improves; score unchanged | G5 `approvalUserInputReplay` now carries approval route decision, duplicate `409`, and replay kind order. | No packaged desktop approval-card walkthrough. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS approval/user-input route oracle. | No live Go approval manager. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask/session protocol, renderer Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers approval decision route evidence from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 75. 2026-06-22 Go G5 MCP Search Meta-Tool Replay Score Rule

This rule covers G5 replay evidence for MCP search meta-tool trust and denied
no-execute behavior. It improves MCP lifecycle evidence, but it does not raise
live MCP, Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| MCP lifecycle | evidence improves; score unchanged | G5 `mcpReplay` now carries search meta-tool trust/no-execute fields. | No credentialed MCP server matrix. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS MCP lifecycle oracle. | No live Go MCP client. |
| Product sovereignty | unchanged | The proof adds no MCP-indexer top-level route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers MCP search meta-tool trust and denied
no-execute behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer protocol parity, top-level MCP-indexer
navigation, live Go MCP client readiness, default Go backend, credentialed MCP
matrix readiness, packaged desktop MCP QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 76. 2026-06-22 Go G5 Parallel Task Dependency Validation Score Rule

This rule covers G5 replay evidence for `parallel_tasks` dependency validation.
It improves sub-agent/job orchestration evidence, but it does not raise live
Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | G5 `jobReplay.parallelValidation` now carries valid order and five invalid dependency cases. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow computes the fields from the TS task-job oracle. | No live Go Job Manager. |
| Product sovereignty | unchanged | The proof adds no Subagent/Workflow/Create Loop route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow replay covers `parallel_tasks` dependency validation from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go Job Manager readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 74. 2026-06-22 Go G5 Abort Cleanup Replay Summary Score Rule

This rule covers replay-summary closure for approval/user-input abort cleanup.
It improves Go G5 evidence consistency, but it does not raise live Go backend,
packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go G5 gate quality | evidence improves; score unchanged | G5 control replay, approval/user-input replay, and executable shadow now report the same abort-cleanup semantics. | No live Go approval/user-input manager. |
| Approval/user-input reliability | evidence improves; score unchanged | Summary fields are derived from the TS approval/user-input route oracle. | No packaged desktop approval-card walkthrough. |
| Product sovereignty | unchanged | The proof adds no Reasonix ask/session protocol, renderer Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 shadow summary and executable output both cover
approval/user-input abort cleanup from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 79. 2026-06-22 Product Sovereignty Scan Engineering Score Rule

This rule covers the reusable forbidden-surface scan. It improves
maintainability and product-sovereignty evidence but does not raise release
readiness by itself.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` bundles top-level entry, identity/protocol, deprecated bridge/settings, Go/Rust/Tauri, file, and locale scans. | No packaged desktop QA. |
| Maintainability | evidence improves; score unchanged | Future absorption batches can run one stable scan command instead of copying release-evidence grep commands. | Scan is not behavior coverage. |
| Kun/Reasonix absorption safety | unchanged | Plugin Marketplace is included in route-surface forbidden-token coverage. | Full release gate remains separate. |

Allowed wording:

```text
Analytix has a reusable product-sovereignty scan gate for upstream absorption
batches.
```

Forbidden wording:

```text
Do not claim release readiness, packaged desktop QA, Reasonix public protocol
parity, default Go backend readiness, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer support, Kun identity, or Rust/Tauri migration from
this scan alone.
```

## 80. 2026-06-22 Go G5 Parallel Task Dependency Executable Shadow Score Rule

This rule covers executable-shadow evidence for `parallel_tasks` dependency
validation. It improves sub-agent/job orchestration evidence, but it does not
raise live Go backend, packaged QA, or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Sub-agent/job orchestration | evidence improves; score unchanged | G5 executable output now computes valid DAG order and five invalid dependency errors. | No packaged desktop sub-agent/task-job QA. |
| Go G5 gate quality | evidence improves; score unchanged | Go shadow executes the dependency validator and compares it to the TS-owned expected oracle. | No live Go Job Manager, rollback, or G6 readiness. |
| Product sovereignty | unchanged | The proof adds no Subagent/Workflow/Create Loop route, Reasonix protocol, Go route, Kun identity, or hidden capability entry. | Release readiness remains unproven. |

Allowed wording:

```text
Analytix G5 executable shadow covers `parallel_tasks` dependency validation
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go Job Manager readiness, default Go backend,
G6 readiness, packaged desktop sub-agent QA, release readiness, Kun identity,
or Rust/Tauri migration.
```

## 81. 2026-06-22 Write Sidebar and Connect Phone Placeholder Sovereignty Score Rule

This rule covers product-sovereignty scan and copy evidence. It improves
absorption safety evidence, but it does not raise packaged QA or
release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | Write sidebar, workspace mode tabs, sidebar projects section, and stronger token variants are now covered by scan/test gates. | No packaged desktop walkthrough. |
| Connect Phone QA | evidence improves; score unchanged | New mapped-conversation placeholders emit `[Connect Phone:...]` instead of `[Claw:...]`. | Internal compatibility names remain by design. |
| Kun/Reasonix absorption safety | unchanged | The proof adds no Reasonix protocol, Kun identity, deprecated bridge/settings fallback, default Go backend, or hidden top-level entry. | Full release gate remains separate. |

Allowed wording:

```text
Analytix product-sovereignty scans cover Write/sidebar entry surfaces, and new
Connect Phone placeholders no longer emit `[Claw:...]`.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, release readiness, removal of legacy Claw
compatibility, Reasonix protocol parity, Kun identity, default Go backend, or
Rust/Tauri migration.
```

## 82. 2026-06-22 HTTP Auto-Plan Payload Boundary Score Rule

This rule covers negative HTTP contract evidence for Reasonix-shaped auto-plan
fields. It improves Plan mode/product-sovereignty evidence, but it does not
raise packaged QA or release-readiness scores.

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Plan mode contract safety | evidence improves; score unchanged | `autoPlan` / `auto_plan` start-turn payload fields do not set Plan mode or advertise `create_plan`. | No packaged Plan mode walkthrough. |
| Reasonix/Kun absorption safety | unchanged | The proof rejects Reasonix public auto-plan shape without adding Kun-absent top-level entries. | Full product auto-plan parity remains unclaimed. |
| Go G5 readiness | unchanged | No Go runtime code changed. | No live Go controller, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix ignores Reasonix-shaped `autoPlan` / `auto_plan` start-turn fields;
only explicit `mode: "plan"` / `guiPlan` can advertise `create_plan`.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user/project auto-plan settings,
controller rebuild parity, packaged Plan mode QA, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 83. 2026-06-22 Plan Step Cancel/Cache Score Rule

Scoring rule:

```text
Planner quality evidence improves only when step gating, cancellation, and
cache accounting are proven together under analytix-owned contracts.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner gating | evidence improves; score unchanged | Captured AgentLoop requests prove Plan step 0 exposes read-only tools plus `create_plan`, while the unsatisfied follow-up exposes only `create_plan`. | No packaged desktop Plan mode QA. |
| Cache stability | evidence improves; score unchanged | Cancelling the follow-up step leaves the next equivalent Plan turn at `cacheDiagnostics.prefixChanged: false`. | No live provider credential matrix. |
| Product sovereignty | unchanged | The fixture uses explicit analytix `mode: "plan"` and adds no Reasonix/Kun public surface. | Release readiness still blocked by packaged QA/signing. |
| Go readiness | unchanged | No Go backend behavior changed. | G5/G6 and rollback gates remain required. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

## 84. 2026-06-22 MCP Stdio Execution-Error Redaction Score Rule

Scoring rule:

```text
MCP lifecycle quality evidence improves only when secret redaction is proven on
actual tool-result payloads, not only diagnostics summaries.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Tool/privacy safety | evidence improves; score unchanged | Executable stdio MCP fake indexer proves `isError` result payloads redact `Bearer lifecycle-secret`. | No credentialed MCP matrix. |
| MCP lifecycle | evidence improves; score unchanged | Redaction now covers thrown errors and protocol-level MCP result errors. | No packaged desktop MCP QA. |
| Product sovereignty | unchanged | No MCP-indexer route/navigation or Reasonix protocol added. | Release readiness still blocked by packaged QA/signing. |
| Go readiness | unchanged | No Go backend behavior changed. | G5/G6 and rollback gates remain required. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

## 85. 2026-06-22 Workflow/Create Loop Quarantine Score Rule

Scoring rule:

```text
Product sovereignty evidence improves when dormant Kun-absent features are
proven quarantined from all top-level entry surfaces.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | Route-surface test and scan gate forbid workflow/create-loop symbols in entry surfaces. | No packaged desktop walkthrough. |
| Kun baseline fidelity | evidence improves; score unchanged | Kun 0.2.13/0.2.14 top-level hierarchy remains unchanged. | Dormant code still exists by design. |
| Release readiness | unchanged | This is source/scan evidence only. | Signing/packaging/manual QA remain separate. |

Evidence:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 86. 2026-06-22 MCP Refresh Catalog Drift Score Rule

Scoring rule:

```text
MCP lifecycle/currentness evidence improves when catalog refresh drift is
executable in the TS oracle and replayed by G5 shadow without exposing a
public MCP-indexer surface.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle/currentness | evidence improves; score unchanged | `mcp_refresh_catalog` is executed after fake-client catalog expansion and proves `catalogDrift: true`. | No credentialed MCP matrix. |
| Go G5 readiness | evidence improves; score unchanged | `mcpReplay.searchRefreshDrift` is derived from the TS-owned MCP oracle in Go shadow. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No MCP-indexer route/navigation or Reasonix protocol is added. | Release readiness still blocked by packaged QA/signing. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 102. 2026-06-22 Browser Preview Bridge / Visible Entry Sovereignty Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when browser preview bridge and visible
sidebar entry surfaces are covered by source guards, rendered smoke tests, and
scan path freshness.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | Browser preview bridge source guard, Sidebar render smoke, and `scan:product-sovereignty` path freshness now cover the non-Electron bridge and visible entry surface. | No packaged desktop walkthrough. |
| Kun baseline preservation | evidence improves; score unchanged | Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer visible labels remain rejected from top-level sidebar and entry surfaces. | Dormant Create Loop internals still require future product redesign before exposure. |
| Reasonix absorption safety | evidence improves; score unchanged | Reasonix-style public protocol/bridge aliases remain rejected while analytix runtime proxy/SSE paths are preserved. | No Reasonix public protocol parity is claimed. |

Evidence:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 103. 2026-06-22 Browser Preview SSE Executable Bridge Score Rule

Scoring rule:

```text
Runtime contract safety evidence improves when browser preview SSE behavior is
covered by executable proxy-path, cursor, and event-normalization tests.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | Browser preview `startSse` now has executable evidence for analytix `/v1/threads/:id/events` proxy path, `Last-Event-ID`, and normalized event payload. | No packaged browser/desktop walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Browser preview remains on `window.analytix` and analytix HTTP/SSE semantics. | No Reasonix public protocol parity. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go bridge, renderer-visible Go route, or default backend. |

Evidence:

```text
npm run test -- src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

## 104. 2026-06-22 Browser Preview Settings Sovereignty Score Rule

Scoring rule:

```text
Settings sovereignty evidence improves when browser preview settings load and
save paths prove rejected legacy/Reasonix envelopes are stripped while valid
top-level runtime fields persist.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | Shared normalize and browser preview tests prove `agentProvider`/`agents`/`deepseek`/`reasonix` plus Reasonix auto-plan fields are stripped on load and save. | No packaged settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Browser preview keeps valid `runtime.model` and `runtime.endpointFormat` under analytix-owned schema. | No deprecated settings fallback is claimed. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go settings bridge, renderer-visible Go route, or default backend. |

Evidence:

```text
npm run test -- src/shared/app-settings.test.ts src/renderer/src/lib/browser-analytix-bridge.test.ts --no-file-parallelism --maxWorkers=1
```

## 105. 2026-06-22 IPC Settings Patch Sovereignty Score Rule

Scoring rule:

```text
Settings sovereignty evidence improves when renderer-to-main IPC settings patch
validation strips top-level legacy/Reasonix envelopes before strict schema
validation while preserving valid analytix patch fields.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | IPC schema test proves top-level legacy envelopes are stripped and valid settings patch fields survive. | No packaged settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Runtime-level legacy agent-shaped settings still reject rather than fallback. | No Reasonix config/auto-plan parity. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go settings bridge, renderer-visible Go route, or default backend. |

Evidence:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts --no-file-parallelism --maxWorkers=1
```

## 106. 2026-06-22 Settings Sovereignty Scan Freshness Score Rule

Scoring rule:

```text
Settings sovereignty evidence improves when the product-sovereignty scan also
requires the key settings/bridge chokepoints and their guard tests to remain in
the source tree.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now requires shared normalize/runtime settings, settings-store, IPC schema, preload, browser preview bridge, and their focused tests to stay present. | No packaged settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | The scan freshness gate makes bridge/settings guard coverage harder to accidentally remove. | No Reasonix config/auto-plan parity. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go settings bridge, renderer-visible Go route, or default backend. |

Evidence:

```text
npm run scan:product-sovereignty
```

## 107. 2026-06-22 Workflow Singular Route Negative Score Rule

Scoring rule:

```text
Runtime contract evidence improves when live HTTP negative-route tests cover
the singular Workflow route used by task-job and Go G4/G5 forbidden-route
oracles.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `http-server.test.ts` now proves `/v1/workflow` and `/v1/workflows` both return structured 404s. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer stay absent as public HTTP routes. | Internal capability code remains allowed behind analytix contracts. |
| Go G5 readiness | unchanged | Go fixtures already replay singular `/v1/workflow`; no Go code changed. | No live Go HTTP route or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/http-server.test.ts --no-file-parallelism --maxWorkers=1
```

## 108. 2026-06-22 Runtime Proof Freshness Score Rule

Scoring rule:

```text
Runtime conformance evidence improves when the product-sovereignty scan
requires the proof fixtures, proof tests, and Go shadow sources to remain
present and scans active route/client source for forbidden public surfaces.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `scan:product-sovereignty` now requires G2/G3/G4/G5 fixtures/tests and Go shadow source paths to stay present. | No packaged desktop route/provider walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Active runtime routes, shared endpoint templates, IPC schema, and renderer runtime client are scanned for forbidden public route/protocol strings. | Negative tests/fixtures intentionally retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | Go conformance docs now reflect closed G2 exact replay and active provider/cache plus approval/user-input shadow evidence. | No live Go backend or default route. |

Evidence:

```text
npm run scan:product-sovereignty
```

## 109. 2026-06-22 Renderer Runtime Request Surface Score Rule

Scoring rule:

```text
Runtime contract evidence improves when renderer provider behavior, not only
source scanning, proves common runtime requests stay on analytix-owned routes.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `analytix-runtime.test.ts` captures renderer provider request paths for connect/thread/turn/goal/todos/approval/user-input/fork/resume flows. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Captured renderer paths are forbidden from matching Reasonix public routes, renderer-visible Go routes, or hidden-capability public routes. | Browser/main/preload route proofs remain separate evidence. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or default route. |

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

## 110. 2026-06-22 Renderer SSE Bridge Cursor Score Rule

Scoring rule:

```text
Runtime contract evidence improves when renderer SSE subscription behavior,
not only browser/main bridge code, proves stream cursor and cleanup stay behind
the analytix bridge.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `analytix-runtime.test.ts` proves `subscribeThreadEvents` calls `startSse(threadId, sinceSeq, streamId)`, receives events for that generated stream id, and cleans up with `stopSse(streamId)`. | No packaged desktop SSE walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer SSE does not use `runtimeRequest` as a side channel and remains behind `window.analytix.runtime`. | Browser/main/preload reconnect and URL/header proofs remain separate evidence. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts --no-file-parallelism --maxWorkers=1
```

## 111. 2026-06-22 Main/Preload SSE IPC Bridge Score Rule

Scoring rule:

```text
Runtime contract evidence improves when desktop SSE IPC proves preload channel
mapping, main route/cursor headers, and exact stream-id stop behavior.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `preload-sandbox.test.ts`, `app-ipc-schemas.test.ts`, and `runtime-sse-ipc.test.ts` prove SSE IPC mapping, strict start payload shape, analytix events route, initial cursor headers, generated stream ids, and exact stop matching. | No packaged desktop SSE walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Desktop SSE stays under `window.analytix.runtime` and `runtime:sse:*` IPC without Reasonix/Kun/Go/Workflow public surfaces. | Browser/main/preload live walkthrough remains separate release evidence. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Evidence:

```text
npm run test -- src/main/runtime-sse-ipc.test.ts src/main/ipc/app-ipc-schemas.test.ts src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

## 112. 2026-06-22 Main Runtime Request Forbidden Route Score Rule

Scoring rule:

```text
Runtime contract evidence improves when main IPC schema and handler tests
explicitly reject upstream public runtime route families before adapter calls.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `app-ipc-schemas.test.ts` and `register-app-ipc-handlers.test.ts` reject Reasonix/Go/Workflow-style runtime request routes and prove invalid payloads do not call `runtimeRequest`. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Main IPC allows only modeled analytix endpoint templates; upstream public route strings remain negative test evidence. | Live desktop route QA remains separate. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Evidence:

```text
npm run test -- src/main/ipc/app-ipc-schemas.test.ts src/main/ipc/register-app-ipc-handlers.test.ts --no-file-parallelism --maxWorkers=1
```

## 113. 2026-06-22 Preload Runtime Request IPC Bridge Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when preload source guards prove runtime
requests can use only the analytix runtime:request IPC channel.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `preload-sandbox.test.ts` proves `runtimeRequest(path, method, body)` maps to `runtime:request` with `{ path, method, body }`. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Source guard rejects Reasonix/Kun/Go/Workflow request IPC channel names. | Live desktop route QA remains separate. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Evidence:

```text
npm run test -- src/preload/preload-sandbox.test.ts --no-file-parallelism --maxWorkers=1
```

## 114. 2026-06-22 Runtime Desktop Bridge Proof Freshness Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when bridge proof source/test files are
required by the reusable scan, not only by release notes.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `runtimeDesktopBridgeProofPaths` requires renderer/preload/main runtime request/SSE bridge sources and guard tests to remain present. | No packaged desktop route/SSE walkthrough. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now guards D-0156 and D-0162 through D-0166 proof path freshness. | Live desktop route QA remains separate. |
| Go G5 readiness | unchanged | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Evidence:

```text
npm run scan:product-sovereignty
```

## 115. 2026-06-22 Provider Cache Coverage Floor Score Rule

Scoring rule:

```text
Provider/cache evidence improves when the oracle schema and runtime tests make
the required provider-family matrix executable instead of prose-only.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `provider-cache-proof.test.ts` pins required usage ids, request-shape ids, endpoint formats, telemetry-supported cases, unsupported unknown cache posture, and custom full endpoint cases. | No credentialed live provider matrix or packaged provider QA. |
| Go G3/G5 readiness | evidence improves; score unchanged | G3/G5 shadow continues to parse the tightened TS provider-cache oracle. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 116. 2026-06-22 G5 Provider Cache Coverage Floor Control Shadow Score Rule

Scoring rule:

```text
Provider/cache and Go readiness evidence improves when the G5 executable
shadow computes the provider-family coverage floor from TS-owned fixtures.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerCacheCoverageFloor` covers provider families, telemetry-supported ids, unsupported unknown ids, and custom full endpoint exact-URL/tool-shape evidence. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits provider cache coverage floor output and Go tests compare it to the TS oracle. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | Output keeps no Reasonix protocol, no top-level route, no live credential, and no live superiority flags. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 104. 2026-06-22 Goal Persistence Off-Lock Score Rule

Scoring rule:

```text
Go G5 readiness evidence improves when a future lock-split requirement is
grounded in current TypeScript source behavior and executable shadow output.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `goalPersistenceOffLock` computes persist-before-event order, no forbidden lock substrings, and warning/error proof. | No live Go goal manager or rollback readiness. |
| Goal reliability | evidence improves; score unchanged | `ThreadService` failure warnings and original error surfacing remain source/test-derived. | No packaged goal workflow QA. |
| Product sovereignty | unchanged | No Reasonix controller protocol, Kun identity, Go route/default backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/thread-service.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 86. 2026-06-22 Tool Result File/Image Boundary Score Rule

Scoring rule:

```text
Go G5 readiness and generated-file reliability evidence improve when
tool-result image/file preservation is source-derived and executable-shadowed.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `toolResultFileImageBoundary` computes inline image kind preservation, base64 eviction, newest-image cap, path/fallback propagation, and generated-file meta flags. | No live Go file/image bridge or rollback readiness. |
| Generated-file reliability | evidence improves; score unchanged | Renderer mapper and attachment-store tests keep `localFilePath`, fallback `FilePath`, tool attachments, and generated files visible. | No packaged generated-file/attachment QA. |
| Product sovereignty | unchanged | No Reasonix file protocol, Kun identity, Go route/default backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts src/loop/tool-result-image.test.ts tests/attachment-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 87. 2026-06-22 Event JSONL Replay Boundary Score Rule

Scoring rule:

```text
Go G5 readiness and replay reliability evidence improve when event JSONL
append/replay/recovery behavior is source-derived and executable-shadowed.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `eventJsonlReplayBoundary` computes append newline, replay filter/sort, highestSeq, malformed recovery, recorder ordering, seq uniqueness, and usage compaction flags. | No live Go event store or rollback readiness. |
| Replay reliability | evidence improves; score unchanged | File session and runtime recorder tests keep event replay, malformed recovery, and compaction failure recovery source-derived. | No packaged long-thread replay QA. |
| Product sovereignty | unchanged | No Reasonix event protocol, Kun identity, Go route/default backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/runtime-event-recorder.test.ts tests/file-session-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 88. 2026-06-22 MCP Malformed Schema Boundary Score Rule

Scoring rule:

```text
Go G5 readiness and MCP catalog safety evidence improve when malformed schema
normalization is source-derived and executable-shadowed.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `mcpMalformedSchemaBoundary` computes safe schema defaulting, invalid property dropping, string-only required filtering, advertised names, and output-schema omission. | No live Go MCP client or rollback readiness. |
| MCP catalog safety | evidence improves; score unchanged | MCP provider tests keep malformed `inputSchema` normalization source-derived before advertisement. | No credentialed MCP matrix or packaged MCP QA. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, Kun identity, Go route/default backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/mcp-tool-provider.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 117. 2026-06-22 Provider Request-Shape Derived Replay Score Rule

Scoring rule:

```text
Provider request safety evidence improves when URL/header/body/tool-shape
matches are derived by G3/G5 shadow instead of only copied as fixture rows.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | Request-shape summaries now include derived URL/header/body/tool-shape match ids and custom full endpoint exact-url ids. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | Go G3/G5 shadow computes request-shape derived match fields and compares them to TS-owned fixtures. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No provider runtime behavior change, Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/provider-cache-proof.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 118. 2026-06-22 Post-881 Runtime Proof Freshness V2 Score Rule

Scoring rule:

```text
Absorption safety evidence improves when reusable scans require key post-881
proof leaves and positive proof tokens to remain present.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now pins provider, planner/task, and MCP proof tokens. | No packaged desktop walkthrough or release QA. |
| Runtime contract safety | evidence improves; score unchanged | Task-job and MCP lifecycle oracle/test leaves are now required by runtime proof freshness. | Scan freshness is not behavior parity. |
| Go G5 readiness | unchanged | Go remains shadow-only; no backend behavior changed. | No live Go provider/MCP/job clients, Electron integration, rollback, or default backend. |

Evidence:

```text
npm run scan:product-sovereignty
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/task-job-orchestration-oracle.test.ts tests/mcp-tool-lifecycle-oracle.test.ts tests/create-plan-tool.test.ts --no-file-parallelism --maxWorkers=1
```

## 119. 2026-06-22 Provider Cache Accounting Raw Payload Score Rule

Scoring rule:

```text
Provider/cache evidence improves when cache accounting is derived from raw
provider usage payloads instead of only copied expected summaries.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | TS and Go G3/G5 shadow parse raw `responseBody.usage` for DeepSeek, OpenAI Responses, Anthropic, and unsupported OpenAI-compatible cases before computing cache accounting. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG3ProviderConformanceOutput` and `BuildG5ControlExecutableOutput` compute raw parsed ids, telemetry-supported ids, expected-usage match ids, totals, and aggregate rate from fixtures. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No provider runtime behavior change, Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 120. 2026-06-22 Provider Raw Accounting Proof Freshness Score Rule

Scoring rule:

```text
Provider/cache evidence improves when raw accounting is a direct oracle proof
and reusable scan-freshness requirement.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `provider-cache-proof.test.ts` derives raw usage snapshots and accounting totals from provider response bodies directly. | No credentialed live provider matrix or packaged provider QA. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now pins raw accounting proof tokens so the D-0172 proof cannot silently disappear. | Scan freshness is not live parity. |
| Go G5 readiness | unchanged | Go remains shadow-only; the TS oracle that Go must match is stronger. | No live Go provider client, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 121. 2026-06-22 Combined Step/Cancel/Cache Proof Freshness Score Rule

Scoring rule:

```text
Agent-loop control evidence improves when combined step/cancel/cache behavior
is a focused oracle proof and reusable scan-freshness requirement.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Agent-loop reliability | evidence improves; score unchanged | `go-runtime-conformance.test.ts` now has a dedicated combined trace test for route-cache reuse, step-limit boundary, cancel result pairing, and stable-prefix isolation. | No packaged long-running cancel/cache walkthrough. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now pins combined proof tokens so the D-0146 proof cannot silently disappear. | Scan freshness is not live loop parity. |
| Go G5 readiness | unchanged | Go remains shadow-only; the TS/G5 oracle that Go must match is stronger. | No live Go agent loop, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 122. 2026-06-22 Approval/User-Input Proof Freshness Score Rule

Scoring rule:

```text
Approval/user-input safety evidence improves when gate behavior is a focused
oracle proof and reusable scan-freshness requirement.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Approval/user-input safety | evidence improves; score unchanged | `go-runtime-conformance.test.ts` now directly checks denied no-execute, answer privacy, late rejection, abort cleanup, and resume cleanup. | No packaged desktop approval-card walkthrough. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now pins approval/user-input proof tokens so the gate proof cannot silently disappear. | Scan freshness is not live gate parity. |
| Go G5 readiness | unchanged | Go remains shadow-only; the TS/G5 oracle that Go must match is stronger. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 123. 2026-06-22 Session Route Proof Freshness Score Rule

Scoring rule:

```text
Thread/session route safety evidence improves when route replay is a focused
oracle proof and reusable scan-freshness requirement.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime route safety | evidence improves; score unchanged | `go-runtime-conformance.test.ts` now directly checks archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, exact hashes, and boundary flags. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now pins session route proof tokens so route/SSE proof cannot silently disappear. | Scan freshness is not live route parity. |
| Go G5 readiness | unchanged | Go remains shadow-only; the TS/G5 oracle that Go must match is stronger. | No live Go thread/session router, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
git diff --check
```

## 124. 2026-06-22 Release Evidence Final-Gate Freshness Score Rule

Scoring rule:

```text
Stage-closure evidence improves when post-881 final command gates are
machine-checkable by the product-sovereignty scan.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Stage closure evidence | evidence improves; score unchanged | `scan:product-sovereignty` now requires final command gate entries for D-0172 through D-0177. | The scan does not execute those commands by itself. |
| Product sovereignty | evidence improves; score unchanged | Release evidence path freshness is part of the same forbidden-surface scan. | Scan freshness is not packaged/live QA. |
| Release readiness | unchanged | No runtime, provider, bridge, Go backend, packaging, or product entry behavior changed. | Packaged desktop QA, live provider/MCP matrices, and G6 remain separate blockers. |

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

## 103. 2026-06-22 Custom Provider Telemetry Seal Score Rule

Scoring rule:

```text
Provider/cache evidence improves when custom endpoint request-shape coverage is
explicitly separated from supported cache telemetry.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `providerCacheCoverageFloor.expected` now has empty custom telemetry ids and request-shape-only booleans. | No credentialed custom provider cache matrix. |
| Go G5 readiness | evidence improves; score unchanged | Go G5 shadow computes the same seal from fixture inputs. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, Kun identity, hidden top-level entry, Go backend, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 125. 2026-06-22 Post-881 Stage Closure Snapshot Score Rule

Scoring rule:

```text
Stage-closure evidence improves when the current capability floor, stronger
than evidence, and remaining open gates are machine-scannable.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Stage closure evidence | evidence improves; score unchanged | `post-881-stage-closure-2026-06-22.md` records capability floor, absorbed deltas, Kun baseline, stronger-than areas, and open gates. | Snapshot is not live parity or release readiness. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` now requires closure tokens and preserves forbidden-surface boundaries. | Scan freshness is not packaged/live QA. |
| Reasonix comparison | evidence improves; score unchanged | Stronger-than claims are limited to verified fixture/contract/governance dimensions. | Live provider/cache superiority, live MCP parity, live Go backend, and packaged QA remain open. |

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

## 102. 2026-06-22 Context Compaction Boundary Control Shadow Score Rule

Scoring rule:

```text
Long-thread/cache evidence improves when latest-compaction effective-history
boundary behavior is computed by typed Go G5 control shadow instead of only a
runtime loop test.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.compactionBoundary` computes effective/dropped ids and latest compaction boundary flags. | No packaged desktop long-history walkthrough. |
| Provider/cache reliability | evidence improves; score unchanged | Compaction control state remains outside stable prefix and noop/older compactions do not leak into the effective model history boundary. | No credentialed live cache matrix. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed compaction boundary output. | No live Go history manager, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/controller protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 120. 2026-06-23 Go Minimal Agent Loop Proof Score Rule

Scoring rule:

```text
Go runtime kernel evidence improves when a conformance-only Go loop proof
executes over HTTP/SSE against TS-owned fixtures and durable replay, while
keeping production backend, Electron, live provider, and live MCP gates closed.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; backend score unchanged | `go-minimal-agent-loop-oracle.json`, `live_local_loop.go`, `live_local_loop_test.go`, and `go-minimal-agent-loop-conformance.test.ts` prove a fixture-scripted Go loop can replay single-turn input, stable prefix/tool fingerprint, model request shape, approval/user-input/MCP/cache events, step/cancel/resume, and recovered state. | No production Go loop, live provider client, live MCP client, approval execution manager, rollback gate, Electron integration, or default backend. |
| Runtime contract reliability | evidence improves; score unchanged | Loop events persist through the D-0236 temp durable store before replay/SSE, and TS conformance starts the Go sidecar instead of using source-string-only assertions. | No packaged desktop restart/crash drill or long-thread QA. |
| Provider/cache/cost efficiency | evidence improves; score unchanged | DeepSeek, OpenAI Responses, and Anthropic Messages hit/miss telemetry survives loop replay without changing custom endpoint behavior. | No credentialed provider matrix or live superiority claim. |
| Product sovereignty | evidence improves; score unchanged | Routes remain under `/v1/conformance/loop/*`, `window.analytix`, top-level `runtime` settings, `analytix serve`, and Kun-derived desktop entry surfaces remain unchanged. | No G6/default backend decision or release readiness. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-minimal-agent-loop-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 130. 2026-06-23 D-0236 Go Temp Durable Store Score Rule

Scoring rule:

```text
Go runtime durability evidence improves when temp-dir event/session storage and
SSE replay are executable in Go and called by TypeScript conformance.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime durability | evidence improves; score unchanged | Go appends newline-terminated temp `events.jsonl`, assigns stable seq, skips malformed lines with diagnostics, and replays by cursor. | No production workspace store or packaged crash drill. |
| Runtime contract safety | evidence improves; score unchanged | Vitest starts the Go conformance sidecar and proves `Last-Event-ID` / `since_seq`, caught-up replay, thread/session temp state, and recovered cache/gate/MCP state. | No Electron integration or renderer-visible Go route. |
| Provider/cache proof | evidence improves; score unchanged | Durable replay preserves DeepSeek/OpenAI/Anthropic cache hit/miss telemetry without changing provider request contracts. | No live credentialed provider matrix or live provider superiority claim. |
| Go backend readiness | base improves; backend score unchanged | D-0236 creates a reversible temp durable base for later Go loop work. | No live Go agent loop, default backend, G6, rollback drill, or release readiness. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm --prefix packages/runtime test -- tests/go-durable-sidecar-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 120. 2026-06-23 Go Live-Local Sidecar Prototype Score Rule

Scoring rule:

```text
Go runtime kernel evidence improves when a real local Go HTTP sidecar can serve
TS-owned G1/G2 fixture contracts while keeping rollback/default-backend guards
false.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go runtime kernel | live-local prototype evidence improves; backend score unchanged | `NewLiveLocalSidecarHandler` and `TestLiveLocalSidecarPrototypeMatchesTypeScriptOracle` start a real local Go HTTP server for G1 health/info/tools plus G2 GET-only route/SSE replay. | No Electron integration, default backend, G6 readiness, durable Go store, live provider/MCP/gate/file mutation, or packaged QA. |
| Runtime contract reliability | evidence improves; score unchanged | Status/body/SSE frames are compared to TS-owned G1/G2 fixtures, and G5 guards keep `electronMainConnected`, `defaultGoBackendEnabled`, and `rendererVisibleGoRoutesAllowed` false. | Durable/production mutating route replay and live runtime services remain future gates; D-0233 covers only isolated in-memory G2 lifecycle replay. |
| Product sovereignty | evidence improves; score unchanged | Product scan requires sidecar tokens while forbidden route, bridge alias, default Go backend, and Rust/Tauri scans remain active. | Negative scans do not replace packaged desktop walkthroughs. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 120A. 2026-06-23 Go Live-Local Isolated Mutating G2 Lifecycle Score Rule

Scoring rule:

```text
Go runtime kernel evidence improves when the live-local sidecar can execute the
TS-owned G2 mutating lifecycle oracle routes against isolated in-memory fixture
state while keeping all production/backend guards false.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go runtime kernel | isolated mutating prototype evidence improves; backend score unchanged | `NewLiveLocalSidecarHarness`, `live_local_store.go`, and `TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptOracle` execute the four G2 mutating routes and verify exact oracle status/body. | No Electron integration, default backend, G6 readiness, durable Go store, live provider/MCP/gate/file mutation, or packaged QA. |
| Runtime contract reliability | evidence improves; score unchanged | The mutation sequence records archive/update/fork/resume state in `LiveLocalSidecarSnapshot`, and a temp `events.jsonl` sentinel proves no real event-log write. | No general Go router, real workspace mutation, or production event persistence. |
| Product sovereignty | evidence improves; score unchanged | Forbidden `/v1/runtime/go`, Reasonix, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes remain 404 in the live-local test server; rollback/default-backend guards remain false. | Negative scans do not replace packaged desktop walkthroughs. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```

## 120B. 2026-06-23 Go Live-Local G3 Provider/Cache Streaming Score Rule

Scoring rule:

```text
Go runtime kernel and provider/cache evidence improves when the live-local
sidecar can replay TS-owned G3 provider/cache usage, request-shape, streaming,
accounting, drift, and diagnostics fixtures while keeping all production,
credential, network, and backend guards false.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go runtime kernel | G3 live-local provider/cache prototype evidence improves; backend score unchanged | `live_local_provider.go` and focused Go tests execute fixture-backed provider usage, request-shape, streaming, cache accounting, drift, and diagnostics routes through a real local `httptest.Server`. | No Electron integration, default backend, G6 readiness, durable Go store, live provider client, live MCP/gate/file mutation, or packaged QA. |
| Provider/cache efficiency | fixture depth improves; live score unchanged | The harness proves DeepSeek native cache precedence, OpenAI Responses cached tokens, Anthropic cache read/create accounting, unsupported unknown telemetry, aggregate hit/miss accounting, and bounded stable-prefix diagnostics. | No credentialed provider matrix, live external cache telemetry, cost reconciliation, or live provider/cache superiority. |
| Runtime contract reliability | evidence improves; score unchanged | Exact request-shape families and `item_delta -> usage -> turn_completed` SSE order are replayed without changing `analytix serve` or TypeScript provider request behavior. | No production Go provider streaming or durable event recorder. |
| Product sovereignty | evidence improves; score unchanged | Tests and scan require no external network, no API-key read, no provider credentials, no Reasonix protocol, no renderer-visible Go route, and no default Go backend. | Negative scans do not replace packaged desktop walkthroughs. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 120C. 2026-06-23 Go Live-Local G4 Approval/User-Input/MCP Manager Score Rule

Scoring rule:

```text
Go runtime kernel and manager-safety evidence improves when the live-local
sidecar can replay TS-owned G4 approval, user-input, remote-entry, and MCP
lifecycle fixtures while keeping all production execution, credential,
connection, file, and backend guards false.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go runtime kernel | G4 isolated manager prototype evidence improves; backend score unchanged | `live_local_g4.go` and focused Go tests execute fixture-backed approval/user-input/MCP manager routes through a real local `httptest.Server`. | No Electron integration, default backend, G6 readiness, durable Go store, live approval manager, live user-input manager, live MCP client, live file mutation, or packaged QA. |
| Approval/user-input safety | fixture depth improves; live score unchanged | The harness proves approval deny no-execute, second decision rejection, user-input cancel/submit status/body shape, structured invalid-choice no-gate behavior, HTTP answer echo, answer-free resolved event evidence, and remote `disableUserInput` preservation. | No live Go approval execution parity, renderer approval-card QA, or production user-input backend replacement. |
| MCP manager safety | fixture depth improves; live score unchanged | The harness proves MCP connect/reload/disconnect/cancel/error replay, approval annotation deny no-execute, search meta-tool advertisement, untrusted workspace hiding, unknown tool error, transport retry/protocol no-retry classification, background reconnect classification, known override diagnostics, and secret redaction. | No live Go MCP client, credentialed MCP matrix, MCP-indexer product surface, or packaged MCP QA. |
| Product sovereignty | evidence improves; score unchanged | Snapshot counters keep approval execution, tool execution, MCP connection, credential read, file mutation, `events.jsonl` write, and real workspace write attempts at `0`; forbidden public routes remain absent. | Negative scans do not replace packaged desktop walkthroughs. |

Evidence:

```text
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 121. 2026-06-22 Runtime HTTP Auth Matrix Score Rule

Scoring rule:

```text
Runtime contract evidence improves when every registered `/v1/*` route from
the route-sovereignty fixture is actually dispatched without auth and returns
the canonical structured 401.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `go-runtime-conformance.test.ts` dispatches all D-0189 routes and proves 43 `/v1/*` routes return structured 401 while `/health` remains public. | No live Go HTTP server or packaged route walkthrough. |
| Approval/user-input and task control safety | evidence improves; score unchanged | SSE, task-job, approval, user-input, and resume-thread route keys are explicitly covered by the unauthenticated matrix. | No packaged desktop card or live task-job QA. |
| Product sovereignty | evidence improves; score unchanged | The matrix is driven by analytix-owned route fixtures, not Reasonix SessionAPI or public control-plane names. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 123. 2026-06-22 Shared Endpoint Builder Sovereignty Score Rule

Scoring rule:

```text
Runtime contract evidence improves when shared endpoint builders URL-encode
route ids and exported templates reject upstream/hidden route identity.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `src/shared/analytix-endpoints.test.ts` proves path builders encode route ids and preserve canonical analytix templates. | No live Go HTTP server or packaged route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Exported shared endpoint strings contain no Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer/session-api tokens. | Negative fixtures still retain forbidden strings as evidence. |
| Approval/user-input safety | evidence improves; score unchanged | Canonical shared user-input endpoint remains plural `/v1/user-inputs/{id}`; singular route stays compatibility-only. | No packaged approval-card QA. |

Evidence:

```text
npm run test -- src/shared/analytix-endpoints.test.ts
```

## 124. 2026-06-22 Renderer Runtime Endpoint Builder Score Rule

Scoring rule:

```text
Renderer/runtime contract evidence improves when the GUI provider uses shared
endpoint constants/builders and URL-encodes dynamic route ids before bridge
requests.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `AnalytixRuntimeProvider` uses shared health/thread root constants and tests prove dynamic route ids are encoded before `runtimeRequest`. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer runtime request paths remain `/health` or `/v1/*` and exclude forbidden upstream/hidden route tokens. | Negative fixtures still retain forbidden strings as evidence. |
| Bridge safety | evidence improves; score unchanged | Sensitive approval/user-input/session/fork/turn paths cross only through `window.analytix.runtime.runtimeRequest` with encoded ids. | No live Go HTTP server or G6 backend selection. |

Evidence:

```text
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts
```

## 125. 2026-06-22 Main IPC Endpoint Builder Allow-list Score Rule

Scoring rule:

```text
Runtime/bridge contract evidence improves when the main IPC runtime request
allow-list accepts shared endpoint builder output for encoded dynamic ids and
rejects raw compatibility drift before the runtime adapter.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `src/main/ipc/app-ipc-schemas.test.ts` proves encoded shared builder paths pass the main IPC `runtimeRequestPayloadSchema`. | No packaged desktop route walkthrough. |
| Bridge safety | evidence improves; score unchanged | Raw extra-segment dynamic ids and singular `/v1/user-input/:id` are rejected before crossing from main IPC into the runtime adapter. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | Main IPC remains limited to analytix-owned `/health` or `/v1/*` modeled templates, not Reasonix SessionAPI or hidden route names. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/main/ipc/app-ipc-schemas.test.ts
```

## 126. 2026-06-22 Main IPC Runtime Adapter Handoff Score Rule

Scoring rule:

```text
Runtime/bridge contract evidence improves when the registered main IPC handler
preserves encoded shared endpoint paths into the runtime adapter and proves
invalid routes are not forwarded.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `src/main/ipc/register-app-ipc-handlers.test.ts` proves encoded builder paths are passed to `runtimeRequest` unchanged. | No packaged desktop route walkthrough. |
| Bridge safety | evidence improves; score unchanged | Handler tests prove method/body preservation and invalid raw/singular paths do not call the runtime adapter. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | The real IPC handler remains bound to analytix-owned `/health` or `/v1/*` routes, not Reasonix SessionAPI or hidden route names. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/main/ipc/register-app-ipc-handlers.test.ts
```

## 127. 2026-06-22 Preload Runtime Request Bridge Score Rule

Scoring rule:

```text
Runtime/bridge contract evidence improves when the preload `window.analytix`
facade preserves encoded runtime request paths into the main IPC channel.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `src/preload/preload-runtime-request.test.ts` loads the exposed `analytix` API and proves encoded path/method/body are passed to `runtime:request` unchanged. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Runtime and diagnostics calls use the same analytix IPC channel and no deprecated bridge alias. | Negative fixtures still retain forbidden strings as evidence. |
| Runtime contract safety | evidence improves; score unchanged | Preload proof connects D-0193 renderer builders to D-0194/D-0195 main IPC validation/handoff. | No live Go HTTP server or G6 backend selection. |

Evidence:

```text
npx vitest run src/preload/preload-runtime-request.test.ts
```

## 128. 2026-06-22 Runtime Host URL Handoff Score Rule

Scoring rule:

```text
Runtime contract evidence improves when the main runtime adapter preserves
encoded shared endpoint paths and request metadata into the managed runtime
HTTP host.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `src/main/runtime/analytix-adapter.test.ts` uses a real local HTTP server to prove encoded path/query preservation. | No packaged desktop route walkthrough. |
| Bridge safety | evidence improves; score unchanged | Method, bearer auth, custom header, content type, and body are preserved into the runtime host request. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | The runtime target remains managed `analytix serve`, not Reasonix SessionAPI or a default Go backend. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/main/runtime/analytix-adapter.test.ts
```

## 129. 2026-06-22 Preload SSE Bridge Score Rule

Scoring rule:

```text
Runtime/SSE bridge evidence improves when the preload `window.analytix` facade
preserves SSE start/stop arguments and event payload delivery on analytix-owned
IPC channels.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `src/preload/preload-sse-bridge.test.ts` loads the exposed `analytix` API and proves `startSse` / `stopSse` argument preservation. | No packaged desktop SSE walkthrough. |
| Runtime contract safety | evidence improves; score unchanged | Event/end/error wrappers forward payloads only and clean up listeners on unsubscribe. | No live Go SSE server or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | SSE bridge traffic stays on `runtime:sse:*` under `window.analytix`, not Reasonix SessionAPI or deprecated aliases. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/preload/preload-sse-bridge.test.ts
```

## 130. 2026-06-22 Main SSE Host URL Encoding Score Rule

Scoring rule:

```text
Runtime/SSE contract evidence improves when main SSE IPC encodes thread ids and
preserves cursor headers before fetching managed runtime event routes.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `src/main/runtime-sse-ipc.test.ts` proves dangerous thread ids are encoded into `/v1/threads/{id}/events`. | No packaged desktop SSE walkthrough. |
| Bridge safety | evidence improves; score unchanged | `since_seq`, `Last-Event-ID`, `Accept`, auth, and stream id error delivery are preserved. | No live Go SSE server or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | SSE host route remains analytix-owned and excludes Reasonix SessionAPI or hidden route families. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/main/runtime-sse-ipc.test.ts
```

## 131. 2026-06-22 Renderer Runtime Client Bridge Score Rule

Scoring rule:

```text
Renderer bridge evidence improves when runtime request and SSE client facades
use only `window.analytix.runtime` and preserve arguments without legacy bridge
fallback.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `src/renderer/src/agent/runtime-client.test.ts` proves runtime request, SSE start/stop, and listener handler arguments are forwarded unchanged. | No packaged desktop runtime walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Throwing `window.kun` / `window.reasonix` getters prove deprecated bridge aliases are not read. | Negative fixtures still retain forbidden strings as evidence. |
| Runtime contract safety | evidence improves; score unchanged | Renderer client proof connects provider/renderer usage to the preload/main/runtime host proof chain. | No live Go bridge or G6 backend selection. |

Evidence:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

## 132. 2026-06-22 Renderer Settings Bridge Score Rule

Scoring rule:

```text
Renderer settings bridge evidence improves when top-level runtime settings
patches use only `window.analytix.settings` and avoid legacy bridge fallback.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | `src/renderer/src/agent/runtime-client.test.ts` proves `setSettings` forwards a top-level `runtime` patch through `window.analytix.settings` unchanged and caches the returned settings. | No packaged desktop settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Throwing `window.kun` / `window.reasonix` getters prove deprecated bridge aliases are not read. | Negative fixtures still retain forbidden strings as evidence. |
| Runtime contract safety | evidence improves; score unchanged | Renderer settings proof keeps active fields under top-level `runtime`, not Reasonix config roots or old runtime-shaped settings fallback. | No live Go bridge or G6 backend selection. |

Evidence:

```text
npx vitest run src/renderer/src/agent/runtime-client.test.ts
```

## 133. 2026-06-22 Renderer Runtime Provider Alias Guard Score Rule

Scoring rule:

```text
Renderer provider bridge evidence improves when provider route and interaction
tests run with legacy bridge aliases guarded by throwing getters.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `src/renderer/src/agent/analytix-runtime.test.ts` installs throwing legacy alias getters for all provider bridge tests. | No packaged desktop runtime walkthrough. |
| Approval/user-input safety | evidence improves; score unchanged | Existing provider tests cover approval/user-input submit/cancel, fork/resume, and dynamic route-id encoding under the alias guard. | No packaged approval-card QA. |
| Product sovereignty | evidence improves; score unchanged | Provider route assertions stay analytix-owned and do not introduce Reasonix SessionAPI, Go, or hidden capability route families. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
```

## 134. 2026-06-22 Renderer Provider Runtime Client Facade Seal Score Rule

Scoring rule:

```text
Renderer provider bridge evidence improves when provider runtime requests are
sealed behind `rendererRuntimeClient` and direct bridge bypasses are scanned.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `archiveThread` now uses `rendererRuntimeClient.runtimeRequest`, matching the rest of the provider runtime request surface. | No packaged desktop runtime walkthrough. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` forbids direct `window.analytix.runtime.runtimeRequest` calls in `analytix-runtime.ts`. | Negative fixtures still retain forbidden strings as evidence. |
| Thread lifecycle safety | evidence improves; score unchanged | Existing archive/restore tests keep the same route/body under the provider alias guard. | No packaged archive/search walkthrough. |

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts
npm run scan:product-sovereignty
```

## 135. 2026-06-22 Side Conversation Relation Provider Contract Score Rule

Scoring rule:

```text
Side conversation bridge evidence improves when promotion uses an
analytix-owned provider relation contract instead of a direct runtime bridge
request from store code.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `promoteSideConversation` calls `provider.updateThreadRelation(sideId, 'primary')` instead of `window.analytix.runtime.runtimeRequest`. | No packaged side-conversation walkthrough. |
| Thread lifecycle safety | evidence improves; score unchanged | `AnalytixRuntimeProvider.updateThreadRelation` sends the same relation PATCH through `rendererRuntimeClient.runtimeRequest`. | No live Go bridge or G6 backend selection. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` forbids direct runtime bridge requests in provider and side-store sources. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts
npm run scan:product-sovereignty
```

## 136. 2026-06-22 Renderer Usage Runtime Client Facade Seal Score Rule

Scoring rule:

```text
Renderer usage/debug bridge evidence improves when generic runtime HTTP
requests use `rendererRuntimeClient` and direct bridge bypasses are forbidden
across renderer production source.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | Thread/day/model usage loaders plus settings diagnostics call `rendererRuntimeClient.runtimeRequest`. | No packaged usage/dashboard walkthrough. |
| Provider/cache observability | evidence improves; score unchanged | Existing usage tests preserve cache/cost parsing and request path construction after transport sealing. | No live provider/cache superiority matrix. |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` forbids direct generic runtime request bridge calls in renderer production source. | Named preload APIs remain separate contracts. |

Evidence:

```text
npx vitest run src/renderer/src/hooks/use-thread-usage.test.ts src/renderer/src/hooks/use-daily-usage.test.ts src/renderer/src/hooks/use-model-usage.test.ts src/renderer/src/components/settings-section-agents.test.ts
npm run scan:product-sovereignty
```

## 136a. 2026-06-23 Renderer Usage Runtime Client Facade G5 Shadow Score Rule

Scoring rule:

```text
Renderer usage/debug bridge evidence further improves when the facade seal is
also computed by typed Go G5 desktop sovereignty shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | `desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` binds thread/day/model usage loaders and settings diagnostics to `rendererRuntimeClient.runtimeRequest`. | No packaged usage/dashboard walkthrough. |
| Provider/cache observability | evidence improves; score unchanged | Usage unit proofs remain tied to request paths and parsing while Go shadow replays the transport seal. | No live provider/cache superiority matrix. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits usage facade coverage, diagnostics coverage, direct bridge rejection, scan guard, and unit-proof booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137. 2026-06-22 Renderer Settings Read Facade Seal Score Rule

Scoring rule:

```text
Renderer settings bridge evidence improves when ordinary settings reads use
`rendererRuntimeClient` and direct settings read bypasses are forbidden across
renderer production source.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | Keyboard shortcuts, speech-to-text, and initial usage model-label reads call `rendererRuntimeClient.getSettings`. | No packaged settings walkthrough. |
| Bridge safety | evidence improves; score unchanged | `scan:product-sovereignty` forbids direct `window.analytix.settings.getSettings` in renderer production source. | Named write APIs remain separate contracts. |
| Product sovereignty | evidence improves; score unchanged | No Reasonix config root, deprecated bridge alias, or old runtime-shaped settings fallback is introduced. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npx vitest run src/renderer/src/components/chat/InitialSessionUsageHeatmap.test.ts src/renderer/src/agent/runtime-client.test.ts
npm run scan:product-sovereignty
```

## 137a. 2026-06-23 Renderer Settings Read Facade G5 Shadow Score Rule

Scoring rule:

```text
Renderer settings-read evidence further improves when the facade seal is also
computed by typed Go G5 desktop sovereignty shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | `desktopSovereignty.rendererSettingsReadFacadeMatrix` binds keyboard shortcut, speech-to-text, and usage model-label reads to `rendererRuntimeClient.getSettings`. | No packaged settings walkthrough. |
| Bridge safety | evidence improves; score unchanged | Go shadow replays direct settings bridge rejection and scan guard presence. | Named write APIs remain separate contracts. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits settings reader coverage, event-sync preservation, direct bridge rejection, and scan guard booleans. | No live Go desktop/settings bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137b. 2026-06-23 AutoResearch Direction Tracking G5 Shadow Score Rule

Scoring rule:

```text
AutoResearch quality evidence improves when attempted research directions are
durably audited and replayed by typed Go G5 full-loop shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Long-task reliability | evidence improves; score unchanged | `record_research_direction` writes `directions_tried.json` and appends `direction_recorded` to `iteration_log.jsonl`. | No packaged long-task walkthrough. |
| Goal/runtime safety | evidence improves; score unchanged | Direction records require an active research goal and unknown requirement evidence remains rejected. | No public Reasonix project protocol. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits direction file, iteration-log, tool-name, and active-goal guard booleans. | No live Go AutoResearch backend, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- go-runtime-conformance
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137c. 2026-06-23 MCP Call-Time Reconnect G5 Shadow Score Rule

Scoring rule:

```text
MCP/tool lifecycle quality evidence improves when transient transport failures
and deterministic protocol errors are classified separately and replayed by
typed Go G5 shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP reliability | evidence improves; score unchanged | Stale connection failures reconnect once and succeed; protocol validation errors do not reconnect. | No credentialed MCP matrix. |
| Tool safety | evidence improves; score unchanged | Deterministic MCP protocol errors return `tool_execution_failed` without losing session state. | No packaged MCP walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits MCP call reconnect classification. | No live Go MCP client, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/mcp-tool-provider.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137d. 2026-06-23 Plan Step/Cancel/Cache G5 Shadow Score Rule

Scoring rule:

```text
Planner/cancel/cache quality evidence improves when explicit Plan-mode tool
gating, aborted follow-up handling, retry baseline reuse, and provider cache
telemetry are replayed by typed Go G5 shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner reliability | evidence improves; score unchanged | Step 0 advertises `create_plan` + `ls`, excludes `bash`, and the follow-up is forced to `create_plan`. | No public auto-plan setting. |
| Cancel/cache stability | evidence improves; score unchanged | Aborted plan follow-up keeps `prefixChanged: false`, empty prefix-change reasons, and retry baseline reuse. | No packaged Plan-mode walkthrough. |
| Provider/cache accounting | evidence improves; score unchanged | DeepSeek `chat_completions` usage preserves `80` hit tokens and `20` miss tokens across the cancelled plan sequence. | No live provider superiority claim. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits Plan step/cancel/cache stability outputs. | No live Go loop, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/loop.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137e. 2026-06-23 Plan/Auto-Route State Reset G5 Shadow Score Rule

Scoring rule:

```text
Planner/currentness quality evidence improves when cancelled Plan state cannot
leak into later normal or auto-routed turns, and the next auto turn reruns the
classifier through typed Go G5 shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner reliability | evidence improves; score unchanged | Post-cancel normal/auto turns have no Plan mode instruction or required `create_plan`. | No public auto-plan setting. |
| Auto-router currentness | evidence improves; score unchanged | Post-cancel `model: "auto"` reruns `_auto_router` once and accepts `deepseek-v4-pro` / `max`. | No Reasonix controller protocol. |
| Stable-prefix safety | evidence improves; score unchanged | Plan/classifier state remains outside the stable prefix. | No packaged Plan/auto-route walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits Plan/auto-route reset outputs. | No live Go loop, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 137f. 2026-06-23 MCP Search Refresh Drift Evidence Closure Score Rule

Scoring rule:

```text
MCP currentness evidence improves when catalog refresh drift is mandatory in
closure scans and remains internal to analytix MCP contracts.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP currentness | evidence improves; score unchanged | `mcp_refresh_catalog` records expanded catalog drift with `totalIndexed: 2` and `catalogDrift: true`. | No credentialed MCP matrix. |
| Product sovereignty | evidence improves; score unchanged | Refresh stays internal and no public MCP-indexer route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go G5 readiness | evidence unchanged; scan freshness improves | `mcpSearchRefreshDrift` was already replayed by Go G5 shadow; D-0231 makes it scan-required. | No live Go MCP client, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 138. 2026-06-22 Renderer Named Bridge API Allow-list Score Rule

Scoring rule:

```text
Renderer bridge safety evidence improves when every direct
`window.analytix.runtime.*` and `window.analytix.settings.*` production call is
either an explicit named contract or fails the product-sovereignty scan.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | Product-sovereignty scan allow-lists only named runtime/config/probe/restart/status/model APIs and named settings writes. | No packaged bridge walkthrough. |
| Settings sovereignty | evidence improves; score unchanged | Direct settings reads remain forbidden; only `setSettings` and `saveSettingsSilent` remain direct writes. | No packaged settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Unlisted direct bridge APIs now fail the scan before they can become hidden public protocol. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
node --check scripts/scan-product-sovereignty.cjs
npm run scan:product-sovereignty
```

## 139. 2026-06-23 Renderer Optional Bridge Bypass Seal Score Rule

Scoring rule:

```text
Renderer bridge safety evidence improves when optional-chaining bridge access is
governed by the same direct-access scan rules as dot access.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Bridge safety | evidence improves; score unchanged | Direct generic runtime/settings bridge scans now cover `window.analytix?.runtime?.runtimeRequest` and `window.analytix?.settings?.getSettings`. | No packaged bridge walkthrough. |
| Settings sovereignty | evidence improves; score unchanged | Connect Phone dialog settings loading uses `rendererRuntimeClient.getSettings`. | No packaged Connect Phone settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Optional-chain access cannot bypass the named API allow-list. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
node --check scripts/scan-product-sovereignty.cjs
rg -n "window\\.analytix(?:\\?\\.|\\.)settings(?:\\?\\.|\\.)getSettings|window\\.analytix(?:\\?\\.|\\.)runtime(?:\\?\\.|\\.)runtimeRequest" src/renderer/src --glob '!**/*.test.ts'
npm run scan:product-sovereignty
```

## 140. 2026-06-23 Renderer Bridge Allow-list G5 Shadow Score Rule

Scoring rule:

```text
Go G5 readiness evidence improves when renderer bridge allow-list data is
source-derived in TypeScript and executable-replayed by Go shadow without
changing the live desktop bridge.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `desktopSovereignty.rendererBridgeAllowList` is validated by TS conformance and replayed by Go. | No live Go desktop bridge, rollback, or G6 backend selection. |
| Bridge safety | evidence improves; score unchanged | Go computes named runtime/settings allow-list status, generic bypass absence, and optional-chain scan coverage. | No packaged bridge walkthrough. |
| Product sovereignty | evidence improves; score unchanged | The proof remains tied to `window.analytix`, not Reasonix SessionAPI or Go routes. | Negative fixtures still retain forbidden strings as evidence. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 141. 2026-06-23 Runtime HTTP Auth Matrix G5 Shadow Score Rule

Scoring rule:

```text
Runtime route safety evidence improves when missing-auth behavior is computed
by typed Go G5 control shadow as well as actual TypeScript router dispatch.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `runtimeHttpRouteSovereignty.authMatrix` binds `/health` 200, all 43 protected `/v1/*` route keys, unauthorized status/body, and sensitive route keys to real TS dispatch. | No packaged desktop route walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits auth-matrix coverage booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/public route protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 142. 2026-06-23 Runtime Forbidden Route Dispatch G5 Shadow Score Rule

Scoring rule:

```text
Runtime route sovereignty evidence improves when valid-auth forbidden-route
dispatch is computed by typed Go G5 control shadow as well as actual
TypeScript router dispatch.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` binds 13 forbidden route tokens, structured 404 body, protocol tokens, and hidden-surface tokens to real TS dispatch. | No packaged desktop route walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits forbidden-dispatch coverage booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Runtime contract safety | evidence improves; score unchanged | Forbidden routes are absent even with valid runtime auth, not merely blocked by missing auth. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 143. 2026-06-23 Shared Endpoint Builder G5 Shadow Score Rule

Scoring rule:

```text
Runtime route safety evidence improves when shared endpoint builder encoding
and template ownership are computed by typed Go G5 control shadow as well as
TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` binds 18 shared builder paths, encoded slash/query/fragment ids, and source unit-test proof. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Exported shared endpoints stay `/health` or `/v1/*`, carry no forbidden upstream/hidden tokens, and keep plural `/v1/user-inputs/{id}` canonical. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits shared-endpoint builder coverage booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 144. 2026-06-23 Renderer Runtime Endpoint Builder G5 Shadow Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when renderer provider endpoint
ownership and dynamic route-id encoding are computed by typed Go G5 control
shadow as well as TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.rendererProviderEndpointMatrix` binds shared root paths, encoded dynamic ids, runtime-client facade use, and unit proof. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer runtime paths stay `/health` or analytix-owned `/v1/*`, with forbidden Reasonix/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route families rejected. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits renderer-provider endpoint builder coverage booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 145. 2026-06-23 Main IPC Endpoint Builder G5 Shadow Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when main IPC endpoint-builder
allow-list acceptance and raw-route rejection are computed by typed Go G5
control shadow as well as TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopMainIpcBoundary.endpointBuilderAllowListMatrix` binds encoded shared-builder paths, raw dynamic rejection, singular user-input rejection, shared templates, and unit proof. | No packaged desktop IPC walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Main IPC accepts only analytix-owned encoded shared paths and keeps Reasonix/Go/hidden route families plus singular user-input compatibility out of the accepted path set. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits main IPC endpoint builder coverage booleans. | No live Go main IPC bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 146. 2026-06-23 Main IPC Runtime Adapter Handoff G5 Shadow Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when main IPC runtime-adapter
handoff preservation and reject-before-call behavior are computed by typed Go
G5 control shadow as well as TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` binds encoded adapter calls, method/body preservation, raw/singular rejection, parse-before-adapter ordering, and unit proof. | No packaged desktop IPC walkthrough. |
| Product sovereignty | evidence improves; score unchanged | The main handler rejects invalid paths before adapter calls and forwards only analytix-owned encoded paths unchanged. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits main IPC runtime-adapter handoff booleans. | No live Go main IPC bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 147. 2026-06-23 Preload Runtime Request Bridge G5 Shadow Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when preload runtime/diagnostics
request preservation is computed by typed Go G5 control shadow as well as
TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.preloadRuntimeRequestBridgeMatrix` binds runtime and diagnostics facade calls, encoded paths, method/body preservation, same-channel behavior, and unit proof. | No packaged desktop preload walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Preload exposes only `analytix` and sends runtime requests to analytix-owned `runtime:request`. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits preload runtime request bridge booleans. | No live Go preload bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 155. 2026-06-23 Side Conversation Relation G5 Shadow Score Rule

Scoring rule:

```text
Desktop side-store evidence improves when side conversation promotion is
proved behind the provider relation contract and direct runtime bridge bypass
remains scan-forbidden.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.sideConversationRelationContractMatrix` binds optional provider contract proof, provider promotion, refresh/close behavior, and unit proof. | No packaged side-conversation walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Side-store production source remains scanned against direct `window.analytix.runtime.runtimeRequest` bridge bypass. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits side conversation relation booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/store/chat-store-side-actions.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 154. 2026-06-23 Renderer Provider Facade Seal G5 Shadow Score Rule

Scoring rule:

```text
Desktop provider evidence improves when provider archive/restore and relation
PATCH calls are sealed behind the renderer runtime client facade and direct
runtime bridge bypass remains scan-forbidden.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.rendererProviderFacadeSealMatrix` binds archive/restore, relation PATCH, lifecycle unit proof, and runtime client facade use. | No packaged desktop provider walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer production source remains scanned against direct `window.analytix.runtime.runtimeRequest` bridge bypass. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits renderer provider facade seal booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 153. 2026-06-23 Renderer Provider Alias Guard G5 Shadow Score Rule

Scoring rule:

```text
Desktop provider evidence improves when provider route ownership,
lifecycle/gate coverage, fork/resume encoding, and legacy alias unread guards
are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.rendererProviderAliasGuardMatrix` binds provider route, lifecycle, approval/user-input, fork/resume, dynamic encoding, and forbidden-route guard proof. | No packaged desktop provider walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer provider tests run with throwing `window.kun` and `window.reasonix` getters unread and stay on `/health` or `/v1/*` analytix routes. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits renderer provider alias guard booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 152. 2026-06-23 Renderer Settings Bridge G5 Shadow Score Rule

Scoring rule:

```text
Desktop settings evidence improves when renderer settings cache/write behavior,
top-level runtime patches, and legacy alias unread guards are computed by typed
Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.rendererSettingsBridgeMatrix` binds settings cache expectations, setSettings refresh, and top-level `runtime` patch preservation. | No packaged desktop settings walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer settings client reads `window.analytix.settings`; throwing `window.kun` and `window.reasonix` getters remain unread. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits renderer settings bridge booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/runtime-client.test.ts -- --runInBand
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 151. 2026-06-23 Renderer Runtime Client Bridge G5 Shadow Score Rule

Scoring rule:

```text
Desktop bridge evidence improves when renderer runtime request/restart/SSE
facade behavior and legacy alias unread guards are computed by typed Go G5
control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.rendererRuntimeClientBridgeMatrix` binds encoded request path variants, request argument counts, restart passthrough, SSE start/stop, listener APIs, and unit proof. | No packaged desktop renderer walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Renderer client reads `window.analytix.runtime`; throwing `window.kun` and `window.reasonix` getters remain unread. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits renderer runtime client bridge booleans. | No live Go desktop bridge, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 150. 2026-06-23 Main SSE Host URL Encoding G5 Shadow Score Rule

Scoring rule:

```text
Desktop streaming bridge evidence improves when main SSE host URL encoding,
cursor headers, auth, and forbidden-route guards are computed by typed Go G5
control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopMainIpcBoundary.mainSseHostEncodingMatrix` binds encoded thread events path, `since_seq`, `Last-Event-ID`, `Accept`, auth, stream id, and error payload. | No packaged desktop SSE walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Main SSE host fetch remains on analytix `/v1/threads/{id}/events`, not Reasonix SessionAPI or a Go public route. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits main SSE host encoding booleans. | No live Go SSE server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 149. 2026-06-23 Preload SSE Bridge G5 Shadow Score Rule

Scoring rule:

```text
Desktop streaming bridge evidence improves when preload SSE start/stop and
payload-only listener behavior are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopSovereignty.preloadSseBridgeMatrix` binds start/stop arguments, event/end/error payloads, listener cleanup, and unit proof. | No packaged desktop SSE walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Preload exposes only `analytix` and sends SSE traffic to analytix-owned `runtime:sse:*` IPC. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits preload SSE bridge booleans. | No live Go SSE server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 148. 2026-06-23 Runtime Host URL Handoff G5 Shadow Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when main runtime host handoff is
computed by typed Go G5 control shadow as well as TypeScript source/unit proof.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `desktopMainIpcBoundary.runtimeHostHandoffMatrix` binds encoded path/query, method/body, bearer auth, custom header, JSON content type, and ensureRuntime host selection. | No packaged desktop route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Host handoff targets managed `analytix serve`, not Reasonix SessionAPI or a Go public route. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits runtime host handoff booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 122. 2026-06-22 Runtime Forbidden Route Dispatch Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when forbidden upstream and
hidden-capability route tokens are dispatched with valid auth against the real
TypeScript router and return structured not_found.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `go-runtime-conformance.test.ts` dispatches every D-0189 forbidden route token and proves structured 404. | Negative fixtures still retain forbidden strings as evidence. |
| Runtime contract safety | evidence improves; score unchanged | Forbidden routes are absent from the active router, not just blocked by missing auth. | No live Go HTTP server or packaged route walkthrough. |
| Reasonix/Kun absorption safety | evidence improves; score unchanged | `/v1/reasonix`, `/v1/runtime/go`, `/session-api`, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer tokens remain unregistered. | Full live route parity remains unclaimed. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
```

## 120. 2026-06-22 Runtime HTTP Route Sovereignty Score Rule

Scoring rule:

```text
Runtime contract and product-sovereignty evidence improves when the active
`analytix serve` route table, auth guard coverage, SSE route, internal runtime
routes, and forbidden route-token absence are computed by typed Go G5 control
shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.runtimeHttpRouteSovereignty` proves 44-route inventory, `/health` as the only unauthenticated route, and authenticated `/v1/*` ownership. | No live Go HTTP server or packaged route walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Forbidden Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route tokens remain absent from the active route table. | Negative fixtures still retain forbidden strings as evidence. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits route count, SSE route, thread lifecycle, approval/user-input, task-job, compatibility-route, and boundary booleans. | No G6 backend selection, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 103. 2026-06-22 User-Input Structured Validation Control Shadow Score Rule

Scoring rule:

```text
Approval/user-input evidence improves when structured request_user_input
validation is computed by typed Go G5 control shadow instead of only G4 summary
fields.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Approval/user-input safety | evidence improves; score unchanged | `controlExecutableCases.userInput.structuredChoiceValidation` computes invalid case count and four reject booleans. | No packaged desktop approval/user-input walkthrough. |
| Runtime contract safety | evidence improves; score unchanged | Invalid structured requests return `invalid_user_input_request` and do not open a pending gate. | No live Go gate manager. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed user-input validation output. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix ask/session protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 104. 2026-06-22 Planner Gate Matrix Control Shadow Score Rule

Scoring rule:

```text
Planner/tool-policy evidence improves when Plan mode enable/disable and
blocked-tool rejection are computed by typed Go G5 control shadow instead of a
single forged-tool row.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner safety | evidence improves; score unchanged | `controlExecutableCases.planner` computes normal-mode `create_plan` hiding, Plan capability gate, step narrowing, and blocked-tool rejection matrix. | No packaged desktop Plan walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed planner gate output and rejection matrix. | No live Go planner/executor, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix planner protocol, public auto-plan setting, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 105. 2026-06-22 Desktop Bridge/Settings Sovereignty Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when desktop bridge and settings-schema
guards are computed by typed Go G5 control shadow from real desktop source
proofs.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `controlExecutableCases.desktopSovereignty` computes single `window.analytix`, `Window.analytix`, analytix-owned facade domains, and no deprecated bridge aliases. | No packaged desktop smoke. |
| Settings/runtime contract safety | evidence improves; score unchanged | The control case includes Reasonix `autoPlan`/`auto_plan` drop proof, legacy agent envelope drop proof, and top-level `runtime` endpoint-format persistence proof. | No live settings migration walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed desktop sovereignty output. | No live Go desktop integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/preload/preload-sandbox.test.ts src/main/settings-store.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 111. 2026-06-22 Provider Cache Privacy Control Shadow Score Rule

Scoring rule:

```text
Provider/cache privacy evidence improves when diagnostics privacy and
live-superiority policy are computed by typed Go G5 control shadow instead of
only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerCachePrivacy` computes diagnostics field count, forbidden substring count, no-leak status, no live credentials, and no live superiority claim. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider cache privacy output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 102. 2026-06-22 Sub-Agent Review Matrix Score Rule

Scoring rule:

```text
Evidence quality improves when parallel sub-agent review findings are reduced
to a machine-scannable decision matrix with explicit accepted, rejected, and
deferred claims.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Reasonix absorption governance | evidence improves; score unchanged | `post-881-subagent-review-2026-06-22.md` records lanes A-F and delta classifications. | No live Reasonix parity or packaged walkthrough. |
| Provider/cache reliability | evidence improves; score unchanged | Custom provider claim is corrected to request-shape-only while DeepSeek/OpenAI Responses/Anthropic raw accounting remains. | No credentialed live provider/cache matrix. |
| Go G5 readiness | evidence improves; score unchanged | Review records executable shadow coverage and open G6 gates. | No live Go router/provider/gate/job manager or rollback readiness. |
| Product sovereignty | unchanged | No forbidden top-level entry, Reasonix protocol, Kun identity, Go backend, or Rust/Tauri path is added. | Release readiness remains blocked. |

Evidence:

```text
npm run scan:product-sovereignty
git diff --check
```

## 106. 2026-06-22 Auto-Router Recommendation/Currentness Score Rule

Scoring rule:

```text
Auto-router currentness evidence improves when classifier recommendation
trust and recent-context boundaries are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Agent/kernel reliability | evidence improves; score unchanged | `controlExecutableCases.autoRouterClassifier` now computes accepted/rejected recommendation counts, pro/max acceptance, auto/malformed rejection, active-turn exclusion, and tool-summary preservation. | No user-facing auto-plan setting or packaged desktop Plan/router QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits the expanded auto-router classifier output. | No live Go auto-router, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix controller protocol, public config root, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/auto-model-router.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 107. 2026-06-22 Exact Session Route Replay Score Rule

Scoring rule:

```text
Runtime contract evidence improves when thread/session routes are replayed as
an exact method/path/auth/status/body-hash/SSE-hash matrix by typed Go G5
control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.sessionRouteReplay` computes exact JSON body route count, exact SSE route count, runtime-token coverage, unauthorized ids, and fork/resume/archive/search/SSE hashes. | No packaged desktop route walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits the exact session route replay output. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI route, renderer-visible Go route, Kun identity, deprecated bridge/settings fallback, or hidden top-level route is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 112. 2026-06-22 Provider Cache Inventory Control Shadow Score Rule

Scoring rule:

```text
Provider/cache currentness evidence improves when stable prefix, tools hash,
and provider/request-shape inventory are computed by typed Go G5 control shadow
instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerCacheInventory` computes stable prefix hash, tools hash, usage/request-shape ids and counts, prefix equivalence, and tools hash stability. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider cache inventory output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 113. 2026-06-22 Session Route Inventory Control Shadow Score Rule

Scoring rule:

```text
Runtime contract safety evidence improves when thread/session route inventory
is computed by typed Go G5 control shadow instead of only implied by route
status replay.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.sessionRouteInventory` computes route ids, JSON/SSE/event/resume/fork/archive/search/read-update groups, runtime-token count, and unauthorized route ids. | No packaged desktop thread/fork/resume/archive/search walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed session route inventory output. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/public route protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 114. 2026-06-22 Approval/User-Input Inventory Control Shadow Score Rule

Scoring rule:

```text
Approval/user-input reliability evidence improves when gate inventory and
answer privacy flags are computed by typed Go G5 control shadow instead of
only carried inside route replay.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Approval/user-input reliability | evidence improves; score unchanged | `controlExecutableCases.approvalUserInputInventory` computes gate ids, approval/user-input id groups, route kinds, replay kind order, answer privacy flags, late statuses, and pending-after sum. | No packaged desktop approval-card/user-input walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed approval/user-input inventory output. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix ask/session protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 115. 2026-06-22 Task Planner Toolset Inventory Control Shadow Score Rule

Scoring rule:

```text
Planner/sub-agent safety evidence improves when read-only and forbidden task
toolset inventory is computed by typed Go G5 control shadow instead of only
carried in job replay summary.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner/sub-agent reliability | evidence improves; score unchanged | `controlExecutableCases.taskJobs.plannerToolsetInventory` computes read-only tools, forbidden task tools, counts, no-overlap, task-tool match, and policy values. | No packaged desktop sub-agent/planner walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed planner toolset inventory output. | No live Go planner/executor, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix public sub-agent/job/planner protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 116. 2026-06-22 MCP Core Lifecycle Boundary Seal Control Shadow Score Rule

Scoring rule:

```text
MCP lifecycle sovereignty evidence improves when provider identity and
product-boundary flags are computed by typed Go G5 control shadow instead of
being implied by surrounding fixtures.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle reliability | evidence improves; score unchanged | `controlExecutableCases.mcpCoreLifecycle` computes `providerId: "mcp:research"` alongside connect/disconnect/reload/cancel/error output. | No packaged desktop MCP walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP lifecycle provider/boundary output. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer public protocol, renderer-visible Go route, Kun identity, or top-level MCP-indexer entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 117. 2026-06-22 Provider Request-Shape Exact Matrix Control Shadow Score Rule

Scoring rule:

```text
Provider/cache request-shape evidence improves when exact URL/header/body/tool
matrix is computed by typed Go G3/G5 shadow instead of only summarized by
counts and endpoint families.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider request-shape reliability | evidence improves; score unchanged | G3/G5 shadow outputs now replay all 7 exact request-shape cases with URLs, headers, body fields, reasoning presence, and tool-shape family. | No credentialed provider matrix. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ShadowSlicesOutput` and `BuildG5ControlExecutableOutput` now carry exact matrix output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, deprecated settings fallback, Kun identity, or default Go backend is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts tests/go-runtime-g3-g4-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 118. 2026-06-22 Combined Step/Cancel/Cache Trace Control Shadow Score Rule

Scoring rule:

```text
Agent-loop control evidence improves when combined route-cache, step-limit, and
cancel behavior is computed as an exact trace instead of summary booleans.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Agent-loop reliability | evidence improves; score unchanged | G5 combined control now records router calls, model-step limits, stable-prefix flags, cancel result counts, and exact cancel rows. | No packaged long-running cancel/cache walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` computes combined trace from TS-owned fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix controller/session protocol, public auto-plan surface, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 119. 2026-06-22 Step-Limit Override/Delegate Matrix Control Shadow Score Rule

Scoring rule:

```text
Agent-loop control evidence improves when step-limit override and delegate
inheritance behavior is computed as a matrix instead of scalar fields.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Planner/sub-agent reliability | evidence improves; score unchanged | G5 step-limit control now records 9 override/delegate rows including planner/headless limits, zero-default disable guard, delegate half, and delegate min-floor. | No packaged step-limit walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` computes step-limit matrix from TS-owned fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix controller/session protocol, public auto-plan surface, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 120. 2026-06-22 History Repair Pair-Integrity Control Shadow Score Rule

Scoring rule:

```text
Runtime legality evidence improves when model-history repair and
tool-call/result pairing are computed by typed Go G5 control shadow from the
same TS-owned repair oracle.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | G5 history repair control now proves complete tool-call/result blocks survive and orphan/missing/duplicate tool items are removed. | No packaged long-history walkthrough. |
| Provider/cache reliability | evidence improves; score unchanged | Repair bookkeeping is explicitly kept out of stable prefix/cache state. | No credentialed long-session cache matrix. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed history repair output from fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/controller protocol, public auto-plan surface, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 88. 2026-06-22 Runtime Settings Legacy-Agent Guard Score Rule

Scoring rule:

```text
Settings sovereignty evidence improves when active `runtime` saves strip or
reject upstream agent-shaped fields.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Settings sovereignty | evidence improves; score unchanged | Shared normalization strips runtime-contained legacy agent shapes; IPC runtime patches reject them. | No packaged settings walkthrough. |
| Reasonix/Kun absorption safety | evidence improves; score unchanged | Reasonix config value stays document/defer unless reimplemented as analytix-owned fields. | No user-visible auto-plan settings. |
| Product sovereignty | unchanged | No bridge alias, deprecated settings fallback, or upstream protocol is added. | Release readiness still blocked by packaged QA/signing. |

Evidence:

```text
npm run test -- src/shared/app-settings.test.ts -t "Reasonix auto-plan" --no-file-parallelism --maxWorkers=1
npm run test -- src/main/ipc/app-ipc-schemas.test.ts -t "legacy.*settings|agent-shaped" --no-file-parallelism --maxWorkers=1
```

## 89. 2026-06-22 Task-Job Route Executable Control Shadow Score Rule

Scoring rule:

```text
Go G5 readiness evidence improves when route executable behavior is computed by
typed control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` computes task-job route executable output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | evidence improves; score unchanged | Unauthorized/output/wait/kill/missing/rehydrated route statuses are tied to TS-owned fixtures. | No packaged desktop sub-agent/task-job QA. |
| Product sovereignty | unchanged | No Reasonix protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 106. 2026-06-22 MCP Core Lifecycle Control Shadow Score Rule

Scoring rule:

```text
MCP lifecycle evidence improves when connect/disconnect/reload/cancel/error
behavior is computed by typed Go G5 control shadow instead of only summarized
in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpCoreLifecycle` computes connect/disconnect diagnostics, reload schema order, cancel no-execute, and approved error shape. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP core lifecycle output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 109. 2026-06-22 Task-Job Tool Contract Boundary Score Rule

Scoring rule:

```text
Sub-agent/job orchestration evidence improves when internal tool contracts and
forbidden route boundaries are computed by typed Go G5 control shadow instead
of only asserted as summary replay.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Task/job orchestration | evidence improves; score unchanged | `controlExecutableCases.taskJobs.toolContractBoundary` computes task/parallel internal-only flags, permission/evidence/dependency/read-only gates, route auth, forbidden top-level routes, no Reasonix protocol, and no top-level route exposure. | No live Go Job Manager or packaged nested-card QA. |
| Product sovereignty | evidence improves; score unchanged | Tool contracts remain internal runtime capability, not top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation. | Release readiness remains blocked. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed task-job tool-boundary output. | No Electron integration, rollback, G6 readiness, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 111. 2026-06-22 Nested Child SSE Metadata Score Rule

Scoring rule:

```text
Sub-agent/job orchestration evidence improves when nested child SSE metadata is
computed by typed Go G5 control shadow instead of only asserted as summary
replay.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Task/job orchestration | evidence improves; score unchanged | `controlExecutableCases.taskJobs.nestedSseMetadata` computes parent/child ids, metadata field coverage, evidence ledger keys, parent/child distinctness, active-goal requirement, no Reasonix protocol, and no top-level route exposure. | No live Go Job Manager or packaged nested-card QA. |
| Timeline/projection safety | evidence improves; score unchanged | Child run attribution remains tied to analytix-owned task-job oracle and evidence ledger metadata. | Runtime live parity remains TypeScript-owned. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed nested child SSE metadata output. | No Electron integration, rollback, G6 readiness, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 110. 2026-06-22 Task Transcript Identity Score Rule

Scoring rule:

```text
Sub-agent/job orchestration evidence improves when transcript continue/fork
identity semantics are computed by typed Go G5 control shadow instead of only
asserted as summary replay.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Task/job orchestration | evidence improves; score unchanged | `controlExecutableCases.taskJobs.transcriptIdentity` computes source/continue/fork ids, incompatible identity error, continue-target-matches-source, fork-target-distinct-from-source, no Reasonix protocol, and no top-level route exposure. | No live Go Job Manager or packaged nested-card QA. |
| Thread/fork safety | evidence improves; score unchanged | Continue/fork identity remains tied to analytix-owned task-job oracle rather than Reasonix SessionAPI. | Runtime live parity remains TypeScript-owned. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed transcript identity output. | No Electron integration, rollback, G6 readiness, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 108. 2026-06-22 Product Boundary Control Shadow Score Rule

Scoring rule:

```text
Product sovereignty evidence improves when bridge/serve/backend/protocol
boundaries are computed by typed Go G5 control shadow instead of only asserted
as manifest metadata.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `controlExecutableCases.productBoundary` computes bridge/serve stability, no Reasonix protocol, no default Go backend, no renderer-visible Go route, no Electron main connection, and no enabled Go backend. | Packaged desktop QA and release readiness remain blocked. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed product-boundary output before other control cases. | No Electron integration, rollback, G6 readiness, or default backend. |
| Upstream absorption safety | evidence improves; score unchanged | Kun/Reasonix capability absorption stays behind analytix contracts and forbidden top-level surfaces. | Full live parity remains unclaimed. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 107. 2026-06-22 MCP Background Reconnect Control Shadow Score Rule

Scoring rule:

```text
MCP lifecycle evidence improves when background reconnect/retry outcomes are
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpBackgroundReconnect` computes failed server ids, suspended provider/reason, connected/error outcomes, retry attempts, retry-all-failed coverage, and no-runtime-restart state. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP background reconnect output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 108. 2026-06-22 MCP Known Override Diagnostics Control Shadow Score Rule

Scoring rule:

```text
MCP lifecycle evidence improves when known override diagnostics are computed by
typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpKnownOverrideDiagnostics` computes known override rows, override kinds, workspace roots, explicit-cwd server ids, daemon-timeout server ids, and low-priority/background-start booleans. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP known override diagnostics output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 109. 2026-06-22 MCP Live-Local Indexer Control Shadow Score Rule

Scoring rule:

```text
MCP/indexer lifecycle evidence improves when live-local indexer behavior is
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpLiveLocalIndexer` computes retry ids/map, initial/resume/active paths, tombstone count, restart status, diagnostic redaction, and execution-error redaction. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP live-local indexer output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 110. 2026-06-22 MCP Search Meta-Tool Control Shadow Score Rule

Scoring rule:

```text
MCP search/indexer trust evidence improves when search meta-tool behavior is
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpSearchMetaTools` computes meta-tool set, refresh advertisement, trusted/untrusted workspace fields, unknown-tool error, `on-request` policy, and denied no-execute. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP search meta-tools output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 105. 2026-06-22 MCP Search Workspace Boundary Control Shadow Score Rule

Scoring rule:

```text
MCP workspace trust evidence improves when trusted/untrusted search boundaries
and denied no-execute behavior are computed by typed Go G5 control shadow
instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpSearchWorkspaceBoundary` computes trusted/untrusted workspaces, query, trusted tool id, untrusted search count, unknown-tool error, call policy, and denied no-execute. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP search workspace boundary output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 104. 2026-06-22 MCP Approval Annotation Control Shadow Score Rule

Scoring rule:

```text
MCP tool approval evidence improves when destructive/open-world annotations and
denied no-execute behavior are computed by typed Go G5 control shadow instead
of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP approval safety | evidence improves; score unchanged | `controlExecutableCases.mcpApprovalAnnotations` computes destructive/open-world hints, normalized tool name, approval id, deny decision, result kind, execution flag, and denied no-execute. | No packaged MCP/approval QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP approval annotation output. | No live Go MCP client, live Go approval manager, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer/approval protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 103. 2026-06-22 MCP Search Refresh Drift Control Shadow Score Rule

Scoring rule:

```text
MCP catalog lifecycle evidence improves when search refresh drift is computed
by typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| MCP lifecycle safety | evidence improves; score unchanged | `controlExecutableCases.mcpSearchRefreshDrift` computes initial tools, expanded tools, indexed count, catalog drift, and no top-level route. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed MCP search refresh drift output. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix MCP-indexer protocol, top-level MCP-indexer route, renderer-visible Go route, Kun identity, or hidden product entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 102. 2026-06-22 Approval/User-Input Route Replay Control Shadow Score Rule

Scoring rule:

```text
Approval/user-input route safety evidence improves when route replay is
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Approval/user-input safety | evidence improves; score unchanged | `controlExecutableCases.approvalUserInputRouteReplay` computes approval deny, submitted/cancelled user-input routes, replay kinds, late rejections, and abort cleanup. | No packaged desktop approval-card QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed approval/user-input route replay output. | No live Go approval/user-input manager, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/ask protocol, renderer-visible Go route, Kun identity, or hidden top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 90. 2026-06-22 Task-Job Lifecycle Control Shadow Score Rule

Scoring rule:

```text
Go G5 readiness evidence improves when task-job lifecycle behavior is computed
by typed control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` computes task-job lifecycle output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | evidence improves; score unchanged | Foreground/background/wait-output-kill lifecycle state is tied to TS-owned fixtures. | No packaged desktop sub-agent/task-job QA. |
| Product sovereignty | unchanged | No Reasonix protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 91. 2026-06-22 Planner-Executor Control Shadow Score Rule

Scoring rule:

```text
Go G5 readiness evidence improves when planner/executor propagation behavior is
computed by typed control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` computes planner/executor propagation output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | evidence improves; score unchanged | Failure, cancellation, output-offset, and transcript propagation are tied to TS-owned fixtures. | No packaged desktop sub-agent/task-job QA. |
| Product sovereignty | unchanged | No Reasonix protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 92. 2026-06-22 Provider Cache Release Guard Control Shadow Score Rule

Scoring rule:

```text
Provider/cache evidence improves when the release guard is computed by typed
Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerCacheReleaseGuard` computes fixture-only tail averages, allowed-low cases, collapse counts, and status. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed release-guard output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 93. 2026-06-22 Provider Usage Parser Control Shadow Score Rule

Scoring rule:

```text
Provider/cache accounting evidence improves when usage parser precedence is
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerUsageParser` computes DeepSeek native precedence, Responses cached tokens, Anthropic cache fields, and unsupported fallback. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed usage parser output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 94. 2026-06-22 Provider Request Shape Control Shadow Score Rule

Scoring rule:

```text
Provider request-surface evidence improves when URL/body/tool-shape replay is
computed by typed Go G5 control shadow instead of only summarized in shadow
slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider request safety | evidence improves; score unchanged | `controlExecutableCases.providerRequestShape` computes exact URL count, endpoint families, full endpoint ids, tool shapes, and body-field family counts. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed request-shape output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 95. 2026-06-22 Provider Cache Accounting Control Shadow Score Rule

Scoring rule:

```text
Provider/cache reliability evidence improves when cache accounting is computed
by typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerCacheAccounting` computes supported telemetry ids, unsupported ids, provider-family ids, hit/miss totals, and aggregate hit rate. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed cache accounting output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/provider-cache-proof.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 96. 2026-06-22 Release/Package Identity Sovereignty Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when release/package identity is covered
by scans and packaging configuration assertions.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `scan:product-sovereignty` covers package manifests, builder config, scripts, workflows, and build config; `packaging-config.test.ts` asserts analytix identity fields. | No packaged Electron smoke or signed release QA. |
| Kun/Reasonix absorption safety | evidence improves; score unchanged | Kun/Reasonix identity is rejected from package/release surfaces. | Historical compatibility code and dormant quarantined source remain documented. |
| Release readiness | unchanged | Source/config guard only. | Signing, notarization, update-channel, and packaged artifact QA remain separate. |

Evidence:

```text
npm run scan:product-sovereignty
npm run test -- src/main/packaging-config.test.ts --no-file-parallelism --maxWorkers=1
```

## 97. 2026-06-22 Provider Streaming Control Shadow Score Rule

Scoring rule:

```text
Provider streaming evidence improves when SSE usage replay is computed by
typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerStreaming` computes event order, usage match, prompt/completion/reasoning/cache tokens, hit rate, and telemetry support. | No credentialed live streaming provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider streaming output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-g3-g4-conformance.test.ts tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 98. 2026-06-22 Provider Offline Parity Seal Control Shadow Score Rule

Scoring rule:

```text
Provider/cache evidence improves when offline parity seal is computed by typed
Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerOfflineParitySeal` computes stable prefix equivalence, DeepSeek cache telemetry, request-shape endpoint formats, release guard status, and fixture-only policy. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider offline parity seal output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 99. 2026-06-22 Session Route Status Control Shadow Score Rule

Scoring rule:

```text
Thread/session route evidence improves when route status is computed by typed
Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.sessionRouteStatus` computes archive/search, read/update, fork, resume, SSE replay/caught-up, and auth status. | No packaged desktop route walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed session route status output. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI/job protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 100. 2026-06-22 Provider Live-Local HTTP Control Shadow Score Rule

Scoring rule:

```text
Provider request/usage evidence improves when live-local HTTP proof is computed
by typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerLiveLocalHttpProof` computes fixture policy, local HTTP transport, usage/request-shape counts, endpoint formats, provider families, and no-live-superiority status. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider live-local HTTP proof output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 101. 2026-06-22 Provider Drift Attribution Control Shadow Score Rule

Scoring rule:

```text
Provider cache currentness evidence improves when drift attribution is computed
by typed Go G5 control shadow instead of only summarized in shadow slices.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Provider/cache reliability | evidence improves; score unchanged | `controlExecutableCases.providerDriftAttribution` computes prefix hashes, stable system/prefix-items hashes, tools/provider/model/endpoint drift, expected reasons, and telemetry status. | No credentialed live cache matrix or packaged provider QA. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` now emits typed provider drift attribution output. | No live Go provider client, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix provider protocol, renderer-visible Go route, Kun identity, or top-level hidden-capability entry is added. | Release readiness remains blocked. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 87. 2026-06-22 Thread/SSE Route Auth Score Rule

Scoring rule:

```text
Thread/session route quality evidence improves when successful replay and
missing-token rejection are proven by the same shared oracle path.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | G2 route replay now includes an unauthorized `/v1/threads/:id/events` case with 401. | No packaged desktop restart/resume walkthrough. |
| Go G5 readiness | evidence improves; score unchanged | G5 `sessionReplay.routeStatusReplay.auth` replays the auth boundary from G2 fixtures. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Product sovereignty | unchanged | No Reasonix SessionAPI or renderer-visible Go route is added. | Release readiness still blocked by packaged QA/signing. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test ./...
```

## 116. 2026-06-22 Desktop Bridge IPC Sovereignty Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when preload runtime request and
SSE IPC ownership are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.desktopSovereignty` now computes runtime request and SSE IPC ownership from preload/shared settings sources. | No packaged desktop bridge walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Forbidden Reasonix/Kun/Go/workflow IPC channels are recorded as absent in the G5 case and product scan. | No live G6/default backend decision. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits `runtimeRequestUsesAnalytixIpc`, `runtimeSseUsesAnalytixIpc`, and `publicApiTypesAnalytixOwned`. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 117. 2026-06-22 Desktop Main IPC Boundary Score Rule

Scoring rule:

```text
Desktop runtime contract evidence improves when main IPC request allow-list and
SSE cursor semantics are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Runtime contract safety | evidence improves; score unchanged | `controlExecutableCases.desktopMainIpcBoundary` proves main runtime request and SSE IPC schema/handler boundaries. | No packaged desktop bridge walkthrough. |
| Product sovereignty | evidence improves; score unchanged | Forbidden Reasonix/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes remain rejected before runtime calls. | No live G6/default backend decision. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits request allow-list, forbidden rejection, SSE route/cursor/stop/batch, and boundary booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 118. 2026-06-22 Renderer Route-Surface Sovereignty Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when renderer top-level route entries,
dormant workflow quarantine, browser preview bridge ownership, and shell
safe-area behavior are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `controlExecutableCases.rendererRouteSurfaceSovereignty` proves AppRoute stays `chat/write/settings/plugins/claw/schedule` and forbidden hidden-capability entries remain absent. | No packaged desktop route walkthrough or release readiness. |
| Runtime contract safety | evidence improves; score unchanged | Browser preview remains `window.analytix` only, while dormant Workflow/Create Loop code stays quarantined from top-level entry surfaces. | No Reasonix UI/session protocol parity claim. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits route-union, quarantine, bridge, plugin safe-area, no-drag, safe-inset, and boundary booleans. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```

## 119. 2026-06-22 Package / Runtime CLI Identity Score Rule

Scoring rule:

```text
Product-sovereignty evidence improves when package/release identity and the
public runtime CLI contract are computed by typed Go G5 control shadow.
```

Updated score impact:

| Dimension | Impact | Evidence | Remaining cap |
| --- | --- | --- | --- |
| Product sovereignty | evidence improves; score unchanged | `controlExecutableCases.packageRuntimeIdentity` proves package, builder, CLI, AppUserModelID, afterPack, and release env identity stay analytix-owned. | No packaged artifact walkthrough or release readiness. |
| Runtime contract safety | evidence improves; score unchanged | Runtime bin and usage remain `analytix serve`, with `ANALYTIX_READY` startup handshake. | No Reasonix CLI/session protocol parity claim. |
| Go G5 readiness | evidence improves; score unchanged | `BuildG5ControlExecutableOutput` emits package/runtime identity booleans while default Go backend remains disabled. | No live Go launcher, Electron integration, rollback, or default backend. |

Evidence:

```text
npm --prefix packages/runtime test -- tests/go-runtime-conformance.test.ts --no-file-parallelism --maxWorkers=1
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
```
