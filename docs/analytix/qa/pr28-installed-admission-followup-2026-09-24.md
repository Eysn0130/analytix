# PR28 installed Core admission follow-up — 2026-09-24 PDT

Status: Historical evidence checkpoint for private Core SOURCE
`14ab14cb9eb28611b1fd0c526865aee01bed5f95` on macOS arm64. The later
`18d5f72f5c4c0a8d1a54e0c1ac756127c96761c5` changes one Go test expectation
only; it is not the package SOURCE. The [operational matrix](../product-completion.md)
and [handover](../handovers/README.md) own current status. Preserve the
[earlier installed continuation](pr28-installed-continuation-2026-09-24.md)
and its separate SOURCEs as historical evidence.

## Source and exact artifact

`1d34045c9` added a QA-only fixed hydration failure class. Its app-only
new-process check passed once; the earlier `9835dfc93` full private DMG had
returned 503 `accepted_final_hydration_unavailable` after two turns were
durably completed, so the inner failure class remained unknown.
`14ab14cb9` treats only an event frontier that has advanced beyond the
captured public snapshot as `public_projection_pending`. A snapshot ahead of
durable events still fails closed. The handler sends no accepted-final delivery
on the pending response. Focused application and HTTP tests pass. The existing
server interleaving test made the old snapshot fail closed but expected the old
error code; exact `14ab` CI failed that assertion. `18d5f72f5` updated only
that expectation, retaining its old-GET rejection and fresh-GET success checks.
The targeted server test passed locally in 12.5 seconds. The exact `18d` PR CI
was still running at this checkpoint. A separate local full `internal/server`
suite hit its existing 10-minute test timeout in a case-source fixture; it is
not reported as passed or as proof of a product regression.

The normal private Core lifecycle built an app, DMG and ZIP from a clean
`14ab` checkout under
`/Volumes/AnalytixCache/development-v3/builds/core-private-14ab14cb9-containers/`.
DMG SHA256:
`a8a86cbe425c8bfa82267bba5595e86ba44eb272cd66aa4473344ea13a9bc209`.
ZIP SHA256:
`c4e7ead6e237a94bc60793281018c74731bfddfeaf514d5bb8193bc3bf4ee3f8`.
The DMG was verified and copied from a read-only mount to
`/Users/sun/Applications/Analytix Core QA/14ab14cb9-dmg/analytix.app`.
The built and installed app have identical app.asar SHA256
`861270dc05943ec2c01a4e56642a0e159d1d8bbb9933003dfdec337bb9bb24fe`
and Go runtime-server SHA256
`5b28784f410a449dc359edd0cb84065957c44194f2f2f479f858610b6daaed73`.
The installed copy passed strict deep signature and independent production-tag
Core `TestConfiguredCorePackageClosure`. The postprocessor completed ZIP
signature readback and update metadata. The artifact is
`development_clean_non_publishable`, without Developer ID or notarization.

The installed app's exact legal receipt is
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/legal-installed-14ab14cb9.json`:
1,172 dependency instances, zero mandatory engineering blockers. Canvas is
absent; `@analytix/updater-lazy@1.0.5+analytix.0` is present and its package
row passes. This is package/material admission, not installed Lazy function,
GUI, signing or publication acceptance.

## Installed sessions on this DMG copy

All sessions used fresh isolated QA state and a local synthetic Provider.
No external Provider request or development credential copy occurred. The
protected receipts are in
`/Volumes/AnalytixCache/development-v3/evidence/pr28-installed-closure-20260924/`.
The session harness requires the current worktree snapshot to match the package
SOURCE. Since canonical HEAD advanced to a test-only commit, it ran with a
clean detached `14ab` temporary worktree as its working directory and the
unchanged canonical script bytes. Canonical remained the only product writer.

| Receipt | Observation and claim limit |
| --- | --- |
| `full-session-dmg-14ab14cb9-v3.json` | Artifact and renderer readiness passed. Eight local Provider requests reached the initial, tool, plan and attachment stages. The protected durable state had four completed turns and one turn aborted during cleanup. The whole 180-second observation window ended before the remaining approval/fork/resume stages; the CDP expression returned `cdp_evaluation_timeout`. This is not 19/19 or a settings/restart failure diagnosis. Cleanup quiesced. |
| `focused-cancel-dmg-14ab14cb9.json` | Pending approval, accepted interrupt, terminal replay, absence of the tool-file effect and cleanup all passed. |
| `focused-fork-initial-dmg-14ab14cb9.json` | Settings write, explicit restart, first-turn replay and one fork passed; child count changed 0→1 and cleanup quiesced. This did not reproduce the older fork400. |
| `focused-fork-approval-allow-dmg-14ab14cb9.json` | Tool timeline and attachment replay, approval denial, approval allowance, the approved tool-file effect and one later fork passed; child count changed 0→1 and cleanup quiesced. |
| `focused-fork-user-input-dmg-14ab14cb9.json` | Approval allowance, user-input request/response and terminal replay, and one later fork passed; child count changed 0→1 and cleanup quiesced. |
| `relaunch-dmg-14ab14cb9.json` | First Main quiesced; second Main read old history and created/completed a second turn. Count changed 1→2; final read and cleanup passed. The older intermittent 503 did not recur, so its exact inner cause is not established. |

Focused harness runs print aggregate `FAILED` for stages intentionally skipped
by their modes; the listed flags and durable state support only the stated
narrow claims. No sent write was replayed on disconnection. The old fork400
still lacks a safe exact response; check child state before any future retry.

## Admission decision

`SourceReady=pending` until accurate post-`18d` CI finishes.
`PrivateCandidateReady=false`: one final installed candidate has not completed
the full tool/approval/fork/resume journey, normal protected Provider setup and
recovery, 120-turn synthetic history through the installed GUI, Core upgrade
and data preservation, or installed PDF preview. Separate focused passes do not
sum into that full acceptance. `MergeReady=false`; PR28 remains Draft and the
true `main` last directly read here was
`ce96cf12581acfa0e19fae7c6aa9c709371012c8`.
`PublicMacReleaseReady=false`; Developer ID, notarization and production
publication authority are future public-distribution conditions, not a global
block on private QA or ordinary PR checks. This package is neither a public
beta nor a formal release.

## Later source and CI checkpoint

The `5c68cf6c7` Source baseline job failed one packaged QA transport test:
after the renderer reported `runtime_bridge_missing`, a final CDP handshake
consumed the remaining caller deadline and overwrote that more specific
preflight category. Product waiting and retry budgets were unchanged.
`e3d60a115` retains the observed renderer phase in this tail-timeout case and
makes the regression deterministic. On the canonical macOS checkout, both
scripts pass `node --check`, the focused soak transport suite passes 42/42,
and `npm run test:baseline` passes 101/101. GitHub Source baseline for exact
`e3d` passed; the rest of its CI was still running at this checkpoint. This
is a QA-diagnostic repair, not an additional installed-app acceptance result.
