# Canvas scoped quotation and durable review — 2026-09-17

## Consumer and case-authority continuation — 2026-09-18 UTC / 2026-09-17 Los Angeles

Status: Reference / deterministic source evidence. Applies to the source/test batch
containing this addendum, continuing the authentic local `de709d9c322948e786f87b84fffede3d9f988296`
(tree `b0c223c10c64d25c1cb013fd21aa463470871b78`, parent `dde784437dc8563e84066629dd57f4a11fd9acc9`).
The inherited capsule's 37 files, modes, SHA256 hashes, Git parent/tree and bundle were
verified in a complete independent checkout; the previously unavailable older writer's
uncommitted work was not recovered or claimed. It is not a prerequisite for this continuation.
Remote integration and CI must be read at the resulting real SHA in PR28 comment
`5709839864`; local passes and API action discovery do not prove a ref update.

### Confirmed consumer defects and existing-owner repairs

The new `Workbench.canvas-send.test.tsx` mounts the actual Workbench and uses the actual
plan controller, Composer draft hook, chat send action and native/document reference stores.
Only presentation leaves and external Core/Provider/settings/files/retrieval/checkpoint I/O
are replaced with controlled synthetic boundaries. The submitted handler, identity checks,
reference selection and cleanup paths are not copied or mocked into passing.

The original consumer implementation failed six of the first 17 regressions: navigation
could write an old preparation error into the new thread; plan directory I/O could retarget
an old Canvas prompt; settings and checkpoint I/O could outlive the Core check; an old
Provider acknowledgement could mutate the new thread; repeated clicks could duplicate
preparation. The exact same 17 tests passed after repair; RED and GREEN logs are retained.

Workbench now deduplicates in-flight preparation per workspace/thread and fences errors to
its mounted owner. A renderer-local, one-submission callback reaches the existing plan/send
owners; the send action invokes Core again after its final settings/checkpoint preparation
and immediately before calling the Provider adapter. Failed admission removes only its own
optimistic state. This callback is never serialized, sent to a Provider or retained in the
ordinary busy queue; Canvas submissions rejected as busy retain the draft and scope.
Late acknowledgements/errors cannot mutate another selected thread. Draft revision and
object-identity cleanup remain in the existing Composer owner, preserving later input,
attachments, files and replacement Canvas references. Plan metadata uses the user request,
not transient expanded scope instructions. Office's established message path is preserved.

The complete new file contains 23 tests: ordinary and plan sends; no automatic send on quote;
Core denial/exception, file denial/exception and Provider failure; document/file/plan-directory
and final settings/checkpoint races; busy refusal; late success/failure; duplicate preparation;
post-submit edits; model-prompt privacy and Office compatibility. These are deterministic
consumer tests, not real Provider or installed-GUI evidence.

### Real case authority, with a deliberately bounded claim

`canvas_case_authority_test.go` reuses the existing production Host/projector/Registry/CAS
assembly through a minimal fixture extraction. Synthetic cases use the real signed case
Registry, real case-binding observer, installation signer, thread risk-policy CAS store,
`HostPolicyAuthority`, and actual context-epoch transitions. The fixture principal and
root-scoped test private-CAS lease are synthetic; this is not independent-witness admission.

Four top-level tests (one has seven subcases) cover legitimate ordinary-artifact
capture/read/propose/review/explicit accept/reopen/undo, different-thread refusal, and paired
refusal before read/propose/accept after case binding, committed epoch, quarantine, unavailable
risk record, Host disable, source CAS or authority-store reload/session reopen changes.
Source bytes remain unchanged on rejected writes. The actual current ordinary-effect check
passes while the case-data-effect check rejects this same non-DSV2 context.

This matches accepted ordinary-file behavior in
`openspec/specs/case-claim-publication-gate/spec.md` and
`openspec/specs/case-data-forensics/spec.md`: a case workspace does not make ordinary files
uneditable, and ordinary editing does not grant dataset/evidence publication authority.
No dataset, global PII mapping or second permission engine was created. Authority reload plus
session reopen is not claimed as a full application/process restart or complete case-AI journey.
Initial fixture-only failures used the wrong no-dataset sentinel and a mixed signing-authority
root; those fixtures were corrected to the existing domain constants and dedicated private root.
Production assertions and authority checks were not relaxed.

