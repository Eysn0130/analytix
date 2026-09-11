# analytix upstream comparison scorecard

Status: Reference / chronological score and evidence ledger.
Current as of: each dated row's recorded source and environment, not the
current worktree by default.

Use this scorecard after upstream sync batches and before release-readiness
claims.

Current-state note (2026-06-25): this scorecard contains long-lived historical
batch rows. Rows that say Go remains shadow-only, TypeScript is the default,
no default Go backend exists, TypeScript remains explicit rollback, or live
evidence blocks default backend readiness are superseded for current startup
behavior by the current adapter/runtime code and focused tests: Go runtime
default delivery is active through
`go-runtime-default`, while `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired
backend. Those historical rows still limit the claims they originally made and
do not become live provider/MCP/packaged/operator evidence.

Governance note (2026-07-02): scorecard baselines are measurement inputs, not
exclusive source boundaries. A low score, rejected public route, or absent
entry records current evidence only. Future analytix-native product entries may
be scored after explicit specs, UX rationale, tests, desktop QA, and
sovereignty scan updates.

Score scale:

| Score | Meaning |
| --- | --- |
| 0 | Missing or broken. |
| 1 | Exists but unstable, incomplete, or manually verified only. |
| 2 | Parity with upstream or previous analytix behavior. |
| 3 | Stronger than upstream in at least one measured way. |
| 4 | Stronger than upstream and covered by automated tests or repeatable QA. |
| 5 | Stronger, tested, documented, and stable across release gates. |

## Current Baseline Template

| Surface | Kun baseline | Reasonix baseline | analytix target | Current analytix score | Evidence | Gap |
| --- | --- | --- | --- | ---: | --- | --- |
| Product workflow | Code, Write, SDD, Connect Phone, schedule, GUI workflows. | Terminal-first agent; desktop emerging. | Stronger than the best relevant upstream workflow coverage with analytix UI. |  |  |  |
| Agent task success | Desktop agent loop. | Strong terminal coding agent and Go engine. | At least parity with the strongest absorbed engine areas. |  |  |  |
| Runtime contract reliability | Local HTTP/SSE `kun serve`. | Go CLI/runtime engine. | analytix HTTP/SSE/app-server contracts with conformance. |  |  |  |
| Tool and MCP reliability | MCP/Skills integrated with desktop. | Plugin-driven MCP-compatible tools. | Combined desktop approvals plus engine reliability. |  |  |  |
| Provider/cache/cost | Provider presets and cache-first loop. | DeepSeek prefix-cache focus. | Best combined provider/cache/cost behavior. |  |  |  |
| Thread smoothness | Desktop thread UI. | Terminal-first. | Analytix-derived virtualizer and Electron QA. |  |  |  |
| Cross-platform desktop | Electron app packages. | Single binary and desktop releases. | Strong desktop packaging plus runtime distribution. |  |  |  |
| Safety and privacy | Desktop permissions and settings. | Engine permissions and config. | Stricter, local, auditable controls. |  |  |  |
| Maintainability | Product upstream. | Engine upstream. | Contract-first absorption with ledgers. |  |  |  |

## Batch Entry Template

```text
## YYYY-MM-DD - <batch/release>

Branch:
Upstream sources:
Impacted dimensions:

Scores:
| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |

Claim:

Decision:

Follow-up:
```

## 2026-06-20 - P0/P2 Reasonix Session-Sidecar Absorption

Branch:
local `codex/p1-1-kun-provider-routing-closure` worktree.

Upstream sources:
Reasonix `main-v2` refreshed from `33545f6` to
`ef7bf970050f2293d85d44e689be1ec108b56fc0`; scoped implementation targets
`249a4f8`, `73e2025`, and `5db1d0b`.

Impacted dimensions:
Runtime contract reliability, thread smoothness, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 2 | 4 | `ThreadSummary` contract now includes optional `preview`, `turnCount`, and `messageCount`; runtime contract/domain tests pass. | Broader provider/cache/tool-schema P2 fixtures remain separate. |
| Thread smoothness | 2 | 3 | Hybrid listing can use sidecar summary and legacy backfill instead of hydrating full message JSONL for list summaries; focused hybrid tests pass. | No Electron sidebar timing trace was captured in this batch. |
| Maintainability | 2 | 4 | Reasonix refresh ledger, absorption ledger, scenario record, focused tests, `npm run typecheck`, and `git diff --check` are aligned. | Full release-strength score still needs broader QA. |

Claim:
Analytix is stronger only for the verified session-list/fork sidecar surface:
thread summaries can carry cached preview/counts through analytix contracts,
forks seed those fields, and legacy sidecars are backfilled once.

Decision:
Accept the session-sidecar absorption slice. Defer `f6ba755` memory/control
lock splitting, provider/cache fixtures, and tool-schema work to separate
batches.

Follow-up:
Run broader tests and real Electron sidebar timing QA before any release-level
claim.

## 2026-06-20 - P2.1 Reasonix Sidecar-Version Absorption

Branch:
local `master` worktree.

Upstream sources:
Reasonix `main-v2` refreshed from
`ef7bf970050f2293d85d44e689be1ec108b56fc0` to
`6d404d80094bb4153bbaec4d7a2739e875d5c8c7`; scoped implementation target
`7ebb08e`, with `341f720` classified for future Go runtime conformance.

Impacted dimensions:
Runtime contract reliability, thread smoothness, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Sidecar summaries now require `schemaVersion: 1` before preview/counts are trusted; legacy summaries decode once and stamp without changing `updatedAt`. | Existing SQLite rows are still treated as rebuildable index entries; full release gate needs desktop QA. |
| Thread smoothness | 3 | 4 | Authoritative zero-count sidecar summaries no longer trigger message JSONL decode during rebuild/list, and legacy missing-version summaries stop re-decoding after backfill. | No real Electron sidebar timing trace captured. |
| Maintainability | 4 | 4 | `341f720` was classified as a Go G4/G5 conformance requirement instead of forcing a fake TS lock abstraction. | Future Go controller contention fixture is still pending. |

Claim:
Analytix is stronger for the verified sidecar authority surface: zero-count
summary semantics are explicit, legacy sidecars self-heal once, and future Go
runtime requirements are documented without leaking Reasonix internals.

Decision:
Accept `7ebb08e` through analytix `metadata.jsonl` summary versioning. Defer
actual `341f720` code work until a Go backend has a controller lock that can be
tested.

Follow-up:
Add Go G2 sidecar-version oracle and G4/G5 goal-lock contention fixtures before
claiming Go parity.

## 2026-06-20 - Architecture/Spec V1 and Kun Master Drift Audit

Branch:
local `master` worktree.

Upstream sources:
Kun `master` moved from `8f2040349fba47fcd8e8b94f50b131943af839b2` to
`8602476c5c449b4561473ad5f31081ee93dc782e`; Kun `develop` stayed at
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` and is an ancestor of master.
Reasonix `main-v2` resolved to
`6d404d80094bb4153bbaec4d7a2739e875d5c8c7`.

Impacted dimensions:
Product workflow planning, provider/cache/cost planning, safety and privacy
planning, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product workflow | 2 | 2 | Workflow/Create Loop, Telegram Connect, tray/session/worktree, and composer/sidebar fixes are classified from Kun master `8602476` without copying Kun UI. | No Kun `8602476` product code has landed yet. |
| Provider/cache/cost | 2 | 2 | GLM custom endpoint fixes, Xiaomi model removal, compaction repair, and per-provider routing are marked P1.1 must-absorb items. | Provider matrix and runtime implementation are pending. |
| Safety and privacy | 2 | 2 | Preload sandbox fix is recorded as already covered plus regression guard; git checkpoint is P3 with Kun ref identity rejected. | No new preload or checkpoint tests in this planning-only batch. |
| Maintainability | 3 | 4 | `kun-sync.md`, `absorption-ledger.md`, implementation plan, blueprint, and `重构升级方案.md` now agree on Kun master `8602476`, Reasonix `6d404d8`, rejected identities, and next batch order. | Full freeze still depends on future sync if upstream HEADs change again. |

Claim:
This batch improves upstream absorption governance and implementation ordering
only. It does not claim that analytix has absorbed Kun master `8602476` product
or runtime behavior.

Decision:
Freeze architecture/spec v1 as the planning baseline and start the next
implementation batch at P1.1 Kun stable critical fixes.

Follow-up:
Run P1.1 provider/preload/runtime tests and desktop QA before making any
product or runtime superiority claim for the Kun stable-master drift.

## 2026-06-20 - P1.1 Kun Stable Critical Code Absorption

Branch:
local `master` worktree.

Upstream sources:
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`; Reasonix `main-v2`
preflight refreshed to `be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`.

Impacted dimensions:
Runtime contract reliability, provider/cache/cost, safety and privacy,
maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 2 | 4 | `thread.providerId` is persisted through thread/create/start-turn/review contracts; runtime routes by provider with `MultiProviderModelClient`; HybridThreadStore SQLite `provider_id` list summaries and sidecar rebuild preservation pass. | Full Electron smoke and packaged startup QA not run in this code slice. |
| Provider/cache/cost | 2 | 4 | Zhipu/Z.ai custom full `/chat/completions` endpoints, Xiaomi deprecated model removal, provider probe/model-list/write-inline/scheduled detector tests, and runtime model-client routing fixture pass. | Broader live-provider matrix remains manual because external credentials are not available. |
| Safety and privacy | 2 | 4 | Preload sandbox guard proves no node builtin preload import and analytix-only bridge exposure; active source scan avoids old bridge/CLI identity. | Git checkpoint safety is still deferred to P3. |
| Maintainability | 4 | 4 | `kun-sync.md`, `reasonix-sync.md`, `absorption-ledger.md`, spec 08, focused tests, typechecks, and `git diff --check` document accepted/deferred work. | Release-strength score still needs packaging and desktop QA at final gate. |

Claim:
Analytix is stronger than the pre-P1.1 state for scoped provider/runtime/preload
surfaces: provider choice no longer requires a runtime restart, GLM full
endpoints are represented explicitly, deprecated Xiaomi flash is removed, and
compaction preserves model-valid tool-call/tool-result tail history.

Decision:
Close the P1.1 provider/runtime/preload code absorption slice for scoped code
behavior. Do not treat this as full release/G0 closure.

Follow-up:
Proceed to P1.2 Connect Phone/Telegram if remote workflow reliability is the
next product priority, or P1.3 Workflow/Create Loop if automation is the next
strategic product surface.

## 2026-06-20 - P1.2 Connect Phone / Telegram Runtime Absorption

Branch:
local `codex/p1-2-connect-phone-absorption` worktree.

Upstream sources:
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`, focused on
Telegram / Connect Phone commit `a502a6d`; Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa` was checked for drift but did not
require code absorption in this slice.

Impacted dimensions:
Product workflow, runtime contract reliability, safety and privacy,
maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product workflow | 2 | 4 | Connect Phone now supports Telegram as a local analytix-managed remote workflow: token verification, private-chat allowlist, inbound text/image relay, generated reply, local GUI-to-Telegram mirror, settings labels, and schedule labels. | Live Telegram bot desktop QA is still required before release/G0 claims. |
| Runtime contract reliability | 4 | 4 | Telegram inbound turns still enter the analytix runtime through HTTP/SSE turn contracts; `thread.providerId`/channel `providerId` and model are preserved; image paths are uploaded to `/v1/attachments` and passed as `attachmentIds`. | No packaged app smoke run yet. |
| Safety and privacy | 4 | 4 | Renderer uses only `window.analytix.connectPhone`; user-facing copy is Connect Phone/analytix; IM turns disable runtime user input and force auto approval/full-access sandbox policy already used by phone workflows; attachment upload failures log and continue. | Real bot-token secret handling should be checked during desktop QA. |
| Maintainability | 4 | 4 | Shared schemas, preload, main runtime, renderer UI, settings normalization, IPC schemas, docs, focused tests, P1.1 regressions, typechecks, and `git diff --check` passed together. | Release-strength score still needs live desktop QA and packaging gates. |

Claim:
Analytix is stronger than the pre-P1.2 state for the scoped remote workflow
surface: Telegram no longer requires relay-only guidance, and remote messages
can enter the bundled analytix runtime while preserving identity, provider
routing, attachment contracts, and Connect Phone UI ownership.

Decision:
Accept the P1.2 Connect Phone / Telegram implementation slice as closed for
the scoped code behavior. Do not treat this as release/G0/full upstream
closure.

Follow-up:
After P1.2, the next strategic paths remain P1.3 Workflow/Create Loop, P2.2
agent quality/runtime conformance, or P3 desktop safety utilities; G0 should
wait for desktop QA, packaging, and the remaining release-readiness gates.

## 2026-06-20 - P1.3 Workflow / Create Loop Scoped Absorption

Branch:
local `codex/p1-3-workflow-create-loop` worktree.

Upstream sources:
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`, focused on
Workflow / Create Loop commits from `5488b9a` through `a43a2d5`; Reasonix
`main-v2` `be67a498adcaed6e33dcf3cbd25395e9cbccd6fa` was checked for workflow,
agent-loop, approval/user-input, event, memory/session, provider/cache, tool,
plugin, and desktop topic-migration relevance but did not require P1.3 code
absorption.

Impacted dimensions:
Product workflow, runtime contract reliability, safety and privacy,
maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product workflow | 2 | 2 | P0 later determined the top-level Workflow route was a product-position error for the Kun 0.2.13/0.2.14 baseline. The retained value is only the isolated Create Loop state machine/view tests, not a current main navigation entry. | Future Workflow/Create Loop product exposure needs a spec-approved entry position, UX review, durable workflow storage decision, and desktop QA. |
| Runtime contract reliability | 4 | 4 | Create Loop creates normal analytix runtime threads and sends turns through `AgentProvider.sendUserMessage`, preserving `providerId`, `model`, and mode without restarting the runtime or adding a new bridge. | Workflow run state itself is renderer-scoped in this slice; durable workflow event replay is deferred. |
| Safety and privacy | 4 | 4 | No `window.kunGui`, old settings envelope, `agents.kun`, `kun serve`, Reasonix protocol, local webhook, hook bridge, or workflow MCP exposure was added. Runtime approvals/user-inputs remain the existing analytix cards. | Full workflow permission/sandbox design is still needed before adding custom code modules, webhooks, or scheduled workflow triggers. |
| Maintainability | 4 | 4 | Focused tests cover create/relay/providerId, approval wait/resume, user-input wait, retry after failure, prompt context propagation, and renderer view states; Kun/Reasonix ledgers record absorbed/rejected/deferred scope. | Release-strength claim still needs real Electron workflow QA, persistent workflow storage decision, and broader builder tests. |

Claim:
This P1.3 claim is superseded for product navigation by the P0 correction. The
underlying Create Loop runtime/view remains useful as future implementation
evidence, but it is not a current top-level product surface and should not
raise the product-workflow score by itself.

Decision:
Accept only the retained scoped Create Loop internals as future evidence. The
top-level Workflow route decision is corrected by the P0 product-entry section
below. Do not treat this as full Kun Workflow builder parity, current product
navigation parity, Reasonix engine parity, release/G0 closure, or a Go runtime
milestone.

Follow-up:
Move next to P2.2 for Reasonix provider/cache/tool lifecycle work or P3 for
durable workflow/event persistence, checkpoint/review/generated-files safety,
and desktop workflow QA. G0 should wait for release-readiness gates and real
Electron QA.

## 2026-06-20 - P2.2 Reasonix Provider/Cache/Tool Lifecycle Diagnostics

Branch:
local `codex/p2-2-reasonix-provider-cache-tool-lifecycle` worktree.

Upstream sources:
Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e` and `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` were checked for regression
conflicts only.

Impacted dimensions:
Provider/cache/cost efficiency, tool and MCP reliability, runtime contract
reliability, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache/cost | 4 | 4 | Usage events now carry optional `cacheDiagnostics` with prefix/tool/provider/model hashes, cache hit/miss tokens, and change reasons, adapting Reasonix cache-shape diagnostics behind analytix contracts; focused cache and loop tests cover stable canonicalization and tool-schema cache drift. | Score stays 4 because live provider matrix and release QA are still pending. |
| Tool and MCP reliability | 3 | 4 | Canonical tool schema ordering no longer creates false cache-drift noise, while real tool catalog changes are attributed as `tools` on usage diagnostics. | Full MCP/plugin lazy lifecycle, cancellation, and protected-path tests remain future P3/Go work. |
| Runtime contract reliability | 4 | 4 | `usage.cacheDiagnostics` is optional, privacy-bounded, replayed through existing events, ignored by current renderer projection, and does not alter usage aggregation. | Future Go backend must match the optional diagnostics contract before parity claims. |
| Maintainability | 4 | 4 | Reasonix/Kun sync ledgers, absorption ledger, specs 08/09, focused tests, P1.2/P1.3 regressions, runtime package tests, typechecks, `git diff --check HEAD`, and identity scans document accepted/deferred/rejected scope. | Release-strength score still needs packaging, desktop QA, and live-provider credentials. |

Claim:
Analytix is stronger than the pre-P2.2 state for the scoped provider/cache/tool
diagnostics surface: provider cache hit/miss telemetry is now explainable in
terms of stable prefix, tool schema, provider, and model changes without
leaking raw prompt or tool content.

Decision:
Accept the P2.2 scoped Reasonix provider/cache/tool lifecycle diagnostics code
slice as closed for scoped code behavior. Do not treat this as full Reasonix
provider parity, full MCP/plugin lifecycle absorption, checkpoint/rollback
closure, Go runtime parity, or release/G0 closure.

Follow-up:
Proceed to P3 safety/checkpoint/durable workflow before G0 release gate. G0
should wait for desktop QA, packaging, identity scans, and any live-provider
matrix available with credentials.

## 2026-06-20 - P3A Tool/MCP/Sandbox/Checkpoint Boundary Slice

Branch:
local `codex/p3a-tool-mcp-safety-checkpoint-boundary` worktree.

Upstream sources:
Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e` and `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` were checked for baseline fidelity,
delta classification, and regression risk.

Impacted dimensions:
Tool and MCP reliability, safety and privacy, runtime contract reliability,
maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Tool and MCP reliability | 4 | 4 | MCP tool descriptors now normalize malformed `inputSchema` values before advertising them to the model/tool catalog; focused MCP fixture passes. | Score stays 4 because full MCP/plugin lazy lifecycle, hung stdio cancellation, protected-path expansion, and desktop QA remain future work. |
| Safety and privacy | 4 | 4 | Denied GUI approval is pinned with a direct no-execute test: the approval item is returned and the tool body is not invoked. | Full checkpoint/rewind safety and broader sandbox parity remain P4/P3B. |
| Runtime contract reliability | 4 | 4 | P3A changes only runtime-internal schema normalization and tests; `window.analytix`, top-level `runtime`, `analytix serve`, and existing HTTP/SSE events remain unchanged. | Go backend must reproduce these as G4 oracle fixtures later. |
| Maintainability | 4 | 4 | Kun/Reasonix sync ledgers, absorption ledger, conflict decision D-0006, Go/Rust boundary notes, and scorecard recorded `be67a498` as the P3A Reasonix checkpoint at the time, with P1.1/P1.2/P1.3/P2.2 as scoped closures. Later scorecard entries carry the current Reasonix head. | Release-strength claim still needs packaging, desktop QA, and live MCP/provider environments. |

Claim:
Analytix is stronger than the pre-P3A state for a narrow tool/MCP/approval
boundary: malformed MCP schemas are bounded before model exposure, and denied
approval execution safety is proven directly.

Decision:
Accepted the P3A scoped code slice after closure validation. Do not treat this as
full Kun master parity, full Reasonix MCP/plugin parity, checkpoint/rewind
closure, Go runtime parity, Rust adoption, or release/G0 closure.

Follow-up:
Move next to P4 checkpoint/rewind if product safety is the priority, or G0 Go
oracle fixture inventory if backend conformance is the priority.

## 2026-06-20 - P4A Checkpoint/Rewind Safety Oracle

Branch:
local `codex/p4a-checkpoint-rewind-safety-oracle` worktree.

Upstream sources:
Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e` and `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` were rechecked with
`git ls-remote`; no upstream HEAD drift was found.

Impacted dimensions:
Safety and privacy, runtime contract reliability, generated-files/review
readiness, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Safety and privacy | 4 | 4 | P4A adds analytix-owned `axcp_` checkpoint metadata, changed-file path escape rejection, and conversation-only rewind planning without file restore or transcript rewrite. | Score stays 4 until combined file restore, crash recovery, review UI, and desktop QA land. |
| Runtime contract reliability | 4 | 4 | `checkpoint_captured` is a versioned runtime event projected through the event reducer; focused oracle tests prove replay can retain the safe transcript boundary. | Go backend parity and live HTTP/SSE route exposure are future gates. |
| Generated-files/review readiness | 2 | 3 | File-change tool results can now be normalized into checkpoint changed-file metadata, giving generated-files/review UI a contract to consume later. | No renderer control or review panel integration in P4A. |
| Maintainability | 4 | 4 | Kun/Reasonix ledgers, conflict decision D-0007, specs 08/09, Go conformance, and focused tests record accepted/deferred/rejected scope. | Release-strength claim still needs packaging, desktop QA, and broader rewind tests. |

Claim:
Analytix is stronger than the pre-P4A state because checkpoint safety is no
longer only an upstream planning note: it has a versioned analytix contract, a
runtime event, path-safety guards, event replay projection, and a Go G5 oracle
fixture.

Decision:
Accept the P4A scoped oracle slice as closed for contract/test behavior. Do not
treat this as full Kun git rollback parity, full Reasonix rewind parity, file
restore, review UI integration, Go runtime parity, Rust adoption, or release/G0
closure.

Follow-up:
Move next to P4B full rewind/review UI if product safety is the priority. G0 Go
oracle inventory can follow after P4B/P4C or run in parallel only if it remains
conformance-only and does not scaffold a backend.

## 2026-06-20 - P4B Full Rewind/Review UI Plan-Only Slice

Branch:
local `codex/p4b-full-rewind-review-ui` worktree.

Upstream sources:
Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e` and `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` were rechecked with
`git ls-remote`; no upstream HEAD drift was found.

Impacted dimensions:
Safety and privacy, runtime contract reliability, generated-files/review
readiness, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Safety and privacy | 4 | 4 | P4B adds `axrp_` plan-only rewind plans, blocks path escape / absolute persisted paths / symlink risk, fixes legal `..name` filenames, and proves no prompt/full-content leakage. | Score stays 4 until destructive restore/apply and desktop QA exist. |
| Runtime contract reliability | 4 | 4 | Added read-only route/service, shared endpoint, IPC allow-list, renderer provider method, and focused code/conversation/combined fixtures. | No apply route, crash-recovery write path, or Go backend parity yet. |
| Generated-files/review readiness | 3 | 4 | Existing `file_change`/TurnChangeSummary/ChangeInspector surfaces can show plan status without fake diffs or apply controls. | Full generated-files confirmation flow and destructive restore UI remain P4C. |
| Maintainability | 4 | 4 | Kun/Reasonix ledgers, conflict decision D-0008, specs 08/09, Go conformance, and scorecard now distinguish plan-only P4B from P4C apply. | Release-strength claim still needs packaging and desktop QA. |

Claim:
Analytix is stronger than the pre-P4B state because checkpoint/rewind is now a
user-auditable runtime/UI boundary rather than only an oracle: code-only,
conversation-only, and combined rewind plans can be generated and displayed
without silently modifying user data.

Decision:
Accept the P4B scoped plan-only slice as closed for contract/test/UI-display
behavior. Do not treat this as full Kun git rollback parity, full Reasonix
destructive rewind parity, Go runtime parity, Rust adoption, or release/G0
closure.

Follow-up:
Move next to P4C destructive restore + desktop QA if product safety is the
priority. G0 Go oracle inventory remains future conformance work and should
not scaffold a Go backend in this P4B slice.

## 2026-06-20 - P4C Confirmed Rewind Restore Apply

Branch:
local `codex/p4c-confirmed-rewind-restore-apply` worktree.

