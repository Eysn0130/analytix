# PR28 N01–N09 continuation: live evidence ledger

Status: in progress. This is a successor to the immutable
`pr28-post-c8-completion-2026-09-26.md` record. It does not reclassify that
record's product SOURCE or transfer its installed acceptance to a new build.

## Exact identity and stage boundary

- Canonical branch: `codex/workbench-product-delivery-20260914`. Starting local
  and remote PR HEAD: `56231ff5e6b7dd8d321d1356b7208587df0841e9`; remote
  `origin/main`: `ce96cf12581acfa0e19fae7c6aa9c709371012c8`.
- The previously admitted synthetic Core installation remains SOURCE
  `74e4316eabff41ad72878001b5be3b89db020a1d`, tree
  `7f0c6b5a47a9349916b90d8b1adbf84216783c3a`. Its live Provider check
  was open. The product matrix's stale e15 Current entry was corrected in
  `a0e2a866f`; e15 remains historical.
- Earlier local successor SOURCE: `28ed097f2f057be0b1535e48e8a50b2c22914abc`,
  tree `76dd4468eed36096a2dffff3b902732ea4422078`. The clean detached
  worktree's Core build produced DMG SHA-256
  `b9a46189e9df3be58869dadb8bf2ecb69369a011e2537f14aa940a6ef2c43bf2`
  and ZIP SHA-256
  `fee8c3fe7686e49fcf74a897f2af42276a9a291b0afaca58dce3ab378642e141`.
  Its DMG-installed copy is at
  `/Volumes/AnalytixCache/development-v3/tmp/pr28-n03-28ed-installed.euf8ti/Applications/analytix.app`.
  Build, read-only DMG mount, install, strict deep signature verification and
  installed-bundle mandatory legal audit exited 0. The installed copy passed
  a bounded live Provider, Go restart, and Main restart journey below. Its
  window startup latency remains unacceptable; a new source change requires
  its own installed verification.
- Core package profile excludes the Funds plugin artifact and professional
  native components. Its ordinary Core results cannot establish Funds fact
  execution. `FUNDS_ACCOUNT_FLOW_STAGE` is a separate, still partial stage.
- As of this record, PR #28 remains Draft/open at remote HEAD `56231ff5e`;
  its prior 57/57 successful checks do not apply to the local successor.
  There has been no push, merge or public release.
- Successor SOURCE `107d68c2e0402895b5e40039323ebcf1401c1940`, tree
  `8c4d8bd4aac3a2b34bdf52d105a9809d8a3f1456`, was built from a clean
  detached worktree. Its immutable Core DMG SHA-256 is
  `249f2f3a4d9d22e5dee806454c40cffa7cb754e4503530d7a5c01b99c1eddf9a`,
  ZIP SHA-256 is
  `c6942c7e06e846de196e60fd857e92910e2d0a3fbc95afd22e0d9205bfe9e752`,
  and build snapshot is
  `9b25d01985ced700569a939499f491fddcf7e1c52d44a6cef94372c9e9732d38`.
  The exact DMG-installed app passed strict deep signature verification and
  mandatory legal audit (`status=passed`). Its installed Main SHA-256 is
  `bbf5508fe330eec315a8daaa1e77e094cab116de146a76e4560b7407c3ae37b4`,
  and Go SHA-256 is
  `b24afa74518730e5fc540cab0ffff04b009c53c890b58ba4f33e29ac2bd038cc`.
  Bundle authority still says `development_clean_non_publishable` and
  `publishable=false`.
- The prior committed local SOURCE `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`,
  tree `d9c3f6ee98ad015074a4e0d7706b734baf79087b`, is **not an accepted
  fix**. Independent audit found a remaining ordinary-result false positive
  and a real-payment false negative in its path-stripping classifier. Its
  clean Core build was stopped before candidate completion. No earlier artifact
  or installed result transfers to its successor.
- The subsequent source-level S1 repair is local commit
  `a5dc3b0ea866cb7792dfb6cf1ab6f4d65f50887a`, tree
  `e164ed1c53012a08609ef136957c855458333374`. It has focused Go,
  Main/shared/Renderer and typecheck evidence below, but no exact-source
  package, installed result, current-HEAD CI or merge result yet.
- An interim S1 successor was `24ff5764edebf73f1769e53817cb2de57bba7a4e`,
  tree `445f77403f9ac7f672523a14b8635319090b02c3`. An independent
  review found two explicit case-assertion forms that `a5dc` did not catch.
  The successor has focused Go HTTP, Main/shared/Renderer and typecheck passes
  for those forms, but no exact-source package, installed result, current-HEAD
  CI or merge result. The finite grammar is bounded evidence, not a guarantee
  for arbitrary natural-language case assertions.
- The latest local source successor is `c8fa02f99e7fcd2cdcd037990f3ebd06b66500e2`,
  tree `812abaaf4ca947af1166e118c9470a1e08b59481`. Review of `24ff`
  exposed a regression in an older acquired-currency shape with a preceding
  year. The narrow `取得` check was restored and the new negative vector passed
  the focused Go HTTP and Main/shared/Renderer checks below. A clean private
  Core DMG/ZIP was built from this exact source, mounted and copied to an
  isolated installation; hash, signature and mandatory legal checks passed.
  The installed ordinary-file journey, current-HEAD CI and merge remain open.

## N01–N09 ledger

