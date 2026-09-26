# PR28 post-c8 implementation and candidate acceptance

Status: Operational completion record. Independent D01–D08 work delivered;
installed live-Provider admission remains BLOCKED_EXTERNAL (X01).
Scope: user-authorized D01–D08, canonical repository and original branch.

Current product SOURCE: `74e4316eabff41ad72878001b5be3b89db020a1d`,
tree `7f0c6b5a47a9349916b90d8b1adbf84216783c3a`. The original branch continues.
The clean-checkout DMG/ZIP and exact installed copy are available. Final 120-turn
and post-Main 121st-turn acceptance, both in-place upgrades and bounded cleanup
passed. Superseded packages
and failed intermediate attempts below remain historical evidence; they do not
replace acceptance of this SOURCE. e15 remains unchanged historical evidence.

## Starting identity

- Branch: `codex/workbench-product-delivery-20260914`.
- START/remote PR HEAD: `6a56571c1da097f748b79e2d52b2ad6ae567356d`;
  previous product SOURCE: `c8cec28cd4de61ae5c8fa23fa9d09d115fd58bc8`.
- Direct remote main: `ce96cf12581acfa0e19fae7c6aa9c709371012c8`;
  local main: `60839b721b273119ab30158c65109dde4db52441` (five behind).
- Preserve the user's modified runbook and two untracked QA drafts.
- Preserve [e15](pr28-e15-credential-authority-2026-09-26.md) and the
  [c8 source evidence](pr28-e15-performance-followup-2026-09-26.md) as history.
- Fresh initial CI: CodeQL success; Development run `36237795723` still
  running with job `108392920220` failed. Its root tests report
  `host_response_event_failed` at the response-stage persistence seam.

## Execution ledger

| ID | Required outcome / owner | Status | Fresh evidence |
| --- | --- | --- | --- |
| D01 | Git/CI/PR identity and current summary | DONE_IMPLEMENTED | SOURCE/main and 57 checks refreshed; this focused documentation successor carries the current PR/ledger summary, with historical checkpoints preserved |
| D02 | Protocol/budget/HTTPS lifecycle regressions | DONE_IMPLEMENTED | c8 owners retained (DONE_EXISTING); added trusted HTTPS/header rotation and budget regressions; final race/production checks exit 0 |
| D03 | Publication, component DOM and composited observation | DONE_IMPLEMENTED | Actual commit/SSE/Main/MessageBubble DOM linked; installed T01–T08 results below |
| D04 | Honest cache shape, wire input, usage and optional identity | DONE_IMPLEMENTED | Final-wire HMAC segments, not_checked/estimator semantics, currency/coverage consumers and consent tests |
| D05 | Public Messages baseline and extension fallback | DONE_IMPLEMENTED | Core Registry→HTTP/SSE and installed Settings path; unknown beta extensions CLOSED_WITH_SUPPORTED_FALLBACK; live X01 |
| D06 | Early requirements, compaction/recovery and effects | DONE_IMPLEMENTED | Scoped V4 sources, actual reader/two compactions/correction/write/restart; same Go subtree; final installed successor recovered the original thread with no repeated effects |
| D07 | Full-path baseline/after and measured hot paths | DONE_IMPLEMENTED | 25 controlled final observations; 17 matching successful baseline samples; full stage table and scoped profiles |
| D08 | Frozen source, new installed candidate and gates | BLOCKED_EXTERNAL | Independent delivery complete: exact DMG/ZIP install/signature/legal, short scenarios, 120+post-Main121, both upgrades and cleanup pass. Only installed live-Provider admission remains X01 |

Allowed dispositions: DONE_IMPLEMENTED, DONE_EXISTING,
CLOSED_WITH_SUPPORTED_FALLBACK, BLOCKED_EXTERNAL, FAILED, UNFINISHED.
Each final evidence entry must identify command, exit code, candidate and scope.

## External dependencies and claims

X01: a packaged real-Provider QA handle must be independently authorized and
available. The development authority is not copied. Perform one bounded
inventory before the corresponding installed check. Synthetic tests are not
live usage or KV-cache observations.

