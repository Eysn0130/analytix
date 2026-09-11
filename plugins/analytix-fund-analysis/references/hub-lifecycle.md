# Hub Lifecycle

This document defines the release, install, uninstall, rollback, and remount
boundary for `analytix-fund-analysis`.

## Boundary

- Public Agent plugin release must go through Analytix Hub admin-console
  (`https://analytix.top/admin-console`) or the equivalent Hub management path.
- The plugin may ship in two approved ways: bundled inside a new Analytix
  release, or installed/upgraded through Analytix Hub for an
  existing Analytix runtime. Both paths must resolve to an Analytix-owned
  runtime home and a versioned plugin cache.
- The plugin must not self-update, mutate backend schemas, create MCP tools, or
  write customer installation state during an ordinary investigation run.
- The separate `analytix-fund-analysis-marketplace` repository is only a Hub
  package source/staging repository when the Hub publishing backend requires it.
  Pushing to GitHub, creating a GitHub Release, or reaching a Git remote is not
  a public customer release path and must not be treated as installation,
  upgrade, release success, or the only release blocker.
- Customer install, uninstall, upgrade, and rollback must go through Analytix
  Hub or the Analytix plugin page.
- The installed plugin cache, required skill copy, generated marketplace file,
  artifacts, traces, and evidence ledgers must stay under the Analytix-owned
  runtime home.
- Do not write to system Codex hooks, MCP config, plugin cache, global system
  Codex home, or system Codex runtime state.
- `sync-runtime-cache.mjs` is a local verification/remount aid only. It must not create a Hub installation, publish a package, edit customer marketplace state, or become the customer upgrade path.

## Capability Gap Lifecycle

When a running plugin discovers that existing semantic MCP tools do not cover a
repeatable investigative口径, it may do only three things in production:

1. try an alternate semantic MCP path;
2. use Controlled Case Workbench for the current case when the request is
   explicit, read-only, cleaned-scope, bounded, and auditable;
3. return a capability gap and productization target when the controlled path is
   unavailable or unsafe.

The new capability becomes production only after source changes add or update
backend/MCP behavior, schemas, focused skills, registry metadata, eval coverage,
doctor checks, package validation, version identity, and Hub/bundled release
evidence. A workbench result, local remount, Git commit, or runtime cache copy is
not a customer-visible upgrade by itself.

## Publish

Use this only after the local repository is clean enough to tag or release.

1. Verify the local plugin source: `node --check` on touched JS/MJS, Python
   compile on touched backend files, `doctor`, `check-health`, release guard,
   B0, and the full expanded eval.
   Release guard must prove `plugin.json` and `mcp/server.mjs` agree on the
   version, `analytix-fund-analysis-v<version>` points at the current `HEAD`,
   and the worktree is clean before any publish action.
2. Prepare the reviewed plugin package for Analytix Hub admin-console. If the
   current Hub tooling requires the separate marketplace repository, copy the
   package there as a source-preparation step.
3. Run the marketplace validator and package integrity checks for the package
   source used by Hub.
4. Publish or select the package version through Analytix Hub admin-console /
   Agent plugin management tooling.
5. Trigger or wait for Analytix Hub plugin update detection.
6. Do not use `git push`, GitHub Release, or `sync-runtime-cache.mjs` as any
   part of the public publish path.

## Install

Use this for a user or test runtime that should consume a Hub package.

1. Open Analytix Hub or the Analytix plugin page.
2. Install `analytix-fund-analysis` from `analytix-hub`.
3. Confirm the runtime now has a versioned plugin cache under
   `plugins/cache/analytix-hub/analytix-fund-analysis/<version>`.
4. Confirm the generated marketplace entry points to the same Hub version.
5. Confirm the required skill copy exists under
   `skills/analytix-fund-analysis/`.
6. Run `doctor` and `check-health`.

## Remount

Use this only for local RC verification when the same version has already been
installed by Analytix Hub.

1. Confirm the target runtime home is Analytix-owned and not system Codex.
2. Confirm the same plugin version already exists in the runtime cache.
3. Run `sync-runtime-cache.mjs --json` as a dry run.
4. If the dry run only copies files inside the existing Analytix runtime home
   and the runtime skill mount already exists, run
   `sync-runtime-cache.mjs --apply --confirm-local-remount`.
5. Run `doctor`, `check-health`, B0, and the relevant eval suite.
6. If the version is missing, stop and install through Analytix Hub instead of
   manufacturing a cache directory by hand.

## Uninstall

