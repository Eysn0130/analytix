# Case Report Publication Specification

## Purpose
Define formal staged report publication and controlled full-value rendering
boundaries independently from ordinary privacy-safe funds answers.

## Requirements

### Requirement: Staged Report Publication Pipeline
A formal case report SHALL follow `staging -> claim validation -> PII projection -> render inspection -> atomic publish -> PublicationReceipt`. No stage SHALL publish case facts before the claim gate succeeds.

#### Scenario: Report request has verified claims
- **WHEN** a user requests a formal report and the claim ledger contains current verified claims
- **THEN** the system renders only those claims in staging, validates the final projection, inspects the render, atomically publishes it, and issues a publication receipt

#### Scenario: Claim validation fails
- **WHEN** any staged factual statement has missing, partial-as-complete, fake, mismatched, stale, or unsupported evidence
- **THEN** formal publication stops and no final report path is presented as delivered

### Requirement: Publication Receipt Authority
A formal report SHALL be considered published only when a host-issued `PublicationReceipt` binds the context digest, report hash, claim-ledger hash, dataset snapshot, PII projection, render inspection, publisher version, atomic target, and issued time in the authoritative registry.

#### Scenario: File exists without receipt
- **WHEN** a report file exists, opens successfully, or passes visual rendering but has no valid publication receipt
- **THEN** it remains an unverified staging artifact and cannot be described as a formal report

#### Scenario: Report bytes change after publication
- **WHEN** published file content no longer matches the receipt's report hash
- **THEN** publication verification fails closed and the artifact is not accepted for delivery

### Requirement: Write Report False Is Zero-Write
`write_report=false` SHALL perform no report, material-pack, manifest, staging-directory, image, or fallback file creation.

#### Scenario: Caller requests analysis without a report
- **WHEN** a report-capable tool receives `write_report=false`
- **THEN** it returns only the permitted in-memory typed outcome and creates no filesystem path

#### Scenario: Backend report service is unavailable
- **WHEN** `write_report=false` and a backend call fails
- **THEN** no local fallback or audit artifact writes report-like files

### Requirement: No Factual Report Fallback
Backend/report-renderer failure SHALL NOT activate a local template that invents, hard-codes, defaults, or broadens case facts. Fallback may only render an already verified claim ledger and SHALL remain staging until the normal publication transaction completes.

#### Scenario: Backend returns no report
- **WHEN** a report backend is unavailable or returns an invalid artifact
- **THEN** the host emits a report blocker or uses the same verified-claim renderer without adding facts, scenarios, amounts, or assumptions

#### Scenario: Required values are missing
- **WHEN** report input lacks an amount, count, entity, date, account, source, or coverage field
- **THEN** the renderer marks it unknown/omits the claim and never substitutes zero or a hard-coded case/scenario

### Requirement: Claim Ledger Is Per-Claim
Every factual report sentence/table cell SHALL bind to the exact verified claim and evidence set that supports it. The report pipeline SHALL NOT assign one bulk citation list to unrelated claims.

#### Scenario: Two report claims use different sources
- **WHEN** one claim concerns an amount and another concerns corporate ownership
- **THEN** each report element references only its own field-matched, source-capable receipt set

#### Scenario: Citation exists but supports another claim
- **WHEN** report validation finds a real receipt whose payload or scope does not match the rendered claim
- **THEN** the report fails validation or removes/narrows that claim before publication

### Requirement: Answer Card Completion Is Not Fact Authorization
Presentation completeness such as `answer_card_complete`, file existence, section count, or render success SHALL remain separate from `fact_answer_allowed` and publication authorization.

#### Scenario: Card has all required sections but unsupported facts
- **WHEN** a report/answer card is structurally complete while one or more claims lack valid receipts
- **THEN** structure may pass but fact authorization and publication fail

### Requirement: Report PII Projection
Every formal published report/artifact SHALL declare and hash its PII
projection. Claim records, provider proposals, generic accepted-final/history,
and generic report/renderer state SHALL carry typed stable entity and source-
field references rather than complete identifiers. Internal entity references
SHALL never be user-visible.

An in-app local report view MAY resolve Final-Gate-accepted typed slots through
`AcceptedSlotDisplay` to source-exact values for the installation-bound current
local session, current main-frame renderer principal, active case, and each
slot's immutable snapshot. `displayMode=full|masked` SHALL affect only that
ephemeral local projection; `full` SHALL be the active-case default. Local view
resolution SHALL NOT require formal staging, PublicationReceipt,
`PIIProjectionGrant`, a controlled artifact, or an external trusted sink and
SHALL NOT place the resolved value in report bytes or generic renderer state.

#### Scenario: Local report view includes an account
- **WHEN** a verified claim contains a stable account entity slot
- **THEN** the generic report model retains the typed binding and Electron Main
  may display the exact bound source field in `full` or a host mask in `masked`
  without exposing the internal reference or persisting the resolved value

#### Scenario: Controlled artifact authorization expires before publish
- **WHEN** staging used a full-PII authorization that is no longer valid at atomic publish
- **THEN** publication is rejected and the artifact is not delivered

### Requirement: Ordinary Funds UI Is Not Formal Report Publication
The packaged funds milestone SHALL deliver its verified flow-analysis result
through a PII-free accepted envelope plus `AcceptedSlotDisplay` in the current
local UI. It SHALL NOT require formal report staging, a PublicationReceipt, a
PIIProjectionGrant, a controlled artifact, or an external trusted sink merely
to display the source-exact local answer. Conversely, a local accepted answer
or Direct Source Preview SHALL NOT be relabelled as a formal published report
or external artifact.

