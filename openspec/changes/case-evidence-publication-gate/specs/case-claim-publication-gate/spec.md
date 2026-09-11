## ADDED Requirements

### Requirement: Normalized Claim Records
Every proposed case fact SHALL be normalized into a versioned `ClaimRecord`
with claim type, typed case-scoped entity references and answer slots, support
state, evidence and counterevidence ids, supported scope, exact snapshot,
factual currentness (`current`, `historical`, `stale`, or `superseded`),
investigation state (`verified`, `candidate`, `refuted`, or `unresolved`),
allowed wording, prohibited upgrades, human-review requirement, and verifier
receipt. Free-form prose, a display mask, or an internal reference string SHALL
NOT be published as an implicit case claim.

#### Scenario: Provider proposes concrete facts
- **WHEN** provider output includes an amount, count, account, entity, direction, date, relationship, quote, device identifier, or legal characterization
- **THEN** each concrete proposition is independently normalized and verified before any user-visible factual wording is accepted

#### Scenario: Proposal cannot be normalized safely
- **WHEN** a proposition is ambiguous, structurally invalid, or cannot be tied to a supported claim type
- **THEN** it remains unresolved and is omitted or rendered only as missing-evidence guidance

### Requirement: Exact Semantic And Field Support
The claim verifier SHALL require evidence whose case-scoped entity identity,
exact snapshot, normalized subject, amount/count, account, direction,
dates/range, relationship, quote, device identifier, currency, granularity,
and scope support the specific claim. Equal display masks or entity labels
SHALL NOT establish identity. Evidence valid for another proposition,
snapshot, or case SHALL NOT support it.

#### Scenario: Citation supports another amount or subject
- **WHEN** a receipt exists but its normalized fields differ from the claim's amount, account, entity, direction, date, or scope
- **THEN** the claim is rejected as mismatched and the receipt remains valid only for its actual supported fields

#### Scenario: Evidence supports the exact claim
- **WHEN** registry receipts exactly support the normalized payload and scope and no disqualifying counterevidence exists
- **THEN** the verifier may mark the claim `verified` and issue a verifier receipt

### Requirement: Funds Flow Summary Claims Are Recomputable
The existing `ClaimAmount` and `ClaimCount` payloads SHALL represent one
snapshot-bound funds-flow summary for a stable case-scoped entity reference
without introducing a parallel claim or
receipt authority. Verified inflow and outflow ClaimAmounts SHALL use
canonical integer-string minor units, explicit currency/scale and direction;
the ClaimCount and bounded evidence-row support SHALL bind the same internal
entity reference, inclusive time range, timezone, completeness state,
query/result hashes, receipt, and immutable snapshot.

The receipt's pagination completeness, source-row lineage, supported scope,
and snapshot currentness SHALL be carried into each amount/count claim's
`SupportedScope` and final-gate decision. Any coverage/currentness wording that
is actually rendered SHALL come from that same receipt-bound scope. A
counterparty SHALL be omitted unless separately normalized evidence facts
support its exact reference and semantics; a row's mere presence or an
unresolved counterparty field SHALL NOT authorize a counterparty claim.

The deterministic host renderer SHALL compute signed
`netMinor = inflowMinor - outflowMinor` only after both amount claims pass the
Final Evidence Gate under that identical scope. Net is a derived display value,
not a separately model-authored fact or new claim version, and remains
auditable through the exact gated inflow/outflow claims and their shared
receipt. The verifier SHALL
reject floating-point/coerced money, mixed currencies or scales, overlapping
or omitted scope, duplicate support, truncated evidence promoted to complete,
or values assembled from different calls.

#### Scenario: Complete flow summary is supported
- **WHEN** one current receipt proves the exact account/entity and time scope,
  complete query coverage, aggregate amounts/count, and supporting source-row
  references
- **THEN** the host may verify the corresponding inflow, outflow, and
  transaction-count claims, deterministically derive their signed net, and
  issue typed entity/claim/source-field display slots; the generic accepted
  answer remains PII-free and the local typed sink renders `full|masked`
  without exposing the internal entity reference