Upstream sources:
Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e` and `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928` remain the P4B upstream
checkpoint for this local code slice.

Impacted dimensions:
Safety and privacy, runtime contract reliability, generated-files/review
readiness, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Safety and privacy | 4 | 4 | P4C adds explicit confirmation, P4B-plan validation, `axrr_` rescue records before mutation, whole-operation preflight blocking, snapshot content hash revalidation, parent/final symlink blocking, lock-time revalidation, staged/untracked/missing snapshot blocking, and idempotency. Real Electron smoke launched and rendered the app with runtime health OK. | Score stays 4 until real Electron live apply/crash-restart QA and production snapshot capture decisions are complete. |
| Runtime contract reliability | 4 | 4 | Added shared apply schema, route/service, additive audit events, IPC allow-list, renderer provider method, and focused service/route tests for code/conversation/combined apply. | Go backend parity remains future G5 conformance. |
| Generated-files/review readiness | 4 | 4 | Existing ChangeInspector and TurnChangeSummary plan displays now expose confirmation controls without a new shell or upstream protocol. | A live desktop fixture with an actual `meta.rewindPlan` block remains required before release closure. |
| Maintainability | 4 | 4 | Kun/Reasonix ledgers, conflict decision D-0009, implementation plan, Go conformance, focused tests, regressions, and typechecks document accepted/deferred/rejected scope. | Spec 07 release/push blockers still apply: verified analytix remote, release URL/repo metadata, signing/notarization, Windows NSIS, packaged QA, live apply, and crash/restart QA remain gates. |

Claim:
Analytix is stronger than the pre-P4C state because checkpoint/rewind now has a
confirmed destructive apply boundary instead of only a plan: file mutations are
rescued, preflighted, audited, and idempotent; conversation rewind remains
append-only and non-corrupting.

Decision:
Accept the P4C scoped code behavior and desktop smoke. Do not treat this as
full release readiness, full production snapshot automation, Go runtime parity,
Rust adoption, or permission to expose Kun/Reasonix public identity.

Follow-up:
After P4C desktop smoke and safety fixes, move to G0/G5 Go conformance inventory
if the priority is runtime parity. Keep Go work conformance-only until the
TypeScript oracle is stable; live desktop apply, crash/restart, packaged QA, and
Spec 07 release/push blockers remain release gates.

## 2026-06-20 - G0/G5 Conformance Inventory Runtime Baseline

Branch:
local `codex/g0-g5-conformance-inventory-runtime-baseline` from
`codex/p4c-confirmed-rewind-restore-apply` at `b3e1674`.

Upstream sources:
Kun 0.2.13 lineage baseline; Kun `master`
`8602476c5c449b4561473ad5f31081ee93dc782e`; Reasonix `main-v2`
`be67a498adcaed6e33dcf3cbd25395e9cbccd6fa`.

Impacted dimensions:
Runtime contract reliability, provider/cache/cost, tool and MCP reliability,
safety and privacy, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Full runtime suite now passes after strict attribution of the 3 loop baseline failures; G0 inventory freezes thread/session, SSE replay, checkpoint/rewind, plan/goal, history repair, usage, cache, and provider parsing surfaces. | Go route fixtures and cross-backend comparison still need implementation before parity. |
| Provider/cache/cost | 4 | 4 | Prompt-token compaction tests now respect the anti-inflation trust factor while preserving normal/aggressive/force pressure behavior. | Live provider credential matrix and cost reconciliation remain missing. |
| Tool and MCP reliability | 4 | 4 | ToolKind persistence test now matches the current post-file-change final-answer contract; MCP malformed schema and denied approval oracles remain part of the G4/G5 fixture list. | Cross-backend tool-call SSE fixtures are still needed. |
| Safety and privacy | 4 | 4 | Inventory keeps P4C confirmed apply, rescue-before-mutation, append-only audit, sandbox/approval, and trace/privacy fixtures as G5 oracles. | Live desktop apply, crash/restart recovery, packaged QA, and production snapshot-capture decision remain release gates. |
| Maintainability | 4 | 4 | Kun/Reasonix sync ledgers, absorption ledger, conflict decision D-0010, spec 08, Go conformance, and this scorecard now agree that G0/G5 inventory is conformance-only and not a Go scaffold. | Final release closure still depends on Spec 07 blockers and verified analytix remote. |

Claim:
Analytix is stronger than the pre-inventory state for runtime governance:
baseline runtime tests are green, stale tests were updated to the current
contracts instead of weakening behavior, and future Go work now has a concrete
oracle inventory.

Decision:
Accept the G0/G5 conformance inventory baseline as closed for documentation and
runtime-suite baseline. Do not treat this as Go runtime parity, Go scaffold,
full Kun/Reasonix superiority, or release readiness.

Follow-up:
Convert the missing G5 oracle gaps into deterministic route/cross-backend
fixtures before starting G1/G2 implementation. Keep live desktop apply,
crash/restart, packaged QA, signing/notarization, Windows NSIS, release
metadata, and verified analytix remote as release blockers.

## 2026-06-20 - Reasonix Goal/Control Delta Oracle

Branch:
local `codex/reasonix-goal-control-delta-oracle` from
`codex/g0-g5-conformance-inventory-runtime-baseline` at `96b43ed`.

Upstream sources:
Reasonix `main-v2` refreshed to
`bc8249c307b261ef7ad05a0d2d1409c7b83b95ff`; requested merge point
`c23d40f74ca998941238154d6c0422676b139673` remains covered. Kun recheck:
`master` `8602476c5c449b4561473ad5f31081ee93dc782e`, `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928`, `v0.2.13` peeled
`2ba8decc2f56862e7f677fcf89bbc3d402ec3a23`, `v0.2.14` peeled
`8f2040349fba47fcd8e8b94f50b131943af839b2`.

Impacted dimensions:
Runtime contract reliability, provider/cache/cost, tool and MCP reliability,
safety and privacy, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Reasonix `dbaea843` goal FSM extraction is absorbed as internal `GoalControlMachine` state while keeping `ThreadGoal`, `goal_updated`, HTTP/SSE, and renderer contracts unchanged. | Cross-backend Go loop fixtures remain pending. |
| Provider/cache/cost | 4 | 4 | A direct oracle now proves inflated provider `promptTokens` above `PROMPT_TOKEN_TRUST_FACTOR` are ignored for compaction pressure. | Live provider credential matrix and cost reconciliation remain missing. |
| Tool and MCP reliability | 4 | 4 | A direct oracle now proves empty final text after `file_change` gets one recovery retry and then a deterministic `empty_post_tool_continuation` failure. | Route-level TS/Go tool-call SSE comparison remains pending. |
| Safety and privacy | 4 | 4 | Blocked-audit guidance stays in analytix goal instructions; no Reasonix marker, sidecar, CLI/settings/event shape, Kun bridge, or old identity is exposed. | ApprovalManager drift is deferred to a separate approval-control batch. |
| Maintainability | 4 | 4 | Reasonix delta is split into absorbed (`dbaea843`), no-op (`bb06f5b4`), and deferred (`726036bd`) items; docs/spec/ledger/conflict records agree that `bc8249c3` is the post-4f515da goal/control baseline. | Release-strength maintainability still needs packaged QA, latest upstream currentness checks, and verified analytix remote. |

Claim:
Analytix is stronger than the G0/G5 inventory baseline because two previously
implicit runtime contracts are now direct oracle tests, and Reasonix's goal FSM
separation has been adapted without leaking upstream public protocol.

Decision:
Accept the scoped goal/control delta as closed for code and documentation after
the required validation suite passes. Do not treat this as full Reasonix
approval/control parity, Go runtime parity, full Kun 8602476 parity, Rust
adoption, or release readiness.

Follow-up:
Convert the approvalManager drift into a future approval-control batch if it
still matters after route/renderer regressions are scoped. Keep provider/settings
schema and bridge canonical-domain checks in the release gate.

## 2026-06-20 - Post Goal-Control Reasonix Store-Sidecar Refresh

Branch:
local `codex/reasonix-goal-control-delta-oracle` at `4f515da`.

Upstream sources:
Reasonix `main-v2` refreshed from post-4f515da baseline
`bc8249c307b261ef7ad05a0d2d1409c7b83b95ff` to
`d02457ee7802256d66b9860276a4f00cd5baea56`. Kun recheck:
`master` `8602476c5c449b4561473ad5f31081ee93dc782e`, `develop`
`ab24a77f0f68fcb1b2c160361e4c50cd92b02928`, `v0.2.13`
`201a1469ffbd911f6b95d450b78471e51b42acae`, and `v0.2.14`
`06be05d76223208724c07301fa0f830641a06e6f`.

Impacted dimensions:
Runtime contract reliability, safety and privacy, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Reasonix `98a57ded` is a store-sidecar path authority refactor with no intended behavior change. Analytix does not change current TS runtime contracts or renderer-visible storage behavior for this refresh. | Future Go G2/G5 store fixtures must prove equivalent session, goal, checkpoint, jobs, and cleanup paths behind analytix contracts. |
| Safety and privacy | 4 | 4 | No Reasonix sidecar filenames, renderer protocol, CLI/settings shape, Kun bridge, or old identity is exposed. | Production identity/protocol scans and Spec 07 release blockers still gate release closure. |
| Maintainability | 4 | 4 | `reasonix-sync.md`, `absorption-ledger.md`, and this scorecard distinguish record-only store-sidecar remote `d02457ee`, post-4f515da baseline `bc8249c3`, and historical P3A checkpoint `be67a498`; later refreshes moved through `48e5b990` and current `c202f970`. | Packaged QA, live provider matrix, signing/notarization, Windows NSIS, release URL/repo metadata, and verified analytix remote remain blockers. |

Claim:
This is a currentness refresh only. It prevents stale-scorecard confusion but
does not make a stronger product claim.

Decision:
Accept the refresh as record-only. Do not absorb Reasonix `internal/store` into
the current TypeScript runtime and do not start Go/Rust runtime work in this
batch.

Follow-up:
Keep the next implementation choice between a focused Reasonix approvalManager
batch and release QA evidence. Because release closure remains blocked, release
QA should be completed before any public readiness claim.

## 2026-06-20 - Post 4f47031f Upstream Currentness Refresh

Branch:
local `codex/reasonix-goal-control-delta-oracle` at `4f47031f`.

Upstream sources:
Reasonix `main-v2` refreshed from store-sidecar checkpoint
`d02457ee7802256d66b9860276a4f00cd5baea56` to
`48e5b990671ca1e579b895080e74babc1e17c333`. Kun recheck: `master`
`8602476c5c449b4561473ad5f31081ee93dc782e`, `develop`
`9605e20f422c90054d930e4f1a0000886b353895`, `v0.2.13`
`201a1469ffbd911f6b95d450b78471e51b42acae`, and `v0.2.14`
`06be05d76223208724c07301fa0f830641a06e6f`.

Impacted dimensions:
Runtime contract maintainability, remote-entry safety, Windows release safety.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime maintainability | 4 | 4 | Reasonix `3e625b91` introduces a SessionAPI driving port and migrates the bot gateway to a narrow lifecycle/turn/approval interface. This refresh identified the next control-port batch, which the following scorecard section marks as scoped-absorbed. | Go G1/G2 still needs cross-backend fixtures before any Go parity claim. |
| Safety and privacy | 4 | 4 | The desired benefit is preventing remote/bot-like entry points from reaching goal/checkpoint/memory surfaces. Current analytix public protocol remains unchanged. | The following control-port absorption section records the negative boundary tests; live Connect Phone/schedule QA remains open. |
| Cross-platform release | 3 | 3 | Kun `d09d52b` was classified here as a later analytix-native `must absorb` release batch; the P0 product-entry/installer section below records that absorption. | Windows NSIS machine verification, signing, release metadata, and packaged launch QA remain blockers. |

Claim:
This is a currentness refresh only. It identifies the next Reasonix
control-port implementation batch and the Kun Windows installer item that was
later absorbed by the P0 product-entry/installer section below; it does not
make a stronger product or release-readiness claim.

Decision:
Do not close final analytix completion from this refresh. Next implementation
should absorb the Reasonix SessionAPI/control-port boundary behind analytix
contracts. The later P0 product-entry/installer section records the Windows
release-safety absorption while keeping Windows machine QA open.

## 2026-06-20 - Reasonix Remote-Entry Control-Port Absorption

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Requested Reasonix `main-v2` baseline
`48e5b990671ca1e579b895080e74babc1e17c333`, target commit
`3e625b91d9a7ee265d5e7fb88ed8f2fdae731d35`; current Reasonix `main-v2`
recheck `c202f97035cd353c4bf3bbd2dc6e94ed22710e67`. Kun recheck: master
`8602476c5c449b4561473ad5f31081ee93dc782e`, requested develop checkpoint
`9605e20f422c90054d930e4f1a0000886b353895`, current develop
`247076f297170c3d0c558baffb073c629894faea`.

Impacted dimensions:
Runtime contract reliability, safety and privacy, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | `RemoteEntryControlPort` gives remote/bot-like runtime entrants a stable analytix-owned lifecycle/turn/approval/user-input surface without changing public HTTP/SSE or renderer bridge contracts. | No Go backend exists yet; future G1/G2 must compare this port with route/SSE behavior. |
| Remote-entry safety | 1 | 4 | `remote-entry-control-port.test.ts` proves by type-level negative assertions and runtime key checks that remote entrants cannot access goal, checkpoint, memory, raw session storage, thread service, thread store, or tool host surfaces. Full runtime suite also preserves denied approval no-execute, user-input, and goal-loop regressions. | Live Connect Phone/schedule QA should later exercise this boundary if those entry points are refactored onto it. |
| Maintainability | 4 | 4 | Reasonix interface segregation is absorbed through `packages/runtime/src/ports/remote-entry-control.ts` and `services/remote-entry-control-port.ts`, while ledgers document current upstream drift and reject Reasonix public protocol, Go scaffold, and renderer bridge changes. | Current Reasonix post-target serve/acp/cli port migrations are record-only until a behavior delta appears; release-strength maintainability still needs Spec 07 evidence. |

Claim:
Analytix is stronger than both the previous analytix runtime and the direct
Reasonix import path for this scoped surface: analytix now has the same
interface-segregation benefit, but with a stricter desktop-owned boundary that
does not expose Reasonix naming/protocol and explicitly excludes unrelated
goal/checkpoint/memory/storage control planes.

Decision:
Accept the scoped Reasonix control-port absorption. This closes the requested
TypeScript runtime code/oracle batch, not Go parity, full Reasonix controller
parity, release readiness, or Windows installer machine QA.

Follow-up:
The P0 product-entry/installer section below now records Kun `d09d52b`
absorption with analytix-native NSIS names; Windows machine verification
remains required. Future Go G1/G2 can begin only as conformance work that keeps
renderer/preload/main contracts unchanged and reproduces the new remote-entry
boundary.

## 2026-06-20 - P0 Product-Entry Correction + Kun Installer Process-Stop

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`; Kun `develop`
`247076f297170c3d0c558baffb073c629894faea`; Kun tags `v0.2.13`
`201a1469ffbd911f6b95d450b78471e51b42acae` and `v0.2.14`
`06be05d76223208724c07301fa0f830641a06e6f`; focused Kun installer commit
`d09d52b0ceb11bcdd0bc85f7dcb6fe49844a23ff`. Requested Reasonix checkpoint
`c202f97035cd353c4bf3bbd2dc6e94ed22710e67`; historical 2026-06-20 observed
Reasonix `main-v2` `5d1ad2ae8cb7a0dbc8775fb3f05e6c204627c2b1` and current
2026-06-21 recheck `49c14762b7da9234525e717830e39a64a2220911` are record-only
drift for this P0 batch.

Impacted dimensions:
Product workflow governance, cross-platform release safety, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product workflow governance | 2 | 4 | Workflow/Create Loop is no longer a top-level analytix route/sidebar/workbench entry when targeting Kun 0.2.13/0.2.14. `Sidebar.test.ts`, `Workbench.route-surface.test.ts`, and `chat-store-app-actions.test.ts` guard the route surface. | Future Workflow exposure still needs an independent spec and desktop QA. |
| Cross-platform release | 3 | 4 | `build/installer.nsh` and `electron-builder.config.cjs` absorb Kun `d09d52b0` as analytix-native NSIS process-stop logic using `ANALYTIX_INSTALLER_*` names. | Windows machine NSIS upgrade verification remains a release blocker, so score does not reach 5. |
| Maintainability | 4 | 4 | Ledgers/spec/scorecard now distinguish Kun 0.2.13/0.2.14 baseline, Kun master/current Workflow, Kun develop speech/SSE/UI drift, and Reasonix post-`c202f970` SessionAPI port drift. | Full release-strength maintainability still needs Spec 07 evidence, live provider matrix, packaging QA, and signed/notarized artifacts. |

Claim:
Analytix is stronger than the pre-P0 state because it corrects an overexposed
product entry before further upstream absorption and closes a concrete Windows
upgrade-safety gap without leaking Kun identity.

Decision:
Accept the P0 product-entry correction and Kun installer process-stop code
slice. Do not claim release readiness until Windows NSIS machine QA passes.

Follow-up:
Prioritize Kun develop `afafa226` SSE IPC reconnect/throttle as the next
release-safety candidate if current analytix has the same failure. Speech/local
Whisper and UI drift require separate scoped reviews. Reasonix approvalManager
and Go G1 should start only after this P0 closure and release blockers are
either closed or explicitly deprioritized.

## 2026-06-21 - Long-Term Absorption Proof Gate

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Kun `master` `8602476c5c449b4561473ad5f31081ee93dc782e`, Kun `develop`
`247076f297170c3d0c558baffb073c629894faea`, Kun `v0.2.13`
`201a1469ffbd911f6b95d450b78471e51b42acae`, Kun `v0.2.14`
`06be05d76223208724c07301fa0f830641a06e6f`, and Reasonix `main-v2`
`49c14762b7da9234525e717830e39a64a2220911`.

Impacted dimensions:
Product workflow governance, provider/cache/cost, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product workflow governance | 4 | 4 | Spec 08, Spec 09, D-0012, and the stage-closure playbook now make target-version entry position a required proof item for Kun absorption. | Future Workflow automation still needs a new spec before exposure. |
| Provider/cache/cost | 4 | 4 | Spec 09, D-0013, and `packages/runtime/tests/provider-cache-proof.test.ts` now prove stable prefix hash, canonical tool schema hash, provider/model/endpoint attribution, DeepSeek/Responses/Anthropic cache parsing, and unsupported-provider no-guess fallback with fixtures. | Live provider credentials, packaged QA, and full non-regression gates remain needed before final Reasonix cache superiority claims. |
| Maintainability | 4 | 4 | `重构升级方案.md`, specs, ledgers, and `stage-closure-playbook.md` now require stage closure instead of one-off prompt churn. | Actual future stages still need code, validation, scorecard updates, and commit decisions. |

Claim:
This is a governance plus fixture-proof update, not a live-provider superiority
claim. It prevents future upstream absorption from claiming completion without
product entry parity or Reasonix engine benchmark evidence, and it gives Go
G0/G3/G5 a concrete TypeScript cache/provider oracle.

Decision:
Accept the methodology hardening and the first fixture-backed Reasonix
cache/provider proof closure. Next implementation work should close the broader
validation gates for this dirty P0/installer/cache batch, then move to live
provider matrix evidence or Kun develop SSE release-safety triage.

## 2026-06-21 - Reasonix Agent-Kernel Five-Batch Route

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` current recheck
`49c14762b7da9234525e717830e39a64a2220911`; Kun baseline context remains
`v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master` `8602476`, and `develop`
`247076f`.

Impacted dimensions:
permission/evidence/Goal, AutoResearch/long-running task,
token economy/tool schema/history-memory/cache, task/parallel/background/
coordinator, Go runtime kernel, maintainability.

Scores:

| Reasonix agent-kernel row | Current score | Route recorded | Proof required before parity or superiority |
| --- | ---: | --- | --- |
| Task closure and permission kernel | 1 | First future batch: `permission Gate`, approval posture `ask` / `auto` / `yolo`, plan approval vs tool approval, `complete_step`, evidence ledger, Goal state machine, blocked-state detection, and headless/subagent approval rules. | Focused fixtures plus scorecard evidence that permissions, completion proof, Goal state, approvals, user input, and sandbox behavior are at least Reasonix parity and safer for desktop. |
| Long-running task system | 0 | Second batch: `/goal --research`, AutoResearch project-local state, `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, `iteration_log.jsonl`, and requirement-by-requirement evidence audit. | Restart/resume and audit fixtures prove long work is durable without polluting `REASONIX.md`, `AGENTS.md`, stable system prefix, tool schema, or renderer-visible Reasonix protocol. |
| Tool surface and context economy | 2 | Third batch builds on existing P2.2 cache/provider fixture proof with token economy mode, `connect_tool_source`, dynamic tools, stable tool schema, history/memory retrieval, compaction archive, and cache diagnostics. | Multi-provider matrix, live cache/cost evidence, and no unsupported-provider 0% hit/all-miss misclassification. |
| Collaborative execution model | 0 | Fourth batch: `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, subagent transcript continuation/fork, and nested event rendering. | Product-grade permission-gated execution; `subagents.enabled` or `delegate_task` alone cannot raise the score. |
| Go runtime kernel | 0 | Fifth batch: Provider Registry, Tool Registry, Controller, Session, Event Sink, Job Manager, Permission Gate, MCP Client, Memory/History Retrieval, and Goal Runtime behind TS oracle G0-G6. | G1-G6 conformance, desktop QA, rollback, release QA, and scorecard evidence; no renderer/preload/main bridge change. |
| Maintainability | 4 | `重构升级方案.md`, specs 08/09, D-0014, upstream ledgers, Go conformance, and this scorecard now agree on the five-batch order. | Future code batches still need implementation, tests, benchmark evidence, live provider/desktop QA where applicable, and commit decisions. |

Claim:
This is a docs/spec governance closure. It fills the missing Reasonix
agent-kernel route and prevents future claims that sub-agent toggles, cache
fixtures, or Go scaffold alone equal Reasonix parity. It does not claim
Reasonix parity, Reasonix superiority, release readiness, or final product
completion.

Decision:
Accept the five-batch route as the benchmark baseline. The next implementation
batch should target task closure and permission kernel. Collaborative execution
and Go runtime kernel stay deferred until earlier proof gates are satisfied.

## 2026-06-21 - D-0014 Batch 1 Task-Closure and Permission-Kernel TS Proof

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` governance baseline
`49c14762b7da9234525e717830e39a64a2220911`. No new Reasonix drift is mixed
into this implementation batch.

Impacted dimensions:
permission/evidence/Goal, approval posture, plan/tool approval split,
headless/subagent approval, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Task closure and permission kernel | 1 | 3 | `complete_step` now records `ThreadGoal.evidenceLedger`; `update_goal complete` and `ThreadService.setGoal(... complete)` require evidence; focused tests cover denied no-execute, ask/auto/yolo posture mapping, plan/tool separation, blocked-state event/status, and headless/subagent policy inheritance. | This is scoped TS proof, not full Reasonix parity. Route approval/user-input fixtures, renderer approval/review/generated-files non-regression, broader runtime/app gates, and release evidence remain open. |
| Goal state reliability | 2 | 3 | Goal completion cannot silently succeed without evidence, and blocked-state exhaustion records durable `goal_updated` plus warning event evidence. | At this D-0014 checkpoint, requirement-by-requirement audit and project-local AutoResearch state were not yet implemented; D-0246 later clears project-local AutoResearch state without claiming full Reasonix project protocol parity. |
| Collaborative execution safety | 0 | 1 | Subagent/headless execution now has a focused no-bypass proof under inherited `approvalPolicy: never`. | `task`, `parallel_tasks`, background jobs, planner/executor Coordinator, transcript fork/continuation, and nested event rendering remain deferred to Batch 4. |
| Go runtime kernel readiness | 0 | 1 | Go conformance now has concrete TS oracle families for permission gate, evidence ledger, `complete_step`, blocked-state, and headless policy inheritance. | No Go scaffold exists or is authorized. G1-G6 cannot start until TS oracle/conformance and earlier batches stay green. |
| Maintainability | 4 | 4 | D-0014, sync ledgers, conformance, scorecard, and release gate distinguish scoped implementation evidence from parity/superiority claims. | Dirty worktree is still multi-theme; final commit grouping and full validation remain pending. |

Claim:
Analytix is stronger than the immediate pre-Batch-1 state for scoped task
completion proof: goals can no longer be completed without evidence through
the model tool or service path. This does not establish Reasonix parity,
Reasonix superiority, full Batch 1 closure, release readiness, or Go runtime
readiness.

Decision:
Accept the scoped TypeScript Batch 1 proof and keep the next implementation
focus on remaining permission/approval/user-input/review non-regression before
moving to AutoResearch or collaborative execution.

Follow-up:
Run full validation gates, update commit boundary, and then continue with the
remaining Batch 1 proof or Batch 2 AutoResearch only after Batch 1 blockers are
explicitly closed or deferred.

## 2026-06-21 - D-0014 Batch 2 AutoResearch Project-Local State TS Proof

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` governance baseline
`49c14762b7da9234525e717830e39a64a2220911`. No new Reasonix drift is mixed
into this implementation batch.

Impacted dimensions:
AutoResearch/long-running task, requirement evidence audit, restart/resume,
stable prefix/tool schema isolation, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Long-running task system | 0 | 2 | `/goal --research` now creates an existing goal/chat-surface research goal with `.analytix/autoresearch/<threadId>/` state, including `task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl`. | This is scoped TS proof, not full Reasonix AutoResearch parity. Desktop crash/restart, live provider research loops, background jobs, planner/executor, and product-level evidence review remain open. |
| Requirement-by-requirement evidence audit | 0 | 3 | `complete_step requirement_id` updates requirement status and `update_goal complete` rejects research completion until every requirement has evidence. | Route-level and renderer-visible evidence review remain future work. |
| Research direction tracking | 0 | 2 | `record_research_direction` writes attempted directions and iteration-log records without exposing Reasonix protocol. | Batch 3 still needs history/memory retrieval and token economy integration. |
| Stable prefix/tool schema isolation | 2 | 3 | Loop fixtures prove AutoResearch paths are per-turn context; state does not enter stable system prefix/tool schema and no `REASONIX.md` or `AGENTS.md` pollution is introduced. | Batch 3 must keep this invariant while adding dynamic tools and `connect_tool_source`. |
| Go runtime kernel readiness | 1 | 2 | Go conformance now has concrete TS oracle families for `/goal --research`, project-local state, direction tracking, restart/resume, and requirement audit. | No Go scaffold exists or is authorized. G1-G6 cannot start until TS oracle/conformance and earlier batches stay green. |
| Maintainability | 4 | 4 | D-0014, sync ledger, conformance, scorecard, and release gate now distinguish scoped Batch 2 proof from parity/superiority claims. | Dirty worktree is still multi-theme; final commit grouping and full validation remain pending. |

Claim:
Analytix is stronger than the immediate pre-Batch-2 state for scoped long-task
proof: long-running research goals now have durable project-local state,
attempted-direction records, and requirement-level evidence gates. This does
not establish Reasonix parity, Reasonix superiority, release readiness, or Go
runtime readiness.

Decision:
Accept the scoped TypeScript Batch 2 proof and require Batch 3 to focus on
tool/context economy and multi-provider cache diagnostics before any
collaborative execution or Go runtime work.

## 2026-06-21 - D-0014 Batch 3 Tool Source and Context Economy TS Proof

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` governance baseline
`49c14762b7da9234525e717830e39a64a2220911`. No new Reasonix drift is mixed
into this implementation batch.

Impacted dimensions:
token economy/tool schema/history-memory/cache, dynamic tool source lifecycle,
multi-provider cache diagnostics, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Tool surface and context economy | 2 | 3 | Dynamic source lifecycle is now fixture-backed: `connectToolSource` / `disconnectToolSource`, connection-order-stable canonical catalog fingerprints, provider-owned order preservation, source diagnostics, and runtime cache diagnostics for `toolSourceChanged`. Existing token economy, history hygiene, memory, compaction, usage, and provider-cache fixtures remain green. | This is scoped TS proof, not full Reasonix parity. Live MCP operations, retrieval ranking, source UI, and long-session desktop evidence remain open. |
| Stable tool schema | 3 | 4 | Dynamic source connection order no longer changes canonical tool catalog fingerprints, and source metadata is excluded from model-visible tool schemas. | Batch 4 task/subagent tools must preserve the same invariant. |
| Cache diagnostics | 3 | 4 | `CacheDiagnostics` reports source lifecycle separately from `prefixChanged`, keeps unsupported providers unknown, and avoids raw prompt/tool-description leakage. | Live provider cache/cost matrix remains a blocker before superiority claims. |
| Multi-provider safety | 3 | 3 | Provider-cache proof still covers DeepSeek, OpenAI-compatible/Responses, Anthropic/MiMo-style usage, and unsupported telemetry fallback without request/body/header/stream rewrites. | Credentialed live provider matrix and write-inline/scheduled detector/provider probe end-to-end proof remain release blockers. |
| Go runtime kernel readiness | 2 | 3 | Go conformance now has concrete TS oracles for Tool Registry source lifecycle, stable tool schema, source diagnostics, and cache diagnostics. | No Go scaffold exists or is authorized; G1-G6 still require implementation, rollback, desktop QA, and release evidence. |
| Maintainability | 4 | 4 | D-0014, sync ledger, conformance, scorecard, and release gate now distinguish scoped Batch 3 proof from parity/superiority claims. | Dirty worktree is still multi-theme; commit grouping remains pending, while local full validation passed on 2026-06-21. |

Claim:
Analytix is stronger than the immediate pre-Batch-3 state for scoped
tool/context economy: dynamic source lifecycle can be observed without
destabilizing model-visible tool schemas or stable cache-prefix diagnostics.
This does not establish Reasonix parity, Reasonix superiority, release
readiness, or Go runtime readiness.

Decision:
Accept the scoped TypeScript Batch 3 proof. Batch 4 collaborative execution may
only start while Batch 1 permission/evidence gates and Batch 3 tool/context
gates stay green.

## 2026-06-21 - D-0014 Batch 4 Delegated Collaboration Evidence TS Proof

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` governance baseline
`49c14762b7da9234525e717830e39a64a2220911`. No new Reasonix drift is mixed
into this implementation batch.

Impacted dimensions:
task/parallel/background/coordinator, delegated child execution, nested event
projection, Goal evidence handoff, maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Collaborative execution model | 0 | 2 | Existing `delegate_task` now has evidence-ledger handoff for active parent Goals; nested child metadata preserves `evidenceLedgered`; maxParallel queueing, queued abort, failure, interruption, and fan-out fixtures remain green. | This is scoped delegated-execution proof, not Reasonix `task` / `parallel_tasks` parity. First-class task APIs, background jobs, planner/executor Coordinator, transcript continuation/fork, and desktop nested QA remain open. |
| Collaboration safety | 1 | 3 | Batch 1 approval inheritance plus Batch 4 evidence handoff prevent child work from being a detached process amplifier. | Broader route/user-input/background approval gates remain required before productization. |
| Nested event rendering | 1 | 2 | Runtime event schema, reducer, and renderer mapper preserve child evidence metadata without a new top-level UI. | Full visual nested event rendering QA is still pending. |
| Go runtime kernel readiness | 3 | 3 | Go conformance now has TS oracle requirements for delegated evidence handoff, nested child metadata, fan-out, queue/cancel/failure, and product-boundary deferrals. | No Go scaffold exists or is authorized; Job Manager/Controller G5 work remains pending. |
| Maintainability | 4 | 4 | D-0014, sync ledger, conformance, scorecard, and release gate now distinguish scoped Batch 4 proof from parity/superiority claims. | Dirty worktree is still multi-theme; commit grouping remains pending, while local full validation passed on 2026-06-21. |

Claim:
Analytix is stronger than the immediate pre-Batch-4 state for scoped delegated
collaboration: completed child work can now become parent Goal evidence and
nested metadata survives replay/projection. This does not establish Reasonix
collaborative-execution parity, Reasonix superiority, release readiness, or Go
runtime readiness.

Decision:
Accept the scoped TypeScript Batch 4 proof. The TS oracle validation is green
for this dirty worktree; the next section opens Batch 5 only as TS shadow
conformance. Go implementation/scaffold work remains deferred until it can
preserve renderer/preload/main contracts without creating an unauthorized
backend path.

## 2026-06-21 - D-0014 Batch 5 Go Runtime G1 Shadow Scaffold

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` governance baseline
`49c14762b7da9234525e717830e39a64a2220911`. No new Reasonix drift is mixed
into this implementation batch.

Impacted dimensions:
Go runtime kernel, backend-neutral conformance, health/config/capabilities,
maintainability.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go runtime kernel readiness | 3 | 4 | `go-runtime-kernel-conformance.ts` maps the ten kernel components to TS authority and oracle tests; the shared oracle fixture, `go-runtime-conformance.test.ts`, and `packages/runtime-go` validate `/health`, `/v1/runtime/info`, and `/v1/runtime/tools` G1 shadow route contracts against TS status codes, auth, JSON shape, tool diagnostics, and capability registry semantics. | No default Go backend, Electron supervisor integration, cross-backend runner, G2+ implementation, rollback, or desktop QA exists yet. |
| Backend-neutral safety | 4 | 4 | Manifest and fixture flags require `shadow-conformance-only`, unchanged renderer/preload/main bridge, unchanged `analytix serve`, no Reasonix public protocol, no default Go backend, and no renderer-visible Go routes. | Future Go G2+ must keep these invariants under actual process supervision. |
| Goal Runtime audit readiness | 3 | 4 | AutoResearch now rejects unknown `requirement_id` evidence without writing findings, tightening the TS oracle future Go must match. | Full Go Goal Runtime and Job Manager parity remain pending. |
| Maintainability | 4 | 4 | D-0014, conformance, scorecard, and release gate now distinguish isolated Go G1 shadow scaffold from a default Go backend. | G2+ route replay, provider/cache shadow parity, rollback, and packaged QA remain pending. |

Claim:
Analytix is stronger than the immediate pre-Batch-5 state for Go readiness:
the Go kernel now has both a machine-checked TS oracle manifest and an isolated
G1 shadow implementation for the first health/config/capabilities routes. This
does not establish Go runtime parity, Reasonix parity, Reasonix superiority,
release readiness, or a default backend path.

Decision:
Accept the G1 shadow scaffold proof. Future Go work may proceed only as G2+
fixture replay or shadow implementation against this manifest and shared
oracle, and must not change renderer/preload/main contracts, `analytix serve`,
top-level `runtime` settings, public event semantics, or release/backend
defaults.

## 2026-06-21 - Reasonix Latest Currentness + G2 Shadow Replay Evidence Draft

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` latest
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e`, advanced from the D-0014
governance baseline `49c14762b7da9234525e717830e39a64a2220911`. Kun baseline
remains `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master` `8602476`, and
`develop` `247076f`.

Impacted dimensions:
MCP/tool-source parity readiness, provider/config parity readiness, Go G2
route replay readiness, product-entry governance, release evidence.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP/tool-source parity readiness | 3 | 4 | Reasonix `45367085` identifies codebase-memory MCP auto-indexing as a future fixture target: cwd-aware stdio indexers may need workspace-root cwd, low-priority scheduling, and background startup under shared hosts. This stage adds generic MCP/tool lifecycle fixture coverage for connect/disconnect/reload/cancel/error and secret-safe diagnostics. | Specific codebase-memory auto-index behavior, low-priority process policy, live MCP operations, and desktop QA remain missing. |
| Provider/cache/cost readiness | 4 | 4 | Reasonix `d9e453a1` moves MiMo built-ins to custom providers, giving a provider-config comparison point. Analytix production scan shows Xiaomi/MiMo presets still exist in current product code, so this stage rejects preset removal. | Live provider credential matrix and cost reconciliation remain missing; future provider batch must decide whether Xiaomi/MiMo presets are product requirements or migration debt. |
| Go G2 route replay readiness | 0 | 2 | Shared G2 oracle now covers thread list/search/archive, read/update, side fork, resume-thread, and SSE `since_seq` replay/caught-up frames. TypeScript route replay and Go shadow replay both pass against the same fixture; temporary official Go `go1.26.4` was SHA-verified and removed after `go test ./...`. | This is shadow-only. No desktop supervisor backend flag, rollback, G3 provider streaming, G4 tools, G5 loop, packaged Go QA, or default backend exists. |
| Product workflow governance | 4 | 4 | Kun refs are unchanged, route-surface guard tests pass, and production route scan has no top-level Workflow/openWorkflow hits in Sidebar, Workbench, AppRoute/store actions. | Future Workflow automation still needs an independent spec and desktop QA. |
| Release evidence readiness | 2 | 2 | Production scans passed for bridge domains, forbidden identity/protocol terms, default Go/backend/Rust/Tauri absence, and release metadata blockers were re-recorded. | Live provider matrix, packaged launch, Windows NSIS, signing/notarization, release URL/repo metadata, verified remote, and Go toolchain remain blockers. |

Claim:
This entry improves currentness, fixture-backed parity hardening, and G2
shadow replay readiness. It does not claim Reasonix latest full parity,
provider/cache superiority, live MCP parity, release readiness, or a default Go
backend path.

Decision:
Accept the scoped stage evidence. Future implementation may start from
Reasonix `91fe06db` currentness and the G2 route replay matrix, but must keep
Go shadow-only unless a later spec approves backend selection, preserve Kun
0.2.13 -> 0.2.14 product-entry baseline, and leave the live provider matrix
explicitly open until credentialed provider evidence exists.

