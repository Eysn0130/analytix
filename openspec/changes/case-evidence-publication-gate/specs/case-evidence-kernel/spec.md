## ADDED Requirements

### Requirement: Immutable Turn Security Context
The runtime SHALL mint one immutable versioned `TurnSecurityContext` before a high-risk turn begins, using host-authoritative thread, turn, real workspace, authenticated identity, case binding, dataset snapshot, source manifest, and the accepted Context Epoch. The runtime SHALL compute `contextDigest` from canonical fields and SHALL reject external attempts to supply or override those fields.

#### Scenario: Turn starts with a valid case binding
- **WHEN** the host resolves a case workspace and immutable dataset snapshot at turn start
- **THEN** it freezes `threadId`, `turnId`, `workspaceRealPath`, `tenantId`, `userId`, `caseId`, `caseBindingHash`, `datasetSnapshotId`, `sourceManifestHash`, `contextEpoch`, `issuedAt`, and `contextDigest` before source or provider work begins

#### Scenario: Model or tool reports another case
- **WHEN** provider or MCP output reports a `caseId`, epoch, snapshot, source identity, or safety field different from host context
- **THEN** the host ignores the reported authority and rejects any outcome whose verified binding does not match the frozen context

#### Scenario: Mixed turn carries ordinary and protected effects
- **WHEN** one user request includes ordinary code/file/test work and a
  snapshot-bound case question
- **THEN** the host freezes the case augmentation before the protected effect,
  while ordinary effects retain their own grants and do not acquire case
  authority merely by sharing the turn

### Requirement: Case Evidence Is Optional To General Agent Work
Case binding, funds-plugin readiness, dataset snapshot authority, evidence
registry readiness, and publication authority SHALL gate only case-fact
capabilities. Their absence, unavailability, corruption, quarantine, or
version mismatch SHALL NOT prevent the packaged product from starting or from
admitting ordinary work under the general execution policy in the same Agent,
thread, turn, or mixed request.

#### Scenario: General workspace has no case or funds configuration
- **WHEN** a user opens an isolated non-case repository and no case binding,
  funds plugin, dataset snapshot, or case evidence authority is configured
- **THEN** the Go runtime starts, ordinary general tools remain available
  subject to their normal grants, and coding, file, shell, test, Todo,
  subagent, compaction, thread recovery, and shutdown paths remain usable

#### Scenario: Case capability is unavailable after startup
- **WHEN** a funds source or case authority fails while ordinary work or a
  mixed turn is active
- **THEN** only case-fact advertisement and publication become boundary-only;
  the host does not cancel, downgrade, or relabel the general turn as failed
  unless a general-work dependency itself failed

### Requirement: Single Agent With Additive Case Capabilities
Analytix SHALL have one Agent, one durable thread model, and one permanent
general capability base. Case funds analysis SHALL be composed as an additive
capability for the current provider request and concrete protected-data effect
only after its current case, snapshot, epoch, source, and grant validate.
Neither startup nor runtime SHALL introduce a `caseMode`, `generalMode`,
general-only fallback Agent, mutually exclusive tool catalog, whole-thread
case switch, or zero-tool case state.

#### Scenario: Case capability is granted in an existing thread
- **WHEN** a current case binding, DSV2 snapshot, source probe, and grant become
  valid in a thread already used for ordinary work
- **THEN** the same Agent adds the exact case tool for that provider request
  while retaining ordinary file, shell, Git, Plan, Todo, Skills, MCP,
  subagent, compaction, and thread capabilities

#### Scenario: Case authority expires during mixed work
- **WHEN** DSV2, source, epoch, evidence, or external-effect PII authority
  expires while a request also contains authorized ordinary work
- **THEN** only the affected protected call, case claim, or explicit external
  full-value effect fails closed; the ordinary portion can complete and
  remains durably visible

