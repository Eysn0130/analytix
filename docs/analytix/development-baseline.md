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
Windows-native DPAPI reopen/corruption and descendant cleanup/pipe-drain evidence
remain separate from macOS or cross-compiled tests.
The current required CI has no Windows-native lane. The accepted first Windows
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

The native-filesystem run at `7b8d14d57` established two independent failures:
Linux ext4 fresh private inodes had `FS_EXTENT_FL` (`0x80000`) and no xattrs,
but the validators rejected every nonzero flag; macOS synthetic roots retained
the `/var` ancestor alias, conflicting with strict canonical-path opens. The
corrections recognize only that storage-format bit (all other inode flags and
xattrs remain rejected), and canonicalize the test helper's temporary parent.
The alias regression failed before the helper correction. These changes do not
turn fixture evidence into packaged acceptance; the updated native and full CI
results still decide PR readiness.

At `8fa2265dc`, all five native macOS packages passed in CI and locally; Linux
passed isolation, final-authority, raw-artifact and full persistence (12.29 s).
Its one remaining secure-generation failure was `getdents` returning `ENOENT`
for an already-unlinked held directory. Cleanup now requires exact inode
identity, zero links and platform detachment proof for that case; it never
treats a missing inventory as empty. Same-name replacements remain untouched.
Darwin's ambiguous replacement case continues to fail closed. Parser-only
packaging fixtures now declare resource presence and inject only synthetic
toolchain identity probes, without changing production admission or claiming a
real package build. The separate toolchain contract tests still run.
At `da77ff2fd`, both native filesystem CI jobs passed, including the complete
persistence package; ordinary and production-tag full Go tests remain separate
required checks. Milestone A/B parser tests now use canonical OS temporary
fixtures instead of requiring the Owner's cache mount. A's existing test-module
instrumentation binds only fixture paths and source-relative imports. B's
mocked APFS test explicitly supplies a Darwin platform, with Linux/Windows
rejection tests retained as executable negative coverage. Their local complete
files passed (175 and 34 tests); this is not actual packaged acceptance.
The D-0242 command-availability integration contract requires real macOS
containment. Non-Darwin production adapters intentionally reject protected
process invocation; installing Go on Linux cannot satisfy that contract. The
`macos-integration` Vitest tag routes this unchanged assertion to a required
macOS lane with actual process-sandbox tests. The complementary Linux lane runs
every untagged application test. Neither lane may be omitted from the merge
gate. This classification does not extend the existing test timeout or pretend
that Linux process containment is implemented.
The current-case projector and checkpoint-recovery guards now check the actual
preservation-aware constructor, exact authority arguments, signed journal,
prepared plan/apply pair and scoped callback under the held exclusion. Mutation
tests reject substituted/nil authorities. The focused guards, strict primary
CAS projection tests and semantic apply/observation tests passed. The provider
registry's domain method check also no longer imports the HTTP transport
package solely for the constant `GET`; exact read-only method validation is
unchanged and covered by negative tests. Other architecture failures, including
application-layer filesystem imports, remain unresolved and are not exempted.
The production Go run at `da77ff2fd` still reached the 20-minute runtimeapp
package deadline. Its active stack was in original attachment inventory
revalidation/directory traversal. The earlier local 6.812 s / 6.196 s commands
were exit-zero observations, not verified subcase passes: exact JSON-event
checking exposed the configured cache volume's removable-APFS fixture skip.
Those timings must not be used as behavior or performance evidence. Required
isolated Linux/macOS restart jobs now execute each complete subcase in a fresh
process under the same CPU/memory bounds and existing 20-minute Go deadline,
and report function-only CPU profiles. Exact subtest run/pass and package pass
events are required; zero selections, skips, failures and incomplete output fail
the gate. Full Go coverage and each process's deadline are unchanged. The cause of the long
Linux traversal still requires these measurements; do not infer deadlock or an
undersized budget merely from the suite timeout.
Neither these focused results nor the source baseline imply full regression,
native packaging or formal release readiness.

The subsequent CI at `322b722ee` executed both Linux held-state subcases:
evidence-registry passed in 219.78 s and pii-authorization in 384.30 s, including
the exact JSON-event gate. These prove isolated execution, not the full-suite
deadline or a product latency target. CPU profiles show substantial filesystem,
hashing and allocation work; they do not establish a deadlock. macOS failed
before the same behavior at package inspection. The diagnostic executable now
uses the physical `RUNNER_TEMP` path, preserving strict executable-path checks.
At `c9d946821`, the native sandbox raw-path assertions still failed while the
same read/write/link/child-process/ordinary-work assertions with canonical roots
did not. Production terminal, process and MCP composition already supplies
canonical protected roots. Both process test packages now use isolated canonical
temporary/home/configuration roots, with a regression for existing and absent
protected descendants through an aliased ancestor. Full local processsandbox
and process packages passed; remote confirmation remains required.
The package-inspection failure persisted with a canonical `-o` destination:
Go 1.26.4 executes its temporary build target, not that copied output. An aliased
`GOTMPDIR` reproduced the exact failure locally; physical `GOTMPDIR` passed the
new current-executable inspection test. CI now sets the actual build root and
executes this precondition alongside the held-state test.

