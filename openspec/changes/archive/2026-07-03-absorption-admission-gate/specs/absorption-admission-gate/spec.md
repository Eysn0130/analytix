## ADDED Requirements

### Requirement: Admission Verdicts

The absorption admission gate SHALL classify every future upstream absorption
row as `ready`, `blocked`, or `not-applicable` before implementation tasks are
started.

#### Scenario: Row is ready
- **WHEN** a row has all required Hard Gate Evidence and no row-level blockers
  remain
- **THEN** the gate records the row as `ready`

#### Scenario: Row is blocked
- **WHEN** a row is missing required evidence, has an unmet dependency, has an
  unknown prompt boundary, or matches a stop condition
- **THEN** the gate records the row as `blocked` and names the blocking reason

#### Scenario: Gate is not applicable
- **WHEN** a row does not touch provider history, context, tools, MCP, memory,
  instructions, compaction, hooks, skills/plugins, subagents, runtime bridge, or
  UI polling
- **THEN** the gate records `not-applicable` with the reason and evidence path

### Requirement: Hard Gate Evidence

The absorption admission gate SHALL require Hard Gate Evidence for every P0/P1
row that touches provider history, context, tools, MCP, memory, instructions,
compaction, hooks, skills/plugins, subagents, runtime bridge, or UI polling.

#### Scenario: Risky row has complete evidence
- **WHEN** a risky row provides prompt boundary, token delta, prefix hash, tool
  schema hash, cache telemetry source, compact metrics, UI A/B where
  applicable, and privacy/trust evidence
- **THEN** the row can proceed to reviewer verdict

#### Scenario: Risky row lacks evidence
- **WHEN** any required Hard Gate Evidence field is missing or explicitly
  unknown without a justified `not-applicable` reason
- **THEN** the row remains `blocked`

### Requirement: Prompt Boundary Classification

The absorption admission gate SHALL require each row to declare one prompt
boundary from `stable-prefix`, `dynamic-context`, `turn-tail`, `store-only`,
`ui-only`, or `diagnostics-only`.

#### Scenario: Stable prefix impact
- **WHEN** a row changes system prompt, stable prefix items, tool schemas, MCP
  schemas, or plugin/skill schemas
- **THEN** the gate requires prefix hash and tool schema hash before/after
  evidence

#### Scenario: Prompt-invisible state
- **WHEN** a row introduces doctor data, local logs, plugin marketplace
  metadata, memory candidates, progress ledgers, or subagent reports
- **THEN** the gate requires the state to be `store-only`, `ui-only`, or
  `diagnostics-only` unless explicitly activated by the user

### Requirement: Cache Telemetry Handling

The absorption admission gate SHALL distinguish provider-native cache telemetry
from unknown telemetry and MUST NOT treat unknown telemetry as a cache miss.

#### Scenario: Telemetry is available
- **WHEN** provider usage exposes cache read, write, hit, or miss fields
- **THEN** the gate records the provider-native fields and their source

#### Scenario: Telemetry is unavailable
- **WHEN** provider usage does not expose cache fields
- **THEN** the gate records the telemetry source as `unknown` with an
  explanation and does not report it as `0`

### Requirement: UI Cache Neutrality

The absorption admission gate SHALL require UI same-prompt open/closed A/B
evidence when a UI, doctor, memory, subagent, plugin marketplace, or runtime
bridge change can touch runtime state.

#### Scenario: Runtime-adjacent UI surface
- **WHEN** a proposed row adds or changes a runtime-adjacent UI or diagnostic
  surface
- **THEN** the gate requires evidence that prompt history, stable prefix hash,
  tool schema hash, and request shape remain unchanged for the same prompt

### Requirement: Stop Conditions

The absorption admission gate SHALL block implementation when any stop condition
from the accepted absorption packet applies.

#### Scenario: Unknown prompt boundary
- **WHEN** the row cannot identify whether state enters stable prefix,
  dynamic context, turn-tail, store-only, UI-only, or diagnostics-only
- **THEN** implementation tasks remain blocked

#### Scenario: Superpowers dependency
- **WHEN** a proposal requires installing Superpowers before plugin/skill
  admission evidence exists
- **THEN** implementation tasks remain blocked

#### Scenario: Live Reasonix claim without live evidence
- **WHEN** a proposal claims live Reasonix or DeepSeek parity without
  credentialed live evidence
- **THEN** the claim is blocked or downgraded to fixture/local non-regression
  language
