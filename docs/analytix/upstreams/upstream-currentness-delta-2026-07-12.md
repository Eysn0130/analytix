# Upstream currentness delta - 2026-07-12

Status: current source-intake evidence for the active
`case-evidence-publication-gate` change. This is not a parity, release, or
`CapabilityBenchmarkV1` pass report.

## Update method and preservation boundary

All nine registered repositories under `/Users/sun/Projects/_upstreams` were
fetched from `origin`, checked for a strict fast-forward relationship, checked
for overlap between incoming paths and local dirty paths, and advanced with a
fast-forward-only merge. No reset, checkout, stash, cleanup, submodule update,
or license restoration was performed. Existing deleted license files and
`Kun/screenlog.0` remain user-owned worktree state; license evidence below is
read from Git objects at the pinned commit.

| Source | Branch | Current commit | Delta from 2026-07-10 review | Local state relevant to review |
| --- | --- | --- | --- | --- |
| CodexDesktop-Rebuild | `master` | `3cb1e86063fe9632f7fbaf385ecab87abe70581b` | 2 commits | clean |
| DeepSeek-Reasonix | `main-v2` | `78e9e2656ae5275cbdd29429053fdcc1cc97373c` | 77 commits | root `LICENSE` deleted locally |
| Kun | `master` | `777e343cdfe228038bb092853c60f5d1e82f5029` | unchanged | license deletions and untracked `screenlog.0` preserved |
| claude-code | `main` | `d4d8fbbb333c627d8fe2c1c583a5ccc26fdb1aed` | 1 commit | `LICENSE.md` deleted locally |
| claw-code | `main` | `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` | unchanged | root `LICENSE` deleted locally |
| gajae-code | `main` | `aedd0df99e7c9dff420b50f7ff47bd6645627bdd` | 4 commits | vendored license deletions preserved |
| hermes-agent | `main` | `6142203bd7af6c5d78f5dd0d58dbe64af5c02345` | 147 commits | root and component license deletions preserved |
| lazycodex | `main` | `9b9f8e8f620e3f797567078734165350e1e46659` | 2 commits | license deletions preserved; `src` gitlink remains uninitialized |
| opencode | `dev` | `9976269ab1accfc9f9dc98a4a688c516934de422` | 15 commits | root and package license deletions preserved |

Every checked-out branch is at its fetched upstream ref with ahead/behind
`0/0`. LazyCodex's uninitialized `src` gitlink remains pinned at
`65715d1c2c35e27ccf2195ef688b0909dddb403c`; its unavailable contents are not
review evidence and cannot contribute to a reached/exceeded result.

## License and authorization record

The license names, classes, blob hashes, and reuse policies in
`upstream-sources.json` remain unchanged because the corresponding tracked
license blobs did not change. The user declared code-level reuse authorization
for all registered sources in the active Goal thread on 2026-07-11. The
manifest records that declaration as `declared-unverified`; it does not rewrite
an upstream license class, admit an uninitialized gitlink, waive third-party
notices, or replace file-to-file provenance in `code-reuse-provenance.md`.

## Security-relevant delta and Analytix decisions

### DeepSeek-Reasonix atomic finalization window

Current implementation evidence:

- `internal/control/controller.go`
- `internal/control/admission_test.go`
- `internal/control/controller_test.go`
- relevant commits `4a57bb86` and `6b2e54f0`

Reasonix now keeps the controller in `finishing` until `TurnDone` fan-out is
complete and parks later asynchronous turns FIFO. Analytix will adapt this as a
typed `Running -> Finalizing -> Accepted|Boundary -> Idle` state machine where
`Finalizing` includes the Final Evidence Gate, signed accepted-final durable
write, and SSE publication. Parked work must bind a frozen
thread/case/epoch/snapshot descriptor and revalidate it before admission; an
in-memory closure is insufficient. This is an open P1 implementation gap, not
an achieved row.

### Hermes effect disposition and terminal choke point

Current implementation evidence:

- `agent/tool_result_classification.py`
- `agent/tool_executor.py`
- `agent/replay_cleanup.py`
- `agent/turn_finalizer.py`
- corresponding classification, replay, and iteration-limit tests

Hermes distinguishes dispatch-before-failure (`none`) from a dispatched
operation whose side effect is unknown (`unknown`) and keeps unknown-effect
calls visible during replay cleanup. Analytix will implement a host-derived
`none | unknown | confirmed` disposition based on the current ExecutionGrant,
dispatch phase, transport outcome, and verified postcondition. `unknown` may
not be retried automatically, sign an EvidenceReceipt, enter verified history,
or support a case claim.

Hermes's single post-loop finalizer structure is useful, but its candidate
publication on budget exhaustion, model-generated terminal summary,
post-final plugin mutation, and `last_reasoning` return are rejected. Analytix
must instead render a deterministic boundary answer and keep reasoning out of
events, history, reports, and exports.

### Reasonix recovery GC and provider schema dialect

`internal/agent/recovery_gc.go` and its tests re-read the actual parent and
branch transcript, validate coverage, and hold save/lease guards across
destructive cleanup. Analytix currently fails closed when compaction would
discard accepted-final or settlement authority; the remaining target is a
signed `CompactionAuthorityArchiveV1` with durable readback, source digests,
registry/settlement heads, case/epoch/snapshot bindings, and an atomic CAS.

`internal/provider/schema_dialect.go` separates provider wire-schema
adaptation from the canonical host schema. Analytix will preserve the
ExecutionGrant's host schema hash and add a separate bounded
`ProviderWireSchemaV1` digest/dialect/transform version. Custom endpoints are
not assigned a dialect by hostname guess. Raw provider error bodies are not
persisted; only a bounded tool-index-to-host-identity diagnostic is admissible.

### OpenCode prompt capture and structured output

`packages/app/src/context/prompt-state.ts` freezes renderer prompt/model/context
state before asynchronous submission. Analytix may use an equivalent
request-local UX snapshot, but the host-issued TurnSecurityContext remains the
only authority and a queued request cannot migrate to a new case implicitly.
OpenCode's required structured-output tool remains structure evidence only;
it cannot mint citation, source, coverage, or publication authority.

### Other reviewed deltas

The current CodexDesktop-Rebuild and Claude Code deltas do not add a relevant
runtime safety mechanism. Gajae's static Handlebars packaging fix is a useful
plugin-package regression idea. The LazyCodex delta and the full 77/147-commit
Reasonix/Hermes ranges still require recursive code/test/doc/skill/eval/release
review before the all-source audit task can close.

## Current drift and proof boundary

- The dated `upstream-capability-audit-2026-07-10.md` remains an immutable
  historical snapshot and retains its original commit pins.
- `upstream-sources.json` now pins the current checkouts; its audit proves
  source identity and license-object evidence, not capability superiority.
- Existing `internal/upstreamaudit` hard-coded matrices do not execute their
  cited commands, include stale paths/pins, and are excluded from production
  builds. They are not `CapabilityBenchmarkV1` evidence.
- OpenSpec tasks 8.1 through 8.20 remain open. `reached` or `exceeded` cannot be
  true until the current implementation, executable scenario, raw result
  digest, zero-skip result, and current source commit are bound in the new
  matrix.

The immediate implementation order remains: finish the strict bidirectional
MCP contract, close pending approval/user-input revalidation and CAS, build the
single atomic terminal finalizer, add effect disposition and trusted
compaction authority, then execute the complete upstream benchmark and release
matrix.
