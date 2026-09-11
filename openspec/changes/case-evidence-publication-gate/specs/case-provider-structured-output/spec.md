## ADDED Requirements

### Requirement: Provider-Native Proposal Mapping
DeepSeek, OpenAI chat completions, OpenAI responses, Anthropic messages, and explicit custom endpoint adapters SHALL map provider-native structured output into the same versioned host proposal contracts without changing local evidence authority.

Case proposals SHALL use typed entity references, ClaimRecord proposals,
evidence references, factual currentness, investigation state, and answer
slots. Complete identifiers and generic strings that purport to contain
source-exact PII SHALL NOT be accepted proposal fields.

#### Scenario: Provider supports strict structured output
- **WHEN** a configured provider family supports the required strict schema mode
- **THEN** the request uses the correct provider-native URL, headers, body, tool/response schema, streaming parser, usage fields, and maps the result to the common proposal type

#### Scenario: Custom full endpoint is configured
- **WHEN** endpoint format is an explicit custom full endpoint
- **THEN** URL construction uses that full endpoint without appending a guessed path and body construction remains separate

### Requirement: Local Final Evidence Gate Is Always Authoritative
Provider schema acceptance, finish reason, tool-call syntax, confidence, citations, support status, or safety flags SHALL NOT authorize a case fact. The local host gate SHALL verify every claim and receipt for every provider.

#### Scenario: Strict output says safe to answer
- **WHEN** provider output is schema-valid and asserts `safeToAnswer=true` but has no matching current registry receipt
- **THEN** the factual claim is rejected and the result becomes boundary-only

#### Scenario: Provider free text accompanies structured output
- **WHEN** a provider includes prose outside the accepted proposal envelope
- **THEN** that prose is ignored for high-risk publication and cannot bypass the host renderer

### Requirement: At Most One Structure-Only Repair
When a provider cannot produce the required structure, the runtime MAY perform at most one repair whose input is bounded to the existing proposal shape and verified support set. Repair SHALL NOT add facts, evidence ids, support state, scope, or legal characterization.

#### Scenario: Repair removes an unsupported claim
- **WHEN** the first proposal is structurally invalid and contains unsupported material
- **THEN** repair may delete or narrow that material and return a valid boundary/partial structure

#### Scenario: Repair adds or changes a fact
- **WHEN** the repaired proposal introduces a new normalized fact or upgrades support/coverage compared with the pre-repair set
- **THEN** the host rejects the repair and emits boundary-only output

### Requirement: Unsupported Strict Mode Has No Free-Text Bypass
If a provider does not support the requested strict schema, the runtime SHALL use the bounded mapping/repair path or return boundary-only. It SHALL NOT accept free Markdown as a case final.

#### Scenario: Provider rejects strict response format
- **WHEN** the endpoint reports that strict/schema output is unsupported
- **THEN** the runtime uses its declared compatible proposal path with one bounded repair or stops without case facts

### Requirement: Provider Tool Calls Require Current Grants
Every provider family's streamed or non-streamed tool call SHALL be reconstructed deterministically and checked against the same current `ExecutionGrant` before persistence or execution.

#### Scenario: Streamed tool arguments are partial or malformed
- **WHEN** a provider stream aborts or finishes with incomplete call id/name/arguments
- **THEN** the host SHALL NOT close strings/brackets or otherwise repair the provider arguments, no grant or ready event is issued, no tool executes, partial arguments are not reused by recovery, and the finalizer handles the terminal

#### Scenario: Provider tool arguments contain duplicate keys
- **WHEN** a provider emits a syntactically complete JSON object whose raw argument bytes contain duplicate keys
- **THEN** the stream adapter preserves those exact bytes for the strict schema boundary, which rejects the ambiguity before grant issuance or execution

#### Scenario: Valid provider call is not advertised
- **WHEN** provider-native syntax is valid but the tool was absent from the current granted schema set
- **THEN** execution is rejected identically across all endpoint families

### Requirement: Provider Reasoning Is Not A Product Event
Reasoning/thinking tokens and signatures SHALL be treated as provider-protocol internals, SHALL NOT be sent to product SSE/history or logs, and SHALL NOT affect claim support.

#### Scenario: DeepSeek emits reasoning before a final
- **WHEN** a DeepSeek-compatible stream contains reasoning tokens
- **THEN** they are excluded from product events and durable history while usage may retain only numeric reasoning-token counts

#### Scenario: Anthropic returns signed thinking
- **WHEN** a messages endpoint requires a signed thinking block for the immediate tool continuation
- **THEN** the runtime keeps it ephemerally for that request chain and discards it before durability/export

