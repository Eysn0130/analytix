# PR28 independent-review continuation — 2026-09-21

Status: Current continuation plus historical execution receipts; not product/release acceptance.
Scope: latest user-adopted continuation A–M, retaining prior A–I receipts and the original A–M/P0–P5 outcome.
Repository and source writer remain the canonical Analytix checkout and original Codex owner.
No Goal, scheduled continuation, new independent task, public release or writer transfer was used.

## Current continuation A–M: review package and Browser implementation

This later execution starts at clean `40a6a1c9aca56229eb56b43dce381b709a86462b`,
tree `616a867512c4281544b8a6113564b34a56c2a611`, on the same branch and writer.
All older checkpoints remain ancestors. SOURCE `65ab0efca8765385dc09d3522ca567d9bffd0f40`, tree
`36370dd74196b8d7e141c5cdcbff45aeb1145eee`, observed `2026-09-21T11:40:25.920424+00:00`; the final report manifest separately
records SOURCE and documentation-only DELIVERY, their trees and observation times.
No old checkpoint was reset or used as a replacement worktree.

The new `Analytix-PR28-Review-Continuation-2026-09-21.zip` was safely extracted
exclusively after path, duplicate/case-fold, file-type, encryption, size/ratio
and CRC checks: 17 files, 439,928 expanded bytes; outer manifest **16/16**.
Its computed archive SHA256 is
`fd9957f0aa0b817dd4a57cc886a4c014cafeab6536a184f8e3a044e23a2750c3`;
no absent external checksum sidecar is claimed verified. The unchanged reference
NO_SOURCE package separately passes **9/9**. These are not the earlier 54/54
and 30/30 counts below. The adopted A–M and 00–11 reports were read. The supplied
checker was inspected before offline execution; **32 tool self-tests** pass,
separate from the prior 13 tool tests/26 Python vectors and product tests.

### Evidence corrections and recovered receipts

The earlier wording is corrected to **16,287 flow steps** and **16,783 records
including separately listed flow endpoints**. Findings=67, flows=248, ignored-flow
warning count=1,184; these meanings and all old fingerprints remain unchanged.
The new report package carries summaries and sanitized executor receipts, not
all raw private logs. Original owner execution records recovered 13 validation
groups with actual command suffixes, working directory, tool call/receipt times,
recorded shell exits, available matched/pass/fail/skip and local-log byte/hash
identities. Missing process start/end, historical per-run toolchain or candidate
fingerprints remain `NOT_RECORDED`. Tool observation timestamps are not process
timestamps. Commands following source edits are labelled validation suffixes,
not falsely presented as the complete tool call. An empty eslint log is not the
basis for exit0; the executor completion receipt is. Reused historical log names
are current retained bytes, not independently proven bytes for every earlier run.
New checks record real start/end, command, platform, locks, before/after HEAD/tree,
dirty-file fingerprints, exit and log hash. No unchanged 995-test suite was rerun.

The current QA, handover and product-completion bodies were read locally, along
with the documentation/spec registries, runtime/desktop guides, development and
Git runbooks, Office admission/resource manifest and knowledge-base governance.
This is a local full-text review of the relevant entries, not ChatGPT approval
of previously unavailable bodies or a claim to have audited every repository doc.
The Git workflow now explicitly retains the unattributed-change approval rule.
The Rust fixture also clarifies that 13,000 is `row_summary.total_amount` across
**two** aggregate rows, ordered by amounts 3,000 then 10,000; it is not 13,000 rows.

### Browser implementation and evidence ceiling

