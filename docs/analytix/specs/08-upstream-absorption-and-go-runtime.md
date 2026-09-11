# analytix upstream absorption and Go runtime evolution spec

Status: Normative upstream/cutover governance with a chronological stage
ledger.
Current as of: 2026-08-26 for the current-state and authority notes only.
Source of truth for as-built runtime behavior: current Go/desktop code, focused
tests, and fresh release-gate evidence.

## 1. Purpose

This spec defines the long-term process for reviewing and, where admitted,
absorbing future Kun, Reasonix, OpenCode, CodexDesktop-Rebuild, Claude Code,
Claw Code, Gajae Code, Hermes Agent, and LazyCodex upgrades into analytix
without letting any upstream control the analytix product architecture.

The goal is to make analytix a stronger, independent product:

- stronger than Kun as a desktop product, because analytix keeps the
  requirement-first GUI workflow while adding a cleaner runtime boundary,
  smoother thread experience, and stronger release identity;
- stronger than Reasonix as a product, because analytix can absorb Reasonix
  engine ideas while providing a full desktop workbench, Write, SDD, Connect
  Phone, schedule, plugin, settings, and QA surface;
- stronger than a direct merge of the upstreams, because every upstream change
  must pass through analytix contracts, specs, tests, benchmarks, and product
  judgment.

License, authorization, copyright provenance, and required notices are part of
the engineering admission gate. No code, test, prompt, document, binary, or
asset may be copied or substantially adapted until its pinned material,
license scope, file/vendor exceptions, reuse mode, destination, and notice
obligations are recorded. Missing or restrictive evidence limits the source to
clean-room behavior study or rejection unless a material-specific written
authorization is recorded.

Current-state note (2026-06-25): historical proof-inventory sections in this
spec may still say Go is shadow-only, TypeScript is authoritative, or default
backend is pending for the stage being described. Those statements document
their original stage and are superseded for current release readiness by
Section 3.0 and the current code path. Dated delivery reports are supporting
evidence, not current authority. The current code default is `go-runtime-default`;
`ANALYTIX_RUNTIME_BACKEND=typescript` is now rejected by the desktop adapter as
a retired backend. Older references to TypeScript as explicit rollback are
historical stage records unless a newer current-state note repeats them.

Current authority note (2026-08-26):
[`11-agent-platform-brand-and-architecture.md`](11-agent-platform-brand-and-architecture.md)
and the accepted `agent-platform-foundation` scoped requirements govern current
brand, one-Harness, Plugin and Privacy Layer terminology. `重构升级方案.md`,
the stage plans and the chronological sections below remain historical decision
and gate ledgers. They do not authorize a second runtime, an everything-is-a-
plugin security kernel, a default Codex Goal, or the old TypeScript-oracle/Go-
shadow construction order. Current construction routing starts from applicable
`AGENTS.md`, current Git/code, accepted specs, the active authorized OpenSpec
change and fresh evidence.

## 2. Inputs

Authority and historical entry documents (repo-relative paths):

```text
AGENTS.md
docs/analytix/README.md
重构升级方案.md
docs/analytix/specs/01-identity-runtime-schema-reset.md
docs/analytix/specs/02-release-packaging-channels.md
docs/analytix/specs/03-brand-assets-visual-system.md
docs/analytix/specs/04-analytix-derived-thread-virtualizer.md
docs/analytix/specs/05-architecture-code-upgrade-inventory.md
docs/analytix/specs/06-implementation-closure-and-acceptance.md
docs/analytix/specs/07-desktop-qa-release-readiness.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
docs/analytix/specs/10-subagent-todo-goal-control-plane.md
docs/analytix/specs/11-agent-platform-brand-and-architecture.md
```

Long-term upstream ledger files:

```text
/Users/sun/Projects/analytix/docs/analytix/upstreams/README.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/kun-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/reasonix-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/opencode-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/codexdesktop-rebuild-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/hermes-agent-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/claude-code-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/claw-code-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/gajae-code-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/lazycodex-sync.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/upstream-sources.json
/Users/sun/Projects/analytix/docs/analytix/upstreams/upstream-capability-audit-2026-07-10.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/code-reuse-provenance.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/absorption-targets.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/continuous-absorption-strategy.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/code-level-absorption-blueprint.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/code-level-implementation-plan.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/stage-closure-playbook.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/conflict-decisions.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/absorption-ledger.md
/Users/sun/Projects/analytix/docs/analytix/upstreams/go-runtime-conformance.md
```

## 3. Frozen Decisions

| Area | Decision |
| --- | --- |
| Product owner | analytix is the mainline product and architecture owner. |
| Local mainline | The private local analytix mainline branch is `main`; upstream research projects must not be configured as analytix Git remotes. |
| Kun role | Product workflow upstream for Code, Write, SDD, Connect Phone, schedule, GUI behaviors, and bug fixes. |
| Reasonix role | Agent/runtime upstream for Go implementation ideas, DeepSeek/cache behavior, CLI ergonomics, tool loop, MCP/plugin, context handling, and engine performance. |
| OpenCode role | Auxiliary comparison source for provider registry, sub-agent, task/TODO, session continuation, permissions, and background execution designs. |
| CodexDesktop-Rebuild role | Desktop feel and architecture reference for thread virtualizer, app-server boundary, workers, signals, visual rhythm, and QA evidence. |
| Hermes Agent role | Self-improving-agent reference for memory/search, skill evolution, gateway delivery, scheduling, terminal backends, tool RPC, sub-agent parallelism, and trajectory compression. |
| Claude Code role | Proprietary behavior baseline for durable attach/resume, remote control, managed settings, plugin lifecycle, and layered review; clean-room only absent written authorization. |
| Claw Code role | Low-confidence/museum reference for small verified lifecycle and Rust fixtures; in-memory registries and parity prose are not production maturity evidence. |
| Gajae Code role | External-control, session-tree/handoff, plugin-quarantine, receipt, and native benchmark reference; direct reuse is blocked until inherited copyright provenance is resolved. |
| LazyCodex role | Committed plugin reference for LSP/CodeGraph and evidence guards; its uninitialized OmO gitlink and file-level exceptions are outside the admitted source set. |
| Reuse admission | `upstream-sources.json` and `code-reuse-provenance.md` must admit the pinned material and notice destination before direct or substantial reuse. |
| Final authority | analytix specs, contracts, tests, and current UI decide how upstream changes land. |
| UI direction | analytix UI remains the product baseline. Upstream UI is not copied over the upgraded analytix UI. |
| Runtime direction | Runtime behavior is contract-first; the only current production implementation is Go in `packages/runtime-go`. Historical shadow/candidate gates remain stage evidence. |
| Public CLI | Public runtime entry remains `analytix serve`; no upstream CLI identity is exposed as the current product CLI. |
| Renderer dependency | Renderer depends only on the analytix HTTP/SSE/bridge contracts and must not expose a runtime-implementation switcher. |
| Settings | Active settings remain top-level `runtime`; no active `agents.kun` or `agents.analytix`. |
| Bridge | Renderer uses `window.analytix`; no deprecated bridge alias or fallback. |

### 3.0 Current Cutover Status Addendum (2026-06-25)

The current implementation has completed the deterministic Go runtime default
cutover. `src/main/runtime/analytix-adapter.ts` resolves an unset
`ANALYTIX_RUNTIME_BACKEND`, plus `analytix`, `go`, `go-runtime`, and
`go-runtime-default`, to `go-runtime-default` and starts
`packages/runtime-go/cmd/runtime-server`. TypeScript is no longer a default
dual track, and `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected as a retired
backend by the current desktop adapter.

Development builds may start the Go runtime through `go run -tags
analytix_prod ./cmd/runtime-server`; packaged apps must not depend on a Go
toolchain or bundled Go source. `scripts/after-pack.cjs` builds a
production-tagged native `runtime-server` binary into
`Resources/runtime-go/bin/`, and the Electron main adapter starts that bundled
binary in packaged mode.

D-0248 strict deterministic gate, D-0250C code-stage Reasonix absorption, the
Go deterministic core, and Reasonix code-stage absorption are complete.
D-0251/D-0252/D-0253 provider, MCP, packaged, operator, soak, and fallback
retirement evidence are post-cutover live validation. Missing external
credentials or packaged/operator inputs do not block the Go runtime default
path, but they still block claims about live provider/MCP parity and candidate
soak completion.

Current status is intentionally split by evidence class:

| Facet | Current state | Notes |
| --- | --- | --- |
| `fixtureProof` | closed for historical D-024x/D-0250C deterministic slices | Fixture and fake-local proof remains useful only as regression evidence. It does not count as product parity. |
| `codeAbsorbed` | partial, ongoing | Go provider request shapes, usage/cost parsing, attachment payload mapping, and selected Reasonix engine ideas are present. Provider usage now propagates `priceConfigured` through Go usage events and `/v1/usage`, so renderer usage chips show missing provider pricing as not configured while preserving explicit zero-cost configured pricing. MCP/tool execution is still limited by configured-server availability. |
| `productParity` | partial | Normal chat now excludes synthetic `Ship` / `Choose direction` waits and D0244/MCP lifecycle fixture events. `/v1/runtime/tools` has an `analytix_prod` HTTP gate proving empty production MCP config stays unavailable/empty and does not expose `mcpLocalProof`, `contractProof`, `upstreamAbsorption`, fake, or fixture markers. `/v1/runtime/info` capabilities are also covered after MCP refresh so configured production MCP availability, server counts, and tool counts stay aligned with the tools surface. Full live tool/MCP and packaged desktop parity still require validation. |
| `liveProviderParity` | partial | OpenAI-compatible, Anthropic messages, custom full endpoint request shapes, stream parsing, usage parsing, DeepSeek-scoped cache telemetry, and error handling are covered by the current `scripts/runtime-go-live-validation.mjs` / `scripts/runtime-go-live-evidence-collector.mjs` gates. Real external-key provider matrix and operator validation remain separate release evidence. |
| `packagedSoak` | partial | Packaged native `runtime-go/bin/runtime-server` startup, D-0251 runtime health/thread/turn/SSE replay, local credentialed MCP HTTP list/call/reconnect/redaction evidence, multi-turn packaged runtime durable-restart/history/usage soak, and packaged Electron `window.analytix` GUI bridge restart/health/runtime-info smoke are covered by local physical-package evidence. The rebuilt macOS arm64 unpacked app passed `npm run runtime:go:packaged-gui-smoke -- --json --gate`, and `scripts/runtime-go-live-evidence-collector.mjs` records GUI smoke as `packagedGui` and packaged multi-turn durable/history/usage/redaction soak as `packagedSoak` component evidence paths. Stale packaged artifacts may still block at older readiness gates. Operator release review remains separate release evidence. |
| `rollbackRetired` | true for production adapter selection | `ANALYTIX_RUNTIME_BACKEND=typescript` is rejected by the current desktop adapter as a retired backend. Historical rollback-retirement scripts and evidence remain audit material, but they are not current operator instructions for starting the production desktop runtime. |

The Go runtime default must keep the Kun/Analytix product baseline:
`window.analytix`, top-level `runtime` settings, the desktop route surface,
multi-provider contracts, attachments, multimodal payload paths, approvals,
user input, MCP, goal/plan state, thread/session/SSE replay, and startup
contracts stay Analytix-owned. Reasonix remains an engine/runtime source only;
no Reasonix SessionAPI/config roots, renderer-visible Go switcher, or top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entry may be
introduced.

The packaged default startup canary is product-clean: it verifies health,
runtime info, thread CRUD/resume/fork cleanup, and hidden Go/proof routes
without creating a model turn or requiring external provider credentials.

Sections 3.1 through 3.8 below are retained as historical gate evolution. Any
statement in those sections that TypeScript is the current production default,
or that D-0251/D-0252/D-0253 live evidence blocks the local deterministic Go
default, is superseded by this addendum and the final Go runtime delivery
report.

### 3.1 D-0238 Internal Backend Gate Addendum

D-0238 introduces a backend-neutral main adapter gate for Go conformance
startup. The default backend remains the TypeScript `analytix serve` runtime.
The Go path is valid only when both env gates are present:

```text
ANALYTIX_RUNTIME_BACKEND=go-conformance
ANALYTIX_GO_RUNTIME_CONFORMANCE=1
```

The gate is internal/test/conformance only. It must not be surfaced as a
settings schema field, renderer route, preload bridge method, runtime-control
panel, or top-level navigation entry. Main must accept Go only after a canary
proves:

```text
/health is analytix serve health
/v1/threads?limit=1 matches the analytix thread API shape
/v1/conformance/loop/boundary is fixture-only and reports no default Go backend
/v1/runtime/go remains hidden
```

Launch or canary failure must stop the Go sidecar, clean temp durable state,
record a fallback reason, and in that historical stage returned to the then
default TypeScript runtime. This is not current startup guidance: after the
2026-06-25 cutover, Go default startup failures fail closed and
`ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic that does
not start the TypeScript runtime.

### 3.2 D-0239 Kernel Scaffold Addendum

D-0239 adds a Go live-local kernel scaffold below:

```text
/v1/conformance/kernel/*
```

The scaffold may combine durable event sink, minimal loop controller,
provider-aware cache accounting, approval/user-input gate replay, MCP catalog
recovery, and job/sub-agent orchestration fixture evidence. It must remain
fixture-backed and internal until production Go managers pass G5/G6 gates.

The scaffold must not:

```text
execute live jobs or subagents
expose Reasonix SessionAPI, config roots, or route names
add Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entries
add renderer-visible Go runtime routes
claim live provider/MCP/job parity or default backend readiness
```

Production Go job/sub-agent execution requires permission-gated runtime
routes, durable crash recovery, packaged desktop QA, renderer projection
evidence, rollback evidence, and a G6 retirement plan for any superseded
TypeScript-only paths.

### 3.3 D-0240 G6 Retirement Cleanup Addendum

D-0240 adds an executable cleanup proof below:

```text
/v1/conformance/kernel/g6-retirement-cleanup
```

The route must report:

```text
g6Ready:false
noImmediateProductionDelete:true
tsRuntimeRetained:true
presentForbiddenRedundancyCount:0
```

It is allowed to list current retained TypeScript/default-runtime paths,
post-G6 delete targets, forbidden redundancy categories, and open G6 blockers.
It is not allowed to delete the production TypeScript runtime, create a
renderer-visible backend switch, add duplicate settings schemas, expose
Reasonix SessionAPI/config roots, or claim default Go backend readiness.

The G6 cleanup proof exists to prevent permanent dual paths after Go becomes
default. At D-0240 time, G6 had not passed, so the TypeScript runtime remained
the production default and the Go cleanup route remained internal/test/
conformance evidence only. That historical rule is superseded for current code
by the 2026-06-25 addendum above: Go is now the default deterministic runtime
core, while live provider/MCP/packaged/operator evidence remains post-cutover
validation.

### 3.4 D-0241 Production-Candidate Go Runtime Parity Slice

D-0241 adds an internal production-candidate Go runtime slice below:

```text
/v1/internal/go-production-candidate/*
```

This route family is stronger than the earlier fixture-only scaffold because it
uses local fake live servers/managers instead of static event drafts for the
core runtime parity proof:

```text
live provider fake HTTP/SSE server for DeepSeek, OpenAI-compatible, and Anthropic-compatible request/stream/usage/cache behavior
durable event/session sink replay for real loop event writes
approval/user-input manager for pending, deny, submit, cancel, timeout, and replay
fake MCP manager lifecycle/search/call proof without credentials or public MCP-indexer routes
job/sub-agent lineage manager bound to parent goal/thread state
single-baseline cleanup checklist for D-0241 and post-G6 delete rules
```

The Electron main adapter may select this slice only when both internal gates
are present:

```text
ANALYTIX_RUNTIME_BACKEND=go-production-candidate
ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE=1
```

The D-0241 canary must include the prior backend-neutral boundary checks plus
live provider fake server, durable replay, approval/user-input manager, fake
MCP manager, job lineage, and single-baseline checklist checks. Failure must
stop Go, clean temporary durable state, record fallback status, and return to
the TypeScript runtime.

D-0241 is still not a default backend switch. It must not expose a renderer
backend selector, preload bridge change, settings schema field, Reasonix
SessionAPI/config root/public subagent protocol, Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer product entry, or public runtime route. DeepSeek cache
handling may use Reasonix-derived precedence and prefix-stability discipline,
but OpenAI/Anthropic/custom endpoint behavior must remain separate and
provider-family-aware.

### 3.5 D-0242 Go Runtime Server Contract Slice

D-0242 adds a real Go runtime contract server command:

```text
packages/runtime-go/cmd/runtime-server
```

Unlike the D-0241 proof route, this server exposes an analytix runtime
HTTP/SSE contract subset:

```text
GET /health
GET /v1/runtime/info
GET /v1/threads
GET /v1/threads/:id
PATCH /v1/threads/:id
POST /v1/threads/:id/fork
POST /v1/sessions/:id/resume-thread
POST /v1/sessions/:id/resume
GET /v1/threads/:id/events
POST /v1/threads/:id/turns
POST /v1/approvals/:id
POST /v1/user-inputs/:id
```

The server may run only behind the new internal gate:

```text
ANALYTIX_RUNTIME_BACKEND=go-runtime-candidate
ANALYTIX_GO_RUNTIME_CANDIDATE=1
```

The main adapter canary must probe real contract endpoints, including runtime
info, thread read/patch/fork/resume, turn creation, SSE replay, approval and
user-input resolution, and absence of renderer-visible Go/proof routes. Failure
must stop Go, remove temporary durable state, record fallback status, and start
the TypeScript runtime.

D-0242 connects the D-0241 production-candidate components to the runtime
contract: fake live provider client, durable event/session sink,
approval/user-input manager, fake MCP manager, and parent goal/thread-bound job
lineage. D-0241 proof routes for provider-live, durable-replay,
approval-user-input, MCP manager, and job-lineage become post-D-0242/G6 delete
candidates once the runtime server contract remains covered by conformance.

D-0242 is not a default backend switch. It must not expose a UI/backend
selector, change renderer/preload/main public contracts, add a settings schema
field, import Reasonix SessionAPI/config roots/public protocols, or create
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer product entries.

### 3.6 D-0243 G6 Readiness Hardening Slice

D-0243 adds executable G6 readiness scaffolds around the D-0242 runtime server
without changing the renderer/preload/main public contract or adding another
public proof route.

The main adapter must expose an internal readiness status that defaults to:

```text
ready:false
defaultGoBackendEnabled:false
rendererVisibleGoSwitcher:false
```

The `go-runtime-candidate` canary may accept Go only when all D-0243 hard
evidence is passed:

```text
ANALYTIX_GO_RUNTIME_G6_READY=1
durableRestartProof
credentialedProviderMatrix
credentialedMCPMatrix
packagedQa
```

Missing or skipped evidence is not a pass. Failure must stop Go and return to
the TypeScript runtime. The readiness status is a main-adapter/canary concern;
runtime-info may expose it only as optional audit metadata under
`capabilities.upstreamAbsorption`, and it must not add a renderer-visible
backend switcher or make matrix-green metadata authoritative for backend
selection.

Current runtime-server validation uses an explicit local durable root through
`--runtime-durable-root`. The retired candidate durable-root CLI alias is not a
production entry point; any remaining candidate durable-root API is restricted
to non-`analytix_prod` compatibility tests. Durable-root drills must not write a
real user workspace and must prove restart recovery from `events.jsonl`,
`highestSeq`, thread/session replay, pending approval/user-input gates, and
turn sequence continuation.

Provider matrix scaffolding must cover DeepSeek, OpenAI-compatible,
Anthropic-compatible, and custom endpoint modes. Fake probes must run without
credentials. Credentialed probes are env-gated and must report `skipped` when
the required key/baseUrl/model variables are absent. Fake/local matrix passes
are local proof only; they must not satisfy the credentialed default-backend
gate. DeepSeek cache hit/prefix stability must have a repeatable benchmark
record format.

MCP matrix scaffolding must run a fake MCP lifecycle and optionally accept
credentialed env-gated configuration. It must cover connect, search, call,
reconnect, approval, and credential redaction while keeping MCP-indexer and
Reasonix public protocol surfaces absent. A fake/local MCP pass must not satisfy
the credentialed MCP default-backend gate without
`ANALYTIX_D0243_MCP_COMMAND` or `ANALYTIX_D0243_MCP_URL`.

Packaged desktop QA remains an external gate, but D-0243 must provide an
executable checklist/script for packaged app startup, Go runtime candidate
gate, health, thread list, turn create, SSE replay, and the historical
rollback-to-TypeScript proof used before cutover. Current production rollback
means restoring a previous release or receiving a retired-backend diagnostic;
it does not mean launching a TypeScript agent runtime from the current app.
The script may skip when packaged app/runtime state is not configured, but
skip is not pass.

Post-D-0243 delete candidates are machine-readable: D-0241 provider-live,
durable-replay, approval-user-input, MCP manager, and job-lineage proof routes
are covered by runtime contract/drill evidence and should be removed after G6
default backend criteria pass. D-0242 temp-only durable startup remains a
fallback for conformance tests; candidate durable root is the migration path.

### 3.7 D-0244 Reasonix Capability Red Matrix

D-0244 converts the Reasonix absorption baseline into a machine-readable red
matrix instead of adding more proof routes. The matrix is pinned to the local
upstream checkout:

```text
/Users/sun/Projects/DeepSeek-Reasonix
7032f39336f4ae5f216e1fcb3368e5f679723490
```

The Go runtime server may expose only analytix-owned absorption metadata under
the existing runtime info contract:

```text
/v1/runtime/info
capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix
```

Goal evidence absorption, when green, must be exposed as an analytix-owned
runtime event such as `goal_evidence_audit` on the existing thread event
stream. It must not add Reasonix `/goal` protocol surfaces, SessionAPI payloads,
config roots, or renderer-visible goal control entries.

MCP lifecycle absorption, when green, must be exposed as analytix-owned runtime
events such as `tool_catalog_changed`, `tool_call_finished`, and
`mcp_lifecycle_audit` on the existing thread event stream. It must not add a
Reasonix MCP-indexer route, public SessionAPI/job protocol, Reasonix config
root, renderer-visible MCP-indexer entry, or product identity drift.

Each row must include Reasonix source paths, an analytix landing, one of
`replace`, `code-port-and-adapt`, `contract-reimplement`, `reject`, or
`defer`, machine check IDs, status, and blockers when red or deferred. Green
rows require code-level analytix evidence and passing test/benchmark/
conformance IDs. Red rows are G6 blockers until their checks pass.

The D-0244 matrix must not add Reasonix SessionAPI/config roots, Reasonix CLI
identity, MCP-indexer public routes, public sub-agent/job protocol, renderer
contract drift, product identity drift, or Kun product-entry drift.

### 3.8 D-0247 Readiness Semantics And Default Backend Gate

D-0247 separates two meanings that must remain distinct:

```text
D-0244/D-0245 matrix green: no red/deferred absorption rows and no forbidden product drift
D-0243 default backend ready: durable restart, credentialed provider matrix, credentialed MCP matrix, packaged QA, and explicit env gate all passed
```

`readyForG6` may remain in D-0244/D-0245 objects for compatibility, but it must
be treated as a matrix-green alias only. Runtime-info must expose clearer
contract fields:

```text
capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix.capabilityMatrixGreen
capabilities.upstreamAbsorption.reasonixAbsorptionMatrixD0245.absorptionMatrixGreen
capabilities.upstreamAbsorption.readinessSemanticsD0247
capabilities.upstreamAbsorption.defaultBackendReadinessD0243
```

`readinessSemanticsD0247` must state that `matrixGreenEnablesDefaultBackend` is
false, `skippedCountsAsPassed` is false, and
`fakeMatrixCountsAsCredentialedPass` is false. D-0246 clears the AutoResearch
project-state blocker, but it does not complete G6/default backend readiness.

Historically, the main adapter kept TypeScript as the backend unless all of the
following were present:

```text
ANALYTIX_RUNTIME_BACKEND=go-runtime-candidate
ANALYTIX_GO_RUNTIME_CANDIDATE=1
ANALYTIX_GO_RUNTIME_G6_READY=1
D-0243 durable restart proof passed
D-0243 provider matrix passed with complete credentialed provider env
D-0243 MCP matrix passed with MCP command or URL env
D-0243 packaged QA passed
```

The current adapter now selects `go-runtime-default` by default after the
deterministic cutover. The legacy `go-runtime-candidate` evidence variables
remain useful for post-cutover live validation and future fallback retirement
authorization, but they are no longer the default runtime startup path.

The consolidated readiness report must aggregate Go server conformance,
durable restart, provider matrix, MCP matrix, packaged QA, product sovereignty,
typecheck, and runtime build evidence. A dry-run skipped result is not a pass,
and fake matrix evidence is local proof only.

## 3.9 Non-Exclusive Source Rule

The upstream roles in this spec are default research lenses, not exclusive
capability boundaries. No upstream owns a capability category. Every analytix
capability may compare all nine registered sources and future admitted sources.
The best proven design may be absorbed only after source admission and after it
is adapted into analytix-owned contracts, product surfaces, tests, benchmarks,
and QA.

The hard boundary protects analytix sovereignty, not source territories:

```text
protect: identity, settings, bridge, runtime protocol, data model, safety
do not protect: artificial ownership of UI/runtime/agent/memory capability areas
```

## 4. Authority Order

When upstreams conflict, use this order:

```text
1. analytix specs and frozen decisions
2. security, privacy, approval, sandbox, and data-loss constraints
3. analytix shared contracts, settings schema, bridge, runtime protocol, and data model
4. analytix current product UI and user workflows
5. measured capability evidence, tests, benchmarks, and desktop QA
6. implementation quality, simplicity, and maintainability
7. default research-lens value
```

Default research-lens value means:

- Kun has more weight for GUI product workflows and user-facing feature ideas.
- Reasonix has more weight for agent engine, tool execution, DeepSeek/cache,
  and Go runtime implementation ideas.
- OpenCode has more weight for comparing provider registry, task/TODO,
  sub-agent permission, session-continuation, and background-work designs
  against Reasonix and analytix.
- CodexDesktop-Rebuild has more weight for desktop smoothness architecture,
  virtualized thread layout, app-server style boundaries, and worker/signal
  patterns.
- Hermes Agent has more weight for learning loop, skill evolution, memory,
  gateway delivery, scheduling, terminal backend, tool RPC, and trajectory
  compression research.
- Claude Code has more weight as a clean-room behavior baseline for durable
  attach/resume, remote control, managed policy, and review workflows.
- Claw Code has limited weight for small verified lifecycle/Rust fixtures, not
  production control-plane maturity.
- Gajae Code has more weight for external-control contracts, session trees,
  plugin quarantine, and receipt design, subject to its blocked provenance.
- LazyCodex has more weight for optional LSP/CodeGraph plugin and evidence
  receipt ideas; unreviewed gitlink content carries no weight.

These weights only help decide where to look first. They must never be used to
block another upstream from informing the same capability when evidence says it
is better.

No upstream has authority to change analytix identity, settings schema,
bridge shape, runtime event contract, release identity, or upgraded UI shell.

## 5. Conflict Matrix

| Conflict area | Default ruling |
| --- | --- |
| Product name, package name, release identity, env prefix | analytix specs win. |
| Settings schema and migrations | analytix top-level `runtime` wins. |
| Preload bridge and IPC shape | `window.analytix` and shared schemas win. |
| Renderer UI, layout, shell, visual identity | analytix UI wins. |
| Code / Write / SDD / Connect Phone / Schedule features | Compare all relevant upstreams; Kun is the inherited workflow regression floor, not the future ceiling. |
| Agent loop, tool calling, MCP, cache, context, CLI engine | Compare all relevant upstreams; Reasonix is the mandatory DeepSeek/runtime benchmark, not the only engine source. |
| Sub-agent, TODO/task, session continuation, background jobs | Compare Reasonix, OpenCode, Hermes Agent, Kun, CodexDesktop-Rebuild, and analytix; land only through analytix task-job, Goal, and thread contracts. |
| Memory, skills, learning loop, cross-channel gateway | Compare Hermes Agent and all relevant sources with analytix Skills, memory, Connect Phone, and schedule; do not expose upstream protocols. |
| Thread events, session history, replay, archive/search/fork | analytix data model and compatibility tests win. |
| Provider endpoint behavior and request bodies | analytix shared provider contract wins; upstream code may improve implementation. |
| Security, approvals, user input, sandbox, permissions | Stricter and more user-controllable behavior wins. |
| Performance optimizations | Benchmarks, traces, and desktop QA decide. |
| Runtime implementation ownership | Production remains Go-only in `packages/runtime-go`; TypeScript owns public contracts/config/telemetry/launcher and retained test/conformance oracles. Renderer remains implementation-neutral. |
| Visual assets and icon structure | analytix brand wins; references stay behind provenance/manifest boundaries. |

## 6. Absorption Classifications

Every upstream change must be classified before implementation:

| Class | Meaning | Required action |
| --- | --- | --- |
| `must absorb` | Fixes correctness, data loss, security, runtime parity, or major user-visible breakage. | Implement an Analytix-native fix promptly with tests; direct porting still requires reuse admission. |
| `should absorb` | Improves core workflows, performance, provider compatibility, or maintainability. | Schedule in the current or next upgrade batch. |
| `optional` | Useful but not central to analytix direction. | Keep in backlog with rationale. |
| `reject` | Conflicts with analytix identity, UI, schema, architecture, or quality bar. | Record reason in conflict decisions. |
| `needs redesign` | Valuable upstream idea cannot land directly. | Create analytix-native design before implementation. |
| `defer pending evidence` | Potentially useful but lacks benchmark, QA, or product proof. | Add spike or measurement task. |

Changes must not be merged as unclassified upstream drift.
Classification is downstream of license/provenance admission. A `must absorb`
or `should absorb` capability may still require clean-room reimplementation if
the source material itself is not admitted for direct reuse.

## 7. Upstream Intake Workflow

Use this workflow for every registered upstream upgrade:

```text
1. Run the strict source audit and capture version, tag, commit range, release notes, and local diff.
2. Create or update the matching sync ledger.
3. Pin license/provenance evidence, file/vendor exceptions, reuse mode, and notice destination.
4. Classify every meaningful capability change.
5. Build a conflict matrix for changes touching analytix contracts or UI.
6. Decide whether each item is admitted direct port, adapter port, clean-room redesign, reject, or defer.
7. Implement only through analytix-owned modules and contracts.
8. Add or update tests, fixtures, and QA scenarios.
9. Run the smallest relevant checks, then broader checks for cross-layer changes.
10. Record absorbed, skipped, conflict, validation, notice, and follow-up outcomes.
```

Required ledger updates:

```text
docs/analytix/upstreams/kun-sync.md
docs/analytix/upstreams/reasonix-sync.md
docs/analytix/upstreams/opencode-sync.md
docs/analytix/upstreams/codexdesktop-rebuild-sync.md
docs/analytix/upstreams/hermes-agent-sync.md
docs/analytix/upstreams/claude-code-sync.md
docs/analytix/upstreams/claw-code-sync.md
docs/analytix/upstreams/gajae-code-sync.md
docs/analytix/upstreams/lazycodex-sync.md
docs/analytix/upstreams/upstream-sources.json
docs/analytix/upstreams/code-reuse-provenance.md
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/conflict-decisions.md
```

Stage-closure rule:

```text
An implementation `/goal` should close a stage, not a single checklist item.
The agent should continue through intake, implementation or record-only
classification, tests, benchmark evidence, docs/spec/ledger updates, and commit
unless it hits a real blocker requiring user decision, external credentials,
Windows hardware, signing/release access, or a failed validation that cannot be
safely attributed.
```

Minimum stage closure checklist:

```text
git status --short --branch
upstream currentness recheck
capability-first all-relevant-source classification
conflict decision if product/contract boundaries are touched
implementation or explicit record-only/defer decision
focused tests and required scans
benchmark/scorecard update when superiority is claimed
docs/spec/ledger update
commit or explicit no-commit reason
next-stage gate decision
```

## 8. Kun Absorption Rules

Kun changes are product-workflow inputs.

Absorb from Kun when it improves:

- Code / Write / SDD / Connect Phone / Schedule workflows;
- requirement drafting, design/prototype, planning, todo, review, and history;
- GUI command behavior, composer workflows, settings behavior, plugin flows;
- model provider presets or desktop product bug fixes;
- docs that clarify retained analytix capabilities.

Do not absorb Kun changes when they:

- revert analytix UI structure, visual system, bridge, settings, runtime package,
  or release identity;
- reintroduce current-product Kun naming;
- reintroduce old provider switchers, old runtime-control panels, or removed
  process managers;
- bypass analytix thread projection, virtualizer, scheduler, or desktop QA
  requirements;
- weaken approvals, sandbox, user input, or data safety.

Landing rule:

```text
Kun may define what product capability is worth absorbing.
analytix decides how that capability is designed, named, persisted, and tested.
```

Product-position rule:

```text
When claiming Kun parity, analytix must preserve the target Kun version's
product position, entry layer, and trigger path. Moving a capability to a
higher or different analytix entry level, or promoting a later Kun current/main
idea, is allowed only as an analytix-native product decision with an independent
spec, explicit UX rationale, and renderer/desktop QA. Later Kun features must
not be represented as historical baseline entries by implication.
```

## 9. Reasonix Absorption Rules

Reasonix changes are agent-engine inputs.

Absorb from Reasonix when it improves:

- Go runtime implementation strategy;
- DeepSeek and OpenAI-compatible cache efficiency;
- model adapters, endpoint parsing, stream parsing, usage parsing, reasoning
  fields, and retries;
- tool calling, MCP/plugin support, shell/file/diff/apply behavior;
- approval/user-input handling;
- context compaction, token estimation, model-history repair;
- CLI packaging, single-binary distribution, performance, or observability.

Do not absorb Reasonix changes when they:

- force the renderer to understand Reasonix-native event shapes;
- expose Reasonix CLI identity or settings as current analytix product surface;
- bypass `window.analytix`, top-level `runtime`, or analytix HTTP/SSE/app-server
  contracts;
- remove desktop GUI workflows that analytix owns;
- weaken cross-platform Electron packaging, desktop QA, or local trace rules.

Landing rule:

```text
Reasonix may define engine techniques worth absorbing.
analytix decides the protocol, desktop integration, data model, and UX behavior.
```

Proof rule:

```text
Reasonix absorption is not complete when code compiles. For engine/runtime
changes, analytix must prove either measured parity with Reasonix or a measured
improvement in the affected dimension. Cache/prefix-cache work must prove
stable prefix shape, canonical tool schema hashing, provider/model/endpoint
attribution, cache hit/miss handling for providers that support it, graceful
unsupported behavior for providers that do not, and no regression in tools,
approvals, user input, generated files, review UI, or non-DeepSeek providers.
```

Code reuse rule:

```text
Reasonix code may be copied, ported, or wrapped behind analytix contracts only
after file-level provenance is recorded and its MIT copyright/permission notice
has a destination. Valuable behavior may otherwise be independently
reimplemented. Do not copy Reasonix public protocol, settings root, CLI
identity, terminal-only UI assumptions, or Go implementation details into
renderer-visible contracts.
```

### 9.1 Reasonix Agent-Kernel Batch Order

Reasonix absorption uses this risk-priority order as the default safety gate.
It is not a source-exclusive roadmap. A future analytix-native design may revise
the order only when it records the capability comparison, preserves the safety
floor, and updates the relevant fixtures and scorecards.

| Order | Batch | Required contents | analytix landing | Gate |
| --- | --- | --- | --- | --- |
| 1 | Task closure and permission kernel | `permission Gate`; approval posture `ask` / `auto` / `yolo`; separate plan approval and tool approval; `complete_step` plus evidence ledger; Goal state machine; blocked-state detection; headless/subagent approval rules. | Existing chat / goal / runtime approval contracts. | Must be specified, implemented, tested, and scorecarded before sub-agent or parallel execution expansion. |
| 2 | Long-running task system | `/goal --research`; AutoResearch project-local state; `task_spec.md`; `progress.json`; `findings.jsonl`; `directions_tried.json`; `iteration_log.jsonl`; requirement-by-requirement evidence audit. | analytix-owned project-local goal/runtime state. | Must not pollute `REASONIX.md`, `AGENTS.md`, stable system prefix, tool schema, or renderer-visible Reasonix protocol. |
| 3 | Tool surface and context economy | token economy mode; `connect_tool_source`; dynamic tools; stable tool schema; history/memory on-demand retrieval; compaction archive; cache diagnostics. | Multi-provider tool/provider/runtime contracts. | Must preserve OpenAI, Anthropic, MiMo, DeepSeek, OpenAI-compatible, and custom endpoint request/body/header/stream/usage behavior. |
| 4 | Collaborative execution model | `task`; `parallel_tasks`; background jobs; planner/executor Coordinator; subagent transcript continuation/fork; nested event rendering. | Product-grade execution inside existing chat / goal / runtime / settings surfaces. | Must not be claimed by only enabling `subagents.enabled` or `delegate_task`. |
| 5 | Go runtime kernel | Provider Registry; Tool Registry; Controller; Session; Event Sink; Job Manager; Permission Gate; MCP Client; Memory/History Retrieval; Goal Runtime. | Backend-neutral Go runtime behind analytix contracts and TS oracle G0-G6. | May start only after TS oracle/conformance is stable for the earlier kernel surfaces. |

Task closure and permission proof are a safety prerequisite for expanding
sub-agents, `parallel_tasks`, or Go execution surfaces. Without task closure,
permissions, evidence, Goal state, and completion proof, sub-agent and parallel
execution become uncontrolled process amplifiers. Future work may change
implementation sequencing only by preserving this safety invariant explicitly.

2026-06-21 implementation status: the first scoped Batch 1 TypeScript proof is
now in progress. It adds `complete_step`, `ThreadGoal.evidenceLedger`,
evidence-gated goal completion, and focused fixtures for denied no-execute,
ask/auto/yolo posture mapping, plan/tool separation, blocked-state detection,
and headless/subagent approval inheritance. This does not close full Batch 1,
does not authorize AutoResearch, collaborative execution, Go/Rust scaffold, or
Reasonix public protocols, and does not claim Reasonix parity or superiority.

2026-06-21 Batch 2 implementation status: a scoped TypeScript AutoResearch
proof now exists behind the existing chat/goal surface. `/goal --research`
creates analytix-owned project-local state under
`.analytix/autoresearch/<threadId>/` with `task_spec.md`, `progress.json`,
`findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`.
`record_research_direction` records attempted directions, and `complete_step`
with `requirement_id` is required before a research goal can complete. This
does not expose a Reasonix protocol, stable-prefix content, dynamic tool schema
state, or a top-level AutoResearch / Workflow / Subagent route, and it does not
authorize collaborative execution or Go/Rust scaffold.

2026-06-21 Batch 3 implementation status: a scoped TypeScript tool/context
economy proof now exists. Dynamic source lifecycle is represented by
analytix-owned `connectToolSource` / `disconnectToolSource` registry semantics;
canonical tool catalog fingerprints stay stable across source connection order;
and `CacheDiagnostics` reports tool-source changes separately from stable-prefix
changes. Existing token economy,
request-history hygiene, memory retrieval, compaction history/archive, usage,
and multi-provider cache fixtures remain green. This does not rewrite provider
request body/header/stream behavior, does not make analytix DeepSeek-only, does
not expose Reasonix protocol, and does not authorize collaborative execution or
Go/Rust scaffold.

2026-06-21 Batch 4 implementation status: a scoped TypeScript delegated
collaboration proof now exists. Existing `delegate_task` child runs can attach
completed child work to an active parent Goal evidence ledger, and nested child
metadata including `evidenceLedgered` / `evidenceLedgerError` survives runtime
events, reducer replay, and renderer projection. Existing fan-out,
maxParallel queueing, queued abort, failure, interruption, and headless child
 approval fixtures remain green. This does not expose Reasonix `task` /
`parallel_tasks`, a background Job Manager, planner/executor Coordinator, or a
top-level Subagent UI, and it does not authorize Go/Rust scaffold.

2026-06-21 Batch 5 implementation status: Go runtime kernel work has advanced
from TS-only TypeScript G0/G1 shadow conformance to an isolated Go G1 shadow
scaffold. The manifest in
`packages/runtime/src/conformance/go-runtime-kernel-conformance.ts` maps
Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager,
Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime to
existing TypeScript authority files and oracle tests. The shared oracle fixture
`packages/runtime/src/conformance/fixtures/go-g1-shadow-oracle.json`,
`packages/runtime/tests/go-runtime-conformance.test.ts`, and
`packages/runtime-go` pin `/health`, `/v1/runtime/info`, and
`/v1/runtime/tools` as G1 health/config/capabilities shadow route contracts.
This does not add a default backend, Electron main integration,
renderer-visible Go routes, Reasonix public protocol, Rust scaffold, Tauri
migration, or bridge/settings changes; `analytix serve` remains TypeScript.

Negative constraints:

- do not copy the Reasonix terminal product shape;
- do not make analytix DeepSeek-only;
- do not let upstream drift implicitly add top-level product routes;
- future AutoResearch, Workflow, Subagent, MCP-indexer, or other new product
  entries require an explicit analytix-native spec, UX rationale, route-surface
  tests, desktop QA, and product-sovereignty scan updates;
- do not import Reasonix public protocol, settings root, CLI identity, or
  renderer-visible event semantics;
- do not bypass spec, code, tests, benchmark, and scorecard proof.

### 9.2 OpenCode Comparison Rules

OpenCode is an auxiliary comparison source for agent architecture. It is most
useful when analytix is deciding between competing Reasonix, Hermes Agent, and
analytix-native designs for sub-agents, TODO/task state, background execution,
provider/model inheritance, and session continuation.

Allowed absorption:

- provider/model registry breadth and provider inheritance ideas;
- explicit sub-agent permission derivation;
- task/TODO state models that improve analytix Goal, Plan, and evidence
  contracts;
- session continuation through metadata instead of prompt-level transcript
  injection;
- background task metadata and parent/child lineage ideas.

Forbidden absorption:

- OpenCode product identity, protocol, or UI shell;
- permission behavior weaker than analytix approval/sandbox/user-input rules;
- task/TODO semantics that bypass analytix Goal, Plan, evidence ledger,
  runtime events, or thread projection;
- provider fallback behavior that hides a missing explicit provider;
- sub-agent continuation that injects previous transcripts into stable prompts
  and hurts DeepSeek cache stability.

OpenCode cannot decide an implementation by itself. For sub-agent, TODO, and
background job work, compare OpenCode against Reasonix, Hermes Agent, and the
current analytix contracts before implementation.

### 9.3 Hermes Agent Reference Rules

Hermes Agent is a reference source for self-improving agents and cross-channel
operations. It is valuable for memory/search, skill evolution, gateway delivery,
scheduling, terminal backend portability, tool RPC, sub-agent parallelism, and
trajectory/compression research.

Allowed absorption:

- memory/search ideas that improve analytix-owned memory and thread recall;
- skill creation and skill self-improvement ideas behind analytix Skills
  contracts;
- gateway delivery reliability ideas that improve Connect Phone and schedule;
- terminal backend portability ideas behind analytix tool-host and sandbox
  boundaries;
- tool RPC and trajectory-compression ideas as benchmark/research inputs;
- sub-agent parallelism ideas only after comparing all relevant sources,
  including Reasonix, OpenCode, Hermes Agent, Kun, CodexDesktop-Rebuild, and
  current analytix, plus task-job contract design.

Forbidden absorption:

- Hermes CLI identity, config shape, public memory protocol, or messaging
  identity;
- autonomous skill or memory mutation without explicit analytix ownership,
  privacy rules, audit trail, and opt-in controls;
- cross-channel gateway behavior that bypasses Connect Phone identity,
  approvals, user input, schedule, or thread/session durability;
- terminal backend behavior that weakens local sandboxing, redaction, or
  desktop trace rules;
- trajectory generation as a user-facing feature without a separate spec and
  privacy review.

Hermes Agent may inspire future analytix learning loops, but analytix must keep
the memory, skill, gateway, and scheduling contracts product-owned and
auditable.

## 10. CodexDesktop-Rebuild Reference Rules

CodexDesktop-Rebuild remains a reference source, not an implementation owner.

Allowed absorption:

- independently specified bottom-distance thread virtualizer and scroll layout
  behavior;
- app-server facade and connection signal patterns;
- worker/chunk decomposition for runtime, markdown, code, diff, and panels;
- composer controller/view separation;
- desktop QA and trace evidence patterns;
- visual rhythm, spacing, shadows, borders, motion, loading, and empty-state
  behavior when independently reimplemented with analytix-owned or separately
  admitted assets/tokens.

The reviewed repository has no project-wide license and its ignored `src/*`
extracts are not commit-pinned source. Therefore "allowed" here means
clean-room behavioral requirements and tests. It does not authorize copying
scripts, extracted code, prompts, binaries, icons, sprites, or other assets.

Forbidden absorption:

- Codex product identity, app identity, or implementation ownership naming;
- minified chunks as unreadable production modules;
- generated/extracted code or assets absent a material-specific written
  authorization and provenance record;
- renderer changes that delete analytix product workflows;
- relying on CodexDesktop-Rebuild as a live upstream runtime.

## 11. Historical Contract-First Go Runtime Strategy

The Go runtime was introduced as an analytix-compatible backend, not as a
second product, and is now the only production agent core. The staged path
below is retained as cutover history, not a dual-runtime construction plan.

Required invariants:

- public CLI remains `analytix serve`;
- `ANALYTIX_READY` readiness line remains the desktop readiness marker;
- HTTP/SSE and any app-server facade remain analytix contracts;
- renderer code must not branch on runtime language;
- Go must continue to satisfy the public TypeScript contract and retained
  conformance fixtures; the retired TypeScript agent loop is not a production
  comparison backend;
- Go backend must preserve approvals, user input, usage, session replay, fork,
  archive/search, workspace status, and schedule/Connect Phone thread parity.

Recommended staged path:

| Stage | Goal | Gate |
| --- | --- | --- |
| G0 | Contract inventory and fixture capture from TypeScript runtime. | Golden fixtures checked into tests or generated deterministically. |
| G1 | Go runtime scaffold implements health, readiness, config, capabilities. | Desktop supervisor can launch it behind an internal backend flag. |
| G2 | Go implements read/list/search/archive/fork/session routes and SSE replay. | Same route fixtures pass for TypeScript and Go. |
| G3 | Go implements model adapter, stream parsing, event recorder, and usage. | Transcript and usage conformance pass. |
| G4 | Go implements tool calling, approvals, user input, MCP/plugin, shell/file/diff. | Tool/approval/user-input conformance pass. |
| G5 | Go implements full agent loop, compaction, cache accounting, resume, interrupt. | Long-thread and desktop QA pass with Go backend. |
| G6 | Go backend becomes the only production agent runtime; TypeScript executable fallback is retired, and rollback means release rollback or a retired-backend diagnostic rather than launching TS runtime. | Release readiness, rollback plan, and migration notes complete. |

Do not skip from G1 to G6. A Go rewrite without conformance gates is not an
upgrade path; it is a second runtime. G1/G2 also may not start as a shortcut
around the Reasonix agent-kernel order in section 9.1; task closure,
permission, evidence, Goal, and provider/tool oracle fixtures must be stable
before Go claims runtime-kernel parity.

## 12. Golden Oracle And Conformance

Until Go is default, the TypeScript runtime is the golden oracle for current
analytix behavior. That does not mean the TypeScript implementation is perfect;
it means changes must intentionally update the contract and fixtures before a
new backend diverges.

Minimum conformance fixture families:

- runtime health and readiness;
- settings/config normalization;
- provider request URL/body/header behavior, including per-thread
  `thread.providerId` routing and custom full endpoint modes;
- streaming text and reasoning deltas;
- tool call lifecycle and tool result image/file handling;
- approvals and user input;
- interrupt/resume/fork/archive/search;
- context compaction and model-history repair;
- usage/cache hit/miss parsing;
- optional usage cache diagnostics, when emitted, including stable prefix hash,
  prefix-change reasons, canonical tool-schema hash/token estimate, provider
  and model metadata, and provider cache hit/miss token counts;
- checkpoint metadata and rewind planning, including analytix-owned checkpoint
  ids, changed-file metadata, event-log projection, and conversation-only
  rewind before any file restore is enabled;
- schedule and Connect Phone thread creation/reuse;
- long-thread event replay from `events.jsonl`;
- malformed or partial event recovery.

Conformance output must compare:

```text
HTTP status
response schema
SSE event sequence
durable event log
projected ThreadRow / transcript state
usage summary
error shape
sanitized logs/traces
```

## 13. Runtime Contract Freeze Points

Before absorbing a large Reasonix runtime change or starting a Go backend stage,
freeze or explicitly revise these contracts:

- `packages/runtime/src/contracts/events.ts`
- `packages/runtime/src/contracts/items.ts`
- `packages/runtime/src/contracts/threads.ts`
- `packages/runtime/src/contracts/turns.ts`
- `packages/runtime/src/contracts/capabilities.ts`
- runtime HTTP routes under `packages/runtime/src/server/routes/`
- renderer mapping in `src/renderer/src/agent/analytix-mapper.ts`
- main runtime adapter in `src/main/runtime/analytix-adapter.ts`
- preload runtime domain in `src/preload/index.ts`
- settings schema in `src/shared/app-settings-runtime.ts`
- provider URL/request contract in `src/shared/openai-compat-url.ts`

Contract changes must be reviewed as cross-layer changes, not local runtime
refactors.

### 13.1 Checkpoint And Rewind Safety Rules

Checkpoint and rewind contracts SHALL remain analytix-owned:

- checkpoint ids, metadata, events, storage, logs, and any future refs SHALL use
  analytix naming and schema ownership;
- `refs/kun/checkpoints` SHALL NOT appear in production code or runtime data;
- Reasonix CLI/settings/renderer protocols SHALL NOT become analytix public
  protocols;
- checkpoint metadata SHALL be versioned, deterministic enough for oracle
  tests, and idempotent across replay;
- changed-file paths SHALL be normalized relative to the workspace and SHALL
  reject path escape;
- conversation-only rewind SHALL be provable from the durable runtime event log
  and transcript projection before any file restore or UI control is enabled;
- destructive checkpoint rewind apply SHALL execute only from a reviewed
  analytix `CheckpointRewindPlan`, not directly from a checkpoint id;
- destructive apply SHALL require explicit user confirmation, validate the
  submitted plan against the current checkpoint/event-derived safety plan, and
  create an analytix-owned rescue record before mutating workspace files;
- file apply SHALL block path escape, absolute paths, parent/final symlinks,
  staged/untracked conflicts, manual-review plans, missing snapshot/hash
  evidence, and snapshot content/hash mismatches instead of guessing recoverable
  content;
- destructive file mutation SHALL revalidate path, git, symlink, and current-hash
  safety inside the mutation queue, and partial failure audit SHALL preserve the
  files already applied before the failure;
- conversation restore SHALL remain append-only audit unless a separate,
  tested projection or migration strategy proves safe history rewriting;
- file restore, transcript rewrite, or schema migration SHALL NOT silently
  rewrite user data and SHALL have focused tests before release.

## 14. Data And Migration Rules

Upstream absorption must not silently rewrite user data.

Rules:

- active analytix data uses analytix paths and schemas;
- legacy Kun data is imported only through explicit legacy boundaries or
  documented one-shot migration code;
- Reasonix storage formats may inform implementation but must be adapted to
  analytix thread/session/event contracts;
- event replay must remain durable across crashes;
- schema changes need versioning, idempotent migration, tests, and rollback
  notes;
- long event logs require bounded replay, pagination, indexing, or compaction
  before they become a desktop startup risk.

## 15. Security And Permission Rules

When upstream behavior conflicts with analytix permissions, choose the stricter
and more user-controllable path.

Required preservation:

- approval policy semantics;
- sandbox mode semantics;
- user-input prompts and cancellation behavior;
- MCP/tool permission boundaries;
- local-only trace privacy;
- no API keys, tokens, full prompts, full assistant output, or direct personal
  contact/payment identifiers in traces;
- cache diagnostics and provider traces must not contain raw system prompts,
  user prompts, tool arguments, tool output, file contents, full assistant
  output, API keys, tokens, or full unsanitized response bodies;
- clear user-facing errors for provider URL, endpoint format, model, network,
  tool, and runtime startup failures.

## 16. Performance And Desktop QA Gates

An upstream performance claim is accepted only with evidence in analytix.

Required evidence for hot-path changes:

- unit tests for projection, scheduler, virtualizer, event reducer, or runtime
  routes as applicable;
- renderer smoke for long-thread behavior when chat output is affected;
- real Electron desktop QA for final release readiness;
- local traces for batch receive, delta buffer/flush, projection reduce,
  virtualizer measure, scroll anchor, React commit, and markdown finalization;
- no forced scroll-to-bottom when the user is reading history;
- composer input remains responsive during streaming and terminal bursts.

## 17. Branch And Change Hygiene

Recommended branch naming:

```text
codex/sync-kun-<version>
codex/sync-reasonix-<version>
codex/go-runtime-conformance-<stage>
```

Each upstream absorption batch should produce:

- sync ledger update;
- absorption target matrix update when a batch adds or changes what analytix
  should absorb from an upstream;
- conflict decisions update when needed;
- changed code/tests/docs;
- validation output;
- unresolved follow-up list.

Do not mix unrelated Kun and Reasonix upgrades in one batch unless the changes
touch the same contract and are reviewed as a combined redesign.

## 18. Required Upstream Artifacts

Each upgrade batch must update at least one of:

```text
docs/analytix/upstreams/kun-sync.md
docs/analytix/upstreams/reasonix-sync.md
```

and should update:

```text
docs/analytix/upstreams/absorption-ledger.md
docs/analytix/upstreams/absorption-targets.md
docs/analytix/upstreams/code-level-absorption-blueprint.md
docs/analytix/upstreams/code-level-implementation-plan.md
docs/analytix/upstreams/conflict-decisions.md
docs/analytix/upstreams/go-runtime-conformance.md
```

when the batch touches conflicts, shared contracts, or Go runtime work.

## 19. Acceptance Criteria

A Kun or Reasonix upgrade is absorbed only when:

- every meaningful upstream change is classified;
- every direct conflict has a recorded analytix decision;
- accepted changes are implemented through analytix contracts and UI;
- rejected/deferred changes have reasons;
- tests or QA cover the changed surface;
- provider/runtime batches prove URL construction, request body shape,
  provider id persistence/routing, and any compaction/model-history repair
  needed to keep tool-call/tool-result pairs valid;
- provider/cache batches that emit cache diagnostics SHALL prove the diagnostics
  are optional, backend-neutral, privacy-bounded, and stable against tool-schema
  ordering noise while still attributing real prefix, tool, provider, or model
  cache drift;
- checkpoint/rewind batches SHALL prove analytix-owned ids/events/storage, path
  safety, non-corrupting event replay, and idempotent versioned schemas before
  any destructive restore path is exposed;
- checkpoint rewind plan routes SHALL remain non-destructive until a separate
  apply contract and desktop QA exist; `plan_only` responses must not rewrite
  workspace files, git refs, `events.jsonl`, or `messages.jsonl`;
- checkpoint rewind apply routes SHALL prove explicit confirmation,
  plan-validation, rescue-before-mutation, snapshot/hash safety,
  staged/untracked conflict blocking, append-only conversation audit, and
  idempotency before release closure;
- `git diff --check` passes;
- relevant naming scans do not introduce current-product upstream identity;
- the sync ledger records version, scope, validation, and remaining follow-up.

A Go runtime stage is accepted only when:

- the stage gate in this spec passes;
- TypeScript and Go runtime outputs match the relevant conformance fixtures or
  intentional contract changes are documented;
- desktop supervisor and renderer remain backend-neutral;
- rollback to the previous backend is documented until Go is the only backend.

## 20. Not Complete Conditions

Do not call an upstream absorption complete if any of the following are true:

- upstream changes were merged without classification;
- current UI was overwritten by upstream UI without analytix redesign;
- renderer code depends on Reasonix-native or Kun-native runtime shapes;
- active settings write old agent structures;
- public CLI exposes upstream identity as current product surface;
- accepted runtime behavior lacks conformance tests;
- security/approval/user-input behavior was weakened without explicit decision;
- long-thread or streaming behavior regresses without a recorded blocker;
- conflict decisions are missing for known disagreements;
- sync ledger is not updated.

## 21. Review Checklist

Before merging any upstream absorption or Go runtime stage, answer:

```text
1. Which upstream version/commit range was reviewed?
2. Which changes were must/should/optional/reject/needs-redesign/defer?
3. Which analytix contracts changed?
4. Which UI surfaces changed, and did the analytix UI remain primary?
5. Which data schemas or migrations changed?
6. Which tests and QA evidence cover the change?
7. Did provider URL/body/header behavior remain consistent?
8. Did approvals, user input, sandbox, and tracing remain safe?
9. Did long-thread streaming and composer responsiveness remain acceptable?
10. Is Go/TypeScript backend behavior equivalent where required?
11. Are rejected/deferred upstream items recorded with reasons?
12. Are remaining risks and follow-ups recorded?
```

If any answer is unknown, the batch is not ready for final closure.

## 22. Initial Completeness Audit

This audit records what this spec covers at creation time. It verifies the
long-term plan and governance model, not any future upstream implementation
batch.

| Goal | Coverage | Status |
| --- | --- | --- |
| Make analytix the product and architecture owner | Sections 3, 4, 5 and decision D-0001. | covered |
| Absorb future Kun upgrades without UI regression | Sections 5, 6, 7, 8 and `kun-sync.md`. | covered |
| Absorb future Reasonix upgrades without protocol drift | Sections 5, 6, 7, 9 and `reasonix-sync.md`. | covered |
| Keep CodexDesktop-Rebuild as reference, not product identity | Sections 3, 4, 10 and decision D-0001. | covered |
| Resolve conflicts consistently | Sections 4, 5, 6, 19, 20 and `conflict-decisions.md`. | covered |
| Introduce Go runtime safely | Sections 11, 12, 13 and `go-runtime-conformance.md`. | covered |
| Preserve analytix settings, bridge, CLI, identity, and runtime contracts | Sections 3, 4, 5, 11, 13, 14 and 19. | covered |
| Preserve security, approval, user input, sandbox, and trace privacy | Section 15 and decision D-0004. | covered |
| Require performance and desktop QA evidence | Section 16 and decision D-0005. | covered |
| Make future batches auditable | Sections 7, 17, 18, 19, 21 and upstream ledgers. | covered |

Known limits:

- This spec does not review a specific future Kun or Reasonix release; each
  release still needs its own sync ledger entry.
- This spec does not implement Go runtime code; it defines the gates that must
  be satisfied before Go can become default.
- This spec does not replace the earlier analytix specs; it extends them for
  long-term upstream absorption.
- This spec does not prove that analytix is stronger after an absorption batch;
  spec `09` and benchmark scorecards provide that proof layer.
- This spec does not change release domain, local project metadata, or signing
  readiness decisions.

Initial audit result:

```text
The long-term absorption scheme is complete enough to govern future Kun and
Reasonix upgrades. Implementation batches still require normal design, tests,
conformance evidence, and desktop QA before they can be called complete.
```

## 23. 2026-06-20 G0/G5 Conformance Inventory Baseline

This section records the current baseline after P4C. It does not supersede the
stage gates above; it makes the next Go/runtime work auditable.

Status:

```text
P4C confirmed rewind restore apply is closed for scoped code behavior and
desktop smoke.
Analytix is not finally complete.
Go runtime work has not started.
G0 is closed only for the current contract inventory baseline.
G5 has an oracle list, not an implementation.
```

TypeScript runtime contract surfaces that must remain frozen or explicitly
versioned before Go work:

```text
thread/session
SSE replay
checkpoint/rewind plan/apply
safety audit
tool calls
approvals
user input
plan/goal
model-history repair
cache accounting
usage
provider request/stream parsing
```

Capability matrix:

| Source | Absorb what | Why better | Required proof | Conflict authority |
| --- | --- | --- | --- | --- |
| Kun 0.2.13 baseline | Product-lineage workflow and safety expectations. | Prevents analytix migration from regressing inherited desktop behavior. | P1.1-P4C regressions, renderer/main tests, desktop QA records. | Registered accepted specs, D-0001, D-0002. |
| Kun 0.2.14/current | Scoped stable deltas for provider/runtime routing, Connect Phone Telegram, hidden Create Loop internals, checkpoint safety, and desktop utility lessons. | Keeps analytix current while preserving `window.analytix`, top-level `runtime`, `analytix serve`, current UI ownership, and target-version entry placement. | Kun sync ledger, absorption ledger, scorecard, runtime suite, route-surface tests, identity/protocol scan. | D-0006, D-0008, D-0009, D-0012. |
| Reasonix main-v2 | Engine/runtime discipline for cache, provider parsing, tools/MCP, approvals, checkpoint/rewind, agent-kernel batch order, and future Go locks. | Gives analytix stronger backend behavior without exposing Reasonix-native protocols or skipping permission/evidence/Goal gates. | Reasonix sync ledger, model/cache/MCP/tool/checkpoint fixtures, cache/prefix-cache benchmark evidence, agent-kernel scorecard rows, future cross-backend oracle runs. | D-0003, D-0004, D-0007, D-0009, D-0010, D-0013, D-0014. |

Golden/oracle requirements:

- Freeze existing TypeScript fixtures listed in
  `docs/analytix/upstreams/go-runtime-conformance.md`.
- Add deterministic route/cross-backend fixtures for resume/interrupt,
  fork/archive/search, approvals, user input, provider request/stream snapshots,
  cache/usage snapshots, and checkpoint apply replay before any Go G5 parity
  claim.
- Keep missing production evidence explicit: live desktop apply,
  crash/restart recovery, packaged QA, live provider matrix, local release
  metadata, signing/notarization, and Windows NSIS.

Baseline runtime-suite attribution:

```text
The 2026-06-20 runtime suite red lights in loop.test.ts were stale tests:
post-file-change tool turns now require final assistant text, and reported
prompt_tokens are ignored when they exceed the local estimate by the configured
trust factor. Tests were updated to the current contracts instead of weakening
runtime behavior.
```

Not complete:

```text
Do not treat this inventory as a Go scaffold, Go parity, Rust adoption, release
readiness, full Kun parity, full Reasonix parity, or final analytix completion.
```

## 24. 2026-06-20 Reasonix Goal/Control Delta Oracle

This addendum records the first post-G0/G5-inventory runtime absorption batch.
It does not change the stage gates in sections 11-13.

Upstream verification:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `ab24a77f0f68fcb1b2c160361e4c50cd92b02928` |
| Kun | `v0.2.13` tag / peeled | `201a1469ffbd911f6b95d450b78471e51b42acae` / `2ba8decc2f56862e7f677fcf89bbc3d402ec3a23` |
| Kun | `v0.2.14` tag / peeled | `06be05d76223208724c07301fa0f830641a06e6f` / `8f2040349fba47fcd8e8b94f50b131943af839b2` |
| Reasonix | `main-v2` | `bc8249c307b261ef7ad05a0d2d1409c7b83b95ff` |

Reasonix delta result:

| Reasonix change | Result |
| --- | --- |
| `dbaea843` goal FSM extraction | Absorbed as `GoalControlMachine`, an analytix-owned runtime helper. |
| `bb06f5b4` inspect/package-lock cleanup | Classified as no-op for analytix; no matching code or redundant nested lockfile exists. |
| `726036bd` approvalManager after requested `c23d40f` | Recorded as current HEAD drift and deferred to a future approval-control batch. |

Canonical domains preserved:

```text
Renderer -> window.analytix -> preload -> main -> analytix runtime HTTP/SSE
```

Settings/provider schema preserved:

```text
Current runtime settings remain top-level `runtime`.
Provider profiles and capabilities remain under `provider`.
Endpoint behavior remains a shared provider contract: baseUrl fallback,
endpointFormat, custom full endpoint mode, headers, request body shape, stream
parsing, usage parsing, and reasoning fields are tested separately.
```

Rejected public-surface changes:

```text
No Reasonix-native renderer protocol.
No Reasonix goal marker contract.
No Reasonix goal-state sidecar format.
No Kun identity, `kun serve`, `window.kunGui`, `agents.kun`, or `KUN_*`.
No default Go backend, renderer-visible Go route, backend switch, Rust helper,
or release claim.
```

New oracle coverage:

- `file_change` followed by empty final assistant text must retry once with
  tool-continuation recovery and then fail with
  `empty_post_tool_continuation` item/event/`turn_failed`.
- `promptTokens` above `PROMPT_TOKEN_TRUST_FACTOR` must be ignored for
  compaction pressure so inflated cache-read token reports do not cause false
  compaction.
- Goal no-tool recovery, blocked audit guidance, and non-progress goal tools
  are centralized in the internal goal-control helper.

Kun 8602476 umbrella status:

```text
Kun master 8602476 remains partially absorbed through previous scoped slices:
P1.1 provider/runtime routing, P1.2 Connect Phone Telegram, P1.3 Create Loop,
P3/P4 checkpoint safety and related regression evidence. It is not full Kun UI,
workflow builder, release, or desktop parity.
```

Scorecard status:

```text
Current score remains 4/4 on runtime contract reliability, provider/cache/cost,
tool/MCP reliability, safety/privacy, and maintainability for scoped runtime
governance. Score does not advance to release strength until packaged QA, live
provider matrix, live rewind apply, crash/restart recovery, local release
metadata, signing/notarization, and Windows NSIS are complete.
```

## 25. 2026-06-20 Post-4f47031f Currentness Addendum

This addendum updates the latest upstream inputs after the post-goal-control
currentness gate. It does not change the product ownership rules or Go stage
gates above.

Latest upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `9605e20f422c90054d930e4f1a0000886b353895` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | `main-v2` | `48e5b990671ca1e579b895080e74babc1e17c333` |

Classification:

| Upstream change | Result |
| --- | --- |
| Reasonix `3e625b91` SessionAPI driving-port refactor, merged by `48e5b990` | Superseded by section 26. This was the currentness finding before the scoped analytix `RemoteEntryControlPort` implementation landed. |
| Kun `d09d52b` Windows installer process-stop fix on develop `9605e20f` | Historical currentness finding superseded by section 27. The behavior is now absorbed as an analytix-native Windows installer include, and Windows machine QA remains the release blocker. |

Non-goals:

```text
No Reasonix-native renderer protocol.
No Reasonix public SessionAPI naming in renderer-visible contracts.
No default Go backend, renderer-visible Go route, backend switch, Rust helper,
or Tauri migration.
No reopening of the current analytix UI shell.
No release-readiness claim.
```

Next implementation priority:

```text
Absorb the Reasonix control-port boundary before starting Go G1/G2. Section 27
now records the Kun Windows installer process-stop code/doc absorption before
Windows release readiness; Windows machine QA remains required.
Spec 07 blockers remain open until live desktop QA, packaged launch, provider
matrix, release metadata, signing/notarization, Windows NSIS, and verified
analytix remote evidence are complete.
```

## 26. 2026-06-20 Remote-Entry Control-Port Absorption Addendum

This addendum supersedes section 25 for the Reasonix `3e625b91` control-port
item. Section 25 remains historical currentness evidence for the moment before
the scoped implementation landed.

Latest upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | `main-v2` | `c202f97035cd353c4bf3bbd2dc6e94ed22710e67` |

Updated classification:

| Upstream change | Result |
| --- | --- |
| Reasonix `3e625b91` SessionAPI driving-port refactor, merged by `48e5b990` | Scoped-absorbed as analytix-owned `RemoteEntryControlPort` and oracle tests. The product keeps analytix HTTP/SSE, `window.analytix`, top-level `runtime`, `analytix serve`, and `ANALYTIX_*` ownership. |
| Reasonix `1280c0f` / `9ae96d8` / `c202f970` serve/acp/cli SessionAPI migrations | `record` until a behavior delta is classified. Do not copy Reasonix public protocol or broaden the remote-entry port merely because upstream widened its internal `SessionAPI`. |
| Kun `d09d52b` Windows installer process-stop fix on the requested develop checkpoint `9605e20f` | Superseded by section 27. It is now absorbed as an analytix-native Windows installer include with no `KUN_*` or Kun artifact names. |
| Kun `9605e20f..247076f` develop drift | `record` for speech/SSE/UI currentness. Classify separately before absorption; do not mix into the installer or Reasonix control-port batch. |

Current gates:

```text
Reasonix control-port G0 oracle is implemented for the TypeScript runtime.
It does not close full Reasonix parity, Go G1-G6, release readiness, Connect
Phone/schedule live QA, or Windows NSIS verification.
```

## 27. 2026-06-20 P0 Product-Entry Correction And Installer Absorption

This addendum records a product-governance correction before further upstream
absorption.

Latest upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | requested `main-v2` | `c202f97035cd353c4bf3bbd2dc6e94ed22710e67` |
| Reasonix | historical P0-time observed `main-v2` | `5d1ad2ae8cb7a0dbc8775fb3f05e6c204627c2b1` |
| Reasonix | current 2026-06-21 recheck `main-v2` | `49c14762b7da9234525e717830e39a64a2220911` |

P0 correction:

| Item | Result |
| --- | --- |
| Workflow/Create Loop top-level route | Removed/hidden from analytix `AppRoute`, `openWorkflow`, Sidebar command, and Workbench main stage because Kun 0.2.13/0.2.14 do not have an equivalent top-level entry. |
| Create Loop internals | Retained as unexposed future capability; not deleted and not claimed as current product navigation. |
| Kun `d09d52b0` installer fix | Absorbed as analytix-native `build/installer.nsh` and `electron-builder.config.cjs` `nsis.include`, using `ANALYTIX_INSTALLER_*` environment variables, Windows process `ExecutablePath`/`Path` matching, and no `KUN_*` names. |
| Kun develop `247076f` speech/SSE/UI drift | Classified only; speech/local Whisper, SSE IPC reconnect/throttle, and UI drift must be separate batches. |
| Reasonix post-`c202f970` historical `5d1ad2a` and current `49c14762` drift | Classified only; no approvalManager, SessionAPI desktop port, Go scaffold, or Rust/Tauri work starts here. |

Why this is better:

```text
The product surface now matches the proven Kun 0.2.13/0.2.14 lineage for main
entries while still preserving the underlying current/master Create Loop work
for a future spec-approved workflow automation surface. The Windows installer
change improves overwrite-upgrade safety without changing analytix identity,
renderer bridge, settings schema, or runtime protocol.
```

Required proof:

```text
Renderer route tests must cover Sidebar, Workbench, and chat-store route/action
surfaces. Source/config scans must prove no `KUN_*`, `kun serve`,
`window.kunGui`, old product identity, default Go backend, renderer-visible Go
route, Rust scaffold, or Tauri surface was introduced. Windows NSIS machine
upgrade QA remains a release
blocker until it exercises the installer include on Windows.
```

Non-goals:

```text
No full Workflow builder parity.
No release-ready claim.
No Reasonix approvalManager.
No default Go backend, Electron integration, or renderer-visible Go route.
No Rust/Tauri migration.
No Kun develop speech/SSE/UI implementation in this batch.
```

## 28. 2026-06-21 Long-Term Absorption Methodology Addendum

This addendum supersedes any older wording that treats P1.3 Workflow/Create
Loop as current product-navigation parity or treats Reasonix cache absorption
as unfinished from scratch.

Current upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | `main-v2` | `49c14762b7da9234525e717830e39a64a2220911` |

Updated rules:

| Area | Rule |
| --- | --- |
| Kun lineage | analytix starts from Kun `0.2.13` and first preserves/absorbs Kun `0.2.14`; later Kun master/current features are valuable but must be classified by target-version product position before exposure. |
| Product entry | Do not promote later/experimental/current Kun capabilities to top-level analytix navigation unless a dedicated spec, UX rationale, route tests, and benchmark evidence approve the new level. |
| Workflow/Create Loop | Current top-level Workflow navigation is not Kun `0.2.13`/`0.2.14` parity. Retain internals only as unexposed future capability until a new workflow automation spec approves exposure. |
| Reasonix engine | Reasonix may be code-reused for engine/runtime improvements behind analytix contracts; reject Reasonix public protocol, settings root, CLI identity, and renderer-visible event shapes. |
| Cache/prefix-cache | P2.2 already added scoped analytix cache diagnostics. The current stage adds fixture-backed proof for stable prefix, canonical tool schema, provider matrix cache parsing, unsupported fallback, and Go conformance fixtures; live provider parity remains a separate evidence gate. |
| Go/Rust | Go starts only as shadow/conformance after TS oracle fixtures are stable. Rust remains a profiling-driven helper option, not a UI/runtime rewrite or Tauri migration. |

Stage closure:

```text
Future `/goal` prompts should ask for stage closure, not one small follow-up at
a time. The implementation thread should continue until the stage has currentness
evidence, code/doc changes or explicit deferrals, tests/scans, benchmark or
scorecard evidence, and a commit/no-commit decision.
```

## 29. 2026-06-21 Reasonix Latest Currentness And Go G2 Shadow Gate

This addendum updates the post-D-0014 state. It does not change the frozen
decisions in sections 3, 9.1, 11, or 12.

Latest upstream recheck:

| Source | Ref | Resolved commit |
| --- | --- | --- |
| Kun | `master` | `8602476c5c449b4561473ad5f31081ee93dc782e` |
| Kun | `develop` | `247076f297170c3d0c558baffb073c629894faea` |
| Kun | `v0.2.13` tag | `201a1469ffbd911f6b95d450b78471e51b42acae` |
| Kun | `v0.2.14` tag | `06be05d76223208724c07301fa0f830641a06e6f` |
| Reasonix | `main-v2` | `91fe06db6177bb052fc8ae1a3081d60bfc104a4e` |

Reasonix latest classification:

| Upstream change | Result |
| --- | --- |
| `45367085` codebase-memory MCP auto-indexing | `wrap-behind-contract` / future MCP-tool parity hardening: known cwd-aware indexer MCP servers may need workspace-root cwd, low-priority scheduling, and background startup under a shared host. This stage adds a generic MCP/tool lifecycle oracle only; it does not expose Reasonix plugin protocol or codebase-memory UI. |
| `d9e453a1` MiMo built-ins migrated to custom providers | `reject` deleting analytix Xiaomi/MiMo product presets; record legacy/custom-provider migration behavior as a future `contract-reimplement` candidate. Analytix currently keeps Xiaomi/MiMo product presets for chat/speech/first-run UX. |
| `85ff9865` / `91fe06db` merge commits | `record` as latest currentness only. Future code batches start from `91fe06db` unless upstream moves again. |

Go G2 shadow rule:

```text
G2 proceeds only as shadow route replay against TypeScript oracle fixtures
for read/list/search/archive/fork/session routes and SSE replay. It must not
change renderer/preload/main, `window.analytix`, top-level `runtime`,
`analytix serve`, public event semantics, default backend selection, product
navigation, release identity, or backend defaults.
```

G2 evidence matrix:

| Surface | Required TypeScript oracle | G2 claim allowed only after |
| --- | --- | --- |
| Thread/session list/read | `packages/runtime/src/conformance/fixtures/go-g2-route-replay-oracle.json` plus `go-runtime-conformance.test.ts` | Go and TS match status codes, response schemas, list filters, metadata summaries, and `latestSeq`. |
| Search/archive | same shared G2 oracle | Go and TS match archive/search semantics without renderer contract changes. |
| Fork/session lineage | same shared G2 oracle | Go and TS match parent/child lineage, workspace metadata, and summary response shape. |
| Resume-thread | same shared G2 oracle | Go and TS match status/body for `POST /v1/sessions/{id}/resume-thread`. |
| SSE replay | same shared G2 oracle plus Go `shadow_g2.go` | Go and TS match `since_seq` replay frames and caught-up empty replay. |
| Usage/cache summary | usage/cache/provider fixtures | Go preserves optional cache telemetry and unsupported-provider unknown behavior. |

Production scan results recorded for this addendum:

| Gate | Result |
| --- | --- |
| Bridge domain scan | pass: no renderer/preload/shared calls outside the allowed `window.analytix` domains. |
| Forbidden identity/protocol scan | pass: no production hits for `window.kunGui`, `agents.kun`, `kun serve`, `KUN_`, Reasonix-native protocol, Cargo/Rust/Tauri terms. |
| Go scope scan | pass: no default/backend switch or renderer-visible Go route hits in `src`, preload/shared, release config, scripts, or workflows. `packages/runtime-go` exists only as G1/G2 shadow code. |
| Product entry scan | pass: no `workflow` / `openWorkflow` hits in Sidebar, Workbench, AppRoute/store action files. |
| G1/G2 TS oracle | pass: focused G2/parity runtime suite passed, 4 files / 10 tests. |
| G1/G2 Go package | pass: official temporary `go1.26.4.darwin-arm64` from `go.dev`, SHA256 verified, `go test ./...` passed in `packages/runtime-go`; toolchain not committed. |
| Live provider matrix | blocked: checked provider credential env vars were missing. |

Not complete:

```text
Do not claim live provider parity, Reasonix provider superiority, default Go
backend readiness, release readiness, or full Kun/Reasonix parity from this
addendum. It closes only the latest-currentness, fixture-backed parity hardening,
and G2 shadow route replay slice.
```

## 30. 2026-06-21 Reasonix P0 Engine Parity And Kun Desktop Hardening Gate

