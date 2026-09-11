# Final desktop QA - 2026-06-18

## Scope

This run validates the current analytix desktop closure on macOS from
`/Users/sun/Projects/analytix`.

Current branch during QA: `codex/analytix-architecture-closure`.

Final handoff branch after QA: `master`.

## Evidence

| Evidence | Result |
| --- | --- |
| Real Electron startup | Pass: `npm run dev` built main/preload/renderer and launched a visible Electron window titled `Analytix Desktop`. |
| Startup screenshot | `docs/analytix/qa/electron-startup-2026-06-18.png` |
| Runtime userData | Pass: Electron used `/Users/sun/Library/Application Support/analytix`. |
| Local trace file | Pass: `/Users/sun/Library/Application Support/analytix/traces/thread-unknown.jsonl` existed with 24,200 bounded trace rows. |
| Trace event families observed | `thread.react.commit_sample`, `thread.projection.reduced`, `thread.rows.updated`, `thread.virtualizer.measured`. |
| Desktop navigation automation | Partial: macOS `System Events` click automation blocked on Accessibility permission, so deeper GUI navigation was not automated. |
| 120-turn real GUI scenario | Partial: renderer long-thread smoke tests passed, but the required real Electron 120-turn scenario was not fully executed in this run. Renderer smoke remains auxiliary evidence only. |

## Command Results

| Command | Result |
| --- | --- |
| `npm run typecheck` | Pass |
| `npm run lint` | Pass with 10 warnings, 0 errors. Warnings are existing `react-hooks/exhaustive-deps` cleanup items documented in spec 07. |
| `npm run build` | Pass through `npm run dist -- --mac dmg --arm64`. |
| `npm run test` | Pass: 186 test files, 1348 tests. |
| `git diff --check` | Pass |
| `git ls-files --others --exclude-standard` | Expected untracked delivery files plus local `.agents/` skill files. `.agents/` must remain uncommitted. |
| `npm run dist -- --mac dmg --arm64` | Pass: generated macOS arm64 dmg/blockmap. Code signing and notarization were intentionally skipped because credentials are not configured. |

## Naming Audit

Current-product source fixes in this pass:

- `src/main/services/workspace-editors.ts` temp file prefix changed from `ds-gui-icon-*` to `analytix-icon-*`.
- Test temp directory prefixes changed from `ds-gui-*` to `analytix-*`.
- Current docs no longer reference the removed `src/shared/ds-gui-api.ts`; write inline docs now point to `src/shared/analytix-api.ts`.
- The root upgrade plan now has a top-level historical snapshot banner so old Kun names are not interpreted as current product identity.

Remaining old-name hits are classified as:

- specs and migration documents describing forbidden old states;
- `docs/legacy/kun/` and `release/legacy/` historical archive;
- isolated compatibility layers such as `legacy-data-migration`, `settings-store`, and legacy plan prompt recognition;
- provider/model names where DeepSeek remains a supported provider;
- the legacy git remote, which was renamed from `origin` to `legacy-origin` before handoff and remains a push blocker.

## Status Matrix

| Area | Status | Evidence | Remaining limitation |
| --- | --- | --- | --- |
| Specs 01-06 architecture closure | Pass | Code scans, tests, spec 07 added | Historical spec text intentionally retains old-state terms. |
| Spec 07 desktop QA | Partial | Real Electron startup screenshot and trace | Full 120-turn Electron GUI scenario not completed. |
| Naming audit | Pass with allowlist | `rg` scan classification and source fixes | Legacy migration/docs still intentionally mention old names. |
| Runtime and settings | Pass | Typecheck, tests, Electron userData path | Deep GUI settings interaction not automated in this run. |
| Long-thread smoothness | Partial | Renderer smoke and scheduler tests | Real GUI long-thread streaming/prepend scenario remains manual QA. |
| Visual/icon readiness | Pass | Startup screenshot, build assets, visual token changes | Deeper cross-mode visual walkthrough not automated. |
| Packaging/release | Partial | macOS arm64 dmg built | Signing/notarization, real release domain, final repository, and Windows NSIS machine verification remain release blockers. |
| Git/remote safety | Pass with limitation | `origin` renamed to `legacy-origin`; `master` has no upstream | No verified analytix remote is configured, so nothing was pushed. |
