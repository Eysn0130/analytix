## ADDED Requirements

### Requirement: Source Registry

The runtime SHALL maintain a thread-scoped Context Epoch source registry for
dynamic context sources that may later influence provider request construction.

#### Scenario: Register prompt-invisible source
- **WHEN** a memory, instruction, compaction recovery, tool artifact, or
  diagnostics source is recorded for a thread
- **THEN** the registry stores source id, kind, digest, sequence, trust state,
  prompt boundary, token budget, and activation state without adding the raw
  source content to stable prefix

#### Scenario: Source is not activated
- **WHEN** a source exists in the registry but is not selected for the active
  turn
- **THEN** provider history and stable prefix hash remain unchanged for the
  same prompt and tool schema

### Requirement: Epoch Snapshot Lifecycle

The runtime SHALL build provider requests from an accepted Context Epoch
snapshot rather than from ad hoc dynamic context reads.

#### Scenario: Default epoch
- **WHEN** a thread has no registered or activated context sources
- **THEN** request construction uses a default epoch that preserves existing
  provider history, stable prefix hash, and tool schema hash

#### Scenario: Epoch reconcile
- **WHEN** registered source metadata changes before a provider turn begins
- **THEN** the runtime reconciles the epoch at a safe request-construction
  boundary and records whether the accepted snapshot changed

#### Scenario: Mid-stream mutation
- **WHEN** an assistant response is already streaming
- **THEN** Context Epoch state MUST NOT mutate the active provider request or
  rewrite the active turn's provider-visible context

### Requirement: Prompt Boundary Classification

Every Context Epoch source SHALL declare one prompt boundary from
`stable-prefix`, `dynamic-context`, `turn-tail`, `store-only`, `ui-only`, or
`diagnostics-only`.

#### Scenario: Store-only source
- **WHEN** a source is classified as `store-only`
- **THEN** the runtime may persist and diagnose the source but MUST NOT include
  it in provider history, stable prefix, or turn-tail content

#### Scenario: Dynamic context source
- **WHEN** a source is classified as `dynamic-context`
- **THEN** the runtime may include only the selected bounded content for the
  active turn and MUST preserve registry metadata as prompt-invisible state

#### Scenario: Stable prefix source
- **WHEN** a source is classified as `stable-prefix`
- **THEN** the runtime requires an explicit epoch bump, prefix hash before/after
  evidence, and a sanitized prefix-change reason

### Requirement: Prefix Change Reasons

Context Epoch diagnostics SHALL record sanitized reason codes for accepted
epoch changes that can affect provider-visible context.

#### Scenario: Source digest changes
- **WHEN** a selected source digest changes and the accepted epoch changes
- **THEN** diagnostics include a `source-digest-changed` reason without exposing
  raw source content

#### Scenario: Activation changes
- **WHEN** a source moves between inactive and active for provider-visible
  context
- **THEN** diagnostics include an `activation-changed` reason and identify
  whether the change affected stable prefix, dynamic context, or turn-tail

#### Scenario: No provider-visible change
- **WHEN** source metadata changes but provider-visible context remains
  identical
- **THEN** diagnostics record no prefix change and preserve the previous
  stable prefix hash for the same prompt/configuration

### Requirement: Cache And Schema Evidence

Context Epoch implementation SHALL produce admission evidence for stable prefix,
tool schema, token delta, cache telemetry source, and request-shape behavior.

#### Scenario: Same prompt default epoch
- **WHEN** the same prompt is run before and after default Context Epoch support
- **THEN** stable prefix hash, prefix items hash, tool schema hash, request body
  fields, and normal-turn prompt token count remain unchanged or the change is
  explicitly justified in Hard Gate Evidence

#### Scenario: Activated dynamic source
- **WHEN** a dynamic context source is intentionally activated
- **THEN** activated-turn token delta is measured separately from normal-turn
  token delta

#### Scenario: Unknown cache telemetry
- **WHEN** provider usage lacks cache read, write, hit, or miss fields
- **THEN** cache telemetry is recorded as `unknown` and MUST NOT be reported as
  a zero hit rate or confirmed miss

### Requirement: Compact And Restart Recovery

Context Epoch state SHALL survive compaction and restart without re-injecting
raw source content or broad recovered state into stable prefix.

#### Scenario: Compaction occurs
- **WHEN** a thread is compacted
- **THEN** the accepted epoch records compact recovery state with bounded
  source references or summaries and MUST NOT create a repeated compaction loop

#### Scenario: Runtime restarts
- **WHEN** a thread resumes after runtime restart
- **THEN** the runtime restores the last accepted epoch metadata before request
  construction and records `restart-reconcile` only if the accepted snapshot
  changes

#### Scenario: Recovery source is unavailable
- **WHEN** a registered source cannot be read during recovery
- **THEN** the epoch marks that source unavailable and keeps provider-visible
  context bounded rather than injecting stale or reconstructed raw content

### Requirement: Dependency Gate For Future Context Features

The Context Epoch capability SHALL gate future memory retrieval, instruction
discovery, compaction recovery, tool-output budget, and plugin/skill context
proposals unless those proposals explicitly remain blocked.

#### Scenario: Future row depends on context
- **WHEN** a future OpenSpec proposal touches memory retrieval, instruction
  discovery, compaction recovery, tool-output budget, hooks, plugins, MCP, or
  skill context
- **THEN** the proposal cites Context Epoch evidence or records a blocked
  dependency before implementation tasks begin

#### Scenario: UI or diagnostics source
- **WHEN** a future UI, doctor, memory, plugin, or subagent panel reads Context
  Epoch diagnostics
- **THEN** same-prompt open/closed evidence proves prompt history, stable prefix
  hash, tool schema hash, and request shape remain unchanged