#### Scenario: Authority is reacquired without replacing the Agent
- **WHEN** the host later obtains a current valid case/snapshot/source grant
- **THEN** the same Agent and thread may resume case analysis without a runtime
  mode change, thread replacement, or loss of ordinary state

### Requirement: Existing Authority Topology Is Reused
The first-stage packaged milestones SHALL reuse the existing
`TurnSecurityContextV2`, `DatasetSnapshotAuthorityV2`, host evidence
authority, `EvidenceReceipt`, authoritative registry, Final Evidence Gate,
current installation/session and renderer-principal binding, the existing
Electron typed-bridge pattern, and optional controlled-artifact foundations. A
new authority, receipt, registry, CAS, staging, or protocol version SHALL NOT be
introduced merely to complete local display wiring, express another lifecycle
state, or make a task independently countable.

A new version MAY be proposed only after a fresh reproducible public-seam
failure demonstrates that the existing topology cannot enforce a named
accepted invariant, and only after the accepted spec records the failed
invariant, compatibility and migration impact, rollback, owner/caller/
consumer path, and end-to-end verification.

#### Scenario: Existing components can bind the funds result
- **WHEN** the current TSCV2, DSV2, host evidence, receipt, registry, claim, and
  Final Gate contracts can represent the exact snapshot-bound funds result
- **THEN** production composition reuses them and no parallel V3/V4 registry,
  capsule, receipt, CAS, or staging authority becomes a prerequisite

#### Scenario: Experimental authority code exists in the worktree
- **WHEN** an uncomposed or partially implemented newer authority version is
  present but no public-seam failure proves it necessary
- **THEN** it remains non-authoritative implementation residue or deferred
  work, cannot block either packaged milestone, and is not advertised as
  product delivery

### Requirement: Snapshot Is Frozen Before Live Probe
An executable case-evidence context SHALL contain one concrete immutable dataset snapshot selected from host-owned snapshot authority before the context is minted. `unresolved:*`, `none`, cached catalog metadata, model/tool/MCP self-report, or a live-probe response SHALL NOT select or upgrade the snapshot. The current-run live probe SHALL verify exact equality with the frozen snapshot and may only preserve or downgrade readiness.

#### Scenario: Host has no accepted immutable snapshot
- **WHEN** a case binding is valid but host snapshot authority cannot resolve one exact accepted dataset snapshot
- **THEN** only the protected case slot/effect is boundary-only before case-source execution or case-fact provider work and requests ingestion/selection rather than using an unresolved placeholder; ordinary provider and tool effects continue under their ordinary grants

#### Scenario: Live source reports a different snapshot
- **WHEN** the authenticated current-run probe reports a snapshot different from the frozen host snapshot
- **THEN** the current turn remains immutable, execution/publication is blocked, and any later accepted snapshot change advances the shared Context Epoch before a new turn

#### Scenario: Transport reconnects without dataset change
- **WHEN** the same installed source reconnects with a new connection epoch but the host-selected immutable raw/source manifest and parser lineage are unchanged
- **THEN** the dataset snapshot identity and its `sourceManifestHash` remain unchanged, while the new server identity/connection epoch must still pass the current-run probe and be bound separately into grants and receipts

#### Scenario: Probe attempts to upgrade unresolved context
- **WHEN** a probe returns a concrete snapshot for a context containing `unresolved:*` or `none`
- **THEN** the probe cannot remint or upgrade that context and no grant/evidence authority is issued

### Requirement: Single Context Epoch Dependency
Case security SHALL extend the thread-scoped accepted `context-epoch` capability produced by archived `context-epoch-foundation` and MUST NOT introduce another epoch counter, registry, or independent recovery state.

#### Scenario: Case binding or dataset changes
- **WHEN** the workspace, case binding, case id, source manifest, or dataset snapshot changes
- **THEN** the shared Context Epoch is reconciled and bumped before another turn is admitted

#### Scenario: Case changes during an active turn
- **WHEN** a new binding is accepted while old-epoch source, provider, tool, approval, input, job, or report work is in flight
- **THEN** old work is cancelled where possible and its late result is rejected by context-digest validation

