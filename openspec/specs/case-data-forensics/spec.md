# Case Data Forensics Specification

## Purpose
Define immutable case data, stable case-scoped entity identity, exact valuable
DuckDB funds analysis, lineage, completeness, safe model semantics, and
controlled full-PII separation.

## Requirements

### Requirement: Immutable Raw Data Snapshots
Every case dataset used for evidence SHALL be represented by an immutable snapshot with acquisition metadata, complete SHA-256 hashes, source manifest, parser version, timezone, currency semantics, row counts, and accepted/rejected/duplicate records.

#### Scenario: A file or database source is ingested
- **WHEN** case data is collected or transformed
- **THEN** the system records source identity, acquisition time/method/operator, full raw SHA-256, parser version, transformation version, timezone/currency assumptions, counts, and immutable snapshot id before analysis

#### Scenario: Source data changes
- **WHEN** any raw byte, parser, transformation, source manifest, or accepted/rejected row set changes
- **THEN** a new dataset snapshot is created and the shared Context Epoch is
  reconciled; prior facts remain bound to the prior snapshot and are projected
  only as historical/stale/superseded rather than silently becoming current

### Requirement: Producer Identity And Raw Graph Are Exact
Every dataset snapshot SHALL bind a closed producer-content manifest to the
exact raw-artifact or database snapshot that it seals. The manifest SHALL
truthfully identify the component, operation, engine, parser/transformation
version, schema, capabilities, content hashes, source/accepted/rejected/
duplicate counts, and queryable immutable DuckDB identity where applicable. A
producer SHALL NOT claim another implementation's engine or capability merely
because an output field has the same shape.

The existing strict-CSV `count_case_rows` capability MAY remain an internal
MCP/authority plumbing canary. It proves only its exact count contract and
SHALL NOT be treated as a funds-analysis result, evidence-row capability,
Milestone B acceptance, or reason to relabel the producer as DuckDB analytics.

#### Scenario: Exact DuckDB snapshot is admitted for funds analysis
- **WHEN** the host selects one immutable DuckDB snapshot whose complete bytes,
  schema, producer profile, case binding, and source lineage all match the
  current DatasetSnapshotAuthorityV2 selection
- **THEN** the manifest advertises only the registered query capabilities for
  that exact snapshot and binds the database content hash without exposing a
  raw path to the model, renderer, or ordinary MCP result

#### Scenario: Count canary succeeds
- **WHEN** `count_case_rows` completes through the current grant and authority
  plumbing
- **THEN** it may verify that narrow canary only; Milestone B remains
  incomplete until the valuable flow analysis and packaged public seam pass

#### Scenario: A valid source has no accepted rows
- **WHEN** the registered producer completely reads a valid empty source
- **THEN** it emits a canonical empty graph whose accepted, rejected,
  duplicate, source, and detail counts are exactly zero without inventing a
  sentinel row

#### Scenario: Producer identity, database bytes, or row disposition changes
- **WHEN** the producer, engine, parser, transformation, schema, capability,
  database/raw byte, or accepted/rejected/duplicate set differs
- **THEN** admission creates a distinct immutable snapshot identity and
  reconciles the shared Context Epoch rather than reusing cached evidence

### Requirement: Row-Level Transformation Lineage
Evidence-bearing derived records SHALL trace to immutable raw records through deterministic transformation lineage and source record identifiers.

#### Scenario: A normalized transaction supports a claim
- **WHEN** the verifier reads a normalized amount/account/date/direction field
- **THEN** the receipt identifies the raw record ids, raw hash,
  parser/transformation lineage, normalized field mapping, and every
  host-derived institution/card-type/region/entity/relationship feature with
  its deterministic derivation and confidence/support state

#### Scenario: A record is rejected or deduplicated
- **WHEN** parsing, validation, or deduplication removes or merges a record
- **THEN** a bounded reject/duplicate ledger preserves the reason and source lineage without silently changing coverage