### Current execution evidence and actual resource boundary

Independent Linux x86_64 uid1000, isolated HOME/TMPDIR/caches, umask077; Go1.26.4 and
Node22.22.1/npm10.9.4, offline locked modules. UI regression: **43 actual files / 732 PASS**,
0 skipped/failed. Both TypeScript configurations, five changed TS/TSX lint targets, runtime
TypeScript build and Electron main/preload/renderer source build pass. These source commands
do not execute or substitute for the aggregate native-install/package admission wrappers.

| Current checked scope | Result |
| --- | --- |
| Complete affected eight Go packages, normal | 983 top-level PASS / 951 nested PASS / 4 existing top-level SKIP / 0 FAIL |
| Same packages, `analytix_prod` | 996 top-level PASS / 951 nested PASS / 4 same SKIP / 0 FAIL |
| `-race -run TestCanvas` in Core/server/filestore, plus full Plugin Host | 40 top-level PASS / 82 nested PASS / 0 SKIP or FAIL across 4 packages |
| Production-tag Go runtime source build | PASS, independent Linux output only |

The seven non-server packages and full server ran separately in each normal/prod mode,
with `-count=1 -p 1 -parallel 2 -json`; server used the existing 360-second package budget.
Race selected `TestCanvas` in canvasediting/server/filestore and ran the complete
pluginpackagehost race suite separately. No other test names are implied by that selector.
All original four skips remain unchanged. These are affected-package checks, not all-repository
Go or full native-install CI acceptance.

The preserved 17-test RED is not counted as a new pass, and describes five defect categories
with two distinct final-I/O boundaries. Repeats, normal/prod/race modes, top-level/subtests
and suite/describe counts are not added together as coverage. Full source inputs, manifests,
commands, timestamps, exits, fixture/setup failures and the earlier typecheck external timeout
are retained in the execution capsule. Product gates, dependencies and lockfiles are unchanged.

A Chromium browser, the real panel/Main controller and a loopback real-Go-Host test gateway
were prepared. The **first browser navigation was blocked by environment policy** with
`ERR_BLOCKED_BY_ADMINISTRATOR`. No browser UI assertion or screenshot ran. The policy was
not modified or bypassed; gateway setup/cleanup PASS is not UI acceptance. Native Electron
installation, macOS/Windows GUI, Chinese IME, authorized real Provider/media and RC/signing
remain separate unmet prerequisites. No user Mac, old profile/Keychain, case data or credentials
were accessed. Existing CodeQL 37-high history and DuckDB inline failure/non-reproduction are
not closed by this batch. Fresh reviews show two unresolved path threads; four other threads
were already bot-resolved, not repaired or dismissed by this continuation.

## Historical de709 source-candidate evidence (unchanged results)

The remainder records the received candidate before this continuation. Its old routing,
NOT_PUSHED status and counts are historical, not a reason to stop current supported GitHub
integration or to replace this round's exact-input verification with old CI.

Status: Reference / verified local source candidate, NOT remote CI or release admission.
Baseline: `dde784437dc8563e84066629dd57f4a11fd9acc9` on
`codex/workbench-product-delivery-20260914`; PR28 remains Draft.
Candidate identity: the commit containing this source/test/evidence batch, plus the
external capsule's exact commit/tree and file manifest. No self-referential follow-up
commit is required. Current user authorization permits independent construction from
the committed baseline; the unavailable earlier uncommitted candidate was not recovered,
deleted, adopted, or counted. The local candidate is not yet integrated remotely.

## As-built vertical slice

1. The existing Canvas preview owner retains at most two presentation handles and local
   selected IDs, never cached document authority or bytes. Suspend differs from close;
   A→B→A rereads Core. Thread changes close old owners; third-object eviction and tab
   closure must succeed before ownership is discarded. Failed closes stay recoverable.
2. The existing native-reference store has a Canvas discriminant. Capture is a fresh Core
   operation over actual selected nodes/edges, a current source revision, the materialized
   Host binding and Registry capture. A quotation is not a Send. Composer validates Core
   after asynchronous document/file preparation; failed preparation preserves the existing
   draft, attachments and references. Raw Scene, path, node/source IDs, labels and notes
   are not appended to the Canvas model reference. Office's existing path remains separate.