#### Scenario: Snapshot advances while entity identity remains stable
- **WHEN** the same authorized case accepts a later immutable snapshot that
  contains a previously verified entity
- **THEN** the Context Epoch and factual evidence version advance, the
  case-scoped entity reference may remain stable, and every old fact remains
  historical/stale/superseded until the new snapshot independently supports it

#### Scenario: Case A switches to case B
- **WHEN** the active binding changes from case A to case B
- **THEN** the Context Epoch advances and case A entity mappings, evidence,
  private results, typed display sessions/payloads/pending responses, pending
  work, provider continuation, and PII cannot enter case B; late case-A display
  responses fail closed while ordinary Agent capabilities remain available

### Requirement: Existing Risk And Boundary Lineage Is Revalidated
For the first-stage packaged milestones, the runtime SHALL reuse and revalidate
the existing host-derived risk/publication policy, `TurnSecurityContextV2`,
`DatasetSnapshotAuthorityV2`, execution grant, and evidence/final authority at
the concrete protected case effect. An independent monotonic witness or a new
thread-risk/Evidence Registry authority family SHALL NOT be an A or B
prerequisite unless a fresh reproducible packaged public-seam failure first
proves that the existing topology cannot enforce a named accepted invariant
and the accepted design is updated with compatibility, rollback, and
verification consequences.

#### Scenario: Existing authority cannot prove a rollback seam
- **WHEN** a rollback scenario cannot be proven safe by the current accepted
  authority and fresh public-seam evidence
- **THEN** that rollback row remains `UNVERIFIED` or the affected protected
  case effect fails closed; ordinary Agent startup and unrelated ordinary
  effects remain available, and the runtime does not silently compose a new
  witness or authority family

### Requirement: Risk Cannot Change Inside A Frozen Turn
External start and steer contracts SHALL be strict and MAY only request a case-risk raise. A steer, attachment, file reference, case binding change, fork, resume, or restart SHALL NOT reuse a frozen general context for a protected case effect when the host observes case risk or a context-changing source. The host SHALL reject/cancel the stale protected continuation and require a newly admitted V2 case augmentation under the current host-derived risk/publication policy, while preserving independently authorized ordinary work.

#### Scenario: Case request is steered into a general turn
- **WHEN** steer text or host-observed state raises case risk after a general turn was frozen
- **THEN** the protected case effect does not reach the provider or publish
  under the old context and is re-admitted under current V2 authority in the
  same Agent/thread; already authorized ordinary effects/results are not
  cancelled merely by the risk raise

#### Scenario: Case thread is forked or resumed
- **WHEN** a thread with case/boundary lineage is forked, resumed, or restored after restart
- **THEN** the child/current thread inherits at least the same host-derived risk floor and revalidates the current TSCV2/DSV2 and protected-effect grant before case provider/effect execution, without granting complete PII or disabling unrelated ordinary capabilities

### Requirement: Host-Issued Execution Grants
The runtime SHALL issue a versioned `ExecutionGrant` only after a current-run source probe verifies connection, server identity/version, connection epoch, fresh tool catalog, schema, case-bound health, scope, approval state, and expiry. Every provider-originated tool call SHALL match the current grant before execution.

#### Scenario: Advertised granted tool is called
- **WHEN** a provider proposes a tool call whose name, schema hash, server identity, scope hash, turn, context digest, approval state, and expiry match an active grant
- **THEN** the host may execute the call through the normal policy boundary

#### Scenario: General and case grants coexist
- **WHEN** one provider request has ordinary workspace tools and a current
  snapshot-bound funds tool
- **THEN** each call is checked against its own exact grant, revoking the funds
  grant does not revoke unrelated ordinary grants, and an ordinary result
  cannot become case evidence