### Requirement: Valuable Snapshot-Bound Funds Flow Analysis
The production funds MCP SHALL expose one fixed business tool,
`analyze_account_flows`, for the first packaged funds milestone. The host
SHALL bind the call to the current case, exact immutable DuckDB snapshot, and
one stable case-scoped account or entity reference resolved under the current
snapshot grant. Entity identity MAY remain stable across snapshots in the same
case; every query result, source row, receipt, and fact remains bound to the
exact current snapshot and epoch. The tool SHALL execute only a predefined
parameterized read-only query under the parser/binder,
table/function allowlist, timeout, cancellation, row, and byte limits; it
SHALL NOT accept arbitrary SQL, a database path, a case path, or caller-supplied
authority fields.

The result SHALL use a strict structured schema containing the covered
account/entity projection, inclusive time range, timezone, currency and scale,
canonical integer-string `inflowMinor`, `outflowMinor`, signed `netMinor`,
`transactionCount`, coverage/completeness state, deterministic query/result
hashes, and a bounded set of supporting evidence rows.
`netMinor = inflowMinor - outflowMinor`. The executor SHALL derive minor units
from exact DECIMAL or validated text and SHALL NOT pass evidence-bearing money
through `DOUBLE`, JavaScript `number`, Rust `f64`, scientific notation,
rounding, or an inferred scale. Excess precision, non-finite, missing,
unparseable, or mixed-currency values remain unknown/blocked. Each evidence
row SHALL contain only an opaque snapshot-bound source-record reference,
timestamp, direction, exact integer-string amount/currency/scale, stable
case-scoped account/entity/counterparty references, and user-display-safe
labels. It SHALL NOT contain a raw database path, SQL text, complete
account/card identifier, reversible identity token, collision-prone mask used
as analysis identity, or unrelated row fields.

#### Scenario: Account flow question is answered completely
- **WHEN** the host resolves one authorized stable account/entity reference
  and a bounded time range, and the exact DuckDB snapshot covers that scope
- **THEN** the tool returns exact minor-unit inflow, outflow, signed net,
  transaction count, and supporting evidence rows whose aggregates recompute
  exactly, whose semantic entity/relationship features are complete for the
  declared schema, and whose query is replayable against the same immutable
  snapshot

#### Scenario: Evidence rows hit a bound
- **WHEN** the aggregate query completes but evidence-row pagination, row
  count, byte count, timeout, cancellation, rejected-row coverage, or source
  coverage is incomplete
- **THEN** the result is explicitly partial, records the exact covered scope,
  and cannot support a complete or whole-case claim

#### Scenario: Caller supplies SQL, a path, or complete account text
- **WHEN** a provider, renderer, or ordinary MCP caller attempts to provide
  arbitrary SQL, a local DuckDB path, snapshot authority, or a complete bank
  account/card value
- **THEN** validation fails before database access and no diagnostic reflects
  the rejected sensitive value or path

#### Scenario: Existing stats path exposes floating-point money
- **WHEN** a candidate native query or semantic stats result represents an
  evidence-bearing amount as `DOUBLE`, `number`, `f64`, rounded decimal, or
  scientific notation
- **THEN** that result is ineligible for an EvidenceReceipt until the fixed
  query derives and returns exact declared-scale minor-unit strings

### Requirement: Stable Case-Scoped Entity Identity
The host SHALL represent governed accounts, cards, identity numbers, phones,
persons, organizations, devices, and other protected identities with a
collision-resistant, non-reversible internal entity reference scoped to the
authenticated case and entity type. The reference SHALL derive from
host-private key material and canonical governed identity, SHALL NOT expose a
raw-value hash, display mask, sequential model label, database id, or global
stable token, and SHALL fail closed on collision or ambiguous resolution.

Entity identity SHALL be independent from snapshot/evidence/claim identity.
The exact raw value and reverse binding SHALL remain only in the existing
host-private persistence topology. The same entity MAY retain its reference
across turns and snapshots in one case, while facts and evidence remain bound
to their exact source snapshot.

#### Scenario: Same entity remains stable across lifecycle operations
- **WHEN** the same authorized entity is used across funds calls, Todo,
  subagent, compaction, restart, resume, or an allowed fork in one case
