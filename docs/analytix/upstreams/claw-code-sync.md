# analytix Claw Code sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"claw-code","reviewedCommit":"4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Reference / low-confidence implementation comparison ledger.
Applies to: task vocabulary, Rust runtime experiments, and test ideas.
Current as of: `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` on 2026-07-20.
Source of truth: pinned source code; Claw parity documents require code checks.

## Source And Reuse Boundary

| Field | Value |
| --- | --- |
| Local source | `/Users/sun/Projects/_upstreams/claw-code` |
| Branch | `main` |
| Reviewed commit | `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` |
| License evidence | `LICENSE` blob `28e6960dd9a2be209c308b49bc9a8973dbf4d60d` |
| License posture | MIT, file-level provenance and notice required |

Claw describes itself as a museum exhibit rather than a production project.
Its Task, Team, and Cron surfaces are largely in-memory registries rather than
durable worker or scheduler implementations. Its top-level and Rust parity
documents also disagree on several stubs. Do not use parity tables as proof of
production maturity.

The current dirty deletion of `LICENSE` was preserved. License conclusions use
the tracked Git blob, not the dirty filesystem copy.

## 2026-07-20 - Tool authority, report, approval, cache, and release audit

| Capability | Current implementation evidence | Decision and required Analytix strengthening |
| --- | --- | --- |
| One tool registry | `rust/crates/tools/src/lib.rs` normalizes an allowlist, rejects unknown names, derives advertised definitions from the same registry, and checks the allowlist again at execution. The CLI injects that registry/set into both provider and executor. | `adapt` the single-authority pattern in Go, then strengthen it with frozen thread/turn/case/epoch/snapshot context, schema/server identity, expiry, and a host-issued `ExecutionGrant`. |
| Report semantics | `report_schema.rs` separates observed fact, inference, hypothesis, and recommendation; it also distinguishes checked-no-hit from unexamined scope and provides deterministic canonicalization/projection. | `adapt` into typed `ClaimRecord`, `FinalAnswerEnvelope`, PII projection, and `PublicationReceipt`, retaining full SHA-256 and exact evidence membership. |
| Approval lifecycle | `approval_tokens.rs` models pending/granted/consumed/expired/revoked and checks scope, executor, expiry, and replay. | `adapt` the consumption semantics, but make the ledger durable, signed, restart-validatable, and context-epoch/grant bound. Its parallel `policy_engine` optional-token check is rejected. |
| Prompt-cache diagnostics | `rust/crates/api/src/prompt_cache.rs` fingerprints model/system/tools/messages and classifies cache read/write and expected/unexpected breaks. | `adapt` provider-native cache observability and stable-prefix SHA-256. Do not cache or replay case final answers as fact authority. |
| Tool schema execution | Advertised schemas commonly set `additionalProperties:false`, but ordinary `serde_json::from_value` targets do not uniformly deny unknown fields. `StructuredOutput` permits arbitrary properties and echoes any non-empty map. | `reject`; advertised schema, runtime decoder, grant, and execution must share one closed contract and a required final cannot be an arbitrary tool. |
| MCP outcome | The protocol preserves `isError` and `_meta`, but the bridge treats only JSON-RPC errors as failure and returns `isError:true` as successful JSON. Server identity is optional and not case/snapshot bound. | `reject`; preserve transport and semantic status independently, verify live identity/connection epoch, and settle only host receipts. |
| Evidence and final publication | Report evidence is optional strings, report hashes are truncated to 16 hex characters, and assistant text enters the session/stream before any final gate. | `reject`; require registry membership, exact claim support, complete SHA-256, and atomic post-gate publication. |
| Reasoning and ordinary privacy | Thinking is accumulated, persisted to JSONL, replayed across turns, and only a narrow API-key sanitizer runs. | `reject`; no reasoning or full bank/card/identity data may enter ordinary stream, history, logs, or exports. |

Fresh Python tests passed 47/47 and the documentation source-of-truth checker
passed. The release-readiness checker failed because the preserved dirty
license deletion makes the license/link gate fail. Focused Rust tests did not
start offline because the lock requires `clap_complete 4.6.5`, absent from the
local cache; this is recorded as unverified rather than replaced with an old
report. CI covers formatting, workspace tests, clippy, and a Windows smoke,
but release artifacts lack the complete signing, notarization, SBOM,
provenance, and platform matrix required by Analytix.

No current Claw mechanism binds facts to the same case, turn, epoch, and
dataset snapshot or enforces one final evidence gate. Parity remains
`not-proven`; the mechanisms above become benchmark rows, not superiority
claims.

## Current Research Lens

| Area | Analytix decision |
| --- | --- |
| Task/team/cron vocabulary | Behavior and fixture reference only; Analytix durable jobs remain the owner. |
| Rust runtime/test vectors | Consider pure parser, workspace fingerprint, and cancellation tests after file-level license review. |
| Permission classification | Reject as a security baseline; shell heuristics retain documented bypass classes. |
| ACP | Record as incomplete; do not claim a usable external-control surface. |
| Windows setup | Documentation reference only, verified against real builds before use. |

## 2026-07-13 Intake

Analytix has durable child-job, heartbeat/stale/dead-letter diagnostics, steer,
pause/resume, reviewed worktree acceptance, repair, cleanup, and evidence
receipt implementations that should be compared against Claw. These are not
marked reached or exceeded until `CapabilityBenchmarkV1` executes against this
pin. Useful Claw material remains a source of small test vectors, status
vocabulary, and isolation/cancellation ideas that pass the Analytix admission
gate.

Rejected:

- in-memory registry behavior presented as durable parity;
- shell-string heuristics as an approval or sandbox boundary;
- Claw CLI/protocol identity or public product routes.
