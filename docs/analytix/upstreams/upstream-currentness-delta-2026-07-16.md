# Upstream Currentness Delta - 2026-07-16

Status: source intake and two focused absorptions only. This report records the
latest fetched pins, the preserved dirty-worktree boundary, reviewed code and
test evidence, and the resulting Analytix decisions. It is not
`CapabilityBenchmarkV1` evidence, does not complete OpenSpec task 8.2, and does
not set any source to `reached` or `exceeded`.

## Update And Preservation Boundary

All eleven registered repositories were fetched with remote pruning and tag
discovery. Every checkout now has the exact tracking-branch commit at `HEAD`
and reports `0 0` for `HEAD...@{upstream}`. Clean worktrees were updated with
`pull --ff-only`; dirty worktrees were advanced only after comparing incoming
paths with existing changes and then using `merge --ff-only`. No repository was
stashed, reset, force-updated, cleaned, staged, or rebased.

The existing dirty paths remain unchanged: they are tracked license deletions
in DeepSeek-Reasonix, Kun, claude-code, claw-code, Gajae, Hermes, LazyCodex, and
OpenCode, plus Kun's untracked `screenlog.0`. License and provenance evidence
must therefore be read from the pinned Git objects rather than from the mutable
checkout. The user has separately declared code-level authorization; original
notices, provenance, excluded-subtree conditions, and file-level obligations
remain part of admission.

Gajae's initial tag fetch rejected remote tag updates for `v0.10.1` and
`v0.9.6` because they would clobber local tags. The branch-only pruned fetch
succeeded and the tracked branch is current. The tags were deliberately not
force-updated and are not used as review authority.

| Source | Latest reviewed branch pin | Commits since 2026-07-14 pin | Dirty entries | Root license object | Parity |
|---|---|---:|---:|---|---|
| CodexDesktop-Rebuild | `02d73c8053edfe0436d288cc9023c9f513ffc1e1` | 1 | 0 | no root license blob | not-proven |
| DeepSeek-Reasonix | `6826411dd158c9594465864090b91fd0f3f86807` | 80 | 1 | MIT, `bc45a281d8050c59c9b833ea2d0b1fb6e02602c0` | not-proven |
| Kun | `9fb5ecf90430bca3aff2fad4fa563f7b69b3ee80` | 0 | 3 | PolyForm Noncommercial 1.0, `52c91ecbe223fed6161e599ccc9b704235e06dc4` | not-proven |
| claude-code | `c39cb0f14bfe8bb519bae5bfc55add6867c5e2ab` | 1 | 1 | Anthropic commercial terms, `645a5d67c6d1e16437bec850dad5bd3b6c81f77c` | not-proven |
| claw-code | `4ea31c1bc91c4e9bcbd67d51c550c01e127e6d0d` | 0 | 1 | MIT, `28e6960dd9a2be209c308b49bc9a8973dbf4d60d` | not-proven |
| gajae-code | `7dc297145f333a00b7e913ce7c8cd5dedeb3fd34` | 131 | 3 | MIT, `16eb3fc020a9dbefb165d2fb1d4597d2203c44bd` | not-proven |
| hermes-agent | `007cd151329c20f9d3854b6338375f3188abc184` | 290 | 5 | MIT, `75410e73319c72cd3e991a501c5455eb78f38375` | not-proven |
| instructor | `47fdb2ca07119d389a3c0e8bc28b9930b814f294` | 0 | 0 | MIT, `f3325f8da4271c8e711369623d138881737177cf` | not-proven |
| lazycodex | `f2e96afcb5b631302081fb9e17de983639f80d29` | 1 | 12 | MIT, `09aac3c3b7f6e26e8520b362eb048d4d8f9165c3` | not-proven |
| opencode | `ef3b67308411614b7a08c2ac81931d930e22c835` | 15 | 4 | MIT, `6439474beed8e0271df9862eff97ffd70ec2464c` | not-proven |
| postgres-mcp | `07eb329c8c48e49640e0d1b5b35465d4d024c3ee` | 0 | 0 | MIT, `49eef5868972bb002014f178b491752cfd50409f` | not-proven |

The exact aggregate deltas from the 2026-07-14 reviewed pins are: 275 files
for Reasonix, 953 for Gajae, 612 for Hermes, 96 for LazyCodex, and 80 for
OpenCode. Those large surfaces have not all been admitted or benchmarked in
this slice.

