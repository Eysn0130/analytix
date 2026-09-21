# PR28 independent-review continuation — 2026-09-21

Status: Historical execution receipt, not product/release acceptance.
Scope: user-adopted independent-review continuation A–I, retaining the original A–M/P0–P5 outcome.
Repository and source writer remain the canonical Analytix checkout and original Codex owner.
No Goal, scheduled continuation, new independent task, public release or writer transfer was used.

## Identity and intake

START HEAD `fe65622e5d827637f6a7423ef60dc048fee20d3c`, tree
`2af1390bc927a86b82a50090a73f2cb9a1a1168d`, clean, original branch
`codex/workbench-product-delivery-20260914`. The verified ancestry retains
`148d188e4f7df71e1cf28bc25bfff988365fa1ea`, `5c2f1b089533b70e3a7623396e9690d2d355aab4`
and `2ef3f9deb22714b7c05583603bf1e394cce1f06e`. No history was rewritten.
The source commit containing this receipt is the new checkpoint; the delivery
manifest records its full HEAD/tree and final dirty state after commit, avoiding
an impossible self-referential commit hash inside this file.

The uploaded `Analytix-PR28-Independent-Review-2026-09-21.zip` SHA256 is
`2adc4a541620bb12cea669fa7ae1d6e311b462a3f92f22e2e7b4e16b66f19742`.
Safe exclusive extraction checked paths, duplicate/case-fold names, types,
symlinks, encrypted entries, sizes/ratios and CRC: 55 files, 8,562,664 expanded bytes.
Outer SHA256SUMS: 54/54; nested reference/input manifest: 30/30. Markdown/text
counterparts match. The 00–07 reports, 67 results, A–I and original A–M/reference
matrix/receipts were read. The attachment contains reports and prior public
analysis, not a source patch or a writer/safety-clearance receipt.

Reviewer tooling was rerun separately: 13 self-tests, 67 results, 248 retained
flows, 16,287 normalized locations, 1,184 ignored-flow warning, 26 Python floating
point vectors. None is counted as a product test or fresh CodeQL analysis.

## Current review findings and actual changes

| Item | Current action and evidence | Limit |
| --- | --- | --- |
| D1 / 67 old findings | New per-finding mapping binds final source SHA/tree/dirty, current blobs/lines, old fingerprints and old source/sink/flow provenance. Changed old locations remain CHANGED/null. | Old analysis 1792355564, CodeQL 2.27.0, dde; no baselineState, no new scan or dismissal. 67 is not the old PR-level 37-high summary. |
| D2 / 29 numeric | Retained all nine 148d production owners; eight existing tests add boundary/nextafter, negative-zero, null/absent, exact json.Number and owner-specific fallback coverage. 13 unique focused top-level tests passed. | Raw historical JSONL/hash/counts verified: 995/1783 and prod12 remain evidence for their original unchanged numeric surface, not a whole-new-candidate suite. 25 fixes, prior57/58, bounded74/75 remain separate. No 32-bit or zero-match prod SSE claim. |
| D3 / 145–146 | Reproduced stale native-document delivery in Office and Canvas. Added Main-owned irreversible document leases; main navigation/crash/destroy revoke replies, pushes, picker/menu and freeze ACK. Office hides but retains pending notes/save state; explicit successful open reattaches. Canvas awaits old disposal before reusing a Core session. | Does not independently close both old filestore findings or provide native GUI evidence. Existing manual local-display versus model thread/principal/projector guards remain. |
| D4 / 129–134,138 | Real Manager tests count source reads: unissued and cross-operation read=0; consumed replay adds zero reads; valid finalize/rollback read once and succeed. | Synthetic source files only. Source authority not redesigned. |
| D4 / 32,137 | Checked actual openat/unix.Open before descriptor wrapping, root/digest locator/generation and file identity guards. Six existing private-CAS regressions passed. | NewFile name does not reopen. Digest locator is not a claim that this layer hashes the body. No all-caller dismissal. |
| D4 / 31,140,141,33 | Distinguished attachment thread/workspace authority, case binding, local-display and metadata probes. WorkspaceStatus still Stat-before-CommandProbe. | No demonstrated low-trust protected metadata exposure; authority/caller review remains open, no speculative path-wide rewrite. |
| D5 / 14 hash | Source formats ProviderID, not APIKey. New actual missing-key configuration→Core failure→durable history/SSE→store reopen/recovery→next model canary passes; private identity, error, credential marker and direct canary digests absent. | 2 unique top-level/4 including subtests. Synthetic Provider transport, store reopen rather than process restart, no production Registry credential or fresh scan. SHA256/signature/domain protocols unchanged. |
| D6 / 9 allocation | Per-sink budgets recorded. Tail-turn capacity exactly3, closed steering map capacity≤35; materialized-data plus small-domain arithmetic reviewed on darwin/arm64. | Resource exhaustion and incomplete aggregate input budgets remain separate. No huge fabricated slice, no 32-bit extrapolation, no unsupported KDF substitution. |
| Rust | Verified identical analysis_compute tree `8338a25502e261b8b219a5b75f0bc508772e402c` across ce96 failure and 9990 test-merge success; retained 3000/10000 ascending and 13000 total fixture semantics. | Same recorded Rust1.94.1, DuckDB1.10504.0 and runner image. No new SQL/environment hypothesis, so no blind rerun or root-cause closure. |

