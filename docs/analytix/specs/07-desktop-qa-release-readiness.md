# analytix desktop QA and release readiness spec

Status: Accepted acceptance contract with dated historical evidence.
Current as of: 2026-07-27 for first-stage A/B routing and current blocker corrections.
Source of truth for a pass claim: a fresh run against the identified commit,
platform, package, credentials, and commands.

The 2026-06-18 results below are historical snapshots, not the current lint or
release state. On 2026-07-10, `npm run lint` did not pass the current worktree
(524 errors and 2 warnings, largely involving generated/tool directories not
covered by the ESLint ignore set). Treat that as baseline/tooling debt until a
focused rerun proves otherwise; never repeat the old “10 warnings, no errors”
statement in present tense. The updater now has an `analytix.top` feed, but
release readiness still requires feed, signing, provenance, packaged-runtime,
and real-platform proof.

## 1. Purpose

This spec is the final desktop-readiness gate for analytix. Specs `01` to `06`
define the architecture, migration, visual system, smoothness path, and closure
rules. This spec defines the last acceptance evidence required before the
project can be considered ready for handoff.

The key rule is stricter than the previous smoke evidence:

```text
Renderer smoke tests are auxiliary evidence only. They cannot replace final GUI
verification in a real Electron desktop process.
```

Renderer tests are still required because they make long-thread and trace
behavior repeatable. Final readiness, however, must include the real app window,
preload bridge, Electron main process, packaged runtime boundary, window chrome,
native menus, dock/tray/icon paths, and platform packaging configuration.

First-stage readiness is reported as two independent formal-package rows over
the same additive Agent:

- **Milestone A — packaged general Agent:** in an isolated non-case repository,
  complete a real request through code read, Plan/Todo, edit, real test,
  subagent, compaction, normal exit, relaunch, and exact thread/result recovery.
  Funds, case binding, DSV2, evidence authority, and controlled PII are not
  startup prerequisites.
- **Milestone B — packaged valuable funds analysis:** in a case workspace,
  complete multi-turn fixed `analyze_account_flows` queries over the exact
  immutable DuckDB snapshot through EvidenceReceipt, ClaimRecord, Final Gate,
  privacy-safe ordinary UI, restart/compaction/snapshot evolution, and the
  separately authorized controlled full-value sink. `count_case_rows`, a
  fixture, direct MCP, or Milestone A cannot satisfy this row.

Case capability is additive per protected call/effect, not a replacement mode.
Authority loss removes only the affected case-data access and factual
publication; ordinary files, shell, Git, Plan/Todo, Skills, ordinary MCP,
thread, subagent, compaction, recovery, and shutdown remain usable in the same
Agent and thread. Missing credentials, lawful case authority, trusted-sink
authorization, signing/notarization, or an approved target platform is recorded
as `BLOCKED`/`UNVERIFIED`, never as a pass and never as a reason to disable the
independent ordinary baseline.

## 2. Final Acceptance Scope

The final QA pass must cover these current analytix surfaces without deleting
or hiding existing functionality:

| Area | Required coverage |
| --- | --- |
| Electron desktop startup | A formal packaged app launches a visible analytix window, reaches a usable runtime state, and reports runtime health. `npm run dev` and a visible window alone are diagnostic evidence only. |
| Long-thread scrolling | At least 120 turns are loaded in the real GUI; bottom-distance virtualizer preserves history position and does not jump on prepend. |
| Streaming | Near-bottom streaming follows the active turn; away-from-bottom streaming does not pull the user out of history. |
| Markdown/code rendering | Streaming uses lightweight rendering; finalized markdown/code blocks render after finalization without blocking composer input. |
| Composer | Text input, model picker, reasoning effort, attachments, file references, queued messages, interrupt, `/plan`, `/review`, `/new`, and side conversation actions remain reachable. |
| Sidebar | Thread list, search, archive/restore, workspace creation, Code/Write/Schedule/Connect Phone/plugin navigation, focus mode, and collapse/resize remain usable. |
| Settings | Runtime, providers, write, updates, memory, media, speech, worktree, legacy import, and Connect Phone settings read/write the current analytix schema. |
| Runtime | `packages/runtime` is bundled, `analytix serve` is the public CLI, HTTP/SSE replay works, approvals/user-inputs/usage remain equivalent. |
| Terminal | Bottom terminal opens, resizes, streams output through its scheduler, and does not steal timeline scroll space. |
| Dev Browser | Preview detection card opens the browser panel; panel resize/collapse works without affecting streaming. |
| Write | Workspace selection, editor/preview, file tree, assistant panel, inline completion, attachments, export, and empty state remain functional. |
| SDD | Requirement draft, assistant panel, prototype/image actions, plan upgrade, trace persistence, and history restore remain functional. |
| Plugin marketplace | Marketplace loads, search/filter/install state renders, and current product naming is analytix. |
| Connect Phone | User-visible copy says Connect Phone/analytix; `claw` may remain only as an internal compatibility module. |
| Schedule | Manual and scheduled tasks can create/reuse analytix runtime threads and retain Connect Phone task context. |
| Logs/update | Logs directory opens, update UI uses analytix release settings, and the configured feed is verified rather than merely present. |
| Focus/dark/light | Focus mode, dark mode, light mode, and reduced-motion states keep text legible and controls reachable. |
| Mascot/cameo | Retained mascot/cameo mode uses current source naming and does not present Kun/iKun as product identity. |
| Window title bar | macOS/Windows title bar controls, drag regions, and collapsed sidebar safe insets remain correct. |
| Dock/tray/app icon | App, dock, tray, installer, splash, and readme icon paths use analytix assets, not Kun or Codex. |
| Packaging | macOS dmg/zip, Windows NSIS x64, and Linux AppImage remain configured; platform-specific execution is recorded truthfully. |

