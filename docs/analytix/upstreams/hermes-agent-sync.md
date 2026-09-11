# analytix Hermes Agent sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"hermes-agent","reviewedCommit":"d7b36070ef807841699ad32c5b6af547fee3ff64","parity":"not-proven","capabilityBenchmarkV1":null} -->

Hermes Agent is a self-improving-agent and cross-channel operations reference
source. Use it for learning loops, skill evolution, memory/search, gateway
delivery, scheduled automations, terminal backend portability, sub-agent
parallelism, tool RPC, and trajectory/compression research.

Hermes Agent does not own analytix product identity, desktop UI, settings
schema, runtime protocol, model-provider contract, or public CLI. Any useful
idea must land behind analytix-owned contracts and UI.

Hermes Agent is a default research lens for learning and cross-channel
operation, not a fenced category. Future capability reviews may compare Hermes
with Reasonix, OpenCode, Kun, CodexDesktop-Rebuild, current analytix, and later
sources, then absorb whichever design is strongest through analytix-native
contracts.

## Local Source

```text
/Users/sun/Projects/_upstreams/hermes-agent
source: https://github.com/NousResearch/hermes-agent
initial reviewed HEAD: 30e947e0a05ef535e4b25a183d8bbe34fd68d1d5
default branch: main
push remote: disabled locally
```

## 2026-07-20 - Post-audit refresh to v2026.7.20

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `d7b36070ef807841699ad32c5b6af547fee3ff64` |
| Previous pin | `456f18b19c4208115acbf0c6b226af49916b5480` |
| Delta | 38 commits; 87 files, 3,620 insertions, 200 deletions; includes tag `v2026.7.20` plus two later fixes. |
| Dirty state | The previously recorded root/component LICENSE deletions remain local and were preserved across the fast-forward. |
| Fresh upstream tests | Not run: the checkout has no `.venv`; code, tests, workflow, and user documentation were inspected directly. |

The strongest new product-completeness input is the Electron Playwright suite.
It supplies isolated home/user-data fixtures, ambient credential stripping, a
local mock inference endpoint, deterministic viewport/reduced-motion setup,
boot/onboarding/chat/dead-backend scenarios, trace and screenshot artifacts,
and a packaged-binary launch path. Analytix will adapt the fixture isolation,
failure-state coverage, and artifact diagnostics for its real
Renderer -> preload -> main -> Go HTTP/SSE chain. It rejects this suite as
release proof by itself: the packaged test skips when no artifact exists,
uses fake boot rather than a real backend, visual diffs are advisory, Linux
launch disables the sandbox, and the Windows installer job is disabled.
Analytix release rows must run without default skips, exercise the actual Go
runtime/evidence gate, and report every unrun platform as unverified.

Hermes also adds a useful version-tagged LSP freshness model: push and pull
diagnostics are tied to the post-edit document version, and the API separates
fresh empty results from no fresh verdict instead of replaying stale errors.
Analytix will generalize that tri-state discipline to source probes, dataset
snapshots, provider/tool results, and diagnostics: `verified empty`,
`unresolved/stale`, and `failed` remain mechanically distinct. The associated
tests cover stale push, stale pull, slow-but-live servers, explicit wait
budgets, and the rule that slow is not automatically broken.

Other reviewed delta decisions:

- `adapt` Moonshot/Kimi's requirement for an explicit `required: []` on every
  object schema as a provider wire transform, while keeping the original
  closed host schema and Final Evidence Gate authoritative. The upstream
  sanitizer's coercion of malformed schemas into permissive strings/objects is
  rejected for execution authority.
- `adapt` the rule that checkpoint discovery must resolve the exact same task
  workspace path as the file mutation and that gateway checkpoint settings
  reach the real agent. The upstream exception-swallowing path that executes a
  mutation after checkpoint failure is rejected; Analytix must fail closed for
  checkpoint-required effects and bind the pinned file identity.
- `adapt selectively` bounded/configured outbound message sizing and quiet
  period batching for Connect Phone. Configuration cannot permit case facts or
  PII to bypass the accepted-final and projection boundaries.
