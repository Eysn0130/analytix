# Admission Gate Validation Examples

These examples prove the accepted evidence shape can represent a low-risk
`not-applicable` verdict and a high-risk `blocked` verdict.

They are examples only; they do not approve implementation.

## Low-Risk Example: Docs Link Update

```yaml
change_id: example-docs-link-update
capability_id: research.audit
row_source: docs-only maintenance
verdict: not-applicable
blocked_reason: ""
prompt_boundary: store-only
stable_prefix_impact: none
dynamic_context_impact: none
turn_tail_content: none
store_only_state: accepted docs index link only
ui_only_state: none
diagnostics_only_state: none
normal_turn_token_delta: 0
activated_turn_token_delta: 0
prefix_hash_before: not-applicable
prefix_hash_after: not-applicable
tool_schema_hash_before: not-applicable
tool_schema_hash_after: not-applicable
cache_telemetry_source: not-applicable
ui_same_prompt_ab: not-applicable
task_brief_source: not-applicable
report_artifacts:
  - docs/analytix/upstreams/absorption-admission-gate/README.md
review_artifacts:
  - openspec/changes/absorption-admission-gate/specs/absorption-admission-gate/spec.md
superpowers_usage: reference-only
superpowers_install_status: blocked
final_verdict: not-applicable
final_notes: Docs-only index maintenance does not touch runtime, prompt, UI, provider, tool schema, MCP, hooks, memory, or subagents.
```

## High-Risk Example: Context Epoch Foundation

```yaml
change_id: context-epoch-foundation
capability_id: context.epoch
row_source: matrix/construction-readiness-index.md
verdict: blocked
blocked_reason: Context Epoch design and prefix before/after evidence are not yet accepted.
gateCommand: npm run runtime:go:speed-cache-gate
cacheEvidenceArtifact: blocked
context_epoch_required: true
plugin_or_skill_admission_required: false
prompt_boundary: dynamic-context
stable_prefix_impact: unknown
dynamic_context_impact: source registry and epoch snapshot would affect context admission
turn_tail_content: unknown
store_only_state: proposed durable source registry
ui_only_state: none
diagnostics_only_state: prefix-change reason codes
content_forbidden_from_prompt:
  - full upstream docs
  - raw project rule dumps
normal_turn_token_delta: unknown
activated_turn_token_delta: unknown
prefix_hash_before: missing
prefix_hash_after: missing
tool_schema_hash_before: not-applicable
tool_schema_hash_after: not-applicable
cache_telemetry_source: unknown until gate run
cache_telemetry_unknown_reason: no accepted Context Epoch implementation or benchmark artifact exists
ui_same_prompt_ab: not-applicable
superpowers_usage: reference-only
superpowers_install_status: blocked
final_verdict: blocked
required_follow_up:
  - create accepted Context Epoch design
  - attach prefix before/after evidence
  - measure normal and activated token deltas
  - run or explicitly block speed/cache gate
```

## Future P0 References

Future proposals must cite this directory before implementation:

- `context-epoch-foundation`: must start blocked until Context Epoch evidence
  exists.
- `tool-output-budget-and-prune`: must provide provider-visible output cap,
  full-output artifact/cursor behavior, and tool-call/result pair preservation
  evidence.

