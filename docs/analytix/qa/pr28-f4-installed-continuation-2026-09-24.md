# PR28 private Core installed continuation — 2026-09-24 PDT

Status: Operational QA checkpoint for product and harness SOURCE
`f4cf3ef14ab6bd8375830310ea8c5220e46793b2`. This records bounded
private acceptance evidence, not a product or public release decision. The
older [closure repair checkpoint](pr28-closure-repair-continuation-2026-09-24.md)
remains historical for SOURCE `14ab14cb9` and harness `4c1f98d97`.

## Candidate and source checks

The attached ChatGPT review packet remained untrusted input. Its three QA
findings were integrated in the original packaged-session owner: a failed
prerequisite stops subsequent writes, explicit retention preserves a protected
diagnostic profile after quiescence, and fork success requires exactly one new
child. The owner also uses one mutating start with token-bound read-only stage
observation and checks that the parent turn list stays unchanged. Later Go
source commits `f5fd97cfa` and `f4cf3ef14` repair long-history public
projection and batch terminal usage settlement during startup recovery.
Focused Go tests, the root baseline's 111 cases, and a protected source
30-turn/restart run passed on this SOURCE. The exact
[Development CI](https://github.com/Eysn0130/analytix/actions/runs/36097798754)
passed 51/51 jobs; PR28 reported 56/56 successful checks at this HEAD. These
source checks do not substitute for installed acceptance.

The private Core app/DMG/ZIP was built from clean `f4cf3ef14` source using
the normal Core packaging command. The installed QA app came from that DMG,
not a source dev server. DMG SHA-256 is
`e6cc40a776e2e4e76c338c6ddfed8c1fce8fe2152cd16d533c04623eb46992ac`;
ZIP SHA-256 is
`caff481474021f8c6a060fa8b56c2888a65375818c6d7bffd21bbade7d8ef877`.
DMG verification, strict installed-copy signature verification, independent
production-tag Core Go inspection, and the exact artifact legal audit passed.
The audit covered 1,172 dependency instances with zero mandatory engineering
blockers. The signature is ad hoc; classification stays
`development_clean_non_publishable`, with publishable and release-eligible
false. The installed app path is
`/Users/sun/Applications/Analytix Core QA/f4cf3ef14-dmg/analytix.app`.

Evidence root:
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/`.
Receipts there bind the app, SOURCE, profile, process and fixture. The two
isolated profiles used for GUI and upgrade work remain preserved for review;
neither is user-owned production state. QA runners stopped their owned apps
and disposed of task Keychain handles.

## Installed acceptance seams

| Seam | Observation on `f4cf3ef14` | Result |
| --- | --- | --- |
| A. One continuous Core journey | `full-session-f4cf3ef14.json`: 19/19 actual installed synthetic-Provider checks passed on one DMG copy, including settings/restart, tool and approval gates, user input, one fork, resume, list/search, usage and cleanup. Top-level `partial` only reflects `--actual-only` skipping deterministic groups; `actualPackagedSessionSoak.status=passed`. | Pass for this synthetic installed journey. |
| B. Cancellation | `focused-cancel-f4cf3ef14.json`: a pending approval was interrupted, terminal replay matched, the intended tool file effect did not occur and cleanup quiesced. Focused mode intentionally does not run the full journey. | Pass for focused installed cancellation. |
| C. Normal protected Provider recovery | No real Provider credential was copied into the synthetic profile. The earlier real-Provider QA authority was retired with its task Keychain; no new approved unlocked authority was available for this candidate. | Blocked. No normal real-Provider setup/restart claim. |
| D. 120-turn installed GUI | `long-history-installed-resume-f4cf3ef14.json` records 120/120 completed, ordered unique local synthetic turns. `long-history-installed-gui-f4cf3ef14.json` and early/middle/late screenshots show the installed GUI's 120-turn label, rendered beginning/middle/end and switch-away/back. A new Main-process recovery was not run. Unique GUI turns 121 and 122 each ended `provider_error`, without replay. A protected Registry probe returned `credential_unavailable` after a Go restart; the replacement loopback fixture received zero requests. | Partial. Rendering passes; post-120 continuation and new Main recovery do not. The credential lifecycle is a lead, not a proved product root cause. |
| E. Core upgrade and data preservation | `upgrade-fixture-f5-to-f4.json` records old installed `f5fd97cfa` writing and reading one completed local synthetic thread, theme, Provider Registry metadata and PDF hash in a fresh isolated profile. Its runner stopped the old Main and launched the `f4cf3ef14` DMG copy in that same profile. The new Main stayed in `SecItemCopyMatching` while macOS `SecurityAgent` was active; no Go process or CDP page became available. The app was stopped without an after-upgrade read or write. | Partial: old-state fixture exists; new-version data preservation and continuation remain unverified. No password was entered or requested from the user. |
| F. Installed PDF preview | `pdf-installed-gui-f4cf3ef14.json` and two screenshots show an installed GUI opening a two-page generated PDF from Files; both pages rendered, next-page changed 1→2, zoom changed 115→125%, search found one second-page match, and close/reopen returned to page one before navigating to page two again. | Pass for this two-page synthetic PDF fixture only. |

The GUI's normal onboarding Save showed “The provider registry returned an
invalid response” in the long-history synthetic profile. A single Settings
bridge alignment had an unknown CDP acknowledgement; durable settings readback
showed the selected local Provider. The two later `provider_error` turns were
not resent. This is a separate observable limitation of that QA session; it
does not prove data loss or a production Provider failure. The old fork400 and
intermittent 503 were not reproduced on this candidate, so their exact earlier
causes remain unproved.

A read-only capture of the task-owned SecurityAgent window is retained as
`upgrade-new-app-keychain-prompt-f4.png` (SHA-256
`bee8e9e758abd102751e7255c6453a54a09a2e6e376d859533e538887ba1dcc0`).
It explicitly asks for the `login` Keychain password before the new app may use
the `Analytix Safe Storage` item. The runner had rebound an isolated QA login
Keychain; the prompt was denied and closed after the app stopped. Both private
apps are ad hoc signed with the same bundle identifier but different code
directory hashes. A changed access control identity is a plausible cause of
the prompt, not a proved sole root cause. No Keychain item or access policy was
modified to bypass it.

## Admission and next dependency

At exact source HEAD `f4cf3ef14`, `SourceReady=true` for its completed source
checks. `PrivateCandidateReady=false`: C, D continuation/new-process recovery,
and E after-upgrade read/write still need valid installed evidence on the same
candidate. `MergeReady=false` and `PublicMacReleaseReady=false`. PR28 is still
Draft and `main` was directly read as
`ce96cf12581acfa0e19fae7c6aa9c709371012c8` at this checkpoint. A later
documentation commit requires its own accurate-head check before any source
readiness claim transfers. GitHub mergeability is not product acceptance.

Resume C only with an already approved, available protected QA credential
authority; do not copy keys or ask for the same key/password again. For D,
first establish the isolated Keychain/Go restart state, then observe one unique
GUI continuation and a new Main-process readback without replay. For E,
resolve the specific macOS Keychain access boundary for the old→new app in a
new task-owned isolated run; compare the pre/post thread, settings, Registry
and PDF, then verify one new completed turn. A separate negative upgrade case
is still required before claiming interrupted/rejected-upgrade data safety.
These are bounded observations, not a request to extend one long expression or
to increase product deadlines. Developer ID, notarization and publication
infrastructure remain future public-distribution work.
