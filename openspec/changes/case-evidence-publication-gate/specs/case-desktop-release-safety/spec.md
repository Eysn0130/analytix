## ADDED Requirements

### Requirement: End-To-End Security Context Propagation
The desktop path `Renderer -> window.analytix -> preload -> main -> Go
HTTP/SSE` SHALL preserve the host-selected workspace/case intent while the Go
runtime remains the sole authority that resolves and freezes
`TurnSecurityContext`. The case context augments concrete protected effects in
the same Agent; it SHALL NOT select a desktop/runtime mode or replace the
ordinary tool catalog.

#### Scenario: Desktop starts a case turn
- **WHEN** the renderer submits a prompt from a bound case workspace
- **THEN** typed shared/main/preload contracts preserve the intended workspace and turn request, and the Go runtime returns the authoritative context digest/epoch through bounded accepted metadata

#### Scenario: Renderer payload attempts to override authority
- **WHEN** a desktop payload supplies case, receipt, support, source identity, or epoch fields outside its allowed request schema
- **THEN** validation rejects or ignores them and the Go host recomputes authority

### Requirement: A0 Packaged General Agent Milestone Is Independent
The packaged Electron product SHALL start and remain useful as a general
Agent when no case project, funds plugin, dataset authority, evidence
registry, or controlled-artifact authority is available. The ordinary Agent
capability base SHALL support coding, file reads and writes, shell, real tests/builds,
research and writing, MCP, Skills, thread, Todo, subagent, compaction,
restart recovery, and normal shutdown under the configured general execution
policy. Case/funds initialization SHALL be lazy or degradable and SHALL NOT be
an unconditional packaged startup prerequisite.

A0/Milestone A acceptance SHALL use the formal packaged Electron application, its
actual renderer/preload/main boundary, the production Go backend, a real
configured provider, and an isolated non-case code repository. It SHALL
complete one user development request by reading the repository, recording a
plan/Todos, modifying files, running the real project test, using a bounded
subagent task, and exercising Git, Skills, ordinary MCP, research, and writing.
With every case dependency absent, a protected funds request SHALL fail closed
as a typed source-unavailable result without disabling the same Agent: that
thread SHALL complete one further small bounded ordinary continuation with a
real ordinary tool before compaction. The formal UI SHALL then invoke manual
`/compact`, observe a durable nonzero compaction item with its source digest
and ancestry and with `auto=false`, exit normally, relaunch the same package in
a fresh process, recover the exact thread, Todo, subagent, compaction, workspace
result, and accepted final, and complete one short real-provider continuation
after relaunch. Final shutdown SHALL leave no owned process, port, or runtime
lock, and the credential scan SHALL pass.

Milestone A SHALL NOT require a single read or echoed tool result of at least
64 KiB, three arbitrary text anchors, or any other large tool result as a
proxy for compaction. The manual `auto=false` event proves the packaged
restart/recovery row only; it SHALL NOT be reported as automatic-compaction
evidence. Unit tests, mock/fake providers, fixture responses, a visible window,
health alone, or process exit alone SHALL NOT satisfy this milestone.

#### Scenario: General packaged coding workflow survives restart
- **WHEN** a user completes the specified non-case development workflow through
  the packaged UI and then exits and relaunches the application
- **THEN** exactly one renderer and one production Go backend served the
  workflow, the real edit and test result remain in the isolated repository,
  the thread/Todos/subagent/compaction ancestry recover consistently, and all
  owned processes exit cleanly after the final shutdown

#### Scenario: Duplicate Plan effect batch is rejected atomically
- **WHEN** an advertised Plan-mode Provider response contains more than one
  `create_plan` call
- **THEN** the host rejects the complete batch before any call-ready record,
  persistence, execution, or plan-file effect, removes the rejected calls and
  arguments from retry history and generic events, and uses a bounded fixed
  correction that permits only a later response with exactly one
  `create_plan` call to execute

#### Scenario: Every case dependency is absent
- **WHEN** the same packaged workflow runs with no case binding, funds plugin,
  dataset snapshot, case evidence witness, or controlled-artifact sink
- **THEN** those capabilities report unavailable without preventing app
  startup, general tool advertisement/execution, persistence, recovery, or
  shutdown

#### Scenario: Formal compaction is manual and recoverable
- **WHEN** the formal Milestone A UI submits `/compact` after the bounded
  post-denial ordinary continuation
- **THEN** exactly the resulting durable nonzero `auto=false` compaction and
  its source digest/ancestry are recovered after a normal quit and fresh
  relaunch, without claiming that an automatic threshold was exercised

### Requirement: B1 Funds Milestone Is Independently Valuable
B1/Milestone B SHALL use the formal packaged Electron application and production
Go/MCP composition in an authorized isolated case workspace bound by the host
to one exact immutable DuckDB snapshot. A real user request for one specified
account/entity and bounded time range SHALL execute
the fixed `analyze_account_flows` funds tool and produce canonical inflow,
outflow, signed net amount, transaction count, and supporting evidence rows.
The host SHALL turn that exact result into an EvidenceReceipt, ClaimRecords,
Final Evidence Gate acceptance, a PII-free generic accepted envelope, and an
`AcceptedSlotDisplay` answer whose active-case local default is source-exact
`full` with user-selectable `masked`.

The model and generic renderer/event/store paths SHALL receive neither the raw
DuckDB path, arbitrary SQL authority, complete account/card values, nor
reversible identity tokens. Within standard local display, only the dedicated
typed sink may receive the exact requested source fields; explicitly invoked
Main/Host typed external effects remain a separate boundary. The result SHALL
be replayable and locally re-resolvable against the same immutable snapshot.
`count_case_rows`, direct MCP invocation, source-only code, fixture data,
unit/integration tests, or Milestone A success SHALL NOT satisfy Milestone B.

B1 SHALL be longitudinal rather than a one-shot turn. The same Agent
and thread SHALL interleave ordinary code/file/shell/Todo/subagent work with
multiple funds queries, preserve stable case-scoped entity identity across
compaction/restart/resume/permitted fork, distinguish historical facts after a
snapshot update, isolate a case switch, and complete both Direct Source Preview
and Final-Gate-gated AcceptedSlotDisplay in `full|masked`. The current local
session, main-frame renderer principal, active case, and immutable snapshot are
the standard local-display authority. Missing `PIIProjectionGrant`, controlled
artifact, legal/third-party approval, or external trusted sink SHALL NOT block
Milestone B local display; unverified external effects remain separate rows.

