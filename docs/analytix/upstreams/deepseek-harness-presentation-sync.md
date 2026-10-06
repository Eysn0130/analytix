# DeepSeek Harness presentation intake

Status: Reference / scoped port-and-adapt record.
Scope: renderer presentation stage 1, plus the bounded protected-local settled Shell/Read candidate described below. Public incremental/tool DTO stage 2 remains design only.
Current base: Analytix `09003e3f185bd0255cb227a9f96f1cc03b683fdd`.
Source: official `deepseek-ai/deepseek-harness`, release `dsh-v0.2.1-alpha.1`,
commit `5badb15009ae1756c3afe0ae0cef1faafc290ccc`. Currentness and parity are not claimed.

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"deepseek-harness","reviewedCommit":"5badb15009ae1756c3afe0ae0cef1faafc290ccc","parity":"not-proven","capabilityBenchmarkV1":null} -->

The user's current delegated stage-1 instruction authorizes these renderer
changes in the isolated candidate worktree. It does not authorize publication,
release, a second runtime, or changes to Go/main/preload/public schemas.
Disposition: `candidate`; focused renderer validation is recorded in the private
review package. Coordinator integration and wider acceptance are pending.
Reviewer: coordinating DSH presentation agent, with two read-only source/UI
reviewers. Source inspection does not establish native or live-provider QA.

## License and exact source binding

Root MIT license blob `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`,
SHA-256 `ebb4f09972aee8608be255debaf78451a68e95c290f55c240dec2ecfa16ea6be`,
copyright (c) 2026 DeepSeek. Official commit/tree and downloaded file objects
were independently bound to the listed Git blobs. Complete MIT terms are
retained in `THIRD_PARTY_NOTICES.md`. No gitlink, vendored third-party code,
font, or image asset is admitted by this row. Existing KaTeX font obligations
and existing admitted icon notices continue to apply independently.

