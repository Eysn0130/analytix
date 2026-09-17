# Analytix Knowledge Base Governance

- Status: Operational
- Applies to: repository knowledge capture、cross-thread continuity、ADR / research indexing 与正式交付路由
- Current as of: 2026-09-16；使用前仍须核对当前 Git / PR / CI
- Source of truth: repository authority matrix below；Notion 与 Google Drive 均不覆盖仓库事实
- Supersedes: 本文件 2026-07-25 版本中“canonical Obsidian vault”作为当前知识层的配置

## Role And Authority

Analytix 采用三层治理，职责不可互换：

| 层 | 当前角色 | 能否定义当前工程事实 |
| --- | --- | --- |
| GitHub repository `Eysn0130/analytix` | 代码、Git 历史、Branch / PR / SHA、accepted specs、OpenSpec、仓库内计划/交接/证据、CI/review | **是，按各自 authority 类型** |
| Notion `Analytix Engineering Knowledge Base` | Construction Ledger、Architecture Decisions、Research & References、可复用 Engineering Workflow | **否；只做结构化知识与治理索引** |
| Google Drive `Analytix` | 正式 Reports、Sheets & Benchmarks、Slides、Release & Evidence Exports、Archive | **否；只做交付与导出层** |

聊天历史、模型记忆、Notion 页面和 Drive 文件都可以帮助定位背景，但不能单独证明
当前代码、当前任务完成度、当前 CI 或 release readiness。

仓库内部的 authority 继续按下表区分：

| Question | Authoritative source |
| --- | --- |
| What the worktree / branch does now | current code, tests, scripts, build configuration and fresh runtime evidence |
| What the product must become or preserve | `docs/analytix/specs/README.md` classified accepted targets/contracts and accepted `openspec/specs/` requirements |
| What an active change proposes or tracks | its `openspec/changes/<change>/` artifacts, only inside the scope authorized by the current user request |
| How agents work | every applicable `AGENTS.md` plus the task-matched workflow / Skill |
| What was verified at a point in time | dated evidence tied to the exact commit/worktree, command, platform/environment and result |
| How a new thread resumes | `docs/analytix/handovers/README.md` plus its latest checkpoint, then fresh GitHub facts |

When these differ, preserve `as-built`, `target` and `gap` separately. Never make them agree by
silently rewriting one layer.

## Configured Knowledge And Delivery Layers

### Notion

The configured project knowledge layer is the connected Notion workspace containing:

- **Analytix Engineering Knowledge Base** — hub page;
- **Construction Ledger** — branch / Base SHA / HEAD SHA / PR / plan / evidence / acceptance / risk index;
- **Architecture Decisions** — ADR decisions, alternatives, rationale, consequences and status;
- **Research & References** — commit/version-pinned external research with Adopt / Adapt / Watch / Reject / Reference Only assessment;
- **Analytix Engineering Workflow & Templates** — reusable construction and thread-resumption workflow.

Do not put private Notion workspace IDs or private page URLs into this public repository merely to
make automation easier. Agents with the connector should locate the pages by the stable names above.
If the connector is unavailable, repository construction must remain resumable from GitHub alone.

### Google Drive

The configured formal-delivery root is a Drive folder named **Analytix**, currently organized as:

```text
Analytix/
├── 01 Reports
├── 02 Sheets & Benchmarks
├── 03 Slides
├── 04 Release & Evidence Exports
└── 05 Archive
```

Drive is not a second source repository. Do not copy normal source trees, routine `.md` plans, branch
state or mutable task truth there. Use it when the user requests or the delivery requires a durable
report, spreadsheet/benchmark workbook, presentation, formal release/evidence export or archive copy.

Do not place private Drive IDs or non-public folder URLs in this public repository unless the user
explicitly chooses to publish them.

## Legacy Obsidian Boundary

The previously configured host-local Obsidian vault
`/Users/sun/Projects/Obsidian_analytix/Analytix` and the noncanonical iCloud vault are retained only as
legacy/historical companion stores. They are **not the current canonical project knowledge layer** as
of 2026-09-16.

Do not automatically update, merge, delete, migrate or synchronize either vault. Historical notes can
still be consulted when a task explicitly requires them, but any reusable current fact must first be
re-established from the repository and then, when appropriate, mirrored to Notion.

## Standing Bounded Knowledge Capture

For a user-authorized Analytix task, agents may update the directly affected Notion knowledge records
without requesting a second confirmation when the task creates durable project knowledge such as:

- an accepted decision, rationale, rejected alternative or supersession;
- a stable architecture, protocol, persistence, security, privacy or ownership boundary;
- a task checkpoint, blocker, dependency order, handover or resumption point;
- a fresh validation result with an honest pass/fail/partial/skip/block/unverified state;
- a sourced upstream finding with repository, version/commit, verification date and
  Adopt/Adapt/Watch/Reject decision;
- a durable runbook, navigation or document-authority change.

This is bounded mirror authorization, not independent product authority. It does not widen the code
scope, authorize unrelated cleanup, permit merge/release/publication, or allow Notion to alter an
accepted requirement silently.

Google Drive writes are narrower: place output there when the current task calls for a formal
report, Sheet/benchmark workbook, Slides deck, release/evidence export or archive delivery. Routine
construction bookkeeping remains GitHub + Notion.

## Write Order

When a normative rule, implementation, executable runbook, current task state or evidence changes:

1. establish and update the authoritative repository source or exact candidate first;
2. run or inspect the evidence appropriate to that change;
3. update the repository handover / QA / evidence record when the information is needed for safe
   cross-thread recovery;
