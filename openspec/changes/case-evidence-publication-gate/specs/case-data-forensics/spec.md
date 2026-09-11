## ADDED Requirements

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
The host SHALL keep three closed identity representations. `SourceExactIdentityV1`
SHALL bind the original source value to exact source/snapshot/row/field lineage
and remain inside trusted local storage/computation and typed local display.
`AuthorityEntityRefV1` SHALL be the existing collision-resistant,
non-reversible, installation-keyed internal reference scoped to authenticated
case and entity type and SHALL remain the durable host/evidence authority.
`ModelEntityAliasV1` SHALL be a short typed selector derived from the existing
private `StableOrdinal`; the initial closed grammar SHALL be
`acct:<positive-decimal>|card:<positive-decimal>` because account/card are the
only implemented entity types.

A model alias SHALL NOT be authority. Before a provider-originated tool call,
the Go host SHALL resolve it through the existing caseentity private store to
exactly one AuthorityEntityRef under the current TSCV2/DSV2 frozen case. No
second registry, CAS, database, provider-visible reverse mapping, global token,
last-opened/current-case alias, raw-value hash, suffix, deterministic
ciphertext, format-preserving ciphertext, or model-assigned label SHALL be
used as identity. Missing, duplicate, ambiguous, unsupported, stale, or
colliding resolution SHALL fail closed.

Entity identity SHALL be independent from snapshot/evidence/claim identity.
The exact raw value, canonical value, AuthorityEntityRef, StableOrdinal, and
reverse binding SHALL remain only in the existing host-private persistence
topology. The same entity MAY retain its reference
across turns and snapshots in one case, while facts and evidence remain bound
to their exact source snapshot.

#### Scenario: Same entity remains stable across lifecycle operations
- **WHEN** the same authorized account/card is used across funds calls, Todo,
  subagent, compaction, restart, resume, an allowed fork, or an independently
  created thread explicitly bound to the same case
- **THEN** the host retains its AuthorityEntityRef and StableOrdinal, supplies
  the same short typed model alias in that case, and never asks the model to
  infer identity from a mask, suffix, canonical value, or per-turn label

#### Scenario: Snapshot change preserves identity but changes fact version
- **WHEN** the same entity is present in a later accepted snapshot
- **THEN** its authority reference and model alias may remain stable, but old amounts,
  relations, coverage, receipts, and claims become
  historical/stale/superseded until reverified from the new snapshot

#### Scenario: Two cases contain the same complete account
- **WHEN** case A and case B independently contain the same source-exact value
- **THEN** their AuthorityEntityRefs and aliases are unlinkable by default and no
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

#### Scenario: Entity binding is merged split or corrected
- **WHEN** deterministic local evidence requires an entity merge, split,
  canonical correction, canonicalizer upgrade, deletion, or identity-key
  rotation
- **THEN** the existing caseentity owner records an immutable private
  transition, never reuses one ordinal for a different entity, keeps old
  evidence historical, revokes ambiguous aliases, and fails closed until every
  affected current binding can be resolved uniquely

#### Scenario: Unsupported entity type is requested
- **WHEN** a person, organization, phone, device, or merchant alias is requested
  before its canonicalizer and correction contract is implemented
- **THEN** the host returns an unsupported typed boundary and does not treat the
  target vocabulary or a normalized string as an as-built entity identity

### Requirement: Safe Semantic Projection Preserves Investigative Meaning
Removing source-exact PII SHALL NOT remove useful, authorized analytical
semantics. Before provider dispatch the host SHALL assemble a typed short model
alias, entity/account type, task-necessary institution semantics, currency,
ownership/subject,
time, exact amount/direction/balance/frequency/count, counterparty and funds
path, permitted relations, query scope, coverage and missing ranges, evidence
and ClaimRecord references, current/historical snapshot state, verified/
candidate/refuted/unresolved findings, counterevidence, derivation method, and
confidence/support state relevant to the task.

The trusted DuckDB/data tool SHALL perform exact matching, joins, entity
resolution, aggregation, money and graph calculation, and deterministic
derivation from identifiers. Money SHALL use integer minor units or exact
decimal plus currency/scale and SHALL NOT use binary float on any
receipt-eligible path. The model
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
- **THEN** the provider receives the relevant short model aliases,
  relationships, verified and refuted findings, gaps, snapshot comparison,
  evidence references, and open questions without raw PII or reverse mappings

