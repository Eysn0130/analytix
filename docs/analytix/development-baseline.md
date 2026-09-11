# Development baseline

Status: Operational. Applies to public-source development and candidate
packaging. Commands and CI results, not this document, establish readiness.

## Update main, branch, install, verify, commit, PR

Use one public `main`, not a second snapshot checkout. Before editing:

```sh
git status --short --branch
git switch main
git pull --ff-only
git switch --no-track -c codex/<short-task-name>
# Configured Owner macOS host only, in the same zsh:
source ./scripts/use-analytix-cache.sh
# Fresh clone or changed dependency manifests/lockfiles only:
npm run bootstrap
npm run verify:baseline
```

On other hosts omit the Owner-specific cache helper. `bootstrap` installs the
Git sync guard and runs locked root and runtime dependency installation. It
does not pull, stash, reset, read Provider credentials or launch the application.
If the working tree is dirty, preserve/reconcile the edits before pulling;
never reset or auto-stash them as routine setup. See [Git workflow](git-workflow.md).

`git pull` updates tracked source; it does **not** install dependencies, refresh
generated outputs, start services or migrate user data. After dependency changes
run `bootstrap`; after code changes rebuild the affected outputs. Root and
runtime lockfiles are separate. Root and runtime install fingerprints include
the manifest, lockfile, linked-package manifests, platform and Node ABI. The
root fingerprint also binds the installation scripts, and the root doctor
checks installed direct dependency versions against the lock.
An existing `node_modules` directory is not sufficient evidence. Failed or
incomplete postinstall does not receive a new success stamp. These fingerprints
check installation inputs, not every transitive package byte or native ABI.

Use Node **22.22.1**, npm **10.9.4** and Go **1.26.4** for the tested baseline.
`.nvmrc`, `.node-version`, `packageManager` and CI agree. Go's `go 1.22` directive
is the language minimum, not a claim that every packaging toolchain is admitted.
Native builds additionally use the pinned Rust **1.94.1**, SDK/tool identities,
verified build storage and native execution authority. Do not weaken those
checks to accommodate a new machine.

Backend development uses Python **3.11** (`backend/.python-version`) and uv
**0.11.5** in CI. `backend/uv.lock` pins the application and development
dependency resolution; it does not qualify native build resources or every
build-system tool. With that uv version installed, run:

```sh
uv sync --project backend --locked --extra dev --python 3.11
uv run --project backend --no-sync python -m pytest backend/tests -q
```