#### Scenario: Packaged valuable funds question succeeds
- **WHEN** the authorized case, exact DuckDB snapshot, source probe, grant,
  bounded flow query, receipt registry, claim verifier, and Final Gate are all
  current
- **THEN** the generic accepted result stays PII-free, the packaged typed local
  UI renders only the bound flow fields/rows in `full` or `masked`, and replay
  produces the same result/query hashes and original-snapshot display

#### Scenario: Valuable funds path is unavailable
- **WHEN** the exact case/snapshot/tool/evidence chain or local
  session/principal/display binding is absent, partial, stale, mismatched,
  cancelled, timed out, or unauthorized
- **THEN** Milestone B remains blocked or unverified and emits only the typed
  case boundary, while Milestone A remains independently usable

### Requirement: A0 Live And B1 Deterministic Tracks Progress Independently
A Provider reasoning/structured-output contract failure or missing fresh
credential authority SHALL block A0 live certification and any formal B1 row
that actually requires that Provider. It SHALL NOT block deterministic B1
implementation, focused/integrated verification, typed local-display tests, or
a focused local product commit. Commercial completion SHALL require fresh,
separately reported A0 and B1 formal evidence; neither result SHALL substitute
for the other.

#### Scenario: Provider contract blocks A0 live certification
- **WHEN** the configured Provider omits required reasoning/structured output,
  returns invalid markup, or fresh credential authority is unavailable
- **THEN** A0 live is reported `BLOCKED`, no retry expansion or gate weakening
  occurs, and B1 deterministic implementation and verification continue

#### Scenario: B1 deterministic slice passes without formal credentials
- **WHEN** the production B1 source slices and deterministic privacy/display
  tests pass but formal Provider/package authority is unavailable
- **THEN** the B1 product candidate may be committed and reported `PARTIAL`,
  while B1 formal and commercial completion remain `UNVERIFIED` or `BLOCKED`

### Requirement: One Agent Composes General And Case Capabilities
The packaged product SHALL expose one Agent and one durable thread whose
general capability base remains present before, during, and after case work.
Case/funds tools SHALL appear and disappear per current authority and protected
effect without selecting a runtime mode, replacing the catalog, restarting the
Agent, or disabling unrelated ordinary work.

#### Scenario: General tools and funds MCP coexist
- **WHEN** a case workspace has current DSV2/source authority
- **THEN** the same provider catalog contains authorized file, shell, Git,
  Plan, Todo, Skills, ordinary MCP, and subagent tools plus the current funds
  tool, each with its own grant

#### Scenario: DSV2 expires while general tools continue
- **WHEN** DSV2 or funds authority becomes stale after a successful query
- **THEN** later funds access and case-fact publication fail closed, while the
  same Agent can continue authorized code/file/shell/test/Todo/subagent work

#### Scenario: Authority is reacquired in the same Agent
- **WHEN** the host accepts a current case/snapshot/source authority again
- **THEN** funds analysis resumes in the same thread without an Agent/mode
  switch or loss of ordinary/case history permitted by policy

#### Scenario: Mixed request partially succeeds
- **WHEN** one request contains valid ordinary work and a case question whose
  authority is missing
- **THEN** the ordinary result is accepted and durable, the case slot is a
  typed boundary, and no global turn failure or fabricated case completion is
  emitted

#### Scenario: Case switch advances epoch
- **WHEN** the thread changes from case A to case B
- **THEN** the host advances Context Epoch and excludes case A entity
  mappings, evidence, private results, PII, typed display sessions/payloads/
  pending responses, pending effects, and child state from case B; late case-A
  display responses fail closed while ordinary Agent capabilities remain
  available

### Requirement: Packaged Longitudinal Mixed-Work Acceptance
Fresh formal packaged public-seam evidence SHALL use the real renderer,
preload, Electron main, production Go runtime, real configured provider,
installed funds plugin, and an authorized immutable DuckDB case source to
exercise one continuous Agent/thread across ordinary and case work. Direct
HTTP/MCP calls, fixtures, fake providers, unit tests, window appearance, or
exit status SHALL NOT substitute.

#### Scenario: Code work and funds analysis interleave in one thread
- **WHEN** the user performs ordinary code work, a funds query, a code
  modification and real test, another funds query, and a final ordinary task
- **THEN** the same Agent/thread preserves each result, uses current
  per-effect grants, and never replaces ordinary tools with a funds-only
  catalog

#### Scenario: Case workspace uses ordinary and funds capabilities together
- **WHEN** one case investigation uses files, shell, Todo, a bounded subagent,
  and `analyze_account_flows`
- **THEN** all ordinary effects remain usable under ordinary authority, the
  protected query uses only DSV2, and every case conclusion binds current
  receipts/claims

#### Scenario: Compaction restart resume and fork preserve isolation
- **WHEN** the mixed thread compacts, exits normally, restarts, resumes, and
  creates an allowed fork
- **THEN** stable entity identity, verified/refuted findings, gaps,
  evidence/claim bindings, snapshot currentness, ordinary results, and
  capability isolation remain correct with zero complete PII in forbidden
  generic state; persisted typed bindings re-resolve their original immutable
  snapshots through the allowlisted local sink

#### Scenario: Subagent returns an evidence-bound scoped finding
- **WHEN** a parent delegates one case hypothesis
- **THEN** the child receives only scoped stable references and safe semantics,
  returns a typed result bound to parent/case/epoch/snapshot/evidence, and
  receives no complete PII

### Requirement: Accepted Finals And Verified Results Only In History
Thread history, provider history, session resume, search/fork projection, and
compaction SHALL read only accepted final envelopes, closed tool projections,
host-retrieved verified typed case semantics, and typed display bindings for
high-risk content. Durable bindings SHALL contain caseId, immutable snapshotId,
ContextEpoch, entity/slot, ClaimRecord/EvidenceReceipt refs, and necessary
currentness without complete PII prose.
Historical accepted claims from an older epoch/snapshot MAY be retrieved for
comparison only when marked historical/stale/superseded; they SHALL NOT become
current support.

#### Scenario: Rejected provider draft exists in memory
- **WHEN** a model draft fails the final gate
- **THEN** it is absent from durable thread items, provider history, search, fork, resume, and compaction inputs