The existing Main guest registry now captures actual isolated-world Selection/
Range nodes and offsets, bound to the host and guest document leases. The renderer
can supply only a registered guest ID and thread ID; it cannot supply source text,
URL, principal or a purported revision. No guest preload, new Provider outlet or
second RAG/permission system was introduced. The existing Go object-editing owner
freezes primary-thread/workspace/principal authority and projects the captured
text through the existing privacy layer. Composer references contain opaque scope
IDs only. Every Core/model read issues a fresh, one-use challenge answered by Main
after checking held nodes, offsets, text and irreversible lifecycle identity.
DOM mutation conservatively invalidates the handle, including text ABA; this is
selected-text currentness, **not full-page visual revision**. Capture/quote never
sends; ordinary explicit Send uses the same main conversation and rechecks after
asynchronous preparation, preserving drafts on rejection.

Budgets: one in-flight Main capture, eight retained Main selections (oldest scope
revoked on replacement), 64 Core scopes, five-minute lifetime, 4,096 UTF-16 /
16,384 UTF-8 bytes per selection, one pending read per scope, five-second read
proof deadline. Foreign full scope bindings are rejected before creating a read
challenge. Two new implementation counterexamples were reproduced RED→GREEN:
concurrent captures bypassing the Main reservation, and altered document bindings
consuming/expiring a legitimate scope. Late Core capture after navigation revokes
its returned scope. Main-frame/iframe, same URL, same document, provisional cancel,
crash, close, cross-thread, authority change, nonce replay, expiry and privacy
projection have bounded tests. Child-frame selection capture itself is not enabled.

The fixed Electron **41.10.3 / Chromium 146.0.7680.216** engine ran eight synthetic
checks: real isolated Selection/Range; page-world getSelection forgery isolation;
DOM ABA; same-document navigation; programmatic same-URL load; child-frame
navigation; programmatic back; actual destroyed event. Initial harness failures
(file permissions, non-cloneable harness return and synchronous destroyed-event
assumption) are retained and distinguished from product defects. This engine
probe uses synthetic files and an isolated profile; it is not actual guest/Main/
Core end-to-end product GUI, fixed Office fidelity, installed or live Provider
acceptance. Browser-specific quick-action UI and complete native journeys remain
open, rather than silently narrowing the accepted product outcome.

### New path #33 counterexample and focused execution

A real synthetic protected directory/file and a symlink into it reproduced
`exists=true` because WorkspaceStatus performed host Stat before the contained
CommandProbe. The HTTP handler accepts its query path directly and production
composition supplies protected roots. The repair keeps the existing owner and
OS process policy: when roots exist, metadata lookup uses a contained `test -e`
process without a shell. Resolution and access occur inside that boundary, not
through a new string-prefix guard. Without protected roots the existing Stat
semantics remain. Unsupported containment fails closed. Ordinary directories,
Git dirty status and the existing malicious-fsmonitor containment check remain
green. This closes the reproduced metadata case, not all15 path findings or the
old CodeQL flow; no alert was dismissed.

Final focused groups (overlapping runs are not summed):

| Group | Actual result | Scope |
| --- | --- | --- |
| Browser Main/script/reference + mounted Workbench + existing consumers | 90/90 Vitest; exit0 | Seven files, new and retained cases; synthetic IPC/Provider seams |
| Browser objectediting/server, normal | 4 top-level / 11 including subtests; exit0 | Actual Core/HTTP/identity/privacy/tool seam; synthetic Main proof |
| Browser same selection under analytix_prod + race | 4 top-level / 11 including subtests; exit0 | Distinct build mode, overlapping cases, no installed/live Provider claim |
| WorkspaceStatus process owner | 4 top-level / 9 including subtests; exit0 | Real synthetic filesystem and existing macOS containment |
| WorkspaceStatus HTTP | 2 top-level / 2 total; exit0 | Actual handler/service/probe, existence denied |
| Fixed Electron probe | 8 assertions; exit0 | Real41.10.3 isolation/navigation; separate from mounted product GUI |
| TypeScript and desktop build | npm run typecheck; npm run build; combined exit0 | Source/build only |
| Production Go composition | go build -tags analytix_prod ./cmd/runtime-server; exit0 | Rebuilt after metadata repair, private temporary output |
| Changed TS/TSX eslint | exit0, 0 errors / 1 warning | DevBrowserPanel captureEpoch ref cleanup heuristic; warning retained |