- **THEN** the host supplies the same entity reference and never asks the
  model to infer identity from a mask or per-turn alias

#### Scenario: Snapshot change preserves identity but changes fact version
- **WHEN** the same entity is present in a later accepted snapshot
- **THEN** its case-scoped reference may remain stable, but old amounts,
  relations, coverage, receipts, and claims become
  historical/stale/superseded until reverified from the new snapshot

#### Scenario: Two cases contain the same complete account
- **WHEN** case A and case B independently contain the same source-exact value
- **THEN** their provider-visible references are unlinkable by default and no
  global token, cache, model history, or reverse mapping correlates them

#### Scenario: Joint analysis is explicitly authorized
- **WHEN** the current user explicitly selects the exact case set and entities
  and the host binds each case authority and audit scope
- **THEN** the host may create a case-local evidence-backed relationship
  between the independently scoped references without merging their
  namespaces, delivering a reverse mapping to model/UI, or creating a global
  identity graph

#### Scenario: Joint analysis continues or loses one case authority
- **WHEN** an authorized joint analysis crosses turns, restart, resume, or fork,
  or authority for one participating case is revoked
- **THEN** every case retains its independent reference namespace, current
  snapshot/currentness, receipts, and claims; the host revalidates the joint
  relation before use and removes it when authorization no longer covers the
  exact case set without leaking or deleting the remaining case state

### Requirement: Safe Semantic Projection Preserves Investigative Meaning
Removing source-exact PII SHALL NOT remove useful, authorized analytical
semantics. Before provider dispatch the host SHALL assemble typed entity
reference, entity/account type, institution, currency, ownership/subject,
time, exact amount/direction/balance/frequency/count, counterparty and funds
path, permitted relations, query scope, coverage and missing ranges, evidence
and ClaimRecord references, current/historical snapshot state, verified/
candidate/refuted/unresolved findings, counterevidence, derivation method, and
confidence/support state relevant to the task.

The trusted DuckDB/data tool SHALL perform exact matching, joins, aggregation,
money calculation, and deterministic derivation from identifiers. The model
SHALL interpret intent, select bounded tools, compare evidence, form
hypotheses, and explain results without receiving complete identifiers or
performing database arithmetic in prose.

#### Scenario: Host derives useful attributes before provider dispatch
- **WHEN** a complete account or identity field deterministically encodes an
  authorized institution, account/card type, region, or other useful feature
- **THEN** the trusted host/tool supplies the typed derived feature with
  lineage and confidence, while the complete source field remains private

#### Scenario: Model receives a longitudinal case context
- **WHEN** a later turn continues an investigation
- **THEN** the provider receives the relevant stable references,
  relationships, verified and refuted findings, gaps, snapshot comparison,
  evidence references, and open questions without raw PII or reverse mappings

### Requirement: Sensitive User Input Is Tokenized Before Provider Dispatch
If a user prompt contains a complete governed account, card, identity number,
phone, or other classified value, Electron/Go host SHALL resolve it inside the
current active case and snapshot boundary before any provider serialization,
ordinary persistence, search indexing, SSE projection, or compaction. The host
SHALL keep the exact original request and binding/audit relation only in
protected private state and SHALL replace the provider-visible span with the
stable case-scoped entity reference plus safe semantic context.

#### Scenario: User enters a complete identifier
- **WHEN** the current case/snapshot authority can resolve the exact value
- **THEN** the host privately binds the original input to the entity and audit
  use, while provider requests/retries/repairs/cache measurement, ordinary
  SSE/log/history/search/compaction, generic bridge events, and free-form
  renderer/durable state contain zero bytes of the complete value; a separate
  allowlisted typed local display may resolve the value under its own exact
  binding

#### Scenario: Identifier cannot be lawfully resolved
- **WHEN** case/snapshot authority is missing or the complete value is absent,
  ambiguous, stale, or outside the authorized scope
- **THEN** the source-exact span still never reaches the provider, the case
  portion receives a typed boundary, and independent ordinary work may
  continue