### Requirement: Private Tool Continuation Is Attempt-Local
Provider adapters MAY receive only a verified host-compiled semantic tool
projection for the immediate continuation of the exact physical request
attempt that owns the current context-effect lease. That projection MAY
contain short typed ModelEntityAlias values, verified semantic features,
exact analytical values, coverage, and evidence references, but SHALL contain
no AuthorityEntityRef, complete PII, raw row, database path, reverse mapping, or unrestricted tool
body. Durable provider history, reconnect/retry reconstruction, pause/resume,
restart, and compaction SHALL use only the closed public tool-result projection
and host-owned typed references and SHALL NOT recreate raw result text/media or
PII from either.

#### Scenario: Continuation remains in the same valid attempt
- **WHEN** the tool settles successfully and the same context/grant/effect lease is still current
- **THEN** the adapter may send the verified safe semantic projection once to
  that continuation without persisting complete PII or raw result material in
  ordinary history

#### Scenario: Attempt reconnects, restarts, or loses authority
- **WHEN** transport reconnect, cancellation, timeout, pause/resume, restart, context change, grant expiry, or effect-lease loss occurs before the private continuation is sent
- **THEN** the private result is not replayed; the host continues only from closed metadata or terminates boundary-only through the final gate

### Requirement: Sensitive Identifier Prompt Ingress Is Host-Tokenized
Before serializing any provider request, retry, repair, or cache-measurement
body, the host SHALL resolve complete protected identifiers found in user input
under the current active case and snapshot authority, privately bind the
source-exact request to audit use, resolve its existing private authority
reference, and replace each provider-visible span with the current frozen
case's short typed ModelEntityAlias plus safe semantic context.

#### Scenario: Complete identifier is replaced before provider serialization
- **WHEN** a user submits a complete account, card, identity number, phone, or
  other classified value that resolves in the current case/snapshot
- **THEN** every provider family receives only the short model alias and
  authorized semantics, and the source-exact value is absent from request
  bytes, retry/repair inputs, cache observations, errors, and traces; the long
  AuthorityEntityRef and reverse mapping are absent as well

#### Scenario: Original prompt is stored only in protected audit state
- **WHEN** the host needs to retain lawful evidence of the user's exact request
- **THEN** it binds the original value, case, snapshot, purpose, requester, and
  audit digest only in existing host-private state, not ordinary
  history/search/SSE/compaction

#### Scenario: Identifier cannot be resolved
- **WHEN** authority is absent or resolution is missing, ambiguous, stale, or
  cross-case
- **THEN** the exact value is still withheld from the provider, the case slot
  becomes boundary-only, and independent ordinary work may continue

### Requirement: Local Display Projection Cannot Change Provider State
`displayMode=full|masked` and every `typed-local-data-surface/v1` variant
(`ImportMappingPreview`, `CleaningDiffPreview`, `DirectSourcePreview`, and
`AcceptedSlotDisplay`) SHALL remain outside every provider request,
response schema, retry/repair body, cache-visible body, continuation, and
durable provider history. The provider SHALL receive the same short typed
model aliases, verified necessary semantics, exact minor-unit amounts,
direction, date range, coverage/currentness, and ClaimRecord/EvidenceReceipt
references regardless of local display mode. Source-exact local display bytes
SHALL NOT be fed back into a later provider turn.

#### Scenario: Accepted Agent result switches from full to masked
- **WHEN** the user changes only `displayMode` for one accepted typed slot
- **THEN** provider request/result bytes, cache shape, ClaimRecord,
  EvidenceReceipt, evidence digest, and Final Gate result remain identical and
  only the final local projection changes

#### Scenario: Direct Source Preview is requested
- **WHEN** the current local typed UI requests source fields from the active
  immutable snapshot
- **THEN** every provider-family call count remains zero and no preview value is
  added to provider history or a later continuation

#### Scenario: Import or cleaning local preview is requested
- **WHEN** the current typed UI requests bound ImportMappingPreview or
  CleaningDiffPreview cells
- **THEN** every provider-family call count remains zero and no exact preview,
  path, raw DTO, cleaning log, or before/after value enters provider history

#### Scenario: Source-exact slot value has been displayed locally
- **WHEN** a later Agent turn assembles provider context
- **THEN** it uses the persisted typed binding and verified semantic state, not
  the prior local display bytes or reconstructed prose

### Requirement: Provider Receives Safe Longitudinal Case Semantics
For a multi-turn case continuation the host SHALL compile a bounded
task-relevant context containing short typed model aliases, verified semantic
features and relationships, exact analytical values, evidence/claim
references, current/historical snapshot state, counterevidence/refuted
hypotheses, data gaps, open questions, and derivation/confidence metadata.
Complete PII, AuthorityEntityRefs, reverse mappings, raw rows, reasoning, and
unrestricted prior tool results SHALL remain absent. Generic Memory SHALL NOT
be an automatic capture, provider injection, or fallback owner for case state.

