# analytix implementation closure and acceptance spec

Status: Normative closure criteria with dated historical acceptance snapshots.
Current as of: 2026-07-10 for the currentness corrections in this header and
Section 22.
Source of truth for as-built behavior: current code/tests and
`docs/analytix/README.md`.

The production agent core is now `packages/runtime-go`; `packages/runtime` is
the TypeScript launcher/contracts/config/telemetry boundary. Historical pass
statements do not prove the current worktree passes. The official Standard
Windows localized display/artifact exception and current `analytix.top` feed
are defined in spec `02`'s implementation addendum.

## 1. Purpose

This spec is the closure layer for the analytix architecture upgrade.

Specs `01` to `05` define the target architecture. This spec defines how the current implementation must be finished, verified, and judged complete.

The goal is not to add another broad redesign. The goal is to close the loop:

- all tests pass;
- current product identity is `analytix`;
- old Kun naming is removed from current product paths;
- production code actually uses the new bridge, settings, runtime, virtualizer, tracing, and visual assets;
- no existing feature is deleted, hidden, or simplified to make the upgrade easier.

## 2. Inputs

Implementation must read these documents before editing:

```text
/Users/sun/Projects/analytix/重构升级方案.md
/Users/sun/Projects/analytix/docs/analytix/specs/01-identity-runtime-schema-reset.md
/Users/sun/Projects/analytix/docs/analytix/specs/02-release-packaging-channels.md
/Users/sun/Projects/analytix/docs/analytix/specs/03-brand-assets-visual-system.md
/Users/sun/Projects/analytix/docs/analytix/specs/04-analytix-derived-thread-virtualizer.md
/Users/sun/Projects/analytix/docs/analytix/specs/05-architecture-code-upgrade-inventory.md
/Users/sun/Projects/analytix/docs/analytix/specs/06-implementation-closure-and-acceptance.md
```

`AGENTS.md` and `docs/AGENTS.md` must be read only to identify stale Kun instructions and update them. They must not override the user-confirmed analytix decisions when they still mention `kun/`, `window.kunGui`, `agents.kun`, or `kun serve`.

## 3. Frozen Decisions

These decisions are final for this implementation pass:

| Area | Decision |
| --- | --- |
| product display name | `analytix`, lowercase |
| package name | `analytix` |
| runtime package directory | `packages/runtime` |
| CLI | only `analytix serve`; no `kun` alias |
| settings schema | top-level `runtime`; no active `agents.kun` or `agents.analytix` |
| preload bridge | `window.analytix`; no `window.kunGui` fallback |
| app id / bundle id | `com.analytix.desktop` |
| Windows AppUserModelID | `com.analytix.desktop` |
| env prefix | `ANALYTIX_*` |
| update channels | `stable` and `beta` only |
| default channel | `stable` |
| release base URL placeholder | `https://<release-domain>/analytix/channels/<channel>/latest/` |
| iKun source naming | rename to `mascot` / `cameo`; assets and mode retained |
| claw | internal module name may remain; user-visible name is `Connect Phone` |
| CodexDesktop-Rebuild | clean-room behavior/layout reference only absent material-specific written authorization; no direct code or asset source |
| final implementation naming | `Analytix-derived`, `AnalytixThreadVirtualizer`, `AnalytixIconRegistry`, `AnalytixSurfaceTokens` |
| Electron | do not upgrade in this pass |
| Rust | do not rewrite runtime or chat renderer in Rust in this pass |
| telemetry | no remote telemetry or Sentry; local tracing/log/crash only |

## 4. Allowed Legacy Exceptions

Old Kun naming is allowed only in these narrow cases:

| Location | Allowed wording |
| --- | --- |
| explicit migration/import code | `legacy-kun`, old data source descriptions |
| tests for legacy migration/import | fixtures may include old names when proving migration behavior |
| isolated historical docs, if retained | must be clearly marked as historical/legacy and not presented as current architecture |

The following are not allowed as current product implementation:

```text
window.kunGui
agents.kun
agents.analytix
kun serve
KUN_*
DEEPSEEK_GUI_*
deepseek-gui release/update identity
KunAgent/Kun current repo/homepage links
Codex-derived implementation ownership names
iKun / ikun current source naming
```

DeepSeek may remain where it is a model provider, model id, endpoint, or provider-specific helper. DeepSeek-GUI may not remain as the current app identity.

## 5. Historical Audit Baseline Before `3921dd9`

This section records the audit state before `3921dd9 feat: complete analytix
architecture closure`. It is retained as implementation history, not as the
current status. Sections 22 and 23 contain the active closure policy.

Known positive progress:

- `package.json` uses `name: analytix` and `productName: analytix`.
- `kun/` has been moved to `packages/runtime`.
- runtime package is named `analytix-runtime`.
- runtime bin exposes `analytix`.
- `APP_PRODUCT_NAME` is lowercase `analytix`.
- `window.analytix` is exposed in preload.
- settings have moved toward top-level `runtime`.
- `StreamingDeltaScheduler` is connected to the chat store.
- brand assets and visual token modules have started.
- `AnalytixThreadVirtualizer` files and tests exist.

Known incomplete areas:

- `npm run test` is not green.
- stale tests and some real behavior still conflict around identity, settings migration, legacy data migration, and bundled UI plugin naming.
- `AGENTS.md`, `docs/AGENTS.md`, README/DESIGN/docs/examples still contain current-product Kun instructions and text.
- `window.analytix` is still a broad flat API object, not the domain facade required by spec `01`.
- `AnalytixThreadVirtualizer` is not fully integrated as a bottom-distance scroll layout.
- virtual spacer heights are currently at risk of drifting because spacer nodes are inside a flex container with row gap.
- `ThreadScrollLayout` exists but is not the primary production scroll contract.
- tracing types and in-memory sink exist, but trace events are not persisted to Electron `userData/traces/*.jsonl`.
- `MarkdownFinalizationQueue` exists but is not connected to the production finalized markdown path.
- Workbench still subscribes to `blocks` in top-level paths that can affect streaming hot-path isolation.
- critical generated assets are untracked and must be included if referenced.

## 6. P0 Test Closure

The first implementation task is to make the full test suite pass.

The previously observed failing files were:

```text
src/main/app-identity.test.ts
src/main/legacy-data-migration.test.ts
src/main/settings-store.test.ts
src/main/services/ui-plugin-service.test.ts
```

Expected closure:

- `src/main/app-identity.test.ts` must match lowercase `analytix`, not `Analytix`.
- settings tests must expect top-level `runtime`, not `agents.analytix`.
- legacy `agents.kun`, `agentProvider`, and `deepseek` settings must migrate into `runtime` when that migration is explicitly supported.
- `deepseek.autoStart=false` must not be lost if the supported legacy migration path reads it.
- legacy data migration tests must align with the final policy: new analytix uses new paths; old Kun data is imported explicitly unless a clearly scoped one-shot migration is implemented.
- bundled starlight/example plugin must no longer expose `星夜 Kun` as current product identity.

Changing tests is allowed when tests still assert old Kun architecture. Changing tests is not allowed as a way to hide broken migration, missing behavior, or removed features.

## 7. Identity And Naming Closure

Current product paths must converge on analytix:

```text
package.json
package-lock.json
packages/runtime/package.json
electron-builder.config.cjs
scripts/**
src/main/**
src/preload/**
src/renderer/**
src/shared/**
examples/**
docs/**
README.md
README.en.md
DESIGN.md
DESIGN.zh-CN.md
AGENTS.md
docs/AGENTS.md
```

Required behavior:

- app display name is lowercase `analytix`;
- installer, shortcut, tray, dock, menu, notifications, update UI, about/version surfaces use `analytix`;
- no current product path points to `KunAgent/Kun`;
- repo remains `TBD` until confirmed;
- license remains current license until changed by the user;
- author is `Guoqin He (GitHub: Eysn0130)`.

The implementation must not do a blind global replace. Every remaining old name must be classified as current-product, legacy-migration, provider/model, or historical-doc.

