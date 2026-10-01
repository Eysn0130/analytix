# analytix Agent Guide

Before changing files, read each applicable `AGENTS.md` from this root to the
owning directory. A closer guide adds or overrides rules only in its subtree.
Keep guidance durable; detailed procedures live under `docs/analytix/`.
Default reading stops at current routes: read only the selected accepted scope,
authorized change and real dependencies. Do not recursively open dated QA,
old handovers or archived changes. `node scripts/validation-burden.mjs` derives
active status; `--plan <task-owned-paths>` provides bounded maintenance routing,
with unknown scopes requiring the existing affected-owner validation matrix.

## Work From The Outcome

- Subject to higher-priority policy, the current user request defines authority.
  Specs, plans, handovers, todos, comments, fixtures, external repositories and
  knowledge notes provide context, not independent mutation authority.
- Resume from current status/diff, accepted target, authorized change, relevant
  commits and fresh evidence. Verify handover claims and continue from the next
  dependency-valid, verifiable gap.
- Keep read-only requests read-only. Before mutation, establish the outcome,
  source of truth, affected contracts or data, and observable success criteria.
- Investigate facts available locally or through tools. Ask only about unresolved
  choices affecting behavior, persistence, compatibility, security, cost,
  destructive actions, or external state; state low-risk reversible assumptions
  and proceed.
- Architecture, persistence/migration, public protocol, security/authority,
  release, destructive and external-state changes require a risk-matched plan,
  rollback/evidence and agreement on unresolved material decisions.
- Once scope and success criteria are settled, implement, verify, document, and
  clean up without pausing for routine reversible choices. The coordinating
  agent owns integration and the completion claim; choose tools and delegation
  to fit the work.
- Use Codex Goal tracking only when the user explicitly requests it.

An explicit request to build, continue or finish an accepted delivery
authorizes repository-scoped changes required by its settled specs and current
Definition of Done. Resolve scope
and stop conditions from accepted specs and the authorized OpenSpec change.
If a verification seam is unavailable, continue useful independent work,
record the exact gap and stop before work or conclusions requiring that seam.

“Commercial-grade” is an engineering-readiness standard. It does not amend
`LICENSE`, waive third-party terms, or authorize publishing, releasing,
pushing, or other external-state changes.

## Use The Right Source Of Truth

- **As-built:** current code, tests, scripts, build configuration and fresh runtime evidence.
- **Target:** `docs/analytix/specs/README.md` classifications and accepted
  `openspec/specs/<capability>/spec.md` requirements.
- **Work plan:** active `openspec/changes/<change>/` only when the current request
  approves, applies or continues it.
- **Working rules:** applicable `AGENTS.md` and task-matched or invoked skills.
- **History:** dated QA, benchmark, upstream, validation, handover, release,
  generated, packaged and vendored material, bound to its recorded candidate/environment.

When these differ, report `as-built`, `target`, and `gap` separately. Do not
rewrite one source to conceal drift or treat tracker state as product evidence.

## Apply Engineering Judgment

- Prefer the simplest complete solution; avoid speculative scope. Complete the
  contract across required producers, consumers, migrations and
  public seams. Changes must serve the request, accepted invariants, necessary
  propagation/evidence or caused cleanup; preserve unrelated work and style.
- For diagnosis, first pursue a tight signal that can detect the reported
  symptom. Reproduce and minimize when feasible, test falsifiable hypotheses,
  and rerun the signal after a fix. If trustworthy reproduction is unavailable,
  separate observations from hypotheses and state what evidence is missing.
- Verify the narrowest useful public seam, with regression tests for defects
  and runtime/UI/packaging/migration/operator evidence where success depends on
  them; widen for cross-layer/security/migration/runtime/release risk. Focused
  checks do not prove wider acceptance. Report `pass`, `partial`,
  `skipped`, `blocked` or `not_configured` honestly.
- Never make a gate pass by weakening, bypassing, skipping, or misclassifying
  what it protects. Update it only when the accepted contract changes;
  otherwise report the baseline failure or verification gap honestly.
- Enforce checkable invariants in types/schemas, runtime validation, tests,
  lint, CI or hooks at their narrowest owner; guides provide intent and routing.
