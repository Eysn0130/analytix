# Context Epoch Foundation Hard Gate Evidence

This artifact is required before implementation of
`context-epoch-foundation`. Missing measured evidence means `blocked`, not
`ready`.

## Metadata

- change_id: `context-epoch-foundation`
- capability_id: `context.epoch`
- row_source:
  `/Users/sun/Projects/_upstreams/analytix-absorption/matrix/row-level-admission-gates.md`
- prepared_by: Codex
- prepared_at: 2026-07-03
- measured_at: 2026-07-10
- review_status: accepted
- reviewer: Codex

## Gate Status

- verdict: accepted
- blocked_reason: none; archive remains the final lifecycle action.
- gateCommand: `npm run runtime:go:speed-cache-gate -- --json` plus focused
  Context Epoch Go tests for inactive-source, activated-source, compact, and
  restart behavior.
- cacheEvidenceArtifact: measured baseline is recorded inline below from the
  local deterministic provider fixture; no transient output file is treated as
  durable evidence.
- context_epoch_required: true
- plugin_or_skill_admission_required: false

## Prompt Boundary

- prompt_boundary: dynamic-context
- stable_prefix_impact: none for default no-source path; explicit epoch bump
  required for any stable-prefix source.
- dynamic_context_impact: selected bounded content may enter the active turn
  only when a source is activated.
- turn_tail_content: allowed only for selected bounded active-turn content.
- store_only_state: source registry, source digests, activation state, trust
  state, token budgets, unavailable-source state.
- ui_only_state: not in scope.
- diagnostics_only_state: prefix-change reasons, hash comparisons, telemetry
  source labels.
- content_forbidden_from_prompt: registry metadata, raw source content unless
  explicitly selected and bounded, raw local paths, logs, hook output, plugin
  catalog metadata, full child transcripts, unavailable-source raw data.

## Prompt And Cache Evidence

- normal_turn_token_delta: before=`100`, after=`100`, delta=`0`
  provider-reported prompt tokens from the deterministic local fixture. This
  fixture count is not presented as credentialed live-provider tokenization.
- activated_turn_token_delta: dynamic-context adds `173` deterministic request
  bytes / `44` estimated token units; turn-tail adds a separate `158` bytes /
  `40` estimated token units in the focused fixture. These are deterministic
  local estimates, not credentialed provider tokenization.
- system_hash_before: `dad3771ea3fc9652`.
- system_hash_after: `dad3771ea3fc9652` for the default no-source path.
- prefix_hash_before: `f17bc8e74c321939`.
- prefix_items_hash_before: `4f53cda18c2baa0c`.
- prefix_hash_after: `f17bc8e74c321939`, byte-identical to before for the
  default no-source path.
- prefix_items_hash_after: `4f53cda18c2baa0c`, byte-identical to before for
  the default no-source path.
- prefix_change_reasons: empty for default; inactive registry-only changes emit
  sanitized Context Epoch reasons with `diagnosticsOnly=true` and do not change
  provider prefix hashes; stable-prefix activation requires a trusted source
  and an explicit accepted epoch change.
- tool_schema_hash_before: `e96a72437f7a1680` (`toolsHash`), with `17` tools
  and `3030` deterministic schema-token units.
- tool_schema_hash_after: `e96a72437f7a1680`, unchanged with `17` tools and
  `3030` deterministic schema-token units.
- tool_schema_change_reasons: target is none.
- cache_telemetry_source: provider-native fields when available; otherwise
  `unknown` with reason.
- cache_telemetry_unknown_reason: not applicable to the deterministic fixture,
  which supplied provider usage fields; credentialed live telemetry was not
  requested and no live-provider cache claim is made.
- deepseek_tail_cache_avg: not claimed in this proposal unless credentialed
  live evidence is added.
- first_visible_token_latency: before=`69ms`, after=`69ms`, threshold
  `<=250ms`.
- e2e_turn_latency: before=`280ms`, after=`285ms`; the `5ms` local timing
  variation did not change request/prefix/schema bytes.

### Measured Default No-Source Request

- command: `npm run runtime:go:speed-cache-gate -- --json`
- result: passed, `18/18` command checks and `12/12` metrics, zero failures.
- deterministic performance fixture command duration: `2799ms` after fixing
  temporary runtime process cleanup.
- runtime launch mode: `go-build-production-tag`; no packaged binary or live
  credential was used for this baseline.