### Requirement: Sensitive User Input Is Tokenized Before Provider Dispatch
If a user prompt contains a complete governed account, card, identity number,
phone, or other classified value, Electron/Go host SHALL resolve it inside the
current active case and snapshot boundary before any provider serialization,
ordinary persistence, search indexing, SSE projection, generic Memory capture,
or compaction. The host
SHALL keep the exact original request and binding/audit relation only in
protected private state and SHALL replace the provider-visible span with the
current case's short typed ModelEntityAlias plus safe semantic context.

#### Scenario: User enters a complete identifier
- **WHEN** the current case/snapshot authority can resolve the exact value
- **THEN** the host privately binds the original input to the authority entity and audit
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

The renderer request SHALL contain only the typed view, ordered allowlisted
fields, bounded row window, and display mode. It SHALL NOT require or accept a
workspace root, thread id, turn id, entity reference, case id, snapshot id,
source locator, database path, or caller-supplied authority. Electron Main
SHALL derive the canonical workspace from current settings and current
main-frame authority, and Go SHALL resolve the active case and current snapshot
from the host-owned case binding. The typed transaction view MAY include
requested source-exact time, account/card/name/identity, amount, direction,
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

Alias resolution MAY use only a process-local short-TTL cache keyed by tenant,
user, case, binding, and epoch. The existing private caseentity store SHALL
remain the source of truth; the alias cache SHALL be cleared on case/snapshot/
epoch change, correction, canonicalizer/key rotation, revoke, deletion,
restart, or ambiguity and SHALL NOT contain source-exact or canonical values.

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
metadata, generic Memory, and unrestricted external effects SHALL use short
typed ModelEntityAlias values plus authorized structured semantics and SHALL NOT
contain source-exact identifiers, reverse mappings, raw rows/evidence,
AuthorityEntityRefs, or DuckDB locators/descriptors. Display masks SHALL NOT serve as analytical
identity, and internal references SHALL NOT appear as user-facing product
language.

Only the closed `typed-local-data-surface/v1` variants
`ImportMappingPreview`, `CleaningDiffPreview`, `DirectSourcePreview`, and
`AcceptedSlotDisplay`, or a Main/Host typed effect path invoked by an explicit
user action, MAY handle source-exact fields outside the trusted private data
plane. The
active-case local UI SHALL default to `displayMode=full`; user-selected
`masked` SHALL change only the final host projection and SHALL NOT change
provider payload, ClaimRecord, EvidenceReceipt, evidence digest, or Final Gate.

#### Scenario: Tool returns full identifiers
- **WHEN** a source outcome contains a complete bank account, identity number, or phone number
- **THEN** the provider receives the short model alias and permitted semantic
  features, every forbidden generic channel remains source-exact-value-free,
  and only an exactly bound typed local sink may receive the requested full or
  host-masked field without receiving the reverse mapping

#### Scenario: Audit artifact is stored
- **WHEN** the runtime persists tool audit material
- **THEN** it is scoped by tenant/thread/case/epoch/snapshot, encrypted or protected according to current storage policy, and does not default to a global cross-case directory containing full PII

### Requirement: Typed Local Data Surfaces Are A Closed Family
The `typed-local-data-surface/v1` family SHALL contain exactly
`ImportMappingPreview`, `CleaningDiffPreview`, `DirectSourcePreview`, and
`AcceptedSlotDisplay`. Every request/response SHALL use a closed field enum,
bounded ordered row window, `full|masked` display mode, no-store policy,
current local session/main-frame principal, exact case and source-generation or
snapshot lineage, expiry, and revoke token. Source-exact bytes SHALL remain in
component-local display state and SHALL be cleared on page replacement,
generation/snapshot/case change, expiry, principal/window invalidation, unmount,
or late response. Generic renderer stores, HTTP/SSE, history, logs, caches,
search, clipboard, and automatic export SHALL receive no display bytes.

`ImportMappingPreview` SHALL bind one host-staged source generation, exact file
or archive-member identity, parser/schema version, mapping draft, fields, and
row window before snapshot admission. It SHALL NOT accept or return a path,
archive locator, raw DTO, unrelated member/column, or caller-supplied authority.
`CleaningDiffPreview` SHALL bind one immutable input snapshot, rule-set digest
and generation, immutable output snapshot, transformation lineage, requested
fields and row window. It SHALL NOT expose raw cleaning logs/history or use a
current database alias. Neither preview SHALL create an Agent turn, provider
call, EvidenceReceipt, ClaimRecord, or Final Gate authority.