- `defer` keep-awake UX until it is tied to active, authorized work and power
  assertions are deterministically released on failure, restart, and quit.

This refresh adds benchmark candidates but no reached/exceeded verdict.
`CapabilityBenchmarkV1` remains null.

## 2026-07-20 - Evidence registry and release white-box audit

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `456f18b19c4208115acbf0c6b226af49916b5480` |
| Release relation | 1,677 commits after `v2026.7.7.2`; exact HEAD is untagged |
| Dirty state | Root and several nested license files are locally deleted; those deletions are preserved and are not a reuse source. |
| Fresh upstream tests | Not run; this is static code/test/document evidence only. |

The often-cited Hermes evidence registry is not a runtime or MCP authority. It
is `optional-skills/security/oss-forensics/scripts/evidence-store.py`, packaged
as an optional Skill helper. It stores a complete SHA-256 of `content`, but the
hash does not cover source, URL, verification state, notes, timestamp, or
custody. Verification status is caller supplied, custody is an unsigned append
list, ids are list-length based, and saving directly overwrites one JSON file
without schema versioning, lock, journal, fsync, atomic replace, or crash
recovery. Its tests check basic length/add/list behavior, not tamper, concurrent
writer, or restart authority. The Skill's two-independent-source policy is
prose rather than a mechanical verifier.

Analytix therefore adapts only the useful reporting taxonomy, counterevidence,
redaction, and two-source policy. It rejects `evidence-store.py` as a storage
or runtime base. The current Analytix receipt/registry implementation already
has host-issued grant/tool/result/source binding, complete hashes, signed
heads, a hash chain, content-addressed capsules, fsync, an atomic root index,
cross-process locks, and crash/restart tests. That is implementation evidence,
not yet a production superiority verdict: `internal/runtimeapp/app.go` still
composes the older registry store, while the fresh-witness V2 service is not
the production authority.

Required benchmark strengthening:

- mutate content, source, URL, actor, verification state, notes, and custody,
  including recomputing a local content hash, and require the host authority to
  reject every mutation;
- derive verified state from distinct source receipts rather than accepting a
  caller label;
- cover concurrent writers and every pre/post-commit restart cut;
- add bounded compaction/revocation verification and safe registry metrics
  without logging evidence payloads, PII, or credentials.

Hermes root is MIT, but the local dirty tree removes the root license. The
security-guidance component additionally requires its Apache-2.0 LICENSE and
NOTICE. `skills/productivity/powerpoint/LICENSE.txt` is restrictive and remains
rejected even though the broader research authorization permits studying the
mechanism. Reuse must come from clean committed objects with complete notices.

Hermes test CI can discover optional-skill tests, but its PyPI tag workflow
builds and publishes without running them; the release script also tags before
artifact construction and lacks a clean-tree/test gate. These mechanisms are
not acceptable as an Analytix release floor. Parity remains `not-proven` and
`CapabilityBenchmarkV1` remains null.

## 2026-07-18 - Sixth source refresh and reviewed delta

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `e53f87fe6925f9b0575f16b814d8994dfd455128` |
| Previous pin | `614dc194ea7d853d39f9e84582ec62156f41a475` |
| Delta | 19 commits; 28 files, 1,955 insertions, 301 deletions. |
| Fresh upstream test run | Not run: the required Hermes test launcher has no configured Python environment on this host. |

The reviewed range adds request-local Anthropic transport cleanup, bounded
Telegram reconnect drains with an independent watchdog, one-query session
projection, delivered-response persistence repair, desktop sidebar batching,
and voice/STT deduplication. Analytix will adapt transport ownership, bounded
recovery, one-authoritative-read/multiple-projection, and ordinary UI batching
behind Go-owned contracts. It rejects Python private-socket cleanup, direct
model/recovery `final_response` persistence, reasoning SQLite storage/replay,
and read-failure-to-empty semantics.