Use the Hub or plugin page to uninstall. A valid uninstall removes the enabled
plugin state and required skill mount from the Analytix runtime. Cache cleanup
may be deferred by the runtime, but a release-quality verification run must not
accidentally use stale cached plugin files.

## Rollback

Use the Hub or plugin page to select the previous known-good package version,
then remount/restart the Analytix runtime and run `doctor` plus `check-health`.
Rollback must not mix files from two versions, and it must not copy files from
the working tree into the rollback version unless this is an explicit local RC
verification remount.

## Release Evidence

A release candidate is not publishable until current evidence proves:

- `doctor` passes with the expected installed version and no stale active
  generated marketplace pointer.
- `doctor` includes the Evidence Ledger traceability contract: supported
  amount/count/account/holder/counterparty/flow-edge/report-claim samples pass,
  while the same samples without source refs fail.
- Release identity is clean: manifest/server version match, the matching
  `analytix-fund-analysis-v<version>` tag points at `HEAD`, and no dirty
  worktree paths remain.
- `check-health` passes against the intended runtime/backend.
- B0 has `hard_diff=0`.
- The functional closure evidence proves the current package, current runtime,
  current case project, source envelope, validation state, delivery state, and
  user-visible output are aligned for the required task families. Historical
  A/B, `full_plugin` scores, golden/oracle runs, or CLI-only direct MCP output
  may be background diagnostics, but they are not release-quality proof.
- Passive non-fund prompts do not route into `analytix_funds`.
- Report review rejects unsupported Mermaid flows, legal overreach, incorrect
  amounts, unsupported account ownership, and missing source boundaries.

## Operator Runbook

Use this order for every future Agent plugin release. The commands may be
automated, but the boundaries must stay the same.

1. Audit the local slice first: git status, current tag, version files,
   release notes, plugin manifest, MCP server version, registry version, and
   generated quality evidence identity.
2. Confirm isolation: no changes under `analytixagent/`, CodexDesktop,
   codex-upstream, system Codex homes, or non-Analytix runtime paths.
3. Run static gates: `node --check` for touched JS/MJS, Python compile for
   touched backend files, `doctor`, release guard, and package-source preflight.
   The doctor run must show the Evidence Ledger traceability contract passing
   before any commit/tag/publish step.
4. Run live gates: backend health, current-case-project deep health, B0, and real
   frontdoor functional closure tasks. The closure artifact must be generated by
   the current tagged identity and must record current case project, runtime cache,
   app-server/backend identity, user-visible output, source envelope,
   validation state, delivery state, and failure classification.
5. If a quality failure is caused by a functional assertion or evidence parser,
   reproduce it with the smallest source-backed task and fix only the assertion
   or parser. Do not change production facts to chase a score. After any
   patch-version bump, install that exact version through the Analytix-owned Hub
   runtime path before generating release-quality functional closure evidence;
   runtime preflight must reject evidence for versions that are not installed.
6. Commit and tag only after the local release slice is clean and all required
   gates have passed.
7. Publish through Hub admin-console or the approved Hub deployment script.
   Verify the public marketplace and install policy report the new version and
   package hash.
8. Install through the Analytix Hub runtime sync path. Confirm the
   Analytix-owned runtime contains only the new versioned plugin cache and the
   required skill copy.
9. Re-run `doctor`, `check-health`, B0, and at least one real Agent UI smoke
   against the installed runtime before declaring the release usable.
10. Record remaining risks separately. A published package can still be marked
    not release-ready if the current evidence fails a hard gate.

## Autonomous Agent Procedure

When the operator explicitly authorizes commit, tag, publish, install, and
Chrome/admin-console operation, the Agent may continue without another prompt
only while all of these conditions remain true:

- The working tree is the intended release slice and has no unrelated dirty
  files outside the plugin or Hub source paths.
- No hard gate is failing: `doctor`, release guard, live `check-health`, B0,
  real frontdoor functional closure, package integrity, public marketplace
  version/hash, and installed-runtime remount health must all pass for the exact
  version.
- The publish action uses Hub admin-console or an approved Hub deployment
  script. `sync-runtime-cache.mjs` may verify an already installed local
  version, but it must not stand in for Hub publish or customer install.
- Chrome may be used for authenticated Hub/admin-console steps only when the
  operator has allowed it in the thread. Any token, cookie, or login state is
  treated as read-only provider state and must not be copied into artifacts.
- If a gate fails, the Agent stops at an honest publishability judgment, lists
  the smallest repair, and does not push, tag, publish, or install a newer
  package to hide the failed evidence.
- After a successful publish/install, the Agent records the commit, tag,
  package hash, public marketplace response, installed runtime path, and
  post-install health evidence in the final handoff.