#### Scenario: Net amount or evidence rows do not reconcile
- **WHEN** the tool-provided net differs from exact inflow minus outflow, money
  crossed a floating-point/coercion boundary, the count differs from the
  covered rows, a row belongs to another subject/snapshot/range, or evidence
  output was truncated without a partial marker
- **THEN** the affected summary remains unresolved or partial and the Final
  Evidence Gate cannot render it as a complete funds result

#### Scenario: Counterparty support is incomplete
- **WHEN** aggregate facts are supported but counterparty resolution is absent,
  ambiguous, truncated, or not represented by exact canonical evidence facts
- **THEN** the aggregate may retain its exact supported scope while the
  counterparty is omitted or rendered only as an explicit gap; no relationship
  or counterparty identity is inferred

### Requirement: Source Capability Enforcement
The verifier SHALL limit each source type to authorized claim families: transaction sources for transactions/accounts/amounts/time/direction; corporate sources for ownership/control/address/change; bid sources for quotes/certificates/edit metadata; device logs for MAC/IP/device identifiers; and authoritative relationship materials for personnel relationships.

#### Scenario: Transaction source is cited for ownership
- **WHEN** a claim about shareholding or corporate control cites only transaction records
- **THEN** the claim remains unsupported regardless of receipt validity

#### Scenario: Multiple source types satisfy a high-risk combination
- **WHEN** a legal-characterization rule requires a specified combination of independent source capabilities and human review
- **THEN** the claim can advance only after every required source class and review receipt is present

### Requirement: Closed Final Answer Union
The final gate SHALL output only `EvidenceBackedAnswer`,
`PartialEvidenceAnswer`, `VerifiedNoHitAnswer`, `SourceUnavailableAnswer`,
`NeedsEvidenceAnswer`, or `GeneralGuidanceAnswer` for each typed case-result
slot. A mixed accepted terminal MAY also contain independently settled
ordinary-work results. Unknown/free-text variants and a case-slot failure that
erases an already accepted ordinary result SHALL be rejected.

#### Scenario: All requested facts are verified
- **WHEN** every rendered factual claim has exact current receipts and requested coverage is complete
- **THEN** the host may render an `EvidenceBackedAnswer`

#### Scenario: No current evidence exists
- **WHEN** no valid current receipt supports the requested case fact
- **THEN** the host renders only source capability, checked scope, missing data, blocker, and evidence-acquisition guidance through a boundary-only variant

#### Scenario: General work succeeds while case result is blocked
- **WHEN** one mixed request completes an authorized code/file/test operation
  but the case slot lacks current authority or evidence
- **THEN** the ordinary result remains accepted and durable while the case slot
  contains only its typed boundary; the host does not fabricate case facts or
  fail the whole request

### Requirement: Empty And Partial Semantics
An empty outcome SHALL NOT mean zero, nonexistence, no relationship, or no anomaly. A partial outcome SHALL NOT support a whole-case conclusion. Verified no-hit wording requires a successful receipt with explicit query scope and completeness.

#### Scenario: Tool result is an empty array
- **WHEN** a current granted query returns `[]` without complete range/pagination/source coverage
- **THEN** the host renders an unresolved or partial boundary and prohibits zero/no-hit upgrades

#### Scenario: Complete query returns no matching records
- **WHEN** a receipt proves exact entity/filter/time/source scope and complete pagination with no matches
- **THEN** the host may render `VerifiedNoHitAnswer` using wording limited to “not found in the checked scope”

#### Scenario: Only one entity or month is covered
- **WHEN** evidence covers one company, one account, one source, or one month of a broader request
- **THEN** only a `PartialEvidenceAnswer` with the exact covered and missing scope is permitted

### Requirement: Typed Ordinary History Never Reuses A Case Transcript
Inside a case-sensitive thread, an ordinary provider lane SHALL reconstruct
prior ordinary assistant results only from a strict `ResultSlotV1` whose
`candidateOrigin` is `provider_ordinary_only` and which is bound either to the
exact committed general terminal CAS/outbox or to a committed case
accepted-final whose complete raw turn still matches the primary CAS. The host
SHALL NOT recover ordinary history by parsing case rendered text, raw
user/provider/tool/background transcript, a current mixed prompt, an internal
entity reference, or a host-fixed boundary.