## Code-And-Test Review Decisions

| Source and mechanism | Upstream implementation/test evidence | Analytix decision | Analytix implementation/test evidence | Current result |
|---|---|---|---|---|
| DeepSeek-Reasonix current reader trust and dispatch revalidation | `internal/mcptrust/trust.go`, `internal/plugin/plugin.go`, `internal/tool/tool.go`; commits `5092b23d` and `9bbd1386`; trust and plugin tests | Adapt the current-run allowlist and second resolution, but retain Analytix's stricter exact `ExecutionGrant` binding to case context, server identity, connection epoch, schema hash, read-only policy, expiry, and live source probe. Reject annotation-stripped fingerprints as a replacement for the execution grant. | `packages/runtime-go/internal/mcp/manager.go` recalculates the managed schema hash under the source execution lock immediately before dispatch; `packages/runtime-go/internal/mcp/manager_test.go` covers frozen advertisement and catalog fingerprint drift. | Implemented before this intake; parity benchmark still required. |
| DeepSeek-Reasonix schema/cache stability | `internal/provider/schema_canonicalize.go`, `internal/mcptrust/trust.go`, `internal/mcptrust/trust_test.go`; upstream preserves `dependentRequired` and legacy `dependencies` constraints in the security fingerprint | Adapt semantic set normalization for draft-07 `dependencies`, including schema-form values, without removing descriptions from Analytix's provider-visible or grant hash. Equivalent contracts should be stable; a structural dependency change must invalidate authority. | `packages/runtime-go/internal/domain/model/schema.go`; `TestCanonicalJSONSchemaNormalizesDraft7Dependencies`; `TestToolSchemaHashBindsCanonicalDraft7Dependencies`; dispatch-time mismatch remains enforced by `managedToolSchemaHash`. | Focused absorption implemented and passing; live cache A/B proof remains open. |
| DeepSeek-Reasonix private verbatim artifact boundary | `internal/jobs/jobs.go`, `internal/jobs/artifacts.go`, `internal/agent/session_events.go`; commit `6826411d`; artifact, boot, config-migration, and session-event tests preserve exact bytes only after tightening directories to `0700` and files to `0600` | Adapt private permissions, exact-byte preservation, and upgrade hardening only for an authorized controlled artifact. Reject Reasonix's restored unredacted ordinary job/session behavior for Analytix chat, SSE, history, logs, compaction, and normal reports. | `src/main/controlled-artifact/file-effects.ts`, `release-effects.ts`, and their tests stage exact bytes in a private per-launch directory and publish without overwrite; `host.ts` keeps release capabilities out of JSON/IPC. Production release remains fail-closed until witnessed evidence authority and `PublicationReceipt` composition are live. | File-effect absorption implemented and passing; end-to-end authority composition remains open. |
| Hermes retry/flush reasoning scrubber | `agent/think_scrubber.py`; commits `a569226f8` and `8b209e0dd`; `tests/agent/test_think_scrubber.py` re-arms a filter after flush and verifies retry isolation | Adapt the invariant, not the Python object lifecycle. Analytix creates a new `PublicTextFilter` for every physical provider attempt and now distinguishes private reasoning receipt from public/tool output that makes a retry ambiguous. A failed reasoning-only attempt is discarded before a same-authority retry. | `packages/runtime-go/internal/app/loop/provider_stream.go`; `TestStreamProviderWithRetryDiscardsFailedAttemptReasoningBeforeRetry`; existing cancellation, SSE, persistence, history, compaction, and export reasoning tests remain authoritative for their boundaries. | Focused absorption implemented and passing; full terminal-path matrix remains open. |
| Hermes scoped background/provider/OAuth delta | `gateway/run.py`, `tools/mcp_oauth.py`, `agent/lmstudio_reasoning.py`, `plugins/model-providers/ollama-cloud/__init__.py`; commits `58010c8b`, `8091c440`, `9078a838`, `5d9a72b7`; focused scope, OAuth, and provider tests | Adapt the two deterministic properties: every background execution must reinstall and revalidate its frozen authority scope, and provider-specific reasoning effort must be capability-gated and monotonic. OAuth callback reuse is admissible only with loopback identity, expiry, one-run ownership, and connection-epoch validation. It does not establish source readiness or evidence. | Existing Analytix context/grant/provider contracts are partial primitives; no production implementation or parity claim is made for this newly fetched delta. | Intake only; benchmark and implementation remain open. |
| Gajae bounded invalid-prompt circuit breaker, descriptor-bound resume, re-minted aggregate receipts, interrupted child preservation, and workflow-gate correlation | commits `cf94f880`, `672890d3`, `fd9ab345`, `0afb4e95`, `91e8652e`, and related coding-agent tests | Candidate adaptations. Any retry must remain structure-only or transport-only and cannot mint evidence; resume must revalidate frozen context and exact grant; child projections cannot complete the parent Goal. | Existing Analytix pending-work, continuation, grant, and parent-owned goal contracts provide partial primitives. No new implementation is claimed in this slice. | Intake only. |
| OpenCode configurable nested subagent depth | `packages/opencode/src/tool/task.ts`; commit `285d315b4e`; `packages/opencode/test/tool/task.test.ts` | Reject configurable recursion as a default high-risk capability. Analytix child agents currently receive no subagent/job/goal/todo/plan/user-input tools, which is a stricter floor; any future depth support needs shared budgets and parent-owned evidence closure. | Current subagent profile and manifest tests; a direct cross-project benchmark row is still required. | Existing stricter policy, superiority not yet proven. |
| LazyCodex CodeGraph MCP bridge lifecycle | `plugins/omo/components/codegraph/src/mcp-bridge.ts`; `serve-mcp-bridge-lifecycle.test.ts` preserves a parent-output error even when the child has already exited | Adapt the error-precedence and child-pipe settlement property to Analytix MCP stdio lifecycle after comparing current cancellation and response-settlement behavior. Do not copy generated `dist` files. | No implementation is claimed in this slice. | Intake only. |
| OpenCode v1.18.3 release synchronization | commit `ef3b6730` changes package versions and `bun.lock` only | Record the new pin; do not manufacture a capability delta or parity claim from release metadata. | No Analytix implementation is required for this delta. | Current pin recorded; prior capability obligations remain open. |
| CodexDesktop-Rebuild, claude-code, Kun, Claw, Instructor, postgres-mcp | CodexDesktop and claude-code deltas are release/version or public-feed changes; Kun, Claw, Instructor, and postgres-mcp have no commits after the prior pin | Retain prior decisions and benchmarks; do not infer parity from a zero delta. | See the 2026-07-14 currentness report and source-specific sync files. | Prior intake remains current; benchmark rows remain open. |