## 8. Documentation And Agent Instruction Closure

`AGENTS.md` and `docs/AGENTS.md` are high priority because future Codex runs read them before editing.

They must be rewritten to describe the new architecture:

```text
Electron + React + TypeScript desktop app
packages/runtime bundled runtime
window.analytix preload bridge
top-level runtime settings
analytix serve
ANALYTIX_* env
Connect Phone user-visible naming
mascot/cameo naming for retained old mascot mode
```

They must no longer instruct future agents to use:

```text
kun/
window.kunGui
agents.kun
kun serve
Kun app identity
DeepSeek-GUI current release identity
```

README, DESIGN, KUN_CONFIG, UI_PLUGINS, old architecture docs, examples, and contribution docs must either be updated to analytix or moved into a clearly marked historical/legacy area.

## 9. Settings And Migration Closure

The active settings shape is:

```ts
type AppSettingsV2 = {
  runtime: AnalytixRuntimeSettingsV1
  provider: ModelProviderSettingsV1
  write: WriteSettingsV1
  schedule: ScheduleSettingsV1
  claw: ClawSettingsV1
  // existing non-runtime top-level settings remain
}
```

Acceptance rules:

- active persisted settings contain `runtime`;
- active persisted settings do not contain `agents`;
- `AppSettingsPatch` updates `runtime`;
- UI settings pages read and write `runtime`;
- Write, Schedule, Connect Phone, SDD, Plan, usage, model picker, provider resolver, and runtime launch all read the same top-level runtime model;
- legacy reads may map old `agents.kun` into `runtime`;
- new saves must not double-write old structures.

If old Kun data migration is not automatic, current UI must provide or preserve explicit import paths. If automatic migration exists, it must be one-shot, idempotent, and safe on case-insensitive file systems.

## 10. Runtime Package And CLI Closure

Runtime source lives in:

```text
packages/runtime
```

Acceptance rules:

- no production import references `../../kun`;
- no build script uses `build:kun`;
- no package rule includes `kun/dist`;
- packaged runtime uses `packages/runtime/dist`;
- child process readiness marker uses `ANALYTIX_READY`;
- user-facing command is `analytix serve`;
- runtime process, resolver, supervisor, health, base URL, adapter, logs, and error copy use analytix naming.

If the GUI launches the runtime entry script directly with flags, that must be clearly internal and not presented as a public `kun` or alias command.

## 11. Preload And IPC Bridge Closure

`window.analytix` must become a typed stable facade. It must not remain an unbounded flat object forever.

Target domain shape:

```ts
window.analytix = {
  settings,
  runtime,
  connectPhone,
  schedule,
  workspace,
  files,
  write,
  speech,
  terminal,
  updates,
  logs,
  app,
  diagnostics
}
```

Acceptance rules:

- no `window.kunGui` exposure or fallback;
- renderer code uses `window.analytix` or a typed wrapper around it;
- IPC schemas remain zod-validated where applicable;
- runtime-generic channels such as `runtime:*` may remain when not product-branded;
- Kun-branded IPC channels are renamed to analytix or legacy-kun import channels;
- `claw:*` internal channels may remain, but user-facing labels are `Connect Phone`.

## 12. Thread Virtualizer And Scroll Closure

The chat middle area must use a production-connected Analytix-derived bottom-distance virtualizer.

Required fixes:

- virtualizer total height must match actual DOM height;
- row gap must be modeled explicitly or removed from the virtualized flex gap path;
- spacer nodes must not be affected by unmodeled `gap-8` layout;
- measured row cache must be updated through batched `ResizeObserver`;
- measurement changes must preserve bottom distance when appropriate;
- user at bottom follows streaming smoothly;
- user reading history is not pulled to bottom by new tokens;
- history prepend preserves visual offset;
- jump rail, load earlier, empty hero, fork banner, review/tool/approval/user-input/process/generated files rows keep working.

Required tests:

- virtual window with row gap or no-gap DOM equivalence;
- bottom-distance anchor;
- measured row cache update;
- ResizeObserver batch behavior;
- history prepend preservation;
- streaming active turn while near bottom;
- streaming active turn while user is away from bottom.

