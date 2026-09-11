# Upstream currentness delta - 2026-07-13

Status: Current source-intake evidence. This is not capability parity,
absorption, or superiority proof.

## Refresh result

All registered checkouts were fetched from `origin`; fast-forward updates were
applied with `git merge --ff-only`, which preserved every existing dirty entry.
`postgres-mcp` and `instructor` were cloned at their current default-branch
heads because the Goal explicitly requires them as audited sources.

| Source | Previous manifest pin | Current pin | Range size | Worktree note |
| --- | --- | --- | ---: | --- |
| CodexDesktop-Rebuild | `3cb1e86063fe` | `a387defab0e0` | 2 | clean; version metadata only in the observed range |
| DeepSeek-Reasonix | `78e9e2656ae5` | `3703cf430c2c` | 192 | existing deleted `LICENSE` preserved; 35-commit follow-up reviewed |
| Kun | `777e343cdfe2` | `9fb5ecf90430` | 276 | existing license deletions and `screenlog.0` preserved |
| Claude Code | `d4d8fbbb333c` | `d4d8fbbb333c` | 0 | existing deleted `LICENSE.md` preserved |
| Claw Code | `4ea31c1bc91c` | `4ea31c1bc91c` | 0 | existing deleted `LICENSE` preserved |
| Gajae Code | `aedd0df99e7c` | `ff03e588a06e` | 117 | existing vendored-license deletions preserved |
| Hermes Agent | `6142203bd7af` | `2ccfdb2db4ee` | 208 | existing license deletions preserved; 88-commit follow-up reviewed |
| LazyCodex | `9b9f8e8f620e` | `3d7416bff3e6` | 2 | existing license deletions preserved |
| OpenCode | `9976269ab1ac` | `bb31c9b92cd6` | 47 | existing license deletions preserved; 21-commit follow-up reviewed |
| postgres-mcp | new | `07eb329c8c48` | intake | clean |
| Instructor | new | `47fdb2ca0711` | intake | clean |

Every checkout is now `0 ahead / 0 behind` its configured upstream branch.
The source manifest records Git-object license blobs, so existing working-tree
license deletions are warnings and were neither restored nor hidden.

## Immediate security-relevant delta triage

The refreshed ranges are large and require substantive code/test/doc review.
High-signal commit subjects and paths establish the next audit queue, not an
adoption decision:

- DeepSeek-Reasonix adds extensive evidence verification/classification,
  delivery readiness, plugin intake, cache UI, and recovery changes.
- Kun adds extension-platform validation, agent-stream resource bounds,
  lifecycle cancellation, subagent settings, workspace durability, preview,
  and packaging hardening.
- Gajae Code adds durable release receipts, context-usage source-of-truth and
  cache work, defensive-copy tests, RPC/QA artifacts, and failure cleanup.
- Hermes Agent adds approval-observer scoping/redaction, session import
  validation, durable Kanban artifact handoff, run/stream lifetime separation,
  and provider configuration fixes.
- LazyCodex synchronizes two Codex marketplace releases; copied plugin content
  and gitlink/file-level license boundaries must be rechecked.
- OpenCode adds provider reasoning-variant fixes and large generated/app UI
  changes; generated churn cannot stand in for runtime capability evidence.
- postgres-mcp and Instructor enter the registered source set for the specific
  SQL execution-safety and structured-output tasks recorded in their ledgers.

No refreshed row is marked adopted, absorbed, reached, or exceeded until its
implementation evidence and deterministic Analytix benchmark row are current.
Every source ledger now carries an exact-commit
`analytix-upstream-review-v1` marker with `parity: not-proven`.

## Provenance anomaly

Gajae Code's local `v0.9.6` tag resolves to
`9cc4846fef2625f1d500e02c72c06d2c23ea8870`, while the remote tag currently
advertises `aedd0df99e7c9dff420b50f7ff47bd6645627bdd`. Fetch therefore rejected the
tag update as a clobber attempt. The branch was refreshed separately and is
current. The local tag was not force-rewritten; no evidence may use `v0.9.6`
until the moved-tag provenance is resolved.

## Commands and current boundary

```text
git -C <source> fetch --prune --tags origin
git -C <source> merge --ff-only @{u}
npm run audit:upstreams -- --json
```

The strict audit is the source-intake gate: it now rejects stale or duplicate
ledger markers, tracking-branch drift, path escape, and unsupported parity
claims without hash-bound `CapabilityBenchmarkV1` evidence. OpenSpec P3 tasks
remain open for recursive
README/docs/code/test/skills/MCP/schema/eval/release review and executable
CapabilityBenchmarkV1 evidence.
