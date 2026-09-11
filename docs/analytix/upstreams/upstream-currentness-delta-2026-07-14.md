# Upstream Currentness Delta - 2026-07-14

Status: source intake only. This report updates reproducible review pins and
records implementation leads. It is not `CapabilityBenchmarkV1` evidence and
does not set any source to `reached` or `exceeded`.

## Update And Worktree Boundary

All eleven registered repositories were fetched with remote pruning. Checkouts
that were behind their configured tracking branch were advanced using
fast-forward-only merges after verifying that the incoming paths did not
overlap existing dirty paths. Every checkout now reports `0 0` for
`HEAD...@{upstream}`.

Existing upstream worktree changes were preserved. In particular, tracked
license files are deleted in several local worktrees and Kun has an untracked
`screenlog.0`; none was restored, removed, staged, or rewritten. License
evidence below is read from the pinned Git object with `git show`/`ls-tree`,
not from the mutable worktree.

| Source | Reviewed commit | Root license evidence | Parity |
|---|---|---|---|
| CodexDesktop-Rebuild | `d466140e11bbb5d14b28e3d0f0ed01f7fae0fc19` | no root license blob at HEAD | not-proven |
| DeepSeek-Reasonix | `ad9c3fc138b3e7b953405d94b96027b3275c4a50` | MIT, blob `bc45a281d8050c59c9b833ea2d0b1fb6e02602c0` | not-proven |
| Kun | `9fb5ecf90430bca3aff2fad4fa563f7b69b3ee80` | PolyForm Noncommercial 1.0, blob `52c91ecbe223fed6161e599ccc9b704235e06dc4` | not-proven |
| claude-code | `b7784f2c63ed4585c32bc20b94d3b64cf4fe6df3` | Anthropic commercial terms, blob `645a5d67c6d1e16437bec850dad5bd3b6c81f77c` | not-proven |
| claw-code | `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` | MIT, blob `28e6960dd9a2be209c308b49bc9a8973dbf4d60d` | not-proven |
| gajae-code | `774bc1677190804017eda6ef8eef6654e40703cd` | MIT, blob `16eb3fc020a9dbefb165d2fb1d4597d2203c44bd` | not-proven |
| hermes-agent | `569b912d7d0931c7256e9f5fb326609e9deda377` | MIT, blob `75410e73319c72cd3e991a501c5455eb78f38375` | not-proven |
| instructor | `47fdb2ca07119d389a3c0e8bc28b9930b814f294` | MIT, blob `f3325f8da4271c8e711369623d138881737177cf` | not-proven |
| lazycodex | `098177c52a4fc989cc1f8ac6fb3f94e330fb63d3` | MIT, blob `09aac3c3b7f6e26e8520b362eb048d4d8f9165c3` | not-proven |
| opencode | `05c3e40a4e641732b991499000ca479e5dad4b02` | MIT, blob `6439474beed8e0271df9862eff97ffd70ec2464c` | not-proven |
| postgres-mcp | `07eb329c8c48e49640e0d1b5b35465d4d024c3ee` | MIT, blob `49eef5868972bb002014f178b491752cfd50409f` | not-proven |

The thread records a user-declared code-level authorization. Original license,
notice, provenance, and excluded-subtree obligations remain recorded and must
still be enforced per admitted file.

## Exact Delta Boundary

Against each repository's `.analytix-audit-latest` marker, the current delta is
77 commits for DeepSeek-Reasonix, 2 for claude-code, 90 for Hermes, and 13 for
OpenCode. Larger recent-log review windows are useful for discovery but are not
the exact delta and must not be presented as such.

## Adopt, Adapt, Reject Intake

### DeepSeek-Reasonix

- Adopt the byte-stable system prefix discipline, canonical tool ordering,
  recursive schema canonicalization, provider-native cached-token parsing,
  terminal-stream usage checks, and independent offline recovery guard.
- Adapt pairing repair as transport compatibility only. A synthetic tool id or
  result must never issue an Analytix EvidenceReceipt.
