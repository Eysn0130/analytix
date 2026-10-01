# Host-local expected predecessor and disk reopen checkpoint

Status: Historical source verification, 2026-09-29 PDT, macOS arm64,
Go 1.26.4. Base HEAD `7ad8351340f4dce08c0756093faf169afba8b7ef` plus the reviewed
49-file Go / 8-file active-change dependency closure. This is a partial B1
source candidate; Final, installed usability and analysis-gain acceptance remain open.

## Delivered behavior

`HostLocalSealedServiceV2.AdmitAfterExactV2` now checks the exact expected
snapshot for the same tenant/user/case/binding under the existing protected
child-writer serializer, after fresh signed head/chain resolution and before
material validation, signing or any candidate write. Malformed ids, absent or
stale predecessors, foreign scope and cancellation are refused. A competing
same-predecessor pair commits once; an old-predecessor retry is refused even
when its output already exists. No authority, CAS, durability or indeterminate
threshold was relaxed.

The real runtime owner test advances a synthetic manifest successor, checks
the exact predecessor record and Dataset-only head transition, closes the
owner and every DSV2 store, reloads file authority/access and disk stores in
Resume mode, resolves the exact successor, refuses the old snapshot/predecessor,
and derives the current query-source descriptor from the successor. Registry
and Publication child digests/counts stay unchanged. A complete relative-path
to content-hash inventory proves no dataset-store writes on stale requests.
That scan covers only `dataset-snapshot-authority`, not all other owner files.

This is production owner/store logic with synthetic materials in fresh temp
directories. It is not a cleaning transform, Context Epoch proof, process or
Electron restart, real DuckDB query, accepted Final/display journey, or live
Provider evidence. Complete-profile rollback remains `UNVERIFIED`.

## Validation

Every Go command sourced `./scripts/use-analytix-cache.sh` in the same shell.
The candidate remained stable through the final source checks.

| Seam | Result |
| --- | --- |
| New expected-predecessor regression before the code fix | exit 1; all three initial cases hit the previous fixed unavailable result |
| All `TestHostLocalSealedV2` cases | pass |
| `TestHostLocalSealedV2ExpectedPredecessor` with `-race` | pass |
| Full datasetsnapshot and fundscsvadmission packages, ordinary / `analytix_prod` | pass |
| Actual Registry CAS and restart test | pass |
| Actual owner/store successor test, ordinary / `analytix_prod` | pass; the first query-source attempt correctly refused a test manifest that omitted the original analytical binding; the fixture was corrected without changing the production guard |
| Runtime host-local profile/startup/borrow/Registry/owner-reopen selection, ordinary / `analytix_prod` | pass |
| Witnessed restart keeps host-local roots absent; production root manifest and installation-key inventory, ordinary / `analytix_prod` | pass |
| Runtime import/cleaning composition and trace/source-binding guards | pass |
| 13 changed/dependency packages listed below, full ordinary / `analytix_prod` runs | pass |
| Active OpenSpec change, `--strict --no-interactive` with telemetry disabled | pass |

The 13 package paths under `internal/` are adapters/outbound/{datasetsnapshot,
evidenceauthority,evidenceauthorityhostlocal,evidenceregistry},
app/{datasetsnapshot,evidenceregistry,fundscsvadmission,fundsquerysource},
domain/{evidence,hostcurrentness,privatecastopology,security}, and mcp.
The full runs used `-count=1 -timeout=180s`; focused runtime selections used
`-count=1 -timeout=120s`. Source assertions and acceptance thresholds were retained.

| Task-owned changed source | SHA-256 |
| --- | --- |
| `app/datasetsnapshot/host_local_sealed_v2.go` | `c0643fa964cf56d57f363fe5cb105cebcf5d12171071687e7af2914f613ac77a` |
| `app/datasetsnapshot/host_local_expected_predecessor_v2_test.go` | `408331eb00bdd86cd5e422f3a9dd182f0911900a6e87d2fdaf803a24c03938fa` |
| `runtimeapp/host_local_expected_predecessor_unix_test.go` | `68d8d4c05145f347507805821967c95ddf4f94dc3447ac04dd5bd90afdc7133f` |

The source paths above are relative to `packages/runtime-go/internal/`.
The inherited dependency closure supplies the separately typed host-local
mode/head, DSV2 and Registry grammars, current-selection capabilities,
startup/recovery routing and unchanged witnessed compatibility. Its exact
57-path source closure fingerprint is
`d53e484554f54f08addb70bd001a6ade22edd7866542db21691aefa7ddffdece`
(ordered path, NUL, SHA-256 and LF). Integration is identified by the focused
local commit containing this checkpoint; archive verification remains next. These passing worktree runs
alone do not assert source-isolated build, remote CI,
merge, installed or release readiness. Continue from the
[canonical handover](../handovers/README.md).
