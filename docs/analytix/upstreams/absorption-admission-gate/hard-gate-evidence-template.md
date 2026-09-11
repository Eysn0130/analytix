# Hard Gate Evidence Template

Use this template for every P0/P1 absorption row that touches provider history,
context, tools, MCP, memory, instructions, compaction, hooks, skills/plugins,
subagents, runtime bridge, or UI polling.

Missing evidence produces `blocked`, not `pass`.

## Metadata

- change_id:
- capability_id:
- row_source:
- prepared_by:
- prepared_at:
- review_status: draft / reviewed / accepted / blocked
- reviewer:

## Gate Status

- verdict: ready / blocked / not-applicable
- blocked_reason:
- gateCommand:
- cacheEvidenceArtifact:
- context_epoch_required: true / false / not-applicable
- plugin_or_skill_admission_required: true / false / not-applicable

## Prompt Boundary

- prompt_boundary: stable-prefix / dynamic-context / turn-tail / store-only /
  ui-only / diagnostics-only
- stable_prefix_impact:
- dynamic_context_impact:
- turn_tail_content:
- store_only_state:
- ui_only_state:
- diagnostics_only_state:
- content_forbidden_from_prompt:

## Prompt And Cache Evidence

- normal_turn_token_delta:
- activated_turn_token_delta:
- prefix_hash_before:
- prefix_hash_after:
- prefix_change_reasons:
- tool_schema_hash_before:
- tool_schema_hash_after:
- tool_schema_change_reasons:
- cache_telemetry_source:
- cache_telemetry_unknown_reason:
- deepseek_tail_cache_avg:
- first_visible_token_latency:
- e2e_turn_latency:

## Context And Compaction

- compact_frequency:
- summary_model_call_count:
- consecutive_compacts:
- post_compact_cache_recovery_turns:
- source_registry_or_epoch_impact:
- tool_output_budget_impact:
- recovery_or_restart_behavior:

## UI And Runtime Bridge

- ui_same_prompt_ab:
- opened_surface:
- closed_surface:
- prompt_history_same: yes / no / not-applicable
- prefix_hash_same: yes / no / not-applicable
- tool_schema_hash_same: yes / no / not-applicable
- runtime_request_shape_same: yes / no / not-applicable

## Agent Handoff And Review

- task_brief_source:
- report_artifacts:
- review_artifacts:
- progress_ledger:
- subagent_context_policy:
- forbidden_handoff_content:
- superpowers_usage: reference-only / not-installed / installed-after-wave0
- superpowers_install_status: blocked / not-applicable / allowed-after-review

## Security And Privacy

- trust_boundary:
- redaction:
- secret_handling:
- local_path_or_log_handling:
- plugin_mcp_hook_permissions:
- prompt_invisible_diagnostics:

## Reviewer Verdict

- final_verdict: ready / blocked / not-applicable
- reviewer_findings:
- required_follow_up:
- final_notes:

