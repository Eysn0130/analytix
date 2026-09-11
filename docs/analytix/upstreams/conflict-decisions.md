# analytix upstream conflict decisions

Status: Reference / chronological ADR ledger
Applies to: 上游能力冲突和 Analytix 取舍
Source of truth for current behavior: current code, accepted specs, active
OpenSpec tasks and fresh validation
Current handover: [`../handovers/2026-07-25-first-stage-pause.md`](../handovers/2026-07-25-first-stage-pause.md)

This file records durable decisions when Kun, Reasonix, CodexDesktop-Rebuild,
or existing analytix behavior disagree.

Current status note (2026-06-25): Go runtime is now the default deterministic
core. Current adapter code rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as a
retired backend. Historical ADRs that discuss TypeScript as current authority,
explicit rollback, or Go as shadow/candidate-only are stage records unless a
newer decision repeats them as current policy. D-0251/D-0252/D-0253 evidence
gates remain post-cutover live validation, not Go default startup blockers.

Governance note (2026-07-02): source roles in this file are decision context,
not exclusive capability territories. A `reject` decision rejects the shape
reviewed in that ADR; it does not permanently ban a future analytix-native
capability that is re-specified, compared across all relevant upstreams,
tested, and accepted by a newer decision.

Decision format:

```text
## D-XXXX - <short title>

Date:
Sources:
Domain:
Status: accepted / superseded / rejected

Conflict:

Decision:

Rationale:

Validation:

Supersedes:
Superseded by:
```

## D-0074 - Reasonix Absorption Must Preserve Kun/Analytix Full-Function Baseline

Date: 2026-06-23
Sources: D-0245 full-function baseline guard, `KunAgent/Kun` tags `v0.2.13`
and `v0.2.14`, `esengine/DeepSeek-Reasonix`, analytix runtime contracts
Domain: upstream absorption / provider runtime / product sovereignty
Status: accepted

Conflict:

Reasonix has stronger engine capabilities around DeepSeek prefix cache,
runtime audit, MCP lifecycle, sub-agent lineage, and evidence enforcement.
Imported incorrectly, those strengths could narrow analytix to DeepSeek-only
or replace the Kun/Analytix desktop product surface with Reasonix SessionAPI,
config roots, public sub-agent routes, AutoResearch/MCP-indexer entries, or a
new bridge/settings schema.

Decision:

Use Kun/Analytix full-function behavior as the immutable product baseline and
embed Reasonix only as internal engine capability. DeepSeek cache/prefix
optimizations are provider-specific enhancements, not provider narrowing.
OpenAI-compatible, Anthropic-compatible, and custom full endpoint providers
must keep their own URL/body/header/stream/usage semantics. `/goal`, MCP,
sub-agent lineage, and research/audit improvements must land behind existing
analytix runtime contracts and UI entries.

Rationale:

The target is not "replace Kun with Reasonix"; it is "preserve Kun/Analytix
full functionality, then absorb stronger Reasonix engine behavior with
regression proof." G6 default Go runtime can only become the unique baseline
after this full-function non-regression evidence passes.

Validation:

`packages/runtime-go/internal/upstreamaudit/baseline_absorption.go`,
`packages/runtime-go/d0245_baseline_absorption_test.go`,
`packages/runtime-go/runtime_server.go`,
`packages/runtime-go/runtime_server_test.go`,
`packages/runtime/src/contracts/capabilities.ts`,
`packages/runtime/tests/go-runtime-conformance.test.ts`,
`packages/runtime/tests/runtime-event-reducer.test.ts`, and
`scripts/scan-product-sovereignty.cjs` assert the baseline guard, absorption
matrix, multi-model runtime turns, DeepSeek-only cache diagnostics, audit-only
goal evidence, and forbidden top-level entries.

Supersedes:
Superseded by:

## D-0076 - Expected-Blocked Preflight Is Not Cutover Evidence

Date: 2026-06-23
Sources: D-0248 strict G6 evidence gate, D-0247 readiness semantics,
D-0243 G6 readiness hardening, Kun/Analytix full-function baseline
Domain: runtime / G6 readiness / release evidence
Status: accepted

Conflict:

D-0248 needs a one-command strict preflight, but real provider credentials,
credentialed MCP servers, and packaged desktop app state may be unavailable in
ordinary code-stage validation. Treating a blocked strict preflight as a pass
would accidentally authorize Go as default without live evidence.

Decision:

`npm run runtime:go:preflight -- --json --gate` is the current local preflight
entrypoint. Historical expected-blocked G6 runs are archive data only and are
not cutover evidence. After an RC source freeze,
`npm run runtime:go:rc-control-plane -- --json` may prove only the source-bound
validation control plane. Go default readiness for release packaging is
governed by `npm run runtime:go:release-gate`, which includes product
sovereignty, preflight, the production-tagged Go runtime server source-set test,
artifact admission, and the active OpenSpec RC ledger, and succeeds only when
the final JSON has `passed: true`.

Rationale:

This preserves useful local validation without weakening the G6 hard gate.
Fake/local provider and MCP matrices remain regression proof, not credentialed
cutover proof.

Validation:

Historical validation used `scripts/d0247-go-runtime-readiness-report.mjs`,
`scripts/d0248-go-runtime-g6-preflight.mjs`, and
`docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md`.
Current validation uses `scripts/runtime-go-preflight.mjs`,
`scripts/runtime-go-product-regression.mjs`,
`src/main/runtime/analytix-adapter.test.ts`, and
`docs/analytix/upstreams/go-runtime-retirement-checklist.md` to assert strict
hard-gate failure on skipped evidence, expected-blocked audit semantics,
TypeScript retired-backend diagnostic when explicitly requested, and no renderer-visible Go
switcher.

Supersedes:
Superseded by: 2026-06-25 Go default delivery and the formal
`runtime-go-*` validation command set.

## D-0075 - AutoResearch State Lands Under Goal, Not A Product Entry

Date: 2026-06-23
Sources: D-0246 AutoResearch Go runtime project-local state,
Reasonix local checkout `7032f39336f4ae5f216e1fcb3368e5f679723490`,
Kun/Analytix full-function baseline
Domain: runtime / goal research / product sovereignty
Status: accepted

Conflict:

Reasonix AutoResearch-style long-task state is useful engine behavior, but
importing it as a top-level AutoResearch route, UI entry, config root, or
workspace protocol would violate the Kun/Analytix product baseline and create a
second public runtime contract.

Decision:

Implement AutoResearch state only as project-local `/goal --research` support
inside the existing analytix runtime turn/SSE contract. The Go runtime may
write `.analytix/autoresearch/<threadId>/task_spec.md`, `progress.json`,
`findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`; it must
reject unknown requirement evidence before writing findings, emit
`autoresearch_state_audit` as audit-only SSE, and preserve replay after restart.
It must not write `REASONIX.md` or `AGENTS.md`, put dynamic research state in
stable prefix/tool schema, expose `/v1/autoresearch`, or add top-level UI.

Rationale:

This captures the durable research-state value without changing ordinary
`/goal` semantics, renderer projection, provider families, settings schema,
bridge contracts, or product navigation.

Validation:

`packages/runtime-go/internal/research/autoresearch_state.go`,
`packages/runtime-go/d0246_autoresearch_state_test.go`,
`packages/runtime-go/runtime_server.go`,
`packages/runtime-go/runtime_server_test.go`,
`packages/runtime/src/contracts/events.ts`,
`packages/runtime/tests/contracts.test.ts`,
`packages/runtime/tests/runtime-event-reducer.test.ts`,
`packages/runtime/tests/go-runtime-conformance.test.ts`, and
`scripts/scan-product-sovereignty.cjs` assert state files, path containment,
unknown-requirement rejection, audit schema, reducer ignore behavior, restart
replay, and absence of forbidden public/product surfaces.

Supersedes:
Superseded by:

## D-0073 - Runtime Info May Expose Absorption Metadata, Not Upstream Protocol

Date: 2026-06-23
Sources: D-0244 Reasonix capability red matrix, D-0243 G6 readiness hardening
slice, Reasonix local checkout `7032f39336f4ae5f216e1fcb3368e5f679723490`,
analytix runtime contracts
Domain: runtime / upstream absorption / product sovereignty
Status: accepted

Conflict:

D-0244 needs a machine-readable Reasonix capability red matrix connected to the
Go runtime server contract. Exposing it incorrectly could look like importing
Reasonix SessionAPI/config roots, a backend switcher, or a public MCP/sub-agent
protocol.

Decision:

Expose only analytix-owned absorption metadata at
`capabilities.upstreamAbsorption.reasonixCapabilityRedMatrix` in
`/v1/runtime/info`. The matrix may name Reasonix source paths, classifications,
machine checks, red/deferred blockers, and post-G6 delete candidates. It must
not add Reasonix routes, config roots, CLI identity, renderer-visible Go routes,
UI entries, product identity changes, or Kun product-entry drift.

Rationale:

The matrix makes G6 blockers executable and contract-visible while keeping
analytix as the contract owner. Green rows require code/test evidence; red and
deferred rows remain explicit blockers instead of being hidden in prose.

Validation:

`packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go`,
`packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go`,
`packages/runtime-go/d0244_reasonix_red_matrix_test.go`,
`packages/runtime/src/contracts/capabilities.ts`,
`packages/runtime-go/runtime_server.go`, and
`packages/runtime/tests/go-runtime-conformance.test.ts` assert the matrix
shape, status counts, blocker rows, runtime info schema, and absence of
`/v1/reasonix`.

Supersedes:
Superseded by:

## D-0072 - G6 Readiness Is Evidence-Gated, Not A Public Backend Switch

Date: 2026-06-23
Sources: D-0243 G6 readiness hardening slice, D-0242 Go runtime server contract
slice, Reasonix provider/cache/MCP/restart review, Kun product baseline,
analytix runtime contracts
Domain: runtime / G6 readiness / product sovereignty
Status: accepted

Conflict:

D-0242 exposed a real Go runtime contract subset. The final cutover keeps that
surface behind the Analytix runtime contract as `go-runtime-default`, while
credentialed provider/MCP evidence, packaged desktop QA, rollback drills, and
the Kun-derived desktop baseline remain post-cutover validation rather than
renderer-visible product switches.

Decision:

Keep G6 readiness inside the main adapter/canary and test scaffolds. The status
defaults to `ready:false` and requires durable restart proof, provider matrix,
MCP matrix, packaged QA, and an explicit `ANALYTIX_GO_RUNTIME_G6_READY=1`
gate. Missing, failed, or skipped evidence rejects Go runtime-candidate startup
and returns to TypeScript. Do not expand `/v1/runtime/info`, add a renderer
backend switcher, change settings schema, or expose Reasonix public protocol.

Rationale:

This lets analytix make G6 hard gates machine-testable without claiming G6 is
complete. It also preserves the existing renderer/preload/main contract while
turning D-0241/D-0242 temporary paths into explicit post-G6 delete candidates.

Validation:

`packages/runtime-go/g6_readiness.go`,
`packages/runtime-go/g6_readiness_test.go`,
`packages/runtime-go/runtime_server_test.go`,
`packages/runtime/tests/go-runtime-conformance.test.ts`,
`src/main/runtime/analytix-adapter.ts`,
`src/main/runtime/analytix-adapter.test.ts`,
`scripts/d0243-packaged-go-runtime-qa.mjs`, and
`scripts/scan-product-sovereignty.cjs` assert readiness defaults, skipped
credentialed probes, durable restart recovery, packaged QA scaffold coverage,
rollback, TypeScript default preservation, and no public contract/UI drift.

Supersedes:
Superseded by:

## D-0071 - Runtime Candidate Uses Contract Server, Not Public Backend

Date: 2026-06-23
Sources: D-0242 Go runtime server contract slice, D-0241 production-candidate
runtime parity slice, Reasonix provider/cache/gate/MCP/job review, Kun product
baseline, analytix runtime contracts
Domain: runtime / HTTP-SSE contract / product sovereignty
Status: accepted

Conflict:

D-0241 proved live local provider, durable replay, approval/user-input, fake
MCP, and job-lineage code under `/v1/internal/go-production-candidate/*`.
Leaving those as the only Go path would keep a proof-route architecture;
promoting them directly to a public/default backend would bypass G6, packaged
QA, rollback drills, and the Kun-derived desktop baseline.

Decision:

Add a separate Go runtime contract server binary,
`packages/runtime-go/cmd/runtime-server`, and a distinct internal gate:
`ANALYTIX_RUNTIME_BACKEND=go-runtime-candidate` plus
`ANALYTIX_GO_RUNTIME_CANDIDATE=1`. Its canary must probe real analytix
contract endpoints: `/health`, `/v1/runtime/info`, thread list/read/patch/fork,
session resume, SSE replay, turn create, approval, and user-input routes. The
server must hide `/v1/runtime/go` and the D-0241 internal proof route. Failure
stops Go, deletes temp state, records fallback status, and returns to
TypeScript.

Rationale:

This moves Go from proof routes toward the runtime contract without exposing a
partial backend to users. It also marks D-0241 provider-live, durable-replay,
approval-user-input, MCP manager, and job-lineage proof routes as
post-D-0242/G6 delete candidates once the contract server remains covered.

Validation:

`packages/runtime-go/runtime_server.go`,
`packages/runtime-go/runtime_server_test.go`,
`packages/runtime-go/cmd/runtime-server`,
`packages/runtime/tests/go-runtime-conformance.test.ts`,
`src/main/runtime/analytix-adapter.ts`,
`src/main/runtime/analytix-adapter.test.ts`, and
`scripts/scan-product-sovereignty.cjs` assert internal gating, real contract
canary coverage, rollback, TypeScript default preservation, no public Reasonix
protocol, no renderer/preload/settings/UI drift, and no new top-level product
entry.

Supersedes:
Superseded by:

## D-0070 - Production-Candidate Go Slice Stays Internal Until G6

Date: 2026-06-23
Sources: D-0241 Go production-candidate runtime parity slice, Reasonix provider/cache/gate/MCP/job review, Kun product baseline, analytix runtime contracts
Domain: runtime / provider parity / product sovereignty
Status: accepted

Conflict:

The Go sidecar now has live local provider, durable replay,
approval/user-input, fake MCP, and job-lineage code that is stronger than the
previous fixture scaffold. Promoting that directly to a user-visible backend or
Reasonix-style public protocol would bypass the Kun-derived desktop baseline,
settings/bridge contracts, packaged QA, and G6 retirement conditions.

Decision:

Add an internal production-candidate route family under
`/v1/internal/go-production-candidate/*` and a dual-env main adapter gate:
`ANALYTIX_RUNTIME_BACKEND=go-production-candidate` plus
`ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE=1`. The canary must prove fake live
provider streaming/cache telemetry, durable replay, approval/user-input
manager behavior, fake MCP lifecycle, job lineage, and single-baseline cleanup.
Any failure stops Go, cleans temp state, records fallback status, and returns
to TypeScript.

Rationale:

This moves Go beyond fixture proof without leaking a partial runtime into the
desktop product. It lets analytix absorb Reasonix engine strengths as
implementation code while preserving `window.analytix`, top-level `runtime`
settings, the Kun-derived UI/workflow surface, and the TypeScript default until
G6.

Validation:

`packages/runtime-go/live_production_candidate.go`,
`packages/runtime-go/live_production_candidate_test.go`,
`packages/runtime/tests/go-production-candidate-conformance.test.ts`,
`src/main/runtime/analytix-adapter.ts`,
`src/main/runtime/analytix-adapter.test.ts`, and
`scripts/scan-product-sovereignty.cjs` assert internal routing, no default Go
backend, no renderer-visible route, no Reasonix public protocol, rollback, and
no renderer/preload/settings/UI drift.

Supersedes:
Superseded by:

## D-0069 - G6 Cleanup Proof Does Not Delete The Default Runtime

Date: 2026-06-23
Sources: D-0240 G6 retirement cleanup proof, D-0238 backend-neutral adapter gate, D-0239 Go kernel scaffold, analytix runtime contracts
Domain: runtime / migration cleanup / product sovereignty
Status: accepted

Conflict:

The Go conformance scaffold now has enough evidence to name the paths that
should disappear after G6. Deleting TypeScript runtime paths now would break
the current production default; keeping every temporary conformance path
forever would create the long-term dual architecture the migration is trying
to avoid.

Decision:

Add an executable cleanup oracle under
`/v1/conformance/kernel/g6-retirement-cleanup`. It must report that G6 is not
ready, the TypeScript runtime is still retained, immediate production deletion
is forbidden, no public redundant surfaces are present, and the post-G6 delete
list is explicit.

Rationale:

This lets analytix prove cleanup intent without turning conformance scaffolds
into product features or removing the current default runtime prematurely.

Validation:

`packages/runtime-go/live_local_kernel.go`,
`packages/runtime-go/live_local_kernel_test.go`,
`packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts`,
`packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-oracle.json`,
and `scripts/scan-product-sovereignty.cjs` assert the cleanup counts, retained
paths, delete-after-G6 ids, forbidden redundancy absence, and open G6 blockers.

Supersedes:
Superseded by:

## D-0068 - Kernel Scaffold Does Not Expose Job Or Subagent Product Routes

Date: 2026-06-23
Sources: D-0239 Go kernel live scaffold, Reasonix internal/jobs, Reasonix subagent store, Kun product baseline
Domain: runtime / job orchestration / product surface
Status: accepted

Conflict:

Reasonix has stronger session-scoped jobs and subagent continuation/fork
lineage semantics. Exposing those as top-level analytix navigation, public job
routes, or a renderer-visible subagent protocol before G6 would conflict with
the Kun-derived desktop baseline and analytix runtime contract.

Decision:

Absorb the kernel structure first as analytix-owned conformance evidence below
`/v1/conformance/kernel/*`. The D-0239 scaffold may prove component mapping,
lineage requirements, cache/gate/MCP/job orchestration boundaries, and
forbidden surface counts. It must not execute live jobs, expose top-level
Subagent/Workflow/AutoResearch/MCP-indexer entries, or import Reasonix
SessionAPI/config/public routes.

Rationale:

This keeps Go runtime progress moving toward G6 while preserving product
sovereignty and avoiding a partial public protocol. Production job/subagent
execution requires permission gates, crash recovery, packaged QA, rollback,
and desktop rendering evidence.

Validation:

`packages/runtime-go/live_local_kernel.go`,
`packages/runtime-go/live_local_kernel_test.go`,
`packages/runtime/tests/go-kernel-live-scaffold-conformance.test.ts`, and
`packages/runtime/src/conformance/fixtures/go-kernel-live-scaffold-oracle.json`
assert fixture-only kernel routes, no public subagent protocol, no top-level
routes, no default backend, no renderer-visible Go route, and zero execution
side effects.

Supersedes:
Superseded by:

## D-0067 - Internal Go Gate Is Not Product Backend Selection

Date: 2026-06-23
Sources: D-0238 backend-neutral adapter gate, D-0236 durable store, D-0237 minimal loop, analytix runtime contracts
Domain: runtime / desktop integration / product boundary
Status: accepted

Conflict:

The Go live-local sidecar is now strong enough to start from Electron main for
internal conformance checks. Treating that gate as a general backend selector
would expose a partial fixture-backed runtime as product behavior, conflict
with the Kun-derived desktop baseline, and make Reasonix-style protocol
surfaces tempting to leak into renderer/main contracts.

Decision:

Historical D-0065 decision: keep the early Go path behind a dual internal gate
while the product runtime remained TypeScript. Current release status supersedes
that decision: Go is the default runtime core,
`ANALYTIX_RUNTIME_BACKEND=typescript` is a retired backend rejected by the
desktop adapter, and the remaining sidecar entry is test/conformance-only.

Rationale:

This let analytix exercise the backend-neutral adapter and Reasonix-inspired
controller/event-sink/gate discipline from the desktop boundary without
changing renderer/preload/settings/UI contracts before Go default delivery.

Validation:

`src/main/runtime/analytix-adapter.ts`,
`src/main/runtime/analytix-adapter.test.ts`, and
`packages/runtime-go/cmd/contract-sidecar/main.go` now preserve the
test/conformance-only sidecar boundary. Current production evidence is the
formal `runtime-go-*` validation command set and `cmd/runtime-server`.

Supersedes:
Superseded by: 2026-06-25 Go default delivery and the formal runtime-go
validation command set.

## D-0066 - Go Durable And Loop Boundaries Are Route-Specific

Date: 2026-06-23
Sources: D-0236 Go temp durable store, D-0237 Go minimal loop skeleton, analytix runtime contracts
Domain: runtime / Go conformance boundary
Status: accepted

Conflict:

The generic `LiveLocalSidecarProductBoundary()` describes the G2/G3/G4
live-local harness, which can run without the temp durable store. Durable and
minimal loop routes, however, intentionally require the D-0236 temp durable
store. Reusing the generic boundary inside durable/loop route responses makes
`tempDurableStorePrototype:false` look like durable replay is disabled.

Decision:

Keep the generic sidecar boundary unchanged for G2/G3/G4 replay, and add
route-specific boundaries for durable and loop conformance routes:
`LiveLocalSidecarDurableProductBoundary()` reports
`tempDurableStorePrototype:true`, while
`LiveLocalSidecarLoopProductBoundary()` reports both
`tempDurableStorePrototype:true` and `minimalAgentLoopPrototype:true`.

Rationale:

The route-specific boundary preserves existing G2/G3/G4 proof semantics while
making D-0236/D-0237 durable behavior explicit. This avoids a false reading
that durable event replay or the minimal loop proof is inactive, without
turning Go into a production backend or changing renderer/main contracts.

Validation:

`packages/runtime-go/live_local_durable.go`,
`packages/runtime-go/live_local_loop.go`,
`packages/runtime-go/live_local_loop_test.go`,
`packages/runtime/tests/go-minimal-agent-loop-conformance.test.ts`, and
`packages/runtime/tests/go-runtime-conformance.test.ts` assert the
route-specific boundaries and the unchanged generic boundary.

Supersedes:
Superseded by:

## D-0001 - analytix specs outrank upstream structure

Date: 2026-06-20
Sources: Kun, Reasonix, CodexDesktop-Rebuild, analytix specs
Domain: governance
Status: accepted

Conflict:

Kun and Reasonix may introduce useful changes that conflict with analytix
identity, settings, bridge, runtime package, event contracts, or UI structure.

Decision:

analytix specs and shared contracts decide how upstream changes land. Upstream
code may be absorbed only as capability, implementation technique, or reference
design.

Rationale:

analytix must become an independent product, not a mixed fork where whichever
upstream changed last controls the architecture.

Validation:

Every upstream batch must update a sync ledger and classify conflicts.

## D-0002 - analytix UI is the product baseline

Date: 2026-06-20
Sources: Kun, analytix UI, CodexDesktop-Rebuild
Domain: UI
Status: accepted

Conflict:

Future Kun changes may include UI layout or interaction changes that overlap
with the upgraded analytix UI.

Decision:

Kun UI changes are not copied over analytix UI by default. Product behavior may
be absorbed, but the final interaction and layout must fit analytix UI and
visual system.

Rationale:

The analytix UI is already an upgraded product asset. Reverting to upstream UI
would weaken the new product identity and destabilize chat hot-path work.

Validation:

Accepted Kun workflow changes must include analytix UI review and relevant
component tests or desktop QA.

## D-0003 - Reasonix engine ideas must conform to analytix runtime contracts

Date: 2026-06-20
Sources: Reasonix, analytix runtime contracts
Domain: runtime
Status: accepted

Conflict:

Reasonix may provide stronger engine behavior or Go implementation, but its
native event shapes and CLI/settings assumptions may differ from analytix.

Decision:

Reasonix behavior must be adapted behind analytix HTTP/SSE or app-server
contracts. Renderer code must not consume Reasonix-native protocol shapes.

Rationale:

This keeps the renderer backend-neutral and allows Go runtime work without
creating a second product.

Validation:

Reasonix-derived runtime changes need contract tests or conformance fixtures.

## D-0004 - stricter permission and privacy behavior wins

Date: 2026-06-20
Sources: Kun, Reasonix, analytix runtime
Domain: security
Status: accepted

Conflict:

Upstream agent behavior may be more permissive with tools, files, shell, MCP,
approvals, user input, or traces.

Decision:

analytix keeps the stricter and more user-controllable behavior unless a
separate security review intentionally changes the contract.

Rationale:

Desktop agent trust depends on local control, predictable approvals, and
privacy-bounded traces.

Validation:

Permission-affecting changes require tests for approvals, user input, sandbox,
and trace sanitization.

## D-0005 - performance claims require analytix evidence

Date: 2026-06-20
Sources: Kun, Reasonix, CodexDesktop-Rebuild, analytix QA
Domain: performance
Status: accepted

Conflict:

Upstreams may claim speed or smoothness improvements that do not automatically
apply to analytix desktop hot paths.

Decision:

Performance decisions require analytix benchmarks, local traces, or Electron
desktop QA evidence.

Rationale:

analytix bottlenecks often live in renderer projection, virtualizer layout,
Markdown/code finalization, terminal bursts, and panel isolation. Engine speed
alone does not prove desktop smoothness.

Validation:

Hot-path changes must include relevant trace, unit, renderer smoke, or desktop
QA evidence.

## D-0006 - checkpoint safety must be analytix-owned

Date: 2026-06-20
Sources: Kun git checkpoint, Reasonix checkpoint/rewind, analytix review/history
Domain: safety
Status: accepted

Conflict:

Kun provides a git-backed turn checkpoint idea, while Reasonix provides a
git-free snapshot/rewind engine. Both are useful, but either can leak upstream
identity or corrupt analytix history if copied directly.

Decision:

Checkpoint and rewind behavior must be adapted behind analytix review,
generated-files, thread history, and runtime event contracts. Future storage,
refs, commands, and UI must be analytix-owned; never use `refs/kun/checkpoints`
or expose Reasonix-native rewind protocols to the renderer.

Rationale:

Checkpoint is a product trust boundary. It must protect user files and
transcripts while preserving analytix identity, HTTP/SSE replay, approvals, and
desktop review UX.

Validation:

Future P4 work must cover code-only, conversation-only, and combined rewind;
event-log replay after rewind; resume after rewind; and identity scans.

## D-0007 - P4A checkpoint oracle is append-only and git-free

Date: 2026-06-20
Sources: Kun `8cbf8e6` git turn rollback checkpoints, Kun master `8602476`,
Reasonix `internal/checkpoint`, Reasonix `internal/control` rewind paths
Domain: safety / runtime history
Status: accepted

Conflict:

Kun proves product safety value with git-backed rollback and rescue
checkpoints, while Reasonix proves engine value with per-turn file snapshots
and rewind boundaries. P4A needs a safe analytix-owned oracle before it can
mutate files, rewrite transcripts, or add renderer controls.

Decision:

P4A starts with versioned analytix `axcp_` checkpoint metadata and an
append-only `checkpoint_captured` runtime event. The focused code slice is
metadata capture, changed-file normalization, workspace escape rejection,
first-touch-wins changed-file behavior, and a non-mutating conversation-only
rewind plan from the event log projection. P4A does not use git refs, does not
restore files, and does not rewrite user transcripts.

Rationale:

This gives analytix a crash-replayable contract: checkpoint state is visible in
the same durable event stream that backs thread replay, and future review UI can
prove what would be retained or removed before any destructive action exists.
It also avoids copying Kun identity such as `refs/kun/checkpoints` or Reasonix
CLI/settings/protocol surfaces.

Validation:

`packages/runtime/tests/checkpoint-rewind-oracle.test.ts` is the P4A oracle
fixture. Future P4B must add combined code+conversation restore behind
analytix-owned storage/refs only, with idempotent schema migration tests before
any data rewrite.

## D-0008 - P4B rewind plans are auditable and non-destructive

Date: 2026-06-20
Sources: P4A oracle, Kun git rollback checkpoints, Reasonix checkpoint/rewind,
analytix review/generated-files/history UI
Domain: safety / runtime history / UI
Status: accepted

Conflict:

Kun's rollback work proves user value through executable git restore, while
Reasonix proves engine value through checkpoint/rewind machinery. Moving
directly from P4A metadata to destructive restore would risk silently rewriting
workspace files or durable conversation history before analytix has desktop QA
and apply confirmation.

Decision:

P4B exposes only an auditable `axrp_` rewind plan. The plan can cover
code-only, conversation-only, or combined scopes, but it is always
`applyMode: plan_only` and `destructive: false`. Path escape, absolute
persisted paths, and symlink risks are blocked or forced to manual review.
Existing analytix review/history/ChangeInspector surfaces may display the plan,
but P4B must not apply files, rewrite `events.jsonl`, rewrite `messages.jsonl`,
or create git refs.

Rationale:

This gives users and future automation a reviewable boundary while preserving
data ownership. It keeps the renderer backend-neutral and avoids copying Kun
refs or Reasonix-native renderer protocols.

Validation:

`packages/runtime/tests/checkpoint-rewind-plan.test.ts` covers code-only,
conversation-only, combined, path escape, absolute path, symlink risk, and
prompt/content non-leakage. Desktop IPC/provider/UI projection tests cover the
route and review display boundary.

## D-0009 - P4C destructive apply is confirmed, rescued, and append-only audited

Date: 2026-06-20
Sources: P4B `CheckpointRewindPlan`, Kun git rollback checkpoints, Reasonix
checkpoint/rewind engine, analytix review/generated-files/history UI
Domain: safety / restore apply / crash recovery / runtime history
Status: accepted

Conflict:

P4C must finally perform destructive restore/apply, but copying Kun's direct git
rollback or Reasonix-native rewind protocol could silently overwrite user files
or rewrite transcript history outside analytix ownership.

Decision:

P4C apply must execute only from a reviewed P4B `CheckpointRewindPlan`; no
checkpoint-only shortcut is allowed. Apply requires an explicit destructive
confirmation phrase, validates the submitted plan against the current
checkpoint/event-derived safety plan, blocks stale/tampered plans, and blocks
path escape, absolute paths, symlinks, staged/untracked conflicts,
manual-review files, missing snapshot/hash evidence, and snapshot content/hash
mismatches. Parent-directory symlinks are blocked as path escapes, and safety is
revalidated inside the file mutation queue before each write/delete. Before any
file mutation, analytix records an `axrr_` rescue record. Apply results use
`axra_` ids and are persisted as append-only `checkpoint_rewind_applied` audit events.
If a later mutation fails after an earlier file was applied, the audit result
must preserve the already-applied file status instead of reporting zero applied.
Conversation restore does not rewrite `events.jsonl` or `messages.jsonl`; it is
recorded as append-only audit until a separately proven migration/projection
strategy exists.

Rationale:

This is the smallest safe destructive boundary: every mutation is explainable
from the plan, recoverable from an analytix-owned rescue record, and replayable
through the existing runtime event stream. It preserves UI/contract sovereignty
and avoids upstream identity or protocol leakage.

Validation:

`packages/runtime/tests/checkpoint-rewind-apply.test.ts` covers successful
apply, blocked/manual review, path escape, parent/final symlinks, staged/untracked
conflicts, snapshot content/hash mismatch, created delete, modified restore,
deleted restore, conversation/combined apply, partial failure audit, rescue/crash-recovery
evidence, route parsing, and idempotency. The closure suite re-runs
P4A/P4B/P3A/P2.2/P1.3/P1.2/P1.1 regressions, typechecks, `git diff --check`,
production identity/protocol scan, and real desktop smoke or an explicit desktop
blocker record.

## D-0010 - G0/G5 inventory closes the oracle baseline, not Go scaffold

Date: 2026-06-20
Sources: P4C confirmed apply, runtime suite baseline, Kun 0.2.13/0.2.14
lineage, Reasonix `main-v2`, Go runtime conformance plan
Domain: runtime contracts / Go staging / upstream absorption
Status: accepted

Conflict:

After P4C, it is tempting to jump directly into a Go runtime scaffold because
checkpoint/rewind/apply has strong TypeScript fixtures and Reasonix offers Go
engine patterns. Doing that before freezing all runtime contracts would create
a second backend that can pass narrow checkpoint tests while drifting on
thread/session, SSE replay, tools, approvals, user input, plan/goal,
model-history repair, cache accounting, usage, and provider parsing.

Decision:

G0/G5 inventory may close only the conformance baseline: the TypeScript runtime
remains the oracle, Kun and Reasonix capabilities are classified, fixture gaps
are listed, and missing production evidence is recorded. It must not introduce
a Go backend scaffold, Rust helper, backend switch, renderer/preload protocol
change, or Reasonix/Kun public identity. G1/G2/G5 implementation work requires
a separate batch that starts from the recorded fixture gaps.

Rationale:

This keeps backend evolution contract-first. It lets analytix absorb Kun
product lineage and Reasonix engine discipline without weakening the current
desktop architecture or making unverified superiority claims.

Validation:

The inventory batch must at minimum pass the full runtime suite, runtime
typecheck, app typecheck, relevant renderer/main focused tests, `git diff
--check`, and production identity/protocol scan. Release and Go parity remain
incomplete until Spec 07 blockers, live apply/crash QA, packaged QA, and
cross-backend route fixtures are satisfied.

## D-0011 - Reasonix goal/control FSM is absorbed as analytix-owned loop state

Date: 2026-06-20
Sources: Reasonix `dbaea8433380a4351fd1c2362ea9df175cb61e51`,
`bb06f5b4b6c78d58147799130dcd51db0c5e8913`,
`c23d40f74ca998941238154d6c0422676b139673`,
`726036bd15e09a208a59d3685b9de0dcb8c9811b`, Reasonix `main-v2`
`bc8249c307b261ef7ad05a0d2d1409c7b83b95ff`, G0/G5 runtime baseline
Domain: goal/control runtime / approval drift / Go conformance
Status: accepted

Conflict:

Reasonix improves controller maintainability by extracting goal state and, at
the refreshed HEAD, approval bookkeeping into Go collaborators. Copying those
public shapes directly would introduce Reasonix-native goal markers, sidecar
state, controller-lock assumptions, or approval/user-input protocol semantics
that are not part of analytix's Electron/React/TypeScript runtime contract.

Decision:

Absorb `dbaea843` as an analytix-owned internal helper only. The TypeScript
runtime now owns goal continuation state in `GoalControlMachine`, covering
active-goal instruction construction, blocked audit guidance, no-tool recovery,
empty post-`file_change` recovery/failure, and non-progress goal tool
classification. Durable state remains `ThreadGoal` plus `goal_updated` events;
turn failure remains analytix `TurnService` / HTTP/SSE behavior. Do not expose
Reasonix `[goal:*]` markers, Reasonix goal-state sidecars, Reasonix CLI/settings
shapes, or Go controller locks.

Classify `bb06f5b4` as Reasonix repo hygiene with no analytix code action.
Record `726036bd` / `bc8249c3` approvalManager as real upstream drift but defer
it to a future approval-control batch because approval routes, renderer cards,
user-input, and denial semantics need their own focused regression suite.

Rationale:

This preserves the useful engineering separation from Reasonix while keeping
analytix product sovereignty: `window.analytix`, top-level `runtime` settings,
`analytix serve`, `ANALYTIX_*`, current Electron/React/TypeScript UI ownership,
and backend-neutral runtime HTTP/SSE remain the public domains. It also gives a
future Go runtime a sharper oracle without letting Go implementation details
drive today's renderer contract.

Validation:

The scoped code slice must pass
`npm --prefix packages/runtime run test -- tests/loop.test.ts tests/goal-tools.test.ts tests/goal-repetition-guard.test.ts --no-file-parallelism --maxWorkers=1`,
then the normal runtime/app typechecks, focused main/renderer tests,
`git diff --check`, and identity/protocol scan before closure.

## D-0012 - Kun product capabilities keep their target-version entry position

Date: 2026-06-20
Sources: Kun `v0.2.13` `201a1469ffbd911f6b95d450b78471e51b42acae`,
Kun `v0.2.14` `06be05d76223208724c07301fa0f830641a06e6f`,
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`,
P0 product-entry correction
Domain: product UI / upstream absorption / route governance
Status: accepted

Conflict:

Kun current/master contains Workflow/Create Loop as a later product surface,
but Kun 0.2.13 and 0.2.14 do not contain an equivalent top-level Workflow
route or sidebar command. Analytix incorrectly promoted that later capability
into a current top-level navigation entry without proving that the target Kun
baseline had the same product position or that an analytix spec approved a
different entry level.

Decision:

Future Kun product absorption must preserve the target Kun version's product
position, entry layer, and trigger path when claiming Kun parity. A
later/current Kun capability cannot be represented as a 0.2.13/0.2.14 baseline
entry. Moving a capability to a higher or different analytix entry level
requires an independent spec decision, explicit UX rationale, renderer tests
for Sidebar, Workbench, and route state, desktop QA, and scan updates. The P0
correction removes/hides the analytix top-level Workflow route, Sidebar
command, and Workbench stage while retaining Create Loop internals as unexposed
future capability.

Rationale:

This prevents upstream absorption from becoming product invention disguised as
parity. It keeps analytix sovereign while still allowing valuable Kun current
features to be productized deliberately through specs, tests, and desktop QA.

Validation:

The P0 correction must include route/action/sidebar/workbench tests or guards,
document the Kun 0.2.13/0.2.14/master evidence paths, and pass identity scans
that reject `kun serve`, `window.kunGui`, `agents.kun`, and `KUN_*` leakage.

## D-0013 - Reasonix engine absorption requires benchmark proof

Date: 2026-06-21
Sources: Reasonix `main-v2` currentness recheck
`49c14762b7da9234525e717830e39a64a2220911`, P2.2 cache diagnostics,
spec 08, spec 09
Domain: engine/runtime / provider cache / benchmark governance
Status: accepted

Conflict:

Reasonix is valuable as an engine/runtime upstream and can be code-reused in
analytix. However, directly copying engine helpers without benchmark proof can
create a false "stronger than Reasonix" claim, can optimize only DeepSeek while
regressing other providers, or can leak Reasonix protocol assumptions into
analytix contracts.

Decision:

Reasonix absorption is complete only when the absorbed capability is adapted
behind analytix contracts and has proof for the affected dimension. For
cache/provider work, proof must include stable prefix shape, canonical tool
schema hashing, provider/model/endpoint attribution, provider-native
cache-hit/cache-miss parsing where available, graceful unsupported behavior for
providers without equivalent fields, and non-regression for tools, approvals,
user input, generated files, review UI, and non-DeepSeek providers.

Code-level reuse is allowed, including helper logic, fixtures, parsers, and
future Go modules, but Reasonix public protocol, settings root, CLI identity,
terminal-only product shape, and renderer-visible event semantics remain
rejected.

Rationale:

The target is not "more copied code"; it is an analytix runtime that is at
least as strong as Reasonix for the absorbed engine capability and stronger as
a multi-provider desktop product. Benchmark and scorecard evidence prevent
DeepSeek-specific optimizations from silently weakening OpenAI, Anthropic,
OpenAI-compatible, custom endpoint, approval, or tool flows.

Validation:

Every Reasonix engine batch must update the relevant sync ledger and scorecard
and run focused tests for the changed surface. Provider/cache batches must run
cache diagnostics and provider matrix tests or explicitly record missing live
credentials as a blocker rather than claiming benchmark closure. The first
fixture-backed P2.2 proof command is:

```text
npm --prefix packages/runtime run test -- tests/provider-cache-proof.test.ts tests/cache.test.ts tests/usage-service.test.ts --no-file-parallelism --maxWorkers=1
```

## D-0014 - Reasonix agent-kernel absorption order

Date: 2026-06-21
Sources: Reasonix `main-v2` currentness recheck
`49c14762b7da9234525e717830e39a64a2220911`, spec 08 section 9.1,
spec 09 section 4.1
Domain: engine/runtime / permission / task execution / Go runtime governance
Status: accepted

Conflict:

Reasonix is stronger than analytix in several agent/runtime kernel areas, but
the value is not merely sub-agents or DeepSeek cache. If analytix enables
sub-agent or parallel execution before task closure, permissions, evidence, and
Goal state are proven, the result is an uncontrolled execution amplifier rather
than a safer product. Directly copying Reasonix terminal UX, public protocol,
settings root, CLI identity, or renderer-visible event semantics would also
violate analytix product sovereignty.

Decision:

Reasonix agent-kernel may be treated as a first-class engine/runtime source,
but only behind analytix-owned contracts. The future absorption order is:

| Order | Batch | Required contents |
| --- | --- | --- |
| 1 | Task closure and permission kernel | `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan approval separated from tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules. |
| 2 | Long-running task system | `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, and requirement-by-requirement evidence audit. |
| 3 | Tool surface and context economy | token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory on-demand retrieval, compaction archive, and cache diagnostics across providers. |
| 4 | Collaborative execution model | `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, and nested event rendering. |
| 5 | Go runtime kernel | Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime behind TS oracle G0-G6. |

Batch 1 is mandatory before sub-agent or `parallel_tasks` expansion. Batch 4
cannot be claimed by merely enabling `subagents.enabled` or `delegate_task`.
Batch 5 cannot start as a default backend or renderer-visible protocol change.

Rejected surfaces:

- Reasonix terminal product shape;
- Reasonix public protocol, settings root, CLI identity, and renderer-visible
  event semantics;
- top-level AutoResearch, Workflow, or Subagent UI navigation;
- DeepSeek-only provider/runtime assumptions;
- Go/Rust/Tauri scaffolds that bypass analytix HTTP/SSE, `window.analytix`,
  top-level `runtime`, or `analytix serve`;
- Kun identity leakage such as `kun serve`, `window.kunGui`, `agents.kun`, or
  `KUN_*` in production code.

Rationale:

This order lets analytix absorb Reasonix's real engine strength while keeping
Kun/analytix desktop workflow as product sovereignty. Permissions, completion,
evidence, and Goal state make collaboration safe; tool/context economy makes
it efficient across providers; Go runtime work then has stable oracle fixtures
instead of becoming a second product.

Validation:

Each batch must update specs, sync ledgers, implementation plan, benchmark
scorecard, and relevant tests or explicit deferrals. No release note may claim
Reasonix parity or superiority without batch-specific proof and scorecard
evidence.

Batch 1 implementation addendum (2026-06-21):

The first task-closure and permission-kernel slice is accepted only as a scoped
TypeScript runtime proof. It adds `complete_step` and an append-only
`ThreadGoal.evidenceLedger`, rejects goal completion without evidence at the
tool and `ThreadService` layers, and adds focused fixtures for:

- denied tool approval must not execute;
- approval posture mapping for ask / auto / yolo without exposing a Reasonix
  public posture protocol;
- plan approval separated from tool approval;
- `complete_step` evidence before `update_goal complete`;
- blocked-state transition with `goal_updated` and
  `goal_auto_resume_exhausted`;
- headless/subagent inheritance of the parent approval policy.

This addendum does not close all of Batch 1. Route-level approval regression,
renderer approval/review non-regression, fuller user-input coverage, broader
runtime/app validation, and release evidence gates remain required. It also
does not authorize AutoResearch, `parallel_tasks`, background jobs, Go/Rust
scaffold, a renderer-visible Reasonix event protocol, or a top-level Workflow /
Subagent / AutoResearch entry.

Batch 2 implementation addendum (2026-06-21):

The first long-running task slice is accepted only as a scoped TypeScript
runtime proof. It adds `/goal --research` through the existing chat/goal
surface and records analytix-owned project-local state under
`.analytix/autoresearch/<threadId>/`:

- `task_spec.md`;
- `progress.json`;
- `findings.jsonl`;
- `directions_tried.json`;
- `iteration_log.jsonl`.

It also adds `record_research_direction` for attempted directions, extends
`complete_step` with `requirement_id`, and rejects research-goal completion
until every requirement has evidence. AutoResearch state is not allowed to
pollute `REASONIX.md`, `AGENTS.md`, the stable system prefix, dynamic tool
schema, renderer-visible Reasonix protocol, or any new top-level UI entry.

This addendum does not close full Reasonix long-task parity. It does not
authorize Batch 4 `parallel_tasks`, background jobs, planner/executor
Coordinator, Go/Rust scaffold, live-provider claims, Reasonix terminal product
shape, or a top-level Workflow / Subagent / AutoResearch entry.

Batch 3 implementation addendum (2026-06-21):

The first tool-surface and context-economy slice is accepted only as a scoped
TypeScript runtime proof. It adds dynamic tool-source lifecycle semantics to
the analytix-owned registry through `connectToolSource` / `disconnectToolSource`
and diagnostics that report tool count, tool names, and catalog fingerprints.
Canonical tool catalog fingerprints stay stable across dynamic source
connection order, while provider-owned advertisement order remains intact.

Cache diagnostics now report tool-source lifecycle separately through
`toolSourceChanged`, `toolSourceChangeReasons`, `toolSourcesHash`, and
`toolSourceIds`. These diagnostics must not count as stable-prefix drift when
the model-visible tool schemas are unchanged, and they must not leak raw prompt
text, tool descriptions, tool arguments, tool outputs, provider bodies, or
secrets.

Existing token economy, request-history hygiene, memory retrieval,
compaction-history/archive, usage accounting, and provider-cache proof fixtures
remain required. Unsupported provider cache telemetry remains unknown rather
than 0% hit or all miss. This addendum does not authorize provider request
body/header/stream rewrites, DeepSeek-only behavior, Batch 4 collaborative
execution, Go/Rust scaffold, Reasonix protocol exposure, or new top-level UI
entries.

Batch 4 implementation addendum (2026-06-21):

The first collaborative-execution slice is accepted only as a scoped
TypeScript runtime proof. It does not add a `parallel_tasks` product surface,
background Job Manager, planner/executor Coordinator, or top-level Subagent UI.
Instead, the existing `delegate_task` substrate now hands completed child work
back into the Batch 1 task-closure kernel: when a parent Goal is active, the
completed child run can append evidence to the parent `ThreadGoal.evidenceLedger`.

Child lifecycle events and projections carry optional `evidenceLedgered` /
`evidenceLedgerError` metadata. Existing maxParallel queueing, queued abort,
child failure, parent interruption, and single-message delegate fan-out
fixtures remain part of the proof. This addendum does not authorize Reasonix
public protocol, terminal UI shape, new top-level navigation, Go/Rust scaffold,
or claims that `delegate_task` alone equals Reasonix `task` / `parallel_tasks`
parity.

Batch 5 implementation addendum (2026-06-21):

The first Go runtime kernel slice is accepted only as G0/G1 shadow
conformance. It adds
`packages/runtime/src/conformance/go-runtime-kernel-conformance.ts` and
`packages/runtime/tests/go-runtime-conformance.test.ts` to map Provider
Registry, Tool Registry, Controller, Session, Event Sink, Job Manager,
Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime to
current TypeScript oracle tests. It also pins `/health`, `/v1/runtime/info`,
and `/v1/runtime/tools` as the G1 health/config/capabilities shadow route
oracle.

The later G1 shadow implementation is allowed to live in `packages/runtime-go`
with its own `go.mod` only as conformance-only handler/test code for those
three routes. This does not authorize default backend selection, Electron main
integration, renderer-visible Go routes, Go G2+ route migration, Reasonix
public protocol, Rust scaffold, Tauri migration, or bridge/settings changes.
Future Go work remains shadow-only until cross-backend conformance fixtures,
rollback, desktop QA, and scorecard evidence exist.

## D-0015 - Go G2 shadow route replay is conformance-only until proven

Date: 2026-06-21
Sources: D-0014 Batch 5 G1 shadow scaffold, Reasonix `main-v2`
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e`, spec 08, Go conformance plan
Domain: Go runtime / route replay / backend neutrality
Status: accepted

Conflict:

After the G1 shadow package exists, it is tempting to expand it directly into
session, thread, and SSE routes or to wire Electron to a Go backend. At the same
time, Reasonix latest drift includes useful MCP/provider hardening that could
pressure Go G2/G3 work before live provider and route evidence are ready.

Decision:

Go G2 may be prepared only as shadow route replay against TypeScript oracle
fixtures for read/list/search/archive/fork/session routes and SSE replay. G2
must not change renderer, preload, Electron main, settings, `analytix serve`,
public event semantics, default backend selection, or product navigation. Any
G2 implementation must compare HTTP status, response schema, SSE event
sequence, durable event log, projected transcript/thread state, usage summary,
error shape, and sanitized logs/traces against the TypeScript runtime before
claiming parity.

Reasonix latest provider/MCP drift may inform future fixtures, but it does not
clear live provider matrix, MCP lifecycle UI, desktop QA, or release readiness
blockers. `packages/runtime-go` remains a shadow package until a later
spec-approved backend-selection and rollback plan exists.

Rationale:

G2 is the first stage that can easily become a second runtime surface. Keeping
it replay-only protects analytix product sovereignty while still letting Go
work progress from measured route equivalence instead of implementation
enthusiasm.

Validation:

This stage accepts only the shadow replay slice after running the TypeScript
route fixtures, the Go shadow route runner, bridge/domain scans,
default-backend scans, identity/protocol scans, `git diff --check`, and
scorecard/release evidence updates. The Go package was tested with a temporary
official `go1.26.4.darwin-arm64` toolchain from `go.dev` after SHA256
verification; the toolchain was not committed. Missing live provider matrix,
desktop backend selection, rollback, packaged QA, and release credentials remain
blockers rather than passing gates.

## D-0016 - Reasonix P0 Oracles And Kun Desktop Hardening Stay Behind Analytix Contracts

Date: 2026-06-21
Sources: Reasonix `main-v2` `91fe06db6177bb052fc8ae1a3081d60bfc104a4e`,
Kun `develop` `247076f297170c3d0c558baffb073c629894faea`, specs 08/09
Domain: provider/cache, task jobs, MCP known overrides, desktop shell, Go G3/G4
Status: accepted

Conflict:

Reasonix P0 engine parity work introduces concepts that could become new public
surfaces: first-class tasks, `parallel_tasks`, background jobs, planner/executor
coordination, MCP indexer startup policy, and Go provider/tool runtime code.
Kun desktop hardening also changes visible shell controls. These improvements
must not accidentally reintroduce top-level Workflow/Create Loop, Kun identity,
Reasonix public protocol, or a default Go backend.

Decision:

Accept the stage only as contract-owned analytix behavior:

- provider/cache parity is TypeScript-runtime behavior with sanitized
  diagnostics and multi-provider fixture coverage;
- task/background/planner work is an internal contract/oracle plus durable
  manager skeleton, not a product route;
- MCP known overrides apply only to recognized stdio indexer servers and
  preserve explicit user cwd;
- Go G3/G4 are shadow conformance outputs matching TS-owned fixtures;
- Kun SSE IPC and shell navigation hardening are desktop stability changes, not
  product-entry expansion.

Top-level Workflow/Create Loop, Subagent/AutoResearch/MCP indexer routes,
Reasonix settings/protocol/event shapes, Kun current-product naming, Rust/Tauri
rewrite, and default Go backend remain rejected.

Rationale:

This lets analytix meet the P0 engine parity proof while keeping the product
contract stable and preserving the Kun 0.2.13 -> 0.2.14 entry baseline. The
stronger-than-Reasonix pieces are the multi-provider cache matrix, sanitized
diagnostics, explicit-cwd-safe MCP override, and TS-owned Go fixture discipline;
none require exposing upstream-native protocols.

Validation:

Focused suites passed for provider/cache, MCP lifecycle, task job oracle, SSE
IPC, shell navigation, and Go G3/G4 shadow output. Final release evidence still
requires full command gates and production scans after docs edits. Live provider
/ MCP QA, packaged desktop QA, Windows/signing/release metadata, and Go G5
remain blockers.

## D-0017 - Reasonix 9e56 MCP And Task Job Closure Must Stay Internal

Date: 2026-06-21
Sources: Reasonix `main-v2`
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: MCP startup, task jobs, provider diagnostics, desktop shell, Go G5
Status: accepted

Conflict:

Reasonix 9e56 closes useful MCP and todo regressions, but the upstream shape
mixes runtime control, frontend retry actions, and task/job concepts that could
be misread as permission to expose new product surfaces. Analytix needs the
runtime parity benefits without reintroducing top-level Workflow/Create Loop,
Reasonix public protocol, or a second runtime backend.

Decision:

Accept only analytix-owned internal behavior:

- todo reload/archive fixes are record-only because analytix owns structured
  todo state;
- MCP lazy-mode removal is represented by the existing analytix enabled-server
  parallel startup and background reconnect contract, not Reasonix tier config;
- MCP retry-all and late-provider disable semantics are reimplemented through
  `CapabilityRegistry` and runtime provider callbacks;
- task-job wait/output/kill is allowed only under authenticated
  `/v1/runtime/task-jobs/*` routes;
- provider probe/model-list diagnostics must redact request URL, response
  excerpt, and network-error secrets;
- Kun contributes shell safe-area closure only in this stage;
- Go remains TS-owned oracle inventory only unless a later backend-selection
  spec authorizes more.

Rationale:

This gives analytix real runtime progress while keeping product ownership
stable. Internal task-job routes are a runtime control primitive; they are not
a Reasonix task protocol, not a renderer-visible workflow product, and not a
Subagent/AutoResearch entry.

Validation:

Focused runtime tests cover registry suspension, retry-all MCP reconnect,
task-job route auth/output/wait/kill/404, and provider probe redaction. Final
stage closure still requires full runtime/app tests, typecheck, build:runtime,
production scans, docs/scorecard consistency, and release evidence updates.

Remaining blockers:

Full Reasonix `task` / `parallel_tasks` / planner-executor parity, live
MCP/indexer QA, live provider/cache superiority, Local Whisper/tray parity,
Go G5/full loop, packaged desktop QA, signing, Windows, release metadata, and
default Go backend readiness remain open.

## D-0018 - Collaborative Execution And Go G5 Proof Stay Oracle-First

Date: 2026-06-21
Sources: Reasonix `main-v2` `9e56c3276880ced538b3375329b8a0ebbe63a67b`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: collaborative execution, provider cache curve, MCP/indexer lifecycle,
Go G5, desktop shell/Kun delta
Status: accepted

Conflict:

Reasonix first-class `task`, `parallel_tasks`, background jobs, planner
coordination, MCP startup/indexer policy, and Go runtime proof are high-value
engine capabilities. They also resemble new public protocol/product surfaces
if copied directly. Kun tray session menu and Local Whisper are useful desktop
delta candidates, but they add platform, packaging, permission, and release
surface area.

Decision:

Accept only analytix-owned oracle-first behavior in this stage:

- internal task/parallel contracts are constants and fixtures, not public
  routes or top-level navigation;
- `depends_on` normalization and invalid dependency rejection are runtime
  guardrails behind analytix job contracts;
- cache curve guard is offline/golden and cannot justify live superiority;
- MCP retry-all, suspended-source tombstones, and codegraph/codebase-memory
  overrides stay behind the existing MCP config/provider contracts;
- Go G5 is a TypeScript-owned oracle inventory only;
- Kun shell safe-area is absorbed, while tray session menu and Local Whisper
  remain future spec/oracle/import-boundary work.

Rationale:

This lets analytix advance toward Reasonix collaborative-execution parity while
preserving the stable app architecture and Kun 0.2.13 -> 0.2.14 product-entry
baseline. The stage improves proof depth but deliberately avoids changing
public protocols, renderer navigation, backend defaults, packaging resources,
or release claims.

Validation:

Focused runtime tests cover task-job oracle, provider-cache curve guard,
MCP lifecycle/indexer fixture, Go G5 inventory binding, and Go G3/G4 fixture
consistency. Shell safe-area focused tests and typecheck passed before the
runtime work continued. Full final gates remain recorded in the release
evidence file.

Remaining blockers:

Full planner/executor implementation, restart/resume crash drill, desktop
nested-card QA, live provider/MCP matrix, tray session menu QA, Local Whisper
import boundary, Go full loop implementation, default backend selection,
packaged QA, signing, Windows, and release metadata remain open.

## D-0019 - Executable Runtime And Kun Tray Must Stay Behind Analytix Contracts

Date: 2026-06-21
Sources: Reasonix `main-v2` `9e56c3276880ced538b3375329b8a0ebbe63a67b`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: executable task graph, DeepSeek cache guard, MCP live-local indexer,
Kun tray session menu, Go G5
Status: accepted

Conflict:

Reasonix has valuable executable task/parallel/job/cache/MCP behavior, and Kun
has a useful tray session-menu desktop affordance. Copying either shape
directly would risk exposing Reasonix protocol, Kun bridge/identity, new
top-level workflow navigation, or a premature second backend.

Decision:

Accept only analytix-owned executable and desktop behavior:

- internal `task` and `parallel_tasks` tools may be registered only through
  analytix runtime providers and backed by `DelegationRuntime`;
- job output offsets and restart stale-job reconciliation are runtime
  reliability features, not a public Reasonix job protocol;
- the cache curve guard must live under analytix runtime code and remain
  offline/golden unless credentialed live provider evidence exists;
- MCP live-local proof uses fake local fixtures and analytix diagnostics, not
  Reasonix plugin protocol;
- the tray session menu is main-process-only and opens existing analytix
  runtime threads without adding renderer routes or bridge aliases;
- Go G5 implementation is blocked on this machine because `go` is unavailable,
  so no Go files may change in this stage.

Rationale:

This advances the executable internal runtime and desktop convenience surface
while preserving the product architecture: `window.analytix`, top-level
`runtime` settings, `analytix serve`, and Renderer -> preload -> main ->
runtime HTTP/SSE remain authoritative. The Kun 0.2.13 -> 0.2.14 product-entry
baseline remains intact.

Validation:

Focused runtime and desktop suites passed for task/parallel execution, cache
curve guard, MCP live-local fixture, and tray session menu. Full command gates
are recorded in release evidence after the final rerun. Go validation is not
available locally and no Go files changed.

Remaining blockers:

Full Reasonix planner/executor parity, runner rehydration, desktop nested-card
QA, live provider/cache superiority, live MCP/indexer parity, packaged tray QA,
Local Whisper, Go G5 implementation with a Go toolchain, packaged release QA,
signing, Windows, and release metadata remain open.

## D-0020 - Go G5 Shadow Output Must Not Become A Backend Claim

Date: 2026-06-21
Sources: Reasonix `main-v2`
`9e56c3276880ced538b3375329b8a0ebbe63a67b..bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: Go G5, Reasonix shell/sandbox drift, backend boundary
Status: accepted

Conflict:

Reasonix advanced with useful PowerShell compatibility work while analytix was
moving Go G5 from inventory toward implementation. There is a risk that a
tested Go shadow output could be overread as a backend, and a risk that
Reasonix shell/sandbox fixes could be copied without a Windows runtime-shell
contract.

Decision:

Accept only:

- currentness classification for Reasonix bfe398 PowerShell compatibility;
- a Go G5 shadow output slice that replays TS-owned oracle inventory;
- temporary SHA-verified official Go toolchain use for `gofmt` and
  `go test ./...`;
- product boundary flags that keep Electron main disconnected, renderer-visible
  Go routes disallowed, Reasonix public protocol disallowed, and default Go
  backend disabled.

Reject in this stage:

- Reasonix shell/sandbox code copy;
- Go full-loop executor claims;
- Electron-to-Go routing;
- default Go backend readiness;
- live provider/MCP parity or release readiness.

Rationale:

The G5 slice is useful only because it keeps TypeScript fixtures authoritative.
It proves Go can replay the required jobs/cache/session/resume/interrupt/MCP
inventory, not that Go can run the analytix runtime.

Validation:

Focused TS conformance and `packages/runtime-go` package tests pass with the
temporary Go toolchain. Full final gates are recorded in release evidence.

Remaining blockers:

Live Go runtime behavior, rollback/default backend plan, packaged QA, live
provider/MCP matrix, and Windows shell-tool compatibility remain open.

## D-0021 - G5 Composite Replay Remains Conformance-Only

Date: 2026-06-21
Sources: Reasonix `main-v2` `bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: Go G5, cross-fixture replay, backend boundary
Status: accepted

Conflict:

Cross-fixture Go replay makes G5 evidence much stronger than a static
inventory. That strength could be misread as permission to route Electron or
`analytix serve` through Go.

Decision:

Accept composite replay only as conformance evidence:

- Go may read task-job, provider-cache, G2 route replay, and MCP lifecycle
  fixtures;
- Go may emit `shadowSlicesExpectedOutput` for jobs/cache/session/MCP;
- TypeScript tests must keep the expected output tied to the source fixtures;
- Electron main, renderer routes, public protocol, and backend defaults remain
  unchanged.

Reject:

- live Go full-loop executor claims;
- default Go backend;
- Electron-to-Go routing;
- Reasonix public protocol;
- live provider/MCP parity or release readiness.

Rationale:

Composite replay proves that Go can represent the same TS-owned evidence across
the G5 surfaces. It does not prove Go can execute turns, tools, providers,
sessions, or MCP servers.

Validation:

Focused TS conformance and `packages/runtime-go` package tests pass. Full final
gates are recorded in release evidence.

Remaining blockers:

Live Go runtime implementation, rollback/default backend decision, packaged QA,
and live provider/MCP matrix remain open.

## D-0022 - Planner Executor And Live-Local Proof Must Remain Internal Evidence

Date: 2026-06-21
Sources: Reasonix `main-v2` `bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`,
Kun baseline `v0.2.13` -> `v0.2.14`, specs 08/09
Domain: planner/executor, durable runner, Windows shell, provider cache,
MCP/indexer, Go G5, product boundary
Status: accepted

Conflict:

Reasonix planner/executor, jobs, shell compatibility, cache, MCP/indexer, and
Go runtime patterns are valuable. Absorbing them too literally would expose
Reasonix public task/planner/plugin protocols, create a top-level workflow or
subagent product surface, or imply a default Go backend before live runtime and
release gates exist.

Decision:

Accept only analytix-owned internal evidence:

- planner/executor coordination stays an internal helper with read-only planner
  policy, executor DAG waves, failure/cancellation propagation, output offsets,
  and parent metadata;
- durable runner rehydration is a runtime reliability feature for existing
  queued/running job records, not a Reasonix job protocol;
- Windows shell compatibility is implemented as analytix runtime fixture logic
  for pwsh path/chaining and Windows PowerShell command guards;
- provider/cache evidence may use an executable local fake provider, but live
  provider superiority still requires credentials and cost reconciliation;
- MCP/indexer evidence may use an executable stdio fake server, but not the
  Reasonix plugin protocol or a top-level MCP-indexer route;
- Go G5 may consume the new planner/runner fields as shadow replay only;
- Kun stays the product-entry baseline and does not authorize Local Whisper or
  new navigation in this runtime stage.

Reject:

- Reasonix public task/planner/plugin protocol;
- Reasonix SessionAPI/frontend task cards;
- top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation;
- Kun identity or bridge aliases;
- default Go backend, Electron-to-Go routing, renderer-visible Go routes;
- Rust/Tauri rewrite;
- release readiness, live provider/cache superiority, or live MCP/indexer
  parity claims.

Rationale:

This gives analytix stronger executable proof while preserving its desktop
architecture and product surface. The TypeScript runtime remains authoritative
and Go remains conformance-only.

Validation:

Focused runtime suites pass for task-job orchestration, builtin shell tools,
provider-cache live-local proof, MCP lifecycle executable fake server, and Go
G5 conformance. Full final gates and forbidden-surface scans are recorded in
release evidence.

Remaining blockers:

Packaged desktop crash/interaction QA, Windows host validation, Local Whisper
resources and permissions, live provider credentials, live MCP/indexer
operation, live Go runtime behavior, rollback/default backend decision,
signing/notarization, and release metadata remain open.

## D-0023 - Reasonix 881 Control Semantics Must Stay Behind Analytix Runtime Contracts

Date: 2026-06-21
Sources: Reasonix `bfe398cc..881b2f2f`, current `origin/main-v2`
`9ada14176629b1d59d7ed78446951b2bb5954904`, Kun refs `v0.2.13`
`201a1469`, `v0.2.14` `06be05d`, `master` `8602476`, `develop`
`247076f`
Domain: cancel, batch results, step limits, task jobs, Go G5 shadow, product
boundary
Status: accepted

Conflict:

Reasonix 881-era commits add valuable runtime control semantics: running turns
can escape cancellation, cancelled batches preserve tool results, and step
limits become user-global. Copying the surface literally would introduce
Reasonix control/config/public protocol concepts and risk changing the desktop
contract.

Decision:

Accept only analytix-owned equivalents:

- `AgentLoop.dispatchToolCalls` must persist one `tool_result` for every model
  tool call already accepted by the turn, including synthetic
  `tool_call_cancelled` results for cancelled or unstarted calls;
- durable task jobs must link parent abort signals to child jobs and preserve
  completed/killed/skipped aggregate evidence;
- step limits live under top-level `runtime.runtimeTuning.stepLimits`, with
  runtime default/user-global/planner/headless settings plus thread/session and
  per-turn overrides;
- dynamic step budget state must not enter immutable system prefix, few-shots,
  or cache-visible prompt text;
- direct `delegate_task`, durable `task`, and `parallel_tasks` may accept
  `max_steps`, but only as tool/runtime arguments behind the existing tool
  contract;
- Go G5 may replay these controls through fixture shadow output only.

Reject:

- Reasonix TUI Esc/Ctrl+C implementation, public control/session/task/planner
  protocol, and config root names;
- Reasonix auto-plan classifier drift after `881b2f2f` until a separate
  proposal exists;
- top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation;
- Kun identity or bridge aliases;
- default Go backend, Electron-to-Go routing, renderer-visible Go routes;
- Rust/Tauri rewrite;
- live provider/cache superiority, live MCP/indexer parity, or release
  readiness claims.

Rationale:

This reaches the useful Reasonix 881 control semantics while preserving the
analytix desktop architecture: `window.analytix`, top-level `runtime` settings,
`analytix serve`, and Renderer -> preload -> main -> runtime HTTP/SSE.

Validation:

Focused loop, task-job, delegation, G5 conformance, settings, IPC, and main
config tests pass. Final full gates and forbidden-surface scans are recorded in
release evidence.

Remaining blockers:

Reasonix current `9ada1417` auto-plan drift is only recorded, live Go execution
is not implemented, and live provider/MCP/release evidence remains blocked by
external environments and packaging gates.

## D-0024 - Post-881 Auto-Plan Drift Is A Router Guard, Not A Public Config Import

Date: 2026-06-21
Sources: Reasonix `881b2f2f..9ada14176629b1d59d7ed78446951b2bb5954904`,
Kun `v0.2.14` context-window default, Go G5 control oracle
Domain: auto-plan currentness, classifier lifecycle, provider defaults, Go G5
shadow
Status: accepted

Conflict:

Reasonix post-881 changes make auto-plan user-level and rebuild the auto-plan
classifier when enabling it. Copying the surface literally would add Reasonix
config roots, CLI verbs, controller APIs, and post-881 auto-plan semantics that
analytix does not currently expose.

Decision:

Accept only analytix-owned equivalents:

- classify Reasonix user-level auto-plan as document-only/deferred unless a
  future analytix spec adds a top-level `runtime` setting;
- reject Reasonix local/project auto-plan overrides and CLI/config protocol;
- absorb the classifier-rebuild value as an auto-model-router classifier
  fingerprint included in same-turn route-cache keys;
- extend Go G5 with fixture-owned executable control cases while keeping Go
  shadow-only;
- absorb Kun 0.2.14's `128_000` unknown-model context-window default in shared
  provider settings.

Reject:

- post-881 Reasonix auto-plan parity claims;
- Reasonix public config/CLI/SessionAPI/controller protocol;
- stable-prefix mutation for auto-plan or classifier state;
- default Go backend, renderer-visible Go route, Electron-to-Go routing;
- Kun identity, deprecated bridge aliases, or top-level Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer entries.

Rationale:

This captures the useful currentness value while preserving analytix product
sovereignty and cache discipline. Classifier lifecycle state remains internal
runtime behavior, not a user-visible Reasonix protocol.

Validation:

Focused auto-router, loop, Go conformance, Go package, and shared provider
tests pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0025 - Approval Abort Cleanup Stays Behind Analytix Gates

Date: 2026-06-21
Sources: Reasonix approvalManager/control drift, analytix approval/user-input
route oracle
Domain: approval gate, user-input gate, SSE replay, turn cancellation
Status: accepted

Conflict:

Reasonix separates approval/ask bookkeeping into runtime collaborators. The
valuable part for analytix is not the public shape; it is the invariant that a
cancelled turn cannot leave actionable pending approvals or user-input waits.
Copying the Reasonix approval manager or SessionAPI would leak upstream
protocol assumptions into the desktop runtime boundary.

Decision:

Accept only analytix-owned cleanup semantics:

- `ApprovalGate.expire()` marks pending approvals as `expired`, rejects the
  waiter, removes pending state, and makes late allow/deny fail;
- `AgentLoop` records `approval_resolved: expired` when a turn aborts while
  waiting for approval;
- `request_user_input` abort records `user_input_resolved: cancelled` so SSE
  replay has a closed pair;
- renderer live mapping consumes `approval_resolved: expired` through the
  existing approval block status instead of adding a Reasonix protocol;
- late HTTP approval decisions return conflict and late user-input resolution
  returns not found after cleanup;
- cancelled tool calls continue to use the existing paired
  `tool_call_cancelled` result path.

Reject:

- Reasonix SessionAPI, public approval manager protocol, or frontend ask
  protocol;
- treating abort as user denial;
- leaving pending GUI gates actionable after a turn is interrupted;
- top-level Workflow/Subagent/AutoResearch/MCP-indexer entries;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, or
  release-readiness claims from this fixture-only batch.

Rationale:

This captures the safety value of Reasonix approval bookkeeping while keeping
analytix's contracts authoritative: HTTP/SSE events, `ApprovalGate`,
`UserInputGate`, `TurnService.interruptTurn`, and the existing renderer bridge.

Validation:

Focused gate, loop, route-oracle, and Go conformance tests pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0026 - Renderer Approval Resolution Uses Analytix Store State

Date: 2026-06-21
Sources: Reasonix approvalManager/control drift, analytix renderer store
approval sink tests
Domain: renderer approval cards, side conversations, product boundary
Status: accepted

Conflict:

Reasonix's frontend approval/session model can close live approval cards, but
copying that model would import upstream protocol and controller assumptions.
Analytix already owns `ThreadEventSink`, main chat store state, and side
conversation state. The useful invariant is that a resolved approval must
update the existing card in the active renderer state.

Decision:

Accept only analytix-owned renderer store semantics:

- main-thread `buildThreadEventSink` applies live `onApprovalStatus` to the
  existing approval block;
- side conversation SSE sinks apply approval status only to the side
  conversation block list;
- status updates do not add a duplicate approval block;
- no Reasonix SessionAPI, frontend ask protocol, controller route, or top-level
  Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Reject:

- Reasonix frontend approval/card protocol or public SessionAPI;
- treating this unit-level store proof as packaged desktop, crash/restart, or
  release readiness;
- bridge/settings/provider changes, default Go backend, renderer-visible Go
  route, Rust/Tauri rewrite, or Kun identity.

Rationale:

This closes the local renderer evidence gap left after approval abort cleanup:
the live resolved event now has tested store-level effects in both the main
thread and side surfaces while keeping analytix's public contracts unchanged.

Validation:

Focused renderer store tests pass. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0030 - Write-Inline Endpoint Formats Stay Provider-Specific

Date: 2026-06-21
Sources: Reasonix provider/cache discipline, analytix write-inline service
Domain: provider request surface, write-inline, product boundary
Status: accepted

Conflict:

Reasonix provider/cache discipline is valuable, but provider request-surface
evidence must cover every analytix desktop entry that can call a model. If
write-inline only proves DeepSeek FIM and chat/custom paths, a future provider
change could leak Anthropic headers to OpenAI Responses or send OpenAI bodies
to Anthropic Messages while runtime provider tests stay green.

Decision:

Accept fixture-backed write-inline endpoint-format regression tests:

- OpenAI Responses write-inline requests must use `/v1/responses`, `input`,
  `max_output_tokens`, and no Anthropic headers;
- Anthropic Messages write-inline requests must use `/v1/messages`, `system`,
  `messages`, `max_tokens`, Anthropic-style headers, and the Messages response
  parser;
- no production provider behavior, bridge, settings schema, runtime route, Go
  backend, or renderer navigation surface changes in this batch.

Reject:

- treating fixture-backed write-inline tests as live provider/cache
  superiority;
- Reasonix provider protocol, public SessionAPI, or controller API;
- provider default rewrites, deprecated bridge/settings fallback, default Go
  backend, Rust/Tauri rewrite, Kun identity, or top-level Workflow/Subagent/
  AutoResearch/MCP-indexer entry.

Rationale:

This turns provider/cache absorption into a wider desktop non-regression proof:
the inline-writing model path now carries the same endpoint-format discipline
as the runtime provider matrix.

Validation:

Focused write-inline tests pass. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0029 - Go G5 Replays Planner-Forbidden Task Tools Only As Shadow

Date: 2026-06-21
Sources: Reasonix planner/executor and task orchestration drift, analytix
task-job oracle, Go G5 shadow conformance
Domain: Go runtime conformance, planner gating, product boundary
Status: accepted

Conflict:

The TS task-job oracle now records `plannerForbiddenToolset`. If Go G5 ignores
that field, its shadow output can drift from the planner safety contract. If
the field is treated as live Go behavior, analytix would overclaim Go backend
readiness and risk bypassing the G5/G6 gates.

Decision:

Accept only shadow-level consumption:

- Go G5 `TaskPermissions` reads `plannerForbiddenToolset`;
- `BuildG5ShadowSlicesOutput` emits it in `jobReplay`;
- the TypeScript Go conformance test derives expected output from
  `task-job-orchestration-oracle.json`;
- no live Go planner/executor, Go Job Manager, provider/cache/session/MCP
  runtime, Electron route, renderer route, or default backend is introduced.

Reject:

- treating Go G5 shadow replay as Go runtime parity or default backend
  readiness;
- Reasonix public planner/task protocol, SessionAPI, or task cards;
- bridge/settings/provider changes, renderer-visible Go route, Rust/Tauri
  rewrite, Kun identity, or top-level Workflow/Subagent/AutoResearch/
  MCP-indexer entry.

Rationale:

The Go shadow should prove it can consume the same analytix-owned planner
safety oracle as TypeScript without becoming a second public runtime surface.

Validation:

Focused Go conformance and Go package tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0031 - Go G5 Planner Gate Execution Remains Shadow-Only

Date: 2026-06-21
Sources: Reasonix planner/executor drift, analytix task-job oracle, Go G5
control executable fixture
Domain: Go runtime conformance, planner gating, product boundary
Status: accepted

Conflict:

Go G5 already replayed `plannerForbiddenToolset`, but replay alone does not
prove the future Go runtime kernel can compute the same Plan mode gate. At the
same time, implementing a live Go planner/executor would bypass G5/G6 gates and
could imply a second public runtime surface.

Decision:

Accept only shadow-level execution:

- `controlExecutableCases.planner` records available tools, read-only tools,
  forbidden tools, `create_plan`, a forged `task`, and expected outputs;
- Go computes step 0 advertised tools as read-only + `create_plan`;
- Go computes step 1 advertised tools as `create_plan` only;
- Go computes the forged `task` call as `failed` with
  `tool_dispatch_rejected` and `executed:false`;
- no live Go planner/executor, Go Job Manager, provider/cache/session/MCP
  runtime, Electron route, renderer route, or default backend is introduced.

Reject:

- treating Go planner gate execution as Go runtime parity or default backend
  readiness;
- Reasonix public planner/task protocol, SessionAPI, or task cards;
- bridge/settings/provider changes, renderer-visible Go route, Rust/Tauri
  rewrite, Kun identity, or top-level Workflow/Subagent/AutoResearch/
  MCP-indexer entry.

Rationale:

This gives Go G5 a real deterministic control calculation for planner gating
while preserving TypeScript fixtures and analytix public contracts as the
source of truth.

Validation:

Focused Go conformance and Go package tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0032 - Structured GUI Input Validation Stays Runtime-Owned

Date: 2026-06-21
Sources: Reasonix approval/user-input/control drift, analytix runtime tool host,
G4 tools/approval/user-input/MCP oracle
Domain: user-input gates, tool contracts, Go conformance, product boundary
Status: accepted

Conflict:

Reasonix-style ask/user-input flows are useful because malformed structured
choices should not create pending GUI state. Copying the Reasonix ask/session
protocol, however, would leak upstream public contracts into analytix and could
create a second renderer/control surface.

Decision:

Accept only analytix-owned runtime tool validation:

- free-form `request_user_input` remains available;
- structured choices reject more than three questions before opening a gate;
- when options are provided, each question must have two to three options;
- option labels are trimmed and compared case-insensitively for duplicates;
- invalid structured choice requests return `invalid_user_input_request`;
- invalid structured choice requests do not call `awaitUserInput`;
- G4 fixture/schema/test and Go shadow output record invalid cases and
  `opensGateOnInvalid:false`;
- no Reasonix SessionAPI, ask protocol, controller route, renderer route,
  bridge/settings/provider change, top-level Workflow/Subagent/AutoResearch/
  MCP-indexer entry, renderer-visible Go route, default Go backend, or
  Rust/Tauri path is introduced.

Reject:

- Reasonix public ask/session/control protocol;
- treating this runtime validation as full Reasonix frontend parity, packaged
  desktop QA, Go runtime parity, or release readiness;
- Kun identity, deprecated bridge/settings fallback, or new product navigation.

Rationale:

This converts a useful upstream control-safety invariant into a smaller
analytix contract: malformed structured GUI input cannot pollute pending gate
state, and future Go work must preserve that behavior through G4 shadow
evidence.

Validation:

Focused runtime tool, G4 conformance, and Go package tests pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0033 - Custom Full Endpoint URLs Stay Explicit

Date: 2026-06-21
Sources: Reasonix provider/cache request-surface discipline, analytix
write-inline provider tests, custom endpoint contract
Domain: provider requests, write-inline, product boundary
Status: accepted

Conflict:

Provider/cache absorption needs stronger request-surface proof for custom
providers. However, treating a user-provided full endpoint URL as a base URL and
appending another `/v1/...` path would break custom gateways and violate the
explicit custom endpoint mode.

Decision:

Accept only analytix-owned request-surface proof:

- `custom_endpoint` URLs ending in `/responses` are used exactly by Write
  inline completion;
- those `/responses` requests use Responses `input` / `max_output_tokens` and
  do not send Anthropic headers;
- `custom_endpoint` URLs ending in `/messages` are used exactly by Write inline
  completion;
- those `/messages` requests use Messages `system` / `messages` / `max_tokens`,
  send Anthropic-style headers, and parse Messages responses;
- no settings schema, provider default, bridge, runtime route, Reasonix
  provider protocol, top-level navigation entry, renderer-visible Go route,
  default Go backend, or Rust/Tauri path is introduced.

Reject:

- appending `/v1/responses`, `/v1/messages`, or `/v1/chat/completions` to a
  custom full endpoint URL that already ends in a known endpoint path;
- treating this fixture proof as live provider/cache superiority, credentialed
  provider matrix completion, full Reasonix provider parity, or release
  readiness;
- Kun identity, deprecated bridge/settings fallback, or new product navigation.

Rationale:

This keeps custom provider configuration explicit while expanding the
provider/request-body non-regression matrix beyond DeepSeek, OpenAI, Anthropic,
and `/completions` custom endpoints.

Validation:

Focused write-inline provider tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0034 - Auto-Route Cache Is Turn-Scoped, Not Auto-Plan Product Surface

Date: 2026-06-21
Sources: Reasonix post-881 auto-plan classifier rebuild/currentness commits,
analytix auto-model router, AgentLoop route-cache proof
Domain: auto routing, classifier currentness, product boundary
Status: accepted

Conflict:

Reasonix's post-881 classifier rebuild work is valuable because stale routing
can make a turn use the wrong model or reasoning effort. Copying Reasonix
auto-plan settings, local/project overrides, or controller APIs would create a
new public product surface that analytix has not approved.

Decision:

Accept only analytix-owned route-cache semantics:

- auto model routing remains internal to `AgentLoop`;
- a multi-step `model:"auto"` turn may reuse one classifier result across model
  steps;
- the following turn must run `_auto_router` again instead of reusing the prior
  turn route;
- route-cache keys include the classifier contract fingerprint;
- classifier/cache dynamic state is not added to the immutable stable prefix;
- no user-level auto-plan setting, local/project `auto_plan`, Reasonix config
  root, desktop controller API, top-level navigation entry, renderer-visible Go
  route, default Go backend, or Rust/Tauri path is introduced.

Reject:

- Reasonix public auto-plan config/protocol;
- project-local auto-plan overrides;
- treating loop-level cache proof as full Reasonix auto-plan parity, packaged
  desktop QA, Go runtime parity, or release readiness;
- Kun identity, deprecated bridge/settings fallback, or new product navigation.

Rationale:

This absorbs the currentness invariant while preserving analytix's manual Plan
mode and existing runtime contract. It also keeps classifier data out of the
stable prefix so cache accounting remains explainable.

Validation:

Focused AgentLoop auto-router tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0035 - Provider Request Shape Belongs In Cache Oracle

Date: 2026-06-21
Sources: Reasonix provider/cache discipline, analytix provider-cache oracle,
CompatModelClient request-shape tests, Go G3/G5 shadow fixtures
Domain: provider requests, cache diagnostics, Go conformance, product boundary
Status: accepted

Conflict:

Cache hit/miss and stable-prefix diagnostics can look correct while a provider
request URL, header, body, or tool-schema shape silently regresses. Conversely,
turning this into a live provider matrix or Go provider client would exceed the
current evidence gate.

Decision:

Accept fixture-backed request-shape oracle coverage:

- `provider-cache-oracle.json` records request-shape cases for DeepSeek official
  chat, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom
  Responses full endpoint mode;
- `provider-cache-proof.test.ts` executes each case against `CompatModelClient`
  with mocked fetch and asserts URL, required/forbidden headers,
  required/forbidden body fields, and provider-specific tool schema shape;
- G3/G5 shadow outputs replay request-shape case ids only;
- no live provider credentials, Go provider client, provider default change,
  settings schema change, bridge, runtime route, top-level navigation entry,
  renderer-visible Go route, default Go backend, or Rust/Tauri path is
  introduced.

Reject:

- claiming live provider/cache superiority or credentialed provider matrix
  completion from fixture-only request-shape proof;
- copying Reasonix provider public protocol or config roots;
- adding DeepSeek-only `thinking` to OpenAI-compatible/custom Responses
  providers;
- appending endpoint paths to custom full endpoint URLs;
- Kun identity, deprecated bridge/settings fallback, or new product navigation.

Rationale:

This makes provider/cache proof more complete without creating a new provider
surface: stable prefix, usage telemetry, cache curve, and request-shape
invariants now live in one oracle that future TypeScript and Go work must
respect.

Validation:

Focused provider-cache, G3/G4, G5, and Go shadow tests pass. Full final gates
and forbidden-surface scans are recorded in release evidence.

## D-0028 - Internal Task Tools Stay Out Of Plan Mode

Date: 2026-06-21
Sources: Reasonix planner/executor and task orchestration drift, analytix
task-job oracle
Domain: planner gating, internal task tools, product boundary
Status: accepted

Conflict:

Reasonix task/planner orchestration is useful, but exposing `task` or
`parallel_tasks` in Plan mode would let a planning turn start child execution
instead of remaining read-only. Copying a Reasonix public planner/task protocol
would also break analytix product sovereignty and shell entry boundaries.

Decision:

Accept only analytix-owned planner gating semantics:

- `task` and `parallel_tasks` remain internal runtime tools behind existing
  task-job contracts;
- Plan mode advertises read-only tools plus `create_plan`, not internal
  child-job tools;
- forged `task` calls during Plan mode are rejected by active tool policy and
  do not execute child work;
- `task-job-orchestration-oracle.json` records `plannerForbiddenToolset` for
  future TypeScript/Go conformance;
- no Reasonix SessionAPI, public task protocol, controller route, or top-level
  Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Reject:

- Reasonix public planner/task protocol or task cards;
- treating this gating proof as full planner/executor Coordinator parity;
- bridge/settings/provider changes, default Go backend, renderer-visible Go
  route, Rust/Tauri rewrite, or Kun identity.

Rationale:

This turns Reasonix task/planner value into a stricter analytix safety property:
internal child execution cannot leak into a read-only planning turn, even when
the model forges a tool call that was not advertised.

Validation:

Focused loop, task-job oracle, and Go conformance tests pass. Full final gates
and forbidden-surface scans are recorded in release evidence.

## D-0027 - User-Input Resolution Uses Stable Analytix Card Identity

Date: 2026-06-21
Sources: Reasonix approvalManager/control drift, analytix renderer store
user-input sink tests
Domain: renderer user-input cards, side conversations, product boundary
Status: accepted

Conflict:

Reasonix's ask/session frontend can associate resolved user-input events with
pending cards, but copying that protocol would leak upstream session semantics.
Analytix already emits both runtime item ids and input/request ids through its
HTTP/SSE contract. Renderer stores must therefore match both forms without
creating a new public ask protocol.

Decision:

Accept only analytix-owned renderer store semantics:

- main-thread `buildThreadEventSink` applies live `onUserInputStatus` to an
  existing user-input block by block id or request id;
- side conversation user-input blocks use the runtime `req.itemId` rather than
  transient local ids;
- side status updates match by item id or request id and remain scoped to side
  conversation blocks;
- answers and error messages are preserved on side status updates;
- no Reasonix SessionAPI, frontend ask protocol, controller route, or top-level
  Workflow/Subagent/AutoResearch/MCP-indexer entry is introduced.

Reject:

- Reasonix frontend ask/card protocol or public SessionAPI;
- treating this store-level proof as packaged desktop, crash/restart, or
  release readiness;
- bridge/settings/provider changes, default Go backend, renderer-visible Go
  route, Rust/Tauri rewrite, or Kun identity.

Rationale:

This closes the remaining GUI-gate live-card gap after approval store proof:
`user_input_resolved` now has tested store-level effects in both main and side
surfaces while analytix's public contracts remain authoritative.

Validation:

Focused renderer store tests pass. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0036 - Top-Level Route Surface Is A Product Contract

Date: 2026-06-21
Sources: Kun 0.2.13/0.2.14 product baseline, Reasonix orchestration
capability absorption, analytix route-surface tests
Domain: renderer routes, product navigation, upstream absorption boundary
Status: accepted

Conflict:

Reasonix and later Kun-current work provide useful orchestration concepts, but
promoting Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer into top-level
analytix navigation would exceed the Kun 0.2.13 -> 0.2.14 target baseline and
weaken analytix product sovereignty.

Decision:

Accept the capabilities only behind existing analytix-owned contracts, and make
the top-level route surface executable evidence:

- `AppRoute` remains `chat | write | settings | plugins | claw | schedule`;
- app actions do not expose `openWorkflow`, `openCreateLoop`,
  `openSubagent`, `openAutoResearch`, or MCP-indexer openers;
- Workbench stage rendering, shell navigation, and sidebar active views do not
  contain top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
  route tokens or entrypoint symbols;
- internal Create Loop/task/research/tool code may remain only when it is not a
  renderer top-level route and remains behind analytix contracts.

Reject:

- using Reasonix public task/planner/session protocol as a renderer route;
- using Kun current or Reasonix concepts to claim Kun 0.2.14 top-level parity;
- adding deprecated bridge/settings fallback, Kun identity, default Go backend,
  Rust/Tauri rewrite, or new public product navigation.

Rationale:

This turns the entry boundary from a static scan into a maintained unit-test
contract. Future absorption can still reuse code-level capability, but a route
promotion must create a new spec/conflict decision instead of slipping through
as a regression.

Validation:

Focused `Workbench.route-surface.test.ts` and `chat-store-app-actions.test.ts`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0037 - Control Composition Must Preserve Cache Boundaries

Date: 2026-06-21
Sources: Reasonix post-881 auto-plan currentness, user-global step limits,
running-turn cancel, cancelled batch-result preservation, analytix G5 shadow
Domain: auto-router cache, step limits, cancellation, Go shadow, stable prefix
Status: accepted

Conflict:

Reasonix improved classifier currentness, step limits, and cancellation
semantics in nearby batches. Absorbing them as separate point fixes is useful,
but the dangerous failure mode is composition: step-limit/control state could
leak into cache-visible prefix text, auto-route cache could hide currentness,
or cancellation could drop already accepted tool-call results.

Decision:

Accept only analytix-owned composed control semantics:

- auto-route cache is same-turn only and may be reused across model steps until
  the resolved step limit fires;
- the next turn must reroute rather than reusing the previous classifier
  result;
- classifier state and step-limit state must not enter the stable system
  prefix or context instructions;
- cancellation preserves completed tool results and emits `tool_call_cancelled`
  for running/unstarted calls by call id;
- Go may compute this combined case only as G5 shadow evidence, not as a live
  backend or renderer-visible route.

Reject:

- Reasonix public control/session/task/planner protocol;
- Reasonix auto-plan settings or local/project config roots;
- stable-prefix mutation for dynamic route/step/cancel state;
- default Go backend, Electron-to-Go routing, Rust/Tauri rewrite, Kun identity,
  or new top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation.

Rationale:

This turns several Reasonix post-881 deltas into a single regression contract:
the runtime can be more capable without sacrificing cache determinism,
turn-control auditability, or analytix product ownership.

Validation:

Focused `loop.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0038 - Public Renderer Bridge Is Analytix Only

Date: 2026-06-21
Sources: Kun 0.2.13/0.2.14 product baseline, Reasonix frontend/session protocol
review, analytix preload bridge contract
Domain: preload bridge, renderer global type, shared public API names
Status: accepted

Conflict:

Reasonix has useful frontend/session protocol ideas, and Kun is analytix's
lineage, but exposing any Kun, Reasonix, DeepSeek, or deprecated GUI facade
name at the renderer boundary would break analytix product sovereignty and
make future absorption depend on non-analytix public contracts.

Decision:

Accept only the analytix-owned bridge/API contract:

- preload exposes exactly one renderer bridge name: `analytix`;
- `Window` declares exactly one app bridge property: `analytix`;
- public shared bridge API names stay under `AnalytixApi` /
  `AnalytixFlatApi` / analytix domain facade types;
- Kun legacy import IPC may remain internal compatibility plumbing, but it may
  not become a renderer bridge alias or public facade;
- Reasonix SessionAPI/frontend protocol names are rejected at the renderer
  public boundary.

Reject:

- `window.kun`, `window.kunGui`, `window.reasonix`, `window.deepseek`,
  `window.analytixGui`, or equivalent bridge aliases;
- exported public facade names such as `KunApi`, `KunGuiApi`,
  `ReasonixApi`, `ReasonixSessionAPI`, `DeepSeekApi`, or `AnalytixGuiApi`;
- deprecated settings fallback, Reasonix public protocol, Kun identity,
  default Go backend, Rust/Tauri rewrite, or new top-level Workflow/Subagent/
  AutoResearch/MCP-indexer navigation.

Rationale:

This converts the bridge/API sovereignty rule from a hand-run scan into a
maintained test contract. Upstream code can still be absorbed behind
analytix-owned domains, but public renderer access must stay anchored on
`window.analytix`.

Validation:

Focused `preload-sandbox.test.ts` passes. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0039 - Renderer Thread Lifecycle Uses Analytix HTTP

Date: 2026-06-21
Sources: Kun thread/workflow baseline, Reasonix session/control protocol
review, analytix renderer runtime adapter
Domain: renderer runtime adapter, thread list/search/archive/fork/resume/SSE
Status: accepted

Conflict:

Thread lifecycle is a tempting place to copy upstream public protocols because
Kun and Reasonix both have session/thread control surfaces. Analytix must keep
the renderer backend-neutral and product-owned: renderer code should call the
analytix bridge and HTTP/SSE routes, not upstream SessionAPI, legacy bridge
aliases, or product-specific protocols.

Decision:

Accept only analytix-owned renderer lifecycle semantics:

- thread list/search/archive filters use `/v1/threads` query parameters;
- archive, restore, rename, workspace update, and delete use
  `/v1/threads/:id` with analytix JSON bodies;
- fork remains `/v1/threads/:id/fork`;
- session resume remains `/v1/sessions/:id/resume-thread`;
- live replay remains the runtime SSE bridge through `window.analytix`;
- renderer lifecycle tests must fail if these operations drift to upstream
  public protocol names or deprecated bridge aliases.

Reject:

- Reasonix SessionAPI or public thread/session control protocol in the
  renderer;
- Kun public protocol or deprecated bridge/settings fallback;
- treating mocked renderer-adapter proof as packaged desktop QA or live Go
  route readiness;
- default Go backend, Electron-to-Go routing, Rust/Tauri rewrite, Kun
  identity, or new top-level Workflow/Subagent/AutoResearch/MCP-indexer
  navigation.

Rationale:

This closes a visible renderer gap next to the preload bridge oracle: the
bridge name is now pinned, and thread lifecycle actions are pinned to the
analytix HTTP/SSE contract that bridge exposes.

Validation:

Focused `analytix-runtime.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0040 - Endpoint Format Persists In Analytix Settings

Date: 2026-06-21
Sources: Reasonix provider/config drift, Kun provider baseline, analytix
top-level runtime settings contract
Domain: settings persistence, provider profiles, endpoint format
Status: accepted

Conflict:

Provider endpoint families affect request URL, headers, body shape, streaming,
usage parsing, and cache accounting. Copying upstream settings roots or legacy
agent envelopes would make those choices ambiguous and could reintroduce
Reasonix/Kun public settings identity.

Decision:

Accept only analytix-owned settings persistence:

- runtime-selected endpoint format persists under top-level `runtime`;
- provider-profile endpoint format persists under `provider.providers`;
- saves must not write top-level `agentProvider` or `agents` envelopes;
- custom full endpoint mode remains explicit and is not normalized into an
  appended-path base URL;
- request-shape behavior remains proven by provider/cache oracles, while this
  decision pins the settings storage side.

Reject:

- Reasonix config roots, CLI config protocol, or provider settings names;
- Kun public settings identity or deprecated agent settings fallback;
- provider default changes hidden inside migration;
- default Go backend, Go provider client, Rust/Tauri rewrite, Kun identity, or
  new top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation.

Rationale:

This connects provider request-shape oracles to durable desktop settings:
endpoint-format choices cannot survive only in memory or leak into old
settings envelopes.

Validation:

Focused `settings-store.test.ts` passes. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0041 - Thread Provider Selection Stays On Analytix Model Client Contracts

Date: 2026-06-21
Sources: Reasonix provider routing/cache review, analytix runtime
`MultiProviderModelClient`
Domain: provider selection, request shape, diagnostics
Status: accepted

Conflict:

Reasonix provider routing can be useful as behavior, but its public provider
protocol and config surface cannot become analytix's public contract. Thread
provider selection must drive URL/header/body shape through analytix-owned
runtime clients, and missing provider records must degrade to the configured
default provider rather than leaking upstream identities.

Decision:

Accept only analytix-owned runtime dispatch semantics:

- a thread-selected custom provider may use `endpointFormat: "custom_endpoint"`
  and an exact full endpoint URL;
- `/messages` custom endpoints resolve to the Messages request body/header
  shape through `CompatModelClient`;
- missing/removed thread providers fall back to the default OpenAI-compatible
  provider path;
- diagnostics may report analytix runtime fields such as provider id, base URL,
  endpoint format, and configured model;
- no Reasonix public provider protocol, settings root, renderer route, or bridge
  alias is introduced.

Reject:

- Reasonix public provider protocol, config root, or session/control API;
- Kun public provider identity, deprecated bridge/settings fallback, or old
  agent settings envelope;
- changing provider defaults as a side effect of request-shape proof;
- treating fake-fetch runtime evidence as live provider/cache superiority;
- default Go backend, Go provider client, Rust/Tauri rewrite, Kun identity, or
  new top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation.

Rationale:

This links the settings persistence oracle to the runtime request path: endpoint
format and provider id are not only durable, they also select the expected model
client shape at execution time.

Validation:

Focused `multi-provider-model-client.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0042 - Scheduled Detector Custom Endpoints Infer Request Family Locally

Date: 2026-06-21
Sources: Reasonix provider request-shape review, Kun schedule baseline,
analytix scheduled reminder detector
Domain: scheduled task detection, provider endpoint format, custom full
endpoint mode
Status: accepted

Conflict:

Scheduled reminder detection is another model consumer. If it appended paths to
custom full endpoints or ignored endpoint-family inference, provider settings
could pass chat but fail in Schedule. Absorbing upstream behavior must remain
inside analytix's main-process detector and settings contract.

Decision:

Accept only analytix-owned scheduled detector semantics:

- `endpointFormat: "custom_endpoint"` means the configured full endpoint URL is
  used exactly;
- full `/messages` endpoints infer Messages body/header/parser behavior;
- full `/chat/completions` endpoints infer chat body/header/parser behavior;
- scheduled detection continues to read analytix top-level runtime/provider
  settings through existing resolver helpers;
- no Reasonix public provider protocol, settings root, renderer route, or bridge
  alias is introduced.

Reject:

- appending another provider path to custom full endpoints;
- Reasonix public provider protocol, config root, or SessionAPI;
- Kun public provider identity, deprecated bridge/settings fallback, or old
  agent settings envelope;
- treating fake-fetch scheduled detector evidence as live provider/cache
  superiority;
- default Go backend, Go scheduled detector, Rust/Tauri rewrite, Kun identity,
  or new top-level Workflow/Subagent/AutoResearch/MCP-indexer navigation.

Rationale:

This extends provider request-shape proof beyond chat and write-inline: Schedule
now has explicit fake-fetch evidence for custom full endpoint inference.

Validation:

Focused `claw-scheduled-task-detector.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0043 - Forbidden Upstream Public Routes Stay Absent

Date: 2026-06-21
Sources: Reasonix route/protocol review, Kun entry baseline, Go G5/G6 gate
Domain: runtime HTTP routing, product sovereignty, Go backend gating
Status: accepted

Conflict:

Analytix can absorb upstream behavior behind its own runtime HTTP/SSE contract,
but must not expose Reasonix public protocols, Kun-forbidden top-level
capabilities, or renderer-visible Go backend routes before G5/G6 gates pass.
Static scans are useful, but the runtime HTTP router also needs executable
negative evidence.

Decision:

Accept only analytix-owned route surfaces:

- Reasonix public route prefixes such as `/v1/reasonix/*` stay absent;
- Go runtime public routes such as `/v1/runtime/go` stay absent while Go remains
  shadow-only;
- Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer HTTP routes stay absent
  as top-level public surfaces;
- unknown forbidden routes return the same structured 404 as other unknown
  routes;
- any future intentional route addition must update this decision, specs,
  ledger, and release evidence.

Reject:

- Reasonix SessionAPI, public thread/session/provider protocol, or route names;
- Kun public protocol or forbidden product-entry routes;
- renderer-visible Go routes or default Go backend before G5/G6;
- Rust/Tauri rewrite or native backend replacement;
- treating hidden internal task/job/MCP/research code as public route
  entitlement.

Rationale:

The router test turns policy into an executable guard: if an upstream public
surface sneaks in, the HTTP suite fails before the renderer can depend on it.

Validation:

Focused `http-server.test.ts` passes. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0044 - Rehydrated Task Jobs Stay On Authenticated Internal Routes

Date: 2026-06-21
Sources: Reasonix sub-agent/job orchestration review, Go G5 durable runner
fixture, analytix `DurableTaskJobManager`
Domain: durable task jobs, restart continuity, internal runtime routes
Status: accepted

Conflict:

Reasonix-style sub-agent/job orchestration is useful, but exposing a public
Subagent/Workflow/job protocol would break analytix product sovereignty. The
restart-continuity behavior must remain behind authenticated analytix runtime
routes that already exist for internal task jobs.

Decision:

Accept only analytix-owned internal route semantics:

- rehydrated task jobs remain accessible through
  `/v1/runtime/task-jobs/output`;
- completed or still-running rehydrated jobs can be observed through
  `/v1/runtime/task-jobs/wait`;
- rehydrated queued jobs can be cancelled through `/v1/runtime/task-jobs/kill`;
- these routes remain authenticated internal runtime routes, not top-level
  product navigation or Reasonix public protocol.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- unauthenticated task-job control;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback;
- treating HTTP harness evidence as packaged desktop restart QA.

Rationale:

This links durable restart proof to the actual GUI-facing runtime HTTP boundary:
after rehydrate, the same internal routes still work without exposing an
upstream public surface.

Validation:

Focused `task-job-orchestration-oracle.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0046 - Go G4 Manager Replay Remains Conformance-Only

Date: 2026-06-23
Sources: Reasonix approval/user-input/MCP manager patterns, analytix
approval-user-input route oracle, analytix MCP lifecycle oracle, Go runtime
conformance plan
Domain: Go runtime, approvals, user input, MCP, product boundary
Status: accepted

Conflict:

G4 approval/user-input/MCP manager behavior is valuable for future Go runtime
parity, but adding live-local routes for it could be mistaken for a real Go
approval manager, user-input backend, MCP client, or renderer-visible Go
runtime surface.

Decision:

Accept only fixture-backed conformance replay:

- G4 live-local routes may exist only under `/v1/conformance/g4/*`;
- route status/body/event evidence must come from TS-owned fixtures;
- denied approval and MCP approval-annotation cases must never execute tools;
- user-input resolved-event evidence must omit submitted answers even when the
  HTTP response echoes answers;
- MCP lifecycle/search/reconnect/diagnostics replay must not connect to real
  MCP servers or read credentials;
- `LiveLocalSidecarSnapshot` must keep approval execution, tool execution, MCP
  connection, credential read, file mutation, `events.jsonl` write, and real
  workspace write attempts at `0`;
- Electron main, preload, renderer, `analytix serve`, and settings surfaces
  remain unchanged.

Reject:

- promoting G4 conformance routes into the active TypeScript runtime route
  table;
- exposing `/v1/runtime/go`, `/v1/reasonix`, `/session-api`, Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer routes, or a public Reasonix control
  plane;
- claiming live Go MCP parity, approval execution parity, default backend, G6
  readiness, packaged desktop QA, or release readiness from D-0235 evidence.

Rationale:

This lets Go absorb Reasonix gate/manager discipline while preserving
analytix-owned contracts and preventing a test harness from becoming a product
surface.

Validation:

`packages/runtime-go go test ./...` exercises the G4 conformance handler
through `httptest.Server` and asserts the zero-attempt counters. Full final
gates and product-sovereignty scans are recorded in release evidence.

## D-0047 - Go Durable Event Store Is Temp-Only Until G6

Date: 2026-06-23
Sources: Reasonix durable session/event recovery patterns, analytix
RuntimeEventRecorder/FileSessionStore/SSE route contracts, D-0236 Go temp
durable sidecar proof
Domain: Go runtime, event persistence, SSE replay, product boundary
Status: accepted

Conflict:

Reasonix-style durable session recovery is required for future Go runtime
parity, but introducing a Go `events.jsonl` writer could be mistaken for a
production runtime replacement, a real workspace store, a renderer-visible Go
route, or permission to bypass the TypeScript runtime contract.

Decision:

Accept only explicit temp-dir conformance durability:

- Go durable writes may exist only under `/v1/conformance/durable/*`;
- the store must be disabled by default and enabled only by a caller-provided
  temp dir;
- durable roots outside `os.TempDir()` are rejected;
- `events.jsonl` appends must be newline-terminated and per-thread `seq` must
  be stable, non-decreasing, and unique under concurrent writes;
- `highestSeq()` must return the persisted max, and `loadEventsSince()` must
  filter, sort, skip malformed lines, and return diagnostics;
- SSE replay must prove caught-up behavior and `Last-Event-ID` / `since_seq`
  equivalence;
- recovered state may include usage/cache, approval/user-input, and MCP catalog
  evidence, but must not execute tools, connect MCP, read credentials, call
  providers, or persist submitted user-input answers;
- Electron main, preload, renderer, `analytix serve`, settings, and active TS
  runtime stores remain unchanged.

Reject:

- writing real workspace runtime state from the Go sidecar;
- promoting durable conformance routes into the active TypeScript runtime route
  table;
- exposing `/v1/runtime/go`, `/v1/reasonix`, `/session-api`, Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer routes, or Reasonix public protocol;
- claiming default backend, G6 readiness, production crash recovery, packaged
  desktop QA, live provider/MCP/gate parity, or release readiness from D-0236.

Rationale:

This lets Go absorb durable event/session recovery mechanics while retaining
analytix ownership of the runtime HTTP/SSE contract and avoiding accidental
product-surface expansion.

Validation:

`packages/runtime-go go test -count=1 ./...` and
`packages/runtime test -- tests/go-durable-sidecar-conformance.test.ts` start
the temp sidecar, write only under temp dirs, and compare replay/recovery
behavior to TS-owned fixtures.

## D-0192 - Renderer Endpoint Builder Replay Does Not Expose A Frontend Protocol

Date: 2026-06-23
Sources: D-0193 renderer runtime endpoint builder proof, D-0213 desktop
sovereignty G5 shadow, Reasonix post-881 route/session boundary review
Domain: renderer provider, runtime bridge, endpoint construction, Go G5
Status: accepted

Conflict:

Reasonix route/session work is useful for endpoint construction discipline, but
promoting renderer provider endpoint builders into Go G5 replay could be
mistaken for permission to expose a Reasonix-style frontend/session protocol or
a renderer-visible Go route.

Decision:

Accept only analytix-owned renderer endpoint replay:

- `desktopSovereignty.rendererProviderEndpointMatrix` records `/health`,
  `/v1/threads`, encoded thread/turn/approval/user-input/session ids, and the
  renderer runtime-client facade;
- Go G5 shadow computes shared-root, encoded dynamic route-id,
  analytix-owned runtime path, facade, and unit-proof booleans;
- renderer calls remain behind `rendererRuntimeClient.runtimeRequest` and
  `window.analytix`, with no deprecated bridge alias;
- TypeScript runtime HTTP/SSE contracts remain authoritative for live behavior.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route names, or config
  roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go HTTP
  server, or Electron Go bridge from this evidence;
- claiming packaged route/SSE walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Endpoint-builder replay raises the proof quality for renderer path safety while
keeping the public product contract as `Renderer -> window.analytix -> preload
-> main -> analytix runtime HTTP/SSE`.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0201 - Renderer Provider Alias Guard Replay Rejects Legacy Bridges

Date: 2026-06-23
Sources: D-0202 renderer runtime provider alias guard, D-0222 desktop
sovereignty G5 shadow, Reasonix post-881 frontend/session boundary review
Domain: renderer provider bridge, route sovereignty, Go G5
Status: accepted

Conflict:

Promoting provider alias guard evidence into Go G5 replay improves desktop
bridge proof, but it must not make Reasonix SessionAPI, Kun bridge aliases,
hidden route families, or a Go desktop backend visible to the renderer.

Decision:

Accept only analytix-owned provider replay:

- `desktopSovereignty.rendererProviderAliasGuardMatrix` records the
  `installDsGui` helper, forbidden `kun` / `reasonix` aliases, guarded test
  names, route/lifecycle/approval-user-input/fork-resume/dynamic-encoding
  coverage, source-only analytix proof, and forbidden-route guard proof;
- Go G5 shadow computes throwing alias installation, route coverage,
  lifecycle/gate coverage, fork/resume/dynamic encoding coverage,
  forbidden-route rejection, and unit-proof presence;
- live behavior remains the TypeScript `AnalytixRuntimeProvider` calling the
  analytix runtime client facade and analytix HTTP/SSE routes;
- no upstream bridge/protocol, Go backend selector, or top-level hidden
  feature route can be inferred from this evidence.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route/session names,
  or config roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged provider walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts`, `analytix-runtime.test.ts`, and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0202 - Renderer Provider Facade Seal Replay Rejects Direct Bridge Bypass

Date: 2026-06-23
Sources: D-0203 renderer provider runtime client facade seal, D-0223 desktop
sovereignty G5 shadow, Reasonix post-881 frontend/session boundary review
Domain: renderer provider bridge, runtime client facade, Go G5
Status: accepted

Conflict:

Promoting provider facade seal evidence into Go G5 replay improves bridge
safety proof, but it must not normalize direct renderer calls to
`window.analytix.runtime.runtimeRequest`, expose upstream bridge protocol, or
suggest a live Go desktop backend.

Decision:

Accept only analytix-owned provider facade replay:

- `desktopSovereignty.rendererProviderFacadeSealMatrix` records
  `rendererRuntimeClient.runtimeRequest`, the forbidden direct bridge token,
  sealed methods, source-only runtime client use, archive/restore proof,
  relation PATCH proof, lifecycle unit proof, scan guard, and provider token
  scan proof;
- Go G5 shadow computes runtime client use, direct bridge rejection,
  archive/restore coverage, relation PATCH coverage, scan guard presence, and
  unit-proof presence;
- live behavior remains the TypeScript `AnalytixRuntimeProvider` calling
  `rendererRuntimeClient.runtimeRequest`;
- the renderer-wide direct bridge bypass scan remains the active guard against
  `window.analytix.runtime.runtimeRequest` in production renderer source.

Reject:

- treating direct `window.analytix.runtime.runtimeRequest` calls in renderer
  production code as acceptable;
- exposing Reasonix SessionAPI, controller APIs, public route/session names,
  or config roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged provider walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts`, `analytix-runtime.test.ts`, and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0203 - Side Conversation Relation Replay Stays Provider-Owned

Date: 2026-06-23
Sources: D-0204 side conversation relation provider contract, D-0224 desktop
sovereignty G5 shadow, Reasonix post-881 frontend/session boundary review
Domain: renderer side store, provider contract, Go G5
Status: accepted

Conflict:

Promoting side conversation relation evidence into Go G5 replay improves side
store proof, but it must not expose Reasonix side/session public protocol,
direct renderer runtime bridge calls, or a hidden top-level side/subagent
entry.

Decision:

Accept only analytix-owned side relation replay:

- `desktopSovereignty.sideConversationRelationContractMatrix` records
  `updateThreadRelation`, `promoteSideConversation`, relation `primary`,
  optional provider contract proof, provider implementation proof, store
  provider-call proof, refresh/close proof, direct bridge rejection, unit
  proof, and scan guard proof;
- Go G5 shadow computes optional provider contract proof, provider promotion,
  refresh/close behavior, direct bridge rejection, scan guard presence, and
  unit-proof presence;
- live behavior remains the TypeScript side store calling
  `provider.updateThreadRelation(sideId, 'primary')`;
- no upstream side/session protocol, Go backend selector, or top-level hidden
  feature route can be inferred from this evidence.

Reject:

- exposing Reasonix side/session public protocol, SessionAPI, controller APIs,
  public route/session names, or config roots;
- treating direct `window.analytix.runtime.runtimeRequest` calls in side-store
  production code as acceptable;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged side walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts`, `analytix-runtime.test.ts`,
`chat-store-side-actions.test.ts`, and `packages/runtime-go go test -count=1
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0204 - Renderer Usage Facade Replay Stays Shadow-Only

Date: 2026-06-23
Sources: D-0205 renderer usage runtime client facade seal, D-0225 desktop
sovereignty G5 shadow, Reasonix post-881 usage/debug observability review
Domain: renderer usage/debug diagnostics, runtime client facade, Go G5
Status: accepted

Conflict:

Promoting renderer usage/debug facade evidence into Go G5 replay improves
bridge safety proof, but it must not normalize direct
`window.analytix.runtime.runtimeRequest` calls, expose Reasonix usage/debug
protocol, or imply a live Go desktop backend.

Decision:

Accept only analytix-owned usage facade replay:

- `desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` records
  `rendererRuntimeClient.runtimeRequest`, the forbidden direct bridge token,
  thread/day/model usage loaders, token economy and LLM debug diagnostics
  loaders, source-only runtime client use, direct bridge rejection, usage unit
  proof, scan guard, and usage facade token scan proof;
- Go G5 shadow computes thread/day/model usage coverage, settings diagnostics
  coverage, direct bridge rejection, scan guard presence, and unit-proof
  presence;
- live behavior remains the TypeScript usage hooks/settings diagnostics calling
  `rendererRuntimeClient.runtimeRequest`;
- the renderer-wide direct bridge bypass scan remains the active guard against
  `window.analytix.runtime.runtimeRequest` in production renderer source.

Reject:

- treating direct `window.analytix.runtime.runtimeRequest` calls in renderer
  production code as acceptable;
- exposing Reasonix SessionAPI, usage/debug public protocol, controller APIs,
  public route/session names, or config roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged usage/dashboard walkthrough, G6 readiness, release
  readiness, or Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0234 - Go G3 Provider/Cache Streaming Stays Fixture-Backed

Date: 2026-06-23
Sources: D-0234 Go G3 provider/cache streaming live-local sidecar, analytix G3
provider/cache oracle, Reasonix cache/provider diagnostics lessons
Domain: Go runtime, provider request shape, streaming, cache telemetry, product
boundary
Status: accepted

Conflict:

A local Go provider/cache harness is useful G3 evidence, but it can be mistaken
for a live external provider client, credentialed provider matrix, default Go
backend, Electron integration, or live provider/cache superiority.

Decision:

Accept provider/cache streaming replay only inside a fixture-backed
test/conformance sidecar harness:

- `NewLiveLocalSidecarHarness` may receive a `ProviderOracle` and serve only
  local `/v1/conformance/g3/provider/*` routes from TS-owned fixtures;
- usage replay must parse the 5 provider payloads for unsupported
  OpenAI-compatible, DeepSeek native hit/miss, DeepSeek native precedence,
  OpenAI Responses cached tokens, and Anthropic cache read/create fields;
- request-shape replay must recompute all 7 URL/header/body/tool-shape cases,
  including custom full endpoints without appending paths;
- streaming replay must preserve `item_delta -> usage -> turn_completed`
  order and compare the usage event to `deepseek-prompt-cache`;
- cache diagnostics may expose bounded hashes and provider/model/endpoint
  attribution only, while keeping prompt/tool/API-key/Authorization substrings
  out of diagnostics;
- `ProviderCallAttempts` must remain `0`, and external network, API-key read,
  and provider credential flags must remain false.

Reject:

- wiring the G3 sidecar to Electron main, preload, renderer, settings, or
  `analytix serve`;
- exposing `/v1/runtime/go`, `/v1/reasonix`, SessionAPI, Reasonix provider
  protocol, config roots, or route names;
- adding top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
  product routes;
- reading real API keys, using real provider credentials, or calling external
  DeepSeek/OpenAI/Anthropic/model services;
- claiming default Go backend, G6 readiness, release readiness, live
  provider/cache superiority, credentialed provider matrix completion,
  packaged provider QA, durable Go event/session storage, live approval/user
  input, MCP credential, or file mutation parity from this batch;
- introducing a Rust/Tauri path, Kun identity, or deprecated bridge/settings
  fallback.

Rationale:

D-0234 closes the G3 provider/cache streaming prototype gap while preserving
TypeScript provider/cache oracle authority. It absorbs Reasonix cache/provider
diagnostic value through analytix-owned request-shape, usage/cache, stable
prefix, and drift contracts rather than through upstream public protocols.

Validation:

`TestLiveLocalSidecarG3ProviderUsageAndCacheReplayMatchesTypeScriptOracle`,
`TestLiveLocalSidecarG3ProviderRequestShapeReplayMatchesTypeScriptOracle`, and
`TestLiveLocalSidecarG3ProviderStreamingReplayMatchesTypeScriptOracle` start a
real local `httptest.Server` and replay provider usage/cache, request shape,
and SSE evidence from TS-owned fixtures. Focused TS conformance source guards
require `live_local_provider.go`, no environment/API-key access, and the
fixture-only local route tokens.

Supersedes:

None. This extends D-0233 without changing its historical G2 lifecycle boundary
claim for that batch.

Superseded by:

None.

## D-0233 - Go Mutating G2 Lifecycle Stays Isolated

Date: 2026-06-23
Sources: D-0233 Go isolated mutating G2 lifecycle sidecar, analytix G2 route
replay oracle, D-0232 sidecar boundary
Domain: Go runtime, session lifecycle mutation, backend selection, product
boundary
Status: accepted

Conflict:

Admitting `PATCH` and `POST` route fixtures into the Go live-local sidecar is
useful for lifecycle proof, but it can be mistaken for production Go routing,
durable Go storage, Electron integration, or renderer-visible backend
selection.

Decision:

Accept mutating G2 lifecycle replay only inside an isolated test/conformance
sidecar harness:

- `NewLiveLocalSidecarHarness` may execute the four TS-owned G2 mutating routes
  against an in-memory fixture store;
- the four admitted routes are `PATCH /v1/threads/thr_g2_beta`,
  `PATCH /v1/threads/thr_g2_read`, `POST /v1/threads/thr_g2_parent/fork`, and
  `POST /v1/sessions/thr_g2_source/resume-thread`;
- route status/body/error shape must stay aligned to
  `go-g2-route-replay-oracle.json`;
- mutation evidence must be observable only through the Go harness snapshot and
  must not touch real workspace files, real `events.jsonl`, provider clients,
  approval execution, MCP credentials, or file mutation;
- G5 product-boundary guards must keep `electronMainConnected:false`,
  `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`.

Reject:

- wiring the sidecar to Electron main, preload, renderer, settings, or
  `analytix serve`;
- exposing `/v1/runtime/go` or Reasonix SessionAPI/public protocol;
- adding top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
  product routes;
- claiming default Go backend, G6 readiness, release readiness, live
  provider/cache superiority, credentialed MCP parity, live approval/user-input
  gate parity, durable Go event/session storage, or file mutation parity from
  this batch;
- introducing a Rust/Tauri path, Kun identity, or deprecated bridge/settings
  fallback.

Rationale:

The D-0233 prototype closes the next G2 lifecycle evidence gap while preserving
TypeScript runtime authority. It proves the Go sidecar can apply fixture-owned
session lifecycle mutations locally without crossing production runtime or
desktop product boundaries.

Validation:

`TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptOracle` starts
a real local `httptest.Server`, compares all G2 status/body/SSE frames to
TS-owned fixtures, records archive/update/fork/resume state in
`LiveLocalSidecarSnapshot`, and asserts forbidden routes stay unexposed.
`TestLiveLocalSidecarMutationsDoNotTouchFilesystem` keeps a temporary
`events.jsonl` sentinel unchanged. The product-sovereignty scan requires the
D-0233 tokens and false guards.

Supersedes:

None. This extends D-0232 without changing its historical read-only boundary
claim for that batch.

Superseded by:

None.

## D-0232 - Go Live-Local Sidecar Prototype Stays Test-Only

Date: 2026-06-23
Sources: D-0232 Go live-local sidecar prototype, analytix G1/G2/G5 oracle
fixtures, Reasonix serve/sidecar runtime ideas
Domain: Go runtime, backend selection, route/SSE replay, product boundary
Status: accepted

Conflict:

A Go sidecar that starts a real local HTTP handler is useful proof, but it can
be mistaken for a Go backend switch, Electron integration, or renderer-visible
runtime route.

Decision:

Accept a narrow test/conformance-only live-local sidecar:

- `NewLiveLocalSidecarHandler` may combine G1 health/info/tools responses with
  G2 route replay only when started by Go tests;
- G2 replay is filtered to `GET` routes, including exact fixture SSE frames and
  the unauthorized replay contract;
- mutating route fixtures such as `PATCH /v1/threads/:id`, fork/resume `POST`,
  approval execution, provider calls, MCP credentials, and file mutation stay
  outside the sidecar;
- G5 product-boundary guards must keep `electronMainConnected:false`,
  `defaultGoBackendEnabled:false`, and `rendererVisibleGoRoutesAllowed:false`.

Reject:

- wiring the sidecar to Electron main, preload, renderer, settings, or
  `analytix serve`;
- exposing `/v1/runtime/go` or Reasonix SessionAPI/public protocol;
- claiming default Go backend, G6 readiness, release readiness, live
  provider/cache superiority, credentialed MCP parity, live approval/user-input
  gate parity, or file mutation parity from this batch;
- introducing a Rust/Tauri path, Kun identity, deprecated bridge/settings
  fallback, or hidden top-level Workflow/Create Loop/Subagent/AutoResearch/
  MCP-indexer entry.

Rationale:

The prototype makes the Go work materially more executable without crossing
the product boundary. It proves local HTTP/SSE replay can match analytix-owned
fixtures while preserving TypeScript runtime authority and rollback.

Validation:

`TestLiveLocalSidecarPrototypeMatchesTypeScriptOracle` starts a real local
`httptest.Server`, compares G1/G2 status/body/SSE frames to TS-owned fixtures,
and asserts mutating routes plus `/v1/runtime/go` remain unexposed. The TS
conformance test and product-sovereignty scan also require the sidecar tokens
and false guards.

Supersedes:

None.

Superseded by:

None.

## D-0231 - MCP Search Refresh Drift Stays Internal

Date: 2026-06-23
Sources: D-0231 MCP search refresh drift evidence closure, existing
`mcp-tool-lifecycle-oracle`, Reasonix post-881 MCP/search currentness review
Domain: MCP search, catalog refresh, Go G5
Status: accepted

Conflict:

MCP catalog refresh improves tool currentness, but exposing it as a public
Reasonix MCP/indexer protocol or top-level product entry would break analytix
product sovereignty and the Kun-compatible navigation boundary.

Decision:

Accept only internal MCP refresh drift evidence:

- `mcp_refresh_catalog` remains an internal MCP meta-tool;
- refresh drift records initial `search_issues` and expanded `search_issues`
  plus `create_issue`;
- runtime tests verify `totalIndexed: 2` and `catalogDrift: true`;
- `controlExecutableCases.mcpSearchRefreshDrift` and Go G5 shadow replay the
  same fields;
- `scan:product-sovereignty` now fails if the refresh drift proof disappears.

Reject:

- exposing Reasonix MCP public protocol, SessionAPI, public indexer routes, or
  upstream MCP control planes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  settings fallbacks;
- enabling a default Go backend, renderer-visible Go route, live Go MCP
  client, Rust/Tauri rewrite, credentialed MCP matrix, packaged MCP QA, G6
  readiness, or release readiness from this evidence.

Validation:

Focused MCP oracle/conformance tests and `packages/runtime-go go test -count=1
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0230 - Plan/Auto-Route Reset Stays Turn-Scoped

Date: 2026-06-23
Sources: D-0230 Plan/auto-route state reset G5 shadow, Reasonix post-881
classifier/planner currentness review
Domain: Planner gating, auto-router currentness, Go G5
Status: accepted

Conflict:

Reasonix-style classifier rebuild and planner enable/disable currentness is
valuable, but analytix must not expose a public Reasonix controller/config
protocol, project auto-plan override, or top-level auto-plan product entry.

Decision:

Accept only analytix-owned turn-scoped reset evidence:

- a cancelled Plan turn does not leak `modeInstruction` into later normal or
  auto-routed turns;
- later normal turns do not inherit `requiredToolName: create_plan`;
- later normal and auto turns do not advertise `create_plan`;
- the next `model: "auto"` turn reruns `_auto_router` once with no tools,
  no prefix, and no Plan mode instruction;
- the auto recommendation is current (`deepseek-v4-pro`, `max`);
- `controlExecutableCases.planCancelStateReset` and Go G5 shadow compute the
  same outputs from TS-owned fixtures.

Reject:

- exposing Reasonix SessionAPI, controller protocol, public auto-plan setting,
  project/local auto-plan override, or config root;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- putting dynamic classifier/planner/cancel state in the stable prefix or tool
  schema;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  settings fallbacks;
- enabling a default Go backend, renderer-visible Go route, live Go loop,
  Rust/Tauri rewrite, packaged Plan/auto-route QA, G6 readiness, or release
  readiness from this evidence.

Validation:

Focused loop/conformance tests and `packages/runtime-go go test -count=1 ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0107/D-0229 - Plan Step/Cancel/Cache Stays Internal And Cache-Stable

Date: 2026-06-23
Sources: D-0107 Plan step/cancel/cache boundary guard, D-0229 Go G5 Plan
step/cancel/cache shadow, Reasonix post-881 planner/cancel/cache review
Domain: Planner gating, cancellation, provider cache, Go G5
Status: accepted

Conflict:

Reasonix-style planner orchestration is useful when step tools, cancellation,
retry, and cache diagnostics remain stable together. That value must not become
a public Reasonix controller/config protocol, top-level auto-plan product
entry, or default Go backend in analytix.

Decision:

Accept only analytix-owned internal planner/cache evidence:

- explicit Plan mode step 0 advertises `create_plan` and read-only `ls` while
  excluding `bash`;
- the plan follow-up is forced to `create_plan` only;
- an aborted follow-up does not advance the cache-prefix baseline;
- the next plan retry reuses the original cache baseline;
- DeepSeek `chat_completions` cache telemetry remains preserved as two usage
  events with `80` hit tokens and `20` miss tokens;
- `controlExecutableCases.planStepCancelCache` and Go G5 shadow compute the
  same outputs from TS-owned fixtures.

Reject:

- exposing Reasonix SessionAPI, controller protocol, public auto-plan setting,
  or config root;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- putting dynamic planner/cache state in the stable prefix or tool schema;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  settings fallbacks;
- enabling a default Go backend, renderer-visible Go route, live Go loop,
  Rust/Tauri rewrite, packaged Plan QA, G6 readiness, or release readiness from
  this evidence.

Validation:

Focused Plan/conformance tests and `packages/runtime-go go test -count=1
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0228 - MCP Call-Time Reconnect Stays Internal And Classified

Date: 2026-06-23
Sources: D-0228 MCP call-time reconnect G5 shadow, MCP tool provider
implementation, Reasonix post-881 MCP/tool lifecycle review
Domain: MCP tool calls, lifecycle, Go G5
Status: accepted

Conflict:

Recovering stale MCP sessions improves tool reliability, but retrying every
MCP error would tear down healthy sessions for deterministic validation or
protocol failures. The useful Reasonix lifecycle value must remain an internal
analytix runtime behavior, not a public MCP/indexer protocol.

Decision:

Accept only classified internal reconnect behavior:

- transport-looking tool-call failures such as stale connections get one
  reconnect + retry;
- deterministic MCP protocol/validation errors return tool-result failures
  without reconnecting;
- `mcp-tool-lifecycle-oracle` records both branches;
- `controlExecutableCases.mcpCallReconnect` and Go G5 shadow compute retry,
  no-retry, attempt counts, close counts, success, and error-code outputs.

Reject:

- retrying deterministic protocol/validation errors as if they were stale
  transport failures;
- exposing Reasonix MCP public protocol, SessionAPI, public indexer routes, or
  upstream MCP control planes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  settings fallbacks;
- enabling a default Go backend, renderer-visible Go route, live Go MCP
  client, Rust/Tauri rewrite, credentialed MCP matrix, packaged MCP QA, G6
  readiness, or release readiness from this evidence.

Validation:

Focused MCP oracle/provider/conformance tests and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0014/D-0227 - AutoResearch Direction Tracking Stays Internal

Date: 2026-06-23
Sources: D-0014 AutoResearch project-local state, D-0227 Go G5 AutoResearch
direction tracking shadow, Reasonix post-881 long-task review
Domain: AutoResearch, goal runtime, Go G5
Status: accepted

Conflict:

Reasonix-style long-task research benefits from durable direction tracking, but
that value must not become a public Reasonix project protocol or a new
top-level AutoResearch product entry in analytix.

Decision:

Accept only analytix-owned internal direction tracking:

- `record_research_direction` remains a local runtime tool for active
  research goals;
- `ThreadService.recordResearchDirection` rejects threads without an active
  research goal;
- AutoResearch state remains project-local under
  `.analytix/autoresearch/<threadId>/`;
- `directions_tried.json` and `iteration_log.jsonl` are the durable audit
  files;
- `controlExecutableCases.autoResearch` and Go G5 shadow now compute direction
  file, iteration-log, tool-name, and active-goal guard outputs.

Reject:

- exposing Reasonix SessionAPI, project protocol, controller APIs, or public
  route/session names;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- writing `REASONIX.md` or `AGENTS.md` for research state;
- putting research state into the stable prefix or dynamic tool schema;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  settings fallbacks;
- enabling a default Go backend, renderer-visible Go route, live Go research
  bridge, Rust/Tauri rewrite, packaged long-task QA, G6 readiness, or release
  readiness from this evidence.

Validation:

Focused `go-runtime-conformance` and `packages/runtime-go go test -count=1
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0205 - Renderer Settings-Read Replay Stays Shadow-Only

Date: 2026-06-23
Sources: D-0206 renderer settings read facade seal, D-0226 desktop sovereignty
G5 shadow, Reasonix post-881 settings/currentness boundary review
Domain: renderer settings reads, runtime client facade, Go G5
Status: accepted

Conflict:

Promoting renderer settings-read evidence into Go G5 replay improves settings
sovereignty proof, but it must not normalize direct
`window.analytix.settings.getSettings` calls, expose Reasonix config/settings
protocol, or imply a live Go settings backend.

Decision:

Accept only analytix-owned settings-read replay:

- `desktopSovereignty.rendererSettingsReadFacadeMatrix` records
  `rendererRuntimeClient.getSettings`, the forbidden direct bridge token,
  keyboard shortcut, speech-to-text, and initial usage model-label readers,
  source-only settings client proof, settings-changed event preservation,
  direct bridge rejection, scan guard, and settings-read token scan proof;
- Go G5 shadow computes settings reader coverage, event-sync preservation,
  direct bridge rejection, and scan guard presence;
- live behavior remains the TypeScript renderer sources calling
  `rendererRuntimeClient.getSettings`;
- the renderer-wide direct settings bypass scan remains the active guard
  against `window.analytix.settings.getSettings` in production renderer source.

Reject:

- treating direct `window.analytix.settings.getSettings` calls in renderer
  production code as acceptable;
- exposing Reasonix SessionAPI, config roots, settings public protocol,
  controller APIs, or public route/session names;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go settings
  bridge, or Electron Go bridge from this evidence;
- claiming packaged settings walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0200 - Renderer Settings Bridge Replay Keeps Top-Level Runtime Contract

Date: 2026-06-23
Sources: D-0201 renderer settings bridge proof, D-0221 desktop sovereignty G5
shadow, Reasonix post-881 settings/currentness boundary review
Domain: renderer settings client, top-level runtime settings, Go G5
Status: accepted

Conflict:

Promoting renderer settings evidence into Go G5 replay improves settings
contract proof, but it must not reintroduce Reasonix config roots, legacy agent
settings envelopes, deprecated Kun bridge aliases, or a live Go settings
backend claim.

Decision:

Accept only analytix-owned settings bridge replay:

- `desktopSovereignty.rendererSettingsBridgeMatrix` records top-level
  `runtime` patch values, cache expectations, analytix settings API source
  proof, cache/write refresh proof, unit proof, and legacy alias unread proof;
- Go G5 shadow computes settings API use, settings read cache, write refresh,
  top-level runtime patch preservation, legacy alias unread status, and
  unit-proof presence;
- live behavior remains the TypeScript renderer client calling
  `window.analytix.settings` with top-level `runtime` patches;
- no upstream settings/config protocol or Go backend selector can be inferred
  from this evidence.

Reject:

- exposing Reasonix settings/config roots, controller APIs, or legacy agent
  settings envelopes;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged settings walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Validation:

Focused `go-runtime-conformance.test.ts`, `runtime-client.test.ts`, and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0199 - Renderer Runtime Client Bridge Replay Stays Behind Analytix Facade

Date: 2026-06-23
Sources: D-0200 renderer runtime client bridge proof, D-0220 desktop
sovereignty G5 shadow, Reasonix post-881 frontend/session boundary review
Domain: renderer runtime client, bridge sovereignty, Go G5
Status: accepted

Conflict:

Promoting renderer runtime client evidence into Go G5 replay improves frontend
bridge proof, but it must not expose a Reasonix public frontend protocol,
deprecated Kun bridge alias, renderer-visible Go route, or default backend
claim.

Decision:

Accept only analytix-owned renderer client replay:

- `desktopSovereignty.rendererRuntimeClientBridgeMatrix` records encoded
  runtime request path variants, argument counts, restart passthrough, SSE
  start/stop calls, listener APIs, source passthrough proof, unit proof, and
  legacy alias unread proof;
- Go G5 shadow computes request argument preservation, restart passthrough,
  SSE control/listener preservation, legacy alias unread status, and unit-proof
  presence;
- live behavior remains the TypeScript renderer client calling
  `window.analytix.runtime` and `window.analytix.settings` only through
  analytix-owned contracts;
- no upstream bridge/session protocol or Go backend selector can be inferred
  from this evidence.

Reject:

- exposing Reasonix SessionAPI, frontend bridge aliases, controller APIs, or
  public route names;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go desktop
  bridge, or Electron Go bridge from this evidence;
- claiming packaged renderer walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

The renderer client facade is the point where Reasonix frontend/runtime value
can be absorbed safely: it strengthens analytix runtime/SSE handoff evidence
while preserving the `window.analytix` product contract and the TypeScript
runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0198 - Main SSE Host Encoding Replay Stays On Analytix Events Route

Date: 2026-06-23
Sources: D-0199 main SSE host URL encoding proof, D-0219 desktop main IPC G5
shadow, Reasonix post-881 route/session boundary review
Domain: main IPC, SSE host URL, cursor headers, Go G5
Status: accepted

Conflict:

Promoting main SSE host URL evidence into Go G5 replay improves stream-route
proof, but it must not expose a Reasonix public SSE protocol, a renderer-visible
Go route, or a live Go SSE server claim.

Decision:

Accept only analytix-owned main SSE host replay:

- `desktopMainIpcBoundary.mainSseHostEncodingMatrix` records dangerous source
  thread id, encoded thread id, `/v1/threads/{id}/events`, `since_seq`,
  `Last-Event-ID`, `Accept: text/event-stream`, bearer auth, stream id, error
  payload, source proof, unit proof, and forbidden-route guard;
- Go G5 shadow computes thread/cursor encoding, header/stream-id preservation,
  forbidden-route rejection, and unit-proof presence;
- live behavior remains TypeScript `registerRuntimeSseIpc` fetching managed
  `analytix serve` thread events with `analytixThreadEventsPath`;
- no upstream SSE/session protocol or Go backend selector can be inferred from
  this evidence.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public SSE route names, or
  upstream stream protocol;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go SSE
  server, Go main bridge, or Electron Go bridge from this evidence;
- claiming packaged SSE walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Main SSE host replay proves the managed runtime event URL stays on the
analytix-owned `/v1/threads/{id}/events` contract while preserving cursor and
auth headers.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0197 - Preload SSE Replay Stays Payload-Only And Analytix-Owned

Date: 2026-06-23
Sources: D-0198 preload SSE bridge proof, D-0218 desktop sovereignty G5 shadow,
Reasonix post-881 route/session boundary review
Domain: preload bridge, SSE IPC, endpoint streaming, Go G5
Status: accepted

Conflict:

Promoting preload SSE bridge evidence into Go G5 replay improves streaming
bridge proof, but it must not expose Electron events, Reasonix public SSE
protocol, deprecated bridge aliases, or a live Go SSE route.

Decision:

Accept only analytix-owned preload SSE replay:

- `desktopSovereignty.preloadSseBridgeMatrix` records start/stop/event/end/error
  channels, thread id, cursor, stream ids, event/end/error payloads,
  payload-only wrapper proof, cleanup proof, and `analytix` bridge exposure
  proof;
- Go G5 shadow computes start/stop argument preservation, payload-only listener
  delivery, cleanup symmetry, and unit-proof presence;
- live behavior remains `window.analytix.runtime.startSse/stopSse` invoking
  main IPC `runtime:sse:start/stop`, with listeners receiving payloads only;
- no upstream SSE/session protocol or Go backend selector can be inferred from
  this evidence.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public SSE route names, or
  upstream stream protocol;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go SSE
  server, Go preload bridge, or Electron Go bridge from this evidence;
- claiming packaged SSE walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Preload SSE replay proves the renderer streaming bridge remains
`window.analytix` and payload-only while preserving the existing
`runtime:sse:*` IPC contract.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0196 - Runtime Host Handoff Replay Stays Behind Analytix Serve

Date: 2026-06-23
Sources: D-0197 runtime host URL handoff proof, D-0217 desktop main IPC G5
shadow, Reasonix post-881 route/session boundary review
Domain: main runtime adapter, managed runtime host, endpoint construction, Go G5
Status: accepted

Conflict:

Promoting runtime host URL handoff evidence into Go G5 replay improves the last
TypeScript hop in `preload -> main -> analytix runtime HTTP`. That replay must
not turn into a Reasonix public host protocol, a renderer-visible Go route, or
a live Go HTTP server claim.

Decision:

Accept only analytix-owned host handoff replay:

- `desktopMainIpcBoundary.runtimeHostHandoffMatrix` records
  `runtimeRequestViaHost`, `getRuntimeBaseUrlForSettings`, encoded thread/turn
  ids, encoded query preservation, POST body, bearer auth, custom header, JSON
  content type, source proof, and ensureRuntime port-switch proof;
- Go G5 shadow computes encoded path/query preservation, method/header/body
  preservation, ensured settings use, and unit-proof presence;
- live behavior remains the TypeScript `runtimeRequestViaHost` fetch against
  the managed `analytix serve` base URL derived from top-level `runtime`
  settings;
- no upstream route protocol or Go backend selector can be inferred from this
  evidence.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route names, host URL
  templates, or config roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go HTTP
  server, Go main/preload bridge, or Electron Go bridge from this evidence;
- claiming packaged route walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Host handoff replay proves the managed runtime HTTP request preserves the same
encoded endpoint contract already established by shared builders, renderer,
preload, main schema, and main handler handoff.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0195 - Preload Runtime Request Replay Stays Behind Window Analytix

Date: 2026-06-23
Sources: D-0196 preload runtime request bridge proof, D-0216 desktop
sovereignty G5 shadow, Reasonix post-881 route/session boundary review
Domain: preload bridge, runtime request IPC, endpoint construction, Go G5
Status: accepted

Conflict:

Promoting preload runtime request bridge evidence into Go G5 replay improves
endpoint-chain proof, but it must not create a second public bridge protocol or
authorize Reasonix/Kun aliases.

Decision:

Accept only analytix-owned preload bridge replay:

- `desktopSovereignty.preloadRuntimeRequestBridgeMatrix` records runtime and
  diagnostics facade calls, encoded ids, path/method/body preservation,
  same-channel diagnostics behavior, and `analytix` bridge exposure proof;
- Go G5 shadow computes runtime facade preservation, diagnostics same-channel
  behavior, analytix-only exposure, and unit-proof presence;
- live behavior remains `window.analytix.runtime.runtimeRequest` and
  `window.analytix.diagnostics.runtimeRequest` invoking main IPC
  `runtime:request`;
- no deprecated bridge alias or upstream SessionAPI can be exposed.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route names, or config
  roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go HTTP
  server, Go preload bridge, or Electron Go bridge from this evidence;
- claiming packaged preload/route walkthrough, G6 readiness, release readiness,
  or Rust/Tauri migration.

Rationale:

Preload bridge replay proves the public renderer bridge remains
`window.analytix` while preserving encoded endpoint arguments on the existing
main IPC channel.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0194 - Main IPC Handoff Replay Does Not Create A Runtime Control Plane

Date: 2026-06-23
Sources: D-0195 main IPC runtime adapter handoff proof, D-0215 desktop main IPC
G5 shadow, Reasonix post-881 route/session boundary review
Domain: main IPC, runtime adapter handoff, endpoint construction, Go G5
Status: accepted

Conflict:

It is valuable to prove the registered main IPC handler passes encoded
endpoint-builder paths to the runtime adapter unchanged. That replay must not
become a new Go/Reasonix runtime control plane or loosen the TypeScript schema
gate before adapter invocation.

Decision:

Accept only analytix-owned handoff replay:

- `desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` records encoded adapter
  calls, raw/singular rejected-before-adapter calls, parse-before-adapter
  ordering, encoded-path proof, method/body proof, and reject-before-call proof;
- Go G5 shadow computes encoded-path preservation, method/body preservation,
  raw dynamic rejection, reject-before-call, and unit-proof presence;
- live behavior remains the TypeScript `runtimeRequestPayloadSchema` and
  `runtime:request` handler calling `runtimeRequest(request.path,
  request.method, request.body)`;
- invalid raw dynamic and singular compatibility paths must fail before the
  runtime adapter can be called.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route names, or config
  roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go HTTP
  server, or Electron Go bridge from this evidence;
- claiming packaged IPC/route walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Adapter handoff replay proves the end of the renderer/preload/main endpoint
chain without changing the authoritative TypeScript runtime adapter boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0193 - Main IPC Builder Replay Remains A Schema Gate

Date: 2026-06-23
Sources: D-0194 main IPC endpoint builder allow-list proof, D-0214 desktop main
IPC G5 shadow, Reasonix post-881 route/session boundary review
Domain: main IPC, runtime request schema, endpoint construction, Go G5
Status: accepted

Conflict:

Promoting main IPC endpoint-builder evidence into Go G5 replay is useful, but
it could be misread as permission to route renderer traffic through a new Go or
Reasonix control plane instead of the existing analytix IPC/runtime schema.

Decision:

Accept only analytix-owned main IPC schema replay:

- `desktopMainIpcBoundary.endpointBuilderAllowListMatrix` records encoded
  shared-builder requests, raw dynamic route rejections, singular user-input
  rejection, shared template compilation, and unit-test proof;
- Go G5 shadow computes accepted shared paths, raw dynamic rejection,
  shared-template use, unit-proof presence, and singular user-input rejection;
- live behavior remains the TypeScript `runtimeRequestPayloadSchema` and
  `runtime:request` handler before the runtime adapter call;
- singular `/v1/user-input/:id` remains runtime compatibility only, not a
  main IPC accepted endpoint-builder path.

Reject:

- exposing Reasonix SessionAPI, controller APIs, public route names, or config
  roots;
- adding `window.kun`, `window.reasonix`, deprecated bridge aliases, or legacy
  runtime settings writes;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling a default Go backend, renderer-visible Go routes, live Go HTTP
  server, or Electron Go bridge from this evidence;
- claiming packaged IPC/route walkthrough, G6 readiness, release readiness, or
  Rust/Tauri migration.

Rationale:

Main IPC schema replay strengthens the proof that renderer-supplied paths must
already be analytix-owned and encoded before the runtime adapter sees them,
without creating a new public protocol.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0048 - Runtime HTTP Route Shadow Is Not Reasonix Route Protocol

Date: 2026-06-22
Sources: Reasonix runtime route review, analytix `analytix serve` route table,
Go G5 executable control shadow
Domain: runtime HTTP/SSE, product sovereignty, Go G5 shadow
Status: accepted

Conflict:

Reasonix runtime route and SessionAPI ideas are useful comparison pressure, but
the analytix desktop contract is already Renderer -> preload -> main ->
`analytix serve` HTTP/SSE. Proving route inventory cannot become permission to
publish Reasonix route names, SessionAPI payloads, or hidden capability
navigation.

Decision:

Accept only analytix-owned route sovereignty proof:

- `controlExecutableCases.runtimeHttpRouteSovereignty` is source-derived from
  the TypeScript route table, router, HTTP server, endpoint templates, and
  HTTP server tests;
- `/health` is the only unauthenticated public route and the rest of the 44
  routes stay authenticated `/v1/*` analytix routes;
- SSE remains `/v1/threads/:id/events`, with thread lifecycle,
  approval/user-input, and resume-thread routes present;
- task-job wait/output/kill routes remain authenticated internal runtime
  routes;
- singular `/v1/user-input/:id` is recorded only as compatibility, while the
  shared contract remains plural `/v1/user-inputs/{id}`;
- Go shadow may compute the same booleans, but TypeScript runtime routes remain
  authoritative.

Reject:

- Reasonix SessionAPI, route names, or public protocol exposure;
- Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public or top-level
  routes;
- unauthenticated task-job or approval/user-input control;
- renderer-visible Go route, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

The proof raises confidence that analytix has absorbed route-surface discipline
while preserving product sovereignty and keeping Go at the G5 shadow gate.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0049 - HTTP Auth Matrix Is Not a Public Control Plane

Date: 2026-06-22
Sources: D-0189 runtime route sovereignty oracle, analytix TypeScript HTTP
router, Reasonix route/control review
Domain: runtime HTTP auth, product sovereignty
Status: accepted

Conflict:

Proving every active route rejects unauthenticated traffic is valuable, but it
must not become a new public control plane, a Reasonix route protocol, or a Go
backend activation signal.

Decision:

Accept only actual TypeScript HTTP dispatch evidence:

- the test reads the D-0189 `runtimeHttpRouteSovereignty.routes` oracle;
- it dispatches every registered route without auth through the real TypeScript
  router;
- `/health` must return 200;
- every other route must return structured 401 with
  `{ code: "unauthorized", message: "unauthorized" }`;
- SSE, task-job, approval, user-input, and resume-thread route keys must be in
  the unauthorized matrix.

Reject:

- Reasonix SessionAPI or public route protocol;
- unauthenticated runtime, task-job, approval, user-input, or SSE controls;
- top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This makes the D-0189 route inventory executable at the real TypeScript HTTP
boundary while preserving analytix product ownership and Go shadow-only status.

Validation:

Focused `go-runtime-conformance.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0051 - Shared Endpoint Builders Are Not a Reasonix Protocol Shim

Date: 2026-06-22
Sources: analytix shared endpoint contract, Reasonix route/control review
Domain: shared runtime endpoints, route-id encoding, product sovereignty
Status: accepted

Conflict:

Centralized shared endpoint builders are necessary for renderer/main/runtime
contract consistency, but they must not expose Reasonix public route names,
hidden capability routes, or a singular user-input compatibility path as the
canonical product contract.

Decision:

Accept only analytix-owned shared endpoint proof:

- path builders URL-encode thread, turn, checkpoint, approval, user-input,
  session, attachment, and memory ids;
- exported endpoint strings stay `/health` or `/v1/*`;
- canonical shared user-input template is plural `/v1/user-inputs/{id}`;
- singular `/v1/user-input/:id` remains server compatibility only and is not
  exported as a shared template;
- exported endpoint strings contain no Reasonix/Kun/DeepSeek/Go/Workflow/
  Create Loop/Subagent/AutoResearch/MCP-indexer/session-api tokens.

Reject:

- Reasonix SessionAPI or public route protocol;
- hidden top-level route or shared endpoint templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

The shared endpoint layer is the narrowest place to prove renderer/main/runtime
path construction stays analytix-owned and cannot be drifted into upstream
route identity by raw ids or compatibility routes.

Validation:

Focused `src/shared/analytix-endpoints.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0052 - Renderer Runtime Provider Must Use Analytix Endpoint Builders

Date: 2026-06-22
Sources: analytix shared endpoint contract, renderer `AnalytixRuntimeProvider`,
Reasonix route/control review
Domain: renderer runtime paths, bridge contract, product sovereignty
Status: accepted

Conflict:

The renderer provider is the first GUI caller of runtime HTTP paths. If it
constructs raw paths outside shared endpoint builders, it can drift from the
main IPC allow-list and runtime route table or accidentally encode upstream
route identity.

Decision:

Accept only analytix-owned renderer path construction:

- health and thread root paths use shared endpoint constants;
- dynamic thread, turn, approval, user-input, and session ids are encoded
  before `runtimeRequest`;
- renderer runtime requests remain `/health` or `/v1/*`;
- forbidden Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/
  MCP-indexer route tokens remain absent from renderer provider requests.

Reject:

- Reasonix SessionAPI or public route protocol;
- hidden top-level route or renderer request paths for Workflow/Create Loop/
  Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This extends D-0192 from shared builders into the actual GUI runtime provider,
which is the product boundary most likely to regress during upstream
absorption.

Validation:

Focused `src/renderer/src/agent/analytix-runtime.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0053 - Main IPC Allow-list Is Not a Compatibility Route Expansion

Date: 2026-06-22
Sources: analytix main IPC runtime request schema, shared endpoint builders,
renderer runtime provider, Reasonix route/control review
Domain: main IPC allow-list, route-id encoding, bridge contract
Status: accepted

Conflict:

The main process must accept renderer runtime requests produced by the shared
endpoint builders, including ids containing slash/query/fragment text after
encoding. That proof must not widen the bridge into a compatibility route shim
or expose singular/server-only routes as public desktop contract.

Decision:

Accept only analytix-owned main IPC allow-list proof:

- `runtimeRequestPayloadSchema` accepts encoded shared builder output for
  thread, turn, checkpoint, approval, user-input, session, attachment, and
  memory routes;
- unencoded ids that create extra route segments are rejected before the
  runtime adapter call;
- singular `/v1/user-input/:id` remains server compatibility only and is
  rejected by the main IPC shared allow-list;
- renderer bridge requests remain `window.analytix.runtime.runtimeRequest`
  calls to modeled `/health` or `/v1/*` analytix routes.

Reject:

- Reasonix SessionAPI or public route protocol;
- hidden top-level route or main IPC request templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- singular user-input compatibility path as shared or bridge-visible contract;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This closes the renderer -> main leg of the D-0192/D-0193 endpoint proof:
builders can encode dangerous ids, the renderer can send them, and the main
process allow-list accepts only the modeled encoded shape.

Validation:

Focused `src/main/ipc/app-ipc-schemas.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0054 - IPC Handler Handoff Must Preserve Analytix Paths

Date: 2026-06-22
Sources: `registerAppIpcHandlers`, analytix main IPC runtime request schema,
shared endpoint builders, Reasonix route/control review
Domain: main IPC handler, runtime adapter handoff, bridge contract
Status: accepted

Conflict:

Schema-level allow-list proof is not enough if the registered handler mutates
paths before calling the runtime adapter or if invalid routes can still reach
the adapter. The handoff must preserve encoded analytix paths while rejecting
compatibility drift before side effects.

Decision:

Accept only analytix-owned handler handoff proof:

- encoded shared endpoint builder paths are passed to `runtimeRequest`
  unchanged;
- method and body values are preserved with the validated path;
- raw extra-segment dynamic ids reject before `runtimeRequest` is called;
- singular `/v1/user-input/:id` remains server compatibility only and is not
  forwarded from the desktop IPC bridge.

Reject:

- Reasonix SessionAPI or public route protocol;
- hidden top-level route or handler request templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- path decoding/rewriting between validation and runtime adapter handoff;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This proves the final main-process hop in the D-0192 through D-0194 path chain:
shared builders encode, renderer sends, main schema validates, and the handler
forwards only the validated analytix path.

Validation:

Focused `src/main/ipc/register-app-ipc-handlers.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0055 - Preload Runtime Requests Stay on window.analytix

Date: 2026-06-22
Sources: preload `window.analytix` facade, shared endpoint builders,
Reasonix route/control review
Domain: preload bridge, runtime request IPC, product sovereignty
Status: accepted

Conflict:

The renderer-to-main proof chain needs an executable preload check, not only
source regexes. That check must preserve the existing analytix bridge and must
not revive deprecated aliases or expose Reasonix route/session protocol.

Decision:

Accept only analytix-owned preload bridge proof:

- `api.runtime.runtimeRequest` invokes `runtime:request` with the encoded path,
  method, and body unchanged;
- diagnostics runtime request compatibility uses the same analytix IPC channel
  and payload shape;
- the exposed bridge remains `window.analytix`;
- no Reasonix/Kun/Go/workflow request channel is added.

Reject:

- Reasonix SessionAPI or public route protocol;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or preload request templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, or packaged
  route/release readiness claims.

Rationale:

This turns the preload leg of the runtime request chain into an executable
contract: shared builders produce encoded paths, renderer calls the facade,
preload invokes `runtime:request`, main validates and forwards the same path.

Validation:

Focused `src/preload/preload-runtime-request.test.ts` passes. Full final gates
and forbidden-surface scans are recorded in release evidence.

## D-0056 - Runtime Host URL Handoff Must Preserve Encoded Analytix Paths

Date: 2026-06-22
Sources: `runtimeRequestViaHost`, shared endpoint builders, runtime bridge
proof chain, Reasonix route/control review
Domain: runtime adapter URL construction, HTTP request shape, product boundary
Status: accepted

Conflict:

The renderer/preload/main proofs are incomplete if the runtime adapter decodes
or rewrites encoded route ids while building the `analytix serve` HTTP URL. The
final desktop-to-runtime hop must preserve the analytix path contract while
still attaching auth and request metadata.

Decision:

Accept only analytix-owned runtime host URL proof:

- encoded shared endpoint paths remain encoded when sent to the runtime host;
- query strings remain intact, including encoded slash values;
- method, bearer auth, custom headers, content type, and body are preserved;
- the target runtime remains the managed analytix base URL from top-level
  `runtime` settings.

Reject:

- Reasonix SessionAPI or public route protocol;
- hidden top-level route or runtime-host request templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- path decoding/rewriting between main IPC handler and runtime host fetch;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This completes the unit-level chain from shared endpoint builders through
renderer, preload, main IPC schema, handler handoff, and runtime host HTTP
request construction.

Validation:

Focused `src/main/runtime/analytix-adapter.test.ts` passes. Full final gates
and forbidden-surface scans are recorded in release evidence.

## D-0057 - Preload SSE Bridge Stays Analytix-Owned

Date: 2026-06-22
Sources: preload `window.analytix` SSE facade, runtime SSE IPC bridge,
Reasonix route/control review
Domain: preload bridge, SSE IPC, product sovereignty
Status: accepted

Conflict:

SSE streaming is a sensitive long-lived runtime channel. The preload facade
must preserve `startSse` / `stopSse` arguments and payload-only listener
delivery without exposing Electron events, deprecated bridge aliases, or
Reasonix SessionAPI/SSE protocol.

Decision:

Accept only analytix-owned preload SSE bridge proof:

- `api.runtime.startSse` invokes `runtime:sse:start` with thread id, cursor,
  and stream id unchanged;
- `api.runtime.stopSse` invokes `runtime:sse:stop` with the stream id
  unchanged;
- `onSseEvent`, `onSseEnd`, and `onSseError` deliver payloads only and remove
  their listeners on unsubscribe;
- the exposed bridge remains `window.analytix`.

Reject:

- Reasonix SessionAPI or public SSE protocol;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or preload SSE templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go SSE server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, or packaged
  SSE/release readiness claims.

Rationale:

This gives the SSE bridge the same executable preload coverage as runtime
requests: renderer-visible code sees only `window.analytix`, and main IPC gets
the analytix-owned SSE channel payloads.

Validation:

Focused `src/preload/preload-sse-bridge.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0058 - Main SSE Host URL Uses Analytix Event Routes Only

Date: 2026-06-22
Sources: `registerRuntimeSseIpc`, shared endpoint builders, Reasonix
route/control review
Domain: main SSE IPC, runtime host URL construction, cursor safety
Status: accepted

Conflict:

Main SSE IPC receives raw thread ids from the renderer-facing bridge. It must
encode those ids before constructing the runtime host event URL and must keep
cursor headers under the analytix route contract rather than accepting
upstream public SSE protocol names.

Decision:

Accept only analytix-owned main SSE URL proof:

- dangerous thread ids containing slash/query/fragment text are encoded before
  fetching `/v1/threads/{id}/events`;
- `since_seq`, `Last-Event-ID`, `Accept: text/event-stream`, and bearer auth
  are preserved;
- fatal route errors return through `runtime:sse-error` with the same stream id;
- forbidden public SSE route families remain absent.

Reject:

- Reasonix SessionAPI or public SSE protocol;
- hidden top-level route or main SSE templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- path decoding/rewriting between main SSE IPC and runtime host fetch;
- renderer-visible Go routes, default Go backend, live Go SSE server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged SSE/release readiness claims.

Rationale:

This pairs D-0198 preload SSE bridge proof with the main-process SSE host URL
construction proof, closing the desktop SSE path at unit level.

Validation:

Focused `src/main/runtime-sse-ipc.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0059 - Renderer Runtime Client Uses Only window.analytix

Date: 2026-06-22
Sources: renderer `runtime-client`, preload bridge proof chain, Reasonix
route/control review
Domain: renderer bridge client, runtime request, SSE facade
Status: accepted

Conflict:

Even if preload/main/runtime boundaries are analytix-owned, the renderer
runtime client must not grow a fallback to deprecated bridge aliases or
Reasonix SessionAPI. It must preserve runtime request and SSE arguments while
reading only `window.analytix`.

Decision:

Accept only analytix-owned renderer client bridge proof:

- `runtimeRequest` forwards path, method, and body unchanged through
  `window.analytix.runtime`;
- `startSse`, `stopSse`, and SSE listener registration forward arguments and
  handlers unchanged;
- tests install throwing `window.kun` and `window.reasonix` getters to prove
  legacy aliases are not read.

Reject:

- Reasonix SessionAPI or public bridge protocol;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer client templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, or packaged
  route/release readiness claims.

Rationale:

This closes the renderer client facade before the preload/main/runtime
handoffs already proved in D-0196 through D-0199.

Validation:

Focused `src/renderer/src/agent/runtime-client.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0051 - Renderer Settings Bridge Uses Top-Level Runtime Settings Only

Date: 2026-06-22
Sources: D-0201 renderer settings bridge proof, active renderer
`runtime-client`, Reasonix settings/config review
Domain: desktop settings bridge, product sovereignty
Status: accepted

Conflict:

Reasonix settings/config improvements are useful only if renderer settings
handoff remains analytix-owned. Importing upstream config roots, bridge aliases,
or legacy runtime-shaped settings would weaken the product boundary.

Decision:

Accept only the analytix-owned renderer settings path:

- `rendererRuntimeClient.setSettings` calls `window.analytix.settings`;
- settings patches preserve top-level `runtime` fields such as `model` and
  `approvalPolicy`;
- the returned settings update the renderer cache;
- tests install throwing `window.kun` and `window.reasonix` getters to prove
  legacy aliases are not read.

Reject:

- Reasonix config roots, SessionAPI, or public settings protocol;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- old runtime-shaped settings fallback or `agents.kun` settings writes;
- hidden top-level route or settings templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  settings walkthrough, or release readiness claims.

Rationale:

This completes the renderer client bridge proof by covering settings handoff
alongside the runtime request and SSE facades already proved in D-0200.

Validation:

Focused `src/renderer/src/agent/runtime-client.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0052 - Renderer Runtime Provider Keeps Legacy Aliases Unread

Date: 2026-06-22
Sources: D-0202 renderer runtime provider alias guard, D-0193 endpoint builder
proof, Reasonix frontend/session boundary review
Domain: renderer provider bridge, product sovereignty
Status: accepted

Conflict:

Renderer provider code touches many sensitive surfaces: thread lifecycle,
approval/user-input, fork/resume, tools, memory, attachments, and SSE. Those
paths may absorb upstream runtime quality only if they remain on
analytix-owned routes and do not consult legacy or upstream bridge aliases.

Decision:

Accept the provider-layer guard:

- the shared `installDsGui` test helper installs throwing `window.kun` and
  `window.reasonix` getters;
- existing provider tests cover analytix-owned HTTP routes, thread lifecycle,
  approval/user-input, fork/resume, and dynamic route id encoding under that
  alias guard;
- route assertions remain `/health` or `/v1/*` analytix endpoints and reject
  hidden upstream route families.

Reject:

- Reasonix SessionAPI, public bridge protocol, or upstream renderer route
  names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  renderer walkthrough, or release readiness claims.

Rationale:

This broadens D-0200/D-0201 from the small runtime-client facade to the
renderer provider methods that users actually exercise.

Validation:

Focused `src/renderer/src/agent/analytix-runtime.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0053 - Renderer Runtime Provider Uses The Runtime Client Facade

Date: 2026-06-22
Sources: D-0203 renderer provider runtime client facade seal, active
`AnalytixRuntimeProvider`, Reasonix frontend/session boundary review
Domain: renderer provider bridge, product sovereignty
Status: accepted

Conflict:

Even direct calls to `window.analytix.runtime.runtimeRequest` are
analytix-owned, but scattering them through provider methods weakens the
single renderer facade proof and makes future alias/config regressions harder
to catch.

Decision:

Accept only provider runtime requests through `rendererRuntimeClient`:

- `archiveThread` uses `rendererRuntimeClient.runtimeRequest`, like the rest
  of the provider runtime request surface;
- archive/restore still uses `analytixThreadPath(threadId)` and the same
  `PATCH` body;
- `scan:product-sovereignty` forbids
  `window.analytix.runtime.runtimeRequest` inside `analytix-runtime.ts`.

Reject:

- direct provider bridge bypasses for runtime requests;
- Reasonix SessionAPI, public bridge protocol, or upstream renderer route
  names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  renderer walkthrough, or release readiness claims.

Rationale:

This makes the renderer provider source match the D-0200/D-0201 facade proofs:
renderer code calls the runtime through one product-owned client boundary.

Validation:

Focused `src/renderer/src/agent/analytix-runtime.test.ts` passes. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0054 - Side Conversation Promotion Uses Provider Relation Contract

Date: 2026-06-22
Sources: D-0204 side conversation relation provider contract, D-0203 renderer
provider runtime client facade seal, Reasonix frontend/session boundary review
Domain: renderer store/provider bridge, product sovereignty
Status: accepted

Conflict:

Side conversation promotion needs to PATCH thread relation, but store code
must not reach directly into the runtime bridge. Otherwise side-conversation
behavior becomes a second renderer control plane outside the provider/runtime
client boundary.

Decision:

Accept the provider relation contract:

- `AgentProvider.updateThreadRelation` is optional and analytix-owned;
- `AnalytixRuntimeProvider.updateThreadRelation` sends `{ relation }` through
  `rendererRuntimeClient.runtimeRequest`;
- `promoteSideConversation` calls
  `provider.updateThreadRelation(sideId, 'primary')`;
- `scan:product-sovereignty` forbids
  `window.analytix.runtime.runtimeRequest` in `chat-store-side-actions.ts`.

Reject:

- direct store/provider bridge bypasses for runtime requests;
- Reasonix SessionAPI, side-conversation public protocol, or upstream renderer
  route names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  side-conversation walkthrough, or release readiness claims.

Rationale:

This keeps side conversation promotion inside the same provider/runtime client
contract as archive/search/fork/resume instead of creating a parallel bridge
path from the store.

Validation:

Focused `src/renderer/src/agent/analytix-runtime.test.ts` and
`src/renderer/src/store/chat-store-side-actions.test.ts` pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0055 - Renderer Runtime Requests Use The Runtime Client Facade

Date: 2026-06-22
Sources: D-0205 renderer usage runtime client facade seal, D-0200 renderer
client bridge proof, D-0204 side conversation relation provider contract
Domain: renderer bridge, usage/debug diagnostics, product sovereignty
Status: accepted

Conflict:

Renderer usage/debug surfaces need runtime HTTP data, but direct
`window.analytix.runtime.runtimeRequest` calls scattered through hooks and
components bypass the single renderer client proof. Dedicated preload APIs such
as config-file access, provider probing, runtime restart, and model fetching
remain separate contracts; generic runtime HTTP requests should use the client
facade.

Decision:

Accept the renderer runtime client facade as the only production generic
runtime request path:

- thread, daily, and model usage loaders call `rendererRuntimeClient.runtimeRequest`;
- settings token economy savings and LLM debug requests call the same facade;
- request paths and parsing remain unchanged;
- `scan:product-sovereignty` forbids
  `window.analytix.runtime.runtimeRequest` across renderer production source.

Reject:

- direct renderer runtime request bridge bypasses;
- Reasonix SessionAPI, usage/debug public protocol, or upstream route names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  usage/dashboard walkthrough, or release readiness claims.

Rationale:

This completes the renderer-side generic runtime request seal. Renderer code
can still use named `window.analytix.runtime.*` APIs for config/probe/restart
contracts, but `/v1/*` HTTP requests go through the facade proved in D-0200.

Validation:

Focused usage/settings tests pass. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0056 - Renderer Settings Reads Use The Runtime Client Facade

Date: 2026-06-22
Sources: D-0206 renderer settings read facade seal, D-0201 renderer settings
bridge proof
Domain: renderer settings bridge, product sovereignty
Status: accepted

Conflict:

Renderer settings readers for keyboard shortcuts, speech-to-text, and initial
usage display should not create a second settings read path outside the
renderer runtime client proof. Settings writes still have explicit named
contracts (`setSettings`, `saveSettingsSilent`) and are not part of the ordinary
read seal.

Decision:

Accept the renderer settings read facade:

- keyboard shortcut settings, speech-to-text settings, and initial usage model
  label reads call `rendererRuntimeClient.getSettings`;
- existing settings-changed event listeners remain unchanged;
- `scan:product-sovereignty` forbids
  `window.analytix.settings.getSettings` in renderer production source.

Reject:

- direct renderer settings read bypasses;
- Reasonix config roots or public settings protocol;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or
  legacy GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated settings fallback, packaged
  settings walkthrough, or release readiness claims.

Rationale:

This closes ordinary renderer settings reads behind the same
`rendererRuntimeClient` boundary as settings writes and generic runtime HTTP
requests, while leaving named write APIs explicit.

Validation:

Focused heatmap/runtime-client tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0057 - Renderer Direct Window APIs Must Be Named Allow-listed Contracts

Date: 2026-06-22
Sources: D-0207 renderer named bridge API allow-list, D-0205 renderer usage
runtime client facade seal, D-0206 renderer settings read facade seal,
D-0200/D-0201 renderer bridge proofs
Domain: renderer bridge, preload contracts, product sovereignty
Status: accepted

Conflict:

After generic runtime HTTP requests and ordinary settings reads are sealed
behind `rendererRuntimeClient`, a few direct `window.analytix.runtime.*` and
`window.analytix.settings.*` calls remain valid because they are named preload
contracts. Without an explicit allow-list, future work could silently add
Reasonix/Kun-like public bridge methods or reintroduce generic bypasses.

Decision:

Accept only named direct renderer window APIs that are explicitly allow-listed:

- runtime config file access, config file save, config directory open,
  provider probe, upstream model fetch, runtime restart, and runtime status
  subscription;
- settings writes through `setSettings` and `saveSettingsSilent`;
- generic runtime HTTP requests continue through
  `rendererRuntimeClient.runtimeRequest`;
- ordinary settings reads continue through `rendererRuntimeClient.getSettings`;
- `scan:product-sovereignty` fails on any unlisted direct
  `window.analytix.runtime.*` or `window.analytix.settings.*` renderer
  production call.

Reject:

- direct generic runtime request or settings read bypasses;
- arbitrary new direct renderer window APIs without an analytix-owned contract
  and scan allow-list update;
- Reasonix SessionAPI, config roots, public bridge/session protocol, or route
  names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or legacy
  GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, packaged bridge walkthrough, or release
  readiness claims.

Rationale:

This keeps named preload contracts explicit while preserving the stronger
renderer client facade for runtime HTTP and settings reads. The boundary is
machine-checkable, so product sovereignty does not depend on code review memory.

Validation:

`node --check scripts/scan-product-sovereignty.cjs` and the full product
sovereignty scan pass. Full final gates are recorded in release evidence.

## D-0058 - Optional Chaining Does Not Exempt Renderer Bridge Governance

Date: 2026-06-23
Sources: D-0208 renderer optional bridge bypass seal, D-0207 renderer named
bridge API allow-list, D-0205/D-0206 renderer client facade seals
Domain: renderer bridge, scan coverage, product sovereignty
Status: accepted

Conflict:

Renderer code can access `window.analytix` through optional chaining. If scans
only match dot access, `window.analytix?.runtime?.runtimeRequest` and
`window.analytix?.settings?.getSettings` can bypass the generic runtime/settings
facade rule while looking harmless as capability checks.

Decision:

Treat optional-chain bridge access as the same governance surface as dot
access:

- `window.analytix?.runtime?.runtimeRequest` is forbidden in renderer
  production source;
- `window.analytix?.settings?.getSettings` is forbidden in renderer production
  source;
- named runtime/settings APIs remain allowed only when the allow-list scan
  accepts the method name;
- Connect Phone dialog settings reads use `rendererRuntimeClient.getSettings`;
- plugin marketplace diagnostics do not probe the generic runtime bridge
  directly.

Reject:

- using optional chaining as a capability-check loophole for generic runtime
  HTTP or settings reads;
- expanding named direct window APIs without an analytix-owned contract and
  scan allow-list update;
- Reasonix SessionAPI, config roots, public bridge/session protocol, or route
  names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or legacy
  GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- renderer-visible Go routes, default Go backend, live Go bridge, Rust/Tauri
  rewrite, packaged bridge walkthrough, or release readiness claims.

Rationale:

Bridge governance has to match JavaScript syntax reality. A direct optional
access is still a direct access, so the scanner and source code now enforce the
same boundary.

Validation:

Focused syntax/negative `rg` checks and full product-sovereignty scan pass.
Full final gates are recorded in release evidence.

## D-0059 - Renderer Bridge Allow-list G5 Shadow Does Not Enable Go Desktop Bridge

Date: 2026-06-23
Sources: D-0209 renderer bridge allow-list G5 shadow, D-0207 named bridge API
allow-list, D-0208 optional bridge bypass seal
Domain: Go G5, renderer bridge, product sovereignty
Status: accepted

Conflict:

The renderer bridge allow-list is important enough to replay in Go G5
conformance, but adding it to `controlExecutableCases.desktopSovereignty` must
not imply that Go owns the desktop bridge or can become the default backend.

Decision:

Accept only pure executable shadow evidence:

- TypeScript conformance derives direct renderer runtime/settings methods from
  production renderer source;
- the fixture records named allow-lists, direct methods, violations, bypass
  counts, and optional-chain scan coverage;
- Go computes the expected bridge-allow-list output from that TS-owned fixture;
- real renderer/preload/main/runtime behavior remains owned by
  `window.analytix`, top-level `runtime` settings, `runtime:request`,
  `runtime:sse:*`, and `analytix serve`.

Reject:

- live Go desktop bridge ownership from this proof;
- renderer-visible Go routes, default Go backend, or Electron-to-Go routing;
- Reasonix SessionAPI, public bridge/session protocol, config roots, or route
  names;
- deprecated bridge aliases such as `window.kun`, `window.reasonix`, or legacy
  GUI bridge names;
- hidden top-level route or renderer templates for Workflow/Create
  Loop/Subagent/AutoResearch/MCP-indexer;
- Rust/Tauri rewrite, packaged bridge walkthrough, G6 readiness, or release
  readiness claims.

Rationale:

G5 shadow is valuable because it makes the bridge allow-list machine-checkable
across TypeScript and Go. It remains evidence, not backend selection.

Validation:

Focused Go conformance and Go package tests pass. Full final gates are recorded
in release evidence.

## D-0050 - Forbidden Route 404 Matrix Is Not Hidden Capability Exposure

Date: 2026-06-22
Sources: D-0189 forbidden route token list, analytix TypeScript HTTP router,
Reasonix route/control review
Domain: runtime HTTP route absence, product sovereignty
Status: accepted

Conflict:

Negative route evidence is useful only if it proves absence. It must not
register placeholder routes, reserve Reasonix protocol paths, or imply hidden
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer capabilities.

Decision:

Accept only actual TypeScript router not-found evidence:

- the test reads `runtimeHttpRouteSovereignty.forbiddenRouteTokens`;
- it dispatches each token with valid auth through the real TypeScript router;
- each token must return structured 404
  `{ code: "not_found", message: "route not found" }`;
- no forbidden route is registered, even as an authenticated stub.

Reject:

- Reasonix SessionAPI or public route protocol;
- placeholder `/v1/reasonix`, `/v1/runtime/go`, Workflow/Create Loop/Subagent/
  AutoResearch/MCP-indexer route registration;
- renderer-visible Go routes, default Go backend, live Go HTTP server, or G6
  readiness from this proof;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged route/release readiness claims.

Rationale:

This complements the source-level route inventory with real router behavior:
forbidden route families are absent, not merely unauthenticated or dormant.

Validation:

Focused `go-runtime-conformance.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0046 - Sub-Agent Review Claims Stay Evidence-Only

Date: 2026-06-22
Sources: D-0179 six-lane sub-agent review, Reasonix post-881 absorption stream,
Kun 0.2.13/0.2.14 baseline
Domain: QA evidence, provider/cache claims, Go G5/G6 boundary
Status: accepted

Conflict:

The six-lane review proves several important Reasonix-inspired capabilities,
but it could be overread as live Reasonix parity, custom provider cache
telemetry, Go backend readiness, or Kun navigation expansion.

Decision:

Accept the review only as scan-visible QA evidence:

- Reasonix deltas must remain classified as contract-reimplement,
  code-port-and-adapt, document-only, reject, or defer;
- custom provider proof is request-shape/full-endpoint coverage only, not
  independent custom cache telemetry;
- Kun top-level navigation remains Code, Write, Settings, Plugins, Connect
  Phone, and Schedule;
- Go remains G5 shadow-only until live implementation, rollback, and G6 gates
  are satisfied.

Reject:

- live provider/cache superiority claims without credentialed matrix evidence;
- Reasonix public protocol, SessionAPI, controller API, config roots, or route
  names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  entries;
- default Go backend, renderer-visible Go routes, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback;
- treating scan freshness as packaged desktop QA or release readiness.

Rationale:

The review closes the decision record for the current stage while keeping the
product boundary and evidence standard sharper than the upstream import path.

Validation:

Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0047 - Custom Providers Stay Request-Shape-Only Until Telemetry Exists

Date: 2026-06-22
Sources: D-0180 provider/cache control shadow, D-0179 sub-agent review matrix,
Reasonix provider/cache review
Domain: provider/cache accounting, custom full endpoints, Go G5 shadow
Status: accepted

Conflict:

Custom full endpoints are part of the provider request-shape surface, but the
current fixtures do not contain independent custom-provider cache telemetry.
Counting them as cache telemetry would overstate analytix parity and blur the
line between endpoint correctness and cache accounting.

Decision:

Accept only a request-shape-only custom provider proof:

- custom full endpoint ids must remain in request-shape cases;
- custom full endpoint telemetry ids must remain empty;
- supported cache telemetry remains DeepSeek/OpenAI Responses/Anthropic only;
- Go G5 may replay the seal as executable shadow evidence.

Reject:

- claiming independent custom provider cache telemetry from request-shape
  fixtures;
- counting unknown/custom providers as cache misses;
- live provider/cache superiority without a credentialed matrix;
- Reasonix provider public protocol or config roots;
- default Go backend, renderer-visible Go routes, Kun identity, deprecated
  bridge/settings fallback, hidden top-level entries, Rust/Tauri rewrite,
  packaged provider QA, G6 readiness, or release readiness.

Rationale:

The seal makes analytix stricter than a naive Reasonix import path: endpoint
coverage and cache telemetry are proven separately.

Validation:

Focused provider-cache and Go conformance tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0048 - Goal Persistence Off-Lock Is a Go Conformance Requirement

Date: 2026-06-22
Sources: D-0181 G5 control shadow, Reasonix `341f720`, analytix `ThreadService`
Domain: goal persistence, controller locks, Go G5/G6 boundary
Status: accepted

Conflict:

Reasonix split goal-state writes around controller locks, but analytix
TypeScript runtime has no equivalent global controller/status/approval lock.
Adding a TypeScript lock abstraction would create architecture that analytix
does not need.

Decision:

Accept the value only as a future-Go conformance requirement:

- TypeScript `ThreadService` remains lock-free for goal persistence;
- `setGoal` and `clearGoal` must persist thread state before emitting goal
  events;
- persistence failures must warn with analytix identity and surface the
  original error;
- Go G5 shadow must prove no shared status/approval/controller lock coupling.

Reject:

- importing Reasonix controller protocol or lock API;
- adding a TypeScript controller lock just to mirror Reasonix;
- live Go goal manager or default Go backend from this evidence;
- renderer-visible Go routes, Kun identity, deprecated bridge/settings
  fallback, hidden top-level entries, Rust/Tauri rewrite, packaged goal QA, G6
  readiness, or release readiness.

Rationale:

The conformance fixture preserves the Reasonix responsiveness lesson while
keeping analytix runtime contracts and current TypeScript architecture intact.

Validation:

Focused `go-runtime-conformance.test.ts`, `thread-service.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0067 - Browser Preview Settings Must Use Analytix Runtime Schema

Date: 2026-06-22
Sources: Reasonix config review, Kun legacy agent settings, analytix browser preview
Domain: settings / bridge / product sovereignty
Status: accepted

Conflict:

Browser preview settings can be hydrated from localStorage and desktop preview
patches before Electron preload is active. If this path accepts upstream
`agentProvider`, `agents`, `deepseek`, `reasonix`, or auto-plan fields, it can
reintroduce deprecated settings fallback without touching the desktop settings
store.

Decision:

Browser preview settings must normalize through the same analytix-owned schema
as desktop settings. Valid settings survive under top-level `runtime`; rejected
legacy/Reasonix app and runtime fields are stripped on load and re-save.

Rationale:

The renderer development bridge is still part of the product contract surface.
It must not become a side door for Kun/Reasonix settings identity or public
auto-plan config.

Validation:

`app-settings.test.ts` and `browser-analytix-bridge.test.ts` cover shared
normalization plus browser preview load/re-save behavior. Full final gates and
forbidden-surface scans are recorded in release evidence.

Supersedes:

Superseded by:

## D-0068 - IPC Settings Patches Strip Top-Level Legacy Envelopes

Date: 2026-06-22
Sources: Reasonix config review, Kun legacy agent settings, analytix IPC settings schema
Domain: settings / IPC / product sovereignty
Status: accepted

Conflict:

Renderer-to-main settings patches are strict, but upstream-polluted patches can
carry top-level `agent`, `autoPlan`, `auto_plan`, `agentProvider`, `agents`,
`deepseek`, or `reasonix` envelopes. Rejecting the whole patch can drop valid
analytix settings changes; accepting the envelopes would reintroduce deprecated
fallback settings.

Decision:

Top-level legacy/Reasonix envelopes are stripped before strict IPC schema
validation. Valid analytix patch fields survive. Runtime-nested legacy
agent-shaped fields remain rejected and are not merged as fallback runtime
configuration.

Rationale:

This preserves user-facing settings updates while preventing Kun/Reasonix
settings identities from entering persisted settings or runtime rebuild input.

Validation:

`app-ipc-schemas.test.ts` covers the polluted patch case. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0069 - Settings Sovereignty Chokepoints Stay In Product Scan

Date: 2026-06-22
Sources: D-0155 browser bridge/entry sovereignty, D-0157 browser settings
sovereignty, D-0158 IPC settings patch sovereignty
Domain: settings / bridge / product sovereignty scan
Status: accepted

Conflict:

The settings and bridge sovereignty proofs now span shared normalization,
settings-store persistence, IPC patch validation, preload, and browser preview
bridge code. If any chokepoint or its guard test disappears from the scan path,
future Kun/Reasonix absorption could silently reopen deprecated settings or
bridge surfaces.

Decision:

`scan:product-sovereignty` must include a dedicated settings sovereignty path
freshness list covering shared settings normalization/runtime settings, main
settings-store, IPC settings schema, preload, browser preview bridge, and their
focused guard tests.

Reject:

- treating path freshness as packaged desktop settings QA;
- accepting Reasonix config roots, public auto-plan settings, or deprecated
  Kun/Reasonix agent-provider fallback;
- exposing deprecated bridge aliases, renderer-visible Go routes, default Go
  backend behavior, or Rust/Tauri migration paths.

Rationale:

This keeps the newly proven settings/bridge chokepoints in the repeatable
forbidden-surface scan without changing active runtime behavior.

Validation:

`npm run scan:product-sovereignty` covers the settings sovereignty path
freshness gate. Full final gates are recorded in release evidence.

## D-0070 - Singular Workflow Route Stays Forbidden

Date: 2026-06-22
Sources: Reasonix public route rejection, task-job route-boundary oracle, Go
G4/G5 forbidden route replay
Domain: runtime HTTP routes / product sovereignty
Status: accepted

Conflict:

The task-job and Go G4/G5 route-boundary fixtures record singular
`/v1/workflow` as a forbidden top-level public route, but the live HTTP
negative-route test previously covered only plural `/v1/workflows`. The public
runtime proof must cover both spellings so a future route addition cannot turn
internal workflow/task capabilities into a public surface.

Decision:

The TypeScript runtime HTTP router negative-route test must include singular
`/v1/workflow` and plural `/v1/workflows`, and both must return structured
404s.

Reject:

- exposing Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP
  routes;
- treating internal task/job/research capabilities as public product entries;
- exposing Reasonix SessionAPI/public protocol, renderer-visible Go routes, or
  default Go backend behavior;
- treating this unit-level route proof as packaged desktop QA or release
  readiness.

Rationale:

This aligns live TS route evidence with the existing G4/G5 forbidden-route
oracle while preserving analytix-owned runtime contracts.

Validation:

`http-server.test.ts` covers singular and plural Workflow route absence. Full
final gates are recorded in release evidence.

## D-0071 - Runtime Conformance Proofs Stay In Product Scan

Date: 2026-06-22
Sources: provider/cache oracle, approval/user-input route oracle, Go G2/G3/G4/G5
shadow conformance, runtime HTTP route surface review
Domain: runtime conformance / product sovereignty scan / Go shadow boundary
Status: accepted

Conflict:

Provider/cache, approval/user-input, route replay, and Go shadow evidence now
live across many fixtures, tests, and Go source files. If those proof entry
points disappear or active route/client source starts exposing forbidden
public route strings, the project could appear current while silently losing
analytix-owned contract evidence.

Decision:

`scan:product-sovereignty` must include runtime proof freshness paths for the
provider-cache oracle, approval/user-input oracle, G2/G3/G4/G5 conformance
fixtures/tests, and Go shadow sources. It must also scan active runtime routes,
shared endpoint templates, IPC schema, and renderer runtime client source for
forbidden Reasonix/Go/Workflow public surfaces.

Reject:

- treating scan freshness as a new provider/cache or approval/user-input
  behavior oracle;
- exposing Reasonix SessionAPI/public protocol or Workflow/Create Loop/
  Subagent/AutoResearch/MCP-indexer public HTTP routes;
- enabling live Go HTTP/provider routes, renderer-visible Go routes, or default
  Go backend behavior;
- treating scan freshness as packaged route/provider QA or release readiness.

Rationale:

This keeps the current runtime proof surface from rotting while preserving the
rule that TypeScript runtime contracts and fixtures remain authoritative until
G5/G6 gates are actually satisfied.

Validation:

`npm run scan:product-sovereignty` covers runtime proof freshness and active
route/client forbidden-surface scans. Full final gates are recorded in release
evidence.

## D-0072 - Renderer Runtime Requests Stay On Analytix Routes

Date: 2026-06-22
Sources: renderer runtime adapter, Reasonix public route rejection, runtime
HTTP route sovereignty
Domain: renderer runtime / product sovereignty / route contract
Status: accepted

Conflict:

Source scans can prove forbidden route strings are absent from active renderer
runtime code, but the renderer provider also needs executable evidence that
common runtime flows actually call analytix-owned HTTP paths and not
Reasonix/Go/Workflow public surfaces.

Decision:

`AnalytixRuntimeProvider` tests must capture common `runtimeRequest` paths and
assert they stay on `/health` or analytix-owned `/v1/*` paths while rejecting
Reasonix public routes, renderer-visible Go routes, and Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer public routes.

Reject:

- exposing Reasonix SessionAPI/public protocol or Kun public protocol through
  renderer runtime requests;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP
  routes or top-level navigation;
- enabling renderer-visible Go routes or default Go backend behavior;
- treating renderer unit proof as packaged desktop route QA or release
  readiness.

Rationale:

This turns route sovereignty into renderer-executable behavior while keeping
the active public contract under `window.analytix` and analytix HTTP/SSE.

Validation:

`analytix-runtime.test.ts` covers the renderer request-surface proof. Full
final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0073 - Renderer SSE Cursor Uses Analytix Bridge

Date: 2026-06-22
Sources: renderer runtime adapter, Reasonix public SSE/session rejection,
analytix bridge contract
Domain: renderer SSE / product sovereignty / bridge contract
Status: accepted

Conflict:

Reasonix-style session/SSE protocols are useful upstream references, but
renderer event streaming in analytix must remain behind `window.analytix` and
the existing preload/main/runtime SSE boundary. A direct renderer runtime
request or public Reasonix SSE route would blur the contract.

Decision:

`AnalytixRuntimeProvider.subscribeThreadEvents` tests must prove the renderer
starts streams through `window.analytix.runtime.startSse(threadId, sinceSeq,
streamId)`, uses a generated non-empty stream id for event dispatch, and calls
`stopSse(streamId)` for cleanup.

Reject:

- exposing Reasonix public SSE routes, SessionAPI, or Kun public protocol;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP
  routes or top-level navigation;
- enabling renderer-visible Go SSE routes or default Go backend behavior;
- treating renderer unit proof as packaged desktop SSE QA or release
  readiness.

Rationale:

This keeps SSE replay/cursor currentness as an analytix-owned bridge contract:
renderer orchestration uses the bridge, while URL/header/reconnect behavior
remain proven at preload/main/browser bridge layers.

Validation:

`analytix-runtime.test.ts` covers the renderer SSE bridge/cursor proof. Full
final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0074 - Main/Preload SSE IPC Stays On The Analytix Bridge

Date: 2026-06-22
Sources: preload bridge, main runtime SSE IPC, Reasonix public SSE/session
rejection
Domain: desktop SSE IPC / product sovereignty / bridge contract
Status: accepted

Conflict:

The useful part of Reasonix SSE/session behavior is durable cursor replay and
stable stream lifecycle handling. In analytix, those semantics must flow
through the desktop bridge (`window.analytix.runtime` -> `runtime:sse:*` IPC ->
`/v1/threads/:id/events`) rather than a public Reasonix, Kun, or Go-visible
SSE protocol.

Decision:

Preload and main IPC tests must prove the desktop SSE chain preserves
`threadId`, `sinceSeq`, and `streamId`, rejects extra upstream session fields,
requests the analytix-owned events route with runtime auth and cursor headers,
and stops only the matching stream id.

Reject:

- exposing Reasonix public SSE routes, SessionAPI, or Kun public protocol;
- adding deprecated bridge aliases such as `window.kunGui` or `window.reasonix`;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP
  routes or top-level navigation;
- enabling renderer-visible Go SSE routes or default Go backend behavior;
- treating source/unit proof as packaged desktop SSE QA or release readiness.

Rationale:

This completes the desktop side of the SSE contract evidence after renderer
bridge proof: renderer subscribes through the bridge, preload maps to
analytix-owned IPC, and main owns the runtime HTTP/SSE route and stream
identity.

Validation:

`preload-sandbox.test.ts`, `app-ipc-schemas.test.ts`, and
`runtime-sse-ipc.test.ts` cover the main/preload SSE IPC bridge proof. Full
final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0075 - Runtime Request IPC Rejects Upstream Public Routes

Date: 2026-06-22
Sources: main IPC runtime request schema, Reasonix public runtime protocol
rejection, renderer runtime request surface proof
Domain: runtime request IPC / product sovereignty / route contract
Status: accepted

Conflict:

Renderer runtime requests are useful only if the main IPC boundary also
prevents upstream public protocols from being smuggled through
`runtime:request`. A generic "outside modeled API" guard is correct, but the
forbidden upstream route families must remain explicit negative evidence.

Decision:

`runtimeRequestPayloadSchema` and the `runtime:request` handler tests must
explicitly reject Reasonix public routes, renderer-visible Go routes,
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route families, and
prove invalid payloads do not call the runtime request adapter.

Reject:

- exposing Reasonix SessionAPI/public runtime routes or Kun public protocol;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP
  routes or top-level navigation;
- enabling renderer-visible Go routes or default Go backend behavior;
- treating IPC schema proof as packaged desktop route QA or release readiness.

Rationale:

This makes the main IPC boundary as explicit as the renderer provider proof:
runtime requests are allowed only through analytix-owned endpoint templates,
and upstream protocol names remain negative tests rather than public surface.

Validation:

`app-ipc-schemas.test.ts` and `register-app-ipc-handlers.test.ts` cover the
main runtime request forbidden-route proof. Full final gates are recorded in
release evidence.

Supersedes:

Superseded by:

## D-0076 - Preload Runtime Requests Use Analytix IPC Only

Date: 2026-06-22
Sources: preload bridge, main runtime request allow-list, Reasonix public
runtime protocol rejection
Domain: preload bridge / product sovereignty / runtime request IPC
Status: accepted

Conflict:

The renderer must talk to main through the analytix preload bridge. If preload
introduced a Reasonix/Kun/Go/Workflow request channel, main allow-list tests
would no longer fully describe the public desktop contract.

Decision:

`preload-sandbox.test.ts` must prove `window.analytix.runtime.runtimeRequest`
maps to `runtime:request` with `{ path, method, body }` and does not introduce
Reasonix, Kun, Go, or Workflow runtime request IPC channel names.

Reject:

- exposing Reasonix SessionAPI/public runtime IPC or Kun public protocol;
- adding deprecated bridge aliases such as `window.kunGui` or `window.reasonix`;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer IPC or public
  route surfaces;
- enabling renderer-visible Go routes or default Go backend behavior;
- treating preload source proof as packaged desktop route QA or release
  readiness.

Rationale:

This closes the bridge handoff around runtime requests: renderer code emits
analytix-owned paths, preload uses the analytix IPC channel, and main owns the
allow-list.

Validation:

`preload-sandbox.test.ts` covers the preload runtime request IPC bridge proof.
Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0077 - Desktop Runtime Bridge Proofs Stay In Product Scan

Date: 2026-06-22
Sources: product-sovereignty scan, renderer/preload/main runtime bridge proofs,
Reasonix public protocol rejection
Domain: proof freshness / product sovereignty / runtime bridge contract
Status: accepted

Conflict:

The runtime bridge proof chain now spans renderer provider tests, browser
preview bridge tests, preload source guards, main IPC schema/handler tests, and
main SSE IPC tests. If those proof files disappear, the product boundary can
regress even if source forbidden-string scans still pass.

Decision:

`scan:product-sovereignty` must include a `runtimeDesktopBridgeProofPaths`
freshness gate covering the source and test files that prove the desktop
runtime request/SSE bridge contract.

Reject:

- treating source absence as acceptable when forbidden strings are not found;
- exposing Reasonix/Kun public runtime or SSE protocols;
- adding Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public HTTP or
  IPC surfaces;
- enabling renderer-visible Go routes or default Go backend behavior;
- treating scan freshness as packaged desktop route/SSE QA or release
  readiness.

Rationale:

This turns the proof chain itself into a guarded artifact: analytix keeps the
renderer -> preload -> main -> runtime boundary strong only if the tests and
source chokepoints remain present.

Validation:

`npm run scan:product-sovereignty` covers runtime desktop bridge proof path
freshness. Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0078 - Provider Cache Coverage Floor Is Fixture-Owned

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go
G3/G5 conformance inputs
Domain: provider/cache / fixture governance / Go shadow boundary
Status: accepted

Conflict:

Reasonix's DeepSeek cache focus is useful, but analytix must also prove that
OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full
endpoint request-shape behavior do not regress. A loose provider-family
metadata field could make the proof look present while the oracle coverage
silently shrinks.

Decision:

Accept only fixture-owned coverage-floor evidence:

- provider-family proof is constrained to DeepSeek, OpenAI-compatible chat,
  OpenAI Responses, Anthropic Messages, and custom full endpoints;
- runtime tests derive provider-family coverage from the usage and request
  shape cases instead of trusting prose metadata alone;
- required usage ids, request-shape ids, endpoint formats, telemetry-supported
  cases, unsupported unknown cache posture, and custom full endpoint cases are
  executable assertions;
- live provider/cache superiority remains blocked until credentialed provider
  QA exists.

Reject:

- treating fixture/local-HTTP evidence as credentialed live provider parity;
- exposing Reasonix provider routes, config roots, or public provider protocol;
- weakening OpenAI/Anthropic/custom provider coverage to DeepSeek-only proof;
- enabling a default Go backend, renderer-visible Go route, Rust/Tauri path,
  Kun identity, deprecated bridge/settings fallback, or top-level hidden
  entries from this evidence.

Rationale:

This keeps Reasonix's cache lessons useful while preserving analytix's broader
provider contract and avoiding unsupported superiority claims.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`go-runtime-g3-g4-conformance.test.ts` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

Supersedes:

Superseded by:

## D-0097 - Provider Coverage Shadow Is Not Go Provider Parity

Date: 2026-06-22
Sources: D-0168 provider/cache coverage floor, Go G5 shadow, Reasonix
provider/cache review
Domain: provider/cache / Go shadow / product sovereignty
Status: accepted

Conflict:

Promoting provider/cache coverage into Go G5 executable output strengthens the
future backend gate, but it can be mistaken for a live Go provider client or a
credentialed provider parity result.

Decision:

Accept only shadow-owned replay:

- `controlExecutableCases.providerCacheCoverageFloor` is derived from
  TypeScript provider-cache fixtures;
- Go computes provider family coverage, telemetry-supported/unsupported cases,
  custom full endpoint exact-URL/tool-shape coverage, and provider-family split
  ids from fixture summaries;
- TypeScript runtime and existing provider clients remain authoritative;
- no renderer-visible Go route or default backend is enabled.

Reject:

- treating Go shadow output as live provider execution or live provider/cache
  superiority;
- exposing Reasonix provider protocol, config roots, SessionAPI, or public
  runtime routes;
- weakening OpenAI/Anthropic/custom provider coverage to DeepSeek-only proof;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries.

Rationale:

The executable shadow should make the future Go gate stricter without changing
the active analytix runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts`, `provider-cache-proof.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go go test`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

Supersedes:

Superseded by:

## D-0098 - Provider Request Shape Replay Is Not Provider Runtime Mutation

Date: 2026-06-22
Sources: D-0170 provider request-shape derived replay, Reasonix provider/cache
review, analytix Base URL / Endpoint format contract
Domain: provider request shape / Go shadow / product sovereignty
Status: accepted

Conflict:

Deriving provider URL/header/body/tool-shape behavior in Go shadow can be
mistaken for permission to change active provider request construction, or for
a live provider compatibility claim.

Decision:

Accept only fixture-owned request-shape replay:

- G3/G5 shadow derives expected URL from `baseUrl` and `endpointFormat`;
- custom full endpoints preserve exact URLs and have appended-path count `0`;
- header/body/tool-shape match ids are computed from fixture inputs for
  OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, DeepSeek chat,
  and custom full endpoints;
- TypeScript provider clients and shared URL helpers remain the active runtime
  authority.

Reject:

- changing active provider URL/body/header/stream/usage behavior from this
  shadow replay;
- exposing Reasonix provider protocol, config roots, SessionAPI, or public
  runtime routes;
- treating fixture replay as credentialed live provider parity;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries.

Rationale:

The replay makes the future Go/provider gate stricter while preserving the
analytix-owned provider contract and avoiding unsupported live-provider claims.

Validation:

Focused `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts`,
`provider-cache-proof.test.ts`, and `packages/runtime-go go test -count=1`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

Supersedes:

Superseded by:

## D-0099 - Runtime Proof Freshness Is A Guard, Not Runtime Parity

Date: 2026-06-22
Sources: D-0171 post-881 runtime proof freshness v2, Reasonix planner/task/MCP/
provider review, product-sovereignty scan
Domain: proof freshness / product sovereignty / runtime parity
Status: accepted

Conflict:

Positive scan tokens are useful to prevent evidence-chain regressions, but they
can be mistaken for live runtime parity or product-surface acceptance.

Decision:

Accept scan freshness only as a guardrail:

- key post-881 proof files must remain present;
- provider coverage/request-shape, planner/task-job, and MCP lifecycle/search/
  approval tokens must remain visible to the scan;
- focused runtime tests remain the behavioral proof;
- TypeScript runtime remains authoritative until explicit G5/G6 live gates
  pass.

Reject:

- treating scan token presence as live provider/MCP/job parity;
- exposing Reasonix public protocols or upstream route names;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries;
- replacing focused runtime tests, Go shadow tests, packaged QA, or release
  evidence with the scan alone.

Rationale:

The guard makes regressions louder without expanding analytix's public product
surface or backend readiness claims.

Validation:

Focused `scan:product-sovereignty`, `go-runtime-conformance.test.ts`,
`task-job-orchestration-oracle.test.ts`, `mcp-tool-lifecycle-oracle.test.ts`,
and `create-plan-tool.test.ts` pass. Full final gates are recorded in release
evidence.

Supersedes:

Superseded by:

## D-0100 - Raw Provider Cache Accounting Is Fixture Replay, Not Live Provider Parity

Date: 2026-06-22
Sources: D-0172 provider cache accounting raw payload replay, Reasonix provider
cache accounting review, analytix provider-cache oracle
Domain: provider cache accounting / Go shadow / product sovereignty
Status: accepted

Conflict:

Parsing raw provider response payloads in Go/TS shadow can be mistaken for a
live provider client, a provider request/stream behavior change, or a claim
that credentialed DeepSeek/OpenAI/Anthropic/custom provider parity is complete.

Decision:

Accept only fixture-owned raw payload replay:

- provider usage/accounting fixtures carry raw `responseBody.usage` payloads;
- TS and Go shadow code derive prompt/completion/reasoning/cache hit/miss/rate
  values from those raw payloads before computing cache accounting;
- unsupported cache telemetry remains unknown rather than counted as misses;
- TypeScript model clients and provider URL/body/stream parsing remain the
  active runtime authority.

Reject:

- changing active provider URL/body/header/stream/usage behavior from this
  shadow replay;
- exposing Reasonix provider protocol, config roots, SessionAPI, or public
  runtime routes;
- treating fixture replay as credentialed live provider parity or live cache
  superiority;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries.

Rationale:

The replay tightens the future G3/G5 provider gate by proving cache accounting
comes from raw provider shapes, while keeping analytix's current provider
contract and live-provider claims unchanged.

Validation:

Focused `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts`,
`provider-cache-proof.test.ts`, and `packages/runtime-go go test -count=1`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

Supersedes:

Superseded by:

## D-0101 - Raw Accounting Proof Freshness Is Not Runtime Provider Parity

Date: 2026-06-22
Sources: D-0173 provider raw accounting proof freshness, D-0172 raw provider
cache accounting replay, product-sovereignty scan
Domain: provider cache accounting / proof freshness / product sovereignty
Status: accepted

Conflict:

A direct raw accounting oracle test and scan token can be mistaken for active
provider behavior parity, live provider superiority, or permission to weaken
the Go shadow-only backend boundary.

Decision:

Accept raw accounting proof freshness only as guardrail evidence:

- `provider-cache-proof.test.ts` must prove raw provider payloads derive the
  same usage/accounting semantics as the oracle;
- `scan:product-sovereignty` must keep raw accounting proof tokens visible;
- focused provider/cache tests remain the behavioral proof;
- TypeScript provider clients remain the active runtime authority until future
  G5/G6 live gates pass.

Reject:

- treating scan token presence as live provider/cache parity;
- changing active provider clients, URL/body/header/stream behavior, or
  settings from this proof;
- exposing Reasonix provider protocol, config roots, SessionAPI, or public
  runtime routes;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries.

Rationale:

The guard makes raw accounting regressions louder without expanding analytix's
public product surface or backend readiness claims.

Validation:

Focused `provider-cache-proof.test.ts`, `scan:product-sovereignty`, and
`git diff --check` pass. Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0102 - Combined Step/Cancel/Cache Proof Freshness Is Not Live Loop Parity

Date: 2026-06-22
Sources: D-0174 combined step/cancel/cache proof freshness, D-0146 combined
trace control shadow, product-sovereignty scan
Domain: auto routing, step limits, cancellation, cache stability, backend boundary
Status: accepted

Conflict:

A focused combined trace oracle and scan token can be mistaken for live Go
agent-loop parity, public auto-plan behavior, or permission to expose Reasonix
controller/session surfaces.

Decision:

Accept combined proof freshness only as guardrail evidence:

- `go-runtime-conformance.test.ts` must keep the combined route-cache,
  step-limit, cancel, and stable-prefix trace inspectable;
- `scan:product-sovereignty` must keep combined proof tokens visible;
- TypeScript agent-loop/runtime behavior remains authoritative for live turns;
- Go remains shadow-only until explicit G5/G6 live gates pass.

Reject:

- treating focused trace or scan token presence as live Go loop parity;
- exposing Reasonix controller/session protocol, public auto-plan settings, or
  upstream route names;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries;
- replacing packaged long-running cancel/cache QA with this fixture proof.

Rationale:

The guard makes combined route-cache/step/cancel regressions louder without
expanding analytix's public product surface or backend readiness claims.

Validation:

Focused `go-runtime-conformance.test.ts`, `scan:product-sovereignty`, and
`git diff --check` pass. Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0103 - Approval/User-Input Proof Freshness Is Not Live Gate Parity

Date: 2026-06-22
Sources: D-0175 approval/user-input proof freshness, approval-user-input route
oracle, product-sovereignty scan
Domain: approvals, user input, replay, backend boundary
Status: accepted

Conflict:

A focused approval/user-input oracle and scan token can be mistaken for live Go
gate-manager parity, Reasonix ask/session protocol parity, or packaged
approval-card QA.

Decision:

Accept approval/user-input proof freshness only as guardrail evidence:

- `go-runtime-conformance.test.ts` must keep denial no-execute, answer privacy,
  late rejection, abort cleanup, and resume cleanup inspectable;
- `scan:product-sovereignty` must keep approval/user-input proof tokens visible;
- TypeScript gate managers and runtime HTTP routes remain authoritative for
  live behavior;
- Go remains shadow-only until explicit G5/G6 live gates pass.

Reject:

- treating focused trace or scan token presence as live Go gate parity;
- exposing Reasonix ask/session protocol, public route names, or upstream
  config shapes;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries;
- replacing packaged approval-card QA with this fixture proof.

Rationale:

The guard makes approval/user-input regressions louder without expanding
analytix's public product surface or backend readiness claims.

Validation:

Focused `go-runtime-conformance.test.ts`, `scan:product-sovereignty`, and
`git diff --check` pass. Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0104 - Session Route Proof Freshness Is Not Live Route Parity

Date: 2026-06-22
Sources: D-0176 session route proof freshness, G2 route replay oracle, G5
control executable oracle, product-sovereignty scan
Domain: thread/session routes, SSE replay, backend boundary
Status: accepted

Conflict:

A focused session route oracle and scan token can be mistaken for live Go
thread/session router parity, Reasonix SessionAPI/public event protocol parity,
or packaged route walkthrough QA.

Decision:

Accept session route proof freshness only as guardrail evidence:

- `go-runtime-conformance.test.ts` must keep archive/search, read/update, fork,
  resume-thread, SSE replay/caught-up, unauthorized auth, exact hashes, and
  boundary flags inspectable;
- `scan:product-sovereignty` must keep session route replay proof tokens
  visible and include the G2 route oracle fixture in post-881 paths;
- TypeScript runtime HTTP/SSE routes remain authoritative for live behavior;
- Go remains shadow-only until explicit G5/G6 live gates pass.

Reject:

- treating focused route/hash proof or scan token presence as live Go route
  parity;
- exposing Reasonix SessionAPI, public event protocol, or upstream route names;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries;
- replacing packaged thread/fork/resume/archive/search walkthrough QA with this
  fixture proof.

Rationale:

The guard makes thread/session route and SSE replay regressions louder without
expanding analytix's public product surface or backend readiness claims.

Validation:

Focused `go-runtime-conformance.test.ts`, `scan:product-sovereignty`, and
`git diff --check` pass. Full final gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0105 - Release Evidence Final-Gate Freshness Is Not Release Readiness

Date: 2026-06-22
Sources: D-0177 release evidence final-gate freshness, product-sovereignty scan,
release evidence gate
Domain: QA evidence, proof freshness, release boundary
Status: accepted

Conflict:

A machine-checkable final-gate scan can be mistaken for having re-run all gates,
completed packaged desktop QA, or reached release readiness.

Decision:

Accept release evidence final-gate freshness only as guardrail evidence:

- `scan:product-sovereignty` must keep the release evidence file in proof path
  freshness;
- the scan must keep final command gate entries for D-0172 through D-0177
  visible;
- the scan must keep the required command names visible: runtime package tests,
  workspace tests, typecheck, runtime build, non-cached Go shadow tests, and
  product-sovereignty scan;
- actual batch closure still requires running the relevant gates and recording
  results.

Reject:

- treating release-evidence token presence as a substitute for running gates;
- treating scan freshness as packaged desktop QA, live provider/MCP QA, G6
  readiness, or release readiness;
- enabling default Go backend, renderer-visible Go route, Rust/Tauri path, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden entries.

Rationale:

The guard makes missing final-gate evidence louder without expanding analytix's
public product surface, runtime behavior, or release readiness claims.

Validation:

Focused `scan:product-sovereignty` and `git diff --check` pass. Full final
gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0106 - Stage Closure Snapshot Is Not Full Upstream Parity

Date: 2026-06-22
Sources: D-0178 post-881 stage closure snapshot, product-sovereignty scan,
upstream scorecard, release evidence gate
Domain: QA evidence, upstream comparison, release boundary
Status: accepted

Conflict:

A closure snapshot can be mistaken for full Reasonix parity, live provider/cache
superiority, or release readiness.

Decision:

Accept the post-881 stage closure snapshot only as current evidence framing:

- the snapshot must state the current fixture-level capability floor;
- it must classify absorbed Reasonix delta families and rejected/deferred
  public-protocol/live-backend items;
- it must preserve Kun `0.2.13` -> `0.2.14` product baseline boundaries;
- it may state analytix is stronger only for verified fixture/contract/
  governance dimensions;
- it must keep live provider/cache, packaged QA, G6, and release readiness as
  open gates.

Reject:

- treating the snapshot as live Reasonix parity, live provider/cache
  superiority, live Go backend readiness, or release readiness;
- exposing Reasonix public protocol, default Go backend, renderer-visible Go
  route, Rust/Tauri path, Kun identity, deprecated bridge/settings fallback, or
  top-level hidden entries;
- replacing actual test execution or packaged QA with the snapshot.

Rationale:

The snapshot makes stage state auditable without changing product surface,
runtime behavior, or release claims.

Validation:

Focused `scan:product-sovereignty` and `git diff --check` pass. Full final
gates are recorded in release evidence.

Supersedes:

Superseded by:

## D-0066 - Browser Preview Bridge And Visible Entries Are Product Surfaces

Date: 2026-06-22
Sources: Reasonix protocol review, Kun entry baseline, analytix browser preview
Domain: UI / bridge / product sovereignty
Status: accepted

Conflict:

Browser preview bridge code and rendered sidebar entries can bypass Electron
preload tests if they are treated as local development conveniences. Upstreams
may also introduce public protocols, aliases, or top-level hidden-capability
entries through these paths.

Decision:

Browser preview bridge and visible sidebar entry surfaces are product-contract
surfaces. They must be covered by source/render tests and sovereignty scans:
only `window.analytix` is allowed, analytix runtime proxy/SSE paths remain the
contract, and Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer labels
must not render as top-level entries without a future analytix spec.

Rationale:

analytix product sovereignty is weaker if only Electron preload is guarded.
Browser preview is used during renderer development, and visible entry labels
are what users experience as the product surface.

Validation:

`Workbench.route-surface.test.ts`, `Sidebar.test.ts`,
`browser-analytix-bridge.test.ts`, and `scan:product-sovereignty` path freshness
guard this boundary. The browser bridge test also exercises the SSE proxy path,
cursor header, and event normalization behavior.

Supersedes:

Superseded by:

## D-0149 - Context Compaction Boundary Remains Analytix-Owned Shadow Proof

Date: 2026-06-22
Sources: Reasonix long-thread/currentness review, analytix context compaction
runtime semantics, Go G5 executable control shadow
Domain: context compaction, model-history boundary, provider cache stability,
Go G5
Status: accepted

Conflict:

Reasonix-style long-thread compaction/currentness behavior is valuable, but
absorbing it must not introduce a Reasonix controller/session protocol, a live
Go history manager, or dynamic cache-prefix state. The effective model history
boundary must stay an analytix-owned runtime contract.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.compactionBoundary` is fixture-owned and derived
  from analytix `effectiveHistoryAfterLatestCompaction` semantics;
- latest `kind: "compaction"` with `replacedTokens > 0` is the effective
  history boundary;
- older compactions, noop compactions, and pre-boundary items are dropped from
  the model-bound history replay;
- post-boundary user/assistant items remain visible to the model;
- compaction runtime bookkeeping remains outside immutable prefix/provider
  cache key material;
- Go tests compare the executable output to the TS-owned expected oracle.

Reject:

- implementing a live Go history manager or default Go backend from this
  evidence;
- exposing Go thread/session routes or renderer-visible Go routes;
- Reasonix SessionAPI/controller protocol or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 parity, G6 readiness, packaged
  desktop QA, or release readiness.

Rationale:

The batch raises long-thread/cache evidence quality while preserving analytix
product sovereignty and keeping TypeScript runtime contracts authoritative.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0150 - Structured User Input Validation Rejects Reasonix Ask Protocol

Date: 2026-06-22
Sources: Reasonix AskTool/user-input review, analytix `request_user_input`
runtime semantics, Go G5 executable control shadow
Domain: user input, approval gates, Go G5, product protocol boundary
Status: accepted

Conflict:

Reasonix AskTool validates structured choices in useful ways, but absorbing
that behavior must not expose Reasonix `ask` protocol, route names, CLI/config
settings, or product copy. The live contract remains analytix
`request_user_input`.

Decision:

Accept only analytix-owned validation semantics:

- `request_user_input` allows at most three questions;
- options, when provided, require two to three choices;
- option labels are deduped case-insensitively;
- invalid structured requests return `invalid_user_input_request`;
- invalid structured requests do not create a pending user-input gate;
- G5 Go shadow replays the validation matrix from TS-owned fixtures only.

Reject:

- Reasonix `ask` public protocol, CLI/config names, routes, or product copy;
- implementing a live Go approval/user-input manager from this evidence;
- exposing Go approval/user-input routes, renderer-visible Go routes, or
  default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 parity, G6 readiness, packaged
  desktop QA, or release readiness.

Rationale:

This absorbs the safety value of Reasonix AskTool validation while preserving
analytix product sovereignty and the existing GUI user-input contract.

Validation:

Focused `go-runtime-conformance.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0151 - Planner Gate Matrix Rejects Public Reasonix Planner Protocol

Date: 2026-06-22
Sources: Reasonix planner/auto-plan gating review, analytix Plan mode tool
policy, Go G5 control executable shadow
Domain: planner gating, tool policy, Go G5, product boundary
Status: accepted

Conflict:

Reasonix planner behavior is useful as a safety gate, but importing it as a
public planner/session protocol or auto-plan setting would break analytix
product sovereignty. The evidence must prove the tool-policy behavior without
creating a new top-level workflow/subagent/autoresearch product surface.

Decision:

Accept only analytix-owned Plan mode gate replay:

- normal agent mode hides `create_plan`;
- Plan capability gating allows only read-only investigation, GUI input gate
  tools, and `create_plan`;
- model step 0 narrows to read-only tools plus `create_plan`;
- later planner steps narrow to only `create_plan`;
- forged blocked calls for `task`, `parallel_tasks`, `bash`, `edit`, `write`,
  and `echo` are rejected with `tool_dispatch_rejected` and do not execute;
- Go G5 computes this matrix from TS-owned fixtures without becoming a live
  planner backend.

Reject:

- Reasonix SessionAPI, planner/task public protocol, public auto-plan config,
  project/local overrides, or Reasonix route names;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  navigation/routes;
- renderer-visible Go routes, default Go backend, Electron-to-Go planner
  integration, Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings
  fallback;
- treating this shadow replay as packaged Plan QA, Go G5/G6 parity, or release
  readiness.

Rationale:

This captures the safety value of planner enable/disable and blocked-tool
rejection while keeping all public behavior inside analytix Plan mode and
runtime contracts.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test -count=1 ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0152 - Desktop Bridge And Settings Reject Reasonix Public Shapes

Date: 2026-06-22
Sources: Reasonix public protocol/config review, analytix preload bridge,
settings store tests, Go G5 control executable shadow
Domain: desktop bridge, settings schema, Go G5, product boundary
Status: accepted

Conflict:

Reasonix public protocol/config ideas are useful as negative test cases, but
analytix must not expose Reasonix bridge aliases, SessionAPI/config roots, or
deprecated settings fallback writes. At the same time, future Go work needs a
typed gate proving the desktop boundary remains owned by analytix.

Decision:

Accept only analytix-owned bridge/settings proof:

- preload exposes exactly `window.analytix`;
- renderer `Window` type declares only `analytix`;
- shared public facade domains remain analytix-owned and exclude `kun` /
  `reasonix` aliases;
- Reasonix `autoPlan` / `auto_plan` shapes are dropped on settings re-save;
- legacy `agentProvider` / `agents` envelopes are dropped on settings re-save;
- endpoint-format persistence remains under top-level `runtime` and
  `provider.providers`;
- Go G5 computes these proof ids and booleans from fixture inputs without
  becoming a desktop backend.

Reject:

- Reasonix SessionAPI, config roots, auto-plan settings, bridge aliases, or
  deprecated settings fallback write paths;
- `window.kun`, `window.reasonix`, `window.analytixGui`, or equivalent public
  facades;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  navigation/routes;
- renderer-visible Go routes, default Go backend, Electron-to-Go desktop
  integration, Rust/Tauri rewrite, Kun identity, or treating this as packaged
  desktop settings QA / G6 readiness / release readiness.

Rationale:

This gives future Go/runtime work a hard desktop sovereignty gate while keeping
all public bridge and settings contracts in analytix-owned shapes.

Validation:

Focused `go-runtime-conformance.test.ts`, `preload-sandbox.test.ts`,
`settings-store.test.ts`, and `packages/runtime-go go test -count=1 ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0126 - Approval/User-Input Route Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix approval/user-input lifecycle review, analytix
approval-user-input route oracle, Go G5 executable control shadow
Domain: Go G5, approval gate, user-input gate, SSE replay, backend boundary
Status: accepted

Conflict:

Approval/user-input route replay is useful evidence for future Go G5 parity,
but the same evidence could be misread as a live Go approval/user-input manager
or as permission to expose Reasonix ask/session protocols.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.approvalUserInputRouteReplay` is fixture-owned and
  derived from the TS approval/user-input route oracle;
- Go shadow computes approval deny route body/status, submitted and cancelled
  user-input routes, replay order, late action rejection, and abort cleanup;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real approval,
  user-input, and SSE replay behavior.

Reject:

- implementing a live Go approval/user-input manager from this evidence;
- exposing Go approval/user-input HTTP routes, renderer-visible Go routes, or
  default Go backend behavior;
- Reasonix SessionAPI, public ask protocol, or upstream route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 runtime parity, G6 readiness,
  packaged desktop QA, or release readiness.

Rationale:

This raises the quality of the G5 gate for approval/user-input and SSE replay
without moving product authority out of the analytix TypeScript runtime.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0133 - Task-Job Tool Boundary Stays Internal

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, Kun baseline review,
analytix task-job oracle
Domain: task jobs, sub-agent orchestration, Go G5, route surface
Status: accepted

Conflict:

Reasonix-style task/sub-agent orchestration is useful, but task and
parallel_tasks must remain internal runtime tools and must not become a public
Reasonix job protocol or top-level product surface.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.toolContractBoundary` is fixture-owned and
  derived from the TS task-job oracle;
- Go shadow computes internal-runtime-only flags, permission/evidence/
  dependency/read-only requirements, runtime task-job routes, protected-route
  auth, unauthorized status, forbidden top-level routes, no Reasonix protocol,
  and no top-level route exposure;
- Go tests compare the executable output to the TS-owned expected oracle;
- real task-job authority remains with the TypeScript runtime and analytix
  HTTP/SSE contracts.

Rationale:

This raises the quality of the G5 gate for sub-agent/job orchestration without
exposing Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer as top-level
entries.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0135 - Nested Child SSE Metadata Stays Analytix-Owned

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, Kun baseline review,
analytix task-job oracle
Domain: task jobs, child event projection, evidence ledger, Go G5
Status: accepted

Conflict:

Reasonix-style child runs need parent/child event metadata for orchestration.
Analytix needs the attribution value, but it must preserve analytix timeline
projection and evidence-ledger contracts without exposing Reasonix SessionAPI.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.nestedSseMetadata` is fixture-owned and
  derived from the TS task-job oracle;
- Go shadow computes parent call id, child run id, metadata field coverage,
  evidence ledger keys, parent/child distinctness, active-goal requirement, no
  Reasonix protocol, and no top-level route exposure;
- Go tests compare the executable output to the TS-owned expected oracle;
- real child event projection authority remains with the TypeScript runtime and
  analytix HTTP/SSE contracts.

Rationale:

This raises the quality of the G5 gate for sub-agent/job orchestration without
changing analytix timeline/projection ownership or exposing a public upstream
protocol.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0134 - Transcript Identity Stays Analytix-Owned

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, Kun baseline review,
analytix task-job oracle
Domain: task jobs, transcript identity, fork/continue semantics, Go G5
Status: accepted

Conflict:

Reasonix-style sub-agent orchestration relies on transcript continuation and
forking semantics. Analytix needs the capability, but it must preserve existing
thread/fork ownership semantics and must not expose Reasonix SessionAPI.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.transcriptIdentity` is fixture-owned and
  derived from the TS task-job oracle;
- Go shadow computes source id, continue target id, fork target id,
  incompatible identity error, same-transcript identity requirement,
  continue-target-matches-source, fork-target-distinct-from-source, no Reasonix
  protocol, and no top-level route exposure;
- Go tests compare the executable output to the TS-owned expected oracle;
- real transcript, resume, and fork authority remains with the TypeScript
  runtime and analytix HTTP/SSE contracts.

Rationale:

This raises the quality of the G5 gate for sub-agent/job orchestration without
changing analytix thread/fork semantics or exposing a public upstream protocol.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0132 - Product Boundary Gate Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix Go/runtime boundary review, Kun baseline review, analytix G5
full-loop oracle
Domain: product sovereignty, Go G5, runtime boundary, route surface
Status: accepted

Conflict:

Global product-boundary evidence should be executable so future Go absorption
cannot accidentally expose Reasonix protocol or make Go default. That proof
must not itself become a renderer-visible Go route or backend switch.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.productBoundary` is fixture-owned and derived from
  the TS G5 oracle;
- Go shadow computes bridge/serve stability, Reasonix protocol disabled,
  default Go backend disabled, renderer-visible Go routes disabled, Electron
  main disconnected, and Go backend not enabled;
- Go tests compare the executable output to the TS-owned expected oracle;
- real product authority remains with `window.analytix`, top-level runtime
  settings, `analytix serve`, and Renderer -> preload -> main -> runtime
  HTTP/SSE.

Rationale:

This raises the quality of the G5 gate for product sovereignty while keeping Go
shadow-only until explicit G5/G6 acceptance.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0131 - MCP Background Reconnect Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP lifecycle/reconnect review, analytix MCP lifecycle
oracle, Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, reconnect/retry, backend boundary
Status: accepted

Conflict:

MCP background reconnect evidence is useful for future Go parity, but it must
not be mistaken for a live Go MCP client, a default backend switch, or a
Reasonix MCP-indexer product protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpBackgroundReconnect` is fixture-owned and derived
  from the TS MCP lifecycle oracle;
- Go shadow computes failed server ids, suspended provider/reason,
  connected/error server outcomes, attempts per failed server, retry-all-failed
  coverage, and no-runtime-restart state;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP reconnect,
  catalog, approval, and tool execution behavior.

Rationale:

This raises the quality of the G5 gate for MCP lifecycle/reconnect behavior
without moving product authority out of the analytix TypeScript runtime.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0136 - MCP Known Override Diagnostics Stay Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP lifecycle/config override review, analytix MCP lifecycle
oracle, Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, known override diagnostics, backend boundary
Status: accepted

Conflict:

MCP known override diagnostics are useful for future Go parity, especially
`codegraph` and `codebase-memory` cwd/priority/background-start behavior. They
must not be mistaken for a live Go MCP client, a default backend switch, or a
Reasonix MCP-indexer product protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpKnownOverrideDiagnostics` is fixture-owned and
  derived from the TS MCP lifecycle oracle;
- Go shadow computes known override rows, variant count, override kinds,
  workspace roots, explicit-cwd server ids, daemon-timeout server ids,
  all-low-priority/all-background-start booleans, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP config,
  catalog, approval, reconnect, and tool execution behavior.

Reject:

- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- implementing a live Go MCP client, renderer-visible Go route, or default Go
  backend from this evidence;
- treating known override diagnostics as credentialed MCP matrix, packaged MCP
  QA, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 gate for MCP config/diagnostic behavior
without moving product authority out of the analytix TypeScript runtime.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0137 - MCP Live-Local Indexer Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle,
Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, live-local indexer, backend boundary
Status: accepted

Conflict:

MCP live-local indexer lifecycle evidence is useful for future Go parity,
especially retry, tombstone, restart/resume, active path, and redaction
behavior. It must not be mistaken for a live Go MCP client, a default backend
switch, or a Reasonix MCP-indexer product protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpLiveLocalIndexer` is fixture-owned and derived
  from the TS MCP lifecycle oracle;
- Go shadow computes retry server ids/map, initial/resume/active paths,
  tombstone count, restart status, late tombstone, secret-safe diagnostic,
  execution-error redaction, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP config,
  catalog, approval, reconnect, indexing, and tool execution behavior.

Reject:

- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- implementing a live Go MCP client, renderer-visible Go route, or default Go
  backend from this evidence;
- treating live-local indexer replay as credentialed MCP matrix, packaged MCP
  QA, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 gate for MCP/indexer lifecycle behavior
without moving product authority out of the analytix TypeScript runtime.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0138 - MCP Search Meta-Tool Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer currentness review, analytix MCP lifecycle
oracle, Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, search meta-tools, backend boundary
Status: accepted

Conflict:

MCP search meta-tool advertised/trust/no-execute evidence is useful for future
Go parity and Reasonix-style indexer discovery. It must not be mistaken for a
live Go MCP client, a default backend switch, or a Reasonix MCP-indexer product
protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpSearchMetaTools` is fixture-owned and derived
  from the TS MCP lifecycle oracle;
- Go shadow computes meta-tool names/count, refresh tool advertisement,
  trusted/untrusted workspace fields, trusted tool id, unknown-tool error,
  `on-request` call policy, denied no-execute, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP config,
  catalog, approval, search, indexing, and tool execution behavior.

Reject:

- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- implementing a live Go MCP client, renderer-visible Go route, or default Go
  backend from this evidence;
- treating search meta-tool replay as credentialed MCP matrix, packaged MCP QA,
  G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 gate for MCP search/indexer trust behavior
without moving product authority out of the analytix TypeScript runtime.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0139 - Provider Cache Privacy Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix provider/cache privacy review, analytix provider-cache
oracle, Go G5 executable control shadow
Domain: Go G5, provider cache, diagnostics privacy, backend boundary
Status: accepted

Conflict:

Provider cache diagnostics privacy and no-live-superiority policy are useful
for future Go parity and Reasonix-style provider/cache evidence. They must not
be mistaken for a live Go provider client, a default backend switch, or a live
provider/cache superiority claim.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.providerCachePrivacy` is fixture-owned and derived
  from the TS provider-cache oracle;
- Go shadow computes diagnostics field count, forbidden diagnostics substring
  count, no forbidden substring leakage, no live credential use, no live
  superiority claim, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real provider request
  URL/body construction, streaming, usage parsing, cache accounting, and
  diagnostics privacy.

Reject:

- exposing Reasonix provider public protocol or route names;
- implementing a live Go provider client, renderer-visible Go route, or
  default Go backend from this evidence;
- treating fixture-only diagnostics privacy as credentialed provider matrix,
  live cache superiority, packaged provider QA, G6 readiness, or release
  readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 provider/cache gate while preserving
analytix provider authority and preventing unsupported live superiority claims.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0140 - Provider Cache Inventory Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix provider/cache prefix currentness review, analytix
provider-cache oracle, Go G5 executable control shadow
Domain: Go G5, provider cache, stable prefix inventory, backend boundary
Status: accepted

Conflict:

Stable prefix hash, tools hash, and provider/request-shape case inventory are
useful for future Go parity and Reasonix-style cache currentness evidence. They
must not become a live Go provider client, a default backend switch, or a path
for dynamic workspace material to enter the stable cache prefix.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.providerCacheInventory` is fixture-owned and derived
  from the TS provider-cache oracle;
- Go shadow computes stable prefix hash, tools hash, usage/request-shape ids
  and counts, prefix equivalence, tools hash stability, and product-boundary
  flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real stable-prefix
  construction, provider request URL/body behavior, streaming, usage parsing,
  and cache accounting.

Reject:

- exposing Reasonix provider public protocol or route names;
- placing file snippets, timestamps, selected text, credentials, or Reasonix
  sidecar material into stable prefix;
- implementing a live Go provider client, renderer-visible Go route, or
  default Go backend from this evidence;
- treating fixture-only inventory as credentialed provider matrix, live cache
  superiority, packaged provider QA, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 provider/cache gate while keeping stable
prefix authority in analytix-owned TypeScript contracts.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0141 - Session Route Inventory Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix session/fork/SSE route review, analytix G2 route oracle,
Go G5 executable control shadow
Domain: Go G5, thread/session routes, SSE replay, backend boundary
Status: accepted

Conflict:

Thread/session route inventory is useful for future Go parity and for proving
that list/archive/search/read/update/fork/resume/SSE/auth surfaces are covered
by fixtures. It must not become a live Go route surface, a default backend
switch, or a Reasonix SessionAPI/public-route compatibility layer.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.sessionRouteInventory` is fixture-owned and derived
  from the TS G2 route oracle;
- Go shadow computes route ids, JSON/SSE/event/resume/fork/archive/search/
  read-update groups, runtime-token protected route count, unauthorized route
  ids, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real thread list,
  archive, search, read/update, fork, resume, auth, and SSE replay behavior.

Reject:

- exposing Reasonix SessionAPI, public route protocol, or route names;
- implementing live Go thread/session routes, renderer-visible Go routes, or a
  default Go backend from this evidence;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer routes;
- treating fixture-only inventory as packaged desktop route QA, G5 runtime
  parity, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 route gate while preserving analytix-owned
HTTP/SSE authority and keeping Go behind shadow-only conformance.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0142 - Approval/User-Input Inventory Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix approval/user-input lifecycle review, analytix
approval/user-input route oracle, Go G5 executable control shadow
Domain: Go G5, approval gate, user-input gate, answer privacy, backend boundary
Status: accepted

Conflict:

Approval/user-input gate inventory is useful for future Go parity and for
proving the approval decision, user-input submit/cancel, abort cleanup, replay
kind order, and answer privacy surfaces are covered by fixtures. It must not
become a live Go approval/user-input manager, a default backend switch, or a
Reasonix ask/session public protocol compatibility layer.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.approvalUserInputInventory` is fixture-owned and
  derived from the TS approval/user-input route oracle;
- Go shadow computes gate ids, approval ids, user-input ids, route kinds,
  replay kind order, abort replay kinds, answer count, HTTP answer echo versus
  resolved-event no-answer privacy, late statuses, pending-after sum, and
  product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real approvals,
  user-input prompts, pending gate cleanup, late action rejection, and SSE
  replay privacy.

Reject:

- exposing Reasonix ask/session public protocol or route names;
- implementing live Go approval/user-input managers, renderer-visible Go
  routes, or a default Go backend from this evidence;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer routes;
- treating fixture-only inventory as packaged approval-card QA, G5 runtime
  parity, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 approval/user-input gate while preserving
analytix-owned gate/event authority and keeping Go behind shadow-only
conformance.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0143 - Task Planner Toolset Inventory Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix planner/sub-agent job orchestration review, analytix
task-job oracle, Go G5 executable control shadow
Domain: Go G5, planner gating, task jobs, sub-agent orchestration, backend boundary
Status: accepted

Conflict:

Planner read-only and forbidden task toolset inventory is useful for future Go
parity and for proving that planner waves cannot directly schedule `task` or
`parallel_tasks`. It must not become a live Go planner/executor, a default
backend switch, or a Reasonix public sub-agent/job/planner protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.plannerToolsetInventory` is fixture-owned
  and derived from the TS task-job oracle;
- Go shadow computes read-only tools, forbidden task tools, tool counts,
  read-only exclusion of task tools, forbidden match against `task` /
  `parallel_tasks`, planner/executor policy, and product-boundary flags;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real planner gating,
  task/parallel task availability, sub-agent execution, and permission policy.

Reject:

- exposing Reasonix public sub-agent/job/planner protocol or route names;
- implementing a live Go planner/executor, Go Job Manager, renderer-visible Go
  routes, or a default Go backend from this evidence;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer routes;
- treating fixture-only inventory as packaged sub-agent QA, G5 runtime parity,
  G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 planner/task-job gate while preserving
analytix-owned planner/tool authority and keeping Go behind shadow-only
conformance.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0145 - Provider Request-Shape Exact Matrix Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix provider/cache request-shape review, analytix provider-cache
oracle, Go G3/G5 shadow conformance
Domain: Provider/cache request shape, Go G3/G5, URL/header/body contract,
backend boundary
Status: accepted

Conflict:

Provider request-shape proof must cover exact URL/header/body/tool-shape
differences across DeepSeek, OpenAI-compatible chat, Responses, Anthropic
Messages, and custom full endpoints. It must not become a Reasonix provider
protocol, live Go provider client, or default backend switch.

Decision:

Accept only pure executable shadow control:

- `provider-cache-oracle.json.requestShapeCases` remains the TS-owned source;
- G3/G5 shadow outputs carry the exact matrix, not only counts and endpoint
  format summaries;
- Go shadow clones and compares exact URL, required/forbidden headers,
  required/forbidden body fields, reasoning-effort presence, and tool-shape
  family;
- Go fixture validation uses `go test -count=1 ./...` when JSON oracle changes
  are involved so cached Go test results cannot mask fixture drift;
- TypeScript provider/runtime contracts remain authoritative for live requests.

Reject:

- exposing Reasonix provider protocol or route names;
- implementing a live Go provider client, renderer-visible Go routes, or a
  default Go backend from this evidence;
- adding deprecated provider/settings fallback paths;
- claiming credentialed provider matrix parity or live superiority from
  fixture-only evidence;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes, Rust/Tauri rewrite, or Kun identity.

Rationale:

This raises provider/cache request-shape evidence from coarse summary to exact
matrix replay while preserving analytix-owned provider settings, URL/body
contracts, and runtime boundary.

Validation:

Focused G3/G5 conformance tests and `packages/runtime-go go test -count=1 ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0146 - Combined Step/Cancel/Cache Trace Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix agent-kernel step/cancel/cache review, analytix G5 full-loop
oracle, Go G5 executable control shadow
Domain: Agent loop controls, auto-route cache, step limit, cancel, backend boundary
Status: accepted

Conflict:

Combined step/cancel/cache evidence is useful only if it can explain which part
of the composition changed. Summary booleans are too weak, but exact trace must
not become a live Go loop, Reasonix controller/session protocol, or public
auto-plan product surface.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.combined.expected` records router calls, model steps,
  stable-prefix flags, cancel result counts, and exact result rows;
- Go shadow computes the trace from fixture inputs and replays cancel result
  pairing through the same helper used by cancel control;
- TypeScript loop/cache/cancel contracts remain authoritative for live runtime
  behavior;
- product-boundary scans remain responsible for preventing hidden
  Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries.

Reject:

- exposing Reasonix controller/session protocol or route names;
- implementing a live Go agent loop, renderer-visible Go routes, or a default
  Go backend from this evidence;
- adding public auto-plan settings or top-level Workflow/Create Loop/Subagent/
  AutoResearch routes;
- treating fixture-only trace as packaged long-running cancel/cache QA, G5
  runtime parity, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises combined cancel/step/cache evidence from coarse pass/fail booleans
to an inspectable trace while preserving analytix-owned loop/cache authority.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0148 - History Repair Pair Integrity Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix model-history/tool-result legality review, analytix
`repairModelHistoryItems`, Go G5 executable control shadow
Domain: Model history repair, tool-call/result pairing, cache stability,
backend boundary
Status: accepted

Conflict:

History repair evidence must prove provider-legal tool-call/result pairing
without turning Go into the live history manager, exposing Reasonix protocol,
or adding dynamic repair state to the stable prefix.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.historyRepair` records simplified model-history
  items and expected repaired/dropped ids;
- TS conformance derives expected output from `repairModelHistoryItems`;
- Go shadow computes complete tool-call blocks, result blocks, bridge items,
  orphan result drops, missing-result call drops, and duplicate result drops;
- TypeScript runtime history repair remains authoritative for live runtime;
- stable-prefix/cache state remains free of repair bookkeeping.

Reject:

- implementing a live Go history manager or default Go backend from this
  evidence;
- exposing Reasonix SessionAPI, controller protocol, public history protocol,
  renderer-visible Go routes, or route names;
- adding public auto-plan settings or top-level Workflow/Create Loop/Subagent/
  AutoResearch routes;
- storing dynamic repair state in immutable prefix/provider cache keys;
- treating fixture-only repair as packaged long-history QA, G5 runtime parity,
  G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises model-history legality evidence from TS-only unit proof into an
inspectable Go G5 executable shadow while preserving analytix-owned runtime
authority and cache-prefix stability.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0147 - Step-Limit Override/Delegate Matrix Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix agent-kernel step-limit/delegation review, analytix G5
full-loop oracle, Go G5 executable control shadow
Domain: Agent loop controls, step limit, planner delegation, backend boundary
Status: accepted

Conflict:

Step-limit evidence must prove override precedence and delegate inheritance
without exposing Reasonix controller/session protocol or making Go a live
agent-loop backend.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.stepLimits.expected.matrix` records default,
  user-global, session, turn, planner, headless, zero-default, delegate, and
  delegate-floor rows;
- Go shadow computes configured/fallback/effective/source plus stable-prefix,
  disable-guard, and delegate-floor flags from fixture inputs;
- TypeScript step-limit behavior remains authoritative for live runtime;
- Go fixture validation uses non-cached runs when JSON oracle evidence changes.

Reject:

- exposing Reasonix controller/session protocol or route names;
- implementing a live Go loop, renderer-visible Go routes, or a default Go
  backend from this evidence;
- adding public auto-plan settings or top-level Workflow/Create Loop/Subagent/
  AutoResearch routes;
- treating fixture-only matrix as packaged step-limit QA, G5 runtime parity,
  G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises step-limit evidence from scalar fields to an inspectable
override/delegate matrix while preserving analytix-owned loop authority.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
-count=1 ./...` pass. Full final gates and forbidden-surface scans are recorded
in release evidence.

## D-0144 - MCP Core Lifecycle Boundary Seal Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle,
Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, provider identity, product boundary, backend boundary
Status: accepted

Conflict:

MCP lifecycle parity needs explicit provider identity and product-boundary
assertions so useful Reasonix lifecycle behavior can be absorbed without
exposing Reasonix MCP-indexer protocol or a top-level MCP-indexer route.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpCoreLifecycle` is fixture-owned and derived from
  the TS MCP lifecycle oracle;
- Go shadow computes `providerId: "mcp:research"`, `usesReasonixProtocol:
  false`, and `topLevelRouteExposed: false` alongside the existing lifecycle
  output;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP lifecycle,
  tool catalog, cancellation, approval/error handling, and permission policy.

Reject:

- exposing Reasonix MCP-indexer public protocol or route names;
- implementing a live Go MCP client, renderer-visible Go routes, or a default
  Go backend from this evidence;
- adding top-level MCP-indexer, Workflow, Create Loop, Subagent, or
  AutoResearch routes;
- treating fixture-only provider/boundary seal as credentialed MCP matrix,
  packaged MCP QA, G5 runtime parity, G6 readiness, or release readiness;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

This raises the quality of the G5 MCP lifecycle gate while preserving
analytix-owned MCP/tool authority and keeping Go behind shadow-only
conformance.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0130 - MCP Core Lifecycle Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP lifecycle review, analytix MCP lifecycle oracle, Go G5
executable control shadow
Domain: Go G5, MCP lifecycle, tool catalog, cancel/error, backend boundary
Status: accepted

Conflict:

MCP connect/disconnect/reload/cancel/error lifecycle evidence is useful for
future Go parity, but it must not be mistaken for a live Go MCP client or a
Reasonix MCP-indexer product protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpCoreLifecycle` is fixture-owned and derived from
  the TS MCP lifecycle oracle;
- Go shadow computes connect/disconnect diagnostics, reload tool names and
  schema-order stability, cancel-before-start no-execute, and approved error
  shape;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP catalog,
  lifecycle, approval, and tool execution behavior.

Reject:

- implementing a live Go MCP client or public MCP-indexer from this evidence;
- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- renderer-visible Go routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 runtime parity, G6 readiness,
  credentialed MCP matrix, packaged desktop MCP QA, or release readiness.

Rationale:

This improves the MCP lifecycle gate while keeping live MCP execution in the
analytix TypeScript runtime and preserving product sovereignty.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0129 - MCP Search Workspace Boundary Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle,
Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, workspace trust, product boundary
Status: accepted

Conflict:

MCP search workspace boundaries are useful evidence for trusted/untrusted
tool discovery, but they can be mistaken for a public MCP-indexer route or a
live Go MCP client claim.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpSearchWorkspaceBoundary` is fixture-owned and
  derived from the TS MCP lifecycle oracle;
- Go shadow computes trusted workspace, untrusted workspace, query, trusted
  tool id, untrusted searched-tools count, unknown-tool error, call policy, and
  denied no-execute state;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP search,
  catalog, approval, and tool execution behavior.

Reject:

- implementing a live Go MCP client or public MCP-indexer from this evidence;
- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- renderer-visible Go routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 runtime parity, G6 readiness,
  credentialed MCP matrix, packaged desktop MCP QA, or release readiness.

Rationale:

This improves the MCP workspace trust gate while keeping search/catalog
behavior an internal analytix conformance proof.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0128 - MCP Approval Annotation Replay Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/tool approval lifecycle review, analytix MCP lifecycle
oracle, Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, tool approval, no-execute, backend boundary
Status: accepted

Conflict:

MCP approval annotations are useful evidence for destructive/open-world tool
gating, but they can be mistaken for a live Go approval manager or a public
Reasonix MCP/approval protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpApprovalAnnotations` is fixture-owned and derived
  from the TS MCP lifecycle oracle;
- Go shadow computes destructive/open-world hints, normalized MCP tool name,
  approval id, deny decision, result kind, execution flag, and denied
  no-execute state;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP approval and
  tool execution behavior.

Reject:

- implementing a live Go MCP client or live Go approval manager from this
  evidence;
- exposing Reasonix MCP-indexer/approval public protocol, route names, or
  top-level navigation;
- renderer-visible Go routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 runtime parity, G6 readiness,
  credentialed MCP matrix, packaged desktop MCP/approval QA, or release
  readiness.

Rationale:

This improves the MCP/tool approval gate while keeping execution authority in
the analytix TypeScript runtime and preserving product sovereignty.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0127 - MCP Search Refresh Drift Stays Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle,
Go G5 executable control shadow
Domain: Go G5, MCP lifecycle, catalog refresh, product boundary
Status: accepted

Conflict:

MCP catalog refresh drift is useful evidence for future Go MCP parity, but it
can be mistaken for a Reasonix MCP-indexer product surface or a live Go MCP
client claim.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.mcpSearchRefreshDrift` is fixture-owned and derived
  from the TS MCP lifecycle oracle;
- Go shadow computes server id, initial tool names, expanded tool names,
  indexed count, catalog drift, and `topLevelRouteExposed:false`;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real MCP lifecycle,
  tool catalog, and approval behavior.

Reject:

- implementing a live Go MCP client or Go MCP route from this evidence;
- exposing Reasonix MCP-indexer public protocol, route names, or top-level
  navigation;
- renderer-visible Go routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 runtime parity, G6 readiness,
  credentialed MCP matrix, packaged desktop QA, or release readiness.

Rationale:

This improves the MCP lifecycle gate while keeping catalog refresh an internal
analytix conformance proof, not a user-visible product surface.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0110 - MCP Refresh Drift Remains Internal Search Metadata

Date: 2026-06-22
Sources: Reasonix MCP/indexer currentness review, analytix MCP lifecycle oracle,
Go G5 shadow conformance
Domain: MCP search metadata, Go G5, product boundary
Status: accepted

Conflict:

Refreshing MCP search catalogs is useful currentness behavior, but Reasonix
MCP-indexer lifecycle concepts must not become an analytix public protocol,
renderer route, or top-level product entry. Go may replay the result only as
shadow evidence.

Decision:

Accept only analytix-owned internal MCP search semantics:

- `mcp_refresh_catalog` can prove catalog drift after fake-client expansion;
- the TS oracle owns initial/expanded tool names and expected indexed count;
- G5 `mcpReplay.searchRefreshDrift` is a summary field, not a Go MCP client;
- `topLevelRouteExposed` must remain false.

Reject:

- Reasonix MCP-indexer public lifecycle protocol, route, or navigation;
- live Go MCP client, Go MCP route, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- credentialed MCP matrix, packaged desktop MCP QA, release readiness, Kun
  identity, deprecated bridge/settings fallback, or Rust/Tauri rewrite.

Rationale:

Catalog refresh currentness is absorbed as a runtime/tool invariant while the
product remains an analytix desktop app with TS runtime authority and G5/G6
Go gates.

Validation:

Focused `mcp-tool-lifecycle-oracle.test.ts`, `go-runtime-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0111 - Thread/SSE Routes Require Analytix Runtime Token

Date: 2026-06-22
Sources: Reasonix session/fork/SSE route review, analytix G2 route replay
oracle, Go G5 shadow conformance
Domain: runtime HTTP/SSE auth, thread/session routes, Go shadow boundary
Status: accepted

Conflict:

Route replay evidence for list/search/archive/fork/resume/SSE is useful only
if it also proves those routes stay behind the analytix runtime token. Missing
auth must not fall through to SSE replay or Reasonix-style public SessionAPI
behavior.

Decision:

Accept only analytix-owned HTTP/SSE authorization semantics:

- the G2 fixture includes a missing-token `/v1/threads/:id/events` route;
- TypeScript conformance dispatches that route without a bearer token and
  requires 401;
- G5 replay records the protected route id/path/status/body code and zero SSE
  frames;
- Go shadow may replay the summary but does not serve Electron or renderer
  traffic.

Reject:

- Reasonix SessionAPI/public thread protocol;
- unauthenticated thread/SSE replay;
- renderer-visible Go route, live Go HTTP server, or default Go backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite,
  packaged desktop QA, or release readiness claims.

Rationale:

This keeps thread/session lifecycle absorption behind `analytix serve` and the
runtime-token contract while strengthening future Go route parity gates.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0112 - Active Runtime Settings Reject Legacy Agent Shapes

Date: 2026-06-22
Sources: Reasonix auto-plan/config review, analytix runtime settings schema
Domain: settings, product identity, bridge boundary
Status: accepted

Conflict:

Reasonix agent config/currentness ideas are useful, but active analytix saves
must not persist Reasonix/Kun-shaped agent settings under `runtime` or revive
old agent settings envelopes.

Decision:

Accept only analytix-owned runtime settings:

- runtime normalization strips `agent`, `agentProvider`, `agents`, `deepseek`,
  and `reasonix` when those fields appear under `runtime`;
- IPC settings patches reject legacy/Reasonix agent-shaped runtime keys;
- explicit migration/import code may still read legacy shapes before rewriting
  them into current contracts.

Reject:

- Reasonix project/local config roots or public settings protocol;
- Kun/DeepSeek legacy agent settings envelopes as active save paths;
- deprecated bridge aliases or settings fallbacks;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, packaged
  settings QA, or release readiness claims.

Rationale:

This closes a small but important persistence gap: analytix can absorb planning
and provider ideas without letting upstream settings identities leak into the
active `runtime` schema.

Validation:

Focused `app-settings.test.ts` and `app-ipc-schemas.test.ts` pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0113 - Task-Job Route Control Shadow Is Not Public Job API

Date: 2026-06-22
Sources: Reasonix task/job route review, analytix task-job route oracle, Go G5
shadow conformance
Domain: Go runtime, task/job orchestration, product boundary
Status: accepted

Conflict:

Reasonix-style task/job route behavior is useful for runtime correctness, but
analytix must not expose public Reasonix job/session protocols or make Go the
default backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.taskJobs.routeExecutable` mirrors the TS-owned
  `task-job-orchestration-oracle.routeExecutable`;
- Go computes typed unauthorized/output/wait/kill/missing/rehydrated route
  outputs through `BuildG5ControlExecutableOutput`;
- the replay carries `usesReasonixProtocol: false` and
  `topLevelRouteExposed: false`.

Reject:

- Reasonix public SessionAPI/job protocol;
- renderer-visible Go task-job routes;
- default Go backend or live Go Job Manager readiness claims;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens Go G5 executable shadow without changing the active
Electron -> `analytix serve` runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0114 - Task-Job Lifecycle Control Shadow Is Not Public Lifecycle API

Date: 2026-06-22
Sources: Reasonix task/job lifecycle review, analytix task-job oracle, Go G5
shadow conformance
Domain: Go runtime, task/job orchestration, product boundary
Status: accepted

Conflict:

Reasonix-style task/job lifecycle behavior is useful for runtime correctness,
but analytix must not expose public Reasonix lifecycle/session protocols or make
Go the default backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.taskJobs.lifecycle` mirrors the TS-owned foreground,
  background, and wait/output/kill lifecycle oracle;
- Go computes typed lifecycle output through `BuildG5ControlExecutableOutput`;
- the replay carries `usesReasonixProtocol: false` and
  `topLevelRouteExposed: false`.

Reject:

- Reasonix public SessionAPI/lifecycle protocol;
- renderer-visible Go task-job routes;
- default Go backend or live Go Job Manager readiness claims;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens Go G5 executable shadow without changing the active
Electron -> `analytix serve` runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0115 - Planner-Executor Control Shadow Is Not Public Planner API

Date: 2026-06-22
Sources: Reasonix planner/executor task orchestration review, analytix task-job
oracle, Go G5 shadow conformance
Domain: Go runtime, planner/executor orchestration, product boundary
Status: accepted

Conflict:

Reasonix-style planner/executor propagation behavior is useful for runtime
correctness, but analytix must not expose public Reasonix planner/session/job
protocols or make Go the default backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.taskJobs.plannerExecutor` mirrors the TS-owned
  planner/executor task-job oracle;
- Go computes typed failure/cancellation/output-offset/transcript-propagation
  output through `BuildG5ControlExecutableOutput`;
- the replay carries `usesReasonixProtocol: false` and
  `topLevelRouteExposed: false`.

Reject:

- Reasonix public SessionAPI/planner/job protocol;
- renderer-visible Go task-job routes;
- default Go backend or live Go Job Manager readiness claims;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens Go G5 executable shadow without changing the active
Electron -> `analytix serve` runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0116 - Provider Cache Release Guard Control Shadow Is Not Live Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider cache accounting, Go runtime, product boundary
Status: accepted

Conflict:

Reasonix-style provider/cache release-guard evidence is useful for showing
cache-hit currentness and prefix-stability behavior, but analytix must not turn
fixture-only cache curves into live provider superiority claims or make Go a
provider backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerCacheReleaseGuard` mirrors the TS-owned
  provider/cache release guard;
- Go computes typed tail averages, allowed-low case count, collapse count, and
  pass/fail status through `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider/cache evidence and Go G5 executable shadow while
preserving `analytix serve` as the active TypeScript runtime boundary.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0117 - Provider Usage Parser Control Shadow Is Not Live Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider usage accounting, Go runtime, product boundary
Status: accepted

Conflict:

Reasonix-style provider usage parser precedence is useful for cache accounting,
but analytix must not turn fixture-only DeepSeek/OpenAI Responses/Anthropic
usage payloads into live provider superiority claims or make Go a provider
backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerUsageParser` mirrors the TS-owned
  provider/cache usage parser oracle;
- Go computes typed DeepSeek native cache precedence, OpenAI Responses cached
  tokens, Anthropic cache fields, and unsupported absent-field output through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider/cache accounting evidence and Go G5 executable
shadow while preserving `analytix serve` as the active TypeScript runtime
boundary.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0118 - Provider Request Shape Control Shadow Is Not Live Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider URL/body/tool-shape contract, Go runtime, product boundary
Status: accepted

Conflict:

Reasonix-style provider request-shape evidence is useful for preventing URL,
body, header, and tool-shape regressions, but analytix must not turn fixture
request cases into live provider parity claims or make Go a provider backend
before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerRequestShape` mirrors the TS-owned
  provider/cache request-shape oracle;
- Go computes typed exact URL count, endpoint families, full endpoint ids,
  tool-shape families, and required/forbidden body-field counts through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider request URL/body evidence and Go G5 executable
shadow while preserving `analytix serve` as the active TypeScript runtime
boundary.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0119 - Provider Cache Accounting Control Shadow Is Not Live Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider cache accounting, Go runtime, product boundary
Status: accepted

Conflict:

Provider cache accounting evidence is useful for proving DeepSeek/OpenAI
Responses/Anthropic cache telemetry handling and unsupported fallback, but
analytix must not turn fixture accounting into live provider superiority claims
or make Go a provider backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerCacheAccounting` mirrors a minimal
  provider/cache accounting projection from the TS-owned oracle;
- Go computes typed supported telemetry case ids, unsupported unknown case ids,
  provider-family case ids, total hit/miss tokens, aggregate hit rate, and the
  no-unsupported-miss invariant through `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider/cache accounting evidence and Go G5 executable
shadow while preserving `analytix serve` as the active TypeScript runtime
boundary.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0120 - Release/Package Identity Must Stay Analytix

Date: 2026-06-22
Sources: Kun/Reasonix product-sovereignty review, packaging configuration,
product-sovereignty scan
Domain: product identity, packaging, release configuration
Status: accepted

Conflict:

Analytix can absorb runtime techniques from Kun/Reasonix, but package names,
app ids, artifact names, release scripts, workflows, and runtime public CLI
must remain analytix-owned. Source-only bridge/settings scans are not enough to
catch release/package identity regressions.

Decision:

Accept source/config guard coverage:

- `scan:product-sovereignty` scans package manifests, Electron builder config,
  scripts, workflows, and build configuration for Kun/Reasonix identity leaks;
- `packaging-config.test.ts` asserts `analytix` root package/product name,
  `com.analytix.desktop` app id, `analytix-*` artifact template, `analytix`
  NSIS shortcut/uninstall names, and `analytix` runtime CLI bin.

Reject:

- Kun/Reasonix public package identity, app id, artifact name, shortcut name,
  runtime bin, release workflow identity, or public protocol;
- deprecated bridge/settings fallback;
- default Go backend or renderer-visible Go route;
- Rust/Tauri rewrite;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries.

Rationale:

This closes a product-sovereignty gap without changing runtime behavior or
packaging functionality.

Validation:

Focused `npm run scan:product-sovereignty` and
`npm run test -- src/main/packaging-config.test.ts --no-file-parallelism --maxWorkers=1`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0121 - Provider Streaming Control Shadow Is Not Live Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix G3 provider streaming
oracle, Go G5 shadow conformance
Domain: provider streaming usage, Go runtime, product boundary
Status: accepted

Conflict:

Provider streaming usage evidence is useful for proving SSE usage/cache events,
but analytix must not turn fixture streaming frames into live provider
superiority claims or make Go a provider backend before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerStreaming` mirrors minimal inputs from the
  TS-owned G3 provider streaming oracle;
- Go computes typed event order, usage event match status, token/cache fields,
  cache hit rate, telemetry support, and product boundary through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider streaming
  superiority claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider streaming usage evidence and Go G5 executable shadow
while preserving `analytix serve` as the active TypeScript runtime boundary.

Validation:

Focused `go-runtime-g3-g4-conformance.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0122 - Provider Offline Parity Seal Control Shadow Is Not Live Provider Superiority

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider cache telemetry, Go runtime, product boundary
Status: accepted

Conflict:

Offline provider parity evidence is useful for proving stable prefix/cache
telemetry behavior, but analytix must not turn fixture-only cache proof into
live provider superiority claims or make Go a provider backend before G5/G6
gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerOfflineParitySeal` mirrors minimal inputs
  from the TS-owned provider-cache oracle;
- Go computes stable prefix equivalence, prefix-items/tools hash stability,
  DeepSeek cache telemetry, request-shape coverage, release guard status, and
  fixture-only/live-superiority policy through `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider/cache parity evidence and Go G5 executable shadow
while preserving `analytix serve` as the active TypeScript runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0125 - Provider Drift Attribution Control Shadow Is Not Live Cache Superiority

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider cache drift attribution, Go runtime, product boundary
Status: accepted

Conflict:

Provider drift attribution is useful for proving stable prefix/currentness
boundaries, but analytix must not turn fixture-only attribution proof into
live cache superiority claims or make Go a provider backend before G5/G6 gates
pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerDriftAttribution` mirrors the TS-owned
  provider-cache oracle drift inputs;
- Go computes previous/current prefix hashes, stable system/prefix-items
  hashes, tools/provider/model/endpoint-format changes, expected reasons,
  telemetry support, and cache-hit-rate known status through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom cache superiority claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider cache currentness/drift evidence and Go G5
executable shadow while preserving `analytix serve` as the active TypeScript
runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0124 - Provider Live-Local HTTP Control Shadow Is Not Live Provider Superiority

Date: 2026-06-22
Sources: Reasonix provider/cache parity review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider request/usage local HTTP proof, Go runtime, product boundary
Status: accepted

Conflict:

Local HTTP provider proof is useful for proving URL/body family coverage, but
analytix must not turn fixture-only fake local HTTP proof into credentialed
provider superiority claims or make Go a provider backend before G5/G6 gates
pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.providerLiveLocalHttpProof` mirrors minimal inputs
  from the TS-owned provider-cache oracle;
- Go computes fixture policy, local HTTP transport, credential policy,
  usage/request-shape counts, expected POST count, endpoint formats, provider
  families, and no-live-superiority status through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not perform live network
  provider calls.

Reject:

- Reasonix public provider/cache protocol or settings shape;
- live Go provider client readiness or default Go backend behavior;
- credentialed live DeepSeek/OpenAI/Anthropic/custom provider superiority
  claims;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens provider request/usage coverage evidence and Go G5 executable
shadow while preserving `analytix serve` as the active TypeScript runtime
boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0123 - Session Route Status Control Shadow Is Not Live Go Routing

Date: 2026-06-22
Sources: Reasonix session/fork/SSE replay review, analytix G2 route oracle,
Go G5 shadow conformance
Domain: thread/session routes, SSE replay, Go runtime, product boundary
Status: accepted

Conflict:

Thread/session route status evidence is useful for proving archive/search/
read/update/fork/resume/SSE replay behavior, but analytix must not turn
fixture-only route proof into live Go HTTP routing or expose Reasonix SessionAPI
before G5/G6 gates pass.

Decision:

Accept only analytix-owned control-shadow replay:

- `controlExecutableCases.sessionRouteStatus` mirrors the TS-owned G2 route
  oracle;
- Go computes route/status counts, archive/search counts, read/update fields,
  fork lineage, session resume summary, SSE replay/caught-up frame counts,
  replay event names, and missing-token auth rejection through
  `BuildG5ControlExecutableOutput`;
- the control case remains fixture-only and does not register live Go routes.

Reject:

- Reasonix SessionAPI, public job/session protocol, or route names;
- live Go thread/session routes or default Go backend behavior;
- renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, packaged desktop QA, or release readiness claims.

Rationale:

This strengthens thread/session/SSE replay evidence and Go G5 executable
shadow while preserving `analytix serve` as the active TypeScript runtime
boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0051 - Cancelled Plan Follow-Up Steps Do Not Advance Cache Baseline

Status: accepted for analytix AgentLoop; Reasonix public planner protocol
rejected.

Decision:
analytix accepts the planner step/cancel/cache stability requirement as an
analytix-owned Plan mode contract:

- explicit `mode: "plan"` / `guiPlan` is the only public Plan mode entry;
- step 0 may advertise read-only tools plus `create_plan`;
- an unsatisfied follow-up Plan step advertises only `create_plan`;
- if that follow-up step is interrupted before provider usage arrives, it must
  not replace the previous cache prefix baseline;
- subsequent Plan turns with the original read-only-plus-plan tool surface must
  keep `cacheDiagnostics.prefixChanged: false` when provider/model/endpoint and
  stable prefix are unchanged.

Rejected:

```text
Reasonix auto-plan settings
Reasonix SessionAPI/controller protocol
public planner route
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry
default Go backend or renderer-visible Go route
Rust/Tauri migration
```

Evidence:

```text
packages/runtime/tests/loop.test.ts
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

## D-0052 - MCP Tool Results Must Redact Protocol-Level Error Payloads

Status: accepted for analytix MCP provider; Reasonix MCP-indexer public
protocol rejected.

Decision:
MCP server errors can arrive either as thrown client errors or as successful
MCP responses with `isError: true` and text content. Both forms are
model-visible through `tool_result.output`, so both must be redacted before
persistence and model feedback.

Accepted:

- redact nested MCP result payloads with the shared secret redactor;
- rethrow non-abort MCP call failures with redacted messages;
- prove the behavior through the executable stdio fake indexer;
- keep MCP lifecycle behind analytix tool contracts.

Rejected:

```text
Reasonix MCP-indexer public protocol
top-level MCP-indexer route/navigation
default Go backend or renderer-visible Go route
Rust/Tauri migration
Kun identity or deprecated bridge/settings fallback
```

Evidence:

```text
packages/runtime/src/adapters/tool/mcp-tool-provider.ts
packages/runtime/tests/mcp-tool-lifecycle-oracle.test.ts
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

## D-0053 - Dormant Workflow/Create Loop Code Must Stay Quarantined

Status: accepted as product-sovereignty guardrail.

Decision:
Dormant Workflow/Create Loop implementation files may remain only as isolated
internal code. They must not be imported by top-level app entry surfaces or
exposed through route actions, Workbench stages, Sidebar/Write/Plugin entry
surfaces, preload APIs, tray/menu shortcuts, settings shortcuts, or public
runtime protocol.

Rejected:

```text
Workflow/Create Loop top-level navigation
Subagent/AutoResearch/MCP-indexer top-level navigation
Reasonix public workflow protocol or SessionAPI
Kun identity or deprecated bridge/settings fallback
default Go backend or renderer-visible Go route
Rust/Tauri migration
```

Evidence:

```text
src/renderer/src/components/Workbench.route-surface.test.ts
scripts/scan-product-sovereignty.cjs
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## D-0048 - Parallel Task Dependency Execution Remains G5 Shadow-Only

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job oracle,
Go G5 executable control shadow
Domain: Go G5, task jobs, sub-agent/job orchestration, backend boundary
Status: accepted

Conflict:

Reasonix-style `parallel_tasks` dependency validation is useful Go-kernel
evidence, but executing the validator in Go must not become a live Go Job
Manager, renderer-visible route, or public sub-agent protocol.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.parallelValidation` is fixture-owned and
  derived from the TS task-job oracle;
- Go shadow computes valid DAG order and the five invalid dependency errors;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real task/job
  execution, approvals, user input, SSE replay, fork/resume/archive/search, and
  provider behavior.

Reject:

- implementing a live Go Job Manager or Go task-job routes from this evidence;
- exposing renderer-visible Go routes or enabling default Go backend behavior;
- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 parity, G6 readiness, packaged
  desktop QA, or release readiness.

Rationale:

This upgrades Go G5 gate quality while preserving analytix-owned contracts and
keeping `parallel_tasks` inside internal runtime/tool-host boundaries.

Validation:

Focused `task-job-orchestration-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0049 - Connect Phone Legacy Recognizers Are Compatibility-Only

Date: 2026-06-22
Sources: Kun Connect Phone lineage, product-sovereignty scan gate, renderer
route-surface tests
Domain: product identity, Connect Phone copy, route surface
Status: accepted

Conflict:

Historical Connect Phone sessions may still carry `[Claw:]` / `[Claw IM:]`
titles, but newly generated renderer placeholders must not reintroduce legacy
product copy or weaken route-surface scans.

Decision:

Accept:

- keep legacy `[Claw:]` / `[Claw IM:]` recognizers for historical recovery;
- generate new mapped-conversation placeholders as `[Connect Phone:...]`;
- include Write sidebar, workspace mode tabs, and sidebar projects section in
  top-level forbidden-entry scans;
- extend scan tokens for camel/no-hyphen variants and explicit Reasonix/Kun
  deprecated identity patterns.

Reject:

- generating new `[Claw:...]` user-visible thread placeholders;
- exposing Kun identity, Reasonix protocol, or deprecated bridge/settings
  fallback;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer routes;
- treating scan/copy proof as packaged desktop QA or release readiness.

Rationale:

Compatibility recognizers protect existing users; new generated copy and route
scans protect analytix product sovereignty during upstream absorption.

Validation:

Focused renderer tests, `scan:product-sovereignty`, and `git diff --check`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0050 - Auto-Plan Payload Fields Do Not Trigger Plan Mode

Date: 2026-06-22
Sources: Reasonix post-881 auto-plan review, analytix HTTP start-turn contract
Domain: Plan mode, HTTP runtime contract, product sovereignty
Status: accepted

Conflict:

Reasonix supports auto-plan style configuration and controller semantics, but
analytix must not accept upstream `autoPlan` / `auto_plan` fields as public
turn contract because that would bypass the analytix-owned Plan mode entry
points and create a shadow public protocol.

Decision:

Accept:

- explicit `mode: "plan"` and `guiPlan` remain the only start-turn Plan mode
  triggers;
- `autoPlan` and `auto_plan` payload fields are ignored/stripped by the
  `StartTurnRequest` contract;
- captured agent-turn model requests must not advertise `create_plan` when
  only upstream-shaped auto-plan fields are present.

Reject:

- Reasonix public auto-plan setting or project/local override;
- Reasonix controller rebuild protocol, SessionAPI, or local config command;
- implicit Plan mode from unknown HTTP payload fields;
- top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback.

Rationale:

This preserves analytix turn-contract sovereignty while still absorbing the
Reasonix currentness lesson as negative contract evidence.

Validation:

Focused `http-server.test.ts -t "auto-plan payload"` passes. Full final gates
and forbidden-surface scans are recorded in release evidence.

## D-0051 - Parent Goal Evidence Replay Remains Internal Task Metadata

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job oracle,
Go G5 executable shadow
Domain: task jobs, parent goal evidence, Go G5 boundary
Status: accepted

Conflict:

Parent goal evidence is required for reliable task/sub-agent orchestration,
but exposing it as a Reasonix public job/session protocol or renderer-visible
route would violate analytix-owned runtime boundaries.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.taskJobs.parentGoalEvidence` records active-goal and
  missing-goal event metadata;
- Go shadow derives `ledgeredWhenActiveGoal` and `errorsWithoutActiveGoal`;
- event keys remain analytix-owned `evidenceLedgered` and
  `evidenceLedgerError`;
- TypeScript task-job orchestration remains the live authority.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- live Go Job Manager, Go task-job routes, renderer-visible Go route, or
  default Go backend;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite, or
  treating executable shadow as packaged desktop QA.

Rationale:

Executable replay improves the G5 gate while keeping parent-goal evidence an
internal analytix task metadata contract.

Validation:

Focused `task-job-orchestration-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0052 - Product Sovereignty Scans Are A Required Absorption Gate

Date: 2026-06-22
Sources: Kun/Reasonix absorption QA, release evidence forbidden scans,
renderer route-surface tests
Domain: product sovereignty, QA automation
Status: accepted

Conflict:

Repeated hand-run grep commands have been enough for local proof, but they are
easy to drift or omit as Kun/Reasonix absorption continues. Product-sovereignty
boundaries need a stable engineering entry point.

Decision:

Accept a reusable scan gate:

- `npm run scan:product-sovereignty` checks forbidden top-level product
  entries, upstream identity/protocol leakage, deprecated bridge/settings
  fallback, default Go/Rust/Tauri enabling, Rust/Tauri files, and Connect
  Phone locale copy;
- the renderer route-surface test includes `PluginMarketplaceView.tsx`, so
  plugin-facing MCP work cannot accidentally become a top-level MCP-indexer
  product entry;
- this scan is a required QA aid for future absorption batches that touch
  product surfaces, settings, bridge, Go, or upstream identity.

Reject:

- treating a passing scan as packaged desktop QA or release readiness;
- replacing focused behavior tests with regex-only evidence;
- allowing scans to whitelist active production `window.kun`, Reasonix
  protocol, deprecated settings fallback, default Go backend, or Rust/Tauri
  activation.

Rationale:

The scan gives analytix a repeatable guardrail while keeping product ownership
with analytix contracts and tests.

Validation:

Focused `npm run scan:product-sovereignty` and
`Workbench.route-surface.test.ts` pass. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0050 - Reasonix Auto-Plan Config Does Not Become Analytix Settings

Date: 2026-06-22
Sources: Reasonix `01d9b173`, analytix settings normalization and runtime
config schema
Domain: settings, auto-router, product sovereignty
Status: accepted

Conflict:

Reasonix adds a user-level auto-plan setting and explicitly rejects project or
local auto-plan overrides. The useful boundary is preventing project-local
state from changing classifier behavior, but importing Reasonix settings names
or controller protocol would violate analytix product contracts.

Decision:

Accept only the negative settings/config boundary:

- root `agent.auto_plan`, root `autoPlan`, and root `auto_plan` are stripped
  during GUI settings normalization;
- runtime `autoPlan` and `auto_plan` are stripped before merge, migration, and
  runtime settings-key generation;
- persisted `analytix-settings.json` re-saves without Reasonix auto-plan
  shapes;
- `analytix serve` config remains strict and rejects `agent.auto_plan`,
  `runtime.auto_plan`, and `serve.autoPlan`.

Reject:

- `reasonix config auto-plan`, `--local`, project/local override files, or
  Reasonix settings schema;
- Reasonix controller API, SessionAPI, or public task/planner protocol;
- product auto-plan toggles outside top-level analytix `runtime` settings;
- renderer-visible Go routes, default Go backend, Kun identity, deprecated
  bridge/settings fallback, Rust/Tauri rewrite, or hidden top-level capability
  routes.

Rationale:

This keeps the classifier/currentness lesson while preserving analytix-owned
settings, bridge, runtime, and product boundaries.

Validation:

Focused `app-settings.test.ts`, `settings-store.test.ts`, and
`analytix-config.test.ts` pass. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0046 - Approval/User-Input Route Body Replay Stays Internal

Date: 2026-06-22
Sources: Reasonix approval/user-input gate review, analytix route oracle, Go G5
shadow gate
Domain: approvals, user input, Go shadow, product boundary
Status: accepted

Conflict:

Reasonix-style approval/user-input gate behavior is useful, especially exact
request/response body handling and structured prompt preservation. Absorbing
that evidence must not introduce a Reasonix public ask/session protocol or
make Go a live gate manager.

Decision:

Accept only analytix-owned route and shadow evidence:

- G5 `approvalUserInputReplay.approvalRoute` records the deny request body,
  response status/body, summary, tool name, and pending counts;
- G5 `approvalUserInputReplay.userInputSubmitRoute` records structured prompt
  questions/options, answer request body, HTTP response body, and resolved
  event redaction;
- G5 `approvalUserInputReplay.userInputCancelRoute` records cancel request
  body, response body, late `404`, and pending cleanup;
- Go shadow computes these fields from `approval-user-input-route-oracle.json`;
- TypeScript runtime remains authoritative for live approval/user-input gates.

Reject:

- Reasonix SessionAPI or public ask protocol;
- renderer-visible Go routes or default Go backend behavior;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite;
- treating shadow replay as packaged desktop approval-card QA, Go G5 parity, G6
  readiness, or release readiness.

Rationale:

This strengthens approval/user-input regression evidence while keeping the
runtime contract under `analytix serve` and the existing renderer -> preload ->
main -> runtime HTTP/SSE boundary.

Validation:

Focused `go-runtime-conformance.test.ts`, `approval-user-input-route-oracle.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0047 - MCP Lifecycle Detail Replay Stays Behind Analytix Contracts

Date: 2026-06-22
Sources: Reasonix MCP/tool lifecycle review, analytix MCP lifecycle oracle, Go
G5 shadow gate
Domain: MCP lifecycle, tool search, known overrides, Go shadow, product boundary
Status: accepted

Conflict:

Reasonix-style MCP lifecycle work has useful reconnect, known override, and
tool-search semantics. Absorbing those details must not expose an MCP-indexer
product entry, Reasonix public lifecycle protocol, or a live Go MCP client.

Decision:

Accept only analytix-owned oracle and shadow evidence:

- G5 `mcpReplay.backgroundReconnect` records retry/error/restart details from
  the TS MCP lifecycle oracle;
- G5 `mcpReplay.knownOverrideDiagnostics` records codegraph/codebase-memory
  cwd, priority, background-start, daemon timeout, and explicit-cwd behavior;
- G5 `mcpReplay.searchWorkspaceBoundary` records trusted/untrusted workspace,
  query, unknown-tool error, on-request policy, and denied no-execute behavior;
- Go shadow computes these fields from `mcp-tool-lifecycle-oracle.json`;
- TypeScript runtime remains authoritative for live MCP lifecycle behavior.

Reject:

- Reasonix MCP-indexer public lifecycle protocol;
- top-level MCP-indexer route or navigation;
- live Go MCP client/indexer or default Go backend behavior;
- renderer-visible Go routes, Kun identity, deprecated bridge/settings
  fallback, Rust/Tauri rewrite;
- treating shadow replay as credentialed MCP matrix, packaged desktop MCP QA,
  Go G5 parity, G6 readiness, or release readiness.

Rationale:

This strengthens MCP lifecycle regression evidence while preserving analytix
runtime/tool contracts and the hidden/internal nature of indexer behavior.

Validation:

Focused `mcp-tool-lifecycle-oracle.test.ts`, `go-runtime-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0048 - Provider Usage Parser Precedence Is Fixture-Owned

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G5
shadow gate
Domain: provider usage parsing, cache accounting, Go shadow, product boundary
Status: accepted

Conflict:

Provider cache accounting needs exact parser precedence across DeepSeek,
OpenAI-compatible responses, Anthropic messages, and unsupported providers.
Absorbing that value must not change provider protocols, expose Reasonix
contracts, or turn Go into the default provider client.

Decision:

Accept only analytix-owned oracle and shadow evidence:

- G5 `cacheReplay.usageParserReplay.deepseekNativePrecedence` proves native
  DeepSeek `prompt_cache_*` fields win over `prompt_tokens_details.cached_tokens`;
- G5 `cacheReplay.usageParserReplay.openaiResponsesCachedTokens` proves
  responses `input_tokens_details.cached_tokens` produces hit/miss accounting;
- G5 `cacheReplay.usageParserReplay.anthropicCacheFields` proves Anthropic
  read/creation cache fields are included in prompt/cache accounting;
- G5 `cacheReplay.usageParserReplay.unsupportedAbsentFields` proves unsupported
  providers do not fabricate cache-hit/cache-miss telemetry;
- Go shadow computes these fields from `provider-cache-oracle.json`.

Reject:

- Reasonix provider public protocol;
- live provider superiority claims from fixture-only evidence;
- live Go provider client or default Go backend behavior;
- renderer-visible Go routes, Kun identity, deprecated bridge/settings
  fallback, Rust/Tauri rewrite;
- treating shadow replay as credentialed provider matrix, packaged provider
  settings QA, Go G5 parity, G6 readiness, or release readiness.

Rationale:

This strengthens provider/cache regression evidence while preserving analytix
provider contracts and fixture-only proof boundaries.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0049 - Auto-Router Classifier Currentness Without Public Auto-Plan

Date: 2026-06-22
Sources: Reasonix post-881 auto-plan review, analytix auto-model-router tests,
Go G5 shadow gate
Domain: auto-router, classifier currentness, planner controls, Go shadow,
product boundary
Status: accepted

Conflict:

Reasonix post-881 changes make classifier rebuild/currentness important, but
analytix must not import Reasonix public auto-plan settings, project overrides,
or controller protocols.

Decision:

Accept only analytix-owned classifier contract evidence:

- G5 `controlExecutableCases.autoRouterClassifier.fingerprint` is tied to the
  live `AUTO_MODEL_ROUTER_FINGERPRINT`;
- classifier contract drift across model, prompt, timeout, sampling, and
  reasoning controls invalidates the route-cache contract;
- the classifier side path remains an isolated short JSON request with no
  prefix, tools, or context-instruction carryover;
- timeout fallback aborts the classifier and returns a heuristic concrete
  model/reasoning result;
- Go shadow replays these booleans from TS-owned fixtures.

Reject:

- Reasonix public auto-plan setting, CLI/config root, project/local override,
  controller API, or SessionAPI;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- live Go auto-router, renderer-visible Go routes, or default Go backend
  behavior;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite;
- treating shadow replay as packaged planner QA, Go G5 parity, G6 readiness,
  or release readiness.

Rationale:

This captures the engineering value of classifier rebuild-on-enable/currentness
while preserving analytix runtime/settings/product contracts.

Validation:

Focused `auto-model-router.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0154 - Session Route Replay Uses Analytix HTTP/SSE Contracts Only

Date: 2026-06-22
Sources: Reasonix session/fork/SSE route review, analytix G2 route replay
oracle, Go G5 executable control shadow
Domain: thread/session routes, SSE replay, fork/resume/archive/search,
runtime boundary
Status: accepted

Conflict:

Reasonix session APIs provide useful route lifecycle coverage, but importing
their public protocol or route names would violate analytix's runtime boundary.
The route proof also needs to be stronger than aggregate counts so fork,
resume, archive/search, and SSE replay regressions are caught precisely.

Decision:

Accept analytix-owned exact route replay:

- G5 `controlExecutableCases.sessionRouteReplay` is derived from the G2
  analytix route oracle;
- each row records method, path, setup, auth, response kind, status,
  request-body hash, response-body shape/hash, SSE frame count, event names,
  and SSE frame hash;
- Go shadow computes exact JSON body route count, exact SSE route count,
  runtime-token coverage, unauthorized ids, key archive/search/fork/resume
  hashes, replay/caught-up SSE hashes, and product-boundary booleans;
- the proof remains shadow-only and does not add renderer-visible Go routes.

Reject:

- Reasonix SessionAPI, public route names, or session/job protocol;
- live Go HTTP server readiness or default Go backend claims;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite, or
  packaged desktop route QA claims from this fixture.

Rationale:

This makes the route contract evidence precise while preserving analytix's
existing `analytix serve` HTTP/SSE boundary as the authority.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0094 - Task Parent Goal Evidence Replay Is Not a Public Sub-Agent Protocol

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job oracle,
Go G5 shadow conformance
Domain: task jobs, parent goal evidence, Go G5, product boundary
Status: accepted

Conflict:

Task/sub-agent orchestration can usefully report whether child output was
ledgered into a parent goal, but Reasonix exposes a broader public
sub-agent/job protocol. Analytix should absorb the evidence semantics without
turning it into a new renderer-visible protocol or top-level navigation entry.

Decision:

Accept only analytix-owned shadow evidence:

- `jobReplay.parentGoalEvidence` is derived from
  `task-job-orchestration-oracle.json`;
- the replay records `requiresActiveGoal:true`, `evidenceLedgered`, and
  `evidenceLedgerError`;
- Go shadow emits explicit `usesReasonixProtocol:false` and
  `topLevelRouteExposed:false` flags;
- TypeScript task/job orchestration remains authoritative for real execution.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or public route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  navigation;
- live Go Job Manager, renderer-visible Go routes, or default Go backend;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite, or
  hidden product entry;
- treating this shadow replay as packaged desktop sub-agent QA, Go G5/G6
  readiness, or release readiness.

Rationale:

This preserves the useful parent-goal evidence invariant while keeping child
task orchestration internal to analytix-owned runtime contracts.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0096 - Task Tool Contracts Remain Internal Runtime Contracts

Date: 2026-06-22
Sources: Reasonix task/sub-agent tool contract review, analytix task-job
oracle, Go G5 shadow conformance
Domain: task tools, sub-agent orchestration, Go G5, product boundary
Status: accepted

Conflict:

`task` and `parallel_tasks` provide useful orchestration contracts, but their
field names and gates must remain internal runtime/tool-host details. Replaying
these contracts must not create a Reasonix-style public task/sub-agent API or a
top-level product entry.

Decision:

Accept only analytix-owned shadow evidence:

- `jobReplay.toolContractBoundary` is derived from
  `task-job-orchestration-oracle.json`;
- the replay records `task` fields `prompt`, `run_in_background`,
  `continue_from`, and `fork_from`;
- the replay records `parallel_tasks` fields `tasks` and `depends_on`;
- both tools remain `internalRuntimeOnly:true`, with permission/dependency and
  planner read-only gates preserved;
- Go shadow emits explicit `usesReasonixProtocol:false` and
  `topLevelRouteExposed:false` flags.

Reject:

- Reasonix public task/sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  navigation;
- live Go Job Manager, renderer-visible Go routes, or default Go backend;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite, or
  hidden product entry;
- treating this shadow replay as packaged desktop task/sub-agent QA, Go G5/G6
  readiness, or release readiness.

Rationale:

This preserves the useful task/sub-agent contract semantics while proving they
remain internal analytix runtime contracts rather than product navigation or
upstream public protocol.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0095 - Planner-Executor Detail Replay Does Not Enable a Public Planner Surface

Date: 2026-06-22
Sources: Reasonix planner/sub-agent job orchestration review, analytix task-job
oracle, Go G5 shadow conformance
Domain: planner executor, task jobs, Go G5, product boundary
Status: accepted

Conflict:

Planner/executor orchestration details are useful evidence for failure,
cancellation, output offsets, and transcript propagation. They must not become
a Reasonix-style public planner/sub-agent protocol, a product planner toggle,
or a live Go Job Manager.

Decision:

Accept only analytix-owned shadow evidence:

- `jobReplay.plannerExecutor` is extended with `skippedReason`,
  `cancelReason`, `outputOffsetJobCount`, and
  `transcriptPropagationJobCount`;
- all fields are derived from `task-job-orchestration-oracle.json`;
- Go shadow emits those fields from `TaskPlannerExecutorOracle`;
- TypeScript task/job orchestration remains authoritative for real execution.

Reject:

- planner product toggle or top-level planner/sub-agent navigation;
- Reasonix public planner/sub-agent/job protocol, SessionAPI, or route names;
- live Go Job Manager, renderer-visible Go routes, or default Go backend;
- Kun identity, deprecated bridge/settings fallback, Rust/Tauri rewrite, or
  hidden product entry;
- treating this shadow replay as packaged desktop planner/sub-agent QA, Go
  G5/G6 readiness, or release readiness.

Rationale:

The replay improves auditability for planner-executor propagation while
preserving analytix-owned internal runtime contracts and product sovereignty.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0088 - G5 Provider Streaming Replay Remains Fixture-Only

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix G3 provider streaming oracle,
Go G5 full-loop shadow
Domain: provider streaming, usage accounting, Go G5, backend boundary
Status: accepted

Conflict:

Reasonix provider/cache work motivates stronger streaming and cache telemetry
evidence. But carrying provider streaming usage into G5 could be mistaken for a
live Go provider client, live provider/cache superiority, or a provider request
behavior change.

Decision:

Accept only analytix-owned fixture replay:

- `providerStreamingReplay` is derived from
  `go-g3-provider-streaming-usage-cache-oracle.json`;
- Go shadow parses the fixture SSE `usage` frame and compares prompt,
  completion, reasoning, total, cache-hit, cache-miss, and cache-hit-rate
  fields with the declared provider usage case;
- the result remains inside `shadowSlicesExpectedOutput`, not a renderer route,
  provider client, settings surface, or `analytix serve` behavior change.

Reject:

- claiming live provider/cache superiority from this evidence;
- changing provider request URL/body/header/stream parsing in this batch;
- exposing Reasonix provider protocol or public session protocol;
- enabling a live Go provider client, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged provider settings release claims.

Rationale:

This strengthens G5 provider/cache proof while preserving analytix product
sovereignty and the TypeScript runtime as the live authority.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0089 - Task-Job Route Executable Replay Does Not Expose Go Routes

Date: 2026-06-22
Sources: Reasonix sub-agent/job lifecycle review, analytix task-job routes,
Go G5 shadow
Domain: task jobs, internal runtime routes, Go G5, backend boundary
Status: accepted

Conflict:

Task-job wait/output/kill route behavior is important for future Go parity, but
putting it into G5 evidence could be mistaken for exposing a live Go route
server or public Reasonix sub-agent/job protocol.

Decision:

Accept only analytix-owned internal route executable replay:

- `task-job-orchestration-oracle.routeExecutable` stores expected TS route
  outcomes for unauthorized access, output offset/replay, wait completion,
  kill, missing output `404`, and rehydrated output/wait/kill;
- `task-job-orchestration-oracle.test.ts` binds the real TypeScript route
  assertions to those oracle fields;
- `jobReplay.routeExecutable` is computed by Go shadow from that TS-owned
  oracle and remains inside conformance output only.

Reject:

- live Go task-job HTTP route server or renderer-visible Go routes;
- default Go backend behavior or `analytix serve` replacement;
- Reasonix SessionAPI, public sub-agent/job protocol, or upstream route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  packaged desktop release claims from this evidence.

Rationale:

This improves G5 executable-shadow quality while keeping the TypeScript runtime
and analytix HTTP/SSE contract authoritative.

Validation:

Focused `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0093 - MCP Approval Annotation Replay Is Not a Live Approval Manager

Date: 2026-06-22
Sources: Reasonix MCP/tool approval review, analytix MCP lifecycle oracle
Domain: MCP approval metadata, Go G5 shadow, product boundary
Status: accepted

Conflict:

MCP annotations such as destructive and open-world hints are important for
approval safety. Replaying them in G5 strengthens cross-backend evidence, but
it must not imply a live Go approval manager, live MCP client, or public
Reasonix MCP protocol.

Decision:

Accept only summary replay:

- `mcpReplay.approvalAnnotations` records normalized MCP tool name,
  destructive/open-world flags, approval id, deny decision, result kind, and
  denied no-execute;
- TypeScript remains the live approval and MCP execution authority;
- Go G5 computes the summary from the TS-owned MCP lifecycle oracle;
- no route, renderer surface, or public protocol is added.

Reject:

- live Go approval manager or live Go MCP client parity from this summary;
- Reasonix MCP-indexer public protocol or top-level MCP-indexer navigation;
- credentialed MCP server matrix claims;
- changing MCP runtime behavior, provider settings, bridge, runtime route, or
  renderer surface in this batch;
- exposing Kun identity, deprecated bridge/settings fallback, default Go
  backend, or Rust/Tauri path;
- treating this as packaged MCP lifecycle QA or release readiness.

Rationale:

This makes high-risk MCP approval metadata auditable in G5 while preserving
analytix-owned approval gates and TypeScript runtime authority.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0092 - G5 MCP Indexer Summary Is Not Live Go MCP

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle
Domain: MCP lifecycle, live-local indexer, Go G5 shadow, product boundary
Status: accepted

Conflict:

Reasonix-style MCP/indexer lifecycle value includes retry, tombstone,
restart, and secret-safe diagnostics. Replaying those fields in G5 is useful,
but could be mistaken for a live Go MCP client or a public MCP-indexer route.

Decision:

Accept only TS-owned summary replay:

- `mcpReplay.liveLocalIndexer` records live-local indexer proof output from the
  MCP lifecycle oracle;
- TypeScript derives active paths, tombstones, restart, and secret safety from
  `runLiveLocalIndexerProof`;
- Go G5 computes the nested summary from the fixture and emits no route;
- the renderer sees no Go MCP route or top-level MCP-indexer entry.

Reject:

- live Go MCP client parity or G5/G6 readiness from this summary;
- Reasonix MCP-indexer public protocol or top-level MCP-indexer navigation;
- credentialed MCP server matrix claims;
- changing MCP runtime behavior, provider settings, bridge, runtime route, or
  renderer surface in this batch;
- exposing Kun identity, deprecated bridge/settings fallback, default Go
  backend, or Rust/Tauri path;
- treating this as packaged MCP lifecycle QA or release readiness.

Rationale:

This strengthens MCP/indexer lifecycle evidence while keeping TypeScript
runtime authority and analytix product sovereignty intact.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0091 - G5 Live-Local Proof Summary Is Not Go Provider Parity

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle,
D-0090 live-local HTTP proof
Domain: provider request shape, cache proof, Go G5 shadow, release claims
Status: accepted

Conflict:

Once local HTTP provider proof exists, it is useful to make that proof visible
to G5 shadow conformance. But a G5 summary could be mistaken for a live Go
provider client or credentialed provider parity.

Decision:

Accept only fixture-owned summary replay:

- `provider-cache-oracle.liveLocalHttpProof` records no-credential local HTTP
  proof scope and product-boundary flags;
- TypeScript tests derive the counts from `providerUsageCases` and
  `requestShapeCases`;
- Go G5 `cacheReplay.liveLocalHttpProof` computes the same compact summary from
  the TS-owned oracle;
- Go does not execute provider HTTP, parse provider streams, change backend
  selection, or expose renderer-visible routes.

Reject:

- claiming live Go provider parity or G5/G6 readiness from this summary;
- treating the summary as credentialed live provider/cache evidence;
- changing provider request URL/body/header/stream/usage behavior in this
  batch;
- exposing Reasonix provider protocol, default Go backend, renderer-visible Go
  routes, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path;
- treating this as packaged provider settings QA or release readiness.

Rationale:

The summary keeps D-0090 evidence replayable across TS and Go conformance while
preserving TypeScript runtime authority and analytix product sovereignty.

Validation:

Focused `provider-cache-proof.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0090 - Live-Local Provider Proof Is Not Live Superiority

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle
Domain: provider request shape, usage accounting, cache proof, release claims
Status: accepted

Conflict:

Running provider-cache cases through a local HTTP server gives stronger
executable evidence than fake-fetch fixtures, but it could be mistaken for
credentialed live provider/cache superiority or external provider QA.

Decision:

Accept only no-credential local HTTP executable proof:

- all provider usage cases execute through a local HTTP provider while the
  client keeps the original provider base URL for request construction;
- all request-shape cases execute through local HTTP and validate path, headers,
  body fields, and tool shape;
- the proof remains test-only and does not change provider clients, settings,
  bridges, runtime routes, or release behavior.

Reject:

- claiming live provider/cache superiority from this evidence;
- treating local HTTP proof as credentialed provider matrix or external load
  testing;
- changing provider request URL/body/header/stream/usage behavior in this
  batch;
- exposing Reasonix provider protocol, default Go backend, renderer-visible Go
  routes, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path;
- treating this as packaged provider settings QA, Go G5/G6 parity, or release
  readiness.

Rationale:

This raises evidence quality while keeping analytix provider contracts and
product sovereignty unchanged.

Validation:

Focused `provider-cache-proof.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0083 - Cache Drift Attribution Does Not Permit Dynamic Prefix Pollution

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G5
shadow conformance
Domain: provider cache diagnostics, stable prefix, Go G5, product sovereignty
Status: accepted

Conflict:

Reasonix-style cache diagnostics need to explain why a cache-visible prefix
changed, but analytix cache efficiency depends on keeping the stable prefix
free of dynamic workspace context, timestamps, selected text, credentials, and
sidecar material. A drift-attribution replay must not become permission to put
mutable data into the stable prefix.

Decision:

Accept only analytix-owned offline drift attribution:

- `cacheReplay.driftAttribution` is derived from `ProviderCacheOracle`;
- previous/current prefix hashes are replayed for fixture comparison;
- `systemHashStable: true` and `prefixItemsHashStable: true` are required;
- Go shadow computes tool/provider/model/endpoint drift flags from oracle
  shapes;
- expected reasons are limited to `tools`, `provider`, and `model`;
- unsupported telemetry remains explicit via `telemetrySupported: false` and
  `cacheHitRateKnown: false`;
- TypeScript provider/cache tests remain the source for request URL/body,
  usage parsing, and cache accounting behavior.

Reject:

- putting workspace snippets, selected text, timestamps, credentials, or
  Reasonix sidecar material into the stable prefix;
- Reasonix provider public protocol, provider identity, or diagnostics format;
- live provider/cache superiority claims from offline fixture data;
- enabling a live Go provider client, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating drift-attribution replay as Go G5 parity, G6 readiness, packaged
  settings QA, or release readiness.

Rationale:

This strengthens cache diagnostics by making allowed drift reasons auditable
while preserving the analytix cache boundary and product-owned provider
contract.

Validation:

Focused provider-cache, Go G3/G4, Go G5 conformance, and Go shadow tests pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0087 - G5 Session Route Replay Does Not Expose Go Routes

Date: 2026-06-22
Sources: Reasonix session/fork/SSE route review, analytix G2 route oracle, Go G5
shadow conformance
Domain: thread/session HTTP/SSE routes, Go G5, backend boundary
Status: accepted

Conflict:

G5 full-loop shadow should prove archive/search/fork/resume/SSE route semantics
without turning Go into a renderer-visible route server or importing Reasonix
SessionAPI/public route names.

Decision:

Accept only offline G5 route-summary replay:

- `sessionReplay.routeStatusReplay` is derived from the G2 route oracle;
- the summary records route/status counts, archive/search result counts,
  read/update fields, fork lineage, resume summary, and SSE replay/caught-up
  frame/event evidence;
- TypeScript conformance derives the expected summary from the G2 fixture;
- Go shadow computes the same summary from route responses and SSE frames;
- TypeScript runtime HTTP/SSE routes remain the execution authority.

Reject:

- exposing renderer-visible Go routes or switching default backend;
- importing Reasonix SessionAPI/public protocol or route names;
- changing `window.analytix`, preload/main bridge, top-level runtime settings,
  or `analytix serve`;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating G5 route summary replay as Go G5/G6 parity, packaged desktop
  restart/resume QA, or release readiness.

Rationale:

This strengthens thread/session HTTP/SSE conformance evidence while preserving
the analytix runtime boundary.

Validation:

Focused G2/G5 conformance and Go shadow tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0189 - Runtime HTTP Auth Matrix Replay Does Not Enable Go HTTP Server

Date: 2026-06-23
Sources: Reasonix route/session review, analytix runtime HTTP router,
Go G5 runtime HTTP route sovereignty shadow
Domain: HTTP route auth, Go G5, backend boundary, product sovereignty
Status: accepted

Conflict:

The D-0190 TypeScript proof dispatches every registered `analytix serve` route
without auth, but Go G5 previously replayed only source-derived route counts
and auth-guard inventory. Future Go route work needs the same auth matrix in
the executable shadow without turning Go into a live HTTP server or public
control plane.

Decision:

Accept only fixture-owned auth-matrix replay:

- `runtimeHttpRouteSovereignty.authMatrix` records `/health` 200, all protected
  `/v1/*` route keys, unauthorized status/body, and sensitive route keys;
- TypeScript conformance dispatches every registered route against that matrix;
- Go shadow computes health public status, all-protected-route 401 status,
  unauthorized body shape, full matrix coverage, and sensitive route
  protection from TS-owned fixture input;
- the active TypeScript runtime router remains authoritative for real
  `analytix serve` behavior.

Reject:

- implementing or enabling a live Go HTTP server from this proof;
- adding renderer-visible Go routes, default Go backend selection, or Electron
  backend switching;
- exposing Reasonix SessionAPI, public route/session protocol, or route names;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating this proof as packaged route walkthrough, G6 readiness, release
  readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
  migration.

Rationale:

This raises the Go G5 route gate from route-inventory evidence to executable
auth-matrix evidence while preserving the TypeScript runtime and analytix-owned
public contract.

Validation:

Focused G5 conformance and Go shadow tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0190 - Runtime Forbidden Dispatch Replay Does Not Expose Go Routes

Date: 2026-06-23
Sources: Reasonix route/session review, analytix runtime HTTP router,
Go G5 runtime HTTP route sovereignty shadow
Domain: forbidden HTTP routes, Go G5, backend boundary, product sovereignty
Status: accepted

Conflict:

The D-0191 TypeScript proof dispatches every forbidden upstream and hidden
route token with valid auth and receives structured 404. Future Go route work
needs that evidence in executable shadow form, but replaying it must not
create a live Go router, public upstream route family, or hidden product
surface.

Decision:

Accept only fixture-owned forbidden-dispatch replay:

- `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` records valid runtime
  auth mode, 404 status/body, all forbidden tokens, upstream protocol tokens,
  and hidden-surface tokens;
- TypeScript conformance dispatches every forbidden token against that matrix;
- Go shadow computes structured not_found, full token coverage, upstream
  protocol rejection, hidden-surface rejection, and valid-auth dispatch mode
  from TS-owned fixture input;
- the active TypeScript runtime router remains authoritative for real
  `analytix serve` behavior.

Reject:

- implementing or enabling a live Go HTTP server from this proof;
- adding renderer-visible Go routes, default Go backend selection, or Electron
  backend switching;
- exposing Reasonix SessionAPI, public route/session protocol, or route names;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating this proof as packaged route walkthrough, G6 readiness, release
  readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
  migration.

Rationale:

This raises the Go G5 route gate from forbidden-token absence to executable
structured-404 replay while preserving the TypeScript runtime and
analytix-owned public contract.

Validation:

Focused G5 conformance and Go shadow tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0191 - Shared Endpoint Builder Replay Does Not Create A Route Protocol

Date: 2026-06-23
Sources: Reasonix route/session review, analytix shared endpoint contract,
Go G5 runtime HTTP route sovereignty shadow
Domain: endpoint builders, Go G5, backend boundary, product sovereignty
Status: accepted

Conflict:

The D-0192 shared endpoint proof centralizes renderer/main/runtime URL
construction, but carrying it into Go G5 must not turn shared templates into a
Reasonix public route protocol, a live Go router, or a hidden product surface.

Decision:

Accept only fixture-owned shared endpoint replay:

- `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` records encoded
  route-id builder outputs, exported endpoint strings, forbidden-token
  absence, canonical plural user-input state, and source unit-test proof;
- TypeScript conformance derives the matrix from shared endpoint source/test
  evidence;
- Go shadow computes builder count, encoded route-id preservation, template
  ownership, canonical plural user-input, sensitive builder coverage, and
  unit-proof presence from TS-owned fixture input;
- the active TypeScript shared endpoint contract remains authoritative for real
  renderer/main/runtime path construction.

Reject:

- implementing or enabling a live Go HTTP server from this proof;
- adding renderer-visible Go routes, default Go backend selection, or Electron
  backend switching;
- exposing Reasonix SessionAPI, public route/session protocol, or route names;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- exporting singular `/v1/user-input/{id}` as the canonical shared endpoint;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating this proof as packaged route walkthrough, G6 readiness, release
  readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
  migration.

Rationale:

This raises the Go G5 route gate from template-presence evidence to executable
builder-encoding replay while preserving the TypeScript shared endpoint layer
and analytix-owned public contract.

Validation:

Focused G5 conformance and Go shadow tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0188 - Package Runtime Identity Shadow Is Not Release Readiness

Date: 2026-06-22
Sources: Reasonix runtime protocol review, Kun packaging baseline,
analytix package/runtime CLI/release identity sources, Go G5 shadow conformance
Domain: package identity, runtime CLI, release identity, Go G5, product boundary
Status: accepted

Conflict:

Reasonix and Kun may provide useful runtime and packaging lessons, but
analytix public package identity and runtime CLI must remain product-owned.
Source-derived proof of `analytix serve` and release identity must not be
mistaken for packaged artifact QA, release readiness, or a default Go backend
claim.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.packageRuntimeIdentity` derives from package
  manifests, electron-builder config, app identity, main AppUserModelID,
  runtime CLI, binary resolver, afterPack validation, packaging tests, and
  release workflow;
- root package/product names remain `analytix`;
- runtime package/bin remain `analytix-runtime` and `analytix ->
  ./dist/cli/serve-entry.js`;
- the public runtime command remains `analytix serve`, with `ANALYTIX_READY`
  startup handshake;
- builder app id, artifact, NSIS names, Windows AppUserModelID, and release
  environment names remain analytix-owned.

Reject:

- exposing Reasonix public CLI/session protocol or route names;
- exposing Kun/DeepSeek public product identity;
- enabling renderer-visible Go routes or a default Go backend;
- adding Rust/Tauri rewrite paths;
- treating D-0188 as packaged artifact QA, G6 readiness, release readiness, or
  proof that signing/notarization/installer validation is complete.

Rationale:

This makes the `analytix serve` and release identity boundary machine-checkable
without changing runtime behavior or public product surfaces.

Validation:

Focused G5 conformance and Go shadow tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0187 - Renderer Route-Surface Shadow Is Not Hidden-Capability Navigation

Date: 2026-06-22
Sources: Reasonix frontend/session surface review, Kun 0.2.14 baseline,
analytix Workbench route-surface tests, Go G5 shadow conformance
Domain: renderer route surface, product sovereignty, dormant workflow code,
Go G5, UI boundary
Status: accepted

Conflict:

Reasonix-inspired orchestration code can exist internally, but Kun-target
top-level product entrypoints must not grow Workflow, Create Loop, Subagent,
AutoResearch, or MCP-indexer navigation. Renderer proof must not be mistaken
for permission to expose dormant workflow code.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.rendererRouteSurfaceSovereignty` derives from
  Workbench route-surface tests and renderer shell sources;
- `AppRoute` remains `chat/write/settings/plugins/claw/schedule`;
- forbidden top-level route tokens and entrypoint symbols are absent;
- dormant Workflow/Create Loop source remains quarantined from app entry
  surfaces;
- browser preview installs only `window.analytix`, plugin marketplace safe-area
  state is propagated, and shell navigation/titlebar regions remain no-drag and
  safe-inset aware.

Reject:

- exposing top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- exposing Reasonix public UI/session protocol, route names, or bridge aliases;
- adding Kun/deprecated bridge aliases or old runtime-shaped settings fallback;
- adding renderer-visible Go routes, default Go backend, or Rust/Tauri rewrite;
- treating D-0187 as packaged desktop QA, Go G6 readiness, release readiness,
  or hidden product authorization.

Rationale:

This makes UI sovereignty machine-checkable while preserving analytix product
ownership and the Kun 0.2.14 entry baseline.

Validation:

Focused G5 conformance, Go shadow tests, route-surface tests, and
forbidden-surface scans are recorded in release evidence.

## D-0091 - Desktop Main IPC Shadow Is Not A Reasonix Route Protocol

Date: 2026-06-22
Sources: Reasonix frontend/session route review, analytix main IPC schemas,
runtime SSE IPC tests, Go G5 shadow conformance
Domain: main IPC, runtime request allow-list, SSE cursor, Go G5, product
boundary
Status: accepted

Conflict:

Future Go parity needs executable evidence that Electron main enforces the
analytix runtime request allow-list and SSE cursor contract. That evidence must
not be mistaken for a Reasonix SessionAPI/main route protocol or renderer-
visible Go backend.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.desktopMainIpcBoundary` derives from real main IPC
  schema/handler and runtime SSE IPC sources plus tests;
- runtime request payloads are strict, analytix allow-list based, and forbidden
  Reasonix/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer routes are
  rejected before runtime adapter calls;
- SSE start rejects Reasonix session payloads, uses `/v1/threads/{id}/events`,
  preserves reconnect cursor, stops only matching stream ids, and batches
  pending events at 100ms;
- TypeScript main/preload/runtime behavior remains authoritative.

Reject:

- exposing Reasonix SessionAPI, public route names, or main IPC protocol;
- adding renderer-visible Go routes, default Go backend, or Electron backend
  selection from this proof;
- adding Kun/deprecated bridge aliases or old runtime-shaped settings fallback;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- treating D-0186 as packaged desktop QA, Go G6 readiness, release readiness,
  Kun identity, or Rust/Tauri migration.

Rationale:

This closes the main-process half of the desktop runtime bridge evidence while
preserving `window.analytix`, top-level runtime settings, `analytix serve`, and
the Renderer -> preload -> main -> runtime HTTP/SSE boundary.

Validation:

Focused G5 conformance, Go shadow tests, and forbidden-surface scans are
recorded in release evidence.

## D-0090 - Desktop Runtime IPC Shadow Is Not A Reasonix Frontend Protocol

Date: 2026-06-22
Sources: Reasonix frontend/session review, analytix preload bridge tests,
settings sovereignty tests, Go G5 shadow conformance
Domain: preload bridge, runtime IPC, SSE IPC, settings sovereignty, Go G5,
product boundary
Status: accepted

Conflict:

Future Go parity needs executable evidence that desktop runtime request and SSE
traffic stay on analytix-owned IPC channels. That evidence must not be mistaken
for a Reasonix SessionAPI/frontend protocol, a public Go renderer route, or a
deprecated Kun bridge fallback.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.desktopSovereignty` derives from real preload,
  window type, shared API, settings-store, and runtime normalizer sources;
- the case records `runtimeIpcChannels` as `runtime:request` and
  `runtime:sse:*`, with no forbidden Reasonix/Kun/Go/workflow IPC channels
  exposed;
- Go shadow computes `runtimeRequestUsesAnalytixIpc`,
  `runtimeSseUsesAnalytixIpc`, `publicApiTypesAnalytixOwned`, and bridge/settings
  ownership booleans from fixture inputs;
- TypeScript preload/main/runtime behavior remains authoritative.

Reject:

- exposing Reasonix SessionAPI, public frontend protocol, or route names;
- adding renderer-visible Go routes, default Go backend, or Electron backend
  selection from this proof;
- adding Kun/deprecated bridge aliases or old runtime-shaped settings fallback;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- treating D-0185 as packaged desktop QA, Go G6 readiness, release readiness,
  Kun identity, or Rust/Tauri migration.

Rationale:

This strengthens the desktop runtime contract evidence while preserving
`window.analytix`, top-level runtime settings, `analytix serve`, and the
Renderer -> preload -> main -> runtime HTTP/SSE boundary.

Validation:

Focused G5 conformance, Go shadow tests, and forbidden-surface scans are
recorded in release evidence.

## D-0089 - MCP Malformed Schema Shadow Is Not MCP-Indexer Protocol

Date: 2026-06-22
Sources: Reasonix post-881 MCP lifecycle review, analytix MCP tool provider
tests, Go G5 shadow conformance
Domain: MCP schema normalization, tool advertisement, Go G5, product boundary
Status: accepted

Conflict:

Future Go parity needs evidence that malformed MCP schemas cannot poison model
tool catalogs. That evidence must not be mistaken for a live Go MCP client, a
Reasonix MCP-indexer protocol, or a top-level MCP-indexer product entry.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.mcpMalformedSchemaBoundary` derives from
  `mcp-tool-provider.ts` and `mcp-tool-provider.test.ts`;
- the case proves non-object `inputSchema` values default to a safe object
  schema, non-record `properties` are dropped, mixed `required` arrays keep
  only strings, normalized tool names remain advertised, and non-record output
  schemas are omitted;
- Go shadow computes the same expected booleans from fixture inputs;
- TypeScript MCP provider behavior remains authoritative.

Reject:

- exposing Reasonix MCP-indexer protocol or route names;
- adding renderer-visible Go routes, default Go backend, or Electron backend
  selection from this proof;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating D-0184 as live Go MCP parity, credentialed MCP QA, G6 readiness,
  release readiness, Kun identity, or Rust/Tauri migration.

Rationale:

This strengthens MCP catalog safety evidence while keeping MCP behavior inside
analytix-owned runtime contracts.

Validation:

Focused G5 conformance, MCP provider tests, Go shadow tests, and
forbidden-surface scans are recorded in release evidence.

## D-0088 - Event JSONL Replay Shadow Is Not An Event Protocol

Date: 2026-06-22
Sources: Reasonix post-881 event durability review, analytix file session store,
runtime event recorder tests, Go G5 shadow conformance
Domain: event persistence, SSE replay, malformed recovery, Go G5, product
boundary
Status: accepted

Conflict:

Future Go parity needs evidence that `events.jsonl` replay, malformed recovery,
and sequence allocation match the TypeScript runtime. That evidence must not be
mistaken for a live Go event store, a Reasonix event/session protocol, or a new
renderer route surface.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.eventJsonlReplayBoundary` derives from
  `file-session-store.ts`, `loop.test.ts`, `runtime-event-recorder.test.ts`,
  and `file-session-store.test.ts`;
- the case proves newline-terminated append, replay filter/sort behavior,
  `highestSeq` max behavior, malformed JSONL line recovery,
  persist-before-publish ordering, concurrent seq uniqueness, persisted
  high-water caching, usage compaction carryover, and compaction-failure
  append-only recovery;
- Go shadow computes the same expected booleans from fixture inputs;
- TypeScript runtime event persistence and HTTP/SSE routes remain
  authoritative.

Reject:

- exposing Reasonix SessionAPI/event protocol or route names;
- adding renderer-visible Go routes, default Go backend, or Electron backend
  selection from this proof;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating D-0183 as live Go event-store parity, packaged long-thread replay
  QA, G6 readiness, release readiness, Kun identity, or Rust/Tauri migration.

Rationale:

This strengthens event replay and long-thread recovery evidence while keeping
durability behavior inside analytix-owned contracts.

Validation:

Focused G5 conformance, runtime event recorder/file-session tests, Go shadow
tests, and forbidden-surface scans are recorded in release evidence.

## D-0087 - Tool Result File/Image Shadow Is Not A File Protocol

Date: 2026-06-22
Sources: Reasonix post-881 file/image result review, analytix tool-result image
and attachment-store tests, renderer mapper tests, Go G5 shadow conformance
Domain: tool result files/images, attachment metadata, Go G5, product boundary
Status: accepted

Conflict:

Future Go parity needs evidence that image/file tool results, attachment
fallbacks, and generated-file metadata survive the runtime boundary. That
evidence must not be mistaken for a live Go file bridge, a Reasonix file route,
or a new renderer product surface.

Decision:

Accept only source-derived G5 shadow evidence:

- `controlExecutableCases.toolResultFileImageBoundary` derives from
  `tool-result-image.ts`, `tool-result-image.test.ts`,
  `attachment-store.test.ts`, and `analytix-mapper.test.ts`;
- the case proves inline image kinds, evicted base64 omission with metadata
  preservation, newest-image cap behavior, attachment `localFilePath`,
  text fallback `FilePath`, DeepSeek v4 fallback routing, and generated-file
  meta lifting;
- Go shadow computes the same expected booleans from fixture inputs;
- TypeScript runtime attachment storage, renderer projection, and model request
  behavior remain authoritative.

Reject:

- exposing Reasonix SessionAPI/file protocol or route names;
- adding renderer-visible Go routes, default Go backend, or Electron backend
  selection from this proof;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- changing `window.analytix`, top-level runtime settings, or `analytix serve`;
- treating D-0182 as live Go attachment parity, packaged attachment QA, G6
  readiness, release readiness, Kun identity, or Rust/Tauri migration.

Rationale:

This strengthens generated-file and attachment-path preservation evidence while
keeping file/image behavior inside analytix-owned contracts.

Validation:

Focused G5 conformance, attachment/tool-result image tests, Go shadow tests,
and forbidden-surface scans are recorded in release evidence.

## D-0086 - G5 Request Shape Replay Is Summary Evidence Only

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G5
shadow conformance
Domain: provider request URL/body/tool shape, Go G5, backend boundary
Status: accepted

Conflict:

G5 full-loop shadow should expose provider request URL/body/tool-shape coverage,
but replaying a compact request-shape summary must not be mistaken for a live
Go provider implementation or a change to the current provider request
contract.

Decision:

Accept only offline G5 summary evidence:

- `cacheReplay.requestShapeReplay` is derived from `ProviderCacheOracle`;
- the summary records case count, exact URL count, endpoint families, custom
  full endpoint ids, tool-shape families, and required/forbidden body-field
  family counts;
- TypeScript conformance derives the expected summary from the provider-cache
  oracle;
- Go shadow computes the same summary from request-shape cases;
- provider request URL/body construction, headers, stream parsing, usage
  parsing, and cache diagnostics remain unchanged.

Reject:

- changing runtime provider URL/body construction from this proof;
- guessing appended paths for custom full endpoint URLs;
- Reasonix provider public protocol, identity, or route names;
- live provider/cache superiority claims from offline fixture data;
- enabling a live Go provider client, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating G5 request-shape replay as Go G5/G6 parity, packaged settings QA, or
  release readiness.

Rationale:

This makes full-loop shadow evidence more inspectable while preserving the
analytix-owned provider contract.

Validation:

Focused provider-cache, Go G3/G4, Go G5 conformance, and Go shadow tests pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0085 - G3 Request Shape Replay Does Not Change Provider Runtime Behavior

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G3
shadow conformance
Domain: provider request URL/body/tool shape, Go G3, backend boundary
Status: accepted

Conflict:

The provider request-shape oracle is strong in TypeScript tests, but Go G3
shadow previously replayed only request-shape ids. Future Go provider work
needs direct URL/body/tool-shape evidence without changing the current
TypeScript runtime provider behavior.

Decision:

Accept only offline G3 request-shape replay:

- `go-g3-provider-streaming-usage-cache-oracle.json` stores raw
  `requestShapeMatrix` cases from `ProviderCacheOracle`;
- `expectedOutput.requestShapeSummary` stores compact evidence for case count,
  exact URL count, endpoint families, custom full endpoint ids, tool-shape
  families, and required/forbidden body-field family counts;
- TypeScript conformance derives matrix and summary from the provider-cache
  oracle;
- Go shadow computes the compact output from raw request-shape cases;
- provider request URL/body construction, headers, stream parsing, usage
  parsing, and cache diagnostics remain unchanged.

Reject:

- changing runtime provider URL/body construction from this proof;
- guessing appended paths for custom full endpoint URLs;
- Reasonix provider public protocol, identity, or route names;
- live provider/cache superiority claims from offline fixture data;
- enabling a live Go provider client, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating G3 request-shape replay as Go G5/G6 parity, packaged settings QA, or
  release readiness.

Rationale:

This strengthens the Go provider gate while keeping the existing analytix
provider contract authoritative.

Validation:

Focused provider-cache, Go G3/G4, Go G5 conformance, and Go shadow tests pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0084 - G3 Drift Attribution Is Provider Gate Evidence Only

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G3
shadow conformance
Domain: provider cache diagnostics, Go G3, backend boundary
Status: accepted

Conflict:

The provider streaming/usage/cache gate should explain cache drift at G3, not
only in a later G5 summary. But moving drift attribution earlier must not be
treated as a provider runtime behavior change, a public protocol migration, or
permission to enable Go as a backend.

Decision:

Accept only offline G3 provider-gate evidence:

- `go-g3-provider-streaming-usage-cache-oracle.json` stores raw
  `cacheDriftAttribution` shapes from `ProviderCacheOracle`;
- `expectedOutput.cacheDriftAttribution` stores the compact replay output;
- TypeScript conformance derives both raw input and output from the
  provider-cache oracle;
- Go shadow computes the compact output from raw shapes;
- provider request URL/body behavior, usage parsing, stream parsing, and live
  credential policy remain unchanged.

Reject:

- changing runtime provider URL/body construction from this proof;
- changing cache diagnostics format or adding dynamic stable-prefix material;
- Reasonix provider public protocol, identity, or route names;
- live provider/cache superiority claims from offline fixture data;
- enabling a live Go provider client, renderer-visible Go route, or default Go
  backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  entries;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating G3 drift attribution as Go G5/G6 parity, packaged settings QA, or
  release readiness.

Rationale:

This makes provider-gate conformance stricter while keeping all runtime
execution and product boundaries under analytix-owned TypeScript contracts.

Validation:

Focused provider-cache, Go G3/G4, Go G5 conformance, and Go shadow tests pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0080 - MCP Core Lifecycle Replay Is Not MCP-Indexer Product Surface

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP tool lifecycle
oracle, Go G5 shadow conformance
Domain: MCP lifecycle, Go G5, product sovereignty
Status: accepted

Conflict:

MCP lifecycle semantics are useful conformance evidence, but making them a
top-level MCP-indexer or public Go MCP route would break the analytix product
contract and Kun entry hierarchy.

Decision:

Accept only shadow replay evidence:

- `mcpReplay.lifecycle.connect*` records connected tool names, availability,
  and tool count;
- `mcpReplay.lifecycle.disconnect*` records disconnect reason, hidden tool
  names, unavailable status, and retained diagnostic tool count;
- `mcpReplay.lifecycle.reload*` records reload tool names and stable schema
  order;
- `mcpReplay.lifecycle.cancel*` records cancel-before-start error text and
  no-execute behavior;
- `mcpReplay.lifecycle.error*` records approved tool execution failure shape;
- Go shadow decodes these values from the TS MCP lifecycle oracle and emits the
  same summary.

Reject:

- Reasonix MCP-indexer public protocol or route names;
- top-level MCP-indexer, Workflow, Create Loop, Subagent, or AutoResearch
  routes;
- live Go MCP client, Go MCP route exposure, renderer-visible Go route, or
  default Go backend;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating lifecycle replay as credentialed MCP matrix, packaged desktop MCP
  QA, Go G5 parity, G6 readiness, or release readiness.

Rationale:

Lifecycle replay improves the Go G5 gate without changing product surface or
backend authority. MCP remains an internal runtime/tool lifecycle capability
until explicitly promoted by an analytix-owned spec.

Validation:

Focused `mcp-tool-lifecycle-oracle.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0081 - Approval Decision Replay Is Not Reasonix Ask Protocol

Date: 2026-06-22
Sources: Reasonix approval/user-input lifecycle review, analytix
approval-user-input route oracle, Go G5 shadow conformance
Domain: approval gate, user-input replay, Go G5
Status: accepted

Conflict:

Approval decision and replay ordering are useful for Go G5 conformance, but
copying Reasonix ask/session protocol or exposing a public Go approval manager
would break analytix-owned runtime contracts.

Decision:

Accept only shadow replay evidence:

- `approvalUserInputReplay.approvalId` records the TS-owned approval id;
- `approvalDecision` and `approvalStatus` record denied decision output;
- `approvalPendingAfter` records no pending approval after resolution;
- `secondApprovalDecisionStatus` records late duplicate decision `409`;
- `replaySinceSeq` and `replayKindsInOrder` record the replay cursor and event
  order;
- Go shadow decodes these values from the TS approval/user-input route oracle
  and emits the same summary.

Reject:

- Reasonix SessionAPI or public approval/user-input protocol;
- live Go approval manager, Go approval route exposure, renderer-visible Go
  route, or default Go backend;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating approval replay as packaged desktop approval-card QA, Go G5 parity,
  G6 readiness, or release readiness.

Rationale:

Approval decision replay belongs in the G5 conformance gate while the
TypeScript runtime remains authoritative for live approval gates.

Validation:

Focused `approval-user-input-route-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0082 - Offline Provider Cache Seal Is Not Live Superiority

Date: 2026-06-22
Sources: Reasonix provider/cache review, analytix provider-cache oracle, Go G5
shadow conformance
Domain: provider/cache, DeepSeek prefix stability, multi-provider regression
Status: accepted

Conflict:

DeepSeek cache and stable-prefix evidence is necessary to prove analytix is not
behind Reasonix, but fixture/live-local evidence must not be presented as live
provider/cache superiority or a credentialed provider matrix.

Decision:

Accept only offline parity evidence:

- `cacheReplay.offlineParitySeal.fixtureOnly` remains `true`;
- `mayClaimLiveSuperiority` remains `false`;
- stable/equivalent DeepSeek prefix hashes, prefix item hashes, and tool hashes
  must match;
- DeepSeek stable-prefix cache hit/miss/rate is replayed from the TS oracle;
- OpenAI Responses, Anthropic Messages, and custom endpoint request-shape
  coverage remains counted in the same seal;
- release guard status, threshold, and tail window are replayed from the
  offline cache curve guard.

Reject:

- live provider/cache superiority claims;
- credentialed provider matrix or packaged provider settings QA from this
  evidence;
- Reasonix provider protocol or provider identity migration;
- live Go provider client, default Go backend, renderer-visible Go route, or
  runtime backend switch;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Rationale:

The seal makes fixture-backed provider/cache proof easier to audit while
keeping live superiority and release readiness explicitly out of scope.

Validation:

Focused `provider-cache-proof.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0078 - Task Job Lifecycle Replay Does Not Expose Job Protocol

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job
orchestration oracle, Go G5 shadow conformance
Domain: task jobs, Go G5, product sovereignty
Status: accepted

Conflict:

Reasonix-style task jobs include useful lifecycle semantics, but exposing a
public job protocol or top-level sub-agent surface would break the analytix
product contract and Kun 0.2.13/0.2.14 entry hierarchy.

Decision:

Accept only shadow replay evidence:

- `jobReplay.lifecycle.foreground` records task completion status/result from
  the TS task-job oracle;
- `jobReplay.lifecycle.background` records cross-turn running status, partial
  output, final completion, and final result;
- `jobReplay.lifecycle.waitOutputKill` records killed status and cancellation
  error;
- Go shadow decodes these source oracle fields and emits the same summary;
- TypeScript runtime contracts remain authoritative for live task/job
  execution.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- live Go Job Manager, Go task/job HTTP route exposure, renderer-visible Go
  route, or default Go backend;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating lifecycle replay as packaged desktop sub-agent QA, Go G5 parity, G6
  readiness, or release readiness.

Rationale:

Task lifecycle semantics are useful as conformance evidence, but product
surface and backend authority stay analytix-owned until G5/G6 gates are closed.

Validation:

Focused `task-job-orchestration-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0079 - Task Job Route Boundary Replay Is Not Public Job API

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job
orchestration oracle, Go G5 shadow conformance
Domain: task jobs, route auth, product sovereignty
Status: accepted

Conflict:

Task/job route semantics are useful for Go G5 conformance, but exposing them as
a Reasonix-like public job protocol or product navigation surface would violate
analytix-owned runtime contracts and Kun entry hierarchy.

Decision:

Accept only route-boundary shadow evidence:

- `jobReplay.routeBoundary.protectedRoutes` records `wait`, `output`, and
  `kill` as protected internal route ids;
- `jobReplay.routeBoundary.unauthorizedStatus` records `401`;
- `jobReplay.routeBoundary.forbiddenTopLevelRoutes` records `/v1/workflow`,
  `/v1/create-loop`, and `/v1/autoresearch`;
- Go shadow decodes these values from the TS task-job oracle and emits the same
  summary;
- TypeScript runtime contracts remain authoritative for live task-job routes.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- live Go task-job HTTP routes, live Go Job Manager, renderer-visible Go route,
  or default Go backend;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating route-boundary replay as packaged desktop sub-agent QA, Go G5 parity,
  G6 readiness, or release readiness.

Rationale:

Route auth belongs in the conformance gate, not in the public product surface.
This keeps task/job absorption useful while preserving analytix product
sovereignty.

Validation:

Focused `task-job-orchestration-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0074 - Go G5 Abort Cleanup Remains Shadow-Only

Date: 2026-06-22
Sources: Reasonix post-881 approval/user-input lifecycle review, analytix
approval-user-input route oracle, Go G5 executable control shadow
Domain: approval gate, user-input gate, SSE replay, Go G5, backend boundary
Status: accepted

Conflict:

Reasonix approvalManager/ask lifecycle cleanup is useful for future Go runtime
quality, but copying it as a live Go approval/user-input manager or public
SessionAPI would break analytix contracts. The evidence must prove the
abort-cleanup semantics without changing the runtime backend boundary.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.abortCleanup` is fixture-owned and derived from the
  TS approval/user-input route oracle;
- `replayG5AbortCleanup` returns expired approval, cancelled user-input, late
  approval decision `409`, late user-input resolve `404`,
  `pendingAfterCleanup: 0`, and replay kinds from the oracle;
- output explicitly records `usesReasonixProtocol: false` and
  `topLevelRouteExposed: false`;
- TypeScript runtime contracts remain authoritative for real approval,
  user-input, SSE replay, and GUI late-action handling.

Reject:

- implementing a live Go approval manager or user-input manager from this
  evidence;
- exposing Reasonix SessionAPI, public approval/user-input protocol, or
  controller route names;
- renderer-visible Go route, default Go backend, Electron main switch, or Go
  task/job routes;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  treating executable shadow evidence as Go G5/G6 parity, packaged desktop QA,
  or release readiness.

Rationale:

This raises Go G5 gate quality for approval/user-input cleanup while preserving
analytix product sovereignty and the TS runtime boundary.

Validation:

Focused `approval-user-input-route-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0076 - MCP Search Meta-Tool Replay Stays Internal

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP tool lifecycle
oracle, Go G5 shadow-slice output
Domain: MCP lifecycle, tool trust, Go G5, backend boundary
Status: accepted

Conflict:

MCP search meta-tools are useful for Reasonix-style indexer discovery, but they
must remain internal analytix tool surfaces. G5 replay evidence must not create
a public MCP-indexer route, a live Go MCP client, or a top-level product entry.

Decision:

Accept G5 replay-summary closure only:

- `mcpReplay` records the four internal meta-tools:
  `mcp_search`, `mcp_describe`, `mcp_call`, and `mcp_refresh_catalog`;
- untrusted workspace search remains hidden with `searchUntrustedSearchedTools:
  0`;
- unknown/untrusted calls surface the expected unknown-tool error;
- `mcp_call` remains `on-request`, and denied calls do not execute the MCP
  client;
- TypeScript runtime and MCP provider remain authoritative for live MCP
  execution.

Reject:

- Reasonix MCP-indexer public protocol or top-level MCP-indexer navigation;
- live Go MCP client or Go MCP routes from this evidence;
- renderer-visible Go route, default Go backend, Electron main switch,
  Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  hidden capability entries;
- treating G5 replay evidence as credentialed MCP matrix, packaged desktop QA,
  Go G5/G6 parity, or release readiness.

Validation:

Focused `mcp-tool-lifecycle-oracle.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0077 - Parallel Task Validation Does Not Expose Subagent Protocol

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job
orchestration oracle, Go G5 shadow-slice output
Domain: task jobs, parallel dependency validation, Go G5, product boundary
Status: accepted

Conflict:

Reasonix-style sub-agent orchestration benefits from strict parallel dependency
validation. Analytix can replay and enforce those internal constraints, but the
proof must not become a public Subagent route, Workflow/Create Loop route, or
Reasonix job/session protocol.

Decision:

Accept internal G5 replay only:

- `jobReplay.parallelValidation` records `depends_on`, the valid execution
  order, and five invalid case ids;
- error strings for single task, duplicate id, self dependency, cycle, and
  unknown dependency are sourced from the TS task-job oracle;
- Go shadow emits the same summary without implementing a live Go Job Manager;
- TypeScript runtime remains authoritative for real `task` / `parallel_tasks`
  execution.

Reject:

- public Reasonix sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  navigation;
- live Go Job Manager, renderer-visible Go route, default Go backend, Electron
  main switch, Rust/Tauri rewrite, Kun identity, deprecated bridge/settings
  fallback, or hidden capability entries;
- treating dependency replay as packaged desktop sub-agent QA, Go G5/G6 parity,
  or release readiness.

Validation:

Focused `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0075 - Abort Cleanup Replay Summary Does Not Create A Live Go Gate

Date: 2026-06-22
Sources: D-0074 executable shadow, analytix approval-user-input route oracle,
Go G5 shadow-slice output
Domain: approval gate, user-input gate, SSE replay, Go G5, backend boundary
Status: accepted

Conflict:

After adding executable abort-cleanup proof, G5 shadow summaries also need to
report the same lifecycle fields. But summary parity must not be treated as a
live Go approval/user-input manager or as Reasonix ask/session protocol parity.

Decision:

Accept replay-summary closure only:

- `controlReplay.abortCleanup` records expired approval, cancelled user-input,
  late approval decision `409`, late user-input resolve `404`,
  `pendingAfterCleanup: 0`, replay kinds, and product-boundary booleans;
- `approvalUserInputReplay` carries the same abort-cleanup ids/statuses and is
  computed in Go from the TS approval/user-input oracle;
- TypeScript runtime remains authoritative for real approval/user-input
  execution, GUI late-action handling, and SSE replay.

Reject:

- live Go approval/user-input managers from summary evidence;
- Reasonix SessionAPI, public ask protocol, controller routes, or renderer
  route exposure;
- default Go backend, Electron main switch, Go task/job routes, Rust/Tauri
  rewrite, Kun identity, deprecated bridge/settings fallback, or top-level
  hidden capability entries;
- treating replay-summary closure as Go G5/G6 parity, packaged desktop QA, or
  release readiness.

Validation:

Focused `approval-user-input-route-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0070 - Custom Chat Full Endpoint Keeps Chat Shape

Date: 2026-06-22
Sources: Reasonix provider/cache request-shape review, analytix
`provider-cache-oracle.json`
Domain: provider URL/body/header/tool shape, Go shadow request-shape replay
Status: accepted

Conflict:

Custom full endpoint mode is explicit. If a configured custom URL already ends
in `/chat/completions`, analytix must not append another endpoint path or send
Responses/Messages body fields. At the same time, adding the oracle must not
turn Go shadow work into a default backend or expose Reasonix provider
protocols.

Decision:

Accept only analytix-owned provider request-shape proof:

- `custom-chat-full-endpoint-request-shape` keeps the configured
  `/chat/completions` URL exact;
- the request uses OpenAI-compatible chat fields: `model`, `stream`,
  `messages`, `tools`, and `reasoning_effort`;
- the request uses Bearer `Authorization` and forbids Anthropic headers;
- the request forbids Responses/Messages-only body fields such as `input`,
  `system`, `max_output_tokens`, and `thinking`;
- G3/G5 Go fixtures may replay the TS-owned request-shape id only.

Reject:

- appending another provider path to a custom full chat endpoint;
- sending Responses or Messages request bodies to `/chat/completions`;
- Reasonix public provider protocol, route names, or settings identity;
- renderer-visible Go route, default Go backend, Electron-to-Go switch, or Go
  provider-client readiness claims from this evidence;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating fake-fetch request-shape proof as credentialed provider matrix,
  packaged settings QA, or release readiness.

Rationale:

This completes the custom full endpoint family oracle while preserving the
analytix provider contract and Go G5/G6 gates.

Validation:

Focused provider-cache/G3/G5 conformance tests and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0071 - Auto-Router Request Contract Drift Changes Fingerprints

Date: 2026-06-22
Sources: Reasonix `2db7acf6`, analytix auto-model-router
Domain: auto-router classifier currentness, route-cache key stability
Status: accepted

Conflict:

Reasonix rebuilds a desktop controller when auto-plan is enabled with a
configured classifier. Analytix does not expose Reasonix auto-plan settings,
project/local overrides, CLI, or controller protocol, but it still needs the
same stale-classifier protection for its internal auto-router route cache.

Decision:

Accept only analytix-owned currentness semantics:

- `AUTO_MODEL_ROUTER_FINGERPRINT` remains the route-cache key component;
- `buildAutoModelRouterFingerprint` fingerprints classifier model, prompt,
  timeout, response format, max tokens, temperature, and reasoning effort;
- default fingerprint output stays stable;
- max-token, temperature, and reasoning-effort drift produce different
  fingerprints;
- classifier state is not placed in the stable system prefix or public
  settings surface.

Reject:

- Reasonix `config auto-plan`, local/project auto-plan override, TOML schema,
  controller API, SessionAPI, or public protocol;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- default Go backend, renderer-visible Go route, Electron-to-Go switch, or
  live Go router parity claims from this evidence;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating runtime fingerprint proof as packaged desktop controller QA,
  user-visible auto-plan parity, or release readiness.

Rationale:

This preserves the useful Reasonix rebuild-on-enable invariant while keeping
analytix product sovereignty and existing runtime contracts intact.

Validation:

Focused `auto-model-router.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0153 - Auto-Router Recommendation Parsing Remains Internal Currentness Evidence

Date: 2026-06-22
Sources: Reasonix post-881 auto-plan classifier/currentness review, analytix
auto-model-router, Go G5 executable control shadow
Domain: auto-router, classifier recommendation trust, recent-context
currentness, product boundary
Status: accepted

Conflict:

Reasonix's classifier work makes stale route/controller state dangerous, but
copying its public auto-plan controller/config surface would leak upstream
protocol and product semantics. Analytix needs the currentness and parsing
invariants without creating a public auto-plan feature.

Decision:

Accept analytix-owned `_auto_router` evidence:

- classifier recommendations are trusted only when they parse to known
  concrete analytix model choices;
- `pro/max` and noisy `v4-flash` recommendations are accepted, while
  `model:"auto"` and malformed JSON are rejected;
- recent context excludes the active turn and carries historical tool results
  only as summarized text;
- G5 `controlExecutableCases.autoRouterClassifier` and Go shadow compute those
  recommendation/context booleans from TS-owned helpers;
- classifier state remains outside the stable prefix and no public route or
  settings surface is created.

Reject:

- Reasonix SessionAPI/controller/config protocol;
- public auto-plan settings or project-local auto-plan overrides;
- exposing classifier internals as renderer-visible Go routes;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- default Go backend, Rust/Tauri rewrite, Kun identity, or deprecated
  bridge/settings fallback;
- treating this fixture as packaged desktop QA or live Go auto-router parity.

Rationale:

This captures the useful Reasonix currentness lesson while keeping analytix
runtime contracts and product surfaces authoritative.

Validation:

Focused `auto-model-router.test.ts`, `go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0072 - Connect Phone Copy Owns New User-Facing Strings

Date: 2026-06-22
Sources: analytix product identity rules, Kun/Reasonix surface review
Domain: product identity, Connect Phone prompts, legacy compatibility
Status: accepted

Conflict:

Connect Phone implementation still has internal `claw` compatibility names.
Those are acceptable as storage/IPC/file-name compatibility, but new
user-visible, model-visible, schema-visible, or log-visible copy must not say
`Claw IM`.

Decision:

Accept:

- new runtime prompt headings say `Connect Phone`;
- new incoming IM thread titles use `[Connect Phone:...]`;
- schedule MCP schema descriptions and scheduled-task comments say Connect
  Phone;
- webhook log messages say Connect Phone;
- renderer recognizers and prompt unwrap logic keep legacy Claw headings/titles
  only so historical sessions still display correctly.

Reject:

- new Claw/Kun/Reasonix product copy, bridge aliases, settings fallbacks,
  public route names, or top-level navigation entries;
- deleting legacy Claw title/prompt recognition in a way that breaks old
  sessions;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, or
  release-readiness claims from this copy fix.

Rationale:

This keeps analytix product sovereignty without breaking compatibility data
that still uses internal `claw` naming.

Validation:

Focused Connect Phone prompt/runtime/renderer tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0073 - Go G5 Restart Shadow Does Not Enable Go Jobs

Date: 2026-06-22
Sources: Reasonix sub-agent/job orchestration review, analytix task-job oracle,
Go G5 shadow
Domain: Go runtime, durable task jobs, sub-agent/job orchestration
Status: accepted

Conflict:

Reasonix job orchestration suggests restart-visible durable work should survive
controller restarts. Analytix has a TypeScript task-job oracle for this, but
Go G5 must not become a live backend before G5/G6 gates are satisfied.

Decision:

Accept pure executable shadow only:

- `controlExecutableCases.taskJobs.restartDrill` is fixture-owned;
- TypeScript conformance derives the case from `task-job-orchestration-oracle`;
- Go shadow computes rehydrated count, combined output with `nextOffset: 2`,
  and queued kill status/error;
- TypeScript runtime remains the only live task-job implementation.

Reject:

- implementing a live Go Job Manager or Go task-job HTTP route;
- making Go the default backend or exposing renderer-visible Go routes;
- Reasonix SessionAPI, public sub-agent/job protocol, or route names;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  product entries;
- Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback, or
  release-readiness claims from this shadow proof.

Rationale:

This raises the quality of the G5 gate while preserving analytix runtime
authority and product sovereignty.

Validation:

Focused `task-job-orchestration-oracle.test.ts`, `go-runtime-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0066 - Auto-Router Classifier Request Stays Internal

Date: 2026-06-22
Sources: Reasonix post-881 auto-plan classifier/currentness review,
analytix auto-model-router
Domain: auto-router, classifier request shape, timeout fallback, cache
currentness, product boundary
Status: accepted

Conflict:

Reasonix post-881 moved classifier lifecycle and rebuild behavior closer to
auto-plan state. Analytix can absorb the safety/currentness value, but the
classifier must remain an internal runtime side path and cannot become a
Reasonix public controller/config protocol.

Decision:

Accept analytix-owned router contract semantics:

- `_auto_router` uses an isolated turn id and compact JSON response path;
- classifier requests advertise no tools, carry no immutable prefix, and do
  not use main-turn context instructions;
- classifier timeout aborts the internal request and falls back to heuristic
  concrete model/reasoning;
- classifier contract drift remains represented by
  `AUTO_MODEL_ROUTER_FINGERPRINT`, not Reasonix settings or controller state.

Reject:

- Reasonix `config auto-plan`, project/local auto-plan override, or upstream
  settings shape;
- exposing classifier request/controller details as public protocol;
- Reasonix SessionAPI/control protocol, default Go backend, renderer-visible Go
  route, Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback,
  or top-level hidden capability routes;
- treating router unit coverage as packaged desktop QA, live provider/cache
  superiority, or release readiness.

Defer:

- user-visible planner/auto-plan enable/disable controls;
- live controller rebuild UX and packaged desktop QA;
- Go auto-router parity.

Rationale:

The useful Reasonix invariant is that classifier contract drift and classifier
failure cannot leave stale or corrupt routing state. Analytix now proves that
in its own auto-router contract while preserving the public runtime boundary.

Validation:

Focused `auto-model-router.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0067 - Planner Executor Transcript Refs Stay Internal

Date: 2026-06-22
Sources: Reasonix task/sub-agent transcript continuation/fork review,
analytix planner-executor coordinator and task-job oracle
Domain: task jobs, planner executor, transcript identity, sub-agent boundary,
Go G5 shadow
Status: accepted

Conflict:

Reasonix exposes richer sub-agent transcript continuation/fork orchestration.
Analytix can absorb the durable metadata and identity-guard value, but must not
expose Reasonix SessionAPI, public task/job routes, or a top-level Subagent
product surface.

Decision:

Accept internal transcript propagation only:

- `runPlannerExecutorCoordinator` may accept optional `transcriptFor(task)`;
- the callback returns an already resolved analytix `TaskJobTranscriptRef`;
- `resolveTranscriptOperation` remains the identity guard before job creation;
- executor jobs persist the transcript ref as durable metadata;
- G5 shadow replay may carry only boolean/mode evidence, not a live Go job
  manager.

Reject:

- Reasonix public sub-agent/session/transcript protocol or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  navigation;
- live Go Job Manager, renderer-visible Go route, default Go backend,
  Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating this proof as packaged desktop sub-agent QA or full Reasonix
  planner/executor parity.

Defer:

- live desktop nested-card/sub-agent QA;
- broader planner/executor model-pairing benchmarks;
- Go live job execution after G5/G6 gates.

Rationale:

This captures the useful transcript continuity invariant while keeping the
public product and runtime contracts analytix-owned and internal.

Validation:

Focused task-job oracle, Go runtime conformance, and Go shadow tests pass. Full
final gates and forbidden-surface scans are recorded in release evidence.

## D-0068 - MCP Search Meta-Tools Stay Behind Trust And Approval

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle and tool discovery review,
analytix MCP search provider
Domain: MCP lifecycle, tool discovery, approval, workspace trust, product
boundary, Go G4 shadow
Status: accepted

Conflict:

Reasonix-style MCP indexer/search discovery is useful, but exposing it as a
public MCP-indexer product surface or bypassing trust/approval would violate
analytix product sovereignty.

Decision:

Accept search meta-tools only behind analytix contracts:

- `mcp_search` searches only workspace-trusted MCP records;
- `mcp_describe` and `mcp_call` resolve only trusted canonical tool ids;
- unknown or untrusted `toolId` returns a tool error without invoking the MCP
  client;
- `mcp_call` remains `on-request`, and denied approval prevents execution;
- G4 shadow evidence may replay advertised/trust/no-execute booleans.

Reject:

- top-level MCP-indexer navigation or public HTTP routes;
- Reasonix MCP/indexer public protocol or route names;
- bypassing workspace trust for search/discovery;
- bypassing approval for `mcp_call`;
- live Go MCP client, renderer-visible Go route, default Go backend,
  Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback.

Defer:

- credentialed MCP server matrix;
- packaged desktop MCP QA;
- Go MCP client readiness after the relevant gates.

Rationale:

This preserves the useful tool-discovery capability while keeping MCP execution
under analytix workspace trust and approval gates.

Validation:

Focused MCP provider, G4 conformance, and Go shadow tests pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0069 - Custom Messages Full Endpoint Keeps Messages Shape

Date: 2026-06-22
Sources: Reasonix provider/cache request-shape review, analytix provider-cache
oracle
Domain: provider request shape, custom endpoint, Anthropic Messages, runtime
contract
Status: accepted

Conflict:

Custom full endpoints are explicit user/provider configuration. A custom URL
ending in `/messages` must not receive another appended path, and must not be
sent OpenAI chat/responses body fields.

Decision:

Accept provider-cache oracle coverage:

- custom full `/messages` endpoint URL remains exact;
- request body uses Anthropic Messages fields: `system`, `messages`,
  `max_tokens`, and `tools[].input_schema`;
- request headers include the Anthropic-compatible headers expected by the
  Messages family;
- OpenAI `input`, `max_output_tokens`, and `thinking` fields are forbidden in
  this case.

Reject:

- Reasonix provider public protocol or settings shape;
- guessing an extra appended path for a custom full endpoint;
- sending OpenAI chat/responses body shape to `/messages`;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, deprecated bridge/settings fallback, or top-level hidden capability
  routes.

Defer:

- credentialed OpenAI/Anthropic/custom provider matrix;
- packaged provider settings QA;
- live provider/cache superiority claims.

Rationale:

This strengthens custom provider non-regression evidence without expanding the
public provider protocol surface.

Validation:

Focused provider-cache proof passes. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0063 - Go G5 Provider Cache Release Guard Stays Offline Shadow Evidence

Date: 2026-06-22
Sources: Reasonix cache curve guard review, analytix provider-cache oracle,
Go G5 shadow conformance
Domain: provider cache, Go runtime gate, release evidence
Status: accepted

Conflict:

Provider/cache release guard evidence is useful for proving stable prefix and
cache-hit curve discipline, but it can be overread as a live DeepSeek/OpenAI/
Anthropic superiority claim or as permission to route traffic through Go.

Decision:

Accept only offline executable shadow evidence:

- `provider-cache-oracle.json` remains the fixture source of truth;
- TypeScript expected output is derived with `evaluateOfflineCacheCurveGuard`;
- Go G5 computes tail averages, collapse counts, low-tail allowance, and
  overall pass/fail from the same fixture;
- `liveCredentialPolicy.mayClaimSuperiority` remains `false`;
- TypeScript runtime/provider clients remain authoritative for live traffic.

Reject:

- live provider/cache superiority claims from fixture or shadow evidence;
- Reasonix provider/cache public protocol or route names;
- default Go backend, renderer-visible Go route, or Electron-to-Go switch;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating offline cache guard evidence as packaged QA, G6 readiness, or release
  readiness.

Rationale:

This strengthens cache proof quality while keeping the analytix provider/runtime
contract and the G5/G6 Go gate intact.

Validation:

Focused `go-runtime-conformance.test.ts`, `provider-cache-proof.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0064 - Auto-Route Step/Cancel Composition Stays Inside Analytix AgentLoop

Date: 2026-06-22
Sources: Reasonix post-881 auto-plan/classifier review, analytix AgentLoop
Domain: auto routing, step limits, cancellation, cache stability
Status: accepted

Conflict:

Reasonix post-881 changes point at classifier/controller currentness and
planner gating. Analytix needs stronger proof that auto routing, step limits,
and cancellation compose safely, but copying Reasonix auto-plan settings or
controller APIs would violate product and protocol boundaries.

Decision:

Accept only an analytix-owned AgentLoop contract proof:

- `model:"auto"` routes once through `_auto_router`;
- the selected model/reasoning route is reused across the next model step;
- step-limit metadata reaches tool context but stays out of stable prefix and
  model-visible context;
- interrupted parallel tool batches preserve completed results and record
  cancelled results for interrupted/not-started calls.

Reject:

- Reasonix auto-plan settings, local/project overrides, controller APIs, or
  SessionAPI public protocol;
- dynamic step/cancel state in the stable system prefix;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback;
- treating this focused AgentLoop proof as packaged interruption QA or release
  readiness.

Rationale:

The useful Reasonix kernel value is composition stability, not its public
controller shape. Analytix keeps the existing runtime contract and adds a direct
test for the combined failure mode.

Validation:

Focused `loop.test.ts` passes. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0065 - Planner Step Limits Stay Runtime-Scoped And Prompt-Clean

Date: 2026-06-22
Sources: Reasonix post-881 planner gating/currentness review, analytix AgentLoop
Domain: planner gating, step limits, prompt/cache hygiene
Status: accepted

Conflict:

Reasonix planner gating is useful, but analytix must not copy Reasonix planner
settings, controller APIs, or public protocol. Planner-specific budget state
also must not become cache-visible prompt content.

Decision:

Accept only an analytix-owned AgentLoop contract proof:

- plan-mode turns use `plannerMaxModelSteps` instead of the higher
  default/user-global step limit;
- step-limit failures record `turn_step_limit_exceeded` with the planner budget;
- plan-mode still advertises the analytix `create_plan` tool;
- planner budget details stay out of stable prefix and model-visible context.

Reject:

- Reasonix planner enable/disable settings, controller APIs, or SessionAPI
  public protocol;
- dynamic planner budget state in stable prefix or model-visible prompt text;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  routes;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback;
- treating this focused AgentLoop proof as packaged plan-mode QA or release
  readiness.

Rationale:

The useful planner-gating value is scoped runtime behavior and prompt/cache
hygiene. Analytix keeps planner controls inside existing plan-mode contracts.

Validation:

Focused `loop.test.ts` passes. Full final gates and forbidden-surface scans are
recorded in release evidence.

## D-0066 - Matrix Green Does Not Mean Go Default Backend Ready

Date: 2026-06-23
Sources: D-0244 Reasonix capability matrix, D-0245 Kun/Analytix baseline guard,
D-0246 AutoResearch project state, D-0247 runtime-info readiness semantics and
main-adapter gate
Domain: Go G6 readiness, default backend cutover, product/runtime boundary
Status: accepted

Conflict:

D-0246 clears the remaining AutoResearch red/deferred row in the D-0244/D-0245
absorption matrices. That makes the capability and absorption matrices green,
but it must not be read as permission to make Go the default backend or retire
the TypeScript runtime.

Decision:

Accept matrix-green evidence only as absorption readiness:

- `readyForG6` remains as a compatibility alias for
  `capabilityMatrixGreen`/`absorptionMatrixGreen`;
- `/v1/runtime/info` exposes `readinessSemanticsD0247` and
  `defaultBackendReadinessD0243` under `capabilities.upstreamAbsorption`;
- default-backend readiness remains governed by D-0243 durable restart,
  credentialed provider matrix, credentialed MCP matrix, packaged QA, and
  `ANALYTIX_GO_RUNTIME_G6_READY=1`;
- skipped dry-run checks and fake/local provider or MCP matrices never count as
  credentialed passes;
- the main adapter keeps or rolls back to TypeScript when any D-0243 hard gate
  is missing.

Reject:

- treating D-0244/D-0245 `readyForG6:true` as default backend readiness;
- enabling Go from renderer/main because matrix rows are green;
- counting fake provider/MCP proofs as credentialed live matrices;
- exposing Reasonix public protocol, Kun identity, deprecated bridge/settings
  fallback, renderer-visible Go routes, or top-level Workflow/Create Loop/
  Subagent/AutoResearch/MCP-indexer routes;
- TypeScript runtime retirement before D-0243 and packaged release gates pass.

Rationale:

This preserves the useful Reasonix engine absorption evidence while keeping the
Kun/Analytix full-function product baseline and default-runtime authority
intact until the cutover gate has real external evidence.

Validation:

Focused `src/main/runtime/analytix-adapter.test.ts`,
`packages/runtime/tests/go-runtime-conformance.test.ts`, and
`packages/runtime-go go test ./...` cover the split. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0057 - Go G5 User-Input Gate Execution Remains Shadow-Only

Date: 2026-06-22
Sources: Reasonix approval/user-input/control lifecycle review, analytix
approval-user-input route oracle, Go G5 executable shadow
Domain: Go G5, user-input gates, replay privacy, backend boundary
Status: accepted

Conflict:

Reasonix-style ask/user-input lifecycles are useful for submitted-answer
delivery and late-resolution cleanup. But copying the public ask/session
protocol, adding Go user-input HTTP routes, or making Go a default backend
would bypass analytix product sovereignty and the G5/G6 gates.

Decision:

Accept only fixture-owned executable shadow control:

- `controlExecutableCases.userInput` is derived from
  `approval-user-input-route-api-gates-v1`;
- submitted answers may be echoed by the HTTP/gate response, but
  `user_input_resolved` replay omits answers;
- cancelled input gates resolve to `cancelled`, have zero pending gates, and
  reject a second resolve with 404;
- `replayG5UserInput` and `buildG5ApprovalUserInputReplay` compute the same
  output in pure Go shadow code;
- TypeScript runtime contracts remain authoritative for real user-input
  routing and GUI live-card behavior.

Reject:

- Reasonix public ask/session protocol, SessionAPI, or route names;
- implementing a live Go user-input manager or Go HTTP user-input route from
  this evidence;
- renderer-visible Go routes, default Go backend, or Electron-to-Go switching;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer
  routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as G5 parity, G6 readiness, packaged
  desktop live-card QA, or release readiness.

Rationale:

This strengthens the approval/user-input privacy and cleanup gate while keeping
Go as conformance-only shadow evidence and preserving analytix-owned runtime
contracts.

Validation:

Focused `go-runtime-conformance.test.ts`, `go-runtime-g3-g4-conformance.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0058 - Go G5 AutoResearch State Remains Project-Local And Shadow-Only

Date: 2026-06-22
Sources: Reasonix long-running task / AutoResearch state review, analytix
AutoResearchProjectStore, Go G5 executable shadow
Domain: Go G5, long-task state, product-entry boundary, cache hygiene
Status: accepted

Conflict:

Reasonix-style AutoResearch state is useful for long-running work, requirement
audits, and restart continuity. But exposing a top-level AutoResearch route or
copying upstream project protocol would violate the Kun 0.2.13/0.2.14 product
entry baseline and analytix-owned runtime contracts.

Decision:

Accept only project-local, fixture-owned executable shadow control:

- `controlExecutableCases.autoResearch` records
  `.analytix/autoresearch/<threadId>/` and the five analytix state files;
- TypeScript conformance creates real `AutoResearchProjectStore` state and
  proves unknown `requirement_id` evidence is rejected without writing
  findings;
- Go shadow computes no `REASONIX.md` write, no `AGENTS.md` mutation, no
  stable-prefix/tool-schema pollution, and no top-level route exposure;
- TypeScript runtime contracts remain authoritative for real `/goal --research`
  behavior.

Reject:

- Reasonix public AutoResearch/project protocol, route names, or project files;
- top-level AutoResearch, Workflow, Create Loop, Subagent, or MCP-indexer
  navigation/routes;
- implementing a live Go AutoResearch state store or default Go backend from
  this evidence;
- renderer-visible Go routes or Electron-to-Go switching;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as live long-task parity, G6 readiness,
  packaged desktop QA, or release readiness.

Rationale:

This preserves the useful long-task state and evidence-audit semantics while
keeping AutoResearch as an internal goal/runtime capability, not a public
product entry or Go backend claim.

Validation:

Focused `go-runtime-conformance.test.ts`, `autoresearch-store.test.ts`, and
`packages/runtime-go go test ./...` pass. Full final gates and forbidden-surface
scans are recorded in release evidence.

## D-0059 - Go G5 MCP Lifecycle Remains Fake-Local And Shadow-Only

Date: 2026-06-22
Sources: Reasonix MCP/indexer lifecycle review, analytix MCP lifecycle oracle,
Go G5 executable shadow
Domain: Go G5, MCP lifecycle, indexer state, diagnostics privacy
Status: accepted

Conflict:

Reasonix MCP/indexer lifecycle behavior is useful for retrying failed startup
servers, preserving tombstones across restart, and redacting diagnostics.
However, exposing a public MCP-indexer surface or copying upstream protocol
would violate analytix product sovereignty and the G5/G6 gates.

Decision:

Accept only fake-local, fixture-owned executable shadow control:

- `controlExecutableCases.mcpLifecycle` records failed server ids,
  connected/error ids, retry attempts, live-local paths, tombstone, resume, and
  redaction expected output;
- TypeScript conformance derives the case from `mcp-tool-lifecycle-oracle.json`
  and `runLiveLocalIndexerProof`;
- Go shadow computes retry attempts, active paths after tombstone/resume,
  `tombstoneCount:1`, `restartedFromSnapshot:true`, and
  `leaksSecret:false`;
- TypeScript runtime contracts remain authoritative for real MCP providers.

Reject:

- Reasonix MCP public protocol, route names, or MCP-indexer UI;
- top-level MCP-indexer, Workflow, Create Loop, Subagent, or AutoResearch
  navigation/routes;
- implementing a live Go MCP client or default Go backend from this evidence;
- renderer-visible Go routes or Electron-to-Go switching;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as live MCP parity, G6 readiness, packaged
  desktop QA, or release readiness.

Rationale:

This preserves the useful MCP lifecycle and diagnostics semantics while keeping
MCP-indexer as internal runtime/tool evidence, not a public product entry or Go
backend claim.

Validation:

Focused `go-runtime-conformance.test.ts`, `mcp-tool-lifecycle-oracle.test.ts`,
and `packages/runtime-go go test ./...` pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0060 - Go G5 Checkpoint/Rewind Remains Analytix-Owned And Shadow-Only

Date: 2026-06-22
Sources: Reasonix checkpoint/rewind engine review, analytix P4A/P4B/P4C
checkpoint oracle, Go G5 executable shadow
Domain: Go G5, checkpoint/rewind, audit safety, product boundary
Status: accepted

Conflict:

Reasonix checkpoint/rewind behavior is useful for safe recovery, but upstream
checkpoint protocol or direct git-ref restore semantics cannot become an
analytix public contract. Go G5 also cannot become the default backend before
G5/G6 gates pass.

Decision:

Accept only analytix-owned, fixture-backed executable shadow control:

- `controlExecutableCases.checkpointRewind` records `axcp_`, `axrp_`,
  `axra_`, and `axrr_` id boundaries, changed files, path risks, confirmation,
  and append-only event kinds;
- TypeScript conformance derives ready/blocked file counts and conversation
  audit from `buildAuditableCheckpointRewindPlan`;
- Go shadow computes path escape blocking, symlink blocking, legal
  dot-dot-prefixed filename readiness, explicit confirmation, append-only audit,
  no transcript rewrite, no git refs, no Reasonix protocol, and no top-level
  route exposure;
- TypeScript checkpoint/rewind services remain authoritative for real plan and
  apply behavior.

Reject:

- Reasonix checkpoint/rewind public protocol, route names, or SessionAPI
  exposure;
- Kun git-ref checkpoint behavior or direct git reset/checkout semantics;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  navigation/routes;
- implementing a live Go checkpoint store, Go checkpoint routes, or default Go
  backend from this evidence;
- renderer-visible Go routes or Electron-to-Go switching;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as live checkpoint parity, G6 readiness,
  packaged desktop QA, or release readiness.

Rationale:

This turns the existing P4A/P4B/P4C checkpoint safety line into a Go G5
executable gate while preserving the analytix-owned runtime contract and
append-only audit model.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0061 - Go G5 Remote Entry Stays A Narrow Analytix Control Port

Date: 2026-06-22
Sources: Reasonix SessionAPI/control-port review, analytix remote-entry control
port oracle, Go G5 executable shadow
Domain: Go G5, remote entry, control boundary, product sovereignty
Status: accepted

Conflict:

Reasonix remote/frontend driving ports are useful for separating lifecycle,
turn, approval, and ask controls, but importing SessionAPI or granting remote
entries access to goal, checkpoint, memory, storage, or tool-host internals
would expose a public upstream protocol and break analytix ownership.

Decision:

Accept only analytix-owned, fixture-backed executable shadow control:

- `controlExecutableCases.remoteEntry` records allowed `approvals`,
  `lifecycle`, and `turns` ports plus forbidden control-plane keys;
- TypeScript conformance derives the case from
  `approval-user-input-route-oracle.json` and the existing remote-entry port
  boundary;
- Go shadow computes forbidden control planes absent, policy override rejected,
  no goal/checkpoint/memory/storage/tool-host access, no Reasonix protocol, and
  no top-level route exposure;
- TypeScript runtime control ports remain authoritative for real remote-entry
  behavior.

Reject:

- Reasonix SessionAPI, public remote-entry protocol, or route names;
- granting remote/bot-like entries goal, checkpoint, memory, raw storage,
  thread service/store, or tool-host access;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  navigation/routes;
- implementing live Go remote-entry routes or default Go backend from this
  evidence;
- renderer-visible Go routes or Electron-to-Go switching;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as live remote-entry parity, G6 readiness,
  packaged desktop QA, or release readiness.

Rationale:

This preserves the useful control-port isolation from Reasonix while proving
the public analytix boundary remains narrower than upstream SessionAPI.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0062 - Go G5 Resume Keeps Pending Gates Non-Actionable

Date: 2026-06-22
Sources: Reasonix approval/user-input resume lifecycle review, analytix
approval/user-input route oracle, Go G5 executable shadow
Domain: Go G5, session resume, approval/user-input gates, product boundary
Status: accepted

Conflict:

Reasonix ask/session lifecycle behavior is useful for remote and resumed
sessions, but copying ask/session protocol or preserving actionable pending GUI
gates after a resume would either expose upstream protocol or create unsafe
duplicate decisions.

Decision:

Accept only analytix-owned, fixture-backed executable shadow control:

- `controlExecutableCases.resumePendingGates` records source pending approval
  and user-input ids plus resumed expired/cancelled statuses;
- TypeScript conformance derives the case from
  `approval-user-input-route-oracle.json`;
- Go shadow computes source pending statuses, resumed expired/cancelled
  statuses, `pendingAfterResume:0`, `answersCopiedToResume:false`, no Reasonix
  protocol, and no top-level route exposure;
- TypeScript runtime session resume remains authoritative for real thread
  cloning behavior.

Reject:

- Reasonix ask/session public protocol, SessionAPI, or route names;
- copying submitted answers or keeping resumed approval/user-input gates
  actionable;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer
  navigation/routes;
- implementing live Go session-resume routes, Go approval/user-input managers,
  or default Go backend from this evidence;
- renderer-visible Go routes or Electron-to-Go switching;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as live resume parity, G6 readiness,
  packaged desktop QA, or release readiness.

Rationale:

This turns the existing resume oracle into a Go G5 gate: resumed threads can
preserve audit visibility without carrying forward actionable GUI gates or
upstream ask/session protocol.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test
./...` pass. Full final gates and forbidden-surface scans are recorded in
release evidence.

## D-0055 - Managed Provider Currentness Stays On Analytix Runtime Settings

Date: 2026-06-21
Sources: Reasonix post-881 auto-plan classifier/currentness review, Kun
baseline identity/fallback review
Domain: runtime settings, provider currentness, product identity
Status: accepted

Conflict:

Reasonix rebuilds its auto-plan classifier when the user-level setting changes.
The useful invariant is that classifier/provider currentness must not reuse a
stale runtime configuration, but copying Reasonix config roots, controller APIs,
or public auto-plan settings would break analytix product ownership.

Decision:

Accept only analytix-owned runtime settings currentness:

- managed runtime fingerprints and settings-apply comparisons use
  `buildAnalytixRuntimeSettingsKey(resolveAnalytixRuntimeSettings(settings))`;
- selected provider base URL, endpoint format, model profile, and reasoning
  drift change that key;
- managed child runtime provider env snapshots refresh the same provider drift;
- model-visible system prompt copy uses Connect Phone and rejects
  `Claw/Kun/Reasonix`;
- public facade top-level domains remain analytix-owned; `agentProvider: "kun"`
  and `agents.kun` are rejected as runtime fallback input.

Reject:

- Reasonix `config auto-plan`, project/local auto-plan override, controller API,
  SessionAPI, or public protocol;
- user-visible auto-plan settings unless future work defines them under
  top-level `runtime` and `window.analytix`;
- Kun identity, Kun public protocol, deprecated bridge/settings fallback, or
  `agents.kun` runtime migration;
- top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer routes;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, live
  provider/cache superiority, packaged restart QA, or release readiness.

Rationale:

This preserves the Reasonix currentness value at the managed runtime boundary
while keeping provider routing, prompt identity, settings persistence, and
renderer bridge surfaces under analytix-owned contracts.

Validation:

Focused runtime prompt/auto-router tests and shared/main/preload/settings tests
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0056 - Auxiliary Provider URL Construction Uses Shared Analytix Helper

Date: 2026-06-21
Sources: Reasonix provider/cache request-shape review, analytix scheduled
detector and write-inline URL audit
Domain: provider URLs, auxiliary model consumers, request-shape safety
Status: accepted

Conflict:

Provider endpoint URL construction appears in multiple auxiliary consumers. If
Schedule keeps a simplified local builder while runtime/Write support versioned
endpoint bases and known full endpoint paths, providers can regress by receiving
duplicated paths such as `/v2/responses/v1/responses`.

Decision:

Accept a shared analytix URL helper:

- `upstreamOpenAiModelEndpointUrl` owns non-custom responses/messages endpoint
  construction, versioned `vN` bases, `/beta` normalization, and known endpoint
  path stripping;
- scheduled task detection and write-inline completion use that helper;
- chat-completions and custom full endpoint behavior stay aligned with existing
  helper semantics;
- tests cover helper behavior plus fake-fetch scheduled detector and write-inline
  non-regression paths.

Reject:

- copying a Reasonix provider protocol or config shape;
- treating this as live provider/cache superiority or credentialed provider
  matrix proof;
- changing renderer bridge/settings contracts, default Go backend, Go provider
  client, Kun identity, or top-level hidden capability routes.

Rationale:

Shared URL construction prevents auxiliary provider consumers from drifting
apart while preserving analytix provider contracts and offline proof boundaries.

Validation:

Focused shared URL, scheduled detector, and write-inline tests pass. Full final
gates and forbidden-surface scans are recorded in release evidence.

## D-0050 - Task-Job Restart Reconciliation Stays Internal

Date: 2026-06-21
Sources: Reasonix tool/sub-agent/job orchestration review, analytix task-job
oracle, Go G5 executable shadow
Domain: task jobs, restart recovery, Go shadow
Status: accepted

Conflict:

Reasonix-style job orchestration requires durable restart hygiene, but absorbing
that lifecycle discipline must not expose a Reasonix public job/session protocol
or promote Go task/job code into the live backend.

Decision:

Accept only analytix-owned restart reconciliation evidence:

- queued and running stale `TaskJobRecord` values are reconciled to explicit
  `failed` status after runtime restart;
- the oracle records the orphan ids, failure reason, expected status, and count;
- Go G5 executable shadow may compute the same stale jobs -> failed output from
  TS-owned fixtures;
- TypeScript runtime contracts remain authoritative for real task/job behavior.

Reject:

- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- renderer-visible Go task/job routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating this as packaged desktop restart QA, G6 readiness, or release
  readiness.

Rationale:

Restart recovery is a real robustness improvement only if it stays inside the
analytix task-job contract and does not leak upstream product/protocol identity.

Validation:

Focused `task-job-orchestration-oracle.test.ts`,
`go-runtime-conformance.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0051 - MCP Annotation Approval Stays Before Client Execution

Date: 2026-06-21
Sources: Reasonix MCP/tool lifecycle review, analytix MCP tool provider,
Go G4 shadow conformance
Domain: MCP tools, approval gate, no-execute safety
Status: accepted

Conflict:

MCP descriptors can declare destructive/open-world annotations and should feed
tool approval posture. But absorbing this behavior must not expose Reasonix MCP
public protocol, create a top-level MCP-indexer surface, or let denied GUI
approval execute an MCP client side effect.

Decision:

Accept only analytix-owned approval-before-execution semantics:

- destructive/openWorld MCP annotations map to approval-gated local tool policy;
- denied GUI approval returns an approval item;
- denied annotated MCP tools do not call `client.callTool`;
- Go G4 shadow may compute the no-execute boolean from TS-owned fixtures.

Reject:

- Reasonix MCP public protocol or MCP-indexer top-level entry;
- executing MCP client calls before approval resolution;
- renderer-visible Go MCP routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating fake-client lifecycle proof as live MCP credential matrix, packaged
  desktop QA, or release readiness.

Rationale:

MCP annotation support is valuable only when approval remains the first
side-effect boundary and MCP stays inside analytix-owned runtime/tool contracts.

Validation:

Focused `mcp-tool-lifecycle-oracle.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0052 - User-Input Answers Stay Out Of SSE Replay

Date: 2026-06-21
Sources: Reasonix user-input lifecycle review, analytix user-input route oracle,
Go G4 shadow conformance
Domain: user input, HTTP/SSE, privacy boundary
Status: accepted

Conflict:

Submitted user-input answers must reach the waiting runtime turn, but replaying
answers through SSE event history would broaden retention and expose more data
than the current analytix user-input item contract requires.

Decision:

Accept only analytix-owned submitted-route semantics:

- `/v1/user-inputs/:id` returns submitted answers in the HTTP response;
- `UserInputGate.request()` resolves with the submitted answers;
- `user_input_resolved` SSE replay records `status: "submitted"` without
  answer payloads;
- Go G4 shadow may compute the answer-echo and answer-free replay booleans from
  TS-owned fixtures.

Reject:

- Reasonix public user-input protocol or route names;
- storing submitted answers in replay events by default;
- renderer-visible Go user-input routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating route/oracle proof as packaged GUI live-card QA, live cross-device
  delivery, or release readiness.

Rationale:

This gives the runtime the submitted answer data it needs while keeping SSE
replay as a status/history mechanism rather than an answer archive.

Validation:

Focused `approval-user-input-route-oracle.test.ts`,
`go-runtime-g3-g4-conformance.test.ts`, and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0053 - Task-Job Control Routes Stay Authenticated Internal Routes

Date: 2026-06-21
Sources: Reasonix sub-agent/job orchestration review, analytix task-job route
oracle
Domain: task jobs, HTTP auth, product boundary
Status: accepted

Conflict:

Task-job wait/output/kill are useful orchestration controls, but exposing them
as unauthenticated or public upstream job APIs would break analytix runtime
contract boundaries.

Decision:

Accept only authenticated internal runtime route semantics:

- `/v1/runtime/task-jobs/wait` requires the runtime token;
- `/v1/runtime/task-jobs/output` requires the runtime token;
- `/v1/runtime/task-jobs/kill` requires the runtime token;
- missing-token requests return 401 before job lookup or mutation.

Reject:

- Reasonix public job/session protocol or route names;
- unauthenticated task-job wait/output/kill control;
- renderer-visible Go task-job routes or default Go backend behavior;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating route/oracle proof as packaged desktop QA, Go route auth readiness,
  or release readiness.

Rationale:

This keeps durable task orchestration behind analytix runtime authentication
while preserving the existing internal route surface for renderer/main/runtime
coordination.

Validation:

Focused `task-job-orchestration-oracle.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0054 - Connect Phone Copy Owns User-Visible Claw Naming

Date: 2026-06-21
Sources: Kun baseline review, Connect Phone product-sovereignty rule, renderer
locales, IM runtime replies, shared prompt hints
Domain: product identity, Connect Phone, internal compatibility naming
Status: accepted

Conflict:

`claw` remains the historical internal module/settings/IPC name for Connect
Phone/IM compatibility, but standalone `Claw` in user-visible locale strings,
IM command replies, runtime errors, or model-facing natural-language tool hints
weakens analytix product sovereignty.

Decision:

Accept Connect Phone copy ownership:

- English and Chinese renderer locale values must not contain standalone
  `Claw`;
- IM `/help`, `/model`, retired task, webhook disabled, and mirror runtime
  initialization replies must say Connect Phone;
- prompt natural-language media/tool hints must say `Connect Phone agent`;
- display unwrap must accept both old `Claw skill policy:` and new
  `Connect Phone skill policy:` prefixes for compatibility.

Reject:

- renaming internal `claw` schema/type/file/IPC/settings keys in this batch;
- breaking old managed prompt markers or legacy Claw thread-title recovery;
- restoring Kun identity, deprecated bridge/settings fallback, or upstream
  public protocol names;
- adding top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer entries;
- enabling default Go backend or starting Rust/Tauri migration;
- treating copy cleanup as live Connect Phone QA or release readiness.

Rationale:

This keeps the user-visible product surface sovereign while preserving the
compatibility internals that current settings, migrations, and thread recovery
still depend on.

Validation:

Focused locale/runtime/prompt tests pass. Full final gates and
forbidden-surface scans are recorded in release evidence.

## D-0046 - Go G5 Replays Task Approval Denial Only As Shadow

Date: 2026-06-21
Sources: Reasonix tool approval and sub-agent/job orchestration review,
analytix task-job oracle, Go G5 shadow conformance
Domain: Go G5, approval gate, task jobs, backend boundary
Status: accepted

Conflict:

After TypeScript proves denied `task` and `parallel_tasks` calls create no jobs
or child runs, future Go G5 work must not accidentally treat task/job approval
as a later concern. At the same time, carrying this proof into Go must not imply
that Go owns approvals, jobs, HTTP routes, or renderer-visible runtime behavior.

Decision:

Accept only shadow replay:

- `task-job-orchestration-oracle.json` records `approvalDenyNoExecute`;
- `go-g5-full-loop-oracle.json` carries that evidence under
  `jobReplay.approvalDenyNoExecute`;
- `BuildG5ShadowSlicesOutput` reads the TS-owned fixture and emits the same
  denied tool names, approval ids, and no-side-effect booleans;
- this proves future Go approval/job work must preserve
  approval-before-side-effect ordering.

Reject:

- implementing a Go approval manager or Go Job Manager from this evidence alone;
- exposing Go task/job HTTP routes, renderer-visible Go routes, or default Go
  backend behavior;
- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating shadow replay as Go G5 parity, G6 readiness, packaged desktop QA, or
  release readiness.

Rationale:

This keeps Go G5 aligned with analytix-owned approval safety while preserving
the rule that TypeScript runtime contracts remain authoritative until G5/G6
gates are satisfied.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0048 - Go Provider Cache Accounting Is Fixture-Only

Date: 2026-06-21
Sources: Reasonix provider/cache accounting review, analytix provider-cache
oracle, Go G3/G5 shadow conformance
Domain: provider cache accounting, Go G3/G5, backend boundary
Status: accepted

Conflict:

Provider cache proof must show that DeepSeek native cache telemetry, OpenAI
Responses cached tokens, Anthropic cache fields, and unsupported providers are
accounted for correctly. But carrying that accounting into Go must not imply
live provider superiority, a Go provider client, or a default Go backend.

Decision:

Accept only fixture-owned executable shadow accounting:

- `go-g3-provider-streaming-usage-cache-oracle.json` and
  `go-g5-full-loop-oracle.json` record `cacheAccounting` from the TypeScript
  provider-cache oracle;
- `buildG3ProviderCacheAccounting` computes supported telemetry case ids,
  unsupported unknown case ids, provider-family case ids, hit/miss totals, and
  aggregate hit rate from `providerUsageMatrix.expectedUsage`;
- `BuildG5ShadowSlicesOutput` computes the same accounting inside
  `cacheReplay`;
- unsupported provider usage remains unknown and is not counted as cache miss;
- TypeScript provider/cache tests remain authoritative for request parsing,
  cache prefix stability, and fake-provider response handling.

Reject:

- claiming live DeepSeek/OpenAI/Anthropic/custom provider superiority;
- using this proof as a credentialed provider matrix or packaged settings QA;
- introducing a Go provider client, renderer-visible Go route, or default Go
  backend;
- Reasonix provider protocol, Kun/deprecated settings identity, Rust/Tauri
  rewrite, or top-level hidden capability routes;
- treating fixture accounting as release readiness.

Rationale:

This strengthens Go G3/G5 gate quality and provider-cache evidence while
preserving analytix runtime authority and avoiding unsupported live claims.

Validation:

Focused `go-runtime-g3-g4-conformance.test.ts`, `go-runtime-conformance.test.ts`,
`provider-cache-proof.test.ts`, and `packages/runtime-go go test ./...` pass.
Full final gates and forbidden-surface scans are recorded in release evidence.

## D-0049 - Auto-Router Classifier Usage Stays Off Main Turn Accounting

Date: 2026-06-21
Sources: Reasonix post-881 auto-plan classifier rebuild/currentness review,
analytix auto-model-router and AgentLoop
Domain: auto-router, classifier fallback, usage/cache accounting, settings
boundary
Status: accepted

Conflict:

Reasonix post-881 treats classifier lifecycle as part of auto-plan state. In
analytix, the classifier can be absorbed only as an internal auto-router
contract. If the classifier emits usage/cache telemetry before failing, that
telemetry must not be recorded as main turn usage or cache evidence.

Decision:

Accept analytix-owned router isolation:

- `_auto_router` uses no tools and no immutable prefix;
- classifier failure falls back to the heuristic concrete model;
- router usage/cache telemetry is ignored by the main turn event stream and
  `UsageService`;
- classifier contract drift remains represented by analytix fingerprint/cache
  keys, not Reasonix config shape.

Reject:

- Reasonix `config auto-plan`, project/local auto-plan override, or Reasonix
  settings shape;
- recording router usage/cache telemetry as user-visible thread usage;
- exposing Reasonix public protocol, default Go backend, renderer-visible Go
  route, Rust/Tauri rewrite, Kun identity, deprecated bridge/settings fallback,
  or top-level hidden capability routes;
- treating this proof alone as Reasonix controller rebuild parity or
  user-visible auto-plan settings coverage.

Rationale:

This preserves the useful classifier/currentness behavior while preventing an
internal routing decision from corrupting user-visible usage/cache accounting.

Validation:

Focused `loop.test.ts -t "falls back to a concrete heuristic model without
recording router usage"` passes. Full final gates and forbidden-surface scans
are recorded in release evidence.

## D-0047 - Go G5 Approval Denial Execution Remains Shadow-Only

Date: 2026-06-21
Sources: Reasonix tool approval and sub-agent/job orchestration review,
analytix task-job oracle, Go G5 executable control shadow
Domain: Go G5, approval gate, task jobs, backend boundary
Status: accepted

Conflict:

Replaying approval-deny fixture data is useful, but future Go G5 work also
needs executable evidence that approval denial produces no durable jobs or
child runs. That calculation must not become a live Go approval manager or a
renderer-visible task/job surface.

Decision:

Accept only pure executable shadow control:

- `controlExecutableCases.approvalDeny` is fixture-owned and derived from the
  TS task-job oracle;
- `replayG5ApprovalDeny` returns denied tool names, approval ids, approval item
  count, `createsDurableJobs: false`, and `createsChildRuns: false`;
- Go tests compare the executable output to the TS-owned expected oracle;
- TypeScript runtime contracts remain authoritative for real approval and
  task/job execution.

Reject:

- implementing a live Go approval manager or Go Job Manager from this evidence;
- exposing Go task/job HTTP routes, renderer-visible Go routes, or default Go
  backend behavior;
- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- Rust/Tauri rewrite, Kun identity, or deprecated bridge/settings fallback;
- treating executable shadow control as Go G5 parity, G6 readiness, packaged
  desktop QA, or release readiness.

Rationale:

Executable shadow control raises the quality of the Go G5 gate while preserving
analytix product sovereignty and the TS runtime boundary.

Validation:

Focused `go-runtime-conformance.test.ts` and `packages/runtime-go go test ./...`
pass. Full final gates and forbidden-surface scans are recorded in release
evidence.

## D-0045 - Task Job Approvals Gate Durable Work Creation

Date: 2026-06-21
Sources: Reasonix tool approval and sub-agent/job orchestration review,
analytix task-job providers
Domain: tool approval, task jobs, child-run safety
Status: accepted

Conflict:

Task/sub-agent orchestration is powerful enough that approval ordering matters:
denying a GUI approval must happen before any durable job or child run is
created. A generic tool denial test is not sufficient evidence for the real
`task` and `parallel_tasks` providers.

Decision:

Accept only analytix-owned permission-gate semantics:

- `task` with `policy: "on-request"` returns an approval item when denied;
- `parallel_tasks` with `policy: "on-request"` returns an approval item when
  denied;
- denied task tools create no `DurableTaskJobManager` records;
- denied task tools create no `DelegationRuntime` child runs;
- this remains an internal runtime/tool-host contract, not a public Reasonix
  sub-agent/job protocol.

Reject:

- creating child jobs before approval resolution;
- Reasonix public sub-agent/job protocol, SessionAPI, or route names;
- top-level Subagent, Workflow, Create Loop, AutoResearch, or MCP-indexer routes;
- unauthenticated task-job control;
- default Go backend, renderer-visible Go route, Rust/Tauri rewrite, Kun
  identity, or deprecated bridge/settings fallback;
- treating runtime tool-host proof as packaged desktop approval-card QA.

Rationale:

This strengthens the approval/user-input safety line: task orchestration can be
absorbed only if the existing analytix permission gate remains the first
side-effect boundary.

Validation:

Focused `task-job-orchestration-oracle.test.ts` passes. Full final gates and
forbidden-surface scans are recorded in release evidence.
