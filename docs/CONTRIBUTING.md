# Local Maintenance Guide

[简体中文](./CONTRIBUTING.zh-CN.md)

analytix is maintained local-first. This guide defines the expected shape,
review, validation, and record of a change. See
[DEVELOPMENT.md](./DEVELOPMENT.md) for commands and environment setup.

## Start With The Current Truth

- Read every applicable `AGENTS.md` from the root through the directory that
  owns the change; more specific guidance prevails on conflict.
- Use [analytix/README.md](./analytix/README.md) to distinguish active specs,
  historical evidence, generated artifacts, and legacy material.
- Inspect current source, tests, and package scripts before trusting a
  present-tense claim in a dated report.
- The local mainline is `main`. Use `codex/...` by default for a new branch
  unless the maintainer requests another prefix.
- Begin with `git status --short --branch` and preserve unrelated user work.

## Scope A Complete Change

A good change is the smallest **complete** change, not simply the change with
the fewest files. It should:

- address one clear problem or outcome;
- update every contract consumer required by that outcome;
- include focused tests or other appropriate evidence;
- update active documentation when behavior or workflow changes;
- avoid speculative features, aliases, broad refactors, and formatting churn.

For runtime API changes, follow the complete path where applicable: public
schema, Go use case and HTTP/SSE adapter, Electron main/preload, renderer
consumer, and focused contract tests. Do not fix only the visible UI or return
production behavior to the retired TypeScript runtime.

## Validation Expectations

Select evidence that matches the changed surface:

- Markdown-only changes: inspect paths/links and run `git diff --check`.
- TypeScript changes: run `npm run typecheck` and focused tests; add
  `npm run build` when production output can change.
- Go runtime changes: run `gofmt` on changed files and focused
  `go test` under `packages/runtime-go`. Go 1.22 is the language baseline.
- Production-only Go paths: include
  `(cd packages/runtime-go && go test -tags analytix_prod ./...)` when
  applicable.
- UI changes: run the relevant tests and manually inspect the flow with
  `npm run dev`; retain screenshots or video when they materially help
  review.
- Packaging/release changes: run the relevant package audit and release gate
  on the approved platform.

The TypeScript command `npm run build:runtime` does not validate the Go core.
After an RC source freeze, use `npm run runtime:go:rc-control-plane -- --json`
once for the source-bound validation control plane. It does not authorize a
package or release. Use `npm run runtime:go:release-gate` only when the change
reaches the product release path; it exits successfully only when its final
JSON has `passed: true`.

Always report exactly which checks ran. If a check fails, distinguish a new
regression from an existing baseline failure.

## Review Standard

Review should assess:

- correctness and regression risk;
- adherence to accepted specs and current architecture;
- boundary validation for IPC, HTTP/SSE, filesystem, provider, and migration
  input;
- security and redaction;
- UX clarity and accessibility where applicable;
- test/evidence relevance rather than raw test count;
- documentation currentness and migration impact;
- whether every intentional change belongs to the requested outcome and
  unavoidable generated differences were inspected.

## Documentation

Update the closest active document:

- `README.md` / `README.en.md`: product-level setup and usage;
- `docs/DEVELOPMENT.md` / `docs/DEVELOPMENT.zh-CN.md`: local workflow;
- `docs/ANALYTIX_CONFIG.md`: configuration boundaries;
- `docs/analytix/specs/`: accepted target behavior, only when the target
  decision itself changes;
- this guide: maintenance and review standards.

Do not refresh a dated QA or release report and silently present it as current.
Record its commit, environment, commands, and result, or keep it classified as
a historical snapshot.

## Security

Never commit or paste:

- API keys, account tokens, runtime bearer tokens, passwords, private keys, or
  product keys;
- remote-control IDs or live host credentials;
- unredacted provider responses, environment dumps, or machine-private paths.

Use approved secret storage and redacted placeholders. If a live secret was
committed, removing it from the current file is not sufficient: rotate it and
coordinate an intentional history/remotes/backups cleanup.

## Problem Records

Include:

- operating system and version;
- analytix app version from Settings/About;
- tested commit or package identity;
- exact reproduction steps and expected/actual behavior;
- sanitized logs, error text, and screenshots;
- actual desktop/runtime process paths and listener ownership for startup
  issues;
- `GET /health` result and, only when authorized, a sanitized summary of
  `GET /v1/runtime/info`.

The public runtime CLI supports `analytix serve`, not a general version
subcommand. Use Settings/About and authorized runtime diagnostics for version
evidence. Never include the bearer token used for an authenticated runtime-info
request.

## Commits And Collaboration

Use focused Angular-style messages such as:

- `feat(scope): short description`
- `fix(scope): short description`
- `docs(scope): short description`

A change note should explain what changed, why, user-visible or migration
impact, and validation actually performed. Local review and validation are
required even when a remote issue, pull request, or CI workflow is used as an
additional collaboration surface.

Collaborate respectfully and align on direction before making destructive,
protocol-changing, migration-changing, or externally visible decisions.

## License

The project is available under the [Apache License 2.0](../LICENSE). By
contributing, you also agree to the repository's [Contributor License
Agreement](../CLA.md).

Analytix's first-party copyright owner and Project Owner is Guoqin He,
publicly operating under the GitHub account [Eysn0130](https://github.com/Eysn0130).
Contributors retain the rights stated in the CLA for their own Contributions.
