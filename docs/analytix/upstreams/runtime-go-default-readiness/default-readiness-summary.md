# Go Runtime Default Readiness Summary

This is the current formal summary for Go runtime default readiness and final
acceptance evidence tracking. Historical D-0252 blocker IDs are archived in
`docs/analytix/upstreams/legacy-stage-evidence-archive.md` and are not current
authorization gates.

Generated at: 2026-06-29T11:51:59.704Z
Status: passed
Go default readiness ready: true
Final acceptance status: ready for the current staged worktree
TypeScript default-path fallback retained: false
TypeScript rollback override retained: false

Current-state note (2026-06-29): Go runtime is the default runtime core.
`ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic in the
current adapter. Physical deletion of remaining historical TypeScript source
remains a separate future authorization tracked under
`docs/analytix/upstreams/runtime-go-retirement/`. This file records the latest
current-worktree default-readiness gate result from
`npm run runtime:go:default-readiness-report -- --gate --json`. Current release
authorization must come from `npm run runtime:go:release-gate`, plus any
required live/operator evidence; after the 2026-06-29 operator authorization,
the current release gate passed on the staged worktree.

## Evidence

- Default readiness report: `npm run runtime:go:default-readiness-report -- --gate --json` returned `passed`; `goDefaultReady:true`, `deterministicDefaultReady:true`, `finalAcceptanceReady:true`, and `finalAcceptanceStatus:"ready"`.
- Preflight final gate: `passed` with `finalGateBlocked:false` and no final gate blockers.
- Live evidence: `docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json` is `passed`; provider, MCP, packaged desktop QA, packaged GUI smoke, packaged session soak, and operator components all passed.
- Product regression matrix: 35 rows passed, including Go runtime default, TypeScript retired-backend diagnostic, provider/settings, MiMo plan mode, tool timeline, MCP, subagent/background jobs, usage, attachments, history/resume/fork, and packaged/dev smoke.
- Speed/cache gate: 17 checks passed, including streaming, cache usage, tool progress, subagent, background jobs, MCP lifecycle/schema cache, approval, attachments/vision, and usage/cost.
- Runtime health smoke: built and started the Go runtime-server with temporary durable roots and verified health, runtime info, runtime tools, product chain, SSE replay, usage, attachments, and production capabilities.
- Product sovereignty scan: passed with no Reasonix public product surface or retired temporary validation capability exposure.
- 2026-06-29 gate update: with `ANALYTIX_RUNTIME_READY=1` and
  `ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1`,
  `npm run runtime:go:release-gate` passed product sovereignty, strict
  preflight, and `go test -tags analytix_prod ./...`.

## Current Gate Result

- `goDefaultReady`: true
- `finalAcceptanceReady`: true
- `finalGateEnabled`: true
- `finalGateBlocked`: false
- `finalGateBlockers`: none
- `operator env gate`: satisfied by `ANALYTIX_RUNTIME_READY=1` and `ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1`
- `credentialSecretsRecorded`: false

## Retained Rollback

- Retired-backend diagnostic: setting `ANALYTIX_RUNTIME_BACKEND=typescript` must return retired_backend rather than start TypeScript.
- Rollback scope: explicit compatibility path only; it is not the default runtime path.
