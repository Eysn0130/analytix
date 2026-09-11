# Post-881 Sub-Agent Review Evidence - 2026-06-22

This file records the six-lane sub-agent review for the Reasonix
`881b2f2f..9ada14176629b1d59d7ed78446951b2bb5954904` absorption stream.
It is a machine-scannable QA matrix, not a replacement for specs, ledgers,
fixtures, tests, or release evidence.

Current-status note (2026-07-02): this is historical stage evidence. Later
specs and ledgers supersede any older default-backend, rollback, or route
boundary wording. `reject` rows here reject the reviewed public/upstream shape,
not a future analytix-native capability that receives its own spec, UX
rationale, tests, desktop QA, and scan update.

## post881SubagentReviewMatrix

| Lane | Review result | Accepted floor | Boundary |
| --- | --- | --- | --- |
| subagentAReasonixAgentKernel | Auto-plan classifier/currentness, planner gating, and step/cancel/cache were already represented by analytix-owned fixtures and G5 control cases. | Fixture/oracle parity for classifier freshness, planner tool narrowing, route-cache reuse, cancel pairing, and dynamic-state exclusion from stable prefix. | No public planner setting, controller API, or Reasonix config root. |
| subagentBKunBaseline | Kun `0.2.13` -> `0.2.14` retained Code, Write, Settings, Plugins, Connect Phone, and Schedule, with dormant Create Loop code quarantined. | Product sovereignty remains guarded by route-surface tests and forbidden scans. | No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry. |
| subagentCRuntimeCache | DeepSeek/OpenAI Responses/Anthropic cache accounting is derived from provider-like raw usage payloads; custom providers have endpoint request-shape-only proof. | DeepSeek prefix stability and cache hit accounting are fixture-level equal or stronger than the Reasonix import path, while OpenAI/Anthropic do not regress and D-0180 seals custom telemetry ids to `[]`. | No credentialed live provider/cache superiority claim. |
| subagentDMcpToolSubagent | Tool approval, user-input, MCP lifecycle, and task/job orchestration are covered by internal runtime oracles. | Denied no-execute, answer privacy, abort cleanup, MCP search/call lifecycle, and task-job route semantics remain analytix-owned. | No Reasonix public Subagent, MCP-indexer, SessionAPI, or job protocol. |
| subagentEGoRuntime | Go G5 control shadow now covers executable slices for provider, route, approval/user-input, MCP, task/job, goal persistence off-lock, and session behavior. | Shadow output deep-equals TS-owned fixtures; D-0181 closes the previous future Go lock fixture gap at source/fixture level. | No live Go backend, renderer-visible Go route, default backend, G6 selection, or rollback readiness. |
| subagentFQaDocs | Specs, upstream docs, scorecard, release evidence, and scans are synchronized through D-0178, with one D-0177 range wording drift corrected here. | Evidence freshness is scan-visible. | Scan freshness is not release readiness or packaged desktop QA. |

## Delta Classification

| Delta / idea | Classification | Analytix decision |
| --- | --- | --- |
| Auto-plan classifier fingerprint/currentness drift | contract-reimplement | Keep inside analytix runtime classifier/currentness fixtures and scan proof tokens. |
| Planner enable/disable behavior | contract-reimplement | Preserve explicit plan-mode and planner tool gating; reject public upstream planner toggles. |
| Step/cancel/cache combined stability | contract-reimplement | Keep route cache, cancel aggregation, and stable-prefix isolation in G5 oracle proof. |
| Provider cache accounting | code-port-and-adapt | Derive DeepSeek/OpenAI Responses/Anthropic accounting from raw provider-like payloads. |
| customProviderRequestShapeOnly | contract-reimplement | Custom provider proof covers exact full-endpoint URL/body/header shape, not independent custom cache telemetry; D-0180 adds `customFullEndpointTelemetryCaseIds: []`. |
| MCP lifecycle and tool approval annotations | code-port-and-adapt | Keep `mcp_search`, `mcp_describe`, `mcp_call`, approvals, cancellation, and refresh behavior internal. |
| Sub-agent/task-job orchestration | code-port-and-adapt | Keep `task`, `parallel_tasks`, and task-job routes internal to the analytix runtime contract. |
| Goal persistence off controller lock | code-port-and-adapt | Keep TS goal persistence lock-free and require Go G5 shadow to replay persist-before-event and warning behavior. |
| Reasonix public protocol, SessionAPI, config roots, and route names | reject | Do not expose upstream protocol or product identity. |
| Kun Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer top-level entries | reject | Kun target versions do not justify these entries in analytix navigation. |
| Live Go provider/router/gate/job managers and G6 backend selection | defer | Continue only through G5/G6 gates and rollback evidence. |

## goRuntimeConformanceRangeCorrected

The Go runtime conformance D-0177 prose previously used an older range ending
at D-0176. The script already checked D-0172 through D-0178. This batch
corrects that wording and extends the scanned final-gate expectation to D-0179
after the required validation commands are recorded.

## subagentOpenGates

The six-lane review deliberately leaves these claims open:

```text
No live provider/cache superiority matrix.
No packaged desktop route/SSE/provider/MCP walkthrough.
No live Go thread/session router, provider client, approval/user-input gate
manager, or Job Manager.
No G6 backend selection or rollback readiness.
No release readiness, signing, notarization, Windows installer QA, or live
distribution proof.
```

## Validation Pointers

This review matrix is guarded by:

```text
git diff --check
npm --prefix packages/runtime test
npm run test
npm run typecheck
npm run build:runtime
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```
