# PR28 independent-review continuation — 2026-09-21

Status: Current continuation plus historical execution receipts; not product/release acceptance.
Scope: latest user-adopted readiness continuation A–N, retaining prior A–I/A–M receipts and the original A–M/P0–P5 outcome.
Repository and source writer remain the canonical Analytix checkout and original Codex owner.
No Goal, scheduled continuation, new independent task, public release or writer transfer was used.

## Current readiness continuation from d864

START `d8640957c4c1c69c5ddee4e3a9b017382c088047` / tree
`2e4576860898320de4805fd5090540c6386ce724`, clean/ahead61. Original repository,
branch and writer retained. SOURCE `2f25c22f7c8fb9626e204e34d573fb1de3e7208b` /
tree `63b1edccec3475abc7990776e77b5cda2df92245`, clean/ahead62 before documentation.
Production/contracts/config4 files +40/-5; tests3 files +62/-4; total7 files +102/-9.
Documentation-only DELIVERY and totals are recorded separately in the final manifest.

### Intake and precise errata

Release-Readiness-Review ZIP SHA256
`305e690e34b36207520bd9980a9698900f464600171c00308f74d381b6978e50`,88062bytes,
20members/19manifest objects/305788 expanded bytes: safe paths, unique NFC/casefold
names, regular types, encryption/size/ratio checks, CRC and19/19SHA256 pass. No
external sidecar was supplied; internal hashes do not prove author authenticity.
The user-adopted A–N was read in full; review attachments are evidence and proposals,
not independent permission. No attachment script was automatically executed. The
reviewer's30 new selftests, prior64 tool selftests and32 scenario specifications are
not product tests and were not rerun merely to raise counts.

- The prior browser-red group discovered59, executed3, passed0, failed3, skipped56.
  Its old `matched:3` meant executed tests; it did not include skipped tests. Original
  receipt remains immutable. Candidate movement still prevents calling it stable RED.
