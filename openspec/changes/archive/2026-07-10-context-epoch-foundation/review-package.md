# Context Epoch Foundation Review Package

## Metadata

- review_id: `context-epoch-foundation-implementation-2026-07-10`
- reviewed_artifact: `openspec/changes/context-epoch-foundation`
- task_brief: `tasks.md` sections 2 through 6
- report_path: `hard-gate-evidence.md`
- reviewer: Codex
- status: pass
- reviewed_at: 2026-07-10

## Review Inputs

- task brief: `tasks.md`
- report file: `hard-gate-evidence.md`
- changed docs: `docs/analytix/upstreams/absorption-targets.md` and
  `docs/analytix/upstreams/upstream-capability-audit-2026-07-10.md`
- matrix rows: `context.epoch`, `cache.prefix-shape`,
  `compact.post-recovery`, and future context consumers
- gate evidence: `npm run runtime:go:speed-cache-gate -- --json`
- progress ledger: the parent Goal and unified
  `openspec/changes/case-evidence-publication-gate/tasks.md`; no subagent was
  dispatched for this implementation slice

## Implementation Evidence

| Contract | Implementation | Deterministic evidence |
| --- | --- | --- |
| Versioned source registry, boundaries, snapshots, digests, strict parse | `packages/runtime-go/internal/domain/contextepoch/types.go` | `packages/runtime-go/internal/domain/contextepoch/types_test.go` |
| Register/update/remove, safe reconcile, impact/reason codes, bounded reads | `packages/runtime-go/internal/app/contextepoch/service.go` and `packages/runtime-go/internal/ports/contextsource/reader.go` | `packages/runtime-go/internal/app/contextepoch/service_test.go` |
| One shared turn-security/context epoch and durable start records | `packages/runtime-go/internal/app/contextepoch/turn.go`, `packages/runtime-go/internal/app/thread/turn_append.go`, and `packages/runtime-go/internal/server/turn_start.go` | `packages/runtime-go/internal/app/contextepoch/turn_test.go` and the real runtime request fixture in `scripts/runtime-go-performance-check.mjs` |
| Prompt-invisible default; bounded stable/dynamic/turn-tail construction | `packages/runtime-go/internal/app/model/context_epoch.go` and the thin call in `packages/runtime-go/internal/server/agent_loop.go` | `packages/runtime-go/internal/app/model/context_epoch_test.go` plus the before/after request body digest in `hard-gate-evidence.md` |
| Sanitized diagnostics and unknown-cache semantics | `packages/runtime-go/internal/app/contextepoch/diagnostics.go`, `packages/runtime-go/internal/app/usage/prefix_shape.go`, and `packages/runtime-go/internal/server/turn_finalize.go` | `TestRuntimeCacheDiagnosticsIncludesSanitizedContextEpochReasons` |
| Compact recovery and no repeated compact loop | `packages/runtime-go/internal/app/contextepoch/compaction.go`, `packages/runtime-go/internal/app/turn/compaction.go`, and `packages/runtime-go/internal/server/durable_turns.go` | `TestCompactionRecoveryDigestDoesNotCreateRepeatedLoop`, `TestBuildCompactionDoesNotRecompactItsOwnSummaryLoop`, and `TestDurableCompactionAdvancesContextEpochOnceAndPersistsRecoveryDigest` |
| Restart restore, unavailable downgrade, corrupt-state fail closed | `packages/runtime-go/internal/app/contextepoch/recovery.go` and `packages/runtime-go/internal/server/runtime_restore.go` | `TestRestartMarksUnreadableActiveSourceUnavailableOnce`, `TestRuntimeRestoreMarksUnreadableContextSourceUnavailableWithoutLoop`, and `TestRuntimeRestoreRejectsCorruptContextEpochState` |
| Fork/resume cross-thread reset and runtime-owned state | `packages/runtime-go/internal/app/thread/fork_projection.go` and `packages/runtime-go/internal/app/thread/lifecycle.go` | `packages/runtime-go/internal/app/thread/fork_projection_test.go` and `lifecycle_test.go` |

## Required Checks

| Check | Verdict | Evidence |
| --- | --- | --- |
| The report answers the brief and does not widen scope. | pass | No memory, instruction discovery, MCP lifecycle, UI, hook, or plugin feature was added. |
| All claims cite exact evidence paths. | pass | Implementation Evidence table above. |
| Capability ids match the absorption taxonomy. | pass | `context.epoch`, `cache.prefix-shape`, and `compact.post-recovery`. |
| Prompt boundary is explicit. | pass | Six closed boundary values in `internal/domain/contextepoch/types.go`. |
| Required row-level gates are named. | pass | Hard Gate Evidence and the speed/cache gate. |
| Hard Gate Evidence is complete or correctly blocked. | pass | Before/after default fields are complete; credentialed live-provider superiority remains explicitly unclaimed. |
| Subagent handoff used bounded files only. | not-applicable | This slice was implemented and reviewed in the parent lane. |
| Full docs, transcripts, and local diagnostics were not prompt-injected. | pass | Registry metadata stays store-only; selected content alone becomes JSON-encoded untrusted data. |
| Superpowers remains reference-only unless plugin admission is accepted. | pass | It was not installed or invoked. |
| Rejected upstream behavior is explicit. | pass | Rejected: workspace-global registries, ad hoc every-turn reads, raw registry/path injection, OpenCode product protocols, TypeScript runtime restoration, hooks, and mid-stream mutation. |
| Next action is unambiguous. | pass | Future memory, instruction, tool-output, inbox, plugin/skill, MCP, and UI work must cite this evidence and add its own source adapter/admission proof. |

## Validation Results

- `go test ./...`: passed.
- `go test -tags analytix_prod ./...`: passed.
- `go test -race ./...`: passed after replacing the unrelated MCP wall-clock
  parallelism threshold with a deterministic two-call barrier.
- `npm run runtime:go:speed-cache-gate -- --json`: passed, `19/19`
  command checks and `13/13` metrics.
- Focused activated-context measurement: dynamic `173` bytes / `44`
  estimated token units; turn-tail `158` bytes / `40` estimated token units.
- Default before/after request body digest:
  `3b6c8a566d4b8a0c236041303f2cd2689f20e1b809f3a14858ec04e67f297ada`.
- Default before/after prefix hashes: system `dad3771ea3fc9652`, prefix
  `f17bc8e74c321939`, prefix-items `4f53cda18c2baa0c`, tools
  `e96a72437f7a1680`.
- Architecture gravity-well gate: passed without raising the server file or
  effective-line budget.

## Findings

| Priority | Finding | Path | Required fix |
| --- | --- | --- | --- |
| none | No blocking finding for the foundation scope. | n/a | n/a |

## Verdict

- final_verdict: pass
- blocking_reasons: none
- non_blocking_follow_up: no production context-source reader is intentionally
  included in this foundation; future source-owning changes must implement the
  port and remain fail closed. Credentialed live-provider cache behavior and
  any UI surface require separate evidence.
- accepted_evidence_paths: `hard-gate-evidence.md`, `review-package.md`, and the
  implementation/test paths above
- next_owner: parent `case-evidence-publication-gate` Goal