#### Scenario: Legacy unverified history is opened
- **WHEN** a pre-migration thread contains free-form case conclusions
- **THEN** the UI may display them as legacy/unverified history but they cannot be replayed as evidence or support new claims

#### Scenario: Accepted typed history is reopened
- **WHEN** a current renderer principal opens an accepted Agent result after
  restart, resume, fork, or compaction
- **THEN** the host revalidates its typed binding and resolves the original
  retained immutable snapshot for local display; deletion or validation failure
  shows `source unavailable` rather than current values or persisted PII prose

### Requirement: Closed Tool Results Across Desktop Surfaces
Generic Go HTTP/SSE, Electron events, preload events, renderer mapping/store,
thread state, search/fork/resume, compaction prose, automatic copy/export,
report bytes, and automatic UI actions SHALL accept only the exact closed
public tool-result projection. They SHALL NOT promote arbitrary result objects
into detail text, citations, attachments, generated files, media URLs, file
reads/saves, commands, diagnostics, source-exact display, or development-
preview navigation. Every typed-local-data-surface variant SHALL resolve source
fields from its host-owned staged/input/output/current/retained binding, never
from a public tool result.

#### Scenario: Hostile result crosses HTTP or SSE
- **WHEN** a result carries a sentinel account/MAC, root summary/details, arbitrary code, citation, `dataUrl`, preview URL, local/absolute path, generated file, or localhost server text
- **THEN** generic main/preload/renderer/history/export paths receive only
  `legacy_output_withheld` or a valid closed host projection, no sentinel byte
  reaches those forbidden channels, and no media/file/network/preview action
  executes; a separate positive test may display a source sentinel only after
  the typed local sink resolves an independent valid source binding

#### Scenario: Typed local source value is displayed
- **WHEN** Direct Source Preview or a Final-Gate-accepted slot binds the current
  principal/session, exact case/snapshot, and requested rows/fields
- **THEN** desktop display resolves that host-owned binding and never trusts
  inline tool bytes, a path, URL, filename, masked token, or tool-supplied id as
  authority

### Requirement: Fact-Safe Streaming
The desktop MAY stream host-originated progress but SHALL NOT stream high-risk factual text before final acceptance.

#### Scenario: Tool query is running
- **WHEN** a high-risk case turn is collecting sources
- **THEN** SSE may report sanitized readiness/progress/blocker states without amounts, accounts, relationships, quotes, device ids, or provider draft prose

#### Scenario: Accepted answer is ready
- **WHEN** the final envelope is atomically persisted
- **THEN** SSE and generic renderer state receive the deterministic PII-free
  accepted answer and typed display binding after durability; source-exact
  fields arrive only through a later AcceptedSlotDisplay request

### Requirement: Renderer External Effects Are Explicit And Host-Owned
The renderer SHALL NOT create a case-data Blob, browser download, clipboard
write, local path, or generic write request from inline display/tool bytes.
Copy, save/export, print, drag/drop, external application, Connector, email,
upload, share, and cross-case analysis SHALL start only from an explicit user
action whose Main/Host typed effect binds the exact target, case, immutable
snapshot, and fields. The user click SHALL be sufficient runtime authorization
for that exact effect; no third-party, administrator, or per-field approval
SHALL be required. A provider-bound Connector SHALL remain PII-free.

#### Scenario: User copies selected visible fields
- **WHEN** the user clicks copy for an exact selected field set
- **THEN** Main resolves and writes only those fields to the clipboard, makes no
  provider call, performs no automatic copy, and requires no third-party
  approval

#### Scenario: User exports selected case data
- **WHEN** the user selects an exact export target, case, snapshot, and fields
- **THEN** Main/Host resolves the typed effect under applicable report/artifact
  gates, and the renderer creates no Blob, object URL, download element, or
  caller-supplied file bytes

#### Scenario: Renderer forges a generic write request
- **WHEN** an authenticated renderer calls a save/write IPC directly with case
  bytes or without the exact user-action binding
- **THEN** Main fails closed before reading the supplied bytes or opening a
  dialog and creates no file

### Requirement: Ordinary DOCX Is A Main-Owned Desktop Write Artifact
The first macOS release SHALL provide ordinary Desktop Write DOCX export without
placing document generation in `packages/runtime-go`, a model-callable tool,
MCP, or the funds plugin. Electron Main SHALL require the current main-frame
renderer, derive the active Write workspace from current settings, resolve the
source and every embedded image inside that canonical workspace, apply the
ordinary masked and no-private-reasoning projection, own the save dialog and
file write, and generate OOXML from an allowlisted plain-text/Markdown document
AST. The renderer SHALL NOT provide workspace authority or output bytes.

The DOCX producer SHALL use no arbitrary HTML-to-DOCX conversion, Pandoc,
LibreOffice, Python, native document runtime, child process, Provider, MCP,
Agent turn, EvidenceReceipt, ClaimRecord, or Final Gate. It SHALL support
Chinese/East-Asian font names, headings, paragraphs, alignment, emphasis,
ordered and unordered lists, tables, workspace-local images, safe links,
explicit page breaks, and A4 page layout. It SHALL embed images and SHALL NOT
emit external file/image relationships. Its admitted production dependency
graph SHALL have no known high or critical vulnerability; an override that
leaves vulnerable bundled code present SHALL NOT satisfy this requirement.

#### Scenario: User exports an ordinary Markdown document
- **WHEN** the current main-frame user chooses DOCX for a text document inside
  the active Write workspace
- **THEN** Main produces a structurally valid OOXML package through the host
  save dialog, Word/WPS-compatible constructs preserve the supported document
  model, complete identifiers are masked, and Agent/Provider/MCP/child-process
  call counts remain zero

#### Scenario: Renderer forges workspace or image authority
- **WHEN** a renderer adds an unknown workspace locator, supplies a source or
  image outside the Main-owned active workspace, uses a symlink escape, or a
  non-current frame calls the export IPC
- **THEN** strict IPC parsing or Main containment fails before the save dialog
  and no output file, external relationship, Provider call, or child process is
  created

#### Scenario: Unsupported active content appears in Markdown
- **WHEN** Markdown contains raw HTML, scripts, remote images, unsafe link
  schemes, an unsupported image signature, or an explicit page-break lookalike
- **THEN** the producer treats it as inert text or rejects it, never executes or
  fetches it, and recognizes only the exact host-defined page-break marker

#### Scenario: Compatibility application is unavailable
- **WHEN** Microsoft Word or another requested compatibility application is not
  installed on the validation host
