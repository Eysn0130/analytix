# Go Runtime Conformance

Status: Historical compatibility index and dated cutover evidence.
Current as of: 2026-06-25 for the snapshot below, not the current worktree.
Current implementation source: `packages/runtime-go/README.md`, current Go and
desktop code, and the applicable fresh validation/release gate.

Authoritative staged Go runtime conformance tracking lives in:

```text
docs/analytix/upstreams/go-runtime-conformance.md
```

This runtime-path index exists because D-0236 references
`docs/analytix/runtime/go-runtime-conformance.md` directly.

Current status as of 2026-06-25:

```text
Go runtime default delivery is active through go-runtime-default. An unset
ANALYTIX_RUNTIME_BACKEND, plus analytix, go, go-runtime, and go-runtime-default
select the Go runtime server. TypeScript is not a default-path fallback; it is
retired when requested through ANALYTIX_RUNTIME_BACKEND=typescript.

The upstream document and final delivery report record their stages. They are
not permanent authority for the current worktree. Current conformance claims
must identify the code commit and rerun the applicable runtime-go validation
scripts. Live provider, MCP, packaged, operator, and soak evidence remains
separate from deterministic local conformance; missing external evidence must
be reported as missing rather than inferred from an old pass.
```

Historical Runtime-Go Domain Package Organization notes:

```text
Runtime-go was organized into internal domain packages, then the formal
credentialed evidence intake and runtime-go-cutover-report path replaced the
old stage-numbered script names. The deterministic Reasonix superiority matrix
keeps Reasonix strengths limited to engine/runtime domains while Analytix/Kun
keeps product layer, settings, bridge, provider endpoint family, MCP search
boundary, and durable SSE/thread contracts.

Older wording in this repository that says TypeScript remains the default
backend, Go default is blocked, or live evidence is required before local
default startup is historical stage text. It is superseded by the 2026-06-25
Go default delivery and should be read as a live-evidence and
fallback-retirement gate, not as current startup behavior.
```

Evidence:

```text
scripts/runtime-go-engine-absorption-report.mjs
scripts/runtime-go-cutover-report.mjs
scripts/runtime-go-preflight.mjs
scripts/runtime-go-product-regression.mjs
scripts/runtime-go-speed-cache-gate.mjs
scripts/runtime-go-live-validation.mjs
docs/analytix/upstreams/reasonix-integration-topology.md
docs/analytix/upstreams/go-runtime-retirement-checklist.md
docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json
packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go
packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go
packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go
packages/runtime-go/internal/upstreamaudit/baseline_absorption.go
src/main/runtime/analytix-adapter.ts
src/main/runtime/analytix-adapter.test.ts
packages/runtime-go/internal/server/durable_store.go
packages/runtime-go/internal/research/autoresearch_state.go
packages/runtime-go/internal/server/runtime_server.go
packages/runtime-go/runtime_server.go
packages/runtime-go/root_shims_test.go
packages/runtime/tests/go-runtime-conformance.test.ts
scripts/scan-product-sovereignty.cjs
docs/analytix/upstreams/go-runtime-conformance.md
```