## Focused Validation Recorded In This Slice

The following commands passed against the working tree:

```text
npm run audit:upstreams

(cd packages/runtime-go && go test ./internal/app/loop -run 'TestStreamProviderWithRetry(DiscardsFailedAttemptReasoningBeforeRetry|RetriesBeforeVisibleOutput|KeepsReasoningPrivateWhileStreamingText|RejectsOutputReturnedAfterHostCancellation)$' -count=1)

(cd packages/runtime-go && go test ./internal/domain/model ./internal/app/toolcatalog ./internal/mcp -run 'Test(CanonicalJSONSchemaNormalizesDraft7Dependencies|ToolSchemaHashBindsCanonicalDraft7Dependencies|ProductionManagerCatalogFingerprintCanonicalizesSchemas|MCPToolAdvertisementSnapshotFreezesSchemaPolicyEpochAndIdentityAtomically)$' -count=1)
```

These results prove only the named deterministic properties. They do not
replace `go test ./...`, race and production-tag runs, provider A/B cache
measurements, funds-plugin gates, desktop E2E, packaging, or release metrics.

## Open Proof Obligations

- Audit the remaining changed code and tests in the 80/131/290/15-commit
  Reasonix, Gajae, Hermes, and OpenCode deltas; record file-level provenance for
  any admitted source.
- Run a fixed-pin, same-account, same-model, same-endpoint DeepSeek A/B benchmark
  using provider-native cache hit/miss counters and signed Analytix physical
  attempt telemetry. Synthetic percentages do not prove superiority.
- Add deterministic benchmark rows for MCP trust drift, schema dependency
  constraints, retry reasoning isolation, descriptor-bound resume, child-depth
  denial, and MCP child/output settlement.
- Complete funds-plugin SQL AST, evidence, coverage, report publication, PII,
  desktop delivery, and packaged-runtime acceptance. No upstream intake can
  substitute for those product-specific gates.

Until these obligations and the complete OpenSpec tasks are closed, the
UpstreamCapabilityMatrix must keep the affected rows at `not-proven`.