#### Scenario: Tool is unadvertised or unknown
- **WHEN** any Agent, Plan, subagent, resume, or recovery response proposes an unadvertised tool, an unknown schema, a wrong server, a stale digest, an expired grant, or a scope mismatch
- **THEN** the runtime rejects it before side effects and records a bounded deterministic blocker

### Requirement: Strict External Contract Schemas
Every external security, MCP, evidence, claim, final-answer, and publication schema SHALL be versioned, SHALL declare all required fields, and SHALL reject unknown properties by default.

#### Scenario: External payload has an unknown field
- **WHEN** an untrusted payload includes a field outside its versioned schema
- **THEN** validation fails closed without using that field to broaden authority or support

#### Scenario: Provider cannot produce the current schema
- **WHEN** provider output is missing required structural fields after the permitted bounded repair
- **THEN** the host emits a boundary-only answer and does not infer defaults for evidence or claims

### Requirement: Closed Public Tool-Result Projection
The runtime SHALL keep raw tool results private to the current provider/effect
attempt. At first settlement, the host SHALL validate the complete process-local
`ToolOutcome` against the outer tool name, call id, context digest, context
epoch, execution grant, case, snapshot, server, transport, semantic, error,
safety, receipt, and evidence bindings. After that validation succeeds, the
canonical private durable tool-result root MAY retain, as case-source read-time
binding metadata, only the outer tool name, call id, context digest, positive
bounded context epoch, execution grant id, and one strict host-only case-source
binding proof. That proof SHALL bind a
closed version and purpose, the exact outer tuple, public projection
kind/status/code, and `isError`; it SHALL NOT contain a `ToolOutcome`, case or
source values, PII, paths, reverse mappings, raw tool bodies, or provider text.

The runtime SHALL persist or expose ordinary tool-result items only through an
exact-root, strict `PublicToolResultProjectionV1`. Every public variant SHALL be
metadata-only, SHALL deny fact-answer/evidence authority, and SHALL use
host-enumerated codes. The generic public projection itself SHALL NOT serialize
the host-only binding proof, `CaseOutcome`, `contextDigest`, `contextEpoch`,
`executionGrantId`, `datasetSnapshotId`, or any other settlement authority.
History, SSE, replay, search, fork, compaction, provider history, and renderer
reads MAY preserve `case_source_status` only when they derive it from a
canonical private durable item whose outer tuple and lifecycle still recompute
to the exact stored host-only proof. Public projections SHALL NOT be sufficient
to reconstruct private result bytes, evidence, citations, artifacts, or the
private binding tuple.

#### Scenario: Tool result contains arbitrary facts or media
- **WHEN** a tool/MCP result or legacy item contains content, PII, unknown root fields, arbitrary codes, citations, attachments, generated files, inline media, URLs, paths, diagnostics, or remote authority claims
- **THEN** ordinary persistence and every public projection contain only the closed metadata variant or `legacy_output_withheld`, and none of those fields gains evidence/artifact authority

#### Scenario: Closed result is bound to another call or context
- **WHEN** a syntactically valid case-source projection is read from a durable
  item whose host-only proof is missing, duplicated, open, unknown, malformed,
  or no longer exactly matches its tool name, call id, context digest, bounded
  epoch, execution grant, projection kind/status/code, lifecycle status, or
  error state
- **THEN** the host rejects or downgrades it to `legacy_output_withheld` before
  history, SSE, search, fork, compaction, replay, provider continuation, or
  publication, without exposing the proof or private tuple

#### Scenario: Provider continuation needs private result material
- **WHEN** the immediate tool continuation is still in the same attempt
- **THEN** only the host-compiled semantic projection with stable references,
  verified features, exact analytical values, coverage, and evidence
  references is available while the exact context-effect lease remains valid;
  complete identifiers/raw rows/paths/reverse mappings are withheld, and
  restart/history can see only closed public and typed host-owned references

