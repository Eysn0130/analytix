# Canvas scoped quotation and durable review — 2026-09-17

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
