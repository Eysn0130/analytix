# Unified workbench and productivity plugins

Status: Reference / accepted implementation direction, 2026-09-14. This is an
incremental design, not an assertion that the native Office editors ship.
Research baseline: `60839b721b273119ab30158c65109dde4db52441`.

## Target and compatibility

The left navigation contains conversations and projects, without Code/Write or
a replacement mode heading. The central conversation retains its thread,
Composer, approvals and Go Core. Documents and other objects open in the right
workspace. Opening a file must not create or select another thread.

| Baseline | Target | Implementation boundary |
| --- | --- | --- |
| `Workbench`, separate Write route/sidebar/assistant | One conversation with a right document surface | Consume old Write routes as surface-open intents; retain thread IDs, histories, settings and original files. Never merge histories. |
| TipTap/CodeMirror Markdown, local selection and autosave | Keep existing editing and local formatting | S1 proves ordinary text open/edit/save/reopen and mock conversation. It does not establish DOCX editing. |
| Write registry excludes threads from ordinary navigation | Historical Write threads remain selectable | Retain registry metadata for compatibility; remove presentation filtering and implicit thread changes. |
| Inline edit creates a temporary Core thread | Explicit editor requests use the central conversation | Keep Core projection and normal tool approvals. Existing tool edits are written before line review; do not call this accept-before-save. |
| File, browser, changes, plan, todo, summary and child panels | Existing surfaces remain reachable | Collapse is a layout action. Preserve drafts; failed persistence blocks destructive close/switch. |
| Renderer quote carries text/path/line | Visible frozen reference with working-copy and thread checks | S1 guards local snapshots. Core-verifiable artifact/selection tokens and typed operations belong to S2; frontend hashes never grant authority. |
| Plugin UI may retain local installation hints | Core registration, permission, readiness and enabled state | S2 must distinguish built-in, enabled, engine-ready and callable. No static Office success cards. |
| `.doc` export contains Word-compatible HTML | Honest format labels and separate native engines | This is not binary DOC. Existing DOCX generation is not native DOCX editing or round-trip fidelity. |

The Go Core retains Provider, permission, projection, task and durable-history
authority. Current ordinary text requests pass both turn-content projection and
the final Provider projector. Local editor visibility does not grant model or
export access. Preserve rejection of unclassified images, existing credential
authority, native isolation requirements and all release acceptance gates.

Artifacts belong to an authorized resource domain; threads reference them.
Deleting a thread or disabling a plugin must not delete a document. Start with
ordered single-object writes and isolated proposals; no collaborative server or
second agent loop is required.

## Engine research admission

No engine below is admitted to the distributable application by this document.
No third-party code, prompt, binary or asset is copied in S0.

| Fixed research input | Evidence and disposition |
| --- | --- |
| ONLYOFFICE DocumentServer 9.4.0, `fb73d33c85f59d1a5d2e5a5ed05388ebc2418337` | DOCX/XLSX/PPTX candidate. Server deployment or an offline DesktopEditors application is not an Electron embedding SDK. [License](https://github.com/ONLYOFFICE/DocumentServer/blob/fb73d33c85f59d1a5d2e5a5ed05388ebc2418337/LICENSE) includes AGPL and additional obligations; assets need separate review. [Save callbacks](https://api.onlyoffice.com/docs/docs-api/usage-api/callback-handler/) require host persistence confirmation. Reserve. |
| SuperDoc 2.14.0, `2930257e6473dfaf5600af728a063e88c0f9ca49` | Browser DOCX candidate, not XLSX/PPTX. [Fixed source](https://github.com/superdoc-dev/superdoc/tree/2930257e6473dfaf5600af728a063e88c0f9ca49) is AGPLv3; proprietary distribution has a separate commercial route. Conditional DOCX experiment; no distribution admission. |
| Univer 0.25.1, `c9c8607471013f110b7a401d79bfb248e59faa1c` | [Core](https://github.com/dream-num/univer/tree/c9c8607471013f110b7a401d79bfb248e59faa1c) is Apache-2.0. [Native XLSX import/export](https://docs.univer.ai/guides/sheets/features/import-export) requires a conversion backend/Pro boundary. Internal snapshot save is not native file persistence. Current online docs may describe a different version. Reserve. |
| zetajs 1.2.0, `57360bcb0e7726ffa0e66567c8041261b959f8dd` | [Bridge](https://github.com/allotropia/zetajs/tree/57360bcb0e7726ffa0e66567c8041261b959f8dd) is MIT. [ZetaOffice](https://zetaoffice.net/) offers client-side Writer/Calc/Impress. Bridge licensing does not cover WASM, LibreOffice, Qt or fonts. Fixed binary/source/notices inventory is missing; moving CDN latest is not admissible. HOLD. |

At most two experiments are selected: SuperDoc with one synthetic DOCX, and
zetajs with one synthetic DOCX/XLSX/PPTX each after binary provenance closes.
Neither has executed at S0. Each must demonstrate offline open, semantic
selection, local editing, save receipt, close/reopen, structure preservation
and visual comparison. Stop on hidden network dependencies, missing license
rights, unsupported operations or unexplained data loss. No account connection,
paid service or dependency upgrade is implicit in an experiment.

Installed host plugins are research inputs, not redistributable product assets.
Manifest labels do not override narrower file licenses or grant rights to host
libraries. Analytix adapters and focused Skills must be independently authored
unless exact source, destination, license and notices are admitted through the
[upstream process](upstreams/README.md).

## Slice acceptance

- S0: actual source/ownership/resource intake, compatibility map and bounded
  engine decision. Keep private machine inventory in the existing Owner ledger.
- S1: existing text editors in the right workspace, one active conversation and
  Composer, retained historical entry points, save/reopen and synthetic quote
  conversation; focused behavior, recovery and UI evidence.
- S2: minimal artifact/selection/operation contracts and Core-backed built-in
  plugin state, including disable/recover/version binding.
- S3: admitted native DOCX editor and honest legacy DOC conversion limits.
- S4: separate spreadsheet and presentation vertical slices and native files.
- S5: canvas and existing browser/files/knowledge reference enhancements.
- S6: cross-surface regression and exact-candidate handoff under existing gates.

Source tests, mock Provider, real Provider, package and installation results
remain separate. S0/S1 cannot mark native Office, A0, B1, Product RC or Formal RC
complete. Use focused commits and ordinary PRs; never bypass required checks,
publish an installer or alter release status on the strength of this design.
