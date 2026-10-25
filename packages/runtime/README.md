# analytix TypeScript Runtime Boundary Package

Status: Operational package reference.

`packages/runtime` is not the production agent implementation. It provides:

- the public `analytix serve` TypeScript launcher;
- shared HTTP/SSE contracts and Zod schemas;
- launcher/config parsing and telemetry helpers;
- TypeScript conformance and regression oracles used while the Go runtime is
  evolved.

The only production agent core is `packages/runtime-go`. `analytix serve`
starts the Go `runtime-server` and keeps the public ready-marker/CLI boundary
stable. Electron rejects `ANALYTIX_RUNTIME_BACKEND=typescript` as retired;
the public launcher starts Go directly.

See [runtime-go/README.md](../runtime-go/README.md) for production composition,
runtime ownership, and Go internals. See
[ANALYTIX_CONFIG.md](../../docs/ANALYTIX_CONFIG.md) for the distinction between
desktop settings, launcher parsing, synchronized config, and fields consumed by
the Go core.

## Production Surface

The build emitted by `tsconfig.build.json` contains only the public boundary:

```text
src/
  cli/          analytix serve parsing and Go runtime launcher
  config/       shared config schemas and redaction helpers
  contracts/    public HTTP/SSE schemas and types
  telemetry/    shared usage/cache telemetry helpers
  hooks/        schema types retained for config compatibility
```

Directories such as `loop-test-support`, `server-test-support`,
`model-test-support`, `tool-test-support`, `services-test-support`, and
`conformance` are test/oracle sources. The retired TypeScript loop, server,
model client, tool host, delegation, and review implementations are migration
references and must not receive new production agent behavior.

Do not generalize that rule to every file outside `tsconfig.build.json`.
Electron main currently imports a small number of source helpers directly,
including `adapters/file/atomic-write.ts` and
`adapters/computer-use/backend-factory.ts`. Their desktop use is real even
though they are not part of the published package build. Check the import graph
before classifying another adapter/service as retired.

## Scripts

Run from `packages/runtime`:

```bash
npm run typecheck
npm run test
npm run build
npm run serve -- --data-dir /path/to/isolated-data
```

For `serve`, supply runtime authentication as described below and use an
isolated data directory.

`npm run build` builds this TypeScript launcher/contracts package. It does not
compile or validate `packages/runtime-go`. Use focused Go tests for Go changes:

```bash
(cd ../runtime-go && go test ./...)
```

Production-sensitive Go changes may also require:

```bash
(cd ../runtime-go && go test -tags analytix_prod ./...)
```

## CLI Boundary

The supported public command is:

```text
analytix serve [options]
```

There is no public `run`, `chat`, `exec`, or `--version` command in the current
dispatcher.

Effective launcher options include:

| Flag | Meaning | Default |
| --- | --- | --- |
| `--config <path>` | Launcher JSON config; its path is also passed to Go for supported settings | `<data-dir>/config.json` when present |
| `--host <host>` | Runtime bind host | `127.0.0.1` |
| `--port <port>` | Runtime port | `8899` |
| `--data-dir <path>` | Runtime durable data root | required by `serve` |
| `--approval-policy <policy>` | Default tool approval policy | `on-request` |
| `--sandbox-mode <mode>` | Default application tool/path policy | `workspace-write` |
| `--insecure` | Disable bearer-token auth for explicit local development | off |

`--runtime-token` / `--runtimeToken` and `--api-key` / `--apiKey` are rejected
in argv. Supply the runtime bearer token through `ANALYTIX_RUNTIME_TOKEN`;
the parser still reads existing `serve.runtimeToken` config for compatibility.
A token is required unless `--insecure` is explicitly selected.

Provider credentials use the local data-directory Registry / Secret Store.
The parser rejects non-blank `ANALYTIX_API_KEY` or `serve.apiKey`, and any
`apiKey` field on provider entries in `ANALYTIX_MODEL_PROVIDERS` or config
`modelProviders`.
`--base-url`, `--model-proxy-url`, `--endpoint-format`, and `--model` remain
legacy parse-only inputs and are not forwarded as Go Provider configuration.

