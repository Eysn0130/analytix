# Local Development Workflow

[简体中文](./DEVELOPMENT.zh-CN.md)

> Status: active maintainer guidance. For architecture and document
> currentness, start with [../AGENTS.md](../AGENTS.md) and
> [analytix/README.md](./analytix/README.md). Current source, tests, and package
> scripts override dated reports.

## Baseline

- The local mainline is `main`.
- Optional short-lived branches should use a focused prefix such as
  `codex/...`, `feat/...`, or `fix/...`.
- The desktop shell is Electron + React + TypeScript.
- The only production agent core is Go code under `packages/runtime-go`.
- `packages/runtime` owns public TypeScript contracts/configuration and the
  `analytix serve` launcher; it is not a second production agent runtime.
- `package.json` and the scripts they invoke are authoritative for available
  commands. Do not infer commands from historical docs or packaged copies.

The project is maintained local-first. A remote review or the verification
workflow in `.github/workflows/release.yml` may provide additional evidence,
but it does not replace local, change-scoped validation or produce the official
desktop packages.

## Prerequisites

- Node.js and npm compatible with the lockfile. The current verification
  workflow uses Node.js 22.
- Go 1.22 or a newer compatible toolchain when editing or validating
  `packages/runtime-go`.
- Platform toolchains only for the affected surface: Rust for native data
  tools, Python for the packaged analysis backend, and the approved Windows or
  macOS build environment for official packages.

Install exact JavaScript dependencies with:

```bash
npm ci
```

## Workflow

1. Inspect `git status --short --branch` and preserve existing user changes.
2. Identify the active source of truth, affected contracts, and success
   criteria.
3. Create a short-lived branch when useful.
4. Implement the smallest complete change; include every required consumer and
   exclude unrelated cleanup.
5. Run the smallest checks that prove the changed behavior.
6. Review the diff, documentation impact, and `git diff --check`.
7. Commit locally only when requested or useful to the maintainer workflow.

## Validation

Choose checks by changed surface rather than running every command
mechanically.

| Changed surface | Expected evidence |
| --- | --- |
| Markdown only | inspect paths/links and run `git diff --check` |
| TypeScript contract or desktop code | `npm run typecheck`, focused tests, and `npm run build` when production output changes |
| Go runtime | `gofmt` on changed Go files and focused `go test`; add the production tag for production-only paths |
| Cross-layer runtime API | public contract, Go adapter/use case, desktop bridge/consumer tests, typecheck, and build |
| UI behavior | focused tests plus `npm run dev` and manual verification; retain a screenshot/video when it helps review |
| Packaging or release | platform package audit and the relevant release gate on the approved host |

Common JavaScript checks:

```bash
npm run typecheck
npm run test
npm run build
npm run lint
git diff --check
```

`npm run build:runtime` builds `packages/runtime`, including the launcher and
public TypeScript contracts. It does **not** compile or test the Go core.

Common Go checks:

```bash
(cd packages/runtime-go && go test ./...)
(cd packages/runtime-go && go test -tags analytix_prod ./...)
```

Format only the Go files in scope, for example:

```bash
gofmt -w packages/runtime-go/internal/app/model/example.go
```

After an RC source freeze, run
`npm run runtime:go:rc-control-plane -- --json` once for the source-bound
validation control plane. It may pass while the product RC remains blocked and
never authorizes package or release. Use `npm run runtime:go:release-gate` for
product release-path runtime changes; it succeeds only when the final JSON has
`passed: true`. Neither command substitutes for focused local tests.

If any check fails, state whether the failure is caused by the current change
or is an existing baseline issue. Never report an unrun or failing check as
passing.

## Manual Runtime Checks

Start the development app with:

```bash
npm run dev
```

For startup diagnosis, combine logs, process ownership, listener ownership, and
the public health endpoint:

```bash
curl http://127.0.0.1:<runtime-port>/health
```

`GET /v1/runtime/info` requires the runtime bearer token unless the runtime was
explicitly started in insecure mode. Never paste that token into a problem
report. The public CLI surface supports `analytix serve`; use Settings/About
and runtime diagnostics for version evidence instead of assuming another CLI
subcommand.

## Local Releases

Development packaging commands include:

```bash
npm run dist
npm run dist:mac
npm run dist:win
npm run dist:linux
```

Maintainer release entry points are:

```bash
npm run release:mac
npm run release:win
```

- Official Windows packaging runs on the approved Windows host.
- `npm run release:win` drives `scripts/release-win.ps1`, creates the
  Standard Windows bootstrapper, runs the package audit, and follows the cache,
  archive, channel, signing, and optional R2 settings implemented by that
  script.
- Authenticode evidence is optional unless
  `ANALYTIX_REQUIRE_WINDOWS_AUTHENTICODE=1` is set. Native binaries, the
  single-owner data-engine gate, and forbidden-file checks remain mandatory.
- Generated `dist*/`, `output/`, package extractions, reports, and local
  release metadata are evidence or artifacts, not implementation sources.

Machine-specific workspaces, caches, archives, certificates, and publishing
credentials belong in approved local configuration. Do not copy them into
maintainer documentation.

## Change Notes

A useful change note records:

- what changed and why;
- user-visible or compatibility impact;
- commands and manual checks actually run;
- known baseline failures or unverified external requirements;
- screenshots or video for interaction changes when appropriate.

Use focused Angular-style commit messages, for example:

- `fix(runtime): preserve SSE replay cursor`
- `feat(settings): add provider endpoint format`
- `docs(development): correct Go validation path`
