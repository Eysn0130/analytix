# analytix TypeScript Runtime Package Agent Guide

This guide adds package-specific guidance for `packages/runtime/`. Inherit the
root `AGENTS.md`.

## Production Role

This package owns public runtime schemas and TypeScript contracts, runtime
configuration, telemetry, stable shared utilities, and the public
`analytix serve` launcher. The launcher resolves and starts the platform Go
`runtime-server`.

The shipped TypeScript surface is defined by `package.json` exports and
`tsconfig.build.json`, not by directory names. Retained loop, model, server,
conformance, migration, and test-support sources are behavior or compatibility
oracles unless the live import graph proves otherwise; they are not a second
production agent runtime.

## Contract Work

- Treat schema changes as cross-runtime contracts. Trace the current Go
  producer or consumer, transport, desktop bridge, renderer consumer,
  persistence compatibility, and fixtures that actually use the field.
- A field being accepted, serialized, or retained does not establish active
  production behavior. Confirm the Go composition and use case.
- Keep launcher behavior narrow: preserve public CLI semantics, start the
  packaged platform binary, and surface startup failures accurately. The
  packaged app must not depend on user-installed Go or bundled Go source.
- Confirm the import graph before classifying a source helper as retired; the
  Electron main process still consumes selected helpers from this package.
- Preserve byte-stable system-prefix and canonical tool-schema behavior where
  cache correctness depends on it; mutable workspace, file, tool-result, and
  temporal context stays outside that prefix.

Prefer a contract-first vertical slice: update the independent public
expectation, implement the current Go behavior, propagate to observable desktop
consumers, and verify compatibility and failure behavior at the public seam.
Close Go gaps in the Go production path rather than masking them with a
TypeScript runtime fallback.

## Validation

The following commands are a menu, not a mandatory sequence. Choose the
smallest applicable checks (including focused test selectors) and source
`./scripts/use-analytix-cache.sh` in the same shell:

```bash
npm --prefix packages/runtime run typecheck
npm --prefix packages/runtime run test
npm --prefix packages/runtime run build
npm run build:runtime
```

Use either the package build or its root wrapper when they cover the same
output. `npm run build:runtime` validates this TypeScript package, not the Go core. Add
focused Go and desktop evidence when a contract crosses those seams.