- Adapt safe mode and hash-verified release-unit rollback so the Final Evidence
  Gate remains enabled after rollback.
- Reject persisted or replayed reasoning content, optional/skip-based release
  cache gates, and model-authored repair plans as host authority.

Reasonix's `PrefixShape` covers logical system/tools rather than the exact
serialized HTTP prefix and uses a shortened diagnostic hash. Its greater-than
90-percent cache test is synthetic or optional in release environments. This
is useful design input, not proof that either product wins a head-to-head
cache comparison.

### Kun, Hermes, OpenCode, And Agent Control

- Adopt Kun's shared advertisement/execution capability resolver and
  execution-time second resolution; strengthen it with the exact Analytix
  ExecutionGrant fields.
- Adapt Hermes restored delegation ownership, empty-stream failure, MCP
  resource bounds, and ordered parallel barriers to exact
  thread/turn/case/epoch/snapshot ownership and receipt hashes.
- Adapt OpenCode per-server permission state and post-lineage recheck; reject
  directory/session autoaccept that lacks exact grant, epoch, expiry, and
  server identity.
- Reject synthetic provider filler such as `Done.` as evidence or case fact.

### Evidence, Reports, And Structured Output

- Reimplement Claw's versioned fact/inference/lead and checked-no-hit versus
  unchecked report taxonomy in Go. Do not reuse its arbitrary evidence string,
  evidence-optional observed fact, or short hash authority.
- Adopt Gajae's full-hash/path-contained workflow receipts and privacy-safe
  child synopsis. Reject citation readiness inferred merely from provider
  search signals and fail-open damaged workflow state.
- Adopt LazyCodex's verified-claim/counterevidence/restraint concepts, but
  reject its executable non-empty-string evidence check.
- Adapt Instructor's schema-first typed parsing, typed exhaustion, and bounded
  retry to one structure-only repair whose fact/support/scope hashes cannot
  increase. Reject free-text bypasses, raw hook logging, and hook failure that
  permits high-risk publication.

### Data And SQL

- Adapt postgres-mcp's AST inspection, database read-only transaction, and
  timeout. Analytix must default to restricted SELECT/WITH, add exact table and
  function allowlists, use bounded fetch, publish row/byte limits as partial
  coverage, and emit typed diagnostics without SQL/PII.
- Reject postgres-mcp's unrestricted default, administrative statement set,
  unbounded `fetchall`, and raw SQL/error logging.

### Desktop And Release

CodexDesktop-Rebuild demonstrates packaging breadth but its current workflow
does not prove signed, checksummed, tested, or integrity-hardened release
readiness. Analytix parity requires a real Renderer -> preload -> main -> Go
startup and gate test, signed/current-platform package evidence, checksums and
provenance, and explicit unverified rows for unrun platforms.

## Cache Superiority Proof Still Required

`CacheVisibleShapeV1` and signed per-physical-attempt telemetry in Analytix are
stronger authority primitives than the reviewed Reasonix telemetry, but the
current code does not yet prove actual serialized prefix byte stability across
dynamic tails. Task 8.21 therefore remains open.

The deterministic benchmark must pin Reasonix at the commit above, use the
same DeepSeek account/model/region/endpoint/credential scope, interleave A/B
runs, and separate unknown from zero. At minimum it must measure provider-native
hit, miss, and input tokens for multi-turn, tool-order/schema-order variation,
multi-tool, malformed pairing, reconnect, retry, approval resume, restart,
compaction, and unavailable recovery.

Required non-secret measurements include exact or HMAC-bound serialized prefix
length/hash, canonical tool schema hash, completed-history prefix hash, signed
attempt count, and settlement completeness. Raw prompts, PII, reasoning,
credentials, and response bodies must not enter benchmark ledgers.

No cache or platform `exceeded=true` result is valid until implementation,
zero-skip deterministic tests, fixed-pin raw results, and release-gate evidence
all exist.
