#!/usr/bin/env node

import { existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'
import process from 'node:process'

const REPO_ROOT = process.cwd()
const MATRIX_PATH = 'docs/analytix/upstreams/runtime-closure-p0-p1-matrix.md'
const UPSTREAM_SOURCE_MANIFEST_PATH = 'docs/analytix/upstreams/upstream-sources.json'
const HASH_RE = /^[0-9a-f]{40}$/i
const OPEN_MATRIX_STATUS_RE = /^(needs_fix|blocked)\b/i
const OPEN_MATRIX_STATUS_TOKEN_RE = /\b(needs_fix|blocked|todo|tbd)\b/i
const ALLOWED_MATRIX_STATUSES = new Set(['fixed', 'acceptable_by_contract', 'blocked'])
const PLACEHOLDER_REGRESSION_RE = /^(n\/a|not applicable|same\b.*\babove\b|same focused.*|tbd|todo|-+)$/i
const SOURCE_REF_RE = /^(?<path>(?:\/[^`:\n]+|(?:[A-Za-z0-9_.-]+\/)*[A-Za-z0-9_.-]+)\.(?:go|ts|tsx|js|mjs|cjs|json|md|yaml|yml)):(?<line>[1-9]\d*)$/
const DEFAULT_UPSTREAM_ROOT = process.env.ANALYTIX_UPSTREAM_ROOT || '/Users/sun/Projects/_upstreams'

const SOURCE_REF_COLUMNS = [
  {
    id: 'analytix',
    column: 'analytix current evidence',
    requiredPerRow: true
  },
  {
    id: 'reasonix',
    column: 'Reasonix reference',
    requiredPerRow: false,
    minimumTotalRefs: 1
  },
  {
    id: 'opencode',
    column: 'OpenCode reference',
    requiredPerRow: false,
    minimumTotalRefs: 1
  },
  {
    id: 'codexdesktop',
    column: 'CodexDesktop reference',
    requiredPerRow: false,
    minimumTotalRefs: 1
  },
  {
    id: 'regression',
    column: 'Regression',
    requiredPerRow: true
  }
]

const UPSTREAM_COMPARISON_COLUMNS = SOURCE_REF_COLUMNS.filter((column) =>
  ['reasonix', 'opencode', 'codexdesktop'].includes(column.id)
)

const EXPLICIT_UPSTREAM_DECISION_RE =
  /\b(absorb(?:ed)?|already|analytix-owned|anti-pattern|auth bypass|boundary|comparison|does not|durable|equivalent|explicit|follows?|keeps?|marks?|mechanism|not (?:a|an|applicable|absorbed|imported|needed|the main source)|no (?:absorption|auth|runtime|upstream)|only|own(?:ed)?|preserv(?:e|ed|es)|references?|relevant|reject(?:ed|s)?|requires?|specific|stays?|treats?|without)\b/i

const VAGUE_UPSTREAM_CELL_RE = /^(same(?: as)? above|see above|done|ok|yes|no|n\/a|-+|tbd|todo)$/i

const REQUIRED_MATRIX_COLUMNS = [
  'Category',
  'analytix current evidence',
  'Reasonix reference',
  'OpenCode reference',
  'CodexDesktop reference',
  'Risk',
  'Absorb?',
  'Implementation / decision',
  'Regression',
  'Status'
]

const REQUIRED_MATRIX_DOMAINS = [
  {
    id: 'turn_lifecycle_terminal',
    label: 'Turn lifecycle terminal settle',
    pattern: /\b(turn lifecycle|terminal CAS|terminal settle|turn_failed|FinishTurn|FinishTurnIfActive|review turn|failed turn)\b/i
  },
  {
    id: 'interrupt_cancel',
    label: 'Interrupt/cancel execution',
    pattern: /\b(interrupt|cancel|aborted|kill|timeout|shutdown cancellation)\b/i
  },
  {
    id: 'sse_replay_heartbeat',
    label: 'SSE/replay/heartbeat',
    pattern: /\b(SSE|replay|heartbeat|lastSeq|recoverActiveTurn|high-water seq)\b/i
  },
  {
    id: 'provider_model_endpoint',
    label: 'Provider/model/endpoint',
    pattern: /\b(provider\/model|provider|model|endpoint|endpointFormat|ModelExecutionRef|semantic probe)\b/i
  },
  {
    id: 'agent_loop_step_recovery',
    label: 'Agent loop step/guard/recovery',
    pattern: /\b(agent loop|interrupted-stream|stream recovery|maxModelSteps|step-budget|post-output stream|recovery depth)\b/i
  },
  {
    id: 'durable_store_event_log_replay',
    label: 'Durable store/event log/replay',
    pattern: /\b(durable store|durable|thread\.json|events\.jsonl|event log|crash\/restart|record durability|partial assistant salvage)\b/i
  },
  {
    id: 'subagent_session_continuation',
    label: 'Subagent/session continuation',
    pattern: /\b(subagent|child-run|child run|child thread|continue\/fork|continue_from|fork_from|session identity)\b/i
  },
  {
    id: 'tools_mcp_plugin',
    label: 'Tools/MCP/plugin',
    pattern: /\b(tools?\/MCP|MCP|plugin|tool_call|tool result|tool-result|skill|manifest)\b/i
  },
  {
    id: 'renderer_terminal_timeline',
    label: 'Renderer terminal/timeline/pinned summary',
    pattern: /\b(Renderer|timeline|pinned summary|right-summary|SubagentSummary|SubagentInspector|recoverActiveTurn)\b/i
  },
  {
    id: 'main_preload_process_package',
    label: 'Main/preload/runtime process/package',
    pattern: /\b(preload|bridge|window\.analytix|runtime process|process boundary|packaged|after-pack|sidecar|SIGTERM|runtime-info)\b/i
  },
  {
    id: 'security_sandbox_redaction',
    label: 'Security/sandbox/secret redaction',
    pattern: /\b(secret|redaction|sandbox|approval boundary|protected read|auth bypass|Bearer|api key)\b/i
  }
]

const REQUIRED_NAMED_HAZARDS = [
  {
    id: 'partial_assistant_delta_durable_salvage',
    label: 'partial assistant delta is not live-only after failure/reload',
    sampleCategory: 'Go runtime partial assistant salvage',
    categoryPattern: /^Go runtime partial assistant salvage$/i,
    terms: [/partial assistant/i, /assistant_(?:text|reasoning)_delta|assistant deltas/i, /thread\.json|materializ/i],
    sample: 'partial assistant assistant_text_delta assistant_reasoning_delta thread.json materialized'
  },
  {
    id: 'turn_failure_event_log_error_visible',
    label: 'recordRuntimeTurnFailure append/event-log failure remains visible',
    sampleCategory: 'Go runtime failure record durability',
    categoryPattern: /^Go runtime failure record durability$/i,
    terms: [/failure record durability|TurnFailurePersistsErrorItemWhenEventLogCannotRecord|recordRuntimeTurnFailure/i, /event log|append|error item/i],
    sample: 'failure record durability TurnFailurePersistsErrorItemWhenEventLogCannotRecord event log append error item'
  },
  {
    id: 'interrupted_stream_recovery_depth',
    label: 'stream interruption recovery is bounded above one attempt',
    sampleCategory: 'Go interrupted-stream recovery depth',
    categoryPattern: /^Go interrupted-stream recovery depth$/i,
    terms: [/interrupted-stream|stream recovery|recovery depth/i, /3|three|Reasonix|maxStreamRecoveries/i],
    sample: 'interrupted-stream recovery depth 3 Reasonix maxStreamRecoveries'
  },
  {
    id: 'foreground_tool_mcp_shell_cancel',
    label: 'foreground provider/tool/MCP/shell execution responds to cancel',
    sampleCategory: 'Interrupt/cancel execution',
    categoryPattern: /^Interrupt\/cancel execution$|^Tools\/MCP pairing and cancel placeholders$/i,
    terms: [/Interrupt\/cancel execution|foreground provider|tool loop/i, /MCP|tool|shell|provider/i, /cancel|timeout|shutdown|aborted/i],
    sample: 'Interrupt/cancel execution foreground provider tool loop MCP shell cancel timeout shutdown'
  },
  {
    id: 'background_job_subagent_terminal',
    label: 'background bash/subagent jobs cannot remain orphan running',
    sampleCategory: 'Interrupt/cancel execution',
    categoryPattern: /^Interrupt\/cancel execution$|^Thread summary terminal projection$/i,
    terms: [/background/i, /shell|subagent|child-run|child run/i, /terminal|stale|running|timeout|shutdown/i],
    sample: 'background shell subagent child-run terminal stale running timeout shutdown'
  },
  {
    id: 'pending_gate_restart_or_interrupt',
    label: 'approval/user_input pending gates settle across restart/interrupt',
    sampleCategory: 'Turn lifecycle terminal CAS',
    categoryPattern: /^Turn lifecycle terminal CAS$|^Durable store crash\/restart recovery$|^Interrupt\/cancel execution$/i,
    terms: [/approval\/user[-_]input|approval\/user-input/i, /pending|gate|continuation/i, /restart|restore|interrupt|terminal/i],
    sample: 'approval/user_input pending gate continuation restart interrupt terminal'
  },
  {
    id: 'events_setup_error_terminal_contract',
    label: '/events setup error is structured and terminal',
    sampleCategory: 'SSE/replay setup error contract',
    categoryPattern: /^SSE\/replay setup error contract$/i,
    terms: [/SSE\/replay setup error|\/events|event:error|kind:"error"/i, /threadId/i, /seq/i, /timestamp/i, /terminal/i],
    sample: '/events SSE/replay setup error event:error kind:"error" threadId seq timestamp terminal'
  },
  {
    id: 'recover_active_turn_preserves_live_terminal',
    label: 'recoverActiveTurn cannot overwrite live terminal errors',
    sampleCategory: 'Renderer timeline / pinned summary settle',
    categoryPattern: /^Renderer timeline \/ pinned summary settle$/i,
    terms: [/recoverActiveTurn/i, /live terminal|newer live|terminal error/i],
    sample: 'recoverActiveTurn newer live terminal error'
  },
  {
    id: 'child_continuation_native_session_identity',
    label: 'child continue/fork uses native session identity, not transcript injection',
    sampleCategory: 'Subagent continue/fork session identity',
    categoryPattern: /^Subagent continue\/fork session identity$/i,
    terms: [/continue\/fork|continue_from|fork_from/i, /child thread|native child-run store|not plain prompt injection|transcript injection/i, /session identity|source identity/i],
    sample: 'continue/fork continue_from fork_from native child-run store child thread not plain prompt injection source identity'
  },
  {
    id: 'child_provider_model_source_identity',
    label: 'child continue/fork inherits source provider/model when parent changes',
    sampleCategory: 'Subagent continue/fork session identity',
    categoryPattern: /^Provider\/model inheritance$|^Subagent continue\/fork session identity$/i,
    terms: [/provider\/model|providerId|provider/i, /source identity|source provider|source child|parent/i, /model|endpoint|effort/i],
    sample: 'provider/model providerId source identity source provider source child parent model endpoint effort'
  },
  {
    id: 'mcp_tool_catalog_schema_cache_stability',
    label: 'plugin/MCP dynamic tool catalog cannot drift cache/tool-schema pairing',
    sampleCategory: 'MCP tool catalog / schema cache stability',
    categoryPattern: /^MCP tool catalog \/ schema cache stability$/i,
    terms: [/MCP|plugin|tool/i, /schema|catalog|description|cache|ToolReadOnlyHint/i, /pairing|configured|readOnlyHint|tool scope/i],
    sample: 'MCP plugin tool schema catalog description cache ToolReadOnlyHint pairing configured readOnlyHint tool scope'
  },
  {
    id: 'renderer_measurement_trace_pressure',
    label: 'renderer measurement/profiler trace is bounded and sanitized',
    sampleCategory: 'Renderer measurement / trace pressure guard',
    categoryPattern: /^Renderer measurement \/ trace pressure guard$/i,
    terms: [/measurement|ResizeObserver|virtualizer|trace|profiler/i, /batch|sanitize|drops string|high-frequency|IPC|disk/i],
    sample: 'measurement ResizeObserver virtualizer trace profiler batch sanitize drops string high-frequency IPC disk'
  },
  {
    id: 'packaged_runtime_resources_match_dev',
    label: 'packaged runtime resources/plugin manifests match dev boundary',
    sampleCategory: 'Packaged file-link dependency materialization',
    categoryPattern: /^Packaged runtime\/process boundary$|^Packaged file-link dependency materialization$|^Plugin manifest \/ MCP \/ skill runtime boundary$/i,
    terms: [/packaged|after-pack|runtime resources|file-link|manifest/i, /dev|packaged|plugin|runtime/i],
    sample: 'packaged after-pack runtime resources file-link manifest dev plugin runtime'
  },
  {
    id: 'thread_crud_terminal_cas_consistency',
    label: 'fork/resume/search/archive/thread list remain consistent after terminal CAS',
    sampleCategory: 'Turn lifecycle terminal CAS',
    categoryPattern: /^Turn lifecycle terminal CAS$|^Interrupt discard history isolation$/i,
    terms: [/fork\/resume|fork|resume|thread list|search|archive/i, /terminal CAS|terminal|history|summary/i],
    sample: 'fork/resume fork resume search archive thread list terminal CAS history summary'
  },
  {
    id: 'interrupted_tool_call_history_repair',
    label: 'interrupted tool_call history repair covers terminal item statuses',
    sampleCategory: 'Tools/MCP pairing and cancel placeholders',
    categoryPattern: /^Tools\/MCP pairing and cancel placeholders$|^Interrupt\/cancel execution$/i,
    terms: [/interrupted tool|tool_call|tool history|tool-result|tool result/i, /aborted|interrupted|killed|failed|running|cancel/i],
    sample: 'interrupted tool_call tool history tool-result aborted interrupted killed failed running cancel'
  }
]

const REQUIRED_ABSORPTION_MATRIX_ROWS = [
  {
    id: 'todo_completion_evidence',
    modulePattern: /TODO\s*\/\s*完成证据|todo.*completion.*evidence/i,
    label: 'TODO / 完成证据',
    terms: [/todo_write/i, /complete_step/i, /evidence ledger/i, /final readiness/i, /goal FSM/i],
    sample: 'todo_write complete_step evidence ledger final readiness goal FSM'
  },
  {
    id: 'session_todo_ui',
    modulePattern: /Session todo UI/i,
    label: 'Session todo UI',
    terms: [/session todo|activeThreadTodos|TodoPanel/i, /store|event|dock|above composer/i],
    sample: 'Session todo UI activeThreadTodos TodoPanel store event dock above composer'
  },
  {
    id: 'subagent_continuation_fork',
    modulePattern: /Subagent continuation\/fork/i,
    label: 'Subagent continuation/fork',
    terms: [/SubagentStore|PrepareFresh|PrepareContinue|RunSubAgentWithSession|continue_from|fork_from/i, /hash identity guard|identity/i],
    sample: 'SubagentStore PrepareFresh PrepareContinue RunSubAgentWithSession continue_from fork_from hash identity guard'
  },
  {
    id: 'subagent_profile_model_inheritance',
    modulePattern: /Subagent profile\s*\/\s*model inheritance/i,
    label: 'Subagent profile / model inheritance',
    terms: [/Agent\.Info|profile/i, /mode:\s*primary\/subagent\/all|primary\/subagent\/all/i, /permission|model inheritance|ModelExecutionRef/i],
    sample: 'Agent.Info profile mode: primary/subagent/all permission model inheritance ModelExecutionRef'
  },
  {
    id: 'parallel_tasks',
    modulePattern: /parallel_tasks/i,
    label: 'parallel_tasks',
    terms: [/dependency/i, /wave/i, /partial aggregate|aggregate/i, /handoff/i],
    sample: 'parallel_tasks dependency validation wave execution partial aggregate handoff'
  },
  {
    id: 'background_jobs',
    modulePattern: /background jobs/i,
    label: 'background jobs',
    terms: [/session-scoped|background/i, /wait/i, /output/i, /kill/i, /promote|extend|UX/i],
    sample: 'background jobs session-scoped wait output kill promote extend UX'
  },
  {
    id: 'provider_wire_openai_compatible',
    modulePattern: /Provider wire\s*\/\s*OpenAI-compatible/i,
    label: 'Provider wire / OpenAI-compatible',
    terms: [/stream reconnect|interrupted-stream/i, /usage parsing|usage/i, /history repair/i, /schema canonicalize|canonical/i],
    sample: 'Provider wire OpenAI-compatible stream reconnect usage parsing history repair schema canonicalize'
  },
  {
    id: 'model_resolver_variant',
    modulePattern: /Model resolver\s*\/\s*variant/i,
    label: 'Model resolver / variant',
    terms: [/Model\.Ref|ModelExecutionRef/i, /providerID|providerId/i, /variant/i, /default model/i],
    sample: 'Model.Ref ModelExecutionRef providerID providerId id variant default model'
  },
  {
    id: 'tool_registry_settlement',
    modulePattern: /Tool registry\s*\/\s*settlement/i,
    label: 'Tool registry / settlement',
    terms: [/materialize/i, /identity/i, /stale tool call/i, /durable settlement|tool result/i],
    sample: 'Tool registry settlement materialize identity stale tool call durable settlement tool result'
  },
  {
    id: 'cache_usage',
    modulePattern: /Cache\/usage/i,
    label: 'Cache/usage',
    terms: [/cache fields|cacheHit|cacheMiss/i, /read\/write|read|write/i, /non-cached|cacheMiss/i, /breakdown/i],
    sample: 'Cache usage cache fields cacheHit cacheMiss read/write non-cached breakdown'
  },
  {
    id: 'vision_unsupported_multimodal',
    modulePattern: /Vision\s*\/\s*unsupported multimodal/i,
    label: 'Vision / unsupported multimodal',
    terms: [/text-only/i, /base64/i, /unsupported media|unsupported/i, /vision bridge/i],
    sample: 'Vision unsupported multimodal text-only base64 unsupported media vision bridge'
  },
  {
    id: 'plugins_skills_mcp_native',
    modulePattern: /插件\/skills\/MCP\/native|plugins\/skills\/MCP\/native/i,
    label: '插件/skills/MCP/native',
    terms: [/resource layout|manifest/i, /catalog/i, /permission|trust/i, /plugin graph|MCP|skills/i],
    sample: 'plugins skills MCP native resource layout manifest catalog permission trust plugin graph'
  }
]

const REQUIRED_VALIDATION_LEDGER = [
  {
    id: 'go_runtime_all',
    label: 'Go runtime full test suite',
    pattern: /^go test \.\/\.\.\.(?:\s|$)/
  },
  {
    id: 'go_runtime_server',
    label: 'Go runtime internal server suite',
    pattern: /^go test \.\/internal\/server(?:\s|$)/
  },
  {
    id: 'npm_test_all',
    label: 'npm full test suite',
    pattern: /^npm run test(?:\s|$)/
  },
  {
    id: 'required_runtime_vitest_group',
    label: 'required runtime/delegation/provider Vitest group',
    pattern: /^npx vitest run\s+packages\/runtime\/tests\/child-agent-executor\.test\.ts\s+packages\/runtime\/tests\/delegation-runtime\.test\.ts\s+packages\/runtime\/tests\/task-job-orchestration-contract\.test\.ts\s+packages\/runtime\/src\/adapters\/model\/multi-provider-model-client\.test\.ts\s+packages\/runtime\/src\/adapters\/model\/compat-model-client\.endpoint-format\.test\.ts\s+packages\/runtime\/tests\/thread-summary-route\.test\.ts(?:\s|$)/
  },
  {
    id: 'required_renderer_plugin_vitest_group',
    label: 'required renderer/plugin/preload Vitest group',
    pattern: /^npx vitest run\s+src\/renderer\/src\/agent\/analytix-mapper\.test\.ts\s+src\/renderer\/src\/agent\/analytix-runtime\.test\.ts\s+src\/renderer\/src\/components\/PluginMarketplaceView\.test\.ts\s+src\/main\/services\/hub-agent-marketplace-service\.test\.ts\s+packages\/runtime\/tests\/skill-runtime\.test\.ts\s+packages\/runtime\/tests\/mcp-tool-provider\.test\.ts\s+src\/main\/ipc\/app-ipc-schemas\.test\.ts\s+src\/preload\/preload-runtime-request\.test\.ts(?:\s|$)/
  },
  {
    id: 'typecheck',
    label: 'TypeScript typecheck',
    pattern: /^npm run typecheck(?:\s|$)/
  },
  {
    id: 'build_runtime',
    label: 'runtime package build',
    pattern: /^npm run build:runtime(?:\s|$)/
  },
  {
    id: 'product_regression',
    label: 'Go runtime product regression',
    pattern: /^npm run runtime:go:product-regression -- --json(?:\s|$)/
  },
  {
    id: 'product_sovereignty',
    label: 'product sovereignty scan',
    pattern: /^npm run scan:product-sovereignty(?:\s|$)/
  },
  {
    id: 'speed_cache_gate',
    label: 'speed/cache gate',
    pattern: /^npm run runtime:go:speed-cache-gate(?:\s|$)/
  },
  {
    id: 'build',
    label: 'production build',
    pattern: /^npm run build(?:\s|$)/
  },
  {
    id: 'dist_mac_arm64',
    label: 'macOS arm64 packaged app build',
    pattern: /^npm run dist:mac:arm64(?:\s|$)/
  },
  {
    id: 'packaged_gui_smoke',
    label: 'packaged GUI runtime smoke',
    pattern: /^npm run runtime:go:packaged-gui-smoke -- --json --actual --timeout-ms 60000(?:\s|$)/
  },
  {
    id: 'packaged_soak',
    label: 'packaged runtime soak',
    pattern: /^npm run runtime:go:packaged-soak -- --json --actual --no-write --timeout-ms 90000(?:\s|$)/
  },
  {
    id: 'lint',
    label: 'lint',
    pattern: /^npm run lint(?:\s|$)/
  },
  {
    id: 'matrix_audit',
    label: 'runtime closure matrix audit',
    pattern: /^node scripts\/runtime-closure-matrix-audit\.mjs --strict-local --json(?:\s|$)/
  },
  {
    id: 'phase4_release_guard',
    label: 'fund-analysis phase4 release guard',
    pattern: /^node plugins\/analytix-fund-analysis\/scripts\/phase4-diagnostics\.mjs --release-guard --json(?:\s|$)/
  },
  {
    id: 'release_slice_summary',
    label: 'release-slice bounded summary',
    pattern: /^node plugins\/analytix-fund-analysis\/scripts\/release-slice-audit\.mjs --summary-json --staging-plan --readiness-plan(?:\s|$)/
  },
  {
    id: 'release_slice_unrelated_dirty_groups_self_test',
    label: 'release-slice unrelated dirty grouping self-test',
    pattern: /^node plugins\/analytix-fund-analysis\/scripts\/release-slice-audit\.mjs --self-test-unrelated-dirty-groups --json(?:\s|$)/
  },
  {
    id: 'fund_analysis_doctor',
    label: 'fund-analysis doctor',
    pattern: /^node\s+plugins\/analytix-fund-analysis\/scripts\/doctor\.mjs(?:\s|$)/
  },
  {
    id: 'fund_analysis_functional_eval',
    label: 'fund-analysis functional eval',
    pattern: /^node\s+plugins\/analytix-fund-analysis\/scripts\/functional-eval\.mjs(?:\s|$)/
  },
  {
    id: 'fund_analysis_mcp_return_oracle',
    label: 'fund-analysis MCP return oracle',
    pattern: /^node\s+plugins\/analytix-fund-analysis\/scripts\/mcp-return-oracle\.mjs(?:\s|$)/
  },
  {
    id: 'diff_check',
    label: 'whitespace diff check',
    pattern: /^git diff --check(?:\s|$)/
  }
]

const REQUIRED_RELEASE_BLOCKER_DISCLOSURES = [
  {
    id: 'phase4_release_guard_command',
    label: 'phase4 release guard command is recorded',
    pattern: /node plugins\/analytix-fund-analysis\/scripts\/phase4-diagnostics\.mjs --release-guard --json/
  },
  {
    id: 'release_slice_summary_command',
    label: 'release-slice summary/readiness command is recorded',
    pattern: /node plugins\/analytix-fund-analysis\/scripts\/release-slice-audit\.mjs --summary-json --staging-plan --readiness-plan/
  },
  {
    id: 'phase4_release_guard_summary',
    label: 'phase4 release guard pass/warn/fail summary is recorded',
    pattern: /(?:106\s+(?:passed|pass)[\s\S]{0,160}0\s+(?:warnings?|warn)[\s\S]{0,160}(?:only\s+)?2\s+(?:failures?|fail))|(?:phase4_release_guard_summary[^`\n]*(?:pass(?:ed)?[:=]\s*106|106\s+pass)[^`\n]*(?:warn(?:ings)?[:=]\s*0|0\s+warn)[^`\n]*(?:fail(?:ures)?[:=]\s*2|2\s+fail))/i
  },
  {
    id: 'phase4_release_guard_only_failures',
    label: 'phase4 release guard only-failure ids are recorded',
    pattern: /(?:only\s+(?:2\s+)?(?:failures?|release-operation failures?)[\s\S]{0,240}release_git_tag_identity[\s\S]{0,240}release_git_worktree_clean)|(?:phase4_release_guard_failures[^`\n]*release_git_tag_identity[^`\n]*release_git_worktree_clean)/i
  },
  {
    id: 'release_tag_blocker',
    label: 'missing fund-analysis release tag blocker is named',
    pattern: /analytix-fund-analysis-v0\.16\.10[^`|\n]*(?:missing|release tag missing)|release_git_tag_identity/i
  },
  {
    id: 'dirty_worktree_blocker',
    label: 'dirty worktree release blocker is named',
    pattern: /release_git_worktree_clean|dirty worktree|dirty\/tag worktree/i
  },
  {
    id: 'publish_blocker_count',
    label: 'publish blocker count is recorded',
    pattern: /publish_blocker_count:5|publish_blocker_count`\s*:\s*5/i
  },
  {
    id: 'tracked_dirty_count',
    label: 'tracked dirty path count is recorded',
    pattern: /tracked_dirty_count:\d+|tracked_dirty_count`\s*:\s*\d+|\d+ dirty tracked paths|\d+ dirty worktree path/i
  },
  {
    id: 'generated_evidence_dirty_count',
    label: 'generated evidence dirty count is recorded',
    pattern: /generated_evidence_dirty_count:\d+|generated_evidence_dirty_count`\s*:\s*\d+|\d+ generated evidence paths/i
  },
  {
    id: 'release_slice_dirty_count',
    label: 'release-slice dirty count is recorded',
    pattern: /release_slice_dirty_count:\d+|release_slice_dirty_count`\s*:\s*\d+|(?:dirty\.)?release_slice`\s*:\s*\d+|\d+ (?:related )?release-slice dirty paths/i
  },
  {
    id: 'unrelated_dirty_count',
    label: 'unrelated dirty count is recorded',
    pattern: /unrelated_dirty_count:\d+|unrelated_dirty_count`\s*:\s*\d+|\d+ unrelated dirty paths/i
  },
  {
    id: 'unrelated_dirty_group_count',
    label: 'unrelated dirty grouping count is recorded',
    pattern: /goal_wide_unrelated_dirty_count:\d+|goal_wide_unrelated_dirty_count`\s*:\s*\d+/i
  },
  {
    id: 'unknown_unrelated_zero',
    label: 'unknown unrelated dirty bucket is explicitly zero',
    pattern: /unknown_unrelated_dirty_count:0|unknown_unrelated_dirty_count`\s*:\s*0/i
  },
  {
    id: 'staging_ready_for_authorized_stage',
    label: 'release-slice authorized staging readiness is recorded',
    pattern: /ready_for_authorized_stage:true|ready_for_authorized_stage`\s*:\s*true/i
  },
  {
    id: 'staging_path_count',
    label: 'release-slice stage path count is recorded',
    pattern: /stage_path_count:\d+|stage_path_count`\s*:\s*\d+/i
  },
  {
    id: 'staging_leave_unstaged_count',
    label: 'release-slice leave-unstaged count is recorded',
    pattern: /leave_unstaged_count:\d+|leave_unstaged_count`\s*:\s*\d+/i
  },
  {
    id: 'readiness_blocker_count',
    label: 'release readiness blocker count is recorded',
    pattern: /readiness_plan\.blocker_count:3|readiness_plan\.blocker_count`\s*:\s*3/i
  },
  {
    id: 'readiness_candidate_blocker_count',
    label: 'release candidate blocker count is recorded',
    pattern: /readiness_plan\.candidate_blocker_count:\d+|readiness_plan\.candidate_blocker_count`\s*:\s*\d+/i
  },
  {
    id: 'authorization_required',
    label: 'release authorization requirement is recorded',
    pattern: /authorization|stage|commit|tag|push|Hub|marketplace/i
  }
]

const REQUIRED_PACKAGED_RUNTIME_DISCLOSURES = [
  {
    id: 'packaged_gui_actual_launch',
    label: 'packaged GUI smoke records actual app launch',
    pattern: /packaged_gui_smoke\.actualPackagedAppLaunched:true|actualPackagedAppLaunched:true|actualPackagedAppLaunched`\s*:\s*true/i
  },
  {
    id: 'packaged_gui_window_analytix',
    label: 'packaged GUI smoke records window.analytix bridge presence',
    pattern: /packaged_gui_smoke\.windowAnalytixPresent:true|window\.analytix[^`\n]*(?:present|available)|windowAnalytixPresent`\s*:\s*true/i
  },
  {
    id: 'packaged_gui_legacy_aliases_absent',
    label: 'packaged GUI smoke records legacy bridge aliases absent',
    pattern: /packaged_gui_smoke\.legacyAliasesAbsent:true|legacy\s+(?:Kun\/Reasonix\s+)?aliases?\s+absent|legacyAliasesAbsent`\s*:\s*true/i
  },
  {
    id: 'packaged_gui_runtime_info_reachable',
    label: 'packaged GUI smoke records runtime restart/health/runtime-info reachability',
    pattern: /packaged_gui_smoke\.runtimeInfoReachable:true|runtime restart\/health\/runtime-info reachable|runtimeInfoReachable`\s*:\s*true/i
  },
  {
    id: 'packaged_gui_no_bundled_go_source',
    label: 'packaged GUI smoke records no bundled Go source',
    pattern: /packaged_gui_smoke\.noBundledGoSource:true|no bundled Go source|noBundledGoSource`\s*:\s*true/i
  },
  {
    id: 'packaged_soak_thread_turn_sse_replay',
    label: 'packaged soak records actual thread/turn and SSE replay',
    pattern: /packaged_soak\.threadTurnSseReplay:true|actual packaged thread\/turn creation[\s\S]{0,240}SSE replay|threadTurnSseReplay`\s*:\s*true/i
  },
  {
    id: 'packaged_soak_tool_timeline_continuations',
    label: 'packaged soak records tool timeline and approval/user_input continuation coverage',
    pattern: /packaged_soak\.toolTimelineContinuations:true|tool timeline[\s\S]{0,280}approval\/user_input continuation|toolTimelineContinuations`\s*:\s*true/i
  },
  {
    id: 'packaged_soak_provider_redaction',
    label: 'packaged soak records provider redaction coverage',
    pattern: /packaged_soak\.providerRedaction:true|provider redaction covered|providerRedaction`\s*:\s*true/i
  }
]

const REQUIRED_PRODUCT_REGRESSION_DISCLOSURES = [
  {
    id: 'product_regression_status_passed',
    label: 'product regression status is recorded as passed',
    pattern: /runtime_go_product_regression\.status:passed|runtime-go-product-regression[^`\n]*(?:status|passed)[^`\n]*passed|product_regression\.status`\s*:\s*["']?passed/i
  },
  {
    id: 'product_regression_matrix_row_count',
    label: 'product regression matrix row count is recorded',
    pattern: /runtime_go_product_regression\.rowCount:35|runtime_go_product_regression\.matrix\.rowCount:35|35 product matrix features|rowCount`\s*:\s*35/i
  },
  {
    id: 'product_regression_runtime_closure_matrix_audit',
    label: 'product regression records runtime-closure matrix audit pass',
    pattern: /runtime_go_product_regression\.runtimeClosureMatrixAudit:passed|runtime-closure-matrix-audit[^`\n]*(?:status|passed)[^`\n]*passed|runtimeClosureMatrixAudit`\s*:\s*["']?passed/i
  },
  {
    id: 'product_regression_speed_cache_gate',
    label: 'product regression records speed/cache gate pass',
    pattern: /runtime_go_product_regression\.speedCacheGate:passed|p0-speed-cache-gate[^`\n]*(?:status|passed)[^`\n]*passed|speedCacheGate`\s*:\s*["']?passed/i
  },
  {
    id: 'product_regression_packaged_gui_contract',
    label: 'product regression records packaged GUI contract pass',
    pattern: /runtime_go_product_regression\.packagedGuiContract:passed|p0-packaged-gui-smoke-contract[^`\n]*(?:status|passed)[^`\n]*passed|packagedGuiContract`\s*:\s*["']?passed/i
  },
  {
    id: 'product_regression_packaged_soak_contract',
    label: 'product regression records packaged session soak contract pass',
    pattern: /runtime_go_product_regression\.packagedSoakContract:passed|p0-packaged-session-soak-contract[^`\n]*(?:status|passed)[^`\n]*passed|packagedSoakContract`\s*:\s*["']?passed/i
  },
  {
    id: 'product_regression_product_sovereignty',
    label: 'product regression records product-sovereignty scan pass',
    pattern: /runtime_go_product_regression\.productSovereignty:passed|product-sovereignty-scan[^`\n]*(?:status|passed)[^`\n]*passed|productSovereignty`\s*:\s*["']?passed/i
  }
]

function defaultUpstreamRepoPath(repoName, legacyPath) {
  const candidate = join(DEFAULT_UPSTREAM_ROOT, repoName)
  return existsSync(candidate) ? candidate : legacyPath
}

const UPSTREAMS = [
  {
    id: 'reasonix',
    manifestId: 'deepseek-reasonix',
    label: 'Reasonix',
    matrixName: 'Reasonix `origin/main-v2`',
    repoEnv: 'ANALYTIX_UPSTREAM_REPO_REASONIX',
    repoPath: defaultUpstreamRepoPath('DeepSeek-Reasonix', '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    ref: 'origin/main-v2'
  },
  {
    id: 'opencode',
    manifestId: 'opencode',
    label: 'OpenCode',
    matrixName: 'OpenCode `origin/dev`',
    repoEnv: 'ANALYTIX_UPSTREAM_REPO_OPENCODE',
    repoPath: defaultUpstreamRepoPath('opencode', '/Users/sun/Projects/opencode'),
    ref: 'origin/dev'
  },
  {
    id: 'codexdesktop',
    manifestId: 'codexdesktop-rebuild',
    label: 'CodexDesktop-Rebuild',
    matrixName: 'CodexDesktop-Rebuild `origin/master`',
    repoEnv: 'ANALYTIX_UPSTREAM_REPO_CODEX_DESKTOP_REBUILD',
    repoPath: defaultUpstreamRepoPath('CodexDesktop-Rebuild', '/Users/sun/Projects/CodexDesktop-Rebuild'),
    ref: 'origin/master'
  }
]

const REQUIRED_UPSTREAM_FILE_REFS = [
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix agent loop',
    path: 'internal/agent/agent.go'
  },
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix task/subagent runner',
    path: 'internal/agent/task.go'
  },
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix subagent store',
    path: 'internal/agent/subagent_store.go'
  },
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix ACP cancel/session service',
    path: 'internal/acp/service.go'
  },
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix controller cancel/session lifecycle',
    path: 'internal/control/controller.go'
  },
  {
    source: 'reasonix',
    column: 'Reasonix reference',
    label: 'Reasonix serve SSE/cancel',
    path: 'internal/serve/serve.go'
  },
  {
    source: 'opencode',
    column: 'OpenCode reference',
    label: 'OpenCode model ref schema',
    path: 'packages/schema/src/model.ts'
  },
  {
    source: 'opencode',
    column: 'OpenCode reference',
    label: 'OpenCode session prompt model inheritance',
    path: 'packages/opencode/src/session/prompt.ts'
  },
  {
    source: 'opencode',
    column: 'OpenCode reference',
    label: 'OpenCode durable assistant/tool history',
    path: 'packages/opencode/src/session/message-v2.ts'
  },
  {
    source: 'opencode',
    column: 'OpenCode reference',
    label: 'OpenCode task tool child session inheritance',
    path: 'packages/opencode/src/tool/task.ts'
  },
  {
    source: 'codexdesktop',
    column: 'CodexDesktop reference',
    label: 'CodexDesktop bundled plugin marketplace',
    path: 'src/mac-arm64/plugins/openai-bundled/.agents/plugins/marketplace.json'
  },
  {
    source: 'codexdesktop',
    column: 'CodexDesktop reference',
    label: 'CodexDesktop bundled plugin manifest',
    path: 'src/mac-arm64/plugins/openai-bundled/plugins/browser/.codex-plugin/plugin.json'
  },
  {
    source: 'codexdesktop',
    column: 'CodexDesktop reference',
    label: 'CodexDesktop resource copy script',
    path: 'scripts/sync-upstream.js'
  },
  {
    source: 'codexdesktop',
    column: 'CodexDesktop reference',
    label: 'CodexDesktop rejected auth bypass patch',
    path: 'scripts/patch-plugin-auth.js'
  }
]

