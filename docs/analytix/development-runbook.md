# Analytix Development And Validation Runbook

- Status: Operational
- Applies to: local development, validation, packaging, and startup diagnosis
- Development routes updated: 2026-09-20; verify the current worktree before use
- Source of truth: `package.json`, package scripts, build configuration, and
  `scripts/`

This page routes commands; a listed command is not evidence that it passed.
Record the current worktree, platform, command, exit status, skips, and relevant
environment whenever the result supports a present-tense claim.

For the public `main` development loop, dependency refresh after `pull`,
portable source checks, runtime-resource preparation and CI/package status,
start with [Development baseline](development-baseline.md). The configured
macOS storage procedures below are host-specific, not prerequisites for every
public source checkout.

## Development Routes

The primary route is **Codex Desktop on the configured local Mac**, using the
canonical `/Users/sun/Projects/analytix` workspace. The user's 2026-09-20
authorization includes source edits, builds, tests, Electron GUI, synthetic
Office/PDF/image tests, Chinese IME, local private installation and proportionate
product acceptance on this Mac. Another Mac, VM, cloud host or independent
execution entry is not a prerequisite. Use the configured cache and isolated
synthetic profiles/data; this authorization does not open existing personal
profiles, credentials or real case data to a test process.

The auxiliary route is **ChatGPT + GitHub** for development and review through
the capabilities actually available and authorized in that environment. Keep
the same repository, real commit ancestry, PR and evidence ownership across
both routes. On return to local work, refresh remote refs/receipts and protect
any newer local or remote candidate before integration. A cloud source test is
not installed Mac/native Office/IME acceptance; a local result does not prove
that its candidate has reached GitHub or main.

The earlier task restriction against using the user's Mac has been superseded
by this explicit authorization. Dated handovers retain their historical facts
and are not current host prohibitions. Neither route change nor general local
execution/Git authority clears the earlier `create_tree` safety rejection,
Chromium `ERR_BLOCKED_BY_ADMINISTRATOR`, or SecurityAgent refusal: each still
requires its own applicable resolution evidence before a dependent retry.
Do not switch interfaces, executors, profiles or credentials to bypass them.
Private installation remains subject to its actual asset/platform/signing
requirements; this strategy does not authorize tags, releases or publication.

## Preserve The Worktree

Before repository-grounded review, diagnosis, implementation, or mutation,
start with:

```bash
git status --short --branch
```

Pure explanation that does not depend on repository state does not require
this inspection. Preserve unrelated user changes. The local mainline is
`main`; new branches use `codex/` unless the user requests another prefix.

After an authorized change satisfies its success criteria and relevant checks
show no change-caused regression, create a focused local commit without asking
again unless the user asks to leave changes uncommitted. Stage only files or
hunks owned by the task. If they cannot be isolated safely from existing user
work, leave the task changes uncommitted and report the conflict.

This standing authority does not include amend, push, force-push, merge,
rebase, tag, publish, release, or pull-request actions. Those require explicit
authorization in the current request. Destructive operations such as
`git reset --hard`, `git clean`, broad checkout/restore, history rewriting, or
deleting untracked content also require explicit authorization, exact verified
targets, and a recovery path.

Do not edit generated apps, package extractions, `output/`, `dist*/`, local
evidence, or temporary logs as source. Their existence or Git history does not
make them current implementation input.

## Configured macOS Cache

Dependency installs, development servers, tests, builds, packaging, and native
compiler/toolchain commands that use build storage on the configured macOS host
must load the policy in the same shell as the command. Pure text inspection and
read-only Git commands are exempt; a cache failure blocks only dependent work:

```bash
source ./scripts/use-analytix-cache.sh && <command>
```

Apply this only when the script exists and the environment is the configured
host. The helper first binds the exact configured DataSSD mount and volume
identity, then requires the v3 sparsebundle to be a real, canonical directory
on that backing device. Before its first directory creation, it binds the v3
image to the exact APFS device, volume UUID, and `/Volumes/AnalytixCache` mount;
requires writable, non-read-only mount flags with ownership enabled; rejects
every attached entity from either retired damaged predecessor image; and
preflights every existing canonical namespace. The retired image paths may be
absent. During a bounded retirement transition, any still-present predecessor
must remain a real, detached, canonical directory on the same backing device.
A symlink, non-directory, foreign owner, foreign device, non-`0700` mode,
non-writable path, or non-canonical path fails closed rather than being repaired
as a side effect of sourcing the helper.