| Item | State | Evidence and remaining condition |
| --- | --- | --- |
| N01 identity and matrix | DONE_IMPLEMENTED | Fresh branch/PR/main/tree inventory; Current corrected to 74e with e15 history retained. Core/Funds stage scope and artifact exclusion inspected. |
| N02 installed real Provider | DONE_IMPLEMENTED | The 28ed installed copy reused the credential reference from normal QA onboarding. A new short turn completed with `2 加 3 等于 5。`; after Go PID 66130→67281 another turn completed with `2 + 3 = 5`; after Main PID 55003→89415 and Go PID 797, the same durable thread displayed both prior turns and a third live turn completed with `56`. No credential value was read into this record or re-entered. This applies only to 28ed; a changed final SOURCE needs a bounded repeat. |
| N03 startup and delivery latency | UNFINISHED | Diagnostic 4e package traced Main settings load 2.4 s and awaited extension account reconciliation through 42.3 s before window creation. The 28ed Main restart still awaited extension reconciliation and OAuth sweep/scheduler before window: 78.2 s from JS evaluation to startup surface. `9faf0ee02` alone did not improve this path. `cf9f1988b` schedules OAuth recovery outside window creation and gates OAuth IPC until ready; `107d68c2e` stops late startup work on confirmed quit. Installed comparison remains. `6c103cc52` protects live sibling runtimes. Host delivery comparison remains. |
| N04 production contract and ordinary content | UNFINISHED | `49d066ef2` and `28ed097f2` extend the shared TS/Go conformance corpus for Chinese years, counts, dates, exact ordinary amounts, UUID/hash, relative numeric/Chinese paths, and synthetic account/identity/phone masking. TS 68/68 and Go shared corpus passed at 28ed. On 107d, an exact installed ordinary file task in a fresh workspace falsely entered the case lane before Provider invocation. The 81b classifier's focused test passed but independent negative vectors invalidated that candidate. `a5dc` passed focused source checks but missed two explicit case assertions; `24ff` repaired those but regressed an acquired-currency form. The `c8fa` successor passed focused Go HTTP/public and Main/shared/Renderer checks for the observed forms and has an exact-source clean private Core package. A later fresh-profile installed synthetic session passed 19/19, but did not exercise the exact ordinary-file assertion. Its visible GUI/real-Provider confirmation remains open after an unsafe app-name UI selection event; the earlier timed-out expression was not replayed. Absolute local paths continue to be withheld by Main logging rules. |
| N05 ordinary and Funds continuity | UNFINISHED | Existing General reader enforces current thread/workspace/principal, rune pagination with `totalRunes`/`complete`, and changed-scope refusal; four focused Go continuity tests passed. Private Funds input, case A→B→A and revoked-source recovery are separate unproved seams. |
| N06 Funds positive data path | UNFINISHED | Existing B1 source test performs normal synthetic CSV import, immutable snapshot, account-flow tool/result, second import and recovery. A direct current-source run stopped before any Provider request because plain `go test` lacks the embedded development signing policy. A second invocation with the exact compiled policy stopped because the test authority was on removable cache storage; neither reached a query. The third run used an isolated 0700 non-removable APFS authority directory but Owner admission returned a generic trust error for an older native package, before any Provider request or query. This is neither an installed Funds candidate nor real-case data access. |
| N07 exact facts and display | UNFINISHED | Existing B1 assertions are source-bound; current installed Funds fact/display and the full negative set remain unverified. No user statement was promoted to bank fact. |
| N08 noninterference and secrets | UNFINISHED | Core package exclusion and sibling runtime protection have direct evidence. Six Funds capability states, cross-platform native installation and source revocation remain separate. |
| N09 freeze, CI, PR/main | UNFINISHED | Clean private Core packages exist at 28ed and now exact `c8fa`, each with own hash/signature/legal receipts. An initial `c8fa` installed synthetic attempt timed out; after the harness repair a fresh-profile segmented run passed 19/19 installed checks. The exact ordinary-file GUI and protected real-Provider checks, current-HEAD CI, PR review/merge gates and public release qualification remain open. |

Allowed ledger states are `DONE_IMPLEMENTED`, `DONE_EXISTING`,
`CLOSED_WITH_SUPPORTED_FALLBACK`, `BLOCKED_EXTERNAL`, `FAILED`, and
`UNFINISHED`. An unshipped stage is a scope fact, not a substitute state.

## Focused repair and verification