- request URL: `/chat/completions`.
- request body fields: `messages`, `model`, `reasoning_effort`, `stream`,
  `stream_options`, `thinking`, `tools`.
- request message count: `2`.
- request tool count: `17`.
- request body digest: `3b6c8a566d4b8a0c236041303f2cd2689f20e1b809f3a14858ec04e67f297ada`.
- cache fixture usage: prompt `100`, completion `8`, hit `64`, miss `36`;
  these are deterministic fixture values, not a credentialed provider claim.
- repeated local runs produced the same system, prefix, prefix-items, tool
  schema, request-field, and body digests.

### Focused Fixture Decision

The existing gate covered runtime request shape, prefix/tool hashes, provider
cache mapping, and renderer streaming, but originally omitted
`prefixItemsHash`, schema size/count, and prompt-token evidence. The fixture now
emits and gates those fields. `go-context-epoch-cache-contract` is now the
nineteenth gate check and `context_epoch_default_stable` the thirteenth metric;
they compare default/inactive request behavior, measure activated dynamic and
turn-tail context separately, and cover compact/restart reconciliation. The
post-implementation gate passed `19/19` checks and `13/13` metrics.

## Context And Compaction

- compact_frequency: one changed compaction advances the accepted epoch once;
  immediately re-compacting the generated summary is a deterministic no-op.
- summary_model_call_count: unchanged (`0` additional calls) for default and
  deterministic compaction paths.
- consecutive_compacts: focused app and durable-store tests prove the second
  unchanged compact does not create a new summary or epoch.
- post_compact_cache_recovery_turns: `0` extra reconciliation turns; the compact
  recovery digest is accepted atomically and preserved by the next normal
  request boundary. No live-provider cache recovery curve is claimed.
- source_registry_or_epoch_impact: adds thread-scoped source registry and
  accepted epoch snapshots.
- tool_output_budget_impact: none in this change; future dependency.
- recovery_or_restart_behavior: restore last accepted epoch metadata before
  request construction; mark unavailable sources without raw reconstruction.

## UI And Runtime Bridge

- ui_same_prompt_ab: not-applicable for this change unless a UI surface is
  added during implementation, which should require scope review.
- opened_surface: not-applicable.
- closed_surface: not-applicable.
- prompt_history_same: true for the measured default no-source request.
- prefix_hash_same: true for the measured default no-source request.
- tool_schema_hash_same: true for the measured default no-source request.
- runtime_request_shape_same: true; request URL, seven body fields, two
  messages, seventeen tools, and body digest remained byte-identical.

## Agent Handoff And Review

- task_brief_source:
  `docs/analytix/upstreams/absorption-admission-gate/agent-task-brief-template.md`
- report_artifacts: required if subagents are used.
- review_artifacts:
  `docs/analytix/upstreams/absorption-admission-gate/agent-review-package-template.md`
- progress_ledger:
  `docs/analytix/upstreams/absorption-admission-gate/agent-progress-ledger-template.md`
- subagent_context_policy: bounded task brief plus exact input paths only.
- forbidden_handoff_content: controller transcript, full upstream documents,
  raw child transcripts, raw diff pasted into parent context, broad
  accumulated summaries.
- superpowers_usage: reference-only
- superpowers_install_status: not-installed, as required by this change.

## Security And Privacy

- trust_boundary: thread-scoped registry; no untrusted hooks/plugins/MCP
  changes in this proposal.
- redaction: sanitized source ids/digests and reason codes only.
- secret_handling: source registry MUST NOT persist secrets in
  provider-visible context.
- local_path_or_log_handling: raw local paths/logs are diagnostics-only or
  redacted.
- plugin_mcp_hook_permissions: not-applicable.
- prompt_invisible_diagnostics: prefix reasons, cache telemetry source,
  unavailable-source state, registry metadata.

## Reviewer Verdict

- final_verdict: accepted
- reviewer_findings: before/after request, prefix, schema, default token, active
  source, compact, restart, corruption, and unavailable-source fixtures pass;
  ordinary, `analytix_prod`, and race Go suites pass. Repo-local absorption
  matrices and the exact-path review package are updated and pass review.
- required_follow_up:
  1. Archive this change and synchronize its accepted capability spec.
  2. Keep future context consumers independently gated; this acceptance does
     not admit memory, instructions, inbox, plugin/skill, MCP, or UI features.
- final_notes: Capability evidence is accepted. This verdict authorizes archive
  of this dependency change, not completion of the parent Goal.
