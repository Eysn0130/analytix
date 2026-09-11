# 2026-08-05 damaged cache predecessor retirement

Status: Operational checkpoint; cache retirement is complete, product delivery
is not complete
Applies to: configured macOS development-cache recovery and the next Analytix
construction thread
Source of truth: current Git worktree, `scripts/use-analytix-cache.sh`, current
host storage state, and fresh commands recorded below
Supersedes: the cache-capacity and predecessor-image state in
`2026-08-04-controlled-thread-checkpoint.md`

## Canonical baseline and decision

- The repository started clean on `main` at
  `c7fcd27a3e18a13c122325b19e75e0c772ea67c5`. This checkpoint is part of the
  focused retirement commit, so its final commit hash must be recorded in the
  completing thread response rather than circularly embedded here.
- `/Users/sun/Projects/analytix` remains the only canonical product source. No
  source, user state, package, or product evidence was copied from either
  retired image.
- The active image remains
  `/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle`, mounted at
  `/Volumes/AnalytixCache` with UUID
  `090478FC-CE2B-4E25-98F1-667C3252DD42`, ownership enabled, and read-write
  APFS authority.
- The v3 preservation bundle remained owner-only and byte-identical before and
  after retirement:
  `/Volumes/AnalytixCache/development-v3/evidence/controlled-thread-convergence-20260804.CxOGk7FP/preservation/controlled-thread-preservation-bundle.tar.gz`,
  SHA-256
  `972ccc841d0dd95652d04ed1dd26036b4041dc5415067f681954df52ae546a34`.
- The two predecessors were damaged development caches, not accepted source,
  current validation evidence, or future delivery inputs. The task explicitly
  authorized retaining required content and deleting content proven
  unnecessary. No separate volume had capacity for a 582 GB forensic copy;
  the damaged band payloads were therefore intentionally and irreversibly
  retired after the canonical and v3 preservation baselines were reverified.

## Retired exact targets

| Exact path | Pre-retirement allocation | Metadata observation | Result |
| --- | ---: | --- | --- |
| `/Volumes/DataSSD/analytix/AnalytixCache-v2.sparsebundle` | 532,288,384 KiB | 127,952 band-directory files including ExFAT AppleDouble entries; `Info.plist` and `Info.bckup` SHA-256 `808f9c347f8763a7200c98e6c5dfde6a2756ee5be2ac708ce3ff1589ea4beb96`; empty token | absent |
| `/Volumes/DataSSD/analytix/AnalytixCache.sparsebundle` | 36,332,928 KiB | 4,443 band-directory files including ExFAT AppleDouble entries; same Info SHA-256; empty token | absent |

Before deletion, both targets were real `0700`, UID-501 directories on the
configured DataSSD device, had zero `hdiutil` attachments, and had zero open
handles. No force deletion, glob, image attachment, repair, or alternate delete
mechanism was used. The first v2 `rm -R` was interrupted only to redirect its
large stderr stream, then the same exact target resumed with a private temporary
log below v3 `tmp`; that log was removed after completion. BSD `rm -R` removed
the real entries while ExFAT simultaneously retired their `._*` AppleDouble
companions, so it reported expected `ENOENT` lines and nonzero internal status.
Success was determined from the exact postcondition: both literal paths are
absent, not from the `rm` status alone.

## Current cache and capacity

Fresh post-retirement and post-verifier readings were:

| Measurement | Before retirement | Current |
| --- | ---: | ---: |
| DataSSD application-available | 39,583,360 KiB | 608,194,176 KiB |
| v3 sparsebundle allocation | 196,960,512 KiB | 196,967,680 KiB |
| v3 APFS container free | 93,222,084,608 bytes | 93,222,027,264 bytes |

The backing volume now has about 622.79 GB (580.02 GiB) application-available,
so the damaged predecessors are no longer the physical-capacity blocker. The
v3 container still has about 93.22 GB of logical free space; every cold build
or package must still compare both layers against its measured peak plus an
explicit margin.

The repository cache contract now permits the exact retired paths to be absent
while continuing to fail closed if either path is attached. During a bounded
transition, any still-present predecessor must be a real, detached, canonical
directory on the trusted backing device. The v3 identity, UUID, ownership,
mount flags, private namespace checks, and post-verification revalidation are
unchanged.

## Fresh verification

- `zsh -n scripts/use-analytix-cache.sh`: `PASS`.
- `node --check scripts/lib/development-cache-environment.cjs`: `PASS`.
- Final focused Vitest before and after external retirement: `2` files, `20`
  tests, all `PASS`. One intermediate post-retirement run exposed a
  test-fixture-only `ReferenceError`; the fixture was corrected and the exact
  suite reran successfully.
- Shell helper and Node volume collector with both retired paths absent:
  `PASS`.
- Canonical `scripts/verify-analytix-cache.sh`: `PASS`; `fsck_apfs` exit `0`,
  cold/warm compiler behavior, byte identity, runnable probe, ad-hoc signing,
  and strict signature verification all passed.
- Fresh receipt:
  `/Volumes/AnalytixCache/development-v3/evidence/cache-verification-v1.receipt.2IV2Nyml`,
  mode `0600`, SHA-256
  `956c7545a9d7e8097269cd6809ea1831a4d27cd558dbf3d83e88ef69b5b03fa5`.
- `com.analytix.mount-cache` was restored and reported last exit code `0`.

These checks prove the configured development-cache seam only. No Analytix
product source implementation, full product test suite, package, packaged UI,
Milestone A/B acceptance, release, or publication was performed. Resume the
approved OpenSpec reconciliation from fresh CLI state, and run A0 or B1 only in
its separately authorized thread with a new capacity preflight.