- **THEN** deterministic OOXML and available-application evidence remains
  reportable while the unavailable application row stays `UNVERIFIED`

### Requirement: Snapshot Selection And Query Handle Are Host-Owned
The renderer MAY request case-source selection only through a zero-authority
host action. A typed local-display request MAY echo a host-issued case/snapshot/
epoch/display-binding selector but SHALL NOT use it as authority or override
host currentness. The renderer SHALL NOT supply or receive a database path, raw
database bytes, private manifest fields/hashes, key, staging slot, query handle,
locator/descriptor, or authority token. Electron Main and Go SHALL bind the
authorized case to one exact immutable analytical DuckDB snapshot; the runtime
SHALL expose only a callback-scoped fixed-operation or requested-preview-field
handle inside the existing DSV2 lease.

The existing strict-CSV staging/count path MAY remain available for its canary
contract, but it does not establish analytical DuckDB readiness. The first
packaged funds milestone SHALL reuse the accepted DSV2 and installed native
component authorities rather than add a staging/admission/receipt/registry
version. Failure to select or query the exact snapshot SHALL disable the
valuable funds tool only and SHALL NOT block general packaged startup.

#### Scenario: Host selects an analytical DuckDB snapshot
- **WHEN** an authorized case action selects a DuckDB snapshot whose exact
  bytes, schema, producer/materialization identity, source lineage, and case
  binding all validate
- **THEN** the existing DSV2 authority admits the selection and makes only the
  fixed `analyze_account_flows` operation and exact Direct Source Preview
  field-selection read available inside `UseExact`, without returning a path
  or reusable handle to Electron renderer or MCP callers

#### Scenario: Snapshot changes or locator is substituted
- **WHEN** database bytes, schema, producer/materialization identity, case
  binding, stable file identity, or the host-private locator differs before or
  after query execution
- **THEN** the funds call fails closed, issues no receipt, reconciles a newly
  accepted snapshot through Context Epoch before later use, and does not fall
  back to a mutable path, environment-derived case database, or count canary

#### Scenario: Only count staging is available
- **WHEN** the packaged host can admit the strict-CSV count artifact but has no
  exact analytical DuckDB selection
- **THEN** `count_case_rows` remains canary-only, `analyze_account_flows` is
  unavailable, Milestone B remains incomplete, and Milestone A remains usable

### Requirement: External Data Runtime Effects Are Identity-Pinned And Bounded
Every data-analysis subprocess, converter, archive extractor, or packaged interpreter SHALL execute only a canonical absolute regular non-symlink executable in an authorized canonical working directory with a closed environment, bounded stdout/stderr, timeout/cancellation, and whole-process-tree termination. Source archives and converter inputs SHALL be read through stable handles, verified by complete SHA-256 against an exact manifest, constrained by count/expanded-byte/path/depth quotas, and committed with fsync-backed no-overwrite publication. Cached markers or path strings SHALL NOT replace live source identity and hash validation.

#### Scenario: Executable, cwd, or archive changes after validation
- **WHEN** an executable/cwd/source path is replaced, symlinked, re-opened to different bytes, or a cache marker refers to stale content
- **THEN** execution/extraction/conversion fails before the untrusted runtime observes the input and no prior published generation is deleted

#### Scenario: Child floods output or archive expands beyond bounds
- **WHEN** a process exceeds output/time/tree bounds or an archive exceeds member, byte, path, or depth limits
- **THEN** the complete process tree is terminated, staging is quarantined or removed safely, and no partial or attacker-selected target is published

### Requirement: Reasoning Zero-Byte Boundary
Reasoning/thinking content SHALL have zero visible or exported bytes across success, failure, tool call, recovery, history, compaction, logs, traces, reports, and exports.

#### Scenario: Desktop replays all events after restart
- **WHEN** a thread with provider reasoning is reloaded from durable state
- **THEN** no reasoning item/delta/text/signature is returned to main, preload, renderer, export, or report surfaces

### Requirement: Automatic Compaction Preserves Typed Continuation
The production Go runtime SHALL perform deterministic pre-turn automatic
compaction for the one Agent continuation when the bounded request estimate
reaches 75% of the selected model's context window. A turn admission SHALL
compact at most once, SHALL re-estimate after compaction, and, when the exact
provider-request estimate still reaches the 85% hard threshold, SHALL make
zero provider calls and return the fixed host blocker.

This automatic contract SHALL be proven at deterministic production-Go
focused/integration seams, including the pre-turn estimate, exact provider
request estimate, one-compaction limit, post-compaction hard blocker, and the
distinct `auto=true` lifecycle event. The formal packaged Milestone A manual
`/compact` row SHALL NOT substitute for that deterministic evidence.

The compaction SHALL preserve a digest-bound active Goal, unfinished Todos,
latest user constraints, private authority refs/StableOrdinals through typed
host state and provider-safe short aliases through provider projection,
current/historical claims, counterevidence/refuted hypotheses, open questions,
data gaps, evidence/snapshot references and currentness, and compaction
ancestry, plus typed display bindings containing caseId, immutable snapshotId,
ContextEpoch, entity/slot, ClaimRecord/EvidenceReceipt refs, and necessary
currentness. Complete PII, reverse mappings, raw rows/tool results, and
reasoning SHALL remain absent. Host-owned claim/evidence/entity/display state
SHALL be retrieved after compaction rather than compressed into unverified
model prose.

When the reconstructed typed state exceeds the post-compaction context budget,
the host SHALL select task-relevant state in this deterministic order: current
verified facts, historical comparison facts, key relationships,
counterevidence/refuted findings, data gaps, evidence/claim references, then
current-versus-historical snapshot differences. It SHALL retain exact
currentness, coverage, and evidence bindings for selected state and SHALL NOT
upgrade omitted or lower-authority state.

#### Scenario: Agent thread reaches the soft threshold
- **WHEN** the current provider request estimate reaches the deterministic 75% soft threshold
- **THEN** the host compacts before turn admission, records `auto=true`, restores the exact typed continuation into the dynamic user context, and invokes the provider only after the post-compaction estimate is below the hard threshold

#### Scenario: Compacted request remains above the hard threshold
- **WHEN** the exact provider-request estimate remains at or above the 85% hard
  threshold after the one permitted automatic compaction
- **THEN** the host makes zero provider calls and returns the fixed context
  blocker without relabeling the automatic event as manual compaction