The range contains no new DeepSeek cache, MCP identity/allowlist, Evidence
Registry, chain-of-custody, structured-output, Skill, PII/report publication,
eval, or release-gate mechanism. It therefore does not change any reached or
exceeded verdict. `CapabilityBenchmarkV1` remains null and parity remains
`not-proven`.

## 2026-07-18 - Fifth source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `614dc194ea7d853d39f9e84582ec62156f41a475` |
| Previous same-day pin | `2637aa607f2017ec5f638ce1843b946312ccbd48` |
| Delta | 43 commits; 65 files, 5,463 insertions, 370 deletions. |

The range adds Discord missed-message recovery and deduplication, desktop
message/session state repair, bounded MCP polling/OOM coverage, computer-use
delivery fallbacks, model-switch persistence, and broader deterministic tests.
These mechanisms are new P3/P4 comparison inputs; they have not yet been
absorbed or benchmarked, so parity remains `not-proven` and
`capabilityBenchmarkV1` remains null.

## 2026-07-18 - Fourth source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `2637aa607f2017ec5f638ce1843b946312ccbd48` |
| Previous same-day pin | `b1fc6530815ca453d5f2ffd9225ecec35b0d8e93` |
| Delta | 20 commits; 42 files, 2,335 insertions, 122 deletions. |

The range adds reconnect-safe preservation of in-flight desktop turns, repairs
resume and teardown stalls, and prevents empty-response advisories from
spuriously triggering compression. Gateway status, browser, and desktop UI
changes are intake evidence only. Reconnect, restart, compression-trigger, and
stream-abort behavior still require deterministic Analytix benchmarks; no
Hermes capability row is reached or exceeded.

## 2026-07-18 - Third source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `b1fc6530815ca453d5f2ffd9225ecec35b0d8e93` |
| Previous same-day pin | `3d9be2789552a495c7adf30148e867e7614a4bdc` |
| Delta | 51 commits; 160 files, 20,606 insertions, 2,109 deletions. |

The range expands gateway shutdown/watchdog handling, reconnect and bounded
adapter teardown, streaming interruption repair, desktop billing state, and
provider helpers. These are routed to deterministic lifecycle, restart,
single-writer, and stream-abort comparisons. The update is source-identity
evidence only; no Hermes capability row is marked reached or exceeded.

## 2026-07-18 - Second source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `3d9be2789552a495c7adf30148e867e7614a4bdc` |
| Previous same-day pin | `ef9e0c98f5c21b81ec3b85b37c1160efbb3d83d4` |
| Delta | 64 commits; 186 files, 6,713 insertions, 1,101 deletions. |

The range includes stateless delegation return handling, cache-key boundary
hardening, best-effort single-writer stream fencing, app-server live-event
streaming, interruption/session-state fixes, and provider/model-switch work.
Those mechanisms are high-value P1/P4 comparison inputs, but some upstream
choices (including a best-effort missing writer guard) are not automatically
safe enough for Analytix. This is source-identity and intake evidence only;
deep code/test/document admission, implementation, and benchmark parity remain
open.

## 2026-07-18 - Earlier same-day currentness delta

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `ef9e0c98f5c21b81ec3b85b37c1160efbb3d83d4` |
| Previous reviewed pin | `007cd151329c20f9d3854b6338375f3188abc184` |
| Delta | 174 commits; 345 files, 26,820 insertions, 3,623 deletions. |

The changed surface includes live-context runtime-cache isolation, compression
runtime switching, transcript append repair, truthful cron-attempt ledgers,
single-writer stream fencing, MCP/session work, and provider/TUI changes. Those
mechanisms are high-value P1/P4 comparison candidates, but the large delta is
still undergoing code/test/document review. No implementation, parity, cache
advantage, or `CapabilityBenchmarkV1` result is claimed for it yet.

## 2026-07-13 - Prior current intake

| Field | Value |
| --- | --- |
| Branch / commit | `main` / `569b912d7d0931c7256e9f5fb326609e9deda377` |
| License evidence | Root MIT at blob `75410e73319c72cd3e991a501c5455eb78f38375`, with file-level exceptions that the root does not override. |
| Admitted posture | Review each source file and retain its notice; `skills/productivity/powerpoint/LICENSE.txt` is Anthropic all-rights-reserved and rejected, while `plugins/security-guidance/LICENSE` and `plugins/security-guidance/NOTICE` require their own Apache/NOTICE handling. |

