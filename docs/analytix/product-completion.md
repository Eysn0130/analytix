# Analytix product completion

Status: Operational. This is the current capability and evidence matrix for the
user-authorized 2026-09-15 product delivery instruction. It does not declare
product acceptance, change licenses, or authorize public release. The accepted
outcome includes a lawful installed macOS ARM64 application, not only source.
Historical QA remains valid only for its recorded candidate and environment.

One main conversation must support generation → native preview → selection and
explicit quick actions → annotation → proposal → review → controlled application
→ reliable reopen/recovery. Passive selection and quoting never send a task;
explicit task actions preserve the existing composer draft and attachments.
The Go runtime remains the sole production agent and authority. Plugins and
format adapters cannot replace permission, privacy, Provider or persistence policy.

## Current capability and evidence matrix

Baseline: PR 28, `c7e66a7ba427cdd01eef1fe91aacd96ee18328c7`, on
`codex/workbench-product-delivery-20260914`. At takeover, Development gate passed,
but the independent CodeQL check failed with 12 path-injection alerts. Individual
successful language-analysis jobs do not close that check. Local changes listed
below are not yet installed-application evidence.

| Required outcome | As built / current work | Remaining evidence or implementation |
| --- | --- | --- |
| Unified workspace | Main conversation, object/tool tabs and separate terminal layout exist | Full keyboard, IME, narrow/wide, theme, scale and native geometry journey |
| Explicit native quick task | Same-thread callback carries its own frozen reference; shared action strip and owner-native context/keyboard menu, with preview-to-proposal dispatch | Real Provider/GUI journey and selection-positioned surface interaction |
| Quote-only send | Native references count toward composer send eligibility; quoting does not dispatch | Real Enter/button/IME journey and full action-entry integration |
| Annotation lifecycle | Per-object/thread notes persist through Core CAS; Main retains pending input and uses a renderer freeze acknowledgement before close/quit flush | Complete display anchors and multiple annotations; real normal-restart, capacity and closed-tab GUI journey |
| Native proposal refresh | Single-flight polling with failure backoff; new events supersede pending reads | Tool-completion refresh integration and installed behavior |
| DOCX | Core absent-only generation calls a data-only `docx` codec; checkpoint binds the created bytes and installation principal; opaque artifacts resolve into the current native workspace | Full headers/fields/links coverage, installed repeated-text positive case and representative rendering |
| XLSX | Go data-only generation writes typed cells, bounded checked formulas, formatting and native charts; sheet/chart IDs persist in OOXML; unsafe numeric/formula-to-text mutation remains rejected | Native recalculation/rendering, typed modification/range operations and summaries/pivots |
| PPTX | Structured generation writes text, shapes, native charts and validated images; stable slide/object IDs; native preview and bounded shape selection | Targeted style/chart edits, native ID roundtrip and per-slide visual quality |
| Canvas / images | AtlasFlow and existing local media assets remain reusable | Stable scene identity, fact/layout separation, selection/notes/local edits, versioned exports; real media Provider integration |
| Plugin Skills | All three Office 0.2.0 packages declare original workflows, fixed generation capabilities and installed snapshots; discovery and generation follow their signed activation | Canvas contribution, remaining dependency handlers and installed end-to-end evidence |
| Write migration | Existing MD/TXT editors, exports and same-thread document surface retained | Map all custom actions, presets, retrieval, autosave, conflicts, review and export to the shared product flow |
| Browser / files / PDF / knowledge | Existing surfaces remain | Shared authorized object opening and typed annotation anchors; no implicit indexing or uploading |
| Apply / recovery / Diff | Core-owned originals, exact save candidates and approved changes; thread-bound discovery, protected-local Diff, explicit interrupted-save continuation, restart undo and cancellation of proven unsubmitted changes | Native format fidelity and actual third-object/restart GUI journey |
| Installation | Source `8365b16ce` produced a private macOS ARM64 DMG; container/layout/signature checks and isolated installation signature checks passed | First launch blocked before application initialization by Chromium default-Keychain authorization; installed end-to-end journey and separate public redistribution obligations remain |
| Security / delivery | Existing macro prohibition, privacy authority and protected profiles preserved; controlled object reads/commits reject hard-link aliases, including post-inspection drift | Exact-candidate CodeQL, applicable CI/review and remaining required negative cases |

