# Reasonix-to-Analytix Integration Topology

Generated on 2026-06-23 during the Go runtime final landing pass.

## Status

- Reasonix integration topology is code-level runtime metadata, exposed only
  through `/v1/runtime/info.capabilities.upstreamAbsorption.reasonixIntegrationTopology`.
- Current Go runtime default delivery is active through `go-runtime-default`.
  `ANALYTIX_RUNTIME_BACKEND=typescript` is retired and rejected by the current
  desktop adapter.
- Reasonix integration topology is runtime-info evidence. It is not live provider/MCP/
  packaged/operator evidence and does not authorize physical fallback
  retirement by itself.
- No Reasonix top-level Workflow, Create Loop, Subagent, AutoResearch, or
  MCP-indexer product entrypoint is added.
- No Reasonix public protocol, renderer bridge alias, settings schema root, or
  product identity change is added.
- Live provider/MCP/packaged/operator validation remains post-cutover evidence
  collection and must not be overstated from this topology document.

## Absorbed Engine Topology

Decision method:

1. Build the Kun/Analytix baseline first: entry, trigger path, runtime
   contract, settings, bridge, providers, MCP, `/goal`, tasks, AutoResearch,
   and internal planner surfaces.
2. Compare Reasonix source node by node and absorb only stronger
   engine/runtime behavior.
3. Keep Kun/Analytix where product semantics, UI, settings, bridge, provider
   compatibility, or public protocol would otherwise drift.
4. For conflicts, product surface follows Kun/Analytix while internal engine
   mechanics may absorb Reasonix under Analytix contracts.

Baseline surfaces retained in code-level runtime metadata:

| Baseline | Decision | Protection |
| --- | --- | --- |
| Desktop entry/UI and bridge | Kun/Analytix retained | `window.analytix`, top-level `runtime`, route-surface tests, product-sovereignty scan |
| Multi-model provider model | Kun/Analytix retained | DeepSeek, OpenAI-compatible, Anthropic-compatible, custom endpoint tests |
| MCP registry/tool calling | Kun/Analytix retained | Existing MCP registry/config/approval routes; no MCP-indexer |
| `/goal`, task, AutoResearch, internal planner | Kun/Analytix retained | Existing Chat, `/goal`, tool, SSE, `.analytix/autoresearch` contracts |

| Capability | Reasonix engine source | Analytix existing entry | Go runtime landing | Guard |
| --- | --- | --- | --- | --- |
| DeepSeek cache/prefix telemetry | `internal/agent/cache_shape.go`, `internal/provider/openai/openai.go`, `cmd/e2ebench/main.go` | Chat turn provider path, provider settings, usage events | `packages/runtime-go/internal/provider/provider.go`, `packages/runtime-go/internal/readiness/readiness.go`, `packages/runtime-go/runtime_server.go`, `packages/runtime-go/internal/upstreamaudit/baseline_absorption.go` | DeepSeek-specific enhancement; OpenAI-compatible, Anthropic-compatible, and custom endpoint remain protected |
| Agent loop/job/sub-agent lineage | `internal/agent/task.go`, `internal/agent/subagent_store.go`, `internal/jobs/*` | Chat turns, `/goal`, task/parallel task tools, SSE child lineage | `packages/runtime-go/internal/agent/gates.go`, `packages/runtime-go/internal/jobs/lineage.go`, `packages/runtime-go/runtime_server.go`, `packages/runtime-go/live_production_candidate.go`, `packages/runtime-go/internal/goal/goal_evidence.go` | Internal orchestration only; no Subagent top-level product entry |
| MCP lifecycle/search/call/reconnect/redaction | `internal/plugin/plugin.go`, `lazy.go`, `canonicalize.go`, `known_overrides.go` | Existing MCP tool registry, tool calls, approval/user-input routes, SSE audit | `packages/runtime-go/internal/mcp/lifecycle.go`, `packages/runtime-go/internal/readiness/readiness.go`, `packages/runtime-go/runtime_server.go` | No MCP-indexer top-level product entry |
| AutoResearch project state | `internal/control/input.go`, `internal/evidence/*` | `/goal --research`, `.analytix/autoresearch`, `autoresearch_state_audit` | `packages/runtime-go/internal/research/autoresearch_state.go`, `packages/runtime-go/runtime_server.go` | No AutoResearch page or new public route |
| Workflow/Create Loop planner discipline | `internal/control/controller.go`, `internal/agent/coordinator.go`, `parallel_tasks.go`, `completestep_test.go` | Chat/plan turns, `/goal`, `create_plan`, `complete_step`, existing runtime events | `packages/runtime-go/internal/goal/goal_evidence.go`, `packages/runtime-go/shadow_g5.go`, `packages/runtime-go/runtime_server.go` | Internal planner/runtime capability only; no Workflow/Create Loop navigation |

Code-level comparison conclusions:

| Capability | Conclusion |
| --- | --- |
| DeepSeek cache/prefix telemetry | Conflict: absorb stronger Reasonix engine telemetry, retain Analytix provider/settings/multi-model constants |
| Agent loop/job/sub-agent lineage | Conflict: absorb stronger lineage/orchestration internals, retain Analytix Chat/`/goal`/task entry semantics |
| MCP lifecycle/search/call/reconnect/redaction | Reasonix engine stronger: absorb lifecycle/reconnect/schema/redaction behavior through existing Analytix MCP registry |
| AutoResearch project state | Conflict: absorb project-state discipline, retain `/goal --research` and `.analytix/autoresearch` |
| Workflow/Create Loop planner discipline | Conflict: absorb internal planner discipline, retain hidden/internal Workflow/Create Loop semantics |

## Contract And Evidence Files

- Go topology implementation:
  `packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go`
- Go compatibility shim:
  retired; topology now lives only in the formal upstream audit package.
- Go topology tests:
  `packages/runtime-go/reasonix_integration_topology_test.go`
- Shared TS contract:
  `packages/runtime/src/contracts/capabilities.ts`
- Runtime conformance:
  `packages/runtime/tests/go-runtime-conformance.test.ts`
- Main-process strict G6 gate:
  `src/main/runtime/analytix-adapter.ts`
- Sovereignty scan:
  `scripts/scan-product-sovereignty.cjs`

## Current Post-Cutover Live Validation

Latest strict preflight:
`npm run runtime:go:preflight -- --json --gate`.

Current live evidence aggregate:
`docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json`.

Current passed live evidence rows for the release gate:

- `provider-matrix-credentialed`
- `mcp-matrix-credentialed`
- `packaged-qa`
- `packaged-gui-bridge-smoke`
- `packaged-session-soak`
- `explicit-env-gate`

These passed rows support current live evidence completeness for the release
gate. They do not restore a TypeScript executable fallback; current rollback
means release rollback or a retired-backend diagnostic.