#### Scenario: Packaged funds answer passes the Final Gate
- **WHEN** the exact flow-summary claims and evidence rows are accepted for
  typed local UI rendering
- **THEN** Milestone B may complete without creating a report file, while all
  formal report and external-artifact rows retain their independent status

### Requirement: Funds Cannot Invoke Ordinary DOCX Publication
The funds plugin SHALL NOT import, advertise, or directly invoke the Desktop
Write DOCX producer. A funds-authored factual DOCX SHALL enter the Host-owned
artifact builder only from the report publication use case after every factual
claim has passed Final Gate and the exact publication authority, target,
snapshot, and PII projection remain current. Ordinary Write's masked export is
not a PublicationReceipt and SHALL NOT be promoted into this controlled path.

#### Scenario: Funds plugin requests DOCX directly
- **WHEN** the funds plugin, model, MCP server, renderer, or an ordinary masked
  Write request attempts to invoke DOCX as a factual funds report
- **THEN** the host rejects the controlled publication, creates no factual
  report file or PublicationReceipt, and leaves ordinary Write and local typed
  funds display available

#### Scenario: Controlled funds report reaches the artifact builder
- **WHEN** an independently implemented report use case presents a
  Final-Gate-accepted typed artifact model plus current publication authority
- **THEN** the Host may render DOCX through the same local OOXML primitives only
  inside staging and atomic publication, while the plugin never receives a
  filesystem path, writer, complete source-exact bytes, or publication token

### Requirement: External Account Artifacts Use The Explicit Typed Effect Path
A copy, save/export, print, drag/drop, external-application, Connector, email,
upload, or share action SHALL derive only from an explicit user action bound to
the exact target, case, immutable snapshot, and fields. The user click SHALL be
sufficient runtime authorization for that exact effect; no third-party,
administrator, or per-field approval SHALL be required. If the effect publishes
an Agent-authored factual artifact, its claims and EvidenceReceipts SHALL first
pass Final Gate. A provider-bound Connector SHALL remain PII-free, and any
source-exact external effect SHALL be resolved by Main/Host rather than a model,
MCP public result, generic renderer payload, or browser Blob.

Where a formal export, external-sharing workflow, or optional high-security
viewer already requires `PIIProjectionGrant`, controlled-artifact/private-CAS,
publication verification, retention, or a one-use trusted sink, the host SHALL
reuse that existing chain. Those optional external controls SHALL NOT gate
Direct Source Preview, AcceptedSlotDisplay, or the in-app report view, and the
first-stage milestones SHALL NOT introduce another receipt, registry, CAS,
staging, sink, or protocol version to express the same effect.

The accepted case answer SHALL contain typed claim/entity/source-field slots
rather than a generic string with complete PII. The host SHALL bind each slot
to the exact source field and claim/evidence graph before a typed effect
resolves it. Regex/string/Markdown replacement SHALL NOT reconstruct PII.
Generic approval/publication/access/status projections SHALL contain only
closed status, hashes, counts, and bounded PII-free audit metadata.

#### Scenario: User explicitly exports selected account fields
- **WHEN** the user selects an exact target and account fields and every
  applicable case/snapshot/claim/receipt/publication binding succeeds
- **THEN** Main/Host releases only those source fields through the typed effect
  path, no internal reference is visible, and generic channels expose only
  bounded status

#### Scenario: External export is unavailable
- **WHEN** any required condition is missing, stale, mismatched, revoked, or
  externally unverified
- **THEN** the action releases zero complete identifier bytes, does not fall
  back to a generic viewer/download/renderer blob, and remains an independent
  blocked or unverified row without blocking standard local display

#### Scenario: External target expires after Final Gate
- **WHEN** typed claims remain accepted but the external target/action or an
  applicable formal-artifact authority expires before the effect
- **THEN** no exact value is released, the external effect/view requests
  reauthorization without displaying an internal reference, and standard local
  results remain available

#### Scenario: Complete value cannot be reconstructed by string replacement
- **WHEN** a provider or renderer returns prose containing an entity-like
  token or masked value outside a typed answer slot
- **THEN** the host does not substitute source-exact PII and neither local
  display nor an external effect is reconstructed from that string

### Requirement: Atomic And Auditable Publish
Formal publication SHALL use atomic file replacement or an equivalent transaction, preserve staging/failure audit metadata without leaking restricted content, and never expose a partial final file.

#### Scenario: Render or filesystem fails mid-publish
- **WHEN** inspection, hashing, fsync, rename, or target verification fails
- **THEN** no partial final artifact is accepted and the failure routes through the final evidence gate

#### Scenario: Publish succeeds
- **WHEN** the final bytes, claims, snapshot, PII projection, and target all verify
- **THEN** the file and publication receipt become visible as one accepted delivery state

### Requirement: Report Generation Requires Appropriate Approval
Unattended `approvalPolicy=never` SHALL NOT authorize report/file publication or another write-capable case tool. Such calls SHALL fail closed unless a separate explicit pre-authorized workflow contract covers the exact artifact action.

#### Scenario: Model calls report tool under approval never
- **WHEN** a high-risk turn requests a write-capable report tool with `approvalPolicy=never` and no explicit artifact authorization
- **THEN** no report tool/file write executes and the answer states the publication boundary