The first Browser typecheck failed on the missing non-Electron bridge facade;
it was repaired before the final check. Earlier HTTP route-package selection had
zero matching tests and is not counted as behavior passed. RED logs for Main
concurrency, altered scope and protected metadata are retained. Cache preflight
also rejected a probe file mode and an executable mistakenly placed in evidence;
the exact task-owned files were corrected/moved into the temporary-build area,
then checks actually reran. No cache guard, signing or security policy changed.
Toolchain observation: Node22.22.1/npm10.9.4, Go1.26.4 darwin/arm64,
Rust1.94.1, macOS26.5.2 build25F84. Historical groups are not assigned these
newly observed versions retroactively. Complete commands, times and log hashes
are in the report-only execution-receipts and prior-execution-receipts records;
raw logs and the temporary runtime executable remain local.

Write retirement recheck found no source references to WriteAssistantPanel,
ensureWriteThreadForWorkspace, createWriteThread or selectWriteThread. The live
WriteWorkspaceView still requires onSubmitPrompt and calls the same-thread owner.
No functionality, UI Refresh surface, gate or old evidence was deleted.

### Remaining A–M/P0–P5 work, still in scope

| Requirement | Current classification and next minimum action |
| --- | --- |
| C / 67 findings | Original scan remains old analysis1792355564/CodeQL2.27.0/dde. Before the new repair, all43 recorded sink/guard files matched9cf. Afterward66/67 sink files still match; #33 is explicitly CHANGED. Identity comparisons are reuse evidence only; caller changes require review. No CodeQL executable was found on this shell PATH; current-source scan and full caller closure remain EVIDENCE_NOT_RUN, not dismissed. |
| D / Office/Canvas | Previous lease/disposal tests retained; new actual Electron event evidence supplements them. Close/reopen and canceled provisional navigation in the complete product remain EVIDENCE_NOT_RUN. |
| E / images | Eight-region CAS, notes, rotation/zoom and three earlier race repairs retained. Multi-region native GUI journey and trusted pixel/metadata projection remain EVIDENCE_NOT_RUN / SOURCE_GAP; no pixel editing or redaction claim. |
| F / Browser | New source chain and focused evidence implemented. Actual attached guest → Main → Core → composer GUI journey, child-frame capture and Browser-specific explicit quick actions remain EVIDENCE_NOT_RUN / SOURCE_GAP. |
| G / DOCX | Product worker still uses local-range setString; no new actual fixed-engine loss counterexample. The synthetic oracle is retained. Fixed-worker original/no-op/edited structural and visual comparisons plus Diff/accept/save/reopen remain EVIDENCE_NOT_RUN. |
| H / XLSX/PPT | Real pivot generation and typed range/PPT geometry/fill remain. Imported pivot refresh/chart modification and additional required styles still need pinned UNO API/object mapping and same-owner integration: SOURCE_GAP, not a GUI-only block. No full-file regeneration substituted. |
| I / Skills/Write | Desktop installHubAgentPlugin returns remote_plugin_materialization_requires_go_archive_authority before mutation. Actual install/upgrade/uninstall/restart and in-flight teardown therefore remain SOURCE_GAP plus EVIDENCE_NOT_RUN. Existing local-remount protection is retained; filesystem lifecycle is not desktop install evidence. No fake Go Host uninstall API added. Existing Write replacements and 8/4 retrieval bounds remain. |
| J / Rust | Same analysis tree and old contrasting CI retained; no new falsifiable SQL/environment hypothesis established, so no blind rerun. DuckDB INTERNAL and historical dyld startup delay remain distinct unresolved evidence. |
| K / install | No package of this new source was made or accepted. Old8365 DMG and current source build do not satisfy it. Resource seal, exact fixed assets/notices, isolated native journey and normal Registry/Provider admission remain prerequisites/evidence gaps. Historical Keychain challenge is not generalized into a Mac GUI ban. |
| L / synchronization | SAFETY_NOT_CLEARED: create_tree PAYLOAD_SCOPE_UNKNOWN / NOT_CLEARED. No push, alternative API, source archive, Ready, merge or release. No writer transfer: OWNER_HANDOFF_NOT_RELEASED; next ChatGPT work remains REVIEW_ONLY. |

