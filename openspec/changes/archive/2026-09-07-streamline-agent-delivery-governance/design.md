## Context

See proposal.md. The current handoff specification carries unconditional file/report requirements while AGENTS.md already delegates method choice to engineering judgment. RC coordination additionally assumes Sol and repeats evidence at several hierarchy levels. Existing protected worktree changes must remain untouched.

## Goals / Non-Goals

**Goals:** One canonical OpenSpec lifecycle, model-neutral RC coordination, complete requirement coverage, proportional evidence, and recoverable host instruction maintenance.

**Non-Goals:** Product runtime changes, weakening formal readiness, changing main model/effort or sandbox/approval configuration, installing Superpowers hooks, or publishing/pushing source.

## Decisions

- Keep a compact direct-work path and load the fleet protocol only for real delegation, takeover or recovery. Preserve explicit epoch retirement and first-turn bootstrap constraints.
- Keep canonical `.agents/skills/openspec-*` implementations and make Claude/Cursor entrypoints adapters. This prevents lifecycle drift without maintaining duplicate implementations.
- Bind evidence to content, environment, command and claim. A separate reviewer can challenge evidence; neither role count nor a new Git commit with identical content forces a repeated suite.
- Keep Superpowers reference-only. Study all 14 skill entrypoints at a pinned MIT source, adopt selected reasoning practices in the existing workflow, and reject its global hook/approval/TDD/review choreography. No product dependency, prompt hook or tool schema is introduced.
- Preserve managed skill bytes before local edits and retain before/after hashes privately. Do not treat a patched plugin cache as upstream source or silently reapply patches to a different version.

## Risks / Trade-offs

- More discretion could miss a real gate → retain precise authority, single-writer, privacy and claim ceilings; exercise adversarial forward scenarios.
- Cached plugins can be replaced by upgrades → retain exact baseline/patch records and require review on version drift, without automatic background rewriting.
- Existing dirty guidance overlaps the task → stage task-only hunks and preserve original user-owned differences, including the standing login grant.
- Instruction behavior is stochastic → static validation and independent scenario tests establish consistency, not a measured delivery-speed improvement or complete runtime guarantee.

## Migration Plan

Apply the approved guidance edits, validate the scoped lifecycle and links, synchronize this spec through the canonical OpenSpec lifecycle, and create a focused local commit. Host files have private before/after snapshots for a reviewed restoration; repository changes are reversible through focused subsequent edits, without resetting other work.
