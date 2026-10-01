# PR28 credential continuity on the e15 installed Core — 2026-09-26 PDT

Status: Historical QA checkpoint for product SOURCE
`96294a3142ffe08f63ee9d367ce1e36c2b760473`. This applies the
user-supplied `Analytix-Credential-Authority-Appendix-2026-09-25.md` as task
direction; its research anchor is not a Git reset target. Accepted repository
requirements and actual code remain the product authority.

## Exact candidate and scope

- Branch: `codex/workbench-product-delivery-20260914`; PR
  [#28](https://github.com/Eysn0130/analytix/pull/28) is Draft/open. Product
  SOURCE is `96294a314`. Directly read `origin/main` was `ce96cf125` and
  local `main` was `60839b721` at this checkpoint. All 57 PR checks at the
  product SOURCE completed successfully, including the native Windows
  credential job; the review decision was empty.
- A clean detached checkout of `96294a314` built
  `analytix-core-1.0.6-mac-arm64.dmg` and ZIP with Electron 41.10.3. DMG
  SHA-256: `4a71c539aaeaec0fc419e662a1bba03a2b5f8e55f390821a5395dbe66f456aaf`.
  ZIP SHA-256: `5777de92d64808e471813d4eb12cd7ca6dd58d4a81eeb881f017ae50ba6637b4`.
  The DMG verified; the installed app passed strict/deep ad hoc code-signature
  checks. An exact installed-app legal audit passed with 1,172 package
  instances and zero artifact admission blockers; its private receipt is
  `e15-96294-artifact-legal.json` in the evidence directory below. The
  installed app came from that DMG. Host: macOS 26.5.2 arm64. This is
  `development_clean_non_publishable`, not a public release.
- The test Provider was an isolated loopback synthetic service. Its input and
  three task-owned profiles were cleaned only after the bounded acceptance;
  neither the persistent development authority nor user production state was
  copied or removed. The key-free private receipt is
  `/Volumes/AnalytixCache/development-v3/evidence/pr28-e2c60aea6-installed/e15-96294-installed-credential-acceptance.json`.

## Installed observations

| Seam | Exact observation | Result |
| --- | --- | --- |
| Fresh visible Save | e15 first-run Settings saved a synthetic key through the normal UI; a local turn returned `LOCAL QA OK`. Reopened Settings showed the selected Registry Provider and “凭据已保存，重启后会自动复用；仅更换时需要重新填写”; the key field was empty/masked. | Pass for installed synthetic C01. |
| Same profile recovery | The installed app answered after Go restart and after a new Main process with the original Registry/ciphertext/master hashes unchanged. | Pass for installed synthetic C02. |
| Long conversation | One e15 installed GUI thread completed 120 consecutive unique synthetic turns. After another new Main, the GUI read back 120 and sent turn 121 successfully. An older e10 thread read back 127 turns on e15 and sent turn 128. | Pass for the synthetic C18 history and continuation seam. |
| Same-version rejected authority | With only the isolated profile's nonsecret authority marker changed to an unsupported value, e15 showed “原配置已保留” and did not run the saved Provider. Registry, ciphertext, master, settings and PDF hashes remained equal to baseline. Restoring the marker allowed the old turn to be read and a second turn to complete; all six files matched baseline. | Pass for this fail-closed/recovery fixture. |
| Actual e13→e15 negative upgrade | The e13 installed app (`492f4c8e6`) saved through visible Settings and completed one turn. Its profile and synthetic PDF were retained. The e15 DMG installed copy opened the same profile with an unsupported authority marker and showed the retained-config unavailable state. Five business files were byte-identical to e13. After exact marker restoration, e15 read the old turn, completed turn two as `LOCAL QA OK`, and Registry, ciphertext, master, authority, settings and PDF all matched e13 baseline hashes. No re-entry or Keychain password was used. | Pass for the bounded synthetic rejected-upgrade and restored-upgrade C18 seam. |
| PDF | The e15 installed GUI opened a two-page synthetic PDF; page navigation, 115→125% zoom, second-page search and reopen worked. | Pass for this fixture only. |
| Real approved development authority | At this product SOURCE, a new source Core process used the existing protected development Registry/Secret Store through `provider verify` with `deepseek-flash`; it reported configured, resolved and reachable, using 40 prompt and 17 completion tokens. Its separate 0600 receipt is `development-provider-current-96294a314.json` in the same evidence directory. | Pass for bounded source Core reuse; it is not packaged real-Provider recovery. |

## Appendix C01–C18 disposition

| IDs | Evidence boundary |
| --- | --- |
| C01–C02 | Installed synthetic UI Save, reopened projection, Go/Main restart and continuation pass. |
| C03 | Existing persistent development authority survived this source/build/worktree continuation and a new Core verification process. Deliberate cache deletion was not performed. |
| C04–C07 | Existing Go/desktop source regressions and exact-head CI pass for Keep/Set/Delete, revision/transaction, initialization and damaged authority paths. This checkpoint adds installed rejection/preservation, but does not claim each failure mode was exercised in the installed GUI. |
| C08 | Normal macOS source path and e15 fuse/partition checks pass; no Keychain prompt was observed in the installed synthetic runs. There is no OS-level trace proving zero Security API access. |
| C09 | The Windows 2022 CI job passes native CurrentUser DPAPI restart, missing-key, ACL, private replacement and Registry tests. A distinct wrong-user Windows login was not exercised. |
| C10 | Windows CI passes `dev-launcher-windows.test.mjs` against the launch contract. A Windows installed desktop product was not run. |
| C11–C12 | Source legacy/file-authority and partition tests pass; the e13→e15 synthetic profile preserves thread, settings and PDF. This is not a live legacy Keychain-only user's manual re-entry or a real site's encrypted-Cookie migration test. |
| C13–C15 | Focused source contracts and exact-head CI pass in their covered cases; the exact installed-app legal audit also passes. This installed run used a synthetic Provider and does not establish every external network failure, tool/export leakage or remote endpoint case end to end. |
| C16 | An unrestricted same-UID shell could hash the synthetic profile's master file; this mode has no proven same-user shell isolation. No Windows PowerShell product execution was observed. |
| C17 | Approved protected real Provider reuse passed through a separate new source Core process and receipt. No approved protected real-Provider handle existed for the packaged QA app, so installed real-Provider save/restart remains blocked without copying a key. |
| C18 | Exact e15 synthetic long-session, restart, positive upgrade and rejected-upgrade/restore observations pass. Original private-candidate admission remains partial because packaged protected real-Provider recovery is absent. |

The master key and ciphertext in the same macOS user's readable private tree
do not isolate an arbitrary process running under that UID. Windows CurrentUser
DPAPI also does not by itself isolate every same-user process. No claim here
substitutes cross-compilation for Windows native execution or a green CI job
for Windows installed-product acceptance.

## Admission and next dependency

`SourceReady=true` for exact product SOURCE `96294a314` and its 57/57 checks.
`PrivateCandidateReady=false`: protected real-Provider recovery in the packaged
app remains unverified; the existing development authority cannot be copied
into the isolated packaged profile. `MergeReady=false`: PR28 stays Draft, with
that product gate open. `PublicMacReleaseReady=false`: this artifact is ad hoc
and nonpublishable. Windows installed desktop acceptance and an OS-level
Keychain-access trace have their separate evidence limits above.

The next dependent action is a normal protected packaged UI save/restart with
an already authorized, available QA credential handle, if one becomes
available under the existing budget. Do not ask for or copy the development
key, infer success from the synthetic run, or weaken the package's isolation.
No product bytes changed during this QA checkpoint; do not rebuild e15 merely
to refresh documentation.