Canonical top-level namespaces under
`/Volumes/AnalytixCache/development-v3` include dependency and compiler caches,
`tmp`, `bin`, and `evidence`. `bin` and `evidence` are part of the same
owner/device/mode inventory rather than unverified side directories. The
helper routes npm, Go, Cargo registry/git and target, Python bytecode and
pip/uv/mypy/ruff, C/C++ and sccache, xwin, Corepack, Electron,
electron-builder, Playwright, Node compile, XDG, and temporary data to that
volume. It leaves the calling shell at `umask 077`; it never widens the umask
to `022`. Directories must be created explicitly as `0700`. Evidence, logs,
checksums, receipts, and other non-executable outputs must be created
explicitly as `0600`; executable cache outputs may be `0700`. The helper
rechecks existing `bin` and `evidence` descendants against those constraints.
Do not silently fall back to system-volume caches.

The configured host mount job is `com.analytix.mount-cache`; its local script
must attach
`/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle` with owners enabled.
`AnalytixCache-v2.sparsebundle` failed read-only APFS verification on
2026-08-02, and the original `AnalytixCache.sparsebundle` was also damaged.
Both predecessor images were retired from the active DataSSD backing volume on
2026-08-05 after the v3 preservation bundle and canonical Git source were
reverified. They are not product source, current validation evidence, or future
development inputs. Their absence is authoritative. Do not recreate, attach,
repair, or copy either predecessor into v3; the helper retains their exact paths
only as an attachment denylist and bounded-transition guard.

Treat cache capacity as three independent layers and record each one before a
cold build or package:

1. backing `/Volumes/DataSSD` application-available bytes are the physical
   ceiling for further sparse-image growth;
2. v3 sparsebundle `du` allocation is the physical space already consumed on
   that backing volume;
3. inner APFS/container free is only logical headroom inside the image.

The cache mount's own `statvfs` application-available value is an additional
application-view reading. It does not replace the backing measurement, and
inner APFS free never proves that backing physical space exists. A minimal
capacity inventory after the helper's fail-closed validation is:

```bash
source ./scripts/use-analytix-cache.sh && \
  df -k /Volumes/DataSSD /Volumes/AnalytixCache && \
  du -sk /Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle && \
  diskutil info /Volumes/AnalytixCache
```

Convert `df -k` application-available blocks and `du -sk` allocation to bytes
with a factor of 1024. Before packaging, compare backing physical availability
against the most recent measured commit-archive, cold-package, unpack, and
transient-test peaks plus an explicit reasonable margin. Do not start package
work when only the inner logical-free number passes that preflight.

Offline compaction on this configured host must follow one bounded sequence:
work from outside the cache volume; suspend only
`com.analytix.mount-cache`; confirm no cache cwd or ordinary open handle; run
`diskutil verifyVolume`; attempt one normal, non-forced detach of the freshly
resolved v3 whole image; compact only that exact detached v3 sparsebundle;
reattach it with owners enabled; restore the mount job; then run the canonical
verifier. If the normal detach is
rejected as busy after the root-owned `syspolicyd` dissent observed on this
host, do not force, terminate or restart the service, or loop retries. Restore
the mount and job, record
`HOST_RESTART_REQUIRED_FOR_OFFLINE_COMPACTION`, and restart the host before a
later single offline attempt. The 2026-08-04 attempt verified this fail-closed
path; it did not produce a successful compact.

After replacing or recreating the cache image, run a read-only
`diskutil verifyVolume` before any development write, confirm ownership and
private `0700`/`0600` mode enforcement, then update the helper's exact local
volume binding. Start every cache namespace empty; never copy an older
`GOCACHE`, `GOMODCACHE`, Cargo target, dependency cache, build cache, or
temporary directory into the replacement image.

On the configured host, the canonical post-mount verification is:

```bash
source ./scripts/use-analytix-cache.sh && ./scripts/verify-analytix-cache.sh
```

The verifier sources the helper again in its own process, temporarily suspends
only the exact configured mount job, and runs `diskutil verifyVolume` before
creating its probe. Disk Arbitration may remount an otherwise valid disk image
with `noowners`; the verifier therefore re-resolves the exact v3 sparsebundle,
whole image device, APFS volume, UUID, mount point, writable/read-only flags,
and open-handle inventory after `verifyVolume`. If and only if ownership is the
sole changed authority, it non-forcibly detaches that exact v3 image and
reattaches the same image with `-owners on`; every other drift fails closed.
It then reruns the complete helper before its first probe write and restores
the mount job. Never run the verifier from a process whose cwd or open files
are on the cache volume.

After that post-verify authority check, the verifier performs isolated cold
and warm Go builds with `HOME`, XDG, Go cache, module cache, and temporary paths
all below the trusted volume. It requires a compiler action in the cold run and
none in the warm run, byte-identical SHA-256 outputs, a successful ad-hoc
`codesign` plus strict verification, and a runnable signed probe. Only after
all checks succeed does it create a unique `0600` receipt below
`development-v3/evidence`; it prints the receipt path. Any failed requirement
exits nonzero and is `BLOCKED`, not a partial pass. The receipt proves only the
configured development-cache and local tool seam. It is not packaged-app,
Developer ID, notarization, release-signing, or product-delivery evidence.