The numeric/formula text-path restriction is interim damage prevention. It is not
acceptance of a text-only spreadsheet product. Persisted note text alone is not
a complete position-aware annotation system. Source tests are not native engine fidelity,
live media generation, or installed GUI evidence.

The initial selection repair was checked with six focused Vitest files (86 tests)
and `tsc --noEmit` for both `tsconfig.web.json` and `tsconfig.node.json` on the
configured macOS host, using `scripts/use-analytix-cache.sh`. The quick-dispatch
and note-retention regressions failed before their fixes; numeric/formula coercion
also failed for both native types before the worker guard. Deferred-response tests
cover version change, superseding events, collapse/remount polling and scope
revocation after changing tabs. This is synthetic source-level evidence only.

The native menu checks cover owner/main-frame validation, target-version changes,
opening without dispatch, selection changes while open, existing non-ASCII custom
action IDs, and one-click preview-to-proposal capture. The renderer suite passed
27 tests separately; Main IPC/surface suites passed 27 tests. Mixed-environment
runs encountered a Vitest worker-start timeout, not a passing combined run.
Hard-link regressions failed before the repair and passed after integration;
ordinary text-tool hard-link compatibility remains covered. Independent review
accepted these bounded source changes. Cross-compilation is not Windows runtime
evidence, and no CodeQL closure is claimed from the local filesystem tests.

The DOCX generation candidate uses a fixed host-supplied codec executable and
entry, bounded stdin/stdout and no inherited credential environment. The codec
receives content and explicit image bytes, never the target workspace path.
Core validates the OOXML package, creates only an absent target and settles the
checkpoint before issuing typed artifact metadata. A configured bundle is still
required; this is not a claim of packaged admission. Hosted lifecycle source checks
are recorded separately below.

Generation tool arguments retain the existing execution-grant limit of 4 MiB
JSON and 1 MiB per UTF-8 string. Generation-specific operation records now support
that limit through protected CAS writes, readback and restart validation. Ordinary
text snapshot and mutation limits remain unchanged. The codec's larger private
asset limit does not expand the model tool argument limit. Large source images
still need a Core-authorized asset-reference path.

Artifact opening validates the current installation principal, conversation,
workspace, root identity, file links, content hash and package structure. New
live receipts open the native object through the same resolver as the artifact
card; replaying an existing receipt does not automatically reopen it. Switching
conversation/workspace or a failed pending text save cancels the open. The public
receipt contains no file path, raw document content or evidence authority.

Current checks include 245 tests across eight focused Vitest files (including
51 DOCX codec/builder tests), both TypeScript configurations, a real
Go-to-bundled-codec integration producing and inspecting Chinese headings/lists/
tables, and focused Go generation/checkpoint/authority tests.
Independent review found two artifact timestamp/digest privacy seams, at durable
append and subsequent public projection. Both now reuse one strict closed-host
metadata predicate. The regression exercises real settlement, durable append,
HTTP SSE replay and replay after reopening the store; ordinary PII, malformed
lookalikes and other tools retain their existing rejection/projection behavior.
The production-tag Go runtime also compiles, and the repository's Electron Main
build emits the fixed codec entry and its chunks into a task-owned cache layout.
The Go integration also passes against that emitted JavaScript entry, producing
and inspecting a real DOCX through the production codec process boundary.
That layout uses the current checkout's dependency tree and is not an independent
installation or a distribution artifact.
No native GUI rendering or installed journey has been established by these checks.

Binary generation settlement and startup reconciliation are distinct from
checkpoint deletion/restore. The existing generic checkpoint rescue is text-only;
generated Office creation therefore reports `manual_review`, not a false
`ready/delete_created_file`. This does not disable native modification proposals
or native undo. Native edit/undo persistence remains an active required outcome.