function parseArgs(argv) {
  const options = {
    json: false,
    strictLocal: false,
    selfTest: false,
    matrixPath: MATRIX_PATH
  }
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    if (arg === '--json') options.json = true
    else if (arg === '--strict-local') options.strictLocal = true
    else if (arg === '--self-test') options.selfTest = true
    else if (arg === '--matrix') {
      options.matrixPath = String(argv[index + 1] || '').trim()
      index += 1
    } else if (arg.startsWith('--matrix=')) {
      options.matrixPath = String(arg.slice('--matrix='.length)).trim()
    } else if (arg === '-h' || arg === '--help') {
      printHelp()
      process.exit(0)
    } else {
      throw new Error(`Unknown argument: ${arg}`)
    }
  }
  return options
}

function printHelp() {
  console.log(`Usage: node scripts/runtime-closure-matrix-audit.mjs [options]

Options:
  --json            Print machine-readable JSON.
  --strict-local    Fail when a configured local upstream repo/ref is missing or stale.
  --matrix <path>   Matrix markdown path. Default: ${MATRIX_PATH}
  --self-test       Run parser/comparison self-test without reading external repos.
`)
}

function parseUpstreamRefs(markdown) {
  const refs = new Map()
  const lines = String(markdown || '').split(/\r?\n/)
  let inSection = false
  for (const line of lines) {
    if (/^##\s+Upstream refs\s*$/i.test(line.trim())) {
      inSection = true
      continue
    }
    if (inSection && /^##\s+/.test(line.trim())) break
    if (!inSection) continue
    const match = /^\|\s*(.*?)\s*\|\s*`?([0-9a-fA-F]{40})`?\s*\|\s*$/.exec(line)
    if (!match) continue
    refs.set(match[1].trim(), match[2].toLowerCase())
  }
  return refs
}

function revParse(repoPath, ref) {
  const result = spawnSync('git', ['-C', repoPath, 'rev-parse', '--verify', ref], {
    cwd: REPO_ROOT,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) {
    return {
      ok: false,
      status: result.status ?? 1,
      error: String(result.stderr || result.stdout || '').trim()
    }
  }
  const hash = String(result.stdout || '').trim().toLowerCase()
  return {
    ok: HASH_RE.test(hash),
    hash,
    error: HASH_RE.test(hash) ? '' : `rev-parse returned non-hash: ${hash}`
  }
}

function isAncestor(repoPath, ancestor, descendant) {
  const result = spawnSync('git', ['-C', repoPath, 'merge-base', '--is-ancestor', ancestor, descendant], {
    cwd: REPO_ROOT,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return {
    ok: result.status === 0,
    error: result.status === 0 || result.status === 1
      ? ''
      : String(result.stderr || result.stdout || '').trim()
  }
}

function compareUpstreamRefs(refs, {
  strictLocal = false,
  historicalSnapshot = false,
  currentRefs = new Map()
} = {}) {
  return UPSTREAMS.map((upstream) => {
    const expected = refs.get(upstream.matrixName) || ''
    const currentExpected = currentRefs.get(upstream.manifestId) || ''
    const repoPath = process.env[upstream.repoEnv] || upstream.repoPath
    const parsed = HASH_RE.test(expected)
    const currentRefValid = HASH_RE.test(currentExpected)
    const local = revParse(repoPath, upstream.ref)
    const localAvailable = local.ok
    const matches = parsed && localAvailable && local.hash === expected
    const currentManifestMatches = currentRefValid && localAvailable && local.hash === currentExpected
    const ancestry = parsed && localAvailable && historicalSnapshot
      ? isAncestor(repoPath, expected, local.hash)
      : { ok: matches, error: '' }
    const skipped = !localAvailable && !strictLocal
    const refAuthorityOK = historicalSnapshot
      ? parsed && currentRefValid && ancestry.ok && currentManifestMatches
      : matches
    return {
      id: upstream.id,
      label: upstream.label,
      matrixName: upstream.matrixName,
      matrixRef: expected,
      matrixRefValid: parsed,
      repoPath,
      ref: upstream.ref,
      localRef: local.ok ? local.hash : '',
      currentManifestRef: currentExpected,
      currentManifestRefValid: currentRefValid,
      currentManifestMatches,
      historicalSnapshot,
      snapshotAncestor: ancestry.ok,
      localAvailable,
      skipped,
      ok: parsed && (refAuthorityOK || skipped),
      matches,
      error: !parsed
        ? `matrix ref missing or invalid for ${upstream.matrixName}`
        : historicalSnapshot && !currentRefValid
          ? `current source manifest ref missing or invalid for ${upstream.manifestId}`
          : ancestry.error || local.error || ''
    }
  })
}

function splitMarkdownTableRow(line) {
  const cells = []
  let current = ''
  let inCode = false
  const value = String(line || '').trim()
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    const previous = index > 0 ? value[index - 1] : ''
    if (char === '`' && previous !== '\\') {
      inCode = !inCode
      current += char
      continue
    }
    if (char === '|' && previous !== '\\' && !inCode) {
      cells.push(current.trim())
      current = ''
      continue
    }
    current += char
  }
  cells.push(current.trim())
  if (cells[0] === '') cells.shift()
  if (cells[cells.length - 1] === '') cells.pop()
  return cells
}

function parseMatrixRows(markdown) {
  const lines = String(markdown || '').split(/\r?\n/)
  let inSection = false
  let header = []
  const rows = []
  for (let index = 0; index < lines.length; index += 1) {
    const trimmed = lines[index].trim()
    if (/^##\s+P0\/P1 matrix\s*$/i.test(trimmed)) {
      inSection = true
      header = []
      continue
    }
    if (inSection && /^##\s+/.test(trimmed)) break
    if (!inSection || !trimmed.startsWith('|')) continue
    const cells = splitMarkdownTableRow(trimmed)
    if (cells.length === 0) continue
    if (cells[0] === '---') continue
    if (cells[0] === 'Category') {
      header = cells
      continue
    }
    if (header.length === 0) continue
    const columns = {}
    for (let cellIndex = 0; cellIndex < header.length; cellIndex += 1) {
      columns[header[cellIndex]] = cells[cellIndex] || ''
    }
    rows.push({
      line: index + 1,
      cells,
      columns,
      category: cells[0] || '',
      status: cells[header.indexOf('Status')] || '',
      regression: cells[header.indexOf('Regression')] || ''
    })
  }
  return rows
}

function parseRequestedAbsorptionMatrixRows(markdown) {
  const lines = String(markdown || '').split(/\r?\n/)
  let inSection = false
  let header = []
  const rows = []
  for (let index = 0; index < lines.length; index += 1) {
    const trimmed = lines[index].trim()
    if (/^##\s+Requested absorption matrix closure\s*$/i.test(trimmed)) {
      inSection = true
      header = []
      continue
    }
    if (inSection && /^##\s+/.test(trimmed)) break
    if (!inSection || !trimmed.startsWith('|')) continue
    const cells = splitMarkdownTableRow(trimmed)
    if (cells.length === 0) continue
    if (cells[0] === '---') continue
    if (/^Module$/i.test(cells[0] || '')) {
      header = cells
      continue
    }
    if (header.length === 0) continue
    const columns = {}
    for (let cellIndex = 0; cellIndex < header.length; cellIndex += 1) {
      columns[header[cellIndex]] = cells[cellIndex] || ''
    }
    rows.push({
      line: index + 1,
      cells,
      columns,
      module: columns.Module || cells[0] || '',
      status: columns.Status || ''
    })
  }
  return rows
}

function isMeaningfulRegression(value) {
  const regression = String(value || '').trim()
  return regression.length > 0 && !PLACEHOLDER_REGRESSION_RE.test(regression)
}

function auditMatrixCoverage(rows) {
  const domains = REQUIRED_MATRIX_DOMAINS.map((domain) => {
    const matchedRows = rows
      .filter((row) => domain.pattern.test(row.cells.join(' ')))
      .map((row) => ({
        line: row.line,
        category: row.category
      }))
    return {
      id: domain.id,
      label: domain.label,
      ok: matchedRows.length > 0,
      rows: matchedRows
    }
  })
  return {
    ok: domains.every((domain) => domain.ok),
    domains,
    missing: domains
      .filter((domain) => !domain.ok)
      .map((domain) => ({
        id: domain.id,
        label: domain.label
      }))
  }
}

function auditNamedHazardCoverage(rows) {
  const hazards = REQUIRED_NAMED_HAZARDS.map((hazard) => {
    const matchedRows = rows
      .filter((row) => {
        const text = row.cells.join(' ')
        if (hazard.categoryPattern && !hazard.categoryPattern.test(row.category)) return false
        return hazard.terms.every((term) => term.test(text))
      })
      .map((row) => ({
        line: row.line,
        category: row.category
      }))
    return {
      id: hazard.id,
      label: hazard.label,
      ok: matchedRows.length > 0,
      rows: matchedRows
    }
  })
  return {
    ok: hazards.every((hazard) => hazard.ok),
    hazards,
    missing: hazards
      .filter((hazard) => !hazard.ok)
      .map((hazard) => ({
        id: hazard.id,
        label: hazard.label
      }))
  }
}

function auditRequestedAbsorptionMatrix(markdown, sourceRoots) {
  const rows = parseRequestedAbsorptionMatrixRows(markdown)
  const missing = []
  const open = []
  const missingTerms = []
  const missingEvidenceRefs = []
  const missingFiles = []
  const lineOutOfRange = []
  const lineCountCache = new Map()
  const required = REQUIRED_ABSORPTION_MATRIX_ROWS.map((requiredRow) => {
    const row = rows.find((candidate) => requiredRow.modulePattern.test(candidate.module))
    if (!row) {
      missing.push({
        id: requiredRow.id,
        label: requiredRow.label
      })
      return {
        id: requiredRow.id,
        label: requiredRow.label,
        ok: false
      }
    }
    const status = String(row.status || '').trim()
    if (status !== 'fixed') {
      open.push({
        id: requiredRow.id,
        label: requiredRow.label,
        line: row.line,
        status
      })
    }
    const text = row.cells.join(' ')
    const absentTerms = requiredRow.terms
      .filter((term) => !term.test(text))
      .map((term) => term.source)
    if (absentTerms.length > 0) {
      missingTerms.push({
        id: requiredRow.id,
        label: requiredRow.label,
        line: row.line,
        missing: absentTerms
      })
    }
    for (const column of ['Analytix closure evidence', 'Regression']) {
      const refs = extractSourceRefs(row.columns[column] || '')
      if (refs.length === 0) {
        missingEvidenceRefs.push({
          id: requiredRow.id,
          label: requiredRow.label,
          line: row.line,
          column
        })
        continue
      }
      for (const ref of refs) {
        const sourceRoot = column === 'Regression' ? sourceRoots.regression : sourceRoots.analytix
        const resolvedPath = ref.path.startsWith('/') ? ref.path : resolve(sourceRoot, ref.path)
        const record = {
          id: requiredRow.id,
          label: requiredRow.label,
          line: row.line,
          column,
          ref: ref.raw,
          path: resolvedPath,
          targetLine: ref.line
        }
        if (!existsSync(resolvedPath) || !statSync(resolvedPath).isFile()) {
          missingFiles.push(record)
          continue
        }
        const fileLineCount = lineCountForFile(resolvedPath, lineCountCache)
        if (ref.line > fileLineCount) {
          lineOutOfRange.push({
            ...record,
            lineCount: fileLineCount
          })
        }
      }
    }
    return {
      id: requiredRow.id,
      label: requiredRow.label,
      line: row.line,
      ok: status === 'fixed' && absentTerms.length === 0
    }
  })
  return {
    ok: rows.length > 0 &&
      missing.length === 0 &&
      open.length === 0 &&
      missingTerms.length === 0 &&
      missingEvidenceRefs.length === 0 &&
      missingFiles.length === 0 &&
      lineOutOfRange.length === 0,
    rowCount: rows.length,
    required,
    missing,
    open,
    missingTerms,
    missingEvidenceRefs,
    missingFiles,
    lineOutOfRange
  }
}

function auditMatrixShape(rows) {
  const missingColumns = []
  const missingCells = []
  const missingRiskLevel = []
  for (const row of rows) {
    for (const column of REQUIRED_MATRIX_COLUMNS) {
      if (!(column in row.columns)) {
        missingColumns.push({
          line: row.line,
          category: row.category,
          column
        })
        continue
      }
      if (String(row.columns[column] || '').trim() === '') {
        missingCells.push({
          line: row.line,
          category: row.category,
          column
        })
      }
    }
    const risk = String(row.columns.Risk || '')
    if (!/(^|[^A-Za-z0-9])P[01]([^A-Za-z0-9]|$)/.test(risk)) {
      missingRiskLevel.push({
        line: row.line,
        category: row.category,
        risk
      })
    }
  }
  return {
    ok: missingColumns.length === 0 && missingCells.length === 0 && missingRiskLevel.length === 0,
    requiredColumns: REQUIRED_MATRIX_COLUMNS,
    missingColumns,
    missingCells,
    missingRiskLevel
  }
}

function auditAcceptableByContractRows(rows) {
  const rowsWithContractDecision = rows.filter((row) => /acceptable_by_contract/i.test(row.cells.join(' ')))
  const missingReason = rowsWithContractDecision
    .filter((row) => {
      const absorb = String(row.columns['Absorb?'] || '')
      const implementation = String(row.columns['Implementation / decision'] || '')
      const decisionText = `${absorb} ${implementation}`
      return !/(because|not applicable|no need|non-absorption|not absorbed|not absorb|rejected|rejects?|keeps?|instead|explicit|without|does not|do not|safe|unsafe|boundary|misaligned)/i.test(decisionText)
    })
    .map((row) => ({
      line: row.line,
      category: row.category,
      absorb: row.columns['Absorb?'] || '',
      implementation: row.columns['Implementation / decision'] || ''
    }))
  return {
    ok: missingReason.length === 0,
    checkedCount: rowsWithContractDecision.length,
    missingReason
  }
}

function auditUpstreamComparisonCells(rows) {
  const vague = []
  const missingDecision = []
  let checkedCount = 0
  for (const row of rows) {
    for (const sourceColumn of UPSTREAM_COMPARISON_COLUMNS) {
      const cell = String(row.columns[sourceColumn.column] || '').trim()
      checkedCount += 1
      if (VAGUE_UPSTREAM_CELL_RE.test(cell)) {
        vague.push({
          line: row.line,
          category: row.category,
          column: sourceColumn.column,
          value: cell
        })
        continue
      }
      if (extractSourceRefs(cell).length > 0 || EXPLICIT_UPSTREAM_DECISION_RE.test(cell)) {
        continue
      }
      missingDecision.push({
        line: row.line,
        category: row.category,
        column: sourceColumn.column,
        value: cell
      })
    }
  }
  return {
    ok: vague.length === 0 && missingDecision.length === 0,
    checkedCount,
    vague,
    missingDecision
  }
}

function extractCodeSpans(text) {
  return [...String(text || '').matchAll(/`([^`]+)`/g)].map((match) => match[1])
}

function extractSourceRefs(text) {
  const refs = []
  for (const codeSpan of extractCodeSpans(text)) {
    const match = SOURCE_REF_RE.exec(codeSpan.trim())
    if (!match?.groups) continue
    refs.push({
      raw: codeSpan.trim(),
      path: match.groups.path,
      line: Number.parseInt(match.groups.line, 10)
    })
  }
  return refs
}

function lineCountForFile(filePath, cache) {
  if (cache.has(filePath)) return cache.get(filePath)
  const lineCount = readFileSync(filePath, 'utf8').split(/\r?\n/).length
  cache.set(filePath, lineCount)
  return lineCount
}

function auditMatrixSourceRefs(rows, sourceRoots, { sourceAvailability = {}, strictLocal = false } = {}) {
  const checked = []
  const countsBySource = {}
  const missingRequired = []
  const missingMinimums = []
  const missingFiles = []
  const lineOutOfRange = []
  const lineCountCache = new Map()
  for (const row of rows) {
    for (const sourceColumn of SOURCE_REF_COLUMNS) {
      const cell = row.columns[sourceColumn.column] || ''
      const refs = extractSourceRefs(cell)
      countsBySource[sourceColumn.id] = (countsBySource[sourceColumn.id] || 0) + refs.length
      const skipUnavailableExternal =
        sourceColumn.minimumTotalRefs > 0 &&
        sourceAvailability[sourceColumn.id] === false &&
        !strictLocal
      if (sourceColumn.requiredPerRow && refs.length === 0) {
        missingRequired.push({
          line: row.line,
          category: row.category,
          column: sourceColumn.column
        })
      }
      if (skipUnavailableExternal) continue
      const sourceRoot = sourceRoots[sourceColumn.id] || REPO_ROOT
      for (const ref of refs) {
        const resolvedPath = ref.path.startsWith('/') ? ref.path : resolve(sourceRoot, ref.path)
        const record = {
          line: row.line,
          category: row.category,
          column: sourceColumn.column,
          source: sourceColumn.id,
          ref: ref.raw,
          path: resolvedPath,
          targetLine: ref.line
        }
        checked.push(record)
        if (!existsSync(resolvedPath) || !statSync(resolvedPath).isFile()) {
          missingFiles.push(record)
          continue
        }
        const fileLineCount = lineCountForFile(resolvedPath, lineCountCache)
        if (ref.line > fileLineCount) {
          lineOutOfRange.push({
            ...record,
            lineCount: fileLineCount
          })
        }
      }
    }
  }
  for (const sourceColumn of SOURCE_REF_COLUMNS) {
    const minimum = sourceColumn.minimumTotalRefs || 0
    if (minimum <= 0) continue
    const skipped =
      sourceAvailability[sourceColumn.id] === false &&
      !strictLocal
    const actual = countsBySource[sourceColumn.id] || 0
    if (!skipped && actual < minimum) {
      missingMinimums.push({
        source: sourceColumn.id,
        column: sourceColumn.column,
        minimum,
        actual
      })
    }
  }
  return {
    ok: missingRequired.length === 0 &&
      missingMinimums.length === 0 &&
      missingFiles.length === 0 &&
      lineOutOfRange.length === 0,
    checkedCount: checked.length,
    countsBySource,
    missingRequired,
    missingMinimums,
    missingFiles,
    lineOutOfRange
  }
}

function auditRequiredUpstreamFileRefs(rows, sourceRoots, { sourceAvailability = {}, strictLocal = false } = {}) {
  const refsBySource = new Map()
  for (const row of rows) {
    for (const sourceColumn of SOURCE_REF_COLUMNS) {
      if (!sourceColumn.column || !sourceRoots[sourceColumn.id]) continue
      const sourceRoot = sourceRoots[sourceColumn.id]
      const refs = extractSourceRefs(row.columns[sourceColumn.column] || '')
      for (const ref of refs) {
        const resolvedPath = ref.path.startsWith('/') ? resolve(ref.path) : resolve(sourceRoot, ref.path)
        if (!refsBySource.has(sourceColumn.id)) refsBySource.set(sourceColumn.id, new Set())
        refsBySource.get(sourceColumn.id).add(resolvedPath)
      }
    }
  }

  const required = REQUIRED_UPSTREAM_FILE_REFS.map((item) => {
    const sourceRoot = sourceRoots[item.source]
    const expectedPath = resolve(sourceRoot, item.path)
    const unavailable = sourceAvailability[item.source] === false
    const skipped = unavailable && !strictLocal
    const found = refsBySource.get(item.source)?.has(expectedPath) === true
    return {
      source: item.source,
      column: item.column,
      label: item.label,
      path: item.path,
      expectedPath,
      skipped,
      ok: skipped || found
    }
  })

  return {
    ok: required.every((item) => item.ok),
    required,
    missing: required
      .filter((item) => !item.ok)
      .map((item) => ({
        source: item.source,
        column: item.column,
        label: item.label,
        path: item.path,
        expectedPath: item.expectedPath
      }))
  }
}

function auditMatrixStatuses(markdown) {
  const rows = parseMatrixRows(markdown)
  const unresolved = rows
    .filter((row) => {
      const status = row.status.trim()
      return OPEN_MATRIX_STATUS_RE.test(status) || OPEN_MATRIX_STATUS_TOKEN_RE.test(status)
    })
    .map((row) => ({
      line: row.line,
      category: row.category,
      status: row.status
    }))
  const missingStatus = rows
    .filter((row) => row.status.trim() === '')
    .map((row) => ({
      line: row.line,
      category: row.category
    }))
  const invalidStatus = rows
    .filter((row) => {
      const status = row.status.trim()
      return status !== '' && !ALLOWED_MATRIX_STATUSES.has(status)
    })
    .map((row) => ({
      line: row.line,
      category: row.category,
      status: row.status
    }))
  const missingRegression = rows
    .filter((row) => !isMeaningfulRegression(row.regression))
    .map((row) => ({
      line: row.line,
      category: row.category,
      regression: row.regression
    }))
  const statuses = {}
  for (const row of rows) {
    const status = row.status.trim() || '<missing>'
    statuses[status] = (statuses[status] || 0) + 1
  }
  const coverage = auditMatrixCoverage(rows)
  const shape = auditMatrixShape(rows)
  return {
    ok: rows.length > 0 &&
      unresolved.length === 0 &&
      missingStatus.length === 0 &&
      invalidStatus.length === 0 &&
      missingRegression.length === 0 &&
      coverage.ok &&
      shape.ok,
    rowCount: rows.length,
    statuses,
    unresolved,
    missingStatus,
    invalidStatus,
    missingRegression,
    coverage,
    shape
  }
}

function auditValidationLedger(markdown) {
  const codeSpans = extractCodeSpans(markdown).map((span) => span.trim())
  const checks = REQUIRED_VALIDATION_LEDGER.map((required) => {
    const matches = codeSpans.filter((span) => required.pattern.test(span))
    return {
      id: required.id,
      label: required.label,
      ok: matches.length > 0,
      matches
    }
  })
  return {
    ok: checks.every((check) => check.ok),
    required: REQUIRED_VALIDATION_LEDGER.map((required) => ({
      id: required.id,
      label: required.label
    })),
    checks,
    missing: checks
      .filter((check) => !check.ok)
      .map((check) => ({
        id: check.id,
        label: check.label
      }))
  }
}

function auditReleaseBlockerDisclosures(markdown) {
  const text = String(markdown || '')
  const checks = REQUIRED_RELEASE_BLOCKER_DISCLOSURES.map((required) => ({
    id: required.id,
    label: required.label,
    ok: required.pattern.test(text)
  }))
  return {
    ok: checks.every((check) => check.ok),
    checks,
    missing: checks
      .filter((check) => !check.ok)
      .map((check) => ({
        id: check.id,
        label: check.label
      }))
  }
}

function auditPackagedRuntimeDisclosures(markdown) {
  const text = String(markdown || '')
  const checks = REQUIRED_PACKAGED_RUNTIME_DISCLOSURES.map((required) => ({
    id: required.id,
    label: required.label,
    ok: required.pattern.test(text)
  }))
  return {
    ok: checks.every((check) => check.ok),
    checks,
    missing: checks
      .filter((check) => !check.ok)
      .map((check) => ({
        id: check.id,
        label: check.label
      }))
  }
}

function auditProductRegressionDisclosures(markdown) {
  const text = String(markdown || '')
  const checks = REQUIRED_PRODUCT_REGRESSION_DISCLOSURES.map((required) => ({
    id: required.id,
    label: required.label,
    ok: required.pattern.test(text)
  }))
  return {
    ok: checks.every((check) => check.ok),
    checks,
    missing: checks
      .filter((check) => !check.ok)
      .map((check) => ({
        id: check.id,
        label: check.label
      }))
  }
}

async function buildAudit(options) {
  const matrixPath = resolve(REPO_ROOT, options.matrixPath)
  const markdown = await readFile(matrixPath, 'utf8')
  const refs = parseUpstreamRefs(markdown)
  const historicalSnapshot = /^Status:\s*Historical\b/im.test(markdown)
  const sourceManifest = JSON.parse(await readFile(resolve(REPO_ROOT, UPSTREAM_SOURCE_MANIFEST_PATH), 'utf8'))
  const currentRefs = new Map(
    (Array.isArray(sourceManifest.sources) ? sourceManifest.sources : [])
      .map((source) => [String(source.id || ''), String(source.commit || '').toLowerCase()])
  )
  const checks = compareUpstreamRefs(refs, {
    strictLocal: options.strictLocal,
    historicalSnapshot,
    currentRefs
  })
  const rows = parseMatrixRows(markdown)
  const matrix = auditMatrixStatuses(markdown)
  const namedHazards = auditNamedHazardCoverage(rows)
  const acceptableByContract = auditAcceptableByContractRows(rows)
  const upstreamComparison = auditUpstreamComparisonCells(rows)
  const sourceAvailability = Object.fromEntries(checks.map((check) => [check.id, check.localAvailable]))
  const sourceRoots = {
    analytix: REPO_ROOT,
    reasonix: process.env.ANALYTIX_UPSTREAM_REPO_REASONIX ||
      defaultUpstreamRepoPath('DeepSeek-Reasonix', '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    opencode: process.env.ANALYTIX_UPSTREAM_REPO_OPENCODE ||
      defaultUpstreamRepoPath('opencode', '/Users/sun/Projects/opencode'),
    codexdesktop: process.env.ANALYTIX_UPSTREAM_REPO_CODEX_DESKTOP_REBUILD ||
      defaultUpstreamRepoPath('CodexDesktop-Rebuild', '/Users/sun/Projects/CodexDesktop-Rebuild'),
    regression: REPO_ROOT
  }
  const requestedAbsorptionMatrix = auditRequestedAbsorptionMatrix(markdown, sourceRoots)
  const sourceRefs = auditMatrixSourceRefs(rows, sourceRoots, {
    sourceAvailability,
    strictLocal: options.strictLocal
  })
  const requiredUpstreamFiles = auditRequiredUpstreamFileRefs(rows, sourceRoots, {
    sourceAvailability,
    strictLocal: options.strictLocal
  })
  const validationLedger = auditValidationLedger(markdown)
  const releaseBlockers = auditReleaseBlockerDisclosures(markdown)
  const packagedRuntime = auditPackagedRuntimeDisclosures(markdown)
  const productRegression = auditProductRegressionDisclosures(markdown)
  const currentCapabilityReview = {
    ok: !historicalSnapshot,
    status: historicalSnapshot ? 'historical_snapshot_requires_current_goal_matrix' : 'current_matrix',
    reason: historicalSnapshot
      ? 'Historical rows cannot authorize current upstream capability or release claims.'
      : ''
  }
  return {
    schemaVersion: 1,
    id: 'runtime-closure-matrix-audit',
    matrixPath,
    strictLocal: options.strictLocal,
    historicalSnapshot,
    currentSourceManifestPath: resolve(REPO_ROOT, UPSTREAM_SOURCE_MANIFEST_PATH),
    ok: checks.every((check) => check.ok) &&
      currentCapabilityReview.ok &&
      matrix.ok &&
      namedHazards.ok &&
      requestedAbsorptionMatrix.ok &&
      acceptableByContract.ok &&
      upstreamComparison.ok &&
      sourceRefs.ok &&
      requiredUpstreamFiles.ok &&
      validationLedger.ok &&
      releaseBlockers.ok &&
      packagedRuntime.ok &&
      productRegression.ok,
    checks,
    currentCapabilityReview,
    matrix,
    namedHazards,
    requestedAbsorptionMatrix,
    acceptableByContract,
    upstreamComparison,
    sourceRefs,
    requiredUpstreamFiles,
    validationLedger,
    releaseBlockers,
    packagedRuntime,
    productRegression
  }
}

function selfTest() {
  const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-runtime-closure-matrix-audit-'))
  try {
    const sampleHash = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    const markdown = [
      '# Test',
      '',
      '## Upstream refs',
      '',
      '| Upstream | Ref |',
      '| --- | --- |',
      `| Reasonix \`origin/main-v2\` | \`${sampleHash}\` |`,
      '| OpenCode `origin/dev` | `bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` |',
      '| CodexDesktop-Rebuild `origin/master` | `cccccccccccccccccccccccccccccccccccccccc` |',
      '',
      '## Next'
    ].join('\n')
    const matrixPath = join(tempRoot, 'matrix.md')
    writeFileSync(matrixPath, markdown, 'utf8')
    const refs = parseUpstreamRefs(markdown)
    const parserOK =
      refs.get('Reasonix `origin/main-v2`') === sampleHash &&
      refs.get('OpenCode `origin/dev`') === 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' &&
      refs.get('CodexDesktop-Rebuild `origin/master`') === 'cccccccccccccccccccccccccccccccccccccccc'
    const invalidRefs = parseUpstreamRefs('## Upstream refs\n| Reasonix `origin/main-v2` | `not-a-hash` |')
    const matrixMarkdown = [
      '## P0/P1 matrix',
      '',
      '| Category | Regression | Status |',
      '| --- | --- | --- |',
      '| Closed row | `go test A|B` | fixed |',
      '| Contract row | n/a | acceptable_by_contract |',
      '| Open row | n/a | needs_fix |',
      '| Hidden blocked row | `go test ./...` | fixed but blocked by release state |',
      '| Vague fixed row | `go test ./...` | fixed for one slice |',
      '',
      '## Next'
    ].join('\n')
    const matrix = auditMatrixStatuses(matrixMarkdown)
    const coverageMarkdown = [
      '## P0/P1 matrix',
      '',
      '| Category | Regression | Status |',
      '| --- | --- | --- |',
      '| Turn lifecycle terminal CAS | `go test ./...` | fixed |',
      '| Interrupt/cancel execution | `go test ./...` | fixed |',
      '| SSE/replay heartbeat | `go test ./...` | fixed |',
      '| Provider/model endpoint | `npm run test -- provider.test.ts --run` | fixed |',
      '| Agent loop step recovery | `npm run test -- loop.test.ts --run` | fixed |',
      '| Durable store event log replay | `go test ./internal/server` | fixed |',
      '| Subagent session continuation | `go test ./...` | fixed |',
      '| Tools/MCP/plugin pairing | `go test ./internal/mcp` | fixed |',
      '| Renderer terminal timeline pinned summary | `npm run test -- renderer.test.ts --run` | fixed |',
      '| Main preload runtime process package bridge | `npm run test -- preload.test.ts --run` | fixed |',
      '| Security sandbox secret redaction | `go test ./internal/server` | fixed |',
      '',
      '## Next'
    ].join('\n')
    const coverageMatrix = auditMatrixStatuses(coverageMarkdown)
    const vagueRegressionMarkdown = [
      '## P0/P1 matrix',
      '',
      '| Category | Regression | Status |',
      '| --- | --- | --- |',
      '| Turn lifecycle terminal CAS | Same focused command above. | fixed |',
      '',
      '## Next'
    ].join('\n')
    const vagueRegressionMatrix = auditMatrixStatuses(vagueRegressionMarkdown)
    const incompleteShapeMarkdown = [
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Turn lifecycle terminal CAS | `source.ts:1` | Not applicable | Not applicable | Not applicable | risk without level | absorb | impl | `go test ./...` | fixed |',
      '',
      '## Next'
    ].join('\n')
    const incompleteShapeMatrix = auditMatrixStatuses(incompleteShapeMarkdown)
    const sourceFile = join(tempRoot, 'source.ts')
    writeFileSync(sourceFile, ['one', 'two', 'three'].join('\n'), 'utf8')
    const sourceRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Turn lifecycle terminal CAS | `source.ts:2` | `source.ts:1` | `source.ts:1` | `source.ts:1` | P0 | absorb | impl | `source.ts:3` | fixed |',
      '| Missing local source | no ref | Not applicable | Not applicable | Not applicable | P1 | absorb | impl | `source.ts:3` | fixed |',
      '| Bad line source | `source.ts:4` | Not applicable | Not applicable | Not applicable | P1 | absorb | impl | `source.ts:3` | fixed |',
      '',
      '## Next'
    ].join('\n'))
    const sourceRefs = auditMatrixSourceRefs(sourceRows, {
      analytix: tempRoot,
      reasonix: tempRoot,
      opencode: tempRoot,
      codexdesktop: tempRoot,
      regression: tempRoot
    })
    const sourceRefsMissingUpstream = auditMatrixSourceRefs(parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Turn lifecycle terminal CAS | `source.ts:2` | `source.ts:1` | Not applicable | Not applicable | P0 | absorb | impl | `source.ts:3` | fixed |',
      '',
      '## Next'
    ].join('\n')), {
      analytix: tempRoot,
      reasonix: tempRoot,
      opencode: tempRoot,
      codexdesktop: tempRoot,
      regression: tempRoot
    })
    const sourceRefsMissingRegression = auditMatrixSourceRefs(parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Turn lifecycle terminal CAS | `source.ts:2` | `source.ts:1` | `source.ts:1` | `source.ts:1` | P0 | absorb | impl | `go test ./...` | fixed |',
      '',
      '## Next'
    ].join('\n')), {
      analytix: tempRoot,
      reasonix: tempRoot,
      opencode: tempRoot,
      codexdesktop: tempRoot,
      regression: tempRoot
    })
    const validationLedger = auditValidationLedger([
      '`go test ./...`',
      '`go test ./internal/server`',
      '`npm run test`',
      '`npx vitest run packages/runtime/tests/child-agent-executor.test.ts packages/runtime/tests/delegation-runtime.test.ts packages/runtime/tests/task-job-orchestration-contract.test.ts packages/runtime/src/adapters/model/multi-provider-model-client.test.ts packages/runtime/src/adapters/model/compat-model-client.endpoint-format.test.ts packages/runtime/tests/thread-summary-route.test.ts`',
      '`npx vitest run src/renderer/src/agent/analytix-mapper.test.ts src/renderer/src/agent/analytix-runtime.test.ts src/renderer/src/components/PluginMarketplaceView.test.ts src/main/services/hub-agent-marketplace-service.test.ts packages/runtime/tests/skill-runtime.test.ts packages/runtime/tests/mcp-tool-provider.test.ts src/main/ipc/app-ipc-schemas.test.ts src/preload/preload-runtime-request.test.ts`',
      '`npm run typecheck`',
      '`npm run build:runtime`',
      '`npm run runtime:go:product-regression -- --json`',
      '`npm run scan:product-sovereignty`',
      '`npm run runtime:go:speed-cache-gate`',
      '`npm run build`',
      '`npm run dist:mac:arm64`',
      '`npm run runtime:go:packaged-gui-smoke -- --json --actual --timeout-ms 60000`',
      '`npm run runtime:go:packaged-soak -- --json --actual --no-write --timeout-ms 90000`',
      '`npm run lint`',
      '`node scripts/runtime-closure-matrix-audit.mjs --strict-local --json`',
      '`node plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs --release-guard --json`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --summary-json --staging-plan --readiness-plan`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --self-test-unrelated-dirty-groups --json`',
      '`node plugins/analytix-fund-analysis/scripts/doctor.mjs`',
      '`node plugins/analytix-fund-analysis/scripts/functional-eval.mjs`',
      '`node plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs --require-case-db --fail-on-gaps`',
      '`git diff --check`'
    ].join('\n'))
    const missingValidationLedger = auditValidationLedger([
      '`go test ./...`',
      '`npm run test`'
    ].join('\n'))
    const namedHazards = auditNamedHazardCoverage(REQUIRED_NAMED_HAZARDS.map((hazard, index) => ({
      line: index + 1,
      cells: [hazard.label, hazard.sample],
      columns: {},
      category: hazard.sampleCategory,
      status: 'fixed',
      regression: '`go test ./...`'
    })))
    const missingNamedHazards = auditNamedHazardCoverage(REQUIRED_NAMED_HAZARDS.slice(1).map((hazard, index) => ({
      line: index + 1,
      cells: [hazard.label, hazard.sample],
      columns: {},
      category: hazard.sampleCategory,
      status: 'fixed',
      regression: '`go test ./...`'
    })))
    const wrongCategoryNamedHazards = auditNamedHazardCoverage(REQUIRED_NAMED_HAZARDS.map((hazard, index) => ({
      line: index + 1,
      cells: [hazard.label, hazard.sample],
      columns: {},
      category: `Wrong ${hazard.sampleCategory}`,
      status: 'fixed',
      regression: '`go test ./...`'
    })))
    const absorptionMarkdown = [
      '## Requested absorption matrix closure',
      '',
      '| Module | Conclusion | Absorb target | Analytix closure evidence | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- |',
      ...REQUIRED_ABSORPTION_MATRIX_ROWS.map((row) =>
        `| ${row.label} | fixed | ${row.sample} | ${row.sample} \`source.ts:1\` | \`source.ts:1\` | fixed |`
      ),
      '',
      '## Next'
    ].join('\n')
    const requestedAbsorptionMatrix = auditRequestedAbsorptionMatrix(absorptionMarkdown, {
      analytix: tempRoot,
      regression: tempRoot
    })
    const missingRequestedAbsorptionMatrix = auditRequestedAbsorptionMatrix([
      '## Requested absorption matrix closure',
      '',
      '| Module | Conclusion | Absorb target | Analytix closure evidence | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- |',
      ...REQUIRED_ABSORPTION_MATRIX_ROWS.slice(1).map((row) =>
        `| ${row.label} | fixed | ${row.sample} | ${row.sample} \`source.ts:1\` | \`source.ts:1\` | fixed |`
      ),
      '',
      '## Next'
    ].join('\n'), {
      analytix: tempRoot,
      regression: tempRoot
    })
    const acceptableByContractRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Contract row | `source.ts:1` | Not applicable | Not applicable | Not applicable | P1 | acceptable_by_contract because analytix keeps the boundary | no upstream code needed because this boundary is explicit | `source.ts:1` | fixed |',
      '',
      '## Next'
    ].join('\n'))
    const acceptableByContract = auditAcceptableByContractRows(acceptableByContractRows)
    const missingAcceptableReasonRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Contract row | `source.ts:1` | Not applicable | Not applicable | Not applicable | P1 | acceptable_by_contract | done | `source.ts:1` | fixed |',
      '',
      '## Next'
    ].join('\n'))
    const missingAcceptableReason = auditAcceptableByContractRows(missingAcceptableReasonRows)
    const upstreamComparisonRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Upstream comparison row | `source.ts:1` | `source.ts:1` | Not applicable; analytix-owned boundary | Equivalent reference, not imported | P1 | absorb | impl | `source.ts:1` | fixed |',
      '',
      '## Next'
    ].join('\n'))
    const upstreamComparison = auditUpstreamComparisonCells(upstreamComparisonRows)
    const vagueUpstreamComparisonRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      '| Vague upstream row | `source.ts:1` | same as above | done | ok | P1 | absorb | impl | `source.ts:1` | fixed |',
      '',
      '## Next'
    ].join('\n'))
    const vagueUpstreamComparison = auditUpstreamComparisonCells(vagueUpstreamComparisonRows)
    const requiredUpstreamRoots = {
      reasonix: join(tempRoot, 'reasonix'),
      opencode: join(tempRoot, 'opencode'),
      codexdesktop: join(tempRoot, 'codexdesktop')
    }
    const requiredRefsFor = (source, refs = REQUIRED_UPSTREAM_FILE_REFS) =>
      refs
        .filter((item) => item.source === source)
        .map((item) => `\`${item.path}:1\``)
        .join(' ')
    const requiredUpstreamRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      `| Upstream file gate | \`source.ts:1\` | ${requiredRefsFor('reasonix')} | ${requiredRefsFor('opencode')} | ${requiredRefsFor('codexdesktop')} | P1 | absorb | impl | \`source.ts:1\` | fixed |`,
      '',
      '## Next'
    ].join('\n'))
    const requiredUpstreamFiles = auditRequiredUpstreamFileRefs(requiredUpstreamRows, requiredUpstreamRoots)
    const missingRequiredRows = parseMatrixRows([
      '## P0/P1 matrix',
      '',
      '| Category | analytix current evidence | Reasonix reference | OpenCode reference | CodexDesktop reference | Risk | Absorb? | Implementation / decision | Regression | Status |',
      '| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |',
      `| Upstream file gate | \`source.ts:1\` | ${requiredRefsFor('reasonix')} | ${requiredRefsFor('opencode')} | ${requiredRefsFor('codexdesktop', REQUIRED_UPSTREAM_FILE_REFS.slice(0, -1))} | P1 | absorb | impl | \`source.ts:1\` | fixed |`,
      '',
      '## Next'
    ].join('\n'))
    const missingRequiredUpstreamFiles = auditRequiredUpstreamFileRefs(missingRequiredRows, requiredUpstreamRoots)
    const releaseBlockers = auditReleaseBlockerDisclosures([
      '`node plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs --release-guard --json`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --summary-json --staging-plan --readiness-plan`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --self-test-unrelated-dirty-groups --json`',
      'phase4_release_guard_summary: pass=106 warn=0 fail=2',
      'phase4_release_guard_failures: release_git_tag_identity, release_git_worktree_clean',
      'release_git_tag_identity: analytix-fund-analysis-v0.16.10 release tag missing',
      'release_git_worktree_clean: dirty worktree',
      'publish_blocker_count:5',
      'tracked_dirty_count:73',
      'generated_evidence_dirty_count:809',
      'release_slice_dirty_count:51',
      'unrelated_dirty_count:22',
      'goal_wide_unrelated_dirty_count:22',
      'unknown_unrelated_dirty_count:0',
      'ready_for_authorized_stage:true',
      'stage_path_count:51',
      'leave_unstaged_count:22',
      'readiness_plan.blocker_count:3',
      'readiness_plan.candidate_blocker_count:1',
      'authorization required before stage commit tag push Hub marketplace'
    ].join('\n'))
    const missingReleaseBlockers = auditReleaseBlockerDisclosures([
      '`node plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs --release-guard --json`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --summary-json --staging-plan --readiness-plan`',
      '`node plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs --self-test-unrelated-dirty-groups --json`',
      'phase4_release_guard_summary: pass=106 warn=0 fail=2',
      'phase4_release_guard_failures: release_git_tag_identity, release_git_worktree_clean',
      'release_git_tag_identity: analytix-fund-analysis-v0.16.10 release tag missing',
      'release_git_worktree_clean: dirty worktree',
      'publish_blocker_count:5',
      'tracked_dirty_count:73',
      'generated_evidence_dirty_count:809',
      'release_slice_dirty_count:51',
      'unrelated_dirty_count:22',
      'goal_wide_unrelated_dirty_count:22',
      'ready_for_authorized_stage:true',
      'stage_path_count:51',
      'leave_unstaged_count:22',
      'readiness_plan.blocker_count:3',
      'readiness_plan.candidate_blocker_count:1',
      'authorization required before stage commit tag push Hub marketplace'
    ].join('\n'))
    const packagedRuntime = auditPackagedRuntimeDisclosures([
      'packaged_gui_smoke.actualPackagedAppLaunched:true',
      'packaged_gui_smoke.windowAnalytixPresent:true',
      'packaged_gui_smoke.legacyAliasesAbsent:true',
      'packaged_gui_smoke.runtimeInfoReachable:true',
      'packaged_gui_smoke.noBundledGoSource:true',
      'packaged_soak.threadTurnSseReplay:true',
      'packaged_soak.toolTimelineContinuations:true',
      'packaged_soak.providerRedaction:true'
    ].join('\n'))
    const missingPackagedRuntime = auditPackagedRuntimeDisclosures([
      'packaged_gui_smoke.actualPackagedAppLaunched:true',
      'packaged_gui_smoke.windowAnalytixPresent:true',
      'packaged_gui_smoke.legacyAliasesAbsent:true',
      'packaged_gui_smoke.runtimeInfoReachable:true',
      'packaged_gui_smoke.noBundledGoSource:true',
      'packaged_soak.threadTurnSseReplay:true',
      'packaged_soak.toolTimelineContinuations:true'
    ].join('\n'))
    const productRegression = auditProductRegressionDisclosures([
      'runtime_go_product_regression.status:passed',
      'runtime_go_product_regression.matrix.rowCount:35',
      'runtime_go_product_regression.runtimeClosureMatrixAudit:passed',
      'runtime_go_product_regression.speedCacheGate:passed',
      'runtime_go_product_regression.packagedGuiContract:passed',
      'runtime_go_product_regression.packagedSoakContract:passed',
      'runtime_go_product_regression.productSovereignty:passed'
    ].join('\n'))
    const missingProductRegression = auditProductRegressionDisclosures([
      'runtime_go_product_regression.status:passed',
      'runtime_go_product_regression.matrix.rowCount:35',
      'runtime_go_product_regression.runtimeClosureMatrixAudit:passed',
      'runtime_go_product_regression.speedCacheGate:passed',
      'runtime_go_product_regression.packagedGuiContract:passed',
      'runtime_go_product_regression.packagedSoakContract:passed'
    ].join('\n'))
    return {
      ok: parserOK &&
        invalidRefs.size === 0 &&
        matrix.rowCount === 5 &&
        matrix.unresolved.length === 2 &&
        matrix.unresolved[0]?.category === 'Open row' &&
        matrix.unresolved[1]?.category === 'Hidden blocked row' &&
        matrix.invalidStatus.length === 3 &&
        coverageMatrix.coverage.ok &&
        coverageMatrix.missingRegression.length === 0 &&
        vagueRegressionMatrix.missingRegression.length === 1 &&
        incompleteShapeMatrix.shape.missingRiskLevel.length === 1 &&
        sourceRefs.missingRequired.length === 1 &&
        sourceRefs.lineOutOfRange.length === 1 &&
        sourceRefs.missingFiles.length === 0 &&
        sourceRefs.missingMinimums.length === 0 &&
        sourceRefsMissingUpstream.missingMinimums.length === 2 &&
        sourceRefsMissingRegression.missingRequired.some((row) => row.column === 'Regression') &&
        validationLedger.ok &&
        missingValidationLedger.missing.some((item) => item.id === 'typecheck') &&
        missingValidationLedger.missing.some((item) => item.id === 'diff_check') &&
        namedHazards.ok &&
        missingNamedHazards.missing.some((item) => item.id === REQUIRED_NAMED_HAZARDS[0]?.id) &&
        wrongCategoryNamedHazards.missing.length === REQUIRED_NAMED_HAZARDS.length &&
        requestedAbsorptionMatrix.ok &&
        missingRequestedAbsorptionMatrix.missing.some((item) => item.id === REQUIRED_ABSORPTION_MATRIX_ROWS[0]?.id) &&
        acceptableByContract.ok &&
        missingAcceptableReason.missingReason.length === 1 &&
        upstreamComparison.ok &&
        vagueUpstreamComparison.vague.length === 3 &&
        requiredUpstreamFiles.ok &&
        missingRequiredUpstreamFiles.missing.some((item) => item.source === 'codexdesktop') &&
        releaseBlockers.ok &&
        missingReleaseBlockers.missing.some((item) => item.id === 'unknown_unrelated_zero') &&
        packagedRuntime.ok &&
        missingPackagedRuntime.missing.some((item) => item.id === 'packaged_soak_provider_redaction') &&
        productRegression.ok &&
        missingProductRegression.missing.some((item) => item.id === 'product_regression_product_sovereignty'),
      parserOK,
      rejectsInvalidHash: invalidRefs.size === 0,
      matrixStatusParserOK: matrix.rowCount === 5,
      detectsOpenMatrixStatus: matrix.unresolved.length === 2,
      detectsInvalidMatrixStatus: matrix.invalidStatus.length === 3,
      matrixCoverageParserOK: coverageMatrix.coverage.ok,
      matrixRegressionParserOK: vagueRegressionMatrix.missingRegression.length === 1,
      matrixShapeParserOK: incompleteShapeMatrix.shape.missingRiskLevel.length === 1,
      matrixSourceRefParserOK: sourceRefs.missingRequired.length === 1 && sourceRefs.lineOutOfRange.length === 1,
      matrixUpstreamSourceMinimumOK: sourceRefsMissingUpstream.missingMinimums.length === 2,
      matrixRegressionSourceRequiredOK: sourceRefsMissingRegression.missingRequired.some((row) => row.column === 'Regression'),
      validationLedgerOK: validationLedger.ok,
      missingValidationLedgerOK: missingValidationLedger.missing.some((item) => item.id === 'typecheck') &&
        missingValidationLedger.missing.some((item) => item.id === 'diff_check'),
      namedHazardCoverageOK: namedHazards.ok,
      missingNamedHazardCoverageOK: missingNamedHazards.missing.some((item) => item.id === REQUIRED_NAMED_HAZARDS[0]?.id),
      namedHazardCategoryGateOK: wrongCategoryNamedHazards.missing.length === REQUIRED_NAMED_HAZARDS.length,
      requestedAbsorptionMatrixOK: requestedAbsorptionMatrix.ok,
      missingRequestedAbsorptionMatrixOK: missingRequestedAbsorptionMatrix.missing.some((item) => item.id === REQUIRED_ABSORPTION_MATRIX_ROWS[0]?.id),
      acceptableByContractReasonOK: acceptableByContract.ok,
      missingAcceptableByContractReasonOK: missingAcceptableReason.missingReason.length === 1,
      upstreamComparisonOK: upstreamComparison.ok,
      vagueUpstreamComparisonOK: vagueUpstreamComparison.vague.length === 3,
      requiredUpstreamFilesOK: requiredUpstreamFiles.ok,
      missingRequiredUpstreamFilesOK: missingRequiredUpstreamFiles.missing.some((item) => item.source === 'codexdesktop'),
      releaseBlockerDisclosureOK: releaseBlockers.ok,
      missingReleaseBlockerDisclosureOK: missingReleaseBlockers.missing.some((item) => item.id === 'unknown_unrelated_zero'),
      packagedRuntimeDisclosureOK: packagedRuntime.ok,
      missingPackagedRuntimeDisclosureOK: missingPackagedRuntime.missing.some((item) => item.id === 'packaged_soak_provider_redaction'),
      productRegressionDisclosureOK: productRegression.ok,
      missingProductRegressionDisclosureOK: missingProductRegression.missing.some((item) => item.id === 'product_regression_product_sovereignty'),
      preservesPipeInsideCodeSpan: parseMatrixRows(matrixMarkdown)[0]?.regression === '`go test A|B`',
      matrixPath
    }
  } finally {
    rmSync(tempRoot, { recursive: true, force: true })
  }
}

