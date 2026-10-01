# Funds current-native vector checkpoint — 2026-09-30

Historical checkpoint for the configured darwin-arm64 host. Overall status:
`partial`. It is not installed acceptance or release evidence. The initial
two-file candidate is historical; later dated sections bind each subsequent
source overlay, test binary and result. The latest checkpoint is the
2026-10-01 fresh-process contract candidate below.

Latest candidate status: seventeen-file Go overlay compiled, three recovery
contract/journal regressions passed, and A/B numeric/row/binding/provenance
assertions passed in the current native run. That run timed out during reopen's
normal codesign verification; Owner-close and fresh-process checks were not
reached. Source-update/missing/corrupt/fresh checks are unverified for the latest
candidate. All changes remain uncommitted. The protected four and frozen native
four are preserved. See the final dated section and its exact receipt.


## Candidate and isolation

- Canonical branch: `codex/workbench-product-delivery-20260914`.
- Clean product source: `8d538b3199d12fa094e999cbf1a7b91f616dc803`, tree
  `f83c21c88ced3203e16f851b0a5eda537137b71d`.
- A fresh Git archive and detached worktree under the verified development
  cache exclude the four protected unrelated dirty files. Those files remain
  unchanged in canonical; no source is replaced from cache.
- The test compiler uses that clean product source and a Go overlay for only
  these two canonical candidate test files:

  | Initial compiled test file (superseded below) | SHA-256 |
  | --- | --- |
  | `packages/runtime-go/internal/runtimeapp/funds_account_flow_b1_public_chain_darwin_test.go` | `cea03984b9b1aaeaaed53795fa5670038bbc57b69e905e6780c206571f3ba0f7` |
  | `packages/runtime-go/internal/runtimeapp/funds_delivery_vector_public_chain_darwin_test.go` | `c9e596d2c9ffb5031ad9731d0c6f9bdd5fafc3b4cc4c0ee697fbc1fa0ba36165` |

The harness composes production Go HTTP services with a synthetic Host and
loopback Provider. When the native test runs, it must select native components
from an exact clean, normally sealed development app. It does not execute the
packaged runtime-server or establish installed GUI or real-Provider acceptance.

## Candidate changes

- Exact current-native input selection no longer requires the historical
  `ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER` identity. Current source identity,
  clean development classification, normal package resource seal and Owner
  admission remain required; the historical relocation checks remain intact.
- The synthetic Provider captures every received request body before route
  or authentication rejection. It reports body lengths and completeness,
  including errors. Normalized input measurements use an explicit unmeasured
  value and flags. It retains no raw body in logs. These are received body
  bytes, not wire/header bytes, tokens or unseen connection attempts.
- The vector test checks net as well as inflow/outflow, exact bounds, currency,
  scale, account, timezone, coverage and seven transaction values. An independent
  CSV integer oracle compares row multiplicities. Evidence reference to value
  mappings must remain stable across A and B on the same immutable snapshot.
- After reopen, the negative test closes the actual active native Owner. It
  requires a readable completed turn with the expected source-unavailable
  boundary, no evidence-backed final view, no additional received Provider
  request and no change to the complete preparation-store digest. It does not
  label this outage as persistent DSV2 revocation.

At this initial checkpoint, these changes were an uncommitted test candidate
and the native vector assertions had not yet passed against a new sealed app.
Later sections preserve the subsequent partial executions and remaining gaps.

## Executed checks

| Check | Result and ceiling |
| --- | --- |
| `gofmt` and `git diff --check` | `pass` for the candidate changes |
| Clean-source `go test -c`, `-tags analytix_prod`, two-file overlay | `pass`; compile only |
| Compiled binary: `-test.run '^TestFundsDeliveryDiagnosticKeepsFiniteThreadFailureCodes$' -test.count=1 -test.v -test.timeout=120s` | `pass`, final exit 0; finite diagnostic codes only |
| Independent Python Decimal and in-memory SQLite integer calculation | `pass`; static reference only, no native execution |
| Archive `runtime-assets.mjs prepare --target darwin-arm64 --from /Users/sun/Projects/analytix` | `pass`, five files copied, no missing or invalid entries |
| Normal `npm run build:data-native:development -- --platform darwin --arch arm64` | `pass`, final exit 0; all four components compiled and passed normal ad-hoc signature validation, generation published and temporary Cargo directory cleaned |
| Native source/context digest comparison | `pass`; canonical, clean archive and published marker match |
| Independent dependency copies and normal archive `npm run postinstall` | `pass`, final exit 0; root install freshness now passes and sampled dependency inodes are independent |
| Clean app `npm run build` | `pass`, final exit 0; archive HEAD remains exact and tracked worktree status is clean |
| Normal pinned builder `--dir` | `pass`, session `97149`, final exit 0; exact-source normal development seal and native admission subsequently verified |
| Current-native admission | `pass` with isolated runtime temporary storage; initial cache-temporary admission failure retained below |
| Initial current-native vector | `fail`; actual A/B native queries reached Provider, but three harness assertions were incorrect |
| Corrected current-native vector | `partial` numerical evidence, overall test `fail`; exact A/B assertions passed, Owner-close continuation failed its async completion guard |
| Current source-update public chain | `partial`; source-update/reopen assertions passed, retained display later exceeded the 20-minute test timeout |

All build and test commands source `scripts/use-analytix-cache.sh` in the same
shell. The compiled test uses the normal local full-content/ad-hoc policy
identity; it does not alter manifests, receipts, source identity or admission
checks. No paid Provider, credentials, live user state, push, PR, merge or
release operation was used.

Compiled test binary SHA-256:
`5bcb5c5525a12005337c1500978e0b8ba79b70199a6794d7bc77e147a7ae60b1`.
Receipt: `/private/tmp/analytix-funds-current-native-20260930-compile.json`.
The exact compile route was:

```sh
source ./scripts/use-analytix-cache.sh &&
cd /Volumes/AnalytixCache/development-v3/tmp/analytix-funds-native-20260930-ae0__i3m/source/packages/runtime-go &&
GOCACHE=/Volumes/AnalytixCache/development-v3/go-build-longitudinal-20260930 \
go test -c -mod=readonly -buildvcs=false -tags analytix_prod \
  -overlay /private/tmp/analytix-funds-native-20260930-test-overlay.json \
  -ldflags '-X analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs.embeddedReleaseProfile=full -X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningPolicySHA256=7625f1de94282690dc72ce92f7e42a293437c24fb9f4a725ff82bfec157ecaf3 -X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningMode=ad-hoc' \
  -o /Volumes/AnalytixCache/development-v3/tmp/analytix-funds-native-20260930-ae0__i3m/runtimeapp-native.test \
  ./internal/runtimeapp
```

## Independent reference

Synthetic fixture SHA-256:
`6894ac103a21260de3260aba39a49c15749ee2ec1db52842f5764abc6fc3cc82`.
The selected import has nine rows; the target account/window has seven
transactions. Decimal and SQLite integer calculations agree:

| Value | Expected |
| --- | --- |
| Currency / scale | CNY / 2 |
| Inflow minor units | `1301001` |
| Outflow minor units | `120060` |
| Net minor units | `1180941` |

The harness derives this CSV by selecting the fixed case, snapshot and CNY
records and explicitly excluding `A011` and `A001_DUP` before import. The
nine-row admission and seven-row window do not prove general product deduplication.

This establishes the static reference, not a native result. The isolated
reference receipt is `/private/tmp/analytix-funds-current-native-20260930-oracle.json`.

## Capacity and remaining evidence

The published native development marker is `development_non_publishable`,
`publishable=false`, `releaseEligible=false`, `authorityUse=development_only`.
It is not the sealed app's package authority. Its source-set digest is
`44e02fd554d48f229e3782df5869ad5fa52899f23fb2e589cff702e3bab3cc6a`;
build-context digest is
`e8e5729c84875aac9beaf7552b1fe377baa2057b35a3d823bfd0cf1de4b45f99`;
marker SHA-256 is
`3afb652adeb311a55c00ebbc6e708551ada289d67c89c12d51473d5d77f15263`.
Both source and context digests independently match canonical and the clean
archive. The completed native command's session was `90819`.

After normal builder cleanup, the inner cache has 3,012,153,344 available
bytes (approximately 2.805 GiB); backing has 49,179,131,904 available bytes
(approximately 45.8 GiB). The task-owned Cargo directory is gone. These are
post-cleanup readings, not a package peak.

Known historical static allocation for root dependencies, runtime dependencies
and one app totals 2,392,484 KiB (approximately 2.282 GiB). It excludes generated
outputs, app/asar staging, concurrent native/runtime copies and an explicit
margin. No trustworthy measured archive plus cold `--dir` transient peak was
found. The static sum must not be used as the complete package requirement.
For this preflight, the planning allowance additionally includes another
historical app-sized staging copy (906,010,624 bytes), a native source-side
materialization copy (137,740,288 allocated bytes), and a 512 MiB margin.
That totals 4,030,525,440 bytes (approximately 3.754 GiB), exceeding inner
availability by about 0.95 GiB. This is a conservative planning allowance,
not a measured current-candidate peak or proof that this amount alone would
suffice. At the first checkpoint the capacity preflight was `blocked` and app
build/package work did not start; this verdict is superseded below. The receipt is
`/private/tmp/analytix-funds-current-native-20260930-generation-capacity.json`.
Foreign caches and retained stages are preserved; the long-lived isolated
runtime PID 59851 remains untouched.

At the initial checkpoint the following were `unmeasured` or `unverified`;
the later execution records below supersede only the stated items:

- Actual current-native numbers, complete received Provider request totals and
  source-unavailable behavior for this candidate.
- Actual started SQL statement counts; tool-command counts are not SQL counts.
- Authenticated seven-row DSV2 material-graph verification; row values and stable
  reference mappings alone do not establish it.
- Persistent DSV2 revocation. The candidate closes an active Owner instead.
- Current native changed-snapshot, missing/corrupt retained-source and fresh
  process recovery checks.
- Numerical answer reuse or efficiency improvement. B performs a fresh native
  query; metadata continuity and larger context do not prove either claim.

The capacity verdict above is superseded by the same-task resource continuation
below. Reuse the completed normal native generation only while its source/context
identities still match, then build the clean app and execute the isolated native
tests. Do not
substitute historical packages, patch authority markers, weaken admission or
delete unowned cache contents to obtain a pass.

## Same-task resource continuation

Actual turn evidence, recorded at `2026-10-01T04:10:47.319Z` (2026-09-30 on the
configured client), is turn `01a0f5a8-5e82-7ea1-88bc-e3186dadfc6c`:
`model=gpt-6.1-sol`, `effort=ultra`, `approval_policy=never`,
`sandbox_policy.type=danger-full-access`. This is the current runtime
`turn_context`, not an inference from the UI or the preceding turn. No safety
configuration was changed.

The exact authorized target is
`/Volumes/AnalytixCache/development-v3/go-build-longitudinal-20260930`.
Explicit `GOCACHE` plus `go env GOCACHE GOMODCACHE GOVERSION` confirmed this
target, the separate unchanged module-download cache and Go 1.26.4. The target
and every path component have no symlink; realpath is unchanged. Device/inode
are `16777244/103713533`, UID is 501 and mode is 0700.

All shard entries were examined: 7,847 action entries and 2,960 data entries,
each an owned regular file with the expected 64-hex `-a`/`-d` name and shard
prefix. Root entries were the 256 hex shards, `README` and `trim.txt`; no
non-cache content, nested directory, symlink or cross-device entry was found.
Independent read-only review confirmed the local official Go cleanup boundary.
The final pre-mutation check found no active Go/compile/link process and no open
handle to the target; the host `lsof` scan returned 0 without warnings.