### Requirement: Direct Source Preview Uses The Trusted Local Data Plane
The installation-bound current local Analytix user session, current main-frame
renderer principal, active case binding, and current immutable dataset snapshot
SHALL jointly authorize Direct Source Preview for that case. Trusted host/data-
plane code SHALL return only the rows and fields explicitly requested through a
short-lived typed local UI contract. Direct Source Preview SHALL NOT call a
provider/model, subagent, MCP, EvidenceReceipt, ClaimRecord, or Final Gate and
SHALL NOT create evidence or claim authority.

The renderer request SHALL contain only its already-known workspace root,
typed view, ordered allowlisted fields, bounded row window, and display mode.
It SHALL NOT require or accept a thread id, turn id, entity reference, case id,
snapshot id, source locator, or caller-supplied authority. Electron Main and Go
SHALL resolve the active case and current snapshot from the current main-frame
and host-owned case binding. The typed transaction view MAY include requested
source-exact time, account/card/name/identity, amount, direction,
counterparty/bank, summary, currency, merchant, and remark fields and SHALL NOT
be limited to an account-only projection.

`displayMode` SHALL be `full|masked`; the active-case local UI SHALL default to
`full`, and `masked` SHALL be produced by the host. The request SHALL NOT expose
the private mapping table, key material, DuckDB path/file descriptor/locator,
unrelated rows or fields, raw evidence blobs, or cross-case identity links.

#### Scenario: Current source rows are previewed in full
- **WHEN** the current main-frame principal requests exact rows and fields from
  the active case's current immutable snapshot with `displayMode=full`
- **THEN** the typed local sink displays those source-exact values, returns no
  unrequested field, and provider/MCP/EvidenceReceipt/ClaimRecord/Final Gate
  call counts remain zero

#### Scenario: Current source rows are previewed masked
- **WHEN** the same request uses `displayMode=masked`
- **THEN** the host returns only the masked projection and creates no provider
  call, durable source-exact prose, clipboard/write, or external effect

#### Scenario: Preview request exceeds its binding
- **WHEN** a request names another case/snapshot, an unrelated field, a mapping
  table, key, locator/descriptor, raw evidence blob, or cross-case link
- **THEN** the host rejects it before reading or returning those bytes

#### Scenario: Snapshot changes after a preview request
- **WHEN** a new immutable snapshot becomes active before the old response is
  delivered
- **THEN** the old pending response is rejected, and a new current preview must
  bind the new snapshot

### Requirement: Protected Case Sources Are Outside Ordinary Tool Authority
Plaintext DuckDB snapshots, raw evidence, decryption material, complete PII,
and private entity reverse mappings SHALL be accessible only through the
current DSV2 broker and trusted bounded data-plane effect. They SHALL NOT be
ordinary workspace resources or ambient same-user files. Hiding a tool or
filtering path strings SHALL NOT satisfy this requirement.

Ordinary read/list/find/glob/grep/Git tools and shell/subprocess execution SHALL
receive no path, environment variable, inherited descriptor, key, mount, or
filesystem capability that can reach those protected stores. Stable-file
identity checks SHALL reject alias, symlink, hardlink, traversal, archive,
subprocess, and replacement bypasses at the actual effect boundary while
leaving ordinary source-workspace access intact.

#### Scenario: Ordinary file tool attempts protected source access
- **WHEN** read/search/Git follows a direct, relative, aliased, symlinked, or
  hardlinked route to a protected DuckDB, raw evidence, or reverse mapping
- **THEN** the effect fails before reading bytes and no case evidence or PII is
  returned

#### Scenario: Shell attempts protected source access
- **WHEN** bash or a descendant uses a discovered path, environment variable,
  inherited descriptor, database CLI, archive utility, or replacement alias
- **THEN** the process lacks the protected filesystem capability and observes
  zero protected bytes; command-string filtering alone is not credited

#### Scenario: Ordinary workspace files remain usable
- **WHEN** the same Agent reads, edits, builds, or tests authorized ordinary
  source files in a case workspace
- **THEN** normal file/shell grants continue to work without acquiring access
  to the separately protected case stores

### Requirement: Executable Source Capability Matrix
The evidence verifier SHALL enforce a versioned source capability matrix mapping source types and fields to allowed claim types and prohibited upgrades.

