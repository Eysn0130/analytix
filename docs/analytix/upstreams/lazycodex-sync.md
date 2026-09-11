# analytix LazyCodex sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"lazycodex","reviewedCommit":"2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Reference / Codex plugin component comparison ledger.
Applies to: committed plugin lifecycle, LSP/CodeGraph, team/worktree,
bootstrap, continuation, and evidence verification components.
Current as of: `2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf` on 2026-07-18.

## Source And Reuse Boundary

| Field | Value |
| --- | --- |
| Local source | `/Users/sun/Projects/_upstreams/lazycodex` |
| Branch / tag | `main` / current tracking branch |
| Reviewed commit | `2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf` |
| Root license evidence | `LICENSE` blob `09aac3c3b7f6e26e8520b362eb048d4d8f9165c3` |
| Gitlink boundary | `src/` is initialized at the parent-pinned OmO commit `65715d1c2c35e27ccf2195ef688b0909dddb403c`; its fetched `origin/dev` is newer and is recorded separately rather than silently replacing the parent gitlink. |
| License posture | MIT root, but gitlinks and component third-party notices require file-level review |

The committed repository contains real code; it is not merely a methodology
reference. CodeGraph, frontend, and other components carry their own Apache or
third-party notices and must not inherit the root MIT classification blindly.

## 2026-07-20 - Verified-claim and completion-gate white-box audit

| Field | Value |
| --- | --- |
| Distribution commit | `main` / `2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf` (`v4.19.0`) |
| Parent-pinned OmO gitlink | `65715d1c2c35e27ccf2195ef688b0909dddb403c` |
| Fetched OmO comparison head | `origin/dev` / `50d9307ea36f775298d50f035f4b0c03d0925efd`; this is not the parent-selected source and was not substituted into the LazyCodex checkout. |
| Nested LSP gitlink / fetched head | `d1ff1681c1f558e062ac33fad4d835baaa7b5edf` / `183a4f402556890a25344eb0a91b4680f7e30e0b` |
| Worktree preservation | Pre-existing deleted license files were not restored, removed, or staged. License conclusions use the pinned Git blobs and component notices, not the dirty filesystem copy. |

Current implementation and test evidence:

| Capability | Current evidence | Decision and analytix requirement |
| --- | --- | --- |
| Claim graph and counterevidence | `plugins/omo/skills/ulw-research/SKILL.md` defines claim status (`supported`, `partial`, `refuted`, `unresolved`), supporting and contradicting observations, independent groups, counter-search, primary-source and temporal requirements, plus a `verified-claims` synthesis allowlist. | `adapt` the epistemic model into typed `ClaimRecord` and host registries. For case facts, Markdown and orchestrator discipline are not authority; registry membership, exact semantic/field support, same context/snapshot receipts, counterevidence, and Final Evidence Gate enforcement are mandatory. |
| Enforceability of the claim allowlist | Repository-wide search finds the claim graph and `verified-claims` rules only in Skill prose and attribution; no parser, validator, publication gate, or deterministic test consumes those files. | `reject` as an implemented safety gate. The Analytix benchmark must show a free-text claim cannot bypass the typed claim allowlist, including failure/recovery/report/export paths. |
| Subagent completion receipt | `components/lazycodex-executor-verify/src/codex-hook.ts` checks that selected worker types report a non-empty regular file whose realpath stays under `<cwd>/.omo/evidence/`; traversal, directory, zero-byte, and symlink cases are covered. | `adapt` realpath/no-follow/contained-artifact checks, but only as artifact hygiene. File existence and non-empty bytes are never evidence of truth. |
| Completion fail-closed behavior | The verifier passes without a receipt after three blocked attempts, passes immediately on context-pressure transcript markers, silently accepts malformed/unknown hook input and unrelated roles, and does not hash or inspect receipt content or bind it to thread/turn/case/epoch/snapshot/tool execution. | `reject`. Analytix must fail closed on malformed state, never waive evidence because of retries/compaction, and require host-issued receipt identity and semantic support. |
| Test evidence | Root `npm test` passed 9/9 and proves distribution metadata/docs/launcher behavior. Focused verifier source test passed 21/21 via `npx --yes vitest@4.1.8 --run --config <empty-temp-config> test/codex-hook.test.ts`. | Treat as implementation truth for both its path protections and intentional fail-open escape hatches. Neither suite validates claim semantics or case publication. |

This audit changes the earlier `src/` limitation from uninitialized to a
three-way currentness record: parent-pinned gitlink, shipped generated plugin
snapshot, and newer fetched OmO head. They must not be conflated. No LazyCodex
row is reached or exceeded until the Analytix implementation and executable
CapabilityBenchmarkV1 rows pass.

## 2026-07-18 - Second source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `2a03f45f1bbc1d5d0cbf6c1293888e9df3ccbebf` |
| Previous same-day pin | `f2e96afcb5b631302081fb9e17de983639f80d29` |
| Delta | One commit; 92 files, 4,551 insertions, 700 deletions. |

The commit synchronizes the Codex marketplace to v4.19.0 and changes generated
bundles, component manifests, hooks, continuation plan checks, migration
guards, and tests. Generated output and marketplace synchronization are not
independent capability proof. File-level source/provenance review and
deterministic admission tests remain required before reuse or parity claims.

## High-Value Comparison Areas

| Area | Analytix decision |
| --- | --- |
| LSP daemon and CodeGraph bridge | Preferred as an Analytix Hub/MCP plugin, not Go-core tool-schema growth. |
| Team/worktree lifecycle | Compare receipts and role gates; keep Analytix parent-owned worktree acceptance. |
| Evidence executor verify | Port realpath, symlink, root, non-empty, retry, and atomic-state invariants into Analytix evidence receipts. |
| Bootstrap/degraded ledger | Reuse checksum and atomic-degraded-state ideas after platform truthfulness tests. |
| Continuation/rules/comment checks | Compare as optional plugin components; do not inject broad default prompt text. |
| Telemetry | Reject default opt-out telemetry; any Analytix telemetry requires explicit product/privacy approval. |

## 2026-07-13 Intake

The plugin README undercounts committed components, and Windows installation
documents disagree: one path says native Windows works while another recommends
WSL2; the PowerShell bootstrap records a degraded hint rather than running the
claimed `winget install`. Future absorption must prove actual behavior from
code and platform tests.
