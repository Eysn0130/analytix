# PR28 installed Core continuation — 2026-09-24 PDT

Status: Historical evidence for the exact private SOURCE
`7b9cb2b4f797c2e22b9cc62fb507a248e03981da` on macOS arm64. This report
does not grant private-candidate, merge or public-release acceptance. The
[operational matrix](../product-completion.md) and
[handover](../handovers/README.md) own current status after this snapshot.

## Source and handoff integration

The canonical branch retained `b9666a5af1835a594bf335218f9ec5ac4dc66f67`
and all later commits. The 2026-09-23 handoff ZIP was checked for unsafe paths,
links, duplicates, CRC and declared hashes before extraction. Its patch was
applied by hunk to the original source, not by replacing whole files. The
following focused commits were made and normally pushed on PR28:

| Commit | Change |
| --- | --- |
| `7d61b4904` | Bound CDP connection/evaluation, avoid post-send write replay, use bounded settings/restart phases, preserve SSE success requirements and cleanup. |
| `867debc8b` | Retain protected failure identity, profile, PID and fork-count diagnostics. |
| `99b054303` | Wait for a healthy installed renderer before session evaluation. |
| `c7b893dad` | Add a single-turn installed fork isolation route. |
| `31783ad95` | Isolate the first failing completed-turn stage. |
| `7b9cb2b4f` | Carry already acknowledged gate events into terminal replay; approval-deny regression was red before and green after. |

The extracted package's 28 function tests and three transport tests were only
narrow Linux handoff evidence. The canonical Mac ran the script syntax check,
targeted regression tests (33/33 at the final source), baseline (92/92),
related lint and existing packaged-session Go contracts. A separate packaging
configuration test timed out; its deadline was not increased, and it is not
treated as an installed acceptance result. PR28's exact `7b9cb2b4f` check
rollup later completed with 56/56 success, including Development gate and
CodeQL. These checks establish source status only.

## Exact private artifact

The source was clean when the Core package was built. The standalone app and
the DMG/ZIP have the same `app.asar` SHA256
`f726b1227c25e46aaab043083da0540eb46c7f002e6a822447fa7853aca12770`
and Go runtime-server SHA256
`b42418e8f5de63901f9e5302b9f38eb594f92343610127b29602cd540f344d1e`.
The private DMG SHA256 is
`3dad81f6b5f932951e792b3ca6be8db11890ce2f3c7ecc91de13dab7d9c50994`;
the private ZIP SHA256 is
`616198d38990bd3ffe06f4a4275fc2fb794e32223968b4a9077911934292659e`.
They are under
`/Volumes/AnalytixCache/development-v3/builds/core-private-7b9cb2b4f-containers/`.
The DMG's mounted app, ZIP app and isolated installed copy match the built app
across 10,524 manifest entries, including 8,607 regular files and 33 links;
file hashes, modes and link targets have zero mismatches. `hdiutil verify`, ZIP
CRC and strict deep signature checks passed; the read-only mount was detached.

The installed DMG copy at
`/Users/sun/Applications/Analytix Core QA/7b9cb2b4f-dmg/analytix.app`
passed independent production-tag Go `TestConfiguredCorePackageClosure` in
2.41 s. Its exact artifact legal audit passed: 1,172 actual dependency
instances, zero mandatory engineering blockers. The Canvas package and old
`lazy-val` are absent; `@analytix/updater-lazy` is present. The replacement's
four source tests pass. The PDF text and read-only preview-state source tests
pass 17/17, but the installed PDF renderer preview was not observed. Legal
admission does not decide signing, notarization, publication or release. The
artifact remains `development_clean_non_publishable`.

## Installed behavior and error localization

All sessions below used fresh isolated QA state and a local synthetic Provider,
with zero external Provider calls and no ordinary-development secret copying.
Each run tracked the app SOURCE, app/Go hashes, Main/Go PID, launch time,
profile, workspace, request shapes and cleanup. Protected raw receipts are in
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260923/`;
they must not be promoted into a public secret-bearing transcript.

| Session | Result and limit |
| --- | --- |
| Standalone installed app, full synthetic journey | 19/19 actual checks pass: settings write, runtime restart, conversation, tool effect, plan, attachment, approval deny/allow, user input, fork and next turn, resume and next turn, list, usage and redaction. The overall report is `partial` only because `--actual-only` skipped source-contract groups. Receipt `full-session-7b9cb2b4f.json`. |
| DMG installed copy, equivalent full journey | The overall 180 s observation budget ended during approval allow. Main/Go were quiesced by cleanup. Durable events show a completed denial and a newly started approval-allow turn then `turn_aborted(cancel)`; no fork request was sent. This does not reproduce fork400 or establish a settings/restart failure. The original failure and isolated profile are retained in `full-session-dmg-7b9cb2b4f.json`. |
| DMG installed copy, approval-allow then one fork | In a fresh bounded stage test, settings/restart, text/tool/attachment, approval denial and approval allow complete. The authorized tool effect is present; a single fork succeeds and child count changes 0→1. The stage intentionally skips MiMo, user input, fork continuation and resume, so its overall full-journey gate is false by design. Receipt `focused-fork-approval-allow-dmg-bound-7b9cb2b4f.json`. |

The earlier installed `fork400` response itself was not recovered with an exact
safe error category. The current Go `HandleFork` maps most service errors to
HTTP 400, and `Service.Fork` records the create before attempting event
publication and public projection. A future failed response must inspect
durable child count and event status before deciding whether a retry would
create another child. No service mutation was made without a reproducible
failure phase. The latest installed checks did not observe `fork400`.

The long DMG run's timeout is a whole-run observation limit, not evidence
that settings or runtime restart hung: prior stages and later focused stages
completed. The cleanup canceled one in-flight turn. That narrow observation
does not replace explicit installed cancellation acceptance, including no
late tool effects or recovery replay.

An additional isolated DMG-copy GUI process was launched under a fresh QA
home to inspect PDF preview. The Computer Use app binding returned
`timeoutReached` twice before a window state could be read. No PDF interaction
was performed, so no installed preview result is claimed. The task-owned
app process was stopped, its temporary login Keychain disposed, the user
search list still contained only `login.keychain-db`, and the QA profile was
retained for diagnosis. This is an automation-observation gap, not proof of
an application preview failure.

## Admission at this checkpoint

| Exit | Status | Remaining exact seam |
| --- | --- | --- |
| SourceReady | `true` for `7b9cb2b4f` checks | Later HEADs require their own check result. |
| PrivateCandidateReady | `false` | One final installed candidate still needs normal Provider setup, explicit cancellation, 120-turn real GUI, new-process recovery, Core upgrade/interruption/data preservation and installed PDF preview. The DMG full journey did not finish. |
| MergeReady | `false` | PR28 is Draft and product acceptance is incomplete. Green CI alone is insufficient. |
| PublicMacReleaseReady | `false` | The package is private and nonpublishable; Developer ID, notarization and production publication authority are separate future public-release requirements. |

No public beta/stable or formal release is claimed. The prior 21/2 package
license rows, old `NOT_CLEARED` outbound state and older failing CI are
historical for this candidate. Private admission and PR merge must follow
the remaining real product evidence, not repeated source tests or longer
timeouts.
