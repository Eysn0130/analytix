# Upstream Currentness Delta - 2026-07-18

Status: source-identity refresh only. This record proves that all eleven
registered research checkouts match their configured tracking branches and
that their pinned license objects remain readable. It is not a completed deep
audit, an absorption decision for every changed file, `CapabilityBenchmarkV1`
evidence, or proof that Analytix has reached or exceeded an upstream.

## Update And Preservation Boundary

All registered repositories were fetched with remote pruning. Earlier same-day
refreshes advanced Kun, Hermes Agent, and OpenCode. The latest refresh advanced
DeepSeek-Reasonix and Hermes Agent only with `git merge --ff-only`, after
confirming that incoming paths did not overlap the existing local changes. The
CodexDesktop-Rebuild checkout already contained its tracking head; its stale
manifest pin was reconciled without changing that checkout. No checkout was
stashed, reset, cleaned, rebased, staged, or force-updated.

The pre-existing dirty paths were preserved: tracked license deletions in
DeepSeek-Reasonix, Kun, Claude Code, Claw Code, Gajae Code, Hermes Agent,
LazyCodex, and OpenCode, plus Kun's untracked `screenlog.0`. License checks used
the pinned Git objects, not mutable worktree files. The license object hashes
remain equal to the manifest; the user's authorization does not remove
provenance, notice, excluded-subtree, or file-level review obligations.

| Source | Tracking HEAD | Commits after 2026-07-16 pin | Dirty entries | Review status |
|---|---|---:|---:|---|
| CodexDesktop-Rebuild | `5e2fe42776c038f493edd4f0902ce8dc6489a7d9` | 1 | 0 | not-proven |
| DeepSeek-Reasonix | `2335d0df9ea4029108ed965f76c2efff30fe6cf4` | 18 | 1 | not-proven |
| Kun | `7c1bc0635c170e716f283e0889ae8f85d14703c8` | 125 | 3 | not-proven |
| Claude Code | `07dcb0e13580b21174ff1bf6a7e1d5ead3b61d60` | 2 | 1 | not-proven |
| Claw Code | `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` | 0 | 1 | not-proven |
| Gajae Code | `7dc297145f333a00b7e913ce7c8cd5dedeb3fd34` | 0 | 3 | not-proven |
| Hermes Agent | `e53f87fe6925f9b0575f16b814d8994dfd455128` | 371 | 5 | not-proven |
| Instructor | `47fdb2ca07119d389a3c0e8bc28b9930b814f294` | 0 | 0 | not-proven |
| LazyCodex | `2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf` | 1 | 12 | not-proven |
| OpenCode | `b8142c7aa8f88222873fb79d636e312e28037c2d` | 41 | 4 | not-proven |
| postgres-mcp | `07eb329c8c48e49640e0d1b5b35465d4d024c3ee` | 0 | 0 | not-proven |

Every checkout reported `0 0` for `HEAD...@{upstream}` after the update.

A second same-day fetch found additional tracking commits in DeepSeek-Reasonix,
Kun, Claude Code, Hermes Agent, LazyCodex, and OpenCode. The pins and cumulative
counts above include that refresh. Root license object hashes remained
unchanged. The additional deltas are admitted only for review; no parity or
absorption status was upgraded.

A third same-day fetch added two DeepSeek-Reasonix commits and 51 Hermes Agent
commits relative to the prior manifest pins. It also reconciled the existing
CodexDesktop-Rebuild tracking HEAD. The exact deltas and the rerun Reasonix
cache baseline are recorded in the per-source ledgers; all three review markers
remain `not-proven` with no capability benchmark result.

A fourth same-day refresh added 20 Hermes Agent commits and four OpenCode
commits relative to the prior manifest pins. Hermes added reconnect-safe
in-flight turn preservation and corrected empty-response/compression
classification; the OpenCode range is desktop-build maintenance. Both review
markers remain `not-proven` with `capabilityBenchmarkV1: null`; neither source
is reached or exceeded.