### Requirement: Transport And Semantic Outcome Separation
The runtime SHALL preserve `transportStatus`, `semanticStatus`, `safeToAnswer`, `isError`, blocker, case/epoch assertions, partial coverage, typed data, `_meta`, and candidate evidence material as separate `ToolOutcome` fields. Transport success SHALL NOT imply semantic or evidence success.

#### Scenario: HTTP succeeds with semantic failure
- **WHEN** an HTTP or JSON-RPC request succeeds but MCP marks `isError`, unsafe answer, blocker, invalid schema, or semantic failure
- **THEN** the outcome cannot issue evidence or support a factual claim

#### Scenario: Outcome is partial
- **WHEN** a successful tool outcome covers only part of the requested entities, accounts, period, pages, or sources
- **THEN** the host preserves explicit partial coverage and prohibits whole-case support

### Requirement: Host-Issued Evidence Receipts
Only the host SHALL issue a versioned `EvidenceReceipt` after validating the current grant, transport and semantic success, canonical argument/result hashes, source identity, connection epoch, case/epoch/snapshot binding, query scope, coverage, lineage, PII class, and registry insertion.

#### Scenario: Verified source result is accepted
- **WHEN** a successful outcome passes all binding, identity, schema, hash, and coverage checks
- **THEN** the host issues a receipt containing `receiptId`, `contextDigest`, `toolCallId`, server identity/version, `connectionEpoch`, `toolName`, `argsHash`, `resultHash`, source type, `datasetSnapshotId`, `queryHash`, range, granularity, currency, timezone, pagination completeness, source record ids, raw SHA-256, transformation lineage, PII classification, issued time, and registry integrity proof

#### Scenario: Tool returns a receipt-like identifier
- **WHEN** untrusted output contains a non-empty receipt/evidence/source id not issued into the current host registry
- **THEN** the identifier is treated as data only and cannot support a claim

### Requirement: Typed Local PII Display Reuses Existing Case Boundaries
Provider analysis SHALL use short typed ModelEntityAlias values plus
host-verified semantic features, while the stable case-scoped non-reversible
AuthorityEntityRef remains private in the existing host
evidence receipt, registry, ClaimRecord, and Final Evidence Gate contracts. A
display mask or model alias SHALL NOT serve as authority, and an
internal reference SHALL NOT be shown to the user.

The installation-bound current local Analytix user session, current main-frame
renderer principal, active case binding, and immutable snapshot SHALL jointly
authorize standard local display. Direct Source Preview SHALL read only
requested active-snapshot rows/fields through trusted host/data-plane code and
SHALL NOT call a provider, MCP, EvidenceReceipt, ClaimRecord, or Final Gate.
Its renderer request SHALL not require thread, turn, entity-reference, case,
snapshot, or source-locator knowledge; Main and Go SHALL resolve those values
from the current main-frame and host case authority.

`ImportMappingPreview` and `CleaningDiffPreview` SHALL join Direct Source
Preview and AcceptedSlotDisplay as the other closed variants of
`typed-local-data-surface/v1`, respectively bound to one staged source
generation and to one input snapshot/rule generation/output snapshot
transformation lineage. They SHALL use the same current session/principal,
typed Main/Go bridge, bounded row/field window, no-store, expiry, clear/revoke,
and late-response rules and SHALL create no Agent turn, provider call,
EvidenceReceipt, ClaimRecord, or Final Gate authority.

Agent-authored factual answers SHALL retain typed entity/claim/source-field
slots; only after Final Gate validates the exact case, snapshot, Context Epoch,
claim, and receipt SHALL Go resolve the referenced fields and Electron Main
deliver `AcceptedSlotDisplay` through the short-lived typed local sink.

The funds tool's host-private evidence carrier SHALL be consumed or discarded
exactly once during evidence settlement and SHALL never enter provider,
message, event, log, telemetry, or display state. AcceptedSlotDisplay SHALL
perform a separate callback-scoped retained source-row/field read against the
accepted result's original witnessed immutable DSV2 snapshot. A canonical
private binding or current database value SHALL NOT substitute for source-exact
display; unavailable or invalid retained material SHALL fail closed without
current-snapshot substitution.