## 3. Required Desktop Long-Thread Scenario

The real desktop QA scenario must create or load a thread with at least 120
turns and include all of these row types:

- user and assistant messages;
- streaming assistant deltas;
- long markdown sections;
- fenced code blocks;
- tool calls;
- approvals;
- `request_user_input` rows;
- file-change summaries;
- review plan and review summary cards;
- generated files or equivalent file output rows;
- terminal activity when the bottom panel is open;
- Dev Browser launch card when a local preview URL is detected.

Acceptance behavior:

- When the user is near the bottom, new streaming content keeps the active turn
  visually attached to the bottom.
- When the user scrolls into history, new streaming content updates unread or
  scroll-to-bottom affordances without forcing a jump.
- When history is prepended, the first visible historical row keeps its visual
  offset.
- Window focus changes, panel open/close, and terminal resize do not freeze the
  composer.
- Markdown finalization does not include prompt or assistant output in trace
  files.

## 4. Local Tracing Evidence

Tracing must be local-only and privacy bounded.

Expected location:

```text
Electron userData/traces/thread-*.jsonl
```

Required event families:

```text
thread.event.batch_received
thread.delta.buffered
thread.delta.flushed
thread.projection.reduced
thread.rows.updated
thread.virtualizer.measured
thread.scroll.anchor_corrected
thread.react.commit_sample
thread.markdown.finalized
```

Allowed trace payload content:

- counts;
- durations;
- booleans;
- status values;
- row kinds;
- bounded ids;
- byte or character lengths.

Forbidden trace payload content:

- API keys or tokens;
- full prompts;
- full assistant output;
- direct personal contact or payment identifiers;
- full filenames when a count or extension category is enough.

## 5. Legacy Compatibility Isolation

The following retained modules are compatibility layers, not current product
identity:

| Compatibility layer | Status | Current-product rule |
| --- | --- | --- |
| `src/main/legacy-data-migration.ts` | Isolated legacy import/migration support. | May mention old `Kun` and `DeepSeek GUI` userData names only to locate old installs. |
| `src/main/settings-store.ts` | Isolated settings fallback. | May read legacy paths; new saves must write analytix settings with top-level `runtime`. |
| `src/renderer/src/plan/plan-prompts.ts` | Isolated legacy prompt recognition. | May recognize old internal plan prompt strings so they do not become user requests. |
| provider/model DeepSeek files | Provider support. | DeepSeek may remain as a provider/model name, never as app identity. |
| Connect Phone `claw` internals | First-stage internal module name. | User-visible copy must say Connect Phone/analytix. |
| `docs/legacy/kun/` and `release/legacy/` | Historical archive. | Must remain clearly marked historical and not linked as current architecture guidance. |

Any old naming outside these boundaries is a current-product bug unless another
spec explicitly allowlists it.

## 6. Naming And Release Blockers

Before handoff, run the bridge and naming scans from spec `06`, then classify
every remaining hit. These blockers must be called out even if tests pass:

| Blocker | Required state |
| --- | --- |
| Update feed | Current config defaults to `https://analytix.top/desktop/releases/standard-win/`; verify reachability, metadata/channel correctness, TLS, artifact compatibility, and release authorization before public updates. |
| Local project metadata | `package.json` must not point at a GitHub repository or placeholder homepage. Local release provenance should come from local tags, build metadata, and configured distribution endpoints. |
| Remote safety | Publishing must not depend on a GitHub remote. Do not push from this worktree as part of the default release process. |
| Code signing | macOS and Windows release artifacts are not production-ready until signing/notarization credentials are configured and verified. |
| Windows NSIS runtime | Windows NSIS must be verified on a Windows machine; macOS/Linux builds cannot certify it. |

## 7. Verification Commands

Minimum command gate:

```bash
npm run typecheck
npm run lint
npm run build
npm run test
git diff --check
git ls-files --others --exclude-standard
```

These commands, process exit zero, and a visible development window are
necessary diagnostics where applicable; none substitutes for the formal
packaged public-seam workflows required by Milestones A and B.

Packaging gate:

```bash
npm run dist
```