#### Scenario: Import mapping preview is bound to one staged generation
- **WHEN** the current principal requests allowlisted fields and a bounded row
  window from one current staged source generation
- **THEN** the host returns only those source-exact or host-masked cells and
  changing/replacing/admitting the generation revokes the response without
  persisting the raw sample DTO or source path

#### Scenario: Cleaning diff preview is bound to deterministic lineage
- **WHEN** the current principal requests allowlisted before/after/status cells
  for one input snapshot, rule generation, output snapshot, and row window
- **THEN** the host returns only those cells plus typed lineage/status and does
  not return raw cleaning logs, unrelated rows, or a generic payload

#### Scenario: Typed local response becomes stale
- **WHEN** its generation, input/output/current/retained snapshot, case,
  principal, window, lease, or requested field no longer matches
- **THEN** the host returns no source-exact bytes, clears pending/component
  state, rejects late delivery, and does not substitute current database data

### Requirement: Field Transformations Preserve Exact Analytics And Bound Reidentification
The host SHALL classify every projected field as direct identifier,
quasi-identifier, exact analytical semantic, untrusted free text,
evidence/provenance identifier, or operational metadata. Direct identifiers,
including names/aliases, identity/passport/tax/organization codes, account/card/
payment/wallet values, counterparty identity, phone/email/address, IP/MAC/device,
coordinates, transaction/order/voucher/case identifiers, filenames/paths,
remarks/OCR/attachment metadata, and biometric/signature material, SHALL remain
local exact and SHALL be projected only as a case alias, closed semantic enum,
declared aggregate/range, or withheld value according to the bounded tool need.

Exact analytical money, balance, net, currency/scale, direction, count,
coverage, missing/null/invalid, query bounds, evidence/currentness, and required
graph direction/topology SHALL remain exact and auditable. Exact timestamp,
merchant/institution/branch, geography, graph degree/rank/community/path, and
rare combinations SHALL be admitted only when task-necessary and after a
combined quasi-identifier risk decision; otherwise the host SHALL use a
declared coarser category/range or withhold them without claiming exactness.
Untrusted free text SHALL be parsed locally, treated as prompt-injection input,
and SHALL NOT become identity or evidence without deterministic extraction and
source lineage. No noise, suffix, bare hash, normalized name, mask,
deterministic ciphertext, or format-preserving encryption SHALL be presented as
exact identity or exact analytical truth.

#### Scenario: Exact money survives privacy projection
- **WHEN** a bounded funds query computes amounts, balance, direction, count,
  coverage, or missing state
- **THEN** trusted DuckDB/Funds code returns integer minor units or exact
  decimal plus currency/scale and closed status enums, and the provider cannot
  replace the deterministic oracle with float arithmetic or prose inference

#### Scenario: Combined quasi identifiers exceed the task need
- **WHEN** exact time, merchant/branch, geography, amount, and graph topology
  together could identify a subject but one or more elements are unnecessary
- **THEN** the host withholds or explicitly generalizes only the unnecessary
  elements, records the resulting precision/coverage, and does not weaken
  exact necessary finance/evidence semantics

#### Scenario: Free text contains identifiers or instructions
- **WHEN** a remark, OCR result, summary, or attachment metadata contains PII
  or prompt-like instructions
- **THEN** local deterministic extraction/quarantine runs before any provider
  projection, unrecognized text is withheld, and no detector is treated as a
  complete safety boundary

### Requirement: Case Storage Memory Logging And Encryption Keep Existing Owners
Raw/normalized/cleaned exact rows, deterministic materializations, and lineage
SHALL remain in existing case DuckDB/immutable snapshots. Source/canonical/
AuthorityEntityRef/StableOrdinal mappings SHALL remain in the existing
caseentity SecurePrivateCAS. Thread/case investigation, evidence, claims,
finals, continuation, and retained display bindings SHALL remain with their
existing typed owners. No second database, vector/graph database, memory
server, identity registry, CAS, or Agent runtime SHALL be added.

