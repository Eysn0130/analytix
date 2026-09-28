# PR28 N03 `7dc07bb` installed diagnostic

Status: **pass** for one bounded, private installed-product K10 journey on
2026-09-27 PDT. This is a Historical exact-candidate record, not admission of
the later branch HEAD, a general latency improvement, formal Funds acceptance,
or public release. The continuation ledger is
[PR28 N01–N09](pr28-next-execution-2026-09-26.md).

## Identity and environment

| Item | Exact observation |
| --- | --- |
| Product SOURCE / tree | `7dc07bb0973e2c5e6a5e5c9235bd585c9c3a1495` / `5028abd6056f25825f93943edeb60fffee31e5e8` |
| Private full arm64 DMG | `/Volumes/AnalytixCache/development-v3/builds/pr28-full-7dc-n03/dist/analytix-1.0.6-mac-arm64.dmg`, SHA-256 `886e91cf2d8a5107663b800d98c5863a6c02c0d628397c485684a4d71154adec` |
| Installed copy | `/Volumes/AnalytixCache/development-v3/builds/pr28-full-7dc-n03/installed/analytix.app`; embedded authority records the same SOURCE |
| Installed Main / Go / app.asar SHA-256 | `3d2ffdeaff6b0bbf52beee5333ee3345fcad5d7722d019eefe4c13c2266e6f75` / `8a85145710b765037021dc74ff692ee333437051a5f6a39a58af5d3da8fbed47` / `5030bccea889dc9c170a57ef7bcc85610c5eed839c886c890a38d5a11a46229a` |
| K10 report | `/Volumes/AnalytixCache/development-v3/builds/pr28-full-7dc-n03/n03-7dc-k10.jsonl`, SHA-256 `c52a467f771aa23359f14c538a2a301d70819b91eaf1e60afa096912f41a653b` |

The build used a clean detached checkout of this SOURCE, the repository cache
helper, and the previously verified pinned Computer Use native package from the
canonical repository. `npm run dist:mac:arm64` exited 0. The DMG passed image
checksum verification on read-only mount; `ditto` copied the app to an isolated
installation and the image was detached. Installed Main, Go and app.asar each
matched the built app byte-for-byte. `codesign --verify --deep --strict` and
the exact built and installed mandatory legal-obligations audits exited 0.
Signing was ad hoc, notarization was skipped, and this diagnostic is not a
publishable release.

## Installed journey and timing

With `./scripts/use-analytix-cache.sh` sourced in the same shell, the owner
ran the repository K10 public-product diagnostic against the installed app:

```sh
node scripts/k10-local-product-seam.mjs --packaged \
  --app-path /Volumes/AnalytixCache/development-v3/builds/pr28-full-7dc-n03/installed/analytix.app/Contents/MacOS/analytix \
  --latency-observation --ordinary-file-probe --trace-transitions
```

The command exited **0**. The report says `green=true` and
`LOCAL_NONPUBLISHABLE_PRODUCT_SEAM_GREEN`. A fresh isolated profile and loopback
synthetic Provider were used; the Provider observed two authorized synthetic
chat requests, the ordinary file-read tool and its actual result. The reply
appeared in the Renderer. The terminal DOM trace was observed before quit.
Normal quit closed the target and Main exited 0 without fallback signal or
SIGKILL; task-owned and final residual process counts were 0. The quit request
had no explicit acknowledgment event, so the supported claim is observed
normal closure, not acknowledgment delivery.

There were two Go starts. The first preflight-to-capability-materialization
span was 27,369 ms and spawn-to-ready-line 34,857 ms; the Settings restart
spans were 3,025 ms and 21,105 ms. Launch-to-composer-ready was 127,917 ms;
the observer issued 4,106 process probes taking 26,150 ms in aggregate, and
maximum observed Main event-loop lag was 1,017 ms. Submit-to-DOM commit was
12,672.9 ms, of which IPC-send-to-DOM was 21.714 ms. These intervals overlap
and must not be added. They are one external-cache-volume and observer-loaded
sample, not a matched performance distribution or proof of a universal speedup.

The prior exact `d244` diagnostic failed `normal_quit_target_stayed_open`;
its source, app bytes and observation timing differed. This `7dc07bb`
installed run closes that symptom on this candidate. It does not identify all
cold-start causes, establish an acceptable latency percentile, or transfer to
the later Funds/authority source. A final frozen candidate needs its own
installed and applicable live-Provider/Funds acceptance.