Fresh read-only GitHub observation still separates remote PR `dde784437dc8563e84066629dd57f4a11fd9acc9`
(OPEN/Draft, 56 checks: 55 success, CodeQL failure) and actual main
`ce96cf12581acfa0e19fae7c6aa9c709371012c8` (64 checks: 62 success, Rust and
Development gate failure). Two review threads remain unresolved. Strict Development
gate/app15368, resolution and extra approval for unattributed changes remain;
ordinary approval count0 does not waive the latter. No fresh local-candidate CI
or merge/main verification is claimed. Complete product, Formal RC and public
release remain separate and unachieved. No Goal, automation, new task or subagent
writer was started.

## Historical execution from fe656 to 9cf / 40a6

## Identity and intake

START HEAD `fe65622e5d827637f6a7423ef60dc048fee20d3c`, tree
`2af1390bc927a86b82a50090a73f2cb9a1a1168d`, clean, original branch
`codex/workbench-product-delivery-20260914`. The verified ancestry retains
`148d188e4f7df71e1cf28bc25bfff988365fa1ea`, `5c2f1b089533b70e3a7623396e9690d2d355aab4`
and `2ef3f9deb22714b7c05583603bf1e394cce1f06e`. No history was rewritten.
Source checkpoint: `9cf3fddbd4ac568d70b4787ef8aebd192d9bd5dc`, tree
`bd1bbae692ce5d4801f0f3854667670146bbc274`. A subsequent evidence-only correction
records race counts as 6 top-level/11 including subtests without changing tested
source. The delivery manifest records the final full HEAD/tree and dirty state.

The uploaded `Analytix-PR28-Independent-Review-2026-09-21.zip` SHA256 is
`2adc4a541620bb12cea669fa7ae1d6e311b462a3f92f22e2e7b4e16b66f19742`.
Safe exclusive extraction checked paths, duplicate/case-fold names, types,
symlinks, encrypted entries, sizes/ratios and CRC: 55 files, 8,562,664 expanded bytes.
Outer SHA256SUMS: 54/54; nested reference/input manifest: 30/30. Markdown/text
counterparts match. The 00–07 reports, 67 results, A–I and original A–M/reference
matrix/receipts were read. The attachment contains reports and prior public
analysis, not a source patch or a writer/safety-clearance receipt.

Reviewer tooling was rerun separately: 13 self-tests, 67 results, 248 retained
flows, 16,287 flow steps (16,783 flow location records when separately listed
source/sink endpoints are included), 1,184 ignored-flow warning, 26 Python floating
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
- Go race image application/server group: 6 top-level/11 including subtests passed.
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

Representative commands and reported outcomes are recorded below. Raw private
execution logs remain in the authorized local evidence directories. The report-only
handoff carries candidate fingerprints, selected terminal/test-name records and
raw-log fingerprints, not raw logs or a complete per-run command/environment/
shell-exit receipt. Missing original fields remain NOT_RECORDED; an empty log
does not establish exit 0. Record consistency is not an independent product rerun. Full earlier suites,
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
approval count is zero, with `require_extra_approval_for_unattributed_changes=true`
still applicable when triggered. Old dde's Development success/CodeQL failure and two
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

Current source change totals: production_contract_config: 25 files +609/-14; test: 9 files +495/-2. Documentation totals and final DELIVERY are recorded after the documentation checkpoint; no tested source is changed by it.
