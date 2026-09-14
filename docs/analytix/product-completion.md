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
| Annotation lifecycle | Per-thread/object in-memory draft survives collapse; stale revision keeps note and requires selection | Protected persistent storage, normal restart, capacity and closed-tab recovery |
| Native proposal refresh | Single-flight polling with failure backoff; new events supersede pending reads | Tool-completion refresh integration and installed behavior |
| DOCX | Core absent-only generation calls a data-only `docx` codec; checkpoint binds the created bytes and installation principal; opaque artifacts resolve into the current native workspace | Full headers/fields/links coverage, installed repeated-text positive case and representative rendering |
| XLSX | Native cells expose type/formula/value/format; unsafe numeric/formula-to-text mutation rejected in Main and worker | Real typed numeric/formula/range operations, calculation validation, charts and summaries/pivots |
| PPTX | Native preview and bounded shape selection | Object-based generation, targeted style/chart edits, per-slide quality checks |
| Canvas / images | AtlasFlow and existing local media assets remain reusable | Stable scene identity, fact/layout separation, selection/notes/local edits, versioned exports; real media Provider integration |
| Plugin Skills | Documents 0.2.0 declares an original workflow, fixed generation capability and installed snapshot; discovery and generation follow signed activation | XLSX/PPTX/Canvas contributions, full dependency handlers and installed end-to-end evidence |
| Write migration | Existing MD/TXT editors, exports and same-thread document surface retained | Map all custom actions, presets, retrieval, autosave, conflicts, review and export to the shared product flow |
| Browser / files / PDF / knowledge | Existing surfaces remain | Shared authorized object opening and typed annotation anchors; no implicit indexing or uploading |
| Apply / recovery / Diff | Core scopes, protected parts, CAS and status queries exist | Real local original-value Diff, third-object eviction, pending-operation and undo restart recovery |
| Installation | Native assets remain `source-experiment`; packaged rejection retained | Exact engine/Qt/font/source obligations, admitted packaged resolution, local installer and installed end-to-end journey |
| Security / delivery | Existing macro prohibition, privacy authority and protected profiles preserved; controlled object reads/commits reject hard-link aliases, including post-inspection drift | Exact-candidate CodeQL, applicable CI/review and remaining required negative cases |

The numeric/formula text-path restriction is interim damage prevention. It is not
acceptance of a text-only spreadsheet product. In-memory note retention is not
a claim of durable restart recovery. Source tests are not native engine fidelity,
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
required; this is not a claim of packaged admission or plugin lifecycle closure.

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