Documents lifecycle checks use the actual repository package, a task-isolated
installation authority and installed copy. They pass for fixed-byte reading,
source/installed separation, installed tampering rejection, current discovery,
inline instruction loading, generation preparation, disable/re-enable rejection
and Host restart with the same snapshot. Host, catalog and side-effect preparation
packages pass their tests; independent source review found no blocker in this
bounded integration. The Skill validator and diff check pass. These are real
materialization fixtures, not an installed desktop GUI journey.

## Three-format generation integration

The shared `generate_office_document` request now discriminates DOCX Markdown,
XLSX typed workbook data and PPTX structured slides. The model-facing schema only
advertises a kind while its exact installed Skill is enabled and its format
writer is available. A project Skill cannot impersonate any of the three fixed
Office namespaces. Host activation, source identity and Skill digest remain part
of the prepared operation; all formats use the same Core create/checkpoint,
opaque receipt and protected-local opening authority.

XLSX uses pinned Excelize in Go without an additional Python or Node dependency.
It keeps literal `=` strings as text, finite numbers and booleans typed, and
formula expressions intact. The finite formula subset has bounded references,
dependency depth, cycle checks and real calculator checks. SUMIF requires static,
equally shaped ranges to prevent unvalidated implicit expansion. Sheet identity
uses Unicode simple folding consistently with the writer. Charts reference an
existing text-label cell and bounded category/value vectors on that sheet.
Formulas request native recalculation; persisted typed caches are not fabricated.

PPTX uses pinned PptxGenJS through the same fixed data-only process entry as DOCX.
Coordinates are bounded to a 16:9 page; IDs are globally unique. Object names use
the writer's public API; each validated slide ID is written into its standard
`p:cSld/@name` attribute in newly generated bytes. Images are decoded and checked,
including repeated-embedding budgets. No URL, template path or file API is
available in the admitted input. A missing optional title remains valid.

Current integration checks pass for three-format current discovery, inline
instruction loading, prepared generation and disable/re-enable behavior, and
for artifact settlement, durable append and SSE replay before/after store reopen.
The emitted Electron Main entry and Go composite produce actual DOCX, XLSX and
PPTX from the same synthetic 100/200/300 data and pass Core OOXML inspection;
this also verifies XLSX generation without Node. Format/builder tests pass
101/101, artifact IPC/opening tests 16/16, both TypeScript configurations pass,
and both new Skill validators pass. The production-tag Go runtime builds.
These are bounded source/runtime checks, not installed GUI evidence.

The format dependencies and exact notice texts are recorded in
[office-generation-dependencies.md](office-generation-dependencies.md). They do
not establish native engine/font admission, packaged dependency presence or GUI
acceptance. Independent review found and fixed the missing-title wrapper,
noncanonical PPTX field names, SUMIF range expansion and Unicode sheet collision.
The SUMIF regression was observed failing before the fix. The corresponding
formula/alias calculator checks and format tests pass; visual/native roundtrip
and complete installed product acceptance remain required.

## Native review and recovery integration

Core captures original bytes before releasing an approved replacement. Private
records bind the installation principal's object identity, conversation, proposal,
revision and fixed save/undo operations. Fresh sessions can discover the current
change without Main remembering an operation ID. Both a reopened commit and undo
participate in the shared managed-file capture and binary CAS. Local review shows
restored original values separately from model-facing protected parts.

Prepare reservations and a bounded retiring list make interrupted metadata/large
blob cleanup discoverable. Superseded/cancelled changes retain compact replay
records; large originals are emptied through precise CAS after retirement.
Unresolved undo originals are retained. Cancellation applies only to confirmed
unsubmitted changes; existing save journals, including ambiguous conflicts, are
not treated as proof that no write occurred. A durable undo intent with no undo
journal can be explicitly continued after restart.