## 13. Streaming, Markdown, And Tracing Closure

Streaming must stay on a narrow hot path.

Acceptance rules:

- text deltas are buffered and flushed at most once per animation frame;
- structural events still flush promptly;
- live streaming row updates do not force unrelated panels to render;
- streaming assistant markdown uses a lightweight text surface;
- finalized assistant markdown enters `MarkdownFinalizationQueue`;
- rich markdown and syntax highlighting run after finalization or idle budget;
- large code blocks do not re-highlight on every token.

Tracing must be local only:

```text
Electron userData/traces/thread-*.jsonl
```

Required event names:

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

Trace records must avoid API keys, tokens, full prompt text, full model output, and direct personal contact/payment identifiers. Use sizes, counts, durations, ids, row kinds, and status values.

## 14. Workbench And Island Closure

Workbench must not be the streaming bottleneck.

Acceptance rules:

- top-level Workbench should not subscribe to `blocks` merely to feed heavy derived panels;
- ChangeInspector, DevBrowser, composer summary, terminal, Write, SDD, side panel, preview, and settings should subscribe through narrow islands/selectors;
- Terminal output uses its own scheduler;
- Write editor and SDD draft state are isolated from chat streaming;
- composer controls remain feature-complete and keep current behavior;
- no feature is deleted to reduce renders.

## 15. Brand, Asset, And Visual Closure

Brand source is `/Users/sun/Downloads/analytix-logo`.

Required project assets:

```text
src/asset/brand/analytix-logo-system-readme.md
src/asset/brand/analytix-logo.png
src/asset/brand/analytix-logo-transparent.png
src/asset/brand/analytix-symbol-color.png
src/asset/brand/analytix-symbol-mono-black.png
src/asset/brand/analytix-symbol-mono-white.png
src/asset/brand/analytix-symbol-reversed.png
src/asset/brand/analytix-app-icon-16.png
src/asset/brand/analytix-app-icon-32.png
src/asset/brand/analytix-app-icon-64.png
src/asset/brand/analytix-app-icon-128.png
src/asset/brand/analytix-app-icon-256.png
src/asset/brand/analytix-app-icon-512.png
src/asset/brand/analytix-app-icon-1024.png
src/asset/brand/analytix-splash.png
src/asset/brand/provenance.json
build/icon.ico
build/icon.icns
```

Acceptance rules:

- referenced brand assets are tracked;
- app icon, dock icon, tray icon, installer icon, splash, and empty states use analytix identity;
- Codex logo is not exposed as analytix logo;
- old Kun app icon is not used as current icon;
- retained old mascot/illustration assets use `mascot` / `cameo` source naming when they are part of current code;
- UI plugin example says analytix, not Kun;
- visual tokens exist for shadow, border, radius, motion, and focus where they affect repeated surfaces;
- chat scroll hot path avoids heavy blur, heavy shadow, and layout-triggering transitions.

## 16. Release And Packaging Closure

Acceptance rules:

- `electron-builder.config.cjs` uses `com.analytix.desktop`;
- artifact name begins with `analytix-`;
- publish URL is analytix channel URL;
- only `stable` and `beta` are accepted;
- `frontier` is removed;
- `KUN_*` and `DEEPSEEK_GUI_*` fallback envs are removed;
- Windows NSIS x64 remains the first Windows target;
- macOS dmg + zip for arm64 and x64 remain configured;
- Linux AppImage x64 remains configured;
- local release scripts use analytix naming.

Historical first-stage config used a documented placeholder release domain.
Current configs instead default to
`https://analytix.top/desktop/releases/standard-win/`; release closure now
requires live feed/metadata, signing, provenance, and platform proof rather
than placeholder removal alone.

## 17. Verification Commands

The final implementation must run:

```bash
npm run test
npm run typecheck
npm run build
npm run lint
git diff --check
```

It must also run naming scans. Recommended scans:

```bash
rg -n "\bKun\b|kunGui|agents\.kun|agents\.analytix|KUN_|DEEPSEEK_GUI_|deepseek-gui|KunAgent/Kun|Codex-derived|codex-derived|iKun|ikun" \
  --glob '!node_modules/**' \
  --glob '!dist/**' \
  --glob '!out/**'

rg -n "kun serve|build:kun|kun/dist|../../kun|window\.kunGui" \
  package.json package-lock.json electron-builder.config.cjs scripts src packages docs examples README.md README.en.md DESIGN.md DESIGN.zh-CN.md AGENTS.md
```

Any remaining hit must be listed in the final report with one of these classifications:

```text
current-product bug
legacy-kun migration/import/test fixture allowed
provider/model name allowed
historical doc isolated
third-party/vendor path allowed
```

Current-product bugs must be fixed before claiming closure.

## 18. Completion Matrix

Final report must include this matrix:

| Spec | Status | Evidence | Remaining Risk |
| --- | --- | --- | --- |
| 01 identity/runtime/settings | complete/partial/blocked | tests, files, scans | risk |
| 02 release/packaging/channels | complete/partial/blocked | tests, files, scans | risk |
| 03 brand/visual/assets | complete/partial/blocked | assets, files, screenshots if run | risk |
| 04 thread virtualizer/smoothness | complete/partial/blocked | tests, code paths, trace | risk |
| 05 architecture inventory | complete/partial/blocked | changed modules | risk |
| 06 closure/acceptance | complete/partial/blocked | command results | risk |

The final report must explicitly say whether the loop is complete. Do not call it complete if tests fail, build fails, naming scans have current-product bugs, or production paths still only contain unused helper modules.

## 19. Work Discipline

Implementation must:

- not touch `.agents/`;
- not commit git changes unless the user asks;
- not delete user changes;
- not use destructive git commands;
- prefer scoped edits;
- update tests alongside code when architecture expectations changed;
- keep existing features and UI paths working;
- use the existing stack and patterns unless the spec explicitly requires a boundary change;
- record validation commands and outcomes.

## 20. Not Complete Conditions

The upgrade is not complete if any of the following are true:

- `npm run test` fails;
- `npm run typecheck` fails;
- `npm run build` fails;
- `git diff --check` fails;
- current product docs still tell future agents to use Kun architecture;
- `window.kunGui` remains exposed;
- active settings save `agents.kun` or `agents.analytix`;
- virtualizer modules exist but production scroll still relies only on old sentinel/scrollHeight behavior;
- tracing types exist but no local JSONL trace is written;
- markdown finalization queue exists but is not used in production;
- referenced brand assets are untracked;
- app icon/tray/dock/installer still use old Kun assets;
- remaining old naming hits are not classified.

## 21. Recommended Execution Order

Use this order for the next implementation pass:

```text
1. Re-run tests and inspect current failures.
2. Fix P0 tests and code/test expectation mismatches.
3. Update AGENTS, README, DESIGN, docs, examples, and current-product instructions.
4. Close settings/runtime/CLI/packaging naming gaps.
5. Refactor window.analytix into domain facade without breaking renderer calls.
6. Fix virtualizer DOM height equivalence and connect bottom-distance scroll layout.
7. Connect markdown finalization and local tracing to production paths.
8. Reduce Workbench hot-path subscriptions through islands/selectors.
9. Track and verify brand assets.
10. Run full verification commands and produce the completion matrix.
```

Do not start with broad visual polish while P0 tests, current-product docs, or scroll/tracing production wiring remain incomplete.

## 22. Current Closure Decisions And Allowlist

This section records the implementation closure decisions for the 2026-06-18
pass. It is part of the acceptance contract and supersedes stale Kun wording in
older project-state documents.

### 22.1 Bridge Closure

`window.analytix` is a domain facade. Renderer code must call only these
domains:

```text
settings
account
runtime
connectPhone
schedule
workspace
files
write
speech
terminal
backgroundTasks
updates
logs
app
diagnostics
dataAnalysis
```

The preload may keep an internal IPC map to build those domains, but it must
not expose flat `window.analytix.xxx()` methods to renderer code. The required
scan is:

```bash
rg -n 'window\.analytix\.(?!settings|account|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|backgroundTasks|updates|logs|app|diagnostics|dataAnalysis)\b|window\.analytix\?\.(?!settings|account|runtime|connectPhone|schedule|workspace|files|write|speech|terminal|backgroundTasks|updates|logs|app|diagnostics|dataAnalysis)\b' \
  src/renderer/src src/preload src/shared --pcre2
```

Expected result: no matches.

### 22.2 Legacy Naming Allowlist

Remaining old-name hits are allowed only in these files/classes of files:

| Path | Allowed old terms | Reason | Future cleanup |
| --- | --- | --- | --- |
| `docs/analytix/specs/` specs `01` through `10` | Kun, `kun serve`, `agents.kun`, `window.kunGui`, old release identifiers, upstream source names | These specs describe the reset from the old architecture, desktop acceptance, upstream absorption, benchmark policy, control-plane evolution, and forbidden current-product terms. | Keep while specs explain migration history and active policy; use each file's lifecycle/currentness note. |
| `docs/analytix/upstreams/` | Kun, Reasonix, CodexDesktop-Rebuild, upstream version/commit names | Required sync ledgers and conflict decisions for future upstream absorption. | Keep while upstream absorption remains active. |
| `docs/analytix/benchmarks/` | Kun, Reasonix, upstream comparison terms | Required benchmark scenarios, quality gates, and scorecards for proving analytix is stronger after upstream absorption. | Keep while benchmark-based release claims remain active. |
| `docs/legacy/kun/` | Kun and `kun` paths | Historical legacy-kun notes with explicit banner; retained only for migration background. | Remove after migration support is retired. |
| `release/legacy/release-v0.2.*.md` | DeepSeek GUI, Kun, legacy release env/commands | Historical release archaeology for pre-analytix versions. | Keep archived; never link as current release docs. |
| `src/main/legacy-data-migration.ts` and `.test.ts` | `Kun`, `DeepSeek GUI` directory names | Exact legacy userData names required to find old installs. | Remove after the legacy import/migration window ends. |
| `src/main/settings-store.ts` | `DeepSeek GUI` userData fallback | Fallback read for legacy settings when directory migration was skipped. | Remove with legacy migration cleanup. |
| `src/renderer/src/plan/plan-prompts.ts` and plan prompt tests | `DeepSeek GUI is asking...` | Recognition of old stored internal GUI plan prompts so they do not become user requests. | Remove after stored legacy thread prompts are rewritten or no longer supported. |
| provider/model files and tests | DeepSeek | Provider/model/protocol name, not product identity. | Keep as long as DeepSeek provider support exists. |
| Connect Phone implementation files | `claw` | Internal module name allowed by spec; user-visible text must say Connect Phone/analytix. | Rename in a later isolated Connect Phone internals pass if desired. |

Current product paths, UI strings, examples, release configs, package metadata,
runtime CLI, bridge API, settings schema, and assets must not use those terms
outside the allowlist.

### 22.3 Smoothness Evidence

The thread hot path must have production wiring, not only helper modules:

- `MessageTimeline` builds stable projection rows before rendering.
- `useTimelineScroll` owns bottom-distance anchoring through
  `ThreadScrollLayout`.
- prepend preservation, streaming snap, and thread reset corrections record
  `thread.scroll.anchor_corrected`.
- `MessageTimeline` records `thread.react.commit_sample` through React
  Profiler with durations and row counts only.
- ResizeObserver batching records `thread.virtualizer.measured`.
- finalized markdown records `thread.markdown.finalized`.

Trace payloads must contain counts, durations, booleans, and sizes only. They
must not contain prompts, assistant text, filenames, or other private content.

### 22.4 Required Final Verification

Before closure, run:

```bash
npm run test
npm run typecheck
npm run build
npm run lint
git diff --check
git ls-files --others --exclude-standard
```

Also run bridge and naming scans, then classify every remaining hit against the
allowlist above. If any hit is a current-product bug, closure is blocked until
it is fixed.

## 23. Third-Round Structure And Smoothness Closure