Current high-value delta:

| Capability | Analytix decision |
| --- | --- |
| Read-only observer lifecycle with correlation, redaction, and fail-open delivery | `must study` for an Analytix-native Go observer ABI before behavior-changing middleware. |
| SessionSource, restart recovery, FIFO delivery, dedupe/reset semantics | `should absorb` into Connect Phone and durable thread delivery without importing Hermes protocols. |
| Compaction state machine and tool-call/result, role, multimodal, retry/cooldown invariants | `should absorb` as Go loop fixtures and policies. |
| SQLite/WAL/FTS5/CJK search and repair | `should absorb` first as read-only memory/thread search; learned-memory promotion remains opt-in. |
| Local/Docker/SSH/Singularity/Modal/Daytona environment abstraction | `defer`; start with a separately specified local/Docker boundary and retain approval/sandbox/credential ownership. |
| Output-cap retry, request-pressure calculation, and text prefilter before AST discovery added after `bd740f20` | `adapt selectively`; retries must be bounded and structure/support non-increasing, and prefilters may optimize but never replace authoritative parsing. |
| Hermes reasoning/compression visibility | `reject as case-history behavior`; private reasoning still reaches product surfaces in reviewed paths, so analytix must keep zero reasoning bytes in UI/SSE/history/export. |

The older initial intake below is a dated record, not the current Hermes
capability ceiling.

## Absorption Bias

| Area | Default stance |
| --- | --- |
| Closed learning loop | Study memory nudges, skill creation, skill self-improvement, and session search; adapt only through analytix skills/memory contracts. |
| Cross-channel gateway | Compare with Connect Phone, schedule, and any stronger upstream; absorb delivery reliability, not Hermes messaging identity. |
| Sub-agent parallelism | Compare with Reasonix, OpenCode, Kun, CodexDesktop-Rebuild, current analytix, and any stronger source before expanding task-job contracts. |
| Tool RPC and terminal backends | Reuse ideas only behind analytix tool-host, approval, sandbox, and desktop registry contracts. |
| Trajectory compression | Treat as research/benchmark data flow; do not expose as product feature without a spec. |

## 2026-07-02 - Initial Hermes Agent intake

Source:
`NousResearch/hermes-agent` main at
`30e947e0a05ef535e4b25a183d8bbe34fd68d1d5`.

Reviewed by:
Codex.

Related branch:
`main`.

Summary:
Hermes Agent is added as a standing research source for self-improving memory,
skill evolution, cross-channel operation, scheduling, terminal backend
portability, sub-agent parallelism, and trajectory/compression research.

Classification:
| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |
| Closed learning loop | Memory/skills | `needs redesign` | Useful, but must land as analytix-owned skill/memory contracts and not Hermes protocol. | Compare with current analytix Skills, memory, and goal evidence surfaces. |
| Cross-channel gateway | Connect Phone/schedule | `should absorb` | Study delivery, pairing, and interruption semantics against Connect Phone. | Add Connect Phone gateway reliability benchmark before implementation. |
| Sub-agent parallelism | Agent runtime | `defer pending evidence` | Compare with Reasonix and OpenCode before changing task-job contracts. | Add a three-way sub-agent/TODO/task comparison batch. |
| Tool RPC scripts | Tool execution | `optional` | May reduce repeated tool-call context cost if approval/sandbox rules remain stricter. | Prototype only behind analytix tool-host fixtures. |
| Trajectory compression | Benchmark/research | `optional` | Useful for agent-quality datasets, not a user-facing feature yet. | Consider under agent benchmark plan. |

Validation:
Record-only intake. No product code changed.

Remaining risks:
Hermes Agent has broad product surface. Future absorption must avoid importing
Hermes CLI identity, messaging identity, configuration shape, or agent memory
protocol into analytix public contracts.