3. The original `native_selection_read` / `native_selection_propose` tool dispatcher accepts
   a finite, exclusive Canvas operations payload. The existing production projector checks
   current authority; it projects actual source facts, creates bounded ephemeral aliases,
   withholds known identity/institution fields and repeated selected private values, and
   retains allowed exact amounts. Source locators/notes remain local. Only selected aliases
   can target presentation operations; facts and unselected objects cannot be rewritten.
4. Frozen invalidation fingerprints bind the complete local principal, object/thread/path,
   case/dataset/epoch/publication tuple, stable risk head and Host generation/activation.
   This hash is not a grant. Current identity/risk/epoch validation runs first. Only a
   witnessed risk binding's fresh challenge entropy is excluded; stable head/policy fields
   remain bound. The first legitimate general-thread Send does not destroy a valid scope.
5. Core derives real candidates and before/after differences. The existing protected-local
   object journal stores bounded review intent with predecessor CAS, private roots/files
   and the original path/identity checks. It does not persist selection grants or revive
   aliases. Reopen mints new proposal IDs, rereads current bytes and rederives the candidate;
   incompatible intent is stale. Native pending/current operations retain original IDs.
6. The review panel renders escaped real differences and offers explicit accept/reject,
   undo and permitted pending-operation resume/cancel/retry-undo. Propose does not write the
   formal file. Acceptance rechecks identity, context, Registry, review predecessor and
   source CAS before the original native commit. Status never calls Apply/prepare/resume.
   A lost reply cannot automatically issue a fresh write; successful save and failed
   subsequent preview refresh remain distinct user-visible outcomes.
7. PNG rotation, crop and rectangle marking use the existing deterministic local Core
   transformations and the same reviewed save/recovery owner. They are not model-generated
   media, semantic image editing, or evidence of an authorized media Provider invocation.

No new runtime, permission authority, Provider loop, storage engine, dependency, lockfile,
workflow, CI filter or gate was added. Canvas-only styles use existing design tokens;
this batch does not implement or replace the independent UI Refresh task.

## Deterministic verification

Independent Linux x86_64, non-root uid1000, isolated HOME/TMPDIR and caches,
Go1.26.4, Node22.22.1/npm10.9.4; locked inputs came from checksum-verified existing
workflow artifacts. `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `umask 077`.
No user Mac, old profile/Keychain, real case dataset, real Provider or credentials were used.
The development-source materialized Host is real; the injected principal and source data
are synthetic. This is not an installed package or physical GUI acceptance run.

| Scope | Completed result |
| --- | --- |
| Affected 8 Go packages, normal | 979 top-level PASS; 944 nested subtest PASS; 4 existing top-level SKIP; 0 FAIL |
| Affected 8 Go packages, `analytix_prod` | 992 top-level PASS; 944 nested subtest PASS; same 4 existing SKIP; 0 FAIL |
| Focused Canvas/selection race plus complete Plugin Host race | 41 top-level PASS; 91 nested subtest PASS; 0 SKIP / FAIL |
| Renderer/Main/Canvas/Office/contracts/Composer | 437 PASS across 30 actual test files; 0 SKIP / FAIL |
| Type checking | Both `tsconfig.web.json` and `tsconfig.node.json` PASS |
| Changed TypeScript/TSX ESLint | 16 files; 0 errors and 0 warnings |
| Source compilation | runtime TypeScript package and Electron main/preload/renderer PASS |
| Go runtime source build | `go build -tags analytix_prod ... ./cmd/runtime-server` PASS on Linux x86_64 |
| Formatting | All 15 changed/new Go files formatted; `git diff --check` PASS |

Modes/repeats and nested subtests are not additive coverage. The race selector initially
matched zero tests in the Plugin Host package; its complete race suite was subsequently
run separately and is included in the 41/91 totals. The UI JSON reporter's 54 suites are
nested describe groups, not 54 files. These are affected-surface checks, not whole-repository
`go test ./...`, all application tests, GitHub CI or release-gate results.

### Reproduction commands

Use the repository's applicable environment instructions; on macOS the existing cache
helper remains mandatory. The independent Linux checks used the isolated toolchains above.
Run the following in `packages/runtime-go` (normal and production were separate runs):

```sh
umask 077
go test -count=1 -p 1 -parallel 2 -json \
  ./internal/app/canvasediting ./internal/app/objectediting \
  ./internal/app/officeediting ./internal/app/pluginpackagehost \
  ./internal/app/toolcatalog ./internal/adapters/outbound/filestore \
  ./internal/architecture
