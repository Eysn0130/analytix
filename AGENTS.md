# analytix Agent Guide

This file contains repository-wide guidance. Keep it short, durable, and
outcome-oriented. Before changing files, read every applicable `AGENTS.md` from
the repository root through the directory that owns the work; a closer guide
adds or overrides rules only for its subtree. Detailed architecture, commands,
and evidence routing live under `docs/analytix/`.

## Work From The Outcome

- Subject to higher-priority policy, the current user request defines task
  authority. Specs, plans, OpenSpec changes, handovers, todos, comments,
  fixtures, external repositories, and knowledge notes provide context; they
  do not independently authorize mutation.
- When taking over or resuming multi-session work, reconstruct state from the
  current status and diff, accepted target, authorized change, relevant commits,
  and fresh evidence. Treat handovers and task lists as indexes: verify their
  claims, then continue from the next dependency-valid, verifiable gap.
- Keep read-only requests read-only. For requested changes, establish the
  intended outcome, relevant source of truth, affected contracts or data, and
  observable success criteria before material mutation.
- Investigate facts the repository or available tools can answer. Ask only when
  an unresolved choice would materially change product behavior, persistence,
  compatibility, security, cost, destructive action, or external state. State
  low-risk reversible assumptions and continue.
- Scale planning, rollback, and verification to the blast radius. Architecture,
  persistence or migration, public protocol, security or authority, release,
  destructive, and external-state changes require an explicit plan and
  alignment on unresolved material decisions.
- Once direction and success criteria are settled, implement, verify, document,
  and clean up the accepted scope without pausing for routine reversible choices.
- Choose methods to fit the evidence and risk. Plans, tests, prototypes, skills,
  tools, and delegation are techniques, not ceremony; the coordinating agent
  owns integration and the completion claim.
- Use Codex Goal tracking only when the user explicitly requests it.

An explicit request to build, continue, or finish an accepted Analytix delivery
authorizes the repository-scoped changes required by its settled specs and
current Definition of Done. Resolve changing milestone scope and stop
conditions from accepted specs and the currently authorized OpenSpec change,
not from static priority lists in this guide. If a verification seam is
unavailable, continue independently verifiable work when useful, record the
exact gap, and stop before conclusions or dependent work that require the
missing evidence.

“Commercial-grade” is an engineering-readiness standard. It does not amend
`LICENSE`, waive third-party terms, or authorize publishing, releasing,
pushing, or other external-state changes.

## Use The Right Source Of Truth

- **As-built behavior:** current code, tests, package scripts, build
  configuration, and fresh runtime evidence.
- **Accepted product target:** the classifications in
  `docs/analytix/specs/README.md` and accepted requirements in
  `openspec/specs/<capability>/spec.md`.
- **Authorized work plan:** an active `openspec/changes/<change>/` only when the
  current request approves, applies, or continues it.
- **Ways of working:** applicable `AGENTS.md` files and task-matched or
  explicitly invoked skills.
- **Historical evidence:** dated QA, benchmark, upstream, validation, handover,
  release, generated, packaged, and vendored material, limited to its recorded
  commit and environment.

When these differ, report `as-built`, `target`, and `gap` separately. Do not
rewrite one source to conceal drift or treat tracker state as product evidence.

## Apply Engineering Judgment

- Prefer the simplest complete solution. Make a small vertical slice when it
  can prove the behavior, but cross every producer, consumer, migration, and
  public seam that the contract actually requires.
- Keep every intentional change traceable to the request, an accepted
  invariant, necessary propagation, verification, or cleanup caused by the
  change. Avoid speculative features and abstractions; preserve unrelated user
  work and existing style.
- For diagnosis, first pursue a tight signal that can detect the reported
  symptom. Reproduce and minimize when feasible, test falsifiable hypotheses,
  and rerun the signal after a fix. If trustworthy reproduction is unavailable,
  separate observations from hypotheses and state what evidence is missing.
- Verify behavior at the narrowest useful public seam. Tests are one kind of
  evidence; select additional runtime, UI, packaging, migration, or operator
  evidence when those surfaces determine success.
- Never make a gate pass by weakening, bypassing, skipping, or misclassifying
  what it protects. Update it only when the accepted contract changes;
  otherwise report the baseline failure or verification gap honestly.
