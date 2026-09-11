## 1. Admission And Evidence

- [x] 1.1 Create and review the Context Epoch Hard Gate Evidence artifact before
  touching runtime code.
- [x] 1.2 Capture baseline same-prompt prefix hash, prefix items hash, tool
  schema hash, request body fields, and normal-turn prompt token count for the
  default no-source path.
- [x] 1.3 Decide whether `npm run runtime:go:speed-cache-gate` already covers
  Context Epoch; if not, define the smallest fixture extension required for
  source registry no-op and activated dynamic context.
- [x] 1.4 Confirm Context Epoch does not add model tools, MCP schemas, plugin
  schemas, pre-output model calls, or UI-triggered prompt mutations.
- [x] 1.5 Prepare bounded task briefs and review-package expectations if
  subagents are used; keep Superpowers reference-only and not installed.

## 2. Domain And Persistence

- [x] 2.1 Add Context Epoch domain types for source registry entries, prompt
  boundary classification, epoch snapshots, activation state, and sanitized
  prefix-change reasons.
- [x] 2.2 Add thread-scoped persistence for default epoch metadata without
  changing existing threads' provider history or request shape.
- [x] 2.3 Add source registry service behavior for register/update/remove,
  digest tracking, trust state, token budget, and prompt-invisible defaults.
- [x] 2.4 Add tests proving registry-only changes do not alter stable prefix or
  provider history for the same prompt/configuration.

## 3. Request Construction Boundary

- [x] 3.1 Integrate accepted epoch snapshots into provider request construction
  at a safe pre-request boundary.
- [x] 3.2 Preserve the current default provider history output when no context
  source is activated.
- [x] 3.3 Add bounded dynamic-context and turn-tail selection behavior for
  activated sources without moving registry metadata into stable prefix.
- [x] 3.4 Reject or defer mid-stream epoch mutations so active streaming turns
  remain replay-safe.

## 4. Diagnostics And Reason Codes

- [x] 4.1 Extend prefix diagnostics with sanitized Context Epoch reason codes
  such as `source-digest-changed`, `activation-changed`,
  `compaction-recovery`, and `restart-reconcile`.
- [x] 4.2 Record whether each accepted epoch change affected stable prefix,
  dynamic context, turn-tail content, or diagnostics only.
- [x] 4.3 Preserve unknown-vs-zero cache telemetry semantics in all Context
  Epoch diagnostics.
- [x] 4.4 Add tests proving raw source content, local diagnostics, and full
  source paths do not enter model-visible context.

## 5. Compact And Restart Recovery

- [x] 5.1 Define compact recovery metadata for accepted epoch snapshots without
  repeatedly re-injecting compacted raw content.
- [x] 5.2 Restore last accepted epoch metadata during runtime/thread recovery
  before request construction.
- [x] 5.3 Add unavailable-source recovery behavior that marks the source
  unavailable and keeps provider-visible context bounded.
- [x] 5.4 Add tests for restart reconcile, compaction recovery, unavailable
  source behavior, and no repeated compaction loops.

## 6. Validation

- [x] 6.1 Run focused Go tests for new Context Epoch domain, service,
  provider-history, prefix-diagnostics, compact, and recovery behavior.
- [x] 6.2 Run or explicitly block `npm run runtime:go:speed-cache-gate` with a
  Hard Gate Evidence note.
- [x] 6.3 Run `openspec validate context-epoch-foundation` after implementation
  task completion and before archive.
- [x] 6.4 Run `git diff --check` for the touched OpenSpec, docs, and runtime
  paths.

## 7. Documentation And Archive

- [x] 7.1 Update the absorption matrices with the final Context Epoch evidence,
  implementation paths, rejected upstream shapes, and gate results.
- [x] 7.2 Update future-row dependency notes so memory, instructions,
  compaction recovery, tool-output budget, hooks/plugins/MCP, and UI
  diagnostics cite Context Epoch or remain blocked.
- [x] 7.3 Produce a review package with exact path findings before marking the
  OpenSpec change ready to archive.
- [x] 7.4 Archive the change only after implementation, tests, Hard Gate
  Evidence, and review package are accepted.
