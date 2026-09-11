# analytix Go Runtime Agent Guide

This guide adds production-runtime guidance for `packages/runtime-go/`.
Inherit the root `AGENTS.md`.

## Ownership

The Go runtime is the only production agent core. Production composition starts
under `internal/runtimeapp`; the executable starts at
`cmd/runtime-server/main.go`.

Place new separable behavior at the responsibility it serves:

- values and invariants: `internal/domain`;
- use cases and orchestration: `internal/app`;
- side-effect interfaces: `internal/ports`;
- HTTP/SSE input: `internal/adapters/inbound`;
- filesystem, persistence, provider, MCP, process, and other effects:
  `internal/adapters/outbound`;
- production assembly: `internal/runtimeapp`.

`internal/server` and `internal/provider` contain transitional compatibility.
A focused fix may follow their current ownership; new separable behavior belongs
at the responsibility above. Confirm the live call path before moving or
retiring compatibility code.

## Contracts And Trust

- Public schemas are owned in `packages/runtime/src/contracts`. Keep Go-private
  routes and structures behind the public HTTP/SSE contract.
- For a runtime API change, follow the applicable cross-layer slice in the root
  and `src/AGENTS.md`. Exercise malformed input, authorization, failure
  projection, restart, and replay when those behaviors are relevant.
- Provider requests and responses, tool calls and results, filesystem input,
  MCP output, and persisted records are untrusted. Preserve enough structured
  evidence to diagnose failures without logging secrets or unsafe bodies.
- Keep provider endpoint selection separate from request-body construction.
  Endpoint format covers headers, body, streaming, usage, cache, errors, and
  reasoning behavior across the live path.
- Cache correctness relies on a stable system prefix, canonical tool schemas,
  valid tool-call/result pairing, and trustworthy provider usage fields;
  mutable turn context follows the stable prefix.

## Execution And Agent Authority

Execution policy, unattended operation, approvals, child agents, writable
worktrees, goals, todos, and evidence closure are security and public-product
contracts. Resolve exact current defaults and migration behavior from accepted
specs, production code, and focused tests rather than duplicating them here.

Preserve these outcomes unless an accepted design explicitly supersedes them:

- authority is explicit and cannot be widened by a convenience fallback;
- unattended work never depends on an unavailable approver;
- child work is isolated, attributable, reviewable, and recoverable;
- child results do not silently close parent-owned work or evidence;
- application `workspace-write` policy is not represented as OS containment.

## Persistence, Replay, And Diagnosis

For memory growth, replay, duplicate event, sequence, compaction, or restart
symptoms, inspect the coupled lifecycle: in-memory ownership, durable logs,
sequence recovery, SSE cursor behavior, cleanup, process restart, and renderer
projection. A single array size, file size, port check, or passing unit test is
not enough when the reported behavior crosses those seams.

Build the tightest feasible signal for the user's exact symptom, minimize it,
and rerun it after the change. Persistence work should prove both the live and
restart paths when restart can change the result.

## Validation

Use the Go language level declared by `go.mod` and a compatible toolchain.
Source `./scripts/use-analytix-cache.sh` in the same shell, format changed Go
files, and select evidence proportionate to the changed seam:

```bash
gofmt -w <changed-go-files>
(cd packages/runtime-go && go test <focused-package-or-test>)
(cd packages/runtime-go && go test ./...)
(cd packages/runtime-go && go test -tags analytix_prod ./...)
```

Focused tests normally precede full suites. Use the `analytix_prod` tag when
production composition is gated by it, and
focused regression tests for release-path code changes. Run
`npm run runtime:go:release-gate` when the authorized task requires formal
release readiness, with its prerequisites and permissions satisfied. TypeScript
`npm run build:runtime` does not validate the Go core; see
`docs/analytix/development-runbook.md` for wider desktop and packaging gates.
