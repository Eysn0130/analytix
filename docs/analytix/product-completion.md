# Analytix product completion

Status: Operational current matrix, 2026-09-24 PDT. Accepted targets remain in the
[spec registry](specs/README.md) and scoped OpenSpec requirements. This matrix does
not shrink the original A–M/P0–P5 outcome or declare product/release acceptance.
Delivery order is owned by the [Operational execution plan](delivery-execution-plan.md).
Resume from the [canonical handover](handovers/README.md), preserving newer local
history before comparing remote state.

## Current PR28 Core admission — 2026-09-24 PDT

The latest product-code SOURCE on
`codex/workbench-product-delivery-20260914` is
`f4cf3ef14ab6bd8375830310ea8c5220e46793b2`. Its private Core
app/DMG/ZIP and installed DMG copy pass container verification, strict ad hoc
signature verification, independent production-tag Core Go inspection and the
exact artifact legal audit (1,172 dependency instances; zero mandatory
engineering blockers). The package remains
`development_clean_non_publishable`. Exact `f4cf3ef14` Development CI passed
51/51 jobs and PR28 showed 56/56 successful checks.

The same installed candidate passes a 19/19 continuous synthetic-Provider
journey, focused cancellation and a two-page PDF GUI preview. A local synthetic
Provider produced 120/120 unique completed turns, and the installed GUI showed
the beginning, middle and end of that history. Post-120 GUI continuation failed
with `provider_error` after protected credential unavailability following a Go
restart; new Main-process recovery was not observed. In a separate upgrade
profile, the old installed Core wrote and read a thread, but the new app blocked
in macOS Keychain access before its data readback. Normal protected real
Provider recovery was not run on this candidate. See the
[current f4 installed checkpoint](qa/pr28-f4-installed-continuation-2026-09-24.md)
for exact receipts, limits and next observations. The
[14ab closure repair checkpoint](qa/pr28-closure-repair-continuation-2026-09-24.md)
and [artifact checkpoint](qa/pr28-installed-admission-followup-2026-09-24.md)
remain historical for their own source and build.

| Exit | Current status and scope |
| --- | --- |
| SourceReady | `true` for exact `f4cf3ef14` source checks. A later documentation HEAD must receive its own accurate check before transferring this status. |
| PrivateCandidateReady | `false`: normal protected Provider recovery, post-120 GUI continuation/new Main recovery, and old→new Core data readback/continuation remain open. PDF and the continuous synthetic journey pass only in their recorded scope. |
| MergeReady | `false`: PR28 remains Draft and installed acceptance is incomplete. GitHub's `MERGEABLE` response is not this product gate. |
| PublicMacReleaseReady | `false`: the private artifact is nonpublishable. Developer ID, notarization and publication authority are future public-distribution conditions, not current private QA repairs. |

The f4 private DMG/ZIP are inspectable QA candidates, not a public beta or
formal release. Do not repeat the retired 21/2 package-license, historical
`NOT_CLEARED` or prior-head CI tasks as current blockers.

## Historical checkpoints retained below

Preserved SOURCE is `0708620e21fc2bd706ba18a377a60824cdb78421`; the retained
DELIVERY is `9c21c0ec911571ddba1c733ac4a828737654e02b`. The execution plan was
integrated at `b485a0357`. Formal Core qualification software and its signing,
Go inspection, formal-evidence, publisher and updater consumers are implemented
at `69e74fdb6` and `0ce2e92a0`; exact legal inventory corrections follow at
`c2ef61c4b`, `5a7d2cebc` and `ac00f1602`. Actual formally qualified artifact and installed acceptance remain
unfulfilled. The private96e55 Core DMG retains its original classification and
bytes; it is not a package of these new sources.

Production/QA credential-path audit confirms no ordinary-user second password.
A subsequent source repair defers onboarding selection until protected validation
and settings save, checks stored-credential availability on restart, and retains
configured state on temporary unavailability; focused checks pass. Installed 8e
GUI secure Save, stored-credential DeepSeek Flash response, new-process restart
without API-key re-entry and history/response recovery pass. The initial composer
kept its prior model until manually selected; a later onboarding handoff fix
and a Settings Registry-unavailable/retry repair pass 107 focused cases and web
typecheck, pending a new candidate build. The installed locked-restart check kept
configuration and recovered through Retry after normal QA Keychain unlock.
See the current release-admission report.

