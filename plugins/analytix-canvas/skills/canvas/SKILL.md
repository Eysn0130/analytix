---
name: canvas
description: Create structured Canvas scenes and help revise selected scene objects or PNG images through reviewed Core operations. Use for diagrams, visual layouts and scoped image work in the current conversation.
---

# Canvas

Separate source facts from visual decisions. Before arranging a scene, identify the user-provided facts, labels, relationships and any unresolved assumptions. Layout choices such as grouping, alignment, color and spacing may clarify those facts; they must not introduce unsupported entities, measurements or causal claims. Keep labels consistent with their source, and describe inferred relationships as inferences.

For a new scene, use the current schema exposed by `generate_canvas_object` to build a typed Scene. Prefer editable scene objects with stable identities and explicit relationships. The Core creates a new `.canvas` object in the current conversation using absent-only creation; an existing file is not an overwrite target. Treat the returned artifact and revision as the authority for what was saved. Do not describe a proposed scene or a tool call without a confirmed result as a saved artifact.

For a change to existing content, use `canvas_selection_read` for the Core-held selection, then `canvas_selection_propose` for a bounded proposal. Preserve the captured object identities and scope. Explain the intended visual change and preserve unrelated facts and objects. The model does not supply a filesystem path, raw file bytes, an arbitrary object selector or a larger selection. Present the local proposal for review; only the Core's finite apply operation may commit it. If the source revision or selection changed, obtain a fresh selection instead of replaying an old proposal.

PNG crop, rotation and marking use the controlled local UI proposal flow. Describe the requested region and transformation without claiming that a model proposal has already changed pixels. Preserve the original and the provenance of the source image through the Core's save and recovery flow. Unsupported image formats or operations must be reported as unsupported rather than converted through an arbitrary shell command.

Use real generated or edited media only through the existing Core media configuration and execution path, with the required external authorization. An unavailable media configuration, missing authorization or failed execution is a failed dependency. Local vector shapes or SVG illustrations may be useful scene elements, but must be identified as local vector work, never as model-generated image output or evidence that a media Provider ran.

The named generation and selection tools are runtime dependencies, not permissions granted by this Skill. Check the current enabled tool catalog before using them. If a dependency is absent, disabled or unavailable, explain the specific unavailable action and preserve the user's draft; do not simulate a successful call or substitute shell, direct file access or network execution. This Skill adds no such authority. After an uncertain save response, query the existing operation status through the controlled UI/Core flow rather than issuing a fresh write. Report confirmed saved changes separately from pending proposals and unresolved media work.