| Candidate and command | Result |
| --- | --- |
| `4e08ed825` startup trace on exact DMG-installed copy, isolated profile | Main in-process window path crossed extension-account reconciliation at 42.3 s; this is one external-volume observation, not a distribution. |
| `9faf0ee02` `src/main/desktop-startup-contract.test.ts` and typecheck | 15/15 focused tests and typecheck exit 0; installed improvement not yet measured cleanly. |
| `6c103cc52` live-sibling port regression | Before fix: preferred port was reclaimed by terminating the live child, exit 1. After fix: fallback port selected and child remained alive; full `analytix-process.test.ts` 57/57 and typecheck exit 0. |
| `28ed097f2` TS and Go ordinary PII shared corpus | TS 68/68 and Go `TestOrdinaryPIIProjectionV1SharedConformanceCorpus` exit 0. An attempted absolute-path preserve vector failed in TS, and switching it to a shared mask vector failed in Go because these projections have different owners. The corrected shared relative-path vector passes; Main's absolute-path masking remains covered in its own test. |
| Clean 28ed Core DMG/ZIP and installed copy | `npm run dist:mac:arm64:core`, DMG attach/ditto/detach, deep strict codesign, and `artifact-legal-obligations-audit.mjs --artifact <installed app> --json`: each exit 0. Ad hoc signed, notarization skipped; mandatory legal audit passed, release authorization unverified. |
| Exact 28ed installed live Provider and restarts | Normal protected Registry readback; three distinct new turns completed in the GUI without key re-entry. Go PID 66130→67281; normal Main quit/relaunch PID 55003→89415 and Go PID 797. Same isolated QA profile and durable thread; no physical Provider request count observed. |
| Exact 28ed Provider configuration and durable turn metadata | QA settings selected `deepseek`, `chat_completions`, `deepseek-v4-flash`; durable `turn_started` records for `turn_1`–`turn_3` each recorded provider `deepseek`, model `deepseek-v4-flash`, reasoning effort `max`. Each turn's durable `pre_send`/`post_send` stages recorded `chat_completions`, one client physical attempt and HTTP 200; `post_start` recorded max model steps 64. This does not prove a cloud-side exact physical request count. Usage events reported input/completion tokens, but the zero rounded USD field is not a provider bill. No raw Registry secret or request header was read. |
| Exact 28ed Main restart startup trace | `extension account reconciliation:done` 77,954 ms, `OAuth authorization sweep:done` 77,955 ms, `createWindow:start` 77,959 ms, `window:startup-surface-ready` 78,205 ms from Main JS evaluation. External-volume startup environment; this is one observation, not p95 or a universal product timing. |
| Current-source B1 positive test attempt | Plain `go test -tags analytix_prod ./internal/runtimeapp -run '^TestFundsAccountFlowB1ProductionPublicChain$'` exit 1 at embedded development signing policy preflight; zero Provider requests. No assertion about funds data passed or failed. |
| Current-source B1 policy/location preflight | With the exact `embeddedSigningMode=ad-hoc` and `embeddedSigningPolicySHA256=7625f1de...` compile flags, the second run exited 1 before Provider use: the external cache volume was mounted `MNT_REMOVABLE`. The third run used a one-use 0700 `/private/tmp` authority root without changing the policy or copying credentials; its result is recorded below. |
| New startup-source focused checks | `cf9f1988b`: IPC handlers 92/92, startup contract 6/6, TypeScript typecheck exit 0. `107d68c2e`: OAuth lifecycle and startup contract 34/34, TypeScript typecheck exit 0. The new SOURCE still requires installed timing and behavior checks. |
| Exact 107d startup trace and installed refusal | On the external cache volume, launch→Main JS evaluation 98,913.696 ms; Main JS→window startup surface 1,649.982 ms; deferred extension/OAuth work finished at Main 26,154–26,157 ms, after the window. One observation, not a cold-start distribution or proof of external-volume causation. A new ordinary workspace contained only a synthetic file (`2026年资料/表单 42.txt`, SHA-256 `8bbba3d985002f0de89afa1361e6db23e3a91e22c2890cd467fedcc449fa6697`); its first turn returned the case-source-unavailable boundary and had no Provider call. The host assigned `riskClass=case`, `caseBindingState=missing`. This is a product classifier defect, not a credential or case-source external blocker. |
| `81b8be0f2` ordinary numeric path classifier | The exact ordinary prompt test failed before the fix (`ContainsProtectedCaseFactCandidate()=true want false`), then passed; full `internal/domain/security` and `internal/app/loop` Go packages passed at that commit. Later independent audit found `金额:1234.56` still blocked downstream and `甲公司支付2645.72元，见doc/a.txt` incorrectly treated as ordinary. The 81b clean Core build was stopped before a candidate existed. Its later replacement is a5dc; this row does not claim installed acceptance. |
| Earlier cross-layer repair test attempt | After sourcing the required cache helper, focused Go and Vitest commands entered uninterruptible I/O wait without a result. A detached 81b build's `ditto` ZIP process also remained in uninterruptible I/O wait after termination signals. That attempt had no test assertion result; the subsequent a5dc source run is recorded below. |
| `a5dc3b0ea` focused source repair | Five Go packages and a related subagent/server pair passed the named regression sets; four changed Vitest files passed 349/349; typecheck and diff check exited 0. Exact details and prior fixture failure are below. No installed or CI conclusion transfers. |
| `24ff5764e` explicit assertion regression | Before the repair, four named security/loop examples and the shared TypeScript projection failed at `a5dc` for `2万元`, `2亿元`, a relationship assertion, and a mixed software/case prompt. After the repair, four Go packages and three server HTTP journeys exited 0; four changed Vitest files passed 355/355; typecheck, gofmt and diff checks exited 0. This is source evidence for the observed grammar only. |
| `c8fa02f99` acquired-currency regression | Before the repair, Go security/loop and TypeScript projection failed for a `取得2026年收益￥2万元` case assertion. After restoring the prior narrow suffix currency check, four Go packages and three server HTTP journeys exited 0; four changed Vitest files passed 358/358; typecheck, gofmt and diff checks exited 0. No installed or CI conclusion transfers. |
| Exact `c8fa02f99` Core package and isolated install | After resolving four setup/layout issues, the clean detached package command exited 0 with source snapshot `eff2e0ed...`. DMG/ZIP SHA-256, installed Main/Go/app.asar equality, strict deep signature and exact 1,172-instance legal audit passed. The artifact remains `development_clean_non_publishable`; functional acceptance and current-HEAD CI are separate. |
| Exact `c8fa02f99` installed synthetic session | The existing packaged-session harness exited 1 with `cdp_evaluation_timeout`. Its loopback Provider recorded 12 requests and some fork responses, so execution did occur; the harness removed its temporary profile before a durable after-state could be read. The timed-out mutating expression was not replayed. This is partial diagnostic evidence, not an installed journey pass. |
| `931667537` harness regression and exact c8fa segmented run | Two new CDP tests failed before the fix: ambiguous sent expressions lost their isolated profile, and segmented observation timeout lacked an unknown-outcome marker. After repair, 57/57 focused Node tests, syntax and diff checks passed. The first retest from the advanced canonical worktree was blocked by exact package/worktree snapshot mismatch before app launch or expression send. A second run used the clean c8fa worktree, the updated harness, an exact installed app path, macOS CDP-port ownership checks and a new isolated profile. Its actual installed synthetic checks passed 19/19, with 13 loopback Provider requests, redaction pass and process cleanup pass. The top-level command reported `PARTIAL` and exit 1 by `--actual-only` design, since the deterministic suite was not run in that invocation; the nested `actualPackagedSessionSoak.passed` is `true`. This is not an ordinary-file or real-Provider acceptance result. |
| Current-source B1 third preflight | With the isolated non-removable 0700 authority root and correct compiled signing policy, the retained older native package failed normal Owner admission with `native_component_host_trust_invalid` before Provider or DuckDB query. Its bundle is `development_dirty_non_publishable` at old SOURCE `b07eef...`, but the Owner folds multiple inspection failures into this error and the registry accepts both clean and dirty development classes. The exact rejection branch is undetermined; no trust bypass was made. A current clean Funds package and a focused admission preflight are needed for the separate stage. |

