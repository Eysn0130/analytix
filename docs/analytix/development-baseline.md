# Development baseline

Status: Operational. Applies to public-source development and candidate
packaging. Commands and CI results, not this document, establish readiness.

The primary development and product execution route is Codex Desktop on the
configured local Mac; ChatGPT + GitHub is the auxiliary route. See
[Development routes](development-runbook.md#development-routes) for the current
host authorization, isolated test data and route-specific evidence boundaries.
An independent Mac or cloud host is not a default prerequisite.

## Update main, branch, install, verify, commit, PR

Use one public `main`, not a second snapshot checkout. The sequence below is
for new work. When explicitly continuing an existing unmerged PR, first refresh
its actual head/base and candidate ancestry, preserve newer work, and continue
that authorized branch; do not switch to old main or rebuild its history.
Before new work:

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

After dependency, baseline, build-layout or broader source changes, use the
applicable source baseline and regression gates. For bounded maintenance use
the routing below before selecting wider checks:

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

## Bounded maintenance routing

Resolve the changed contract and its actual consumers first. The existing
`validation-burden.mjs --plan <task-owned-paths> --json` maps Markdown under
`docs/`, the root README/AGENTS, and **two exact source/test fixtures** below.
It is not a whole-repository affected selector: scopes containing any unknown
path return `unmapped` and require affected-owner selection from the existing CI matrix.
It checks relative link paths, not anchors or remote URLs.

| Representative task | Owner and necessary first checks | Widen when |
| --- | --- | --- |
| Documentation correction | `docs/AGENTS.md`, current source/accepted scope; `git diff --check` and `node scripts/validation-burden.mjs --check-links <changed-markdown>`; verify edited anchors/commands against their actual owner | Executable/generated claims need their owning gate. OpenSpec task edits also need strict validation of the selected change; boxes are not implementation evidence. |
| Local TS schedule maintenance | `src/shared/gui-update-schedule.ts`, its test and `src/main/gui-updater.ts`; mapped plan runs `npm run typecheck` then `npm test -- src/shared/gui-update-schedule.test.ts` (tests-only edits run that test) | Changed consumers, public/IPC/settings/provider contracts, bundling or UI behavior require their own owner checks. This fixture does not map other TS files. |
| Go grant authority maintenance | `internal/domain/security/grant_registry.go` and its tests; mapped plan runs the three `TestExecutionGrantRegistry*` cases in both ordinary and `analytix_prod` modes | Live authorization effects, persistence/restart, HTTP/SSE or production assembly changes require their actual consumers and security/public-seam evidence. Platform suffix/build tags require the affected native lane; production tag alone is not a platform claim. |

On the configured Mac, source the cache helper in the same shell before tests,
compiler or build commands. Formal package/native/Provider checks follow their
own authorization and acceptance triggers; ordinary text maintenance does not
invoke them. Reuse unchanged evidence with its source/environment fingerprint.
PR37's shared regression ownership remains in the product-regression runner:
packaging and settings owners execute their shared files once per same invocation,
and dependent named gates consume complete assertion receipts. Standalone local
fixture selection and that shared runner are different scopes; neither removes
CI tests or assertions. See [the existing evidence](qa/shared-regression-source-row-2026-10-01.md).

## Development modes and real limits

| Command | What it actually establishes |
| --- | --- |
| `npm run doctor` | Source tools and dependency inputs are available/current. No compiler or app launch. |
| `npm run doctor -- --native` | Also checks Electron SQLite/terminal module loading, configured host, pinned versions and runtime asset hashes. Native binary/SDK authority is still checked during build. |
| `npm run verify:baseline` | Read-only validation/entry burden report, Doctor, sync/setup regression tests, TypeScript typecheck, Electron + TS launcher build, and built-output layout smoke. |
| `npm test` | Application/renderer/host/TS-runtime Vitest suite. Not every Go/Rust/Python/script test. |
| `npm run dev` | Existing native development build, TS launcher build, then Electron with private task state and shared protected development credentials. |
| `npm run dev:fast` | Reuses already built native/runtime inputs. Not a bootstrap, isolation or acceptance replacement. |
| `npm run dev:isolated` | Private task state with shared protected development credentials; `--isolated-keychain` explicitly selects release/QA isolation. Whole desktop lifecycle acceptance is separate. |
| `npm run test:plugin-contracts` | Deterministic Funds production-entry closure and report-scenario contracts; no live model request. Not full plugin acceptance. |
| `npm run assets:verify` | Pinned resource presence, size and hash checks for the current target. |

The current native builder admits macOS targets on its configured macOS build
authority. The verified host toolchain manifests currently cover
`darwin-arm64` only. Public source checks on Linux do not prove full native
development or packaging on Linux/Windows. Windows official packaging still
requires Windows x64, its signing identity and an implemented/admitted native
build route; those requirements cannot be supplied by changing a workflow label.

The Electron and CLI Go launchers remove case aliases of their reserved child
environment names before installing the managed runtime token. Electron also
removes aliases of its host-owned config/resource paths and existing protected
authority fields. Unrelated `Path` and `SystemRoot` values retain their original
names and values. This follows the pinned [Node 22.22.1 child-process environment
contract](https://nodejs.org/download/release/v22.22.1/docs/api/child_process.html#child-process),
which folds Windows environment names at spawn. Local producer and subprocess
tests do not establish Windows-native execution or credential qualification.

Windows process groups/Job Objects and shell discovery do not establish
filesystem containment. The server now forwards the selected process protected
roots on every platform, including the mandatory floor under
`danger-full-access`. Non-Darwin adapters reject that policy before process start until native containment is
implemented; removing the roots to enable shell execution is not a fallback.
The required [Windows Provider credential backend CI lane](../../.github/workflows/ci.yml)
runs on Windows 2022 and covers native DPAPI, ACL, replacement, Registry restart,
development credential-authority isolation/restart and source-profile wiring.
That focused lane does not establish descendant cleanup/pipe-drain, filesystem
containment or complete Windows native packaging. The accepted first Windows
installer target remains x64 NSIS; no minimum OS version or WSL qualification is
established by these source fixes, and no Windows installer is produced here.

Normal `npm run dev` (or `dev:fast` with admitted existing build inputs) uses
private task `home` and `user-data` directories under `~/.analytix-development`.
The `dev:isolated` alias keeps this application-state isolation; credential
isolation is a separate explicit choice. Ordinary profiles have a
`development-profile-` prefix so a retained QA profile is never silently
converted. Protocol/login registrations and update traffic remain suppressed;
this is state isolation, not an OS sandbox.

Ordinary development shares one existing Provider Registry and encrypted Secret
Store under `~/.analytix-development/provider-credentials`, independent of
checkout/task lifecycle. The existing fallback master-key provider uses private
0700 directories and 0600 files, exclusive initialization and readback. It is
protected-local file authority, not OS-backed storage. Task profiles never copy
credential values. Core receives this choice only through the main-private
startup frame; the source binary requires `analytix_dev_credentials` in addition
to `analytix_prod`. Packaged Electron rejects the option and ordinary packaged
Go builds omit the development tag. Production selection is unchanged.

Core's existing Registry OS lock, transaction/recovery and generation fences
also govern the shared development owner. Startup semantic planning does not
recover or initialize that independent owner; activation performs its normal
recovery. The authority directory is a mandatory protected root for model tools.

Once bootstrapped, a noninteractive bounded check uses the existing runtime CLI:

```sh
source ./scripts/use-analytix-cache.sh
cd packages/runtime-go
go run -tags analytix_prod,analytix_dev_credentials ./cmd/runtime-server provider verify \
  --development-authority-dir "$HOME/.analytix-development/provider-credentials"
```

The one-time `--bootstrap-stdin` option accepts protected stdin bytes, never a
command argument, environment file or log. It refuses to replace an existing
Provider; replacement remains the existing Settings transaction. The check
resolves the stored credential through Core/Registry/Secret Store, sends one
synthetic no-tool `deepseek-flash` request to the exact official DeepSeek endpoint
with 32 output tokens and no reconnect, and selects a new Provider only after
success. A failed validation retains an unselected retryable credential.
Output includes only safe status and usage; `returnedModel` is null because the
current adapter does not retain the upstream model echo. It does not substitute
for GUI acceptance or authorize exceeding the user's test budget.

Use `--isolated-keychain` only for explicit release-admission/credential-isolation
QA. For example, `npm run dev:isolated -- --isolated-keychain --profile qa` retains
the prior task-specific namespace and protected password prompt. Add
`--unlock-keychain` only in that mode. Credentials are never imported from the
shared development owner or Production. Creation/unlock follows successful
prerequisites, leaving the full unlocked window for testing. Cancellation or
missing retained state preserves the incomplete profile; `--fresh` makes a
separate profile. A locked QA Keychain remains unavailable and must not fall
back to the development file authority.

Production still uses normal OS-backed authority and system authorization;
there is no separate Analytix password. CI fixtures continue to use explicitly
injected synthetic/ephemeral authority and never the host's development store.

The Go Secret Store persists committed physical Keychain identity in the V2
record under the existing private binding filename. Restart rejects replacement
even when database bytes are identical. Only an owned protected write with
exact readback may commit an inode transition. Failure between the Security
write and identity commit preserves state and refuses automatic adoption.
Older digest-only V1 bindings do not carry the required historical identity;
they remain preserved and unavailable pending explicitly authorized migration.
The launcher only reads the Core record before unlock/launch; it never repairs
or refreshes authority. Pending identity metadata blocks `--unlock-keychain`;
launch once without that flag to let Core validate and recover metadata first.
An unconfirmed write remains unavailable. A confirmed write with only cleanup
residue can recover before the next explicit unlock. Rollback cannot replace a
valid identity record with an unknown backup. V2 records require the writer's
canonical encoding; duplicate fields and linked records are rejected. Each
Security credential command has its own ten-second deadline. Explicit task
commands first run a bounded, noninteractive native lock-metadata check; an
already locked or unqualified Keychain fails before starting `security`. This
does not change the lock policy or read credentials in the metadata process.
Locking after preflight remains a race covered by the command deadline, so this
does not guarantee suppression of every possible OS authorization dialog. A timeout does
not prove a native write had no side effect. These controls and focused tests do not establish GUI,
real Provider, installation or complete restart acceptance by themselves.

Ordinary `electron-vite` development runs a cache-built Go executable without
the native package resource seal. Building native tools does not grant that
process import authority: data import remains unavailable in this mode. Use
the existing non-publishable development `.app` build, including its complete
packaging/signing lifecycle, to assemble the native desktop inputs. That does
not establish successful GUI launch or installer acceptance. The earlier
isolated packaged launch encountered a separate startup wait: Electron's enabled Cookie encryption
initializes OS key storage before Core, while the explicit task binding covers
only the Go Secret Store. The old task-login GUI harness does not satisfy the
accepted non-login binding contract and must not be reused as a fallback.
Keep Cookie encryption enabled. The later installed8e QA session completed
normal startup and Provider/restart checks; it does not establish stable
Developer ID identity or eliminate all future OS authorization prompts.

In a runtime with admitted native import capability, on first CSV/ZIP selection
in a workspace without a case, Main requests explicit
native confirmation to create an empty case identity. Core binds that one-use
intent to its current process, principal, source selection and physical
workspace. Restart or replacement invalidates the old confirmation. Creation
does not activate evidence or mint a receipt: the existing preview/confirm and
snapshot admission path still applies. A canceled preview retains the empty
case. A confirmed import without a committed evidence receipt retains its source
and snapshot but needs explicit reconfirmation after restart; it is not reported
as lost data or as committed evidence.

Independent media requests use a fixed Main-only transport and the Core's final
outbound projection. Image generation projects text before serialization;
image editing and speech transcription remain explicitly unavailable while
trusted local media projection is absent. Authentication or base64 encoding is
not a privacy projection, and these unavailable operations remain in any
acceptance denominator that originally required them.
In particular, the accepted
[production configuration contract](../../openspec/specs/production-config-truthfulness/spec.md)
retains speech-to-text and Write image generation in their real Electron
scopes. Refusing unprojected audio preserves the privacy boundary but leaves
the speech-to-text functionality gap open. The separate unavailable Go
text-to-speech/music/video tools do not waive this requirement.

Ordinary Write HTML, DOC, DOCX, PDF and rich clipboard exports refuse Markdown
images, including reference, remote and data-URI images, before publication.
Current workspace/frame authority and filename checks do not classify media
content. A synthetic PNG metadata canary reached exported HTML/DOC and embedded
DOCX media before this gate; this is controlled byte-level evidence, not a claim
about real user data or pixel/OCR coverage. Masked text and code examples remain
available. The accepted workspace-image embedding requirement remains **BLOCKED**
until trusted image content projection is implemented and verified; refusal is
not feature completion or a source-exact export authorization.

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
Funds contracts, application tests, native filesystem contracts on Linux and
macOS ARM64, bounded-concurrency full Go suites
(default and `analytix_prod`), locked Python backend tests and four Rust component unit
suites. Private-authority test jobs use `umask 077`, as the local cache helper
does; this does not relax permission validators. Source-set tests explicitly
model Darwin, Linux and Windows instead of assuming the CI host is Darwin.
The isolation-helper tests and complete secure-generation, final-authority,
raw-artifact and persistence packages run on both native hosts before the dependent full Go suites. A failed
filesystem prerequisite blocks those suites and still fails the aggregate; it
never substitutes a focused pass for the full suites. Native metadata failures
report inode flags and xattr counts, not file contents or credentials. These
synthetic filesystem tests are not Keychain, installer or live-provider acceptance.
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

<a id="verification-snapshot-and-remaining-work"></a>
Historical evidence: [development 373–408](documentation-delivery-review-2026-09-09.md#snapshot-d469-development-373).

## Stable development route

Preserve the existing product and the one Go Core. Use current code and required CI for source status, and
[product completion](product-completion.md) for remaining installed/product exits.
Historical stabilization candidates are retained at the links below, with their
original failures and limits.

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

<a id="pr-mode-transition-2026-09-12"></a>
<a id="pr-22-follow-up-persistence-and-ci-attribution"></a>
<a id="pr-22-legacy-source-review-delta-2026-09-13"></a>
Historical evidence: [development 429–1152](documentation-delivery-review-2026-09-09.md#snapshot-d469-development-429).
