# Analytix Spec Registry

- Status: Operational registry
- Applies to: numbered product specs in this directory
- Current as of: 2026-09-09
- Source of truth: each registered spec for its accepted scope; current code
  and fresh validation for as-built behavior

This registry classifies which numbered documents define accepted targets,
which define operational acceptance contracts, and which are retained only as
historical reference. A registered target is not proof that the current
worktree implements or passes it.

| Spec | Classification | Accepted scope and currentness rule |
| --- | --- | --- |
| [`01-identity-runtime-schema-reset.md`](01-identity-runtime-schema-reset.md) | Accepted target | Identity, bridge, settings, CLI, and migration invariants. Historical pre-migration audit sections are not current implementation facts. |
| [`02-release-packaging-channels.md`](02-release-packaging-channels.md) | Accepted target | Release identity, channels, and artifact decisions. Verify scripts, versions, and packaging behavior against current configuration. |
| [`03-brand-assets-visual-system.md`](03-brand-assets-visual-system.md) | Accepted target | Brand assets and provenance constraints. Recorded host paths are provenance, not portable runtime dependencies. |
| [`04-analytix-derived-thread-virtualizer.md`](04-analytix-derived-thread-virtualizer.md) | Accepted target | Streaming, virtualization, and scroll invariants. Pre-migration implementation paths are historical. |
| [`05-architecture-code-upgrade-inventory.md`](05-architecture-code-upgrade-inventory.md) | Historical reference | Migration inventory and reference checklist. It does not define current architecture or reopen risks already closed by later accepted targets and implementation. |
| [`06-implementation-closure-and-acceptance.md`](06-implementation-closure-and-acceptance.md) | Accepted target | Cross-layer closure criteria. Separate current closure requirements from dated baseline and acceptance snapshots. |
| [`07-desktop-qa-release-readiness.md`](07-desktop-qa-release-readiness.md) | Accepted acceptance contract | Real Electron, long-thread, platform, and packaging acceptance requirements. A checklist row is not a current pass without fresh evidence. |
| [`08-upstream-absorption-and-go-runtime.md`](08-upstream-absorption-and-go-runtime.md) | Accepted target | Upstream absorption and Go-runtime evolution invariants. Use its currentness notes and later accepted addenda; chronological stage records remain historical evidence. |
| [`09-agent-quality-product-benchmark.md`](09-agent-quality-product-benchmark.md) | Accepted target | Quality dimensions, benchmark method, and evidence requirements. Dated upstream snapshots and results expire as current evidence. |
| [`10-subagent-todo-goal-control-plane.md`](10-subagent-todo-goal-control-plane.md) | Accepted target | Subagent, todo, goal, background-job, and worktree control-plane contracts. Read currentness notes, implementation status, and deferred items separately. |
| [`11-agent-platform-brand-and-architecture.md`](11-agent-platform-brand-and-architecture.md) | Accepted target | Agent Platform brand, the one Go Agent Harness, built-in Privacy Layer, package-plane versus Core capability-plane plugin boundaries, sensitive-data projection, and Funds-as-flagship-plugin positioning. Its target and current gaps remain separate. |

Accepted scoped requirements under
`../../../openspec/specs/<capability>/spec.md` complement this registry. A
later, more specific accepted requirement supersedes an older statement only
for the same scope. Active OpenSpec changes remain proposals and work plans
until the current request authorizes applying or continuing them.

When adding, replacing, accepting, or retiring a numbered spec, update this
registry in the same change. Do not infer lifecycle from filename order,
modification time, a `final` label, or Git tracking alone.

## Current scoped requirement synchronization

The accepted OpenSpec capabilities for claim publication, data forensics,
desktop release safety, the evidence kernel, provider structured output,
report publication, and source MCP readiness record the 2026-08-16 synchronization snapshot of
`case-evidence-publication-gate`. Later active deltas are not all synchronized;
read the authorized change for its additional target and dependencies. The synchronized target
separates the Go-host-owned fixed funds analysis seam, Direct Source Preview,
retained AcceptedSlotDisplay recovery, and host-private evidence carriers from
ordinary MCP configuration and generic public channels.

This synchronization records accepted requirements only. Implementation,
source-freeze, Electron, formal package, Provider, signing, notarization,
commercial-license, and publication status still require their own fresh
evidence and must not be inferred from this registry entry.

The accepted
[`agent-platform-foundation`](../../../openspec/specs/agent-platform-foundation/spec.md)
capability defines the common platform contract. Existing `case-*` capabilities
remain narrower professional-data and publication contracts under that
foundation; their existence does not put Funds or case-domain semantics into
Analytix Core.


## Credential migration and active platform work

The local Provider change was archived at
[`2026-08-31-establish-local-provider-credential-authority`](../../../openspec/changes/archive/2026-08-31-establish-local-provider-credential-authority/proposal.md).
Its accepted [credential authority](../../../openspec/specs/local-provider-credential-authority/spec.md)
and revised `hub-*` specifications supersede ordinary Hub-first startup and
Hub-login acceptance instructions. The `hub-*` filenames are compatibility
identifiers; their current bodies define local startup and lazy compatibility.
Archive and checked tasks are historical completion records, not a fresh
credential-security or product-release result.

The platform change's later package-declaration and local-build admission
deltas also extend the synchronized foundation. Neither `status.isComplete`
nor an artifact marked `done` closes implementation tasks. Use the
[dated review](../documentation-delivery-review-2026-09-09.md) for the examined
conflicts and scope limits; current work remains governed by the applicable
accepted requirement and authorized change, not by a new audit roadmap.
