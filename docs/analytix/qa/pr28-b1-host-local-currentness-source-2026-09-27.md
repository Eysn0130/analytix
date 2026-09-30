# PR28 B1 host-local currentness source checkpoint — 2026-09-27 to 2026-09-28

Status: **partial source candidate, not installed B1 acceptance**.

## Candidate and observed gap

- Canonical branch: `codex/workbench-product-delivery-20260914`; base HEAD
  `c2cd3e29174e701569220e1c39076e0acff24a53`, tree
  `55893ead1fc3a6bce73ede22a251005a14810532`.
- This checkpoint's OpenSpec and Go edits are uncommitted. The user-owned
  `docs/analytix/development-runbook.md` modification and two untracked QA
  notes present before this work were preserved and are outside this slice.
- At the start of this work, `runtimeapp/app.go` composed Funds CSV admission
  only when native owner, shared evidence authority, and DSV2 snapshot owner
  were all present. Absent protected witness enrollment left shared evidence
  uncomposed. The current uncommitted source candidate adds a distinct
  host-local composition for eligible empty-lineage installations.
  The older installed `0ff97f7` synthetic B run returned
  `funds_import_capability_unavailable`; it is not evidence about a new
  packaged candidate. The c2cd installed startup/entry trace remains open.

## Bounded source change

- Active `case-evidence-publication-gate` design, architecture, threat,
  migration, rollback, and delta requirements now distinguish an empty-lineage
  `host_local` currentness profile from an already enrolled witnessed profile.
  Whole-profile rollback stays `UNVERIFIED`; invalid or mixed legacy state
  remains boundary-only. Strict OpenSpec validation passed.
- `domain/hostcurrentness/HeadV1` is an installation-key-signed, closed
  `host_local` record. Generation zero is the mode commitment. Each successor
  advances exactly one existing DSV2, registry, or publication child root.
- `evidenceauthorityhostlocal.HostLocalStore` persists immutable signed head history
  and one exact-CAS selector; startup/current reads validate the complete
  chain and reject every orphan. The selector CAS is the local commit point.
  A losing signed candidate remains a quarantine event because it is
  indistinguishable from selector rollback without stronger settlement proof.
  An authenticated recovery plan for this event is **not implemented**.
- The initial in-package store passed its focused tests but failed the full
  witnessed adapter architecture test, which forbids local current selection.
  Moving the host profile into its own adapter package kept that existing gate
  unchanged. The new package's architecture test forbids directory-order
  selection and automatic residue reconciliation.
- The current source candidate composes the host-local owner in `runtimeapp`
  without creating a mode head during ordinary Core startup or read-only
  lookup. A valid Funds import reaches `EnsureEvidenceCurrent` after exact case
  binding validation; only that callback may admit a fresh empty lineage and
  commit generation zero. Existing committed mode is opened read-only on
  restart. A callback-spanning owner borrow lets DSV2 and Registry use the
  same CAS handles without holding the owner mutex through nested calls.
- The host-local DSV2 service admits an exact signed dataset generation, and
  the V3 Registry service commits a prepared receipt through its actual CAS.
  Query and local-display source selectors accept the closed host-local
  selection shape. V1 witnessed encodings and the old witnessed adapter remain
  unchanged. The source candidate still lacks host-local accepted-Final
  issuance/recovery and installed positive B1 proof. Cleaning's expected
  predecessor host-local admission is also unavailable.

## Verification and limits

| Check | Result | Scope |
| --- | --- | --- |
| `go test ./internal/domain/hostcurrentness` | PASS | Signed mode/head chain, strict parsing, key/root binding. |
| `go test ./internal/adapters/outbound/evidenceauthorityhostlocal -run TestHostLocalStore -count=1` | PASS as part of complete package run | Empty mode, sequential writes, restart readback, stale expected head, prepared orphan, and CAS loser quarantine. The test was first run before the store existed and failed to compile on missing symbols, then passed after implementation. |
| `go test ./internal/adapters/outbound/evidenceauthority ./internal/adapters/outbound/evidenceauthorityhostlocal -count=1` | PASS | Preserved witnessed no-local-selection architecture gate and new host profile tests. |
| `openspec validate case-evidence-publication-gate --strict` | PASS | Planning artifact syntax/structure; it does not prove implementation. |
| `go test ./internal/runtimeapp -run 'TestRuntimeHostLocal\|TestFundsCSVAdmissionTrace\|TestFundsImportAndCleaningProductionComposition' -count=1` | PASS | Fresh ordinary Core does not create mode roots; explicit Funds effect commits and resumes an empty mode; nested owner borrow and source composition. |
| `go test ./internal/runtimeapp -run '^TestRuntimeHostLocalPreparedReceiptUsesActualRegistryCASAndRestarts$' -count=1 -timeout=60s` | PASS | Synthetic exact DSV2 admission, actual prepared Registry CAS, receipt readback, and same signed head after restart. No native/installed or Final proof. |

Source microbenchmark on Apple M4 Pro, arm64, 24 GiB RAM, Go 1.26.4, with
`use-analytix-cache.sh` active and test temp files under
`/Volumes/AnalytixCache/development-v3/tmp`; `-benchtime=3x -count=1`, warm
compiler cache. Command from `packages/runtime-go`:

```bash
go test ./internal/adapters/outbound/evidenceauthorityhostlocal -run '^$' \
  -bench '^BenchmarkHostLocalStoreCurrent$' -benchtime=3x -count=1
```

| Signed heads | `Current` mean per operation |
| ---: | ---: |
| 1 | 2.90 ms |
| 10 | 12.04 ms |
| 100 | 78.18 ms |

The source read cost grows with complete immutable inventory size. Three
measurements per size are diagnostic only; these figures are neither an N03
installed UI latency result nor a production history-size limit. Every
protected-effect read remains conservative until a separately verified faster
path preserves orphan/rollback refusal. Ordinary Core may read a committed
profile on restart, but startup does not create its mode commitment.

N06/B16 remain open: host-local Final admission/recovery, the complete
installed Funds import/query/Final/display/restart journey, and
crash/ambiguous-commit recovery are not verified in a packaged process.
No credentialed Provider, real user profile, package, push, merge, or release
step was used for this checkpoint.