The earlier competing 4e, 28ed and 107d QA instances were quit before the
new Core build. Their protected QA profile was retained without copying or
committing it. The later protected-state incident below stops further direct
access or readback under the current task boundary.

## Read-only Astra/xhigh architecture audit, 2026-09-26

The separate audit inspected committed `81b8be0f2` and the changing worktree;
it did not freeze or test the later candidate. It found that the reported
ordinary-file refusal is consistent with turn-start lexical risk escalation,
not with a failed funds query. The committed 81b path-stripping change is
incomplete. The current uncommitted candidate separates some ordinary values
from case risk, but must still prove ordinary code/relationship language,
payment-related software tasks, and an already case-classified thread's
independent ordinary result. A new General thread passing is insufficient to
prove the old thread's monotonic risk policy can be bypassed; no policy
downgrade or history rewrite is authorized.

The audit also identified source-level coupling outside this immediate repair:
the Funds protected call holds a shared MCP execution lock for its duration,
and reconnect can rebuild unrelated clients. Actual latency impact remains
unmeasured. First-party optional capability fault tests already cover several
absence/degradation states, but normal UI disable, in-flight revoke, and
re-enable are not an installed six-state lifecycle proof. The existing Funds
B1 test contains real synthetic import, immutable snapshot, query, and changed
result after a second import; its current installed positive path remains
unverified. Full local values currently reach the main Renderer DOM through
the accepted-slot component, so controlled API access must not be described as
an isolated private display surface. These are separate follow-on boundaries,
not grounds to replace the single Go Agent Harness or relax case authority.

## 2026-09-26 23:14 PDT S1 candidate and verification boundary

The local committed HEAD remains `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`
(tree `d9c3f6ee98ad015074a4e0d7706b734baf79087b`). The uncommitted
`packages/runtime-go` and `src` product/test diff has SHA-256
`6b2ae2db0288a7a204baaea93a91a5836e0ba66dbdabde92a2a587297ec8d2ea`
at this observation. This is a review pointer, not a source or artifact identity.
The unrelated dirty development runbook and two 2026-09-25 QA drafts are preserved
and are not part of the task-owned product diff.

The candidate separates structured PII, unbound case-risk signals, and the
strict guard for a frozen case-sensitive turn across Go, Main, shared SSE and
Renderer. An exact process-local ordinary-only provider input can now compile
an ordinary numeric result in a case-sensitive thread without lowering its
historical case risk; an unproven candidate still uses the strict guard.
Focused additions cover real `read` tool results from a synthetic numeric and
Chinese relative path, a second file version changing all four answer fields,
ordinary numeric result replay validation, a case-to-ordinary steering path,
and Main/SSE/Renderer acceptance. These are **candidate assertions**, not
executed pass claims. The private case Final Gate and structured PII checks
remain required.

`gofmt` on changed Go files and `git diff --check` exited 0. The required
`source ./scripts/use-analytix-cache.sh` preflight exited 0; the backing volume
had 56,968,832 KiB application-available and the mounted APFS cache had
40,139,888 KiB at observation. The focused Go command
`go test ./internal/domain/security ./internal/domain/ordinaryresult ./internal/app/turn -run 'TestContainsUnboundCaseRiskV1SeparatesOrdinaryValuesFromCaseAssertions|TestResultSlotV1KeepsOrdinaryNumericFileAnswer|TestGuardGeneralOutputKeepsOrdinaryNumericFileAnswer' -count=1`
remained in uninterruptible I/O wait with no result after exact-process TERM
and KILL requests.
An earlier focused Go test and Vitest process had the same wait; an earlier
task-owned `ditto` ZIP process remained in `U` state for over one hour. No new
Go/Vitest result, exact-source package, installed result, CI, push or merge is
claimed from this candidate. The mount remained writable with owners enabled;
the precise host I/O cause is not established.