#### Scenario: Bid metadata supports a quote
- **WHEN** a bid source receipt contains an exact entity, lot, quote, currency, date, and complete record reference
- **THEN** it may support that quote but cannot by itself support banking, ownership, personnel, or device claims

#### Scenario: Personnel relationship lacks an authoritative source
- **WHEN** only similar names, transaction data, addresses, or model inference suggest a family/personnel relationship
- **THEN** the relationship remains an investigative lead and cannot become a verified fact

### Requirement: Query Scope And Completeness
Every query receipt SHALL state filters, entities/accounts, direction, time range, source set, granularity, pagination/row-limit status, rejected records, and coverage completeness.

#### Scenario: Query hits a row limit or timeout
- **WHEN** execution stops at a row/byte/time limit or pagination is incomplete
- **THEN** the result is partial and all downstream claims are limited to the observed scope

#### Scenario: Query is complete and empty
- **WHEN** all configured sources, pages, records, and requested range were checked with no match
- **THEN** the receipt may support a no-hit statement limited to that exact scope

### Requirement: Missing Values Remain Unknown
Missing, null, unavailable, invalid, or unparsed numeric and categorical values SHALL remain explicitly unknown and MUST NOT be converted to zero, false, absent, or safe.

#### Scenario: Amount is missing in a report input
- **WHEN** a derived row has no valid amount
- **THEN** the claim and renderer preserve `unknown`/unresolved and do not output `0` or `0.00`

#### Scenario: Import API fails
- **WHEN** acquisition or cleaning transport fails before a count is verified
- **THEN** summaries report source failure rather than zero imported/cleaned rows

### Requirement: Case-Bound Factual Caches
Every factual cache SHALL be keyed by authenticated tenant/user, thread, case, binding hash, context epoch, dataset snapshot, source/connection identity, schema/parser version, tool/query/argument/scope hashes, and SHALL have explicit expiry/invalidation. `active`, `current`, last-opened, or equivalent aliases SHALL NOT be factual cache keys.

Case-scoped entity identity continuity SHALL be stored separately from factual
cache entries: changing a snapshot invalidates cached facts without
renumbering a verified same-case entity. No cache or entity token SHALL be
stable across cases by default.

#### Scenario: Same pair is queried in two cases
- **WHEN** cases A and B ask the same pair/amount question without an explicit business `case_id` argument
- **THEN** each resolved frozen context has a distinct key and case B cannot receive case A data

#### Scenario: Dataset changes within one case
- **WHEN** the same case receives a new snapshot or source connection epoch
- **THEN** prior factual cache entries are not reused for current evidence or
  publication, while the case-scoped entity reference may continue and old
  facts remain explicitly historical/stale/superseded

### Requirement: Default PII Projection
Provider/model context and continuation/history, subagent inputs/results, MCP
public results, ordinary tool public projections, generic HTTP/SSE text or
events, free-form durable conversation and renderer state, search indexes,
logs, traces, telemetry, crash reports, compaction prose, generic audit
metadata, and unrestricted external effects SHALL use stable case-scoped
internal entity references plus authorized structured semantics and SHALL NOT
contain source-exact identifiers, reverse mappings, raw rows/evidence, or
DuckDB locators/descriptors. Display masks SHALL NOT serve as analytical
identity, and internal references SHALL NOT appear as user-facing product
language.

Only the allowlisted Direct Source Preview and AcceptedSlotDisplay typed local
sinks, or a Main/Host typed effect path invoked by an explicit user action, MAY
handle source-exact fields outside the trusted private data plane. The
active-case local UI SHALL default to `displayMode=full`; user-selected
`masked` SHALL change only the final host projection and SHALL NOT change
provider payload, ClaimRecord, EvidenceReceipt, evidence digest, or Final Gate.

#### Scenario: Tool returns full identifiers
- **WHEN** a source outcome contains a complete bank account, identity number, or phone number
- **THEN** the provider receives the stable reference and permitted semantic
  features, every forbidden generic channel remains source-exact-value-free,
  and only an exactly bound typed local sink may receive the requested full or
  host-masked field without receiving the reverse mapping