The subsequent ordinary-development credential slice reuses one persistent
protected Registry/Secret Store, independently of private task profiles. Explicit
QA Keychains remain opt-in and packaged runtimes reject the development authority.
Actual Core bootstrap, independent-process reuse, GUI response, Electron/Core
restart response and new-task GUI response pass without API-key or password
re-entry. A reproduced renderer defect replacing the Registry model with stale
Settings was repaired. These are source-development results, not a new package.
The same report records exact tests and remaining replacement/signing limits.

Historical artifact SOURCE at this checkpoint was `b2cf1179e7b6e23bf565f4de94e8f1e4715a2c94`;
prior SOURCE `55838f047fbdccd2f8e073b157815d896c084d5d` and DELIVERY
`d46d10b389285ce2b3e8a0a4d7ff68da7e75b60b` remain ancestors. A clean new
Core1.0.6 private app/DMG/ZIP completed the normal lifecycle. All8,639 app files
and links match the ZIP, DMG and isolated installed copy; strict signature and
independent production-tag Go inspection pass. The actual1,174-package scan
leaves two package blockers (Canvas embedded provenance and lazy-val exact text):
of the original31 plus6 rows,34 pass,2 block and1 is absent. Embedded native,
WASM and font obligations remain separate. See the
[current admission evidence](qa/pr28-core-release-admission-2026-09-22.md).
The candidate remains `development_clean_non_publishable`. A later clean 8e5142250
app-only candidate also passes strict signatures and independent installed Go
inspection (2.36s), with the same two actual package blockers; its 1,705 font/PDF.js
WASM bytes are bound to the verified archives. No later DMG/ZIP is claimed.

Current-scope source egress was reassessed through the original interface and
normal pushes reached `8e5142250`. Its four CodeQL analyses and gate pass;
Development CI run35805133381 passed all51 jobs. Later ff298 CodeQL also passed,
but its Rust statistics CLI hit a DuckDB internal error in redundant count-page
execution. The subsequent existing-owner repair records actual result rows;
all95 local CLI regressions pass. Exact successor Linux CI remains pending. The earlier b2cf Development CI
passed all51 jobs and is not a substitute for later HEAD checks. PR28 remains
Draft/BLOCKED pending review and applicable acceptance. Main remains
`ce96cf12581acfa0e19fae7c6aa9c709371012c8`. The original locked QA profile remains
preserved after authentication failure. A separate fresh isolated 8e session
completed normal protected setup and GUI Provider/restart checks, including
credential-unavailable refusal while locked followed by successful unlock/retry.
These GUI results are separate from the earlier standalone ten-token HTTPS probe.

Multi-snapshot SOURCE `dc059977ae0ae3b4986698eda82c25bfe9a54275` preserves original A1/A2 bindings, applies
current display permission separately, and retains whole-thread atomicity.
Dataset order/concurrency/restoration, case A→B→A epoch checks and post-witness
revocation/cancellation checks pass at their source owners. The final production-tag
native chain passes1776.55s: actual public GET200 and original A1/A2 digests,
A1 historical/A2 current labels, missing/corrupt503 with no partial batch,
exact-byte restoration, ordinary Provider continuity, immutable original stores,
and fresh OS process recovery/protected local display. Existing accepted-final and
compaction crash matrices also pass with production tags/race in124.842s. The earlier500 failure and
all prior failed runs remain recorded. This is synthetic Host/loopback composition,
not installed or external Provider acceptance.
The terminal harness now waits at most180seconds after two observed90-second
harness timeouts with successful completion during cleanup; HTTP remains45seconds.
These test budgets are not product latency acceptance claims.