After the I/O seam is healthy, run the focused security, ordinary-result,
turn, loop and server regression tests with the helper in the same shell, then
the changed Main/shared/Renderer Vitest tests and TypeScript typecheck. Review
the full task-owned diff and fix any failed assertion before a focused commit.
Next validate the producer-to-durable/public consumer chain and negative case
vectors, then freeze a new SOURCE for the installed Core journey. Keep the
separate Funds positive query, display, six-state lifecycle, performance and
conditional PR/main exits open until their own evidence exists.

First bounded retry commands, only after the stuck cache I/O has cleared:

```bash
source ./scripts/use-analytix-cache.sh && (cd packages/runtime-go && go test ./internal/domain/security ./internal/domain/ordinaryresult ./internal/app/turn ./internal/app/loop ./internal/server -run '^(TestContainsUnboundCaseRiskV1SeparatesOrdinaryValuesFromCaseAssertions|TestResultSlotV1KeepsOrdinaryNumericFileAnswer|TestGuardGeneralOutputKeepsOrdinaryNumericFileAnswer|TestIsolatedOrdinaryResultKeepsNumericFileAnswerInCaseThread|TestRuntimeRunnerOrdinarySteerRemovesProtectedToolPair|TestOrdinaryNumericFileHTTPPublicSeamReadsChangedContent|TestCaseRiskThreadHTTPPublicSeamReadsIndependentNumericFile|TestCaseRiskClassifierKeepsNumericOrdinaryFileRequestGeneral)$' -count=1)
source ./scripts/use-analytix-cache.sh && npx vitest run src/shared/ordinary-log-pii-projection.test.ts src/shared/public-runtime-sse.test.ts src/main/general-terminal-publication.test.ts src/renderer/src/agent/analytix-mapper.test.ts
source ./scripts/use-analytix-cache.sh && npm run typecheck
git diff --check
```

These commands are recorded for continuation; none is reported as passed here.

## 2026-09-26 23:26 PDT S1 static review continuation

The local committed HEAD is still `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`.
The uncommitted `packages/runtime-go` and `src` product/test diff now has SHA-256
`b00ec22b741fd867a26bdee32123a5acf837267fe7c97f13f61d2bc625c7c489`.
This is a mutable review pointer, not a source, artifact, or accepted candidate.

Static review found that the newly relaxed ordinary-only result classifier
would admit an entity-specific kinship assertion such as “张某与李某是父子” in an
old case-sensitive thread. A payment with an intervening payee also lacked a
bounded positive risk signal. The Go and TypeScript classifiers now include
these two negative shapes while preserving ordinary DOM relationship language,
travel prose, software payment-module counts, and the numeric file answer.
The Go case-thread guard and Main, public SSE, and Renderer tests include
the corresponding sealed negative candidates. These are unexecuted assertions,
not evidence that the gate passed.

`gofmt` on the changed Go files and `git diff --check` exited 0. At observation,
the prior `ditto` PID 9439 remained in `U` state after 1 h 15 min, and the
focused Go test PID 86258 remained in `U` state after 21 min. No new Go,
Vitest, typecheck, package, installed, CI, push, or merge result was obtained.
The previously requested normal-restart decision remains unanswered in this
construction thread; dependent validation must wait for a healthy supported
cache I/O seam.

## 2026-09-26 23:30 PDT S1 path-complete unexecuted regression

The committed HEAD remains `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`.
The latest uncommitted product/test diff SHA-256 is
`40ab06a4edd8a973098fcc43407453473077d42d14e421350a93481a302b6391`.
The revision above adds an HTTP case-risk-thread journey: a case-source
unavailable turn makes zero Provider calls, then a separate ordinary turn
must actually read a synthetic numeric file through the `read` tool, publish
its fields, and leave the earlier case boundary intact. The monetary action
classifier now requires a currency marker or unit for a directly adjacent
number, preserving ordinary phrases such as “支付2次测试”; a bounded payee form
still treats a concrete amount as case risk. Go and TypeScript have matched
positive and negative fixtures for those rules. The test command above now
includes the new HTTP test name.

`gofmt` and `git diff --check` exited 0 on this revision. The earlier `ditto`
PID 9439 remained `U` after 1 h 18 min; focused Go test PID 86258 remained
`U` after 25 min. No test assertion, package, installed, or CI result is
available for this revision. Do not commit, push, or merge it as accepted.

## 2026-09-26 23:41 PDT B1 independent-reference candidate

The committed HEAD is still `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`.
The `packages/runtime-go` and `src` product/test diff now has SHA-256
`7cfb0b14824a682d7525ed392cfa4119553e6b261930efc819ed13d49f90ed9a`.
This is a mutable worktree review pointer, not a frozen SOURCE or test result.

The existing B1 production-composition test now reads the two actual synthetic
CSV import inputs and independently computes the selected CNY account's inflow,
outflow, net minor units and transaction count with integer arithmetic. It
compares those values with the native-result model-safe semantic envelope and
the prepared fact record, while retaining the normal import, immutable
snapshot, Final Gate, protected display and recovery path. The changed input
must yield a different reference outflow and net. This oracle covers the
current bounded account-flow fixture; it is not a general funds-query proof or
an installed Funds acceptance.

