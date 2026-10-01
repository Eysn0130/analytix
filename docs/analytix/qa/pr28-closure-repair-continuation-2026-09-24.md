# PR28 closure repair and installed Core continuation — 2026-09-24 PDT

Status: Historical QA checkpoint for product SOURCE
`14ab14cb9eb28611b1fd0c526865aee01bed5f95` and QA harness
`4c1f98d97621733fffd226834a8bb217fac915a4`. Current branch, CI and
admission decisions must be read afresh from GitHub and the canonical checkout.
The installed app is
`/Users/sun/Applications/Analytix Core QA/14ab14cb9-dmg/analytix.app`.
Its DMG, ZIP, app.asar and Go runtime hashes remain those in
[the artifact checkpoint](pr28-installed-admission-followup-2026-09-24.md).
Classification remains `development_clean_non_publishable`.

## Repair and source evidence

The attached ChatGPT repair packet was treated as untrusted review input.
All 13 regular ZIP entries passed its `SHA256SUMS.json` check before use. The
original packet's 46 source-fragment assertions had 12 passes and 34 expected
failures against `433fa3d93`; its applicator's eight transformation tests
are not repository or installation acceptance. The three fixes were integrated
in the original QA owner: failed prerequisites stop later writes, explicit
protected diagnostic retention keeps a quiesced profile, and a successful fork
requires exactly one new child. The packet's 46 assertions then passed at the
first integration. Subsequent adjacent QA work added a single mutating start
and token-bound read-only stage observation, retained the original renderer
readiness budget, and checked that the parent turn list is unchanged across
the full-session fork. Packet fragment anchors are frozen to their earlier
source shape; five now reject the intentionally revised fork block. The real
owner's 52 focused cases and root baseline's 111 cases pass at `4c1f98d97`.

Focused commits normally pushed to PR28 are `415f4d690` (three repair
conditions), `aa0e9c2da` (stage observation), `d60e90811` (renderer startup
budget), and `4c1f98d97` (parent history). The latter changes only QA script
and tests after the 14ab product build. No post-14ab product or package input
change is claimed. Exact `4c1f98d97` CI was still running at this checkpoint;
older green CI does not transfer.

## Installed evidence on the exact 14ab DMG copy

The original repaired full run still reached the user-input gate near the
180-second total CDP evaluation deadline and returned an unknown evaluation
outcome. A token-bound read-only progress snapshot observed that stage; it did
not resend the mutating journey. The segmented observation owner kept the
product request and SSE deadlines unchanged. Its complete journey then passed
all 19 actual installed checks on the same installed DMG copy. Repeating after
the parent-history assertion at `4c1f98d97` again passed 19/19: one dependent
sequence covers settings, restart, ordinary and tool turns, plan, attachment,
approval deny and allow, user input, fork continuation, resume continuation,
list/search, usage, terminal events and process cleanup. The fork child count
was 0→1 and the parent's public turn list matched before and after fork.
Thirteen Provider requests stayed on the isolated loopback synthetic Provider;
no external Provider network call occurred. The receipt is
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/full-session-parent-history-4c1f98d97.json`.
Its top-level `status=partial` and CLI exit 1 reflect `--actual-only` skipping
deterministic contract groups; `actualPackagedSessionSoak.status=passed` and
19/19 actual checks are the installed result. The separate focused cancellation
receipt for the same SOURCE is `focused-cancel-dmg-14ab14cb9.json`; it does not
replace the continuous journey.

The old intermittent fork400 and accepted-final GET 503 did not reproduce in
this installed run. Their earlier precise inner causes remain unproved; neither
is relabeled as fixed by the QA harness changes.

## Remaining admission seams

| Seam | Current observation | Admission result |
| --- | --- | --- |
| Normal protected Provider setup and restart recovery | The earlier 8e installed QA profile had a stored DeepSeek credential and an isolated task Keychain. This 14ab run used only a synthetic loopback credential. No development credential, Keychain password or API key was copied or requested. | Not run on 14ab; no current real-Provider acceptance. |
| 120-turn installed GUI | A 120-turn source renderer fixture exists, but this run did not generate 120 turns through installed Core or inspect its actual GUI. | Not run. |
| Core-to-Core upgrade and data preservation | The existing installed 8e and 14ab app authorities were read and a new isolated profile started each app in sequence. The old process did not yield a session write/readback fixture before replacement; no state digest or interruption recovery was obtained. | Not verified. |
| Installed PDF preview | A two-page synthetic PDF was generated and both pages rendered legibly as a QA fixture. A fresh isolated 14ab Main and Go process started, but macOS Computer Use app binding returned `timeoutReached` before a window tree or page interaction could be read. The task-owned process was stopped. | Not verified in the installed GUI. |

The protected GUI runner receipts are `installed-gui-runner.json` and
`installed-upgrade-runner.json` beside the full-session receipt. They establish
process/profile identity and cleanup only; they are not GUI or data-preservation
passes. The direct relaunch of a previously retained synthetic profile used a
login Keychain already retired by its QA owner and reached a startup alert;
that attempt is not a product-code regression or a valid restored-profile test.

Accordingly, `PrivateCandidateReady=false`, `MergeReady=false` and
`PublicMacReleaseReady=false`. `SourceReady` depends on the exact final HEAD
CI. PR28 remains Draft and remote `main` still requires a direct read before
any future merge claim. Developer ID, notarization and publication authority
belong to future public macOS distribution, not to this private QA repair.