This section records the 2026-06-18 third-round closure pass on top of
`3921dd9 feat: complete analytix architecture closure`.

### 23.1 Historical Docs Isolation

Historical legacy-kun notes have been moved out of the docs root:

```text
docs/legacy/kun/
```

The docs root no longer contains `docs/kun-architecture*.md`,
`docs/kun-cache-optimization*.md`, `docs/kun-contributing*.md`, or
`docs/kun-hooks*.md`. `docs/legacy/kun/README.md` marks them as historical
migration background only. README documentation maps may link to that index as
legacy background, but current architecture instructions remain `AGENTS.md`,
`docs/AGENTS*.md`, and `docs/analytix/specs/` specs `01` through `10`.

### 23.2 Workbench Hot-Path Evidence

Third-round renderer changes:

- Workbench no longer subscribes to `blocks` only to feed composer change
  summary; that subscription is isolated in `FloatingComposerIsland`.
- `WriteWorkspaceView`, `WriteAssistantPanel`, `WriteSidebar`,
  `SddAssistantPanel`, and `SddDraftEditorView` are lazy islands behind
  Suspense boundaries.
- `MarkdownFinalizationQueue` now has a shared
  `MarkdownFinalizationScheduler` with a frame/idle budget. Streaming rows keep
  the lightweight text surface; rich markdown is enabled after finalization.
- Repeated heavy shadows in Terminal and Write empty-state surfaces use
  `--ax-shadow-*` visual tokens.

Bundle evidence from `npm run build`:

| Build point | Workbench chunk | Notes |
| --- | ---: | --- |
| baseline at `3921dd9` | `4,727.51 kB` | Write/SDD panels were still inside the Workbench route chunk. |
| third-round closure | `1,354.50 kB` | Write/SDD/editor work moved to lazy chunks. |

New lazy chunks include `WriteWorkspaceView` (`911.01 kB`),
`WriteMarkdownEditor` (`2,236.17 kB`), `WriteAssistantPanel` (`14.31 kB`),
`SddAssistantPanel` (`15.01 kB`), `SddDraftEditorView` (`41.77 kB`), and
`WriteSidebar` (`32.74 kB`). This is an intentional route/island split, not a
feature removal.

### 23.3 Long-Thread Smoke Evidence

`src/renderer/src/thread/thread-long-thread-smoke.test.ts` constructs a
120-turn fixture with long markdown/code blocks, tools, approvals, user-input
requests, file-change blocks, review blocks, live streaming text, virtualized
windowing, bottom-distance resize anchoring, history prepend offset
preservation, and trace sanitization.

The smoke test verifies:

- stable projection produces 120 turns and jump anchors;
- the latest turn carries live streaming state;
- virtualizer output is a bounded window, with row gap included in total height;
- near-bottom resize preserves bottom distance;
- away-from-bottom resize does not force-scroll the user;
- prepend preserves visual position;
- trace JSONL payloads keep numeric/boolean fields and drop prompt/output text.

This historical renderer-smoke row is diagnostic only. It no longer substitutes
for formal packaged public-seam evidence: the independent Milestone A/B gates
and non-substitution rules in
`07-desktop-qa-release-readiness.md` govern current acceptance.

### 23.4 Release And Remote Safety

Release configuration remains:

- `appId`: `com.analytix.desktop`
- artifact pattern: `analytix-${version}-${os}-${arch}.${ext}`
- channels: `stable` / `beta`
- env prefix: `ANALYTIX_*`
- Windows target: NSIS x64
- macOS targets: dmg + zip for arm64/x64
- Linux target: AppImage x64

Historical closure used `https://release-domain.invalid/analytix` as an
intentional placeholder. Current packaging no longer uses that placeholder;
it defaults to `https://analytix.top/desktop/releases/standard-win/`. This only
closes the configuration-placeholder gap. Reachability, update metadata,
signing, provenance, and platform QA remain release-time evidence requirements.

analytix is maintained as a local project. Do not push from this worktree as
part of the default release process; local release provenance and configured
distribution endpoints are the current authority.
