# Go runtime speed/cache absorption contract

Status: Reference contract and gate input, distilled from the dated Kun/Reasonix
comparison. Current implementation is established by code and fresh evidence;
this file does not assert parity, a new source review, or release readiness.

Current owners are the [documentation map](../README.md),
[benchmark requirements](../specs/09-agent-quality-product-benchmark.md),
[Context Epoch contract](../../../openspec/specs/context-epoch/spec.md), and
[admission/provenance rules](README.md). Source comparison does not mandate
Kun product routes or authorize copying upstream code. The chronological
matrices and test receipts remain recoverable from Git at
`7fc1210fc8259d2c7983edf856cbb7d293b7433d`, this same path.

## 11. Reasonix speed/cache parity

These contracts remain required when the affected behavior changes. A short
fixture proves only its exercised seam; production parity, live Provider,
packaged GUI and release claims require their own applicable evidence.

### 反馈速率硬合同

1. Provider text, tool-call start/completion, usage, error and retry chunks must
   enter typed Go events incrementally. Do not wait for turn/tool completion to
   publish an eligible public delta or first tool indication.
2. Publish/persist coordination must preserve replay without holding the first
   eligible token or continuous stream behind persistence. SSE sequence, thread,
   turn and tool identities must remain stable; live and replay must not block
   one another or lose ordering.
3. Once tool name and call id are known, expose an eligible partial dispatch
   without executing incomplete arguments. Long tools, MCP and subagents must
   expose progress or running state; retry/backoff, interrupted streams and
   empty-final recovery must provide a notice instead of unexplained waiting.
4. Background execution must return a durable job identity and preserve
   start/output/terminal state through output, wait and cancellation controls.
   Read-only work may run concurrently; writer ordering and authority remain
   protected.
5. Main transports must preserve eligible deltas without excessive batching.
   Renderer batching stays at frame granularity; buffering, virtual lists and
   measured-row caches must not swallow live updates.
6. Reasoning required for provider continuation remains bounded and ephemeral.
   Its generic/public/durable exclusion and the separately authorized protected
   process-local native display follow the current
   [publication contract](../../../openspec/specs/case-claim-publication-gate/spec.md),
   replacing the old audit's generic reasoning-display/storage instructions.
7. Deterministic guards must exercise event order, first delta before turn end,
   partial tools, segmented progress, retries, empty-final recovery, stable
   replay identities and background-job state. Measure first feedback, partial
   tool latency and progress gaps without exposing raw reasoning or prompts.

### DeepSeek cache 硬合同

1. Provider-visible system/mode instructions, tool schemas, memory prefix, skill
   index and MCP catalog must have stable, auditable prefix/schema identities.
   Timestamps, selected text, workspace excerpts, clipboard/OCR, live MCP results
   and other dynamic context belong in the admitted turn tail.
2. Ordinary OpenAI-compatible/DeepSeek assistant history must not unconditionally
   re-upload reasoning. Required DeepSeek tool-call continuation and Anthropic
   signed thinking stay provider-specific and obey the current ephemeral
   continuation boundary; hooks must not rewrite required signatures/bytes.
3. Tool names, descriptions, schemas, read-only flags and ordering must be
   canonical. Naming migrations preserve historical tool-call pairing through
   an explicit compatibility contract; healthy history uses a fast repair path
   without unnecessary prefix churn.
4. MCP lazy/background placeholders, schema fingerprints and cached catalogs
   must remain stable. Installation or dynamic registration must not make the
   ordinary-chat tool catalog change unpredictably on every turn.
5. Parse provider-native DeepSeek hit/miss and compatible nested cached-token
   telemetry, preserving supported, known-zero and unknown states. Missing
   telemetry or pricing must not become invented zero hit, miss or cost.
   Composer/thread/model/daily projections must carry the available usage.
6. Bounded diagnostics explain system/tools/history/MCP/schema/memory/style
   drift without raw prompts, tool arguments/results, files or provider bodies.
   Compaction must not repeatedly rewrite the prefix when it cannot improve
   the window; retain the accepted pause/degrade behavior and safe recovery.
7. Guards must cover ordinary/tool-call reasoning request shapes, provider
   isolation, signed continuation, cache parsing, prefix-shape attribution,
   equivalent-turn stable hashes, tool/schema reorder invariance and MCP cache
   identity. DeepSeek optimization must not break OpenAI-compatible,
   Anthropic/messages or custom endpoint formats.

### Existing validation owners

The source-owned [speed/cache gate](../../../scripts/runtime-go-speed-cache-gate.mjs)
and [product regression gate](../../../scripts/runtime-go-product-regression.mjs)
retain their required coverage: settings/providers, stream/parser/SSE/renderer,
partial tools/progress/retry/recovery, MCP lifecycle/schema cache, subagents/jobs,
approval/user input, attachments/vision, usage/cost and compaction/history.
Focused gate identities must resolve to real checks; a large suite or a copied
historical PASS does not substitute for a required focused guard. TypeScript
contracts and diagnostic fixtures do not restore a second production runtime.

## 12. 引入 Reasonix CONTRIBUTING cache-first gate

The [cache-first review gate](../../../scripts/cache-first-review-gate.mjs) owns
executable classification and metadata validation. Sensitive surfaces include
provider-visible instructions and prefix items, tool schemas/naming, MCP
registration/cache, provider serialization/stream/usage, history/compaction,
attachments/references, event projections and settings/provider migration.

Every affected change records concrete values and reasons for all six fields:

- `Cache-impact:` none / low / medium / high, with rationale.
- `Cache-guard:` the focused guard or exact existing coverage.
- `System-prompt-review:` reviewer/approval note for affected prompt/prefix items.
- `Provider-compat-impact:` affected DeepSeek, compatible, Anthropic and custom formats.
- `Speed-impact:` first token, transport, buffering, progress or jobs impact.
- `Speed-guard:` deterministic streaming/partial-tool/progress/retry/usage coverage.

Medium/high cache impact without a concrete guard cannot be complete. Prompt,
memory, style or skill-index changes require system-prompt review. Provider
serialization changes require DeepSeek and at least one other Provider fixture;
tool/MCP schema changes require canonical hash fixtures; streaming/event mapping
changes require speed fixtures. Usage changes preserve available values and
unknown telemetry; history repair preserves the healthy fast path; install/MCP
changes preserve ordinary-chat catalog stability. These obligations are retained,
not waived by the removal of chronological audit rows.