function printHuman(audit) {
  console.log(`${audit.ok ? 'PASS' : 'FAIL'} ${audit.id}`)
  for (const check of audit.checks) {
    const status = check.ok ? (check.skipped ? 'SKIP' : 'OK') : 'FAIL'
    const detail = check.matches
      ? check.localRef
      : check.historicalSnapshot && check.snapshotAncestor && check.currentManifestMatches
        ? `historical=${check.matrixRef} current=${check.currentManifestRef}`
      : check.skipped
        ? check.error
        : check.error || `matrix=${check.matrixRef} local=${check.localRef}`
    console.log(`${status} ${check.label} ${check.ref}: ${detail}`)
  }
  if (!audit.currentCapabilityReview.ok) {
    console.log(`FAIL current capability review: ${audit.currentCapabilityReview.reason}`)
  }
  const matrixStatus = audit.matrix.ok ? 'OK' : 'FAIL'
  console.log(`${matrixStatus} matrix rows: ${audit.matrix.rowCount}`)
  for (const row of audit.matrix.unresolved) {
    console.log(`FAIL ${row.category}: ${row.status}`)
  }
  for (const row of audit.matrix.missingStatus) {
    console.log(`FAIL ${row.category}: missing status`)
  }
  for (const row of audit.matrix.missingRegression) {
    console.log(`FAIL ${row.category}: missing concrete regression`)
  }
  for (const domain of audit.matrix.coverage.missing) {
    console.log(`FAIL matrix coverage ${domain.id}: ${domain.label}`)
  }
  for (const row of audit.matrix.shape.missingColumns) {
    console.log(`FAIL ${row.category}: missing column ${row.column}`)
  }
  for (const row of audit.matrix.shape.missingCells) {
    console.log(`FAIL ${row.category}: empty cell in ${row.column}`)
  }
  for (const row of audit.matrix.shape.missingRiskLevel) {
    console.log(`FAIL ${row.category}: Risk must include P0 or P1`)
  }
  if (audit.sourceRefs) {
    const sourceStatus = audit.sourceRefs.ok ? 'OK' : 'FAIL'
    console.log(`${sourceStatus} source refs: ${audit.sourceRefs.checkedCount}`)
    for (const row of audit.sourceRefs.missingRequired) {
      console.log(`FAIL ${row.category}: missing source ref in ${row.column}`)
    }
    for (const source of audit.sourceRefs.missingMinimums) {
      console.log(`FAIL matrix source refs ${source.source}: expected at least ${source.minimum}, found ${source.actual}`)
    }
    for (const row of audit.sourceRefs.missingFiles) {
      console.log(`FAIL ${row.category}: missing source file ${row.ref}`)
    }
    for (const row of audit.sourceRefs.lineOutOfRange) {
      console.log(`FAIL ${row.category}: ${row.ref} exceeds ${row.lineCount} lines`)
    }
  }
  if (audit.namedHazards) {
    const hazardStatus = audit.namedHazards.ok ? 'OK' : 'FAIL'
    console.log(`${hazardStatus} named hazard coverage`)
    for (const item of audit.namedHazards.missing) {
      console.log(`FAIL named hazard ${item.id}: missing ${item.label}`)
    }
  }
  if (audit.requestedAbsorptionMatrix) {
    const absorptionStatus = audit.requestedAbsorptionMatrix.ok ? 'OK' : 'FAIL'
    console.log(`${absorptionStatus} requested absorption matrix rows: ${audit.requestedAbsorptionMatrix.rowCount}`)
    for (const item of audit.requestedAbsorptionMatrix.missing) {
      console.log(`FAIL absorption matrix ${item.id}: missing ${item.label}`)
    }
    for (const item of audit.requestedAbsorptionMatrix.open) {
      console.log(`FAIL absorption matrix ${item.id}: status ${item.status || '<missing>'}`)
    }
    for (const item of audit.requestedAbsorptionMatrix.missingTerms) {
      console.log(`FAIL absorption matrix ${item.id}: missing terms ${item.missing.join(', ')}`)
    }
    for (const item of audit.requestedAbsorptionMatrix.missingEvidenceRefs) {
      console.log(`FAIL absorption matrix ${item.id}: missing source ref in ${item.column}`)
    }
    for (const item of audit.requestedAbsorptionMatrix.missingFiles) {
      console.log(`FAIL absorption matrix ${item.id}: missing source file ${item.ref}`)
    }
    for (const item of audit.requestedAbsorptionMatrix.lineOutOfRange) {
      console.log(`FAIL absorption matrix ${item.id}: ${item.ref} exceeds ${item.lineCount} lines`)
    }
  }
  if (audit.acceptableByContract) {
    const contractStatus = audit.acceptableByContract.ok ? 'OK' : 'FAIL'
    console.log(`${contractStatus} acceptable_by_contract reasons: ${audit.acceptableByContract.checkedCount}`)
    for (const row of audit.acceptableByContract.missingReason) {
      console.log(`FAIL ${row.category}: acceptable_by_contract requires a reason in Absorb? or Implementation / decision`)
    }
  }
  if (audit.upstreamComparison) {
    const upstreamStatus = audit.upstreamComparison.ok ? 'OK' : 'FAIL'
    console.log(`${upstreamStatus} upstream comparison cells: ${audit.upstreamComparison.checkedCount}`)
    for (const row of audit.upstreamComparison.vague) {
      console.log(`FAIL ${row.category}: vague ${row.column} cell: ${row.value}`)
    }
    for (const row of audit.upstreamComparison.missingDecision) {
      console.log(`FAIL ${row.category}: ${row.column} lacks source ref or explicit decision`)
    }
  }
  if (audit.requiredUpstreamFiles) {
    const requiredStatus = audit.requiredUpstreamFiles.ok ? 'OK' : 'FAIL'
    console.log(`${requiredStatus} required upstream source files`)
    for (const item of audit.requiredUpstreamFiles.missing) {
      console.log(`FAIL upstream ${item.source}: missing ${item.path} in ${item.column}`)
    }
  }
  if (audit.validationLedger) {
    const ledgerStatus = audit.validationLedger.ok ? 'OK' : 'FAIL'
    console.log(`${ledgerStatus} validation ledger`)
    for (const item of audit.validationLedger.missing) {
      console.log(`FAIL validation ledger ${item.id}: missing ${item.label}`)
    }
  }
  if (audit.releaseBlockers) {
    const releaseStatus = audit.releaseBlockers.ok ? 'OK' : 'FAIL'
    console.log(`${releaseStatus} release blocker disclosures`)
    for (const item of audit.releaseBlockers.missing) {
      console.log(`FAIL release blocker disclosure ${item.id}: missing ${item.label}`)
    }
  }
  if (audit.packagedRuntime) {
    const packagedStatus = audit.packagedRuntime.ok ? 'OK' : 'FAIL'
    console.log(`${packagedStatus} packaged runtime disclosures`)
    for (const item of audit.packagedRuntime.missing) {
      console.log(`FAIL packaged runtime disclosure ${item.id}: missing ${item.label}`)
    }
  }
  if (audit.productRegression) {
    const productStatus = audit.productRegression.ok ? 'OK' : 'FAIL'
    console.log(`${productStatus} product regression disclosures`)
    for (const item of audit.productRegression.missing) {
      console.log(`FAIL product regression disclosure ${item.id}: missing ${item.label}`)
    }
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  if (options.selfTest) {
    const result = selfTest()
    if (options.json) console.log(JSON.stringify(result, null, 2))
    else console.log(`${result.ok ? 'PASS' : 'FAIL'} runtime-closure-matrix-audit self-test`)
    if (!result.ok) process.exitCode = 1
    return
  }
  const audit = await buildAudit(options)
  if (options.json) console.log(JSON.stringify(audit, null, 2))
  else printHuman(audit)
  if (!audit.ok) process.exitCode = 1
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
})
