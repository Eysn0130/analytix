# Analytix nine-source capability and documentation audit

Status: Reference / current white-box audit snapshot.
Applies to: the nine repositories in `upstream-sources.json`, Analytix at
`8ddfdfd1fda2a744eaca1ad6a99fe5a48354a448`, and only the explicitly named
in-progress worktree observations below. Dirty state is not commit-pinned or
release proof.
Current as of: 2026-07-10.
Source of truth: pinned Git objects, current Analytix code/tests, and fresh
commands recorded below. This document is not implementation or release proof.

Post-snapshot provenance note (2026-07-10): the commands below ran against the
pre-rewrite Analytix baseline `8ddfdfd1fda2a744eaca1ad6a99fe5a48354a448`.
Its retained, sanitized equivalent is
`99ba683f517fb4a58ed26f9d5e9f06b104f1613d`; the controlled rewrite removed
two retired sensitive paths and is not a rerun of this audit. Resolve other
Analytix-local evidence identifiers through
[`../history-rewrite-provenance-2026-07-10.json`](../history-rewrite-provenance-2026-07-10.json).
External upstream commit, tag, and blob identifiers are unchanged.

Subsequent closure note: `close-security-config-artifact-risks` completed and
was archived on 2026-07-10, synchronizing four accepted capability specs under
`openspec/specs/`. The in-progress row below remains the original snapshot
observation, not the current implementation verdict.

Subsequent Context Epoch note: the current Goal worktree implements the
`context-epoch-foundation` domain/app/ports boundary and records measured
before/after evidence in
`openspec/changes/archive/2026-07-10-context-epoch-foundation/hard-gate-evidence.md`. The default
request body, prefix hashes, and tool-schema hash remain byte-identical; active
dynamic/turn-tail content is separately bounded and measured; compact/restart
fixtures fail closed. This is implementation evidence in a dirty worktree, not
commit-pinned release proof.

## Outcome

The existing documentation is directionally strong but not complete or fully
current. It formally governed five sources while the local research root has
nine; several entry documents pin old commits, cite deleted TypeScript runtime
paths, or treat ignored Codex application extracts as stable source evidence.
The most serious error was excluding licensing from code-level absorption
while allowing direct copying.

Analytix should not become a union of nine product shells. Its current durable
Goal/Todo evidence closure, read-only child default, parent-owned worktree
review/accept/repair, Go runtime ownership, HTTP/SSE desktop contract, provider
endpoint families, and renderer bridge are stronger governance boundaries than
many upstream equivalents and must remain the floor.

The highest implementation risk found is durable storage safety: current JSON
map publication is not atomic, process-local locking does not protect the same
data directory from a second runtime, event sequence recovery scans append-only
logs, and there is no complete lease/revision/CAS/conflict-recovery contract.
Reasonix now has concrete patterns and tests for this area, but the fix requires
a separate persisted-data OpenSpec change.

## Fixed Source Snapshot

| Source | Branch / commit | Root posture | Default use |
| --- | --- | --- | --- |
| CodexDesktop-Rebuild | `master` / `1454a79a` | No project license at HEAD | Clean-room behavior and build-drift research only. |
| DeepSeek-Reasonix | `main-v2` / `34cfac29` | MIT | Engine algorithms/tests after file-level provenance and notice. |
| Kun | `master` / `777e343c` (`v0.2.25`) | PolyForm Noncommercial | Product comparison and restricted reuse under source terms. |
| Claude Code | `main` / `15a21e1b` | All rights reserved/commercial terms | Clean-room behavior requirements only. |
| Claw Code | `main` / `4ea31c1b` | MIT | Low-confidence fixtures/algorithms, not a production parity source. |
| Gajae Code | `main` / `9cc4846f` (`v0.9.6`) | Root MIT; inherited notice chain unresolved | Reference only until provenance is resolved. |
| Hermes Agent | `main` / `540f9019` | MIT root with file exceptions | File-level port/reference after notice review. |
| LazyCodex | `main` / `132a61da` (`v4.16.2`) | MIT root; gitlink/file exceptions | Committed plugin components only after file-level review. |
| OpenCode | `dev` / `01131c75` | MIT | Contract/test/algorithm port with notice. |

Machine evidence lives in `upstream-sources.json`; run
`npm run audit:upstreams` after refreshing sources. A hash change is a stale
signal, not permission to update the manifest without reviewing the delta.

## Documentation Findings Corrected By This Change