A fifth same-day live-remote probe found 43 additional Hermes Agent commits.
After proving a pure fast-forward and zero path overlap with the five existing
worktree deletions, the checkout advanced with `git merge --ff-only`. The new
Discord recovery, MCP polling/OOM, computer-use delivery, desktop state, and
model-switch tests are admitted for deep review only; no parity marker or
benchmark result was upgraded.

A sixth refresh advanced Hermes by another 19 commits through a pure
fast-forward while preserving the same five local license deletions. The
28-file delta received code/test/document review. Request-local transport
cleanup, bounded recovery watchdogs, one-query projection, sidebar batching,
and voice/STT cache invalidation are routed for Analytix-native adaptation;
direct model-final persistence, reasoning replay, private socket internals,
and read-failure-to-empty behavior are rejected. The range contains no new
evidence-chain, MCP, structured-output, Skill, DeepSeek-cache, eval, or release
mechanism, so parity remains `not-proven` and no benchmark verdict changed.

## Preliminary Delta Routing

These rows route the new work; they are not final adopt/adapt/reject decisions.

| Source | Observable delta | Required next proof |
|---|---|---|
| DeepSeek-Reasonix | Memory-v5 compiler removal, MCP persistent-session/trust changes, stalled error-body/session-switch repair, and desktop/TUI fixes across 164 files | Review implementation, tests, docs, request/cache effects, and trust regressions; compare against exact Analytix `ExecutionGrant` and current-run source authority. |
| Kun | Mid-turn guidance, delegation timeout changes, runtime-thread migration, release hardening, and broad UI/media work across 930 files | Separate product features from safety mechanisms; reject unbounded delegation; create focused migration, release, and control-plane benchmarks before absorption. |
| Hermes Agent | Live-context cache isolation, compression runtime switching, transcript repair, truthful cron ledger, single-writer stream fencing, reconnect-safe in-flight turn preservation, Discord missed-message recovery, bounded MCP polling/OOM coverage, computer-use delivery ladders, and broader runtime/provider work | Review exact code/tests/docs and build deterministic cache-context, transcript, terminal-path, restart, compression-trigger, dedupe/backfill, process-memory, delivery-fallback, and stream-writer benchmark rows. |
| OpenCode | Prompt-editor preservation, recovery-window guards, Azure endpoint restoration, provider/pricing data, session/desktop UI changes, and a latest Nix/OpenTUI/desktop-build-only range across 155 files | Distinguish generated/UI/build changes from reusable runtime invariants; compare recovery and endpoint behavior through Analytix-owned contracts. |
| CodexDesktop-Rebuild | Version bump only | Record current pin; no capability claim. |
| Claude Code | Changelog/feed only | Use behavior-only research plus exact authorization/provenance for any material reuse; no implementation claim. |
| Remaining four sources | No commit after the 2026-07-16 pin | Prior open benchmark and absorption obligations remain open; zero delta does not prove parity. |

## Verification

The following command passed against this working tree:

```text
npm run audit:upstreams

PASS upstream source audit contract
PASS upstream source audit (11 sources)
```

The audit verifies discovery, branch/tracking identity, exact pin, remote,
review marker, and license-object evidence. It does not execute upstream test
suites, Analytix capability benchmarks, the funds-plugin matrix, or release
acceptance.

## Open Proof Obligations

- Complete file-level code, test, README/docs, schema, skill, MCP, evaluation,
  release, and license review for the changed surfaces above.
- Record exact provenance before direct or substantial code reuse.
- Build and execute `CapabilityBenchmarkV1` rows against exact upstream and
  Analytix commits; keep every affected matrix row `not-proven` until then.
- Run a same-account, same-model, same-endpoint, randomized DeepSeek A/B cache
  experiment using provider-native hit/miss counters. Structural stability
  tests alone cannot establish a higher live cache hit rate.
- Complete the P0-P4 implementation, funds-plugin safety gates, desktop E2E,
  packaging, and release metrics independently of source currentness.