Missing, stale, malformed, or inconsistent history authority SHALL yield an
empty prior ordinary history with no raw-text fallback. It SHALL NOT disable
the current authorized ordinary task or the Agent's ordinary tool base.

#### Scenario: Prior ordinary work survives an interleaved case turn
- **WHEN** a thread contains committed general and mixed-case terminals with
  independently bound provider-originated ordinary result slots
- **THEN** the next ordinary provider request receives those typed results in
  durable turn order without receiving the case transcript or rendered case
  answer

#### Scenario: Protected authority disappears before an ordinary transition
- **WHEN** a protected provider/tool attempt loses current authority and the
  same Agent continues on an exact ordinary lane
- **THEN** the runtime returns to the base system prompt plus trusted typed
  ordinary history and never sanitizes-and-reuses the unpartitioned protected
  transcript as ordinary input

#### Scenario: Typed history authority is unavailable or tampered
- **WHEN** a primary-CAS observation, raw turn digest, terminal item binding,
  result digest, candidate origin, or active-turn inventory is missing or
  inconsistent
- **THEN** the affected history is withheld with no case-prose or host-fixed
  fallback while new ordinary work remains available

### Requirement: Every Terminal Uses One Final Evidence Gate
Every turn-terminal candidate capable of producing an accepted assistant event
or case-factual slot SHALL call the same Final Evidence Gate before that event
exists. This includes normal success, source unavailable, semantic failure,
provider failure, cancel, timeout, stream abort, recovery exhaustion, approval
or user-input resolution, step limit, and any future factual fallback.

Resume, restart, and recovery admission SHALL revalidate current authority
before creating a new turn candidate; the continuity transition itself is not
an assistant terminal, and the new candidate SHALL use the same gate.
Background completion SHALL append only closed typed lifecycle delivery to an
already-terminal parent and SHALL NOT re-finalize or mutate that parent's
accepted terminal. An explicit continuation created from that delivery is a
new turn whose terminal candidate SHALL use the gate. A report fallback with
no production emitter SHALL create no publication; any future fallback that
emits an accepted assistant candidate SHALL use the gate.

The gate SHALL govern typed case-factual slots and any Agent-authored case
claims included in an external effect. A separate explicit user action SHALL
authorize the exact external effect; an Agent SHALL NOT authorize it. The gate
SHALL NOT turn a failed case slot into a whole-Agent failure or discard an
independently verified ordinary result from the same terminal candidate.
Direct Source Preview is not an Agent terminal or factual claim and SHALL NOT
call Final Gate; previewed fields SHALL NOT gain claim authority from display.

#### Scenario: A terminal is added or changed
- **WHEN** code introduces a new terminal reason or return path capable of
  producing an accepted assistant event or case-factual slot
- **THEN** a terminal-coverage test fails until that reason is mapped through the finalizer

#### Scenario: Background completion reaches a terminal parent
- **WHEN** a child completes after its parent already has an accepted terminal
- **THEN** the host appends exactly one closed typed completion delivery and
  does not re-finalize or mutate the parent's accepted terminal authority; an
  explicit continuation creates a new gated turn

#### Scenario: Turn fails after partial provider text
- **WHEN** a provider stream aborts, times out, is cancelled, or errors after emitting proposed facts
- **THEN** unaccepted text is discarded and the finalizer emits only a verified or boundary-only envelope

### Requirement: Source Unavailable Is Host Terminal Output
When the required current-case source is unavailable, the runtime SHALL NOT call a model to produce or repair a case conclusion. It SHALL render a fixed host-owned capability boundary.

#### Scenario: Funds source is absent at turn start
- **WHEN** a case-fact request has no verified current-run funds source/grant
- **THEN** the runtime makes no provider request that could invent that case
  fact and emits `SourceUnavailableAnswer` for the case slot, while authorized
  ordinary code/file/test work in the same request may continue

#### Scenario: Cached catalog remains after disconnect
- **WHEN** a prior tool catalog exists but the native current-run probe fails
- **THEN** the catalog cannot change unavailable status or enable model case-fact generation