#### Scenario: Repeated compaction carries unknown or failed state
- **WHEN** a Goal/Todo/evidence reference is missing, unknown, failed, canceled, partial, or unverified before one or more compactions
- **THEN** every later continuation preserves or lowers that state and MUST NOT turn it into zero, completed, passed, verified, or evidence authority

#### Scenario: Case investigation continues after compaction
- **WHEN** an authorized case thread reaches the soft threshold and its
  host-owned typed investigation state can be reassembled
- **THEN** automatic compaction preserves entity continuity, verified and
  refuted state, gaps, current/historical snapshot binding, and evidence
  references; the same Agent continues with forbidden generic channels PII-free
  and can re-resolve accepted display bindings against their original retained
  snapshots

#### Scenario: Case state cannot be reassembled safely
- **WHEN** required claim/evidence/entity currentness is corrupt, ambiguous, or
  unavailable after compaction
- **THEN** only the affected case continuation is boundary-only with zero case
  provider/effect calls; ordinary Agent work remains available and no state is
  guessed or deleted

### Requirement: Todo Terminal Lifecycle Is Auditable
Todo state SHALL use the closed union `pending`, `in_progress`, `completed`, `failed`, and `canceled`. `failed` and `canceled` SHALL require a host-enumerated `statusReasonCode`; other states SHALL reject that field. Failed or canceled work SHALL remain unfinished for Goal closure, SHALL survive restart/fork/resume, and SHALL NOT be deleted or converted to completed merely because an Agent final says the task is done.

#### Scenario: Tool, subagent, or runtime work fails
- **WHEN** an active Todo cannot complete
- **THEN** the host persists `failed` with a fixed non-sensitive reason code, retains it as unfinished, and projects the same state through HTTP, Electron, renderer, and restart recovery

#### Scenario: User or parent cancels work
- **WHEN** an authorized cancellation targets a pending or active Todo
- **THEN** the host persists `canceled` with its fixed reason code and does not delete the audit record or count it toward Goal completion

#### Scenario: Malformed Todo replacement is submitted
- **WHEN** an inbound Todo payload contains a non-object item, duplicate id, unknown property, invalid transition, multiple `in_progress` entries, or a terminal state with an invalid/missing reason
- **THEN** the request fails before persistence and the prior durable Todo set remains unchanged

#### Scenario: Failed work is explicitly retried
- **WHEN** an authorized transition moves a failed or canceled Todo back to `in_progress`
- **THEN** the old terminal reason is cleared, the retry is audited, and later completion still requires the normal Goal evidence rules

### Requirement: Safe Evidence Observability
Operational events SHALL expose blocker reason, source readiness, receipt coverage, claim rejection, epoch mismatch, recovery downgrade, terminal reason, and publication state using bounded codes/counts/hashes and SHALL exclude credentials, raw case data, full PII, unsafe response bodies, and reasoning.

#### Scenario: Claim is rejected for citation mismatch
- **WHEN** the verifier rejects a claim
- **THEN** diagnostics identify the claim type and stable mismatch reason without logging the full claim payload or evidence content

### Requirement: Restart And Resume Fail Closed
Durable restart SHALL restore the accepted Context Epoch, receipt registry,
accepted final history, pending gate metadata, stable case-scoped entity
bindings, investigation states, counterevidence, gaps, open questions, and
current/historical snapshot relationships, then revalidate source identity,
grants, snapshots, and integrity before resuming protected work. It SHALL not
restore complete PII or reverse mappings into provider history, generic
accepted-final/history/events, free-form renderer state, search, logs, or
compaction prose. It SHALL retain typed display bindings and re-resolve their
original immutable snapshots through the current typed local sink.

#### Scenario: Runtime restarts with a stale pending grant
- **WHEN** a persisted grant no longer matches source connection, epoch, case, catalog, or expiry
- **THEN** it is rejected and no tool/report side effect executes

#### Scenario: Accepted final replays after restart
- **WHEN** receipt/claim/final registry integrity verifies
- **THEN** the exact deterministic PII-free accepted envelope and typed binding
  may replay without another model call, and local source-exact display is
  freshly resolved from the bound immutable snapshot

#### Scenario: Same case resumes longitudinal analysis
- **WHEN** the application restarts under a current authorization for the same
  case
- **THEN** the host reassembles stable entity semantics, verified/refuted
  findings, gaps, historical/current facts, and evidence references before the
  next provider call, without relying on a free-text summary

### Requirement: Production Gate Is Mandatory
The final evidence and report publication gates SHALL be enabled for every
production high-risk case path and SHALL NOT have an ordinary user setting,
provider option, prompt, skill, MCP response, or environment toggle that
permits free-form high-risk publication. Missing case gate dependencies SHALL
disable the affected protected data call, case-factual answer slot, report, or
controlled PII effect, not the general Agent runtime, ordinary catalog, or
independently verified ordinary result in the same turn.

#### Scenario: Production starts with missing gate dependencies
- **WHEN** registry, verifier, snapshot, or publication components cannot initialize
- **THEN** high-risk case answers and reports stay boundary-only rather than starting in an unsafe mode

#### Scenario: General production turn starts without case gates
- **WHEN** the packaged runtime is healthy for general work but case gate
  dependencies are unavailable
- **THEN** the general turn starts under its ordinary policy and cannot invoke
  or impersonate a case-fact capability

### Requirement: Fail-Closed Rollback
Rollback SHALL retain the publication gate or disable high-risk case publication. It SHALL NOT restore free-form case answers, unsafe report fallback, or the retired TypeScript agent runtime.

#### Scenario: New release is rolled back
- **WHEN** an operator selects a previous supported build
- **THEN** release evidence proves that build enforces equivalent receipt/final gates or enters boundary-only mode

### Requirement: Desktop Failure Matrix
Release tests SHALL cover cold user configuration, no MCP, disconnected MCP,
cached stale catalog, missing plugin, spoofed source, DSV2 loss/reacquisition,
mixed ordinary/case requests, protected-source read/bash bypass, case switch,
stale approval, compaction/restart/fork/subagent/independent-thread recovery,
stable three-layer entity identity, snapshot evolution, all four typed local
data surfaces in `full|masked`, provider/generic-channel PII zero-scans, allowlisted typed-sink
positive scans, exactly-once carrier consumption, display revocation,
existing-surface automatic-disclosure prevention, packaged runtime, UI
  projection, and rollback. New external-effect behavior is outside B1.