| Finding | Evidence | Correction |
| --- | --- | --- |
| Only five standing source ledgers | `docs/analytix/upstreams/README.md`, strategy, targets | Add Claude, Claw, Gajae, Lazy ledgers and all-source inventory. |
| OpenCode/Codex/Reasonix matrix refs stale | `node scripts/runtime-closure-matrix-audit.mjs --strict-local --json` returned `ok:false` for all three refs | Mark matrix as dated until substantive re-review; do not keep 42 `fixed` rows as current upstream parity. |
| Codex ignored extracts treated as tracked evidence | Codex HEAD has no tracked `src/`; `.gitignore` excludes extracts | Permit observed behavior only; generated extracts require hash/provenance and no direct reuse absent authorization. |
| Codex assets described as fully authorized | No authorization record or covering project license exists in the reviewed repository | Remove the authorization claim and require written scope plus asset hashes. |
| Code-level blueprint excludes licensing | Blueprint permits copy/port modes | Make license, authorization, notice, and provenance a hard admission gate. |
| `packages/runtime` described as a production runtime landing | Current production core is `packages/runtime-go`; retired TS loop/server paths are absent | Restrict `packages/runtime` to contracts/config/telemetry/launcher and retained test/conformance oracles. |
| LazyCodex described as "no code source" | Committed LSP, CodeGraph, team, bootstrap, continuation, and evidence components exist | Record real committed code while keeping the uninitialized OmO gitlink out of scope. |
| Claw role overstates maturity | Its own README says museum exhibit; task/team/cron are mostly in-memory | Limit it to low-confidence reference/tests. |
| Hermes role is stale | Current source has observer/middleware ABI, recovery, compaction, environments, and SQLite/FTS5 | Expand its source role and implementation packets. |

## Analytix As-Built Risks And Gaps

| Priority | Capability | Analytix as-built | Upstream evidence | Gap / required decision |
| --- | --- | --- | --- | --- |
| P0 | Durable atomic persistence and ownership | `json_map.go` uses direct `os.WriteFile`; runtime store locking is process-local | Reasonix session lease, atomic replace, meta ledger, recovery branch/GC | Specify atomic publish, fsync/Windows replacement, revision/CAS, process lease, conflict recovery, migration, and fault injection. |
| P0 | License/provenance closure | This change establishes a unified manifest/queue, but existing Kun/Reasonix/Codex lineage and destination notices remain unresolved | All nine license objects plus Gajae/Hermes/Lazy subtrees | Use `code-reuse-provenance.md`; close exact files/notices before a license-closure claim. |
| P0 | Safe execution/config truth | Baseline commit `8ddfdfd...` used `auto` + `danger-full-access`; the separate dirty `close-security-config-artifact-risks` worktree changes defaults to `on-request` + `workspace-write`, but that change remains separately owned and unvalidated here | Analytix's active `close-security-config-artifact-risks` change owns this | Complete and validate that change separately; do not present either the old baseline or the in-progress replacement as the final released state. |
| P1 | Event-log index, retention, and replay cost | `HighestSeq`, search, and cold replay read full `events.jsonl`; no complete retention/compaction policy | Reasonix event index/meta sidecars; OpenCode SQL/session runner | Define index rebuild, bounded memory, pruning/archival, cursor correctness, restart, and OOM tests together. |
| P1 | Durable input inbox and per-thread coordinator | Running-turn steer is durable, but no OpenCode-style persisted steer/queue inbox and wake coordinator | OpenCode Session V2 input/run coordinator/context epoch | Reimplement in Go with idempotency and replay; OpenCode runner TODOs prevent calling it a mature baseline. |
| P1 | Context Epoch | Snapshot baseline lacked dynamic source admission; the subsequent Goal worktree now has versioned source registry/snapshots, safe-boundary reconcile, prompt-invisible metadata, bounded provider fragments, durable compact/restart recovery, and one shared security epoch | OpenCode context epoch, Reasonix history/cache, Hermes session context | Current implementation evidence is in `openspec/changes/archive/2026-07-10-context-epoch-foundation/hard-gate-evidence.md`; future memory/instruction/inbox readers must use its reader port and their own admission gates. No live-provider cache superiority is claimed. |
| P1 | Compaction integrity | Go compaction uses deterministic summaries; model-summary/retry/cooldown invariants remain limited | Hermes compressor, Gajae branch/handoff, Reasonix compaction/history search | Add tool-pair, multimodal, image-budget, role, failure-cooldown, no-repeat, and recovery tests after Context Epoch. |
| P1 | MCP OAuth, roots, prompts, resources | Go MCP primarily owns list/call/catalog; prompts/resources are diagnostic-only; OAuth is absent | OpenCode MCP auth/resources; Kun remote MCP OAuth | Design OS-secret-backed OAuth and protocol coverage without importing upstream config stores. |
| P1 | Production LSP/formatter/CodeGraph | `code_index` is explicitly a lightweight fallback, not LSP/call graph | OpenCode LSP/format; Lazy LSP daemon/CodeGraph | Prefer optional Hub/MCP plugin first; benchmark schema/token cost and process lifecycle. |
| P1 | Plugin supply-chain quarantine | Hub checksum/trust exists, but compile-before-copy, per-file ownership and runtime drift quarantine are incomplete | Gajae plugin compiler/installer/policy | Add file manifest, symlink/path policy, atomic directory publish, drift quarantine, and constrained hooks. |
| P1 | Observer lifecycle | Runtime events exist but no stable read-only observer ABI across session/turn/tool/approval/subagent | Hermes observability and middleware | Start with fail-open, redacted, correlated read-only observers; behavior-changing middleware waits for safer defaults. |
| P1 | Searchable memory and controlled learning | Baseline UI could overstate runtime use; the separate dirty worktree now labels the store manual and reports model injection/automatic capture unavailable, but no FTS/candidate promotion loop exists | Hermes SQLite/FTS5, memory review/curator; Reasonix candidates | Validate the in-progress truthfulness fix, then add read-only search and user-reviewed candidates; automatic mutation remains opt-in and auditable. |
| P1 | Connect Phone delivery continuity | Feishu/Weixin/Telegram exist, but cross-restart source/FIFO/delivery recovery is less explicit | Hermes `SessionSource`, FIFO and restart delivery; Gajae notifications | Specify source identity, no-loss ordering, reset/recovery, dedupe, redaction, and approval continuity before adding channels. |
| P1 | Long-thread rendering complexity | Analytix virtualizer preserves bottom distance but window calculation scans rows linearly and continuity E2E is narrower | OpenCode measured timeline and performance E2E; Codex behavior reference | Add indexed offsets/LRU measurements and production-build stability tests before claiming best-in-class smoothness. |
| P1 | Evidence receipt hardening | Goal/Todo and child evidence are strong, but file receipt admission can be stricter | Lazy executor verify | Add realpath/root/symlink/non-empty/atomic-state/retry invariants to existing Analytix receipts. |
| P1/P2 | Kun Design workflow | Analytix has design tokens/icons, not Kun's current canvas/AI rail/ShapeOps/handoff product | Kun `docs/DESIGN_MODE.md` and design source | Requires a separate product spec; never restore an old route by treating it as inherited parity. |
| P2 | External RPC/ACP control | Public desktop contract is HTTP/SSE; no general RPC/ACP host | Gajae coordinator/RPC/ACP; Claude behavior reference | Decide whether IDE/automation embedding is a product goal; keep protocol fail-closed and Analytix-owned. |
| P2 | Restricted Code Mode | No equivalent sandboxed multi-call code interpreter | OpenCode experimental Code Mode | Separate threat model with timeout, call, output, network, filesystem and approval budgets. |
| P2 | Remote execution environments | Local process/PTY and worktree isolation exist; no unified Docker/SSH/etc. backend | Hermes BaseEnvironment | Start with local + Docker only after secret/sandbox/process-registry design; SSH later. |
| P2 | Offline speech and advanced provider auth | Local Whisper and subscription OAuth remain incomplete | Kun current source | Separate packaging, secret-store, provider terms, cross-platform and product UX review. |