To create a retained command log without relying on `umask` alone, allocate it
before redirection with an explicit mode:

```bash
source ./scripts/use-analytix-cache.sh && \
  analytix_command_log="$(/usr/bin/mktemp \
    "$ANALYTIX_DEV_CACHE_ROOT/evidence/command-log.XXXXXXXX")" && \
  /bin/chmod 600 "$analytix_command_log" && \
  <command> >"$analytix_command_log" 2>&1
```

The exact `mktemp` target makes the name unique; never replace it with a fixed
path that could overwrite an existing receipt or user-owned evidence. A
verifier exit code, hash, or signature check does not by itself prove that an
unrelated test, package, or product flow passed.

The external volume expands storage capacity; it is not additional RAM and it
does not become a second source repository. Product edits remain under
`/Users/sun/Projects/analytix`. Source files edited only in a cache extraction
or temporary worktree are `NOT_INTEGRATED` until the exact change is landed in
the canonical repository, freshly verified, committed, and reverified from that
commit's archive.

The helper does not transparently move an existing worktree, `node_modules`,
runtime profile, secret store, or package. For capacity-constrained dependency,
build, or package work, create a task-owned clean commit-archive extraction
below `$ANALYTIX_DEV_CACHE_ROOT/tmp`, run `npm ci` and all generated outputs
there, and put package/evidence outputs in helper-controlled cache namespaces.
Those derived trees are never source. Runtime profiles and credentials use a
separate task-owned isolated profile and must not be mixed into dependency or
compiler caches.

Existing user-owned runtime state, including `~/.analytix/data`, is not
development test input. Use isolated fixtures or temporary directories by
default. Real user state requires explicit authorization plus compatibility,
backup, rollback, failure-recovery, and verification plans.

## Setup And Execution

After pulling, use `npm run doctor` to check root/runtime installation inputs.
Install dependencies when absent, unstamped, or changed (including linked
package manifests and Node ABI):

```bash
npm ci
```

Start the development application with:

```bash
npm run dev
```

`npm run dev` is an execution path, not a validation verdict. Define the
observable behavior being checked and capture failures separately.
Ordinary `dev` / `dev:fast` retain their existing behavior; do not run them
against real user state as automated acceptance. The explicit `dev:isolated`
candidate prepares checkout-specific state and checks native inputs, but also
requires a provisioned task Keychain. Its provisioning/restart lifecycle is
not yet a general development bootstrap and has not replaced the default entry.
See [development baseline](development-baseline.md) for the full development
loop, platform limits and remaining CI/package work.

### Identify a GUI candidate before comparing it

Keep development instance, profile, source commit and build fingerprints in
private acceptance records. They do not belong in the product top bar or the
composer's always-visible controls. Model display names describe the configured
model selection; they do not attest the implementation behind a custom endpoint.
Keep technical IDs and connection details available in Settings and tooltips.

For GUI acceptance across tasks, retain the source commit plus task-owned dirty
blob manifest, frozen Main/preload/Renderer build, profile ID and connection
kind alongside screenshots. Use one integrator for canonical source changes.
Do not keep acceptance attached to a shared HMR build while another task edits
that source. A Mock screenshot and an official-Provider screenshot exercise
different configurations; neither substitutes for the other. Investigate a
suspected regression by comparing those identities before reverting code or
reinitializing any protected profile.

## Validation Matrix

Choose the smallest evidence that can fail on the changed behavior.

| Changed surface | Starting checks | Add when risk crosses layers |
| --- | --- | --- |
| Documentation only | path/link inspection; `git diff --check` | executable command only for a present-tense executable claim |
| Renderer/main/preload/shared | focused Vitest; `npm run typecheck` | `npm run build`; real Electron or packaged check for OS/runtime behavior |
| TypeScript runtime contracts/launcher | `npm --prefix packages/runtime run typecheck`; focused package tests | package build, focused Go tests, desktop tests |
| Go runtime | `gofmt`; focused `go test` | `go test ./...`; `-tags analytix_prod`; desktop contract tests |
| OpenSpec skills | focused content/link inspection | `npm run verify:openspec-codex-skills` |
| RC validation control plane | focused contract tests | after source freeze, one `npm run runtime:go:rc-control-plane -- --json` run |
| Packaging/release path | focused build and package audit | `npm run runtime:go:release-gate`; approved platform QA |

Common repository checks:

```bash
npm run typecheck
npm run test
npm run lint
npm run build
npm run build:runtime
git diff --check
```