If `npm run dist` is too broad for the local machine, run the platform-specific
command that can execute truthfully on the current OS and record the rest as
pending platform verification. On macOS, `npm run dist:mac` is preferred when
time and signing constraints allow it. Windows NSIS must not be marked passed
from a non-Windows machine.

## 8. QA Evidence Folder

Desktop QA evidence should be saved under:

```text
docs/analytix/qa/
```

Recommended evidence:

- desktop startup screenshot;
- long-thread near-bottom screenshot;
- long-thread away-from-bottom screenshot;
- settings screenshot;
- Write screenshot;
- SDD screenshot;
- Plugin marketplace screenshot;
- terminal/dev browser screenshot;
- a short markdown report with pass/fail/blocker rows.

Screenshots must not include private prompt/output content unless intentionally
created from synthetic fixtures.

## 9. Final Readiness Matrix

The final report must include:

| Area | Status | Evidence | Remaining limitation |
| --- | --- | --- | --- |
| Registered accepted specs covering architecture, upstream absorption, benchmark, and control-plane closure | pass/partial/blocker | files/tests/scans | limitation |
| Spec 07 desktop QA | pass/partial/blocker | Electron run, screenshots, trace | limitation |
| Naming audit | pass/partial/blocker | scan output + allowlist classification | limitation |
| Runtime and settings | pass/partial/blocker | tests and GUI behavior | limitation |
| Milestone A — packaged general Agent | pass/partial/blocker | formal package; isolated repository read/Plan/Todo/edit/real test/subagent/compaction/exit/relaunch/thread recovery | limitation |
| Milestone B — packaged privacy-safe funds analysis | pass/partial/blocker | formal package; real immutable DuckDB `analyze_account_flows`; receipt/claim/Final Gate; multi-turn/restart/snapshot evolution | limitation |
| Controlled full-value case result | pass/partial/blocker | lawful same-case authorization; PIIProjectionGrant; audit binding; trusted sink; zero-leak scans | limitation |
| Deferred P3 and unrun target platforms | pass/partial/blocker | explicit deferred or platform-specific evidence | limitation |
| Long-thread smoothness | pass/partial/blocker | GUI scenario + renderer smoke | limitation |
| Visual/icon readiness | pass/partial/blocker | assets/screenshots/scans | limitation |
| Packaging/release | pass/partial/blocker | build/dist result | limitation |
| Git/remote safety | pass/partial/blocker | branch, commit, remote | limitation |

Do not claim final closure if either required formal packaged public seam could
not be executed. A real Electron development window, renderer smoke, unit
tests, or package process exit cannot upgrade either row. Report the exact
blocker and keep that row partial or blocked.

## 10. Historical Lint Warning Baseline

The 2026-06-18 final pass had ESLint warnings but no ESLint errors. They
are classified as existing hook dependency warnings and are not identity,
runtime, naming, packaging, or smoothness blockers for this closure pass:

| File | Warning class | Classification |
| --- | --- | --- |
| `src/renderer/src/components/SettingsView.tsx` | `react-hooks/exhaustive-deps` for `writeTypography` | Existing settings hook dependency cleanup. |
| `src/renderer/src/components/chat/ConnectPhoneView.tsx` | `react-hooks/exhaustive-deps` for `cancelInstallAttempt` | Existing Connect Phone install-flow hook cleanup. |
| `src/renderer/src/components/chat/SidebarClawDialog.tsx` | `react-hooks/exhaustive-deps` for `cancelInstallAttempt` | Existing Connect Phone dialog hook cleanup. |
| `src/renderer/src/components/write/WritePdfViewer.tsx` | `react-hooks/exhaustive-deps` for `scrollToPage`, `searchMatches`, and complex dependency expression | Existing Write PDF viewer hook cleanup. |

These warnings should be cleared in a focused follow-up before a strict
warning-free lint gate is required.

## 11. 2026-06-18 Closure Run

Evidence recorded in:

```text
docs/analytix/qa/final-desktop-qa-2026-06-18.md
docs/analytix/qa/electron-startup-2026-06-18.png
```

Observed results:

- `npm run dev` launched a real Electron window titled `Analytix Desktop`.
- Electron used `/Users/sun/Library/Application Support/analytix` as userData.
- Local trace evidence existed at
  `/Users/sun/Library/Application Support/analytix/traces/thread-unknown.jsonl`.
- `npm run typecheck`, `npm run test`, `git diff --check`, and macOS arm64 dmg
  packaging passed.
- `npm run lint` exited successfully with the 10 warnings listed above.
- `npm run dist -- --mac dmg --arm64` generated the macOS arm64 dmg/blockmap,
  with signing and notarization skipped because credentials are not configured.
- No GitHub remote or PR flow is required for release readiness; no push is
  part of the default local release process.

Current desktop QA status is partial, not full pass, because the real Electron
120-turn long-thread scenario was not fully executed in this run. The renderer
long-thread smoke remains useful auxiliary evidence, but it does not replace
the final GUI scenario required by this spec.