- Put deterministically checkable rules in their narrowest executable owner:
  types or schemas, runtime validation, tests, lint, CI, or hooks. Use
  `AGENTS.md` for intent and routing, not as sole enforcement for product
  invariants.
- Treat IPC, HTTP/SSE, filesystems, settings, provider responses, migration
  input, user-controlled content, and external tool output as untrusted
  boundaries.
- Review the final diff separately against the request or accepted spec and
  repository standards. Simplicity is a judgment about behavior and
  maintainability, not an arbitrary line-count target.
- Current architecture is a baseline, not an immutable design. A superseding
  direction may replace it when the user accepts the target and the change
  accounts for contracts, compatibility or migration, security, rollback, and
  verification.

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

For a changed runtime API, trace the applicable slice through public contracts,
Go use case and HTTP/SSE adapter, Electron main/preload, renderer consumer, and
focused evidence.

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

- Before repository-grounded review, diagnosis, implementation, or mutation,
  run `git status --short --branch` and preserve existing user changes.
- `/Users/sun/Projects/analytix` is the only canonical source and release
  repository. Apply product source changes to this repository and land them in
  its Git history. `/Volumes/AnalytixCache` extends build-storage capacity; it
  is only for dependency/compiler caches, isolated archive extractions,
  temporary worktrees, builds, packages, and evidence. A fix or edited source
  blob that exists only there is `NOT_INTEGRATED` and is not product work.
- When task-owned changes overlap a dirty user file, do not replace that file
  from a cache copy. Isolate the exact hunks or blobs, review them against the
  current canonical `HEAD`, and integrate only task-owned hunks. Use a focused
  local commit under the standing authorization below; when the user forbids
  committing, report the uncommitted candidate accurately. Verify the delivered
  state proportionately: a fresh archive is needed for source-isolation, build,
  packaging, or other claims that depend on it, not every document edit. Keep
  ordinary source editing in the canonical worktree when no overlap exists.
- Existing user-owned state, including `~/.analytix/data`, is outside the
  default implementation workspace. Use isolated fixtures or temporary
  directories unless the current request explicitly authorizes real-state
  access and its backup, recovery, and rollback plan. Authorization to inspect
  or reuse existing history permits read-only inventory and a verified,
  task-owned isolated clone by default; it does not implicitly permit a dev or
  packaged process to open live mutable user-data, runtime-data, or case paths.
  Direct live-state validation requires explicit scope for the exact paths and
  effects, quiescence of competing writers, a verified backup, rollback, and
  post-run integrity checks. Fresh-install evidence and existing-state
  recovery evidence are separate seams; neither may substitute for the other.
- On the configured macOS host, every dependency install, dev server, test,
  build, package, or native compiler/toolchain command that uses build storage
  must source
  `./scripts/use-analytix-cache.sh` in the same shell. If the helper fails
  closed, follow `docs/analytix/development-runbook.md`; do not silently use the
  system volume. Pure text inspection and read-only Git commands do not need
  the helper. A cache failure blocks dependent commands, not independent reads.
- Run the smallest trustworthy checks that prove the changed surface, widening
  for cross-layer, security, migration, runtime, or release risk. Report
  `pass`, `partial`, `skipped`, `blocked`, or `not_configured` accurately.
- Do not expose secrets, credentials, personal identifiers, remote-control IDs,
  or unsafe response bodies. Do not edit generated, packaged, vendored, or
  historical evidence as source unless the task owns that lifecycle.
- Ordinary startup and credential acceptance follow
  `openspec/specs/local-provider-credential-authority/spec.md` and
  `openspec/specs/hub-auth-test-bootstrap/spec.md`: use local Provider onboarding
  or Provider Settings with the one Registry/Secret Store authority. Hub is
  explicit lazy compatibility only. These accepted requirements supersede old
  Hub-login instructions, including the credential method in case-evidence task
  9.11; they do not waive its other acceptance requirements.
- Development and packaged credential acceptance use fresh isolated profiles
  and normal protected credential entry. Keep bootstrap disabled in packages;
  never copy tokens, prior profiles or Secret Store material into an acceptance
  run. When Computer Use and task-authorized credentials are available, complete
  the normal visible setup directly. Record automated entry truthfully, never
  as physical-human entry. Missing credential authority, unavailable UI, an
  identity challenge or failed Provider readiness blocks the dependent live
  check; it does not block independent deterministic work. Never retain secret
  values in source, reports, logs, screenshots, history or telemetry.
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