This addendum closes the scoped Reasonix P0 engine-parity and Kun desktop
hardening stage behind analytix contracts. It extends section 29 without
changing the product boundary: `window.analytix`, top-level `runtime` settings,
`analytix serve`, and Renderer -> preload -> main -> runtime HTTP/SSE remain the
current architecture.

Absorbed in this stage:

| Upstream area | analytix result |
| --- | --- |
| Reasonix provider/cache | Provider/cache proof now covers DeepSeek top-level `prompt_cache_hit_tokens` / miss tokens, OpenAI-compatible and Responses nested cached tokens, Anthropic cache read/create tokens, reasoning tokens, canonical tool schema hashing, stable tool order, unsupported-provider unknown fallback, and sanitized provider/model/endpoint/request URL diagnostics. |
| Reasonix task/background/planner | Added analytix-owned task job contract/oracle and durable job manager skeleton for foreground/background task continuation, wait/output/kill, parallel dependency/cycle validation, nested event metadata, permission inheritance, transcript continue/fork, and planner read-only toolset. |
| Reasonix MCP/codebase-memory | Added codegraph/codebase-memory known override behind analytix MCP config/provider contracts: workspace-root cwd only when no explicit cwd exists, explicit cwd preserved, low-priority/background-start diagnostics, schema cache, cancel/timeout, and secret-safe diagnostics. |
| Go G3/G4 conformance | Added shadow-only G3 provider streaming/usage/cache oracle and G4 tools/approval/user-input/MCP oracle. Go output matches TS-owned fixtures and remains isolated in `packages/runtime-go`. |
| Kun SSE IPC | Absorbed Kun develop SSE IPC hardening as analytix main-process behavior: 100ms pending-event throttle, destroyed-renderer safe send, reconnect with flushed `since_seq`, and focused tests. |
| Kun desktop shell | Consolidated sidebar/back/forward/new-chat controls at the workbench shell level, removed duplicate Write/Connect Phone/Plugin local toggles, and preserved the Kun 0.2.13 -> 0.2.14 entry baseline. |

Rejected or deferred:

```text
Reasonix public protocol, settings root, CLI identity, renderer-visible event
shape, default Go backend, Electron-to-Go supervisor switch, Rust/Tauri rewrite,
top-level Workflow/Create Loop, top-level Subagent/AutoResearch/MCP indexer UI,
Kun current identity, Local Whisper bundled resources, tray session menu, and
live provider superiority claims.
```

Gate evidence:

| Gate | Evidence |
| --- | --- |
| Provider/cache P0 | `packages/runtime/tests/provider-cache-proof.test.ts`, `model-client.test.ts`, `usage-service.test.ts`, `cache.test.ts`. |
| Task/background/planner oracle | `packages/runtime/tests/task-job-orchestration-oracle.test.ts`, `delegation-runtime.test.ts`, `child-agent-executor.test.ts`, `task-job-orchestration-oracle.json`. |
| MCP/codebase-memory lifecycle | `packages/runtime/tests/mcp-tool-provider.test.ts`, `mcp-tool-lifecycle-oracle.test.ts`, `mcp-config.test.ts`, `capability-registry.test.ts`. |
| Kun SSE IPC | `src/main/runtime-sse-ipc.test.ts`. |
| Go G3/G4 | `packages/runtime/tests/go-runtime-g3-g4-conformance.test.ts`; `packages/runtime-go/shadow_g3g4.go`; `go test ./...` with a temporary SHA-verified official Go toolchain. |
| Shell baseline | `ShellNavigationControls.test.tsx`, `WriteWorkspaceToolbar.test.ts`, `ConnectPhoneView.test.ts`, `Workbench.route-surface.test.ts`. |

This stage may be described as scoped fixture/oracle parity for the named P0
engine surfaces and desktop hardening. It must not be described as live
provider parity, live MCP parity, packaged release readiness, Go backend
readiness, or Reasonix/Kun full superiority.

## 31. 2026-06-21 Reasonix 9e56 Currentness And Internal Runtime Parity Addendum

This addendum updates section 30 for Reasonix
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b`.
The product boundary is unchanged: `window.analytix`, top-level `runtime`
settings, `analytix serve`, and Renderer -> preload -> main -> runtime HTTP/SSE
remain authoritative. No top-level Workflow/Create Loop, Reasonix public
protocol, Kun identity, Rust/Tauri path, or default Go backend is authorized.

Currentness classification:

| Reasonix item | Class | analytix ruling |
| --- | --- | --- |
| `dffb6973` todo clear reload | `record` | Structured analytix todos already persist empty lists as explicit clear state. Record the invariant; do not copy Reasonix transcript rebuild code. |
| `02b25d8b` bounded archived todo args | `record` | Analytix does not use the Reasonix frontend tool-args archive path for todos. Record as future regression guard only. |
| `7ba42955` remove lazy MCP startup | `wrap-behind-contract` | Keep analytix `McpServerConfig` and parallel enabled-server startup; do not import Reasonix tier/lazy config. |
| `e5a48d9c` retry all available MCP servers | `contract-reimplement` | Reimplement as analytix background reconnect over all failed startup servers. |
| `315c95b3` retry review feedback | `contract-reimplement` | Add registry-level source suspension so late MCP reconnect cannot reinstall disabled tools. |
| `027e7dc0` MCP merge | `record` | Merge-only record for the MCP currentness set. |

Implementation evidence:

| Surface | New requirement now met | Remaining non-parity gap |
| --- | --- | --- |
| MCP startup parity | `CapabilityRegistry.suspendToolSource` / `resumeToolSource`, late reconnect registration through `connectToolSource`, retry-all failed server test, suspended-source test, and known override cwd/priority/diagnostic tests. | Live MCP/indexer operation and Reasonix plugin protocol remain out of scope. |
| Task/background jobs | `DurableTaskJobManager.wait({ timeoutMs })`, file-backed serve runtime job manager, authenticated internal `/v1/runtime/task-jobs/wait|output|kill` routes, and route tests for auth/output/wait/kill/404. | First-class `task`, `parallel_tasks`, planner/executor Coordinator, restart/crash drill, and desktop nested-card QA remain open. |
| Provider/cache release guard | Existing provider-cache oracle remains offline/golden; provider probe/model-list diagnostics now redact URL/body/network-error secrets. | Credentialed live provider matrix, write-inline/scheduled/probe live evidence, and cost reconciliation remain blockers. |
| Kun shell safe-area | Native titlebar safe-area and shell navigation dirty baseline is reviewed and committed separately. | Local Whisper and tray session menu stay deferred behind future spec/QA gates. |
| Go G5 | This addendum updates TS-owned oracle requirements only. | No Go code changed; no full loop, job manager, cache, compaction, resume, interrupt, Electron integration, or default backend. |

Required wording:

```text
Reasonix 9e56 currentness is classified; MCP startup fixture parity and
internal task-job runtime routes are advanced. This is not Reasonix full
task/parallel/planner parity, live provider/MCP parity, Go G5 parity, or
release readiness.
```

## 32. 2026-06-21 Collaborative Execution, Cache Curve, MCP Indexer, And Go G5 Oracle Gate

This addendum advances the post-9e56 stage without changing the product
boundary. Dirty shell/native titlebar safe-area work must be closed and
committed before this gate may proceed.

New TypeScript-owned requirements:

| Surface | Requirement | Non-goal |
| --- | --- | --- |
| Collaborative execution | `task` and `parallel_tasks` are pinned only as analytix internal contracts/oracles: permission gate, parent Goal evidence keys, background field, transcript continue/fork, `depends_on` normalization, duplicate/self/unknown/cycle dependency rejection, wait/output/kill internal routes, and planner read-only toolset. | No top-level Subagent/Workflow/AutoResearch entry and no Reasonix public task protocol. |
| Provider/cache | Offline cache curve guard must model Reasonix release-cache behavior, including a 90% tail threshold and a bounded too-small-window allowed-low case. Multi-provider parsing remains required for DeepSeek, Responses, Anthropic, reasoning tokens, and unsupported fallback. | No live provider superiority claim without credentials and live write-inline/scheduled/probe/model-list evidence. |
| MCP/indexer | The lifecycle oracle must cover retry-all failed startup servers, late-provider suspension, and codegraph/codebase-memory cwd/low-priority/backgroundStart behavior while preserving explicit cwd. | No Reasonix plugin protocol and no MCP-indexer top-level UI. |
| Go G5 | `go-g5-full-loop-oracle.json` is the TS-owned G5 inventory for full loop, jobs, cache/compaction, resume/interrupt, and MCP indexer. | No Electron main integration, no `analytix serve` replacement, no renderer-visible Go route, and no default Go backend. |
| Kun desktop | Kun remains the v0.2.13 -> v0.2.14 product-entry baseline. Native titlebar/shell safe-area can be absorbed; Local Whisper/tray session menu stay behind future import-boundary and packaged QA gates. | No Kun identity or top-level Workflow/Create Loop expansion. |

Allowed wording:

```text
Analytix has advanced TS-owned collaborative-execution, cache-curve,
MCP/indexer lifecycle, and Go G5 oracle evidence behind existing contracts.
DeepSeek cache fixture evidence is at least Reasonix-style, and multi-provider
coverage is broader than Reasonix's DeepSeek-only guard.
```

Forbidden wording:

```text
Do not claim full Reasonix task/planner parity, live provider superiority,
live MCP/indexer parity, Local Whisper/tray parity, Go G5/full-loop parity,
release readiness, or default Go backend.
```

## 33. 2026-06-21 Executable Runtime, Cache Guard, MCP Live-Local, And Tray Gate

This addendum advances section 32 from oracle-only proof to a narrower
executable/internal-runtime gate. The public product boundary is unchanged:
`window.analytix`, top-level `runtime` settings, `analytix serve`, and Renderer
-> preload -> main -> runtime HTTP/SSE remain authoritative.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Collaborative execution | Internal `task` and `parallel_tasks` providers execute through `DelegationRuntime`, preserve the permission gate, support background jobs, prove dependency order, expose output offsets, and reconcile stale queued/running jobs on restart. | This is not Reasonix public task/planner protocol, not a top-level Subagent/Workflow route, and not full planner/executor Coordinator parity. |
| Provider/cache | `evaluateOfflineCacheCurveGuard` is a runtime-owned guard, and DeepSeek native cache hit/miss telemetry has precedence over conflicting generic cached-token fields. | This is not live provider/cache superiority without credentials, cost reconciliation, and write-inline/scheduled/probe/model-list live evidence. |
| MCP/indexer | Fake live-local indexer proof covers retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics. | This is not Reasonix plugin protocol and not live MCP/indexer parity. |
| Kun desktop | Tray session menu is absorbed as a main-process analytix-native surface over existing runtime thread APIs and existing window creation. | This is not a Kun bridge/identity import, not Local Whisper parity, and not a new top-level product route. |
| Go G5 | The local attempt is blocked because `go` is unavailable; no Go files changed. | No Go G5 implementation, no Electron integration, no renderer-visible Go route, no `analytix serve` replacement, and no default Go backend. |

Allowed wording:

```text
Analytix now has executable internal task/parallel runtime proof, a
runtime-owned offline cache curve guard, fake live-local MCP/indexer lifecycle
evidence, and an analytix-native tray session menu. DeepSeek cache fixtures are
at least Reasonix-style, and multi-provider cache coverage remains broader.
```

Forbidden wording:

```text
Do not claim full Reasonix planner/executor parity, live provider/cache
superiority, live MCP/indexer parity, Go G5 parity, packaged release readiness,
Local Whisper parity, Reasonix public protocol, Kun identity, or default Go
backend readiness.
```

## 34. 2026-06-21 Reasonix bfe398 Drift And Go G5 Shadow Output Gate

This addendum updates the currentness target from Reasonix `9e56c327` to
`bfe398cc`. The new Reasonix drift is limited to PowerShell 7 shell/sandbox
compatibility and shell-tool guidance. It is classified as record/future
contract-reimplementation input, not as a required code import for this G5
runtime slice.

Go G5 requirement now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 shadow output | `packages/runtime-go` now builds a G5 output from the TS-owned full-loop oracle fixture, including source oracle ids, full-loop tests, job routes/behaviors, cache/compaction tests, resume/interrupt tests, MCP/indexer tests, blockers, and product boundary flags. | This is not a live Go full-loop executor, not Electron main integration, not a renderer-visible Go route, not `analytix serve` replacement, and not default Go backend readiness. |
| Currentness | Reasonix bfe398 PowerShell compatibility drift is classified. | No Reasonix shell/sandbox code is copied in this stage. |

Allowed wording:

```text
Go G5 has a tested shadow output slice tied to TypeScript-owned oracle
fixtures. Reasonix bfe398 is currentness-classified.
```

Forbidden wording:

```text
Do not claim Go G5 runtime parity, default Go backend readiness, live provider
or MCP parity, release readiness, Reasonix public protocol, or imported
Reasonix shell/sandbox behavior.
```

## 35. 2026-06-21 Go G5 Composite Shadow Replay Gate

This addendum strengthens section 34. G5 shadow output must be built from the
same TypeScript-owned source fixtures that define runtime behavior, not only
from the G5 inventory fixture.

Requirement now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Jobs | Go composite replay reads task/parallel tool names, internal routes, dependency order, planner read-only tools, nested metadata, and transcript identity from the task-job oracle. | Not a live Go Job Manager. |
| Cache | Go composite replay reads stable prefix and release-guard status evidence from the provider-cache oracle. | Not live cache superiority. |
| Session/resume/SSE | Go composite replay reads G2 route ids, resume/fork route ids, and SSE route ids from the G2 route replay oracle. | Not live session runtime. |
| MCP/indexer | Go composite replay reads retry, tombstone, known override, live-local active paths, and redacted diagnostics from the MCP lifecycle oracle. | Not a live Go MCP client or Reasonix plugin protocol. |

Allowed wording:

```text
Go G5 has cross-fixture composite shadow replay for jobs, cache, session/resume,
SSE, and MCP/indexer.
```

Forbidden wording:

```text
Do not claim Go G5 runtime parity, default Go backend readiness, Electron
integration, live provider/cache superiority, live MCP/indexer parity, or
release readiness.
```

## 36. 2026-06-21 Planner Executor, Shell Compatibility, Live-Local Evidence, And G5 Runner Shadow Gate

This addendum advances section 35 from cross-fixture shadow replay into a
scoped executable/internal-runtime gate. The product boundary is unchanged:
`window.analytix`, top-level `runtime` settings, `analytix serve`, and Renderer
-> preload -> main -> runtime HTTP/SSE remain authoritative.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Planner/executor | An internal coordinator starts a read-only planner job, schedules executor jobs by DAG waves, records output offsets, preserves parent Goal/nested metadata, skips downstream tasks when a dependency fails, and kills active executor jobs on cancellation. | Not a Reasonix public task/planner protocol and not a top-level Workflow/Subagent/AutoResearch UI. |
| Durable runner rehydration | Queued/running jobs persisted across restart can be rehydrated when a runner is available, then still support output, wait, and kill. Missing runners fail jobs explicitly instead of leaving phantom active work. | Not a live desktop crash-drill claim and not stable-prefix/cache diagnostic state. |
| Windows shell compatibility | Runtime shell detection absorbs the bfe398 PowerShell value: standard pwsh/Windows PowerShell paths, pwsh `&&` / `||` support, and Windows PowerShell unquoted chaining guard with quoted text allowed. | No Reasonix shell/sandbox code copy and no Reasonix protocol change. |
| Provider/cache | DeepSeek cache telemetry is proved against an executable local provider server, including final request path, native hit/miss precedence over conflicting generic fields, and reasoning tokens. | Not credentialed live provider superiority, cost reconciliation, or live write-inline/scheduled/probe/model-list evidence. |
| MCP/indexer | Fake live-local indexer proof now uses an executable stdio MCP server with persistent state, restart/resume, tombstones, cwd, low-priority/backgroundStart, and redacted diagnostics. | Not Reasonix plugin protocol and not live MCP/indexer parity. |
| Go G5 | Composite shadow replay includes planner/executor and durable-runner restart fields from the TS-owned task-job oracle. | Not a live Go Job Manager, Go MCP client, Go cache/session runtime, Electron integration, or default Go backend. |
| Kun desktop | Kun remains the v0.2.13 -> v0.2.14 product-entry baseline; this runtime stage does not add new Kun UI surfaces. | Local Whisper and packaged tray/platform QA remain gated. |

Allowed wording:

```text
Analytix has scoped executable/internal-runtime evidence for planner/executor
failure/cancellation, durable job rehydration, Windows shell compatibility,
live-local provider cache telemetry, executable stdio MCP indexer lifecycle,
and Go G5 runner shadow replay.
```

Forbidden wording:

```text
Do not claim full Reasonix runtime parity, live provider/cache superiority,
live MCP/indexer parity, Go G5 runtime parity, release readiness, Local Whisper
parity, Reasonix public protocol, Kun identity, or default Go backend.
```

## 37. 2026-06-21 Reasonix 881 Control And Step-Limit Gate

This addendum advances section 36 for Reasonix `bfe398cc..881b2f2f` control
semantics. The fetched current `origin/main-v2` is `9ada1417`, so post-881
auto-plan drift is recorded but not imported.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Cancelled tool batches | Every model-accepted tool call receives a persisted paired result before the turn returns `aborted`: real output for completed calls, synthetic `tool_call_cancelled` output for cancelled/unstarted calls. | Not Reasonix public control protocol or TUI Esc/Ctrl+C UX. |
| Task-job cancellation | Parent abort signals kill running foreground/background jobs, preserve partial output/offsets, and return skipped unstarted `parallel_tasks` children. | Not Reasonix public task protocol or packaged crash-drill evidence. |
| Step limits | Runtime step limits are analytix-owned under top-level `runtime.runtimeTuning.stepLimits`; thread/session and per-turn overrides are supported; planner/headless/delegation budgets are represented. | Not Reasonix config root and not dynamic stable-prefix prompt state. |
| Go G5 control shadow | G5 `controlReplay` shadows cancel, task-job cancel aggregation, and step-limit controls. | Still no live Go full-loop executor, Electron integration, renderer-visible Go route, or default Go backend. |
| Kun desktop | Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/Connect Phone/Schedule. | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route and no Kun identity. |

Allowed wording:

```text
Analytix has scoped Reasonix 881 control parity for cancelled batch result
preservation and analytix-owned runtime step-limit controls, with Go G5
control shadow evidence.
```

Forbidden wording:

```text
Do not claim full Reasonix parity, post-881 auto-plan parity, live provider or
MCP superiority, Go G5 runtime parity, release readiness, Reasonix public
protocol, Kun identity, default Go backend, or Rust/Tauri migration.
```

## 38. 2026-06-21 Post-881 Auto-Router Guard, G5 Executable Control, And Kun Context Window

This addendum advances section 37 from `881b2f2f` to current Reasonix
`9ada14176629b1d59d7ed78446951b2bb5954904` without importing Reasonix public
auto-plan configuration.

Currentness classification:

| Upstream delta | Classification | analytix result |
| --- | --- | --- |
| `01d9b173` user-level auto-plan | `document-only` / `defer` | No public auto-plan setting is added. Any future setting must live under top-level `runtime` and analytix contracts. |
| `01d9b173` project/local auto-plan rejection | `reject` | No `reasonix config auto-plan --local`, project auto-plan override, or Reasonix config root. |
| `2db7acf6` classifier rebuild on enable | `contract-reimplement` | Auto-model router cache keys include a classifier contract fingerprint, so classifier prompt/model/timeout drift cannot reuse stale same-turn routing. |
| `9ada1417` merge | `record-only` | Merge context only. |
| Kun 0.2.14 unknown-model context window | `code-port-and-adapt` | Shared provider defaults now use `128_000`, matching runtime defaults, while explicit profile values still override. |
| Go G5 control replay | `shadow-only executable proof` | Go consumes TS-owned `controlExecutableCases` and computes cancel/task-job/step-limit output, but remains non-default. |

Allowed wording:

```text
Analytix has a scoped post-881 classifier-lifecycle guard through its
auto-model-router fingerprint, plus Go G5 executable control shadow evidence
and Kun 0.2.14 context-window default parity.
```

Forbidden wording:

```text
Do not claim Reasonix post-881 auto-plan parity, Reasonix config/CLI protocol,
live provider/cache superiority, live MCP/indexer parity, Go G5 runtime parity,
default Go backend readiness, release readiness, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer entries, Kun identity, or Rust/Tauri
migration.
```

## 39. 2026-06-21 Approval/User-Input Abort Cleanup Gate

This addendum advances the approval/control portion of the Reasonix
agent-kernel absorption order without importing Reasonix SessionAPI,
controller APIs, or frontend ask protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Approval abort cleanup | Pending approval waits expire when the owning turn aborts; the waiter rejects, the gate is no longer pending, and `approval_resolved: expired` is replayable. | Not Reasonix approvalManager public protocol and not TUI/CLI approval UX. |
| Late approval decision | A late HTTP allow/deny after cleanup returns conflict and cannot execute the original tool call. | Not a new renderer route or Reasonix SessionAPI action. |
| User-input abort cleanup | `request_user_input` abort records `user_input_resolved: cancelled` after the pending gate is cleared. | Not a new user-input protocol or background workflow surface. |
| Renderer live status | `approval_resolved: expired` maps to the existing approval block error state, avoiding stale pending cards during live SSE. | Full desktop click-through QA remains separate. |
| Route oracle | `approval-user-input-route-oracle.json` pins late approval to 409, late user-input resolve to 404, and replay kinds for cleanup. | Live renderer approval-card QA remains separate. |
| Tool history | Existing cancelled tool-result pairing remains the model-history repair surface; abort is not represented as user denial. | No Reasonix public task/control protocol. |

Allowed wording:

```text
Analytix now has fixture-backed approval/user-input abort cleanup: pending GUI
gates close on turn interrupt, replay emits resolved states, and late GUI
actions cannot revive cancelled work.
```

Forbidden wording:

```text
Do not claim full Reasonix approval-manager parity, Reasonix SessionAPI,
renderer-visible Reasonix protocol, live provider/MCP parity, Go runtime
parity, release readiness, default Go backend, or top-level Workflow/Subagent
navigation from this batch.
```

## 40. 2026-06-21 Renderer Approval Live-Card Store Gate

This addendum extends section 39 with renderer store evidence. It does not add
new runtime contracts, Go behavior, provider behavior, or UI navigation.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Main-thread approval card | `buildThreadEventSink` updates an existing approval block when `onApprovalStatus` arrives. | Not a new approval protocol or Reasonix frontend ask card. |
| Side conversation approval card | Side SSE sinks update only the side conversation block list and leave main thread blocks untouched. | Not full nested-card visual QA or packaged desktop QA. |
| Duplicate prevention | The live-card path proves status updates mutate the existing approval block rather than adding a duplicate block. | Crash/restart renderer replay QA remains separate. |
| Product boundary | No bridge, settings, route, provider, Go, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live provider/MCP claims remain blocked. |

Allowed wording:

```text
Analytix now has renderer store-level proof that live approval resolution closes
existing main-thread and side approval cards.
```

Forbidden wording:

```text
Do not claim full Reasonix frontend approval parity, packaged desktop QA,
release readiness, default Go backend, Reasonix public protocol, Kun identity,
or new Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 41. 2026-06-21 User-Input Live-Card Store Gate

This addendum extends section 40 from approval cards to user-input cards. It
keeps the same analytix-owned renderer store boundary and adds no runtime route,
bridge, settings, Go, provider, or navigation surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Main-thread user-input card | `buildThreadEventSink` can close an existing user-input block when `onUserInputStatus` arrives with either the item id or input/request id. | Not a new user-input protocol or Reasonix ask card. |
| Side conversation user-input card | Side user-input blocks use the runtime item id and status updates match by item id or request id. | Not full visual nested-card QA or packaged desktop QA. |
| Side scope safety | Side user-input status updates mutate only side blocks and leave main thread blocks untouched. | Crash/restart renderer replay QA remains separate. |
| Product boundary | No bridge, settings, route, provider, Go, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live provider/MCP claims remain blocked. |

Allowed wording:

```text
Analytix now has renderer store-level proof that live user-input resolution
closes existing main-thread and side user-input cards.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input frontend parity, packaged desktop QA,
release readiness, default Go backend, Reasonix public protocol, Kun identity,
or new Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 42. 2026-06-21 Planner Gating For Internal Task Tools

This addendum tightens the task/background/planner oracle. Internal
`task` / `parallel_tasks` tools may exist behind analytix runtime contracts,
but Plan mode must not advertise or execute them.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Plan-mode tool advertisement | Even when `task` and `parallel_tasks` are registered, Plan mode advertises only read-only tools plus `create_plan`. | Not a Reasonix public planner/task protocol. |
| Forged task-call rejection | A forged `task` tool call in Plan mode is rejected by active tool policy and does not execute child work. | Not full planner/executor Coordinator parity. |
| Runtime oracle | `task-job-orchestration-oracle.json` records `plannerForbiddenToolset` for future cross-backend gates. | Go remains shadow-only and non-default. |
| Product boundary | No bridge, settings, provider, Go default, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live provider/MCP claims remain blocked. |

Allowed wording:

```text
Analytix has fixture-backed planner gating that keeps internal task tools out
of Plan mode and rejects forged child-job calls.
```

Forbidden wording:

```text
Do not claim full Reasonix planner/executor parity, public task protocol,
release readiness, default Go backend, Reasonix public protocol, Kun identity,
or new Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 43. 2026-06-21 Go G5 Planner-Forbidden Shadow Replay

This addendum extends section 42 into Go G5 shadow evidence. The TypeScript
task-job oracle remains authoritative; Go only consumes and replays its
planner-forbidden toolset through `shadowSlicesExpectedOutput`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Go G5 job replay | `BuildG5ShadowSlicesOutput` includes `plannerForbiddenToolset` alongside the read-only planner toolset. | Not a live Go planner/executor. |
| TS binding | The Go conformance test derives expected `plannerForbiddenToolset` from `task-job-orchestration-oracle.json`. | No independent Go-owned protocol or fixture drift. |
| Product boundary | No bridge, settings, provider, Electron route, default Go backend, Rust/Tauri, or top-level navigation surface changed. | G6 remains blocked by live runtime, rollback, and packaging evidence. |

Allowed wording:

```text
Go G5 shadow now replays the planner-forbidden task-tool gate from the
TS-owned task-job oracle.
```

Forbidden wording:

```text
Do not claim Go runtime parity, full Reasonix planner/executor parity, default
Go backend readiness, live provider/cache superiority, live MCP parity, release
readiness, Reasonix public protocol, Kun identity, or new Workflow/Subagent/
AutoResearch/MCP-indexer navigation from this batch.
```

## 47. 2026-06-21 Write-Inline Custom Full Endpoint Proof

This addendum extends section 44. Custom full endpoint paths are explicit
provider configuration, so write-inline must not append another endpoint path
when a user-provided URL already ends in `/responses` or `/messages`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Custom `/responses` full endpoint | Write-inline POSTs to the exact custom URL, uses Responses `input` / `max_output_tokens`, and omits Anthropic headers. | Not live OpenAI credential evidence. |
| Custom `/messages` full endpoint | Write-inline POSTs to the exact custom URL, uses Messages `system` / `messages` / `max_tokens`, sends Anthropic-style headers, and parses Messages responses. | Not live Anthropic credential evidence. |
| Product boundary | No runtime route, bridge, settings schema, provider default, Go backend, Rust/Tauri, or top-level navigation surface changed. | Live provider/cache superiority and release readiness remain blocked. |

Allowed wording:

```text
Analytix has write-inline regression proof that custom full endpoint URLs for
Responses and Messages keep their exact URL and matching request body shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
completion, full Reasonix provider parity, default Go backend readiness,
release readiness, Reasonix public protocol, Kun identity, or new
Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 48. 2026-06-21 Auto-Model Route Cache Lifecycle Proof

This addendum extends the post-881 auto-plan currentness work without adding a
public auto-plan setting. The accepted value is classifier route-cache
currentness inside analytix `AgentLoop`, not Reasonix config/controller API.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Same-turn auto routing | A multi-step `model:"auto"` turn calls `_auto_router` once, then reuses the selected model/reasoning route on later steps. | Not Reasonix auto-plan. |
| Cross-turn currentness | A following turn in the same thread runs `_auto_router` again instead of reusing the previous turn route. | Not a public classifier cache API. |
| Stable prefix safety | The proof uses runtime route-cache behavior and does not add classifier state to the immutable system prefix. | Live cache/cost superiority remains separate. |
| Product boundary | No settings schema, controller API, Reasonix config root, provider default, Go backend, Rust/Tauri, or top-level navigation surface changed. | Full Reasonix auto-plan parity and release readiness remain blocked. |

Allowed wording:

```text
Analytix has loop-level proof that auto model routing is reused within a
multi-step turn but recalculated for the next turn.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan parity, user-level auto-plan settings,
project/local auto-plan overrides, Reasonix controller APIs, default Go backend
readiness, release readiness, Reasonix public protocol, Kun identity, or new
Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 49. 2026-06-21 Provider Request-Shape Oracle Matrix

This addendum extends the provider/cache oracle. Request URL/header/body shape
is part of provider/cache non-regression evidence because cache accounting is
only useful if provider-specific request contracts stay stable.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| DeepSeek official chat | The oracle verifies `/v1/chat/completions`, OpenAI-style headers, OpenAI function tools, and DeepSeek-only thinking/reasoning fields. | Not live credential evidence. |
| OpenAI-compatible chat | The oracle verifies OpenAI-compatible chat body/header shape and explicitly forbids DeepSeek-only `thinking`. | Not live provider matrix completion. |
| Responses | The oracle verifies `/responses`, `input`, `max_output_tokens`, Responses function tools, and no Anthropic headers. | Not live OpenAI credential evidence. |
| Anthropic Messages | The oracle verifies `/messages`, `system` / `messages` / `max_tokens`, Anthropic headers, and `input_schema` tools. | Not live Anthropic credential evidence. |
| Custom full endpoint | The oracle verifies a custom `/responses` URL is used exactly and follows Responses body shape. | Not broad custom provider certification. |
| Go shadow | G3/G5 shadow outputs now replay request-shape case ids only. | No Go provider client or default backend. |

Allowed wording:

```text
Analytix has fixture-backed provider request-shape oracle coverage for
DeepSeek, OpenAI-compatible, Responses, Anthropic Messages, and custom
Responses full endpoints.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
completion, Go provider parity, default Go backend readiness, release
readiness, Reasonix public protocol, Kun identity, or new Workflow/Subagent/
AutoResearch/MCP-indexer navigation from this batch.
```

## 46. 2026-06-21 Structured User-Input Choice Validation

This addendum tightens the G4 tools/approval/user-input oracle. Structured GUI
input options are an analytix-owned runtime tool contract, not a Reasonix ask
protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Structured `request_user_input` | Malformed choice requests return `invalid_user_input_request` before a GUI gate is opened. | Not a Reasonix SessionAPI or ask protocol. |
| Choice limits | Structured choices allow at most three questions, and two to three options when options are provided. | Free-form input remains available without options. |
| Duplicate labels | Labels are trimmed and compared case-insensitively before opening the gate. | Not desktop visual QA. |
| Go G4 shadow | The G4 oracle and Go shadow output record invalid cases, stable error code, and `opensGateOnInvalid:false`. | Go remains shadow-only and non-default. |
| Product boundary | No bridge, settings, provider, renderer route, default Go backend, Rust/Tauri, or top-level navigation surface changed. | Release readiness and live provider/MCP claims remain blocked. |

Allowed wording:

```text
Analytix rejects malformed structured GUI input choices before opening a
user-input gate, with G4 shadow evidence for the error contract.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input parity, desktop live QA, release
readiness, default Go backend, Reasonix public protocol, Kun identity, or new
Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 44. 2026-06-21 Write-Inline Provider Request-Surface Proof

This addendum tightens the provider/cache non-regression matrix for the desktop
write-inline path. It does not change provider behavior; it pins existing URL,
body, header, and parser behavior for `responses` and `messages` endpoint
formats.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| OpenAI Responses write-inline | The write-inline request uses `/v1/responses`, `input`, and `max_output_tokens`, and does not send Anthropic headers. | Not live OpenAI credential evidence. |
| Anthropic Messages write-inline | The write-inline request uses `/v1/messages`, `system` / `messages` / `max_tokens`, Anthropic-style headers, and the Anthropic response parser. | Not live Anthropic credential evidence. |
| Product boundary | No runtime route, bridge, settings, provider defaults, Go backend, Rust/Tauri, or top-level navigation surface changed. | Live provider/cache superiority and release readiness remain blocked. |

Allowed wording:

```text
Analytix has write-inline regression proof for OpenAI Responses and Anthropic
Messages request surfaces.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
completion, full Reasonix provider parity, default Go backend readiness,
release readiness, Reasonix public protocol, Kun identity, or new
Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 45. 2026-06-21 Go G5 Planner Tool-Policy Executable Shadow

This addendum extends section 43. The Go G5 control executable fixture now
includes planner tool-policy gating in addition to cancel, task-job, and
step-limit cases.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Planner step 0 advertised tools | Go computes read-only tool names plus `create_plan` from fixture input. | Not a live Go planner turn. |
| Planner step 1 advertised tools | Go computes `create_plan` only after the investigation step. | Not a Go agent loop or model call. |
| Forged task rejection | Go computes a forged `task` call as `failed` / `tool_dispatch_rejected` / `executed:false`. | Not a Go tool host or Job Manager. |
| Product boundary | No bridge, settings, provider, Electron route, default Go backend, Rust/Tauri, or top-level navigation surface changed. | G6 remains blocked by live runtime, rollback, and packaging evidence. |

Allowed wording:

```text
Go G5 executable shadow now covers planner tool-policy gating for read-only
tools, `create_plan`, and forged internal task rejection.
```

Forbidden wording:

```text
Do not claim Go runtime parity, full Reasonix planner/executor parity, default
Go backend readiness, live provider/cache superiority, live MCP parity, release
readiness, Reasonix public protocol, Kun identity, or new Workflow/Subagent/
AutoResearch/MCP-indexer navigation from this batch.
```

## 50. 2026-06-21 Kun Top-Level Route-Surface Oracle

This addendum turns the product-entry boundary into a renderer test contract.
It does not add or remove runtime capability; it proves future upstream
absorption cannot accidentally expose Kun-target-absent or Reasonix-native
orchestration as top-level analytix navigation.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| App routes | `AppRoute` remains limited to `chat`, `write`, `settings`, `plugins`, `claw`, and `schedule`. | Not a new navigation system. |
| App actions | `createAppActions` does not expose Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer open actions. | Internal runtime tools may still exist. |
| Workbench stage | Workbench top-level stage rendering only exposes existing product surfaces and does not import `WorkflowCreateLoopView`. | Internal Create Loop code remains hidden. |
| Shell/sidebar | Shell navigation and sidebar active views cannot add forbidden top-level route tokens or entrypoint symbols without failing tests. | Not desktop packaged QA. |
| Product boundary | Kun target-version entry baseline is executable evidence, not only a static scan. | Future route promotion requires a new spec/conflict decision. |

Allowed wording:

```text
Analytix has executable route-surface proof that Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer are not top-level entries for the Kun
0.2.13 -> 0.2.14 target baseline.
```

Forbidden wording:

```text
Do not claim a new product surface, full Kun current parity, Reasonix public
protocol, default Go backend readiness, Rust/Tauri migration, or release
readiness from this route-surface oracle.
```

## 51. 2026-06-21 Auto-Route Step/Cancel Control Composition

This addendum turns separate post-881 control proofs into a composed runtime
and G5 shadow requirement. It does not add a new public setting, Go backend, or
Reasonix protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime auto-route + step limit | A real `AgentLoop` turn with `model:"auto"` routes once, uses the route for two model steps, then fails at the user-global step limit. | Not a Reasonix auto-plan setting. |
| Stable prefix | The same test proves step-limit/control state is absent from system prompt, prefix, and context instructions. | Not a live provider/cache claim. |
| Cancel composition | G5 executable fixture computes completed-result preservation, `tool_call_cancelled`, and unstarted `aborted` status alongside cache/step invariants. | Not a live Go AgentLoop. |
| Go G5 | Go shadow computes the combined case from TS-owned fixtures. | No default backend, Electron route, provider/session/MCP runtime, or G6 rollback path. |

Allowed wording:

```text
Analytix has runtime and G5 shadow evidence that auto-route cache, step-limit,
and cancel-result preservation compose without stable-prefix drift.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan parity, live Go runtime parity, default Go
backend readiness, live provider/cache superiority, live MCP parity, release
readiness, Reasonix public protocol, Kun identity, or new Workflow/Subagent/
AutoResearch/MCP-indexer navigation from this batch.
```

## 52. 2026-06-21 Preload Bridge/API Sovereignty Oracle

This addendum turns the public renderer bridge boundary into a source-level
test requirement. It does not add a new bridge, protocol, settings fallback, Go
backend, or renderer route.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Preload bridge exposure | `contextBridge.exposeInMainWorld(...)` may expose only `analytix`. | Not a new public API surface. |
| Renderer global type | `interface Window` may declare only the `analytix` bridge. | Does not remove internal legacy import IPC. |
| Shared public API names | `AnalytixApi` remains the public facade; exported Kun/Reasonix/DeepSeek/deprecated GUI API names are rejected. | Does not claim full Reasonix frontend protocol parity. |
| Product boundary | No deprecated bridge/settings fallback, provider default, default Go backend, Rust/Tauri path, Kun identity, or top-level navigation surface changed. | Packaged desktop QA and release readiness remain blocked. |

Allowed wording:

```text
Analytix has executable preload/API evidence that the renderer public bridge is
`window.analytix` only.
```

Forbidden wording:

```text
Do not claim a new bridge, Reasonix SessionAPI/frontend protocol parity, Kun
public protocol, old settings fallback, live Go backend readiness, release
readiness, or new Workflow/Subagent/AutoResearch/MCP-indexer navigation from
this batch.
```

## 53. 2026-06-21 Renderer Thread Lifecycle HTTP Oracle

