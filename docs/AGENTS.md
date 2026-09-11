# analytix Documentation Agent Guide

This guide adds documentation-specific guidance for `docs/`. Inherit the root
`AGENTS.md` and read `docs/analytix/README.md` before treating an existing
document as current architecture or acceptance evidence.

## Separate Authority And Evidence

- Root and nested `AGENTS.md` files govern how agents work; they do not prove
  product behavior.
- `docs/analytix/specs/README.md` classifies numbered product specs. Accepted
  scoped requirements also live in `openspec/specs/<capability>/spec.md`.
- An active `openspec/changes/<change>/` is a proposal and work plan. It becomes
  an implementation instruction only when the current request authorizes that
  change; an archived change is decision history.
- Current code, tests, package scripts, and reproducible runtime or build
  results define the as-built worktree.
- Dated QA, benchmark, upstream, validation, handover, and release documents
  are snapshots tied to their recorded commit, platform, environment, and
  command.
- `docs/legacy/`, `release/legacy/`, `output/`, `dist*/`, package-resource
  copies, and `vendor/` are historical, generated, or vendored inputs unless a
  current source explicitly says otherwise.

If accepted target and current implementation differ, document `as-built`,
`target`, and `gap` separately. A document cannot remove drift by changing
tense, and accepted status does not imply implementation or fresh validation.

## Write Durable Documents

Use these status terms consistently:

- `Normative`: accepted target or invariant.
- `Operational`: current runbook or workflow.
- `Reference`: design input, comparison, or explanation.
- `Historical`: retained snapshot that does not drive current implementation.

- State status, scope, source of truth, and currentness or supersession for new
  high-impact documents when it helps a reader choose the right source. Avoid
  ceremonial metadata that adds no routing value.
- Verify present-tense paths, commands, configuration fields, architecture
  owners, and public behavior against current sources before publishing them.
  A schema accepting a field does not prove the production runtime uses it.
- Prefer repository-relative links. Keep machine-specific paths in explicitly
  scoped host or provenance documentation.
- Preserve historical reports as history; add a currentness note rather than
  silently rewriting old evidence. Remove or redact material only when keeping
  it creates a privacy, secret, or active safety risk.
- Update Chinese and English counterparts together when they express the same
  current product fact.
- Keep the repository authoritative. Follow
  `docs/analytix/knowledge-base.md` for any bounded knowledge mirror, and do not
  create a competing specification in a vault note.
- Keep edits focused. Do not refresh generated evidence, package snapshots, or
  unrelated ledgers as incidental cleanup.

## Make Evidence Claims Precisely

- A recent timestamp, a filename containing `final`, or a historical `passed`
  result is not current release authorization.
- Record the tested commit or worktree, exact command, relevant platform and
  environment limits, and an honest status: `pass`, `partial`, `skipped`,
  `blocked`, or `not_configured`.
- Identify fixture, conformance, shadow, deterministic, and fake-provider
  results as such. Do not relabel them as live-provider, live-MCP,
  packaged-GUI, or operator evidence.
- Rerun the relevant current check before converting a snapshot into a
  present-tense claim.
- Never include real passwords, private keys, product or API keys, gateway
  tokens, remote-control IDs, recovery codes, personal case evidence, or
  credential-bearing command output.

## Validation

For documentation-only changes, run `git diff --check` and verify changed
relative links and referenced paths. Add executable checks only when the claim
depends on code, generated output, runtime behavior, or packaging. Never report
a command as run when it was copied from historical evidence.
