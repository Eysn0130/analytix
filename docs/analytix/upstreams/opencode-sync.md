# analytix OpenCode sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"opencode","reviewedCommit":"849c2598abc7d2b40261e74b5826bc74ffc78308","parity":"not-proven","capabilityBenchmarkV1":null} -->

OpenCode is an auxiliary agent-architecture comparison source. Use it to
compare against DeepSeek-Reasonix, Kun, and analytix for provider registry,
sub-agent, TODO/task, permission, session-continuation, and background-work
designs.

OpenCode does not own analytix product identity, UI, settings schema, runtime
protocol, or desktop workflow.

OpenCode's role here is a default research lens, not an exclusive boundary.
Any capability can compare OpenCode against Reasonix, Kun, Hermes Agent,
CodexDesktop-Rebuild, current analytix, or future sources before deciding what
to absorb.

## Local Source

```text
/Users/sun/Projects/_upstreams/opencode
```

## 2026-07-20 - Provider transform and agent-instruction refresh

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `849c2598abc7d2b40261e74b5826bc74ffc78308` |
| Previous pin | `4cc022481c184cb9d5e14f7e882b6749d2414553` |
| Delta | Three commits; 21 files, 133 insertions, 98 deletions. |
| Dirty state | The previously recorded root/package LICENSE deletions remain local and were preserved across the fast-forward. |
| Fresh upstream tests | Not run: `node_modules` and a local Bun test runner are absent. Source and focused test changes were inspected directly. |

The only executable provider change broadens Mistral-family detection to
Codestral, Pixtral, and Mixtral model ids before normalizing paired tool-call
and tool-result ids to the provider's nine-character alphanumeric wire shape.
Its focused test proves both sides of one pair receive the same transformed
id, including a custom OpenAI-compatible endpoint. Analytix will adapt the
compatibility need through a typed per-request internal-id <-> wire-id map
with collision detection and exact result pairing. It rejects mutating
history in place or truncating ids without collision proof, because two calls
sharing the first nine alphanumerics must never alias one ExecutionGrant.

The updated Meta system prompt adds useful evidence-before-synthesis,
constraint retention, execution verification, and parallelism guidance. This
is instruction quality, not host authority, and therefore cannot satisfy any
Analytix evidence, tool, privacy, or publication row. The provider options
delta also removes a Meta `reasoningEffort=xhigh` default; it provides no cache
or safety advantage. The previously audited structured-final implementation
is unchanged, so its free-tool-choice and reasoning-persistence limitations
remain. No parity or superiority verdict changes; `CapabilityBenchmarkV1`
remains null.

## 2026-07-20 - Current structured-final and typed-part white-box audit

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `4cc022481c184cb9d5e14f7e882b6749d2414553` |
| Audit scope | Structured output, final-tool requirement, typed source/tool parts, retry behavior, reasoning persistence/export, permissions, and current delta. |
| Current delta | `b8142c7..4cc0224` is primarily app/UI, provider transform/error, import diagnostics, generated/version, and release maintenance. It does not close the case-evidence or publication gaps below. |

Current implementation and decisions:

| Capability | Current implementation evidence | Decision and analytix requirement |
| --- | --- | --- |
| Required structured final | `packages/opencode/src/session/prompt.ts` adds a schema-backed `StructuredOutput` tool, appends a dedicated system instruction, sets `toolChoice: "required"`, captures validated arguments host-side, and records `StructuredOutputError` when a provider finishes without structured output. | `adapt` the required final-tool and host capture pattern. Analytix must make its discriminated `FinalAnswerEnvelope` the only publishable terminal, independently of provider support or prompt compliance. |
| Schema validation | `createStructuredOutputTool` removes `$schema`, passes the requested JSON Schema to the AI SDK, and only captures arguments after SDK validation. | `adapt` for structure only. Analytix additionally closes nested objects by default, validates locally at every external boundary, and verifies that a repair never adds facts, evidence ids, or scope. |
| Required-tool precision | `toolChoice: "required"` requires some tool, not specifically `StructuredOutput`; all resolved tools remain present. The description says “exactly once”, but host code does not count calls, and provider text/tool parts can be persisted before the structured call. | `reject` as a case publication gate. High-risk fact text must remain attempt-local and invisible until one exact final envelope passes the host gate; wrong/multiple final calls fail closed. |
| Retry contract | `OutputFormatJsonSchema.retryCount` defaults to 2 and is persisted/tested, but the current prompt loop does not consume it; missing structured output records `retries: 0`. Unit comments claim retry is handled by the loop without executable code demonstrating that behavior. | `reject` the declared-but-unused retry behavior. Analytix permits at most one structure-only repair and compares before/after fact/support/scope hashes. |
| Typed source and tool parts | `packages/schema/src/v1/session.ts` defines discriminated file/symbol/resource sources and pending/running/completed/error tool states, which is materially stronger than untyped transcript strings. | `adapt`; replace open `Any` metadata/input/output and raw-string results at case boundaries with strict source/tool/outcome projections plus host identity, grant, context, receipt, semantic status, and PII policy. |
| Reasoning isolation | `packages/opencode/src/session/processor.ts` persists reasoning parts and deltas; `packages/opencode/src/cli/cmd/export.ts` exports a redacted reasoning field. | `reject` for Analytix. Redaction is not zero-byte isolation: reasoning must never enter SSE, UI, event stores, history, compaction, reports, or exports. Only numeric usage may persist. |
| Permission model | Tool resolution composes agent/session rules and performs runtime permission requests, which is a useful generic agent pattern. | `adapt`, but an approval is not a case-grade grant. Analytix revalidates exact context digest, case/epoch/snapshot, provider/server/tool/schema/arguments/scope, expiry, and effect class on execute/resume/restart. |
| Test evidence | Source tests deeply cover schema/tool construction, while live integration tests skip without an Anthropic key. The focused current command `bun test test/session/structured-output.test.ts --timeout 30000` could not start because workspace dependencies were absent (`preload not found "@opentui/solid/preload"`). | Record as unrun, not passed. The source assertions and current implementation mismatch on retries remain a white-box finding and a required Analytix benchmark case. |

