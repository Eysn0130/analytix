# Built-in Office package host

Status: Reference. Current source integration under the continuous product target
in [product-completion.md](product-completion.md). Source capability, installed
local acceptance and permission to redistribute remain separate claims.

## Package contribution and Core authority

Documents, Spreadsheets and Presentations are first-party sources under
`plugins/analytix-*`. The native canvas remains a read-only user interface; it
is not a complete manual Office editor. That restriction does not prohibit
Core-owned generation, reviewed proposals or explicit application of a proposal.

Each Office 0.2.0 package declares one original fixed Skill and a paired
capability: `documents` / `office.document-generation`, `spreadsheets` /
`office.workbook-generation`, and `presentations` / `office.presentation-generation`.
All are restricted to a new file in the current conversation. Existing
`office.local-preview` contributions retain `user-selected-object` and
`read-only` constraints. No package contains another Agent, credentials, an
ambient shell entrypoint, MCP server or Hub login dependency.

The materialization owner authenticates the full source tree, canonical package
identity, declaration, UI/adapter descriptors and declared Skill bytes. A fixed
Skill hash extends the source registration; historical registrations without
Skills retain their exact old bytes and digest. Undeclared files do not become
executable contributions. The runtime reads instructions from the verified
installed generation, not a source path added to the ordinary Skill search roots.

The existing installation authority signs activation for an exact generation and
revision. Missing activation is `unset`. Disable withdraws hosted discovery and
new generation calls; re-enabling does not revive a prepared task from an older
activation revision. Upgrades require new activation. Public discovery contains
bounded summary metadata; full instructions are supplied only by `run_skill`.
The three fixed `analytix-documents`, `analytix-spreadsheets` and
`analytix-presentations` namespaces cannot be replaced by project Skills.

Core still owns thread/workspace authorization, Provider policy, privacy
projection and persistence. Declaring a Skill or installing a plugin grants none
of those rights. Native selection proposals use current Core capture authority;
their explicit apply path checks the object, version and operation identity.
The complete typed-edit capability inventory remains part of product completion.

## Native selection and controlled modification

Opening a supported OOXML object creates a sandboxed Electron native work
surface. Its finite bridge exposes no generic desktop API, filesystem path,
Provider or unrestricted network. The fixed resource server serves verified
held bytes with COOP/COEP. Navigation, downloads, permissions and native editing
or save shortcuts remain blocked. A compact file header and native status/footer
provide bounded view controls; closing, collapsing and focusing are independent.

The source implementation now captures native selections and exposes a shared
selection action catalog through the surface, host context menu and keyboard.
Opening a menu does not send a task. Reference adds a composer card; an explicit
quick action starts a task in the same main conversation while preserving the
composer draft. Selection authority includes current object/version/capture
identity. Visual line numbers and screenshots alone cannot authorize a patch.

Core accepts bounded proposals, and explicit apply uses the managed editing
owner and compare-and-swap file commit. The adapter therefore has controlled
capture, proposal, commit and status operations in addition to open/close.
The earlier assertion that it rejects every write operation is obsolete.
The current recovery candidate persists an approved change and its Core-read
original before the native engine replaces content. Commits carry the bound
thread, change and fixed save operation. Fresh sessions discover recovery by
object and thread; undo obtains the original from Core, holds managed-file
capture and checks the current binary revision. Main no longer supplies an
in-memory original as undo authority. Local review text is reconstructed into a
separate protected-local output; model proposal parts remain privacy-projected.
Unknown save outcomes are not discarded or represented as saved. Core retains
the exact approved save candidate for an explicit, thread-bound continuation
against the original revision. Ordinary commit replay and result queries never
retry that write. Old records without retained candidates remain queryable but
cannot acquire continuation authority from caller bytes. Installed recovery
evidence remains required.
Numeric/formula/merged spreadsheet targets remain excluded from text replacement
until their typed modification paths are complete. Persistent annotation,
format-fidelity, native undo/restart and multi-object recovery acceptance remain
required; focused source tests do not establish those product outcomes.

The native surface remains protected from direct manual edits. A finite worker
operation authorized through Core may nevertheless change an isolated document
model for a proposal. Such a proposal is not a saved source file. The product
must show apply/unknown outcome/conflict truthfully and must not clear native
modification flags to fabricate a successful save.

## Generation, opening and lifecycle

DOCX and PPTX codecs receive bounded content and explicit image bytes through a
fixed host-built process entry. XLSX uses a data-only Go writer. They receive no
target path, filesystem authority or Provider credentials. The tool directory
advertises only the intersection of activated packages and available writers. Core validates the resulting OOXML, creates only an absent
target, settles the checkpoint and emits a typed artifact receipt. Local opening
resolves that receipt through the current installation principal and conversation;
public metadata contains no private file path. Live results open the native
object; replaying an existing receipt does not reopen it automatically.

One Host serializes activation with complete contribution consumption. The lock
order is Host then workspace mutation; code holding the workspace lease must not
re-enter the Host. Generation binds its prepared identity to the installed
package, activation and Skill digest before entering the file mutation owner.
An adapter or hosted instruction consumer must not recursively enter the Host.

Unpackaged Electron supplies its own source root to Go. Packaged-executable
inspection still denies this source route; `analytix_prod` by itself is not
packaged admission. The development receipt remains `development-source`,
`source-experiment`, non-publishable. A lawful qualified local packaged route is
still required by the current target; removing the denial is not that route.

Core checks native files with bounded ZIP/XML inspection and rejects active
content and external relationships. Pinned engine resources/fonts and their
notices require actual installed-file and rendering evidence. The historical
check below applies only to its recorded candidate, not subsequent code.

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