`git diff --check` exited 0. A read-only `gofmt -d` inspection of the changed
Go file exited 0 with no formatting diff; it uses no build storage. A separate
attempt to source the required cache helper before `gofmt -w` made no progress
for 22 seconds and was interrupted with exit 130 before `gofmt` ran. At the
last process check, the older `ditto` PID 9439 and Go test PID 86258 remained
in `U` state. Do not launch another dependent test until the supported cache
I/O seam is healthy. The B1 oracle still needs the correctly admitted clean
current Funds native input and a focused
production-tag run before it can be accepted. N06, N07 and N09 remain
`UNFINISHED`; no new package, installed test, CI, push or merge was completed.

Source inventory for N08 remains narrower than installed acceptance:
`TestRuntimeOptionalPluginOrdinaryLifecycle` covers missing, disabled,
incompatible, unauthorized and domain-semantic faults with ordinary coding,
writing, research, Skill, MCP, job and recovery effects. The MCP manager tests
cover Funds grant stop/reconnect and post-native revocation separately. They do
not constitute one installed absent→disabled→revoked→re-enabled journey or
prove current GUI reachability. No N08 test was rerun on this worktree.

## 2026-09-27 00:04 PDT S1 audited counterexample candidate

The committed local HEAD remains `81b8be0f2e2e0db13b9cfd1213ce7a44b74be350`
(tree `d9c3f6ee98ad015074a4e0d7706b734baf79087b`). The mutable
`packages/runtime-go` and `src` product/test diff SHA-256 is
`36e96490b7177055ca3104a5f2a3d284f8d7fd3579f86c5d57654a06465bce05`.
It is not a frozen SOURCE or artifact identity. The untracked 2026-09-25 QA
drafts and the user's dirty development runbook remain outside this candidate.

The unexecuted S1 repair extends the Go and TypeScript unbound classifiers to
recognize a concrete payment with a directly following payee and amount, while
retaining ordinary payment test counts. A bare DOM `父子关系` no longer raises
unbound risk; explicit entity assertions such as `张某与李某是父子` remain rejected.
Go input admission now checks concrete assertions before applying the pure
software shortcut. Focused assertions cover the exact mixed software/case
prompt, independent software partition, fresh HTTP input and active-steer
boundaries, ordinary ResultSlot creation and replay, isolated old-Case ordinary
output, and sealed
Main/SSE/Renderer acceptance or refusal. These are source assertions only;
there is no claim of runtime success or installed safety.

Read-only `gofmt -d` on the changed Go source and tests returned no formatting
diff, and `git diff --check` exited 0. At the observation, prior `ditto` PID
9439 remained in `U` after 1 h 50 min and prior Go test PID 86258 remained in
`Us` after 56 min. No Go/Vitest/typecheck command, source freeze, package,
installed journey, current-HEAD CI, push or merge ran for this candidate. The
supported cache I/O seam is still blocked; its precise host cause is unknown.
N04, N06, N07 and N09 remain `UNFINISHED`, and all four current-worktree exits
remain false.

After the supported cache seam is healthy, source
`./scripts/use-analytix-cache.sh` in the same shell as each verification
command. Start with `TestContainsUnboundCaseRiskV1SeparatesOrdinaryValuesFromCaseAssertions`,
`TestContainsUnboundCaseFactAssertionV1DoesNotUseSoftwareWordsAsAuthority`,
`TestCaseRiskClassifierKeepsCaseAssertionsAheadOfSoftwareShortcut`,
`TestResultSlotV1RejectsDirectPayeeFactOnCreationAndReplay`,
`TestIsolatedOrdinaryResultKeepsNumericFileAnswerInCaseThread` and
`TestMixedSoftwareCaseAssertionHTTPAdmissionKeepsLaterOrdinaryTurn` and
`TestHighRiskSteerCannotEnterFrozenGeneralTurn`, together with the existing
numeric-file and case-thread regressions named above.
Then run the changed four Vitest files and TypeScript typecheck, fix any actual
failure, and review the stable diff before a focused commit. B1 still needs a
clean current Funds native package and its separate positive query run.

## 2026-09-27 S1 focused source verification and local commit

After the Mac restart cleared the earlier stuck processes, the cache preflight
completed normally. The first focused Go run passed security, ordinaryresult,
turn and loop but failed two new server HTTP tests with `turn_failed` 500.
The synthetic Provider fixture had incorrectly expected its raw tool-call ID
after the Host minted the actual tool-call ID; it also read the General and
Case terminal item through the same helper despite their distinct durable
representations. The fixture now checks the Host ID, reads the latest actual
`read` tool result from the file, and checks the appropriate public terminal
for each turn. Both HTTP tests then passed together (`internal/server`,
40.227 s, exit 0). No product code was changed in response to that fixture
failure.

The final focused Go command covered 13 named regression tests across
`internal/domain/security`, `internal/domain/ordinaryresult`,
`internal/app/turn`, `internal/app/loop` and `internal/server`; all five
packages exited 0, with server at 74.683 s. A second focused command covered
six foreground handoff, child steer, inline completion and case-risk
admission tests in `internal/app/subagent` and `internal/server`; both packages
exited 0. The four changed Vitest files passed 349/349 tests, `npm run
typecheck` exited 0, `gofmt -d` for the edited HTTP test was empty, and
`git diff --check` exited 0. Every test/build command sourced
`./scripts/use-analytix-cache.sh` in the same shell.