The [preserved recovery evidence](qa/pr28-release-closure-2026-09-22.md) records
exact source/artifact identities, failures, commands, checks and remaining seams.
The [previous Core and single-snapshot evidence](qa/pr28-independent-review-execution-2026-09-21.md#current-core-release--funds-recovery-from-5fe1)
remains historical and preserved. No ordinary source/fixture check is installed
GUI, normal Provider, Developer ID, notarization or public-release proof.

One main conversation must support generation/import → native preview → selection
and explicit action → annotation/reference → real Diff → explicit acceptance →
CAS/journal/receipt/undo → save/reopen/recovery. Passive selection/quote never sends;
drafts, attachments and IME remain protected. Go Core retains sole authority.

## Accepted delivery order — 2026-09-21

The user-approved Core/Funds continuation changes delivery order and stage scope,
not the original complete-product denominator or historical results. No stage is
currently accepted. The existing source branch remains the sole writer.

| Stage | Included outcome | Excluded from this stage's feature completion, retained in full product | Current admission |
| --- | --- | --- | --- |
| CORE_STAGE | Ordinary project/thread, Provider selection, controlled tools and approvals, cancellation/terminal, persistence/restart, file/terminal policy and first-party Host; usable without Funds | Funds account flow and advanced Office features are NOT_INCLUDED_IN_THIS_STAGE targets; their currently reachable code and shipped assets still require safety, isolation and license checks | partial: newb2cf1179e private Core app/DMG/ZIP and exact source/seal/container/installed-copy checks pass; actual package scan leaves2 blockers, with embedded asset admission separate. Controlled Core software qualification remains implemented. Complete resource licenses, Developer ID/publication authority, installed GUI/normal Provider/long-history/update-data journeys and stage admission remain outstanding |
| FUNDS_ACCOUNT_FLOW_STAGE | The same Core plus admitted first-party account-flow import, immutable DuckDB snapshot, exact facts, model-safe Provider request, Final Gate, protected local display and recovery/revocation | Wider investigation/report/export, arbitrary remote archives and advanced Office remain in COMPLETE_PRODUCT | partial: original single-snapshot/new-Go-process evidence retained; current A1/A2 public/fault/new-process chain passes at its source boundary. Exact aac3 native components are reused only at their verified unchanged source boundary. Synthetic Host/loopback evidence does not qualify installed GUI, external Provider or all case/revocation journeys |
| COMPLETE_PRODUCT | Original P0–P5, Office/Browser/Canvas/images, imported pivot/chart, archive lifecycle and broader accepted Funds scope | Nothing is silently removed | original matrix below remains applicable |

Core installation and stage release do not depend on unshipped Funds features.
An artifact that exposes Funds must additionally pass the Funds journey. Merely
hiding controls does not exclude code, catalog, restored state or package assets.
Existing A0/B1 and numerical/platform acceptance requirements retain their own
scope: B1 sensitive workflow applies to a Funds-bearing artifact; complete-product
formal evidence remains outstanding. No test threshold or mandatory CI is reduced.
The continuation worklist C01–C07 maps to runtimeapp, Provider Registry, execution,
Host and recovery owners; F01–F07 maps to existing import/snapshot/host/projection/
evidence/display owners; P01–P03 and D01–D04 remain packaging and delivery gates.

## Current capability and evidence matrix

| Accepted scope | As built / owner | Current gap and next evidence | Gate |
| --- | --- | --- | --- |
| P0 identity, authority and security | Core Registry, privacy projection, numeric boundary fixes and WorkspaceStatus metadata containment; 67 legacy finding mappings retained | Complete current caller review and accurate-candidate CodeQL; mappings are not dismissals | SOURCE / CI / security review |
| P1 DOCX local editing | `src/main/office/surface/office-worker.js`: captured native range, structure guard, new body/table script-size guard; Main/Core review/CAS owner retained | Original default-font failure is refused before mutation; two fixed-engine explicit/inherited-size controls edit/export/reopen successfully. Full non-target structure/visual fidelity including headers/footnotes and installed Core journey remain unaccepted | SOURCE partial / fixed engine partial / native not run |
| P2 image collections | Original image/canvas owner: up to8 regions, stable IDs, original-pixel geometry, notes, shared budget, v2 read/v3 CAS, display rotation/zoom; three race fixes retained | Actual multiple-region GUI/save/conflict/reopen journey; trusted pixel/metadata projector remains absent for non-generate editing | SOURCE implemented in recorded scope / native not run |
| P2 Browser and PDF | Main guest/document lease, isolated selection/Range, opaque Core reference/currentness, explicit same-thread Explain; existing PDF remains | Child-frame capture unsupported; complete actual guest/Main/Core/installed lifecycle still unverified. URL/text/hash alone never establishes authority | SOURCE partial / fixed Electron partial / installed not run |
| P3 XLSX/PPT | Real pivot generation, typed cells/formulas and bounded PPT text/geometry/fill; existing Core objectediting and fixed worker | Imported pivot/chart edit/refresh stable relationship/object/field binding is still SOURCE_GAP; shared-cache preservation, original/no-op/edited/reopen and true Diff required | SOURCE incomplete / native incomplete |
| P4 Skills lifecycle | Existing `pluginpackagehost` and `pluginmaterializationfs`: signed materialization, enable/disable; Invoke and actual adapter admission reject activation/generation changes across readiness, pre-effect and completion, including real-store disable-enable/reopen | Remote archive publisher/registry trust, stage/atomic upgrade/revoke/uninstall/restart chain remains SOURCE_GAP. FormalPackageBinding is not archive authority; desktop install still fails closed. Source tasks remain unfinished, not all blocked on GUI | SOURCE incomplete / installed not run |
| P4 one conversation / cleanup | Primary conversation, bounded retrieval8 caches/4 builds, completion cancellation and retained history; old three Write routes removed | H01–H08 end-to-end authority/compaction/recovery evidence remains partial. Do not restore second runtime/RAG/Provider or alter independent UI Refresh | SOURCE retained / integration partial |
| P0 Rust correctness | Fixed3000/10000 ascending/13000 fixture, static stage labels and prior bounded query controls retained | Old main INTERNAL root cause unresolved. COUNT-wrapper source lead differs from target; no new arbitrary reruns or alternative-binary symbols | EVIDENCE_FAILURE / root cause open |
| P5 desktop storage/install | b2cf container/copy checks retained; later 8e app-only installed signature and Go inspection pass; fresh protected Save, stored-credential response and restart/history recovery pass | Original locked profile retained; later first-run composer-model handoff repair is source-verified and awaits a new build. Full installed tools, long history and upgrade/failure recovery remain outstanding | partial / bounded Provider journey passes |
| P5 integration and release | Original branch/history and single writer retained; 8e5142250 CodeQL analyses and gate pass | Current-scope egress reassessment and normal push pass; 8e Development CI passed all51 jobs. Exact-HEAD review, applicable extra approval, actual main, native, resource licenses/SBOM/fonts, signing/notarization and release gate remain | All five exits pending |

## Evidence limits and release exits

The previous fixed-engine matrix remains4 cases/22 assertions/14pass/8 typography
failures, bound to its candidate. New format refusal prevents the known loss; it
does not convert those8 into successful edits. New controls preserve every measured
style property; only adjacent runs with identical properties are merged for semantic
comparison. A positive control does not replace the immutable original fixture.

Review-tool30/64 selftests,32 synthetic specifications, product unit tests, fixed
engine probes, installed GUI and real Provider checks are distinct populations.
Overlapping focused groups are not summed. No complete-product percentage is claimed.

The old private DMG SHA256 `200a8772dbfe36fc1212684a7b1bb0d183005b0f03a72892d6ba040c18f63338`
is470395220bytes, `development_dirty_non_publishable`, ad-hoc and NOT_INSTALLED;
source2f25 is not that artifact. It is not a clean-main release candidate or notarized.

MERGED_MAIN: no. VERIFIED_MAIN: no. COMPLETE_PRODUCT: no. FORMAL_RC: no.
PUBLIC_RELEASE: no. The latest conditional final-publication goal remains accepted,
subject to all applicable source/refusal, resource, installation and release gates.
No ordinary branch push or merge can substitute for clearing the specific source
refusal. Current local source has no corresponding remote CI; old dde/main checks
must keep their own identities. Documentation or a clean checkout grants no release
qualification and never transfers the writer.

## Historical entry anchors

The following links preserve prior entry URLs; their targets are dated evidence, not current next actions.

- [Current capability and evidence matrix](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-16)

<a id="historical-capability-matrix-before-this-execution"></a>

- [Historical capability matrix before this execution](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-93)

<a id="completion-recheck-and-bounded-retrieval--2026-09-20-2006-pdt"></a>

- [Completion recheck and bounded retrieval — 2026-09-20 20:06 PDT](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-121)

<a id="previous-focused-local-changes-and-evidence--candidate-33510c813"></a>

- [Previous focused local changes and evidence — candidate 33510c813](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-176)

<a id="historical-capability-matrix--c7-takeover-2026-09-15"></a>

- [Historical capability matrix — c7 takeover, 2026-09-15](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-222)

<a id="three-format-generation-integration"></a>

- [Three-format generation integration](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-329)

<a id="native-review-and-recovery-integration"></a>

- [Native review and recovery integration](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-374)

<a id="annotation-note-persistence-and-safe-exit"></a>

- [Annotation note persistence and safe exit](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-426)

<a id="qualified-private-local-installation-candidate"></a>

- [Qualified private-local installation candidate](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-451)

<a id="private-installation-probe-2026-09-15"></a>

- [Private installation probe, 2026-09-15](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-487)

<a id="completion-evidence"></a>

- [Completion evidence](handovers/2026-09-16-pr28-continuation.md#snapshot-d864-product-525)