After sourcing the helper in the same shell, the executed official command was:

```sh
GOCACHE=/Volumes/AnalytixCache/development-v3/go-build-longitudinal-20260930 go clean -cache
```

It returned exit 0. Default global cache, module cache, other candidates and
runtime 59851 were preserved.

| Measurement | Before | After |
| --- | --- | --- |
| Target allocated bytes | `1562251264` | `8192` |
| Cache available bytes | `3012153344` | `4583337984` |
| Target root contents | Go shards, `README`, `trim.txt` | `README`, `trim.txt` |

The pre-clean values were captured with directory allocation and `statvfs`;
literal pre-clean `df` output was **not obtained**. Post-clean `df -k` was
executed: cache available 4,475,916 KiB, backing available 48,026,112 KiB.
The small difference between freed allocation and the change in available
bytes is not attributed solely to this operation.

Post-clean availability is approximately 4.268 GiB, exceeding the existing
3.754 GiB planning allowance by about 0.514 GiB. The planning preflight now
passes; the actual current-candidate cold-package peak remains `unmeasured`.
Receipt:
`/private/tmp/analytix-funds-current-native-20260930-go-cache-recovery.json`.

The four native file hashes still match the published marker; the independent
test binary and marker hashes, and all four protected dirty-file hashes, are
unchanged. Source/context identities remain as recorded above. Independent
root and runtime dependency copies completed with exit 0 in session `51034`,
preserving relative symlinks and executable modes. Sampled dependency files
have distinct inodes; no shared writable dependency symlink is introduced.

Normal archive `npm run postinstall` followed by `npm run build` completed with
exit 0 in session `32452`. The root install freshness check now passes. The
archive still has exact HEAD `8d538b3199d12fa094e999cbf1a7b91f616dc803` and empty
tracked worktree status. Pre-package cache availability was 2,860,272 KiB.

The normal pinned `electron-builder@26.15.3 --dir --publish never --mac --arm64`
is running in session `97149`, using `ANALYTIX_RELEASE_PROFILE=full` for the
required content and the normal local ad-hoc signing policy. This is a
development directory carrier, not a formal Full release. Its normal SQLite
and node-pty dependency handling completed; Electron 41.10.3 was extracted and
app packaging began. The after-extract source snapshot is
`5563b8f76c5667f9b50b1be96a4a05a0a607860c8ed5e20de2b8c19826e2faf2`.
Go module verification passed in 21,945 ms and normal Go runtime compilation
completed in 616,668 ms. Funds MCP production closure (seven modules) and the
bundled Computer Use helper identity checks passed. The runtime binary and
native development marker now exist in the app, and the normal signing
subprocess is active. These intermediate steps do not establish a complete
sealed app. The final builder exit, package seal/signature and exact native
input admission remain pending. Actual two-thread numerical/request-byte validation remains
**not executed**. This continuation changes the resource/build verdicts, not
the native numerical, installed acceptance or release verdict.

## Completed carrier and initial execution

Normal builder session `97149` completed with exit 0. The carrier is
`/Volumes/AnalytixCache/development-v3/tmp/analytix-funds-native-20260930-ae0__i3m/source/dist/mac-arm64/analytix.app`.
Its normal packaged authority is schema 2,
`development_clean_non_publishable`, exact source
`8d538b3199d12fa094e999cbf1a7b91f616dc803`, clean snapshot
`5563b8f76c5667f9b50b1be96a4a05a0a607860c8ed5e20de2b8c19826e2faf2`,
authority digest
`fea0cd03587b888a83bccc78fd69d86e2df64fb21d8f7de20ede384f096c6109`.
`publishable`, `releaseEligible`, and `publicationReceiptIssued` are false.
The four normally signed native payloads retain the published development
source-set identity above. Normal ad-hoc signing and resource sealing completed;
notarization was skipped by the normal no-Apple-credentials route. No DMG,
Windows carrier, formal release or installed acceptance was produced.

The initial standalone test binary remains preserved with SHA-256
`5bcb5c5525a12005337c1500978e0b8ba79b70199a6794d7bc77e147a7ae60b1`.
It corresponds to the initial two-file hashes, not the corrected candidate.

| Initial execution | Result |
| --- | --- |
| Fixed native input admission with helper cache TMPDIR | `fail`, exit 1 in 25.22 s; `native_component_host_trust_invalid / native_component_registry_component_invalid: cleaning-ops` |
| Same app/binary admission with fresh isolated runtime TMPDIR | `pass`, exit 0 in 8.43 s; normal current-source inspection and Owner admission |
| Vector launched from repository root | `fail` before native or Provider work: relative fixture not found; corrected working directory on the next run |
| Vector from its owning package, runtime profile under `/private/tmp` | `fail`, exit 1 in 9.40 s; import confirm 409, native INSTALL unavailable, Provider requests 0; secondary cleanup failure retained separately |
| Vector from owning package with safe runtime profile below | `fail`, exit 1 in 218.88 s; import 9, two native preparations, four received Provider requests; harness role, row-value and binding-cardinality assertions failed |

The first admission failure's exact lower cause remains unknown; the wrapper
does not retain it. Runtime placement and I/O are contributors to investigate,
not a uniquely established cause. The `/private/tmp` vector profile violates
the immutable snapshot installer's existing no-writable-ancestor contract:
`/private/tmp` is mode 01777. That precondition was verified from source and
filesystem metadata; the outer unavailable error alone does not identify a
specific errno. No ancestor permission, installer gate or deadline was changed.

The corrected runtime profile is fresh synthetic state under
`/Users/sun/Library/Caches/analytix-funds-qa-cjomneh_`, mode 0700, with runtime
temporary storage in its mode-0700 `runtime-temp` child. Every ancestor was
verified as non-symlink, root/current-UID-owned and without group/other write.
No prior user state, credentials, Registry or Secret Store material was copied.
Build outputs, compiler caches and the app remain on AnalytixCache. This
runtime-state placement is not a compiler-storage fallback.

