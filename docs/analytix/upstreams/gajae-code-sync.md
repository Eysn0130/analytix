# analytix Gajae Code sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"gajae-code","reviewedCommit":"ee15071cb52316708a1a2235958ee7bfdd7738ec","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Reference / blocked-provenance code comparison ledger.
Applies to: orchestration, external control, sessions, plugins, notifications,
native tools, and long-task evidence.
Current as of: `ee15071cb52316708a1a2235958ee7bfdd7738ec`
(`v0.11.5`) on 2026-07-20.

## Source And Reuse Boundary

| Field | Value |
| --- | --- |
| Local source | `/Users/sun/Projects/_upstreams/gajae-code` |
| Branch / tag | `main`; exact HEAD is `v0.11.5` |
| Reviewed commit | `ee15071cb52316708a1a2235958ee7bfdd7738ec` |
| Root license evidence | `LICENSE` blob `16eb3fc020a9dbefb165d2fb1d4597d2203c44bd` |
| Current posture | Clean committed MIT/NOTICE objects may be adapted with complete notices; the dirty checkout is not a copy source. |

The current root says MIT and `NOTICE.md` records the inherited lineage.
Vendored Brush and insane-search files have their own MIT notices. The local
checkout currently deletes three nested license files, so code-level reuse
must read the clean committed objects and carry the applicable root, NOTICE,
and nested notices; the dirty working tree is never evidence that those
obligations disappeared.

## 2026-07-20 Citation And Corrupt-State White-Box Audit

The current web-search result contract does not prove that a citation came
from a successful search execution. `packages/coding-agent/src/web/search/
types.ts` exposes sources and citations without an execution receipt. The
OpenAI-compatible, Codex, Anthropic, and xAI adapters accept at least one of a
structured citation, call-start/request-count signal, or URL parsed from model
text without requiring a matching successful search result. Several tests
explicitly expect citation-only responses to succeed. Gemini grounding
metadata is stronger but the shared contract still lacks one uniform success
authority. The documentation's fail-closed claim therefore conflicts with
reachable code.

Analytix decision:

- adapt URL normalization and deduplication only after a host-verified search
  execution receipt;
- reject request `tool_choice`, annotation presence, request count, call-start,
  or free-text URL extraction as proof of execution;
- add a local fake-stream matrix covering requested, started, failed,
  cancelled, succeeded, mismatched-result, malformed URL, and private URL
  states before crediting Gajae parity.

Gajae's `.gjc` state layer has useful `absent/corrupt/valid` classification and
strict CLI mutation paths. It is not uniformly fail closed: ordinary reads,
team-summary persistence, reconcile, and derived active-state paths can treat
corrupt state as empty, overwrite it, or swallow the error. Its checksum is
stored beside the mutable payload and the atomic writer lacks a complete
fsync/lock/restart authority. Analytix will adopt tri-state strict mutation and
raw-byte quarantine, but reject tolerant overwrite and same-file checksum as
authority.

Fresh upstream tests were not run in this audit. Current citation tests are
mocked provider tests and no uniform execution-proof matrix exists. Parity and
`CapabilityBenchmarkV1` therefore remain `not-proven` / null.

## High-Value Comparison Areas

| Area | Source family | Analytix landing | Decision |
| --- | --- | --- | --- |
| Coordinator MCP, RPC stdio, ACP | `packages/coding-agent/src/{coordinator-mcp,modes}` | Future Analytix external-control port | Contract study; keep HTTP/SSE desktop contract authoritative. |
| Session tree, branch summary, handoff, blobs | `packages/coding-agent/src/session`, `packages/agent/src/compaction` | Go thread/event/compaction domain | Clean-room contract reimplementation after provenance review. |
| Plugin compile/validate/quarantine | `packages/coding-agent/src/extensibility/gjc-plugins` | Hub/package trust and Go plugin boundaries | High-value P1 behavior; do not import Bun runtime. |
| Notification WebSocket and Telegram daemon | `crates/gjc-notifications`, `packages/coding-agent/src/notifications` | Connect Phone | P2 after delivery, identity, and approval threat model. |
| Rust scan/AST/hashline/PTY | `crates/pi-natives`, `crates/pi-shell` | Native helper or Go adapter | Benchmark first; high packaging coupling. |
| Long-task receipts and gates | `packages/coding-agent/src/gjc-runtime` | Goal/Todo/evidence and child worktree | Absorb receipt freshness and restricted-role tests, not product vocabulary. |

## 2026-07-13 Intake

Gajae has a stronger external-control vocabulary, plugin quarantine pipeline,
session branch/handoff model, notification SDK, and native performance toolkit
than current Analytix in those narrow areas. Its Bun/TypeScript/Rust/N-API
stack cannot be inserted into the Go-only production runtime. Prefer protocol
fixtures, algorithms, and clean-room Go/Hub implementations after provenance
and benchmark gates.

Upstream documentation conflicts are recorded: current computer-use code
exists despite older documentation saying the N-API/TypeScript layer is
pending, and ordinary `bun build --compile` does not prove a self-contained
release with native and skill assets.