### Requirement: Evidence State Is Monotonic Under Recovery
Recovery, structure repair, retry, resume, and fallback SHALL NOT create evidence, change unresolved/partial claims to verified, broaden supported scope, or add new factual payloads.

Snapshot evolution MAY only lower an old current fact to
historical/stale/superseded for current-answer purposes. It SHALL NOT upgrade
an old receipt into evidence for a newer snapshot merely because the
case-scoped entity reference is unchanged.

#### Scenario: Final repair is needed
- **WHEN** a provider result has invalid structure but an unchanged verified claim set exists
- **THEN** at most one repair may restructure, remove, or narrow content without adding facts or raising support state

#### Scenario: Recovery has no successful receipt
- **WHEN** the prior attempt failed or produced no valid receipt
- **THEN** recovery remains boundary-only and cannot request the model to complete the case analysis

### Requirement: High-Risk Draft Isolation
Provider-originated high-risk factual text SHALL be held as untrusted proposal material until final acceptance and SHALL NOT enter SSE, UI, disk events, thread history, compaction, reports, or exports before the gate.

#### Scenario: Model streams a fabricated amount before calling a tool
- **WHEN** a high-risk turn receives a text delta containing an amount or other case fact
- **THEN** the delta is not emitted or persisted and cannot be observed by the renderer

#### Scenario: Final envelope is accepted
- **WHEN** the gate accepts a typed envelope
- **THEN** one host-rendered PII-free accepted final event containing typed
  display bindings is atomically persisted and then projected to generic
  SSE/UI/history; source-exact values are resolved only by the separate local
  typed sink

### Requirement: Reasoning Is Ephemeral And Non-Exportable
Provider reasoning/thinking content SHALL NOT enter UI, SSE, event logs, thread items, tool items, provider durable history, compaction, reports, logs, traces, or exports on success or failure paths.

#### Scenario: Provider emits reasoning and a tool call
- **WHEN** reasoning is required transiently for a current provider protocol continuation
- **THEN** it remains bounded in memory for that continuation and is discarded before any durable or user-visible boundary

#### Scenario: Turn fails or is recovered
- **WHEN** reasoning was received before failure, cancellation, timeout, recovery, or restart
- **THEN** zero reasoning bytes appear in persisted/replayed/exported artifacts and renderer events

### Requirement: Deterministic Host Rendering
The host SHALL render accepted case answers from verified claim records and allowed wording. A model SHALL NOT be called to turn unavailable, partial, or verified claims into final case prose.

Generic accepted-final rendering SHALL keep complete PII out of prose and SHALL
carry typed entity/claim/source-field slots without exposing internal
references. After Final Gate verifies the exact case, immutable snapshot,
Context Epoch, ClaimRecord, and EvidenceReceipt bindings, Go SHALL resolve only
the referenced original source file/row/field lineage from the accepted
result's retained immutable snapshot and Electron Main SHALL return
`AcceptedSlotDisplay` to
the current main-frame renderer principal. The active-case local UI SHALL
default to `full`; `masked` SHALL be user-selectable and host-produced. Neither
mode SHALL change the provider body, claim, receipt, evidence digest, or Final
Gate. `PIIProjectionGrant`, controlled artifact, publication, legal approval,
or an external trusted sink SHALL NOT gate standard local display. Generic
string, regex, or Markdown replacement SHALL NOT perform resolution.
A canonical private binding, current database value, display label, model
alias, or reparsed approximation SHALL NOT substitute for source-exact display.

#### Scenario: Legal characterization requires review
- **WHEN** a proposed conclusion such as collusive bidding, bribery, benefit transfer, or case-filing eligibility lacks a required evidence combination or human-review receipt
- **THEN** deterministic rendering labels it as an unresolved investigative lead or omits it and lists required evidence

#### Scenario: General guidance is requested
- **WHEN** the answer contains no current-case factual proposition
- **THEN** the host may render `GeneralGuidanceAnswer` while prohibiting entity-specific claims that imply case evidence

#### Scenario: Accepted typed slot is resolved locally
- **WHEN** its exact case/snapshot/epoch/claim/receipt/source-file/row/field binding and
  current renderer session/principal validate after Final Gate