| Source path | Pinned Git blob | Analytix destination |
| --- | --- | --- |
| `packages/client/ui-primitives/src/markdown/MarkdownText.tsx` | `ab2c0b380adae9843c6584794837be71275a9643` | `src/renderer/src/components/chat/presentation/markdown/MarkdownText.tsx` |
| `packages/client/ui-primitives/src/markdown/MarkdownText.module.css` | `d54374106a35283c468d09763cd35dfd43db6a29` | `src/renderer/src/components/chat/presentation/markdown/MarkdownText.module.css` |
| `packages/client/ui-primitives/src/markdown/incremental.ts` | `dc216e51552911d8a7afb51607e4bdc7546f0322` | `src/renderer/src/components/chat/presentation/markdown/incremental.ts` |
| `packages/client/ui-primitives/src/markdown/parse.ts` | `46559c3f47f46edc5222f1fb0518e971a96fc480` | `src/renderer/src/components/chat/presentation/markdown/parse.ts` |
| `packages/client/ui-primitives/src/markdown/render.tsx` | `cdb9afcbdad8f2e2d6f73834ee9ded97560bfc6d` | `src/renderer/src/components/chat/presentation/markdown/render.tsx` |
| `packages/client/ui-primitives/src/markdown/highlight.ts` | `0ae164d138036090c7d988e5fdbeac09699f3a9d` | `src/renderer/src/components/chat/presentation/markdown/highlight.ts` |
| `packages/client/ui-primitives/src/markdown/katex.tsx` | `45c1226b833dae0db4bebbde1ee14c93e11d66a6` | `src/renderer/src/components/chat/presentation/markdown/katex.tsx` |
| `packages/client/ui-primitives/src/markdown/cjkFriendlyStrong.ts` | `aba740d058ebea7d1bbd9fa2009cc80f2f9c13f2` | `src/renderer/src/components/chat/presentation/markdown/cjkFriendlyStrong.ts` |
| `packages/client/ui-primitives/src/markdown/mathCompatibility.ts` | `3edd9d1e6311824572be9819eb2bb1cc2ef0b1a6` | `src/renderer/src/components/chat/presentation/markdown/mathCompatibility.ts` |
| `packages/client/ui-primitives/src/markdown/file-link.ts` | `bb1ca89b0bdf9696fc4992e63ba2f8797970fd9f` | `src/renderer/src/components/chat/presentation/markdown/file-link.ts` |
| `packages/client/ui-primitives/src/markdown/local-image-syntax.ts` | `c59a3239230daaa2171fe08592d3fa3da0b8abfe` | `src/renderer/src/components/chat/presentation/markdown/local-image-syntax.ts` |
| `packages/client/ui-primitives/src/markdown/MarkdownDelegate.tsx` | `681e8f56088f4d8040f69fca2a04437d2d04c0f1` | `src/renderer/src/components/chat/presentation/markdown/MarkdownDelegate.tsx` |
| `packages/client/ui-primitives/src/markdown/CodeBlock.tsx` | `51d175393d2a95ad59a15e0bf7dfc8e05a03ad24` | `src/renderer/src/components/chat/presentation/markdown/CodeBlock.tsx` |
| `packages/client/ui-primitives/src/markdown/CodeBlock.module.css` | `b59bef283d2b8a06e8b178a2cc6efe4727206fa1` | `src/renderer/src/components/chat/presentation/markdown/CodeBlock.module.css` |
| `packages/client/ui-primitives/src/markdown/useViewportHighlighting.ts` | `2f4114ce71ac084e6a174908bc81c29500711480` | `src/renderer/src/components/chat/presentation/markdown/useViewportHighlighting.ts` |
| `packages/client/ui-primitives/src/DisclosureRow.tsx` | `7b98eca2fcd72c0a41331670f7d5023849979781` | `src/renderer/src/components/chat/presentation/DisclosureRow.tsx` |
| `packages/client/ui-primitives/src/DisclosureRow.module.css` | `90fc1f8c2dc88a33035866076ae5f6774d547ca8` | `src/renderer/src/components/chat/presentation/DisclosureRow.module.css` |
| `packages/client/ui-primitives/src/TextShimmer.tsx` | `020073a8ae0fd968d571ccb65e70f184eab8671c` | `src/renderer/src/components/chat/presentation/TextShimmer.tsx` |
| `packages/client/ui-primitives/src/TextShimmer.module.css` | `5131ecd6539c4c9c8f8909e0e4fd88482baac780` | `src/renderer/src/components/chat/presentation/TextShimmer.module.css` |
| `packages/client/ui-primitives/src/CodeToolbar.tsx` | `897d4cd1f7846a1912f6099f9cb6f360fb1bc779` | `src/renderer/src/components/chat/presentation/CodeToolbar.tsx` |
| `packages/client/ui-primitives/src/CodeCard.module.css` | `359002d14ff847d17920d53f7ff46f97dba91d5b` | `src/renderer/src/components/chat/presentation/CodeCard.module.css` |
| `packages/client/ui-primitives/src/Tooltip.tsx` | `6f32ecab9071ec8adf81e547cea4103eb3a59ae3` | `src/renderer/src/components/chat/presentation/Tooltip.tsx` |
| `packages/client/ui-primitives/src/Tooltip.module.css` | `d52b752f87ab9158dde1135934c0c7fcd2b05d60` | `src/renderer/src/components/chat/presentation/Tooltip.module.css` |
| `packages/client/ui-primitives/src/HoverCard.tsx` | `4aa1b586520634759b3a752ae823134480a7ce85` | `src/renderer/src/components/chat/presentation/HoverCard.tsx` |
| `packages/client/ui-primitives/src/HoverCard.module.css` | `50c62fa8289c4e4de5d7e6d5cd71f050d5c1a29e` | `src/renderer/src/components/chat/presentation/HoverCard.module.css` |
| `packages/client/ui-primitives/src/pointer-grace.ts` | `4b91a84b0a01f13d86d75618a25f2ce513966d8c` | `src/renderer/src/components/chat/presentation/pointer-grace.ts` |
| `packages/client/ui-primitives/src/input-modality.ts` | `d6b1488f36f1a130b0fdda6ed4b708e305f61680` | `src/renderer/src/components/chat/presentation/input-modality.ts` |
| `packages/client/ui-primitives/src/useDismissOnOutsidePointer.ts` | `2f16f43476d3821ac142edf86d53751d0942d72a` | `src/renderer/src/components/chat/presentation/useDismissOnOutsidePointer.ts` |
| `packages/client/ui-primitives/src/ShortcutKeys.tsx` | `33c91db0aa7ec9a0b70131b3183e57b0d164780e` | `src/renderer/src/components/chat/presentation/ShortcutKeys.tsx` |
| `packages/client/ui-primitives/src/ShortcutKeys.module.css` | `10afdefb533c17ec4396dd97c62b4af45aad3c79` | `src/renderer/src/components/chat/presentation/ShortcutKeys.module.css` |
| `packages/client/ui-primitives/src/clipboard.ts` | `aa45f4c76d197532bac80a53a7aaaa518f2857ac` | `src/renderer/src/components/chat/presentation/clipboard.ts` |
| `packages/client/ui-theme/src/styles/shiki.css` | `c7a3c5d27219d146d965d0fb37fcc0445d32b089` | `src/renderer/src/components/chat/presentation/shiki.css` |
| `packages/client/ui-primitives/src/overlay-top-margin.ts` | `02ed6792808a0ac4048f8cba7ccaa7a080f3288a` | `src/renderer/src/components/chat/presentation/overlay-top-margin.ts` |
| `packages/client/ui-primitives/src/ImagePreview.tsx` | `1875a151c864e7e52885e4bbf31e2a26ee5f4386` | `src/renderer/src/components/chat/presentation/ImagePreview.tsx` |
| `packages/client/ui-primitives/src/ImagePreview.module.css` | `a12f77ec4767a1d3ad3d7aeb87ac3e3b4562762a` | `src/renderer/src/components/chat/presentation/ImagePreview.module.css` |

