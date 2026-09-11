# Analytix Git workflow

Status: Operational. The canonical checkout and GitHub `main` share the public
history rooted at `eed7dfb1a1e6cdd50002f7ce8fcb2e2888b0547d`.

## Daily use

In the project directory, before editing:

```sh
git status --short --branch
git pull
```

Then refresh changed dependencies and build outputs; `pull` does not do that
for you. `npm run bootstrap` performs locked installation, and
`npm run verify:baseline` checks the source-development baseline. On the
configured Owner macOS host, source `./scripts/use-analytix-cache.sh` in the
same shell first. See [Development baseline](development-baseline.md) for
tools, native resources, CI and packaging limits.

After editing and running the checks appropriate to the change:

```sh
git diff --check
git diff
git add <only-the-files-you-intend-to-publish>
git diff --cached
git commit -m "Describe the change"
git push
```

`pull` downloads and integrates remote changes. `commit` records local changes;
`push` uploads committed changes. Uncommitted or ignored files are not uploaded.
Do not use `git add -f`, `push --all`, `push --mirror`, or a force push as part of
normal synchronization. Review for secrets and private data before committing.

Pull is configured as fast-forward-only: if local and remote both gained
commits, Git stops without choosing a merge or rewriting local commits. Keep
local edits safe, inspect `git log --oneline --left-right main...origin/main`,
then deliberately reconcile the public branches (for example, rebase only
unpublished local commits). Do not reset away work or merge the private archive.
For a new feature branch, use `git switch -c codex/<name>` followed by
`git push -u origin codex/<name>` after committing.

## A new clone

```sh
git clone https://github.com/Eysn0130/analytix.git
cd analytix
npm run git:setup
```

Setup requires Git and Node, not dependency installation. It preserves an
existing `origin`, configures `main` tracking, `pull.ff=only`, `push.default=simple`
and a pre-push hook. Hooks are not installed automatically by Git clone.
Authenticate with your own GitHub account using your normal Git credential
helper or SSH setup; never put a token in a tracked file or remote URL. Configure
your own commit name and public/noreply email before your first commit.

The hook refuses private/unrelated history, non-fast-forward updates, the
existing local-only file exclusions, private-key/env file names and oversized
Git blobs. It inspects outgoing commits, not only the final tree. A deletion in
a later commit does not make an earlier private blob safe to publish. Full
history is required; use a full clone or fetch missing history for shallow
clones. The hook is a local accident-prevention measure, not a comprehensive
secret scanner, licensing decision, release gate or server-side security rule.

## What the alignment preserves

The 2026-09-12 alignment carries the six previously local commits into the
public mainline: eight new source/test files and six updated files for the
permission service, runtime idle/SSE lifecycle and regression tests. Their
implementation bytes are preserved; this is not a product rewrite.

The old private mainline is preserved in a local private Git reference and a
verified Owner-private bundle. Existing branches, worktrees and stash are not
deleted. Do not push these archive refs or merge their ancestry into public
branches. Extract only reviewed source changes when recovering older work.

The 57 previously excluded files are recorded in
[`scripts/public-source-policy.json`](../../scripts/public-source-policy.json).
They remain recoverable in Owner-private history/archives, but are no longer
tracked by the public mainline. The list includes
generated diagnostics, screenshots, Python metadata, a compiled test binary,
extracted managed-browser resources and the native Computer Use application.
Those resources were not newly removed from the public repository by this
alignment. `.gitignore` prevents accidental staging; do not force-add them.

The subsequent baseline cleanup archived and hash-verified 38 generated or
dated-evidence files before removing those exact local copies. The 18 required
browser/native resources and one local launch configuration remain in place.
An empty, unused root Clang analyzer report was also archived and retired.
No product source, test or resource capability was deleted.

Consequently, public source synchronization does not establish complete
installer reproducibility: managed-browser/native Computer Use packaging
resources and the already known engineering/acceptance gaps remain separate
work. No function, test assertion or privacy enforcement is removed or weakened
by this Git transition. `LICENSE` and `THIRD_PARTY_NOTICES.md` retain their
existing bytes; public availability and source alignment do not grant new
third-party licensing rights.

The one-off `PRIVATE_SNAPSHOT.md` and `source-snapshot-manifest.json` overlays
are retired from the active tree; their historical versions remain in the
public baseline. Current Git trees, this workflow and the exclusion policy now
describe synchronization instead of a manually refreshed snapshot copy.

Git configuration semantics: [official git-config documentation](https://git-scm.com/docs/git-config).
