## Context

Analytix already has strong pieces for provider history, prefix diagnostics,
tool schema hashing, durable thread state, usage/cache diagnostics, and manual
compaction. The missing boundary is an explicit Context Epoch: a durable record
of which dynamic context sources are allowed to influence a provider request,
when that set changed, and why the stable prefix did or did not change.

The absorption matrices identify `context.epoch` as the first Wave 1 runtime
foundation because it blocks or constrains memory retrieval, instruction
discovery, compaction recovery, project-rule loading, and future tool-output
budget work. The speed/cache contract requires every one of those capabilities
to keep dynamic state outside the stable prefix unless an explicit, measured
epoch bump occurs.

Existing code to preserve:

- `packages/runtime-go/internal/app/model/prefix_shape.go` already captures
  system, tool, prefix item, route, provider, and model hashes.
- `packages/runtime-go/internal/app/model/provider_history.go` already builds
  provider history and preserves tool call/result pairing.
- `packages/runtime-go/internal/app/toolcatalog` already owns canonical tool
  schema materialization and hashing.
- `packages/runtime-go/internal/app/turn/compaction.go` already creates
  durable compaction summaries and source digests.
- `packages/runtime-go/internal/app/thread` and durable thread metadata are
  the right persistence boundary for per-thread epoch state.

Superpowers is relevant only as an execution discipline. OpenSpec owns the
requirements, design, task order, admission verdicts, and archive. If later
implementation uses subagents, it must use bounded briefs, report files, review
packages, and a progress ledger derived from this OpenSpec change.

## Goals / Non-Goals

**Goals:**

- Define a durable Context Epoch model that separates source registry state,
  epoch snapshots, and provider-visible request construction.
- Record why an epoch changed and whether that change affected stable prefix,
  dynamic context, turn-tail content, or diagnostics only.
- Make prefix hash and tool schema hash comparisons first-class acceptance
  evidence for this and future context-sensitive rows.
- Provide a foundation for future memory, instruction discovery, compaction
  recovery, and tool-output budget changes without adding every-turn prompt
  weight.
- Keep all Superpowers-inspired execution mechanics outside runtime prompts and
  subordinate to OpenSpec.

**Non-Goals:**

- Do not implement memory retrieval, memory curator, project instruction
  discovery, automatic compaction, tool-output pruning, hooks, plugins, MCP
  lifecycle changes, or UI panels in this change.
- Do not install or invoke Superpowers.
- Do not add a new prompt injector, bridge alias, product identity, provider
  shortcut, or alternate runtime boundary.
- Do not claim live Reasonix/DeepSeek speed superiority from local fixtures.

## Decisions

### Decision 1: Context Epoch is thread-scoped durable metadata

Context Epoch state belongs with durable thread/session state rather than in
renderer state, process globals, or prompt text. A future implementation should
store the current epoch id, source registry entries, baseline sequence, and
last accepted snapshot with the thread.

Rationale: the same thread can resume after restart or compact without
reconstructing context from controller memory. Thread scope also prevents
workspace-global context from leaking between unrelated sessions.

Alternatives considered:

- Workspace-global registry: rejected because it risks cross-thread leakage and
  accidental every-turn prompt weight.
- Renderer-only state: rejected because restart, headless runtime tests, and
  provider history construction need the boundary before UI exists.

### Decision 2: Source registry entries are typed and prompt-invisible by default

A source registry entry should describe source id, kind, path or store
reference, digest, sequence, trust state, prompt boundary, token budget, and
activation reason. The registry itself is store-only diagnostics. Provider
history receives only the selected bounded content for the active turn.

Rationale: future memory, instructions, compact recovery, and tool artifacts
can all register source facts without forcing those facts into stable prefix.

Alternatives considered:

- Inline all discovered source text into the system prompt: rejected because it
  breaks cache stability.
- Only keep ad hoc per-feature flags: rejected because future features would
  re-solve the same prompt-boundary problem inconsistently.

### Decision 3: Epoch snapshots are generated at safe provider-turn boundaries

An epoch snapshot should be reconciled or replaced before request construction,
after explicit settings/source changes, after compaction, or during restart
recovery. The active provider request must be built from one accepted snapshot.

Rationale: request construction gets a stable view, and diagnostics can compare
before/after prefix shape deterministically.

Alternatives considered:

- Recompute dynamic context opportunistically inside every provider request:
  rejected because it hides prefix drift and makes cache regressions hard to
  attribute.
- Mutate epoch state mid-stream: rejected because streaming turns must remain
  replay-safe and diagnosable.

### Decision 4: Prefix-change reasons are explicit, sanitized diagnostics

The epoch layer should report reason codes such as `source-added`,
`source-removed`, `source-digest-changed`, `activation-changed`,
`budget-changed`, `compaction-recovery`, `restart-reconcile`, or
`tool-schema-changed`. Reasons are diagnostics and must not become prompt
content by default.

Rationale: prefix hash changes without reasons are not actionable. Reason
codes let future gates distinguish intentional epoch bumps from accidental UI
or dynamic-state leaks.

Alternatives considered:

- Hash-only diagnostics: rejected because they prove a change happened without
  explaining why.
- Raw source diffs in diagnostics: rejected because they can leak local files,
  secrets, and memory content.

### Decision 5: Context Epoch does not change tool schemas

This capability should reuse existing tool catalog and schema hashing. It may
record `tool_schema_hash_before/after`, but it must not add new model tools or
route-specific tool exposure by itself.

Rationale: the Context Epoch boundary is about context source admission, not
tool catalog expansion. Keeping schemas stable makes the first implementation
safer and easier to validate.

Alternatives considered:

- Add a context diagnostics tool in the same change: rejected as scope creep
  and because direct-answer tool schema weight must remain unchanged.

### Decision 6: Superpowers maps to execution controls, not product behavior

For this change, Superpowers contributes only the process pattern:

```text
OpenSpec task -> bounded task brief -> report file -> review package
-> progress ledger -> main-thread synthesis -> OpenSpec update/archive
```

It must not create requirements under `docs/superpowers/*`, install hooks, add
prompt bootstrap text, or override an OpenSpec admission verdict.

Rationale: Superpowers v6.1.1 reduced Codex hook risk by declaring `hooks: {}`,
but its skills are still a broad workflow layer. Analytix already has OpenSpec
as the authoritative change system, so the useful part is the bounded handoff
discipline.

## Risks / Trade-offs

- Context Epoch becomes a second history composer -> Keep provider history
  construction in `internal/app/model/provider_history.go`; epoch only selects
  and annotates dynamic sources before request construction.
- Registry state leaks into stable prefix -> Add tests proving registry-only
  changes do not alter prefix hash unless selected content is activated.
- Prefix reason codes expose sensitive source names -> Store sanitized source
  ids/digests, not raw local paths or file contents, in model-visible data.
- Future features bypass the epoch boundary -> Mark memory, instruction
  discovery, compact recovery, and tool-output budget proposals blocked unless
  they cite `context-epoch` acceptance evidence.
- Implementation becomes too broad -> Keep this change to the foundation:
  domain types, service boundary, request-construction integration,
  diagnostics, and tests. Leave UI and feature-specific context sources to
  later OpenSpec changes.
- Superpowers process adds context weight -> Use file handoffs and report paths
  only; never paste subagent transcripts or broad accumulated summaries into
  parent prompts.

## Migration Plan

1. Add Context Epoch domain/service types behind existing runtime boundaries.
2. Attach empty/default epoch metadata to threads so existing threads remain
   compatible.
3. Wire request construction to consume an epoch snapshot without changing
   provider history for the no-source/default case.
4. Add diagnostics and tests for prefix/tool hash stability, epoch bump reason
   codes, restart recovery, and compaction recovery boundaries.
5. Run focused Go tests and the runtime speed/cache gate or mark the gate
   blocked with a Hard Gate Evidence artifact.

Rollback is safe if the default epoch is prompt-invisible and no provider
request shape changes when no sources are activated. The feature can be ignored
by leaving threads on the default epoch.

## Open Questions

- Should epoch source metadata live directly in thread records, a sidecar store,
  or an app-level context store keyed by thread id?
- Which exact event names should represent epoch initialization, reconcile, and
  replacement in the durable event log?
- Should prefix-change reasons be emitted only in usage diagnostics, or also in
  thread summary/runtime info diagnostics?
- Can the existing `runtime:go:speed-cache-gate` cover Context Epoch directly,
  or does it need a new fixture case for source registry no-op and activated
  dynamic context?