`displayMode=full|masked` SHALL affect only the final local projection, with
`full` the active-case default. No third-party, administrator, per-field legal
approval, `PIIProjectionGrant`, controlled artifact, publication verification,
or external trusted sink SHALL gate any standard typed local variant. The system
SHALL reuse current session/principal/case/snapshot, TSCV2, DSV2, existing
claims/receipts/Final Gate, and the Electron typed-bridge pattern and SHALL NOT
create a new grant, authority, receipt, registry, CAS, staging, or protocol
version for local display.

Provider/model context, provider continuation/history, subagent inputs/results,
MCP public results, ordinary tool public projections, generic HTTP/SSE/events,
free-form durable conversation/renderer state, search, logs, telemetry, crash
reports, generic Memory, and compaction prose SHALL remain
source-exact-PII-free. The typed
local sink SHALL NOT receive the private mapping table, key material, DuckDB
locator/descriptor, unrelated fields, raw evidence blobs, or cross-case
identity links.

#### Scenario: Funds analysis is accepted for Agent display
- **WHEN** a snapshot-bound funds result supports the exact flow-summary claims
  and Final Gate accepts its typed entity/claim/source-field slots
- **THEN** the generic accepted envelope remains PII-free and the current local
  typed sink may display the requested source-exact or host-masked fields
  without exposing an internal reference

#### Scenario: Direct Preview displays current source fields
- **WHEN** the current principal requests exact rows/fields from the active
  immutable snapshot
- **THEN** trusted host/data-plane code returns only that `full|masked` local
  projection with zero provider/MCP/receipt/claim/gate calls

#### Scenario: Import and cleaning previews stay local and lineage-bound
- **WHEN** the current principal requests a bounded ImportMappingPreview or
  CleaningDiffPreview under its exact staged/input/rule/output generation
- **THEN** the typed Main/Go sink returns only requested cells, creates zero
  provider/MCP/receipt/claim/gate calls, and stores no exact value in generic
  renderer state, history, or Memory

#### Scenario: Standard local display prerequisites are current
- **WHEN** the current local session, main-frame principal, active case, bound
  immutable snapshot, exact requested fields, and any Agent slot gate bindings
  validate
- **THEN** no PIIProjectionGrant, controlled artifact, legal approval, or
  external trusted sink is requested before local display

#### Scenario: Standard local display binding is stale
- **WHEN** the case, snapshot, epoch, renderer session/principal, requested
  field, expiry, or Agent carrier-consumption binding is stale or mismatched
- **THEN** no value is delivered, old payloads and pending responses are
  cleared, and a late response fails closed

#### Scenario: Explicit external effect needs source-exact values
- **WHEN** the user clicks copy/export/share/print/Connector/email/upload for an
  exact target/case/snapshot/field set
- **THEN** Main/Host executes the typed effect, a provider-bound Connector
  remains PII-free, and any existing controlled-artifact or PIIProjectionGrant
  workflow applies only to that external effect or optional high-security view

#### Scenario: Existing authority proves insufficient
- **WHEN** a fresh reproducible public-seam failure shows that the accepted
  receipt/registry/controlled-artifact topology cannot enforce a named safety
  invariant
- **THEN** implementation stops at the failing boundary until a separately
  accepted spec records why a new version is necessary and how it migrates,
  rolls back, composes, and is verified end to end

### Requirement: Evidence Registry Integrity
The runtime SHALL keep a durable authoritative receipt inventory that can
assemble a bounded thread/case investigation across turns while every receipt
remains strictly bound to its original turn, context, epoch, and snapshot. The
existing registry SHALL preserve canonical hashes, membership, revocation,
current/historical/stale/superseded projection, append, replay, compaction
reference, and restart recovery without becoming a new identity or history
registry.