X02: SOURCE `74e4316ea` now has **57/57 successful checks**. Development CI
[36267289661](https://github.com/Eysn0130/analytix/actions/runs/36267289661)
completed successfully; CodeQL succeeded. The final documentation successor is
reported separately. A queue or failed job is not a pass. No force push,
direct-main write or weakened gate is allowed.

SourceReady=true for product SOURCE `74e4316ea`; PrivateCandidateReady=false;
MergeReady=false; PublicMacReleaseReady=false. X01 is not cleared by source
checks or the completed synthetic installed journey.

The same SOURCE's native Windows job
[108474348168](https://github.com/Eysn0130/analytix/actions/runs/36267289661/job/108474348168)
passed both DPAPI/ACL/replacement/Registry restart and Windows source-profile /
shared-authority launcher wiring steps. Windows installed product and distinct
wrong-user login remain unverified. No arbitrary same-UID isolation or OS trace
of zero Keychain calls is claimed. CodeQL success is its changed-code check scope,
not a proof of zero vulnerabilities across the repository.

## Final SOURCE: code and verification map

All paths below are repository relative. Go `internal/` paths are under
`packages/runtime-go/`. The named regressions are part of existing package/CI
entrypoints. No parallel product branch, new credential authority or trace service
was introduced.

| Work | Production owner and consumer | Commits / regression evidence |
| --- | --- | --- |
| D02/D05 | `internal/adapters/outbound/provider/{anthropic,compat,schema,stream,client}`; `internal/runtimeapp/provider_registry_*`; Provider Settings | `892c43612`, `755c2bec7`, `5969d66ae`; six effort Registry→wire cases, trusted HTTPS reuse/rotation, immutable saved protocol identity |
| D03 | `internal/app/turn/publication_timing.go`, server publication/turn owners; `src/main/services/thread-trace-service.ts`; `message-timeline-bubbles.tsx`, `thread-performance-trace.ts` | `892c43612`, `2c7dd911a`, `baae102ed`; actual accepted commit, owned Core generation, closed public schemas, actual component layout effect |
| D04 | `internal/domain/cachetelemetry/input_segments.go`, client cache/identity, app cachetelemetry/usage; shared public contracts and usage hooks | `892c43612`, `baae102ed`; missing versus zero, latest cumulative usage, all attempts, currency and consent/domain vectors |
| D06 | `internal/app/turn/{task_continuation,continuation_sources}.go`, model source reconstruction, tool catalog and `internal/server/tools_execution.go` | `892c43612`, `5e505d4fa`, `b7c128300`; versioned scoped original sources, real reader, strict recovered arrays, no Goal dependency |
| D07 | `internal/app/loop/provider_stream.go`; managed navigation/index refresh; existing main/side status owners | `892c43612`, `e7e250812`, `6a5e2f088`; incremental reasoning accumulation, acknowledged Send release, fixed trusted stage labels |
| Delivery repairs | `src/main/runtime-sse-ipc.ts`; main/side client-message ID owners; existing generated-artifact fixture | `6d29c8634`, `74e4316ea`; coalesced replay ends at verified terminal, no insecure RNG fallback, alphabetic synthetic workspace |

Final frozen local checks (each command sourced `scripts/use-analytix-cache.sh`
in its own shell; desktop commands used `NODE_DISABLE_COMPILE_CACHE=1`):

| Command / exact scope | Result / exit |
| --- | --- |
| `npx vitest run src/renderer/src/store/chat-store-side-actions.test.ts src/renderer/src/store/chat-store-thread-actions.test.ts src/renderer/src/store/chat-store.test.ts src/renderer/src/components/Workbench.canvas-send.test.tsx src/renderer/src/components/chat/message-timeline-terminal-trace.test.tsx` | 159/159, 5.44 s, **0** |
| `npm run typecheck` | **0** |
| `go test -race ./internal/adapters/outbound/provider/client -count=1` | 2.065 s, **0**, SOURCE 74e |
| `go test -tags analytix_prod ./internal/runtimeapp -run 'Test(RuntimeOwnedResourceClosesAfterInnerDrainAndRetriesSafely\|NewRuntimeServerHandlerAssemblesRunnableHandler\|RuntimeAppRequiresTokenUnlessInsecureIsExplicit)$' -count=1` | 3 selected tests, 22.664 s, **0**, SOURCE 74e |
| `go test ./internal/app/loop -run '^$' -bench '^BenchmarkProviderStreamChunking$/^reasoning$/^1024$/^[13]$' -benchtime=2s -count=1 -benchmem` with CPU/alloc/block profiles | 2 real benchmarks, package 7.734 s, **0**, SOURCE 74e |
| `npm run dist:mac:arm64:core` in clean candidate checkout, existing reviewed Computer Use package root | **0**, SOURCE 74e |
| Read-only DMG mount, `ditto` installation, strict deep `codesign` verification, existing artifact legal audit | each **0**, installed 74e |

`source-verification.json` and protected command logs bind these results. The
Go subtree `53df1b7728281ef62e83cec360b51537675b9a82` is byte-identical to 6d;
the final desktop successor only replaces insecure ID fallbacks. Earlier full
package, root semantic and native protocol commands are recorded chronologically
below, with their actual SOURCE and exits; they are not relabelled as fresh 74e
executions. Current 74e CI separately runs the full default/production packages
and regression shards.

### Exact delivered artifact identity

- Clean detached build checkout:
  `/Users/sun/.codex/worktrees/post-c8-candidate/analytix`.
- Snapshot digest:
  `d4f56a03a539cac4337da85b58267892ce54198d6019f9ce4f0dcc9cd582cca8`.
- Electron 41.10.3 / electron-builder 26.15.3; normal `core` build,
  `isolated-local-v1`, ad-hoc signed, not notarized. The reviewed existing native
  package provenance and seven hashes passed the normal build owner; no arbitrary
  replacement native binary was introduced.
- Immutable task copies in
  `/Volumes/AnalytixCache/development-v3/evidence/pr28-post-c8/candidate-74e4316ea/`:

| File | Bytes | SHA-256 |
| --- | ---: | --- |
| `analytix-core-1.0.6-mac-arm64.dmg` | 321,610,603 | `edd32919b070b8314b65de1e2b5eed079096261f9c9b22ddfe31b7cd98a87cf8` |
| `analytix-core-1.0.6-mac-arm64.zip` | 331,427,709 | `687450b61cb642309a705c5cf6ed4015b809f3223b5384b89ce88efc97707618` |

- Actual installed app:
  `/Volumes/AnalytixCache/development-v3/tmp/analytix-post-c8-installed-74e4316ea/Applications/analytix.app`.
  It was copied from a read-only mount of that DMG. Exact installed legal audit
  passed for **1,172 dependency instances**, zero mandatory admission blockers.
- Authority remains **`development_clean_non_publishable`**;
  `publishable=false`, `releaseEligible=false`, no publication receipt. Legal
  admission and ad-hoc signature validation do not grant public release.
- `74e4316ea-installed-artifact.json`, `74e4316ea-installed-bundle-legal.json`
  and `candidate-74e4316ea/artifact-copies.json` retain build/source/install/copy
  evidence. The final installed acceptance outcome is separate from those build
  receipts. Historical e15 and intermediate candidates keep their original names
  and hashes.

## Final installed short journey and timing

Evidence owner: `final-74e4316ea/` under the private task evidence directory.
Fresh profile: `analytix-post-c8-after-ihpxu5`; exact app installed from the final
DMG, with its packaged Core. Only loopback synthetic credentials were entered.

- Visible Save, reopen Settings with blank password, keep existing value, explicit
  replacement, and subsequent actual requests passed. The saved protocol remains
  immutable. The side conversation also produced one response for one request.
- Go restart `46787→47537` preserved the original conversation. Main
  `46340→47780` continued with both Renderer trace flags absent. Public Messages
  was selected through normal Settings; Main `47780→48319` retained that native
  Provider and continued. Final history Main `48319→50366` continued the same
  `thr_durable_23` without re-entry.
- Native wire observations: `/messages`, `x-api-key`, thinking `enabled`, cap
  4096, no beta header. The pre-restart sample used `low`; the resumed sample
  used the current UI default `max`. They prove protocol/persistence, not an
  effort-controlled performance comparison. Generic benchmark wire consistently
  omitted effort/thinking/cap in both before and after; model was
  `deepseek-v4-flash`. Model labels alone are not wire evidence.
- Two startup observer failures involved a stale closed page / a page not yet
  created; retries were read-only and no Send was replayed. Cold window startup
  waits are recorded separately from send-to-answer measurements.

| Scene | Installed observation |
| --- | --- |
| T01 | Text/reasoning, ASCII/CJK, 1/64/1024 chunks, short/64 KiB; actual public batch→real DOM→screenshot |
| T02 | Verified `accepted_final_batch`, `source_unavailable/case_terminal_source_unavailable`; physical requests 38→38. This is a protected fail-closed result, not positive Funds facts |
| T03 | Two bounded truncated attempts; private sentinel absent from public events/DOM; `turn_recovery_boundary`, then unique same-thread follow-up passed |
| T04 | Actual Provider heartbeat, visible Stop, socket close, Main HTTP 200 with `aborted`; unique follow-up passed. UI stopped its stream before terminal SSE; durable readback is the evidence |
| T05 | Original A→native B→A; visible prior answers, physical requests 38→38 |
| T06 | Native minimize during delayed response, completion, Raise and visible answer. DOM visibility stayed `visible`; no OS minimized flag or physical scanout timing claimed |
| T07 | Historical replay ACKs `[true,true,true,false]`, no error, no new Provider request; final false is intentional duplicate ACK |
| T08 | Two trace-off answers completed without Core/DOM diagnostic linkage; trace-on cases link. Missing off diagnostics are N/A, not zero |

PDF fixture SHA-256:
`aa7861fe40b25509b40a4eb039138a33fc3a3b8fb629cfe16ab314e9f9e2ab52`.
The actual viewer passed pages 1/2, zoom 115%→125%, second-page search 1/1,
close/reopen and settled page-2 readback. Source bytes were unchanged. Captures
show the read-only viewer; an immediate page-control read before the scroll
settled was retained and then verified as 2, not counted as a product failure.

### Semantic coverage and effect boundaries

| Vector | Actual consumer / assertion |
| --- | --- |
| S01/S03/S06 | `TestPostC8ContinuationSourcesDriveRealToolEffectsAfterTwoCompactionsAndRestart` reads real outgoing continuation references and actual `read_task_history` results before issuing a write. Initial A-only/B-forbidden, seven inert user messages, then C correction; no Goal; A/B untouched and C correct |
| S02 | Two real automatic soft-limit crossings, Core shutdown/reopen, canonical public history, a second restart; installed 6d repeats the actual source-read/write journey, with the identical 74e Go subtree |
| S04 | `TestRestartOpenNotRequiredSideEffectIntentIsOutcomeUnknownAndCannotResend` and `TestRestartPreservesKnownDurableSideEffectIntentOutcome` retain unknown, known success and known failure; closed effects cannot be resent. Installed completed C write retains its mtime across Main recovery |
| S05 | `TestContinuationSourceReadRejectsChangedPrincipalCaseAndThread`, scoped reconstruction tests and `TestWorkspaceReadUsesCurrentCoreAuthorityWithoutHistory` reject stale scope/revocation and reread changed file content |
| Related tool ordering | Existing `TestRunToolStepFlushesReadOnlyBeforeSerialWrite`, mixed-effect partition ordering, cancelled batch settlement, and `TestExecuteBatchKeepsResultOrderWhileRunningInParallel` (delayed first member) cover the existing scheduler; no new scheduler was needed |

The eleven-seed installed semantic fixture records four observed snapshot
identities, two source reads and one visible approved C.txt write. It is an
input-sensitive protocol/effect fixture, not a model that blindly returns OK.
The fixtures prove the specified Host consumers and local effects, not real model
reasoning quality or remote exactly-once execution.

The exact 74e installed successor then opened the retained 6d semantic profile
in place (Main PID 91292). Before Send, the normal Main HTTP reader returned 200
for `thr_durable_3`, no Goal and canonical public arrays. The original UI thread
showed the completed C correction and accepted one new `QA_SOAK 10201`, producing
one physical request and a completed terminal/DOM result in 17,055.839 ms.
A/B contents and mtimes, and C's content and mtime `1790451346562.3818`, exactly
matched the pre-upgrade receipt. The successor fixture observed zero history
reads and zero writes: completed effects were not replayed. This is recovery
evidence; the new short continuation is not substituted for the earlier two
source reads and actual write. `semantic-upgrade-result.json` and the linked
Core/Main/DOM stage table retain the observations. The owned driver stopped
with exit 0.

### Final continuous installed journey

The frozen 74e DMG-installed app completed **120/120** consecutive visible
synthetic turns in `thr_durable_28`. The receipt verifies 120 unique turn IDs,
strictly increasing terminal sequence numbers, 120 completed durable turns and
exactly one physical loopback request for each input number 1–120.

Main `50366→79334` then reopened that same profile and thread. The 180-second
page observer expired before any Send; the same Main subsequently created its
window. No process restart or business request was retried. The existing window
read back all 120 turns and completed a single `QA_SOAK 121` follow-up without
credential re-entry. Final readback contains 121 completed turns, the settled UI
shows 121 turns, and inputs 1–121 each have exactly one physical Provider request.
This slow cold startup on the external-cache QA installation is retained as a
separate observation, not hidden inside response timing or declared fixed.

`final-soak-progress.json` carries both readbacks and the integrity assertions;
milestone captures include 1/30/60/90/120 and the post-Main continuation.
The final offline stage table contains 159 observations, 155 with Core and actual
DOM linkage. The four intentional exceptions are two trace-off observations,
the rejected truncated draft (Main/DOM recovery notice, no successful candidate
commit), and cancellation (durable aborted readback after its UI stream closed).
These are not missing successful-publication timings represented as zero.

### Original c8 profile upgrade and final cleanup

The original `6a56571c1` baseline profile was retained until final acceptance,
then opened in place by the DMG-installed 74e app (Main PID 3616). No profile,
credential or master key was copied. Main returned 200 for `thr_durable_25`;
the UI displayed its prior 9001 answer before a single new `QA_SOAK 10901`.
The new response arrived in 13,251.427 ms with exactly one physical request,
and durable completed turns increased from five to six. All six pre/post hashes
matched: Registry, encrypted credentials, master key, authority marker, final
answer signing authority and user-data settings. Existing valid file authority
was reused without migration or credential re-entry.

The baseline's synthetic key predates the later driver's initial/replacement
labels, so the wire observer reports `other-synthetic`. This proves a nonempty
authorization header and unchanged persisted authority, not an invented exact
wire-key comparison. Real Provider acceptance remains X01.

The first upgrade timing analysis exited 1 because the retained c8 profile
contained old diagnostic records without turn IDs. The corrected offline reader
obtains the new Main epoch from the new turn's Core receipt and excludes 136
pre-upgrade timing rows; current records still require full opaque identities.
Re-analysis exits 0 and links Core/Main/actual DOM for the new turn. No business
request or product validation was replayed or weakened.

Both upgrade drivers stopped with exit 0. After the complete bounded journey,
the cleanup script verified stopped owners, exact task-owned roots and no active
process references, then removed 15 disposable synthetic profile/fixture roots
(10 cohorts and five abandoned startup fixtures), exit 0. This includes their
synthetic Registry/Secret Store data. It preserved the installed apps, immutable
installers, e15, safe receipts and the reproduction driver/inputs. Persistent
development authority and user data were outside the deletion manifest.

`cleanup-result.json` records 15/15 absent afterward. Offline re-analysis after
deletion succeeds from retained numeric/opaque inputs: 159 fresh observations
(155 Core+DOM), one semantic upgrade and one c8 upgrade (both Core+DOM).
`upgrade-result.json`, `upgrade-{before,after}-integrity.json`, and
`semantic-upgrade-result.json` retain the exact assertions.

### Documentation successor and remaining action

The final focused successor changes only this QA record and the handover index.
It does not change product SOURCE 74e, its tree, dependencies or package inputs.
`electron-builder.config.cjs` excludes `**/docs/**` and does not select these
files in extraResources; no document-only rebuild or repeated 120-turn run is
needed. Its actual HEAD/CI is reported separately through PR28 and the final
receipt; 74e's 57 successful checks are not relabelled as checks on that HEAD.
PR28 remains Draft/open, without merge or publication. The current summary is
mirrored to the original Construction Ledger; historical sections remain.

Next dependent action: configure one independently authorized QA Provider in
the exact candidate's normal Settings, within the existing allowed model,
endpoint and budget, then run the bounded real Save/use/Go/Main continuation.
Do not paste a key in chat, copy development credentials or weaken package
isolation. Real KV/cache/DSH comparisons also remain unobserved. Windows GUI,
distinct wrong-user login, OS-wide Keychain tracing and arbitrary same-UID
isolation are unverified scopes, not newly imposed performance gates.

### Full-path before/after (milliseconds)

Before is the clean packaged app at `6a56571c1`; after is the DMG-installed 74e
copy. Same matching synthetic inputs/protocol/requested model/wire effort/cap,
single Mac M4 Pro, external cache storage. No build/profiler ran during final
controlled observations. Profile-wide history and cache warmness differ, so these
are measured cohorts, not a causal speedup or cloud/DSH comparison.

| Sample | Before answer observed | After answer observed | After publication gap | Main verify | Store→actual DOM | After capture interval |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Short ASCII / 1 chunk, n=3 median | 11,675.383 | 6,568.692 | 669.105 on median sample | 2.188 | 3.0 | 6,573.686–6,706.816 |
| 64 KiB ASCII / 1,024 chunks, n=1 | 10,627.677 | 9,990.765 | 1,882.679 | 9.268 | 2.9 | 10,000.670–10,225.457 |
| 64 KiB CJK / 1,024 chunks, n=1 | 11,440.972 | 8,956.827 | 1,600.612 | 7.201 | 2.8 | 8,960.219–9,144.554 |
| Reasoning ASCII / 1,024 chunks, n=1 | Failed schema admission | 9,447.537 | 1,491.389 | 2.150 | 2.5 | 9,452.084–9,699.111 |
| Fourth same-thread 64 KiB response | 13,209.186 | 11,724.115 | 2,488.557 | 9.036 | 9.8 | 11,728.314–11,941.660 |
| Same history after Main restart | 12,708.051 | 13,676.211 | 2,464.662 | 3.155 | 7.3 | 13,695.093–13,918.035 |

Short ranges: before 9,572.443–13,913.635; after 6,281.365–6,792.602.
There are 25 controlled after samples and 17 matching successful before samples.
The known baseline CJK submission lock prevented two additional short cases;
they are not zero-latency results. Heavy cases have one sample, not a p95/SLA.
The Main-restart sample did not improve. No universal product speedup is claimed.

For the 64 KiB/1,024 ASCII example, Host stream entry→first raw text is
1,716.947 ms (includes supervised setup, not pure Provider inference), projection
→durable commit 817.135 ms and commit→SSE deliverable 1,269.532 ms. These identify
substantial Host/persistence delivery waits; Main/DOM are much smaller here.
Go publication gap joins only the same candidate and process generation. Each
process's internal durations use its own monotonic clock; a single observer
measures Send and screenshot bounds. No cross-process clock subtraction or
React/rAF-as-physical-pixels assumption is used.
The offline analyzer explicitly excludes the two trace-off samples from timing
joins and matches Main timing to the Core receipt's Main time origin. This fixes
an observer-only attribution error where a later trace-on replay of an old turn
could otherwise populate its earlier trace-off row. No product change or replay
was required. Only opaque references and fixed numeric terminal timing records
are retained for analysis after synthetic profile cleanup.

The final scoped reasoning benchmark is 230,119 ns/op ASCII and 329,826 ns/op
CJK, about 802,308 B/op and 84 allocations. Sampled allocation space is dominated
by result normalization (63.06%) and builder growth (35.78%); this remaining
sub-millisecond local work does not explain multi-second full-path latency.
Block profiling attributes delay to benchmark/channel orchestration, not a
measured product mutex bottleneck. Exact per-systemcall I/O counters were not
obtained: ordinary-user `fs_usage` exited 1 because root is required; no privilege
bypass was attempted. These profiles exclude sockets, audit persistence and UI;
they are not claimed as complete system call counts or a new merge gate.

## Historical implementation checkpoints

The working candidate adds explicit `deepseek-messages` Registry selection,
public thinking/effort semantics, strict role/tool pairing, complete system/tool
baseline rebuild, and no unverified beta fields. Private reasoning uses the
existing volatile replay authority; the public terminal validator remains intact.

The first production reasoning regression exposed an existing c8 mismatch:
`firstReasoningLatencyMs` was emitted but rejected by the closed event projector.
A numeric-only validator and rejection vectors fix that producer/consumer seam.

Core diagnostics now observe provider content/body/finish/return, last dependency,
candidate projection, accepted CAS commit, and SSE deliverability. Main and the
actual message component correlate safe turn/thread references; DOM commit and
next-frame observation remain distinct from a composited screenshot. The latter
and full installed stage table are still pending.

Cache work preserves old omitted fields and records `not_checked` plus the
`utf8_bytes_div4` estimator. Final-wire segment HMACs and a default-off authorized
stable Provider identity path are implemented. Physical-attempt cost estimates
retain currency and known/unknown coverage. Renderer fixed-rate conversion is
removed. These are Host estimates, not bills or real server KV observations.

Continuation work preserves chronological original-user sources in the existing
sealed continuation. Large sources have bounded references and a current-scope
`read_task_history` consumer. It does not promote user prose to execution
permission. A debug attempt to reuse automatic V4 for manual `/compact` failed
because V4 intentionally requires `auto=true`; that experiment was removed.
The production integration test now crosses real automatic context thresholds.

Debug evidence (intermediate trees, not final SourceReady):

- Go provider/client/schema/usage, cache/domain, loop/turn/model checkpoints
  passed. The aggregate invocation failed because an overly early duplicate-key
  check in the stream accumulator violated the existing raw-arguments contract;
  that experiment was removed. Host strict argument validation is retained.
- A Messages Registry→wire six-effort checkpoint passed, but the same aggregate
  command exited 1 on the original manual-compaction continuation fixture.
  The Messages fixture is being strengthened to assert actual public completion,
  observed-model classification and private-history non-disclosure.
- Desktop checkpoint: 4 files / 55 tests passed (exit 0) using threads; Vitest
  also warned about worker termination. A forks attempt ran 0 tests because
  its worker did not start; it is not acceptance.
- Baseline exact `6a56571c1` loop microbenchmark (three samples, 100 ms each):
  64 KiB / 1,024 ASCII reasoning chunks: median 2.848298 ms, range
  2.809011–2.869792 ms, about 36.5 MB allocated. Same text path: median
  0.433855 ms. This measures the local stream use case, excluding HTTP,
  audit/persistence, Main and Renderer. A builder change awaits final comparison.

No new source commit, final race result, installed candidate acceptance or
whole-product speed claim is established by these checkpoints.

### Further debug findings

- Fresh remote read: the starting HEAD's Development run `36237795723` is now
  completed/failed (Go package job and aggregate gate); CodeQL and the historical
  Windows credential lane passed. No result transfers to the working candidate.
- The strengthened Messages test initially looked for response model telemetry
  in the thread snapshot. That contract publishes diagnostics through usage SSE;
  the test now observes both actual snapshot completion and the matching turn's
  usage event. The private Messages tool/restart test passed (100.24 s) inside an
  aggregate command that failed the incorrect snapshot assertion; no aggregate
  pass is claimed.
- Automatic compaction's explicit count revealed only one compaction before the
  tenth seeded request. The fixture now supplies the following threshold-crossing
  request before asserting two compactions and restarting. Prior file inspection
  showed A/B untouched and C corrected, but its second restart timed out; this is
  intermediate evidence, not D06 acceptance.
- Scope review found that removing only `userHistory` could leave the same new
  snapshot's summary/older constraints available after workspace change. The
  new scoped snapshot is now rejected as a whole; legacy absent-field records
  retain their existing contract. Current-principal and case-boundary vectors
  were added to the existing source-reader tests.
- Desktop startup diagnosis: three worker-start failures ran no actual component
  tests. A standalone jsdom import took 117,682 ms; a macOS process sample showed
  synchronous Node compile-cache reads/opens. A per-command
  `NODE_DISABLE_COMPILE_CACHE=1` reduced the import to 447 ms and the real message
  component suite passed 4/4, exit 0 (1.75 s). This changes an optional tool cache,
  not product validation, assertion strength, or build-storage location.
- The exact starting baseline package reached DMG/ZIP creation and then its normal
  ZIP signature-preserving postprocessor. No shortcut package or signature bypass
  was used. The first baseline UI launch used a fresh isolated synthetic profile;
  Save was dispatched visibly but Core readiness was unavailable. It is not a
  Save pass. The cohort was stopped and retained while competing I/O settles.

## Frozen implementation and baseline checkpoint

- `892c43612`: explicit Messages baseline and fallback, wire/cache/cost diagnostics,
  scoped continuation plus real source-reader/file-effect regressions, Go/DOM
  publication timing and measured reasoning accumulation repair.
- `2c7dd911a`: the desktop originally retained Core stderr only as a private
  failure digest. The successor connects strictly bounded, owned-process timing
  lines to the existing Main trace writer. It validates PID/generation, opaque
  references, fixed numeric fields and HMACs, drops unrelated stderr, and does
  not admit Core trace names through renderer IPC.
- Desktop typecheck exits 0. Seven selected files on `892c43612` pass 182 tests.
  The receiver follow-up exposed two old exact-data assertions in the store test;
  these now require the actual added numeric clock fields. The six-file receiver,
  adapter, SSE, store and real-component cohort passes 234 tests, exit 0.
- The final Go tree `5ca3ff9ec83d08b8c3d890a309179aeda737ab08` is identical
  between these two commits. Its 20 test-bearing packages, client race and three
  production assembly tests pass. A fresh root integration/race invocation on
  `2c7dd911a` is running; the earlier complete three-test production invocation
  exited 0 in 461.040 s. Interrupted invocations are not aggregate passes.
- A screenshot file initially had mode 0644, which the cache preflight correctly
  rejected. That invocation was stopped and excluded; task-owned captures were
  corrected to 0600 and verification rerun after successful preflight.
- The `892c43612` package build was deliberately stopped when the missing desktop
  trace consumer was found. It is not a delivered artifact. The clean build
  checkout now uses `2c7dd911a` and the normal Core DMG/ZIP command.

### Exact starting baseline

The clean `6a56571c1` normal Core build completed (exit 0), with Electron 41.10.3.
The task-owned synthetic cohort uses that build app; it is a packaged-app baseline,
not a claim that this baseline copy was installed from its DMG.

- DMG SHA-256: `28365cb5028641e3b1580e69fcfc0c94fd51eeb55e79b2d32d914ff57a28f321`.
- ZIP SHA-256: `7d3f833eca0b56e9fd53dae600c18afe978a41ed6364d755c1bd80d82f92030a`.
- Baseline artifacts remain in the `post-c8-performance` managed worktree.
- Normal visible Save persisted the synthetic credential; after the interrupted
  first startup, visible Settings → Use Provider completed selection. This
  recovery is recorded rather than called a flawless initial Save journey.
- Successful short, 64 KiB, ASCII/CJK and 1/64/1024-block text observations are in
  `before-installed-samples.json`. Repeated identical short cases are distinct
  new threads. Four legal 64 KiB continuations build longer history; after a new
  Main process the same five answers load and the sixth answer completes.
- A reasoning-dominant 64 KiB/1024-block response reproduces
  `sse_event_rejected`; the thread-list status is idle but detail read returns
  502. It has no successful-response latency. No blind replay was attempted.
- Screenshots prove synthetic content was captured. The interval from capture
  invocation to completion is an observation bound, not physical display timing.

### Local stream benchmark

Same loop use case, 64 KiB, three 100 ms samples, no concurrent build for after:
ASCII reasoning/1024 blocks changes from median 2.825417 ms and 36,504,824 B/op
to 0.251558 ms and 802,654 B/op; CJK changes from 2.840370 ms to 0.332746 ms.
Allocations change from 1,097 to 85. The benchmark crosses budgeting, decoder,
normalization and final callback, but excludes sockets, durable audit, Main and
Renderer. These figures do not establish an overall speedup.

Private host evidence directory:
`/Volumes/AnalytixCache/development-v3/evidence/pr28-post-c8/`.
All retained profiles are isolated synthetic fixtures; real or development
Provider credentials were not copied or invoked.

### Exact-head CI repair before installed acceptance

- Frozen `2c7dd911a` root production/continuation/private-Messages tests completed
  successfully in 543.295 s, followed by client race (exit 0). Its package command
  also exited 0, but that artifact was superseded before installed acceptance.
- Current-head Development CI found two stale cost-label assertions, the native
  SSE conformance rejection, and an architecture inventory mismatch. These are
  actual failures, not an external-runner blocker. The cost consumers correctly
  say `Cost unavailable`; both component expectations now require that value
  while retaining the not-zero checks.
- The Go numeric reasoning-latency repair had not propagated to desktop's two
  independent content/closed-event validators. A new public-SSE regression first
  failed (exit 1); both validators now admit only a nonnegative safe integer and
  retain rejection of text, objects, negative/fractional/nonfinite values.
- The continuation timing callback wraps the same publicationauthority method
  that was previously passed as a method value. The closed AST call inventory now
  explicitly lists that existing server owner, preserving every prior entry.
- CodeQL reports 14 high hashing alerts. Inspection of the downloaded SARIF
  shows all reported paths start at the missing-key error's formatted Provider
  identity, not a credential value. The new continuation hash is an integrity
  digest, not a password verifier. Rather than change digest algorithms or
  suppress checks, the missing-key admission error now uses a fixed sentinel
  without interpolated configured identity; the regression requires exact text,
  `errors.Is`, and absence of identity and unrelated credential material. Final
  CodeQL disposition awaits the successor's actual result.

- Desktop follow-up exposed a second independent mismatch: the Go canonical
  terminal cache diagnostics had evolved while the TypeScript sealed-carrier
  schema still rejected the new fields. The shared schema now preserves the
  closed enums and canonical numeric fields, with the same attempt-cost
  cross-field consistency rules. Seven terminal/public-contract files pass
  376 tests; typecheck exits 0. The original native D-0242 process contract
  subsequently passes (115.84 s test time, exit 0).
- Remaining root CI failures identified an always-advertised history reader and
  pre-change pricing expectations. The reader is now offered only when the
  current scoped continuation needs out-of-line source access; the existing
  tool-count ceiling stays intact, with an explicit negative assertion. Pricing
  vectors require the exact configured CNY amount, no inferred USD exchange,
  and explicit zero Messages cache buckets for a fully known priced request.
  Missing buckets remain unknown in the dedicated protocol vectors.
- The root repair invocation exited 1 (353.375 s): automatic two-compaction
  recovery and real source-tool/file effects passed (170.76 s), ordinary catalog
  regression passed (44.27 s), and configured pricing passed (93.60 s), but two
  further old FX expectations in thread buckets/detail failed. The aggregate is
  not a pass. After correcting those exact currency consumers, the complete
  usage aggregation test passed (43.007 s, exit 0). Final client race passed
  (2.028 s) and the three production assembly/lifecycle tests passed (30.171 s);
  the sequential verification command exited 0. No product source changed
  during those final checks.

### Fixed diagnostic wording and final candidate selection

`baae102ed` CodeQL check `108422008292` still reports 14 high hashing alerts.
Downloaded Go SARIF analysis `1844532282` traces each reported source to the
constant `errors.New` at `provider_config.go:23`; removing Provider identity
alone did not close the check. The upstream
[SensitiveCall heuristic](https://github.com/github/codeql/blob/main/go/ql/lib/semmle/go/security/SensitiveActions.qll)
classifies calls with string arguments containing `apiKey` as possible password
lookups. This is consistent with the observed constant-error source, not evidence
that a credential value reaches the integrity digest. The fixed admission text
now says `credential is required`; error identity and rejection semantics remain
covered. No hash algorithm, CodeQL query, exclusion or gate is changed, and no
alert is dismissed. The successor's actual scan is still required.

This final wording change supersedes the `baae102ed` intermediate installer.
Any use of that installer is a development probe, not final D08 acceptance.

### Exact 8fd candidate and subsequent test-owner repair

- Normal `dist:mac:arm64:core` completed with exit 0 at SOURCE
  `8fd0a7e1a4964df045480d38e5932cc50ab0b4ad`. The DMG installation
  copy passed `codesign --verify --deep --strict`. DMG SHA-256:
  `b0afe64b4dccd4cce28791f79d76328d5edff47a5156fc71fbadc6cfcbe2c9b3`.
  Exact artifact legal audit and installed acceptance are still running.
- That SHA's CodeQL aggregate passed. Development run `36249194724`
  passed macOS process integration and Windows credential backend, but failed
  Application tests (1 failed / 7,401 passed / 22 skipped) and the production
  Go package lane. These are real failures, not an external queue excuse.
- The Go failure is an obsolete assertion of the identity-bearing missing-key
  message. It is updated to require the actual typed sentinel and exact fixed
  diagnostic while preserving all durable/SSE/recovery non-disclosure checks.
- The TypeScript compatibility oracle directly overwrote job JSON. Its wait
  route could observe the intermediate empty file as not-found. It now reuses
  the existing atomic file writer; a regression pauses the physical write and
  asserts the old record remains readable before the new record is published.
  The first test instrumentation failed because an ESM native export cannot
  be spied on; after explicit Vitest module wrapping, all 14 existing/new tests
  passed (6.13 s, exit 0). The Go recovery check is still running.
- These follow-up files are a Go `_test.go`, a TypeScript contract test and
  `delegation-test-support/job-manager.ts`. The latter is a compatibility
  oracle, outside the production package entry graph and desktop imports.
  Their repair does not change the installed product SOURCE or require
  another package. The subsequent HEAD's CI must still be reported separately.

The follow-up verification command completed with exit 0: task-job contracts
14/14 (6.13 s), `go test -tags analytix_prod ./internal/server -run
'^TestMissingProviderKeySourceIsClosedAcrossDurableSSERecoveryAndModel$'
-count=1` (26.947 s), and `npm run typecheck` (exit 0).
The first legal command used the narrower `app.asar` reader and exited 1
because supplemental licenses live outside ASAR under the bundle Resources.
It cannot admit the whole installed bundle. The corrected audit uses the exact
installed `.app` reader; its result is pending. No license gate is waived.

The corrected exact installed-bundle audit completed with exit 0: 1,172
package instances, 829 unique package/version pairs, zero admission blockers.
The package remains `development_clean_non_publishable`. ZIP SHA-256:
`9968705e5b4543f5409484e9ae1ace50359875952359e1632a4d761c04ef490c`.
Test/oracle repair commit `03afc3287aa7388caf64069b0e11ef4b5c34efe0`
is pushed. Its Development CI is run `36250474007` (queued at first read).
The predecessor run was auto-cancelled by the successor after its two failures;
it is not an overall pass. Product SOURCE remains `8fd0a7e1a`.

### Installed 8fd preflight and native protocol entry gap

The exact DMG installation completed one visible initial Save and five visible
turns in the same thread. Empty-key Settings Save preserved generation 1; an
explicit synthetic replacement advanced generation to 2. Physical requests
observed initial, initial, replacement, replacement, replacement credentials.
Go PID changed from 21160 to 21757; Main changed from 20631 to 21990. Both
restarts continued the original thread without entering a key. The observer's
20-second Core restart call timed out and reset its REPL; it was not replayed.
A fresh observer confirmed the new process, durable thread and successful reply.

The initial preflight had Renderer tracing off (Core/Main and screenshot only).
After enabling the existing Renderer localStorage diagnostic switch, the next
three measured turns included actual DOM commits, 4.8–5.0 ms after store commit.
These are debug/preflight observations, not the complete before/after matrix.

Installed inspection then found a real D05 entry gap: the new reasoning label
was absent from the editor's option list, and model metadata edits did not
persist through Registry Save. The prior source-only Messages fixture supplied
legacy ModelProvidersJSON and therefore did not prove this desktop entry.
`8fd0a7e1a` is retained as a superseded intermediate candidate; no 120-turn or
complete installed acceptance is claimed. Its isolated cohort is stopped and
retained. The correction adds an explicit native Messages Registry kind and
Provider-level UI choice, independent of legacy UI model metadata, with fresh
settings-view readback and real Core reopen/wire regressions. Generic Messages
keeps its existing behavior. Final candidate work continues after these tests.

### Native Registry entry freeze: 755c2bec7

Product SOURCE `755c2bec7cefe316d0fc3b2ed057d007dc271926`, tree
`2e317851cf1ae6e1eb245b341efc19f26a9435ed`, is committed and pushed.
The Provider Settings endpoint selector now saves `deepseek-messages` through
the existing Registry transaction, preserves an empty credential draft with
`keep`, and reads the choice back from the Core projection. The production
resolver derives the native request protocol from this committed kind without
depending on legacy UI model metadata. Generic Messages is not auto-promoted.

Frozen verification, all exit 0:

- Mounted Registry/settings/save suites: 3 files, 104 tests, 6.89 s.
- `TestRuntimeHTTPNativeMessagesRegistrySurvivesRestartWithoutUIModelMetadata`:
  six actual HTTP/SSE turns across Core shutdown/reopen, 93.72 s (package
  95.187 s), no `ModelProvidersJSON` fixture. Observed low/medium/high/max/off/auto
  encoding, the default cap, no unconfirmed beta fields, exactly six physical
  requests and completed public replies. Synthetic local server only.
- Registry resolver tests: 0.864 s.
- `npm run typecheck`: exit 0.
- Final `go test -race ./internal/adapters/outbound/provider/client -count=1`:
  2.129 s, exit 0.
- Production assembly, native Registry resolver and bounded model-metadata
  regressions: six selected tests, package 28.355 s, exit 0.

The normal clean-checkout package and DMG installation completed with exit 0.
Strict deep signature verification and the whole installed-bundle legal audit
also exited 0. Its installed acceptance is not inherited from the 8fd
intermediate. The 8fd DMG/ZIP and preflight receipts are retained together in
the protected evidence directory.

The original 6a baseline's physical requests used `deepseek-v4-flash`; the 8fd
debug preflight used `deepseek-flash`. These are not matched performance samples.
The final comparison explicitly aligns the model and supplements the baseline's
older observer, which did not record protocol/effort/cap. The stopped 6a fixture
is resumed through its existing exact-artifact acceptance owner for this bounded
control, with original metadata retained. No failed mutating turn is replayed.

### Installed submit-lock repair: e7e250812

The 755 installed benchmark exposed a genuine desktop defect: after the first
new-thread turn completed, sidebar/index I/O could still retain Workbench's
`[workspace, null]` submission lock. A subsequent enabled New Agent Send click
was ignored; no second Core thread or Provider request was created. The retained
draft, actual DOM click, lock identity and durable state were inspected before
any retry. This is not a Provider latency observation.

The mounted Workbench regression reproduced the defect (exit 1, expected two
Provider sends but observed one). The fix ends the send receipt after Core
acknowledgement and lets the existing context-fenced sidebar refresh finish
independently. Its original scope/generation checks remain. Unexpected refresh
failure is a fixed diagnostic, not a retryable chat error after acknowledgement.
The complete-candidate publication contract is unchanged.

Final frozen source checks, all exit 0:

- `NODE_DISABLE_COMPILE_CACHE=1 npx vitest run src/renderer/src/components/Workbench.canvas-send.test.tsx src/renderer/src/store/chat-store-thread-actions.test.ts src/renderer/src/store/chat-store-navigation-actions.test.ts`: 153/153, 3.92 s.
- `NODE_DISABLE_COMPILE_CACHE=1 npm run typecheck`.
- `go test -race ./internal/adapters/outbound/provider/client -count=1`: 2.046 s.

The 755 DMG and ZIP, whole-bundle audit and installed observations are retained
under `intermediate-755c2bec7` in the protected evidence directory. Its premature
credential-keep observer sample and ignored-send samples are excluded from speed
comparisons. The stopped 755 cohort is not called the final acceptance cohort.
The final source is rebuilt and installed before the remaining journey. A cache
preflight once rejected a file moved concurrently by evidence retention; no
build/commit ran from that failed command. The same command passed once the
retention operation completed, without relaxing the helper.

### Installed protocol identity correction: 5969d66ae

The e7 DMG install, strict signature and whole-bundle legal audit passed (exit 0).
Its visible synthetic Save/keep/set, original-thread Go and Main continuation,
17 short/large ASCII/CJK text/reasoning samples, five complete trace samples,
and four larger-history continuations plus Main recovery completed. These are
retained intermediate observations, not the final candidate's 120-turn gate.

The first 23 samples enabled Main/Core trace but left the independent Renderer
localStorage opt-in off. They include actual captured answers but no DOM trace.
The next five explicitly enabled the existing Renderer opt-in and correlate
Core, Main, store, actual MessageBubble DOM, and capture interval. No missing
stage is fabricated. Long-history Main startup took several minutes on this
host; process sampling found a waiting Main and a Core read at the sample point,
which is insufficient to attribute a specific algorithmic cause. The same
process eventually recovered all four answers and continued without re-entry.

Installed native selection then exposed an actual identity-contract mismatch:
Registry `Manager.execute` and transaction validation reject any saved Provider
kind rewrite. The new editor choice and its in-memory test oracle had allowed
one. Two bounded visible attempts left Registry revision 4 unchanged; no key,
selection or identity was modified. A normally created native Provider then
persisted as `deepseek-messages` and sent a successful Messages request with
`thinking=enabled`, `output_config.effort=low` and default `max_tokens=4096`.

The correction preserves the Core invariant. Protocol is chosen at creation;
a committed Provider shows a disabled selector with an English/Chinese
explanation. The persistence helper rejects a protocol rewrite before mutation.
The mounted fixture now enforces Core kind immutability and stable generation
on credential keep. The old false-positive kind-switch assertion is replaced
by real creation/readback/keep and explicit rewrite rejection. Existing normal
credentials do not migrate or require re-entry.

Checks on this frozen source, all exit 0:

- Four desktop suites: 158/158, 4.74 s; the focused pre-fix regression exited 1.
- `npm run typecheck`.
- `TestManagerRejectsProviderKindRewriteWithoutMutation`: 0.475 s.
- Client race; its exact timing is retained in the protected command log.

The e7 artifact and 34 physical synthetic requests are retained under
`intermediate-e7e250812`. Its profile is stopped. The successor is rebuilding
from a clean checkout; no previous installed gate is renamed as this successor.

### Current-SHA CI found a continuation reference/privacy collision

`5969d66ae` Development run `36256854787`, job `108445476512`, failed
`TestPostC8ContinuationSourcesDriveRealToolEffectsAfterTwoCompactionsAndRestart`:
second source reference was no longer resolvable (`reads=1`, `writes=0`).
This is a product correctness defect, not an external queue condition. A
deterministic decimal-heavy digest reproduced the final privacy projection
collision (exit 1, `analytix-post-c8-reference-red.log`).

The successor uses a domain-separated, letter-only model-side source selector
and snapshot commitment. Local sealed V4 snapshots and their hex references
remain unchanged; the bounded source reader accepts the legacy reference and
the new selector only against the current thread/principal/workspace. No PII
projection exception or permission was added. The provider view is explicitly
versioned v2; source text remains ordinary untrusted task data.

The five affected domain/privacy/turn/catalog/model packages passed (exit 0;
0.578/0.461/0.859/0.974/1.404 seconds). The failed full consumer/effect test is
being rerun before freezing the successor.

`5969d66ae` is retained under `intermediate-5969d66ae`: visible credential
Save/keep/set, Go/Main recovery, native Messages creation/recovery, 20 benchmark
samples, four long-history samples and original-thread Main continuation.
All 32 observed samples have linked Core and actual DOM observations. The last
Main continuation overlapped cache preflight and is correctness-only evidence.
No final semantic or 120-turn result is claimed for this superseded candidate.

The corrected full production consumer/effect regression passed with
`GOMAXPROCS=2 go test . -run
'^TestPostC8ContinuationSourcesDriveRealToolEffectsAfterTwoCompactionsAndRestart$'
-count=1 -v`: 174.51 seconds (package 175.314), exit 0. It checks two
automatic compactions at the actual wire consumer, a new Core, two authorized
source reads, exactly one C.txt write, unchanged A.txt/B.txt, and another Core
readback without repeating the completed effect.

The same frozen correction then passed provider-client race (2.000 seconds)
and the three required `analytix_prod` assembly/drain/authority tests
(30.309 seconds); the combined command exited 0. UI source is unchanged from
the 158-test/typecheck-verified `5969d66ae` tree. No new desktop test claim is
created by this Go-only successor.

### Frozen successor 5e505d4fa

SOURCE `5e505d4fa7bd4083f2d7e66f5546108f3d8f0249`, tree
`2dc5dcc692490fcec50089bcd00aa4178f2cc4f4`, pushed normally on the original
branch. Build, read-only DMG install, strict deep codesign verification and
whole installed-bundle legal audit all exited 0. Artifact audit has zero
admission blockers; signing/notarization/release authority remain separate.

- DMG: `d9a6798b037e929128985859b7b173d948b11c1a4fe484cf7b5ab70e26e13e6f`
  (321,606,946 bytes).
- ZIP: `3bb5cffdc4f539cb275d322357df2aabd2f5d0f69edc048897aca892838876e7`
  (331,427,878 bytes).
- Independently hashed retained delivery copies: evidence
  `candidate-5e505d4fa/`; installed app `analytix-post-c8-installed-5e505d4fa/Applications/analytix.app`.
- Current Development run `36259104639` was queued at initial read.
- Fresh synthetic acceptance cohort `analytix-post-c8-after-8QHPjB`, Main
  60532 at launch. No previous profile, key or authority was copied into it.

### Installed compaction readback defect found on 5e505d4fa

The fresh cohort passed visible Save/keep/set, Go/Main restarts and native
Messages creation. Eleven semantic seed turns reached two automatic
compactions at the actual Provider consumer, before any semantic file effect.
Thread detail then returned HTTP 502 both before and after a Go restart. This
was a failed public readback; absence of a Goal was not inferred from the error.

Offline reproduction using this synthetic snapshot found an exact shape drift:
Core's compaction turn omitted `steering`, `attachmentIds`, `activeSkillIds`
and `injectedMemoryIds`. TypeScript parsing inserted defaults, and Main's
unchanged exact-value check correctly rejected the changed shape. Neither PII
projection nor continuation content caused this failure.

The correction materializes only absent contract defaults in the Go public
copy, including legacy/reloaded turns; durable state and existing invalid
values are not rewritten. The focused pre-fix regression failed on missing
`steering` (exit 1); post-fix thread package passed (2.695 seconds, exit 0).
An offline copy of the actual synthetic response now passes Main unchanged
(status 200, no schema/sanitizer changes). Main adapter tests plus the temporary
diagnostic: 109/109, exit 0, 7.44 seconds. The temporary diagnostics were removed;
permanent compaction/reload and full production recovery assertions remain.

Additional installed probing on this superseded source: truncated private text
was never public. One explicit bounded model recovery occurred and produced a
`recovery` terminal, not ordinary success. An initial Stop fired before Core
accepted a turn; waiting for a turn-terminal consequently timed out. It is
recorded as an early-cancel fixture mismatch, not in-flight cancellation proof.

The frozen readback correction is commit `b7c1283005a331b6e4523835b83c57078c8620ed`
(tree `9ca6a86f27cfb3160aeb5afbcf0c4d145f727051`), pushed normally.
Final targeted commands on that source all exited 0: thread package 2.695 s;
full two-compaction/real-tool/restart root regression 229.40 s (package 230.968);
client race 2.201 s; the three `analytix_prod` assembly/drain/token-authority
tests 29.483 s. The root recovery assertion also checks no Goal and the exact
public arrays after a second restart. Production/Main publication validators
were not relaxed.

The 5e intermediate profile is stopped and retained. Additional probes observed
an accepted-final `source_unavailable` result with no extra Provider call,
ordinary replay with the duplicate ACK ignored and no extra Provider call,
and in-flight Stop with a cancelled fixture connection and Main HTTP 200
readback of `aborted`. The cancelled UI stream is removed before final SSE,
so its old observer-only timeout is retained rather than counted as a pass.
The native minimize/raise route completed a synthetic response; this host's
Electron CDP lacks `Browser.getWindowForTarget` and document visibility stayed
`visible`. No OS-level minimized-state or pixel-scan timestamp is claimed.

The b7 exact candidate is rebuilding. Its fresh acceptance evidence is kept
under a separate `final-b7c128300` directory so the 5e failure records are not
overwritten. Development CI `36261507817` was initially queued; source and
installed-readiness verdicts remain pending/false until their own evidence.

### Final visible progress consumer correction

The D07 audit found that ordinary parent `pre_send`, `post_send` and
`response_received` events were filtered out of the existing status owner.
`6a5e2f088` admits this closed stage set, requires a turn identity, and updates
one existing row per turn. Main and side conversations use fixed English/Chinese
labels and never event-supplied prose. No private text or publication capability
is added. The regression first failed without the mapping.

The first test command passed 216 tests but exited 2 at typecheck because two
new test arrays inferred `string`; the arrays now retain literal types. Final
main/side/mapper tests: 212/212, 1.61 s, exit 0; actual component: 4/4, 2.04 s,
exit 0; `npm run typecheck`: exit 0. The accidentally nonexistent component
filter in the first final command selected only the three real suites; the
actual component command was then executed separately. Provider client race:
2.139 s, exit 0. Production assembly: three selected tests, package 37.793 s, exit 0.

The b7 DMG was successfully installed and audited, but its later launch was
correctly refused after this product correction made the canonical snapshot
differ. It never entered a credential or sent a Provider request. Its artifact
receipt and `final-b7c128300` directory are intermediate evidence, not final
acceptance. The final candidate is the clean `6a5e2f088` successor.

Final candidate launch initially used the dirty canonical cwd and correctly
failed `packaged_build_authority_worktree_snapshot_mismatch` before launch. The
acceptance owner compares `process.cwd()` with the build snapshot, including
retained workspace changes. The retry uses the actual clean build checkout as
cwd; the canonical imported owner is byte-identical to that SOURCE. No guard,
artifact or protected user file was changed. This is not a credential failure.

### 6a5e2f088 installed result and replay correction

The exact DMG-installed cohort passed visible Save/keep/set, Go process replacement,
Main recovery, public Messages wire and Main recovery, eleven semantic seeds with
at least two automatic compactions, then actual source-reader calls and a single
approved C.txt write. A/B stayed unchanged. Main/Core recovery preserved the
completed effect (C mtime unchanged), public readback 200, no Goal, and continuation.

Twenty independent benchmark samples and four same-thread 64 KiB outputs completed.
Short ASCII single-chunk median: 11,712.536 ms (three samples); the four history
samples: 10,147.315 / 11,531.499 / 13,181.967 / 14,933.822 ms. Main-restart
continuation: 15,190.894 ms. These are single-host observer measurements, not
cloud latency or an overall speedup. A 45-second window-start observer expired;
the same Main later produced its window without restart. This startup wait is
excluded from send-to-answer samples and retained as a separate observation.

The stage table linked 45 of the first 46 observations to Core and real DOM.
Example 64 KiB/1024 text: total 12,965.264 ms; Core publication gap 3,166.941 ms;
Main verification 8.714 ms; store-to-DOM 3.6 ms; composited capture call bounded
12,970.063–13,173.408 ms. Host stream-entry to raw text was 2,193.435 ms, including
Host supervised setup, not pure model latency. The 6a56571c1 baseline is unchanged.

Installed T03 withheld truncated private text and settled with explicit recovery
classification before a unique follow-up succeeded. T04 waited for the actual
Provider heartbeat, cancelled through visible Stop, observed server connection
close and Main readback `aborted`, then a unique follow-up succeeded. Its early
screenshot preceded delivery of pre_send, so it is not evidence of that label.
T02 returned accepted-final source_unavailable with zero Provider requests; this
is a valid fail-closed protected terminal, not positive Funds facts. T05 A→B→A
readback left the physical Provider count unchanged.

T07 exposed a real Main bug. Two-turn replay delivered the first verified terminal
but then parsed the remaining coalesced historical frames as one unfinished frame,
emitting malformed_frame before the ACK settled. No new model request or file
effect occurred. The focused existing test now covers separate and coalesced
chunks: separate passed, coalesced failed before the fix (exit 1). The successor
ends parsing when the existing terminal owner ends the connection, preserving
all current-frame verification and ACK checks. Sealed protected terminal coverage
uses the same chunking cases. Final 120 has not started, and 6a is an intermediate
candidate despite the evidence directory's earlier `final-` name.

CI run 36262438682 job 108461014986 failed at CreateThread in the generated-artifact
PPTX test, before artifact settlement: its ordinary workspace used numeric
testing.TempDir suffixes. The correction reuses the existing alphabetic
workspacetest.New helper; privacy validation and the deliberately decimal artifact
receipt hash remain unchanged. This is a test-input correction, not a privacy
exception. The job's exact log is retained privately.

The replay correction's four desktop suites pass 155/155 (6.37 s, exit 0),
including coalesced ordinary and sealed protected terminal frames, hostile frame
rejection, preload filtering and actual component traces. Typecheck exits 0.
The generated-artifact test passes three repetitions, each covering DOCX/XLSX/PPTX
(package 37.513 s, exit 0). The CI did not log its rejected workspace value;
the numeric-path attribution follows the fixture and established helper contract,
not a captured sensitive value. No privacy matcher or artifact assertion changed.


### 6d29c8634 installed evidence and security-gate successor

The 6d DMG/ZIP were built normally and installed from a read-only DMG mount.
Strict signature and legal checks exited 0. DMG SHA-256:
`b310b78a32b5bdb99df09fda43cc2d4e777055acc39df6a3d527b8465b898723`;
ZIP: `91f961554bd06e74280a303927020751231f82fc50245c5b9246df66ddb1f095`.
Separate copies in `candidate-6d29c8634` were hash verified before dist reuse.
Fresh client race with `-count=1` passed in 2.012 s; production assembly in 21.603 s.

Normal visible Save, empty keep, explicit replacement, Go restart (42868→43580),
Main trace-off recovery (42539→43764), and Messages plus trace-on Main recovery
(43764→44116) passed. Actual wire used the saved synthetic revision, Messages
x-api-key, enabled thinking, low, max_tokens 4096, and no beta header. The observer
attempted a composer fill too early after Settings Back and timed out before Send;
request count was unchanged, and a separately numbered follow-up passed.

T07 now passes on the installed two-plus-turn replay: normal ACKs true, duplicate
ACK false, no malformed_frame and no new Provider call. Eleven semantic seeds
produced at least two automatic compactions. Go restart 44124→44795 returned 200
with canonical public arrays and no Goal. Two actual read_task_history calls
consumed the early A/B restriction and later C correction; one visible Allow
approved the C write. A/B remained untouched; C became corrected task completed.
Main 44116→46480 preserved C mtime, one write and strict semantic readback 200.

The frozen 6d cohort completed twenty independent benchmark cases, four same-thread
64 KiB samples and the new-Main continuation. Short ASCII median was 10,889.997 ms;
64 KiB/1024 ASCII text 13,006.442 ms, publication gap 2,249.460 ms, Main verification
8.809 ms, store-to-DOM 3.4 ms, capture bound 13,014.400–13,223.278 ms. History sample
four was 16,808.473 ms and post-Main 16,790.852 ms. Of 44 total observations, 42
linked Core and actual DOM; the two trace-off cases correctly had no diagnostics
and completed normally. Raw timing remains an observation, not a causal speedup.
A pre-window CDP connection refusal was retried read-only; the same Main eventually
created its window. Slow cold startup is recorded separately from turn timing.
The 1-second native process sample found the Main thread waiting in the AppKit
run loop; this alone does not establish the asynchronous startup dependency.

CodeQL check 108468258427 on 6d failed with one high insecure-randomness alert at
side conversation client-message ID creation. The existing Math.random fallback
was also present in the related steering ID owner. `74e4316ea` removes both fallback
paths and uses Web Crypto directly. Two new negative tests first failed because
messages still sent without crypto (exit 1). Five final desktop suites pass 159/159
in 5.44 s, exit 0; typecheck exits 0. No unsafe PR alert dismissal or rule suppression
was used. This successor's Go subtree is identical to 6d:
`53df1b7728281ef62e83cec360b51537675b9a82`. Final 120 has not begun. The retained
6d semantic profile will be reopened in place by the new exact candidate; no
profile or credential material is copied.
