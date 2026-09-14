# Built-in Office package host

Status: Reference. Describes source-development integration, not native preview
acceptance, packaged admission or release readiness.

Documents, Spreadsheets and Presentations are real first-party package sources
under `plugins/analytix-*`. Their static declarations request only local read-only preview of a
user-selected object. This supersedes the earlier editing experiment following
the owner’s 2026-09-14 product decision. They install through the
existing plugin materialization owner and installation signing authority. They
contain no alternate Agent, credentials, shell entrypoint, MCP server or Hub
login dependency.

Unpackaged Electron supplies its own source root to the Go runtime. Ambient
copies of that configuration are removed before child launch. Packaged-executable inspection denies this source path even when a caller
supplies the source environment. The normal development launcher also builds
with `analytix_prod`; that Go tag is not evidence of a packaged application.
Inspection binds the source tree, canonical declaration and adapter descriptor
to an explicit `development-source` receipt. It never becomes formal package
authority or a publishable artifact. Interrupted materialization keeps its first
durable intent rather than selecting a new transaction on restart.

The protected `/v1/local-display/plugin-package-host` endpoint lists packages,
changes desired state and invokes a finite static adapter operation. It uses the
existing runtime bearer and typed local-display header. Electron accepts this
IPC only from its own main frame. Browser previews and third-party frames cannot
manage the host. The renderer cannot register roots, executables or principals.

Signed activation is scoped to the exact installed generation and uses revision
compare-and-swap. Missing activation is `unset`, not a fabricated disabled
record. The plugin page reads and persists state through Core; it does not use
marketplace localStorage flags as installation or execution authority. After
either a successful or uncertain mutation it reads the current signed state.
Upgrading an authentic historical local-edit installation reconstructs its old
registration only for signed installed-tree comparison. New source admission
remains preview-only, and the new generation requires its own activation.
Enabled and available are distinct: without a ready adapter, an enabled package
is displayed as an unavailable preview.

One Host orders activation changes and complete invocations. Disable waits for
an invocation already executing; after its commit, the previous enabled state
cannot start another call. An adapter must not re-enter the Host. Current
source registrations can attach the fixed local Zeta adapter when all pinned
engine resources pass verification. Loading a plugin page does not establish
that an Office engine has been admitted for distribution or executed.

In the source experiment, opening DOCX/XLSX/PPTX creates a sandboxed, isolated
Electron work surface with a finite Main-owned message bridge. The page has no
generic desktop bridge, filesystem paths, Provider or unrestricted network
access. The fixed resource server sets COOP/COEP and serves verified held bytes;
navigation, downloads, permissions and native save/open shortcuts are blocked.
The product has one file header, the native content area and a compact preview
status bar. It exposes no formatting, formula input, insertion, undo or Save.
DOCX defaults to page-width fit; XLSX defaults to 100% with native sheet
navigation; PPTX defaults to whole-page fit with bounded previous/next controls.
The compact footer reports actual native zoom. Manual zoom exits automatic fit.
These view controls stay inside the isolated surface and cannot dispatch model
edits or arbitrary UNO commands. A light reading background is configured only
inside the engine's isolated virtual configuration, never the host Office profile.
Close, collapse and focus remain independent actions in the containing workspace.

Core reads at most 16 MiB of native OOXML per object, checks bounded ZIP/XML
structure and rejects active content and external relationships. The protected
local transport uses strict base64, never a Markdown conversion. Office adapters
advertise only open and close operations. They reject commit/status calls, and
the Office-specific file encoder rejects writes even through a direct session.
Existing text editing remains separate and unchanged by this preview policy.

The native loader requires ReadOnly and verifies XStorable.isReadonly before
publishing the object; LockEditDoc, LockSave and LockExport close native UI
alternatives. Native read-only mode alone is not an API write barrier, so the
worker removes mutation and export dispatch entirely. The surface blocks input,
paste, drop and editing shortcuts. Main also rejects legacy editing requests and
never exports, commits or acknowledges a save. Writer also emits modify notifications for view changes. Only a confirmed
read-only, unmodified model is treated as a view notification; true or unknown
mutation still invalidates the preview. The worker never clears modification
flags to conceal a change. Closing a preview releases its
resources without a Save/Discard dialogue.

The current surface supports one active native object per window. Selection is
local viewing metadata, not authority for AI patches. Pinned Noto CJK fonts load
inside the native virtual filesystem; full glyph and layout coverage still
requires actual-file checks. Packaged execution remains denied: source preview
availability does not establish binary, Qt or font redistribution permission.
The earlier editable-engine outputs and evidence are retained as historical
experiments and are not preview acceptance evidence.

Focused evidence includes all three actual repository package trees materialized
into an isolated installation, persisted activation across a fresh Host, exact
protected-route checks, interrupted-intent reuse, Host race tests, and Main
response/generation/revision checks. It does not replace the mandatory aggregate
gate or GUI/native file acceptance of the final candidate.

## Read-only preview visual check — 2026-09-14

The designed development candidate based on `9e19f947e` was exercised in an
isolated Electron Mock profile with synthetic two-page DOCX, two-sheet XLSX and
three-slide PPTX files. Actual native rendering, sheet/slide navigation,
selection, fold/restore and focused-panel resizing passed. DOCX uses page-width
fit, XLSX keeps native sheet tabs at 100%, and PPTX fits the complete slide.
Keyboard editing and save attempts were blocked; all three original SHA-256
values remained unchanged after normal preview close and application quit.
Plugin enablement survived a normal application restart.

The UI uses existing design-system spacing and icon controls, a single file
header, a light reading background and a compact status bar. The owner's Codex
screenshots are design references, not Analytix acceptance evidence. The local
frontend and product-audit skills informed composition; transitions.dev was
consulted for restrained motion principles without copying or installing its
skill or adding a motion dependency.

Final screenshots and the source/file-hash receipts are retained in the
2026-09-14 Owner evidence directory (`office-preview-showcase` and
`native-office-designed-preview-source-snapshot.json`). This is bounded
three-file development GUI evidence. It does not establish general Office
fidelity, dark-theme coverage, screen-reader coverage, packaged admission, or
the mandatory gate of a later candidate. Existing real-provider credentials
were neither re-entered nor accessed by these checks.