4. mirror only the durable summary to the matching Notion Construction Ledger / ADR / Research item;
5. create or update a Drive artifact only if there is a formal deliverable;
6. verify links, SHA bindings, status wording and sensitive-data exclusions.

A Notion edit that precedes the repository does not make the described state true. If an unavoidable
connector ordering issue creates temporary drift, mark it pending and reconcile it in the same task.

## Cross-Thread Continuity Contract

`docs/analytix/handovers/README.md` is the repository-resident, connector-independent resumption
entry. Each meaningful checkpoint should preserve enough information that a fresh thread can answer:

- repository, branch, PR and Base SHA;
- start HEAD and delivered/current candidate HEAD;
- actual user-authorized scope;
- changes actually made versus planned;
- tests/checks that actually completed and their exact result;
- current CI / CodeQL / review status;
- PASS / FAIL / BLOCKED / UNVERIFIED seams;
- external/environment blockers;
- exact next dependency-valid action;
- explicitly out-of-scope work;
- whether the Notion ledger was updated or remains pending.

New threads must fresh-query GitHub before writing. Handover and Notion records are indexes, not leases
or automatic proof. If the current HEAD, user request or accepted target changed, the new facts win and
the mirror is updated afterward.

Do not create a new branch, PR, controller ledger or knowledge database merely because the chat thread
changed. Reuse the current construction line when it still owns the task.

## Knowledge Record Shape

### Construction Ledger

Each active material work item should, when known, record:

- Work Item / Status / Priority / Type;
- Repository / Branch / Base SHA / HEAD SHA / PR;
- Plan / Execution / Evidence links when they exist;
- concise Summary / Acceptance / Risk;
- Started / Completed dates;
- related ADR / Research references when material.

`Complete` means the scoped implementation and its required evidence are actually closed. A plan,
commit, PR opening or partial test suite is insufficient.

### Architecture Decisions

Create or update an ADR when a decision is hard to reverse, surprising without context, or affects
multiple future changes. Record context, alternatives, decision, rationale, consequences, owner/status
and the repository ADR/spec link when available. Avoid ceremonial ADRs for ordinary local implementation
details.

### Research & References

External projects are research inputs, not authority. Pin the source to a version / commit when possible,
record the verification date, relevant topic, concrete applicability and risks, and classify the outcome
as `Adopt`, `Adapt`, `Watch`, `Reject` or `Reference Only`.

## Content That Must Not Be Mirrored Or Exported

Do not put the following in Notion, Drive delivery folders or public repository knowledge records:

- credentials, tokens, private keys, passwords, recovery codes, product/API keys or secret-bearing
  commands;
- remote-control IDs or private authentication locators that are not explicitly approved for public
  operator documentation;
- complete personal identifiers, banking details, private contact data or raw case/evidence material;
- chain-of-thought, hidden reasoning, private model traces or raw prompt/tool transcripts;
- unbounded logs, large generated payloads or copied source trees as “knowledge”;
- speculative conclusions represented as verified fact;
- old validation results stripped of their commit/date/environment limitation;
- third-party instructions copied as Analytix product authority.

Use redacted summaries and approved source locators. Sensitive raw material remains in its governing
protected system.

## Actions Requiring Explicit Scope

Obtain explicit current scope before:

- bulk moving, renaming, consolidating, overwriting or deleting Notion databases/pages or Drive folders;
- changing sharing/publication permissions for Notion or Drive;
- publishing internal knowledge or deliverables externally;
- importing personal, financial, legal-case or other restricted evidence;
- reactivating, merging or deleting legacy Obsidian vaults;
- creating automated, bidirectional or background synchronization among GitHub, Notion, Drive or
  legacy vaults;
- making Notion/Drive a runtime dependency of Analytix itself.

An automated sync/indexer is a product feature, not a documentation convenience. It requires an
accepted design covering source authority, identity, conflict resolution, idempotency, atomic writes,
audit history, sensitive-data filters, failure recovery and rollback.

## Conflict Resolution

Use this order when records disagree:

1. current user request defines task authority;
2. applicable `AGENTS.md` defines ways of working;
3. accepted product/OpenSpec requirements define target;
4. current code/tests/build/runtime evidence define as-built behavior;
5. current GitHub PR/commit/CI/review defines candidate state;
6. handover records explain prior state and next-action intent;
7. Notion mirrors durable summaries and task indexes;
8. Drive contains formal outputs only;
9. legacy Obsidian/chat history provides background only.

Do not delete the losing historical statement merely because it became stale. Mark it superseded or
update the current mirror, while preserving useful provenance.

## Verification And Failure Handling

After a repository-to-Notion update:

- confirm the Notion record names the correct repository / branch / PR / SHA or explicitly marks unknown;
- check that `Complete`, `PASS`, `Blocked` and `Unverified` wording matches the actual evidence;
- verify no secret or restricted material was copied;
- ensure any related ADR / Research relation points to the intended record;
- if Notion is unavailable, record `pending mirror` in the handover only when the pending state matters;
  it must not block independently valid repository work.

After creating a formal Drive deliverable:

- verify the artifact exists in the intended `Analytix` subfolder;
- make clear which repository commit/PR/evidence it represents;
- do not infer source freshness from Drive modified time;
- preserve existing sharing settings unless the user explicitly asks to change them.

If authority is unclear, preserve both observations as `as-built`, `target`, historical evidence or
unverified mirror and flag the conflict. Never resolve ambiguity by silently overwriting accepted
requirements, current code or older evidence.