#### Scenario: Packaged app has no funds plugin
- **WHEN** a new user opens a case request without a configured live source
- **THEN** the visible desktop answer is the fixed source-unavailable boundary and contains no case fact

#### Scenario: Non-case workspace has no funds plugin
- **WHEN** a new user opens an ordinary code project without a funds plugin
- **THEN** the packaged renderer, Go backend, Agent loop, general tools,
  threads, Todos, subagents, compaction, recovery, and normal exit remain
  available

#### Scenario: Case changes while source call is in flight
- **WHEN** the desktop switches from case A to B before A completes
- **THEN** A's tool/display result is rejected, A's display payload and pending
  response are cleared, and no late A value appears in B's typed UI, generic
  history, report, or export

#### Scenario: DSV2 expires during mixed work
- **WHEN** a funds call loses DSV2 authority while ordinary code/file/test work
  remains authorized
- **THEN** the funds call and case slot fail closed, ordinary work completes,
  and reacquiring authority later restores funds analysis in the same Agent

#### Scenario: Ordinary shell attempts protected DuckDB access
- **WHEN** read/bash or a descendant tries a direct or aliased protected source
  path
- **THEN** the public seam observes zero protected bytes while ordinary
  workspace file/build/test operations continue

### Requirement: Packaged Stable-Identity And Typed Local-Display Acceptance
Fresh formal packaged evidence SHALL prove stable three-layer case identity,
exact DuckDB semantics, all four typed-local-data-surface variants, independent
same-case thread rehydration, forbidden generic-channel
privacy, and display/effect lifecycle against a real immutable DuckDB snapshot.
Source-only tests, fixtures, direct MCP calls, masks compared as identity, a
renderer mock, or historical package evidence SHALL NOT satisfy these rows.

#### Scenario: Import Mapping Preview is generation-bound
- **WHEN** the current principal previews allowlisted source cells from one
  staged file/archive-member generation before snapshot admission
- **THEN** only requested source-exact or host-masked cells reach component-
  local typed state, no path/raw DTO/provider/generic event is emitted, and
  replacement/admission/expiry revokes the response

#### Scenario: Cleaning Diff Preview is lineage-bound
- **WHEN** the current principal previews allowlisted before/after/status cells
  for one input snapshot, rule generation, and output snapshot
- **THEN** only requested typed cells and lineage/status reach the local sink,
  raw cleaning logs/history stay generic-channel-free, and stale/late delivery
  fails closed

#### Scenario: Direct Preview full returns only the selected source fields
- **WHEN** the current local session and main-frame principal request specified
  typed rows/fields with `displayMode=full` without supplying thread, turn,
  entity-reference, case, snapshot, or source-locator authority
- **THEN** the typed local sink shows their source-exact values, returns no
  unrelated field, mapping table, key, locator/descriptor, raw evidence, or
  cross-case link, and provider/MCP/EvidenceReceipt/ClaimRecord/Final Gate call
  counts are zero

#### Scenario: Direct Preview masked has no secondary effect
- **WHEN** the same preview uses `displayMode=masked`
- **THEN** the host returns a masked projection and creates no provider call,
  durable PII, clipboard/write, or external effect

#### Scenario: Agent Result full uses settled evidence and retained binding
- **WHEN** `analyze_account_flows` produces a PII-free provider request/result
  and EvidenceReceipt, ClaimRecord, and Final Gate all match the exact
  case/snapshot/epoch and typed source-field slots
- **THEN** the execution evidence carrier has already been consumed or
  discarded exactly once, Go reads only the original source file/row/field
  lineage through one callback against the retained witnessed DSV2, and the typed local
  sink shows source-exact values while generic accepted-final/history/HTTP/
  SSE/events/stores remain PII-free; canonical binding/current database values
  are not accepted substitutes

#### Scenario: Agent Result masked changes only final projection
- **WHEN** the same accepted Agent result is viewed with `displayMode=masked`
- **THEN** provider body/result, ClaimRecord, EvidenceReceipt, evidence digest,
  Final Gate, and persisted binding match the `full` run exactly, and only the
  host-produced local projection differs

#### Scenario: Reopen restart fork and compaction re-resolve original snapshot
- **WHEN** an accepted Agent result is reopened after any of those lifecycle
  operations
- **THEN** free-form prose/provider continuation remains PII-free, the typed
  case/snapshot/epoch/entity/slot/claim/receipt/currentness binding is retained,
  and the local sink resolves its original immutable snapshot; explicit
  deletion or validation failure displays `source unavailable`

#### Scenario: Independent thread rehydrates same-case aliases and state
- **WHEN** the user creates a separate thread and explicitly binds the same
  authorized case after restart
- **THEN** existing case owners rehydrate the same account/card authority refs
  and short aliases plus bounded typed currentness/evidence, while generic
  Memory, old prose, last-opened case, exact PII, and reverse maps remain absent

#### Scenario: Existing external surface does not disclose automatically
- **WHEN** an existing copy/export/share/print/drag-drop/external-app/
  Connector/email/upload surface is present during B1 validation
- **THEN** it emits zero source-exact bytes without an explicit user action,
  no new external-effect behavior is added, and a Provider-bound Connector
  remains source-exact-PII-free

#### Scenario: Case switch revokes display state and late responses
- **WHEN** the active binding changes from case A to case B while a preview or
  AcceptedSlotDisplay response is pending
- **THEN** case A's display session, payload, and pending response are cleared,
  the late response fails closed, and no A value appears in B

#### Scenario: Snapshot update separates current preview and historical answer
- **WHEN** a later immutable snapshot is accepted in the same case
- **THEN** current Direct Source Preview uses the new snapshot, entity
  continuity may remain stable, prior facts are historical/stale/superseded,
  and a historical Agent result continues resolving its bound old snapshot

#### Scenario: Cross-case selection preserves independent namespaces
- **WHEN** the user explicitly selects the exact cases for joint analysis
- **THEN** each entity reference stays case-scoped, each case keeps its own
  snapshot/currentness/claims/receipts, and no model or UI receives a cross-case
  reverse mapping; revoking one case removes the relation without leaking or
  deleting the remaining case state

#### Scenario: Forbidden-channel zero scan and typed-sink positive scan agree
- **WHEN** account, card, identity-number, phone, address, device,
  transaction-id, path, OCR/free-text, and quasi-identifier sentinels traverse
  all applicable typed local and Agent scenarios