This addendum strengthens the renderer-side runtime contract for thread
lifecycle actions. It does not add a new thread surface, bridge, protocol, or
backend.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Thread list/search | Renderer `listThreads` options produce the analytix `/v1/threads` query for limit, search, include archived, and archived-only filters. | Not packaged desktop sidebar QA. |
| Thread lifecycle mutations | Archive, restore, rename, workspace update, and delete use `/v1/threads/:id` with `PATCH`/`DELETE` and analytix JSON bodies. | Does not implement a new runtime route. |
| Adjacent lifecycle coverage | Existing renderer tests cover fork, resume-thread, and SSE subscription through the same bridge/runtime adapter. | Go live route handling remains separate. |
| Product boundary | No Reasonix SessionAPI, Kun protocol, deprecated bridge/settings fallback, provider default, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has renderer adapter evidence that thread lifecycle actions stay on
the analytix `/v1/threads` HTTP contract.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, Reasonix SessionAPI/thread protocol parity,
Kun public protocol, live Go route readiness, release readiness, or new
Workflow/Subagent/AutoResearch/MCP-indexer navigation from this batch.
```

## 54. 2026-06-21 Settings/Provider EndpointFormat Persistence Oracle

This addendum pins the settings storage side of provider request-shape safety.
It does not change provider behavior, defaults, or live runtime routing.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime endpoint format | Top-level `runtime.endpointFormat` persists to disk and survives reload. | Not live provider credential evidence. |
| Custom provider endpoint format | Custom provider `endpointFormat` persists under `provider.providers`, including explicit custom full endpoint URLs. | Does not change provider defaults. |
| Legacy envelope exclusion | Saves do not write top-level `agentProvider` or `agents` envelopes. | Legacy import/migration compatibility remains separate. |
| Product boundary | No Reasonix config root, Kun settings identity, deprecated bridge/settings fallback, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has settings-store evidence that endpoint-format choices persist under
top-level `runtime` and `provider.providers`.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
completion, Go provider readiness, Reasonix config parity, Kun settings
identity, release readiness, or new Workflow/Subagent/AutoResearch/MCP-indexer
navigation from this batch.
```

## 55. 2026-06-21 Runtime Provider-Selection Request-Shape Oracle

This addendum pins the runtime dispatch side of provider request-shape safety.
It does not change provider defaults, settings schema, public protocol, or live
backend routing.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Thread-selected provider | A thread `providerId` selects the configured provider inside `MultiProviderModelClient`. | Not a Reasonix provider protocol. |
| Custom full endpoint shape | `endpointFormat: "custom_endpoint"` with a full `/messages` URL is used exactly and produces Messages headers/body fields. | Not live credentialed provider evidence. |
| Missing provider fallback | Missing/removed thread providers fall back to the default OpenAI-compatible `/chat/completions` request path. | Does not change provider defaults. |
| Diagnostics | Runtime diagnostics report provider id, provider base URL, endpoint format, and configured model through analytix-owned fields. | Not a public Reasonix SessionAPI. |
| Product boundary | No Reasonix config root, Kun settings identity, deprecated bridge/settings fallback, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has runtime fake-fetch evidence that thread provider selection drives
the expected custom full endpoint and default fallback request shapes.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
completion, Go provider readiness, Reasonix provider protocol parity, Kun
settings identity, release readiness, or new Workflow/Subagent/AutoResearch/
MCP-indexer navigation from this batch.
```

## 56. 2026-06-21 Scheduled Detector Custom Endpoint Inference Oracle

This addendum pins another provider consumer path: scheduled reminder detection
must honor custom full endpoint mode and infer the endpoint family locally.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Custom `/messages` endpoint | Full `/messages` URLs are used exactly, with Messages headers/body shape and no Responses `input`. | Not live credentialed provider evidence. |
| Custom `/chat/completions` endpoint | Full `/chat/completions` URLs are used exactly, with chat messages body and JSON response format. | Does not change provider defaults. |
| Parser family | Scheduled detector parsing follows the inferred endpoint family. | Not a public Reasonix provider protocol. |
| Product boundary | No Reasonix config root, Kun settings identity, deprecated bridge/settings fallback, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has scheduled detector fake-fetch evidence for custom `/messages` and
`/chat/completions` endpoint inference.
```

Forbidden wording:

```text
Do not claim live scheduled-task provider parity, credentialed provider matrix
completion, Go provider readiness, Reasonix provider protocol parity, Kun
settings identity, release readiness, or new Workflow/Subagent/AutoResearch/
MCP-indexer navigation from this batch.
```

## 57. 2026-06-21 Forbidden Public Runtime Route Oracle

This addendum pins runtime HTTP route sovereignty. Forbidden upstream public
routes must remain absent even when internal capabilities exist behind
analytix-owned contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Reasonix route rejection | `/v1/reasonix/sessions` and `/v1/reasonix/threads/:id` return structured 404s. | Not Reasonix SessionAPI parity. |
| Go route gating | `/v1/runtime/go` and `/v1/runtime/go/health` return structured 404s. | Go remains shadow-only until G5/G6. |
| Hidden capability route rejection | `/v1/workflow`, `/v1/workflows`, `/v1/create-loop`, `/v1/subagents`, `/v1/autoresearch`, and `/v1/mcp-indexer` return structured 404s. | Internal task/job/MCP/research code is not a public route entitlement. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has runtime HTTP negative-route evidence that forbidden upstream
public routes stay absent.
```

Forbidden wording:

```text
Do not claim Reasonix public protocol parity, Kun public protocol parity, live
Go backend readiness, default Go backend, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 58. 2026-06-21 Rehydrated Task-Job Route Oracle

This addendum pins restart continuity for internal task/sub-agent jobs at the
runtime HTTP boundary. Rehydrated jobs must remain operable through authenticated
analytix routes without exposing upstream public protocols.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Output after rehydrate | `/v1/runtime/task-jobs/output` returns combined pre/post-restart output for a rehydrated running job. | Not packaged desktop restart QA. |
| Wait after rehydrate | `/v1/runtime/task-jobs/wait` observes the rehydrated running job's completion status. | Not Reasonix public job protocol. |
| Kill after rehydrate | `/v1/runtime/task-jobs/kill` cancels a rehydrated queued job and wait returns killed status/error. | Not a top-level Subagent route. |
| Product boundary | Routes remain authenticated internal runtime routes; no Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has runtime HTTP harness evidence that rehydrated task jobs remain
operable through authenticated internal routes.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow routes, live Go backend readiness, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 59. 2026-06-21 Task-Job Approval Deny No-Execute Oracle

This addendum pins approval-before-side-effect ordering for internal task/job
providers. Denied approvals must not create durable task jobs or child runs.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| `task` denial | Denied `task` execution returns an approval item and creates no durable job. | Not packaged desktop approval-card QA. |
| `parallel_tasks` denial | Denied `parallel_tasks` execution returns an approval item and creates no durable jobs. | Not Reasonix public job protocol. |
| Child-run safety | Denied task tools leave `DelegationRuntime` child runs empty. | Not a top-level Subagent route. |
| Product boundary | Behavior stays inside the existing analytix permission gate; no Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has runtime tool-host evidence that denied task-job approvals create
no durable jobs or child runs.
```

Forbidden wording:

```text
Do not claim packaged desktop approval QA, Reasonix public sub-agent/job
protocol parity, top-level Subagent or Workflow routes, live Go backend
readiness, default Go backend, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 60. 2026-06-21 Go G5 Task-Job Approval Deny Shadow Replay

This addendum pins the Go G5 shadow requirement for task-job approval ordering.
The TypeScript runtime remains authoritative; Go may only replay the fixture
until G5/G6 gates are satisfied.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source oracle | `task-job-orchestration-oracle.json` records denied `task` / `parallel_tasks` tool names, approval ids, and no-side-effect booleans. | Not live Go approval behavior. |
| G5 job replay | `go-g5-full-loop-oracle.json` carries `jobReplay.approvalDenyNoExecute` from the TS task-job oracle. | Not Go Job Manager readiness. |
| Go shadow output | `BuildG5ShadowSlicesOutput` emits the approval deny no-execute fields from the source fixture. | Not a renderer-visible Go route or backend. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix Go G5 shadow replay now consumes TS-owned approval deny no-execute
evidence for internal task jobs.
```

Forbidden wording:

```text
Do not claim Go approval manager readiness, live Go task/job execution, Go G5
runtime parity, default Go backend, Reasonix public sub-agent/job protocol,
packaged desktop approval QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 77. 2026-06-22 D-0063 Go G5 Provider Cache Release Guard Executable Shadow

This addendum pins provider cache release guard shadow requirements. The guard
is offline fixture evidence only; it must not become a live provider superiority
claim or a default Go backend signal.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 cache replay schema | `cacheReplay.releaseGuard` records fixture-only status, low-tail count, threshold/window metadata, and per-case tail/collapse output. | Not live provider telemetry. |
| TS source of truth | `go-runtime-conformance.test.ts` derives expected output from `evaluateOfflineCacheCurveGuard`. | Not Reasonix provider protocol. |
| Go shadow calculation | `shadow_g5.go` computes tail averages, collapse counts, allowed-low status, and overall pass/fail from `provider-cache-oracle.json`. | Not Go provider client readiness. |
| Product boundary | No Reasonix provider protocol, live provider superiority claim, renderer-visible Go route, default Go backend, Kun identity, or top-level route changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix Go G5 executable shadow computes the offline provider cache release
guard from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix,
Go provider client readiness, default Go backend, Reasonix provider protocol
parity, packaged provider QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 78. 2026-06-22 D-0064 Auto-Route Step/Cancel AgentLoop Composition Proof

This addendum pins the AgentLoop composition requirement for auto-route cache,
step-limit metadata, and cancellation. It is a TypeScript runtime proof, not a
Reasonix auto-plan product surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Auto-route cache | A real `model:"auto"` turn runs `_auto_router` once and reuses the selected model/reasoning on the next model step. | Not Reasonix controller rebuild parity. |
| Step-limit hygiene | Tool context receives `runtimeStepLimits.currentMaxModelSteps`, while model-visible prefix/context omit step-limit text. | Not a stable-prefix state channel. |
| Cancel/result stability | Interrupted parallel tool batches preserve completed results and record cancelled results for remaining calls. | Not packaged desktop interruption QA. |
| Product boundary | No Reasonix auto-plan setting, public controller protocol, default Go backend, Kun identity, or top-level route changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix AgentLoop has focused test evidence that auto-route cache, step-limit
metadata, and cancelled parallel tool results compose safely.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, planner enable/disable product
controls, Reasonix controller/SessionAPI protocol parity, default Go backend,
packaged desktop interruption QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 79. 2026-06-22 D-0065 Planner Step-Limit AgentLoop Proof

This addendum pins the AgentLoop requirement for planner-specific step limits.
It is a TypeScript runtime proof, not a Reasonix planner product toggle or
public controller protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Planner step gating | A real plan-mode turn uses `plannerMaxModelSteps: 2` despite higher default/user-global limits and records `turn_step_limit_exceeded`. | Not Reasonix planner enable/disable parity. |
| Plan-mode tool surface | The same requests advertise the existing analytix `create_plan` tool. | Not a new top-level Workflow/Create Loop route. |
| Prompt/cache hygiene | Planner budget state stays out of stable prefix and model-visible context. | Not a stable-prefix state channel. |
| Product boundary | No Reasonix planner setting, public controller protocol, default Go backend, Kun identity, or top-level route changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix AgentLoop has focused test evidence that planner-specific step limits
gate plan-mode turns without entering model-visible prompt content.
```

Forbidden wording:

```text
Do not claim Reasonix planner enable/disable product controls, Reasonix
controller/SessionAPI protocol parity, Go planner backend readiness, default Go
backend, packaged desktop plan-mode QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 71. 2026-06-22 Go G5 User-Input Gate Executable Shadow

This addendum advances the G5 shadow gate for approval/user-input lifecycle
without creating a live Go route or exposing Reasonix ask/session protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source fixture binding | G5 shadow slices now include `approval-user-input-route-api-gates-v1` as a source fixture. | Not a Reasonix public ask protocol. |
| Submitted input | `controlExecutableCases.userInput.submitted` records submitted answers, pending counts, and `user_input_resolved` answer omission. | Answers remain out of SSE replay. |
| Cancelled input | The executable case records cancelled status, `secondResolveStatus:404`, and no pending gate after cancellation. | Not live Go user-input routing. |
| Go calculation | `replayG5UserInput` and `buildG5ApprovalUserInputReplay` compute the expected shadow output from TS-owned fixtures. | TypeScript runtime remains authoritative. |
| Product boundary | No bridge/settings/provider/default-backend/renderer route changes. | G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go G5 shadow computes submitted and cancelled user-input gate outcomes
from TS-owned route fixtures while keeping submitted answers out of replay.
```

Forbidden wording:

```text
Do not claim live Go user-input manager readiness, Go runtime parity, default Go
backend, Reasonix ask/session protocol parity, renderer-visible Go routes,
packaged desktop live-card QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 72. 2026-06-22 / 2026-06-23 Go AutoResearch Project-Local State

This addendum now covers the D-0246 Go runtime contract implementation for
long-running research state while keeping AutoResearch inside existing
goal/chat contracts and away from top-level navigation.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Project-local state | `packages/runtime-go/internal/research/autoresearch_state.go` writes `.analytix/autoresearch/<threadId>/` and the five required state files. | Not a top-level AutoResearch route. |
| Evidence audit | Go tests reject unknown requirement evidence without writing findings, and runtime-server tests replay `autoresearch_state_audit` after restart. | Not a default Go backend claim. |
| Prefix isolation | `autoresearch_state_audit` records `stablePrefixContainsState:false` and `toolSchemaContainsState:false`; product scan locks those tokens. | Not a cache superiority claim. |
| Go runtime contract | `/v1/threads/:id/turns` emits audit only for `/goal --research`; `/v1/threads/:id/events` replays it under the existing SSE contract. | No `/v1/autoresearch` public route. |
| Product boundary | No bridge/settings/provider/default-backend/renderer route changes. | D-0243/G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go runtime now has a project-local AutoResearch state contract for
/goal --research turns, with audit-only SSE replay and no top-level
AutoResearch route.
```

Forbidden wording:

```text
Do not claim default Go backend, full live Go runtime parity, Reasonix
project/AutoResearch protocol parity, renderer-visible Go routes, top-level
AutoResearch navigation, packaged desktop QA, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 73. 2026-06-22 Go G5 MCP Lifecycle Executable Shadow

This addendum advances G5 shadow evidence for MCP lifecycle/indexer behavior
while keeping MCP-indexer hidden behind analytix-owned runtime/tool contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Retry-all startup | `controlExecutableCases.mcpLifecycle` records failed server ids, expected connected/error servers, attempts per failed server, and no runtime restart requirement. | Not live credentialed MCP QA. |
| Tombstone/resume | G5 fixture records live-local initial paths, late tombstone, and resume paths; Go shadow computes active paths and tombstone count. | Not a real Go MCP client. |
| Diagnostics privacy | Go shadow computes `Authorization=<redacted>` and `leaksSecret:false`. | Not a public MCP protocol. |
| TS binding | `go-runtime-conformance.test.ts` derives the case from `mcp-tool-lifecycle-oracle.json` and `runLiveLocalIndexerProof`. | TypeScript runtime remains authoritative. |
| Product boundary | No MCP-indexer top-level route, bridge/settings/provider/default-backend, or renderer route changes. | G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go G5 shadow computes MCP retry, tombstone/resume, and redaction
boundaries from TS-owned MCP lifecycle fixtures.
```

Forbidden wording:

```text
Do not claim live credentialed MCP parity, Go MCP client readiness, Go runtime
parity, default Go backend, Reasonix MCP protocol parity, renderer-visible Go
routes, top-level MCP-indexer navigation, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 74. 2026-06-22 Go G5 Checkpoint/Rewind Executable Shadow

This addendum advances G5 shadow evidence for checkpoint/rewind safety while
keeping real checkpoint planning and apply behavior in the TypeScript runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Analytix-owned ids | `controlExecutableCases.checkpointRewind` records `axcp_`, `axrp_`, `axra_`, and `axrr_` ids. | Not Reasonix or Kun checkpoint protocol. |
| Path safety | TS conformance derives ready/blocked files from `buildAuditableCheckpointRewindPlan`; Go shadow computes path escape and symlink blocking plus legal `..name` readiness. | Not live Go file mutation. |
| Confirmation/audit | The fixture pins `APPLY_CHECKPOINT_REWIND`, append-only event kinds, no transcript rewrite, and no git refs. | Not Kun git-ref checkpoint semantics. |
| Go calculation | `replayG5CheckpointRewind` computes the expected shadow output from fixture-owned data. | TypeScript runtime remains authoritative. |
| Product boundary | No checkpoint top-level route, bridge/settings/provider/default-backend, or renderer-visible Go route changes. | G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go G5 shadow computes checkpoint/rewind safety boundaries from
TS-owned checkpoint oracle fixtures.
```

Forbidden wording:

```text
Do not claim live Go checkpoint readiness, Go runtime parity, default Go
backend, Reasonix checkpoint/rewind protocol parity, Kun git-ref checkpoint
parity, renderer-visible Go routes, packaged desktop rewind QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 75. 2026-06-22 Go G5 Remote-Entry Boundary Executable Shadow

This addendum advances G5 shadow evidence for remote-entry control boundaries
while keeping real remote-entry behavior in TypeScript runtime control ports.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Narrow port set | `controlExecutableCases.remoteEntry` records only `approvals`, `lifecycle`, and `turns` as allowed ports. | Not Reasonix SessionAPI. |
| Forbidden control planes | The fixture records goal, checkpoint, memory, session/thread storage, thread service, and tool host as forbidden. | Remote entries cannot become full runtime controllers. |
| Override rejection | The fixture records `approvalPolicy` override rejection. | Not remote provider/sandbox policy control. |
| Go calculation | `replayG5RemoteEntry` computes the expected shadow output from fixture-owned data. | TypeScript runtime remains authoritative. |
| Product boundary | No remote-entry top-level route, bridge/settings/provider/default-backend, or renderer-visible Go route changes. | G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go G5 shadow computes the remote-entry narrow control-port boundary
from TS-owned route fixtures.
```

Forbidden wording:

```text
Do not claim live Go remote-entry readiness, Go runtime parity, default Go
backend, Reasonix SessionAPI/protocol parity, renderer-visible Go routes,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 76. 2026-06-22 Go G5 Resume Pending Gates Executable Shadow

This addendum advances G5 shadow evidence for session-resume approval/user-input
gate safety while keeping real resume behavior in the TypeScript runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source gate state | `controlExecutableCases.resumePendingGates` records source approval/user-input statuses as `pending`. | Not a public ask/session protocol. |
| Resumed gate state | The fixture records resumed approval as `expired` and resumed user input as `cancelled`. | Resumed gates are not actionable. |
| Privacy/audit | Go shadow computes `pendingAfterResume:0` and `answersCopiedToResume:false`. | Not answer archival in SSE replay. |
| Go calculation | `replayG5ResumePendingGates` computes expected output from fixture-owned data. | TypeScript runtime remains authoritative. |
| Product boundary | No resume top-level route, bridge/settings/provider/default-backend, or renderer-visible Go route changes. | G6 and release readiness remain blocked. |

Allowed wording:

```text
Analytix Go G5 shadow computes resume pending-gate safety from TS-owned route
fixtures.
```

Forbidden wording:

```text
Do not claim live Go resume readiness, Go runtime parity, default Go backend,
Reasonix ask/session protocol parity, renderer-visible Go routes, packaged
desktop resume QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 62. 2026-06-21 Go G3/G5 Provider Cache Accounting Shadow

This addendum pins fixture-only provider cache accounting for Go G3/G5 shadow.
The TypeScript provider/cache oracle remains authoritative; Go may compute
fixture accounting but must not become a provider client or backend.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider usage accounting | `cacheAccounting` records supported telemetry cases, unsupported unknown cases, hit/miss totals, and aggregate hit rate. | Not live provider superiority. |
| Provider family coverage | DeepSeek, OpenAI Responses, and Anthropic cache cases are grouped explicitly; unsupported OpenAI-compatible stays unknown. | Not credentialed provider matrix QA. |
| Go shadow output | `buildG3ProviderCacheAccounting` and G5 `cacheReplay` compute accounting from fixture-owned provider usage cases. | Not a Go provider client or backend. |
| Product boundary | No Reasonix provider protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

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
Rust/Tauri migration from this batch.
```

## 63. 2026-06-21 Auto-Router Failure Usage Isolation

This addendum pins the analytix-owned auto-router boundary for Reasonix
post-881 classifier/currentness absorption. The classifier is internal routing
logic, not a user-visible auto-plan config or public protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Failure fallback | `_auto_router` failure falls back to heuristic model selection. | Not Reasonix auto-plan config parity. |
| Router request isolation | Router request carries no tools and no immutable prefix. | Not public planner/agent protocol. |
| Usage/cache isolation | Router usage/cache telemetry emitted before failure is not recorded as main thread usage and does not accumulate in `UsageService`. | Not live provider/cache superiority. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Managed settings rebuild proof remains separate. |

Allowed wording:

```text
Analytix auto-router failure falls back to heuristic routing without recording
router usage/cache telemetry as main turn usage.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible auto-plan settings,
Reasonix controller rebuild parity, live provider/cache superiority, default Go
backend, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 64. 2026-06-21 Task-Job Stale Restart Reconciliation Oracle

This addendum pins queued/running stale task-job reconciliation as an
analytix-owned runtime contract. It absorbs Reasonix-style job lifecycle
discipline without exposing a Reasonix public job protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| TS task-job oracle | `durableRunner.staleReconcile` records running and queued orphan ids, restart reason, expected failed status, and reconciled count. | Not a public sub-agent/job API. |
| Runtime reconciliation proof | `task-job-orchestration-oracle.test.ts` writes queued + running stale records and proves both become `failed` after restart reconciliation. | Not packaged desktop QA. |
| Go G5 executable shadow | `controlExecutableCases.taskJobs.staleReconcile` and `replayG5TaskJobStaleReconcile` compute stale jobs -> failed from TS-owned fixtures. | Not a Go Job Manager or default backend. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | G6 readiness remains blocked. |

Allowed wording:

```text
Analytix reconciles queued and running stale task jobs to explicit failed state
in TS runtime tests and Go G5 executable shadow.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, live Go task/job
execution, Go default backend, renderer-visible Go routes, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 65. 2026-06-21 MCP Annotation Approval No-Execute Oracle

This addendum pins annotation-driven MCP approval as an analytix-owned tool
contract. MCP descriptors may influence approval posture, but denied approval
must happen before `client.callTool`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| MCP lifecycle oracle | `approvalAnnotations` records the source server/tool, normalized tool name, annotations, approval id, deny decision, approval result kind, and no-execute flag. | Not a public MCP protocol. |
| Runtime approval proof | `mcp-tool-lifecycle-oracle.test.ts` proves a destructive/openWorld MCP tool denied by GUI approval returns an approval item and never calls the MCP client. | Not live MCP credential matrix QA. |
| Go G4 shadow | `mcpApprovalAnnotatedNoExecute` is computed from fixture-owned decision/executed fields. | Not a Go MCP client or backend. |
| Product boundary | No Reasonix public protocol, MCP-indexer top-level entry, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix MCP annotations feed approval gating, and denied annotated MCP tools
do not execute in TS lifecycle tests or Go G4 shadow evidence.
```

Forbidden wording:

```text
Do not claim Reasonix MCP public protocol parity, MCP-indexer top-level entry,
live MCP credential compatibility, Go MCP client readiness, default Go backend,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 66. 2026-06-21 User-Input Submitted Route Oracle

This addendum pins submitted user-input answers as an analytix-owned HTTP/gate
contract while keeping SSE replay answer-free.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Submitted route | `/v1/user-inputs/:id` returns submitted answers and resolves the pending user-input gate with the same answers. | Not a Reasonix public user-input protocol. |
| SSE replay boundary | `user_input_resolved` replay records `status: "submitted"` but omits answers. | Not answer archival in event history. |
| Go G4 shadow | `userInputSubmittedAnswersEchoed` and `userInputResolvedEventOmitsAnswers` are computed from fixture-owned fields. | Not a Go user-input server or backend. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Packaged GUI live-card QA remains separate. |

Allowed wording:

```text
Analytix user-input submitted answers are echoed through HTTP/gate resolution,
while SSE replay records only submitted status.
```

Forbidden wording:

```text
Do not claim Reasonix public user-input protocol parity, answer archival in SSE
history, live cross-device user-input delivery, Go user-input backend readiness,
default Go backend, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 67. 2026-06-21 Task-Job Route Auth Matrix Oracle

This addendum pins task-job wait/output/kill as authenticated internal runtime
routes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Auth matrix | `routeContract.authMatrix` lists wait/output/kill and expected unauthorized 401. | Not a public job API. |
| Runtime route proof | `task-job-orchestration-oracle.test.ts` verifies all three task-job routes reject missing runtime token. | Not packaged desktop QA. |
| Product boundary | No Reasonix job/session protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Go route auth remains deferred because no Go route is exposed. |

Allowed wording:

```text
Analytix task-job wait/output/kill routes are authenticated internal runtime
routes and reject missing runtime tokens.
```

Forbidden wording:

```text
Do not claim Reasonix public job protocol parity, unauthenticated task-job
control, Go route auth readiness, default Go backend, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 68. 2026-06-21 Connect Phone Product Copy Sovereignty

This addendum pins the Connect Phone naming boundary for Kun baseline
preservation. The internal `claw` module/settings/IPC names may remain as
compatibility implementation details, but user-visible and model-visible
natural-language copy must say Connect Phone/analytix.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Renderer locales | `connect-phone-product-copy.test.ts` scans English and Chinese locale strings and rejects standalone `Claw`. | Does not rename internal keys. |
| IM/runtime replies | IM help/model replies, retired task reply, webhook disabled reply, and IPC mirror-init errors use Connect Phone wording. | Not a new public protocol. |
| Prompt natural language | Connect Phone agent/tool hints replace Claw agent wording while display unwrap accepts both old and new skill-policy prefixes. | Internal prompt markers stay compatible. |
| Product boundary | No Kun identity, Reasonix public protocol, deprecated bridge/settings fallback, default Go backend, Rust/Tauri path, or new top-level navigation surface changed. | Internal `claw` rename remains separate. |

Allowed wording:

```text
Analytix Connect Phone user-visible and model-visible copy is guarded against
standalone Claw wording while internal `claw` compatibility names remain.
```

Forbidden wording:

```text
Do not claim internal `claw` schema/IPC/type names were renamed, Kun public
identity was restored, Reasonix public protocol was exposed, Go backend
readiness improved, default Go backend was enabled, release readiness was
achieved, or Rust/Tauri migration was started from this batch.
```

## 69. 2026-06-21 Managed Runtime Provider Currentness And Identity Guard

This addendum closes the scoped managed runtime rebuild proof for provider
currentness. It does not add a Reasonix auto-plan setting or public protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime settings key | Main `ensureRuntime` fingerprint and settings-apply changed check share `buildAnalytixRuntimeSettingsKey(resolveAnalytixRuntimeSettings(...))`. | Not a Reasonix config root. |
| Provider currentness | Selected provider base URL, endpoint format, model profile, and reasoning drift change the runtime key and refresh `ANALYTIX_MODEL_PROVIDERS`. | Not live provider credential QA. |
| Model-visible identity | `ANALYTIX_SYSTEM_PROMPT` uses Connect Phone wording and rejects `Claw/Kun/Reasonix` natural-language leaks. | Internal `claw` compatibility names remain. |
| Bridge/settings fallback | Public facade domains stay analytix-owned, and `agentProvider: "kun"` / `agents.kun` cannot become active runtime fallback. | Not an internal compatibility rename. |
| Product boundary | No Reasonix auto-plan setting, project/local override, controller API, SessionAPI, Kun identity, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Public auto-plan parity remains rejected/deferred. |

Allowed wording:

```text
Analytix managed runtime provider currentness is keyed into runtime rebuild
decisions and child provider snapshots without exposing upstream config or
public bridge domains.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible auto-plan settings,
Reasonix public protocol, live provider/cache superiority, internal `claw`
contract rename, default Go backend, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 70. 2026-06-21 Provider Endpoint URL Builder Parity

This addendum pins shared endpoint URL construction for auxiliary provider
consumers. The goal is request-shape non-regression, not live provider parity.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Shared URL helper | `upstreamOpenAiModelEndpointUrl` supports responses/messages versioned bases and known endpoint path stripping while preserving chat/custom behavior. | Not a public provider protocol. |
| Scheduled detector | Reminder detection uses the shared helper and no longer duplicates paths for `/v2/responses` or `/v3/messages`. | Not packaged Schedule QA. |
| Write-inline | Inline completion uses the same helper, with focused tests guarding existing custom endpoint behavior. | Not live Write provider matrix. |
| Product boundary | No Reasonix provider protocol, Kun identity, deprecated bridge/settings fallback, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Live provider/cache superiority remains deferred. |

Allowed wording:

```text
Analytix auxiliary provider consumers share endpoint URL construction for
versioned responses/messages bases and known endpoint paths.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix,
Reasonix provider protocol parity, packaged Schedule QA, release readiness,
default Go backend, Kun identity, or Rust/Tauri migration from this batch.
```

## 61. 2026-06-21 Go G5 Task Approval Deny Executable Control

This addendum pins the executable Go G5 shadow requirement for task approval
denial. The calculation is fixture-only and must not be exposed as live runtime
behavior.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Executable input | `controlExecutableCases.approvalDeny` carries attempted tool names and approval ids derived from the TS task-job oracle. | Not live Go approval behavior. |
| Executable output | `replayG5ApprovalDeny` returns denied tool names, approval ids/count, and no durable-job/child-run side effects. | Not Go Job Manager readiness. |
| Test binding | Go tests compare executable output against `go-g5-full-loop-oracle.json`; TS tests bind the input to `approvalDenyNoExecute`. | Not renderer-visible Go behavior. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix Go G5 executable shadow computes task approval denial no-side-effect
results from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Go approval manager readiness, live Go task/job execution, Go G5
runtime parity, default Go backend, Reasonix public sub-agent/job protocol,
packaged desktop approval QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 71. 2026-06-22 Auto-Router Classifier Request Contract

This addendum pins the analytix `_auto_router` classifier as an internal
short-JSON side path. It absorbs Reasonix classifier/currentness value without
importing Reasonix auto-plan config or public controller protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Router request shape | `auto-model-router.test.ts` proves `_auto_router` uses the DeepSeek flash classifier, compact JSON response format, `stream:false`, `maxTokens:96`, `temperature:0`, `reasoningEffort:"off"`, `tools: []`, and `prefix: []`. | Not a public classifier protocol. |
| Prompt boundary | The classifier prompt wraps selected mode, recent context, and latest request in one classifier user item without main-turn context instructions. | Not Reasonix SessionAPI/controller state. |
| Timeout fallback | Slow classifier calls abort internally and fall back to heuristic concrete model/reasoning. | Not user-visible auto-plan setting parity. |
| Fingerprint currentness | Timeout drift changes `AUTO_MODEL_ROUTER_FINGERPRINT`, preserving route-cache rebuild semantics. | Not live controller rebuild UX. |
| Product boundary | No Reasonix auto-plan config, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix `_auto_router` is an isolated short-JSON classifier path with
fingerprinted currentness and heuristic timeout fallback.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible planner toggles,
Reasonix SessionAPI/controller protocol, Go auto-router parity, live
provider/cache superiority, default Go backend, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 72. 2026-06-22 Planner-Executor Transcript Propagation

This addendum pins planner-executor transcript propagation as an internal
analytix task-job contract. It absorbs Reasonix transcript continuity value
without exposing a Subagent product surface or Reasonix public protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Coordinator | `runPlannerExecutorCoordinator` accepts optional `transcriptFor(task)` and persists the returned `TaskJobTranscriptRef` on executor jobs. | Not a public task/job API. |
| Identity guard | `task-job-orchestration-oracle.test.ts` uses `resolveTranscriptOperation` before propagation and proves executor jobs carry the fork ref. | Not Reasonix SessionAPI. |
| G5 shadow | G5 fixture/schema and Go shadow output replay `requiresTranscriptPropagation` and `transcriptPropagationMode`. | Not a live Go Job Manager. |
| Product boundary | No Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix planner-executor jobs can persist identity-checked transcript refs as
internal durable metadata.
```

Forbidden wording:

```text
Do not claim Reasonix public Subagent/SessionAPI parity, top-level Subagent or
Workflow navigation, live Go Job Manager readiness, default Go backend,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 73. 2026-06-22 MCP Search Meta-Tool Trust Boundary

This addendum pins MCP search discovery as an analytix-owned internal tool
surface. Search-mode meta-tools must respect workspace trust and approval
gates, and must not become a top-level MCP-indexer product surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| MCP search | `mcp-tool-provider.test.ts` proves trusted workspace search returns indexed tools while untrusted workspace search sees 0 tools. | Not a public MCP-indexer route. |
| MCP meta-call | The same test proves untrusted `toolId` returns an error without client execution. | Not Reasonix MCP protocol. |
| Approval | The same test proves denied `mcp_call` returns an approval item before MCP client execution. | Not approval bypass. |
| G4 shadow | G4 fixture/schema and Go shadow replay advertised/trust/no-execute evidence. | Not a live Go MCP client. |
| Product boundary | No MCP-indexer route, Reasonix public protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix MCP search meta-tools are advertised only as internal tools and
respect workspace trust plus approval before MCP client execution.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer protocol parity, top-level MCP-indexer
navigation, credentialed MCP matrix readiness, live Go MCP client readiness,
default Go backend, packaged desktop MCP QA, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 74. 2026-06-22 Custom Messages Full Endpoint Request Shape

This addendum pins custom full `/messages` endpoint behavior for provider
request-shape non-regression.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Custom endpoint URL | `provider-cache-oracle.json` includes a custom full `/messages` endpoint whose expected URL is exact. | Not a Reasonix provider protocol. |
| Messages request shape | `provider-cache-proof.test.ts` verifies Anthropic Messages headers/body/tool shape and forbids OpenAI body fields. | Not live provider QA. |
| Product boundary | No Reasonix provider protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has fake-fetch provider evidence that custom full `/messages`
endpoints keep exact URL and Anthropic Messages request shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed OpenAI/Anthropic/
custom provider matrix readiness, Reasonix provider protocol parity, default Go
backend, packaged provider settings QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 76. 2026-06-22 Auto-Router Request Contract Fingerprint Currentness

This addendum pins the post-881 Reasonix classifier-rebuild value as
analytix-owned route-cache currentness, without accepting Reasonix public
auto-plan settings.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Auto-router fingerprint | `buildAutoModelRouterFingerprint` includes classifier request-contract fields with stable defaults. | Not a Reasonix controller rebuild. |
| Currentness proof | `auto-model-router.test.ts` proves max-token, temperature, and reasoning-effort drift changes the fingerprint. | Not a user-visible auto-plan setting. |
| Product boundary | No Reasonix config/protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

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
migration from this batch.
```

## 77. 2026-06-22 Connect Phone Copy Sovereignty

This addendum pins Connect Phone product copy while retaining legacy Claw
compatibility for historical sessions.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime prompts | New Connect Phone runtime prompts emit Connect Phone managed/agent headings. | Internal compatibility constants may still use `CLAW_` names. |
| Incoming titles/logs | New incoming IM thread titles and webhook log messages say Connect Phone. | Legacy Claw-titled threads are still recognized. |
| Schedule schema | Schedule MCP description and scheduled-task type comment say Connect Phone. | `claw_channel_id` remains compatibility input. |
| Product boundary | No Kun/Reasonix/Claw public product identity, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix emits Connect Phone copy for new Connect Phone prompts, titles,
schema descriptions, and logs while retaining legacy Claw recognizers.
```

Forbidden wording:

```text
Do not claim all internal compatibility names were renamed, legacy Claw
history support was removed, default Go backend readiness, release readiness,
Kun identity, Reasonix protocol parity, or Rust/Tauri migration from this batch.
```

## 78. 2026-06-22 Go G5 Durable Runner Restart Executable Shadow

This addendum pins durable task-job restart continuity as a G5 executable
shadow case.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 task-job restart | `go-g5-full-loop-oracle.json` includes `controlExecutableCases.taskJobs.restartDrill`. | Not a live Go Job Manager. |
| TS oracle authority | `go-runtime-conformance.test.ts` derives expected restart values from `task-job-orchestration-oracle.json`. | TypeScript runtime remains live authority. |
| Go shadow | `packages/runtime-go` computes rehydrated count, combined output/next offset, and queued kill result. | Not a default backend. |
| Product boundary | No Reasonix protocol, renderer-visible Go route, Kun public protocol, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay durable task-job restart output,
offset, and kill behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 75. 2026-06-22 Custom Chat Full Endpoint Request Shape

This addendum pins custom full `/chat/completions` endpoint behavior for
provider request-shape non-regression.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Custom endpoint URL | `provider-cache-oracle.json` includes a custom full `/chat/completions` endpoint whose expected URL is exact. | Not a Reasonix provider protocol. |
| Chat request shape | `provider-cache-proof.test.ts` verifies OpenAI-compatible chat headers/body/tool shape and forbids Responses/Messages-only fields. | Not live provider QA. |
| Go shadow | G3/G5 fixtures replay the custom chat request-shape id. | Not a live Go provider client or default backend. |
| Product boundary | No Reasonix provider protocol, Kun public protocol, default Go backend, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix has fake-fetch provider evidence that custom full
`/chat/completions` endpoints keep exact URL and OpenAI-compatible chat
request shape.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed OpenAI/Anthropic/
custom provider matrix readiness, Reasonix provider protocol parity, default Go
backend, packaged provider settings QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 79. 2026-06-22 Go G5 Approval/User-Input Abort Cleanup Executable Shadow

This addendum pins approval/user-input abort cleanup as a G5 executable shadow
case. It extends existing TypeScript route/renderer evidence into Go shadow
without making Go the live backend.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 abort cleanup | `go-g5-full-loop-oracle.json` includes `controlExecutableCases.abortCleanup`. | Not a live Go approval/user-input manager. |
| TS oracle authority | `go-runtime-conformance.test.ts` derives expired approval, cancelled user-input, late statuses, pending count, and replay kinds from `approval-user-input-route-oracle.json`. | TypeScript runtime remains live authority. |
| Go shadow | `packages/runtime-go` computes `expired`, `cancelled`, late approval `409`, late user-input `404`, `pendingAfterCleanup: 0`, replay kinds, and product-boundary booleans. | Not a default backend. |
| Product boundary | No Reasonix ask/session protocol, renderer-visible Go route, Kun public protocol, Rust/Tauri path, or top-level navigation surface changed. | Release readiness remains blocked. |

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
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 83. 2026-06-22 Sub-Agent Review Matrix Gate

This addendum makes the six-lane sub-agent review a required QA artifact for
the post-881 absorption stream.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Review evidence | `post-881-subagent-review-2026-06-22.md` records lanes A-F and delta classifications. | Not a live Reasonix parity claim. |
| Provider/cache wording | Custom provider proof is explicitly request-shape-only; DeepSeek/OpenAI Responses/Anthropic keep raw accounting proof. | No credentialed provider/cache superiority matrix. |
| Go G5/G6 boundary | The review records Go G5 executable shadow coverage and open G6 gates. | Not a default backend or renderer-visible Go route. |
| Kun baseline | The review repeats the no top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer boundary. | Not a Kun navigation expansion. |

Allowed wording:

```text
Analytix has scan-visible sub-agent review evidence for post-881 Reasonix
absorption decisions and open gates.
```

Forbidden wording:

```text
Do not claim live Reasonix parity, live provider/cache superiority,
independent custom provider cache telemetry, default Go backend, G6 readiness,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 84. 2026-06-22 Custom Provider Telemetry Seal