#### Scenario: Receipt membership is checked
- **WHEN** a claim cites an evidence id
- **THEN** the host verifies exact registry membership, integrity, context digest, dataset snapshot, and non-revoked status before reading support

#### Scenario: Registry state is damaged or inconsistent
- **WHEN** recovery finds a missing event, hash mismatch, duplicate conflicting receipt, impossible sequence, or unknown registry version
- **THEN** evidence recovery fails closed and current publication remains blocked

#### Scenario: A later turn compares an older snapshot
- **WHEN** the host assembles relevant accepted receipts from a prior snapshot
  in the same case
- **THEN** it may expose their stable entity references and verified semantics
  as historical comparison only, and cannot use them as current-snapshot
  support

### Requirement: Persisted Pending Work Revalidation
Pending approvals, user inputs, background jobs, provider continuations, and report stages SHALL persist the context digest and relevant grant identifiers and SHALL revalidate them before resume or execution.

#### Scenario: Approval resumes in the same context
- **WHEN** an approval decision arrives for an unexpired grant whose context still matches
- **THEN** the runtime may continue after rechecking current source and approval state

#### Scenario: Approval resumes after a case switch or restart
- **WHEN** a persisted pending action has a stale epoch, digest, snapshot, identity, catalog, or grant after resume/restart
- **THEN** the runtime rejects the action and routes the turn through the final evidence gate without executing it

#### Scenario: Case continuation becomes stale while ordinary work remains valid
- **WHEN** a persisted mixed request contains a stale protected case action and
  an ordinary action whose workspace/execution grant remains current
- **THEN** the host rejects only the protected action, preserves the ordinary
  action/result, and records the case boundary without changing Agent mode

#### Scenario: Persisted job reports an artifact path
- **WHEN** a child-run record contains a relative, external, aliased, filename-mismatched, or duplicate job/artifact identity
- **THEN** startup fails before recovery writes; artifact paths are derived only by the host from the frozen child-run root and strict `job-N` id

### Requirement: Durable Multi-Turn Case Investigation Context
The host SHALL assemble a bounded longitudinal case context from the existing
thread, stable entity binding, EvidenceReceipt, ClaimRecord, snapshot, Todo,
accepted-final, compaction, and private-state owners. It SHALL preserve
verified, candidate, refuted, unresolved, historical, stale, and superseded
findings; relevant relationships; prior query scope; counterevidence; data
gaps; open questions; next steps; and typed display bindings containing caseId,
immutable snapshotId, ContextEpoch, entity/slot, ClaimRecord/EvidenceReceipt
refs, and necessary currentness. Complete PII prose SHALL NOT be persisted, and
model free text SHALL NOT be the authority for this state.

The host SHALL support an independently created thread that the user
explicitly binds to the same case. It SHALL add the minimum immutable
case-level longitudinal index inside the existing caseentity owner, binding
tenant/user/case/binding, case-state generation, snapshot/currentness,
AuthorityEntityRefs/StableOrdinals, and claim/evidence/continuation digests.
The new thread SHALL revalidate current TSCV2/DSV2 and retrieve only bounded
provider-safe typed state. It SHALL NOT discover case state through generic
Memory, last-opened/current aliases, ambient workspace scanning, a global
reverse map, or a second registry.

The provider SHALL receive only the task-relevant short model aliases,
verified semantic features, evidence/claim references, and currentness states.
Complete PII, reverse mappings, raw rows, reasoning, and unrestricted prior
tool results SHALL remain excluded.

When the eligible typed state exceeds the bounded context budget, the host
SHALL select task-relevant state in this deterministic priority order: current
verified facts, historical comparison facts, key relationships,
counterevidence and refuted findings, data gaps, evidence/claim references,
then current-versus-historical snapshot differences. Truncation SHALL preserve
each selected item's exact currentness and evidence binding and SHALL NOT
upgrade omitted, stale, historical, candidate, refuted, or unresolved state.