## Product implementation: image regions and preview

The existing image owner now persists up to eight regions, each with a stable
48hex identity, original-pixel geometry and note. The whole collection shares
4096 UTF-16 units/16384 UTF-8 bytes and the existing 64KiB record ceiling.
The strict Go/TS private contract uses a collection; v2 disk records are read-only
mapped to a deterministic region ID while retaining their exact CAS hash.
Only explicit save writes v3. Office v1 remains a distinct strict record.

The renderer adds/selects/removes regions, preserves each note, draws all overlays,
and references individual regions in the same main conversation. Matching
annotation/source revisions permit independent region references to coexist;
any collection mutation revokes old scopes. Core capture, read and model projection
still revalidate current authority; raw image pixels are not model-authorized.

Display-only quarter-turn rotation and zoom invert pointer coordinates to the
original snapshot. Cancelling or rotating mid-gesture restores prior geometry
without rolling back concurrent notes. Existing non-normal EXIF and animated PNG
rejection remains; no EXIF normalization or pixel-redaction claim is introduced.

Integration review reproduced and fixed three further draft defects: switching
region while awaiting save could redirect a reference; recapture after conflict
could adopt another writer's CAS; successive redraws after source replacement
could clear completed regions. Regression tests now retain original reference
intent, preserve the original conflicting CAS, and accumulate redraws on the
same current source. Every stale region must be redrawn or explicitly removed
before saving, while all notes survive. This is source and deterministic UI/Core
evidence, not a native installed GUI journey.

## Verification, failures and reuse

All storage-consuming commands sourced `./scripts/use-analytix-cache.sh` in the
same shell, with Go1.26.4 darwin/arm64 and the existing lockfiles. No dependency,
license, hash protocol or CI rule was changed. Evidence is isolated, directories
0700/files0600. Two task-created metadata files initially had 0644 permissions;
the cache helper refused before tests started. Their exact modes were corrected
and normal commands rerun; this was not a bypass or a product RED.

- Actual initial Office/Canvas RED: two failed cases (old view/old document bytes).
  Final IPC/controller group: 70/70; root recheck of Office/Canvas IPC: 36/36.
- Initial rotation RED: 2 failed/25 passed. The three later draft regressions
  failed against the candidate before their repairs (one selection case was
  repeated and is not counted twice).
- Go image/annotation focused group: 5 packages, 20 top-level/85 including
  subtests, zero fail/skip. Covers strict budgets/identity, v2 migration/no read
  mutation, exact old CAS, v3 replay, two-region persistence/capture, wrong IDs,
  source/revision changes, whole-collection scope revocation and Office v1 fields.
- `analytix_prod` server multi-region capture/revocation: 1/1 passed, exit0.
- Go race image application/server group: 6 top-level/6 including subtests passed.
  The server test process initially sampled only `_dyld_start`, then naturally
  entered and passed its tests; no signing/security change or executor swap.
- TS contract/Main ACK/reference store/send-time group: 4 files, 78/78.
- Final image store/mounted preview/navigation group: 3 files, 63/63.
- Additional existing chat navigation/file-action fixtures were adapted and
  passed; overlapping groups are not summed as independent tests.
- Root current missing-key/Manager group: 3 top-level/16 including subtests,
  3 packages passed. No zero-match package is called behavior coverage.
- Skills real isolated filesystem materialization→enable→upgrade→disable→reopen
  and in-flight rejection: 1 new integration test passed. Old-generation activation
  cannot revoke the new generation. First run failed because the fixture imposed
  a 15-second deadline on the entire filesystem workflow; the corrected fixture
  bounds its two channel waits separately and retains all assertions. Initial
  dyld startup delay, fixture failure, compile-only and final pass are separate.
  No nonexistent Go Host uninstall/dispose API was simulated.
- Full desktop `npm run typecheck` and `npm run build` (including runtime package
  before Main/preload/renderer) passed. An intermediate typecheck failed on two
  old test fixtures and JSX narrowing; fixed before the final pass.
- Focused ESLint and `git diff --check` passed. Build artifacts were not staged.

Exact commands, exit codes, candidate file hashes and raw local logs accompany
the local execution evidence and new report-only handoff. Full earlier suites,
real Provider, fixed Office engine, Chromium navigation, native installation,
IME, current CodeQL and remote CI were not rerun or claimed.

Representative exact commands (from repository root, with the helper in each shell):

