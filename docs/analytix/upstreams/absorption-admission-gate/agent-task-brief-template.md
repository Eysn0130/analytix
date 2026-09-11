# Agent Task Brief Template

Use this template before dispatching a subagent, reviewer, or focused research
lane. The brief is the bounded context.

Do not paste controller history, full upstream documents, raw transcripts, or
broad accumulated summaries into the handoff.

## Metadata

- brief_id:
- parent_openspec_change:
- lane:
- owner:
- status: draft / dispatched / reported / reviewed / accepted / blocked
- created_at:
- report_path:
- review_package_path:

## Objective

<One precise question or task.>

## Capability Rows

- primary capability_id:
- supporting capability_ids:
- matrix rows to inspect:
- row-level admission gates:

## Required Inputs

List exact files or directories the agent may read.

- <path>

## Out Of Scope

- implementation unless an accepted OpenSpec task explicitly requests it;
- broad essays not tied to evidence paths;
- full transcript reconstruction;
- copying upstream product identity, config roots, or route protocols;
- installing or invoking Superpowers as a default workflow.

## Required Output

The report must include:

- answer_first_summary:
- evidence_paths:
- capability_id:
- current_Analytix_state:
- upstream_or_method_reference:
- absorb:
- reject:
- prompt_boundary:
- speed_cache_risk:
- gate_status:
- unresolved_questions:
- recommendation:

## Prompt And Handoff Bounds

- maximum_context_sources:
- forbidden_handoff_content:
- allowed_summary_size:
- report_artifact_required: yes
- raw_transcript_allowed: no

## Review Expectations

- reviewer:
- review_focus:
- pass_fail_required: yes
- exact_path_findings_required: yes