- **THEN** provider/model/continuation, subagent, MCP/public tool result, generic
  HTTP/SSE/event, free-form durable conversation/renderer state, search, log,
  telemetry, crash-report, compaction-prose, and unrestricted-external channels
  contain zero sentinel bytes, while a separate positive assertion proves the
  same sentinel appears in each applicable allowlisted typed local sink; an all-UI/all-DOM
  zero-byte assertion is not used

#### Scenario: Multi-turn DuckDB truth and entity stability
- **WHEN** the same account/subject is analyzed over multiple turns and related
  queries
- **THEN** its AuthorityEntityRef/StableOrdinal and short model alias remain
  stable in that case and exact amounts,
  relationships, counts, and evidence rows match independently recomputed
  DuckDB truth without using a mask as identity

#### Scenario: Count canary cannot satisfy acceptance
- **WHEN** `count_case_rows` succeeds without longitudinal flow analysis,
  snapshot evolution, all four typed local-display variants, and lifecycle evidence
- **THEN** it remains plumbing evidence only and every Milestone B acceptance
  row remains open

### Requirement: Platform Validation Is Truthful
The current platform SHALL complete a real Electron and package validation. Other platforms SHALL be marked passed only after supported CI or an approved host actually runs the relevant build, package, startup, and gate tests.

#### Scenario: Windows was not run from an approved Windows environment
- **WHEN** only macOS package tests were executed
- **THEN** Windows remains unverified rather than inferred from configuration

### Requirement: Deterministic RC Structure And Performance Gates Are Separate
The runtime structural gate SHALL prove durable-before-publish, first-event and
terminal ordering, zero provider draft/reasoning/PII leakage, and correct
provider-attempt and rejection diagnostics without applying a wall-clock
threshold. The RC performance gate SHALL separately bind exact HEAD/tree, the
Go and Node toolchains, the current supported macOS arm64 host profile, and the
SHA-256 of one production-tag runtime binary built exactly once. It SHALL use
fresh isolated cold data/thread roots, reuse that binary across two unmeasured
warmups and twenty measured samples, record this cache-state contract and
nearest-rank p50/p95, and SHALL require `first_runtime_event_ms` p95 `<=250ms`; it SHALL
report every sample and max without using max as the gate statistic.

#### Scenario: One measured sample exceeds 250 ms but p95 passes
- **WHEN** all structural checks pass and the nearest-rank p95 across twenty
  measured samples is at most 250 ms
- **THEN** the performance gate passes and reports the larger maximum as
  diagnostic information without replacing p95 with that single value

#### Scenario: The host profile is unsupported
- **WHEN** the benchmark host does not match the accepted macOS arm64 profile
- **THEN** the benchmark is `NOT_CONFIGURED` and `UNVERIFIED`, its release
  authorization is false, and no sample or historical receipt can make it pass

#### Scenario: Controlled p95 fails
- **WHEN** nearest-rank p95 exceeds 250 ms
- **THEN** the receipt reports PII-free phase evidence for HTTP admission,
  history read/preflight, durable append/fsync, and SSE publish; any
  unattributable phase stays unavailable and blocks both optimization and the
  performance gate, and the source freeze is not blindly retried for a PASS

### Requirement: RC Release Validation Is Single-Execution And Source-Bound
The normalized release gate SHALL own one execution manifest in which
typecheck, runtime build, full root tests, ordinary/prod/race Go matrices,
structural performance, and the controlled benchmark each execute at most once
for an exact source freeze. It SHALL generate and revalidate fresh receipts
bound to HEAD, tree, command, cwd, toolchain, result, and any applicable exact
production binary. Nested standalone validation scripts SHALL NOT cause a
second execution of a heavyweight entry.

The one logical run SHALL expose explicit execution and independent final
adjudication phases. Execution SHALL freeze tested source S, the initial
canonical task-ledger hash, the exact permitted closure row IDs from
10.4/10.8/10.9 and one report path with its initial absent or exact blob/mode
state. Substantive success SHALL retain every existing artifact, upstream,
formal A0/B1, execution-count and receipt predicate, and SHALL emit only an
execution success seal with final `passed=false`. Only final adjudication MAY
emit final `passed=true`.

The existing validation wrapper SHALL observe its own execution child exit 0
without a signal, spawn error or wrapper interruption before writing a
completion receipt bound to the child's IPC seal, PID, source and run. Final
adjudication SHALL require that receipt; PID absence or a user-supplied exit
code SHALL NOT establish successful completion.

After execution exits, closure source C SHALL be exactly one direct child
commit of S. Only the declared existing checkbox bytes `[ ]` to `[x]` and the
predeclared regular report MAY change; all other task bytes and all other Git
objects, paths, types and modes SHALL remain unchanged. Final adjudication
SHALL be read-only for source, tasks and Git index, SHALL invoke no heavyweight
checks, and SHALL require exact same-run evidence, a structurally complete
10.8 report and a valid current ledger with zero open RC_REQUIRED IDs.
Original package/A0/B1 receipts SHALL remain bound to S; the final receipt
SHALL separately record S, C and the exact validated diff. Report structure
SHALL NOT substitute for the Owner's factual acceptance of actual obligations.

#### Scenario: Execution succeeds before closure rows can truthfully close
- **WHEN** every substantive check passes while declared execution/report/DoD
  rows remain open in the frozen initial ledger
- **THEN** execution emits its separately named success seal without final PASS
- **AND** the Owner may review and close only factually complete rows after
  the execution process exits, without rerunning the heavyweight manifest

#### Scenario: Exact closure enables the independent final verdict
- **WHEN** the execution process has exited and C contains only the declared
  checkbox changes and complete report with matching run/seal/receipt digests
- **AND** every real RC_REQUIRED obligation is closed and all frozen source,
  task, report and receipt bytes still match
- **THEN** read-only final adjudication emits the final receipt with S and C
- **AND** an identical-input repetition returns that receipt without overwrite

#### Scenario: Closure or frozen evidence changes beyond the supported protocol
- **WHEN** a product, script, test, dependency, requirement, extra document,
  task ID/classification/description, type/mode, receipt or report hash changes,
  or C has an extra commit/parent, or a real RC obligation remains open
- **THEN** final adjudication fails closed without source/task writes or
  heavyweight execution and does not overwrite historical receipts

