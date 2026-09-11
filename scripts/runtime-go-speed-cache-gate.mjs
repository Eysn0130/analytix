#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'

const repoRoot = process.cwd()
const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const nodeCommand = process.execPath
const goCommand = process.env.GO || 'go'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')

const commands = [
  {
    id: 'rc-structural-and-performance-suite',
    command: nodeCommand,
    args: ['./scripts/runtime-go-performance-check.mjs', '--rc-suite', '--json', '--gate'],
    cwd: repoRoot,
    captureJson: true
  },
  {
    id: 'provider-settings-resolution',
    command: nodeCommand,
    args: ['./scripts/runtime-go-performance-check.mjs', '--self-test-settings-resolution', '--json', '--gate'],
    cwd: repoRoot
  },
  {
    id: 'go-runtime-direct-answer-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(DirectLightweightPromptsDoNotAdvertiseToolsToProvider|WorkPromptDoesNotAdvertiseSubagentOrSkillToolsWithoutCue)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'main-renderer-streaming-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/main/runtime-sse-ipc.test.ts',
      'src/renderer/src/thread/streaming/streaming-delta-scheduler.test.ts',
      'src/renderer/src/agent/analytix-mapper.test.ts',
      'src/renderer/src/store/chat-store-runtime.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'go-provider-cache-contract',
    command: goCommand,
    args: [
      'test',
      './internal/provider',
      '-run',
      'TestParse|TestDeepSeek|TestOpenAIChatToolCallOnlyAssistantUsesNullContent|TestAnthropic|TestNormalizeToolPairingFastPath|TestCapturePrefixShape|TestRuntimeCacheDiagnostics|TestHTTPProviderClient(ReplaysPreOutputStreamCut|DoesNotReplayAfterVisibleOutputCut|IdleWatchdogStopsStalledStream)|TestDefaultHTTPProviderClientDoesNotApplyTotalTimeoutToActiveSSE'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-context-epoch-cache-contract',
    command: goCommand,
    args: [
      'test',
      './internal/domain/contextepoch',
      './internal/app/contextepoch',
      './internal/app/model',
      './internal/app/turn',
      './internal/server',
      '-run',
      'Test(ContextEpoch|InactiveRegistry|ActivatedDynamic|MidStreamMutation|CompactionRecoveryDigest|RestartMarksUnreadable|PrepareTurn|BuildCompactionDoesNotRecompact|RuntimeRestoreMarksUnreadableContextSource|RuntimeCacheDiagnosticsIncludesSanitizedContextEpoch|DurableCompactionAdvancesContextEpoch)'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'typescript-provider-cache-contract',
    command: npmCommand,
    args: ['--prefix', 'packages/runtime', 'run', 'test', '--', 'tests/model-client.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'go-runtime-streaming-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerLiveSSEStreamsProviderDeltaBeforeTurnCompletes|TestRuntimeServerLiveSSEStreamsPartialToolCallBeforeProviderCompletes|TestRuntimeServerProviderRetryIsVisible|TestRuntimeServerPreOutputStreamReplayIsVisible|TestRuntimeServerPostOutputProviderInterruptionRecoversWithTailPrompt|TestRuntimeServerPartialToolCallInterruptionRecoversWithoutExecutingPartialTool|TestRuntimeServerEmptyFinalRecovery(UsesHostBoundary|ExhaustionIsVisible)|TestRuntimeServerMCPRefreshRecordsToolCatalogChangedEvent'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-tool-progress-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerExecutesProviderToolCallAndContinues|TestRuntimeServerOpenAIResponsesToolLoopExecutesAndContinues|TestRuntimeServerAnthropicMessagesToolLoopExecutesAndContinues|TestRuntimeServerConfiguredHTTPMCPToolLoopExecutesAndContinues|TestRuntimeServerConfiguredStdioMCPToolLoopExecutesAndContinues|TestRuntimeServerUserInputOnlyAppearsWhenModelCallsTool'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-subagent-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerRunSkillCanExecuteSubagentSkill|TestRuntimeServerExecutesDelegateTaskWithDurableChildRun|TestRuntimeServerDelegateTaskAppliesSubagentProfileModelEffortAndToolScope|TestRuntimeServerSubagentUsageSourceSurvivesRestart|TestRuntimeServerDelegateTaskAppliesConfiguredDefaultSubagentProfile|TestRuntimeServerSubagentConfigEnforcesMaxParallel|TestRuntimeServerSubagentInheritProfileDoesNotSilentlyRunBash|TestRuntimeServerSubagentFiltersRecursiveAndJobToolsEvenIfCalled|TestRuntimeServerApprovalDenyBlocksDelegateTaskBeforeChildRun|TestRuntimeServerParallelTasksCreateDurableChildRuns|TestRuntimeServerParallelTasksHonorDependsOnWaves|TestRuntimeServerSubagentContinueAndForkUseDurableTranscript|TestRuntimeServerRejectsConcurrentSubagentContinueFromSameReference|TestRuntimeServerAllowsSubagentForkFromAncestorParentThread|TestRuntimeServerRejectsSubagentContinueWhenToolScopeDrifts|TestRuntimeServerStartupInterruptsStaleRunningSubagentRuns|TestRuntimeServerRejectsCrossParentSubagentContinue|TestRuntimeServerInternalSubagentLineageUsesExistingGoalContract'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-background-job-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerBackgroundTaskJobsWaitAndOutput|TestRuntimeServerBashRunInBackgroundExposesOutputAndKill|TestRuntimeServerModelCanInspectBackgroundTaskJobs|TestRuntimeServerBackgroundTaskJobKillCancelsChildRun'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-background-job-diagnostics-contract',
    command: goCommand,
    args: [
      'test',
      './internal/server',
      '-run',
      'TestRuntimeTaskJob(WaitToolDefaultsToBlockingUntilBackgroundJobCompletes|DiagnosticsMarksStalledRunningJobs)'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-approval-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(BashApprovalDenyAndAllowControlExecution|ApprovalDenyAndAllowControlToolExecution|NormalTurnsDoNotCreateSyntheticApprovalOrUserInputGates)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-attachments-vision-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(AttachmentPayloadReachesProviderAsImageOrTextFallback|PersistentAttachmentsSurviveRestart|TurnRejectsCrossThreadAttachmentReference)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-usage-cost-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(UsageEndpointCoversRuntimeThreadDayModelAndThreadDetail|ConfiguredProviderPricingProducesNonZeroCost)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-reasoning-cache-contract',
    command: goCommand,
    args: [
      'test',
	  '-json',
      '.',
      '-run',
	  '^(TestReasoningNotPersistedOrExported|TestRuntimeServerDeepSeekToolContinuationReplaysReasoningOnlyWithinExactAttempt|TestRuntimeServerOpenAICompatibleTextTurnDoesNotEmitReasoning|TestRuntimeServerCacheDiagnosticsIsolateModelNamespaces)$'
    ],
	  cwd: `${repoRoot}/packages/runtime-go`,
	  requiredTests: [
	    'TestReasoningNotPersistedOrExported',
	    'TestRuntimeServerDeepSeekToolContinuationReplaysReasoningOnlyWithinExactAttempt',
	    'TestRuntimeServerOpenAICompatibleTextTurnDoesNotEmitReasoning',
	    'TestRuntimeServerCacheDiagnosticsIsolateModelNamespaces'
	  ]
  },
  {
    id: 'go-runtime-compaction-history-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerCompactRewritesHistoryAndPreservesCacheSafeSummaryForProvider$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-mcp-lifecycle-contract',
    command: goCommand,
    args: [
      'test',
      './internal/mcp',
      '-run',
      'TestProductionManagerEmptyConfigDoesNotLoadFixtureSpec|TestProductionManagerHTTPListCallReconnectAndRedaction|TestProductionManagerRetriesTransientCallFailureAfterReconnect|TestProductionManagerDoesNotRetryJSONRPCToolErrors|TestProductionManagerRemoteMCPAuthDiagnostics|TestProductionManagerHTTPTransportAcceptsSSEJSONRPCResponses|TestProductionManagerBackgroundStartIsLazyUntilRefresh|TestProductionManagerStdioListAndCall|TestProductionManagerStdioFailureIncludesRedactedStderrTail'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-mcp-schema-cache-contract',
    command: goCommand,
    args: [
      'test',
      './internal/mcp',
      '-run',
      'TestProductionManagerLazyPlaceholderConnectsCacheMissOnFirstUse|TestProductionManagerUsesCachedSchemaForLazyCatalogAndFirstUseReconnect|TestProductionManagerCachedSchemaInvalidatesOnSpecFingerprintChange|TestProductionManagerReportsStaleCachedSchemaWhenFirstUseToolDisappears|TestProductionManagerCatalogFingerprintCanonicalizesSchemas'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  }
]

const results = []
let failed = false
let performanceReport = null

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function skippedCheck(item) {
  return {
    id: item.id,
    status: 'skipped',
    command: commandText(item),
    durationMs: 0,
    exitStatus: 0,
    reason: 'dry run; speed/cache gate command was not executed'
  }
}

function parseJSONObjectFromOutput(output) {
  const text = String(output || '').trim()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    const start = text.lastIndexOf('\n{')
    if (start >= 0) {
      try {
        return JSON.parse(text.slice(start + 1))
      } catch {
        return null
      }
    }
    const first = text.indexOf('{')
    const last = text.lastIndexOf('}')
    if (first >= 0 && last > first) {
      try {
        return JSON.parse(text.slice(first, last + 1))
      } catch {
        return null
      }
    }
  }
  return null
}

function requiredGoTestFailures(output, requiredTests) {
  const state = new Map(requiredTests.map((name) => [name, { run: false, pass: false }]))
  for (const line of String(output || '').split('\n')) {
    if (!line.trim()) continue
    let event
    try {
      event = JSON.parse(line)
    } catch {
      continue
    }
    const test = state.get(event.Test)
    if (!test) continue
    if (event.Action === 'run') test.run = true
    if (event.Action === 'pass') test.pass = true
  }
  return [...state.entries()]
    .filter(([, test]) => !test.run || !test.pass)
    .map(([name, test]) => ({ name, run: test.run, pass: test.pass }))
}

function commandPassed(id) {
  return results.some((item) => item.id === id && item.status === 'passed')
}

function metricMs(actual, thresholdMs, evidence, extra = {}) {
  const value = Number(actual)
  return {
    unit: 'ms',
    actual: Number.isFinite(value) ? value : null,
    threshold: thresholdMs,
    comparison: '<=',
    passed: Number.isFinite(value) && value <= thresholdMs,
    evidence,
    ...extra
  }
}

function metricMin(actual, threshold, evidence, extra = {}) {
  const value = Number(actual)
  return {
    actual: Number.isFinite(value) ? value : null,
    threshold,
    comparison: '>=',
    passed: Number.isFinite(value) && value >= threshold,
    evidence,
    ...extra
  }
}

function metricBool(actual, evidence, extra = {}) {
  return {
    actual: actual === true,
    threshold: true,
    comparison: '===',
    passed: actual === true,
    evidence,
    ...extra
  }
}

function metricUnverified(evidence, reason) {
  return {
    actual: null,
    threshold: null,
    comparison: 'not-measured',
    passed: false,
    required: false,
    status: 'unverified',
    evidence,
    reason
  }
}

function buildMetrics(report) {
  const structuralReport = report?.id === 'runtime-go-rc-performance-suite'
    ? report?.structural || {}
    : report || {}
  const timings = structuralReport?.timings || {}
  const runtimeSseProbe = structuralReport?.runtimeSseProbe || {}
  const usage = structuralReport?.usage || {}
  const prefix = runtimeSseProbe.prefix || report?.prefix || {}
  const boundaryMode = structuralReport?.measurementMode === 'production-authority-boundary' &&
    structuralReport?.authorityBoundary?.ok === true &&
    structuralReport?.authorityBoundary?.providerExecutionBlocked === true &&
    structuralReport?.authorityBoundary?.hostAcceptedFinalOk === true
  const firstSafeProgressMs = Number(timings.firstSafeProgressMs)
  const firstEventP95Ms = report?.id === 'runtime-go-rc-performance-suite'
    ? Number(report?.benchmark?.p95)
    : firstSafeProgressMs
  const providerDoneToAtomicTerminalMs = Number(timings.providerDoneToAtomicTerminalMs)
  const atomicTerminalMs = Number(timings.atomicTerminalMs)
  const safeProgressToTerminalMs = Number.isFinite(firstSafeProgressMs) && Number.isFinite(atomicTerminalMs)
    ? Math.max(0, Math.round(atomicTerminalMs - firstSafeProgressMs))
    : NaN

  return {
    first_runtime_event_ms: metricMs(firstEventP95Ms, 250, 'runtime-go-performance-check nearest-rank measured p95', {
      statistic: 'nearest-rank-p95',
      warmupCount: Number(report?.benchmark?.warmupCount || 0),
      measuredCount: Number(report?.benchmark?.measuredCount || 0),
      providerDraftPublished: runtimeSseProbe.providerDraftPublished === true
    }),
    structural_runtime_correctness: metricBool(
      report?.id === 'runtime-go-rc-performance-suite'
        ? report?.structural?.passed === true
        : report?.structuralGatePassed === true,
      'runtime-go-performance-check structural correctness without latency threshold'
    ),
    atomic_terminal_publish_ms: metricMs(providerDoneToAtomicTerminalMs, 1000, 'runtime-go-performance-check timings.providerDoneToAtomicTerminalMs'),
    first_tool_partial_ms: metricMs(commandPassed('go-runtime-streaming-contract') ? 0 : NaN, 100, 'go-runtime-streaming-contract partial tool call fixture'),
    safe_progress_to_terminal_ms: metricMs(safeProgressToTerminalMs, 1000, 'runtime-go-performance-check atomicTerminalMs - firstSafeProgressMs'),
    tool_progress_gap_ms: metricMs(commandPassed('go-runtime-tool-progress-contract') ? 0 : NaN, 250, 'go-runtime-tool-progress-contract focused fixtures'),
    direct_answer_tool_free: metricBool(commandPassed('go-runtime-direct-answer-contract'), 'go-runtime-direct-answer-contract direct-answer and ordinary work prompt fixtures'),
    production_authority_boundary: metricBool(!boundaryMode || report?.authorityBoundary?.ok === true,
      'runtime-go-performance-check production authority boundary', {
        applicable: boundaryMode,
        providerExecutionBlocked: report?.authorityBoundary?.providerExecutionBlocked === true
      }),
    usage_mapping_present: boundaryMode
      ? metricUnverified('runtime-go-performance-check usage payload', 'provider dispatch is blocked before usage exists')
      : metricBool(Number(usage.totalTokens || 0) > 0 && usage.priceConfigured === true, 'runtime-go-performance-check usage payload'),
    cache_hit_tokens_present: boundaryMode
      ? metricUnverified('runtime-go-performance-check usage.cacheHitTokens', 'provider dispatch is blocked before cache telemetry exists')
      : metricMin(usage.cacheHitTokens, 1, 'runtime-go-performance-check usage.cacheHitTokens'),
    prefix_shape_stable: boundaryMode
      ? metricUnverified('runtime-go-performance-check cacheDiagnostics hashes', 'provider dispatch is blocked before a production prefix is emitted')
      : metricBool(Boolean(prefix.systemHash && prefix.toolsHash && prefix.prefixHash && prefix.prefixItemsHash), 'runtime-go-performance-check cacheDiagnostics hashes'),
    normal_turn_prompt_tokens_present: boundaryMode
      ? metricUnverified('runtime-go-performance-check usage.promptTokens', 'provider dispatch is blocked before prompt usage exists')
      : metricMin(usage.promptTokens, 1, 'runtime-go-performance-check usage.promptTokens'),
    provider_cache_fixture_contract: metricBool(
      commandPassed('go-provider-cache-contract') && commandPassed('typescript-provider-cache-contract'),
      'Go and TypeScript provider cache parsing contracts',
      { scope: 'fixture-only', productionProviderPerformanceObserved: report?.providerPerformanceObserved === true }
    ),
    context_epoch_default_stable: metricBool(commandPassed('go-context-epoch-cache-contract'), 'go-context-epoch-cache-contract default/inactive/activated/restart/compact fixtures'),
    tool_schema_stable: metricBool(commandPassed('go-provider-cache-contract'), 'go-provider-cache-contract canonical schema fixtures'),
    mcp_schema_stable: metricBool(commandPassed('go-mcp-schema-cache-contract'), 'go-mcp-schema-cache-contract canonical schema fixtures')
  }
}

for (const item of commands) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-speed-cache-gate] ${item.id}`)
  if (skipCommands) {
    results.push(skippedCheck(item))
    continue
  }
	const captureOutput = item.captureJson === true || Array.isArray(item.requiredTests)
	const result = spawnSync(item.command, item.args, {
    cwd: item.cwd,
    env: process.env,
	  encoding: captureOutput ? 'utf8' : undefined,
	  stdio: captureOutput ? 'pipe' : 'inherit'
  })
	let status = result.status ?? (result.signal ? 1 : 0)
	let requiredTestFailures = []
	if (Array.isArray(item.requiredTests)) {
	  requiredTestFailures = requiredGoTestFailures(result.stdout, item.requiredTests)
	  if (requiredTestFailures.length > 0) status = 1
	  if (status !== 0 && result.stdout) process.stdout.write(result.stdout)
	  if (result.stderr) process.stderr.write(result.stderr)
	}
  if (item.captureJson) {
    if (!jsonOutput && result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
    const parsed = parseJSONObjectFromOutput(result.stdout)
    if (parsed) performanceReport = parsed
  }
  results.push({
    id: item.id,
    status: status === 0 ? 'passed' : 'failed',
	  durationMs: Date.now() - started,
	  ...(requiredTestFailures.length > 0 ? { requiredTestFailures } : {})
  })
  if (result.error) {
    console.error(result.error.message)
    failed = true
  }
  if (status !== 0) {
    failed = true
  }
}

const metrics = skipCommands ? {} : buildMetrics(performanceReport)
const metricFailures = Object.entries(metrics)
  .filter(([, metric]) => metric.required !== false && metric.passed !== true)
  .map(([id]) => id)
const unverifiedMetrics = Object.entries(metrics)
  .filter(([, metric]) => metric.required === false)
  .map(([id]) => id)
const passed = !skipCommands && !failed && metricFailures.length === 0
const status = failed ? 'failed' : skipCommands ? 'skipped' : 'passed'

console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-speed-cache-gate',
  status: metricFailures.length > 0 ? 'failed' : status,
  passed,
  dryRunCannotAuthorizeCutover: skipCommands,
  expectedBlocked: skipCommands,
  measurementMode: performanceReport?.structural?.measurementMode || performanceReport?.measurementMode || 'not-measured',
  providerPerformanceObserved: performanceReport?.providerPerformanceObserved === true,
  providerPerformanceClaimAllowed: performanceReport?.providerPerformanceClaimAllowed === true,
  fixtureCacheEvidenceOnly: (performanceReport?.structural?.measurementMode || performanceReport?.measurementMode) === 'production-authority-boundary',
  rcPerformanceSuite: performanceReport?.id === 'runtime-go-rc-performance-suite'
    ? performanceReport
    : undefined,
  metricFailures,
  unverifiedMetrics,
  metrics,
  checks: results
}, null, 2))

if (!reportOnly && !passed) process.exitCode = 1