## What Analytix Already Does Better

- Parent goals, todos, and evidence closure remain parent-owned and cannot be
  completed automatically by a child.
- Child agents default to `readOnly`; writable worktrees require explicit
  `inherit`, a clean parent, durable ownership, review, approval, and explicit
  cleanup.
- Background jobs have durable status, output cursors, heartbeat/stale/dead-
  letter diagnostics, steer, pause/resume, kill, and renderer projection.
- Worktree accept/repair paths reject auto-merge, auto-commit, silent conflict
  resolution, and cross-parent control.
- The desktop public contract remains one Electron -> preload -> main -> Go
  HTTP/SSE path with `window.analytix` and provider endpoint-family support.

These are regression floors. Upstream absorption must not trade them away for
feature parity.

## Code-Level Reuse Map

| Candidate | Mode | Analytix landing | Admission boundary |
| --- | --- | --- | --- |
| Reasonix atomic write/session lease/event index tests | `port-and-adapt` or independent Go reimplementation | filestore/eventlog/thread app services | MIT notice, exact source commit/path, persisted-data spec, Windows/crash/concurrency tests. |
| OpenCode timeline algorithms/E2E | `port-and-adapt` | React virtualizer and Playwright/performance tests | MIT notice; Solid-to-React rewrite; measure against Analytix baseline. |
| OpenCode MCP OAuth/resources/prompts | Contract/test reference plus Go implementation | MCP app/outbound adapters and OS secret boundary | MIT notice for copied tests; threat model and multi-provider/product tests. |
| Lazy LSP/CodeGraph | Optional plugin port | Hub/MCP plugin, not production Go core by default | Component license/NOTICE and gitlink exclusion; process cleanup and cache/tool-schema benchmark. |
| Hermes observer/compaction/session recovery | `port-and-adapt` tests and invariants | Go observer, compaction, Connect Phone | Per-file license review; exclude all-rights-reserved PowerPoint skill and preserve Apache NOTICE where applicable. |
| Gajae plugin quarantine/session handoff | `clean-room-reference` while provenance is blocked | Hub/package trust and Go thread/compaction | Resolve copyright lineage before direct reuse. |
| Claude daemon/enterprise/remote behavior | `clean-room-reference` | New Analytix specs/tests | No copying of code, prompts, plugin or docs text. |
| Codex desktop feel/build drift | `clean-room-reference` | Renderer benchmarks and source manifest tooling | No direct code/asset/extract copying absent written authorization. |
| Claw lifecycle/workspace fixtures | `selective port` only for small verified files | Go/Rust-neutral tests or isolated helpers | MIT notice and exact provenance; low-confidence/museum status, no in-memory registry or shell-heuristic safety reuse. |
| Kun Design/OAuth/product flows | `port-and-adapt` only after source terms/product approval | Renderer/main/shared + Go contracts | PolyForm obligations, commercial authorization if applicable, explicit product spec. |