#### Scenario: Audit artifact is stored
- **WHEN** the runtime persists tool audit material
- **THEN** it is scoped by tenant/thread/case/epoch/snapshot, encrypted or protected according to current storage policy, and does not default to a global cross-case directory containing full PII

### Requirement: Complete Values Remain Outside Model And Generic Funds Channels
The production funds tool SHALL accept only a host-resolved opaque,
case-scoped stable subject reference resolved under the current snapshot grant
and SHALL return only stable references, semantic features, and safe
PII-free labels for account/card identities. The trusted plugin/data plane
MAY read the exact governed DuckDB fields
inside its bounded execution in order to compute the authorized result, but it
SHALL NOT return, log, cache, persist, or place a complete identifier in an MCP
response, candidate receipt material, provider continuation, generic HTTP/SSE,
free-form renderer/durable state, history, search, fork/resume, compaction
prose, report bytes, automatic export, log, telemetry, or crash report.
Source-exact local display SHALL occur only through Direct Source Preview or a
Final-Gate-accepted typed slot, never through the funds tool's public result.

#### Scenario: Ordinary account analysis succeeds
- **WHEN** the host resolves an authorized stable case-scoped account
  reference under the current snapshot grant and the
  bounded query matches source rows containing a complete identifier
- **THEN** aggregation and evidence matching use the governed source internally
  while provider semantics, downstream claims, and generic channels contain
  only the stable case-scoped reference and PII-free semantics; a later exact
  local display is separately resolved by an allowlisted typed sink

#### Scenario: Plugin or model requests a complete value
- **WHEN** a provider, plugin tool argument, generic renderer/event/store, or
  unbound caller tries to select or receive a complete account/card value
- **THEN** the host returns a fixed boundary, performs no generic value-bearing
  response, and creates no durable or streamed copy of the exact value

### Requirement: Agent Accepted Slots Resolve Source-Exact Values Locally
Agent-authored case-factual answers SHALL carry typed entity/claim/source-field
slots. After Final Gate verifies the exact case, immutable snapshot, Context
Epoch, ClaimRecord, and EvidenceReceipt bindings, the Go host SHALL resolve only
the referenced source fields and Electron Main SHALL deliver them through a
short-lived `AcceptedSlotDisplay` contract to the current main-frame renderer
principal. The funds execution's host-private evidence carrier SHALL already
have been consumed or discarded exactly once before claim settlement and SHALL
never enter display or generic state. AcceptedSlotDisplay SHALL instead
re-resolve the retained case binding through one callback-scoped exact-value
use against the accepted result's original witnessed immutable DSV2 snapshot.

The installation-bound current local session, renderer principal, active case
binding, and bound immutable snapshot SHALL authorize this local display. No
third-party, administrator, per-field legal approval, `PIIProjectionGrant`,
controlled artifact, publication receipt, or external trusted sink SHALL be a
runtime prerequisite. Generic accepted-final/history/HTTP/SSE/bridge events and
free-form renderer state SHALL remain PII-free. Generic string, regex, or
Markdown substitution SHALL NOT reconstruct a complete identifier.

#### Scenario: Accepted Agent slot displays in full
- **WHEN** the receipt, claim, Final Gate, case, snapshot, epoch, renderer
  session, and requested source field all match with `displayMode=full`
- **THEN** the typed local sink displays the source-exact natural value after
  one callback-scoped retained-binding use, while generic channels contain only
  the typed binding and PII-free prose

#### Scenario: The same Agent slot displays masked
- **WHEN** the identical accepted binding is requested with
  `displayMode=masked`
- **THEN** the host returns a masked local projection without changing the
  provider body, claim, receipt, evidence digest, or Final Gate result

#### Scenario: Accepted slot binding is stale or mismatched
- **WHEN** its case, snapshot, epoch, claim, receipt, field, renderer session,
  principal, retained DSV2 material, or current active-case authority fails
  validation