`runtime:go:rc-control-plane` proves only that the source-bound validation
control plane completed. It may exit successfully while the product RC remains
blocked, and its receipt must retain that claim ceiling. By contrast,
`runtime:go:release-gate` exits successfully only when its final JSON has
`passed: true`, including artifact admission, the complete active OpenSpec
`RC_REQUIRED` projection, preflight, and all authorized formal product seams.

TypeScript runtime package:

```bash
npm --prefix packages/runtime run typecheck
npm --prefix packages/runtime run test
npm --prefix packages/runtime run build
```

Go runtime:

```bash
(cd packages/runtime-go && go test ./...)
(cd packages/runtime-go && go test -tags analytix_prod ./...)
```

`npm run build:runtime` builds the TypeScript launcher/contracts package. It
does not compile or validate the Go core.

If a check fails, determine whether the change introduced it or it is a
pre-existing baseline failure. Report both truthfully; do not label a partial,
skipped, blocked, or unconfigured path as passed.

## Packaging

Current entry points are:

```bash
npm run dist
npm run dist:mac
npm run dist:mac:arm64
npm run dist:mac:x64
npm run dist:mac:signed
npm run dist:win
npm run dist:linux
```

General packaging uses `electron-builder.config.cjs`; official Windows
packaging also uses `electron-builder.standard-win.cjs` and the approved
Windows host gate. Packaging is an external-state and platform-sensitive path:
do not run a release or publication workflow merely because the command is
listed. On the configured macOS host, the cache-capacity preflight above is a
package prerequisite: backing physical availability, not inner APFS logical
free, must cover the measured package, unpack, archive, transient-test, and
margin requirement.

Packaged apps must ship the platform-native Go `runtime-server` binary and must
not depend on a user-installed Go toolchain or bundled Go source.

## Startup Diagnosis

Do not infer health from a port alone. Correlate:

1. the actual desktop and runtime process paths;
2. app/main/runtime logs and timestamps;
3. listener ownership and port;
4. `curl http://127.0.0.1:<runtime-port>/health`;
5. runtime identity/info and the package being tested;
6. renderer-visible behavior when the symptom is cross-layer.

Useful routing starts at:

- `src/main/runtime/analytix-adapter.ts`
- `packages/runtime/src/cli/serve-entry.ts`
- `packages/runtime-go/cmd/runtime-server/main.go`
- `packages/runtime-go/internal/runtimeapp/app.go`

For provider, settings, attachment, SSE, or event-growth diagnosis, follow all
applicable `AGENTS.md` files from the root through the owning directory; those
paths require more than startup evidence.

## Keep This Runbook Current

Verify scripts against the current `package.json` and build configuration
before editing this page. Add commands only when they are maintained entry
points. Move date-bound results to evidence documents rather than turning this
runbook into a pass ledger.

## Core package candidate profile

`npm run dist:mac:arm64:core` selects the explicit `core` profile in the existing
Electron builder and after-pack owners. The default remains `full`. The current
Core profile is a local, non-publishable Darwin arm64 candidate path; formal
release intent is rejected until the Core disposition is qualified by the
existing signing/publication lifecycle. This is a source implementation, not
installed acceptance or release approval.

The Core resource closure excludes the Funds plugin tree, Python data backend,
all four data-analysis native binaries and their native authority generations,
private Office engine and document-runtime. Both after-pack and the packaged
Go reader reject unexpected professional payloads; the Go executable binds the
profile at link time. Main skips Funds materialization, and the Go Host denies
saved Funds activation before source inspection, catalog advertisement or effects.
Existing case and installed-plugin state is retained. Case tasks remain subject
to protected-root and authority checks; ordinary tools do not gain case access.

The same Go Core, Electron/Main/preload/renderer, ordinary tool/terminal lanes,
Provider Registry/Secret Store, approvals, history, Skills/MCP and ordinary child
execution remain included. Existing Office codec WASM, document preview and
renderer code remain packaged: their license and applicable shared checks are
still required. Excluding the private Office engine does not claim those assets
or advanced editing have passed qualification. The complete profile and its
backlog remain intact.

Use the cache helper in the same shell. Pin the source commit and inspect the
actual resulting resource/seal/authority closure before installation. Do not
launch a GUI across the unresolved Chromium/OS storage admission boundary or
interpret an ad-hoc Core candidate as a public release.

A clean build worktree does not contain ignored Computer Use native artifacts.
The existing after-pack owner accepts `ANALYTIX_COMPUTER_USE_PACKAGE_ROOT` for
that dependency. When reusing a local input, first verify its pinned provenance,
listed file and adaptation hashes, and signature; supply the package root through
that entry point and retain the ordinary after-pack validation. A private ad-hoc
input is not release-qualified. A failed package must be rebuilt through the
normal lifecycle, not resumed with `--prepackaged` or relabeled as complete.