- **THEN** Go reads only that field from the original retained immutable
  snapshot, consumes the private carrier exactly once, and Electron Main
  displays its source-exact or host-masked projection
  without writing the value into generic accepted-final/history/SSE/store state

#### Scenario: Text resembles an entity slot
- **WHEN** provider, legacy, or renderer prose contains an internal-looking ref,
  masked token, or source-like substring outside a valid typed slot
- **THEN** the host performs no source-exact substitution and leaves the text
  PII-free or boundary-only

### Requirement: Direct Source Preview Is Separate From Agent Claim Publication
Direct Source Preview SHALL use trusted host/data-plane code against the active
immutable snapshot and SHALL return only user-requested rows/fields to the
current typed local UI sink. It SHALL make zero provider/model, subagent, MCP,
EvidenceReceipt, ClaimRecord, and Final Gate calls and SHALL NOT persist a case
claim, accepted final, or source-exact conversation prose.

ImportMappingPreview and CleaningDiffPreview SHALL be equally separate from
Agent claim publication: their exact staged-generation and input/rule/output-
generation views SHALL create zero Agent/provider/MCP/receipt/claim/Final Gate
calls and SHALL NOT become evidence merely because the user viewed them.

#### Scenario: User previews source rows before asking the Agent
- **WHEN** the current renderer principal requests bound rows/fields for the
  active case/snapshot
- **THEN** the host returns only that full or masked local projection and no
  preview field becomes evidence or an Agent answer

#### Scenario: Preview and Agent result cover the same field
- **WHEN** a later Agent answer cites a field previously previewed
- **THEN** the Agent slot still requires its own matching EvidenceReceipt,
  ClaimRecord, and Final Gate; the preview cannot satisfy or bypass them

#### Scenario: Import or cleaning preview is viewed before an Agent turn
- **WHEN** the current principal views source-exact mapping cells or cleaning
  before/after cells through their typed local surface
- **THEN** the view creates no claim/evidence/final authority and a later Agent
  statement still requires independent source-row evidence and Final Gate

### Requirement: Longitudinal Investigation State Is Host-Owned
Verified facts, candidate relationships, refuted hypotheses, unresolved
questions, counterevidence, data gaps, prior query scopes, stable entity
references, and current/historical snapshot states SHALL be derived from and
bound to the existing host-owned claim/evidence/thread structures. A model
summary or accepted prose SHALL NOT replace this typed state. Durable display
bindings SHALL retain caseId, immutable snapshotId, ContextEpoch, entity/slot,
ClaimRecord/EvidenceReceipt refs, and necessary currentness without persisting
complete PII prose.

#### Scenario: Relevant context is reassembled after restart
- **WHEN** a user resumes an authorized case after restart or compaction
- **THEN** the host retrieves the relevant claims, counterevidence, gaps,
  open questions, entity references, and evidence/snapshot states and supplies
  only their PII-free semantic projection to the provider; the local UI may
  separately re-resolve an accepted typed slot against its original retained
  immutable snapshot

#### Scenario: Independent thread reuses case authority without prose memory
- **WHEN** a newly created thread explicitly binds the same authorized case
- **THEN** the host retrieves bounded typed claims/evidence/entity currentness
  through existing case owners, resolves the same account/card model aliases,
  and does not use generic Memory, last-opened case, exact PII, reverse maps, or
  old assistant prose as authority

#### Scenario: Historical findings are compared with a new snapshot
- **WHEN** the user asks what changed after a new snapshot
- **THEN** old findings remain historical/stale/superseded, new findings use
  only current evidence, and the host renders the comparison without
  converting the old snapshot into current support or substituting new source
  values into an old display binding

#### Scenario: Free-text summary contradicts host state
- **WHEN** a model or legacy summary says a refuted, stale, or unsupported
  proposition is verified/current
- **THEN** the typed host state prevails and the unsupported upgrade is
  rejected before publication

#### Scenario: Historical snapshot is unavailable
- **WHEN** a persisted display binding points to an explicitly deleted snapshot
  or fails immutable-snapshot validation
- **THEN** the local slot displays `source unavailable`; the host does not use
  the current snapshot, persisted prose, or model memory as a substitute