The reviewed 28-file S1 product/test slice was locally committed as
`a5dc3b0ea866cb7792dfb6cf1ab6f4d65f50887a`. The separate uncommitted
B1 oracle, user-owned development runbook edit and two 2026-09-25 QA drafts
were not staged. This is source-level evidence for S1 only. An exact-source
Core package, installed file-read/case negative journey, current-HEAD CI,
Funds positive native test, PR review and merge are still open.

## 2026-09-27 S1 explicit-assertion successor

An independent review of `a5dc3b0ea` identified two concrete false negatives:
`甲公司支付乙公司2万元。` (also `2亿元`) and
`张某与李某存在父子关系。`. The mixed prompt
`修改代码并写明当前案件甲公司支付给乙公司2万元。` could take the
software shortcut. Before changing source, focused Go security/loop tests
failed for these examples and the shared TypeScript projection failed for the
new monetary example. This was a source-level counterexample, not an installed
observation.

The Go and TypeScript classifiers now recognize the bounded magnitude-unit
payment and entity relationship forms before the ordinary software shortcut.
Negative examples retain ordinary `支付2元件`/`2万个元件` and DOM parent-child
language. ResultSlot creation and replay, a frozen Case thread's ordinary
result guard, Main/SSE/Renderer projections and a same-thread HTTP sequence
cover the corresponding boundaries. The HTTP sequence rejects each mixed
case prompt before Provider access, then completes an independent ordinary
DOM turn. These tests establish only their stated examples; the lexical
classifier is not an exhaustive natural-language authority mechanism.

At local commit `24ff5764edebf73f1769e53817cb2de57bba7a4e` (tree
`445f77403f9ac7f672523a14b8635319090b02c3`), the focused Go run for
security, ordinaryresult, turn and loop exited 0 with all four packages
passing. A separate server run passed the ordinary numeric-file HTTP public
seam, the earlier Case thread's independent file result and the mixed
software/case admission sequence (`internal/server`, 60.003 s, exit 0).
The four changed Vitest files passed 355/355 tests, `npm run typecheck`
exited 0, `gofmt -d` was empty, and `git diff --check` exited 0. Each
test/build command sourced `./scripts/use-analytix-cache.sh` in the same
shell. The initial combined Go command lost its terminal handle after the
four core package pass lines; those packages and server were subsequently
re-run separately to obtain explicit exit codes. No exact `24ff` package,
installed acceptance, current-HEAD CI, push or merge is claimed here.

A read-only Funds B1 audit corrected an earlier inference: the older
`b07eef...` package's `development_dirty_non_publishable` classification
does not itself explain the generic `native_component_host_trust_invalid`
Owner refusal. The exact native inspection failure remains unknown. The
normal full macOS development packaging route exists, but a clean current
Funds package, focused native-input admission and positive B1 composition
run remain necessary before that separate stage can advance.

## 2026-09-27 acquired-currency regression and local successor

Read-only review of `24ff5764e` found that
`甲公司取得2026年收益￥2万元。` no longer raised unbound risk: the
generic action scanner stopped at the year before finding the currency
symbol. Focused Go security and loop tests and the shared TypeScript
projection failed before the repair. The previous narrow `取得`-suffix
currency-symbol check was restored alongside the new magnitude-unit check.
ResultSlot creation/replay, frozen Case-thread output, HTTP admission,
Main/SSE and Renderer tests now include the same vector.

At local commit `c8fa02f99e7fcd2cdcd037990f3ebd06b66500e2` (tree
`812abaaf4ca947af1166e118c9470a1e08b59481`), the four focused Go
packages exited 0. Three server HTTP journeys exited 0 (`internal/server`,
53.491 s), including a mixed-case input rejected before Provider use and a
later ordinary turn. The four changed Vitest files passed 358/358 tests,
TypeScript typecheck exited 0, `gofmt -d` was empty and `git diff --check`
exited 0. All test/build commands used the required cache helper in the same
shell. This is source evidence for tested forms only. No exact `c8fa`
package, installed journey, current-HEAD CI, push, PR merge or public release
is claimed at this checkpoint.

## 2026-09-27 exact c8fa private Core package and installed boundary

The detached managed worktree at SOURCE
`c8fa02f99e7fcd2cdcd037990f3ebd06b66500e2` and tree
`812abaaf4ca947af1166e118c9470a1e08b59481` was clean under the
task-local generated/dependency exclusion file. Setup attempts failed
before a candidate: a `dist` symlink was rejected by the worktree snapshot;
the worktree lacked the ignored Computer Use native app; an `out` symlink
omitted the Office codec from the package; and a root `node_modules` symlink
was rejected by the legal material reader. These were distinct build-input
layout failures, not product-code changes or bypasses. The ignored native app
was copied only after its five files matched the tracked provenance SHA-256
values and its strict deep signature passed, then verified again in the
isolated worktree. `out` and `node_modules` became real generated/dependency
directories there; Electron's two distribution-license files matched the
tracked legal pins without symlink ancestors. The exact-source Git snapshot
remained clean with zero untracked source files.