## 2026-06-21 - Reasonix P0 Engine Parity + Kun Desktop Hardening

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` `91fe06db6177bb052fc8ae1a3081d60bfc104a4e`; Kun
`master` `8602476c5c449b4561473ad5f31081ee93dc782e`; Kun `develop`
`247076f297170c3d0c558baffb073c629894faea`; Kun `v0.2.13` `201a1469`;
Kun `v0.2.14` `06be05d`.

Impacted dimensions:
provider/cache/cost readiness, tool/MCP reliability, runtime contract
reliability, Go runtime kernel readiness, desktop shell smoothness, release
evidence.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache fixture readiness | 4 | 5 | Provider/cache proof covers DeepSeek top-level hit/miss, OpenAI-compatible/Responses cached tokens, Anthropic cache fields, reasoning tokens, canonical schema hash, stable tool order, unsupported unknown fallback, and sanitized request attribution. | Live provider credential matrix, write-inline/scheduled/probe live checks, and cost reconciliation remain missing. |
| MCP/tool lifecycle fixture readiness | 4 | 5 | MCP lifecycle and provider tests cover codegraph/codebase-memory known overrides, workspace-root cwd fallback, explicit cwd preservation, low-priority/background-start diagnostics, schema cache, cancel/timeout, and secret redaction. | Live MCP/indexer operation and desktop lifecycle QA remain missing. |
| Task/background/planner oracle readiness | 2 | 4 | Task job oracle and durable manager skeleton cover foreground/background continuation, wait/output/kill, parallel dependency/cycle validation, nested event metadata, permission inheritance, transcript continue/fork, and planner read-only tools. | Full productized background jobs, restart crash drill, and desktop nested cards remain missing. |
| Runtime SSE desktop stability | 2 | 4 | Main-process SSE IPC now batches pending events at 100ms, stops safely for destroyed renderers, and reconnects with flushed `since_seq`; focused tests pass. | Packaged long-thread desktop QA remains required. |
| Go G3/G4 readiness | 0 | 2 | G3/G4 TS fixtures and Go shadow output comparison pass; Go remains conformance-only. | No Go provider/tool implementation, G5 loop, rollback, packaged QA, or default backend. |
| Product workflow / shell baseline | 4 | 4 | Shell navigation controls are consolidated without adding Workflow/Create Loop or changing Code/Write/Settings/Plugins/Connect Phone/Schedule baseline. | Full desktop QA and deferred Local Whisper/tray menu remain open. |
| Release evidence readiness | 2 | 2 | Stage evidence is recorded and focused suites pass. | Full command gate, packaged app launch, Windows NSIS, signing/notarization, release metadata, verified remote, and live provider/MCP QA remain blockers. |

Claim:
Analytix now has scoped P0 fixture/oracle parity for the named Reasonix engine
surfaces and absorbs Kun desktop SSE/shell hardening without product-entry
regression. This does not establish live provider superiority, live MCP parity,
release readiness, or default Go backend readiness.

Decision:
Accept the scoped stage score changes only for fixture/readiness dimensions.
The next stage should move to live provider/MCP/desktop packaged QA, or Go G5
only after a fresh currentness check and without changing the default backend.

## 2026-06-21 - Reasonix 9e56 Currentness + Internal Runtime Parity Addendum

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2`
`91fe06db6177bb052fc8ae1a3081d60bfc104a4e..9e56c3276880ced538b3375329b8a0ebbe63a67b`.
Kun product-entry baseline remains `v0.2.13` -> `v0.2.14`.

Impacted dimensions:
MCP startup reliability, internal runtime task-job reliability,
provider-diagnostic safety, desktop shell safety, Go G5 readiness.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP startup fixture reliability | 4 | 5 | Runtime tests now cover retrying every failed MCP startup server and blocking late reconnect from reinstalling a suspended source, on top of existing codegraph/codebase-memory cwd/priority diagnostics. | Live MCP/indexer QA and Reasonix plugin protocol parity are not claimed. |
| Internal task-job runtime readiness | 3 | 4 | Durable task jobs now have authenticated internal `/v1/runtime/task-jobs/wait|output|kill` routes, bounded wait, partial-output read, kill, and 404 tests. | First-class Reasonix `task`, `parallel_tasks`, planner/executor Coordinator, restart crash drill, and desktop nested cards remain missing. |
| Provider diagnostic safety | 4 | 5 | Provider probe/model-list diagnostics redact secret-bearing request URLs, HTTP excerpts, and network errors; offline provider-cache oracle remains unchanged. | Live write-inline/scheduled/probe/provider matrix and cost reconciliation remain blocked by credentials. |
| Desktop shell safety | 3 | 4 | Dirty native-titlebar safe-area baseline was reviewed, focused-tested, and committed separately. | Packaged desktop QA, Local Whisper, and tray session menu remain deferred. |
| Go runtime kernel readiness | 2 | 2 | This stage adds only TS-owned G5 oracle inventory inputs; no Go code changed. | No full Go loop/jobs/cache/compaction/resume/interrupt, rollback, packaged QA, or default backend. |
| Product boundary governance | 4 | 4 | No top-level Workflow/Create Loop, Reasonix protocol, Kun identity, Rust/Tauri path, or default Go backend was added. | Full release evidence and live parity remain open. |

Claim:
Analytix is stronger than the immediate pre-9e56 state for scoped MCP startup
fixtures, internal task-job runtime control, and provider-probe diagnostic
safety. This does not establish full Reasonix task/planner parity, live MCP
parity, live provider/cache superiority, Go G5 parity, or release readiness.

Decision:
Accept the score changes only for fixture/internal runtime readiness. Keep live
provider/MCP/desktop/Go backend dimensions explicitly blocked until their
credentialed or packaged gates pass.

## 2026-06-21 - Collaborative Execution / Cache Curve / MCP Indexer / Go G5 Oracle Closure

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` `9e56c3276880ced538b3375329b8a0ebbe63a67b`; Kun
product-entry baseline `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master`
`8602476`, `develop` `247076f`.

Impacted dimensions:
collaborative execution, provider/cache efficiency, MCP/indexer reliability,
Go G5 readiness, desktop shell safety, product-entry governance.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Collaborative execution oracle readiness | 4 | 5 | Internal task/parallel contracts now pin permission gate, background, parent Goal evidence, transcript continue/fork, planner read-only, and `depends_on`; validation rejects duplicate/self/unknown/cycle cases. | No full Reasonix planner/executor Coordinator, restart/resume crash drill, or desktop nested-card QA. |
| Provider/cache fixture readiness | 5 | 5 | Offline cache curve guard adds Reasonix-style 90% tail threshold and bounded too-small-window allowed-low case on top of DeepSeek/Responses/Anthropic/reasoning/unsupported matrix. | Live provider/cache superiority score remains blocked by credentials and cost reconciliation. |
| MCP/indexer fixture readiness | 5 | 5 | Lifecycle oracle now covers retry-all failed startup servers, suspended-source tombstones, and codegraph/codebase-memory cwd/low-priority/backgroundStart variants. | Live MCP/indexer QA and Reasonix plugin protocol parity are not claimed. |
| Go G5 readiness | 2 | 3 | `go-g5-full-loop-oracle.json` and manifest binding freeze TS-owned full-loop/job/cache/compaction/resume/interrupt/MCP inventory. | No Go full loop implementation, Electron integration, rollback, packaged QA, or default backend. |
| Product workflow / desktop | 4 | 4 | Dirty shell/native titlebar safe-area baseline is verified and committed; Kun entry baseline remains unchanged. | Tray session menu and Local Whisper remain future import-boundary/QA items. |
| Release evidence readiness | 2 | 2 | Focused conformance tests pass and docs/spec/ledger are updated. | Full command gate, production scans, live provider/MCP, packaged desktop, signing, Windows, and release metadata remain blockers until final evidence is recorded. |

Claim:
Analytix is stronger than the immediate pre-stage state for internal
collaborative-execution proof, offline cache curve proof, MCP/indexer fixture
coverage, and Go G5 readiness. DeepSeek cache proof is at least Reasonix-style,
and multi-provider oracle coverage is broader. This does not establish live
provider/cache superiority, live MCP parity, full Reasonix task/planner parity,
Kun tray/Local Whisper parity, Go G5 parity, release readiness, or default Go
backend readiness.

Decision:
Accept the score changes only for fixture/oracle readiness and internal
runtime reliability. Keep live parity and release scores blocked until their
credentialed or packaged gates pass.

## 2026-06-21 - Executable Runtime / Cache Guard / MCP Live-Local / Kun Tray Score Rule

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` `9e56c3276880ced538b3375329b8a0ebbe63a67b`; Kun
product-entry baseline `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master`
`8602476`, `develop` `247076f`.

Impacted dimensions:
collaborative execution, cache efficiency, MCP/indexer reliability, desktop
tray affordance, Go G5 readiness, product-entry governance.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Collaborative execution runtime readiness | 5 | 5 | Oracle score was already capped; implementation depth improves because internal `task` and `parallel_tasks` tool providers now execute through `DelegationRuntime`, prove dependency order, expose output offsets, and reconcile stale queued/running jobs after restart. | No full Reasonix planner/executor Coordinator, runner rehydration after restart, or desktop nested-card QA. |
| Provider/cache fixture readiness | 5 | 5 | Runtime-owned `offline-cache-curve-guard` replaces test-local logic, and DeepSeek native cache hit/miss fields take precedence over conflicting generic cached-token telemetry. Multi-provider fixture coverage remains broader than Reasonix's DeepSeek-focused guard. | No credentialed live provider/cache superiority, cost reconciliation, or live write-inline/scheduled/probe/model-list matrix. |
| MCP/indexer fixture readiness | 5 | 5 | Fake live-local indexer proof covers retry-all, late tombstone, cwd, low-priority/backgroundStart, restart/resume, and secret-safe diagnostics without exposing Reasonix plugin protocol. | No live MCP/indexer parity or desktop lifecycle QA. |
| Desktop Kun delta | 4 | 4 | Tray session menu is now absorbed as an analytix-native main-process surface that groups runtime threads and opens existing windows without renderer route or bridge changes. Product-entry score stays unchanged. | Packaged tray QA and Local Whisper remain open. |
| Go runtime kernel | 3 | 3 | G5 remains a TS-owned inventory. No Go files changed because this machine has no `go` command, so required `go test ./...` validation cannot run. | No Go full loop/jobs/cache/session/resume/interrupt/MCP implementation, backend selection, rollback, packaged QA, or default backend. |
| Product boundary governance | 4 | 4 | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry, Kun identity, Reasonix public protocol, Rust/Tauri path, or default Go backend was added. | Full release evidence and live parity remain open. |

Claim:
Analytix now has an executable internal task/parallel runtime slice, a
runtime-owned offline cache curve guard, broader DeepSeek/provider cache
fixtures than Reasonix's focused cache guard, a fake live-local MCP/indexer
lifecycle proof, and an analytix-native Kun tray session menu. These are
fixture/internal-runtime and desktop-affordance claims only.

Decision:
Keep live parity, release, and default backend scores blocked. Do not raise Go
or live provider/MCP scores until the required toolchain, credentials, and
packaged/live QA gates pass.

## 2026-06-21 - Reasonix bfe398 Drift And Go G5 Shadow Slice Score Rule

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2`
`9e56c3276880ced538b3375329b8a0ebbe63a67b..bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`;
Kun refs unchanged from the v0.2.13 -> v0.2.14 baseline.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Reasonix currentness | classified at 9e56 | classified at bfe398 | New drift is PowerShell 7 shell/sandbox compatibility and shell-tool guidance; no task/cache/MCP/product-entry drift. | Future Windows shell-tool compatibility fixture if runtime host-shell behavior becomes a product contract. |
| Go runtime kernel | 3 | 3 | G5 now has a Go shadow output slice that replays source oracle ids, full-loop inventory, job routes/behaviors, cache/compaction, resume/interrupt, MCP/indexer, blockers, and boundary flags. | No live Go full-loop executor, backend selection, rollback, Electron integration, packaged QA, or default backend. |
| Product boundary governance | 4 | 4 | Shadow output preserves no Reasonix public protocol, no renderer-visible Go route, no Electron-to-Go, and no default Go backend. | Full release evidence and live parity remain open. |

Allowed wording:

```text
Go G5 advanced from inventory-only to a tested shadow output slice. Reasonix
bfe398 drift is classified as shell/sandbox compatibility input.
```

Forbidden wording:

```text
Do not claim Go G5 runtime parity, default Go backend readiness, live provider
or MCP parity, release readiness, or imported Reasonix shell/sandbox behavior.
```

## 2026-06-21 - Go G5 Composite Shadow Replay Score Rule

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Go runtime kernel | shadow depth improves; backend score unchanged | Go now computes a composite G5 replay from task-job, provider-cache, G2 route replay, and MCP lifecycle fixtures instead of replaying only the G5 inventory. | No live Go full-loop executor, live Job Manager, live cache/session/MCP runtime, Electron integration, rollback/default-backend plan, or packaged QA. |
| Cache/MCP/session conformance | fixture cross-link improves | G5 shadow output includes cache release-guard statuses, G2 resume/fork/SSE route ids, and MCP live-local/tombstone/redaction fields from source fixtures. | Live provider superiority and live MCP/indexer parity remain blocked. |

Allowed wording:

```text
Go G5 has cross-fixture composite shadow replay for jobs/cache/session/MCP.
```

Forbidden wording:

```text
Do not claim Go G5 runtime parity, default Go backend readiness, live provider
or MCP parity, release readiness, or Go execution of the full loop.
```

## 2026-06-21 - Planner Executor / Shell / Live-Local / G5 Runner Score Rule

Branch:
local `codex/reasonix-goal-control-delta-oracle`.

Upstream sources:
Reasonix `main-v2` `bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0`; Kun
product-entry baseline `v0.2.13` `201a1469`, `v0.2.14` `06be05d`, `master`
`8602476`, `develop` `247076f`.

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 5 | 5 | Planner/executor coordinator and durable-runner rehydration deepen the already capped fixture score with executable DAG failure/cancel/output and restart output/wait/kill proof. | No public Reasonix task protocol, packaged crash drill, or top-level workflow UI. |
| Windows shell compatibility | 3 | 4 | Runtime fixtures cover standard pwsh path fallback, pwsh chaining support, Windows PowerShell unquoted chaining guard, and quoted literal allowance. | Native Windows host and packaged terminal QA remain missing. |
| Provider/cache fixture readiness | 5 | 5 | Executable local DeepSeek-compatible provider proves request URL shape, native cache hit/miss precedence, hit rate, and reasoning tokens without credentials. | Live provider superiority, write-inline/scheduled/probe/model-list live matrix, and cost reconciliation remain blocked. |
| MCP/indexer fixture readiness | 5 | 5 | Executable stdio fake MCP indexer server proves persistent restart/resume, tombstone, cwd, priority/background diagnostics, and redaction. | Live MCP/indexer QA and Reasonix plugin protocol parity remain unclaimed. |
| Go runtime kernel | 3 | 3 | G5 composite replay includes planner/executor and durable runner restart fields from TS fixtures; Go package tests pass with a temporary verified toolchain. | No live Go full-loop executor, live Job Manager, backend selection, rollback, packaged QA, or default backend. |
| Product boundary governance | 4 | 4 | Kun entry baseline remains unchanged and scans must keep top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer absent. | Local Whisper/package/platform QA remain open. |

Allowed wording:

```text
Analytix has stronger scoped internal runtime, Windows shell, live-local
provider/MCP, and Go G5 runner shadow evidence than the previous fixture layer.
```

Forbidden wording:

```text
Do not claim full Reasonix runtime parity, live provider/cache superiority,
live MCP/indexer parity, Go G5 runtime parity, release readiness, Reasonix
public protocol, Kun identity, or default Go backend readiness.
```

## 2026-06-21 - Reasonix 881 Control Parity Gate Score Rule

This score rule covers the next control gate after section 24. It improves
runtime-control reliability and Go shadow evidence but does not raise live
provider/MCP/release/default-backend scores.

Currentness:

```text
Requested Reasonix target 881b2f2f2644d1873c7867f29c64cae7be7d9c24 is no
longer origin/main-v2 HEAD. Fetched origin/main-v2 is
9ada14176629b1d59d7ed78446951b2bb5954904. Post-881 auto-plan drift is recorded
only, not absorbed.
```

Benchmark effect:

| Dimension | Score effect | Evidence | Remaining gap |
| --- | --- | --- | --- |
| Cancel/batch control | implementation depth improves inside capped runtime score | `AgentLoop` preserves completed/cancelled/unstarted tool results by call id and order; cancelled batches no longer discard provider-history evidence. | No Reasonix public control protocol or TUI cancel UX. |
| Durable task control | implementation depth improves | Parent abort propagates to foreground/background jobs; `parallel_tasks` returns completed/killed/skipped evidence and output offsets. | No public Reasonix task protocol or packaged crash-drill claim. |
| Step limits | contract depth improves | top-level `runtime.runtimeTuning.stepLimits`, thread/session `runtimeStepLimits`, per-turn `maxModelSteps`, planner/headless handling, and delegate/task `max_steps` are implemented without stable-prefix mutation. | No Reasonix config root or auto-plan classifier import. |
| Provider/cache | no live score change | Step-limit/cancel tests verify stable prefix is unchanged; provider/cache matrices remain existing fixture/live-local evidence. | Credentialed DeepSeek/OpenAI Responses/Anthropic/custom provider live matrix remains blocked. |
| Go G5 | shadow evidence improves; backend score unchanged | G5 `controlReplay` shadows cancel, task-job cancel aggregation, and step-limit controls. | No live Go runtime, Electron integration, rollback/default-backend plan, or packaged QA. |
| Kun desktop | product-entry score unchanged | Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/Connect Phone/Schedule; duplicate preload/loading stashes are retained, not dropped. | Local Whisper and platform package QA remain gated. |

Allowed wording:

```text
Analytix has reached scoped Reasonix 881 control parity for cancelled batch
result preservation and runtime-owned step-limit controls, with Go G5 shadow
evidence for the same controls.
```

Forbidden wording:

```text
Do not claim full Reasonix parity, post-881 auto-plan parity, live provider or
MCP superiority, Go G5 runtime parity, release readiness, Kun identity,
Reasonix public protocol, default Go backend, or Rust/Tauri migration.
```

## 2026-06-21 - Post-881 Router Guard / G5 Executable Control / Kun Context Score Rule

Scope:

```text
Reasonix 881b2f2f..9ada1417 is classified. Only classifier lifecycle value is
absorbed as an analytix-owned router guard. Kun 0.2.14 context-window drift is
corrected. Go G5 remains shadow-only but now executes control cases.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime classifier governance | 2 | 3 | Auto-router route cache keys include a classifier contract fingerprint, proving prompt/model/timeout drift rebuilds same-turn routing state. | No public auto-plan setting or Reasonix post-881 auto-plan parity. |
| Go runtime kernel | 3 | 3 | Score remains capped, but evidence deepens because Go computes cancel/task-job/step-limit executable cases from the TS-owned G5 oracle. | No live Go full-loop executor, Electron integration, rollback/default-backend plan, packaged QA, or default backend. |
| Kun provider defaults | 3 | 4 | Shared provider profiles now default unknown explicit text models to `128_000` context tokens, matching Kun 0.2.14/runtime behavior, while explicit profile values override. | Local Whisper/package/platform QA remains open. |
| Provider/cache live proof | 5 | 5 | No provider/cache body, URL, or usage parsing behavior changed; existing offline/live-local cache evidence remains valid. | Credentialed live matrix and cost reconciliation remain blocked. |

Allowed wording:

```text
Analytix is stronger than the prior fixture layer for classifier drift guards,
Go G5 executable shadow controls, and Kun provider-default parity.
```

Forbidden wording:

```text
Do not claim post-881 Reasonix auto-plan parity, full Reasonix parity, live
provider/cache superiority, live MCP parity, Go runtime parity, release
readiness, default Go backend, Reasonix public protocol, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-21 - Approval/User-Input Abort Cleanup Score Rule

Scope:

```text
Reasonix approvalManager/control value is absorbed only as analytix-owned gate
cleanup. No Reasonix SessionAPI, public approval protocol, frontend task card,
or new navigation surface is imported.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Safety and approval cleanup | 4 | 5 | Pending approvals expire on turn abort, emit `approval_resolved: expired`, reject late allow/deny, and cannot execute the guarded tool. | No full Reasonix approval-manager parity or live renderer-card QA claim. |
| User-input replay | 4 | 5 | Abort while awaiting `request_user_input` emits `user_input_resolved: cancelled`; late HTTP resolve returns 404. | No new public user-input protocol or desktop crash drill. |
| Renderer live mapping | 4 | 5 | `approval_resolved: expired` updates existing approval cards to the current error state in live SSE mapping. | Full desktop click-through QA remains separate. |
| Runtime route oracle | 4 | 5 | Route fixture pins cleanup replay kinds, late approval 409, and late user-input 404. | Full release gate and packaged QA remain separate. |
| Go runtime kernel | 3 | 3 | G3/G4/G5 conformance focused tests remain green; no Go runtime behavior changed. | No live Go approval/user-input executor or default backend. |
| Product boundary governance | 4 | 4 | No Reasonix SessionAPI, public protocol, top-level Workflow/Subagent entry, Kun identity, Rust/Tauri path, or default Go backend was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix has stronger approval/user-input abort cleanup and replay evidence:
cancelled GUI gates close deterministically and late GUI actions cannot revive
cancelled work.
```

Forbidden wording:

```text
Do not claim full Reasonix approval-manager parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Renderer Approval Live-Card Store Score Rule

Scope:

```text
Approval/user-input abort cleanup evidence is extended from mapper-level proof
to renderer store-level live-card proof for main and side conversations.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Renderer live approval cards | 4 | 5 | Main-thread `buildThreadEventSink` updates an existing approval block when live `onApprovalStatus` arrives. | No packaged desktop click-through or crash/restart QA claim. |
| Side/nested event rendering | 2 | 3 | Side conversation SSE sink updates only the side approval card and leaves main thread blocks unchanged. | Full nested visual rendering QA remains open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix has renderer store-level proof that live approval resolution closes
main-thread and side-conversation approval cards.
```

Forbidden wording:

```text
Do not claim full Reasonix frontend approval parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - User-Input Live-Card Store Score Rule

Scope:

```text
Approval/user-input abort cleanup evidence is extended to user-input store
live-card proof for main and side conversations, including itemId/inputId
matching.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Renderer live user-input cards | 4 | 5 | Main-thread `buildThreadEventSink` updates an existing user-input block when live `onUserInputStatus` arrives by item id or input/request id. | No packaged desktop click-through or crash/restart QA claim. |
| Side/nested event rendering | 3 | 4 | Side user-input cards use runtime item ids; side status updates remain scoped to side blocks. | Full nested visual rendering QA remains open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix has renderer store-level proof that live user-input resolution closes
main-thread and side-conversation user-input cards.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input frontend parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Planner Gating For Internal Task Tools Score Rule

Scope:

```text
Internal `task` / `parallel_tasks` tools remain available only behind
analytix-owned runtime contracts and are explicitly excluded from Plan mode.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Collaborative execution safety | 5 | 5 | Score remains capped, but evidence deepens because Plan mode does not advertise internal task tools and forged `task` calls cannot execute child work. | Full Reasonix planner/executor Coordinator and desktop nested-card QA remain open. |
| Go/runtime oracle readiness | 3 | 3 | `task-job-orchestration-oracle.json` now records `plannerForbiddenToolset` for future Go gates. | No live Go full-loop executor, Electron integration, rollback/default-backend plan, or packaged QA. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix has stronger planner-gating proof: internal child-job tools cannot
leak into Plan mode or run through forged calls.
```

Forbidden wording:

```text
Do not claim full Reasonix planner/executor parity, live provider/cache
superiority, live MCP parity, Go runtime parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Go G5 Planner-Forbidden Shadow Replay Score Rule

Scope:

```text
Go G5 shadow output now consumes the TS-owned plannerForbiddenToolset from the
task-job oracle and records it in jobReplay.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go/runtime oracle readiness | 3 | 3 | Score remains capped, but evidence deepens because Go G5 `jobReplay` now asserts `plannerForbiddenToolset` from the TS task-job oracle. | No live Go full-loop executor, Electron integration, rollback/default-backend plan, or packaged QA. |
| Collaborative execution safety | 5 | 5 | Existing planner-gating proof is now carried through the cross-backend shadow fixture. | Full Reasonix planner/executor Coordinator and desktop nested-card QA remain open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Go G5 shadow carries planner-forbidden task-tool evidence from the TypeScript
oracle while remaining non-default and non-live.
```

Forbidden wording:

```text
Do not claim Go runtime parity, default Go backend readiness, full Reasonix
planner/executor parity, live provider/cache superiority, live MCP parity,
release readiness, Reasonix public protocol, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-21 - Structured User-Input Choice Validation Score Rule

Scope:

```text
The runtime `request_user_input` tool now rejects malformed structured choice
requests before opening a GUI user-input gate, and G4/Go shadow evidence records
the invalid-case contract.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input control safety | 5 | 5 | Score remains capped, but evidence deepens because malformed structured choices cannot create pending user-input gates. | Packaged desktop visual/live user-input QA remains open. |
| Go/runtime oracle readiness | 3 | 3 | G4 oracle/schema/test and Go shadow output now assert invalid cases, `invalid_user_input_request`, and `opensGateOnInvalid:false`. | No live Go approval/user-input manager, Electron integration, rollback/default-backend plan, or packaged QA. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider default change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix rejects malformed structured GUI input choices before opening a gate,
with G4/Go shadow evidence for the error contract.
```

Forbidden wording:

```text
Do not claim full Reasonix ask/user-input parity, desktop live QA, live
provider/cache superiority, live MCP parity, release readiness, default Go
backend, Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Write-Inline Custom Full Endpoint Score Rule

Scope:

```text
Desktop write-inline provider calls now have explicit custom full endpoint
regression tests for URLs ending in `/responses` and `/messages`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 5 | 5 | Score remains capped, but evidence deepens because custom full endpoint URLs are not path-appended and preserve Responses/Messages body/header/parser shape. | Credentialed live provider matrix remains open. |
| Provider/cache fixture readiness | 5 | 5 | Custom provider non-regression is now covered beyond `/completions` and invalid endpoint paths. | No live provider/cache superiority or cost reconciliation. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider default change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Write-inline custom full endpoint proof now covers Responses and Messages
request surfaces without path appending.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Auto-Model Route Cache Lifecycle Score Rule

Scope:

```text
Runtime loop tests now prove that `model:"auto"` route selection is cached
within a multi-step turn and recalculated for the next turn.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Agent routing quality | 5 | 5 | Score remains capped, but evidence deepens because same-turn auto routing avoids repeated classifier calls while next-turn routing stays current. | No Reasonix auto-plan product surface or desktop controller API. |
| Cache/currentness safety | 5 | 5 | The route cache is proved as turn-scoped runtime state rather than stable-prefix state. | Live provider/cache superiority and cost reconciliation remain open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, settings schema change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and full Reasonix parity still blocked. |

Allowed wording:

```text
Analytix has loop-level proof for auto-route same-turn reuse and next-turn
currentness.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan parity, user-level auto-plan settings,
project/local auto-plan overrides, live provider/cache superiority, release
readiness, default Go backend, Reasonix public protocol, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-21 - Provider Request-Shape Oracle Matrix Score Rule

Scope:

```text
The provider/cache oracle now includes request URL/header/body shape cases for
DeepSeek official chat, OpenAI-compatible chat, Responses, Anthropic Messages,
and custom Responses full endpoint mode.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 5 | 5 | Score remains capped, but evidence deepens because provider request-shape cases are centralized in the cache oracle and executed against `CompatModelClient`. | Credentialed live provider matrix remains open. |
| Provider/cache fixture readiness | 5 | 5 | Cache usage, cache diagnostics, cache curve, and request-shape fixtures now share one oracle. | No live provider/cache superiority or cost reconciliation. |
| Go/runtime oracle readiness | 3 | 3 | G3/G5 shadow replay carries request-shape case ids without creating a Go provider client. | No live Go provider/runtime integration, rollback, or packaged QA. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, settings schema change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Provider/cache proof now centrally covers request-shape invariants for the
major endpoint families and custom full endpoint mode.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Write-Inline Provider Request-Surface Score Rule

Scope:

```text
Desktop write-inline provider calls now have explicit OpenAI Responses and
Anthropic Messages request-surface regression tests.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache fixture readiness | 5 | 5 | Score remains capped, but evidence deepens because write-inline now pins Responses URL/body/header and Messages URL/body/header/parser behavior. | No credentialed live provider/cache superiority or cost reconciliation. |
| Provider diagnostic safety | 5 | 5 | Responses tests prove Anthropic headers do not leak to Responses; Messages tests prove the messages-specific parser/body path. | Live write-inline/scheduled/probe/model-list matrix remains open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider default change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Analytix provider request-surface evidence now includes write-inline Responses
and Messages regression tests.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
credentialed provider matrix completion, release readiness, default Go backend,
Reasonix public protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Go G5 Planner Tool-Policy Executable Shadow Score Rule

Scope:

```text
Go G5 control executable output now computes planner tool-policy gating from
TS-owned fixture input.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go/runtime oracle readiness | 3 | 3 | Score remains capped, but evidence deepens because Go computes step 0 read-only + `create_plan`, step 1 `create_plan` only, and forged `task` rejection. | No live Go full-loop executor, Electron integration, rollback/default-backend plan, or packaged QA. |
| Collaborative execution safety | 5 | 5 | Existing planner-gating proof now has executable Go shadow coverage instead of only fixture replay. | Full Reasonix planner/executor Coordinator and desktop nested-card QA remain open. |
| Product boundary governance | 4 | 4 | No Reasonix protocol, Kun identity, deprecated bridge, provider default change, Go/Rust/Tauri path, or top-level Workflow/Subagent entry was added. | Release readiness and live parity still blocked. |

Allowed wording:

```text
Go G5 executable shadow covers planner tool-policy gating while remaining
non-default and non-live.
```

Forbidden wording:

```text
Do not claim Go runtime parity, default Go backend readiness, full Reasonix
planner/executor parity, live provider/cache superiority, live MCP parity,
release readiness, Reasonix public protocol, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-21 - Kun Top-Level Route-Surface Oracle Score Rule

Scope:

```text
Renderer route-surface tests now pin the Kun target-version entry boundary for
AppRoute, app actions, Workbench stage rendering, shell navigation, and sidebar
active views.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product boundary governance | 4 | 4 | Score remains capped, but evidence deepens because forbidden top-level entries are now unit-tested, not only scanned. | Packaged desktop QA and future product-entry specs remain open. |
| Kun target-version fidelity | 4 | 4 | AppRoute/action tests prove Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer cannot appear as top-level actions/routes. | Does not prove full Kun current parity. |
| Reasonix absorption safety | 5 | 5 | Internal orchestration can continue behind analytix contracts without becoming a renderer route. | Full Reasonix orchestration parity is still incomplete. |

Allowed wording:

```text
Analytix has executable route-surface evidence that Kun-target-absent
orchestration features are not top-level navigation entries.
```

Forbidden wording:

```text
Do not claim new product navigation, full Kun current parity, full Reasonix
planner/subagent parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Auto-Route Step/Cancel Control Composition Score Rule

Scope:

```text
Runtime and G5 shadow evidence now compose auto-route cache reuse, user-global
step-limit failure, stable-prefix exclusion, and cancelled tool-result
preservation.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Control-safety composition | 5 | 5 | Score remains capped, but evidence deepens because auto-route cache and step-limit behavior are now tested together. | Packaged interruption UX and crash/restart drills remain separate. |
| Go/runtime oracle readiness | 3 | 3 | Go G5 shadow computes combined cache/step/cancel output from TS-owned fixture input. | No live Go AgentLoop, providers, sessions, MCP, rollback, or Electron integration. |
| Cache determinism | 5 | 5 | Dynamic route/step/cancel state remains out of stable prefix/context instructions. | Live provider/cache superiority and cost reconciliation remain open. |

Allowed wording:

```text
Analytix has composed runtime and G5 shadow evidence for auto-route cache,
step-limit, and cancel-result preservation.
```

Forbidden wording:

```text
Do not claim live Go backend readiness, Reasonix auto-plan parity, live
provider/cache superiority, live MCP parity, release readiness, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Preload Bridge/API Sovereignty Score Rule

Scope:

```text
Source-level preload/API tests now pin renderer bridge ownership to
`window.analytix` and analytix-owned public facade type names.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product boundary governance | 4 | 4 | Score remains capped, but evidence deepens because bridge exposure, `Window` typing, and public facade names are now unit-tested. | Packaged desktop QA and future bridge specs remain open. |
| Kun target-version fidelity | 4 | 4 | Deprecated Kun GUI bridge aliases and public Kun facade names are rejected by source-level tests. | Does not prove full Kun current parity. |
| Reasonix absorption safety | 5 | 5 | Reasonix SessionAPI/frontend protocol names cannot appear as public shared bridge API exports. | Full Reasonix frontend/session parity remains incomplete and is not a target public protocol. |

Allowed wording:

```text
Analytix has executable preload/API evidence that the renderer public bridge
remains `window.analytix`.
```

Forbidden wording:

```text
Do not claim a new public bridge, full Reasonix SessionAPI parity, full Kun
current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Renderer Thread Lifecycle HTTP Score Rule

Scope:

```text
Renderer runtime adapter tests now pin thread lifecycle operations to
analytix-owned `/v1/threads` HTTP paths.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but evidence deepens because list/search/archive/restore/rename/workspace/delete are now tested at the renderer adapter. | Packaged desktop sidebar/thread-list QA remains open. |
| Reasonix absorption safety | 5 | 5 | Renderer lifecycle calls stay on analytix HTTP/SSE and do not expose Reasonix SessionAPI. | Full Reasonix frontend/session parity is not a public-contract target. |
| Kun target-version fidelity | 4 | 4 | Lifecycle calls do not reintroduce Kun public protocol or legacy bridge assumptions. | Does not prove full Kun current parity. |

Allowed wording:

```text
Analytix has renderer adapter evidence that thread lifecycle actions stay on
the analytix HTTP runtime contract.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, full Reasonix SessionAPI parity, full Kun
current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Settings/Provider EndpointFormat Persistence Score Rule

Scope:

```text
Settings-store tests now pin runtime and provider endpoint-format persistence
to analytix-owned settings envelopes.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but evidence deepens because endpoint-format settings persist before request-shape oracles consume them. | Credentialed provider matrix and live cache/cost comparison remain open. |
| Product boundary governance | 4 | 4 | Persisted settings omit legacy `agentProvider` and `agents` envelopes. | Packaged settings UI QA remains separate. |
| Reasonix/Kun absorption safety | 5 | 5 | Endpoint format does not require Reasonix config roots or Kun/deprecated settings identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has settings-store evidence that endpoint-format choices persist in
top-level `runtime` and `provider.providers`.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - D-0063 Go G5 Provider Cache Release Guard Shadow Score Rule

Scope:

```text
G5 shadow now computes the offline provider cache release guard from the
existing analytix provider-cache oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider cache safety | 4 | 4 | Score remains capped, but G5 shadow covers release guard tail averages, collapse counts, allowed-low budget, and overall pass. | Credentialed provider matrix and live cache/cost comparison remain open. |
| Go G5 gate quality | 3 | 3 | Go shadow computes the release guard output from fixture data and TS conformance derives expected output from `evaluateOfflineCacheCurveGuard`. | No Go provider client, route, rollback, or default backend. |
| Product boundary governance | 5 | 5 | No Reasonix provider protocol, live superiority claim, Go route, Kun identity, or hidden top-level entry is added. | Release readiness still requires packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes the offline provider cache release guard
consistently with the TypeScript cache guard oracle.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix,
Go provider client readiness, default Go backend, Reasonix provider protocol
parity, packaged provider QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - D-0064 Auto-Route Step/Cancel Composition Score Rule

Scope:

```text
AgentLoop now has a focused proof that auto-route cache, step-limit metadata,
and cancellation compose safely in one real turn.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but `loop.test.ts` now proves auto route reuse across a second model step before interrupted parallel tools. | Packaged interruption QA remains separate. |
| Cache/prefix hygiene | 4 | 4 | Step-limit metadata reaches tool context while prefix/context stay stable and model-visible text omits step budget details. | Live provider/cache superiority remains open. |
| Product boundary governance | 5 | 5 | No Reasonix auto-plan setting, controller protocol, Go route, Kun identity, or hidden top-level entry is added. | Product auto-plan parity remains rejected/deferred. |

Allowed wording:

```text
Analytix AgentLoop proves auto-route cache, step-limit metadata, and cancelled
parallel tool results compose safely in a focused runtime test.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, planner enable/disable product
controls, Reasonix controller/SessionAPI protocol parity, live provider/cache
superiority, default Go backend, packaged desktop QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - D-0065 Planner Step-Limit Score Rule

Scope:

```text
AgentLoop now has a focused proof that planner-specific step limits gate
plan-mode turns without leaking planner budget state into prompt/cache surfaces.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but `loop.test.ts` now proves `plannerMaxModelSteps` applies to real plan-mode turns. | Packaged plan-mode QA remains separate. |
| Cache/prefix hygiene | 4 | 4 | Prefix/context stay free of planner step-budget text. | Live provider/cache superiority remains open. |
| Product boundary governance | 5 | 5 | No Reasonix planner setting, controller protocol, Go route, Kun identity, or hidden top-level entry is added. | Product planner toggles remain rejected/deferred. |

Allowed wording:

```text
Analytix AgentLoop proves planner-specific step limits gate plan-mode turns
without polluting prompt/cache surfaces.
```

Forbidden wording:

```text
Do not claim Reasonix planner enable/disable product controls, Reasonix
controller/SessionAPI protocol parity, live provider/cache superiority, default
Go backend, packaged desktop QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Go G5 User-Input Gate Shadow Score Rule

Scope:

```text
G5 shadow now computes submitted/cancelled user-input gate outcomes from the
existing approval-user-input route oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 gate quality | 3 | 3 | Score remains capped, but Go shadow now has executable submitted/cancelled user-input gate proof. | No live Go user-input manager or default backend. |
| Approval/user-input safety | 4 | 4 | HTTP/gate answer echo and SSE replay answer omission are bound into G5 fixture and Go tests. | Packaged renderer live-card QA remains separate. |
| Product boundary governance | 5 | 5 | No Reasonix ask protocol, Go route, Kun identity, or hidden top-level entry is added. | Release readiness still requires full gates and packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes submitted/cancelled user-input gate outcomes while
keeping submitted answers out of SSE replay.
```

Forbidden wording:

```text
Do not claim live Go backend parity, default Go backend, Reasonix ask/session
protocol parity, renderer-visible Go routes, packaged desktop live-card QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 AutoResearch State Shadow Score Rule

Scope:

```text
G5 shadow now computes AutoResearch project-local state and requirement-audit
boundaries from the existing TypeScript store contract.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Long-running task state | 3 | 3 | Score remains capped, but G5 shadow now covers state path, file inventory, unknown requirement rejection, and forbidden public files. | No live Go AutoResearch manager or packaged restart QA. |
| Cache/prefix hygiene | 4 | 4 | Research state remains outside stable prefix and tool schema in G5 expected output. | Live provider/cache superiority remains separate. |
| Product boundary governance | 5 | 5 | No top-level AutoResearch route, Reasonix project protocol, Go route, Kun identity, or hidden top-level entry is added. | Release readiness still requires full packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes AutoResearch project-local state boundaries without
exposing a top-level AutoResearch surface.
```

Forbidden wording:

```text
Do not claim live AutoResearch product parity, live Go backend parity, default
Go backend, Reasonix AutoResearch/project protocol parity, top-level
AutoResearch navigation, packaged desktop QA, release readiness, Kun identity,
or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Lifecycle Shadow Score Rule

Scope:

```text
G5 shadow now computes MCP retry-all, tombstone/resume, and redaction
boundaries from the existing TypeScript MCP lifecycle oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle | 3 | 3 | Score remains capped, but G5 shadow covers retry attempts, connected/error server ids, active paths, tombstone count, and restart snapshot semantics. | No live credentialed MCP matrix. |
| Privacy/tool safety | 4 | 4 | Redacted diagnostics and `leaksSecret:false` are pinned in G5 expected output. | Packaged desktop MCP QA remains separate. |
| Product boundary governance | 5 | 5 | No top-level MCP-indexer route, Reasonix MCP protocol, Go route, Kun identity, or hidden top-level entry is added. | Release readiness still requires packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes MCP lifecycle boundaries without exposing
MCP-indexer as a product surface.
```

Forbidden wording:

```text
Do not claim live MCP parity, Go MCP client readiness, live Go backend parity,
default Go backend, Reasonix MCP protocol parity, top-level MCP-indexer
navigation, packaged desktop QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - D-0060 Go G5 Checkpoint/Rewind Shadow Score Rule

Scope:

```text
G5 shadow now computes checkpoint/rewind executable safety boundaries from the
existing analytix checkpoint oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Checkpoint/rewind safety | 4 | 4 | Score remains capped, but G5 shadow covers ready/blocked file counts, path escape blocking, symlink blocking, legal `..name` handling, and append-only conversation audit. | Packaged desktop rewind QA remains separate. |
| Go G5 gate quality | 3 | 3 | Go shadow computes `axcp_` / `axrp_` / `axra_` / `axrr_` boundaries, explicit confirmation, no transcript rewrite, no git refs, and no public route. | No live Go checkpoint store, Go route, rollback, or default backend. |
| Product boundary governance | 5 | 5 | No Reasonix checkpoint/rewind protocol, Kun git-ref checkpoint contract, Go route, Kun identity, or hidden top-level entry is added. | Release readiness still requires packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes checkpoint/rewind safety boundaries while preserving
analytix-owned ids, path guards, explicit confirmation, and append-only audit.
```

Forbidden wording:

```text
Do not claim live Go checkpoint parity, default Go backend, Reasonix
checkpoint/rewind protocol parity, Kun git-ref checkpoint parity, packaged
desktop rewind QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - D-0061 Go G5 Remote-Entry Boundary Shadow Score Rule

Scope:

```text
G5 shadow now computes the remote-entry control-port boundary from the existing
analytix approval/user-input route oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Remote-entry safety | 4 | 4 | Score remains capped, but G5 fixture and TS conformance derive allowed `approvals`/`lifecycle`/`turns` ports and forbidden goal/checkpoint/memory/storage/tool-host keys. | Packaged desktop remote-entry QA remains separate. |
| Go G5 gate quality | 3 | 3 | Go shadow computes forbidden control planes absent, policy override rejected, no Reasonix protocol, and no top-level route. | No live Go remote-entry route, Electron integration, rollback, or default backend. |
| Product boundary governance | 5 | 5 | No Reasonix SessionAPI/control protocol, Go route, Kun identity, bridge/settings fallback, or hidden top-level entry is added. | Release readiness still requires packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes that remote entries stay limited to lifecycle, turn,
approval, and user-input control ports.
```

Forbidden wording:

```text
Do not claim live Go remote-entry parity, default Go backend, Reasonix
SessionAPI/protocol parity, renderer-visible Go routes, packaged desktop QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - D-0062 Go G5 Resume Pending Gates Shadow Score Rule

Scope:

```text
G5 shadow now computes session-resume pending approval/user-input gate safety
from the existing analytix approval/user-input route oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Resume safety | 4 | 4 | Score remains capped, but G5 fixture and TS conformance derive source pending and resumed expired/cancelled gate statuses. | Packaged desktop resume QA remains separate. |
| Approval/user-input safety | 4 | 4 | Go shadow computes `pendingAfterResume:0` and `answersCopiedToResume:false`. | No live Go approval/user-input manager or Go session-resume route. |
| Product boundary governance | 5 | 5 | No Reasonix ask/session protocol, Go route, Kun identity, bridge/settings fallback, or hidden top-level entry is added. | Release readiness still requires packaged QA. |

Allowed wording:

```text
Analytix G5 shadow computes that resumed threads preserve approval/user-input
gate audit visibility without leaving pending gates actionable.
```

Forbidden wording:

```text
Do not claim live Go resume parity, default Go backend, Reasonix ask/session
protocol parity, renderer-visible Go routes, packaged desktop resume QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Managed Runtime Provider Currentness Score Rule

Scope:

```text
Managed runtime rebuild decisions and child provider snapshots now include
selected provider endpoint/model-profile currentness, and identity guards cover
model-visible prompt copy, public facade domains, and legacy Kun agent envelopes.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but runtime settings key/change checks are shared and provider currentness is tested. | Packaged restart QA remains separate. |
| Provider request safety | 4 | 4 | Managed env snapshot refreshes provider base URL, endpoint format, and model reasoning profile drift. | Credentialed provider matrix and live cache/cost comparison remain open. |
| Product sovereignty | 5 | 5 | Model-visible system prompt, public facade domains, and `agents.kun` fallback are guarded. | Internal `claw` compatibility rename remains deferred. |

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

## 2026-06-21 - Provider Endpoint URL Builder Parity Score Rule

Scope:

```text
Schedule and Write auxiliary provider consumers now share endpoint URL
construction for versioned responses/messages bases and known endpoint paths.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but shared helper and fake-fetch tests prevent duplicated versioned endpoint paths. | Credentialed provider matrix and live cache/cost comparison remain open. |
| Auxiliary consumer reliability | 4 | 4 | Scheduled detector and write-inline use the same helper and focused tests pass. | Packaged Schedule/Write provider QA remains separate. |
| Go runtime readiness | 3 | 3 | No Go runtime code changed. | No Go provider client, route, Electron integration, rollback, or default backend. |

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

## 2026-06-21 - Connect Phone Product Copy Score Rule

Scope:

```text
Connect Phone user-visible/model-visible natural-language copy is guarded
against standalone Claw wording while internal `claw` compatibility names remain.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but renderer locale scan now prevents standalone `Claw` in user-visible strings. | Internal `claw` compatibility rename remains future work. |
| Kun baseline preservation | 5 | 5 | Connect Phone naming is corrected without changing the Code/Write/Settings/Plugins/Connect Phone/Schedule entry baseline. | Live Connect Phone desktop/remote QA remains release-gated. |
| Go runtime readiness | 3 | 3 | No Go runtime code changed. | No Go route, backend, or Electron integration. |

Allowed wording:

```text
Analytix guards Connect Phone copy against standalone Claw wording while
preserving internal compatibility contracts.
```

Forbidden wording:

```text
Do not claim live Connect Phone superiority, internal `claw` contract rename,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Task-Job Stale Restart Reconciliation Score Rule

Scope:

```text
Queued and running stale task jobs now have oracle-backed restart reconciliation
evidence in the TypeScript runtime tests and Go G5 executable shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but evidence deepens because restart reconciliation now covers queued and running stale jobs. | Packaged restart QA remains separate. |
| Reasonix absorption safety | 5 | 5 | Job lifecycle discipline is absorbed without Reasonix public job/session protocol. | No Reasonix public sub-agent/job protocol parity claim. |
| Go runtime shadow quality | 3 | 3 | G5 executable shadow computes stale jobs -> failed from TS-owned oracle. | No Go Job Manager, renderer route, or default backend. |

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

## 2026-06-21 - MCP Annotation Approval No-Execute Score Rule

Scope:

```text
Destructive/openWorld MCP tool annotations now have oracle-backed proof that
denied GUI approval prevents MCP client execution.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Tool approval safety | 4 | 4 | Score remains capped, but evidence deepens because denied annotated MCP tools never call `client.callTool`. | Packaged approval-card QA remains separate. |
| Reasonix absorption safety | 5 | 5 | MCP lifecycle semantics are absorbed into analytix-owned tool contracts. | No Reasonix MCP public protocol or MCP-indexer entry. |
| Go runtime shadow quality | 3 | 3 | G4 shadow computes the no-execute claim from fixture fields. | No Go MCP client, renderer route, or default backend. |

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

## 2026-06-21 - User-Input Submitted Route Score Rule

Scope:

```text
Submitted user-input answers now have oracle-backed HTTP/gate resolution proof,
while SSE replay remains answer-free.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| User-input lifecycle | 4 | 4 | Score remains capped, but evidence deepens because submitted answers are echoed through HTTP and gate resolution. | Packaged GUI live-card QA remains separate. |
| Runtime/SSE privacy boundary | 4 | 4 | Replay carries submitted status but no answers. | No answer archival or cross-device delivery claim. |
| Go runtime shadow quality | 3 | 3 | G4 shadow computes submitted-answer echo and answer-free replay flags. | No Go user-input backend, renderer route, or default backend. |

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

## 2026-06-21 - Task-Job Route Auth Matrix Score Rule

Scope:

```text
Task-job wait/output/kill now have oracle-backed unauthorized 401 coverage.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but evidence deepens because all task-job control routes reject missing runtime tokens. | Packaged desktop route QA remains separate. |
| Reasonix absorption safety | 5 | 5 | Job control remains authenticated and internal to analytix runtime contracts. | No public Reasonix job protocol or SessionAPI. |
| Go runtime shadow quality | 3 | 3 | Unchanged; no Go route is exposed. | Go route auth remains deferred until Go routes exist behind G5/G6 gates. |

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

## 2026-06-21 - Forbidden Public Runtime Route Score Rule

Scope:

```text
Runtime HTTP tests now prove forbidden upstream public routes return structured
404 responses.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but evidence deepens because forbidden Reasonix, Go, and top-level capability routes are executable 404 guards. | Packaged desktop QA remains separate. |
| Product boundary governance | 4 | 4 | `/v1/reasonix/*`, `/v1/runtime/go`, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public routes stay absent. | Future intentional route additions must update specs/ledger. |
| Reasonix/Kun/Go absorption safety | 5 | 5 | The proof rejects upstream public routes while preserving analytix HTTP/SSE sovereignty and Go shadow-only status. | Does not prove full live Go parity or release readiness. |

Allowed wording:

```text
Analytix has runtime HTTP negative-route evidence that forbidden upstream
public routes stay absent.
```

Forbidden wording:

```text
Do not claim Reasonix public protocol parity, Kun public protocol parity, live
Go backend readiness, default Go backend, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-21 - Rehydrated Task-Job Route Score Rule

Scope:

```text
Task-job restart tests now prove rehydrated jobs remain operable through
authenticated internal runtime routes.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 4 | 4 | Score remains capped, but evidence deepens because output/wait/kill routes are proven after durable job rehydrate. | Packaged desktop restart QA remains separate. |
| Sub-agent/job orchestration | 4 | 4 | Internal task-job continuity improves without exposing public Subagent/Workflow routes. | Live child-agent execution after desktop restart remains open. |
| Reasonix/Go absorption safety | 5 | 5 | Durable-runner behavior is absorbed behind authenticated analytix routes; Go stays shadow-only. | Does not prove live Go backend readiness. |

Allowed wording:

```text
Analytix has runtime HTTP harness evidence that rehydrated task jobs remain
operable through authenticated internal routes.
```

Forbidden wording:

```text
Do not claim Reasonix public sub-agent/job protocol parity, top-level Subagent
or Workflow routes, live Go backend readiness, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Task-Job Approval Deny Score Rule

Scope:

```text
Task-job provider tests now prove denied `task` and `parallel_tasks` approvals
create no durable jobs or child runs.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input safety | 4 | 4 | Score remains capped, but evidence deepens because real task-job providers now prove approval-before-side-effect ordering. | Packaged desktop approval-card QA remains separate. |
| Sub-agent/job orchestration | 4 | 4 | Denied `task`/`parallel_tasks` calls create no durable jobs or child runs. | Live child-agent approval flow remains open. |
| Reasonix/Go absorption safety | 5 | 5 | Reasonix-style orchestration stays behind analytix permission gates; Go remains shadow-only. | Does not prove Go approval manager readiness. |

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
migration.
```

## 2026-06-21 - Go G5 Task-Job Approval Deny Shadow Replay Score Rule

Scope:

```text
Go G5 shadow replay now consumes the TS-owned task-job approval deny
no-execute fixture.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but `BuildG5ShadowSlicesOutput` now emits `approvalDenyNoExecute` from the task-job oracle. | No live Go approval manager, Go Job Manager, Electron integration, rollback, or default backend. |
| Approval/user-input safety | 4 | 4 | Go G5 shadow must preserve denied tool names, approval ids, and no durable-job/child-run side effects. | Packaged desktop approval-card QA remains separate. |
| Reasonix/Go absorption safety | 5 | 5 | Reasonix-style task orchestration remains analytix-owned and fixture-gated. | No Reasonix public protocol or top-level Subagent route. |

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

## 2026-06-21 - Go G5 Task Approval Deny Executable Control Score Rule

Scope:

```text
Go G5 executable shadow now computes task approval denial no-side-effect
results from the TS-owned fixture.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but `BuildG5ControlExecutableOutput` now computes `approvalDeny` output from the oracle. | No live Go approval manager, Go Job Manager, Electron integration, rollback, or default backend. |
| Approval/user-input safety | 4 | 4 | The executable output preserves denied tool names, approval ids/count, and no durable-job/child-run side effects. | Packaged desktop approval-card QA remains separate. |
| Reasonix/Go absorption safety | 5 | 5 | Reasonix-style task orchestration remains analytix-owned and executable only as shadow control. | No Reasonix public protocol or top-level Subagent route. |

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

## 2026-06-21 - Go G3/G5 Provider Cache Accounting Score Rule

Scope:

```text
Go G3/G5 shadow now computes provider cache accounting from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but G3/G5 shadow now computes supported telemetry cases, unsupported unknown cases, hit/miss totals, and aggregate hit rate. | Live credentialed provider matrix remains separate. |
| Go G5 readiness | 3 | 3 | G3 expected output and G5 `cacheReplay` compute provider cache accounting from the oracle. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/provider absorption safety | 5 | 5 | DeepSeek/OpenAI/Anthropic semantics stay analytix-owned and fixture-bound; unsupported providers are not counted as misses. | No Reasonix provider protocol or live superiority claim. |

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

## 2026-06-21 - Auto-Router Failure Usage Isolation Score Rule

Scope:

```text
Auto-router failure fallback now proves router usage/cache telemetry is not
recorded as main turn usage.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Auto-router/currentness | 4 | 4 | Score remains capped, but classifier failure now falls back without usage/cache contamination. | Managed runtime provider/settings rebuild proof is covered by the later currentness guard; Reasonix controller parity remains rejected. |
| Provider/cache proof | 4 | 4 | Router usage/cache telemetry is ignored by main thread usage events and `UsageService`. | Live provider/cache superiority remains separate. |
| Reasonix absorption safety | 5 | 5 | Classifier behavior stays analytix-owned without Reasonix auto-plan config. | No user-visible auto-plan setting or Reasonix public protocol. |

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

## 2026-06-21 - Scheduled Detector Custom Endpoint Score Rule

Scope:

```text
Scheduled reminder detector tests now pin custom full endpoint inference for
`/messages` and `/chat/completions`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but evidence deepens because Schedule now has fake-fetch coverage for custom full endpoint body/header/parser inference. | Credentialed scheduled-task provider matrix remains open. |
| Product boundary governance | 4 | 4 | The detector keeps using analytix settings/runtime contracts and adds no public upstream protocol. | Packaged Schedule QA remains separate. |
| Reasonix/Kun absorption safety | 5 | 5 | The proof absorbs endpoint inference behavior without Reasonix provider protocol or Kun/deprecated settings identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has scheduled detector fake-fetch evidence for custom `/messages` and
`/chat/completions` endpoint inference.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-21 - Runtime Provider-Selection Request-Shape Score Rule

Scope:

```text
Runtime model-dispatch tests now pin thread provider selection to the expected
provider URL/header/body shape.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but evidence deepens because a selected custom provider and missing-provider fallback now execute through `MultiProviderModelClient` with fake fetch. | Credentialed provider matrix and live cache/cost comparison remain open. |
| Runtime contract reliability | 4 | 4 | Diagnostics stay on analytix provider id/base URL/endpoint format/configured model fields. | Does not prove packaged desktop settings UI or live provider behavior. |
| Reasonix/Kun absorption safety | 5 | 5 | The proof absorbs routing behavior without Reasonix provider protocol or Kun/deprecated settings identity. | Full live provider superiority remains unproven. |

Allowed wording:

```text
Analytix has runtime fake-fetch evidence that thread provider selection drives
the expected custom full endpoint and default fallback request shapes.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, full Reasonix provider parity,
full Kun current parity, release readiness, default Go backend, Reasonix public
protocol, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Auto-Router Classifier Request Contract Score Rule

Scope:

```text
Auto-router unit tests now pin the isolated classifier request shape, timeout
fallback, and classifier fingerprint currentness.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Auto-router/currentness | 4 | 4 | Score remains capped, but evidence deepens because `_auto_router` request shape and timeout fallback are directly tested. | No user-visible auto-plan/planner toggles or live controller rebuild UX. |
| Runtime contract reliability | 4 | 4 | Router calls use no tools, no prefix, short JSON mode, and an isolated turn id. | Does not prove packaged desktop model-routing QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Classifier/currentness behavior is absorbed without Reasonix config/protocol or Kun/deprecated identity. | Full live provider/cache superiority remains unproven. |

Allowed wording:

```text
Analytix has router-unit evidence that `_auto_router` stays an isolated
short-JSON classifier with timeout fallback and fingerprinted currentness.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan config parity, user-visible planner toggles,
Reasonix SessionAPI/controller protocol, Go auto-router parity, live
provider/cache superiority, full Kun current parity, release readiness, default
Go backend, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Planner-Executor Transcript Propagation Score Rule

Scope:

```text
Planner-executor runtime tests now prove identity-checked transcript refs are
persisted on durable executor jobs, and G5 shadow evidence replays the
requirement.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Sub-agent/job orchestration | 4 | 4 | Score remains capped, but evidence deepens because executor jobs now carry durable transcript refs after identity validation. | No packaged desktop sub-agent/nested-card QA. |
| Go G5 readiness | 3 | 3 | G5 shadow replays transcript propagation mode and requirement flags. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Transcript continuity is absorbed without Reasonix public protocol or Kun/deprecated identity. | Full Reasonix planner/executor parity remains unclaimed. |

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

## 2026-06-22 - MCP Search Meta-Tool Trust Score Rule

Scope:

```text
MCP search-mode runtime tests now prove meta-tools respect workspace trust and
approval before MCP client execution, with G4 shadow replay.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle | 4 | 4 | Score remains capped, but evidence deepens because `mcp_search` and `mcp_call` now prove trust/no-execute behavior. | No credentialed MCP server matrix or packaged MCP QA. |
| Go G4 readiness | 3 | 3 | G4 shadow replays meta-tool advertised/trust/no-execute fields. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP discovery is absorbed without Reasonix MCP-indexer protocol or Kun/deprecated identity. | Full live MCP parity remains unclaimed. |

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

## 2026-06-22 - Custom Messages Full Endpoint Score Rule

Scope:

```text
Provider-cache oracle now covers custom full `/messages` endpoint exact URL and
Anthropic Messages request shape.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but fake-fetch evidence now covers custom full `/messages` exact URL plus Messages body/header/tool shape. | No credentialed provider matrix or packaged settings QA. |
| Provider/cache proof | 4 | 4 | Oracle now covers custom `/responses` and `/messages` endpoint families. | No live provider/cache superiority claim. |
| Reasonix/Kun absorption safety | 5 | 5 | Request-shape behavior is absorbed without Reasonix provider protocol or Kun/deprecated identity. | Full live provider parity remains unclaimed. |

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

## 2026-06-23 - Go G3 Provider/Cache Streaming Live-Local Score Rule

Scope:

```text
Go runtime work now has a test/conformance-only local HTTP G3 provider/cache
streaming harness for fixture-backed usage parsing, request shape, SSE order,
cache accounting, drift attribution, and bounded diagnostics.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go runtime kernel | 3 | 3 | Score remains capped, but `live_local_provider.go` starts through the Go test harness and replays G3 provider/cache fixtures over local HTTP. | No production Go provider client, durable store, backend-neutral adapter switch, Electron integration, G6, or packaged QA. |
| Provider/cache/cost | 4 | 4 | DeepSeek native hit/miss precedence, OpenAI Responses cached tokens, Anthropic cache read/create fields, unsupported unknown telemetry, aggregate hit/miss accounting, stable-prefix diagnostics, and cache drift attribution are executable in Go live-local fixture replay. | No credentialed provider matrix, live cache telemetry, or live provider/cache superiority claim. |
| Runtime contract reliability | 4 | 4 | The G3 harness proves exact request-shape families and SSE `item_delta -> usage -> turn_completed` order without touching active `analytix serve`. | Production provider streaming, event recorder persistence, and packaged route walkthrough remain pending. |
| Product sovereignty | 5 | 5 | Tests and scan tokens keep no external network, no API-key read, no provider credentials, no Reasonix protocol, no renderer-visible Go route, and no default Go backend. | Full release readiness remains unclaimed. |

Allowed wording:

```text
Analytix Go live-local evidence improved to a fixture-backed G3 provider/cache
streaming prototype.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, G6 readiness,
packaged provider settings QA, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-23 - Go Isolated Mutating G2 Lifecycle Score Rule

Scope:

```text
Go runtime work advanced the test/conformance-only local sidecar from read-only
G1/G2 replay to isolated in-memory G2 lifecycle mutation replay.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go runtime kernel | 3 | 3 | Score remains capped, but `LiveLocalSidecarSnapshot` records archive/update/fork/resume fixture mutations while Go tests verify all G2 route status/body/SSE frames. | No durable Go event/session store, production router, Electron integration, G6, or packaged QA. |
| Runtime contract reliability | 4 | 4 | The four mutating G2 lifecycle fixtures execute in an isolated harness and a temp `events.jsonl` sentinel remains unchanged. | Durable replay, live event persistence, and packaged route walkthrough remain pending. |
| Product sovereignty | 5 | 5 | Boundary guards keep provider live calls, approval execution, MCP credentials, file mutation, renderer-visible Go routes, and default Go backend disabled. | Full release readiness remains unclaimed. |

Allowed wording:

```text
Analytix Go live-local evidence improved to an isolated mutating G2 lifecycle
prototype.
```

Forbidden wording:

```text
Do not claim Go default backend, G6 readiness, Electron integration, live
provider/cache superiority, credentialed MCP parity, approval/user-input gate
parity, file mutation parity, Reasonix SessionAPI/public protocol parity,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-23 - Go Live-Local Sidecar Prototype Score Rule

Scope:

```text
Go runtime work now has a test/conformance-only local HTTP sidecar prototype
for the G1/G2 read-only fixture slice.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go runtime kernel | 3 | 3 | Score remains capped, but `NewLiveLocalSidecarHandler` starts a real local Go HTTP server in tests and matches G1 health/info/tools plus G2 GET/SSE oracle output. | No live Go backend, Electron integration, durable store, provider client, MCP client, gate manager, file mutation, rollback/default-backend readiness, or packaged QA. |
| Runtime contract reliability | 4 | 4 | G1/G2 status/body/SSE fixtures are executable through the sidecar, and non-GET route replay is rejected. | Mutating route replay, live event persistence, and packaged route walkthrough remain pending. |
| Product sovereignty | 5 | 5 | G5 guards and product scan keep `electronMainConnected:false`, `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, no `/v1/runtime/go`, and no Rust/Tauri path. | Full release readiness remains unclaimed. |

Allowed wording:

```text
Analytix Go runtime has a live-local, fixture-backed sidecar prototype for the
read-only G1/G2 conformance slice.
```

Forbidden wording:

```text
Do not claim Go default backend, G6 readiness, Electron integration, live
provider/cache superiority, credentialed MCP parity, approval/user-input gate
parity, file mutation parity, Reasonix SessionAPI/public protocol parity,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Runtime HTTP Route Sovereignty Score Rule

Scope:

```text
G5 shadow now computes active `analytix serve` HTTP/SSE route sovereignty from
TS-owned route, router, HTTP server, endpoint-template, and HTTP-server-test
proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Source-derived proof now pins exact 44-route inventory, `/health` auth exception, `/v1/*` ownership, `/v1/threads/:id/events`, thread lifecycle, approval/user-input, and internal task-job routes. | No live Go HTTP server or packaged route walkthrough. |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now rejects Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route-token drift. | Negative fixtures retain forbidden strings as evidence. |
| Reasonix/Kun absorption safety | 5 | 5 | Route-surface discipline is absorbed without Reasonix SessionAPI, Kun identity, hidden top-level entries, or default Go backend. | Full live route parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay runtime HTTP/SSE route sovereignty and
prove the public `analytix serve` route table remains analytix-owned.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Renderer Runtime Endpoint Builder Score Rule

Scope:

```text
Renderer `AnalytixRuntimeProvider` now has tests proving GUI runtime calls use
shared analytix endpoint constants/builders and encoded dynamic ids.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | GUI provider root paths use shared constants; dynamic thread/turn/approval/user-input/session ids are encoded before bridge requests. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Renderer runtime provider calls remain analytix-owned `/health` or `/v1/*` paths and reject forbidden upstream/hidden route drift. | Negative fixtures retain forbidden strings as evidence. |
| Bridge safety | 4 | 4 | Sensitive approval/user-input/session/fork/turn route paths stay behind `window.analytix.runtime.runtimeRequest`. | No live Go HTTP server or G6 backend selection. |

Allowed wording:

```text
Analytix renderer runtime provider uses shared endpoint constants/builders and
encodes dynamic route ids before bridge requests.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Main IPC Endpoint Builder Allow-list Score Rule

Scope:

```text
Main IPC runtime request schema now has tests proving encoded shared endpoint
builder output is accepted and raw compatibility drift is rejected.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Main IPC accepts encoded shared builder output for thread, turn, checkpoint, approval, user-input, session, attachment, and memory paths. | No packaged desktop route walkthrough. |
| Bridge safety | 4 | 4 | Raw extra-segment paths and singular `/v1/user-input/:id` are rejected before the runtime adapter. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | 5 | 5 | The bridge allow-list remains analytix-owned `/health` or `/v1/*` templates, not Reasonix SessionAPI or hidden route names. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix main IPC runtime request allow-list accepts encoded shared endpoint
builder paths and rejects raw extra-segment compatibility drift.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Main IPC Runtime Adapter Handoff Score Rule

Scope:

```text
Main IPC handler tests now prove encoded shared endpoint paths are handed to
the runtime adapter unchanged and invalid compatibility drift is not forwarded.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | `runtime:request` handler preserves encoded path/method/body when calling `runtimeRequest`. | No packaged desktop route walkthrough. |
| Bridge safety | 4 | 4 | Raw extra-segment paths and singular `/v1/user-input/:id` reject before adapter invocation. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | 5 | 5 | Handler proof stays on `window.analytix.runtime.runtimeRequest` -> main IPC -> analytix runtime adapter, with no Reasonix public protocol. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix main IPC handler preserves encoded shared endpoint paths into the
runtime adapter and rejects invalid paths before adapter calls.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Preload Runtime Request Bridge Score Rule

Scope:

```text
Preload bridge tests now execute the exposed `analytix` API and prove runtime
request arguments enter the main `runtime:request` IPC channel unchanged.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | `api.runtime.runtimeRequest` preserves encoded path/method/body into `ipcRenderer.invoke('runtime:request', ...)`. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Diagnostics compatibility uses the same analytix IPC channel, not Reasonix/Kun/deprecated bridge aliases. | Negative fixtures retain forbidden strings as evidence. |
| Runtime contract safety | 4 | 4 | Preload proof joins renderer shared builders to main IPC validation and adapter handoff evidence. | No live Go HTTP server or G6 backend selection. |

Allowed wording:

```text
Analytix preload runtime request bridge preserves encoded shared endpoint paths
into the main runtime IPC channel.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Runtime Host URL Handoff Score Rule

Scope:

```text
Runtime adapter tests now prove encoded shared endpoint paths and request
metadata reach the managed runtime HTTP host unchanged.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | `runtimeRequestViaHost` preserves encoded path segments and query values when calling the local runtime host. | No packaged desktop route walkthrough. |
| Bridge safety | 4 | 4 | Method, bearer auth, custom header, content type, and body are preserved. | No live Go HTTP server or G6 backend selection. |
| Product sovereignty | 5 | 5 | Runtime host remains managed `analytix serve`, not Reasonix SessionAPI or default Go backend. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix runtime adapter preserves encoded shared endpoint paths into the
managed runtime HTTP host.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Preload SSE Bridge Score Rule

Scope:

```text
Preload SSE bridge tests now execute the exposed `analytix` API and prove
start/stop arguments plus event payloads stay on analytix-owned IPC channels.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | `api.runtime.startSse` / `stopSse` preserve thread id, cursor, and stream id into `runtime:sse:*`. | No packaged desktop SSE walkthrough. |
| Runtime contract safety | 4 | 4 | Event/end/error wrappers forward payloads only and clean up listeners. | No live Go SSE server or G6 backend selection. |
| Product sovereignty | 5 | 5 | SSE bridge remains `window.analytix` plus analytix IPC channels, not Reasonix SessionAPI or deprecated aliases. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix preload SSE bridge preserves start/stop arguments and payload-only
listener delivery on `window.analytix`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public SSE protocol parity, live Go SSE server
readiness, renderer-visible Go routes, default Go backend, packaged route QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-22 - Main SSE Host URL Encoding Score Rule

Scope:

```text
Main SSE IPC tests now prove renderer-provided thread ids are encoded before
fetching managed runtime event routes, with cursor headers preserved.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Dangerous thread ids are encoded into analytix-owned `/v1/threads/{id}/events`. | No packaged desktop SSE walkthrough. |
| Bridge safety | 4 | 4 | `since_seq`, `Last-Event-ID`, `Accept`, auth, and stream id error delivery are preserved. | No live Go SSE server or G6 backend selection. |
| Product sovereignty | 5 | 5 | SSE host route excludes Reasonix SessionAPI and hidden route families. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix main SSE IPC encodes thread ids before fetching managed runtime event
routes and preserves cursor headers.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public SSE protocol parity, live Go SSE server
readiness, renderer-visible Go routes, default Go backend, packaged route QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-22 - Renderer Runtime Client Bridge Score Rule

Scope:

```text
Renderer runtime client tests now prove runtime request and SSE facade calls go
through `window.analytix.runtime` without reading legacy bridge aliases.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Runtime request path/method/body and SSE start/stop/listener arguments are forwarded unchanged. | No packaged desktop runtime walkthrough. |
| Product sovereignty | 5 | 5 | Throwing `window.kun` / `window.reasonix` getters prove deprecated aliases are not read. | Negative fixtures retain forbidden strings as evidence. |
| Runtime contract safety | 4 | 4 | Renderer client proof connects GUI provider calls to preload/main/runtime host evidence. | No live Go bridge or G6 backend selection. |

Allowed wording:

```text
Analytix renderer runtime client preserves runtime request and SSE arguments
through `window.analytix.runtime` without legacy bridge fallback.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged route QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-22 - Renderer Settings Bridge Score Rule

Scope:

```text
Renderer runtime client tests now prove settings writes go through
`window.analytix.settings` with top-level `runtime` patches and without reading
legacy bridge aliases.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 4 | 4 | `runtime.model` and `runtime.approvalPolicy` patches are forwarded under top-level `runtime` and the returned settings refresh the renderer cache. | No packaged desktop settings walkthrough. |
| Product sovereignty | 5 | 5 | Throwing `window.kun` / `window.reasonix` getters prove deprecated aliases are not read. | Negative fixtures retain forbidden strings as evidence. |
| Runtime contract safety | 4 | 4 | Renderer settings proof connects GUI settings writes to the active `window.analytix` bridge. | No live Go bridge or G6 backend selection. |

Allowed wording:

```text
Analytix renderer settings client preserves top-level runtime settings patches
through `window.analytix.settings` without legacy bridge fallback.
```

Forbidden wording:

```text
Do not claim Reasonix config/public settings protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged settings
QA, release readiness, Kun identity, old runtime-shaped settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Renderer Runtime Provider Alias Guard Score Rule

Scope:

```text
Renderer runtime provider tests now run with throwing `window.kun` /
`window.reasonix` getters installed by the shared bridge helper.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Provider route, SSE, approval/user-input, fork/resume, and encoding tests keep legacy aliases unread. | No packaged desktop runtime walkthrough. |
| Product sovereignty | 5 | 5 | Provider routes remain analytix-owned `/health` or `/v1/*` and forbidden upstream route families stay absent. | Negative fixtures retain forbidden strings as evidence. |
| Approval/user-input route safety | 4 | 4 | Submit/cancel compatibility endpoint tests run under the alias guard. | No packaged approval-card QA. |

Allowed wording:

```text
Analytix renderer runtime provider tests run under a legacy alias guard while
exercising approval/user-input, fork/resume, thread lifecycle, and route
encoding paths.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged renderer
QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Renderer Provider Runtime Client Facade Seal Score Rule

Scope:

```text
Renderer runtime provider source now routes archive/restore through
`rendererRuntimeClient.runtimeRequest`, and product-sovereignty scan forbids
direct runtime request bridge bypasses in `analytix-runtime.ts`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Archive/restore now uses the same renderer runtime client facade as other provider runtime requests. | No packaged desktop runtime walkthrough. |
| Product sovereignty | 5 | 5 | The scan forbids direct `window.analytix.runtime.runtimeRequest` in provider source. | Negative fixtures retain forbidden strings as evidence. |
| Thread lifecycle route safety | 4 | 4 | Archive/restore keeps the same analytix thread route and body under the provider alias guard. | No packaged archive/search walkthrough. |

Allowed wording:

```text
Analytix renderer provider runtime requests are sealed behind
`rendererRuntimeClient`, including archive/restore.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged renderer
QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Side Conversation Relation Provider Contract Score Rule

Scope:

```text
Side conversation promotion now uses `AgentProvider.updateThreadRelation`, and
the Analytix provider sends relation PATCH requests through
`rendererRuntimeClient.runtimeRequest`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Store promotion no longer calls `window.analytix.runtime.runtimeRequest` directly. | No packaged side-conversation walkthrough. |
| Thread lifecycle route safety | 4 | 4 | Relation PATCH body is tested through the Analytix provider. | No live Go bridge or G6 backend selection. |
| Product sovereignty | 5 | 5 | The scan forbids direct runtime bridge requests in provider and side-store sources. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix side conversation promotion uses the provider relation contract and
keeps relation PATCH requests behind `rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/side-conversation protocol parity, live Go
bridge readiness, renderer-visible Go routes, default Go backend, packaged
side-conversation QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Renderer Usage Runtime Client Facade Seal Score Rule

Scope:

```text
Renderer thread/day/model usage, token economy savings, and LLM debug runtime
HTTP requests now use `rendererRuntimeClient.runtimeRequest`, and renderer
production source is scanned against direct generic runtime request bridge
bypass.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Generic renderer `/v1/*` runtime HTTP requests are sealed behind the client facade. | No packaged usage/dashboard walkthrough. |
| Provider/cache observability | 4 | 4 | Usage tests preserve cache/cost parsing and request paths after transport sealing. | No live provider/cache superiority matrix. |
| Product sovereignty | 5 | 5 | The scan forbids `window.analytix.runtime.runtimeRequest` in renderer production source. | Named preload APIs remain separate contracts. |

Allowed wording:

```text
Analytix renderer generic runtime HTTP requests are sealed behind
`rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/usage protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged
usage/dashboard QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Usage Runtime Client Facade G5 Shadow Score Rule

Scope:

```text
The D-0205 usage/debug runtime client facade seal is now replayed by
`desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Go G5 shadow now computes thread/day/model usage coverage, settings diagnostics coverage, direct bridge rejection, scan guard, and unit proof. | No packaged usage/dashboard walkthrough. |
| Provider/cache observability | 4 | 4 | Usage unit tests remain the behavior proof while shadow replay makes the facade seal executable. | No live provider/cache superiority matrix. |
| Go readiness | 3 | 3 | Evidence moved from source/scan-only into `BuildG5ControlExecutableOutput`. | No live Go desktop bridge or default backend. |

Allowed wording:

```text
Analytix G5 shadow replays renderer usage/debug facade evidence.
```

## 2026-06-22 - Renderer Settings Read Facade Seal Score Rule

Scope:

```text
Renderer keyboard shortcut, speech-to-text, and initial usage model-label
settings reads now use `rendererRuntimeClient.getSettings`, and renderer
production source is scanned against direct settings read bridge bypass.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 4 | 4 | Ordinary renderer settings reads are sealed behind the client facade. | No packaged settings walkthrough. |
| Bridge safety | 4 | 4 | The scan forbids direct `window.analytix.settings.getSettings` in renderer production source. | Named write APIs remain separate contracts. |
| Product sovereignty | 5 | 5 | No Reasonix config root, deprecated bridge alias, or old runtime-shaped settings fallback is introduced. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix renderer settings reads are sealed behind `rendererRuntimeClient`.
```

Forbidden wording:

```text
Do not claim Reasonix settings protocol parity, live Go bridge readiness,
renderer-visible Go routes, default Go backend, packaged settings QA, release
readiness, Kun identity, deprecated settings fallback, or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Settings Read Facade G5 Shadow Score Rule

Scope:

```text
The D-0206 settings-read facade seal is now replayed by
`desktopSovereignty.rendererSettingsReadFacadeMatrix` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 4 | 4 | Go G5 shadow now computes keyboard shortcut, speech-to-text, usage model-label, and settings-changed event evidence. | No packaged settings walkthrough. |
| Bridge safety | 4 | 4 | Go shadow replays direct settings bridge rejection and scan guard presence. | Named write APIs remain separate contracts. |
| Go readiness | 3 | 3 | Evidence moved from source/scan-only into `BuildG5ControlExecutableOutput`. | No live Go desktop/settings bridge or default backend. |

Allowed wording:

```text
Analytix G5 shadow replays renderer settings-read facade evidence.
```

## 2026-06-23 - AutoResearch Direction Tracking G5 Shadow Score Rule

Scope:

```text
Existing AutoResearch direction tracking is now replayed by
`controlExecutableCases.autoResearch` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Long-task reliability | 3 | 3 | Go G5 shadow now computes direction file, iteration-log, tool-name, and active-goal guard evidence. | No packaged long-task walkthrough. |
| Goal/runtime safety | 4 | 4 | Direction recording requires active research goals and unknown requirement evidence remains rejected. | No live Go research backend. |
| Product sovereignty | 5 | 5 | AutoResearch state remains project-local and no top-level AutoResearch route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go readiness | 3 | 3 | Evidence moved from source/test-only into `BuildG5ControlExecutableOutput`. | No live Go backend or default backend. |

Allowed wording:

```text
Analytix G5 shadow replays AutoResearch direction tracking evidence.
```

Forbidden wording:

```text
Do not claim Reasonix project protocol parity, live Go AutoResearch backend
readiness, renderer-visible Go routes, default Go backend, packaged long-task
QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-23 - MCP Call-Time Reconnect G5 Shadow Score Rule

Scope:

```text
Existing MCP call-time reconnect classification is now replayed by
`controlExecutableCases.mcpCallReconnect` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP reliability | 4 | 4 | Go G5 shadow now computes stale transport retry and protocol no-retry classification. | No credentialed MCP matrix. |
| Tool safety | 4 | 4 | Protocol validation errors return `tool_execution_failed` without reconnecting. | No packaged MCP walkthrough. |
| Product sovereignty | 5 | 5 | MCP remains internal runtime/tool evidence and no MCP-indexer route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go readiness | 3 | 3 | Evidence moved from source/test-only into `BuildG5ControlExecutableOutput`. | No live Go MCP client or default backend. |

Allowed wording:

```text
Analytix G5 shadow replays MCP call-time reconnect classification.
```

Forbidden wording:

```text
Do not claim Reasonix MCP protocol parity, live Go MCP client readiness,
renderer-visible Go routes, default Go backend, credentialed MCP matrix,
packaged MCP QA, release readiness, Kun identity, deprecated settings fallback,
or Rust/Tauri migration.
```

## 2026-06-23 - Plan Step/Cancel/Cache G5 Shadow Score Rule

Scope:

```text
Existing explicit Plan-mode step/cancel/cache behavior is now replayed by
`controlExecutableCases.planStepCancelCache` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Planner reliability | 4 | 4 | Go G5 shadow now computes Plan step-0 `create_plan` + `ls`, `bash` exclusion, and follow-up-only `create_plan`. | No public auto-plan setting or packaged Plan QA. |
| Cancel/cache stability | 4 | 4 | Aborted plan follow-up keeps the cache baseline unchanged and the retry reuses the original baseline. | No live Go loop claim. |
| Provider/cache accounting | 4 | 4 | DeepSeek `chat_completions` hit/miss telemetry is preserved as `80/20` across the cancelled Plan sequence. | No live provider superiority claim. |
| Product sovereignty | 5 | 5 | Planner evidence remains behind analytix runtime contracts and no Reasonix controller route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go readiness | 3 | 3 | Evidence moved from TS loop/source proof into `BuildG5ControlExecutableOutput`. | No default Go backend. |

Allowed wording:

```text
Analytix G5 shadow replays Plan step/cancel/cache stability.
```

Forbidden wording:

```text
Do not claim Reasonix controller/session protocol parity, public auto-plan
settings, live Go loop readiness, renderer-visible Go routes, default Go
backend, packaged Plan QA, release readiness, Kun identity, deprecated settings
fallback, or Rust/Tauri migration.
```

## 2026-06-23 - Plan/Auto-Route State Reset G5 Shadow Score Rule

Scope:

```text
Cancelled Plan mode state reset and post-cancel auto-route currentness are now
replayed by `controlExecutableCases.planCancelStateReset` and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Planner reliability | 4 | 4 | Runtime test and Go G5 shadow prove later normal/auto turns have no Plan mode instruction, no required `create_plan`, and no `create_plan` advertisement. | No public auto-plan setting. |
| Auto-router currentness | 4 | 4 | The next `model: "auto"` turn reruns `_auto_router` once with an isolated request and accepts the current recommendation. | No Reasonix controller protocol. |
| Product sovereignty | 5 | 5 | Reset/currentness evidence remains behind analytix runtime contracts and no auto-plan/controller route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go readiness | 3 | 3 | Evidence moved from TS runtime proof into `BuildG5ControlExecutableOutput`. | No live Go loop or default backend. |

Allowed wording:

```text
Analytix G5 shadow replays Plan/auto-route state reset after cancellation.
```

Forbidden wording:

```text
Do not claim Reasonix controller/session protocol parity, public auto-plan
settings, project overrides, live Go loop readiness, renderer-visible Go
routes, default Go backend, packaged Plan/auto-route QA, release readiness, Kun
identity, deprecated settings fallback, or Rust/Tauri migration.
```

## 2026-06-23 - MCP Search Refresh Drift Evidence Closure Score Rule

Scope:

```text
Existing MCP catalog refresh/currentness drift evidence is now required by
`scan:product-sovereignty` and stage closure docs.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP currentness | 4 | 4 | `mcp_refresh_catalog` records initial and expanded catalog names plus `totalIndexed: 2` / `catalogDrift: true`. | No credentialed MCP matrix. |
| Product sovereignty | 5 | 5 | Refresh remains internal; no public MCP-indexer route is exposed. | Negative scans retain forbidden strings as evidence. |
| Go readiness | 3 | 3 | Existing `mcpSearchRefreshDrift` Go G5 shadow proof is now scan-required. | No live Go MCP client or default backend. |

Allowed wording:

```text
Analytix closure evidence includes MCP search refresh drift currentness.
```

Forbidden wording:

```text
Do not claim Reasonix MCP protocol parity, live Go MCP client readiness,
renderer-visible Go routes, default Go backend, credentialed MCP matrix,
packaged MCP QA, release readiness, Kun identity, deprecated settings fallback,
or Rust/Tauri migration.
```

## 2026-06-22 - Renderer Named Bridge API Allow-list Score Rule

Scope:

```text
Remaining direct renderer `window.analytix.runtime.*` and
`window.analytix.settings.*` production calls are now scanned against explicit
named allow-lists, while generic runtime HTTP requests and ordinary settings
reads remain behind `rendererRuntimeClient`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | Direct renderer window bridge calls are restricted to named preload contracts. | No packaged bridge walkthrough. |
| Settings sovereignty | 4 | 4 | Direct settings reads stay forbidden; only named settings writes remain allow-listed. | No packaged settings walkthrough. |
| Product sovereignty | 5 | 5 | The scan fails on unlisted `window.analytix.runtime.*` / `window.analytix.settings.*` production calls. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix renderer direct window bridge calls are restricted to explicit named
preload contracts.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged bridge QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Renderer Optional Bridge Bypass Seal Score Rule

Scope:

```text
Optional-chaining direct bridge access is now scanned under the same renderer
bridge rules as dot access, and the remaining optional-chain generic
runtime/settings bypasses have been removed from production renderer source.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Bridge safety | 4 | 4 | The scan forbids both `window.analytix.runtime.runtimeRequest` and `window.analytix?.runtime?.runtimeRequest`. | No packaged bridge walkthrough. |
| Settings sovereignty | 4 | 4 | Connect Phone dialog settings reads now use `rendererRuntimeClient.getSettings`. | No packaged Connect Phone settings walkthrough. |
| Product sovereignty | 5 | 5 | Optional-chain direct methods must pass the same named API allow-list. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix renderer bridge scans cover optional-chain and dot direct access.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/bridge protocol parity, live Go bridge
readiness, renderer-visible Go routes, default Go backend, packaged bridge QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Renderer Bridge Allow-list G5 Shadow Score Rule

Scope:

```text
Renderer bridge allow-list evidence now participates in G5 desktop sovereignty
shadow replay without changing the live desktop bridge or enabling Go as a
backend.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Go shadow computes renderer named API allow-list status and bypass absence from TS-owned fixture input. | No live Go desktop bridge, rollback, or G6 backend selection. |
| Bridge safety | 4 | 4 | The direct bridge method inventory is source-derived from renderer production code. | No packaged bridge walkthrough. |
| Product sovereignty | 5 | 5 | The proof remains on `window.analytix` and rejects Reasonix/Kun bridge identities. | Negative fixtures retain forbidden strings as evidence. |

Allowed wording:

```text
Analytix G5 shadow executable-replays renderer bridge allow-list evidence.
```

Forbidden wording:

```text
Do not claim live Go bridge readiness, renderer-visible Go routes, default Go
backend, Reasonix SessionAPI/bridge protocol parity, packaged bridge QA,
release readiness, Kun identity, deprecated settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Runtime HTTP Auth Matrix G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes runtime HTTP auth matrix coverage from TS-owned
`analytix serve` route fixtures and real TypeScript dispatch proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but the auth matrix is now represented in both actual TS router dispatch and Go G5 shadow output. | No packaged desktop route/SSE walkthrough. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for route auth coverage without starting a Go HTTP server. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Route-auth value is absorbed without Reasonix public route protocol, Kun/deprecated identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay the runtime HTTP auth matrix for the
analytix-owned route table.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged route QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-23 - Runtime Forbidden Route Dispatch G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes runtime forbidden-route dispatch coverage from TS-owned
`analytix serve` route fixtures and real TypeScript valid-auth dispatch proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but forbidden route tokens are now represented in both actual TS router dispatch and Go G5 shadow output. | No packaged desktop route/SSE walkthrough. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for structured 404 route rejection without starting a Go HTTP server. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Route-rejection value is absorbed without Reasonix public route protocol, Kun/deprecated identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay the runtime forbidden-route dispatch
matrix for the analytix-owned route table.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged route QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-23 - Shared Endpoint Builder G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes shared endpoint builder encoding and template ownership
from TS-owned shared endpoint source and D-0192 unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but shared endpoint encoding is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop route/SSE walkthrough. |
| Product sovereignty | 5 | 5 | Exported endpoint strings stay analytix-owned and the singular user-input compatibility route is not exported as canonical. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for endpoint builder coverage without starting a Go HTTP server. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay shared endpoint builder encoding and
template ownership evidence.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged route QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Runtime Endpoint Builder G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes renderer provider endpoint-builder ownership and
dynamic route-id encoding from TS-owned renderer provider source and D-0193
unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but renderer provider endpoint construction is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop route/SSE walkthrough. |
| Product sovereignty | 5 | 5 | Renderer provider paths stay analytix-owned, use the runtime-client facade, and avoid Reasonix/Go/hidden route families. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for desktop endpoint builder coverage without starting a Go desktop bridge. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer provider endpoint-builder
ownership and dynamic route-id encoding before bridge runtime requests.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged route QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Main IPC Endpoint Builder G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes main IPC endpoint-builder allow-list acceptance and raw
route rejection from TS-owned main IPC schema source and D-0194 unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but main IPC endpoint-builder schema coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop IPC/route walkthrough. |
| Product sovereignty | 5 | 5 | Main IPC accepts encoded analytix-owned shared paths and rejects raw dynamic route ids plus singular user-input compatibility. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for main IPC boundary coverage without starting a Go main bridge. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay main IPC endpoint-builder allow-list
coverage and raw route rejection before runtime adapter handoff.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged IPC/route QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Main IPC Runtime Adapter Handoff G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes main IPC runtime-adapter handoff preservation and
reject-before-call behavior from TS-owned main IPC handler source and D-0195
unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but main IPC handler handoff coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop IPC/route walkthrough. |
| Product sovereignty | 5 | 5 | Encoded analytix-owned paths, methods, and bodies reach the adapter unchanged, while raw/singular paths are rejected before adapter invocation. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for main IPC handoff coverage without starting a Go main bridge. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay main IPC runtime-adapter handoff and
reject-before-call behavior.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged IPC/route QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Side Conversation Relation G5 Shadow Score Rule

Scope:

```text
Go G5 shadow now computes side conversation provider relation promotion,
refresh/close behavior, and direct bridge bypass scan coverage from TS-owned
side-store proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves side promotion stays behind `AgentProvider.updateThreadRelation` and direct side-store runtime bridge bypass stays scan-forbidden. | No packaged side walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Optional provider contract, provider promotion, refresh/close behavior, direct bridge rejection, and unit proof are now source-derived G5 proof. | No live Go desktop bridge or backend switch. |
| Go G5 readiness | 3 | 3 | G5 shadow replay quality improves for the renderer side-store boundary while Go remains shadow-only. | No live Go bridge, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay side conversation relation promotion
and prove it stays behind the provider relation contract.
```

Forbidden wording:

```text
Do not claim Reasonix side/session protocol parity, direct side-store runtime
bridge permission, packaged side QA, release readiness, default Go backend
readiness, Kun/deprecated bridge identity, or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Provider Facade Seal G5 Shadow Score Rule

Scope:

```text
Go G5 shadow now computes renderer provider archive/restore and relation PATCH
facade sealing plus direct bridge bypass scan coverage from TS-owned provider
proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves provider runtime calls stay behind `rendererRuntimeClient.runtimeRequest` and direct renderer bridge bypass stays scan-forbidden. | No packaged provider walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Archive/restore, relation PATCH, lifecycle unit proof, and direct-bypass scan guard are now source-derived G5 proof. | No live Go desktop bridge or backend switch. |
| Go G5 readiness | 3 | 3 | G5 shadow replay quality improves for the renderer provider facade boundary while Go remains shadow-only. | No live Go bridge, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer provider facade sealing and
prove archive/restore plus relation PATCH stay behind the runtime client
facade.
```

Forbidden wording:

```text
Do not claim direct renderer runtime bridge permission, Reasonix
SessionAPI/public bridge protocol parity, packaged provider QA, release
readiness, default Go backend readiness, Kun/deprecated bridge identity, or
Rust/Tauri migration.
```

## 2026-06-23 - Renderer Provider Alias Guard G5 Shadow Score Rule

Scope:

```text
Go G5 shadow now computes renderer provider route ownership,
lifecycle/gate/fork-resume coverage, dynamic encoding, and legacy alias unread
behavior from TS-owned provider proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves provider tests keep throwing `window.kun` / `window.reasonix` aliases unread. | No packaged provider walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Provider route, lifecycle, approval/user-input, fork/resume, dynamic encoding, and forbidden-route guard coverage are now source-derived G5 proof. | No live Go desktop bridge or backend switch. |
| Go G5 readiness | 3 | 3 | G5 shadow replay quality improves for the renderer provider boundary while Go remains shadow-only. | No live Go bridge, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer provider alias guard
behavior and prove provider routes stay analytix-owned under throwing legacy
aliases.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public bridge protocol parity, packaged
provider QA, release readiness, default Go backend readiness, Kun/deprecated
bridge identity, or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Settings Bridge G5 Shadow Score Rule

Scope:

```text
Go G5 shadow now computes renderer settings cache/write/top-level runtime patch
behavior from TS-owned renderer facade proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves renderer settings calls stay behind `window.analytix.settings` and legacy aliases are unread. | No packaged settings walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Settings read cache, setSettings refresh, and top-level `runtime` patch preservation are now source-derived G5 proof. | No live Go desktop bridge or backend switch. |
| Go G5 readiness | 3 | 3 | G5 shadow replay quality improves for the renderer settings boundary while Go remains shadow-only. | No live Go bridge, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer settings bridge behavior and
prove top-level `runtime` patches stay behind `window.analytix.settings`.
```

Forbidden wording:

```text
Do not claim Reasonix settings/config protocol parity, packaged settings QA,
release readiness, default Go backend readiness, Kun/deprecated bridge identity,
or Rust/Tauri migration.
```

## 2026-06-23 - Renderer Runtime Client Bridge G5 Shadow Score Rule

Scope:

```text
Go G5 shadow now computes renderer runtime client request/restart/SSE bridge
behavior from TS-owned renderer facade proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves renderer bridge calls stay behind `window.analytix` and legacy aliases are unread. | No packaged renderer walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Renderer request argument preservation, restart passthrough, and SSE control/listener passthrough are now source-derived G5 proof. | No live Go desktop bridge or backend switch. |
| Go G5 readiness | 3 | 3 | G5 shadow replay quality improves for the renderer bridge boundary while Go remains shadow-only. | No live Go bridge, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer runtime client bridge
behavior and prove request/restart/SSE calls stay behind `window.analytix`.
```

Forbidden wording:

```text
Do not claim Reasonix frontend/session protocol parity, packaged renderer QA,
release readiness, default Go backend readiness, Kun/deprecated bridge identity,
or Rust/Tauri migration.
```

## 2026-06-23 - Main SSE Host URL Encoding G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes main SSE host URL encoding, cursor/header preservation,
and forbidden-route guarding from TS-owned main SSE source and D-0199 unit
proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but main SSE host encoding coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop SSE walkthrough. |
| Product sovereignty | 5 | 5 | SSE host fetches remain on analytix `/v1/threads/{id}/events`, not Reasonix SessionAPI or Go public routes. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for main SSE host route coverage without starting a Go SSE server. | No live Go HTTP/SSE server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay main SSE host URL encoding and cursor
header preservation.
```

Forbidden wording:

```text
Do not claim live Go SSE server readiness, Reasonix SessionAPI/public SSE
protocol parity, default Go backend readiness, packaged SSE QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Preload SSE Bridge G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes preload SSE start/stop and payload-only listener
preservation from TS-owned preload source and D-0198 unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but preload SSE bridge coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop SSE walkthrough. |
| Product sovereignty | 5 | 5 | SSE traffic remains on `window.analytix` and analytix-owned `runtime:sse:*` IPC channels. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for preload SSE bridge coverage without starting a Go SSE server. | No live Go HTTP/SSE server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay preload SSE bridge start/stop and
payload-only listener behavior.
```

Forbidden wording:

```text
Do not claim live Go SSE server readiness, Reasonix SessionAPI/public SSE
protocol parity, default Go backend readiness, packaged SSE QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Runtime Host URL Handoff G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes runtime host URL handoff preservation from TS-owned
runtime adapter source and D-0197 unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but runtime host handoff coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Host requests remain targeted at managed `analytix serve`, not Reasonix SessionAPI or a Go public route. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for main-to-runtime host handoff without starting a live Go HTTP server. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay runtime host URL handoff and
method/header/body preservation.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged route QA, release
readiness, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri
migration.
```

## 2026-06-23 - Preload Runtime Request Bridge G5 Shadow Score Rule

Scope:

```text
G5 shadow now computes preload runtime/diagnostics request preservation from
TS-owned preload source and D-0196 unit proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but preload runtime request bridge coverage is now represented in both TS unit proof and Go G5 shadow output. | No packaged desktop preload/route walkthrough. |
| Product sovereignty | 5 | 5 | Runtime and diagnostics requests remain on `window.analytix` and the analytix-owned `runtime:request` IPC channel. | Negative fixtures retain forbidden strings as evidence. |
| Go G5 readiness | 3 | 3 | Shadow quality improves for preload bridge coverage without starting a Go preload bridge. | No live Go HTTP server, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay preload runtime request bridge and
diagnostics same-channel behavior.
```

Forbidden wording:

```text
Do not claim live Go HTTP server readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend readiness, packaged preload/route QA,
release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Shared Endpoint Builder Sovereignty Score Rule

Scope:

```text
Shared analytix endpoint builders now have unit evidence for route-id encoding,
canonical user-input templates, and forbidden upstream/hidden endpoint absence.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Shared builders encode ids for thread, turn, checkpoint, approval, user-input, session, attachment, and memory routes. | No live Go HTTP server or packaged route walkthrough. |
| Product sovereignty | 5 | 5 | Exported endpoint strings stay `/health` or `/v1/*` and contain no Reasonix/Kun/Go/Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer/session-api tokens. | Negative fixtures retain forbidden strings as evidence. |
| Approval/user-input route safety | 4 | 4 | Canonical shared user-input template is plural `/v1/user-inputs/{id}`; singular route is not exported as shared contract. | No packaged approval-card QA. |

Allowed wording:

```text
Analytix shared endpoint builders URL-encode route ids and keep exported
runtime endpoints analytix-owned.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Runtime Forbidden Route Dispatch Score Rule

Scope:

```text
The active TypeScript HTTP router now has a G5-fixture-driven forbidden route
dispatch matrix for upstream/hidden public route tokens.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but every D-0189 forbidden route token returns structured 404 with valid auth. | Negative fixtures retain forbidden strings as evidence. |
| Runtime contract safety | 4 | 4 | Forbidden route families are absent from the active router instead of being dormant authenticated stubs. | No live Go HTTP server or packaged route walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix, Kun, Go runtime, Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer public route tokens remain unregistered. | Full live route parity remains unclaimed. |

Allowed wording:

```text
Analytix has executable TypeScript HTTP evidence that forbidden upstream and
hidden-capability route tokens return structured not_found.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Runtime HTTP Auth Matrix Score Rule

Scope:

```text
The active TypeScript HTTP router now has a G5-fixture-driven unauthenticated
dispatch matrix for every registered D-0189 route.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | All 43 `/v1/*` route keys from `runtimeHttpRouteSovereignty.routes` return structured 401 when dispatched without auth; `/health` remains 200. | No live Go HTTP server or packaged route walkthrough. |
| Approval/user-input and task control safety | 4 | 4 | SSE, task-job, approval, user-input, and resume-thread routes are explicit members of the unauthorized matrix. | No packaged approval/task-job walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Auth coverage is proved through analytix route keys without importing Reasonix SessionAPI, Kun identity, or hidden entries. | Full live route parity remains unclaimed. |

Allowed wording:

```text
Analytix has executable TypeScript HTTP evidence that every registered `/v1/*`
runtime route rejects unauthenticated requests with the canonical structured
401.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route protocol parity, live Go HTTP
server readiness, renderer-visible Go routes, default Go backend, packaged
route QA, release readiness, Kun identity, deprecated settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Desktop Bridge IPC Sovereignty Score Rule

Scope:

```text
G5 desktop sovereignty now covers preload runtime request and SSE IPC bridge
ownership in addition to bridge name, public API types, and top-level runtime
settings shape.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but G5 shadow computes `runtimeRequestUsesAnalytixIpc` and `runtimeSseUsesAnalytixIpc` from source-derived fixture inputs. | No packaged desktop bridge walkthrough. |
| Product sovereignty | 5 | 5 | Forbidden Reasonix/Kun/Go/workflow IPC channels are recorded as absent, and shared public API types remain analytix-owned. | Full live release QA remains unclaimed. |
| Go G5 readiness | 3 | 3 | Go shadow computes the desktop bridge IPC proof without becoming a live backend. | No Electron Go integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has source-derived G5 shadow proof that desktop runtime request and
SSE traffic stay on `window.analytix.runtime` and `runtime:*` IPC channels.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/frontend protocol parity, renderer-visible Go
routes, default Go backend readiness, packaged desktop bridge QA, release
readiness, Kun identity, deprecated settings fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Desktop Main IPC Boundary Score Rule

Scope:

```text
G5 control shadow now covers main-process runtime request allow-list and SSE
cursor/stop/batch boundaries.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but G5 shadow computes strict request schema, allowed analytix routes, forbidden route rejection, and handler no-execute. | No packaged desktop bridge walkthrough. |
| SSE bridge safety | 4 | 4 | Main SSE proof covers strict start payload, `/v1/threads/{id}/events`, matching stop id, reconnect cursor, and 100ms batches. | No packaged restart/reconnect walkthrough. |
| Go G5 readiness | 3 | 3 | Go shadow computes main IPC boundary results without becoming a live backend. | No Electron Go integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has source-derived G5 shadow proof that Electron main enforces
runtime request allow-list and SSE cursor semantics under analytix contracts.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/main protocol parity, renderer-visible Go
routes, default Go backend readiness, packaged desktop bridge QA, release
readiness, Kun identity, deprecated settings fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Sub-Agent Review Matrix Score Rule

Scope:

```text
Parallel sub-agent findings are now reduced to a scan-visible post-881 review
matrix with explicit delta classifications and open gates.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Reasonix absorption governance | 4 | 4 | Score remains capped, but six review lanes and delta classifications are now machine-scannable. | No live Reasonix parity walkthrough. |
| Provider/cache reliability | 4 | 4 | Score remains capped, but custom provider proof is corrected to request-shape-only and raw cache accounting remains scoped to DeepSeek/OpenAI Responses/Anthropic. | No credentialed provider/cache superiority matrix. |
| Go G5 readiness | 3 | 3 | Score remains capped, but review evidence records executable shadow coverage and open G6 gates. | No live Go manager stack or rollback readiness. |
| Product sovereignty | 5 | 5 | No hidden top-level entry, Reasonix protocol, Kun identity, Go backend, or Rust/Tauri path is added. | Release readiness remains unclaimed. |

Allowed wording:

```text
Analytix has scan-visible sub-agent review evidence for post-881 absorption
decisions, provider/cache wording, Kun baseline preservation, and Go G5/G6
open gates.
```

Forbidden wording:

```text
Do not claim live Reasonix parity, live provider/cache superiority,
independent custom provider cache telemetry, default Go backend, packaged
desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Custom Provider Telemetry Seal Score Rule

Scope:

```text
Custom full endpoint request-shape evidence is now explicitly sealed away from
supported cache telemetry ids in TS conformance and Go G5 shadow.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but custom endpoint coverage is now request-shape-only by executable proof. | No credentialed custom provider cache matrix. |
| Go G5 readiness | 3 | 3 | Score remains capped, but Go shadow computes the same custom telemetry seal. | No live Go provider client or rollback readiness. |
| Product sovereignty | 5 | 5 | No Reasonix provider protocol, Kun identity, default Go backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains unclaimed. |

Allowed wording:

```text
Analytix proves custom full endpoints are request-shape-only in current
provider/cache fixtures and Go G5 control shadow.
```

Forbidden wording:

```text
Do not claim independent custom provider cache telemetry, live provider/cache
superiority, default Go backend, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - Goal Persistence Off-Lock Score Rule

Scope:

```text
Goal persistence off-lock behavior is now source-derived G5 control shadow
evidence, not only a future note.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but the previous future lock-fixture gap now has executable shadow evidence. | No live Go goal manager or rollback readiness. |
| Goal reliability | 4 | 4 | Source proof covers persist-before-event order and warning/error surfacing. | No packaged goal walkthrough. |
| Product sovereignty | 5 | 5 | No Reasonix controller protocol, Kun identity, default Go backend, hidden top-level entry, or Rust/Tauri path is added. | Release readiness remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow proves current goal persistence off-lock behavior from
source and tests.
```

Forbidden wording:

```text
Do not claim live Go goal persistence, TypeScript controller-lock migration,
Reasonix controller protocol parity, default Go backend, packaged goal QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Raw Accounting Proof Freshness Score Rule

Scope:

```text
Raw provider cache accounting now has a direct provider-cache oracle proof and
a product-sovereignty scan freshness token.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but `provider-cache-proof.test.ts` now derives raw usage snapshots and aggregate accounting from provider response bodies directly. | No credentialed live provider matrix or packaged provider QA. |
| Product sovereignty | 5 | 5 | Score remains capped, but `scan:product-sovereignty` now requires raw accounting proof tokens across post-881 runtime proof paths. | Scan freshness is not live provider parity. |
| Go G3/G5 readiness | 3 | 3 | No Go behavior changed; the TS oracle future Go code must match is stronger. | No live Go provider client, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has direct provider-cache oracle evidence and scan freshness for raw
provider cache accounting.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Combined Step/Cancel/Cache Proof Freshness Score Rule

Scope:

```text
Combined route-cache, step-limit, and cancel evidence now has a focused G5
oracle proof and a product-sovereignty scan freshness token.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Agent-loop reliability | 4 | 4 | Score remains capped, but `go-runtime-conformance.test.ts` now directly checks route-cache reuse, next-turn reroute, step-limit boundary, cancel result pairing, and stable-prefix isolation. | No packaged long-running cancel/cache walkthrough. |
| Product sovereignty | 5 | 5 | Score remains capped, but `scan:product-sovereignty` now requires combined step/cancel/cache proof tokens across post-881 runtime proof paths. | Scan freshness is not live loop parity. |
| Go G5 readiness | 3 | 3 | No Go behavior changed; the G5 oracle future Go code must match is stronger. | No live Go agent loop, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has focused G5 oracle evidence and scan freshness for combined
step/cancel/cache stability.
```

Forbidden wording:

```text
Do not claim live Go agent-loop parity, public auto-plan setting parity,
Reasonix controller/session protocol parity, default Go backend, packaged
cancel/cache QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Approval/User-Input Proof Freshness Score Rule

Scope:

```text
Approval/user-input gate safety now has a focused G5 oracle proof and a
product-sovereignty scan freshness token.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input safety | 4 | 4 | Score remains capped, but `go-runtime-conformance.test.ts` now directly checks denied no-execute, answer privacy, late rejection, abort cleanup, and resume cleanup. | No packaged desktop approval-card walkthrough. |
| Product sovereignty | 5 | 5 | Score remains capped, but `scan:product-sovereignty` now requires approval/user-input proof tokens across post-881 runtime proof paths. | Scan freshness is not live gate parity. |
| Go G5 readiness | 3 | 3 | No Go behavior changed; the G5 oracle future Go code must match is stronger. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has focused G5 oracle evidence and scan freshness for approval/user
input gate safety.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager parity, Reasonix ask/session
protocol parity, default Go backend, packaged approval-card QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Session Route Proof Freshness Score Rule

Scope:

```text
Thread/session route replay now has a focused G5 oracle proof and a
product-sovereignty scan freshness token.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime route safety | 4 | 4 | Score remains capped, but `go-runtime-conformance.test.ts` now directly checks archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, exact hashes, and boundary flags. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Score remains capped, but `scan:product-sovereignty` now requires session route proof tokens across post-881 runtime proof paths. | Scan freshness is not live route parity. |
| Go G5 readiness | 3 | 3 | No Go behavior changed; the G5 oracle future Go code must match is stronger. | No live Go thread/session router, Electron integration, rollback, or default backend. |

Allowed wording:

```text
Analytix has focused G5 oracle evidence and scan freshness for thread/session
route replay.
```

Forbidden wording:

```text
Do not claim live Go thread/session router parity, Reasonix SessionAPI/public
event protocol parity, default Go backend, packaged route walkthrough QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Release Evidence Final-Gate Freshness Score Rule

Scope:

```text
Post-881 release evidence final command gates are now machine-checkable by
`scan:product-sovereignty`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Stage closure evidence | 4 | 4 | Score remains capped, but `scan:product-sovereignty` now requires final command gate entries for D-0172 through D-0177. | The scan does not execute the full suite by itself. |
| Product sovereignty | 5 | 5 | Score remains capped, but release evidence path freshness is part of the same forbidden-surface scan. | Scan freshness is not packaged/live QA. |
| Release readiness | 3 | 3 | No behavior changed; evidence hygiene improves only. | Packaged desktop QA, live provider/MCP matrices, G6, signing, and distribution checks remain outside this batch. |

Allowed wording:

```text
Analytix product-sovereignty scan guards post-881 release evidence final gate
freshness.
```

Forbidden wording:

```text
Do not claim release readiness, packaged desktop QA, live provider/MCP matrix
completion, G6 readiness, default Go backend, Reasonix protocol parity, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - Post-881 Stage Closure Snapshot Score Rule

Scope:

```text
The current post-881 capability floor, stronger-than evidence, and remaining
open gates are now captured in a machine-scannable closure snapshot.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Stage closure evidence | 4 | 4 | Score remains capped, but `post-881-stage-closure-2026-06-22.md` records the current floor, absorbed deltas, Kun baseline, stronger-than areas, and open gates. | Snapshot is not live parity or release readiness. |
| Product sovereignty | 5 | 5 | Score remains capped, but `scan:product-sovereignty` requires closure tokens and forbidden-surface boundaries remain intact. | Scan freshness is not packaged/live QA. |
| Reasonix comparison | 4 | 4 | Score remains capped, but stronger-than claims are limited to verified fixture/contract/governance dimensions. | Live provider/cache superiority, live MCP parity, live Go backend, and packaged QA remain open. |

Allowed wording:

```text
Analytix has a machine-scannable post-881 stage closure snapshot for verified
fixture, contract, and governance evidence.
```

Forbidden wording:

```text
Do not claim live Reasonix parity, live provider/cache superiority, packaged
desktop QA, live Go backend readiness, G6 readiness, release readiness, Kun
identity, Reasonix public protocol parity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Cache Accounting Raw Payload Score Rule

Scope:

```text
Provider cache accounting now derives totals and provider-family coverage from
raw provider `responseBody.usage` fixtures before comparing expected usage.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but evidence deepens because accounting is computed from raw DeepSeek, OpenAI Responses, Anthropic, and unsupported payloads. | No credentialed live provider matrix or packaged provider QA. |
| Go G3/G5 readiness | 3 | 3 | Go shadow parses raw payload maps and computes raw parsed ids, telemetry-supported ids, totals, and aggregate rate from TS-owned fixtures. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache value is absorbed without Reasonix provider protocol, Kun identity, hidden top-level route, or active provider behavior change. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix has fixture-backed evidence that provider cache accounting is derived
from raw provider usage payloads in TS and Go G3/G5 shadow replay.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Browser Bridge And Visible Entry Sovereignty Score Rule

Scope:

```text
Browser preview bridge and visible sidebar entry surfaces now have source,
rendered-smoke, and scan path freshness evidence.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but evidence improves because the browser preview bridge and rendered sidebar are now guarded, not only preload/source routes. | No packaged desktop walkthrough. |
| Kun baseline preservation | 5 | 5 | Rendered sidebar rejects Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer labels while scan freshness covers Workbench/Sidebar/bridge paths. | Future workflow productization still needs a new spec. |
| Reasonix absorption safety | 5 | 5 | Reasonix public protocol and bridge aliases remain rejected while analytix runtime proxy/SSE paths are preserved. | No Reasonix public protocol parity is claimed. |

Allowed wording:

```text
Analytix has source, rendered-shell, and scan evidence that browser preview
bridge and visible entry surfaces stay on analytix-owned contracts.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, Reasonix public protocol parity, Kun
Workflow/Create Loop top-level navigation, Subagent/AutoResearch/MCP-indexer
entries, default Go backend, renderer-visible Go route, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - Browser Preview SSE Executable Bridge Score Rule

Scope:

```text
Browser preview SSE now has executable proxy-path, cursor, and event
normalization evidence.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but evidence improves because browser preview `startSse` is tested against analytix `/v1/threads/:id/events` proxy path and `Last-Event-ID`. | No packaged browser/desktop walkthrough. |
| Product sovereignty | 5 | 5 | Browser preview event payloads normalize to analytix `seq`/`kind` fields without Reasonix protocol exposure. | No Reasonix public protocol parity. |
| Go G5 readiness | 3 | 3 | No Go code changed; this is a renderer bridge proof only. | No live Go bridge, renderer-visible Go route, or default backend. |

Allowed wording:

```text
Analytix browser preview SSE bridge has executable evidence for analytix proxy
path, cursor header, and event normalization.
```

Forbidden wording:

```text
Do not claim packaged browser/desktop QA, Reasonix SessionAPI/public protocol
parity, live Go bridge, default Go backend, renderer-visible Go route, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Browser Preview Settings Sovereignty Score Rule

Scope:

```text
Browser preview settings load/re-save now prove legacy and Reasonix agent
envelopes are stripped while valid top-level runtime fields persist.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 5 | 5 | Score remains capped, but evidence improves because browser preview settings now share the same rejected-field normalization as desktop settings. | No packaged settings walkthrough. |
| Product sovereignty | 5 | 5 | `runtime.model` and `runtime.endpointFormat` persist under analytix-owned schema while `agentProvider`/`agents`/`deepseek`/`reasonix` and auto-plan fields are stripped. | No Reasonix config/auto-plan parity. |
| Go G5 readiness | 3 | 3 | No Go code changed; this is a shared settings/browser bridge proof. | No live Go settings bridge, renderer-visible Go route, or default backend. |

Allowed wording:

```text
Analytix browser preview settings drop legacy/Reasonix agent envelopes on load
and re-save while preserving valid top-level runtime settings.
```

Forbidden wording:

```text
Do not claim packaged settings QA, Reasonix config/auto-plan parity,
deprecated settings fallback, live Go bridge, default Go backend,
renderer-visible Go route, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - IPC Settings Patch Sovereignty Score Rule

Scope:

```text
Renderer-to-main settings patch validation now strips top-level legacy and
Reasonix envelopes before strict validation while preserving legal analytix
patch fields.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 5 | 5 | Score remains capped, but evidence improves because IPC patch validation now handles polluted top-level envelopes without saving them or losing legal patch fields. | No packaged settings walkthrough. |
| Product sovereignty | 5 | 5 | Runtime-nested legacy agent-shaped settings still reject rather than fallback. | No Reasonix config/auto-plan parity. |
| Go G5 readiness | 3 | 3 | No Go code changed; this is an IPC schema proof only. | No live Go settings bridge, renderer-visible Go route, or default backend. |

Allowed wording:

```text
Analytix IPC settings patches drop top-level legacy/Reasonix envelopes before
strict validation while preserving valid analytix settings patch fields.
```

Forbidden wording:

```text
Do not claim packaged settings QA, Reasonix config/auto-plan parity,
deprecated settings fallback, live Go bridge, default Go backend,
renderer-visible Go route, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Settings Sovereignty Scan Freshness Score Rule

Scope:

```text
Product-sovereignty scans now require settings/bridge sovereignty chokepoints
and their focused guard tests to remain present.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 5 | 5 | Score remains capped, but evidence improves because the scan now guards shared normalization, runtime settings, settings-store, IPC schema, preload, browser preview bridge, and their tests. | No packaged settings walkthrough. |
| Product sovereignty | 5 | 5 | Path freshness makes bridge/settings guard coverage harder to remove accidentally. | No Reasonix config/auto-plan parity. |
| Go G5 readiness | 3 | 3 | No Go code changed; this is a source scan proof only. | No live Go bridge, renderer-visible Go route, or default backend. |

Allowed wording:

```text
Analytix product-sovereignty scans include settings/bridge sovereignty
chokepoints and guard tests as required paths.
```

Forbidden wording:

```text
Do not claim packaged settings QA, Reasonix config/auto-plan parity,
deprecated settings fallback, live Go bridge, default Go backend,
renderer-visible Go route, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Workflow Singular Route Negative Score Rule

Scope:

```text
Live runtime HTTP route tests now cover singular `/v1/workflow` absence
alongside plural `/v1/workflows`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because live HTTP negative-route tests now cover the singular Workflow path recorded by G4/G5 fixtures. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer remain absent as public HTTP routes. | Internal capability code remains allowed behind analytix contracts. |
| Go G5 readiness | 3 | 3 | Go fixtures already replay singular `/v1/workflow`; no Go code changed. | No live Go HTTP route or default backend. |

Allowed wording:

```text
Analytix live HTTP route tests prove singular `/v1/workflow` remains absent
alongside plural `/v1/workflows`.
```

Forbidden wording:

```text
Do not claim Workflow product-entry readiness, Reasonix public protocol parity,
live Go routes, renderer-visible Go route, default Go backend, packaged desktop
QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Runtime Proof Freshness Score Rule

Scope:

```text
Product-sovereignty scans now require runtime conformance proof files and
active route/client source surfaces to remain present and clean.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because G2/G3/G4/G5 fixtures/tests and Go shadow sources are now path-freshness guarded. | No packaged desktop route/provider walkthrough. |
| Product sovereignty | 5 | 5 | Active runtime routes, endpoint templates, IPC schema, and renderer runtime client are scanned for forbidden public route/protocol strings. | Tests/fixtures intentionally retain forbidden paths as negative evidence. |
| Go G5 readiness | 3 | 3 | Conformance docs now reflect closed G2 exact replay and active provider/cache plus approval/user-input shadow evidence. | No live Go backend or default route. |

Allowed wording:

```text
Analytix product-sovereignty scans include runtime conformance proof freshness
and active route/client forbidden-surface checks.
```

Forbidden wording:

```text
Do not claim new provider/cache behavior, credentialed provider parity,
live Go routes, renderer-visible Go route, default Go backend, packaged route
or provider QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Renderer Runtime Request Surface Score Rule

Scope:

```text
Renderer provider behavior now proves common runtime requests stay on
analytix-owned HTTP routes and avoid forbidden upstream public surfaces.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because renderer provider calls now have executable request-path coverage. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Captured renderer paths reject Reasonix public routes, renderer-visible Go routes, and hidden-capability public routes. | Browser/main/preload route proofs remain separate evidence. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or default route. |

Allowed wording:

```text
Analytix renderer runtime provider requests stay on analytix-owned HTTP routes
and avoid forbidden upstream public surfaces.
```

Forbidden wording:

```text
Do not claim packaged desktop QA, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Renderer SSE Bridge Cursor Score Rule

Scope:

```text
Renderer SSE subscription behavior now proves stream cursor/start/cleanup stay
on the `window.analytix.runtime` bridge contract.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because renderer SSE now has executable bridge/cursor coverage. | No packaged desktop SSE walkthrough. |
| Product sovereignty | 5 | 5 | Renderer SSE does not use `runtimeRequest` as a side channel and does not expose Reasonix/Go/Workflow public surfaces. | Browser/main/preload reconnect and URL/header proofs remain separate evidence. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix renderer SSE subscription stays on the window.analytix bridge/cursor
contract and cleans up the same generated stream id.
```

Forbidden wording:

```text
Do not claim packaged desktop SSE QA, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Main/Preload SSE IPC Bridge Score Rule

Scope:

```text
Desktop SSE IPC behavior now proves preload channel mapping, main route/header
cursor handling, generated stream id continuity, and exact stop matching.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because preload/main SSE IPC now has route/cursor/stream lifecycle coverage. | No packaged desktop SSE walkthrough. |
| Product sovereignty | 5 | 5 | Desktop SSE remains under `window.analytix.runtime` and `runtime:sse:*` IPC without Reasonix/Kun/Go/Workflow public surfaces. | Live desktop restart/reconnect QA remains separate. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix desktop SSE IPC preserves the analytix-owned thread cursor and stream
id contract from preload to main.
```

Forbidden wording:

```text
Do not claim packaged desktop SSE QA, Reasonix public protocol parity, live Go
routes, renderer-visible Go route, default Go backend, Workflow product-entry
readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Main Runtime Request Forbidden Route Score Rule

Scope:

```text
Main IPC runtime request schema/handler behavior now explicitly rejects
Reasonix/Go/Workflow-style public route families before adapter calls.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because main IPC now has explicit forbidden-route and handler no-call coverage. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | `runtime:request` stays limited to modeled analytix endpoint templates without upstream public route families. | Live desktop route QA remains separate. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix main IPC rejects upstream public runtime routes before they reach the
runtime request adapter.
```

Forbidden wording:

```text
Do not claim packaged desktop route QA, Reasonix public protocol parity, live
Go routes, renderer-visible Go route, default Go backend, Workflow
product-entry readiness, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Preload Runtime Request IPC Bridge Score Rule

Scope:

```text
Preload runtime request source behavior now proves the bridge uses only the
analytix-owned `runtime:request` IPC channel.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because preload runtime request IPC mapping is pinned. | No packaged desktop route walkthrough. |
| Product sovereignty | 5 | 5 | Source guard rejects Reasonix/Kun/Go/Workflow request IPC channel names. | Live desktop route QA remains separate. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix preload runtime requests use only the analytix-owned runtime:request
IPC channel.
```

Forbidden wording:

```text
Do not claim packaged desktop route QA, Reasonix public protocol parity, live
Go routes, renderer-visible Go route, default Go backend, Workflow
product-entry readiness, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Provider Cache Coverage Floor Score Rule

Scope:

```text
Provider/cache oracle schema and runtime tests now make provider-family
coverage executable.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but the oracle now pins DeepSeek, OpenAI-compatible chat, OpenAI Responses, Anthropic Messages, and custom full endpoint coverage through schema and tests. | No credentialed live provider matrix or packaged provider QA. |
| Go G3/G5 readiness | 3 | 3 | Go shadow inputs remain aligned with the tightened TypeScript provider-cache oracle. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache coverage is strengthened without Reasonix provider protocol, Kun identity, or hidden top-level entries. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix provider/cache fixtures have an executable provider-family coverage
floor across DeepSeek, OpenAI-compatible, OpenAI Responses, Anthropic Messages,
and custom full endpoints.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
parity, Reasonix provider protocol parity, default Go backend readiness,
renderer-visible Go route, packaged provider QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - G5 Provider Cache Coverage Floor Control Shadow Score Rule

Scope:

```text
Go G5 executable control shadow now replays provider/cache coverage floor from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but G5 executable output now checks provider family coverage, telemetry-supported/unknown ids, and custom full endpoint exact URL/tool-shape ids. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes `providerCacheCoverageFloor` and Go tests compare it to the TS-owned oracle. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache coverage is replayed without Reasonix provider protocol, Kun identity, or hidden top-level entries. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow executable output replays provider/cache coverage floor from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider parity, live provider/cache superiority,
credentialed provider matrix readiness, Reasonix provider protocol parity,
default Go backend readiness, renderer-visible Go route, packaged provider QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Request-Shape Derived Replay Score Rule

Scope:

```text
Go G3/G5 shadow now derives provider request-shape URL/header/body/tool matches
from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but derived replay now checks exact URL, header, body, and tool shape for all seven request-shape cases. | No credentialed provider matrix or packaged settings QA. |
| Go G3/G5 readiness | 3 | 3 | Go shadow computes match ids and custom full endpoint appended-path count `0`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Request-shape value is absorbed without Reasonix provider protocol, Kun identity, deprecated bridge/settings fallback, or hidden top-level entry. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G3/G5 shadow derives provider request-shape URL/header/body/tool
matches from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Post-881 Runtime Proof Freshness V2 Score Rule

Scope:

```text
Product-sovereignty scan now guards key post-881 runtime proof leaves and
positive proof tokens.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but scan freshness now requires provider, planner/task, and MCP proof tokens. | No packaged desktop walkthrough or release QA. |
| Runtime contract safety | 4 | 4 | Task-job and MCP lifecycle oracle/test leaves are required by the reusable gate. | Scan freshness is not behavior parity. |
| Reasonix/Kun absorption safety | 5 | 5 | The guard protects post-881 proof chains without adding Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix product-sovereignty scan guards key post-881 runtime proof tokens.
```

Forbidden wording:

```text
Do not claim live provider/MCP/job parity, Reasonix public protocol parity,
default Go backend readiness, renderer-visible Go routes, packaged QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Runtime Desktop Bridge Proof Freshness Score Rule

Scope:

```text
Product-sovereignty scan now requires the desktop runtime bridge proof chain
source and test paths to remain present.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but evidence improves because renderer/preload/main runtime request/SSE proof files are scan-guarded. | No packaged desktop route/SSE walkthrough. |
| Product sovereignty | 5 | 5 | `scan:product-sovereignty` now fails if the bridge proof chain disappears. | Live desktop route QA remains separate. |
| Go G5 readiness | 3 | 3 | No Go code or fixture changed. | No live Go backend or renderer-visible Go route. |

Allowed wording:

```text
Analytix product-sovereignty scan guards the desktop runtime bridge proof
chain for path freshness.
```

Forbidden wording:

```text
Do not claim packaged desktop route/SSE QA, Reasonix public protocol parity,
live Go routes, renderer-visible Go route, default Go backend, Workflow
product-entry readiness, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Go G5 Auto-Router Recommendation/Currentness Score Rule

Scope:

```text
Go G5 shadow now executable-replays auto-router recommendation trust and
recent-context currentness from TS-owned auto-model-router fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Agent/kernel reliability | 4 | 4 | Score remains capped, but classifier currentness evidence now includes accepted/rejected recommendation parsing and active-turn exclusion. | No user-facing auto-plan setting or packaged desktop router QA. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes the expanded `autoRouterClassifier` output. | No live Go auto-router, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | The Reasonix classifier rebuild lesson is absorbed without Reasonix controller protocol, public config, Kun identity, or hidden top-level routes. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay auto-router recommendation parsing
and recent-context currentness from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix controller/session protocol parity, public auto-plan
settings, live Go auto-router readiness, default Go backend readiness,
packaged desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Exact Session Route Replay Score Rule

Scope:

```text
Go G5 shadow now executable-replays exact thread/session route contracts from
the TS-owned G2 route oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 4 | 4 | Score remains capped, but route proof now includes per-route method/path/auth/status/body hash/SSE hash for fork/resume/archive/search/SSE. | No packaged desktop route walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes exact route replay output, not only status/inventory summaries. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Session-route value is absorbed without Reasonix SessionAPI, Kun identity, deprecated bridge/settings fallback, or hidden top-level routes. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay exact thread/session route contracts
for fork/resume/archive/search/SSE from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI/public route parity, live Go HTTP server
readiness, renderer-visible Go routes, default Go backend readiness, packaged
desktop QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Context Compaction Boundary Score Rule

Scope:

```text
Go G5 shadow now executable-replays latest-compaction effective-history
boundary behavior from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Long-thread/runtime safety | 4 | 4 | Score remains capped, but executable shadow quality improves for latest compaction boundary and dropped pre-boundary items. | No packaged desktop long-history walkthrough. |
| Provider/cache proof | 4 | 4 | Compaction control state is explicitly excluded from the stable prefix. | No credentialed live cache matrix or live superiority claim. |
| Go G5 readiness | 3 | 3 | TS oracle and Go shadow now agree on effective/dropped ids for compaction boundary replay. | No live Go history manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | The behavior is absorbed without Reasonix SessionAPI/controller protocol, Kun identity, top-level hidden-capability entry, or deprecated bridge/settings fallback. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay latest-compaction effective-history
boundary behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go history manager readiness, default Go backend readiness,
Reasonix SessionAPI/controller protocol parity, packaged desktop long-history
QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 User-Input Structured Validation Score Rule

Scope:

```text
Go G5 shadow now executable-replays structured request_user_input validation
from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input safety | 4 | 4 | Score remains capped, but executable shadow quality improves for structured invalid request rejection and no-gate behavior. | No packaged desktop approval/user-input walkthrough. |
| Go G5 readiness | 3 | 3 | TS G4/G5 oracle and Go shadow now agree on validation case count and reject booleans. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | The behavior is absorbed without Reasonix ask/session protocol, Kun identity, top-level hidden-capability entry, or deprecated bridge/settings fallback. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay structured request_user_input
validation from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix ask/session protocol parity, packaged desktop approval-card
QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Planner Gate Matrix Score Rule

Scope:

```text
Go G5 shadow now executable-replays Plan mode enable/disable, step narrowing,
and blocked-tool rejection matrix from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Planner/tool-policy safety | 4 | 4 | Score remains capped, but executable shadow quality improves for normal-mode hiding, Plan gate, and forged blocked-tool rejection. | No packaged desktop Plan walkthrough. |
| Go G5 readiness | 3 | 3 | TS G5 oracle and Go shadow now agree on capability gate output, step output, rejected tool names, and rejection count. | No live Go planner/executor, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | The behavior is absorbed without Reasonix planner/session protocol, public auto-plan setting, Kun identity, or top-level hidden-capability entry. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay planner gate matrix from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim live Go planner/executor readiness, default Go backend readiness,
Reasonix planner/session protocol parity, public auto-plan setting, packaged
desktop Plan QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Desktop Bridge/Settings Sovereignty Score Rule

Scope:

```text
Go G5 shadow now executable-replays desktop bridge/settings sovereignty from
real TS desktop source proofs.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score stays capped, but evidence improves because G5 shadow now replays `window.analytix`, `Window.analytix`, and analytix-owned facade domains. | No packaged Electron smoke. |
| Settings/runtime contract safety | 4 | 4 | Settings proof ids cover Reasonix auto-plan drops, legacy agent envelope drops, and top-level `runtime` endpoint-format persistence. | No live settings migration walkthrough. |
| Go G5 readiness | 3 | 3 | TS desktop proofs and Go shadow agree on bridge/settings boundary output. | No live Go desktop integration, rollback, or default backend. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay desktop bridge/settings sovereignty
from TS-owned desktop source proofs.
```

Forbidden wording:

```text
Do not claim live Go desktop integration, default Go backend readiness,
Reasonix SessionAPI/config protocol parity, deprecated bridge/settings fallback
support, packaged desktop settings QA, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Task Planner Toolset Inventory Score Rule

Scope:

```text
Go G5 shadow now executable-replays task planner read-only and forbidden
toolset inventory from TS-owned task-job fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Planner/sub-agent safety | 4 | 4 | Score remains capped, but G5 control now checks read-only tools, forbidden `task`/`parallel_tasks`, counts, no-overlap, task-tool match, and policy values. | No packaged desktop sub-agent/planner walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes planner toolset inventory from TS-owned fixture inputs. | No live Go planner/executor, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Planner toolset value is absorbed without Reasonix public sub-agent/job/planner protocol or Kun/deprecated identity. | Full live planner/sub-agent parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay task planner toolset inventory from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go planner/executor readiness, Reasonix public sub-agent/job/
planner protocol parity, default Go backend, packaged desktop sub-agent QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Boundary Seal Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP core lifecycle provider identity and
product-boundary flags from TS-owned MCP fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle reliability | 4 | 4 | Score remains capped, but G5 control now checks `providerId: "mcp:research"`, no Reasonix protocol, and no top-level route exposure with lifecycle output. | No packaged desktop MCP walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes MCP provider/boundary seal from TS-owned fixture inputs. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP lifecycle value is absorbed without Reasonix MCP-indexer public protocol or Kun/deprecated identity. | Full live MCP parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP core lifecycle provider/boundary
seal from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP-indexer protocol parity,
credentialed MCP matrix readiness, default Go backend, packaged desktop MCP QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G3/G5 Provider Request-Shape Exact Matrix Score Rule

Scope:

```text
Go G3/G5 shadow now executable-replays provider request-shape exact matrix from
TS-owned provider/cache fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request-shape reliability | 4 | 4 | Score remains capped, but G3/G5 shadow now checks exact URLs, required/forbidden headers, required/forbidden body fields, reasoning presence, and tool-shape family for all 7 cases. | No credentialed provider matrix. |
| Go G5 readiness | 3 | 3 | `BuildG5ShadowSlicesOutput` and `BuildG5ControlExecutableOutput` compute exact matrix from TS-owned fixture inputs. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider request-shape value is absorbed without Reasonix provider protocol, deprecated settings fallback, or Kun identity. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G3/G5 shadow can executable-replay provider request-shape exact matrix
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go provider client readiness, Reasonix provider protocol
parity, credentialed provider matrix readiness, default Go backend, packaged
desktop provider QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Combined Step/Cancel/Cache Trace Score Rule

Scope:

```text
Go G5 shadow now executable-replays combined auto-route cache, step-limit, and
cancel trace from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Agent-loop reliability | 4 | 4 | Score remains capped, but G5 control now checks router call counts, model-step limit pairing, stable-prefix flags, cancel result counts, and exact result rows. | No packaged long-running cancel/cache walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes combined trace from fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Combined Reasonix-style control value is absorbed without public controller/session protocol, auto-plan product surface, or Kun identity. | Full live loop parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay combined step/cancel/cache trace from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, Reasonix controller/session protocol
parity, public auto-plan product surface, default Go backend, packaged desktop
cancel/cache QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Step-Limit Override/Delegate Matrix Score Rule

Scope:

```text
Go G5 shadow now executable-replays step-limit override and delegate inheritance
matrix from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Planner/sub-agent reliability | 4 | 4 | Score remains capped, but G5 control now checks default/user/session/turn/planner/headless/zero/delegate rows, delegate half, and min-floor behavior. | No packaged step-limit walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes step-limit matrix from fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Step-limit Reasonix-style control value is absorbed without public controller/session protocol, auto-plan product surface, or Kun identity. | Full live loop parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay step-limit override/delegate matrix
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, Reasonix controller/session protocol
parity, public auto-plan product surface, default Go backend, packaged desktop
step-limit QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 History Repair Pair-Integrity Score Rule

Scope:

```text
Go G5 shadow now executable-replays model-history repair pair integrity from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but G5 control now checks complete tool blocks, orphan result drops, missing-result call drops, duplicate result drops, and bridge text preservation. | No packaged long-history walkthrough. |
| Provider/cache proof | 4 | 4 | Repair state is explicitly kept out of stable prefix/cache material. | No credentialed long-session cache matrix. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes history repair output from fixture inputs. | No live Go agent loop, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | History legality value is absorbed without Reasonix SessionAPI/controller protocol, public auto-plan surface, or Kun identity. | Full live loop parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay history repair pair integrity from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go agent-loop readiness, live Go history-manager readiness,
Reasonix SessionAPI/controller protocol parity, public auto-plan product
surface, default Go backend, packaged desktop long-history QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Approval/User-Input Inventory Score Rule

Scope:

```text
Go G5 shadow now executable-replays approval/user-input gate inventory and
answer privacy flags from TS-owned approval/user-input fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input reliability | 4 | 4 | Score remains capped, but G5 control now checks gate ids, approval/user-input id groups, route kinds, replay kind order, HTTP answer echo, resolved-event no-answer privacy, late statuses, and pending-after sum. | No packaged desktop approval-card/user-input walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes approval/user-input inventory from TS-owned fixture inputs. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Gate inventory value is absorbed without Reasonix ask/session protocol or Kun/deprecated identity. | Full live gate parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay approval/user-input gate inventory
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, Reasonix ask/session
protocol parity, default Go backend, packaged desktop approval-card QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Session Route Inventory Score Rule

Scope:

```text
Go G5 shadow now executable-replays thread/session route inventory from
TS-owned G2 route fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime route contract | 4 | 4 | Score remains capped, but G5 control now checks route ids, JSON/SSE/event/resume/fork/archive/search/read-update groups, runtime-token count, and unauthorized route ids. | No packaged desktop thread/fork/resume/archive/search walkthrough. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes session route inventory from TS-owned fixture inputs. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Route inventory value is absorbed without Reasonix SessionAPI/public route protocol or Kun/deprecated identity. | Full live route parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay thread/session route inventory from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go routing readiness, Reasonix SessionAPI/public route
protocol parity, default Go backend, packaged desktop route QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Provider Cache Inventory Score Rule

Scope:

```text
Go G5 shadow now executable-replays provider cache prefix/tool/case-id
inventory from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but G5 control now checks stable prefix hash, tools hash, usage/request-shape ids and counts, prefix equivalence, and tools hash stability. | No credentialed provider matrix or packaged settings QA. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes provider cache inventory from TS-owned fixture inputs. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache inventory value is absorbed without Reasonix provider protocol or Kun/deprecated identity. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider cache prefix/tool/case-id
inventory from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Provider Cache Privacy Score Rule

Scope:

```text
Go G5 shadow now executable-replays provider cache diagnostics privacy and
no-live-superiority policy from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but G5 control now checks bounded diagnostics, forbidden substring no-leak, no live credentials, and no live superiority claim. | No credentialed provider matrix or packaged settings QA. |
| Go G5 readiness | 3 | 3 | `BuildG5ControlExecutableOutput` computes provider cache privacy from TS-owned fixture inputs. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache privacy value is absorbed without Reasonix provider protocol or Kun/deprecated identity. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider cache diagnostics privacy
and no-live-superiority policy from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, packaged
provider settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Approval/User-Input Route Replay Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays approval/user-input route behavior from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for approval/user-input route replay. | No live Go approval/user-input manager, Electron integration, rollback, G6 readiness, or default backend. |
| Approval/user-input safety | 4 | 4 | TS oracle and Go shadow now agree on approval deny, submit/cancel, replay order, late action rejection, and abort cleanup. | No packaged desktop approval-card QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Gate lifecycle value is absorbed without Reasonix SessionAPI/ask protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay approval/user-input route behavior
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, top-level hidden-capability
navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Search Refresh Drift Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP search refresh catalog drift from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP refresh drift. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on catalog expansion, indexed count, drift, and no top-level route. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP lifecycle value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP search refresh catalog drift from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP core lifecycle behavior from TS-owned
fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP core lifecycle replay. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on connect/disconnect diagnostics, reload schema order, cancel no-execute, and approved error shape. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP lifecycle value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP core lifecycle behavior from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Product Boundary Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays product-boundary invariants from TS-owned
fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but product-boundary output now gates control executable replay. | No live Go backend, Electron integration, rollback, G6 readiness, or default backend. |
| Product sovereignty | 5 | 5 | TS oracle and Go shadow agree on bridge/serve stability, no Reasonix protocol, no renderer-visible Go route, and no enabled Go backend. | Packaged desktop QA and release readiness remain unclaimed. |
| Reasonix/Kun absorption safety | 5 | 5 | Upstream capability absorption stays behind analytix contracts and forbidden route-surface scans. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay product-boundary invariants from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go backend readiness, default Go backend readiness, Reasonix
public protocol parity, renderer-visible Go route, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer navigation, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Nested Child SSE Metadata Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays nested child SSE metadata invariants from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but nested child SSE metadata output now gates control executable replay. | No live Go Job Manager, Electron integration, rollback, G6 readiness, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow agree on parent call id, child run id, metadata field coverage, evidence ledger keys, active-goal requirement, and no Reasonix protocol. | No packaged nested-card QA or live Go task-job routes. |
| Timeline/projection safety | 4 | 4 | Child run attribution is analytix-owned and not imported as Reasonix SessionAPI. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay nested child SSE metadata from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix SessionAPI or public sub-agent/job protocol parity, renderer-visible
Go route, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Task Transcript Identity Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays task transcript continue/fork identity
invariants from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but transcript identity output now gates control executable replay. | No live Go Job Manager, Electron integration, rollback, G6 readiness, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow agree on source id, continue target, fork target, incompatible error, continue-target-matches-source, fork-target-distinct-from-source, and no Reasonix protocol. | No packaged nested-card QA or live Go task-job routes. |
| Thread/fork safety | 4 | 4 | Continue/fork identity is analytix-owned and not imported as Reasonix SessionAPI. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay task transcript identity from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix SessionAPI or public sub-agent/job protocol parity, renderer-visible
Go route, top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Task-Job Tool Contract Boundary Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays task-job tool contract and route-boundary
invariants from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but task-job tool-boundary output now gates control executable replay. | No live Go Job Manager, Electron integration, rollback, G6 readiness, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow agree on internal-only task tools, permission/evidence/dependency/read-only gates, runtime route auth, forbidden top-level routes, and no Reasonix protocol. | No packaged nested-card QA or live Go task-job routes. |
| Reasonix/Kun absorption safety | 5 | 5 | Sub-agent/job value is absorbed without top-level Subagent/Workflow/Create Loop/AutoResearch/MCP-indexer navigation or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay task-job tool contract boundary from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go Job Manager readiness, default Go backend readiness,
Reasonix public sub-agent/job protocol parity, renderer-visible Go route,
top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Background Reconnect Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP background reconnect behavior from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP reconnect/retry replay. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on failed server ids, suspended provider/reason, connected/error outcomes, retry attempts, and no-runtime-restart state. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP reconnect value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP background reconnect behavior
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Known Override Diagnostics Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP known override diagnostics from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP known override diagnostics. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on override kind, effective cwd, workspace root, explicit-cwd server ids, daemon-timeout server ids, low priority, and background start. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP config/diagnostic value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP known override diagnostics from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Live-Local Indexer Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP live-local indexer lifecycle from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP live-local indexer lifecycle. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on retry ids/map, initial/resume/active paths, tombstone count, restart status, diagnostic redaction, and execution-error redaction. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP/indexer lifecycle value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP live-local indexer lifecycle from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Search Meta-Tool Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP search meta-tool advertised/trust/
no-execute behavior from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP search meta-tool trust/no-execute. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on meta-tool names/count, refresh advertisement, workspace trust fields, unknown-tool error, `on-request` policy, and denied no-execute. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP search/indexer trust value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP search meta-tool advertised/trust/
no-execute behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Approval Annotation Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP approval annotations and denied
no-execute behavior from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP approval annotation replay. | No live Go MCP client, live Go approval manager, Electron integration, rollback, G6 readiness, or default backend. |
| MCP approval safety | 4 | 4 | TS oracle and Go shadow now agree on destructive/open-world hints, normalized tool name, deny decision, and denied no-execute. | No packaged MCP/approval QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP approval lifecycle value is absorbed without Reasonix MCP-indexer/approval protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP approval annotations and denied
no-execute behavior from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, live Go approval manager readiness,
default Go backend readiness, Reasonix MCP-indexer/approval protocol parity,
top-level MCP-indexer navigation, credentialed MCP matrix readiness, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Search Workspace Boundary Control Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays MCP search workspace trust boundaries from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP workspace-boundary replay. | No live Go MCP client, Electron integration, rollback, G6 readiness, or default backend. |
| MCP lifecycle safety | 4 | 4 | TS oracle and Go shadow now agree on trusted/untrusted search, unknown-tool error, call policy, and denied no-execute. | No credentialed MCP matrix or packaged MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP workspace trust value is absorbed without Reasonix MCP-indexer protocol, top-level MCP-indexer navigation, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay MCP search workspace trust boundaries
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation,
credentialed MCP matrix readiness, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Task-Job Route Executable Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays task-job route status behavior from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but typed control shadow now computes task-job route executable output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | Route executable status/offset/rehydration evidence is now in `controlExecutableCases.taskJobs`. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Route behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay task-job route status behavior from
TS-owned fixtures without exposing Go or Reasonix routes.
```

Forbidden wording:

```text
Do not claim live Go task-job route readiness, default Go backend readiness,
Reasonix SessionAPI/job protocol parity, top-level Subagent navigation, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Task-Job Lifecycle Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays task-job lifecycle behavior from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but typed control shadow now computes task-job lifecycle output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | Foreground/background/wait-output-kill lifecycle evidence is now in `controlExecutableCases.taskJobs`. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Lifecycle behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay task-job lifecycle behavior from
TS-owned fixtures without exposing Go or Reasonix lifecycle routes.
```

Forbidden wording:

```text
Do not claim live Go task-job lifecycle readiness, default Go backend readiness,
Reasonix SessionAPI/job protocol parity, top-level Subagent navigation, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Planner-Executor Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays planner/executor propagation from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but typed control shadow now computes planner/executor propagation output. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | Failure/cancellation/output-offset/transcript propagation evidence is now in `controlExecutableCases.taskJobs`. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Planner/executor behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay planner/executor propagation from
TS-owned fixtures without exposing Go or Reasonix planner/job routes.
```

Forbidden wording:

```text
Do not claim live Go planner/job readiness, default Go backend readiness,
Reasonix SessionAPI/planner protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Cache Release Guard Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider cache release guard from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes release-guard tail averages, allowed-low cases, collapse counts, and status. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerCacheReleaseGuard`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider cache release guard from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity,
top-level hidden navigation, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Provider Usage Parser Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider usage parser precedence
from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes DeepSeek native precedence, Responses cached tokens, Anthropic cache fields, and unsupported fallback. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerUsageParser`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider usage parser precedence from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity,
top-level hidden navigation, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Provider Request Shape Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider request shape from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but typed control shadow now computes exact URL count, endpoint families, full endpoint ids, tool shapes, and body-field family counts. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerRequestShape`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider request-shape behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider request shape from TS-owned
fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity,
top-level hidden navigation, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Provider Cache Accounting Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider cache accounting from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes supported telemetry ids, unsupported ids, provider-family ids, hit/miss totals, and aggregate hit rate. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerCacheAccounting`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider cache accounting is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider cache accounting from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity,
top-level hidden navigation, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Release/Package Identity Sovereignty Score Rule

Scope:

```text
Release/package identity is now covered by product-sovereignty scan and
packaging configuration assertions.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but release/package identity now has explicit scan and packaging-test coverage. | No packaged Electron smoke or signed release QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Package manifests, builder config, scripts, workflows, build config, and runtime bin reject Kun/Reasonix identity leaks. | Historical compatibility code remains quarantined where documented. |
| Release readiness | 2 | 2 | Source/config guard improves evidence but does not change release readiness. | Signing, notarization, update-channel, and packaged artifact QA remain separate. |

Allowed wording:

```text
Analytix release/package identity is guarded at source/config level.
```

Forbidden wording:

```text
Do not claim packaged Electron smoke, signed release readiness, Kun/Reasonix
public identity acceptance, default Go backend readiness, top-level hidden
navigation, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Streaming Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider streaming usage from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes SSE event order, usage match, token/cache fields, cache hit rate, and telemetry support. | No credentialed live streaming provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerStreaming`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider streaming behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider streaming usage from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed streaming provider matrix
parity, default Go backend readiness, Reasonix provider/cache protocol parity,
top-level hidden navigation, release readiness, Kun identity, or Rust/Tauri
migration.
```

## 2026-06-22 - Provider Offline Parity Seal Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider offline parity seal from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes stable prefix equivalence, DeepSeek cache telemetry, request-shape endpoint formats, release guard status, and fixture-only policy. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerOfflineParitySeal`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Offline parity behavior is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider offline parity seal from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity, top-level
hidden navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Session Route Status Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays thread/session route status from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract safety | 5 | 5 | Score remains capped, but typed control shadow now computes archive/search, read/update, fork, resume, SSE replay/caught-up, and missing-token auth status. | No packaged desktop route walkthrough. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.sessionRouteStatus`. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Session route behavior is absorbed without Reasonix SessionAPI/job protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay thread/session route status from
TS-owned fixtures without exposing Go or Reasonix session routes.
```

Forbidden wording:

```text
Do not claim live Go thread/session routes, default Go backend readiness,
Reasonix SessionAPI/job protocol parity, top-level hidden navigation, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Live-Local HTTP Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider live-local HTTP proof from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes fixture policy, usage/request-shape counts, endpoint formats, provider families, and no-live-superiority status. | No credentialed live provider matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerLiveLocalHttpProof`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Local HTTP provider proof is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider live-local HTTP proof from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed provider matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity, top-level
hidden navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Drift Attribution Control Shadow Score Rule

Scope:

```text
Go G5 control shadow now executable-replays provider drift attribution from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache reliability | 4 | 4 | Score remains capped, but typed control shadow now computes prefix hashes, stable system/prefix-items hashes, tools/provider/model/endpoint drift, expected reasons, and telemetry status. | No credentialed live cache matrix or packaged provider QA. |
| Go G5 readiness | 3 | 3 | G5 evidence improves through `controlExecutableCases.providerDriftAttribution`. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Drift attribution is absorbed without Reasonix protocol, Kun identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay provider drift attribution from
TS-owned fixtures without exposing Go or Reasonix provider routes.
```

Forbidden wording:

```text
Do not claim live provider superiority, credentialed cache matrix parity,
default Go backend readiness, Reasonix provider/cache protocol parity, top-level
hidden navigation, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - MCP Refresh Catalog Drift Score Rule

Scope:

```text
MCP search catalog refresh drift is now executable in the TS oracle and
replayed by G5 shadow output.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle/currentness | 4 | 4 | Score remains capped, but evidence deepens because catalog expansion plus `mcp_refresh_catalog` now proves `catalogDrift: true`. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | 3 | 3 | G5 shadow replays `mcpReplay.searchRefreshDrift` from TS-owned fixtures only. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix MCP currentness value is absorbed without Reasonix MCP-indexer protocol, Kun identity, deprecated bridge/settings fallback, or hidden top-level entry. | Full live MCP parity remains unclaimed. |

Allowed wording:

```text
Analytix has deterministic evidence that MCP catalog refresh detects search
catalog drift and G5 shadow replays the result.
```

Forbidden wording:

```text
Do not claim Reasonix MCP-indexer public lifecycle parity, top-level
MCP-indexer navigation, live Go MCP client readiness, default Go backend,
credentialed MCP matrix readiness, packaged desktop MCP QA, release readiness,
Kun identity, deprecated bridge/settings fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Thread/SSE Route Auth Score Rule

Scope:

```text
G2/G5 route replay now proves missing-token rejection for thread SSE replay.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime route safety | 4 | 4 | Score remains capped, but route replay now includes structured 401 for missing-token thread events. | No packaged desktop route walkthrough. |
| Go G5 readiness | 3 | 3 | G5 shadow replays `routeStatusReplay.auth` from G2 fixtures only. | No live Go HTTP server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Thread/SSE value is absorbed without Reasonix SessionAPI, Kun identity, deprecated bridge/settings fallback, or hidden top-level entry. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix has deterministic route evidence that thread SSE replay rejects
missing runtime tokens and Go G5 shadow replays the boundary.
```

Forbidden wording:

```text
Do not claim Reasonix SessionAPI parity, unauthenticated SSE replay, live Go
HTTP route readiness, default Go backend, packaged desktop restart/resume QA,
release readiness, Kun identity, deprecated bridge/settings fallback, or
Rust/Tauri migration.
```

## 2026-06-22 - Runtime Settings Legacy-Agent Guard Score Rule

Scope:

```text
Active runtime settings now strip or reject Reasonix/Kun-shaped agent fields
when they appear under `runtime`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Settings sovereignty | 5 | 5 | Score remains capped, but evidence deepens because runtime-contained upstream agent shapes cannot persist. | No packaged settings walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Upstream config shapes remain rejected/deferred unless reimplemented as analytix-owned runtime fields. | User-visible auto-plan settings remain unclaimed. |
| Product boundary | 5 | 5 | No bridge alias, deprecated settings fallback, Reasonix protocol, Kun identity, default Go backend, or Rust/Tauri path is added. | Release readiness remains separate. |

Allowed wording:

```text
Analytix active runtime settings strip or reject Reasonix/Kun-shaped agent
fields before persistence.
```

Forbidden wording:

```text
Do not claim Reasonix config parity, Kun legacy agent settings as an active
save path, deprecated bridge/settings fallback, packaged settings QA, release
readiness, default Go backend, or Rust/Tauri migration.
```

## 2026-06-22 - Plan Step Cancel/Cache Score Rule

Evidence added:

```text
packages/runtime/tests/loop.test.ts proves that explicit analytix Plan mode
keeps step gating and cache diagnostics stable across an interrupted follow-up
step.
```

Score impact:

| Capability | Before | After | Change |
| --- | --- | --- | --- |
| Planner step gating | deterministic Plan mode tests existed, but not the cancel/cache combination | step 0 read-only+`create_plan`, step 1 `create_plan`-only, interrupted step, and next-turn cache baseline are covered together | evidence improves |
| Provider cache accounting | provider/cache fixtures covered DeepSeek-style telemetry | Plan-mode cancellation now also preserves native hit/miss/rate diagnostics | evidence improves |
| Product sovereignty | protected | unchanged; no upstream protocol or top-level route added | no score change |

Rejected score claims:

```text
No packaged Plan mode QA, live provider superiority, Go G6 readiness, or
release readiness is claimed by this fixture.
```

## 2026-06-22 - MCP Stdio Execution-Error Redaction Score Rule

Evidence added:

```text
packages/runtime/tests/mcp-tool-lifecycle-oracle.test.ts proves that the
executable stdio fake MCP indexer can return a protocol-level `isError`
payload without leaking `Bearer lifecycle-secret` into model-visible tool
results.
```

Score impact:

| Capability | Before | After | Change |
| --- | --- | --- | --- |
| MCP privacy | diagnostics redaction covered summaries and G5 replay | tool-result payload redaction now covers actual stdio MCP execution errors | evidence improves |
| MCP lifecycle | live-local restart/tombstone proof existed | failing diagnostic tool path is also executable | evidence improves |
| Product sovereignty | protected | unchanged; no MCP-indexer protocol or top-level entry added | no score change |

Rejected score claims:

```text
No credentialed MCP matrix, packaged MCP QA, Go MCP client readiness, default
Go backend, or release readiness is claimed by this fixture.
```

## 2026-06-22 - Workflow/Create Loop Quarantine Score Rule

Evidence added:

```text
Workbench route-surface tests and `scan:product-sovereignty` now reject
Workflow/Create Loop implementation symbols in top-level app entry surfaces.
```

Score impact:

| Capability | Before | After | Change |
| --- | --- | --- | --- |
| Product sovereignty | forbidden route scan covered major renderer entry paths | quarantine scan also covers dormant workflow symbols across preload/tray/settings shortcut surfaces | evidence improves |
| Kun baseline fidelity | no top-level Workflow/Create Loop route | explicit test proves dormant code stays unmounted | evidence improves |
| Release readiness | blocked | unchanged | no score change |

Rejected score claims:

```text
No removal of dormant workflow files, packaged desktop QA, external marketplace
copy audit, default Go backend, or release readiness is claimed by this scan.
```

## 2026-06-22 - Go G5 Parallel Task Dependency Executable Shadow Score Rule

Scope:

```text
Go G5 shadow now executable-replays `parallel_tasks` dependency validation
from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for dependency validation. | No live Go Job Manager, rollback, Electron integration, G6 readiness, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow now agree on valid order plus five invalid dependency errors. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Job orchestration value is absorbed without Reasonix protocol, Kun/deprecated identity, or top-level entry expansion. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 executable shadow covers `parallel_tasks` dependency validation
from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
G6 readiness, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Write Sidebar and Connect Phone Placeholder Sovereignty Score Rule

Scope:

```text
Product-sovereignty scan coverage now includes Write/sidebar entry surfaces,
and generated Connect Phone placeholders use Connect Phone copy.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score stays capped, but scan coverage now includes Write sidebar, mode tabs, project section, and stronger forbidden-token variants. | No packaged desktop walkthrough. |
| Connect Phone QA | 3 | 3 | New mapped-conversation placeholders emit `[Connect Phone:...]`. | Internal compatibility names and legacy recognizers remain by design. |
| Reasonix/Kun absorption safety | 5 | 5 | No Reasonix protocol, Kun identity, deprecated bridge/settings fallback, default Go backend, or hidden top-level entry is added. | Full release gate remains separate. |

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

## 2026-06-22 - HTTP Auto-Plan Payload Boundary Score Rule

Scope:

```text
HTTP start-turn payloads containing Reasonix-shaped `autoPlan` / `auto_plan`
fields remain agent turns and do not advertise `create_plan`.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Plan mode contract safety | 4 | 4 | Score stays capped, but negative HTTP evidence proves only explicit analytix `mode: "plan"` / `guiPlan` triggers Plan mode. | No packaged Plan mode walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix public auto-plan shape is rejected without adding Kun-absent top-level entries. | Full product auto-plan parity remains unclaimed. |
| Go G5 readiness | 3 | 3 | No Go runtime code changed. | No live Go controller, rollback, or default backend. |

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

## 2026-06-22 - Product Sovereignty Scan Engineering Score Rule

Scope:

```text
Forbidden-surface release scans are now available as a reusable
`scan:product-sovereignty` engineering gate, and Plugin Marketplace is included
in top-level route-surface coverage.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped, but the gate now checks top-level route entries, upstream identity/protocol leakage, deprecated bridge/settings fallback, Go/Rust/Tauri enabling, Rust/Tauri files, and Connect Phone locale copy. | No packaged desktop walkthrough or signing/notarization proof. |
| Kun route-surface safety | 5 | 5 | Plugin Marketplace is now part of route-surface forbidden-token coverage. | Scan coverage is not behavior coverage. |
| Reasonix/Kun absorption safety | 5 | 5 | The reusable gate makes each future absorption batch prove it did not expose Reasonix protocol, Kun identity, deprecated bridges/settings, or forbidden top-level entries. | Full release readiness remains separately gated. |

Allowed wording:

```text
Analytix has a reusable product-sovereignty scan gate for upstream absorption
batches.
```

Forbidden wording:

```text
Do not claim release readiness, packaged desktop QA, default Go backend,
Reasonix public protocol parity, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer support, Kun identity, or Rust/Tauri migration from
this scan alone.
```

## 2026-06-22 - Reasonix Auto-Plan Config Boundary Score Rule

Scope:

```text
Settings/config tests now reject Reasonix auto-plan public config shapes.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score stays capped, but evidence improves because upstream `agent.auto_plan`, `autoPlan`, and `auto_plan` shapes are stripped or rejected. | No packaged settings QA. |
| Runtime contract reliability | 4 | 4 | Runtime settings keys and `analytix serve` config reject upstream auto-plan roots. | No product auto-plan toggle. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix boundary value is absorbed without Reasonix protocol, Kun identity, new top-level route, or Go backend changes. | Full planner/auto-plan product parity remains unclaimed. |

Allowed wording:

```text
Analytix rejects Reasonix auto-plan public config shapes while preserving
analytix-owned runtime settings.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan setting parity, project/local auto-plan
overrides, Reasonix controller parity, packaged settings QA, release readiness,
Kun identity, default Go backend, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Task Parent Goal Evidence Executable Score Rule

Scope:

```text
G5 control executable output now replays task parent-goal evidence metadata.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Sub-agent/job orchestration | 4 | 4 | Score remains capped, but executable output now proves active-goal ledger success and missing-goal error behavior. | No packaged desktop sub-agent/task-job QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes parent-goal fields from the TS task-job oracle. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Evidence is absorbed without Reasonix protocol, Kun identity, new top-level route, or Go backend activation. | Full live parity remains unclaimed. |

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
migration.
```

## 2026-06-22 - Go G5 Approval/User-Input Route Body Replay Score Rule

Scope:

```text
Go G5 shadow now replays approval/user-input deny, submit, and cancel route
bodies plus structured prompt option labels from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input reliability | 4 | 4 | Score remains capped, but G5 replay now includes exact HTTP bodies, pending counts, prompt options, and SSE answer redaction. | No packaged desktop approval-card walkthrough. |
| Go G5 readiness | 3 | 3 | Shadow quality improves because Go computes route-body and prompt summaries from the TS route oracle. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Gate behavior is absorbed without Reasonix ask/session protocol, Kun/deprecated identity, or hidden top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow replays approval/user-input route bodies and structured
prompt shape from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval/user-input manager readiness, default Go backend
readiness, Reasonix SessionAPI/ask protocol parity, packaged desktop
approval-card QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Lifecycle Detail Replay Score Rule

Scope:

```text
Go G5 shadow now replays MCP background reconnect, known override diagnostics,
and search workspace-boundary details from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle | 4 | 4 | Score remains capped, but G5 replay now includes reconnect retry/error details, known override diagnostics, and search trust boundaries. | No credentialed MCP server matrix. |
| Go G5 readiness | 3 | 3 | Shadow quality improves because Go computes MCP detail summaries from the TS lifecycle oracle. | No live Go MCP client/indexer, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP value is absorbed without Reasonix public lifecycle protocol, MCP-indexer navigation, Kun identity, or Rust/Tauri path. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow replays MCP reconnect, known-override, and search
workspace-boundary details from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client/indexer readiness, default Go backend
readiness, Reasonix MCP-indexer lifecycle protocol parity, top-level
MCP-indexer navigation, credentialed MCP matrix, packaged desktop MCP QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Provider Usage Parser Precedence Score Rule

Scope:

```text
Go G5 shadow now replays provider usage parser precedence from TS-owned
provider-cache fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but G5 replay now includes unsupported absence, DeepSeek native precedence, OpenAI responses cached tokens, and Anthropic cache fields. | No credentialed provider matrix or packaged settings QA. |
| Go G5 readiness | 3 | 3 | Shadow quality improves because Go computes parser-precedence summaries from raw fixture response bodies. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache value is absorbed without Reasonix provider protocol, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow replays provider usage parser precedence from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, live Go provider client,
default Go backend readiness, packaged provider settings QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Auto-Router Classifier Currentness Score Rule

Scope:

```text
Go G5 shadow now replays auto-router classifier currentness, request isolation,
contract drift invalidation, and timeout fallback from TS-owned runtime
constants.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Auto-plan/currentness proof | 4 | 4 | Score remains capped, but G5 replay now binds classifier fingerprint, isolated request shape, drift invalidation, and fallback behavior. | No public product auto-plan setting or packaged planner QA. |
| Go G5 readiness | 3 | 3 | Shadow quality improves because Go computes classifier-currentness output from TS-owned fixtures. | No live Go auto-router, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix post-881 value is absorbed without Reasonix controller protocol, Kun identity, deprecated bridge/settings fallback, or Rust/Tauri path. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow replays auto-router classifier currentness and timeout
fallback from TS-owned runtime constants.
```

Forbidden wording:

```text
Do not claim Reasonix auto-plan setting parity, project/local auto-plan
override support, Reasonix controller/SessionAPI protocol parity, live Go
auto-router readiness, default Go backend readiness, packaged planner QA,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - G5 Task Parent Goal Evidence Score Rule

Scope:

```text
Go G5 shadow now replays task/sub-agent parent-goal evidence requirements from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow output now carries parent-goal evidence keys. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow now agree on active-goal requirement and evidence ledger/error event keys. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Job evidence is absorbed without Reasonix protocol, top-level Subagent route, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay task parent-goal evidence requirements from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - G5 Planner-Executor Detail Score Rule

Scope:

```text
Go G5 shadow now replays planner-executor skipped/cancel reasons and
output/transcript job counts from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow output now carries planner-executor detail fields. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Planner/sub-agent orchestration | 4 | 4 | TS oracle and Go shadow now agree on dependency failure/cancel reasons and output/transcript job counts. | No packaged desktop planner/sub-agent QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Planner-executor value is absorbed without Reasonix protocol, top-level Subagent/Workflow route, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay planner-executor detail evidence from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix planner/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - G5 Task Tool Contract Boundary Score Rule

Scope:

```text
Go G5 shadow now replays internal `task` and `parallel_tasks` tool-contract
boundaries from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow output now carries task/parallel tool-contract boundaries. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/sub-agent orchestration | 4 | 4 | TS oracle and Go shadow now agree on internal-runtime-only, permission, dependency, and planner read-only gates. | No packaged desktop task/sub-agent QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Task orchestration value is absorbed without Reasonix protocol, top-level Subagent/Workflow route, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay internal task tool-contract boundaries from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix task/sub-agent protocol parity, top-level Subagent navigation, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Provider Live-Local HTTP Proof Score Rule

Scope:

```text
Provider usage and request-shape oracle cases now execute through a
no-credential local HTTP provider while preserving original provider request
construction.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but all usage cases now execute through local HTTP and validate cache/usage mapping. | No credentialed provider matrix or external cache hit proof. |
| Provider request safety | 4 | 4 | Request-shape cases validate path/header/body/tool shape over real HTTP POST. | No packaged provider settings QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Proof adds no Reasonix protocol, Kun identity, deprecated bridge/settings fallback, Go backend, or hidden product entry. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix provider/cache proof executes all usage and request-shape oracle
cases through no-credential local HTTP.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, provider behavior changes, Reasonix provider protocol parity,
default Go backend, packaged provider settings QA, release readiness, Kun
identity, or Rust/Tauri migration.
```

## 2026-06-22 - G5 Provider Live-Local Proof Summary Score Rule

Scope:

```text
G5 shadow now replays fixture-owned metadata for the provider live-local HTTP
proof, deriving counts and endpoint formats from the TS provider-cache oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but live-local proof scope is now explicit oracle metadata. | No credentialed provider matrix or external cache hit proof. |
| Go G5 readiness | 3 | 3 | Go shadow computes `cacheReplay.liveLocalHttpProof` from TS-owned fixtures. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Summary adds no Reasonix protocol, Kun identity, deprecated bridge/settings fallback, Go backend, or hidden product entry. | Full live provider parity remains unclaimed. |

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

## 2026-06-22 - G5 MCP Approval Annotation Score Rule

Scope:

```text
G5 shadow now replays destructive/open-world MCP approval metadata and denied
no-execute evidence from the TS MCP lifecycle oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Approval/user-input reliability | 4 | 4 | Score remains capped, but high-risk MCP annotation denial is now replayed in G5. | No packaged approval-card walkthrough. |
| MCP lifecycle | 4 | 4 | G5 `mcpReplay.approvalAnnotations` carries normalized tool, approval id, denial, and no-execute evidence. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes the annotation summary without becoming a live approval manager. | No live Go MCP client, Electron integration, rollback, or default backend. |

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

## 2026-06-22 - G5 MCP Live-Local Indexer Summary Score Rule

Scope:

```text
G5 shadow now replays MCP live-local indexer lifecycle evidence, including
retry, tombstone, restart, active path, and secret-safe diagnostic fields.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle | 4 | 4 | Score remains capped, but `mcpReplay.liveLocalIndexer` now carries richer lifecycle proof. | No credentialed MCP matrix or packaged MCP QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes the nested MCP summary from TS-owned fixtures. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Summary adds no Reasonix protocol, Kun identity, deprecated bridge/settings fallback, Go backend, or hidden product entry. | Full live MCP parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Provider Streaming Usage Score Rule

Scope:

```text
G5 full-loop shadow now replays provider streaming usage evidence from the G3
provider streaming/usage/cache oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but G5 now carries streaming event order plus parsed DeepSeek usage/cache telemetry. | No credentialed provider matrix or packaged provider settings QA. |
| Go G5 readiness | 3 | 3 | Go shadow parses fixture SSE usage data and compares it to the TS-owned provider usage case. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache value is absorbed without Reasonix protocol, Kun identity, provider behavior change, or hidden top-level entry. | Full live provider parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Task-Job Route Executable Score Rule

Scope:

```text
G5 full-loop shadow now replays internal task-job route executable evidence
from the TS task-job orchestration oracle.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Task/job orchestration | 4 | 4 | Score remains capped, but route executable proof now covers unauthorized/output/wait/kill/missing/rehydrated cases. | No packaged desktop sub-agent/task-job QA. |
| Go G5 readiness | 3 | 3 | Go shadow emits `jobReplay.routeExecutable` from the TS-owned oracle. | No live Go Job Manager, route server, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Task/job value is absorbed without Reasonix SessionAPI, public job protocol, Kun identity, or hidden top-level entry. | Full live task/job parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Task Job Lifecycle Replay Score Rule

Scope:

```text
Go G5 shadow now replays base task-job lifecycle fields for foreground,
background, and wait/output/kill behavior from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow replay quality improves for base task-job lifecycle. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow now agree on foreground/background/wait-output-kill summary fields. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Job lifecycle value is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay base task-job lifecycle from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job manager readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Task Job Route Boundary Replay Score Rule

Scope:

```text
Go G5 shadow now replays task-job route auth and forbidden top-level route
boundaries from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow replay quality improves for task-job route auth. | No live Go task-job routes, Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow now agree on protected route ids, unauthorized `401`, and forbidden top-level routes. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Route boundary value is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay task-job route auth and forbidden top-level
route boundaries from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go task-job route readiness, default Go backend readiness,
Reasonix SessionAPI/sub-agent protocol parity, top-level Subagent navigation,
release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Replay Score Rule

Scope:

```text
Go G5 shadow now replays MCP connect/disconnect/reload/cancel/error lifecycle
fields from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow replay quality improves for MCP core lifecycle. | No live Go MCP client, Electron integration, rollback, or default backend. |
| MCP lifecycle | 4 | 4 | TS oracle and Go shadow now agree on connect/disconnect/reload/cancel/error summary fields. | No credentialed MCP matrix or packaged desktop MCP QA. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP lifecycle value is absorbed without Reasonix protocol, MCP-indexer route, or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay MCP core lifecycle from TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, default Go backend readiness,
Reasonix MCP-indexer protocol parity, top-level MCP-indexer navigation, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Approval Decision Route Replay Score Rule

Scope:

```text
Go G5 shadow now replays approval decision route and event replay ordering from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow replay quality improves for approval decision routes. | No live Go approval manager, Electron integration, rollback, or default backend. |
| Approval/user-input reliability | 4 | 4 | TS oracle and Go shadow now agree on denied approval, duplicate `409`, and replay kind order. | No packaged desktop approval-card QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Approval route value is absorbed without Reasonix ask/session protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay approval decision route evidence from TS-owned
fixtures.
```

Forbidden wording:

```text
Do not claim live Go approval manager readiness, default Go backend readiness,
Reasonix SessionAPI/ask protocol parity, release readiness, Kun identity, or
Rust/Tauri migration.
```

## 2026-06-22 - Offline Provider/Cache Parity Seal Score Rule

Scope:

```text
Go G5 shadow now replays an offline provider/cache parity seal from TS-owned
provider-cache fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider/cache proof | 4 | 4 | Score remains capped, but evidence is clearer for DeepSeek stable prefix/cache and multi-provider request/usage regression coverage. | No credentialed provider matrix or live superiority claim. |
| Go G5 readiness | 3 | 3 | G5 shadow computes the same seal from the TS provider-cache oracle. | No live Go provider client or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Provider/cache value is absorbed without Reasonix provider protocol or Kun/deprecated identity. | Full live provider parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can replay an offline provider/cache parity seal from
TS-owned fixtures.
```

Forbidden wording:

```text
Do not claim live provider/cache superiority, credentialed provider matrix
readiness, Reasonix provider protocol parity, default Go backend, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Abort Cleanup Score Rule

Scope:

```text
Go G5 shadow now executable-replays approval/user-input abort cleanup state,
late GUI action statuses, and replay event order from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for approval/user-input abort cleanup. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Approval/user-input reliability | 4 | 4 | TS oracle and Go shadow now agree on expired approval, cancelled user-input, late `409`/`404`, no pending gates, and replay kinds. | No packaged desktop approval-card QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Abort-cleanup lifecycle value is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Cache Drift Attribution Score Rule

Go G5 shadow now replays provider-cache drift attribution from TS-owned
fixtures. This improves provider/cache diagnostic proof and future Go gate
quality while keeping live provider and backend scores capped.

Benchmark effect:

| Dimension | Before | After | Reason | Remaining gap |
| --- | --- | --- | --- | --- |
| Provider/cache diagnostics | 4 | 4 | Evidence now distinguishes allowed tool/provider/model drift from forbidden stable-prefix pollution. | Live credentialed provider drift matrix remains open. |
| Go G5 readiness | 3 | 3 | Go shadow computes drift attribution fields from the TS provider-cache oracle. | No live Go provider client or default backend. |
| Product sovereignty | 5 | 5 | No Reasonix protocol, Go route, Kun identity, dynamic stable-prefix material, or hidden product entry was added. | Packaged provider settings QA remains separate. |

Allowed claim:

```text
Analytix G5 shadow replay attributes provider-cache drift to allowed
tool/provider/model changes while preserving stable prefix hygiene.
```

Still not claimable:

```text
Live provider/cache superiority, Reasonix provider protocol parity, live Go
provider client readiness, default Go backend, packaged settings QA, release
readiness, Kun identity, dynamic-context stable prefix, or Rust/Tauri migration.
```

## 2026-06-22 - Go G3 Cache Drift Attribution Score Rule

Go G3 provider conformance now computes provider-cache drift attribution from
TS-owned raw shapes. This gives the lower provider gate the same cache drift
explanation that G5 later summarizes.

Benchmark effect:

| Dimension | Before | After | Reason | Remaining gap |
| --- | --- | --- | --- | --- |
| Provider/cache diagnostics | 4 | 4 | G3 now ties raw drift shapes and compact attribution output to `ProviderCacheOracle`. | Live credentialed provider drift matrix remains open. |
| Go G3 readiness | 3 | 3 | Go shadow computes cache drift attribution in provider conformance output. | No live Go provider client or default backend. |
| Runtime contract stability | 5 | 5 | Provider URL/body behavior, usage parsing, and stream parsing are unchanged. | Packaged provider settings QA remains separate. |
| Product sovereignty | 5 | 5 | No Reasonix protocol, Go route, Kun identity, dynamic stable-prefix material, or hidden product entry was added. | Release readiness remains unproven. |

Allowed claim:

```text
Analytix G3 provider conformance computes cache drift attribution from
TS-owned raw shapes while preserving stable prefix hygiene.
```

Still not claimable:

```text
Live provider/cache superiority, provider request behavior changes, Reasonix
provider protocol parity, live Go provider client readiness, default Go backend,
packaged settings QA, release readiness, Kun identity, dynamic-context stable
prefix, or Rust/Tauri migration.
```

## 2026-06-22 - Go G3 Provider Request Shape Score Rule

Go G3 provider conformance now replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures. This strengthens the provider
gate for future Go work while keeping live provider/runtime scores capped.

Benchmark effect:

| Dimension | Before | After | Reason | Remaining gap |
| --- | --- | --- | --- | --- |
| Provider request safety | 4 | 4 | G3 now ties request-shape matrix and compact request-surface summary to `ProviderCacheOracle`. | Live credentialed provider endpoint matrix remains open. |
| OpenAI/Anthropic/custom non-regression | 4 | 4 | Summary covers endpoint families, full endpoint cases, tool-shape families, and body-field counts. | Packaged provider settings QA remains separate. |
| Go G3 readiness | 3 | 3 | Go shadow computes request-shape summary in provider conformance output. | No live Go provider client or default backend. |
| Product sovereignty | 5 | 5 | No Reasonix protocol, Go route, Kun identity, provider behavior change, or hidden product entry was added. | Release readiness remains unproven. |

Allowed claim:

```text
Analytix G3 provider conformance replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Still not claimable:

```text
Live provider/cache superiority, provider request behavior changes, Reasonix
provider protocol parity, live Go provider client readiness, default Go backend,
packaged settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Session Route Status Score Rule

Go G5 full-loop shadow now replays thread/session route status and SSE replay
semantics from TS-owned G2 HTTP/SSE fixtures. This strengthens runtime contract
evidence without changing backend readiness scores.

Benchmark effect:

| Dimension | Before | After | Reason | Remaining gap |
| --- | --- | --- | --- | --- |
| Thread/session runtime contract | 4 | 4 | G5 now carries archive/search/read/update/fork/resume/SSE route status evidence. | Packaged desktop restart/resume QA remains open. |
| Go G5 readiness | 3 | 3 | Go shadow computes route status replay from G2 route responses and SSE frames. | No live Go route server or default backend. |
| Product sovereignty | 5 | 5 | No Reasonix SessionAPI, renderer-visible Go route, Kun identity, or hidden product entry was added. | Release readiness remains unproven. |

Allowed claim:

```text
Analytix G5 full-loop shadow replays thread/session route status and SSE
semantics from TS-owned G2 HTTP/SSE fixtures.
```

Still not claimable:

```text
Live Go route readiness, default Go backend, Reasonix SessionAPI protocol
parity, packaged desktop restart/resume QA, release readiness, Kun identity,
deprecated bridge/settings fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 Provider Request Shape Score Rule

Go G5 full-loop shadow now replays provider request URL/body/tool-shape coverage
from TS-owned request-shape fixtures. This makes the full-loop summary carry the
same provider request-surface evidence as the lower G3 provider gate.

Benchmark effect:

| Dimension | Before | After | Reason | Remaining gap |
| --- | --- | --- | --- | --- |
| Provider request safety | 4 | 4 | G5 now carries compact request-surface summary tied to `ProviderCacheOracle`. | Live credentialed provider endpoint matrix remains open. |
| OpenAI/Anthropic/custom non-regression | 4 | 4 | Summary covers endpoint families, full endpoint cases, tool-shape families, and body-field counts. | Packaged provider settings QA remains separate. |
| Go G5 readiness | 3 | 3 | Go shadow computes request-shape replay in full-loop shadow output. | No live Go provider client or default backend. |
| Product sovereignty | 5 | 5 | No Reasonix protocol, Go route, Kun identity, provider behavior change, or hidden product entry was added. | Release readiness remains unproven. |

Allowed claim:

```text
Analytix G5 full-loop shadow replays provider request URL/body/tool-shape
coverage from TS-owned request-shape fixtures.
```

Still not claimable:

```text
Live provider/cache superiority, provider request behavior changes, Reasonix
provider protocol parity, live Go provider client readiness, default Go backend,
packaged settings QA, release readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Go G5 MCP Search Meta-Tool Replay Score Rule

Scope:

```text
G5 MCP replay now carries MCP search meta-tool trust and denied no-execute
evidence from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| MCP lifecycle | 4 | 4 | Score remains capped, but G5 replay now includes search meta-tool trust/no-execute fields. | No credentialed MCP server matrix or packaged MCP QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes mcpReplay search fields from the TS MCP lifecycle oracle. | No live Go MCP client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP search value is absorbed without Reasonix MCP-indexer protocol or Kun/deprecated identity. | Full live MCP parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Parallel Task Dependency Validation Score Rule

Scope:

```text
G5 job replay now carries `parallel_tasks` dependency validation evidence from
TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Sub-agent/job orchestration | 4 | 4 | Score remains capped, but G5 replay now includes valid order and five invalid dependency cases. | No packaged desktop sub-agent/task-job QA. |
| Go G5 readiness | 3 | 3 | Go shadow computes parallel validation fields from the TS task-job oracle. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Dependency validation is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

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

## 2026-06-22 - Go G5 Abort Cleanup Replay Summary Score Rule

Scope:

```text
G5 control replay, approval/user-input replay, and executable shadow all cover
the same abort-cleanup semantics from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but shadow summary and executable output now agree on abort cleanup. | No live Go approval/user-input manager, Electron integration, rollback, or default backend. |
| Approval/user-input reliability | 4 | 4 | Replay-summary fields are derived from the TS approval/user-input route oracle. | No packaged desktop approval-card QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Summary closure adds no Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

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

## 2026-06-22 - Auto-Router Fingerprint Currentness Score Rule

Scope:

```text
Auto-router fingerprint proof now covers classifier request-contract drift.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Auto-router currentness | 4 | 4 | Score remains capped, but evidence deepens because request-contract drift changes route-cache fingerprints. | No user-visible auto-plan setting or packaged controller QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Reasonix stale-classifier value is absorbed without Reasonix config/protocol or Kun/deprecated identity. | Full product auto-plan parity remains unclaimed. |
| Go G5 readiness | 3 | 3 | No Go runtime code changed; TS oracle quality improves for future Go parity. | No live Go router, Electron integration, rollback, or default backend. |

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

## 2026-06-22 - Connect Phone Copy Sovereignty Score Rule

Scope:

```text
New Connect Phone prompt/title/schema/log copy now uses Connect Phone while
legacy Claw recognizers remain compatibility-only.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score stays capped, but evidence improves because new Connect Phone copy no longer emits `Claw IM`. | Internal compatibility names remain by design. |
| Connect Phone QA | 3 | 3 | Focused prompt/runtime/renderer tests pass. | No packaged Connect Phone walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Copy correction adds no Kun/Reasonix identity, route, bridge, settings fallback, or Go backend. | Full packaged QA remains unclaimed. |

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

## 2026-06-22 - Go G5 Durable Runner Restart Score Rule

Scope:

```text
Go G5 shadow now executable-replays durable task-job restart output, offset,
and queued kill behavior from TS-owned fixtures.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for durable runner restart. | No live Go Job Manager, Electron integration, rollback, or default backend. |
| Task/job orchestration | 4 | 4 | TS oracle and Go shadow now agree on rehydrated output/nextOffset/kill behavior. | No packaged desktop sub-agent/task-job QA. |
| Reasonix/Kun absorption safety | 5 | 5 | Job orchestration value is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

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

## 2026-06-22 - Tool Result File/Image Boundary Score Rule

Scope:

```text
G5 shadow now computes tool-result file/image preservation from TS-owned
tool-result image, attachment-store, and renderer mapper proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for file/image result boundaries. | No live Go file/image bridge, Electron integration, rollback, or default backend. |
| Generated-file reliability | 4 | 4 | Source-derived proof keeps `localFilePath`, fallback `FilePath`, tool attachments, and generated files visible. | No packaged desktop generated-file/attachment walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | File/image result value is absorbed without Reasonix protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay tool result file/image preservation
from TS-owned fixtures while keeping old base64 payloads out of model-visible
context.
```

Forbidden wording:

```text
Do not claim live Go file/image bridge readiness, Reasonix file protocol
parity, default Go backend readiness, packaged attachment QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Event JSONL Replay Boundary Score Rule

Scope:

```text
G5 shadow now computes event JSONL replay and malformed recovery from TS-owned
file session and runtime recorder proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for event replay/recovery boundaries. | No live Go event store, Electron integration, rollback, or default backend. |
| Replay reliability | 4 | 4 | Source-derived proof keeps append/replay/highestSeq/malformed recovery and usage compaction failure behavior visible. | No packaged desktop long-thread replay walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | Event replay value is absorbed without Reasonix event protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay event JSONL append/replay/recovery
from TS-owned fixtures while keeping event persistence analytix-owned.
```

Forbidden wording:

```text
Do not claim live Go event-store readiness, Reasonix event protocol parity,
default Go backend readiness, packaged long-thread replay QA, release
readiness, Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - MCP Malformed Schema Boundary Score Rule

Scope:

```text
G5 shadow now computes malformed MCP schema normalization from TS-owned MCP
provider proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for MCP schema normalization boundaries. | No live Go MCP client, Electron integration, rollback, or default backend. |
| MCP catalog safety | 4 | 4 | Source-derived proof keeps non-object schema defaulting, invalid property dropping, required filtering, and output-schema omission visible. | No credentialed MCP matrix or packaged MCP walkthrough. |
| Reasonix/Kun absorption safety | 5 | 5 | MCP schema value is absorbed without Reasonix MCP-indexer protocol or Kun/deprecated identity. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay malformed MCP schema normalization
from TS-owned fixtures before tool catalog exposure.
```

Forbidden wording:

```text
Do not claim live Go MCP client readiness, Reasonix MCP-indexer protocol
parity, default Go backend readiness, credentialed MCP QA, release readiness,
Kun identity, or Rust/Tauri migration.
```

## 2026-06-22 - Renderer Route-Surface Sovereignty Score Rule

Scope:

```text
G5 shadow now computes renderer top-level route sovereignty from TS-owned
Workbench route-surface, browser bridge, plugin marketplace, shell navigation,
Workbench shell, and base shell CSS proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves forbidden hidden-capability entries remain absent. | No packaged desktop route walkthrough or release readiness. |
| Go G5 readiness | 3 | 3 | Score remains capped, but executable shadow quality improves for renderer route-surface boundaries. | No live Go renderer route, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Renderer route value is absorbed without Reasonix protocol, Kun/deprecated bridge alias, or Kun-absent top-level entries. | Full live parity remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay renderer route-surface sovereignty and
prove Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries remain out
of the top-level product surface.
```

Forbidden wording:

```text
Do not claim Reasonix UI/session protocol parity, top-level Workflow/Create
Loop/Subagent/AutoResearch/MCP-indexer navigation, default Go backend
readiness, packaged desktop route QA, release readiness, Kun identity,
deprecated settings fallback, or Rust/Tauri migration.
```

## 2026-06-22 - Package Runtime CLI Identity Score Rule

Scope:

```text
G5 shadow now computes package/runtime CLI identity from TS-owned package,
builder, app identity, runtime CLI, afterPack, packaging test, and release
workflow proof.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Product sovereignty | 5 | 5 | Score remains capped at the maximum, but executable shadow now proves package/release/CLI identity is analytix-owned. | No packaged artifact walkthrough or release readiness. |
| Runtime contract safety | 4 | 4 | Source-derived proof keeps `analytix serve`, `ANALYTIX_READY`, and bundled serve-entry validation visible. | No live Go launcher or backend switch. |
| Reasonix/Kun absorption safety | 5 | 5 | Runtime identity value is absorbed without Reasonix CLI protocol, Kun/DeepSeek identity, or default Go backend. | Full release QA remains unclaimed. |

Allowed wording:

```text
Analytix G5 shadow can executable-replay package/runtime CLI identity and prove
the public runtime command remains `analytix serve`.
```

Forbidden wording:

```text
Do not claim Reasonix CLI/session protocol parity, packaged artifact QA,
release readiness, default Go backend readiness, Kun/DeepSeek product identity,
or Rust/Tauri migration.
```

## 2026-06-22 - Custom Chat Full Endpoint Score Rule

Scope:

```text
Provider-cache oracle now covers custom full `/chat/completions` endpoint
exact URL and OpenAI-compatible chat request shape.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Provider request safety | 4 | 4 | Score remains capped, but fake-fetch evidence now covers custom full `/chat/completions` exact URL plus chat body/header/tool shape. | No credentialed provider matrix or packaged settings QA. |
| Provider/cache proof | 4 | 4 | Oracle now covers custom `/responses`, `/messages`, and `/chat/completions` endpoint families. | No live provider/cache superiority claim. |
| Go G3/G5 readiness | 3 | 3 | G3/G5 shadow replays the new request-shape id only. | No live Go provider client, Electron integration, rollback, or default backend. |
| Reasonix/Kun absorption safety | 5 | 5 | Request-shape behavior is absorbed without Reasonix provider protocol or Kun/deprecated identity. | Full live provider parity remains unclaimed. |

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

## 2026-06-25 - Go Runtime Default Delivery And Contract Equivalence

Scope:

```text
Go runtime default delivery is active through go-runtime-default.
ANALYTIX_RUNTIME_BACKEND=typescript is a retired backend rejected by the
desktop adapter. The
2026-06-25 contract-equivalence closure fixed Go-default turn body, attachment,
memory, approval/user-input, diagnostics, and SSE replay gaps without adding a
renderer-visible Go route, Reasonix public protocol, or new product entry.
```

Scores:

| Surface | Before | After | Evidence | Remaining gap |
| --- | ---: | ---: | --- | --- |
| Runtime contract reliability | 3 | 4 | `docs/analytix/upstreams/final-go-runtime-delivery-report.md`, `src/main/runtime/analytix-adapter.test.ts`, and `packages/runtime-go` tests prove Go default startup plus Analytix turn fields, attachments, memory, approval/user-input, diagnostics, and SSE replay contract equivalence. | Full packaged desktop route/restart walkthrough and post-cutover live evidence remain separate release-strength work. |
| Provider/cache boundary | 4 | 4 | Deterministic provider non-regression covers OpenAI chat completions, OpenAI responses, Anthropic messages, custom full endpoints, request-body separation, usage parsing, and DeepSeek telemetry isolation. DeepSeek live probe is scoped to DeepSeek only. | No credentialed OpenAI-compatible, Anthropic-compatible, custom endpoint, MCP, packaged, or operator superiority claim is made. |
| Safety/privacy | 4 | 4 | Go default preserves bearer-token auth when configured, supports explicit insecure local mode for empty runtime tokens, records no credential values, keeps product-sovereignty scan green, and adds no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry. | Credentialed MCP execution, packaged approval/user-input QA, and operator-reviewed retirement evidence are still pending. |
| Maintainability | 4 | 4 | The final delivery report, upstream conformance, absorption ledger, retirement checklist, and adapter tests now describe one default Go path and retired TypeScript backend evidence. | Remaining release work is live/provider/MCP/operator evidence, not a TypeScript startup path. |

Allowed wording:

```text
Analytix now starts the Go runtime by default and rejects the retired TypeScript
backend while preserving the current Analytix HTTP/SSE,
provider-boundary, safety, privacy, and product-surface contracts.
```

Forbidden wording:

```text
Do not claim credentialed OpenAI/Anthropic/custom live provider superiority,
live MCP superiority, packaged-app superiority, operator-approved fallback
retirement, Reasonix public protocol parity, top-level Workflow/Create Loop/
Subagent/AutoResearch/MCP-indexer product entry, or TypeScript physical
retirement from this entry.
```
