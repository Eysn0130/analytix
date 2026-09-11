# Kun upstream sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"kun","reviewedCommit":"f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7","parity":"not-proven","capabilityBenchmarkV1":null} -->

Kun is treated as a product-workflow upstream for analytix. Its changes may
inform Code, Write, SDD, Connect Phone, schedule, GUI workflows, command
behavior, provider presets, and desktop bug fixes.

Kun does not decide analytix UI, product identity, settings schema, bridge,
runtime contract, release identity, or QA gates.

This is a default research lens, not an exclusive boundary. Kun is the
regression floor for inherited product workflows, but future analytix product
capabilities may also learn from Reasonix, OpenCode, CodexDesktop-Rebuild,
Hermes Agent, or other sources when a capability-first comparison proves a
better design.

Current status note (2026-07-20): Go runtime default delivery is active through
`go-runtime-default`; `ANALYTIX_RUNTIME_BACKEND=typescript` is retired and
rejected by the current desktop adapter. Older rows that describe TypeScript
as the default runtime, explicit rollback, or Go as conformance-only are
historical stage records and are superseded for current startup behavior by
the final Go runtime delivery report and current specs. A new audit at
`f65ac05` is now the current comparison input; parity remains unproven.

## Baseline

| Field | Value |
| --- | --- |
| analytix origin | Started from former Kun `0.2.13` codebase. |
| current reviewed input | Kun `master` at `f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7`. Older `777e343`, `8602476`, `9fb5ecf`, `21dbeaaf`, and `7c1bc06` rows are historical implementation evidence. |
| default landing rule | Absorb capability through analytix UI, contracts, and tests. |

## 2026-07-20 - Current white-box tool-authority audit

| Field | Value |
| --- | --- |
| Branch / commit | `master` / `f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7` |
| Audit scope | Turn-frozen tool catalogs, execution-time resolution, extension schema validation, MCP capability bundles, approvals, outcome recovery, and reasoning surfaces. |
| Test posture | Current implementation and relevant tests were inspected at the pinned commit. This entry does not assert product parity or successful execution of the complete Kun suite. |

Current code evidence and decisions:

| Capability | Current implementation evidence | Decision and analytix requirement |
| --- | --- | --- |
| Turn-frozen catalog and drift | `kun/src/loop/turn-tool-catalog.ts` and `kun/src/loop/model-step-service.ts` freeze the model-visible catalog for a turn and defer/catalogue drift. | `adapt`; freeze the catalog inside immutable `TurnSecurityContext`, include full schema/provider/server fingerprints, and use the same set when issuing grants and executing calls. |
| Execution-time re-resolution | `kun/src/adapters/tool/local-tool-host.ts` and `capability-registry.ts` prepare and resolve the current registry/policy again before execution. | `adapt`; preserve re-resolution, but require an exact current `ExecutionGrant`. Current registry presence must never authorize a tool absent from the frozen advertised set. |
| Advertised/executed allowlist identity | `allowedToolNames` is optional in `tool-context-factory.ts`; execution resolves the live registry and policy, while the frozen model catalog is not itself a mandatory execution predicate on every path. A newly registered tool can therefore be resolvable when no explicit allowlist was supplied. | `reject` as sufficient authority. Analytix must prove `RejectUnadvertisedToolInAgentMode` for every agent mode, independent of plan state, aliases, registry mutation, or provider spelling. |
| Extension input validation | `kun/src/extensions/json-schema-validator.ts` uses Ajv 2020 with non-mutating options and extension tools validate declared input schemas. | `adapt`; all external schemas default to recursive `additionalProperties:false`, unknown schemas fail closed, and validation occurs both before grant issuance and immediately before execution. |
| MCP capability bundle | `kun/src/adapters/tool/mcp-tool-search.ts` provides bounded search/describe/call tools and typed source metadata. | `adapt` the small capability bundle and focused ownership. Strengthen with negotiated server/protocol identity, argument validation, semantic-status preservation, case-bound live health, and host-issued evidence receipts. |
| Unknown mid-flight outcome | `kun/src/loop/round-outcome-coordinator.ts` avoids blindly replaying an unknown execution outcome. | `adapt`; terminal uncertainty must become blocked/boundary-only and pass the same Final Evidence Gate. Recovery cannot raise evidence status. |
| Approval authority | Current approval context does not bind the complete context digest, case/epoch/snapshot, schema/argument/scope hash, server identity, and expiry required for a case-grade grant. | `reject` as case authority; revalidate the persisted grant and frozen turn context after approval, resume, and restart. |
| Reasoning surfaces | Kun still has renderer/history paths that represent live reasoning, even though some external streamer paths drop reasoning deltas. | `reject`; analytix requires zero reasoning bytes in every public or durable surface, including failure, recovery, history, compaction, report, and export paths. |

Kun's frozen catalog, current-policy re-resolution, Ajv validation, and MCP
search/describe/call bundle are useful donors. Analytix exceeds them only when
the benchmark proves that advertisement and execution share the exact same
host authority and that stale identity, schema, approval, epoch, or evidence
fails closed. No documentation row sets `parity` or `exceeded` by itself.

## 2026-07-18 - Second source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `master` / `7c1bc0635c170e716f283e0889ae8f85d14703c8` |
| Previous same-day pin | `21dbeaaf4061473c31637cc4b7c9198eb8a0e48a` |
| Delta | Eight commits; 27 files, 2,083 insertions, 417 deletions. |

The range adds file-identity tests, cross-platform media process-runner work,
packaged desktop/OCR/video smoke hardening, and macOS native-architecture
verification. These are release and process-authority comparison inputs, not
proof that Analytix already matches the refreshed Kun checkout. Direct reuse
still requires file-level provenance; desktop/package claims require execution
on the named host and architecture.

## 2026-07-18 - Earlier same-day current intake

| Field | Value |
| --- | --- |
| Branch / commit / tag | `master` / `21dbeaaf4061473c31637cc4b7c9198eb8a0e48a` / current tracking branch |
| License evidence | PolyForm Noncommercial 1.0.0, pinned license blob `def8c00355e33337e1177ffe749c85d776e1c254`. The current text adds a separately confirmed internal-enterprise-use request path; it does not convert the repository to a permissive or commercial-use license. |
| Admitted posture | Required notice and noncommercial terms apply; commercial use requires written authorization. Shared project lineage does not prove such authorization. |
| Previous reviewed pin | `9fb5ecf90430bca3aff2fad4fa563f7b69b3ee80` |
| Delta | 117 commits; 930 files, 187,403 insertions, 4,162 deletions. |

The large delta includes mid-turn guidance, delegation timeout changes,
runtime-thread migration work, release/packaging hardening, and extensive media
and UI additions. It remains under code/test/document review. In particular,
removing subagent time limits is not admissible without shared budgets,
cancellation, and parent-owned evidence closure; currentness is not parity.

Bounded delta decisions since the old `8602476` planning line:

| Capability | Decision |
| --- | --- |
| Design workflow, SVG/infinite canvas, AI Rail, ShapeOps, tokens, review, page prototypes, operation journal, and Design-to-Code binding | `new product spec required`; study the React/pure-type parts, but do not restore a hidden or legacy route as implied parity. |
| Claude/Codex subscription auth, PKCE, encrypted OAuth storage, and remote MCP OAuth | `should absorb` only through Analytix provider profiles, OS-backed secrets, Go runtime contracts, and a security review. |
| Trusted workspace, MCP package pin/validation, supply-chain audit, repo map | `should absorb` by extending current Hub/MCP controls, not by reintroducing Kun's TypeScript agent runtime. |
| Memory provenance/confidence/expiry and Markdown import/export | `should absorb` behind opt-in, auditable memory contracts. |
| Kun Agent SDK or runtime loop | `reject` for production; `packages/runtime-go` remains the only production agent core. |

## Review Checklist

```text
1. Which Kun release/tag/commit range was reviewed?
2. Which user workflows changed?
3. Which changes are bug fixes vs product features vs UI reshapes?
4. Does any change reintroduce current-product Kun naming?
5. Does any change bypass `window.analytix`, top-level `runtime`/`provider`, the public TypeScript contracts/launcher, or `packages/runtime-go` production ownership?
6. Does any change affect Write, SDD, Connect Phone, schedule, plugin marketplace, or settings?
7. Does any change affect thread history, file paths, localFilePath/FilePath, or attachments?
8. What tests and desktop QA cover the accepted changes?
```

## 2026-06-23 - D-0246 AutoResearch Go Runtime Kun Boundary

Source:

```text
D-0246 adds Go runtime project-local AutoResearch state for `/goal --research`
turns and an audit-only SSE event. It does not change the Kun-derived desktop
product surface.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Runtime contract | Use existing `/v1/threads/:id/turns` and `/v1/threads/:id/events`; `autoresearch_state_audit` is audit-only and ignored by UI projection. |
| Project files | Write only `.analytix/autoresearch/<threadId>/task_spec.md`, `progress.json`, `findings.jsonl`, `directions_tried.json`, and `iteration_log.jsonl` under the active workspace. |
| Forbidden files/protocols | Do not write `REASONIX.md` or `AGENTS.md`; do not add Reasonix SessionAPI/config roots, `/v1/autoresearch`, bridge aliases, or settings schema changes. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin/MCP, provider/model, approval, user-input, slash commands, and `/goal` remain the preserved baseline. |
| Go runtime direction | D-0246 clears the AutoResearch deferred row, but default Go backend still requires D-0243 provider/MCP/packaged QA/readiness evidence. |

Validation:

```text
packages/runtime-go/d0246_autoresearch_state_test.go
packages/runtime-go/runtime_server_test.go
packages/runtime/tests/contracts.test.ts
packages/runtime/tests/runtime-event-reducer.test.ts
packages/runtime/tests/go-runtime-conformance.test.ts
scripts/scan-product-sovereignty.cjs
```

## 2026-06-23 - D-0245 Full-Function Baseline Guard

Source:

```text
KunAgent/Kun v0.2.13: 2ba8decc2f56862e7f677fcf89bbc3d402ec3a23
KunAgent/Kun v0.2.14: 8f2040349fba47fcd8e8b94f50b131943af839b2
```

Kun/Analytix decision:

| Surface | Decision |
| --- | --- |
| Product baseline | Preserve Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin/MCP, provider/model, chat/thread/session, attachments/workspace, SSE projection, slash commands, `/goal`, packaging, release identity, and desktop shell behavior as the baseline. |
| Reasonix role | Treat Reasonix as an engine donor for this baseline guard. Reasonix may improve runtime kernel, cache, DeepSeek prefix, MCP lifecycle, sub-agent lineage, goal evidence, and audit quality inside analytix contracts; future capability comparisons may include all relevant upstreams. |
| Provider baseline | Preserve DeepSeek, OpenAI-compatible, Anthropic-compatible, and `custom_endpoint`; DeepSeek cache/prefix optimization is provider-specific and must not narrow other providers. |
| Top-level entries | No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry is added. Existing quarantined workflow code remains non-entry product code. |
| Bridge/settings | Keep `window.analytix`, top-level `runtime` settings, `ANALYTIX_*`, `stable`/`beta`, and `com.analytix.desktop`; no deprecated bridge alias or old settings write-back. |
| Go runtime direction | Go runtime may become the unique baseline only after full-function non-regression passes; no long-term dual-track claim is made by this slice. |

Validation:

```text
packages/runtime-go/internal/upstreamaudit/baseline_absorption.go
packages/runtime-go/d0245_baseline_absorption_test.go
packages/runtime-go/runtime_server_test.go
packages/runtime/tests/go-runtime-conformance.test.ts
packages/runtime/tests/runtime-event-reducer.test.ts
scripts/scan-product-sovereignty.cjs
```

## 2026-06-23 - D-0243 G6 Readiness Hardening Kun Boundary

Source:

```text
D-0243 adds internal Go runtime readiness scaffolds: provider/MCP matrices,
candidate durable root crash/restart drill, packaged desktop QA checklist, and
main-adapter G6 readiness status. It does not change the Kun-derived desktop
product surface.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible `analytix`; no Kun identity, Reasonix identity, deprecated bridge alias, or release identity drift is introduced. |
| Runtime contract | Keep `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE`; readiness status is an internal main/canary gate, not a public runtime-info schema expansion. |
| Settings/UI | No Go backend switcher, duplicate settings schema, runtime-control panel, job manager panel, or MCP-indexer UI is exposed. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin, approval, user-input, usage, update, and release surfaces are not touched. |
| Rollback | Missing/skipped D-0243 durable/provider/MCP/packaged evidence rejects Go runtime-candidate startup and returns to TypeScript. |

Validation:

```text
`g6_readiness_test.go`, `runtime_server_test.go`,
`go-runtime-conformance.test.ts`, `src/main/runtime/analytix-adapter.test.ts`,
`scripts/d0243-packaged-go-runtime-qa.mjs`, and
`scan-product-sovereignty.cjs` prove the hardening is internal, defaults to
not-ready, and does not expose new product routes or settings.
```

## 2026-06-23 - D-0242 Go Runtime Server Kun Boundary

Source:

```text
D-0242 adds an internal Go runtime server contract binary for thread/session,
turn, SSE replay, approval/user-input, provider/cache, fake MCP, and job-lineage
contract coverage. It does not change the Kun-derived desktop product surface.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible `analytix`; no Kun identity, deprecated bridge alias, release identity drift, or Reasonix identity is introduced. |
| Runtime contract | Keep `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE`; D-0242 uses the existing contract shape and default TypeScript remains active. |
| Settings/UI | No Go backend switcher, duplicate settings schema, runtime-control panel, job manager panel, or MCP-indexer UI is exposed. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin, approval, user-input, usage, update, and release surfaces are not touched. |
| Rollback | Runtime-candidate Go canary failure stops Go, cleans temp state, records fallback status, and returns to the TypeScript runtime. |

Validation:

```text
`go-runtime-conformance.test.ts`, `runtime_server_test.go`,
`src/main/runtime/analytix-adapter.test.ts`, and
`scan-product-sovereignty.cjs` prove the server is internal, keeps TypeScript
default, uses the existing public contract, and exposes no Reasonix public
protocol or new top-level product route.
```

## 2026-06-23 - D-0241 Go Production-Candidate Kun Boundary

Source:

```text
D-0241 adds internal Go production-candidate runtime code for provider,
durable replay, approval/user-input, fake MCP, job lineage, and main-adapter
canary behavior. It does not change the Kun-derived desktop product surface.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible `analytix`; no Kun identity, deprecated bridge alias, release identity drift, or Reasonix identity is introduced. |
| Runtime contract | Keep `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE`; D-0241 is internal and default TypeScript remains active. |
| Settings/UI | No Go backend switcher, duplicate settings schema, runtime-control panel, job manager panel, or MCP-indexer UI is exposed. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin, approval, user-input, usage, update, and release surfaces are not touched. |
| Rollback | Production-candidate Go canary failure stops Go, cleans temp state, records fallback status, and returns to the TypeScript runtime. |

Validation:

```text
`go-production-candidate-conformance.test.ts`,
`src/main/runtime/analytix-adapter.test.ts`, and
`scan-product-sovereignty.cjs` prove the slice is internal, has no renderer/
preload/settings/UI change, keeps TypeScript default, and exposes no Reasonix
public protocol or new top-level product route.
```

## 2026-06-23 - D-0240 G6 Retirement Cleanup Kun Boundary

Source:

```text
D-0240 adds an internal Go kernel conformance route that records which
TypeScript/default-runtime paths stay retained before G6, which temporary
conformance paths are deleted after G6, and which redundant public surfaces
remain forbidden.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible `analytix`; no Kun identity, bridge alias, or release identity change. |
| Runtime contract | Keep TypeScript runtime as the default backend; cleanup evidence exists only under `/v1/conformance/kernel/g6-retirement-cleanup`. |
| Settings/UI | No Go switcher, backend selector, duplicate settings schema, or runtime-control panel is exposed. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin, approval, and user-input surfaces are not touched. |

Validation:

```text
`go-kernel-live-scaffold-conformance.test.ts`, `live_local_kernel_test.go`,
and `scan-product-sovereignty.cjs` prove the cleanup route is fixture-only,
retains the current TypeScript production path, and records zero forbidden
renderer-visible or UI redundancy.
```

## 2026-06-23 - D-0239 Go Kernel Scaffold Kun Boundary

Source:

```text
D-0239 adds fixture-backed Go kernel conformance routes for durable event
sink, loop controller, cache accounting, approval/user-input gates, MCP catalog
recovery, and job/sub-agent orchestration proof. The routes are internal
conformance evidence only and do not change Kun-derived product workflows.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible `analytix`; no Kun identity, bridge alias, or release identity change. |
| Runtime contract | Keep renderer/preload/main contract unchanged; kernel proof exists only below `/v1/conformance/kernel/*` in the Go sidecar. |
| Settings/UI | No Go switcher, job manager panel, subagent panel, AutoResearch entry, or MCP-indexer UI is exposed. |
| Desktop baseline | Code, Write, SDD, Connect Phone, schedule, terminal, settings, plugin, approval, and user-input surfaces are not touched. |

Validation:

```text
`go-kernel-live-scaffold-conformance.test.ts` and `live_local_kernel_test.go`
prove kernel evidence is fixture-only, default backend remains disabled,
renderer-visible Go route remains forbidden, and job/sub-agent execution stays
deferred.
```

## 2026-06-23 - D-0238 Backend-Neutral Adapter Kun Boundary

Source:

```text
D-0238 adds an internal/test/conformance backend gate in Electron main so the
Go live-local sidecar can be started behind analytix contracts. It does not
change the Kun-derived product entry, UI shell, desktop workflow surface, or
settings UI.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE`; only main owns the internal backend gate. |
| Settings | Keep top-level `runtime`; no Go switch, backend selector, config root, or runtime-control panel is exposed. |
| Default backend | Historical D-0238 stage: TypeScript was still default and the Go sidecar required `ANALYTIX_RUNTIME_BACKEND=go-conformance` plus `ANALYTIX_GO_RUNTIME_CONFORMANCE=1`. Current status is Go default with TypeScript backend retired in current adapter. |
| Canary | Main accepts Go only after health/thread API/loop-boundary checks and a hidden `/v1/runtime/go` check; current Go-default startup fails closed instead of rolling back to TypeScript. |

Validation:

```text
`src/main/runtime/analytix-adapter.test.ts` proved the gate stayed internal at
D-0238 time, Go canary boundaries were fixture-only, and a
default-backend/renderer-visible Go claim was rejected. Current Go default
validation is covered by the formal `runtime-go-*` gates.
```

## 2026-06-23 - D-0237 Go Minimal Agent Loop Kun Boundary

Source:

```text
D-0237 is a Go runtime conformance/prototype batch. It adds a fixture-backed
minimal agent loop proof below `/v1/conformance/loop/*`, using the D-0236 temp
durable store plus existing G2/G3/G4 fixtures, and does not change the
Kun-derived desktop workflow surface, route navigation, settings UI, or product
identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go loop sidecar is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no Go backend switch, loop-control setting, credential field, or runtime-control panel is exposed. |
| Go boundary | The minimal loop proof is test/conformance-only, fixture-scripted, local HTTP/SSE only, and keeps `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, `electronMainConnected:false`, `reasonixPublicProtocolAllowed:false`, tool execution, approval execution, MCP connection, credential reads, provider calls, and real workspace mutation disabled. |

Validation:

```text
Go and TS conformance tests start a local Go sidecar, run the fixture-only loop,
verify durable event replay/SSE/recovered state, approval/user-input/MCP/cache
boundaries, and prove no desktop product surface or packaged app behavior is
changed.
```

## 2026-06-23 - D-0236 Go Temp Durable Store Kun Boundary

Source:

```text
D-0236 is a Go runtime conformance/prototype batch. It adds an explicit opt-in
temp-dir durable event/session store and SSE replay harness inside
`packages/runtime-go` and does not change the Kun-derived desktop workflow
surface, route navigation, settings UI, or product identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go durable harness is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no Go backend switch, durable-store setting, credential field, or runtime-control panel is exposed. |
| Go boundary | The durable harness is test/conformance-only, explicit temp-dir opt-in, local HTTP only, and keeps `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, `electronMainConnected:false`, `reasonixPublicProtocolAllowed:false`, and real workspace/credential/provider/MCP attempts disabled. |

Validation:

```text
Go and TS conformance tests start a local Go sidecar, write only to temp
`events.jsonl`, verify event replay/SSE/thread-session state/recovered cache and
gate state, and prove no desktop product surface or packaged app behavior is
changed.
```

## 2026-06-23 - D-0235 Go G4 Approval/User-Input/MCP Manager Kun Boundary

Source:

```text
D-0235 is a Go runtime conformance/prototype batch. It advances the
test/conformance-only Go sidecar from G3 provider/cache streaming replay to
fixture-backed G4 approval/user-input/MCP manager replay and does not change the
Kun-derived desktop workflow surface, route navigation, settings UI, or product
identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go G4 harness is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no Go backend switch, MCP credential field, approval-control panel, or live MCP setting is exposed. |
| Go boundary | The G4 harness is test/conformance-only, fixture-backed, local HTTP only, and keeps `approvalExecutionAllowed:false`, `toolExecutionAllowed:false`, `mcpConnectionAllowed:false`, `credentialReadAllowed:false`, `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, and `electronMainConnected:false`. |

Validation:

```text
Focused Go tests prove the sidecar replays approval deny, user-input
cancel/submit/validation, remote-entry ports, MCP lifecycle/search/approval
annotation/reconnect/diagnostics, and secret redaction only from TS-owned
fixtures. No real tool, MCP server, credential, Electron bridge, renderer
route, settings surface, or product entry is added.
```

## 2026-06-23 - D-0234 Go G3 Provider/Cache Streaming Kun Boundary

Source:

```text
D-0234 is a Go runtime conformance/prototype batch. It advances the
test/conformance-only Go sidecar from G2 lifecycle replay to fixture-backed G3
provider/cache streaming replay and does not change the Kun-derived desktop
workflow surface, route navigation, settings UI, or product identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go provider/cache harness is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no Go provider backend switch, API-key field, or live provider setting is exposed. |
| Go boundary | The G3 harness is test/conformance-only, fixture-backed, local HTTP only, and keeps `providerLiveCallsAllowed:false`, `externalNetworkAllowed:false`, `apiKeyReadAllowed:false`, `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, and `electronMainConnected:false`. |

Validation:

```text
Focused Go and TS conformance tests prove the sidecar replays provider usage,
request shape, streaming, cache accounting, cache drift, and diagnostics only
from TS-owned fixtures. No real provider, API key, Electron bridge, renderer
route, or settings surface is added.
```

## 2026-06-23 - D-0233 Go Isolated Mutating G2 Lifecycle Kun Boundary

Source:

```text
D-0233 is a Go runtime conformance/prototype batch. It advances the D-0232
sidecar harness inside tests only and does not change the Kun-derived desktop
workflow surface, route navigation, settings UI, or product identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go sidecar is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no backend switch or Go runtime setting is exposed. |
| Go boundary | The sidecar is test/conformance-only, isolated in-memory for G2 mutating lifecycle replay, and keeps `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, and `electronMainConnected:false`. |

Validation:

```text
Focused Go tests prove the sidecar accepts only the TS-owned G2 mutating route
fixtures into isolated harness state, leaves temp `events.jsonl` unchanged, and
still rejects `/v1/runtime/go`, Reasonix routes, hidden product routes, approval
execution, provider live calls, MCP credentials, and file mutation.
```

## 2026-06-23 - D-0232 Go Live-Local Sidecar Prototype Kun Boundary

Source:

```text
D-0232 is a Go runtime conformance/prototype batch. It does not change the
Kun-derived desktop workflow surface, route navigation, settings UI, or product
identity.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep `analytix serve` and `Renderer -> window.analytix -> preload -> main -> runtime HTTP/SSE` unchanged; the Go sidecar is not connected to Electron main. |
| Settings | Keep top-level `runtime` settings; no backend switch or Go runtime setting is exposed. |
| Go boundary | The sidecar is test/conformance-only, GET-only for G2 replay, and keeps `defaultGoBackendEnabled:false`, `rendererVisibleGoRoutesAllowed:false`, and `electronMainConnected:false`. |

Validation:

```text
Focused TS conformance and Go tests prove the sidecar stays fixture-backed and
does not accept mutating route replay, approval execution, provider live calls,
MCP credentials, or file mutation.
```

## 2026-06-23 - D-0231 MCP Search Refresh Drift Kun Boundary

Source:

```text
D-0231 is an MCP evidence-closure batch. It does not change the Kun-derived
desktop workflow surface or add a Kun-incompatible top-level MCP-indexer entry.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Runtime contract | Keep MCP refresh evidence behind analytix runtime tests and G5 shadow; no renderer-visible Go route or default backend is added. |
| MCP product surface | Do not convert internal MCP search refresh proof into a public MCP-indexer product route. |

Validation:

```text
The D-0231 scan and final gates retain forbidden route, bridge alias,
MCP-indexer, default Go backend, and Rust/Tauri checks.
```

## 2026-06-23 - D-0230 Plan/Auto-Route State Reset Kun Boundary

Source:

```text
D-0230 is a Reasonix/Go runtime-quality absorption batch. It does not change
the Kun-derived desktop workflow surface or add a Kun-incompatible top-level
planner/auto-plan product entry.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Settings | Keep top-level `runtime` settings and do not add public auto-plan/project override settings. |
| Runtime contract | Keep reset/currentness evidence behind TypeScript runtime tests and G5 shadow; no renderer-visible Go route or default backend is added. |

Validation:

```text
The D-0230 scan and final gates retain forbidden route, bridge alias, settings
fallback, public auto-plan, default Go backend, and Rust/Tauri checks.
```

## 2026-06-23 - D-0229 Plan Step/Cancel/Cache G5 Shadow Kun Boundary

Source:

```text
D-0229 is a Reasonix/Go runtime-quality absorption batch. Kun 0.2.13/0.2.14
does not provide a product-level Workflow, Create Loop, Subagent,
AutoResearch, or MCP-indexer navigation entry for this capability.
```

Kun decision:

| Surface | Decision |
| --- | --- |
| Top-level navigation | Preserve the analytix/Kun route surface; no Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry is added. |
| Product identity | Keep user-visible product identity `analytix`; no Kun public identity or deprecated bridge alias is reintroduced. |
| Settings | Keep top-level `runtime` settings; no legacy Kun/Reasonix runtime-shaped save path is added. |
| Runtime contract | Keep the evidence behind `analytix serve` / TypeScript runtime / G5 shadow; no renderer-visible Go route or default backend is added. |

Validation:

```text
The D-0229 scan and final gates retain forbidden route, bridge alias, settings
fallback, default Go backend, and Rust/Tauri checks.
```

## 2026-06-20 - Kun master code-level audit

Source:

```text
Kun version/tag: master snapshot
Commit range: 8f20403
Release notes: not used for this audit; repository source and README were reviewed
Local comparison branch: /tmp/analytix-upstreams/Kun
```

Summary:

```text
Kun remains the product-workflow upstream. The reviewed tree confirms the main
absorption value is Code, Write, SDD, Connect Phone, schedule, MCP/Skills,
multimodal/media, provider presets, and the existing TypeScript runtime
contract. Future absorption should be delta-based because analytix already
started from the Kun 0.2.13 lineage.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `kun/src/contracts`, `kun/src/server/routes`, `kun/src/services` | runtime | `must absorb` | Use as product/runtime parity source, adapted to analytix contracts. | Compare against current `packages/runtime` before each batch. |
| `kun/src/loop` | runtime | `must absorb` | Preserve cache-first loop, append-only log, compaction, repair, and storm guard behavior where it improves current runtime. | Convert useful tests to analytix fixtures. |
| `kun/src/adapters/model`, `kun/src/adapters/tool` | provider/tool | `should absorb` | Port endpoint/tool/provider improvements through analytix shared schemas and approval flow. | Add provider/tool matrix rows. |
| `src/renderer/src/components/write`, `src/renderer/src/write`, `docs/WRITE_*` | Write | `should absorb` | Keep Write product breadth while preserving upgraded analytix UI. | Add Write workflow benchmark evidence. |
| `src/renderer/src/components/sdd`, `src/renderer/src/sdd` | SDD | `should absorb` | Port requirement-first flow, prompts, trace, and restore behavior. | Add SDD benchmark evidence. |
| Connect Phone, schedule, remote task flow | desktop workflow | `should absorb` | Absorb workflow and streaming fixes; user-facing copy remains analytix / Connect Phone. | Add remote streaming and schedule QA. |
| Old product identity, old bridge/settings, old UI shell | identity/UI | `reject` | Do not reintroduce current-product Kun naming or old bridge/settings surfaces. | Naming scans remain mandatory. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Kun UI changes vs upgraded analytix UI | analytix UI wins; Kun contributes workflow behavior only. | `code-level-absorption-blueprint.md` |
| `kun serve`, `window.kunGui`, `agents.kun` vs analytix contracts | analytix `analytix serve`, `window.analytix`, and top-level `runtime` win. | spec 08 / spec 01 |

Absorbed:

```text
No implementation was absorbed in this audit entry. The audit produced the
code-level landing plan in code-level-absorption-blueprint.md.
```

Skipped:

```text
No source files were rejected individually in this audit; rejected categories
are current-product Kun identity, old bridge/settings, and UI replacement.
```

Validation:

```text
Reviewed repository snapshot 8f20403, inspected key runtime and renderer
directories, added code-level absorption blueprint, and ran git diff --check.
```

Remaining risks:

```text
The next implementation batch must compare exact upstream deltas against the
current analytix tree before copying code. This audit does not by itself prove
Kun parity or product superiority.
```

## 2026-06-20 - Kun develop pre-release candidate audit

Source:

```text
Kun version/tag: develop snapshot
Commit range: master 8f20403 -> develop ab24a77
Release notes: develop includes unreleased or pre-release work beyond master
Local comparison branch: /tmp/analytix-upstreams/Kun
```

Summary:

```text
Kun develop is a large candidate absorption line, not a safe single-batch merge.
It adds a workflow/Loop builder, workflow runtime and UI, Telegram Connect,
provider routing by thread.providerId, endpoint/model fixes, preload sandbox
fixes, git checkpoint service, tray session menu, project/worktree grouping,
composer/picker refinements, and many tests.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Preload sandbox fixes | desktop startup/security | `must absorb` | Port equivalent fixes if analytix preload can hit node builtin/sandbox issues. | Add preload startup tests. |
| `thread.providerId` and per-provider HTTP clients | provider/runtime | `must absorb` | Adapt through analytix provider contract and top-level `runtime` settings. | Extend provider matrix and settings tests. |
| GLM endpoint alignment and deprecated Xiaomi model removal | provider presets | `should absorb` | Update provider presets only through analytix provider schema. | Add preset/probe tests. |
| Telegram Connect improvements | Connect Phone | `should absorb` | Fold into Connect Phone without user-visible old naming. | Add Connect Phone settings and remote task QA. |
| Workflow/Loop builder and runtime | product automation | `needs redesign` | High-value feature source, but must become analytix-native workflow automation. | Create a separate proposal before code import. |
| Git checkpoint service | review/safety | `should absorb` | Evaluate as part of review/rollback safety. | Add git checkpoint tests. |
| Tray session menu and worktree grouping | desktop UX | `should absorb` | Port through upgraded analytix UI patterns. | Add tray/session/worktree QA. |
| Broad renderer UI changes | UI | `needs redesign` | Do not copy over upgraded analytix shell. | Use as behavior reference only. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Workflow builder as a major new surface | Requires analytix-native proposal and UX review before implementation. | `code-level-implementation-plan.md` |
| Develop branch volatility | Do not merge as one batch; split into provider/preload/Connect Phone/workflow/review utilities. | `code-level-implementation-plan.md` |

Absorbed:

```text
No implementation was absorbed in this audit entry. The audit updates the
implementation plan so follow-up threads can split Kun develop safely.
```

Skipped:

```text
No feature is permanently rejected here. Workflow/Loop and broad UI changes are
deferred pending analytix-native redesign.
```

Validation:

```text
Fetched origin/develop at ab24a77, reviewed commit log, diff stat, and changed
file list from origin/master..origin/develop.
```

Remaining risks:

```text
The develop diff is very large: 207 files and more than 22k insertions in the
local audit. It must be split into smaller implementation batches before any
code import.
```

## 2026-06-20 - Kun master drift audit to 8602476

Source:

```text
Kun version/tag: master snapshot
Commit range: 8f2040349fba47fcd8e8b94f50b131943af839b2..8602476c5c449b4561473ad5f31081ee93dc782e
Remote HEAD: master 8602476c5c449b4561473ad5f31081ee93dc782e
Develop HEAD: ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Local comparison branch: legacy-origin/master after `git fetch legacy-origin master develop`
```

Summary:

```text
Kun master moved from 8f20403 to 8602476 by merging develop (#444). The earlier
develop pre-review is no longer only a candidate line: Create Loop / Workflow,
Telegram Connect runtime, provider routing and endpoint fixes, preload sandbox
startup fixes, git checkpoint, tray session menu, worktree/project grouping,
composer/sidebar polish, and v0.2.15 release notes are now stable-master drift.
Analytix must still absorb by capability through analytix identity, settings,
bridge, runtime contract, UI, tests, benchmark, and QA gates.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Create Loop / Workflow runtime and UI (`src/main/workflow-runtime.ts`, `src/renderer/src/components/workflow`, `src/shared/workflow-*`, `docs/workflow-loop*`) | workflow automation | `redesign` | Confirmed merged into master through `8602476`; do not copy the Kun UI shell. Redesign as an analytix-native workflow automation surface. | P1.3 proposal/design, then runtime fixtures and desktop workflow QA. |
| Telegram runtime and Connect Phone settings/UI (`a502a6d`, `src/main/telegram-runtime.ts`) | Connect Phone / IM | `should absorb` | Current analytix has Connect Phone and Telegram relay guidance, but no Kun-style local Telegram runtime. Absorb into Connect Phone with user-facing analytix / Connect Phone copy only. | P1.2 Connect Phone / Telegram settings, pairing, relay/runtime, approval and streaming QA. |
| Per-provider runtime routing (`6bd1edc`, `thread.providerId`, `MultiProviderModelClient`) | runtime/provider | `must absorb` | Current analytix carries provider ids through UI/schedule/Connect Phone, but runtime composition still uses one default `CompatModelClient`. Add per-provider HTTP routing behind analytix runtime contracts. | P1.1 provider/runtime fixture: requested provider id routes to that provider without changing renderer contracts. |
| GLM coding endpoints and compaction repair (`5772400`) | provider / loop correctness | `must absorb` | Current analytix has GLM presets and per-model endpoint support, but still uses old GLM base URLs and lacks the custom full-endpoint correction in presets/tests. | P1.1 provider matrix: Zhipu/Z.ai use custom full `/chat/completions` endpoints; compaction keeps tool-call/result pairs intact. |
| Deprecated Xiaomi `mimo-v2-flash` removal (`694fc4e`) | provider preset correctness | `must absorb` | Current analytix still lists `mimo-v2-flash`; remove through analytix provider presets and docs. | P1.1 provider preset test and docs update. |
| Preload sandbox node-builtin startup fix (`b14e31f`, `dd3252d`) | preload / desktop startup | `already covered` | Current analytix preload imports only Electron APIs and does not require `node:os`; keep this as a startup regression guard, not a Kun bridge/API port. | P1.1 add/confirm sandbox preload startup test; do not add `window.kunGui` or flat renderer calls. |
| Git turn rollback checkpoints (`8cbf8e6`) | review/safety | `should absorb` | Useful safety primitive, but Kun refs use `refs/kun/checkpoints`; analytix must adapt names, storage, review UI, and generated-file/history integration. | P3 checkpoint batch with tests for create/restore, untracked files, event replay, and no Kun ref identity. |
| Dynamic tray session menu (`ec42254`) | desktop utility | `should absorb` | Current analytix has tray hide/quit behavior only; absorb session menu through analytix tray labels and runtime thread contract. | P1.1 or P3 desktop utility slice with tray/session QA. |
| Worktree/project grouping, composer picker, sidebar, terminal, shortcut fixes (`5f17899`, `67a6016`, `f76c8ce`, `5e4c571`, `261d7f5`, related UI commits) | desktop UX | `should absorb` | Port behavior into current analytix UI patterns; do not copy old Kun shell or broad renderer layout. | P1.1 stable desktop fixes, focused renderer tests, then Electron QA. |
| Release-note backfill and README workflow copy | docs/release | `already covered` / `optional` | Current analytix docs own product identity; Kun release notes remain upstream archaeology only. | Reference in ledger; do not import current-product Kun release identity. |
| Kun product identity, `kun serve`, `window.kunGui`, `agents.kun`, `refs/kun/*`, broad Kun UI shell | identity / protocol | `reject` | Do not reintroduce current-product Kun naming or runtime surfaces. | Naming and bridge scans remain mandatory in implementation batches. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Kun develop is now stable master, but its Workflow UI is a large product surface | Stable status upgrades priority, not landing mode; Workflow/Create Loop still requires analytix-native redesign. | `code-level-implementation-plan.md` |
| Kun per-provider runtime uses Kun config/CLI names | Absorb per-provider routing behavior behind top-level `runtime` and `analytix serve`; do not expose Kun config or CLI. | spec 08 / provider contract rules |
| Kun preload fix passes home dir through `--kun-home-dir` | analytix may use the same sandbox principle but must use analytix naming or avoid the argument if unnecessary. | spec 01 / spec 08 |
| Kun git checkpoint refs use `refs/kun/checkpoints/*` | Rename/adapt storage and refs before landing. | `code-level-absorption-blueprint.md` |

Absorbed:

```text
No implementation was absorbed in this closure batch. The batch updates the
architectural ledger and next implementation order after Kun master drift.
```

Skipped:

```text
No Kun stable-master code was ported here. Workflow/Create Loop direct UI copy,
Kun public identity, old bridge/settings/runtime naming, and Kun-native
checkpoint refs remain rejected or redesign-only.
```

Validation:

```text
Ran git status --short --branch.
Ran git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop.
Ran git ls-remote https://github.com/esengine/DeepSeek-Reasonix.git refs/heads/main-v2.
Ran git fetch legacy-origin master develop.
Verified legacy-origin/master = 8602476c5c449b4561473ad5f31081ee93dc782e and
legacy-origin/develop = ab24a77f0f68fcb1b2c160361e4c50cd92b02928.
Verified ab24a77 is an ancestor of 8602476.
Reviewed git log, git diff --stat, git diff --name-status, and focused diffs
for workflow, Telegram, provider, preload, tray, worktree, and git checkpoint
areas.
Ran git diff --check.
```

Remaining risks:

```text
This is a planning and closure audit only. Kun master 8602476 product/runtime
delta is not implemented in analytix yet. Do not claim Create Loop, Telegram
runtime, per-provider runtime routing, GLM endpoint fixes, Xiaomi model removal,
dynamic tray sessions, or git checkpoint absorption until their dedicated
implementation batches land with tests and QA.
```

## 2026-06-20 - P1.1 stable critical code absorption

Source:

```text
Kun version/tag: master snapshot
Commit range: stable critical items from master 8602476c5c449b4561473ad5f31081ee93dc782e
Reasonix preflight: main-v2 advanced from 6d404d80094bb4153bbaec4d7a2739e875d5c8c7 to be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Closure recheck: Kun master 8602476c5c449b4561473ad5f31081ee93dc782e, Kun develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928, Reasonix main-v2 be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Local comparison branches: /tmp/analytix-upstreams/Kun and /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
P1.1 landed the stable critical Kun provider/runtime/preload fixes through
analytix-native contracts. The implementation keeps product identity as
analytix, uses only window.analytix, keeps top-level runtime settings, and
preserves analytix serve as the public runtime CLI.
```

Absorbed:

| Upstream item | analytix implementation | Proof |
| --- | --- | --- |
| `6bd1edc` per-provider runtime routing | Added `thread.providerId` to runtime thread/turn/review contracts and renderer DTOs; GUI sends composer providerId without restarting the runtime; serve mode wraps `CompatModelClient` in `MultiProviderModelClient` and routes by stored thread provider; HybridThreadStore persists `provider_id` in its rebuildable SQLite index and preserves it when sidecars rebuild list summaries. | Contract/domain/hybrid/thread-service tests; `tests/model-client.test.ts`; renderer runtime/store tests; hybrid-store providerId SQLite, deleted-index sidecar, and damaged-message sidecar tests. |
| `5772400` GLM endpoint and compaction repair | Zhipu/Z.ai coding presets now use custom full `/chat/completions` endpoints; runtime provider env preserves custom endpoint format; compaction tail repair keeps tool-call/tool-result pairs together. | Provider preset tests, provider URL/probe/write-inline/scheduled tests, runtime model-client test, `tests/context-compactor.test.ts`. |
| `694fc4e` Xiaomi removal | Removed deprecated Xiaomi flash model from presets, token-plan presets, docs, and active test fixtures. | Provider preset tests and active-source scans. |
| `b14e31f` / `dd3252d` preload sandbox startup guard | Current analytix preload already avoids node builtin imports and exposes only `window.analytix`; added a source regression guard. | `src/preload/preload-sandbox.test.ts`. |

Deferred:

| Item | Reason | Next target |
| --- | --- | --- |
| Workflow/Create Loop | Large product surface requiring analytix-native UX and runtime proposal. | P1.3 |
| Telegram / full Connect Phone absorption | Out of P1.1 stable critical scope; needs pairing, relay, approval, and desktop QA. | P1.2 |
| Git checkpoint / rollback refs | Kun uses `refs/kun/*`; analytix needs renamed storage and review/generated-file integration. | P3 |
| Tray/session/worktree/composer UI drift | Useful desktop fixes but not provider/runtime/preload critical path. | P3 or dedicated desktop utility slice |
| Reasonix `ad3d742` topic migration marker | New Reasonix HEAD change is unrelated to P1.1 provider/runtime/preload contracts. | Future migration-performance batch |

Validation:

```text
npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/contracts.test.ts tests/domain.test.ts tests/thread-service.test.ts tests/model-client.test.ts tests/context-compactor.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/agent/analytix-mapper.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings-provider.test.ts -- --runInBand
npm run test -- src/shared/openai-compat-url.test.ts src/main/upstream-models.test.ts src/main/provider-connection.test.ts src/main/claw-scheduled-task-detector.test.ts src/main/services/write-inline-completion-service.test.ts -- --runInBand
npm --prefix packages/runtime run typecheck
npm run typecheck
git diff --check
```

Remaining risks:

```text
P1.1 is closed for the scoped provider/runtime/preload code slice, but this is
not a release/G0 closure. Electron desktop startup QA, packaging, and the
deferred Kun product surfaces remain open.
```

## 2026-06-20 - P1.2 Connect Phone / Telegram implementation slice

Source:

```text
Kun version/tag: master snapshot
Commit range: Telegram / Connect Phone item from master 8602476c5c449b4561473ad5f31081ee93dc782e
Focused Kun commit: a502a6d fix(claw): improve telegram connect settings and ui (#434)
Closure recheck: Kun master 8602476c5c449b4561473ad5f31081ee93dc782e, Kun develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928, Reasonix main-v2 be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Local comparison branches: /tmp/analytix-upstreams/Kun and /tmp/analytix-upstreams/DeepSeek-Reasonix
```

Summary:

```text
P1.2 absorbs Kun's Telegram Connect capability as an analytix-native remote
workflow slice. The landed path keeps Connect Phone user-facing copy, stores
Telegram credentials in the existing phone-channel settings, exposes only
window.analytix.connectPhone, and starts turns through the analytix runtime
HTTP/SSE contract with P1.1 providerId routing intact.
```

Absorbed:

| Upstream item | analytix implementation | Proof |
| --- | --- | --- |
| Kun `src/main/telegram-runtime.ts` token verification and long polling | Added `src/main/telegram-runtime.ts` with Telegram `getMe` verification, long-poll `getUpdates`, retry/backoff logging, private-chat allowlist, photo download cap, and `analytix-telegram-attachments` temp naming. | `src/main/telegram-runtime.test.ts` covers allowlist parsing, invalid token fast-fail, successful `getMe`, rejected token, and network failure mapping. |
| Kun Telegram Connect settings/UI | Extended analytix phone-channel provider types to include `telegram`; added `connectTelegramBot` to `window.analytix.connectPhone`; Connect Phone renders a token/allowlist form and settings/schedule provider labels show Telegram. | `ConnectPhoneView.test.ts`, `settings-section-claw.test.ts`, `ScheduleTasksView.test.ts`, shared settings and IPC schema tests. |
| Remote Telegram inbound relay | `ClawRuntime.handleTelegramUpdate` resolves Telegram channel/conversation, sends welcome/command replies, delegates schedule creation, uploads local photo paths to `/v1/attachments`, starts analytix turns with `disableUserInput`, auto approval, sandbox full access, and replies over Telegram. | `claw-runtime.test.ts` verifies inbound text/image relay, attachment id propagation, provider/model routing, schedule detector options, and Telegram reply. |
| GUI/local resume back to remote Telegram chat | `mirrorThreadMessageToIm` now supports Telegram conversations; user-direction mirrors include `From Analytix:` while assistant-direction mirrors preserve assistant text. | `claw-runtime.test.ts` local Telegram mirror fixture. |
| Delayed result push and recovery logging | Existing delayed-result push now recognizes active Telegram channels; attachment upload failures log and continue instead of blocking text turns; Telegram runtime logs poll/send/download failures with channel/chat context. | Focused runtime tests plus implementation review. |

Rejected:

| Upstream item | Reason |
| --- | --- |
| Kun product identity, `window.kunGui`, `agents.kun`, `kun serve`, `refs/kun/*` | P1.2 must remain analytix-native. No old bridge alias, old settings envelope, old runtime-control panel, or Kun public identity is reintroduced. |
| External relay-only Telegram guidance | Analytix now owns a local Telegram runtime behind Connect Phone; relay-only docs are replaced by bot-token and allowlist copy. |
| Broad Kun Connect Phone visual shell | Current analytix UI remains sovereign; only targeted controls, labels, and tests were added. |

Deferred:

| Item | Reason | Next target |
| --- | --- | --- |
| Live Telegram bot desktop QA | Requires real bot token/network and cannot be completed by unit tests alone. | Desktop QA/release readiness before G0. |
| Workflow/Create Loop | Still out of P1.2 scope and needs analytix-native redesign. | P1.3 |
| Git checkpoint / rollback | Still rejected as Kun refs and deferred to renamed analytix safety design. | P3 |
| Go runtime | No Go runtime work in this slice. | Future Go runtime phases |

Validation:

```text
npm run test -- src/renderer/src/components/chat/ConnectPhoneView.test.ts src/renderer/src/components/chat/SidebarClawDialogHelpers.test.ts src/renderer/src/components/settings-section-claw.test.ts src/renderer/src/components/schedule/ScheduleTasksView.test.ts src/shared/app-settings.test.ts src/main/ipc/app-ipc-schemas.test.ts src/main/ipc/register-app-ipc-handlers.test.ts src/main/telegram-runtime.test.ts src/main/claw-runtime.test.ts -- --runInBand
npm --prefix packages/runtime run test -- tests/hybrid-store.test.ts tests/model-client.test.ts tests/context-compactor.test.ts --no-file-parallelism --maxWorkers=1
npm run test -- src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/agent/analytix-mapper.test.ts src/preload/preload-sandbox.test.ts src/shared/app-settings-provider.test.ts -- --runInBand
npm --prefix packages/runtime run typecheck
npm run typecheck
git diff --check
```

Remaining risks:

```text
P1.2 is closed only as a Connect Phone / Telegram code slice. It is not a
release/G0/full upstream closure, and it does not claim live Telegram desktop
QA, Workflow/Create Loop, Go runtime, or git checkpoint absorption.
```

## 2026-06-20 - P1.3 Workflow / Create Loop implementation slice

Source:

```text
Kun version/tag: master snapshot
Commit range: Workflow / Create Loop item from master 8602476c5c449b4561473ad5f31081ee93dc782e
Focused Kun commits: 5488b9a through a43a2d5, including Create Loop runtime, typed inputs, run log, human approval, hook bridge, and node catalog work
Closure recheck: Kun master 8602476c5c449b4561473ad5f31081ee93dc782e, Kun develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928, Reasonix main-v2 be67a498adcaed6e33dcf3cbd25395e9cbccd6fa
Local comparison branch: /tmp/analytix-upstreams/Kun using origin/master
```

Summary:

```text
P1.3 absorbs Kun's Workflow / Create Loop product idea as a narrow
analytix-native manual workflow slice. The landing does not copy Kun's canvas,
settings envelope, bridge, hook bridge, MCP exposure, local webhook, or
workflow-specific runtime-control shell. This entry originally landed a
Workflow route, but the P0 product-entry correction below supersedes that
route decision because Kun 0.2.13 and 0.2.14 do not have an equivalent
Workflow/Create Loop top-level entry. The retained part is the Create Loop
state machine that creates an analytix runtime thread, relays Plan / Execute /
Review turns through the existing Analytix HTTP/SSE contract, records a run log,
pauses on runtime approval/user-input cards, and can resume or retry the same
workflow run.
```

Absorbed:

| Upstream item | analytix implementation | Proof |
| --- | --- | --- |
| Kun Create Loop run history idea | Retained the focused Create Loop view/state machine as unexposed future capability; P0 removed the top-level analytix Workflow route/sidebar/workbench entry. | `src/renderer/src/components/workflow/WorkflowCreateLoopView.test.ts` still covers the isolated view; P0 route guards cover the hidden entry. |
| Kun graph runtime's minimum state-machine behavior | Reimplemented a scoped Plan / Execute / Review state machine in `src/renderer/src/workflow/create-loop-runtime.ts`; no Kun UI shell or workflow settings storage was copied. | `create-loop-runtime.test.ts` covers successful create/relay, prompt context propagation, and final success. |
| Kun AI-node-to-runtime-thread mapping | Workflow execution creates a normal analytix thread and sends turns through `AgentProvider.sendUserMessage`, preserving `model` and `providerId`. | Focused tests assert runtime thread creation and every turn carries `model`, `providerId`, and `mode: agent`. |
| Kun live run-log concept | Each step records status, turn id, output, waiting reason, and errors in the Workflow view. | UI test covers waiting state; runtime tests cover run updates. |
| Kun failure handling idea | Failed steps can be retried on the same workflow run/thread; waiting approval/user-input steps can be resumed without duplicating the already-waiting turn. | Focused tests cover approval wait/resume, user-input wait, and retry after failed step. |

Rejected:

| Upstream item | Reason |
| --- | --- |
| `window.kunGui`, `agents.kun`, `kun serve`, Kun product identity, and old bridge/settings names | analytix identity, bridge, settings, and public runtime contract remain sovereign. |
| Full Kun visual canvas / React Flow shell | Too broad for the P1.3 code slice and conflicts with current analytix UI direction; future builder work must be analytix-native. |
| Kun top-level `workflow` settings envelope and settings-stored run history | Current P1.3 slice avoids adding a new settings schema. Future persistent workflow storage must be contract-designed rather than copied from app settings. |
| Kun local webhook, hook trigger bridge, and MCP `list_workflows` / `run_workflow` exposure | They introduce additional protocol surfaces and old hook/MCP assumptions; defer until workflow contracts, event replay, permissions, and desktop QA are ready. |
| Custom Python/Bash modules, image generation node, subworkflow/loop parallel execution | Useful later, but out of the minimal Create Loop closure and needs stronger sandbox/generated-file/event contracts. |

Deferred:

| Item | Reason | Next target |
| --- | --- | --- |
| Persistent workflow event log and restart resume | The landed slice keeps run state in the renderer and uses the runtime thread as the durable execution record; full event-sourced workflow storage remains a larger contract change. | P3 or a dedicated workflow persistence slice before release/G0 claims. |
| Visual builder, typed variable picker, node descriptors, hook triggers, and schedule/webhook triggers | High-value Kun capabilities, but they need analytix-native UX and runtime contracts. | Follow-on P1.3.x / P3 workflow-builder slices. |
| Generated-files/review summary first-class propagation | Current review step asks the runtime to summarize generated files and risks, but workflow does not yet create new `GeneratedFilesPanel` or `ReviewSummaryCard` contracts. | P3 review/generated-files integration. |
| Real Electron Workflow walkthrough | Unit/render tests cover code paths; desktop QA remains required before release/G0. | Desktop QA / release-readiness gate. |

Validation:

```text
npm run test -- src/renderer/src/workflow/create-loop-runtime.test.ts src/renderer/src/components/workflow/WorkflowCreateLoopView.test.ts -- --runInBand
npx tsc --noEmit -p tsconfig.web.json --pretty false
P1.1/P1.2 regressions, full typechecks, and git diff --check remain required before final commit.
```

Remaining risks:

```text
P1.3 is closed only for the scoped Create Loop code slice after final
validation passes. It is not a full Kun Workflow builder absorption, not a
release/G0 closure, and not a Go runtime milestone.
```

## 2026-06-20 - P0 product-entry correction and d09d52b installer absorption

Source:

```text
Kun master: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop: 247076f297170c3d0c558baffb073c629894faea
Kun v0.2.13 tag: 201a1469ffbd911f6b95d450b78471e51b42acae
Kun v0.2.14 tag: 06be05d76223208724c07301fa0f830641a06e6f
Focused installer commit: d09d52b0ceb11bcdd0bc85f7dcb6fe49844a23ff
```

Summary:

```text
P0 corrects an analytix product-position error: Workflow/Create Loop was
absorbed from the Kun current/master line as a top-level analytix route even
though Kun 0.2.13 and 0.2.14 do not contain an equivalent top-level Workflow
entry. The corrected rule is that Kun product capabilities must keep the target
Kun version's product position, entry level, and trigger path unless an
independent analytix spec explicitly approves a different level and tests prove
the new placement.
```

Kun Workflow/Create Loop position evidence:

| Kun ref | Evidence |
| --- | --- |
| `v0.2.13` `201a146` | `src/renderer/src/store/chat-store-types.ts` AppRoute is `chat/write/settings/plugins/claw/schedule`; no `workflow`. `Sidebar.tsx` has Code/Write, plugins, schedule, Connect Phone/settings footer; no Workflow command. |
| `v0.2.14` `06be05d` | Same route and sidebar surface as 0.2.13 for this entry. |
| `master` `8602476` | Adds top-level `workflow` AppRoute, `openWorkflow`, Sidebar Workflow command, Workbench `WorkflowView`, docs `workflow-loop.md`, and workflow runtime files. |
| analytix P0 result | `workflow` is removed from AppRoute/actions, Sidebar command, and Workbench main stage. `WorkflowCreateLoopView` and `create-loop-runtime` remain isolated and unexposed. |

Product-entry audit table:

| Entry | analytix current position | Kun 0.2.13 | Kun 0.2.14 | Kun master/current | Allowed to keep | P0 decision | Evidence path |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Code | `WorkspaceModeTabs` Code tab, `AppRoute` `chat`, default Workbench main stage | yes | yes | yes | yes | unchanged | `src/renderer/src/components/chat/WorkspaceModeTabs.tsx`; Kun `Sidebar.tsx`; `chat-store-types.ts` |
| Write | `WorkspaceModeTabs` Write tab, `AppRoute` `write`, `WriteSidebar`/`WriteWorkspaceView` | yes | yes | yes | yes | unchanged | `src/renderer/src/components/write/WriteSidebar.tsx`; `src/renderer/src/components/Workbench.tsx` |
| Plugins / MCP / Skills | Sidebar command and `AppRoute` `plugins`, `PluginMarketplaceView`; settings agents can open plugin management | yes | yes | yes | yes | unchanged | `src/renderer/src/components/chat/Sidebar.tsx`; `src/renderer/src/components/PluginMarketplaceView.tsx`; Kun `PluginMarketplaceView.tsx` |
| Schedule | Sidebar command and `AppRoute` `schedule`, `ScheduleTasksView`; schedule also reachable from session header/task flows | yes | yes | yes | yes | unchanged | `src/renderer/src/components/chat/Sidebar.tsx`; `src/renderer/src/components/schedule/ScheduleTasksView.tsx`; Kun `Sidebar.tsx` |
| Connect Phone | Sidebar footer toggle with `ConnectPhoneSidebarPanel`; internal compatibility route remains `claw` while user copy says Connect Phone | yes, old `claw` surface | yes, old `claw` surface | yes | yes, with analytix/Connect Phone naming | unchanged | `src/renderer/src/components/chat/Sidebar.tsx`; `ConnectPhoneView.tsx`; settings `claw` compatibility section |
| SDD new requirement | Sidebar accent command inside Code/Write project context; SDD draft editor opens inside chat/workbench, not as a standalone top route | yes | yes | yes | yes | unchanged | `src/renderer/src/components/chat/Sidebar.tsx`; `src/renderer/src/components/sdd/SddDraftEditorView.tsx` |
| Settings sections | Settings route with general/providers/write/media/speech/agents/archives/permissions/worktree/memory/shortcuts/easterEgg/updates/claw/debug; image generation remains from earlier baseline/settings support | yes for the broad settings surface | yes for the broad settings surface | yes, with minor category drift | yes; not a new main navigation entry | unchanged; speech/local Whisper develop drift is record-only | `src/renderer/src/components/SettingsSidebar.tsx`; `src/shared/app-settings-runtime.ts`; Kun `SettingsSidebar.tsx` |
| Agent / tool visible entry | Agents settings and plugin marketplace; tool visibility remains runtime/plugin scoped, not a separate top route | yes | yes | yes | yes | unchanged | `src/renderer/src/components/settings-section-agents.tsx`; `PluginMarketplaceView.tsx` |
| Workflow / Create Loop top-level navigation | No longer present in `AppRoute`, Sidebar, or Workbench stage after P0 | no | no | yes | no for 0.2.13/0.2.14 baseline without spec approval | removed/hidden; bottom code retained as unexposed future capability | `src/renderer/src/store/chat-store-types.ts`; `src/renderer/src/components/chat/Sidebar.tsx`; `src/renderer/src/components/Workbench.tsx`; `src/renderer/src/workflow/create-loop-runtime.ts` |

Absorbed:

| Upstream item | analytix implementation | Proof |
| --- | --- | --- |
| Kun `d09d52b0` Windows installer process-stop | Added `build/installer.nsh` with `customCheckAppRunning` that finds and stops processes under `$INSTDIR` before upgrade using `ANALYTIX_INSTALLER_APP_ROOT`, `ANALYTIX_INSTALLER_SELF_PID`, and the Windows process `ExecutablePath`/`Path` fallback; wired it through `electron-builder.config.cjs` `nsis.include`. | Static source/config scan; Windows NSIS machine QA remains required before release readiness. |
| Product-position correction | Removed `workflow` from `AppRoute`, `openWorkflow`, Sidebar Workflow command, and Workbench workflow stage. | `Sidebar.test.ts`, `Workbench.route-surface.test.ts`, and `chat-store-app-actions.test.ts` guard the route surface. |

Classified but not implemented:

| Kun develop item | Classification | Next action |
| --- | --- | --- |
| `867b975a` through `d1e6df9d` local Whisper/speech runner work | `should absorb` only after packaging, binary provenance, settings contract, and platform QA review. | Next speech batch; do not mix into installer. |
| `afafa226` SSE IPC throttle/reconnect | `must/should assess`; possible release blocker if current analytix has the same reconnect/throttle failure. | Next runtime-SSE batch; record as candidate blocker. |
| `08b5b1e`, `54b6c93`, `bea44ec`, `b6a0092` UI drift | `should assess` as desktop polish, not a P0 release-safety fix. | Future UI regression/polish batch. |

Rules added:

```text
Future Kun product absorption must preserve the target Kun version's product
position, entry layer, and trigger path when claiming Kun parity. Moving a
later/current Kun feature, or any source-derived product idea, to a higher
analytix entry level requires an independent spec decision and tests that prove
the new placement is deliberate and safe.
```

Remaining risks:

```text
Windows NSIS process-stop behavior still requires a Windows machine upgrade
verification before release readiness. This P0 correction does not claim full
Workflow builder parity, release readiness, Go G1, Reasonix approvalManager
absorption, or Kun develop speech/SSE/UI absorption.
```

## 2026-06-20 - P2.2 Reasonix cache/tool lifecycle conflict check

Source:

```text
Kun version/tag: master snapshot
Remote HEAD: master 8602476c5c449b4561473ad5f31081ee93dc782e
Develop HEAD: ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Release notes: not used; remote HEAD and local source were checked only for P2.2 conflicts
Local comparison branch: /tmp/analytix-upstreams/Kun using origin/master and origin/develop
```

Summary:

```text
P2.2 is a Reasonix-led provider/cache/tool lifecycle batch. Kun was not a code
source in this slice because the relevant P1.1 provider routing, P1.2 Connect
Phone, and P1.3 Create Loop slices are already absorbed through analytix
contracts. The Kun check exists to prevent regression: no P2.2 change may
reintroduce `kun serve`, `window.kunGui`, old settings envelopes, or workflow
builder scope creep.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Kun provider routing from P1.1 | provider/runtime | `already covered` | Keep existing `thread.providerId` and `MultiProviderModelClient` paths; P2.2 diagnostics must remain additive. | P1.1 regression tests stay in closure suite. |
| Kun Connect Phone / Telegram from P1.2 | remote workflow | `already covered` | Do not alter Connect Phone runtime relay or settings while adding cache diagnostics. | P1.2 regression tests stay in closure suite. |
| Kun Workflow / Create Loop from P1.3 | product workflow | `already covered` | Do not broaden P2.2 into workflow builder, hooks, local webhooks, or MCP workflow exposure. | P1.3 focused tests stay in closure suite. |
| Kun public identity / old bridge / old settings / `kun serve` | identity/protocol | `reject` | Continue rejecting in production source. | Identity/protocol scan. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Kun product workflow priorities vs Reasonix engine diagnostics | P2.2 remains Reasonix-led because the implemented value is runtime cache/tool/provider diagnostics, not product workflow. | spec 08 |
| Kun workflow builder expansion during P2.2 | Defer; scoped Create Loop is already closed, full builder needs a separate analytix-native design. | P1.3 ledger |

Absorbed:

```text
No Kun code was absorbed in P2.2.
```

Skipped:

```text
Kun workflow builder, Connect Phone changes, checkpoint refs, tray/worktree
polish, and old identity surfaces were not part of this Reasonix-led batch.
```

Validation:

```text
Remote HEADs were checked with git ls-remote. Closure validation includes P1.1,
P1.2, and P1.3 regression commands plus production identity/protocol scans.
```

Remaining risks:

```text
Kun remains relevant for future P3 safety/checkpoint/durable workflow and
desktop utility work. This P2.2 check does not close those future surfaces.
```

## 2026-06-20 - P3A Kun baseline fidelity and 8602476 delta audit

Source:

```text
Kun baseline lineage: analytix originated from Kun 0.2.13
Kun master HEAD: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop HEAD: ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Remote check: git ls-remote on 2026-06-20
Local comparison refs: refs/remotes/legacy-origin/master and develop
```

Summary:

```text
P3A treats Kun as the product-lineage baseline.
The audit checks that analytix still preserves the Kun 0.2.13 product baseline
and that the already-landed P1.1/P1.2/P1.3 scoped slices cover the highest-risk
0.2.14 / 8602476 deltas without reopening full Workflow builder or checkpoint
implementation in this batch.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Code thread, HTTP/SSE, approvals, request_user_input, interrupt, thread search/archive/fork/resume, usage/cache | Code/runtime baseline | `already preserved` | Keep existing analytix runtime contract and P1.1/P2.2 regression tests as the proof path. | Closure suite remains required. |
| Write workspace, inline completion, selected text actions, export/RAG/media paths | Write baseline | `baseline fidelity audit` | No P3A code change; preserve existing Write route and settings while tool/MCP work stays runtime-scoped. | Future Write benchmark before release/G0. |
| SDD requirement/design/plan/review flows | SDD baseline | `baseline fidelity audit` | No P3A code change; ensure P3A does not alter renderer workflow contracts. | Future SDD benchmark before release/G0. |
| Connect Phone / Telegram runtime from P1.2 | Remote workflow | `already preserved` | P3A must not alter Connect Phone settings, attachment relay, provider/model propagation, or GUI-to-Telegram mirror paths. | P1.2 regressions in closure suite. |
| Schedule task thread reuse and provider labels | Schedule | `already preserved` | P3A remains runtime tool safety; no schedule schema change. | P1.2 regression coverage. |
| Provider presets and per-provider routing from P1.1 | Provider/runtime | `already preserved` | Keep `thread.providerId` routing and GLM/Xiaomi fixes. | P1.1 runtime/provider regressions. |
| MCP/Skills/tool catalog | Tool/runtime | `0.2.14+ delta` | P3A strengthens the MCP malformed schema boundary through analytix runtime tests rather than porting Kun UI or old bridge names. | Future full MCP lifecycle/cancel/protected path fixtures. |
| Multimodal/media/file/image propagation | Media/runtime | `baseline fidelity audit` | Existing attachment and tool-result image tests remain the proof path; P3A does not change media contracts. | File/image result propagation fixture can expand in P3B. |
| Workflow / Create Loop | Product workflow | `already preserved for scoped slice` / `defer` | P1.3 closes only the scoped Create Loop surface. Full visual builder, typed variables, hooks, custom modules, durable workflow storage, and workflow MCP exposure remain deferred. | Dedicated workflow builder slices after UX/contract redesign. |
| Git checkpoint / rollback from Kun `8cbf8e6` | Safety | `defer` | Record as product safety input, but reject `refs/kun/checkpoints/*` identity. P3A only documents checkpoint boundary and leaves implementation to P4. | P4 checkpoint/rewind design and tests. |
| Tray session/worktree/composer/sidebar desktop utility deltas | Desktop utility | `defer pending desktop QA` | Keep as future analytix UI polish; do not mix into P3A tool safety slice. | Desktop utility slice with Electron QA. |
| `window.kunGui`, `agents.kun`, `kun serve`, Kun identity, old UI shell | Identity/protocol | `reject` | No regression allowed. | Production identity/protocol scan. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Kun checkpoint refs vs analytix identity | Future checkpoint work may reuse the safety idea, but storage/refs/UI must be analytix-named and contract-owned. | D-0006 |
| Kun product workflow breadth vs P3A Reasonix-led engine slice | P3A remains a tool/MCP/sandbox/checkpoint boundary slice; Kun surfaces are regression inputs, not scope expansion. | D-0002 / D-0003 |

Absorbed:

```text
No new Kun code was absorbed in P3A. Existing P1.1/P1.2/P1.3 scoped slices are
treated as preserved baseline/delta closure and are re-validated.
```

Skipped:

```text
Full Workflow builder, durable workflow storage, Kun git checkpoint service,
tray/session utility work, and any old identity/bridge/settings/CLI surface.
```

Validation:

```text
P3A focused tests plus P1.1/P1.2/P1.3 regression commands, typechecks,
git diff --check HEAD, and production identity/protocol scan.
```

Remaining risks:

```text
P3A improves proof around tool/MCP safety boundaries. It does not close full
Kun master parity, full Workflow builder parity, live Connect Phone QA, or
checkpoint/rewind product implementation.
```

## 2026-06-20 - P4A checkpoint/rewind safety oracle

Source:

```text
Kun version/tag: master snapshot
Commit range: 8f2040349fba47fcd8e8b94f50b131943af839b2..8602476c5c449b4561473ad5f31081ee93dc782e
Remote HEAD: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Local comparison branch: /tmp/analytix-upstreams/Kun
```

Summary:

```text
P4A treats Kun checkpoint/rollback work as 0.2.13 基线保真、0.2.14/8602476
差异补齐、升级过程中可能打坏能力的回归修复. The useful product-safety value is
that a turn can have an auditable, user-understandable safety point before file
edits are trusted. Analytix converts that value into analytix-owned checkpoint
contracts and tests instead of copying Kun identity, refs, bridge, settings, or
UI.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| 0.2.13 baseline file-change/review expectations | baseline fidelity | `baseline regression` | Preserve current analytix file-change tool results, generated-files, review, and history surfaces as checkpoint landing points. | P1.1/P1.2/P1.3/P3A regressions stay in the closure suite. |
| Kun `8cbf8e6` git turn rollback checkpoints | safety / rollback | `0.2.14/8602476 delta closure` | Accept the product safety requirement: checkpoint metadata must name workspace, turn, changed files, timestamp, status, and a safe restore boundary. Implemented P4A as analytix `axcp_` metadata and `checkpoint_captured` oracle only. | P4B may add file restore after review UI and crash-recovery design. |
| Post-checkpoint commits, staged/unstaged/untracked restore tests | safety / git | `defer pending evidence` | Valuable coverage, but P4A does not mutate git or user files. If git is used later, refs must be analytix-owned and restore must be non-silent and tested. | P4B combined code+conversation rewind. |
| Kun `refs/kun/checkpoints`, `window.kunGui`, old settings, old UI shell | identity/protocol/UI | `reject` | These are Kun identity or product shell details and cannot be reused. | Production identity/protocol scan remains mandatory. |

Absorbed:

```text
No Kun code or refs were copied. Analytix absorbed the product-safety contract
pressure by adding versioned analytix checkpoint metadata, changed-file
normalization, and event replay fixtures in the runtime.
```

Skipped:

```text
Kun git ref storage, direct rollback execution, rescue checkpoints, renderer
rollback controls, tray/worktree UI, and any Kun bridge/settings/identity.
```

Validation:

```text
P4A focused test: packages/runtime/tests/checkpoint-rewind-oracle.test.ts.
Full closure also re-runs P3A, P2.2, P1.3, P1.2, P1.1 regressions, typechecks,
git diff --check HEAD, and production identity/protocol scan.
```

Remaining risks:

```text
P4A proves contract and oracle behavior only. It does not close full Kun git
rollback parity, restore staged/untracked files, desktop rollback UI, or release
QA.
```

## 2026-06-20 - P4B full rewind/review UI plan-only slice

Source:

```text
Kun version/tag: master snapshot
Commit range: 8f2040349fba47fcd8e8b94f50b131943af839b2..8602476c5c449b4561473ad5f31081ee93dc782e
Remote HEAD recheck: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Local branch: codex/p4b-full-rewind-review-ui
```

Summary:

```text
P4B keeps Kun checkpoint/rollback work as product-safety evidence, not as code
or identity. The accepted slice is an analytix-owned auditable restore/rewind
plan: `axrp_` plan ids, code-only / conversation-only / combined scopes, path
risk statuses, runtime route, IPC allow-list, renderer provider method, and
existing ChangeInspector / turn-change review display. No Kun git refs,
rollback commands, old bridge, settings, or shell UI were adopted.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| 0.2.13 baseline file-change / generated-files / review surfaces | baseline fidelity | `baseline regression` | Preserved existing `file_change`, generated-files, review card, and ChangeInspector ownership; P4B adds only `meta.rewindPlan` display and no new shell. | Keep P1.1/P1.2/P1.3/P3A regressions in closure suite. |
| Kun git turn rollback checkpoints | safety / rollback | `0.2.14/8602476 delta partial closure` | Absorb the rollback safety requirement as an auditable `plan_only` runtime contract. Do not apply files or rewrite transcript in P4B. | P4C destructive restore, staged/untracked coverage, desktop QA. |
| Kun git refs and direct reset/checkout behavior | git / identity | `defer pending evidence` / `reject identity` | No `refs/kun/checkpoints`, no `git reset --hard`, no silent restore. Future storage may be hybrid only behind analytix-owned refs/snapshots and review confirmation. | P4C design must prove non-silent apply and crash recovery. |
| Kun UI/bridge/settings identity | identity / UI | `reject` | No `window.kunGui`, old settings fallback, Kun route shell, or Kun renderer protocol. | Production identity/protocol scan remains mandatory. |

Absorbed:

```text
Analytix absorbed the safety contract and UI boundary: shared endpoint,
runtime route/service, restore-plan schema, main IPC allow-list, renderer
provider method, and existing review/history/ChangeInspector display for
plan-only rewind evidence.
```

Skipped:

```text
Kun git checkpoint refs, destructive rollback apply, staged/untracked restore,
rescue checkpoints, tray/worktree rollback UI, and any Kun identity or bridge
surface. These remain P4C or later only after analytix-owned apply design.
```

Validation:

```text
Focused P4B tests cover code-only, conversation-only, combined, path escape,
absolute path, symlink risk, and no raw prompt/full-content leakage. Full
closure re-runs P4A, P3A, P2.2, P1.3, P1.2, P1.1 regressions, typechecks,
git diff --check HEAD, and production identity/protocol scan.
```

Remaining risks:

```text
P4B does not prove destructive restore, staged/untracked git parity, crash-safe
apply/resume, packaged desktop QA, or release/G0 readiness. Those move to P4C,
while Go checkpoint parity remains a G5 oracle requirement.
```

## 2026-06-20 - P4C confirmed rewind restore apply

Source:

```text
Kun version/tag: master 8602476c5c449b4561473ad5f31081ee93dc782e; develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928
Commit range: no new Kun fetch in this code slice; P4C builds on the P4B upstream checkpoint and local baseline.
Release notes: not used.
Local comparison branch: codex/p4c-confirmed-rewind-restore-apply
```

Summary:

```text
P4C treats Kun as product-lineage and safety-regression input. The work checks
that analytix did not break the Kun 0.2.13 baseline review/generated-files/file
change surfaces, closes more of the 0.2.14/8602476 rollback safety delta, and
continues to reject Kun public identity, git refs, bridge names, and shell UI.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| 0.2.13 baseline file-change / generated-files / review surfaces | baseline fidelity | `baseline regression` | Preserved existing `file_change`, TurnChangeSummary, ChangeInspector, and review-surface ownership; P4C adds confirmation controls inside those existing surfaces rather than adding a Kun shell. | Desktop QA must prove the controls are reachable and safe. |
| Kun rollback safety requirement | safety / rollback | `0.2.14/8602476 delta closure for scoped apply` | Implemented confirmed destructive apply from P4B plans only, with `axra_` apply ids, `axrr_` rescue records, snapshot content/hash checks, parent/final symlink blocking, staged/untracked blocking, lock-time revalidation, partial-failure audit, and append-only audit. | Automatic production snapshot capture/storage remains future work. |
| Kun git refs and direct reset/checkout behavior | git / identity | `reject` | No `refs/kun/checkpoints`, no `git reset --hard`, no `git checkout` restore, and no silent overwrite path. | Any future git-backed storage must be analytix-owned and re-proven. |
| Kun UI/bridge/settings identity | identity / UI | `reject` | No `window.kunGui`, `agents.kun`, `kun serve`, Kun route shell, or Kun renderer protocol. | Production identity/protocol scan remains mandatory. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Direct Kun rollback vs confirmed analytix apply | Destructive apply must be plan-based, confirmed, rescue-backed, and append-only audited. | D-0009 |
| Kun checkpoint refs vs analytix identity | Reject Kun refs and old bridge/settings/CLI names. | D-0006 / D-0009 |

Absorbed:

```text
analytix-owned apply contract, runtime service/route, shared endpoint, IPC
allow-list, renderer provider method, and existing review/ChangeInspector /
TurnChangeSummary confirmation controls for P4B rewind plans.
```

Skipped:

```text
Kun git checkpoint refs, direct git reset/checkout, tray/worktree rollback UI,
old bridge/settings/runtime identity, and automatic file snapshot capture.
Snapshot-backed modified/deleted restore is supported by the apply contract,
but missing snapshot/hash evidence is intentionally blocked.
```

Validation:

```text
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-apply.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-plan.test.ts --no-file-parallelism --maxWorkers=1
npm --prefix packages/runtime run test -- tests/checkpoint-rewind-oracle.test.ts --no-file-parallelism --maxWorkers=1
P3A/P2.2/P1.3/P1.2/P1.1 regression commands re-run in the P4C closure pass.
npm --prefix packages/runtime run typecheck
npm run typecheck
docs/analytix/qa/p4c-desktop-qa-2026-06-20.md records real Electron smoke.
```

Remaining risks:

```text
P4C code behavior and desktop smoke are covered, but final release closure
still depends on a live desktop apply fixture, crash/restart apply QA, packaged QA,
production identity/protocol scan, `git diff --check`, Spec 07 release/push
blockers, and an explicit decision on whether automatic snapshot capture belongs
before G0.
```

## 2026-06-20 - G0/G5 conformance inventory runtime baseline

Source:

```text
Kun 0.2.13: product-lineage baseline for current analytix preservation checks.
Kun current/stable: master 8602476c5c449b4561473ad5f31081ee93dc782e; develop ab24a77f0f68fcb1b2c160361e4c50cd92b02928 remains merged into master.
Local comparison branch: codex/p4c-confirmed-rewind-restore-apply at b3e1674 before the inventory branch.
```

Summary:

```text
This batch does not absorb new Kun code. It freezes the analytix TypeScript
runtime contracts that must stay compatible with the product-lineage behavior
already absorbed from Kun 0.2.13 and Kun 0.2.14/current before any Go runtime
work begins.
```

Capability matrix:

| Source | Absorb what | Why better | How to prove better | Conflict authority |
| --- | --- | --- | --- | --- |
| Kun 0.2.13 baseline | Preserve Code/Write/SDD/Connect Phone/schedule expectations, generated-file and review surfaces, approvals, provider presets, and local HTTP/SSE workflow continuity. | Analytix migration remains product-compatible while using current identity, settings, bridge, and runtime contracts. | Existing P1.1/P1.2/P1.3/P3A/P4A/P4B/P4C regressions, renderer/main focused tests, and desktop smoke where recorded. | Specs 01-09, D-0001, D-0002. |
| Kun 0.2.14/current `8602476` | Keep scoped closures for provider/runtime routing, Telegram Connect Phone, hidden Create Loop internals, checkpoint/rewind safety, and desktop utility lessons. | Analytix keeps pace with Kun stable drift without reintroducing Kun public identity, UI shell, or unapproved top-level Workflow navigation. | Kun ledger, absorption ledger, scorecard P1.1-P4C entries, full runtime suite, typechecks, route-surface tests, and identity/protocol scan. | D-0006, D-0008, D-0009, D-0012; reject `kun serve`, `window.kunGui`, `agents.kun`, Kun refs, old settings, and unapproved Workflow entry placement. |

G0/G5 impact:

| Contract surface | Kun reason to preserve it | TypeScript oracle / future gate |
| --- | --- | --- |
| Thread/session and SSE replay | Kun product workflows depend on stable desktop thread history and reuse. | `thread-service.test.ts`, `hybrid-store.test.ts`, `file-session-store.test.ts`, runtime/renderer projection tests. |
| Tool calls, approvals, user input | Kun Code/Write/Connect Phone flows depend on reviewable tools and safe human gates. | `loop.test.ts`, `builtin-tools.test.ts`, `user-input-disabled.test.ts`, focused renderer/main tests. |
| Provider request/stream parsing and usage | Kun provider presets and routing deltas are only better if request shape and cost accounting remain correct. | `model-client.test.ts`, provider focused tests, usage/cache tests. |
| Checkpoint/rewind plan/apply | Kun rollback value is absorbed only as analytix-owned review and confirmed apply semantics. | `checkpoint-rewind-oracle.test.ts`, `checkpoint-rewind-plan.test.ts`, `checkpoint-rewind-apply.test.ts`; live apply/crash QA remains release evidence. |

Skipped:

```text
No Go scaffold, Rust helper, new Kun import, Kun UI shell, Kun bridge, Kun
settings envelope, direct git reset/checkout, or Kun refs were added.
```

Validation:

```text
npm --prefix packages/runtime run test -- --no-file-parallelism --maxWorkers=1
```

The old `loop.test.ts` prompt-token fixtures conflicted with the current
anti-inflation compaction contract; the toolKind test conflicted with the
post-file-change final-answer contract. The tests were updated to match those
contracts rather than weakening runtime behavior.

Remaining risks:

```text
This historical batch used a scoped product-superiority label for previously
absorbed slices; it is not a current `CapabilityBenchmarkV1` result. Full
Workflow builder parity, live Connect Phone QA, packaged release QA, live
rewind apply/crash recovery, verified analytix remote, signing/notarization,
and Windows NSIS machine QA remain outside this G0/G5 inventory batch.
```

## 2026-06-20 - Kun Develop Windows Installer Currentness Refresh

Source:

```text
Kun master: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop: 9605e20f422c90054d930e4f1a0000886b353895
Kun v0.2.13 tag: 201a1469ffbd911f6b95d450b78471e51b42acae
Kun v0.2.14 tag: 06be05d76223208724c07301fa0f830641a06e6f
Commit range: ab24a77f0f68fcb1b2c160361e4c50cd92b02928..9605e20f422c90054d930e4f1a0000886b353895
Local comparison branch: /tmp/analytix-upstream-audit-kun-20260620 origin/develop
```

Commit log:

```text
d09d52b fix(installer): stop bundled processes before windows upgrade
```

Diff stat:

```text
build/installer.nsh         | 36 ++++++++++++++++++++++++++++++++++++
electron-builder.config.cjs |  1 +
2 files changed, 37 insertions(+)
```

Summary:

```text
Kun develop advanced past stable master with a Windows installer fix. The NSIS
custom macro stops processes running from the installation directory before an
upgrade retries, reducing Windows overwrite failures. This is release-safety
work, not a product workflow or runtime contract change.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `d09d52b` installer process stop | Windows NSIS / release upgrade safety | `must absorb` | Historical refresh decision superseded by the later P0 product-entry/installer section above: the behavior is now absorbed as `build/installer.nsh` and `electron-builder.config.cjs` `nsis.include` using analytix-owned names. | Windows NSIS machine upgrade verification remains required before release readiness. |

Conflicts:

| Conflict | Ruling | Decision id |
| --- | --- | --- |
| Kun NSIS variable/env names vs analytix release identity | Keep the behavior, rename implementation to analytix-owned installer naming. | D-0002 / Spec 02 |
| Kun develop drift vs stable master baseline | Do not reopen all Kun develop work; stable `8602476` remains the main product baseline, with `d09d52b` tracked as a narrow release fix. | Spec 08 section 21 |

Absorbed:

```text
No code was absorbed in this historical currentness patch. The later P0
product-entry/installer section records the actual analytix-native absorption.
```

Skipped:

```text
Kun product identity, `KUN_*` env names, `Kun-*` artifacts, full develop UI
diff, and any old bridge/settings naming.
```

Validation:

```text
git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14
git log --oneline ab24a77f0f68fcb1b2c160361e4c50cd92b02928..origin/develop
git diff --stat ab24a77f0f68fcb1b2c160361e4c50cd92b02928..origin/develop
git show --stat --patch d09d52b
```

Remaining risks:

```text
Windows NSIS still requires a Windows machine verification run. Release URL,
repository metadata, signing/notarization, packaged launch QA, and verified
analytix remote remain release blockers.
```

## 2026-06-20 - Kun Installer Follow-Up Recheck During Reasonix Control-Port Batch

Source:

```text
Requested stable baseline: master 8602476c5c449b4561473ad5f31081ee93dc782e
Requested develop checkpoint: 9605e20f422c90054d930e4f1a0000886b353895
Current remote recheck: master 8602476c5c449b4561473ad5f31081ee93dc782e, develop 247076f297170c3d0c558baffb073c629894faea
Installer target still present: d09d52b fix(installer): stop bundled processes before windows upgrade
Local comparison branch: /tmp/analytix-upstreams/Kun origin/develop
```

Summary:

```text
The Reasonix control-port implementation batch did not touch Kun product or
installer code. Kun master still matches the requested stable baseline. Kun
develop has moved past the requested 9605e20f checkpoint. This historical
control-port recheck kept d09d52b out of that runtime batch; the later P0
product-entry/installer section above records the analytix-native absorption.
The new develop speech/UI/SSE drift is not part of the control-port or
installer patch.
```

Classification:

| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| `d09d52b` installer process stop | Windows NSIS / release upgrade safety | `must absorb` | Historical control-port-batch deferral superseded by the later P0 product-entry/installer section above. It is now implemented as analytix-native NSIS/include logic. | Verify on Windows. |
| `9605e20f..247076f` develop drift | speech/UI/SSE/develop currentness | `record` | Currentness only; no code absorption in this batch. | Classify separately if a future product or release batch targets these changes. |

Absorbed:

```text
No Kun code was absorbed in the Reasonix control-port batch. The later P0
product-entry/installer batch absorbed only the installer process-stop
behavior and corrected the Workflow entry exposure.
```

Skipped:

```text
During the Reasonix control-port batch, Kun installer logic, Kun product
identity, `KUN_*`, `Kun-*` artifacts, old bridge/settings/runtime naming, and
unrelated develop speech/UI/SSE drift were skipped. The installer behavior was
later absorbed without Kun identity leakage.
```

Validation:

```text
git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop
git -C /tmp/analytix-upstreams/Kun fetch origin develop:refs/remotes/origin/develop --prune
git -C /tmp/analytix-upstreams/Kun show --stat --oneline d09d52b
git -C /tmp/analytix-upstreams/Kun log --oneline --max-count=12 9605e20f422c90054d930e4f1a0000886b353895..origin/develop
```

Remaining risks:

```text
The Kun Windows installer must-absorb item is no longer open as a code/doc
slice after the later P0 product-entry/installer batch. Windows NSIS machine
verification remains required before release readiness can claim the upgrade
fix.
```

## 2026-06-21 - Kun Product-Lineage Absorption Methodology

Source:

```text
Kun v0.2.13: 201a1469ffbd911f6b95d450b78471e51b42acae
Kun v0.2.14: 06be05d76223208724c07301fa0f830641a06e6f
Kun master: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop: 247076f297170c3d0c558baffb073c629894faea
```

Summary:

```text
Analytix started from the Kun 0.2.13 lineage and first absorbs/preserves Kun
0.2.14. Later Kun master/current changes remain valuable, but they must be
classified by target version, product position, entry layer, and trigger path
before exposure in analytix.
```

Permanent rule:

| Area | Decision |
| --- | --- |
| Target version | Each Kun capability must state whether it belongs to 0.2.13, 0.2.14, master/current, or develop-only drift. |
| Entry layer | Analytix must preserve the target Kun entry layer when claiming Kun parity. A separate analytix spec may approve a different placement for an analytix-native capability. |
| Workflow/Create Loop | Current/master Workflow value is retained as future capability input, not as 0.2.13/0.2.14 top-level navigation parity. |
| Product proof | Product-entry parity is benchmark evidence; wrong entry level is a product regression even if code works. |
| Reasonix interaction | Reasonix engine improvements must not create Kun product entry changes. |

Validation rule:

```text
Future Kun absorption batches must include a product surface table covering
Sidebar, Workbench route, AppRoute/store action, mode tabs, settings sections,
visible commands, Connect Phone, Schedule, Write, SDD, and any plugin/tool
entry that becomes user-visible.

Current P0 guard evidence is `src/renderer/src/components/chat/Sidebar.test.ts`,
`src/renderer/src/components/Workbench.route-surface.test.ts`, and
`src/renderer/src/store/chat-store-app-actions.test.ts`. The Kun
`d09d52b0` installer process-stop behavior is implemented in
`build/installer.nsh` and `electron-builder.config.cjs`, but Windows NSIS
machine verification remains a release blocker.
```

## 2026-06-21 - Kun Baseline Recheck During Reasonix Latest / Go G2 Draft

Source:

```text
Kun master: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop: 247076f297170c3d0c558baffb073c629894faea
Kun v0.2.13: 201a1469ffbd911f6b95d450b78471e51b42acae
Kun v0.2.14: 06be05d76223208724c07301fa0f830641a06e6f
Reasonix currentness context: main-v2 91fe06db6177bb052fc8ae1a3081d60bfc104a4e
```

Summary:

```text
This is a Kun baseline preservation check attached to the Reasonix latest
currentness and Go G2 shadow-route planning batch. Kun refs did not move from
the prior P0/methodology baseline. Therefore this batch does not reopen Kun UI
or product-surface absorption.
```

Product-entry decision:

| Surface | Kun 0.2.13 | Kun 0.2.14 | analytix current rule | Result |
| --- | --- | --- | --- | --- |
| Top-level Workflow / Create Loop route | absent | absent | Keep hidden unless a future analytix workflow automation spec approves a new entry layer. | preserved |
| Create Loop internals | absent as top-level product entry | absent as top-level product entry | May remain isolated as unexposed future capability. | preserved |
| Go G2 route replay | not a Kun product entry | not a Kun product entry | Backend conformance work must not create navigation or route-surface changes. | preserved |
| Reasonix provider/MCP hardening | not a Kun product entry | not a Kun product entry | May inform runtime/provider parity only behind analytix contracts. | no Kun UI change |

Validation:

```text
git ls-remote https://github.com/KunAgent/Kun.git refs/heads/master refs/heads/develop refs/tags/v0.2.13 refs/tags/v0.2.14
npm run test -- src/renderer/src/components/chat/Sidebar.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/store/chat-store-app-actions.test.ts -- --runInBand
rg -n "\bworkflow\b|openWorkflow" src/renderer/src/components/chat/Sidebar.tsx src/renderer/src/components/Workbench.tsx src/renderer/src/store/chat-store-types.ts src/renderer/src/store/chat-store-app-actions.ts
```

Result:

```text
Kun refs matched the previous baseline. Route-surface guard tests passed
3 files / 9 tests. The production route-surface scan had no hits for
top-level Workflow / openWorkflow in Sidebar, Workbench, AppRoute/store action
files.
```

Remaining risks:

```text
Windows NSIS machine upgrade QA, live Connect Phone QA, packaged app launch
matrix, release metadata, signing/notarization, verified analytix remote, and
future Workflow automation productization remain open release/product gates.
```

## 2026-06-21 - Kun Desktop Hardening During Reasonix P0 Closure

Source:

```text
Kun master: 8602476c5c449b4561473ad5f31081ee93dc782e
Kun develop: 247076f297170c3d0c558baffb073c629894faea
Kun v0.2.13: 201a1469ffbd911f6b95d450b78471e51b42acae
Kun v0.2.14: 06be05d76223208724c07301fa0f830641a06e6f
Relevant develop commits reviewed:
  afafa22 SSE IPC reconnect/safe-send lineage
  867b975/7043a16/defa304/ca645a0/d1e6df9 Local Whisper lineage
  ec42254 dynamic tray session menu
  08b5b1e/54b6c93 composer/dropdown/jump rail fixes
```

Summary:

```text
This stage absorbs only desktop hardening that is safe behind existing
analytix UI/runtime contracts. The Kun 0.2.13 -> 0.2.14 product-entry baseline
remains authoritative.
```

Classification:

| Item | Class | analytix decision | Follow-up |
| --- | --- | --- | --- |
| Runtime SSE IPC reconnect/throttle | `must absorb` | Implemented as analytix main-process 100ms pending-event throttle, safe send when renderer is destroyed, and reconnect with flushed `since_seq`. | Packaged long-thread desktop QA remains required. |
| Workbench shell navigation hardening | `should absorb` | Consolidated sidebar/back/forward/new-chat controls at the shell level and removed duplicated Write/Connect Phone/Plugin local sidebar toggles. | Full desktop screenshot/interaction QA remains release evidence. |
| Local Whisper | `defer` | Not imported in this stage to avoid resource/package/signing risk without a dedicated spec/oracle. | Future speech package-size and permission gate. |
| Dynamic tray session menu | `defer` | Not imported in this stage; tray behavior changes need desktop QA and platform-specific evidence. | Future tray QA batch. |

Validation:

```text
Focused tests passed for runtime SSE IPC, shell navigation controls,
Write toolbar, Connect Phone, and Workbench route-surface guards.
```

Remaining risks:

```text
Local Whisper, tray session menu, packaged app launch, Windows NSIS, signing,
and live Connect Phone QA remain open.
```

## YYYY-MM-DD - Template

Source:

```text
Kun version/tag:
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
<Commands, tests, GUI QA, screenshots, or blockers>
```

Remaining risks:

```text
<Known gaps and next actions>
```

## 2026-06-21 - Kun Baseline During Reasonix 9e56 Runtime Closure

Source:

```text
Kun product baseline: v0.2.13 -> v0.2.14
Current checked refs remain:
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
  v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
Reasonix runtime context: 9e56c3276880ced538b3375329b8a0ebbe63a67b
```

Summary:

```text
Kun did not drive a new product-entry change in this stage. The dirty shell
baseline was reviewed as native-titlebar safe-area / shell navigation closure
and committed separately. Runtime work stayed behind analytix contracts.
```

Classification:

| Item | Class | analytix decision | Follow-up |
| --- | --- | --- | --- |
| Native titlebar safe-area / shell navigation closure | `should absorb` | Reviewed the dirty baseline, ran focused renderer/shared tests, and committed `fix(shell): keep navigation clear of native titlebar controls`. | Packaged desktop screenshot/interaction QA remains release evidence. |
| Composer/dropdown/jump rail lineage | `record / already guarded` | Existing shell/Workbench route-surface tests guard the current entry baseline; no extra Kun UI code was imported in this runtime stage. | Revisit only with a focused desktop UX batch. |
| Tray session menu | `defer` | Not imported because it needs platform-specific desktop QA and release evidence. | Future tray QA batch. |
| Local Whisper | `defer` | Still only a future spec/oracle/import-boundary candidate; no bundled resource or signing/package risk was added. | Future speech resource and permission gate. |

Product-entry result:

```text
Code / Write / Settings / Plugins / Connect Phone / Schedule remain the
preserved entry baseline. Top-level Workflow/Create Loop remains absent.
Create Loop internals, if present, remain hidden future capability only.
```

Validation:

```text
npm run test -- src/shared/window-chrome.test.ts src/renderer/src/AppShell.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/components/WindowsTitleBar.test.ts
git diff --check
```

Remaining risks:

```text
Packaged app launch, tray QA, Local Whisper package/signing risk, live Connect
Phone QA, Windows NSIS, signing/notarization, and release metadata remain open.
```

## 2026-06-21 - Kun Product Baseline During Collaborative Runtime Closure

Source:

```text
Kun product baseline:
  v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
Current refs:
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
Reasonix runtime context:
  9e56c3276880ced538b3375329b8a0ebbe63a67b
```

Summary:

```text
Kun contributed only desktop boundary classification in this runtime-heavy
stage. Dirty shell/native titlebar safe-area work was reviewed, verified, and
committed separately before Reasonix-oriented runtime changes continued.
```

Classification:

| Item | Class | analytix decision | Follow-up |
| --- | --- | --- | --- |
| Native titlebar / shell safe-area | `should absorb` | Closed in code and tests before runtime work continued; Workbench shell keeps root no-drag, stage drag, safe traffic-light inset, and interactive navigation buttons. | Packaged screenshot/click QA remains release evidence. |
| Composer/dropdown/jump rail polish | `record / already guarded` | Current composer and route-surface tests continue to guard the preserved product-entry baseline; no Kun UI expansion in this stage. | Future focused desktop UX batch only. |
| Tray session menu | `defer pending spec/QA` | Not imported because it needs a main/renderer session-selection contract, platform tray QA, and release evidence. | Future tray session-menu spec/oracle. |
| Local Whisper | `defer pending import boundary` | Not imported; no bundled speech resource, entitlement/signing change, or package-size risk was added. | Future Local Whisper spec/oracle/import boundary. |

Product-entry result:

```text
Code / Write / Settings / Plugins / Connect Phone / Schedule remain the
v0.2.13 -> v0.2.14 baseline. Top-level Workflow/Create Loop remains absent.
```

Validation:

```text
npm run test -- src/shared/window-chrome.test.ts src/main/window-chrome-config.test.ts src/renderer/src/components/Workbench.route-surface.test.ts
git diff --check
```

Remaining risks:

```text
Packaged desktop QA, tray session menu, Local Whisper, Connect Phone live QA,
Windows NSIS, signing/notarization, and release metadata remain open.
```

## 2026-06-21 - Kun Tray Session Menu Absorption

Source:

```text
Kun product baseline:
  v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
Current refs:
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
```

Summary:

```text
The tray session menu candidate is now absorbed as an analytix-native main
process feature. It reads the bundled runtime thread list, groups running and
recent sessions, and opens selected sessions through existing window creation
logic. It does not add a renderer route, bridge alias, or Kun identity.
```

Classification:

| Item | Class | analytix decision | Follow-up |
| --- | --- | --- | --- |
| Tray session menu | `absorbed behind analytix contracts` | Added `src/main/tray-session-menu.ts` with deterministic grouping, archived/deleted filtering, workspace basename labels, overflow handling, and main-process thread opening via existing `openThreadInNewWindow`. | Packaged tray interaction QA remains release evidence. |
| Local Whisper | `defer pending import boundary` | Still not imported; no bundled speech resource, permission, entitlement, package-size, or signing risk was added. | Future Local Whisper spec/oracle/import-boundary and platform QA. |
| Composer/dropdown/jump rail polish | `record / already guarded` | No new Kun UI code was imported in this stage; current Workbench route-surface and shell tests continue to guard product-entry baseline. | Future focused desktop UX batch only. |

Product-entry result:

```text
Code / Write / Settings / Plugins / Connect Phone / Schedule remain the
v0.2.13 -> v0.2.14 baseline. The tray menu is a desktop convenience surface,
not a top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer route.
```

Validation:

```text
npm run test -- src/main/tray-session-menu.test.ts src/main/index.test.ts src/main/window-chrome-config.test.ts
npm run typecheck
git diff --check
```

Remaining risks:

```text
Packaged tray menu QA, Local Whisper import boundary, Connect Phone live QA,
Windows NSIS, signing/notarization, and release metadata remain open.
```

## 2026-06-21 - Kun Baseline During Planner Executor Runtime Closure

Source:

```text
Kun product baseline:
  v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
Current refs:
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
Reasonix runtime context:
  bfe398cc89c6cba27fa26f3d55ee2cfc8f5208d0
```

Summary:

```text
Kun did not drive a new product-entry change in this runtime-heavy stage. The
stage absorbed Reasonix runtime/shell/provider/MCP/Go evidence only behind
analytix contracts. Existing Kun-derived desktop affordances remain governed by
the v0.2.13 -> v0.2.14 entry baseline.
```

Classification:

| Item | Class | analytix decision | Follow-up |
| --- | --- | --- | --- |
| Composer/dropdown/jump rail lineage | `record / already guarded` | No new Kun UI code was imported. Current Workbench route-surface and shell tests continue to guard the product-entry baseline. | Future focused desktop UX batch only. |
| Tray session menu | `already absorbed / package QA pending` | Existing analytix-native tray menu remains a desktop convenience surface, not a renderer route or bridge alias. | Packaged tray interaction QA remains release evidence. |
| Local Whisper | `defer pending import boundary` | Still not imported; no bundled speech resource, entitlement, permission, package-size, or signing risk was added. | Future Local Whisper spec/oracle/import-boundary and platform QA. |

Product-entry result:

```text
Code / Write / Settings / Plugins / Connect Phone / Schedule remain the
v0.2.13 -> v0.2.14 baseline. No top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry was added.
```

Validation:

```text
Final route-surface and forbidden-entry scans are recorded in release evidence
after the final rerun.
```

Remaining risks:

```text
Packaged desktop QA, Local Whisper import boundary, Connect Phone live QA,
Windows NSIS, signing/notarization, and release metadata remain open.
```

## 2026-06-21 - Kun Baseline During Reasonix 881 Control Closure

Source:

```text
Kun product baseline:
  v0.2.13 / kun-v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 / kun-v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
Current refs:
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
Reasonix runtime context:
  requested 881b2f2f2644d1873c7867f29c64cae7be7d9c24
  current origin/main-v2 9ada14176629b1d59d7ed78446951b2bb5954904
```

Summary:

```text
This stage absorbed Reasonix control/runtime semantics only behind analytix
contracts. It did not import a new Kun desktop surface. Code / Write /
Settings / Plugins / Connect Phone / Schedule remain the product-entry
baseline.
```

Stash audit:

| Stash | Decision |
| --- | --- |
| `codex-preserve-workbench-preload-before-reasonix-parity` | `duplicate / covered-by-HEAD semantic`; retained because it is not byte-identical and may preserve user audit intent. |
| `codex-preserve-loading-page-before-reasonix-parity` | `duplicate / covered-by-HEAD semantic`; retained because it is not byte-identical and may preserve user audit intent. |

Rejected:

```text
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry.
No Kun identity or deprecated bridge alias.
No Local Whisper import beyond boundary notes.
No default Go backend, Electron-to-Go routing, or renderer-visible Go route.
```

## 2026-06-21 - Kun 0.2.14 Provider Context Window Correction

Source:

```text
Kun target refs remain:
  v0.2.13 / kun-v0.2.13 201a1469ffbd911f6b95d450b78471e51b42acae
  v0.2.14 / kun-v0.2.14 06be05d76223208724c07301fa0f830641a06e6f
  master 8602476c5c449b4561473ad5f31081ee93dc782e
  develop 247076f297170c3d0c558baffb073c629894faea
```

Correction:

Kun 0.2.14 raised the default unknown-model context window to `128_000`.
Analytix runtime already used `DEFAULT_CONTEXT_WINDOW_TOKENS = 128_000`, but
the shared provider settings layer still defaulted explicit text profiles
without `contextWindowTokens` to `24_000`.

Absorbed:

| Area | analytix-owned result |
| --- | --- |
| Shared provider default | `src/shared/app-settings-provider.ts` now uses `128_000` for provider model profiles that omit `contextWindowTokens`. |
| Override safety | `src/shared/app-settings-provider.test.ts` proves explicit `contextWindowTokens` still wins. |
| Product boundary | This correction does not add Kun master/develop Workflow/Create Loop routes, Kun identity, or deprecated bridge/settings aliases. |

Validation:

```text
npm run test -- src/shared/app-settings-provider.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-21 - Approval/User-Input Abort Cleanup Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The approval/user-input abort cleanup batch is Reasonix-runtime inspired and
does not absorb a Kun product-entry, settings, bridge, provider, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, or old
runtime settings write path is added.
```

## 2026-06-21 - Renderer Approval Live-Card Evidence Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer approval live-card store evidence batch is a Reasonix-runtime
cleanup proof at the analytix renderer/store boundary. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, or Kun public protocol is added.
```

## 2026-06-21 - Go G5 Planner-Forbidden Shadow Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G5 planner-forbidden shadow replay batch is Reasonix-runtime inspired
and applies only to analytix conformance fixtures plus Go shadow code. It does
not absorb a Kun product-entry, settings, bridge, provider, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, or default Go backend is added.
```

## 2026-06-21 - Structured User-Input Choice Validation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The structured `request_user_input` choice validation batch is Reasonix-control
inspired and applies only to analytix runtime tool validation plus G4/Go shadow
evidence. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Connect Phone Copy Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Connect Phone copy sovereignty batch is a product-identity correction for
analytix. It does not absorb a Kun product-entry, settings, bridge, provider
default, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added. Legacy Claw recognizers remain compatibility-only for old
threads/prompts.
```

## 2026-06-22 - Go G5 Durable Runner Restart Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G5 durable runner restart executable shadow is Reasonix/Go-runtime
inspired and applies only to internal G5 conformance evidence. It does not
absorb a Kun product-entry, settings, bridge, provider default, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Runtime Client Bridge G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime client bridge G5 shadow batch is Reasonix-runtime inspired
and applies only to analytix renderer client/oracle and Go G5 shadow evidence.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix public bridge protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Settings Bridge G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer settings bridge G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix renderer settings/oracle and Go G5 shadow evidence.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix settings protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Preload Runtime Request Bridge G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload runtime request bridge G5 shadow is Reasonix-runtime inspired and
applies only to analytix preload bridge proof, G5 conformance fixtures, Go
shadow replay, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone
copy change, Local Whisper behavior, tray behavior, packaging identity, or
UI/navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Main IPC Runtime Adapter Handoff G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main IPC runtime-adapter handoff G5 shadow is Reasonix-runtime inspired and
applies only to analytix main IPC handler proof, G5 conformance fixtures, Go
shadow replay, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone
copy change, Local Whisper behavior, tray behavior, packaging identity, or
UI/navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Main IPC Endpoint Builder G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main IPC endpoint-builder G5 shadow is Reasonix-runtime inspired and
applies only to analytix main IPC schema proof, G5 conformance fixtures, Go
shadow replay, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone
copy change, Local Whisper behavior, tray behavior, packaging identity, or
UI/navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Runtime Endpoint Builder G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime endpoint-builder G5 shadow is Reasonix-runtime inspired
and applies only to analytix renderer provider source proof, G5 conformance
fixtures, Go shadow replay, and product-sovereignty scan tokens. It does not
absorb a Kun product-entry, settings schema, bridge alias, provider default,
Connect Phone copy change, Local Whisper behavior, tray behavior, packaging
identity, or UI/navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Runtime Endpoint Builder Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime endpoint builder proof is Reasonix-runtime inspired and
applies only to analytix renderer provider tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Main IPC Endpoint Builder Allow-list Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main IPC endpoint builder allow-list proof is Reasonix-runtime inspired and
applies only to analytix main IPC schema tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Main IPC Runtime Adapter Handoff Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main IPC runtime adapter handoff proof is Reasonix-runtime inspired and
applies only to analytix IPC handler tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Preload Runtime Request Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload runtime request bridge proof is Reasonix-runtime inspired and
applies only to analytix preload bridge tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge alias,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Runtime Host URL Handoff G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime host URL handoff G5 shadow is Reasonix-runtime inspired and applies
only to analytix runtime adapter conformance plus Go shadow replay. It does not
absorb a Kun product-entry, settings, bridge alias, provider default, Connect
Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go HTTP server, or Rust/Tauri path is added.
```

## 2026-06-23 - Main SSE Host URL Encoding G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main SSE host URL encoding G5 shadow is Reasonix-runtime inspired and
applies only to analytix main SSE IPC conformance plus Go shadow replay. It
does not absorb a Kun product-entry, settings, bridge alias, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go SSE server, or Rust/Tauri path is added.
```

## 2026-06-23 - Preload SSE Bridge G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload SSE bridge G5 shadow is Reasonix-runtime inspired and applies only
to analytix preload bridge conformance plus Go shadow replay. It does not
absorb a Kun product-entry, settings, bridge alias, provider default, Connect
Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go SSE server, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime Host URL Handoff Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime host URL handoff proof is Reasonix-runtime inspired and applies
only to analytix runtime adapter tests plus product-sovereignty scan tokens. It
does not absorb a Kun product-entry, settings, bridge alias, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Preload SSE Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload SSE bridge proof is Reasonix-runtime inspired and applies only to
analytix preload bridge tests plus product-sovereignty scan tokens. It does
not absorb a Kun product-entry, settings, bridge alias, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Main SSE Host URL Encoding Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main SSE host URL encoding proof is Reasonix-runtime inspired and applies
only to analytix main SSE IPC tests plus product-sovereignty scan tokens. It
does not absorb a Kun product-entry, settings, bridge alias, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - MCP Call-Time Reconnect G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP call-time reconnect G5 shadow is an internal analytix runtime/tool
conformance proof. It records existing MCP provider behavior for stale
connection retry and deterministic protocol error no-retry in TS/Go shadow
fixtures. It does not absorb a Kun product-entry, MCP-indexer surface, settings
schema, bridge alias, provider default, Connect Phone, Local Whisper, tray,
packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix MCP public protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - AutoResearch Direction Tracking G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The AutoResearch direction-tracking G5 shadow is an internal analytix
goal/runtime conformance proof. It records existing `record_research_direction`
behavior, project-local `.analytix/autoresearch/<threadId>/` files, and active
research-goal gating in TS/Go shadow fixtures. It does not absorb a Kun
product-entry, top-level AutoResearch surface, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix project protocol, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Settings Read G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer settings-read G5 shadow is an analytix bridge/conformance proof.
It applies only to conformance fixtures, Go shadow replay, settings-read source
evidence, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Usage Facade G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer usage facade G5 shadow is an analytix bridge/conformance proof.
It applies only to conformance fixtures, Go shadow replay, usage/settings
source evidence, usage tests, and product-sovereignty scan tokens. It does not
absorb a Kun product-entry, settings schema, bridge alias, provider default,
Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Side Conversation Relation G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The side conversation relation G5 shadow is an analytix store/provider
contract proof. It applies only to conformance fixtures, Go shadow replay,
side-store tests, provider tests, and product-sovereignty scan tokens. It does
not absorb a Kun product-entry, settings schema, bridge alias, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Provider Facade Seal G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer provider facade seal G5 shadow is an analytix bridge hardening
change. It applies only to conformance fixtures, Go shadow replay, renderer
provider tests, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Provider Alias Guard G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer provider alias guard G5 shadow is Reasonix-runtime inspired and
applies only to analytix conformance fixtures, Go shadow replay, renderer
provider tests, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Runtime Client Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime client bridge proof is Reasonix-runtime inspired and
applies only to analytix renderer runtime client tests plus product-sovereignty
scan tokens. It does not absorb a Kun product-entry, settings, bridge alias,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Settings Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer settings bridge proof is Reasonix-runtime inspired and applies
only to analytix renderer settings facade tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Runtime Provider Alias Guard Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime provider alias guard is Reasonix-runtime inspired and
applies only to analytix renderer provider tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Provider Runtime Client Facade Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer provider runtime client facade seal is an analytix bridge
hardening change. It routes archive/restore through `rendererRuntimeClient`
and adds a direct-bypass scan, but it does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Side Conversation Relation Provider Contract Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The side conversation relation provider contract is an analytix bridge
hardening change. It routes side promotion through the provider contract and
runtime client facade, but it does not absorb a Kun product-entry, settings
schema, bridge alias, provider default, Connect Phone, Local Whisper, tray,
packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Usage Runtime Client Facade Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer usage runtime client facade seal is an analytix bridge hardening
change. It routes usage and diagnostics requests through `rendererRuntimeClient`
and scans against direct runtime request bridge calls in renderer production
source, but it does not absorb a Kun product-entry, settings schema, bridge
alias, provider default, Connect Phone, Local Whisper, tray, packaging, or UI
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Settings Read Facade Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer settings read facade seal is an analytix bridge hardening change.
It routes ordinary renderer settings reads through `rendererRuntimeClient` and
scans against direct `window.analytix.settings.getSettings` in production
renderer source, but it does not absorb a Kun product-entry, settings schema,
bridge alias, provider default, Connect Phone, Local Whisper, tray, packaging,
or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Named Bridge API Allow-list Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer named bridge API allow-list is an analytix bridge hardening change.
It keeps only explicit named `window.analytix.runtime.*` and
`window.analytix.settings.*` contracts available for direct renderer production
use while generic runtime HTTP requests and settings reads remain sealed behind
the renderer client facade. It does not absorb a Kun product-entry, settings
schema, bridge alias, provider default, Connect Phone, Local Whisper, tray,
packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Optional Bridge Bypass Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer optional bridge bypass seal is an analytix bridge hardening and
scan-freshness change. It treats optional-chaining direct bridge access as the
same surface as dot access, routes the Connect Phone dialog settings read
through `rendererRuntimeClient`, and removes plugin marketplace probing of the
generic runtime bridge. It does not absorb a Kun product-entry, settings schema,
bridge alias, provider default, Connect Phone, Local Whisper, tray, packaging,
or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Renderer Bridge Allow-list G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer bridge allow-list G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix conformance fixtures, Go shadow replay, and
product-sovereignty scan tokens. It does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Runtime HTTP Auth Matrix G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime HTTP auth matrix G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix runtime conformance fixtures, Go shadow replay, and
product-sovereignty scan tokens. It does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Runtime Forbidden Route Dispatch G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime forbidden route dispatch G5 shadow batch is Reasonix-runtime
inspired and applies only to analytix runtime conformance fixtures, Go shadow
replay, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-23 - Shared Endpoint Builder G5 Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The shared endpoint builder G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix runtime conformance fixtures, Go shadow replay, and
product-sovereignty scan tokens. It does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Shared Endpoint Builder Sovereignty Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The shared endpoint builder sovereignty proof is Reasonix-runtime inspired and
applies only to analytix shared endpoint tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime Forbidden Route Dispatch Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime forbidden route dispatch proof is Reasonix-runtime inspired and
applies only to analytix runtime conformance tests plus product-sovereignty
scan tokens. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime HTTP Auth Matrix Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime HTTP auth matrix proof is Reasonix-runtime inspired and applies
only to analytix runtime conformance tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime HTTP Route Sovereignty Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime HTTP route sovereignty batch is Reasonix-runtime inspired and
applies only to analytix runtime route fixtures, conformance tests, Go shadow
output, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings, bridge, provider default, Connect Phone, Local
Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Package Runtime CLI Identity Shadow Has No Kun Product Delta

Kun baseline:

```text
Kun 0.2.13/0.2.14 product entry structure remains unchanged. This batch does
not add Kun product identity, deprecated bridge aliases, old settings fallback,
or Kun-absent top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
navigation.
```

Analytix-owned proof:

| Surface | D-0188 guard |
| --- | --- |
| Package identity | Root package name/product name stay `analytix`; runtime package stays `analytix-runtime`. |
| Runtime CLI | Runtime package bin stays `analytix -> ./dist/cli/serve-entry.js`, and CLI usage stays `analytix serve [options]`. |
| Desktop release identity | electron-builder app id stays `com.analytix.desktop`, product/artifact/NSIS names stay `analytix`, and AppUserModelID stays `com.analytix.desktop`. |
| Release env | Release workflow and builder config use `ANALYTIX_*` env names. |

Boundary:

```text
No Kun identity, Reasonix public protocol, renderer-visible Go route, default
Go backend, Rust/Tauri path, packaged artifact walkthrough, or release
readiness is added.
```

## 2026-06-22 - Tool Result File/Image Boundary Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The tool result file/image boundary batch is Reasonix-runtime inspired and
applies only to analytix G5 conformance tests, Go shadow fixtures, renderer
projection proof, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings, bridge, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Event JSONL Replay Boundary Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The event JSONL replay boundary batch is Reasonix-runtime inspired and applies
only to analytix G5 conformance tests, Go shadow fixtures, runtime event
source proof, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings, bridge, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Route-Surface Sovereignty Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer route-surface sovereignty shadow is a Reasonix/Kun boundary proof
and applies only to analytix Workbench route-surface evidence plus Go G5 shadow
conformance. It does not absorb a Kun product-entry, settings section, bridge
alias, provider default, Connect Phone, Local Whisper, tray, packaging, or
feature navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Desktop Main IPC Boundary Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The desktop main IPC boundary shadow is a Reasonix-runtime/frontend absorption
proof and applies only to analytix main IPC schema/handler plus runtime SSE
conformance evidence. It does not absorb a Kun product-entry, settings section,
bridge alias, provider default, Connect Phone, Local Whisper, tray, packaging,
or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Desktop Bridge IPC Sovereignty Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The desktop bridge IPC sovereignty shadow is a Reasonix-runtime/frontend
absorption proof and applies only to analytix preload/shared API settings
contracts plus Go G5 shadow evidence. It does not absorb a Kun product-entry,
settings section, bridge alias, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - MCP Malformed Schema Boundary Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP malformed schema boundary batch is Reasonix-runtime inspired and
applies only to analytix G5 conformance tests, Go shadow fixtures, MCP provider
source proof, and product-sovereignty scan tokens. It does not absorb a Kun
product-entry, settings, bridge, provider default, Connect Phone, Local Whisper,
tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Sub-Agent Review Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section;
the D-0179 review is a QA evidence matrix for Reasonix absorption decisions.
```

Decision:

The six-lane sub-agent review does not add, rename, or promote a Kun product
entry. It records that Kun `0.2.13` -> `0.2.14` still preserves Code, Write,
Settings, Plugins, Connect Phone, and Schedule, while dormant Create Loop code
remains quarantined from top-level navigation.

Boundary:

```text
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry,
Kun identity, deprecated bridge alias, old runtime settings write path, Kun
public protocol, default Go backend, renderer-visible Go route, or Rust/Tauri
path is added.
```

## 2026-06-22 - Custom Provider Telemetry Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section;
D-0180 only strengthens Reasonix-runtime-inspired provider/cache evidence.
```

Decision:

The custom provider telemetry seal does not add, remove, rename, or promote any
Kun product entry. It keeps custom provider behavior as request-shape proof
inside analytix provider/cache fixtures and Go G5 shadow evidence.

Boundary:

```text
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry,
Kun identity, deprecated bridge alias, old runtime settings write path, Kun
public protocol, default Go backend, renderer-visible Go route, provider
runtime behavior change, custom provider cache telemetry, or Rust/Tauri path is
added.
```

## 2026-06-22 - Goal Persistence Off-Lock Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section;
D-0181 only strengthens Reasonix-runtime-inspired Go G5 conformance evidence.
```

Decision:

The goal persistence off-lock control shadow does not add, remove, rename, or
promote any Kun product entry. It records analytix runtime behavior around
`ThreadService` goal persistence and future Go conformance only.

Boundary:

```text
No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entry,
Kun identity, deprecated bridge alias, old runtime settings write path, Kun
public protocol, default Go backend, renderer-visible Go route, live Go goal
manager, Reasonix controller protocol, or Rust/Tauri path is added.
```

## 2026-06-22 - Post-881 Stage Closure Snapshot Preserves Kun Baseline

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The post-881 stage closure snapshot is QA/docs inspired and applies only to
analytix closure evidence plus `scan:product-sovereignty`. Its
`kunBaselinePreserved` section explicitly keeps Kun `0.2.13` -> `0.2.14`
product-entry boundaries intact. It does not absorb a Kun product-entry,
settings, bridge, provider default, Connect Phone, Local Whisper, tray,
packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Release Evidence Final-Gate Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The release evidence final-gate freshness batch is QA/docs inspired and applies
only to `scan:product-sovereignty` plus analytix release evidence. It does not
absorb a Kun product-entry, settings, bridge, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Session Route Proof Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The session route proof-freshness batch is Reasonix/session-runtime inspired
and applies only to analytix G5 conformance tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Request-Shape Derived Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider request-shape derived replay batch is Reasonix/provider-runtime
inspired and applies only to analytix conformance fixtures plus Go G3/G5 shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, provider behavior change, or Rust/Tauri path is added.
```

## 2026-06-22 - Post-881 Runtime Proof Freshness V2 Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The proof-freshness v2 batch is a reusable scan guard for Reasonix-inspired
runtime evidence. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, provider behavior change, MCP route, task-job route, or Rust/Tauri
path is added.
```

## 2026-06-22 - IPC Settings Patch Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The IPC settings patch sovereignty batch is an analytix settings-schema guard.
It does not absorb a Kun product-entry, settings UI, provider default, Connect
Phone, Local Whisper, tray, packaging, or Workflow runtime delta. It only proves
renderer-to-main settings patches keep legal analytix fields while dropping
legacy Kun/Reasonix top-level envelopes before validation.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime Desktop Bridge Proof Freshness Scan Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime desktop bridge proof freshness scan is a guard over existing
analytix-owned bridge evidence. It does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Preload Runtime Request IPC Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload runtime request IPC bridge proof is an analytix desktop bridge
guard. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Main Runtime Request Forbidden Route Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main runtime request forbidden-route proof is an analytix IPC allow-list
guard. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Main/Preload SSE IPC Bridge Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The main/preload SSE IPC bridge proof is an analytix desktop runtime contract
guard. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer SSE Bridge Cursor Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer SSE bridge/cursor proof is an analytix runtime adapter guard. It
does not absorb a Kun product-entry, settings schema, bridge alias, provider
default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Renderer Runtime Request Surface Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer runtime request-surface proof is an analytix runtime adapter
guard. It does not absorb a Kun product-entry, settings schema, bridge alias,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime Proof Freshness Scan Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime proof freshness batch is a scan/document guard over existing
analytix runtime conformance evidence. It does not absorb a Kun product-entry,
settings schema, bridge alias, provider default, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Workflow Singular Route Negative Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The workflow singular route negative proof is an HTTP router guard. It does
not absorb a Kun product-entry, settings schema, bridge alias, provider
default, Connect Phone, Local Whisper, tray, or packaging delta. It only proves
singular `/v1/workflow` stays absent alongside plural `/v1/workflows`.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Settings Sovereignty Scan Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The settings sovereignty scan-freshness batch is a guardrail over existing
analytix settings and bridge ownership paths. It does not absorb a Kun
product-entry, settings schema, bridge alias, provider default, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Browser Preview Settings Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The browser preview settings sovereignty batch is an analytix settings-schema
guard. It does not absorb a Kun product-entry, settings UI, provider default,
Connect Phone, Local Whisper, tray, packaging, or Workflow runtime delta. It
only proves browser preview settings keep valid top-level `runtime` fields and
reject legacy Kun/Reasonix agent envelopes on load and re-save.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Browser Bridge And Visible Entry Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The browser preview bridge and visible-entry sovereignty batch is a product
boundary guard. It does not absorb a Kun product-entry, settings, provider,
Connect Phone, Local Whisper, tray, packaging, or Workflow runtime delta.
Instead, it strengthens the proof that analytix keeps the Kun v0.2.13 /
v0.2.14 top-level entry baseline while hidden future-capability code remains
quarantined.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. The rendered sidebar and source/scan guards reject
Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entries,
Kun identity, deprecated bridge aliases, old runtime settings write path,
Kun public protocol, default Go backend, renderer-visible Go route, and
Rust/Tauri path.
```

## 2026-06-22 - Browser Preview SSE Bridge Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The browser preview SSE bridge proof is an analytix runtime-contract guard. It
does not absorb a Kun product-entry, settings, provider, Connect Phone, Local
Whisper, tray, packaging, or Workflow runtime delta. It only proves browser
preview SSE stays on analytix `window.analytix.runtime.startSse` and
`/v1/threads/:id/events` semantics.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Auto-Router Recommendation/Currentness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the desktop bridge/settings sovereignty
section; Kun explorer rechecked top-level entry and bridge/settings boundaries.
```

Decision:

The auto-router recommendation/currentness control-shadow batch is
Reasonix-runtime inspired and applies only to analytix internal `_auto_router`
evidence plus Go G5 shadow output. It does not absorb a Kun product-entry,
settings, bridge, provider, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix controller protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

Follow-up:

```text
Kun develop drift was observed by read-only explorer as newer than some older
ledger records; treat that as a future drift-triage record, not a blocker for
this D-0153 runtime-only batch.
```

## 2026-06-22 - Exact Session Route Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the auto-router recommendation/currentness
section.
```

Decision:

The exact session route replay batch is Reasonix/session-runtime inspired and
applies only to analytix internal thread/session route conformance evidence plus
Go G5 shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix SessionAPI/public route
protocol, default Go backend, renderer-visible Go route, or Rust/Tauri path is
added.
```

## 2026-06-22 - User-Input Structured Validation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The structured user-input validation control-shadow batch is Reasonix-runtime
inspired and applies only to analytix internal `request_user_input` and Go G5
shadow evidence. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Planner Gate Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The planner gate matrix control-shadow batch is Reasonix-runtime inspired and
applies only to analytix Plan mode tool-policy evidence plus Go G5 shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, public auto-plan setting, or Rust/Tauri path is added.
```

## 2026-06-22 - Desktop Bridge/Settings Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The desktop bridge/settings sovereignty control-shadow batch is driven by
Reasonix public-protocol/config rejection and analytix product-boundary proof.
It does not absorb a Kun product-entry, provider, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, public auto-plan setting, or Rust/Tauri path is added.
```

## 2026-06-22 - Context Compaction Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The compaction-boundary executable shadow batch is Reasonix-runtime inspired
and applies only to analytix internal long-thread/cache conformance evidence
plus Go G5 shadow output. It does not absorb a Kun product-entry, settings,
bridge, provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Provider Cache Inventory Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider cache inventory control-shadow batch is Reasonix-runtime inspired
and applies only to analytix provider/cache conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Session Route Inventory Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The session route inventory control-shadow batch is Reasonix-runtime inspired
and applies only to analytix thread/session route conformance evidence plus Go
G5 shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Approval/User-Input Inventory Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The approval/user-input inventory control-shadow batch is Reasonix-runtime
inspired and applies only to analytix approval/user-input conformance evidence
plus Go G5 shadow output. It does not absorb a Kun product-entry, settings,
bridge, provider default, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task Planner Toolset Inventory Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task planner toolset inventory control-shadow batch is Reasonix-runtime
inspired and applies only to analytix internal task-job/planner conformance
evidence plus Go G5 shadow output. It does not absorb a Kun product-entry,
settings, bridge, provider default, Connect Phone, Local Whisper, tray, or
packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Boundary Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP core lifecycle boundary-seal control-shadow batch is Reasonix-runtime
inspired and applies only to analytix internal MCP lifecycle conformance
evidence plus Go G5 shadow output. It does not absorb a Kun product-entry,
settings, bridge, provider default, Connect Phone, Local Whisper, tray, or
packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G3/G5 Provider Request-Shape Exact Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider request-shape exact-matrix batch is Reasonix-runtime inspired and
applies only to analytix internal provider/cache conformance evidence plus Go
G3/G5 shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Combined Step/Cancel/Cache Trace Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The combined step/cancel/cache trace batch is Reasonix-runtime inspired and
applies only to analytix internal G5 conformance evidence plus Go shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 History Repair Pair-Integrity Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The history repair pair-integrity batch is Reasonix-runtime inspired and
applies only to analytix internal G5 conformance evidence plus Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Step-Limit Override/Delegate Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The step-limit override/delegate matrix batch is Reasonix-runtime inspired and
applies only to analytix internal G5 conformance evidence plus Go shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

Parallel Kun baseline checkpoint:

```text
Read-only sub-agent review reconfirmed Kun v0.2.13 = 2ba8decc, Kun v0.2.14 =
8f204034, and current legacy Kun refs remain outside the analytix product-entry
target. Current Kun Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer
surfaces remain reject/defer for top-level analytix navigation unless a future
analytix-owned spec changes the route hierarchy.
```

## 2026-06-22 - Go G5 Provider Cache Privacy Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider cache privacy control-shadow batch is Reasonix-runtime inspired
and applies only to analytix provider/cache conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task-Job Tool Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job tool contract boundary control-shadow batch is Reasonix-runtime
inspired and applies only to internal task/sub-agent orchestration conformance
evidence. It does not add a Kun product entry or change Kun 0.2.13/0.2.14
top-level navigation expectations.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Nested Child SSE Metadata Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The nested child SSE metadata control-shadow batch is Reasonix-runtime inspired
and applies only to internal task/sub-agent event attribution conformance
evidence. It does not add a Kun product entry, change Kun 0.2.13/0.2.14
top-level navigation expectations, or expose Subagent as a top-level product
surface.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task Transcript Identity Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task transcript identity control-shadow batch is Reasonix-runtime inspired
and applies only to internal task/sub-agent orchestration conformance evidence.
It does not add a Kun product entry, change Kun 0.2.13/0.2.14 top-level
navigation expectations, or expose Subagent as a top-level product surface.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Product Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The product-boundary control-shadow batch is a cross-upstream sovereignty gate,
not a Kun product feature. It proves analytix still owns bridge, serve,
settings/runtime, route-surface, and backend-default decisions while continuing
to absorb Reasonix/Kun capabilities.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Background Reconnect Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP background reconnect control-shadow batch is Reasonix-runtime inspired
and applies only to analytix MCP/tool lifecycle conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Known Override Diagnostics Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP known override diagnostics control-shadow batch is Reasonix-runtime
inspired and applies only to analytix MCP/tool lifecycle conformance evidence
plus Go G5 shadow output. It does not absorb a Kun product-entry, settings,
bridge, provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Live-Local Indexer Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP live-local indexer control-shadow batch is Reasonix-runtime inspired
and applies only to analytix MCP/tool lifecycle conformance evidence plus Go
G5 shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Search Meta-Tool Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP search meta-tool control-shadow batch is Reasonix-runtime inspired and
applies only to analytix MCP/tool lifecycle conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP core lifecycle control-shadow batch is Reasonix-runtime inspired and
applies only to analytix MCP/tool lifecycle conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Search Workspace Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP search workspace boundary control-shadow batch is Reasonix-runtime
inspired and applies only to analytix MCP/tool lifecycle conformance evidence
plus Go G5 shadow output. It does not absorb a Kun product-entry, settings,
bridge, provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Approval Annotation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP approval annotation control-shadow batch is Reasonix-runtime inspired
and applies only to analytix MCP/tool approval conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Search Refresh Drift Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP search refresh drift control-shadow batch is Reasonix-runtime inspired
and applies only to analytix MCP/tool lifecycle conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Approval/User-Input Route Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The approval/user-input route replay control-shadow batch is Reasonix-runtime
inspired and applies only to analytix approval/user-input conformance evidence
plus Go G5 shadow output. It does not absorb a Kun product-entry, settings,
bridge, provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - MCP Refresh Catalog Drift Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP refresh catalog drift replay batch is Reasonix-runtime inspired and
applies only to analytix internal MCP/tool lifecycle evidence plus Go G5 shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix MCP-indexer protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Thread/SSE Route Auth Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The thread/SSE route auth replay batch is runtime-contract evidence only. It
does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix SessionAPI protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Runtime Settings Legacy-Agent Guard Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The runtime settings legacy-agent pollution guard is settings-contract evidence
only. It does not absorb a Kun product-entry, UI, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix settings protocol, default
Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Task-Job Route Executable Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the route executable replay section.
```

Decision:

The task-job route executable control-shadow batch is Go G5 conformance
evidence only. It does not absorb a Kun UI, navigation, bridge, provider,
Connect Phone, Local Whisper, tray, packaging, or public protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix SessionAPI/job protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Task-Job Lifecycle Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the task-job lifecycle replay section.
```

Decision:

The task-job lifecycle control-shadow batch is Go G5 conformance evidence
only. It does not absorb a Kun UI, navigation, bridge, provider, Connect Phone,
Local Whisper, tray, packaging, or public protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix lifecycle/session protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Planner-Executor Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the planner-executor replay section.
```

Decision:

The planner-executor control-shadow batch is Go G5 conformance evidence only.
It does not absorb a Kun UI, navigation, bridge, provider, Connect Phone, Local
Whisper, tray, packaging, or public protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix planner/job protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Cache Release Guard Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the planner-executor control-shadow
section.
```

Decision:

The provider cache release-guard control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Usage Parser Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider cache release-guard
control-shadow section.
```

Decision:

The provider usage parser control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Request Shape Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider usage parser control-shadow
section.
```

Decision:

The provider request-shape control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Cache Accounting Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider request-shape control-shadow
section.
```

Decision:

The provider cache accounting control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Release/Package Identity Guard Keeps Kun Out Of Public Identity

Source:

```text
Kun target refs remain unchanged from the provider cache accounting
control-shadow section.
```

Decision:

The release/package identity guard is analytix product-sovereignty evidence. It
does not absorb a Kun UI, navigation, bridge, provider default, Connect Phone,
Local Whisper, tray, packaging behavior, or public protocol delta; it proves
release/package configuration remains analytix-owned.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Streaming Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the release/package identity guard
section.
```

Decision:

The provider streaming control-shadow batch is Go G5/provider-streaming
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Offline Parity Seal Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider streaming control-shadow
section.
```

Decision:

The provider offline parity seal control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Session Route Status Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider offline parity seal
control-shadow section.
```

Decision:

The session route status control-shadow batch is Go G5/thread-route
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix SessionAPI/job protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Live-Local HTTP Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the session route status control-shadow
section.
```

Decision:

The provider live-local HTTP control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Drift Attribution Control Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider live-local HTTP
control-shadow section.
```

Decision:

The provider drift attribution control-shadow batch is Go G5/provider-cache
conformance evidence only. It does not absorb a Kun UI, navigation, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or public
protocol delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix provider/cache protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Plan Step Cancel/Cache Boundary Has No Kun Product Delta

Scope:

```text
D-0107 is Reasonix planner/cache inspired and stays inside the existing
analytix AgentLoop Plan mode contract. Kun 0.2.13/0.2.14 product hierarchy is
unchanged.
```

Kun guardrails preserved:

| Area | Result |
| --- | --- |
| Navigation hierarchy | No top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer route/entry was added. |
| Product identity | No Kun identity, Reasonix identity, `window.kun`, deprecated bridge alias, or legacy settings fallback was added. |
| Plan mode ownership | The fixture uses explicit analytix `mode: "plan"`; no Kun or Reasonix public planner protocol is exposed. |
| Provider behavior | The fake DeepSeek-style stream only proves local cache diagnostics; no provider URL/body behavior changes. |
| Go/Rust boundary | No default Go backend, renderer-visible Go route, Rust/Tauri path, or native rewrite was added. |

Validation:

```text
npm --prefix packages/runtime test -- tests/loop.test.ts -t "aborted plan follow-up" --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - MCP Stdio Execution-Error Redaction Has No Kun Product Delta

Scope:

```text
D-0108 is an internal analytix MCP/tool privacy fix. Kun 0.2.13/0.2.14
navigation, identity, settings, bridge, and provider hierarchy remain
unchanged.
```

Kun guardrails preserved:

| Area | Result |
| --- | --- |
| Navigation hierarchy | No top-level Workflow, Create Loop, Subagent, AutoResearch, or MCP-indexer route/entry was added. |
| Product identity | No Kun identity, Reasonix identity, `window.kun`, deprecated bridge alias, or legacy settings fallback was added. |
| MCP surface | The fix redacts internal tool results; it does not expose Reasonix MCP-indexer public protocol. |
| Go/Rust boundary | No default Go backend, renderer-visible Go route, Rust/Tauri path, or native rewrite was added. |

Validation:

```text
npm --prefix packages/runtime test -- tests/mcp-tool-lifecycle-oracle.test.ts --no-file-parallelism --maxWorkers=1
```

## 2026-06-22 - Workflow/Create Loop Quarantine Scan

Scope:

```text
D-0109 strengthens Kun 0.2.13/0.2.14 product hierarchy proof by making the
dormant Workflow/Create Loop code quarantine explicit.
```

Kun guardrails preserved:

| Area | Result |
| --- | --- |
| Route hierarchy | `WorkflowCreateLoopView`, `runCreateLoopWorkflow`, `findPendingWorkflowGate`, and `analytix-create-loop` are forbidden from top-level entry surfaces. |
| Entry surfaces | Workbench, Sidebar, Write sidebar, Plugin Marketplace, store actions/navigation, preload, tray menu, and settings shortcuts are included in the quarantine scan/test. |
| Product identity | No Kun identity, Reasonix identity, deprecated bridge alias, or legacy settings fallback was added. |
| Go/Rust boundary | No default Go backend, renderer-visible Go route, Rust/Tauri path, or native rewrite was added. |

Validation:

```text
npm run test -- src/renderer/src/components/Workbench.route-surface.test.ts --no-file-parallelism --maxWorkers=1
npm run scan:product-sovereignty
```

## 2026-06-22 - G5 Parallel Task Dependency Executable Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 `parallel_tasks` dependency executable-shadow batch is
Reasonix-runtime inspired and applies only to analytix internal task-job
conformance plus Go G5 shadow output. It does not absorb a Kun product-entry,
settings, bridge, provider, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live Go Job Manager, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - HTTP Auto-Plan Payload Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The HTTP auto-plan payload guard is Reasonix-runtime inspired and applies only
to analytix start-turn contract safety. It does not absorb a Kun product-entry,
settings bridge, provider, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live Go Job Manager, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Write Sidebar and Connect Phone Placeholder Sovereignty

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

This batch protects the Kun 0.2.13 -> 0.2.14 entry baseline by scanning Write
sidebar surfaces and preventing newly generated Connect Phone placeholders
from emitting legacy `[Claw:...]` titles. It does not add a Kun product-entry,
settings bridge, provider, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live Go Job Manager, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Reasonix Auto-Plan Config Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
Reasonix `01d9b173` is settings-boundary input only.
```

Decision:

The auto-plan config boundary batch is Reasonix-settings inspired and affects
only analytix normalization, persistence, and runtime config rejection tests.
It does not absorb a Kun product-entry, bridge, provider, Connect Phone, Local
Whisper, tray, packaging, or Workflow/Create Loop delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Reasonix auto-plan setting, Reasonix controller protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Task Parent Goal Evidence Executable Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task parent-goal executable replay batch is Reasonix-runtime inspired and
applies only to analytix internal task-job evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, packaging, or Workflow/Create Loop delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Reasonix public sub-agent/job protocol, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Product Sovereignty Scan Engineering Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The product-sovereignty scan batch is QA engineering. It strengthens evidence
that Kun 0.2.13 -> 0.2.14 entry baseline remains intact while Reasonix/Kun
absorption continues. It does not absorb a Kun product-entry, bridge,
provider, Connect Phone, Local Whisper, tray, packaging, or Workflow/Create
Loop feature.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Reasonix public protocol, default Go backend,
renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Auto-Router Classifier Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The D-0100 auto-router classifier currentness replay batch is Reasonix-runtime
inspired and applies only to analytix internal auto-model-router evidence plus
Go G5 shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, Reasonix auto-plan setting, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Usage Parser Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The D-0099 provider usage-parser precedence replay batch is provider/cache
non-regression evidence. It does not absorb a Kun product-entry, settings,
bridge, provider default, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - MCP Lifecycle Detail Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The D-0098 MCP lifecycle detail replay batch is Reasonix-runtime inspired and
applies only to analytix internal MCP/tool lifecycle evidence plus Go G5 shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Approval/User-Input Route Body Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The D-0097 approval/user-input route-body replay batch is Reasonix-runtime
inspired and applies only to analytix internal conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Provider Streaming Usage Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider streaming usage replay batch is Reasonix-runtime/provider-cache
inspired and applies only to analytix G3/G5 conformance evidence plus Go shadow
output. It does not absorb a Kun product-entry, navigation, settings, bridge,
Connect Phone, Local Whisper, tray, installer, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, provider behavior change, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task-Job Route Executable Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job route executable replay batch is Reasonix-runtime/task
orchestration inspired and applies only to analytix internal task-job route
conformance evidence plus Go G5 shadow output. It does not absorb a Kun
product-entry, navigation, settings, bridge, provider, Connect Phone, Local
Whisper, tray, installer, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, public job protocol, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - G5 MCP Approval Annotation Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 MCP approval annotation replay batch is Reasonix-runtime/MCP approval
evidence work only. It records Go shadow summary fields derived from analytix
MCP lifecycle fixtures; it does not absorb a Kun product-entry, navigation,
settings, bridge, Connect Phone, Local Whisper, tray, installer, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live Go approval manager, live Go MCP
client, default Go backend, renderer-visible Go route, or Rust/Tauri path is
added.
```

## 2026-06-22 - G5 MCP Live-Local Indexer Summary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 MCP live-local indexer summary batch is Reasonix-runtime/MCP evidence
work only. It records Go shadow summary fields derived from analytix MCP
lifecycle fixtures; it does not absorb a Kun product-entry, navigation,
settings, bridge, Connect Phone, Local Whisper, tray, installer, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live Go MCP client, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - G5 Provider Live-Local Proof Summary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 provider live-local proof summary batch is Reasonix-runtime/provider
evidence work only. It records provider-cache oracle metadata and Go shadow
summary fields; it does not absorb a Kun product-entry, navigation, settings,
bridge, Connect Phone, Local Whisper, tray, installer, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, provider behavior change, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Live-Local HTTP Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider live-local HTTP proof batch is Reasonix-runtime/provider-cache
inspired and applies only to analytix runtime provider tests. It does not
absorb a Kun product-entry, navigation, settings, bridge, Connect Phone, Local
Whisper, tray, installer, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, provider behavior change, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Session Route Status Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 session route status replay batch is Reasonix/session-runtime inspired
and applies only to analytix thread/session HTTP/SSE conformance evidence plus
Go shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or navigation
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Provider Request Shape Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 provider request-shape replay batch is Reasonix/provider-cache inspired
and applies only to analytix provider/cache conformance evidence plus Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G3 Provider Request Shape Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G3 provider request-shape replay batch is Reasonix/provider-cache inspired
and applies only to analytix provider/cache conformance evidence plus Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G3 Cache Drift Attribution Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G3 cache-drift attribution replay batch is Reasonix/provider-cache inspired
and applies only to analytix provider/cache conformance evidence plus Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or navigation delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Cache Drift Attribution Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The cache-drift attribution replay batch is Reasonix/provider-cache inspired
and applies only to analytix provider/cache conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider default, Connect Phone, Local Whisper, tray, packaging, or navigation
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task Job Lifecycle Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job lifecycle replay batch is Reasonix-runtime inspired and applies
only to analytix task/job conformance evidence plus Go G5 shadow output. It
does not absorb a Kun product-entry, settings, bridge, provider, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go Job Manager, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Task Job Route Boundary Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job route-boundary replay batch is Reasonix-runtime inspired and
applies only to analytix task/job conformance evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go task-job routes, live Go Job Manager, or Rust/Tauri path is
added.
```

## 2026-06-22 - Go G5 MCP Core Lifecycle Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP core lifecycle replay batch is Reasonix-runtime inspired and applies
only to analytix MCP/tool lifecycle conformance evidence plus Go G5 shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go MCP client, live Go MCP route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Approval Decision Route Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The approval decision route replay batch is Reasonix-runtime inspired and
applies only to analytix approval/user-input conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live Go approval manager, or Rust/Tauri path is added.
```

## 2026-06-22 - Offline Provider/Cache Parity Seal Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The offline provider/cache parity seal is Reasonix-runtime inspired and applies
only to analytix provider/cache conformance evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, live provider matrix, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Parallel Task Dependency Validation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The `parallel_tasks` dependency validation replay batch is Reasonix-runtime
inspired and affects only analytix task-job conformance fixtures plus Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 MCP Search Meta-Tool Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP search meta-tool replay batch is Reasonix-runtime inspired and affects
only analytix MCP lifecycle conformance fixtures plus Go shadow output. It does
not absorb a Kun product-entry, settings, bridge, provider, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Abort Cleanup Replay Summary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The abort-cleanup replay-summary batch is Reasonix-runtime inspired and affects
only analytix G5 conformance fixtures plus Go shadow output. It does not absorb
a Kun product-entry, settings, bridge, provider, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Auto-Router Fingerprint Currentness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-router request contract fingerprint batch is Reasonix-runtime
inspired and applies only to analytix internal auto-router cache currentness.
It does not absorb a Kun product-entry, settings, bridge, provider default,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Custom Chat Full Endpoint Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The custom `/chat/completions` full endpoint batch is provider request-shape
non-regression evidence. It does not absorb a Kun product-entry, settings,
bridge, provider default, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Go G5 Provider Cache Release Guard Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider cache release guard shadow batch is Reasonix-runtime inspired and
applies only to analytix runtime/provider-cache conformance evidence. It does
not absorb a Kun product-entry, settings, bridge, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, live provider superiority claim,
default Go backend, or Rust/Tauri path is added.
```

## 2026-06-22 - D-0064 Auto-Route Step/Cancel Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-route step/cancel composition proof is Reasonix-agent-kernel inspired
and applies only to analytix TypeScript AgentLoop test evidence. It does not
absorb a Kun product-entry, settings, bridge, provider, Connect Phone, Local
Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix auto-plan setting, default
Go backend, or Rust/Tauri path is added.
```

## 2026-06-22 - D-0065 Planner Step-Limit Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The planner step-limit proof is Reasonix-agent-kernel inspired and applies only
to analytix TypeScript AgentLoop test evidence. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix planner setting, default Go
backend, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Checkpoint/Rewind Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The checkpoint/rewind G5 executable shadow batch is Reasonix-runtime inspired
and applies only to analytix checkpoint oracle and Go shadow evidence. It does
not absorb Kun git-ref checkpoint behavior, Kun UI, product-entry, settings,
bridge, provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, `refs/kun/checkpoints`, default Go
backend, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 User-Input Gate Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The submitted/cancelled user-input G5 shadow batch is Reasonix-runtime inspired
and applies only to analytix runtime conformance and shadow-only Go gate
evidence. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix ask protocol, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 AutoResearch State Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The AutoResearch project-local G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix internal goal/research state conformance. It does not
absorb a Kun product-entry, settings, bridge, provider, Connect Phone, Local
Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix AutoResearch/project
protocol, default Go backend, renderer-visible Go route, or Rust/Tauri path is
added.
```

## 2026-06-22 - Go G5 MCP Lifecycle Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP lifecycle G5 shadow batch is Reasonix-runtime inspired and applies only
to analytix internal runtime/tool conformance. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix MCP protocol, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Remote-Entry Boundary Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The remote-entry boundary G5 shadow batch is Reasonix-runtime inspired and
applies only to analytix internal runtime/control-port conformance. It does not
absorb a Kun product-entry, settings, bridge, provider, Connect Phone, Local
Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix SessionAPI/control protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Go G5 Resume Pending Gates Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The resume pending approval/user-input gates G5 shadow batch is
Reasonix-runtime inspired and applies only to analytix internal runtime/gate
conformance. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix ask/session protocol,
default Go backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-21 - Managed Runtime Currentness Guard Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction and
route-surface oracle sections.
```

Decision:

The managed runtime provider currentness batch is primarily Reasonix-runtime
inspired and applies only to analytix settings/runtime identity guards. It does
not absorb a Kun product-entry, public protocol, provider switcher, bridge alias,
Connect Phone feature, Local Whisper feature, tray feature, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. `agentProvider: "kun"` and `agents.kun` are explicitly
rejected as active runtime fallback input and are not persisted after settings
writeback. No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level
entry, Kun identity, deprecated bridge alias, old runtime settings write path,
Kun public protocol, default Go backend, or Rust/Tauri path is added.
```

## 2026-06-21 - Provider Endpoint URL Builder Parity Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction and
route-surface oracle sections.
```

Decision:

The provider endpoint URL builder parity batch is an analytix provider
non-regression change for shared URL helpers, Write, and Schedule. It does not
absorb a Kun product-entry, bridge, settings envelope, Connect Phone, Local
Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Connect Phone Product Copy Sovereignty

Source:

```text
Kun baseline rule that `claw` may remain only as Connect Phone internal
compatibility naming; user-visible copy must say Connect Phone/analytix.
```

Decision:

This batch is a Kun baseline correction, not a new Kun feature import. It
removes standalone `Claw` from renderer locale copy, IM command/runtime replies,
and model-facing prompt natural language while preserving internal `claw`
compatibility contracts.

Classification:

| Delta / idea | Classification | analytix decision |
| --- | --- | --- |
| User-visible Connect Phone copy | `contract-reimplement` | Locale scan and runtime tests guard against standalone `Claw`. |
| Internal `claw` module/settings names | `document-only` | Keep as compatibility internals for this phase. |
| Kun identity / bridge / settings shape | `reject` | Do not restore Kun public identity or old bridge/settings contracts. |

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Task-Job Stale Restart Reconciliation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job stale restart reconciliation batch is Reasonix-runtime inspired and
applies only to analytix internal runtime/oracle and Go G5 shadow evidence. It
does not absorb a Kun product-entry, settings, bridge, provider, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-21 - MCP Annotation Approval No-Execute Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP annotation approval no-execute batch is Reasonix-runtime inspired and
applies only to analytix internal MCP lifecycle oracle and Go G4 shadow
evidence. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, Reasonix MCP public protocol, or Rust/Tauri path is added.
```

## 2026-06-21 - User-Input Submitted Route Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The user-input submitted route batch is Reasonix-runtime inspired and applies
only to analytix internal HTTP/SSE route oracle and Go G4 shadow evidence. It
does not absorb a Kun product-entry, settings, bridge, provider, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, Reasonix public user-input protocol, or Rust/Tauri path is added.
```

## 2026-06-21 - Task-Job Route Auth Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job route auth matrix batch is Reasonix-runtime inspired and applies
only to analytix internal runtime route oracle evidence. It does not absorb a
Kun product-entry, settings, bridge, provider, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, Reasonix public job protocol, or Rust/Tauri path is added.
```

## 2026-06-21 - Go G5 Task-Job Approval Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G5 task-job approval shadow replay batch is Reasonix-runtime inspired
and applies only to analytix runtime/G5 shadow evidence. It does not absorb a
Kun product-entry, settings, bridge, provider, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-21 - Go Provider Cache Accounting Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G3/G5 provider cache accounting batch is Reasonix-runtime inspired and
applies only to analytix runtime/provider-cache shadow evidence. It does not
absorb a Kun product-entry, settings, bridge, provider preset, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, Reasonix provider protocol, or Rust/Tauri path is added.
```

## 2026-06-21 - Auto-Router Failure Isolation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-router failure usage isolation batch is Reasonix-runtime inspired and
applies only to analytix internal runtime routing/accounting evidence. It does
not absorb a Kun product-entry, settings, bridge, provider preset, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, Reasonix auto-plan config, default Go
backend, renderer-visible Go route, or Rust/Tauri path is added.
```

## 2026-06-21 - Go G5 Approval Executable Control Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G5 task approval executable control batch is Reasonix-runtime inspired
and applies only to analytix runtime/G5 shadow evidence. It does not absorb a
Kun product-entry, settings, bridge, provider, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-21 - Runtime Provider Selection Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider-selection request-shape oracle is a runtime non-regression proof,
not a Kun product-entry or settings migration. It keeps analytix's provider
selection behind the bundled runtime client boundary.

Boundary:

```text
Thread provider selection may choose a custom full endpoint or fall back to the
default OpenAI-compatible provider, but it does not add Kun provider identity,
Kun public protocol, deprecated bridge alias, old agent settings envelope,
default Go backend, Rust/Tauri path, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation.
```

## 2026-06-21 - Scheduled Detector Endpoint Inference Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The scheduled detector custom endpoint inference batch is a backend
non-regression proof for Schedule's model consumer. It does not absorb a Kun
product-entry, settings, bridge, provider identity, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Schedule may infer `/messages` and `/chat/completions` request families from
custom full endpoint URLs, but it does not add Kun provider identity, Kun public
protocol, deprecated bridge alias, old agent settings envelope, default Go
backend, Rust/Tauri path, or top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer navigation.
```

## 2026-06-21 - Forbidden Runtime Routes Preserve Kun Entry Baseline

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The forbidden public runtime route oracle converts the Kun entry baseline into
HTTP router evidence. It does not add a Kun feature; it prevents forbidden
top-level capabilities from becoming public runtime routes.

Boundary:

```text
Runtime HTTP routes for Workflow, Create Loop, Subagent, AutoResearch, and
MCP-indexer return 404, including `/v1/workflow` and `/v1/mcp-indexer`; `/v1/runtime/go` also
returns 404. Kun public protocol, deprecated bridge alias, old agent settings
envelope, Kun identity, default Go backend, renderer-visible Go route, and
Rust/Tauri path remain absent.
```

## 2026-06-21 - Rehydrated Task-Job Routes Have No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The rehydrated task-job route oracle is internal runtime evidence. It does not
add a Kun product-entry, public route, bridge, provider identity, Connect Phone,
Local Whisper, tray, or packaging delta.

Boundary:

```text
Rehydrated jobs may be controlled only through authenticated
`/v1/runtime/task-jobs/output`, `/v1/runtime/task-jobs/wait`, and
`/v1/runtime/task-jobs/kill`. Top-level Subagent, Workflow, Create Loop,
AutoResearch, and MCP-indexer routes remain rejected; Kun public protocol,
deprecated bridge alias, default Go backend, renderer-visible Go route, and
Rust/Tauri path remain absent.
```

## 2026-06-21 - Task-Job Approval Denial Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task-job approval denial oracle is internal runtime safety evidence. It
does not add a Kun product-entry, public route, bridge, provider identity,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Denied `task` and `parallel_tasks` calls create no durable jobs or child runs.
Top-level Subagent, Workflow, Create Loop, AutoResearch, and MCP-indexer routes
remain rejected; Kun public protocol, deprecated bridge alias, default Go
backend, renderer-visible Go route, and Rust/Tauri path remain absent.
```

## 2026-06-21 - Preload Bridge/API Sovereignty Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The preload bridge/API sovereignty batch does not absorb a Kun product feature.
It turns the existing analytix ownership boundary into executable tests so Kun
lineage or future Kun-current imports cannot reintroduce `window.kun`,
`window.kunGui`, old GUI facade names, or old settings fallback paths.

Boundary:

```text
Renderer access remains `window.analytix` only, and the renderer `Window` type
declares only `analytix`. Kun legacy session import IPC may remain internal
compatibility plumbing, but no Kun bridge alias, Kun public protocol,
deprecated settings fallback, top-level Workflow/Create Loop/Subagent/
AutoResearch/MCP-indexer entry, default Go backend, or Rust/Tauri path is
added.
```

## 2026-06-21 - Renderer Thread Lifecycle Oracle Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The renderer thread lifecycle HTTP oracle does not absorb a Kun product
feature. It proves existing analytix thread actions remain on analytix-owned
HTTP routes and cannot quietly drift back to Kun public protocol or legacy
bridge assumptions.

Boundary:

```text
Thread list/search/archive/restore/rename/workspace/delete use
`window.analytix` -> `/v1/threads` analytix runtime paths. The Kun v0.2.13 ->
v0.2.14 entry baseline remains Code/Write/Settings/Plugins/Connect Phone/
Schedule. No Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level
entry, Kun identity, deprecated bridge alias, old runtime settings write path,
Kun public protocol, default Go backend, or Rust/Tauri path is added.
```

## 2026-06-21 - Settings/Provider EndpointFormat Persistence Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The settings/provider endpoint-format persistence oracle does not absorb a Kun
product feature. It proves provider endpoint choices stay in the analytix
top-level `runtime` and `provider.providers` schema, not old Kun/agent settings
envelopes.

Boundary:

```text
Endpoint format persistence uses top-level `runtime` and `provider.providers`.
The Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Provider Request-Shape Oracle Matrix Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider request-shape oracle matrix is a Reasonix-provider discipline
proof at the analytix runtime conformance boundary. It does not absorb a Kun
product-entry, settings, bridge, provider default, Connect Phone, Local Whisper,
tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Auto-Model Route Cache Lifecycle Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-model route cache lifecycle proof is Reasonix-currentness inspired and
applies only to analytix runtime loop tests plus docs. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Write-Inline Custom Full Endpoint Proof Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The write-inline custom full endpoint proof is a provider non-regression test
batch at the analytix main-process Write boundary. It does not absorb a Kun
product-entry, settings, bridge, Connect Phone, Local Whisper, tray, or
packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-21 - Go G5 Planner Tool-Policy Executable Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The Go G5 planner tool-policy executable shadow batch is Reasonix-runtime
inspired and applies only to analytix conformance fixtures plus Go shadow code.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, or default Go backend is added.
```

## 2026-06-21 - Write-Inline Provider Tests Have No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The write-inline provider request-surface proof is a provider non-regression
test batch at the analytix main-process service boundary. It does not absorb a
Kun product-entry, settings, bridge, Connect Phone, Local Whisper, tray, or
packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, or default Go backend is added.
```

## 2026-06-21 - Planner Task-Tool Gating Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The planner task-tool gating batch is Reasonix-runtime inspired and applies
only to analytix internal runtime tool policy. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, or Kun public protocol is added.
```

## 2026-06-21 - User-Input Live-Card Evidence Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The user-input live-card store evidence batch is a Reasonix-runtime cleanup
proof at the analytix renderer/store boundary. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, or Kun public protocol is added.
```

## 2026-06-21 - Top-Level Route-Surface Oracle Preserves Kun Entry Baseline

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The route-surface oracle batch does not absorb a new Kun feature. It converts
the existing Kun v0.2.13 -> v0.2.14 product-entry boundary into executable
renderer tests so future Reasonix or Kun-current absorption cannot accidentally
promote hidden/internal capabilities into top-level navigation.

Boundary:

```text
`AppRoute`, app actions, Workbench stage rendering, shell navigation, and
sidebar active views remain limited to the current analytix product surfaces:
Code/chat, Write, Settings, Plugins, Connect Phone, and Schedule. Workflow,
Create Loop, Subagent, AutoResearch, and MCP-indexer remain rejected as
top-level entries for the Kun target-version baseline; internal runtime code
may remain only when hidden behind analytix-owned contracts.
```

## 2026-06-21 - Auto-Route Step/Cancel Composition Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-route step/cancel composition batch is Reasonix-runtime inspired and
applies only to analytix runtime/G5 shadow evidence. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Auto-Router Classifier Request Contract Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The auto-router classifier request contract batch is Reasonix-runtime inspired
and applies only to analytix runtime test evidence. It does not absorb a Kun
product-entry, settings, bridge, provider, Connect Phone, Local Whisper, tray,
or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Planner-Executor Transcript Propagation Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The planner-executor transcript propagation batch is Reasonix-runtime inspired
and applies only to analytix internal runtime/job metadata and G5 shadow
evidence. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - G5 Task Tool Contract Boundary Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task tool-contract boundary replay batch is Reasonix-runtime inspired and
applies only to analytix internal task-job evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - G5 Planner-Executor Detail Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The planner-executor detail replay batch is Reasonix-runtime inspired and
applies only to analytix internal task-job evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - G5 Task Parent Goal Evidence Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The task parent-goal evidence replay batch is Reasonix-runtime inspired and
applies only to analytix internal task-job evidence plus Go G5 shadow output.
It does not absorb a Kun product-entry, settings, bridge, provider, Connect
Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - MCP Search Meta-Tool Trust Boundary Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The MCP search meta-tool trust-boundary batch is Reasonix-runtime inspired and
applies only to analytix internal MCP/tool lifecycle evidence plus G4 shadow
fixtures. It does not absorb a Kun product-entry, settings, bridge, provider,
Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Custom Messages Full Endpoint Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The custom `/messages` full endpoint batch is provider request-shape
non-regression evidence. It does not absorb a Kun product-entry, settings,
bridge, provider default, Connect Phone, Local Whisper, tray, or packaging
delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, or Rust/Tauri
path is added.
```

## 2026-06-22 - Go G5 Approval/User-Input Abort Cleanup Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The abort-cleanup executable shadow batch is Reasonix-runtime inspired and
applies only to analytix approval/user-input conformance evidence plus Go G5
shadow output. It does not absorb a Kun product-entry, settings, bridge,
provider, Connect Phone, Local Whisper, tray, or packaging delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Cache Coverage Floor Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider/cache coverage-floor batch is Reasonix-runtime inspired and
applies only to analytix runtime fixtures, schema validation, and Go shadow
inputs. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - G5 Provider Cache Coverage Floor Shadow Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The G5 provider/cache coverage-floor shadow batch is Reasonix-runtime inspired
and applies only to TypeScript fixtures plus Go G5 shadow output. It does not
absorb a Kun product-entry, settings, bridge, provider default, Connect Phone,
Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Cache Accounting Raw Payload Replay Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider/cache raw accounting replay batch is Reasonix-runtime inspired and
applies only to analytix runtime fixtures, conformance tests, and Go shadow
output. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Provider Raw Accounting Proof Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The provider raw accounting proof-freshness batch is Reasonix-runtime inspired
and applies only to analytix provider-cache tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Combined Step/Cancel/Cache Proof Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The combined step/cancel/cache proof-freshness batch is Reasonix-runtime
inspired and applies only to analytix G5 conformance tests plus
product-sovereignty scan tokens. It does not absorb a Kun product-entry,
settings, bridge, provider default, Connect Phone, Local Whisper, tray,
packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```

## 2026-06-22 - Approval/User-Input Proof Freshness Has No Kun Product Delta

Source:

```text
Kun target refs remain unchanged from the provider-context correction section.
```

Decision:

The approval/user-input proof-freshness batch is Reasonix-runtime inspired and
applies only to analytix G5 conformance tests plus product-sovereignty scan
tokens. It does not absorb a Kun product-entry, settings, bridge, provider
default, Connect Phone, Local Whisper, tray, packaging, or UI delta.

Boundary:

```text
Kun v0.2.13 -> v0.2.14 entry baseline remains Code/Write/Settings/Plugins/
Connect Phone/Schedule. No Workflow/Create Loop/Subagent/AutoResearch/
MCP-indexer top-level entry, Kun identity, deprecated bridge alias, old runtime
settings write path, Kun public protocol, default Go backend, renderer-visible
Go route, or Rust/Tauri path is added.
```