- The old LibreOffice CharFontSize link ending `#L7363` is an incorrect anchor.
  Correct pinned location is [7943–7957](https://github.com/LibreOffice/core/blob/efaf0670b4d055f838a2849becb10f08aa06a257/sw/source/filter/ww8/docxattributeoutput.cxx#L7943-L7957).
  This erratum does not rewrite the old experiment or upgrade it to a complete proof.
- The old tail's25production+609/-14 and9tests+495/-2 refer to65ab, not4343 or2f25.
  Its title is now version-qualified. Old13/26/32/64 review/vector counts stay separate.
- Missing every OOXML font-size source does not mandate10.5pt. Ad-hoc verification is
  not Developer ID/notarization. Empty combined-status output is not green check-runs.

### F0/F1/F2 and actual DOCX protection

The immutable original fixture SHA256
`056c46b78817f37af957d76ca7738a31bd5cad3724b51fdac97adf7c59b9560f`
has four ZIP parts: document, its relationships, root relationships and content types.
It has no styles/settings/fontTable, paragraph/character style references, direct
w:sz or w:szCs. Relationship IDs are unique. Thus there is no basedOn chain or cycle
in this fixture; observed11/10.5 sizes are application defaults, not declared values.
One run declares Liberation Serif/Noto Sans CJK SC; file bytes alone do not establish
font substitution. Member bytes/hashes and relationship records are retained privately.

F1 adds only styles/docDefaults w:sz22 plus its relationship/content type; document.xml
remains byte-identical. Fixed engine original/no-op/reopen stays11/11. F2 adds a default
paragraph style w:sz21 and a run-local22 override: original/no-op/reopen stays11/11 and
10.5/10.5 respectively. Both comparisons retain every recorded Western/Asian font,
size, weight, posture, color, link and text value. Adjacent identical-style runs may
merge for semantic comparison; no font property is dropped. Both screenshot pairs
were visually read with no observed layout difference in this small synthetic page.
This is not a full rendered-document oracle or installed GUI acceptance.

The existing worker now checks native body/table portions before edit admission and
again before export, with8192 traversal steps and16 nested-table levels. Different
Western/CJK sizes, unavailable/invalid properties and exceeded bounds return finite
`unsupported-format-fidelity` before storeToURL. Main maps the refusal to the existing
unsupported-selection contract; the isolated surface explains that the original was
not saved and preview remains available. The document is not silently normalized.
The existing field/link structure guard and Core CAS/review/undo chain remain intact.
This check does not claim coverage of every header, footnote, drawing, relationship or
formatting property; broader candidate validation and original preservation remain open.

Real pinned LibreOffice `efaf0670b4d055f838a2849becb10f08aa06a257` / zetajs
`57360bcb0e7726ffa0e66567c8041261b959f8dd`, all6assets byte/hash verified, running in
Electron41.10.3 on this arm64 Mac: original fixture rejects editing with sequence0,
clean state and no export; both explicit/inherited-size positive controls still accept
cross-run replacement, export and reopen with all measured properties preserved.
The private probe uses test-only selection/inspection hooks; product source adds no
arbitrary UNO/IPC or preload exception. Original4-case14pass/8fail evidence remains;
the new refusal is a loss-prevention result, not8 successful original-file edits.

### Proportional verification and retained failures

All storage-consuming commands source `scripts/use-analytix-cache.sh` in the same
shell. New runner records command/cwd/start/end, source HEAD/tree/dirty fingerprints,
lockfile hashes, platform, actual exit and raw log bytes/hash. New native helpers and
the test-only worker were fingerprinted **before** execution. Per-run executable
versions not captured by a receipt are NOT_RECORDED, not backfilled; the separate
version observation is Node22.22.1/npm10.9.4/Python3.11.15. Engine logs bind Electron41.10.3.

| Actual group | Exit / observed result | Scope |
| --- | --- | --- |
| typography-red |1;3 executed/0pass/3fail/18skip,21discovered | Stable original worker + new tests; no moving product candidate |
| typography-green |0;45pass/0fail/0skip | worker/typed/PPT/transport |
| typography-controller |1;62pass/1fail | New test incorrectly passed revision fields to the close request; retained fixture failure |
| typography-controller-final |0;78pass/0fail/0skip | Main/controller/surface/preload/transport after exact close request correction |
| typography-final |0;41pass/0fail/0skip | final worker/transport, including invalid sizes and traversal bounds |
| typography-typecheck |0 | Both web/node TS configurations |
| typography-lint |0 | Five changed TS production/test files; empty stdout is not inferred success, executor exit is recorded |
| docdefaults-control / style-control |0 each | Actual fixed engine F1/F2, not mock or alternate Office |
| typography-native |1 overall | Original pre-mutation refusal passed; positive comparison stopped on semantically identical run segmentation |
| typography-native-positive |0 | Two controls cross-run edit/export/reopen; canonicalization merges only identical complete style maps |

Groups overlap and must not be summed. No995-test/full-CI, repeated Rust experiment,
new private package, installation, Provider or release check was run. Old private4343
DMG cannot validate2f25 source. Native raw logs and helper implementations remain in
private task evidence, not report-only exports. The report contains actual commands,
receipts and fingerprints, not a claim that all raw logs travel with it.

### Source-gap investigation without fabricated admission

Skills: current `app/pluginmaterialization/service.go` freezes FormalPackageBinding
from pre-admitted packaged resources; `app/pluginpackagehost/service.go` accepts a
bounded fixed development-source registration set, not arbitrary publisher ZIPs.
`pluginmaterializationfs` already owns signed stage/index/journal and activation.
Desktop remote install still returns `remote_plugin_materialization_requires_go_archive_authority`.
No current trusted remote publisher/registry descriptor binding was found in these
owners; hashing a ZIP or adding a fake Host uninstall method would not complete it.
Archive authorization/materialization and installed uninstall remain SOURCE_GAP,
not a completed filesystem fixture or merely a GUI environment limitation.

Imported pivot/chart: existing typed cells, real pivot generation and PPT geometry/fill
are retained. Stable imported part/relationship/field identities, actual fixed UNO
mutation/refresh, shared-cache preservation and reopen rebind are still SOURCE_GAP.
No SUMIF substitute, whole-file reconstruction or permissive renderer authority was added.

Chromium: nine Electron41.10.3 source blobs were read and their Git object hashes checked.
[PostCreateMainMessageLoop](https://github.com/electron/electron/blob/v41.10.3/shell/browser/electron_browser_main_parts.cc#L517-L562)
sets macOS service `<app name> Safe Storage` and account `<app name>`.
[Network-service initialization](https://github.com/electron/electron/blob/v41.10.3/shell/browser/net/system_network_context_manager.cc#L279-L284)
gets the process-bound raw encryption key when Cookie encryption is enabled.
[Fixed safeStorage exports](https://github.com/electron/electron/blob/v41.10.3/shell/browser/api/electron_api_safe_storage.cc)
provide no public task-Keychain selector; Linux password-store configuration does not
apply on macOS. Chromium146.0.7680.216 keychain/OSCrypt sources were also read.
These are static source observations, not an OS credential trace. No personal profile,
Keychain, SecurityAgent, global search list or signing policy was accessed/changed.
Isolated userData and the Go Secret Store binding do not establish this missing native
boundary. Exact process-specific storage admission is still required before installed launch.

Rust: current require_nonempty_query performs COUNT(wrapper) then registers the observed
range; current query_rows already labels aggregate_count/page_nonempty/page_values/
nonempty/output_nonempty/page_output. Historical0bc output_file order is not the ce96
aggregate target. No new discriminating hypothesis or matching binary/symbol artifact
was established, so no repeat of the prior six query executions was performed.
No SQL,3000/10000ascending fixture, optimizer or DuckDB version changed.

### Documentation, remote state and remaining outcome

The two long current entry files were consolidated into a single current handover and
product matrix. Their complete d864 contents were moved into the existing dated
`handovers/2026-09-16-pr28-continuation.md`, labeled Historical with exact original
content SHA256, source commit and compatibility anchors. Product-relative links were
rebased. No old failure, legal notice or execution result was removed. The existing
consolidation register records the move; unreviewed repository documents remain unreviewed.

Fresh read-only GitHub observation at2026-09-21T17:06:22.418597Z: PR28open/Draft/unmerged,
head `dde784437dc8563e84066629dd57f4a11fd9acc9`, actual main/base
`ce96cf12581acfa0e19fae7c6aa9c709371012c8`. Rules remain separately applicable.
This read did not refresh every historical CI job and does not validate2f25.
Original create_tree refusal remains PAYLOAD_SCOPE_UNKNOWN / NOT_CLEARED; exact payload
and regular clearance are not newly available. No alternate push/API/sourceZIP/executor,
remote write, Ready, merge, tag, publication, Goal or automatic continuation occurred.

Full A–N/P0–P5 is **partial**, not all locally implementable source work completed.
The current matrix lists SOURCE_GAP for imported editing/Skills/Chromium, EVIDENCE_NOT_RUN
for installed/native/Provider/current-scan seams, EVIDENCE_FAILURE for unresolved old
Rust and original typography, and SAFETY_NOT_CLEARED for source egress.
MERGED_MAIN, VERIFIED_MAIN, COMPLETE_PRODUCT, FORMAL_RC and PUBLIC_RELEASE remain false.
The original writer is retained. New report-only evidence permits REVIEW_ONLY follow-up;
it neither supplies nor approves the current product diff or releases the writer.

<a id="current-closure-continuation-an-browser-actions-docx-structure-and-bounded-rust-probes"></a>

## Historical 4343/d864 closure: Browser actions, DOCX structure and bounded Rust probes

START `5242782f099f10ddb6af91eb70e8ad5a0944aa1e`, tree
`c52bd237bd5e1520d8c4a69c54423cfaee74700d`, was clean/ahead57 on the original
branch. SOURCE `4343ec436ce5df65ba4f413ed49fabab90825a8c`, tree
`2e94577ae8d9bac4adabb53a33f7cf9513193aec`, retains every prior checkpoint.
The original owner still holds writer; documentation DELIVERY is recorded separately
in the final manifest. No Goal, automatic continuation, independent task or parallel
writer was started. This is an incomplete product continuation, not an A–N closure verdict.

### Intake and independent-review boundary

`Analytix-PR28-Review-Closure-2026-09-21.zip`: 20 members,177233 expanded bytes,
19/19 manifest entries and CRC pass; safe paths/types, encryption, size/ratio,
casefold and Unicode NFC collision checks pass. Computed ZIP SHA256:
`88a7507739fe1041a963b7a9f59d948edecfd305ba8988f8c7de507e7864f8a6`.
The external checksum sidecar was not supplied. All A–N, report/errata/research/
interface/GitHub/receipt documents and 32 scenario specifications were read.
The supplied verifier was read before execution: **64 tool self-tests pass**;
it also recomputed the previous report package's12/12. These are neither product
tests nor32 passed scenarios. Prior16/16+9/9 and54/54+30/30 remain different inputs.

Five current-document corrections were applied using the actual matching anchors:
older Browser gap marked historical, GitHub recovery limited to lawfully synced
state, historical GitData text not an alternative authorization, conditional release
goal separated from qualification, and pinned setString risk initially labelled a
hypothesis. The new fixed-engine counterexample below supersedes that hypothesis
for fields only. Canonical QA/handover/product-completion remain the owners.

### Implemented and observed

- `dcdbcaca23037becc0b7bb0b1e6c671cd42c18b4`: Browser's explicit Explain selection
  button routes a fresh opaque reference through the existing primary-thread submit
  owner. Passive quote remains non-sending. Held preparation/revocation tests retain
  composer drafts and attachments. A focused iframe returns a finite unsupported
  result before Core capture. Real Electron41.10.3/Chromium146.0.7680.216 showed
  simultaneous parent and child selections: baseline5242 captured the stale parent,
  current source rejects the child. No guest preload, new Provider or second owner.
- `8a2a3d544caec521239487a034e54f764bfcff88`: Rust adds static failure-stage context
  and three bounded test controls using existing SQL builders and verified read-only
  sessions. No production SQL, fixture, optimizer, thread setting or expected result
  changed. Original exact CLI target passes1/1 with94 filtered. See limitations below.
- `4343ec436ce5df65ba4f413ed49fabab90825a8c`: fixed Office worker previously flattened
  a selected DATE field via setString and reported success. The captured-range owner
  now checks bounded native text portions before mutation; fields, links and unknown
  structure reject with unsupported-selection. It does not scan or replace the whole
  document. Fixed-engine field/link rejection preserves model text, changeSequence0
  and clean state. Ordinary and cross-run plain text edits remain admitted. Controller
  coverage retains the prepared cancellable review, performs no export/commit on
  rejection, and does not invent Saved. Notes and the original file remain unchanged.

Source changes total18 files,+314/-28: production/contract/config11 files,+82/-23;
tests7 files,+232/-5. Exact per-file numstat is in the final source manifest. Browser13+130/-12; Rust2+127/-13; Office3+57/-3.
No production Go, Canvas, image, Write alternate route, UI Refresh, CI or license gate
was modified. Prior source evidence is retained within its original scope.

### Fresh checks, failures and provenance

All storage commands source `scripts/use-analytix-cache.sh` in the same shell.
New receipts record command/cwd, actual UTC start/end, platform, HEAD/tree/dirty,
modified/untracked fingerprints, lockfiles, process exit and retained log byte/hash.
Per-run hashes of external temporary probe scripts were NOT_RECORDED; later helper
contents are not used as retroactive proof of earlier invocations. Named result files,
logs and committed product hashes retain their separate evidence value.
Toolchain observation: macOS26.5.2 arm64, Node22.22.1, npm10.9.4, Go1.26.4,
Rust/Cargo1.94.1, Python3.11.15. Per-run prior unrecorded versions are not backfilled.
Host evidence root is `/Volumes/AnalytixCache/development-v3/evidence/pr28-closure-20260921`;
raw logs, injected probe scripts, synthetic binaries and profile stay local. Report
output carries sanitized receipts/results, not source, patches or private raw logs.

| Group | Actual result and ceiling |
| --- | --- |
| browser-dom |59/59:52 mounted Workbench +7 isolated-script fixtures; exit0 |
| browser-panel | Main13 passed; new panel suite failed collection due incomplete i18n mock; exit1 |
| browser-panel-fixture-fix | Real mounted panel4/4, exit0; passive/explicit/unsupported/late capture |
| browser-focused | JSON records28 passes in3 files, but2 requested files absent and shell exit1; PARTIAL, not full pass |
| browser-red |3 failures/56 skipped; relevant source changed before process end, so not an exact stable-candidate RED. The later fixed-Electron baseline comparison independently reproduces the actual stale-parent defect |
| browser-electron-baseline-comparison |2 actual engine assertions pass plus old5242 stale-parent substitution observed; not full guest/Main/Core/installed acceptance |
| office-structured-red |5 new structural assertions fail,13 existing pass; stable pre-fix source; exit1 |
| office-structured-green |42/42 across worker/typed/PPT/transport, exit0 |
| office-structure-controller |41/41 including no-save/cancellable-review structural refusal, exit0 |
| candidate-typecheck-lint | Root web/node typecheck and changed Office/test lint exit0; earlier Browser lint exit0,0 errors/0 warnings |
| candidate-desktop-build | Exact SOURCE desktop bundle exit0; earlier runtime build also exit0; not package acceptance |
| rust-target | Original exact stats_query_cli target1 pass/94 filtered; no semantic changes |
| rust-plan-controls | Wrong --bin target matched0; exit0 is not behavioral evidence |
| rust-plan-controls-library |3 query results met assertions, then all3 failed verified-session commit because the new diagnostic omitted range receipt registration; exit101 |
| rust-page-controls-bounded | Receipt fixed using actually observed row count; page and COUNT(page)2/2 pass,287 filtered; aggregate end-to-end case not rerun |
| office-fixed-font-default-control |2 no-op observations,1 explicit-size stability assertion pass; original omitted-size drift remains; exit0 |
| office-fixed-complete-observations |4 fixed-engine cases,22 component assertions:14 pass/8 typography failures; exit1. Never report this matrix as pass |

Rust used six total target/query executions: original1, first controls3, corrected
page pair2. A cancelled incorrect multi-filter command executed no query. Fresh
read-only connections had threads8, preserve_insertion_order=false,
enable_external_access=false, no disabled optimizer, memory_limit19.1GiB;
DuckDB enginev1.5.4, locked Rust duckdb/libduckdb-sys1.10504.0. Exact synthetic SQL,
fixture hash and parameters remain in local receipts; the report includes hashes
and outcomes. No INTERNAL occurred, so R-H1 is **not reproduced**, not disproved.
No extra threads1/optimizer trial or repeated original target was run. The first
aggregate SQL/result is observed but its corrected full receipt path was not rerun.

A read-only upstream lead is [DuckDB issue22846](https://github.com/duckdb/duckdb/issues/22846):
its report concerns an intermittent parallel window failure; comments also discuss
an unrelated SQLite binding stack. Generic index0/size0 text is insufficient to match
Analytix's cause. No upstream SQL/code was copied, no version was upgraded, and no
old main failure was dismissed. The old main job103860905234 log was subsequently fetched through read-only GitHub:
103329 bytes, SHA256 a0d86bd46fd71886fbf1afbf940c38d6b04146a84fa3320d0401a9892ce4074c.
It confirms94/95 and24 relative stack offsets without function symbols or statement
stage. Matching retained Linux binary/symbols are needed; never symbolize these
offsets against a different local build. Fresh run34807061316 artifact inventory is empty (total_count0); no matching symbols
were available from that workflow inventory. This is an evidence limit, not a rerun
authorization or proof that no copy can exist elsewhere.

### Fixed-engine DOCX evidence and unresolved fidelity

All6 pinned assets pass byte/hash verification: LibreOffice
`efaf0670b4d055f838a2849becb10f08aa06a257`, zetajs
`57360bcb0e7726ffa0e66567c8041261b959f8dd`. A private synthetic DOCX contains Chinese,
mixed bold/italic/color runs, duplicate text, hyperlink, DATE field and table.
The real product surface and worker were used with a local test-only selection/
inspection hook; no product IPC exception or arbitrary UNO entry point was added.
Four cases cover field, link, uniform and cross-run ranges, with original/no-op/
edited/reopened observations. Unsupported field/link edits preserve the current
model; table text and hyperlink relationships remain in all8 exports. Ordinary and
cross-run edits preserve the unselected field paragraph. Reopened render images
exist, one was visually inspected; this is not a complete original-vs-edited visual
comparison or the full Main/Core/Diff/CAS/installed journey.

**A separate unresolved observation remains:** the fixture omits explicit font-size
defaults. On no-op export/reopen, CharHeightAsian changes10.5→11 while Western
CharHeight stays11. All4 no-op and4 edited-reopen typography comparisons fail.
This is a fixed-engine import/export default-style drift, not a proven setString
style-loss cause or permission to rewrite all DOCX. The pinned
[DocxAttributeOutput::CharFontSize](https://github.com/LibreOffice/core/blob/efaf0670b4d055f838a2849becb10f08aa06a257/sw/source/filter/ww8/docxattributeoutput.cxx#L7363)
maps Western/CJK size to the same OOXML size element; this is a diagnostic lead,
not a complete root-cause proof. A subsequent two-input no-op control keeps the original fixture intact: omitted-size
input again changes Western/Asian11/10.5→11/11; a derivative with explicit w:sz22
in each text run stays11/11→11/11. Its one stability assertion passes, not the
original fidelity comparison. Original/no-op images were visually inspected and show
changed Chinese glyph spacing/line positions. This narrows the investigation to omitted
default/script-size representation in this fixture, not a complete codec repair.
Next: source OOXML/run inheritance and a precise preservation
or unsupported-format decision at the existing codec/worker owner. Do not erase
this failure by accepting a weaker typography comparison or silently normalizing input.

Probe failures were also retained: wrong Electron module import; protocol registered
on the wrong session; bootstrap return not cloneable; missing required canvas ID;
missing product layout constraints causing test-canvas growth/OOM; equivalent native
run splitting before adjacent-equal-style normalization. None is labelled a product
regression. Two terminated probe processes reported exit0 despite no success result;
their semantic outcome remains FAILED/TERMINATED, not pass. The final matrix still
keeps all typography assertions and exits1.

### Accurate private package attempt and remaining startup boundary

The first private build failed after native/desktop compilation at Go module
verification: cached golang.org/x/sys v0.46.0 ZIP had a confirmed CRC failure and
SHA256 e6ff34bba08b6a1d0c88d31a601614bfb41749852fb2acf40f98c912a757351a.
The exact corrupt ZIP/hash bytes were preserved privately, then normal module
download restored CRC validity and `go mod verify` passed without changing locks,
source, signing policy or checks. One retry stopped at the cache helper because a
new task-owned evidence JSON used0644; its exact mode was corrected to0600 before
any build started. The next invocation used the original private build wrapper.

`private-package-verified-cache` exits0 and produces
`analytix-1.0.6-mac-arm64.dmg`,470395220 bytes, SHA256
`200a8772dbfe36fc1212684a7b1bb0d183005b0f03a72892d6ba040c18f63338`, under the
host cache's `tmp/pr28-closure-private-package-4343ec436-verified-cache` directory.
SOURCE4343 is bound to snapshot
`81f8d866b602a9b4be4ea7ba229e87d664b1e6e130b79a21e7455939462e0b61`.
Its classification is **development_dirty_non_publishable**: three task-owned
uncommitted documentation files were captured; production source was committed.
Later documentation DELIVERY does not relabel this immutable artifact as clean.
Build-authority digest is
`9d906375b75ed46854db223aa1b4657aa83aa62c707cec2f842c386157d03d60`;
Office35-file qualification digest is
`af69fde591af1aa3d66cb0d4cfa14386d03c618eb944153fa41438496b5b7538`.
Read-only `codesign --verify --deep --strict`, `hdiutil verify` and the existing
`verifyPrivateLocal` all exit0. Original build-owned ad-hoc signing is retained;
notarization is explicitly skipped by the private configuration. Both publishable
and releaseEligible remain false. The artifact stays local and is not in the report ZIP.

Installation path is NOT_INSTALLED; no actual app launch/Provider/common journey
is claimed. The current [development baseline](../development-baseline.md) records
an additional concrete gap: Electron Cookie encryption initializes OS key storage
before Core; the accepted non-login task-Keychain binding covers only Go Secret
Store. A separately admitted Chromium storage boundary is still required. Keep
Cookie encryption enabled, do not reuse the old task-login harness, and do not
access personal Keychains or profiles. This is not a blanket prohibition on Mac GUI.

### A–N and original P0–P5 remaining scope

| Section | Actual disposition / next minimum evidence |
| --- | --- |
| A/B/C | Intake and64 tool tests complete; five document errata and current evidence updated.32 scenarios have per-item coverage, not32 product passes |
| D/L | SAFETY_NOT_CLEARED: bounded original-task record lookup found historical references, no original create_tree payload/call or clearance. Keep PAYLOAD_SCOPE_UNKNOWN/NOT_CLEARED; no alternative push/API/source ZIP |
| E/P0 |67 findings/248 flows/16287 steps/16783 endpoint-inclusive records retained. All67 sink-file blobs equal65ab,50 original-line UNCHANGED/17 CHANGED remain; not all-caller proof or fresh scan. #33 original containment evidence reused; dynamic-policy and stress/timing/global-resource tests not newly run. CodeQL absent from current PATH and checked conventional tool locations, not asserted absent everywhere |
| F/P0 | Exact original target and bounded page controls observed; Rust root cause unresolved, original main failure remains |
| G/P1 | DATE/link structure guard verified; default typography drift remains EVIDENCE_FAILURE/SOURCE_GAP in fixed filter/codec preservation. Full Diff/save/recovery/native acceptance incomplete |
| H/P2/P3 | Browser explicit action and child rejection implemented. Actual guest/Core/installed journey and image GUI remain EVIDENCE_NOT_RUN. Imported pivot/chart edit/refresh, stable imported-object binding and wider PPT style integration remain SOURCE_GAP; existing real pivot generation and typed cells/geometry/fill retained |
| I/P4 | SOURCE_GAP: desktop archive install still fails closed. Existing Go FormalPackageBinding validates pre-admitted immutable package/source identity, not an arbitrary Hub archive digest. Archive identity/trust/envelope→stage→generation→desktop revoke/uninstall integration remains absent. Required admin/local-remount protections retained; no fake Host API |
| J | H01–H08 existing owners and evidence mapped below; broad egress/cache/restart/hook journey not declared complete |
| K/P5 | DMG build exit0; signature/DMG/35-file Office content checks exit0; development_dirty_non_publishable, not installed. Accurate installed/common native/provider journey is unaccepted; the current development baseline also records a missing admitted Chromium cookie-encryption storage boundary before Core; the Go task-Keychain binding alone is insufficient. Keep encryption enabled and do not reuse the old task-login harness. No personal/old Keychain or profile is read or reused |
| M | Conditional final publication is the current goal; source refusal, fixed-binary build/relink/license materials and exact installed/RC/release gates are unmet. No tag/Release/public upload |
| N | Three focused source commits and canonical evidence update; report-only next handoff REVIEW_ONLY. OWNER_HANDOFF_NOT_RELEASED; clean is not writer release |

Harness mapping (source inspection unless fresh test named): H01 existing
`app/pluginpackagehost/skills.go` rechecks generation/principal/digest before and after
consumption, backed by prior installed-filesystem tests; H02 current Workbench
held-queue revocation tests pass; H03 `server/runtime_restore_execution_grant_test.go`
contains approved/non-required/report-stage unknown-outcome recovery without resend;
H04 existing `async_terminal_closure_test.go`/`event_bundle_delivery_test.go` own
cancel/terminal/cursor semantics; H05 `app/thread/compaction_privacy_projection_test.go`
checks durable exact privacy proofs; H06 `auto_compaction.go` preserves exact-input
budget handling, without inventing a competitor auto=false setting; H07
`app/turnsecurity/service.go` frozen hooks do not replace execution authority;
H08 workspace/attachment/projector/recovery seams remain existing owners, not a
second RAG. No new general hook/autonomous-Skill framework or unmeasured token claim.

Fresh read-only GitHub observation2026-09-21T14:05:05Z: PR28 OPEN/Draft/BLOCKED,
HEAD `dde784437dc8563e84066629dd57f4a11fd9acc9`; actual main
`ce96cf12581acfa0e19fae7c6aa9c709371012c8`; two unresolved filestore review threads.
Strict Development gate/app15368, no bypass, resolved threads and extra
unattributed-change approval remain applicable. Old CodeQL/main checks do not test
SOURCE4343. MERGED_MAIN_SHA=NOT_MERGED; OBSERVED_MAIN_SHA=ce96cf1…;
VERIFIED_MAIN_SHA=NOT_VERIFIED. Complete product, Formal RC and public release are
not achieved. No current-source push, CI dispatch, review resolution or merge occurred.


## Historical continuation A–M: review package and Browser implementation

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

## Historical remaining P0–P5 at 9cf / 40a6 — superseded by later 65ab / 4343 sections

The Browser source-chain gap below records the earlier candidate. SOURCE65ab implements that chain; SOURCE4343 adds the explicit action and child-frame refusal. Actual guest GUI and child-frame capture remain unaccepted. Use the current A–N/P0–P5 table above, not this historical row, to choose new implementation.

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

Historical 65ab source change totals: production_contract_config: 25 files +609/-14; test: 9 files +495/-2. Documentation totals and final DELIVERY are recorded after the documentation checkpoint; no tested source is changed by it.