## Modifications and excluded closure

- Adapt local imports and React 19 refs/inert; preserve source-offset keys,
  incremental parse generation resets and settled reference/footnote repair.
- Use Analytix system-font, color, focus, motion and light/dark token owners.
- Replace DSH glyphs/artwork with the existing admitted 139-icon facade.
- Files/media use existing workspace validation, image read, preview/editor
  callbacks and modal focus; no DSH file scheme or unvalidated local path.
- Preserve code-source copy/download, bounded large-content fallback and
  actual timeline measurement/virtualization/scroll anchoring.
- Controlled disclosure consumes existing public chat blocks; no DSH host,
  conversation/session store, raw JSON inspector, engine or private reasoning.
- Rich shell/read/diff/search/web output is unavailable under current public
  ToolResultPublicProjectionV1. Its DTO seam belongs to stage 2, not fixtures
  presented as live functionality.

Validation owner: focused Markdown/parser/code/disclosure/media/settings
regressions, existing timeline/gates/scroll tests, TypeScript checks and one
renderer build. Synthetic screenshots bind candidate files and viewport;
packaged/native/provider acceptance remains outside this stage.

## Candidate closure and compatibility

The single production presentation route is `AssistantMarkdown` → `DshAssistant`
→ the admitted `MarkdownText` leaf. The old central Streamdown owners are retired;
existing file-preview highlighting, timeline/store/public stream authority,
virtualizer/measurement/scroll, all real business callbacks and 139 admitted icon
sources remain. Three actual process headers use controlled `DisclosureRow`.
Interactive summary/diagnostic/file slots render once outside decorative shimmer.
Pets and pet-only hooks/styles/settings navigation are removed; actual elapsed
time and declarative theme plugin install/activate/remove callbacks remain.

The 16 approved direct declarations keep existing Shiki/grammar `3.23.0` and
KaTeX `0.16.46`, versus upstream Shiki `4.3.1` and KaTeX `0.16.47`. The lock adds
exactly mdast-util-math `3.0.0`, micromark-extension-math `3.1.0`,
unist-util-remove-position `5.0.0` and @types/katex `0.16.8`; old entries do not
drift. Dependency-only `npm ci --ignore-scripts` runs in an isolated cache tree.
Application/native postinstall acceptance is not claimed. Existing full KaTeX
OFL font terms and Reserved Font Names remain in the packaged notice.

Adaptations include React 19 refs/inert, existing icons/system-font/token aliases,
grapheme pacing and heavyweight fallback, 12-line code collapse, source-text
copy/download, viewport/lazy highlighter with bounded fallback and exception
rollback, `trust:false`/bounded KaTeX, existing checked file navigation (including
line/column and double-click editor), raster-only host images including AVIF,
HTTP(S)/mailto navigation, modal focus lifecycle and selectable public text.
Remote images have no invented cross-origin download capability.

See [all 67 dispositions](deepseek-harness-stage1-dispositions.md) and the
[proposed public producer plan](deepseek-harness-public-projection-plan.md).
Final source-bound validation and synthetic screenshot evidence are recorded in
the task's private delivery bundle. Native, live-provider and full upstream
feature/pixel parity remain unproven. Coordinator integration is pending.

The [67-function disposition record](deepseek-harness-stage1-dispositions.md)
and [proposed public projection plan](deepseek-harness-public-projection-plan.md)
separate retained product behavior from stage-1 adaptation and future producer gaps.