#### Scenario: Compaction and restart resume an investigation
- **WHEN** a case thread compacts, exits, and restarts under the same authorized
  case
- **THEN** the host revalidates current authority and reassembles entity
  continuity, verified/current and historical findings, counterevidence,
  gaps, open questions, and evidence bindings without replaying raw PII or
  relying on an unverifiable prose summary; an accepted display binding is
  re-resolved against its original retained immutable snapshot

#### Scenario: Allowed fork preserves case-scoped identity
- **WHEN** an authorized fork inherits the same case scope
- **THEN** it inherits stable entity references and bounded verified semantics
  while every receipt remains bound to its original turn/snapshot and the
  child cannot broaden the case or PII scope

#### Scenario: Independent new thread rehydrates the same case
- **WHEN** the user creates a separate thread and explicitly selects the same
  authorized case under a current snapshot
- **THEN** the host resolves the same account/card authority refs and aliases,
  retrieves relevant typed facts/currentness/evidence through existing owners,
  and supplies no exact PII, reverse map, old prose, generic Memory, or
  last-opened-case fallback

#### Scenario: Independent new thread has no current case authority
- **WHEN** the selected case/snapshot/binding is absent, stale, revoked,
  ambiguous, or belongs to another tenant/user
- **THEN** case rehydration and provider/data effects fail closed while ordinary
  Agent work remains available and no ambient case is selected

#### Scenario: Subagent receives only delegated semantic context
- **WHEN** a parent delegates a bounded case hypothesis
- **THEN** the child receives only the delegated references, semantic
  features, allowed tools, scope, and evidence references; no complete PII or
  reverse mapping is exposed, and the returned typed result must bind the
  parent, case, epoch, snapshot, claims, and receipts

#### Scenario: New snapshot preserves identity but supersedes facts
- **WHEN** a later snapshot is admitted for the same case
- **THEN** stable entity identity may continue, old factual states become
  historical/stale/superseded, current Direct Source Preview uses the new
  snapshot, a historical Agent result continues resolving its bound old
  snapshot, and only new current evidence supports new conclusions

#### Scenario: Case switch excludes prior-case context
- **WHEN** the thread changes from case A to case B
- **THEN** case A references, facts, private results, PII, and child
  continuations are absent from the case B context; the old typed display
  session/payload/pending response is revoked and late delivery fails closed,
  while ordinary Agent state remains available

#### Scenario: Historical display source is deleted or invalid
- **WHEN** an openable historical result's immutable snapshot or binding is
  explicitly deleted or fails validation
- **THEN** the display reports `source unavailable` and does not substitute the
  current snapshot, persisted complete PII prose, or model memory

#### Scenario: Context budget cannot retain every eligible case item
- **WHEN** a continuation would exceed the bounded provider context budget
- **THEN** the host applies the fixed retrieval priority, retains exact
  currentness and evidence bindings for selected items, records omitted-state
  coverage, and never lets truncation promote a historical, refuted, stale, or
  unsupported finding

#### Scenario: Authorized joint-case work survives lifecycle boundaries
- **WHEN** the user explicitly selected the exact case set and the joint
  investigation resumes, restarts, or forks, or one participating case
  authority is later revoked
- **THEN** each case keeps its independent entity namespace, current snapshot,
  currentness, receipts, and claims; the joint relation is revalidated and is
  removed when its scope is no longer authorized without delivering a cross-
  case reverse mapping or leaking/destroying the still-authorized case state

### Requirement: Durable Inventory Read Isolation
Durable thread inventory and detail reads SHALL NOT discover, copy, merge, delete, or activate legacy roots. Legacy migration SHALL run only from a sealed, journaled startup plan.

#### Scenario: Legacy thread appears after store construction
- **WHEN** a legacy runtime thread directory appears after the product store has completed construction
- **THEN** all-id, list, limited-list, and detail reads exclude it, preserve the legacy source bytes, and create no product thread directory