The failed safe-profile run captured every received Provider body in memory,
including failure paths, and logged only lengths:

| Received dispatch | Request JSON bytes | Normalized messages/tools JSON bytes |
| --- | ---: | ---: |
| A initial | 21902 | 21819 |
| A native result | 26084 | 26001 |
| B initial | 20239 | 20156 |
| B native result | 24421 | 24338 |
| Total | 92646 | 92314 |

All four body-complete and normalized-measured flags were true. These are
failed-candidate observations, not an accepted numerical or efficiency verdict.
The context measurement in that candidate included static system tag text and
cannot establish actual Host-block growth. The derived CSV SHA-256 was
`39e764977ee70279bd757d8d85947d5f98a3e6f16abdf1bf9975675e39bc4382`;
the independent reference hash above identifies the original JSONL fixture.
Those representations are intentionally distinct.

Safe logs remain under `/private/tmp/analytix-funds-current-native-20260930-`:
`admission.log`, `admission-private-temp.log`, `vector.log`,
`vector-package-dir.log`, and `vector-safe-profile.log`. No raw Provider body
is retained in these logs.

## Corrected candidate

The harness now distinguishes the static inline tag description from a unique,
final, valid-JSON Host block on its own lines. Recognition precedes, and does
not replace, the strict user-role requirement. A focused regression covers
inline instructions, valid final blocks, duplicates, trailing text and invalid
JSON. Public transaction rows use the existing `inflow`/`outflow` direction
constants and exact minor-unit magnitude; the imported signed source decimal
is a separate contract. Complete seven-row queries require exactly seven
canonical bindings and seven unique receipt source IDs, matching their own
Provider evidence references. B1's deliberately partial one-row evidence
assertion retains its exact one-binding requirement.

| Corrected overlay test file | SHA-256 |
| --- | --- |
| `funds_account_flow_b1_public_chain_darwin_test.go` | `19c4c1890a975205b04e35e8de2acdcda1097a10c5d0661e531574e6cc249d1b` |
| `funds_delivery_vector_public_chain_darwin_test.go` | `9af5a0adb05dbd719d1c76d9e09b6d2a26fded2be981cb2b679204e66be03b80` |

Precompile availability was 3,971,956,736 bytes. A conservative allowance of
2,435,592,192 bytes covers the previous compiler-cache allocation, another test
binary, a transient allowance and margin; this is not a measured peak.
The same clean source and two-file overlay were compiled with the same
production/full-content/ad-hoc identity flags, using the helper's normal global
`/Volumes/AnalytixCache/development-v3/go-build` cache. The explicitly cleaned
dedicated cache remains preserved. The new output filename is
`runtimeapp-native-context.test`, preserving the old binary. The four native
components are reused without changing their source or cold rebuilding them.
Precompile receipt:
`/private/tmp/analytix-funds-current-native-20260930-context-precompile.json`.
Compilation session `9285` completed with exit 0. The new binary is 68,033,010
bytes, SHA-256
`06a0d26ec64b663e015a5ce3d3e47ce201757d9b811db771becbc16a0c869a89`.
Both overlay hashes still match their precompile values and the old binary is
unchanged. Compile receipt:
`/private/tmp/analytix-funds-current-native-20260930-context-compile.json`.

The new binary's focused semantic-block and finite-diagnostic regressions
completed with exit 0 in session `88805`. Safe log:
`/private/tmp/analytix-funds-current-native-20260930-context-regression.log`.
The corrected vector ran from its owning package with the verified safe
runtime profile in session `44889`. It exited 1 after 1050.33 s. The exact A/B
numeric, seven-row, canonical binding, receipt and reference checks reached
their positive log; both final Provider semantic requests matched the independent
reference (`2/2`) and two signed native preparations were observed. Both first
requests had one system and one user message. Context-bearing user-message
bytes were A 296 and B 2879; these include the user prompt, not only semantic
JSON. B received all three current A claim metadata items and performed a
fresh native query. This does not prove answer reuse or an efficiency improvement.

Every received request was captured. Final arrays were
`request_json_bytes=[21902,26084,20239,24421]`, total `92646`, and normalized
`messages_tools_json_bytes=[21819,26001,20156,24338]`, total `92314`.
All completeness and measurement flags were true. There was no fifth received
Provider request during the failing continuation. These totals cover requests
received by the loopback server, not unseen connection attempts, wire headers,
tokens or cost.

Reopen recovered two fact records with two admitted and zero held. After the
actual Owner close prerequisite succeeded, the third turn finished with
`completion_phase=case_longitudinal_append`,
`completion_error=deadline_exceeded`, and `failure_record_error=unclassified`.
The runtime also emitted `ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED`.
The strict R131 guard failed. Failure logging observed two preparations and two
one-entry capsules, but the later whole-store digest, source-unavailable final
view and fresh-process assertions were **not reached**. Counts alone cannot
establish unchanged evidence contents. The vector is therefore **not pass**.
No guard or timeout was weakened, and no positive overall-chain log was emitted.
Safe log and structured receipt:
`/private/tmp/analytix-funds-current-native-20260930-context-vector.log` and
`/private/tmp/analytix-funds-current-native-20260930-context-vector.json`.

The independent existing B1 source-update/retained-source chain in session
`43891` exited 2 at the unchanged 20-minute test timeout against the same stable
binary and app. It passed exact source-update aggregate values `1250/225`,
transaction count `2` with one evidence row, distinct immutable lineage,
two successful async completions, reopen recovery, original-slot preservation
and current preview checks. It timed out in retained accepted-slot display;
later missing/corrupt material and fresh-process assertions were not reached.
The timeout can bypass normal fixture cleanup, so its isolated synthetic
evidence is preserved. Safe log:
`/private/tmp/analytix-funds-current-native-20260930-context-source-update.log`.