Fresh verification passes for both Go application packages and the focused
filestore Office/object-editing/recovery suite. A real three-format file fixture
passes approval, save, fresh Store/Service/Adapter discovery, undo, replay and
external-version rejection, with managed capture asserted during each CAS.
Main/controller/IPC/contracts pass 64 tests; the renderer's 30 tests cover local
Diff, missing-review refusal and thread isolation. Both TypeScript checks pass;
focused ESLint has zero errors and three existing effect-dependency warnings.
Independent storage review found no additional concrete defect in that candidate.

Core now retains the exact exported candidate, bounded to 16 MiB, before the first
save journal. A fresh session can explicitly continue that approved change using
only its identity and original revision. Core checks private file identity, hash,
OOXML kind, thread ownership and current original bytes before the same-operation
CAS. A completed journal remains query-only. Ordinary commit retries and status
queries never resume a pending write, including when the journal is absent.
Legacy records without the retained candidate cannot gain resume permission from
caller-supplied bytes. Retired originals and candidates are emptied through CAS.

The desktop exposes this continuation separately from result checking. A failed
query refreshes the Core recovery capabilities without writing; a new explicit
click is required to continue. After a lost reply, fixed-operation result checking
and exact current-byte validation precede native reload. This preserves uncertainty
without permanently locking the Main controller after a pre-journal interruption.
Native GUI/format fidelity and installed recovery remain unverified by these
source/fixture checks. Note persistence is covered separately below.

The explicit-continuation candidate passes the two Go application suites and
the focused filestore native recovery/resume and object-editing tests (8.900s).
The three desktop/controller/contract files pass 75 tests, including query-only
capability refresh, fresh-controller continuation and lost replies. Both
TypeScript configurations pass. Independent review caught and verified the fix
for ordinary replay with a missing journal; that path now returns unknown without
creating a journal or candidate. These checks use synthetic files on the configured
macOS host and do not establish installed recovery or native formatting quality.

## Annotation note persistence and safe exit

Core stores one note draft per object identity and conversation with a revision
CAS, protected file permissions and exact failed-request replay. Main retains
new input synchronously while an earlier save is pending; a stale acknowledgement
cannot replace later text. Normal object close, capacity eviction, thread change
and application exit flush pending notes. A failed save keeps the note and offers
retry. Restored text and its historical source revision do not recreate an old
selection token or grant modification authority.

A renderer/preload acknowledgement freezes note input before the final close or
quit flush. Independent holds prevent concurrent file close and application quit
from unfreezing each other. A missing acknowledgement leaves input state unknown:
legal notes remain receivable, while the next close must obtain a fresh freeze
acknowledgement. Core and PTYs stop only after exit is accepted; cancelling exit
or hiding to the tray preserves the running session.

Focused Main, IPC, actual before-quit callback, preload and renderer tests pass
75/75. Core annotation/recovery filestore and the three relevant application
packages pass. The old-acknowledgement overwrite regression failed before repair.
Independent read-only review accepted the bounded close/quit corrections. These
checks establish note persistence and shutdown ordering in source/fixtures;
multiple location-aware annotations, installed IME behavior and normal-restart
GUI acceptance remain required.

## Qualified private-local installation candidate

A separate build entry stages the pinned engine, surface/preload, Office packages
and notices before the existing packaged authority and resource seal. Core derives
the directory from its real executable inspection, refuses insecure private
composition and never falls back to development roots when package verification
fails. Current resource identity is checked before and after materialization and
Host operations. The server retains the concrete adapter pointers for binding
selection/privacy/capture; the Host wraps those same instances for qualification.

Main uses a nonce-bound, bearer-protected local-display query unavailable to the
Renderer. A private internal token permits loading only the fixed qualified
resource directory. The metadata digest and held bytes must agree with Core,
including the native preload. Normal source startup retains its original gate.
Note input is handed to an existing Main controller synchronously so a new
qualification query cannot delay the last keystroke past a close acknowledgement.

The independent codec bundle includes its JavaScript dependencies and leaves only
Node builtins external. Its exact two-file directory is unpacked and checked before
packaged authority generation. Go through a real Electron Helper successfully
generates and inspects all three formats from an isolated copy; the bundle does
not resolve the repository dependency tree. This proves that bounded execution
chain, not an actual installed application or native visual result.