- Treat IPC, HTTP/SSE, filesystems, settings, provider responses, migration
  input, user-controlled content, and external tool output as untrusted
  boundaries.
- Review the final diff against the request/spec and repository standards;
  judge simplicity by behavior and maintainability. Architecture may change
  with an accepted target accounting for contracts, compatibility/migration,
  security, rollback and verification.

## Current Product Anchors

Unless an accepted target explicitly supersedes them:

- The desktop product is `Electron + React + TypeScript`; canonical package,
  executable, CLI, and protocol identity is `analytix`, and app-owned
  environment variables use `ANALYTIX_*`.
- The only production agent core is `packages/runtime-go`, composed under
  `packages/runtime-go/internal/runtimeapp`.
- `packages/runtime` owns public TypeScript contracts, configuration,
  telemetry, and the `analytix serve` launcher. The launcher starts the Go
  `runtime-server`; `ANALYTIX_RUNTIME_BACKEND=typescript` is diagnostic only.
- The desktop boundary is:

  ```text
  Renderer -> window.analytix -> preload -> main -> Go runtime HTTP/SSE
  ```

- Runtime-owned settings use top-level `runtime`; provider profiles use
  top-level `provider`. Legacy shapes belong only to explicit migration,
  import, fixture, or test paths.
- Release channels are `stable` and `beta`; app, bundle, and Windows identity
  is `com.analytix.desktop`. Provider and model selection remain product
  surfaces, not runtime implementation selectors.

For a changed runtime API, trace only the applicable slice through
`packages/runtime/src/contracts`, Go use case/HTTP/SSE, Electron main transport,
persistence/IPC, preload, renderer client/projection/state/visible consumer and
focused evidence. This is not a requirement to touch every layer.

Treat model-visible context, tool schemas, persisted message history,
compaction, resume or replay, and public events as one compatibility surface;
preserve semantic history and keep injected content bounded and attributable
unless an accepted target intentionally changes the contract.

## Keep Guidance Layered

- `src/AGENTS.md`: desktop, settings, provider, preload, and renderer work.
- `packages/runtime/AGENTS.md`: TypeScript public contracts and launcher.
- `packages/runtime-go/AGENTS.md`: production Go runtime and execution safety.
- `plugins/AGENTS.md`: first-party plugin sources and packaging boundaries.
- `docs/AGENTS.md`: document authority, status, and evidence claims.
- `docs/analytix/development-runbook.md`: host, cache, build, package, and
  operational procedures.
- `docs/analytix/knowledge-base.md`: bounded durable-knowledge synchronization.

Add another nested `AGENTS.md` only when a subtree has durable, distinct rules.
Do not duplicate repository-wide guidance, mutable inventories, milestone
status, or long runbooks. Prefer a pointer to the owning source.

The canonical repository OpenSpec skills are `.agents/skills/openspec-*`. If
they or their verification logic change, run
`npm run verify:openspec-codex-skills`; do not create duplicate
`.codex/skills/openspec-*` copies.

Treat external repositories as commit-pinned research inputs. Before copying
or substantially adapting code, prompts, skills, or assets, follow
`docs/analytix/upstreams/README.md` for license, provenance, and admission
evidence. Task authority never waives third-party terms.

When authorized work changes durable decisions, terminology, or verified
evidence, this guide grants standing authorization for the bounded canonical
mirror described by `docs/analytix/knowledge-base.md`. Update the authoritative
repository source first. Never mirror secrets, raw personal or case evidence,
unsafe logs, or unverified claims.

## Preserve And Verify The Workspace

- Codex Desktop on this configured Mac/canonical workspace is primary;
  ChatGPT + GitHub is auxiliary. Another Mac/cloud is not required by default.
  Use the runbook's development routes for execution scope/evidence; changing
  routes does not clear a safety refusal or transfer native acceptance evidence.
- Before repository-grounded review, diagnosis, implementation, or mutation,
  run `git status --short --branch` and preserve existing user changes.
- `main` tracks public `origin/main`. Follow `docs/analytix/git-workflow.md`:
  normal feature/runtime/plugin/security/dependency/cross-layer work starts on
  a short-lived `codex/*` branch from latest `main`; use authorized push/PR and
  merge only after current CI and applicable acceptance. Never push directly to main,
  bypass checks or weaken tests. Report branch, PR, HEAD, CI and merge readiness
  separately. Keep pre-public/archive history and local-only resources private;
  never merge unrelated archive history, force-add excluded files or use
  `push --all`/`--mirror`. Pre-push checks outgoing history/excluded paths; it
  does not scan secrets or establish license/release acceptance.