`--locked` rejects a stale lock instead of silently resolving newer packages.
Use a dedicated `UV_PROJECT_ENVIRONMENT` for isolated validation; ordinary
`uv sync` manages `backend/.venv` and can remove undeclared packages. Review
intentional dependency updates together with the lockfile and backend tests.
See the [uv locking and syncing contract](https://docs.astral.sh/uv/concepts/projects/sync/).

After a source change:

```sh
npm run verify:baseline
# Run the relevant application/Go/backend/plugin regression checks too.
git diff --check
git add <reviewed-task-owned-files>
git diff --cached
git commit -m "Describe the change"
git push -u origin HEAD
gh pr create --base main
```

Changing business logic still requires its contract-specific tests. Source
baseline success is not a substitute for full regression, live Provider,
privacy, packaged GUI or installer acceptance. Normal development uses a
short-lived `codex/*` branch and PR; merge only after current CI and applicable
acceptance pass. Do not push directly to main, bypass a check, or merge private
archive ancestry, force push, or push all local branches/tags.

## Development modes and real limits

| Command | What it actually establishes |
| --- | --- |
| `npm run doctor` | Source tools and dependency inputs are available/current. No compiler or app launch. |
| `npm run doctor -- --native` | Also checks Electron SQLite/terminal module loading, configured host, pinned versions and runtime asset hashes. Native binary/SDK authority is still checked during build. |
| `npm run verify:baseline` | Doctor, sync/setup regression tests, TypeScript typecheck, Electron + TS launcher build, and built-output layout smoke. |
| `npm test` | Application/renderer/host/TS-runtime Vitest suite. Not every Go/Rust/Python/script test. |
| `npm run dev` | Existing native development build, TS launcher build, then Electron. Default behavior is preserved; this command is not automatically isolated from real user state. |
| `npm run dev:fast` | Reuses already built native/runtime inputs. Not a bootstrap, isolation or acceptance replacement. |
| `npm run dev:isolated` | Explicit candidate: checkout-specific private profile, no ambient credentials, native doctor and existing build chain. Refuses launch without an explicit task Keychain; not yet a general first-run bootstrap. |
| `npm run test:plugin-contracts` | Deterministic Funds production-entry closure and report-scenario contracts; no live model request. Not full plugin acceptance. |
| `npm run assets:verify` | Pinned resource presence, size and hash checks for the current target. |

The current native builder admits macOS targets on its configured macOS build
authority. The verified host toolchain manifests currently cover
`darwin-arm64` only. Public source checks on Linux do not prove full native
development or packaging on Linux/Windows. Windows official packaging still
requires Windows x64, its signing identity and an implemented/admitted native
build route; those requirements cannot be supplied by changing a workflow label.

The explicit `dev:isolated` candidate retains its own named profile under the configured
cache's private `tmp` tree, keyed by checkout path. It does not reuse installed
Analytix data, import the caller's Provider/Hub credentials, enable Hub bootstrap,
inherit a prebuilt runtime override, or read the repository's `.env` through
Vite. Its child-only home/config paths share the existing desktop isolation
boundary; the invoking shell and normal application profile are unchanged.
Protocol/login-item registrations and update traffic remain suppressed by that
boundary. This is **state isolation, not an OS sandbox**; manually selected
files and explicitly enabled tools still require their normal permissions.

After explicit task-Keychain provisioning and lifecycle qualification, the
candidate accepts `--fast`, `--profile <name>` and `--fresh`. Do not advertise
those flags as an already qualified first-run/restart workflow: new directories
alone cannot satisfy the runtime's Keychain binding. The launcher checks this
before building or starting Electron, never creates a fake database and never
falls back to the default/login Keychain. Automatic protected provisioning,
unlock, identity continuity and whole-launcher restart remain an open work
package. The default `dev` commands were deliberately **not switched** to this
incomplete candidate.

The launcher rejects symlinked, shared or non-canonical profile directories
instead of silently repairing them. Profiles are not automatically deleted:
archive/clean exact task-owned paths only after confirming they are no longer
in use. The existing `dev:test-account` remains an **explicit legacy Hub QA**
entrypoint; it is not ordinary Provider onboarding. No credentials are copied
into a fresh profile. A real Provider is only needed for explicitly authorized
live model work. Do not point automated tests or candidates at Owner case data.

## Runtime resources and the 57 excluded files

`scripts/public-source-policy.json` remains the exact exclusion inventory.
Exclusion from public Git is not evidence that a file is unused.

| Files in the 57-file inventory | Treatment |
| --- | --- |
| 6 generated test/Python metadata files | Regenerable; keep a private recovery archive before local cleanup. |
| 32 screenshots, browser QA and dated diagnostic/benchmark artifacts | Private historical evidence; archive outside the worktree, not product source. |
| 1 local launch configuration | Keep locally; not public development instructions. |
| 13 managed-browser + 5 macOS Computer Use files | Required resource inputs. Keep until a verified replacement supply route exists. |

The 2026-09-12 cleanup completed the first two rows: 38 exact files were
hash-verified in a repository-external Owner-private archive before removal.
The archive also retains the empty, unused root Clang analyzer plist retired
from tracked source. The remaining resource/configuration files were preserved.

`scripts/runtime-assets.json` pins the present macOS ARM64 and Windows x64
resource sets, including the additional local Windows Computer Use helper.
It records no binary payload and does not grant distribution or release rights.
Prepare only from an explicitly selected, retained archive with that layout:

```sh
npm run assets:prepare -- --from /path/to/approved-resource-archive --target darwin-arm64
npm run assets:verify -- --target darwin-arm64
```

Preparation verifies the whole source set first and refuses unknown destination
bytes, symlink redirection and unsupported targets. It never downloads from an
unverified URL or searches private directories. These owner-supplied resources
mean a public clone alone is **not yet a reproducible complete installer**.
Do not force-add the excluded payloads to Git to conceal that gap.

Keep `build/` entitlements, icons and NSIS sources. Build configuration consumes
them. `out/`, `dist*`, dependency/compiler caches and dated evidence are not
maintained source; clean only exact task-owned generated outputs with recovery.
Keep the existing plugin/skill reference mirrors until their consumers and
byte-equivalence tests are deliberately changed. Historical Markdown remains
history; fix misleading current entrypoints rather than rewriting old evidence.

## CI and package candidates

`.github/workflows/ci.yml` runs on main pushes, PRs and manual dispatch:
source baseline, actual candidate history/path/blob policy, two deterministic
Funds contracts, application tests, bounded-concurrency full Go suites
(default and `analytix_prod`), locked Python backend tests and four Rust component unit
suites. Private-authority test jobs use `umask 077`, as the local cache helper
does; this does not relax permission validators. Source-set tests explicitly
model Darwin, Linux and Windows instead of assuming the CI host is Darwin.
Failures remain failures;
no `continue-on-error` converts them to green. Actions use commit pins, read-only
tokens and no Provider secrets. Dependabot covers Actions, root/runtime/Atlasflow
npm, Go, four Rust crates, backend uv and the Windows Python lock. Backend uv
and pip are [distinct Dependabot ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories).
Minor and
patch development-tool updates can be grouped; major framework updates remain
separate reviewable PRs. No automatic merge is enabled. Existing large upgrade
PRs are not safe to merge merely because the new grouping is narrower.

The public main ruleset requires PR integration and a successful `Development
gate` from GitHub Actions on a candidate up to date with main. That aggregate
fails if any required suite fails, is cancelled, is skipped or is missing.
Force updates/deletion remain forbidden; no bypass actor is configured.
Commit/push targets the feature branch, never main. PR CI runs once per PR
update instead of duplicating the same expensive suites on both branch-push
and PR events. A merge triggers separate CI for the resulting main commit.
See [Git workflow](git-workflow.md) for accepted-PR updates and conflict handling.

`package.yml` is **manual, main-only, macOS ARM64, candidate-only**. It checks
explicit readiness before queuing a dedicated runner, stages hash-pinned
resources, runs validation, calls the existing native/DMG packaging path and
verifies DMG container, resource layout, bundle identity and code signature.
Only the installer and checksum are uploaded, for seven days. No Release, tag,
R2 upload, feed promotion or production update is performed. The candidate feed
is deliberately non-serving; it is not a production update configuration.

Runner readiness is initially `NOT_CONFIGURED`. Do not set
`ANALYTIX_MACOS_PACKAGE_RUNNER_READY=true` until a **dedicated, isolated** runner
with labels `self-hosted`, `macOS`, `ARM64`, `analytix-packaging` has the pinned
toolchains, admitted storage and an explicit `ANALYTIX_RUNTIME_ASSET_ARCHIVE`.
Never register a personal/production workstation just to make this green.
PR code never runs on this packaging runner. Checkout and package output must
be fresh. A readiness variable alone does not constitute qualification.
Preflight also requires successful Development CI from a main push at the exact
candidate SHA. An older green run or a different commit cannot admit a package.

The previous `release.yml` remains a manual **legacy formal verification**
workflow, accurately named; it is not an installer factory. Its historical
platform/authority limitations are not hidden by the new CI.

DMG inspection is not app installation/upgrade, live GUI, notarization or formal
release acceptance. Those still need platform-specific, isolated execution
with real artifacts. Windows/Linux candidate jobs must not be advertised as
working until their resource supply and native host authority are implemented
and verified. The current work does not redesign mature product code to bypass
these boundaries.

## Verification snapshot and remaining work

The first [public CI run at e4fe8e0](https://github.com/Eysn0130/analytix/actions/runs/34628860194)
passed source baseline and all **1,617 backend tests**. Application tests
reported **6,088 passed, 59 failed and 15 pre-existing skips**; full Go tests
also failed. These are recorded failures, not release approval. Subsequent
commits must use their own CI results rather than inheriting this snapshot.

That run exposed three misnamed non-Darwin Go files: the `_darwin.go` filename
suffix contradicted their build constraints. Their implementation bytes were
preserved while correcting platform selection, with a dedicated regression
test. This is a compile/source-selection correction, not a new security design
or a claim that Linux native authority and all platform tests are qualified.
The corrected files passed platform-selection tests, the three existing Darwin
path/alias security regressions, and a Linux x64 `analytix_prod` runtime-server
cross-build. Cross-compilation is not Linux runtime or package acceptance.

Remaining work has distinct owners and acceptance criteria:

- Platform-aware test fixtures and source-set assertions: preserve their
  privacy/authority assertions; explicitly model the platform under test.
- Package tests and supply: make resource fixtures self-contained; real
  browser/native resources still need a verified supply route and a qualified
  dedicated builder. Do not replace missing real payloads with empty files.
- Historical formal-evidence tests: reconcile public-baseline fixtures with
  their actual contracts, without importing private history or fabricated
  acceptance receipts.
- Dependency security: the first enabled Dependabot inventory had 78 open
  advisories (33 high, 42 medium, 3 low), often multiple advisories for one
  package. Review minimal compatible fixes with regression tests; enabling
  scanning is not a claim of zero vulnerabilities. CodeQL default setup also
  completed successfully for Actions, Go, JavaScript/TypeScript and Python.
- Full installer/GUI/upgrade acceptance remains unexecuted. The package
  workflow's missing-runner preflight was exercised and correctly rejected
  packaging as `NOT_CONFIGURED`; it did not produce or publish an installer.

## Stable development route

Preserve the existing product and the one Go Core. The current stabilization
changes are an explicit isolation candidate, dependency freshness, executable CI policy,
bounded JSON/Unicode handling and QA network containment—not a product rewrite,
license reclassification or replacement of healthy implementation.

Proceed in dependency order; keep an unmet item visible rather than calling
the whole product stable because `verify:baseline` passes:

| Work package | Completion evidence |
| --- | --- |
| Development loop | Update main → short-lived codex branch → locked install/doctor → edit/rebuild/regression → branch commit/push → PR CI/acceptance → merge main. Isolated dev restart must preserve only its own profile. |
| CI portability | Separate real product failures from platform/toolchain/private-storage fixture assumptions. Make fixtures explicit; keep original negative security assertions and execute all suites. No skip/continue-on-error or fake receipt as package evidence. |
| Security maintenance | Verify each alert's reachable behavior; apply minimal compatible patches. Do not merge the mixed major Electron/TypeScript/Vite/Tailwind PR as one batch. Content digests are not password hashes. |
| Reproducible inputs | Keep npm/Cargo/Go/Python locks current through reviewed updates; retain hashes and approved supply for excluded resources. Independent native builder profiles need their own storage/toolchain qualification; an environment variable must not impersonate the Owner volume. |
| Core business regression | Isolated onboarding → synthetic Provider/stream/tool approval → deny/cancel/timeout → persistence/restart; plugin install must not self-authorize. Reuse existing public-contract tests before adding another harness. |
| Funds regression | Synthetic import → snapshot → calculation → model-safe projection → protected local display/export. Match source rows, amounts and evidence references; no real case fixtures or model credentials. |
| Candidate delivery | Qualified disposable macOS ARM64 builder + approved resources + exact-SHA CI → DMG inspection → isolated install/start/restart/upgrade. This remains distinct from a Release, feed promotion and notarization. |

The previous `52b5874` CI finished with successful source/backend jobs and
failed application/default-Go/production-Go jobs. Its application result was
6,094 passed / 53 failed / 15 pre-existing skips. The present changes fix
specific causes (including an unconfigured update-feed fixture, host-dependent
Go source-set assertions and CI private-directory creation); they do not erase
that baseline failure or prove all remaining tests have passed.

The 2026-09-12 stabilization candidate based on `52b5874` was verified locally
on macOS ARM64: 41 baseline tests, native doctor, TypeScript checks, source
build/layout smoke, two deterministic Funds contracts, Darwin/Linux/Windows Go
source-selection tests, and the complete affected Go evidence/reasoning packages
in ordinary and production modes passed. Targeted updater, isolation and
non-disclosure fixture tests passed; these are not a full application-suite
result. A fresh isolated Python 3.11 environment installed from `backend/uv.lock`
passed all **1,617 backend tests** (two dependency deprecation warnings).
Workflow lint passed. Installer and live Provider checks were not executed.
The Git round-trip regression installs the real pre-push hook in both synthetic
clones, including its argv/stdin handling and rejection before remote mutation;
an always-successful hook stub is not evidence of the protected push path.

### PR-mode transition (2026-09-12)

[PR #22](https://github.com/Eysn0130/analytix/pull/22) carries the PR-only hook,
fail-closed aggregate gate and platform-fixture corrections. The public main
remains `58b365b3bc878e987e215d1fb1862ef111a1a37e` until a verified merge.
Local baseline/type/build checks, 18 Git/gate tests and all 15 updater tests
passed. Keychain file-identity and recovery-journal replacement fixtures now
retain the old inode instead of assuming unlink/recreate changes identity.
The case-sensitivity fixture uses distinct alphabetic names and asserts both
filesystem behaviors instead of treating a numeric temporary name as an alias.
These targeted Go checks passed locally in ordinary and `analytix_prod` modes;
the changed Linux branches still require Linux CI. No production permission,
storage or privacy validator was relaxed.

The local full persistence package attempt exceeded a three-minute package
deadline while progressing through startup crash-cut tests: **timeout, not
PASS**. The single previously failing statistics CLI case passed on macOS;
this does not resolve the Linux CI DuckDB internal error. The main CI also
reported application and both full Go-suite failures. Keep this PR draft until
the current candidate's required CI and applicable acceptance actually pass.
Neither these focused results nor the source baseline imply full regression,
native packaging or formal release readiness.

One public historical secret-scanning alert identifies a key-shaped negative
test fixture. The current fixture now constructs an explicitly synthetic
canary and retains its non-disclosure assertion. That does **not** prove the
previous literal was never issued, remove its public historical blob or justify
dismissing the alert. Resolve original credential provenance/rotation separately
without printing or exercising the value. No real Provider check is part of
these deterministic development tests.