That Application run reported 6,161 passes and four failures. Two actual
`/bin/zsh` parent-owned test contracts now join the required macOS integration
lane with unchanged deadlines. Missing cache configuration in the Milestone B
dry run is explicitly BLOCKED, still non-passing and nonzero when gated;
an invalid configured relative path remains FAIL. The oversized-source test
uses exact `Buffer.equals` instead of object-property traversal, retaining
byte/length equality and all no-side-effect checks. The focused 13-case check
passed; parallel local testing also exposed six settings timeouts, while its
isolated full 69-case run passed. Do not hide that resource/isolation evidence
behind a timeout increase.

Attachment creation-residue recovery fixtures now freeze the same original
inventory as production startup before entering recovery. The original failure
was reproduced; all six owner/leaf/shard preservation and independent-write
subcases then executed and passed. Production recovery validation is unchanged.
Registry architecture guards now distinguish the sole settlement producer from
its exact input/error-preserving shared-owner forwarding method, enforce the
current-owner lock lifetime, and bind import activation and finalization to that
same owner. Mutation checks reject changed arguments, owner selection, errors
and lock release. These focused guard passes do not waive the other architecture
failures; the local activation integration remains unverified because its
non-removable-APFS prerequisite causes a skip, which the exact-event checker
correctly rejects.

At `c9d946821`, Application CI subsequently passed **6,163 tests** (18 existing
or native-lane exclusions), and both Linux held-state contracts passed again.
The accumulated runtimeapp package deadline is now handled by four dynamically
discovered partitions per Go build mode. Every discovered Test/Example/Fuzz name
belongs to exactly one partition; no manual name list can omit new tests. All
other Go packages remain in the complementary package job. Every partition is
required by Development gate, retains the 20-minute process deadline, and
propagates nonzero exits/signals. The existing separate exact-event held-state
gate still rejects skips. Unit tests prove disjoint/full partitions and failure
propagation; actual local inventories were partitioned for both build modes.
This is coverage-preserving orchestration, not a full runtime PASS: remaining
implementation/fixture/architecture failures must still be resolved.

### PR #22 follow-up: persistence and CI attribution