The timeout stack is runnable inside a secure CAS authorization inventory scan,
reached by observation persistence during currentness checks inside two nested
retained-selection callbacks. This is an observed stack, not a measured stage
duration or proof of the vector timeout's unique cause. The Authority mutex is
released before entering its fresh-head callback; nested access to the same
Authority is not evidence of a self-deadlock.

A narrow retained-only Go candidate now uses the existing callback-scoped,
exact-bundle fresh witness challenge for the retained lease's pre/post checks.
Ordinary `UseExact`, post-native outer-use restrictions, initial complete
observations, all current/historical DSV2 material and ancestry checks,
post-callback verification and the 15-second terminal budget remain required.
No native component is changed or cold rebuilt. The candidate remains
uncommitted while negative-path success criteria are unmet.

The nested-retained real-Authority regression failed before the production
change: two callbacks caused eight observation writes, eight observation
readbacks, eight projections and eight fresh witness calls. After the change,
both app packages passed (`datasetsnapshot` 2.303 s, `evidenceauthority` 1.874 s,
session `27494`, exit 0). The same regression requires four observation writes,
readbacks and projections while preserving eight witness calls. Coverage also
includes no-challenge fallback with four complete observations, pre/post
challenge unavailability, current/historical material tampering, mock head
digest drift, cancellation and callback errors. Existing exact capability,
post-native and real signed witness/floor/replay tests passed in these packages.
This does not measure native CAS duration or prove a legitimate external
head advance in the new mock drift test. Safe logs:
`/private/tmp/analytix-funds-current-native-20260930-retained-before.log` and
`/private/tmp/analytix-funds-current-native-20260930-retained-after.log`.

The new runtimeapp compiler input is clean base `8d538b3` plus an explicit
three-file overlay: the two corrected harness files and
`packages/runtime-go/internal/app/datasetsnapshot/sealed_service_v2.go`
(SHA-256 `bb7f6201acea4bd6aebae856e4f4d9ac185d984fa1752c1de7b71939a275b7ce`).
Its unit test SHA-256 is
`13689028de589f77f466f5b1e6879aabfadfdcd2aa7723e8370b39db8a422640`.
The already sealed app and native components still have source `8d538b3`;
the harness does not execute that app's Go runtime-server. Compilation must
not be represented as a new installed app or a sealed app containing this Go
change. Precompile available space was 9,310,576,640 bytes; all four protected
canonical files and the prior binary were unchanged. Precompile receipt:
`/private/tmp/analytix-funds-current-native-20260930-retained-precompile.json`.
Compilation session `76099` exited 0. The separate preserved output is
`runtimeapp-native-retained.test`, 68,032,930 bytes, SHA-256
`5b3af48f5bf934075f9c6fc6a7349958c2cfdd728518c49acb6bcacc2f383a16`.
All three source hashes and the unit-test hash still match; the prior binary is
unchanged. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-retained-compile.json`.
The vector rerun in session `91260` used the same secure runtime profile,
owning package directory, normal native app and unchanged 20-minute test
timeout. It exited 1 after 984.92 s. Both async query completions, exact A/B
numeric/row/binding/receipt assertions and pre-reopen thread-detail hydration
passed. Reopen again recovered two admitted facts with zero held. Values,
complete received-body arrays/totals and two native preparations match the
corrected previous run. The Owner-close continuation again failed the strict
guard at `case_longitudinal_append` with `deadline_exceeded` and
`failure_record_error=unclassified`. Later source-unavailable public-final,
whole preparation-store digest and fresh-process assertions were not reached.
Retained package regressions passing has therefore not resolved this vector
completion failure. No budget or guard was weakened.
Safe log and receipt:
`/private/tmp/analytix-funds-current-native-20260930-retained-vector.log` and
`/private/tmp/analytix-funds-current-native-20260930-retained-vector.json`.

A three-second read-only OS stack sample of this exact synthetic test process
captured native OS thread waits, including child-process wait; it did not
capture an attributable retained/CAS Go call path or locate the later budget
exhaustion. It is diagnostic, not proof of a deadlock or a successful negative
boundary. Sample is private at
`/private/tmp/analytix-funds-current-native-20260930-retained-live-sample.txt`.
The independent B1 source-update chain in session `5685` used the same
unchanged retained candidate and naturally exited 2 at the unchanged
20-minute timeout. Source-update numeric assertions, reopen, original/current
slot preservation, missing-material fail-closed behavior and exact-byte
restoration passed. Corrupt-material and fresh-process assertions were not
reached. The timeout stack is a syscall directory open during secure CAS
authorization inventory verification for DSV2 material `Read`, reached through
`verifyEvidenceGraphV2`, `ResolveWitnessedV2`, `UseCurrentLocalDisplay` and
`DirectSourcePreview`. This locates the sampled B1 operation; it is neither a
stage-duration measurement nor the cause of the separate vector append
failure. Safe log and receipt:
`/private/tmp/analytix-funds-current-native-20260930-retained-source-update.log`
and
`/private/tmp/analytix-funds-current-native-20260930-retained-source-update.json`.

Further read-only review identifies a separate countable repetition in the
append wrapper. Its success path calls full `ValidateCurrent` four times,
including twice inside the existing exact DSV2 lease. Each full validation
resolves DSV2 with two complete EvidenceAuthority observations; with the
selection/capability's own four observations this is at least twelve complete
observations. A new append-only candidate is being prepared to use the existing
inside-exact principal/risk/workspace/binding validator for those two inner
checks. The two outer full checks, ordinary `UseExact` and all canonical
store/continuity checks remain required. This independent repetition is not
yet evidence of where the vector exhausted its budget. It is not part of the
`5b3af48...` binary used by B1.

The append reader is specifically `h.store.GetThread`, a classified/normalized
durable thread view; it does not call the trusted public/local-display
projector. That transport projection belongs to a separate public read path.
The input read still does not accept the fixed terminal ctx, so contention or
read time can consume the budget. No raw repair accessor is substituted.

Read-only code review narrows, but does not resolve, that failure. Async turn
work begins with a background cancel context; HTTP deadlines are not inherited.
Source-unavailable fixed publication uses the existing 15-second terminal
budget, starting before `PersistCurrentCaseFixed`. Longitudinal append shares
the remainder. The 60-second live-evidence branch explicitly excludes this
boundary. Entering `case_longitudinal_append` means the previous publication
call returned nil; this does not identify which later operation exhausted the
budget. Append builds input through a thread read without ctx, then performs
currentness checks, latest-index/continuity scans and writes. A phase/error
label alone cannot locate the expensive operation.

Failure-record persistence starts a new independent 15-second context. A
different fixed final for a turn with an existing winner can produce an
ordinary conflict error classified as `unclassified`. That is a plausible
secondary explanation, not a branch verified by this run. Owner close itself
does not cancel the caller or directly close the CASE/CAS stores. Production
currentness resolution includes distinct authority/witness/store dependencies;
their exact downstream involvement must not be inferred from the native error.

An independent 32-KiB synthetic-file probe on the isolated runtime volume
measured write plus file fsync at about 0.078 ms, readback at about 0.010 ms and
directory fsync at about 0.001 ms; available space was 10,391,502,848 bytes. The
exact newly created file was removed after inode verification. This single
probe is not CASE/CAS scan performance or cache-volume evidence and does not
establish the timeout's cause. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-runtime-io-probe.json`.