The local table adaptation uses `markdown/MarkdownTable.tsx` and `table-data.ts`
around the existing parsed, sanitized table. These are local action owners, not
additional pinned upstream leaves. Copy offers CSV/TSV/Markdown; download offers
CSV/Markdown; fullscreen retains the original table node and scroll position.
Exports use public cell labels and math source. Spreadsheet text prefixes and
control-character escaping change the representation; arbitrary GFM round-trip
or universal spreadsheet safety is not claimed. The existing modal focus owner
handles nested layers, opener restoration, and opt-in body scroll locks.

SessionHeader owns initial menu focus, same-level arrows, nested Escape, native
Tab exit and its original callbacks/gates. A gate disabling the focused action
closes its invalid submenu and returns to an enabled root action; outside and
callback-owned focus retain precedence. ImagePreviewLightbox uses natural
dimensions × viewport fit × zoom, preserving callbacks and resetting on src or
reopen. Function evidence remains source-bound in the private review bundle.

The unused Streamdown root edge/scan and its unproduced table CSS are retired by
normal lock generation. Shared Tiptap `marked`, ReactMarkdown and Shiki consumers
remain. The dead local ImageLightbox wrapper is retired; its optional label shape
is inline in MarkdownDelegate. Existing media consumers remain authoritative.
Four unused mascot locale keys are removed while legacy preference compatibility
remains. Icon provenance follows current consumers; the 139 admitted SVGs and
their license/provenance identities are unchanged. The historical 35-source
intake and 67 disposition rows do not establish 67 fresh acceptance results.

## Protected-local settled Shell/Read candidate

The separately approved bounded task starts from Analytix
`c7d2a370855a8d371c61c32cc2422515b9404ef2`. It adds the following
renderer-only MIT adaptations. The stage-1 source rows above keep their
recorded base and evidence. This candidate is uncommitted. Local affected-owner
Go/TypeScript checks, the production exact-owner checks and source build have
passed. Canonical integration, native UI acceptance and current CI remain
pending; local source evidence does not establish release or parity.

| Exact upstream path at the pinned commit | Git blob | Local destination |
| --- | --- | --- |
| `packages/client/ui-primitives/src/TerminalBlock.tsx` | `76e7f92808f3783f68b1b6da27b98895ad6a732d` | `src/renderer/src/components/chat/presentation/TerminalBlock.tsx` |
| `packages/client/ui-primitives/src/TerminalBlock.module.css` | `c4d4ddda1a7a901bae644120152ebff17c578113` | `src/renderer/src/components/chat/presentation/TerminalBlock.module.css` |
| `packages/client/ui-primitives/src/ReadBlock.tsx` | `0eaf96374f7689ed9b7aae68df9bedaa8e716ddf` | `src/renderer/src/components/chat/presentation/ReadBlock.tsx` |
| `packages/client/ui-primitives/src/ReadBlock.module.css` | `601bb29f48e11edebaed068488c0db71aabcf883` | `src/renderer/src/components/chat/presentation/ReadBlock.module.css` |
| `packages/client/ui-primitives/src/ansi.ts` | `2b5ae35a5dca178ad214c853dbd9cc9b2425b011` | `src/renderer/src/components/chat/presentation/ansi.ts` |
| `packages/client/ui-primitives/src/head-tail-cap.ts` | `195a1c98bebebbfe69b8be9bc1cc0b57b5c1e7a6` | `src/renderer/src/components/chat/presentation/head-tail-cap.ts` |

Task source research verified the official commit/tag, complete source tree
and six source blobs; two read-only reviewers checked the integration boundaries. The ui-primitives
package declares MIT; its selected tree has no gitlink, nested license
exception, or selected font/image asset. Root MIT terms and copyright remain
in `THIRD_PARTY_NOTICES.md` and every adapted file header. Existing React,
clsx and Shiki dependencies retain their existing notices; no dependency was
added. Upstream `anser` is replaced by an Analytix-owned bounded numeric SGR
resolver around the admitted cursor logic. Raw clipboard hooks, inferred
success, running deltas and DSH runtime/session authority are excluded.

Actual producer capture, private CAS, authorization and settlement are
Analytix Go owners. Main mediates fresh reads and ordinary Write masking for
copy/save. The effect removes SGR decoration before masking split tokens;
other terminal control languages are unsupported for copy/save. The lazy
renderer payload does not become a public projection,
Markdown transcript, model history, execution argument or second runtime.
The initial body cap is 32 KiB UTF-8; folding and ANSI/highlight budgets do not
alter the frozen copy/save source. Preview, Search/Diff/Web and source-exact
case export are outside this candidate. Official upstream testing/pre-push
guides were inspected as reference; upstream CI was not run or claimed green.