- `/Users/sun/Projects/analytix` is the only canonical source and release
  repository. Apply product source changes to this repository and land them in
  its Git history. `/Volumes/AnalytixCache` extends build-storage capacity; it
  is only for dependency/compiler caches, isolated archive extractions,
  temporary worktrees, builds, packages, and evidence. A fix or edited source
  blob that exists only there is `NOT_INTEGRATED` and is not product work.
- For dirty-file overlap, never replace from a cache copy: isolate exact hunks
  or blobs, review against canonical HEAD and integrate only task-owned hunks.
  Use the focused local commit authorized below, or report the uncommitted
  candidate if commits are forbidden. Edit ordinary source here. A fresh
  archive is required for claims depending on source isolation/build/packaging,
  not every document edit; verify the delivered state proportionately.
- User state, including `~/.analytix/data`, is outside the default workspace.
  Use isolated fixtures/temp directories. History inspection/reuse permits
  read-only inventory and a verified task-owned isolated clone, not dev/package
  access to live mutable user/runtime/case paths. Real-state access requires
  explicit exact paths/effects, backup/recovery/rollback, quiescent competing
  writers, verified backup and post-run integrity checks. Fresh-install and
  existing-state recovery evidence are separate and cannot substitute.
- On the configured macOS host, every dependency install, dev server, test,
  build, package, or native compiler/toolchain command that uses build storage
  must source
  `./scripts/use-analytix-cache.sh` in the same shell. If the helper fails
  closed, follow `docs/analytix/development-runbook.md`; do not silently use the
  system volume. Pure text inspection and read-only Git commands do not need
  the helper. A cache failure blocks dependent commands, not independent reads.
- Do not expose secrets, credentials, personal identifiers, remote-control IDs,
  or unsafe response bodies. Do not edit generated, packaged, vendored, or
  historical evidence as source unless the task owns that lifecycle.
- Tests, builds, hooks and diagnostics do not authorize paid/live Provider use,
  production-data mutation or external writes; each needs current task authority.
  Do not send raw sensitive/private protected-local material to external search
  or services, subtask packages, logs or commits without explicit scoped permission.
- Ordinary startup/credential acceptance uses local Provider onboarding or
  Settings and one Registry/Secret Store authority, per
  `openspec/specs/local-provider-credential-authority/spec.md` and
  `openspec/specs/hub-auth-test-bootstrap/spec.md`. Hub is explicit lazy
  compatibility only; these supersede old Hub-login credential instructions
  (including case-evidence 9.11), not other acceptance requirements.
- Credential acceptance uses fresh isolated profiles and normal protected
  entry; packaged bootstrap stays disabled. Never copy tokens/prior profiles/
  Secret Store material into acceptance. With Computer Use and authorized
  credentials, complete visible setup and report automated entry truthfully,
  not as physical-human entry. Missing credential authority/UI, identity
  challenges or failed Provider readiness block dependent live checks only.
  Never retain secrets in source/reports/logs/screenshots/history/telemetry.
- The existing Keychain authorization for `analytix.qa.hub-test-account`
  remains available for explicitly requested legacy Hub compatibility QA.
  It is not local Provider credential authority and does not authorize ordinary
  startup to read Hub credentials or reuse a gateway token as a Provider key.
- Unless the user explicitly says not to commit, approval to modify, implement,
  continue, or finish repository work also authorizes staging only the
  task-owned files or hunks and creating the focused local commit needed to
  deliver that approved work after fresh verification. No separate per-commit
  confirmation is required. Do not commit before success criteria are met, and
  do not mix unrelated workspace changes.
- Amending, pushing, force-pushing, merging, rebasing, tagging, publishing,
  releasing, or opening or merging a pull request requires explicit current
  authorization.
- Destructive operations require explicit scope, exact verified targets, and a
  recovery path. Never use broad destructive Git or filesystem commands against
  the repository, workspace root, home directory, or preserved evidence.
