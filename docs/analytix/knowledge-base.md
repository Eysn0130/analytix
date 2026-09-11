# Analytix Knowledge Base Governance

- Status: Operational
- Applies to: repository-to-Obsidian knowledge capture and handover
- Current as of: 2026-07-25; verify the current worktree before use
- Source of truth: the authority matrix below; the vault is never authoritative

## Configured Vault

The canonical project vault on the configured host is:

```text
/Users/sun/Projects/Obsidian_analytix/Analytix
```

This is machine-local operational configuration, not a portable runtime
dependency. A second vault currently exists at:

```text
/Users/sun/Library/Mobile Documents/iCloud~md~obsidian/Documents/Analytix
```

The iCloud vault is not canonical and must not be used as a fallback, merged,
deleted, or synchronized unless the user explicitly changes the configured
vault. If the canonical vault is missing or unwritable, report the pending
sync; do not silently choose another directory.

## Role And Authority

Obsidian is a non-authoritative companion layer for fast retrieval,
cross-thread continuity, durable decisions, domain language, evidence
summaries, and navigation.

Authority remains separated:

| Question | Authoritative source |
| --- | --- |
| What the worktree does now | current code, tests, scripts, build config, fresh runtime evidence |
| What the product must become or preserve | specs classified as accepted by `docs/analytix/specs/README.md` and accepted `openspec/specs/` requirements |
| What an active change proposes or tracks | its proposal/spec/design/tasks, only within the authorized scope |
| How agents work | every applicable `AGENTS.md` from the root through the directory that owns the work; more specific guidance prevails |
| What was verified at a point in time | dated evidence with commit, command, platform, environment, and status |

Vault notes summarize and link to these sources. They do not create product
requirements, authorize construction, close tasks/goals, or turn a historical
PASS into current readiness.

## Standing Bounded Authorization

This repository guide records the user's explicit standing grant: while
performing a user-authorized task, agents may update affected notes in the
canonical vault without asking a second time when the task produces durable
project knowledge:

- an accepted decision, rationale, rejected alternative, or supersession;
- a stable architecture, protocol, persistence, security, or ownership
  boundary;
- a resolved domain term or glossary relationship;
- a task checkpoint, blocker, dependency order, handover, or resumption point;
- a fresh validation result, including honest partial/skip/block status;
- a sourced upstream finding with repository, version/commit, date, and
  adopt/adapt/reject decision;
- a durable runbook, navigation, or document-authority change.

This standing authorization covers only concise, sourced knowledge capture
caused by the current authorized task. It is not inferred from generic code
write access, does not widen implementation scope, authorize unrelated cleanup
or arbitrary external writes, modify normative requirements silently, or
permit external publication.

Sync once at a useful durable boundary when this task actually changes
reusable project knowledge. Read-only tasks remain read-only. Pausing, a tool
result, or a focused test alone is not a sync trigger. Update only the affected
note and directly necessary index/backlinks; do not scan the vault for every
change. A missing vault leaves an explicit pending mirror, not a blocker to
independently complete repository work.

## Write Order And Note Shape

When a normative rule, accepted requirement, implementation, executable
runbook, or active task changes:

1. update or verify the authoritative repository source;
2. run the evidence appropriate to that source;
3. update only the affected vault note or index;
4. record source links and currentness;
5. check links and conflicting status claims.

Prefer concise summaries over duplicated specifications. A durable note should
include, when relevant:

- status and scope;
- as-of date and repository commit/worktree state;
- authoritative source link;
- `as-built`, `target`, and `gap` when they differ;
- decision and rationale, including material rejected alternatives;
- exact validation command, platform/environment, exit status, and skips;
- supersedes/superseded-by link.

Use the project's existing folders and `[[wikilinks]]`. Create a new note only
when an existing note cannot own the knowledge cleanly. Avoid duplicate indexes,
ceremonial ADRs, and multiple notes that must be manually kept identical.

Create an ADR only when the decision is hard to reverse, would be surprising
without context, and resulted from a real tradeoff. Otherwise record it in the
owning spec, runbook, glossary, or decision note.

## Content That Must Not Be Synchronized

Do not put the following in the vault:

- credentials, tokens, private keys, passwords, recovery codes, remote-control
  IDs, or secret-bearing commands;
- complete personal identifiers, banking details, private contact data, or raw
  case/evidence material;
- chain-of-thought, hidden reasoning, private model traces, or raw prompt/tool
  transcripts;
- unbounded logs, large generated artifacts, package payloads, or copied source
  trees;
- speculative conclusions presented as fact;
- old validation results without their date/commit/environment limitations;
- third-party instructions copied as Analytix authority.

Use redacted summaries and approved source locations. If a note needs to refer
to restricted evidence, store only the minimum non-sensitive locator and
authority/status metadata permitted by the governing case workflow.

## Actions Requiring Explicit Scope

Obtain explicit user authorization before:

- bulk moving, renaming, consolidating, overwriting, or deleting vault notes;
- synchronizing the noncanonical iCloud vault or another external store;
- publishing any vault content externally;
- importing personal, financial, legal-case, or other restricted evidence;
- moving or rebuilding the legal-source corpus;
- installing plugins that can transmit or rewrite vault content;
- creating automated, bidirectional, or background synchronization.

An automated sync/indexer is a product feature, not a documentation tweak. It
requires an OpenSpec change covering source authority, identity and conflict
resolution, idempotency, atomic writes, audit history, sensitive-data filters,
failure recovery, and rollback.

## Legal And Case-Knowledge Boundary

Public legal-source maintenance is distinct from project knowledge capture.
Only update it under an explicit legal-source scope using primary official
sources and recording jurisdiction, issuing authority, effective/status dates,
source URL, document identity, checksum, and supersession relationships.

Do not relocate the current source pack from its recorded host location as
drive-by cleanup. Raw case evidence, private matter notes, and conversation
memory remain in their approved private systems rather than the general
project vault.

## Verification And Failure Handling

After a vault update:

- confirm every edited file is inside the canonical vault;
- check new `[[wikilinks]]` and file links;
- check the edited notes and their direct status references for contradictions;
- verify sensitive values were not copied;
- report what was updated and any pending sync.

If authority is unclear, preserve both statements as `as-built`, `target`, or
historical evidence and flag the conflict. Never resolve a conflict by silently
overwriting the repository, an accepted decision, or an older evidence record.