The standard `npm run dist:mac:arm64:core` command then exited 0 with
`ANALYTIX_DIST_DIR` on the cache volume. Its embedded package authority
records SOURCE `c8fa02f99e7fcd2cdcd037990f3ebd06b66500e2`, snapshot
`eff2e0ed9c87af8ef925a99f180e8d4e4f68f96337c2ea9d25a38f64ad9f2fc4`,
`state=clean`, `dirty=false`, zero untracked source and
`development_clean_non_publishable`. The private DMG SHA-256 is
`7b5ca38cfe2e83e0ecdf4b85aa96b01e99263aa56a3f62abd13c527ff1b3cd92`;
the ZIP SHA-256 is
`4ddde724ff3ba21ff012c74c174826b3040f7e548cfad420ff8042e9a03973cd`.
The build `.app` passed independent `codesign --verify --deep --strict` and
`artifact-legal-obligations-audit.mjs --artifact` checks: status `passed`,
1,172 dependency instances, zero mandatory blockers; release authorization
remains pending. Ad hoc signing and skipped notarization do not qualify public
release.

The exact DMG passed read-only mount checksum verification. Its app was copied
to `/private/tmp/analytix-pr28-c8fa.GupXec/Applications/analytix.app` and
the image detached. Installed `app.asar`, executable, Go `runtime-server`
and package-authority SHA-256 values matched the build app. The installed
copy independently passed strict deep signature and the same 1,172-instance
legal audit. This proves the package/install integrity seam, not runtime
behavior.

An existing synthetic loopback Provider packaged-session harness ran against
this installed copy with fresh temporary HOME/user-data. It exited 1 after
`cdp_evaluation_timeout`. Its report identified exact c8fa hashes and 12
local Provider requests, but the long CDP expression returned no renderer
result. Some fork HTTP responses were observed; the harness removed its
temporary profile, leaving no durable after-state for this ambiguous send.
The expression was not replayed. It is a partial diagnostic, not a pass for
the ordinary numeric-file task, Provider recovery, or the full installed
session.

### Protected user-state incident and UI stop boundary

At approximately 03:56 PDT, after a fresh isolated c8fa app was started,
the computer-use selector was called by application name. A second Analytix
Main PID 58648, using an existing user profile, was observed starting at
03:56:06. The selector's causation is plausible but not proven. A child
migration process targeting the existing user state was briefly observed in
process metadata; no user-data contents were read. The agent incorrectly
treated the close start time as proof that PID 58648 was task-owned, sent
SIGTERM, and after observing it still running about five seconds later sent
SIGKILL to that exact PID. PID 58648 and its helpers exited. No backup,
rollback, integrity result or exact mutation inventory exists, so the effect
on protected user state is unknown. This was an unauthorized process action
and must not be presented as a clean QA shutdown.

All application-name UI selection and direct live-user-state access stopped.
The isolated c8fa PID was then stopped separately. The exact installed visible
GUI ordinary-file journey and real-Provider repeat remain **blocked at this UI
seam** until a safe precise binding for those journeys and the protected-state
response are handled by the coordinating owner. Do not retry the timed-out
mutating CDP expression or infer acceptance from the package, synthetic requests, or
source tests.

## 2026-09-27 isolated segmented synthetic retest after harness repair

Local harness commit `931667537361c597e082b6c63e5d5ec9d94e9d0d` retains a
task-created temporary profile after a sent CDP expression has an unknown
result, but classifies it as a diagnostic profile only after owned processes
are quiesced. The automatic report adds only the profile path and fixed status
fields; explicit protected diagnostic output retains its separate detail
mode. On macOS the harness verifies that its debug port belongs to its exact
spawned Main PID before CDP use. The two new targeted regression assertions
failed before repair; after repair the CDP test file passed 57/57, both
script syntax checks and `git diff --check` exited 0. A rejected renderer
promise is conservatively classed as an unknown durable result for retention,
not permission to resend the journey.

The first retest invoked the new harness from canonical HEAD `931667537`
against the older c8fa package. Artifact authority refused a mismatched
current worktree snapshot before an app launch, profile or expression; that
attempt was `LIVE_BLOCKED` and proves no installed behavior. The retest then
used the clean detached c8fa worktree as its authority context and the
committed newer harness by absolute path. The command used the existing
DMG-installed c8fa app, `--actual --actual-only --segmented-observation`,
`--expected-source-commit c8fa02f99e7fcd2cdcd037990f3ebd06b66500e2`,
and a fresh task-owned profile. The installed app authority matched c8fa;
macOS CDP-port ownership preflight passed without application-name UI
selection or direct access to live user state.

The second report at
`/Volumes/AnalytixCache/development-v3/builds/pr28-core-s1-a5dc/c8fa-installed-session-soak-segmented-931667-v2.json`
records `actualPackagedSessionSoak.passed=true` with 19/19 passed
checks: artifact, launch, renderer bridge, settings, synthetic Provider
profile, runtime restart, thread/turn, SSE, tool timeline, plan, attachment,
approval/input, fork/resume, list, redaction and process cleanup. The local
contract Provider saw 13 requests and no external Provider network was used.
No diagnostic profile was retained on this successful run. The top-level
status is `PARTIAL` with exit 1 because `--actual-only` omits the deterministic
suite; it is not a full final-gate pass. The bounded source checks recorded
above remain separate. The exact c8fa visible ordinary-file journey,
protected real-Provider repeat and user-state response remain open. The
prior ambiguous expression was never replayed.