## 2026-10-01 source-only isolation and append candidate

The current native B1 execution ended naturally before further compiler/test
I/O. A short natural idle window was left for the separately authorized cleanup
task to perform its own fresh-reference/hash gate. Its completion has not been
confirmed by the last compact snapshot; this task performed no further
deletion and preserved all app, binary and failure evidence references.

Two small independent regressions now characterize the secondary failure.
The concrete finalizer first commits a source-unavailable winner, then attempts
runtime deadline failure persistence using a fresh live context. It reaches
the exact same-turn admission error `case terminal already has another private
preparation`, with preparation, winner and terminal event bytes unchanged
(session `96786`, exit 0). The real async fixture explicitly withholds native
and bundled Host capabilities and makes zero Provider requests. Its natural
fixed publication/append passes (662 ms / 2609 ms, barrier 0 ms). The deliberately
expired branch keeps the existing 15-second budget, releases 16 seconds after
fixed publication starts, and reproduces
`case_longitudinal_append/deadline_exceeded/unclassified/completed` while
preserving the winner and four private authority leaves (session `18950`,
exit 0). The intentional wait is not performance evidence. This isolates a
secondary conflict mechanism, not the original vector's performance cause.
Receipt: `/private/tmp/analytix-funds-current-native-20260930-append-isolation.json`.

The append count regression failed before wiring its candidate: full validator
calls `4` and inside-exact calls `0`, versus required `2/2`; the ordinary
fallback passed. The narrow production candidate now uses the existing
principal/risk/workspace/case-binding validator only for append's two checks
inside its owned `UseExact` callback. Its two outer full validators, complete
DSV2 lease pre/post checks, all store integrity/continuity checks, operation
error handling and terminal budgets remain intact. Other case-entity methods
still use the full validator four times. Runtime assembly requires the extra
validator and disables this additive capability if it is absent.

Full focused packages passed after wiring (`caseentity` 2.080 s and
`turnsecurity` 2.045 s, session `35925`, exit 0). New regressions cover exact
callback placement, ordinary fallback/non-append behavior, outer/inner failures,
dataset revocation caught by the capability postcheck, cancellation and failed
canonical writes. These fixture counts are not native CAS duration measurements.
Logs: `/private/tmp/analytix-funds-current-native-20260930-append-red.log` and
`/private/tmp/analytix-funds-current-native-20260930-append-green.log`.

Clean base `8d538b3` plus an explicit eleven-file overlay compiled in session
`32352` (exit 0). Separate binary `runtimeapp-native-append.test` is 68,085,362
bytes, SHA-256
`b034062ed973c38a3f9cd029cadc13bec0680b00dccb0d3f11de4b8b61f5df34`;
the prior retained binary is unchanged. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-append-compile.json`.
The compiled production composition, source-unavailable natural/expired chain,
existing async candidate/fixed/longitudinal fault/restart/ordinary-continuation
regressions, semantic parser and finite failure-code tests all passed in session
`85981` (exit 0). The new natural fixture measured fixed publication 671 ms and
append 1632 ms, with zero barrier delay and zero Provider calls. Neither this
fixture comparison nor the count reduction proves native performance or the
original timeout's cause. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-append-runtime-focused.json`.