```sh
source ./scripts/use-analytix-cache.sh && npm run typecheck && npm run build
source ./scripts/use-analytix-cache.sh && npx vitest run \
  src/renderer/src/write/image-region-session.test.ts \
  src/renderer/src/components/write/WriteImagePreview.test.tsx \
  src/renderer/src/write/image-thread-navigation.test.ts
source ./scripts/use-analytix-cache.sh && cd packages/runtime-go && \
  go test -p 1 ./internal/ports/objectediting ./internal/app/objectediting \
  ./internal/adapters/inbound/httpapi ./internal/adapters/outbound/filestore \
  ./internal/server -run 'Test(Image|AnnotationDraft)' -count=1 -json
source ./scripts/use-analytix-cache.sh && cd packages/runtime-go && \
  go test -p 1 -race ./internal/app/objectediting ./internal/server -run '^TestImage' -count=1 -json
source ./scripts/use-analytix-cache.sh && cd packages/runtime-go && \
  go test -p 1 ./internal/app/model ./internal/server ./internal/app/providerregistry \
  -run '^(TestMissingProviderKey|TestManagerRequiresProductionOwnerSourceReceiptBeforeMigrationEffects)' -count=1 -json
source ./scripts/use-analytix-cache.sh && cd packages/runtime-go && \
  go test -p 1 -tags analytix_prod ./internal/server \
  -run '^TestImageProductMultipleRegionsCaptureSelectedNotesAndCollectionRevocation$' -count=1 -timeout 2m -json
source ./scripts/use-analytix-cache.sh && cd packages/runtime-go && \
  go test -p 1 ./internal/adapters/outbound/pluginmaterializationfs \
  -run '^TestOfficeSkillHostMaterializationLifecycle$' -count=1 -timeout 2m
```

## Remaining P0–P5 and specific gates

| Package | Current remaining work |
| --- | --- |
| P0 | Fresh lawful-source independent diff/CodeQL and exact-candidate CI; Rust's original DuckDB INTERNAL root cause. Review gaps above remain explicit. |
| P1 | Fixed product Office engine original/no-op/edited comparisons and visuals; no new verified loss supports changing setString speculatively. |
| P2 | Native acceptance of the new image collection/rotation. Browser still lacks a trusted guest/document selection→Core→same-thread reference chain; this is a source capability gap, not a blanket GUI block. Preserve PDF owners. |
| P3 | Imported pivot modification, chart and wider typed PPT style APIs plus fixed-engine preservation/reopen evidence. Real generated pivot and bounded PPT geometry/fill are already implemented and retained. |
| P4 | Installed Skills lifecycle and true uninstall/old-generation disposal at their actual owner; retain same-thread completion, Core retrieval, 8/4 resource bounds and existing cleanup. |
| P5 | Qualified private darwin-arm64 installation and shared synthetic workspace/thread journey, native IME/reopen/recovery and authorized real Provider. Source-experiment/publishable=false is not qualification. |

Browser work needs an actual attached guest/host document registry, isolated
engine-derived Selection/Range anchors and irreversible incarnation/currentness,
plus a private Core read-time proof. The current URL guard removes guest preloads;
no renderer text/hash, URL, MutationObserver counter or frame ID alone establishes
that authority or full visual revision. No new browser authority was fabricated.

Source create_tree refusal remains PAYLOAD_SCOPE_UNKNOWN/NOT_CLEARED:
“This tool call was blocked by OpenAI because we couldn't determine the safety status of the request.”
No ordinary push, alternate API, source ZIP/bundle or executor was substituted.
Chromium's historical ERR_BLOCKED_BY_ADMINISTRATOR lacks exact scope/clearance;
SecurityAgent specifically rejects Computer Use access to com.apple.SecurityAgent.
Neither is generalized to all Mac work. No personal credential/profile/controller
or real case data was used, and no security/signature setting was disabled.

Remote PR28 remains OPEN/Draft at `dde784437dc8563e84066629dd57f4a11fd9acc9`;
actual main remains `ce96cf12581acfa0e19fae7c6aa9c709371012c8`. Strict Development
gate/app15368, resolved conversations, no bypass remain applicable; ordinary
approval count is zero. Old dde's Development success/CodeQL failure and two
unresolved path review threads are not current-local-candidate acceptance.
Fresh inventories remain dde 56 checks (55 success, CodeQL failure) and main 64
checks (62 success, Rust and Development gate failure); these are separate SHAs.
No push/Ready/merge/tag/Release/publication was performed. No new main was created.

Five judgments remain separate and negative: merged-main, verified-main, complete
product, Formal RC, public release. The next handoff is REVIEW_ONLY with new
per-finding and executed evidence summaries; source writer is not released and
latest local source is not supplied through the report package. Repository QA and
handover are authoritative; the existing Notion ledger receives only a bounded
verified summary after the local checkpoint, never source or raw private logs.