OpenCode supplies a strong structural donor, not factual authority. Analytix
may mark the row exceeded only after deterministic tests prove no free-text
bypass, one exact final call, no fact-increasing repair, same-context receipts,
and zero reasoning bytes across every public and durable surface.

## 2026-07-18 - Third source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `b8142c7aa8f88222873fb79d636e312e28037c2d` |
| Previous same-day pin | `fab213312927ea64cf968832c527206e8c944f9e` |
| Delta | Four commits; 7 files, 107 insertions, 80 deletions. |

This range is Nix, OpenTUI, and desktop-build maintenance: restored desktop
integration, a relaxed Bun-version check, dependency hashes, and an OpenTUI
bump. It supplies no new agent authority, evidence, or publication mechanism.
Deep admission and benchmark parity remain open; no OpenCode capability row is
reached or exceeded.

## 2026-07-18 - Second source refresh

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `fab213312927ea64cf968832c527206e8c944f9e` |
| Previous same-day pin | `69a80663a2ed7d671d2b4d5dd6f2d605714675a5` |
| Delta | Ten commits; 44 files, 218 insertions, 201 deletions. |

The only agent/provider mechanism in this range corrects reasoning-option
semantics and adds focused provider tests; the remainder is predominantly
generated provider/model data, TUI dependency changes, trust-center copy, and
UI fixes. The provider change enters typed-provider comparison, but it does
not supply evidence receipts, same-context binding, or a final publication
authority. Source identity is current; deep admission and benchmark parity are
still open.

## 2026-07-18 - Earlier same-day currentness delta

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `69a80663a2ed7d671d2b4d5dd6f2d605714675a5` |
| Previous reviewed pin | `ef3b67308411614b7a08c2ac81931d930e22c835` |
| Delta | 27 commits; 127 files, 5,899 insertions, 1,938 deletions. |

The delta includes prompt-editor preservation, destroyed recovery-window
guards, Azure Cognitive Services endpoint restoration, provider/pricing data,
and substantial session/desktop UI work. Code/test/document admission remains
open; generated and pricing-only changes do not create capability evidence.
No Analytix implementation or parity result is claimed for this delta.

## 2026-07-13 - Prior current intake

| Field | Value |
| --- | --- |
| Branch / commit | `dev` / `05c3e40a4e641732b991499000ca479e5dad4b02` |
| License evidence | Root MIT license at pinned blob `6439474beed8e0271df9862eff97ffd70ec2464c`. |
| Admitted posture | File-level provenance and MIT notice required before direct or substantial reuse. |

Bounded delta decisions:

| Capability | Decision |
| --- | --- |
| Durable input inbox, run coordinator, execution ownership, and Context Epoch | `should absorb` through Go app/store contracts after a persisted-data/concurrency spec; do not import Effect/Drizzle APIs. |
| Timeline projection/virtualization and production E2E continuity fixtures | `should absorb` with React adaptation and MIT notice; benchmark the current Analytix O(n) window calculation first. |
| MCP OAuth, roots, resources/templates, prompts, and LSP/formatter services | `should absorb` through Go adapters or optional Hub/MCP plugins with OS secret storage and workspace trust. |
| Snapshot/worktree performance techniques | `selective`; preserve Analytix symlink, size, review, and approval invariants. |
| Experimental Code Mode | `defer`; requires a separate threat model and hard timeout/call/output budgets. |
| Session V2 maturity claims | `reject as current proof`; upstream runner TODOs still identify ownership, retry, and persistence gaps. |
| Provider pro-reasoning options, xAI response storage defaults, and release/provider updates after `b3a012cb` | `adapt` only in typed provider configuration and private-reasoning parsing; these changes provide no EvidenceReceipt, same-case binding, or final publication authority. |

## Absorption Bias

| Area | Default stance |
| --- | --- |
| Provider/model registry | Compare against Reasonix and analytix before adopting. |
| Sub-agent/task continuation | Treat as a design reference for analytix child-thread and task-job contracts. |
| TODO/task semantics | Absorb only if it improves analytix Goal, Plan, and evidence ledger behavior. |
| Permissions | Keep stricter analytix approval/sandbox/user-input rules. |
| UI | May inform product interaction only through a separate analytix UI spec, UX rationale, tests, and desktop QA. |

## Batch Template

```text
## YYYY-MM-DD - OpenCode <commit-or-range>

Source:
Reviewed by:
Related branch:

Compared against:
- DeepSeek-Reasonix:
- Kun:
- analytix current:

Summary:

Classification:
| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |

Validation:

Remaining risks:
```