A later harness-only diagnostic adds fixed-publication/append timestamps and
at most three append goroutine samples at 1/3/8 seconds. It projects only state
and function names, retains no raw stack/arguments/IDs/paths/bodies in logs, and
joins the sampling goroutine during fixture cleanup. It preserves the async
guard, actual native Owner close and all budgets. This diagnostic is not in the
`b034062...` binary. A twelve-file clean-base overlay subsequently compiled in
session `17152` (exit 0) as `runtimeapp-native-append-diagnostic.test`, 68,103,906
bytes, SHA-256
`91d3626daf056a76e0834326a9af0dc98e56670d8b9e5c707f8e3ddc60a5ad6e`.
The two prior binaries are unchanged, secure runtime-profile ancestors are
verified, and compile-time cache availability is 15,499,907,072 bytes. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-append-diagnostic-compile.json`.
The diagnostic projection (including the append input reader), semantic parser
and finite error-code tests passed against this binary in session `93306`
(exit 0). The full native vector in session `74622` verified all twelve source
hashes and the binary hash before starting; it ended naturally at the unchanged
20-minute test limit (exit 2). No native
component was rebuilt, and the source `8d538b3` sealed app's Go runtime is still
not executed. The candidate remains uncommitted and overall partial.
Notion mirror has not been updated at this in-progress checkpoint.

The two-thread numeric/selected-row/binding/provenance assertions, hydration,
and reopen recovery (two admitted facts, zero held) passed again. The actual
native Owner close also completed. Unlike the preceding candidate, the
source-unavailable async terminal guard passed: three complete operations,
zero failures, with completion and failure-record codes both `none`.
Fixed publication start to append entry measured 3870 ms; append entry to
async-finished observation measured 10168 ms. These are fixture phase times,
including diagnostic overhead; the latter is not an isolated CAS duration.
The bounded 1/3/8-second scheduled samples found full validator material reads,
fresh-head observation writes and full validator observation reads within
SecurePrivateCAS inventory/shard validation. Actual sample offset and overhead
were not separately measured.

The subsequent public thread GET in `b1WaitTerminal` did not return before the
overall timeout. Its sampled timeout stack follows retained-fact verification,
`WithRecoveredFactFinalWitness`, historical DSV2 selection, shared-head
confirmation and an observation CAS read with inventory scanning. This proves
the wait location, not one scan's duration or a unique root cause. Post-close
public SourceUnavailable/numeric-negative assertions, zero additional Provider
dispatch, unchanged whole preparation-tree digest and fresh-process recovery
were not reached. The panic also bypassed deferred all-received body-size
logging; those aggregate sizes are unavailable for this exact run and must not
be copied from a prior candidate. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-append-diagnostic-vector.json`.

## 2026-10-01 historical-read challenge candidate

The next source-only regression uses the real EvidenceAuthority implementation
with controlled witness, persistence and material fixtures. Two nested
historical snapshot reads initially performed six observation puts, readbacks
and projections, six fresh witness observations and 152 material reads. The
count regression failed before the candidate while the unsupported-coordinator
fallback passed (session `10269`, exit 1). These are call counts, not actual
SecurePrivateCAS inventory timings.

The candidate in `historical_fact_v2.go` now obtains its initial current head
through the existing callback-scoped `WithFreshHeadChallenge`, validates that
head, and binds both pre/post challenges to it. The historical selection still
names its original witnessed head. Current signed index/bundle ancestry and
original selected snapshot material resolution, post-callback equality, current
permission/epoch checks, primary-CAS admission and the two recovered-final
verification gates remain intact. A coordinator without the scoped capability
still performs three full observations. No authority/CAS scanner or terminal
budget was changed.

The regression now requires two full observation puts/readbacks/projections,
six fresh witness observations and the same 152 material reads for the two
selections. It covers pre/post challenge failures, historical material and
current index drift, cancellation and callback errors. A real Authority signed
and witnessed successor with intentionally absent child material also causes
the old candidate to fail with `ErrCurrentChanged`; this is shared-head drift,
not a positive valid-source advancement test. An initial test incorrectly
expected the memory-only historical reader to resolve an unrelated current
snapshot's raw manifest material; that expectation failed and was corrected to
current signed-index drift. It does not replace the separate real CAS whole
inventory integrity gate. Full focused packages passed in session `5100`
(exit 0): datasetsnapshot 2.846 s, evidenceauthority 1.297 s,
evidenceregistry 0.994 s and thread 2.570 s. Logs:
`/private/tmp/analytix-funds-current-native-20260930-historical-red.log`,
`/private/tmp/analytix-funds-current-native-20260930-historical-green.log`
(initial expectation failure), and
`/private/tmp/analytix-funds-current-native-20260930-historical-green-2.log`.

The native harness now emits only each received body's byte count and
completeness, then its normalized messages/tools byte count, immediately;
aggregate receipt evidence will no longer depend solely on deferred logging
after a possible test panic. It also timestamps the post-Owner-close public
hydration step. No body, raw stack, identity or path is logged by these additions.
A fifteen-file explicit overlay compiled successfully in session `88454`
(exit 0) into `runtimeapp-native-historical.test`, 68,104,370 bytes,
SHA-256 `fa16a86d21b16e6986b5cdb275e6643c1cd7ede261cdbeab2b71e00ca45a0259`.
The previous diagnostic binary is unchanged. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-historical-compile.json`.
The compiled production composition, natural/expired SourceUnavailable append,
semantic parser, finite failure codes and stack projection tests passed in
session `84440` (exit 0). The natural fixed/append times were 691/1688 ms with
zero Provider calls; deliberate expiry reproduced the expected secondary
deadline/conflict, not a product acceptance pass. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-historical-runtime-focused.json`.
After a natural idle window, session `83262` ran the full native vector and
ended naturally at the unchanged 20-minute limit (exit 2). Native four remain
frozen. In this exact run, the previously blocked post-Owner-close public
hydration returned in 22166 ms. The numeric/row/provenance checks, reopen,
actual Owner close, SourceUnavailable/numeric-negative assertions, zero extra
Provider dispatch and unchanged whole preparation-tree digest all passed.
The strict async guard reported three complete operations with zero failures;
fixed-to-append/append-to-finished measured 3388/8977 ms, with both error codes
`none`. This demonstrates that public seam for this candidate, not a general
CAS/native performance guarantee.

All four received bodies were complete. Immediate logs preserve body sizes
`[21902,26084,20239,24421]` (total 92646 bytes), and normalized messages/tools
sizes `[21819,26001,20156,24338]` (total 92314 bytes) even though the deferred
aggregate log was bypassed by the panic. The final wait was in
`b1AssertFreshProcessRecovery -> CombinedOutput -> Wait4`; child output had not
returned. Fresh-process recovery is therefore unverified, and the entire test
is partial. A subsequent read-only process snapshot found no remaining
historical-candidate test process or descendants; no process was killed here.
Receipt: `/private/tmp/analytix-funds-current-native-20260930-historical-vector.json`.