CI run [34687727224](https://github.com/Eysn0130/analytix/actions/runs/34687727224)
at `b67058578` passed Application, Backend, all four Rust components,
Linux/macOS filesystem contracts, and **all four exact held-state restart
jobs**. macOS evidence-registry and pii-authorization executed in 310.95 s and
572.10 s respectively, with the no-skip JSON-event checker. The physical
`GOTMPDIR` correction is therefore remotely verified. That run still failed:
Source baseline found ShellCheck's masked-assignment rule, and macOS process
integration exceeded the Unix socket pathname limit under the long default
temporary parent. Separate assignment/export and a short physical `TMPDIR`
correct those fixture/orchestration causes; remote revalidation is required.

Four runtime partitions were insufficient: after 20 minutes ordinary shards
0/1 were at top-level entries 45/92 and 37/91, with the active tests only 20 s
and 9 s old. This is cumulative load, not proof those tests deadlocked. The
candidate uses sixteen complete dynamic partitions per build mode, at most
four independent runners concurrently, retaining `-p 1`, `-parallel 2` and
the original 20-minute Go deadline. Verbose execution exposes individual
durations. Partition completeness/disjointness, matrix alignment, invalid
indices and failure propagation pass five local tests. Both actual macOS
inventories partition completely (367 ordinary / 376 production names);
Linux selection remains platform-native. This is not yet a runtime PASS.

The history mutation failure was a real concurrency defect: benign terminal
appends may coexist with usage-index rebuilding, but compaction and rewind
must not rewrite history across that barrier. Commit `91ea62157` rejects
destructive rewrites under the same owner lock before effects. Original
history/event equality assertions remain; rewind rejection and a successful
post-rebuild retry are covered. Focused ordinary tests passed; production
compaction/rewind/terminal regression passed (69.803 s), the complete usage
index package passed (21.123 s), and the combined race check passed (28.666 s).
No product data directory or real Provider was used.

Commit `f813f41c9` makes the two CAS contenders take their preflight snapshots
before release, so both actually compete at the intended CAS boundary. All
winner/loser/disposition/restart assertions remain. Five ordinary and three
production repetitions passed; sequential competing admission remains denied.
Diagnostic tests now inspect the retained private error cause and assert the
exact closed public error separately; no public diagnostic exposure is added.

Provider-authentication regression was reproduced with an empty Registry:
legacy configuration does not authorize execution under the accepted
`local-provider-credential-authority` contract. Reusing the explicit synthetic
Registry Connect/readback fixture restores the actual loopback Provider call
and preserves the original structured-error/redaction assertions. The fixture
uses its isolated file-backed encryption authority, not the Owner Keychain.
Authentication plus existing lightweight-prompt integration passed locally
(116.890 s). Do not automatically grant credentials to every test: negative,
multi-provider and restart fixtures require their own explicit state contracts.

The deferred-registry activation and body-free report recovery guards now bind
the actual prepared closure/visitor, with mutation tests rejecting substituted
owners and readers. The complete architecture package at `839ead4d7` plus the
Provider fixture worktree still reports **11 failures**. Actual application
filesystem/ownership boundaries and stale lexical guards must be resolved
individually. Root/server/migration failures remain, and PR #22 is not mergeable.
Native installer and isolated packaged acceptance follow a genuine Development
gate; they are not implied by these focused results.

At `7d3e60a0f`, both Linux exact held-state jobs passed again (218.41 s /
282.41 s) after removing the unnecessary restart-lane `TMPDIR` override.
Canonical `GOTMPDIR` remains, and the short `TMPDIR` applies only to the native
socket lane. This establishes the bounded environment correction, not a claim
that all storage locations have equal performance. Source baseline and both
filesystem lanes passed. Full runtime/architecture regression is still open.

The macOS native processsandbox/process packages now pass remotely. The two
parent-owned Milestone A tests initially failed because their stderr digest
exactly matched the Owner cache backing-volume rejection: a synthetic parser
fixture still sourced the real hardware-specific preflight. Commit `a638a2734`
supplies a test-only sourced preflight bound to the parent's disposable
home/temp/cache, including exact exit-1 negative tests. Production preflight
and acceptance classes are unchanged. The complete local file passed 176 tests;
all four native Vitest cases passed remotely, but that job remained failing
on an unhandled AppShell dynamic import during teardown. A tag-excluded suite
does not run its `afterAll`; collection-owned preloads must settle before its
environment disappears. The test now explicitly awaits its own preloads during
collection, with deterministic pending/ready route fixtures instead of a race
against module-cache speed. Original loading/layout/boot assertions remain and
a ready-state check was added. All four tests passed locally; tag-excluded
collection completed without unhandled imports (not a behavior PASS). No errors
are caught or suppressed. The complete local native-tag lane then passed all
four selected tests (213.39 s), including the real D-0242 contract, with no
unhandled errors. Other cases remain covered by the complementary application
lane. This lifecycle correction still needs current-head remote verification;
four passing cases alone never
justify ignoring an unhandled error.

Commit `ab5a5ec08` also passed the three original tool-scope, invalid-model and
stale-thread-model tests (129.463 s) after explicit synthetic Registry setup.
Invalid models still cause zero Provider requests and the exact structured
failure. Existing capability/security assertions were not relaxed.

One public historical secret-scanning alert identifies a key-shaped negative
test fixture. The current fixture now constructs an explicitly synthetic
canary and retains its non-disclosure assertion. That does **not** prove the
previous literal was never issued, remove its public historical blob or justify
dismissing the alert. Resolve original credential provenance/rotation separately
without printing or exercising the value. No real Provider check is part of
these deterministic development tests.

PR #22 final-closure work began from `82e055026` with a clean canonical
worktree and matching remote PR HEAD; main was `58b365b3b`. Development run
`34690548012` passed Source baseline, Application, macOS process integration,
Filesystem and Backend lanes, but still had Go/Runtime failures. Those partial
results do not admit a merge.

The private-CAS constructor guard incorrectly treated a slice of existing CAS
handles as constructing authority. It now checks AST construction, including
negative cases for direct, pointer and nested implicit container literals.
The original-residue opening method was consolidated unchanged into the sole
CAS constructor owner, retaining access, binding, inventory and generation
checks. Local `go test -count=1 ./internal/architecture -run
'TestPrivateCAS|TestEveryRuntimePrivateCAS'` passed (1.585 s); the complete
`internal/adapters/outbound/finalauthority` package passed (183.916 s).
This closes that focused guard/ownership failure only; full architecture,
current-head remote CI and merge acceptance remain open.

Checkpoint restart preservation now delegates host path normalization and
physical-root alias overlap to its existing filesystem observer port. The
application retains the held-thread and recovery-source decisions. Symmetric
exact/parent/child overlap, adjacent-name exclusion and physical aliases have
focused coverage; complete local checkpoint and filestore packages passed
(0.469 s / 28.955 s). No overlap rejection, recovery budget or architecture
import constraint was relaxed.

Provider legacy-migration physical source reads now belong to the filesystem
adapter behind a read-only port. The Manager still owns one-use challenges,
source/owner equality and registry generation; absent source-reader authority
fails closed. The adapter retains bounded reads, physical identity before open,
single-link regular-file checks and before/open/after identity checks. Local
migration Manager checks passed (3.597 s); new adapter physical-boundary cases
passed (3.739 s), and the full adapter package passed (505.542 s). A concurrent
full Manager run timed out after 10 minutes in filesystem crash-point recovery;
process sampling showed sync I/O, so this is not recorded as full-package PASS.
No timeout or required CI coverage was changed. Current-head remote closure
remains required.

Funds CSV admission now receives its diagnostic writer from composition; the
application no longer accesses global process stderr. Its error identity and
stage/class diagnostic regression, and complete local admission package, passed
(0.641 s). Existing accepted-final observation/turn equality is now an error-only
domain validator, preserving the strict primary reader as the observation
producer. Complete local domain/evidence and app/evidence packages passed
(1.465 s / 48.723 s), including existing negative bindings.

Server active-history and child observation now use the immutable primary reader
bound by runtime composition, instead of constructing finalauthority in server.
The restart fixture binds the same real reader on reopen. Local active-history
regressions exposed that missing fixture binding; after correction, all four
compaction commit/crash-cut/absence cases passed (40.40 s), and stored-child seed
error preservation passed (7.83 s). Historical child lookup exposes a read-only
resolver using the original terminal validation and cloning, with no mutable
projection API. The marker-stripped source fixture now requires derivation
rejection, unchanged source/lineage/inventory and all original privacy checks;
its focused server test passed (1.35 s). These are local contract/recovery checks,
not a full server or runtime-suite acceptance.

Provider endpoint protocol-shape classification is now a pure model value
function shared by application and transport. This removes the loop's reverse
dependency on the legacy Provider adapter while preserving request URL parsing
semantics. Complete local loop/model/compat packages passed (20.416 s / 0.580 s /
0.394 s). Full architecture was still failing on the separately tracked media
HTTP composition and private report-owner references at this point.

Media execution now receives a transport factory through a narrow port. HTTP
client/request construction, committed proxy handling, bounded timeouts and
redirect refusal belong to the outbound adapter. Application-owned Registry
revalidation remains before send, after headers and after bounded body reads;
credentials are cleared and image downloads never receive the Provider header.
All existing loopback media tests and the new adapter transport tests passed as
complete local packages (0.490 s / 0.399 s). No live Provider was contacted.

The complete local architecture package now passes (12.008 s). Real violations
were fixed in their owners: private controlled-access outcome parsing/matching,
report settlement/history interpretation and restricted dataset-context matching
stay inside their application owners; runtime composes only bounded read-only
interfaces. Full local reportpublication/datasetsnapshot/piiauthorization
packages passed (5.943 s / 0.748 s / 0.842 s), retaining existing negatives and
adding invalid/missing/duplicate outcome-inventory coverage.

Stale guard assumptions were replaced by narrower structural checks, not
exemptions: original inventory observation requires its read-only prepared
physical bracket; copied-stage observation requires frozen roots and complete
Close/error propagation; historical report composition cannot gain current
capabilities or escape semantic validation. Mutation negatives exercise those
violations. Go test fixtures are excluded from the production dependency graph
using Go's exact _test.go rule; production files and alias imports still fail
negative tests. The old server line/file cap already failed at public baseline
`eed7dfb1a` (56 files / 10037 lines against 54 / 8649); it is replaced by exact
adapter-construction ownership, including alias/indirect/new/literal and duplicate
construction negatives. Other server layering guards remain. This is a local
architecture PASS only; current-head remote Development acceptance is pending.

Case turn snapshot admission now checks the composition-owned Registry's local
availability before contacting shared witness authority. The confirmed-import
owner retains its underlying snapshot capability and remains the only activator.
Restart fixtures explicitly seed synthetic protected Provider Registry authority
for ordinary loopback turns. Six focused zero-generation/partial/non-JSON/corrupt/
bound-sibling witnessed cases passed locally (151.68 s), retaining zero-witness
and preservation assertions. Fresh import activation passed (6.76 s); real CAS
readback-fault and exact-operation suites passed (13.57 s / 41.47 s). Combined
focused run passed (214.246 s), no skips. An earlier entire witnessed group ran
into its unchanged local 10-minute budget after reaching the last subcase;
that run is partial, not full PASS. Sampling and sequential progress indicate
cumulative filesystem cost; no timeout or CI partition was increased.

The two server child-delegation contracts used a stale synthetic source
capability lacking the prepared Registry effect surface. Their fixture now
binds one real signed legacy-store commit to its callback, exact source/context/
selection/prepared/marker and signed readback. No-op, repeat, mismatch, cancellation,
revocation and escaped-lease negatives remain fail-closed; this is synthetic
legacy public-contract evidence, not V2 shared-witness acceptance. A concurrent
local run subsequently missed parent continuation near its existing task budget;
no timeout or assertion was changed. Stable serial ordinary and DeepSeek public
contracts both passed (41.73 s / 43.09 s), including parent=3/child=1, typed handoff,
durable restart and all original raw/private non-disclosure checks. Primary
reader missing/rebind/cancellation coverage passed (1.73 s); combined local
server selection passed (87.203 s). Full current-head remote server/package
acceptance remains required.

Thread listing now excludes only explicitly qualified restart-held scopes before
public snapshot/cursor reads; independent read failures still propagate. The
complete local thread application package passed (2.212 s), and the full varied
inventory ordinary HTTP/witness-outage test passed across two restarts (319.88 s).
Signed report-attempt fixtures without original primary/enrollment now assert
their actual earlier authority rejection instead of expecting a later recovery
gate. Reserved and corrected aborted cases passed (24.86 s / 26.97 s); all three
Core-linkage negatives passed (66.53 s), and the genuinely enrolled reserved
history retention test passed (0.48 s). The combined initial selection failed on
the old aborted assertion; its corrected focused rerun passed. These are local
focused results, not full runtime acceptance. Remote run 34693285203 independently
reported a plan-turn 500 in the Ubuntu native PII held-state lane; investigation
continues before merge admission.

The CLI private-frame HTTP contract now distinguishes a cold Registry from a
real committed historical Registry. The latter is provisioned through signed
prepared history, actual Registry CAS and the enrolled shared witness, with
matching durable turn/grant/result/epoch and case-thread authority. Cold startup
and admission remain local; committed startup performs read-only reconciliation,
and an outage on the subsequent case HTTP request reaches the exact shared
namespace without advancing it. Both retain typed zero-fact source-unavailable
output and the private-frame/ambient/ready non-disclosure checks. The subprocess
uses one isolated filesystem and its actual TMPDIR for authority and temporary
durable owners; compiler caches remain external. The existing 30 s ready budget
is unchanged. Final local focused execution passed both scenarios without skip
(cold 3.33 s, committed 5.17 s; package 9.194 s). This is synthetic CLI contract
coverage, not native dataset, Provider, packaging or full remote acceptance.

Remaining runtime contract fixtures now reflect their actual producer seams:
CSV materialization fields match the native producer's flattened wire contract;
late-failure startup cases keep the earlier child identity inventory valid and
inject an invalid lifecycle in the later migration phase; inherited legacy final
display is taken from the real source finalizer before testing sealed read-only
preservation. Publication negatives assert the current exact original bytes,
mode and absence rejection, preserving all mutation and independent-write checks.
Local individual results passed: CSV composition 1.93 s, pending-work late failure
4.19 s, final-event late failure 7.95 s, seven publication mutations 6.11 s,
inherited final readers 7.74 s, and signed-added-dependency rejection 4.21 s.
The two combined selections also included an unfinished CASE fixture and failed
on that separate case; they are not full runtime PASS. The intermittent async
second-restart equality test passed locally (45.56 s) with bounded metadata-only
failure diagnostics; its remote failure remains unresolved until current-head
CI establishes the cause and result.

The intermittent plan failure has a deterministic privacy-projection defect:
a valid RFC3339Nano savedAt with seconds 13–19 and nine fractional digits is
classified by the general prose scanner as an eleven-digit phone number. Seven
fixed inputs failed against the original event projection despite valid typed
PlanStatus. The existing exact host-bound, canonical closed-plan metadata path
now preserves validated savedAt alongside its SHA-256 contentHash. General text,
lookalikes, invalid timestamps and phone-bearing display fields retain their
original privacy rejection. All seven regressions and negatives passed; the
whole local event domain package passed (0.341 s). Remote failure timestamps
fit this mechanism, but current-head runtime and native PII CI are still needed
to establish remote closure. No timeout was increased and no raw-error diagnostic
was added to HTTP, persistence, model context or production logging.

The CASE terminal-closure fixture now starts with a real nonempty witnessed
Registry and its exact durable originating turn, signed preparation, grant,
result marker, epoch and case-thread authority. Counts distinguish that original
history from the single new terminal chain and prove the historical records are
unchanged. The complete local candidate/fixed/longitudinal test passed (381.97 s):
each actual failure phase drains, recovery authenticates the original winner,
and same-thread ordinary continuation preserves original CAS/private bytes.
This result covers an already committed Registry; first-import/restart continuity
with an empty Registry remains a separate contract under review. The combined
selection still failed on desktop TypeScript retirement before its later owner
migration, so no full-runtime or desktop-migration PASS is claimed.

Checkpoint preflight no longer interprets a failed Git subprocess as a clean
index. The filesystem/process adapter proves a non-Git workspace by a complete
ancestor inventory; repository probes retain containment and propagate a fixed
unavailable result on lookup, configuration, containment or execution failure.
The consumer blocks mutation on unavailable or missing probes. Local focused
process checks passed (1.098 s), including actual contained staged detection and
protected configuration denial; the unsupported-containment case belongs to its
non-macOS build and awaits Linux CI. Filestore mutation/UTF16 checks passed
(0.690 s). The macOS HTTP staged checkpoint contract passed (50.57 s), with target
and Git index byte equality. No unrestricted Git fallback or safety waiver was
introduced; Linux protected repository probes remain explicitly fail-closed.

G5 Go conformance now uses the actual product display name Analytix and the
current useSpeechToTextEnabled reader already enforced by the TypeScript source
scanner. Runtime/package machine identity remains analytix. The facade, source
implementation and evidence requirements are retained; four new negative
mutations reject direct facade, missing source, retired reader and absent
source evidence. Full G5 control executable comparison and those negatives
passed locally (0.662 s). The preceding root selection exposed the stale reader
name after its other four current Registry/Git contracts passed; that combined
selection was FAIL, not full application or Go acceptance.

The committed Provider resolver now retains the desktop's key-free tariffs,
reasoning protocol and aliases only for the exact Registry-admitted route and
canonical models. The Registry remains the sole credential, selection, endpoint
and executable model authority; legacy profile capabilities are not copied.
Ambiguous aliases or duplicate attributed routes fail closed. Deterministic
metadata regressions failed before the fix; the production-mode resolver/pricing
selection passed afterward (16.280 s, no skip). Public HTTP regressions passed
for four-provider pricing (81.88 s), cache accounting (58.07 s), caller Provider
IDs unable to override selection (40.93 s), exact-attempt DeepSeek reasoning
(57.16 s), subagent model/effort/tool scope (52.92 s), selected Xiaomi (64.23 s)
and its canonical alias (64.12 s). These fixtures perform actual protected
synthetic onboarding and explicit selection through the Registry HTTP contract;
plain unavailable-authority constructors remain unchanged. Full root-package
and current-head remote acceptance have not yet passed.

First-import continuity is a separate, precisely bounded next-stage seam.
Current confirmed admission persists DSV2 and activates an in-memory Registry
service without requiring an evidence receipt. Before the first receipt,
restart intentionally leaves the generation-zero Registry dormant. The normal
stage/confirm entrypoint can admit a new source revision and reactivate it; that
is not automatic restoration of the prior activation. Code review found no
accepted receipt-free activation continuity requirement that authorizes
removing the current fail-closed gate. No first-import/no-receipt/restart
acceptance is claimed here, and the committed-Registry closure fixture above
must not substitute for it in isolated-development or packaging acceptance.

Runtime shard admission now validates streamed Go JSON execution, in addition
to deterministic inventory partitioning: every selected test and observed child
must run and pass exactly once. Empty, missing, duplicate, foreign-package,
skipped, failed and incomplete execution is rejected. The five synthetic
ordinary lifecycle faults move to a required macOS lane with a current-source
schedule entrypoint; that lane also executes the contained schedule contract
in both build modes. Its ordinary-mode root selection covers ten exact stdio,
Git and Bash contracts, independently of the Linux package inventory. The
successful-stdio contract is compiled for Darwin, where containment is supported;
the unsupported-host process rejection and Linux Bash behavior remain covered.
All receiver names must exist in the actual discovered inventory. The same
20-minute Go budget and sixteen runtime shards remain in force.

Four pre-existing explicit external diagnostic entrypoints are classified before
execution and logged as not_executed, never as required PASS or allowed skips:
TestRuntimeOptionalPublicFixturePathV1, TestRuntimeOptionalPluginPublicConsumerV1,
TestRuntimeOptionalPluginUnauthorizedFirstMCPTerminalDiagnosticV1, and
TestLocalNonPublishablePackageInspectionWhenExplicitlyProvided. They require
Owner task/corpus/diagnostic authority or an exact packaged executable/data.
Missing schedule build inputs are not part of this exclusion. The new platform
job belongs to the same required Development gate. Root-reviewed Node partition,
receiver and gate regressions passed (8/8, no skip); real platform and complete
remote execution are still pending. CI admission is stronger, not relaxed.

Root HTTP tests that actually execute a synthetic Provider now explicitly
onboard it through real Registry revision/incarnation CAS and protected secret
ingress. The opt-in fixture rejects non-loopback endpoints and never repopulates
an existing Registry on restart. Missing-authority tests retain their plain
constructors. Provider selection tests perform the real select operation;
caller/profile Provider IDs cannot create a second execution authority.

Live turn, multi-model and goal-lineage tests now create real threads rather
than reuse a G2 static completed turn lacking current terminal credentials.
The five-family replay/cache/atomic-terminal test passed (107.84 s). Goal lineage
is produced by the actual terminal-backed goal completion path, not a fabricated
event; its exact goal/thread/turn binding and private projection passed (48.85 s).
The turn-field contract passed (39.29 s), and real contained stdio startup/catalog
plus mutation-authority rejection passed on macOS (37.89 s). Subagent execution
keeps the admitted model/variant/source fingerprint but must not persist Registry
endpoint authority; the positive HTTP/route/absence test passed (47.68 s), and the
unselected-Provider zero-call negative passed (49.35 s). Earlier combined runs
failed on stale route-field assertions and fixture cardinality; they are not
full package PASS. Complete new-head remote root coverage is still required.

Desktop retirement now recognizes exact-witnessed legacy TypeScript parents
whose original product storage contains only metadata/messages sidecars. The
ordinary primary inventory remains strict. Only the desktop retirement path
carries that bounded audit input into the semantic stage, where the normal
recovery/upsert path materializes a canonical primary before strict inventory
and signed publication. Invalid or missing lineage is checked before initial
authority directory creation and repeated during semantic admission. A new
negative initially exposed two housekeeping directories created before rejection;
the earlier witness fixes that mutation instead of weakening whole-tree equality.
The final complete desktop-migration selection, strict/wrong-family/unwitnessed
inventory checks and both invalid-lineage no-mutation branches passed locally.
Current-head remote runtime acceptance remains required.

The first stronger-admission remote run at 7099f0015 exposed two additional
issues in the newly required native lane. The schedule HTTP fixture still had
legacy configuration without admitted Provider Registry authority; it now uses
the same real synthetic onboarding fixture before composition. Its production
focused contract passed locally (7.47 s), with current-source Node entrypoint.
The workflow command substitution now separates assignment from export so
actionlint can observe failures (SC2155); pinned actionlint 1.7.12 passed.
Remote Application, production non-runtimeapp package tests (including
architecture and server), and completed native contracts passed at 7099f0015.
That SHA is not a complete required-CI PASS: source lint and both runtime-platform
lanes failed. Both platform runs exposed a one-shot inherited replay timeout
after fork/restart in the missing-optional-plugin scenario; the other four faults
passed remotely. This timeout remains under investigation without a budget
increase. A local external-filesystem run separately exhausted the unchanged
20-minute cumulative package budget after three complete faults; this is not
a full lifecycle PASS. Synthetic runtime profiles may use isolated native temp
roots while compiler/dependency storage remains on the configured cache volume.

At 7099f0015 the complete ordinary root inventory executed all 321 tests:
312 passed and nine failed, with zero unstarted tests. Eight failures were
remaining contract/fixture drift. Attachment and research positives referenced
an uncreated historical workspace; they now create real isolated workspaces
and threads. Attachment restart reuses the same durable root and owner identity.
These three public tests passed locally (3.91/3.74/8.46 s). The public-final
checks now require V3 acceptedFinalView, reject private acceptedFinal in public
records, and validate complete durable publication slots and payload digests.
Cross-parent continuation retains the closed validation_error classification
and all non-disclosure/zero-child-call negatives. Those four tests passed
locally (4.80/3.66/3.81/8.88 s).

Vision capability configuration is an intent/limit fixture, not executable
authority. Its legacy route cannot establish Registry media admission or the
still-unimplemented trusted local image privacy projector; runtime-info must
remain unavailable and withhold that route. The corrected negative passed
(3.27 s), retaining enabled intent, probe metadata, budgets and secret/endpoint
redaction. No working image/vision path is claimed or implemented in this CI
closure. The remaining stale-child startup fixture and native lifecycle cost
are still being resolved; these focused results are not full remote PASS.

All ten exact native root process contracts passed locally (43.086 s, no skip).
The missing-optional-plugin lifecycle completed locally in 347.894 s with CPU
profiling. Its profile attributes 209.33 s cumulative CPU to pending trusted
inventory and 193.60 s to individual secure receipt reads. Snapshot inventory
already reads all receipts twice; disposition validation redundantly rereads
each receipt and rescans the protected CAS tree. The pending-store fix is being
validated at that owner boundary, without caching across requests, changing
replay authority checks, or increasing the existing timeout.

The ninth ordinary-root failure was an unsigned orphan child fixture. Current
Core startup intentionally rejects that inventory before recovery; treating it
as an accepted stale child would bypass signed producer association. The same
fixture now exercises the error-returning constructor and proves exact
ErrChildProducerInventoryIncomplete, unchanged preserved root contents/modes
and zero Provider requests (0.44 s). It does not manufacture a receipt or reopen
a completed job. The existing legitimate producer-allocation, bound parent
recovery, and stale/lease-expired/orphan recovery endpoint tests were separately
executed and passed (server selection 1.980 s). This is constructor-negative
and focused recovery evidence, not a new crash-process acceptance claim.

Pending inventory now reuses only the already-validated receipt set from the
same observation. Both real CAS listings, both ordered observations, root/store
locks, exact disposition-to-receipt binding and changed-inventory rejection
remain intact. Ordinary ListDispositions still performs its original reads.
The new access-count regression failed before the change (four receipt CAS
accesses at one disposition versus two at zero) and passes afterward with a
constant count through four dispositions. Missing, mismatched and duplicate
receipts, deletion between observations and existing concurrent append tests
pass; the complete pendingworkstore package passed (10.192 s).

Under the same isolated native-temp profiling setup, the complete missing-fault
lifecycle fell from 347.894 s to 141.900 s, with every assertion retained. The
full production platform partition then passed (391.129 s): current-source
schedule contract plus all five lifecycle faults, admitted by the exact JSON
execution checker with no skipped or missing test. Broader local pendingwork,
thread, casethread and architecture packages passed. The whole server package
run failed a pre-existing blanket /private/ substring assertion when its
ordinary fixture workspace used /private/tmp; that is being checked separately
and is not reported as a full server PASS.

Remote ordinary runtime shard 8 at 7099f0015 exhausted the unchanged 20-minute
package budget after successful tests; its active cancel_after leaf had run
only one second. The largest completed test was the joint held-owner ordinary
HTTP test (639.80 s), plus 144.30/140.73/119.05 s tests in the same partition.
The held fixture has only one pending receipt/disposition, so the lifecycle
optimization cannot be assumed to solve this separate cumulative cost. A
focused held/pii profile is in progress before any further orchestration change.

The local server privacy failure was a boundary mismatch in the assertion:
LoadEventsSince returns internal events including the legitimate ordinary
workspaceRealPath. A blanket /private/ match therefore classified the isolated
/private/tmp workspace as an error leak. Durable replay still rejects the exact
fault canary and full persistence root. Public privacy assertions now also run
the actual production projector, retain the /private/ restriction, reject
workspace/securityContext disclosure, and require a visible turn_started event
so withholding everything cannot satisfy the check. All nine original
success/failure/cancel by archive/events/usage cuts passed (package 3.008 s).

The held/pii profile completed successfully (369.11 s test, 368.25 s profile
wall time). It attributed 274.78 s cumulative CPU to original-create residue
revalidation, including repeated protected filesystem observations. Those
currentness checks cannot be removed without a separately proven owner
contract. This is distinct from the corrected pending receipt scan.

All eight tests cut off or not started in the remote ordinary shard 8 run
passed in a fresh local exact-selection run (52.737 s, no skip or missing test).
The create-residue recovery test took 7.09 s. These are focused local results,
not a completed remote shard. Existing remote timing samples cover 360 of 368
required top-level tests; the remaining eight now have local evidence, which
cannot predict identical hosted-runner timing.

The CI partition correction reserves shard 15 for the exact held-owner test
and distributes all other automatically discovered required tests across the
remaining 15 shards. Both build modes, all 32 required runtime jobs, concurrency
and the 20-minute package budget remain unchanged. Missing dedicated identity,
empty selection, skipped/incomplete execution and child-process failure still
fail closed. Eight orchestration regressions passed, including full inventory
coverage, disjointness, deterministic ordering and minimum inventory boundaries.
Actual current-SHA remote execution remains required; this scheduling correction
does not establish a product startup-latency improvement.

After the public-projector assertion correction, the complete local server
package passed on the stable source candidate (139.422 s, ordinary mode,
-count=1 -p 1 -parallel 2 -timeout 20m, isolated native synthetic temp profile).
This closes the nine local closure-privacy failures without claiming a remote
or production-mode rerun from this local result.

## PR 22 legacy-source review delta (2026-09-13)

Review-conversation inspection found eight CodeQL path-flow annotations
(129–136) in the legacy Provider source adapter. They share one request path
flow. Application authority consumes an exact one-use challenge bound to the
retained recovery before the adapter runs; the adapter checks device/inode,
single-link identity and the canonical physical-path digest, and the application
then checks source bytes/hash and the complete owner challenge contract.
Those custom constraints must be considered when interpreting the static flow;
no scanner exclusion or broad sanitizer is added.

The review also identified a concrete open/read race: the old helper opened a
path normally and checked the opened object's identity only after reading.
Replacing either source or owner with a FIFO after observation blocked the
read. A bounded subprocess regression reproduced both failures before the fix
(5-second deadline each, child reaped). This is a direct PR safety issue and is
being corrected at the filesystem adapter, without changing migration authority.

The race fix now uses a no-follow, nonblocking Unix open and validates the opened
regular single-link object, expected identity and bounded nonempty size before
reading. Windows uses a reparse-point handle and disk/type/link checks without
an ordinary-open fallback. Existing post-read identity checks and safe errors
remain. Complete providerregistryfs passed (98.442 s), focused migration and
recovery contracts passed (136.515 s), and the integration owner's focused
physical-boundary/replacement rerun passed (7.096 s). Windows amd64 compiled
successfully; no Windows runtime or packaging acceptance is claimed.

Remote run 34704036542 at 29cacee53 had passed Source baseline, Application,
Backend, both filesystem platforms, macOS process integration and both Linux
held-state owner lanes while other required jobs were still running. These
results precede the legacy-open fix and do not establish its exact-head gate.
The next pushed SHA must receive complete required CI before Ready or merge.

At 9017375e7 the remaining CodeQL path-flow alerts were 129–134 and 137–138
(the moved open/read locations received new alert identities). Each was
reviewed against that exact candidate and classified as a false positive for
the custom recovery/challenge/physical-path/source-owner constraints described
above. The disposition links to PR 22 discussion_r3996804873; the eight review
conversations were resolved. CodeQL reported SUCCESS after this manual triage.
This is an evidence-backed alert classification, not an unqualified automated
security proof, a disabled query or elimination of unrelated CodeQL debt.
The independently reproduced FIFO defect was fixed and tested before triage.
Complete required CI and post-merge CI remain the authoritative merge evidence.