If the eligible typed state does not fit the bounded provider context, the host
SHALL select task-relevant state in this fixed order: current verified facts,
historical comparison facts, key relationships, counterevidence and refuted
findings, data gaps, evidence/claim references, then
current-versus-historical snapshot differences. Selection SHALL retain exact
currentness, coverage, and evidence bindings and SHALL NOT promote omitted or
lower-confidence state.

#### Scenario: Restart reassembles relevant verified and refuted state
- **WHEN** an authorized case thread resumes after restart or compaction
- **THEN** the provider receives the host-retrieved relevant semantic state,
  not an unverifiable prose memory or replayed raw result

#### Scenario: Independent thread binds the same case
- **WHEN** a newly created independent thread explicitly binds an authorized
  same case and current snapshot
- **THEN** the host retrieves only task-relevant typed case state through the
  existing case owners, resolves the same account/card aliases, and sends no
  old prose, generic Memory content, exact PII, reverse map, or last-opened case

#### Scenario: Subagent receives a delegated semantic subset
- **WHEN** a parent delegates one bounded case question
- **THEN** the child provider receives only the permitted references,
  semantic features, scope, allowed tools, and evidence references and returns
  typed bound proposals without complete PII

#### Scenario: Provider context budget requires deterministic selection
- **WHEN** all eligible longitudinal state cannot fit the provider context
- **THEN** the host applies the fixed retrieval priority, exposes omitted-state
  coverage, and preserves every selected item's snapshot/currentness and
  evidence/claim binding

### Requirement: Provider Failure Produces Typed Boundaries
Provider 4xx/5xx, malformed streams, unsupported formats, timeout, cancellation, retry exhaustion, and semantic parse errors SHALL produce typed terminal blockers and SHALL NOT invoke a factual fallback model answer.

#### Scenario: Model-request 404 occurs
- **WHEN** a provider endpoint returns 404
- **THEN** user guidance points to Provider Base URL and Endpoint format using sanitized provider/model/final-URL metadata and no credentials or unsafe body dump

#### Scenario: Provider fails after factual deltas
- **WHEN** a stream errors after unaccepted case-fact text
- **THEN** all draft text is discarded and only a host-rendered typed boundary can be accepted

### Requirement: Cross-Provider Conformance Evidence
Release validation SHALL cover the relevant request, streaming, grant, proposal, repair, final-gate, reasoning suppression, and error paths for every supported provider family.

#### Scenario: Provider matrix is incomplete
- **WHEN** a provider family or endpoint format has not been executed in the supported test/live environment
- **THEN** its row remains unverified and no all-provider release claim is made

#### Scenario: Provider contract blocks live certification
- **WHEN** a required live Provider reasoning or structured-output contract is
  missing, invalid, or cannot be exercised with fresh credential authority
- **THEN** A0 live certification and any dependent formal B1 row remain
  `BLOCKED` without fallback, retry expansion, or weakened gates, while
  Provider-independent B1 implementation and deterministic verification
  continue

### Requirement: Production-Wire Provider Cache Measurement
Provider cache correctness SHALL be computed from the exact serialized
production request body and every logical provider call, including tool-loop
steps, outer retries, transport reconnects, pauses, resumes, restarts,
failures, and all terminal paths. Persisted cache observations SHALL contain
only keyed digests, counts, fixed enums, timing, and numeric usage; raw
prompts/bodies, credentials, endpoints with secrets, case text, complete PII,
and reasoning content SHALL NOT be persisted. Cache telemetry SHALL be reported
as the current Analytix product measurement and SHALL NOT depend on or imply a
cross-product comparison.

#### Scenario: Multi-step DeepSeek turn reports cache usage
- **WHEN** three provider calls report cache hit/miss usage before the accepted terminal
- **THEN** the host sums all observed calls, recomputes the aggregate rate from token totals, distinguishes unknown usage from zero, and never reports only the last call

#### Scenario: Request wire shape or digest key changes
- **WHEN** endpoint family, model configuration, reasoning mode, system/tool bytes, conversation prefix, or installation digest key changes
- **THEN** the cache-shape comparison records the exact safe change class and does not compare incompatible digest epochs as a hit regression

#### Scenario: Cache measurement is unavailable
- **WHEN** a provider call does not expose trustworthy cache usage fields
- **THEN** the observation remains unknown rather than becoming zero, a cache
  miss, or a comparative product claim