The generic Memory registry SHALL remain manual/store-only and SHALL NOT
automatically capture, accept as case authority, inject, or retain case PII,
raw case facts, reverse mappings, AuthorityEntityRefs, case aliases as state,
source paths, raw tool bodies, or retained-snapshot substitutes. Logs,
telemetry, crash reports, diagnostics, and search SHALL contain only fixed
codes, bounded counts, timings, booleans, and schema versions; exact/canonical
values, keys, paths, raw SQL/tool bodies, mappings, case/ref/alias values, and
source rows SHALL be excluded.

SecurePrivateCAS filesystem/private integrity SHALL NOT be described as content
encryption. DuckDB AES-GCM or randomized AEAD inside the existing CAS SHALL
remain disabled until one accepted key contract separates identity-HMAC and
data-encryption keys and specifies OS wrapping, key id/generation/AAD, logical
versus ciphertext digests, backup/restore, retained-snapshot migration,
rotation/rollback, deletion limits, and fail-closed missing/wrong-key behavior.

#### Scenario: Generic Memory receives case content
- **WHEN** automatic capture or a caller attempts to store case PII, raw facts,
  reverse mappings, authority refs, case aliases as continuity state, or source
  paths in `MemoryRecord.content`
- **THEN** the case path rejects the write/injection and continues to use the
  existing caseentity/evidence/claim/continuation owners

#### Scenario: Encryption capability exists in a dependency
- **WHEN** DuckDB or an evaluated crypto library supports at-rest encryption
- **THEN** Analytix does not enable it or change snapshot/CAS bytes until the
  full key/migration/backup/rotation contract and synthetic compatibility tests
  pass; deterministic ciphertext is never used as a model alias

### Requirement: Privacy And Token Economy Have Independent Acceptance
Privacy admission SHALL require zero source-exact bytes and AuthorityEntityRefs
in every provider/model-visible and generic forbidden channel. Token economy
SHALL be measured separately with fully synthetic identifiers by exact
provider/tokenizer/model/version and SHALL report bytes and tokenizer tokens
for source-shaped values, AuthorityEntityRef, ModelEntityAlias, and safe natural
labels. Character count divided by a constant SHALL NOT be token evidence;
unknown provider tokenizers SHALL remain `UNVERIFIED`. Neither token savings
nor PII absence SHALL substitute for semantic/DuckDB-oracle equality.

#### Scenario: Short alias is scored for one provider
- **WHEN** an offline synthetic benchmark has the exact provider-compatible
  tokenizer and version
- **THEN** it reports actual token counts for every candidate representation,
  preserves the separate privacy verdict, and makes no provider call or
  provider-neutral claim

#### Scenario: Provider tokenizer is unavailable
- **WHEN** the target provider/model tokenizer cannot be reproduced exactly
- **THEN** token economy remains `UNVERIFIED` for that provider even if an
  OpenAI-compatible `tiktoken` score exists

### Requirement: Complete Values Remain Outside Model And Generic Funds Channels
The production funds tool SHALL accept only a short typed ModelEntityAlias.
The Go host SHALL resolve that selector to exactly one AuthorityEntityRef under
the current TSCV2/DSV2 snapshot grant before database access and SHALL return
only model aliases, semantic features, and safe
PII-free labels for account/card identities. The trusted plugin/data plane
MAY read the exact governed DuckDB fields
inside its bounded execution in order to compute the authorized result, but it
SHALL NOT return, log, cache, persist, or place a complete identifier in an MCP
response, candidate receipt material, provider continuation, generic HTTP/SSE,
free-form renderer/durable state, history, search, fork/resume, compaction
prose, report bytes, automatic export, log, telemetry, or crash report.
Source-exact local display SHALL occur only through the closed typed local
family, never through the funds tool's public result.

#### Scenario: Ordinary account analysis succeeds
- **WHEN** the host resolves an authorized model alias to one authority account
  reference under the current snapshot grant and the
  bounded query matches source rows containing a complete identifier
- **THEN** aggregation and evidence matching use the governed source internally
  while provider semantics, downstream claims, and generic channels contain
  only the short model alias and PII-free semantics; a later exact
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
re-resolve the accepted result's original source file/row/field lineage through
one callback-scoped exact read against its original witnessed immutable DSV2
snapshot. A private canonical value, current database value, reparsed
approximation, display label, suffix, or model alias SHALL NOT substitute for
that source-exact field.

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
  one callback-scoped retained row/field read, while generic channels contain only
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
gate any standard typed-local-data-surface variant and SHALL NOT require a new
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
