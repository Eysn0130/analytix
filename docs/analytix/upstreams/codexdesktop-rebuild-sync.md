# analytix CodexDesktop-Rebuild sync ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"codexdesktop-rebuild","reviewedCommit":"5b83c2a2a8503aaaf0f346010df179e038f59bbc","parity":"not-proven","capabilityBenchmarkV1":null} -->

CodexDesktop-Rebuild is a desktop-experience reference source. Use it for
responsiveness, scroll behavior, thread virtualizer architecture, app-server
style boundaries, worker decomposition, signal/query state, and interaction
rhythm.

CodexDesktop-Rebuild does not own analytix product identity, feature set,
runtime protocol, or implementation naming. Do not copy minified chunks as
production modules.

It also has no repository-wide project license at the reviewed commit. The
README's statement about the original Codex CLI being Apache-2.0 does not
license this repository's rebuild scripts, downloaded application, extracted
chunks, or assets. Unless a material-specific written authorization is
recorded, this source is behavior-only clean-room input.

CodexDesktop-Rebuild is a default desktop-feel lens, not the only source for
interaction quality. Future UX/runtime reviews may compare it with Kun,
Reasonix, OpenCode, Hermes Agent, current analytix, and later sources before
choosing what to absorb.

## Local Source

```text
/Users/sun/Projects/_upstreams/CodexDesktop-Rebuild
```

## 2026-07-20 - Current white-box intake

| Field | Value |
| --- | --- |
| Branch / commit / tag | `master` / `5b83c2a2a8503aaaf0f346010df179e038f59bbc` / current tracking branch |
| License evidence | No root project license tracked at the pinned commit. |
| Admitted posture | `reference-only-unlicensed`; behavior observation and clean-room tests only. |
| Reproducibility boundary | `src/*` application extracts are ignored/generated local output and are not commit-pinned source evidence. |

The repository is not an independently buildable desktop source tree. Its
pipeline downloads official ZIP/MSIX artifacts, extracts roughly 1.7 GB of
ignored application material, patches minified ASAR/native payloads, and
repackages them. Generated `src/*` bytes are therefore neither review-marker
evidence nor a safe source of Analytix runtime code.

| Area | Current evidence | Decision |
| --- | --- | --- |
| Artifact acquisition | macOS appcast and Windows store payloads are downloaded without complete content/signature verification; the Windows selector can fall back to the first URL when no digest matches. | `reject`; Analytix packaging must verify pinned digest/signature and fail closed on lookup drift. |
| Signing and integrity | macOS removes upstream signing and uses ad-hoc signing; Windows patches the executable ASAR hash without Authenticode. | `reject`; require actual signing/notarization or approved-host evidence and preserve update compatibility. |
| Electron fuses | Linux enables RunAsNode, `NODE_OPTIONS`, and inspect while disabling cookie encryption, ASAR integrity, and only-load-from-ASAR. | `reject`; benchmark secure fuse defaults and packaged-runtime integrity. |
| Authorization patches | `patch-plugin-auth.js` removes plugin authentication, bypasses feature/Statsig checks, forces bundled capabilities, and bypasses native-peer signing authorization. | `reject`; these are security bypasses, not compatibility mechanisms. |
| Update path | The updater is disabled with no authenticated replacement; several patch misses report success/skip. | `reject`; Analytix update and rollback must remain authenticated and fail closed. |
| Cross-platform build organization | Platform/architecture matrix, macOS `ditto`, native-module rebuild, ASAR header-hash update, and missing-artifact failure are concrete engineering ideas. | `adapt` only through Analytix-owned build scripts, source contracts, and fresh packaged E2E evidence. |

The tracked repository has no test, lint, typecheck, E2E, or security-gate
script; `npm test` is absent. Twenty tracked JavaScript files pass syntax
checking, which proves only parseability. There is still no root project
license. User-declared reuse authorization remains a separate provenance
record and does not make downloaded application assets source evidence.

To exceed this source's desktop completeness, Analytix must prove its own
Renderer -> preload -> main -> Go runtime chain, native artifact manifest,
secure fuses, signed/notarized package where applicable, authenticated update
and fail-closed rollback, supported-host matrix, SBOM/provenance, and real
startup/E2E behavior. Merely producing an archive is insufficient.

Bounded decision:

- retain version-drift, artifact-manifest, hashing, cross-platform packaging,
  virtualizer, worker-split, and interaction observations as research inputs;
- reject direct copying of scripts, minified chunks, prompts, binaries, icons,
  brands, or other extracted assets without a separately recorded grant;
- reject upstream patches that remove authentication/feature gates, disable
  updates, or weaken Electron fuses as Analytix production patterns.

## Absorption Bias

| Area | Default stance |
| --- | --- |
| Thread virtualizer | Absorb layout and bottom-distance behavior through analytix-owned virtualizer code. |
| Streaming responsiveness | Use frame scheduling, buffered projection, and worker separation evidence. |
| App-server boundary | Compare with analytix runtime HTTP/SSE and future app-server facade contracts. |
| Composer interactions | Absorb controller/view separation and latency-hiding behavior without deleting analytix workflows. |
| Visual rhythm | Rewrite into analytix tokens/assets; no Codex identity or chunk ownership leaks. |

## Batch Template

```text
## YYYY-MM-DD - CodexDesktop-Rebuild <commit-or-range>

Source:
Reviewed by:
Related branch:

Experience hypothesis:

Classification:
| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |

Desktop QA:

Remaining risks:
```