#### Scenario: Execution or final report is incomplete
- **WHEN** execution failed, was interrupted, skipped or omitted a substantive
  check, terminated abnormally after writing a seal, lacks the supervising
  wrapper's completion receipt, or the report omits a mandatory 10.8 section
  or same-run reference
- **THEN** no final PASS is available and the existing RED or blocked evidence
  remains preserved; structural report validation alone cannot accept facts

#### Scenario: A fresh receipt matches the current release-gate run
- **WHEN** the receipt was generated by the same release-gate run and every
  source, command, toolchain, binary, result, and digest field matches
- **THEN** the release gate may consume it once and record the receipt digest

#### Scenario: A receipt or execution manifest drifts
- **WHEN** a receipt is historical or any bound source, command, toolchain,
  binary, result, digest, or expected execution count differs
- **THEN** the control plane fails closed; a dry-run, skip flag, or environment
  override cannot authorize it

### Requirement: Artifact Legal Obligations Are Artifact-Bound
Artifact admission SHALL inspect the exact packaged file and production
dependency inventory for mandatory LICENSE, NOTICE, copyright, attribution,
and redistribution obligations. It SHALL fail closed only when a concrete
artifact entry has a governing term that requires an action which the exact
artifact does not satisfy. External research checkout state, historical
starting point, product-lineage labels, comparison freshness, and private
authorization records SHALL NOT be product or package gates.

#### Scenario: Exact packaged entry has an unmet mandatory obligation
- **WHEN** a concrete file or production dependency in the artifact requires a
  LICENSE, NOTICE, copyright, attribution, or redistribution action that the
  artifact does not contain or perform
- **THEN** admission blocks and identifies that exact artifact entry,
  governing term, and missing required action

#### Scenario: No concrete artifact obligation is missing
- **WHEN** the exact artifact inventory satisfies every mandatory legal notice
  and redistribution obligation applicable to its packaged entries
- **THEN** artifact admission passes without consulting research checkout
  freshness, historical lineage labels, or a private authorization ledger

### Requirement: First-Stage Milestones Stop Independently Of Later Scope
Milestones A and B SHALL each have fresh packaged public-seam evidence and
SHALL be recorded separately. Milestone B includes Direct Source Preview and
Agent AcceptedSlotDisplay in `full|masked` under current local-display
authority; no controlled artifact, PIIProjectionGrant, external trusted sink,
or third-party/legal approval is a prerequisite. Signing/notarization, unrun
target operating systems, formal report expansion, external effects, optional
high-security viewer remain
external-condition or later-scope rows. No row SHALL be fabricated or
substituted for another.

#### Scenario: Both packaged milestones pass on the current platform
- **WHEN** fresh public-seam evidence proves Milestone A and Milestone B on the
  current supported package
- **THEN** first-stage construction stops without automatically entering
  formal report expansion or unrun cross-platform release work; unexecuted
  external effects remain
  separately unverified without negating the standard local-display result

#### Scenario: Deterministic B1 closes while A0 live is blocked
- **WHEN** B1 focused/integrated production tests pass and the A0 Provider
  contract or fresh credential authority remains unavailable
- **THEN** the deterministic B1 candidate is retained and may be committed,
  while A0 live, B1 formal, package, and commercial statuses remain separately
  blocked or unverified

### Requirement: Deterministic Release Metrics
The release evidence SHALL compute the following metrics from executable
tests: unsupported high-risk claim released, unknown/mismatched citation
accepted, cross-case receipt accepted, unadvertised tool executed,
stale/spoofed MCP accepted, source-unavailable case fact, recovery evidence
upgrade, reasoning visible/exported bytes, report without publication receipt,
`write_report=false` file writes, receipt coverage, applicable P0/P1
pass/skip state, packaged general-Agent acceptance, and packaged valuable-funds
acceptance; entity-reference drift/collision, stale fact promoted to current,
default cross-case token correlation, protected-source ordinary-tool bytes,
mixed-request global failure, complete identifier bytes in provider/model/
continuation/subagent/MCP/public-tool/generic HTTP-SSE-event/free-form durable
history-renderer/search/log/telemetry/crash/compaction channels, internal
reference visible to the user, Direct Preview full/masked selection leakage,
Agent private-carrier consumption count, full/masked pre-projection mismatch,
typed-sink positive source-value display, original-snapshot history resolution,
stale display delivery after case/snapshot/session/window change, external
effect without explicit action, and provider-bound Connector PII leakage. A
global UI/DOM zero-byte metric SHALL NOT replace the forbidden-channel negative
scan plus allowlisted-sink positive scan.

#### Scenario: Any redline metric is nonzero or unknown
- **WHEN** release evidence shows a violation, a missing applicable
  first-stage measurement, a failed applicable first-stage test, or a skipped
  applicable P0/P1 scenario
- **THEN** release is blocked and the Goal remains active

#### Scenario: LLM judge reports success
- **WHEN** model adherence or a qualitative reviewer passes but a deterministic metric fails or is absent
- **THEN** the deterministic gate remains authoritative and release stays blocked

### Requirement: Skills Are Content-Addressed And Cannot Expand Authority
The Go host SHALL discover each Skill into an immutable versioned package snapshot that binds the manifest, selected entry, every consumed reference, and every advertised helper script with complete SHA-256 content identity. Provider execution SHALL read only snapshot bytes. Manifest paths, live filesystem paths, symlinks, reparse points, non-regular files, post-discovery mutations, or a non-empty identifier string SHALL NOT become Skill content authority. Skill metadata and provider arguments MAY narrow the host-authorized child tool set but SHALL NOT select or upgrade child policy, sandbox authority, approval authority, or MCP identity.

#### Scenario: Skill entry or reference escapes or changes
- **WHEN** a manifest uses traversal/absolute/non-canonical entry syntax, a package component is a symlink/non-regular file, a component changes during discovery, or live bytes change after discovery
- **THEN** discovery/execution fails closed or uses only the already completed immutable snapshot; no outside or replacement byte reaches a provider

#### Scenario: Skill requests write, shell, or MCP tools
- **WHEN** `allowedTools` contains tools outside the current host-authorized child scope
- **THEN** the effective set is the explicit intersection, an empty intersection rejects the child, and the Skill does not set `toolPolicy=inherit`

#### Scenario: Skill continuation uses changed content
- **WHEN** a continue/fork request resolves a package digest different from the durable source child run
- **THEN** source validation rejects the request before the child provider or any tool executes
