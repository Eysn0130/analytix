# analytix Claude Code sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"claude-code","reviewedCommit":"015170d3fd84fb57ef4685a64b673fadd0690dc1","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Reference / behavior-only comparison ledger.
Applies to: Claude Code behavior, deployment, plugin, and safety research.
Current as of: `015170d3fd84fb57ef4685a64b673fadd0690dc1` on 2026-07-20.
Source of truth: the pinned Git object and current Analytix code/tests.

## Source And Reuse Boundary

| Field | Value |
| --- | --- |
| Local source | `/Users/sun/Projects/_upstreams/claude-code` |
| Branch / tag | `main` / current tracking branch |
| Reviewed commit | `015170d3fd84fb57ef4685a64b673fadd0690dc1` |
| License evidence | `LICENSE.md` blob `645a5d67c6d1e16437bec850dad5bd3b6c81f77c` |
| License posture | Anthropic all-rights-reserved, subject to commercial terms |
| Allowed default use | Behavior observation and clean-room requirements/tests only |

The repository is not an open-source implementation source. Direct reuse
requires the separately granted authorization to identify the exact material
and still requires provenance and modification records. Public behavior can
always inform independently written requirements and acceptance tests.

## 2026-07-20 - White-box boundary and repository drift audit

This checkout is not the production Claude Code core. It contains release
history, plugins, hooks, examples, and workflows, but no buildable core runtime
or core test suite. Consequently, `CHANGELOG.md` is behavior to benchmark, not
evidence that an inspectable implementation meets that behavior. The current
dirty deletion of `LICENSE.md` was preserved; the license classification above
comes from the tracked Git object.

| Capability | Current source/test evidence | Analytix decision |
| --- | --- | --- |
| Hookify rule engine | `plugins/hookify/core/rule_engine.py` has a bounded compiled-regex cache and combines block/warn results. Unknown operators, field-read failures, and invalid regexes evaluate as no match. | `reject` as authority because a damaged security rule fails open. Keep only the bounded-cache idea for non-authoritative developer diagnostics. |
| Security pattern identities | `plugins/security-guidance/hooks/patterns.py` assigns stable rule ids, verifies its inventory at import, and emits bitmask telemetry. | `adapt` stable diagnostic identities, but never use regex or an LLM hook as an `ExecutionGrant`, evidence, or publication gate. |
| Hook schema validation | `plugins/plugin-dev/skills/hook-development/scripts/validate-hook-schema.sh` expects events at the JSON root, while all five repository-owned `hooks.json` files wrap them under `hooks`. The validator failed on 5/5 manifests with `jq: Cannot index string with number`. | Record upstream drift and add wrapper/schema compatibility to deterministic plugin admission tests; do not inherit the validator. |
| Transcript and reasoning | Current release notes permit reasoning effort in transcripts and subagent thinking in stream JSON. | `reject`; Analytix must keep reasoning bytes at zero across stream, persistence, history, compaction, report, and export. |
| Core Agent features | Permission parsing, long-command approval, transcript integrity, bounded retention, stable cache blocks, background completion, and structured-output behavior appear only as release claims in this repository. | `benchmark-only` until an executable upstream artifact or public black-box fixture proves the claim. Reimplement clean-room tests rather than treating prose as parity evidence. |

Fresh checks passed Python syntax for 21 files and shell syntax for 20 files.
There is no current core-runtime test command to run. The 5/5 manifest failure
is an upstream defect, not an Analytix exception or a reason to weaken plugin
schema admission.

CapabilityBenchmarkV1 must cover fail-closed permission parsing for shell
composition and path/worktree escapes, bounded long-session state, authentic
background completion, schema failure without free-text fallback, and
provider-native cache-break telemetry. These rows remain `not-proven` until
their Analytix implementations and deterministic tests pass.

The second 2026-07-18 refresh from `67f390c9a0b1440d369aebe2ff6a5023db35bf8e`
contains one commit across `CHANGELOG.md` and `feed.xml` (104 insertions and 27
deletions). It does not expose a new implementation mechanism or prove product
parity. The marker records source identity only; behavior admission and
`CapabilityBenchmarkV1` remain open.

## Current Research Lens

| Area | Analytix decision |
| --- | --- |
| Durable daemon and attach/resume | Study behavior and recovery invariants; reimplement through Analytix thread/runtime contracts. |
| Background agents and worktree isolation | Compare UX with Analytix durable jobs and reviewed worktree receipts; keep Analytix's stricter ownership gates. |
| Remote Control | Treat as a clean-room product/security research item, not a protocol source. |
| Enterprise managed settings | Study MDM/GPO precedence, lock indicators, and auditability through a separate enterprise policy spec. |
| Plugins, commands, agents, skills, hooks, MCP | Use only as lifecycle and security-review behavior input. |
| Multi-agent review | Compare review quality and evidence closure; do not copy prompts. |

## 2026-07-13 Intake

The current repository primarily provides release history, plugin examples,
deployment examples, and links to official documentation; it is not the core
product source. High-value behavior deltas include durable background state,
restart attach/resume, remote control, managed settings, worktree confirmation,
plugin lifecycle, layered security review, transcript integrity, and
accessibility improvements.

Analytix has explicit implementations and tests for evidence-backed Goal/Todo
closure, a default read-only child policy, parent-owned worktree review, and
durable task-job diagnostics. These are comparison candidates, not a current
superiority claim: `CapabilityBenchmarkV1` has not yet established parity or
exceeded status for this reviewed commit.

Remaining risks:

- release notes and public behavior do not prove internal architecture;
- commercial terms can change independently of this snapshot;
- behavior parity is not permission to copy implementation or text.