Current checks: 99 focused Main/native/admission/annotation/quit tests and all
56 process-launch tests pass. Fourteen qualification/staging checks and four
private build-scope checks pass. Go asset/materialization packages, protected
admission routing and production-tag composition checks pass. Independent review
caught and corrected a missing transport allowlist, insecure authorization,
post-operation qualification and a concrete-adapter binding regression. Main's
qualification metadata also cross-checks the production JavaScript contract.
The original source gate, signed activation and file/privacy authority remain.
The installation probe below supplies package staging and signed-payload evidence
for its exact source. Native GUI and complete installation acceptance remain;
no public-release permission is established.

## Private installation probe, 2026-09-15

Source `8365b16ce42b143e4163931fd944222d6d71995d` built with
`scripts/package-office-private-local.mjs`, using the fixed host Office assets
and a fresh output directory. All four development native components, desktop
bundles, packaged Go runtime, private Office qualification, staged payload seal,
application signing and DMG creation completed. The resulting
`analytix-1.0.6-mac-arm64.dmg` is 469609175 bytes, SHA-256
`a25295033de0527a50d48f8e6d9862ef73bb45b54aed2f87508e885e5344e508`.
Its classification is `development_clean_non_publishable`; `publishable` and
`releaseEligible` remain false. No artifact was published.

`node scripts/package-candidate-smoke.mjs <fresh-output-directory>` passed DMG
verification, read-only mounting, product layout, bundle identity and strict deep
signature verification. The app was then copied from that read-only DMG to an
exclusive task installation directory. Its signature passed again, and the
installed qualification and package authority reference the same source commit.
This does not establish notarization, upgrade, native rendering or product RC.

A new named synthetic-only profile was created on the managed local filesystem,
with separate application/runtime state, an explicit task Keychain and a retained
controller whose reconnect endpoint was checked before provisioning. No prior
profile or credential was copied. Previously generated synthetic Office files
were copied only as opening fixtures; they are not installed-UI generation
evidence. The separate loopback-only synthetic Provider passed metadata health
checks but was not configured through the app or used for a model task.

The first installed launch stalled before application initialization. A bounded
sample showed `SecItemAdd → defaultKeychainUI → AuthorizationCopyRights` in
Electron/Chromium. The Go task-Keychain binding does not bind that cookie-encryption
path. Computer Use refused access to `com.apple.SecurityAgent`. The task-owned
launch was stopped; the installation, profile, original task Keychain and evidence
were retained. No default-Keychain policy or encryption setting was changed.
An independent usable macOS acceptance session is requested for this dependent
GUI path. Real text/media Provider authority also remains unavailable. Other
implementation and review continue; none of these gaps reduces the completion
contract below.

## Completion evidence

Use synthetic materials in one conversation to generate XLSX, then DOCX and PPTX
from the same data, then a structured canvas and image. Select native content,
dispatch explicit tasks, review and apply changes, verify untouched objects and
types, undo, close/reopen, switch tabs and restart the installed application.
Inspect real package structure, formula expressions/results, styles, object
identity and representative rendering in addition to screenshots and hashes.

Cover stale thread/workspace/object/revision/generation, incomplete captures,
hostile embedded instructions, external modification, duplicate apply, lost
commit replies and same-operation recovery, plugin disable/upgrade, third-object
eviction, annotation drafts, macros/external relations and protected model
projection versus authorized local display. Unresolved data loss, type corruption,
wrong-range modification, privacy leaks or false saved status prevent acceptance.

Delivery includes the exact source/PR/check state, installer path/hash/version and
admission status, actual sample files before/after changes, self-contained GUI
evidence, plugin/Skill capability and dependency inventory, and the Write migration
map. Missing real media authorization, engine distribution obligations, signing
or an unavoidable system interaction blocks only the dependent evidence. It does
not remove that requirement or establish overall completion.