## Construction Sequence To Exceed The Sources

1. Close provenance and security truth first.
   The current change adds the source/license gate; the existing security
   change owns safe defaults, config truthfulness, and artifact hygiene.
2. Make persistence unambiguously safe.
   Add atomic publication, revision/CAS, cross-process lease, recovery branch,
   event index and bounded replay before more autonomous state is added.
3. Use the implemented Context Epoch foundation for durable input admission.
   Context Epoch now protects the default cache shape and compact/restart
   metadata; memory, instructions, and queued steering remain separate gated
   consumers rather than implicit prompt injectors.
4. Strengthen context and observability.
   Add compaction invariants, read-only observers, searchable memory candidates,
   and Connect Phone delivery recovery.
5. Expand tools through optional boundaries.
   Add MCP OAuth/resources/prompts and LSP/CodeGraph as controlled adapters or
   plugins before considering Code Mode or remote environments.
6. Add product differentiators only with product evidence.
   Kun Design, advanced provider auth, offline speech, enterprise policy, and
   external control each require their own OpenSpec change and desktop QA.

## Superiority Gates

| Claim | Required proof |
| --- | --- |
| Safer persistence than Reasonix/OpenCode | Multi-process contention, crash/kill -9, truncated write, Windows replace, stale lease, CAS conflict, recovery GC, event-index rebuild, and replay-cursor tests. |
| Better long-thread experience than OpenCode/Codex reference | Identical long-thread dataset, DOM/heap/frame/scroll-anchor measurements, height mutation, prepend, streaming, keyboard/focus/wheel E2E in production build. |
| Better context/cache behavior than Reasonix/OpenCode/Hermes | Same-prompt request bytes, stable-prefix/tool-schema hashes, provider-native cache telemetry, compaction/restart continuity, token/latency/cost scorecard. |
| Better agent control plane | Permission floor, restart lineage, durable inbox, parent evidence ownership, job control, worktree conflict, nested budget and renderer-drilldown tests. |
| Better MCP/tool ecosystem | OAuth/secret safety, roots/resources/prompts, trust, schema cache, reconnect, approval, supply-chain quarantine and plugin lifecycle matrix. |
| Better product breadth than Kun | Code/Write/SDD/Connect Phone/Schedule plus any approved Design/auth/speech additions complete in real Electron QA without identity/schema/runtime regressions. |

## Upstream Documentation Caveats

- Reasonix's Chinese README understates direct Go dependencies compared with
  `go.mod`.
- Claw parity documents contain stale HEADs and disagree about remaining stubs.
- Gajae computer-use and standalone-build documentation lags current code and
  native asset requirements.
- Lazy component and Windows/bootstrap documents undercount committed
  components and overstate automatic installation behavior.
- OpenCode Session V2 runner still documents unfinished clustered ownership,
  retry and snapshot persistence work; it is a candidate, not a mature oracle.
- CodexDesktop generated extracts are not tracked source and have no covering
  project license at the reviewed commit.

## Validation Recorded

Before correction, this command returned stale refs for Reasonix, OpenCode and
CodexDesktop:

```text
node scripts/runtime-closure-matrix-audit.mjs --strict-local --json
```

The replacement source inventory is validated with:

```text
npm run audit:upstreams
```

No provider, MCP, packaged-GUI, performance, or legal-authorization claim is
made from this documentation audit alone.
