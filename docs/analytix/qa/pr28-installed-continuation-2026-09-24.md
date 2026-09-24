# PR28 installed Core continuation — 2026-09-24 PDT, later checkpoint

Status: Historical evidence for the exact private sources below. This report
does not admit a private candidate, merge PR28, or authorize public release.
The [operational matrix](../product-completion.md) and
[handover](../handovers/README.md) own current status. The earlier
[7b checkpoint](pr28-installed-closure-2026-09-24.md) remains intact.

## Identity and scope

The canonical branch retains `b9666a5af1835a594bf335218f9ec5ac4dc66f67`
and all valid successors. Source and installed evidence must not be combined
across these separate candidates:

| SOURCE | Artifact and scope |
| --- | --- |
| `c25beaa1ee12bd47e8af1d97f04692495301e50c` | Private full app/DMG/ZIP; installed focused cancellation and fork after approval allow passed. |
| `65a91d0468ce89695aa644e5c1f8b301d8cff25b` | Latest full private app/DMG/ZIP in this checkpoint; installed focused fork and new-process recovery had mixed results. |
| `701dea32038a6447ef71198c60c73b6707ad839d` | App-only diagnostic build, not a DMG/ZIP; installed new-process recovery passed in one fresh isolated session. |

All are `development_clean_non_publishable`. The `65a` DMG SHA256 is
`04047b67fa43afbdb2194bd396b35f21719a77ba903b97173c06f2d0337c8ecb`,
ZIP SHA256 is
`710eadac2b05cc49daae0df981cf655c4d21995c25aca8af7ddd89fa7a0a1b04`,
and its app.asar SHA256 is
`4686eca497defc54ffc4a32ad23260514b72744c2d36addf081831f8dbfad51c`.
The installed DMG copy is at
`/Users/sun/Applications/Analytix Core QA/65a91d046-dmg/analytix.app`.
`hdiutil verify`, strict signature, DMG/ZIP app identity, independent
production-tag Go `TestConfiguredCorePackageClosure` on the installed copy,
and exact legal audit passed. The audit found 1,172 actual dependency
instances and zero mandatory engineering blockers. Canvas is absent; only the
replacement `@analytix/updater-lazy@1.0.5+analytix.0` is present. The legal
receipt is
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/legal-installed-65a91d046.json`.

The installed `701` app is
`/Users/sun/Applications/Analytix Core QA/701dea320-app/analytix.app`,
app.asar SHA256
`b29cf93d4304dc95febee9087c432ce073a3e262a293977f937df9cdadc7ea61`.
It passes strict signature but has no matching complete DMG/ZIP or exact
container audit in this checkpoint. Its diagnostic addition logs only fixed
status/error classes for QA thread reads; no response body, thread ID, path,
credential or prompt is emitted.

## Source repair and verification

`c25beaa1e` repaired the public TS interrupt response contract after an
installed cancellation had reached a durable aborted terminal but surfaced
HTTP 502 through the desktop bridge. A targeted regression was red before and
green after the correction. `65a91d046` added explicit public
`public_projection_pending` mapping and a bounded read-only GET retry within
the existing recovery stage. It does not replay a sent POST. `701dea320`
added the fixed-category QA diagnostic and canary redaction check. Script
42/42, adapter 106/106, root typecheck and affected ESLint passed at `701`;
the startup-probe test fixture was adjusted for scheduler load without changing
product or installed-stage time budgets. Earlier `65a` verification included
111 targeted TS, script 41/41, baseline 100/100, root/runtime typechecks,
lint and Go `TestThreadHandlersRecordErrorMapping`.

These are source/transport checks. Accurate PR28 CI for `701` was still
running at this evidence checkpoint; older green CI is not inherited.

## Installed sessions

Every run used a fresh isolated QA profile and local synthetic Provider, with
no external Provider call or development credential copying. The protected
receipts are under
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/`.
The focused harness prints full-journey `FAILED` for intentionally skipped
stages; stage flags and cleanup, not that aggregate word, decide its narrow
result.

| SOURCE / receipt | Observed result |
| --- | --- |
| `c25` / `focused-cancel-dmg-c25beaa1e-v8-180s.json` | Pending approval was explicitly interrupted; terminal became durable aborted, no tool effect occurred, and owned processes were quiesced. |
| `c25` / `focused-fork-approval-allow-dmg-c25beaa1e.json` | Approval allow and tool effect completed; one fork changed child count 0→1. |
| `65a` / `relaunch-dmg-65a91d046.json` | Settings write exceeded its existing 20 s stage. The retained isolated settings file showed the requested active Provider afterward, so write persistence occurred, but acknowledgment/synchronization timing was not localized. No turn was sent. |
| `65a` / `focused-fork-initial-dmg-65a91d046.json` | Fresh session completed settings write, explicit restart health, first turn and one fork (0→1), with cleanup. This narrows the prior timeout to an intermittent observation. |
| `65a` / `relaunch-dmg-65a91d046-v2.json` | Second Main process recovered old history and created/completed a second turn, but its final GET returned 503 `internal_error` through the desktop. The protected durable thread file showed two completed turns. The raw Go 503 class was not captured; recovery was not accepted. |
| `701` / `relaunch-app-701dea320.json` | App-only fresh session: first Main quiesced, second Main became ready, old history recovered, continuation created/completed, turn count 1→2, cleanup quiesced. The fixed-category thread-read diagnostic was empty because the prior 503 did not recur. This is one passing diagnostic session, not proof the intermittent failure is fixed. |

The earlier fork400 has no exact sanitized response and was not reproduced on
these focused sessions. Its service can create the child before a later
projection/publication failure; check child count and event state before any
retry. No fork write was blindly replayed. Settings write and runtime restart
must be diagnosed as separate stages; the observed `65a` timeout was in
settings write. The public `public_projection_pending` contract repair is
valid, but the `65a` 503 cannot be attributed to that code without a raw safe
classification. The `701` diagnostic is ready to classify a future recurrence.

## Admission boundary

The full private package is `65a`; the current source is later (`701`) and
only has an app build. A single final installed app/container has not completed
the normal protected Provider setup/recovery, tool and approval, cancellation,
120-turn synthetic long history through actual GUI, new-process recovery,
Core-to-Core upgrade/data preservation, and installed PDF preview. The 7b full
synthetic standalone journey, c25/65a focused checks, and 701 app recovery
cannot be summed into one final-candidate pass. The source 120-turn renderer
test is not installed GUI evidence. PrivateCandidateReady and MergeReady remain
`false`; PR28 remains Draft. Developer ID, notarization and publication
authority apply to future public macOS distribution, not this private QA or
ordinary PR checks. No public beta or formal release is claimed.