- **THEN** no value is delivered, the pending payload is cleared, and the
  display reports source unavailable/fails closed without substituting the
  current snapshot or exposing an internal reference

### Requirement: External Source-Exact Effects Require An Explicit User Action
Copy, save/export, print, drag/drop, external application, Connector, email,
upload, share, and cross-case analysis SHALL require an explicit user action
bound to the exact target, case, immutable snapshot, and fields. The user click
SHALL be sufficient runtime authorization for that exact effect; no third-party,
administrator, or per-field approval SHALL be required. A provider-bound
Connector SHALL NOT receive complete PII. An external effect that needs exact
values SHALL execute through a Main/Host typed effect path rather than model,
MCP public result, generic renderer data, or browser-generated artifact bytes.

Existing `PIIProjectionGrant`, controlled-artifact, publication, private-CAS,
or one-use trusted-sink mechanisms MAY remain requirements of a formal export,
external-sharing workflow, or optional high-security viewer. They SHALL NOT
gate Direct Source Preview or AcceptedSlotDisplay and SHALL NOT require a new
authority, receipt, registry, CAS, or protocol version.

#### Scenario: User copies selected fields
- **WHEN** the user clicks copy for an exact allowlisted field selection
- **THEN** Main writes only those fields to the clipboard, performs no automatic
  copy or provider call, and requires no third-party approval

#### Scenario: User invokes an external effect
- **WHEN** the user explicitly selects export, share, print, Connector, email,
  upload, drag/drop, or external-app delivery
- **THEN** the host binds target/case/snapshot/fields and sends exact values only
  through the typed effect path; a provider-bound Connector receives only the
  PII-free semantic projection

#### Scenario: External action or target binding is absent or stale
- **WHEN** the user did not initiate the effect or its target, case, snapshot,
  fields, renderer principal, or optional formal-artifact authority no longer
  matches
- **THEN** no external value-bearing effect occurs, without disabling standard
  local display or independent ordinary work

### Requirement: Parser-Aware Read-Only Analytics
Custom database analysis SHALL use parser/AST or engine binder validation, read-only connections, table/function allowlists, timeouts, row/byte limits, cancellation, and deterministic diagnostics. Regex blacklists alone SHALL NOT authorize a query.

#### Scenario: Query hides a write in comments or nested syntax
- **WHEN** parser/AST/binder analysis finds mutation, extension loading, filesystem/network access, disallowed functions, multi-statements, or policy ambiguity
- **THEN** execution is rejected before the database runs it

#### Scenario: Comment delimiters occur inside string literals
- **WHEN** a query places `/*` and `*/` in strings around a filesystem/table function or uses an allowed-looking view/macro to reach external data
- **THEN** the string/comment-aware tokenizer, concrete relation/function manifest, and engine-level disabled external access reject it before EXPLAIN or execution can observe external bytes

#### Scenario: Valid bounded read query runs
- **WHEN** the query is parseable, read-only, source-scoped, within limits, and bound to the current snapshot
- **THEN** the engine may execute it and records deterministic scope, limit, timing, and result hashes

#### Scenario: Identifier or money exceeds JavaScript precision
- **WHEN** a source returns an account/card identifier, DECIMAL money, or integer minor units that cannot round-trip through a JavaScript number
- **THEN** textual identifiers and canonical decimal/integer strings preserve exact bytes; a numeric-typed account/card identifier fails closed rather than being rounded or reformatted

### Requirement: Investigative And Legal Wording Classes
Case conclusions SHALL distinguish `verified fact`, `analysis inference`, `investigative lead`, and `legal characterization`. High-risk legal characterizations SHALL require configured evidence combinations and human review.

#### Scenario: Evidence suggests coordinated bidding but is incomplete
- **WHEN** some relationships or bid anomalies are verified but the required combination and review are missing
- **THEN** the answer may state a bounded investigative lead and MUST NOT assert collusion, bribery, benefit transfer, or filing eligibility

#### Scenario: Required combination and review are present
- **WHEN** the versioned legal rule, all required independent evidence sources, counterevidence review, and human-review receipt are satisfied
- **THEN** a legal-characterization claim may be rendered with its scope and review status