The recovery helper also has a separate harness mismatch: it expects
`1 + len(AdditionalFinals)` fact candidates. The vector passes only A's final
although its fixture contains two distinct-thread facts. The same-thread B1
source-update call supplies its second final, so that path expects two. The
new mismatch was found by source review after the timeout; this run's missing
child output does not prove the mismatch caused its current wait. The next
work isolates that helper contract and records bounded safe child phases
before considering further native execution. No time budget is extended.


## 2026-10-01 fresh-process contract candidate

Independent source review confirmed a harness input defect, distinct from the
unattributed subprocess wait. The private recovery input now carries the full
explicit final set with each ThreadID, TurnID, original digest and expected
history state. Vector A/B are two current finals in separate threads. B1 source
update is the same thread's retained A1 and current A2. The child requires exact
candidate/admitted equality to the complete set and zero held; duplicate,
incomplete or unknown-history expectations are rejected. Each final is checked
against its own thread GET, original digest/history and its own protected local
accepted-slot display. Only same-thread GET responses are shared.

A redundant test-only Host materialization validation was removed. It did not
open a native Owner. The mandatory production startup Host validation and actual
native Owner package/signature admission remain; positive native/semantic/final
assertions are unchanged. This is not a measured startup speed improvement.

The subprocess now streams only fixed-whitelist phase names and bounded ordinal/
candidate/admitted/held counts through a schema-checked writer into a private
0600 journal. Input lines are bounded to 256 bytes, valid records to 64, and the
closed record schema keeps output below 16 KiB. Unknown/malformed/oversized
output, raw stdout and all stderr/panic stacks are discarded. Every accepted
record is written and fsynced immediately. A panic leaves the last durably
observed phase, which cannot prove the exact child state at that instant.
Normal test cleanup removes the journal and private input; the safe parent
phase log is the retained run artifact. No private input, source body, identity,
credential or process capability is copied into that log.

A separate non-native regression checks both same-thread and two-thread routing,
all expected finals, exact admission count/zero-held enforcement, duplicate and
incomplete inputs, read/check failures, fragmented diagnostic writes, unsafe/
oversized output rejection, immediate private persistence and record/byte bounds.
Compilation and execution are pending at this checkpoint. Child 11-minute test,
parent 12-minute context, overall 20-minute test, public GET 45-second timeout
and SourceUnavailable 15-second completion guard are unchanged. Native four
remain frozen; no cold native build, packaged-runtime execution or live Provider
was introduced. Overall remains partial and uncommitted; Notion mirror pending.


The first seventeen-file overlay compiled (session `21708`, exit 0) into
`runtimeapp-native-recovery.test`, SHA-256
`080dbcefea77f6778423470d9f2003b7b20c093fe231ae927e97496f841f2e20`.
An independent read-only review then identified two ignored file-read errors
in the new journal regression; those reads now require success and the final
file must equal exactly 64 canonical records. The first binary is preserved;
its receipt is
`/private/tmp/analytix-funds-current-native-20260930-recovery-initial-compile.json`.
The corrected candidate compilation is pending, and no native run has used
that first binary. Protected four remain unchanged.

Notion tools are exposed, but the mandatory `get_tool_access` discovery tool is
not exposed in the current tool inventory. Content-search access therefore
could not be established; no Notion page was read or written. The bounded
mirror remains pending and does not block repository work.


The corrected seventeen-file candidate compiled in session `84664` (exit 0)
into `runtimeapp-native-recovery-verified.test`, 68141154 bytes,
SHA-256 `8c7a3b29c330b913bc2bf93c2fea48511715506b6268b8d36e2df36fb5ffd096`. Prior binaries are unchanged. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-recovery-compile.json`.
The three recovery routing/admission/private-journal regressions passed in
session `52156` (exit 0), with journal persistence 0.21 s. This is non-native
contract evidence, not new-process runtime recovery. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-recovery-focused.json`.
The first launch in session `31240` failed immediately (exit 1, 0.03 s)
because the command used the module root rather than the owning runtimeapp
directory for the relative fixture path. No native acceptance was reached.
Receipt: `/private/tmp/analytix-funds-current-native-20260930-recovery-vector-startup-failure.json`.
The corrected launch is session `23298`, with a separately verified fixture
location and log. Source/binary are unchanged; final result pending.


Session `23298` ended naturally at the unchanged 20-minute timeout (exit 2).
The current A/B exact numeric, seven-row, seven-binding, reference/provenance
and initial public hydration assertions passed. Four complete received bodies
were `[21902,26084,20239,24421]` (92646 bytes total); normalized messages/tools
were `[21819,26001,20156,24338]` (92314 total). The strict async guard reported
two completed operations and zero failures. Reopen observed two candidates,
two admitted and zero held, but its new production assembly did not finish:
the final wait was `driver.reopen -> normal Owner -> InspectPackageV2 ->
runBoundedCodesignV2 -> Wait4`. This is a wait location, not a measured duration
for all signature checks or a confirmed storage root cause. Read-only snapshots
also observed owned codesign children in `U` I/O wait during this run.

Actual Owner-close/SourceUnavailable, zero-extra-Provider/unchanged-preparation
negative checks and the corrected fresh-process helper were not reached in
this exact run. No child phase journal was created because the helper was not
called. The earlier historical binary's passed negative seam cannot be copied
into this candidate's receipt. A post-run owned-process snapshot found no test
process or tracked codesign child; no process was killed. Log SHA-256:
`08e9eb086fad702d918f77f26b5cb0e6dd8624a49753431662a20d9b7fe8c75f`. Receipt:
`/private/tmp/analytix-funds-current-native-20260930-recovery-vector-2.json`.

A focused read-only review found no existing trusted package-inspection result
on Owner that could safely replace the initial exact-source/seal test assertion.
Both inspections remain. No raw unverified authority JSON, cross-startup
signature cache, skipped Owner gate, or extended budget was used. Repeating the
same vector without a changed candidate or cache/verification seam is deferred.
Source-update/missing/corrupt/fresh-process checks remain unverified for this
seventeen-file candidate; overall partial and uncommitted. Notion pending.