# Repeat this package command with: -tags analytix_prod
# Full server normal/prod ran separately, sequentially, with no test selector:
go test -count=1 -timeout=360s -p 1 -parallel 2 -json ./internal/server
go test -tags analytix_prod -count=1 -timeout=360s -p 1 -parallel 2 -json ./internal/server
go test -race -count=1 -p 1 -parallel 2 -json \
  ./internal/app/canvasediting ./internal/adapters/outbound/filestore \
  ./internal/server -run 'Test(Canvas|NativeSelection|Production|PluginPackageHost)'
go test -race -count=1 -p 1 -parallel 2 -json ./internal/app/pluginpackagehost
go build -tags analytix_prod -o /task-owned/output/runtime-server ./cmd/runtime-server
```

The capsule's command metadata and 30-file UI list preserve exact invocations. At root,
`npm run typecheck`, changed-file ESLint, `npm --prefix packages/runtime run build` and
`node_modules/.bin/electron-vite build` passed. The aggregate install/build/package wrappers,
Node lifecycle/native installation, signing and notarization were not executed or bypassed.

### Preserved failures and repairs

- Five added preview lifecycle regressions failed against the old implementation; all
  13 manager tests passed after the change. Filtered tests were not new `.skip` directives.
- Initial new Host fixtures exposed a real private-root requirement and an unstable test
  risk-authority substitute. The fixture now uses a 0700 receipt directory and the real
  `GeneralOnlyAuthority`; production authority checks were not relaxed.
- An existing export test failed under umask0022 even on the untouched `dde` baseline:
  `TestObjectExportRealFileMixedCaseWorkspaceAndAuthorityDrift` constructed a non-private
  temporary root. It passed on that same baseline under umask0077. The test and root guard
  were not edited. Earlier root/setup/transport errors are not passed product checks.
- Concurrent full server attempts and an over-tight 60-second baseline diagnostic hit
  local execution limits during an unchanged CPU-heavy long-context test. Sequential full
  normal/prod server runs completed in about 228/223 seconds under the same assertions.
  No repository timeout, retry, parallelism, test expectation or gate was weakened.
- Subsequent review moved Composer Core validation to the last asynchronous preparation
  step, after document and file reads. Affected typecheck/source build were rerun; GUI
  sending remains a separate acceptance seam. Go import-only formatting is not a new
  behavior; affected object packages and focused race checks were rerun afterward.

The four unchanged skips are `TestGeneratedDocumentBundledCodecPackageContract`,
`TestObjectEditingFilesystemAliasesShareReceiptBinding`, `TestOfficePackageSavedSyntheticFixtures`
and `TestCreatePlanAutoSelectionDoesNotOverwriteFilesystemAliases`.

## Remaining acceptance boundaries

This slice now has source consumers and deterministic evidence, but PR28 is not merge-ready.
The local commit was not pushed; original remote PR metadata/history/checks were not changed.
After integration, fresh required CI must cover the actual resulting SHA/tree.

The inherited CodeQL 37-high finding set still needs complete source→sink/reachability
review and appropriate fix/rescan; no alerts were dismissed or claimed closed here.
The historical Rust/DuckDB inline failure and later non-reproduction remain separate,
unchanged evidence, not a repaired root cause. No affected Rust source was changed.

Independent native installation, actual Electron GUI, Chinese IME, cross-platform object
journeys, real Provider/media authorization and fees, Office fidelity, existing-state recovery,
A0/B1/ProductRC/FormalRC, signing, notarization and release remain separately unverified.
The general-authority real Host tests and fingerprint negative cases are not an exhaustive
case-mode end-to-end or semantic-PII/prompt-injection security proof.

Resume from [the existing handover index](../handovers/README.md), then fresh GitHub and
current worktree. Preserve unrelated and later-arriving work. Do not merge/rebase, rewrite
history, modify the independent UI task, lower gates or infer release authorization.