Example with `ANALYTIX_RUNTIME_TOKEN` already supplied through the environment
and an isolated data directory:

```bash
analytix serve \
  --host 127.0.0.1 \
  --port 8899 \
  --data-dir /path/to/isolated-data
```

The desktop normally starts the Go binary directly through Electron main after
synchronizing GUI-managed config. Packaged apps ship a platform-native
`runtime-server`; they do not depend on an installed Go toolchain or bundled Go
source.

The launcher and new Go threads default to `approvalPolicy: on-request` plus
`sandboxMode: workspace-write`. Workspace-write is an application-level
tool/path policy, not an OS sandbox; it blocks foreground and background host
shell execution. Full host shell and unrestricted file access require an
explicit `danger-full-access` selection. Desktop normalization migrates only
the exact unversioned legacy `auto` plus `danger-full-access` pair once; other
valid combinations and a version-2 full-access opt-in remain valid. Unattended
Connect Phone and scheduled-task turns use `never` plus `workspace-write`.

## Configuration Boundary

Do not infer production behavior from schema acceptance alone.

- The launcher parses supported serve options from its JSON config and passes
  the config path to Go as `--mcp-config-path`; legacy Provider CLI defaults
  are not forwarded.
- Electron main synchronizes `<dataDir>/config.json` and passes that path to
  the Go core for currently wired capability settings.
- The Go production composition root is
  `packages/runtime-go/internal/runtimeapp/app.go`. A key is production-active
  only when it is traced from config/launch input into that composition and a
  Go use case/adapter.
- Some TypeScript-era schema keys remain accepted or synchronized for
  compatibility but are not currently consumed by the Go production path.
  These include command-hook/quality execution, SQLite/hybrid storage
  selection, model-based context-compaction settings, token-economy behavior,
  and configurable tool-storm/argument-repair behavior. Treat them as an
  implementation gap, not an active feature.

The ordinary packaged desktop path uses local Provider onboarding and the
data-directory-scoped Registry / Secret Store authority. Provider credentials
must not be written into desktop settings, `config.json`, examples, or logs.
Hub is explicit lazy compatibility only; it is not a default Provider or
credential fallback. See [configuration](../../docs/ANALYTIX_CONFIG.md).

## Runtime Data And API Ownership

The Go core owns durable runtime data under `<dataDir>`, including thread JSONL
logs, events, attachments, memory, task jobs, and runtime-owned worktrees. Do
not use old TypeScript SQLite/index layouts as current production guidance.

For current routes and implementation, inspect:

- `packages/runtime-go/internal/runtimeapp/app.go`
- `packages/runtime-go/internal/adapters/inbound/httpapi/`
- `packages/runtime-go/internal/adapters/inbound/sse/`
- `packages/runtime-go/internal/adapters/outbound/eventlog/`
- `packages/runtime-go/internal/adapters/outbound/filestore/`

The renderer/main boundary must continue to speak the public contracts in this
package. A Go implementation detail or conformance route must not become a new
renderer-visible product protocol.

## Troubleshooting

- Launcher cannot find Go runtime: set a valid packaged binary path through the
  supported runtime environment or run from a checkout containing
  `packages/runtime-go/go.mod` with a compatible Go toolchain.
- Runtime is unreachable: inspect the actual child process, exit status,
  listener ownership, `/health`, and `/v1/runtime/info` together. The launcher
  suppresses raw child output; do not require an unsafe stderr tail as evidence.
- Provider 404: verify selected provider, model, Base URL, endpoint format, and
  final sanitized request URL.
- Config appears ignored: determine whether the field is only accepted/written
  by the TypeScript schema or is actually loaded by `runtimeapp` and the
  relevant Go service.