This addendum turns the custom provider request-shape-only boundary into an
executable provider/cache and Go G5 shadow gate.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Custom full endpoints | Custom `/responses`, `/messages`, and `/chat/completions` ids remain request-shape cases. | Not custom cache telemetry. |
| Cache telemetry | `customFullEndpointTelemetryCaseIds` is `[]`, and `telemetrySupportedExcludesCustomFullEndpoints` is `true`. | No credentialed custom provider cache matrix. |
| Go G5 control shadow | `providerCacheCoverageFloor.expected` carries the same seal and Go computes it from fixture inputs. | Not a live Go provider client. |
| Product boundary | Scan tokens now include the custom telemetry seal. | Not a provider runtime behavior change. |

Allowed wording:

```text
Analytix proves custom full endpoints are request-shape-only in the current
provider/cache oracle and Go G5 shadow.
```

Forbidden wording:

```text
Do not claim independent custom provider cache telemetry, live provider/cache
superiority, default Go backend, G6 readiness, packaged provider QA, release
readiness, Kun identity, Reasonix provider protocol, or Rust/Tauri migration
from this batch.
```

## 85. 2026-06-22 Goal Persistence Off-Lock Control Shadow

This addendum closes the previous Go conformance gap for goal persistence
outside shared controller/status/approval locks.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source-derived proof | `go-runtime-conformance.test.ts` derives the case from `ThreadService` and `thread-service.test.ts`. | Not a TS runtime behavior change. |
| Persist-before-event ordering | `setGoal` and `clearGoal` persist before `goal_updated` / `goal_cleared`. | Not a live Go goal manager. |
| Failure handling | Persistence failures warn with analytix identity and surface the original error. | Not Reasonix controller protocol. |
| Go G5 shadow | `goalPersistenceOffLock` computes no forbidden controller/status/approval lock coupling. | Not default Go backend or G6 readiness. |

Allowed wording:

```text
Analytix G5 shadow has source-derived proof that current goal persistence is
off shared controller/status/approval locks and warns on persistence failures.
```

Forbidden wording:

```text
Do not claim live Go goal persistence, TypeScript controller-lock migration,
Reasonix controller protocol parity, default Go backend, G6 readiness,
packaged goal QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 86. 2026-06-22 Tool Result File/Image Boundary Shadow

This addendum moves tool result file/image preservation into G5 executable
shadow evidence while keeping live attachment behavior in the TypeScript
runtime and renderer projection.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source-derived proof | `go-runtime-conformance.test.ts` derives `toolResultFileImageBoundary` from tool-result image, attachment-store, and renderer mapper tests. | Not a runtime behavior change. |
| Image context hygiene | Inline image kinds and newest-image cap are preserved while evicted base64 payloads are omitted and metadata remains. | Not a live Go model-history implementation. |
| Attachment path propagation | Attachment `localFilePath` and text fallback `FilePath` remain evidence-bearing fields. | Not Reasonix file protocol. |
| Generated-file projection | Renderer mapper proof keeps tool attachment/generated-file meta lifting visible. | Not a new top-level UI entry. |
| Go G5 shadow | Go computes the file/image boundary output from fixture inputs. | Not default Go backend or G6 readiness. |

Allowed wording:

```text
Analytix G5 shadow has source-derived proof that tool result images/files keep
path and generated-file metadata without retaining old base64 payloads in
model-visible context.
```

Forbidden wording:

```text
Do not claim live Go attachment/file bridge readiness, Reasonix file protocol
parity, renderer-visible Go routes, default Go backend, packaged attachment QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 87. 2026-06-22 Event JSONL Replay Boundary Shadow

This addendum moves event JSONL replay and malformed-event recovery into G5
executable shadow evidence while keeping live event persistence in the
TypeScript runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source-derived proof | `go-runtime-conformance.test.ts` derives `eventJsonlReplayBoundary` from file-session and runtime-event tests. | Not a runtime behavior change. |
| Event replay | `events.jsonl` append is newline-terminated; `loadEventsSince` filters/sorts by seq; `highestSeq` preserves the max seq. | Not a live Go event store. |
| Recorder ordering | Runtime events persist before publish, concurrent seqs stay unique, and persisted high-water is read once per thread. | Not Reasonix SessionAPI. |
| Recovery/compaction | Malformed JSONL lines are skipped; usage compaction keeps carryover and compaction failure keeps the append-only log. | Not packaged long-thread replay QA. |
| Go G5 shadow | Go computes the event replay boundary output from fixture inputs. | Not default Go backend or G6 readiness. |

Allowed wording:

```text
Analytix G5 shadow has source-derived proof that event JSONL replay,
malformed recovery, and usage compaction boundaries remain analytix-owned and
append-safe.
```

Forbidden wording:

```text
Do not claim live Go event-store readiness, Reasonix event protocol parity,
renderer-visible Go routes, default Go backend, packaged long-thread replay
QA, release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 88. 2026-06-22 MCP Malformed Schema Boundary Shadow

This addendum moves malformed MCP schema normalization into G5 executable
shadow evidence while keeping live MCP behavior in the TypeScript runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Source-derived proof | `go-runtime-conformance.test.ts` derives `mcpMalformedSchemaBoundary` from MCP provider source and tests. | Not a runtime behavior change. |
| Safe input schema defaults | Non-object `inputSchema` values normalize to safe object schemas before advertisement. | Not a live Go MCP client. |
| Schema sanitization | Non-record `properties` are dropped and mixed `required` arrays keep only string entries. | Not Reasonix MCP-indexer protocol. |
| Catalog safety | Normalized tool names remain advertised and non-record output schemas are omitted. | Not credentialed MCP QA. |
| Go G5 shadow | Go computes the malformed schema boundary output from fixture inputs. | Not default Go backend or G6 readiness. |

Allowed wording:

```text
Analytix G5 shadow has source-derived proof that malformed MCP schemas are
normalized before model/tool catalog exposure.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP-indexer protocol
parity, renderer-visible Go routes, default Go backend, credentialed MCP QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 103. 2026-06-22 Browser Preview Bridge / Visible Entry Sovereignty

This addendum strengthens the upstream absorption gate outside the Go runtime:
browser preview bridge code and visible shell entries are product-contract
surfaces, not test-only conveniences.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Browser preview bridge | Source guard proves the dev browser bridge installs `window.analytix`, uses the analytix runtime proxy and `/v1/threads/:id/events` SSE path, and exposes no deprecated aliases. | Not a Reasonix public protocol or Electron preload replacement. |
| Visible sidebar entry surface | Static render smoke rejects Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer labels from the top-level sidebar. | Dormant Create Loop internals remain quarantined only. |
| Product-sovereignty scan | Scan path freshness now fails if Workbench, Sidebar, preload/shared contracts, browser bridge, or guard tests disappear from expected coverage. | Not packaged desktop QA. |

Allowed wording:

```text
Analytix has source and rendered-shell evidence that browser preview bridge and
visible sidebar entry surfaces stay on analytix-owned contracts.
```

Forbidden wording:

```text
Do not claim Reasonix public protocol parity, Kun Workflow/Create Loop top-level
navigation, Subagent/AutoResearch/MCP-indexer product entries, default Go
backend, renderer-visible Go routes, packaged desktop QA, release readiness,
Kun identity, deprecated bridge/settings fallback, or Rust/Tauri migration from
this batch.
```

## 104. 2026-06-22 Browser Preview SSE Executable Bridge Proof

This addendum turns browser preview SSE bridge sovereignty from a source-only
assertion into an executable unit proof.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Browser preview SSE path | `browser-analytix-bridge.test.ts` starts an SSE stream through `window.analytix.runtime.startSse` and proves it requests the analytix `/v1/threads/:id/events` proxy path. | Not Reasonix SessionAPI or a public session route protocol. |
| Cursor/currentness | The request carries `Last-Event-ID` from `since_seq`, preserving analytix runtime replay semantics. | Not packaged browser QA. |
| Event normalization | SSE `id`, `event`, and JSON `data` normalize to renderer payload fields `seq`, `kind`, and payload values. | Renderer consumers still see analytix-owned event payloads only. |

Allowed wording:

```text
Analytix browser preview SSE bridge executable tests prove analytix proxy path,
cursor header, and event normalization behavior.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public protocol parity, live Go bridge,
default Go backend, renderer-visible Go routes, packaged browser/desktop QA,
release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration from this batch.
```

## 105. 2026-06-22 Browser Preview Settings Sovereignty Proof

This addendum closes the browser-preview settings side of the bridge boundary:
development browser settings must normalize through the same analytix-owned
schema as desktop settings.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Shared settings normalization | `normalizeAppSettings` strips top-level `agentProvider`, `agents`, `deepseek`, and `reasonix` alongside Reasonix `agent`/`autoPlan`/`auto_plan`. | Not legacy settings fallback. |
| Browser preview settings load | Polluted localStorage can keep valid `runtime.model` and `runtime.endpointFormat`, but rejected app/runtime fields do not survive `getSettings`. | Not Reasonix config root or auto-plan settings parity. |
| Browser preview settings save | `saveSettingsSilent` persists normalized browser preview settings without rejected app/runtime fields. | Not packaged settings QA. |

Allowed wording:

```text
Analytix browser preview settings normalize through top-level runtime settings
and drop legacy/Reasonix agent envelopes on load and re-save.
```

Forbidden wording:

```text
Do not claim Reasonix config/auto-plan parity, deprecated settings fallback,
Kun agent-provider fallback, live Go backend, renderer-visible Go routes,
packaged settings QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 106. 2026-06-22 IPC Settings Patch Sovereignty Proof

This addendum closes the renderer-to-main settings patch boundary: legacy and
Reasonix top-level settings envelopes are stripped before strict IPC schema
validation, while valid analytix patch fields still survive.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| IPC patch sanitization | `stripLegacySettingsPatchKeys` strips top-level `agent`, `autoPlan`, `auto_plan`, `agentProvider`, `agents`, `deepseek`, `reasonix`, and `quickChat`. | Not deprecated settings fallback. |
| Valid patch preservation | Schema test proves legal `locale`, `disabledSkillIds`, provider media patch, and runtime patch fields survive pollution stripping. | Not Reasonix config import. |
| Runtime-level legacy shapes | Runtime nested legacy agent-shaped keys still reject instead of merging as fallback. | No public auto-plan setting. |

Allowed wording:

```text
Analytix IPC settings patch validation drops top-level legacy/Reasonix envelopes
before strict validation while preserving valid analytix settings patch fields.
```

Forbidden wording:

```text
Do not claim Reasonix config/auto-plan parity, deprecated settings fallback,
Kun agent-provider fallback, live Go backend, renderer-visible Go routes,
packaged settings QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 107. 2026-06-22 Settings Sovereignty Scan Freshness Proof

This addendum makes the recently closed bridge/settings sovereignty proofs part
of the product-sovereignty scan path contract. The scan now fails if shared
settings normalization, active runtime settings, settings-store persistence,
IPC settings patches, preload bridge, browser preview bridge, or their core
guard tests disappear from the source tree.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Settings chokepoints | `scan:product-sovereignty` requires shared normalize/runtime settings, main settings store, IPC settings schema, preload, and browser preview bridge paths. | Not packaged settings QA. |
| Guard tests | The same scan requires shared settings, settings-store, IPC schema, preload sandbox, and browser preview bridge tests to remain present. | Not a replacement for focused behavioral tests. |
| Product boundary | No new route, bridge alias, settings fallback, or Go backend is introduced. | Not Reasonix config or public auto-plan parity. |

Allowed wording:

```text
Analytix product-sovereignty scans now include settings/bridge sovereignty
chokepoints and guard tests as required source paths.
```

Forbidden wording:

```text
Do not claim packaged settings QA, Reasonix config/auto-plan parity,
deprecated settings fallback, live Go bridge, renderer-visible Go route,
default Go backend, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 108. 2026-06-22 Workflow Singular Route Negative Proof

This addendum aligns live TypeScript HTTP negative-route evidence with the
existing task-job and Go G4/G5 forbidden route oracles. The singular
`/v1/workflow` route is now tested directly alongside plural `/v1/workflows`
and the other forbidden upstream public surfaces.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Workflow route rejection | `http-server.test.ts` proves `/v1/workflow` and `/v1/workflows` return structured 404s. | Not a Workflow product entry. |
| Oracle alignment | The live HTTP test now covers the same singular `/v1/workflow` path recorded by task-job and Go G4/G5 fixtures. | Not live Go HTTP routing. |
| Product boundary | No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route is added. | Internal capability code remains behind analytix-owned contracts. |

Allowed wording:

```text
Analytix live HTTP route tests prove singular `/v1/workflow` remains absent
alongside plural `/v1/workflows`.
```

Forbidden wording:

```text
Do not claim Workflow product-entry readiness, Reasonix public protocol parity,
live Go routes, renderer-visible Go route, default Go backend, packaged desktop
QA, release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 109. 2026-06-22 Runtime Proof Freshness Scan

This addendum makes runtime conformance proof entry points part of the
product-sovereignty scan. It does not add new provider/cache or
approval/user-input behavior; it keeps the existing proof files, fixtures, and
Go shadow sources from silently disappearing.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime proof freshness | `scan:product-sovereignty` requires provider-cache, approval/user-input, G2/G3/G4/G5 fixtures/tests, and Go shadow source paths. | Not a new behavior oracle. |
| Route/client forbidden scan | Active runtime routes, shared endpoint templates, IPC schema, and renderer runtime client are scanned for forbidden public route/protocol strings. | Tests/fixtures may still record forbidden paths as negative evidence. |
| Go conformance currentness | The Go conformance table now records G2 exact route replay as closed and G3/G4/G5 provider/cache and approval/user-input evidence as active shadow oracle input. | No live Go backend or default route. |

Allowed wording:

```text
Analytix product-sovereignty scans now include runtime conformance proof
freshness and active route/client forbidden-surface checks.
```

Forbidden wording:

```text
Do not claim new provider/cache behavior, credentialed provider parity,
live Go routes, renderer-visible Go route, default Go backend, packaged route
or provider QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 110. 2026-06-22 Renderer Runtime Request Surface Proof

This addendum turns renderer runtime route sovereignty from source scanning
into executable provider behavior. A renderer test now calls the real
`AnalytixRuntimeProvider` methods for common HTTP flows and checks every
captured `runtimeRequest` path.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Renderer request paths | Provider calls for connect, thread list/create/turn, steer/interrupt, compact, goal/todos, approval/user-input, fork, and resume stay on `/health` or analytix-owned `/v1/*` routes. | Not packaged desktop QA. |
| Forbidden public surfaces | The captured renderer paths must not match Reasonix public routes, renderer-visible Go routes, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public routes. | Tests/fixtures may still record forbidden paths as negative evidence. |
| Product boundary | No bridge alias, Reasonix protocol, Kun protocol, or Go default backend is introduced. | Live Go remains gated. |

Allowed wording:

```text
Analytix renderer runtime provider requests stay on analytix-owned HTTP routes
and avoid forbidden upstream public surfaces.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 111. 2026-06-22 Renderer SSE Bridge Cursor Proof

This addendum turns renderer SSE bridge/cursor sovereignty into executable
provider behavior. The renderer provider must subscribe through
`window.analytix.runtime.startSse` with the caller's thread id and replay
cursor, not by introducing a direct Reasonix or Go-visible SSE route.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Renderer SSE start | `AnalytixRuntimeProvider.subscribeThreadEvents` calls `startSse(threadId, sinceSeq, streamId)` with the original thread/cursor and a generated non-empty stream id. | Not packaged desktop SSE QA. |
| Stream cleanup | Aborted streams call `stopSse(streamId)` for the same stream id used to dispatch events. | Does not prove main-process reconnect behavior. |
| Bridge boundary | Renderer SSE subscription does not use `runtimeRequest` as a side channel and remains behind `window.analytix.runtime`. | URL/header/reconnect behavior remains covered by preload/main/browser bridge tests. |

Allowed wording:

```text
Analytix renderer SSE subscription stays on the window.analytix bridge/cursor
contract and cleans up the same generated stream id.
```

Forbidden wording:

```text
Do not claim packaged desktop SSE QA, Reasonix public SSE protocol parity,
live Go routes, renderer-visible Go route, default Go backend, Workflow
product-entry readiness, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 112. 2026-06-22 Main/Preload SSE IPC Bridge Proof

This addendum completes the desktop SSE proof below the renderer provider:
preload must map SSE only through analytix-owned IPC, and main must preserve
the thread/cursor/stream contract while requesting the analytix runtime events
route.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Preload SSE IPC | `startSse` and `stopSse` map to `runtime:sse:start` and `runtime:sse:stop` under `window.analytix.runtime`. | Not a public Reasonix/Kun bridge. |
| Main SSE schema | `sseStartPayloadSchema` trims and preserves `threadId`, `sinceSeq`, and optional `streamId`, and rejects extra session/protocol fields. | Not Reasonix SessionAPI. |
| Main route/header/cursor | The first runtime fetch uses `/v1/threads/:id/events?since_seq=...` with runtime auth and initial `Last-Event-ID` for non-zero cursors. | Not a Go-visible route. |
| Stream lifecycle | Generated stream ids are returned and carried in error payloads; `stopSse` aborts only matching stream ids. | Packaged desktop streaming QA remains separate. |

Allowed wording:

```text
Analytix desktop SSE IPC preserves the analytix-owned thread cursor and stream
id contract from preload to main.
```

Forbidden wording:

```text
Do not claim packaged desktop SSE QA, Reasonix public SSE protocol parity,
live Go routes, renderer-visible Go route, default Go backend, Workflow
product-entry readiness, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 113. 2026-06-22 Main Runtime Request Forbidden Route Proof

This addendum completes the runtime request side of the desktop bridge proof:
main IPC must reject upstream public route families before they can reach the
runtime request adapter.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Forbidden route table | `runtimeRequestPayloadSchema` explicitly rejects Reasonix, renderer-visible Go, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer request paths. | Negative test strings are not public routes. |
| Handler boundary | `runtime:request` rejects invalid payloads and does not call `runtimeRequest`. | Not packaged desktop route QA. |
| Analytix allow-list | Valid main IPC runtime requests remain limited to modeled analytix endpoint templates. | Not Reasonix SessionAPI or Kun protocol. |

Allowed wording:

```text
Analytix main IPC rejects upstream public runtime routes before they reach the
runtime request adapter.
```

Forbidden wording:

```text
Do not claim packaged desktop route QA, Reasonix public runtime protocol
parity, live Go routes, renderer-visible Go route, default Go backend,
Workflow product-entry readiness, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 114. 2026-06-22 Preload Runtime Request IPC Bridge Proof

This addendum completes the preload side of runtime request sovereignty:
preload must expose only the analytix `runtime:request` IPC channel for
runtime HTTP calls.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Preload runtime request IPC | `runtimeRequest(path, method, body)` maps to `ipcRenderer.invoke('runtime:request', { path, method, body })`. | Not a public Reasonix/Kun bridge. |
| Upstream channel rejection | Source guard rejects Reasonix/Kun/Go/Workflow request IPC channel names. | Negative strings are not public channels. |
| Main allow-list handoff | Route authorization remains owned by the main IPC schema/handler. | Not packaged desktop route QA. |

Allowed wording:

```text
Analytix preload runtime requests use only the analytix-owned runtime:request
IPC channel.
```

Forbidden wording:

```text
Do not claim packaged desktop route QA, Reasonix public runtime protocol
parity, live Go routes, renderer-visible Go route, default Go backend,
Workflow product-entry readiness, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 115. 2026-06-22 Runtime Desktop Bridge Proof Freshness Scan

This addendum makes the runtime request/SSE desktop bridge proof chain part of
the product-sovereignty scan. The goal is proof freshness, not new runtime
behavior.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime bridge proof paths | `scan:product-sovereignty` requires renderer provider/client, browser preview bridge, preload bridge/types, main IPC schema/handler, main SSE IPC, shared API/endpoints, and focused tests. | Not packaged desktop route/SSE QA. |
| Proof continuity | D-0156 and D-0162 through D-0166 source/test chokepoints must remain present. | No new route behavior. |
| Product boundary | Existing forbidden-surface scans still reject active Reasonix/Go/Workflow public route strings outside negative tests/docs. | Negative evidence remains in tests/docs only. |

Allowed wording:

```text
Analytix product-sovereignty scan now guards the desktop runtime bridge proof
chain for path freshness.
```

Forbidden wording:

```text
Do not claim packaged desktop route/SSE QA, Reasonix public runtime protocol
parity, live Go routes, renderer-visible Go route, default Go backend,
Workflow product-entry readiness, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 116. 2026-06-22 Provider Cache Coverage Floor

This addendum makes provider/cache oracle breadth an executable contract. The
goal is to prevent a future Reasonix/Kun/Go absorption batch from silently
shrinking DeepSeek, OpenAI-compatible, OpenAI Responses, Anthropic Messages, or
custom full endpoint coverage.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider family schema | `ProviderLiveLocalHttpProof.coveredProviderFamilies` accepts only the five required fixture families. | Not live provider QA. |
| Runtime oracle coverage | `provider-cache-proof.test.ts` derives family coverage from usage/request-shape cases and pins required case ids. | No provider behavior change. |
| Go shadow input | Existing G3/G5 shadow continues to consume the TS-owned oracle; no Go backend route or provider client is added. | Go remains shadow-only. |

Allowed wording:

```text
Analytix provider/cache fixtures now have an executable coverage floor for
DeepSeek, OpenAI-compatible, OpenAI Responses, Anthropic Messages, and custom
full endpoints.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
parity, Reasonix provider protocol parity, default Go backend readiness,
renderer-visible Go routes, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 117. 2026-06-22 G5 Provider Cache Coverage Floor Control Shadow

This addendum promotes the provider/cache coverage floor into Go G5 executable
control output while keeping Go shadow-only.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 provider/cache control | `controlExecutableCases.providerCacheCoverageFloor` is present in the G5 oracle. | Not a live Go provider client. |
| Executable replay | Go computes required/covered provider families, telemetry-supported cases, unsupported unknown cache posture, and custom full endpoint exact-URL/tool-shape coverage. | Not credentialed provider QA. |
| Product boundary | Output pins no live credentials, no live superiority claim, no Reasonix protocol, and no top-level route exposure. | Go remains shadow-only. |

Allowed wording:

```text
Analytix G5 shadow executable output now replays the provider/cache coverage
floor from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider parity, live provider/cache superiority,
credentialed provider matrix readiness, Reasonix provider protocol parity,
default Go backend readiness, renderer-visible Go routes, packaged provider
QA, release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 118. 2026-06-22 Provider Request-Shape Derived Replay

This addendum strengthens provider URL/body contract evidence by requiring
request-shape replay to derive URL, header, body, and tool-shape matches from
fixture inputs.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G3/G5 provider replay | Request-shape summaries include derived URL/header/body/tool-shape match ids. | Not a live provider client. |
| Custom full endpoint safety | Custom `/responses`, `/messages`, and `/chat/completions` endpoints preserve exact URLs with appended-path count `0`. | Not credentialed provider QA. |
| Product boundary | No provider runtime behavior, Reasonix protocol, Go route, default backend, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G3/G5 shadow replay derives provider request-shape URL/header/body
matches from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend readiness,
renderer-visible Go routes, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 119. 2026-06-22 Post-881 Runtime Proof Freshness V2

This addendum makes the reusable product-sovereignty scan guard the important
post-881 runtime proof leaves and tokens.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Scan freshness | Task-job and MCP lifecycle fixture/test leaves are part of runtime proof freshness. | Not a behavior change. |
| Positive proof tokens | Provider, planner/task, and MCP proof tokens must remain scan-visible. | Not live parity. |
| Product boundary | No Reasonix protocol, Go route/default backend, Kun identity, hidden top-level entry, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix product-sovereignty scan now guards key post-881 runtime proof tokens.
```

Forbidden wording:

```text
Do not claim live provider/MCP/job parity, default Go backend readiness,
renderer-visible Go routes, Reasonix public protocol parity, packaged QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 120. 2026-06-22 Provider Cache Accounting Raw Payload Replay

This addendum upgrades provider cache accounting from expected-summary replay
to raw provider payload replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G3/G5 fixtures | Provider usage/accounting rows carry raw `responseBody.usage` payloads. | Not a live provider matrix. |
| TS and Go shadow | Cache accounting is derived from raw DeepSeek/OpenAI Responses/Anthropic/unsupported usage shapes before totals are computed. | Not a live Go provider client. |
| Product boundary | Unsupported cache telemetry remains unknown, and no Reasonix provider/cache protocol is exposed. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G3/G5 shadow can derive provider cache accounting from TS-owned
raw provider payload fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend readiness,
renderer-visible Go routes, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 121. 2026-06-22 Provider Raw Accounting Proof Freshness

This addendum makes raw provider cache accounting a direct TypeScript oracle
invariant and a product-sovereignty scan freshness requirement. The active
runtime boundary remains `analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider-cache proof | Raw `responseBody.usage` payloads derive usage snapshots and aggregate cache accounting directly. | Not a live provider matrix. |
| Scan freshness | `scan:product-sovereignty` requires raw accounting proof tokens across post-881 proof paths. | Not behavior parity by itself. |
| Product boundary | No provider client, settings, URL/body, Go backend, or route behavior changes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix provider-cache proof directly derives raw provider cache accounting
from TS-owned payload fixtures and guards that proof with sovereignty scan
tokens.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend readiness,
renderer-visible Go routes, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 122. 2026-06-22 Combined Step/Cancel/Cache Proof Freshness

This addendum makes combined auto-route cache, step-limit, and cancel evidence
a focused G5 oracle and a product-sovereignty scan freshness requirement. The
active runtime boundary remains `analytix serve`; no Go agent loop is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 conformance | A dedicated test proves same-turn route-cache reuse, next-turn reroute, step-limit boundary, cancel result pairing, and stable-prefix isolation. | Not live Go loop parity. |
| Scan freshness | `scan:product-sovereignty` requires combined step/cancel/cache proof tokens across post-881 proof paths. | Not behavior parity by itself. |
| Product boundary | No runtime loop, settings, Go backend, route, or navigation changes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix G5 proof directly guards the combined step/cancel/cache trace and
pins that evidence with sovereignty scan tokens.
```

Forbidden wording:

```text
Do not claim live Go agent-loop parity, public auto-plan setting parity,
Reasonix controller/session protocol parity, default Go backend readiness,
renderer-visible Go routes, packaged cancel/cache QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 123. 2026-06-22 Approval/User-Input Proof Freshness

This addendum makes approval/user-input gate safety a focused G5 oracle and a
product-sovereignty scan freshness requirement. The active runtime boundary
remains `analytix serve`; no Go gate manager is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 conformance | A dedicated test proves denied no-execute, answer privacy, late rejection, abort cleanup, resume cleanup, and boundary flags. | Not live Go gate parity. |
| Scan freshness | `scan:product-sovereignty` requires approval/user-input proof tokens across post-881 proof paths. | Not behavior parity by itself. |
| Product boundary | No live gate manager, settings, Go backend, route, or navigation changes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix G5 proof directly guards approval/user-input gate safety and pins
that evidence with sovereignty scan tokens.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager parity, Reasonix ask/session
protocol parity, default Go backend readiness, renderer-visible Go routes,
packaged approval-card QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 124. 2026-06-22 Session Route Proof Freshness

This addendum makes thread/session route replay a focused G5 oracle and a
product-sovereignty scan freshness requirement. The active runtime boundary
remains `analytix serve`; no Go thread/session router is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 conformance | A dedicated test proves archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, exact hashes, and boundary flags. | Not live Go route parity. |
| Scan freshness | `scan:product-sovereignty` requires session route proof tokens across post-881 proof paths and keeps the G2 route oracle in scope. | Not behavior parity by itself. |
| Product boundary | No live router, settings, Go backend, route, or navigation changes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix G5 proof directly guards thread/session route replay and pins that
evidence with sovereignty scan tokens.
```

Forbidden wording:

```text
Do not claim live Go thread/session router parity, Reasonix SessionAPI/public
event protocol parity, default Go backend readiness, renderer-visible Go
routes, packaged route walkthrough QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 125. 2026-06-22 Release Evidence Final-Gate Freshness

This addendum makes post-881 release evidence final gates a
product-sovereignty scan freshness requirement. The active runtime boundary
remains `analytix serve`; no runtime, provider, bridge, or Go backend behavior
changes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Release evidence | `scan:product-sovereignty` requires final command gate entries for D-0172 through D-0177. | Not a replacement for running gates. |
| Command coverage | The scan requires runtime package tests, workspace tests, typecheck, runtime build, non-cached Go shadow tests, and product-sovereignty scan commands to remain visible. | Not packaged/live QA. |
| Product boundary | No live router, settings, Go backend, route, or navigation changes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix product-sovereignty scan now guards post-881 release evidence final
gate freshness.
```

Forbidden wording:

```text
Do not claim release readiness, packaged desktop QA, live provider/MCP matrix
completion, G6 readiness, default Go backend readiness, renderer-visible Go
routes, Kun identity, Reasonix public protocol parity, or Rust/Tauri migration
from this batch.
```

## 126. 2026-06-22 Post-881 Stage Closure Snapshot

This addendum makes the current post-881 capability floor, absorbed
Reasonix delta families, Kun baseline preservation, stronger-than evidence,
and remaining open gates a product-sovereignty scan freshness requirement.
The active runtime boundary remains `analytix serve`; no runtime, provider,
bridge, or Go backend behavior changes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Closure snapshot | `post-881-stage-closure-2026-06-22.md` records floor, absorbed deltas, Kun baseline, stronger-than evidence, and open gates. | Not a release-readiness claim. |
| Scan freshness | `scan:product-sovereignty` requires closure tokens for floor, Reasonix absorption, Kun baseline, stronger-than evidence, and open gates. | Not behavior parity by itself. |
| Product boundary | Snapshot and scan preserve no top-level hidden entries, no Reasonix protocol, no default Go backend, and no Rust/Tauri rewrite. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix has a machine-scannable post-881 stage closure snapshot for the
verified fixture/contract/governance floor.
```

Forbidden wording:

```text
Do not claim live Reasonix parity, live provider/cache superiority, packaged
desktop QA, live Go backend readiness, G6 readiness, release readiness, Kun
identity, Reasonix public protocol parity, or Rust/Tauri migration from this
batch.
```

## 106. 2026-06-22 Auto-Router Recommendation/Currentness Control Shadow

Reasonix post-881 classifier rebuild/currentness remains absorbed only through
the analytix `_auto_router` runtime contract. G5 now requires
`controlExecutableCases.autoRouterClassifier` to include recommendation parsing
and recent-context currentness evidence, not just fingerprint and timeout
fallback fields.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Recommendation parsing | Go G5 computes two accepted and two rejected classifier recommendation cases, including `pro/max` acceptance and `model:"auto"` / malformed rejection. | Not Reasonix auto-plan protocol. |
| Context currentness | G5 proves active-turn text is excluded from recent context and historical tool results are summarized. | Not a user-facing auto-plan setting. |
| Product boundary | No Reasonix controller protocol, renderer-visible Go route, default backend, deprecated bridge/settings fallback, or top-level hidden-capability route is added. | Packaged desktop QA and G6 remain blocked. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay auto-router recommendation parsing
and recent-context currentness from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix controller/session protocol parity, public auto-plan
settings, live Go auto-router readiness, default Go backend, packaged desktop
QA, release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 107. 2026-06-22 Exact Session Route Replay Control Shadow

Thread/session route evidence must prove the concrete analytix HTTP/SSE route
contracts, not only aggregate route counts. G5 now requires
`controlExecutableCases.sessionRouteReplay`, a G2-derived exact route matrix for
thread list/archive/search/read/update, fork, resume, SSE replay, caught-up SSE,
and unauthorized SSE access.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Exact JSON routes | The G5 matrix records method, path, auth, status, request-body hash, response-body shape/hash for 9 JSON routes. | Not a live Go HTTP server. |
| Exact SSE routes | Replay and caught-up SSE routes record frame count, event names, and frame hash. | Not renderer-visible Go routes. |
| Product boundary | Go shadow computes no Reasonix protocol, no top-level route, no renderer-visible Go route, and no default Go backend. | Packaged desktop route QA and G6 remain blocked. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay exact thread/session route contracts
for fork/resume/archive/search/SSE from TS-owned G2 fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route parity, live Go HTTP server
readiness, renderer-visible Go routes, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 83. 2026-06-22 Go G5 Context Compaction Boundary Replay

This addendum promotes latest-compaction effective-history semantics into G5
control executable replay. It strengthens long-thread/cache evidence without
making Go the default backend.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.compactionBoundary` records effective ids, dropped ids, latest compaction id, and product-boundary booleans. | Not a live Go history manager. |
| Long-history model input | The latest compaction item with `replacedTokens > 0` becomes the first effective history item; older/noop compactions and pre-boundary items are dropped. | Not packaged long-history QA. |
| Cache isolation | Compaction boundary control state remains outside the stable prefix. | Not a live provider cache superiority claim. |

Allowed wording:

```text
Analytix G5 shadow executable output covers latest compaction boundary replay
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go history manager readiness, default Go backend,
Reasonix SessionAPI/controller protocol parity, packaged desktop long-history
QA, G6 readiness, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 84. 2026-06-22 Go G5 User-Input Structured Validation Replay

This addendum promotes structured `request_user_input` validation into G5
control executable replay. It absorbs the useful Reasonix AskTool validation
idea through analytix-owned contracts only.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.userInput.structuredChoiceValidation` records max questions, option bounds, duplicate-label handling, invalid cases, and product-boundary booleans. | Not a live Go user-input manager. |
| Invalid structured requests | Too many questions, single option, too many options, and case-insensitive duplicate labels are all rejected with `invalid_user_input_request`. | Not Reasonix `ask` protocol. |
| Gate safety | Invalid structured requests do not open a pending user-input gate. | Not packaged approval/user-input QA. |

Allowed wording:

```text
Analytix G5 shadow executable output covers structured request_user_input
validation from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend,
Reasonix ask/session protocol parity, packaged desktop approval-card QA, G6
readiness, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 103. 2026-06-22 Provider Cache Privacy Control Shadow

This addendum promotes provider cache diagnostics privacy and live-superiority
policy into G5 executable control shadow. It strengthens provider/cache
evidence without exposing Reasonix provider protocol or enabling Go providers.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 provider control | `controlExecutableCases.providerCachePrivacy` records diagnostics field count, forbidden substring count, no-leak status, no live credentials, and no live superiority claim. | Not a live Go provider client. |
| Diagnostics privacy | Stable prefix text, tool schema text, API key text, and `Authorization` are forbidden from diagnostics payloads. | Not a credentialed provider matrix. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers provider cache diagnostics privacy and
no-live-superiority policy from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider readiness, default Go backend readiness,
Reasonix provider protocol parity, credentialed provider matrix readiness,
live provider/cache superiority, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 104. 2026-06-22 Provider Cache Inventory Control Shadow

This addendum promotes provider cache stable-prefix/tool/case inventory into
G5 executable control shadow. It strengthens provider/cache currentness
evidence without exposing Reasonix provider protocol or enabling Go providers.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 provider control | `controlExecutableCases.providerCacheInventory` records stable prefix hash, tools hash, provider usage ids/count, request-shape ids/count, prefix equivalence, and tools hash stability. | Not a live Go provider client. |
| Stable prefix safety | Inventory is fixture-only and must not add dynamic workspace material to the stable prefix. | Not Reasonix provider protocol. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers provider cache prefix/tool/case-id
inventory from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider readiness, default Go backend readiness,
Reasonix provider protocol parity, credentialed provider matrix readiness,
dynamic stable-prefix material, live provider/cache superiority, packaged
provider QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 105. 2026-06-22 Session Route Inventory Control Shadow

This addendum promotes thread/session route inventory into G5 executable
control shadow. It strengthens route-surface conformance without exposing
Reasonix SessionAPI or enabling Go thread/session routes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 route control | `controlExecutableCases.sessionRouteInventory` records route ids, JSON/SSE route ids, event/resume/fork/archive/search/read-update groups, runtime-token count, and unauthorized route ids. | Not a live Go HTTP server. |
| Runtime HTTP/SSE safety | Inventory is fixture-only and remains derived from the TS-owned G2 route oracle. | Not Reasonix SessionAPI or public route protocol. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, top-level hidden-capability route, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers thread/session route inventory from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go routing readiness, default Go backend readiness,
Reasonix SessionAPI/public route protocol parity, packaged desktop route QA,
renderer-visible Go routes, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 106. 2026-06-22 Approval/User-Input Inventory Control Shadow

This addendum promotes approval/user-input gate inventory into G5 executable
control shadow. It strengthens gate/replay conformance without exposing
Reasonix ask/session protocol or enabling Go approval/user-input managers.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 gate control | `controlExecutableCases.approvalUserInputInventory` records gate ids, approval ids, user-input ids, route kinds, replay kinds, abort replay kinds, answer count, late statuses, and pending-after sum. | Not a live Go approval/user-input manager. |
| Answer privacy | HTTP response may echo submitted answers, while resolved SSE events remain `includesAnswers: false`. | Not Reasonix ask/session protocol. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, top-level hidden-capability route, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers approval/user-input gate inventory from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix ask/session protocol parity, packaged desktop approval-card
QA, renderer-visible Go routes, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 107. 2026-06-22 Task Planner Toolset Inventory Control Shadow

This addendum promotes task/sub-agent planner read-only and forbidden toolset
inventory into G5 executable control shadow. It strengthens planner gating
evidence without exposing Reasonix sub-agent/job protocol or enabling a Go
planner/executor.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 task-job control | `controlExecutableCases.taskJobs.plannerToolsetInventory` records read-only tools, forbidden `task`/`parallel_tasks`, counts, no-overlap check, task-tool match check, and policy values. | Not a live Go planner/executor or Job Manager. |
| Planner safety | Planner read-only toolset excludes task orchestration tools; forbidden toolset matches the task orchestration tools. | Not Reasonix public sub-agent/job protocol. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, top-level hidden-capability route, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers task planner toolset inventory from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go planner/executor readiness, default Go backend readiness,
Reasonix public sub-agent/job/planner protocol parity, packaged desktop
sub-agent QA, renderer-visible Go routes, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer navigation, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 108. 2026-06-22 MCP Core Lifecycle Boundary Seal Control Shadow

This addendum seals MCP core lifecycle provider identity and product-boundary
flags inside G5 executable control shadow. It strengthens MCP lifecycle
evidence without exposing Reasonix MCP-indexer protocol or enabling a Go MCP
client.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 MCP lifecycle control | `controlExecutableCases.mcpCoreLifecycle` records `providerId: "mcp:research"` and product-boundary flags in addition to core lifecycle input/output. | Not a live Go MCP client. |
| Product boundary | Go shadow computes no Reasonix protocol and no top-level route exposure for the MCP core lifecycle case. | Not Reasonix MCP-indexer public protocol. |
| Runtime contract | The TS MCP lifecycle oracle remains authoritative and no renderer/preload/main/runtime public contract changes. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers MCP core lifecycle provider/boundary seal
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, credentialed MCP matrix readiness,
packaged desktop MCP QA, renderer-visible Go routes, top-level MCP-indexer
navigation, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 109. 2026-06-22 Provider Request-Shape Exact Matrix Control Shadow

This addendum promotes provider request-shape evidence from summary counts into
G3/G5 exact matrix replay. It strengthens provider/cache URL/header/body
contract evidence without enabling a Go provider client or Reasonix provider
protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G3 provider/cache shadow | `expectedOutput.requestShapeSummary.matrix` carries all 7 TS-owned exact request-shape cases. | Not a live provider client. |
| G5 cache/control shadow | `cacheReplay.requestShapeReplay.matrix` and `controlExecutableCases.providerRequestShape.expected.matrix` carry exact URL/header/body/tool-shape cases. | Not credentialed provider matrix parity. |
| Go fixture validation | Go tests for fixture-driven shadow changes must use `-count=1` so JSON oracle drift is executed, not hidden by cache. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G3/G5 executable shadow covers provider request-shape exact matrix
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider client readiness, default Go backend readiness,
Reasonix provider protocol parity, credentialed provider matrix readiness,
packaged desktop provider settings QA, renderer-visible Go routes, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 110. 2026-06-22 Combined Step/Cancel/Cache Trace Control Shadow

This addendum promotes combined auto-route cache, step-limit, and cancel
evidence from summary booleans into G5 executable trace replay. It strengthens
agent-loop control evidence without enabling a live Go agent loop.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 combined control | `controlExecutableCases.combined.expected` records router calls, main/max model steps, next-turn reroute calls, stable-prefix flags, and cancel counts. | Not a live Go agent loop. |
| Cancel trace | The combined output includes exact completed/running/unstarted cancel result rows and completed/aborted counts. | Not packaged long-running cancel QA. |
| Cache/step boundary | Same-turn route cache reuse is tied to `mainModelSteps == maxModelSteps`, while classifier and dynamic step state remain outside stable prefix. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers combined step/cancel/cache trace from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, default Go backend readiness,
Reasonix controller/session protocol parity, public auto-plan product surface,
packaged desktop cancel/cache QA, renderer-visible Go routes, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 111. 2026-06-22 Step-Limit Override/Delegate Matrix Control Shadow

This addendum promotes step-limit override and delegate inheritance evidence
from scalar output into G5 executable matrix replay. It strengthens planner and
sub-agent limit evidence without enabling a live Go agent loop.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 step-limit control | `controlExecutableCases.stepLimits.expected.matrix` records default, user, session, turn, planner, headless, zero-default, delegate, and delegate-floor rows. | Not a live Go agent loop. |
| Delegate inheritance | Delegate parent-half and min-floor behavior are both pinned (`12 -> 6`, `8 -> 5`). | Not packaged step-limit QA. |
| Stable-prefix boundary | Each row records `dynamicLimitInStablePrefix:false`; zero-default records guard disable without changing product routing. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers step-limit override/delegate matrix from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, default Go backend readiness,
Reasonix controller/session protocol parity, public auto-plan product surface,
packaged desktop step-limit QA, renderer-visible Go routes, release readiness,
Kun identity, or Rust/Tauri migration from this batch.
```

## 112. 2026-06-22 History Repair Pair-Integrity Control Shadow

This addendum promotes model-history repair and tool-call/result pair
integrity into G5 executable replay. It strengthens provider legality and
cache-stability evidence without enabling a live Go history manager.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 history repair control | `controlExecutableCases.historyRepair` records simplified history items plus repaired/dropped ids. | Not a live Go agent loop. |
| Pair integrity | Complete multi-tool blocks survive while orphan results, missing-result calls, and duplicate results are dropped. | Not packaged long-history QA. |
| Stable-prefix boundary | `stablePrefixContainsRepairState:false` is pinned alongside no Reasonix protocol and no top-level route exposure. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers history repair pair integrity from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, live Go history-manager readiness,
default Go backend readiness, Reasonix SessionAPI/controller protocol parity,
public auto-plan product surface, packaged desktop long-history QA,
renderer-visible Go routes, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 83. 2026-06-22 Go G5 Approval/User-Input Route Replay Control Shadow

This addendum promotes approval/user-input route replay into G5 executable
control shadow. It strengthens approval/user-input and SSE replay evidence
without creating a live Go gate manager.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.approvalUserInputRouteReplay` records approval deny route, submitted user-input route, cancelled user-input route, replay kinds, late action statuses, and abort cleanup. | Not a live Go approval/user-input manager. |
| GUI gate semantics | Submitted answers may echo in HTTP response, but resolved events omit answers; cancelled and late resolves stay rejected. | Not Reasonix ask/session protocol. |
| Product boundary | No Go renderer route, default Go backend, Reasonix protocol, Kun identity, deprecated bridge/settings fallback, or hidden top-level route is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays approval/user-input route
semantics from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, G6 readiness, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 84. 2026-06-22 Go G5 MCP Search Refresh Drift Control Shadow

This addendum promotes MCP search refresh catalog drift into G5 executable
control shadow. It strengthens MCP lifecycle evidence without creating a live
Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpSearchRefreshDrift` records server id, initial tool names, expanded tool names, indexed count, catalog drift, and no top-level route. | Not a live Go MCP client. |
| MCP lifecycle | Catalog refresh drift remains tied to analytix-owned MCP lifecycle oracle and internal meta-tool boundaries. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP search refresh catalog drift
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 87. 2026-06-22 Go G5 MCP Core Lifecycle Control Shadow

This addendum promotes MCP connect/disconnect/reload/cancel/error evidence into
G5 executable control shadow. It strengthens MCP lifecycle evidence without
creating a live Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpCoreLifecycle` records connect/disconnect diagnostics, reload tool names, schema-order stability, cancel no-execute, and approved error shape. | Not a live Go MCP client. |
| MCP lifecycle | Cancel-before-start remains non-executing, and approved errors preserve `tool_execution_failed`. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP core lifecycle behavior from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 90. 2026-06-22 Go G5 Task-Job Tool Contract Boundary Control Shadow

This addendum promotes task/parallel task tool contract and route-boundary
evidence into G5 executable control shadow. It strengthens sub-agent/job
orchestration evidence without creating a live Go Job Manager or public
Reasonix job protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.taskJobs.toolContractBoundary` records `task`/`parallel_tasks` internal-runtime-only flags, permission/evidence/dependency/read-only gates, runtime task-job routes, protected route auth, unauthorized status, forbidden top-level routes, no Reasonix protocol, and no top-level route exposure. | Not a live Go Job Manager. |
| Product contract | Task/sub-agent capability remains an internal runtime tool contract, not a top-level route or public protocol. | Not Reasonix SessionAPI/job protocol parity. |
| Route surface | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays task-job tool contract boundary
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix public sub-agent/job protocol parity, renderer-visible Go route,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged desktop QA, G6 readiness, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 92. 2026-06-22 Go G5 Nested Child SSE Metadata Control Shadow

This addendum promotes nested child SSE metadata evidence into G5 executable
control shadow. It strengthens child event attribution evidence without
creating a live Go Job Manager or public Reasonix SessionAPI.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.taskJobs.nestedSseMetadata` records parent call id, child run id, metadata field coverage, evidence ledger keys, parent/child distinctness, active-goal requirement, no Reasonix protocol, and no top-level route exposure. | Not a live Go Job Manager. |
| Timeline/projection | Child event attribution keeps parent/child and evidence ledger metadata available for analytix-owned projection. | Not Reasonix SessionAPI/job protocol parity. |
| Route surface | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays nested child SSE metadata from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix SessionAPI or public sub-agent/job protocol parity, renderer-visible
Go route, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged desktop QA, G6 readiness, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 91. 2026-06-22 Go G5 Task Transcript Identity Control Shadow

This addendum promotes transcript continue/fork identity evidence into G5
executable control shadow. It strengthens sub-agent/job orchestration evidence
without creating a live Go Job Manager or public Reasonix SessionAPI.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.taskJobs.transcriptIdentity` records source id, continue target id, fork target id, incompatible identity error, same-transcript identity requirement, continue target matching source, fork target distinct from source, no Reasonix protocol, and no top-level route exposure. | Not a live Go Job Manager. |
| Thread/fork semantics | Continue preserves the source target and fork creates a distinct target, preserving analytix-owned transcript ownership. | Not Reasonix SessionAPI/job protocol parity. |
| Route surface | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays task transcript identity from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix SessionAPI or public sub-agent/job protocol parity, renderer-visible
Go route, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged desktop QA, G6 readiness, release readiness, Kun identity,
or Rust/Tauri migration from this batch.
```

## 89. 2026-06-22 Go G5 Product Boundary Control Shadow

This addendum promotes global product-boundary evidence into G5 executable
control shadow. It strengthens analytix sovereignty proof without creating a
live Go backend, renderer-visible Go route, or public Reasonix protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.productBoundary` records bridge/serve stability, no Reasonix public protocol, no default Go backend, no renderer-visible Go route, no Electron main connection, and no enabled Go backend. | Not a live Go backend. |
| Product contract | `window.analytix`, top-level runtime settings, `analytix serve`, and Renderer -> preload -> main -> runtime HTTP/SSE remain authoritative. | Not Reasonix protocol or SessionAPI parity. |
| Route surface | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays product-boundary invariants from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go backend readiness, default Go backend readiness, Reasonix
public protocol parity, renderer-visible Go route, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged desktop QA, G6
readiness, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 88. 2026-06-22 Go G5 MCP Background Reconnect Control Shadow

This addendum promotes MCP background reconnect evidence into G5 executable
control shadow. It strengthens MCP lifecycle/retry evidence without creating a
live Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpBackgroundReconnect` records failed server ids, suspended provider/reason, connected/error outcomes, retry attempts, retry-all-failed coverage, and no-runtime-restart state. | Not a live Go MCP client. |
| MCP lifecycle | Failed-server reconnect evidence remains tied to the TS-owned MCP lifecycle oracle. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP background reconnect behavior
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 89. 2026-06-22 Go G5 MCP Known Override Diagnostics Control Shadow

This addendum promotes MCP known override diagnostics into G5 executable
control shadow. It strengthens MCP config/diagnostic evidence without creating
a live Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpKnownOverrideDiagnostics` records known override rows, variant count, override kinds, workspace roots, explicit-cwd server ids, daemon-timeout server ids, and low-priority/background-start booleans. | Not a live Go MCP client. |
| MCP lifecycle | Known override diagnostics remain tied to the TS-owned MCP lifecycle oracle and analytix-owned internal config semantics. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP known override diagnostics
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 90. 2026-06-22 Go G5 MCP Live-Local Indexer Control Shadow

This addendum promotes MCP live-local indexer lifecycle evidence into G5
executable control shadow. It strengthens MCP/indexer lifecycle proof without
creating a live Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpLiveLocalIndexer` records retry server ids/map, initial/resume/active paths, tombstone count, snapshot restart, late tombstone, secret-safe diagnostic, and execution-error redaction. | Not a live Go MCP client. |
| MCP lifecycle | Live-local indexer behavior remains tied to the TS-owned MCP lifecycle oracle and analytix-owned internal config semantics. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP live-local indexer lifecycle
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 91. 2026-06-22 Go G5 MCP Search Meta-Tool Control Shadow

This addendum promotes MCP search meta-tool advertised/trust/no-execute
evidence into G5 executable control shadow. It strengthens MCP search/indexer
trust evidence without creating a live Go MCP client or public MCP-indexer
surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpSearchMetaTools` records meta-tool names/count, refresh tool advertisement, trusted/untrusted workspace fields, trusted tool id, unknown-tool error, `on-request` policy, and denied no-execute. | Not a live Go MCP client. |
| MCP lifecycle | Search meta-tool behavior remains tied to the TS-owned MCP lifecycle oracle and analytix-owned internal tool semantics. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP search meta-tool
advertised/trust/no-execute behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 85. 2026-06-22 Go G5 MCP Approval Annotation Control Shadow

This addendum promotes MCP approval annotation/no-execute evidence into G5
executable control shadow. It strengthens MCP tool approval evidence without
creating a live Go MCP client or approval manager.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpApprovalAnnotations` records destructive/open-world hints, normalized tool name, approval id, deny decision, result kind, execution flag, and denied no-execute. | Not a live Go MCP client or approval manager. |
| Tool approval | Destructive/open-world MCP tools stay approval-gated and denied calls do not execute. | Not Reasonix approval protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP approval annotations and
denied no-execute behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, live Go approval manager readiness,
default Go backend readiness, Reasonix MCP-indexer/approval protocol parity,
top-level MCP-indexer navigation, credentialed MCP matrix, packaged desktop
MCP/approval QA, G6 readiness, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 86. 2026-06-22 Go G5 MCP Search Workspace Boundary Control Shadow

This addendum promotes MCP search workspace trust-boundary evidence into G5
executable control shadow. It strengthens MCP/indexer lifecycle evidence
without creating a live Go MCP client or public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control executable | `controlExecutableCases.mcpSearchWorkspaceBoundary` records trusted/untrusted workspaces, query, trusted tool id, untrusted search count, unknown-tool error, call policy, and denied no-execute. | Not a live Go MCP client. |
| Workspace trust | Untrusted workspace search remains `0` searched tools; unknown tools and denied calls do not execute. | Not Reasonix MCP-indexer protocol. |
| Product boundary | No top-level MCP-indexer route, Go renderer route, default Go backend, Reasonix protocol, Kun identity, or deprecated bridge/settings fallback is added. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 control shadow executable-replays MCP search workspace trust
boundaries from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix, packaged desktop MCP QA, G6 readiness, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 90. 2026-06-22 Plan Step Cancel/Cache Boundary Guard

This addendum absorbs Reasonix planner stability lessons through the
analytix-owned AgentLoop contract instead of upstream public protocol.

Requirement:

| Area | Required behavior | Excluded behavior |
| --- | --- | --- |
| Plan entry | Only explicit analytix `mode: "plan"` / `guiPlan` activates Plan mode. | No Reasonix auto-plan setting, controller protocol, or public planner route. |
| Step gating | Step 0 may advertise read-only tools plus `create_plan`; an unsatisfied follow-up step advertises only `create_plan`. | No mutating tools in Plan-mode investigation or forced follow-up step. |
| Cancellation | Interrupting the follow-up step aborts the turn and does not execute additional tools. | No hidden retry or protocol handoff. |
| Cache baseline | A cancelled follow-up step without usage must not advance `cachePrefixShapes`; the next equivalent Plan turn must report `prefixChanged: false`. | No dynamic context or cancelled tool surface in the stable cache prefix. |
| Provider accounting | DeepSeek-style cache hit/miss/rate telemetry remains provider-native and sanitized. | No guessed cache rate or live superiority claim. |
| Go boundary | TypeScript loop remains authoritative; Go remains shadow-only until G5/G6. | No default Go backend. |

Evidence:

```text
packages/runtime/tests/loop.test.ts
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

Acceptance statement:

```text
Analytix now has loop-level evidence that Plan-mode step narrowing and
interrupt handling do not regress DeepSeek-style cache baseline stability.
```

## 91. 2026-06-22 MCP Stdio Execution-Error Redaction

This addendum absorbs a Reasonix MCP lifecycle safety lesson by strengthening
analytix-owned MCP/tool result privacy.

Requirement:

| Area | Required behavior | Excluded behavior |
| --- | --- | --- |
| MCP thrown errors | Non-abort MCP failures must be redacted before `LocalToolHost` persists `tool_result.output.error`. | No raw secrets in model-visible errors. |
| MCP protocol errors | MCP results with `isError: true` must have nested text payloads redacted before becoming `tool_result.output.result`. | No raw `Authorization`/Bearer content in returned result payloads. |
| Executable proof | The stdio fake indexer must execute a failing diagnostic tool and prove the secret is absent. | No credentialed external MCP dependency. |
| Product boundary | The behavior stays inside `mcp-tool-provider` and analytix tool contracts. | No Reasonix MCP-indexer public route/navigation. |
| Go boundary | TypeScript provider remains authoritative for this slice. | No default Go backend. |

Evidence:

```text
packages/runtime/src/adapters/tool/mcp-tool-provider.ts
packages/runtime/tests/mcp-tool-lifecycle-oracle.test.ts
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

Acceptance statement:

```text
Analytix MCP tool results now redact both thrown failures and protocol-level
`isError` payloads before model-visible persistence.
```

## 92. 2026-06-22 Workflow/Create Loop Quarantine Scan

This addendum makes the dormant Workflow/Create Loop boundary explicit.

Requirement:

| Area | Required behavior | Excluded behavior |
| --- | --- | --- |
| Dormant code | Existing workflow/create-loop files may remain only as isolated dormant source. | No Workbench stage, route, sidebar entry, tray/menu shortcut, or preload API. |
| Scan gate | `scan:product-sovereignty` must reject workflow/create-loop symbols in entry surfaces. | No reliance on manual review only. |
| Route-surface test | The renderer route test must prove dormant symbols are absent from top-level sources. | No hidden import through store/navigation actions. |
| Kun boundary | Kun 0.2.13/0.2.14 top-level hierarchy remains Code/Write/Settings/Plugins/Connect Phone/Schedule. | No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry. |

Evidence:

```text
src/renderer/src/components/Workbench.route-surface.test.ts
scripts/scan-product-sovereignty.cjs
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 83. 2026-06-22 Reasonix Auto-Plan Config Boundary Guard

This addendum absorbs the useful boundary from Reasonix `01d9b173`: auto-plan
state must not become a project/local override or public upstream settings
shape in analytix.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| GUI settings | Root `agent.auto_plan`, `autoPlan`, and `auto_plan` are stripped during normalization. | Not a product auto-plan setting. |
| Runtime settings | Runtime `autoPlan` / `auto_plan` do not affect `buildAnalytixRuntimeSettingsKey` and are not persisted. | Not Reasonix controller rebuild protocol. |
| Runtime config | `AnalytixConfigSchema` rejects `agent.auto_plan`, `runtime.auto_plan`, and `serve.autoPlan`. | Not a new `analytix serve` public option. |

Allowed wording:

```text
Analytix rejects Reasonix auto-plan public config shapes and keeps future
auto-plan controls behind analytix-owned runtime settings.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan setting parity, project/local auto-plan
overrides, Reasonix controller API parity, packaged settings QA, default Go
backend readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 84. 2026-06-22 Go G5 Task Parent Goal Evidence Executable Replay

This addendum promotes task/sub-agent parent-goal evidence from G5 summary into
executable shadow output.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 task-job executable replay | `controlExecutableCases.taskJobs.parentGoalEvidence` records active-goal and missing-goal metadata. | Not a live Go Job Manager. |
| Parent evidence safety | Go shadow derives `ledgeredWhenActiveGoal` and `errorsWithoutActiveGoal` from event metadata. | Not Reasonix SessionAPI/job protocol. |
| Product boundary | `usesReasonixProtocol:false` and `topLevelRouteExposed:false` remain pinned. | No Subagent/Workflow/Create Loop route. |

Allowed wording:

```text
Analytix G5 executable shadow replays task parent-goal evidence metadata from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, live Go Job Manager
readiness, default Go backend readiness, top-level Subagent navigation,
packaged desktop sub-agent QA, release readiness, Kun identity, or Rust/Tauri
migration from this batch.
```

## 85. 2026-06-22 Product Sovereignty Scan Engineering

This addendum turns the repeated forbidden-surface release scans into a
reusable npm gate.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| QA entry point | `npm run scan:product-sovereignty` runs the product-sovereignty scan bundle. | Not packaged desktop QA. |
| Renderer route surface | Plugin Marketplace is included in top-level forbidden route-token coverage. | Does not hide or remove Plugins/MCP management. |
| Backend boundary | Scan includes default Go backend and Rust/Tauri activation/file checks. | Not Go backend readiness. |

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

## 126. 2026-06-22 G5 MCP Lifecycle Detail Replay

This addendum extends the MCP lifecycle shadow gate. The G5 shadow replay must
now include reconnect, known-override, and search workspace-boundary details
from the TS-owned MCP lifecycle oracle.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Background reconnect | `mcpReplay.backgroundReconnect` records failed server ids, suspended provider/reason, connected/error ids, retry attempts, retry coverage, and no-restart requirement. | Not a live Go MCP client. |
| Known overrides | `knownOverrideDiagnostics` records codegraph/codebase-memory effective cwd, priority, background-start, workspace root, daemon timeout, and explicit cwd. | Not Reasonix lifecycle protocol. |
| Search workspace boundary | `searchWorkspaceBoundary` records trusted/untrusted workspace, query, trusted tool id, unknown-tool error, on-request policy, and denied no-execute behavior. | Not top-level MCP-indexer navigation. |
| Product boundary | No MCP-indexer route, Reasonix protocol, Go route, Kun identity, default Go backend, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 127. 2026-06-22 G5 Provider Usage Parser Precedence Replay

This addendum extends the provider/cache shadow gate. The G5 shadow replay must
now include provider usage parser precedence details from the TS-owned
provider-cache oracle.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Unsupported provider usage | `cacheReplay.usageParserReplay.unsupportedAbsentFields` records absent cache-hit/cache-miss fields and unknown cache-hit rate. | Not live provider superiority. |
| DeepSeek precedence | `deepseekNativePrecedence` records native `prompt_cache_*` fields winning over `prompt_tokens_details.cached_tokens`. | Not a provider protocol change. |
| OpenAI responses usage | `openaiResponsesCachedTokens` records cached-token hit/miss and reasoning-token preservation. | Not a live Go provider client. |
| Anthropic usage | `anthropicCacheFields` records read/creation cache fields in prompt/cache accounting. | Not a default backend. |

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
Kun identity, or Rust/Tauri migration from this batch.
```

## 128. 2026-06-22 G5 Auto-Router Classifier Currentness Replay

This addendum extends the post-881 auto-plan/currentness gate. G5
`controlExecutableCases` must now include the analytix auto-router classifier
contract, drift invalidation, isolated request shape, timeout fallback, and
product-boundary rejection flags.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Classifier fingerprint | `autoRouterClassifier.fingerprint` is tied to live `AUTO_MODEL_ROUTER_FINGERPRINT`. | Not Reasonix auto-plan parity. |
| Contract drift | Model, prompt, timeout, max tokens, temperature, and reasoning drift invalidate the classifier route-cache contract. | Not a public setting. |
| Request isolation | Classifier request remains short JSON with no prefix/tools/context-instruction carryover. | Not public protocol. |
| Timeout fallback | Timeout aborts classifier and falls back to heuristic concrete model/reasoning. | Not live Go auto-router. |
| Product boundary | No public auto-plan setting, project override, Reasonix controller protocol, Go route, or default Go backend. | Release readiness remains blocked. |

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
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 125. 2026-06-22 G5 Approval/User-Input Route Body And Prompt Replay

This addendum extends section 80. The G5 shadow replay must now include exact
approval/user-input route body and prompt-shape evidence from the TS-owned
approval/user-input oracle.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Approval route replay | `approvalUserInputReplay.approvalRoute` records deny request body, denial response body, summary/tool name, and pending counts. | Not a live Go gate manager. |
| User-input submit replay | `userInputSubmitRoute` records prompt, question option labels, answer request body, HTTP response body, and SSE answer redaction. | Not Reasonix ask protocol. |
| User-input cancel replay | `userInputCancelRoute` records prompt, cancel request body, response body, late `404`, and pending cleanup. | Not a default backend. |
| Product boundary | No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Reasonix protocol, Go route, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow replay covers approval/user-input route bodies and
structured prompt shape from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 104. 2026-06-22 Go G5 Planner Gate Matrix Control Shadow

This addendum upgrades planner tool-policy evidence from a single forged task
rejection into a G5 control executable matrix. The active runtime boundary
remains `analytix serve`; no Go planner/executor backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.planner` carries normal-mode, capability-gate, step-0, step-1, and blocked-tool rejection rows. | Not live Go planner execution. |
| Go shadow | `BuildG5ControlExecutableOutput` computes normal agent `create_plan` hiding, Plan capability advertisement, step narrowing, and forged-tool rejection count/names. | Not a Go Job Manager or planner/executor. |
| Product boundary | The matrix rejects Reasonix planner/session protocol and top-level hidden-capability routes. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay planner gate matrix from
TS-owned fixtures without exposing Go or Reasonix planner routes.
```

Forbidden wording:

```text
Do not claim live Go planner/executor readiness, Reasonix SessionAPI/planner
protocol parity, public auto-plan settings, renderer-visible Go routes, default
Go backend, packaged desktop Plan QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 105. 2026-06-22 Desktop Bridge/Settings Sovereignty Control Shadow

This addendum upgrades desktop bridge and settings-schema sovereignty into G5
control executable evidence. The active renderer boundary remains
`window.analytix`; settings remain top-level `runtime`; no Reasonix public
protocol or Go desktop backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.desktopSovereignty` mirrors real preload/window/API/settings proof ids. | Not live Go Electron integration. |
| TS conformance | `go-runtime-conformance.test.ts` reads real desktop source files and derives bridge names, window type properties, facade domains, and settings proof ids. | Not a packaged app smoke test. |
| Go shadow | `BuildG5ControlExecutableOutput` computes single `analytix` bridge exposure, no forbidden aliases, Reasonix auto-plan drop proof, legacy envelope drop proof, and top-level runtime persistence proof. | Not a Go settings store. |
| Product boundary | Deprecated bridge aliases and settings fallbacks remain rejected. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay desktop bridge/settings
sovereignty from TS-owned desktop source proofs without exposing Go or Reasonix
public protocols.
```

Forbidden wording:

```text
Do not claim live Go desktop integration, Reasonix SessionAPI/config protocol
parity, deprecated bridge/settings fallback support, renderer-visible Go
routes, default Go backend, packaged desktop settings QA, release readiness,
Kun identity, or Rust/Tauri migration from this batch.
```

## 99. 2026-06-22 G5 Task Parent Goal Evidence Replay

This addendum promotes task/sub-agent parent-goal evidence into G5 shadow
replay. The evidence remains internal to analytix task-job contracts and must
not create a public sub-agent/job surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.parentGoalEvidence` records active-goal requirement and evidence ledger/error event keys. | Not a live Go Job Manager. |
| Product boundary | Replay records `usesReasonixProtocol:false` and `topLevelRouteExposed:false`. | Not Reasonix public sub-agent/job protocol. |
| Go shadow | `BuildG5ShadowSlicesOutput` computes the parent-goal evidence summary from the TS task-job oracle. | Not a default backend or renderer route. |

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
migration from this batch.
```

## 87. 2026-06-22 Go G5 Parallel Task Dependency Executable Shadow

This addendum promotes `parallel_tasks` dependency validation from G5
`jobReplay` summary evidence into executable G5 shadow output. It keeps the
TypeScript runtime authoritative and does not expose a public sub-agent/job
protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 executable control | `controlExecutableCases.taskJobs.parallelValidation` declares a valid `depends_on` plan, five invalid plans, product-boundary booleans, and exact expected errors. | Not a live Go Job Manager. |
| Go shadow | `BuildG5ControlExecutableOutput` runs dependency validation and emits valid order plus invalid-case errors. | Not a default backend or renderer-visible Go route. |
| Product boundary | No Reasonix protocol, Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer route, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 executable shadow covers `parallel_tasks` dependency validation
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow navigation, live Go Job Manager readiness, default Go backend, G6
readiness, packaged desktop sub-agent QA, release readiness, Kun identity, or
Rust/Tauri migration from this batch.
```

## 88. 2026-06-22 Write Sidebar and Connect Phone Placeholder Sovereignty

This addendum strengthens upstream-absorption product-sovereignty gates. It
does not add runtime behavior, a Go backend, or a new product route.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Route-surface scan | Write sidebar, workspace mode tabs, and sidebar projects section are included in forbidden top-level entry checks. | Not a new UI route. |
| Product copy | Newly generated Connect Phone mapped-conversation placeholders use `[Connect Phone:...]`. | Legacy `[Claw:]` recognizers remain compatibility-only. |
| Reusable scan | `scan:product-sovereignty` covers camel/no-hyphen forbidden entry variants plus explicit `window.reasonix` / `reasonixGui` / `agentProvider: "kun"` patterns. | Not packaged desktop QA. |

Allowed wording:

```text
Analytix product-sovereignty scans cover Write/sidebar entry surfaces, and new
Connect Phone placeholders no longer emit `[Claw:...]`.
```

Forbidden wording:

```text
Do not claim all internal compatibility names were renamed, legacy Claw
history support was removed, packaged desktop Connect Phone QA, release
readiness, Kun identity, Reasonix protocol parity, default Go backend, or
Rust/Tauri migration from this batch.
```

## 89. 2026-06-22 HTTP Auto-Plan Payload Boundary Guard

This addendum proves Reasonix-shaped auto-plan payload fields do not become an
analytix public turn contract. Plan mode remains explicit and analytix-owned.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| HTTP start-turn contract | `autoPlan` / `auto_plan` are ignored/stripped and do not set turn `mode` or `guiPlan`. | Not Reasonix public auto-plan config. |
| Plan tool gating | Captured agent-turn model requests do not advertise `create_plan` when only upstream auto-plan fields are present. | Not a controller rebuild protocol. |
| Product boundary | No Reasonix SessionAPI, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route, Kun identity, or Go backend path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix ignores Reasonix-shaped `autoPlan` / `auto_plan` start-turn fields;
only explicit `mode: "plan"` / `guiPlan` can advertise `create_plan`.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user/project auto-plan settings,
controller rebuild parity, packaged Plan mode QA, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 100. 2026-06-22 G5 Planner-Executor Detail Replay

This addendum promotes planner-executor detail evidence into G5 shadow replay.
The evidence remains internal to analytix task-job contracts and must not
create a planner product toggle or public sub-agent/job surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.plannerExecutor` records skipped reason, cancel reason, output-offset job count, and transcript-propagation job count. | Not a live Go Job Manager. |
| Planner/executor propagation | Dependency failure and cancellation details remain tied to the TS task-job oracle. | Not Reasonix planner/sub-agent protocol. |
| Product boundary | No renderer-visible Go route, top-level planner/sub-agent entry, or default backend is added. | Not release readiness. |

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
or Rust/Tauri migration from this batch.
```

## 101. 2026-06-22 G5 Task Tool Contract Boundary Replay

This addendum promotes internal task tool-contract boundary evidence into G5
shadow replay. The evidence must remain an analytix runtime/tool-host contract,
not a public sub-agent/job API or product navigation surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.toolContractBoundary` records `task` and `parallel_tasks` field names and safety gates. | Not a live Go Job Manager. |
| Runtime-only boundary | Both tool contracts remain `internalRuntimeOnly:true` with permission/dependency/planner-read-only gates. | Not Reasonix public task/sub-agent protocol. |
| Product boundary | Replay records `usesReasonixProtocol:false` and `topLevelRouteExposed:false`. | Not a top-level route or default backend. |

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
or Rust/Tauri migration from this batch.
```

## 95. 2026-06-22 Provider Live-Local HTTP Executable Proof

This addendum promotes provider/cache evidence from fake-fetch-only oracle
checks to no-credential local HTTP execution. It strengthens provider request
and usage proof while keeping provider behavior and product surfaces unchanged.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider usage proof | All provider usage oracle cases execute through a local HTTP provider and validate mapped usage fields. | Not credentialed live provider QA. |
| Request-shape proof | All request-shape cases execute through local HTTP while preserving original provider URL/host construction. | Not provider behavior change. |
| Product/runtime boundary | No settings, bridge, runtime route, Go backend, Kun identity, Reasonix protocol, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix provider/cache proof executes every usage and request-shape oracle
case through no-credential local HTTP while preserving provider request
construction.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, provider behavior changes, Reasonix provider protocol parity,
default Go backend, packaged provider settings QA, release readiness, Kun
identity, or Rust/Tauri migration from this batch.
```

## 96. 2026-06-22 G5 Provider Live-Local Proof Summary Replay

This addendum makes the D-0090 provider live-local HTTP proof replayable as
oracle metadata and G5 shadow summary. It strengthens auditability without
turning Go into a provider runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider oracle | `liveLocalHttpProof` records no-credential local HTTP proof scope and no-live-superiority policy. | Not credentialed provider QA. |
| G5 cache replay | `cacheReplay.liveLocalHttpProof` computes usage/request-shape/post counts and endpoint formats from the TS oracle. | Not live Go provider execution. |
| Product/runtime boundary | No provider behavior, settings, bridge, runtime route, default Go backend, Kun identity, Reasonix protocol, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow replays a fixture-owned summary of provider live-local HTTP
proof from the TS-owned provider-cache oracle.
```

Forbidden wording:

```text
Do not claim live Go provider parity, live provider/cache superiority,
credentialed provider matrix readiness, provider behavior changes, Reasonix
provider protocol parity, default Go backend, packaged provider settings QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 98. 2026-06-22 G5 MCP Approval Annotation Replay

This addendum expands MCP approval evidence inside G5 shadow replay. It
strengthens high-risk tool approval auditability without turning Go into the
approval manager or MCP runtime.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| MCP approval replay | `mcpReplay.approvalAnnotations` records normalized tool name, destructive/open-world flags, approval id, deny decision, result kind, and no-execute. | Not a live Go approval manager. |
| Approval safety | Denied destructive/open-world MCP tools remain `executed:false`. | Not credentialed MCP QA. |
| Product/runtime boundary | No MCP route, top-level MCP-indexer entry, default Go backend, Kun identity, Reasonix protocol, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 97. 2026-06-22 G5 MCP Live-Local Indexer Summary Replay

This addendum expands MCP live-local indexer evidence inside G5 shadow replay.
It strengthens MCP/indexer lifecycle auditability while keeping Go shadow-only
and avoiding a public MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| MCP replay | `mcpReplay.liveLocalIndexer` records retry attempts, active paths, tombstone count, snapshot restart, late tombstone, and secret-safe diagnostics. | Not a live Go MCP client. |
| Secret safety | The G5 summary records `leaksSecret:false` and redacted diagnostics. | Not credentialed MCP QA. |
| Product/runtime boundary | No MCP route, top-level MCP-indexer entry, default Go backend, Kun identity, Reasonix protocol, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
Rust/Tauri migration from this batch.
```

## 93. 2026-06-22 Go G5 Provider Streaming Usage Replay

This addendum carries G3 provider streaming/usage evidence into the G5
full-loop shadow summary. It strengthens provider/cache proof while keeping
provider runtime behavior unchanged.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 provider streaming replay | `providerStreamingReplay` records G3 source id, thread id, since-seq, SSE frame count, and event order. | Not a live Go provider client. |
| Usage payload proof | Go shadow parses the fixture SSE `usage` payload and compares it with `deepseek-prompt-cache` expected usage. | Not live provider telemetry. |
| Product boundary | No provider behavior, bridge, settings, renderer route, default Go backend, Reasonix protocol, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays provider streaming usage evidence from
TS-owned G3 fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, live Go provider client readiness, default Go backend, provider
request/stream parser changes, Reasonix provider protocol parity, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 94. 2026-06-22 Go G5 Task-Job Route Executable Replay

This addendum carries internal task-job wait/output/kill route executable
evidence into G5 shadow replay. It strengthens Job Manager readiness evidence
without exposing Go routes or public sub-agent protocols.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Task-job route oracle | `routeExecutable` records unauthorized, output, wait, kill, missing output, and rehydrated cases. | Not a public job protocol. |
| G5 job replay | `jobReplay.routeExecutable` stores route status/offset/result summary computed from the TS-owned oracle. | Not a live Go Job Manager. |
| Product boundary | No Go route server, renderer route, default backend, Reasonix SessionAPI, public Subagent/Workflow route, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 88. 2026-06-22 Go G5 Cache Drift Attribution Replay

This addendum promotes provider-cache drift attribution into G5 shadow replay.
It strengthens Reasonix provider/cache absorption evidence while preserving the
analytix stable-prefix contract.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 cache replay | `cacheReplay.driftAttribution` records previous/current prefix hashes and expected reasons. | Not a live Go provider client. |
| Stable prefix hygiene | `systemHashStable` and `prefixItemsHashStable` prove the fixture drift is not caused by dynamic stable-prefix pollution. | Dynamic workspace context remains outside stable prefix. |
| Drift flags | Go shadow computes tool/provider/model/endpoint changes from TS-owned oracle shapes. | Not a provider protocol migration. |
| Product boundary | No renderer-visible Go route, default backend, Reasonix protocol, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow replay attributes provider-cache drift to allowed
tool/provider/model changes while preserving stable system/prefixItems hashes.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, Reasonix provider protocol parity,
live Go provider client readiness, default Go backend, packaged provider
settings QA, release readiness, Kun identity, dynamic-context stable prefix, or
Rust/Tauri migration from this batch.
```

## 89. 2026-06-22 Go G3 Cache Drift Attribution Replay

This addendum carries provider-cache drift attribution into the G3 provider
streaming/usage/cache gate. It uses the same TS-owned provider-cache oracle as
G5, but proves the lower provider conformance layer can compute the attribution
directly from raw shapes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G3 provider oracle | `cacheDriftAttribution` stores raw previous/current shapes tied to `ProviderCacheOracle`. | Not a live provider matrix. |
| G3 expected output | `expectedOutput.cacheDriftAttribution` stores compact drift replay output. | Not a runtime provider behavior change. |
| Go G3 shadow | `BuildG3ProviderConformanceOutput` computes stable system/prefixItems and changed tool/provider/model/endpoint flags. | Not a default Go backend. |
| Product boundary | No renderer-visible Go route, Reasonix protocol, Kun identity, dynamic stable-prefix material, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
dynamic-context stable prefix, or Rust/Tauri migration from this batch.
```

## 90. 2026-06-22 Go G3 Provider Request Shape Replay

This addendum carries provider request URL/body/tool-shape evidence into the G3
provider streaming/usage/cache gate. It uses the same TS-owned provider-cache
oracle that already drives fake-fetch runtime request assertions.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G3 provider oracle | `requestShapeMatrix` stores expected URL, headers, body fields, endpoint format, and tool shape for each request-shape case. | Not a live provider matrix. |
| G3 expected output | `requestShapeSummary` stores exact URL count, endpoint/tool-shape families, custom full endpoint ids, and body-field family counts. | Not a runtime provider behavior change. |
| Go G3 shadow | `BuildG3ProviderConformanceOutput` computes request-shape summary from raw cases. | Not a default Go backend. |
| Product boundary | No renderer-visible Go route, Reasonix protocol, Kun identity, provider request behavior change, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
Rust/Tauri migration from this batch.
```

## 92. 2026-06-22 Go G5 Session Route Status Replay

This addendum carries G2 thread/session route semantics into the G5 full-loop
`sessionReplay` summary. It strengthens the proof that archive/search/fork/
resume/SSE replay remain analytix-owned HTTP/SSE contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 session replay | `routeStatusReplay` stores route/status counts, archive/search status/counts, read/update fields, fork lineage, resume summary, and SSE replay evidence. | Not live Go thread/session routes. |
| Go G5 shadow | `BuildG5ShadowSlicesOutput` computes route status replay from G2 route cases and SSE frames. | Not a default Go backend. |
| Runtime contract | Archive/search/fork/resume/SSE semantics are visible in both G2 HTTP/SSE oracle and G5 full-loop summary. | Not packaged desktop restart/resume QA. |
| Product boundary | No renderer-visible Go route, Reasonix SessionAPI, Kun identity, bridge/settings fallback, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 full-loop shadow replays thread/session route status and SSE
semantics from TS-owned G2 HTTP/SSE fixtures.
```

Forbidden wording:

```text
Do not claim live Go route readiness, default Go backend, Reasonix SessionAPI
protocol parity, packaged desktop restart/resume QA, release readiness, Kun
identity, deprecated bridge/settings fallback, or Rust/Tauri migration from
this batch.
```

## 91. 2026-06-22 Go G5 Provider Request Shape Replay

This addendum carries provider request URL/body/tool-shape evidence into the G5
full-loop `cacheReplay` summary. It complements the G3 provider-gate replay by
making the same non-regression evidence visible in full-loop shadow output.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 cache replay | `requestShapeReplay` stores exact URL count, endpoint/tool-shape families, custom full endpoint ids, and body-field family counts. | Not a live provider matrix. |
| Go G5 shadow | `BuildG5ShadowSlicesOutput` computes request-shape replay from provider-cache request-shape cases. | Not a default Go backend. |
| Provider non-regression | OpenAI chat/responses, Anthropic messages, and custom full endpoint families are visible in G5 summary. | Not a runtime provider behavior change. |
| Product boundary | No renderer-visible Go route, Reasonix protocol, Kun identity, provider request behavior change, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
Rust/Tauri migration from this batch.
```

## 87. 2026-06-22 Offline Provider/Cache Parity Seal

This addendum promotes fixture-only provider/cache parity evidence into G5
shadow replay. It strengthens DeepSeek prefix/cache and multi-provider
request/usage proof while keeping live provider superiority out of scope.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 cache replay | `cacheReplay.offlineParitySeal` records stable/equivalent DeepSeek prefix equality, hit/miss/rate, usage/request-shape counts, endpoint families, and release guard status. | Not a live provider matrix. |
| Multi-provider regression | OpenAI Responses, Anthropic Messages, and custom endpoint request-shape coverage are counted in the seal. | Not credentialed provider QA. |
| Product/backend boundary | `fixtureOnly: true` and `mayClaimLiveSuperiority: false` remain explicit. | Not a default Go backend or Reasonix provider protocol. |

Allowed wording:

```text
Analytix has fixture-backed offline provider/cache parity evidence for
DeepSeek stable prefix/cache and multi-provider request/usage regressions.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration
from this batch.
```

## 83. 2026-06-22 Go G5 Task Job Lifecycle Replay

This addendum promotes base `task` lifecycle evidence into G5 shadow replay.
It strengthens task/sub-agent orchestration evidence while keeping all task-job
surfaces internal.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.lifecycle` records foreground completion, background cross-turn output/final completion, and wait/output/kill cancellation. | Not a live Go Job Manager. |
| Task-job route safety | Killed status and cancellation error are pinned from the TS task-job oracle. | Not a public job route protocol. |
| Product boundary | No Subagent/Workflow/Create Loop/AutoResearch route, Reasonix protocol, Go route, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
migration from this batch.
```

## 84. 2026-06-22 Go G5 Task Job Route Boundary Replay

This addendum promotes task-job route auth and forbidden top-level route
evidence into G5 shadow replay. It strengthens job orchestration evidence while
keeping all task-job routes internal.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.routeBoundary` records protected wait/output/kill ids, unauthorized `401`, and forbidden top-level route paths. | Not live Go task-job routes. |
| Product boundary | `/v1/workflow`, `/v1/create-loop`, and `/v1/autoresearch` are replayed as forbidden top-level surfaces. | Not a public Subagent protocol. |
| Backend boundary | No Go route, default backend, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
migration from this batch.
```

## 85. 2026-06-22 Go G5 MCP Core Lifecycle Replay

This addendum promotes MCP connect/disconnect/reload/cancel/error evidence into
G5 shadow replay. It strengthens MCP lifecycle evidence while keeping
MCP-indexer internal and shadow-only.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 MCP replay | `mcpReplay.lifecycle` records connect/disconnect diagnostics, reload schema stability, cancel no-execute, and approved error shape. | Not a live Go MCP client. |
| MCP lifecycle safety | Cancel-before-start remains no-execute and approved execution failure is distinct from denied approval. | Not a credentialed MCP matrix. |
| Product boundary | No MCP-indexer route, Reasonix protocol, Go route, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
Rust/Tauri migration from this batch.
```

## 87. 2026-06-22 Go G5 MCP Refresh Catalog Drift Replay

This addendum promotes MCP search catalog refresh drift into TS-owned
conformance and G5 shadow replay. It keeps MCP search/currentness internal to
analytix runtime contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| MCP refresh currentness | `mcp_refresh_catalog` is tested after fake-client catalog expansion and must return `catalogDrift: true`. | Not Reasonix MCP-indexer protocol. |
| G5 MCP replay | `mcpReplay.searchRefreshDrift` records server id, initial/expanded tool names, total indexed count, and no top-level route exposure. | Not a live Go MCP client. |
| Product boundary | No MCP-indexer route, Reasonix protocol, Go route, default backend, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow replay covers MCP refresh catalog drift from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer public lifecycle parity, top-level
MCP-indexer navigation, live Go MCP client readiness, default Go backend,
credentialed MCP matrix readiness, packaged desktop MCP QA, release readiness,
Kun identity, deprecated bridge/settings fallback, or Rust/Tauri migration from
this batch.
```

## 88. 2026-06-22 Thread/SSE Route Auth Replay

This addendum promotes missing-token runtime route rejection into the shared
G2/G5 oracle path. It strengthens thread/session/SSE parity while keeping all
route control behind analytix-owned HTTP/SSE contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G2 route replay | `/v1/threads/:id/events` without a bearer token returns structured 401 JSON. | Not Reasonix SessionAPI. |
| G5 session replay | `routeStatusReplay.auth` records protected route id/path/status/body code and zero SSE frames. | Not a live Go HTTP server. |
| Product boundary | No renderer-visible Go route, default backend, Reasonix protocol, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix route replay proves thread/SSE routes reject missing runtime tokens,
and G5 shadow replays that auth boundary from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI parity, unauthenticated SSE replay, live Go
HTTP route readiness, default Go backend, packaged desktop restart/resume QA,
release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration from this batch.
```

## 89. 2026-06-22 Runtime Settings Legacy-Agent Pollution Guard

This addendum pins the active settings schema boundary. Reasonix/Kun-shaped
agent settings may be read only in explicit migration/import contexts; they
must not survive inside saved top-level `runtime`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime normalization | Runtime-contained `agent`, `agentProvider`, `agents`, `deepseek`, and `reasonix` keys are stripped before active runtime merge/migration spread. | Not Reasonix config parity. |
| IPC patch schema | Settings patches reject legacy agent-shaped keys inside `runtime`. | Not a fallback save path. |
| Product boundary | No bridge alias, old settings envelope, Reasonix public protocol, Kun identity, default Go backend, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix active runtime settings strip or reject Reasonix/Kun-shaped agent
settings fields while preserving top-level `runtime` ownership.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, Kun legacy agent settings
compatibility as an active save path, deprecated bridge/settings fallback,
packaged settings QA, release readiness, default Go backend, or Rust/Tauri
migration from this batch.
```

## 90. 2026-06-22 Task-Job Route Executable Control Shadow

This addendum upgrades internal task-job route executable evidence from G5
summary replay into G5 control executable replay. The active runtime boundary
remains `analytix serve`; no Go route is exposed to the renderer.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.routeExecutable` mirrors the TS-owned route executable oracle. | Not a public job API. |
| Go shadow | `BuildG5ControlExecutableOutput` computes unauthorized/output/wait/kill/missing/rehydrated route status output. | Not a live Go Job Manager. |
| Product boundary | The executable output carries `usesReasonixProtocol: false` and `topLevelRouteExposed: false`. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay task-job route status behavior from
TS-owned fixtures without exposing Go or Reasonix routes.
```

Forbidden wording:

```text
Do not claim live Go task-job route readiness, Reasonix SessionAPI/job protocol
parity, renderer-visible Go routes, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 91. 2026-06-22 Task-Job Lifecycle Control Shadow

This addendum upgrades task-job lifecycle evidence from G5 summary replay into
G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go route is exposed to the renderer.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.lifecycle` mirrors foreground/background/wait-output-kill task-job lifecycle evidence. | Not a public lifecycle API. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed lifecycle output. | Not a live Go Job Manager. |
| Product boundary | The executable output carries `usesReasonixProtocol: false` and `topLevelRouteExposed: false`. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay task-job lifecycle behavior from
TS-owned fixtures without exposing Go or Reasonix lifecycle routes.
```

Forbidden wording:

```text
Do not claim live Go task-job lifecycle readiness, Reasonix SessionAPI/job
protocol parity, renderer-visible Go routes, default Go backend, packaged
desktop QA, release readiness, Kun identity, or Rust/Tauri migration from this
batch.
```

## 92. 2026-06-22 Planner-Executor Control Shadow

This addendum upgrades planner/executor propagation evidence from G5 summary
replay into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go route is exposed to the renderer.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.taskJobs.plannerExecutor` mirrors planner/executor failure, cancellation, output-offset, and transcript-propagation evidence. | Not a public planner API. |
| Go shadow | `BuildG5ControlExecutableOutput` computes typed planner/executor output. | Not a live Go Job Manager. |
| Product boundary | The executable output carries `usesReasonixProtocol: false` and `topLevelRouteExposed: false`. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay planner/executor propagation from
TS-owned fixtures without exposing Go or Reasonix planner/job routes.
```

Forbidden wording:

```text
Do not claim live Go planner/job readiness, Reasonix SessionAPI/planner protocol
parity, renderer-visible Go routes, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 93. 2026-06-22 Provider Cache Release Guard Control Shadow

This addendum upgrades provider/cache release-guard evidence from G5 summary
replay into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerCacheReleaseGuard` mirrors the provider/cache release guard. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes tail averages, allowed-low cases, collapse counts, and status. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider cache release guard from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 94. 2026-06-22 Provider Usage Parser Control Shadow

This addendum upgrades provider usage parser precedence from G5 summary replay
into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerUsageParser` mirrors provider usage parser inputs and expected output. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes DeepSeek native precedence, OpenAI Responses cached tokens, Anthropic cache fields, and unsupported fallback. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider usage parser precedence
from TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 95. 2026-06-22 Provider Request Shape Control Shadow

This addendum upgrades provider request URL/body/tool-shape replay from G5
summary replay into G5 control executable replay. The active runtime boundary
remains `analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerRequestShape` mirrors provider request-shape inputs and expected output. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes exact URL count, endpoint families, custom full endpoint ids, tool shapes, and body-field family counts. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider request shape from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 96. 2026-06-22 Provider Cache Accounting Control Shadow

This addendum upgrades provider cache accounting from G5 summary replay into
G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerCacheAccounting` mirrors provider cache accounting inputs and expected output. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes telemetry-supported ids, unsupported ids, provider-family ids, hit/miss totals, aggregate hit rate, and unsupported fallback behavior. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider cache accounting from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 97. 2026-06-22 Release/Package Identity Sovereignty Guard

This addendum extends product-sovereignty evidence from runtime/UI source into
release and package configuration. It does not add product functionality.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Product scan | `scan:product-sovereignty` scans package manifests, builder config, scripts, workflows, and build config for Kun/Reasonix identity leaks. | Not packaged app runtime QA. |
| Packaging test | `packaging-config.test.ts` asserts root package/product name, app id, artifact template, NSIS names, and runtime bin. | Not signing/notarization proof. |
| Product boundary | Release/package identity stays analytix-owned. | No Kun/Reasonix identity or public protocol is accepted. |

Allowed wording:

```text
Analytix release/package identity is guarded by source scans and packaging
configuration tests.
```

Forbidden wording:

```text
Do not claim packaged Electron smoke, signing/notarization readiness, release
readiness, Kun/Reasonix public identity acceptance, default Go backend,
renderer-visible Go route, or Rust/Tauri migration from this batch.
```

## 98. 2026-06-22 Provider Streaming Control Shadow

This addendum upgrades provider streaming usage replay from G5 summary replay
into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerStreaming` mirrors G3 provider streaming inputs and expected output. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes SSE event order, usage match, token/cache fields, cache hit rate, telemetry support, and product boundary. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider streaming usage from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider streaming matrix parity, Reasonix provider/cache protocol
parity, renderer-visible Go routes, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 99. 2026-06-22 Provider Offline Parity Seal Control Shadow

This addendum upgrades provider offline parity seal replay from G5 summary
replay into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go provider backend is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerOfflineParitySeal` mirrors minimal provider-cache oracle inputs and expected output. | Not a live provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes stable prefix equivalence, DeepSeek cache telemetry, request-shape coverage, release guard status, and fixture-only policy. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider offline parity seal from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 100. 2026-06-22 Session Route Status Control Shadow

This addendum upgrades thread/session route status replay from G5 summary
replay into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no Go thread/session route is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.sessionRouteStatus` mirrors G2 route replay inputs and expected output. | Not live Go HTTP routing. |
| Go shadow | `BuildG5ControlExecutableOutput` computes archive/search, read/update, fork, resume, SSE replay/caught-up, and auth status. | Not a live Go route server. |
| Product boundary | The control case is fixture-only and does not expose Reasonix SessionAPI/job protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay thread/session route status from
TS-owned fixtures without exposing Go or Reasonix session routes.
```

Forbidden wording:

```text
Do not claim live Go thread/session routes, Reasonix SessionAPI/job protocol
parity, renderer-visible Go routes, default Go backend, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 101. 2026-06-22 Provider Live-Local HTTP Control Shadow

This addendum upgrades provider live-local HTTP proof from G5 summary replay
into G5 control executable replay. The active runtime boundary remains
`analytix serve`; no live Go provider client is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerLiveLocalHttpProof` mirrors minimal provider-cache oracle inputs and expected output. | Not a credentialed provider matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes local HTTP fixture policy, usage/request-shape counts, expected POST count, endpoint formats, and provider families. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider live-local HTTP proof from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom provider matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 102. 2026-06-22 Provider Drift Attribution Control Shadow

This addendum upgrades provider drift attribution from G5 summary replay into
G5 control executable replay. The active runtime boundary remains
`analytix serve`; no live Go provider client is enabled.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control fixture | `controlExecutableCases.providerDriftAttribution` mirrors provider-cache drift inputs and expected output. | Not a live cache matrix. |
| Go shadow | `BuildG5ControlExecutableOutput` computes prefix hashes, stable system/prefix-items hashes, tools/provider/model/endpoint drift, expected reasons, and telemetry status. | Not a live Go provider client. |
| Product boundary | The control case is fixture-only and does not expose Reasonix provider/cache protocol. | Default Go backend remains rejected. |

Allowed wording:

```text
Analytix Go G5 shadow can executable-replay provider drift attribution from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed DeepSeek/OpenAI/Anthropic/
custom cache matrix parity, Reasonix provider/cache protocol parity,
renderer-visible Go routes, default Go backend, packaged desktop QA, release
readiness, Kun identity, or Rust/Tauri migration from this batch.
```

## 86. 2026-06-22 Go G5 Approval Decision Route Replay

This addendum promotes approval decision route and replay-order evidence into
G5 shadow replay. It strengthens approval/user-input conformance without
exposing Reasonix ask/session protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 approval replay | `approvalUserInputReplay` records approval id, deny decision, denied status, pending-after cleanup, duplicate `409`, replay cursor, and replay kinds. | Not a live Go approval manager. |
| Replay safety | Approval and user-input requested/resolved kinds remain pinned in order from the TS oracle. | Not Reasonix ask protocol. |
| Product boundary | No Reasonix protocol, Go route, default backend, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

Allowed wording:

```text
Analytix G5 shadow replay covers approval decision route evidence from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 81. 2026-06-22 Go G5 MCP Search Meta-Tool Replay

This addendum promotes MCP search meta-tool trust/no-execute evidence into G5
shadow replay. It keeps MCP search internal and does not create a public
MCP-indexer surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 MCP replay | `mcpReplay` records meta-tool names, trusted tool id, unknown-tool error, untrusted search count, call policy, and denied no-execute. | Not a live Go MCP client. |
| Workspace trust | Untrusted workspace search remains `0` searched tools. | Not Reasonix MCP-indexer protocol. |
| Approval gate | `mcp_call` remains `on-request`, and denied calls do not execute the MCP client. | Not approval bypass or credentialed MCP QA. |

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
Rust/Tauri migration from this batch.
```

## 82. 2026-06-22 Go G5 Parallel Task Dependency Validation Replay

This addendum promotes `parallel_tasks` dependency validation into G5 shadow
replay. It strengthens sub-agent/job orchestration evidence while keeping task
jobs internal.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 job replay | `jobReplay.parallelValidation` records `depends_on`, valid order, invalid case ids, and error strings. | Not a live Go Job Manager. |
| Dependency safety | Single-task, duplicate-id, self-dependency, cycle, and unknown-dependency cases are all pinned. | Not a public Subagent protocol. |
| Product boundary | No Subagent/Workflow/Create Loop route, Reasonix protocol, Go route, Kun identity, or Rust/Tauri path changed. | Release readiness remains blocked. |

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
migration from this batch.
```

## 80. 2026-06-22 Go G5 Abort Cleanup Replay Summary Closure

This addendum closes the replay-summary side of the abort-cleanup G5 proof.
Executable shadow and summary/replay output must describe the same
approval/user-input cleanup semantics.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 control replay | `controlReplay.abortCleanup` records expired/cancelled statuses, late `409`/`404`, no pending gates, replay kinds, and product-boundary booleans. | Not a live Go gate manager. |
| Approval/user-input replay | `approvalUserInputReplay` includes abort-cleanup ids/statuses, late statuses, pending cleanup, and replay kinds. | Not Reasonix ask protocol. |
| Go shadow | `BuildG5ShadowSlicesOutput` computes the replay-summary fields from the TS approval/user-input oracle. | Not a default backend. |

Allowed wording:

```text
Analytix G5 shadow summary and executable output both cover
approval/user-input abort cleanup from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration from
this batch.
```

## 116. 2026-06-22 Desktop Bridge IPC Sovereignty Shadow

This addendum extends the existing desktop sovereignty control shadow so
preload runtime request and SSE IPC channels are part of the G5 executable
oracle.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime request IPC | `controlExecutableCases.desktopSovereignty.runtimeIpcChannels.request` is `runtime:request`, and Go shadow computes `runtimeRequestUsesAnalytixIpc: true`. | Not a Reasonix frontend protocol. |
| SSE IPC | `runtime:sse:start/stop/event/end/error` are recorded and Go shadow computes `runtimeSseUsesAnalytixIpc: true`. | Not a renderer-visible Go route. |
| Public API ownership | Shared API types and facade domains remain analytix-owned, with no Kun/Reasonix public API names. | Not a deprecated bridge fallback. |

Allowed wording:

```text
Analytix G5 shadow covers the desktop runtime request/SSE IPC bridge through
`window.analytix.runtime` without exposing Reasonix or Kun bridge surfaces.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/frontend protocol parity, renderer-visible Go
routes, default Go backend readiness, packaged desktop bridge QA, release
readiness, Kun identity, deprecated settings fallback, or Rust/Tauri migration
from this batch.
```

## 117. 2026-06-22 Desktop Main IPC Boundary Shadow

This addendum adds a main-process IPC boundary control shadow so preload bridge
proof is paired with main handler/schema proof.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime request IPC | Main `runtime:request` parses `runtimeRequestPayloadSchema`, allows only modeled analytix routes, and rejects forbidden public routes before runtime calls. | Not a Reasonix SessionAPI route protocol. |
| SSE main IPC | Main `runtime:sse:start/stop` keeps strict thread cursor payloads, `/v1/threads/{id}/events`, matching stream-id stop, reconnect cursor, and 100ms batching. | Not a live Go SSE server. |
| Product boundary | `/v1/runtime/go`, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes remain forbidden. | Not a default Go backend. |

Allowed wording:

```text
Analytix G5 shadow covers main runtime request/SSE IPC boundary enforcement
from source-derived main-process fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/main protocol parity, renderer-visible Go
routes, default Go backend readiness, packaged desktop bridge QA, release
readiness, Kun identity, deprecated settings fallback, or Rust/Tauri migration
from this batch.
```

## 118. 2026-06-22 Renderer Route-Surface Sovereignty Shadow

This addendum adds renderer route-surface sovereignty to the G5 executable
oracle. The proof is source-derived from Workbench route-surface tests, route
types, browser preview bridge, plugin marketplace, shell navigation, Workbench
shell, and base shell CSS.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Top-level app route union | `controlExecutableCases.rendererRouteSurfaceSovereignty` proves `AppRoute` remains `chat/write/settings/plugins/claw/schedule`. | Not permission to add Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation. |
| Dormant workflow quarantine | Workflow/Create Loop source may exist only as quarantined dormant code and has no top-level entrypoint symbol hit. | Not a hidden product route. |
| Renderer bridge/shell ownership | Browser preview installs only `window.analytix`; plugin marketplace safe-area state and shell no-drag/safe-inset controls are preserved. | Not Reasonix SessionAPI/frontend protocol or Kun bridge fallback. |

Allowed wording:

```text
Analytix G5 shadow covers renderer route-surface sovereignty and proves
forbidden hidden-capability entries remain absent.
```

Forbidden wording:

```text
Do not claim Reasonix UI/session protocol parity, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer navigation, renderer-visible Go routes,
default Go backend readiness, packaged desktop route QA, release readiness, Kun
identity, deprecated settings fallback, or Rust/Tauri migration from this
batch.
```

## 119. 2026-06-22 Package / Runtime CLI Identity Shadow

This addendum adds package/runtime CLI identity to the G5 executable oracle.
The proof is source-derived from package manifests, electron-builder config,
app identity, AppUserModelID, runtime binary resolver, runtime CLI, afterPack
validation, packaging tests, and release workflow.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime CLI | `controlExecutableCases.packageRuntimeIdentity` proves runtime bin remains `analytix -> ./dist/cli/serve-entry.js` and CLI usage remains `analytix serve [options]`. | Not a Reasonix CLI/session protocol. |
| Release identity | Root package/product, builder app id, artifact/NSIS names, app product name, and Windows AppUserModelID remain analytix-owned. | Not release readiness or packaged artifact QA. |
| Bundled runtime validation | afterPack requires `packages/runtime/dist/cli/serve-entry.js`, and release env ownership stays `ANALYTIX_*`. | Not a default Go backend or Go launcher. |

Allowed wording:

```text
Analytix G5 shadow covers package/runtime CLI identity and proves the public
runtime contract remains `analytix serve`.
```

Forbidden wording:

```text
Do not claim Reasonix CLI/session protocol parity, packaged artifact QA,
release readiness, renderer-visible Go routes, default Go backend readiness,
Kun/DeepSeek product identity, or Rust/Tauri migration from this batch.
```

## 120. 2026-06-22 Runtime HTTP Route Sovereignty Shadow

This addendum adds runtime HTTP/SSE route sovereignty to the G5 executable
oracle. The proof is source-derived from the active `analytix serve` route
table, router matching, HTTP server not-found behavior, shared endpoint
templates, and HTTP server tests.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Route inventory | `controlExecutableCases.runtimeHttpRouteSovereignty` proves the route table is exactly 44 entries, with `/health` as the only unauthenticated route and all runtime routes owned by `/v1/*`. | Not a Reasonix SessionAPI or route protocol. |
| Thread/SSE and gate routes | Thread lifecycle, resume-thread, approval, user-input, and `/v1/threads/:id/events` SSE routes remain present through analytix-owned endpoint templates. | Not a renderer-visible Go route or live Go server. |
| Internal runtime controls | Task-job wait/output/kill routes remain authenticated internal runtime endpoints, and singular `/v1/user-input/:id` is compatibility-only. | Not top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation. |
| Forbidden route scan | Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route tokens remain absent from the active route table. | Negative fixtures may still record forbidden strings as evidence. |

Allowed wording:

```text
Analytix G5 shadow covers runtime HTTP/SSE route sovereignty and proves the
public `analytix serve` route table remains analytix-owned.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 156. 2026-06-23 Go Minimal Agent Loop Skeleton

This addendum defines the D-0237 scope for advancing Go from durable
event/session replay into a conformance-only minimal loop proof. The live
TypeScript runtime remains authoritative; Go must stay behind
`/v1/conformance/loop/*` and must not connect to Electron, renderer, settings,
or `analytix serve`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| TS-owned oracle | `go-minimal-agent-loop-oracle.json` covers a single user turn, stable prefix/tool fingerprint, model request shape, DeepSeek/OpenAI/Anthropic cache telemetry, assistant reasoning/text, tool call/result, approval denial, submitted/cancelled user input, MCP catalog visibility, step limit, cancel, resume, and durable recovered-state equality. | Not a live provider/MCP/gate backend. |
| Go loop skeleton | `live_local_loop.go` exposes only `/v1/conformance/loop/*`, consumes fixture data, writes every event through the D-0236 temp durable store, and replays through JSON/SSE. | Not a production Job Manager or default backend. |
| Product boundary | Durable and loop routes now return route-specific nested boundaries so durable/loop proofs show `tempDurableStorePrototype:true`; generic G2/G3/G4 sidecar boundary remains unchanged. | Not a renderer-visible Go route or settings switch. |
| TS sidecar conformance | `go-minimal-agent-loop-conformance.test.ts` starts the Go sidecar and drives HTTP/SSE endpoints, while `go-runtime-conformance.test.ts` keeps source/fixture guard coverage. | Not packaged desktop QA or release readiness. |

Allowed wording:

```text
Analytix D-0237 adds a conformance-only Go minimal agent loop proof that
combines G2 state, G3 provider/cache fixture, G4 gate/MCP fixture, and the
D-0236 temp durable store behind analytix-owned routes.
```

Forbidden wording:

```text
Do not claim live Go agent backend readiness, live provider superiority, live
MCP parity, approval/user-input execution parity, default Go backend readiness,
G6 readiness, release readiness, Reasonix public protocol/SessionAPI parity,
renderer-visible Go routes, Electron integration, settings switches, or
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation
from this batch.
```

## 125. 2026-06-23 Go Temp Durable Store / SSE Replay Proof

This addendum pins D-0236 as an opt-in temp durable event/session store proof.
It strengthens the Go runtime base without changing the production TypeScript
runtime, Electron integration, or product surface.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Temp durable event log | Go appends newline-terminated `events.jsonl` only under a caller-provided temp dir and rejects non-temp durable roots. | Not production workspace storage. |
| Seq and replay | Per-thread `seq` is stable under concurrent writes; `highestSeq()` returns persisted max; `loadEventsSince()` filters, sorts, skips malformed lines, and returns diagnostics. | Not a full Go agent loop or production session store. |
| SSE cursor behavior | Go conformance routes prove caught-up replay and `Last-Event-ID` / `since_seq` equivalence. | Not a live GUI SSE server or packaged restart drill. |
| Thread/session state | Temp durable list/search/archive/fork/resume state is exercised through local conformance routes. | Not Electron route integration or `analytix serve` replacement. |
| Runtime recovery state | Durable replay preserves DeepSeek/OpenAI/Anthropic cache accounting, approval/user-input gate state, answer-free submitted user-input events, and MCP tool-catalog recovery. | Not live provider calls, live MCP client, approval execution, credentialed MCP matrix, or persisted user answers. |
| TS->Go proof | Historical D-0236 Vitest launched `cmd/conformance-sidecar`; current conformance naming is `cmd/contract-sidecar`, and production startup uses `cmd/runtime-server`. | Not source-only evidence; not a production entrypoint. |

Allowed wording:

```text
Analytix Go live-local evidence now includes D-0236 temp durable
event/session storage and SSE replay proof under conformance-only routes.
```

Forbidden wording:

```text
Do not claim production Go runtime replacement, live Go agent loop, live
provider/MCP/gate backend parity, renderer-visible Go routes, default Go
backend readiness, G6 readiness, Reasonix SessionAPI/public protocol parity,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged restart QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 124. 2026-06-23 Go Live-Local G4 Approval/User-Input/MCP Manager Proof

This addendum pins D-0235 as a test/conformance-only Go live-local proof. It
extends the sidecar harness with G4 manager replay while preserving the
analytix runtime boundary and product sovereignty rules.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G4 conformance routes | Go exposes fixture-backed manager routes only below `/v1/conformance/g4/manager/*`. | Not part of the active `analytix serve` route table. |
| Approval replay | Deny decision status/body, pending-before/after, second decision status, replay kinds, and denied no-execute evidence come from the TS approval/user-input oracle. | Not live Go approval execution parity. |
| User-input replay | Cancel and submit status/body, invalid structured-choice cases, no-gate invalid behavior, HTTP answer echo, answer-free resolved event evidence, and remote `disableUserInput` preservation are replayed from fixtures. | Not a production Go user-input backend. |
| MCP replay | Connect/reload/disconnect/cancel/error, approval annotations, search meta-tools, untrusted workspace hiding, unknown tool error, call-time reconnect, background reconnect, known override diagnostics, and diagnostics redaction are replayed from the TS MCP lifecycle oracle. | Not a live Go MCP client or credentialed MCP matrix. |
| Isolation counters | `LiveLocalSidecarSnapshot` records G4 stub replay counters while approval execution, tool execution, MCP connection, credential read, file mutation, `events.jsonl` write, and real workspace write attempts remain `0`. | Not durable Go state or real workspace mutation. |

Allowed wording:

```text
Analytix Go live-local evidence has advanced to D-0235 G4 isolated
approval/user-input/MCP manager replay under conformance-only routes.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, live approval execution parity,
live user-input backend parity, renderer-visible Go routes, default Go backend
readiness, G6 readiness, Reasonix SessionAPI/public control-plane parity,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
credentialed MCP matrix, packaged desktop QA, release readiness, Kun identity,
deprecated settings fallback, or Rust/Tauri migration from this batch.
```

## 122. 2026-06-23 Go Live-Local Sidecar Prototype

This addendum starts the first Go live-local sidecar prototype while keeping
the TypeScript runtime and analytix desktop contracts authoritative.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Live-local sidecar | `NewLiveLocalSidecarHandler` starts in Go tests and serves G1 `/health`, `/v1/runtime/info`, and `/v1/runtime/tools` from the TS oracle. | Not `analytix serve`, not Electron main, and not default backend. |
| Read-only route replay | G2 `GET` thread list/search/read and SSE replay cases are served from fixture data; non-GET G2 routes are filtered out. | Not archive/fork/resume mutation, provider call, approval execution, MCP credential, or file mutation. |
| SSE mock | Fixture frames are returned with `text/event-stream` and compared exactly by Go tests. | Not a durable Go event store, live bus, heartbeat proof, or packaged SSE QA. |
| Rollback gate | G5 guards keep `electronMainConnected:false`, `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`. | G6/backend selection remains pending. |

Allowed wording:

```text
Analytix Go runtime work has entered a test/conformance-only live-local
sidecar prototype for the G1/G2 read-only fixture slice.
```

Forbidden wording:

```text
Do not claim live Go backend readiness, default Go backend, G6 readiness,
Electron integration, renderer-visible Go routes, Reasonix SessionAPI/public
protocol parity, live provider/cache superiority, live MCP/client credential
parity, approval/user-input gate parity, file mutation parity, packaged desktop
QA, release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration from this batch.
```

## 123. 2026-06-23 Go Live-Local Isolated Mutating G2 Lifecycle Prototype

This addendum advances the D-0232 sidecar only inside the
test/conformance harness. The TypeScript runtime and desktop contracts remain
authoritative.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Isolated mutable G2 store | `NewLiveLocalSidecarHarness` starts a Go `httptest.Server` with an in-memory fixture store and exposes a `LiveLocalSidecarSnapshot` for tests. | Not a durable Go store, not Electron main, not `analytix serve`, and not default backend. |
| Mutating lifecycle routes | The live-local sidecar serves the four G2 mutating oracle routes: archive PATCH, title/workspace PATCH, side fork POST, and resume-thread POST with TS-owned status/body shape. | Not approval execution, provider calls, MCP credentials, file mutation, or a general route backend. |
| Isolation proof | Go tests verify mutation ids, archived thread state, fork id, and resumed session id stay in the harness snapshot, while a temporary `events.jsonl` sentinel remains unchanged. | Not real workspace mutation, not real event persistence, and not packaged desktop QA. |
| Rollback gate | Product boundary keeps `electronMainConnected:false`, `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`. | G6/backend selection remains pending. |

Allowed wording:

```text
Analytix Go live-local evidence improved from a read-only G1/G2 prototype to a
test/conformance-only isolated mutating G2 lifecycle prototype.
```

Forbidden wording:

```text
Do not claim G6 readiness, default Go backend, Electron integration,
renderer-visible Go routes, durable Go thread/session storage, Reasonix
SessionAPI/public protocol parity, live provider/MCP/gate/file mutation parity,
packaged desktop QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 123A. 2026-06-23 Go Live-Local G3 Provider/Cache Streaming Prototype

This addendum advances the D-0233 sidecar only inside the test/conformance
harness. The TypeScript provider/cache oracle, runtime contracts, and desktop
contracts remain authoritative.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Fixture-backed G3 provider harness | `NewLiveLocalSidecarHarness` accepts a `ProviderOracle` and serves only local `/v1/conformance/g3/provider/*` routes from TS-owned fixtures. | Not a production provider client, not `analytix serve`, not Electron main, and not default backend. |
| Usage/cache replay | The harness parses the 5 provider usage cases for unsupported unknown telemetry, DeepSeek native hit/miss and precedence, OpenAI Responses cached tokens, and Anthropic cache read/create fields. | Not a credentialed provider matrix, not live cache telemetry, and not live provider/cache superiority. |
| Request-shape replay | The harness recomputes all 7 DeepSeek/OpenAI-compatible/Responses/Anthropic/custom full endpoint URL/header/body/tool-shape cases, including exact custom full endpoints with no appended path. | Not a provider request behavior change in the TypeScript runtime. |
| Streaming replay | The harness returns fixture SSE frames in `item_delta -> usage -> turn_completed` order and validates the usage payload against `deepseek-prompt-cache`. | Not a durable Go event recorder, live bus, heartbeat proof, or packaged SSE QA. |
| Stable-prefix diagnostics | The harness exposes bounded cache hashes and provider/model/endpoint attribution while keeping prompt, tool text, API-key, and Authorization substrings out of diagnostics. | Dynamic workspace state, credentials, and Reasonix sidecar material remain outside the stable prefix. |
| Rollback gate | Product boundary keeps external network, API-key read, provider credentials, provider live calls, `electronMainConnected`, `defaultGoBackendEnabled`, and `rendererVisibleGoRoutesAllowed` false. | G6/backend selection remains pending. |

Allowed wording:

```text
Analytix Go live-local evidence improved from a G2 lifecycle prototype to a
test/conformance-only G3 provider/cache streaming prototype.
```

Forbidden wording:

```text
Do not claim G6 readiness, default Go backend, Electron integration,
renderer-visible Go routes, live external provider calls, credentialed provider
matrix readiness, live provider/cache superiority, Reasonix provider/cache
protocol parity, packaged provider QA, release readiness, Kun identity,
deprecated settings fallback, or Rust/Tauri migration from this batch.
```

## 124. 2026-06-22 Renderer Runtime Endpoint Builder Proof

This addendum extends shared endpoint builder proof into the GUI runtime
provider. Renderer runtime requests must remain derived from analytix-owned
constants/builders so GUI code cannot drift into upstream route identity.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Renderer root runtime paths | Connect/list/create-thread root calls use shared `ANALYTIX_HEALTH_PATH` and `ANALYTIX_THREADS_PATH`. | Not a route behavior change. |
| Renderer dynamic route encoding | Thread, turn, approval, user-input, and session ids containing `/`, query, and fragment text are encoded before `runtimeRequest`. | Not a Reasonix SessionAPI bridge. |
| Renderer forbidden route guard | Runtime provider requests remain `/health` or `/v1/*` and are checked against forbidden upstream/hidden route tokens. | Not top-level product navigation. |

Allowed wording:

```text
Analytix renderer runtime provider calls use shared endpoint constants/builders
and URL-encode dynamic route ids before crossing the bridge.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 125. 2026-06-22 Main IPC Endpoint Builder Allow-list Proof

This addendum closes the renderer-to-main endpoint builder chain. The main IPC
runtime request schema must accept paths produced by shared analytix builders
after URL encoding, while rejecting raw extra-segment drift and compatibility
routes that are not part of the desktop bridge contract.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Main IPC encoded builder acceptance | `runtimeRequestPayloadSchema` accepts shared builder output for thread, turn, checkpoint, approval, user-input, session, attachment, and memory paths. | Not a runtime behavior change. |
| Main IPC raw path rejection | Unencoded ids that create extra route segments are rejected before reaching the runtime adapter. | Not a Reasonix SessionAPI bridge. |
| Compatibility route boundary | Singular `/v1/user-input/:id` remains server compatibility only and is rejected by the main IPC shared allow-list. | Not a public desktop contract. |

Allowed wording:

```text
Analytix main IPC runtime request allow-list accepts encoded shared endpoint
builder paths and rejects raw extra-segment or singular compatibility drift.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 126. 2026-06-22 Main IPC Runtime Adapter Handoff Proof

This addendum proves the registered main IPC handler preserves the validated
analytix path when handing off to the runtime adapter. Validation and handoff
must remain a single product-owned chain: shared builder, renderer bridge, main
schema, adapter call.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime adapter path handoff | Encoded shared endpoint builder paths are passed to `runtimeRequest` unchanged. | Not a runtime behavior change. |
| Method/body preservation | Handler tests prove method and body travel with the validated path. | Not a Reasonix SessionAPI bridge. |
| Invalid route no-forward | Raw extra-segment and singular user-input compatibility paths reject before adapter invocation. | Not a public desktop contract. |

Allowed wording:

```text
Analytix main IPC handler preserves encoded shared endpoint paths when handing
requests to the runtime adapter and blocks invalid paths before adapter calls.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 127. 2026-06-22 Preload Runtime Request Bridge Proof

This addendum proves the renderer-facing preload facade passes runtime request
arguments to the main IPC channel without rewriting the encoded path. It
completes the source-level chain from renderer endpoint builders through
preload and main IPC handler proof.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Preload runtime request handoff | `api.runtime.runtimeRequest` invokes `runtime:request` with encoded path, method, and body unchanged. | Not a runtime behavior change. |
| Preload runtime restart handoff | `api.runtime.restartRuntime` invokes the analytix-owned `runtime:restart` IPC channel. | Not a Reasonix control protocol. |
| Diagnostics compatibility | Diagnostics runtime request fallback uses the same analytix IPC channel and payload shape. | Not a Reasonix SessionAPI bridge. |
| Bridge identity | The executable test loads only the exposed `analytix` API. | Not a deprecated bridge alias. |

Allowed wording:

```text
Analytix preload runtime bridge preserves encoded shared endpoint paths into
the main `runtime:request` IPC channel and exposes restart through
`runtime:restart`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 128. 2026-06-22 Runtime Host URL Handoff Proof

This addendum proves the final TypeScript desktop runtime request hop:
`runtimeRequestViaHost` must preserve encoded analytix paths when constructing
the managed runtime HTTP URL.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime host encoded URL | Encoded shared endpoint path segments and encoded query values reach the local runtime host unchanged. | Not a live Go HTTP server. |
| Request shape preservation | POST method, bearer auth, custom header, JSON content type, and body are preserved. | Not a Reasonix SessionAPI bridge. |
| Runtime ownership | The target remains `analytix serve` through the managed analytix runtime base URL. | Not a default Go backend. |

Allowed wording:

```text
Analytix runtime adapter preserves encoded shared endpoint paths when
forwarding runtime requests to the managed runtime HTTP host.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 129. 2026-06-22 Preload SSE Bridge Proof

This addendum gives the renderer-facing SSE facade executable bridge evidence.
SSE streaming must remain on `window.analytix` and analytix-owned
`runtime:sse:*` IPC channels, with payload-only listener delivery.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| SSE start/stop handoff | `api.runtime.startSse` and `stopSse` preserve thread id, cursor, stream id, and stop id into analytix IPC channels. | Not a Reasonix SessionAPI bridge. |
| SSE listener boundary | Event/end/error wrappers forward payloads only and remove listeners on unsubscribe. | Not Electron event exposure. |
| Bridge identity | The executable test loads only the exposed `analytix` API. | Not a deprecated bridge alias. |

Allowed wording:

```text
Analytix preload SSE bridge preserves start/stop arguments and payload-only
listener delivery on `window.analytix`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public SSE protocol parity, live Go SSE
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 130. 2026-06-22 Main SSE Host URL Encoding Proof

This addendum proves main SSE IPC encodes renderer-provided thread ids before
fetching the managed runtime event stream. It pairs with D-0198 preload bridge
proof and keeps SSE streaming on analytix-owned runtime routes.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| SSE runtime host URL | Dangerous thread ids are encoded into `/v1/threads/{id}/events` before fetch. | Not a Reasonix SessionAPI route. |
| Cursor/header preservation | `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, and bearer auth are preserved. | Not a live Go SSE server. |
| Error channel ownership | Fatal host failures return via `runtime:sse-error` for the same stream id. | Not a public upstream protocol. |

Allowed wording:

```text
Analytix main SSE IPC encodes thread ids before fetching managed runtime event
routes and preserves cursor headers.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public SSE protocol parity, live Go SSE
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 131. 2026-06-22 Renderer Runtime Client Bridge Proof

This addendum proves the renderer runtime client facade uses only
`window.analytix.runtime` for runtime request, restart, and SSE operations. It
prevents legacy bridge fallback from re-entering the renderer layer.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Runtime request facade | Path, method, and body are forwarded unchanged through `window.analytix.runtime`. | Not a Reasonix SessionAPI bridge. |
| Runtime restart facade | Explicit restart is forwarded through `window.analytix.runtime.restartRuntime`, enabling first-run provider setup to restart the managed runtime before probing. | Not a renderer-visible backend switcher. |
| SSE facade | Start/stop arguments and listener handlers are forwarded unchanged. | Not a public upstream protocol. |
| Alias rejection | Throwing `window.kun` / `window.reasonix` getters prove legacy aliases are not read. | Not deprecated bridge compatibility. |

Allowed wording:

```text
Analytix renderer runtime client preserves runtime request, runtime restart,
and SSE arguments through `window.analytix.runtime` without legacy bridge
fallback.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 132. 2026-06-22 Renderer Settings Bridge Proof

This addendum proves the renderer settings client facade uses only
`window.analytix.settings` for settings writes. It prevents Reasonix config
roots or deprecated bridge aliases from entering the renderer settings path.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Settings facade | `setSettings` forwards patches through `window.analytix.settings` unchanged. | Not a Reasonix settings protocol. |
| Top-level runtime settings | `runtime.model` and `runtime.approvalPolicy` stay under the top-level `runtime` settings object. | Not an old runtime-shaped fallback. |
| Alias rejection | Throwing `window.kun` / `window.reasonix` getters prove legacy aliases are not read during settings writes. | Not deprecated bridge compatibility. |

Allowed wording:

```text
Analytix renderer settings client preserves top-level runtime settings patches
through `window.analytix.settings` without legacy bridge fallback.
```

Forbidden wording:

```text
Do not claim Reasonix config/public settings protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
settings QA, release readiness, Kun identity, old runtime-shaped settings
fallback, or Rust/Tauri migration from this batch.
```

## 133. 2026-06-22 Renderer Runtime Provider Alias Guard

This addendum proves the renderer runtime provider tests run under a legacy
alias guard. It extends the client facade proof to provider methods that drive
thread lifecycle, approval/user-input, fork/resume, and dynamic route-id
encoding.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider alias guard | Shared provider test helper installs throwing `window.kun` / `window.reasonix` getters. | Not deprecated bridge compatibility. |
| Interaction coverage | Existing tests exercise analytix-owned routes, approval/user-input submit/cancel, fork/resume, archive/search, and dynamic id encoding under the guard. | Not Reasonix SessionAPI. |
| Route ownership | Provider route assertions remain `/health` or `/v1/*` analytix endpoints and exclude forbidden upstream route families. | Not hidden capability routes. |

Allowed wording:

```text
Analytix renderer runtime provider tests run under a legacy alias guard while
exercising thread lifecycle, approval/user-input, fork/resume, and route
encoding paths.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
renderer QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 134. 2026-06-22 Renderer Provider Runtime Client Facade Seal

This addendum seals the renderer provider runtime request surface. Provider
methods must call the runtime through `rendererRuntimeClient`, not by reaching
directly into `window.analytix.runtime.runtimeRequest`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Archive facade | `archiveThread` uses `rendererRuntimeClient.runtimeRequest` while preserving the existing `PATCH` route/body. | Not a behavior or protocol change. |
| Direct bypass scan | Product-sovereignty scan forbids `window.analytix.runtime.runtimeRequest` inside `analytix-runtime.ts`. | Not a deprecated bridge fallback. |
| Lifecycle coverage | Existing archive/restore tests keep passing under the provider alias guard. | Not packaged desktop QA. |

Allowed wording:

```text
Analytix renderer provider runtime requests are sealed behind
`rendererRuntimeClient`, including archive/restore.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
renderer QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 135. 2026-06-22 Side Conversation Relation Provider Contract

This addendum seals side conversation promotion behind the same renderer
provider/runtime client boundary as thread lifecycle operations.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Provider contract | `AgentProvider.updateThreadRelation` expresses relation PATCH as an analytix-owned provider operation. | Not Reasonix side/session protocol. |
| Store promotion path | `promoteSideConversation` calls the provider contract, refreshes threads, and closes the side panel. | Not direct bridge access from store code. |
| Direct bypass scan | Product-sovereignty scan forbids direct runtime request bridge calls in provider and side-store sources. | Not packaged desktop QA. |

Allowed wording:

```text
Analytix side conversation promotion uses the provider relation contract and
keeps relation PATCH requests behind `rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/side-conversation protocol parity, live Go
bridge readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged side-conversation QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 136. 2026-06-22 Renderer Usage Runtime Client Facade Seal

This addendum seals generic renderer runtime HTTP requests behind
`rendererRuntimeClient`. Dedicated named preload APIs remain allowed for
config/probe/restart/model-list contracts, but `/v1/*` runtime HTTP requests
from renderer production source must use the client facade.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Usage hooks | Thread, daily, and model usage loaders use `rendererRuntimeClient.runtimeRequest`. | Not a route or parser behavior change. |
| Settings diagnostics | Token economy savings and LLM debug rounds use the same facade. | Not Reasonix usage/debug protocol. |
| Renderer-wide scan | Product-sovereignty scan forbids `window.analytix.runtime.runtimeRequest` in renderer production source. | Named preload APIs remain separate contracts. |

Allowed wording:

```text
Analytix renderer generic runtime HTTP requests are sealed behind
`rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/usage protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
usage/dashboard QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 136a. 2026-06-23 Renderer Usage Runtime Client Facade G5 Shadow

This addendum promotes the renderer usage/debug facade seal into the G5
desktop sovereignty shadow. The live renderer remains TypeScript-owned; Go
computes only source/fixture-derived proof booleans.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Usage shadow replay | `desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` records thread/day/model usage loaders behind `rendererRuntimeClient.runtimeRequest`. | Not a live Go usage bridge. |
| Settings diagnostics shadow replay | Token economy savings and LLM debug diagnostics are included as facade-backed diagnostics evidence. | Not Reasonix usage/debug protocol. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits usage coverage, diagnostics coverage, direct bridge rejection, scan guard presence, and unit-proof presence. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays renderer usage/debug runtime-client facade
evidence.
```

Forbidden wording:

```text
Do not claim live Go desktop bridge readiness, Reasonix usage/debug/session
protocol parity, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged usage/dashboard QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 137. 2026-06-22 Renderer Settings Read Facade Seal

This addendum seals ordinary renderer settings reads behind
`rendererRuntimeClient.getSettings`. Explicit named write APIs remain separate
contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Settings reads | Keyboard shortcuts, speech-to-text, and initial usage model-label reads use `rendererRuntimeClient.getSettings`. | Not a settings schema change. |
| Event sync | Existing `analytix:settings-changed` listeners remain unchanged. | Not packaged settings QA. |
| Renderer-wide scan | Product-sovereignty scan forbids `window.analytix.settings.getSettings` in renderer production source. | `setSettings` and `saveSettingsSilent` remain named write contracts. |

Allowed wording:

```text
Analytix renderer settings reads are sealed behind `rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix settings protocol parity, live Go bridge readiness,
renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
settings QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 137a. 2026-06-23 Renderer Settings Read Facade G5 Shadow

This addendum promotes the renderer settings-read facade seal into the G5
desktop sovereignty shadow. The live renderer remains TypeScript-owned; Go
computes only source/fixture-derived proof booleans.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Settings-read shadow replay | `desktopSovereignty.rendererSettingsReadFacadeMatrix` records keyboard shortcut, speech-to-text, and usage model-label reads behind `rendererRuntimeClient.getSettings`. | Not a live Go settings bridge. |
| Event sync proof | Settings hooks preserve `analytix:settings-changed` event sync while initial usage model-label remains one-shot read evidence. | Not Reasonix config/settings protocol. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits settings reader coverage, event-sync preservation, direct bridge rejection, and scan guard presence. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays renderer settings-read facade evidence.
```

Forbidden wording:

```text
Do not claim live Go settings bridge readiness, Reasonix config/settings/session
protocol parity, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged settings QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 137b. 2026-06-23 AutoResearch Direction Tracking G5 Shadow

This addendum promotes existing AutoResearch direction tracking into the G5
full-loop shadow. The live runtime remains TypeScript-owned; Go computes only
source/fixture-derived proof booleans.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Direction tracking replay | `controlExecutableCases.autoResearch` records direction text, outcome, summary, and `record_research_direction` tool name. | Not a public Reasonix project protocol. |
| Durable audit files | The conformance test writes `directions_tried.json` and appends `direction_recorded` to `iteration_log.jsonl`. | Not stable-prefix or tool-schema state. |
| Active-goal guard | Direction records require an active research goal and unknown requirement evidence still writes no findings. | Not a top-level AutoResearch route. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits direction file, iteration-log, tool-name, and active-goal guard booleans. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays AutoResearch direction tracking evidence.
```

Forbidden wording:

```text
Do not claim live Go AutoResearch backend readiness, Reasonix project/session
protocol parity, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged long-task QA, release readiness, Kun identity, deprecated bridge or
settings fallback, or Rust/Tauri migration from this batch.
```

## 137c. 2026-06-23 MCP Call-Time Reconnect G5 Shadow

This addendum promotes MCP tool-call reconnect classification into the G5
full-loop shadow. The live MCP provider remains TypeScript-owned; Go computes
only source/fixture-derived retry/no-retry booleans.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Stale connection recovery | `mcpCallReconnect` records a stale MCP transport failure that closes the old client and succeeds on the second attempt. | Not a live Go MCP client. |
| Protocol error classification | Deterministic MCP protocol validation errors return `tool_execution_failed` without reconnecting. | Not a Reasonix MCP public protocol. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits transport retry, protocol no-retry, attempt counts, close counts, success, and error-code booleans. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays MCP call-time reconnect classification.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP/session protocol
parity, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, credentialed
MCP matrix, packaged MCP QA, release readiness, Kun identity, deprecated bridge
or settings fallback, or Rust/Tauri migration from this batch.
```

## 137d. 2026-06-23 Plan Step/Cancel/Cache G5 Shadow

This addendum promotes the explicit Plan-mode step/cancel/cache boundary guard
into the G5 full-loop shadow. The live TypeScript loop remains authoritative;
Go computes only source/fixture-derived planner gating, cancellation, retry,
and cache telemetry evidence.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Step-0 planner gating | `planStepCancelCache` records `create_plan` and read-only `ls` as advertised tools while `bash` remains excluded. | Not a public Reasonix auto-plan setting. |
| Forced plan follow-up | The follow-up request is constrained to the `create_plan` tool only. | Not a Reasonix controller protocol. |
| Cancel/cache baseline | An aborted plan follow-up keeps `prefixChanged: false`, empty prefix-change reasons, and retry baseline reuse. | Not dynamic state in the stable prefix or tool schema. |
| Provider cache telemetry | The case preserves two DeepSeek `chat_completions` usage events with `80` cache-hit and `20` cache-miss tokens. | Not a live provider or packaged Plan QA claim. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits Plan step/cancel/cache booleans. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays Plan step/cancel/cache stability.
```

Forbidden wording:

```text
Do not claim live Go loop readiness, Reasonix controller/session protocol
parity, public auto-plan settings, renderer-visible Go routes, default Go
backend readiness, top-level Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer navigation, packaged Plan QA, release readiness, Kun identity,
deprecated bridge or settings fallback, or Rust/Tauri migration from this
batch.
```

## 137e. 2026-06-23 Plan/Auto-Route State Reset G5 Shadow

This addendum promotes cancelled Plan turn reset and post-cancel auto-route
currentness into the G5 full-loop shadow. The live TypeScript loop remains
authoritative; Go computes only source/fixture-derived reset, reroute, router
isolation, and product-boundary evidence.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Cancelled Plan no-leak | Later normal and auto turns have no Plan `modeInstruction` after an aborted Plan follow-up. | Not a public Reasonix auto-plan setting. |
| Required tool reset | Later normal and auto turns do not inherit `requiredToolName: create_plan`. | Not a Reasonix controller protocol. |
| Tool surface reset | Later normal and auto turns do not advertise `create_plan`. | Not a top-level Workflow/Create Loop entry. |
| Auto-route currentness | The next `model: "auto"` turn invokes `_auto_router` once and accepts the current `deepseek-v4-pro` / `max` recommendation. | Not a public auto-plan or SessionAPI route. |
| Go G5 output | `BuildG5ControlExecutableOutput` emits Plan/auto-route reset booleans. | Go is still shadow-only and cannot become the default backend. |

Allowed wording:

```text
Analytix G5 shadow now replays Plan/auto-route state reset after cancellation.
```

Forbidden wording:

```text
Do not claim live Go loop readiness, Reasonix controller/session protocol
parity, public auto-plan settings, project auto-plan overrides,
renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
Plan/auto-route QA, release readiness, Kun identity, deprecated bridge or
settings fallback, or Rust/Tauri migration from this batch.
```

## 137f. 2026-06-23 MCP Search Refresh Drift Evidence Closure

This addendum makes the existing MCP catalog refresh/currentness drift proof
mandatory in closure scans. It does not change runtime behavior; it keeps MCP
refresh behind analytix-owned MCP meta-tools and G5 shadow evidence.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Refresh meta-tool | `mcp_refresh_catalog` remains covered by the MCP lifecycle oracle. | Not a public MCP-indexer route. |
| Catalog currentness | Refresh drift records initial `search_issues`, expanded `search_issues` + `create_issue`, `totalIndexed: 2`, and `catalogDrift: true`. | Not a credentialed MCP matrix claim. |
| Go G5 output | `mcpSearchRefreshDrift` is already replayed by Go G5 shadow. | Go is still shadow-only and cannot become the default backend. |
| Scan freshness | `scan:product-sovereignty` now requires the refresh drift tokens. | Not release readiness. |

Allowed wording:

```text
Analytix closure evidence includes MCP search refresh drift currentness.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP/session protocol
parity, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, credentialed
MCP matrix, packaged MCP QA, release readiness, Kun identity, deprecated bridge
or settings fallback, or Rust/Tauri migration from this batch.
```

## 138. 2026-06-22 Renderer Named Bridge API Allow-list

This addendum makes remaining direct renderer `window.analytix.runtime.*` and
`window.analytix.settings.*` production calls explicit allow-listed named
contracts. Generic runtime HTTP requests and ordinary settings reads remain
sealed behind `rendererRuntimeClient`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Named runtime APIs | Direct renderer runtime calls are limited to config file access/save/open, provider probe, upstream model fetch, runtime restart, and runtime status subscription. | Not a Reasonix public bridge or SessionAPI. |
| Named settings APIs | Direct renderer settings calls are limited to `setSettings` and `saveSettingsSilent`. | Not a settings read path or legacy settings fallback. |
| Renderer-wide scan | Product-sovereignty scan fails on any unlisted direct `window.analytix.runtime.*` or `window.analytix.settings.*` production call. | Not packaged desktop bridge QA. |

Allowed wording:

```text
Analytix renderer direct window bridge calls are restricted to explicit named
preload contracts.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
bridge QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 139. 2026-06-23 Renderer Optional Bridge Bypass Seal

This addendum closes the optional-chaining variant of direct renderer bridge
access. `window.analytix?.runtime?.*` and `window.analytix?.settings?.*` are
governed by the same rules as dot access.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Optional-chain generic bypass | Product-sovereignty scan forbids both dot and optional-chain direct `runtimeRequest` / `getSettings` access in renderer production source. | Not a Reasonix bridge protocol shim. |
| Connect Phone settings read | Connect Phone dialog settings loading uses `rendererRuntimeClient.getSettings`. | Does not change Connect Phone product entry or settings schema. |
| Plugin marketplace diagnostics | Plugin marketplace diagnostics no longer probe generic `window.analytix?.runtime?.runtimeRequest`; diagnostics stay behind the provider/runtime facade. | Not packaged marketplace/MCP QA. |

Allowed wording:

```text
Analytix renderer bridge scans cover both dot and optional-chain direct access.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend readiness, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
bridge QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 140. 2026-06-23 Renderer Bridge Allow-list G5 Shadow

This addendum promotes renderer bridge allow-list evidence into Go G5
`desktopSovereignty` shadow replay. The source of truth remains TypeScript
renderer/preload/main/runtime contracts.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererBridgeAllowList` records allowed named APIs, source-derived direct APIs, violation sets, bypass counts, and optional-chain scan coverage. | Not a live Go desktop bridge. |
| TS conformance | The fixture is derived from real renderer production source and the product-sovereignty scan script. | Not Reasonix SessionAPI or bridge protocol. |
| Go replay | Go computes named API allow-list status and generic bypass absence from the TS-owned fixture. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer bridge allow-list evidence.
```

Forbidden wording:

```text
Do not claim live Go bridge readiness, renderer-visible Go routes, default Go
backend readiness, Reasonix SessionAPI/bridge protocol parity, top-level
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged
bridge QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration from this batch.
```

## 141. 2026-06-23 Runtime HTTP Auth Matrix G5 Shadow

This addendum promotes the D-0190 runtime HTTP auth matrix into Go G5
`runtimeHttpRouteSovereignty` shadow replay. The source of truth remains the
TypeScript `analytix serve` router and its analytix-owned `/v1/*` route table.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `runtimeHttpRouteSovereignty.authMatrix` records `/health` 200, all protected `/v1/*` route keys, unauthorized status/body, and sensitive route keys. | Not a live Go HTTP server. |
| TS conformance | The real TypeScript router dispatches every registered route without auth and compares status/body against the fixture matrix. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes public health status, protected-route auth rejection, unauthorized body shape, full matrix coverage, and sensitive-route protection from the TS-owned fixture. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays the runtime HTTP auth matrix for the
analytix-owned route table.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 142. 2026-06-23 Runtime Forbidden Route Dispatch G5 Shadow

This addendum promotes the D-0191 runtime forbidden route dispatch proof into
Go G5 `runtimeHttpRouteSovereignty` shadow replay. The source of truth remains
the TypeScript `analytix serve` router and its analytix-owned route table.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` records valid-auth mode, structured 404 status/body, all forbidden tokens, upstream protocol tokens, and hidden-surface tokens. | Not a live Go HTTP server. |
| TS conformance | The real TypeScript router dispatches every forbidden token with valid auth and compares status/body against the fixture matrix. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes structured not_found, full token coverage, protocol-token rejection, hidden-surface rejection, and valid-auth dispatch mode from the TS-owned fixture. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays the runtime forbidden-route dispatch
matrix for the analytix-owned route table.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 143. 2026-06-23 Shared Endpoint Builder G5 Shadow

This addendum promotes the D-0192 shared endpoint builder proof into Go G5
`runtimeHttpRouteSovereignty` shadow replay. The source of truth remains the
TypeScript shared endpoint contract used by renderer, preload/main, and
runtime adapter paths.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` records 18 encoded builder cases, exported endpoint strings, forbidden-token absence, and canonical user-input state. | Not a live Go HTTP server. |
| TS conformance | The matrix is derived from shared endpoint source and the D-0192 unit-test proof. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes builder count, encoded route-id preservation, template ownership, plural user-input canonical status, sensitive builder coverage, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays shared endpoint builder encoding and
template ownership evidence.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 144. 2026-06-23 Renderer Runtime Endpoint Builder G5 Shadow

This addendum promotes the D-0193 renderer-provider endpoint builder proof into
Go G5 `desktopSovereignty` shadow replay. The source of truth remains the
analytix renderer provider using shared endpoint constants/builders before
calling the preload/main runtime bridge.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererProviderEndpointMatrix` records shared root paths, encoded dynamic ids, runtime request paths, facade use, ownership guard, and unit proof. | Not a live Go desktop bridge. |
| TS conformance | The matrix is derived from `AnalytixRuntimeProvider`, its route-encoding unit test, and forbidden public runtime route guard. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes shared-root, encoded dynamic route-id, analytix-owned runtime path, facade, and unit-proof outputs. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer provider endpoint-builder
ownership and dynamic route-id encoding before bridge runtime requests.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 145. 2026-06-23 Main IPC Endpoint Builder G5 Shadow

This addendum promotes the D-0194 main IPC endpoint-builder allow-list proof
into Go G5 `desktopMainIpcBoundary` shadow replay. The source of truth remains
the analytix `runtimeRequestPayloadSchema` gate before the main runtime adapter
handoff.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopMainIpcBoundary.endpointBuilderAllowListMatrix` records encoded shared-builder requests, raw dynamic route rejection, singular user-input rejection, shared templates, and unit proof. | Not a live Go main IPC bridge. |
| TS conformance | The matrix is derived from `app-ipc-schemas.ts` and the D-0194 runtime request schema unit tests. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes accepted shared paths, raw dynamic rejection, shared-template use, unit-proof presence, and singular user-input rejection. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays main IPC endpoint-builder allow-list
coverage and rejection of raw dynamic route ids before runtime adapter handoff.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 146. 2026-06-23 Main IPC Runtime Adapter Handoff G5 Shadow

This addendum promotes the D-0195 registered main IPC handler handoff proof
into Go G5 `desktopMainIpcBoundary` shadow replay. The source of truth remains
the analytix `runtime:request` handler parsing with `runtimeRequestPayloadSchema`
before calling the runtime adapter.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` records encoded adapter calls, rejected raw/singular calls, parse-before-adapter ordering, and unit proof. | Not a live Go main IPC bridge. |
| TS conformance | The matrix is derived from `register-app-ipc-handlers.ts` and the D-0195 handler unit tests. | Not Reasonix SessionAPI or public route protocol. |
| Go replay | Go computes encoded path preservation, method/body preservation, raw-route rejection, reject-before-call, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays main IPC runtime-adapter handoff coverage
and proves invalid raw/singular paths are rejected before adapter invocation.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 147. 2026-06-23 Preload Runtime Request Bridge G5 Shadow

This addendum promotes the D-0196 preload runtime request bridge proof into Go
G5 `desktopSovereignty` shadow replay. The source of truth remains the
analytix preload facade exposed as `window.analytix`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.preloadRuntimeRequestBridgeMatrix` records runtime/diagnostics facade calls, encoded ids, path/method/body preservation, and unit proof. | Not a live Go preload bridge. |
| TS conformance | The matrix is derived from `src/preload/index.ts` and the D-0196 preload runtime request unit test. | Not Reasonix SessionAPI or public bridge protocol. |
| Go replay | Go computes runtime request preservation, diagnostics same-channel behavior, analytix-only exposure, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays preload runtime request bridge coverage
and proves runtime/diagnostics calls remain on `window.analytix` and
`runtime:request`.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 155. 2026-06-23 Side Conversation Relation G5 Shadow

This addendum promotes the D-0204 side conversation relation provider contract
proof into Go G5 `desktopSovereignty` shadow replay. The source of truth
remains the TypeScript side store calling the analytix-owned provider
contract, not a direct runtime bridge or upstream side protocol.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.sideConversationRelationContractMatrix` records `updateThreadRelation`, `promoteSideConversation`, relation `primary`, optional provider contract proof, provider implementation proof, store proof, refresh/close proof, direct bridge rejection, unit proof, and scan guard. | Not a live Go side-store bridge. |
| TS conformance | The matrix is derived from `types.ts`, `analytix-runtime.ts`, `chat-store-side-actions.ts`, `chat-store-side-actions.test.ts`, and `scan-product-sovereignty.cjs`. | Not Reasonix side/session protocol. |
| Go replay | Go computes optional provider contract proof, provider promotion, refresh/close behavior, direct bridge rejection, scan guard presence, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays side conversation relation promotion and
proves it stays behind `AgentProvider.updateThreadRelation`.
```

Forbidden wording:

```text
Do not claim live Go side-store bridge readiness, renderer-visible Go routes,
default Go backend readiness, direct side-store runtime bridge permission,
Reasonix side/session protocol parity, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation, packaged side QA, release readiness, Kun
identity, deprecated settings fallback, or Rust/Tauri migration from this
batch.
```

## 154. 2026-06-23 Renderer Provider Facade Seal G5 Shadow

This addendum promotes the D-0203 renderer provider runtime-client facade seal
proof into Go G5 `desktopSovereignty` shadow replay. The source of truth
remains the TypeScript `AnalytixRuntimeProvider` calling
`rendererRuntimeClient.runtimeRequest`, not direct renderer bridge calls.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererProviderFacadeSealMatrix` records the runtime client facade, forbidden direct bridge token, sealed methods, archive/restore proof, relation PATCH proof, lifecycle unit proof, and scan guards. | Not a live Go provider bridge. |
| TS conformance | The matrix is derived from `src/renderer/src/agent/analytix-runtime.ts`, `analytix-runtime.test.ts`, and `scan-product-sovereignty.cjs`. | Not Reasonix SessionAPI or direct bridge permission. |
| Go replay | Go computes runtime client use, direct bridge rejection, archive/restore coverage, relation PATCH coverage, scan guard presence, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer provider facade sealing and
proves archive/restore plus relation PATCH stay behind
`rendererRuntimeClient.runtimeRequest`.
```

Forbidden wording:

```text
Do not claim live Go provider bridge readiness, renderer-visible Go routes,
default Go backend readiness, direct renderer runtime bridge permission,
Reasonix SessionAPI/public bridge protocol parity, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer navigation, packaged provider QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration from this batch.
```

## 153. 2026-06-23 Renderer Provider Alias Guard G5 Shadow

This addendum promotes the D-0202 renderer provider alias guard proof into Go
G5 `desktopSovereignty` shadow replay. The source of truth remains the
TypeScript `AnalytixRuntimeProvider` calling analytix-owned runtime routes
through the renderer runtime client facade.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererProviderAliasGuardMatrix` records the `installDsGui` helper, forbidden `kun` / `reasonix` aliases, guarded provider tests, coverage flags, source-only analytix proof, and forbidden-route guard proof. | Not a live Go provider bridge. |
| TS conformance | The matrix is derived from `src/renderer/src/agent/analytix-runtime.ts` and `analytix-runtime.test.ts`. | Not Reasonix SessionAPI or public frontend protocol. |
| Go replay | Go computes throwing alias installation, route coverage, lifecycle/gate coverage, fork/resume/dynamic encoding coverage, forbidden-route rejection, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer provider alias guard behavior
and proves provider route/lifecycle/gate/fork/resume paths stay behind
`window.analytix` and analytix-owned `/v1/*` routes.
```

Forbidden wording:

```text
Do not claim live Go provider bridge readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public bridge protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged provider QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 152. 2026-06-23 Renderer Settings Bridge G5 Shadow

This addendum promotes the D-0201 renderer settings bridge proof into Go G5
`desktopSovereignty` shadow replay. The source of truth remains the TypeScript
renderer client facade calling `window.analytix.settings`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererSettingsBridgeMatrix` records top-level `runtime` patch values, settings cache expectations, settings API source proof, unit proof, and legacy alias unread proof. | Not a live Go settings bridge. |
| TS conformance | The matrix is derived from `src/renderer/src/agent/runtime-client.ts` and `runtime-client.test.ts`. | Not Reasonix settings protocol or config root. |
| Go replay | Go computes settings API use, settings read cache, write refresh, top-level runtime patch preservation, legacy alias unread status, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer settings bridge behavior and
proves top-level `runtime` settings patches stay behind `window.analytix`.
```

Forbidden wording:

```text
Do not claim live Go settings bridge readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix settings/config protocol parity,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged settings QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 151. 2026-06-23 Renderer Runtime Client Bridge G5 Shadow

This addendum promotes the D-0200 renderer runtime client bridge proof into Go
G5 `desktopSovereignty` shadow replay. The source of truth remains the
TypeScript renderer client facade calling `window.analytix.runtime`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.rendererRuntimeClientBridgeMatrix` records encoded request path variants, argument counts, restart passthrough, SSE start/stop, listener APIs, unit proof, and legacy alias unread proof. | Not a live Go desktop bridge. |
| TS conformance | The matrix is derived from `src/renderer/src/agent/runtime-client.ts` and `runtime-client.test.ts`. | Not Reasonix SessionAPI or public frontend protocol. |
| Go replay | Go computes renderer request argument preservation, restart passthrough, SSE control/listener preservation, legacy alias unread status, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer runtime client bridge behavior
and proves renderer request/restart/SSE calls stay behind `window.analytix`.
```

Forbidden wording:

```text
Do not claim live Go desktop bridge readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public frontend protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged renderer QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 150. 2026-06-23 Main SSE Host URL Encoding G5 Shadow

This addendum promotes the D-0199 main SSE host URL encoding proof into Go G5
`desktopMainIpcBoundary` shadow replay. The source of truth remains the
TypeScript `registerRuntimeSseIpc` bridge fetching managed `analytix serve`
thread events.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopMainIpcBoundary.mainSseHostEncodingMatrix` records encoded thread events path, cursor, `Last-Event-ID`, `Accept`, auth, stream id, error payload, and forbidden-route guard. | Not a live Go SSE server. |
| TS conformance | The matrix is derived from `src/main/runtime-sse-ipc.ts` and the D-0199 main SSE unit test. | Not Reasonix SessionAPI or public SSE protocol. |
| Go replay | Go computes encoded thread/cursor preservation, header/stream-id preservation, forbidden-route rejection, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays main SSE host URL encoding and proves
dangerous thread ids remain encoded on `/v1/threads/{id}/events`.
```

Forbidden wording:

```text
Do not claim live Go SSE server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public SSE protocol parity,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged SSE QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 149. 2026-06-23 Preload SSE Bridge G5 Shadow

This addendum promotes the D-0198 preload SSE bridge proof into Go G5
`desktopSovereignty` shadow replay. The source of truth remains the analytix
preload facade exposed as `window.analytix`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopSovereignty.preloadSseBridgeMatrix` records SSE channels, start/stop calls, event/end/error payloads, payload-only wrappers, cleanup, and unit proof. | Not a live Go SSE server. |
| TS conformance | The matrix is derived from `src/preload/index.ts` and the D-0198 preload SSE bridge unit test. | Not Reasonix SessionAPI or public SSE protocol. |
| Go replay | Go computes start/stop preservation, payload-only listener delivery, cleanup symmetry, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays preload SSE bridge coverage and proves
streaming calls remain on `window.analytix` and `runtime:sse:*` with
payload-only listener delivery.
```

Forbidden wording:

```text
Do not claim live Go SSE server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public SSE protocol parity,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged SSE QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 148. 2026-06-23 Runtime Host URL Handoff G5 Shadow

This addendum promotes the D-0197 runtime host URL handoff proof into Go G5
`desktopMainIpcBoundary` shadow replay. The source of truth remains the
TypeScript `runtimeRequestViaHost` adapter targeting managed `analytix serve`.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| G5 fixture | `desktopMainIpcBoundary.runtimeHostHandoffMatrix` records adapter/base-url functions, encoded ids, query, method/body, auth/header/content-type, and ensureRuntime proof. | Not a live Go HTTP server. |
| TS conformance | The matrix is derived from `src/main/runtime/analytix-adapter.ts` and the D-0197 runtime adapter unit test. | Not Reasonix SessionAPI or public host route protocol. |
| Go replay | Go computes encoded path/query preservation, method/header/body preservation, ensured settings use, and unit-proof presence. | Does not enable default Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix G5 shadow executable-replays runtime host URL handoff coverage and
proves `runtimeRequestViaHost` preserves encoded endpoint paths, query,
headers, and body into managed `analytix serve`.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, renderer-visible Go routes,
default Go backend readiness, Reasonix SessionAPI/public route protocol
parity, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, packaged route QA, release readiness, Kun identity, deprecated
settings fallback, or Rust/Tauri migration from this batch.
```

## 123. 2026-06-22 Shared Endpoint Builder Sovereignty Proof

This addendum pins the shared endpoint builder contract that renderer, main IPC,
and runtime route evidence depend on. It proves IDs are URL-encoded before they
become path segments and that exported shared endpoint strings remain
analytix-owned.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| ID encoding | Thread, turn, checkpoint, approval, user-input, session, attachment, and memory path builders encode `/`, query, and fragment text. | Not a runtime behavior change. |
| Canonical user-input endpoint | Shared template remains plural `/v1/user-inputs/{id}`; singular `/v1/user-input/:id` is not exported as canonical. | Singular route remains compatibility only. |
| Forbidden shared endpoints | Exported endpoint strings contain no Reasonix/Kun/DeepSeek/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer/session-api tokens. | Not a Reasonix protocol shim. |

Allowed wording:

```text
Analytix shared endpoint builders URL-encode route ids and keep exported
runtime endpoint templates analytix-owned.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 122. 2026-06-22 Runtime Forbidden Route Dispatch Proof

This addendum ties D-0189 forbidden route-token governance to actual
TypeScript HTTP dispatch behavior. The proof confirms forbidden upstream and
hidden-capability route families are absent from the real router, not merely
unauthenticated.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Forbidden route matrix | Every `runtimeHttpRouteSovereignty.forbiddenRouteTokens` path is dispatched with valid auth and returns structured 404. | Not placeholder route registration. |
| Upstream route protocol rejection | `/v1/reasonix`, `/v1/runtime/go`, `/session-api`, and related route families are absent from the active router. | Not Reasonix SessionAPI or a Go route. |
| Hidden capability route rejection | Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route tokens are not registered. | Not top-level product navigation. |

Allowed wording:

```text
Analytix HTTP route forbidden-surface coverage is executable against the real
TypeScript router and returns structured not_found for forbidden route tokens.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```

## 121. 2026-06-22 Runtime HTTP Auth Matrix Proof

This addendum ties the D-0189 source-derived route table to actual TypeScript
HTTP dispatch behavior. The proof uses the G5 route-sovereignty fixture as the
route source and confirms runtime auth gates execute before route-specific body
parsing or side effects.

New requirements now met:

| Surface | Requirement now met | Non-parity boundary |
| --- | --- | --- |
| Auth matrix | Every registered D-0189 route is dispatched without auth; `/health` returns 200 and all 43 `/v1/*` routes return structured 401. | Not a live Go HTTP server or public control plane. |
| Sensitive routes | SSE, task-job wait/output/kill, approval, user-input, and resume-thread route keys are explicitly included in the unauthorized matrix. | Not Reasonix SessionAPI or task/sub-agent protocol. |
| Fixture binding | The test uses `runtimeHttpRouteSovereignty.routes` and `authenticatedRouteCount`, so route-table drift must update both route inventory and auth proof. | Not a renderer-visible Go route. |

Allowed wording:

```text
Analytix HTTP route auth coverage is executable against the real TypeScript
router for every registered `/v1/*` route.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend readiness,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
packaged route QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration from this batch.
```
